package main

import (
	"sync"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

var sharedPresentationTestSpriteSheet = sync.OnceValue(sprites.NewSpriteSheet)

func TestDissolvePresentationPreservesGameplayRNGSnapshot(t *testing.T) {
	scene, _ := newShardWorkflowTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	before, err := scene.gameState.CaptureSaveSnapshot()
	require.NoError(t, err)

	scene.initializeWandererDissolveEffect()
	scene.setWandererDissolveLevel(0.5)
	scene.cleanupWandererDissolveEffect()
	scene.initializeAvatarDissolveEffect()
	scene.setAvatarDissolveLevel(0.5)
	scene.cleanupAvatarDissolveEffect()

	options := TileDissolveOptions{
		Position: references.Position{X: 8, Y: 6}, SpriteIndex: indexes.Apparation1,
		Mode: visual.DissolveModeReveal, OutlineColor: sprites.WhiteOutline,
		PositionMode: PositionModeFixedGrid, RenderLayer: RenderLayerOverFade,
	}
	dissolveID := scene.StartTileDissolveWithOptions(options)
	scene.UpdateTileDissolveProgress(dissolveID, 0.5)
	scene.StopTileDissolve(dissolveID)
	skippedID := scene.StartTileDissolveWithOptions(options)
	scene.StopTileDissolve(skippedID)

	after, err := scene.gameState.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, before.Random, after.Random)
}

func TestTileDissolveOptionsProduceStablePresentation(t *testing.T) {
	scene, _ := newShardWorkflowTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	options := TileDissolveOptions{
		Position: references.Position{X: -2, Y: 6}, SpriteIndex: indexes.Apparation1,
		Mode: visual.DissolveModeReveal, OutlineColor: sprites.WhiteOutline,
		PositionMode: PositionModeFixedGrid, RenderLayer: RenderLayerOverFade,
	}

	firstID := scene.StartTileDissolveWithOptions(options)
	secondID := scene.StartTileDissolveWithOptions(options)
	scene.UpdateTileDissolveProgress(firstID, 0.5)
	scene.UpdateTileDissolveProgress(secondID, 0.5)
	require.Len(t, scene.activeTileDissolves, 2)

	firstMask := dissolveMaskPixels(scene.activeTileDissolves[0].Effect.effect.GetMask())
	secondMask := dissolveMaskPixels(scene.activeTileDissolves[1].Effect.effect.GetMask())
	changedOptions := options
	changedOptions.Position.X++
	assert.NotEqual(t, tileDissolveSeed(options), tileDissolveSeed(changedOptions))
	assert.Equal(t, firstMask, secondMask)
}

func TestDeathEarthquakePreservesGameplayRNG(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(8675309).
		Build()
	scene := &GameScene{gameState: state}
	beforeCalls := state.GetRNG().Calls()

	const workflowName = "test_death_earthquake"
	state.EventQueueManager.Workflow(workflowName).
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenDeathEarthquake()).
		Start()
	for range 100 {
		state.EventQueueManager.Update()
		state.ElapsedMs += 20
		if !state.EventQueueManager.HasWorkflow(workflowName) {
			break
		}
	}

	require.False(t, state.EventQueueManager.HasWorkflow(workflowName))
	assert.Equal(t, beforeCalls, state.GetRNG().Calls())
	require.NotEmpty(t, callbacks.ScreenShakeOffsets)
	assert.Equal(t, 0.0, callbacks.ScreenShakeOffsets[len(callbacks.ScreenShakeOffsets)-1].X)
	assert.Equal(t, 0.0, callbacks.ScreenShakeOffsets[len(callbacks.ScreenShakeOffsets)-1].Y)
}

func dissolveMaskPixels(mask *ebiten.Image) []byte {
	pixels := make([]byte, mask.Bounds().Dx()*mask.Bounds().Dy()*4)
	mask.ReadPixels(pixels)

	return pixels
}
