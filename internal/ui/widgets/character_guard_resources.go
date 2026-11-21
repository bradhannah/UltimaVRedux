package widgets

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// CharacterSelectionResources is an opening snapshot, not a rest prediction.
// MP deliberately preserves the same byte-valued getter contract as the party.
type CharacterSelectionResources struct {
	CurrentHP uint16
	MaximumHP uint16
	CurrentMP byte
	MaximumMP byte
}

const (
	guardResourceFooterLines        = 2
	guardResourceFooterBufferLines  = guardResourceFooterLines * 3
	guardResourceDetailLines        = 16
	guardResourceConditionColumn    = 2
	guardResourcePreferredFontPoint = 8
	guardResourceMinimumFontPoint   = 6
	guardResourceDetailPadding      = 3
	guardResourceNameLabel          = "Name"
	guardResourceConditionLabel     = "Condition"
)

func characterGuardResourceValues(option CharacterSelectionOption) [3]string {
	if option.Resources == nil {
		return [3]string{"", "", option.Status}
	}
	resources := option.Resources

	return [3]string{
		fmt.Sprintf("%d/%d", resources.CurrentHP, resources.MaximumHP),
		fmt.Sprintf("%d/%d", resources.CurrentMP, resources.MaximumMP),
		option.Status,
	}
}

// The guard uses a content-sized ledger above its explanation. A side-by-side
// detail pane would steal space from names or complete resource values at 320x200.
func (s *characterRosterDialogShell) ensureGuardResourceLayout(width, height int, view characterRosterDialogView) {
	if s.titleOutput != nil && width == s.lastScreenWidth && height == s.lastScreenHeight &&
		len(s.layout.rowRects) == len(view.options) {
		return
	}
	s.lastScreenWidth, s.lastScreenHeight = width, height
	s.guardResourcesCompact = false
	for reduction := 0.0; ; reduction++ {
		s.createGuardResourceOutputs(width, reduction)
		s.calculateGuardResourceLayout(width, height, view)
		if !s.guardResourcesCompact && !s.guardResourceNamesFit(view.options) {
			s.guardResourcesCompact = true
			s.calculateGuardResourceLayout(width, height, view)
		}
		if s.guardResourceLayoutFits(width, height, view) ||
			s.titleOutput.Font.TextFace.Size <= guardResourceMinimumFontPoint {
			return
		}
	}
}

// Try the normal typography first. Only this guard card can step down to the
// compact minimum, and only after measuring every row and wrapped explanation.
func (s *characterRosterDialogShell) createGuardResourceOutputs(width int, reduction float64) {
	makeOutput := func(size StandardFontSize, lines int) (*text.UltimaFont, *text.Output) {
		preferred := max(guardResourcePreferredFontPoint, text.GetScaledNumberToWidth(width, float64(size)))
		point := max(guardResourceMinimumFontPoint, preferred-reduction)

		return newScaledFontWithOutput(FontConfig{
			Size: size, LineSpacingMultiplier: characterRosterDialogLineSpacing,
			MaxLines: lines, MaxChars: characterRosterDialogFooterMaximumChars,
		}, point, func(value float64) float64 { return value * point / float64(size) })
	}
	_, s.titleOutput = makeOutput(FontSizeTitle, 1)
	s.bodyFont, s.rowOutput = makeOutput(FontSizeCompact, 1)
	_, s.headerOutput = makeOutput(FontSizeCompact, 1)
	_, s.detailOutput = makeOutput(FontSizeCompact, guardResourceDetailLines)
	// Intermediate candidates may wrap the two hints to more physical lines. Do
	// not overwrite the beginning before measuring whether that candidate fits.
	_, s.footerOutput = makeOutput(FontSizeBody, guardResourceFooterBufferLines)
}

func (s *characterRosterDialogShell) guardResourceNamesFit(options []CharacterSelectionOption) bool {
	for index, option := range options {
		columns := s.guardResourceColumns(s.layout.rowRects[index], options)
		// Reserve every identity marker before selecting the layout, including
		// focus and the active marker even when this row is not currently focused.
		width, _ := s.bodyFont.MeasureString(fmt.Sprintf(">X%d %s*", max(0, option.PartyPosition), option.Name))
		if int(math.Ceil(width)) > columns[0].Dx() {
			return false
		}
	}

	return true
}

