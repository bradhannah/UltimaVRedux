package widgets

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/grammar"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*InputBox)(nil)

// NewYesNoCommand creates a yes/no grammar command with callbacks.
// This helper extracts the common pattern for yes/no dialogs.
//
// Parameters:
//   - onYes: Callback executed when user enters "yes" (can be nil)
//   - onNo: Callback executed when user enters "no" (can be nil)
//   - dialogStack: DialogStack for accessing and popping the input box
//
// Returns a configured TextCommand ready for use with InputBox.
//
// Example usage:
//
//	yesNoCmd := widgets.NewYesNoCommand(
//	    func() { executeYesAction() },
//	    func() { executeNoAction() },
//	    &g.dialogStack,
//	)
//	dialog := widgets.NewInputBox("Are you sure?", yesNoCmd, keyboard).
//	    WithAutoReplace(true).
//	    WithHorizontalPosition(widgets.DialogHorizontalMapOnly).
//	    Build()
//	g.dialogStack.PushModalDialog(dialog)
func NewYesNoCommand(onYes func(), onNo func(), dialogStack *DialogStack) *grammar.TextCommand {
	return grammar.NewTextCommand(
		[]grammar.Match{
			grammar.MatchStringList{
				Strings:              []string{"yes", "no"},
				Description:          "Yes or No",
				CaseSensitive:        false,
				SingleCharacterInput: true,
			},
		},
		func(_ string, _ *grammar.TextCommand) {
			inputBox := dialogStack.GetOrAssertTopInputBox()
			if inputBox == nil {
				return
			}
			response := strings.ToLower(inputBox.GetText())

			// Invoke onDismiss callback if available (handles auto-pop via AutoCloseAndPop)
			// This must happen BEFORE user callbacks to allow them to safely push new dialogs
			inputBox.InvokeOnDismiss()

			switch response {
			case "yes":
				if onYes != nil {
					onYes()
				}
			case "no":
				if onNo != nil {
					onNo()
				}
			}
		})
}

// InputBoxConfig defines the size and position of the input box dialog
type InputBoxConfig struct {
	StartPercentX float64 // 0.0-1.0 (left edge position)
	EndPercentX   float64 // 0.0-1.0 (right edge position)
	StartPercentY float64 // 0.0-1.0 (top edge position)
	EndPercentY   float64 // 0.0-1.0 (bottom edge position)
	TextLines     int     // Maximum question lines
	CharsPerLine  int     // Maximum characters per line
	AutoReplace   bool    // Enable single-character auto-replace mode

	// Regex mode support (mutually exclusive with grammar mode)
	RegexPattern     string                            // Regex validation pattern (empty = grammar mode)
	ResultCallback   func(text string, submitted bool) // Callback for regex mode (Enter/Escape)
	EscapeClearsText bool                              // Escape clears input instead of cancelling regex mode
}

// InputBoxBuilder provides a fluent API for constructing InputBox instances.
// This replaces the multiple New* constructors with a single, extensible builder pattern.
//
// Example usage:
//
//	// Simple question dialog (backward compatible)
//	dialog := NewInputBox("Are you sure?", textCommand, keyboard).Build()
//
//	// Custom positioned dialog
//	:= NewInputBox("Choose wisely:", textCommand, keyboard).
//	    WithTextLines(2).
//	    WithCharsPerLine(40).
//	    Build()
type InputBoxBuilder struct {
	// Required fields
	question    string
	textCommand *grammar.TextCommand
	keyboard    *input.Keyboard

	// Optional fields with defaults
	// Position configuration
	centerX *float64 // Center point X percentage (nil = use preset)
	centerY *float64 // Center point Y percentage (nil = use preset)

	// Auto-sizing configuration
	textLines    *int // Number of question lines (nil = auto-calculate)
	charsPerLine *int // Max characters per line (nil = use preset default)

	// Single-character mode
	autoReplace bool // Enable auto-replace for single-character inputs

	// Regex mode support (mutually exclusive with textCommand grammar mode)
	regexPattern     *string                           // Regex validation pattern (nil = grammar mode)
	resultCallback   func(text string, submitted bool) // Callback for regex mode results
	escapeClearsText bool

	autoCloseStack *DialogStack
}

const (
	inputBoxFontSize = FontSizeBody
	// Line spacing uses 1:1 ratio (same as font size) - default in GetScaledFontWithOutput
)

// InputBox is a modal prompt card that asks a question and accepts typed input,
// validated either by grammar commands or by a regex pattern.
type InputBox struct {
	textInput    *TextInput
	textQuestion *text.Output
	footerOutput *text.Output
	Question     string

	font *text.UltimaFont

	config InputBoxConfig
	layout inputBoxLayout

	// Regex mode support
	isRegexMode      bool
	regexPattern     string
	resultCallback   func(text string, submitted bool)
	escapeClearsText bool
	keyboard         *input.Keyboard
	onDismiss        func() // Optional: callback after resultCallback
	dismissInvoked   bool
	lastWidth        int
	lastHeight       int
}

