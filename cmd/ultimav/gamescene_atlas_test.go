package main

import (
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasSafeOpeningContexts(t *testing.T) {
	for _, location := range []references.Location{references.BritanniaUnderworld, references.Britain, references.Deceit} {
		state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(location).WithSystemCallbacks().Build()
		scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
		require.True(t, scene.canOpenAtlas(), "%s", location)
		scene.secondaryKeyState = LookDirectionInput
		require.False(t, scene.canOpenAtlas())
		scene.secondaryKeyState = PrimaryInput
		scene.inputBlocked = true
		require.False(t, scene.canOpenAtlas())
		scene.inputBlocked = false
		modal := &countingDialog{}
		scene.dialogStack.PushModalDialog(modal)
		require.False(t, scene.canOpenAtlas())
		scene.dialogStack.MustRemoveWidgetInstance(modal)
		state.EventQueueManager.Workflow("atlas-reject-workflow").Priority(event_queue.PriorityNormal).Sequential().First(func() {}).Start()
		require.False(t, scene.canOpenAtlas())
		state.EventQueueManager.Shutdown()
		for _, virtual := range []references.Location{references.CampingLocation, references.CombatLocation,
			references.ShrineLocation, references.CodexLocation, references.BlackthornPrison, references.FinaleLocation} {
			state.MapState.PlayerLocation.Location = virtual
			require.False(t, scene.canOpenAtlas(), "%s", virtual)
		}
	}
}

func TestAtlasFullUpdateFreezesQueuesAndReleaseFrame(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(42).Build()
	keys := map[ebiten.Key]bool{ebiten.KeyShiftRight: true, ebiten.KeyM: true}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
	now := time.Unix(100, 0)
	scene := &GameScene{gameState: state, gameConfig: state.GameConfig, keyboard: keyboard,
		output: text.NewOutput(nil, 1, 64, 80), now: func() time.Time { return now }, startTime: now,
		spriteSheet: sharedPresentationTestSpriteSheet()}
	state.ElapsedMs = 321
	beforeDate, beforeCalls, beforeWind := state.DateTime, state.GetRNG().Calls(), state.GetCurrentWind()
	require.NoError(t, scene.Update(nil), "accepted opening must precede ordinary scene updates")
	require.NotNil(t, scene.atlasSession)
	require.Equal(t, 1, scene.atlasView.Zoom)
	visits := scene.journalVisits
	fired := 0
	state.EventQueueManager.Workflow("atlas-ready-workflow").Priority(event_queue.PriorityNormal).Sequential().First(func() { fired++ }).Start()
	state.EventQueueManager.TriggerFeedback("atlas-ready-feedback", event_queue.PriorityNormal,
		func(q *event_queue.SequentialEventBuilder) { q.First(func() { fired++ }) })
	now = now.Add(time.Hour)
	require.NoError(t, scene.Update(nil))
	require.NotNil(t, scene.atlasSession, "held opener must not close")
	clear(keys)
	require.NoError(t, scene.Update(nil))
	keys[ebiten.KeyEqual] = true
	require.NoError(t, scene.Update(nil))
	require.Equal(t, 2, scene.atlasView.Zoom)
	clear(keys)
	require.NoError(t, scene.Update(nil))
	keys[ebiten.KeyShiftLeft], keys[ebiten.KeyM] = true, true
	require.NoError(t, scene.Update(nil))
	require.True(t, scene.atlasSession.closing)
	require.Zero(t, scene.dialogStack.Count())
	now = now.Add(time.Hour)
	require.NoError(t, scene.Update(nil))
	require.NotNil(t, scene.atlasSession, "closing key still owns pause until release")
	clear(keys)
	require.NoError(t, scene.Update(nil))
	require.Nil(t, scene.atlasSession)
	require.Zero(t, fired)
	require.EqualValues(t, 321, scene.getElapsedMs())
	require.EqualValues(t, 321, state.ElapsedMs)
	require.Equal(t, beforeDate, state.DateTime)
	require.Equal(t, beforeCalls, state.GetRNG().Calls())
	require.Equal(t, beforeWind, state.GetCurrentWind())
	require.Equal(t, visits, scene.journalVisits)
	now = now.Add(5 * time.Millisecond)
	require.EqualValues(t, 326, scene.getElapsedMs())
	state.EventQueueManager.Shutdown()
	view := scene.atlasView
	scene.openAtlas(nil)
	require.Equal(t, view, scene.atlasView)
	scene.Dispose()
	require.Nil(t, scene.atlasSession)
	require.Zero(t, scene.dialogStack.Count())
}

func TestAtlasUnavailableChordIsConsumedThroughRelease(t *testing.T) {
	keys := map[ebiten.Key]bool{ebiten.KeyShiftLeft: true, ebiten.KeyM: true}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
	scene := &GameScene{keyboard: keyboard}
	require.True(t, scene.pollAtlasRequest())
	require.True(t, scene.atlasChordHeld)
	require.False(t, keyboard.IsKeyPressedAndRegister(ebiten.KeyM), "cannot fall through to Mix")
	delete(keys, ebiten.KeyShiftLeft)
	require.False(t, scene.pollAtlasRequest())
	require.True(t, scene.atlasChordHeld)
	clear(keys)
	require.False(t, scene.pollAtlasRequest())
	require.False(t, scene.atlasChordHeld)
}

func TestFunctionKeysOpenAndCloseMapAndJournal(t *testing.T) {
	now := time.Unix(100, 0)
	keys := map[ebiten.Key]bool{}
	// The keyboard refuses a repeat of the same key within one input-clock
	// millisecond, so each tap advances this clock instead of the wall clock.
	inputMs := int64(1000)
	newScene := func(state *game_state.GameState) *GameScene {
		keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
		keyboard.SetElapsedMillisecondsSource(func() int64 { return inputMs })
		scene := &GameScene{gameState: state, gameConfig: state.GameConfig, keyboard: keyboard,
			output: text.NewOutput(nil, 1, 64, 80), spriteSheet: sharedPresentationTestSpriteSheet(),
			now: func() time.Time { return now }, startTime: now}
		t.Cleanup(state.EventQueueManager.Shutdown)

		return scene
	}
	// Each frame stays inside the reader's own pause: press, then release.
	tap := func(scene *GameScene, key ebiten.Key) {
		t.Helper()
		inputMs += keyPressDelay + 1
		keys[key] = true
		require.NoError(t, scene.Update(nil))
		clear(keys)
		require.NoError(t, scene.Update(nil))
	}

	atlasState, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(42).Build()
	atlasScene := newScene(atlasState)
	tap(atlasScene, widgets.AtlasFunctionKey)
	require.NotNil(t, atlasScene.atlasSession, "F6 opens the map like Shift+M")
	tap(atlasScene, widgets.AtlasFunctionKey)
	require.Nil(t, atlasScene.atlasSession, "F6 closes the map like Shift+M")

	journalState := newGameActionCallbacksKnowledgeState(t)
	journalScene := newScene(journalState)
	journalState.SystemCallbacks.UI.JournalChanged = journalScene.journalChanged
	journalState.SystemCallbacks.UI.ShowQuestJournalDossier = journalScene.ShowQuestJournalDossier
	tap(journalScene, widgets.QuestJournalFunctionKey)
	require.NotNil(t, journalScene.journalSession, "F5 opens the journal like Shift+J")
	tap(journalScene, widgets.QuestJournalFunctionKey)
	require.Nil(t, journalScene.journalSession, "F5 closes the journal like Shift+J")
}
