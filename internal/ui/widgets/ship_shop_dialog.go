package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	shipShopOfferCount          = 2
	shipShopMinimumInset        = 4
	shipShopInsetDivisor        = 64
	shipShopLineSpacing         = 1.06
	shipShopMinimumTextChars    = 8
	shipShopCardTextLines       = 3
	shipShopManifestLines       = 12
	shipShopFrigateSkiffs       = 2
	shipShopCenterDivisor       = 2
	shipShopShowroomHeightRatio = 0.43
)

var _ Widget = (*ShipShopDialog)(nil)

// ShipShopDialogPhase identifies the workflow-owned shipwright lifecycle.
type ShipShopDialogPhase byte

const (
	// ShipShopBrowse accepts offer selection, purchase review, harbour review, and leave input.
	ShipShopBrowse ShipShopDialogPhase = iota
	// ShipShopAwaitingPurchaseApproval blocks while the purchase modal owns input.
	ShipShopAwaitingPurchaseApproval
	// ShipShopAwaitingPurchaseResult blocks while purchase authority owns the request.
	ShipShopAwaitingPurchaseResult
	// ShipShopAwaitingTravelApproval blocks while the travel modal owns input.
	ShipShopAwaitingTravelApproval
	// ShipShopAwaitingTravelResult blocks while the workflow revalidates and performs travel.
	ShipShopAwaitingTravelResult
	// ShipShopSuccessReceipt holds a refreshed created-vessel receipt while dispatch owns input.
	ShipShopSuccessReceipt
	// ShipShopNonterminalRefusal requires acknowledgement before returning to Browse.
	ShipShopNonterminalRefusal
	// ShipShopFundsRefusal is the terminal insufficient-funds result.
	ShipShopFundsRefusal
	// ShipShopFarewell displays the merchant farewell before exact dismissal.
	ShipShopFarewell
	// ShipShopPreviewStopped displays the terminal Preview authority boundary.
	ShipShopPreviewStopped
	// ShipShopSessionError displays an unrecoverable session error.
	ShipShopSessionError
)

// ShipShopDialogProse contains finalized merchant copy supplied by the workflow.
type ShipShopDialogProse struct {
	Greeting               string
	Farewell               string
	BlockedAcknowledgement string
}

// ShipShopDialogConfig configures one detached, keyboard-only ship showroom.
type ShipShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	FrigateSprite  *ebiten.Image
	SkiffSprite    *ebiten.Image
	State          game_state.ShipShopState
	Prose          ShipShopDialogProse
	Preview        bool
	IsKeyPressed   func(ebiten.Key) bool
}

// ShipPurchaseIntent carries a trusted quote and comparison-only request into approval.
type ShipPurchaseIntent struct {
	Quote       game_state.ShipPurchaseQuote
	Request     game_state.ShipPurchaseRequest
	HarbourName string
	Travel      game_state.HarbourTravelReview
	Preview     bool
}

// ShipTravelIntent carries a trusted comparison-only harbour review into approval.
type ShipTravelIntent struct {
	Review  game_state.HarbourTravelReview
	Preview bool
}

// ShipShopResultDisplay contains finalized workflow text for a result or refusal.
type ShipShopResultDisplay struct {
	MerchantExchange string
	Receipt          string
}

// ShipShopDialog presents dual vessel cards and an authority-backed harbour manifest.
type ShipShopDialog struct {
	keyboard     *input.Keyboard
	isKeyPressed func(ebiten.Key) bool
	state        game_state.ShipShopState
	prose        ShipShopDialogProse
	preview      bool
	sprites      [shipShopOfferCount]*ebiten.Image
	selected     int
	phase        ShipShopDialogPhase
	dismissed    bool

	latestExchange string
	receiptText    string
	pendingBuy     *ShipPurchaseIntent
	pendingTravel  *ShipTravelIntent
	awaitingBuy    game_state.ShipPurchaseRequest
	awaitingTrip   game_state.HarbourTravelReview

	shell            *MerchantDialogShell
	cardOutputs      [shipShopOfferCount]*text.Output
	manifestOutput   *text.Output
	cardFont         *text.UltimaFont
	manifestFont     *text.UltimaFont
	workspace        image.Rectangle
	showroomBounds   image.Rectangle
	cardBounds       [shipShopOfferCount]image.Rectangle
	cardTitleBounds  [shipShopOfferCount]image.Rectangle
	cardSpriteBounds [shipShopOfferCount]image.Rectangle
	cardTextBounds   [shipShopOfferCount]image.Rectangle
	manifestBounds   image.Rectangle
	manifestText     image.Rectangle

	lastScreenWidth  int
	lastScreenHeight int
	directionsReady  bool
}

// NewShipShopDialog creates the concrete Showroom and Manifest presentation.
func NewShipShopDialog(config ShipShopDialogConfig) *ShipShopDialog {
	validateShipShopDialogConfig(config)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = ebiten.IsKeyPressed
	}
	dialog := &ShipShopDialog{ //nolint:exhaustruct_v5 // Layout and transient intent fields initialize on demand.
		keyboard:        config.Keyboard,
		isKeyPressed:    isKeyPressed,
		state:           config.State,
		prose:           config.Prose,
		preview:         config.Preview,
		sprites:         [shipShopOfferCount]*ebiten.Image{config.FrigateSprite, config.SkiffSprite},
		selected:        0,
		phase:           ShipShopBrowse,
		latestExchange:  config.Prose.Greeting,
		directionsReady: !isKeyPressed(ebiten.KeyLeft) && !isKeyPressed(ebiten.KeyRight),
	}
	dialog.shell = NewMerchantDialogShell(MerchantDialogShellConfig{
		Header: MerchantDialogHeaderView{
			ShopIdentity: "",
			Owner:        "",
			PartyGold:    "",
		},
		Exchange:       MerchantDialogExchangeView{Text: ""},
		Footer:         MerchantDialogFooterView{Guidance: ""},
		MerchantSprite: config.MerchantSprite,
	})
	dialog.ensureLayout()
	dialog.syncPresentation()

	return dialog
}

// Draw renders the shared shell, vessel cards, and manifest.
func (d *ShipShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	d.drawShowroom(screen)
	d.drawManifest(screen)
}

// Update processes keyboard-only parent input.
func (d *ShipShopDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}
	switch d.phase {
	case ShipShopBrowse:
		d.updateBrowse()
	case ShipShopNonterminalRefusal:
		d.updateRefusalAcknowledgement()
	case ShipShopSuccessReceipt:
		return
	case ShipShopFundsRefusal, ShipShopFarewell, ShipShopPreviewStopped, ShipShopSessionError:
		d.updateTerminalAcknowledgement()
	case ShipShopAwaitingPurchaseApproval, ShipShopAwaitingPurchaseResult,
		ShipShopAwaitingTravelApproval, ShipShopAwaitingTravelResult:
		return
	default:
		panic("ShipShopDialog.Update: unknown phase")
	}
}

