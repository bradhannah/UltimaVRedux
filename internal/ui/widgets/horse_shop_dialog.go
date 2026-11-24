package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	horseShopShowroomWidthRatio  = 0.90
	horseShopMinimumInset        = 4
	horseShopInsetDivisor        = 64
	horseShopDoubleInset         = 2
	horseShopSpriteHeightUnits   = 32
	horseShopLineSpacing         = 1.06
	horseShopMinimumTextChars    = 8
	horseShopStatusMinimumLines  = 5
	horseShopShowroomDetailLines = 2
	horseShopCenterDivisor       = 2
	horseStableClosedText        = "The stables are closed."
	horseShopPreviewStoppedText  = "Preview stopped before purchase authority."
	horseShopProcessingText      = "Processing purchase"
)

var _ Widget = (*HorseShopDialog)(nil)

// HorseShopDialogPhase identifies the one-off horse purchase lifecycle.
type HorseShopDialogPhase byte

const (
	// HorseShopBrowse accepts review or leave input.
	HorseShopBrowse HorseShopDialogPhase = iota
	// HorseShopAwaitingApproval blocks the parent while the workflow owns the approval dialog.
	HorseShopAwaitingApproval
	// HorseShopAwaitingResult blocks input while authority processes an approved request.
	HorseShopAwaitingResult
	// HorseShopFarewell displays the rude cancellation acknowledgement.
	HorseShopFarewell
	// HorseShopSuccess displays the terminal polite purchase result.
	HorseShopSuccess
	// HorseShopFundsRefusal displays the terminal insufficient-funds refusal without a farewell.
	HorseShopFundsRefusal
	// HorseShopStableClosed displays the faithful capacity or placement refusal.
	HorseShopStableClosed
	// HorseShopPreviewStopped displays the terminal Preview authority boundary.
	HorseShopPreviewStopped
	// HorseShopSessionError displays an unrecoverable session error.
	HorseShopSessionError
)

// HorseShopDialogProse contains finalized merchant copy supplied by the workflow.
type HorseShopDialogProse struct {
	Greeting       string
	RudeFarewell   string
	PoliteFarewell string
	FundsRefusal   string
}

// HorseShopDialogConfig configures one detached, keyboard-only horse showroom.
type HorseShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	HorseSprite    *ebiten.Image
	State          game_state.HorseShopState
	Prose          HorseShopDialogProse
	Preview        bool
}

// HorsePurchaseIntent carries one detached quote and its comparison-only authority request.
type HorsePurchaseIntent struct {
	Quote   game_state.HorsePurchaseQuote
	Request game_state.HorsePurchaseRequest
	Preview bool
}

// HorseShopDialog presents one horse offer and emits one workflow-owned approval intent.
type HorseShopDialog struct {
	keyboard    *input.Keyboard
	horseSprite *ebiten.Image
	state       game_state.HorseShopState
	prose       HorseShopDialogProse
	preview     bool

	phase           HorseShopDialogPhase
	dismissed       bool
	latestExchange  string
	receiptText     string
	pendingIntent   *HorsePurchaseIntent
	awaitingQuote   game_state.HorsePurchaseQuote
	awaitingRequest game_state.HorsePurchaseRequest

	shell                 *MerchantDialogShell
	showroomTitleOutput   *text.Output
	showroomDetailsOutput *text.Output
	statusOutput          *text.Output
	showroomFont          *text.UltimaFont
	statusFont            *text.UltimaFont
	workspaceContent      image.Rectangle
	showroomBounds        image.Rectangle
	showroomTitleText     image.Rectangle
	horseSpriteBounds     image.Rectangle
	showroomDetailsText   image.Rectangle
	statusBounds          image.Rectangle
	statusTextBounds      image.Rectangle
	lastScreenWidth       int
	lastScreenHeight      int
}

