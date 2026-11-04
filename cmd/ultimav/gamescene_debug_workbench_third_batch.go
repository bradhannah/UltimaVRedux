package main

import (
	"errors"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	errSearchFeedbackCallbacks  = errors.New("search feedback callbacks are unavailable")
	errSearchGameplayCallbacks  = errors.New("search gameplay callbacks are unavailable")
	errCombatSearchCallbacks    = errors.New("combat search callbacks are unavailable")
	errMoongateTravelCallbacks  = errors.New("moongate travel callbacks are unavailable")
	errCampingCallbacks         = errors.New("camping callbacks are unavailable")
	errCampingWorkflowCallbacks = errors.New("camping workflow callbacks are unavailable")
)

type queuedWorkflowResult struct {
	Workflow string `json:"workflow"`
	Queued   bool   `json:"queued"`
}

type holeUpWorkflowResult struct {
	queuedWorkflowResult
	Hours         int  `json:"hours"`
	ForceWanderer bool `json:"forceWanderer"`
	ForceAmbush   bool `json:"forceAmbush"`
	GuardIndex    *int `json:"guardRosterIndex,omitempty"`
}

type searchWorkflowResult struct {
	queuedWorkflowResult
	MapType      string `json:"mapType"`
	Direction    string `json:"direction"`
	StartOutcome string `json:"startOutcome"`
}

type moongateTravelWorkflowResult struct {
	queuedWorkflowResult
	Route moongateRouteResult `json:"route"`
}

func (g *GameScene) executeThirdBatchDebugCommand(request debugging.Request) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandTestHoleUp:
		return g.executeDebugHoleUp(request), true
	case debugging.CommandTestSearch:
		return g.executeDebugSearch(request), true
	case debugging.CommandMoongateTravel:
		return g.executeDebugMoongateTravel(request), true
	default:
		return debugging.Result{}, false
	}
}

func (g *GameScene) thirdBatchWorkflowBlocked(request debugging.Request) *debugging.Result {
	if g.dialogStack.HasOpenDialog() {
		result := debugging.FailureResult(
			request, "dialog_active", "command workflow is unavailable while a modal dialog is open",
		)

		return &result
	}
	if g.gameState.EventQueueManager == nil {
		result := debugging.FailureResult(request, "event_queue_unavailable", "gameplay event queue is unavailable")

		return &result
	}
	if g.gameState.EventQueueManager.HasActiveWorkflows() {
		result := debugging.FailureResult(
			request, "gameplay_workflow_active", "command workflow is unavailable during another gameplay workflow",
		)

		return &result
	}

	return nil
}

func (g *GameScene) executeDebugHoleUp(request debugging.Request) debugging.Result {
	if blocked := g.thirdBatchWorkflowBlocked(request); blocked != nil {
		return *blocked
	}
	if g.gameState.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return debugging.FailureResult(request, "unsupported_map_type", "hole up testing requires the Britannia large map")
	}
	if err := validateCampingCallbacks(g.gameState); err != nil {
		return debugging.FailureResult(request, "callbacks_unavailable", err.Error())
	}
	roster := g.gameState.CurrentPartyRosterSnapshot()
	if mustDebugStringArgument(request, "expectedPartyFingerprint") != roster.Fingerprint {
		return debugging.FailureResult(request, "stale_party_roster", "active party changed after camping review")
	}
	guardIndex := game_state.NoGuardAssigned
	var guardResult *int
	if requestedGuard, supplied, err := optionalDebugIntegerArgument(request, "guardRosterIndex"); err != nil {
		return debugging.FailureResult(request, "invalid_guard", err.Error())
	} else if supplied {
		if requestedGuard >= len(roster.Characters) ||
			!g.gameState.PartyState.GetCharacter(requestedGuard).IsInParty() {
			return debugging.FailureResult(
				request, "stale_guard_membership", "selected guard is no longer an active party member",
			)
		}
		guardIndex = requestedGuard
		guardResult = &requestedGuard
	}
	hours := mustDebugIntegerArgument(request, "hours")
	forceWanderer := mustDebugBooleanArgument(request, "forceWanderer")
	forceAmbush := mustDebugBooleanArgument(request, "forceAmbush")
	if !g.gameState.ActionHoleUpLargeMapWithDebug(hours, guardIndex, forceWanderer, forceAmbush) {
		return debugging.FailureResult(request, "hole_up_not_started", "production hole-up preconditions rejected the request")
	}

	return debugSuccessResultWithMode(request, debugging.ExecutionWorkflow, "Camping workflow queued.", holeUpWorkflowResult{
		queuedWorkflowResult: queuedWorkflowResult{Workflow: "camping_debug_workflow", Queued: true},
		Hours:                hours, ForceWanderer: forceWanderer, ForceAmbush: forceAmbush, GuardIndex: guardResult,
	})
}

