package widgets

import (
	"os"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func archiveTestExchange(id string, archived bool) journal.Exchange {
	return journal.Exchange{ID: id, Speaker: "Iolo", Location: "Britain", Input: "WORD", Archived: archived,
		Passages: []journal.Passage{{Text: "Speech " + id, Runic: false}}}
}

func TestExcerptArchiveKeyboardVisibilitySearchAndRestore(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	keyboard := newWidgetTestKeyboard(0)
	configuration := testQuestJournalConfig(keyboard)
	configuration.Excerpts = []journal.Exchange{archiveTestExchange("hidden", true), archiveTestExchange("active", false)}
	configuration.LastConversation = journal.Conversation{Exchanges: []journal.Exchange{archiveTestExchange("active", false)}}
	stored := cloneJournalExchanges(configuration.Excerpts)
	configuration.ArchiveExcerpt = func(id string, archived bool) ([]journal.Exchange, error) {
		for index := range stored {
			if stored[index].ID == id {
				stored[index].Archived = archived
			}
		}

		return stored, nil
	}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.switchTabBy(int(QuestJournalExcerptsTab))
	require.False(t, dialog.showArchived)
	require.Equal(t, "active", dialog.selectedExchange().ID)
	dialog.setSearchQuery("hidden")
	require.Empty(t, dialog.searchResults)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyH)
	require.Equal(t, "active", dialog.selectedExchange().ID, "visibility changes retain the selected stable identity")
	require.Len(t, dialog.searchResults, 1)
	require.Contains(t, dialog.footerGuidance(), "Show Archived: On")
	dialog.setSearchQuery("archived")
	require.Len(t, dialog.searchResults, 1, "the visible archive indicator is searchable")
	dialog.beginSearch()
	dialog.openSearchResult()
	require.Equal(t, "hidden", dialog.selectedExchange().ID)
	require.Contains(t, dialog.listRowText(dialog.GetSelectedIndex()), "[Archived]")
	require.Contains(t, dialog.footerGuidance(), "[A] Restore")
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyA)
	require.False(t, stored[0].Archived)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyH)
	require.Equal(t, "hidden", dialog.selectedExchange().ID)
	require.Contains(t, dialog.footerGuidance(), "Show Archived: Off")
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyA)
	require.Equal(t, "active", dialog.selectedExchange().ID)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyA)
	require.Equal(t, -1, dialog.GetSelectedIndex())
	require.Equal(t, "All excerpts archived.", dialog.emptyListText())
	require.Equal(t, configuration.LastConversation, dialog.lastConversation, "archiving never edits Last Conversation")
	require.Equal(t, configuration.Excerpts, []journal.Exchange{archiveTestExchange("hidden", true), archiveTestExchange("active", false)}, "opening input is detached")
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyH)
	require.Equal(t, 2, dialog.currentItemCount(), "Show Archived remains usable with an empty active list")
	reopened := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	require.False(t, reopened.showArchived, "visibility is local to this Journal session")
}

func TestExcerptArchiveDeleteConfirmationAndErrorRollback(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	keyboard := newWidgetTestKeyboard(0)
	configuration := testQuestJournalConfig(keyboard)
	configuration.Excerpts = []journal.Exchange{archiveTestExchange("one", false)}
	configuration.Excerpts[0].Passages[0].Text = strings.Repeat("Whole speech\n", 100)
	configuration.ArchiveExcerpt = func(string, bool) ([]journal.Exchange, error) {
		return []journal.Exchange{archiveTestExchange("wrong", true)}, os.ErrPermission
	}
	deletes := 0
	configuration.DeleteExcerpt = func(id string) ([]journal.Exchange, error) {
		require.Equal(t, "one", id)
		deletes++
		if deletes == 1 {
			return nil, os.ErrPermission
		}

		return nil, nil
	}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.switchTabBy(int(QuestJournalExcerptsTab))
	dialog.pageSelectedExchange(1)
	page := dialog.GetDetailPage()
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyA)
	require.Equal(t, configuration.Excerpts, dialog.excerpts)
	require.Equal(t, page, dialog.GetDetailPage(), "failed archive must not move the reader")
	require.Contains(t, dialog.excerptStatus, "Could not keep that.")
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyD)
	require.True(t, dialog.deletePending)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyH)
	require.False(t, dialog.showArchived, "confirmation isolates all management/navigation input")
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyEscape)
	require.False(t, dialog.deletePending)
	require.False(t, dialog.IsDismissed())
	require.Zero(t, deletes)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyDelete)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, configuration.Excerpts, dialog.excerpts, "failed delete preserves the full selected excerpt")
	require.Equal(t, page, dialog.GetDetailPage())
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyD)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, 2, deletes)
	require.Empty(t, dialog.excerpts)
	require.Equal(t, -1, dialog.GetSelectedIndex())
}

