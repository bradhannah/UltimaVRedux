package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestDeathKarmaReportWaitsForAcknowledgementAndCleansUp(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 10, 80),
	}
	state.EventQueueManager.Workflow("death_report_acknowledgement").
		Priority(event_queue.PriorityNormal).
		Sequential().
		ThenSequence(scene.thenShowDeathKarmaReport("synthetic report")).
		Start()

	state.EventQueueManager.Update()
	overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
	require.True(t, ok)
	assert.True(t, overlay.IsVisible())
	assert.True(t, state.EventQueueManager.HasWorkflow("death_report_acknowledgement"))

	state.EventQueueManager.Update()
	assert.True(t, state.EventQueueManager.HasWorkflow("death_report_acknowledgement"))
	overlay.Dismiss()
	for range 3 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
	}

	assert.False(t, state.EventQueueManager.HasWorkflow("death_report_acknowledgement"))
	assert.Nil(t, scene.dialogStack.PeekTopModalDialog())
}
