package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/semantic_input"
)

func TestShrineQuestCompletionShowsGuidanceOnlyAtAcknowledgement(t *testing.T) {
	callbacks := game_state.NewMockSystemCallbacks(t)
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	for index := range roster {
		roster[index] = new(party_state.PlayerCharacter)
	}
	state := &game_state.GameState{
		SystemCallbacks: callbacks.ToSystemCallbacks(),
		PartyState:      *party_state.NewPartyStateFromRoster(roster),
	}
	state.EventQueueManager = event_queue.NewEventQueueManager(
		func() int64 { return state.ElapsedMs }, &dialogWorkflowTestInput{callbacks: state.SystemCallbacks},
	)
	scene := &GameScene{gameState: state}
	pressed := false
	scene.gameState.SystemCallbacks.Visual.IsAnyKeyPressed = func() bool { return pressed }
	const message = "Thou hast completed thy quest."
	const workflow = "shrine_completion_guidance"
	scene.gameState.EventQueueManager.Workflow(workflow).
		Priority(event_queue.PriorityNormal).Sequential().BlockInput().
		First(func() {}).ThenSequence(
		scene.thenShowShrineQuestCompletionEffects(message, game_state.DeedExperienceAward{})).Start()
	dialog := waitForTopModalWithin[*widgets.MessageDialog](t, scene, 1000, 200)
	expected := widgets.NewMessageDialog(message).WithAutoSizeDefault().WithSeparatePromptArea().
		WithContentTextAlignment(widgets.ContentAlignCenter).WithEffects(false).Build()
	expected.ShowWithoutAnimation()

	// Compare actual rendering without wall-clock dissolve animation. The reference
	// starts footerless and gains the shared hint only once the workflow can accept input.
	width, height := display.GetManager().GetCurrentSize()
	actualScreen, expectedScreen := ebiten.NewImage(width, height), ebiten.NewImage(width, height)
	actualPixels, expectedPixels := make([]byte, width*height*4), make([]byte, width*height*4)
	assertPresentation := func() {
		t.Helper()
		dialog.ShowWithoutAnimation()
		actualScreen.Clear()
		expectedScreen.Clear()
		dialog.Draw(actualScreen)
		expected.Draw(expectedScreen)
		actualScreen.ReadPixels(actualPixels)
		expectedScreen.ReadPixels(expectedPixels)
		require.True(t, bytes.Equal(actualPixels, expectedPixels), "shrine acknowledgement presentation differs")
	}
	assertPresentation()
	for range 100 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if callbacks.StopRequestCount(game_state.SoundEarthquake) > 0 {
			break
		}
	}
	require.Positive(t, callbacks.StopRequestCount(game_state.SoundEarthquake))
	require.True(t, callbacks.InputBlocked)
	expected.SetAcknowledgementPrompt(widgets.DialogAnyKeyHint)
	assertPresentation()

	scene.gameState.EventQueueManager.Update()
	pressed = true
	scene.gameState.EventQueueManager.Update()
	pressed = false
	drainSceneWorkflow(t, scene, workflow, 1000)
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.False(t, callbacks.InputBlocked)
}

type dialogWorkflowTestInput struct {
	callbacks *game_state.SystemCallbacks
}

func (d *dialogWorkflowTestInput) BlockAllInput(blocked bool) {
	d.callbacks.Visual.BlockAllInput(blocked)
}

func (d *dialogWorkflowTestInput) IsAnyKeyPressed() bool {
	return d.callbacks.Visual.IsAnyKeyPressed()
}

func (d *dialogWorkflowTestInput) GetPressedKey() *semantic_input.Key {
	return d.callbacks.Visual.GetPressedKey()
}

func TestShrineMeditationRetainsTranscriptThroughThreeMantras(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)

	scene.StartShrineWorkflow(0)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, "HONE")
	for _, mantra := range []string{"AHM", "WRONG", "AHM"} {
		require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Mantra:"))
		enterTranscriptText(t, scene, transcript, mantra)
	}

	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "Thy thoughts are unfocused."))
	transcriptText := strings.Join(strings.Fields(transcript.TranscriptText()), " ")
	require.Contains(t, transcriptText, "Virtue? > HONE")
	require.Equal(t, 3, strings.Count(transcriptText, "Mantra: >"))
	require.Contains(t, transcriptText, "Thy thoughts are unfocused.")
	acknowledgeTranscript(t, scene, transcript)
	drainSceneWorkflow(t, scene, "shrine_meditation", 1000)

	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
}

