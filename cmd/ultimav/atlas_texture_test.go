package main

import (
	"image/color"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/atlasterrain"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasTextureBorrowSurvivesModalAndSceneReplacement(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().Build()
	terrain := state.GameReferences.OverworldLargeMapReference
	input, err := atlasterrain.Capture(func(x, y int) int {
		return int(terrain.GetSpriteIndex(references.Coordinate(x), references.Coordinate(y)))
	})
	require.NoError(t, err)
	// This test proves shared texture ownership, not 4096-square geography
	// generation (covered by atlasterrain's CPU cold/warm cache tests). Seed the
	// production owner's cache-hit path with small, positively observed artwork.
	first := ebiten.NewImage(8, 8)
	first.Fill(color.RGBA{R: 73, G: 19, B: 41, A: 255})
	game := &Game{atlasTexture: atlasTextureOwner{
		root: t.TempDir(), texture: first, identity: atlasterrain.Identity(input, sprites.TileAtlasPNG()),
	}}
	t.Cleanup(game.atlasTexture.dispose)
	cached, err := game.atlasTexture.load(terrain)
	require.NoError(t, err)
	require.Same(t, first, cached)
	for range 2 {
		scene := &GameScene{gameState: state, keyboard: newPresentationTestKeyboard()}
		scene.openAtlas(game)
		require.Same(t, first, game.atlasTexture.texture)
		pixels := make([]byte, 8*8*4)
		first.ReadPixels(pixels)
		for index := 0; index < len(pixels); index += 4 {
			require.Equal(t, []byte{73, 19, 41, 255}, pixels[index:index+4], "closing a borrower must preserve its owner's artwork")
		}
		scene.closeAtlasSession(true)
		scene.Dispose()
		require.Same(t, first, game.atlasTexture.texture)
	}
	game.atlasTexture.dispose()
	require.Nil(t, game.atlasTexture.texture)
	require.Empty(t, game.atlasTexture.identity)
	require.NotPanics(t, game.atlasTexture.dispose)
}
