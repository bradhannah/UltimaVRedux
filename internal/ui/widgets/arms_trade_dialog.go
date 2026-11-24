package widgets

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	armsAmountNoun    = "Quantity"
	armsRefusalFooter = "[Enter/Esc] Continue"
)

var _ Widget = (*ArmsTradeDialog)(nil)

// ArmsTradeRefusalKind distinguishes local no-request sale outcomes.
type ArmsTradeRefusalKind int

const (
	// ArmsTradeNoRefusal allows quantity selection and confirmation.
	ArmsTradeNoRefusal ArmsTradeRefusalKind = iota
	// ArmsTradeUsedAmmunitionRefusal rejects used ammunition and ends Arms Talk.
	ArmsTradeUsedAmmunitionRefusal
	// ArmsTradeZeroPriceRefusal rejects equipment without a sale price.
	ArmsTradeZeroPriceRefusal
	// ArmsTradeGoldCapacityRefusal rejects a sale that would exceed the gold cap.
	ArmsTradeGoldCapacityRefusal
)

// IsTerminal reports whether dismissal should end the complete Arms interaction.
func (k ArmsTradeRefusalKind) IsTerminal() bool {
	return k == ArmsTradeUsedAmmunitionRefusal
}

// ArmsTradeDialogProse contains finalized copy selected by the workflow.
type ArmsTradeDialogProse struct {
	Pitch   string
	Offer   string
	Refusal string
}

// ArmsTradeDialogConfig configures one focused nested Arms modal.
type ArmsTradeDialogConfig struct {
	Keyboard *input.Keyboard
	Intent   ArmsTradeIntent
	Prose    ArmsTradeDialogProse
}

// ArmsTradeDialog confirms one bounded buy or sell request.
type ArmsTradeDialog struct {
	quantityInput
	keyboard  *input.Keyboard
	intent    ArmsTradeIntent
	prose     ArmsTradeDialogProse
	quantity  uint16
	refusal   ArmsTradeRefusalKind
	dismissed bool
	cancelled bool
	pending   *ArmsTradeRequest
	window    *amountWindow
}

