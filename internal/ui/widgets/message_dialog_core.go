package widgets

import (
	"image"
	"image/color"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/visual"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// MessageDialog provides a unified dialog system with optional advanced features.
// - Simple mode: Lightweight confirmation dialogs matching InputBox positioning
// - Advanced mode: Full FF-style dialogs with GPU-accelerated dissolve effects and dual-output
//
// Feature toggles allow disabling fancy effects for guard extortion confirmations while
// preserving sophisticated animations for camping/debug dialogs.
type MessageDialog struct {
	// Core fields (always present)
	frameBounds image.Rectangle
	visible     bool
	config      MessageDialogConfig
	font        *text.UltimaFont
	output      *text.Output // Primary text output

	baseConfig     MessageDialogConfig
	screenRelative bool
	popupViewport  image.Rectangle
	renderOutput   *text.Output
	renderPrompt   *text.Output

	// Input handling (only for simple modal dialogs)
	keyboard  *input.Keyboard
	dismissed bool   // Simple mode: tracks user dismissal
	onDismiss func() // Optional: callback on ESC/Enter dismissal

	// Feature toggles
	enableDissolveEffects bool // GPU-accelerated fade animations
	enableDualOutput      bool // Separate content + prompt outputs
	enableInputDismissal  bool // Allow ESC/Enter to close (simple mode)

	// Dissolve effect fields (only used if enableDissolveEffects = true)
	dissolveLevel     float64   // 0.0 = fully visible, 1.0 = fully dissolved
	fadeState         fadeState // Current animation state
	fadeDurationInMs  int       // Fade-in duration in milliseconds
	fadeDurationOutMs int       // Fade-out duration in milliseconds
	fadeStartTime     int64     // Timestamp when fade animation started
	animationClock    func() int64
	animationStarted  bool

	// Dual output fields (only used if enableDualOutput = true)
	promptOutput *text.Output
	promptColor  color.Color

	// Performance optimization fields
	contentBuffer            *ebiten.Image               // Cached rendering of border/background/text
	maskedBuffer             *ebiten.Image               // Persistent buffer for dissolve masking
	contentDirty             bool                        // Flag indicating content needs re-rendering
	dissolveEffect           *visual.PixelDissolveEffect // GPU-accelerated dissolve effect
	disableDirtyOptimization bool                        // Debug flag to bypass dirty checks
	lastScreenWidth          int
	lastScreenHeight         int

	// Transient feedback may measure a complete card once, then move that cached
	// composition without rebuilding it on every animation frame.
	completeCardLayout   bool
	presentationPosition struct{ X, Y float64 }
	presentationClip     image.Rectangle
	bakedBounds          image.Rectangle

	// Static content flag
	staticContent bool // If true, dialog does not accept mirrored text via addRowStr()
}

// MessageDialogConfig defines the size and position of the message dialog
type MessageDialogConfig struct {
	StartPercentX        float64          // 0.0-1.0 (left edge position)
	EndPercentX          float64          // 0.0-1.0 (right edge position)
	StartPercentY        float64          // 0.0-1.0 (top edge position)
	EndPercentY          float64          // 0.0-1.0 (bottom edge position)
	TextLines            int              // Total line capacity (includes prompt if separatePromptArea enabled)
	PromptLines          int              // Internal: Calculated split for prompt area (1 if separatePromptArea, else 0)
	CharsPerLine         int              // Maximum characters per line for this dialog width
	ContentAlignment     ContentAlignment // How content is vertically aligned (bottom-up or centered)
	TextHorizontalCenter bool             // If true, center text horizontally within box; if false, left-align
}

// ContentAlignment defines how content text is positioned vertically
type ContentAlignment int

// Content alignments for MessageDialog text; the prompt always stays at the bottom.
const (
	ContentAlignBottomUp ContentAlignment = iota // Terminal-style: newest at bottom, grows upward
	ContentAlignCenter                           // Content centered vertically in available space
	ContentAlignTop                              // Content aligned to top of available space
)

// DialogHorizontalPosition defines semantic horizontal positioning for dialogs
type DialogHorizontalPosition int

// Horizontal dialog positions; ToPercentage gives each centre as a screen fraction.
const (
	DialogHorizontalMapOnly     DialogHorizontalPosition = iota // Center within map area (excluding right panel) = 37%
	DialogHorizontalWholeScreen                                 // Center on entire screen = 50%
)

// Horizontal dialog centres as screen fractions.
const (
	dialogMapCenterPercent    = 0.37 // Center of map area (1.5% to 75% horizontal span)
	dialogScreenCenterPercent = 0.50 // Center of entire screen
)

// ToPercentage converts DialogHorizontalPosition to screen percentage (0.0-1.0)
func (pos DialogHorizontalPosition) ToPercentage() float64 {
	switch pos {
	case DialogHorizontalMapOnly:
		return dialogMapCenterPercent
	case DialogHorizontalWholeScreen:
		return dialogScreenCenterPercent
	default:
		return dialogScreenCenterPercent // Default to whole screen
	}
}

// MessageDialogBuilder provides a fluent API for constructing MessageDialog instances.
// This replaces the multiple New* constructors with a single, extensible builder pattern.
//
// Example usage:
//
//	// Simple confirmation dialog
//	:= NewMessageDialog("Pay 10 gold?").
//	    WithInputDismissal(keyboard).
//	    WithEffects(false).
//	    Build()
//
//	// Advanced dialog with effects (default Medium sizing: 10 lines, 45 chars)
//	dialog := NewMessageDialog("Camping...").Build()
type MessageDialogBuilder struct {
	// Required fields
	message string

	// Optional fields with defaults
	enableEffects        bool             // Default: true (dissolve animations)
	separatePromptArea   bool             // Default: true (content + prompt)
	enableInputDismissal bool             // Default: false (managed by Event Queue)
	keyboard             *input.Keyboard  // Only needed if enableInputDismissal = true
	onDismiss            func()           // Optional: callback on dismissal
	fadeInMs             int              // Default: 3000
	fadeOutMs            int              // Default: 1000
	contentAlignment     ContentAlignment // Default: ContentAlignBottomUp (terminal-style)

	// Explicit text capacity configuration (nil = use Medium preset defaults: 10 lines, 45 chars)
	textLines    *int // Total number of lines (includes prompt if separatePromptArea enabled)
	charsPerLine *int // Maximum characters per line

	// Center point positioning (nil = use Medium preset placement)
	centerX        *float64 // Center point X percentage (0.0-1.0)
	centerY        *float64 // Center point Y percentage (0.0-1.0)
	screenRelative bool     // Explicit screen/numeric anchors bypass the active map viewport.

	// Static content flag
	staticContent bool // If true, dialog does not accept mirrored text via addRowStr()

	// Text horizontal centering
	textHorizontalCenter bool // If true, center text horizontally within box; if false, left-align

	// Auto-sizing configuration
	autoSize         bool    // If true, measure message content and size dialog to fit
	maxWidthPercent  float64 // Maximum width as percentage of screen (0.0-1.0)
	maxHeightPercent float64 // Maximum height as percentage of screen (0.0-1.0)

	// styledLines follow the message in their own colours.
	styledLines []StyledDialogLine
}

// StyledDialogLine is one extra line shown below a dialog's message.
type StyledDialogLine struct {
	Text  string
	Color color.Color
}

// fadeState represents the current state of the dissolve animation
type fadeState int

const (
	fadeNone  fadeState = iota // No animation in progress
	fadingIn                   // Dissolving from hidden to visible
	fadingOut                  // Dissolving from visible to hidden
)

// Dialog configuration constants
const (
	// Message text uses the standard body size with extra line spacing for readability.
	messageFontPoint                   = float64(FontSizeBody)
	messageFontLineSpacing             = messageFontPoint + 6
	messageMaxTextLines                = 10 // Default total line capacity, including any prompt
	messageMaxCharsPerLine             = 45
	messagePercentTextIndentFromBorder = 0.02

	// Dissolve effect timing
	MessageDialogFadeInMs  = 3000 // Fade-in dissolve duration (3 second materialization)
	MessageDialogFadeOutMs = 1000 // Fade-out dissolve duration (1 second dematerialization)

	// messageDialogEffectSeed keeps every dissolve pattern identical between runs.
	messageDialogEffectSeed = 12345
)

// NewMessageDialog creates a new MessageDialogBuilder with sensible defaults.
// This is the preferred way to create MessageDialog instances going forward.
//
// Default configuration:
//   - Size: Medium preset (10 total lines including prompt, 45 chars, positioned at bottom-left)
//   - Effects: Enabled (3s fade-in, 1s fade-out)
//   - Dual output: Disabled (single content area only)
//   - Input dismissal: Disabled (managed by Event Queue)
//
// Example usage:
//
//	// Advanced dialog with defaults (Medium preset sizing)
//	dialog := NewMessageDialog("Camping...").Build()
//
//	// Explicit sizing (total line capacity)
//	dialog := NewMessageDialog("Brief message").
//	    WithTextLines(6).  // 6 total lines (includes prompt if dual mode)
//	    WithCharsPerLine(40).
//	    Build()
//
//	// Simple confirmation dialog
//	:= NewMessageDialog("Pay 10 gold?").
//	    WithInputDismissal(keyboard).
//	    WithEffects(false).
//	    Build()
func NewMessageDialog(message string) *MessageDialogBuilder {
	return &MessageDialogBuilder{
		message:              message,
		enableEffects:        true,
		separatePromptArea:   false,
		enableInputDismissal: false,
		keyboard:             nil,
		onDismiss:            nil,
		fadeInMs:             MessageDialogFadeInMs,
		fadeOutMs:            MessageDialogFadeOutMs,
		contentAlignment:     ContentAlignBottomUp, // Default: terminal-style bottom-up scrolling
		textLines:            nil,                  // Medium preset capacity
		charsPerLine:         nil,
		centerX:              nil, // Medium preset placement
		centerY:              nil,
		screenRelative:       false,
		staticContent:        false,
		textHorizontalCenter: false,
		autoSize:             false,
		maxWidthPercent:      0,
		maxHeightPercent:     0,
		styledLines:          nil,
	}
}

// WithEffects enables or disables GPU-accelerated dissolve effects.
// When enabled: 3-second fade-in, 1-second fade-out animations.
// When disabled: Instant show/hide (suitable for simple confirmations).
func (b *MessageDialogBuilder) WithEffects(enabled bool) *MessageDialogBuilder {
	b.enableEffects = enabled

	return b
}

// WithSeparatePromptArea enables a separate bottom prompt area.
// When enabled: Separate content (white) and prompt (yellow) text areas.
// Use SetPrompt() to update the bottom prompt area.
// Default: Single unified text area.
func (b *MessageDialogBuilder) WithSeparatePromptArea() *MessageDialogBuilder {
	b.separatePromptArea = true

	return b
}

// WithInputDismissal enables ESC/Enter key dismissal and the shared close footer.
// Requires a keyboard handler. Typically used for confirmation dialogs like guard extortion.
// Advanced dialogs managed by Event Queue should not use this.
func (b *MessageDialogBuilder) WithInputDismissal(keyboard *input.Keyboard) *MessageDialogBuilder {
	b.enableInputDismissal = true
	b.separatePromptArea = true
	b.keyboard = keyboard

	return b
}

// WithDismissCallback sets a callback to execute when dialog is dismissed via ESC/Enter.
// Provides maximum flexibility for custom cleanup logic.
// Only works with WithInputDismissal() - requires enableInputDismissal = true.
//
// Parameters:
//   - callback: Function to execute on dismissal
//
// Example usage:
//
//	dialog := widgets.NewMessageDialog("Custom").
//	    WithInputDismissal(keyboard).
//	    WithDismissCallback(func() {
//	        log.Println("Dialog dismissed")
//	        dialogStack.PopModalDialog()
//	        playSoundEffect(SoundClose)
//	    }).
//	    Build()
func (b *MessageDialogBuilder) WithDismissCallback(callback func()) *MessageDialogBuilder {
	b.onDismiss = callback

	return b
}

// WithFadeDurations customizes the fade animation timing.
// Only applies when effects are enabled via WithEffects(true).
func (b *MessageDialogBuilder) WithFadeDurations(fadeInMs, fadeOutMs int) *MessageDialogBuilder {
	b.fadeInMs = fadeInMs
	b.fadeOutMs = fadeOutMs

	return b
}

// WithContentTextAlignment sets how content text is vertically aligned.
// - ContentAlignBottomUp: Terminal-style scrolling (newest at bottom, grows upward) - DEFAULT
// - ContentAlignCenter: Content centered vertically in content area
// - ContentAlignTop: Content aligned to top of available space
// Note: Prompt is always bottom-aligned regardless of content alignment.
func (b *MessageDialogBuilder) WithContentTextAlignment(alignment ContentAlignment) *MessageDialogBuilder {
	b.contentAlignment = alignment

	return b
}

// WithTextLines explicitly sets the total number of lines for the dialog.
// This represents the total line capacity INCLUDING the prompt line if dual output mode is enabled.
//
// Examples:
//   - WithTextLines(10) with dual mode enabled = 9 content lines + 1 prompt line
//   - WithTextLines(10) with dual mode disabled = 10 content lines
//
// Use this for precise control over dialog total capacity.
func (b *MessageDialogBuilder) WithTextLines(lines int) *MessageDialogBuilder {
	b.textLines = &lines

	return b
}

// WithCharsPerLine explicitly sets the maximum characters per line.
// This overrides the CharsPerLine value from any size preset (Small/Medium/Large).
// Use this for precise control over dialog text width.
func (b *MessageDialogBuilder) WithCharsPerLine(chars int) *MessageDialogBuilder {
	b.charsPerLine = &chars

	return b
}

// WithHorizontalPosition sets the horizontal center position.
// When not specified, defaults to DialogHorizontalMapOnly (37% - map-centered).
//
// Parameters:
//   - xPos: Horizontal positioning (DialogHorizontalMapOnly or DialogHorizontalWholeScreen)
//
// Example: WithHorizontalPosition(DialogHorizontalWholeScreen) centers at 50% X
func (b *MessageDialogBuilder) WithHorizontalPosition(xPos DialogHorizontalPosition) *MessageDialogBuilder {
	x := xPos.ToPercentage()
	b.centerX = &x
	b.screenRelative = xPos != DialogHorizontalMapOnly

	return b
}

// WithVerticalPosition sets the vertical center position.
// When not specified, uses Medium preset default (varies by dialog configuration).
//
// Parameters:
//   - y: Vertical center position percentage (0.0-1.0)
//
// Example: WithVerticalPosition(0.82) centers at 82% Y
func (b *MessageDialogBuilder) WithVerticalPosition(y float64) *MessageDialogBuilder {
	b.centerY = &y

	return b
}

// WithStaticContent marks the dialog as static - it will not receive mirrored text
// from game output. Use this for popups that should only show their initial message.
// Static content dialogs display their initial message and ignore subsequent AddRowStr() calls.
func (b *MessageDialogBuilder) WithStaticContent() *MessageDialogBuilder {
	b.staticContent = true

	return b
}

// WithStyledLines adds lines below the message, each in its own colour. They
// count toward auto-sizing, so the dialog is built large enough to show them.
func (b *MessageDialogBuilder) WithStyledLines(lines ...StyledDialogLine) *MessageDialogBuilder {
	b.styledLines = append(b.styledLines, lines...)

	return b
}

// measuredMessage is the message plus its styled lines, for auto-sizing.
func (b *MessageDialogBuilder) measuredMessage() string {
	lines := make([]string, 0, len(b.styledLines)+1)
	lines = append(lines, b.message)
	for _, line := range b.styledLines {
		lines = append(lines, line.Text)
	}

	return strings.Join(lines, "\n")
}

// WithTextHorizontalCenter sets whether text should be centered horizontally within the dialog box.
// When enabled: Text is horizontally centered (useful for area transitions like "Entering Britain").
// When disabled: Text is left-aligned (default behavior, suitable for general messages).
func (b *MessageDialogBuilder) WithTextHorizontalCenter(centered bool) *MessageDialogBuilder {
	b.textHorizontalCenter = centered

	return b
}

// WithAutoSize enables automatic dialog sizing based on message content.
// The dialog will measure the text and expand to fit, respecting the specified maximum constraints.
// This prevents text truncation by dynamically calculating the required dimensions.
//
// Parameters:
//   - maxWidthPercent: Maximum dialog width as percentage of screen (0.0-1.0), typically 0.80
//   - maxHeightPercent: Maximum dialog height as percentage of screen (0.0-1.0), typically 0.60
//
// Example usage:
//
//	// Custom constraints
//	dialog := NewMessageDialog("Long message...").
//	    WithAutoSize(0.70, 0.50).  // Max 70% width, 50% height
//	    Build()
func (b *MessageDialogBuilder) WithAutoSize(maxWidthPercent, maxHeightPercent float64) *MessageDialogBuilder {
	b.autoSize = true
	b.maxWidthPercent = maxWidthPercent
	b.maxHeightPercent = maxHeightPercent

	return b
}

// WithAutoSizeDefault enables automatic dialog sizing with default constraints.
// Uses AutoSizeMaxWidthPercent (80%) and AutoSizeMaxHeightPercent (60%) from dialog_constants.go.
// This is the recommended method for most auto-sizing use cases.
//
// Example usage:
//
//	// Death sequence messages, karma messages, etc.
//	dialog := NewMessageDialog("Thy Karma hast decreased!").
//	    WithAutoSizeDefault().
//	    Build()
func (b *MessageDialogBuilder) WithAutoSizeDefault() *MessageDialogBuilder {
	return b.WithAutoSize(AutoSizeMaxWidthPercent, AutoSizeMaxHeightPercent)
}

// calculateAutoSizeDimensions computes dialog dimensions from content requirements.
// Returns percentage-based placement that exactly fits the specified content capacity.
//
// This method now uses the shared CalculateDialogAutoSizeDimensions() function from
// dialog_sizing.go to avoid code duplication with InputBox.
//
// Parameters:
//   - centerX, centerY: Center point percentages (0.0-1.0)
//   - textLines: Total number of lines (including prompt if present)
//   - charsPerLine: Maximum characters per line
//   - promptLines: Calculated internally based on separatePromptArea
func (b *MessageDialogBuilder) calculateAutoSizeDimensions(
	centerX, centerY float64,
	textLines, charsPerLine int,
) MessageDialogConfig {
	if b.enableInputDismissal {
		// The automatic footer must fit even when the message or explicit capacity is tiny.
		// One spare column covers key brackets, which are wider than an average column.
		charsPerLine = max(charsPerLine, len(DialogCloseHint)+1)
		textLines = max(textLines, DialogConfirmationLines)
	}
	// Calculate prompt split if dual output mode enabled
	var promptLines int
	if b.separatePromptArea {
		promptLines = 1 // Reserve 1 line for prompt
	} else {
		promptLines = 0
	}

	// Use shared auto-sizing function
	placement := CalculateDialogAutoSizeDimensions(NewDialogAutoSizingParams(
		centerX,
		centerY,
		textLines,
		charsPerLine,
		promptLines, // Still pass to sizing function for layout calculation
		messageFontPoint,
		messageFontLineSpacing,
	))

	return MessageDialogConfig{
		StartPercentX:        placement.StartPercentX,
		EndPercentX:          placement.EndPercentX,
		StartPercentY:        placement.StartPercentY,
		EndPercentY:          placement.EndPercentY,
		TextLines:            textLines,
		PromptLines:          promptLines,
		CharsPerLine:         charsPerLine,
		ContentAlignment:     b.contentAlignment,
		TextHorizontalCenter: b.textHorizontalCenter,
	}
}

// buildAutoSizedConfig measures message content and creates a config that fits the text.
// This prevents text truncation by dynamically calculating dialog dimensions.
func (b *MessageDialogBuilder) buildAutoSizedConfig() MessageDialogConfig {
	dm := display.GetManager()
	screenWidth, screenHeight := dm.GetCurrentSize()

	// Create scaled font for measurement (must match rendering font for accurate sizing)
	scaledFontSize := text.GetScaledNumberToResolution(dm, messageFontPoint)
	scaledLineSpacing := text.GetScaledNumberToResolution(dm, messageFontLineSpacing)
	tempFont := text.NewUltimaFont(scaledFontSize)

	// Calculate maximum available dimensions in pixels
	maxWidthPx := float64(screenWidth) * b.maxWidthPercent
	maxHeightPx := float64(screenHeight) * b.maxHeightPercent

	// Account for dialog padding (borders, text margins)
	paddingWidthPx := float64(screenWidth) * AutoSizeHorizontalPadding
	paddingHeightPx := float64(screenHeight) * AutoSizeVerticalPadding
	usableWidthPx := maxWidthPx - paddingWidthPx
	usableHeightPx := maxHeightPx - paddingHeightPx

	// Estimate max characters per line based on available width and average character width
	maxCharsFromWidth := tempFont.CharsForWidth(int(usableWidthPx))

	// Apply min/max constraints to chars per line
	charsPerLine := maxCharsFromWidth
	if charsPerLine < AutoSizeMinCharsPerLine {
		charsPerLine = AutoSizeMinCharsPerLine
	}
	if charsPerLine > messageMaxCharsPerLine {
		charsPerLine = messageMaxCharsPerLine
	}

	// Measure text with the calculated chars per line for wrapping
	measurement := text.MeasureText(b.measuredMessage(), tempFont, scaledLineSpacing, charsPerLine)

	// Shrink dialog width to fit content while respecting minimum aesthetic width.
	targetChars := measurement.MaxLineChars
	if targetChars < AutoSizeMinCharsPerLine {
		targetChars = AutoSizeMinCharsPerLine
	}
	if targetChars < charsPerLine {
		charsPerLine = targetChars
	}

	// Calculate required text lines based on measurement
	textLines := measurement.Lines
	if textLines < AutoSizeMinLines {
		textLines = AutoSizeMinLines
	}
	if b.separatePromptArea {
		textLines++
	}

	// Calculate max lines based on available height
	maxLinesFromHeight := int(usableHeightPx / scaledLineSpacing)
	if textLines > maxLinesFromHeight && maxLinesFromHeight > 0 {
		textLines = maxLinesFromHeight
	}

	// Use center position if specified, otherwise use dialog defaults
	centerX := dialogMapCenterPercent // Default: map area center
	centerY := 0.70                   // Default: lower portion of screen (DialogDefaultVerticalPos)

	if b.centerX != nil {
		centerX = *b.centerX
	}
	if b.centerY != nil {
		centerY = *b.centerY
	}

	// Generate the config using existing sizing infrastructure
	config := b.calculateAutoSizeDimensions(centerX, centerY, textLines, charsPerLine)

	return config
}

// Build constructs the final MessageDialog instance.
// This method applies all configured options and returns a ready-to-use dialog.
func (b *MessageDialogBuilder) Build() *MessageDialog {
	// Validate required dependencies for optional features
	if b.enableInputDismissal && b.keyboard == nil {
		panic("WithInputDismissal() requires keyboard - pass keyboard via WithInputDismissal(keyboard)")
	}

	dialog := b.newDialog(b.resolveConfig())
	dialog.initializeVisualElements()

	// Add initial message if provided
	if b.message != "" {
		dialog.output.AddText(b.message)
	}
	for _, line := range b.styledLines {
		dialog.output.AddTextWithColor(line.Text, line.Color)
	}
	if b.enableInputDismissal {
		dialog.SetAcknowledgementPrompt(DialogCloseHint)
	}
	dialog.contentDirty = true
	width, height := display.GetManager().GetCurrentSize()
	dialog.resolvePopupLayout(image.Rect(0, 0, width, height))

	return dialog
}

// resolveConfig sizes the dialog from measured message content when auto-sizing
// is enabled, otherwise from the centre point and text capacity.
func (b *MessageDialogBuilder) resolveConfig() MessageDialogConfig {
	if b.autoSize && b.message != "" {
		return b.buildAutoSizedConfig()
	}

	// Use explicit values if set, otherwise fall back to defaults
	const defaultCenterX = 0.35 // Slightly left of center (map-area centered)
	const defaultCenterY = 0.78 // Bottom third of screen

	centerX := defaultCenterX
	centerY := defaultCenterY

	if b.centerX != nil {
		centerX = *b.centerX
	}
	if b.centerY != nil {
		centerY = *b.centerY
	}

	// Use default text capacity, with explicit overrides if provided
	textLines := messageMaxTextLines       // 10 - total lines including prompt
	charsPerLine := messageMaxCharsPerLine // 45

	if b.textLines != nil {
		textLines = *b.textLines
	}
	if b.charsPerLine != nil {
		charsPerLine = *b.charsPerLine
	}

	return b.calculateAutoSizeDimensions(centerX, centerY, textLines, charsPerLine)
}

// newDialog creates the hidden dialog state; Build prepares its visuals and text.
func (b *MessageDialogBuilder) newDialog(config MessageDialogConfig) *MessageDialog {
	return &MessageDialog{
		frameBounds:              image.Rect(0, 0, 0, 0), // Set by initializeVisualElements
		visible:                  false,
		config:                   config,
		font:                     nil,
		output:                   nil,
		baseConfig:               config,
		screenRelative:           b.screenRelative,
		popupViewport:            image.Rect(0, 0, 0, 0),
		renderOutput:             nil,
		renderPrompt:             nil,
		keyboard:                 b.keyboard,
		dismissed:                false,
		onDismiss:                b.onDismiss,
		enableDissolveEffects:    b.enableEffects,
		enableDualOutput:         b.separatePromptArea,
		enableInputDismissal:     b.enableInputDismissal,
		dissolveLevel:            0.0,
		fadeState:                fadeNone,
		fadeDurationInMs:         b.fadeInMs,
		fadeDurationOutMs:        b.fadeOutMs,
		fadeStartTime:            0,
		animationClock:           func() int64 { return time.Now().UnixMilli() },
		animationStarted:         false,
		promptOutput:             nil,
		promptColor:              pkgcolor.EGALightBlue,
		contentBuffer:            nil,
		maskedBuffer:             nil,
		contentDirty:             false,
		dissolveEffect:           nil,
		disableDirtyOptimization: false,
		lastScreenWidth:          0,
		lastScreenHeight:         0,
		completeCardLayout:       false,
		presentationPosition:     struct{ X, Y float64 }{X: 0, Y: 0},
		presentationClip:         image.Rect(0, 0, 0, 0),
		bakedBounds:              image.Rect(0, 0, 0, 0),
		staticContent:            b.staticContent,
	}
}

// initializeVisualElements prepares geometry, fonts, and output without textures.
// The classic frame is drawn directly into contentBuffer; a legacy Border would
// allocate unused background/frame images and retain every size in its cache.
func (md *MessageDialog) initializeVisualElements() {
	md.updateFrameBounds()
	md.lastScreenWidth, md.lastScreenHeight = display.GetManager().GetCurrentSize()

	// Initialize font with display manager for responsive scaling
	md.font = text.NewUltimaFont(md.getScaledFontSize())

	// Initialize main content output
	md.initializeTextOutput(&md.output, md.config.TextLines, pkgcolor.EGAWhite)

	// Initialize prompt output only if dual output enabled
	if md.enableDualOutput {
		md.initializeTextOutput(&md.promptOutput, md.config.PromptLines, pkgcolor.EGAYellow)
	}
}

func (md *MessageDialog) updateFrameBounds() {
	md.frameBounds = *sprites.GetRectangleFromPercents(sprites.PercentBasedPlacement{
		StartPercentX: md.config.StartPercentX,
		EndPercentX:   md.config.EndPercentX,
		StartPercentY: md.config.StartPercentY,
		EndPercentY:   md.config.EndPercentY,
	})
}

// createDissolveEffect creates a new dissolve effect with the specified mode.
// Each effect draws from a fresh fixed-seed RNG, so the pattern never varies.
func (md *MessageDialog) createDissolveEffect(
	screenWidth, screenHeight int, mode visual.DissolveMode,
) *visual.PixelDissolveEffect {
	maskWidth, maskHeight := visual.CalculateMaskSizeForDialog(screenWidth, screenHeight)

	return visual.NewPixelDissolveEffect(
		screenWidth, screenHeight,
		maskWidth, maskHeight,
		mode,
		beradrng.New(messageDialogEffectSeed),
	)
}

// getContentVerticalAlignment returns the etext.Align value based on ContentAlignment configuration
func (md *MessageDialog) getContentVerticalAlignment() etext.Align {
	switch md.config.ContentAlignment {
	case ContentAlignBottomUp:
		return etext.AlignEnd // Bottom-up scrolling (text grows upward)
	case ContentAlignTop:
		return etext.AlignStart // Top-aligned content
	case ContentAlignCenter:
	}

	return etext.AlignCenter // Vertical centering, also used for unknown values
}

// getTextHorizontalAlignment returns the etext.Align value for horizontal text alignment
func (md *MessageDialog) getTextHorizontalAlignment() etext.Align {
	if md.config.TextHorizontalCenter {
		return etext.AlignCenter // Center text horizontally
	}

	return etext.AlignStart // Left-aligned (default)
}

// calculateTextAreaBounds calculates the horizontal text area bounds from percentage-based config
// Returns the X position (in pixels) and width (in pixels) of the text rendering area
func (md *MessageDialog) calculateTextAreaBounds() (int, int) {
	textStartX := md.config.StartPercentX + messagePercentTextIndentFromBorder
	textEndX := md.config.EndPercentX - messagePercentTextIndentFromBorder // Symmetric right indent
	textRect := sprites.GetRectangleFromPercents(sprites.PercentBasedPlacement{
		StartPercentX: textStartX,
		EndPercentX:   textEndX,
		StartPercentY: md.config.StartPercentY,
		EndPercentY:   md.config.EndPercentY,
	})

	return textRect.Min.X, textRect.Max.X - textRect.Min.X
}

// getScaledLineSpacing returns the resolution-scaled line spacing value
// This centralizes the display manager scaling calculation used throughout the dialog
func (md *MessageDialog) getScaledLineSpacing() float64 {
	dm := display.GetManager()

	return text.GetScaledNumberToResolution(dm, messageFontLineSpacing)
}

// hasPromptOutput returns true if dual output mode is enabled and prompt output is initialized
// This guard check is used before accessing prompt-specific functionality
func (md *MessageDialog) hasPromptOutput() bool {
	return md.enableDualOutput && md.promptOutput != nil
}

// initializeTextOutput creates or reinitializes a text.Output with current font and scaling
// Parameters:
//   - output: the text.Output pointer to initialize (will be created if nil)
//   - lines: maximum number of lines for this output
//   - defaultColor: color to set on newly created output (ignored if output already exists)
func (md *MessageDialog) initializeTextOutput(output **text.Output, lines int, defaultColor color.Color) {
	lineSpacing := md.getScaledLineSpacing()

	if *output == nil {
		*output = text.NewOutput(
			md.font,
			lineSpacing,
			lines,
			md.config.CharsPerLine,
		)
		(*output).SetColor(defaultColor)
	} else {
		(*output).SetFont(md.font, lineSpacing)
	}
}

// getScaledFontSize returns the resolution-scaled font point size
// Provides API symmetry with getScaledLineSpacing() for consistent scaling interface
func (md *MessageDialog) getScaledFontSize() float64 {
	dm := display.GetManager()

	return text.GetScaledNumberToResolution(dm, messageFontPoint)
}

// needsBufferRecreation checks if the content buffer needs to be recreated
// Returns true if buffer is missing or screen dimensions have changed
func (md *MessageDialog) needsBufferRecreation(screenWidth, screenHeight int) bool {
	return md.contentBuffer == nil ||
		md.contentBuffer.Bounds().Dx() != screenWidth ||
		md.contentBuffer.Bounds().Dy() != screenHeight
}

// dualOutputLayout holds calculated positions for dual output mode rendering
type dualOutputLayout struct {
	PromptHeight  float64 // Height of the prompt area in pixels
	PromptStartY  float64 // Y coordinate where prompt rendering begins
	ContentStartY float64 // Y coordinate where content rendering begins
	ContentHeight int     // Available height for content area in pixels
}

// calculateDualOutputLayout computes the vertical positioning for dual output mode
// Calculates prompt position (bottom-aligned) and content area (fills remaining space)
func (md *MessageDialog) calculateDualOutputLayout(
	borderBounds image.Rectangle,
	topMargin, bottomMargin, scaledLineSpacing float64,
) dualOutputLayout {
	// CRITICAL: Calculate prompt position from BOTTOM (always bottom-aligned)
	promptHeight := float64(md.config.PromptLines) * scaledLineSpacing
	promptStartY := float64(borderBounds.Max.Y) - bottomMargin - promptHeight

	// Content area: from top to prompt start
	contentStartY := float64(borderBounds.Min.Y) + topMargin
	contentHeight := int(promptStartY - contentStartY)

	return dualOutputLayout{
		PromptHeight:  promptHeight,
		PromptStartY:  promptStartY,
		ContentStartY: contentStartY,
		ContentHeight: contentHeight,
	}
}
