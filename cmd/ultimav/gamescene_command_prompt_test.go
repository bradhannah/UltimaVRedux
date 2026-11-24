package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

// TestMapCommandHandlersUseOriginalPrompt prevents directional command echoes
// from bypassing the bitmap marker used by the original command loop.
func TestMapCommandHandlersUseOriginalPrompt(t *testing.T) {
	gameState, mockCallbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithSystemCallbacks().
		Build()

	testCases := []struct {
		name   string
		handle func(*GameScene)
	}{
		{name: "small map", handle: func(scene *GameScene) { scene.smallMapInputHandler(ebiten.KeyL) }},
		{name: "large map", handle: func(scene *GameScene) { scene.largeMapInputHandler(ebiten.KeyL) }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			mockCallbacks.Reset()
			scene := &GameScene{
				gameState: gameState,
				keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
			}

			testCase.handle(scene)

			assert.Equal(t, []string{"Look-"}, mockCallbacks.CommandPrompts)
			assert.Equal(t, LookDirectionInput, scene.secondaryKeyState)
		})
	}
}

func TestBoundKeysGameExcludesBackquote(t *testing.T) {
	assert.NotContains(t, boundKeysGame, ebiten.KeyBackquote)
}

// TestInputHandlersExcludeLegacyDebugExit prevents restoring Ctrl+X shortcuts
// that bypass normal exits and can start duplicate debug_exit workflows.
func TestInputHandlersExcludeLegacyDebugExit(t *testing.T) {
	for _, filename := range []string{
		"gamescene_input_smallmap.go",
		"gamescene_input_dungeon.go",
		"gamescene_input_combat.go",
	} {
		source, err := os.ReadFile(filepath.Clean(filename))
		if err != nil {
			t.Fatalf("read %s: %v", filename, err)
		}

		assert.NotContains(t, string(source), "debug_exit", filename)
		assert.NotContains(t, string(source), "ModifierCtrl, ebiten.KeyX", filename)
	}
}
