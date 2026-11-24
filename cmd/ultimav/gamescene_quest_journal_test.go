package main

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestGameSceneQuestJournalModalIsNoTimeAcrossGameplayContexts(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		combat   bool
		camping  bool
	}{
		{name: "exploration", location: references.BritanniaUnderworld},
		{name: "small map", location: references.Britain},
		{name: "dungeon", location: references.Deceit},
		{name: "combat", location: references.Britain, combat: true},
		{name: "camping", location: references.BritanniaUnderworld, camping: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithSystemCallbacks().
				WithRandomSeed(42).
				Build()
			if test.combat {
				require.NoError(t, state.TransitionToCombatMap(
					0,
					references.BritanniaCombatTerritory,
					game_state.CombatEntryNorth,
				))
				state.BeginCombatTurns(game_state.CombatTurnsPresented)
			}
			if test.camping {
				require.NoError(t, state.TransitionToCampingMap())
			}
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 7,
			}
			keyboard := newPresentationTestKeyboard()
			scene := &GameScene{
				gameState:   state,
				keyboard:    keyboard,
				output:      text.NewOutput(nil, 1, 64, 80),
				spriteSheet: sharedPresentationTestSpriteSheet(),
			}
			now := time.Unix(100, 0)
			scene.now = func() time.Time { return now }
			scene.startTime = now.Add(-time.Second)
			state.ElapsedMs = 321
			fired := 0
			state.EventQueueManager.Workflow("journal_ready_queue").Priority(event_queue.PriorityNormal).Sequential().
				First(func() { fired++ }).Start()
			state.EventQueueManager.TriggerFeedback("journal_ready_feedback", event_queue.PriorityNormal,
				func(q *event_queue.SequentialEventBuilder) { q.First(func() { fired++ }) })
			beforeCalls, beforeWind := state.GetRNG().Calls(), state.GetCurrentWind()
			state.SystemCallbacks.UI.ShowQuestJournalDossier = scene.ShowQuestJournalDossier
			callbacks.Reset()
			beforeDate := state.DateTime
			beforeEffect := state.ActiveDurationEffect
			var combatIdentity game_state.CombatTurnIdentity
			var combatQueue []game_state.CombatTurnForecastEntry
			var pendingAdvance bool
			if test.combat {
				combatIdentity = state.CombatScene.GetCurrentTurnIdentity()
				combatQueue = zStatsCombatQueueForecast(state.CombatScene.TurnQueue)
				pendingAdvance = state.CombatScene.HasScheduledCombatStep()
			}

			require.False(t, scene.handleQuestJournalShortcut(ebiten.KeyJ, false))
			require.Zero(t, scene.dialogStack.Count())
			require.True(t, scene.handleQuestJournalShortcut(ebiten.KeyJ, true))
			dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.QuestJournalDialog)
			require.True(t, ok)

			for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeyDown, ebiten.KeySpace} {
				now = now.Add(time.Hour)
				keyboard.SimulateKeyPress(key)
				require.NoError(t, scene.Update(nil))
				require.EqualValues(t, 321, scene.getElapsedMs())
			}
			keyboard.SimulateKeyPress(ebiten.KeyEscape)
			require.NoError(t, scene.Update(nil))
			assert.Zero(t, fired, "ready callbacks must not run even with frozen deadlines")
			assert.Equal(t, beforeCalls, state.GetRNG().Calls())
			assert.Equal(t, beforeWind, state.GetCurrentWind())
			assert.EqualValues(t, 321, state.ElapsedMs)
			assert.EqualValues(t, 321, scene.getElapsedMs(), "no paused wall-time catch-up")
			now = now.Add(10 * time.Millisecond)
			assert.EqualValues(t, 331, scene.getElapsedMs())

			assert.True(t, dialog.IsDismissed())
			assert.Zero(t, scene.dialogStack.Count())
			assert.Equal(t, []string{"Journal..."}, callbacks.CommandPrompts)
			assert.Equal(t, beforeDate, state.DateTime)
			assert.Equal(t, beforeEffect, state.ActiveDurationEffect)
			assert.Empty(t, callbacks.TimeAdvanced())
			if test.location == references.Deceit {
				assert.True(t, state.IsAtDungeonCommandBoundary())
			}
			if test.combat {
				assert.Equal(t, combatIdentity, state.CombatScene.GetCurrentTurnIdentity())
				assert.Equal(t, combatQueue, zStatsCombatQueueForecast(state.CombatScene.TurnQueue))
				assert.Equal(t, pendingAdvance, state.CombatScene.HasScheduledCombatStep())
			}
		})
	}
}

