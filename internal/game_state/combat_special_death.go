package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func (g *GameState) resolveSpecialCombatMonsterDeath(monster *CombatMonster, monsterIdx int) bool {
	switch monster.MonsterType() { //nolint:exhaustive // Only source-defined special deaths are handled here.
	case references.MonsterGazer:
		g.resolveGazerDeath(monster, monsterIdx)

		return true
	case references.MonsterGargoyle:
		g.setCombatTerrain(monster.Position(), indexes.SmallRockWall)

		return true
	default:
		return false
	}
}

// resolveGazerDeath replicates the direct same-square Insect creation in
// original combat damage rules. Ordinary placement legality is intentionally not consulted.
// See ref:COMBAT-0009.
func (g *GameState) resolveGazerDeath(gazer *CombatMonster, monsterIdx int) {
	if !g.CombatScene.transferCombatantObjectTo(CombatTurnIdentity{
		IsPartyMember: false,
		EntityIndex:   monsterIdx,
	}, NewCombatCorpse(gazer.Position())) {
		panic("resolveGazerDeath: dead Gazer has no original object slot")
	}
	if !g.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
		return
	}

	insectReference, err := g.enemyReferenceForCombat(references.MonsterInsect)
	if err != nil {
		panic(fmt.Errorf("resolving Gazer death: %w", err))
	}
	insect := NewCombatMonster(
		references.MonsterInsect,
		insectReference,
		gazer.Position(),
		g.AsRandomProvider(),
	)
	if !g.CombatScene.hasActiveObjectSlot() {
		return
	}
	g.CombatScene.AddMonsterToQueue(insect)
}

func (g *GameState) setCombatTerrain(position references.Position, tile indexes.SpriteIndex) {
	g.CombatScene.CombatMapRef.SetTileNumber(position, tile)
	layer := g.GetCurrentLayeredMap()
	layer.SetTileByLayer(map_state.MapLayer, &position, tile)
	// Combat terrain is encounter-local. An earlier Open/Jimmy override must
	// not mask a later authoritative replacement (including Skull Key unlocks).
	layer.UnSetTileByLayer(map_state.MapOverrideLayer, &position)
	layer.InvalidateLightSourceCache()
}
