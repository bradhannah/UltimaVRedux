package widgets

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
)

// stackTestWidget's field gives it a size, so each instance has a distinct address
// for the stack's duplicate-instance check.
type stackTestWidget struct {
	id int
}

func (*stackTestWidget) Draw(_ *ebiten.Image) {}
func (*stackTestWidget) Update()              {}

func TestDialogStackPeekTopModalSkipsNonBlockingDialogs(t *testing.T) {
	var stack DialogStack
	modal := &stackTestWidget{}
	feedback := &stackTestWidget{}
	stack.PushModalDialog(modal)
	stack.PushNonBlockingDialog(feedback)

	assert.Same(t, modal, stack.PeekTopModalDialog())
}

func TestDialogStackRejectsDuplicateInstance(t *testing.T) {
	var stack DialogStack
	dialog := &stackTestWidget{}
	stack.PushModalDialog(dialog)

	assert.Panics(t, func() { stack.PushNonBlockingDialog(dialog) })
}

func TestInputBoxAutoCloseRemovesExactInstance(t *testing.T) {
	initializeDialogTestDisplay(t)
	var stack DialogStack
	dialog := NewInputBox("Name?", nil, newWidgetTestKeyboard(100)).
		WithRegexValidation(".*").
		WithResultCallback(func(string, bool) {}).
		AutoCloseAndPop(&stack).
		Build()
	above := &stackTestWidget{}
	stack.PushModalDialog(dialog)
	stack.PushModalDialog(above)

	dialog.InvokeOnDismiss()

	assert.Equal(t, 1, stack.Count())
	assert.Same(t, above, stack.PeekTopModalDialog())
}

func TestDialogStackFindsInputBoxBelowNonBlockingDialog(t *testing.T) {
	initializeDialogTestDisplay(t)
	var stack DialogStack
	inputBox := NewInputBox("Name?", nil, newWidgetTestKeyboard(100)).
		WithRegexValidation(".*").
		WithResultCallback(func(string, bool) {}).
		Build()
	stack.PushModalDialog(inputBox)
	stack.PushNonBlockingDialog(&stackTestWidget{})

	assert.Same(t, inputBox, stack.GetOrAssertTopInputBox())
}

func TestMessageDialogDismissCallbackRunsOnce(t *testing.T) {
	initializeDialogTestDisplay(t)
	dismissCount := 0
	dialog := NewMessageDialog("once").
		WithInputDismissal(newWidgetTestKeyboard(100)).
		WithEffects(false).
		WithDismissCallback(func() { dismissCount++ }).
		Build()

	dialog.dismiss()
	dialog.dismiss()

	assert.Equal(t, 1, dismissCount)
}

func initializeDialogTestDisplay(t *testing.T) {
	t.Helper()
	setResponsiveDialogTestResolution(t, 1920, 1080)
}

// A window opened on top of another shades everything behind it, once and by the
// same amount whatever lies between; a window alone over the play screen, or with
// only passing feedback above it, shades nothing.
func TestDialogStackShadesWhatLiesBehindAWindowOnAWindow(t *testing.T) {
	draw := func(build func(stack *DialogStack)) uint32 {
		screen := ebiten.NewImage(8, 8)
		t.Cleanup(screen.Deallocate)
		screen.Fill(color.White)
		var stack DialogStack
		build(&stack)
		stack.Draw(screen)
		red, _, _, _ := screen.At(4, 4).RGBA()

		return red >> 8
	}
	const white = uint32(255)
	shaded := white - backdropShadeAlpha

	assert.Equal(t, white, draw(func(stack *DialogStack) { stack.PushModalDialog(&stackTestWidget{}) }))
	assert.Equal(t, white, draw(func(stack *DialogStack) {
		stack.PushModalDialog(&stackTestWidget{})
		stack.PushNonBlockingDialog(&stackTestWidget{})
	}), "feedback over a window is not a window on a window")
	assert.Equal(t, shaded, draw(func(stack *DialogStack) {
		stack.PushModalDialog(&stackTestWidget{})
		stack.PushModalDialog(&stackTestWidget{})
	}))
	assert.Equal(t, shaded, draw(func(stack *DialogStack) {
		stack.PushModalDialog(&stackTestWidget{})
		stack.PushModalDialog(&stackTestWidget{})
		stack.PushModalDialog(&stackTestWidget{})
	}), "only the window being answered is set apart, so the shade is laid once")
}
