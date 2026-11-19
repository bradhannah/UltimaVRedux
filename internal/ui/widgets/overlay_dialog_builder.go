package widgets

import (
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// OverlayDialogBuilder provides a fluent API for constructing OverlayDialog instances
// with explicit configuration requirements. All appearance and duration settings must
// be explicitly specified - there are no hidden defaults.
//
// Required configuration (Build() panics if missing):
//   - Sizing: WithSizing() or use a template
//   - Position: WithPosition() or use a template
//   - Effects: WithEffects() or use a template
//   - ContentAlignment: WithContentAlignment() or use a template
//   - TextAlignment: WithTextAlignment() or use a template
//   - Duration: Exactly one of WithExternallyManagedDuration() or WithUserDismissed()
//
// Example usage:
//
//	// Concise with template
//	dialog := widgets.NewOverlayDialog("Success!").
//	    AsOverlayFeedback().
//	    WithExternallyManagedDuration(1000).
//	    Build()
//
//	// Fully explicit
//	dialog := widgets.NewOverlayDialog("Custom").
//	    WithSizing(FixedLinesSizing{Lines: 4, CharsPerLine: 50}).
//	    WithPosition(PositionScreenCenter).
//	    WithEffects(EffectsNone).
//	    WithContentAlignment(ContentAlignCentered).
//	    WithTextAlignment(TextAlignCenter).
//	    WithUserDismissed(keyboard).
//	    Build()
type OverlayDialogBuilder struct {
	// Message content
	message string

	// Required appearance configuration
	sizing       SizingStrategy
	position     Position
	effects      EffectsConfig
	contentAlign ContentAlignment
	textAlign    OverlayTextAlignment

	// Duration - exactly one must be set
	durationMs int             // Set by either timed duration method
	keyboard   *input.Keyboard // Set by WithUserDismissed()

	// Optional configuration
	mirrorToOutput *text.Output       // Optional: also write to output
	styledLines    []StyledDialogLine // Optional: coloured lines below the message
	dialogStack    *DialogStack       // Optional: auto-pop on dismiss
	onDismiss      func()             // Optional: callback on dismiss

	// Validation tracking - ensures all required config is set
	sizingSet           bool
	positionSet         bool
	effectsSet          bool
	contentAlignSet     bool
	textAlignSet        bool
	externalDurationSet bool // True if WithExternallyManagedDuration() called
	userDismissSet      bool // True if WithUserDismissed() called
}

// NewOverlayDialog creates a new OverlayDialogBuilder with the given message.
// All configuration must be explicitly set before calling Build().
func NewOverlayDialog(message string) *OverlayDialogBuilder {
	builder := new(OverlayDialogBuilder)
	builder.message = message

	return builder
}

// WithStyledLines adds lines below the message, each in its own colour. The
// popup is sized to show them.
func (b *OverlayDialogBuilder) WithStyledLines(lines ...StyledDialogLine) *OverlayDialogBuilder {
	b.styledLines = append(b.styledLines, lines...)

	return b
}

// =====================================
// REQUIRED: Appearance Configuration
// =====================================

// WithSizing sets the dialog sizing strategy.
// Required - Build() panics if not set (unless using a template).
func (b *OverlayDialogBuilder) WithSizing(s SizingStrategy) *OverlayDialogBuilder {
	b.sizing = s
	b.sizingSet = true

	return b
}

// WithPosition sets the dialog screen position.
// Required - Build() panics if not set (unless using a template).
func (b *OverlayDialogBuilder) WithPosition(p Position) *OverlayDialogBuilder {
	b.position = p
	b.positionSet = true

	return b
}

// WithEffects sets the fade animation configuration.
// Required - Build() panics if not set (unless using a template).
func (b *OverlayDialogBuilder) WithEffects(e EffectsConfig) *OverlayDialogBuilder {
	b.effects = e
	b.effectsSet = true

	return b
}

// WithContentAlignment sets how content is vertically aligned.
// Required - Build() panics if not set (unless using a template).
func (b *OverlayDialogBuilder) WithContentAlignment(a ContentAlignment) *OverlayDialogBuilder {
	b.contentAlign = a
	b.contentAlignSet = true

	return b
}

// WithTextAlignment sets how text is horizontally aligned.
// Required - Build() panics if not set (unless using a template).
func (b *OverlayDialogBuilder) WithTextAlignment(a OverlayTextAlignment) *OverlayDialogBuilder {
	b.textAlign = a
	b.textAlignSet = true

	return b
}

// =====================================
// REQUIRED: Duration (exactly one)
// =====================================

// WithExternallyManagedDuration configures a timed dialog whose owner controls expiry.
// OverlayDialog.Update will not advance or hide it. This is intended for owners with
// an authoritative clock that also remove the exact dialog instance from DialogStack.
func (b *OverlayDialogBuilder) WithExternallyManagedDuration(durationMs int) *OverlayDialogBuilder {
	b.durationMs = durationMs
	b.externalDurationSet = true

	return b
}

// WithUserDismissed sets user-dismissed behavior.
// The dialog shows the shared close footer and remains visible until Enter or Escape.
// Cannot be combined with either timed duration mode - Build() panics if combined.
func (b *OverlayDialogBuilder) WithUserDismissed(keyboard *input.Keyboard) *OverlayDialogBuilder {
	b.keyboard = keyboard
	b.userDismissSet = true

	return b
}

// =====================================
// OPTIONAL Configuration
// =====================================

// MirrorToOutput also writes the message to the provided text output.
// Optional - does not affect validation.
func (b *OverlayDialogBuilder) MirrorToOutput(output *text.Output) *OverlayDialogBuilder {
	b.mirrorToOutput = output

	return b
}

// AutoPopFromStack automatically pops the dialog from the stack when dismissed.
//
// IMPORTANT: This only works for user-dismissed dialogs (WithUserDismissed).
// For timed dialogs (WithExternallyManagedDuration), use explicit cleanup in Finally():
//
//	.Finally(func() { g.dialogStack.PopModalDialog() })
//
// How it works:
//   - When user presses any key, MessageDialog's InputDismissal triggers
//   - The dismiss callback calls dialogStack.PopModalDialog()
//   - This happens automatically via the callback chain
//
// When NOT to use:
//   - Timed dialogs (use Finally() for cleanup)
//   - When you need the dialog to remain for additional operations
//
// Optional - does not affect validation.
func (b *OverlayDialogBuilder) AutoPopFromStack(stack *DialogStack) *OverlayDialogBuilder {
	b.dialogStack = stack

	return b
}

// OnDismiss sets a callback to execute when the dialog is dismissed.
// Optional - does not affect validation.
func (b *OverlayDialogBuilder) OnDismiss(fn func()) *OverlayDialogBuilder {
	b.onDismiss = fn

	return b
}

// =====================================
// BUILD
// =====================================

// Build creates the OverlayDialog with all configured settings.
// Panics if required configuration is missing or invalid.
//
// Validation rules:
//   - All appearance settings must be set (sizing, position, effects, contentAlign, textAlign)
//   - Exactly one duration method must be called
//   - Timed and user-dismissed duration modes cannot be combined
func (b *OverlayDialogBuilder) Build() *OverlayDialog {
	b.validate()

	return b.build()
}

// validate checks that all required configuration is present and valid.
// Panics with a descriptive message if validation fails.
func (b *OverlayDialogBuilder) validate() {
	var missing []string

	// Check required appearance config
	if !b.sizingSet {
		missing = append(missing, "Sizing (call WithSizing() or use a template)")
	}

	if !b.positionSet {
		missing = append(missing, "Position (call WithPosition() or use a template)")
	}

	if !b.effectsSet {
		missing = append(missing, "Effects (call WithEffects() or use a template)")
	}

	if !b.contentAlignSet {
		missing = append(missing, "ContentAlignment (call WithContentAlignment() or use a template)")
	}

	if !b.textAlignSet {
		missing = append(missing, "TextAlignment (call WithTextAlignment() or use a template)")
	}

	// Check duration - exactly one required
	durationModeCount := 0
	for _, configured := range []bool{b.externalDurationSet, b.userDismissSet} {
		if configured {
			durationModeCount++
		}
	}

	if durationModeCount > 1 {
		panic("OverlayDialog.Build(): cannot combine duration modes - choose exactly one")
	}

	if durationModeCount == 0 {
		missing = append(missing,
			"Duration (call WithExternallyManagedDuration(ms) or WithUserDismissed(keyboard))")
	}

	if len(missing) > 0 {
		panic("OverlayDialog.Build(): missing required configuration:\n  - " + strings.Join(missing, "\n  - "))
	}
}

// build creates the OverlayDialog after validation passes.
func (b *OverlayDialogBuilder) build() *OverlayDialog {
	// Convert our config to MessageDialogBuilder settings
	msgBuilder := NewMessageDialog(b.message).WithStyledLines(b.styledLines...)

	// Configure sizing
	lines, charsPerLine, autoFit := b.sizing.GetConfig()
	if autoFit {
		// Use auto-sizing with min chars from config
		if autoSizing, ok := b.sizing.(AutoFitSizing); ok {
			msgBuilder = msgBuilder.WithAutoSize(autoSizing.MaxWidthPercent, AutoSizeMaxHeightPercent)
		} else {
			msgBuilder = msgBuilder.WithAutoSizeDefault()
		}
	} else {
		msgBuilder = msgBuilder.WithTextLines(lines).WithCharsPerLine(charsPerLine)
	}

	// Configure position
	msgBuilder = msgBuilder.WithHorizontalPosition(b.position.GetHorizontalMode())
	msgBuilder = msgBuilder.WithVerticalPosition(b.position.GetVerticalPercent())

	// Configure effects
	if b.effects.HasAnyEffect() {
		msgBuilder = msgBuilder.WithEffects(true).WithFadeDurations(b.effects.FadeInMs, b.effects.FadeOutMs)
	} else {
		msgBuilder = msgBuilder.WithEffects(false)
	}

	// Configure content alignment
	msgBuilder = msgBuilder.WithContentTextAlignment(b.contentAlign)

	// Configure text alignment
	msgBuilder = msgBuilder.WithTextHorizontalCenter(b.textAlign == TextAlignCenter)

	// Configure static content (overlay dialogs don't accept mirrored text)
	msgBuilder = msgBuilder.WithStaticContent()

	// Configure input dismissal if user-dismissed
	if b.userDismissSet {
		msgBuilder = msgBuilder.WithInputDismissal(b.keyboard)

		if b.dialogStack == nil && b.onDismiss != nil {
			msgBuilder = msgBuilder.WithDismissCallback(b.onDismiss)
		}
	}

	// Build the underlying MessageDialog
	messageDialog := msgBuilder.Build()

	// Create the OverlayDialog wrapper
	overlay := &OverlayDialog{
		dialog:                    messageDialog,
		message:                   b.message,
		durationMs:                b.durationMs,
		isTimed:                   b.externalDurationSet,
		durationExternallyManaged: b.externalDurationSet,
		mirrorToOutput:            b.mirrorToOutput,
		onDismiss:                 b.onDismiss,
	}
	if b.dialogStack != nil {
		messageDialog.onDismiss = func() {
			b.dialogStack.MustRemoveWidgetInstance(overlay)
			if b.onDismiss != nil {
				b.onDismiss()
			}
		}
	}

	return overlay
}

// =====================================
// GETTER METHODS (for testing)
// =====================================
