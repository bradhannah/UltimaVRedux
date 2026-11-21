package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	reagentShopMinimumOffers    = 3
	reagentShopMaximumOffers    = 5
	reagentShopCatalogColumns   = 4
	reagentShopDetailLines      = 4
	reagentShopMinimumInset     = 4
	reagentShopInsetDivisor     = 64
	reagentShopCellInsetUnits   = 2
	reagentShopDoubleInset      = 2
	reagentShopLineSpacing      = 1.08
	reagentShopMinimumTextChars = 8
)

var _ Widget = (*ReagentShopDialog)(nil)

// ReagentShopDialogPhase identifies the workflow-owned reagent purchase phase.
type ReagentShopDialogPhase byte

const (
	// ReagentShopBrowse accepts catalog navigation and purchase review.
	ReagentShopBrowse ReagentShopDialogPhase = iota
	// ReagentShopAwaitingQuantity blocks the parent while the purchase modal is active.
	ReagentShopAwaitingQuantity
	// ReagentShopAwaitingResult blocks input while authority processes an approved request.
	ReagentShopAwaitingResult
	// ReagentShopFarewell displays the deterministic exit exchange before dismissal.
	ReagentShopFarewell
	// ReagentShopSessionError displays a fatal session error until dismissal.
	ReagentShopSessionError
)

// ReagentShopDialogProse contains finalized merchant copy used by the dialog.
type ReagentShopDialogProse struct {
	Greeting       string
	RudeFarewell   string
	PoliteFarewell string
}

// ReagentShopDialogConfig configures one detached, keyboard-only reagent shop.
type ReagentShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	State          game_state.ReagentShopState
	Prose          ReagentShopDialogProse
	Preview        bool
}

// ReagentPurchaseIntent carries the trusted selected quote into the quantity modal.
type ReagentPurchaseIntent struct {
	Quote game_state.ReagentOfferQuote
}

// ReagentPurchaseResultDisplay contains finalized text supplied after authority returns.
type ReagentPurchaseResultDisplay struct {
	MerchantExchange string
	Receipt          string
}

// ReagentShopDialog presents a dynamic reagent catalog and emits quantity-selection intents.
type ReagentShopDialog struct {
	navigation
	keyboard *input.Keyboard
	state    game_state.ReagentShopState
	prose    ReagentShopDialogProse
	preview  bool

	listState       *ScrollableListState
	phase           ReagentShopDialogPhase
	dismissed       bool
	purchased       bool
	latestExchange  string
	receiptText     string
	pendingIntent   *ReagentPurchaseIntent
	awaitingQuote   game_state.ReagentOfferQuote
	awaitingRequest game_state.ReagentPurchaseRequest

	shell         *MerchantDialogShell
	catalogOutput *text.Output
	detailOutput  *text.Output
	catalogFont   *text.UltimaFont
	detailFont    *text.UltimaFont

	catalogBounds    image.Rectangle
	catalogHeader    image.Rectangle
	catalogRows      []image.Rectangle
	catalogColumns   [reagentShopCatalogColumns]image.Rectangle
	detailBounds     image.Rectangle
	detailTextBounds image.Rectangle

	lastScreenWidth  int
	lastScreenHeight int
}

