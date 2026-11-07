package widgets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func notesTestNotebook(t *testing.T) journal.NotesState {
	t.Helper()
	at := journal.GameTime{Year: 139, Month: 4, Day: 12, Hour: 9, Minute: 15}
	notes := journal.DefaultNotes()
	notes.Sections = append(notes.Sections, journal.NoteSection{ID: 3, Name: "Clues", Folded: true})
	notes.LastSectionID, notes.LastNoteID = 3, 3
	notes.Notes = []journal.NoteRecord{
		{ID: 1, SectionID: 1, Title: "Buy sextant", Text: "", Place: "Minoc", RecordedAt: at, Done: true},
		{ID: 2, SectionID: 1, Title: "", Text: "Keys and torches\nThe guild has both.", Place: "Minoc", RecordedAt: at},
		{ID: 3, SectionID: 3, Title: "Hermit's riddle", Text: "Watch the moongates at night.", Place: "Yew", RecordedAt: at},
	}
	require.NoError(t, journal.ValidateNotes(notes))

	return notes
}

func notesListRows(dialog *QuestJournalDialog) []string {
	var rows []string
	for index := range dialog.currentItemCount() {
		rows = append(rows, dialog.listRowText(index))
	}

	return rows
}

func TestMyNotesTabSectionsKeysSearchAndSimpleMode(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	var intents []NotesIntent
	configuration := testQuestJournalConfig(keyboard)
	configuration.Notes = notesTestNotebook(t)
	configuration.OnNotesIntent = func(intent NotesIntent) { intents = append(intents, intent) }
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	press := func(target *QuestJournalDialog, key ebiten.Key, shift bool) {
		keyboard.SetAllowKeyPressImmediately()
		held[key], held[ebiten.KeyShift] = true, shift
		target.Update()
		delete(held, key)
		delete(held, ebiten.KeyShift)
		keyboard.ClearFrameConsumption()
	}

	// My Notes is last, so Shift+Tab reaches it from Chronicle in one press.
	press(dialog, ebiten.KeyTab, true)
	require.Equal(t, QuestJournalNotesTab, dialog.activeTab)
	require.Equal(t, []string{"- TO DO (2)", "  Keys and torches", "  [Done] Buy sextant", "- PLACES (0)", "+ CLUES (1)"},
		notesListRows(dialog), "newest first, ticked notes sink, folded sections hide their notes")

	// Keys on a note only ask the scene; the scene owns every change.
	press(dialog, ebiten.KeyDown, false)
	for _, key := range []ebiten.Key{ebiten.KeyX, ebiten.KeyE, ebiten.KeyN} {
		press(dialog, key, false)
	}
	press(dialog, ebiten.KeyN, true)
	press(dialog, ebiten.KeyS, false)
	require.Contains(t, dialog.footerGuidance(), "[Enter] Move")
	press(dialog, ebiten.KeyDown, false)
	press(dialog, ebiten.KeyDown, false)
	press(dialog, ebiten.KeyEnter, false)
	press(dialog, ebiten.KeyD, false)
	require.Contains(t, dialog.footerGuidance(), "Permanently delete this note?")
	press(dialog, ebiten.KeyEnter, false)
	require.Equal(t, []NotesIntent{
		{Action: NotesToggleDone, NoteID: 2, SectionID: 1, Folded: false},
		{Action: NotesEdit, NoteID: 2, SectionID: 1, Folded: false},
		{Action: NotesNew, NoteID: 0, SectionID: 1, Folded: false},
		{Action: NotesNewSection, NoteID: 0, SectionID: 0, Folded: false},
		{Action: NotesMove, NoteID: 2, SectionID: 3, Folded: false},
		{Action: NotesDelete, NoteID: 2, SectionID: 1, Folded: false},
	}, intents)

	// Headings: Places keeps its name, built-ins can't be deleted, own sections fold and delete after asking.
	intents = nil
	press(dialog, ebiten.KeyEnd, false)
	press(dialog, ebiten.KeyUp, false)
	press(dialog, ebiten.KeyR, false)
	require.Contains(t, strings.Join(detailValues(dialog), " "), "Places keeps its name.")
	press(dialog, ebiten.KeyEnd, false)
	press(dialog, ebiten.KeyRight, false)
	press(dialog, ebiten.KeyR, false)
	press(dialog, ebiten.KeyD, false)
	require.Contains(t, dialog.footerGuidance(), "Delete section Clues? Its 1 notes move to To Do.")
	press(dialog, ebiten.KeyEscape, false)
	require.False(t, dialog.IsDismissed(), "Esc cancels only the confirmation")
	require.Equal(t, []NotesIntent{
		{Action: NotesFold, NoteID: 0, SectionID: 3, Folded: false},
		{Action: NotesRenameSection, NoteID: 0, SectionID: 3, Folded: false},
	}, intents)

	// The scene's answer refreshes the list, selects the note and explains the result.
	updated := notesTestNotebook(t)
	updated.Sections[2].Folded = false
	dialog.NotesChanged(updated, 3, 0, "Note saved.")
	require.Equal(t, "  Hermit's riddle", dialog.listRowText(dialog.selections[QuestJournalNotesTab]))
	require.Equal(t, "Note saved.", detailValues(dialog)[0])

	// Search reaches a note in a folded section, unfolds it and selects it.
	intents = nil
	dialog.NotesChanged(notesTestNotebook(t), 0, 1, "")
	dialog.switchTabBy(1)
	dialog.beginSearch()
	dialog.setSearchQuery("moongates")
	require.Len(t, dialog.searchResults, 1)
	dialog.openSearchResult()
	require.Equal(t, QuestJournalNotesTab, dialog.activeTab)
	require.Equal(t, "  Hermit's riddle", dialog.listRowText(dialog.selections[QuestJournalNotesTab]))
	require.Equal(t, []NotesIntent{{Action: NotesFold, NoteID: 3, SectionID: 3, Folded: false}}, intents)

	// Every row kind's footer fits its reserved rows at every supported size.
	sizes := append(config.SupportedWindowResolutions(), config.ScreenResolution{X: 320, Y: 200})
	for _, size := range sizes {
		setQuestJournalTestResolution(t, size.X, size.Y)
		dialog.ensureLayout()
		for row := range dialog.currentItemCount() {
			dialog.selections[QuestJournalNotesTab] = row
			rows := packedFooterRows(dialog.font, dialog.footerGuidance(), dialog.footerContentRect().Dx())
			require.LessOrEqual(t, len(rows), dialog.footerLineCount(), "%dx%d row %d", size.X, size.Y, row)
		}
	}
	setQuestJournalTestResolution(t, 1280, 720)
	dialog.ensureLayout()
	writeNotesReview(t, dialog, "my-notes")

}

