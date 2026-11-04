package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const originalCombatPlayerShape = 0x1c

// CombatTerrainAllowsObjectDisplay reproduces object display projection object admission.
// The actors and objects remain authoritative while their authored underlay conceals them.
// See ref:COMBAT-0001.
func (g *GameState) CombatTerrainAllowsObjectDisplay(position references.Position) bool {
	return !CombatTerrainConceals(g.combatMapOnlyTerrain(position))
}

// isCombatMapPosition reports whether a position lies on the 11x11 tactical map.
func isCombatMapPosition(position references.Position) bool {
	return position.X >= 0 && position.X < references.XCombatMapTiles &&
		position.Y >= 0 && position.Y < references.YCombatMapTiles
}

// CombatTerrainConceals reports the authored underlays, Nothing and Arch, that hide
// the objects and actors standing on them. See ref:COMBAT-0001.
func CombatTerrainConceals(terrain indexes.SpriteIndex) bool {
	return terrain == indexes.BlackSquare || terrain == indexes.BrickWallArchway
}

func (g *GameState) combatTerrainAllowsDestination(position references.Position) bool {
	return g.combatMapOnlyTerrain(position) != indexes.BlackSquare
}

func (g *GameState) combatMapOnlyTerrain(position references.Position) indexes.SpriteIndex {
	layeredMap := g.GetCurrentLayeredMap()
	if layeredMap == nil {
		panic("combat terrain requested without an active layered map")
	}
	terrain := layeredMap.GetTileTopMapOnlyTile(&position)
	if terrain == nil {
		return indexes.NoSprites
	}

	return terrain.Index
}

func originalCombatMonsterShape(monsterType references.MonsterType) int {
	return references.OriginalMonsterShape(monsterType)
}

// combatTerrainAllowsOriginalShape shares terrain passability's creature terrain
// rules with world movement. Tactical occupancy and placement gates stay local.
// See ref:COMBAT-0001.
func combatTerrainAllowsOriginalShape(shape int, terrain *references.Tile) bool {
	return references.OriginalCreatureTerrainAllowsShape(shape, terrain)
}
