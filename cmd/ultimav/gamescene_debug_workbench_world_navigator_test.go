package main

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestWorldPositionStatusReturnsEveryDestinationFloorAndDefault(t *testing.T) {
	scene := newWorldNavigatorDebugTestScene(t)

	result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandWorldPositionStatus})

	require.True(t, result.Success, result.ErrorMessage)
	var status debugging.WorldPositionStatusData
	require.NoError(t, json.Unmarshal(result.Data, &status))
	assert.Equal(t, int(references.Britain), status.LocationID)
	assert.Equal(t, 0, status.Floor)
	assert.Equal(t, 10, status.X)
	assert.Equal(t, 11, status.Y)
	assert.Len(t, status.SourceFingerprint, sha256HexLength)
	assert.Equal(t, debugging.WorldPositionBoundsData{MinimumX: 0, MaximumX: 31, MinimumY: 0, MaximumY: 31}, status.Bounds)
	require.Len(t, status.Destinations, int(references.Doom)+1)

	for location := references.BritanniaUnderworld; location <= references.Doom; location++ {
		destination := status.Destinations[int(location)]
		assert.Equal(t, int(location), destination.LocationID)
		assert.Equal(t, debugMapTypeName(location.GetMapType()), destination.MapType)
		assert.Contains(t, destination.Floors, destination.DefaultSetup.Floor)
		assert.GreaterOrEqual(t, destination.DefaultSetup.X, destination.Bounds.MinimumX)
		assert.LessOrEqual(t, destination.DefaultSetup.X, destination.Bounds.MaximumX)
		assert.GreaterOrEqual(t, destination.DefaultSetup.Y, destination.Bounds.MinimumY)
		assert.LessOrEqual(t, destination.DefaultSetup.Y, destination.Bounds.MaximumY)
		if location == references.BritanniaUnderworld {
			assert.Nil(t, destination.WorldEntrance)
		} else if location.GetMapType() != references.DungeonMapType {
			require.NotNil(t, destination.WorldEntrance)
			worldPosition, worldFloor := scene.gameState.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(location)
			assert.Equal(t, int(worldFloor), destination.WorldEntrance.Floor)
			assert.Equal(t, int(worldPosition.X), destination.WorldEntrance.X)
			assert.Equal(t, int(worldPosition.Y), destination.WorldEntrance.Y)
		} else {
			assert.Nil(t, destination.WorldEntrance)
			entrance := scene.gameState.GetDungeonEntranceCoordinates()[location]
			underworldOutside := worldPlacementTarget(
				t,
				destination,
				debugging.WorldPlacementDungeonOutside,
				debugging.WorldRouteUnderworld,
			)
			require.True(t, underworldOutside.Available)
			require.NotNil(t, underworldOutside.Setup)
			assert.Equal(t, int(references.BritanniaUnderworld), underworldOutside.LocationID)
			assert.Equal(t, -1, underworldOutside.Setup.Floor)
			assert.Equal(t, int(entrance.X), underworldOutside.Setup.X)
			assert.Equal(t, int(entrance.Y), underworldOutside.Setup.Y)

			underworldStart := worldPlacementTarget(
				t,
				destination,
				debugging.WorldPlacementDungeonInsideStart,
				debugging.WorldRouteUnderworld,
			)
			require.True(t, underworldStart.Available)
			require.NotNil(t, underworldStart.Setup)
			if location == references.Doom {
				assert.Len(t, destination.PlacementTargets, 3)
				assert.Equal(t, debugging.WorldPositionSetupData{Floor: 0, X: 1, Y: 1}, *underworldStart.Setup)
				assert.Equal(t, "East", underworldStart.FacingLabel)
				assert.Equal(t, "Arrived from above", underworldStart.LastDirectionLabel)
				exitTarget := worldPlacementTarget(
					t,
					destination,
					debugging.WorldPlacementDungeonUnderworldExit,
					debugging.WorldRouteUnderworld,
				)
				assert.False(t, exitTarget.Available)
				assert.Nil(t, exitTarget.Setup)
				assert.Contains(t, exitTarget.UnavailableReason, "no down-ladder Underworld endpoint")
				assert.Empty(t, worldPlacementTargets(destination, debugging.WorldRouteBritannia))
			} else {
				assert.Len(t, destination.PlacementTargets, 5)
				britanniaOutside := worldPlacementTarget(
					t,
					destination,
					debugging.WorldPlacementDungeonOutside,
					debugging.WorldRouteBritannia,
				)
				require.NotNil(t, britanniaOutside.Setup)
				assert.Equal(t, debugging.WorldPositionSetupData{
					Floor: 0,
					X:     int(entrance.X),
					Y:     int(entrance.Y),
				}, *britanniaOutside.Setup)
				britanniaStart := worldPlacementTarget(
					t,
					destination,
					debugging.WorldPlacementDungeonInsideStart,
					debugging.WorldRouteBritannia,
				)
				require.NotNil(t, britanniaStart.Setup)
				assert.Equal(t, debugging.WorldPositionSetupData{Floor: 0, X: 1, Y: 1}, *britanniaStart.Setup)
				assert.Equal(t, "East", britanniaStart.FacingLabel)
				assert.Equal(t, "Arrived from above", britanniaStart.LastDirectionLabel)
				assert.Equal(t, debugging.WorldPositionSetupData{Floor: 7, X: 7, Y: 7}, *underworldStart.Setup)
				assert.Equal(t, "West", underworldStart.FacingLabel)
				assert.Equal(t, "Arrived from below", underworldStart.LastDirectionLabel)
				exitTarget := worldPlacementTarget(
					t,
					destination,
					debugging.WorldPlacementDungeonUnderworldExit,
					debugging.WorldRouteUnderworld,
				)
				require.True(t, exitTarget.Available)
				require.NotNil(t, exitTarget.Setup)
				assert.Equal(t, debugging.WorldPositionSetupData{Floor: 7, X: 7, Y: 7}, *exitTarget.Setup)
				assert.Equal(t, "West", exitTarget.FacingLabel)
				assert.Equal(t, "West", exitTarget.LastDirectionLabel)
			}
		}

		switch location.GetMapType() {
		case references.LargeMapType:
			assert.Equal(t, []int{-1, 0}, destination.Floors)
			assert.Equal(t, 255, destination.Bounds.MaximumX)
		case references.DungeonMapType:
			assert.Equal(t, []int{0, 1, 2, 3, 4, 5, 6, 7}, destination.Floors)
			assert.Equal(t, 7, destination.Bounds.MaximumX)
		case references.SmallMapType:
			locationReference := scene.gameState.GameReferences.LocationReferences.GetLocationReference(location)
			require.NotNil(t, locationReference)
			expectedFloors := make([]int, len(locationReference.ListOfFloors))
			for index, floor := range locationReference.ListOfFloors {
				expectedFloors[index] = int(floor)
			}
			assert.Equal(t, expectedFloors, destination.Floors)
			assert.Equal(t, 31, destination.Bounds.MaximumX)
		case references.CombatMapType, references.CampingMapType:
			t.Fatalf("unexpected destination map type %d", location.GetMapType())
		}
	}
}

