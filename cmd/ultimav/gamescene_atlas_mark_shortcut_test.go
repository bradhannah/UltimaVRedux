package main

import (
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func newAtlasMarkShortcutScene(t *testing.T, location references.Location, floor references.FloorNumber) (
	*GameScene, map[ebiten.Key]bool, func(), *game_state.MockSystemCallbacks,
) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithLocation(location).WithFloor(floor).
		WithPlayerAt(1, 1).WithSystemCallbacks().WithRandomSeed(42).Build()
	state.DebugOptions.MonsterGen = false
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	scene := newSurfaceCompletionTestScene(state)
	// Shortcut/command ownership is detached from the platform text service.
	scene.appendInputChars = func(chars []rune) []rune { return chars }
	initializeDungeonUpdateTestScene(scene, state)
	keys := map[ebiten.Key]bool{}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(keyPressDelay, func(key ebiten.Key) bool { return keys[key] })
	now := time.Unix(100, 0)
	scene.now, scene.startTime, scene.elapsedBaseMs = func() time.Time { return now }, now, state.ElapsedMs
	callbacks.Reset()
	update := func() {
		t.Helper()
		require.NoError(t, scene.Update(nil))
	}
	t.Cleanup(scene.Dispose)
	return scene, keys, update, callbacks
}

func TestAtlasMarkShortcutOpensCurrentFixAndCancelsThroughExistingOwner(t *testing.T) {
	for _, control := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyControlRight} {
		t.Run(control.String(), func(t *testing.T) {
			scene, keys, update, callbacks := newAtlasMarkShortcutScene(t, references.BritanniaUnderworld, 0)
			state := scene.gameState
			scene.atlasView.Initialized, scene.atlasView.Zoom = true, 4
			scene.atlasView.CenterX, scene.atlasView.CenterY = 200, 180
			clock, calls, food, elapsed := state.DateTime, state.GetRNG().Calls(), state.PartyState.Inventory.Provisions.Food.Get(), state.ElapsedMs
			// Record the repeat identity without consuming N or capturing a key edge.
			keys[ebiten.KeyN] = true
			require.True(t, scene.keyboard.IsKeyPressedAndRegisterRepeatable(ebiten.KeyN))
			scene.keyboard.SetLastKeyPressedNowPlusMs(60000)
			require.False(t, scene.keyboard.IsKeyPressedAndRegisterRepeatable(ebiten.KeyN))
			keys[control] = true
			update()
			require.NotNil(t, scene.atlasSession)
			session, child := scene.atlasSession, scene.atlasSession.child
			require.NotNil(t, child)
			require.Same(t, child, scene.dialogStack.PeekTopModalDialog())
			require.Equal(t, 2, scene.dialogStack.Count())
			require.True(t, session.noteDraft.Creating())
			require.Equal(t, journal.SiteLocation{Map: references.BritanniaUnderworld, X: 1, Y: 1}, session.noteDraft.Record().Position)
			require.Equal(t, 4, scene.atlasView.Zoom)
			require.Equal(t, 200.0, scene.atlasView.CenterX, "the camera is not the captured position")
			for range 3 {
				update()
				require.Same(t, session, scene.atlasSession)
				require.Same(t, child, session.child)
			}
			delete(keys, control)
			update()
			require.True(t, scene.keyboard.IsWaitingForRelease(), "N cannot bleed into the label when Ctrl releases first")
			clear(keys)
			update()
			keys[ebiten.KeyEscape] = true
			update()
			require.Nil(t, session.child)
			require.Same(t, session.dialog, scene.dialogStack.PeekTopModalDialog())
			update()
			require.False(t, session.closing, "held child cancellation cannot also close the map")
			clear(keys)
			update()
			keys[ebiten.KeyEscape] = true
			scene.keyboard.SetAllowKeyPressImmediately()
			update()
			clear(keys)
			update()
			require.Nil(t, scene.atlasSession)
			require.Zero(t, scene.dialogStack.Count())
			require.Empty(t, state.CaptureAtlasNotes().Records)
			require.Equal(t, clock, state.DateTime)
			require.Equal(t, calls, state.GetRNG().Calls())
			require.Equal(t, food, state.PartyState.Inventory.Provisions.Food.Get())
			require.Equal(t, elapsed, state.ElapsedMs)
			require.Empty(t, callbacks.TimeAdvanced())
			require.NotContains(t, callbacks.CommandPrompts, "New Order...")
			keys[ebiten.KeyN] = true
			update()
			require.Contains(t, callbacks.CommandPrompts, "New Order...", "a fresh N immediately after closing the map still works")
		})
	}
}

