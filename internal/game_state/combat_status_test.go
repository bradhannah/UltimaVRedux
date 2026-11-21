package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestUnpossessedSadujCopiesUseMonsterConditionRouting(t *testing.T) {
	for _, useClone := range []bool{false, true} {
		for _, underground := range []bool{false, true} {
			for _, wake := range []bool{false, true} {
				t.Run(fmt.Sprintf("clone=%t/underground=%t/wake=%t", useClone, underground, wake), func(t *testing.T) {
					gs, mock, source, clone := setupSadujCombatCopies(t)
					identity, other := source, clone
					if useClone {
						identity, other = clone, source
					}
					gs.setCombatPCSleeping(identity, true)
					gs.CombatScene.setCombatPCUnderground(identity, underground)
					seed := findCombatWakeSeed(t, gs, combatMonsterWakeRollMaximum, func(roll int) bool {
						return (roll == combatMonsterWakeRollMaximum) == wake
					})
					gs.SetRandomSeed(seed)
					mock.Reset()

					assert.Equal(t, combatTurnSkipped, gs.processCombatTurnStart(gs.CombatScene.TurnQueue.GetEntryByIdentity(identity)))

					assert.False(t, gs.CombatScene.isCombatPCPossessed(identity))
					assert.False(t, gs.CombatScene.isCombatPCSleeping(other))
					assert.Equal(t, underground, gs.CombatScene.isCombatPCUnderground(identity))
					assert.Equal(t, underground || !wake, gs.CombatScene.isCombatPCSleeping(identity))
					if underground {
						assert.Zero(t, gs.GetRNG().Calls(), "monster-side UNDER_MASK gate precedes sleep and regurgitation")
						assert.Empty(t, mock.CombatEvents)
						assert.Empty(t, mock.Messages)
					} else {
						assert.Equal(t, uint64(1), gs.GetRNG().Calls())
						actualNext := gs.RandomIntInRange(0, 10000)
						gs.SetRandomSeed(seed)
						gs.RandomIntInRange(0, combatMonsterWakeRollMaximum)
						assert.Equal(t, gs.RandomIntInRange(0, 10000), actualNext)
						require.Len(t, mock.CombatEvents, 1)
						assert.Equal(t, identity.Slot, mock.CombatEvents[0].Target.Slot)
					}
				})
			}
		}
	}
}

func TestCombatPCTurnStartSleepUsesOriginalRollAndSkipsTurn(t *testing.T) {
	tests := []struct {
		name              string
		wake              bool
		expectedStatus    party_state.CharacterStatus
		expectedCondition CombatConditionResult
	}{
		{
			name:              "remains asleep",
			wake:              false,
			expectedStatus:    party_state.Sleep,
			expectedCondition: CombatConditionSleeping,
		},
		{
			name:              "wakes but loses current turn",
			wake:              true,
			expectedStatus:    party_state.Good,
			expectedCondition: CombatConditionWoke,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, mock := setupCombatStatusTest(t)

			pcIndex := firstActiveCombatPC(gs)
			character := gs.PartyState.GetCharacter(pcIndex)
			gs.setCombatPCSleeping(CombatTurnIdentity{IsPartyMember: true, EntityIndex: pcIndex}, true)
			seed := findCombatWakeSeed(t, gs, combatPCWakeRollMaximum, func(roll int) bool {
				return (roll < combatPCWakeThreshold) == tt.wake
			})
			entry := gs.CombatScene.TurnQueue.GetEntry(pcIndex, true)
			require.NotNil(t, entry)
			mock.Reset()
			gs.SetRandomSeed(seed)

			result := gs.processCombatTurnStart(entry)
			actualNextRoll := gs.RandomIntInRange(0, 10000)

			gs.SetRandomSeed(seed)
			gs.RandomIntInRange(0, combatPCWakeRollMaximum)
			expectedNextRoll := gs.RandomIntInRange(0, 10000)

			assert.Equal(t, combatTurnSkipped, result)
			assert.Equal(t, tt.expectedStatus, character.GetStatus())
			assert.Equal(t, expectedNextRoll, actualNextRoll, "sleep turn must consume exactly one RNG roll")
			assert.Equal(t, []string{"Zzzzz..."}, mock.Messages)
			require.Len(t, mock.CombatEvents, 1)
			assert.Equal(t, CombatEventCondition, mock.CombatEvents[0].Kind)
			assert.Equal(t, tt.expectedCondition, mock.CombatEvents[0].Condition)
			assert.True(t, mock.CombatEvents[0].Target.IsPartyMember)
			assert.Equal(t, pcIndex, mock.CombatEvents[0].Target.Index)
		})
	}
}

