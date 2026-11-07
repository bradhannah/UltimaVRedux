package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	saveDescriptionModalStartX       = 0.15
	saveDescriptionModalEndX         = 0.85
	saveDescriptionReferenceWidth    = 1920
	saveDescriptionContentPadding    = 25
	saveDescriptionMinimumPadding    = 8
	saveDescriptionFieldPaddingX     = 16
	saveDescriptionFieldPaddingY     = 8
	saveDescriptionVerticalSpacing   = 16
	saveDescriptionMinimumPaddingX   = 6
	saveDescriptionMinimumPaddingY   = 4
	saveDescriptionMinimumSpacing    = 6
	saveDescriptionHintMaximumLines  = 2
	saveDescriptionCenterDivisor     = 2
	saveDescriptionTailMarker        = "..."
	saveDescriptionValidationLines   = 2
	saveDescriptionBackspaceDelay    = 21 // 350ms at Ebitengine's default 60 update ticks/second.
	saveDescriptionBackspaceInterval = 3  // 50ms; no global keyboard repeat changes.
	saveDescriptionInputErrorMessage = "Text input failed."
)

var _ modalFocusListener = (*SaveDescriptionEntryModal)(nil)

// SaveDescriptionEntryCallback receives the description and whether Enter submitted it.
// A false submitted value means the modal was cancelled.
type SaveDescriptionEntryCallback func(text string, submitted bool)

// SaveDescriptionEntryModalConfig configures optional checkpoint text and the
// reviewed overwrite warning. Persistence remains the caller's responsibility.
type SaveDescriptionEntryModalConfig struct {
	Title string
	// Prompt names the field; empty keeps "Description (optional)".
	Prompt       string
	Notice       string
	InitialText  string
	Keyboard     *input.Keyboard
	OnResult     SaveDescriptionEntryCallback
	Validate     func(string) string
	MaxRunes     int
	IsKeyPressed func(ebiten.Key) bool
}

// SaveDescriptionEntryResult is the typed value returned through GetResult.
type SaveDescriptionEntryResult struct {
	Text      string
	Submitted bool
}

type saveDescriptionTextField = nativeTextField

// SaveDescriptionEntryModal is a centered, single-line checkpoint description editor.
// Push it with DialogStack.PushModalDialog so platform input focus follows the
// active modal. Use AutoCloseAndPop when the modal should remove itself.
type SaveDescriptionEntryModal struct {
	prompt   string
	keyboard *input.Keyboard
	onResult SaveDescriptionEntryCallback
	validate func(string) string
	field    saveDescriptionTextField

	isKeyPressed                func(ebiten.Key) bool
	isWindowFocused             func() bool
	validationMessage           string
	clearLineHeld               bool
	backspaceHeld               bool
	backspaceTicks              int
	compositionBackspaceBlocked bool
	compositionClearBlocked     bool
	selectionReversed           bool
	enterBlocked                bool
	maxRunes                    int
	title                       string
	notice                      string
	titleOutput                 *text.Output
	noticeOutput                *text.Output

	autoCloseStack *DialogStack
	finished       bool
	result         SaveDescriptionEntryResult

	border        *Border
	font          *text.UltimaFont
	promptOutput  *text.Output
	fieldOutput   *text.Output
	hintOutput    *text.Output
	messageOutput *text.Output
	fieldRect     image.Rectangle
	lastWidth     int
	lastHeight    int
}

// NewSaveDescriptionEntryModal creates a description editor with native text input.
// OnResult and Keyboard are mandatory. The field gains focus when this widget
// becomes the top modal in a DialogStack.
func NewSaveDescriptionEntryModal(config SaveDescriptionEntryModalConfig) *SaveDescriptionEntryModal {
	return newSaveDescriptionEntryModal(config, newNativeTextField())
}

