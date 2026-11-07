package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestGetDungeonForWord(t *testing.T) {
	tests := []struct {
		name     string
		word     string
		expected references.Location
		exists   bool
	}{
		{"Valid word FALLAX", "FALLAX", references.Deceit, true},
		{"Valid word lowercase", "fallax", references.Deceit, true},
		{"Valid word mixed case", "FaLlAx", references.Deceit, true},
		{"Valid word with spaces", " FALLAX ", references.Deceit, true},
		{"All valid words", "VILIS", references.Despise, true},
		{"VERAMOCOR for Doom", "VERAMOCOR", references.Doom, true},
		{"Invalid word", "INVALID", references.Location(0), false},
		{"Empty word", "", references.Location(0), false},
		{"Partial word", "FALL", references.Location(0), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dungeon, exists := GetDungeonForWord(tt.word)
			assert.Equal(t, tt.expected, dungeon)
			assert.Equal(t, tt.exists, exists)
		})
	}
}

func TestGetDungeonForResponseRequiresCompleteCanonicalWordInsideInput(t *testing.T) {
	tests := []struct {
		name     string
		response string
		dungeon  references.Location
		matched  bool
	}{
		{name: "exact", response: "FALLAX", dungeon: references.Deceit, matched: true},
		{name: "contained", response: "speak fallax now", dungeon: references.Deceit, matched: true},
		{name: "canonical order", response: "VILIS then FALLAX", dungeon: references.Deceit, matched: true},
		{name: "prefix", response: "FALL", matched: false},
		{name: "suffix fragment", response: "ALLAX", matched: false},
		{name: "empty", response: "", matched: false},
		{name: "wrong", response: "BANANA", matched: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dungeon, matched := GetDungeonForResponse(test.response)
			assert.Equal(t, test.matched, matched)
			assert.Equal(t, test.dungeon, dungeon)
		})
	}
}

func TestDungeonOpenEntranceSpritesMatchOriginalProjection(t *testing.T) {
	tests := []struct {
		dungeon references.Location
		sprite  indexes.SpriteIndex
	}{
		{references.Deceit, indexes.DoomEntrance},
		{references.Despise, indexes.CaveEntrance},
		{references.Destard, indexes.CaveEntrance},
		{references.Wrong, indexes.DoomEntrance},
		{references.Covetous, indexes.DoomEntrance},
		{references.Shame, indexes.MineEntrance},
		{references.Hythloth, indexes.MineEntrance},
		{references.Doom, indexes.CaveEntrance},
	}
	for _, test := range tests {
		t.Run(test.dungeon.String(), func(t *testing.T) {
			assert.Equal(t, test.sprite, dungeonOpenEntranceSprite(test.dungeon))
		})
	}
}

func TestProcessWordOfPowerRequiresMatchingCardinalEntranceTileAndPlane(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.OVERWORLD)).
		WithSystemCallbacks().
		Build()
	entrance := state.GetDungeonEntranceCoordinates()[references.Deceit]
	state.MapState.PlayerLocation.Position = entrance.Right()
	state.GetLayeredMapByCurrentLocation().SetTileByLayer(
		map_state.MapLayer,
		&entrance,
		dungeonOpenEntranceSprite(references.Deceit),
	)

	assert.True(t, state.ProcessWordOfPower("I invoke FALLAX now"))
	assert.True(t, state.IsDungeonOpen(references.Deceit))
	assert.Empty(t, callbacks.Messages)

	state.SetDungeonState(references.Deceit, false)
	state.MapState.PlayerLocation.Position = entrance.Right().Right()
	assert.False(t, state.ProcessWordOfPower("FALLAX"))
	assert.False(t, state.IsDungeonOpen(references.Deceit))
	assert.Contains(t, callbacks.Messages, "Nothing happens.")
	callbacks.Reset()
	assert.False(t, state.ProcessWordOfPower(""))
	assert.Contains(t, callbacks.Messages, "Nothing happens.")

}

