package widgets

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// dialogEntry represents a dialog in the stack with its modal status.
type dialogEntry struct {
	widget  Widget
	isModal bool // true = blocks input, false = non-blocking (like action feedback popups)
}

// modalFocusListener is implemented by modal widgets that own platform input focus.
// Only the highest modal receives focus; non-blocking dialogs do not affect it.
type modalFocusListener interface {
	onDialogFocusChanged(focused bool)
}

// DialogStack manages a stack of dialogs with LIFO (last-in-first-out) behavior.
// It provides centralized dialog lifecycle management for the game's interface system.
// Dialogs can be modal (blocking input) or non-blocking (allowing game to continue).
type DialogStack struct {
	entries []dialogEntry
}

// PushModalDialog adds a new modal (input-blocking) dialog to the top of the stack.
// The dialog becomes the active modal and will receive input focus.
// Use this for dialogs that require user interaction before continuing (confirmations, menus, conversations).
func (d *DialogStack) PushModalDialog(dialog Widget) {
	d.mustNotContain(dialog)
	previousModal := d.PeekTopModalDialog()
	logging.Infof(logging.CategoryUI, "PushModalDialog called, stack size before: %d", len(d.entries))
	d.entries = append(d.entries, dialogEntry{widget: dialog, isModal: true})
	d.updateModalFocus(previousModal, dialog)
	logging.Infof(logging.CategoryUI, "PushModalDialog completed, stack size after: %d", len(d.entries))
}

// PushNonBlockingDialog adds a non-blocking dialog to the stack.
// Non-blocking dialogs are displayed but don't prevent game input from being processed.
// Use this for action feedback popups and other informational displays that shouldn't interrupt gameplay.
func (d *DialogStack) PushNonBlockingDialog(dialog Widget) {
	d.mustNotContain(dialog)
	logging.Infof(logging.CategoryUI, "PushNonBlockingDialog called, stack size before: %d", len(d.entries))
	d.entries = append(d.entries, dialogEntry{widget: dialog, isModal: false})
	logging.Infof(logging.CategoryUI, "PushNonBlockingDialog completed, stack size after: %d", len(d.entries))
}

// PeekTopModalDialog returns the highest input-blocking dialog without removing it.
// Non-blocking dialogs may be physically above it in the stack.
//
//nolint:ireturn // The stack holds many dialog types; callers check which is on top.
func (d *DialogStack) PeekTopModalDialog() Widget {
	for i := len(d.entries) - 1; i >= 0; i-- {
		if d.entries[i].isModal {
			return d.entries[i].widget
		}
	}

	return nil
}

// HasOpenDialog returns true if there are any dialogs currently on the stack.
func (d *DialogStack) HasOpenDialog() bool {
	return len(d.entries) > 0
}

// HasModalDialog returns true if there are any modal (input-blocking) dialogs on the stack.
// This is used by the game loop to determine if input should be blocked.
func (d *DialogStack) HasModalDialog() bool {
	for _, entry := range d.entries {
		if entry.isModal {
			return true
		}
	}

	return false
}

// GetOrAssertTopInputBox returns the top modal dialog as an InputBox. It returns
// nil when no modal is open, and logs a warning and returns nil when the top
// modal is another widget type.
func (d *DialogStack) GetOrAssertTopInputBox() *InputBox {
	top := d.PeekTopModalDialog()
	if top == nil {
		return nil
	}

	inputBox, ok := top.(*InputBox)

	if !ok {
		logging.Warnf(logging.CategoryUI, "Expected InputBox on dialog stack but found different type, returning nil")

		return nil // Graceful degradation instead of crash
	}

	return inputBox
}

// RemoveWidgetInstance removes a specific widget instance from the stack by pointer equality.
// Returns true if the widget was found and removed, false if not found.
// This is safer than RemoveWidget when multiple dialogs of the same type may exist,
// as it matches by instance rather than by type.
func (d *DialogStack) RemoveWidgetInstance(widget Widget) bool {
	for index, entry := range d.entries {
		if entry.widget == widget {
			previousModal := d.PeekTopModalDialog()
			d.entries = append(d.entries[:index], d.entries[index+1:]...)
			d.updateModalFocus(previousModal, d.PeekTopModalDialog())
			logging.Debugf(logging.CategoryUI,
				"RemoveWidgetInstance: Removed widget at index %d, stack size now: %d", index, len(d.entries))

			return true
		}
	}

	logging.Debugf(logging.CategoryUI, "RemoveWidgetInstance: Widget not found in stack (already removed or never added)")

	return false
}

// ContainsWidgetInstance reports whether the exact widget is still stack-owned.
func (d *DialogStack) ContainsWidgetInstance(widget Widget) bool {
	for _, entry := range d.entries {
		if entry.widget == widget {
			return true
		}
	}

	return false
}

// MustRemoveWidgetInstance removes an owned widget or panics if stack ownership was lost.
func (d *DialogStack) MustRemoveWidgetInstance(widget Widget) {
	if !d.RemoveWidgetInstance(widget) {
		panic("DialogStack.MustRemoveWidgetInstance: required widget is not owned by this stack")
	}
}

// UpdateNonBlockingDialogs advances dialogs that cannot consume modal input.
func (d *DialogStack) UpdateNonBlockingDialogs() {
	for _, entry := range d.entries {
		if !entry.isModal {
			entry.widget.Update()
		}
	}
}

// Draw renders all dialogs from bottom to top without allocating a snapshot.
func (d *DialogStack) Draw(screen *ebiten.Image) {
	top, modals := -1, 0
	for index, entry := range d.entries {
		if entry.isModal {
			top = index
			modals++
		}
	}
	for index, entry := range d.entries {
		// A window opened on top of another is the one being answered: everything
		// behind it is shaded so that it reads as waiting. A window alone over the
		// play screen shades nothing.
		if index == top && modals > 1 {
			drawBackdropShade(screen)
		}
		entry.widget.Draw(screen)
	}
}

// backdropShadeAlpha is how dark the screen behind a window on top of another is
// made, of 255: enough that it recedes, while it can still be read.
const backdropShadeAlpha = 150

// drawBackdropShade darkens everything drawn so far with one flat shade.
func drawBackdropShade(screen *ebiten.Image) {
	bounds := screen.Bounds()
	vector.FillRect(screen, float32(bounds.Min.X), float32(bounds.Min.Y), float32(bounds.Dx()), float32(bounds.Dy()),
		color.RGBA{R: 0, G: 0, B: 0, A: backdropShadeAlpha}, false)
}

func (d *DialogStack) mustNotContain(widget Widget) {
	if widget == nil {
		panic("DialogStack: cannot push a nil widget")
	}
	for _, entry := range d.entries {
		if entry.widget == widget {
			panic("DialogStack: cannot push the same widget instance twice")
		}
	}
}

func (*DialogStack) updateModalFocus(previous, current Widget) {
	if previous == current {
		return
	}
	if listener, ok := previous.(modalFocusListener); ok {
		listener.onDialogFocusChanged(false)
	}
	if listener, ok := current.(modalFocusListener); ok {
		listener.onDialogFocusChanged(true)
	}
}

// Dialogs returns all widgets in the stack for iteration (e.g., drawing all dialogs).
// Returns widgets in order from bottom to top.
func (d *DialogStack) Dialogs() []Widget {
	widgets := make([]Widget, len(d.entries))
	for i, entry := range d.entries {
		widgets[i] = entry.widget
	}

	return widgets
}