func newSaveDescriptionEntryModal(
	config SaveDescriptionEntryModalConfig, field saveDescriptionTextField,
) *SaveDescriptionEntryModal {
	if config.Keyboard == nil {
		panic("SaveDescriptionEntryModal: keyboard is nil")
	}
	if config.OnResult == nil {
		panic("SaveDescriptionEntryModal: OnResult is nil")
	}
	if field == nil {
		panic("SaveDescriptionEntryModal: text field is nil")
	}

	modal := new(SaveDescriptionEntryModal)
	modal.prompt = "Description (optional)"
	if config.Prompt != "" {
		modal.prompt = config.Prompt
	}
	modal.maxRunes = config.MaxRunes
	modal.title = config.Title
	if modal.title == "" {
		modal.title = "Save Game"
	}
	modal.notice = strings.TrimSpace(config.Notice)
	modal.keyboard = config.Keyboard
	modal.onResult = config.OnResult
	modal.validate = config.Validate
	modal.field = field
	modal.isKeyPressed = config.IsKeyPressed
	modal.isWindowFocused = ebiten.IsFocused
	if modal.isKeyPressed == nil {
		modal.isKeyPressed = ebiten.IsKeyPressed
	}
	modal.SetText(config.InitialText)

	return modal
}

// Draw renders the modal, description field, and keyboard hints.
func (m *SaveDescriptionEntryModal) Draw(screen *ebiten.Image) {
	if screen == nil {
		return
	}

	width, height := screen.Bounds().Dx(), screen.Bounds().Dy()
	rects := m.preparePresentation(width, height)
	FillClassicPixelRoundedRect(screen, m.border.GetBounds(), m.pixelUnit(), ucolor.DialogBackground.RGBA())
	m.drawDescriptionField(screen, rects.field)

	m.promptOutput.DrawPanelContent(screen, rects.prompt.Min, rects.prompt.Dx(), etext.AlignStart)
	m.titleOutput.DrawPanelContent(screen,
		image.Pt(rects.title.Min.X, centeredGlyphOutputY(rects.title, m.titleOutput, m.title)),
		rects.title.Dx(), etext.AlignCenter)
	m.noticeOutput.DrawPanelContent(screen, rects.notice.Min, rects.notice.Dx(), etext.AlignStart)

	if m.validationMessage != "" {
		m.messageOutput.DrawPanelContent(screen, rects.message.Min, rects.message.Dx(), etext.AlignStart)
	}
	m.hintOutput.DrawPanelContent(screen, rects.hint.Min, rects.hint.Dx(), etext.AlignCenter)
	DrawClassicPixelRoundedFrame(screen, m.border.GetBounds(), m.pixelUnit(), DialogRuleColor)
}

func (m *SaveDescriptionEntryModal) preparePresentation(width, height int) saveDescriptionEntryRects {
	m.ensureRendering(width, height)
	rects := m.calculateRects()
	m.fieldRect = rects.field
	m.promptOutput.Reset()
	m.promptOutput.AppendLine().Text(m.prompt, DialogHeaderColor).Done()
	if m.validationMessage != "" {
		m.messageOutput.Reset()
		m.messageOutput.AppendLine().Text(m.validationMessage, DialogStatusWarningColor).WordWrap().Done()
	}

	m.hintOutput.Reset()
	AppendControlHints(m.hintOutput.AppendLine(), "[Ctrl+U] Clear description").Done()
	AppendControlHints(m.hintOutput.AppendLine(), "[Enter] Save   [Esc] Back").Done()

	return rects
}

func (m *SaveDescriptionEntryModal) drawDescriptionField(screen *ebiten.Image, fieldRect image.Rectangle) {
	innerRect := m.drawDescriptionFieldFrame(screen, fieldRect)
	contentRect := m.descriptionFieldContentRect(innerRect)
	fieldWidth := max(1, contentRect.Dx())
	_, selectionEnd := m.directedSelection()
	start, end := orderedSaveDescriptionSelection(m.field)
	if compositionStart, compositionEnd, composing := m.field.CompositionSelection(); composing {
		start, end = start+compositionStart, start+compositionEnd
		selectionEnd = end
	}
	value := m.field.TextForRendering()
	selectionEnd = min(max(selectionEnd, 0), len(value))
	start, end = min(max(start, 0), len(value)), min(max(end, start), len(value))
	visibleText, cursorRune := saveDescriptionVisibleWindow(
		value,
		selectionEnd,
		m.font.CharsForWidth(fieldWidth),
	)
	fieldY := contentRect.Min.Y + max(
		0,
		(contentRect.Dy()-int(m.fieldOutput.GetFontHeight()))/saveDescriptionCenterDivisor,
	)
	runes := []rune(visibleText)
	offset := utf8.RuneCountInString(value[:selectionEnd]) - cursorRune
	first := min(len(runes), max(0, utf8.RuneCountInString(value[:start])-offset))
	last := min(len(runes), max(first, utf8.RuneCountInString(value[:end])-offset))
	if !m.field.IsFocused() {
		first, last = 0, 0
	}
	prefix, selected, suffix := string(runes[:first]), string(runes[first:last]), string(runes[last:])
	prefixWidth, selectedWidth, _ := inputTextGeometry(m.font, visibleText,
		len(prefix), len(prefix)+len(selected), len(prefix))
	vector.FillRect(screen, float32(contentRect.Min.X)+float32(prefixWidth), float32(contentRect.Min.Y),
		float32(selectedWidth), float32(contentRect.Dy()), DialogSelectedBackground, false)
	m.fieldOutput.Reset()
	m.fieldOutput.AppendLine().Text(prefix, ucolor.DialogInputTextColor).
		Text(selected, DialogSelectedText).Text(suffix, ucolor.DialogInputTextColor).Done()
	m.fieldOutput.DrawPanelContent(
		screen,
		image.Pt(contentRect.Min.X, fieldY),
		fieldWidth,
		etext.AlignStart,
	)
	m.drawDescriptionCursor(screen, contentRect, visibleText, cursorRune)
}