func (g *GameScene) executeDebugSearch(request debugging.Request) debugging.Result {
	if blocked := g.thirdBatchWorkflowBlocked(request); blocked != nil {
		return *blocked
	}
	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	if err := validateSearchCallbacks(g.gameState, mapType); err != nil {
		return debugging.FailureResult(request, "callbacks_unavailable", err.Error())
	}
	directionName := mustDebugStringArgument(request, "direction")
	direction, ok := debugSearchDirection(directionName)
	if !ok {
		return debugging.FailureResult(request, "invalid_direction", "unsupported search direction")
	}
	workflowName := "debug_workbench.command_search"
	g.gameState.EventQueueManager.Workflow(workflowName).
		Priority(event_queue.PriorityMedium).
		Sequential().BlockInput().
		First(func() { dispatchDebugSearch(g.gameState, mapType, direction) }).
		Finally(func() {}).
		Start()

	return debugSuccessResultWithMode(request, debugging.ExecutionWorkflow, "Search workflow queued.", searchWorkflowResult{
		queuedWorkflowResult: queuedWorkflowResult{Workflow: workflowName, Queued: true},
		MapType:              debugMapTypeName(mapType), Direction: directionName, StartOutcome: "queued",
	})
}

func (g *GameScene) executeDebugMoongateTravel(request debugging.Request) debugging.Result {
	if blocked := g.thirdBatchWorkflowBlocked(request); blocked != nil {
		return *blocked
	}
	if g.gameState.IsTeleportationActive() {
		return debugging.FailureResult(request, "teleportation_active", "moongate teleportation is already active")
	}
	if err := validateMoongateCallbacks(g.gameState); err != nil {
		return debugging.FailureResult(request, "callbacks_unavailable", err.Error())
	}
	if mustDebugStringArgument(request, "expectedSourceFingerprint") != moongateSourceFingerprint(g.gameState) {
		return debugging.FailureResult(request, "stale_moongate_source", "moongate source readiness changed after review")
	}
	travel := g.gameState.ReviewMoongateTravel()
	if !travel.Success {
		return debugging.FailureResult(
			request, "moongate_source_unavailable", "player is not ready to travel through a resolved active moongate",
		)
	}
	route, err := g.gameState.GetMoongateTravelRoute()
	if err != nil || route != travel.Route {
		return debugging.FailureResult(request, "stale_moongate_route", "moongate route changed after review")
	}
	fingerprint := moongateRouteFingerprint(route)
	if mustDebugStringArgument(request, "expectedRouteFingerprint") != fingerprint {
		return debugging.FailureResult(request, "stale_moongate_route", "moongate route changed after review")
	}
	workflowName := game_state.MoongateTeleportWorkflow
	g.queueReviewedMoongateTravel(travel)

	return debugSuccessResultWithMode(
		request, debugging.ExecutionWorkflow, "Moongate travel workflow queued.", moongateTravelWorkflowResult{
			queuedWorkflowResult: queuedWorkflowResult{Workflow: workflowName, Queued: true},
			Route:                debugMoongateRoute(route),
		},
	)
}

func debugSearchDirection(value string) (references.Direction, bool) {
	switch value {
	case "up":
		return references.Up, true
	case "down":
		return references.Down, true
	case "left":
		return references.Left, true
	case "right":
		return references.Right, true
	default:
		return references.NoneDirection, false
	}
}

