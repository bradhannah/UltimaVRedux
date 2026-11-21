package widgets

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const horseEndsTalk = "Buying anyway ends the talk."

var _ Widget = (*HorsePurchaseDialog)(nil)

// HorsePurchaseDialogConfig configures one nested horse approval modal.
type HorsePurchaseDialogConfig struct {
	Keyboard *input.Keyboard
	Intent   HorsePurchaseIntent
}

// HorsePurchaseDialog presents the horse price and party-gold consequence before authority.
type HorsePurchaseDialog struct {
	keyboard  *input.Keyboard
	intent    HorsePurchaseIntent
	dismissed bool
	cancelled bool
	pending   *game_state.HorsePurchaseRequest

	window *amountWindow
}

// NewHorsePurchaseDialog creates a centered keyboard-only approval modal.
func NewHorsePurchaseDialog(config HorsePurchaseDialogConfig) *HorsePurchaseDialog {
	if config.Keyboard == nil {
		panic("HorsePurchaseDialog: keyboard is required")
	}
	validateHorsePurchaseIntent(config.Intent)
	dialog := &HorsePurchaseDialog{
		keyboard:  config.Keyboard,
		intent:    config.Intent,
		dismissed: false,
		cancelled: false,
		pending:   nil,
		window:    newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque horse approval window.
func (d *HorsePurchaseDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update confirms or cancels the horse approval after transition-key release.
func (d *HorsePurchaseDialog) Update() {
	if d.dismissed {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancelled = true
		d.dismissed = true

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		request := d.intent.Request
		d.pending = &request
		d.dismissed = true
	}
}

// ConsumePendingPurchaseRequest returns and clears the one confirmed authority request.
func (d *HorsePurchaseDialog) ConsumePendingPurchaseRequest() (game_state.HorsePurchaseRequest, bool) {
	if d.pending == nil {
		return emptyHorsePurchaseRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetBounds returns the centered modal bounds.
func (d *HorsePurchaseDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape closed the modal without confirmation.
func (d *HorsePurchaseDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the modal may be removed.
func (d *HorsePurchaseDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the modal without emitting a request.
func (d *HorsePurchaseDialog) Dismiss() { d.dismissed = true }

// step describes the window: the price is the change in gold. A party that
// cannot pay still sends the request, which the stable keeper refuses and ends
// the talk with, so Enter stays and says what it will do.
func (d *HorsePurchaseDialog) step() amountStep {
	quote := d.intent.Quote
	step := emptyAmountStep()
	step.preview, step.title, step.verb = d.intent.Preview, amountBuyVerb+" Horse", amountBuyVerb
	if quote.Affordable {
		step.rows = []amountRow{{label: amountGoldRow, have: int(quote.GoldBefore), then: int(quote.GoldAfterPayment)}}

		return step
	}
	step.verb += tavernAnyway
	step.notes = shortOfGoldNotes("It costs", quote.Price, int(quote.GoldBefore), horseEndsTalk)

	return step
}

func validateHorsePurchaseIntent(intent HorsePurchaseIntent) {
	quote := intent.Quote
	request := intent.Request
	if quote.BasePrice <= 0 || quote.Price <= 0 || quote.Price > math.MaxUint16 {
		panic("HorsePurchaseDialog: valid horse quote is required")
	}
	validateHorseQuoteGold("HorsePurchaseDialog", quote)
	if request.ReviewedMerchantID == "" || request.ReviewedPricingCharacterIndex < 0 ||
		request.ReviewedPrice != quote.Price || request.ReviewedGoldBefore != quote.GoldBefore ||
		request.ReviewedPlacement != quote.Placement {
		panic("HorsePurchaseDialog: detached request does not match quote")
	}
	validateHorsePlacementStatus(
		"HorsePurchaseDialog",
		quote.Placement.BlockStatus,
		intent.Preview,
		false,
	)
}