func (m *SaveDescriptionEntryModal) drawDescriptionFieldFrame(
	screen *ebiten.Image,
	fieldRect image.Rectangle,
) image.Rectangle {
	frameWidth := m.pixelUnit()
	fieldBorderColor := DialogRuleLightColor
	if m.field.IsFocused() && !m.finished {
		fieldBorderColor = ucolor.EGACyan
	}
	vector.FillRect(
		screen,
		float32(fieldRect.Min.X),
		float32(fieldRect.Min.Y),
		float32(fieldRect.Dx()),
		float32(fieldRect.Dy()),
		fieldBorderColor,
		false,
	)
	innerRect := fieldRect.Inset(frameWidth)
	vector.FillRect(
		screen,
		float32(innerRect.Min.X),
		float32(innerRect.Min.Y),
		float32(innerRect.Dx()),
		float32(innerRect.Dy()),
		ucolor.EGABlack,
		false,
	)

	return innerRect
}

func (m *SaveDescriptionEntryModal) descriptionFieldContentRect(innerRect image.Rectangle) image.Rectangle {
	paddingX := m.scaledDimension(saveDescriptionFieldPaddingX, saveDescriptionMinimumPaddingX)
	paddingY := m.scaledDimension(saveDescriptionFieldPaddingY, saveDescriptionMinimumPaddingY)

	return image.Rect(
		min(innerRect.Max.X, innerRect.Min.X+paddingX),
		min(innerRect.Max.Y, innerRect.Min.Y+paddingY),
		max(innerRect.Min.X, innerRect.Max.X-paddingX),
		max(innerRect.Min.Y, innerRect.Max.Y-paddingY),
	)
}

func (m *SaveDescriptionEntryModal) drawDescriptionCursor(
	screen *ebiten.Image,
	contentRect image.Rectangle,
	visibleText string,
	cursorRune int,
) {
	if !m.field.IsFocused() || m.finished {
		return
	}
	visibleRunes := []rune(visibleText)
	cursorRune = min(max(cursorRune, 0), len(visibleRunes))
	position := len(string(visibleRunes[:cursorRune]))
	_, _, textAdvance := inputTextGeometry(m.font, visibleText, position, position, position)
	cursorX := min(contentRect.Max.X, contentRect.Min.X+int(textAdvance))
	vector.StrokeLine(
		screen,
		float32(cursorX),
		float32(contentRect.Min.Y),
		float32(cursorX),
		float32(contentRect.Max.Y),
		float32(m.pixelUnit()),
		ucolor.DialogInputTextColor,
		false,
	)
}

// Update consumes platform text input before handling modal control keys.
func (m *SaveDescriptionEntryModal) Update() {
	if m.finished || !m.field.IsFocused() || !m.isWindowFocused() {
		m.resetEditingRepeat()
		m.enterBlocked = true

		return
	}
	if !m.anyKeyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter) {
		m.enterBlocked = false
	}
	width, height := display.GetManager().GetCurrentSize()
	m.ensureRendering(width, height)
	if m.fieldRect.Empty() {
		m.fieldRect = m.calculateRects().field
	}
	previous := m.field.Text()
	start, end := m.directedSelection()
	if m.handleNativeInput(previous, start, end) {
		return
	}
	if m.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		m.finish(false)

		return
	}
	// Drain platform events first, then arbitrate owned physical edit keys. A
	// non-composition deletion or typed shortcut letter must not duplicate the local edit.
	if m.handlePhysicalEditing(previous, start, end) {
		return
	}
	if !m.acceptPlatformText(previous, start, end) {
		return
	}
	m.handleControlKey()
}

