package widgets

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const tavernResultFooter = "[Enter/Esc] Continue"

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
	window       *amountWindow
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
	dialog := &TavernResultDialog{
		keyboard: config.Keyboard, isKeyPressed: isKeyPressed, view: config.View,
		visible: true, dismissed: false, keysReady: tavernResultControlsReleased(isKeyPressed),
		window: newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque result modal.
func (d *TavernResultDialog) Draw(screen *ebiten.Image) {
	if screen == nil || !d.IsVisible() {
		return
	}
	d.window.draw(screen)
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
	return d.window.bounds()
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

// step describes the receipt: what happened, what it did to the party's food and
// gold, what Faulinei took, and the keeper's word. It is only acknowledged.
func (d *TavernResultDialog) step() amountStep {
	view := d.view
	step := emptyAmountStep()
	step.title, step.unit, step.footer = view.Title, view.Item, tavernResultFooter
	if view.ShowFood {
		step.rows = append(step.rows, amountRow{label: "Food", have: int(view.FoodBefore), then: int(view.FoodAfter)})
	}
	if view.ShowGold {
		step.rows = append(step.rows, amountRow{label: amountGoldRow, have: int(view.GoldBefore), then: int(view.GoldAfter)})
	}
	if view.Detail != "" {
		step.notes = append(step.notes, amountNote{text: view.Detail, tone: amountPlain})
	}
	paid := int(view.GoldBefore) - int(view.GoldAfter)
	if view.Charge > 0 && (!view.ShowGold || view.Charge != paid) {
		step.notes = append(step.notes, amountNote{
			text: fmt.Sprintf("%s: %d gold.", view.ChargeLabel, view.Charge), tone: amountPlain,
		})
	}
	if view.Theft > 0 {
		step.notes = append(step.notes, amountNote{
			text: fmt.Sprintf("Faulinei took %d more gold.", view.Theft), tone: amountWarning,
		})
	}
	if view.Note != "" {
		if len(step.notes) > 0 {
			step.notes = append(step.notes, amountNote{text: "", tone: amountPlain})
		}
		step.notes = append(step.notes, amountNote{text: view.Note, tone: tavernResultNoteTone(view.Tone)})
	}

	return step
}

// tavernResultNoteTone draws the keeper's word grey, or in the readable red when
// the order was refused or ended.
func tavernResultNoteTone(tone TavernResultTone) amountTone {
	switch tone {
	case TavernResultSuccess, TavernResultCaution:
		return amountPlain
	case TavernResultFailure:
		return amountWarning
	default:
		panic("TavernResultDialog: unknown result tone")
	}
}

func validateTavernResultView(view TavernResultView) {
	if strings.TrimSpace(view.Title) == "" || strings.TrimSpace(view.Item) == "" {
		panic("TavernResultDialog: title and item are required")
	}
	_ = tavernResultNoteTone(view.Tone)
}

func tavernResultControlsReleased(isKeyPressed func(ebiten.Key) bool) bool {
	return !isKeyPressed(ebiten.KeyEnter) &&
		!isKeyPressed(ebiten.KeyNumpadEnter) &&
		!isKeyPressed(ebiten.KeyEscape)
}
