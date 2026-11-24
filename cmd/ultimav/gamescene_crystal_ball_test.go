package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestCrystalBallRetainsFramingTranscriptUntilGemView(t *testing.T) {
	scene, callbacks := newCrystalBallWorkflowTestScene(t)
	dateBefore := scene.gameState.DateTime

	scene.StartCrystalBallWorkflow()
	transcript, selector := waitForCrystalBallSelector(t, scene)
	require.Equal(t, 2, scene.dialogStack.Count())
	require.Same(t, transcript, scene.dialogStack.Dialogs()[0])
	option := selector.GetCurrentOption()
	scene.gameState.PartyState.GetCharacter(option.CharacterIndex).Intelligence = 99
	selector.HandleEnter()

	waitForCrystalBallTranscriptResult(t, scene, transcript, "Strange vision!", "[Enter/Esc] Continue")
	require.Contains(t, transcript.TranscriptText(), option.Name+" looks into the crystal ball.")
	require.NotContains(t, transcript.TranscriptText(), "Who will look?")
	acknowledgeTranscript(t, scene, transcript)
	gemView := waitForTopModalWithin[*widgets.GemViewDialog](t, scene, 1000, 100)
	require.NotContains(t, scene.dialogStack.Dialogs(), transcript)
	gemView.Dismiss()
	drainSceneWorkflow(t, scene, "crystal_ball_workflow", 1000)

	expectedDate := dateBefore
	expectedDate.Advance(1)
	require.Equal(t, expectedDate, scene.gameState.DateTime)
	require.Equal(t, []int{1}, callbacks.TimeAdvanced())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
}

func TestCrystalBallDeathVisionRetainsResultUntilAcknowledged(t *testing.T) {
	scene, callbacks := newCrystalBallWorkflowTestScene(t)

	scene.StartCrystalBallWorkflow()
	transcript, selector := waitForCrystalBallSelector(t, scene)
	option := selector.GetCurrentOption()
	character := scene.gameState.PartyState.GetCharacter(option.CharacterIndex)
	character.Intelligence = 0
	hpBefore := character.GetCurrentHp()
	selector.HandleEnter()

	waitForCrystalBallTranscriptResult(t, scene, transcript, "Death vision!", "[Enter/Esc] Close")
	require.Equal(t, hpBefore-1, character.GetCurrentHp())
	require.Contains(t, transcript.TranscriptText(), option.Name+" looks into the crystal ball.")
	require.NotContains(t, transcript.TranscriptText(), "Who will look?")
	require.Contains(t, callbacks.SoundEffectsPlayed, game_state.SoundDeathVision)
	acknowledgeTranscript(t, scene, transcript)
	drainSceneWorkflow(t, scene, "crystal_ball_workflow", 1000)

	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestCrystalBallCancellationCleansTranscript(t *testing.T) {
	scene, callbacks := newCrystalBallWorkflowTestScene(t)
	rngCalls := scene.gameState.GetRNG().Calls()
	hpBefore := scene.gameState.PartyState.GetCharacter(0).GetCurrentHp()

	scene.StartCrystalBallWorkflow()
	_, selector := waitForCrystalBallSelector(t, scene)
	selector.HandleEscape()
	drainSceneWorkflow(t, scene, "crystal_ball_workflow", 1000)

	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Equal(t, []int{1}, callbacks.TimeAdvanced())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.Equal(t, rngCalls, scene.gameState.GetRNG().Calls())
	require.Equal(t, hpBefore, scene.gameState.PartyState.GetCharacter(0).GetCurrentHp())
	require.Empty(t, callbacks.SoundEffectsPlayed)
}

func TestCrystalBallUsesEligibleActiveViewerWithoutSelector(t *testing.T) {
	for _, status := range []party_state.CharacterStatus{party_state.Good, party_state.Poisoned} {
		for _, success := range []bool{false, true} {
			name := characterSelectionStatus(status) + "/Death"
			if success {
				name = characterSelectionStatus(status) + "/Strange"
			}
			t.Run(name, func(t *testing.T) {
				scene, callbacks := newCrystalBallWorkflowTestScene(t)
				state := scene.gameState
				state.SetRandomSeed(499)
				avatar := state.PartyState.GetCharacter(0)
				viewer := state.PartyState.GetCharacter(1)
				viewer.SetStatus(status)
				avatar.Intelligence, viewer.Intelligence = 99, 0
				result, hint := "Death vision!", "[Enter/Esc] Close"
				if success {
					avatar.Intelligence, viewer.Intelligence = 0, 99
					result, hint = "Strange vision!", "[Enter/Esc] Continue"
				}
				require.True(t, state.SetActivePlayer(1))
				hpBefore := make([]uint16, state.PartyState.GetCharacterCount())
				for index := range hpBefore {
					hpBefore[index] = state.PartyState.GetCharacter(index).GetCurrentHp()
				}
				target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.CrystalBall)
				state.SystemCallbacks.Flow.Observation.ShowCrystalBallInteraction = scene.ShowCrystalBallInteraction
				require.True(t, state.ActionLookSmallMap(references.Right))
				var transcript *widgets.InteractionTranscript
				for range 100 {
					state.ElapsedMs += 1000
					state.EventQueueManager.Update()
					_, selecting := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
					require.False(t, selecting, "an eligible active viewer must bypass selection")
					if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok {
						transcript = dialog
						break
					}
				}
				require.NotNil(t, transcript)
				waitForCrystalBallTranscriptResult(t, scene, transcript, result, hint)
				require.Contains(t, transcript.TranscriptText(), viewer.GetNameAsString()+" looks into the crystal ball.")
				require.NotContains(t, transcript.TranscriptText(), avatar.GetNameAsString()+" looks into the crystal ball.")
				for index, hp := range hpBefore {
					if index == 1 && !success {
						hp--
					}
					require.Equal(t, hp, state.PartyState.GetCharacter(index).GetCurrentHp())
				}
				wantCalls := uint64(1)
				if !success {
					wantCalls++ // The existing damage builder draws even for fixed damage.
				}
				require.Equal(t, wantCalls, state.GetRNG().Calls())
				acknowledgeTranscript(t, scene, transcript)
				if success {
					waitForTopModalWithin[*widgets.GemViewDialog](t, scene, 1000, 100).Dismiss()
				}
				drainSceneWorkflow(t, scene, "crystal_ball_workflow", 1000)
				require.False(t, scene.dialogStack.HasOpenDialog())
				require.Equal(t, []int{1}, callbacks.TimeAdvanced())
				require.Equal(t, 1, callbacks.BlockInputCalls)
				require.Equal(t, 1, callbacks.UnblockInputCalls)
			})
		}
	}
}

