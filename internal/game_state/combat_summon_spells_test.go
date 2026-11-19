package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

type strictSummonRNG struct {
	t        *testing.T
	values   []int
	index    int
	requests []int
}

func (r *strictSummonRNG) Intn(n int) int {
	r.t.Helper()
	require.Less(r.t, r.index, len(r.values), "summon consumed unexpected RNG value for Intn(%d)", n)
	value := r.values[r.index]
	r.index++
	r.requests = append(r.requests, n)
	require.GreaterOrEqual(r.t, value, 0)
	require.Less(r.t, value, n)

	return value
}

func newStrictSummonRNG(t *testing.T, values ...int) *strictSummonRNG {
	t.Helper()

	return &strictSummonRNG{t: t, values: values}
}

func newCombatSummonTestState(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	gs, mock := newBritainCombatTestState(t, 1)

	rawData := &[references.XCombatMapTiles][references.YCombatMapTiles]byte{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			rawData[x][y] = byte(indexes.BrickFloor)
			position := references.Position{X: x, Y: y}
			gs.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BrickFloor)
		}
	}
	gs.CombatScene.CombatMapRef.SetRawData(rawData)
	for index := range gs.CombatScene.PartyActive {
		gs.CombatScene.PartyActive[index] = false
	}
	mock.Reset()

	return gs, mock
}

func TestKalXenMonsterTypeUsesOriginalWeights(t *testing.T) {
	tests := []struct {
		roll     int
		expected references.MonsterType
	}{
		{roll: 0, expected: references.MonsterRat},
		{roll: 5, expected: references.MonsterRat},
		{roll: 6, expected: references.MonsterSpider},
		{roll: 10, expected: references.MonsterSpider},
		{roll: 11, expected: references.MonsterBat},
		{roll: 13, expected: references.MonsterBat},
		{roll: 14, expected: references.MonsterSnake},
		{roll: 15, expected: references.MonsterSnake},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, kalXenMonsterType(test.roll), "roll %d", test.roll)
	}
}

func TestCombatSummonsAreImplementedNoTargetSpells(t *testing.T) {
	for _, spell := range []references.Spell{references.KalXen, references.InBetXen, references.KalXenCorp} {
		assert.True(t, IsImplementedSpellInContext(spell, references.ContextCombat))
		assert.Equal(t, references.TargetAutomatic, references.GetSpellInfo(spell).Targeting)
	}
}

func TestCastCombatKalXenStopsAfterEightPlacementAttempts(t *testing.T) {
	gs, mock := newCombatSummonTestState(t)
	values := []int{0}
	for range combatSummonPlacementAttempts {
		values = append(values, 15, 15)
	}
	rng := newStrictSummonRNG(t, values...)

	result := gs.castCombatKalXen(rng)

	assert.Equal(t, CombatSummonNoLegalPosition, result.Outcome)
	assert.Equal(t, combatSummonPlacementAttempts, result.Attempts)
	assert.Zero(t, result.Count)
	assert.Zero(t, gs.CombatScene.MonsterManager.Count())
	assert.Empty(t, mock.CombatEvents)
	assert.Equal(t, len(values), rng.index)
}

func TestCastCombatKalXenCreatesPartyLoyalMonsterAndStructuredEvent(t *testing.T) {
	gs, mock := newCombatSummonTestState(t)
	rng := newStrictSummonRNG(t, 6, 5, 5, 0)

	result := gs.castCombatKalXen(rng)

	require.Equal(t, CombatSummonSucceeded, result.Outcome)
	require.Len(t, result.Created, 1)
	created := result.Created[0]
	monster := gs.CombatScene.MonsterManager.GetMonster(created.Index)
	require.NotNil(t, monster)
	assert.Equal(t, references.MonsterSpider, monster.MonsterType())
	assert.True(t, monster.IsPartyLoyal())
	assert.Equal(t, references.Position{X: 5, Y: 5}, monster.Position())
	assert.Equal(t, monster.CombatantID(), created.CombatantID)
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, CombatEventSummon, event.Kind)
	assert.Equal(t, references.KalXen, event.Summon.Spell)
	assert.Equal(t, result.Created, event.Summon.Created)
	assert.Equal(t, 1, event.Summon.Count)
}