// handleNativeInput reports whether native input or its error owns this tick.
func (m *SaveDescriptionEntryModal) handleNativeInput(previous string, start, end int) bool {
	_, _, wasComposing := m.field.CompositionSelection()
	handled, err := m.field.HandleInputWithBounds(m.fieldRect)
	if err != nil {
		// The native Field retains input errors; keep the text and explanation
		// visible until Escape rather than silently returning to the browser.
		if m.validationMessage != saveDescriptionInputErrorMessage {
			logging.Warnf(logging.CategoryUI, "Text entry input failed: %v", err)
		}
		m.validationMessage = saveDescriptionInputErrorMessage
		if m.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			m.finish(false)
		}

		return true
	}
	if handled {
		m.selectionReversed = false
	}
	_, _, composing := m.field.CompositionSelection()
	if wasComposing || composing || handled && !m.isNativeShortcutEdit(previous, start, end) {
		// Preedit changes (including deletion of its last character) belong to
		// the IME. SetTextAndSelection would terminate that native session.
		m.resetEditingRepeat()
		m.compositionBackspaceBlocked = m.isKeyPressed(ebiten.KeyBackspace)
		m.compositionClearBlocked = m.controlModifierPressed() && m.isKeyPressed(ebiten.KeyU)
		m.enterBlocked = m.enterBlocked || m.anyKeyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter)
		if m.acceptPlatformText(previous, start, end) && handled {
			m.validationMessage = ""
		}

		return true
	}

	return false
}

func (m *SaveDescriptionEntryModal) acceptPlatformText(previous string, anchor, caret int) bool {
	if !m.acceptFieldText(m.field.Text()) {
		m.setFieldTextAndSelection(previous, anchor, caret)

		return false
	}

	return true
}

// Only known non-composition edits may use local chord debounce/repeat instead
// of the platform's cadence. All other handled events retain native ownership.
func (m *SaveDescriptionEntryModal) isNativeShortcutEdit(previous string, anchor, caret int) bool {
	start, end := min(anchor, caret), max(anchor, caret)
	if m.controlModifierPressed() && m.isKeyPressed(ebiten.KeyU) {
		return m.field.Text() == previous[:start]+"u"+previous[end:] ||
			m.field.Text() == previous[:start]+"U"+previous[end:]
	}
	if !m.isKeyPressed(ebiten.KeyBackspace) {
		return false
	}
	if start == end && start > 0 {
		_, runeLength := utf8.DecodeLastRuneInString(previous[:start])
		start -= runeLength
	}

	return start != end && m.field.Text() == previous[:start]+previous[end:]
}

func (m *SaveDescriptionEntryModal) handlePhysicalEditing(previous string, start, end int) bool {
	clearPressed := m.controlModifierPressed() && m.isKeyPressed(ebiten.KeyU)
	backspacePressed := m.isKeyPressed(ebiten.KeyBackspace)
	if !backspacePressed {
		m.backspaceHeld = false
		m.backspaceTicks = 0
		m.compositionBackspaceBlocked = false
	}
	if !clearPressed {
		m.compositionClearBlocked = false
	}
	if m.compositionBackspaceBlocked || m.compositionClearBlocked {
		return true
	}
	if clearPressed {
		m.setFieldTextAndSelection(previous, start, end)

		return m.handleClearLineShortcut()
	}
	m.clearLineHeld = false
	if !backspacePressed {
		return false
	}
	currentStart, currentEnd := m.directedSelection()
	if previous != m.field.Text() || start != currentStart || end != currentEnd {
		m.setFieldTextAndSelection(previous, start, end)
	}
	if !m.backspaceHeld {
		m.backspaceHeld = true
		m.backspaceTicks = saveDescriptionBackspaceDelay
	} else {
		m.backspaceTicks--
		if m.backspaceTicks > 0 {
			return true
		}
		m.backspaceTicks = saveDescriptionBackspaceInterval
	}
	m.validationMessage = ""
	m.deleteBackward()

	return true
}

