package main

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

type blockingDebugWorkflowDialog struct{}

func (*blockingDebugWorkflowDialog) Draw(*ebiten.Image) {}
func (*blockingDebugWorkflowDialog) Update()            {}

func TestDebugAstronomyWorkflowsQueueProductionEntryPointsWithTelemetry(t *testing.T) {
	testCases := []struct {
		name           string
		commandID      string
		workflow       string
		hour           byte
		expectedBranch string
		grantSpyglass  bool
	}{
		{name: "telescope day", commandID: debugging.CommandTestTelescope, workflow: "telescope_workflow", hour: 12, expectedBranch: "sun_damage"},
		{name: "telescope night", commandID: debugging.CommandTestTelescope, workflow: "telescope_workflow", hour: 22, expectedBranch: "night_sky"},
		{name: "spyglass day", commandID: debugging.CommandTestSpyglass, workflow: "spyglass_workflow", hour: 12, expectedBranch: "no_stars", grantSpyglass: true},
		{name: "crystal ball", commandID: debugging.CommandTestCrystalBall, workflow: "crystal_ball_workflow", hour: 22, expectedBranch: "character_selection"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			scene := newCommandWorkflowTestScene(t)
			scene.gameState.DateTime.Hour = testCase.hour
			scene.gameState.DateTime.Minute = 17
			if testCase.grantSpyglass {
				scene.gameState.PartyState.Inventory.QuestItems.Set(references.Spyglass, 1)
			}

			result := scene.ExecuteDebugCommand(debugging.Request{CommandID: testCase.commandID})

			require.True(t, result.Success, result.ErrorMessage)
			assert.Equal(t, debugging.ExecutionWorkflow, result.EffectiveExecutionMode)
			assert.True(t, scene.gameState.EventQueueManager.HasWorkflow(testCase.workflow))
			var telemetry commandWorkflowQueuedResult
			require.NoError(t, json.Unmarshal(result.Data, &telemetry))
			assert.True(t, telemetry.Queued)
			assert.Equal(t, testCase.workflow, telemetry.Workflow)
			assert.Equal(t, testCase.expectedBranch, telemetry.ExpectedBranch)
			assert.Equal(t, int(testCase.hour), telemetry.Context.Hour)
			assert.Equal(t, 17, telemetry.Context.Minute)
			assert.Equal(t, "small", telemetry.Context.MapType)
			assert.Equal(t, references.Britain.String(), telemetry.Context.LocationName)
		})
	}
}

func TestDebugCommandWorkflowsRejectLifecycleContextInventoryAndCallbackFailures(t *testing.T) {
	t.Run("dialog", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		scene.dialogStack.PushModalDialog(&blockingDebugWorkflowDialog{})

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestTelescope})

		assert.Equal(t, "dialog_active", result.ErrorCode)
		assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	})

	t.Run("active workflow", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		first := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestTelescope})
		require.True(t, first.Success)

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestCrystalBall})

		assert.Equal(t, "gameplay_workflow_active", result.ErrorCode)
		assert.False(t, scene.gameState.EventQueueManager.HasWorkflow("crystal_ball_workflow"))
	})

	t.Run("map context", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		scene.gameState.MapState.PlayerLocation.Location = references.BritanniaUnderworld

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestTelescope})

		assert.Equal(t, "unsupported_map_type", result.ErrorCode)
		assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	})

	t.Run("spyglass ownership", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestSpyglass})

		assert.Equal(t, "spyglass_not_owned", result.ErrorCode)
		assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	})

	t.Run("spyglass floor", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		scene.gameState.PartyState.Inventory.QuestItems.Set(references.Spyglass, 1)
		scene.gameState.MapState.PlayerLocation.Floor = -1

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestSpyglass})

		assert.Equal(t, "unsupported_floor", result.ErrorCode)
		assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	})

	t.Run("callbacks", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		scene.gameState.SystemCallbacks.Audio.PlaySoundEffect = nil

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestCrystalBall})

		assert.Equal(t, "callbacks_unavailable", result.ErrorCode)
		assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
	})
}

func newCommandWorkflowTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithFloor(0).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	return &GameScene{
		gameState:   state,
		output:      text.NewOutput(nil, 1, 10, 80),
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet: &sprites.SpriteSheet{},
	}
}
