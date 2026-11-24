package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestEbExcerptAttributionWithoutNameQuestion(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithRandomSeed(42).WithSystemCallbacks().Build()
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(-1, noSceneTestKeyHeld),
		activePlaythrough: playthroughMetadata{id: "0123456789abcdef0123456789abcdef"}}
	var actor *map_units.NPCFriendly
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		if npc, ok := unit.(*map_units.NPCFriendly); ok && npc.NPCReference.DialogNumber == 8 {
			actor = npc
			break
		}
	}
	require.NotNil(t, actor)
	dialog := NewLinearTalkDialog(scene, actor)
	require.NotNil(t, dialog)
	scene.PushDialog(dialog)
	// No NAME question or prior identity evidence is needed to label kept notes.
	for _, topic := range []string{"GLASS", "N", "Y", "N"} {
		dialog.TextInput.SetText(topic)
		dialog.onEnter()
	}
	require.Equal(t, "Eb", dialog.exchange.Speaker)
	scene.keyboard.SimulateKeyPress(ebiten.KeyTab)
	dialog.Update()
	passages := dialog.exchange.Clone().Passages
	excerpts, err := scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 1)
	require.Equal(t, "Eb", excerpts[0].Speaker)
	require.Equal(t, passages, excerpts[0].Passages)
	dialog.Close()

	// A later session keeps the same attribution without asking NAME.
	dialog = NewLinearTalkDialog(scene, actor)
	require.Equal(t, "Eb", dialog.knownSpeaker)
	dialog.Close()
	// Simulate a reloaded scene whose Last Conversation was replaced. Existing
	// kept notes remain independently stored in the active game.
	scene.lastConversation = journal.Conversation{}
	dialog = NewLinearTalkDialog(scene, actor)
	scene.PushDialog(dialog)
	require.Equal(t, "Eb", dialog.knownSpeaker)
	for _, topic := range []string{"GLASS", "N", "Y", "N"} {
		dialog.TextInput.SetText(topic)
		dialog.onEnter()
	}
	dialog.keepPresentedExchange()
	dialog.keepPresentedExchange()
	excerpts, err = scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 2, "repeat capture of the same exchange is idempotent")
	require.Equal(t, "Eb", excerpts[1].Speaker)
	require.Equal(t, passages, excerpts[1].Passages)
	dialog.Close()
	// Even removing all retained notes must not bring back the confusing label.
	scene.excerpts = nil
	scene.lastConversation = journal.Conversation{}
	dialog = NewLinearTalkDialog(scene, actor)
	require.Equal(t, "Eb", dialog.knownSpeaker)
	scene.PushDialog(dialog)
	dialog.keepPresentedExchange()
	excerpts, err = scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 1)
	require.Equal(t, "Eb", excerpts[0].Speaker)
	dialog.Close()
}

func TestExcerptNameLabelDoesNotCaptureUnpresentedSpeech(t *testing.T) {
	definition := boundaryTalkDefinition(semantic.Body{
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "a stranger"}}},
	})
	definition.Name = semantic.Body{
		&semantic.PauseAction{},
		&semantic.SayAction{Content: semantic.RichText{&semantic.TextPart{Text: "Hidden name"}}},
	}
	dialog := newBoundaryTalkDialog(t, definition)
	scene := dialog.gameScene
	scene.activePlaythrough = playthroughMetadata{id: "0123456789abcdef0123456789abcdef"}
	scene.dialogStack.PushModalDialog(dialog)
	dialog.mount()
	dialog.TextInput.SetText("NAME")
	dialog.onEnter()
	require.True(t, dialog.isPausing)
	require.Equal(t, "Hidden name", dialog.knownSpeaker)
	dialog.keepPresentedExchange()
	dialog.Close()
	excerpts, err := scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 1)
	require.Equal(t, "Hidden name", excerpts[0].Speaker)
	for _, passage := range excerpts[0].Passages {
		require.NotContains(t, passage.Text, "Hidden name", "attribution cannot flush the pending NAME response")
	}
	require.Empty(t, scene.gameState.CurrentQuestJournalDossier().Chronicle)
}