func TestAtlasMarkShortcutEditsExistingNoteAndCommitsOnce(t *testing.T) {
	scene, keys, update, _ := newAtlasMarkShortcutScene(t, references.BritanniaUnderworld, 0)
	state := scene.gameState
	draft, err := state.BeginAtlasNoteDraft(0)
	require.NoError(t, err)
	note, err := state.CommitAtlasNote(draft, "Landing", "Return with supplies.")
	require.NoError(t, err)
	before := state.CaptureAtlasNotes()
	keys[ebiten.KeyControlRight], keys[ebiten.KeyN] = true, true
	update()
	require.False(t, scene.atlasSession.noteDraft.Creating())
	require.Equal(t, note, scene.atlasSession.noteDraft.Record())
	clear(keys)
	update()
	keys[ebiten.KeyControlRight], keys[ebiten.KeyEnter] = true, true
	for range 3 {
		update()
	}
	require.Nil(t, scene.atlasSession.child)
	require.Equal(t, before, state.CaptureAtlasNotes(), "a shortcut reopens the existing pin, never duplicates it")
	require.Equal(t, journal.AtlasNotePlaces, scene.atlasView.Filters.Places)
	require.Equal(t, journal.AtlasNoteKey(note.ID), scene.atlasView.SelectedKey)
	require.Equal(t, 1, scene.dialogStack.Count())
}

func TestAtlasMarkShortcutUsesExistingSextantEligibility(t *testing.T) {
	for _, test := range []struct {
		name     string
		sextant  uint16
		hour     byte
		flexible bool
		eligible bool
	}{
		{name: "missing sextant", hour: 19},
		{name: "strict day", sextant: 1, hour: 12},
		{name: "strict night", sextant: 1, hour: 19, eligible: true},
		{name: "flexible day", sextant: 1, hour: 12, flexible: true, eligible: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, keys, update, callbacks := newAtlasMarkShortcutScene(t, references.BritanniaUnderworld, 0)
			state := scene.gameState
			state.PartyState.Inventory.QuestItems.Set(references.Sextant, test.sextant)
			state.GameConfig.GameplayConfiguration.FlexibleSextantUse = test.flexible
			state.DateTime.Hour = test.hour
			callbacks.Reset()
			clock, calls := state.DateTime, state.GetRNG().Calls()
			keys[ebiten.KeyControlLeft], keys[ebiten.KeyN] = true, true
			update()
			require.NotNil(t, scene.atlasSession, "the atlas owns any eligibility explanation")
			require.Equal(t, test.eligible, scene.atlasSession.child != nil)
			if !test.eligible {
				require.NotEmpty(t, state.CurrentAtlasDossier().FixUnavailable)
				require.Equal(t, 1, scene.dialogStack.Count())
			}
			require.Equal(t, clock, state.DateTime)
			require.Equal(t, calls, state.GetRNG().Calls())
			require.Empty(t, state.CaptureAtlasNotes().Records)
			require.Empty(t, callbacks.TimeAdvanced())
		})
	}
}

func TestAtlasMarkShortcutUnavailableContextsDoNotBecomeNewOrder(t *testing.T) {
	for _, test := range []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
	}{
		{"town", references.Britain, 0},
		{"underworld", references.BritanniaUnderworld, -1},
		{"dungeon", references.Deceit, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, keys, update, callbacks := newAtlasMarkShortcutScene(t, test.location, test.floor)
			clock := scene.gameState.DateTime
			keys[ebiten.KeyControlLeft], keys[ebiten.KeyN] = true, true
			update()
			delete(keys, ebiten.KeyControlLeft)
			update()
			require.Nil(t, scene.atlasSession)
			require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			require.False(t, scene.gameState.EventQueueManager.HasWorkflow("new_order_workflow"))
			require.NotContains(t, callbacks.CommandPrompts, "New Order...")
			require.Equal(t, clock, scene.gameState.DateTime)
		})
	}
}

