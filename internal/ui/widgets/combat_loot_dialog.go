package widgets

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	lootSides                = 2
	lootOuterInsets          = 4
	lootPaddingDivisor       = 64
	lootMaximumTextRows      = 8
	lootInitialTextChars     = 96
	lootMapWidthNumerator    = 3
	lootMapWidthDenominator  = 4
	combatSummaryWidthRatio  = 540.0 / 1280
	combatSummaryBottomRatio = 0.88
)

// Summary-only geometry keeps discovery's reveal layout independent. Scrolling
// uses logical pixels so even a single unusually long entry remains readable.
type combatSummaryLayout struct {
	outcome       string
	screenWidth   int
	padding       int
	outcomeOutput *text.Output
	footerSource  *text.Output
	// notice sits above the key hints, a padding apart, so the two never touch.
	notice                                                       *text.Output
	experience                                                   combatSummaryExperience
	values                                                       []*text.Output
	rowHeights, valueTops                                        []int
	headingHeight, outcomeTop, contentHeight, scroll, scrollStep int
	body, footer                                                 image.Rectangle
}

// CombatLootDialog retains an accumulating, scrollable discovery list. Its owner
// reveals entries and completes gameplay; the widget only acknowledges the view.
type CombatLootDialog struct {
	entries                    []game_state.CombatLootReveal
	keyboard                   *input.Keyboard
	sheet                      *sprites.SpriteSheet
	revealed, offset, capacity int
	ready, dismissed           bool
	output, heading            *text.Output
	rows                       []*text.Output
	icons                      []*ebiten.Image
	bounds                     image.Rectangle
	width                      int
	title, notice, trapNotice  string
	onDismiss                  func()
	quantities                 []uint64
	summary                    *combatSummaryLayout
}

// NewCombatLootDialog creates a detached discovery readout controlled by its workflow.
func NewCombatLootDialog(
	entries []game_state.CombatLootReveal, keyboard *input.Keyboard, sheet *sprites.SpriteSheet,
) *CombatLootDialog {
	if keyboard == nil || sheet == nil || len(entries) == 0 {
		panic("loot dialog requires entries, keyboard and sprites")
	}

	return &CombatLootDialog{
		entries: append([]game_state.CombatLootReveal(nil), entries...), keyboard: keyboard, sheet: sheet, capacity: 1,
		revealed: 0, offset: 0, ready: false, dismissed: false, output: nil, heading: nil, rows: nil, icons: nil,
		bounds: image.Rect(0, 0, 0, 0), width: 0,
		title: "Chest contents", notice: "These items are on the ground.", trapNotice: "",
		onDismiss: nil, quantities: nil, summary: nil,
	}
}

// NewCombatSummaryDialog reuses the measured, keyboard-scrollable discovery
// readout for detached totals. Its dismissal callback owns exact stack removal.
func NewCombatSummaryDialog(
	conclusion game_state.CombatConclusion, keyboard *input.Keyboard, sheet *sprites.SpriteSheet, onDismiss func(),
) *CombatLootDialog {
	if onDismiss == nil {
		panic("combat summary requires dismissal ownership")
	}
	entries := make([]game_state.CombatLootReveal, 0, max(1, len(conclusion.Collected)))
	for _, item := range conclusion.Collected {
		entries = append(entries, game_state.CombatLootReveal{
			Sprite: item.Sprite, Description: item.Name,
		})
	}
	if len(entries) == 0 {
		entries = append(entries, game_state.CombatLootReveal{Sprite: indexes.NoSprites, Description: "No items collected."})
	}
	dialog := NewCombatLootDialog(entries, keyboard, sheet)
	for _, item := range conclusion.Collected {
		dialog.quantities = append(dialog.quantities, item.Quantity)
	}
	dialog.title = "Battle complete"
	dialog.summary = new(combatSummaryLayout)
	dialog.summary.outcome = combatSummaryOutcome(conclusion.Reason)
	dialog.summary.experience.summary = conclusion.Experience
	dialog.notice = ""
	if len(conclusion.Collected) > 0 {
		dialog.notice = "Collected into party inventory."
	}
	dialog.onDismiss = onDismiss
	dialog.Reveal(len(entries))
	dialog.FinishReveal()
	dialog.offset = 0

	return dialog
}