func TestWorldPositionSetPreservesSwitchesAndClearsDungeonAuthority(t *testing.T) {
	scene := newWorldNavigatorDebugTestScene(t)
	state := scene.gameState
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		0,
		references.Position{X: 1, Y: 1},
		references.DungeonFacingWest,
		false,
	)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(5, references.Position{X: 6, Y: 7}, func(references.DungeonCell) references.DungeonCell {
		return references.DungeonCell(0xc8)
	})
	require.NoError(t, err)
	beforePreserved, active := state.DungeonStateSnapshot()
	require.True(t, active)

	preserved := scene.ExecuteDebugCommand(worldPositionSetDebugRequest(
		state,
		references.Deceit,
		5,
		references.Position{X: 6, Y: 7},
	))

	require.True(t, preserved.Success, preserved.ErrorMessage)
	afterPreserved, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, beforePreserved, afterPreserved)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Deceit,
		Floor:    5,
		Position: references.Position{X: 6, Y: 7},
	}, state.MapState.PlayerLocation)
	assert.Equal(t, []string{"world.floor", "world.position.x", "world.position.y"}, stateChangePaths(preserved.Changes))
	noOpPosition := scene.ExecuteDebugCommand(worldPositionSetDebugRequest(
		state,
		references.Deceit,
		5,
		references.Position{X: 6, Y: 7},
	))
	require.True(t, noOpPosition.Success, noOpPosition.ErrorMessage)
	assert.Empty(t, noOpPosition.Changes)
	state.MapState.Lighting.RestoreTorchMinutes(17)
	state.MagicLightMinutesRemaining = 23
	state.MapState.Lighting.SetMagicLight(true)

	switched := scene.ExecuteDebugCommand(worldPositionSetDebugRequest(
		state,
		references.Doom,
		2,
		references.Position{X: 2, Y: 3},
	))

	require.True(t, switched.Success, switched.ErrorMessage)
	switchSnapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, references.DungeonFacingEast, switchSnapshot.Facing)
	assert.Equal(t, references.DungeonLastEast, switchSnapshot.LastDirection)
	assert.Zero(t, switchSnapshot.Revision)
	assert.True(t, state.MapState.Lighting.HasAnyLight())
	assert.Equal(t, 17, state.MapState.Lighting.TorchMinutesRemaining())
	assert.Equal(t, 23, state.MagicLightMinutesRemaining)
	assert.Contains(t, stateChangePaths(switched.Changes), "world.locationId")
	assert.Contains(t, stateChangePaths(switched.Changes), "dungeon.facing")
	assert.Contains(t, stateChangePaths(switched.Changes), "dungeon.lastDirection")
	assert.Contains(t, stateChangePaths(switched.Changes), "dungeon.revision")
	assert.NotContains(t, stateChangePaths(switched.Changes), "lighting.lit")
	assert.NotContains(t, stateChangePaths(switched.Changes), "lighting.torchMinutesRemaining")
	assert.NotContains(t, stateChangePaths(switched.Changes), "lighting.magicLightMinutesRemaining")

	left := scene.ExecuteDebugCommand(worldPositionSetDebugRequest(
		state,
		references.Britain,
		1,
		references.Position{X: 15, Y: 30},
	))

	require.True(t, left.Success, left.ErrorMessage)
	_, active = state.DungeonStateSnapshot()
	assert.False(t, active)
	assert.Equal(t, references.Britain, state.MapState.PlayerLocation.Location)
	assert.Contains(t, stateChangePaths(left.Changes), "dungeon.active")
}

