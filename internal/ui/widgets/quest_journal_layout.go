package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func (d *QuestJournalDialog) rowHeight() int {
	return max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))
}

func (d *QuestJournalDialog) visibleRows(rect image.Rectangle) int {
	return max(1, rect.Dy()/d.rowHeight())
}

func (d *QuestJournalDialog) handleNavigationInput() bool {
	key, owned := d.Poll(d.keyboard,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyPageUp, ebiten.KeyPageDown, ebiten.KeyHome, ebiten.KeyEnd)
	if key != nil {
		d.navigate(*key)
	}

	return owned
}

func (d *QuestJournalDialog) navigate(key ebiten.Key) {
	if d.isConversationTab() && !d.searching {
		if key == ebiten.KeyPageUp || key == ebiten.KeyPageDown || d.keyboard.IsModifierPressed(input.ModifierCtrl) {
			return
		}
	}
	capacity, count := d.visibleRows(d.listContentRect()), d.currentItemCount()
	if d.searching {
		capacity, count = max(1, d.visibleRows(d.searchContentRect())/journalSearchResultRows), len(d.searchResults)
	}
	delta := 0
	switch key { //nolint:exhaustive // Only navigation keys enter this method.
	case ebiten.KeyUp:
		delta = -1
	case ebiten.KeyDown:
		delta = 1
	case ebiten.KeyPageUp:
		delta = -capacity
	case ebiten.KeyPageDown:
		delta = capacity
	case ebiten.KeyHome:
		delta = -count
	case ebiten.KeyEnd:
		delta = count
	}
	if d.searching {
		d.searchSelection = max(0, min(d.searchSelection+delta, count-1))

		return
	}
	d.moveSelection(delta)
}

func (d *QuestJournalDialog) rebuildFrames() {
	body := []image.Rectangle{d.layout.listRect, d.layout.detailRect}
	if d.searching {
		body = []image.Rectangle{d.layout.bodyRect}
	}
	d.frameRects = connectClassicPaneGrid([][]image.Rectangle{
		{d.layout.headerRect}, body, {d.layout.footerRect},
	}, d.pixelUnit())
}

func (d *QuestJournalDialog) entryIsNew(tab QuestJournalTab, row int) bool {
	return tab >= 0 && int(tab) < len(d.newEntries) && row >= 0 && row < len(d.newEntries[tab]) && d.newEntries[tab][row]
}

func (d *QuestJournalDialog) tabIsNew(tab QuestJournalTab) bool {
	if tab < 0 || int(tab) >= len(d.newEntries) {
		return false
	}
	for _, isNew := range d.newEntries[tab] {
		if isNew {
			return true
		}
	}

	return false
}

func (d *QuestJournalDialog) tabTitle(tab QuestJournalTab) string {
	title := questJournalTabTitle(tab)
	if d.tabIsNew(tab) {
		title += " *"
	}

	return title
}

func (d *QuestJournalDialog) calculateTabs() {
	const headerRows = 2
	rows := [][]QuestJournalTab{
		{QuestJournalChronicleTab, QuestJournalVirtuesTab, QuestJournalShadowlordsTab, QuestJournalDungeonsTab},
		{QuestJournalConversationTab, QuestJournalExcerptsTab, QuestJournalMoonstonesTab, QuestJournalNotesTab},
	}
	for row, tabs := range rows {
		total := 0
		widths := make([]int, len(tabs))
		for i, tab := range tabs {
			width, _ := d.font.MeasureString(d.tabTitle(tab))
			widths[i] = int(math.Ceil(width))
			total += widths[i]
		}
		gap := max(0, (d.layout.tabsRect.Dx()-total)/len(tabs))
		left := d.layout.tabsRect.Min.X + (d.layout.tabsRect.Dx()-total-gap*(len(tabs)-1))/questJournalCenterDivisor
		top := d.layout.tabsRect.Min.Y + row*d.layout.tabsRect.Dy()/headerRows
		for i, tab := range tabs {
			d.layout.tabRects[tab] = image.Rect(left, top, left+widths[i], top+d.layout.tabsRect.Dy()/headerRows)
			left += widths[i] + gap
		}
	}
}

// Fit before inserting into a one-row Output: wrapping there overwrites prefixes.
func fitJournalLine(value string, font *text.UltimaFont, width int) string {
	measured, _ := font.MeasureString(value)
	if measured <= float64(width) {
		return value
	}
	const ellipsis = "..."
	for value != "" {
		_, size := utf8.DecodeLastRuneInString(value)
		value = value[:len(value)-size]
		measured, _ = font.MeasureString(value + ellipsis)
		if measured <= float64(width) {
			return value + ellipsis
		}
	}

	return ""
}

func (d *QuestJournalDialog) drawSingleLine(
	screen *ebiten.Image, rect image.Rectangle, value string, lineColor stdcolor.Color,
) {
	d.prepareSingleLine(rect, value, lineColor)
	d.rowOutput.DrawPanelContent(screen, rect.Min, max(1, rect.Dx()), etext.AlignStart)
}

