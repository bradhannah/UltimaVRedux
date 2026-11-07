package game_state

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestValidateDungeonDoorwaySetupRejectsTrappedFacing(t *testing.T) {
	var cells references.DungeonCells
	position := references.Position{X: 3, Y: 3}
	cells.SetCell(0, position, references.DungeonCell(0xe0))
	cells.SetCell(0, references.Position{X: 3, Y: 2}, references.DungeonCell(0xb0))
	cells.SetCell(0, references.Position{X: 3, Y: 4}, references.DungeonCell(0xc0))
	cells.SetCell(0, references.Position{X: 2, Y: 3}, references.DungeonCell(0x00))
	dungeon, err := newDungeonState(
		references.Deceit, cells, references.DungeonFacingNorth, references.DungeonLastNorth,
	)
	require.NoError(t, err)

	err = validateDungeonDoorwaySetup(dungeon, 0, position, references.DungeonFacingNorth)
	assert.ErrorContains(t, err, "no traversable forward or reverse route")
	assert.NoError(t, validateDungeonDoorwaySetup(dungeon, 0, position, references.DungeonFacingWest))
}

func TestDebugEnterMerchantLocationUsesProductionControllerAndBypassesVehicleGates(t *testing.T) {
	for _, vehicle := range []references.VehicleType{
		references.NoPartyVehicle,
		references.CarpetVehicle,
		references.HorseVehicle,
		references.SkiffVehicle,
		references.FrigateVehicle,
	} {
		t.Run(vehicle.GetLowerCaseName(), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithSystemCallbacks().
				WithRandomSeed(288).
				Build()
			setPartyTestVehicle(state, vehicle)
			before := state.MapState.PlayerLocation
			beforeRNGCalls := state.GetRNG().Calls()

			require.NoError(t, state.ValidateDebugMerchantLocationTransition(references.MerchantGuildPaws))
			assert.Equal(t, before, state.MapState.PlayerLocation)
			assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())

			operation, err := state.DebugEnterMerchantLocation(references.MerchantGuildPaws)
			require.NoError(t, err)
			assert.Equal(t, before, operation.Before)
			assert.Equal(t, references.Paws, operation.After.Location)
			assert.Equal(t, references.Paws, state.MapState.PlayerLocation.Location)
			assert.NotNil(t, state.CurrentNPCAIController)
			assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
			_, err = state.ReviewDebugMerchantSessionByID(references.MerchantGuildPaws)
			require.NoError(t, err)
		})
	}
}

func TestValidateDebugMerchantLocationTransitionSupportsAllMerchantFamilies(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	merchantIDs := []references.MerchantID{
		references.MerchantArmsBritain,
		references.MerchantTavernBritain,
		references.MerchantHorseTrinsic,
		references.MerchantShipJhelom,
		references.MerchantReagentMoonglow,
		references.MerchantGuildPaws,
		references.MerchantHealerMinoc,
		references.MerchantInnBritain,
	}

	for _, id := range merchantIDs {
		t.Run(string(id), func(t *testing.T) {
			merchant, found := state.GameReferences.MerchantReferences.Definition(id)
			require.True(t, found)
			npcReferences := state.GameReferences.LocationReferences.
				GetLocationReference(merchant.Location).GetNPCReferences()
			npcReference := (*npcReferences)[merchant.NPCIndex]
			available := false
			for hour := range 24 {
				candidate := state.DateTime
				candidate.Hour = byte(hour)
				desiredSlot := npcReference.Schedule.DesiredSlot(candidate)
				if npcReference.Schedule.GetIndividualNPCBehaviourBySlot(desiredSlot).Floor ==
					smallMapStartingPositionFloor {
					state.DateTime = candidate
					available = true

					break
				}
			}
			require.True(t, available, "merchant has no schedule on the tester target floor")
			require.NoError(t, state.ValidateDebugMerchantLocationTransition(id))
		})
	}
}

