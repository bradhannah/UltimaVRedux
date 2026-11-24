package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSkullKeyWorkflowPassPreservesInventoryAndTime(t *testing.T) {
	t.Parallel()

	scene, callbacks := newUseAuthorityTestScene(t)
	scene.gameState.PartyState.Inventory.Provisions.SkullKeys.Set(1)
	commit, review := scene.gameState.BeginSkullKeyUse()
	require.Equal(t, game_state.SkullKeyUseReady, review.Result)
	require.NotNil(t, commit)

	scene.StartSkullKeyWorkflow(commit)
	waitForUseAuthorityDialog(t, scene)
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.Provisions.SkullKeys.Get())
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	drainSceneWorkflow(t, scene, "skull_key_workflow", 1000)

	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestCarpetWorkflowStaleCommitDoesNotPartiallyBoard(t *testing.T) {
	t.Parallel()

	scene, callbacks := newUseAuthorityTestScene(t)
	scene.gameState.PartyState.Inventory.QuestItems.Set(references.Carpet, 1)
	commit, review := scene.gameState.BeginCarpetBoarding()
	require.Equal(t, game_state.CarpetBoardingReady, review.Result)
	require.NotNil(t, commit)
	scene.gameState.PartyState.Inventory.QuestItems.Set(references.Carpet, 2)

	scene.StartCarpetBoardingWorkflow(commit)
	drainSceneWorkflow(t, scene, "carpet_boarding", 1000)

	assert.Equal(t, uint16(2), scene.gameState.PartyState.Inventory.QuestItems.Get(references.Carpet))
	assert.Equal(t, references.NoPartyVehicle, scene.gameState.PartyVehicle.GetVehicleDetails().VehicleType)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.NotContains(t, callbacks.SoundEffectsPlayed, game_state.SoundSummon)
}

func newUseAuthorityTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(12, 12).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet: &sprites.SpriteSheet{},
		output:      text.NewOutput(nil, 1, 10, 80),
	}, callbacks
}

func waitForUseAuthorityDialog(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 10 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if _, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.MessageDialog); ok {
			return
		}
	}
	t.Fatal("Use authority direction dialog did not open")
}
