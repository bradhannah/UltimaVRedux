package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// DungeonRoomApproach describes where the party can stand so that one forward
// step enters a fixed dungeon room. Dungeon maps wrap, so a room on the map edge
// is approached from the opposite edge.
type DungeonRoomApproach struct {
	Room              int
	Floor             references.FloorNumber
	RoomCell          references.Position
	Available         bool
	UnavailableReason string
	Approach          references.Position
	Facing            references.DungeonFacing
}

// DungeonRoomApproaches lists every fixed room placed in a dungeon, ordered by
// floor and then room number. It reads the reference dungeon cells, as
// DungeonRoomPlacement does, and changes no game state.
func (g *GameState) DungeonRoomApproaches(location references.Location) []DungeonRoomApproach {
	if _, hasRooms := references.DungeonLocationToCombatIndex(location); !hasRooms {
		return nil
	}
	reference := g.GameReferences.LocationReferences.GetLocationReference(location)
	var approaches []DungeonRoomApproach
	seen := map[int]bool{}
	for floor := range references.FloorNumber(references.DungeonFloorCount) {
		floorRooms := map[int]DungeonRoomApproach{}
		for y := range references.YDungeonMapTiles {
			for x := range references.XDungeonMapTiles {
				position := references.Position{X: x, Y: y}
				cell, found := reference.GetDungeonCell(floor, position)
				if !found || !isDungeonRoomCell(cell) {
					continue
				}
				room := int(cell.Payload())
				if seen[room] {
					continue
				}
				seen[room] = true
				approach := DungeonRoomApproach{Room: room, Floor: floor, RoomCell: position}
				approach.resolve(func(candidate references.Position) (references.DungeonCell, bool) {
					return reference.GetDungeonCell(floor, candidate)
				})
				floorRooms[room] = approach
			}
		}
		for room := range references.MaxDungeonRooms {
			if approach, placed := floorRooms[room]; placed {
				approaches = append(approaches, approach)
			}
		}
	}

	return approaches
}

func (a *DungeonRoomApproach) resolve(cellAt func(references.Position) (references.DungeonCell, bool)) {
	var fallback *DungeonRoomApproach
	// Fixed neighbour order: stand north, east, south, then west of the room, facing back at it.
	for _, facing := range [...]references.DungeonFacing{
		references.DungeonFacingSouth,
		references.DungeonFacingWest,
		references.DungeonFacingNorth,
		references.DungeonFacingEast,
	} {
		candidate := facing.TurnAround().Direction().GetNewPositionInDirection(&a.RoomCell).GetWrapped(
			references.XDungeonMapTiles,
			references.YDungeonMapTiles,
		)
		cell, ok := cellAt(*candidate)
		if !ok || isDungeonRoomCell(cell) || cell.Raw() == 0x83 ||
			!dungeonCellAllowsMovement(cell.Kind(), DungeonAdvance) {
			continue
		}
		found := *a
		found.Available, found.Approach, found.Facing = true, *candidate, facing
		if cell.Kind() == references.DungeonPassage {
			*a = found

			return
		}
		if fallback == nil {
			fallback = &found
		}
	}
	if fallback != nil {
		*a = *fallback

		return
	}
	a.UnavailableReason = fmt.Sprintf(
		"Room %d has no open cell beside it to stand on and step in from", a.Room,
	)
}