// NewInputBox creates a new InputBoxBuilder with sensible defaults.
// This is the preferred way to create InputBox instances going forward.
//
// Parameters:
//   - question: The question text to display
//   - textCommand: Grammar command for validation (can be nil if using WithYesNoCallbacks())
//   - keyboard: Input handler
//
// Default configuration:
//   - Position: Map-centered horizontally (37% X), bottom area (70% Y)
//   - Auto-replace: Disabled
//   - Question lines: 2 (default)
//   - Chars per line: 32 (standard InputBox width)
//   - Size: Auto-calculated from content requirements
//
// Example usage:
//
//	// Simple dialog with text command
//	dialog := NewInputBox("Are you sure?", textCommand, keyboard).Build()
//
//	// Yes/no dialog with callbacks (textCommand = nil)
//	dialog := NewInputBox("Are you sure?", nil, keyboard).
//	    WithYesNoCallbacks(onYes, onNo, &dialogStack).
//	    Build()
//
//	// Custom positioned dialog
//	:= NewInputBox("Choose wisely:", textCommand, keyboard).
//	    WithTextLines(2).
//	    Build()
func NewInputBox(question string, textCommand *grammar.TextCommand, keyboard *input.Keyboard) *InputBoxBuilder {
	return &InputBoxBuilder{
		question:         question,
		textCommand:      textCommand,
		keyboard:         keyboard,
		centerX:          nil, // Map-centred
		centerY:          nil, // Standard bottom position
		textLines:        nil,
		charsPerLine:     nil,
		autoReplace:      false,
		regexPattern:     nil, // Grammar mode
		resultCallback:   nil,
		escapeClearsText: false,
		autoCloseStack:   nil,
	}
}

// WithHorizontalPosition sets the horizontal center position.
// When not specified, defaults to DialogHorizontalMapOnly (37% - map-centered).
//
// Parameters:
//   - xPos: Horizontal positioning (DialogHorizontalMapOnly or DialogHorizontalWholeScreen)
//
// Example: WithHorizontalPosition(DialogHorizontalWholeScreen) centers at 50% X
func (b *InputBoxBuilder) WithHorizontalPosition(xPos DialogHorizontalPosition) *InputBoxBuilder {
	x := xPos.ToPercentage()
	b.centerX = &x

	return b
}

// WithVerticalPosition sets the vertical center position.
// When not specified, defaults to 0.70 (70% from top - standard InputBox bottom position).
//
// Parameters:
//   - y: Vertical center position percentage (0.0-1.0)
//
// Example: WithVerticalPosition(0.82) centers at 82% Y
func (b *InputBoxBuilder) WithVerticalPosition(y float64) *InputBoxBuilder {
	b.centerY = &y

	return b
}

// WithTextLines explicitly sets the maximum number of question text lines.
// This overrides auto-calculation from question text.
func (b *InputBoxBuilder) WithTextLines(lines int) *InputBoxBuilder {
	b.textLines = &lines

	return b
}

// WithCharsPerLine explicitly sets the maximum characters per line.
// This overrides the default value of 32 characters.
func (b *InputBoxBuilder) WithCharsPerLine(chars int) *InputBoxBuilder {
	b.charsPerLine = &chars

	return b
}

// WithAutoReplace enables auto-replace mode for single-character inputs.
// When enabled, typing a new character automatically replaces existing text.
// Ideal for yes/no prompts and direction selections.
func (b *InputBoxBuilder) WithAutoReplace(enabled bool) *InputBoxBuilder {
	b.autoReplace = enabled

	return b
}

// WithRegexValidation enables regex validation mode (alternative to grammar commands).
// This is mutually exclusive with grammar mode - Build() will panic if both are set.
//
// Parameters:
//   - pattern: Regular expression pattern for input validation
//
// Example: WithRegexValidation("^[a-zA-Z]*$") - letters only
func (b *InputBoxBuilder) WithRegexValidation(pattern string) *InputBoxBuilder {
	b.regexPattern = &pattern

	return b
}

// WithResultCallback sets the callback for regex mode (invoked on Enter/Escape).
// Required when using WithRegexValidation().
//
// Parameters:
//   - callback: Function called with (text, submitted) when user presses Enter or Escape
//   - text: The input text entered by the user
//   - submitted: true if Enter pressed, false if Escape pressed
//
// Example:
//
//	WithResultCallback(func(text string, submitted bool) {
//	    if submitted && text != "" {
//	        processInput(text)
//	    }
//	    dialogStack.PopModalDialog()
//	})
func (b *InputBoxBuilder) WithResultCallback(callback func(text string, submitted bool)) *InputBoxBuilder {
	b.resultCallback = callback

	return b
}