func TestDebugEnterMerchantLocationRejectsDestinationFailuresWithoutMutation(t *testing.T) {
	tests := []struct {
		name      string
		merchant  references.MerchantID
		configure func(*testing.T, *GameState, references.MerchantDefinition)
		message   string
	}{
		{
			name:     "unknown merchant",
			merchant: references.MerchantID("guild.unknown"),
			message:  "unknown merchant ID",
		},
		{
			name:     "missing location references",
			merchant: references.MerchantGuildPaws,
			configure: func(_ *testing.T, state *GameState, _ references.MerchantDefinition) {
				gameReferences := *state.GameReferences
				gameReferences.LocationReferences = nil
				state.GameReferences = &gameReferences
			},
			message: "destination references are unavailable",
		},
		{
			name:     "absent NPC reference",
			merchant: references.MerchantGuildPaws,
			configure: func(t *testing.T, state *GameState, merchant references.MerchantDefinition) {
				npcReferences := state.GameReferences.LocationReferences.
					GetLocationReference(merchant.Location).GetNPCReferences()
				original := *npcReferences
				*npcReferences = append([]references.NPCReference(nil), original[:merchant.NPCIndex]...)
				t.Cleanup(func() { *npcReferences = original })
			},
			message: "no canonical NPC reference",
		},
		{
			name:     "malformed NPC identity",
			merchant: references.MerchantGuildPaws,
			configure: func(t *testing.T, state *GameState, merchant references.MerchantDefinition) {
				npcReferences := state.GameReferences.LocationReferences.
					GetLocationReference(merchant.Location).GetNPCReferences()
				original := *npcReferences
				replacement := append([]references.NPCReference(nil), original...)
				replacement[merchant.NPCIndex].DialogNumber = 0x89
				*npcReferences = replacement
				t.Cleanup(func() { *npcReferences = original })
			},
			message: "canonical NPC reference does not match",
		},
		{
			name:     "dead NPC",
			merchant: references.MerchantGuildPaws,
			configure: func(_ *testing.T, state *GameState, merchant references.MerchantDefinition) {
				state.PartyState.SetDeadNpc(merchant.Location, merchant.NPCIndex)
			},
			message: "is dead",
		},
		{
			name:     "NPC scheduled off target floor",
			merchant: references.MerchantGuildPaws,
			configure: func(t *testing.T, state *GameState, merchant references.MerchantDefinition) {
				npcReferences := state.GameReferences.LocationReferences.
					GetLocationReference(merchant.Location).GetNPCReferences()
				original := *npcReferences
				replacement := append([]references.NPCReference(nil), original...)
				npc := &replacement[merchant.NPCIndex]
				desiredSlot := npc.Schedule.DesiredSlot(state.DateTime)
				npc.Schedule.Floor[desiredSlot] = byte(smallMapStartingPositionFloor + 1)
				*npcReferences = replacement
				t.Cleanup(func() { *npcReferences = original })
			},
			message: "rather than target floor",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithSystemCallbacks().
				Build()
			merchant, _ := state.GameReferences.MerchantReferences.Definition(test.merchant)
			if test.configure != nil {
				test.configure(t, state, merchant)
			}
			beforeLocation := state.MapState.PlayerLocation
			beforeController := state.CurrentNPCAIController
			beforeMap := state.GetCurrentLayeredMap()
			beforeRNGCalls := state.GetRNG().Calls()

			operation, err := state.DebugEnterMerchantLocation(test.merchant)

			require.ErrorIs(t, err, ErrInvalidWorldSetup)
			assert.ErrorContains(t, err, test.message)
			assert.Equal(t, beforeLocation, operation.Before)
			assert.Equal(t, beforeLocation, operation.After)
			assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
			assert.Same(t, beforeController, state.CurrentNPCAIController)
			assert.Same(t, beforeMap, state.GetCurrentLayeredMap())
			assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
		})
	}
}

