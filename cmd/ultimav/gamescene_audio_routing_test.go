package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestAmbientTerrainSourceIsNotHiddenByAnOccupant(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	scene := &GameScene{gameState: state}
	source := references.Position{X: 11, Y: 10}
	terrain := state.GetCurrentLayeredMap()
	terrain.SetTileByLayer(map_state.MapOverrideLayer, &source, indexes.Fountain1)
	terrain.SetTileByLayer(map_state.MapUnitLayer, &source, indexes.Guard1)
	tiles := scene.getNearbyTilesForAmbientSound(references.Position{X: 10, Y: 10}, 2)
	found := false
	for _, tile := range tiles {
		if tile.Position == source {
			found = true
			require.Equal(t, indexes.SpriteIndex(indexes.Fountain1), tile.Tile.Index)
		}
	}
	require.True(t, found)
}

func TestAmbientScanReusesStorageButRefreshesStationaryTerrain(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	scene := &GameScene{gameState: state}
	center := references.Position{X: 10, Y: 10}
	initial := scene.getNearbyTilesForAmbientSound(center, 3)
	require.NotEmpty(t, initial)
	storage := &initial[0]
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &center, indexes.Fountain1)
	updated := scene.getNearbyTilesForAmbientSound(center, 3)
	require.Same(t, storage, &updated[0])
	found := false
	for _, tile := range updated {
		if tile.Position == center {
			found = true
			require.Equal(t, indexes.SpriteIndex(indexes.Fountain1), tile.Tile.Index)
		}
	}
	require.True(t, found)
	shrunk := scene.getNearbyTilesForAmbientSound(center, 0)
	require.Len(t, shrunk, 1)
	require.Equal(t, center, shrunk[0].Position)
	for _, cleared := range scene.ambientTiles[1:cap(scene.ambientTiles)] {
		require.Nil(t, cleared.Tile, "shorter scans must not retain old pointer-bearing entries")
	}
}

func TestDungeonBackgroundOwnerSurvivesTacticalMapAndExit(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	scene := &GameScene{gameState: state}
	state.MapState.PlayerLocation.Location = references.Deceit
	require.Equal(t, references.Deceit, scene.dungeonAudioLocation())
	state.CombatScene = &game_state.CombatState{
		PreCombatPlayerLocation: references.PlayerLocation{Location: references.Deceit, Floor: 3},
	}
	state.MapState.PlayerLocation.Location = references.CombatLocation
	require.Equal(t, references.Deceit, scene.dungeonAudioLocation())
	state.CombatScene = nil
	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	require.Equal(t, references.BritanniaUnderworld, scene.dungeonAudioLocation())
}

func TestNavalAudioSituationTracksVesselAndSailsNotAutomaticMovement(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	scene := &GameScene{gameState: state}
	vessel, raised := scene.navalAudioSituation()
	require.Empty(t, vessel)
	require.False(t, raised)
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.SkiffVehicle, references.Position{}, references.FloorNumber(references.OVERWORLD))
	vessel, raised = scene.navalAudioSituation()
	require.Equal(t, "skiff", vessel)
	require.False(t, raised)
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle, references.Position{}, references.FloorNumber(references.OVERWORLD))
	vessel, raised = scene.navalAudioSituation()
	require.Equal(t, "frigate", vessel)
	require.False(t, raised)
	state.PartyVehicle.GetVehicleDetails().SetSailState(map_units.SailRaised)
	vessel, raised = scene.navalAudioSituation()
	require.Equal(t, "frigate", vessel)
	require.True(t, raised, "raised sails own music even when automatic sailing is stopped")
	state.PartyVehicle.GetVehicleDetails().SetSailState(map_units.SailFurled)
	vessel, raised = scene.navalAudioSituation()
	require.Equal(t, "frigate", vessel, "furled sails do not stop onboard ambience")
	require.False(t, raised)
}
