package widgets

import (
	"errors"
	"image"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasNoteEditorPhysicalKeyboardMultilineAndValidation(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	keys := map[ebiten.Key]bool{ebiten.KeyN: true}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
	keyboard.PrepareForTransition(0)
	var label, description string
	saves, cancels := 0, 0
	reject := true
	editor := NewAtlasNoteEditor(AtlasNoteEditorConfig{Keyboard: keyboard,
		Submit: func(l, d string) error {
			if reject {
				return errors.New("stale fix")
			}
			label, description = l, d
			saves++
			return nil
		}, Cancel: func() { cancels++ }})
	chars := []rune("n")
	editor.appendChars = func(dst []rune) []rune { return append(dst, chars...) }
	step := func(held ...ebiten.Key) {
		clear(keys)
		for _, key := range held {
			keys[key] = true
		}
		keyboard.ClearFrameConsumption()
		editor.Update()
		chars = nil
	}
	press := func(key ebiten.Key) { step(); step(key); step() }
	step(ebiten.KeyN)
	require.Empty(t, editor.fields[0], "held opener cannot type into new draft")
	step()
	chars = []rune("Landing")
	step()
	press(ebiten.KeyEnter)
	require.Equal(t, "Landing", string(editor.fields[0]))
	require.Zero(t, saves)
	press(ebiten.KeyTab)
	require.Equal(t, 1, editor.focus)
	chars = []rune("Walk north.")
	step()
	press(ebiten.KeyEnter)
	press(ebiten.KeyEnter)
	chars = []rune("Bring food.")
	step()
	require.Equal(t, "Walk north.\n\nBring food.", string(editor.fields[1]))
	step(ebiten.KeyShiftLeft, ebiten.KeyTab)
	step()
	require.Equal(t, 0, editor.focus)
	step(ebiten.KeyControlLeft, ebiten.KeyEnter)
	step()
	require.Equal(t, "stale fix", editor.message)
	require.False(t, editor.finished)
	require.Equal(t, "Landing", string(editor.fields[0]))
	require.Equal(t, "Walk north.\n\nBring food.", string(editor.fields[1]))
	reject = false
	step(ebiten.KeyControlLeft, ebiten.KeyEnter)
	require.True(t, editor.finished)
	require.Equal(t, 1, saves)
	require.Equal(t, "Landing", label)
	require.Equal(t, "Walk north.\n\nBring food.", description)
	step(ebiten.KeyControlLeft, ebiten.KeyEnter)
	require.Equal(t, 1, saves)
	require.Zero(t, cancels)
}

func TestAtlasNoteEditorBoundsCursorParagraphsAndCancellation(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	cancelled := 0
	editor := NewAtlasNoteEditor(AtlasNoteEditorConfig{Keyboard: newWidgetTestKeyboard(0), Submit: func(string, string) error { return nil }, Cancel: func() { cancelled++ }})
	editor.insert([]rune(strings.Repeat("x", 32)))
	editor.insert([]rune("z"))
	require.Len(t, editor.fields[0], 32)
	require.Contains(t, editor.message, "32")
	editor.handleKey(ebiten.KeyHome)
	editor.handleKey(ebiten.KeyDelete)
	editor.insert([]rune("A"))
	require.Equal(t, 'A', editor.fields[0][0])
	editor.handleKey(ebiten.KeyTab)
	editor.insert([]rune("First\n\nThird"))
	editor.handleKey(ebiten.KeyHome)
	editor.handleKey(ebiten.KeyUp)
	require.Equal(t, 6, editor.cursors[1], "vertical navigation preserves empty paragraph")
	editor.handleKey(ebiten.KeyBackspace)
	require.Equal(t, "First\nThird", string(editor.fields[1]))
	editor.fields[1] = []rune(strings.Repeat("b", 500))
	editor.cursors[1] = 500
	editor.insert([]rune("c"))
	require.Len(t, editor.fields[1], 500)
	require.Contains(t, editor.message, "500")
	editor.handleKey(ebiten.KeyEscape)
	editor.Update()
	require.Equal(t, 1, cancelled)
}

func TestAtlasNoteEditorLayoutsScrollWithoutShrinkingAtPresets(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setQuestJournalTestResolution(t, size.X, size.Y)
		editor := NewAtlasNoteEditor(AtlasNoteEditorConfig{Record: journal.AtlasNoteRecord{Label: strings.Repeat("M", 32), Description: strings.Repeat("Paragraph\n", 50)},
			Keyboard: newWidgetTestKeyboard(0), Submit: func(string, string) error { return nil }, Cancel: func() {}})
		editor.focus = 1
		editor.ensureLayout()
		editor.prepareFieldScroll(1, editor.fieldLayout(1))
		require.True(t, editor.pane.In(image.Rect(0, 0, size.X, size.Y)))
		require.True(t, editor.fieldRects[0].In(editor.pane))
		require.True(t, editor.fieldRects[1].In(editor.pane))
		require.Less(t, editor.fieldRects[0].Max.Y, editor.fieldRects[1].Min.Y)
		require.Greater(t, editor.fieldRects[1].Dy(), editor.lineHeight)
		require.Greater(t, editor.scroll[1], 0, "long descriptions scroll to the editing cursor")
		require.Contains(t, editor.output.GetOutputStr(false), "Ctrl+Enter")
	}
}