func (d *ShipShopDialog) updateRefusalAcknowledgement() {
	if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.receiptText = ""
		d.transitionTo(ShipShopBrowse)
		d.syncPresentation()
	}
}

func (d *ShipShopDialog) updateTerminalAcknowledgement() {
	if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()
	}
}

func (d *ShipShopDialog) updateBrowse() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.latestExchange = d.prose.Farewell
		d.receiptText = ""
		d.transitionTo(ShipShopFarewell)
		d.syncPresentation()

		return
	}
	if d.handleSelectionInput() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyD) && d.travelAvailable() {
		d.emitTravelIntent()

		return
	}
	if d.enterPressed() {
		d.reviewSelectedOffer()
	}
}

// ConsumePendingPurchaseIntent returns and clears one trusted purchase approval intent.
func (d *ShipShopDialog) ConsumePendingPurchaseIntent() (ShipPurchaseIntent, bool) {
	if d.pendingBuy == nil {
		return emptyShipPurchaseIntent(), false
	}
	intent := *d.pendingBuy
	d.pendingBuy = nil

	return intent, true
}

// CancelPurchaseApproval returns to Browse without changing detached authority state.
func (d *ShipShopDialog) CancelPurchaseApproval() {
	if d.phase != ShipShopAwaitingPurchaseApproval {
		panic("ShipShopDialog.CancelPurchaseApproval: not awaiting purchase approval")
	}
	d.clearAwaitingPurchase()
	d.transitionTo(ShipShopBrowse)
	d.syncPresentation()
}

// AcceptPurchaseApproval records the exact modal request and awaits authority.
func (d *ShipShopDialog) AcceptPurchaseApproval(request game_state.ShipPurchaseRequest) {
	if d.phase != ShipShopAwaitingPurchaseApproval ||
		!shipPurchaseRequestsShareReview(request, d.awaitingBuy) ||
		!shipRequestMatchesApproval(request, d.GetSelectedQuote()) {
		panic("ShipShopDialog.AcceptPurchaseApproval: request does not match pending approval")
	}
	d.pendingBuy = nil
	d.awaitingBuy = request
	d.transitionTo(ShipShopAwaitingPurchaseResult)
	d.syncPresentation()
}

// ConsumePendingTravelIntent returns and clears one trusted harbour approval intent.
func (d *ShipShopDialog) ConsumePendingTravelIntent() (ShipTravelIntent, bool) {
	if d.pendingTravel == nil {
		return emptyShipTravelIntent(), false
	}
	intent := *d.pendingTravel
	d.pendingTravel = nil

	return intent, true
}

// CancelTravelApproval returns to Browse without performing travel.
func (d *ShipShopDialog) CancelTravelApproval() {
	if d.phase != ShipShopAwaitingTravelApproval {
		panic("ShipShopDialog.CancelTravelApproval: not awaiting travel approval")
	}
	d.clearAwaitingTravel()
	d.transitionTo(ShipShopBrowse)
	d.syncPresentation()
}

// AcceptTravelApproval records the exact reviewed travel and awaits workflow authority.
func (d *ShipShopDialog) AcceptTravelApproval(review game_state.HarbourTravelReview) {
	if d.phase != ShipShopAwaitingTravelApproval || review != d.awaitingTrip {
		panic("ShipShopDialog.AcceptTravelApproval: review does not match pending travel")
	}
	d.pendingTravel = nil
	d.transitionTo(ShipShopAwaitingTravelResult)
	d.syncPresentation()
}

// AcceptDispatchTravel records a first-delivery dispatch choice and awaits workflow authority.
func (d *ShipShopDialog) AcceptDispatchTravel(review game_state.HarbourTravelReview) {
	if d.phase != ShipShopSuccessReceipt || review != d.state.Travel ||
		review.BlockReason.IsBlocked() || review.Fingerprint == "" {
		panic("ShipShopDialog.AcceptDispatchTravel: review does not match available refreshed travel")
	}
	d.awaitingTrip = review
	d.transitionTo(ShipShopAwaitingTravelResult)
	d.syncPresentation()
}

// ApplyPurchaseResult installs a complete refreshed snapshot and shows its success receipt.
func (d *ShipShopDialog) ApplyPurchaseResult(
	result game_state.ShipPurchaseResult,
	refreshed game_state.ShipShopState,
	display ShipShopResultDisplay,
) {
	if d.phase != ShipShopAwaitingPurchaseResult ||
		!shipPurchaseResultIdentityMatches(result, d.awaitingBuy) ||
		!shipPurchaseResultEconomicsMatch(result, d.awaitingBuy) {
		panic("ShipShopDialog.ApplyPurchaseResult: result does not match accepted request")
	}
	if result.Quantity < d.awaitingBuy.Quantity {
		display.Receipt += fmt.Sprintf(" Batch stopped: %d of %d purchased.", result.Quantity, d.awaitingBuy.Quantity)
	}
	validateShipShopResultDisplay(display)
	d.applyRefresh(refreshed, result.VehicleType)
	if refreshed.Gold != result.GoldAfterTheft {
		panic("ShipShopDialog.ApplyPurchaseResult: refreshed gold does not match result")
	}
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaitingPurchase()
	if result.Action == game_state.ShipPurchaseLoadSkiff {
		d.transitionTo(ShipShopBrowse)
	} else {
		d.transitionTo(ShipShopSuccessReceipt)
	}
	d.syncPresentation()
}

// ApplyPurchaseRefusal installs refreshed state and waits for a nonterminal acknowledgement.
func (d *ShipShopDialog) ApplyPurchaseRefusal(
	refreshed game_state.ShipShopState,
	display ShipShopResultDisplay,
) {
	if d.phase != ShipShopAwaitingPurchaseResult {
		panic("ShipShopDialog.ApplyPurchaseRefusal: not awaiting purchase authority")
	}
	validateShipShopResultDisplay(display)
	selected := d.awaitingBuy.VehicleType
	d.applyRefresh(refreshed, selected)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaitingPurchase()
	d.transitionTo(ShipShopNonterminalRefusal)
	d.syncPresentation()
}

// ApplyFundsRefusal displays a terminal insufficient-funds result.
func (d *ShipShopDialog) ApplyFundsRefusal(display ShipShopResultDisplay) {
	if d.phase != ShipShopAwaitingPurchaseResult {
		panic("ShipShopDialog.ApplyFundsRefusal: not awaiting purchase authority")
	}
	validateShipShopResultDisplay(display)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaitingPurchase()
	d.transitionTo(ShipShopFundsRefusal)
	d.syncPresentation()
}

