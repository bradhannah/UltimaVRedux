package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestWishingWellEscapeCompletesWorkflowAndPreservesResources(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Paws).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(10)
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 10, 80),
	}
	position := findWishingWellWorkflowPosition(t, state)

	scene.ShowWellInteraction(&position)
	require.True(t, state.EventQueueManager.HasWorkflow(wishingWellWorkflowName))
	assert.True(t, callbacks.InputBlocked)
	require.NotPanics(t, func() { scene.ShowWellInteraction(&position) })
	assert.Equal(t, 1, callbacks.BlockInputCalls)

	dialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	assert.Contains(t, dialog.TranscriptText(), "Thou dost see a well.")
	assert.Equal(t, "Drop a coin?", dialog.ActivePrompt())
	answerWishingWellCoinPrompt(t, scene, dialog, ebiten.KeyY)
	waitForWishingWellPrompt(t, scene, dialog, "Thy wish?")
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	dialog.Update()

	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow(wishingWellWorkflowName) {
			break
		}
	}
	require.False(t, state.EventQueueManager.HasWorkflow(wishingWellWorkflowName))
	assert.False(t, scene.dialogStack.HasOpenDialog())
	assert.Equal(t, uint16(10), state.PartyState.Inventory.Gold.Get())
	assert.False(t, callbacks.InputBlocked)
}

func TestWishingWellZeroGoldYesCompletesAndReleasesInput(t *testing.T) {
	scene, callbacks := newWishingWellWorkflowTestScene(t, 0)
	position := findWishingWellWorkflowPosition(t, scene.gameState)

	scene.ShowWellInteraction(&position)
	dialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	answerWishingWellCoinPrompt(t, scene, dialog, ebiten.KeyY)
	acknowledgeWishingWell(t, scene, dialog)
	drainWishingWellWorkflow(t, scene)

	assert.Contains(t, dialog.TranscriptText(), "Drop a coin?  > Yes")
	assert.Contains(t, dialog.TranscriptText(), "Thou hast not enough gold!")
	assert.Contains(t, scene.output.GetOutputStr(false), "Thou hast not enough gold!")
	assert.Equal(t, uint16(0), scene.gameState.PartyState.Inventory.Gold.Get())
	assert.False(t, scene.dialogStack.HasOpenDialog())
	assert.False(t, scene.keyboard.IsWaitingForRelease())
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, 1, callbacks.BlockInputCalls)
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestWishingWellNoCompletesWithoutWishInput(t *testing.T) {
	scene, callbacks := newWishingWellWorkflowTestScene(t, 10)
	position := findWishingWellWorkflowPosition(t, scene.gameState)

	scene.ShowWellInteraction(&position)
	dialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	answerWishingWellCoinPrompt(t, scene, dialog, ebiten.KeyN)
	acknowledgeWishingWell(t, scene, dialog)
	drainWishingWellWorkflow(t, scene)

	assert.Contains(t, dialog.TranscriptText(), "Drop a coin?  > No")
	assert.Equal(t, uint16(10), scene.gameState.PartyState.Inventory.Gold.Get())
	assert.False(t, scene.dialogStack.HasOpenDialog())
	assert.False(t, scene.keyboard.IsWaitingForRelease())
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestWishingWellSubmittedWishCompletesAndOwnsDialog(t *testing.T) {
	scene, callbacks := newWishingWellWorkflowTestScene(t, 10)
	position := findWishingWellWorkflowPosition(t, scene.gameState)
	dateBefore := scene.gameState.DateTime

	scene.ShowWellInteraction(&position)
	dialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	require.Equal(t, 1, scene.dialogStack.Count())
	answerWishingWellCoinPrompt(t, scene, dialog, ebiten.KeyY)
	waitForWishingWellPrompt(t, scene, dialog, "Thy wish?")
	require.Same(t, dialog, scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, 1, scene.dialogStack.Count())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	acknowledgeWishingWell(t, scene, dialog)
	drainWishingWellWorkflow(t, scene)

	assert.Contains(t, dialog.TranscriptText(), "Thy wish?  >")
	assert.Contains(t, dialog.TranscriptText(), "No effect!")
	assert.Equal(t, uint16(9), scene.gameState.PartyState.Inventory.Gold.Get())
	assert.Equal(t, dateBefore, scene.gameState.DateTime)
	assert.Nil(t, scene.dialogStack.PeekTopModalDialog())
	assert.False(t, scene.keyboard.IsWaitingForRelease())
	assert.False(t, callbacks.InputBlocked)
	assert.Contains(t, scene.output.GetOutputStr(false), "No effect!")
}

func TestWishingWellLookOwnsOneDeferredSurfaceCycle(t *testing.T) {
	scene, callbacks := newWishingWellWorkflowTestScene(t, 10)
	position := findWishingWellWorkflowPosition(t, scene.gameState)
	scene.gameState.MapState.PlayerLocation.Position = position.Down()
	scene.gameState.SystemCallbacks.Flow.WorldInteraction.ShowWellInteraction = scene.ShowWellInteraction
	scene.secondaryKeyState = LookDirectionInput
	dateBefore := scene.gameState.DateTime
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)

	scene.smallMapHandleSecondaryInput()
	require.True(t, scene.gameState.EventQueueManager.HasWorkflow(wishingWellWorkflowName))
	require.True(t, scene.gameState.EventQueueManager.HasWorkflow("surface_command_completion"))
	assert.Equal(t, dateBefore, scene.gameState.DateTime)

	dialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	answerWishingWellCoinPrompt(t, scene, dialog, ebiten.KeyY)
	waitForWishingWellPrompt(t, scene, dialog, "Thy wish?")
	enterWishingWellText(t, scene, dialog, "DRAGON WISHX")
	assert.Equal(t, references.NoPartyVehicle, scene.gameState.PartyVehicle.GetVehicleDetails().VehicleType)
	assert.Equal(t, dateBefore, scene.gameState.DateTime)
	acknowledgeWishingWell(t, scene, dialog)
	drainWishingWellWorkflow(t, scene)
	for range 20 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow("surface_command_completion") {
			break
		}
	}

	expectedDate := dateBefore
	expectedDate.Advance(1)
	assert.Equal(t, expectedDate, scene.gameState.DateTime)
	assert.Equal(t, uint16(9), scene.gameState.PartyState.Inventory.Gold.Get())
	assert.False(t, callbacks.InputBlocked)
}