// NewReagentShopDialog creates a visible reagent shop from authoritative detached state.
func NewReagentShopDialog(config ReagentShopDialogConfig) *ReagentShopDialog {
	if config.Keyboard == nil {
		panic("ReagentShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("ReagentShopDialog: merchant sprite is required")
	}
	validateReagentShopState(config.State)
	validateReagentFinalizedText("greeting", config.Prose.Greeting)
	validateReagentFinalizedText("rude farewell", config.Prose.RudeFarewell)
	validateReagentFinalizedText("polite farewell", config.Prose.PoliteFarewell)

	state := cloneReagentShopState(config.State)
	var emptyBounds image.Rectangle
	var catalogColumns [reagentShopCatalogColumns]image.Rectangle
	dialog := &ReagentShopDialog{
		navigation:       newNavigation(config.Keyboard.IsKeyHeld),
		keyboard:         config.Keyboard,
		state:            state,
		prose:            config.Prose,
		preview:          config.Preview,
		listState:        NewScrollableListState(len(state.Offers), reagentShopMaximumOffers),
		phase:            ReagentShopBrowse,
		dismissed:        false,
		purchased:        false,
		latestExchange:   config.Prose.Greeting,
		receiptText:      "",
		pendingIntent:    nil,
		awaitingQuote:    emptyReagentOfferQuote(),
		awaitingRequest:  emptyReagentPurchaseRequest(),
		shell:            nil,
		catalogOutput:    nil,
		detailOutput:     nil,
		catalogFont:      nil,
		detailFont:       nil,
		catalogBounds:    emptyBounds,
		catalogHeader:    emptyBounds,
		catalogRows:      make([]image.Rectangle, len(state.Offers)),
		catalogColumns:   catalogColumns,
		detailBounds:     emptyBounds,
		detailTextBounds: emptyBounds,
		lastScreenWidth:  0,
		lastScreenHeight: 0,
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

// Draw renders the shared shell and full reagent-family workspace.
func (d *ReagentShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	d.drawCatalog(screen)
	d.drawDetail(screen)
}

// Update processes keyboard-only shop input.
func (d *ReagentShopDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}
	switch d.phase {
	case ReagentShopBrowse:
		d.updateBrowse()
	case ReagentShopAwaitingQuantity, ReagentShopAwaitingResult:
		return
	case ReagentShopFarewell, ReagentShopSessionError:
		if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.Dismiss()
		}
	default:
		panic("ReagentShopDialog.Update: unknown phase")
	}
}

func (d *ReagentShopDialog) updateBrowse() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.showFarewell()

		return
	}
	if key, owned := d.Poll(d.keyboard); owned {
		if key != nil {
			if delta := listNavigationDelta(*key, 0); delta != 0 {
				d.moveSelection(delta)
			}
		}

		return
	}
	if d.enterPressed() {
		d.emitPurchaseIntent()
	}
}

// ConsumePendingPurchaseIntent returns and clears one trusted quantity-selection intent.
func (d *ReagentShopDialog) ConsumePendingPurchaseIntent() (ReagentPurchaseIntent, bool) {
	if d.pendingIntent == nil {
		return ReagentPurchaseIntent{Quote: emptyReagentOfferQuote()}, false
	}
	intent := *d.pendingIntent
	intent.Quote = cloneReagentOfferQuote(intent.Quote)
	d.pendingIntent = nil

	return intent, true
}

// CancelQuantitySelection returns to browsing after the nested modal is cancelled.
func (d *ReagentShopDialog) CancelQuantitySelection() {
	if d.phase != ReagentShopAwaitingQuantity {
		panic("ReagentShopDialog.CancelQuantitySelection: dialog is not awaiting quantity")
	}
	d.clearAwaitingPurchase()
	d.transitionTo(ReagentShopBrowse)
	d.syncPresentation()
}

// AcceptQuantitySelection records the modal's validated request and awaits authority.
func (d *ReagentShopDialog) AcceptQuantitySelection(request game_state.ReagentPurchaseRequest) {
	if d.phase != ReagentShopAwaitingQuantity ||
		request.Reagent != d.awaitingQuote.Reagent ||
		request.BundleCount == 0 ||
		int(request.BundleCount) > len(d.awaitingQuote.ApprovalViews) {
		panic("ReagentShopDialog.AcceptQuantitySelection: request does not match the selected quote")
	}
	d.pendingIntent = nil
	d.awaitingRequest = request
	d.transitionTo(ReagentShopAwaitingResult)
	d.syncPresentation()
}

