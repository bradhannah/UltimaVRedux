package game_state

import (
	"fmt"
	"maps"
	"reflect"
	"slices"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newDaylightTintingTestState(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.SerpentsHold).
		WithFloor(0).
		WithPlayerAt(10, 25).
		WithSystemCallbacks().
		WithRandomSeed(526).
		Build()
	state.DateTime = datetime.UltimaDate{Year: 139, Month: 1, Day: 1, Hour: 12}
	state.initializeMoongateStage()
	state.MapState.ResetTransientDoor()
	return state, callbacks
}

func assertGameDaylightTarget(t *testing.T, state *GameState, pos references.Position, want float32, active bool) {
	t.Helper()
	got, ok := state.DaylightTintTarget(pos)
	require.Equal(t, active, ok, "target availability at %v", pos)
	assert.InDelta(t, want, got, 0.000001, "full-day target at %v", pos)
}

// Observe detached values, not aliases to cached source tiles or visibility maps.
// The full tint update may change its cosmetic lit-tile map, but not these inputs.
func checkDaylightLightingInputs(t *testing.T, state *GameState, layer *map_state.LayeredMap, callbacks *MockSystemCallbacks) func() {
	t.Helper()
	sources := slices.Clone(layer.GetLastComputedLightSources())
	for index := range sources {
		tile := *sources[index].Tile
		sources[index].Tile = &tile
	}
	visible := maps.Clone(layer.GetVisibleFlags())
	for coordinate, row := range visible {
		visible[coordinate] = maps.Clone(row)
	}
	primary := maps.Clone(layer.GetPrimaryDistanceMap())
	secondary := maps.Clone(layer.GetLightSourcesDistanceMap())
	lighting, date, elapsed := state.MapState.Lighting, state.DateTime, state.ElapsedMs
	seed, calls := state.GetRNG().Seed(), state.GetRNG().Calls()
	magicMinutes := state.MagicLightMinutesRemaining
	callbackCounts := []int{callbacks.MessageCallCount, callbacks.AudioCallCount, callbacks.VisualCallCount,
		callbacks.ScreenCallCount, callbacks.FlowCallCount, callbacks.TalkCallCount, callbacks.DisplayUpdates}
	topLeft, bottomRight := layer.TopLeft, layer.BottomRight
	type cell struct {
		sprites  [3]indexes.SpriteIndex
		source   indexes.SpriteIndex
		emission int
		exposure map_state.ExposureKind
		visible  bool
	}
	var source *references.SmallLocationReference
	if state.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType {
		source = state.GetCurrentSmallLocationReference()
	}
	observeCells := func() []cell {
		cells := make([]cell, 0, int(layer.XMaxTilesPerMap)*int(layer.YMaxTilesPerMap))
		for y := references.Coordinate(0); y < layer.YMaxTilesPerMap; y++ {
			for x := references.Coordinate(0); x < layer.XMaxTilesPerMap; x++ {
				pos := references.Position{X: x, Y: y}
				value := cell{exposure: layer.ExposureAt(pos),
					visible: layer.IsPositionVisible(&pos, state.DateTime, state.MapState.PlayerLocation.Floor < 0)}
				for kind := map_state.MapLayer; kind <= map_state.MapUnitLayer; kind++ {
					if tile := layer.GetTileByLayer(kind, &pos); tile != nil {
						value.sprites[kind] = tile.Index
					}
				}
				if source != nil {
					value.source = source.GetTileNumber(pos, state.MapState.PlayerLocation.Floor)
				}
				if tile := layer.GetTileTopMapOnlyTile(&pos); tile != nil {
					value.emission = tile.GetLightEmission()
				}
				cells = append(cells, value)
			}
		}
		return cells
	}
	cells := observeCells()
	return func() {
		t.Helper()
		assert.Equal(t, sources, layer.GetLastComputedLightSources(), "daylight must not become an emission source")
		assert.Equal(t, visible, layer.GetVisibleFlags())
		assert.Equal(t, primary, layer.GetPrimaryDistanceMap())
		assert.Equal(t, secondary, layer.GetLightSourcesDistanceMap())
		assert.Equal(t, cells, observeCells(), "terrain, emission, exposure and LOS stay authoritative")
		assert.Equal(t, topLeft, layer.TopLeft)
		assert.Equal(t, bottomRight, layer.BottomRight)
		// Lighting's dimensions binding points back to the entire GameState.
		// Compare identity/scalars without letting failure output dump that graph.
		require.True(t, lighting == state.MapState.Lighting, "lighting settings or dimensions binding changed")
		assert.Equal(t, magicMinutes, state.MagicLightMinutesRemaining)
		assert.Equal(t, date, state.DateTime)
		assert.Equal(t, elapsed, state.ElapsedMs)
		assert.Equal(t, seed, state.GetRNG().Seed())
		assert.Equal(t, calls, state.GetRNG().Calls())
		assert.Equal(t, callbackCounts, []int{callbacks.MessageCallCount, callbacks.AudioCallCount, callbacks.VisualCallCount,
			callbacks.ScreenCallCount, callbacks.FlowCallCount, callbacks.TalkCallCount, callbacks.DisplayUpdates})
	}
}