// ApplyTravelRefusal refreshes the manifest and waits for acknowledgement before Browse.
func (d *ShipShopDialog) ApplyTravelRefusal(
	refreshed game_state.ShipShopState,
	display ShipShopResultDisplay,
) {
	if d.phase != ShipShopAwaitingTravelResult {
		panic("ShipShopDialog.ApplyTravelRefusal: not awaiting travel authority")
	}
	validateShipShopResultDisplay(display)
	d.applyRefresh(refreshed, d.GetSelectedVehicle())
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaitingTravel()
	d.transitionTo(ShipShopNonterminalRefusal)
	d.syncPresentation()
}

// ApplyTravelCompleted exactly dismisses the shop after the reviewed travel succeeds.
func (d *ShipShopDialog) ApplyTravelCompleted(review game_state.HarbourTravelReview) {
	if d.phase != ShipShopAwaitingTravelResult || review != d.awaitingTrip {
		panic("ShipShopDialog.ApplyTravelCompleted: travel does not match accepted review")
	}
	d.clearAwaitingTravel()
	d.Dismiss()
}

// ApplyPreviewStopped records a purchase Preview boundary without invoking authority.
func (d *ShipShopDialog) ApplyPreviewStopped(request game_state.ShipPurchaseRequest) {
	if d.phase != ShipShopAwaitingPurchaseResult || request != d.awaitingBuy {
		panic("ShipShopDialog.ApplyPreviewStopped: request does not match accepted preview")
	}
	d.latestExchange = "Preview stopped before ship purchase authority."
	d.receiptText = "Preview only: no vessel was purchased and party gold is unchanged."
	d.clearAwaitingPurchase()
	d.transitionTo(ShipShopPreviewStopped)
	d.syncPresentation()
}

// ApplyTravelPreviewStopped records a travel Preview boundary without performing transition work.
func (d *ShipShopDialog) ApplyTravelPreviewStopped(review game_state.HarbourTravelReview) {
	if d.phase != ShipShopAwaitingTravelResult || review != d.awaitingTrip {
		panic("ShipShopDialog.ApplyTravelPreviewStopped: review does not match accepted preview")
	}
	d.latestExchange = "Preview stopped before harbour travel."
	d.receiptText = "Preview only: the party remains in town and no time was advanced."
	d.clearAwaitingTravel()
	d.transitionTo(ShipShopPreviewStopped)
	d.syncPresentation()
}

// ApplySessionError disables all actions until the terminal message is dismissed.
func (d *ShipShopDialog) ApplySessionError(message string) {
	validateShipWidgetText("session error", message)
	d.pendingBuy = nil
	d.pendingTravel = nil
	d.clearAwaitingPurchase()
	d.clearAwaitingTravel()
	d.latestExchange = message
	d.receiptText = message
	d.transitionTo(ShipShopSessionError)
	d.syncPresentation()
}

// ContinueShoppingAfterSuccess returns the refreshed parent to Browse.
func (d *ShipShopDialog) ContinueShoppingAfterSuccess() {
	if d.phase != ShipShopSuccessReceipt {
		panic("ShipShopDialog.ContinueShoppingAfterSuccess: no success receipt is active")
	}
	d.receiptText = ""
	d.transitionTo(ShipShopBrowse)
	d.syncPresentation()
}

// GetSelectedVehicle returns the stable selected vessel identity.
func (d *ShipShopDialog) GetSelectedVehicle() references.VehicleType {
	if d.selected == 0 {
		return references.FrigateVehicle
	}

	return references.SkiffVehicle
}

// GetSelectedQuote returns the trusted quote for the selected card.
func (d *ShipShopDialog) GetSelectedQuote() game_state.ShipPurchaseQuote {
	if d.selected == 0 {
		return d.state.FrigateQuote
	}

	return d.state.SkiffQuote
}

// GetShell returns the shared 27/73 merchant shell.
func (d *ShipShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// IsDismissed reports whether the parent can be removed exactly once.
func (d *ShipShopDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the parent without emitting another intent.
func (d *ShipShopDialog) Dismiss() { d.dismissed = true }

func (d *ShipShopDialog) handleSelectionInput() bool {
	leftHeld := d.isKeyPressed(ebiten.KeyLeft)
	rightHeld := d.isKeyPressed(ebiten.KeyRight)
	if !d.directionsReady {
		if leftHeld || rightHeld {
			return false
		}
		d.directionsReady = true
	}
	if leftHeld && rightHeld {
		return false
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) {
		d.setSelected(0)

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight) {
		d.setSelected(1)

		return true
	}

	return false
}

func (d *ShipShopDialog) setSelected(index int) {
	index = max(0, min(shipShopOfferCount-1, index))
	if d.selected == index {
		return
	}
	d.selected = index
	d.receiptText = ""
	d.populateOutputs()
}

func (d *ShipShopDialog) reviewSelectedOffer() {
	quote := d.GetSelectedQuote()
	if quote.BlockReason.IsBlocked() {
		d.latestExchange = d.prose.BlockedAcknowledgement
		d.receiptText = shipBlockConsequence(quote)
		d.transitionTo(ShipShopNonterminalRefusal)
		d.syncPresentation()

		return
	}
	d.receiptText = ""
	request := shipPurchaseRequestFromState(d.state, quote)
	intent := ShipPurchaseIntent{
		Quote:       quote,
		Request:     request,
		HarbourName: d.state.Travel.HarbourName,
		Travel:      d.state.Travel,
		Preview:     d.preview,
	}
	d.pendingBuy = &intent
	d.awaitingBuy = request
	d.transitionTo(ShipShopAwaitingPurchaseApproval)
	d.syncPresentation()
}

func (d *ShipShopDialog) emitTravelIntent() {
	if d.phase != ShipShopBrowse || d.pendingTravel != nil || !d.travelAvailable() {
		return
	}
	intent := ShipTravelIntent{Review: d.state.Travel, Preview: d.preview}
	d.pendingTravel = &intent
	d.awaitingTrip = d.state.Travel
	d.transitionTo(ShipShopAwaitingTravelApproval)
	d.syncPresentation()
}

func (d *ShipShopDialog) travelAvailable() bool {
	return !d.state.Travel.BlockReason.IsBlocked() && d.state.Travel.Fingerprint != ""
}

func (d *ShipShopDialog) applyRefresh(state game_state.ShipShopState, selected references.VehicleType) {
	validateShipShopState(state)
	if state.Merchant.ID != d.state.Merchant.ID {
		panic("ShipShopDialog: refreshed state does not match active merchant")
	}
	d.state = state
	if selected == references.SkiffVehicle {
		d.selected = 1
	} else {
		d.selected = 0
	}
	d.calculateLayout()
}

func (d *ShipShopDialog) clearAwaitingPurchase() {
	d.pendingBuy = nil
	d.awaitingBuy = emptyShipPurchaseRequest()
}

func (d *ShipShopDialog) clearAwaitingTravel() {
	d.pendingTravel = nil
	d.awaitingTrip = emptyHarbourTravelReview()
}

func (d *ShipShopDialog) transitionTo(phase ShipShopDialogPhase) {
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
	d.directionsReady = !d.isKeyPressed(ebiten.KeyLeft) && !d.isKeyPressed(ebiten.KeyRight)
}

func (d *ShipShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *ShipShopDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return
	}
	if d.manifestOutput != nil && width == d.lastScreenWidth && height == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth, d.lastScreenHeight = width, height
	d.shell.GetBounds()
	d.cardFont, d.cardOutputs[0] = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: shipShopLineSpacing,
		MaxLines: shipShopCardTextLines, MaxChars: shipShopMinimumTextChars,
	})
	_, d.cardOutputs[1] = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: shipShopLineSpacing,
		MaxLines: shipShopCardTextLines, MaxChars: shipShopMinimumTextChars,
	})
	d.manifestFont, d.manifestOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: shipShopLineSpacing,
		MaxLines: shipShopManifestLines, MaxChars: shipShopMinimumTextChars,
	})
	d.calculateLayout()
	d.populateOutputs()
}