func TestWorldPositionSetResolvesDungeonRouteIntentsAndRestartsAuthority(t *testing.T) {
	scene := newWorldNavigatorDebugTestScene(t)
	state := scene.gameState
	state.MapState.Lighting.RestoreTorchMinutes(17)
	state.MagicLightMinutesRemaining = 23
	state.MapState.Lighting.SetMagicLight(true)
	beforeDate := state.DateTime
	beforeRNGCalls := state.GetRNG().Calls()
	entrance := state.GetDungeonEntranceCoordinates()[references.Deceit]

	outside := scene.ExecuteDebugCommand(worldRoutePositionSetDebugRequest(
		state,
		references.Deceit,
		debugging.WorldPlacementDungeonOutside,
		debugging.WorldRouteUnderworld,
	))

	require.True(t, outside.Success, outside.ErrorMessage)
	assert.Equal(t, references.PlayerLocation{
		Location: references.BritanniaUnderworld,
		Floor:    -1,
		Position: entrance,
	}, state.MapState.PlayerLocation)
	assert.False(t, state.IsDungeonOpen(references.Deceit))
	assert.False(t, state.ActionEnterLargeMap(), "outside placement must not bypass the normal sealed-entrance gate")
	_, active := state.DungeonStateSnapshot()
	assert.False(t, active)
	assert.Contains(t, outside.Summary, "floor -1")
	assert.Contains(t, outside.Summary, "via Underworld")
	var outsideStatus debugging.WorldPositionStatusData
	require.NoError(t, json.Unmarshal(outside.Data, &outsideStatus))
	assert.Equal(t, int(references.BritanniaUnderworld), outsideStatus.LocationID)
	assert.Equal(t, -1, outsideStatus.Floor)
	assert.Equal(t, int(entrance.X), outsideStatus.X)
	assert.Equal(t, int(entrance.Y), outsideStatus.Y)
	assert.Equal(t, []string{"world.locationId", "world.floor", "world.position.x", "world.position.y"}, stateChangePaths(outside.Changes))

	inside := scene.ExecuteDebugCommand(worldRoutePositionSetDebugRequest(
		state,
		references.Deceit,
		debugging.WorldPlacementDungeonInsideStart,
		debugging.WorldRouteUnderworld,
	))

	require.True(t, inside.Success, inside.ErrorMessage)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Deceit,
		Floor:    7,
		Position: references.Position{X: 7, Y: 7},
	}, state.MapState.PlayerLocation)
	insideSnapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, references.DungeonFacingWest, insideSnapshot.Facing)
	assert.Equal(t, references.DungeonArrivedFromBelow, insideSnapshot.LastDirection)
	assert.Equal(t, 17, state.MapState.Lighting.TorchMinutesRemaining())
	assert.Equal(t, 23, state.MagicLightMinutesRemaining)
	assert.True(t, state.MapState.Lighting.HasMagicLight())
	if state.HasPendingDungeonRoomEncounter() {
		require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
		state.EndCombat(game_state.CombatDefeat)
		beforeRNGCalls = state.GetRNG().Calls()
	}

	changedPosition := references.Position{X: 6, Y: 5}
	_, err := state.TransformDungeonCell(7, changedPosition, func(cell references.DungeonCell) references.DungeonCell {
		return cell ^ 0x01
	})
	require.NoError(t, err)
	changedSnapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	require.NotZero(t, changedSnapshot.Revision)

	exitTarget := scene.ExecuteDebugCommand(worldRoutePositionSetDebugRequest(
		state,
		references.Deceit,
		debugging.WorldPlacementDungeonUnderworldExit,
		debugging.WorldRouteUnderworld,
	))

	require.True(t, exitTarget.Success, exitTarget.ErrorMessage)
	exitSnapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Zero(t, exitSnapshot.Revision)
	assert.Equal(t, references.DungeonFacingWest, exitSnapshot.Facing)
	assert.Equal(t, references.DungeonLastWest, exitSnapshot.LastDirection)
	assert.NotEqual(t, changedSnapshot.Cells, exitSnapshot.Cells)
	assert.Equal(t, beforeDate, state.DateTime)
	assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
	assert.Equal(t, 17, state.MapState.Lighting.TorchMinutesRemaining())
	assert.Equal(t, 23, state.MagicLightMinutesRemaining)
	if state.HasPendingDungeonRoomEncounter() {
		require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
		state.EndCombat(game_state.CombatDefeat)
	}

	doomStart := scene.ExecuteDebugCommand(worldRoutePositionSetDebugRequest(
		state,
		references.Doom,
		debugging.WorldPlacementDungeonInsideStart,
		debugging.WorldRouteUnderworld,
	))
	require.True(t, doomStart.Success, doomStart.ErrorMessage)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Doom,
		Floor:    0,
		Position: references.Position{X: 1, Y: 1},
	}, state.MapState.PlayerLocation)
	doomSnapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, references.DungeonFacingEast, doomSnapshot.Facing)
	assert.Equal(t, references.DungeonArrivedFromAbove, doomSnapshot.LastDirection)
	if state.HasPendingDungeonRoomEncounter() {
		require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
		state.EndCombat(game_state.CombatDefeat)
	}

	beforeLocation := state.MapState.PlayerLocation
	beforeDungeon, active := state.DungeonStateSnapshot()
	require.True(t, active)
	unavailable := scene.ExecuteDebugCommand(worldRoutePositionSetDebugRequest(
		state,
		references.Doom,
		debugging.WorldPlacementDungeonUnderworldExit,
		debugging.WorldRouteUnderworld,
	))
	assert.Equal(t, "invalid_world_setup", unavailable.ErrorCode)
	assert.Contains(t, unavailable.ErrorMessage, "Doom has no down-ladder Underworld endpoint")
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	afterDungeon, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, beforeDungeon, afterDungeon)
	assert.Empty(t, unavailable.Changes)
}

