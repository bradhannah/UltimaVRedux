package widgets

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/text"
)

// Words that shop tables and tab rows repeat.
const (
	shopHeadingPrice = "Price"
	shopHeadingReady = "Ready"
	shopTitleLeave   = "Leave"
)

// shopTabGap separates the page titles of a shop's tab row.
const shopTabGap = "    "

// shopTab is one page title in a shop's tab row.
type shopTab struct {
	title string
	open  bool
}

// appendShopTabs writes the titles onto line, the open page in white on the
// shared plate (see drawShopTabPlate) and the other pages dimmed.
func appendShopTabs(line *text.LineBuilder, tabs []shopTab) {
	for index, tab := range tabs {
		if index > 0 {
			line.Text(shopTabGap, DialogTabColor)
		}
		colour := DialogTabColor
		if tab.open {
			colour = DialogActiveTabText
		}
		line.Text(tab.title, colour)
	}
	line.Done()
}

// drawShopTabPlate fills the light blue plate behind the open title. The tab row
// is the centred line of output at index row, after lead letters of other text
// on that line, and output is drawn inside bounds as the shop draws it.
func drawShopTabPlate(
	screen *ebiten.Image, output *text.Output, bounds image.Rectangle,
	unit, row int, lead string, tabs []shopTab,
) {
	line := lead
	offset, openWidth := 0.0, 0.0
	for index, tab := range tabs {
		if index > 0 {
			line += shopTabGap
		}
		if tab.open {
			offset, _ = output.Font.MeasureString(line)
			openWidth, _ = output.Font.MeasureString(tab.title)
		}
		line += tab.title
	}
	total, _ := output.Font.MeasureString(line)
	left := bounds.Min.X + (bounds.Dx()-int(math.Ceil(total)))/2 + int(offset)
	top := verticallyCenteredOutputY(bounds, output) + int(math.Round(float64(row)*output.GetLineSpacing()))
	cell := image.Rect(left, top, left+int(math.Ceil(openWidth)), top+int(math.Ceil(output.GetFontHeight())))
	DrawActiveTabPlate(screen, cell, openWidth, top, cell.Dy(), unit)
}
