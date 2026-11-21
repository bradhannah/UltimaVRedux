package widgets

import (
	"image"
	stdcolor "image/color"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestNewYesNoInputBoxRetainsEveryQuestionRow(t *testing.T) {
	tests := []struct {
		name             string
		question         string
		minimumLineCount int
	}{
		{
			name:             "explicit lines including separator",
			question:         "A guard demands\na 20 gp tribute\nto Blackthorn!\n\nDost thou pay?",
			minimumLineCount: 5,
		},
		{
			name:             "wrapped question",
			question:         strings.Repeat("This confirmation text must wrap without losing words. ", 3),
			minimumLineCount: 3,
		},
		{
			name:             "exact-width line remains intact",
			question:         "Preamble\n" + strings.Repeat("a", 22) + " " + strings.Repeat("b", 24),
			minimumLineCount: 2,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialog := newTestYesNoInputBox(test.question)

			require.GreaterOrEqual(t, dialog.textQuestion.GetPanelContentLineCount(), test.minimumLineCount)
			require.Equal(t,
				strings.Join(strings.Fields(test.question), " "),
				strings.Join(strings.Fields(dialog.textQuestion.GetOutputStr(true)), " "),
			)
		})
	}
}

func TestNewYesNoInputBoxAllocatesSpaceForBlankQuestionRow(t *testing.T) {
	withSeparator := newTestYesNoInputBox("First\nSecond\nThird\n\nQuestion?")
	withoutSeparator := newTestYesNoInputBox("First\nSecond\nThird\nQuestion?")

	require.Less(t, withSeparator.layout.panelRect.Min.Y, withoutSeparator.layout.panelRect.Min.Y)
}

func TestNewYesNoInputBoxFitsSingleLineInput(t *testing.T) {
	tests := []struct {
		name         string
		question     string
		expectedRows int
	}{
		{name: "leave town", question: "Dost thou wish to leave?", expectedRows: 1},
		{name: "generic tribute", question: "A guard demands a 20 gp tribute to Blackthorn! Dost thou pay?", expectedRows: 2},
		{name: "Minoc tribute", question: "\"Thou wilt give half thy gold to charity!\" Dost thou pay?", expectedRows: 2},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dialog := newTestYesNoInputBox(test.question)
			dialog.textInput.SetText("Yes")

			require.Equal(t, test.expectedRows, dialog.textQuestion.GetPanelContentLineCount())
			require.Equal(t, "Yes", dialog.textInput.GetText())
			require.True(t, dialog.layout.questionTextRect.In(dialog.layout.panelRect))
			require.True(t, dialog.layout.inputTextRect.In(dialog.layout.panelRect))
			require.True(t, dialog.layout.footerTextRect.In(dialog.layout.panelRect))
			require.Equal(t, dialog.layout.padding,
				dialog.layout.questionTextRect.Min.X-dialog.layout.panelRect.Min.X)
			require.Equal(t, dialog.layout.padding,
				dialog.layout.questionTextRect.Min.Y-dialog.layout.panelRect.Min.Y)
			require.Equal(t, dialog.layout.padding,
				dialog.layout.panelRect.Max.Y-dialog.layout.footerTextRect.Max.Y)
			require.Equal(t, dialog.layout.padding,
				dialog.layout.inputTextRect.Min.X-dialog.layout.panelRect.Min.X)
			require.Equal(t, dialog.layout.padding,
				dialog.layout.footerTextRect.Min.X-dialog.layout.panelRect.Min.X)
			require.Equal(t,
				dialog.layout.questionTextRect.Max.Y+dialog.layout.lineGap,
				dialog.layout.inputTextRect.Min.Y)
			require.Equal(t,
				dialog.layout.inputTextRect.Max.Y+dialog.layout.lineGap,
				dialog.layout.footerTextRect.Min.Y)
		})
	}
}