// Original data establishes opening identity and exposure; brightness is Redux cosmetic policy.
func TestDaylightTintingOriginalWindowsAndAuthority(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	indoor, outdoor := defaults.IndoorBrightness, defaults.OutdoorBrightness
	near, far := indoor+(outdoor-indoor)*defaults.OpeningFalloff[1], indoor+(outdoor-indoor)*defaults.OpeningFalloff[2]
	state, callbacks := newDaylightTintingTestState(t)
	state.SetPlayerPosition(references.Position{X: 10, Y: 24})
	layer := state.GetLayeredMapByCurrentLocation()
	require.NotNil(t, layer)
	source := state.GetCurrentSmallLocationReference()
	window := references.Position{X: 12, Y: 25}
	windows := []references.Position{{X: 12, Y: 23}, window, {X: 12, Y: 27}}
	for _, pos := range windows {
		require.Equal(t, indexes.SpriteIndex(indexes.StoneCrossWindow), source.GetTileNumber(pos, 0))
		require.Equal(t, map_state.ExposureBoundary, layer.ExposureAt(pos))
		require.Equal(t, map_state.ExposureOutdoor, layer.ExposureAt(references.Position{X: 13, Y: pos.Y}))
		require.Equal(t, map_state.ExposureIndoor, layer.ExposureAt(references.Position{X: 11, Y: pos.Y}))
	}
	require.Equal(t, map_state.ExposureIndoor, layer.ExposureAt(state.MapState.PlayerLocation.Position))
	for _, pos := range []references.Position{{X: 12, Y: 24}, {X: 12, Y: 26}} {
		require.Equal(t, indexes.SpriteIndex(indexes.StoneBrickWall), source.GetTileNumber(pos, 0))
		require.Equal(t, map_state.ExposureBoundary, layer.ExposureAt(pos))
	}
	for _, pos := range []references.Position{{X: 11, Y: 22}, {X: 10, Y: 22}} {
		require.Equal(t, indexes.SpriteIndex(indexes.StoneBrickWall), source.GetTileNumber(pos, 0))
		require.Equal(t, map_state.ExposureBoundary, layer.ExposureAt(pos), "off-axis receiver at %v", pos)
	}
	for x := references.Coordinate(9); x <= 11; x++ {
		require.Equal(t, map_state.ExposureIndoor, layer.ExposureAt(references.Position{X: x, Y: 25}))
	}
	state.MapState.Lighting.LightTorch()
	state.ActivateLightEffect(100)
	// Source collection retains contributing emitters, not hidden all-map stock.
	// Install one in this room so the daylight-only purity check remains nontrivial.
	emitter := references.Position{X: 10, Y: 25}
	layer.SetTileByLayer(map_state.MapOverrideLayer, &emitter, indexes.Brazier)
	layer.RecalculateVisibleTilesWithCustomLights(state.MapState.PlayerLocation.Position, &state.MapState.Lighting,
		nil, state.GetVisibleQuestItemLights(), state.GetVisibleNPCLights(), state.GetPlayerLightType())
	require.NotEmpty(t, layer.GetLastComputedLightSources(), "exercise existing emitters, not an empty source list")
	require.NotEmpty(t, layer.GetPrimaryDistanceMap())
	require.NotEmpty(t, layer.GetLightSourcesDistanceMap())
	unchanged := checkDaylightLightingInputs(t, state, layer, callbacks)
	state.ClearLightSourceTinting()
	unchanged()
	assertGameDaylightTarget(t, state, state.MapState.PlayerLocation.Position, 0, false)

	before, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	state.updateDaylightTinting(layer)
	after, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.True(t, reflect.DeepEqual(before, after), "daylight-only preparation changed the save snapshot")
	unchanged()
	field := state.daylightTinting
	for repeat := 0; repeat < 3; repeat++ {
		state.UpdateLightSourceTinting(layer)
		assert.Same(t, field, state.daylightTinting)
		assert.Same(t, layer, state.daylightLayer)
		for _, pos := range windows {
			assertGameDaylightTarget(t, state, pos, outdoor, true)
		}
		for _, pos := range []references.Position{{X: 12, Y: 24}, {X: 12, Y: 26}} {
			assertGameDaylightTarget(t, state, pos, outdoor, true)
		}
		// Off-axis receivers use ceil-Euclidean bands: sqrt(2) and sqrt(5).
		assertGameDaylightTarget(t, state, references.Position{X: 11, Y: 22}, near, true)
		assertGameDaylightTarget(t, state, references.Position{X: 10, Y: 22}, far, true)
		assertGameDaylightTarget(t, state, references.Position{X: 13, Y: 25}, outdoor, true)
		for index, weight := range defaults.OpeningFalloff {
			target := indoor + (outdoor-indoor)*weight
			assertGameDaylightTarget(t, state, references.Position{X: 11 - references.Coordinate(index), Y: 25}, target, true)
		}
		unchanged()
	}
	for _, pos := range []references.Position{{X: -1, Y: 25}, {X: 32, Y: 25}, {X: 10, Y: -1}, {X: 10, Y: 32}} {
		assertGameDaylightTarget(t, state, pos, 0, false)
	}
	require.Positive(t, state.LightSourceTintingState.GetLitTileCount())
	state.ClearLightSourceTinting()
	assert.Zero(t, state.LightSourceTintingState.GetLitTileCount())
	assert.Nil(t, state.daylightLayer)
	assertGameDaylightTarget(t, state, references.Position{X: 10, Y: 25}, 0, false)
	unchanged()
	state.UpdateLightSourceTinting(layer)
	assertGameDaylightTarget(t, state, references.Position{X: 10, Y: 25}, near, true)
	unchanged()

	// This original door separates two indoor regions; opening it must not relay daylight.
	door := references.Position{X: 5, Y: 25}
	require.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(door, 0))
	for _, pos := range []references.Position{{X: 4, Y: 25}, {X: 6, Y: 25}} {
		require.Equal(t, map_state.ExposureIndoor, layer.ExposureAt(pos))
		assertGameDaylightTarget(t, state, pos, indoor, true)
	}
	state.SetPlayerPosition(references.Position{X: 6, Y: 25})
	require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Left))
	state.UpdateLightSourceTinting(layer)
	for x := references.Coordinate(4); x <= 6; x++ {
		assertGameDaylightTarget(t, state, references.Position{X: x, Y: 25}, indoor, true)
	}
	assert.Equal(t, indexes.SpriteIndex(indexes.StoneCrossWindow), source.GetTileNumber(window, 0))
	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(door, 0))
}

