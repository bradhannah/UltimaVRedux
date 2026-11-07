package widgets

import (
	"errors"
	"fmt"
	"image"
	stdcolor "image/color"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// questJournalExcerptStorageUnavailable is shown when an excerpt action has no storage callback.
const questJournalExcerptStorageUnavailable = "Excerpt storage unavailable."

func cloneJournalExchanges(exchanges []journal.Exchange) []journal.Exchange {
	result := make([]journal.Exchange, len(exchanges))
	for i, exchange := range exchanges {
		result[i] = exchange.Clone()
	}

	return result
}

func (d *QuestJournalDialog) isConversationTab() bool {
	return d.activeTab == QuestJournalConversationTab || d.activeTab == QuestJournalExcerptsTab
}

func (d *QuestJournalDialog) selectedExchange() journal.Exchange {
	if d.currentItemCount() == 0 {
		return journal.Exchange{} //nolint:exhaustruct_v5 // Empty selection.
	}
	if d.activeTab == QuestJournalConversationTab {
		return d.lastConversation.Exchanges[d.selections[d.activeTab]]
	}

	return d.excerpts[d.visibleExcerpts[d.selections[d.activeTab]]]
}

// Visible rows are a projection of the complete notebook, never a second store.
func (d *QuestJournalDialog) refreshVisibleExcerpts(selectedID string) {
	d.visibleExcerpts = d.visibleExcerpts[:0]
	selection := d.selections[QuestJournalExcerptsTab]
	for index, excerpt := range d.excerpts {
		if excerpt.Archived && !d.showArchived {
			continue
		}
		if excerpt.ID == selectedID {
			selection = len(d.visibleExcerpts)
		}
		d.visibleExcerpts = append(d.visibleExcerpts, index)
	}
	d.selections[QuestJournalExcerptsTab] = max(0, min(selection, len(d.visibleExcerpts)-1))
}

func (d *QuestJournalDialog) toggleArchivedExcerpts() {
	selectedID := d.selectedExchange().ID
	d.showArchived = !d.showArchived
	d.refreshVisibleExcerpts(selectedID)
	d.exchangeOutput = nil
	d.detailPage = 0
	d.buildSearchEntries()
}

func excerptTitle(exchange journal.Exchange) string {
	if exchange.Archived {
		// Prefix the state so even a clipped long title retains its indication.
		return "[Archived] " + exchange.Title()
	}

	return exchange.Title()
}

func (d *QuestJournalDialog) emptyListText() string {
	if d.activeTab == QuestJournalExcerptsTab && len(d.excerpts) > 0 && !d.showArchived {
		return "All excerpts archived."
	}

	return "No entries recorded."
}

func (d *QuestJournalDialog) journalRowColors(
	tab QuestJournalTab, row int, selected bool,
) (stdcolor.Color, stdcolor.Color) {
	archived := tab == QuestJournalExcerptsTab && d.excerpts[d.visibleExcerpts[row]].Archived
	if selected {
		if archived {
			return DialogSelectedText, pkgcolor.JournalArchived
		}

		return DialogSelectedText, DialogSelectedBackground
	}
	if archived {
		return pkgcolor.JournalArchived, DialogSelectedBackground
	}

	return DialogItemNameColor, DialogSelectedBackground
}

func (d *QuestJournalDialog) exchangeKept(id string) bool {
	for _, excerpt := range d.excerpts {
		if excerpt.ID == id {
			return true
		}
	}

	return false
}

func (d *QuestJournalDialog) handleExcerptInput() bool {
	if d.deletePending {
		switch {
		case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
			d.deletePending = false
		case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter), d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
			excerpts, err := d.deleteExcerpt(d.selectedExchange().ID)
			d.excerptResult(excerpts, err, "Excerpt removed")
			d.deletePending = false
		}

		return true
	}
	if d.searching || !d.isConversationTab() {
		return false
	}
	if d.activeTab == QuestJournalExcerptsTab && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyH) {
		d.toggleArchivedExcerpts()

		return true
	}
	if d.currentItemCount() == 0 {
		return false
	}
	if d.activeTab == QuestJournalConversationTab && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyK) {
		if d.keepExcerpt == nil {
			d.excerptStatus = questJournalExcerptStorageUnavailable

			return true
		}
		excerpts, err := d.keepExcerpt(d.selectedExchange())
		d.excerptResult(excerpts, err, "Excerpt kept")

		return true
	}
	if d.activeTab == QuestJournalExcerptsTab {
		return d.handleExcerptManagementInput()
	}

	return false
}

