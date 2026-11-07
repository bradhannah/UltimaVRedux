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
	indicatorX := content.Max.X - indicatorWidth
	if start > 0 {
		drawMasterDetailCenteredLine(
			screen,
			image.Rect(indicatorX, content.Min.Y, content.Max.X, content.Min.Y+rowHeight),
			"^",
			DialogScrollIndicator,
			font,
			output,
		)
	}
	if end < count {
		top := content.Min.Y + max(0, end-start-1)*rowHeight
		drawMasterDetailCenteredLine(
			screen,
			image.Rect(indicatorX, top, content.Max.X, min(content.Max.Y, top+rowHeight)),
			"v",
			DialogScrollIndicator,
			font,
			output,
		)
	}
}
