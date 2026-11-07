package party_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// campingMPPerTenMinutes is the MP each character regains per ten minutes of rest.
const campingMPPerTenMinutes = 2

// RegenerateSleepHealth heals sleeping characters for one 10-minute cycle
// Returns messages for any deaths that occur from poison damage
// NOTE: This is for INN REST, not bed rest. Bed rest does NOT heal in original Ultima V.
func (p *PartyState) RegenerateSleepHealth() []string {
	var messages []string

	for i := range p.characters {
		char := p.characters[i]

		// Nil or dead characters don't heal
		if char == nil || !char.IsAlive() {
			continue
		}

		// Poisoned characters take damage while sleeping instead of healing
		if char.IsPoisoned() {
			oldHp := char.GetCurrentHp()
			char.SubtractHp(1)

			// Check for death from poison
			if oldHp > 0 && char.GetCurrentHp() == 0 {
				char.MarkAsDead()
				messages = append(messages,
					fmt.Sprintf("Alas! %s has died!", char.GetNameAsString()))
			}

			continue
		}

		// Only process sleeping characters for healing
		if !char.IsAsleep() {
			continue
		}

		// Normal healing for sleeping characters: 1 HP per 10 minutes
		if !char.IsFullyHealed() {
			char.AddHp(1)
		}

		// MP restoration: 2 MP per 10 minutes
		if char.GetCurrentMp() < char.GetMaxMp() {
			char.AddMp(campingMPPerTenMinutes)
		}
	}

	return messages
}

// SetPartySleepStatus toggles sleep status for the entire party
// sleeping=true: Good status characters become Sleep
// sleeping=false: Sleep status characters become Good
func (p *PartyState) SetPartySleepStatus(sleeping bool) {
	for i := range p.characters {
		char := p.characters[i]
		if char == nil {
			continue
		}
		if sleeping && char.IsGood() {
			char.SetStatus(Sleep)
		} else if !sleeping && char.IsAsleep() {
			char.SetStatus(Good)
		}
		// Other statuses (Dead, Poisoned) remain unchanged
	}
}

// IsFullyHealed reports whether the character is at maximum HP.
func (p *PlayerCharacter) IsFullyHealed() bool {
	return p.currentHp >= p.maxHp
}

// AddMp restores MP, capped at the character's maximum.
func (p *PlayerCharacter) AddMp(amount byte) {
	maxMp := p.GetMaxMp()
	p.currentMp += amount
	if p.currentMp > maxMp {
		p.currentMp = maxMp
	}
}

// HasTwoHandedWeaponEquipped returns true if the character has a two-handed weapon equipped.
func (p *PlayerCharacter) HasTwoHandedWeaponEquipped() bool {
	weapon := p.GetEquippedItem(references.SlotWeapon)
	if weapon == references.NoEquipment {
		return false
	}

	return weapon.IsTwoHanded()
}

// NewBlankPartyState returns sixteen zero-valued roster slots, the party an
// all-zero original save decodes to. Tests place the members they need.
func NewBlankPartyState() *PartyState {
	var roster [NRosterSlots]*PlayerCharacter
	for index := range roster {
		roster[index] = new(PlayerCharacter)
	}

	return NewPartyStateFromRoster(roster)
}