func (s *characterRosterDialogShell) guardResourceLayoutFits(width, height int, view characterRosterDialogView) bool {
	workspace := s.guardResourceWorkspace(width, height).Inset(s.layout.pixelUnit * characterRosterDialogMarginUnits)
	titleWidth, _ := s.titleOutput.Font.MeasureString(view.title)
	if !s.layout.panelRect.In(workspace) || titleWidth > float64(s.layout.headerRect.Dx()) ||
		!s.guardResourceNamesFit(view.options) {
		return false
	}
	columns := s.guardResourceColumns(s.layout.rowRects[0], view.options)
	for index, column := range columns {
		if !column.In(s.layout.rowRects[0]) {
			return false
		}
		for _, other := range columns[index+1:] {
			if column.Overlaps(other) {
				return false
			}
		}
	}

	return true
}

func (s *characterRosterDialogShell) guardResourceWorkspace(width, height int) image.Rectangle {
	return image.Rect(
		int(float64(width)*s.placement.StartPercentX), int(float64(height)*s.placement.StartPercentY),
		int(float64(width)*s.placement.EndPercentX), int(float64(height)*s.placement.EndPercentY),
	)
}

func guardOutputHeight(output *text.Output, lines int) int {
	return int(math.Ceil(output.GetTotalHeightForLines(lines)))
}

func (s *characterRosterDialogShell) appendGuardDetail(option CharacterSelectionOption) {
	s.detailOutput.Reset()
	if len(option.Facts) > 0 {
		line := s.detailOutput.AppendLine()
		for index, fact := range option.Facts {
			if index > 0 {
				line.Text("   ", DialogLabelColor)
			}
			line.Text(fact.Label+": ", DialogLabelColor).Text(fact.Value, DialogValueColor)
		}
		line.WordWrap().Done()
	}
	for _, detail := range option.Details {
		s.detailOutput.AppendLine().Text(detail, characterRosterDetailColor(option.State)).WordWrap().Done()
	}
}

func (s *characterRosterDialogShell) appendGuardFooter(footer string, width int) {
	s.footerOutput.Reset()
	PackControlHints(s.footerOutput, ParseControlHints(footer), width, etext.AlignCenter)
}

// All seven rows and the longest explanation reserve measured space at opening
// and resize. Inspection never resizes the card or displaces its footer.
//
//nolint:funlen // Publish the mutually constrained roster, explanation and footer geometry in one measured pass.
func (s *characterRosterDialogShell) calculateGuardResourceLayout(width, height int, view characterRosterDialogView) {
	workspace := s.guardResourceWorkspace(width, height)
	unit := getClassicUIPixelUnit(width, height)
	padding := unit * characterRosterDialogPaddingUnits
	if s.guardResourcesCompact {
		// Each member already occupies two lines. One logical pixel of inset
		// preserves separation without spending the minimum viewport on gutters.
		padding = unit
	}
	panel := workspace.Inset(unit * characterRosterDialogMarginUnits)
	panelWidth := min(panel.Dx(), s.guardResourcePreferredWidth(view, padding))
	panel.Min.X = workspace.Min.X + (workspace.Dx()-panelWidth)/characterRosterDialogCenterDivisor
	panel.Max.X = panel.Min.X + panelWidth
	contentWidth := panel.Dx() - padding*characterRosterDialogCenterDivisor
	s.detailOutput.SetMaxCharsPerLine(max(1, s.bodyFont.CharsForWidth(contentWidth)))
	detailLines := 1
	for _, option := range view.options {
		s.appendGuardDetail(option)
		detailLines = max(detailLines, s.detailOutput.GetPanelContentLineCount())
	}
	s.appendGuardFooter(view.footer, contentWidth)
	headerHeight := guardOutputHeight(s.titleOutput, 1) + padding*characterRosterDialogCenterDivisor
	rowLines := 1
	if s.guardResourcesCompact {
		rowLines = characterRosterDialogCenterDivisor
	}
	columnHeight := guardOutputHeight(s.headerOutput, 1)*rowLines + padding
	rowHeight := (guardOutputHeight(s.rowOutput, 1) + padding) * rowLines
	rosterHeight := columnHeight + len(view.options)*rowHeight + padding*characterRosterDialogCenterDivisor
	detailHeight := guardOutputHeight(s.headerOutput, 1) + guardOutputHeight(s.detailOutput, detailLines) +
		padding*guardResourceDetailPadding
	footerHeight := guardOutputHeight(s.footerOutput, s.footerOutput.GetPanelContentLineCount()) +
		padding*characterRosterDialogCenterDivisor
	panelHeight := headerHeight + rosterHeight + detailHeight + footerHeight
	top := workspace.Min.Y + (workspace.Dy()-panelHeight)/characterRosterDialogCenterDivisor
	panel.Min.Y, panel.Max.Y = top, top+panelHeight
	header := image.Rect(panel.Min.X, top, panel.Max.X, top+headerHeight)
	roster := image.Rect(panel.Min.X, header.Max.Y, panel.Max.X, header.Max.Y+rosterHeight)
	detail := image.Rect(panel.Min.X, roster.Max.Y, panel.Max.X, roster.Max.Y+detailHeight)
	footer := image.Rect(panel.Min.X, detail.Max.Y, panel.Max.X, panel.Max.Y)
	rosterBody := roster.Inset(padding)
	rosterBody.Min.Y += columnHeight
	detailBody := detail.Inset(padding)
	detailBody.Min.Y += guardOutputHeight(s.headerOutput, 1) + padding
	rows := make([]image.Rectangle, len(view.options))
	for index := range rows {
		y := rosterBody.Min.Y + index*rowHeight
		rows[index] = image.Rect(rosterBody.Min.X, y, rosterBody.Max.X, y+rowHeight)
	}
	s.layout = characterRosterDialogLayout{
		panelRect: panel, headerRect: header, rosterRect: roster, detailRect: detail, footerRect: footer,
		rosterBodyRect: rosterBody, detailBodyRect: detailBody, detailTextRect: detailBody,
		portraitRect: image.Rect(0, 0, 0, 0), rowRects: rows, padding: padding, pixelUnit: unit,
	}
}

