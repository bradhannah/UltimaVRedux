package game_state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func newCombatSoloResetTestState(t *testing.T, condition string) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := newCombatPlayerSelectionTestState(t)
	combat := state.CombatScene
	selected := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}
	combat.SetSoloMoveCharacter(selected.EntityIndex)
	switch condition {
	case "sleep":
		combat.setCombatPCSleeping(selected, true)
	case "captive":
		combat.setCombatPCUnderground(selected, true)
	case "possessed":
		combat.setCombatPCPossessed(selected, true)
	case "enemy":
		_, monsterIndex := addControlledTestMonster(state, references.MonsterOrc,
			references.Position{X: 5, Y: 5}, CombatMonsterLoyaltyMonster)
		combat.TurnQueue.GetEntry(monsterIndex, false).turnCountdown = 1
		combat.SetSoloMoveCharacter(0)
	default:
		t.Fatalf("unknown condition %q", condition)
	}
	state.FinishCombatAction()
	require.True(t, combat.HasScheduledCombatStep())
	callbacks.Reset()

	return state, callbacks
}

func TestCombatSoloResetPreservesScheduledActionAndCostsNothing(t *testing.T) {
	for _, condition := range []string{"enemy", "sleep", "captive", "possessed"} {
		t.Run(condition, func(t *testing.T) {
			state, callbacks := newCombatSoloResetTestState(t, condition)
			combat := state.CombatScene
			before := *combat
			pending := *combat.pendingPresentationStep
			entries := make([]TurnQueueEntry, len(combat.TurnQueue.entries))
			for index, entry := range combat.TurnQueue.entries {
				entries[index] = *entry
			}
			count, cursor := combat.TurnQueue.actionCount, combat.TurnQueue.currentIndex
			inventory, spells := state.PartyState.Inventory.LockerSnapshot(), state.PartyState.Inventory.SpellSnapshot()
			date, elapsed, duration := state.DateTime, state.ElapsedMs, state.ActiveDurationEffect
			calls := state.GetRNG().Calls()
			expectedRNG := beradrng.Restore(state.GetRNG().Seed(), calls)

			boundary, eligible := state.ProjectCombatSoloReset()
			require.True(t, eligible)
			require.False(t, boundary.Claimed, "query must not claim presentation")
			require.Equal(t, pending.step, boundary.Step)
			require.True(t, state.ResetCombatSoloDuringWait())
			before.SoloMoveMode = false
			require.Equal(t, before, *combat, "only the solo filter changes inside combat")
			require.Equal(t, -1, state.activePlayerIndex)
			require.Equal(t, pending, *combat.pendingPresentationStep)
			require.Equal(t, count, combat.TurnQueue.actionCount)
			require.Equal(t, cursor, combat.TurnQueue.currentIndex)
			for index, entry := range combat.TurnQueue.entries {
				require.Equal(t, entries[index], *entry, "countdown/dispatch identity is preserved")
			}
			require.Equal(t, inventory, state.PartyState.Inventory.LockerSnapshot())
			require.Equal(t, spells, state.PartyState.Inventory.SpellSnapshot())
			require.Equal(t, date, state.DateTime)
			require.Equal(t, elapsed, state.ElapsedMs)
			require.Equal(t, duration, state.ActiveDurationEffect)
			require.Equal(t, calls, state.GetRNG().Calls())
			require.Empty(t, callbacks.CombatEvents)
			require.Empty(t, callbacks.TimeAdvanced())
			require.Empty(t, callbacks.CommandPrompts)
			require.Equal(t, []string{"Set active player: None!"}, callbacks.Messages)
			require.True(t, state.ResetCombatSoloDuringWait())
			require.Equal(t, []string{"Set active player: None!"}, callbacks.Messages, "all-party reset is idempotent")
			require.Equal(t, expectedRNG.Intn(1<<20), state.GetRNG().Intn(1<<20))
			step, claimed := state.ReviewScheduledCombatStep()
			require.True(t, claimed, "query/reset leave the real owner free to claim once")
			require.Equal(t, boundary.Step, step)
			boundary, eligible = state.ProjectCombatSoloReset()
			require.True(t, eligible)
			require.True(t, boundary.Claimed)
		})
	}
}

