package references

import (
	"errors"
	"fmt"
	"os"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Static errors for combat map operations
var (
	ErrCombatMapIndexOutOfRange = errors.New("combat map index out of range")
)

// CombatMapReference represents a single combat map (camping, combat, etc.)
// Following the pattern of SmallLocationReference
type CombatMapReference struct {
	// Raw map data - single floor since combat maps don't have multiple levels
	rawData *[XCombatMapTiles][YCombatMapTiles]byte
	// dungeonTriggerSlots is present only when this reference was parsed from
	// DUNGEON.CBT. Generated and Britannia maps deliberately have no slots, so
	// an all-zero source slot (including trigger coordinate 0,0) stays valid.
	dungeonTriggerSlots *[DungeonCombatTriggerSlotCount]DungeonCombatMapTrigger
	// dungeonDualLadderPosition preserves encounter-local ladder direction metadata; ref:DNG-0010.
	dungeonDualLadderPosition  Position
	hasDungeonDualLadder       bool
	generatedDungeonChest      *GeneratedDungeonChest
	generatedChestMaterialized bool

	// Player starting positions for each entry direction
	PlayerPositions [NumDirections][MaxPlayersPerMap]Position

	// Monster entry points
	MonsterPositions [MaxMonstersPerMap]Position
	// DungeonEntityTiles preserves row-5 fixed-room entity descriptors; ref:DNG-0012.
	DungeonEntityTiles [MaxMonstersPerMap]byte

	// Map metadata
	MapIndex  int
	Territory CombatMapTerritory
	MapName   string
}

// GeneratedDungeonChest retains the source depth and underlying terrain for a
// generated 0xDC center marker until gameplay materializes the chest object.
type GeneratedDungeonChest struct {
	Position    Position
	SourceFloor FloorNumber
	FloorTile   indexes.SpriteIndex
}

// GeneratedDungeonChest returns detached metadata for the generated center chest.
func (c *CombatMapReference) GeneratedDungeonChest() (GeneratedDungeonChest, bool) {
	if c == nil || c.generatedDungeonChest == nil {
		return GeneratedDungeonChest{}, false
	}

	return *c.generatedDungeonChest, true
}

// MaterializeGeneratedDungeonChest replaces the intermediate display marker
// with its source-style floor exactly once.
func (c *CombatMapReference) MaterializeGeneratedDungeonChest() (GeneratedDungeonChest, bool) {
	chest, found := c.GeneratedDungeonChest()
	if !found || c.generatedChestMaterialized {
		return GeneratedDungeonChest{}, false
	}
	c.SetTileNumber(chest.Position, chest.FloorTile)
	c.generatedChestMaterialized = true

	return chest, true
}

// IsDungeonDualLadder reports whether a generated dungeon encounter offers
// both vertical routes from its visually up-facing center ladder.
func (c *CombatMapReference) IsDungeonDualLadder(position Position) bool {
	return c != nil && c.hasDungeonDualLadder && c.dungeonDualLadderPosition == position
}

// DungeonCombatMapTrigger is one ordered room-trigger entry; ref:DNG-0013.
type DungeonCombatMapTrigger struct {
	ReplacementTile indexes.SpriteIndex
	Trigger         Position
	Destinations    [2]Position
}

// NewCombatMapReference creates a new combat map reference
func NewCombatMapReference(mapIndex int, territory CombatMapTerritory) *CombatMapReference {
	cmr := &CombatMapReference{
		MapIndex:  mapIndex,
		Territory: territory,
	}

	// Set friendly name based on map index and territory
	if territory == BritanniaCombatTerritory {
		switch BritanniaCombatMap(mapIndex) {
		case CampFireCombatMap:
			cmr.MapName = "Camping"
		case SwampCombatMap:
			cmr.MapName = "Swamp Combat"
		case GladeCombatMap:
			cmr.MapName = "Glade Combat"
		case TreedCombatMap:
			cmr.MapName = "Treed Combat"
		case DesertCombatMap:
			cmr.MapName = "Desert Combat"
		case CleanTreeCombatMap:
			cmr.MapName = "Clean Tree Combat"
		case MountainsCombatMap:
			cmr.MapName = "Mountains Combat"
		case BigBridgeCombatMap:
			cmr.MapName = "Big Bridge Combat"
		case BrickCombatMap:
			cmr.MapName = "Brick Combat"
		case BasementCombatMap:
			cmr.MapName = "Basement"
		case ShadowlordCombatMap:
			cmr.MapName = "Shadowlord"
		case ShipSeaCombatMap:
			cmr.MapName = "Ship-Sea"
		case LandShipCombatMap:
			cmr.MapName = "Land-Ship"
		case ShipShoreCombatMap:
			cmr.MapName = "Ship-Shore"
		case ShipShipCombatMap:
			cmr.MapName = "Ship-Ship"
		case LandSeaCombatMap:
			cmr.MapName = "Land-Sea"
		default:
			cmr.MapName = "Unknown Combat Map"
		}
	} else {
		cmr.MapName = "Dungeon Combat Map"
	}

	return cmr
}

// GetTileNumber returns the sprite index at a given position
func (c *CombatMapReference) GetTileNumber(xy Position) indexes.SpriteIndex {
	if c.rawData == nil {
		return indexes.SpriteIndex(0)
	}

	return indexes.SpriteIndex(c.rawData[xy.X][xy.Y])
}

// SetRawData sets the raw map data after loading from file
func (c *CombatMapReference) SetRawData(data *[XCombatMapTiles][YCombatMapTiles]byte) {
	c.rawData = data
}

// SetTileNumber replaces one mutable combat-map tile during an encounter.
func (c *CombatMapReference) SetTileNumber(xy Position, tile indexes.SpriteIndex) {
	if c.rawData == nil || xy.X < 0 || xy.X >= XCombatMapTiles || xy.Y < 0 || xy.Y >= YCombatMapTiles {
		panic("combat tile position out of bounds or map data unavailable")
	}
	c.rawData[xy.X][xy.Y] = byte(tile)
}

// DungeonTriggerSlots returns a detached snapshot and whether this map has
// DUNGEON.CBT trigger metadata. The bool, rather than coordinate sentinels,
// distinguishes an active (0,0) trigger from a map with no trigger slots.
func (c *CombatMapReference) DungeonTriggerSlots() (
	[DungeonCombatTriggerSlotCount]DungeonCombatMapTrigger,
	bool,
) {
	if c.dungeonTriggerSlots == nil {
		return [DungeonCombatTriggerSlotCount]DungeonCombatMapTrigger{}, false
	}

	return *c.dungeonTriggerSlots, true
}

// ResolveDungeonTrigger consumes every slot matching triggerPosition and
// applies independently valid destinations in source order. Later slots win
// when destinations overlap.
func (c *CombatMapReference) ResolveDungeonTrigger(triggerPosition Position) bool {
	if c.dungeonTriggerSlots == nil {
		return false
	}

	handled := false
	for slotIndex := range c.dungeonTriggerSlots {
		slot := &c.dungeonTriggerSlots[slotIndex]
		if slot.Trigger != triggerPosition {
			continue
		}
		slot.Trigger = Position{
			X: DungeonTriggerConsumedPosition,
			Y: DungeonTriggerConsumedPosition,
		}
		for _, destination := range slot.Destinations {
			if destination.X >= XCombatMapTiles || destination.Y >= YCombatMapTiles {
				continue
			}
			c.SetTileNumber(destination, slot.ReplacementTile)
		}
		handled = true
	}

	return handled
}

// GetCampfirePosition returns the center position where campfire is placed
func (c *CombatMapReference) GetCampfirePosition() Position {
	return Position{X: CampfirePositionX, Y: CampfirePositionY}
}

// LoadCombatMap loads a specific combat map from the appropriate CBT file
func LoadCombatMap(mapIndex int, territory CombatMapTerritory, gameConfig *config.UltimaVConfiguration) (*CombatMapReference, error) {
	// Create the reference
	cmr := NewCombatMapReference(mapIndex, territory)

	// Get the appropriate CBT file path
	cbtFile := GetCombatMapFile(territory)
	fullPath := files.Resolve(gameConfig.SavedConfigData.DataFilePath, cbtFile)

	// Read the entire CBT file
	data, err := os.ReadFile(fullPath)
	if err != nil {
		return nil, fmt.Errorf("reading CBT file %s: %w", cbtFile, err)
	}

	// Calculate offset for the requested map
	offset := mapIndex * CombatMapDataSize
	if offset+CombatMapDataSize > len(data) {
		return nil, fmt.Errorf("%w: map index %d in %s", ErrCombatMapIndexOutOfRange, mapIndex, cbtFile)
	}

	// Extract map data
	mapData := &[XCombatMapTiles][YCombatMapTiles]byte{}

	// Parse the map tiles from rows 0-10 (first CombatMapTileDataBytes bytes of each row)
	// Each of the 11 rows contains 11 bytes of tile data (11×11 grid)
	for row := range YCombatMapTiles {
		rowOffset := offset + int(row)*CombatMapBytesPerRow
		for col := range CombatMapTileDataBytes {
			mapData[col][row] = data[rowOffset+col]
		}
	}

	cmr.SetRawData(mapData)

	// Parse player positions (rows FirstDirectionRow to FirstDirectionRow+NumDirections-1)
	for dir := range NumDirections {
		dirRow := FirstDirectionRow + dir
		rowOffset := offset + (dirRow * CombatMapBytesPerRow)

		for player := range MaxPlayersPerMap {
			xCoord := data[rowOffset+PlayerXCoordOffset+player]
			yCoord := data[rowOffset+PlayerYCoordOffset+player]
			cmr.PlayerPositions[dir][player] = Position{
				X: Coordinate(xCoord),
				Y: Coordinate(yCoord),
			}
		}
	}

	// Original BRIT.CBT metadata stores all monster X coordinates (ref:CBT-0002)
	// in row 6 and all Y coordinates in row 7.
	monsterXRowOffset := offset + (MonsterXCoordRow * CombatMapBytesPerRow)
	monsterYRowOffset := offset + (MonsterYCoordRow * CombatMapBytesPerRow)
	for monsterIndex := range MaxMonstersPerMap {
		cmr.DungeonEntityTiles[monsterIndex] = data[offset+DungeonEntityRow*CombatMapBytesPerRow+DungeonEntityOffset+monsterIndex]
		cmr.MonsterPositions[monsterIndex] = Position{
			X: Coordinate(data[monsterXRowOffset+MonsterCoordOffset+monsterIndex]),
			Y: Coordinate(data[monsterYRowOffset+MonsterCoordOffset+monsterIndex]),
		}
	}

	if territory == DungeonCombatTerritory {
		triggerSlots := &[DungeonCombatTriggerSlotCount]DungeonCombatMapTrigger{}
		for slotIndex := range DungeonCombatTriggerSlotCount {
			triggerSlots[slotIndex] = DungeonCombatMapTrigger{
				ReplacementTile: indexes.SpriteIndex(data[offset+DungeonTriggerTileRow*CombatMapBytesPerRow+DungeonTriggerXOffset+slotIndex]),
				Trigger: Position{
					X: Coordinate(data[offset+DungeonTriggerCoordinateRow*CombatMapBytesPerRow+DungeonTriggerXOffset+slotIndex]),
					Y: Coordinate(data[offset+DungeonTriggerCoordinateRow*CombatMapBytesPerRow+DungeonTriggerYOffset+slotIndex]),
				},
				Destinations: [2]Position{
					{
						X: Coordinate(data[offset+DungeonTriggerDestination1Row*CombatMapBytesPerRow+DungeonTriggerXOffset+slotIndex]),
						Y: Coordinate(data[offset+DungeonTriggerDestination1Row*CombatMapBytesPerRow+DungeonTriggerYOffset+slotIndex]),
					},
					{
						X: Coordinate(data[offset+DungeonTriggerDestination2Row*CombatMapBytesPerRow+DungeonTriggerXOffset+slotIndex]),
						Y: Coordinate(data[offset+DungeonTriggerDestination2Row*CombatMapBytesPerRow+DungeonTriggerYOffset+slotIndex]),
					},
				},
			}
		}
		cmr.dungeonTriggerSlots = triggerSlots
	}

	logging.Debugf(logging.CategorySystem, "Loaded combat map %d from %s", mapIndex, cbtFile)

	return cmr, nil
}

// LoadCampingMap is a convenience function to load the camping map specifically
func LoadCampingMap(gameConfig *config.UltimaVConfiguration) (*CombatMapReference, error) {
	campingMap, err := LoadCombatMap(int(CampFireCombatMap), BritanniaCombatTerritory, gameConfig)
	if err != nil {
		return nil, fmt.Errorf("loading camping map: %w", err)
	}

	// Override the campfire tile at center
	if campingMap.rawData != nil {
		campfirePos := campingMap.GetCampfirePosition()
		campingMap.rawData[campfirePos.X][campfirePos.Y] = byte(indexes.CampFire)
	}

	return campingMap, nil
}