func TestDebugTravelToLandmarkUsesPhysicalDestinations(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	for _, test := range []struct {
		name        string
		destination references.PlayerLocation
	}{
		{name: "honesty", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 233, Y: 66}}},
		{name: "compassion", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 128, Y: 92}}},
		{name: "valour", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 36, Y: 229}}},
		{name: "justice", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 73, Y: 11}}},
		{name: "sacrifice", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 205, Y: 45}}},
		{name: "honor", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 81, Y: 207}}},
		{name: "humility", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 231, Y: 216}}},
		{name: "codex", destination: references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: references.Position{X: 233, Y: 233}}},
		{name: "lord_british_secret_room", destination: references.PlayerLocation{Location: references.LordBritishsCastle, Floor: 2, Position: references.Position{X: 17, Y: 12}}},
		{name: "flame_truth", destination: references.PlayerLocation{Location: references.Lycaeum, Floor: 2, Position: references.Position{X: 15, Y: 9}}},
		{name: "flame_love", destination: references.PlayerLocation{Location: references.EmpathAbbey, Floor: 1, Position: references.Position{X: 15, Y: 3}}},
		{name: "flame_courage", destination: references.PlayerLocation{Location: references.SerpentsHold, Floor: -1, Position: references.Position{X: 15, Y: 16}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			result, err := state.DebugTravelToLandmark(test.name)
			require.NoError(t, err)
			assert.Equal(t, test.destination, result.After)
		})
	}

	before := state.MapState.PlayerLocation
	result, err := state.DebugTravelToLandmark("spirituality")
	assert.True(t, errors.Is(err, ErrInvalidWorldSetup))
	assert.Equal(t, before, result.After)
}

func TestDebugSetupDungeonSupportsEveryDungeonAndExactSemanticSelection(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	for location := references.Deceit; location <= references.Doom; location++ {
		index := int(location - references.Deceit)
		floor := references.FloorNumber(index)
		position := references.Position{X: references.Coordinate(index), Y: references.Coordinate(index)}
		facing := references.DungeonFacing(index % 4)
		lit := index%2 == 0

		result, err := state.DebugSetupDungeon(location, floor, position, facing, lit)
		require.NoError(t, err)
		assert.Equal(t, references.PlayerLocation{Location: location, Floor: floor, Position: position}, result.After)
		assert.True(t, result.AfterDungeon.Active)
		assert.Equal(t, facing, result.AfterDungeon.Facing)
		assert.Equal(t, references.DungeonLastDirection(facing), result.AfterDungeon.LastDirection)
		assert.Equal(t, lit, result.AfterDungeon.Lit)
		assert.Zero(t, result.AfterDungeon.Revision)
		assert.Equal(t, lit, state.MapState.Lighting.HasAnyLight())
		assert.Equal(t, location, state.activeDungeon.location)
		assert.NotNil(t, state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, floor))

		snapshot, active := state.DungeonStateSnapshot()
		require.True(t, active)
		assert.Equal(t, facing, snapshot.Facing)
		assert.Equal(t, references.DungeonLastDirection(facing), snapshot.LastDirection)
	}
}

func TestDebugSetupDungeonRejectsEveryInvalidBoundAtomically(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		4,
		references.Position{X: 3, Y: 2},
		references.DungeonFacingSouth,
		true,
	)
	require.NoError(t, err)

	tests := []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		position references.Position
		facing   references.DungeonFacing
	}{
		{name: "location below", location: references.SerpentsHold, floor: 0, position: references.Position{}},
		{name: "location above", location: references.CombatRestingShrine, floor: 0, position: references.Position{}},
		{name: "floor below", location: references.Deceit, floor: -1, position: references.Position{}},
		{name: "floor above", location: references.Deceit, floor: 8, position: references.Position{}},
		{name: "x below", location: references.Deceit, floor: 0, position: references.Position{X: -1}},
		{name: "x above", location: references.Deceit, floor: 0, position: references.Position{X: 8}},
		{name: "y below", location: references.Deceit, floor: 0, position: references.Position{Y: -1}},
		{name: "y above", location: references.Deceit, floor: 0, position: references.Position{Y: 8}},
		{name: "facing above", location: references.Deceit, floor: 0, position: references.Position{}, facing: 4},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beforeLocation := state.MapState.PlayerLocation
			beforeDungeon := state.activeDungeon
			beforeSnapshot := beforeDungeon.Snapshot()
			beforeProjection := state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, 4)
			beforeDate := state.DateTime
			beforeElapsed := state.ElapsedMs
			beforeRNGCalls := state.GetRNG().Calls()

			result, setupErr := state.DebugSetupDungeon(
				test.location,
				test.floor,
				test.position,
				test.facing,
				false,
			)

			assert.ErrorIs(t, setupErr, ErrInvalidWorldSetup)
			assert.Equal(t, beforeLocation, result.Before)
			assert.Equal(t, beforeLocation, result.After)
			assert.Equal(t, result.BeforeDungeon, result.AfterDungeon)
			assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
			assert.Same(t, beforeDungeon, state.activeDungeon)
			assert.Equal(t, beforeSnapshot, state.activeDungeon.Snapshot())
			assert.Same(t, beforeProjection, state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, 4))
			assert.True(t, state.MapState.Lighting.HasAnyLight())
			assert.Equal(t, beforeDate, state.DateTime)
			assert.Equal(t, beforeElapsed, state.ElapsedMs)
			assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
		})
	}
}

