package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestCombatCompositionConsumesOriginalInterleavedFriendAndDexDraws(t *testing.T) {
	for _, test := range []struct {
		monster references.MonsterType
		friend  references.MonsterType
		count   int
		enabled bool
	}{
		{references.MonsterBat, references.MonsterBat, 1, true},
		{references.MonsterBat, references.MonsterBat, 3, true},
		{references.MonsterBat, references.MonsterBat, 4, true},
		{references.MonsterBat, references.MonsterBat, 16, true},
		{references.MonsterGuard, references.MonsterGuard, 8, true},
		{references.MonsterRat, references.MonsterSkeleton, 10, true},
		{references.MonsterRat, references.MonsterSkeleton, 16, false},
	} {
		t.Run(fmt.Sprintf("%s/count_%d/friends_%t", test.monster, test.count, test.enabled), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).Build()
			seed := uint64(86)
			request := newComposerRequest()
			request.CountMode = CombatEncounterCountExact
			request.ExactCount = test.count
			request.MonsterType = test.monster
			request.FriendsEnabled = test.enabled
			request.Seed = &seed
			result, err := state.StartCombatEncounter(request)
			require.NoError(t, err)
			assert.Equal(t, test.count, result.PrimaryCount+result.FriendCount)
			assertOriginalEncounterMonsterDraws(t, state, beradrng.New(seed), test.monster, test.friend, test.count, test.enabled)
		})
	}
}

// Independent original combat initialization oracle: do not call composer, friend,
// or monster-constructor helpers to derive expected draws and DEX values.
// See ref:ENEMY-0003.
func assertOriginalEncounterMonsterDraws(
	t *testing.T,
	state *GameState,
	oracle *beradrng.Source,
	primary, friend references.MonsterType,
	count int,
	friendsEnabled bool,
) {
	t.Helper()
	require.Equal(t, count, state.CombatScene.MonsterManager.Count())
	for index := range count {
		wantType := primary
		if friendsEnabled && index >= 1 && index <= count/4 {
			if oracle.Intn(9) == 0 { // Original command rules: bounded random draw is inclusive.
				// See ref:ENEMY-0003.
				wantType = friend
			}
		}
		modifier := oracle.Intn(8) - 4
		ref, err := state.enemyReferenceForCombat(wantType)
		require.NoError(t, err)
		wantDex := ref.Dexterity + modifier
		if wantDex < 0 || wantDex > 30 {
			wantDex = ref.Dexterity
		}
		monster := state.CombatScene.MonsterManager.GetMonster(index)
		assert.Equal(t, wantType, monster.MonsterType(), "monster %d type", index)
		assert.Equal(t, modifier, monster.DexterityModifier(), "monster %d modifier", index)
		assert.Equal(t, wantDex, monster.CurrentDex(), "monster %d DEX", index)
	}
	assert.Equal(t, oracle.Calls(), state.GetRNG().Calls(), "composition RNG consumption")
	assert.Equal(t, oracle.Intn(1000000), state.GetRNG().Intn(1000000), "next gameplay draw")
}

func TestStartCombatEncounterRejectsActiveCombatWithoutMutation(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).Build()
	require.NoError(t, gs.TransitionToCombatMap(
		int(references.GladeCombatMap),
		references.BritanniaCombatTerritory,
		CombatEntryNorth,
	))
	originalCombat := gs.CombatScene
	originalLocation := gs.MapState.PlayerLocation

	result, err := gs.StartCombatEncounter(newComposerRequest())

	assert.ErrorIs(t, err, ErrCombatEncounterAlreadyActive)
	assert.Nil(t, result)
	assert.Same(t, originalCombat, gs.CombatScene)
	assert.Equal(t, originalLocation, gs.MapState.PlayerLocation)
}

func TestStartCombatEncounterRejectsActiveGameplayWorkflow(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).Build()
	gs.EventQueueManager.Workflow("map_transition").
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {}).
		Finally(func() {}).
		Start()
	originalLocation := gs.MapState.PlayerLocation

	result, err := gs.StartCombatEncounter(newComposerRequest())

	assert.ErrorIs(t, err, ErrInvalidCombatEncounter)
	assert.Nil(t, result)
	assert.Nil(t, gs.CombatScene)
	assert.Equal(t, originalLocation, gs.MapState.PlayerLocation)
}