func combatSummaryOutcome(reason game_state.CombatEndReason) string {
	switch reason {
	case game_state.CombatVictory:
		return "Victory"
	case game_state.CombatDefeat:
		return "Battle is lost!"
	case game_state.CombatEvacuated:
		return "Departed"
	case game_state.CombatNotEnded:
		panic("cannot summarize an unfinished battle")
	default:
		panic("unknown battle outcome")
	}
}

// SetTrapNotice keeps the chest's actual consequence above the discovery list.
func (d *CombatLootDialog) SetTrapNotice(message string) {
	d.trapNotice = message
	d.width = 0
}

// Reveal admits the next immutable entries and scrolls to the newest one.
func (d *CombatLootDialog) Reveal(count int) {
	if count < d.revealed || count > len(d.entries) {
		panic("invalid loot reveal progress")
	}
	d.revealed = count
	d.offset = max(0, count-d.capacity)
}

// FinishReveal enables acknowledgement after isolating keys held during discovery.
func (d *CombatLootDialog) FinishReveal() {
	d.ready = true
	d.keyboard.PrepareForTransition(0)
}

// IsDismissed reports acknowledgement to the owning workflow.
func (d *CombatLootDialog) IsDismissed() bool { return d.dismissed }

// GetBounds returns measured map-relative bounds for feedback composition.
func (d *CombatLootDialog) GetBounds() image.Rectangle { return d.bounds }

// Update accepts only scrolling and acknowledgement after discovery completes.
func (d *CombatLootDialog) Update() {
	if !d.ready || d.dismissed {
		return
	}
	if d.keyboard.IsKeyPressedAndRegisterRepeatable(ebiten.KeyUp) {
		if d.summary != nil {
			d.summary.scroll = max(0, d.summary.scroll-d.summary.scrollStep)
		} else {
			d.offset = max(0, d.offset-1)
		}
	}
	if d.keyboard.IsKeyPressedAndRegisterRepeatable(ebiten.KeyDown) {
		if d.summary != nil {
			d.summary.scroll = min(max(0, d.summary.contentHeight-d.summary.body.Dy()), d.summary.scroll+d.summary.scrollStep)
		} else {
			d.offset = min(max(0, d.revealed-d.capacity), d.offset+1)
		}
	}
	if !d.dismissed && (d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape)) {
		d.dismissed = true
		if d.onDismiss != nil {
			d.onDismiss()
		}
	}
}

// Draw measures the accumulated list and displays a keyboard-scrollable page.
func (d *CombatLootDialog) Draw(screen *ebiten.Image) {
	viewport := display.GetManager().MapPopupViewport(screen.Bounds())
	if viewport.Empty() {
		viewport = screen.Bounds()
		viewport.Max.X = viewport.Min.X + viewport.Dx()*lootMapWidthNumerator/lootMapWidthDenominator
	}
	if d.summary != nil {
		d.drawSummary(screen, viewport)

		return
	}
	rowHeight, headingHeight, footerHeight, padding := d.prepareDiscovery(screen.Bounds(), viewport)
	d.initializeIcons()
	left, top, width, height := d.bounds.Min.X, d.bounds.Min.Y, d.bounds.Dx(), d.bounds.Dy()
	FillClassicPixelRoundedRect(screen, d.bounds, padding/lootSides, ucolor.DialogBackground.RGBA())
	DrawClassicPixelRoundedFrame(screen, d.bounds, padding/lootSides, DialogRuleColor)
	d.heading.DrawPanelContent(screen, image.Pt(left+padding, top+padding), width-padding*lootSides, etext.AlignCenter)
	for row, index := 0, d.offset; index < min(d.revealed, d.offset+d.capacity); row, index = row+1, index+1 {
		rowTop := top + padding + headingHeight + row*rowHeight
		icon := d.icons[index]
		iconOptions := &ebiten.DrawImageOptions{}
		size := d.output.GetTotalHeightForLines(lootSides)
		scale := size / sprites.TileSize
		iconHeight, iconWidth := 0.0, 0.0
		if icon != nil {
			iconHeight, iconWidth = float64(icon.Bounds().Dy())*scale, float64(icon.Bounds().Dx())*scale
		}
		iconY, textY := lootRowContentTops(float64(rowTop), float64(rowHeight-padding), iconHeight,
			d.rows[index].GetTotalHeightForLines(d.rows[index].GetPanelContentLineCount()))
		iconOptions.GeoM.Scale(scale, scale)
		iconOptions.GeoM.Translate(float64(left+padding)+(size-iconWidth)/lootSides, iconY)
		if icon != nil {
			screen.DrawImage(icon, iconOptions)
		}
		rowStart := left + padding + int(math.Ceil(size)) + padding
		d.rows[index].DrawPanelContent(screen, image.Pt(rowStart, int(math.Round(textY))),
			left+width-padding-rowStart, etext.AlignStart)
	}
	d.output.DrawPanelContent(screen, image.Pt(left+padding, top+height-padding-footerHeight),
		width-padding*lootSides, etext.AlignCenter)
}

