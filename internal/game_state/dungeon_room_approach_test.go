package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestDungeonRoomApproachesListEveryPlacedRoomWithAStandingCell(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).Build()

	for _, location := range []references.Location{references.Deceit, references.Covetous, references.Doom} {
		reference := gs.GameReferences.LocationReferences.GetLocationReference(location)
		dungeon, _ := references.DungeonLocationToCombatIndex(location)
		approaches := gs.DungeonRoomApproaches(location)
		require.NotEmpty(t, approaches, location.String())
		for index, approach := range approaches {
			placement, placed := gs.DungeonRoomPlacement(dungeon, approach.Room)
			require.True(t, placed)
			assert.Equal(t, placement, DungeonRoomPlacement{Floor: approach.Floor, Cell: approach.RoomCell},
				"%s room %d uses the same cell as DungeonRoomPlacement", location, approach.Room)
			if index > 0 {
				previous := approaches[index-1]
				assert.True(t, previous.Floor < approach.Floor ||
					previous.Floor == approach.Floor && previous.Room < approach.Room, "ordered by floor, then room")
			}
			if !approach.Available {
				assert.NotEmpty(t, approach.UnavailableReason)

				continue
			}
			standing, _ := reference.GetDungeonCell(approach.Floor, approach.Approach)
			assert.False(t, isDungeonRoomCell(standing), "%s room %d", location, approach.Room)
			assert.NotEqual(t, byte(0x83), standing.Raw(), "never the electric field")
			assert.True(t, dungeonCellAllowsMovement(standing.Kind(), DungeonAdvance))
			stepped := approach.Facing.Direction().GetNewPositionInDirection(&approach.Approach).GetWrapped(
				references.XDungeonMapTiles, references.YDungeonMapTiles)
			assert.Equal(t, approach.RoomCell, *stepped, "one step forward enters the room")
		}
	}
}

func TestDungeonRoomApproachPrefersPlainPassagesAndWrapsAroundTheMapEdge(t *testing.T) {
	room := DungeonRoomApproach{Room: 3, RoomCell: references.Position{X: 0, Y: 0}}
	cells := map[references.Position]references.DungeonCell{
		{X: 0, Y: 7}: references.DungeonCell(0x60), // north, wrapped: a trap
		{X: 1, Y: 0}: references.DungeonCell(0xB0), // east: wall
		{X: 0, Y: 1}: references.DungeonCell(0x00), // south: plain passage
		{X: 7, Y: 0}: references.DungeonCell(0x00), // west, wrapped: plain passage, but south comes first
	}

	room.resolve(func(position references.Position) (references.DungeonCell, bool) {
		cell, ok := cells[position]

		return cell, ok
	})

	assert.True(t, room.Available)
	assert.Equal(t, references.Position{X: 0, Y: 1}, room.Approach, "the first plain passage in north, east, south, west order")
	assert.Equal(t, references.DungeonFacingNorth, room.Facing)

	cells[references.Position{X: 0, Y: 1}] = references.DungeonCell(0x83)
	cells[references.Position{X: 7, Y: 0}] = references.DungeonCell(0xA0) // a room, not a place to stand
	room.Available = false
	room.resolve(func(position references.Position) (references.DungeonCell, bool) {
		cell, ok := cells[position]

		return cell, ok
	})
	assert.True(t, room.Available, "a trap is acceptable when no passage is open")
	assert.Equal(t, references.Position{X: 0, Y: 7}, room.Approach)
	assert.Equal(t, references.DungeonFacingSouth, room.Facing, "stood north, wrapped, facing back at the room")

	cells[references.Position{X: 0, Y: 7}] = references.DungeonCell(0xB0)
	room = DungeonRoomApproach{Room: 3, RoomCell: references.Position{X: 0, Y: 0}}
	room.resolve(func(position references.Position) (references.DungeonCell, bool) {
		cell, ok := cells[position]

		return cell, ok
	})
	assert.False(t, room.Available)
	assert.Contains(t, room.UnavailableReason, "Room 3")
}