// WithEscapeClearsText keeps regex input active and clears its text when Escape is pressed.
func (b *InputBoxBuilder) WithEscapeClearsText() *InputBoxBuilder {
	b.escapeClearsText = true

	return b
}

// AutoCloseAndPop is a convenience method that configures automatic dialog stack removal.
// Creates a dismiss callback that pops the dialog from the provided stack.
// Callback executes AFTER resultCallback completes.
//
// Parameters:
//   - dialogStack: Dialog stack reference for auto-pop (captured in closure)
//
// Example usage:
//
//	dialog := widgets.NewYesNoInputBox("Continue?", yesNoCmd, keyboard, dialogStack)
//	dialogStack.PushModalDialog(dialog)
func (b *InputBoxBuilder) AutoCloseAndPop(dialogStack *DialogStack) *InputBoxBuilder {
	if dialogStack == nil {
		panic("InputBox.AutoCloseAndPop: dialog stack is nil")
	}
	b.autoCloseStack = dialogStack

	return b
}

// Build constructs the final InputBox instance.
// This method applies all configured options and returns a ready-to-use input dialog.
func (b *InputBoxBuilder) Build() *InputBox {
	// Mode validation - must be grammar OR regex, not both
	hasGrammarMode := b.textCommand != nil
	hasRegexMode := b.regexPattern != nil

	if hasGrammarMode && hasRegexMode {
		panic("InputBox cannot use both grammar commands and regex validation - choose one mode")
	}

	if !hasGrammarMode && !hasRegexMode {
		panic("InputBox requires either textCommand (grammar mode) or regex pattern (regex mode)")
	}

	// Regex mode requires callback
	if hasRegexMode && b.resultCallback == nil {
		panic("InputBox regex mode requires WithResultCallback() to handle Enter/Escape")
	}

	// Auto-size from center point + content requirements
	// Apply defaults for unspecified coordinates
	centerX := DialogHorizontalMapOnly.ToPercentage() // Default X: map-centered (37%)
	if b.centerX != nil {
		centerX = *b.centerX
	}

	centerY := 0.70 // Default Y: 70% from top (standard InputBox bottom position)
	if b.centerY != nil {
		centerY = *b.centerY
	}

	textLines := 2     // Default
	charsPerLine := 32 // Default InputBox width

	if b.textLines != nil {
		textLines = *b.textLines
	}
	if b.charsPerLine != nil {
		charsPerLine = *b.charsPerLine
	}

	config := b.calculateAutoSizeDimensions(centerX, centerY, textLines, charsPerLine)
	config.AutoReplace = b.autoReplace

	// Populate regex mode fields if present
	if b.regexPattern != nil {
		config.RegexPattern = *b.regexPattern
	}
	if b.resultCallback != nil {
		config.ResultCallback = b.resultCallback
	}
	config.EscapeClearsText = b.escapeClearsText

	// Create InputBox from config
	inputBox := createInputBoxFromConfig(b.question, b.textCommand, b.keyboard, config)
	if b.autoCloseStack != nil {
		inputBox.onDismiss = func() {
			b.autoCloseStack.MustRemoveWidgetInstance(inputBox)
		}
	}

	return inputBox
}

// calculateAutoSizeDimensions computes dialog dimensions from content requirements using shared function.
func (b *InputBoxBuilder) calculateAutoSizeDimensions(
	centerX, centerY float64, textLines, charsPerLine int,
) InputBoxConfig {
	// Use shared auto-sizing function (InputBox has no prompt lines)
	placement := CalculateDialogAutoSizeDimensions(NewDialogAutoSizingParams(
		centerX,
		centerY,
		textLines,
		charsPerLine,
		0, // InputBox has no separate prompt area
		float64(inputBoxFontSize),
		float64(inputBoxFontSize), // 1:1 line spacing
	))

	return InputBoxConfig{
		StartPercentX:    placement.StartPercentX,
		EndPercentX:      placement.EndPercentX,
		StartPercentY:    placement.StartPercentY,
		EndPercentY:      placement.EndPercentY,
		TextLines:        textLines,
		CharsPerLine:     charsPerLine,
		AutoReplace:      false, // Set by Build() method
		RegexPattern:     "",
		ResultCallback:   nil,
		EscapeClearsText: false,
	}
}

