package game_state

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCrystalBallLookSucceedsOnlyBelowIntelligence(t *testing.T) {
	const seed = 499
	probe, _ := NewIntegrationTestBuilder(t).WithRandomSeed(seed).Build()
	roll := probe.RollDice(crystalBallDie)
	for intelligence, want := range map[int]bool{roll: false, roll + 1: true} {
		state, _ := NewIntegrationTestBuilder(t).WithRandomSeed(seed).Build()
		state.PartyState.GetCharacter(0).Intelligence = byte(intelligence)
		require.Equal(t, want, state.CrystalBallLook(0), "roll %d, intelligence %d", roll, intelligence)
	}
}
