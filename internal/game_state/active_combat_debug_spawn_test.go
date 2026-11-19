package game_state

import (
	"strconv"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSpawnDebugCombatMonstersAfterLifecycleTransitions(t *testing.T) {
	for _, lifecycle := range []string{"corpse", "debug defeat", "flee", "daemon retirement", "Rel Xen Bet"} {
		t.Run(lifecycle, func(t *testing.T) {
			state := newDebugReinforcementLifecycleState(t)
			combat := state.CombatScene
			monster := combat.MonsterManager.GetMonster(0)
			originalID := monster.CombatantID()
			originalEntry := combat.TurnQueue.GetEntry(0, false)
			originalSlot := originalEntry.sourceSlot
			switch lifecycle {
			case "corpse":
				defeatDebugReinforcementMonster(t, state, 0)
				require.False(t, originalEntry.sourceSlotReleased)
			case "debug defeat":
				_, err := state.KillDebugCombatMonster(DebugCombatKillRequest{
					EncounterID: combat.EncounterID, CombatantID: originalID, ExpectedHP: monster.CurrentHP(),
				})
				require.NoError(t, err)
				require.True(t, combat.Victory)
			case "flee":
				monster.SetPosition(references.Position{X: 0, Y: 5})
				monster.SetCurrentHP(1)
				monster.SetFleeing(true)
				combat.SetPartyMemberPosition(0, references.Position{X: 5, Y: 5})
				state.executeFleeingMonsterTurnWithRNG(monster, 0, newSequenceRNG(0, 255))
				require.False(t, monster.IsActive())
				require.True(t, originalEntry.sourceSlotReleased)
			case "daemon retirement":
				monster.monsterType = references.MonsterDevil
				monster.enemyRef = &(*state.GameReferences.EnemyReferences)[references.MonsterDevil]
				state.resolveMonsterPossession(monster, 0, CombatTurnIdentity{IsPartyMember: true}, newSequenceRNG(60))
				require.False(t, monster.IsActive())
				require.False(t, monster.IsDead())
				require.True(t, originalEntry.sourceSlotReleased)
			case "Rel Xen Bet":
				state.PartyState.GetAvatar().Intelligence = 99
				require.True(t, state.CastCombatRelXenBet(0, CombatTurnIdentity{EntityIndex: 0}))
				require.False(t, monster.IsActive())
				require.Equal(t, references.MonsterRat, combat.MonsterManager.GetMonster(1).MonsterType())
			}
			countBefore := combat.MonsterManager.Count()
			activeBefore := combat.MonsterManager.CountActive()
			nextID := combat.nextCombatantID

			result, err := state.SpawnDebugCombatMonsters(debugReinforcementRequest(state, 1))

			require.NoError(t, err)
			require.Len(t, result.Spawned, 1)
			assert.Equal(t, countBefore, result.TotalCountBefore)
			assert.Equal(t, countBefore+1, result.TotalCountAfter)
			assert.Equal(t, activeBefore, result.ActiveCountBefore)
			assert.Equal(t, activeBefore+1, result.ActiveCountAfter)
			assert.Equal(t, nextID, result.Spawned[0].CombatantID)
			assert.Same(t, monster, combat.MonsterManager.GetMonsterByID(originalID))
			assert.Same(t, originalEntry, combat.TurnQueue.GetEntry(0, false))
			assert.False(t, combat.Victory)
			_, queued := combat.TurnQueue.CountRemaining()
			assert.Equal(t, result.ActiveCountAfter, queued)
			spawnedEntry := combat.TurnQueue.GetEntry(result.Spawned[0].ManagerIndex, false)
			require.NotNil(t, spawnedEntry)
			assert.Same(t, combat.MonsterManager.GetMonsterByID(nextID), spawnedEntry.Combatant)
			if lifecycle == "corpse" || lifecycle == "Rel Xen Bet" {
				assert.NotEqual(t, originalSlot, spawnedEntry.sourceSlot)
			} else {
				assert.Equal(t, originalSlot, spawnedEntry.sourceSlot)
			}
		})
	}
}

func TestSpawnDebugCombatMonstersPreflightsSharedCapacityBeforeRNGOrIDs(t *testing.T) {
	// Loot and corpses no longer count: see the yielded-slot tests below.
	for _, occupant := range []string{"field", "PC clone"} {
		for _, freeSlots := range []int{0, 1} {
			t.Run(occupant+"/free="+strconv.Itoa(freeSlots), func(t *testing.T) {
				state := newDebugReinforcementLifecycleState(t)
				combat := state.CombatScene
				position := references.Position{X: 9, Y: 9}
				for combat.originalObjectSlotCount() < sourceCombatQueueSlots-freeSlots {
					switch occupant {
					case "field":
						require.True(t, combat.AddField(NewCombatField(CombatFieldFire, position)))
					case "PC clone":
						clone := combat.addPCCloneFromSource(CombatTurnIdentity{IsPartyMember: true}, 0,
							position, NewCombatPCProvider(state.PartyState.GetAvatar()))
						require.NotZero(t, clone.Slot)
					}
				}
				request := debugReinforcementRequest(state, 2)
				assertDebugReinforcementRejectedWithoutMutation(t, state, request, ErrDebugCombatSpawn)
				if freeSlots == 0 {
					return
				}
				nextID := combat.nextCombatantID
				request.ExactCount = 1
				result, err := state.SpawnDebugCombatMonsters(request)
				require.NoError(t, err)
				require.Len(t, result.Spawned, 1)
				assert.Equal(t, nextID, result.Spawned[0].CombatantID)
				assert.Equal(t, sourceCombatQueueSlots, combat.originalObjectSlotCount())
			})
		}
	}
}

// REDUX DEVIATION: COMBAT-0001. A fight full of the dead has room again: each
// new monster takes the lowest queue slot held only for a dead monster's remains,
// and the remains stay on the ground.
func TestSpawnDebugCombatMonstersReusesSlotsHeldForRemains(t *testing.T) {
	state := newDebugReinforcementLifecycleState(t)
	combat := state.CombatScene
	fillDebugReinforcementMonsterSlots(state)
	for index := range combat.MonsterManager.Count() {
		defeatDebugReinforcementMonster(t, state, index)
	}
	remains := combat.ObjectCount()
	require.Equal(t, sourceCombatQueueSlots-sourceMonsterSlotStart, remains)
	require.Zero(t, combat.MonsterManager.CountActive())
	for slot := sourceMonsterSlotStart; slot < sourceCombatQueueSlots; slot++ {
		require.NotNil(t, combat.TurnQueue.getRetainedEntryAtSourceSlot(slot), "every monster slot is held for remains")
	}

	result, err := state.SpawnDebugCombatMonsters(debugReinforcementRequest(state, 2))

	require.NoError(t, err)
	require.Len(t, result.Spawned, 2)
	for offset, spawned := range result.Spawned {
		entry := combat.TurnQueue.GetEntry(spawned.ManagerIndex, false)
		assert.Equal(t, sourceMonsterSlotStart+offset, entry.sourceSlot, "the lowest held slots are reused first")
		assert.NotZero(t, entry.originalObjectSlot)
	}
	assert.Equal(t, remains, combat.ObjectCount(), "no corpse or chest is removed to make room")
	assert.Equal(t, 2, combat.MonsterManager.CountActive())
}

func TestSpawnDebugCombatMonstersIgnoresLootHoldingObjectSlots(t *testing.T) {
	state := newDebugReinforcementLifecycleState(t)
	combat := state.CombatScene
	position := references.Position{X: 9, Y: 9}
	for combat.originalObjectSlotCount() < sourceCombatQueueSlots {
		require.True(t, combat.addLootObject(NewCombatLooseItem(position, references.Food, 1, indexes.ItemFood)))
	}
	loot := combat.ObjectCount()

	result, err := state.SpawnDebugCombatMonsters(debugReinforcementRequest(state, 1))

	require.NoError(t, err)
	require.Len(t, result.Spawned, 1)
	assert.Equal(t, loot, combat.ObjectCount())
	assert.Equal(t, sourceCombatQueueSlots, combat.originalObjectSlotCount(), "the table itself stays full")
}

func TestSpawnDebugCombatMonstersReusesReleasedSlotsPastHistoricalLimit(t *testing.T) {
	state := newDebugReinforcementLifecycleState(t)
	combat := state.CombatScene
	fillDebugReinforcementMonsterSlots(state)
	monster := combat.MonsterManager.GetMonster(0)
	entry := combat.TurnQueue.GetEntry(0, false)
	freedSource, freedObject := entry.sourceSlot, entry.originalObjectSlot
	monster.SetPosition(references.Position{X: 0, Y: 5})
	monster.SetCurrentHP(1)
	monster.SetFleeing(true)
	combat.SetPartyMemberPosition(0, references.Position{X: 5, Y: 5})
	state.executeFleeingMonsterTurnWithRNG(monster, 0, newSequenceRNG(0, 255))
	require.False(t, monster.IsActive())
	require.Greater(t, combat.MonsterManager.Count(), references.MaxMonstersInCombat)
	request := debugReinforcementRequest(state, 2)
	assertDebugReinforcementRejectedWithoutMutation(t, state, request, ErrDebugCombatSpawn)

	request.ExactCount = 1
	result, err := state.SpawnDebugCombatMonsters(request)

	require.NoError(t, err)
	require.Len(t, result.Spawned, 1)
	spawned := combat.TurnQueue.GetEntry(result.Spawned[0].ManagerIndex, false)
	assert.Equal(t, freedSource, spawned.sourceSlot)
	assert.Equal(t, freedObject, spawned.originalObjectSlot)
	assert.Equal(t, sourceCombatQueueSlots-sourceMonsterSlotStart+1, result.TotalCountAfter)
	assert.Equal(t, sourceCombatQueueSlots-sourceMonsterSlotStart, result.ActiveCountAfter)
}

func TestSpawnDebugCombatMonstersPreservesSixteenPerRequestLimit(t *testing.T) {
	state := newDebugReinforcementLifecycleState(t)
	request := debugReinforcementRequest(state, references.MaxMonstersInCombat+1)
	assertDebugReinforcementRejectedWithoutMutation(t, state, request, ErrDebugCombatSpawn)
	request.ExactCount = references.MaxMonstersInCombat

	result, err := state.SpawnDebugCombatMonsters(request)

	require.NoError(t, err)
	assert.Len(t, result.Spawned, references.MaxMonstersInCombat)
	assert.Equal(t, references.MaxMonstersInCombat+1, result.TotalCountAfter)
}

func TestSpawnDebugCombatMonstersNonExactModesRespectSharedCapacity(t *testing.T) {
	for _, mode := range []CombatEncounterCountMode{CombatEncounterCountAutomatic, CombatEncounterCountMaximum} {
		t.Run(strconv.Itoa(int(mode)), func(t *testing.T) {
			state := newDebugReinforcementLifecycleState(t)
			combat := state.CombatScene
			for combat.originalObjectSlotCount() < sourceCombatQueueSlots-1 {
				require.True(t, combat.AddField(NewCombatField(CombatFieldFire, references.Position{X: 9, Y: 9})))
			}
			request := debugReinforcementRequest(state, 0)
			request.CountMode = mode
			request.FriendsEnabled = true
			(*state.GameReferences.EnemyReferences)[references.MonsterOrc].Friend =
				&(*state.GameReferences.EnemyReferences)[references.MonsterRat]

			result, err := state.SpawnDebugCombatMonsters(request)

			require.NoError(t, err)
			assert.Len(t, result.Spawned, 1)
			assert.Equal(t, 1, result.PrimaryCount+result.FriendCount)
			assert.Equal(t, sourceCombatQueueSlots, combat.originalObjectSlotCount())
		})
	}
}

func TestSpawnDebugCombatMonstersUsesExactCloneOccupancyAfterSourceDeparts(t *testing.T) {
	for _, underground := range []bool{false, true} {
		t.Run(map[bool]string{false: "surface", true: "UNDER"}[underground], func(t *testing.T) {
			state := newDebugReinforcementLifecycleState(t)
			combat := state.CombatScene
			position := references.Position{X: 2, Y: 2}
			setDebugReinforcementPositions(combat, position)
			clone := combat.addPCCloneFromSource(CombatTurnIdentity{IsPartyMember: true}, 0,
				position, NewCombatPCProvider(state.PartyState.GetAvatar()))
			require.NotZero(t, clone.Slot)
			combat.setCombatPCUnderground(clone, underground)
			state.departPCFromCombat(CombatTurnIdentity{IsPartyMember: true})
			require.False(t, combat.PartyActive[0])
			require.True(t, combat.IsCombatPCActive(clone))
			request := debugReinforcementRequest(state, 1)
			if !underground {
				assertDebugReinforcementRejectedWithoutMutation(t, state, request, ErrDebugCombatSpawn)
				return
			}

			result, err := state.SpawnDebugCombatMonsters(request)

			require.NoError(t, err)
			require.Len(t, result.Spawned, 1)
			assert.Equal(t, position, result.Spawned[0].Position)
		})
	}
}

func TestSpawnDebugCombatMonstersFieldOccupancy(t *testing.T) {
	for _, kind := range []CombatFieldKind{CombatFieldProtection, CombatFieldFire, CombatFieldPoison, CombatFieldSleep} {
		t.Run(strconv.Itoa(int(kind)), func(t *testing.T) {
			state := newDebugReinforcementLifecycleState(t)
			combat := state.CombatScene
			position := references.Position{X: 2, Y: 2}
			setDebugReinforcementPositions(combat, position)
			require.True(t, combat.AddField(NewCombatField(kind, position)))
			request := debugReinforcementRequest(state, 1)
			if kind == CombatFieldProtection {
				assertDebugReinforcementRejectedWithoutMutation(t, state, request, ErrDebugCombatSpawn)
				return
			}

			result, err := state.SpawnDebugCombatMonsters(request)

			require.NoError(t, err)
			require.Len(t, result.Spawned, 1)
			assert.Equal(t, position, result.Spawned[0].Position)
			assert.Len(t, combat.Fields(), 1)
		})
	}
}

func TestSpawnDebugCombatMonstersAuthoredPositionOccupancy(t *testing.T) {
	for _, occupant := range []string{"PC", "UNDER PC", "sleeping monster", "loose item", "corpse", "wall terrain", "duplicate positions"} {
		t.Run(occupant, func(t *testing.T) {
			state := newDebugReinforcementLifecycleState(t)
			combat := state.CombatScene
			position := references.Position{X: 2, Y: 2}
			setDebugReinforcementPositions(combat, position)
			wantSpawn := false
			switch occupant {
			case "PC", "UNDER PC":
				combat.SetPartyMemberPosition(0, position)
				wantSpawn = occupant == "UNDER PC"
				combat.setCombatPCUnderground(CombatTurnIdentity{IsPartyMember: true}, wantSpawn)
			case "sleeping monster":
				monster := combat.MonsterManager.GetMonster(0)
				monster.SetPosition(position)
				monster.SetAsleep(true)
			case "loose item":
				require.True(t, combat.AddObject(NewCombatLooseItem(position, references.Food, 1, indexes.ItemFood)))
			case "corpse":
				require.True(t, combat.AddObject(NewCombatCorpse(position)))
			case "wall terrain":
				state.setCombatTerrain(position, indexes.StoneBrickWall)
				wantSpawn = true
			case "duplicate positions":
				assertDebugReinforcementRejectedWithoutMutation(t, state, debugReinforcementRequest(state, 2), ErrDebugCombatSpawn)
				wantSpawn = true
			}
			request := debugReinforcementRequest(state, 1)
			if !wantSpawn {
				assertDebugReinforcementRejectedWithoutMutation(t, state, request, ErrDebugCombatSpawn)
				return
			}

			result, err := state.SpawnDebugCombatMonsters(request)

			require.NoError(t, err)
			require.Len(t, result.Spawned, 1)
			assert.Equal(t, position, result.Spawned[0].Position)
			assert.Equal(t, 1, result.AvailablePositions)
		})
	}
}

func TestSpawnDebugCombatMonstersRejectsBoundaryBeforeMutation(t *testing.T) {
	for _, boundary := range []string{"stale", "monster turn", "targeting"} {
		t.Run(boundary, func(t *testing.T) {
			state := newDebugReinforcementLifecycleState(t)
			request := debugReinforcementRequest(state, 1)
			wantErr := ErrDebugCombatBusy
			switch boundary {
			case "stale":
				request.EncounterID++
				wantErr = ErrDebugCombatStale
			case "monster turn":
				state.CombatScene.ActiveMonsterIndex = 0
			case "targeting":
				state.CombatScene.ActiveTargetPCIndex = 0
			}
			assertDebugReinforcementRejectedWithoutMutation(t, state, request, wantErr)
		})
	}
}

func TestSpawnDebugCombatMonstersDetectsWrongIdentityWithMatchingCounts(t *testing.T) {
	state := newDebugReinforcementLifecycleState(t)
	combat := state.CombatScene
	reference := combat.MonsterManager.GetMonster(0).EnemyRef()
	combat.TurnQueue.GetEntry(0, false).Combatant = NewCombatMonster(
		references.MonsterOrc, reference, references.Position{X: 4, Y: 4}, newFixedRNG(1),
	)
	_, queued := combat.TurnQueue.CountRemaining()
	require.Equal(t, combat.MonsterManager.CountActive(), queued)
	seed, calls, nextID := state.RandomSeed(), state.GetRNG().Calls(), combat.nextCombatantID

	assert.Panics(t, func() {
		_, _ = state.SpawnDebugCombatMonsters(debugReinforcementRequest(state, 1))
	})
	assert.Equal(t, seed, state.RandomSeed())
	assert.Equal(t, calls, state.GetRNG().Calls())
	assert.Equal(t, nextID, combat.nextCombatantID)
}

func newDebugReinforcementLifecycleState(t *testing.T) *GameState {
	t.Helper()
	state := newDebugCombatSpawnTestState()
	state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	state.PartyState.GetAvatar().SetStatus(party_state.Good)
	for _, monsterType := range []references.MonsterType{references.MonsterRat, references.MonsterDevil} {
		reference := (*state.GameReferences.EnemyReferences)[references.MonsterOrc]
		reference.MonsterType = monsterType
		(*state.GameReferences.EnemyReferences)[monsterType] = reference
	}
	var rawMap [references.XCombatMapTiles][references.YCombatMapTiles]byte
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			rawMap[x][y] = byte(indexes.BrickFloor)
		}
	}
	state.CombatScene.CombatMapRef.SetRawData(&rawMap)
	tiles := references.NewTileReferences()
	state.GameReferences.TileReferences = tiles
	state.MapState.LayeredMaps = *map_state.NewLayeredMaps(tiles,
		&references.LargeMapReference{}, &references.LargeMapReference{}, 19, 13)
	state.MapState.LayeredMaps.ResetAndCreateCombatMap(state.CombatScene.CombatMapRef, tiles, 19, 13)

	return state
}

