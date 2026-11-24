package main

import (
	"errors"
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func quickLoadGameForTest(scene *GameScene, candidate Scene) (*Game, *playerSaveTestRepository) {
	game, repository := quickSaveGameForTest(scene)
	game.playerSaveSceneBuilder = func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return candidate, nil
	}
	game.sceneGeneration.Store(1)

	return game, repository
}

func TestQuickLoadPhysicalChordReplacesAllNormalMapBoundaries(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld, references.Deceit} {
		for _, control := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyControlRight} {
			t.Run(location.String()+"/"+control.String(), func(t *testing.T) {
				scene, held, _, callbacks := newRunicReferenceTestScene(t, location)
				setQuickSaveConfirmationForTest(scene, false)
				candidate := &playerSaveTestScene{}
				game, repository := quickLoadGameForTest(scene, candidate)
				date, position, elapsed := scene.gameState.DateTime, scene.gameState.MapState.PlayerLocation, scene.gameState.ElapsedMs
				randomBefore := scene.gameState.GetRNG().Calls()
				callbacks.Reset()
				held[control], held[ebiten.KeyQ], held[ebiten.KeyCapsLock] = true, true, true
				require.NoError(t, scene.Update(game))
				assert.Same(t, candidate, game.currentScene)
				assert.Equal(t, uint64(2), game.DebugSceneGeneration())
				assert.Equal(t, []player_save.SlotID{player_save.Slot06}, repository.loads)
				assert.Empty(t, repository.saves)
				assert.Equal(t, date, scene.gameState.DateTime)
				assert.Equal(t, position, scene.gameState.MapState.PlayerLocation)
				assert.Equal(t, elapsed, scene.gameState.ElapsedMs, "no outgoing update tail")
				assert.Equal(t, randomBefore, scene.gameState.GetRNG().Calls())
				assert.Empty(t, callbacks.TimeAdvanced())
				assert.True(t, scene.gameState.IsAtDungeonCommandBoundary(), "quickload never creates a dungeon token")
				assert.True(t, scene.disposed)
				assert.Zero(t, scene.dialogStack.Count())
			})
		}
	}
}

func TestQuickLoadUnderworldBoundary(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithFloor(-1).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	scene := newOwnedSurfaceCompletionTestScene(t, state)
	initializeDungeonUpdateTestScene(scene, state)
	held := map[ebiten.Key]bool{ebiten.KeyControlLeft: true, ebiten.KeyQ: true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	candidate := &playerSaveTestScene{}
	setQuickSaveConfirmationForTest(scene, false)
	game, repository := quickLoadGameForTest(scene, candidate)
	before := state.DateTime
	require.NoError(t, scene.Update(game))
	assert.Same(t, candidate, game.currentScene)
	assert.Len(t, repository.loads, 1)
	assert.Equal(t, before, state.DateTime)
}

func TestQuickLoadCombatDisposesFeedbackBeforePublishingNewScene(t *testing.T) {
	scene, held, _ := newCombatDirectionUpdateScene(t, "first_actor")
	state := scene.gameState
	require.True(t, state.SelectCombatPlayer(state.CombatScene.GetCurrentTurnIdentity().EntityIndex))
	projection, eligible := state.ProjectSoloCombatInput()
	require.True(t, eligible)
	scene.transientCommandInput.pending = pendingCombatCommand{active: true, key: ebiten.KeyG, opportunity: projection.Opportunity}
	scene.ShowActionFeedback("Lingering action card", false)
	scene.combatImpactEffects = []CombatImpactEffect{{Elapsed: 0.5}}
	require.NotEmpty(t, scene.transientActionFeedback)
	candidate := &playerSaveTestScene{}
	setQuickSaveConfirmationForTest(scene, false)
	game, repository := quickLoadGameForTest(scene, candidate)
	finalizers := 0
	state.EventQueueManager.TriggerFeedback("quickload_cleanup", event_queue.PriorityNormal, func(builder *event_queue.SequentialEventBuilder) {
		builder.First(func() {}).Wait(1000).Finally(func() {
			finalizers++
			assert.Same(t, scene, game.currentScene, "retirement finalizers still resolve the outgoing scene")
		})
	})
	before := snapshotBlockedCombatState(t, state)
	elapsed := state.ElapsedMs
	held[ebiten.KeyControlRight], held[ebiten.KeyQ] = true, true
	require.NoError(t, scene.Update(game))
	assert.Same(t, candidate, game.currentScene)
	assert.Equal(t, before, snapshotBlockedCombatState(t, state), "loading spends no action, resource, calendar time or RNG")
	assert.Equal(t, elapsed, state.ElapsedMs)
	assert.Len(t, repository.loads, 1)
	assert.Empty(t, repository.saves)
	assert.Equal(t, 1, finalizers)
	assert.True(t, state.EventQueueManager.IsShutdown())
	assert.Empty(t, scene.combatImpactEffects)
	assert.Empty(t, scene.transientActionFeedback)
	assert.False(t, scene.transientCommandInput.pending.active, "buffered Get must not start before quickload")
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dialogStack.Count())
	scene.Dispose()
	assert.Equal(t, 1, finalizers)
}

