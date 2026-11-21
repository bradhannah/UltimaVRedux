package widgets

import (
	"image"
	"math"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	inputPromptMinimumPadding      = 6
	inputPromptPaddingUnits        = 3
	inputPromptLineGapUnits        = 2
	inputPromptMinimumVisibleChars = 16
	inputPromptCenterDivisor       = 2
	inputPromptMarker              = "> "
	yesNoControlHint               = "[Y/N] Choose   [Enter] Confirm"
	textSubmitControlHint          = "[Enter] Submit   [Esc] Cancel"
)

type inputBoxLayout struct {
	panelRect        image.Rectangle
	questionTextRect image.Rectangle
	inputTextRect    image.Rectangle
	footerTextRect   image.Rectangle
	pixelUnit        int
	padding          int
	lineGap          int
}

//nolint:funlen // Keeping the responsive prompt-pane geometry together makes its visual hierarchy auditable.
func (i *InputBox) ensureLayout(width, height int) {
	if width <= 0 || height <= 0 {
		return
	}
	if i.lastWidth == width && i.lastHeight == height {
		return
	}
	i.lastWidth = width
	i.lastHeight = height

	pixelUnit := getClassicUIPixelUnit(width, height)
	padding := max(inputPromptMinimumPadding, pixelUnit*inputPromptPaddingUnits)
	lineGap := max(1, pixelUnit*inputPromptLineGapUnits)
	i.font = GetScaledFont(inputBoxFontSize)

	footerFont := GetScaledFont(inputBoxFontSize)
	paneWidth := i.calculatePromptWidth(width, padding, footerFont)
	questionLines := i.rebuildPromptOutputs(footerFont)

	questionHeight := int(math.Ceil(i.textQuestion.GetTotalHeightForLines(questionLines)))
	inputHeight := int(math.Ceil(i.textInput.output.GetFontHeight()))
	footerHeight := int(math.Ceil(i.footerOutput.GetFontHeight()))
	panelHeight := padding*inputPromptCenterDivisor + questionHeight + inputHeight + footerHeight +
		lineGap*inputPromptCenterDivisor

	centerX := int(math.Round(
		(i.config.StartPercentX + i.config.EndPercentX) * float64(width) / inputPromptCenterDivisor,
	))
	centerY := int(math.Round(
		(i.config.StartPercentY + i.config.EndPercentY) * float64(height) / inputPromptCenterDivisor,
	))
	panelMinY := min(max(padding, centerY-panelHeight/2), height-padding-panelHeight)
	paneMinX := centeredPromptRectMinX(centerX, paneWidth, width, padding)
	panel := image.Rect(paneMinX, panelMinY, paneMinX+paneWidth, panelMinY+panelHeight)
	contentRect := insetRectangle(panel, padding)
	questionTextRect := image.Rect(
		contentRect.Min.X,
		contentRect.Min.Y,
		contentRect.Max.X,
		contentRect.Min.Y+questionHeight,
	)
	inputTextRect := image.Rect(
		contentRect.Min.X,
		questionTextRect.Max.Y+lineGap,
		contentRect.Max.X,
		questionTextRect.Max.Y+lineGap+inputHeight,
	)
	footerTextRect := image.Rect(
		contentRect.Min.X,
		inputTextRect.Max.Y+lineGap,
		contentRect.Max.X,
		inputTextRect.Max.Y+lineGap+footerHeight,
	)

	i.layout = inputBoxLayout{
		panelRect:        panel,
		questionTextRect: questionTextRect,
		inputTextRect:    inputTextRect,
		footerTextRect:   footerTextRect,
		pixelUnit:        pixelUnit,
		padding:          padding,
		lineGap:          lineGap,
	}
	i.textInput.SetPlacement(pixelRectangleToPercentPlacement(i.layout.inputTextRect, width, height))
}

