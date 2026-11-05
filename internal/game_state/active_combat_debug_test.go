package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCombatMonsterIDsAreEncounterScopedAndNeverReused(t *testing.T) {
	party := party_state.LoadFromRaw([4192]byte{})
	combat := NewCombatState(references.NewCombatMapReference(0, references.BritanniaCombatTerritory), CombatEntryNorth, party)
	reference := activeCombatDebugEnemyReference()
	first := NewCombatMonster(references.MonsterOrc, &reference, references.Position{X: 4, Y: 4}, newFixedRNG(1))
	second := NewCombatMonster(references.MonsterOrc, &reference, references.Position{X: 5, Y: 5}, newFixedRNG(1))

	combat.AddMonsterToQueue(first)
	combat.AddMonsterToQueue(second)

	assert.Equal(t, CombatantID(1), first.CombatantID())
	assert.Equal(t, CombatantID(2), second.CombatantID())
	assert.Same(t, second, combat.MonsterManager.GetMonsterByID(2))
	assert.Nil(t, combat.MonsterManager.GetMonsterByID(3))
	assert.Panics(t, func() { combat.AddMonsterToQueue(first) })
}

func TestSetDebugCombatMonsterAsleepUsesStableIdentityAndQuiescentBoundary(t *testing.T) {
	state, monster := newActiveCombatDebugTestState()

	change, err := state.SetDebugCombatMonsterAsleep(7, monster.CombatantID(), false, true)
	require.NoError(t, err)
	assert.True(t, change.After)
	assert.True(t, monster.IsAsleep())

	_, err = state.SetDebugCombatMonsterAsleep(7, monster.CombatantID(), false, true)
	assert.ErrorIs(t, err, ErrDebugCombatStale)
	state.CombatScene.ActiveMonsterIndex = 0
	_, err = state.SetDebugCombatMonsterAsleep(7, monster.CombatantID(), true, false)
	assert.ErrorIs(t, err, ErrDebugCombatBusy)
}

func newActiveCombatDebugTestState() (*GameState, *CombatMonster) {
	party := party_state.LoadFromRaw([4192]byte{})
	combat := NewCombatState(references.NewCombatMapReference(0, references.BritanniaCombatTerritory), CombatEntryNorth, party)
	combat.EncounterID = 7
	reference := activeCombatDebugEnemyReference()
	monster := NewCombatMonster(references.MonsterOrc, &reference, references.Position{X: 4, Y: 4}, newFixedRNG(1))
	combat.AddMonsterToQueue(monster)
	state := &GameState{
		PartyState:  *party,
		CombatScene: combat,
	}
	state.MapState.PlayerLocation.Location = references.CombatLocation
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return 0 }, nil)

	return state, monster
}

func activeCombatDebugEnemyReference() references.EnemyReference {
	return references.EnemyReference{
		MonsterType: references.MonsterOrc,
		Dexterity:   12,
		HitPoints:   20,
		Name:        "Orc",
		NamePlural:  "Orcs",
		EnemyAbilities: map[references.EnemyAbility]bool{
			references.Poison: true,
		},
	}
}

func TestExitDebugCombatRejectsFixedDungeonRoomContinuation(t *testing.T) {
	state, _ := newActiveCombatDebugTestState()
	state.CombatScene.DungeonRoomOrigin = &DungeonRoomEncounterOrigin{
		Location: references.Deceit, DungeonIndex: references.DeceitDungeon, Room: 1, Live: true,
	}

	_, err := state.ExitDebugCombat(state.CombatScene.EncounterID)

	assert.ErrorIs(t, err, ErrDebugCombatBusy)
	assert.NotNil(t, state.CombatScene)
}

func TestExitDebugCombatRejectsDungeonWandererContinuation(t *testing.T) {
	for _, kind := range []DungeonWandererEncounterKind{
		DungeonWandererEncounterDirect,
		DungeonWandererEncounterContact,
	} {
		state, _ := newActiveCombatDebugTestState()
		state.CombatScene.DungeonWandererOrigin = &DungeonWandererEncounterOrigin{Kind: kind}

		_, err := state.ExitDebugCombat(state.CombatScene.EncounterID)

		assert.ErrorIs(t, err, ErrDebugCombatBusy)
		assert.NotNil(t, state.CombatScene)
	}
}