func TestCombatSoloResetRejectsOtherBoundaries(t *testing.T) {
	for _, context := range []string{"outside", "unstarted", "immediate", "ready", "player completion", "targeting", "replaced encounter"} {
		t.Run(context, func(t *testing.T) {
			state, callbacks := newCombatSoloResetTestState(t, "enemy")
			combat := state.CombatScene
			switch context {
			case "outside":
				state.CombatScene = nil
			case "unstarted":
				combat.turnsStarted = false
			case "immediate":
				combat.presentTurns = false
			case "ready":
				combat.pendingPresentationStep = nil
			case "player completion":
				combat.pendingPresentationStep = nil
				state.FinishCombatTurnDeferred()
			case "targeting":
				combat.AttackTargeting = &CombatAttackTargeting{}
			case "replaced encounter":
				combat.EncounterID++
			}
			calls, count := state.GetRNG().Calls(), combat.TurnQueue.actionCount
			_, eligible := state.ProjectCombatSoloReset()
			require.False(t, eligible)
			require.False(t, state.ResetCombatSoloDuringWait())
			require.True(t, combat.IsSoloMoveActive())
			require.Equal(t, calls, state.GetRNG().Calls())
			require.Equal(t, count, combat.TurnQueue.actionCount)
			require.Empty(t, callbacks.CurrentRowContent)
			require.Empty(t, callbacks.Messages)
		})
	}
}

func TestCombatSoloResetKeepsSpentSkipsAndNextDEXActor(t *testing.T) {
	state, _ := newCombatSoloResetTestState(t, "enemy")
	combat := state.CombatScene
	spent := combat.TurnQueue.actionCount
	step, claimed := state.ReviewScheduledCombatStep()
	require.True(t, claimed)
	_, err := state.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)
	state.ResolvePendingMonsterMissile()
	// PC 1 is due before the former solo selection, PC 0. The enemy's spent
	// action and earlier synchronous solo skips must not be replayed.
	combat.TurnQueue.GetEntry(0, true).turnCountdown = 2
	combat.TurnQueue.GetEntry(1, true).turnCountdown = 1
	require.True(t, state.ResetCombatSoloDuringWait())
	require.Equal(t, spent, combat.TurnQueue.actionCount)
	require.NoError(t, state.CompleteScheduledCombatStep(step.Token))
	require.Equal(t, CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}, combat.GetCurrentTurnIdentity())
	require.Equal(t, spent+1, combat.TurnQueue.actionCount)
	require.ErrorIs(t, state.CompleteScheduledCombatStep(step.Token), ErrCombatPresentationDuplicate)
	require.Equal(t, spent+1, combat.TurnQueue.actionCount)
}

func TestCombatSoloResetDoesNotJumpPastAnotherDueHostile(t *testing.T) {
	state, _ := newCombatSoloResetTestState(t, "enemy")
	combat := state.CombatScene
	step, claimed := state.ReviewScheduledCombatStep()
	require.True(t, claimed)
	_, err := state.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)
	state.ResolvePendingMonsterMissile()
	_, nextMonster := addControlledTestMonster(state, references.MonsterRat,
		references.Position{X: 1, Y: 1}, CombatMonsterLoyaltyMonster)
	for _, entry := range combat.TurnQueue.entries {
		entry.turnCountdown = 3
	}
	combat.TurnQueue.GetEntry(nextMonster, false).turnCountdown = 1
	before := combat.TurnQueue.actionCount
	require.True(t, state.ResetCombatSoloDuringWait())
	require.Equal(t, before, combat.TurnQueue.actionCount)
	require.NoError(t, state.CompleteScheduledCombatStep(step.Token))
	boundary, eligible := state.ProjectCombatSoloReset()
	require.True(t, eligible)
	require.Equal(t, CombatPresentationMonsterTurn, boundary.Step.Kind)
	require.Equal(t, CombatTurnIdentity{EntityIndex: nextMonster}, boundary.Step.Actor)
	require.Equal(t, before+1, combat.TurnQueue.actionCount)
}

func TestCombatSoloResetReadySelectionRemainsFreeAndIdempotent(t *testing.T) {
	state, callbacks := newCombatPlayerSelectionTestState(t)
	require.True(t, state.SelectCombatPlayer(0))
	callbacks.Reset()
	combat := state.CombatScene
	actor, count, calls := combat.GetCurrentTurnIdentity(), combat.TurnQueue.actionCount, state.GetRNG().Calls()
	require.True(t, state.SelectCombatPlayer(-1))
	require.True(t, state.SelectCombatPlayer(-1))
	require.False(t, combat.IsSoloMoveActive())
	require.Equal(t, actor, combat.GetCurrentTurnIdentity())
	require.Equal(t, count, combat.TurnQueue.actionCount)
	require.Equal(t, calls, state.GetRNG().Calls())
	require.Equal(t, []string{"Set active player: None!"}, callbacks.Messages)
	require.Empty(t, callbacks.CombatEvents)
}
