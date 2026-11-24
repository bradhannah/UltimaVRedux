package main

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestMoongateRendererDerivesRowsFromGameStateStage(t *testing.T) {
	renderer := NewMoongateRenderer(references.Position{X: 10, Y: 20}, 3)
	renderer.SetSprite(ebiten.NewImage(16, 16))

	_, rows, draw := renderer.GetDrawInfo(game_state.MoongateStageHidden, 0)
	assert.False(t, draw)
	assert.Zero(t, rows)

	_, rows, draw = renderer.GetDrawInfo(game_state.MoongateStage(1), 0)
	assert.True(t, draw)
	assert.Equal(t, 1, rows)

	_, rows, draw = renderer.GetDrawInfo(game_state.MoongateStageFull, 0)
	assert.True(t, draw)
	assert.Equal(t, 16, rows)
}

func TestMoongateStageControlsAmbientVolume(t *testing.T) {
	assert.Zero(t, moongateVolumeForStage(game_state.MoongateStageHidden))
	assert.InDelta(t, 0.5, moongateVolumeForStage(game_state.MoongateStage(8)), 0.0001)
	assert.Equal(t, 1.0, moongateVolumeForStage(game_state.MoongateStageFull))
}

func TestMoongateRendererRetainsExposedSourceUntilPickup(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(223, 133).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 21
	state.UpdateMoongateTransition()
	for range 40 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}
	require.Equal(t, game_state.MoongateStageFull, state.MoongateStage())
	visuals := NewMoongateVisualSystem(state)
	visuals.Update()
	pos := references.Position{X: 224, Y: 133}
	before := visuals.GetActiveMoongateAt(pos)
	require.NotNil(t, before)

	state.GetMoonstoneSystem().MarkMoonstoneFoundAtPosition(0)
	visuals.Update()
	assert.Same(t, before, visuals.GetActiveMoongateAt(pos), "exposure must not hide an active source")
	assert.Contains(t, visuals.GetVisibleMoongatePositions(), pos)

	state.GetMoonstoneSystem().TakeMoonstoneToInventory(0)
	visuals.Update()
	assert.Nil(t, visuals.GetActiveMoongateAt(pos))
	assert.NotContains(t, visuals.GetVisibleMoongatePositions(), pos)
}

func TestMoongateSourcesFollowWorldLayerBeforeAndAfterUpdate(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithFloor(0).
		WithPlayerAt(224, 133).WithSystemCallbacks().Build()
	state.DateTime.Hour = 21
	state.UpdateMoongateTransition()
	for range 40 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}
	require.Equal(t, game_state.MoongateStageFull, state.MoongateStage())
	visuals := NewMoongateVisualSystem(state)
	manager, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	scene := &GameScene{gameState: state, moongateVisualSystem: visuals, audioManager: manager}
	pos := state.MapState.PlayerLocation.Position
	visuals.Update()
	require.NotNil(t, visuals.GetActiveMoongateAt(pos))
	require.True(t, state.IsPlayerOnActiveMoongate())
	require.NotEmpty(t, scene.getMoongateAmbientSources())

	for _, location := range []references.PlayerLocation{
		{Location: references.BritanniaUnderworld, Floor: -1, Position: pos},
		{Location: references.Britain, Floor: 0, Position: pos},
	} {
		state.MapState.PlayerLocation = location
		for _, update := range []bool{false, true} {
			if update {
				visuals.Update()
				assert.Empty(t, visuals.activeRenderers, "wrong-layer resources must be cleared")
			}
			assert.Empty(t, visuals.GetActiveRenderers())
			assert.Nil(t, visuals.GetActiveMoongateAt(pos))
			assert.Empty(t, visuals.GetVisibleMoongatePositions(), "no moongate light sources")
			assert.Empty(t, scene.getMoongateAmbientSources(), "no moongate audio sources")
			assert.False(t, state.IsPlayerOnActiveMoongate())
			assert.False(t, state.IsMoongateActiveAt(pos))
			assert.False(t, state.ReviewMoongateTravel().Success)
		}
		state.MapState.PlayerLocation = references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: pos}
		visuals.Update()
		assert.NotNil(t, visuals.GetActiveMoongateAt(pos))
		assert.Contains(t, visuals.GetVisibleMoongatePositions(), pos)
		assert.NotEmpty(t, scene.getMoongateAmbientSources())
		assert.True(t, state.IsPlayerOnActiveMoongate())
	}
}

func TestRisingMoongateIsMaskedAtGroundLevel(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	const width, height = 400, 208
	manager.PublishLogicalSize(width, height)
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState: state,
		mapImage: ebiten.NewImage(
			sprites.TileSize*xTilesVisibleOnGameScreen,
			sprites.TileSize*yTilesVisibleOnGameScreen,
		),
	}

	previousSprite, previousGlowMask := moongateSprite, moongateGlowMask
	t.Cleanup(func() {
		moongateSprite, moongateGlowMask = previousSprite, previousGlowMask
	})
	moongateSprite = ebiten.NewImage(sprites.TileSize, sprites.TileSize)
	t.Cleanup(moongateSprite.Deallocate)
	moongateSprite.Fill(color.White)
	moongateGlowMask = ebiten.NewImage(sprites.TileSize, sprites.TileSize)
	t.Cleanup(moongateGlowMask.Deallocate)
	moongateGlowMask.Fill(color.White)

	screen := newPresentationRaster(width, height)
	t.Cleanup(screen.Deallocate)
	t.Cleanup(scene.mapImage.Deallocate)
	scene.drawAnimatedMoongate(screen, state.MapState.PlayerLocation.Position, 1, sprites.TileSize/2)

	tileX, tileY := xTilesVisibleOnGameScreen/2, yTilesVisibleOnGameScreen/2
	mapOptions := sprites.GetDrawOptionsFromPercentsForWholeScreen(scene.mapImage, gameScreenPercents)
	visibleX, visibleY := mapOptions.GeoM.Apply(
		float64(tileX*sprites.TileSize+sprites.TileSize/2),
		float64(tileY*sprites.TileSize+sprites.TileSize*3/4),
	)
	belowX, belowY := mapOptions.GeoM.Apply(
		float64(tileX*sprites.TileSize+sprites.TileSize/2),
		float64((tileY+1)*sprites.TileSize+sprites.TileSize/4),
	)

	_, _, _, visibleAlpha := screen.At(int(visibleX), int(visibleY)).RGBA()
	require.NotZero(t, visibleAlpha, "the revealed half of the gate must be visible")
	_, _, _, belowAlpha := screen.At(int(belowX), int(belowY)).RGBA()
	assert.Zero(t, belowAlpha, "the rising gate must not render into the tile below")
}