func (m *SaveDescriptionEntryModal) resetEditingRepeat() {
	m.backspaceHeld = false
	m.backspaceTicks = 0
	m.clearLineHeld = false
	m.compositionBackspaceBlocked = false
	m.compositionClearBlocked = false
}

func (m *SaveDescriptionEntryModal) handleControlKey() {
	if m.handleClearLineShortcut() || m.handlePrimaryShortcut() || m.handleCompletionKey() {
		return
	}
	m.handleEditingKey()
}

func (m *SaveDescriptionEntryModal) handleClearLineShortcut() bool {
	pressed := m.controlModifierPressed() && m.isKeyPressed(ebiten.KeyU)
	if !pressed {
		m.clearLineHeld = false

		return false
	}
	if m.clearLineHeld {
		return true
	}

	m.clearLineHeld = true
	m.SetText("")

	return true
}

func (m *SaveDescriptionEntryModal) handlePrimaryShortcut() bool {
	if !m.primaryModifierPressed() || !m.keyboard.IsKeyPressedAndRegister(ebiten.KeyA) {
		return false
	}
	valueLength := len(m.field.Text())
	m.setFieldTextAndSelection(m.field.Text(), 0, valueLength)

	return true
}

func (m *SaveDescriptionEntryModal) handleCompletionKey() bool {
	if m.enterBlocked {
		return true
	}
	switch {
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter),
		m.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		m.submit()
	default:
		return false
	}

	return true
}

func (m *SaveDescriptionEntryModal) handleEditingKey() {
	switch {
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyBackspace):
		m.validationMessage = ""
		m.deleteBackward()
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete):
		m.validationMessage = ""
		m.deleteForward()
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft):
		m.moveCursorLeft(m.shiftPressed())
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight):
		m.moveCursorRight(m.shiftPressed())
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyHome):
		m.moveCursorHome(m.shiftPressed())
	case m.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnd):
		m.moveCursorEnd(m.shiftPressed())
	}
}

func (m *SaveDescriptionEntryModal) submit() {
	if !m.acceptFieldText(m.field.Text()) {
		return
	}
	if m.validate != nil {
		if message := strings.TrimSpace(m.validate(m.field.Text())); message != "" {
			m.validationMessage = message

			return
		}
	}
	m.validationMessage = ""
	m.finish(true)
}

func (m *SaveDescriptionEntryModal) primaryModifierPressed() bool {
	return m.controlModifierPressed() || m.anyKeyPressed(
		ebiten.KeyMeta,
		ebiten.KeyMetaLeft,
		ebiten.KeyMetaRight,
	)
}

func (m *SaveDescriptionEntryModal) controlModifierPressed() bool {
	return m.anyKeyPressed(ebiten.KeyControl, ebiten.KeyControlLeft, ebiten.KeyControlRight)
}

func (m *SaveDescriptionEntryModal) shiftPressed() bool {
	return m.anyKeyPressed(ebiten.KeyShift, ebiten.KeyShiftLeft, ebiten.KeyShiftRight)
}

func (m *SaveDescriptionEntryModal) anyKeyPressed(keys ...ebiten.Key) bool {
	for _, key := range keys {
		if m.isKeyPressed(key) {
			return true
		}
	}

	return false
}

// SetText replaces a valid description and places the cursor at the end.
func (m *SaveDescriptionEntryModal) SetText(value string) {
	if !m.acceptFieldText(value) {
		return
	}
	m.setFieldTextAndSelection(value, len(value), len(value))
	m.validationMessage = ""
}

func (m *SaveDescriptionEntryModal) onDialogFocusChanged(focused bool) {
	m.resetEditingRepeat()
	if focused && !m.finished {
		m.field.Focus()
		m.enterBlocked = m.anyKeyPressed(ebiten.KeyEnter, ebiten.KeyNumpadEnter)

		return
	}
	m.field.Blur()
}

func (m *SaveDescriptionEntryModal) finish(submitted bool) {
	if m.finished {
		return
	}

	m.finished = true
	m.resetEditingRepeat()
	m.result = SaveDescriptionEntryResult{
		Text:      m.field.Text(),
		Submitted: submitted,
	}
	m.field.Blur()
	m.onResult(m.result.Text, submitted)
	if m.autoCloseStack != nil {
		m.autoCloseStack.MustRemoveWidgetInstance(m)
	}
}