func TestSpawnDebugCombatMonstersCommitsExactBatchToManagerAndQueue(t *testing.T) {
	state := newDebugCombatSpawnTestState()
	seed := uint64(1234)

	result, err := state.SpawnDebugCombatMonsters(DebugCombatSpawnRequest{
		EncounterID: 9, MonsterType: references.MonsterOrc, CountMode: CombatEncounterCountExact,
		ExactCount: 2, FriendsEnabled: false, Seed: &seed,
	})

	require.NoError(t, err)
	assert.Equal(t, 3, state.CombatScene.MonsterManager.Count())
	_, queuedMonsters := state.CombatScene.TurnQueue.CountRemaining()
	assert.Equal(t, 3, queuedMonsters)
	assert.Equal(t, 2, result.PrimaryCount)
	assert.Zero(t, result.FriendCount)
	require.Len(t, result.Spawned, 2)
	assert.NotEqual(t, result.Spawned[0].CombatantID, result.Spawned[1].CombatantID)
	assert.False(t, state.CombatScene.Victory)
}

func TestSpawnDebugCombatMonstersRejectsOversizedBatchWithoutPartialMutation(t *testing.T) {
	state := newDebugCombatSpawnTestState()
	countBefore := state.CombatScene.MonsterManager.Count()
	_, queuedBefore := state.CombatScene.TurnQueue.CountRemaining()

	result, err := state.SpawnDebugCombatMonsters(DebugCombatSpawnRequest{
		EncounterID: 9, MonsterType: references.MonsterOrc, CountMode: CombatEncounterCountExact,
		ExactCount: references.MaxMonstersInCombat + 1, FriendsEnabled: false,
	})

	require.ErrorIs(t, err, ErrDebugCombatSpawn)
	assert.Nil(t, result)
	assert.Equal(t, countBefore, state.CombatScene.MonsterManager.Count())
	_, queuedAfter := state.CombatScene.TurnQueue.CountRemaining()
	assert.Equal(t, queuedBefore, queuedAfter)
}

func TestDebugActiveCombatSnapshotReportsStableIDsHPQueueAndSeed(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 2)
	state.SetRandomSeed(8675309)

	snapshot, err := state.DebugActiveCombatSnapshot(21)

	require.NoError(t, err)
	assert.Equal(t, uint64(8675309), snapshot.RNGSeed)
	require.Len(t, snapshot.Monsters, 2)
	assert.Equal(t, monsters[0].CombatantID(), snapshot.Monsters[0].CombatantID)
	assert.Equal(t, monsters[1].CombatantID(), snapshot.Monsters[1].CombatantID)
	assert.Equal(t, 20, snapshot.Monsters[0].CurrentHP)
	assert.Equal(t, CombatTurnActive, snapshot.Monsters[0].QueueStatus)
	assert.Equal(t, 2, snapshot.QueueMonsterCount)
}

func TestDamageDebugCombatMonsterRejectsStaleHPWithoutMutation(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 1)
	monster := monsters[0]

	result, err := state.DamageDebugCombatMonster(DebugCombatDamageRequest{
		EncounterID: 21, CombatantID: monster.CombatantID(), ExpectedHP: 19, Damage: 5,
	})

	require.ErrorIs(t, err, ErrDebugCombatStale)
	assert.Nil(t, result)
	assert.Equal(t, 20, monster.CurrentHP())
	queue, found := state.CombatScene.TurnQueue.GetEntrySnapshot(CombatTurnIdentity{
		IsPartyMember: false, EntityIndex: 0,
	})
	require.True(t, found)
	assert.Equal(t, CombatTurnActive, queue.Status)
}

