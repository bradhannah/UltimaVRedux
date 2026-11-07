package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/stretchr/testify/require"
)

func TestAtlasLightingFollowsDarknessAndCarriedLight(t *testing.T) {
	requireGrey := func(level float32, terrain [3]float32, message string) {
		t.Helper()
		for _, channel := range terrain {
			require.InDelta(t, level, channel, 1e-6, message)
		}
	}
	for _, light := range []map_state.PlayerLightType{map_state.PlayerLightNone, map_state.PlayerLightTorch, map_state.PlayerLightInLor} {
		day := atlasLightingFor(0, light)
		require.Equal(t, widgets.AtlasDaylight().Terrain, day.Terrain, "the day chart is unchanged")
		require.Zero(t, day.EdgeShade, "no pool by day")
		require.Zero(t, day.Flicker, "no flicker by day")
	}
	requireGrey(map_state.UnlitDarkeningFactor, atlasLightingFor(1, map_state.PlayerLightAmbient).Terrain,
		"unlit night matches the game view")
	requireGrey(0.70, atlasLightingFor(0.5, map_state.PlayerLightNone).Terrain, "dusk is halfway, on the game view's curve")

	torch := atlasLightingFor(1, map_state.PlayerLightTorch)
	require.Equal(t, [3]float32{atlasTorchR, atlasTorchG, atlasTorchB}, torch.Terrain)
	require.InDelta(t, atlasTorchEdgeShade, torch.EdgeShade, 1e-6, "radial pool")
	require.Less(t, torch.EdgeShade, float32(1), "the map's border stays visible")
	require.InDelta(t, atlasTorchFlicker, torch.Flicker, 1e-9)
	require.Equal(t, [3]float32{atlasMagicR, atlasMagicG, atlasMagicB}, atlasLightingFor(1, map_state.PlayerLightInLor).Terrain)
	require.Zero(t, atlasLightingFor(1, map_state.PlayerLightVasLor).Flicker, "magic light is steady")

	dungeon, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.Deceit).WithSystemCallbacks().Build()
	dungeon.DateTime.Hour = 12
	scene := &GameScene{gameState: dungeon}
	requireGrey(map_state.UnlitDarkeningFactor, scene.atlasLighting().Terrain, "dungeons are dark at noon")
	dungeon.EventQueueManager.Shutdown()
}