// createInputBoxFromConfig constructs an InputBox from a configuration struct.
// This is the low-level constructor used by the builder's Build() method.
func createInputBoxFromConfig(
	question string, textCommand *grammar.TextCommand, keyboard *input.Keyboard, config InputBoxConfig,
) *InputBox {
	isRegexMode := config.RegexPattern != ""
	inputBox := &InputBox{
		font:         GetScaledFont(inputBoxFontSize),
		textInput:    newInputBoxTextInput(textCommand, keyboard, config, isRegexMode),
		textQuestion: nil, // Built by ensureLayout
		footerOutput: nil,
		Question:     question,
		config:       config,
		layout: inputBoxLayout{ // Computed by ensureLayout
			panelRect:        image.Rect(0, 0, 0, 0),
			questionTextRect: image.Rect(0, 0, 0, 0),
			inputTextRect:    image.Rect(0, 0, 0, 0),
			footerTextRect:   image.Rect(0, 0, 0, 0),
			pixelUnit:        0,
			padding:          0,
			lineGap:          0,
		},
		isRegexMode:      isRegexMode,
		regexPattern:     config.RegexPattern,
		resultCallback:   config.ResultCallback,
		escapeClearsText: config.EscapeClearsText,
		keyboard:         keyboard,
		onDismiss:        nil, // Set by AutoCloseAndPop
		dismissInvoked:   false,
		lastWidth:        0, // Forces the first ensureLayout
		lastHeight:       0,
	}

	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	inputBox.ensureLayout(screenWidth, screenHeight)

	return inputBox
}

// newInputBoxTextInput creates the light-green entry line, validated by the
// grammar command or, in regex mode, by the configured pattern.
func newInputBoxTextInput(
	textCommand *grammar.TextCommand, keyboard *input.Keyboard, config InputBoxConfig, isRegexMode bool,
) *TextInput {
	// Create text commands for grammar mode (nil for regex mode)
	var textCommands *grammar.TextCommands
	if !isRegexMode {
		textCommands = &grammar.TextCommands{
			*textCommand,
		}
	}

	textInput := NewTextInput(
		sprites.PercentBasedPlacement{
			StartPercentX: 0,
			EndPercentX:   0,
			StartPercentY: 0,
			EndPercentY:   0,
		},
		GetScaledLineHeight(inputBoxFontSize), config.CharsPerLine,
		textCommands,
		TextInputCallbacks{
			AmbiguousAutoComplete: func(_ string) {},
			OnEnter:               nil,
			OnCommandExecuted:     nil,
			ShowCommandList:       nil,
		},
		keyboard)
	// Set all input colors to light green for dialog input
	textInput.SetInputColors(TextInputColors{
		DefaultColor:          color.DialogInputTextColor,
		NoMatchesColor:        color.DialogInputTextColor,
		OneMatchColor:         color.DialogInputTextColor,
		MoreThanOneMatchColor: color.DialogInputTextColor,
	})

	// Regex validation provides the primary input filtering; maxCharsPerLine in
	// NewTextInput enforces the length.
	if isRegexMode {
		if err := textInput.SetInputValidationRegex(config.RegexPattern); err != nil {
			panic("Invalid regex pattern: " + err.Error())
		}
	}
	textInput.SetAutoReplaceMode(config.AutoReplace)

	return textInput
}

// Draw renders the prompt card at the current screen size.
func (i *InputBox) Draw(screen *ebiten.Image) {
	if screen == nil {
		return
	}
	i.ensureLayout(screen.Bounds().Dx(), screen.Bounds().Dy())
	i.drawPromptCard(screen)
}

// Update handles input. Grammar mode lets TextInput handle every key; regex mode
// resolves Enter, Space and Escape here and reports the result.
func (i *InputBox) Update() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	i.ensureLayout(screenWidth, screenHeight)
	if !i.isRegexMode {
		i.textInput.Update()

		return
	}
	i.updateRegexMode()
}

// updateRegexMode handles Enter/Escape at InputBox level (like TextInputDialog),
// then invokes the result callback followed by the dismiss callback.
func (i *InputBox) updateRegexMode() {
	result := ""
	submitted := false

	switch {
	case i.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		i.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		result = i.textInput.GetText()
		submitted = true
	case i.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
		if i.escapeClearsText {
			i.textInput.Clear()

			return
		}
	default:
		// Let TextInput handle character input
		i.textInput.Update()

		return
	}

	i.resultCallback(result, submitted)
	i.InvokeOnDismiss()
}

// AcceptsText reports that letter keys type into this box.
func (i *InputBox) AcceptsText() bool {
	return true
}

// GetText returns the text currently typed into the box.
func (i *InputBox) GetText() string {
	return i.textInput.GetText()
}

// InvokeOnDismiss calls the onDismiss callback if it's set.
// This is used by grammar commands to trigger auto-pop behavior via AutoCloseAndPop.
func (i *InputBox) InvokeOnDismiss() {
	if i.dismissInvoked {
		return
	}
	i.dismissInvoked = true
	if i.onDismiss != nil {
		i.onDismiss()
	}
}
