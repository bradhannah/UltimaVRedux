package game_state

import "github.com/bradhannah/UltimaVRedux/internal/party_state"

// jimmyAlwaysSucceeds is a Dexterity no lockpicking roll can beat.
const jimmyAlwaysSucceeds = 255

// setJimmyOutcome settles every lockpicking attempt by Dexterity alone, whatever
// the dice show: the highest Dexterity always succeeds and zero always fails on
// doors and restraints. A chest also needs a treasure value of 30 to always fail.
func setJimmyOutcome(state *GameState, succeeds bool) {
	dexterity := byte(0)
	if succeeds {
		dexterity = jimmyAlwaysSucceeds
	}
	for index := range party_state.MaxCharactersInParty {
		state.PartyState.GetCharacter(index).Dexterity = dexterity
	}
}
