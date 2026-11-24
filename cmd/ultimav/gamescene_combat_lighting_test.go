package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

// TestCombatVisibility_IgnoresExplorationLighting prevents dungeon combatants
// spawning outside a fixed center light pool from being hidden.
func TestCombatVisibility_IgnoresExplorationLighting(t *testing.T) {
	gs, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	err := gs.TransitionToCombatMap(6, references.DungeonCombatTerritory, game_state.CombatEntrySouth)
	require.NoError(t, err)
	layer := gs.GetCurrentLayeredMap()
	partyPos := gs.CombatScene.GetPartyMemberPosition(gs.CombatScene.GetActiveCombatantIndex())
	layer.SetVisible(false, &partyPos)
	g := &GameScene{gameState: gs}

	assert.True(t, g.isPositionVisible(layer, &partyPos),
		"combat must display the battlefield even when exploration LOS marks the spawn dark")
}

func TestCombatVisibility_DisablesLightTinting(t *testing.T) {
	gs, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	err := gs.TransitionToCombatMap(6, references.DungeonCombatTerritory, game_state.CombatEntrySouth)
	require.NoError(t, err)
	gs.LightSourceTintingState = map_state.NewLightSourceTintingState()
	gs.LightSourceTintingState.UpdatePlayerLight(
		references.Position{X: 5, Y: 5},
		map_state.PlayerLightTorch,
	)
	g := &GameScene{gameState: gs}
	drawOptions := &ebiten.DrawImageOptions{}

	assert.False(t, g.applyLightTinting(
		references.Position{X: 5, Y: 2},
		drawOptions,
		0,
	), "combat must not tint or darken tiles around a fixed player-light origin")
}

func TestDuskAmbientTintDoesNotChangeVisibilityLight(t *testing.T) {
	gs, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	gs.DateTime.Hour = 19
	gs.DateTime.Minute = 45
	gs.LightSourceTintingState = map_state.NewLightSourceTintingState()
	assert.Equal(t, map_state.PlayerLightNone, gs.GetPlayerLightType(),
		"cosmetic dusk tint must not grant ambient visibility before night")

	g := &GameScene{gameState: gs}
	drawOptions := &ebiten.DrawImageOptions{}
	assert.True(t, g.applyLightTinting(references.Position{}, drawOptions, 0))
	want := float32(1 + (map_state.UnlitDarkeningFactor-1)*0.5)
	assert.InDelta(t, want, drawOptions.ColorScale.R(), 0.0001)
	assert.InDelta(t, want, drawOptions.ColorScale.G(), 0.0001)
	assert.InDelta(t, want, drawOptions.ColorScale.B(), 0.0001)
}
