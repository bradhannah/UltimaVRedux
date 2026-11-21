package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*ShipPurchaseDialog)(nil)

// ShipPurchaseDialogConfig configures one nested vessel approval modal.
type ShipPurchaseDialogConfig struct {
	Keyboard *input.Keyboard
	Intent   ShipPurchaseIntent
}

// ShipPurchaseDialog displays only detached quote consequences and emits the reviewed request.
type ShipPurchaseDialog struct {
	quantityInput
	keyboard  *input.Keyboard
	intent    ShipPurchaseIntent
	quantity  int
	dismissed bool
	cancelled bool
	pending   *game_state.ShipPurchaseRequest
	window    *amountWindow
}

// NewShipPurchaseDialog creates a centered keyboard-only purchase approval.
func NewShipPurchaseDialog(config ShipPurchaseDialogConfig) *ShipPurchaseDialog {
	if config.Keyboard == nil {
		panic("ShipPurchaseDialog: keyboard is required")
	}
	validateShipPurchaseIntent(config.Intent)
	dialog := &ShipPurchaseDialog{
		quantityInput: newQuantityInput(config.Keyboard, config.Keyboard.IsKeyHeld,
			ebiten.KeyM, ebiten.KeyUp, ebiten.KeyDown),
		keyboard:  config.Keyboard,
		intent:    config.Intent,
		quantity:  1,
		dismissed: false,
		cancelled: false,
		pending:   nil,
		window:    newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque semantic merchant modal.
func (d *ShipPurchaseDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update confirms or cancels the reviewed request after release debounce.
func (d *ShipPurchaseDialog) Update() {
	if d.dismissed {
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
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		request := d.intent.Request
		approval := d.GetApprovalView()
		request.Quantity = approval.Quantity
		request.ReviewedTotalPrice = approval.TotalPrice
		d.pending = &request
		d.Dismiss()
	}
}

func (d *ShipPurchaseDialog) handleQuantityInput() bool {
	if !d.choosesQuantity() {
		_, owned := d.pollQuantityInput()

		return owned
	}
	quantity, owned := d.pollAmount(d.quantity, len(d.intent.Quote.ApprovalViews))
	if quantity != d.quantity {
		d.quantity = quantity
		d.window.set(d.step())
	}

	return owned
}

// choosesQuantity reports that this is skiffs going aboard a frigate, more than
// one of which can be had.
func (d *ShipPurchaseDialog) choosesQuantity() bool {
	return d.intent.Quote.Action == game_state.ShipPurchaseLoadSkiff && len(d.intent.Quote.ApprovalViews) > 1
}

// ConsumePendingPurchaseRequest returns and clears the one comparison-only request.
func (d *ShipPurchaseDialog) ConsumePendingPurchaseRequest() (game_state.ShipPurchaseRequest, bool) {
	if d.pending == nil {
		return emptyShipPurchaseRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetApprovalView returns the authority-supplied projection for the selected quantity.
func (d *ShipPurchaseDialog) GetApprovalView() game_state.ShipPurchaseApprovalView {
	return d.intent.Quote.ApprovalViews[d.quantity-1]
}

// GetBounds returns the centered modal bounds.
func (d *ShipPurchaseDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape closed the approval.
func (d *ShipPurchaseDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the modal can be removed.
func (d *ShipPurchaseDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the modal without emitting a request.
func (d *ShipPurchaseDialog) Dismiss() {
	d.onDialogFocusChanged(false)
	d.dismissed = true
}

// step describes the approval window: the vessel, its price, and what the party's
// gold and the frigate's skiffs will be.
func (d *ShipPurchaseDialog) step() amountStep {
	quote, approval := d.intent.Quote, d.GetApprovalView()
	step := emptyAmountStep()
	step.preview, step.verb = d.intent.Preview, amountBuyVerb
	step.title = amountBuyVerb + " " + shipVehicleName(quote.VehicleType)
	delivered := amountNote{text: "Delivered to " + d.intent.HarbourName + ".", tone: amountPlain}
	gold := amountRow{label: amountGoldRow, have: int(approval.GoldBefore), then: int(approval.GoldAfterPayment)}
	switch quote.Action {
	case game_state.ShipPurchaseCreateFrigate:
		step.unit = fmt.Sprintf("With %d skiffs aboard", shipShopFrigateSkiffs)
		step.rows, step.notes = []amountRow{gold}, []amountNote{delivered}
	case game_state.ShipPurchaseCreateStandaloneSkiff:
		step.rows, step.notes = []amountRow{gold}, []amountNote{delivered}
	case game_state.ShipPurchaseLoadSkiff:
		step.title = amountBuyVerb + " Skiffs"
		step.unit = bundleUnitPrice(approval.UnitPrice, 1)
		step.noun, step.count, step.most = "Skiffs", approval.Quantity, len(quote.ApprovalViews)
		step.rows = []amountRow{
			{label: "Aboard", have: quote.Berth.SkiffCount, then: approval.SkiffCountAfter}, gold,
		}
		if quote.ApprovalViews[len(quote.ApprovalViews)-1].SkiffCountAfter == game_state.ShipShopMaximumSkiffs {
			step.reserve = 1
		}
		if approval.SkiffCountAfter == game_state.ShipShopMaximumSkiffs {
			step.notes = []amountNote{{text: "The frigate will then be full.", tone: amountPlain}}
		}
	default:
		panic("ShipPurchaseDialog.step: unknown action")
	}
	if !approval.Affordable {
		// Asking without the gold is still a real request: the shipwright refuses
		// and the talk ends (MERCH-0008), so Enter stays and says what it will do.
		step.noun, step.rows, step.reserve, step.verb = "", nil, 0, "Ask anyway"
		step.notes = shortOfGoldNotes("It costs", approval.TotalPrice, int(approval.GoldBefore),
			"Asking anyway ends the talk.")
	}

	return step
}

func validateShipPurchaseIntent(intent ShipPurchaseIntent) {
	quote := intent.Quote
	request := intent.Request
	if strings.TrimSpace(intent.HarbourName) == "" || quote.BlockReason.IsBlocked() {
		panic("ShipPurchaseDialog: available detached quote is required")
	}
	validateShipPurchaseQuoteIdentity(quote)
	validateShipPurchaseRequestMatch(quote, request)
	validateShipBerth(quote.Berth)
	validateShipTravelReview(intent.Travel, false)
	validateShipPurchaseIntentLinks(intent)
	validateShipPurchaseGold(quote)
}

func validateShipPurchaseQuoteIdentity(quote game_state.ShipPurchaseQuote) {
	if quote.VehicleType != references.FrigateVehicle && quote.VehicleType != references.SkiffVehicle ||
		quote.BasePrice <= 0 || quote.Price <= 0 || quote.Price > math.MaxUint16 || quote.RequestFingerprint == "" {
		panic("ShipPurchaseDialog: valid detached quote is required")
	}
	validateShipQuoteAction(quote, quote.VehicleType)
}

func validateShipPurchaseIntentLinks(intent ShipPurchaseIntent) {
	if intent.HarbourName != intent.Travel.HarbourName ||
		intent.Request.ReviewedMerchantID != intent.Travel.MerchantID ||
		intent.Quote.Berth.Location != intent.Travel.DockLocation ||
		intent.Quote.Berth.Position != intent.Travel.BerthPosition ||
		intent.Quote.Berth.VehicleType != intent.Travel.VehicleType ||
		intent.Quote.Berth.Pending != intent.Travel.PendingDelivery {
		panic("ShipPurchaseDialog: detached quote and harbour review do not match")
	}
}

func validateShipPurchaseRequestMatch(
	quote game_state.ShipPurchaseQuote,
	request game_state.ShipPurchaseRequest,
) {
	if request.ReviewedMerchantID == "" || request.ReviewedPricingCharacterIndex < 0 ||
		request.VehicleType != quote.VehicleType || request.Action != quote.Action ||
		request.ReviewedPrice != quote.Price || request.ReviewedGoldBefore != quote.GoldBefore ||
		request.ReviewedFingerprint != quote.RequestFingerprint ||
		!shipRequestMatchesApproval(request, quote) {
		panic("ShipPurchaseDialog: detached request does not match quote")
	}
}

func validateShipPurchaseGold(quote game_state.ShipPurchaseQuote) {
	if len(quote.ApprovalViews) == 0 {
		panic("ShipPurchaseDialog: available quote requires approval views")
	}
	for index, approval := range quote.ApprovalViews {
		validateShipApprovalIdentity(quote, approval, index+1)
		validateShipApprovalGold(approval)
	}
	first := quote.ApprovalViews[0]
	if quote.Affordable != first.Affordable || quote.GoldAfterPayment != first.GoldAfterPayment {
		panic("ShipPurchaseDialog: quote and first approval differ")
	}
}

func validateShipApprovalIdentity(
	quote game_state.ShipPurchaseQuote,
	approval game_state.ShipPurchaseApprovalView,
	quantity int,
) {
	if approval.Quantity != quantity || approval.UnitPrice != quote.Price ||
		approval.TotalPrice != quote.Price*quantity || approval.GoldBefore != quote.GoldBefore {
		panic("ShipPurchaseDialog: approval projection is inconsistent")
	}
}

func validateShipApprovalGold(approval game_state.ShipPurchaseApprovalView) {
	expectedAfter := approval.GoldBefore
	if approval.Affordable {
		if approval.TotalPrice > int(approval.GoldBefore) {
			panic("ShipPurchaseDialog: affordable approval exceeds gold")
		}
		expectedAfter -= uint16(approval.TotalPrice) //nolint:gosec // Trusted affordability bounds the total.
	} else if approval.TotalPrice <= int(approval.GoldBefore) {
		panic("ShipPurchaseDialog: unaffordable approval is within gold")
	}
	if approval.GoldAfterPayment != expectedAfter {
		panic("ShipPurchaseDialog: approval gold consequence is inconsistent")
	}
}
