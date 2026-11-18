package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type surfaceCadenceController struct {
	ai.NPCAIController
	turns int
}

func (c *surfaceCadenceController) AdvanceNextTurnCalcAndMoveNPCs() { c.turns++ }

func TestSurfaceTownPassBypassesMountedGateWithoutResettingIt(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.CarpetVehicle,
		state.MapState.PlayerLocation.Position, state.MapState.PlayerLocation.Floor)
	controller := &surfaceCadenceController{NPCAIController: state.CurrentNPCAIController}
	state.CurrentNPCAIController = controller
	scene := newSurfaceCompletionTestScene(state)
	callbacks.Reset()
	scene.completeSurfaceMovement(newSurfaceCommandResult(surfaceCommandRejected, references.SmallMapType))
	assert.Zero(t, controller.turns, "first mounted blocked attempt skips actors")
	scene.smallMapInputHandler(ebiten.KeySpace)
	assert.Equal(t, 1, controller.turns, "town Pass runs actors without consuming riding phase")
	scene.completeSurfaceMovement(newSurfaceCommandResult(surfaceCommandRejected, references.SmallMapType))
	assert.Equal(t, 2, controller.turns, "second mounted blocked attempt processes actors")
	assert.Equal(t, []int{1, 1, 1}, callbacks.TimeAdvanced())
}

func TestSurfacePrimaryCompletionUsesFixedPerMapCycle(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		key      ebiten.Key
		wantTime []int
		handle   func(*GameScene, ebiten.Key)
	}{
		{
			name: "completed town command", location: references.Britain, key: ebiten.KeySpace,
			wantTime: []int{game_state.DefaultSmallMapMinutesPerTurn},
			handle:   (*GameScene).smallMapInputHandler,
		},
		{
			name: "rejected overworld command", location: references.BritanniaUnderworld, key: ebiten.KeyEnter,
			wantTime: []int{game_state.DefaultLargeMapMinutesPerTurn},
			handle:   (*GameScene).largeMapInputHandler,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := game_state.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(15, 15).
				WithRandomSeed(387).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
			}
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)

			test.handle(scene, test.key)

			assert.Equal(t, test.wantTime, callbacks.TimeAdvanced())
			assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
		})
	}
}

func TestSurfaceSecondaryEscapeCancelsWithoutCycle(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
	}
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	scene.secondaryKeyState = AttackDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)

	scene.smallMapHandleSecondaryInput()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
}

func TestSurfaceNoOpAndQuitCommandsDoNotConsumeCycle(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		key      ebiten.Key
		feedback string
		handle   func(*GameScene, ebiten.Key)
	}{
		{name: "small map D", location: references.Britain, key: ebiten.KeyD, feedback: "D-What?", handle: (*GameScene).smallMapInputHandler},
		{name: "small map W", location: references.Britain, key: ebiten.KeyW, feedback: "W-What?", handle: (*GameScene).smallMapInputHandler},
		{name: "large map D", location: references.BritanniaUnderworld, key: ebiten.KeyD, feedback: "D-What?", handle: (*GameScene).largeMapInputHandler},
		{name: "large map W", location: references.BritanniaUnderworld, key: ebiten.KeyW, feedback: "W-What?", handle: (*GameScene).largeMapInputHandler},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := game_state.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
			}
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)

			test.handle(scene, test.key)

			assert.Equal(t, []string{test.feedback}, callbacks.Messages)
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
		})
	}
}

func TestSurfaceQuitRetainsInputWithoutConsumingCycle(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
	}
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)

	scene.smallMapInputHandler(ebiten.KeyQ)

	assert.Equal(t, SurfaceQuitInput, scene.secondaryKeyState)
	assert.Empty(t, callbacks.CommandPrompts)
	assert.Empty(t, callbacks.Messages)
	require.NotNil(t, scene.saveQuestionDialog)
	assert.Contains(t, scene.saveQuestionDialog.GetContentText(), "[Y]")
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)

	scene.keyboard.SimulateKeyPress(ebiten.KeyA)
	scene.surfaceQuitSecondaryInput(nil)
	assert.Equal(t, SurfaceQuitInput, scene.secondaryKeyState)

	scene.keyboard.SimulateKeyPress(ebiten.KeyN)
	scene.surfaceQuitSecondaryInput(nil)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Contains(t, callbacks.Messages, "Save game? No")
	assert.Nil(t, scene.saveQuestionDialog)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
}

