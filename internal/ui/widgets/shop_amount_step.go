package widgets

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/text"
)

const (
	shopAmountLabelColumns = 9 // The amount row's label, padded so its arrows line up.
	shopAmountFigureWidth  = 4 // The Have and Then columns; gold and quantities never pass 9999.
	shopTheftNote          = "Faulinei may take more."
)

// shopAmount is the one choice of an amount step and what it changes: the goods
// and the party's gold, as they stand and as they will be.
type shopAmount struct {
	label      string // "Bundles" or "Quantity"; empty for a step with nothing to choose.
	count      int
	most       int
	price      string
	goods      string
	goodsNow   int
	goodsThen  int
	goldNow    int
	goldThen   int
	theftAfter bool // A purchase: Faulinei's theft comes after the payment shown.
}

// appendShopAmount writes the amount row, the price line and the table of what
// changes, and returns the output line of the amount row (-1 when there is none)
// so the caller can put the selection bar behind it.
func appendShopAmount(output *text.Output, amount shopAmount) int {
	row := -1
	if amount.label != "" {
		row = output.GetPanelContentLineCount()
		output.AppendLine().
			Text(fmt.Sprintf(" %-*s<  %-3d>", shopAmountLabelColumns, amount.label, amount.count), DialogSelectedText).
			Text(fmt.Sprintf("    of %d", amount.most), DialogSelectedLabelColor).Done()
		output.AppendLine().White("").Done() // The bar's own row: the price line starts clear of it.
	}
	output.AppendLine().LightGray(" " + amount.price).WordWrap().Done()
	output.AppendLine().White("").Done()
	name := max(len(amount.goods), len("Gold"))
	output.AppendLine().Text(fmt.Sprintf(" %-*s %*s %*s", name, "", shopAmountFigureWidth, "Have",
		shopAmountFigureWidth, "Then"), DialogLabelColor).Done()
	for _, line := range [...]struct {
		name      string
		now, then int
	}{{amount.goods, amount.goodsNow, amount.goodsThen}, {"Gold", amount.goldNow, amount.goldThen}} {
		output.AppendLine().Text(fmt.Sprintf(" %-*s %*d %*d", name, line.name, shopAmountFigureWidth, line.now,
			shopAmountFigureWidth, line.then), DialogValueColor).Done()
	}
	if amount.theftAfter {
		output.AppendLine().LightGray(" " + shopTheftNote).WordWrap().Done()
	}

	return row
}

// fillBodyLine puts the selection bar behind one line of the body text.
func (f *merchantModalFrame) fillBodyLine(screen *ebiten.Image, body *text.Output, line int) {
	if line < 0 {
		return
	}
	top := f.bodyText.Min.Y + int(math.Round(float64(line)*body.GetLineSpacing()))
	fillSelectedRow(screen, image.Rect(f.bodyText.Min.X, top, f.bodyText.Max.X, top+int(math.Ceil(body.GetFontHeight()))))
}

// appendAmountGuidance writes an amount step's hint rows. When there is only one
// amount to choose, the keys that choose a number are dimmed.
func appendAmountGuidance(output *text.Output, guidance string, width int, onlyOne bool) {
	dimmed := func(hint ControlHint) bool { return onlyOne && (hint.Key == "Left/Right" || hint.Key == "M") }
	for _, hints := range parseMerchantFooterGuidance(guidance) {
		PackControlHintsDimming(output, hints, width, etext.AlignCenter, dimmed)
	}
}
