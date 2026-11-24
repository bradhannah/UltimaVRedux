package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

const (
	potionTestRandomEffectRollMax = 16
	potionTestRandomEffectSleep   = 0
)

func TestPotionColorsRouteToOriginalEffects(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		potion references.Potion
	}{
		{"Blue", references.Blue},
		{"Yellow", references.Yellow},
		{"Red", references.Red},
		{"Green", references.Green},
		{"Orange", references.Orange},
		{"Purple", references.Purple},
		{"Black", references.Black},
		{"White", references.White},
	}
	scene := newPotionTestScene(t)

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene.gameState.PartyState.Inventory.Potions.Set(test.potion, 1)

			startTestPotionWorkflow(t, scene, test.potion)
			scene.gameState.EventQueueManager.Update()

			assert.True(t, scene.gameState.EventQueueManager.HasWorkflow("potion_workflow"))
			assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.Potions.Get(test.potion))
			dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
			require.True(t, ok)
			dialog.HandleEscape()
			drainPotionWorkflow(t, scene)
		})
	}
}

func TestRandomPotionFailureForcesOrangeSleepEffect(t *testing.T) {
	t.Parallel()

	scene := newPotionTestScene(t)
	seed := findPotionRollSeed(t, scene.gameState, func(roll int) bool { return roll == potionTestRandomEffectSleep })
	scene.gameState.SetRandomSeed(seed)

	scene.gameState.PartyState.Inventory.Potions.Set(references.Blue, 1)
	commit, _ := scene.gameState.BeginPotionUse(references.Blue, game_state.UsageField)
	outcome, committed := scene.gameState.CommitPotionUse(commit)
	require.True(t, committed)
	assert.Equal(t, game_state.PotionEffectSlept, scene.gameState.CommitPotionEffect(outcome, 0))

	assert.Equal(t, references.Orange, outcome.FinalPotion)
	assert.True(t, outcome.Mislabelled)

	scene.gameState.SetRandomSeed(seed)
	scene.gameState.PartyState.Inventory.Potions.Set(references.Orange, 1)
	scene.gameState.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	commit, _ = scene.gameState.BeginPotionUse(references.Orange, game_state.UsageField)
	outcome, committed = scene.gameState.CommitPotionUse(commit)
	require.True(t, committed)
	assert.Equal(t, game_state.PotionEffectSlept, scene.gameState.CommitPotionEffect(outcome, 0))
	assert.Equal(t, references.Orange, outcome.FinalPotion)
	assert.False(t, outcome.Mislabelled)
}

func TestOrangePotionAppliesSleep(t *testing.T) {
	t.Parallel()

	scene := newPotionTestScene(t)
	seed := findPotionRollSeed(t, scene.gameState, func(roll int) bool { return roll >= 2 })
	scene.gameState.SetRandomSeed(seed)
	scene.gameState.PartyState.Inventory.Potions.Set(references.Orange, 1)
	target := scene.gameState.PartyState.GetCharacter(0)
	require.True(t, target.IsGood())

	startTestPotionWorkflow(t, scene, references.Orange)
	scene.gameState.EventQueueManager.Update()
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleEnter()

	for range 10 {
		scene.gameState.ElapsedMs += KeyboardDebounceShortMs + DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if target.IsAsleep() {
			break
		}
	}

	assert.True(t, target.IsAsleep())
	assert.Zero(t, scene.gameState.PartyState.Inventory.Potions.Get(references.Orange))
}

func TestBluePotionAllowsSleepingTargetAndConsumesOnConfirmation(t *testing.T) {
	t.Parallel()

	scene := newPotionTestScene(t)
	seed := findPotionRollSeed(t, scene.gameState, func(roll int) bool { return roll >= 2 })
	scene.gameState.SetRandomSeed(seed)
	scene.gameState.PartyState.Inventory.Potions.Set(references.Blue, 1)
	target := scene.gameState.PartyState.GetCharacter(1)
	target.PartyMap = party_state.PartyMapActive
	target.SetStatus(party_state.Sleep)

	startTestPotionWorkflow(t, scene, references.Blue)
	scene.gameState.EventQueueManager.Update()
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleNumber(2)
	require.Equal(t, 1, dialog.GetCurrentSelection())
	require.Equal(t, widgets.CharacterSelectionAvailable, dialog.GetCurrentOption().State)
	dialog.HandleEnter()
	for range 20 {
		scene.gameState.ElapsedMs += KeyboardDebounceShortMs + DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if target.IsGood() {
			break
		}
	}

	assert.True(t, target.IsGood())
	assert.Zero(t, scene.gameState.PartyState.Inventory.Potions.Get(references.Blue))
	require.NotPanics(t, scene.gameState.EventQueueManager.Shutdown)
}

func TestPotionCharacterSelectionCancellationPreservesInventoryAndDoesNotApplyEffect(t *testing.T) {
	t.Parallel()

	scene := newPotionTestScene(t)
	seed := findPotionRollSeed(t, scene.gameState, func(roll int) bool { return roll >= 2 })
	scene.gameState.SetRandomSeed(seed)
	scene.gameState.PartyState.Inventory.Potions.Set(references.Green, 1)
	target := scene.gameState.PartyState.GetCharacter(0)
	require.True(t, target.IsGood())

	startTestPotionWorkflow(t, scene, references.Green)
	scene.gameState.EventQueueManager.Update()
	rngCallsBeforeCancel := scene.gameState.GetRNG().Calls()
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleEscape()
	for range 10 {
		scene.gameState.ElapsedMs += KeyboardDebounceShortMs + DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow("potion_workflow") {
			break
		}
	}

	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.Potions.Get(references.Green))
	assert.True(t, target.IsGood())
	assert.Equal(t, rngCallsBeforeCancel, scene.gameState.GetRNG().Calls())
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow("potion_workflow"))
}

func TestDuplicatePotionWorkflowDoesNotConsumeEitherCommit(t *testing.T) {
	t.Parallel()

	scene := newPotionTestScene(t)
	scene.gameState.PartyState.Inventory.Potions.Set(references.Blue, 1)
	scene.gameState.PartyState.Inventory.Potions.Set(references.Red, 1)
	first, _ := scene.gameState.BeginPotionUse(references.Blue, game_state.UsageField)
	second, _ := scene.gameState.BeginPotionUse(references.Red, game_state.UsageField)

	scene.StartPotionWorkflow(first)
	assert.Panics(t, func() { scene.StartPotionWorkflow(second) })

	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.Potions.Get(references.Blue))
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.Potions.Get(references.Red))
}

func newPotionTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet: &sprites.SpriteSheet{},
	}
}

func startTestPotionWorkflow(t *testing.T, scene *GameScene, potion references.Potion) {
	t.Helper()
	commit, review := scene.gameState.BeginPotionUse(potion, game_state.UsageField)
	require.True(t, review.Ready)
	require.NotNil(t, commit)
	scene.StartPotionWorkflow(commit)
}

func findPotionRollSeed(t *testing.T, state *game_state.GameState, matches func(int) bool) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 1000; seed++ {
		state.SetRandomSeed(seed)
		if matches(state.RandomIntInRange(0, potionTestRandomEffectRollMax-1)) {
			return seed
		}
	}
	require.FailNow(t, "no deterministic potion seed found")

	return 0
}

func drainPotionWorkflow(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 10 {
		scene.gameState.ElapsedMs += KeyboardDebounceShortMs + DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow("potion_workflow") {
			return
		}
	}
	require.False(t, scene.gameState.EventQueueManager.HasWorkflow("potion_workflow"))
}