// ApplyPurchaseResult applies authoritative result data, a complete refreshed state, and finalized text.
func (d *ReagentShopDialog) ApplyPurchaseResult(
	result game_state.ReagentPurchaseResult,
	refreshed game_state.ReagentShopState,
	display ReagentPurchaseResultDisplay,
) {
	if d.phase != ReagentShopAwaitingResult {
		panic("ReagentShopDialog.ApplyPurchaseResult: dialog is not awaiting a result")
	}
	if result.Reagent != d.awaitingRequest.Reagent ||
		result.BundleCount == 0 || result.BundleCount > d.awaitingRequest.BundleCount {
		panic("ReagentShopDialog.ApplyPurchaseResult: result does not match the accepted request")
	}
	if result.BundleCount < d.awaitingRequest.BundleCount {
		display.Receipt += fmt.Sprintf(" Batch stopped: %d of %d bundles purchased.",
			result.BundleCount, d.awaitingRequest.BundleCount)
	}
	validateReagentShopState(refreshed)
	if refreshed.Merchant.ID != d.state.Merchant.ID ||
		refreshed.PricingCharacterIndex != d.state.PricingCharacterIndex ||
		refreshed.PricingCharacterName != d.state.PricingCharacterName {
		panic("ReagentShopDialog.ApplyPurchaseResult: refreshed state does not match the active shop session")
	}
	selectedIndex := reagentOfferIndex(refreshed.Offers, result.Reagent)
	if result.QuantityAfter != refreshed.Offers[selectedIndex].QuantityBefore ||
		result.GoldAfterTheft != refreshed.Gold ||
		result.ReagentName != refreshed.Offers[selectedIndex].ReagentName {
		panic("ReagentShopDialog.ApplyPurchaseResult: refreshed state does not contain the authoritative result")
	}
	validateReagentFinalizedText("purchase exchange", display.MerchantExchange)
	validateReagentFinalizedText("purchase receipt", display.Receipt)

	d.state = cloneReagentShopState(refreshed)
	d.listState.Reset(len(d.state.Offers))
	d.listState.SetIndex(reagentOfferIndex(d.state.Offers, result.Reagent))
	d.calculateLayout()
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.purchased = true
	d.clearAwaitingPurchase()
	d.transitionTo(ReagentShopBrowse)
	d.syncPresentation()
}

// ApplyPreviewStopped returns to Browse without changing the detached shop state.
func (d *ReagentShopDialog) ApplyPreviewStopped(request game_state.ReagentPurchaseRequest) {
	if d.phase != ReagentShopAwaitingResult || request != d.awaitingRequest {
		panic("ReagentShopDialog.ApplyPreviewStopped: request does not match the accepted preview")
	}
	d.latestExchange = "Preview stopped before purchase authority."
	d.receiptText = "Preview only: no purchase was committed; gold and reagent inventory are unchanged."
	d.clearAwaitingPurchase()
	d.transitionTo(ReagentShopBrowse)
	d.syncPresentation()
}

// ApplySessionError replaces the exchange and disables further purchases.
func (d *ReagentShopDialog) ApplySessionError(message string) {
	validateReagentFinalizedText("session error", message)
	d.pendingIntent = nil
	d.clearAwaitingPurchase()
	d.latestExchange = message
	d.receiptText = ""
	d.transitionTo(ReagentShopSessionError)
	d.syncPresentation()
}

// GetSelectedIndex returns the selected catalog index.
func (d *ReagentShopDialog) GetSelectedIndex() int { return d.listState.GetCurrentIndex() }

// GetCatalogHeadings returns the exact four reagent catalog headings.
func (d *ReagentShopDialog) GetCatalogHeadings() [reagentShopCatalogColumns]string {
	return [reagentShopCatalogColumns]string{"Reagent", "Bundle", "Owned", shopHeadingPrice}
}

