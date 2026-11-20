package main

import (
	"context"
	"image"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/hajimehoshi/ebiten/v2"
)

// These adapters let focused fixtures exercise production operations without
// provisioning the surrounding application. They are not gameplay entry points.

// noSceneTestKeyHeld isolates synthetic fixtures from the live desktop. Tests
// exercising physical holds, modifiers or release gates inject their own key map.
func noSceneTestKeyHeld(ebiten.Key) bool { return false }

func activeEffectIconLayout(bounds image.Rectangle, count int) []image.Rectangle {
	return statusIconRow(bounds, count, maximumPersistentEffects, true)
}

func (g *GameScene) largeMapInputHandler(key ebiten.Key) {
	g.largeMapInputHandlerWithGame(nil, key)
}

func (g *GameScene) smallMapInputHandler(key ebiten.Key) {
	g.smallMapInputHandlerWithGame(nil, key)
}

func (g *GameScene) dungeonMapInputHandler(key ebiten.Key) {
	g.dungeonMapInputHandlerWithGame(nil, key)
}

// Ordinary direction-input fixtures exercise the non-Cast cancellation policy.
func (g *GameScene) thenShowDirectionPrompt(
	message, hintText string,
	onDirection func(references.Direction),
	onPass func(),
) event_queue.Sequence {
	return g.thenShowDirectionPromptWithCancellation(message, hintText, directionPromptChars, onDirection, onPass, false)
}

func (g *Game) prepareLegacyPlayerSave(selectedSavedGamPath string) (preparedLegacyPlayerSave, error) {
	return g.prepareLegacyPlayerSaveContext(context.Background(), selectedSavedGamPath)
}

func (g *Game) replaceScene(scene Scene) {
	if scene == nil {
		panic("Game.replaceScene: scene is required")
	}
	if err := g.ReplaceScene(func() (Scene, error) { return scene, nil }); err != nil {
		panic("Game.replaceScene: " + err.Error())
	}
}