// NewHorseShopDialog creates a concrete horse showroom from authoritative detached state.
func NewHorseShopDialog(config HorseShopDialogConfig) *HorseShopDialog {
	validateHorseShopDialogConfig(config)

	phase := HorseShopBrowse
	latestExchange := config.Prose.Greeting
	if horsePlacementClosesStable(config.State.Quote.Placement.BlockStatus) {
		phase = HorseShopStableClosed
		latestExchange = horseStableClosedText
	}
	var emptyBounds image.Rectangle
	dialog := &HorseShopDialog{
		keyboard:              config.Keyboard,
		horseSprite:           config.HorseSprite,
		state:                 config.State,
		prose:                 config.Prose,
		preview:               config.Preview,
		phase:                 phase,
		dismissed:             false,
		latestExchange:        latestExchange,
		receiptText:           "",
		pendingIntent:         nil,
		awaitingQuote:         emptyHorsePurchaseQuote(),
		awaitingRequest:       emptyHorsePurchaseRequest(),
		shell:                 nil,
		showroomTitleOutput:   nil,
		showroomDetailsOutput: nil,
		statusOutput:          nil,
		showroomFont:          nil,
		statusFont:            nil,
		workspaceContent:      emptyBounds,
		showroomBounds:        emptyBounds,
		showroomTitleText:     emptyBounds,
		horseSpriteBounds:     emptyBounds,
		showroomDetailsText:   emptyBounds,
		statusBounds:          emptyBounds,
		statusTextBounds:      emptyBounds,
		lastScreenWidth:       0,
		lastScreenHeight:      0,
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

func validateHorseShopDialogConfig(config HorseShopDialogConfig) {
	if config.Keyboard == nil {
		panic("HorseShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("HorseShopDialog: merchant sprite is required")
	}
	if config.HorseSprite == nil {
		panic("HorseShopDialog: horse sprite is required")
	}
	validateHorseShopState(config.State, config.Preview)
	validateHorseShopProse(config.Prose)
}

// Draw renders the shared merchant shell, showroom card, and remaining-space status panel.
func (d *HorseShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	if d.showroomVisible() {
		d.drawShowroom(screen)
	}
	d.drawStatus(screen)
}

// Update processes keyboard-only showroom and terminal acknowledgement input.
func (d *HorseShopDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}
	switch d.phase {
	case HorseShopBrowse:
		if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.latestExchange = d.prose.RudeFarewell
			d.receiptText = ""
			d.transitionTo(HorseShopFarewell)
			d.syncPresentation()

			return
		}
		if d.enterPressed() {
			d.emitPurchaseIntent()
		}
	case HorseShopAwaitingApproval, HorseShopAwaitingResult:
		return
	case HorseShopFarewell, HorseShopSuccess, HorseShopFundsRefusal,
		HorseShopStableClosed, HorseShopPreviewStopped, HorseShopSessionError:
		if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.Dismiss()
		}
	default:
		panic("HorseShopDialog.Update: unknown phase")
	}
}

// ConsumePendingPurchaseIntent returns and clears one detached horse approval intent.
func (d *HorseShopDialog) ConsumePendingPurchaseIntent() (HorsePurchaseIntent, bool) {
	if d.pendingIntent == nil {
		return HorsePurchaseIntent{
			Quote:   emptyHorsePurchaseQuote(),
			Request: emptyHorsePurchaseRequest(),
			Preview: false,
		}, false
	}
	intent := *d.pendingIntent
	d.pendingIntent = nil

	return intent, true
}

// CancelPurchaseApproval returns to Browse after the workflow-owned modal is cancelled.
func (d *HorseShopDialog) CancelPurchaseApproval() {
	if d.phase != HorseShopAwaitingApproval {
		panic("HorseShopDialog.CancelPurchaseApproval: dialog is not awaiting approval")
	}
	// REDUX DEVIATION: Original horse purchase ends the interaction (ref:MERCH-0008)
	// when the second horse prompt is refused. Escape from the Redux approval
	// returns to Browse to match the established merchant Back contract.
	d.clearAwaitingPurchase()
	d.transitionTo(HorseShopBrowse)
	d.syncPresentation()
}

// AcceptPurchaseApproval records the modal request and advances to authority.
func (d *HorseShopDialog) AcceptPurchaseApproval(request game_state.HorsePurchaseRequest) {
	if d.phase != HorseShopAwaitingApproval || request != d.awaitingRequest {
		panic("HorseShopDialog.AcceptPurchaseApproval: request does not match pending approval")
	}
	d.pendingIntent = nil
	d.transitionTo(HorseShopAwaitingResult)
	d.syncPresentation()
}