func TestStartCombatEncounterRejectsExactCountBoundsAtomically(t *testing.T) {
	for _, count := range []int{0, references.MaxMonstersInCombat + 1} {
		t.Run(string(rune(count+'0')), func(t *testing.T) {
			gs, _ := NewIntegrationTestBuilder(t).Build()
			control, _ := NewIntegrationTestBuilder(t).Build()
			gs.SetRandomSeed(71)
			control.SetRandomSeed(71)
			request := newComposerRequest()
			request.CountMode = CombatEncounterCountExact
			request.ExactCount = count
			seed := uint64(999)
			request.Seed = &seed
			originalLocation := gs.MapState.PlayerLocation

			result, err := gs.StartCombatEncounter(request)

			assert.ErrorIs(t, err, ErrInvalidCombatEncounter)
			assert.Nil(t, result)
			assert.Nil(t, gs.CombatScene)
			assert.Equal(t, originalLocation, gs.MapState.PlayerLocation)
			assert.Equal(t, control.RandomIntInRange(1, 1000), gs.RandomIntInRange(1, 1000),
				"failed preflight must neither reset nor advance the centralized RNG")
		})
	}
}

func TestStartCombatEncounterSeededCompositionIsRepeatable(t *testing.T) {
	first, _ := NewIntegrationTestBuilder(t).Build()
	second, _ := NewIntegrationTestBuilder(t).Build()
	seed := uint64(179)
	request := newComposerRequest()
	request.CountMode = CombatEncounterCountExact
	request.ExactCount = 8
	request.FriendsEnabled = true
	request.Seed = &seed

	firstResult, firstErr := first.StartCombatEncounter(request)
	secondResult, secondErr := second.StartCombatEncounter(request)

	require.NoError(t, firstErr)
	require.NoError(t, secondErr)
	assert.Equal(t, firstResult, secondResult)
	assert.NotSame(t, request.Seed, firstResult.Seed, "result must not retain caller-owned seed storage")
	assert.Equal(t, combatMonsterSignatures(first), combatMonsterSignatures(second))
}

func TestStartCombatEncounterAccountsForFriends(t *testing.T) {
	var gs *GameState
	var result *CombatEncounterResult
	for seed := uint64(0); seed < 100; seed++ {
		gs, _ = NewIntegrationTestBuilder(t).Build()
		request := newComposerRequest()
		request.CountMode = CombatEncounterCountExact
		request.ExactCount = 16
		request.FriendsEnabled = true
		request.Seed = &seed
		var err error
		result, err = gs.StartCombatEncounter(request)
		require.NoError(t, err)
		if result.FriendCount > 0 {
			break
		}
	}

	require.NotNil(t, result)
	assert.Positive(t, result.FriendCount, "seed fixture must exercise Orc friend replacement")
	assert.Equal(t, 16, result.PrimaryCount+result.FriendCount)
	assert.Equal(t, result.PrimaryCount,
		gs.CombatScene.MonsterManager.GetMonsterTypeCount(references.MonsterOrc))
	assert.Equal(t, result.FriendCount,
		gs.CombatScene.MonsterManager.GetMonsterTypeCount(references.MonsterTroll))
}

func TestStartCombatEncounterSynchronizesQueueAndReportsPositions(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).Build()
	request := newComposerRequest()
	request.CountMode = CombatEncounterCountExact
	request.ExactCount = 4

	result, err := gs.StartCombatEncounter(request)

	require.NoError(t, err)
	assert.Equal(t, references.BritanniaCombatTerritory, result.Territory)
	assert.Equal(t, int(references.GladeCombatMap), result.MapIndex)
	assert.Equal(t, request.ExactCount, gs.CombatScene.MonsterManager.Count())
	_, queuedMonsters := gs.CombatScene.TurnQueue.CountRemaining()
	assert.Equal(t, request.ExactCount, queuedMonsters)
	for index, position := range result.Positions {
		monster := gs.CombatScene.MonsterManager.GetMonster(index)
		require.NotNil(t, monster)
		assert.Equal(t, position, monster.Position())
		for partyIndex, active := range gs.CombatScene.PartyActive {
			if active {
				assert.NotEqual(t, gs.CombatScene.PartyPositions[partyIndex], position)
			}
		}
	}
}

func TestAutomaticCombatEncounterCountAppliesOpeningCampaignMonthReduction(t *testing.T) {
	actual := &GameState{}
	expected := &GameState{}
	actual.DateTime.Year, actual.DateTime.Month = 139, 4
	actual.openingCampaignEncounterReduction = true
	expected.DateTime = actual.DateTime
	actual.SetRandomSeed(179)
	expected.SetRandomSeed(179)
	request := newComposerRequest()
	const maximum = 6
	enemy := &references.EnemyReference{MaxPerMap: maximum}
	want := expected.RollDice(maximum)
	want = expected.RollDice(want)

	got := actual.resolveCombatEncounterCount(request, enemy, maximum)

	assert.Equal(t, want, got)
	assert.Equal(t, expected.GetRNG().Calls(), actual.GetRNG().Calls())
}