func TestSurfaceMovementCompletionSeparatesBlockedAndMovedOverworldSteps(t *testing.T) {
	t.Run("blocked overworld movement consumes no cycle", func(t *testing.T) {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithPlayerAt(50, 50).
			WithSystemCallbacks().
			Build()
		state.DebugOptions.MonsterGen = false
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
		}
		target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
		state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.TallMountains)
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)

		scene.largeMapInputHandler(ebiten.KeyRight)

		assert.Empty(t, callbacks.TimeAdvanced())
		assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
	})

	t.Run("moved overworld step adds movement-only terrain cost", func(t *testing.T) {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithPlayerAt(50, 50).
			WithRandomSeed(387).
			WithSystemCallbacks().
			Build()
		state.DebugOptions.FreeMove = true
		state.DebugOptions.MonsterGen = false
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
		}
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)

		scene.largeMapInputHandler(ebiten.KeyRight)

		movementCost := state.GetCurrentAvatarGroundTile().GetSpeedFactor()
		elapsed := 0
		for _, minutes := range callbacks.TimeAdvanced() {
			elapsed += minutes
		}
		assert.Equal(t, movementCost, elapsed)
		wantAdvances := uint32(1)
		if movementCost > game_state.DefaultLargeMapMinutesPerTurn {
			wantAdvances++
		}
		assert.Equal(t, wantAdvances, state.DateTime.Turn, "slow terrain time precedes ordinary completion")
		assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	})
}

func TestSurfaceTransitionSuppressesOriginCycleButDeclinedBoundaryConsumesTownCycle(t *testing.T) {
	t.Run("successful transition", func(t *testing.T) {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithSystemCallbacks().
			Build()
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
		}
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)
		result := newSurfaceCommandResult(surfaceCommandCompleted, references.SmallMapType)
		result.MapTransitioned = true

		scene.completeSurfaceCommand(result)
		scene.completeSurfaceMovement(result)

		assert.Empty(t, callbacks.TimeAdvanced())
		assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
	})

	t.Run("declined boundary", func(t *testing.T) {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithSystemCallbacks().
			Build()
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
		}
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)
		result := newSurfaceCommandResult(surfaceCommandCancelled, references.SmallMapType)

		scene.completeSurfaceMovement(result)

		assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
		assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	})
}

func TestSurfaceResultContractDistinguishesAllStates(t *testing.T) {
	states := []surfaceCommandState{
		surfaceCommandCompleted,
		surfaceCommandRejected,
		surfaceCommandCancelled,
		surfaceCommandDeferred,
		surfaceCommandStillActive,
	}
	for _, state := range states {
		result := newSurfaceCommandResult(state, references.SmallMapType)
		assert.Equal(t, state, result.State)
		assert.Equal(t, state < surfaceCommandDeferred, result.ConsumesNormalCycle)
		assert.False(t, result.MovementOccurred)
		assert.False(t, result.MapTransitioned)
	}
}

func TestConsumedRejectedAndCancelledTurnsRunCodexCheckpointOnlyAtCompletion(t *testing.T) {
	for _, commandState := range []surfaceCommandState{surfaceCommandRejected, surfaceCommandCancelled} {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.OVERWORLD)).
			WithPlayerAt(0xe9, 0xeb).
			WithSystemCallbacks().
			Build()
		state.DebugOptions.MonsterGen = false
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)

		scene.completeSurfaceCommand(newSurfaceCommandResult(commandState, references.LargeMapType))

		assert.Equal(t, references.Position{X: 0xe9, Y: 0xec}, state.MapState.PlayerLocation.Position)
		assert.Contains(t, callbacks.Messages, "\"Thou art not upon a Sacred Quest!\nPassage denied!\"")
	}
}

