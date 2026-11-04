package main

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestDebugHoleUpRejectsStaleRosterAndQueuesWithoutPreStartSceneMutation(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	state.SystemCallbacks.Screen.HideMessageDialog = func() {}
	scene := &GameScene{gameState: state}
	request := debugging.Request{CommandID: debugging.CommandTestHoleUp, Arguments: map[string]json.RawMessage{
		"hours": json.RawMessage(`8`), "forceWanderer": json.RawMessage(`true`), "forceAmbush": json.RawMessage(`false`),
		"expectedPartyFingerprint": json.RawMessage(`"stale"`), "guardRosterIndex": json.RawMessage(`0`),
	}}

	stale := scene.ExecuteDebugCommand(request)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_party_roster", stale.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())

	request.Arguments["expectedPartyFingerprint"] = mustDebugJSON(state.CurrentPartyRosterSnapshot().Fingerprint)
	request.Arguments["guardRosterIndex"] = json.RawMessage(`8`)
	staleGuard := scene.ExecuteDebugCommand(request)
	assert.False(t, staleGuard.Success)
	assert.Equal(t, "stale_guard_membership", staleGuard.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())

	request.Arguments["guardRosterIndex"] = json.RawMessage(`0`)
	queued := scene.ExecuteDebugCommand(request)
	require.True(t, queued.Success, queued.ErrorMessage)
	assert.Equal(t, debugging.ExecutionWorkflow, queued.EffectiveExecutionMode)
	assert.True(t, state.EventQueueManager.HasWorkflow("camping_debug_workflow"))
	assert.Nil(t, state.CampingScene, "camping scene must not mutate before the reserved workflow executes")
}

func TestDebugHoleUpValidatesCallbacksBeforeQueueMutation(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	state.SystemCallbacks.Screen.CreateAndPushMessageDialog = nil
	scene := &GameScene{gameState: state}
	result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestHoleUp, Arguments: map[string]json.RawMessage{
		"hours": json.RawMessage(`8`), "forceWanderer": json.RawMessage(`false`), "forceAmbush": json.RawMessage(`false`),
		"expectedPartyFingerprint": mustDebugJSON(state.CurrentPartyRosterSnapshot().Fingerprint),
	}})

	assert.False(t, result.Success)
	assert.Equal(t, "callbacks_unavailable", result.ErrorCode)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestDebugSearchQueuesMapAwareProductionAction(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state}
	result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestSearch, Arguments: map[string]json.RawMessage{
		"direction": json.RawMessage(`"right"`),
	}})

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, debugging.ExecutionWorkflow, result.EffectiveExecutionMode)
	var data searchWorkflowResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "large", data.MapType)
	assert.Equal(t, "right", data.Direction)
	assert.Equal(t, "queued", data.StartOutcome)
	assert.True(t, data.Queued)
}

func TestDebugMoongateTravelUsesReviewedRouteAndDefersEffects(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(224, 133).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 21
	state.DateTime.Day = 1
	stabilizeMoongateStage(t, state)
	scene := &GameScene{gameState: state}
	route, err := state.GetMoongateTravelRoute()
	require.NoError(t, err)
	callbacks.Reset()
	request := debugging.Request{CommandID: debugging.CommandMoongateTravel, Arguments: map[string]json.RawMessage{
		"expectedSourceFingerprint": mustDebugJSON(moongateSourceFingerprint(state)),
		"expectedRouteFingerprint":  mustDebugJSON(moongateRouteFingerprint(route)),
	}}

	result := scene.ExecuteDebugCommand(request)
	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, debugging.ExecutionWorkflow, result.EffectiveExecutionMode)
	assert.Empty(t, callbacks.TimeAdvanced(), "time must not advance before the workflow starts")
	assert.Empty(t, callbacks.SoundEffectsPlayed, "audio must not play before the workflow starts")
	assert.True(t, state.EventQueueManager.HasWorkflow(game_state.MoongateTeleportWorkflow))

	state.EventQueueManager.Update()
	assert.Empty(t, callbacks.TimeAdvanced(), "moongate activation has no intrinsic minute (ref:MOON-0005)")
}