func TestCombatMonsterTurnStartSleepUsesOriginalRollAndSkipsTurn(t *testing.T) {
	tests := []struct {
		name              string
		wake              bool
		expectedCondition CombatConditionResult
	}{
		{name: "remains asleep", wake: false, expectedCondition: CombatConditionSleeping},
		{name: "wakes but loses current turn", wake: true, expectedCondition: CombatConditionWoke},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

			monster.SetAsleep(true)
			seed := findCombatWakeSeed(t, gs, combatMonsterWakeRollMaximum, func(roll int) bool {
				return (roll == combatMonsterWakeRollMaximum) == tt.wake
			})
			entry := gs.CombatScene.TurnQueue.GetEntry(0, false)
			require.NotNil(t, entry)
			mock.Reset()
			gs.SetRandomSeed(seed)

			result := gs.processCombatTurnStart(entry)
			actualNextRoll := gs.RandomIntInRange(0, 10000)

			gs.SetRandomSeed(seed)
			gs.RandomIntInRange(0, combatMonsterWakeRollMaximum)
			expectedNextRoll := gs.RandomIntInRange(0, 10000)

			assert.Equal(t, combatTurnSkipped, result)
			assert.Equal(t, !tt.wake, monster.IsAsleep())
			assert.Equal(t, expectedNextRoll, actualNextRoll, "sleep turn must consume exactly one RNG roll")
			assert.Empty(t, mock.Messages)
			require.Len(t, mock.CombatEvents, 1)
			assert.Equal(t, CombatEventCondition, mock.CombatEvents[0].Kind)
			assert.Equal(t, tt.expectedCondition, mock.CombatEvents[0].Condition)
			assert.False(t, mock.CombatEvents[0].Target.IsPartyMember)
		})
	}
}

func TestPossessedPCSleepUsesMonsterTurnWakeRoll(t *testing.T) {
	gs, mock := setupCombatStatusTest(t)
	identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: firstActiveCombatPC(gs)}
	gs.CombatScene.setCombatPCPossessed(identity, true)
	gs.setCombatPCSleeping(identity, true)
	seed := findCombatWakeSeed(t, gs, combatMonsterWakeRollMaximum, func(roll int) bool {
		return roll == combatMonsterWakeRollMaximum
	})
	entry := gs.CombatScene.TurnQueue.GetEntryByIdentity(identity)
	require.NotNil(t, entry)
	mock.Reset()
	gs.SetRandomSeed(seed)

	result := gs.processCombatTurnStart(entry)

	assert.Equal(t, combatTurnSkipped, result)
	assert.False(t, gs.CombatScene.isCombatPCSleeping(identity))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatConditionWoke, mock.CombatEvents[0].Condition)
}

func TestSleepingChaosSwordWielderUsesMonsterWakeRule(t *testing.T) {
	gs, mock := setupCombatStatusTest(t)
	identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: firstActiveCombatPC(gs)}
	gs.PartyState.GetCharacter(identity.EntityIndex).SetEquippedItem(references.SlotShield, references.ChaosSword)
	gs.setCombatPCSleeping(identity, true)
	seed := findCombatWakeSeed(t, gs, combatMonsterWakeRollMaximum, func(roll int) bool {
		return roll != combatMonsterWakeRollMaximum
	})
	entry := gs.CombatScene.TurnQueue.GetEntryByIdentity(identity)
	require.NotNil(t, entry)
	mock.Reset()
	gs.SetRandomSeed(seed)

	result := gs.processCombatTurnStart(entry)

	assert.Equal(t, combatTurnSkipped, result)
	assert.True(t, gs.CombatScene.isCombatPCPossessed(identity))
	assert.True(t, gs.CombatScene.isCombatPCSleeping(identity))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatConditionSleeping, mock.CombatEvents[0].Condition)
}

