package main

import (
	"fmt"
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
	require.Equal(t, &journal.SiteLocation{Map: references.Britain}, notes.Notes[0].Site, "linked to the town")
	require.Equal(t, before, state.DateTime, "writing a note costs no game time")

	// M shows the town's own Map entry; the Map never locates it unmeasured.
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesShowOnMap, NoteID: notes.Notes[0].ID,
		SectionID: journal.NoteSectionToDo})
	require.NotNil(t, scene.journalSession.mapRequest, "the next frame opens the Map")
	scene.openNoteMap(nil, scene.journalSession)
	require.IsType(t, &widgets.AtlasDialog{}, scene.journalSession.child)
	require.Equal(t, "chart-02", scene.atlasView.SelectedKey)
	require.Equal(t, journal.AtlasTownes, scene.atlasView.Filters.Places)
	scene.closeNotesChild()

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

func TestJournalShowsNotesOnTheMapAndEditsMapNotes(t *testing.T) {
	scene, held, update, _ := newRunicReferenceTestScene(t, references.BritanniaUnderworld)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	state := scene.gameState
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = false
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	draft, err := state.BeginAtlasNoteDraft(0)
	require.NoError(t, err)
	mapNote, err := state.CommitAtlasNote(draft, "Ferry", "Boats wait here.")
	require.NoError(t, err)
	pending := []rune("Hermit hut")
	scene.appendInputChars = func(dst []rune) []rune {
		dst = append(dst, pending...)
		pending = nil

		return dst
	}
	scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
	t.Cleanup(func() {
		if scene.journalSession != nil {
			scene.closeJournalSession(scene.journalSession, false)
		}
	})
	session := scene.journalSession

	// A note written outdoors at night with a sextant links its exact spot.
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesNew, SectionID: journal.NoteSectionPlaces})
	update()
	held[ebiten.KeyControlLeft], held[ebiten.KeyEnter] = true, true
	update()
	clear(held)
	update()
	note := state.PlayerNotes().Notes[0]
	require.Equal(t, &journal.SiteLocation{Map: references.BritanniaUnderworld, X: 1, Y: 1}, note.Site)

	// M opens the Map over the Journal on that spot; held Esc returns to the Journal only.
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesShowOnMap, NoteID: note.ID,
		SectionID: journal.NoteSectionPlaces})
	scene.openNoteMap(nil, session)
	require.Same(t, session.child, scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, fmt.Sprintf("journal-note-%d", note.ID), scene.atlasView.SelectedKey)
	require.Equal(t, journal.AtlasNotePlaces, scene.atlasView.Filters.Places)
	require.Equal(t, []float64{1, 1}, []float64{scene.atlasView.CenterX, scene.atlasView.CenterY})
	require.Nil(t, scene.atlasSession, "the Journal keeps its pause; the real Map session never opens")
	update()
	held[ebiten.KeyEscape] = true
	update()
	require.Nil(t, session.child)
	update()
	require.Same(t, session, scene.journalSession, "held Esc cannot close the Journal")
	require.False(t, session.closing)
	clear(held)
	update()

	// A Map note opens on its own Map entry, and edits and deletes from Places.
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesShowOnMap, NoteID: mapNote.ID,
		SectionID: journal.NoteSectionPlaces, MapNote: true})
	scene.openNoteMap(nil, session)
	require.Equal(t, journal.AtlasNoteKey(mapNote.ID), scene.atlasView.SelectedKey)
	scene.closeNotesChild()
	pending = []rune(" crossing")
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesEdit, NoteID: mapNote.ID,
		SectionID: journal.NoteSectionPlaces, MapNote: true})
	require.IsType(t, &widgets.AtlasNoteEditor{}, session.child)
	update()
	held[ebiten.KeyControlLeft], held[ebiten.KeyEnter] = true, true
	update()
	clear(held)
	require.Nil(t, session.child)
	require.Nil(t, session.noteDraft)
	require.Equal(t, "Ferry crossing", state.CaptureAtlasNotes().Records[0].Label)
	update()
	scene.handleNotesIntent(widgets.NotesIntent{Action: widgets.NotesDelete, NoteID: mapNote.ID,
		SectionID: journal.NoteSectionPlaces, MapNote: true})
	require.Empty(t, state.CaptureAtlasNotes().Records)
	require.Len(t, state.PlayerNotes().Notes, 1, "deleting a Map note leaves Journal notes alone")
}
