package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMixWorkflowShutdownRemovesOwnedDialogWithoutCompletingCommand(t *testing.T) {
	for _, started := range []bool{false, true} {
		t.Run(map[bool]string{false: "before first update", true: "dialog mounted"}[started], func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
			scene := newProductionCastTestScene(state)
			beforeTime, beforeRNG := state.DateTime, state.GetRNG().Calls()
			completions := 0
			scene.startReagentMixingWorkflowWithCompletion(func() { completions++ })
			if started {
				for range 10 {
					state.ElapsedMs += DialogPollingRateMs
					state.EventQueueManager.Update()
					if scene.dialogStack.Count() != 0 {
						break
					}
				}
				require.IsType(t, &widgets.ReagentMixingDialog{}, scene.dialogStack.PeekTopModalDialog())
			}
			state.EventQueueManager.Shutdown()
			state.EventQueueManager.Shutdown()
			assert.Zero(t, scene.dialogStack.Count(), "shutdown must release the workflow-owned modal")
			assert.Zero(t, completions, "shutdown must not complete a gameplay command")
			assert.Equal(t, beforeTime, state.DateTime)
			assert.Equal(t, beforeRNG, state.GetRNG().Calls())
			assert.False(t, state.EventQueueManager.HasActiveWorkflows())
		})
	}
}

func TestMixWorkflowDismissalCompletesOnceAndCleansUp(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithSystemCallbacks().Build()
	scene := newProductionCastTestScene(state)
	completions := 0
	scene.startReagentMixingWorkflowWithCompletion(func() { completions++ })
	for range 10 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		if scene.dialogStack.Count() != 0 {
			break
		}
	}
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReagentMixingDialog)
	require.True(t, ok)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	dialog.Update()
	for range 30 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
	}
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, 1, completions)
	state.EventQueueManager.Shutdown()
	assert.Equal(t, 1, completions)
}