func dispatchDebugSearch(state *game_state.GameState, mapType references.GeneralMapType, direction references.Direction) {
	switch mapType {
	case references.SmallMapType:
		state.ActionSearchSmallMap(direction)
	case references.LargeMapType:
		state.ActionSearchLargeMap(direction)
	case references.DungeonMapType:
		token, _ := state.BeginDungeonCommandCycle()
		state.ActionSearchDungeonMap(direction)
		state.CompleteDungeonCommandCycle(token, game_state.DungeonFullEndPlayer)
	case references.CombatMapType:
		state.ActionSearchCombatMap(direction)
	case references.CampingMapType:
		panic("validated search map type became unsupported")
	default:
		panic("validated search map type became unsupported")
	}
}

func validateSearchCallbacks(state *game_state.GameState, mapType references.GeneralMapType) error {
	if state.SystemCallbacks == nil || state.SystemCallbacks.UI.ShowActionFeedback == nil ||
		state.SystemCallbacks.Message.AddRowStr == nil {
		return errSearchFeedbackCallbacks
	}
	if mapType != references.CombatMapType {
		if state.SearchManager == nil ||
			state.SystemCallbacks.Screen.UpdateDisplay == nil || state.SystemCallbacks.Audio.Play == nil {
			return errSearchGameplayCallbacks
		}

		return nil
	}
	if state.SystemCallbacks.Audio.PlaySoundEffect == nil {
		return errCombatSearchCallbacks
	}

	return nil
}

func validateMoongateCallbacks(state *game_state.GameState) error {
	if state.SystemCallbacks == nil || state.SystemCallbacks.Audio.Play == nil ||
		state.SystemCallbacks.Audio.PlaySoundEffect == nil || state.SystemCallbacks.Message.AddRowStr == nil ||
		state.SystemCallbacks.Screen.UpdateDisplay == nil ||
		state.SystemCallbacks.Visual.InitializeAvatarDissolveEffect == nil || state.SystemCallbacks.Visual.SetAvatarDissolveLevel == nil ||
		state.SystemCallbacks.Visual.SetMapFadeLevel == nil || state.SystemCallbacks.Visual.CleanupAvatarDissolveEffect == nil ||
		state.SystemCallbacks.Visual.ResetTeleportationSourceRenderer == nil ||
		state.SystemCallbacks.Visual.StabilizeTeleportationDestinationRenderer == nil {
		return errMoongateTravelCallbacks
	}

	return nil
}

func validateCampingCallbacks(state *game_state.GameState) error {
	if state.SystemCallbacks == nil {
		return errCampingCallbacks
	}
	callbacks := state.SystemCallbacks
	if callbacks.Message.AddRowStr == nil || callbacks.Message.AppendStyledMessage == nil ||
		callbacks.Audio.Play == nil || callbacks.Audio.Stop == nil || callbacks.Audio.PlaySoundEffect == nil ||
		callbacks.Audio.PauseBackgroundMusic == nil || callbacks.Audio.ResumeBackgroundMusic == nil ||
		callbacks.Screen.CreateAndPushMessageDialog == nil || callbacks.Screen.PopMessageDialog == nil ||
		callbacks.Screen.CreateAndPushCampingResultDialog == nil ||
		callbacks.Screen.HideMessageDialog == nil || callbacks.Screen.ClearMessageDialog == nil ||
		callbacks.Screen.SetMessagePrompt == nil || callbacks.Screen.ClearMessagePrompt == nil ||
		callbacks.Screen.SetMessageDialogFadeDurations == nil || callbacks.Screen.GetMessageDialogFadeInDuration == nil ||
		callbacks.Screen.GetMessageDialogFadeOutDuration == nil || callbacks.Screen.ShowPressAnyKeyPrompt == nil ||
		callbacks.Screen.HideAllOverlays == nil || callbacks.Screen.ShowAllOverlays == nil ||
		callbacks.Visual.InitializeWandererDissolveEffect == nil || callbacks.Visual.SetWandererDissolveLevel == nil ||
		callbacks.Visual.CleanupWandererDissolveEffect == nil {
		return errCampingWorkflowCallbacks
	}

	return nil
}
