package widgets

import (
	"image"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	runicReferenceColumns       = 3
	runicReferenceRows          = 10
	runicReferenceExtraRows     = 3 // Title, gap, and close hint.
	runicReferencePaddingUnits  = 4
	runicReferencePixelsPerUnit = 160
)

// RunicReferenceFunctionKey toggles the rune chart exactly like Ctrl+R.
const RunicReferenceFunctionKey = ebiten.KeyF7

// The alphabet uses the same byte-indexed font as signs and conversation.
// Dungeon graffiti includes compound forms (ref:SIGN-0005): [E, WRO],
// D\\P, and LO_. See docs/gamefacts/SIGNS_AND_FONTS.md.
//
//nolint:gochecknoglobals // Immutable glyph catalog; Go has no constant arrays.
var runicReferenceEntries = [...]struct{ glyph, label string }{
	{"A", "A"}, {"B", "B"}, {"C", "C"}, {"D", "D"}, {"E", "E"},
	{"F", "F"}, {"G", "G"}, {"H", "H"}, {"I", "I"}, {"J", "J"},
	{"K", "K"}, {"L", "L"}, {"M", "M"}, {"N", "N"}, {"O", "O"},
	{"P", "P"}, {"Q", "Q"}, {"R", "R"}, {"S", "S"}, {"T", "T"},
	{"U", "U"}, {"V", "V"}, {"W", "W"}, {"X", "X"}, {"Y", "Y"},
	{"Z", "Z"}, {"[", "TH"}, {"\\", "EE"}, {"]", "NG"}, {"_", "ST"},
}

// RunicReferencePanel is a passive reference, with no input or dialog focus.
// GameScene supplies its bounds and owns visibility and session lifetime.
type RunicReferencePanel struct {
	bounds     image.Rectangle
	content    image.Rectangle
	header     *text.Output
	footer     *text.Output
	cells      [len(runicReferenceEntries)]*text.Output
	cellBounds [len(runicReferenceEntries)]image.Rectangle
	pixelUnit  int
}

func (p *RunicReferencePanel) ensureLayout(bounds image.Rectangle, font *text.UltimaFont, lineSpacing float64) {
	if p.header != nil && p.bounds == bounds && p.header.Font == font && p.header.GetLineSpacing() == lineSpacing {
		return
	}
	p.bounds = bounds
	p.pixelUnit = max(1, bounds.Dx()/runicReferencePixelsPerUnit)
	p.content = bounds.Inset(runicReferencePaddingUnits * p.pixelUnit)
	cellWidth := p.content.Dx() / runicReferenceColumns
	rowHeight := p.content.Dy() / (runicReferenceRows + runicReferenceExtraRows)
	// Reuse the main output's typography: bitmap rune size follows line spacing,
	// so matching only the Latin font size would still leave oversized runes.
	newOutput := func(width int) *text.Output {
		return text.NewOutput(font, lineSpacing, 1, font.CharsForWidth(width))
	}
	p.header = newOutput(p.content.Dx())
	p.header.AppendLine().Yellow("RUNIC ALPHABET").Done()
	p.footer = newOutput(p.content.Dx())
	AppendControlHints(p.footer.AppendLine(), "[Ctrl+R] Close").Done()
	for index, entry := range runicReferenceEntries {
		column, row := index/runicReferenceRows, index%runicReferenceRows
		x := p.content.Min.X + column*cellWidth
		y := p.content.Min.Y + (row+runicReferenceExtraRows-1)*rowHeight
		p.cellBounds[index] = image.Rect(x, y, x+cellWidth, y+rowHeight)
		p.cells[index] = newOutput(cellWidth)
		p.cells[index].AppendLine().RunicWithColor(entry.glyph, ucolor.EGAYellow).
			White(" " + entry.label).Done()
	}
}

// Draw covers the right rail, leaving the source inscription unobscured.
func (p *RunicReferencePanel) Draw(
	screen *ebiten.Image, bounds image.Rectangle, font *text.UltimaFont, lineSpacing float64,
) {
	p.ensureLayout(bounds, font, lineSpacing)
	FillClassicPixelRoundedRect(screen, bounds, p.pixelUnit, ucolor.DialogBackground.RGBA())
	DrawClassicPixelRoundedFrame(screen, bounds, p.pixelUnit, DialogRuleColor)
	p.header.DrawPanelContent(screen, p.content.Min, p.content.Dx(), etext.AlignCenter)
	for index, output := range p.cells {
		cell := p.cellBounds[index]
		output.DrawPanelContent(screen, cell.Min, cell.Dx(), etext.AlignCenter)
	}
	footerPosition := image.Pt(p.content.Min.X, p.content.Max.Y-int(p.footer.GetTotalHeightForLines(1)))
	p.footer.DrawPanelContent(screen, footerPosition, p.content.Dx(), etext.AlignCenter)
}
