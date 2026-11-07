package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestOutdoorWindowSaveContinueAndHydrationNextBlink(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		state := newBlinkHistoryFixture(t, floor)
		window := state.outdoorWindow
		snapshot, err := state.CaptureSaveSnapshot()
		require.NoError(t, err)
		assert.Equal(t, window, state.outdoorWindow, "capture cannot reset live history")
		loaded, err := NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences, 19, 13)
		require.NoError(t, err)
		assert.Equal(t, references.Position{X: 112, Y: 112}, loaded.outdoorWindow.origin)
		assert.Equal(t, floor, loaded.outdoorWindow.floor)
		require.Equal(t, BlinkSuccess, loaded.BlinkTeleport(references.Right))
		assert.Equal(t, references.Position{X: 140, Y: 120}, loaded.MapState.PlayerLocation.Position)
		require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Right))
		assert.Equal(t, references.Position{X: 124, Y: 120}, state.MapState.PlayerLocation.Position)
	}
}

func TestOutdoorWindowRejectedHydrationAndDebugRequestPreserveLiveHistory(t *testing.T) {
	state := newBlinkHistoryFixture(t, -1)
	window, before := state.outdoorWindow, state.MapState.PlayerLocation
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	snapshot.World.Position.X = 256
	candidate, err := NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences, 19, 13)
	require.Error(t, err)
	assert.Nil(t, candidate)
	_, err = state.DebugSetWorldPosition(references.BritanniaUnderworld, -2, before.Position)
	require.Error(t, err)
	assert.Equal(t, window, state.outdoorWindow)
	assert.Equal(t, before, state.MapState.PlayerLocation)
	require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Left))
	assert.Equal(t, references.Position{X: 100, Y: 120}, state.MapState.PlayerLocation.Position)
}

func TestOutdoorWindowWorldRelocationsInitializeEvenAtSamePosition(t *testing.T) {
	for _, route := range []string{"world setup", "Gate", "cross-plane Gate", "town return", "floor transition"} {
		t.Run(route, func(t *testing.T) {
			floor := references.FloorNumber(-1)
			if route == "cross-plane Gate" {
				floor = 0
			}
			state := newBlinkHistoryFixture(t, floor)
			position := state.MapState.PlayerLocation.Position
			underworld := state.MapState.LayeredMaps.GetLayeredMap(references.LargeMapType, -1)
			setBlinkRayTiles(t, underworld, position, references.Right, indexes.Forest1)
			underworld.SetTileByLayer(map_state.MapOverrideLayer, &references.Position{X: 140, Y: 120}, indexes.Grass)
			switch route {
			case "world setup":
				_, err := state.DebugSetWorldPosition(references.BritanniaUnderworld, -1, position)
				require.NoError(t, err)
			case "Gate", "cross-plane Gate":
				state.EventQueueManager.Workflow("blink_window_gate_test").
					Priority(event_queue.PriorityHigh).Sequential().First(func() {}).
					ThenSequence(state.BuildMoongateTeleportationSequence(MoongateDestination{
						Location: references.BritanniaUnderworld, Level: 255, X: 120, Y: 120,
					})).Finally(func() {}).Start()
				drainGameplayWorkflows(t, state)
			case "town return":
				state.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 15}, TransitionFullLocation, true)
				state.TransitionToLocation(references.BritanniaUnderworld, state.LastLargeMapFloor, state.LastLargeMapPosition, TransitionFullLocation, false)
			case "floor transition":
				state.TransitionToLocation(references.BritanniaUnderworld, 0, position, TransitionFloorOnly, false)
				state.TransitionToLocation(references.BritanniaUnderworld, -1, position, TransitionFloorOnly, false)
			}
			assert.Equal(t, references.Position{X: 112, Y: 112}, state.outdoorWindow.origin)
			assert.Equal(t, references.FloorNumber(-1), state.outdoorWindow.floor)
			require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Right))
			assert.Equal(t, references.Position{X: 140, Y: 120}, state.MapState.PlayerLocation.Position)
		})
	}
}