func TestSpellExpiredPopupCoalescesIdenticalLiveWorkflow(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
	}
	workflowName := "spell_expired_Time Stop"

	scene.showSpellExpiredPopup("Time Stop")
	require.True(t, state.EventQueueManager.HasWorkflow(workflowName))
	require.NotPanics(t, func() { scene.showSpellExpiredPopup("Time Stop") })
	assert.True(t, state.EventQueueManager.HasWorkflow(workflowName))
	assert.Equal(t, 0, callbacks.BlockInputCalls)
}

func TestShrineAndCodexWorkflowsCoalesceDuplicateStarts(t *testing.T) {
	tests := []struct {
		name         string
		workflowName string
		start        func(*GameScene)
	}{
		{name: "shrine", workflowName: "shrine_meditation", start: func(scene *GameScene) {
			scene.StartShrineWorkflow(0)
		}},
		{name: "Codex", workflowName: "codex_reading", start: func(scene *GameScene) {
			scene.StartCodexWorkflow()
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
			scene := &GameScene{
				gameState: state,
				keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
			}

			test.start(scene)
			require.NotPanics(t, func() { test.start(scene) })
			assert.True(t, state.EventQueueManager.HasWorkflow(test.workflowName))
			assert.Equal(t, 1, callbacks.BlockInputCalls)
		})
	}
}

func TestFailedDeathWorkflowRestoresEntityVisibility(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
	}
	scene.HideNonAvatarEntities()

	scene.StartDeathResurrectionWorkflow()
	for range 5 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}

	assert.False(t, state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName))
	assert.False(t, scene.hideMapUnits)
	assert.False(t, scene.hideQuestItems)
	assert.False(t, scene.hideItemStacks)
	assert.False(t, callbacks.InputBlocked)
}

