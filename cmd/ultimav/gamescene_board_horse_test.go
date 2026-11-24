package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSmallMapBoardOwnedHorseCompletesOneCycleWithoutDialog(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithFloor(references.MainFloor).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	position := state.MapState.PlayerLocation.Position
	units := state.CurrentNPCAIController.GetNpcs()
	units.ClearAllMapUnits()
	horse := map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, position, references.MainFloor)
	horse.GetVehicleDetails().IsOwnedByNPC = true
	stored := units.InsertVehicle(*horse)
	require.NotNil(t, stored)
	hitch := position.Right()
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &hitch, indexes.HitchingPost)
	before := stored.GetVehicleDetails().Snapshot()
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)

	scene.smallMapInputHandler(ebiten.KeyB)

	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, "\"Nay!\"", callbacks.CurrentRowContent)
	assert.False(t, callbacks.HorseRefusalShown)
	assert.Equal(t, references.NoPartyVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)
	assert.Same(t, stored, units.GetVehicleAtPositionAndFloorOrNil(position, references.MainFloor))
	assert.Equal(t, before, stored.GetVehicleDetails().Snapshot())
}
