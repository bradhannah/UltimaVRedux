package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const maxCombatFields = 32

// CombatFieldKind identifies a transient magical field on a combat tile.
type CombatFieldKind int

const (
	// CombatFieldPoison creates the original poison-gas field.
	CombatFieldPoison CombatFieldKind = iota
	// CombatFieldSleep creates the original sleep field.
	CombatFieldSleep
	// CombatFieldFire creates the original fire field.
	CombatFieldFire
	// CombatFieldProtection creates the original movement-blocking field.
	CombatFieldProtection
)

// CombatField is encounter-local state that may coexist with objects and actors.
type CombatField struct {
	kind               CombatFieldKind
	position           references.Position
	createdThisAction  bool
	originalObjectSlot uint8
}

// NewCombatField constructs an encounter-local field at a combat position.
func NewCombatField(kind CombatFieldKind, position references.Position) CombatField {
	return CombatField{kind: kind, position: position}
}

// Kind returns the field's original effect type.
func (field CombatField) Kind() CombatFieldKind { return field.kind }

// Position returns the field's combat-grid position.
func (field CombatField) Position() references.Position { return field.position }

// Sprite returns the field's rendering sprite.
func (field CombatField) Sprite() indexes.SpriteIndex {
	switch field.kind {
	case CombatFieldPoison:
		return indexes.PoisonField
	case CombatFieldSleep:
		return indexes.MagicField
	case CombatFieldFire:
		return indexes.FireField
	case CombatFieldProtection:
		return indexes.ElectricField
	default:
		panic("unknown combat field kind")
	}
}

// BlocksMovement reports whether the field prevents combat movement.
func (field CombatField) BlocksMovement() bool { return field.kind == CombatFieldProtection }

// AddField admits a field in the lowest free original object slot.
func (combatState *CombatState) AddField(field CombatField) bool {
	if len(combatState.fields) >= maxCombatFields {
		return false
	}
	slot, found := combatState.claimActiveObjectSlot()
	if !found {
		return false
	}
	field.originalObjectSlot = slot
	insertAt := len(combatState.fields)
	for index, existing := range combatState.fields {
		if existing.originalObjectSlot > field.originalObjectSlot {
			insertAt = index

			break
		}
	}
	combatState.fields = append(combatState.fields, CombatField{})
	copy(combatState.fields[insertAt+1:], combatState.fields[insertAt:])
	combatState.fields[insertAt] = field

	return true
}

// Fields returns the encounter's fields in original creation order.
func (combatState *CombatState) Fields() []CombatField { return combatState.fields }

// HasProtectionFieldAt reports whether a field blocks movement at a position.
func (combatState *CombatState) HasProtectionFieldAt(position references.Position) bool {
	for _, field := range combatState.fields {
		if field.Position() == position && field.BlocksMovement() {
			return true
		}
	}

	return false
}

func (combatState *CombatState) removeFirstFieldAt(position references.Position) (CombatField, bool) {
	for index, field := range combatState.fields {
		if field.Position() != position {
			continue
		}
		combatState.fields = append(combatState.fields[:index], combatState.fields[index+1:]...)

		return field, true
	}

	return CombatField{}, false
}

func (combatState *CombatState) hasFieldAt(position references.Position) bool {
	for _, field := range combatState.fields {
		if field.Position() == position {
			return true
		}
	}

	return false
}

func (combatState *CombatState) firstHarmfulFieldAt(position references.Position) (CombatField, bool) {
	for _, field := range combatState.fields {
		if field.Position() == position && field.Kind() != CombatFieldProtection {
			return field, true
		}
	}

	return CombatField{}, false
}

func (combatState *CombatState) expireFields(randomInt func(int) int) []CombatField {
	expired := make([]CombatField, 0, len(combatState.fields))
	remaining := combatState.fields[:0]
	for _, field := range combatState.fields {
		if randomInt(256) < 16 {
			expired = append(expired, field)

			continue
		}
		field.createdThisAction = false
		remaining = append(remaining, field)
	}
	combatState.fields = remaining

	return expired
}
