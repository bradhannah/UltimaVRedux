package widgets

import (
	"image"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
)

var _ modalFocusListener = (*AtlasNoteEditor)(nil)

// Dismiss releases the native session without submitting or invoking Cancel.
// Stack removal already releases focus; owners can also use this for teardown.
func (e *AtlasNoteEditor) Dismiss() {
	e.finished = true
	e.SuspendTextInput()
}

// SuspendTextInput retains accepted drafts and selection, discards undelivered
// native input, and requires held controls to release before the next Update.
// Call this when an owner suspends keyboard polling without updating the editor.
func (e *AtlasNoteEditor) SuspendTextInput() {
	e.keysReady = false
	clear(e.controlsBlocked[:])
	e.releaseNativeFields()
}

func (e *AtlasNoteEditor) onDialogFocusChanged(focused bool) {
	e.focused = focused
	e.nativeFocusOwned = focused
	e.SuspendTextInput()
}

func (e *AtlasNoteEditor) nativeEditing() bool {
	return e.appendChars == nil && !e.config.Deleting && e.nativeFields[e.focus].IsFocused()
}

func atlasEditorControlKeys() []ebiten.Key {
	return []ebiten.Key{
		ebiten.KeyEscape, ebiten.KeyTab, ebiten.KeyEnter, ebiten.KeyNumpadEnter,
		ebiten.KeyBackspace, ebiten.KeyDelete, ebiten.KeyLeft, ebiten.KeyRight,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyHome, ebiten.KeyEnd,
		ebiten.KeyPageUp, ebiten.KeyPageDown,
	}
}

// Native delivery precedes physical controls. A candidate-confirming press stays
// owned until release, even when composition ends inside a single native pump.
func (e *AtlasNoteEditor) prepareNativeInput() bool {
	// Construction still supports detached layout and the explicit legacy test
	// seam, but only the stack's top-modal notification grants native ownership.
	if !e.nativeFocusOwned {
		return true
	}
	if !e.isWindowFocused() || e.config.Keyboard.IsWaitingForRelease() {
		e.SuspendTextInput()

		return true
	}
	if !e.keysReady {
		for _, key := range atlasEditorControlKeys() {
			if e.config.Keyboard.IsKeyHeld(key) {
				return true
			}
		}
		e.keysReady = true
	}
	if e.config.Deleting || e.inputFailed {
		return e.blockNativeControls(false)
	}
	field := e.nativeFields[e.focus]
	if !field.IsFocused() {
		field.Focus()
	}
	_, _, wasComposing := field.CompositionSelection()
	handled, err := field.HandleInputWithBounds(e.nativeCaretBounds())
	if err != nil {
		logging.Warnf(logging.CategoryUI, "Note text input failed: %v", err)
		e.message = "Typing stopped working here. Press Esc and open the note again."
		e.inputFailed = true
		e.releaseNativeFields()
		e.blockNativeControls(true)

		return true
	}
	_, _, composing := field.CompositionSelection()
	e.acceptNativeText(handled, composing)
	owned := handled || wasComposing || composing
	blocked := e.blockNativeControls(owned)

	return owned || blocked
}

func (e *AtlasNoteEditor) blockNativeControls(owned bool) bool {
	blocked := false
	for _, key := range append(atlasEditorControlKeys(), ebiten.KeyA) {
		if !e.config.Keyboard.IsKeyHeld(key) {
			e.controlsBlocked[key] = false
		} else if owned {
			e.controlsBlocked[key] = true
		}
		blocked = blocked || e.controlsBlocked[key]
	}

	return blocked
}

func (e *AtlasNoteEditor) acceptNativeText(handled, composing bool) {
	index := e.focus
	field := e.nativeFields[index]
	value := field.Text()
	message := e.message
	// Keep the established all-or-nothing ASCII and capacity policy. Rejecting
	// an oversized replacement restores its original selection and suffix;
	// truncating the whole value would destroy otherwise untouched text.
	if !e.validFieldText(value) {
		if composing {
			// A pump can commit text and start another composition. Preserve
			// that live preedit; enforce rejected committed text when it ends.
			e.message = message

			return
		}
		e.restoreNativeField(index)

		return
	}
	start, end := field.Selection()
	anchor, caret := utf8.RuneCountInString(value[:start]), utf8.RuneCountInString(value[:end])
	if !handled && e.anchors[index] > e.cursors[index] {
		anchor, caret = caret, anchor
	}
	if value != string(e.fields[index]) {
		e.message = ""
	}
	if handled {
		e.upstream[index] = false
	}
	e.fields[index], e.anchors[index], e.cursors[index] = []rune(value), anchor, caret
}

func (e *AtlasNoteEditor) releaseNativeFields() {
	for index, field := range e.nativeFields {
		if field.IsFocused() {
			// Blur can flush native commits; only the accepted Update snapshot
			// survives a focus change, cancellation, submission or suspension.
			field.Blur()
			e.restoreNativeField(index)
		}
	}
}

