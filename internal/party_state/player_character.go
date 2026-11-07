package party_state

import (
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// GetNameAsString returns the name without its trailing NUL padding.
func (p *PlayerCharacter) GetNameAsString() string {
	return strings.TrimRight(string(p.Name[:]), string(rune(0)))
}

// GetKeySpriteIndex returns the party-panel key sprite for the character class, or 0 for an unknown class.
func (p *PlayerCharacter) GetKeySpriteIndex() indexes.SpriteIndex {
	switch p.Class {
	case Avatar:
		return indexes.AvatarKeyIndex
	case Wizard:
		return indexes.WizardKeyIndex
	case Fighter:
		return indexes.FighterKeyIndex
	case Bard:
		return indexes.BardKeyIndex
	}

	return 0
}

// IsAlive reports whether the character is in the party, has hit points and is not dead.
func (p *PlayerCharacter) IsAlive() bool {
	return p.currentHp > 0 && !p.IsDead() && p.IsInParty()
}

// IsDead reports whether the character status is Dead.
func (p *PlayerCharacter) IsDead() bool {
	return p.status == Dead
}

// IsPoisoned reports whether the character status is Poisoned.
func (p *PlayerCharacter) IsPoisoned() bool {
	return p.status == Poisoned
}

// IsAsleep reports whether the character status is Sleep.
func (p *PlayerCharacter) IsAsleep() bool {
	return p.status == Sleep
}

// IsGood reports whether the character status is Good.
func (p *PlayerCharacter) IsGood() bool {
	return p.status == Good
}

// MarkAsDead sets the character status to Dead
func (p *PlayerCharacter) MarkAsDead() {
	p.status = Dead
}

// SetStatus sets the character status
func (p *PlayerCharacter) SetStatus(status CharacterStatus) {
	p.status = status
}

// IsInParty reports whether the character is in the packed active roster.
func (p *PlayerCharacter) IsInParty() bool {
	return p.PartyMap == PartyMapActive
}

// GetCurrentHp returns the current hit points.
func (p *PlayerCharacter) GetCurrentHp() uint16 {
	return p.currentHp
}

// GetMaxHp returns the maximum hit points.
func (p *PlayerCharacter) GetMaxHp() uint16 {
	return p.maxHp
}

// GetCurrentMp returns the current magic points.
func (p *PlayerCharacter) GetCurrentMp() byte {
	return p.currentMp
}

// GetStatus returns the character status letter.
func (p *PlayerCharacter) GetStatus() CharacterStatus {
	return p.status
}

// AddHp heals by the amount, clamped to maximum HP, and revives a character at zero HP to Good.
func (p *PlayerCharacter) AddHp(amount uint16) {
	if p.currentHp <= 0 {
		p.status = Good
	}
	p.currentHp += amount
	if p.currentHp > p.maxHp {
		p.currentHp = p.maxHp
	}
}

// SetHp sets current HP, clamped to maximum HP, and revives a character at zero HP to Good.
func (p *PlayerCharacter) SetHp(amount uint16) {
	if p.currentHp <= 0 {
		p.status = Good
	}
	p.currentHp = amount
	if p.currentHp > p.maxHp {
		p.currentHp = p.maxHp
	}
}

// SubtractHp removes HP, floors it at zero and marks the character Dead at zero.
func (p *PlayerCharacter) SubtractHp(amount uint16) {
	if amount >= p.currentHp {
		p.currentHp = 0
	} else {
		p.currentHp -= amount
	}
	if p.currentHp <= 0 {
		p.status = Dead
	}
}

// SetMaxHp sets maximum HP and lowers current HP to fit.
func (p *PlayerCharacter) SetMaxHp(amount uint16) {
	p.maxHp = amount
	// Clamp current HP if exceeds new max
	if p.currentHp > p.maxHp {
		p.currentHp = p.maxHp
	}
}

// GetMaxMp returns the magic point cap, which is always the Intelligence score.
func (p *PlayerCharacter) GetMaxMp() byte {
	return p.Intelligence // MP cap is always INT
}

// CanCastSpells reports whether the character's class can ever use magic.
func (p *PlayerCharacter) CanCastSpells() bool {
	return p.Class == Avatar || p.Class == Wizard || p.Class == Bard
}

// SetMp sets current MP, clamped to Intelligence.
func (p *PlayerCharacter) SetMp(amount byte) {
	maxMp := p.GetMaxMp()
	p.currentMp = amount
	if p.currentMp > maxMp {
		p.currentMp = maxMp
	}
}

// SubtractMp removes MP and floors it at zero.
func (p *PlayerCharacter) SubtractMp(amount byte) {
	if amount >= p.currentMp {
		p.currentMp = 0
	} else {
		p.currentMp -= amount
	}
}

// bardMpDivisor halves Intelligence for the Bard magic point restoration.
const bardMpDivisor = 2

// RestoreMpByClass restores MP based on character class
// Camping and Wanderer restoration share this cap; ref:CAMP-0005 and ref:PROGRESS-0003.
func (p *PlayerCharacter) RestoreMpByClass() {
	switch p.Class {
	case Wizard, Avatar: // 'M' and 'A' in original
		p.SetMp(p.Intelligence)
	case Bard: // 'B' in original
		p.SetMp(p.Intelligence / bardMpDivisor)
	case Fighter:
		// Fighters have no magic points to restore.
	}
}

// =============================================================================
// Equipment Management Methods
// =============================================================================

// GetEquippedItem returns the equipment in the specified slot.
// Returns references.NoEquipment if the slot is empty.
func (p *PlayerCharacter) GetEquippedItem(slot references.EquipmentSlot) references.Equipment {
	switch slot {
	case references.SlotHelm:
		return references.Equipment(p.Helmet)
	case references.SlotArmour:
		return references.Equipment(p.Armor)
	case references.SlotWeapon:
		// The persisted field name is legacy; Ready treats this byte as generic hand0.
		return references.Equipment(p.Weapon)
	case references.SlotShield:
		// The persisted field name is legacy; Ready treats this byte as generic hand1.
		return references.Equipment(p.Shield)
	case references.SlotRing:
		return references.Equipment(p.Ring)
	case references.SlotAmulet:
		return references.Equipment(p.Amulet)
	case references.SlotNone:
		return references.NoEquipment
	default:
		return references.NoEquipment
	}
}

// SetEquippedItem sets the equipment in the specified slot.
// Use references.NoEquipment to unequip.
func (p *PlayerCharacter) SetEquippedItem(slot references.EquipmentSlot, item references.Equipment) {
	if item < 0 || item > references.NoEquipment {
		panic("equipped item must fit the original one-byte character record")
	}
	encoded := byte(item)
	switch slot {
	case references.SlotHelm:
		p.Helmet = encoded
	case references.SlotArmour:
		p.Armor = encoded
	case references.SlotWeapon:
		p.Weapon = encoded
	case references.SlotShield:
		p.Shield = encoded
	case references.SlotRing:
		p.Ring = encoded
	case references.SlotAmulet:
		p.Amulet = encoded
	case references.SlotNone:
	}
}

// IsSlotEmpty returns true if the specified slot has no equipment.
func (p *PlayerCharacter) IsSlotEmpty(slot references.EquipmentSlot) bool {
	return p.GetEquippedItem(slot) == references.NoEquipment
}

// GetTotalAttack returns the sum of attack values from all equipped items.
func (p *PlayerCharacter) GetTotalAttack() int {
	total := 0
	for _, slot := range references.EquipmentSlots() {
		item := p.GetEquippedItem(slot)
		if item != references.NoEquipment {
			total += item.GetAttackValue()
		}
	}

	return total
}

// GetTotalDefense returns the sum of defense values from all equipped items.
func (p *PlayerCharacter) GetTotalDefense() int {
	total := 0
	for _, slot := range references.EquipmentSlots() {
		item := p.GetEquippedItem(slot)
		if item != references.NoEquipment {
			total += item.GetDefenseValue()
		}
	}

	return total
}

// GetTotalEquippedStrRequirement returns the sum of STR requirements for all equipped items.
// This follows the cumulative equipment-strength rule (ref:EQUIP-0011), which calculates
// the cumulative "weight" of all readied items to determine if a new item can be equipped.
func (p *PlayerCharacter) GetTotalEquippedStrRequirement() int {
	sum := 0
	for _, slot := range references.EquipmentSlots() {
		item := p.GetEquippedItem(slot)
		if item != references.NoEquipment {
			sum += item.GetStrengthRequired()
		}
	}

	return sum
}