func TestDebugSetupDungeonConsumesNoTimeOrRandomness(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithRandomSeed(253).
		WithSystemCallbacks().
		Build()
	beforeDate := state.DateTime
	beforeElapsed := state.ElapsedMs
	beforeRNG := state.GetRNG()
	beforeRNGCalls := state.GetRNG().Calls()

	_, err := state.DebugSetupDungeon(
		references.Hythloth,
		7,
		references.Position{X: 7, Y: 0},
		references.DungeonFacingWest,
		true,
	)

	require.NoError(t, err)
	assert.Equal(t, beforeDate, state.DateTime)
	assert.Equal(t, beforeElapsed, state.ElapsedMs)
	assert.Same(t, beforeRNG, state.GetRNG())
	assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
}

func TestDebugSetupDungeonRouteRestartsAuthorityAndPreservesLightTimeAndRandomness(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithRandomSeed(253).
		WithSystemCallbacks().
		Build()
	changedPosition := references.Position{X: 6, Y: 5}
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		0,
		references.Position{X: 1, Y: 1},
		references.DungeonFacingEast,
		false,
	)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(7, changedPosition, func(cell references.DungeonCell) references.DungeonCell {
		return cell ^ 0x01
	})
	require.NoError(t, err)
	before, active := state.DungeonStateSnapshot()
	require.True(t, active)
	state.MapState.Lighting.RestoreTorchMinutes(17)
	state.MagicLightMinutesRemaining = 23
	state.MapState.Lighting.SetMagicLight(true)
	beforeDate := state.DateTime
	beforeElapsed := state.ElapsedMs
	beforeRNG := state.GetRNG()
	beforeRNGCalls := beforeRNG.Calls()

	result, err := state.DebugSetupDungeonRoute(
		references.Deceit,
		7,
		references.Position{X: 7, Y: 7},
		references.DungeonFacingWest,
		references.DungeonArrivedFromBelow,
	)

	require.NoError(t, err)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Deceit,
		Floor:    7,
		Position: references.Position{X: 7, Y: 7},
	}, result.After)
	after, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, references.DungeonFacingWest, after.Facing)
	assert.Equal(t, references.DungeonArrivedFromBelow, after.LastDirection)
	assert.Zero(t, after.Revision)
	assert.NotEqual(t, before.Cells, after.Cells)
	assert.Equal(t, 17, state.MapState.Lighting.TorchMinutesRemaining())
	assert.Equal(t, 23, state.MagicLightMinutesRemaining)
	assert.True(t, state.MapState.Lighting.HasMagicLight())
	assert.Equal(t, beforeDate, state.DateTime)
	assert.Equal(t, beforeElapsed, state.ElapsedMs)
	assert.Same(t, beforeRNG, state.GetRNG())
	assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
}

