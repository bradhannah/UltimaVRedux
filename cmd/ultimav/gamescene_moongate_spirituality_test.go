package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestMoongateSpiritualityHandoffUsesCapturedRouteAndRestoresSource(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	state := scene.gameState
	state.TransitionToLocation(references.LordBritishsCastle, 1, references.Position{X: 15, Y: 17},
		game_state.TransitionFullLocation, false)
	state.GetMoonstoneSystem().BuryMoonstone(0, 15, 17, uint8(references.LordBritishsCastle), 1)
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 0, Minute: 9}
	stabilizeMoongateStage(t, state)
	require.True(t, state.IsPlayerOnActiveMoongate(), "source stone must match the nonzero map floor")
	callbacks.Reset()
	source := state.MapState.PlayerLocation
	beforeJournal := state.CurrentQuestJournalDossier().Moonstones
	resetSourceRenderer := state.SystemCallbacks.Visual.ResetTeleportationSourceRenderer
	resetCalls := 0
	state.SystemCallbacks.Visual.ResetTeleportationSourceRenderer = func() {
		resetCalls++
		resetSourceRenderer()
	}
	cleanupCalls := 0
	state.SystemCallbacks.Visual.CleanupAvatarDissolveEffect = func() {
		cleanupCalls++
		assert.Equal(t, source, state.MapState.PlayerLocation, "source must finalize before shrine entry")
		assert.Nil(t, state.ShrineScene)
		assert.Equal(t, 1, resetCalls)
		assert.True(t, callbacks.InputBlocked, "source cleanup retains input ownership")
	}
	state.SystemCallbacks.Visual.StabilizeTeleportationDestinationRenderer = func(game_state.MoongateDestination) {
		t.Error("Spirituality must not stabilize an ordinary destination renderer")
	}
	blockInput := state.SystemCallbacks.Visual.BlockAllInput
	state.SystemCallbacks.Visual.BlockAllInput = func(blocked bool) {
		if blocked && callbacks.BlockInputCalls == 1 {
			assert.Equal(t, 1, cleanupCalls, "source must finalize before successor acquires input")
			assert.Equal(t, 1, callbacks.UnblockInputCalls)
			assert.False(t, state.EventQueueManager.HasWorkflow(game_state.MoongateTeleportWorkflow))
		}
		blockInput(blocked)
	}

	scene.triggerMoongateTravel()
	require.True(t, state.EventQueueManager.HasWorkflow(game_state.MoongateTeleportWorkflow))
	assert.Nil(t, state.ShrineScene, "shrine transition must wait for the reserved workflow")

	// Crossing the boundary after review must not change the captured route.
	state.DateTime.Minute = 10
	callbacks.BindGameState(state)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	require.NotNil(t, state.ShrineScene)
	assert.Equal(t, references.ShrineLocation, state.MapState.PlayerLocation.Location)
	assert.Equal(t, game_state.SpiritualityShrineIndex, state.ShrineScene.ShrineIndex)
	assert.Equal(t, source, state.ShrineScene.OriginalPlayerLocation)
	assert.Equal(t, 1, resetCalls)
	assert.Equal(t, 1, cleanupCalls)
	assert.Equal(t, []float64{0}, callbacks.MapFadeLevels, "shrine handoff only clears source fade")
	assert.Empty(t, callbacks.TimeAdvanced(), "entry must not charge ordinary teleport time")
	assert.True(t, callbacks.InputBlocked)
	assert.Equal(t, game_state.MoongateStageHidden, state.MoongateStage())

	enterTranscriptText(t, scene, transcript, "WRONG")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "Thy thoughts are unfocused."))
	acknowledgeTranscript(t, scene, transcript)
	drainSceneWorkflow(t, scene, shrineMeditationWorkflowName, 1000)

	assert.Equal(t, source, state.MapState.PlayerLocation)
	assert.Nil(t, state.ShrineScene)
	assert.Nil(t, scene.miniMapScene)
	assert.False(t, scene.dialogStack.HasOpenDialog())
	assert.Equal(t, 2, callbacks.BlockInputCalls)
	assert.Equal(t, 2, callbacks.UnblockInputCalls)
	assert.False(t, callbacks.InputBlocked)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.Equal(t, []int{game_state.ShrineTimeCostMinutes}, callbacks.TimeAdvanced())
	finishedDate := state.DateTime
	state.EventQueueManager.Update()
	assert.Equal(t, finishedDate, state.DateTime, "finished shrine must not charge twice")
	assert.Equal(t, 1, cleanupCalls)
	assert.Equal(t, 2, callbacks.UnblockInputCalls)
	assert.Equal(t, beforeJournal, state.CurrentQuestJournalDossier().Moonstones, "Spirituality is not moonstone arrival")
}

func TestMoongateFailedActivationContractsAllGatesWithoutTime(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	state := scene.gameState
	stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(0)
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.BritanniaUnderworld,
		Floor:    references.MainFloor,
		Position: references.Position{X: references.Coordinate(stone.X), Y: references.Coordinate(stone.Y)},
	}
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 21}
	stabilizeMoongateStage(t, state)
	sourceIndex := (state.DateTime.GetMoongateMoonPhase().ToMoonstoneIndex() + 1) % 8
	for index := 0; index < 8; index++ {
		if index != sourceIndex {
			state.GetMoonstoneSystem().GetMoonstoneByIndex(index).State = game_state.MoonstoneStateInInventory
		}
	}
	stone = state.GetMoonstoneSystem().GetMoonstoneByIndex(sourceIndex)
	state.MapState.PlayerLocation.Position = references.Position{X: references.Coordinate(stone.X), Y: references.Coordinate(stone.Y)}
	// A captured failed activation must retain its ordinary owner's prior cost.
	callbacks.BindGameState(state)
	state.AdvanceGameplayMinutes(2)
	before := state.DateTime
	beforeJournal := state.CurrentQuestJournalDossier().Moonstones
	state.SetRandomSeed(123)
	result := state.ReviewMoongateTravel()
	require.True(t, result.FailedButActivated)
	scene.triggerMoongateTravel()
	drainSceneWorkflow(t, scene, "moongate_activation", 1000)
	assert.Equal(t, game_state.MoongateStageHidden, state.MoongateStage())
	assert.Empty(t, state.GetActiveMoongatePositions())
	assert.Equal(t, before, state.DateTime)
	assert.Zero(t, state.GetRNG().Calls())
	assert.Equal(t, []int{2}, callbacks.TimeAdvanced())
	assert.Equal(t, beforeJournal, state.CurrentQuestJournalDossier().Moonstones, "failed activation earns no observation")
}