func TestDaylightTintingOriginalRuntimeOverridesDoNotRedefineOpenings(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	indoor, outdoor := defaults.IndoorBrightness, defaults.OutdoorBrightness
	state, callbacks := newDaylightTintingTestState(t)
	layer := state.GetLayeredMapByCurrentLocation()
	window, room := references.Position{X: 12, Y: 25}, references.Position{X: 10, Y: 25}
	door, inside := references.Position{X: 11, Y: 14}, references.Position{X: 10, Y: 14}
	source := state.GetCurrentSmallLocationReference()
	require.Equal(t, indexes.SpriteIndex(indexes.StoneCrossWindow), source.GetTileNumber(window, 0))
	require.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(door, 0))
	layer.RecalculateVisibleTilesWithMoongates(room, &state.MapState.Lighting, nil, state.GetPlayerLightType())
	// Runtime terrain/occupants cannot erase an original window or turn a closed door into one.
	layer.SetTileByLayer(map_state.MapLayer, &window, indexes.RegularDoor)
	layer.SetTileByLayer(map_state.MapOverrideLayer, &window, indexes.StoneBrickWall)
	layer.SetTileByLayer(map_state.MapUnitLayer, &room, indexes.StoneBrickWall)
	layer.SetTileByLayer(map_state.MapOverrideLayer, &door, indexes.StoneCrossWindow)
	unchanged := checkDaylightLightingInputs(t, state, layer, callbacks)
	state.UpdateLightSourceTinting(layer)
	assertGameDaylightTarget(t, state, room, indoor+(outdoor-indoor)*defaults.OpeningFalloff[1], true)
	assertGameDaylightTarget(t, state, window, outdoor, true)
	assertGameDaylightTarget(t, state, inside, indoor, true)
	assertGameDaylightTarget(t, state, door, indoor, true)
	assert.False(t, state.MapState.IsPositionOpenDoor(&door))
	unchanged()
	// A floor override is not sufficient either: only MapState's live door state activates it.
	layer.SetTileByLayer(map_state.MapOverrideLayer, &door, indexes.BrickFloor)
	unchanged = checkDaylightLightingInputs(t, state, layer, callbacks)
	state.UpdateLightSourceTinting(layer)
	assertGameDaylightTarget(t, state, inside, indoor, true)
	assertGameDaylightTarget(t, state, door, indoor, true)
	unchanged()
}

