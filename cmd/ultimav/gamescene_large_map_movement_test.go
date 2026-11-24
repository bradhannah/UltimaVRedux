package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestMovementPromptUsesVehicleConfigurationWithoutAutomaticSailing(t *testing.T) {
	for _, test := range []struct {
		vehicle references.VehicleType
		sails   map_units.SailState
		prompt  string
	}{
		{references.NoPartyVehicle, map_units.SailFurled, "East"},
		{references.HorseVehicle, map_units.SailFurled, "Ride East"},
		{references.CarpetVehicle, map_units.SailFurled, "Fly East"},
		{references.SkiffVehicle, map_units.SailFurled, "Row East"},
		{references.FrigateVehicle, map_units.SailFurled, "Row East"},
		{references.FrigateVehicle, map_units.SailRaised, "Sail East"},
	} {
		t.Run(test.prompt+test.vehicle.GetLowerCaseName(), func(t *testing.T) {
			fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
			vehicle := fixture.state.PartyVehicle.GetVehicleDetails()
			vehicle.VehicleType = test.vehicle
			vehicle.SetSailState(test.sails)
			vehicle.SetPartyVehicleDirection(references.Right)
			assert.False(t, fixture.state.IsSailing())
			fixture.scene.handleMovementWithBlockedFeedback("East", ebiten.KeyRight, true)
			assert.Contains(t, fixture.callbacks.CommandPrompts, test.prompt)
		})
	}
}

// TestFrigateHeadingChangeFinishesExactlyOneTurn prevents handleMovement and
// largeMapInputHandler from both processing the same heading-change turn.
func TestFrigateHeadingChangeFinishesExactlyOneTurn(t *testing.T) {
	gs, mockCallbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithRandomSeed(179).
		WithSystemCallbacks().
		Build()
	gs.DebugOptions.MonsterGen = false
	scene := &GameScene{
		gameState:            gs,
		keyboard:             input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false }),
		output:               text.NewOutput(nil, 1, 10, 80),
		dialogStack:          widgets.DialogStack{},
		moongateVisualSystem: NewMoongateVisualSystem(gs),
	}
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetPartyVehicleDirection(references.Right)
	gs.PartyVehicle = *frigate
	gs.DateTime.Hour = 12
	gs.DateTime.Minute = 0
	gs.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3}
	before := gs.MapState.PlayerLocation.Position
	mockCallbacks.Reset()

	scene.largeMapInputHandler(ebiten.KeyUp)

	assert.Equal(t, references.Up, gs.PartyVehicle.GetVehicleDetails().CurrentDirection())
	assert.Equal(t, byte(game_state.RowingSpeed), gs.DateTime.Minute)
	assert.Equal(t, before, gs.MapState.PlayerLocation.Position)
	assert.Equal(t, 2, gs.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, []int{game_state.RowingSpeed}, mockCallbacks.TimeAdvanced())
	assert.Contains(t, mockCallbacks.CommandPrompts, "Row North")
}

func TestHMSCapeProductionFurledFrigateMovementStillAdvancesTwoMinutes(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithRandomSeed(387).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.FreeMove = true
	state.DebugOptions.MonsterGen = false
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		state.MapState.PlayerLocation.Position,
		state.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetPartyVehicleDirection(references.Right)
	state.PartyVehicle = *frigate
	state.SetHMSCapeActivated(true)
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	before := state.MapState.PlayerLocation.Position

	scene.largeMapInputHandler(ebiten.KeyRight)

	assert.NotEqual(t, before, state.MapState.PlayerLocation.Position)
	assert.Equal(t, []int{game_state.RowingSpeed}, callbacks.TimeAdvanced())
	assert.Equal(t, uint32(1), state.DateTime.Turn)
	assert.Contains(t, callbacks.CommandPrompts, "Row East")
}

func TestRoughSeasProductionMovementDoesNotAddTimeOrProvisionStage(t *testing.T) {
	for _, test := range []struct {
		name        string
		vehicleType references.VehicleType
	}{
		{name: "skiff", vehicleType: references.SkiffVehicle},
		{name: "carpet", vehicleType: references.CarpetVehicle},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(50, 50).
				WithRandomSeed(543).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			state.DateTime.Hour = 5
			state.DateTime.Minute = 57
			vehicle := map_units.NewNPCFriendlyVehiceNewRef(
				test.vehicleType,
				state.MapState.PlayerLocation.Position,
				state.MapState.PlayerLocation.Floor,
			)
			state.PartyVehicle = *vehicle
			destination := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, destination, indexes.Water1)
			foodBefore := state.PartyState.Inventory.Provisions.Food.Get()
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)

			scene.largeMapInputHandler(ebiten.KeyRight)
			assert.Contains(t, callbacks.CommandPrompts, test.vehicleType.GetMovementPrefix()+"East")

			assert.Equal(t, byte(5), state.DateTime.Hour)
			assert.Equal(t, byte(59), state.DateTime.Minute)
			assert.Equal(t, []int{2}, callbacks.TimeAdvanced())
			assert.True(t, state.EventQueueManager.HasWorkflow("skiff_deep_water"))
			for range 10 {
				state.ElapsedMs += 1000
				state.EventQueueManager.Update()
				if !state.EventQueueManager.HasActiveWorkflows() {
					break
				}
			}

			assert.False(t, state.EventQueueManager.HasActiveWorkflows())
			assert.Equal(t, byte(5), state.DateTime.Hour)
			assert.Equal(t, byte(59), state.DateTime.Minute)
			assert.Equal(t, foodBefore, state.PartyState.Inventory.Provisions.Food.Get())
			assert.Equal(t, []int{2}, callbacks.TimeAdvanced())
		})
	}
}
