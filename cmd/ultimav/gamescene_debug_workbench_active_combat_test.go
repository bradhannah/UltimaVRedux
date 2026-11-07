package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

type activeCombatFixedRNG struct{}

func (activeCombatFixedRNG) Intn(int) int { return 1 }

func TestActiveCombatQueueStatusIncludesDoomMirrorAbsorption(t *testing.T) {
	assert.Equal(t, "absorbed", activeCombatQueueStatus(game_state.CombatTurnAbsorbed))
	assert.Equal(t, "doom_mirror", activeCombatExitRoute(game_state.CombatExitDoomMirror))
}

func TestActiveCombatStatusReturnsDetachedMapCombatantsAndForecast(t *testing.T) {
	state, monster := newActiveCombatWorkbenchTestState()
	queueBefore, found := state.CombatScene.TurnQueue.GetEntrySnapshot(game_state.CombatTurnIdentity{
		IsPartyMember: false,
		EntityIndex:   0,
	})
	require.True(t, found)

	result, handled := executeActiveCombatDebugCommand(debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandCombatStatus,
	}, state)

	require.True(t, handled)
	require.True(t, result.Success, "%s: %s", result.ErrorCode, result.Summary)
	var status activeCombatResult
	require.NoError(t, json.Unmarshal(result.Data, &status))
	assert.True(t, status.Active)
	assert.Equal(t, "9", status.EncounterID)
	require.NotNil(t, status.Map)
	assert.Len(t, status.Map.Cells, 121)
	require.Len(t, status.Monsters, 1)
	assert.Equal(t, "1", status.Monsters[0].ID)
	assert.Equal(t, monster.CurrentHP(), status.Monsters[0].CurrentHP)
	assert.Equal(t, state.RandomSeed(), status.RNGSeed)
	assert.NotEmpty(t, status.Forecast)
	assert.Equal(t, activeCombatFingerprint(status), status.SnapshotFingerprint)
	queueAfter, found := state.CombatScene.TurnQueue.GetEntrySnapshot(game_state.CombatTurnIdentity{
		IsPartyMember: false,
		EntityIndex:   0,
	})
	require.True(t, found)
	assert.Equal(t, queueBefore, queueAfter)
}

func TestActiveCombatStatusOutsideCombatSucceedsAsUnavailable(t *testing.T) {
	state := newApprovedToolTestState()
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return 0 }, nil)

	result, handled := executeActiveCombatDebugCommand(debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandCombatStatus,
	}, state)

	require.True(t, handled)
	require.True(t, result.Success)
	var status activeCombatResult
	require.NoError(t, json.Unmarshal(result.Data, &status))
	assert.False(t, status.Active)
	assert.NotEmpty(t, status.UnavailableReason)
}

func TestActiveCombatSleepSetRequiresReviewedSnapshotAndStableID(t *testing.T) {
	state, monster := newActiveCombatWorkbenchTestState()
	status := newActiveCombatResult(state)
	request := debugging.Request{
		SessionID: "test", ID: "sleep", CommandID: debugging.CommandCombatMonsterSleepSet,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"9"`),
			"monsterId":        json.RawMessage(`"1"`),
			"asleep":           json.RawMessage(`true`),
			"expectedAsleep":   json.RawMessage(`false`),
			"expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}

	result, handled := executeActiveCombatDebugCommand(request, state)
	require.True(t, handled)
	require.True(t, result.Success)
	assert.True(t, monster.IsAsleep())
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "combat.encounters.9.monsters.1.asleep", result.Changes[0].Path)

	stale, handled := executeActiveCombatDebugCommand(request, state)
	require.True(t, handled)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_combat", stale.ErrorCode)
}

func TestActiveCombatExitDiscardsReviewedEncounterWithoutResolvingOrigin(t *testing.T) {
	state, _ := newActiveCombatWorkbenchTestState()
	state.CombatScene.PreCombatPlayerLocation = references.PlayerLocation{
		Location: references.BritanniaUnderworld,
		Position: references.Position{X: 12, Y: 34},
	}
	status := newActiveCombatResult(state)
	request := debugging.Request{
		SessionID: "test", ID: "exit", CommandID: debugging.CommandCombatExitDebug,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"9"`),
			"expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}

	result, handled := executeActiveCombatDebugCommand(request, state)

	require.True(t, handled)
	require.True(t, result.Success)
	assert.Nil(t, state.CombatScene)
	assert.Equal(t, references.BritanniaUnderworld, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.Position{X: 12, Y: 34}, state.MapState.PlayerLocation.Position)
	var exit activeCombatExitDebugResult
	require.NoError(t, json.Unmarshal(result.Data, &exit))
	assert.Equal(t, "9", exit.EncounterID)
	assert.Equal(t, 1, exit.ActiveMonsterCount)
	assert.Equal(t, "surface", exit.ExitContext)
	require.Len(t, result.Changes, 3)
}