func TestShrineMeditationEmptyOrEscapeExitsWithoutUnfocusedFeedback(t *testing.T) {
	for stage := range 4 {
		for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
			t.Run(fmt.Sprintf("stage_%d/key_%d", stage, key), func(t *testing.T) {
				scene, callbacks := newShrineTranscriptTestScene(t)
				scene.StartShrineWorkflow(0)
				transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
				if stage > 0 {
					enterTranscriptText(t, scene, transcript, "HONESTY")
					waitForTranscriptPrompt(t, scene, "Mantra:")
				}
				for range stage - 1 {
					enterTranscriptText(t, scene, transcript, "WRONG")
					waitForTranscriptPrompt(t, scene, "Mantra:")
				}
				scene.keyboard.ClearFrameConsumption()
				scene.keyboard.SetForceWaitAnyKey(-1000)
				scene.keyboard.SimulateKeyPress(key)
				transcript.Update()
				require.True(t, transcript.InputComplete())
				drainSceneWorkflow(t, scene, shrineMeditationWorkflowName, 1000)
				require.NotContains(t, transcript.TranscriptText(), "unfocused")
				require.False(t, scene.gameState.IsShrineQuestActive(0))
				require.False(t, scene.dialogStack.HasOpenDialog())
				require.Nil(t, scene.miniMapScene)
				require.Equal(t, 1, callbacks.UnblockInputCalls)
			})
		}
	}
}

func TestShrineAssignmentSendsVisitorToCodexForOpposedVice(t *testing.T) {
	scene, _ := newShrineTranscriptTestScene(t)
	const shrineIndex = 2

	scene.StartShrineWorkflow(shrineIndex)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, "VALOUR")
	for range game_state.MantraRepetitions {
		waitForTranscriptPrompt(t, scene, "Mantra:")
		enterTranscriptText(t, scene, transcript, "RA")
	}

	assignment := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
	message := strings.Join(strings.Fields(assignment.GetMessage()), " ")
	require.Contains(t, message, "go unto the Codex and learn the failing of a life without Valour!")
	require.NotContains(t, message, "VERAMOCOR")
	assignment.Dismiss()
	followup := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
	require.Contains(t, followup.GetMessage(), "Return again when thy Quest is done!")
	followup.Dismiss()
	drainSceneWorkflow(t, scene, "shrine_meditation", 1000)
}

func TestCompassionCodexKeepsWordingTranscriptQuestAndKeyboardDismissal(t *testing.T) {
	const wisdom = "Only a detested life owes its pleasures to another's pain."
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
		t.Run(fmt.Sprint(key), func(t *testing.T) {
			scene, callbacks := newShrineTranscriptTestScene(t)
			for index := range game_state.TotalShrines {
				scene.gameState.SetShrineQuestActive(index, index == 1)
				scene.gameState.SetShrineQuestCompleted(index, false)
			}
			scene.StartCodexWorkflow()
			dismissNextShrineOverlay(t, scene)
			answer := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
			require.Contains(t, answer.GetMessage(), "\n"+wisdom)
			require.Contains(t, callbacks.Messages, wisdom)
			require.True(t, scene.gameState.IsShrineQuestCompleted(1))
			require.True(t, scene.gameState.IsShrineQuestActive(1), "return to the shrine still owns the reward")
			require.False(t, scene.gameState.IsShrineQuestCompleted(0))
			scene.keyboard.ClearFrameConsumption()
			scene.keyboard.SetForceWaitAnyKey(-1000)
			scene.keyboard.SimulateKeyPress(key)
			answer.Update()
			require.True(t, answer.IsDismissed())
			drainSceneWorkflow(t, scene, "codex_reading", 1000)
			require.False(t, scene.dialogStack.HasOpenDialog())
			require.False(t, callbacks.InputBlocked)
		})
	}
}

func TestFinalCodexCompletionPresentsSourceBackedRevelation(t *testing.T) {
	scene, _ := newShrineTranscriptTestScene(t)
	for index := range game_state.TotalShrines - 1 {
		scene.gameState.SetShrineQuestCompleted(index, true)
		scene.gameState.SetShrineQuestActive(index, false)
	}
	lastQuest := game_state.TotalShrines - 1
	scene.gameState.SetShrineQuestCompleted(lastQuest, false)
	scene.gameState.SetShrineQuestActive(lastQuest, true)

	scene.StartCodexWorkflow()
	dismissNextShrineOverlay(t, scene) // The book opens.
	answer := dismissNextShrineOverlay(t, scene)
	require.NotContains(t, answer, "VERAMOCOR")

	revelation := make([]string, 0, 5)
	for range 5 {
		revelation = append(revelation, dismissNextShrineOverlay(t, scene))
	}
	drainSceneWorkflow(t, scene, "codex_reading", 1000)

	transcript := strings.Join(revelation, " ")
	require.Contains(t, transcript, "SHAME'S EGRESS")
	require.Contains(t, transcript, "CENTRE OF THE UNDERWORLD")
	require.Contains(t, transcript, "GATE TO THE CORE OF THE WORLD")
	require.Contains(t, transcript, "VERAMOCOR")
	require.Contains(t, transcript, "ETHEREAL WARDS")
	require.Contains(t, transcript, "STEALERS OF SOULS")
	require.True(t, scene.gameState.AreAllQuestsComplete())
}