func TestAdvanceCombatTurnSleepingPCSchedulesNonActionableTurn(t *testing.T) {
	gs, mock := setupCombatStatusTest(t)

	pcIndex := firstActiveCombatPC(gs)
	gs.setCombatPCSleeping(CombatTurnIdentity{IsPartyMember: true, EntityIndex: pcIndex}, true)
	forceNextCombatActor(gs.CombatScene.TurnQueue, pcIndex, true)
	seed := findCombatWakeSeed(t, gs, combatPCWakeRollMaximum, func(roll int) bool {
		return roll >= combatPCWakeThreshold
	})
	gs.SetRandomSeed(seed)
	gs.CombatScene.presentTurns = true
	mock.Reset()

	gs.AdvanceCombatTurn()
	step, scheduled := gs.ReviewScheduledCombatStep()

	require.True(t, scheduled)
	assert.Equal(t, CombatPresentationAutoPass, step.Kind)
	assert.Equal(t, pcIndex, step.Actor.EntityIndex)
	assert.Equal(t, pcIndex, gs.CombatScene.CurrentTurnIndex)
	assert.Equal(t, -1, gs.GetActivePlayerIndex())
	assert.Empty(t, mock.CommandPrompts, "sleeping PC must not receive an actionable prompt")
	conditionEvent := lastCombatConditionEvent(t, mock.CombatEvents)
	assert.Equal(t, CombatConditionSleeping, conditionEvent.Condition)
}

func TestAdvanceCombatTurnSleepingMonsterSchedulesIdleTurn(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	monster.SetAsleep(true)
	forceNextCombatActor(gs.CombatScene.TurnQueue, 0, false)
	seed := findCombatWakeSeed(t, gs, combatMonsterWakeRollMaximum, func(roll int) bool {
		return roll != combatMonsterWakeRollMaximum
	})
	gs.SetRandomSeed(seed)
	gs.CombatScene.presentTurns = true
	originalPosition := monster.Position()
	mock.Reset()

	gs.AdvanceCombatTurn()
	step, scheduled := gs.ReviewScheduledCombatStep()
	require.True(t, scheduled)
	_, err := gs.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)

	assert.Equal(t, 0, step.MonsterIndex)
	assert.False(t, step.HasTarget)
	assert.Equal(t, originalPosition, monster.Position())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatConditionSleeping, mock.CombatEvents[0].Condition)
}

func TestSleepingScheduledTurnAdvancesCombatTimeAtTenthAction(t *testing.T) {
	gs, _ := setupCombatStatusTest(t)

	pcIndex := firstActiveCombatPC(gs)
	gs.setCombatPCSleeping(CombatTurnIdentity{IsPartyMember: true, EntityIndex: pcIndex}, true)
	forceNextCombatActor(gs.CombatScene.TurnQueue, pcIndex, true)
	gs.CombatScene.TurnQueue.actionCount = actionsPerTimeAdvance - 1
	seed := findCombatWakeSeed(t, gs, combatPCWakeRollMaximum, func(roll int) bool {
		return roll >= combatPCWakeThreshold
	})
	gs.SetRandomSeed(seed)
	gs.CombatScene.presentTurns = true
	turnBefore := gs.DateTime.Turn

	gs.AdvanceCombatTurn()

	assert.Equal(t, turnBefore+1, gs.DateTime.Turn,
		"the tenth scheduled actor advances time even when sleep consumes the action")
	assert.Zero(t, gs.CombatScene.TurnQueue.actionCount)
}

func TestCombatPoisonedPCRemainsActionableWithoutTurnDamageOrRNG(t *testing.T) {
	gs, mock := setupCombatStatusTest(t)

	pcIndex := firstActiveCombatPC(gs)
	character := gs.PartyState.GetCharacter(pcIndex)
	character.SetStatus(party_state.Poisoned)
	hpBefore := character.GetCurrentHp()
	forceNextCombatActor(gs.CombatScene.TurnQueue, pcIndex, true)
	const seed = uint64(181)
	gs.SetRandomSeed(seed)
	mock.Reset()

	gs.AdvanceCombatTurn()
	actualNextRoll := gs.RandomIntInRange(0, 10000)

	gs.SetRandomSeed(seed)
	expectedNextRoll := gs.RandomIntInRange(0, 10000)
	assert.Equal(t, pcIndex, gs.GetActivePlayerIndex())
	assert.Equal(t, hpBefore, character.GetCurrentHp())
	assert.True(t, character.IsPoisoned())
	assert.Equal(t, expectedNextRoll, actualNextRoll, "poisoned turns consume no status RNG")
	assert.NotEmpty(t, mock.CommandPrompts)
}