func (s *characterRosterDialogShell) guardResourcePreferredWidth(view characterRosterDialogView, padding int) int {
	widths := s.guardResourceColumnWidths(view.options)
	gap := padding * characterRosterDialogCenterDivisor
	preferred := (len(widths) - 1) * gap
	for _, width := range widths {
		preferred += width
	}
	if s.guardResourcesCompact {
		preferred = max(widths[0]+widths[3], widths[1]+widths[2]) + gap
	}
	titleWidth, _ := s.titleOutput.Font.MeasureString(view.title)
	preferred = max(preferred, int(math.Ceil(titleWidth)))
	// Reserve the two-row footer: actions on the first row, then the final cancel hint.
	hints := ParseControlHints(view.footer)
	for _, row := range [][]ControlHint{hints[:len(hints)-1], hints[len(hints)-1:]} {
		footerWidth, _ := s.footerOutput.Font.MeasureString(joinControlHints(row, controlHintSeparator))
		preferred = max(preferred, int(math.Ceil(footerWidth)))
	}

	return preferred + padding*characterRosterDialogCenterDivisor
}

func (s *characterRosterDialogShell) guardResourceColumnWidths(options []CharacterSelectionOption) [4]int {
	widths := [4]int{}
	for index, label := range [4]string{guardResourceNameLabel, "HP", "MP", guardResourceConditionLabel} {
		width, _ := s.headerOutput.Font.MeasureString(label)
		widths[index] = int(math.Ceil(width))
	}
	for _, option := range options {
		nameWidth, _ := s.bodyFont.MeasureString(fmt.Sprintf(">X%d %s*", max(0, option.PartyPosition), option.Name))
		widths[0] = max(widths[0], int(math.Ceil(nameWidth)))
		for index, value := range characterGuardResourceValues(option) {
			width, _ := s.bodyFont.MeasureString(value)
			widths[index+1] = max(widths[index+1], int(math.Ceil(width)))
		}
	}

	return widths
}

func (s *characterRosterDialogShell) guardResourceColumns(
	row image.Rectangle, options []CharacterSelectionOption,
) [4]image.Rectangle {
	widths := s.guardResourceColumnWidths(options)
	gap := s.layout.padding * characterRosterDialogCenterDivisor
	if s.guardResourcesCompact {
		middle := row.Min.Y + row.Dy()/characterRosterDialogCenterDivisor
		nameWidth := max(0, min(widths[0], row.Dx()-widths[3]-gap))
		conditionLeft := row.Min.X + nameWidth + gap
		mpLeft := row.Min.X + widths[1] + gap

		return [4]image.Rectangle{
			image.Rect(row.Min.X, row.Min.Y, row.Min.X+nameWidth, middle),
			image.Rect(row.Min.X, middle, row.Min.X+widths[1], row.Max.Y),
			image.Rect(mpLeft, middle, mpLeft+widths[2], row.Max.Y),
			image.Rect(conditionLeft, row.Min.Y, conditionLeft+widths[3], middle),
		}
	}
	columns := [4]image.Rectangle{}
	widths[0] = max(0, min(widths[0], row.Dx()-widths[1]-widths[2]-widths[3]-(len(widths)-1)*gap))
	left := row.Min.X
	for index, width := range widths {
		columns[index] = image.Rect(left, row.Min.Y, left+width, row.Max.Y)
		left += width + gap
	}

	return columns
}