func TestDaylightTintingOriginalExteriorDoorLifecycle(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	state, callbacks := newDaylightTintingTestState(t)
	layer := state.GetLayeredMapByCurrentLocation()
	inside, door, outside := references.Position{X: 10, Y: 14}, references.Position{X: 11, Y: 14}, references.Position{X: 12, Y: 14}
	source := state.GetCurrentSmallLocationReference()
	require.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(door, 0))
	require.Equal(t, map_state.ExposureBoundary, layer.ExposureAt(door))
	require.Equal(t, map_state.ExposureIndoor, layer.ExposureAt(inside))
	require.Equal(t, map_state.ExposureOutdoor, layer.ExposureAt(outside))
	state.SetPlayerPosition(inside)
	layer.RecalculateVisibleTilesWithMoongates(inside, &state.MapState.Lighting, nil, state.GetPlayerLightType())
	state.UpdateLightSourceTinting(layer)
	assertGameDaylightTarget(t, state, inside, defaults.IndoorBrightness, true)
	assertGameDaylightTarget(t, state, door, defaults.IndoorBrightness, true)
	assertGameDaylightTarget(t, state, outside, defaults.OutdoorBrightness, true)
	require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Right))
	require.True(t, state.MapState.IsPositionOpenDoor(&door))
	require.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), layer.GetTileTopMapOnlyTile(&door).Index)

	// Opening/expiry own terrain mutations. Observe only after each explicit door action.
	for turn := 0; turn <= 3; turn++ {
		t.Run(fmt.Sprintf("door turn %d", turn), func(t *testing.T) {
			open := turn < 3
			if turn > 0 {
				assert.Equal(t, !open, state.MapState.SmallMapProcessTurnDoors(state.isSmallMapDoorOccupied))
			}
			unchanged := checkDaylightLightingInputs(t, state, layer, callbacks)
			for repeat := 0; repeat < 3; repeat++ {
				state.UpdateLightSourceTinting(layer)
				assert.Equal(t, open, state.MapState.IsPositionOpenDoor(&door), "tint preparation cannot advance the door timer")
				target := defaults.IndoorBrightness
				if open {
					target = defaults.OutdoorBrightness
				}
				assertGameDaylightTarget(t, state, inside, target, true)
				assertGameDaylightTarget(t, state, door, target, true)
				assertGameDaylightTarget(t, state, outside, defaults.OutdoorBrightness, true)
				unchanged()
			}
		})
	}
	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), layer.GetTileTopMapOnlyTile(&door).Index)
	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(door, 0))
}

