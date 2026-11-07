package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

const (
	questJournalQueryLimit       = 128
	questJournalSearchHeaderRows = 3
)

type questJournalSearchEntry struct {
	tab     QuestJournalTab
	row     int
	noteID  uint64
	title   string
	text    string
	source  string
	body    string
	snippet string
}

func (d *QuestJournalDialog) buildSearchEntries() {
	var selected questJournalSearchEntry
	hasSelection := len(d.searchResults) > 0
	if hasSelection {
		selected = d.searchEntries[d.searchResults[d.searchSelection]]
	}
	query := d.searchQuery
	d.searchQuery = ""
	d.searchResults = nil
	d.searchSelection = 0
	d.searchEntries = nil
	if !d.notesOnly() {
		d.appendCuratedSearchEntries()
		d.appendConversationSearchEntries()
	}
	d.appendNoteSearchEntries()
	d.setSearchQuery(query)
	if hasSelection {
		for index, result := range d.searchResults {
			entry := d.searchEntries[result]
			if entry.tab == selected.tab && entry.row == selected.row && entry.noteID == selected.noteID &&
				entry.title == selected.title {
				d.searchSelection = index

				break
			}
		}
	}
}

func (d *QuestJournalDialog) appendCuratedSearchEntries() {
	active := d.activeTab
	for _, tab := range []QuestJournalTab{QuestJournalChronicleTab, QuestJournalVirtuesTab,
		QuestJournalShadowlordsTab, QuestJournalDungeonsTab, QuestJournalMoonstonesTab} {
		d.activeTab = tab
		selected := d.selections[tab]
		for row := range d.currentItemCount() {
			d.selections[tab] = row
			title := d.listRowText(row)
			var searchable strings.Builder
			searchable.WriteString(title)
			// Only rendered, discovery-gated values enter the index, never semantic IDs.
			for _, line := range d.detailLines() {
				if line.value == questJournalNotLearned || tab == QuestJournalMoonstonesTab && moonstonePlaceholder(line.value) {
					continue
				}
				searchable.WriteByte(' ')
				if tab == QuestJournalMoonstonesTab {
					searchable.WriteString(line.label + ": ")
				}
				searchable.WriteString(line.value)
			}
			source := questJournalTabTitle(tab)
			d.searchEntries = append(d.searchEntries, questJournalSearchEntry{
				tab: tab, row: row, noteID: 0, title: title, text: strings.ToLower(searchable.String()),
				source: source, body: searchable.String(), snippet: "",
			})
		}
		d.selections[tab] = selected
	}
	d.activeTab = active
}

func (d *QuestJournalDialog) appendConversationSearchEntries() {
	for row, exchange := range d.lastConversation.Exchanges {
		d.searchEntries = append(d.searchEntries, questJournalSearchEntry{
			tab: QuestJournalConversationTab, row: row, noteID: 0, title: exchange.Title(),
			text:   strings.ToLower(exchange.SearchText()),
			source: exchange.Speaker + " - " + exchange.Location, body: journalSnippetSource(exchange), snippet: "",
		})
	}
	for row, index := range d.visibleExcerpts {
		exchange := d.excerpts[index]
		searchable := exchange.SearchText()
		if exchange.Archived {
			searchable += " Archived"
		}
		d.searchEntries = append(d.searchEntries, questJournalSearchEntry{
			tab: QuestJournalExcerptsTab, row: row, noteID: 0, title: excerptTitle(exchange), text: strings.ToLower(searchable),
			source: exchange.Speaker + " - " + exchange.Location, body: journalSnippetSource(exchange), snippet: "",
		})
	}
}

func (d *QuestJournalDialog) beginSearch() {
	d.Reset()
	d.runicReferenceOpen = false
	d.searching = true
	if d.searchInput == nil {
		d.searchInput = NewTextInput(d.placement, d.font.TextFace.Size, questJournalQueryLimit, nil,
			TextInputCallbacks{}, d.keyboard) //nolint:exhaustruct_v5 // Journal owns Enter and dismissal.
		d.searchInput.SetFreeTextMode()
	}
	d.searchInput.SetText(d.searchQuery)
	d.rebuildFrames()
}

func (d *QuestJournalDialog) leaveSearch() {
	d.searching = false
	d.Reset()
	d.rebuildFrames()
}

func (d *QuestJournalDialog) setSearchQuery(query string) {
	if query == d.searchQuery {
		return
	}
	selected := -1
	if len(d.searchResults) > 0 {
		selected = d.searchResults[d.searchSelection]
	}
	d.searchQuery = query
	d.searchResults = d.searchResults[:0]
	d.searchSelection = 0
	words := strings.Fields(strings.ToLower(query))
	if len(words) == 0 {
		return
	}
	for index, entry := range d.searchEntries {
		matches := true
		for _, word := range words {
			if !strings.Contains(entry.text, word) {
				matches = false

				break
			}
		}
		if !matches {
			continue
		}
		if index == selected {
			d.searchSelection = len(d.searchResults)
		}
		d.searchEntries[index].snippet = journalSearchSnippet(entry.body, query,
			d.font.CharsForWidth(d.searchResultTextRect().Dx()))
		d.searchResults = append(d.searchResults, index)
	}
}

func (d *QuestJournalDialog) handleSearchInput() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		d.openSearchResult()

		return
	}
	if d.handleNavigationInput() {
		return
	}
	// TextInput owns spaces, punctuation, editing and Tab while typing. None can
	// fall through to the journal's tab/detail commands in the same update.
	d.searchInput.Update()
	d.setSearchQuery(d.searchInput.GetText())
}