func TestQuickLoadControlledMonsterTurn(t *testing.T) {
	state, _, _ := setupControlledMonsterInputTest(t)
	scene := newOwnedSurfaceCompletionTestScene(t, state)
	held := attachCombatSoloResetUpdateInput(t, scene)
	candidate := &playerSaveTestScene{}
	setQuickSaveConfirmationForTest(scene, false)
	game, repository := quickLoadGameForTest(scene, candidate)
	before := snapshotBlockedCombatState(t, state)
	held[ebiten.KeyControlLeft], held[ebiten.KeyQ] = true, true
	require.NoError(t, scene.Update(game))
	assert.Same(t, candidate, game.currentScene)
	assert.Equal(t, before, snapshotBlockedCombatState(t, state))
	assert.Len(t, repository.loads, 1)
}

func TestQuickLoadCanonicalReplacementShutsDownCombatContinuations(t *testing.T) {
	scene, _, _ := newCombatDirectionUpdateScene(t, "first_actor")
	candidate := &playerSaveTestScene{}
	setQuickSaveConfirmationForTest(scene, false)
	game, _ := quickLoadGameForTest(scene, candidate)
	finalizers, completions := 0, 0
	scene.gameState.EventQueueManager.Workflow("old_combat_owner").Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().First(func() {}).Wait(1000).
		Finally(func() {
			finalizers++
			assert.Same(t, scene, game.currentScene)
		}).OnComplete(func() { completions++ }).Start()
	require.False(t, scene.quickLoadInputEligible(), "the keyboard cannot interrupt an action owner")
	// Exercise disposal independently through the canonical loading backend.
	_, err := game.quickLoadPlayerSave()
	require.NoError(t, err)
	assert.Equal(t, 1, finalizers)
	assert.Zero(t, completions, "shutdown must not start a post-combat continuation")
	assert.Same(t, candidate, game.currentScene)
	assert.True(t, scene.gameState.EventQueueManager.IsShutdown())
	scene.Dispose()
	assert.Equal(t, 1, finalizers)
}

func TestQuickLoadFailurePreservesReadyCombatAndBufferedIntent(t *testing.T) {
	scene, held, callbacks := newCombatDirectionUpdateScene(t, "first_actor")
	state := scene.gameState
	require.True(t, state.SelectCombatPlayer(state.CombatScene.GetCurrentTurnIdentity().EntityIndex))
	projection, eligible := state.ProjectSoloCombatInput()
	require.True(t, eligible)
	pending := pendingCombatCommand{active: true, key: ebiten.KeyG, opportunity: projection.Opportunity}
	scene.transientCommandInput.pending = pending
	setQuickSaveConfirmationForTest(scene, false)
	game, repository := quickLoadGameForTest(scene, &playerSaveTestScene{preflightErr: errors.New("cannot activate")})
	before := snapshotBlockedCombatState(t, state)
	held[ebiten.KeyControlLeft], held[ebiten.KeyQ] = true, true
	require.NoError(t, scene.Update(game))
	assert.Same(t, scene, game.currentScene)
	assert.Equal(t, uint64(1), game.DebugSceneGeneration())
	assert.Equal(t, pending, scene.transientCommandInput.pending)
	assert.Equal(t, before, snapshotBlockedCombatState(t, state))
	assert.False(t, scene.disposed)
	assert.False(t, state.EventQueueManager.IsShutdown())
	assert.Equal(t, "reviewed", scene.activePlayerSaveRevision)
	assert.Len(t, repository.loads, 1)
	assert.Zero(t, scene.dialogStack.Count(), "failure must not replace the source input owner")
	require.NotEmpty(t, callbacks.Messages)
	assert.Contains(t, callbacks.Messages[len(callbacks.Messages)-1], "Load failed:")
	for range 3 {
		require.NoError(t, scene.Update(game))
	}
	assert.Equal(t, pending, scene.transientCommandInput.pending)
	assert.Len(t, repository.loads, 1, "held failure must not retry")
}

