package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestActionHoleUpLargeMap_NotOnFoot(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()

	skiff := map_units.NewNPCFriendlyVehiceNewRef(
		references.SkiffVehicle,
		references.Position{X: 50, Y: 50},
		0,
	)
	gs.PartyVehicle = *skiff

	assert.False(t, gs.ActionHoleUpLargeMap())
	assert.Contains(t, mockCallbacks.Messages, "On foot!")
}

func TestMountedOutdoorCampingEntryAndWaterRefusal(t *testing.T) {
	for _, vehicle := range []references.VehicleType{references.HorseVehicle, references.CarpetVehicle} {
		for _, tile := range []indexes.SpriteIndex{indexes.Grass, indexes.Water1, indexes.Water2, indexes.WaterCoast} {
			gs, callbacks := newCampingAmbushTestState(t)
			position := gs.MapState.PlayerLocation.Position
			gs.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, position, 0)
			gs.PartyVehicle.GetVehicleDetails().SetPartyVehicleDirection(references.Left)
			gs.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &position, tile)
			before := gs.PartyVehicle.GetVehicleDetails().Snapshot()
			rngBefore := gs.GetRNG().Calls()
			requests := 0
			gs.SystemCallbacks.Flow.Rest.RequestCampDuration = func() { requests++ }
			want := tile == indexes.Grass
			assert.Equal(t, want, gs.ActionHoleUpLargeMap())
			if want {
				assert.Equal(t, 1, requests)
			} else {
				assert.Zero(t, requests)
				assert.Contains(t, callbacks.Messages, "On land or ship!")
			}
			assert.Equal(t, before, gs.PartyVehicle.GetVehicleDetails().Snapshot())
			assert.Equal(t, rngBefore, gs.GetRNG().Calls())
		}
	}
}

func TestMountedOutdoorCampingPreservesVehicleAfterCompletionOrInterruption(t *testing.T) {
	for _, vehicle := range []references.VehicleType{references.HorseVehicle, references.CarpetVehicle} {
		for _, ambush := range []bool{false, true} {
			state, _ := newCampingAmbushTestState(t)
			position := state.MapState.PlayerLocation.Position
			state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, position, 0)
			state.PartyVehicle.GetVehicleDetails().SetPartyVehicleDirection(references.Left)
			before := state.PartyVehicle.GetVehicleDetails().Snapshot()
			carpets := state.PartyState.Inventory.QuestItems.Get(references.Carpet)
			location := state.MapState.PlayerLocation
			installAutomaticAnyKey(state)
			require.True(t, state.ActionHoleUpLargeMapWithDebug(2, NoGuardAssigned, false, ambush))
			runCampingWorkflowToCompletion(t, state)
			assert.Equal(t, location, state.MapState.PlayerLocation)
			if vehicle == references.CarpetVehicle {
				assert.Equal(t, references.NoPartyVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)
				assert.Equal(t, carpets+1, state.PartyState.Inventory.QuestItems.Get(references.Carpet))
			} else {
				assert.Equal(t, before, state.PartyVehicle.GetVehicleDetails().Snapshot())
			}
		}
	}
}

func TestActionHoleUpLargeMap_OnFootRequestsDuration(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()

	gs.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	position := &gs.MapState.PlayerLocation.Position
	gs.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, position, indexes.Grass)
	requested := false
	gs.SystemCallbacks.Flow.Rest.RequestCampDuration = func() {
		requested = true
	}

	assert.True(t, gs.ActionHoleUpLargeMap())
	assert.True(t, requested)
}

func TestCampingReportsAMissingCampMapInsteadOfCrashing(t *testing.T) {
	state, callbacks := newCampingAmbushTestState(t)
	state.GameConfig.SavedConfigData.DataFilePath = t.TempDir()
	location := state.MapState.PlayerLocation

	require.False(t, state.ActionHoleUpLargeMapWithDebug(2, NoGuardAssigned, false, false))
	assert.True(t, callbacks.MessagesContain("Camping could not begin."))
	assert.Equal(t, location, state.MapState.PlayerLocation)
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}