func TestDebugSetupDungeonRejectsUnavailableSourceBeforeMutation(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		1,
		references.Position{X: 1, Y: 1},
		references.DungeonFacingEast,
		true,
	)
	require.NoError(t, err)
	beforeLocation := state.MapState.PlayerLocation
	beforeDungeon := state.activeDungeon
	beforeProjection := state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, 1)
	gameReferences := *state.GameReferences
	gameReferences.LocationReferences = nil
	state.GameReferences = &gameReferences

	result, err := state.DebugSetupDungeon(
		references.Doom,
		7,
		references.Position{X: 7, Y: 7},
		references.DungeonFacingWest,
		false,
	)

	assert.ErrorIs(t, err, ErrInvalidWorldSetup)
	assert.Equal(t, beforeLocation, result.After)
	assert.Equal(t, result.BeforeDungeon, result.AfterDungeon)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Same(t, beforeDungeon, state.activeDungeon)
	assert.Same(t, beforeProjection, state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, 1))
	assert.True(t, state.MapState.Lighting.HasAnyLight())
}

func TestDebugSetupDungeonReplacesActiveVisitAndProjection(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	position := references.Position{X: 2, Y: 3}
	_, err := state.DebugSetupDungeon(references.Deceit, 0, position, references.DungeonFacingNorth, true)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(0, position, func(references.DungeonCell) references.DungeonCell {
		return references.DungeonCell(0xb0)
	})
	require.NoError(t, err)
	beforeDungeon := state.activeDungeon
	beforeProjection := state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, 0)

	result, err := state.DebugSetupDungeon(references.Doom, 6, position, references.DungeonFacingEast, false)

	require.NoError(t, err)
	assert.Equal(t, references.Doom, result.After.Location)
	assert.NotSame(t, beforeDungeon, state.activeDungeon)
	assert.Equal(t, references.Doom, state.activeDungeon.location)
	assert.NotSame(t, beforeProjection, state.MapState.LayeredMaps.GetLayeredMap(references.DungeonMapType, 0))
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Zero(t, snapshot.Revision)
	expected, ok := state.GameReferences.LocationReferences.GetLocationReference(references.Doom).GetDungeonCell(6, position)
	require.True(t, ok)
	actual, ok := snapshot.Cells.Cell(6, position)
	require.True(t, ok)
	assert.Equal(t, expected, actual)
}