func (d *ShipShopDialog) calculateLayout() {
	if d.manifestOutput == nil {
		return
	}
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(shipShopMinimumInset, min(workspace.Dx(), workspace.Dy())/shipShopInsetDivisor)
	d.workspace = insetRectangle(workspace, inset)
	gap := inset
	showroomHeight := max(1, int(math.Round(float64(d.workspace.Dy())*shipShopShowroomHeightRatio)))
	d.showroomBounds = image.Rect(
		d.workspace.Min.X, d.workspace.Min.Y, d.workspace.Max.X,
		min(d.workspace.Max.Y, d.workspace.Min.Y+showroomHeight),
	)
	cardWidth := max(1, (d.showroomBounds.Dx()-gap)/shipShopOfferCount)
	for index := range shipShopOfferCount {
		left := d.showroomBounds.Min.X + index*(cardWidth+gap)
		right := left + cardWidth
		if index == shipShopOfferCount-1 {
			right = d.showroomBounds.Max.X
		}
		d.cardBounds[index] = image.Rect(left, d.showroomBounds.Min.Y, right, d.showroomBounds.Max.Y)
		d.calculateCardLayout(index, inset)
	}
	d.manifestBounds = image.Rect(
		d.workspace.Min.X, min(d.workspace.Max.Y, d.showroomBounds.Max.Y+gap),
		d.workspace.Max.X, d.workspace.Max.Y,
	)
	d.manifestText = insetRectangle(d.manifestBounds, inset)
	d.manifestOutput.SetMaxCharsPerLine(max(
		shipShopMinimumTextChars,
		d.manifestFont.CharsForWidth(max(1, d.manifestText.Dx())),
	))
}

func (d *ShipShopDialog) calculateCardLayout(index, inset int) {
	content := insetRectangle(d.cardBounds[index], inset)
	lineHeight := max(1, int(math.Ceil(d.cardOutputs[index].GetFontHeight())))
	d.cardTitleBounds[index] = image.Rect(content.Min.X, content.Min.Y, content.Max.X,
		min(content.Max.Y, content.Min.Y+lineHeight))
	textHeight := min(content.Dy(), int(math.Ceil(
		d.cardOutputs[index].GetTotalHeightForLines(shipShopCardTextLines),
	)))
	d.cardTextBounds[index] = image.Rect(content.Min.X, max(content.Min.Y, content.Max.Y-textHeight),
		content.Max.X, content.Max.Y)
	d.cardSpriteBounds[index] = image.Rect(content.Min.X, d.cardTitleBounds[index].Max.Y,
		content.Max.X, max(d.cardTitleBounds[index].Max.Y, d.cardTextBounds[index].Min.Y))
	d.cardOutputs[index].SetMaxCharsPerLine(max(
		shipShopMinimumTextChars,
		d.cardFont.CharsForWidth(max(1, d.cardTextBounds[index].Dx())),
	))
}

func (d *ShipShopDialog) drawShowroom(screen *ebiten.Image) {
	for index := range shipShopOfferCount {
		bounds := d.cardBounds[index]
		background := MerchantDialogBackgroundColor
		if index == d.selected && d.phase == ShipShopBrowse {
			background = DialogSelectedBackground
		}
		vector.FillRect(screen, float32(bounds.Min.X), float32(bounds.Min.Y),
			float32(bounds.Dx()), float32(bounds.Dy()), background, false)
		DrawClassicPixelRoundedFrame(screen, bounds, d.pixelUnit(), DialogRuleLightColor)
		d.drawCardSprite(screen, index)
		d.cardOutputs[index].DrawPanelContent(
			screen,
			image.Pt(d.cardTextBounds[index].Min.X,
				verticallyCenteredOutputY(d.cardTextBounds[index], d.cardOutputs[index])),
			max(1, d.cardTextBounds[index].Dx()),
			etext.AlignCenter,
		)
	}
}

func (d *ShipShopDialog) drawCardSprite(screen *ebiten.Image, index int) {
	sprite := d.sprites[index]
	target := d.cardSpriteBounds[index]
	if sprite == nil || sprite.Bounds().Empty() || target.Empty() {
		return
	}
	source := sprite.Bounds()
	scale := min(float64(target.Dx())/float64(source.Dx()), float64(target.Dy())/float64(source.Dy()))
	if scale <= 0 {
		return
	}
	options := new(ebiten.DrawImageOptions)
	options.Filter = ebiten.FilterNearest
	options.GeoM.Translate(float64(-source.Min.X), float64(-source.Min.Y))
	options.GeoM.Scale(scale, scale)
	options.GeoM.Translate(
		float64(target.Min.X)+(float64(target.Dx())-float64(source.Dx())*scale)/shipShopCenterDivisor,
		float64(target.Min.Y)+(float64(target.Dy())-float64(source.Dy())*scale)/shipShopCenterDivisor,
	)
	screen.DrawImage(sprite, options)
}

func (d *ShipShopDialog) drawManifest(screen *ebiten.Image) {
	drawMerchantPane(screen, d.manifestBounds, d.pixelUnit())
	d.manifestOutput.DrawPanelContent(screen, d.manifestText.Min, max(1, d.manifestText.Dx()), etext.AlignStart)
}