// ApplyPurchaseResult applies one authoritative result and enters terminal polite success.
func (d *HorseShopDialog) ApplyPurchaseResult(result game_state.HorsePurchaseResult) {
	if d.phase != HorseShopAwaitingResult ||
		result.MerchantID != d.awaitingRequest.ReviewedMerchantID ||
		result.PricingCharacterIndex != d.awaitingRequest.ReviewedPricingCharacterIndex ||
		result.Price != d.awaitingRequest.ReviewedPrice ||
		result.GoldBefore != d.awaitingQuote.GoldBefore {
		panic("HorseShopDialog.ApplyPurchaseResult: result does not match the accepted request")
	}
	d.state.Gold = result.GoldAfterTheft
	d.latestExchange = d.prose.PoliteFarewell
	d.receiptText = fmt.Sprintf("Bought: one horse for %d gold.", result.Price)
	if result.Theft > 0 {
		d.receiptText = fmt.Sprintf(
			"Bought: one horse for %d gold. %d more gold was stolen! Thou hast %d gold left.",
			result.Price,
			result.Theft,
			result.GoldAfterTheft,
		)
	}
	d.clearAwaitingPurchase()
	d.transitionTo(HorseShopSuccess)
	d.syncPresentation()
}

// ApplyFundsRefusal enters the faithful terminal no-farewell funds refusal.
func (d *HorseShopDialog) ApplyFundsRefusal() {
	if d.phase != HorseShopAwaitingResult {
		panic("HorseShopDialog.ApplyFundsRefusal: dialog is not awaiting a result")
	}
	d.latestExchange = d.prose.FundsRefusal
	if d.awaitingQuote.Affordable {
		d.receiptText = "Thou hast not enough gold!"
	} else {
		d.receiptText = fmt.Sprintf(
			"Need %d gold; party has %d.",
			d.awaitingQuote.Price,
			d.awaitingQuote.GoldBefore,
		)
	}
	d.clearAwaitingPurchase()
	d.transitionTo(HorseShopFundsRefusal)
	d.syncPresentation()
}

// ApplyStableClosed enters the terminal stable-closed state after stale placement or capacity.
func (d *HorseShopDialog) ApplyStableClosed() {
	if d.phase != HorseShopAwaitingResult {
		panic("HorseShopDialog.ApplyStableClosed: dialog is not awaiting a result")
	}
	d.latestExchange = horseStableClosedText
	d.receiptText = horseStableClosedText
	d.clearAwaitingPurchase()
	d.transitionTo(HorseShopStableClosed)
	d.syncPresentation()
}

// ApplyPreviewStopped records the accepted Preview boundary without invoking authority.
func (d *HorseShopDialog) ApplyPreviewStopped(request game_state.HorsePurchaseRequest) {
	if d.phase != HorseShopAwaitingResult || request != d.awaitingRequest {
		panic("HorseShopDialog.ApplyPreviewStopped: request does not match the accepted preview")
	}
	d.latestExchange = horseShopPreviewStoppedText
	d.receiptText = "Preview only: no purchase was committed."
	d.clearAwaitingPurchase()
	d.transitionTo(HorseShopPreviewStopped)
	d.syncPresentation()
}

// ApplySessionError replaces presentation with a terminal workflow/session error.
func (d *HorseShopDialog) ApplySessionError(message string) {
	requireFinalizedText("HorseShopDialog", "session error", message)
	d.pendingIntent = nil
	d.clearAwaitingPurchase()
	d.latestExchange = message
	d.receiptText = message
	d.transitionTo(HorseShopSessionError)
	d.syncPresentation()
}