func TestDebugWorldSetupOperationsRejectActiveDungeonCommandWithoutMutation(t *testing.T) {
	operations := []struct {
		name string
		run  func(*GameState) error
	}{
		{name: "set world position", run: func(state *GameState) error {
			_, err := state.DebugSetWorldPosition(references.BritanniaUnderworld, 0, references.Position{X: 80, Y: 100})
			return err
		}},
		{name: "setup dungeon", run: func(state *GameState) error {
			_, err := state.DebugSetupDungeon(
				references.Doom,
				7,
				references.Position{X: 7, Y: 7},
				references.DungeonFacingWest,
				false,
			)
			return err
		}},
		{name: "set dungeon position", run: func(state *GameState) error {
			_, err := state.DebugSetDungeonPosition(3, references.Position{X: 3, Y: 3})
			return err
		}},
		{name: "travel to landmark", run: func(state *GameState) error {
			_, err := state.DebugTravelToLandmark("honesty")
			return err
		}},
	}

	for _, operation := range operations {
		t.Run(operation.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithRandomSeed(253).
				WithSystemCallbacks().
				Build()
			_, err := state.DebugSetupDungeon(
				references.Deceit,
				2,
				references.Position{X: 2, Y: 2},
				references.DungeonFacingNorth,
				true,
			)
			require.NoError(t, err)
			token, _ := state.BeginDungeonCommandCycle()
			beforeLocation := state.MapState.PlayerLocation
			beforeDungeon := state.activeDungeon.Snapshot()
			beforeDate := state.DateTime
			beforeRNGCalls := state.GetRNG().Calls()
			beforeGold := state.PartyState.Inventory.Gold.Get()

			err = operation.run(state)

			assert.ErrorIs(t, err, ErrInvalidWorldSetup)
			assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
			assert.Equal(t, beforeDungeon, state.activeDungeon.Snapshot())
			assert.Equal(t, beforeDate, state.DateTime)
			assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
			assert.Equal(t, beforeGold, state.PartyState.Inventory.Gold.Get())
			assert.False(t, state.IsAtDungeonCommandBoundary())
			state.CompleteDungeonCommandCycle(token, DungeonCommandOnly)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestTransitionToLocationPanicsBeforeAbandoningDungeonCommand(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		2,
		references.Position{X: 2, Y: 2},
		references.DungeonFacingNorth,
		true,
	)
	require.NoError(t, err)
	token, _ := state.BeginDungeonCommandCycle()
	beforeLocation := state.MapState.PlayerLocation
	beforeDungeon := state.activeDungeon
	beforeSnapshot := beforeDungeon.Snapshot()

	assert.PanicsWithValue(t, "cannot leave dungeon during an active dungeon command cycle", func() {
		state.TransitionToLocation(
			references.BritanniaUnderworld,
			0,
			references.Position{X: 80, Y: 100},
			TransitionFullLocation,
			false,
		)
	})
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Same(t, beforeDungeon, state.activeDungeon)
	assert.Equal(t, beforeSnapshot, state.activeDungeon.Snapshot())
	assert.False(t, state.IsAtDungeonCommandBoundary())

	state.CompleteDungeonCommandCycle(token, DungeonCommandOnly)
	state.TransitionToLocation(
		references.BritanniaUnderworld,
		0,
		references.Position{X: 80, Y: 100},
		TransitionFullLocation,
		false,
	)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	_, active := state.DungeonStateSnapshot()
	assert.False(t, active)
}

func TestWorldSetupCannotBypassPendingStartupDungeonRoom(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		7,
		references.Position{X: 7, Y: 7},
		references.DungeonFacingWest,
		true,
	)
	require.NoError(t, err)
	require.True(t, state.HasPendingDungeonRoomEncounter())
	beforeLocation := state.MapState.PlayerLocation

	_, err = state.DebugSetDungeonPosition(7, references.Position{X: 6, Y: 6})

	assert.ErrorIs(t, err, ErrInvalidWorldSetup)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.PanicsWithValue(t, "cannot transition maps while a dungeon room continuation is pending", func() {
		state.TransitionToLocation(
			references.BritanniaUnderworld,
			0,
			references.Position{X: 80, Y: 100},
			TransitionFullLocation,
			false,
		)
	})
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
}

func TestDungeonTransitionsDetachAndReconstructMapUnitAuthority(t *testing.T) {
	tests := []struct {
		name        string
		location    references.Location
		position    references.Position
		prepareFrom func(*GameState)
		assertAfter func(*testing.T, *GameState, any)
	}{
		{
			name: "small map", location: references.Britain, position: references.Position{X: 10, Y: 11},
			prepareFrom: func(*GameState) {},
			assertAfter: func(t *testing.T, state *GameState, original any) {
				t.Helper()
				assert.IsType(t, original, state.CurrentNPCAIController)
			},
		},
		{
			name: "large map", location: references.BritanniaUnderworld, position: references.Position{X: 80, Y: 100},
			prepareFrom: func(state *GameState) {
				state.CurrentNPCAIController = state.GetCurrentLargeMapNPCAIController()
			},
			assertAfter: func(t *testing.T, state *GameState, _ any) {
				t.Helper()
				assert.Same(t, state.GetCurrentLargeMapNPCAIController(), state.CurrentNPCAIController)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(int(test.position.X), int(test.position.Y)).
				WithSystemCallbacks().
				Build()
			test.prepareFrom(state)
			originalController := state.CurrentNPCAIController
			require.NotNil(t, originalController)

			_, err := state.DebugSetupDungeon(
				references.Deceit,
				0,
				references.Position{X: 4, Y: 4},
				references.DungeonFacingEast,
				true,
			)
			require.NoError(t, err)
			assert.Nil(t, state.CurrentNPCAIController)
			assert.Zero(t, state.CurrentMonsterEcosystemSnapshot().CurrentUnitCount)
			removed, supported := state.ClearCurrentMapEnemies()
			assert.Zero(t, removed)
			assert.False(t, supported)

			_, err = state.DebugSetWorldPosition(test.location, 0, test.position)
			require.NoError(t, err)
			test.assertAfter(t, state, originalController)
		})
	}
}

func TestDebugSetupDungeonCapturesCompleteAuthorityReplacement(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	position := references.Position{X: 2, Y: 3}
	_, err := state.DebugSetupDungeon(references.Deceit, 0, position, references.DungeonFacingNorth, true)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(7, references.Position{X: 6, Y: 5}, func(cell references.DungeonCell) references.DungeonCell {
		return cell ^ 0x01
	})
	require.NoError(t, err)
	state.activeDungeon.quicknessSkipNext = true
	state.activeDungeon.wanderer = &DungeonWanderer{
		Monster:          references.MonsterDragon,
		Floor:            4,
		Position:         references.Position{X: 3, Y: 4},
		PreviousPosition: references.Position{X: 2, Y: 4},
		OnCeiling:        true,
	}
	before := state.activeDungeon.Snapshot()

	result, err := state.DebugSetupDungeon(references.Deceit, 0, position, references.DungeonFacingNorth, true)

	require.NoError(t, err)
	assert.True(t, result.BeforeDungeon.Active)
	assert.Equal(t, before, result.BeforeDungeon.DungeonStateSnapshot)
	assert.True(t, result.BeforeDungeon.QuicknessSkipNext)
	assert.Equal(t, before.Wanderer, result.BeforeDungeon.Wanderer)
	assert.True(t, result.AfterDungeon.Active)
	assert.False(t, result.AfterDungeon.QuicknessSkipNext)
	assert.Nil(t, result.AfterDungeon.Wanderer)
	assert.Zero(t, result.AfterDungeon.Revision)
	assert.NotEqual(t, result.BeforeDungeon.Cells, result.AfterDungeon.Cells)
}

func TestDebugSetWorldPositionCombinesLocationFloorAndCoordinateAtomically(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithRandomSeed(253).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Destard,
		3,
		references.Position{X: 4, Y: 5},
		references.DungeonFacingSouth,
		true,
	)
	require.NoError(t, err)
	beforeDate := state.DateTime
	beforeElapsed := state.ElapsedMs
	beforeRNG := state.GetRNG()
	beforeRNGCalls := state.GetRNG().Calls()

	largeResult, err := state.DebugSetWorldPosition(
		references.BritanniaUnderworld,
		-1,
		references.Position{X: 255, Y: 255},
	)
	require.NoError(t, err)
	assert.Equal(t, references.PlayerLocation{
		Location: references.BritanniaUnderworld,
		Floor:    -1,
		Position: references.Position{X: 255, Y: 255},
	}, largeResult.After)
	_, active := state.DungeonStateSnapshot()
	assert.False(t, active)

	smallResult, err := state.DebugSetWorldPosition(
		references.Britain,
		1,
		references.Position{X: 31, Y: 31},
	)
	require.NoError(t, err)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Britain,
		Floor:    1,
		Position: references.Position{X: 31, Y: 31},
	}, smallResult.After)

	_, err = state.DebugSetWorldPosition(
		references.BritanniaUnderworld,
		0,
		references.Position{},
	)
	require.NoError(t, err)
	_, err = state.DebugSetWorldPosition(references.Moonglow, 0, references.Position{})
	require.NoError(t, err)
	assert.Equal(t, beforeDate, state.DateTime)
	assert.Equal(t, beforeElapsed, state.ElapsedMs)
	assert.Same(t, beforeRNG, state.GetRNG())
	assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
}