func TestOutdoorWindowTemporarySceneReturnsPreserveNextBlink(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		for _, route := range []string{"combat", "camping", "shrine", "Codex"} {
			t.Run(route, func(t *testing.T) {
				state := newBlinkHistoryFixture(t, floor)
				window, before := state.outdoorWindow, state.MapState.PlayerLocation
				switch route {
				case "combat":
					require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntrySouth))
					assert.Equal(t, window, state.outdoorWindow)
					state.EndCombat(CombatVictory)
				case "camping":
					scene := camping.NewCampingSceneWithCombatRef(2, NoGuardAssigned, before, testCampingMap(t))
					state.CampingScene = scene
					require.NoError(t, state.TransitionToCampingMap())
					state.finalizeCampingSequence(scene, &CampingState{AmbushedAt: NoAmbush, ActivePlayerIndex: -1}, nil)
				case "shrine":
					require.NoError(t, state.TransitionToShrineMap(0))
					state.TransitionFromShrineMap()
				case "Codex":
					require.NoError(t, state.TransitionToCodexMap())
					state.TransitionFromShrineMap()
				}
				assert.Equal(t, before, state.MapState.PlayerLocation)
				assert.Equal(t, window, state.outdoorWindow)
				require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Left))
				assert.Equal(t, references.Position{X: 100, Y: 120}, state.MapState.PlayerLocation.Position)
			})
		}
	}
}

func TestOutdoorWindowKlimbScrollsAndWhirlpoolInitializes(t *testing.T) {
	state := newBlinkHistoryFixture(t, 0)
	state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
	state.SetPlayerPosition(references.Position{X: 122, Y: 120})
	target := references.Position{X: 123, Y: 120}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, indexes.SmallMountains)
	require.True(t, state.ActionKlimbLargeMap(references.Right))
	assert.Equal(t, target, state.MapState.PlayerLocation.Position)
	assert.Equal(t, references.Position{X: 112, Y: 112}, state.outdoorWindow.origin)
	drainGameplayWorkflows(t, state)

	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, target, 0)
	state.PartyVehicle.GetVehicleDetails().SetHullHP(99)
	result := state.ResolveWhirlpoolHit()
	require.True(t, result.Teleported)
	assert.Equal(t, references.FloorNumber(-1), state.outdoorWindow.floor)
	assert.Equal(t, references.Position{X: 16, Y: 0}, state.outdoorWindow.origin)
	setBlinkRayTiles(t, state.GetCurrentLayeredMap(), state.MapState.PlayerLocation.Position, references.Right, indexes.Forest1)
	grass := references.Position{X: 47, Y: 18}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &grass, indexes.Grass)
	require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Right))
	assert.Equal(t, grass, state.MapState.PlayerLocation.Position)
}

func TestOutdoorWindowHythlothBottomExitInitializesUnderworld(t *testing.T) {
	const floor references.FloorNumber = 7
	state, callbacks := NewIntegrationTestBuilder(t).WithPlayerAt(119, 120).WithRandomSeed(35).Build()
	prepared, err := state.PrepareDungeonState(references.Hythloth, references.DungeonFacingWest, references.DungeonLastWest)
	require.NoError(t, err)
	// Choose the route from loaded semantic cells before installing a visit, so
	// an arbitrary live-room starting square cannot create a pending encounter.
	preparedSnapshot := prepared.Snapshot()
	position, authored := outdoorWindowFindDownLadder(preparedSnapshot.Cells, floor)
	if authored {
		t.Logf("using loaded Hythloth floor %d down ladder at (%d,%d)", floor, position.X, position.Y)
	} else {
		// This branch verifies the real exit lifecycle with synthetic terrain;
		// it does not establish an authored Hythloth ladder or a traversable route.
		position = references.Position{X: 1, Y: 1}
		require.True(t, preparedSnapshot.Cells.SetCell(floor, position, references.DungeonCell(0x20)))
		prepared, err = newDungeonState(references.Hythloth, preparedSnapshot.Cells,
			references.DungeonFacingWest, references.DungeonLastWest)
		require.NoError(t, err)
		t.Log("no loaded Hythloth floor-seven down ladder; using an explicit synthetic exit-ladder fixture")
	}
	state.installPreparedDungeonState(prepared)
	state.TransitionToLocation(references.Hythloth, floor, position, TransitionFullLocation, true)
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	cell, found := snapshot.Cells.Cell(floor, position)
	require.True(t, found)
	require.Contains(t, []references.DungeonCellKind{references.DungeonLadderDown, references.DungeonLadderBoth}, cell.Kind(),
		"the installed exit fixture must provide a down ladder")
	require.True(t, state.DungeonKlimbOptionsAtCurrentCell().Down)
	require.True(t, state.IsAtDungeonCommandBoundary())

	token, _ := state.BeginDungeonCommandCycle()
	state.ActionKlimbDungeonMap(token, DungeonKlimbDown)
	require.True(t, callbacks.InputBlocked)
	runDungeonWorkflow(t, state, "dungeon_klimb")

	assert.False(t, callbacks.InputBlocked)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	_, active = state.DungeonStateSnapshot()
	assert.False(t, active)
	assert.Contains(t, callbacks.Messages, "Exit to Underworld!")
	assert.Equal(t, references.BritanniaUnderworld, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.Position{X: 239, Y: 240}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, references.FloorNumber(-1), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, references.Position{X: 224, Y: 224}, state.outdoorWindow.origin)
	assert.Equal(t, references.FloorNumber(-1), state.outdoorWindow.floor)
	setBlinkRayTiles(t, state.GetCurrentLayeredMap(), state.MapState.PlayerLocation.Position, references.Right, indexes.Forest1)
	endpoint := references.Position{X: 255, Y: 240}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &endpoint, indexes.Grass)
	require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Right))
	assert.Equal(t, endpoint, state.MapState.PlayerLocation.Position)
}