// GetShell returns the shared merchant presentation shell.
func (d *HorseShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// IsDismissed reports whether the dialog may be removed.
func (d *HorseShopDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the horse shop without producing another intent.
func (d *HorseShopDialog) Dismiss() { d.dismissed = true }

func (d *HorseShopDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return
	}
	if d.statusOutput != nil && width == d.lastScreenWidth && height == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth, d.lastScreenHeight = width, height
	d.shell.GetBounds()
	d.rebuildOutputs()
	d.calculateLayout()
	d.populateWorkspaceOutputs()
}

func (d *HorseShopDialog) rebuildOutputs() {
	d.showroomFont, d.showroomTitleOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: horseShopLineSpacing,
		MaxLines: 1, MaxChars: horseShopMinimumTextChars,
	})
	_, d.showroomDetailsOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: horseShopLineSpacing,
		MaxLines: horseShopShowroomDetailLines, MaxChars: horseShopMinimumTextChars,
	})
	d.statusFont, d.statusOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: horseShopLineSpacing,
		MaxLines: horseShopStatusMinimumLines, MaxChars: horseShopMinimumTextChars,
	})
}

func (d *HorseShopDialog) calculateLayout() {
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(horseShopMinimumInset, min(workspace.Dx(), workspace.Dy())/horseShopInsetDivisor)
	d.workspaceContent = insetRectangle(workspace, inset)
	if !d.showroomVisible() {
		d.showroomBounds = image.Rect(0, 0, 0, 0)
		d.showroomTitleText = image.Rect(0, 0, 0, 0)
		d.horseSpriteBounds = image.Rect(0, 0, 0, 0)
		d.showroomDetailsText = image.Rect(0, 0, 0, 0)
		d.statusBounds = d.workspaceContent
		d.configureStatusOutput(inset)

		return
	}
	d.calculateShowroomLayout(inset)
}

func (d *HorseShopDialog) calculateShowroomLayout(inset int) {
	gap := max(inset, d.pixelUnit()*horseShopDoubleInset)
	showroomWidth := max(1, int(math.Round(float64(d.workspaceContent.Dx())*horseShopShowroomWidthRatio)))
	showroomLeft := d.workspaceContent.Min.X + (d.workspaceContent.Dx()-showroomWidth)/horseShopCenterDivisor
	titleHeight := int(math.Ceil(d.showroomTitleOutput.GetFontHeight())) + inset*horseShopDoubleInset
	detailHeight := int(math.Ceil(
		d.showroomDetailsOutput.GetTotalHeightForLines(horseShopShowroomDetailLines),
	)) + inset*horseShopDoubleInset
	spriteHeight := max(d.horseSprite.Bounds().Dy()*horseShopDoubleInset, d.pixelUnit()*horseShopSpriteHeightUnits)
	naturalHeight := titleHeight + spriteHeight + detailHeight
	minimumStatusHeight := int(math.Ceil(
		d.statusOutput.GetTotalHeightForLines(horseShopStatusMinimumLines),
	)) + inset*horseShopDoubleInset
	maximumShowroomHeight := max(1, d.workspaceContent.Dy()-minimumStatusHeight-gap)
	showroomHeight := min(naturalHeight, maximumShowroomHeight)
	d.showroomBounds = image.Rect(
		showroomLeft,
		d.workspaceContent.Min.Y,
		showroomLeft+showroomWidth,
		d.workspaceContent.Min.Y+showroomHeight,
	)
	showroomInterior := insetRectangle(d.showroomBounds, inset)
	actualTitleHeight := min(titleHeight, showroomInterior.Dy())
	actualDetailHeight := min(detailHeight, max(0, showroomInterior.Dy()-actualTitleHeight))
	d.showroomTitleText = image.Rect(
		showroomInterior.Min.X,
		showroomInterior.Min.Y,
		showroomInterior.Max.X,
		showroomInterior.Min.Y+actualTitleHeight,
	)
	d.showroomDetailsText = image.Rect(
		showroomInterior.Min.X,
		showroomInterior.Max.Y-actualDetailHeight,
		showroomInterior.Max.X,
		showroomInterior.Max.Y,
	)
	d.horseSpriteBounds = image.Rect(
		showroomInterior.Min.X,
		d.showroomTitleText.Max.Y,
		showroomInterior.Max.X,
		max(d.showroomTitleText.Max.Y, d.showroomDetailsText.Min.Y),
	)
	d.statusBounds = image.Rect(
		showroomLeft,
		min(d.workspaceContent.Max.Y, d.showroomBounds.Max.Y+gap),
		showroomLeft+showroomWidth,
		d.workspaceContent.Max.Y,
	)
	d.showroomTitleOutput.SetMaxCharsPerLine(max(
		horseShopMinimumTextChars,
		d.showroomFont.CharsForWidth(max(1, d.showroomTitleText.Dx())),
	))
	d.showroomDetailsOutput.SetMaxCharsPerLine(max(
		horseShopMinimumTextChars,
		d.showroomDetailsOutput.Font.CharsForWidth(max(1, d.showroomDetailsText.Dx())),
	))
	d.configureStatusOutput(inset)
}