func (i *InputBox) calculatePromptWidth(
	width int,
	panePadding int,
	footerFont *text.UltimaFont,
) int {
	availableWidth := max(1, width-panePadding*inputPromptCenterDivisor)
	configuredMaximum := int(math.Round((i.config.EndPercentX - i.config.StartPercentX) * float64(width)))
	configuredMaximum = max(1, min(configuredMaximum, availableWidth))

	lineSpacing := GetScaledLineHeight(inputBoxFontSize)
	questionWidth := text.MeasureTextUnwrapped(i.Question, i.font, lineSpacing).MaxLineWidth
	markerWidth, _ := i.font.MeasureString(inputPromptMarker)
	visibleInputChars := min(max(1, i.config.CharsPerLine), inputPromptMinimumVisibleChars)
	minimumInputWidth, _ := i.font.MeasureString(strings.Repeat("M", visibleInputChars))
	footerWidth, _ := footerFont.MeasureString(i.footerPlainText())
	questionPaneWidth := int(math.Ceil(questionWidth)) + panePadding*inputPromptCenterDivisor
	inputPaneWidth := int(math.Ceil(markerWidth+minimumInputWidth)) + panePadding*inputPromptCenterDivisor
	guidanceWidth := int(math.Ceil(footerWidth)) + panePadding*inputPromptCenterDivisor
	preferredWidth := max(min(configuredMaximum, questionPaneWidth), min(configuredMaximum, inputPaneWidth), guidanceWidth)

	return min(availableWidth, preferredWidth)
}

func (i *InputBox) rebuildPromptOutputs(footerFont *text.UltimaFont) int {
	lineSpacing := GetScaledLineHeight(inputBoxFontSize)
	questionChars := max(1, i.config.CharsPerLine)
	measurement := text.MeasureText(i.Question, i.font, lineSpacing, questionChars)
	questionLines := max(1, measurement.Lines)
	questionCapacity := max(i.config.TextLines, questionLines)
	i.textQuestion = text.NewOutput(i.font, lineSpacing, questionCapacity, questionChars)
	i.textQuestion.SetColor(DialogValueColor)
	i.textQuestion.AddText(i.Question)
	questionLines = max(1, i.textQuestion.GetPanelContentLineCount())

	footerLineSpacing := GetScaledLineHeight(inputBoxFontSize)
	footerChars := len(i.footerPlainText()) + 1
	i.footerOutput = text.NewOutput(footerFont, footerLineSpacing, 1, footerChars)
	i.populateFooterOutput()

	i.textInput.SetFontPoint(GetScaledLineHeight(inputBoxFontSize))
	i.textInput.SetPromptPrefix(inputPromptMarker)
	i.textInput.SetPromptColor(ucolor.DialogInputTextColor)

	return questionLines
}

func centeredPromptRectMinX(centerX, rectWidth, screenWidth, edgePadding int) int {
	return min(max(edgePadding, centerX-rectWidth/inputPromptCenterDivisor), screenWidth-edgePadding-rectWidth)
}

func (i *InputBox) footerPlainText() string {
	if !i.isRegexMode {
		if i.config.AutoReplace {
			return yesNoControlHint
		}

		return "[Enter] Confirm"
	}
	if i.escapeClearsText {
		return "[Enter] Submit   [Esc] Clear"
	}

	return textSubmitControlHint
}

func (i *InputBox) populateFooterOutput() {
	i.footerOutput.Reset()
	AppendControlHints(i.footerOutput.AppendLine().WithAlignment(etext.AlignCenter), i.footerPlainText()).Done()
}

func (i *InputBox) drawPromptCard(screen *ebiten.Image) {
	FillClassicPixelRoundedRect(screen, i.layout.panelRect, i.layout.pixelUnit, ucolor.DialogBackground.RGBA())
	i.textQuestion.DrawPanelContent(
		screen,
		i.layout.questionTextRect.Min,
		i.layout.questionTextRect.Dx(),
		etext.AlignStart,
	)
	i.textInput.Draw(screen)
	i.footerOutput.DrawPanelContent(
		screen,
		image.Pt(
			i.layout.footerTextRect.Min.X,
			verticallyCenteredOutputY(i.layout.footerTextRect, i.footerOutput),
		),
		i.layout.footerTextRect.Dx(),
		etext.AlignCenter,
	)
	DrawClassicPixelRoundedFrame(screen, i.layout.panelRect, i.layout.pixelUnit, DialogRuleColor)
}

func pixelRectangleToPercentPlacement(rect image.Rectangle, width, height int) sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: float64(rect.Min.X) / float64(width),
		EndPercentX:   float64(rect.Max.X) / float64(width),
		StartPercentY: float64(rect.Min.Y) / float64(height),
		EndPercentY:   float64(rect.Max.Y) / float64(height),
	}
}
