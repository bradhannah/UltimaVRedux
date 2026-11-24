package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

type countingDialog struct {
	updates int
}

func (*countingDialog) Draw(_ *ebiten.Image) {}
func (d *countingDialog) Update()            { d.updates++ }

func TestGameSceneUpdatesTopModalOnceAndProgressesNonBlockingDialog(t *testing.T) {
	scene := &GameScene{}
	modal := &countingDialog{}
	feedback := &countingDialog{}
	scene.dialogStack.PushModalDialog(modal)
	scene.dialogStack.PushNonBlockingDialog(feedback)

	scene.updateNonBlockingDialogs()
	blocked := scene.updateDialogs()

	assert.True(t, blocked)
	assert.Equal(t, 1, modal.updates)
	assert.Equal(t, 1, feedback.updates)
}

func TestFinishSpellCastingWorkflowRemovesOwnedPhaseInput(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1920, 1080)
	gs, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	keyboard := input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)
	scene := &GameScene{gameState: gs, keyboard: keyboard}
	dialog := widgets.NewInputBox("To phase:", nil, keyboard).
		WithRegexValidation("^[1-8]$").
		WithResultCallback(func(string, bool) {}).
		Build()
	scene.dialogStack.PushModalDialog(dialog)
	state := &spellCastingWorkflowState{phaseInputDialog: dialog}

	scene.finishSpellCastingWorkflow(state)

	assert.Nil(t, state.phaseInputDialog)
	assert.Equal(t, 0, scene.dialogStack.Count())
}

func TestActionFeedbackRetainsProtectedSurvivor(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1920, 1080)
	gs := &game_state.GameState{}
	gs.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return gs.ElapsedMs }, nil)
	scene := &GameScene{gameState: gs}

	scene.ShowActionFeedback("Unlocked!", false)
	scene.ShowActionFeedback("Key broke!", false)

	require.Len(t, scene.transientActionFeedback, 2)
	assert.Equal(t, 2, scene.dialogStack.Count())

	gs.ElapsedMs = widgets.ActionFeedbackDisplayMs + widgets.ActionFeedbackFadeOutMs
	scene.updateTransientActionFeedback()

	require.Len(t, scene.transientActionFeedback, 1, "the rearranged older card retains its protected interval")
	assert.Equal(t, 1, scene.dialogStack.Count())
	gs.ElapsedMs = 10_000
	scene.updateTransientActionFeedback()
	assert.Empty(t, scene.transientActionFeedback)
	assert.Zero(t, scene.dialogStack.Count())
	assert.False(t, scene.dialogStack.HasModalDialog())
}

func TestActionFeedbackIsolatedLifecycleUsesExactSceneClockBoundaries(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1920, 1080)
	gs := &game_state.GameState{}
	scene := &GameScene{gameState: gs}

	scene.ShowActionFeedback("a hidden door!", false)
	require.Len(t, scene.transientActionFeedback, 1)
	assert.Equal(t, 400, widgets.ActionFeedbackFadeInMs)

	feedback := scene.transientActionFeedback[0]
	for _, checkpoint := range []struct {
		at      int64
		phase   transientFeedbackPhase
		visible bool
	}{
		{399, transientFeedbackFadingIn, true},
		{400, transientFeedbackHolding, true},
		{1999, transientFeedbackHolding, true},
		{2000, transientFeedbackFadingOut, true},
		{2299, transientFeedbackFadingOut, true},
	} {
		gs.ElapsedMs = checkpoint.at
		scene.updateTransientActionFeedback()
		scene.updateNonBlockingDialogs()
		require.Len(t, scene.transientActionFeedback, 1)
		assert.Equal(t, checkpoint.phase, feedback.phase)
		assert.Equal(t, checkpoint.visible, feedback.dialog.IsVisible())
	}
	assert.Greater(t, feedback.dialog.DissolveLevel(), 0.99, "the final fade millisecond is nearly dissolved")
	gs.ElapsedMs = 2300
	scene.updateTransientActionFeedback()
	assert.Empty(t, scene.transientActionFeedback)
	assert.False(t, feedback.dialog.IsVisible())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestActionFeedbackUsesLowerPositionOnlyForDungeonExploration(t *testing.T) {
	t.Parallel()

	dungeonState := &game_state.GameState{}
	dungeonState.MapState.PlayerLocation.Location = references.Deceit
	worldState := &game_state.GameState{}
	worldState.MapState.PlayerLocation.Location = references.Britain
	combatState := &game_state.GameState{}
	combatState.MapState.PlayerLocation.Location = references.CombatLocation
	dungeonScene := &GameScene{gameState: dungeonState}
	worldScene := &GameScene{gameState: worldState}
	combatScene := &GameScene{gameState: combatState}

	assert.Equal(t, widgets.PositionMapCenterBottom, dungeonScene.actionFeedbackPosition())
	assert.Equal(t, widgets.PositionMapCenterLower, worldScene.actionFeedbackPosition())
	assert.Equal(t, widgets.PositionMapCenterLower, combatScene.actionFeedbackPosition())
}