func (d *HorseShopDialog) configureStatusOutput(inset int) {
	d.statusTextBounds = insetRectangle(d.statusBounds, inset)
	d.statusOutput = text.NewOutput(
		d.statusFont,
		d.statusOutput.GetLineSpacing(),
		max(1, d.statusOutput.GetLineCapacity(max(1, d.statusTextBounds.Dy()))),
		max(horseShopMinimumTextChars, d.statusFont.CharsForWidth(max(1, d.statusTextBounds.Dx()))),
	)
}

func (d *HorseShopDialog) drawShowroom(screen *ebiten.Image) {
	drawMerchantPane(screen, d.showroomBounds, d.pixelUnit())
	d.showroomTitleOutput.DrawPanelContent(
		screen,
		image.Pt(d.showroomTitleText.Min.X, verticallyCenteredOutputY(d.showroomTitleText, d.showroomTitleOutput)),
		max(1, d.showroomTitleText.Dx()),
		etext.AlignCenter,
	)
	d.drawHorseSprite(screen)
	d.showroomDetailsOutput.DrawPanelContent(
		screen,
		image.Pt(
			d.showroomDetailsText.Min.X,
			verticallyCenteredOutputY(d.showroomDetailsText, d.showroomDetailsOutput),
		),
		max(1, d.showroomDetailsText.Dx()),
		etext.AlignCenter,
	)
}

func (d *HorseShopDialog) drawHorseSprite(screen *ebiten.Image) {
	if d.horseSpriteBounds.Empty() || d.horseSprite.Bounds().Empty() {
		return
	}
	source := d.horseSprite.Bounds()
	scale := min(
		float64(d.horseSpriteBounds.Dx())/float64(source.Dx()),
		float64(d.horseSpriteBounds.Dy())/float64(source.Dy()),
	)
	if scale <= 0 {
		return
	}
	drawWidth := float64(source.Dx()) * scale
	drawHeight := float64(source.Dy()) * scale
	options := new(ebiten.DrawImageOptions)
	options.Filter = ebiten.FilterNearest
	options.GeoM.Translate(float64(-source.Min.X), float64(-source.Min.Y))
	options.GeoM.Scale(scale, scale)
	options.GeoM.Translate(
		float64(d.horseSpriteBounds.Min.X)+(float64(d.horseSpriteBounds.Dx())-drawWidth)/horseShopCenterDivisor,
		float64(d.horseSpriteBounds.Min.Y)+(float64(d.horseSpriteBounds.Dy())-drawHeight)/horseShopCenterDivisor,
	)
	screen.DrawImage(d.horseSprite, options)
}

func (d *HorseShopDialog) drawStatus(screen *ebiten.Image) {
	drawMerchantPane(screen, d.statusBounds, d.pixelUnit())
	d.statusOutput.DrawPanelContent(
		screen,
		image.Pt(d.statusTextBounds.Min.X, verticallyCenteredOutputY(d.statusTextBounds, d.statusOutput)),
		max(1, d.statusTextBounds.Dx()),
		etext.AlignStart,
	)
}

func (d *HorseShopDialog) populateWorkspaceOutputs() {
	if d.showroomTitleOutput == nil || d.showroomDetailsOutput == nil || d.statusOutput == nil {
		return
	}
	d.showroomTitleOutput.Reset()
	d.showroomTitleOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text("Horse for Sale", DialogHeaderColor).Done()
	d.showroomDetailsOutput.Reset()
	d.showroomDetailsOutput.AppendLine().WithAlignment(etext.AlignCenter).White("Horse").Done()
	d.showroomDetailsOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Yellowf("%d gold", d.state.Quote.Price).Done()
	d.populateStatusOutput()
}