func TestDamageDebugCombatMonsterEnforcesDomainBoundsBeforeMutation(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 1)
	monster := monsters[0]

	for _, damage := range []int{DebugCombatDamageMinimum - 1, DebugCombatDamageMaximum + 1} {
		result, err := state.DamageDebugCombatMonster(DebugCombatDamageRequest{
			EncounterID: 21, CombatantID: monster.CombatantID(), ExpectedHP: 20, Damage: damage,
		})
		require.ErrorIs(t, err, ErrDebugCombatTarget)
		assert.Nil(t, result)
		assert.Equal(t, 20, monster.CurrentHP())
	}
}

func TestDamageDebugCombatMonsterUpdatesNonlethalHPWithoutQueueRemoval(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 1)
	monster := monsters[0]

	result, err := state.DamageDebugCombatMonster(DebugCombatDamageRequest{
		EncounterID: 21, CombatantID: monster.CombatantID(), ExpectedHP: 20, Damage: 7,
	})

	require.NoError(t, err)
	assert.Equal(t, 13, monster.CurrentHP())
	require.Len(t, result.Changes, 1)
	assert.Equal(t, DebugCombatHPChange{
		CombatantID: monster.CombatantID(), BeforeHP: 20, AfterHP: 13, Defeated: false,
	}, result.Changes[0])
	assert.Equal(t, CombatTurnActive, result.After.Monsters[0].QueueStatus)
	assert.False(t, result.After.Victory)
}

func TestLethalDebugDamageSynchronizesQueueWithoutXPOrObjects(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 2)
	monster := monsters[0]
	avatar := state.PartyState.GetAvatar()
	experienceBefore := avatar.Exp
	objectsBefore := state.CombatScene.ObjectCount()

	result, err := state.DamageDebugCombatMonster(DebugCombatDamageRequest{
		EncounterID: 21, CombatantID: monster.CombatantID(), ExpectedHP: 20, Damage: 20,
	})

	require.NoError(t, err)
	assert.True(t, monster.IsDead())
	assert.Equal(t, CombatTurnDead, result.After.Monsters[0].QueueStatus)
	assert.Equal(t, 1, result.After.QueueMonsterCount)
	assert.Equal(t, experienceBefore, avatar.Exp)
	assert.Equal(t, objectsBefore, state.CombatScene.ObjectCount())
	assert.Zero(t, result.ExperienceAwarded)
	assert.Zero(t, result.ObjectsCreated)
	assert.False(t, result.VictoryTriggered)
}

func TestKillDebugCombatMonsterTriggersVictoryUnderDebugPolicy(t *testing.T) {
	state, monsters, mock := newCombatMutationTestState(t, 1)
	monster := monsters[0]

	result, err := state.KillDebugCombatMonster(DebugCombatKillRequest{
		EncounterID: 21, CombatantID: monster.CombatantID(), ExpectedHP: 20,
	})

	require.NoError(t, err)
	assert.True(t, result.VictoryTriggered)
	assert.True(t, result.After.Victory)
	assert.Equal(t, CombatTurnDead, result.After.Monsters[0].QueueStatus)
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventVictory, mock.CombatEvents[0].Kind)
}

func TestKillAllDebugCombatMonstersIsAtomicAndSynchronizesVictory(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 3)
	expected := debugCombatSnapshotForTest(t, state)
	expected.Monsters[1].CurrentHP--

	result, err := state.KillAllDebugCombatMonsters(DebugCombatBatchRequest{EncounterID: 21, Expected: expected})

	require.ErrorIs(t, err, ErrDebugCombatStale)
	assert.Nil(t, result)
	for _, monster := range monsters {
		assert.Equal(t, 20, monster.CurrentHP())
	}

	result, err = state.KillAllDebugCombatMonsters(DebugCombatBatchRequest{
		EncounterID: 21, Expected: debugCombatSnapshotForTest(t, state),
	})
	require.NoError(t, err)
	require.Len(t, result.Changes, 3)
	assert.Zero(t, result.After.ActiveCount)
	assert.Zero(t, result.After.QueueMonsterCount)
	assert.True(t, result.After.Victory)
	for _, snapshot := range result.After.Monsters {
		assert.Zero(t, snapshot.CurrentHP)
		assert.Equal(t, CombatTurnDead, snapshot.QueueStatus)
	}
}

