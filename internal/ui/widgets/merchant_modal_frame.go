package widgets

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	gametext "github.com/bradhannah/UltimaVRedux/internal/text"
)

const (
	merchantModalInsetDivisor = 48
	merchantFitHalves         = 2.0
	merchantFitInsetPair      = 2
	merchantFitInsets         = 9 // Three panes' two insets each, the content's two, and one of slack.
	merchantFitSideInsets     = 4 // The content's inset and the text's, on each side.
	merchantHeaderNoteGap     = 0.2
)

type merchantModalFrameConfig struct {
	placement             sprites.PercentBasedPlacement
	headerRatio           float64
	footerRatio           float64
	minimumInset          int
	lineSpacingMultiplier float64
	footerMinimumLines    int
}

type merchantModalFrame struct {
	config merchantModalFrameConfig
	border *Border

	headerBounds image.Rectangle
	bodyBounds   image.Rectangle
	footerBounds image.Rectangle
	headerText   image.Rectangle
	bodyText     image.Rectangle
	footerText   image.Rectangle
	inset        int
	fitLines     int // When positive, the window is only tall enough for this many body lines.
	fitChars     int // When positive, the window is only wide enough for this many body letters.
	fitInset     int // The inset the full-size window would have, kept when it is fitted.

	headerNoteLines int // Body-size lines under the title, such as the price of one.

	lastScreenWidth  int
	lastScreenHeight int
}

func newMerchantModalFrame(config merchantModalFrameConfig) *merchantModalFrame {
	if config.headerRatio <= 0 || config.footerRatio <= 0 || config.minimumInset < 0 ||
		config.lineSpacingMultiplier <= 0 || config.footerMinimumLines <= 0 {
		panic("merchantModalFrame: invalid layout configuration")
	}

	return &merchantModalFrame{config: config} //nolint:exhaustruct_v5 // Layout initializes on the first ensureLayout.
}

func (f *merchantModalFrame) ensureLayout() bool {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return false
	}
	if f.border != nil && width == f.lastScreenWidth && height == f.lastScreenHeight {
		return false
	}

	f.lastScreenWidth, f.lastScreenHeight = width, height
	f.fitLines, f.fitChars = 0, 0
	f.layout()

	return true
}

// fit sizes the window, about its centre, to hold exactly the given number of body
// text lines and body letters across, with the header and footer at their least
// height, so that the window is as large as what it shows and never changes size
// while its content does. The caller then rebuilds its outputs, whose capacity
// follows the new size.
func (f *merchantModalFrame) fit(lines, chars int) {
	f.fitLines, f.fitChars = lines, chars
	f.layout()
}

func (f *merchantModalFrame) layout() {
	placement := f.config.placement
	f.border = NewOpaqueBorder(placement, BorderWidthStandard, MerchantDialogBackgroundColor)
	if f.fitLines > 0 {
		full, interior := f.border.GetBounds(), f.border.GetInteriorBounds()
		inset := max(f.config.minimumInset, min(interior.Dx(), interior.Dy())/merchantModalInsetDivisor)
		f.fitInset = inset
		lineSpacing := f.bodyLineSpacing()
		body := scaledFontHeight(FontSizeBody) + float64(f.fitLines-1)*lineSpacing
		footer := scaledFontHeight(FontSizeBody) + float64(f.config.footerMinimumLines-1)*lineSpacing
		// Header, body and footer each carry two insets, the content one more pair,
		// and one inset of slack absorbs rounding.
		need := int(math.Ceil(f.headerTextHeight()+body+footer)) + inset*merchantFitInsets
		outer := min(f.lastScreenHeight, need+full.Dy()-interior.Dy())
		half := float64(outer) / float64(f.lastScreenHeight) / merchantFitHalves
		centre := (placement.StartPercentY + placement.EndPercentY) / merchantFitHalves
		placement.StartPercentY, placement.EndPercentY = centre-half, centre+half
		if f.fitChars > 0 {
			letter, _ := GetScaledFont(FontSizeBody).MeasureString("M")
			// The text sits two insets in from each side of the interior; one pixel
			// more keeps the last letter from being rounded away.
			need = int(math.Ceil(letter*float64(f.fitChars))) + 1 + inset*merchantFitSideInsets
			// The border's own margin is a share of the window, so it grows with it.
			outer = min(f.lastScreenWidth, int(math.Ceil(float64(need*full.Dx())/float64(interior.Dx())))+1)
			half = float64(outer) / float64(f.lastScreenWidth) / merchantFitHalves
			centre = (placement.StartPercentX + placement.EndPercentX) / merchantFitHalves
			placement.StartPercentX, placement.EndPercentX = centre-half, centre+half
		}
		f.border = NewOpaqueBorder(placement, BorderWidthStandard, MerchantDialogBackgroundColor)
	}
	f.calculateLayout()
}

