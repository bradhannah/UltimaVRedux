package game_state

// crystalBallDie is the sides of the die rolled against Intelligence.
const crystalBallDie = 30

// CrystalBallDeathVisionDamage is the damage of a failed crystal ball look.
const CrystalBallDeathVisionDamage = 1

// CrystalBallLook rolls a character's look into the crystal ball and reports
// whether they see a strange vision. The roll is one gameplay RNG draw, and
// the look succeeds when it is below the character's Intelligence; a failed
// look is a death vision costing CrystalBallDeathVisionDamage, which the
// caller applies.
func (g *GameState) CrystalBallLook(characterIndex int) bool {
	character := g.PartyState.GetCharacter(characterIndex)

	return g.RollDice(crystalBallDie) < int(character.Intelligence)
}
