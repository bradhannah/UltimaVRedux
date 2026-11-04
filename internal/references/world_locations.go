package references

import (
	"fmt"
	"io"

	"github.com/bradhannah/UltimaVRedux/internal/config"
)

const (
	xCoordsOffset  = 0x1e9a
	yCoordsOffset  = 0x1ec2
	totalLocations = 40
)

// WorldLocations holds the overworld position of each location, read from DATA.OVL.
type WorldLocations struct {
	LargeMapLocationPositions map[Location]WorldLocation `json:"largeMapLocationPositions" yaml:"largeMapLocationPositions"`
	// shrinePositions
}

// WorldLocation is a location with its overworld position and floor.
type WorldLocation struct {
	Position Position    `json:"position" yaml:"position"`
	Location Location    `json:"location" yaml:"location"`
	Floor    FloorNumber `json:"floor"    yaml:"floor"`
}

// NewWorldLocations loads location coordinates from the configured DATA.OVL bytes.
func NewWorldLocations(gameConfig *config.UltimaVConfiguration) (*WorldLocations, error) {
	requiredLength := yCoordsOffset + totalLocations
	if len(gameConfig.RawDataOvl) < requiredLength {
		return nil, fmt.Errorf("reading world locations: expected at least %d DATA.OVL bytes, got %d: %w",
			requiredLength, len(gameConfig.RawDataOvl), io.ErrUnexpectedEOF)
	}

	wl := &WorldLocations{}
	wl.LargeMapLocationPositions = make(map[Location]WorldLocation)

	for i := range totalLocations {
		x := Coordinate(gameConfig.RawDataOvl[xCoordsOffset+i])
		y := Coordinate(gameConfig.RawDataOvl[yCoordsOffset+i])
		location := Location(i + 1)

		// Ararat is the only non-dungeon location in the Underworld. Dungeon exits
		// ignore this floor: climb direction picks Britannia or the Underworld
		// (exitDungeonToWorld; ref:DNG-0003).
		floor := FloorNumber(OVERWORLD)
		if location == Ararat {
			floor = FloorNumber(UNDERWORLD)
		}

		wl.LargeMapLocationPositions[location] = WorldLocation{
			Location: location,
			Position: Position{
				X: x,
				Y: y,
			},
			Floor: floor,
		}
	}

	return wl, nil
}

// GetLocationByPosition returns the location entered at the position, or EmptyLocation.
func (wl *WorldLocations) GetLocationByPosition(position Position) Location {
	for _, worldLocation := range wl.LargeMapLocationPositions {
		if worldLocation.Position.Equals(&position) {
			return worldLocation.Location
		}
	}

	return EmptyLocation
}

// GetWorldLocation returns the world map position and floor for a location.
// Used when exiting towns/dungeons to determine where player appears on large map.
// Panics if location not found (programming error - all valid locations 1-40 should exist).
func (wl *WorldLocations) GetWorldLocation(location Location) (Position, FloorNumber) {
	worldLoc, exists := wl.LargeMapLocationPositions[location]
	if !exists {
		panic("GetWorldLocation: location not found in world locations map - programming error")
	}

	return worldLoc.Position, worldLoc.Floor
}
