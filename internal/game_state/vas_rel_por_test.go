package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestVasRelPorDestinationUsesSelectedMovableMoonstone(t *testing.T) {
	gs := &GameState{moonstoneSystem: NewMoonstoneSystem()}
	for phase := datetime.MoonPhaseNew; phase <= datetime.MoonPhaseCrescentWaning; phase++ {
		stone := &gs.moonstoneSystem.Moonstones[phase]
		stone.X = uint8(20 + phase)
		stone.Y = uint8(40 + phase)
		stone.Map = 0
		stone.Level = uint8(phase % 2)

		destination, err := gs.GetMoongateDestinationForPhase(phase)
		require.NoError(t, err)
		assert.Equal(t, stone.X, destination.X)
		assert.Equal(t, stone.Y, destination.Y)
		assert.Equal(t, stone.Level, destination.Level)
		assert.Equal(t, references.BritanniaUnderworld, destination.Location)
	}
}

func TestVasRelPorDestinationPreservesTownMapAndFloor(t *testing.T) {
	gs := &GameState{moonstoneSystem: NewMoonstoneSystem()}
	stone := &gs.moonstoneSystem.Moonstones[datetime.MoonPhaseFull]
	stone.X, stone.Y = 12, 23
	stone.Map = uint8(references.Britain)
	stone.Level = 2

	destination, err := gs.GetMoongateDestinationForPhase(datetime.MoonPhaseFull)

	require.NoError(t, err)
	assert.Equal(t, references.Britain, destination.Location)
	assert.Equal(t, uint8(2), destination.Level)
	assert.Equal(t, uint8(12), destination.X)
	assert.Equal(t, uint8(23), destination.Y)
}

func TestVasRelPorDestinationRejectsUnavailableMoonstoneState(t *testing.T) {
	for _, makeUnavailable := range []func(*MoonstoneLocation){
		func(stone *MoonstoneLocation) { stone.State = MoonstoneStateInInventory },
		func(stone *MoonstoneLocation) { stone.Map = 0xFF },
	} {
		gs := &GameState{moonstoneSystem: NewMoonstoneSystem()}
		makeUnavailable(&gs.moonstoneSystem.Moonstones[datetime.MoonPhaseFirstQuarter])

		_, err := gs.GetMoongateDestinationForPhase(datetime.MoonPhaseFirstQuarter)

		require.Error(t, err)
	}
}

func TestVasRelPorUnavailableDestinationPreservesResourcesAndTime(t *testing.T) {
	gs, mock := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	caster := gs.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	gs.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
	stone := gs.GetMoonstoneSystem().GetMoonstoneByIndex(int(datetime.MoonPhaseGibbousWaning))
	stone.Map = 0xFF
	stone.State = MoonstoneStateInInventory
	mock.Reset()

	prerequisite := gs.ValidateVasRelPorPrerequisites(CastRequest{Spell: references.VasRelPor, CasterIdx: 0})
	require.True(t, prerequisite.Valid)
	_, err := gs.GetMoongateDestinationForPhase(datetime.MoonPhaseGibbousWaning)

	require.Error(t, err)
	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.Spells.Get(references.VasRelPor))
	assert.Empty(t, mock.TimeAdvanced())
}

func TestVasRelPorSelectedPhaseIgnoresAstronomyAndShrineOverride(t *testing.T) {
	gs := &GameState{
		DateTime:        datetime.UltimaDate{Day: 1, Hour: 0, Minute: 5},
		moonstoneSystem: NewMoonstoneSystem(),
	}
	selected := datetime.MoonPhaseLastQuarter
	want := gs.moonstoneSystem.Moonstones[selected]

	destination, err := gs.GetMoongateDestinationForPhase(selected)

	require.NoError(t, err)
	assert.Equal(t, want.X, destination.X)
	assert.Equal(t, want.Y, destination.Y)
}

func TestVasRelPorFrigatePrerequisitePreservesResources(t *testing.T) {
	gs, mock := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	caster := gs.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(8)
	gs.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
	gs.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle, gs.MapState.PlayerLocation.Position, gs.MapState.PlayerLocation.Floor,
	)
	mock.Reset()

	result := gs.ValidateVasRelPorPrerequisites(CastRequest{Spell: references.VasRelPor, CasterIdx: 0})

	require.False(t, result.Valid)
	assert.Equal(t, VasRelPorShipMessage, result.ErrorMsg)
	assert.Equal(t, byte(8), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.Spells.Get(references.VasRelPor))
	assert.Empty(t, mock.TimeAdvanced())
}