func (m *SaveDescriptionEntryModal) acceptFieldText(value string) bool {
	if !utf8.ValidString(value) || strings.ContainsFunc(value, unicode.IsControl) {
		m.validationMessage = "Description must be a single line without control characters."

		return false
	}
	if m.maxRunes > 0 && utf8.RuneCountInString(value) > m.maxRunes {
		m.validationMessage = fmt.Sprintf("Description must be at most %d characters.", m.maxRunes)

		return false
	}

	return true
}

func (m *SaveDescriptionEntryModal) deleteBackward() {
	value := m.field.Text()
	start, end := orderedSaveDescriptionSelection(m.field)
	if start == end && start > 0 {
		_, runeLength := utf8.DecodeLastRuneInString(value[:start])
		start -= runeLength
	}
	if start == end {
		return
	}
	value = value[:start] + value[end:]
	m.setFieldTextAndSelection(value, start, start)
}

func (m *SaveDescriptionEntryModal) deleteForward() {
	value := m.field.Text()
	start, end := orderedSaveDescriptionSelection(m.field)
	if start == end && end < len(value) {
		_, runeLength := utf8.DecodeRuneInString(value[end:])
		end += runeLength
	}
	if start == end {
		return
	}
	value = value[:start] + value[end:]
	m.setFieldTextAndSelection(value, start, start)
}

func (m *SaveDescriptionEntryModal) moveCursorLeft(selecting bool) {
	value := m.field.Text()
	selectionStart, selectionEnd := m.directedSelection()
	if !selecting {
		start, end := orderedSaveDescriptionSelection(m.field)
		if start == end && start > 0 {
			_, runeLength := utf8.DecodeLastRuneInString(value[:start])
			start -= runeLength
		}
		m.setFieldTextAndSelection(value, start, start)

		return
	}
	selectionEnd = min(max(selectionEnd, 0), len(value))
	if selectionEnd > 0 {
		_, runeLength := utf8.DecodeLastRuneInString(value[:selectionEnd])
		selectionEnd -= runeLength
	}
	m.setFieldTextAndSelection(value, selectionStart, selectionEnd)
}

func (m *SaveDescriptionEntryModal) moveCursorRight(selecting bool) {
	value := m.field.Text()
	selectionStart, selectionEnd := m.directedSelection()
	if !selecting {
		start, end := orderedSaveDescriptionSelection(m.field)
		if start == end && end < len(value) {
			_, runeLength := utf8.DecodeRuneInString(value[end:])
			end += runeLength
		}
		m.setFieldTextAndSelection(value, end, end)

		return
	}
	selectionEnd = min(max(selectionEnd, 0), len(value))
	if selectionEnd < len(value) {
		_, runeLength := utf8.DecodeRuneInString(value[selectionEnd:])
		selectionEnd += runeLength
	}
	m.setFieldTextAndSelection(value, selectionStart, selectionEnd)
}

func (m *SaveDescriptionEntryModal) moveCursorHome(selecting bool) {
	if selecting {
		selectionStart, _ := m.directedSelection()
		m.setFieldTextAndSelection(m.field.Text(), selectionStart, 0)

		return
	}
	m.setFieldTextAndSelection(m.field.Text(), 0, 0)
}

func (m *SaveDescriptionEntryModal) moveCursorEnd(selecting bool) {
	textLength := len(m.field.Text())
	if selecting {
		selectionStart, _ := m.directedSelection()
		m.setFieldTextAndSelection(m.field.Text(), selectionStart, textLength)

		return
	}
	m.setFieldTextAndSelection(m.field.Text(), textLength, textLength)
}

// The native field requires ordered replacement ranges. Keep the active end
// locally so Shift navigation can shrink, reverse, and extend a selection.
func (m *SaveDescriptionEntryModal) setFieldTextAndSelection(value string, anchor, caret int) {
	m.selectionReversed = anchor > caret
	m.field.SetTextAndSelection(value, min(anchor, caret), max(anchor, caret))
}

func (m *SaveDescriptionEntryModal) directedSelection() (int, int) {
	start, end := orderedSaveDescriptionSelection(m.field)
	if m.selectionReversed {
		return end, start
	}

	return start, end
}

