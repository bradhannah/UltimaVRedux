package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestCombatEncounterOptionsReturnStableSpawnableReferences(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).Build()

	result := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "options", CommandID: debugging.CommandCombatEncounterOptions,
	}, state)

	require.True(t, result.Success, result.ErrorMessage)
	var options combatEncounterOptionsResult
	require.NoError(t, json.Unmarshal(result.Data, &options))
	assert.Len(t, options.BritanniaMaps, references.TotalCombatMaps)
	assert.Len(t, options.BritanniaMaps[0].Preview.Tiles, int(references.XCombatMapTiles*references.YCombatMapTiles))
	assert.Len(t, options.Dungeons, references.MaxDungeonCombatIndex+1)
	assert.Len(t, options.Dungeons[0].Rooms, references.MaxDungeonRooms)
	graveRoom := options.Dungeons[references.CovetousDungeon].Rooms[12]
	require.NotNil(t, graveRoom.Floor)
	assert.Equal(t, 5, *graveRoom.Floor)
	assert.Equal(t, &debugPositionResult{X: 3, Y: 4}, graveRoom.Cell)
	assert.NotEmpty(t, graveRoom.Preview.Entities)
	assert.Len(t, graveRoom.Preview.Party["west"], references.MaxPlayersPerMap)
	assert.NotEmpty(t, options.Monsters)
	orc := findEncounterMonster(options.Monsters, references.MonsterOrc.Index())
	require.NotNil(t, orc)
	assert.Equal(t, "Orc", orc.Name)
	assert.Positive(t, orc.MaxGroup)
	require.NotNil(t, orc.Friend)
	assert.Equal(t, references.MonsterTroll.Index(), orc.Friend.ID)
}

func TestCombatEncounterStartValidatesConditionalArgumentsAndTranslatesActiveCombat(t *testing.T) {
	state := newApprovedToolTestState()
	missingMap := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "missing-map", CommandID: debugging.CommandCombatEncounterStart,
		Arguments: map[string]json.RawMessage{
			"territory": json.RawMessage(`"britannia"`), "entryDirection": json.RawMessage(`"north"`),
			"monsterType": json.RawMessage(`"32"`), "countMode": json.RawMessage(`"automatic"`), "friends": json.RawMessage(`true`),
		},
	}, state)
	assert.False(t, missingMap.Success)
	assert.Equal(t, "invalid_combat_encounter", missingMap.ErrorCode)

	authoredWithMonster := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "authored-monster", CommandID: debugging.CommandCombatEncounterStart,
		Arguments: map[string]json.RawMessage{
			"territory": json.RawMessage(`"dungeon"`), "dungeon": json.RawMessage(`3`), "dungeonRoom": json.RawMessage(`12`),
			"entryDirection": json.RawMessage(`"west"`), "contents": json.RawMessage(`"authored"`),
			"monsterType": json.RawMessage(`"32"`),
		},
	}, state)
	assert.False(t, authoredWithMonster.Success)
	assert.Equal(t, "invalid_combat_encounter", authoredWithMonster.ErrorCode)

	state.CombatScene = &game_state.CombatState{}
	active := executeSecondBatchStateDebugCommand(validEncounterDebugRequest(), state)
	assert.False(t, active.Success)
	assert.Equal(t, "combat_encounter_active", active.ErrorCode)
}

func TestCombatEncounterStartReturnsResolvedComposition(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).Build()

	result := executeSecondBatchStateDebugCommand(validEncounterDebugRequest(), state)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, debugging.ExecutionSynchronized, result.EffectiveExecutionMode)
	var data combatEncounterStartResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "britannia", data.Territory)
	assert.Equal(t, int(references.GladeCombatMap), data.MapIndex)
	assert.Equal(t, 4, data.TotalCount)
	assert.Len(t, data.Positions, 4)
	assert.NotNil(t, state.CombatScene)
}

func TestMonsterGenerationAndUnsupportedClearReturnStructuredResults(t *testing.T) {
	state := newApprovedToolTestState()
	state.TheOdds = references.NewDefaultTheOdds()

	set := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "generation", CommandID: debugging.CommandMonsterGenerationSet,
		Arguments: map[string]json.RawMessage{"enabled": json.RawMessage(`true`), "odds": json.RawMessage(`64`)},
	}, state)
	require.True(t, set.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, set.EffectiveExecutionMode)
	assert.Len(t, set.Changes, 2)

	clear := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "clear", CommandID: debugging.CommandMonsterClearMapEnemies,
		Arguments: map[string]json.RawMessage{
			"expectedLocationId": json.RawMessage(`0`),
			"expectedFloor":      json.RawMessage(`0`),
			"expectedEnemyCount": json.RawMessage(`0`),
		},
	}, state)
	require.True(t, clear.Success)
	var clearData monsterClearResult
	require.NoError(t, json.Unmarshal(clear.Data, &clearData))
	assert.False(t, clearData.Supported)
	assert.Zero(t, clearData.Removed)
	assert.NotEmpty(t, clearData.UnsupportedReason)
}

func TestMonsterClearRejectsStaleReviewedState(t *testing.T) {
	state := newApprovedToolTestState()
	state.TheOdds = references.NewDefaultTheOdds()

	result := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "clear", CommandID: debugging.CommandMonsterClearMapEnemies,
		Arguments: map[string]json.RawMessage{
			"expectedLocationId": json.RawMessage(`99`),
			"expectedFloor":      json.RawMessage(`0`),
			"expectedEnemyCount": json.RawMessage(`0`),
		},
	}, state)

	assert.False(t, result.Success)
	assert.Equal(t, "stale_monster_ecosystem", result.ErrorCode)
}

