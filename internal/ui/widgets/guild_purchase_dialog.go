package widgets

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*GuildPurchaseDialog)(nil)

// GuildPurchaseDialogConfig configures a nested quantity modal for one trusted offer.
type GuildPurchaseDialogConfig struct {
	Keyboard *input.Keyboard
	Quote    game_state.GuildOfferQuote
	Pitch    string
}

// GuildPurchaseDialog selects a bounded bundle count and emits one authoritative request.
type GuildPurchaseDialog struct {
	quantityInput
	keyboard    *input.Keyboard
	quote       game_state.GuildOfferQuote
	pitch       string
	bundleCount uint16
	dismissed   bool
	cancelled   bool
	pending     *game_state.GuildPurchaseRequest
	window      *amountWindow
}

// NewGuildPurchaseDialog creates a centered keyboard-only quantity modal.
func NewGuildPurchaseDialog(config GuildPurchaseDialogConfig) *GuildPurchaseDialog {
	if config.Keyboard == nil {
		panic("GuildPurchaseDialog: keyboard is required")
	}
	validateGuildPurchaseQuote(config.Quote)
	if strings.TrimSpace(config.Pitch) == "" {
		panic("GuildPurchaseDialog: finalized pitch is required")
	}

	bundleCount := uint16(0)
	if config.Quote.MaxBundles > 0 {
		bundleCount = 1
	}
	dialog := &GuildPurchaseDialog{
		quantityInput: newQuantityInput(config.Keyboard, config.Keyboard.IsKeyHeld,
			ebiten.KeyM, ebiten.KeyUp, ebiten.KeyDown),
		keyboard:    config.Keyboard,
		quote:       config.Quote,
		pitch:       config.Pitch,
		bundleCount: bundleCount,
		dismissed:   false,
		cancelled:   false,
		pending:     nil,
		window:      newAmountWindow(),
	}
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the nested purchase quantity modal.
func (d *GuildPurchaseDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update processes keyboard-only quantity, confirmation, and cancellation input.
func (d *GuildPurchaseDialog) Update() {
	if d.dismissed {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancelled = true
		d.Dismiss()

		return
	}
	if d.quote.MaxBundles == 0 {
		return
	}
	count, owned := d.pollAmount(int(d.bundleCount), int(d.quote.MaxBundles))
	if count != int(d.bundleCount) {
		d.bundleCount = uint16(count) //nolint:gosec // pollAmount stays within the quote's small maximum.
		d.window.set(d.step())
	}
	if owned {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		request := game_state.GuildPurchaseRequest{
			Provision:   d.quote.Provision,
			BundleCount: d.bundleCount,
		}
		d.pending = &request
		d.Dismiss()
	}
}

// ConsumePendingPurchaseRequest returns and clears the one confirmed request.
func (d *GuildPurchaseDialog) ConsumePendingPurchaseRequest() (game_state.GuildPurchaseRequest, bool) {
	if d.pending == nil {
		return game_state.GuildPurchaseRequest{
			Provision:   references.NoProvision,
			BundleCount: 0,
		}, false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetBounds returns the nested modal's centered outer bounds.
func (d *GuildPurchaseDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape closed the modal without confirmation.
func (d *GuildPurchaseDialog) IsCancelled() bool {
	return d.cancelled
}

// IsDismissed reports whether the modal can be removed from the dialog stack.
func (d *GuildPurchaseDialog) IsDismissed() bool {
	return d.dismissed
}

// Dismiss closes the modal without emitting a request.
func (d *GuildPurchaseDialog) Dismiss() {
	d.onDialogFocusChanged(false)
	d.dismissed = true
}

// step describes the amount window for the bundle count chosen, or says why no
// bundle can be bought.
func (d *GuildPurchaseDialog) step() amountStep {
	quote := d.quote
	step := emptyAmountStep()
	step.title, step.verb = amountBuyVerb+" "+guildProvisionName(quote.Provision), amountBuyVerb
	step.unit = bundleUnitPrice(quote.PricePerBundle, int(quote.BundleQuantity))
	if quote.MaxBundles == 0 {
		step.refused, step.notes = true, guildRefusalNotes(quote)

		return step
	}
	step.pitch = quotedGuildPitch(d.pitch)
	step.noun, step.count, step.most = "Bundles", int(d.bundleCount), int(quote.MaxBundles)
	step.rows = []amountRow{
		{label: amountCarriedRow, have: int(quote.QuantityBefore), then: int(projectedGuildQuantity(quote, d.bundleCount))},
		{label: amountGoldRow, have: int(quote.Gold), then: int(quote.Gold) - quote.PricePerBundle*int(d.bundleCount)},
	}
	if guildOverflow(quote, quote.MaxBundles) > 0 {
		step.reserve = 1
	}
	if lost := guildOverflow(quote, d.bundleCount); lost > 0 {
		step.notes = overflowNote(lost)
	}

	return step
}

// guildOverflow is how many of the bundles' goods will not fit in the pack.
func guildOverflow(quote game_state.GuildOfferQuote, bundleCount uint16) int {
	return int(quote.QuantityBefore) + int(quote.BundleQuantity)*int(bundleCount) -
		int(projectedGuildQuantity(quote, bundleCount))
}

// guildRefusalNotes says, with its figures, why not even one bundle can be bought.
func guildRefusalNotes(quote game_state.GuildOfferQuote) []amountNote {
	if quote.QuantityBefore >= party_state.ItemQuantitySmallMax {
		return []amountNote{
			{text: "No room. The party carries", tone: amountWarning},
			{text: fmt.Sprintf("%d, the most it can.", quote.QuantityBefore), tone: amountWarning},
		}
	}

	return shortOfGoldNotes("A bundle costs", quote.PricePerBundle, int(quote.Gold))
}

func quotedGuildPitch(pitch string) string {
	return `"` + strings.Trim(strings.TrimSpace(pitch), `"`) + `"`
}

func validateGuildPurchaseQuote(quote game_state.GuildOfferQuote) {
	switch quote.Provision {
	case references.Key, references.Gem, references.Torches:
	case references.NoProvision, references.SkullKeys, references.Food, references.Gold:
		panic("GuildPurchaseDialog: supported guild provision is required")
	default:
		panic("GuildPurchaseDialog: known guild provision is required")
	}
	if quote.BundleQuantity == 0 || quote.BasePrice <= 0 || quote.PricePerBundle <= 0 {
		panic("GuildPurchaseDialog: quote prices and bundle quantity must be positive")
	}
	if quote.QuantityBefore > party_state.ItemQuantitySmallMax {
		panic("GuildPurchaseDialog: quote inventory quantity is invalid")
	}
	usefulBundles := 0
	if quote.QuantityBefore < party_state.ItemQuantitySmallMax {
		remaining := int(party_state.ItemQuantitySmallMax - quote.QuantityBefore)
		usefulBundles = (remaining + int(quote.BundleQuantity) - 1) / int(quote.BundleQuantity)
	}
	affordableBundles := int(quote.Gold) / quote.PricePerBundle
	// Both bounds fit the small inventory/gold ranges.
	expectedMax := uint16(min(usefulBundles, affordableBundles)) //nolint:gosec
	if quote.MaxBundles != expectedMax {
		panic("GuildPurchaseDialog: quote maximum is inconsistent with inventory or gold")
	}
}

func projectedGuildQuantity(quote game_state.GuildOfferQuote, bundleCount uint16) uint16 {
	quantity := uint32(quote.QuantityBefore) + uint32(quote.BundleQuantity)*uint32(bundleCount)

	// Saturation guarantees the conversion fits.
	return uint16(min(quantity, uint32(party_state.ItemQuantitySmallMax)))
}