// GetCatalogRowText returns the four non-projected values displayed for one quote.
func (d *ReagentShopDialog) GetCatalogRowText(index int) [reagentShopCatalogColumns]string {
	if index < 0 || index >= len(d.state.Offers) {
		panic("ReagentShopDialog.GetCatalogRowText: index out of range")
	}
	quote := d.state.Offers[index]

	price := strconv.Itoa(quote.PricePerBundle)
	if quote.Full {
		price = "--"
	}

	return [reagentShopCatalogColumns]string{
		quote.ReagentName,
		"x" + strconv.FormatUint(uint64(quote.BundleQuantity), 10),
		strconv.FormatUint(uint64(quote.QuantityBefore), 10),
		price,
	}
}

// GetShell returns the shared merchant presentation shell.
func (d *ReagentShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// IsDismissed reports whether the dialog may be removed.
func (d *ReagentShopDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the shop without producing a purchase request.
func (d *ReagentShopDialog) Dismiss() { d.dismissed = true }

func (d *ReagentShopDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return
	}
	if d.catalogOutput != nil && width == d.lastScreenWidth && height == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth, d.lastScreenHeight = width, height
	d.shell.GetBounds()
	d.catalogFont, d.catalogOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: reagentShopLineSpacing,
		MaxLines: 1, MaxChars: reagentShopMinimumTextChars,
	})
	d.detailFont, d.detailOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: reagentShopLineSpacing,
		MaxLines: reagentShopDetailLines, MaxChars: reagentShopMinimumTextChars,
	})
	d.calculateLayout()
	d.populateDetailOutput()
}

func (d *ReagentShopDialog) calculateLayout() {
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(reagentShopMinimumInset, min(workspace.Dx(), workspace.Dy())/reagentShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, d.pixelUnit()*reagentShopDoubleInset)
	minimumDetailHeight := int(math.Ceil(d.detailOutput.GetTotalHeightForLines(reagentShopDetailLines))) +
		inset*reagentShopDoubleInset
	naturalHeaderHeight := int(math.Ceil(d.catalogOutput.GetFontHeight())) + inset*reagentShopDoubleInset
	naturalRowHeight := naturalHeaderHeight
	offerCount := len(d.state.Offers)
	maximumCatalogHeight := max(offerCount+1, content.Dy()-minimumDetailHeight-gap)
	catalogHeight := min(naturalHeaderHeight+naturalRowHeight*offerCount, maximumCatalogHeight)
	headerHeight := min(naturalHeaderHeight, max(1, catalogHeight-offerCount))
	rowHeight := max(1, (catalogHeight-headerHeight)/offerCount)

	d.catalogHeader = image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+headerHeight)
	d.catalogRows = make([]image.Rectangle, len(d.state.Offers))
	for index := range d.catalogRows {
		top := d.catalogHeader.Max.Y + index*rowHeight
		d.catalogRows[index] = image.Rect(content.Min.X, top, content.Max.X, top+rowHeight)
	}
	d.catalogBounds = image.Rect(content.Min.X, content.Min.Y, content.Max.X, d.catalogRows[len(d.catalogRows)-1].Max.Y)
	d.detailBounds = image.Rect(
		content.Min.X,
		min(content.Max.Y, d.catalogBounds.Max.Y+gap),
		content.Max.X,
		content.Max.Y,
	)
	d.detailTextBounds = insetRectangle(d.detailBounds, inset)
	d.configureTextLayout()
}

func (d *ReagentShopDialog) configureTextLayout() {
	ratios := [...]float64{0, 0.43, 0.63, 0.81, 1}
	for index := range reagentShopCatalogColumns {
		left := d.catalogBounds.Min.X + int(math.Round(float64(d.catalogBounds.Dx())*ratios[index]))
		right := d.catalogBounds.Min.X + int(math.Round(float64(d.catalogBounds.Dx())*ratios[index+1]))
		d.catalogColumns[index] = image.Rect(left, d.catalogBounds.Min.Y, right, d.catalogBounds.Max.Y)
	}
	d.detailOutput = text.NewOutput(
		d.detailFont,
		d.detailOutput.GetLineSpacing(),
		max(1, d.detailOutput.GetLineCapacity(max(1, d.detailTextBounds.Dy()))),
		max(reagentShopMinimumTextChars, d.detailFont.CharsForWidth(max(1, d.detailTextBounds.Dx()))),
	)
}

