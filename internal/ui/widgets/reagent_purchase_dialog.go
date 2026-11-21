package widgets

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*ReagentPurchaseDialog)(nil)

// ReagentPurchaseDialogConfig configures one nested reagent quantity modal.
type ReagentPurchaseDialogConfig struct {
	Keyboard *input.Keyboard
	Quote    game_state.ReagentOfferQuote
	Preview  bool
}

// ReagentPurchaseDialog selects supplied approval views and emits one stable request.
type ReagentPurchaseDialog struct {
	quantityInput
	keyboard    *input.Keyboard
	quote       game_state.ReagentOfferQuote
	preview     bool
	bundleCount uint16
	dismissed   bool
	cancelled   bool
	pending     *game_state.ReagentPurchaseRequest
	window      *amountWindow
}

// NewReagentPurchaseDialog creates a centered keyboard-only quantity modal.
func NewReagentPurchaseDialog(config ReagentPurchaseDialogConfig) *ReagentPurchaseDialog {
	if config.Keyboard == nil {
		panic("ReagentPurchaseDialog: keyboard is required")
	}
	validateReagentPurchaseQuote(config.Quote)
	dialog := &ReagentPurchaseDialog{
		quantityInput: newQuantityInput(config.Keyboard, config.Keyboard.IsKeyHeld,
			ebiten.KeyM, ebiten.KeyUp, ebiten.KeyDown),
		keyboard:    config.Keyboard,
		quote:       cloneReagentOfferQuote(config.Quote),
		preview:     config.Preview,
		bundleCount: 1,
		dismissed:   false,
		cancelled:   false,
		pending:     nil,
		window:      newAmountWindow(),
	}
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the centered opaque purchase modal.
func (d *ReagentPurchaseDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update processes quantity selection, confirmation, and cancellation.
func (d *ReagentPurchaseDialog) Update() {
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
		request := game_state.ReagentPurchaseRequest{
			Reagent:     d.quote.Reagent,
			BundleCount: d.bundleCount,
		}
		d.pending = &request
		d.Dismiss()
	}
}

func (d *ReagentPurchaseDialog) handleQuantityInput() bool {
	count, owned := d.pollAmount(int(d.bundleCount), len(d.quote.ApprovalViews))
	if count != int(d.bundleCount) {
		d.bundleCount = uint16(count) //nolint:gosec // Valid reagent quotes contain at most 99 views.
		d.window.set(d.step())
	}

	return owned
}

// ConsumePendingPurchaseRequest returns and clears the one confirmed request.
func (d *ReagentPurchaseDialog) ConsumePendingPurchaseRequest() (game_state.ReagentPurchaseRequest, bool) {
	if d.pending == nil {
		return emptyReagentPurchaseRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetApprovalView returns the supplied authority view selected by bundle count.
func (d *ReagentPurchaseDialog) GetApprovalView() game_state.ReagentPurchaseApprovalView {
	return d.quote.ApprovalViews[d.bundleCount-1]
}

// GetBounds returns the centered opaque modal bounds.
func (d *ReagentPurchaseDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape closed the modal without confirmation.
func (d *ReagentPurchaseDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the modal may be removed.
func (d *ReagentPurchaseDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the modal without emitting a request.
func (d *ReagentPurchaseDialog) Dismiss() {
	d.onDialogFocusChanged(false)
	d.dismissed = true
}

// step describes the amount window for the bundle count chosen.
func (d *ReagentPurchaseDialog) step() amountStep {
	approval := d.GetApprovalView()
	step := emptyAmountStep()
	step.title, step.preview = amountBuyVerb+" "+approval.ReagentName, d.preview
	step.unit = bundleUnitPrice(approval.PricePerBundle, int(approval.BundleQuantity))
	step.noun, step.verb = "Bundles", amountBuyVerb
	step.count, step.most = int(approval.BundleCount), int(approval.MaxBundles)
	step.rows = []amountRow{
		{label: amountCarriedRow, have: int(approval.QuantityBefore), then: int(approval.QuantityAfter)},
		{label: amountGoldRow, have: int(approval.GoldBefore), then: int(approval.GoldAfterPayment)},
	}
	if d.quote.ApprovalViews[len(d.quote.ApprovalViews)-1].PaidOverflow {
		step.reserve = 1
	}
	if approval.PaidOverflow {
		step.notes = overflowNote(int(approval.OverflowQuantity))
	}

	return step
}

func emptyReagentPurchaseRequest() game_state.ReagentPurchaseRequest {
	return game_state.ReagentPurchaseRequest{Reagent: references.SulfurAsh, BundleCount: 0}
}

func validateReagentPurchaseQuote(quote game_state.ReagentOfferQuote) {
	validateReagentOfferQuote(quote, quote.Gold)
	if quote.BlockReason.IsBlocked() || quote.Full || quote.MaxBundles == 0 {
		panic("ReagentPurchaseDialog: an available reagent quote is required")
	}
}