func TestShrineOfferingRetainsTranscriptThroughPaymentResult(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	const shrineIndex = 0
	scene.gameState.SetShrineQuestCompleted(shrineIndex, true)
	scene.gameState.SetShrineQuestActive(shrineIndex, false)
	scene.gameState.PartyState.Inventory.Gold.Set(500)

	scene.StartShrineWorkflow(shrineIndex)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, "HONESTY")
	for range game_state.MantraRepetitions {
		require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Mantra:"))
		enterTranscriptText(t, scene, transcript, "AHM")
	}
	require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Hundredweights?"))
	enterTranscriptText(t, scene, transcript, "3")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "ALAKAZAM!"))

	transcriptText := strings.Join(strings.Fields(transcript.TranscriptText()), " ")
	require.Contains(t, transcriptText, "Hundredweights? > 3")
	require.Contains(t, transcriptText, "ALAKAZAM!")
	require.Equal(t, uint16(200), scene.gameState.PartyState.Inventory.Gold.Get())
	require.Contains(t, callbacks.SoundEffectsPlayed, game_state.SoundShrineOffering)
	acknowledgeTranscript(t, scene, transcript)
	drainSceneWorkflow(t, scene, "shrine_meditation", 1000)

	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
}

func TestShrineOfferingCancellationRetainsGoldAndRemovesTranscriptBeforeWalkOut(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	const shrineIndex = 0
	scene.gameState.SetShrineQuestCompleted(shrineIndex, true)
	scene.gameState.SetShrineQuestActive(shrineIndex, false)
	scene.gameState.PartyState.Inventory.Gold.Set(500)

	scene.StartShrineWorkflow(shrineIndex)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, "HONESTY")
	for range game_state.MantraRepetitions {
		require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Mantra:"))
		enterTranscriptText(t, scene, transcript, "AHM")
	}
	require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Hundredweights?"))
	enterTranscriptText(t, scene, transcript, "0")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "Thou dost leave the shrine."))
	require.Equal(t, uint16(500), scene.gameState.PartyState.Inventory.Gold.Get())

	acknowledgeTranscript(t, scene, transcript)
	scene.gameState.ElapsedMs += 1000
	scene.gameState.EventQueueManager.Update()
	require.False(t, scene.dialogStack.HasOpenDialog())
	drainSceneWorkflow(t, scene, "shrine_meditation", 1000)

	require.Equal(t, uint16(500), scene.gameState.PartyState.Inventory.Gold.Get())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestShrineOfferingInsufficientGoldRepromptsAndAcceptsAffordableRetry(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	const shrineIndex = 0
	scene.gameState.SetShrineQuestCompleted(shrineIndex, true)
	scene.gameState.SetShrineQuestActive(shrineIndex, false)
	scene.gameState.PartyState.Inventory.Gold.Set(100)
	originalLocation := scene.gameState.MapState.PlayerLocation
	expectedDate := scene.gameState.DateTime
	expectedDate.Advance(game_state.ShrineTimeCostMinutes)

	scene.StartShrineWorkflow(shrineIndex)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, "HONESTY")
	for range game_state.MantraRepetitions {
		waitForTranscriptPrompt(t, scene, "Mantra:")
		enterTranscriptText(t, scene, transcript, "AHM")
	}
	require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Hundredweights?"))
	enterTranscriptText(t, scene, transcript, "3")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "Thou hast not that much gold!"))

	require.Contains(t, strings.Join(strings.Fields(transcript.TranscriptText()), " "),
		"Thou hast not that much gold!")
	require.Equal(t, uint16(100), scene.gameState.PartyState.Inventory.Gold.Get())
	acknowledgeTranscript(t, scene, transcript)
	require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Hundredweights?"))
	enterTranscriptText(t, scene, transcript, "1")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "ALAKAZAM!"))
	require.Equal(t, uint16(0), scene.gameState.PartyState.Inventory.Gold.Get())
	require.Equal(t, 1, strings.Count(transcript.TranscriptText(), "Offer how many hundredweights gold?"))
	acknowledgeTranscript(t, scene, transcript)
	drainSceneWorkflow(t, scene, "shrine_meditation", 1000)

	require.Equal(t, originalLocation, scene.gameState.MapState.PlayerLocation)
	require.Equal(t, expectedDate, scene.gameState.DateTime)
	offeringSounds := 0
	for _, sound := range callbacks.SoundEffectsPlayed {
		if sound == game_state.SoundShrineOffering {
			offeringSounds++
		}
	}
	require.Equal(t, 1, offeringSounds)
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Nil(t, scene.miniMapScene)
}