func TestDebugMoongateTravelHandsOffSpiritualityRouteAndRestoresSourceOnCancel(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	state := scene.gameState
	stone := state.GetMoonstoneSystem().GetMoonstoneByIndex(0)
	require.NotNil(t, stone)
	state.MapState.PlayerLocation.Position = references.Position{
		X: references.Coordinate(stone.X), Y: references.Coordinate(stone.Y),
	}
	source := state.MapState.PlayerLocation
	state.DateTime = datetime.UltimaDate{Year: 1, Month: 1, Day: 1, Hour: 0, Minute: 9}
	stabilizeMoongateStage(t, state)
	route, err := state.GetMoongateTravelRoute()
	require.NoError(t, err)
	callbacks.Reset()
	state.SystemCallbacks.Visual.StabilizeTeleportationDestinationRenderer = func(game_state.MoongateDestination) {
		t.Error("Workbench Spirituality travel must not stabilize an ordinary destination renderer")
	}
	request := debugging.Request{CommandID: debugging.CommandMoongateTravel, Arguments: map[string]json.RawMessage{
		"expectedSourceFingerprint": mustDebugJSON(moongateSourceFingerprint(state)),
		"expectedRouteFingerprint":  mustDebugJSON(moongateRouteFingerprint(route)),
	}}

	result := scene.ExecuteDebugCommand(request)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.True(t, state.EventQueueManager.HasWorkflow(game_state.MoongateTeleportWorkflow))
	var data moongateTravelWorkflowResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "spirituality", data.Route.Kind)
	assert.Nil(t, data.Route.Destination)
	require.NotNil(t, data.Route.ShrineIndex)
	assert.Equal(t, game_state.SpiritualityShrineIndex, *data.Route.ShrineIndex)
	assert.Nil(t, state.ShrineScene)
	assert.Equal(t, source, state.MapState.PlayerLocation)

	// Execution must use the accepted semantic route even after the window closes.
	state.DateTime.Minute = 10
	callbacks.BindGameState(state)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	require.NotNil(t, state.ShrineScene)
	assert.Equal(t, references.ShrineLocation, state.MapState.PlayerLocation.Location)
	assert.Equal(t, game_state.SpiritualityShrineIndex, state.ShrineScene.ShrineIndex)
	assert.Equal(t, source, state.ShrineScene.OriginalPlayerLocation)
	assert.False(t, state.EventQueueManager.HasWorkflow(game_state.MoongateTeleportWorkflow))
	assert.True(t, callbacks.InputBlocked)
	assert.Empty(t, callbacks.TimeAdvanced())

	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	transcript.Update()
	require.True(t, transcript.InputComplete())
	drainSceneWorkflow(t, scene, shrineMeditationWorkflowName, 1000)

	assert.Equal(t, source, state.MapState.PlayerLocation)
	assert.Nil(t, state.ShrineScene)
	assert.Nil(t, scene.miniMapScene)
	assert.False(t, scene.dialogStack.HasOpenDialog())
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.NotContains(t, transcript.TranscriptText(), "unfocused")
	assert.Equal(t, 2, callbacks.BlockInputCalls)
	assert.Equal(t, 2, callbacks.UnblockInputCalls)
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, []int{game_state.ShrineTimeCostMinutes}, callbacks.TimeAdvanced())
	finishedDate := state.DateTime
	state.EventQueueManager.Update()
	assert.Equal(t, finishedDate, state.DateTime, "cancelled shrine must not charge twice")
	assert.Equal(t, 2, callbacks.UnblockInputCalls)
}

func TestDebugMoongateTravelRejectsStaleSourceWithoutMutation(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state}
	callbacks.Reset()
	result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandMoongateTravel, Arguments: map[string]json.RawMessage{
		"expectedSourceFingerprint": json.RawMessage(`"stale"`),
		"expectedRouteFingerprint":  json.RawMessage(`"stale"`),
	}})

	assert.False(t, result.Success)
	assert.Equal(t, "stale_moongate_source", result.ErrorCode)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestDebugMoongateTravelRejectsStaleSpiritualityRouteAtBoundary(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(224, 133).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 0
	state.DateTime.Minute = 9
	state.DateTime.Day = 1
	stabilizeMoongateStage(t, state)
	reviewedRoute, err := state.GetMoongateTravelRoute()
	require.NoError(t, err)
	request := debugging.Request{CommandID: debugging.CommandMoongateTravel, Arguments: map[string]json.RawMessage{
		"expectedSourceFingerprint": mustDebugJSON(moongateSourceFingerprint(state)),
		"expectedRouteFingerprint":  mustDebugJSON(moongateRouteFingerprint(reviewedRoute)),
	}}
	state.DateTime.Minute = 10
	beforeLocation := state.MapState.PlayerLocation
	beforeTime := state.DateTime
	callbacks.Reset()
	scene := &GameScene{gameState: state}

	result := scene.ExecuteDebugCommand(request)

	assert.False(t, result.Success)
	assert.Equal(t, "stale_moongate_route", result.ErrorCode)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Equal(t, beforeTime, state.DateTime)
	assert.Empty(t, callbacks.SoundEffectsPlayed)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestDebugMoongateTravelRequiresDisplayRefreshBeforeQueueMutation(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(224, 133).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 21
	state.DateTime.Day = 1
	stabilizeMoongateStage(t, state)
	route, err := state.GetMoongateTravelRoute()
	require.NoError(t, err)
	request := debugging.Request{CommandID: debugging.CommandMoongateTravel, Arguments: map[string]json.RawMessage{
		"expectedSourceFingerprint": mustDebugJSON(moongateSourceFingerprint(state)),
		"expectedRouteFingerprint":  mustDebugJSON(moongateRouteFingerprint(route)),
	}}
	beforeLocation := state.MapState.PlayerLocation
	beforeTime := state.DateTime
	callbacks.Reset()
	state.SystemCallbacks.Screen.UpdateDisplay = nil
	scene := &GameScene{gameState: state}

	result := scene.ExecuteDebugCommand(request)

	assert.False(t, result.Success)
	assert.Equal(t, "callbacks_unavailable", result.ErrorCode)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Equal(t, beforeTime, state.DateTime)
	assert.Empty(t, callbacks.SoundEffectsPlayed)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func stabilizeMoongateStage(t *testing.T, state *game_state.GameState) {
	t.Helper()
	state.UpdateMoongateTransition()
	for range 20 {
		if !state.IsMoongateTransitionActive() {
			return
		}
		state.ElapsedMs += 200
		state.EventQueueManager.Update()
	}
	require.False(t, state.IsMoongateTransitionActive())
}