func TestCrystalBallClearsIneligibleActiveViewerBeforeFallback(t *testing.T) {
	for _, status := range []party_state.CharacterStatus{party_state.Sleep, party_state.Dead, party_state.Charmed} {
		t.Run(characterSelectionStatus(status), func(t *testing.T) {
			scene, callbacks := newCrystalBallWorkflowTestScene(t)
			state := scene.gameState
			viewer := state.PartyState.GetCharacter(1)
			viewer.SetStatus(party_state.Good)
			require.True(t, state.SetActivePlayer(1))
			viewer.SetStatus(status)
			callbacks.Reset()
			rngCalls := state.GetRNG().Calls()

			scene.StartCrystalBallWorkflow()
			_, selector := waitForCrystalBallSelector(t, scene)
			require.Equal(t, -1, state.GetActivePlayerIndex())
			require.NotEqual(t, 1, selector.GetCurrentOption().CharacterIndex)
			selector.HandleEscape()
			drainSceneWorkflow(t, scene, "crystal_ball_workflow", 1000)
			require.Equal(t, rngCalls, state.GetRNG().Calls())
			require.False(t, scene.dialogStack.HasOpenDialog())
			require.Equal(t, []int{1}, callbacks.TimeAdvanced())
			require.Equal(t, 1, callbacks.UnblockInputCalls)
		})
	}
}

func newCrystalBallWorkflowTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:      text.NewOutput(nil, 1, 20, 80),
		spriteSheet: &sprites.SpriteSheet{},
	}, callbacks
}

func waitForCrystalBallSelector(
	t *testing.T,
	scene *GameScene,
) (*widgets.InteractionTranscript, *widgets.CharacterSelectionDialog) {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		selector, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
		if !ok || scene.dialogStack.Count() != 2 {
			continue
		}
		transcript, ok := scene.dialogStack.Dialogs()[0].(*widgets.InteractionTranscript)
		if ok {
			return transcript, selector
		}
	}
	t.Fatal("crystal-ball selector was not shown above its transcript")

	return nil, nil
}

func waitForCrystalBallTranscriptResult(
	t *testing.T,
	scene *GameScene,
	transcript *widgets.InteractionTranscript,
	content string,
	hint string,
) {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		_, selecting := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
		require.False(t, selecting, "viewer selection must be resolved before the vision")
		if scene.dialogStack.PeekTopModalDialog() == transcript &&
			transcript.ActivePrompt() == "" && !transcript.InputComplete() &&
			transcript.ControlHint() == hint &&
			strings.Contains(transcript.TranscriptText(), content) {
			return
		}
	}
	t.Fatalf("crystal-ball transcript result %q was not shown", content)
}
