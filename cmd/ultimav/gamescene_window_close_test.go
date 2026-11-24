package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func TestWindowCloseInGameplayOffersSaveThenSecondCloseQuits(t *testing.T) {
	scene := newEscapeMenuTestScene()
	game := NewGame(scene)

	game.handleWindowCloseRequest()

	require.False(t, game.exitRequested, "the first close keeps the window open for the save choices")
	prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	assert.Equal(t, []string{"Quick Save", "Save", "Exit Without Saving", "Cancel"}, prompt.ButtonTexts())

	game.handleWindowCloseRequest()

	assert.True(t, game.exitRequested, "a second close while the choices are open quits")
}

func TestWindowCloseCancelReturnsToPlayAndCanAskAgain(t *testing.T) {
	scene := newEscapeMenuTestScene()
	game := NewGame(scene)

	game.handleWindowCloseRequest()
	prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	for range 3 {
		prompt.GoDown()
	}
	pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)

	assert.False(t, scene.dialogStack.HasModalDialog(), "Cancel returns to play, not to the Escape menu")
	assert.False(t, game.exitRequested)

	game.handleWindowCloseRequest()
	requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	assert.False(t, game.exitRequested)
}

func TestWindowCloseExitWithoutSaveQuits(t *testing.T) {
	scene := newEscapeMenuTestScene()
	game := NewGame(scene)

	game.handleWindowCloseRequest()
	prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	prompt.GoDown()
	prompt.GoDown()
	pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)

	assert.True(t, game.exitRequested)
}

func TestWindowCloseOutsideGameplayQuitsImmediately(t *testing.T) {
	game := NewGame(&playerSaveTestScene{})
	game.windowCloseRequested = func() bool { return true }

	assert.ErrorIs(t, game.Update(), ebiten.Termination)
}
