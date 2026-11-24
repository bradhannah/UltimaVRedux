package map_units

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// hullWarningHP is the hull strength below which a damaged ship warns.
const hullWarningHP = 10

// GetFirstFriendlyMatching returns the first friendly NPC matching the predicate, or nil if none found
func (m *MapUnits) GetFirstFriendlyMatching(match FriendlyMatchFunc) *NPCFriendly {
	for _, mu := range *m {
		if friendly, ok := mu.(*NPCFriendly); ok {
			if match(friendly) {
				return friendly
			}
		}
	}

	return nil
}

// GetHullWarningMessage returns warning message based on current hull HP
// Based on original Ultima V:
//   - HP < 10 shows "DANGER: SHIP BADLY DAMAGED!"; ref:SHIP-0003.
//   - HP 0 shows "Ship sunk!"; ref:SHIP-0003.
//
// Returns empty string if HP >= 10 (healthy ship)
func (v *VehicleDetails) GetHullWarningMessage() string {
	if v.HullHP <= 0 {
		return "Ship sunk!"
	}
	if v.HullHP < hullWarningHP {
		return "DANGER: SHIP BADLY DAMAGED!"
	}

	return "" // Healthy ship, no warning
}

// RepairHull increases hull HP by the given amount
// HP is clamped to 99 maximum (original Ultima V max)
func (v *VehicleDetails) RepairHull(amount int) {
	v.HullHP = clampHullHP(v.HullHP + amount)
}