func TestWorldPositionSetOutsideARoomLeavesOneStepFromThatRoom(t *testing.T) {
	scene := newWorldNavigatorDebugTestScene(t)
	state := scene.gameState
	state.MapState.Lighting.RestoreTorchMinutes(17)
	state.MagicLightMinutesRemaining = 23
	state.MapState.Lighting.SetMagicLight(true)

	var status debugging.WorldPositionStatusData
	require.NoError(t, json.Unmarshal(scene.ExecuteDebugCommand(debugging.Request{
		CommandID: debugging.CommandWorldPositionStatus,
	}).Data, &status))
	destination := status.Destinations[int(references.Covetous)]
	require.Equal(t, int(references.Covetous), destination.LocationID)
	require.NotEmpty(t, destination.Rooms)
	var listed debugging.WorldPositionDungeonRoomData
	for _, room := range destination.Rooms {
		if room.Room == 12 {
			listed = room
		}
	}
	require.True(t, listed.Available, listed.UnavailableReason)
	assert.Equal(t, debugging.WorldPositionDungeonRoomData{
		Room: 12, Label: "Room 12", Floor: 5, RoomX: 3, RoomY: 4, Available: true,
		Setup: listed.Setup, FacingLabel: listed.FacingLabel, Summary: listed.Summary,
	}, listed)
	require.NotNil(t, listed.Setup)
	assert.NotEmpty(t, listed.FacingLabel)
	assert.NotEmpty(t, listed.Summary, "the room preview names its fixed monsters")
	assert.Empty(t, status.Destinations[int(references.Britain)].Rooms, "only dungeons list rooms")

	request := worldRoutePositionSetDebugRequest(
		state, references.Covetous, debugging.WorldPlacementDungeonOutsideRoom, debugging.WorldRouteBritannia,
	)
	request.Arguments["room"] = mustDebugJSON(12)
	result := scene.ExecuteDebugCommand(request)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Covetous,
		Floor:    references.FloorNumber(listed.Setup.Floor),
		Position: references.Position{X: references.Coordinate(listed.Setup.X), Y: references.Coordinate(listed.Setup.Y)},
	}, state.MapState.PlayerLocation)
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, listed.FacingLabel, debugDungeonFacingLabel(snapshot.Facing))
	assert.Equal(t, references.DungeonLastDirection(snapshot.Facing), snapshot.LastDirection)
	assert.Equal(t, 17, state.MapState.Lighting.TorchMinutesRemaining())
	assert.Equal(t, 23, state.MagicLightMinutesRemaining)
	assert.False(t, state.HasPendingDungeonRoomEncounter(), "the party stands beside the room, not in it")

	token, _ := state.BeginDungeonCommandCycle()
	move := state.TryDungeonAdvance()
	state.CompleteDungeonCommandCycle(token, move.Completion)

	assert.Equal(t, game_state.DungeonMoveMoved, move.Outcome)
	assert.Equal(t, references.Position{X: 3, Y: 4}, state.MapState.PlayerLocation.Position)
	require.True(t, state.HasPendingDungeonRoomEncounter())
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	assert.Equal(t, 12, state.CombatScene.DungeonRoomOrigin.Room)
}

