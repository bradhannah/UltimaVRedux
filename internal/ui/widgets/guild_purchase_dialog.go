package widgets

import (
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	guildPurchaseStartPercentX    = 0.28
	guildPurchaseEndPercentX      = 0.72
	guildPurchaseStartPercentY    = 0.14
	guildPurchaseEndPercentY      = 0.86
	guildPurchaseHeaderRatio      = 0.11
	guildPurchaseFooterRatio      = 0.17
	guildPurchaseMinimumInset     = 6
	guildPurchaseLineSpacing      = 1.12
	guildPurchaseMaximumLines     = 12
	guildPurchaseMinimumTextChars = 8
	guildPurchaseFooterLines      = 2
	guildPurchaseTotalLines       = 2
	guildPurchaseCenterDivisor    = 2
	guildPurchaseDoubleInset      = 2
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

	frame       *merchantModalFrame
	totalBounds image.Rectangle
	totalText   image.Rectangle

	headerOutput *text.Output
	bodyOutput   *text.Output
	totalOutput  *text.Output
	footerOutput *text.Output
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
	footerLines := 1
	if config.Quote.MaxBundles > 0 {
		bundleCount = 1
		footerLines = guildPurchaseFooterLines
	}
	var emptyBounds image.Rectangle
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
		frame: newMerchantModalFrame(merchantModalFrameConfig{
			placement:             guildPurchasePlacement(),
			headerRatio:           guildPurchaseHeaderRatio,
			footerRatio:           guildPurchaseFooterRatio,
			minimumInset:          guildPurchaseMinimumInset,
			lineSpacingMultiplier: guildPurchaseLineSpacing,
			footerMinimumLines:    footerLines,
		}),
		totalBounds:  emptyBounds,
		totalText:    emptyBounds,
		headerOutput: nil,
		bodyOutput:   nil,
		totalOutput:  nil,
		footerOutput: nil,
	}
	dialog.ensureLayout()

	return dialog
}

// Draw renders the nested purchase quantity modal.
func (d *GuildPurchaseDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
	if d.quote.MaxBundles > 0 {
		vector.StrokeLine(
			screen,
			float32(d.totalBounds.Min.X),
			float32(d.totalBounds.Min.Y),
			float32(d.totalBounds.Max.X),
			float32(d.totalBounds.Min.Y),
			float32(d.frame.pixelUnit()),
			DialogRuleLightColor,
			false,
		)
		d.totalOutput.DrawPanelContent(
			screen,
			image.Pt(d.totalText.Min.X, verticallyCenteredOutputY(d.totalText, d.totalOutput)),
			d.totalText.Dx(),
			etext.AlignCenter,
		)
	}
}

// Update processes keyboard-only quantity, confirmation, and cancellation input.
func (d *GuildPurchaseDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancelled = true
		d.Dismiss()

		return
	}
	if d.quote.MaxBundles == 0 {
		return
	}
	if d.handleQuantityInput() {
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

func (d *GuildPurchaseDialog) handleQuantityInput() bool {
	key, owned := d.pollQuantityInput()
	if key == nil {
		return owned
	}
	switch *key { //nolint:exhaustive // The quantity helper returns only this dialog's configured keys.
	case ebiten.KeyM:
		d.setBundleCount(d.quote.MaxBundles)
	case ebiten.KeyLeft, ebiten.KeyUp:
		d.setBundleCount(max(uint16(1), d.bundleCount-1))
	case ebiten.KeyRight, ebiten.KeyDown:
		d.setBundleCount(min(d.quote.MaxBundles, d.bundleCount+1))
	}

	return owned
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
	d.ensureLayout()

	return d.frame.bounds()
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

func (d *GuildPurchaseDialog) setBundleCount(count uint16) {
	if count == d.bundleCount {
		return
	}
	d.bundleCount = count
	d.populateOutputs()
}

func (d *GuildPurchaseDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	// The total strip is carved from the body first, so the body fits what remains.
	_, d.totalOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: guildPurchaseLineSpacing,
		MaxLines: guildPurchaseTotalLines, MaxChars: guildPurchaseMinimumTextChars,
	})
	d.calculateBodyTextLayout()
	d.headerOutput, d.bodyOutput, d.footerOutput = d.frame.newOutputs(merchantModalTextBudget{
		lineSpacing: guildPurchaseLineSpacing, maximumBodyLines: guildPurchaseMaximumLines,
		footerLines: guildPurchaseFooterLines, minimumChars: guildPurchaseMinimumTextChars, fixedBody: false,
	})
	if d.quote.MaxBundles > 0 {
		d.totalOutput.SetMaxCharsPerLine(max(guildPurchaseMinimumTextChars,
			d.totalOutput.Font.CharsForWidth(max(1, d.totalText.Dx()))))
	}
	d.populateOutputs()
}