func (d *QuestJournalDialog) handleExcerptManagementInput() bool {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyA) {
		if d.archiveExcerpt == nil {
			d.excerptStatus = questJournalExcerptStorageUnavailable

			return true
		}
		exchange := d.selectedExchange()
		excerpts, err := d.archiveExcerpt(exchange.ID, !exchange.Archived)
		status := "Excerpt archived"
		if exchange.Archived {
			status = "Excerpt restored"
		}
		d.excerptResult(excerpts, err, status)

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyD) {
		if d.deleteExcerpt == nil {
			d.excerptStatus = questJournalExcerptStorageUnavailable
		} else {
			d.deletePending = true
		}

		return true
	}

	return false
}

func (d *QuestJournalDialog) excerptResult(excerpts []journal.Exchange, err error, success string) {
	if err != nil {
		d.excerptStatus = "Excerpt operation failed."
		if errors.Is(err, journal.ErrNotebookFull) {
			d.excerptStatus = "Notebook full; delete to free space."
		}

		return
	}
	selectedID := d.selectedExchange().ID
	d.excerpts = cloneJournalExchanges(excerpts)
	d.refreshVisibleExcerpts(selectedID)
	d.excerptStatus = success
	d.clampCurrentSelection()
	if d.activeTab == QuestJournalExcerptsTab {
		d.exchangeOutput = nil
		d.detailPage = 0
	}
	d.buildSearchEntries()
}

func (d *QuestJournalDialog) drawExchange(screen *ebiten.Image) {
	d.prepareExchangeReader()
	content := d.exchangeContentRect()
	heading := d.detailContentRect()
	status := d.excerptStatus
	if d.activeTab == QuestJournalConversationTab && d.currentItemCount() > 0 {
		status = fmt.Sprintf("Exchange %d/%d", d.selections[d.activeTab]+1, d.currentItemCount())
	}
	if pages := d.exchangePageCount(); pages > 1 {
		status = fmt.Sprintf("Page %d/%d", d.detailPage+1, pages)
		if d.detailPage == pages-1 {
			status += " - Last page"
		}
	}
	if d.excerptStatus != "" && status != d.excerptStatus {
		status += " - " + d.excerptStatus
	}
	if d.activeTab == QuestJournalConversationTab && d.lastConversation.Truncated {
		status += " - Older exchanges omitted"
	}
	if status != "" {
		rect := image.Rect(heading.Min.X, heading.Min.Y, heading.Max.X, content.Min.Y)
		d.drawSingleLine(screen, rect, status, DialogLabelColor)
	}
	if d.currentItemCount() == 0 {
		return
	}
	capacity := max(1, d.exchangeOutput.GetLineCapacity(content.Dy()))
	if selected := d.exchangeHighlightRect(); !selected.Empty() {
		vector.FillRect(screen, float32(selected.Min.X), float32(selected.Min.Y),
			float32(selected.Dx()), float32(selected.Dy()), DialogSelectedBackground, false)
	}
	d.exchangeOutput.DrawPanelContentRange(screen, content.Min, content.Dx(), etext.AlignStart, d.passageRow, capacity)
}

// Reserve a stable status row so keeping an excerpt cannot move the text or
// change the size of its pages when the confirmation appears.
func (d *QuestJournalDialog) exchangeContentRect() image.Rectangle {
	content := d.detailContentRect()
	content.Min.Y = min(content.Max.Y, content.Min.Y+d.rowHeight())

	return content
}

