package widgets

import (
	"image"
	stdcolor "image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/text"
)

func drawMasterDetailCenteredLine(
	screen *ebiten.Image,
	rect image.Rectangle,
	value string,
	lineColor stdcolor.Color,
	font *text.UltimaFont,
	output *text.Output,
) {
	output.SetMaxCharsPerLine(max(1, font.CharsForWidth(max(1, rect.Dx()))))
	output.Reset()
	output.AppendLine().WithAlignment(etext.AlignCenter).Text(value, lineColor).Done()
	top := rect.Min.Y + max(0, (rect.Dy()-int(math.Ceil(output.GetFontHeight())))/2) //nolint:mnd // Geometric center.
	output.DrawPanelContent(screen, image.Pt(rect.Min.X, top), max(1, rect.Dx()), etext.AlignCenter)
}

// rowDetailIndent sets a two-line row's second line (a spell's meaning) in from
// its first, so each pair reads as one entry.
const rowDetailIndent = "  "

// fillSelectedRow draws the shared white bar behind a row drawn in DialogSelectedText.
func fillSelectedRow(screen *ebiten.Image, row image.Rectangle) {
	vector.FillRect(screen, float32(row.Min.X), float32(row.Min.Y), float32(row.Dx()), float32(row.Dy()),
		DialogSelectedBackground, false)
}

func drawMasterDetailScrollIndicators(
	screen *ebiten.Image,
	content image.Rectangle,
	start int,
	end int,
	count int,
	rowHeight int,
	indicatorWidth int,
	font *text.UltimaFont,
	output *text.Output,
) {
	top := content.Min.Y + max(0, end-start-1)*rowHeight
	drawMasterDetailScrollMarks(screen,
		image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+rowHeight),
		image.Rect(content.Min.X, top, content.Max.X, min(content.Max.Y, top+rowHeight)),
		start > 0, end < count, indicatorWidth, font, output)
}

// drawMasterDetailScrollMarks marks rows hidden above and below at the right of
// the first and last visible rows, which may differ in height.
func drawMasterDetailScrollMarks(
	screen *ebiten.Image, first, last image.Rectangle, above, below bool,
	indicatorWidth int, font *text.UltimaFont, output *text.Output,
) {
	if above {
		first.Min.X = first.Max.X - indicatorWidth
		drawMasterDetailCenteredLine(screen, first, "^", DialogScrollIndicator, font, output)
	}
	if below {
		last.Min.X = last.Max.X - indicatorWidth
		drawMasterDetailCenteredLine(screen, last, "v", DialogScrollIndicator, font, output)
	}
}

// DrawActiveTabPlate fills the plate behind the open tab's title. Every tabbed
// dialog draws its open tab on it in DialogActiveTabText and its other tabs in
// DialogTabColor, so tabs look the same across the game. The title is titleWidth
// wide, centred across cell, with its glyphs between textTop and textTop+textHeight.
func DrawActiveTabPlate(
	screen *ebiten.Image, cell image.Rectangle, titleWidth float64, textTop, textHeight, pixelUnit int,
) {
	const (
		sideUnits = 3
		halves    = 2
	)
	side, width := sideUnits*pixelUnit, int(math.Ceil(titleWidth))
	left := cell.Min.X + (cell.Dx()-width)/halves
	plate := image.Rect(left-side, textTop-pixelUnit, left+width+side, textTop+textHeight+pixelUnit)
	FillClassicPixelRoundedRect(screen, plate, pixelUnit, DialogActiveTabBackground)
}