func TestProcessTurnEffectsDoesNotRunTownAutoWakeDuringCombat(t *testing.T) {
	gs, _ := setupCombatStatusTest(t)

	pcIndex := firstActiveCombatPC(gs)
	character := gs.PartyState.GetCharacter(pcIndex)
	character.SetStatus(party_state.Sleep)
	seed := findCombatWakeSeed(t, gs, combatPCWakeRollMaximum, func(roll int) bool {
		return roll < combatPCWakeThreshold
	})
	gs.SetRandomSeed(seed)

	gs.ProcessTurnEffects()
	actualFirstRoll := gs.RandomIntInRange(0, combatPCWakeRollMaximum)

	gs.SetRandomSeed(seed)
	expectedFirstRoll := gs.RandomIntInRange(0, combatPCWakeRollMaximum)
	assert.True(t, character.IsAsleep())
	assert.Equal(t, expectedFirstRoll, actualFirstRoll, "combat turn effects must not consume a wake roll")
}

func setupCombatStatusTest(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()

	gs, mock := newBritainCombatTestState(t, 42)
	require.NotNil(t, gs.CombatScene)
	mock.Reset()

	return gs, mock
}

func firstActiveCombatPC(gs *GameState) int {
	for i := range party_state.MaxActiveParty {
		if gs.CombatScene.IsPartyMemberActive(i) {
			return i
		}
	}

	return NoTargetPC
}

func forceNextCombatActor(queue *CombatTurnQueue, entityIndex int, isPC bool) {
	for _, entry := range queue.entries {
		entry.turnCountdown = turnCountdownBase
		if entry.EntityIndex == entityIndex && entry.Combatant.IsPC() == isPC {
			entry.turnCountdown = 1
		}
	}
	queue.currentIndex = 0
}

func findCombatWakeSeed(
	t *testing.T,
	gs *GameState,
	maximum int,
	matches func(int) bool,
) uint64 {
	t.Helper()

	for seed := uint64(0); seed < 1000; seed++ {
		gs.SetRandomSeed(seed)
		if matches(gs.RandomIntInRange(0, maximum)) {
			return seed
		}
	}

	t.Fatalf("no matching wake seed for range 0..%d", maximum)
	return 0
}

func lastCombatConditionEvent(t *testing.T, events []CombatEvent) CombatEvent {
	t.Helper()

	for i := len(events) - 1; i >= 0; i-- {
		if events[i].Kind == CombatEventCondition {
			return events[i]
		}
	}

	t.Fatal("no combat condition event published")
	return CombatEvent{}
}

func TestRegurgitatedPCLandsOnNearestFreeSquareWhenItsSquareWasTaken(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 7)
	clearCombatMonstersForTest(gs.CombatScene)
	swallowed := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	stepper := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}
	for i := 2; i < len(gs.CombatScene.PartyActive); i++ {
		gs.CombatScene.PartyActive[i] = false
	}
	// Find two horizontally adjacent squares the party can stand on.
	var home, beside references.Position
	for y := range references.Coordinate(references.YCombatMapTiles) {
		for x := range references.Coordinate(references.XCombatMapTiles - 1) {
			home, beside = references.Position{X: x, Y: y}, references.Position{X: x + 1, Y: y}
			if gs.isCombatPositionValidForPC(&home, stepper) && gs.isCombatPositionValidForPC(&beside, stepper) {
				break
			}
		}
		if gs.isCombatPositionValidForPC(&home, stepper) && gs.isCombatPositionValidForPC(&beside, stepper) {
			break
		}
	}
	gs.CombatScene.SetPartyMemberPosition(0, home)
	gs.CombatScene.SetPartyMemberPosition(1, beside)
	gs.CombatScene.setCombatPCUnderground(swallowed, true)
	// The original ignores swallowed members, so the other member may step onto the square.
	gs.CombatScene.setCombatantPosition(stepper, home)

	for seed := uint64(1); seed < 200 && gs.CombatScene.isCombatPCUnderground(swallowed); seed++ {
		gs.SetRandomSeed(seed)
		gs.processUndergroundPCTurnStart(swallowed)
	}

	require.False(t, gs.CombatScene.isCombatPCUnderground(swallowed))
	landed := gs.CombatScene.GetCombatantPosition(swallowed)
	assert.NotEqual(t, home, landed)
	assert.Equal(t, home, gs.CombatScene.GetCombatantPosition(stepper))
	assert.LessOrEqual(t, absoluteInt(int(landed.X-home.X))+absoluteInt(int(landed.Y-home.Y)), 2)
}
