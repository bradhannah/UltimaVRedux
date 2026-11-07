package party_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// CanEquipItem checks if a character can equip an item.
// Returns (canEquip bool, reason string).
// Validates:
//   - Strength requirement met
//   - Two-handed weapon conflicts (shield must be free)
//   - Shield conflicts with two-handed weapons
//
// Note: This method allows REPLACING items in the same slot.
// The caller is responsible for unequipping the old item before equipping the new one.
// The only conflicts checked are cross-slot conflicts (2H weapon vs shield).
func (p *PlayerCharacter) CanEquipItem(item references.Equipment) (bool, string) {
	if item == references.NoEquipment {
		return false, "Nothing to equip"
	}

	slot := item.GetSlotType()
	strRequired := item.GetStrengthRequired()
	twoHanded := item.IsTwoHanded()

	// Check cumulative equipment strength requirement; ref:EQUIP-0011.
	// The original game sums ALL equipped items' STR requirements, then checks if adding
	// the new item would exceed the character's STR.
	currentStrLoad := p.GetTotalEquippedStrRequirement()

	// If replacing an item in the same slot, subtract the old item's STR (not double-counted)
	existingItem := p.GetEquippedItem(slot)
	if existingItem != references.NoEquipment {
		currentStrLoad -= existingItem.GetStrengthRequired()
	}

	totalRequired := currentStrLoad + strRequired
	if totalRequired > int(p.Strength) {
		return false, fmt.Sprintf("Thou art not strong enough! (Need %d, have %d STR)", totalRequired, p.Strength)
	}

	// Check two-handed weapon conflicts
	if twoHanded {
		// Two-handed weapon: shield slot must be free (weapon slot will be replaced)
		// Original Ultima V message: "Both hands must be free before thou canst wield that!"
		if !p.IsSlotEmpty(references.SlotShield) {
			return false, "Both hands must be free!"
		}
	} else if slot == references.SlotShield {
		// Shield: cannot equip if two-handed weapon is equipped
		if p.HasTwoHandedWeaponEquipped() {
			return false, "Cannot use shield with two-handed weapon!"
		}
	}

	// Other slot conflicts (same slot replacement) are handled by caller
	// which unequips the old item before calling SetEquippedItem

	return true, ""
}