func (d *GuildPurchaseDialog) calculateBodyTextLayout() {
	var emptyBounds image.Rectangle
	d.frame.bodyText = insetRectangle(d.frame.bodyBounds, d.frame.inset)
	d.totalBounds = emptyBounds
	d.totalText = emptyBounds
	if d.quote.MaxBundles == 0 {
		return
	}

	totalHeight := int(math.Ceil(d.totalOutput.GetTotalHeightForLines(guildPurchaseTotalLines))) +
		d.frame.inset*guildPurchaseDoubleInset
	totalHeight = min(totalHeight, d.frame.bodyText.Dy())
	d.totalBounds = image.Rect(
		d.frame.bodyText.Min.X,
		d.frame.bodyText.Max.Y-totalHeight,
		d.frame.bodyText.Max.X,
		d.frame.bodyText.Max.Y,
	)
	d.totalText = insetRectangle(d.totalBounds, d.frame.inset)
	d.frame.bodyText = image.Rect(
		d.frame.bodyText.Min.X,
		d.frame.bodyText.Min.Y,
		d.frame.bodyText.Max.X,
		max(d.frame.bodyText.Min.Y, d.totalBounds.Min.Y-d.frame.inset),
	)
}

func (d *GuildPurchaseDialog) populateOutputs() {
	if d.headerOutput == nil || d.bodyOutput == nil || d.totalOutput == nil || d.footerOutput == nil {
		return
	}
	d.headerOutput.Reset()
	d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text(strings.ToUpper("Purchase "+guildProvisionName(d.quote.Provision)), DialogHeaderColor).
		Done()

	d.bodyOutput.Reset()
	d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
		White(quotedGuildPitch(d.pitch)).WordWrap().Done()
	d.bodyOutput.AppendLine().White("").Done()
	if d.quote.MaxBundles == 0 {
		note, _ := guildSelectionNote(d.quote)
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			White(guildUnavailableMerchantResponse(d.quote)).WordWrap().Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text(note, DialogStatusWarningColor).Done()
	} else {
		projected := projectedGuildQuantity(d.quote, d.bundleCount)
		totalPrice := d.quote.PricePerBundle * int(d.bundleCount)
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).LightGray("BUNDLES (BEFORE THEFT)").Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			White("<   ").Yellowf("%d", d.bundleCount).White("   >").Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			LightGrayf("of %d", d.quote.MaxBundles).Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Whitef("%d %s", d.quote.BundleQuantity, strings.ToUpper(guildProvisionName(d.quote.Provision))).
			LightGray(" per bundle").Done()
		d.bodyOutput.AppendLine().White("").Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			LightGray(strings.ToUpper(guildProvisionName(d.quote.Provision))+"  ").
			Cyanf("%d", d.quote.QuantityBefore).
			White("  ->  ").
			LightGreenf("%d", projected).
			Done()
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			LightGray("GOLD  ").
			Cyanf("%d", d.quote.Gold).
			White("  ->  ").
			LightGreenf("%d", int(d.quote.Gold)-totalPrice).
			Done()
	}
	d.totalOutput.Reset()
	if d.quote.MaxBundles > 0 {
		totalPrice := d.quote.PricePerBundle * int(d.bundleCount)
		d.totalOutput.AppendLine().WithAlignment(etext.AlignCenter).Yellow("TOTAL PRICE").Done()
		d.totalOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Yellowf("%d", totalPrice).White(" GOLD").Done()
	}
	d.populateFooterOutput()
}

func (d *GuildPurchaseDialog) populateFooterOutput() {
	d.footerOutput.Reset()
	guidance := "[Esc] Back"
	if d.quote.MaxBundles > 0 {
		guidance = merchantQuantityFooter
	}
	appendControlGuidance(d.footerOutput, guidance, d.frame.footerText.Dx())
}

func quotedGuildPitch(pitch string) string {
	return `"` + strings.Trim(strings.TrimSpace(pitch), `"`) + `"`
}

func guildUnavailableMerchantResponse(quote game_state.GuildOfferQuote) string {
	if quote.QuantityBefore >= party_state.ItemQuantitySmallMax {
		return `"Thou canst carry no more ` + strings.ToLower(guildProvisionName(quote.Provision)) + `."`
	}

	return `"Thou hast not enough gold for even one bundle."`
}

func guildPurchasePlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: guildPurchaseStartPercentX,
		EndPercentX:   guildPurchaseEndPercentX,
		StartPercentY: guildPurchaseStartPercentY,
		EndPercentY:   guildPurchaseEndPercentY,
	}
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