func TestExcerptFullNotebookFeedbackPreservesKeptSnapshot(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	keyboard := newWidgetTestKeyboard(0)
	configuration := testQuestJournalConfig(keyboard)
	configuration.Excerpts = []journal.Exchange{archiveTestExchange("kept", true)}
	configuration.LastConversation.Exchanges = []journal.Exchange{archiveTestExchange("new", false)}
	configuration.KeepExcerpt = func(journal.Exchange) ([]journal.Exchange, error) {
		return nil, journal.ErrNotebookFull
	}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.switchTabBy(int(QuestJournalConversationTab))
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyK)
	require.Equal(t, configuration.Excerpts, dialog.excerpts)
	require.Contains(t, dialog.excerptStatus, "Delete a note to write another")
}

func TestExcerptArchiveInputDoesNotCaptureSearchTypingOrUnfocusedInput(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	keyboard := newWidgetTestKeyboard(0)
	configuration := testQuestJournalConfig(keyboard)
	configuration.Excerpts = []journal.Exchange{archiveTestExchange("one", false)}
	configuration.ArchiveExcerpt = func(string, bool) ([]journal.Exchange, error) {
		t.Fatal("archive must not run outside focused browsing")

		return nil, nil
	}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.switchTabBy(int(QuestJournalExcerptsTab))
	dialog.beginSearch()
	for _, key := range []ebiten.Key{ebiten.KeyA, ebiten.KeyH, ebiten.KeyD, ebiten.KeyDelete} {
		pressQuestJournalKey(dialog, keyboard, key)
	}
	require.False(t, dialog.showArchived)
	require.False(t, dialog.deletePending)
	dialog.leaveSearch()
	dialog.onDialogFocusChanged(false)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyA)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyH)
	require.False(t, dialog.showArchived)
}

func TestExcerptArchiveSemanticColorsAndFooterFits(t *testing.T) {
	for _, resolution := range append(config.SupportedWindowResolutions(), config.ScreenResolution{X: 320, Y: 200}) {
		setQuestJournalTestResolution(t, resolution.X, resolution.Y)
		configuration := testQuestJournalConfig(newWidgetTestKeyboard(0))
		configuration.Excerpts = []journal.Exchange{archiveTestExchange("one", true)}
		configuration.Excerpts[0].Passages[0].Text = strings.Repeat("Long speech\n", 150)
		dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
		dialog.switchTabBy(int(QuestJournalExcerptsTab))
		dialog.toggleArchivedExcerpts()
		dialog.pageSelectedExchange(1)
		foreground, _ := dialog.journalRowColors(QuestJournalExcerptsTab, 0, false)
		require.Equal(t, pkgcolor.JournalArchived, foreground)
		foreground, background := dialog.journalRowColors(QuestJournalExcerptsTab, 0, true)
		require.Equal(t, DialogSelectedText, foreground)
		require.Equal(t, pkgcolor.JournalArchived, background)
		require.NotEqual(t, foreground, background)
		dialog.ensureLayout()
		width := dialog.footerContentRect().Dx()
		lines := packedFooterRows(dialog.font, dialog.footerGuidance(), width)
		footer := strings.Join(lines, "\n")
		require.LessOrEqual(t, len(lines), dialog.footerLineCount())
		for _, line := range lines {
			measured, _ := dialog.font.MeasureString(line)
			require.LessOrEqual(t, measured, float64(width), "%dx%d: %s", resolution.X, resolution.Y, line)
		}
		for _, hint := range []string{"[A] Restore", "[Del/D] Delete", "[H] Show Archived", "Space", "Search", "Close"} {
			require.Contains(t, footer, hint)
		}
	}
}
