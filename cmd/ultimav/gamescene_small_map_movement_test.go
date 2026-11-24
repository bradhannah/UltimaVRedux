package main

import (
	"encoding/json"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestMovementLoggingEnabledAfterStatusLoadRetainsSubsequentMovement(t *testing.T) {
	logger := logging.GetGlobalLogger()
	beforeEnabled, beforeLevel, beforeCategories := logger.IsEnabled(), logger.GetMinLevel(), logger.GetCategoryStates()
	t.Cleanup(func() {
		logger.SetEnabled(beforeEnabled)
		logger.SetMinLevel(beforeLevel)
		logger.SetCategoryStates(beforeCategories)
	})
	logger.SetEnabled(true)
	logger.SetMinLevel(logging.DEBUG)
	logger.SetCategoryEnabled(logging.CategoryMovement, false)
	cursor := logger.EntriesAfter(0, 1).NewestSequence

	status := executeDebugCommand(nil, debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandLoggingStatus,
	})
	require.True(t, status.Success)
	category := executeDebugCommand(nil, debugging.Request{
		SessionID: "test", ID: "movement", CommandID: debugging.CommandLoggingCategorySet,
		Arguments: map[string]json.RawMessage{
			"category": json.RawMessage(`"movement"`),
			"enabled":  json.RawMessage(`true`),
		},
	})
	require.True(t, category.Success)

	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: state}
	scene.moveToNewPositionByDirection(references.Right)

	batch := logger.EntriesAfter(cursor, 10)
	require.NotEmpty(t, batch.Entries)
	entry := batch.Entries[len(batch.Entries)-1]
	assert.Equal(t, logging.CategoryMovement, entry.Category)
	assert.Contains(t, entry.Message, "from=(15,15) to=(16,15)")
}

func TestSmallMapBoundaryMovementPromptsToLeaveWithoutOutOfBoundsMessage(t *testing.T) {
	tests := []struct {
		name      string
		position  references.Position
		direction references.Direction
		key       ebiten.Key
	}{
		{name: "north", position: references.Position{X: 15, Y: 0}, direction: references.Up, key: ebiten.KeyUp},
		{name: "south", position: references.Position{X: 15, Y: 31}, direction: references.Down, key: ebiten.KeyDown},
		{name: "west", position: references.Position{X: 0, Y: 15}, direction: references.Left, key: ebiten.KeyLeft},
		{name: "east", position: references.Position{X: 31, Y: 15}, direction: references.Right, key: ebiten.KeyRight},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(int(test.position.X), int(test.position.Y)).
				WithSystemCallbacks().
				Build()
			output := text.NewOutput(nil, 1, 10, 80)
			scene := &GameScene{
				gameState:            gs,
				output:               output,
				moongateVisualSystem: NewMoongateVisualSystem(gs),
			}

			result := scene.handleMovement(test.direction.GetDirectionCompassName(), test.key)

			assert.Equal(t, surfaceCommandDeferred, result.State)
			assert.Equal(t, test.position, gs.MapState.PlayerLocation.Position)
			assert.True(t, gs.EventQueueManager.HasActiveWorkflows())
			assert.True(t, callbacks.InputBlocked)
			assert.Equal(t, 1, callbacks.BlockInputCalls)
			require.NotPanics(t, func() {
				repeated := scene.handleMovement(test.direction.GetDirectionCompassName(), test.key)
				assert.Equal(t, surfaceCommandStillActive, repeated.State)
			})
			assert.Equal(t, 1, callbacks.BlockInputCalls, "repeated movement must not start another exit workflow")
			assert.NotContains(t, output.GetOutputStr(true), "OUT OF BOUNDS")
		})
	}
}

func TestSmallMapBlockedHeldMovementRequiresPhysicalRelease(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithRandomSeed(426).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	origin, direction := findBlockedSmallMapStep(t, state)
	state.MapState.PlayerLocation.Position = origin
	callbacks.Reset()

	held := true
	key := smallMapMovementKey(direction)
	scene := newSurfaceCompletionTestScene(state)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(candidate ebiten.Key) bool {
		return candidate == key && held
	})

	require.True(t, scene.keyboard.IsKeyPressedAndRegisterRepeatable(key))
	scene.smallMapInputHandler(key)

	assert.Equal(t, origin, state.MapState.PlayerLocation.Position)
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Contains(t, scene.output.GetOutputStr(true), "Blocked!")
	require.True(t, scene.keyboard.IsWaitingForRelease())

	scene.keyboard.ClearFrameConsumption()
	require.False(t, scene.keyboard.IsKeyPressedAndRegisterRepeatable(key))
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())

	held = false
	scene.keyboard.ClearFrameConsumption()
	require.False(t, scene.keyboard.IsWaitingForRelease())

	state.DebugOptions.FreeMove = true
	held = true
	require.True(t, scene.keyboard.IsKeyPressedAndRegisterRepeatable(key))
	scene.smallMapInputHandler(key)

	target := direction.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)
	assert.Equal(t, *target, state.MapState.PlayerLocation.Position)
	assert.Equal(t, []int{
		game_state.DefaultSmallMapMinutesPerTurn,
		game_state.DefaultSmallMapMinutesPerTurn,
	}, callbacks.TimeAdvanced())
	require.True(t, scene.keyboard.IsKeyPressedAndRegisterRepeatable(key),
		"successful movement must remain repeatable while held")
}