func TestSleepAllDebugCombatMonstersWakesNobodyAndSkipsTheDefeated(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 3)
	monsters[0].SetAsleep(true)
	expected := debugCombatSnapshotForTest(t, state)
	expected.Monsters[2].CurrentHP--

	changes, err := state.SleepAllDebugCombatMonsters(DebugCombatBatchRequest{EncounterID: 21, Expected: expected})

	require.ErrorIs(t, err, ErrDebugCombatStale)
	assert.Nil(t, changes)
	assert.False(t, monsters[1].IsAsleep(), "a stale review changes nothing")

	_, err = state.KillDebugCombatMonster(DebugCombatKillRequest{
		EncounterID: 21, CombatantID: monsters[2].CombatantID(), ExpectedHP: monsters[2].CurrentHP(),
	})
	require.NoError(t, err)
	changes, err = state.SleepAllDebugCombatMonsters(DebugCombatBatchRequest{
		EncounterID: 21, Expected: debugCombatSnapshotForTest(t, state),
	})

	require.NoError(t, err)
	assert.Equal(t, []DebugCombatSleepChange{{CombatantID: monsters[1].CombatantID(), Before: false, After: true}}, changes)
	assert.True(t, monsters[0].IsAsleep())
	assert.True(t, monsters[1].IsAsleep())
	assert.False(t, monsters[2].IsAsleep(), "the defeated are not put to sleep")
}

func TestClearDebugCombatMonstersIsOutcomeFreeAndRemovesMonsterQueueEntries(t *testing.T) {
	state, monsters, mock := newCombatMutationTestState(t, 2)
	state.CombatScene.LastAttackTargets[0] = 1
	avatar := state.PartyState.GetAvatar()
	experienceBefore := avatar.Exp
	objectsBefore := state.CombatScene.ObjectCount()
	partyEntriesBefore := state.CombatScene.TurnQueue.TotalEntryCount() - len(monsters)

	result, err := state.ClearDebugCombatMonsters(DebugCombatBatchRequest{
		EncounterID: 21, Expected: debugCombatSnapshotForTest(t, state),
	})

	require.NoError(t, err)
	assert.Zero(t, state.CombatScene.MonsterManager.Count())
	assert.Equal(t, partyEntriesBefore, state.CombatScene.TurnQueue.TotalEntryCount())
	assert.Equal(t, NoActiveMonster, state.CombatScene.ActiveMonsterIndex)
	assert.Equal(t, NoTargetPC, state.CombatScene.ActiveTargetPCIndex)
	assert.Equal(t, NoCombatTarget, state.CombatScene.LastAttackTargets[0])
	assert.False(t, state.CombatScene.Victory)
	assert.Equal(t, experienceBefore, avatar.Exp)
	assert.Equal(t, objectsBefore, state.CombatScene.ObjectCount())
	assert.False(t, result.VictoryTriggered)
	assert.Empty(t, mock.CombatEvents)

	state.AdvanceCombatTurn()
	assert.False(t, state.CombatScene.Victory)
	assert.Empty(t, mock.CombatEvents)
}

func TestClearDebugDungeonRoomEnemiesDefeatsHostilesAndDeclaresVictory(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 2)
	state.CombatScene.ExitContext = CombatExitDungeonRoom
	state.CombatScene.DungeonRoomOrigin = &DungeonRoomEncounterOrigin{
		Location: references.Deceit, DungeonIndex: references.DeceitDungeon, Room: 4, Live: true,
	}
	monsters[1].SetLoyalty(CombatMonsterLoyaltyParty)

	result, err := state.ClearDebugDungeonRoomEnemies(DebugCombatBatchRequest{
		EncounterID: 21,
		Expected:    debugCombatSnapshotForTest(t, state),
	})

	require.NoError(t, err)
	require.Len(t, result.Changes, 1)
	assert.True(t, monsters[0].IsDead())
	assert.False(t, monsters[1].IsDead())
	assert.True(t, state.CombatScene.Victory)
	assert.True(t, result.VictoryTriggered)
	assert.Zero(t, result.ExperienceAwarded)
	assert.Zero(t, result.ObjectsCreated)
}

