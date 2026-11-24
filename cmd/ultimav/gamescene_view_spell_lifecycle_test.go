package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestGemViewWorkflowRemovesDialogAfterDismissal(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
	var dialog *widgets.GemViewDialog

	state.EventQueueManager.Workflow("test_gem_view_lifecycle").
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenShowGemViewDialog(&dialog)).
		Finally(func() {}).
		Start()
	state.EventQueueManager.Update()

	require.NotNil(t, dialog)
	assert.Same(t, dialog, scene.dialogStack.PeekTopModalDialog())
	dialog.Dismiss()

	for range 5 {
		state.ElapsedMs += GemViewFlashIntervalMs
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("test_gem_view_lifecycle") {
			break
		}
	}

	assert.Nil(t, dialog)
	assert.Zero(t, scene.dialogStack.Count())
	assert.False(t, state.EventQueueManager.HasWorkflow("test_gem_view_lifecycle"))
}

func TestFinishSpellCastingWorkflowRestoresOwnedViewState(t *testing.T) {
	scene := &GameScene{keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
	dialog := widgets.NewGemViewDialog(widgets.GemViewDialogConfig{
		Placement: centeredGameDialogPercents,
		Keyboard:  scene.keyboard,
	})
	scene.dialogStack.PushModalDialog(dialog)
	scene.xRayVisionActive = true
	state := &spellCastingWorkflowState{
		gemViewDialog: dialog,
		restoreXRay:   true,
		previousXRay:  false,
	}

	scene.finishSpellCastingWorkflow(state)
	scene.finishSpellCastingWorkflow(state)

	assert.Nil(t, state.gemViewDialog)
	assert.False(t, scene.xRayVisionActive)
	assert.False(t, state.restoreXRay)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestFinishSpellCastingWorkflowPreservesPreviousXRayState(t *testing.T) {
	scene := &GameScene{keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), xRayVisionActive: true}
	state := &spellCastingWorkflowState{restoreXRay: true, previousXRay: true}

	scene.finishSpellCastingWorkflow(state)
	scene.xRayVisionActive = false
	scene.finishSpellCastingWorkflow(state)

	assert.False(t, scene.xRayVisionActive, "idempotent cleanup must not reapply stale owned state")
	assert.False(t, state.restoreXRay)
}

func TestSurfaceViewTileSourceWrapsAndExcludesUnits(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(0, 0).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: state}
	position := references.Position{X: 255, Y: 255}
	theMap := state.GetLayeredMapByCurrentLocation()
	theMap.SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
	theMap.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.BrickFloor)
	theMap.SetTileByLayer(map_state.MapUnitLayer, &position, indexes.Guard1)

	assert.Equal(t, int(indexes.BrickFloor), scene.getSurfaceViewTileIndex(-1, -1))
}

func TestGemViewWorkflowDoesNotOwnTimeAdvancement(t *testing.T) {
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}

	scene.StartGemViewWorkflow()
	state.EventQueueManager.Update()
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.GemViewDialog)
	require.True(t, ok)
	dialog.Dismiss()
	for range 5 {
		state.ElapsedMs += GemViewFlashIntervalMs
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("gem_view_workflow") {
			break
		}
	}

	assert.Empty(t, mock.TimeAdvanced())
}