func TestCastCombatInBetXenStacksFourIndependentInsects(t *testing.T) {
	gs, mock := newCombatSummonTestState(t)
	rng := newStrictSummonRNG(t, 4, 7, 0, 1, 2, 3)

	result := gs.summonCombatMonsters(
		references.InBetXen,
		references.MonsterInsect,
		inBetXenSummonCount,
		CombatMonsterLoyaltyParty,
		rng,
	)

	position := references.Position{X: 4, Y: 7}
	assert.Equal(t, CombatSummonSucceeded, result.Outcome)
	assert.Equal(t, inBetXenSummonCount, result.Count)
	require.Len(t, result.Created, inBetXenSummonCount)
	stack := gs.CombatScene.MonsterManager.GetMonstersAtPosition(position)
	require.Len(t, stack, inBetXenSummonCount)
	ids := make(map[CombatantID]struct{}, inBetXenSummonCount)
	for _, monster := range stack {
		assert.Equal(t, references.MonsterInsect, monster.MonsterType())
		assert.True(t, monster.IsPartyLoyal())
		ids[monster.CombatantID()] = struct{}{}
	}
	assert.Len(t, ids, inBetXenSummonCount, "stack members must be independent combatants")
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, inBetXenSummonCount, mock.CombatEvents[0].Summon.Count)
}

func TestCastCombatInBetXenStopsAtRemainingOriginalCapacity(t *testing.T) {
	gs, _ := newCombatSummonTestState(t)
	ref, err := gs.enemyReferenceForCombat(references.MonsterOrc)
	require.NoError(t, err)
	for index := 0; index < sourceCombatQueueSlots-sourceMonsterSlotStart-1; index++ {
		monster := NewCombatMonster(
			references.MonsterOrc,
			ref,
			references.Position{X: references.Coordinate(index % 11), Y: references.Coordinate(index/11 + 8)},
			newFixedRNG(1),
		)
		gs.CombatScene.AddMonsterToQueue(monster)
	}
	rng := newStrictSummonRNG(t, 5, 5, 0)

	result := gs.summonCombatMonsters(
		references.InBetXen,
		references.MonsterInsect,
		inBetXenSummonCount,
		CombatMonsterLoyaltyParty,
		rng,
	)

	assert.Equal(t, CombatSummonSucceeded, result.Outcome)
	assert.Equal(t, 1, result.Count)
	assert.Equal(t, sourceCombatQueueSlots-sourceMonsterSlotStart, gs.CombatScene.MonsterManager.Count())
}

func TestCastCombatKalXenDoesNotMutateAtFullCapacity(t *testing.T) {
	gs, mock := newCombatSummonTestState(t)
	ref, err := gs.enemyReferenceForCombat(references.MonsterOrc)
	require.NoError(t, err)
	for index := range sourceCombatQueueSlots - sourceMonsterSlotStart {
		gs.CombatScene.AddMonsterToQueue(NewCombatMonster(
			references.MonsterOrc,
			ref,
			references.Position{X: references.Coordinate(index % 11), Y: references.Coordinate(index / 11)},
			newFixedRNG(1),
		))
	}
	rng := newStrictSummonRNG(t, 15, 5, 5)

	result := gs.castCombatKalXen(rng)

	assert.Equal(t, CombatSummonCapacityFull, result.Outcome)
	assert.Equal(t, sourceCombatQueueSlots-sourceMonsterSlotStart, gs.CombatScene.MonsterManager.Count())
	assert.Empty(t, mock.CombatEvents)
	assert.Equal(t, 3, rng.index, "Placement is attempted before combatant-capacity validation (ref:SPELL-0005)")
}

// REDUX DEVIATION: COMBAT-0001. A kill makes room: the summon takes the queue and
// object slots the dead monster's remains were holding, and the remains stay.
func TestCastCombatKalXenTakesTheSlotsOfADeadMonstersRemains(t *testing.T) {
	gs, _ := newCombatSummonTestState(t)
	ref, err := gs.enemyReferenceForCombat(references.MonsterOrc)
	require.NoError(t, err)
	for index := range sourceCombatQueueSlots - sourceMonsterSlotStart {
		gs.CombatScene.AddMonsterToQueue(NewCombatMonster(
			references.MonsterOrc,
			ref,
			references.Position{X: references.Coordinate(index % 11), Y: references.Coordinate(index / 11)},
			newFixedRNG(1),
		))
	}
	fillCombatObjectTableWithFields(t, gs.CombatScene, references.Position{X: 10, Y: 10})
	require.Equal(t, CombatSummonCapacityFull, gs.castCombatKalXen(newStrictSummonRNG(t, 15, 5, 5)).Outcome)

	victim := gs.CombatScene.MonsterManager.GetMonster(0)
	require.True(t, victim.TakeDamage(victim.CurrentHP()))
	gs.finalizeCombatMonsterDeath(victim, 0, gs.PartyState.GetAvatar(), 0)
	require.True(t, gs.CombatScene.HasObjectAt(victim.Position()))
	deadEntry := gs.CombatScene.TurnQueue.GetEntry(0, false)
	require.False(t, deadEntry.sourceSlotReleased, "remains hold the slot until something living needs it")
	require.Equal(t, sourceCombatQueueSlots, gs.CombatScene.originalObjectSlotCount())

	result := gs.castCombatKalXen(newStrictSummonRNG(t, 15, 5, 5, 0))

	require.Equal(t, CombatSummonSucceeded, result.Outcome)
	summoned := gs.CombatScene.TurnQueue.GetEntry(result.Created[0].Index, false)
	assert.Equal(t, deadEntry.sourceSlot, summoned.sourceSlot)
	assert.True(t, deadEntry.sourceSlotReleased)
	assert.NotZero(t, summoned.originalObjectSlot)
	assert.True(t, gs.CombatScene.HasObjectAt(victim.Position()), "the corpse or chest stays on the ground")
	assert.Equal(t, CombatSummonCapacityFull, gs.castCombatKalXen(newStrictSummonRNG(t, 15, 6, 6)).Outcome,
		"the fight is full of the living again")
}

