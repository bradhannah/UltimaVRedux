package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
)

func (g *GameScene) executeWorldSetupDebugCommand(request debugging.Request) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandMovementLandmarkTravel:
		if result, blocked := g.worldSetupBlockedResult(request); blocked {
			return result, true
		}
		operation, err := g.gameState.DebugTravelToLandmark(mustDebugStringArgument(request, "landmark"))

		return worldSetupOperationResult(request, "Landmark setup loaded.", g.gameState, operation, err), true
	default:
		return debugging.Result{}, false
	}
}

func (g *GameScene) worldSetupBlockedResult(request debugging.Request) (debugging.Result, bool) {
	code, message, blocked := g.worldSetupBlock()
	if blocked {
		return debugging.FailureResult(request, code, message), true
	}

	return debugging.Result{}, false
}

func (g *GameScene) worldSetupBlock() (string, string, bool) {
	if g.dialogStack.HasOpenDialog() {
		return "dialog_active", "world setup is unavailable while a dialog is open", true
	}
	if g.gameState.EventQueueManager.HasActiveWorkflows() {
		return "gameplay_workflow_active", "world setup is unavailable during a gameplay workflow", true
	}
	if g.miniMapScene != nil || g.gameState.ShrineScene != nil {
		return "scripted_scene_active", "world setup is unavailable during a shrine or Codex scene", true
	}

	return "", "", false
}

func worldSetupOperationResult(
	request debugging.Request,
	summary string,
	state *game_state.GameState,
	operation game_state.WorldSetupResult,
	err error,
) debugging.Result {
	if err != nil {
		return debugging.FailureResult(request, "invalid_world_setup", err.Error())
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		summary,
		newWorldPositionResult(state),
	)
	appendWorldSetupChanges(&result.Changes, operation)

	return result
}

func appendWorldSetupChanges(changes *[]debugging.StateChange, operation game_state.WorldSetupResult) {
	appendSemanticChange(changes, "world.locationId", int(operation.Before.Location), int(operation.After.Location))
	appendSemanticChange(changes, "world.floor", int(operation.Before.Floor), int(operation.After.Floor))
	appendSemanticChange(changes, "world.position.x", int(operation.Before.Position.X), int(operation.After.Position.X))
	appendSemanticChange(changes, "world.position.y", int(operation.Before.Position.Y), int(operation.After.Position.Y))
	appendSemanticChange(changes, "effects.smashed", operation.SmashedBefore, operation.SmashedAfter)
}
