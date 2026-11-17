package main

import (
	"errors"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

type popupViewportTestScene struct {
	noDebugDispatchTestScene
	viewport   image.Rectangle
	onActivate func()
	onUpdate   func(*Game) error
	onDraw     func()
}

func (s *popupViewportTestScene) mapPopupViewport(image.Rectangle) image.Rectangle {
	return s.viewport
}

func (s *popupViewportTestScene) Activate() {
	if s.onActivate != nil {
		s.onActivate()
	}
}

func (s *popupViewportTestScene) Update(game *Game) error {
	if s.onUpdate != nil {
		return s.onUpdate(game)
	}

	return nil
}

func (s *popupViewportTestScene) Draw(*ebiten.Image) {
	if s.onDraw != nil {
		s.onDraw()
	}
}

func TestGameMapPopupViewportOwnership(t *testing.T) {
	manager := display.GetManager()
	screen := image.Rect(0, 0, 1280, 720)
	previous := manager.SetMapPopupViewportProvider(nil)
	t.Cleanup(func() { manager.SetMapPopupViewportProvider(previous) })
	first := &popupViewportTestScene{viewport: image.Rect(20, 30, 600, 700)}
	second := &popupViewportTestScene{viewport: image.Rect(40, 50, 500, 600)}
	first.onActivate = func() {
		assert.Equal(t, first.viewport, manager.MapPopupViewport(screen), "bound before initial activation")
	}
	game := NewGame(first)
	assert.Equal(t, image.Rectangle{}, manager.MapPopupViewport(screen), "construction does not leak ownership")
	first.onUpdate = func(game *Game) error {
		assert.Equal(t, first.viewport, manager.MapPopupViewport(screen))
		dormant := &GameScene{gameState: &game_state.GameState{}}
		dormant.gameState.MapState.PlayerLocation.Location = references.CombatLocation
		assert.Equal(t, combatLayout{}.battlefieldRectangle(screen), dormant.mapPopupViewport(screen))
		assert.Equal(t, first.viewport, manager.MapPopupViewport(screen), "dormant scene does not bind")
		NewGame(&popupViewportTestScene{viewport: image.Rect(1, 1, 2, 2)})
		assert.Equal(t, first.viewport, manager.MapPopupViewport(screen), "nested construction restores ownership")
		require.NoError(t, game.ReplaceScene(func() (Scene, error) { return second, nil }))
		assert.Equal(t, second.viewport, manager.MapPopupViewport(screen), "replacement is visible in the same update")

		return nil
	}
	require.NoError(t, game.Update())
	assert.Equal(t, image.Rectangle{}, manager.MapPopupViewport(screen))
	second.onDraw = func() {
		assert.Equal(t, second.viewport, manager.MapPopupViewport(screen), "draw owns its provider")
	}
	game.Draw(nil)
	assert.Equal(t, image.Rectangle{}, manager.MapPopupViewport(screen))
	second.onUpdate = func(game *Game) error {
		game.replaceScene(&noDebugDispatchTestScene{})
		assert.Equal(t, image.Rectangle{}, manager.MapPopupViewport(screen), "optional interface may be absent")
		game.currentScene = nil
		assert.Equal(t, image.Rectangle{}, manager.MapPopupViewport(screen), "nil scene has no override")

		return nil
	}
	require.NoError(t, game.Update())
}

func TestGameMapPopupViewportRestoresAfterFailure(t *testing.T) {
	manager := display.GetManager()
	screen := image.Rect(0, 0, 1280, 720)
	outer := image.Rect(10, 10, 100, 100)
	previous := manager.SetMapPopupViewportProvider(func(image.Rectangle) image.Rectangle { return outer })
	t.Cleanup(func() { manager.SetMapPopupViewportProvider(previous) })
	scene := &popupViewportTestScene{viewport: image.Rect(20, 30, 600, 700)}
	game := NewGame(scene)
	assert.Equal(t, outer, manager.MapPopupViewport(screen))
	updateErr := errors.New("update failed")
	scene.onUpdate = func(*Game) error { return updateErr }
	require.ErrorIs(t, game.Update(), updateErr)
	assert.Equal(t, outer, manager.MapPopupViewport(screen))
	scene.onDraw = func() { panic("draw failed") }
	assert.Panics(t, func() { game.Draw(nil) })
	assert.Equal(t, outer, manager.MapPopupViewport(screen))
	scene.onActivate = func() { panic("activation failed") }
	assert.Panics(t, func() { NewGame(scene) })
	assert.Equal(t, outer, manager.MapPopupViewport(screen))
}

func TestGameSceneMapPopupViewport(t *testing.T) {
	scene := &GameScene{}
	assert.Equal(t, image.Rectangle{}, scene.mapPopupViewport(image.Rect(0, 0, 1280, 720)))
	scene.gameState = &game_state.GameState{}
	scene.gameState.MapState.PlayerLocation.Location = references.Britain
	assert.Equal(t, image.Rectangle{}, scene.mapPopupViewport(image.Rect(0, 0, 1280, 720)))
	scene.gameState.MapState.PlayerLocation.Location = references.CombatLocation
	for _, bounds := range []image.Rectangle{
		image.Rect(0, 0, 1280, 720),
		image.Rect(0, 0, 1920, 1080),
		image.Rect(100, 200, 2660, 1640),
	} {
		viewport := scene.mapPopupViewport(bounds)
		assert.Equal(t, combatLayout{}.battlefieldRectangle(bounds), viewport)
		assert.False(t, viewport.Empty())
		assert.True(t, viewport.In(bounds))
	}
	scene.gameState.MapState.PlayerLocation.Location = references.Britain
	assert.Equal(t, image.Rectangle{}, scene.mapPopupViewport(image.Rect(0, 0, 1280, 720)), "combat exit is dynamic")
}
