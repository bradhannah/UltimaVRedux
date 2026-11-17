package widgets

import (
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

// The player's notes are hard to miss: they lead the complete list, their tab
// carries a count, and the notes bar and empty tab explain in words what to do.
func TestAtlasNotesLeadTheListAndExplainTheirKeys(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	empty := atlasCleanupDossier()
	empty.FixUnavailable, empty.FixNeedsNight = "Night only: 19:00-05:59.", true
	view := NewAtlasViewState()
	view.Filters.Places = journal.AtlasNotePlaces
	dialog := NewAtlasDialog(empty, nil, newWidgetTestKeyboard(0), &view, func() {})
	dialog.SetNoteIntentHandler(func(AtlasNoteIntent) {})
	require.Equal(t, "My Notes (0)", dialog.atlasTabLabel(journal.AtlasNotePlaces))
	var teaching []string
	for _, field := range dialog.detailFields() {
		teaching = append(teaching, strings.TrimSpace(field[0]+" "+field[1]))
	}
	require.Contains(t, teaching, "Ctrl+N while exploring.")
	require.Contains(t, teaching, "By day Configure, Flexible Sextant Use.")
	dialog.drawNotesBar(screen)
	require.Contains(t, dialog.row.GetOutputStr(false), "N: Night only: 19:00-05:59.",
		"an unavailable key says why in words")

	noted := atlasCleanupDossier()
	noted.CurrentFix, noted.FixUnavailable = atlasMeasuredSite(19, 29), ""
	note := journal.AtlasNoteRecord{ID: 7, Label: "Landing", Position: journal.SiteLocation{X: 19, Y: 29}}
	noted.Entries = append(noted.Entries, journal.AtlasEntry{Key: journal.AtlasNoteKey(7), Name: note.Label,
		Kind: "note", Category: journal.AtlasNotes, Note: note})
	view = NewAtlasViewState()
	dialog = NewAtlasDialog(noted, nil, newWidgetTestKeyboard(0), &view, func() {})
	dialog.SetNoteIntentHandler(func(AtlasNoteIntent) {})
	require.Equal(t, journal.AtlasNoteKey(7), dialog.entries[0].Key, "notes lead All instead of trailing every place")
	require.Equal(t, "My Notes (1)", dialog.atlasTabLabel(journal.AtlasNotePlaces))
	dialog.navigate(ebiten.KeyDown)
	dialog.drawNotesBar(screen)
	require.Contains(t, dialog.row.GetOutputStr(false), "E, D: select one of your notes.")
}
