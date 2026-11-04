package map_units

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// EnemyVariant identifies special large/small-map runtime semantics that cannot
// be recovered from the reused original monster-table slot alone.
type EnemyVariant string

const (
	// EnemyVariantStandard identifies an ordinary monster-table enemy.
	EnemyVariantStandard EnemyVariant = "standard"
	// EnemyVariantPirate identifies a directionally rendered pirate ship.
	EnemyVariantPirate EnemyVariant = "pirate"
	// EnemyVariantWhirlpool identifies the non-combat whirlpool hazard.
	EnemyVariantWhirlpool EnemyVariant = "whirlpool"
	// EnemyVariantShadowlord identifies an enemy with a specific Shadowlord identity.
	EnemyVariantShadowlord EnemyVariant = "shadowlord"
)

// NPCEnemy single instance of an enemy NPC
type NPCEnemy struct {
	EnemyReference references.EnemyReference
	mapUnitDetails MapUnitDetails
	variant        EnemyVariant
	currentHP      int
	facing         references.Direction
	keyFrameTile   *references.Tile
	shadowlord     references.Shadowlord
}

// NewEnemyNPC creates an enemy from its monster reference, numbered npcNum on its map.
func NewEnemyNPC(enemyRef references.EnemyReference, npcNum int) NPCEnemy {
	enemy := NPCEnemy{} //nolint:exhaustruct_v5
	enemy.EnemyReference = enemyRef
	enemy.variant = enemyVariant(enemyRef)
	enemy.currentHP = enemyRef.HitPoints
	enemy.keyFrameTile = enemyRef.KeyFrameTile
	enemy.facing = facingForEnemyTile(enemyRef.KeyFrameTile)
	enemy.shadowlord = references.NoShadowlord
	enemy.mapUnitDetails.NPCNum = npcNum
	enemy.mapUnitDetails.aiOverrideProvenance = references.AIOverrideProvenanceNone
	enemy.mapUnitDetails.CurrentPath = nil

	return enemy
}

func enemyVariant(enemyRef references.EnemyReference) EnemyVariant {
	switch {
	case enemyRef.KeyFrameTile != nil && references.IsPirateShipSprite(enemyRef.KeyFrameTile.Index):
		return EnemyVariantPirate
	case enemyRef.MonsterType == references.MonsterWhirlpool:
		return EnemyVariantWhirlpool
	case enemyRef.MonsterType == references.MonsterShadowlord ||
		(enemyRef.KeyFrameTile != nil && enemyRef.KeyFrameTile.Index == indexes.ShadowLordKeyIndex):
		return EnemyVariantShadowlord
	default:
		return EnemyVariantStandard
	}
}

func facingForEnemyTile(tile *references.Tile) references.Direction {
	if tile == nil {
		return references.NoneDirection
	}
	switch tile.Index { //nolint:exhaustive // Only directional pirate tiles carry facing state.
	case indexes.PirateShipUp:
		return references.Up
	case indexes.PirateShipRight:
		return references.Right
	case indexes.PirateShipDown:
		return references.Down
	case indexes.PirateShipLeft:
		return references.Left
	default:
		return references.NoneDirection
	}
}

// Variant returns this enemy's semantic runtime variant.
func (enemy *NPCEnemy) Variant() EnemyVariant { return enemy.variant }

// CurrentHP returns this instance's current durability.
func (enemy *NPCEnemy) CurrentHP() int { return enemy.currentHP }

// SetCurrentHP restores or changes instance durability.
func (enemy *NPCEnemy) SetCurrentHP(hitPoints int) {
	if hitPoints < 0 {
		panic("enemy current HP cannot be negative")
	}
	enemy.currentHP = hitPoints
}

// ApplyDamage subtracts damage and reports whether the enemy was destroyed.
func (enemy *NPCEnemy) ApplyDamage(damage int) bool {
	if damage < 0 {
		panic("enemy damage cannot be negative")
	}
	enemy.currentHP = max(enemy.currentHP-damage, 0)

	return enemy.currentHP == 0
}

// Facing returns the semantic cardinal facing for directional variants.
func (enemy *NPCEnemy) Facing() references.Direction { return enemy.facing }

// SetFacing updates instance-owned facing and sprite state.
func (enemy *NPCEnemy) SetFacing(direction references.Direction, tile *references.Tile) {
	if direction < references.Up || direction > references.Right || tile == nil {
		panic("enemy facing requires a cardinal direction and tile")
	}
	enemy.facing = direction
	enemy.keyFrameTile = tile
}

// SetShadowlordIdentity binds the exact Shadowlord represented by this instance.
func (enemy *NPCEnemy) SetShadowlordIdentity(lord references.Shadowlord) {
	if enemy.variant != EnemyVariantShadowlord || lord < references.Faulinei || lord > references.Nosfentor {
		panic(fmt.Sprintf("invalid Shadowlord identity %d for enemy variant %q", lord, enemy.variant))
	}
	enemy.shadowlord = lord
}

// ShadowlordIdentity returns the exact identity and whether this is a bound Shadowlord.
func (enemy *NPCEnemy) ShadowlordIdentity() (references.Shadowlord, bool) {
	return enemy.shadowlord, enemy.variant == EnemyVariantShadowlord && enemy.shadowlord != references.NoShadowlord
}

// GetMapUnitType reports Enemy for every NPCEnemy.
func (enemy *NPCEnemy) GetMapUnitType() MapUnitType {
	return Enemy
}

// Pos returns the position by value; PosPtr exposes the live field for in-place updates.
func (enemy *NPCEnemy) Pos() references.Position {
	return enemy.mapUnitDetails.Position
}

// MapUnitDetails returns the shared per-unit state, which the caller may mutate.
func (enemy *NPCEnemy) MapUnitDetails() *MapUnitDetails {
	return &enemy.mapUnitDetails
}

// SetVisible shows or hides the unit; hidden units do not occupy tiles.
func (enemy *NPCEnemy) SetVisible(visible bool) {
	enemy.mapUnitDetails.Visible = visible
}

// IsVisible reports whether the unit is shown; hidden units are excluded from occupancy.
func (enemy *NPCEnemy) IsVisible() bool {
	return enemy.mapUnitDetails.Visible
}

// IsEmptyMapUnit reports whether the unit has no key frame tile, so it occupies no tile.
func (enemy *NPCEnemy) IsEmptyMapUnit() bool {
	return enemy.keyFrameTile == nil
}

// Floor returns the dungeon floor the unit is on.
func (enemy *NPCEnemy) Floor() references.FloorNumber {
	return enemy.mapUnitDetails.Floor
}

// PosPtr returns the live position so callers can update it in place.
func (enemy *NPCEnemy) PosPtr() *references.Position {
	return &enemy.mapUnitDetails.Position
}

// SetPos records a new map position without any movement or collision checks.
func (enemy *NPCEnemy) SetPos(position references.Position) {
	enemy.mapUnitDetails.Position = position
}

// SetFloor records the dungeon floor the unit occupies.
func (enemy *NPCEnemy) SetFloor(floor references.FloorNumber) {
	enemy.mapUnitDetails.Floor = floor
}

// GetKeySpriteIndex returns the sprite of the key frame tile, or sprite 0 when the unit has none.
func (enemy *NPCEnemy) GetKeySpriteIndex() indexes.SpriteIndex {
	if enemy.keyFrameTile == nil {
		return indexes.SpriteIndex(0)
	}

	return enemy.keyFrameTile.Index
}
