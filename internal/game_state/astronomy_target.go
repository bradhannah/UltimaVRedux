package game_state

import "github.com/bradhannah/UltimaVRedux/internal/party_state"

// TelescopeSunDamage is the damage of looking at the sun through a telescope.
const TelescopeSunDamage = 1

// TelescopeSunDamageTarget preserves an explicit designation and otherwise
// chooses the first Good/Poisoned party member (ref:ASTRO-0003).
func (g *GameState) TelescopeSunDamageTarget() int {
	if g.activePlayerIndex >= 0 {
		return g.activePlayerIndex
	}
	target := -1
	g.PartyState.ForEachCharacter(func(index int, character *party_state.PlayerCharacter) {
		if target < 0 && character.IsInParty() && (character.IsGood() || character.IsPoisoned()) {
			target = index
		}
	})

	return target
}
