package widgets

import (
	"image"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestJournalSearchShowsSnippetAndOpensActualWrappedMatch(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	configuration := testQuestJournalConfig(newWidgetTestKeyboard(-1))
	exchange := journal.Exchange{ID: "long", Speaker: "Iolo", Location: "Britain", Input: "JOB",
		Passages: []journal.Passage{
			{Text: "needle hidden runic", Runic: true},
			{Text: strings.Repeat("Already recorded words.\n", 100), Runic: false},
			{Text: "The NEEDLE is beneath the old tree.", Runic: false},
		}}
	configuration.Excerpts = []journal.Exchange{exchange}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.beginSearch()
	dialog.setSearchQuery("needle")
	require.Len(t, dialog.searchResults, 1)
	entry := dialog.searchEntries[dialog.searchResults[0]]
	require.Contains(t, entry.snippet, "NEEDLE")
	require.NotContains(t, entry.snippet, "hidden runic")
	require.NotContains(t, entry.snippetText(), "...", "a long line is cut plainly")
	dialog.openSearchResult()
	require.Equal(t, QuestJournalExcerptsTab, dialog.activeTab)
	require.Greater(t, dialog.detailPage, 0)
	start, end := dialog.selectedExchangeSpan()
	row, found := dialog.exchangeOutput.FirstPanelLineWithColor(start, end, journalMatchColour(false))
	require.True(t, found)
	require.GreaterOrEqual(t, row, dialog.passageRow)
	capacity := dialog.exchangeOutput.GetLineCapacity(dialog.exchangeContentRect().Dy())
	require.Less(t, row, dialog.passageRow+capacity)
	require.Equal(t, exchange, dialog.selectedExchange(), "source text and runic boundaries remain intact")
}

func TestJournalSearchUnicodeHighlightAndSnippetBounds(t *testing.T) {
	value := strings.Repeat("old passage ", 30) + "Éb knows the NEEDLE location."
	snippet := journalSearchSnippet(value, "éb needle", 40)
	require.True(t, utf8.ValidString(snippet))
	require.LessOrEqual(t, utf8.RuneCountInString(snippet), 40)
	require.Contains(t, snippet, "Éb")
	require.Contains(t, snippet, "NEEDLE")
	output := text.NewOutput(nil, 1, 20, 12)
	line := output.AppendLine()
	appendJournalMatches(line, "Before ÉB and Needle", "éb needle", DialogValueColor, false)
	line.Done()
	_, found := output.FirstPanelLineWithColor(0, output.GetPanelContentLineCount(), journalMatchColour(false))
	require.True(t, found)
}

// A result shows the one line that matched, with its label, under its title.
func TestJournalSearchShowsTheMatchedLineWithItsLabel(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	configuration := testQuestJournalConfig(newWidgetTestKeyboard(-1))
	configuration.Excerpts = []journal.Exchange{{ID: "a", Speaker: "Iolo", Location: "Britain", Input: "JOB",
		Passages: []journal.Passage{{Text: "I am a bard."}}}}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.beginSearch()
	dialog.setSearchQuery("britain")
	require.Len(t, dialog.searchResults, 1)
	require.Equal(t, "Place: Britain", dialog.searchEntries[dialog.searchResults[0]].snippetText())
	dialog.setSearchQuery("bard")
	require.Equal(t, "I am a bard.", dialog.searchEntries[dialog.searchResults[0]].snippetText())
}

func TestJournalSearchResultRowsAndPagingShareCapacity(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	configuration := testQuestJournalConfig(newWidgetTestKeyboard(-1))
	for index := range 40 {
		configuration.Excerpts = append(configuration.Excerpts, journal.Exchange{ID: string(rune('A' + index)),
			Speaker: "Iolo", Location: "Britain", Input: "JOB", Passages: []journal.Passage{{Text: "needle"}}})
	}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.beginSearch()
	dialog.setSearchQuery("needle")
	capacity := max(1, dialog.visibleRows(dialog.searchContentRect())/journalSearchResultRows)
	dialog.navigate(ebiten.KeyPageDown)
	require.Equal(t, capacity, dialog.searchSelection)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	require.True(t, dialog.searchContentRect().In(image.Rect(0, 0, 1280, 720)))
	require.Less(t, dialog.searchResultTextRect().Max.X, dialog.searchContentRect().Max.X,
		"long source/snippet rows must leave the scroll indicators unobscured")
}