func TestGameSceneQuestJournalOwnsExactModalAndInputIsolation(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	keyboard := newPresentationTestKeyboard()
	scene := &GameScene{
		gameState:   state,
		keyboard:    keyboard,
		output:      text.NewOutput(nil, 1, 64, 80),
		spriteSheet: sharedPresentationTestSpriteSheet(),
	}
	underlay := &countingDialog{}
	scene.dialogStack.PushModalDialog(underlay)

	scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.QuestJournalDialog)
	require.True(t, ok)
	require.Equal(t, 2, scene.dialogStack.Count())

	keyboard.SimulateKeyPress(ebiten.KeyTab)
	require.True(t, scene.updateDialogs())
	assert.Equal(t, widgets.QuestJournalVirtuesTab, dialog.GetActiveTab())
	assert.Zero(t, underlay.updates, "only the top modal receives input")

	keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.True(t, scene.updateDialogs())
	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.Same(t, underlay, scene.dialogStack.PeekTopModalDialog())
}

func TestJournalSessionAcknowledgesOnlyVisitedOpeningRevision(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	scene := &GameScene{gameState: state, keyboard: newPresentationTestKeyboard(),
		output: text.NewOutput(nil, 1, 64, 80), spriteSheet: sharedPresentationTestSpriteSheet()}
	scene.journalChanged(0, 1)
	scene.journalChanged(1, 0)
	scene.journalChanged(2, 0)
	scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
	session := scene.journalSession
	scene.journalChanged(0, 2)
	scene.keyboard.SimulateKeyPress(ebiten.KeyTab)
	require.NoError(t, scene.Update(nil))
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(nil))
	assert.False(t, scene.journalVisits.IsNew(0, 1))
	assert.False(t, scene.journalVisits.IsNew(1, 0))
	assert.True(t, scene.journalVisits.IsNew(0, 2))
	assert.True(t, scene.journalVisits.IsNew(2, 0))
	scene.closeJournalSession(session, true)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestJournalFullUpdatePausesReadyQueuesAndResumesWithoutCatchup(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	now := time.Unix(100, 0)
	scene := &GameScene{
		gameState: state, gameConfig: state.GameConfig, keyboard: newPresentationTestKeyboard(),
		output: text.NewOutput(nil, 1, 64, 80), spriteSheet: sharedPresentationTestSpriteSheet(),
		now: func() time.Time { return now }, startTime: now,
		moongateVisualSystem: NewMoongateVisualSystem(state),
		activeEffectsOverlay: NewActiveEffectsOverlay(nil), windDisplay: NewWindDisplay(),
	}
	state.SystemCallbacks.UI.JournalChanged = scene.journalChanged
	state.ElapsedMs = 321
	state.ProcessWindThroughElapsed(state.ElapsedMs)
	before, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	beforeDate := state.DateTime
	var workflow, feedback int
	state.EventQueueManager.Workflow("journal_ready_workflow").Priority(event_queue.PriorityNormal).Sequential().
		First(func() { workflow++ }).Start()
	state.EventQueueManager.TriggerFeedback("journal_ready_feedback", event_queue.PriorityNormal,
		func(q *event_queue.SequentialEventBuilder) { q.First(func() { feedback++ }) })
	scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
	require.ErrorIs(t, scene.ValidateSaveSnapshotEligibility(), ErrGameSceneSnapshotNotRepresentable)
	for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeySlash, ebiten.KeyEscape, ebiten.KeyTab} {
		now = now.Add(time.Hour)
		scene.keyboard.SimulateKeyPress(key)
		require.NoError(t, scene.Update(nil))
		require.EqualValues(t, 321, scene.getElapsedMs())
	}
	// Pause belongs to the Journal owner, not whichever modal temporarily has focus.
	cover := &countingDialog{}
	scene.dialogStack.PushModalDialog(cover)
	now = now.Add(time.Hour)
	require.NoError(t, scene.Update(nil))
	require.Equal(t, 1, cover.updates)
	scene.dialogStack.MustRemoveWidgetInstance(cover)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(nil))
	require.Nil(t, scene.journalSession)
	require.Zero(t, workflow)
	require.Zero(t, feedback)
	require.EqualValues(t, 321, state.ElapsedMs)
	require.EqualValues(t, 321, scene.getElapsedMs())
	require.Equal(t, beforeDate, state.DateTime)
	require.Equal(t, before.Random.SourceCalls, state.GetRNG().Calls())
	// A different modal consumes only UI input after resuming; due queues run once.
	scene.dialogStack.PushModalDialog(cover)
	now = now.Add(5 * time.Millisecond)
	require.NoError(t, scene.Update(nil))
	require.EqualValues(t, 326, state.ElapsedMs)
	require.Equal(t, 1, workflow)
	require.Equal(t, 1, feedback)
	state.EventQueueManager.Shutdown()
	after, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, before.Wind, after.Wind, "5ms cannot replay five paused hours of wind")
	require.Equal(t, before.Random, after.Random)
	require.Equal(t, before.World.Calendar, after.World.Calendar)
}

