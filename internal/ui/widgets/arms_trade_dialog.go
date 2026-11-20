package widgets

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	armsTradeStartPercentX    = 0.28
	armsTradeEndPercentX      = 0.72
	armsTradeStartPercentY    = 0.16
	armsTradeEndPercentY      = 0.84
	armsTradeHeaderRatio      = 0.13
	armsTradeFooterRatio      = 0.17
	armsTradeMinimumInset     = 5
	armsTradeLineSpacing      = 1.1
	armsTradeMaximumBodyLines = 12
	armsTradeFooterLines      = 3 // Quantity/Sell All wraps once before the explicit confirm/back row.
	armsTradeMinimumTextChars = 8
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

	amountLine int // The body line holding the amount row, or -1.

	frame *merchantModalFrame

	headerOutput *text.Output
	bodyOutput   *text.Output
	footerOutput *text.Output
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
		validateArmsFinalizedText("refusal", config.Prose.Refusal)
	case config.Intent.Mode == ArmsShopBuy:
		validateArmsFinalizedText("purchase pitch", config.Prose.Pitch)
	case config.Intent.Mode == ArmsShopSell:
		validateArmsFinalizedText("sale offer", config.Prose.Offer)
	}

	dialog := &ArmsTradeDialog{ //nolint:exhaustruct_v5 // Layout, output and result fields initialize on demand.
		keyboard:   config.Keyboard,
		intent:     config.Intent,
		prose:      config.Prose,
		quantity:   1,
		refusal:    refusal,
		amountLine: -1,
		frame: newMerchantModalFrame(merchantModalFrameConfig{
			placement:             armsTradePlacement(),
			headerRatio:           armsTradeHeaderRatio,
			footerRatio:           armsTradeFooterRatio,
			minimumInset:          armsTradeMinimumInset,
			lineSpacingMultiplier: armsTradeLineSpacing,
			footerMinimumLines:    armsTradeFooterLines,
		}),
	}
	// M is the shared Max key; A remains the former Sell all alias.
	dialog.quantityInput = newQuantityInput(config.Keyboard, config.Keyboard.IsKeyHeld,
		ebiten.KeyM, ebiten.KeyA, ebiten.KeyUp, ebiten.KeyDown)
	dialog.ensureLayout()

	return dialog
}

// Draw renders the focused modal.
func (d *ArmsTradeDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.fillBodyLine(screen, d.bodyOutput, d.amountLine)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
}