func (d *QuestJournalDialog) openSearchResult() {
	if len(d.searchResults) == 0 {
		return
	}
	entry := d.searchEntries[d.searchResults[d.searchSelection]]
	d.activeTab = entry.tab
	d.selections[entry.tab] = entry.row
	d.detailPage = 0
	d.leaveSearch()
	d.enterTab()
	if entry.tab == QuestJournalNotesTab {
		d.openNoteSearchResult(entry.noteID)
	}
	d.readingQuery = d.searchQuery
	d.exchangeOutput, d.cachedDetailPages = nil, nil
	d.revealSearchMatch()
}

func (d *QuestJournalDialog) searchContentRect() image.Rectangle {
	content := insetRectangle(d.layout.bodyRect, d.layout.padding*questJournalPaneInsetMultiplier)
	content.Min.Y += d.rowHeight() * questJournalSearchHeaderRows

	return content
}

func (d *QuestJournalDialog) searchResultTextRect() image.Rectangle {
	content := d.searchContentRect()
	// Keep text out of the shared scroll-indicator gutter, including long sources.
	content.Max.X -= max(d.layout.padding*questJournalPaneInsetMultiplier, int(math.Ceil(d.font.TextFace.Size)))

	return content
}

func (d *QuestJournalDialog) searchStatus() string {
	if len(strings.Fields(d.searchQuery)) == 0 {
		return "Type words to search the Journal."
	}
	if len(d.searchResults) == 0 {
		return "No matching entries."
	}

	return fmt.Sprintf("%d matching entries", len(d.searchResults))
}

//nolint:funlen // Query, result rows and scroll cues share one measured viewport and clipping contract.
func (d *QuestJournalDialog) drawSearch(screen *ebiten.Image) {
	content := d.searchContentRect()
	prompt := image.Rect(content.Min.X, content.Min.Y-d.rowHeight()*questJournalSearchHeaderRows,
		content.Max.X, content.Min.Y-d.rowHeight()*(questJournalSearchHeaderRows-1))
	label := "Search Journal: "
	if d.notesOnly() {
		label = "Search My Notes: "
	}
	labelWidth, _ := d.font.MeasureString(label)
	query := d.searchQuery + "_"
	for len(query) > 1 {
		width, _ := d.font.MeasureString(query)
		if width+labelWidth <= float64(prompt.Dx()) {
			break
		}
		query = query[1:]
	}
	d.rowOutput.SetMaxCharsPerLine(len(label) + len(query))
	d.rowOutput.Reset()
	d.rowOutput.AppendLine().Text(label, DialogLabelColor).Text(query, DialogValueColor).Done()
	d.rowOutput.DrawPanelContent(screen, prompt.Min, prompt.Dx(), etext.AlignStart)
	d.drawSingleLine(screen,
		image.Rect(content.Min.X, prompt.Max.Y, content.Max.X, prompt.Max.Y+d.rowHeight()),
		d.searchStatus(), DialogEmptyColor)
	scope := "Recent history; archived excerpts hidden (Excerpts: H)."
	if d.notesOnly() {
		scope = "Simple Notes Mode: searching My Notes only."
	}
	if d.showArchived {
		scope = "Recent history; archived excerpts included."
	}
	d.drawSingleLine(screen, image.Rect(content.Min.X, content.Min.Y-d.rowHeight(), content.Max.X, content.Min.Y),
		scope, DialogControlHintColor)
	rowHeight := d.rowHeight() * journalSearchResultRows
	textBounds := d.searchResultTextRect()
	start, end := zStatsVisibleRange(d.searchSelection, len(d.searchResults), max(1, content.Dy()/rowHeight))
	for index := start; index < end; index++ {
		entry := d.searchEntries[d.searchResults[index]]
		top := content.Min.Y + (index-start)*rowHeight
		rect := image.Rect(textBounds.Min.X, top, textBounds.Max.X, top+rowHeight)
		rowColor, background := d.journalRowColors(entry.tab, entry.row, index == d.searchSelection)
		categoryColor := DialogEmptyColor
		if index == d.searchSelection {
			vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y),
				float32(rect.Dx()), float32(rect.Dy()), background, false)
			categoryColor = DialogSelectedText
		}
		category := questJournalTabTitle(entry.tab)
		width, _ := d.font.MeasureString(category)
		categoryRect := rect
		categoryRect.Max.Y = top + d.rowHeight()
		categoryRect.Min.X = rect.Max.X - int(width) - d.layout.padding
		titleRect := image.Rect(rect.Min.X, top, categoryRect.Min.X-d.layout.padding, top+d.rowHeight())
		title := entry.title
		if d.entryIsNew(entry.tab, entry.row) {
			title = "* " + title
		}
		if entry.tab == QuestJournalMoonstonesTab {
			d.drawMoonstoneRow(screen, titleRect, entry.row, index == d.searchSelection, rowColor)
		} else {
			d.drawJournalMatchLine(screen, titleRect, title, d.searchQuery, rowColor, index == d.searchSelection)
		}
		d.drawSingleLine(screen, categoryRect, category, categoryColor)
		sourceRect := image.Rect(rect.Min.X, titleRect.Max.Y, rect.Max.X, titleRect.Max.Y+d.rowHeight())
		d.drawJournalMatchLine(screen, sourceRect, entry.source, d.searchQuery, categoryColor, index == d.searchSelection)
		snippetRect := image.Rect(rect.Min.X, sourceRect.Max.Y, rect.Max.X, rect.Max.Y)
		d.drawJournalMatchLine(screen, snippetRect, entry.snippet, d.searchQuery, rowColor, index == d.searchSelection)
	}
	d.drawScrollIndicators(screen, content, start, end, len(d.searchResults), rowHeight)
}