func (d *HorseShopDialog) populateStatusOutput() {
	d.statusOutput.Reset()
	if d.phase == HorseShopBrowse {
		d.populateBrowseStatusOutput()

		return
	}
	d.populateNonBrowseStatusOutput()
}

func (d *HorseShopDialog) populateNonBrowseStatusOutput() {
	switch d.phase {
	case HorseShopAwaitingApproval:
		d.statusOutput.AppendLine().Text(
			"Review this purchase in the approval dialog.", DialogControlHintColor,
		).WordWrap().Done()
	case HorseShopAwaitingResult:
		d.statusOutput.AppendLine().Text(horseShopProcessingText+".", DialogNavHintColor).Done()
	case HorseShopFarewell:
		d.statusOutput.AppendLine().Text("Press ", DialogControlHintColor).
			Text("Enter", DialogControlKeyColor).Text(" or ", DialogControlHintColor).
			Text("Esc", DialogControlKeyColor).Text(" to leave.", DialogControlHintColor).Done()
	case HorseShopSuccess, HorseShopFundsRefusal, HorseShopSessionError:
		statusColor := DialogStatusWarningColor
		if d.phase == HorseShopSuccess {
			statusColor = DialogDeltaPositive
		}
		d.statusOutput.AppendLine().Text(d.receiptText, statusColor).WordWrap().Done()
	case HorseShopStableClosed:
		d.statusOutput.AppendLine().White(horseStableClosedText).Done()
	case HorseShopPreviewStopped:
		d.statusOutput.AppendLine().Text("Preview", DialogStatusWarningColor).Done()
		d.statusOutput.AppendLine().Text(d.receiptText, DialogControlHintColor).WordWrap().Done()
	case HorseShopBrowse:
		panic("HorseShopDialog.populateNonBrowseStatusOutput: Browse handled separately")
	default:
		panic("HorseShopDialog.populateNonBrowseStatusOutput: unknown phase")
	}
}

func (d *HorseShopDialog) populateBrowseStatusOutput() {
	if d.preview {
		d.statusOutput.AppendLine().Text("Preview", DialogStatusWarningColor).Done()
	}
	if d.state.Quote.Placement.BlockStatus == game_state.HorseShopBlockAvailabilityDeferred {
		d.statusOutput.AppendLine().Text(
			"Availability will be checked in Live.", DialogControlHintColor,
		).WordWrap().Done()
	}
	d.statusOutput.AppendLine().Text("Enter", DialogControlKeyColor).
		Text(" to review this purchase.", DialogControlHintColor).WordWrap().Done()
}

func (d *HorseShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateWorkspaceOutputs()
}

func (d *HorseShopDialog) footerGuidance() string {
	switch d.phase {
	case HorseShopBrowse:
		return "[Enter] Review purchase   [Esc] Leave"
	case HorseShopAwaitingApproval:
		return "Confirm or cancel the purchase."
	case HorseShopAwaitingResult:
		return ""
	case HorseShopFarewell, HorseShopSuccess, HorseShopFundsRefusal,
		HorseShopStableClosed, HorseShopPreviewStopped, HorseShopSessionError:
		return merchantLeaveFooter
	default:
		panic("HorseShopDialog.footerGuidance: unknown phase")
	}
}

func (d *HorseShopDialog) emitPurchaseIntent() {
	if d.phase != HorseShopBrowse || d.pendingIntent != nil {
		return
	}
	request := horsePurchaseRequest(d.state)
	intent := HorsePurchaseIntent{Quote: d.state.Quote, Request: request, Preview: d.preview}
	d.pendingIntent = &intent
	d.awaitingQuote = d.state.Quote
	d.awaitingRequest = request
	d.transitionTo(HorseShopAwaitingApproval)
	d.syncPresentation()
}

func (d *HorseShopDialog) transitionTo(phase HorseShopDialogPhase) {
	if d.phase == phase {
		return
	}
	wasStableClosed := d.phase == HorseShopStableClosed
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
	if d.statusOutput != nil && wasStableClosed != (phase == HorseShopStableClosed) {
		d.calculateLayout()
	}
}

func (d *HorseShopDialog) clearAwaitingPurchase() {
	d.pendingIntent = nil
	d.awaitingQuote = emptyHorsePurchaseQuote()
	d.awaitingRequest = emptyHorsePurchaseRequest()
}

