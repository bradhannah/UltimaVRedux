package references

import (
	"github.com/bradhannah/UltimaVRedux/internal/files"
)

// Combat map dimensions (11x11 maps like camping, shrine, codex)
const (
	XCombatMapTiles = Coordinate(11)
	YCombatMapTiles = Coordinate(11)
)

// CBT file structure constants
//
//goland:noinspection GoUnusedConst
const (
	CombatMapBytesPerRow   = 0x20  // 32 bytes per row in CBT file
	CombatMapDataSize      = 0x160 // 352 bytes per map (11 rows * 32 bytes)
	CombatMapTileDataBytes = 11    // First 11 bytes of each row contain tile data
	TotalCombatMaps        = 16    // Total number of combat maps in BRIT.CBT (not 10)

	// Row structure in CBT data
	TileDataRow       = 0 // Row 0: Map tiles (11×11 grid stored linearly)
	FirstDirectionRow = 1 // Row 1-4: Player positions for 4 entry directions
	NumDirections     = 4 // 4 entry directions (N, S, E, W)
	MonsterXCoordRow  = 6 // Row 6: X coordinates for all 16 monster positions
	MonsterYCoordRow  = 7 // Row 7: Y coordinates for all 16 monster positions
	DungeonEntityRow  = 5 // Row 5: Fixed dungeon-room entity tile descriptors
	LastDataRow       = 7 // Last row with meaningful metadata

	// DUNGEON.CBT trigger metadata. Original room processing reads eight
	// parallel entries from rows 0 and 8-10 after the 11 terrain bytes.
	DungeonTriggerTileRow          = 0
	DungeonTriggerCoordinateRow    = 8
	DungeonTriggerDestination1Row  = 9
	DungeonTriggerDestination2Row  = 10
	DungeonTriggerXOffset          = 11
	DungeonTriggerYOffset          = 19
	DungeonCombatTriggerSlotCount  = 8
	DungeonTriggerConsumedPosition = Coordinate(0xff)

	// Player position offsets in direction rows (rows 1-4)
	PlayerXCoordOffset = 11 // X coordinates start at byte 11
	PlayerYCoordOffset = 17 // Y coordinates start at byte 17 (11 + 6)
	MaxPlayersPerMap   = 6  // Maximum 6 party members

	// Monster position offsets in rows 6-7
	MonsterCoordOffset  = 11 // Coordinates start at byte 11 in each metadata row
	DungeonEntityOffset = 11 // Entity descriptors start at byte 11 in row 5
	MaxMonstersPerMap   = 16 // Maximum 16 monsters per map
)

// Camping map specific constants
const (
	CampfirePositionX = 5 // Center of 11x11 map
	CampfirePositionY = 5 // Center of 11x11 map
)

// CombatMapTerritory represents which CBT file to use
type CombatMapTerritory int

// Combat map files: BritanniaCombatTerritory selects BRIT.CBT (overworld and towns), DungeonCombatTerritory the dungeon file.
const (
	BritanniaCombatTerritory CombatMapTerritory = iota
	DungeonCombatTerritory
)

// BritanniaCombatMap indices into BRIT.CBT
type BritanniaCombatMap int