func TestDeathWorkflowReleasesAmbientOwnershipOnShutdown(t *testing.T) {
	for _, initiallyHidden := range []bool{false, true} {
		name := "visible"
		if initiallyHidden {
			name = "hidden"
		}
		t.Run(name, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
			for _, character := range state.ActivePartyCharacters() {
				state.PartyState.GetCharacter(character.Slot).SetStatus(party_state.Dead)
			}
			scene := &GameScene{
				gameState: state,
				keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
			}
			if initiallyHidden {
				scene.HideAmbientSounds()
			}
			scene.StartDeathResurrectionWorkflow()
			state.EventQueueManager.Update()
			require.True(t, scene.hideAmbientSounds, "death presentation owns ambient sound")
			state.EventQueueManager.Shutdown()
			require.Equal(t, initiallyHidden, scene.hideAmbientSounds)
		})
	}
}

func TestStonegateKeepsAlreadyLiveDeathWorkflow(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Stonegate).
		WithPlayerAt(15, 16).
		WithSystemCallbacks().
		Build()
	require.True(t, state.GetCurrentAvatarGroundTile().Is(indexes.BrickFloorHole))
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 10, 80),
	}

	review, err := state.ReviewStonegateTrap()
	require.NoError(t, err)
	outcome, err := state.CommitStonegateTrap(review.Fingerprint)
	require.NoError(t, err)
	scene.StartStonegateTrapWorkflow(outcome)
	state.EventQueueManager.Update()
	// Simulate a death workflow already queued independently of the guarded
	// callback; Stonegate must retain it rather than create a duplicate handoff.
	scene.startDeathResurrectionWorkflow()
	for update := 0; update < 100 && state.EventQueueManager.HasWorkflow("stonegate_trap_workflow"); update++ {
		state.ElapsedMs += 1000
		require.NotPanics(t, state.EventQueueManager.Update)
	}

	assert.False(t, state.EventQueueManager.HasWorkflow("stonegate_trap_workflow"))
	assert.True(t, state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName))
}

func newWishingWellWorkflowTestScene(t *testing.T, gold uint16) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Paws).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(gold)

	return &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 10, 80),
	}, callbacks
}

func answerWishingWellCoinPrompt(
	t *testing.T,
	scene *GameScene,
	dialog *widgets.InteractionTranscript,
	answer ebiten.Key,
) {
	t.Helper()
	scene.keyboard.SimulateKeyPress(answer)
	dialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
}

func waitForWishingWellPrompt(
	t *testing.T,
	scene *GameScene,
	dialog *widgets.InteractionTranscript,
	prompt string,
) {
	t.Helper()
	for range 20 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if dialog.ActivePrompt() == prompt && !dialog.InputComplete() {
			return
		}
	}
	require.Equal(t, prompt, dialog.ActivePrompt())
}

func acknowledgeWishingWell(t *testing.T, scene *GameScene, dialog *widgets.InteractionTranscript) {
	t.Helper()
	for range 20 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if dialog.ActivePrompt() == "" && !dialog.InputComplete() {
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			dialog.Update()

			return
		}
	}
	require.False(t, dialog.InputComplete(), "wishing-well acknowledgement was not shown")
}

func enterWishingWellText(
	t *testing.T,
	scene *GameScene,
	dialog *widgets.InteractionTranscript,
	value string,
) {
	t.Helper()
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	for _, character := range strings.ToUpper(value) {
		scene.keyboard.ClearFrameConsumption()
		if character == ' ' {
			scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
			dialog.Update()

			continue
		}
		if character < 'A' || character > 'Z' {
			t.Fatalf("unsupported wishing-well test character %q", character)
		}
		scene.keyboard.SimulateKeyPress(ebiten.KeyA + ebiten.Key(character-'A'))
		dialog.Update()
	}
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
}

func drainWishingWellWorkflow(t *testing.T, scene *GameScene) {
	t.Helper()
	for range 20 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow(wishingWellWorkflowName) {
			return
		}
	}
	require.False(t, scene.gameState.EventQueueManager.HasWorkflow(wishingWellWorkflowName))
}

func findWishingWellWorkflowPosition(t *testing.T, state *game_state.GameState) references.Position {
	t.Helper()
	for y := range 64 {
		for x := range 64 {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			if state.DetectWishingWellAtPosition(&position) {
				return position
			}
		}
	}
	t.Fatal("no wishing well found in workflow test map")

	return references.Position{}
}