func (d *ReagentShopDialog) drawCatalog(screen *ebiten.Image) {
	drawMerchantPane(screen, d.catalogBounds, d.pixelUnit())
	for index, heading := range d.GetCatalogHeadings() {
		d.drawCatalogCell(screen, d.catalogHeader, d.catalogColumns[index], heading, DialogLabelColor, etext.AlignCenter)
	}
	d.drawHorizontalRule(screen, d.catalogHeader, DialogRuleColor)
	start, end := d.listState.GetVisibleRange()
	for index := start; index < end; index++ {
		row := d.catalogRows[index-start]
		selected := index == d.GetSelectedIndex()
		if selected {
			fillSelectedRow(screen, row)
		}
		nameColor, valueColor := reagentCatalogRowColors(d.state.Offers[index], selected)
		values := d.GetCatalogRowText(index)
		for column, value := range values {
			alignment := etext.AlignCenter
			color := valueColor
			if column == 0 {
				alignment = etext.AlignStart
				color = nameColor
			}
			d.drawCatalogCell(screen, row, d.catalogColumns[column], value, color, alignment)
		}
		if index < end-1 {
			d.drawHorizontalRule(screen, row, DialogRuleLightColor)
		}
	}
}

func (d *ReagentShopDialog) drawCatalogCell(
	screen *ebiten.Image,
	row image.Rectangle,
	column image.Rectangle,
	value string,
	color stdcolor.Color,
	alignment etext.Align,
) {
	inset := max(1, d.pixelUnit()*reagentShopCellInsetUnits)
	drawMerchantTableCell(screen, d.catalogOutput, row, column, inset, value, color, alignment)
}

func (d *ReagentShopDialog) drawHorizontalRule(screen *ebiten.Image, bounds image.Rectangle, color stdcolor.Color) {
	vector.StrokeLine(screen, float32(bounds.Min.X), float32(bounds.Max.Y), float32(bounds.Max.X),
		float32(bounds.Max.Y), float32(d.pixelUnit()), color, false)
}

func (d *ReagentShopDialog) drawDetail(screen *ebiten.Image) {
	drawMerchantPane(screen, d.detailBounds, d.pixelUnit())
	d.detailOutput.DrawPanelContent(
		screen,
		image.Pt(d.detailTextBounds.Min.X, verticallyCenteredOutputY(d.detailTextBounds, d.detailOutput)),
		max(1, d.detailTextBounds.Dx()),
		etext.AlignStart,
	)
}

func (d *ReagentShopDialog) populateDetailOutput() {
	if d.detailOutput == nil {
		return
	}
	d.detailOutput.Reset()
	if d.receiptText != "" {
		if d.preview {
			d.detailOutput.AppendLine().Text("PREVIEW", DialogStatusWarningColor).Done()
		}
		d.detailOutput.AppendLine().Text(d.receiptText, DialogValueColor).WordWrap().Done()

		return
	}
	quote := d.selectedQuote()
	header := d.detailOutput.AppendLine()
	if d.preview {
		header.Text("PREVIEW - ", DialogStatusWarningColor)
	}
	header.Text(quote.ReagentName, DialogHeaderColor).Done()
	switch quote.BlockReason {
	case game_state.ReagentOfferBlockNone:
		d.detailOutput.AppendLine().Textf(
			"Per bundle: %d %s for %d gold.",
			DialogValueColor,
			quote.BundleQuantity,
			quote.ReagentName,
			quote.PricePerBundle,
		).Done()
		d.detailOutput.AppendLine().Textf(
			"Max: %d bundles.", DialogDeltaPositive, quote.MaxBundles,
		).Done()
	case game_state.ReagentOfferBlockInventoryFull:
		d.detailOutput.AppendLine().Textf(
			"Thou canst not carry any more %s!",
			DialogStatusWarningColor,
			quote.ReagentName,
		).WordWrap().Done()
	case game_state.ReagentOfferBlockUnaffordable:
		d.detailOutput.AppendLine().Textf(
			"Cannot buy %s: need %d gold per bundle; party has %d.",
			DialogStatusWarningColor,
			quote.ReagentName,
			quote.PricePerBundle,
			quote.Gold,
		).WordWrap().Done()
	default:
		panic("ReagentShopDialog.populateDetailOutput: unknown block reason")
	}
}