func (d *CombatLootDialog) prepareDiscovery(screenBounds, viewport image.Rectangle) (int, int, int, int) {
	padding := max(lootSides, viewport.Dx()/lootPaddingDivisor)
	width := viewport.Dx() - padding*lootOuterInsets
	rowHeight, headingHeight, footerHeight := d.prepareText(width, screenBounds.Dx(), padding)
	d.capacity = max(1, (viewport.Dy()-padding*lootOuterInsets-headingHeight-footerHeight)/rowHeight)
	height := min(d.capacity, len(d.entries))*rowHeight + headingHeight + footerHeight + padding*lootSides
	left := viewport.Min.X + (viewport.Dx()-width)/lootSides
	top := viewport.Min.Y + (viewport.Dy()-height)/lootSides
	d.bounds = image.Rect(left, top, left+width, top+height)
	d.offset = min(d.offset, max(0, d.revealed-d.capacity))

	return rowHeight, headingHeight, footerHeight, padding
}

func lootRowContentTops(top, height, iconHeight, textHeight float64) (float64, float64) {
	return top + (height-iconHeight)/lootSides, top + (height-textHeight)/lootSides
}

func (d *CombatLootDialog) prepareText(width, screenWidth, padding int) (int, int, int) {
	if d.output == nil || d.width != width {
		d.initializeText(width, screenWidth, padding)
	}
	d.output.Reset()
	d.output.AppendLine().Text(d.notice, DialogLabelColor).WordWrap().Done()
	if d.ready {
		PackControlHints(d.output, []ControlHint{{hintKeyUpDown, "Scroll"}, {"Enter/Esc", "Continue"}},
			width-padding*lootSides, etext.AlignStart)
	} else {
		d.output.AppendLine().Text("Revealing contents...", DialogControlHintColor).WordWrap().Done()
	}
	rowHeight := int(math.Ceil(d.output.GetTotalHeightForLines(lootSides)))
	for _, row := range d.rows {
		rowHeight = max(rowHeight, int(math.Ceil(row.GetTotalHeightForLines(row.GetPanelContentLineCount()))))
	}
	headingHeight := int(math.Ceil(d.heading.GetTotalHeightForLines(d.heading.GetPanelContentLineCount()))) + padding
	footerHeight := int(math.Ceil(d.output.GetTotalHeightForLines(d.output.GetPanelContentLineCount()))) + padding

	return rowHeight + padding, headingHeight, footerHeight
}

