package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ActionPushSmallMap pushes or pulls adjacent town furniture onto an eligible floor.
func (g *GameState) ActionPushSmallMap(direction references.Direction) bool {
	// Auto-shut doors if needed (equivalent to shut_previous_door)
	doorClosed := g.MapState.SmallMapProcessTurnDoors(g.isSmallMapDoorOccupied)
	if doorClosed {
		g.SystemCallbacks.Audio.Play(SoundCloseDoor).Start()
	}

	pushableThingPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if pushableThingPosition == nil {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}
	smallMap := g.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, g.MapState.PlayerLocation.Floor)
	pushableThingTile := smallMap.GetSmallMapPersistentTerrainTile(pushableThingPosition, g.DateTime.Hour)

	// Detailed validation with game state
	if pushableThingTile == nil || g.ObjectPresentAt(pushableThingPosition) || !pushableThingTile.IsPushable() {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}

	legalFloorIndex := originalPushLegalFloor(pushableThingTile.Index)

	farSideOfPushableThingPosition := direction.GetNewPositionInDirection(pushableThingPosition)
	if farSideOfPushableThingPosition == nil {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}
	// Furniture swaps stored terrain, not the temporary floor under a doorway
	// occupant. Treating that live floor as movable would relocate a closed door.
	farSideTile := smallMap.GetSmallMapPersistentTerrainTile(farSideOfPushableThingPosition, g.DateTime.Hour)
	playerTile := smallMap.GetSmallMapPersistentTerrainTile(&g.MapState.PlayerLocation.Position, g.DateTime.Hour)

	// Try to push object forward into farSide
	if farSideTile != nil &&
		!g.IsOutOfBounds(*farSideOfPushableThingPosition) &&
		!g.ObjectPresentAt(farSideOfPushableThingPosition) &&
		farSideTile.Is(legalFloorIndex) {
		// Execute push
		g.pushIt(smallMap, pushableThingTile, pushableThingPosition, farSideOfPushableThingPosition, direction)
		g.MapState.PlayerLocation.Position = *pushableThingPosition

		// User feedback via callbacks
		g.SystemCallbacks.Message.AddRowStr("Pushed!")
		g.SystemCallbacks.Audio.Play(SoundPushObject).Start()
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true
	} else if playerTile != nil && playerTile.Is(legalFloorIndex) && !g.ObjectPresentAt(&g.MapState.PlayerLocation.Position) {
		// Try swapping (pulling)
		g.swapIt(smallMap, pushableThingTile, &g.MapState.PlayerLocation.Position, pushableThingPosition, direction)
		g.MapState.PlayerLocation.Position = *pushableThingPosition

		// User feedback via callbacks
		g.SystemCallbacks.Message.AddRowStr("Pulled!")
		g.SystemCallbacks.Audio.Play(SoundPushObject).Start()
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true
	}

	// Failed to push or pull - use "Won't budge" (no exclamation) when player not on legal floor
	g.SystemCallbacks.Message.AddRowStr("Won't budge")

	return false
}

// Helper function to handle pushing object forward
func (g *GameState) pushIt(smallMap *map_state.LayeredMap, objTile *references.Tile, fromPos, toPos *references.Position, direction references.Direction) {
	smallMap.SwapTiles(fromPos, toPos)
	if objTile.IsChair() || objTile.IsCannon() {
		smallMap.SetTileByLayer(map_state.MapLayer, toPos, originalPushFacing(objTile.Index, direction))
	}
}

// Helper function to handle swapping player with object (pull)
func (g *GameState) swapIt(smallMap *map_state.LayeredMap, objTile *references.Tile, playerPos, objPos *references.Position, direction references.Direction) {
	smallMap.SwapTiles(playerPos, objPos)
	if objTile.IsChair() || objTile.IsCannon() {
		smallMap.SetTileByLayer(map_state.MapLayer, playerPos, originalPushFacing(objTile.Index, direction.GetOppositeDirection()))
	}
}

// ActionPushLargeMap pushes or pulls outdoor terrain with temporary write ownership.
func (g *GameState) ActionPushLargeMap(direction references.Direction) bool {
	defer g.AdvanceOrdinarySurfaceCommandMinutes(DefaultLargeMapMinutesPerTurn)

	if g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}
	actorPosition := g.MapState.PlayerLocation.Position
	targetPosition, ok := largeMapPushPosition(actorPosition, direction)
	if !ok {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}
	largeMap := g.GetCurrentLayeredMap()
	object := largeMap.SpriteIndexByLayer(map_state.MapLayer, targetPosition)
	if g.largeMapPushOccupied(targetPosition) || !isOriginalPushableTile(object) {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}

	legalFloor := originalPushLegalFloor(object)
	forwardPosition, _ := largeMapPushPosition(targetPosition, direction)
	forwardTile := largeMap.SpriteIndexByLayer(map_state.MapLayer, forwardPosition)
	actorTile := largeMap.SpriteIndexByLayer(map_state.MapLayer, actorPosition)

	if !g.largeMapPushOccupied(forwardPosition) && forwardTile == legalFloor {
		g.setTransientLargeMapPushTile(largeMap, targetPosition, forwardTile)
		g.setTransientLargeMapPushTile(largeMap, forwardPosition, originalPushFacing(object, direction))
		g.MapState.PlayerLocation.Position = targetPosition
		g.SystemCallbacks.Message.AddRowStr("Pushed!")

		return true
	}
	if actorTile == legalFloor {
		g.setTransientLargeMapPushTile(
			largeMap,
			actorPosition,
			originalPushFacing(object, direction.GetOppositeDirection()),
		)
		g.setTransientLargeMapPushTile(largeMap, targetPosition, actorTile)
		g.MapState.PlayerLocation.Position = targetPosition
		g.SystemCallbacks.Message.AddRowStr("Pulled!")

		return true
	}

	g.SystemCallbacks.Message.AddRowStr("Won't budge")

	return false
}