func detailValues(dialog *QuestJournalDialog) []string {
	var values []string
	for _, line := range dialog.detailLines() {
		values = append(values, line.value)
	}

	return values
}

func writeNotesReview(t *testing.T, dialog *QuestJournalDialog, name string) {
	t.Helper()
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	screen.Fill(ucolor.EGABlack)
	dialog.Draw(screen)
	writeBatchWidgetReview(t, screen, name)
}

func TestNoteEditorUsesJournalWordingAndAcceptsUntitledNotes(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	var saved []string
	editor := NewAtlasNoteEditor(AtlasNoteEditorConfig{Keyboard: newWidgetTestKeyboard(0), Cancel: func() {},
		Submit: func(title, text string) error {
			saved = append(saved, title, text)

			return nil
		},
		Text: &NoteEditorText{Heading: "NEW NOTE",
			Info: [2][]EditorInfo{{{Label: "Section", Value: "To Do"}},
				{{Label: "Recorded", Value: "4-12-139 09:15"}, {Label: "Place", Value: "Britain"}}},
			Captions: [2]string{"Title (optional)", "Text"},
			Limits:   [2]int{journal.MaxNoteTitleRunes, journal.MaxNoteTextRunes},
			Initial:  [2]string{"", strings.Repeat("Ask Iolo about the Shards. ", 80)[:journal.MaxNoteTextRunes]},
			Validate: journal.ValidateNoteText}})
	require.Equal(t, "Title (optional)", editor.fieldCaption(0))
	require.Equal(t, journal.MaxNoteTextRunes, editor.fieldLimit(1))
	editor.focus = 1
	require.False(t, editor.validFieldText(string(editor.fields[1])+"x"), "2,000 characters is the limit")
	require.Equal(t, fmt.Sprintf("This field allows at most %d characters.", journal.MaxNoteTextRunes), editor.message)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	editor.Draw(screen)
	writeBatchWidgetReview(t, screen, "note-editor")
	editor.submit()
	require.Equal(t, []string{"", string(editor.fields[1])}, saved, "an untitled note saves")

	mapEditor := NewAtlasNoteEditor(AtlasNoteEditorConfig{Keyboard: newWidgetTestKeyboard(0), Cancel: func() {},
		Submit: func(string, string) error { return nil }})
	require.Equal(t, "Map label", mapEditor.fieldCaption(0), "without note wording the Map editor is unchanged")
	require.Equal(t, journal.MaxAtlasNoteDescriptionRunes, mapEditor.fieldLimit(1))
}