func (d *QuestJournalDialog) prepareSingleLine(rect image.Rectangle, value string, lineColor stdcolor.Color) {
	value = fitJournalLine(value, d.font, rect.Dx())
	d.rowOutput.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(value)))
	d.rowOutput.Reset()
	d.rowOutput.AppendLine().Text(value, lineColor).Done()
}

func (d *QuestJournalDialog) drawEntryTitle(
	screen *ebiten.Image, rect image.Rectangle, title string, isNew bool, titleColor stdcolor.Color,
) {
	starWidth := 0
	starColor := DialogLabelColor
	if titleColor == DialogSelectedText {
		starColor = DialogSelectedText
	}
	if isNew {
		width, _ := d.font.MeasureString("* ")
		starWidth = int(math.Ceil(width))
	}
	title = fitJournalLine(title, d.font, max(1, rect.Dx()-starWidth-d.layout.padding))
	d.rowOutput.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(title)+len("* ")))
	d.rowOutput.Reset()
	builder := d.rowOutput.AppendLine()
	if isNew {
		builder.Text("* ", starColor)
	}
	builder.Text(title, titleColor).Done()
	d.rowOutput.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignStart)
}

func (d *QuestJournalDialog) shadowlordSpriteRect() image.Rectangle {
	if d.activeTab != QuestJournalShadowlordsTab || d.shadowlordSprite == nil {
		return image.Rect(0, 0, 0, 0)
	}
	content := insetRectangle(d.layout.detailRect, d.layout.padding*questJournalPaneInsetMultiplier)
	bounds := d.shadowlordSprite.Bounds()
	if bounds.Empty() {
		return image.Rect(0, 0, 0, 0)
	}
	const maximumScale = 2
	scale := min(maximumScale, d.pixelUnit(),
		content.Dx()/questJournalVerticalRegionDivisor/bounds.Dx(), content.Dy()/bounds.Dy())
	if scale < 1 {
		return image.Rect(0, 0, 0, 0)
	}

	return image.Rect(content.Max.X-bounds.Dx()*scale, content.Min.Y, content.Max.X, content.Min.Y+bounds.Dy()*scale)
}

func (d *QuestJournalDialog) drawShadowlordSprite(screen *ebiten.Image) {
	rect := d.shadowlordSpriteRect()
	if rect.Empty() {
		return
	}
	scale := rect.Dx() / d.shadowlordSprite.Bounds().Dx()
	options := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	options.GeoM.Scale(float64(scale), float64(scale))
	options.GeoM.Translate(float64(rect.Min.X), float64(rect.Min.Y))
	screen.DrawImage(d.shadowlordSprite, options)
}

func (d *QuestJournalDialog) footerGuidance() string {
	if d.runicReferenceOpen {
		return "[Ctrl+R/Esc] Close rune reference  [Space/Shift+Space] Detail page  [Shift+J] Close Journal"
	}
	if d.activeTab == QuestJournalNotesTab && !d.searching {
		tabs := "[Tab/Shift+Tab] Tab  "
		if d.notesOnly() {
			tabs = ""
		}

		return d.notesFooterGuidance(tabs)
	}
	if d.deletePending {
		return "Permanently delete excerpt?  [Enter] Delete  [Esc] Cancel"
	}
	if d.isConversationTab() && !d.searching {
		return d.exchangeFooterGuidance()
	}
	if d.searching {
		return "[Up/Down] Select  [Shift+Up/Down] Page  [Home/End] First/Last  " +
			"[Enter] Open  [Esc] Back  [Shift+J] Close"
	}

	paging := ""
	if pages := d.GetDetailPageCount(); pages > 1 {
		paging = fmt.Sprintf("[Space] Detail %d/%d  ", d.detailPage+1, pages)
	}

	return "[Up/Down] Select  [Shift+Up/Down] List page  [/] Search  " +
		"[Tab/Shift+Tab] Tab  " + paging + "[Esc/Shift+J] Close"
}

func drawModifierFooter(screen *ebiten.Image, content image.Rectangle, guidance string, output *text.Output) {
	PrepareKeyboardFooter(content, guidance, output)
	output.DrawPanelContent(screen,
		image.Pt(content.Min.X, verticallyCenteredOutputY(content, output)), content.Dx(), etext.AlignCenter)
}

// PrepareKeyboardFooter replaces output with the complete bracketed hints packed
// into measured rows for content. Source line breaks are soft. It composes
// presentation only: it does not draw, consume input, or invoke callbacks.
func PrepareKeyboardFooter(content image.Rectangle, guidance string, output *text.Output) {
	output.Reset()
	PackControlHints(output, ParseControlHints(strings.ReplaceAll(guidance, "\n", " ")), content.Dx(), etext.AlignCenter)
}