func TestActionFeedbackAboveModalDoesNotHideOrRemoveModal(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1920, 1080)
	gs := &game_state.GameState{}
	scene := &GameScene{gameState: gs}
	modal := &countingDialog{}
	scene.dialogStack.PushModalDialog(modal)

	scene.ShowActionFeedback("No effect!", false)
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())

	gs.ElapsedMs = 10_000
	scene.updateTransientActionFeedback()

	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
}

// TestActionFeedbackExpirationToleratesEarlierRemoval prevents map or dialog
// transitions from leaving a stale lifecycle record that crashes on expiration.
func TestActionFeedbackExpirationToleratesEarlierRemoval(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(1920, 1080)
	gs := &game_state.GameState{}
	scene := &GameScene{gameState: gs}
	modal := &countingDialog{}
	scene.dialogStack.PushModalDialog(modal)
	scene.ShowActionFeedback("Klimb-Up!", false)
	require.Len(t, scene.transientActionFeedback, 1)
	feedbackDialog := scene.transientActionFeedback[0].dialog
	require.True(t, scene.dialogStack.RemoveWidgetInstance(feedbackDialog))

	gs.ElapsedMs = 10_000
	assert.NotPanics(t, scene.updateTransientActionFeedback)

	assert.Empty(t, scene.transientActionFeedback)
	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
}

func TestActionFeedbackInvalidLifecyclePanics(t *testing.T) {
	gs := &game_state.GameState{}
	scene := &GameScene{gameState: gs}
	feedback := &transientActionFeedback{
		dialog: &widgets.MessageDialog{},
		phase:  transientFeedbackPhase(99),
	}
	scene.transientActionFeedback = []*transientActionFeedback{feedback}

	assert.Panics(t, func() { scene.reconcileTransientActionFeedback(0, true) })
}

func TestCombatExitCompletesWhileActionFeedbackIsVisible(t *testing.T) {
	gs, mockCallbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(
		0,
		references.BritanniaCombatTerritory,
		game_state.CombatEntryNorth,
	))
	gs.CombatScene.Victory = true
	scene := &GameScene{gameState: gs}
	scene.ShowActionFeedback("No trap!", false)

	members := gs.ActiveCombatPartyMembers()
	scene.startCombatVictoryExitWorkflow()
	for index := range members {
		if index > 0 {
			gs.ElapsedMs += combatVictoryExitDelayMs
		}
		gs.EventQueueManager.Update()
	}
	gs.ElapsedMs += combatVictoryExitDelayMs
	gs.EventQueueManager.Update()

	assert.Nil(t, gs.CombatScene)
	assert.True(t, mockCallbacks.InputBlocked, "queue waits for key release after combat transition")

	gs.ElapsedMs = 10_000
	scene.updateTransientActionFeedback()
	gs.EventQueueManager.Update()

	assert.Empty(t, scene.transientActionFeedback)
	assert.False(t, scene.dialogStack.HasModalDialog())
	assert.False(t, mockCallbacks.InputBlocked)
}