func TestDebugSetWorldPositionRejectsDestinationBoundsBeforeMutation(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithFloor(1).
		WithPlayerAt(10, 11).
		WithSystemCallbacks().
		Build()

	tests := []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		position references.Position
	}{
		{name: "dungeon", location: references.Deceit, floor: 0, position: references.Position{}},
		{name: "unsupported location", location: references.CombatRestingShrine, floor: 0, position: references.Position{}},
		{name: "large floor below", location: references.BritanniaUnderworld, floor: -2, position: references.Position{}},
		{name: "large floor above", location: references.BritanniaUnderworld, floor: 1, position: references.Position{}},
		{name: "small floor below", location: references.Britain, floor: -1, position: references.Position{}},
		{name: "small floor above", location: references.Britain, floor: 2, position: references.Position{}},
		{name: "large x below", location: references.BritanniaUnderworld, floor: 0, position: references.Position{X: -1}},
		{name: "large x above", location: references.BritanniaUnderworld, floor: 0, position: references.Position{X: 256}},
		{name: "large y below", location: references.BritanniaUnderworld, floor: 0, position: references.Position{Y: -1}},
		{name: "large y above", location: references.BritanniaUnderworld, floor: 0, position: references.Position{Y: 256}},
		{name: "small x above", location: references.Britain, floor: 0, position: references.Position{X: 32}},
		{name: "small y above", location: references.Britain, floor: 0, position: references.Position{Y: 32}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			before := state.MapState.PlayerLocation
			beforeController := state.CurrentNPCAIController
			beforeDate := state.DateTime
			beforeRNGCalls := state.GetRNG().Calls()

			result, setupErr := state.DebugSetWorldPosition(test.location, test.floor, test.position)

			assert.ErrorIs(t, setupErr, ErrInvalidWorldSetup)
			assert.Equal(t, before, result.Before)
			assert.Equal(t, before, result.After)
			assert.Equal(t, before, state.MapState.PlayerLocation)
			assert.Same(t, beforeController, state.CurrentNPCAIController)
			assert.Equal(t, beforeDate, state.DateTime)
			assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
		})
	}
}