func TestInputBoxPromptCardUsesClassicFrameAndPreservesTextOnResize(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := newTestYesNoInputBox("Dost thou wish to leave?")
	dialog.textInput.SetText("YES")
	screen := ebiten.NewImage(1280, 720)
	dialog.Draw(screen)

	requireClassicFrameSilhouette(t, screen, dialog.layout.panelRect)
	require.Equal(t,
		dialog.layout.questionTextRect.Max.Y+dialog.layout.lineGap,
		dialog.layout.inputTextRect.Min.Y)
	require.Equal(t,
		dialog.layout.inputTextRect.Max.Y+dialog.layout.lineGap,
		dialog.layout.footerTextRect.Min.Y)
	requireColorRGBA(t, ucolor.DialogBackground.RGBA(), screen.At(
		dialog.layout.panelRect.Min.X+dialog.layout.panelRect.Dx()/2,
		dialog.layout.questionTextRect.Max.Y+dialog.layout.lineGap/2,
	))
	require.Contains(t, dialog.footerOutput.GetOutputStr(false), "[Y/N] Choose")
	require.Contains(t, dialog.footerOutput.GetOutputStr(false), "[Enter] Confirm")
	require.Equal(t, dialog.textQuestion.Font.TextFace.Size, dialog.footerOutput.Font.TextFace.Size)
	requireImageRegionContainsColor(t, screen, dialog.layout.footerTextRect, DialogControlHintColor)
	require.Equal(t, ucolor.DialogInputTextColor, dialog.textInput.promptColor)
	originalTextInput := dialog.textInput

	setResponsiveDialogTestResolution(t, 1920, 1080)
	dialog.ensureLayout(1920, 1080)

	require.Equal(t, "YES", dialog.GetText())
	require.Same(t, originalTextInput, dialog.textInput)
	require.Equal(t, 1920, dialog.lastWidth)
	require.Equal(t, 1080, dialog.lastHeight)
}

func TestInputBoxPromptCardVisibleLinesHaveEqualSpacing(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)

	for _, question := range []string{"Enter", "First line\nEnter"} {
		dialog := newAlignmentTestInputBox(question)
		dialog.textInput.SetText("Enter")
		dialog.textInput.SetCursorVisible(false)
		screen := ebiten.NewImage(1280, 720)
		dialog.Draw(screen)

		questionInk := requireColorBounds(t, screen, dialog.layout.questionTextRect, DialogValueColor)
		inputInk := requireColorBounds(t, screen, dialog.layout.inputTextRect, ucolor.DialogInputTextColor)
		footerInk := requireColorBounds(t, screen, dialog.layout.footerTextRect, DialogControlHintColor)
		questionToInput := inputInk.Min.Y - questionInk.Max.Y
		inputToFooter := footerInk.Min.Y - inputInk.Max.Y

		require.InDelta(t, questionToInput, inputToFooter, 1,
			"visible line spacing should be equal for question %q", question)
	}
}

func TestInputBoxPromptAndEnteredTextShareVisibleVerticalCenter(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := newAlignmentTestInputBox("Enter")
	dialog.textInput.SetCursorVisible(false)

	promptScreen := ebiten.NewImage(1280, 720)
	dialog.Draw(promptScreen)
	promptInk := requireColorBounds(t, promptScreen, dialog.layout.inputTextRect, ucolor.DialogInputTextColor)

	dialog.textInput.ClearPrompt()
	dialog.textInput.SetText("Enter")
	inputScreen := ebiten.NewImage(1280, 720)
	dialog.Draw(inputScreen)
	inputInk := requireColorBounds(t, inputScreen, dialog.layout.inputTextRect, ucolor.DialogInputTextColor)

	require.InDelta(t, promptInk.Min.Y+promptInk.Max.Y, inputInk.Min.Y+inputInk.Max.Y, 2,
		"prompt and entered text should share a visible vertical center")
}

func TestInputBoxPromptCardStaysOnScreenAtSupportedResolutions(t *testing.T) {
	for _, size := range []struct {
		width  int
		height int
	}{
		{width: 320, height: 200},
		{width: 1280, height: 720},
		{width: 1920, height: 1080},
	} {
		setResponsiveDialogTestResolution(t, size.width, size.height)
		dialog := newTestYesNoInputBox("A guard demands\na 20 gp tribute\nto Blackthorn!\n\nDost thou pay?")
		dialog.ensureLayout(size.width, size.height)

		require.True(t, dialog.layout.panelRect.In(image.Rect(0, 0, size.width, size.height)), "%dx%d panel", size.width, size.height)
		require.True(t, dialog.layout.questionTextRect.In(dialog.layout.panelRect))
		require.True(t, dialog.layout.inputTextRect.In(dialog.layout.panelRect))
		require.True(t, dialog.layout.footerTextRect.In(dialog.layout.panelRect))
	}
}