func outdoorWindowFindDownLadder(cells references.DungeonCells, floor references.FloorNumber) (references.Position, bool) {
	for y := references.Coordinate(0); y < references.YDungeonMapTiles; y++ {
		for x := references.Coordinate(0); x < references.XDungeonMapTiles; x++ {
			position := references.Position{X: x, Y: y}
			cell, valid := cells.Cell(floor, position)
			if valid && (cell.Kind() == references.DungeonLadderDown || cell.Kind() == references.DungeonLadderBoth) {
				return position, true
			}
		}
	}

	return references.Position{}, false
}

func TestOutdoorWindowGuardianCorrectionPreservesAttemptedShift(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithPlayerAt(233, 248).Build()
	require.Equal(t, references.Position{X: 224, Y: 240}, state.outdoorWindow.origin)
	// A direct displacement (as used by Push) preserves this window. The next
	// attempted step scrolls before the guardian sends the party south again.
	state.SetPlayerPosition(references.Position{X: 233, Y: 236})
	state.MovePlayerOneStep(references.Up)
	shifted := state.outdoorWindow
	require.Equal(t, references.Position{X: 224, Y: 224}, shifted.origin)
	state.processCodexGuardianCheckpoint()
	assert.Equal(t, references.Position{X: 233, Y: 236}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, shifted, state.outdoorWindow)
}

func TestOutdoorWindowViewAndDisplayUpdatesPreserveNextBlink(t *testing.T) {
	state := newBlinkHistoryFixture(t, -1)
	window := state.outdoorWindow
	state.PartyState.Inventory.Provisions.Gems.Set(2)
	require.True(t, state.ActionViewLargeMap())
	state.MapState.XTilesVisibleOnGameScreen = 31
	state.MapState.YTilesVisibleOnGameScreen = 21
	for range 5 {
		state.SystemCallbacks.Screen.UpdateDisplay()
		state.EventQueueManager.Update()
	}
	assert.Equal(t, window, state.outdoorWindow)
	require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Right))
	assert.Equal(t, references.Position{X: 124, Y: 120}, state.MapState.PlayerLocation.Position)
}

func TestOutdoorWindowWaterfallUpdatesEachForcedSouthStep(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		for _, test := range []struct {
			startY      references.Coordinate
			firstOrigin references.Coordinate
		}{
			{startY: 121, firstOrigin: 96},  // second forced step crosses the threshold
			{startY: 122, firstOrigin: 112}, // first forced step crosses the threshold
		} {
			state, _ := NewIntegrationTestBuilder(t).WithFloor(floor).WithPlayerAt(120, 119).Build()
			for y := references.Coordinate(119); y < test.startY; y++ {
				state.MovePlayerOneStep(references.Down)
			}
			require.Equal(t, references.Position{X: 112, Y: 96}, state.outdoorWindow.origin)
			cascade := &waterfallCascade{landing: state.MapState.PlayerLocation}
			state.activeWaterfall = cascade
			state.advanceWaterfallSouthStep(cascade)
			assert.Equal(t, references.Position{X: 120, Y: test.startY + 1}, state.MapState.PlayerLocation.Position)
			assert.Equal(t, references.Position{X: 112, Y: test.firstOrigin}, state.outdoorWindow.origin)
			state.advanceWaterfallSouthStep(cascade)
			assert.Equal(t, references.Position{X: 120, Y: test.startY + 2}, state.MapState.PlayerLocation.Position)
			assert.Equal(t, references.Position{X: 112, Y: 112}, state.outdoorWindow.origin)
			state.cancelWaterfallCascade(cascade)
			setBlinkRayTiles(t, state.GetCurrentLayeredMap(), state.MapState.PlayerLocation.Position, references.Down, indexes.Forest1)
			grass := references.Position{X: 120, Y: 140}
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &grass, indexes.Grass)
			require.Equal(t, BlinkSuccess, state.BlinkTeleport(references.Down))
			assert.Equal(t, grass, state.MapState.PlayerLocation.Position)
		}
	}
}