func TestProcessWordOfPowerTogglesNonDoomEntrancesFromTheUnderworld(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.UNDERWORLD)).
		WithSystemCallbacks().
		Build()
	for dungeon, entrance := range state.GetDungeonEntranceCoordinates() {
		if dungeon == references.Doom {
			continue
		}
		t.Run(dungeon.String(), func(t *testing.T) {
			raw := state.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(&entrance).Index
			require.True(t, raw == dungeonOpenEntranceSprite(dungeon) || raw == indexes.BlockEntrance,
				"the Underworld entrance must share the surface coordinate, got tile %d", raw)

			state.SetDungeonState(dungeon, false)
			state.MapState.PlayerLocation.Position = entrance.Right()
			require.Equal(t, indexes.SpriteIndex(indexes.BlockEntrance), state.GetEffectiveTile(&entrance))
			require.True(t, state.ProcessWordOfPower(wordForDungeon(dungeon)))
			assert.True(t, state.IsDungeonOpen(dungeon))
			assert.Empty(t, callbacks.Messages)

			state.MapState.PlayerLocation.Position = entrance
			assert.True(t, state.canEnterDungeonAtCurrentLocation(dungeon, state.GetEffectiveTile(&entrance)))
		})
	}
}

func TestProcessWordOfPowerRefusesDoomOnTheSurface(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.OVERWORLD)).
		WithSystemCallbacks().
		Build()
	entrance := state.GetDungeonEntranceCoordinates()[references.Doom]
	state.MapState.PlayerLocation.Position = entrance.Right()
	state.GetLayeredMapByCurrentLocation().SetTileByLayer(
		map_state.MapLayer,
		&entrance,
		dungeonOpenEntranceSprite(references.Doom),
	)

	assert.False(t, state.ProcessWordOfPower("VERAMOCOR"))
	assert.Contains(t, callbacks.Messages, "Nothing happens.")
}

func TestProcessWordOfPowerRejectsWrongTileAndUsesWestSouthEastNorthPriority(t *testing.T) {
	newState := func(t *testing.T) *GameState {
		t.Helper()
		state, _ := NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.OVERWORLD)).
			WithSystemCallbacks().
			Build()
		return state
	}

	t.Run("wrong entrance tile", func(t *testing.T) {
		state := newState(t)
		entrance := state.GetDungeonEntranceCoordinates()[references.Deceit]
		state.MapState.PlayerLocation.Position = entrance.Right()
		state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &entrance, indexes.Forest1)

		assert.False(t, state.ProcessWordOfPower("FALLAX"))
		assert.False(t, state.IsDungeonOpen(references.Deceit))
	})

	t.Run("earlier cardinal candidate wins", func(t *testing.T) {
		tests := []struct {
			name       string
			playerFrom func(references.Position) references.Position
			decoyFrom  func(references.Position) references.Position
		}{
			{name: "west before south", playerFrom: references.Position.Up, decoyFrom: references.Position.Left},
			{name: "south before east", playerFrom: references.Position.Left, decoyFrom: references.Position.Down},
			{name: "east before north", playerFrom: references.Position.Down, decoyFrom: references.Position.Right},
		}
		for _, test := range tests {
			t.Run(test.name, func(t *testing.T) {
				state := newState(t)
				entrance := state.GetDungeonEntranceCoordinates()[references.Deceit]
				player := test.playerFrom(entrance)
				state.MapState.PlayerLocation.Position = player
				openSprite := dungeonOpenEntranceSprite(references.Deceit)
				state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &entrance, openSprite)
				decoy := test.decoyFrom(player)
				state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &decoy, openSprite)

				assert.False(t, state.ProcessWordOfPower("FALLAX"))
				assert.False(t, state.IsDungeonOpen(references.Deceit))
			})
		}
	})

	t.Run("all cardinal projections", func(t *testing.T) {
		players := []references.Position{}
		state := newState(t)
		entrance := state.GetDungeonEntranceCoordinates()[references.Deceit]
		players = append(players, entrance.Right(), entrance.Up(), entrance.Left(), entrance.Down())
		for _, player := range players {
			state.SetDungeonState(references.Deceit, false)
			state.MapState.PlayerLocation.Position = player
			state.GetLayeredMapByCurrentLocation().SetTileByLayer(
				map_state.MapLayer,
				&entrance,
				dungeonOpenEntranceSprite(references.Deceit),
			)
			require.True(t, state.ProcessWordOfPower("FALLAX"), "player at %+v", player)
		}
	})
}