func TestShrineOfferingInsufficientGoldRepromptsAndZeroExits(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	const shrineIndex = 0
	scene.gameState.SetShrineQuestCompleted(shrineIndex, true)
	scene.gameState.SetShrineQuestActive(shrineIndex, false)
	scene.gameState.PartyState.Inventory.Gold.Set(100)
	originalLocation := scene.gameState.MapState.PlayerLocation
	expectedDate := scene.gameState.DateTime
	expectedDate.Advance(game_state.ShrineTimeCostMinutes)

	scene.StartShrineWorkflow(shrineIndex)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, "HONESTY")
	for range game_state.MantraRepetitions {
		require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Mantra:"))
		enterTranscriptText(t, scene, transcript, "AHM")
	}
	require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Hundredweights?"))
	enterTranscriptText(t, scene, transcript, "3")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "Thou hast not that much gold!"))
	require.Equal(t, uint16(100), scene.gameState.PartyState.Inventory.Gold.Get())
	acknowledgeTranscript(t, scene, transcript)
	require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Hundredweights?"))
	enterTranscriptText(t, scene, transcript, "0")
	require.Same(t, transcript, waitForShrineTranscriptResult(t, scene, transcript, "Thou dost leave the shrine."))
	require.Equal(t, uint16(100), scene.gameState.PartyState.Inventory.Gold.Get())
	require.Equal(t, 1, strings.Count(transcript.TranscriptText(), "Offer how many hundredweights gold?"))
	acknowledgeTranscript(t, scene, transcript)
	drainSceneWorkflow(t, scene, shrineMeditationWorkflowName, 1000)

	require.Equal(t, uint16(100), scene.gameState.PartyState.Inventory.Gold.Get())
	require.Equal(t, originalLocation, scene.gameState.MapState.PlayerLocation)
	require.Equal(t, expectedDate, scene.gameState.DateTime)
	require.NotContains(t, callbacks.SoundEffectsPlayed, game_state.SoundShrineOffering)
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Nil(t, scene.miniMapScene)
}

