package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// newBridgeTrollTestState returns a state whose sixteen characters are all at
// the inn, so each test places exactly the party members it needs.
func newBridgeTrollTestState() *GameState {
	gs := &GameState{}
	gs.PartyState = *party_state.NewBlankPartyState()
	for i := range 16 {
		char := gs.PartyState.GetCharacter(i)
		char.PartyMap = party_state.PartyMapAtInn(references.Britain)
		char.SetStatus(party_state.Good)
	}

	return gs
}

// The toll is 99 - 3 * strength, unclamped for edited stats above the normal
// range. See ref:ENV-0004.
func TestCalculateTrollToll(t *testing.T) {
	for _, test := range []struct {
		strength byte
		want     int
	}{{10, 69}, {20, 39}, {30, 9}, {35, -6}} {
		gs := newBridgeTrollTestState()
		char := gs.PartyState.GetCharacter(0)
		char.PartyMap = party_state.PartyMapActive
		char.Strength = test.strength
		assert.Equal(t, test.want, gs.CalculateTrollToll(), "strength %d", test.strength)
	}
}

func TestCalculateTrollTollUsesFirstGoodOrPoisonedPartyMember(t *testing.T) {
	gs := newBridgeTrollTestState()
	first := gs.PartyState.GetCharacter(0)
	first.PartyMap = party_state.PartyMapActive
	first.SetStatus(party_state.Poisoned)
	first.Strength = 10
	second := gs.PartyState.GetCharacter(1)
	second.PartyMap = party_state.PartyMapActive
	second.Strength = 30

	assert.Equal(t, 69, gs.CalculateTrollToll())
}

// Paying succeeds only when gold covers the toll; a negative original toll adds
// gold. See ref:ENV-0004.
func TestPayTrollToll(t *testing.T) {
	for _, test := range []struct {
		name     string
		gold     uint16
		toll     int
		paid     bool
		wantGold uint16
	}{
		{name: "sufficient", gold: 100, toll: 50, paid: true, wantGold: 50},
		{name: "insufficient", gold: 30, toll: 50, paid: false, wantGold: 30},
		{name: "exact", gold: 50, toll: 50, paid: true, wantGold: 0},
		{name: "zero", gold: 100, toll: 0, paid: true, wantGold: 100},
		{name: "negative adds gold", gold: 100, toll: -6, paid: true, wantGold: 106},
	} {
		gs := newBridgeTrollTestState()
		gs.PartyState.Inventory.Gold.Set(test.gold)
		assert.Equal(t, test.paid, gs.PayTrollToll(test.toll), test.name)
		assert.Equal(t, test.wantGold, gs.PartyState.Inventory.Gold.Get(), test.name)
	}
}

// A sneak is caught when the 1-30 roll exceeds dexterity. ref:ENV-0004
func TestPerformSneakCheckCatchRateFollowsDexterity(t *testing.T) {
	for _, test := range []struct {
		dexterity            byte
		minCaught, maxCaught int
	}{{30, 0, 0}, {15, 35, 65}, {1, 90, 100}} {
		gs := newBridgeTrollTestState()
		gs.SetRandomSeed(12345)
		char := gs.PartyState.GetCharacter(0)
		char.Dexterity = test.dexterity
		caught := 0
		for range 100 {
			if gs.PerformSneakCheck(char) {
				caught++
			}
		}
		assert.GreaterOrEqual(t, caught, test.minCaught, "dexterity %d", test.dexterity)
		assert.LessOrEqual(t, caught, test.maxCaught, "dexterity %d", test.dexterity)
	}
}

// Sneak participants are active members who are neither Dead nor Sleeping.
// ref:ENV-0004
func TestGetLivingAwakePartyMembers(t *testing.T) {
	inn := party_state.PartyMapAtInn(references.Britain)
	for _, test := range []struct {
		name     string
		maps     [3]party_state.PartyMap
		statuses [3]party_state.CharacterStatus
		want     int
	}{
		{name: "all alive", want: 3},
		{name: "one dead", statuses: [3]party_state.CharacterStatus{2: party_state.Dead}, want: 2},
		{name: "one asleep", statuses: [3]party_state.CharacterStatus{1: party_state.Sleep}, want: 2},
		{name: "one at the inn", maps: [3]party_state.PartyMap{1: inn}, want: 2},
		{
			name:     "all dead",
			statuses: [3]party_state.CharacterStatus{party_state.Dead, party_state.Dead, party_state.Dead},
			want:     0,
		},
	} {
		gs := newBridgeTrollTestState()
		for i := range 3 {
			char := gs.PartyState.GetCharacter(i)
			char.PartyMap = test.maps[i] // The zero PartyMap is the active roster.
			if test.statuses[i] != 0 {
				char.SetStatus(test.statuses[i])
			}
		}
		assert.Len(t, gs.GetLivingAwakePartyMembers(), test.want, test.name)
	}
}

func TestQueueBridgeTrollCombatWaitsForWorkflowThenStartsSourceLessEncounter(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	gs.SetRandomSeed(95)
	before := gs.MapState.PlayerLocation
	gs.EventQueueManager.Workflow("bridge_dialog").
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().
		First(func() {}).
		Finally(func() {}).
		Start()
	gs.QueueBridgeTrollCombat()

	gs.FinishTurn()

	assert.Nil(t, gs.CombatScene)
	assert.True(t, gs.HasPendingOverworldContact())
	for gs.EventQueueManager.HasActiveWorkflows() {
		gs.ElapsedMs++
		gs.EventQueueManager.Update()
	}
	require.NoError(t, gs.ResolvePendingOverworldContact())
	require.NotNil(t, gs.CombatScene)
	assert.False(t, gs.HasPendingOverworldContact())
	assert.Equal(t, int(references.BigBridgeCombatMap), gs.CombatScene.CombatMapRef.MapIndex)
	assert.Nil(t, gs.CombatScene.OverworldOrigin)
	assert.GreaterOrEqual(t, gs.CombatScene.MonsterManager.Count(), 1)
	assert.LessOrEqual(t, gs.CombatScene.MonsterManager.Count(), 4)
	assert.Equal(t, references.MonsterTroll, gs.CombatScene.MonsterManager.GetMonster(0).MonsterType())

	gs.EndCombat(CombatVictory)
	assert.Equal(t, before, gs.MapState.PlayerLocation)
}

func TestBridgeTrollCombatReturnsToBridgeForEveryOutcome(t *testing.T) {
	for _, testCase := range []struct {
		name   string
		reason CombatEndReason
	}{
		{name: "victory", reason: CombatVictory},
		{name: "defeat", reason: CombatDefeat},
		{name: "evacuated", reason: CombatEvacuated},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			gs, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(42, 73).
				WithSystemCallbacks().
				Build()
			before := gs.MapState.PlayerLocation
			gs.SetRandomSeed(95)
			gs.QueueBridgeTrollCombat()

			require.NoError(t, gs.ResolvePendingOverworldContact())
			require.NotNil(t, gs.CombatScene)
			gs.EndCombat(testCase.reason)

			assert.Nil(t, gs.CombatScene)
			assert.Equal(t, before, gs.MapState.PlayerLocation)
			assert.False(t, gs.HasPendingOverworldContact())
		})
	}
}