func (d *ShipShopDialog) populateOutputs() {
	if d.cardOutputs[0] == nil || d.manifestOutput == nil {
		return
	}
	d.populateCard(0, d.state.FrigateQuote)
	d.populateCard(1, d.state.SkiffQuote)
	d.populateManifest()
}

func (d *ShipShopDialog) populateCard(index int, quote game_state.ShipPurchaseQuote) {
	output := d.cardOutputs[index]
	output.Reset()
	selected := index == d.selected && d.phase == ShipShopBrowse
	nameColor, valueColor := shipCardColors(quote, selected)
	name := shipVehicleName(quote.VehicleType)
	output.AppendLine().WithAlignment(etext.AlignCenter).Text(name, nameColor).Done()
	if quote.VehicleType == references.FrigateVehicle && shipBerthHasFrigate(d.state.Berth) {
		output.AppendLine().WithAlignment(etext.AlignCenter).Text("AT BERTH", valueColor).Done()
		escort := "Escort unavailable"
		if d.travelAvailable() {
			escort = "[D] Escort"
		}
		output.AppendLine().WithAlignment(etext.AlignCenter).Text(escort, valueColor).Done()

		return
	}
	if quote.VehicleType == references.FrigateVehicle {
		output.AppendLine().WithAlignment(etext.AlignCenter).
			Textf("Includes %d skiffs", valueColor, shipShopFrigateSkiffs).Done()
	} else {
		output.AppendLine().WithAlignment(etext.AlignCenter).Text("Harbour tender", valueColor).Done()
	}
	price := fmt.Sprintf("%d gold", quote.Price)
	if quote.Action == game_state.ShipPurchaseLoadSkiff {
		price += " each"
	}
	output.AppendLine().WithAlignment(etext.AlignCenter).Text(price, valueColor).Done()
}

func (d *ShipShopDialog) populateManifest() {
	d.manifestOutput.Reset()
	quote := d.GetSelectedQuote()
	d.manifestOutput.AppendLine().Text("MANIFEST", DialogHeaderColor).Done()
	if d.preview {
		d.manifestOutput.AppendLine().Text("PREVIEW", DialogStatusWarningColor).Done()
	}
	d.manifestOutput.AppendLine().LightGray("Harbour: ").White(d.state.Travel.HarbourName).Done()
	d.manifestOutput.AppendLine().LightGray("Berth: ").White(shipBerthLabel(d.state.Berth)).Done()
	d.manifestOutput.AppendLine().LightGray("Vessel: ").White(shipBerthVehicleLabel(d.state.Berth)).Done()
	if d.state.Berth.VehicleType == references.FrigateVehicle {
		d.manifestOutput.AppendLine().LightGray("Skiffs aboard: ").Cyanf(
			"%d/%d", d.state.Berth.SkiffCount, game_state.ShipShopMaximumSkiffs,
		).Done()
	}
	frigateAtBerth := quote.VehicleType == references.FrigateVehicle && shipBerthHasFrigate(d.state.Berth)
	if !frigateAtBerth {
		d.manifestOutput.AppendLine().LightGray("Action: ").Yellow(shipActionLabel(quote.Action)).Done()
		consequence := shipQuoteConsequence(quote, d.state.Travel.HarbourName)
		color := DialogValueColor
		if quote.BlockReason.IsBlocked() {
			color = DialogStatusWarningColor
		}
		d.manifestOutput.AppendLine().Text(consequence, color).WordWrap().Done()
	}
	escort := fmt.Sprintf("Escort unavailable - %d minutes", d.state.Travel.TimeCostMinutes)
	escortColor := DialogEmptyColor
	if d.travelAvailable() {
		escort = fmt.Sprintf("[D] Escort to vessel - %d minutes", d.state.Travel.TimeCostMinutes)
		escortColor = DialogDeltaPositive
	}
	d.manifestOutput.AppendLine().Text(escort, escortColor).Done()
	if d.receiptText != "" {
		d.manifestOutput.Reset()
		d.manifestOutput.AppendLine().Text("MANIFEST", DialogHeaderColor).Done()
		d.manifestOutput.AppendLine().Text(d.receiptText, colorForShipPhase(d.phase)).WordWrap().Done()
		if d.phase == ShipShopBrowse && d.state.Berth.VehicleType == references.FrigateVehicle {
			d.manifestOutput.AppendLine().LightGray("Skiffs aboard: ").Cyanf(
				"%d/%d", d.state.Berth.SkiffCount, game_state.ShipShopMaximumSkiffs,
			).Done()
			d.manifestOutput.AppendLine().Text(escort, escortColor).Done()
		}
	}
}

func (d *ShipShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateOutputs()
}

func (d *ShipShopDialog) footerGuidance() string {
	if d.phase == ShipShopBrowse {
		return d.browseFooterGuidance()
	}
	if shipShopTerminalPhase(d.phase) {
		return merchantLeaveFooter
	}

	return shipShopActiveFooterGuidance(d.phase)
}

func shipShopActiveFooterGuidance(phase ShipShopDialogPhase) string {
	if phase == ShipShopAwaitingPurchaseApproval {
		return "Complete or cancel purchase review"
	}
	if phase == ShipShopAwaitingPurchaseResult {
		return "Processing vessel order"
	}
	if phase == ShipShopAwaitingTravelApproval {
		return "Complete or cancel harbour review"
	}
	if phase == ShipShopAwaitingTravelResult {
		return "Preparing harbour escort"
	}
	if phase == ShipShopSuccessReceipt {
		return "Await vessel dispatch acknowledgement"
	}
	if phase == ShipShopNonterminalRefusal {
		return "[Enter/Esc] Acknowledge"
	}

	panic("shipShopActiveFooterGuidance: inactive phase")
}

func (d *ShipShopDialog) browseFooterGuidance() string {
	if d.travelAvailable() {
		return "[Left/Right] Select   [Enter] Review\n[D] Escort   [Esc] Leave"
	}

	return "[Left/Right] Select   [Enter] Review   [Esc] Leave"
}

func shipShopTerminalPhase(phase ShipShopDialogPhase) bool {
	switch phase {
	case ShipShopFundsRefusal, ShipShopFarewell, ShipShopPreviewStopped, ShipShopSessionError:
		return true
	case ShipShopBrowse, ShipShopAwaitingPurchaseApproval, ShipShopAwaitingPurchaseResult,
		ShipShopAwaitingTravelApproval, ShipShopAwaitingTravelResult, ShipShopSuccessReceipt,
		ShipShopNonterminalRefusal:
		return false
	default:
		panic("shipShopTerminalPhase: unknown phase")
	}
}