func debugReinforcementRequest(state *GameState, count int) DebugCombatSpawnRequest {
	seed := uint64(8675309)

	return DebugCombatSpawnRequest{
		EncounterID: state.CombatScene.EncounterID, MonsterType: references.MonsterOrc,
		CountMode: CombatEncounterCountExact, ExactCount: count, Seed: &seed,
	}
}

func assertDebugReinforcementRejectedWithoutMutation(
	t *testing.T, state *GameState, request DebugCombatSpawnRequest, wantErr error,
) {
	t.Helper()
	combat := state.CombatScene
	// A nonzero call count catches resetting to the old seed as well as consuming RNG.
	state.RollDice(30)
	seed, calls := state.RandomSeed(), state.GetRNG().Calls()
	nextID, nextCloneSlot := combat.nextCombatantID, combat.nextPCCloneSlot
	snapshot := state.debugCombatSnapshot()
	queue := make([]TurnQueueEntry, len(combat.TurnQueue.entries))
	for index, entry := range combat.TurnQueue.entries {
		queue[index] = *entry
	}
	objects := append([]CombatObject(nil), combat.objects...)
	fields := append([]CombatField(nil), combat.fields...)
	queueIndex, actions := combat.TurnQueue.currentIndex, combat.TurnQueue.actionCount

	result, err := state.SpawnDebugCombatMonsters(request)

	require.ErrorIs(t, err, wantErr)
	assert.Nil(t, result)
	assert.Equal(t, seed, state.RandomSeed())
	assert.Equal(t, calls, state.GetRNG().Calls())
	assert.Equal(t, nextID, combat.nextCombatantID)
	assert.Equal(t, nextCloneSlot, combat.nextPCCloneSlot)
	assert.Equal(t, snapshot, state.debugCombatSnapshot())
	require.Len(t, combat.TurnQueue.entries, len(queue))
	for index, entry := range combat.TurnQueue.entries {
		assert.Equal(t, queue[index], *entry)
	}
	assert.Equal(t, queueIndex, combat.TurnQueue.currentIndex)
	assert.Equal(t, actions, combat.TurnQueue.actionCount)
	assert.Equal(t, objects, append([]CombatObject(nil), combat.objects...))
	assert.Equal(t, fields, append([]CombatField(nil), combat.fields...))
}

func fillDebugReinforcementMonsterSlots(state *GameState) {
	combat := state.CombatScene
	reference := combat.MonsterManager.GetMonster(0).EnemyRef()
	for combat.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
		combat.AddMonsterToQueue(NewCombatMonster(references.MonsterOrc, reference,
			references.Position{X: 8, Y: 8}, newFixedRNG(1)))
	}
}

func defeatDebugReinforcementMonster(t *testing.T, state *GameState, index int) {
	t.Helper()
	monster := state.CombatScene.MonsterManager.GetMonster(index)
	require.True(t, monster.TakeDamage(monster.CurrentHP()))
	state.finalizeCombatMonsterDeath(monster, index, state.PartyState.GetAvatar(), 0)
	require.True(t, state.CombatScene.HasObjectAt(monster.Position()))
}

func setDebugReinforcementPositions(combat *CombatState, position references.Position) {
	for index := range combat.CombatMapRef.MonsterPositions {
		combat.CombatMapRef.MonsterPositions[index] = position
	}
}
