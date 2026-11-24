package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestOutdoorYellDefersSurfaceCompletionUntilWordEffectCleanup(t *testing.T) {
	scene, callbacks := newOutdoorYellWorkflowScene(t)
	entrance := scene.gameState.GetDungeonEntranceCoordinates()[references.Deceit]
	scene.gameState.MapState.PlayerLocation.Position = entrance.Right()
	scene.gameState.GetLayeredMapByCurrentLocation().SetTileByLayer(
		map_state.MapLayer,
		&entrance,
		indexes.DoomEntrance,
	)
	dateBefore := scene.gameState.DateTime

	scene.largeMapInputHandler(ebiten.KeyY)
	require.True(t, scene.gameState.EventQueueManager.HasWorkflow("word_of_power_entry"))
	require.True(t, scene.gameState.EventQueueManager.HasWorkflow("surface_command_completion"))
	assert.Equal(t, dateBefore, scene.gameState.DateTime)

	dialog := waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200)
	enterInputBoxText(t, scene, dialog, strings.Repeat("A", 24)+game_state.WordFallax)
	assert.True(t, scene.gameState.IsDungeonOpen(references.Deceit))
	assert.Equal(t, dateBefore, scene.gameState.DateTime)

	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)
	require.False(t, scene.dialogStack.HasOpenDialog())
	drainSceneWorkflow(t, scene, "surface_command_completion", 1000)
	expectedDate := dateBefore
	expectedDate.Advance(2)
	assert.Equal(t, expectedDate, scene.gameState.DateTime)
	assert.Equal(t, 1, callbacks.BlockInputCalls)
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestOutdoorYellEscapePreservesMapAndCompletesOneCycle(t *testing.T) {
	scene, _ := newOutdoorYellWorkflowScene(t)
	entrance := scene.gameState.GetDungeonEntranceCoordinates()[references.Deceit]
	scene.gameState.MapState.PlayerLocation.Position = entrance.Right()
	scene.gameState.GetLayeredMapByCurrentLocation().SetTileByLayer(
		map_state.MapLayer,
		&entrance,
		indexes.DoomEntrance,
	)
	dateBefore := scene.gameState.DateTime

	scene.largeMapInputHandler(ebiten.KeyY)
	dialog := waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	dialog.Update()
	assert.False(t, scene.gameState.IsDungeonOpen(references.Deceit))
	assert.Equal(t, dateBefore, scene.gameState.DateTime)

	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)
	drainSceneWorkflow(t, scene, "surface_command_completion", 1000)
	expectedDate := dateBefore
	expectedDate.Advance(2)
	assert.Equal(t, expectedDate, scene.gameState.DateTime)
	assert.False(t, scene.dialogStack.HasOpenDialog())
}

func TestOutdoorYellCompletionRunsCodexCheckpointExactlyOnce(t *testing.T) {
	scene, callbacks := newOutdoorYellWorkflowScene(t)
	scene.gameState.MapState.PlayerLocation.Position = references.Position{X: 0xe9, Y: 0xeb}
	callbacks.Reset()

	scene.largeMapInputHandler(ebiten.KeyY)
	dialog := waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	dialog.Update()
	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)
	drainSceneWorkflow(t, scene, "surface_command_completion", 1000)

	assert.Equal(t, references.Position{X: 0xe9, Y: 0xec}, scene.gameState.MapState.PlayerLocation.Position)
	assert.Equal(t, 1, countEqual(callbacks.Messages, "\"Thou art not upon a Sacred Quest!\nPassage denied!\""))
}

func newOutdoorYellWorkflowScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.OVERWORLD)).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 20, 80),
	}
	state.SystemCallbacks.UI.ShowYellDialogLargeMap = scene.ShowYellDialogLargeMap

	return scene, callbacks
}