func TestDaylightTintingOriginalDebugRebuildClearsOpenDoor(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	for _, test := range []struct {
		name   string
		floors []references.FloorNumber
	}{
		{name: "same location and floor world position", floors: []references.FloorNumber{0}},
		{name: "floor round trip", floors: []references.FloorNumber{1, 0}},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := newDaylightTintingTestState(t)
			inside, door, outside := references.Position{X: 10, Y: 14}, references.Position{X: 11, Y: 14}, references.Position{X: 12, Y: 14}
			ground := state.GetLayeredMapByCurrentLocation()
			source := state.GetCurrentSmallLocationReference()
			require.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(door, 0))
			require.Equal(t, map_state.ExposureBoundary, ground.ExposureAt(door))
			require.Equal(t, map_state.ExposureIndoor, ground.ExposureAt(inside))
			require.Equal(t, map_state.ExposureOutdoor, ground.ExposureAt(outside))
			state.SetPlayerPosition(inside)
			state.UpdateLightSourceTinting(ground)
			assertGameDaylightTarget(t, state, inside, defaults.IndoorBrightness, true)
			require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Right))
			require.True(t, state.MapState.IsPositionOpenDoor(&door))
			require.True(t, state.MapState.HasTransientDoor())
			require.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), ground.GetTileTopMapOnlyTile(&door).Index)
			state.UpdateLightSourceTinting(ground)
			assertGameDaylightTarget(t, state, inside, defaults.OutdoorBrightness, true)
			assertGameDaylightTarget(t, state, door, defaults.OutdoorBrightness, true)
			date, elapsed := state.DateTime, state.ElapsedMs
			rng, seed, calls := state.GetRNG(), state.GetRNG().Seed(), state.GetRNG().Calls()

			// Do not process a turn: source rebuilding, not timer expiry, must close the door.
			for _, floor := range test.floors {
				before := state.MapState.PlayerLocation
				var result WorldSetupResult
				var err error
				position := inside
				if len(test.floors) == 1 {
					position = references.Position{X: 9, Y: 14}
				}
				result, err = state.DebugSetWorldPosition(references.SerpentsHold, floor, position)
				require.NoError(t, err)
				assert.Equal(t, before, result.Before)
				assert.Equal(t, references.PlayerLocation{Location: references.SerpentsHold, Floor: floor, Position: position}, result.After)
				assert.Equal(t, result.After, state.MapState.PlayerLocation)
				assert.Equal(t, date, state.DateTime)
				assert.Equal(t, elapsed, state.ElapsedMs)
				assert.Same(t, rng, state.GetRNG())
				assert.Equal(t, seed, state.GetRNG().Seed())
				assert.Equal(t, calls, state.GetRNG().Calls())

				rebuiltGround := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 0)
				require.NotNil(t, rebuiltGround)
				require.NotSame(t, ground, rebuiltGround)
				assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), rebuiltGround.GetTileTopMapOnlyTile(&door).Index)
				assert.False(t, state.MapState.IsPositionOpenDoor(&door), "clear the stale position on the first rebuild, even upstairs")
				assert.False(t, state.MapState.HasTransientDoor(), "clear the timer together with its position")
				assertGameDaylightTarget(t, state, inside, 0, false)
				layer := state.GetLayeredMapByCurrentLocation()
				require.NotNil(t, layer)
				unchanged := checkDaylightLightingInputs(t, state, layer, callbacks)
				state.UpdateLightSourceTinting(layer)
				if floor == 0 {
					assertGameDaylightTarget(t, state, inside, defaults.IndoorBrightness, true)
					assertGameDaylightTarget(t, state, door, defaults.IndoorBrightness, true)
					assertGameDaylightTarget(t, state, outside, defaults.OutdoorBrightness, true)
				}
				unchanged()
				ground = rebuiltGround
			}
		})
	}
}

