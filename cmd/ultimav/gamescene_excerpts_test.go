package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestGardnerJournalRetainsSubmittedExchanges(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).
		WithRandomSeed(42).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(-1, noSceneTestKeyHeld),
		activePlaythrough: playthroughMetadata{id: "0123456789abcdef0123456789abcdef", userDescription: ""}}
	var actor *map_units.NPCFriendly
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		if npc, ok := unit.(*map_units.NPCFriendly); ok && npc.NPCReference.DialogNumber == 25 {
			actor = npc
			break
		}
	}
	require.NotNil(t, actor)
	dialog := NewLinearTalkDialog(scene, actor)
	require.NotNil(t, dialog)
	require.Equal(t, "Gardner", dialog.engine.ExtractNPCName())
	scene.PushDialog(dialog)
	_, err := scene.keepExcerpt(scene.lastConversation.Exchanges[0])
	require.NoError(t, err)
	for _, topic := range []string{"NAME", "JOB", "BLACK", "FLAME", "BYE"} {
		require.True(t, dialog.waitingForInput, topic)
		dialog.TextInput.SetText(topic)
		dialog.onEnter()
	}
	require.Len(t, scene.lastConversation.Exchanges, 6)
	require.Equal(t, "Gardner", scene.lastConversation.Exchanges[0].Speaker)
	require.Equal(t, "SERPENT'S HOLD", scene.lastConversation.Exchanges[0].Location)
	require.Contains(t, scene.lastConversation.Exchanges[2].SearchText(), "keeper of the Flame of Courage")
	require.Contains(t, scene.lastConversation.Exchanges[4].SearchText(), "I tend this Flame")
	require.Contains(t, scene.lastConversation.Exchanges[5].SearchText(), "Fare thee well")
	excerpts, err := scene.loadExcerpts()
	require.NoError(t, err)
	require.Equal(t, "Gardner", excerpts[0].Speaker, "earlier kept greeting should use the now-revealed name")
	retainedConversation := scene.lastConversation.Clone()
	scene.lastConversation = journal.Conversation{Exchanges: []journal.Exchange{
		{ID: "replacement", Speaker: "Another", Location: "Britain", Input: "", Passages: []journal.Passage{{Text: "Hello", Runic: false}}},
	}}
	excerpts, err = scene.loadExcerpts()
	require.NoError(t, err)
	require.Equal(t, "Gardner", excerpts[0].Speaker, "revealed attribution must survive replacement of Last Conversation")
	scene.lastConversation = retainedConversation
	// Existing v17 logs already on disk also get corrected display attribution.
	scene.lastConversation.Exchanges[0].Speaker = "Speaker"
	scene.lastConversation.Exchanges[0].Location = "SerpentsHold"
	projected := scene.journalConversationForDisplay()
	require.Equal(t, "Gardner", projected.Exchanges[0].Speaker)
	require.Equal(t, "SERPENT'S HOLD", projected.Exchanges[0].Location)
	require.Equal(t, "Speaker", scene.lastConversation.Exchanges[0].Speaker, "display projection remains detached")
}

func TestJournalInterruptPreservesTalkAndWaitsForClosingKeyRelease(t *testing.T) {
	dialog := newBoundaryTalkDialog(t, boundaryTalkDefinition(semantic.Body{
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "Visible"}}},
		&semantic.WaitForKeyAction{},
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "Not yet"}}},
	}))
	scene := dialog.gameScene
	scene.output = text.NewOutput(nil, 1, 64, 80)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.gameState.SystemCallbacks.UI.ShowQuestJournalDossier = scene.ShowQuestJournalDossier
	scene.dialogStack.PushModalDialog(dialog)
	dialog.mount()
	dialog.TextInput.SetText("partial")
	held := map[ebiten.Key]bool{ebiten.KeyShift: true, ebiten.KeyJ: true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	require.NoError(t, scene.Update(nil))
	require.NotNil(t, scene.journalSession)
	require.True(t, dialog.waitingForKeypressContinue)
	require.Equal(t, "partial", dialog.TextInput.GetText())
	delete(held, ebiten.KeyShift)
	delete(held, ebiten.KeyJ)
	require.NoError(t, scene.Update(nil))
	held[ebiten.KeyEscape] = true
	require.NoError(t, scene.Update(nil))
	require.Same(t, dialog, scene.dialogStack.PeekTopModalDialog())
	require.NotNil(t, scene.journalSession, "the closing chord must not reach raw any-key waits")
	require.NoError(t, scene.Update(nil))
	require.True(t, dialog.waitingForKeypressContinue)
	delete(held, ebiten.KeyEscape)
	require.NoError(t, scene.Update(nil))
	require.Nil(t, scene.journalSession)
	require.True(t, dialog.waitingForKeypressContinue)
}

func TestShiftJTypesWhileAConversationKeywordIsBeingEntered(t *testing.T) {
	dialog := newBoundaryTalkDialog(t, boundaryTalkDefinition(semantic.Body{
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "Visible"}}},
		&semantic.WaitForKeyAction{},
	}))
	scene := dialog.gameScene
	scene.dialogStack.PushModalDialog(dialog)
	dialog.mount()
	require.True(t, dialog.waitingForKeypressContinue)
	require.False(t, scene.textEntryFocused(), "a key wait still lets Shift+J open the Journal")

	dialog.waitingForKeypressContinue = false
	require.True(t, scene.textEntryFocused(), "keyword entry takes Shift+J as a capital J")

	scene.dialogStack.PushModalDialog(widgets.NewSaveDescriptionEntryModal(widgets.SaveDescriptionEntryModalConfig{
		Title: "Save", Prompt: "Description", Keyboard: scene.keyboard, MaxRunes: 22,
		OnResult: func(string, bool) {},
	}))
	require.True(t, scene.textEntryFocused(), "a save description takes Shift+J as a capital J")
}

func TestConversationExcerptWaitIsolationAndPresentedTail(t *testing.T) {
	dialog := newBoundaryTalkDialog(t, boundaryTalkDefinition(semantic.Body{
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "Visible first"}}},
		&semantic.WaitForKeyAction{},
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.RunicPart{Text: "SUMM"}}},
		&semantic.PauseAction{},
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "Unseen tail"}}},
	}))
	scene := dialog.gameScene
	scene.activePlaythrough = playthroughMetadata{id: "0123456789abcdef0123456789abcdef", userDescription: ""}
	require.Empty(t, scene.lastConversation.Exchanges, "unmounted construction is not presented")
	scene.dialogStack.PushModalDialog(dialog)
	dialog.mount()
	require.True(t, dialog.waitingForKeypressContinue)
	held := map[ebiten.Key]bool{}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	held[ebiten.KeyTab] = true
	dialog.Update()
	require.True(t, dialog.waitingForKeypressContinue, "capture cannot acknowledge the wait")
	excerpts, err := scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 1)
	require.Contains(t, excerpts[0].SearchText(), "Visible first")
	require.NotContains(t, excerpts[0].SearchText(), "Unseen tail")
	delete(held, ebiten.KeyTab)
	scene.keyboard.ClearFrameConsumption()
	held[ebiten.KeySpace] = true
	dialog.Update()
	require.True(t, dialog.isPausing)
	dialog.keepPresentedExchange()
	excerpts, err = scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 1)
	runic := false
	for _, passage := range excerpts[0].Passages {
		runic = runic || passage.Runic && passage.Text == "SUMM"
	}
	require.True(t, runic)
	dialog.Close()
	require.Len(t, scene.lastConversation.Exchanges, 1)
	require.NotContains(t, scene.lastConversation.Exchanges[0].SearchText(), "Unseen tail")
}
