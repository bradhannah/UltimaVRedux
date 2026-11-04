package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// CombatEncounterEntityKind classifies one authored fixed-room descriptor. See ref:DNG-0012.
type CombatEncounterEntityKind int

const (
	// CombatEncounterEntityMonster is a fixed monster type.
	CombatEncounterEntityMonster CombatEncounterEntityKind = iota
	// CombatEncounterEntityRandomMonster is drawn from the room's four variable creatures.
	CombatEncounterEntityRandomMonster
	// CombatEncounterEntityObject is a non-collectible object such as a dead body.
	CombatEncounterEntityObject
	// CombatEncounterEntityLoot is a chest or collectible item.
	CombatEncounterEntityLoot
	// CombatEncounterEntityField is a magic field.
	CombatEncounterEntityField
)

// DungeonRoomPlacement is the dungeon level and map cell that hold a fixed room.
type DungeonRoomPlacement struct {
	Floor references.FloorNumber
	Cell  references.Position
}

// CombatEncounterPreviewEntity is one authored fixed-room entity as it would load.
type CombatEncounterPreviewEntity struct {
	Kind      CombatEncounterEntityKind
	Name      string
	Sprite    indexes.SpriteIndex
	Position  references.Position
	Concealed bool
}

// CombatEncounterPreview describes a combat map before an encounter starts on it.
type CombatEncounterPreview struct {
	// Tiles are row-major: Tiles[y][x].
	Tiles [references.YCombatMapTiles][references.XCombatMapTiles]indexes.SpriteIndex
	// PartyPositions are indexed by CombatEntryDirection.
	PartyPositions [references.NumDirections][references.MaxPlayersPerMap]references.Position
	MonsterSlots   []references.Position
	// Entities and Triggers are empty outside fixed dungeon rooms.
	Entities []CombatEncounterPreviewEntity
	Triggers []references.Position
}

// CombatEncounterPreview loads one encounter map without changing game state.
func (g *GameState) CombatEncounterPreview(encounterMap CombatEncounterMap) (CombatEncounterPreview, error) {
	preview := CombatEncounterPreview{}
	mapIndex, err := resolveCombatEncounterMap(encounterMap)
	if err != nil {
		return preview, err
	}
	combatMap, err := references.LoadCombatMap(mapIndex, encounterMap.Territory, g.GameConfig)
	if err != nil {
		return preview, fmt.Errorf("loading encounter combat map %d: %w", mapIndex, err)
	}
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			preview.Tiles[y][x] = combatMap.GetTileNumber(references.Position{X: x, Y: y})
		}
	}
	preview.PartyPositions = combatMap.PlayerPositions
	for _, position := range combatMap.MonsterPositions {
		if isCombatMapPosition(position) {
			preview.MonsterSlots = append(preview.MonsterSlots, position)
		}
	}
	if encounterMap.Territory != references.DungeonCombatTerritory {
		return preview, nil
	}
	for slot, descriptor := range combatMap.DungeonEntityTiles {
		position := combatMap.MonsterPositions[slot]
		if descriptor == 0 || !isCombatMapPosition(position) {
			continue
		}
		entity, entityErr := g.fixedDungeonPreviewEntity(descriptor, position)
		if entityErr != nil {
			return preview, fmt.Errorf("fixed dungeon room slot %d: %w", slot, entityErr)
		}
		entity.Concealed = CombatTerrainConceals(combatMap.GetTileNumber(position))
		preview.Entities = append(preview.Entities, entity)
	}
	if slots, ok := combatMap.DungeonTriggerSlots(); ok {
		seen := map[references.Position]bool{}
		for _, slot := range slots {
			unused := slot.Trigger == (references.Position{}) && slot.ReplacementTile == 0
			if !unused && isCombatMapPosition(slot.Trigger) && !seen[slot.Trigger] {
				seen[slot.Trigger] = true
				preview.Triggers = append(preview.Triggers, slot.Trigger)
			}
		}
	}

	return preview, nil
}

// DungeonRoomPlacement finds the dungeon level and cell that hold a fixed room.
func (g *GameState) DungeonRoomPlacement(
	dungeon references.DungeonCombatIndex,
	room int,
) (DungeonRoomPlacement, bool) {
	location, ok := references.DungeonCombatIndexLocation(dungeon)
	if !ok {
		return DungeonRoomPlacement{}, false
	}
	reference := g.GameReferences.LocationReferences.GetLocationReference(location)
	for floor := range references.FloorNumber(references.DungeonFloorCount) {
		for y := range references.YDungeonMapTiles {
			for x := range references.XDungeonMapTiles {
				position := references.Position{X: x, Y: y}
				cell, found := reference.GetDungeonCell(floor, position)
				if found && isDungeonRoomCell(cell) && int(cell.Payload()) == room {
					return DungeonRoomPlacement{Floor: floor, Cell: position}, true
				}
			}
		}
	}

	return DungeonRoomPlacement{}, false
}

func (g *GameState) fixedDungeonPreviewEntity(
	descriptor byte,
	position references.Position,
) (CombatEncounterPreviewEntity, error) {
	kind := fixedDungeonEntityKind(descriptor)
	entity := CombatEncounterPreviewEntity{Kind: kind, Position: position, Sprite: fixedDungeonDescriptorSprite(descriptor)}
	switch kind {
	case CombatEncounterEntityMonster:
		enemyRef, err := g.enemyReferenceForCombat(fixedDungeonMonsterType(descriptor))
		if err != nil {
			return entity, err
		}
		entity.Name, entity.Sprite = enemyRef.Name, indexes.NoSprites
		if enemyRef.KeyFrameTile != nil {
			entity.Sprite = enemyRef.KeyFrameTile.Index
		}

		return entity, nil
	case CombatEncounterEntityRandomMonster:
		entity.Name, entity.Sprite = "Random monster", indexes.NoSprites

		return entity, nil
	case CombatEncounterEntityField:
		entity.Sprite = NewCombatField(CombatFieldKind(descriptor&3), position).Sprite()
	case CombatEncounterEntityLoot:
		if descriptor == 1 {
			entity.Sprite = indexes.Chest
		}
	case CombatEncounterEntityObject:
	}
	entity.Name = fmt.Sprintf("Object 0x%02x", descriptor)
	if tile := g.GameReferences.TileReferences.GetTile(entity.Sprite); tile != nil {
		entity.Name = tile.Name
	}

	return entity, nil
}

// fixedDungeonEntityKind is the one descriptor classification shared by room setup and preview.
func fixedDungeonEntityKind(descriptor byte) CombatEncounterEntityKind {
	base := descriptor & 0xfc
	switch {
	case base == 0xe8:
		return CombatEncounterEntityField
	case descriptor < originalDungeonNPCBaseTile || base == 0xb4:
		if descriptor >= 0x10 && base != 0xb4 {
			return CombatEncounterEntityObject
		}

		return CombatEncounterEntityLoot
	case base == originalDungeonNPCBaseTile+4*byte(references.MonsterWhirlpool):
		return CombatEncounterEntityRandomMonster
	default:
		return CombatEncounterEntityMonster
	}
}

func fixedDungeonMonsterType(descriptor byte) references.MonsterType {
	return references.MonsterType((descriptor - originalDungeonNPCBaseTile) / 4)
}

func fixedDungeonDescriptorSprite(descriptor byte) indexes.SpriteIndex {
	return indexes.SpriteIndex(int(indexes.StarPattern) + int(descriptor))
}