func (d *QuestJournalDialog) prepareExchangeReader() {
	if d.currentItemCount() == 0 {
		return
	}
	content := d.exchangeContentRect()
	if d.exchangeOutput == nil {
		exchanges := d.lastConversation.Exchanges
		selected := d.selections[d.activeTab]
		if d.activeTab == QuestJournalExcerptsTab {
			exchanges = []journal.Exchange{d.selectedExchange()}
			// This pane contains only the saved excerpt, not a capture selection.
			selected = -1
		}
		d.exchangeOutput, d.conversationRows = journalExchangesOutput(
			exchanges, d.font, d.rowOutput.GetLineSpacing(), content.Dx(), selected, d.readingQuery)
	}
	capacity := max(1, d.exchangeOutput.GetLineCapacity(content.Dy()))
	start, end := d.selectedExchangeSpan()
	pages := d.exchangePageCount()
	d.detailPage = max(0, min(d.detailPage, pages-1))
	if end-start >= capacity {
		// Long exchanges start at the pane's top, including their last page. Do
		// not pull earlier text back into view merely to fill that page's bottom.
		d.passageRow = start + d.detailPage*capacity
	} else {
		// A fitting exchange can retain surrounding context without hiding any
		// of the selection. The attribution header remains outside the span.
		top := start - (capacity-(end-start))/questJournalCenterDivisor
		d.passageRow = max(0, min(top, max(0, d.exchangeOutput.GetPanelContentLineCount()-capacity)))
	}
}

func (d *QuestJournalDialog) selectedExchangeSpan() (int, int) {
	if d.exchangeOutput == nil || d.currentItemCount() == 0 {
		return 0, 0
	}
	selected := 0
	if d.activeTab == QuestJournalConversationTab {
		selected = d.selections[d.activeTab]
	}
	end := d.exchangeOutput.GetPanelContentLineCount()
	if selected+1 < len(d.conversationRows) {
		end = d.conversationRows[selected+1]
	}

	return d.conversationRows[selected], end
}

func (d *QuestJournalDialog) exchangePageCount() int {
	if d.exchangeOutput == nil || d.currentItemCount() == 0 {
		return 1
	}
	start, end := d.selectedExchangeSpan()
	capacity := max(1, d.exchangeOutput.GetLineCapacity(d.exchangeContentRect().Dy()))

	return max(1, (end-start+capacity-1)/capacity)
}

func (d *QuestJournalDialog) pageSelectedExchange(delta int) {
	d.prepareExchangeReader()
	d.detailPage = max(0, min(d.detailPage+delta, d.exchangePageCount()-1))
	d.prepareExchangeReader()
}

func (d *QuestJournalDialog) exchangeHighlightRect() image.Rectangle {
	if d.activeTab != QuestJournalConversationTab {
		return image.Rect(0, 0, 0, 0)
	}
	content := d.exchangeContentRect()
	start, end := d.selectedExchangeSpan()
	capacity := max(1, d.exchangeOutput.GetLineCapacity(content.Dy()))
	start, end = max(start, d.passageRow), min(end, d.passageRow+capacity)
	if start >= end {
		return image.Rect(0, 0, 0, 0)
	}
	spacing := d.exchangeOutput.GetLineSpacing()
	top := content.Min.Y + int(math.Floor(float64(start-d.passageRow)*spacing))
	bottom := content.Min.Y + int(math.Ceil(float64(end-d.passageRow)*spacing))

	return image.Rect(content.Min.X, top, content.Max.X, bottom).Intersect(content)
}