// NewArmsTradeDialog creates a centered keyboard-only transaction modal.
func NewArmsTradeDialog(config ArmsTradeDialogConfig) *ArmsTradeDialog {
	if config.Keyboard == nil {
		panic("ArmsTradeDialog: keyboard is required")
	}
	validateArmsTradeIntent(config.Intent)
	refusal := armsTradeRefusal(config.Intent)
	switch {
	case refusal != ArmsTradeNoRefusal:
		requireFinalizedText("Arms widgets", "refusal", config.Prose.Refusal)
	case config.Intent.Mode == ArmsShopBuy:
		requireFinalizedText("Arms widgets", "purchase pitch", config.Prose.Pitch)
	case config.Intent.Mode == ArmsShopSell:
		requireFinalizedText("Arms widgets", "sale offer", config.Prose.Offer)
	}

	dialog := &ArmsTradeDialog{
		// M is the shared Most key; A remains the former Sell all alias.
		quantityInput: newQuantityInput(config.Keyboard, config.Keyboard.IsKeyHeld,
			ebiten.KeyM, ebiten.KeyA, ebiten.KeyUp, ebiten.KeyDown),
		keyboard:  config.Keyboard,
		intent:    config.Intent,
		prose:     config.Prose,
		quantity:  1,
		refusal:   refusal,
		dismissed: false,
		cancelled: false,
		pending:   nil,
		window:    newAmountWindow(),
	}
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the focused modal.
func (d *ArmsTradeDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update handles quantity adjustment, confirmation, acknowledgment, and cancellation.
func (d *ArmsTradeDialog) Update() {
	if d.dismissed {
		return
	}
	if d.refusal != ArmsTradeNoRefusal {
		if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.Dismiss()
		}

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancelled = true
		d.Dismiss()

		return
	}
	if d.handleQuantityInput() {
		return
	}
	if d.enterPressed() {
		request := d.makeRequest()
		d.pending = &request
		d.Dismiss()
	}
}

// ConsumePendingRequest returns and clears the modal's one typed request.
func (d *ArmsTradeDialog) ConsumePendingRequest() (ArmsTradeRequest, bool) {
	if d.pending == nil {
		return emptyArmsTradeRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetIntent returns the trusted detached quote shown by the modal.
func (d *ArmsTradeDialog) GetIntent() ArmsTradeIntent { return d.intent }

// GetRefusalKind returns the local no-request refusal classification.
func (d *ArmsTradeDialog) GetRefusalKind() ArmsTradeRefusalKind { return d.refusal }

// GetBounds returns the centered modal bounds.
func (d *ArmsTradeDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape dismissed the modal.
func (d *ArmsTradeDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the modal can be removed.
func (d *ArmsTradeDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the modal without emitting a request.
func (d *ArmsTradeDialog) Dismiss() {
	d.onDialogFocusChanged(false)
	d.dismissed = true
}

func (d *ArmsTradeDialog) handleQuantityInput() bool {
	if d.intent.Mode == ArmsShopBuy && d.intent.BuyQuote.AmmoRefill {
		_, owned := d.pollQuantityInput()

		return owned
	}
	quantity, owned := d.pollAmount(int(d.quantity), int(d.maximumQuantity()))
	if quantity != int(d.quantity) {
		d.quantity = uint16(quantity) //nolint:gosec // pollAmount stays within the quote's small maximum.
		d.window.set(d.step())
	}

	return owned
}

func (d *ArmsTradeDialog) maximumQuantity() uint16 {
	if d.intent.Mode == ArmsShopBuy {
		return max(uint16(1), d.intent.BuyQuote.MaxQuantity)
	}

	return d.intent.SellQuote.MaxQuantity
}

func (d *ArmsTradeDialog) makeRequest() ArmsTradeRequest {
	request := emptyArmsTradeRequest()
	request.Mode = d.intent.Mode
	if d.intent.Mode == ArmsShopBuy {
		request.Buy = game_state.ArmsBuyRequest{Equipment: d.intent.BuyQuote.Equipment, Quantity: d.quantity}
	} else {
		request.Sell = game_state.ArmsSellRequest{Equipment: d.intent.SellQuote.Equipment, Quantity: d.quantity}
	}

	return request
}

func (d *ArmsTradeDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

// step describes the amount window for the quantity chosen.
func (d *ArmsTradeDialog) step() amountStep {
	step := emptyAmountStep()
	step.title = d.intent.Mode.String() + " " + d.intent.Equipment().String()
	step.verb = d.intent.Mode.String()
	switch {
	case d.refusal != ArmsTradeNoRefusal:
		step.pitch = quoteArmsProse(d.prose.Refusal)
		step.notes = []amountNote{{text: armsRefusalLabel(d.refusal), tone: amountVerdict}}
		step.footer = armsRefusalFooter
	case d.intent.Mode == ArmsShopBuy:
		d.describeBuy(&step)
	default:
		d.describeSell(&step)
	}

	return step
}

func (d *ArmsTradeDialog) describeBuy(step *amountStep) {
	quote := d.intent.BuyQuote
	step.pitch = quoteArmsProse(d.prose.Pitch)
	step.unit = bundleUnitPrice(quote.UnitPrice, 1)
	step.noun, step.count, step.most = armsAmountNoun, int(d.quantity), int(d.maximumQuantity())
	carried := int(quote.QuantityBefore) + int(d.quantity)
	if quote.AmmoRefill {
		step.noun, carried = "", int(party_state.ItemQuantitySmallMax)
		step.unit = fmt.Sprintf("%d gold to refill to %d", quote.UnitPrice, carried)
	}
	step.rows = []amountRow{
		{label: amountCarriedRow, have: int(quote.QuantityBefore), then: carried},
		{label: amountGoldRow, have: int(quote.Gold), then: int(quote.Gold) - quote.UnitPrice*int(d.quantity)},
	}
}

func (d *ArmsTradeDialog) describeSell(step *amountStep) {
	quote := d.intent.SellQuote
	step.pitch = quoteArmsProse(d.prose.Offer)
	step.unit = bundleUnitPrice(quote.UnitOffer, 1)
	step.noun, step.count, step.most = armsAmountNoun, int(d.quantity), int(quote.MaxQuantity)
	step.rows = []amountRow{
		{label: amountCarriedRow, have: int(quote.QuantityBefore), then: int(quote.QuantityBefore) - int(d.quantity)},
		{label: amountGoldRow, have: int(quote.Gold), then: int(quote.Gold) + quote.UnitOffer*int(d.quantity)},
	}
}

func armsTradeRefusal(intent ArmsTradeIntent) ArmsTradeRefusalKind {
	if intent.Mode == ArmsShopBuy {
		return ArmsTradeNoRefusal
	}
	quote := intent.SellQuote
	switch {
	case quote.Equipment.IsAmmo():
		return ArmsTradeUsedAmmunitionRefusal
	case quote.BasePrice == 0 || quote.UnitOffer == 0 || !quote.Sellable:
		return ArmsTradeZeroPriceRefusal
	case quote.MaxQuantity == 0:
		return ArmsTradeGoldCapacityRefusal
	default:
		return ArmsTradeNoRefusal
	}
}

func armsRefusalLabel(kind ArmsTradeRefusalKind) string {
	switch kind {
	case ArmsTradeUsedAmmunitionRefusal:
		return "The trade is ended."
	case ArmsTradeZeroPriceRefusal:
		return "Choose another."
	case ArmsTradeGoldCapacityRefusal:
		return "Thou canst not carry any more gold! Choose another."
	case ArmsTradeNoRefusal:
		return ""
	default:
		panic("armsRefusalLabel: unknown refusal")
	}
}

func quoteArmsProse(value string) string {
	return `"` + strings.Trim(strings.TrimSpace(value), `"`) + `"`
}

func validateArmsTradeIntent(intent ArmsTradeIntent) {
	switch intent.Mode {
	case ArmsShopBuy:
		validateArmsBuyTradeIntent(intent.BuyQuote)
	case ArmsShopSell:
		validateArmsSellTradeIntent(intent.SellQuote)
	default:
		panic("ArmsTradeDialog: known mode is required")
	}
}

func validateArmsBuyTradeIntent(quote game_state.ArmsBuyQuote) {
	if !validArmsEquipment(quote.Equipment) || !validArmsBuyPriceAndGold(quote, quote.Gold) ||
		!validArmsBuyQuantity(quote) {
		panic("ArmsTradeDialog: invalid buy intent")
	}
}

func validateArmsSellTradeIntent(quote game_state.ArmsSellQuote) {
	if !validArmsEquipment(quote.Equipment) || !validArmsSellQuantityAndGold(quote, quote.Gold) {
		panic("ArmsTradeDialog: invalid sell intent")
	}
	unsellable := quote.Equipment.IsAmmo() || quote.BasePrice == 0
	if unsellable && !validRefusedArmsSellQuote(quote) ||
		!unsellable && !validOfferedArmsSellQuote(quote) {
		panic("ArmsTradeDialog: inconsistent sell intent")
	}
}