func (m *SaveDescriptionEntryModal) ensureRendering(width, height int) {
	if m.border != nil && width == m.lastWidth && height == m.lastHeight {
		return
	}
	m.lastWidth = width
	m.lastHeight = height
	// Establish the measured content width before sizing the complete dialog height.
	m.border = newDialogBorder(sprites.PercentBasedPlacement{
		StartPercentX: saveDescriptionModalStartX, EndPercentX: saveDescriptionModalEndX,
		StartPercentY: 0, EndPercentY: 1,
	})
	m.font, m.fieldOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	})
	_, m.promptOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	})
	_, m.hintOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              saveDescriptionHintMaximumLines,
		MaxChars:              1,
	})
	_, m.messageOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              saveDescriptionValidationLines,
		MaxChars:              1,
	})
	m.configureDescriptionRendering()

	rects := m.calculateRects()
	m.fieldRect = rects.field
	m.promptOutput.SetMaxCharsPerLine(m.promptOutput.Font.CharsForWidth(max(1, rects.prompt.Dx())))
	fieldInset := saveDescriptionCenterDivisor *
		(m.pixelUnit() + m.scaledDimension(saveDescriptionFieldPaddingX, saveDescriptionMinimumPaddingX))
	m.fieldOutput.SetMaxCharsPerLine(m.font.CharsForWidth(max(1, rects.field.Dx()-fieldInset)))
	m.hintOutput.SetMaxCharsPerLine(m.hintOutput.Font.CharsForWidth(max(1, rects.hint.Dx())))
	m.messageOutput.SetMaxCharsPerLine(m.messageOutput.Font.CharsForWidth(max(1, rects.message.Dx())))
}

type saveDescriptionEntryRects struct {
	title   image.Rectangle
	notice  image.Rectangle
	prompt  image.Rectangle
	field   image.Rectangle
	message image.Rectangle
	hint    image.Rectangle
}

func (m *SaveDescriptionEntryModal) scaledDimension(value, minimum int) int {
	return max(minimum, int(math.Round(float64(value)*float64(max(1, m.lastWidth))/saveDescriptionReferenceWidth)))
}

func (m *SaveDescriptionEntryModal) pixelUnit() int {
	return getClassicUIPixelUnit(m.lastWidth, m.lastHeight)
}

func saveDescriptionVisibleWindow(value string, cursorByte, maxRunes int) (string, int) {
	if maxRunes <= 0 {
		return "", 0
	}
	runes := []rune(value)
	cursorByte = min(max(cursorByte, 0), len(value))
	cursorRune := utf8.RuneCountInString(value[:cursorByte])
	if len(runes) <= maxRunes {
		return value, cursorRune
	}
	markerLength := utf8.RuneCountInString(saveDescriptionTailMarker)
	if maxRunes <= markerLength {
		start := min(max(cursorRune-maxRunes/saveDescriptionCenterDivisor, 0), len(runes)-maxRunes)

		return string(runes[start : start+maxRunes]), cursorRune - start
	}
	contentCapacity := maxRunes - markerLength
	if cursorRune <= contentCapacity {
		return string(runes[:contentCapacity]) + saveDescriptionTailMarker, cursorRune
	}
	if len(runes)-cursorRune <= contentCapacity {
		start := len(runes) - contentCapacity

		return saveDescriptionTailMarker + string(runes[start:]), markerLength + cursorRune - start
	}
	if maxRunes <= markerLength*saveDescriptionCenterDivisor {
		start := min(max(cursorRune-contentCapacity/saveDescriptionCenterDivisor, 0), len(runes)-contentCapacity)

		return saveDescriptionTailMarker + string(runes[start:start+contentCapacity]), markerLength + cursorRune - start
	}

	contentCapacity = maxRunes - markerLength*saveDescriptionCenterDivisor
	start := cursorRune - contentCapacity/saveDescriptionCenterDivisor
	start = min(max(start, 0), len(runes)-contentCapacity)

	return saveDescriptionTailMarker + string(runes[start:start+contentCapacity]) + saveDescriptionTailMarker,
		markerLength + cursorRune - start
}

func orderedSaveDescriptionSelection(field saveDescriptionTextField) (int, int) {
	start, end := field.Selection()
	start = min(max(start, 0), len(field.Text()))
	end = min(max(end, 0), len(field.Text()))
	if start > end {
		return end, start
	}

	return start, end
}