func (d *ShipShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func shipCardColors(quote game_state.ShipPurchaseQuote, selected bool) (stdcolor.Color, stdcolor.Color) {
	if selected {
		return DialogSelectedText, DialogSelectedText
	}
	if quote.BlockReason.IsBlocked() {
		return DialogEmptyColor, DialogEmptyColor
	}
	if !quote.Affordable {
		return DialogItemNameColor, DialogStatusWarningColor
	}

	return DialogItemNameColor, DialogValueColor
}

func colorForShipPhase(phase ShipShopDialogPhase) stdcolor.Color {
	if phase == ShipShopSuccessReceipt {
		return DialogDeltaPositive
	}

	return DialogStatusWarningColor
}

func shipVehicleName(vehicle references.VehicleType) string {
	switch vehicle {
	case references.FrigateVehicle:
		return "Frigate"
	case references.SkiffVehicle:
		return "Skiff"
	case references.NoPartyVehicle, references.CarpetVehicle, references.HorseVehicle, references.NPC:
		return "None"
	default:
		panic("shipVehicleName: unknown vehicle type")
	}
}

func shipBerthLabel(berth game_state.ShipBerthReview) string {
	switch berth.State {
	case game_state.ShipBerthEmpty:
		return "Empty"
	case game_state.ShipBerthPendingFrigate, game_state.ShipBerthPendingSkiff:
		return "Pending arrival"
	case game_state.ShipBerthMooredFrigate, game_state.ShipBerthMooredSkiff:
		return "Moored"
	case game_state.ShipBerthObstructed:
		return "Obstructed"
	default:
		panic("shipBerthLabel: unknown berth state")
	}
}

func shipBerthVehicleLabel(berth game_state.ShipBerthReview) string {
	if berth.State == game_state.ShipBerthObstructed {
		return "Unrelated obstruction"
	}
	if berth.State == game_state.ShipBerthEmpty {
		return "None"
	}

	return shipVehicleName(berth.VehicleType)
}

func shipActionLabel(action game_state.ShipPurchaseAction) string {
	switch action {
	case game_state.ShipPurchaseCreateFrigate:
		return "Dispatch new frigate"
	case game_state.ShipPurchaseCreateStandaloneSkiff:
		return "Dispatch standalone skiff"
	case game_state.ShipPurchaseLoadSkiff:
		return "Load skiffs aboard frigate"
	default:
		panic("shipActionLabel: unknown action")
	}
}

func shipQuoteConsequence(quote game_state.ShipPurchaseQuote, harbourName string) string {
	if quote.BlockReason.IsBlocked() {
		return shipBlockConsequence(quote)
	}
	switch quote.Action {
	case game_state.ShipPurchaseCreateFrigate:
		return fmt.Sprintf("Includes %d skiffs; arrival at %s.", shipShopFrigateSkiffs, harbourName)
	case game_state.ShipPurchaseCreateStandaloneSkiff:
		return "One standalone skiff will arrive at this harbour."
	case game_state.ShipPurchaseLoadSkiff:
		return fmt.Sprintf("Load one or more skiffs aboard the current %d/%d frigate.",
			quote.Berth.SkiffCount, game_state.ShipShopMaximumSkiffs)
	default:
		panic("shipQuoteConsequence: unknown action")
	}
}

func shipBlockConsequence(quote game_state.ShipPurchaseQuote) string {
	switch quote.BlockReason {
	case game_state.ShipPurchaseBlockPendingOrderElsewhere:
		return "Another harbour order must arrive first."
	case game_state.ShipPurchaseBlockBerthOccupied:
		return "The berth is obstructed."
	case game_state.ShipPurchaseBlockStandaloneSkiff:
		return "A standalone skiff already holds this berth."
	case game_state.ShipPurchaseBlockFrigateAlreadyPresent:
		return "A frigate already holds this berth."
	case game_state.ShipPurchaseBlockMapCapacity:
		return "No safe delivery can be arranged."
	case game_state.ShipPurchaseBlockSkiffLimit:
		return fmt.Sprintf("This frigate already carries %d/%d skiffs.",
			quote.Berth.SkiffCount, game_state.ShipShopMaximumSkiffs)
	case game_state.ShipPurchaseBlockNone:
		return ""
	default:
		panic("shipBlockConsequence: unknown block reason")
	}
}

func shipPurchaseRequestFromState(
	state game_state.ShipShopState,
	quote game_state.ShipPurchaseQuote,
) game_state.ShipPurchaseRequest {
	if len(quote.ApprovalViews) == 0 {
		panic("shipPurchaseRequestFromState: available approval is required")
	}

	return shipPurchaseRequestForApproval(state, quote, quote.ApprovalViews[0])
}

func shipPurchaseRequestForApproval(
	state game_state.ShipShopState,
	quote game_state.ShipPurchaseQuote,
	approval game_state.ShipPurchaseApprovalView,
) game_state.ShipPurchaseRequest {
	return game_state.ShipPurchaseRequest{
		ReviewedMerchantID:            state.Merchant.ID,
		ReviewedPricingCharacterIndex: state.PricingCharacterIndex,
		VehicleType:                   quote.VehicleType,
		Action:                        quote.Action,
		Quantity:                      approval.Quantity,
		ReviewedPrice:                 quote.Price,
		ReviewedTotalPrice:            approval.TotalPrice,
		ReviewedGoldBefore:            quote.GoldBefore,
		ReviewedFingerprint:           quote.RequestFingerprint,
	}
}

func shipPurchaseRequestsShareReview(first, second game_state.ShipPurchaseRequest) bool {
	return first.ReviewedMerchantID == second.ReviewedMerchantID &&
		first.ReviewedPricingCharacterIndex == second.ReviewedPricingCharacterIndex &&
		first.VehicleType == second.VehicleType && first.Action == second.Action &&
		first.ReviewedPrice == second.ReviewedPrice &&
		first.ReviewedGoldBefore == second.ReviewedGoldBefore &&
		first.ReviewedFingerprint == second.ReviewedFingerprint
}

func shipPurchaseResultIdentityMatches(
	result game_state.ShipPurchaseResult,
	request game_state.ShipPurchaseRequest,
) bool {
	return result.MerchantID == request.ReviewedMerchantID &&
		result.PricingCharacterIndex == request.ReviewedPricingCharacterIndex &&
		result.VehicleType == request.VehicleType && result.Action == request.Action
}

func shipPurchaseResultEconomicsMatch(
	result game_state.ShipPurchaseResult,
	request game_state.ShipPurchaseRequest,
) bool {
	return result.Quantity > 0 && result.Quantity <= request.Quantity && result.UnitPrice == request.ReviewedPrice &&
		result.Price == result.Quantity*request.ReviewedPrice && result.GoldBefore == request.ReviewedGoldBefore
}

func shipRequestMatchesApproval(request game_state.ShipPurchaseRequest, quote game_state.ShipPurchaseQuote) bool {
	if request.Quantity <= 0 || request.Quantity > len(quote.ApprovalViews) {
		return false
	}
	approval := quote.ApprovalViews[request.Quantity-1]

	return request.ReviewedTotalPrice == approval.TotalPrice
}

func shipBerthHasFrigate(berth game_state.ShipBerthReview) bool {
	return berth.State == game_state.ShipBerthPendingFrigate || berth.State == game_state.ShipBerthMooredFrigate
}

func emptyShipPurchaseIntent() ShipPurchaseIntent {
	return ShipPurchaseIntent{
		Quote:       emptyShipPurchaseQuote(),
		Request:     emptyShipPurchaseRequest(),
		HarbourName: "",
		Travel:      emptyHarbourTravelReview(),
		Preview:     false,
	}
}

func emptyShipTravelIntent() ShipTravelIntent {
	return ShipTravelIntent{Review: emptyHarbourTravelReview(), Preview: false}
}

func emptyShipPurchaseQuote() game_state.ShipPurchaseQuote {
	return game_state.ShipPurchaseQuote{
		VehicleType:        references.NoPartyVehicle,
		Action:             game_state.ShipPurchaseCreateFrigate,
		BasePrice:          0,
		Price:              0,
		GoldBefore:         0,
		GoldAfterPayment:   0,
		Affordable:         false,
		BlockReason:        game_state.ShipPurchaseBlockNone,
		Berth:              emptyShipBerthReview(),
		RequestFingerprint: "",
		ApprovalViews:      nil,
	}
}

func emptyShipPurchaseRequest() game_state.ShipPurchaseRequest {
	return game_state.ShipPurchaseRequest{
		ReviewedMerchantID:            "",
		ReviewedPricingCharacterIndex: 0,
		VehicleType:                   references.NoPartyVehicle,
		Action:                        game_state.ShipPurchaseCreateFrigate,
		Quantity:                      0,
		ReviewedPrice:                 0,
		ReviewedTotalPrice:            0,
		ReviewedGoldBefore:            0,
		ReviewedFingerprint:           "",
	}
}

func emptyShipBerthReview() game_state.ShipBerthReview {
	return game_state.ShipBerthReview{
		Location:       references.Location(0),
		Position:       references.Position{X: 0, Y: 0},
		State:          game_state.ShipBerthEmpty,
		VehicleType:    references.NoPartyVehicle,
		Pending:        false,
		SkiffCount:     0,
		MapUnitCount:   0,
		HasMapCapacity: false,
	}
}

func emptyHarbourTravelReview() game_state.HarbourTravelReview {
	return game_state.HarbourTravelReview{
		MerchantID:      "",
		DockLocation:    references.Location(0),
		HarbourName:     "",
		BerthPosition:   references.Position{X: 0, Y: 0},
		ArrivalPosition: references.Position{X: 0, Y: 0},
		VehicleType:     references.NoPartyVehicle,
		PendingDelivery: false,
		TimeCostMinutes: 0,
		BlockReason:     game_state.HarbourTravelBlockNone,
		Fingerprint:     "",
	}
}

func validateShipShopDialogConfig(config ShipShopDialogConfig) {
	if config.Keyboard == nil {
		panic("ShipShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil || config.FrigateSprite == nil || config.SkiffSprite == nil {
		panic("ShipShopDialog: merchant, frigate, and skiff sprites are required")
	}
	validateShipShopState(config.State)
	validateShipWidgetText("greeting", config.Prose.Greeting)
	validateShipWidgetText("farewell", config.Prose.Farewell)
	validateShipWidgetText("blocked acknowledgement", config.Prose.BlockedAcknowledgement)
}

func validateShipShopState(state game_state.ShipShopState) {
	validateShipShopIdentity(state)
	validateShipQuote(state, state.FrigateQuote, references.FrigateVehicle)
	validateShipQuote(state, state.SkiffQuote, references.SkiffVehicle)
	validateShipBerth(state.Berth)
	validateShipTravelReview(state.Travel, false)
	validateShipShopHarbourState(state)
}

func validateShipShopIdentity(state game_state.ShipShopState) {
	validateShipMerchantIdentity(state)
	validateShipPricingIdentity(state)
	validateShipHarbourIdentity(state)
}

func validateShipMerchantIdentity(state game_state.ShipShopState) {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyShip ||
		strings.TrimSpace(state.Merchant.OwnerName) == "" || strings.TrimSpace(state.Merchant.ShopName) == "" {
		panic("ShipShopDialog: complete detached ship merchant is required")
	}
}

func validateShipPricingIdentity(state game_state.ShipShopState) {
	if state.PricingCharacterIndex < 0 || strings.TrimSpace(state.PricingCharacterName) == "" ||
		state.PricingIntelligence < 0 {
		panic("ShipShopDialog: complete detached pricing character is required")
	}
}

func validateShipHarbourIdentity(state game_state.ShipShopState) {
	if strings.TrimSpace(state.Travel.HarbourName) == "" || state.Travel.MerchantID != state.Merchant.ID ||
		state.Travel.DockLocation != state.Merchant.Location {
		panic("ShipShopDialog: complete detached harbour review is required")
	}
}

func validateShipShopHarbourState(state game_state.ShipShopState) {
	if state.Berth.Location != state.Merchant.Location || state.Travel.BerthPosition != state.Berth.Position ||
		state.Travel.VehicleType != state.Berth.VehicleType || state.Travel.PendingDelivery != state.Berth.Pending {
		panic("ShipShopDialog: detached berth and harbour review do not match")
	}
}

func validateShipQuote(
	state game_state.ShipShopState,
	quote game_state.ShipPurchaseQuote,
	vehicle references.VehicleType,
) {
	if quote.VehicleType != vehicle || quote.BasePrice <= 0 || quote.Price <= 0 || quote.Price > math.MaxUint16 ||
		quote.GoldBefore != state.Gold || quote.Berth != state.Berth || quote.RequestFingerprint == "" {
		panic("ShipShopDialog: invalid detached ship quote")
	}
	validateShipQuoteAction(quote, vehicle)
	if quote.BlockReason > game_state.ShipPurchaseBlockSkiffLimit {
		panic("ShipShopDialog: unknown purchase block reason")
	}
	validateShipQuoteApprovals(quote)
}

func validateShipQuoteApprovals(quote game_state.ShipPurchaseQuote) {
	if quote.BlockReason.IsBlocked() {
		if len(quote.ApprovalViews) != 0 {
			panic("ShipShopDialog: blocked quote cannot contain approval views")
		}
		validateBlockedShipQuoteGold(quote)

		return
	}
	validateShipPurchaseGold(quote)
	wantCount := expectedShipApprovalCount(quote)
	if len(quote.ApprovalViews) != wantCount {
		panic("ShipShopDialog: approval count is inconsistent")
	}
	for _, approval := range quote.ApprovalViews {
		wantAfter := expectedShipApprovalSkiffsAfter(quote, approval.Quantity)
		if approval.SkiffCountBefore != quote.Berth.SkiffCount || approval.SkiffCountAfter != wantAfter {
			panic("ShipShopDialog: approval skiff projection is inconsistent")
		}
	}
}

func expectedShipApprovalCount(quote game_state.ShipPurchaseQuote) int {
	if quote.Action != game_state.ShipPurchaseLoadSkiff {
		return 1
	}
	remaining := game_state.ShipShopMaximumSkiffs - quote.Berth.SkiffCount

	return min(remaining, max(1, int(quote.GoldBefore)/quote.Price))
}

func expectedShipApprovalSkiffsAfter(quote game_state.ShipPurchaseQuote, quantity int) int {
	switch quote.Action {
	case game_state.ShipPurchaseCreateFrigate:
		return shipShopFrigateSkiffs
	case game_state.ShipPurchaseCreateStandaloneSkiff:
		return 0
	case game_state.ShipPurchaseLoadSkiff:
		return quote.Berth.SkiffCount + quantity
	default:
		panic("expectedShipApprovalSkiffsAfter: unknown action")
	}
}

func validateBlockedShipQuoteGold(quote game_state.ShipPurchaseQuote) {
	expectedAfter := quote.GoldBefore
	if quote.Affordable {
		if quote.Price > int(quote.GoldBefore) {
			panic("ShipShopDialog: affordable ship quote exceeds party gold")
		}
		expectedAfter -= uint16(quote.Price) //nolint:gosec // Trusted quote affordability bounds the price.
	} else if quote.Price <= int(quote.GoldBefore) {
		panic("ShipShopDialog: unaffordable ship quote is within party gold")
	}
	if quote.GoldAfterPayment != expectedAfter {
		panic("ShipShopDialog: ship quote gold consequence is inconsistent")
	}
}

func validateShipQuoteAction(quote game_state.ShipPurchaseQuote, vehicle references.VehicleType) {
	switch quote.Action {
	case game_state.ShipPurchaseCreateFrigate:
		if vehicle != references.FrigateVehicle {
			panic("ShipShopDialog: frigate action has wrong vehicle")
		}
	case game_state.ShipPurchaseCreateStandaloneSkiff, game_state.ShipPurchaseLoadSkiff:
		if vehicle != references.SkiffVehicle {
			panic("ShipShopDialog: skiff action has wrong vehicle")
		}
	default:
		panic("ShipShopDialog: unknown purchase action")
	}
}

func validateShipBerth(berth game_state.ShipBerthReview) {
	if berth.SkiffCount < 0 || berth.SkiffCount > game_state.ShipShopMaximumSkiffs {
		panic("ShipShopDialog: berth skiff count is invalid")
	}
	switch berth.State {
	case game_state.ShipBerthEmpty:
		validateEmptyShipBerth(berth)
	case game_state.ShipBerthPendingFrigate:
		validatePendingFrigateBerth(berth)
	case game_state.ShipBerthPendingSkiff:
		validatePendingSkiffBerth(berth)
	case game_state.ShipBerthMooredFrigate:
		validateMooredFrigateBerth(berth)
	case game_state.ShipBerthMooredSkiff:
		validateMooredSkiffBerth(berth)
	case game_state.ShipBerthObstructed:
	default:
		panic("ShipShopDialog: unknown berth state")
	}
}

func validateEmptyShipBerth(berth game_state.ShipBerthReview) {
	if berth.VehicleType != references.NoPartyVehicle || berth.Pending || berth.SkiffCount != 0 {
		panic("ShipShopDialog: empty berth is inconsistent")
	}
}

func validatePendingFrigateBerth(berth game_state.ShipBerthReview) {
	if berth.VehicleType != references.FrigateVehicle || !berth.Pending {
		panic("ShipShopDialog: pending frigate berth is inconsistent")
	}
}

func validatePendingSkiffBerth(berth game_state.ShipBerthReview) {
	if berth.VehicleType != references.SkiffVehicle || !berth.Pending || berth.SkiffCount != 0 {
		panic("ShipShopDialog: pending skiff berth is inconsistent")
	}
}

func validateMooredFrigateBerth(berth game_state.ShipBerthReview) {
	if berth.VehicleType != references.FrigateVehicle || berth.Pending {
		panic("ShipShopDialog: moored frigate berth is inconsistent")
	}
}

func validateMooredSkiffBerth(berth game_state.ShipBerthReview) {
	if berth.VehicleType != references.SkiffVehicle || berth.Pending || berth.SkiffCount != 0 {
		panic("ShipShopDialog: moored skiff berth is inconsistent")
	}
}

func validateShipTravelReview(review game_state.HarbourTravelReview, requireAvailable bool) {
	if review.MerchantID == "" || strings.TrimSpace(review.HarbourName) == "" ||
		review.TimeCostMinutes != game_state.HarbourTravelTimeCostMinutes || review.Fingerprint == "" ||
		review.BlockReason > game_state.HarbourTravelBlockArrivalOccupied {
		panic("Ship widgets: invalid harbour travel review")
	}
	validateShipTravelAvailability(review, requireAvailable)
}

func validateShipTravelAvailability(review game_state.HarbourTravelReview, requireAvailable bool) {
	if review.BlockReason.IsBlocked() {
		if requireAvailable {
			panic("Ship widgets: available harbour travel review is required")
		}

		return
	}
	switch review.VehicleType {
	case references.FrigateVehicle, references.SkiffVehicle:
		return
	case references.NoPartyVehicle, references.CarpetVehicle, references.HorseVehicle, references.NPC:
		panic("Ship widgets: available harbour travel review is required")
	default:
		panic("Ship widgets: unknown harbour travel vehicle")
	}
}

func validateShipShopResultDisplay(display ShipShopResultDisplay) {
	validateShipWidgetText("merchant exchange", display.MerchantExchange)
	validateShipWidgetText("receipt", display.Receipt)
}

func validateShipWidgetText(label, value string) {
	if strings.TrimSpace(value) == "" {
		panic("Ship widgets: finalized " + label + " is required")
	}
}