func (d *HorseShopDialog) showroomVisible() bool { return d.phase != HorseShopStableClosed }

func (d *HorseShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *HorseShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func horsePurchaseRequest(state game_state.HorseShopState) game_state.HorsePurchaseRequest {
	return game_state.HorsePurchaseRequest{
		ReviewedMerchantID:            state.Merchant.ID,
		ReviewedPricingCharacterIndex: state.PricingCharacterIndex,
		ReviewedPrice:                 state.Quote.Price,
		ReviewedGoldBefore:            state.Quote.GoldBefore,
		ReviewedPlacement:             state.Quote.Placement,
	}
}

func horsePlacementClosesStable(status game_state.HorseShopBlockStatus) bool {
	return status == game_state.HorseShopBlockMapUnitCapacity ||
		status == game_state.HorseShopBlockNoLegalPlacement
}

func validateHorseShopState(state game_state.HorseShopState, preview bool) {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyHorse {
		panic("HorseShopDialog: detached horse shop state is required")
	}
	if state.PricingCharacterIndex < 0 || strings.TrimSpace(state.PricingCharacterName) == "" {
		panic("HorseShopDialog: detached pricing character is required")
	}
	quote := state.Quote
	if quote.BasePrice <= 0 || quote.Price <= 0 || quote.Price > math.MaxUint16 || quote.GoldBefore != state.Gold {
		panic("HorseShopDialog: invalid detached horse quote")
	}
	validateHorseQuoteGold("HorseShopDialog", quote)
	validateHorsePlacementStatus("HorseShopDialog", quote.Placement.BlockStatus, preview, true)
}

func validateHorseShopProse(prose HorseShopDialogProse) {
	requireFinalizedText("HorseShopDialog", "greeting", prose.Greeting)
	requireFinalizedText("HorseShopDialog", "rude farewell", prose.RudeFarewell)
	requireFinalizedText("HorseShopDialog", "polite farewell", prose.PoliteFarewell)
	requireFinalizedText("HorseShopDialog", "funds refusal", prose.FundsRefusal)
}

func validateHorseQuoteGold(owner string, quote game_state.HorsePurchaseQuote) {
	affordable := int(quote.GoldBefore) >= quote.Price
	expectedAfter := quote.GoldBefore
	if affordable {
		expectedAfter -= uint16(quote.Price) //nolint:gosec // Callers range-check every trusted quote price.
	}
	if quote.Affordable != affordable || quote.GoldAfterPayment != expectedAfter {
		panic(owner + ": horse quote gold consequence is inconsistent")
	}
}

func validateHorsePlacementStatus(
	owner string,
	status game_state.HorseShopBlockStatus,
	preview bool,
	allowStableClosed bool,
) {
	switch status {
	case game_state.HorseShopBlockNone:
	case game_state.HorseShopBlockAvailabilityDeferred:
		if !preview {
			panic(owner + ": deferred availability is valid only in Preview")
		}
	case game_state.HorseShopBlockMapUnitCapacity, game_state.HorseShopBlockNoLegalPlacement:
		if !allowStableClosed {
			panic(owner + ": stable-closed quote cannot be approved")
		}
	default:
		panic(owner + ": unknown horse placement status")
	}
}

func emptyHorsePurchaseQuote() game_state.HorsePurchaseQuote {
	return game_state.HorsePurchaseQuote{
		BasePrice:        0,
		Price:            0,
		GoldBefore:       0,
		GoldAfterPayment: 0,
		Affordable:       false,
		Placement: game_state.HorsePlacementReview{
			Position:    references.Position{X: 0, Y: 0},
			Floor:       0,
			Terrain:     0,
			BlockStatus: game_state.HorseShopBlockNone,
		},
	}
}

func emptyHorsePurchaseRequest() game_state.HorsePurchaseRequest {
	return game_state.HorsePurchaseRequest{
		ReviewedMerchantID:            "",
		ReviewedPricingCharacterIndex: 0,
		ReviewedPrice:                 0,
		ReviewedGoldBefore:            0,
		ReviewedPlacement:             emptyHorsePurchaseQuote().Placement,
	}
}