func (d *CombatLootDialog) initializeText(width, screenWidth, padding int) {
	font, output := GetScaledFontWithOutputForWidth(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: 1,
		MaxLines: lootMaximumTextRows, MaxChars: lootInitialTextChars,
	}, screenWidth)
	d.output, d.width = output, width
	d.heading = text.NewOutput(font, output.GetLineSpacing(), lootMaximumTextRows, lootInitialTextChars)
	charWidth, _ := font.MeasureString("M")
	output.SetMaxCharsPerLine(max(1, int(float64(width-padding*lootSides)/charWidth)))
	d.heading.SetMaxCharsPerLine(max(1, int(float64(width-padding*lootSides)/charWidth)))
	d.heading.AppendLine().Text(d.title, DialogHeaderColor).WordWrap().Done()
	if d.trapNotice != "" {
		d.heading.AppendLine().Text("TRAP: ", ucolor.CombatReportWarning).
			Text(d.trapNotice, DialogValueColor).WordWrap().Done()
	}
	iconWidth := int(math.Ceil(output.GetTotalHeightForLines(lootSides)))
	chars := max(1, int(float64(width-padding*(lootSides+1)-iconWidth)/charWidth))
	d.rows = make([]*text.Output, len(d.entries))
	for index, entry := range d.entries {
		row := text.NewOutput(font, output.GetLineSpacing(), lootMaximumTextRows, chars)
		row.AppendLine().Text(entry.Description, DialogValueColor).WordWrap().Done()
		d.rows[index] = row
	}
}

func (d *CombatLootDialog) initializeIcons() {
	if d.icons == nil {
		d.icons = make([]*ebiten.Image, len(d.entries))
		for index, entry := range d.entries {
			if entry.Sprite != indexes.NoSprites {
				d.icons[index] = trimLootIcon(d.sheet.GetRawSprite(entry.Sprite))
			}
		}
	}
}

func combatSummaryText(value string, colour color.Color, template *text.Output, width int) *text.Output {
	// Retain the whole source before wrapping, including embedded newlines. The
	// wrapped copy sizes its own buffer; long names cannot overwrite earlier rows.
	chars := max(1, utf8.RuneCountInString(value))
	output := text.NewOutput(template.Font, template.GetLineSpacing(), chars+1, chars)
	output.AppendLine().Text(value, colour).Done()

	return output.WrappedCopyForWidth(max(1, width))
}

func combatSummaryTextHeight(output *text.Output) int {
	return int(math.Ceil(output.GetTotalHeightForLines(output.GetPanelContentLineCount())))
}

func (d *CombatLootDialog) initializeSummaryText(width, screenWidth, padding int) {
	layout := d.summary
	_, d.output = GetScaledFontWithOutputForWidth(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: 1,
		MaxLines: lootMaximumTextRows, MaxChars: lootInitialTextChars,
	}, screenWidth)
	layout.footerSource = d.output
	_, heading := GetScaledFontWithOutputForWidth(FontConfig{
		Size: FontSizeTitle, LineSpacingMultiplier: 1,
		MaxLines: lootMaximumTextRows, MaxChars: lootInitialTextChars,
	}, screenWidth)
	innerWidth := width - padding*lootSides
	d.heading = combatSummaryText(d.title, DialogHeaderColor, heading, innerWidth)
	layout.outcomeOutput = combatSummaryText(layout.outcome, DialogValueColor, d.output, innerWidth)
	titleWidth, _ := heading.Font.MeasureString(d.title)
	outcomeWidth, _ := d.output.Font.MeasureString(layout.outcome)
	layout.outcomeTop = 0
	layout.headingHeight = max(combatSummaryTextHeight(d.heading), combatSummaryTextHeight(layout.outcomeOutput))
	if d.heading.GetPanelContentLineCount() > 1 || titleWidth+outcomeWidth+float64(padding) > float64(innerWidth) {
		layout.outcomeTop = combatSummaryTextHeight(d.heading)
		layout.headingHeight = layout.outcomeTop + combatSummaryTextHeight(layout.outcomeOutput)
	}
	layout.headingHeight += padding
	layout.scrollStep = int(math.Ceil(d.output.GetLineSpacing()))
	d.rows = make([]*text.Output, len(d.entries))
	layout.values = make([]*text.Output, len(d.entries))
	layout.rowHeights, layout.valueTops = make([]int, len(d.entries)), make([]int, len(d.entries))
	layout.contentHeight = 0
	iconSize := int(math.Ceil(d.output.GetTotalHeightForLines(lootSides)))
	for index, entry := range d.entries {
		textWidth := innerWidth
		if len(d.quantities) > 0 {
			textWidth -= iconSize + padding
		}
		d.rows[index] = combatSummaryText(entry.Description, DialogLabelColor, d.output, textWidth)
		rowHeight := combatSummaryTextHeight(d.rows[index])
		if index < len(d.quantities) {
			value := strconv.FormatUint(d.quantities[index], 10)
			layout.values[index] = combatSummaryText(value, DialogValueColor, d.output, textWidth)
			nameWidth, _ := d.output.Font.MeasureString(entry.Description)
			valueWidth, _ := d.output.Font.MeasureString(value)
			if d.rows[index].GetPanelContentLineCount() > 1 || nameWidth+valueWidth+float64(padding) > float64(textWidth) {
				layout.valueTops[index] = rowHeight
			}
			rowHeight = max(rowHeight, layout.valueTops[index]+combatSummaryTextHeight(layout.values[index]), iconSize)
		}
		layout.rowHeights[index] = rowHeight + padding
		layout.contentHeight += layout.rowHeights[index]
	}
	layout.experience.initialize(d.output, innerWidth, padding)
	layout.screenWidth, layout.padding, d.width = screenWidth, padding, width
}

