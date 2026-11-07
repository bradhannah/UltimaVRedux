package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func TestJournalNoteEditorSavesStampedNoteAndHeldEscStaysInJournal(t *testing.T) {
	scene, held, update, _ := newRunicReferenceTestScene(t, references.Britain)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	pending := []rune("Ask Iolo about the Shards")
	scene.appendInputChars = func(dst []rune) []rune {
		dst = append(dst, pending...)
		pending = nil

		return dst
	}
	state := scene.gameState
	scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
	t.Cleanup(func() {
		if scene.journalSession != nil {
			scene.closeJournalSession(scene.journalSession, false)
		}
	})
	before := state.DateTime

	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesNew, NoteID: 0,
		SectionID: journal.NoteSectionToDo, Folded: false})
	require.Same(t, scene.journalSession.child, scene.dialogStack.PeekTopModalDialog())
	update()
	held[ebiten.KeyControlLeft], held[ebiten.KeyEnter] = true, true
	update()
	clear(held)
	require.Nil(t, scene.journalSession.child)
	notes := state.PlayerNotes()
	require.Len(t, notes.Notes, 1)
	require.Equal(t, "Ask Iolo about the Shards", notes.Notes[0].Title)
	require.Equal(t, "Britain, main", notes.Notes[0].Place, "the Save screen's place name")
	require.Equal(t, before, state.DateTime, "writing a note costs no game time")

	// Esc held in the editor closes only the editor, never the Journal under it.
	update()
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesEdit, NoteID: notes.Notes[0].ID,
		SectionID: journal.NoteSectionToDo, Folded: false})
	update()
	held[ebiten.KeyEscape] = true
	update()
	require.Nil(t, scene.journalSession.child)
	update()
	require.NotNil(t, scene.journalSession, "held child cancel cannot close the Journal")
	require.False(t, scene.journalSession.closing)
}

func TestSimpleNotesModeSkipsJournalNoticeAndKeepExcerpt(t *testing.T) {
	dialog := newBoundaryTalkDialog(t, boundaryTalkDefinition(semantic.Body{}))
	scene := dialog.gameScene
	require.Contains(t, dialog.conversationFooterGuidance(1000), "[Tab] Keep excerpt")
	scene.gameConfig = &config.UltimaVConfiguration{GameplayConfiguration: config.DefaultGameplayConfiguration()}
	scene.gameConfig.GameplayConfiguration.SimpleNotesMode = true
	require.NotContains(t, dialog.conversationFooterGuidance(1000), "Keep excerpt")
	require.Contains(t, dialog.conversationFooterGuidance(1000), "[Shift+J] Journal")

	scene.output.Clear()
	scene.writeKnowledgeNotice(journal.AtlasChange{})
	require.NotContains(t, scene.output.GetOutputStr(false), "Journal updated.")
}