func TestClearDebugDungeonRoomEnemiesRejectsCombatWithoutFixedRoomOrigin(t *testing.T) {
	state, monsters, _ := newCombatMutationTestState(t, 1)

	result, err := state.ClearDebugDungeonRoomEnemies(DebugCombatBatchRequest{
		EncounterID: 21,
		Expected:    debugCombatSnapshotForTest(t, state),
	})

	require.ErrorIs(t, err, ErrDebugDungeonRoomRequired)
	assert.Nil(t, result)
	assert.Equal(t, 20, monsters[0].CurrentHP())
}

func newCombatMutationTestState(
	t *testing.T,
	monsterCount int,
) (*GameState, []*CombatMonster, *MockSystemCallbacks) {
	t.Helper()
	party := party_state.LoadFromRaw([4192]byte{})
	party.GetAvatar().SetMaxHp(100)
	party.GetAvatar().SetHp(100)
	combat := NewCombatState(
		references.NewCombatMapReference(0, references.BritanniaCombatTerritory), CombatEntryNorth, party,
	)
	combat.EncounterID = 21
	reference := activeCombatDebugEnemyReference()
	monsters := make([]*CombatMonster, 0, monsterCount)
	for index := range monsterCount {
		monster := NewCombatMonster(
			references.MonsterOrc, &reference,
			references.Position{X: references.Coordinate(index + 1), Y: 4}, newFixedRNG(1),
		)
		combat.AddMonsterToQueue(monster)
		monsters = append(monsters, monster)
	}
	mock := NewMockSystemCallbacks(t)
	state := &GameState{
		PartyState: *party, CombatScene: combat, SystemCallbacks: mock.ToSystemCallbacks(),
	}
	state.SetRandomSeed(1)
	state.MapState.PlayerLocation.Location = references.CombatLocation
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return 0 }, nil)

	return state, monsters, mock
}

func debugCombatSnapshotForTest(t *testing.T, state *GameState) DebugCombatSnapshot {
	t.Helper()
	snapshot, err := state.DebugActiveCombatSnapshot(state.CombatScene.EncounterID)
	require.NoError(t, err)

	return snapshot
}

func newDebugCombatSpawnTestState() *GameState {
	party := party_state.LoadFromRaw([4192]byte{})
	party.GetAvatar().SetMaxHp(100)
	party.GetAvatar().SetHp(100)
	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	for index := range combatMap.MonsterPositions {
		combatMap.MonsterPositions[index] = references.Position{
			X: references.Coordinate(index%8 + 1), Y: references.Coordinate(index/8 + 1),
		}
	}
	combat := NewCombatState(combatMap, CombatEntryNorth, party)
	combat.EncounterID = 9
	orc := references.EnemyReference{
		MonsterType: references.MonsterOrc, Dexterity: 12, HitPoints: 20, MaxPerMap: 6,
		Name: "Orc", NamePlural: "Orcs", EnemyAbilities: map[references.EnemyAbility]bool{},
	}
	combat.AddMonsterToQueue(NewCombatMonster(
		references.MonsterOrc, &orc, references.Position{X: 4, Y: 4}, debugCombatFixedRNG{},
	))
	enemyReferences := make(references.EnemyReferences, references.TotalMonsterTypes)
	enemyReferences[references.MonsterOrc] = orc
	state := &GameState{
		PartyState: *party, CombatScene: combat,
		GameReferences: &references.GameReferences{EnemyReferences: &enemyReferences},
	}
	state.SetRandomSeed(1)
	state.MapState.PlayerLocation.Location = references.CombatLocation
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return 0 }, nil)

	return state
}

type debugCombatFixedRNG struct{}

func (debugCombatFixedRNG) Intn(int) int { return 1 }