func (d *CombatLootDialog) prepareSummaryFooter(width int, scrolling bool) {
	output := d.summary.footerSource
	d.summary.notice = nil
	if d.notice != "" {
		d.summary.notice = combatSummaryText(d.notice, DialogLabelColor, output, width)
	}
	output.Reset()
	if scrolling {
		AppendControlHints(output.AppendLine(), "[Up/Down] Scroll").Done()
	}
	AppendControlHints(output.AppendLine(), "[Enter/Esc] Continue").Done()
	d.output = output.WrappedCopyForWidth(width)
}

// summaryFooterHeight covers the notice, a padding of space, and the key hints.
func (d *CombatLootDialog) summaryFooterHeight() int {
	height := combatSummaryTextHeight(d.output)
	if notice := d.summary.notice; notice != nil {
		height += combatSummaryTextHeight(notice) + d.summary.padding
	}

	return height
}

func (d *CombatLootDialog) drawSummary(screen *ebiten.Image, viewport image.Rectangle) {
	d.prepareSummary(screen.Bounds(), viewport)
	d.initializeIcons()
	layout := d.summary
	padding := layout.padding
	innerWidth := d.bounds.Dx() - padding*lootSides
	frameUnit := getClassicUIPixelUnit(screen.Bounds().Dx(), screen.Bounds().Dy())
	FillClassicPixelRoundedRect(screen, d.bounds, frameUnit, ucolor.DialogBackground.RGBA())
	d.heading.DrawPanelContent(screen, d.bounds.Min.Add(image.Pt(padding, padding)), innerWidth, etext.AlignStart)
	layout.outcomeOutput.DrawPanelContent(screen,
		d.bounds.Min.Add(image.Pt(padding, padding+layout.outcomeTop)), innerWidth, etext.AlignEnd)
	layout.experience.draw(screen, d.bounds.Min.Add(image.Pt(padding, padding+layout.headingHeight)), innerWidth, padding)
	d.drawSummaryRows(screen, padding)
	hints := layout.footer.Min
	if layout.notice != nil {
		layout.notice.DrawPanelContent(screen, hints, innerWidth, etext.AlignCenter)
		hints.Y += combatSummaryTextHeight(layout.notice) + padding
	}
	d.output.DrawPanelContent(screen, hints, innerWidth, etext.AlignCenter)
	DrawClassicPixelRoundedFrame(screen, d.bounds, frameUnit, DialogRuleColor)
}

