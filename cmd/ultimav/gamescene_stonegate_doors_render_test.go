package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestStonegateLavaRevealRendersFormerDoorAsLava(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Stonegate).
		WithPlayerAt(15, 17).WithRandomSeed(701).WithSystemCallbacks().Build()
	scene := newSurfaceCompletionTestScene(state)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.unscaledMapImage = ebiten.NewImage(18, 18)
	t.Cleanup(scene.unscaledMapImage.Deallocate)
	scene.xRayVisionActive = true
	state.SystemCallbacks.Flow.Consequences.TriggerStonegateTrap = scene.StartStonegateTrapWorkflow
	door := state.MapState.PlayerLocation.Position.Left()
	layer := state.GetCurrentLayeredMap()
	layer.SetTileByLayer(map_state.MapLayer, &door, indexes.RegularDoorView)
	require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Left))
	state.MapState.CloseOpenDoor()
	scene.smallMapInputHandler(ebiten.KeyUp)
	require.True(t, state.EventQueueManager.HasWorkflow("stonegate_trap_workflow"))
	require.True(t, state.PartyState.IsAllPartyDead())
	scene.refreshStaticMapTiles(&door, layer, &ebiten.DrawImageOptions{})
	expected := ebiten.NewImage(18, 18)
	t.Cleanup(expected.Deallocate)
	options := &ebiten.DrawImageOptions{}
	scene.applyLightTinting(door, options, scene.getElapsedMs())
	expected.DrawImage(scene.spriteSheet.GetAnimatedSprite(indexes.Lava, door.AnimationHash(), scene.getElapsedMs()), options)
	actualPixels, expectedPixels := make([]byte, 18*18*4), make([]byte, 18*18*4)
	scene.unscaledMapImage.ReadPixels(actualPixels)
	expected.ReadPixels(expectedPixels)
	require.Equal(t, expectedPixels, actualPixels, "effective door overrides cannot cover the committed lava reveal")
	state.EventQueueManager.Shutdown()
}