type transientLargeMapPushTileKey struct {
	location references.Location
	floor    references.FloorNumber
	position references.Position
}

type transientLargeMapPushTile struct {
	before indexes.SpriteIndex
	after  indexes.SpriteIndex
}

func largeMapPushPosition(position references.Position, direction references.Direction) (references.Position, bool) {
	target := direction.GetNewPositionInDirection(&position)
	if target == nil {
		return references.Position{}, false
	}

	return *target.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles), true
}

func isOriginalPushableTile(tile indexes.SpriteIndex) bool {
	//nolint:exhaustive // This is the exact pushable terrain validation allowlist.
	// See ref:CMD-0001.
	switch tile {
	case indexes.Plant,
		indexes.ChairBackForward, indexes.ChairBackLeft, indexes.ChairBackBack, indexes.ChairBackRight,
		indexes.EndTable, indexes.Barrel, indexes.Vanity, indexes.WaterJugTable,
		indexes.Dresser, indexes.CoffeeTable, indexes.Box,
		indexes.CannonUp, indexes.CannonDown, indexes.CannonRight, indexes.CannonLeft:
		return true
	default:
		return false
	}
}

// originalPushLegalFloor is the floor a push or pull needs: cannons roll only on
// metal floor, other furniture on brick floor.
func originalPushLegalFloor(object indexes.SpriteIndex) indexes.SpriteIndex {
	if object >= indexes.CannonUp && object <= indexes.CannonLeft {
		return indexes.SpriteIndex(indexes.MetalFloor)
	}

	return indexes.SpriteIndex(indexes.BrickFloor)
}

func originalPushFacing(tile indexes.SpriteIndex, direction references.Direction) indexes.SpriteIndex {
	var base indexes.SpriteIndex
	switch {
	case tile >= indexes.ChairBackForward && tile <= indexes.ChairBackRight:
		base = indexes.ChairBackForward
	case tile >= indexes.CannonUp && tile <= indexes.CannonLeft:
		base = indexes.CannonUp
	default:
		return tile
	}

	switch direction {
	case references.Up:
		return base
	case references.Right:
		return base + 1
	case references.Down:
		return base + 2
	case references.Left:
		return base + 3
	case references.NoneDirection:
		return tile
	default:
		return tile
	}
}

func (g *GameState) largeMapPushOccupied(position references.Position) bool {
	if g.ItemStacksMap.HasItemStackAtPosition(&position) {
		return true
	}

	return g.ObjectPresentAt(&position)
}

func (g *GameState) setTransientLargeMapPushTile(
	largeMap *map_state.LayeredMap,
	position references.Position,
	value indexes.SpriteIndex,
) {
	if g.transientLargeMapPushTiles == nil {
		g.transientLargeMapPushTiles = make(map[transientLargeMapPushTileKey]transientLargeMapPushTile)
	}
	key := transientLargeMapPushTileKey{
		location: g.MapState.PlayerLocation.Location,
		floor:    g.MapState.PlayerLocation.Floor,
		position: position,
	}
	actual := largeMap.SpriteIndexByLayer(map_state.MapLayer, position)
	record, tracked := g.transientLargeMapPushTiles[key]
	if !tracked || actual != record.after {
		record.before = actual
	}
	record.after = value
	g.transientLargeMapPushTiles[key] = record
	largeMap.SetTileByLayer(map_state.MapLayer, &position, value)
}

func (g *GameState) persistentTileWithoutTransientLargeMapPush(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	actual indexes.SpriteIndex,
) indexes.SpriteIndex {
	record, tracked := g.transientLargeMapPushTiles[transientLargeMapPushTileKey{
		location: location,
		floor:    floor,
		position: position,
	}]
	if tracked && actual == record.after {
		return record.before
	}

	return actual
}

// expireOutdoorPushTiles models logical quadrant reloads without rebuilding the
// full plane. Restore a write only while Push still owns its current value.
func (g *GameState) expireOutdoorPushTiles(expired func(transientLargeMapPushTileKey) bool) {
	for key, record := range g.transientLargeMapPushTiles {
		if !expired(key) {
			continue
		}
		layer := g.MapState.LayeredMaps.GetLayeredMap(references.LargeMapType, key.floor)
		if layer.SpriteIndexByLayer(map_state.MapLayer, key.position) == record.after {
			layer.SetTileByLayer(map_state.MapLayer, &key.position, record.before)
		}
		delete(g.transientLargeMapPushTiles, key)
	}
}