func (d *ReagentShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateDetailOutput()
}

func (d *ReagentShopDialog) footerGuidance() string {
	switch d.phase {
	case ReagentShopBrowse:
		return "[Up/Down] Select   [Enter] Buy   [Esc] Leave"
	case ReagentShopAwaitingQuantity:
		return "Choose purchase quantity"
	case ReagentShopAwaitingResult:
		return ""
	case ReagentShopFarewell, ReagentShopSessionError:
		return merchantLeaveFooter
	default:
		panic("ReagentShopDialog.footerGuidance: unknown phase")
	}
}

func (d *ReagentShopDialog) moveSelection(direction int) {
	current := d.GetSelectedIndex()
	switch {
	case direction < 0:
		if current == 0 {
			d.listState.SetIndex(len(d.state.Offers) - 1)
		} else {
			d.listState.MoveUp()
		}
	case current == len(d.state.Offers)-1:
		d.listState.SetIndex(0)
	default:
		d.listState.MoveDown()
	}
	d.receiptText = ""
	d.populateDetailOutput()
}

func (d *ReagentShopDialog) emitPurchaseIntent() {
	if d.phase != ReagentShopBrowse || d.pendingIntent != nil {
		return
	}
	quote := d.selectedQuote()
	if quote.BlockReason.IsBlocked() {
		return
	}
	intent := ReagentPurchaseIntent{Quote: cloneReagentOfferQuote(quote)}
	d.pendingIntent = &intent
	d.awaitingQuote = cloneReagentOfferQuote(quote)
	d.transitionTo(ReagentShopAwaitingQuantity)
	d.syncPresentation()
}

func (d *ReagentShopDialog) showFarewell() {
	d.receiptText = ""
	d.latestExchange = d.prose.RudeFarewell
	if d.purchased {
		d.latestExchange = d.prose.PoliteFarewell
	}
	d.transitionTo(ReagentShopFarewell)
	d.syncPresentation()
}

func (d *ReagentShopDialog) selectedQuote() game_state.ReagentOfferQuote {
	return d.state.Offers[d.GetSelectedIndex()]
}

func (d *ReagentShopDialog) transitionTo(phase ReagentShopDialogPhase) {
	if d.phase == phase {
		return
	}
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
}

func (d *ReagentShopDialog) clearAwaitingPurchase() {
	d.pendingIntent = nil
	d.awaitingQuote = emptyReagentOfferQuote()
	d.awaitingRequest = emptyReagentPurchaseRequest()
}