func TestDeferredFreeAndTransitionResultsDoNotRunCodexCheckpoint(t *testing.T) {
	states := []surfaceCommandResult{
		newSurfaceCommandResult(surfaceCommandDeferred, references.LargeMapType),
		newSurfaceCommandResult(surfaceCommandStillActive, references.LargeMapType),
	}
	free := newSurfaceCommandResult(surfaceCommandCompleted, references.LargeMapType)
	free.ConsumesNormalCycle = false
	states = append(states, free)
	transition := newSurfaceCommandResult(surfaceCommandCompleted, references.LargeMapType)
	transition.MapTransitioned = true
	states = append(states, transition)

	for _, result := range states {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.OVERWORLD)).
			WithPlayerAt(0xe9, 0xeb).
			WithSystemCallbacks().
			Build()
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)

		scene.completeSurfaceCommand(result)

		assert.Equal(t, references.Position{X: 0xe9, Y: 0xeb}, state.MapState.PlayerLocation.Position)
		assert.Empty(t, callbacks.Messages)
	}
}

func TestSurfaceReadyAndViewCompleteOnlyAfterModalCleanup(t *testing.T) {
	tests := []struct {
		name         string
		location     references.Location
		key          ebiten.Key
		workflowName string
		prepare      func(*game_state.GameState)
		dismiss      func(*testing.T, *GameScene)
		wantMinutes  int
	}{
		{
			name: "town Ready", location: references.Britain, key: ebiten.KeyR,
			workflowName: "ready_equipment_workflow", wantMinutes: game_state.DefaultSmallMapMinutesPerTurn,
			dismiss: func(t *testing.T, scene *GameScene) {
				dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReadyEquipmentDialog)
				require.True(t, ok)
				dialog.Dismiss()
			},
		},
		{
			name: "overworld Ready", location: references.BritanniaUnderworld, key: ebiten.KeyR,
			workflowName: "ready_equipment_workflow", wantMinutes: game_state.DefaultLargeMapMinutesPerTurn,
			dismiss: func(t *testing.T, scene *GameScene) {
				dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReadyEquipmentDialog)
				require.True(t, ok)
				dialog.Dismiss()
			},
		},
		{
			name: "town View", location: references.Britain, key: ebiten.KeyV,
			workflowName: "gem_view_workflow", wantMinutes: game_state.DefaultSmallMapMinutesPerTurn,
			prepare: func(state *game_state.GameState) {
				state.PartyState.Inventory.Provisions.Gems.Set(1)
			},
			dismiss: func(t *testing.T, scene *GameScene) {
				dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.GemViewDialog)
				require.True(t, ok)
				dialog.Dismiss()
			},
		},
		{
			name: "overworld View", location: references.BritanniaUnderworld, key: ebiten.KeyV,
			workflowName: "gem_view_workflow", wantMinutes: game_state.DefaultLargeMapMinutesPerTurn,
			prepare: func(state *game_state.GameState) {
				state.PartyState.Inventory.Provisions.Gems.Set(1)
			},
			dismiss: func(t *testing.T, scene *GameScene) {
				dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.GemViewDialog)
				require.True(t, ok)
				dialog.Dismiss()
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := game_state.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(15, 15).
				WithRandomSeed(387).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
			}
			if test.prepare != nil {
				test.prepare(state)
			}
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)
			before := state.DateTime

			if test.location.GetMapType() == references.SmallMapType {
				scene.smallMapInputHandler(test.key)
			} else {
				scene.largeMapInputHandler(test.key)
			}

			require.True(t, state.EventQueueManager.HasWorkflow(test.workflowName))
			assert.Equal(t, before, state.DateTime)
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)

			state.EventQueueManager.Update()
			test.dismiss(t, scene)
			assert.Equal(t, before, state.DateTime)
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)

			drainSurfaceCompletionWorkflow(t, state, test.workflowName)
			assert.Equal(t, []int{test.wantMinutes}, callbacks.TimeAdvanced())
			assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
			assert.Zero(t, scene.dialogStack.Count())

			completedAt := state.DateTime
			state.ElapsedMs += DialogPollingRateMs
			state.EventQueueManager.Update()
			assert.Equal(t, completedAt, state.DateTime, "cleanup must not complete twice")
		})
	}
}