func TestDaylightTintingOriginalClockGatesFullDayTargets(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	near := defaults.IndoorBrightness + (defaults.OutdoorBrightness-defaults.IndoorBrightness)*defaults.OpeningFalloff[1]
	state, _ := newDaylightTintingTestState(t)
	layer := state.GetLayeredMapByCurrentLocation()
	room := references.Position{X: 10, Y: 25}
	for _, test := range []struct {
		hour, minute byte
		active       bool
	}{
		{12, 0, true}, {19, 30, true}, {19, 45, true}, {19, 59, true},
		{20, 0, false}, {0, 0, false}, {4, 59, false}, {5, 0, false},
		{5, 1, true}, {5, 15, true}, {5, 30, true}, {6, 0, true},
	} {
		t.Run(fmt.Sprintf("%02d:%02d", test.hour, test.minute), func(t *testing.T) {
			state.DateTime.Hour, state.DateTime.Minute = 12, 0
			state.updateDaylightTinting(layer)
			assertGameDaylightTarget(t, state, room, near, true)
			state.DateTime.Hour, state.DateTime.Minute = test.hour, test.minute
			state.initializeMoongateStage()
			target := float32(0)
			if test.active {
				target = near
			}
			// The public getter must reject newly ineligible clocks before a render update.
			assertGameDaylightTarget(t, state, room, target, test.active)
			before, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			state.updateDaylightTinting(layer)
			after, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			require.True(t, reflect.DeepEqual(before, after), "daylight preparation changed the save snapshot")
			assertGameDaylightTarget(t, state, room, target, test.active)
			if !test.active {
				assert.Nil(t, state.daylightLayer)
				_, active := state.daylightTinting.TargetAt(room)
				assert.False(t, active, "night must clear stored targets as well as gate access")
			}
		})
	}
}

func TestDaylightTintingOriginalLayerLifecycle(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	near := defaults.IndoorBrightness + (defaults.OutdoorBrightness-defaults.IndoorBrightness)*defaults.OpeningFalloff[1]
	state, _ := newDaylightTintingTestState(t)
	room := references.Position{X: 10, Y: 25}
	ground := state.GetLayeredMapByCurrentLocation()
	state.UpdateLightSourceTinting(ground)
	field := state.daylightTinting
	assertGameDaylightTarget(t, state, room, near, true)

	state.MapState.PlayerLocation.Floor = 1
	upper := state.GetLayeredMapByCurrentLocation()
	require.NotNil(t, upper)
	require.NotSame(t, ground, upper)
	assertGameDaylightTarget(t, state, room, 0, false)
	state.UpdateLightSourceTinting(upper)
	_, active := state.DaylightTintTarget(room)
	assert.True(t, active, "nonnegative small-map floors use their own exposure")
	assert.Same(t, upper, state.daylightLayer)
	assert.Same(t, field, state.daylightTinting)
	state.MapState.PlayerLocation.Floor = 0
	assertGameDaylightTarget(t, state, room, 0, false)
	state.UpdateLightSourceTinting(ground)
	assertGameDaylightTarget(t, state, room, near, true)

	state.reloadCurrentSmallMapTerrain()
	replacement := state.GetLayeredMapByCurrentLocation()
	require.NotSame(t, ground, replacement)
	assert.Same(t, upper, state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 1))
	assertGameDaylightTarget(t, state, room, 0, false)
	state.UpdateLightSourceTinting(ground)
	assertGameDaylightTarget(t, state, room, 0, false)
	state.UpdateLightSourceTinting(replacement)
	assertGameDaylightTarget(t, state, room, near, true)
	assert.Same(t, field, state.daylightTinting)

	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	assertGameDaylightTarget(t, state, room, 0, false)
	state.UpdateLightSourceTinting(state.GetLayeredMapByCurrentLocation())
	for _, pos := range []references.Position{{X: 0, Y: 0}, {X: 255, Y: 255}, {X: -1, Y: -1}, {X: 256, Y: 256}} {
		assertGameDaylightTarget(t, state, pos, defaults.OutdoorBrightness, true)
	}
	state.MapState.PlayerLocation.Location = references.SerpentsHold
	assertGameDaylightTarget(t, state, room, 0, false)
	state.UpdateLightSourceTinting(replacement)
	assertGameDaylightTarget(t, state, room, near, true)
	state.ClearLightSourceTinting()
	assert.Nil(t, state.daylightLayer)
	assertGameDaylightTarget(t, state, room, 0, false)
	_, active = field.TargetAt(room)
	assert.False(t, active)
	state.UpdateLightSourceTinting(replacement)
	assert.Same(t, field, state.daylightTinting)
	assertGameDaylightTarget(t, state, room, near, true)
}