// prepareSummary measures text, bounds and scrolling without rasterizing the dialog.
func (d *CombatLootDialog) prepareSummary(screenBounds, viewport image.Rectangle) {
	layout := d.summary
	padding := max(lootSides, viewport.Dx()/lootPaddingDivisor)
	width := min(int(math.Round(float64(screenBounds.Dx())*combatSummaryWidthRatio)), viewport.Dx()-padding*lootSides)
	if d.output == nil || d.width != width || layout.screenWidth != screenBounds.Dx() || layout.padding != padding {
		d.initializeSummaryText(width, screenBounds.Dx(), padding)
	}
	innerWidth := width - padding*lootSides
	bottom := min(screenBounds.Min.Y+int(float64(screenBounds.Dy())*combatSummaryBottomRatio), viewport.Max.Y-padding)
	// The experience section adds to the limit, so the list keeps its room.
	maxHeight := min(viewport.Dy()/lootSides+layout.experience.height, bottom-viewport.Min.Y-padding)
	d.prepareSummaryFooter(innerWidth, false)
	fixedHeight := layout.headingHeight + layout.experience.height + d.summaryFooterHeight() + padding*lootSides
	if layout.contentHeight > maxHeight-fixedHeight {
		d.prepareSummaryFooter(innerWidth, true)
		fixedHeight = layout.headingHeight + layout.experience.height + d.summaryFooterHeight() + padding*lootSides
	}
	bodyHeight := min(layout.contentHeight, max(1, maxHeight-fixedHeight))
	height := fixedHeight + bodyHeight
	left := viewport.Min.X + (viewport.Dx()-width)/lootSides
	d.bounds = image.Rect(left, bottom-height, left+width, bottom)
	layout.body = image.Rect(left+padding, d.bounds.Min.Y+padding+layout.headingHeight+layout.experience.height,
		left+width-padding, bottom-padding-d.summaryFooterHeight())
	layout.footer = image.Rect(layout.body.Min.X, layout.body.Max.Y, layout.body.Max.X, bottom-padding)
	layout.scroll = min(layout.scroll, max(0, layout.contentHeight-bodyHeight))
}

func (d *CombatLootDialog) drawSummaryRows(screen *ebiten.Image, padding int) {
	layout := d.summary
	body, ok := screen.SubImage(layout.body).(*ebiten.Image)
	if !ok {
		panic("combat summary requires an Ebitengine subimage")
	}
	top := layout.body.Min.Y - layout.scroll
	iconSize := d.output.GetTotalHeightForLines(lootSides)
	for index, row := range d.rows {
		height := layout.rowHeights[index]
		if top+height > layout.body.Min.Y && top < layout.body.Max.Y {
			left := layout.body.Min.X
			if len(d.quantities) > 0 {
				left += int(math.Ceil(iconSize)) + padding
			}
			textHeight := combatSummaryTextHeight(row)
			if layout.values[index] != nil {
				textHeight = max(textHeight, layout.valueTops[index]+combatSummaryTextHeight(layout.values[index]))
			}
			_, textTop := lootRowContentTops(float64(top), float64(height-padding), iconSize, float64(textHeight))
			if icon := d.icons[index]; icon != nil {
				scale := iconSize / sprites.TileSize
				iconTop, _ := lootRowContentTops(float64(top), float64(height-padding),
					float64(icon.Bounds().Dy())*scale, float64(textHeight))
				op := &ebiten.DrawImageOptions{}
				op.GeoM.Scale(scale, scale)
				op.GeoM.Translate(float64(layout.body.Min.X)+(iconSize-float64(icon.Bounds().Dx())*scale)/lootSides, iconTop)
				body.DrawImage(icon, op)
			}
			row.DrawPanelContent(body, image.Pt(left, int(math.Round(textTop))), layout.body.Max.X-left, etext.AlignStart)
			if value := layout.values[index]; value != nil {
				value.DrawPanelContent(body, image.Pt(left, int(math.Round(textTop))+layout.valueTops[index]),
					layout.body.Max.X-left, etext.AlignEnd)
			}
		}
		top += height
	}
}

// Trim once per dialog so an icon's transparent atlas padding (notably the
// gold sack's top rows) cannot move its visible centre below the description.
// Drawing keeps the original pixel scale instead of enlarging the cropped art.
func trimLootIcon(source *ebiten.Image) *ebiten.Image {
	const channels = 4
	var pixels [sprites.TileSize * sprites.TileSize * channels]byte
	source.ReadPixels(pixels[:])
	left, top, right, bottom := sprites.TileSize, sprites.TileSize, 0, 0
	for row := range sprites.TileSize {
		for column := range sprites.TileSize {
			if pixels[(row*sprites.TileSize+column)*channels+channels-1] == 0 {
				continue
			}
			left, top = min(left, column), min(top, row)
			right, bottom = max(right, column+1), max(bottom, row+1)
		}
	}
	if right <= left || bottom <= top {
		return source
	}

	return ebiten.NewImageFromImage(source.SubImage(image.Rect(left, top, right, bottom)))
}