func TestQuickLoadExtraModifiersNeverBecomeSave(t *testing.T) {
	for _, modifiers := range [][]ebiten.Key{
		{ebiten.KeyShiftLeft}, {ebiten.KeyMetaLeft}, {ebiten.KeyAltRight},
		{ebiten.KeyControlLeft, ebiten.KeyShiftRight}, {ebiten.KeyControlRight, ebiten.KeyAltLeft},
		{ebiten.KeyControlRight, ebiten.KeyMetaRight},
	} {
		t.Run(modifiers[len(modifiers)-1].String(), func(t *testing.T) {
			scene, held, _, _ := newRunicReferenceTestScene(t, references.Deceit)
			setQuickSaveConfirmationForTest(scene, false)
			game, repository := quickLoadGameForTest(scene, &playerSaveTestScene{})
			held[ebiten.KeyQ] = true
			for _, key := range modifiers {
				held[key] = true
			}
			date := scene.gameState.DateTime
			require.NoError(t, scene.Update(game))
			for _, key := range modifiers {
				delete(held, key)
			}
			for range 3 {
				require.NoError(t, scene.Update(game))
			}
			assert.Empty(t, repository.loads)
			assert.Empty(t, repository.saves)
			assert.Equal(t, date, scene.gameState.DateTime)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Zero(t, scene.dialogStack.Count())
		})
	}
}

func TestShiftQIsTypedIntoATextFieldButReservedInPlay(t *testing.T) {
	scene, held, _, _ := newRunicReferenceTestScene(t, references.Deceit)
	press := func(keys ...ebiten.Key) bool {
		clear(held)
		scene.quickLoadChordHeld = false
		for _, key := range keys {
			held[key] = true
		}
		scene.captureQuickLoadRequest(scene.beginTransientCommandInputFrame())

		return scene.quickLoadChordHeld
	}
	require.True(t, press(ebiten.KeyShiftLeft, ebiten.KeyQ), "in play a modified Q never becomes plain Q")

	scene.dialogStack.PushModalDialog(widgets.NewSaveDescriptionEntryModal(widgets.SaveDescriptionEntryModalConfig{
		Title: "Save", Prompt: "Description", Keyboard: scene.keyboard, MaxRunes: 22,
		OnResult: func(string, bool) {},
	}))
	require.False(t, press(ebiten.KeyShiftLeft, ebiten.KeyQ), "a capital Q belongs to the text field")
	require.True(t, press(ebiten.KeyControlLeft, ebiten.KeyShiftLeft, ebiten.KeyQ), "other chords stay reserved")
}

func TestQuickLoadBlockedChordCannotReviveAfterSecondaryOrModalCloses(t *testing.T) {
	for _, owner := range []string{"secondary", "modal", "atlas", "workflow", "focus"} {
		t.Run(owner, func(t *testing.T) {
			scene, held, _, _ := newRunicReferenceTestScene(t, references.Britain)
			now := time.Unix(1000, 0)
			scene.now = func() time.Time { return now }
			scene.startTime, scene.lastEffectsUpdate = now, now
			scene.elapsedBaseMs = scene.gameState.ElapsedMs
			setQuickSaveConfirmationForTest(scene, false)
			game, repository := quickLoadGameForTest(scene, &playerSaveTestScene{})
			focused, workflowActive := true, true
			var dialog *widgets.OverlayDialog
			switch owner {
			case "secondary":
				scene.secondaryKeyState = GetDirectionInput
			case "modal":
				dialog = widgets.NewOverlayDialog("Retained owner").AsConfirmation().WithUserDismissed(scene.keyboard).Build()
				dialog.ShowWithoutAnimation()
				scene.dialogStack.PushModalDialog(dialog)
			case "atlas":
				scene.openAtlas(nil)
			case "workflow":
				scene.gameState.EventQueueManager.Workflow("quickload_blocker").Priority(event_queue.PriorityNormal).
					Sequential().BlockInput().First(func() {}).
					While(func() bool { return workflowActive }, func() {}, DialogPollingRateMs).Start()
			case "focus":
				focused = false
				scene.transientCommandInput.focused = func() bool { return focused }
			}
			held[ebiten.KeyControlLeft], held[ebiten.KeyQ] = true, true
			require.NoError(t, scene.Update(game))
			assert.Empty(t, repository.loads)
			switch owner {
			case "secondary":
				assert.Equal(t, GetDirectionInput, scene.secondaryKeyState)
				scene.secondaryKeyState = PrimaryInput
			case "modal":
				require.False(t, dialog.IsDismissed())
				scene.dialogStack.MustRemoveWidgetInstance(dialog)
			case "atlas":
				scene.closeAtlasSession(true)
			case "workflow":
				require.True(t, scene.gameState.EventQueueManager.HasWorkflow("quickload_blocker"))
				workflowActive = false
				now = now.Add(time.Duration(DialogPollingRateMs) * time.Millisecond)
			case "focus":
				focused = true
			}
			delete(held, ebiten.KeyControlLeft)
			for range 5 {
				require.NoError(t, scene.Update(game))
			}
			assert.Empty(t, repository.loads, "a blocked request is not deferred")
			assert.Empty(t, repository.saves, "releasing Ctrl never turns the chord into Q")
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			clear(held)
			for range 5 {
				now = now.Add(time.Duration(DialogPollingRateMs) * time.Millisecond)
				require.NoError(t, scene.Update(game))
			}
			if owner == "workflow" {
				require.False(t, scene.gameState.EventQueueManager.HasWorkflow("quickload_blocker"))
			}
			require.True(t, scene.quickLoadInputEligible())
			held[ebiten.KeyControlRight], held[ebiten.KeyQ] = true, true
			require.NoError(t, scene.Update(game))
			assert.Len(t, repository.loads, 1, "a new physical press can retry")
		})
	}
}