func (s *characterRosterDialogShell) drawGuardResourceRoster(screen *ebiten.Image, view characterRosterDialogView) {
	header := s.layout.rosterRect.Inset(s.layout.padding)
	header.Max.Y = s.layout.rosterBodyRect.Min.Y - s.layout.padding
	labels := [4]string{guardResourceNameLabel, "HP", "MP", guardResourceConditionLabel}
	for index, rect := range s.guardResourceColumns(header, view.options) {
		s.headerOutput.Reset()
		s.headerOutput.AppendLine().Text(labels[index], DialogLabelColor).Done()
		drawGuardResourceOutput(screen, s.headerOutput, rect, etext.AlignStart)
	}
	for index, option := range view.options {
		row := s.layout.rowRects[index]
		columns := s.guardResourceColumns(row, view.options)
		selected := index == view.selected
		s.appendRosterRow(option, selected, columns[0].Dx())
		nameRect := columns[0]
		nameRect.Min.Y += (nameRect.Dy() - guardOutputHeight(s.rowOutput, 1)) / characterRosterDialogCenterDivisor
		drawGuardResourceOutput(screen, s.rowOutput, nameRect, etext.AlignStart)
		for field, value := range characterGuardResourceValues(option) {
			s.appendGuardResourceCell(option, selected, field, value)
			rect := columns[field+1]
			rect.Min.Y += (rect.Dy() - guardOutputHeight(s.rowOutput, 1)) / characterRosterDialogCenterDivisor
			drawGuardResourceOutput(screen, s.rowOutput, rect, etext.AlignStart)
		}
	}
}

func (s *characterRosterDialogShell) appendGuardResourceCell(
	option CharacterSelectionOption, selected bool, field int, value string,
) {
	colour := characterRosterRowColor(option.State, selected)
	if field == guardResourceConditionColumn {
		colour = option.statusColor(colour)
	}
	s.rowOutput.Reset()
	s.rowOutput.AppendLine().Text(value, colour).Done()
}

// Output's width controls alignment, not clipping. Keep each measured cell and
// explanation inside its own pane without allocating a second rendering surface.
func drawGuardResourceOutput(screen *ebiten.Image, output *text.Output, rect image.Rectangle, align etext.Align) {
	clipped, ok := screen.SubImage(rect).(*ebiten.Image)
	if !ok {
		panic("guard resource output requires an Ebitengine subimage")
	}
	output.DrawPanelContent(clipped, rect.Min, rect.Dx(), align)
}

func (s *characterRosterDialogShell) drawGuardResources(screen *ebiten.Image, view characterRosterDialogView) {
	bounds := screen.Bounds()
	s.ensureGuardResourceLayout(bounds.Dx(), bounds.Dy(), view)
	panel := s.layout.panelRect
	vector.FillRect(screen, float32(panel.Min.X), float32(panel.Min.Y), float32(panel.Dx()), float32(panel.Dy()),
		pkgcolor.DialogBackground.RGBA(), false)
	s.drawHeader(screen, view.title)
	s.drawGuardResourceRoster(screen, view)
	option := view.options[view.selected]
	s.headerOutput.Reset()
	s.headerOutput.AppendLine().Text(option.Name, DialogHeaderColor).Done()
	detailHeader := s.layout.detailRect.Inset(s.layout.padding)
	detailHeader.Max.Y = s.layout.detailBodyRect.Min.Y - s.layout.padding
	drawGuardResourceOutput(screen, s.headerOutput, detailHeader, etext.AlignStart)
	s.appendGuardDetail(option)
	drawGuardResourceOutput(screen, s.detailOutput, s.layout.detailBodyRect, etext.AlignStart)
	footer := s.layout.footerRect.Inset(s.layout.padding)
	s.appendGuardFooter(view.footer, footer.Dx())
	drawGuardResourceOutput(screen, s.footerOutput, footer, etext.AlignCenter)
	DrawClassicPixelRoundedFrame(screen, panel, s.layout.pixelUnit, DialogRuleLightColor)
	for _, y := range []int{s.layout.headerRect.Max.Y, s.layout.detailRect.Min.Y, s.layout.footerRect.Min.Y} {
		vector.FillRect(screen, float32(panel.Min.X), float32(y), float32(panel.Dx()),
			float32(s.layout.pixelUnit), DialogRuleColor, false)
	}
}