func TestInputBoxPromptCardFooterMatchesInputMode(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	callback := func(string, bool) {}
	dialogStack := &DialogStack{}
	yesNoCommand := NewYesNoCommand(nil, nil, dialogStack)

	tests := []struct {
		name     string
		dialog   *InputBox
		expected string
	}{
		{
			name: "yes or no",
			dialog: NewInputBox("Proceed?", yesNoCommand, keyboard).
				WithAutoReplace(true).
				Build(),
			expected: "[Y/N] Choose   [Enter] Confirm",
		},
		{
			name: "grammar confirmation",
			dialog: NewInputBox("Proceed?", yesNoCommand, keyboard).
				Build(),
			expected: "[Enter] Confirm",
		},
		{
			name: "regex submission",
			dialog: NewInputBox("Thy wish?", nil, keyboard).
				WithRegexValidation("^[a-zA-Z]*$").
				WithResultCallback(callback).
				Build(),
			expected: "[Enter] Submit   [Esc] Cancel",
		},
		{
			name: "escape clears",
			dialog: NewInputBox("What dost thou yell?", nil, keyboard).
				WithRegexValidation("^[a-zA-Z]*$").
				WithResultCallback(callback).
				WithEscapeClearsText().
				Build(),
			expected: "[Enter] Submit   [Esc] Clear",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, test.dialog.footerPlainText())
			require.Equal(t, test.expected, test.dialog.footerOutput.GetOutputStr(false))
		})
	}
}

func TestNewYesNoInputBoxKeepsMinocSpeechBetweenQuotes(t *testing.T) {
	dialog := newTestYesNoInputBox("\"Thou wilt give half thy gold to charity!\" Dost thou pay?")
	rendered := strings.ReplaceAll(dialog.textQuestion.GetOutputStr(true), " \n", "\n")

	require.Equal(t, "\"Thou wilt give half thy gold to charity!\"\nDost thou pay?", rendered)
}

func newTestYesNoInputBox(question string) *InputBox {
	dialogStack := &DialogStack{}
	yesNoCommand := NewYesNoCommand(nil, nil, dialogStack)

	return NewYesNoInputBox(question, yesNoCommand, newWidgetTestKeyboard(0), dialogStack)
}

func newAlignmentTestInputBox(question string) *InputBox {
	return NewInputBox(question, nil, newWidgetTestKeyboard(0)).
		WithRegexValidation("^[a-zA-Z]*$").
		WithResultCallback(func(string, bool) {}).
		WithTextLines(2).
		WithCharsPerLine(40).
		Build()
}

func requireColorBounds(
	t *testing.T,
	screen *ebiten.Image,
	region image.Rectangle,
	target stdcolor.Color,
) image.Rectangle {
	t.Helper()
	targetRed, targetGreen, targetBlue, targetAlpha := target.RGBA()
	bounds := image.Rectangle{Min: region.Max, Max: region.Min}
	found := false
	for y := region.Min.Y; y < region.Max.Y; y++ {
		for x := region.Min.X; x < region.Max.X; x++ {
			red, green, blue, alpha := screen.At(x, y).RGBA()
			if red != targetRed || green != targetGreen || blue != targetBlue || alpha != targetAlpha {
				continue
			}
			found = true
			bounds.Min.X = min(bounds.Min.X, x)
			bounds.Min.Y = min(bounds.Min.Y, y)
			bounds.Max.X = max(bounds.Max.X, x+1)
			bounds.Max.Y = max(bounds.Max.Y, y+1)
		}
	}
	require.True(t, found, "expected color %v in %v", target, region)

	return bounds
}

func TestInputBoxPromptCardUsesDynamicSharedPaneWidth(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	shortDialog := newTestYesNoInputBox("Proceed?")
	longDialog := newTestYesNoInputBox("A guard demands a 20 gp tribute to Blackthorn! Dost thou pay?")
	// A short text entry on a wide configured pane, like the yell prompt.
	yellDialog := NewInputBox("What dost thou yell?", nil, newWidgetTestKeyboard(0)).
		WithRegexValidation("^[a-zA-Z]{0,10}$").
		WithResultCallback(func(string, bool) {}).
		WithAutoReplace(false).
		WithTextLines(DialogConfirmationLines).
		WithCharsPerLine(dialogDefaultCharsPerLine).
		WithHorizontalPosition(DialogHorizontalMapOnly).
		WithVerticalPosition(DialogDefaultVerticalPos).
		Build()

	require.Less(t, shortDialog.layout.panelRect.Dx(), longDialog.layout.panelRect.Dx())
	footerFont := GetScaledFont(inputBoxFontSize)
	footerTextWidth, _ := footerFont.MeasureString(shortDialog.footerPlainText())
	require.GreaterOrEqual(t, shortDialog.layout.panelRect.Dx(),
		int(math.Ceil(footerTextWidth))+shortDialog.layout.padding*2,
		"guidance should fit when another visible requirement determines card width")
	configuredYellWidth := int(
		(yellDialog.config.EndPercentX - yellDialog.config.StartPercentX) * float64(yellDialog.lastWidth),
	)
	require.Less(t, yellDialog.layout.panelRect.Dx(), configuredYellWidth,
		"input capacity should cap pane width rather than forcing it")
}
