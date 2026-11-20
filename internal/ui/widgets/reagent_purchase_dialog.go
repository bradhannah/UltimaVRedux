package widgets

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	reagentPurchaseStartPercentX    = 0.27
	reagentPurchaseEndPercentX      = 0.73
	reagentPurchaseStartPercentY    = 0.12
	reagentPurchaseEndPercentY      = 0.88
	reagentPurchaseHeaderRatio      = 0.12
	reagentPurchaseFooterRatio      = 0.17
	reagentPurchaseMinimumInset     = 6
	reagentPurchaseLineSpacing      = 1.12
	reagentPurchaseMaximumBodyLines = 13
	reagentPurchaseMinimumTextChars = 8
	reagentPurchaseFooterLines      = 2
	reagentPurchaseWarningLines     = 2
	reagentPurchaseFooter           = "[Left/Right] Bundles   [M] Most\n[Enter] Buy   [Esc] Back"
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
	amountLine  int // The body line holding the amount row.

	frame        *merchantModalFrame
	headerOutput *text.Output
	bodyOutput   *text.Output
	footerOutput *text.Output
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
		amountLine:  -1,
		frame: newMerchantModalFrame(merchantModalFrameConfig{
			placement:             reagentPurchasePlacement(),
			headerRatio:           reagentPurchaseHeaderRatio,
			footerRatio:           reagentPurchaseFooterRatio,
			minimumInset:          reagentPurchaseMinimumInset,
			lineSpacingMultiplier: reagentPurchaseLineSpacing,
			footerMinimumLines:    reagentPurchaseFooterLines,
		}),
		headerOutput: nil,
		bodyOutput:   nil,
		footerOutput: nil,
	}
	dialog.ensureLayout()

	return dialog
}

// Draw renders the centered opaque purchase modal.
func (d *ReagentPurchaseDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.fillBodyLine(screen, d.bodyOutput, d.amountLine)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
}

// Update processes quantity selection, confirmation, and cancellation.
func (d *ReagentPurchaseDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
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
	key, owned := d.pollQuantityInput()
	if key == nil {
		return owned
	}
	switch *key { //nolint:exhaustive // The quantity helper returns only this dialog's configured keys.
	case ebiten.KeyM:
		d.setBundleCount(uint16(len(d.quote.ApprovalViews))) //nolint:gosec // Valid reagent quotes contain at most 99 views.
	case ebiten.KeyLeft, ebiten.KeyUp:
		d.setBundleCount(max(uint16(1), d.bundleCount-1))
	case ebiten.KeyRight, ebiten.KeyDown:
		maximum := uint16(len(d.quote.ApprovalViews)) //nolint:gosec // Valid quotes contain at most 99 views.
		d.setBundleCount(min(maximum, d.bundleCount+1))
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
	d.ensureLayout()

	return d.frame.bounds()
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

func (d *ReagentPurchaseDialog) setBundleCount(count uint16) {
	if count == d.bundleCount {
		return
	}
	d.bundleCount = count
	d.populateOutputs()
}

func (d *ReagentPurchaseDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	d.buildOutputs()
	// The window is as tall as its content, with room kept for the two warning
	// lines, so it never changes size as the amount does.
	lines := d.bodyOutput.GetPanelContentLineCount()
	if !d.GetApprovalView().PaidOverflow {
		lines += reagentPurchaseWarningLines
	}
	d.frame.fitBodyLines(lines)
	d.buildOutputs()
}

func (d *ReagentPurchaseDialog) buildOutputs() {
	d.headerOutput, d.bodyOutput, d.footerOutput = d.frame.newOutputs(merchantModalTextBudget{
		lineSpacing: reagentPurchaseLineSpacing, maximumBodyLines: reagentPurchaseMaximumBodyLines,
		footerLines: reagentPurchaseFooterLines, minimumChars: reagentPurchaseMinimumTextChars, fixedBody: false,
	})
	d.populateOutputs()
}

func (d *ReagentPurchaseDialog) populateOutputs() {
	if d.headerOutput == nil || d.bodyOutput == nil || d.footerOutput == nil {
		return
	}
	approval := d.GetApprovalView()
	d.headerOutput.Reset()
	header := d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter)
	if d.preview {
		header.Text("PREVIEW - ", DialogStatusWarningColor)
	}
	header.Text("Purchase "+approval.ReagentName, DialogHeaderColor).Done()

	d.bodyOutput.Reset()
	d.amountLine = appendShopAmount(d.bodyOutput, shopAmount{
		label: "Bundles", count: int(approval.BundleCount), most: int(approval.MaxBundles),
		price: fmt.Sprintf("%d gold for %d %s", approval.PricePerBundle, approval.BundleQuantity,
			approval.ReagentName),
		goods: approval.ReagentName, goodsNow: int(approval.QuantityBefore), goodsThen: int(approval.QuantityAfter),
		goldNow: int(approval.GoldBefore), goldThen: int(approval.GoldAfterPayment), theftAfter: true,
	})
	if approval.PaidOverflow {
		d.bodyOutput.AppendLine().Textf(
			" %d %s will not fit.", DialogWarningColor,
			approval.OverflowQuantity, reagentOverflowNoun(approval.OverflowQuantity),
		).Done()
		d.bodyOutput.AppendLine().Text(" Full bundle prices apply.", DialogWarningColor).Done()
	}

	d.footerOutput.Reset()
	appendAmountGuidance(d.footerOutput, reagentPurchaseFooter, d.frame.footerText.Dx(), d.quote.MaxBundles <= 1)
}

func reagentOverflowNoun(quantity uint16) string {
	if quantity == 1 {
		return "reagent"
	}

	return "reagents"
}

func reagentPurchasePlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: reagentPurchaseStartPercentX,
		EndPercentX:   reagentPurchaseEndPercentX,
		StartPercentY: reagentPurchaseStartPercentY,
		EndPercentY:   reagentPurchaseEndPercentY,
	}
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