func TestWorldPositionSetOutsideARoomRejectsMissingAndRoomlessDungeons(t *testing.T) {
	scene := newWorldNavigatorDebugTestScene(t)
	state := scene.gameState
	before := state.MapState.PlayerLocation
	request := worldRoutePositionSetDebugRequest(
		state, references.Covetous, debugging.WorldPlacementDungeonOutsideRoom, debugging.WorldRouteBritannia,
	)

	missing := scene.ExecuteDebugCommand(request)

	assert.Equal(t, "invalid_world_setup", missing.ErrorCode)
	assert.Contains(t, missing.ErrorMessage, "requires a room")

	request.Arguments["location"] = mustDebugJSON(strconv.Itoa(int(references.Despise)))
	request.Arguments["room"] = mustDebugJSON(0)
	rejected := scene.ExecuteDebugCommand(request)

	assert.Equal(t, "invalid_world_setup", rejected.ErrorCode)
	assert.Contains(t, rejected.ErrorMessage, "room 0 is not placed in Despise")
	assert.Equal(t, before, state.MapState.PlayerLocation)
	_, active := state.DungeonStateSnapshot()
	assert.False(t, active)
}

func TestWorldPositionSetRejectsStaleReviewsWithoutPartialMutation(t *testing.T) {
	t.Run("world position", func(t *testing.T) {
		scene := newWorldNavigatorDebugTestScene(t)
		state := scene.gameState
		request := worldPositionSetDebugRequest(
			state,
			references.Minoc,
			0,
			references.Position{X: 15, Y: 30},
		)
		request.Arguments["expectedX"] = json.RawMessage(`9`)
		before := state.MapState.PlayerLocation

		result := scene.ExecuteDebugCommand(request)

		assert.Equal(t, "stale_world_position", result.ErrorCode)
		assert.Equal(t, "world position changed after review", result.ErrorMessage)
		assert.Equal(t, before, state.MapState.PlayerLocation)
		assert.Empty(t, result.Changes)
	})

	t.Run("world source lighting", func(t *testing.T) {
		scene := newWorldNavigatorDebugTestScene(t)
		state := scene.gameState
		request := worldPositionSetDebugRequest(
			state,
			references.Minoc,
			0,
			references.Position{X: 15, Y: 30},
		)
		before := state.MapState.PlayerLocation
		state.MapState.Lighting.RestoreTorchMinutes(9)

		result := scene.ExecuteDebugCommand(request)

		assert.Equal(t, "stale_world_position", result.ErrorCode)
		assert.Equal(t, "world source state changed after review", result.ErrorMessage)
		assert.Equal(t, before, state.MapState.PlayerLocation)
		assert.Equal(t, 9, state.MapState.Lighting.TorchMinutesRemaining())
		assert.Empty(t, result.Changes)
	})
}