func TestRuinedShrineRestorationUsesOneTranscriptForVirtueAndMantras(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	shrineData := game_state.GetShrineData(0)
	require.NotNil(t, shrineData)
	scene.gameState.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	scene.gameState.MapState.PlayerLocation.Position = references.Position{
		X: references.Coordinate(shrineData.WorldX - 1),
		Y: references.Coordinate(shrineData.WorldY),
	}
	scene.gameState.SetShrineRuined(shrineData.Index, true)
	scene.gameState.SetShrineRuined(1, true)
	require.NotNil(t, scene.gameState.GetAdjacentRuinedShrine(shrineData.WordOfPower))

	scene.ShowYellDialogLargeMap()
	yell := waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200)
	enterInputBoxText(t, scene, yell, "SPEAK"+shrineData.WordOfPower)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, shrineData.VirtueName+"ISMYVIRTUE")
	for range game_state.MantraRepetitions {
		require.Same(t, transcript, waitForTranscriptPrompt(t, scene, "Mantra:"))
		enterTranscriptText(t, scene, transcript, shrineData.Mantra)
	}
	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)

	require.False(t, scene.gameState.IsShrineRuined(shrineData.Index))
	require.True(t, scene.gameState.IsShrineRuined(1))
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestRuinedShrineRestorationRejectsMeditationKeyAndStopsBeforeMantras(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	shrineData := game_state.GetShrineData(0)
	require.NotNil(t, shrineData)
	scene.gameState.MapState.PlayerLocation.Position = references.Position{
		X: references.Coordinate(shrineData.WorldX - 1),
		Y: references.Coordinate(shrineData.WorldY),
	}
	scene.gameState.SetShrineRuined(shrineData.Index, true)

	scene.ShowYellDialogLargeMap()
	yell := waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200)
	enterInputBoxText(t, scene, yell, shrineData.WordOfPower)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, shrineData.VirtueKey)
	overlay := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
	overlay.Dismiss()
	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)

	require.True(t, scene.gameState.IsShrineRuined(shrineData.Index))
	require.NotContains(t, transcript.TranscriptText(), "Mantra:")
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestRuinedShrineRestorationEscapePreservesStateAndReleasesInput(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	shrineData := game_state.GetShrineData(0)
	require.NotNil(t, shrineData)
	scene.gameState.MapState.PlayerLocation.Position = references.Position{
		X: references.Coordinate(shrineData.WorldX - 1),
		Y: references.Coordinate(shrineData.WorldY),
	}
	scene.gameState.SetShrineRuined(shrineData.Index, true)

	scene.ShowYellDialogLargeMap()
	yell := waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200)
	enterInputBoxText(t, scene, yell, shrineData.WordOfPower)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	transcript.Update()
	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)

	require.True(t, scene.gameState.IsShrineRuined(shrineData.Index))
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestRuinedShrineMantraCancellationStopsRemainingQuestions(t *testing.T) {
	scene, callbacks := newShrineTranscriptTestScene(t)
	shrine := game_state.GetShrineData(0)
	scene.gameState.MapState.PlayerLocation.Position = references.Position{
		X: references.Coordinate(shrine.WorldX - 1), Y: references.Coordinate(shrine.WorldY),
	}
	scene.gameState.SetShrineRuined(shrine.Index, true)
	scene.ShowYellDialogLargeMap()
	enterInputBoxText(t, scene, waitForTopModalWithin[*widgets.InputBox](t, scene, 1000, 200), shrine.WordOfPower)
	transcript := waitForTranscriptPrompt(t, scene, "Virtue?")
	enterTranscriptText(t, scene, transcript, shrine.VirtueName)
	waitForTranscriptPrompt(t, scene, "Mantra:")
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	transcript.Update()
	drainSceneWorkflow(t, scene, "word_of_power_entry", 1000)
	require.NotContains(t, transcript.TranscriptText(), "Thy thoughts are unfocused.")
	require.True(t, scene.gameState.IsShrineRuined(shrine.Index))
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func newShrineTranscriptTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState: state,
		keyboard:  newPresentationTestKeyboard(),
		output:    text.NewOutput(nil, 1, 20, 80),
	}, callbacks
}

func waitForTranscriptPrompt(
	t *testing.T,
	scene *GameScene,
	prompt string,
) *widgets.InteractionTranscript {
	t.Helper()
	for range 200 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			dialog.ActivePrompt() == prompt && !dialog.InputComplete() {
			return dialog
		}
	}
	t.Fatalf("interaction transcript prompt %q was not shown", prompt)

	return nil
}

func enterTranscriptText(
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
		var key ebiten.Key
		switch {
		case character >= 'A' && character <= 'Z':
			key = ebiten.KeyA + ebiten.Key(character-'A')
		case character >= '0' && character <= '9':
			key = ebiten.Key0 + ebiten.Key(character-'0')
		default:
			t.Fatalf("unsupported transcript test character %q", character)
		}
		scene.keyboard.SimulateKeyPress(key)
		dialog.Update()
	}
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	require.True(t, dialog.InputComplete())
}

func waitForShrineTranscriptResult(
	t *testing.T,
	scene *GameScene,
	transcript *widgets.InteractionTranscript,
	content string,
) *widgets.InteractionTranscript {
	t.Helper()
	for range 200 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if scene.dialogStack.PeekTopModalDialog() == transcript &&
			transcript.ActivePrompt() == "" && !transcript.InputComplete() &&
			transcript.ControlHint() == "[Enter/Esc] Close" &&
			strings.Contains(strings.Join(strings.Fields(transcript.TranscriptText()), " "), content) {
			return transcript
		}
	}
	t.Fatalf("shrine transcript result %q was not shown", content)

	return nil
}

func enterInputBoxText(t *testing.T, scene *GameScene, dialog *widgets.InputBox, value string) {
	t.Helper()
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	for _, character := range strings.ToUpper(value) {
		scene.keyboard.ClearFrameConsumption()
		key := ebiten.KeyA + ebiten.Key(character-'A')
		scene.keyboard.SimulateKeyPress(key)
		dialog.Update()
	}
	require.Equal(t, strings.ToUpper(value), dialog.GetText())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
}

func dismissNextShrineOverlay(t *testing.T, scene *GameScene) string {
	t.Helper()
	dialog := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
	message := strings.Join(strings.Fields(dialog.GetMessage()), " ")
	dialog.Dismiss()

	return message
}
