package widgets

import (
	"image"
	stdcolor "image/color"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	tavernResultStartPercentX    = 0.27
	tavernResultEndPercentX      = 0.73
	tavernResultStartPercentY    = 0.22
	tavernResultEndPercentY      = 0.78
	tavernResultHeaderRatio      = 0.18
	tavernResultFooterRatio      = 0.18
	tavernResultMinimumInset     = 5
	tavernResultLineSpacing      = 1.08
	tavernResultMaximumBodyLines = 12
	tavernResultMinimumTextChars = 8
	tavernResultFooter           = "[Enter/Esc] Continue"
)

var _ Widget = (*TavernResultDialog)(nil)

// TavernResultTone controls the semantic emphasis of one finalized result.
type TavernResultTone byte

const (
	// TavernResultSuccess presents a fully successful purchase.
	TavernResultSuccess TavernResultTone = iota
	// TavernResultCaution presents partial or saturated fulfillment.
	TavernResultCaution
	// TavernResultFailure presents refusal or ejection.
	TavernResultFailure
)

// TavernResultView is finalized meal or provision presentation supplied by workflow.
type TavernResultView struct {
	Title       string
	Item        string
	Detail      string
	ChargeLabel string
	Charge      int
	ShowFood    bool
	FoodBefore  uint16
	FoodAfter   uint16
	ShowGold    bool
	GoldBefore  uint16
	GoldAfter   uint16
	Theft       uint16
	Note        string
	Tone        TavernResultTone
}

// TavernResultDialogConfig configures one acknowledged Tavern result.
type TavernResultDialogConfig struct {
	Keyboard     *input.Keyboard
	View         TavernResultView
	IsKeyPressed func(ebiten.Key) bool
}

// TavernResultDialog displays a coloured finalized purchase result.
type TavernResultDialog struct {
	keyboard     *input.Keyboard
	isKeyPressed func(ebiten.Key) bool
	view         TavernResultView
	visible      bool
	dismissed    bool
	keysReady    bool

	frame        *merchantModalFrame
	headerOutput *text.Output
	bodyOutput   *text.Output
	footerOutput *text.Output
}

// NewTavernResultDialog creates a visible finalized-result modal.
func NewTavernResultDialog(config TavernResultDialogConfig) *TavernResultDialog {
	if config.Keyboard == nil {
		panic("TavernResultDialog: keyboard is required")
	}
	validateTavernResultView(config.View)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = ebiten.IsKeyPressed
	}
	dialog := &TavernResultDialog{ //nolint:exhaustruct_v5 // Resolution-dependent presentation initializes on demand.
		keyboard: config.Keyboard, isKeyPressed: isKeyPressed, view: config.View,
		visible: true, keysReady: tavernResultControlsReleased(isKeyPressed),
		frame: newMerchantModalFrame(merchantModalFrameConfig{
			placement:             tavernResultPlacement(),
			headerRatio:           tavernResultHeaderRatio,
			footerRatio:           tavernResultFooterRatio,
			minimumInset:          tavernResultMinimumInset,
			lineSpacingMultiplier: tavernResultLineSpacing,
			footerMinimumLines:    1,
		}),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.ensureLayout()

	return dialog
}