func (d *ReagentShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *ReagentShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func reagentCatalogRowColors(quote game_state.ReagentOfferQuote, selected bool) (stdcolor.Color, stdcolor.Color) {
	if selected {
		return DialogSelectedText, DialogSelectedText
	}
	switch quote.BlockReason {
	case game_state.ReagentOfferBlockNone:
		return DialogItemNameColor, DialogValueColor
	case game_state.ReagentOfferBlockInventoryFull:
		return DialogEmptyColor, DialogEmptyColor
	case game_state.ReagentOfferBlockUnaffordable:
		return DialogEmptyColor, DialogEmptyColor
	default:
		panic("ReagentShopDialog: unknown reagent offer block reason")
	}
}

func validateReagentShopState(state game_state.ReagentShopState) {
	validateReagentShopIdentity(state)
	validateReagentShopOffers(state)
}

func validateReagentShopIdentity(state game_state.ReagentShopState) {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyReagent ||
		strings.TrimSpace(state.Merchant.OwnerName) == "" || strings.TrimSpace(state.Merchant.ShopName) == "" {
		panic("ReagentShopDialog: detached reagent shop state is required")
	}
	if state.PricingCharacterIndex < 0 || strings.TrimSpace(state.PricingCharacterName) == "" ||
		state.PricingIntelligence < 0 {
		panic("ReagentShopDialog: detached pricing character is invalid")
	}
	if len(state.Offers) < reagentShopMinimumOffers || len(state.Offers) > reagentShopMaximumOffers {
		panic("ReagentShopDialog: reagent catalog must contain three to five offers")
	}
}

func validateReagentShopOffers(state game_state.ReagentShopState) {
	seen := make(map[references.Reagent]struct{}, len(state.Offers))
	for _, quote := range state.Offers {
		if _, exists := seen[quote.Reagent]; exists {
			panic("ReagentShopDialog: duplicate reagent offer")
		}
		seen[quote.Reagent] = struct{}{}
		validateReagentOfferQuote(quote, state.Gold)
	}
}

func validateReagentOfferQuote(quote game_state.ReagentOfferQuote, stateGold uint16) {
	validateReagentOfferIdentity(quote)
	validateReagentOfferAvailability(quote, stateGold)
	validateReagentOfferApprovals(quote)
}

func validateReagentOfferIdentity(quote game_state.ReagentOfferQuote) {
	if quote.Reagent < references.SulfurAsh || quote.Reagent > references.MandrakeRoot ||
		strings.TrimSpace(quote.ReagentName) == "" {
		panic("ReagentShopDialog: known reagent identity and name are required")
	}
	if quote.BundleQuantity == 0 || quote.BasePrice <= 0 || quote.PricePerBundle <= 0 {
		panic("ReagentShopDialog: positive bundle quantity and prices are required")
	}
}

func validateReagentOfferAvailability(quote game_state.ReagentOfferQuote, stateGold uint16) {
	if quote.Gold != stateGold || quote.QuantityBefore > party_state.ItemQuantitySmallMax {
		panic("ReagentShopDialog: reagent quote does not match the detached inventory snapshot")
	}

	switch quote.BlockReason {
	case game_state.ReagentOfferBlockNone:
		validateAvailableReagentOffer(quote)
	case game_state.ReagentOfferBlockInventoryFull:
		validateFullReagentOffer(quote)
	case game_state.ReagentOfferBlockUnaffordable:
		validateUnaffordableReagentOffer(quote)
	default:
		panic("ReagentShopDialog: unknown reagent offer block reason")
	}
}

func validateAvailableReagentOffer(quote game_state.ReagentOfferQuote) {
	if quote.Full || quote.UsefulBundles == 0 || quote.AffordableBundles == 0 || quote.MaxBundles == 0 ||
		quote.MaxBundles != min(quote.UsefulBundles, quote.AffordableBundles) {
		panic("ReagentShopDialog: available reagent quote has inconsistent bounds")
	}
}

func validateFullReagentOffer(quote game_state.ReagentOfferQuote) {
	if !quote.Full || quote.QuantityBefore != party_state.ItemQuantitySmallMax || quote.UsefulBundles != 0 ||
		quote.MaxBundles != 0 {
		panic("ReagentShopDialog: full reagent quote is inconsistent")
	}
}

func validateUnaffordableReagentOffer(quote game_state.ReagentOfferQuote) {
	if quote.Full || quote.QuantityBefore == party_state.ItemQuantitySmallMax || quote.AffordableBundles != 0 ||
		quote.MaxBundles != 0 {
		panic("ReagentShopDialog: unaffordable reagent quote is inconsistent")
	}
}

func validateReagentOfferApprovals(quote game_state.ReagentOfferQuote) {
	if len(quote.ApprovalViews) != int(quote.MaxBundles) {
		panic("ReagentShopDialog: reagent quote bounds or block reason are inconsistent")
	}
	for index, approval := range quote.ApprovalViews {
		validateReagentApprovalView(quote, approval, uint16(index+1))
	}
}

func validateReagentApprovalView(
	quote game_state.ReagentOfferQuote,
	approval game_state.ReagentPurchaseApprovalView,
	bundleCount uint16,
) {
	validateReagentApprovalIdentity(quote, approval, bundleCount)
	validateReagentApprovalInventory(quote, approval)
	validateReagentApprovalPayment(quote, approval)
}

func validateReagentApprovalIdentity(
	quote game_state.ReagentOfferQuote,
	approval game_state.ReagentPurchaseApprovalView,
	bundleCount uint16,
) {
	if approval.Reagent != quote.Reagent || approval.ReagentName != quote.ReagentName ||
		approval.BundleQuantity != quote.BundleQuantity || approval.BundleCount != bundleCount ||
		approval.MaxBundles != quote.MaxBundles {
		panic("ReagentShopDialog: reagent approval identity is inconsistent")
	}
}

func validateReagentApprovalInventory(
	quote game_state.ReagentOfferQuote,
	approval game_state.ReagentPurchaseApprovalView,
) {
	if approval.RequestedQuantity == 0 || approval.QuantityBefore != quote.QuantityBefore ||
		approval.QuantityAfter < approval.QuantityBefore || approval.QuantityAfter > party_state.ItemQuantitySmallMax ||
		approval.ActualReceipt != approval.QuantityAfter-approval.QuantityBefore ||
		approval.ActualReceipt > approval.RequestedQuantity ||
		approval.PaidOverflow != (approval.ActualReceipt < approval.RequestedQuantity) ||
		approval.OverflowQuantity != approval.RequestedQuantity-approval.ActualReceipt {
		panic("ReagentShopDialog: reagent approval inventory projection is inconsistent")
	}
}

func validateReagentApprovalPayment(
	quote game_state.ReagentOfferQuote,
	approval game_state.ReagentPurchaseApprovalView,
) {
	if approval.PricePerBundle != quote.PricePerBundle || approval.TotalPrice <= 0 ||
		approval.GoldBefore != quote.Gold || approval.GoldAfterPayment > approval.GoldBefore ||
		approval.TotalPrice != int(approval.GoldBefore-approval.GoldAfterPayment) {
		panic("ReagentShopDialog: reagent approval payment projection is inconsistent")
	}
}

func emptyReagentOfferQuote() game_state.ReagentOfferQuote {
	return game_state.ReagentOfferQuote{
		Reagent:           references.SulfurAsh,
		ReagentName:       "",
		BundleQuantity:    0,
		BasePrice:         0,
		PricePerBundle:    0,
		Gold:              0,
		QuantityBefore:    0,
		UsefulBundles:     0,
		AffordableBundles: 0,
		MaxBundles:        0,
		Full:              false,
		BlockReason:       game_state.ReagentOfferBlockNone,
		ApprovalViews:     nil,
	}
}

func reagentOfferIndex(offers []game_state.ReagentOfferQuote, reagent references.Reagent) int {
	for index, quote := range offers {
		if quote.Reagent == reagent {
			return index
		}
	}
	panic("ReagentShopDialog: selected reagent is missing after refresh")
}

func cloneReagentShopState(state game_state.ReagentShopState) game_state.ReagentShopState {
	clone := state
	clone.Offers = make([]game_state.ReagentOfferQuote, len(state.Offers))
	for index, quote := range state.Offers {
		clone.Offers[index] = cloneReagentOfferQuote(quote)
	}

	return clone
}

func cloneReagentOfferQuote(quote game_state.ReagentOfferQuote) game_state.ReagentOfferQuote {
	clone := quote
	clone.ApprovalViews = append([]game_state.ReagentPurchaseApprovalView(nil), quote.ApprovalViews...)

	return clone
}

func validateReagentFinalizedText(label, value string) {
	if strings.TrimSpace(value) == "" {
		panic(fmt.Sprintf("ReagentShopDialog: finalized %s is required", label))
	}
}