func TestAutomaticCombatEncounterCountUsesOriginalFixedNumericSizes(t *testing.T) {
	for _, test := range []struct {
		monster references.MonsterType
		maximum int
	}{
		{monster: references.MonsterVillager, maximum: 1},
		{monster: references.MonsterSkeleton, maximum: 8},
		{monster: references.MonsterHeadless, maximum: 8},
		{monster: references.MonsterBat, maximum: 16},
	} {
		t.Run(test.monster.String(), func(t *testing.T) {
			state := &GameState{}
			state.SetRandomSeed(484)
			request := newComposerRequest()
			request.MonsterType = test.monster
			reference := &references.EnemyReference{MaxPerMap: test.maximum}
			before := state.GetRNG().Calls()
			assert.Equal(t, test.maximum, state.resolveCombatEncounterCount(request, reference, 16))
			assert.Equal(t, before, state.GetRNG().Calls(), "fixed counts must not roll")
		})
	}
	actual, expected := &GameState{}, &GameState{}
	actual.SetRandomSeed(484)
	expected.SetRandomSeed(484)
	request := newComposerRequest()
	request.MonsterType = references.MonsterDragon
	reference := &references.EnemyReference{MaxPerMap: 2}
	assert.Equal(t, expected.RollDice(2), actual.resolveCombatEncounterCount(request, reference, 16))
	assert.Equal(t, expected.GetRNG().Calls(), actual.GetRNG().Calls(), "Dragons roll their original maximum of two")
}

func TestAuthoredDungeonRoomStartsWhatItsPreviewShows(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).Build()
	room := CombatEncounterMap{Territory: references.DungeonCombatTerritory, Dungeon: references.CovetousDungeon, DungeonRoom: 12}
	placement, placed := gs.DungeonRoomPlacement(room.Dungeon, room.DungeonRoom)
	require.True(t, placed)
	assert.Equal(t, DungeonRoomPlacement{Floor: 5, Cell: references.Position{X: 3, Y: 4}}, placement)
	preview, err := gs.CombatEncounterPreview(room)
	require.NoError(t, err)
	require.NotEmpty(t, preview.Triggers, "the graves are push triggers")
	monsters, objects := 0, 0
	for _, entity := range preview.Entities {
		if entity.Kind == CombatEncounterEntityMonster || entity.Kind == CombatEncounterEntityRandomMonster {
			monsters++
		} else {
			objects++
		}
		if entity.Position == (references.Position{X: 9, Y: 4}) {
			assert.True(t, entity.Concealed, "%s behind the grave starts hidden", entity.Name)
		}
	}

	result, err := gs.StartCombatEncounter(CombatEncounterRequest{Map: room, EntryDirection: CombatEntryWest, Authored: true})

	require.NoError(t, err)
	assert.Nil(t, gs.CombatScene.DungeonRoomOrigin, "a Workbench room leaves save progress untouched")
	assert.Equal(t, monsters, result.PrimaryCount)
	assert.Equal(t, monsters, gs.CombatScene.MonsterManager.Count())
	assert.Len(t, gs.CombatScene.objects, objects)
}

func newComposerRequest() CombatEncounterRequest {
	return CombatEncounterRequest{
		Map: CombatEncounterMap{
			Territory:    references.BritanniaCombatTerritory,
			BritanniaMap: references.GladeCombatMap,
		},
		EntryDirection: CombatEntryNorth,
		MonsterType:    references.MonsterOrc,
		CountMode:      CombatEncounterCountAutomatic,
		FriendsEnabled: false,
	}
}

type combatMonsterSignature struct {
	MonsterType references.MonsterType
	Position    references.Position
	Dexterity   int
}

func combatMonsterSignatures(gs *GameState) []combatMonsterSignature {
	signatures := make([]combatMonsterSignature, gs.CombatScene.MonsterManager.Count())
	gs.CombatScene.MonsterManager.ForEach(func(index int, monster *CombatMonster) {
		signatures[index] = combatMonsterSignature{
			MonsterType: monster.MonsterType(),
			Position:    monster.Position(),
			Dexterity:   monster.CurrentDex(),
		}
	})

	return signatures
}