func (d *QuestJournalDialog) exchangeFooterGuidance() string {
	action := "[K] Keep"
	if d.activeTab == QuestJournalExcerptsTab {
		action = "[A] Archive"
		if d.selectedExchange().Archived {
			action = "[A] Restore"
		}
		action += "  [Del/D] Delete  [H] Show Archived: "
		if d.showArchived {
			action += "On"
		} else {
			action += "Off"
		}
	}
	paging := ""
	if d.GetDetailPageCount() > 1 {
		if d.detailPage < d.exchangePageCount()-1 {
			paging += "[Space] Next  "
		}
		if d.detailPage > 0 {
			paging += "[Shift+Space] Previous  "
		}
	}

	reference := ""
	if d.hasRunicExchange() {
		reference = "  [Ctrl+R] Rune reference"
	}

	return "[Up/Down] Exchange  " + paging + action + reference + "  [/] Search  [Tab/Shift+Tab] Tab  [Esc/Shift+J] Close"
}

func (d *QuestJournalDialog) hasRunicExchange() bool {
	if !d.isConversationTab() || d.currentItemCount() == 0 {
		return false
	}
	for _, passage := range d.selectedExchange().Passages {
		if passage.Runic {
			return true
		}
	}

	return false
}

func (d *QuestJournalDialog) handleRunicReferenceInput() bool {
	if d.searching || d.deletePending {
		return false
	}
	if d.runicReferenceOpen && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.runicReferenceOpen = false

		return true
	}
	if d.keyboard.IsModifierPressed(input.ModifierCtrl) && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyR) ||
		d.keyboard.IsKeyPressedAndRegister(RunicReferenceFunctionKey) {
		if d.hasRunicExchange() {
			d.runicReferenceOpen = !d.runicReferenceOpen
		}

		return true
	}
	if d.runicReferenceOpen {
		// The reference footer advertises reading/closing, not notebook edits.
		// Do not allow a hidden Delete confirmation behind the alphabet pane.
		if !d.handleDismissInput() {
			d.handleBrowseInput()
		}

		return true
	}

	return false
}

func journalExchangesOutput(
	exchanges []journal.Exchange, font *text.UltimaFont, spacing float64, width, selected int,
	query string,
) (*text.Output, []int) {
	// A one-column stress layout can require one row per source byte. Allocate
	// once per selection/reflow so even a long exchange is never circularly lost.
	const headingAllowance = 32
	rows := headingAllowance
	for _, exchange := range exchanges {
		rows += len(exchange.Speaker) + len(exchange.Location) + len(exchange.Input) + headingAllowance
		for _, passage := range exchange.Passages {
			rows += len(passage.Text)
		}
	}
	output := text.NewOutput(font, spacing, rows, max(1, font.CharsForWidth(width)))
	starts := make([]int, 0, len(exchanges))
	if len(exchanges) > 0 {
		output.AddText(exchanges[0].Speaker + " - " + exchanges[0].Location)
	}
	for index, exchange := range exchanges {
		starts = append(starts, output.GetPanelContentLineCount())
		foreground := DialogValueColor
		if index == selected {
			foreground = DialogSelectedText
		}
		heading := "Greeting"
		if exchange.Input != "" {
			heading = "Avatar: " + exchange.Input
		}
		highlight := ""
		if selected < 0 || index == selected {
			highlight = query
		}
		line := output.AppendLine()
		appendJournalMatches(line, heading, highlight, foreground, index == selected)
		line.Done()
		appendJournalPassages(output, exchange, foreground, highlight, index == selected)
	}

	return output, starts
}

func appendJournalPassages(
	output *text.Output, exchange journal.Exchange, foreground stdcolor.Color, query string, selected bool,
) {
	segments := make([]text.ColorSegment, 0, len(exchange.Passages))
	for _, passage := range exchange.Passages {
		if !passage.Runic && query != "" {
			segments = append(segments, journalMatchSegments(passage.Text, query, foreground, selected)...)

			continue
		}
		fontType := text.FontTypeNormal
		if passage.Runic {
			fontType = text.FontTypeRunic
		}
		segments = append(segments, text.ColorSegment{
			Text: passage.Text, Color: foreground, FontType: fontType, FontScale: text.FontScaleNormal,
		})
	}
	output.AddStyledText(segments, etext.AlignStart)
}