func TestDaylightTintingOriginalBelowgroundPreservesLighting(t *testing.T) {
	for _, location := range []references.Location{references.LordBritishsCastle, references.BritanniaUnderworld} {
		for _, hour := range []byte{12, 0} {
			t.Run(fmt.Sprintf("%s/%02d", location, hour), func(t *testing.T) {
				state, callbacks := NewIntegrationTestBuilder(t).WithLocation(location).WithFloor(-1).
					WithPlayerAt(10, 14).WithSystemCallbacks().WithRandomSeed(526).Build()
				state.DateTime.Hour, state.DateTime.Minute = hour, 0
				state.initializeMoongateStage()
				state.MapState.Lighting.LightTorch()
				layer := state.GetLayeredMapByCurrentLocation()
				require.NotNil(t, layer)
				layer.RecalculateVisibleTilesWithMoongates(state.MapState.PlayerLocation.Position,
					&state.MapState.Lighting, nil, state.GetPlayerLightType())
				unchanged := checkDaylightLightingInputs(t, state, layer, callbacks)
				legacy := map_state.NewLightSourceTintingState()
				legacy.BuildLitTilesMap(layer.GetLastComputedLightSources(), layer.GetVisibleFlags())
				legacy.SetLargeMapMode(location == references.BritanniaUnderworld)
				legacy.UpdatePlayerLight(state.MapState.PlayerLocation.Position, map_state.PlayerLightTorch)
				// Seed a previous surface field so the test also catches failure to clear it.
				state.daylightTinting = map_state.NewDaylightField(map_state.DefaultDaylightOptions())
				state.daylightTinting.PrepareOutdoor()
				state.daylightLayer = layer
				assertGameDaylightTarget(t, state, state.MapState.PlayerLocation.Position, 0, false)
				state.UpdateLightSourceTinting(layer)
				assertGameDaylightTarget(t, state, state.MapState.PlayerLocation.Position, 0, false)
				assert.Nil(t, state.daylightLayer)
				_, active := state.daylightTinting.TargetAt(state.MapState.PlayerLocation.Position)
				assert.False(t, active)
				require.True(t, reflect.DeepEqual(legacy, state.LightSourceTintingState),
					"belowground source or carried-light tint changed")
				unchanged()
			})
		}
	}
}

