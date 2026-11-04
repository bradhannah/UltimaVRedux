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

const merchantModalInsetDivisor = 48

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
	f.border = NewOpaqueBorder(f.config.placement, BorderWidthStandard, MerchantDialogBackgroundColor)
	f.calculateLayout()

	return true
}

func (f *merchantModalFrame) calculateLayout() {
	interior := f.border.GetInteriorBounds()
	f.inset = max(f.config.minimumInset, min(interior.Dx(), interior.Dy())/merchantModalInsetDivisor)
	content := insetRectangle(interior, f.inset)

	fontHeight := scaledFontHeight(FontSizeBody)
	lineSpacing := gametext.GetScaledNumberToResolution(
		display.GetManager(),
		float64(FontSizeBody)*f.config.lineSpacingMultiplier,
	)
	footerTextHeight := fontHeight + float64(f.config.footerMinimumLines-1)*lineSpacing
	headerHeight := max(
		int(math.Ceil(scaledFontHeight(FontSizeTitle)))+f.inset*2,
		int(float64(content.Dy())*f.config.headerRatio),
	)
	footerHeight := max(
		int(math.Ceil(footerTextHeight))+f.inset*2,
		int(float64(content.Dy())*f.config.footerRatio),
	)

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
	header.DrawPanelContent(
		screen,
		image.Pt(f.headerText.Min.X, verticallyCenteredOutputY(f.headerText, header)),
		f.headerText.Dx(),
		etext.AlignCenter,
	)
	body.DrawPanelContent(screen, f.bodyText.Min, f.bodyText.Dx(), etext.AlignStart)
	footer.DrawPanelContent(
		screen,
		image.Pt(f.footerText.Min.X, verticallyCenteredOutputY(f.footerText, footer)),
		f.footerText.Dx(),
		etext.AlignCenter,
	)
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