func TestJournalNoticeCoalescesWithoutInputOrSaveRestriction(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	now := time.Unix(100, 0)
	scene := &GameScene{gameState: state, output: text.NewOutput(nil, 1, 64, 80),
		spriteSheet: sharedPresentationTestSpriteSheet(), now: func() time.Time { return now }}
	scene.output.EnableJournalHistory()
	scene.output.AddText("A retained message.")
	require.NoError(t, scene.ValidateSaveSnapshotEligibility())
	scene.journalChanged(journal.ChronicleTab, 1)
	scene.journalChanged(journal.VirtuesTab, 0)
	scene.updateJournalNotice()
	require.False(t, scene.journalNotice.pending)
	require.Empty(t, scene.journalNotice.queue.pending)
	require.NoError(t, scene.ValidateSaveSnapshotEligibility())
	require.False(t, scene.inputBlocked)
	require.Zero(t, scene.dialogStack.Count())
	snapshot, err := scene.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, "A retained message.\nJournal updated.\n[Shift+J] View journal", snapshot.Presentation.History)
	require.Equal(t, scene.journalVisits, snapshot.Presentation.Visits)
	// Drawing cannot duplicate the normal-output notice.
	screen := newPresentationRaster(1280, 720)
	defer screen.Deallocate()
	scene.drawDialogs(screen)
	require.Equal(t, snapshot.Presentation.History, scene.output.JournalHistory())
	now = now.Add(time.Hour)
	scene.updateJournalNotice()
	require.Equal(t, snapshot.Presentation.History, scene.output.JournalHistory())
}

func TestJournalDisposeDoesNotAcknowledgeAndReleasesPauseOnce(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	scene := &GameScene{gameState: state, keyboard: newPresentationTestKeyboard(),
		output: text.NewOutput(nil, 1, 64, 80), spriteSheet: sharedPresentationTestSpriteSheet()}
	scene.journalChanged(journal.ChronicleTab, 1)
	scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
	scene.Dispose()
	require.Nil(t, scene.journalSession)
	require.Zero(t, scene.dialogStack.Count())
	require.True(t, scene.journalVisits.IsNew(journal.ChronicleTab, 1))
	require.Equal(t, journalNotice{}, scene.journalNotice)
	require.NotPanics(t, scene.Dispose)
}

func TestPlainJStillStartsJimmyOnSmallMap(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState:         state,
		keyboard:          newPresentationTestKeyboard(),
		autoOpenDoorState: &AutoOpenDoorState{},
	}

	scene.smallMapInputHandler(ebiten.KeyJ)

	assert.Equal(t, JimmyDoorDirectionInput, scene.secondaryKeyState)
}