// ActionPushDungeonMap reports the original immediate dungeon rejection.
func (g *GameState) ActionPushDungeonMap() bool {
	g.SystemCallbacks.Message.AddRowStr("Not here!")

	return false
}

// ActionPushCombatMap pushes or pulls terrain using the active combatant's identity.
func (g *GameState) ActionPushCombatMap(direction references.Direction) bool {
	if g.CombatScene == nil || g.CombatScene.CombatMapRef == nil {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}

	identity := g.CombatScene.GetCurrentTurnIdentity()
	actorPosition, actorPresent := g.combatPushActorPosition(identity)
	objectPosition := direction.GetNewPositionInDirection(&actorPosition)
	if !actorPresent || objectPosition == nil {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}
	if g.CheckTrap(objectPosition) {
		return true
	}
	if !isCombatMapPosition(*objectPosition) || g.combatPushOccupied(*objectPosition) {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}

	object := g.CombatScene.CombatMapRef.GetTileNumber(*objectPosition)
	if !isOriginalPushableTile(object) {
		g.SystemCallbacks.Message.AddRowStr("Won't budge!")

		return false
	}
	legalFloor := originalPushLegalFloor(object)

	forwardPosition := direction.GetNewPositionInDirection(objectPosition)
	if forwardPosition != nil && isCombatMapPosition(*forwardPosition) &&
		!g.combatPushOccupied(*forwardPosition) &&
		g.CombatScene.CombatMapRef.GetTileNumber(*forwardPosition) == legalFloor {
		g.setCombatTerrain(*objectPosition, legalFloor)
		g.setCombatTerrain(*forwardPosition, originalPushFacing(object, direction))
		g.moveCombatPushActor(identity, actorPosition, *objectPosition)
		g.SystemCallbacks.Message.AddRowStr("Pushed!")

		return true
	}

	if g.CombatScene.CombatMapRef.GetTileNumber(actorPosition) == legalFloor {
		g.setCombatTerrain(actorPosition, originalPushFacing(object, direction.GetOppositeDirection()))
		g.setCombatTerrain(*objectPosition, legalFloor)
		g.moveCombatPushActor(identity, actorPosition, *objectPosition)
		g.SystemCallbacks.Message.AddRowStr("Pulled!")

		return true
	}

	g.SystemCallbacks.Message.AddRowStr("Won't budge")

	return false
}

func (g *GameState) combatPushActorPosition(identity CombatTurnIdentity) (references.Position, bool) {
	if identity.IsPartyMember {
		return g.CombatScene.GetCombatantPosition(identity),
			g.CombatScene.IsCombatPCActive(identity) && !g.CombatScene.IsCombatPCUnderground(identity)
	}
	monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
	if monster == nil || !monster.IsActive() || monster.IsDead() || !monster.IsPartyLoyal() {
		return references.Position{}, false
	}

	return monster.Position(), true
}

func (g *GameState) combatPushOccupied(position references.Position) bool {
	return g.CombatScene.isPartyMemberAtPosition(position, -1) ||
		g.CombatScene.MonsterManager.IsPositionOccupied(position) ||
		g.CombatScene.HasObjectAt(position) ||
		g.CombatScene.hasFieldAt(position)
}

func (g *GameState) moveCombatPushActor(
	identity CombatTurnIdentity,
	fromPosition references.Position,
	toPosition references.Position,
) {
	if identity.IsPartyMember {
		g.CombatScene.setCombatantPosition(identity, toPosition)
	} else {
		monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
		if monster == nil {
			panic("combat Push actor disappeared during resolution")
		}
		monster.SetPosition(toPosition)
	}
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventMove,
		Attacker: CombatFeedbackEntity{
			IsPartyMember: identity.IsPartyMember,
			Index:         identity.EntityIndex,
			Slot:          identity.Slot,
			Position:      toPosition,
		},
		From: fromPosition,
		To:   toPosition,
	})
}

// Helper methods - These could be moved to more appropriate files (map_state or collision system)
// when the codebase is restructured, but are kept here for now to support push functionality.

// ObjectPresentAt reports a visible map unit on the player's current floor.
func (g *GameState) ObjectPresentAt(position *references.Position) bool {
	if position == nil {
		return false
	}
	// Check if any NPC/object is present at this position AND on the same floor as the player
	// This implements floor-aware collision detection - objects on different floors don't block movement
	if g.CurrentNPCAIController == nil || g.CurrentNPCAIController.GetNpcs() == nil {
		return false
	}
	playerFloor := g.MapState.PlayerLocation.Floor
	mapUnit := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(*position, playerFloor)
	if mapUnit == nil {
		return false
	}

	// Quest item conditional visibility - hidden quest item NPCs don't block movement
	if g.isQuestItemNPCHidden(*mapUnit) {
		return false
	}

	return true
}

// Note: IsInCombat() check removed from ActionPushSmallMap since small maps are never combat maps