func TestCombatSummonCapacityFailurePreservesCreateEntryRNGOrder(t *testing.T) {
	t.Run("queue full skips dex roll", func(t *testing.T) {
		gs, _ := newCombatSummonTestState(t)
		source := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
		for gs.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
			identity := gs.CombatScene.addPCCloneFromSource(
				source,
				source.EntityIndex,
				references.Position{X: 1, Y: 1},
				NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
			)
			require.NotZero(t, identity.Slot)
		}
		rng := newStrictSummonRNG(t, 5, 5)

		result := gs.summonCombatMonsters(
			references.InBetXen, references.MonsterInsect, 1, CombatMonsterLoyaltyParty, rng,
		)

		assert.Equal(t, CombatSummonCapacityFull, result.Outcome)
		assert.Equal(t, []int{combatSummonRandomCoordinate, combatSummonRandomCoordinate}, rng.requests)
	})

	t.Run("object full consumes dex roll", func(t *testing.T) {
		gs, _ := newCombatSummonTestState(t)
		fillCombatObjectTableWithFields(t, gs.CombatScene, references.Position{X: 1, Y: 1})
		rng := newStrictSummonRNG(t, 5, 5, 0)

		result := gs.summonCombatMonsters(
			references.InBetXen, references.MonsterInsect, 1, CombatMonsterLoyaltyParty, rng,
		)

		assert.Equal(t, CombatSummonCapacityFull, result.Outcome)
		assert.Equal(t, []int{combatSummonRandomCoordinate, combatSummonRandomCoordinate, 8}, rng.requests)
	})
}

func TestCastCombatKalXenCorpBindingBoundary(t *testing.T) {
	tests := []struct {
		name            string
		bindingRollZero int
		expectedBinding CombatSummonBindingOutcome
		expectedLoyalty CombatMonsterLoyalty
	}{
		{
			name:            "roll below intelligence binds",
			bindingRollZero: 38, expectedBinding: CombatSummonBoundToParty,
			expectedLoyalty: CombatMonsterLoyaltyParty,
		},
		{
			name:            "roll equal to intelligence reports oops",
			bindingRollZero: 40, expectedBinding: CombatSummonBindingFailed,
			expectedLoyalty: CombatMonsterLoyaltyMonster,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, mock := newCombatSummonTestState(t)
			casterIdx := 0
			gs.PartyState.GetCharacter(casterIdx).Intelligence = 20
			rng := newStrictSummonRNG(t, 5, 5, 0, test.bindingRollZero)

			result := gs.castCombatKalXenCorp(casterIdx, rng, func() int {
				return rollOriginalD30(rng)
			})

			require.Equal(t, CombatSummonSucceeded, result.Outcome)
			assert.Equal(t, test.expectedBinding, result.Binding)
			require.Len(t, result.Created, 1)
			monster := gs.CombatScene.MonsterManager.GetMonster(result.Created[0].Index)
			assert.Equal(t, references.MonsterDevil, monster.MonsterType())
			assert.Equal(t, test.expectedLoyalty, monster.Loyalty())
			require.Len(t, mock.CombatEvents, 1)
			assert.Equal(t, test.expectedBinding, mock.CombatEvents[0].Summon.Binding)
			assert.Equal(t, []int{16, 16, 8, originalD30SourceValueCount}, rng.requests)
		})
	}
}

func TestKalXenCorpScrollAlwaysBindsDaemon(t *testing.T) {
	gs, mock := newCombatSummonTestState(t)
	gs.SetRandomSeed(1)
	result := gs.UseCombatKalXenCorpScroll(0)

	require.Equal(t, CombatSummonSucceeded, result.Outcome)
	assert.Equal(t, CombatSummonBoundToParty, result.Binding)
	require.Len(t, result.Created, 1)
	monster := gs.CombatScene.MonsterManager.GetMonster(result.Created[0].Index)
	assert.Equal(t, CombatMonsterLoyaltyParty, monster.Loyalty())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatSummonBoundToParty, mock.CombatEvents[0].Summon.Binding)
}