func TestDeferredSurfaceCompletionRunsAfterActionWorkflow(t *testing.T) {
	priorities := []struct {
		name     string
		priority int
	}{
		{name: "low", priority: event_queue.PriorityLow},
		{name: "normal", priority: event_queue.PriorityNormal},
		{name: "high", priority: event_queue.PriorityHigh},
		{name: "critical", priority: event_queue.PriorityCritical},
	}

	for _, test := range priorities {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := game_state.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithSystemCallbacks().
				Build()
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
			}
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)
			actionActive := true
			state.EventQueueManager.Workflow("surface_action").
				Priority(test.priority).
				Sequential().
				First(func() {}).
				While(func() bool { return actionActive }, func() {}, 1).
				Finally(func() {}).
				Start()
			scene.deferSurfaceCommandCompletion(
				newSurfaceCommandResult(surfaceCommandDeferred, references.SmallMapType),
			)

			state.EventQueueManager.Update()
			state.ElapsedMs++
			state.EventQueueManager.Update()
			assert.True(t, state.EventQueueManager.HasWorkflow("surface_action"))
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)

			actionActive = false
			drainSurfaceCompletionWorkflow(t, state, "surface_command_completion")
			assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
			assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
		})
	}
}

func TestDeferredSurfaceCompletionSurvivesShutdownBeforeFirstUpdate(t *testing.T) {
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)

	scene.deferSurfaceCommandCompletion(
		newSurfaceCommandResult(surfaceCommandDeferred, references.SmallMapType),
	)
	require.True(t, state.EventQueueManager.HasWorkflow("surface_command_completion"))
	require.NotPanics(t, state.EventQueueManager.Shutdown)

	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	state.EventQueueManager.Shutdown()
	assert.Len(t, callbacks.TimeAdvanced(), 1)
}

func TestSurfaceUseOwnershipCleansUpOnCancellationAndNoItemRejection(t *testing.T) {
	t.Run("cancellation", func(t *testing.T) {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithSystemCallbacks().
			Build()
		clearCombatUseTestInventory(state)
		clearCombatUseQuestItems(state)
		state.PartyState.Inventory.Potions.Set(references.Black, 1)
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
		}
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)

		scene.smallMapInputHandler(ebiten.KeyU)
		state.EventQueueManager.Update()
		dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ItemSelectionDialog)
		require.True(t, ok)
		assert.True(t, state.EventQueueManager.HasWorkflow("use_item_workflow"))
		assert.Empty(t, callbacks.TimeAdvanced())
		dialog.Dismiss()
		drainSurfaceCompletionWorkflow(t, state, "use_item_workflow")

		assert.False(t, state.EventQueueManager.HasWorkflow("use_item_workflow"))
		assert.Empty(t, callbacks.TimeAdvanced())
		assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
		require.NotPanics(t, func() { scene.smallMapInputHandler(ebiten.KeySpace) })
	})

	t.Run("no items", func(t *testing.T) {
		state, callbacks := game_state.NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithSystemCallbacks().
			Build()
		clearCombatUseTestInventory(state)
		clearCombatUseQuestItems(state)
		callbacks.Reset()
		scene := newSurfaceCompletionTestScene(state)

		scene.smallMapInputHandler(ebiten.KeyU)

		assert.False(t, state.EventQueueManager.HasWorkflow("use_item_workflow"))
		assert.Empty(t, callbacks.TimeAdvanced())
		require.NotPanics(t, func() { scene.smallMapInputHandler(ebiten.KeySpace) })
	})
}

func drainSurfaceCompletionWorkflow(t *testing.T, state *game_state.GameState, workflowName string) {
	t.Helper()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow(workflowName) {
			return
		}
	}
	t.Fatalf("workflow %q did not complete", workflowName)
}

func newSurfaceCompletionTestScene(state *game_state.GameState) *GameScene {
	return &GameScene{
		gameState:            state,
		keyboard:             input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:               text.NewOutput(nil, 1, 10, 80),
		spriteSheet:          &sprites.SpriteSheet{SpriteImage: ebiten.NewImage(512, 256)},
		autoOpenDoorState:    &AutoOpenDoorState{},
		moongateVisualSystem: NewMoongateVisualSystem(state),
	}
}