func (f *merchantModalFrame) bodyLineSpacing() float64 {
	return gametext.GetScaledNumberToResolution(
		display.GetManager(), float64(FontSizeBody)*f.config.lineSpacingMultiplier)
}

// headerTextHeight is the title's height and that of any body-size lines under it,
// which stand a little clear of the larger title.
func (f *merchantModalFrame) headerTextHeight() float64 {
	if f.headerNoteLines == 0 {
		return scaledFontHeight(FontSizeTitle)
	}

	return scaledFontHeight(FontSizeTitle) + float64(f.headerNoteLines)*f.bodyLineSpacing() +
		scaledFontHeight(FontSizeBody)*merchantHeaderNoteGap
}

func (f *merchantModalFrame) calculateLayout() {
	interior := f.border.GetInteriorBounds()
	f.inset = max(f.config.minimumInset, min(interior.Dx(), interior.Dy())/merchantModalInsetDivisor)
	if f.fitLines > 0 {
		f.inset = f.fitInset
	}
	content := insetRectangle(interior, f.inset)

	fontHeight := scaledFontHeight(FontSizeBody)
	lineSpacing := f.bodyLineSpacing()
	footerTextHeight := fontHeight + float64(f.config.footerMinimumLines-1)*lineSpacing
	headerTextHeight := int(math.Ceil(f.headerTextHeight()))
	headerHeight := max(
		headerTextHeight+f.inset*2,
		int(float64(content.Dy())*f.config.headerRatio),
	)
	footerHeight := max(
		int(math.Ceil(footerTextHeight))+f.inset*2,
		int(float64(content.Dy())*f.config.footerRatio),
	)
	if f.fitLines > 0 {
		headerHeight = headerTextHeight + f.inset*merchantFitInsetPair
		footerHeight = int(math.Ceil(footerTextHeight)) + f.inset*merchantFitInsetPair
	}

	f.headerBounds = image.Rect(
		content.Min.X,
		content.Min.Y,
		content.Max.X,
		min(content.Max.Y, content.Min.Y+headerHeight),
	)
	f.footerBounds = image.Rect(
		content.Min.X,
		max(f.headerBounds.Max.Y, content.Max.Y-footerHeight),
		content.Max.X,
		content.Max.Y,
	)
	f.bodyBounds = image.Rect(content.Min.X, f.headerBounds.Max.Y, content.Max.X, f.footerBounds.Min.Y)
	f.headerText = insetRectangle(f.headerBounds, f.inset)
	f.bodyText = insetRectangle(f.bodyBounds, f.inset)
	f.footerText = insetRectangle(f.footerBounds, f.inset)
}

func (f *merchantModalFrame) draw(screen *ebiten.Image) {
	if screen == nil {
		return
	}
	for _, bounds := range [...]image.Rectangle{f.headerBounds, f.bodyBounds, f.footerBounds} {
		FillClassicPixelRoundedRect(screen, bounds, f.pixelUnit(), MerchantDialogBackgroundColor)
	}
	DrawClassicPixelRoundedFrame(screen, f.headerBounds, f.pixelUnit(), DialogRuleColor)
	DrawClassicPixelRoundedFrame(screen, f.bodyBounds, f.pixelUnit(), DialogRuleColor)
	DrawClassicPixelRoundedFrame(screen, f.footerBounds, f.pixelUnit(), DialogRuleColor)
}

// merchantModalTextBudget sets the line spacing and line/character limits of
// a merchant modal's header, body and footer outputs. A fixed body keeps
// maximumBodyLines rather than the number of lines its area fits.
type merchantModalTextBudget struct {
	lineSpacing      float64
	maximumBodyLines int
	footerLines      int
	minimumChars     int
	fixedBody        bool
}

