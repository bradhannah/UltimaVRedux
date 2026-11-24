package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceFountainRetainsTranscriptAroundCharacterSelection(t *testing.T) {
	scene, callbacks := newFountainWorkflowTestScene(t)
	dateBefore := scene.gameState.DateTime

	scene.ShowFountainInteraction()
	require.NotPanics(t, scene.ShowFountainInteraction)
	transcript := waitForFountainTranscript(t, scene, "Thou dost see a gurgling fountain!")
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, scene.dialogStack.Count())
	acknowledgeTranscript(t, scene, transcript)

	selector := waitForTopModalWithin[*widgets.CharacterSelectionDialog](t, scene, 1000, 100)
	require.Equal(t, 2, scene.dialogStack.Count())
	require.Same(t, transcript, scene.dialogStack.Dialogs()[0])
	selectedName := selector.GetCurrentOption().Name
	selector.HandleEnter()

	waitForFountainTranscript(t, scene, selectedName+" drinks from the fountain.")
	require.Same(t, transcript, scene.dialogStack.PeekTopModalDialog())
	require.Contains(t, transcript.TranscriptText(), "Who shall drink?  > "+selectedName)
	acknowledgeTranscript(t, scene, transcript)

	waitForFountainTranscript(t, scene, "Refreshing!")
	require.Contains(t, transcript.TranscriptText(), "Refreshing!")
	acknowledgeTranscript(t, scene, transcript)
	drainFountainWorkflow(t, scene)

	expectedDate := dateBefore
	expectedDate.Advance(1)
	require.Equal(t, expectedDate, scene.gameState.DateTime)
	require.Equal(t, []int{1}, callbacks.TimeAdvanced())
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
}

func TestSurfaceFountainCharacterCancellationCleansPersistentTranscript(t *testing.T) {
	scene, callbacks := newFountainWorkflowTestScene(t)
	dateBefore := scene.gameState.DateTime

	scene.ShowFountainInteraction()
	transcript := waitForFountainTranscript(t, scene, "Thou dost see a gurgling fountain!")
	acknowledgeTranscript(t, scene, transcript)
	selector := waitForTopModalWithin[*widgets.CharacterSelectionDialog](t, scene, 1000, 100)
	selector.HandleEscape()
	drainFountainWorkflow(t, scene)

	require.Equal(t, dateBefore, scene.gameState.DateTime)
	require.Empty(t, callbacks.TimeAdvanced())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
}

func newFountainWorkflowTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet: &sprites.SpriteSheet{},
	}, callbacks
}

func waitForFountainTranscript(
	t *testing.T,
	scene *GameScene,
	content string,
) *widgets.InteractionTranscript {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			dialog.ActivePrompt() == "" && !dialog.InputComplete() &&
			containsCollapsedText(dialog.TranscriptText(), content) {
			return dialog
		}
	}
	t.Fatalf("fountain transcript did not show %q", content)

	return nil
}

func drainFountainWorkflow(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow(fountainWorkflowName) {
			return
		}
	}
	t.Fatal("fountain workflow did not complete")
}

func containsCollapsedText(actual, expected string) bool {
	return strings.Contains(strings.Join(strings.Fields(actual), " "), strings.Join(strings.Fields(expected), " "))
}