// Draw renders the opaque result modal.
func (d *TavernResultDialog) Draw(screen *ebiten.Image) {
	if screen == nil || !d.IsVisible() {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
}

// Update waits for explicit result acknowledgement.
func (d *TavernResultDialog) Update() {
	if !d.IsVisible() {
		return
	}
	if !d.keysReady {
		if !tavernResultControlsReleased(d.isKeyPressed) {
			return
		}
		d.keysReady = true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()
	}
}

// GetBounds returns the responsive modal bounds.
func (d *TavernResultDialog) GetBounds() image.Rectangle {
	d.ensureLayout()

	return d.frame.bounds()
}

// IsDismissed reports whether workflow may remove the modal.
func (d *TavernResultDialog) IsDismissed() bool { return d.dismissed }

// IsVisible reports whether the result currently owns presentation.
func (d *TavernResultDialog) IsVisible() bool { return d.visible && !d.dismissed }

// Dismiss closes the result.
func (d *TavernResultDialog) Dismiss() {
	if d.dismissed {
		return
	}
	d.keyboard.PrepareForTransition(0)
	d.visible = false
	d.dismissed = true
}

func (d *TavernResultDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	d.headerOutput, d.bodyOutput, d.footerOutput = d.frame.newOutputs(merchantModalTextBudget{
		lineSpacing: tavernResultLineSpacing, maximumBodyLines: tavernResultMaximumBodyLines,
		footerLines: 1, minimumChars: tavernResultMinimumTextChars, fixedBody: false,
	})
	d.populateOutputs()
}

func (d *TavernResultDialog) populateOutputs() {
	if d.headerOutput == nil || d.bodyOutput == nil || d.footerOutput == nil {
		return
	}
	d.headerOutput.Reset()
	d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text(strings.ToUpper(d.view.Title), tavernResultToneColor(d.view.Tone)).Done()
	d.bodyOutput.Reset()
	d.populateBodyOutput()
	d.footerOutput.Reset()
	appendControlGuidance(d.footerOutput, tavernResultFooter, d.frame.footerText.Dx())
}

func (d *TavernResultDialog) populateBodyOutput() {
	d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).Yellow(d.view.Item).Done()
	if d.view.Detail != "" {
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).LightGray(d.view.Detail).Done()
	}
	d.bodyOutput.AppendLine().White("").Done()
	d.appendFoodOutput()
	d.appendChargeOutput()
	d.appendGoldOutput()
	d.appendClosingOutput()
}

func (d *TavernResultDialog) appendFoodOutput() {
	if d.view.ShowFood {
		line := d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			LightGray("FOOD  ").Cyanf("%d", d.view.FoodBefore).White("  ->  ").
			Textf("%d", tavernResultAfterColor(d.view), d.view.FoodAfter)
		if d.view.FoodAfter > d.view.FoodBefore {
			line.LightGreenf("  (+%d)", d.view.FoodAfter-d.view.FoodBefore)
		}
		line.Done()
	}
}

func (d *TavernResultDialog) appendChargeOutput() {
	if d.view.Charge > 0 {
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			LightGray(strings.ToUpper(d.view.ChargeLabel)+"  ").Yellowf("%d GOLD", d.view.Charge).Done()
	}
}

func (d *TavernResultDialog) appendGoldOutput() {
	if d.view.ShowGold {
		line := d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			LightGray("GOLD  ").Cyanf("%d", d.view.GoldBefore).White("  ->  ").
			Textf("%d", tavernResultAfterColor(d.view), d.view.GoldAfter)
		if d.view.GoldBefore > d.view.GoldAfter {
			line.Textf("  (-%d)", DialogLabelColor, d.view.GoldBefore-d.view.GoldAfter)
		}
		line.Done()
	}
}

func (d *TavernResultDialog) appendClosingOutput() {
	if d.view.Theft > 0 {
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Textf("Faulinei took %d additional gold.", DialogStatusWarningColor, d.view.Theft).Done()
	}
	if d.view.Note != "" {
		d.bodyOutput.AppendLine().White("").Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text(d.view.Note, tavernResultToneColor(d.view.Tone)).WordWrap().Done()
	}
}

func tavernResultToneColor(tone TavernResultTone) stdcolor.Color {
	switch tone {
	case TavernResultSuccess:
		return DialogDeltaPositive
	case TavernResultCaution:
		return DialogLabelColor
	case TavernResultFailure:
		return DialogStatusWarningColor
	default:
		panic("TavernResultDialog: unknown result tone")
	}
}

func tavernResultAfterColor(view TavernResultView) stdcolor.Color {
	if view.Tone == TavernResultFailure {
		return DialogValueColor
	}

	return DialogDeltaPositive
}

func tavernResultPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: tavernResultStartPercentX,
		EndPercentX:   tavernResultEndPercentX,
		StartPercentY: tavernResultStartPercentY,
		EndPercentY:   tavernResultEndPercentY,
	}
}

func validateTavernResultView(view TavernResultView) {
	if strings.TrimSpace(view.Title) == "" || strings.TrimSpace(view.Item) == "" {
		panic("TavernResultDialog: title and item are required")
	}
	_ = tavernResultToneColor(view.Tone)
}

func tavernResultControlsReleased(isKeyPressed func(ebiten.Key) bool) bool {
	return !isKeyPressed(ebiten.KeyEnter) &&
		!isKeyPressed(ebiten.KeyNumpadEnter) &&
		!isKeyPressed(ebiten.KeyEscape)
}