func TestVasRelPorPrerequisiteAllowsNonFrigateTravel(t *testing.T) {
	for _, vehicle := range []references.VehicleType{
		references.NoPartyVehicle,
		references.CarpetVehicle,
		references.HorseVehicle,
	} {
		gs, _ := NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithSystemCallbacks().
			Build()
		caster := gs.PartyState.GetCharacter(0)
		caster.Class = party_state.Wizard
		caster.Level = 8
		caster.SetMp(8)
		gs.PartyState.Inventory.Spells.Set(references.VasRelPor, 1)
		gs.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
			vehicle, gs.MapState.PlayerLocation.Position, gs.MapState.PlayerLocation.Floor,
		)

		result := gs.ValidateVasRelPorPrerequisites(CastRequest{Spell: references.VasRelPor, CasterIdx: 0})

		assert.True(t, result.Valid, "vehicle %d", vehicle)
	}
}

func TestTeleportationSequenceMutatesCapturedDestinationWithoutAdvancingTime(t *testing.T) {
	gs, mock := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(3, 4).
		WithSystemCallbacks().
		Build()
	captured := MoongateDestination{
		X: 9, Y: 11, Map: uint8(references.Britain), Level: 2, Location: references.Britain,
	}
	wantDestination := captured
	source := gs.MapState.PlayerLocation
	beforeTime := gs.DateTime
	gs.moongateStage = 7
	var order []string
	messageWritten := false
	gs.SystemCallbacks.Visual.ResetTeleportationSourceRenderer = func() {
		assert.Equal(t, source, gs.MapState.PlayerLocation)
		order = append(order, "source renderer reset")
	}
	gs.SystemCallbacks.Visual.StabilizeTeleportationDestinationRenderer = func(destination MoongateDestination) {
		assert.Equal(t, references.Britain, gs.MapState.PlayerLocation.Location)
		assert.Equal(t, references.FloorNumber(2), gs.MapState.PlayerLocation.Floor)
		assert.Equal(t, references.Position{X: 9, Y: 11}, gs.MapState.PlayerLocation.Position)
		assert.Equal(t, wantDestination, destination)
		order = append(order, "destination renderer stabilized")
	}
	gs.SystemCallbacks.Screen.UpdateDisplay = func() {
		order = append(order, "display updated")
	}
	gs.SystemCallbacks.Message.AddRowStr = func(message string) {
		assert.Equal(t, "The moongate whisks thee away!", message)
		messageWritten = true
		order = append(order, "message written")
	}
	gs.SystemCallbacks.Visual.SetAvatarDissolveLevel = func(level float64) {
		if messageWritten && level == 1 {
			order = append(order, "avatar stabilized")
		}
	}
	gs.SystemCallbacks.Visual.SetMapFadeLevel = func(level float64) {
		if messageWritten && level == 1 {
			order = append(order, "map stabilized")
		}
	}
	mock.Reset()

	sequence := gs.BuildTeleportationSequence(captured)
	captured = MoongateDestination{X: 99, Y: 98, Location: references.Minoc}
	gs.EventQueueManager.Workflow("captured_teleport_test").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(sequence).
		Finally(func() {}).
		Start()
	for range 8 {
		gs.ElapsedMs += 1000
		gs.EventQueueManager.Update()
	}

	assert.Equal(t, references.Britain, gs.MapState.PlayerLocation.Location)
	assert.Equal(t, references.FloorNumber(2), gs.MapState.PlayerLocation.Floor)
	assert.Equal(t, references.Position{X: 9, Y: 11}, gs.MapState.PlayerLocation.Position)
	assert.Equal(t, beforeTime, gs.DateTime, "teleportation must not own gameplay time")
	assert.Equal(t, MoongateStage(7), gs.MoongateStage(), "generic teleportation must not alter moongates")
	assert.Equal(t, []string{
		"source renderer reset",
		"destination renderer stabilized",
		"display updated",
		"message written",
		"avatar stabilized",
		"map stabilized",
		"map stabilized", // Fade-in admission presents its initial (fully black) frame.
	}, order)
}

func TestTeleportationSequenceWritesDestinationMessageVariants(t *testing.T) {
	tests := []struct {
		name        string
		destination MoongateDestination
		message     string
	}{
		{
			name: "Skara Brae",
			destination: MoongateDestination{
				X: 9, Y: 11, Map: uint8(references.SkaraBrae), Location: references.SkaraBrae,
			},
			message: "The moongate whisks thee to the ruins of Skara Brae!",
		},
		{
			name: "default",
			destination: MoongateDestination{
				X: 9, Y: 11, Map: uint8(references.Britain), Location: references.Britain,
			},
			message: "The moongate whisks thee away!",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, mock := NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithSystemCallbacks().
				Build()
			gs.EventQueueManager.Workflow("teleport_message_test").
				Priority(event_queue.PriorityHigh).
				Sequential().
				First(func() {}).
				ThenSequence(gs.BuildTeleportationSequence(test.destination)).
				Finally(func() {}).
				Start()

			for gs.EventQueueManager.HasActiveWorkflows() {
				gs.ElapsedMs += 1000
				gs.EventQueueManager.Update()
			}

			assert.Contains(t, mock.Messages, test.message)
		})
	}
}
