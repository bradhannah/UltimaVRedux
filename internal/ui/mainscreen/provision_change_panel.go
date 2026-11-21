package mainscreen

import (
	"image"
	"math"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/profiling"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	provisionPanelColumns       = 3
	provisionPanelMarginDivisor = 256
	provisionPanelTopInset      = 2
	provisionPanelTitle         = "Recent provisions"
	provisionPanelSides         = 2
	provisionPanelOuterInsets   = 4
)

// ProvisionChangePanel is a display-only strip above the normal merchant shell.
// It deliberately has no keyboard, Widget, DialogStack, or gameplay ownership.
type ProvisionChangePanel struct {
	rows         [provisionChangeCount]ProvisionChange
	outputs      [provisionChangeCount]*text.Output
	header       *text.Output
	bounds       image.Rectangle
	cells        [provisionChangeCount]image.Rectangle
	headerBounds image.Rectangle
	viewport     image.Rectangle
	screenWidth  int
	foodHeading  *text.Output
	foodDetails  *text.Output
	foodBounds   image.Rectangle
}

// Update prepares measured text only when content or viewport changes. At normal
// quantities all six resources fit in two rows above the merchant's 10% top edge.
// Very long accumulated amounts reduce columns rather than truncate signed values.
func (p *ProvisionChangePanel) Update(
	rows [provisionChangeCount]ProvisionChange, viewport image.Rectangle, screenWidth int,
) {
	if screenWidth <= 0 {
		panic("provision typography requires a positive logical screen width")
	}
	if p.header != nil && p.rows == rows && p.viewport == viewport && p.screenWidth == screenWidth {
		return
	}
	if p.header == nil || p.screenWidth != screenWidth {
		p.initializeOutputs(screenWidth)
	}
	p.screenWidth = screenWidth
	p.rows = rows
	p.viewport = viewport
	p.populateOutputs()
	p.layout()
}

func (p *ProvisionChangePanel) initializeOutputs(width int) {
	profiling.Count(profiling.ProvisionFontRebuilds)
	font, header := widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size: widgets.FontSizeBody, LineSpacingMultiplier: 1,
		MaxLines: 1, MaxChars: len(provisionPanelTitle),
	}, width)
	p.header = header
	p.header.AppendLine().Text(provisionPanelTitle, widgets.DialogHeaderColor).Done()
	for index := range p.outputs {
		p.outputs[index] = text.NewOutput(font, header.GetLineSpacing(), 1, 1)
	}
	_, p.foodHeading = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size: widgets.FontSizeTitle, LineSpacingMultiplier: 1,
		MaxLines: 1, MaxChars: provisionFoodChars,
	}, width)
	_, p.foodDetails = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size: widgets.FontSizeBody, LineSpacingMultiplier: 1,
		MaxLines: provisionPanelSides, MaxChars: provisionFoodChars,
	}, width)
}

func (p *ProvisionChangePanel) populateOutputs() {
	p.populateFoodLoss()
	for index, row := range p.rows {
		output := p.outputs[index]
		output.Reset()
		if row.Name == "" {
			continue
		}
		gain, loss := provisionSignedAmounts(row)
		total := strconv.FormatUint(uint64(row.Total), 10)
		// Layout measures the complete composed string and allocates a cell at
		// least that wide, so this physical one-line output must not wrap.
		output.SetMaxCharsPerLine(len(row.Name + gain + loss + "  Now " + total))
		output.AppendLine().Text(row.Name, widgets.DialogLabelColor).
			Text(gain, widgets.DialogDeltaPositive).Text(loss, widgets.DialogDeltaNegative).
			Text("  Now ", widgets.DialogLabelColor).Text(total, widgets.DialogValueColor).Done()
	}
}

func provisionSignedAmounts(row ProvisionChange) (string, string) {
	var gain, loss string
	if row.Gained != 0 {
		gain = " +" + strconv.FormatUint(row.Gained, 10)
	}
	if row.Lost != 0 {
		loss = " -" + strconv.FormatUint(row.Lost, 10)
	}

	return gain, loss
}