func (e *AtlasNoteEditor) restoreNativeField(index int) {
	field := e.nativeFields[index]
	value := string(e.fields[index])
	anchor := len(string(e.fields[index][:e.anchors[index]]))
	caret := len(string(e.fields[index][:e.cursors[index]]))
	start, end := field.Selection()
	if value != field.Text() || start != min(anchor, caret) || end != max(anchor, caret) {
		field.SetTextAndSelection(value, min(anchor, caret), max(anchor, caret))
	}
}

func (e *AtlasNoteEditor) writeNativeField() {
	if e.nativeEditing() {
		e.restoreNativeField(e.focus)
	}
}

func (e *AtlasNoteEditor) selectAllNativeText() bool {
	keyboard := e.config.Keyboard
	if !keyboard.IsModifierPressed(input.ModifierCtrl) && !keyboard.IsModifierPressed(input.ModifierMeta) ||
		!keyboard.IsTextKeyPressedAndRegister(ebiten.KeyA) {
		return false
	}
	e.anchors[e.focus], e.cursors[e.focus] = 0, len(e.fields[e.focus])
	e.upstream[e.focus] = false
	e.writeNativeField()

	return true
}

func (e *AtlasNoteEditor) editNativeSelection(key ebiten.Key) {
	index := e.focus
	anchor, caret := e.anchors[index], e.cursors[index]
	start, end := min(anchor, caret), max(anchor, caret)
	selecting := e.config.Keyboard.IsModifierPressed(input.ModifierShift)
	switch {
	case start != end && (key == ebiten.KeyBackspace || key == ebiten.KeyDelete):
		e.fields[index] = append(e.fields[index][:start], e.fields[index][end:]...)
		e.cursors[index] = start
		e.upstream[index] = false
	case start != end && !selecting && key == ebiten.KeyLeft:
		e.cursors[index] = start
		e.upstream[index] = false
	case start != end && !selecting && key == ebiten.KeyRight:
		e.cursors[index] = end
		e.upstream[index] = false
	default:
		e.editAtCursor(key)
	}
	if !selecting || key == ebiten.KeyBackspace || key == ebiten.KeyDelete {
		e.anchors[index] = e.cursors[index]
	}
	e.writeNativeField()
}

// Rendering uses native preedit and byte selection without putting uncommitted
// Unicode into the ASCII journal draft. Logical movement stays rune-indexed.
func (e *AtlasNoteEditor) fieldPresentation(index int) ([]rune, int, int) {
	field := e.nativeFields[index]
	if e.appendChars != nil || !field.IsFocused() {
		return e.fields[index], e.cursors[index], e.cursors[index]
	}
	value := field.TextForRendering()
	anchor, caret := field.Selection()
	if e.anchors[index] > e.cursors[index] {
		anchor, caret = caret, anchor
	}
	if start, end, composing := field.CompositionSelection(); composing {
		base, _ := field.Selection()
		anchor, caret = base+start, base+end
	}
	anchor, caret = min(anchor, len(value)), min(caret, len(value))

	return []rune(value), utf8.RuneCountInString(value[:anchor]), utf8.RuneCountInString(value[:caret])
}

func (e *AtlasNoteEditor) nativeCaretBounds() image.Rectangle {
	index := e.focus
	layout := e.fieldLayout(index)
	content, _, caret := e.fieldPresentation(index)
	lines, row := e.scrollForContent(index, layout, content, caret)
	value := string(content[lines[row].start:lines[row].end])
	position := len(string(content[lines[row].start:caret]))
	_, _, advance := inputTextGeometry(e.font, value, position, position, position)

	return layout.caretRect(row-e.scroll[index], advance).Intersect(layout.content)
}

func (e *AtlasNoteEditor) populateFieldLine(screen *ebiten.Image, content []rune, line atlasEditorLine,
	point image.Point, anchor, caret, height int,
) {
	start := max(line.start, min(line.end, min(anchor, caret)))
	end := max(start, min(line.end, max(anchor, caret)))
	if start == end {
		atlasSingleLine(e.output, string(content[line.start:line.end]), DialogValueColor)

		return
	}
	prefix := string(content[line.start:start])
	selected, suffix := string(content[start:end]), string(content[end:line.end])
	before, width, _ := inputTextGeometry(e.font, prefix+selected+suffix,
		len(prefix), len(prefix)+len(selected), len(prefix))
	fillAtlasRect(screen, image.Rect(point.X+int(before), point.Y, point.X+int(before+width), point.Y+height),
		DialogSelectedBackground)
	e.output.Reset()
	e.output.SetMaxCharsPerLine(max(1, line.end-line.start))
	e.output.AppendLine().Text(prefix, DialogValueColor).Text(selected, DialogSelectedText).
		Text(suffix, DialogValueColor).Done()
}