// Update handles quantity adjustment, confirmation, acknowledgment, and cancellation.
func (d *ArmsTradeDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
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

// GetFooterGuidance returns only controls valid for this trade.
func (d *ArmsTradeDialog) GetFooterGuidance() string {
	if d.refusal != ArmsTradeNoRefusal {
		return "[Enter/Esc] Acknowledge"
	}
	if d.intent.Mode == ArmsShopBuy && d.intent.BuyQuote.AmmoRefill {
		return "[Enter] Buy   [Esc] Back"
	}

	return "[Left/Right] Quantity   [M] Most\n[Enter] " + d.intent.Mode.String() + "   [Esc] Back"
}

// onlyOneAmount reports that Left/Right and M have nothing to choose between.
func (d *ArmsTradeDialog) onlyOneAmount() bool {
	if d.refusal != ArmsTradeNoRefusal {
		return false
	}
	if d.intent.Mode == ArmsShopBuy {
		return d.intent.BuyQuote.MaxQuantity <= 1
	}

	return d.intent.SellQuote.MaxQuantity <= 1
}

// GetBounds returns the centered modal bounds.
func (d *ArmsTradeDialog) GetBounds() image.Rectangle {
	d.ensureLayout()

	return d.frame.bounds()
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
	key, owned := d.pollQuantityInput()
	if key == nil {
		return owned
	}
	if d.intent.Mode == ArmsShopBuy && d.intent.BuyQuote.AmmoRefill {
		return owned
	}
	maximum := d.maximumQuantity()
	switch *key { //nolint:exhaustive // The quantity helper returns only this dialog's configured keys.
	case ebiten.KeyM, ebiten.KeyA:
		d.setQuantity(maximum)
	case ebiten.KeyLeft, ebiten.KeyUp:
		d.setQuantity(max(uint16(1), d.quantity-1))
	case ebiten.KeyRight, ebiten.KeyDown:
		d.setQuantity(min(maximum, d.quantity+1))
	}

	return owned
}

func (d *ArmsTradeDialog) maximumQuantity() uint16 {
	if d.intent.Mode == ArmsShopBuy {
		return max(uint16(1), d.intent.BuyQuote.MaxQuantity)
	}

	return d.intent.SellQuote.MaxQuantity
}

func (d *ArmsTradeDialog) setQuantity(quantity uint16) {
	if quantity == d.quantity {
		return
	}
	d.quantity = quantity
	d.populateOutputs()
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

func (d *ArmsTradeDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	d.buildOutputs()
	if d.refusal == ArmsTradeNoRefusal {
		// The amount step's window is as tall as its content, pitch included.
		d.frame.fitBodyLines(d.bodyOutput.GetPanelContentLineCount())
		d.buildOutputs()
	}
}

func (d *ArmsTradeDialog) buildOutputs() {
	d.headerOutput, d.bodyOutput, d.footerOutput = d.frame.newOutputs(merchantModalTextBudget{
		lineSpacing: armsTradeLineSpacing, maximumBodyLines: armsTradeMaximumBodyLines,
		footerLines: armsTradeFooterLines, minimumChars: armsTradeMinimumTextChars, fixedBody: false,
	})
	d.populateOutputs()
}

func (d *ArmsTradeDialog) populateOutputs() {
	if d.headerOutput == nil || d.bodyOutput == nil || d.footerOutput == nil {
		return
	}
	equipment := d.intent.Equipment()
	d.amountLine = -1
	d.headerOutput.Reset()
	d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text(d.intent.Mode.String()+" "+equipment.String(), DialogHeaderColor).Done()
	d.bodyOutput.Reset()
	switch {
	case d.refusal != ArmsTradeNoRefusal:
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			White(quoteArmsProse(d.prose.Refusal)).WordWrap().Done()
		d.bodyOutput.AppendLine().White("").Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text(armsRefusalLabel(d.refusal), DialogStatusWarningColor).Done()
	case d.intent.Mode == ArmsShopBuy:
		d.populateBuyBody()
	default:
		d.populateSellBody()
	}
	d.populateFooter()
}

func (d *ArmsTradeDialog) populateBuyBody() {
	quote := d.intent.BuyQuote
	d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
		White(quoteArmsProse(d.prose.Pitch)).WordWrap().Done()
	d.bodyOutput.AppendLine().White("").Done()
	total := quote.UnitPrice * int(d.quantity)
	amount := shopAmount{
		label: "Quantity", count: int(d.quantity), most: int(max(uint16(1), quote.MaxQuantity)),
		price: fmt.Sprintf("%d gold each", quote.UnitPrice), goods: quote.Equipment.String(),
		goodsNow: int(quote.QuantityBefore), goodsThen: int(quote.QuantityBefore) + int(d.quantity),
		goldNow: int(quote.Gold), goldThen: int(quote.Gold) - total, theftAfter: true,
	}
	if quote.AmmoRefill {
		amount.label, amount.goodsThen = "", int(party_state.ItemQuantitySmallMax)
		amount.price = fmt.Sprintf("Refills to %d for %d gold", party_state.ItemQuantitySmallMax, total)
	}
	d.amountLine = appendShopAmount(d.bodyOutput, amount)
}

func (d *ArmsTradeDialog) populateSellBody() {
	quote := d.intent.SellQuote
	d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
		White(quoteArmsProse(d.prose.Offer)).WordWrap().Done()
	d.bodyOutput.AppendLine().White("").Done()
	total := quote.UnitOffer * int(d.quantity)
	d.amountLine = appendShopAmount(d.bodyOutput, shopAmount{
		label: "Quantity", count: int(d.quantity), most: int(quote.MaxQuantity),
		price: fmt.Sprintf("%d gold each", quote.UnitOffer), goods: quote.Equipment.String(),
		goodsNow: int(quote.QuantityBefore), goodsThen: int(quote.QuantityBefore) - int(d.quantity),
		goldNow: int(quote.Gold), goldThen: int(quote.Gold) + total, theftAfter: false,
	})
}

func (d *ArmsTradeDialog) populateFooter() {
	d.footerOutput.Reset()
	appendAmountGuidance(d.footerOutput, d.GetFooterGuidance(), d.frame.footerText.Dx(), d.onlyOneAmount())
}

func armsTradePlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: armsTradeStartPercentX,
		EndPercentX:   armsTradeEndPercentX,
		StartPercentY: armsTradeStartPercentY,
		EndPercentY:   armsTradeEndPercentY,
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
		return "USED AMMUNITION: ARMS SESSION ENDS"
	case ArmsTradeZeroPriceRefusal:
		return "NO OFFER: RETURN TO SELL"
	case ArmsTradeGoldCapacityRefusal:
		return "GOLD CAPACITY: RETURN TO SELL"
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
