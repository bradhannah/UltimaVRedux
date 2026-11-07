package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// The Traveller's Map night looks, chosen from rendered variants (docs/ui/WIDGET_COMPONENTS.md).
// The map's torch is softer than the game view's torch tint, which would turn
// the whole chart orange, and its pool leaves the map's border readable.
const (
	atlasTorchR, atlasTorchG, atlasTorchB             = 1.10, 0.92, 0.66
	atlasTorchEdgeR, atlasTorchEdgeG, atlasTorchEdgeB = 0.02, 0.02, 0.04
	atlasTorchEdgeShade                               = 0.55
	atlasTorchPoolCore                                = 0.25
	atlasTorchFlicker                                 = 0.06
	atlasMagicR, atlasMagicG, atlasMagicB             = 0.95, 0.97, 1.02
)

// atlasLighting is the light the map terrain is drawn under for this opening.
// Dungeons are dark at every hour, like the Underworld; otherwise the map
// follows the game view's dawn and dusk curve.
func (g *GameScene) atlasLighting() widgets.AtlasLighting {
	darkness := g.gameState.AmbientTintDarkness()
	if g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType {
		darkness = 1
	}

	return atlasLightingFor(darkness, g.gameState.GetPlayerLightType())
}

// atlasLightingFor blends from the charted map by day (darkness 0) to its
// night look under the carried light (darkness 1).
func atlasLightingFor(darkness float32, light map_state.PlayerLightType) widgets.AtlasLighting {
	toNight := func(night float32) float32 { return 1 + (night-1)*darkness }
	lighting := widgets.AtlasDaylight()
	switch light {
	case map_state.PlayerLightTorch:
		lighting.Terrain = [3]float32{toNight(atlasTorchR), toNight(atlasTorchG), toNight(atlasTorchB)}
		lighting.EdgeShade = atlasTorchEdgeShade * darkness
		lighting.EdgeColour = [3]float32{atlasTorchEdgeR, atlasTorchEdgeG, atlasTorchEdgeB}
		lighting.PoolCore = atlasTorchPoolCore
		lighting.Flicker = atlasTorchFlicker * float64(darkness)
	case map_state.PlayerLightInLor, map_state.PlayerLightVasLor:
		lighting.Terrain = [3]float32{toNight(atlasMagicR), toNight(atlasMagicG), toNight(atlasMagicB)}
	case map_state.PlayerLightNone, map_state.PlayerLightAmbient:
		level := map_state.UnlitAmbientBrightness(darkness)
		lighting.Terrain = [3]float32{level, level, level}
	}

	return lighting
}