func TestWordForDungeon(t *testing.T) {
	tests := []struct {
		name     string
		dungeon  references.Location
		expected string
	}{
		{"Deceit dungeon", references.Deceit, "FALLAX"},
		{"Despise dungeon", references.Despise, "VILIS"},
		{"Destard dungeon", references.Destard, "INOPIA"},
		{"Wrong dungeon", references.Wrong, "MALUM"},
		{"Covetous dungeon", references.Covetous, "AVIDUS"},
		{"Shame dungeon", references.Shame, "INFAMA"},
		{"Hythloth dungeon", references.Hythloth, "IGNAVUS"},
		{"Doom dungeon", references.Doom, "VERAMOCOR"},
		{"Invalid dungeon", references.Britain, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			word := wordForDungeon(tt.dungeon)
			assert.Equal(t, tt.expected, word)
		})
	}
}

func TestGetDungeonEntranceCoordinatesMatchOriginalTownTables(t *testing.T) {
	state := &GameState{}
	assert.Equal(t, map[references.Location]references.Position{
		references.Deceit:   {X: 0xf0, Y: 0x49},
		references.Despise:  {X: 0x5b, Y: 0x43},
		references.Destard:  {X: 0x48, Y: 0xa8},
		references.Wrong:    {X: 0x7e, Y: 0x14},
		references.Covetous: {X: 0x9c, Y: 0x1b},
		references.Shame:    {X: 0x3a, Y: 0x66},
		references.Hythloth: {X: 0xef, Y: 0xf0},
		references.Doom:     {X: 0x80, Y: 0x80},
	}, state.GetDungeonEntranceCoordinates())
}

func TestGameState_IsDungeonOpen(t *testing.T) {
	gs := &GameState{}

	// Test default state (all closed)
	assert.False(t, gs.IsDungeonOpen(references.Deceit))
	assert.False(t, gs.IsDungeonOpen(references.Doom))
	assert.False(t, gs.IsDungeonOpen(references.Britain)) // Invalid dungeon

	// Open some dungeons
	gs.DungeonStates[DeceitIndex] = true
	gs.DungeonStates[DoomIndex] = true

	assert.True(t, gs.IsDungeonOpen(references.Deceit))
	assert.True(t, gs.IsDungeonOpen(references.Doom))
	assert.False(t, gs.IsDungeonOpen(references.Despise))
}

func TestGameState_ToggleDungeon(t *testing.T) {
	gs := &GameState{}

	// Initially all dungeons are closed (false)
	newState := gs.ToggleDungeon(references.Deceit)
	assert.True(t, newState)
	assert.True(t, gs.DungeonStates[DeceitIndex])

	// Toggle again
	newState = gs.ToggleDungeon(references.Deceit)
	assert.False(t, newState)
	assert.False(t, gs.DungeonStates[DeceitIndex])

	// Try invalid dungeon
	newState = gs.ToggleDungeon(references.Britain)
	assert.False(t, newState) // Should return false for invalid dungeon
}

func TestDungeonIndexFollowsDungeonStatesOrder(t *testing.T) {
	dungeons := []references.Location{
		references.Deceit, references.Despise, references.Destard, references.Wrong,
		references.Covetous, references.Shame, references.Hythloth, references.Doom,
	}

	for want, dungeon := range dungeons {
		index, exists := dungeonIndex(dungeon)
		assert.True(t, exists, "Dungeon %v should have an index", dungeon)
		assert.Equal(t, DungeonIndex(want), index)
	}
	_, exists := dungeonIndex(references.Britain)
	assert.False(t, exists)
}

func TestAbsInt(t *testing.T) {
	tests := []struct {
		input    int
		expected int
	}{
		{5, 5},
		{-5, 5},
		{0, 0},
		{1, 1},
		{-1, 1},
		{100, 100},
		{-100, 100},
	}

	for _, tt := range tests {
		t.Run("absInt function", func(t *testing.T) {
			result := absInt(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}