func TestGameSceneRoutesActiveCombatExitCommand(t *testing.T) {
	state, _ := newActiveCombatWorkbenchTestState()
	status := newActiveCombatResult(state)
	scene := &GameScene{gameState: state}

	result := scene.ExecuteDebugCommand(debugging.Request{
		SessionID: "test", ID: "exit", CommandID: debugging.CommandCombatExitDebug,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"9"`),
			"expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	})

	require.True(t, result.Success, "%s: %s", result.ErrorCode, result.Summary)
	assert.Nil(t, state.CombatScene)
}

func TestGameSceneRejectsActiveCombatExitDuringMovement(t *testing.T) {
	state, _ := newActiveCombatWorkbenchTestState()
	status := newActiveCombatResult(state)
	scene := &GameScene{gameState: state}
	scene.combatMovement.Active = true

	result := scene.ExecuteDebugCommand(debugging.Request{
		SessionID: "test", ID: "exit", CommandID: debugging.CommandCombatExitDebug,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"9"`),
			"expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	})

	assert.False(t, result.Success)
	assert.Equal(t, "combat_busy", result.ErrorCode)
	assert.NotNil(t, state.CombatScene)
}

func TestActiveCombatExitRejectsStaleSnapshotWithoutMutation(t *testing.T) {
	state, _ := newActiveCombatWorkbenchTestState()
	request := debugging.Request{
		SessionID: "test", ID: "exit", CommandID: debugging.CommandCombatExitDebug,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"9"`),
			"expectedSnapshot": json.RawMessage(`"stale"`),
		},
	}

	result, handled := executeActiveCombatDebugCommand(request, state)

	require.True(t, handled)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_combat", result.ErrorCode)
	assert.NotNil(t, state.CombatScene)
}

func TestActiveCombatSpawnRejectsOversizedExactBatchAtomically(t *testing.T) {
	state, _ := newActiveCombatWorkbenchTestState()
	// Keep the request within the published limit, but leave only 15 authored positions free.
	state.CombatScene.MonsterManager.GetMonster(0).SetPosition(state.CombatScene.CombatMapRef.MonsterPositions[0])
	status := newActiveCombatResult(state)
	countBefore := state.CombatScene.MonsterManager.Count()
	_, queuedBefore := state.CombatScene.TurnQueue.CountRemaining()
	request := debugging.Request{
		SessionID: "test", ID: "spawn", CommandID: debugging.CommandCombatMonsterSpawn,
		Arguments: map[string]json.RawMessage{
			"encounterId":      json.RawMessage(`"9"`),
			"expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
			"monsterType":      json.RawMessage(`32`),
			"countMode":        json.RawMessage(`"exact"`),
			"exactCount":       json.RawMessage(`16`),
			"friends":          json.RawMessage(`false`),
			"seed":             json.RawMessage(`1234`),
		},
	}

	result, handled := executeActiveCombatDebugCommand(request, state)

	require.True(t, handled)
	assert.False(t, result.Success)
	assert.Equal(t, "combat_spawn_unavailable", result.ErrorCode)
	assert.Equal(t, countBefore, state.CombatScene.MonsterManager.Count())
	_, queuedAfter := state.CombatScene.TurnQueue.CountRemaining()
	assert.Equal(t, queuedBefore, queuedAfter)
}

func TestActiveCombatRNGSeedSetReportsGlobalBeforeAndAfterFromReviewedCombat(t *testing.T) {
	state, _ := newActiveCombatWorkbenchTestState()
	state.SetRandomSeed(41)
	status := newActiveCombatResult(state)
	request := debugging.Request{
		SessionID: "test", ID: "seed", CommandID: debugging.CommandRNGSeedSet,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"9"`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
			"seed": json.RawMessage(`42`),
		},
	}

	result, handled := executeActiveCombatDebugCommand(request, state)

	require.True(t, handled)
	require.True(t, result.Success)
	assert.Equal(t, uint64(42), state.RandomSeed())
	var data activeCombatRNGSeedResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, uint64(41), data.BeforeSeed)
	assert.Equal(t, uint64(42), data.AfterSeed)
	assert.Equal(t, "global_gameplay_rng_including_post_combat", data.Scope)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "gameplay.rng.seed", result.Changes[0].Path)
}