func (p *ProvisionChangePanel) layout() {
	p.bounds = image.Rect(0, 0, 0, 0)
	p.cells = [provisionChangeCount]image.Rectangle{}
	p.foodBounds, p.headerBounds = image.Rect(0, 0, 0, 0), image.Rect(0, 0, 0, 0)
	if p.hasFoodLoss() {
		p.layoutFoodLoss()

		return
	}
	count, cellWidth := 0, 0
	for index, row := range p.rows {
		if row.Name == "" {
			continue
		}
		width, _ := p.header.Font.MeasureString(p.outputs[index].GetOutputStr(false))
		cellWidth = max(cellWidth, int(math.Ceil(width)))
		count++
	}
	if count == 0 {
		return
	}
	padding := max(1, p.viewport.Dx()/provisionPanelMarginDivisor)
	gap := padding * provisionPanelSides
	available := p.viewport.Dx() - padding*provisionPanelOuterInsets
	columns := min(provisionPanelColumns, count, max(1, (available+gap)/(cellWidth+gap)))
	rowHeight := int(math.Ceil(p.header.GetTotalHeightForLines(1)))
	headerWidth, _ := p.header.Font.MeasureString(provisionPanelTitle)
	width := max(int(math.Ceil(headerWidth)), columns*cellWidth+(columns-1)*gap) + provisionPanelSides*padding
	rowCount := (count + columns - 1) / columns
	height := (rowCount+1)*rowHeight + (rowCount+provisionPanelSides)*padding
	left := p.viewport.Min.X + (p.viewport.Dx()-width)/provisionPanelSides
	top := p.viewport.Min.Y + padding*provisionPanelTopInset
	p.bounds = image.Rect(left, top, left+width, top+height)
	p.headerBounds = image.Rect(left+padding, top+padding, left+width-padding, top+padding+rowHeight)
	indexInGrid := 0
	for index, row := range p.rows {
		if row.Name == "" {
			continue
		}
		x := left + padding + (indexInGrid%columns)*(cellWidth+gap)
		y := p.headerBounds.Max.Y + padding + (indexInGrid/columns)*(rowHeight+padding)
		p.cells[index] = image.Rect(x, y, x+cellWidth, y+rowHeight)
		indexInGrid++
	}
}

// Bounds returns the prepared screen-space bounds for scene composition.
func (p *ProvisionChangePanel) Bounds() image.Rectangle {
	return p.bounds
}

// Place translates the measured panel and all its rows without rebuilding text.
func (p *ProvisionChangePanel) Place(origin image.Point) {
	if p.bounds.Empty() {
		return
	}
	delta := origin.Sub(p.bounds.Min)
	p.bounds = p.bounds.Add(delta)
	if !p.headerBounds.Empty() {
		p.headerBounds = p.headerBounds.Add(delta)
	}
	if !p.foodBounds.Empty() {
		p.foodBounds = p.foodBounds.Add(delta)
	}
	for index, cell := range p.cells {
		if !cell.Empty() {
			p.cells[index] = cell.Add(delta)
		}
	}
}

// Draw renders prepared output without changing expiry or focus. The scene owns
// placement relative to modals so feedback cannot obscure their content.
func (p *ProvisionChangePanel) Draw(screen *ebiten.Image) {
	if p.bounds.Empty() {
		return
	}
	unit := max(1, p.viewport.Dx()/provisionPanelMarginDivisor/provisionPanelSides)
	widgets.FillClassicPixelRoundedRect(screen, p.bounds, unit, ucolor.DialogBackground.RGBA())
	widgets.DrawClassicPixelRoundedFrame(screen, p.bounds, unit, widgets.DialogRuleColor)
	if !p.headerBounds.Empty() {
		p.header.DrawPanelContent(screen, p.headerBounds.Min, p.headerBounds.Dx(), etext.AlignStart)
	}
	if !p.foodBounds.Empty() {
		padding := max(1, p.viewport.Dx()/provisionPanelMarginDivisor)
		content := p.foodBounds.Inset(padding)
		p.foodHeading.DrawPanelContent(screen, content.Min, content.Dx(), etext.AlignCenter)
		content.Min.Y += int(math.Ceil(p.foodHeading.GetTotalHeightForLines(1))) + padding
		p.foodDetails.DrawPanelContent(screen, content.Min, content.Dx(), etext.AlignCenter)
	}
	for index, cell := range p.cells {
		if !cell.Empty() {
			p.outputs[index].DrawPanelContent(screen, cell.Min, cell.Dx(), etext.AlignStart)
		}
	}
}
