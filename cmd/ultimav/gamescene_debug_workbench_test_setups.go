package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

type testSetupStatusResult struct {
	PartyFingerprint       string                      `json:"partyFingerprint"`
	WipeReview             activePartyWipeReviewResult `json:"wipeReview"`
	LocationID             int                         `json:"locationId"`
	LocationName           string                      `json:"locationName"`
	MapType                string                      `json:"mapType"`
	Floor                  int                         `json:"floor"`
	Date                   string                      `json:"date"`
	Time                   string                      `json:"time"`
	Food                   uint16                      `json:"food"`
	CombatActive           bool                        `json:"combatActive"`
	DialogActive           bool                        `json:"dialogActive"`
	GameplayWorkflowActive bool                        `json:"gameplayWorkflowActive"`
	ScriptedSceneActive    bool                        `json:"scriptedSceneActive"`
	GameplayTerminal       bool                        `json:"gameplayTerminal"`
	FinaleActive           bool                        `json:"finaleActive"`
	AllPartyDead           bool                        `json:"allPartyDead"`
	Party                  []activeCharacterResult     `json:"party"`
}

func (g *GameScene) executeTestSetupDebugCommand(request debugging.Request) (debugging.Result, bool) {
	if request.CommandID != debugging.CommandTestSetupStatus {
		return debugging.Result{}, false
	}

	return debugSuccessResult(request, "Test setup status loaded.", newTestSetupStatusResult(g)), true
}

func newTestSetupStatusResult(scene *GameScene) testSetupStatusResult {
	state := scene.gameState
	location := state.MapState.PlayerLocation
	characters := state.ActivePartyCharacters()
	party := make([]activeCharacterResult, 0, len(characters))
	for _, character := range characters {
		party = append(party, newActiveCharacterResult(character))
	}

	return testSetupStatusResult{
		PartyFingerprint: state.CurrentPartyRosterSnapshot().Fingerprint,
		WipeReview:       newActivePartyWipeReviewResult(state.ReviewActivePartyWipe()),
		LocationID:       int(location.Location), LocationName: location.Location.String(),
		MapType: debugMapTypeName(location.Location.GetMapType()), Floor: int(location.Floor),
		Date: state.DateTime.GetDateAsString(), Time: state.DateTime.GetTimeAsString(),
		Food: state.PartyState.Inventory.Provisions.Food.Get(), CombatActive: state.IsInCombat(),
		DialogActive: scene.dialogStack.HasOpenDialog(), GameplayWorkflowActive: state.EventQueueManager.HasActiveWorkflows(),
		ScriptedSceneActive: scene.miniMapScene != nil || state.ShrineScene != nil,
		GameplayTerminal:    state.IsGameplayTerminal(),
		FinaleActive:        !state.CanAcceptGameplayCommands(),
		AllPartyDead:        state.PartyState.IsAllPartyDead(), Party: party,
	}
}