// BritanniaCombatMap constants for named combat map indices in BRIT.CBT
const (
	CampFireCombatMap   BritanniaCombatMap = 0  // Camping mode
	SwampCombatMap      BritanniaCombatMap = 1  // SWAMP terrain
	GladeCombatMap      BritanniaCombatMap = 2  // GRASS terrain (default outdoor)
	TreedCombatMap      BritanniaCombatMap = 3  // BRUSH, THICK_BRUSH terrain
	DesertCombatMap     BritanniaCombatMap = 4  // DESERT, WASTE terrain
	CleanTreeCombatMap  BritanniaCombatMap = 5  // FOREST, TROPICAL terrain
	MountainsCombatMap  BritanniaCombatMap = 6  // HILLS, MOUNTAINS, PEAKS terrain
	BigBridgeCombatMap  BritanniaCombatMap = 7  // BRIDGE tiles
	BrickCombatMap      BritanniaCombatMap = 8  // FLOOR terrain, indoor towns
	BasementCombatMap   BritanniaCombatMap = 9  // Default town combat
	ShadowlordCombatMap BritanniaCombatMap = 10 // SHADOW_LORD encounters only
	ShipSeaCombatMap    BritanniaCombatMap = 11 // Ship attacking sea monsters
	LandShipCombatMap   BritanniaCombatMap = 12 // Land party vs pirate ship
	ShipShoreCombatMap  BritanniaCombatMap = 13 // Ship vs land monsters
	ShipShipCombatMap   BritanniaCombatMap = 14 // Ship vs pirate ship combat
	LandSeaCombatMap    BritanniaCombatMap = 15 // Land party vs sea monsters
)

// GetCombatMapFile returns the appropriate CBT file for a territory
func GetCombatMapFile(territory CombatMapTerritory) string {
	switch territory {
	case BritanniaCombatTerritory:
		return files.BritCbt
	case DungeonCombatTerritory:
		return files.DungeonCbt
	default:
		panic("unhandled combat map territory")
	}
}

// DungeonCombatIndex maps dungeon names to indices for DUNGEON.CBT.
// Note: Despise has no rooms, so it's skipped in the index calculation.
// DUNGEON.CBT stores 16 rooms per dungeon, indexed linearly.
type DungeonCombatIndex int

// Dungeon combat index constants for DUNGEON.CBT
const (
	DeceitDungeon   DungeonCombatIndex = 0
	DestardDungeon  DungeonCombatIndex = 1 // Despise skipped (no rooms)
	WrongDungeon    DungeonCombatIndex = 2
	CovetousDungeon DungeonCombatIndex = 3
	ShameDungeon    DungeonCombatIndex = 4
	HythlothDungeon DungeonCombatIndex = 5
	DoomDungeon     DungeonCombatIndex = 6
)

// Dungeon combat constants
const (
	MaxDungeonRooms       = 16  // 16 rooms per dungeon
	MaxDungeonCombatIndex = 6   // Maximum dungeon index (0-6)
	MaxDungeonLinearIndex = 111 // 7 dungeons * 16 rooms - 1 = 111
)

// DungeonNames returns the list of valid dungeon names for debug commands
func DungeonNames() []string {
	return []string{"deceit", "destard", "wrong", "covetous", "shame", "hythloth", "doom"}
}

// CalculateDungeonCombatMapIndex calculates the linear map index for dungeon combat.
// Formula: linearIndex = dungeonIndex * 16 + roomIndex
func CalculateDungeonCombatMapIndex(dungeonIndex DungeonCombatIndex, roomIndex int) int {
	return int(dungeonIndex)*MaxDungeonRooms + roomIndex
}

// DungeonCombatIndexLocation is the inverse of DungeonLocationToCombatIndex.
func DungeonCombatIndexLocation(index DungeonCombatIndex) (Location, bool) {
	for location := Deceit; location <= Doom; location++ {
		if candidate, ok := DungeonLocationToCombatIndex(location); ok && candidate == index {
			return location, true
		}
	}

	return 0, false
}

// DungeonLocationToCombatIndex converts a room-bearing dungeon location to its
// compressed DUNGEON.CBT index. Despise has no fixed-room records.
func DungeonLocationToCombatIndex(location Location) (DungeonCombatIndex, bool) {
	//nolint:exhaustive // Only seven room-bearing dungeon locations have DUNGEON.CBT indexes.
	switch location {
	case Deceit:
		return DeceitDungeon, true
	case Destard:
		return DestardDungeon, true
	case Wrong:
		return WrongDungeon, true
	case Covetous:
		return CovetousDungeon, true
	case Shame:
		return ShameDungeon, true
	case Hythloth:
		return HythlothDungeon, true
	case Doom:
		return DoomDungeon, true
	default:
		return 0, false
	}
}
