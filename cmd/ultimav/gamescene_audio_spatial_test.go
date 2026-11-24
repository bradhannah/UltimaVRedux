package main

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestAmbientWorldAndDungeonSeamsProjectNearestUniqueTiles(t *testing.T) {
	for _, location := range []references.Location{references.BritanniaUnderworld, references.Deceit} {
		t.Run(location.String(), func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(location).WithPlayerAt(0, 0).WithSystemCallbacks().Build()
			scene := &GameScene{gameState: state}
			layered := state.GetLayeredMapByCurrentLocation()
			require.True(t, layered.IsWrappedMap())
			corner := references.Position{X: layered.XMaxTilesPerMap - 1, Y: layered.YMaxTilesPerMap - 1}
			layered.SetTileByLayer(map_state.MapLayer, &corner, indexes.FireLogs)
			tiles := scene.getNearbyTilesForAmbientSound(references.Position{}, 32)
			seen := make(map[references.Position]bool)
			found := false
			for _, tile := range tiles {
				require.False(t, seen[tile.Position], "wrapped scanning duplicated a physical tile")
				seen[tile.Position] = true
				if tile.Position == (references.Position{X: -1, Y: -1}) {
					found = true
					require.Equal(t, indexes.SpriteIndex(indexes.FireLogs), tile.Tile.Index)
				}
			}
			require.True(t, found, "opposite map corner is a nearby sound source")
			if location == references.Deceit {
				require.Len(t, tiles, 64, "a large hearing radius visits each dungeon cell once")
			}
		})
	}
}

func TestAmbientBoundedMapsDoNotWrap(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(0, 0).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state}
	require.False(t, state.GetLayeredMapByCurrentLocation().IsWrappedMap())
	for _, tile := range scene.getNearbyTilesForAmbientSound(references.Position{}, 3) {
		require.GreaterOrEqual(t, int(tile.Position.X), 0)
		require.GreaterOrEqual(t, int(tile.Position.Y), 0)
	}
}

func TestAmbientProjectionKeepsEmitterIdentityAcrossWorldSeam(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state}
	sources := []audio.AmbientSoundSource{{Emitter: "moongate", Position: references.Position{X: 255, Y: 0}}}
	scene.projectAmbientSources(references.Position{}, sources)
	require.Equal(t, references.Position{X: -1, Y: 0}, sources[0].Position)
	identity := sources[0].ID
	scene.projectAmbientSources(references.Position{X: 255, Y: 0}, sources)
	require.Equal(t, references.Position{X: 255, Y: 0}, sources[0].Position)
	require.Equal(t, identity, sources[0].ID)
}