// newOutputs creates header, body and footer outputs sized to the frame's text
// areas; unless fixed, the body holds as many lines as its area fits.
func (f *merchantModalFrame) newOutputs(budget merchantModalTextBudget) (*gametext.Output, *gametext.Output,
	*gametext.Output,
) {
	_, headerOutput := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeTitle, LineSpacing: 0, LineSpacingMultiplier: budget.lineSpacing,
		MaxLines: 1, MaxChars: budget.minimumChars,
	})
	bodyFont, bodyOutput := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: budget.lineSpacing,
		MaxLines: budget.maximumBodyLines, MaxChars: budget.minimumChars,
	})
	_, footerOutput := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: budget.lineSpacing,
		MaxLines: budget.footerLines, MaxChars: budget.minimumChars,
	})
	headerOutput.SetMaxCharsPerLine(max(budget.minimumChars,
		headerOutput.Font.CharsForWidth(max(1, f.headerText.Dx()))))
	bodyLines := max(1, bodyOutput.GetLineCapacity(max(1, f.bodyText.Dy())))
	if budget.fixedBody {
		bodyLines = budget.maximumBodyLines
	}
	bodyOutput = gametext.NewOutput(bodyFont, bodyOutput.GetLineSpacing(), bodyLines,
		max(budget.minimumChars, bodyFont.CharsForWidth(max(1, f.bodyText.Dx()))))
	footerOutput.SetMaxCharsPerLine(max(budget.minimumChars,
		footerOutput.Font.CharsForWidth(max(1, f.footerText.Dx()))))

	return headerOutput, bodyOutput, footerOutput
}

// drawOutputs draws the centred header, top-left body and centred footer text.
func (f *merchantModalFrame) drawOutputs(screen *ebiten.Image, header, body, footer *gametext.Output) {
	headerTop := verticallyCenteredOutputY(f.headerText, header)
	if f.headerNoteLines > 0 {
		headerTop = f.headerText.Min.Y
	}
	header.DrawPanelContent(screen, image.Pt(f.headerText.Min.X, headerTop), f.headerText.Dx(), etext.AlignCenter)
	body.DrawPanelContent(screen, f.bodyText.Min, f.bodyText.Dx(), etext.AlignStart)
	footer.DrawPanelContent(
		screen,
		image.Pt(f.footerText.Min.X, verticallyCenteredOutputY(f.footerText, footer)),
		f.footerText.Dx(),
		etext.AlignCenter,
	)
}

// newHeaderNoteOutput creates the body-size output for the lines under the title.
func (f *merchantModalFrame) newHeaderNoteOutput(lineSpacing float64) *gametext.Output {
	font, output := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: lineSpacing,
		MaxLines: max(1, f.headerNoteLines), MaxChars: 1,
	})
	output.SetMaxCharsPerLine(font.CharsForWidth(max(1, f.headerText.Dx())))

	return output
}

// drawHeaderNote draws the lines under the title, against the foot of the header.
func (f *merchantModalFrame) drawHeaderNote(screen *ebiten.Image, note *gametext.Output) {
	height := int(math.Ceil(note.GetTotalHeightForLines(f.headerNoteLines)))
	note.DrawPanelContent(screen, image.Pt(f.headerText.Min.X, f.headerText.Max.Y-height), f.headerText.Dx(),
		etext.AlignCenter)
}

// drawMerchantPane fills a shop pane with the merchant background and draws its light frame.
func drawMerchantPane(screen *ebiten.Image, bounds image.Rectangle, pixelUnit int) {
	vector.FillRect(screen, float32(bounds.Min.X), float32(bounds.Min.Y), float32(bounds.Dx()), float32(bounds.Dy()),
		MerchantDialogBackgroundColor, false)
	DrawClassicPixelRoundedFrame(screen, bounds, pixelUnit, DialogRuleLightColor)
}

func (f *merchantModalFrame) bounds() image.Rectangle {
	return f.border.GetBounds()
}

func (f *merchantModalFrame) pixelUnit() int {
	return getClassicUIPixelUnit(f.lastScreenWidth, f.lastScreenHeight)
}
