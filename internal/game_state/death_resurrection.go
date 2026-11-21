package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Death/resurrection constants from original Ultima V party recovery rules
// See ref:RECOVERY-0002.
const (
	// Resurrection location - Castle Britannia throne room area
	ResurrectionLocationX = 10
	ResurrectionLocationY = 10
	ResurrectionFloor     = 1 // Floor 1 of Castle Britannia

	// Post-resurrection state
	ResurrectionHour       = 6  // Resurrection advances in nine-minute steps until the clock enters 6 AM.
	MinimumKarmaAfterDeath = 75 // Karma raised to at least 75
	MinimumFoodAfterDeath  = 63 // Empty food stores are restored to 63 units.

	// KarmaPercentageBase is the divisor for karma-based XP penalty calculation.
	// Formula: newXP = (currentXP * karma) / KarmaPercentageBase
	// Example: karma 50 = lose 50% XP, karma 80 = lose 20% XP
	KarmaPercentageBase = 100
)

// applyFullPartyDeathConsequences is the single full-party death penalty mutator.
// Guardian recovery restores each member separately after this shared state change.
func (gs *GameState) applyFullPartyDeathConsequences() {
	karma := gs.PartyState.Karma.Value

	// Apply XP penalty for each active party member (no resurrection)
	for i := range party_state.MaxActiveParty {
		char := gs.PartyState.GetCharacter(i)
		if char == nil || char.PartyMap != party_state.PartyMapActive {
			continue
		}

		if karma < 98 {
			char.Exp = uint16((int(char.Exp) * karma) / KarmaPercentageBase)
		}

		// Recalculate level based on new XP
		gs.recalculateLevelFromExperience(char)
	}

	// Raise karma to minimum 75 if below
	if gs.PartyState.Karma.Value < MinimumKarmaAfterDeath {
		gs.PartyState.Karma.Value = MinimumKarmaAfterDeath
	}

	// Original party recovery rules restores food only when the party has none.
	// See ref:RECOVERY-0002.
	if gs.PartyState.Inventory.Provisions.Food.Get() == 0 {
		gs.PartyState.Inventory.Provisions.Food.Set(MinimumFoodAfterDeath)
	}

	// Clear active spell/duration effects
	gs.ClearDurationEffect()

	// Extinguish all light sources (torches, magic light)
	gs.extinguishAllLights()
}

// ResurrectCharacterWithFullRecovery resurrects a single party member by index.
// Sets status to Good, restores full HP, and restores MP by class.
// Used by death sequence for Guardian resurrection (restores to full, unlike spell resurrect at HP=1).
func (gs *GameState) ResurrectCharacterWithFullRecovery(index int) {
	char := gs.PartyState.GetCharacter(index)
	if char == nil || char.PartyMap != party_state.PartyMapActive {
		return
	}

	char.SetStatus(party_state.Good)
	char.SetHp(char.GetMaxHp())
	char.RestoreMpByClass()
}

// recalculateLevelFromExperience applies individual revival's repeated-halving formula.
// See ref:RECOVERY-0002.
func (gs *GameState) recalculateLevelFromExperience(char *party_state.PlayerCharacter) {
	newLevel := CalculateLevelFromExperience(char.Exp)
	char.Level = newLevel

	newMaxHP := uint16(newLevel) * 30
	char.SetMaxHp(newMaxHP)

	// Ensure current HP doesn't exceed new max
	if char.GetCurrentHp() > newMaxHP {
		char.SetHp(newMaxHP)
	}
}

// extinguishAllLights turns off all active light sources.
func (gs *GameState) extinguishAllLights() {
	// Turn off magic light from Vas Lor spell
	gs.MagicLightMinutesRemaining = 0
	gs.MapState.Lighting.SetMagicLight(false)

	// Extinguish torch by setting turns to 0
	gs.MapState.Lighting.ExtinguishTorch()

	// Invalidate light cache
	gs.GetCurrentLayeredMap().InvalidateLightSourceCache()
}

// GetResurrectionLocation returns the Castle Britannia location for resurrection.
func (gs *GameState) GetResurrectionLocation() references.Location {
	return references.LordBritishsCastle
}

// GetResurrectionPosition returns the specific coordinates within Castle Britannia.
func (gs *GameState) GetResurrectionPosition() references.Position {
	return references.Position{
		X: ResurrectionLocationX,
		Y: ResurrectionLocationY,
	}
}

// GetResurrectionFloor returns the floor level for resurrection.
func (gs *GameState) GetResurrectionFloor() references.FloorNumber {
	return references.FloorNumber(ResurrectionFloor)
}

// checkForPartyWipe checks if all party members are dead and triggers the death sequence.
// Called at the end of turn processing after environmental hazards.
func (gs *GameState) checkForPartyWipe() {
	if gs.combatRecoveryReviewDeferred {
		return
	}
	if gs.PartyState.IsAllPartyDead() {
		gs.SystemCallbacks.Flow.Consequences.TriggerDeathSequence()
	}
}
