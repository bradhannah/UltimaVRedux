package widgets

import (
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/grammar"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// Shared dialog sizing constants are defined in dialog_constants.go.

// NewYesNoInputBox creates InputBox for yes/no confirmation prompts.
// This is a convenience constructor for yes/no dialogs with auto-replace.
//
// Features:
//   - Auto-replace enabled (single key press submits)
//   - Auto-close and pop from dialog stack after result callback
//   - Natural wrapping with capacity expanded to fit the complete question
//   - Map-centered positioning
//   - Grammar command for yes/no validation
//
// Parameters:
//   - question: The confirmation question to display
//   - yesNoCommand: Grammar command handling yes/no responses
//   - keyboard: Input handler
//   - dialogStack: Dialog stack to pop from after result callback
//
// Example usage:
//
//	yesNoCommand := widgets.NewYesNoCommand(
//	    func() { performAction() },
//	    func() { /* cancel */ },
//	    &dialogStack,
//	)
//	dialog := widgets.NewYesNoInputBox("Dost thou wish to leave?", yesNoCommand, keyboard, &dialogStack)
//	dialogStack.PushModalDialog(dialog)
func NewYesNoInputBox(
	question string, yesNoCommand *grammar.TextCommand, keyboard *input.Keyboard, dialogStack *DialogStack,
) *InputBox {
	return NewYesNoInputBoxAt(question, yesNoCommand, keyboard, dialogStack, DialogDefaultVerticalPos)
}

// NewYesNoInputBoxAt is NewYesNoInputBox centred at another height, as a share
// of the screen from the top, for a question asked beside a dialog that already
// fills the standard position.
func NewYesNoInputBoxAt(
	question string, yesNoCommand *grammar.TextCommand, keyboard *input.Keyboard, dialogStack *DialogStack,
	centerY float64,
) *InputBox {
	questionLines := text.MeasureText(
		question,
		GetScaledFont(inputBoxFontSize),
		GetScaledLineHeight(inputBoxFontSize),
		dialogConfirmationCharsPerLine,
	).Lines

	return NewInputBox(question, yesNoCommand, keyboard).
		WithAutoReplace(true).
		AutoCloseAndPop(dialogStack).
		WithTextLines(max(DialogConfirmationLines, questionLines)).
		WithCharsPerLine(dialogConfirmationCharsPerLine).
		WithHorizontalPosition(DialogHorizontalMapOnly).
		WithVerticalPosition(centerY).
		Build()
}