func TestActiveCombatDamageRejectsStaleFingerprintBeforeCoreMutation(t *testing.T) {
	state, monster := newActiveCombatWorkbenchTestState()
	request := debugging.Request{
		SessionID: "test", ID: "damage", CommandID: debugging.CommandCombatMonsterDamageRaw,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"9"`), "monsterId": json.RawMessage(`"1"`),
			"expectedCurrentHp": json.RawMessage(`20`), "expectedSnapshot": json.RawMessage(`"stale"`),
			"damage": json.RawMessage(`7`),
		},
	}

	result, handled := executeActiveCombatDebugCommand(request, state)

	require.True(t, handled)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_combat", result.ErrorCode)
	assert.Equal(t, 20, monster.CurrentHP())
}

func TestActiveCombatDamageAndKillUseStableIDExpectedHPAndNoRewardPolicy(t *testing.T) {
	state, monster := newActiveCombatWorkbenchTestState()
	status := newActiveCombatResult(state)
	damage := debugging.Request{
		SessionID: "test", ID: "damage", CommandID: debugging.CommandCombatMonsterDamageRaw,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"9"`), "monsterId": json.RawMessage(`"1"`),
			"expectedCurrentHp": json.RawMessage(`20`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
			"damage": json.RawMessage(`7`),
		},
	}

	damageResult, handled := executeActiveCombatDebugCommand(damage, state)
	require.True(t, handled)
	require.True(t, damageResult.Success)
	assert.Equal(t, 13, monster.CurrentHP())
	assert.Equal(t, "combat.encounters.9.monsters.1.currentHp", damageResult.Changes[0].Path)

	status = newActiveCombatResult(state)
	kill := debugging.Request{
		SessionID: "test", ID: "kill", CommandID: debugging.CommandCombatMonsterKill,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"9"`), "monsterId": json.RawMessage(`"1"`),
			"expectedCurrentHp": json.RawMessage(`13`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}
	killResult, handled := executeActiveCombatDebugCommand(kill, state)
	require.True(t, handled)
	require.True(t, killResult.Success)
	var data activeCombatMutationResult
	require.NoError(t, json.Unmarshal(killResult.Data, &data))
	assert.Equal(t, "no_xp_no_objects_queue_synchronized_victory_enabled", data.Policy)
	assert.Zero(t, data.ExperienceAwarded)
	assert.Zero(t, data.ObjectsCreated)
	assert.True(t, data.VictoryTriggered)
}

func TestActiveCombatKillAllAndClearReturnDistinctOutcomePolicies(t *testing.T) {
	for _, testCase := range []struct {
		commandID string
		policy    string
		victory   bool
		remaining int
	}{
		{commandID: debugging.CommandCombatMonsterKillAll, policy: "no_xp_no_objects_queue_synchronized_victory_enabled", victory: true, remaining: 1},
		{commandID: debugging.CommandCombatMonsterClear, policy: "outcome_free_no_xp_no_objects_no_victory", victory: false, remaining: 0},
	} {
		t.Run(testCase.commandID, func(t *testing.T) {
			state, _ := newActiveCombatWorkbenchTestState()
			status := newActiveCombatResult(state)
			request := debugging.Request{
				SessionID: "test", ID: testCase.commandID, CommandID: testCase.commandID,
				Arguments: map[string]json.RawMessage{
					"encounterId": json.RawMessage(`"9"`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
				},
			}

			result, handled := executeActiveCombatDebugCommand(request, state)
			require.True(t, handled)
			require.True(t, result.Success)
			var data activeCombatMutationResult
			require.NoError(t, json.Unmarshal(result.Data, &data))
			assert.Equal(t, testCase.policy, data.Policy)
			assert.Equal(t, testCase.victory, state.CombatScene.Victory)
			assert.Equal(t, testCase.remaining, state.CombatScene.MonsterManager.Count())
			_, queued := state.CombatScene.TurnQueue.CountRemaining()
			assert.Zero(t, queued)
		})
	}
}

func TestActiveCombatSleepAllSleepsEveryLiveMonsterUnderTheReviewedSnapshot(t *testing.T) {
	state, monster := newActiveCombatWorkbenchTestState()
	status := newActiveCombatResult(state)
	request := debugging.Request{
		SessionID: "test", ID: "sleep-all", CommandID: debugging.CommandCombatMonsterSleepAll,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"9"`), "expectedSnapshot": mustDebugJSON("stale"),
		},
	}

	stale, handled := executeActiveCombatDebugCommand(request, state)
	require.True(t, handled)
	assert.Equal(t, "stale_combat", stale.ErrorCode)
	assert.False(t, monster.IsAsleep())

	request.Arguments["expectedSnapshot"] = mustDebugJSON(status.SnapshotFingerprint)
	result, handled := executeActiveCombatDebugCommand(request, state)

	require.True(t, handled)
	require.True(t, result.Success, result.ErrorMessage)
	assert.True(t, monster.IsAsleep())
	assert.False(t, state.CombatScene.Victory, "sleeping monsters are still part of the fight")
	require.Len(t, result.Changes, 1)
	assert.Contains(t, result.Changes[0].Path, ".asleep")
	var after activeCombatResult
	require.NoError(t, json.Unmarshal(result.Data, &after))
	require.Len(t, after.Monsters, 1)
	assert.True(t, after.Monsters[0].Asleep)
}