func TestMonsterSpawnersReturnAccurateRejection(t *testing.T) {
	state := newApprovedToolTestState()
	state.TheOdds = references.NewDefaultTheOdds()
	state.MapState.PlayerLocation = references.PlayerLocation{Location: references.Britain}

	for _, commandID := range []string{debugging.CommandMonsterSpawnWhirlpool, debugging.CommandMonsterSpawnPirate} {
		result := executeSecondBatchStateDebugCommand(debugging.Request{SessionID: "test", ID: commandID, CommandID: commandID}, state)
		require.True(t, result.Success)
		var data monsterSpawnResult
		require.NoError(t, json.Unmarshal(result.Data, &data))
		assert.False(t, data.Spawned)
		assert.Equal(t, "current map is not a Britannia large map", data.RejectionReason)
		assert.Nil(t, data.Position)
	}
}

func TestVehicleCommandsUseNonSinkingHullAuthority(t *testing.T) {
	state := newApprovedToolTestState()
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, references.Position{}, 0)
	state.PartyVehicle.GetVehicleDetails().SetHullHP(50)

	damage := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "damage", CommandID: debugging.CommandShipDamageRaw,
		Arguments: map[string]json.RawMessage{"amount": json.RawMessage(`17`)},
	}, state)
	require.True(t, damage.Success)
	assert.Equal(t, 33, state.PartyVehicle.GetVehicleDetails().GetHullHP())
	require.Len(t, damage.Changes, 1)

	rejected := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "sink", CommandID: debugging.CommandShipDamageRaw,
		Arguments: map[string]json.RawMessage{"amount": json.RawMessage(`33`)},
	}, state)
	assert.False(t, rejected.Success)
	assert.Equal(t, "invalid_ship_hull_operation", rejected.ErrorCode)
	assert.Equal(t, 33, state.PartyVehicle.GetVehicleDetails().GetHullHP())

	repair := executeSecondBatchStateDebugCommand(debugging.Request{SessionID: "test", ID: "repair", CommandID: debugging.CommandShipRepairFull}, state)
	require.True(t, repair.Success)
	assert.Equal(t, 99, state.PartyVehicle.GetVehicleDetails().GetHullHP())
	var status vehicleStatusResult
	require.NoError(t, json.Unmarshal(repair.Data, &status))
	assert.Equal(t, "frigate", status.VehicleName)
}

func TestVehicleSpawnAndForceBoardCommandsReturnStructuredChangesAndStableErrors(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).Build()
	state.MapState.PlayerLocation.Position = references.Position{X: 200, Y: 200}

	spawn := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "spawn", CommandID: debugging.CommandVehicleSpawn,
		Arguments: map[string]json.RawMessage{
			"type": json.RawMessage(`"skiff"`), "placement": json.RawMessage(`"avatar"`),
		},
	}, state)
	require.True(t, spawn.Success, spawn.ErrorMessage)
	assert.Equal(t, debugging.ExecutionSynchronized, spawn.EffectiveExecutionMode)
	assert.Equal(t, []string{"vehicle.map.unitCount", "vehicle.map.targetVehicleType"}, stateChangePaths(spawn.Changes))
	var spawnData vehicleSpawnResult
	require.NoError(t, json.Unmarshal(spawn.Data, &spawnData))
	assert.Equal(t, "skiff", spawnData.VehicleName)
	assert.Equal(t, "avatar", spawnData.Placement)
	assert.Equal(t, debugPositionResult{X: 200, Y: 200}, spawnData.Position)

	board := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "board", CommandID: debugging.CommandVehicleForceBoard,
		Arguments: map[string]json.RawMessage{"type": json.RawMessage(`"horse"`)},
	}, state)
	require.True(t, board.Success, board.ErrorMessage)
	assert.Contains(t, stateChangePaths(board.Changes), "vehicle.type")
	var boardData vehicleForceBoardResult
	require.NoError(t, json.Unmarshal(board.Data, &boardData))
	assert.True(t, boardData.TerrainBypassed)
	assert.Equal(t, "horse", boardData.VehicleStatus.VehicleName)

	missingDock := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "dock", CommandID: debugging.CommandVehicleSpawn,
		Arguments: map[string]json.RawMessage{
			"type": json.RawMessage(`"frigate"`), "placement": json.RawMessage(`"dock"`),
		},
	}, state)
	assert.False(t, missingDock.Success)
	assert.Equal(t, "vehicle_dock_required", missingDock.ErrorCode)

	state.CurrentNPCAIController = nil
	unsupported := executeSecondBatchStateDebugCommand(debugging.Request{
		SessionID: "test", ID: "unsupported", CommandID: debugging.CommandVehicleForceBoard,
		Arguments: map[string]json.RawMessage{"type": json.RawMessage(`"skiff"`)},
	}, state)
	assert.False(t, unsupported.Success)
	assert.Equal(t, "unsupported_vehicle_context", unsupported.ErrorCode)
}

func validEncounterDebugRequest() debugging.Request {
	return debugging.Request{
		SessionID: "test", ID: "encounter", CommandID: debugging.CommandCombatEncounterStart,
		Arguments: map[string]json.RawMessage{
			"territory": json.RawMessage(`"britannia"`), "britanniaMap": json.RawMessage(`2`),
			"entryDirection": json.RawMessage(`"north"`), "monsterType": json.RawMessage(`"32"`),
			"countMode": json.RawMessage(`"exact"`), "exactCount": json.RawMessage(`4`),
			"friends": json.RawMessage(`true`), "seed": json.RawMessage(`1`),
		},
	}
}

func findEncounterMonster(monsters []combatEncounterMonsterResult, id int) *combatEncounterMonsterResult {
	for index := range monsters {
		if monsters[index].ID == id {
			return &monsters[index]
		}
	}
	return nil
}