func TestAtlasMarkShortcutPreservesHigherPriorityOwners(t *testing.T) {
	for _, owner := range []string{"secondary", "modal", "input block", "workflow"} {
		t.Run(owner, func(t *testing.T) {
			scene, keys, update, callbacks := newAtlasMarkShortcutScene(t, references.BritanniaUnderworld, 0)
			var modal *countingDialog
			switch owner {
			case "secondary":
				scene.secondaryKeyState = AttackDirectionInput
			case "modal":
				modal = &countingDialog{}
				scene.dialogStack.PushModalDialog(modal)
			case "input block":
				scene.inputBlocked = true
			case "workflow":
				scene.gameState.EventQueueManager.Workflow("mark-blocker").Priority(event_queue.PriorityNormal).
					Sequential().First(func() {}).Wait(10000).Start()
			}
			clock := scene.gameState.DateTime
			keys[ebiten.KeyControlLeft], keys[ebiten.KeyN] = true, true
			update()
			require.Nil(t, scene.atlasSession)
			delete(keys, ebiten.KeyControlLeft)
			update()
			if owner == "secondary" {
				require.Equal(t, AttackDirectionInput, scene.secondaryKeyState)
			}
			if modal != nil {
				require.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
			}
			require.Equal(t, clock, scene.gameState.DateTime)
			require.NotContains(t, callbacks.CommandPrompts, "New Order...")
		})
	}
}

func TestAtlasMarkShortcutIsExactAndPlainNStillReorders(t *testing.T) {
	for _, modifiers := range [][]ebiten.Key{{}, {ebiten.KeyShiftLeft}, {ebiten.KeyAltLeft}, {ebiten.KeyMetaLeft},
		{ebiten.KeyControlLeft, ebiten.KeyShiftRight}, {ebiten.KeyControlRight, ebiten.KeyAltRight},
		{ebiten.KeyControlLeft, ebiten.KeyMetaRight}} {
		keys := map[ebiten.Key]bool{ebiten.KeyN: true}
		for _, key := range modifiers {
			keys[key] = true
		}
		scene := &GameScene{keyboard: input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })}
		require.False(t, scene.pollAtlasMarkRequest())
		require.False(t, scene.atlasMarkChordHeld)
	}
	scene, keys, update, callbacks := newAtlasMarkShortcutScene(t, references.BritanniaUnderworld, 0)
	scene.inputBlocked = true
	keys[ebiten.KeyControlLeft], keys[ebiten.KeyN] = true, true
	update()
	scene.inputBlocked = false
	delete(keys, ebiten.KeyControlLeft)
	update()
	require.Nil(t, scene.atlasSession, "blocked requests are not deferred until they become eligible")
	require.Empty(t, callbacks.CommandPrompts)
	clear(keys)
	update()
	keys[ebiten.KeyN] = true
	update()
	require.Contains(t, callbacks.CommandPrompts, "New Order...")
	require.True(t, scene.gameState.EventQueueManager.HasWorkflow("new_order_workflow"))
}

func TestAtlasMarkShortcutRejectedDuringTransitionCannotReplayWhileHeld(t *testing.T) {
	scene, keys, update, callbacks := newAtlasMarkShortcutScene(t, references.BritanniaUnderworld, 0)
	scene.inputBlocked = true
	scene.keyboard.SetForceWaitAnyKey(60000)
	keys[ebiten.KeyControlLeft], keys[ebiten.KeyN] = true, true
	update()
	require.True(t, scene.atlasMarkChordHeld)
	require.Nil(t, scene.atlasSession)
	// Model completion of the transition without wall-clock sleeps. A held
	// request rejected at that boundary must not turn into deferred authoring.
	scene.inputBlocked = false
	scene.keyboard.SetForceWaitAnyKey(0)
	for range 3 {
		update()
		require.Nil(t, scene.atlasSession)
	}
	delete(keys, ebiten.KeyControlLeft)
	update()
	require.Empty(t, callbacks.CommandPrompts)
	clear(keys)
	update()
	keys[ebiten.KeyControlRight], keys[ebiten.KeyN] = true, true
	update()
	require.NotNil(t, scene.atlasSession)
	require.NotNil(t, scene.atlasSession.child, "a new physical press may open normally")
}