func TestDebugSetDungeonPositionCombinesFloorAndCoordinateWithoutReplacingAuthority(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Destard,
		2,
		references.Position{X: 1, Y: 2},
		references.DungeonFacingSouth,
		true,
	)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(6, references.Position{X: 7, Y: 5}, func(references.DungeonCell) references.DungeonCell {
		return references.DungeonCell(0xc8)
	})
	require.NoError(t, err)
	beforeDungeon := state.activeDungeon
	beforeSnapshot := beforeDungeon.Snapshot()
	beforeLight := state.MapState.Lighting.HasAnyLight()
	beforeMagicLight := state.MagicLightMinutesRemaining

	result, err := state.DebugSetDungeonPosition(6, references.Position{X: 7, Y: 5})

	require.NoError(t, err)
	assert.Equal(t, references.PlayerLocation{
		Location: references.Destard,
		Floor:    6,
		Position: references.Position{X: 7, Y: 5},
	}, result.After)
	assert.Same(t, beforeDungeon, state.activeDungeon)
	assert.Equal(t, beforeSnapshot, state.activeDungeon.Snapshot())
	assert.Equal(t, beforeLight, state.MapState.Lighting.HasAnyLight())
	assert.Equal(t, beforeMagicLight, state.MagicLightMinutesRemaining)
}

func TestDebugSetDungeonPositionRejectsInvalidInputWithoutPartialMutation(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	_, err := state.DebugSetupDungeon(
		references.Deceit,
		3,
		references.Position{X: 2, Y: 4},
		references.DungeonFacingWest,
		false,
	)
	require.NoError(t, err)
	beforeLocation := state.MapState.PlayerLocation
	beforeDungeon := state.activeDungeon
	beforeSnapshot := beforeDungeon.Snapshot()

	result, err := state.DebugSetDungeonPosition(8, references.Position{X: 7, Y: 7})

	assert.ErrorIs(t, err, ErrInvalidWorldSetup)
	assert.Equal(t, beforeLocation, result.After)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Same(t, beforeDungeon, state.activeDungeon)
	assert.Equal(t, beforeSnapshot, state.activeDungeon.Snapshot())
}
