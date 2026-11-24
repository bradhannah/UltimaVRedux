package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

const (
	chestSafeMessage        = "The chest is safe."
	chestUncertainMessage   = "Thou canst not determine whether this chest is trapped. Opening may be dangerous."
	chestAlreadySafeMessage = "Already safe - nothing to disarm."
	chestDisarmedMessage    = "Trap disarmed!"
)

func (g *GameState) inspectObjectChest(assessment references.ChestAssessment, armed bool, value, intelligence int) references.ChestAssessment {
	if assessment != references.ChestUninspected {
		return assessment
	}
	difficulty := (combatTreasureDiceSides - intelligence) / combatChestDifficultyDivisor
	if armed {
		difficulty = (combatTreasureDiceSides + value - intelligence) / combatChestDifficultyDivisor
	}
	// REDUX DEVIATION: COMBAT-0010: object chest inspection rerolls and can lie on every Search.
	// Keep its first INT check, but remember truthful certainty or uncertainty
	// for the whole party so cautious play does not require deciphering false reports.
	// See ref:COMBAT-0010, ref:CMD-0007.
	return chestInspectionResult(g.RollDice(combatTreasureDiceSides) >= difficulty, armed)
}

func chestInspectionResult(conclusive, armed bool) references.ChestAssessment {
	if !conclusive {
		return references.ChestUncertain
	}
	if armed {
		return references.ChestTrapped
	}

	return references.ChestSafe
}

func objectChestAssessmentMessage(assessment references.ChestAssessment, value int) string {
	switch assessment {
	case references.ChestSafe:
		return chestSafeMessage
	case references.ChestUncertain:
		return chestUncertainMessage
	case references.ChestTrapped:
		switch {
		case value < simpleTrapMaximumValue:
			return "Thou dost find a simple trap!"
		case value > complexTrapMinimumValue:
			return "Thou dost find a complex trap!"
		default:
			return "Thou dost find a trap!"
		}
	case references.ChestUninspected:
		panic("reporting an uninspected chest")
	default:
		panic("invalid chest assessment")
	}
}