func TestDaylightTintingSyntheticModeGates(t *testing.T) {
	defaults := map_state.DefaultDaylightOptions()
	// No installation, source files, or nil-layer stand-ins: public gates must run in data-safe CI too.
	layers := map_state.NewLayeredMaps(nil, &references.LargeMapReference{}, &references.LargeMapReference{}, 19, 13)
	state := &GameState{DateTime: datetime.UltimaDate{Year: 139, Month: 1, Day: 1, Hour: 12}}
	state.MapState.LayeredMaps = *layers
	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	layer := state.GetLayeredMapByCurrentLocation()
	pos := references.Position{X: 10, Y: 25}
	assertGameDaylightTarget(t, state, pos, 0, false)
	state.ClearLightSourceTinting()
	for _, clock := range []struct{ hour, minute byte }{{12, 0}, {5, 1}, {5, 15}, {5, 30}, {19, 45}, {19, 59}} {
		t.Run(fmt.Sprintf("surface %02d:%02d", clock.hour, clock.minute), func(t *testing.T) {
			state.DateTime.Hour, state.DateTime.Minute = clock.hour, clock.minute
			state.UpdateLightSourceTinting(layer)
			for _, position := range []references.Position{pos, {X: 0, Y: 0}, {X: 255, Y: 255}, {X: -1, Y: -1}, {X: 256, Y: 256}} {
				assertGameDaylightTarget(t, state, position, defaults.OutdoorBrightness, true)
			}
			state.ClearLightSourceTinting()
			assertGameDaylightTarget(t, state, pos, 0, false)
			assert.Nil(t, state.daylightLayer)
		})
	}
	for _, test := range []struct {
		name   string
		mutate func()
	}{
		{"small basement", func() {
			state.MapState.PlayerLocation.Location, state.MapState.PlayerLocation.Floor = references.SerpentsHold, -1
		}},
		{"underworld", func() { state.MapState.PlayerLocation.Floor = -1 }},
		{"large above surface", func() { state.MapState.PlayerLocation.Floor = 1 }},
		{"dungeon", func() { state.MapState.PlayerLocation.Location = references.Deceit }},
		{"combat", func() { state.MapState.PlayerLocation.Location = references.CombatLocation }},
		{"camping", func() { state.MapState.PlayerLocation.Location = references.CampingLocation }},
		{"shrine", func() { state.MapState.PlayerLocation.Location = references.ShrineLocation }},
		{"codex", func() { state.MapState.PlayerLocation.Location = references.CodexLocation }},
		{"prison", func() { state.MapState.PlayerLocation.Location = references.BlackthornPrison }},
		{"finale map", func() { state.MapState.PlayerLocation.Location = references.FinaleLocation }},
		{"pending finale", func() { state.finaleHandoffPending = true }},
		{"active finale", func() { state.lordBritishFinale = &LordBritishFinaleState{outcome: LordBritishFinaleInProgress} }},
		{"successful finale", func() { state.lordBritishFinale = &LordBritishFinaleState{outcome: LordBritishFinaleSucceeded} }},
		{"failed finale", func() { state.lordBritishFinale = &LordBritishFinaleState{outcome: LordBritishFinaleFailed} }},
		{"night", func() { state.DateTime.Hour = 0 }},
		{"dawn boundary", func() { state.DateTime.Hour = 5 }},
		{"dusk boundary", func() { state.DateTime.Hour = 20 }},
	} {
		t.Run(test.name, func(t *testing.T) {
			state.MapState.PlayerLocation.Location, state.MapState.PlayerLocation.Floor = references.BritanniaUnderworld, 0
			state.DateTime.Hour, state.DateTime.Minute = 12, 0
			state.finaleHandoffPending, state.lordBritishFinale = false, nil
			state.updateDaylightTinting(layer)
			assertGameDaylightTarget(t, state, pos, defaults.OutdoorBrightness, true)
			test.mutate()
			assert.False(t, state.usesDaylightTinting())
			assertGameDaylightTarget(t, state, pos, 0, false)
			state.updateDaylightTinting(layer)
			assertGameDaylightTarget(t, state, pos, 0, false)
			assert.Nil(t, state.daylightLayer)
			_, active := state.daylightTinting.TargetAt(pos)
			assert.False(t, active)
		})
	}
}