func TestAtlasNoteIntentsOnlyRealNoteRowsAndNoCameraMutation(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	view.Zoom = 4
	view.CenterX = 9
	view.CenterY = 11
	dossier := atlasCleanupDossier()
	dossier.CurrentFix = atlasMeasuredSite(19, 29)
	note := journal.AtlasNoteRecord{ID: 7, Label: "Landing", Position: journal.SiteLocation{X: 19, Y: 29}}
	dossier.Entries = append(dossier.Entries, journal.AtlasEntry{Key: journal.AtlasNoteKey(7), Name: note.Label, Kind: "note", Category: journal.AtlasNotes, Note: note})
	dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
	var intents []AtlasNoteIntent
	dialog.SetNoteIntentHandler(func(intent AtlasNoteIntent) { intents = append(intents, intent) })
	require.Equal(t, journal.AtlasNoteKey(7), view.SelectedKey, "notes lead the complete list")
	dialog.navigate(ebiten.KeyDown)
	dialog.handleKey(ebiten.KeyE)
	dialog.handleKey(ebiten.KeyD)
	require.Empty(t, intents, "a place row offers no note edit or delete")
	dialog.handleKey(ebiten.KeyN)
	require.Equal(t, []AtlasNoteIntent{{Action: AtlasNoteMark}}, intents)
	dialog.NoteCommitted(dossier, 7)
	require.Equal(t, journal.AtlasNotePlaces, view.Filters.Places)
	require.Equal(t, journal.AtlasNoteKey(7), view.SelectedKey)
	require.Equal(t, 4, view.Zoom)
	require.Equal(t, float64(9), view.CenterX)
	require.Equal(t, float64(11), view.CenterY)
	dialog.handleKey(ebiten.KeyE)
	dialog.handleKey(ebiten.KeyD)
	require.Equal(t, AtlasNoteIntent{Action: AtlasNoteEdit, ID: 7}, intents[1])
	require.Equal(t, AtlasNoteIntent{Action: AtlasNoteDelete, ID: 7}, intents[2])
	dialog.navigate(ebiten.KeyDown)
	require.True(t, view.Overview)
	dialog.handleKey(ebiten.KeyE)
	dialog.handleKey(ebiten.KeyD)
	require.Len(t, intents, 3)
	dossier.CurrentFix = journal.SurveyedLocation{}
	dossier.FixUnavailable = "Night only: 19:00-05:59."
	dialog.Refresh(dossier)
	dialog.handleKey(ebiten.KeyN)
	require.Len(t, intents, 3)
	require.Equal(t, dossier.FixUnavailable, dialog.status)
}

func TestAtlasNoteReaderRetainsEveryDescriptionParagraphForPaging(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	note := journal.AtlasNoteRecord{ID: 1, Label: "Long note", Description: strings.Repeat("\n", 499) + "Z"}
	dialog := NewAtlasDialog(journal.AtlasDossier{Entries: []journal.AtlasEntry{{Key: "note-1", Name: note.Label, Category: journal.AtlasNotes, Kind: "note", Note: note}}}, nil, newWidgetTestKeyboard(0), &view, func() {})
	dialog.prepareDetails()
	require.Greater(t, dialog.details.GetPanelContentLineCount(), 500)
	require.Contains(t, dialog.details.GetOutputStr(false), "Map label")
	require.Contains(t, dialog.details.GetOutputStr(false), "Z")
	dialog.detailPage = 1000
	dialog.prepareDetails()
	require.Greater(t, dialog.detailPage, 0)
	dialog.handleKey(ebiten.KeySpace)
	require.Zero(t, dialog.detailPage, "Space pages details and wraps from the last page")
}

func (e *AtlasNoteEditor) prepareFieldScroll(field int, layout atlasEditorFieldLayout) ([]atlasEditorLine, int) {
	return e.scrollForContent(field, layout, e.fields[field], e.cursors[field])
}