func TestQuickLoadHeldEnemyChordNeverLoadsAtLaterPlayerTurn(t *testing.T) {
	scene, held := newBufferedCombatUpdateScene(t)
	setQuickSaveConfirmationForTest(scene, false)
	game, repository := quickLoadGameForTest(scene, &playerSaveTestScene{})
	held[ebiten.KeyControlRight], held[ebiten.KeyQ] = true, true
	require.NoError(t, scene.Update(game))
	assert.Empty(t, repository.loads)
	delete(held, ebiten.KeyControlRight)
	for range 200 {
		scene.gameState.ElapsedMs += 16
		require.NoError(t, scene.Update(game))
	}
	assert.Empty(t, repository.loads)
	assert.Empty(t, repository.saves)
	assert.False(t, scene.transientCommandInput.pending.active)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	clear(held)
	for range 200 {
		scene.gameState.ElapsedMs += 16
		require.NoError(t, scene.Update(game))
		if scene.quickLoadInputEligible() {
			break
		}
	}
	require.True(t, scene.quickLoadInputEligible())
	held[ebiten.KeyControlLeft], held[ebiten.KeyQ] = true, true
	require.NoError(t, scene.Update(game))
	assert.Len(t, repository.loads, 1)
}

// With Confirm Quick Save on, Ctrl+Q asks first, as Q does before it saves:
// N or Esc leaves the game alone, Y reloads the last slot.
func TestQuickLoadAsksFirstWhenConfirmQuickSaveIsOn(t *testing.T) {
	for _, answer := range []ebiten.Key{ebiten.KeyN, ebiten.KeyEscape, ebiten.KeyY} {
		t.Run(answer.String(), func(t *testing.T) {
			scene, held, _, callbacks := newRunicReferenceTestScene(t, references.Britain)
			setQuickSaveConfirmationForTest(scene, true)
			candidate := &playerSaveTestScene{}
			game, repository := quickLoadGameForTest(scene, candidate)
			held[ebiten.KeyControlLeft], held[ebiten.KeyQ] = true, true
			require.NoError(t, scene.Update(game))
			assert.Empty(t, repository.loads, "the question comes before the load")
			assert.Equal(t, 1, scene.dialogStack.Count())
			clear(held)
			require.NoError(t, scene.Update(game))
			assert.Empty(t, repository.loads)
			held[answer] = true
			require.NoError(t, scene.Update(game))
			assert.Zero(t, scene.dialogStack.Count())
			if answer == ebiten.KeyY {
				assert.Equal(t, []player_save.SlotID{player_save.Slot06}, repository.loads)
				assert.Contains(t, callbacks.Messages, "Load last save? Yes")

				return
			}
			assert.Empty(t, repository.loads)
			assert.Contains(t, callbacks.Messages, "Load last save? No")
			assert.Same(t, scene, game.currentScene)
		})
	}
}
