package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// CombatLootReveal is a detached discovery, not an inventory acquisition.
type CombatLootReveal struct {
	Sprite      indexes.SpriteIndex
	Description string
}

func (s *CombatState) lootSlotMask() uint64 {
	var mask uint64
	for _, object := range s.objects {
		mask |= 1 << object.originalObjectSlot
	}

	return mask
}

func (s *CombatState) newLootReveals(position references.Position, before uint64, lines []string) []CombatLootReveal {
	entries := make([]CombatLootReveal, 0, len(lines))
	// additem admits descending slots. Reverse the stable ascending object list
	// to pair each newly admitted object's icon with its generated description.
	for index := len(s.objects) - 1; index >= 0; index-- {
		object := s.objects[index]
		if object.position != position || before&(1<<object.originalObjectSlot) != 0 {
			continue
		}
		if len(entries) >= len(lines) {
			panic("loot reveal count differs from admitted objects")
		}
		entries = append(entries, CombatLootReveal{Sprite: object.Sprite(), Description: lines[len(entries)]})
	}
	if len(entries) != len(lines) {
		panic("loot reveal lost an admitted object")
	}

	return entries
}

// CombatPileSprite selects stable ground presentation without changing slots or
// action targeting. A winning field is drawn by the field overlay, so its tile
// underlay is terrain rather than a second object or a duplicate field sprite.
func (g *GameState) CombatPileSprite(position references.Position) (indexes.SpriteIndex, bool) {
	if g.CombatScene == nil || !g.CombatTerrainAllowsObjectDisplay(position) {
		return indexes.NoSprites, false
	}
	sprite, _, field := g.CombatScene.combatGroundDisplayAt(position)
	if field {
		return g.combatMapOnlyTerrain(position), true
	}

	return sprite, sprite != indexes.NoSprites
}

// CombatFieldIsDisplayed applies the same ground selection to the separate
// field overlay. Multiple overlapping fields/markers cannot paint over its winner.
func (g *GameState) CombatFieldIsDisplayed(field CombatField) bool {
	if g.CombatScene == nil || !g.CombatTerrainAllowsObjectDisplay(field.Position()) {
		return false
	}
	_, slot, isField := g.CombatScene.combatGroundDisplayAt(field.Position())

	return isField && slot == field.originalObjectSlot
}

func (s *CombatState) combatGroundDisplayAt(position references.Position) (indexes.SpriteIndex, uint8, bool) {
	// Object display projection scans slots 31 down to 0. Ordinary objects
	// always write: the lowest ordinary slot wins. Fields and PC/monster corpses
	// only write while terrain projection > 0: without an ordinary object, the highest such
	// slot wins. There is no chest priority or relative PC/monster-body priority.
	// See ref:COMBAT-0010.
	fallback, fallbackSlot := indexes.NoSprites, uint8(0)
	for _, object := range s.objects {
		if object.position != position {
			continue
		}
		if !isCombatCorpseMarker(object.Sprite()) {
			return object.Sprite(), object.originalObjectSlot, false
		}
		if object.originalObjectSlot > fallbackSlot {
			fallback, fallbackSlot = object.Sprite(), object.originalObjectSlot
		}
	}
	isField := false
	for _, field := range s.fields {
		if field.Position() == position && field.originalObjectSlot > fallbackSlot {
			fallback, fallbackSlot, isField = field.Sprite(), field.originalObjectSlot, true
		}
	}

	return fallback, fallbackSlot, isField
}