func TestActiveCombatClearRoomRequiresFixedOriginAndDeclaresVictory(t *testing.T) {
	state, monster := newActiveCombatWorkbenchTestState()
	status := newActiveCombatResult(state)
	request := debugging.Request{
		SessionID: "test", ID: "clear-room", CommandID: debugging.CommandCombatDungeonRoomClear,
		Arguments: map[string]json.RawMessage{
			"encounterId": json.RawMessage(`"9"`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}

	rejected, handled := executeActiveCombatDebugCommand(request, state)
	require.True(t, handled)
	assert.False(t, rejected.Success)
	assert.Equal(t, "dungeon_room_combat_required", rejected.ErrorCode)
	assert.Equal(t, 20, monster.CurrentHP())

	state.CombatScene.ExitContext = game_state.CombatExitDungeonRoom
	state.CombatScene.DungeonRoomOrigin = &game_state.DungeonRoomEncounterOrigin{
		Location: references.Deceit, DungeonIndex: references.DeceitDungeon, Room: 10, Live: true,
	}
	status = newActiveCombatResult(state)
	request.Arguments["expectedSnapshot"] = mustDebugJSON(status.SnapshotFingerprint)

	result, handled := executeActiveCombatDebugCommand(request, state)
	require.True(t, handled)
	require.True(t, result.Success)
	var data activeCombatMutationResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, "clear-room", data.Operation)
	assert.True(t, data.VictoryTriggered)
	assert.True(t, state.CombatScene.Victory)
	assert.True(t, monster.IsDead())
	assert.True(t, status.FixedDungeonRoom)
	assert.Equal(t, 10, status.DungeonRoom)
}

func newActiveCombatWorkbenchTestState() (*game_state.GameState, *game_state.CombatMonster) {
	party := party_state.NewBlankPartyState()
	avatar := party.GetAvatar()
	avatar.SetMaxHp(100)
	avatar.SetHp(100)
	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	for index := range combatMap.MonsterPositions {
		combatMap.MonsterPositions[index] = references.Position{X: references.Coordinate(index%8 + 1), Y: references.Coordinate(index/8 + 1)}
	}
	combat := game_state.NewCombatState(combatMap, game_state.CombatEntryNorth, party)
	combat.EncounterID = 9
	reference := references.EnemyReference{
		MonsterType: references.MonsterOrc,
		Dexterity:   12,
		HitPoints:   20,
		MaxPerMap:   6,
		Name:        "Orc",
		NamePlural:  "Orcs",
		EnemyAbilities: map[references.EnemyAbility]bool{
			references.Poison: true,
		},
	}
	monster := game_state.NewCombatMonster(
		references.MonsterOrc,
		&reference,
		references.Position{X: 4, Y: 4},
		activeCombatFixedRNG{},
	)
	combat.AddMonsterToQueue(monster)
	enemyReferences := make(references.EnemyReferences, references.TotalMonsterTypes)
	enemyReferences[references.MonsterOrc] = reference
	state := &game_state.GameState{
		PartyState:     *party,
		CombatScene:    combat,
		GameReferences: &references.GameReferences{EnemyReferences: &enemyReferences},
		SystemCallbacks: &game_state.SystemCallbacks{
			Combat: game_state.NewCombatCallbacks(
				func(game_state.CombatEvent) {},
				func(game_state.CombatConclusion) {},
			),
			Screen: game_state.ScreenCallbacks{
				UpdateDisplay: func() {},
			},
		},
	}
	state.SetRandomSeed(1)
	state.MapState.PlayerLocation.Location = references.CombatLocation
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return 0 }, nil)

	return state, monster
}