func TestDungeonSetupChangesReportWandererSemantically(t *testing.T) {
	before := dungeonAuthoritySnapshot{
		Active: true,
		Snapshot: game_state.DungeonStateSnapshot{
			Location: references.Deceit,
			Wanderer: &game_state.DungeonWanderer{
				Monster:          references.MonsterSlime,
				Floor:            4,
				Position:         references.Position{X: 3, Y: 4},
				PreviousPosition: references.Position{X: 2, Y: 4},
				OnCeiling:        true,
			},
		},
	}
	after := before
	after.Snapshot.Wanderer = nil
	changes := make([]debugging.StateChange, 0)

	appendDungeonSetupChanges(&changes, before, after)

	require.Len(t, changes, 1)
	assert.Equal(t, "dungeon.wanderer", changes[0].Path)
	assert.JSONEq(t, `{"monster":24,"floor":4,"x":3,"y":4,"previousX":2,"previousY":4,"onCeiling":true}`, string(changes[0].Before))
	assert.JSONEq(t, `null`, string(changes[0].After))
}

func TestWorldPositionSetUsesWorldSetupSceneBlockers(t *testing.T) {
	blockers := []struct {
		name      string
		errorCode string
		apply     func(*GameScene)
	}{
		{
			name: "dialog", errorCode: "dialog_active",
			apply: func(scene *GameScene) {
				scene.dialogStack.PushModalDialog(&blockingDebugWorkflowDialog{})
			},
		},
		{
			name: "workflow", errorCode: "gameplay_workflow_active",
			apply: func(scene *GameScene) {
				scene.gameState.EventQueueManager.Workflow("blocking").
					Priority(event_queue.PriorityNormal).
					Sequential().
					First(func() {}).
					Finally(func() {}).
					Start()
			},
		},
		{
			name: "scripted scene", errorCode: "scripted_scene_active",
			apply: func(scene *GameScene) {
				scene.miniMapScene = minimap.NewMiniMapScene()
			},
		},
	}
	for _, blocker := range blockers {
		t.Run(blocker.name, func(t *testing.T) {
			scene := newWorldNavigatorDebugTestScene(t)
			state := scene.gameState
			request := worldPositionSetDebugRequest(state, references.Minoc, 0, references.Position{X: 15, Y: 30})
			before := state.MapState.PlayerLocation
			blocker.apply(scene)

			result := scene.ExecuteDebugCommand(request)

			assert.Equal(t, blocker.errorCode, result.ErrorCode)
			assert.Equal(t, before, state.MapState.PlayerLocation)
			_, active := state.DungeonStateSnapshot()
			assert.False(t, active)
		})
	}
}

func newWorldNavigatorDebugTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithFloor(0).
		WithPlayerAt(10, 11).
		WithSystemCallbacks().
		Build()

	return &GameScene{gameState: state}
}

func worldPositionSetDebugRequest(
	state *game_state.GameState,
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
) debugging.Request {
	arguments := reviewedWorldPositionDebugArguments(state)
	arguments["location"] = mustDebugJSON(strconv.Itoa(int(location)))
	arguments["floor"] = mustDebugJSON(strconv.Itoa(int(floor)))
	arguments["x"] = mustDebugJSON(int(position.X))
	arguments["y"] = mustDebugJSON(int(position.Y))

	return debugging.Request{CommandID: debugging.CommandWorldPositionSet, Arguments: arguments}
}

func worldRoutePositionSetDebugRequest(
	state *game_state.GameState,
	location references.Location,
	placement string,
	route string,
) debugging.Request {
	arguments := reviewedWorldPositionDebugArguments(state)
	arguments["placement"] = mustDebugJSON(placement)
	arguments["route"] = mustDebugJSON(route)
	arguments["location"] = mustDebugJSON(strconv.Itoa(int(location)))

	return debugging.Request{CommandID: debugging.CommandWorldPositionSet, Arguments: arguments}
}

func reviewedWorldPositionDebugArguments(state *game_state.GameState) map[string]json.RawMessage {
	current := state.MapState.PlayerLocation

	return map[string]json.RawMessage{
		"expectedLocationId":        mustDebugJSON(int(current.Location)),
		"expectedFloor":             mustDebugJSON(int(current.Floor)),
		"expectedX":                 mustDebugJSON(int(current.Position.X)),
		"expectedY":                 mustDebugJSON(int(current.Position.Y)),
		"expectedSourceFingerprint": mustDebugJSON(debugWorldSourceFingerprint(state)),
	}
}

func worldPlacementTarget(
	t *testing.T,
	destination debugging.WorldPositionDestinationData,
	placement string,
	route string,
) debugging.WorldPositionPlacementTargetData {
	t.Helper()
	for _, target := range destination.PlacementTargets {
		if target.Placement == placement && target.Route == route {
			return target
		}
	}
	require.FailNow(t, "world placement target not found", "%s via %s", placement, route)

	return debugging.WorldPositionPlacementTargetData{}
}

func worldPlacementTargets(
	destination debugging.WorldPositionDestinationData,
	route string,
) []debugging.WorldPositionPlacementTargetData {
	targets := make([]debugging.WorldPositionPlacementTargetData, 0)
	for _, target := range destination.PlacementTargets {
		if target.Route == route {
			targets = append(targets, target)
		}
	}

	return targets
}

const sha256HexLength = 64
