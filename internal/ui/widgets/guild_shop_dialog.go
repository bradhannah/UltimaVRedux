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
	guildShopOfferCount           = 3
	guildShopCatalogColumns       = 4
	guildShopCatalogWidthRatio    = 0.94
	guildShopMinimumInset         = 4
	guildShopInsetDivisor         = 48
	guildShopCenterDivisor        = 2
	guildShopDoubleInset          = 2
	guildShopHeaderBreathingUnits = 3
	guildShopLineSpacing          = 1.08
	guildShopMinimumTextChars     = 8
	guildShopStatusMinimumLines   = 3
)

var _ Widget = (*GuildShopDialog)(nil)

// GuildShopDialogPhase identifies the workflow-owned purchase phase.
type GuildShopDialogPhase int

const (
	// GuildShopBrowse allows keyboard catalog selection.
	GuildShopBrowse GuildShopDialogPhase = iota
	// GuildShopAwaitingQuantity blocks the shop while its nested quantity dialog is active.
	GuildShopAwaitingQuantity
	// GuildShopAwaitingResult blocks input while the authority commits a purchase.
	GuildShopAwaitingResult
	// GuildShopError displays a fatal session error until the player leaves.
	GuildShopError
)

// GuildShopDialogProse contains finalized merchant text. The widget never resolves records.
type GuildShopDialogProse struct {
	Greeting     string
	OfferPitches [guildShopOfferCount]string
}

// GuildShopDialogConfig configures one concrete, detached guild shop.
type GuildShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	State          game_state.GuildShopState
	Prose          GuildShopDialogProse
	Preview        bool
	IsKeyPressed   func(ebiten.Key) bool
}

// GuildPurchaseIntent carries the selected trusted quote and its original finalized pitch.
type GuildPurchaseIntent struct {
	Quote game_state.GuildOfferQuote
	Pitch string
}

// GuildPurchaseResultDisplay contains finalized text supplied after an authoritative purchase.
type GuildPurchaseResultDisplay struct {
	MerchantExchange string
	Receipt          string
}

// GuildShopDialog presents the compact guild catalog and emits quantity-selection intents.
type GuildShopDialog struct {
	navigation
	keyboard       *input.Keyboard
	isKeyPressed   func(ebiten.Key) bool
	state          game_state.GuildShopState
	prose          GuildShopDialogProse
	preview        bool
	offers         [guildShopOfferCount]game_state.GuildOfferQuote
	listState      *ScrollableListState
	phase          GuildShopDialogPhase
	dismissed      bool
	latestExchange string
	receiptText    string

	pendingIntent   *GuildPurchaseIntent
	awaitingQuote   game_state.GuildOfferQuote
	awaitingRequest game_state.GuildPurchaseRequest

	shell         *MerchantDialogShell
	catalogOutput *text.Output
	statusOutput  *text.Output
	catalogFont   *text.UltimaFont
	statusFont    *text.UltimaFont

	catalogBounds  image.Rectangle
	catalogHeader  image.Rectangle
	catalogRows    [guildShopOfferCount]image.Rectangle
	catalogColumns [guildShopCatalogColumns]image.Rectangle
	statusBounds   image.Rectangle
	statusText     image.Rectangle

	lastScreenWidth    int
	lastScreenHeight   int
	directionKeysReady bool
}

// NewGuildShopDialog creates a visible guild shop from an authoritative detached snapshot.
func NewGuildShopDialog(config GuildShopDialogConfig) *GuildShopDialog {
	if config.Keyboard == nil {
		panic("GuildShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("GuildShopDialog: merchant sprite is required")
	}
	offers := validatedGuildOffers(config.State)
	validateGuildShopProse(config.Prose)

	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = config.Keyboard.IsKeyHeld
	}
	dialog := buildGuildShopDialog(config, offers, isKeyPressed)
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

func buildGuildShopDialog(
	config GuildShopDialogConfig,
	offers [guildShopOfferCount]game_state.GuildOfferQuote,
	isKeyPressed func(ebiten.Key) bool,
) *GuildShopDialog {
	var emptyQuote game_state.GuildOfferQuote
	var emptyRequest game_state.GuildPurchaseRequest
	var emptyBounds image.Rectangle
	var catalogRows [guildShopOfferCount]image.Rectangle
	var catalogColumns [guildShopCatalogColumns]image.Rectangle

	return &GuildShopDialog{
		navigation:         newNavigation(isKeyPressed),
		keyboard:           config.Keyboard,
		isKeyPressed:       isKeyPressed,
		state:              config.State,
		prose:              config.Prose,
		preview:            config.Preview,
		offers:             offers,
		listState:          NewScrollableListState(guildShopOfferCount, guildShopOfferCount),
		phase:              GuildShopBrowse,
		dismissed:          false,
		latestExchange:     config.Prose.Greeting,
		receiptText:        "",
		pendingIntent:      nil,
		awaitingQuote:      emptyQuote,
		awaitingRequest:    emptyRequest,
		shell:              nil,
		catalogOutput:      nil,
		statusOutput:       nil,
		catalogFont:        nil,
		statusFont:         nil,
		catalogBounds:      emptyBounds,
		catalogHeader:      emptyBounds,
		catalogRows:        catalogRows,
		catalogColumns:     catalogColumns,
		statusBounds:       emptyBounds,
		statusText:         emptyBounds,
		lastScreenWidth:    0,
		lastScreenHeight:   0,
		directionKeysReady: !isKeyPressed(ebiten.KeyUp) && !isKeyPressed(ebiten.KeyDown),
	}
}

// Draw renders the shared shell, compact catalog, and selection or receipt status.
func (d *GuildShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	d.drawCatalog(screen)
	d.drawStatus(screen)
}

// Update processes keyboard-only shop input.
func (d *GuildShopDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}

	switch d.phase {
	case GuildShopAwaitingQuantity, GuildShopAwaitingResult:
		return
	case GuildShopError:
		d.updateError()
	case GuildShopBrowse:
		d.updateBrowse()
	default:
		panic("GuildShopDialog.Update: unknown phase")
	}
}

func (d *GuildShopDialog) updateError() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) || d.enterPressed() {
		d.Dismiss()
	}
}

func (d *GuildShopDialog) updateBrowse() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()

		return
	}
	if d.handleDirectionalNavigation() {
		return
	}
	if d.enterPressed() {
		d.emitPurchaseIntent()
	}
}

// ConsumePendingPurchaseIntent returns and clears the one pending quantity-selection intent.
func (d *GuildShopDialog) ConsumePendingPurchaseIntent() (GuildPurchaseIntent, bool) {
	if d.pendingIntent == nil {
		var quote game_state.GuildOfferQuote

		return GuildPurchaseIntent{Quote: quote, Pitch: ""}, false
	}
	intent := *d.pendingIntent
	d.pendingIntent = nil

	return intent, true
}

// CancelQuantitySelection returns to browsing without changing the latest exchange.
func (d *GuildShopDialog) CancelQuantitySelection() {
	if d.phase != GuildShopAwaitingQuantity {
		panic("GuildShopDialog.CancelQuantitySelection: dialog is not awaiting quantity")
	}
	d.pendingIntent = nil
	d.transitionTo(GuildShopBrowse)
	d.syncPresentation()
}

// AcceptQuantitySelection records the modal's validated request and awaits authority.
func (d *GuildShopDialog) AcceptQuantitySelection(request game_state.GuildPurchaseRequest) {
	if d.phase != GuildShopAwaitingQuantity {
		panic("GuildShopDialog.AcceptQuantitySelection: dialog is not awaiting quantity")
	}
	if request.Provision != d.awaitingQuote.Provision ||
		request.BundleCount == 0 ||
		request.BundleCount > d.awaitingQuote.MaxBundles {
		panic("GuildShopDialog.AcceptQuantitySelection: request does not match the selected quote")
	}
	d.pendingIntent = nil
	d.awaitingRequest = request
	d.transitionTo(GuildShopAwaitingResult)
	d.syncPresentation()
}

// ApplyPurchaseResult applies authoritative result data, refreshed quotes, and finalized display text.
func (d *GuildShopDialog) ApplyPurchaseResult(
	result game_state.GuildPurchaseResult,
	refreshed game_state.GuildShopState,
	display GuildPurchaseResultDisplay,
) {
	if d.phase != GuildShopAwaitingResult {
		panic("GuildShopDialog.ApplyPurchaseResult: dialog is not awaiting a result")
	}
	if result.Provision != d.awaitingRequest.Provision ||
		result.BundleCount == 0 || result.BundleCount > d.awaitingRequest.BundleCount {
		panic("GuildShopDialog.ApplyPurchaseResult: result does not match the accepted request")
	}
	if result.BundleCount < d.awaitingRequest.BundleCount {
		display.Receipt += fmt.Sprintf(" Batch stopped: %d of %d bundles purchased.",
			result.BundleCount, d.awaitingRequest.BundleCount)
	}
	validateFinalizedMerchantText("purchase exchange", display.MerchantExchange)
	validateFinalizedMerchantText("purchase receipt", display.Receipt)
	offers := validatedGuildOffers(refreshed)
	selectedProvision := d.awaitingRequest.Provision

	d.state = refreshed
	d.offers = offers
	d.listState.SetIndex(guildOfferIndex(d.offers, selectedProvision))
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaitingPurchase()
	d.transitionTo(GuildShopBrowse)
	d.syncPresentation()
}

// ApplyPreviewStopped returns to Browse at the transaction authority boundary
// and clearly records that the reviewed request was not committed.
func (d *GuildShopDialog) ApplyPreviewStopped(request game_state.GuildPurchaseRequest) {
	if d.phase != GuildShopAwaitingResult {
		panic("GuildShopDialog.ApplyPreviewStopped: dialog is not awaiting a result")
	}
	if request != d.awaitingRequest {
		panic("GuildShopDialog.ApplyPreviewStopped: request does not match the accepted preview")
	}

	d.latestExchange = "Preview stopped before purchase authority."
	d.receiptText = "Preview only: no purchase was committed; gold and inventory are unchanged."
	d.clearAwaitingPurchase()
	d.transitionTo(GuildShopBrowse)
	d.syncPresentation()
}

// ApplySessionError replaces the latest exchange and disables further purchases.
func (d *GuildShopDialog) ApplySessionError(message string) {
	validateFinalizedMerchantText("session error", message)
	d.pendingIntent = nil
	d.clearAwaitingPurchase()
	d.latestExchange = message
	d.receiptText = ""
	d.transitionTo(GuildShopError)
	d.syncPresentation()
}

// GetSelectedQuote returns the currently selected trusted quote.
func (d *GuildShopDialog) GetSelectedQuote() game_state.GuildOfferQuote {
	return d.selectedQuote()
}

// GetShell returns this concrete dialog's shared presentation shell.
func (d *GuildShopDialog) GetShell() *MerchantDialogShell {
	return d.shell
}

// IsDismissed reports whether the dialog may be removed.
func (d *GuildShopDialog) IsDismissed() bool {
	return d.dismissed
}

// Dismiss closes the shop without producing a purchase request.
func (d *GuildShopDialog) Dismiss() {
	d.dismissed = true
}

func (d *GuildShopDialog) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if d.catalogOutput != nil && screenWidth == d.lastScreenWidth && screenHeight == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth = screenWidth
	d.lastScreenHeight = screenHeight
	d.shell.GetBounds()

	d.catalogFont, d.catalogOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: guildShopLineSpacing,
		MaxLines:              1,
		MaxChars:              guildShopMinimumTextChars,
	})
	d.statusFont, d.statusOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: guildShopLineSpacing,
		MaxLines:              guildShopStatusMinimumLines,
		MaxChars:              guildShopMinimumTextChars,
	})
	d.calculateCatalogLayout()
	d.populateWorkspaceOutputs()
}

func (d *GuildShopDialog) calculateCatalogLayout() {
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(guildShopMinimumInset, min(workspace.Dx(), workspace.Dy())/guildShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	catalogWidth := max(1, int(math.Round(float64(content.Dx())*guildShopCatalogWidthRatio)))
	catalogLeft := content.Min.X + (content.Dx()-catalogWidth)/guildShopCenterDivisor
	pixelUnit := d.pixelUnit()
	gap := max(inset, pixelUnit*guildShopDoubleInset)
	naturalRowHeight := int(math.Ceil(d.catalogOutput.GetFontHeight())) +
		max(inset, pixelUnit*guildShopDoubleInset)
	naturalHeaderHeight := naturalRowHeight + max(
		inset*guildShopDoubleInset,
		pixelUnit*guildShopHeaderBreathingUnits,
	)
	minimumStatusHeight := int(math.Ceil(d.statusOutput.GetTotalHeightForLines(guildShopStatusMinimumLines))) +
		inset*guildShopDoubleInset
	naturalCatalogHeight := naturalHeaderHeight + naturalRowHeight*guildShopOfferCount
	maximumCatalogHeight := max(
		guildShopOfferCount+1,
		content.Dy()-minimumStatusHeight-gap,
	)
	catalogHeight := min(naturalCatalogHeight, maximumCatalogHeight)
	headerHeight := min(naturalHeaderHeight, max(1, catalogHeight-guildShopOfferCount))
	rowHeight := max(1, (catalogHeight-headerHeight)/guildShopOfferCount)

	d.catalogHeader = image.Rect(catalogLeft, content.Min.Y, catalogLeft+catalogWidth, content.Min.Y+headerHeight)
	for index := range guildShopOfferCount {
		top := d.catalogHeader.Max.Y + index*rowHeight
		d.catalogRows[index] = image.Rect(catalogLeft, top, catalogLeft+catalogWidth, top+rowHeight)
	}
	d.catalogBounds = image.Rect(
		catalogLeft,
		content.Min.Y,
		catalogLeft+catalogWidth,
		d.catalogRows[guildShopOfferCount-1].Max.Y,
	)
	d.statusBounds = image.Rect(
		catalogLeft,
		min(content.Max.Y, d.catalogBounds.Max.Y+gap),
		catalogLeft+catalogWidth,
		content.Max.Y,
	)
	d.statusText = insetRectangle(d.statusBounds, inset)
	d.configureCatalogColumns()
	d.configureStatusOutput()
}

func (d *GuildShopDialog) configureCatalogColumns() {
	columnRatios := [...]float64{0, 0.45, 0.65, 0.83, 1}
	for index := range guildShopCatalogColumns {
		left := d.catalogBounds.Min.X + int(math.Round(float64(d.catalogBounds.Dx())*columnRatios[index]))
		right := d.catalogBounds.Min.X + int(math.Round(float64(d.catalogBounds.Dx())*columnRatios[index+1]))
		d.catalogColumns[index] = image.Rect(left, d.catalogHeader.Min.Y, right, d.catalogBounds.Max.Y)
	}
}

func (d *GuildShopDialog) configureStatusOutput() {
	d.statusOutput = text.NewOutput(
		d.statusFont,
		d.statusOutput.GetLineSpacing(),
		max(1, d.statusOutput.GetLineCapacity(max(1, d.statusText.Dy()))),
		max(guildShopMinimumTextChars, d.statusFont.CharsForWidth(max(1, d.statusText.Dx()))),
	)
}

func (d *GuildShopDialog) drawCatalog(screen *ebiten.Image) {
	drawMerchantPane(screen, d.catalogBounds, d.pixelUnit())

	headings := [...]string{"Item", "Bundle", "Owned", shopHeadingPrice}
	for index, heading := range headings {
		d.drawCatalogCell(screen, d.catalogHeader, d.catalogColumns[index], heading, DialogLabelColor, etext.AlignCenter)
	}
	vector.StrokeLine(
		screen,
		float32(d.catalogHeader.Min.X),
		float32(d.catalogHeader.Max.Y),
		float32(d.catalogHeader.Max.X),
		float32(d.catalogHeader.Max.Y),
		float32(d.pixelUnit()),
		DialogRuleColor,
		false,
	)

	for row, quote := range d.offers {
		rowBounds := d.catalogRows[row]
		selected := row == d.listState.GetCurrentIndex()
		if selected {
			fillSelectedRow(screen, rowBounds)
		}
		textColor := DialogItemNameColor
		if selected {
			textColor = DialogSelectedText
		}
		d.drawCatalogRowCells(screen, rowBounds, quote, textColor)
		if row < guildShopOfferCount-1 {
			vector.StrokeLine(
				screen,
				float32(rowBounds.Min.X),
				float32(rowBounds.Max.Y),
				float32(rowBounds.Max.X),
				float32(rowBounds.Max.Y),
				float32(d.pixelUnit()),
				DialogRuleLightColor,
				false,
			)
		}
	}
}

func (d *GuildShopDialog) drawCatalogRowCells(
	screen *ebiten.Image,
	rowBounds image.Rectangle,
	quote game_state.GuildOfferQuote,
	textColor stdcolor.Color,
) {
	values := [...]string{
		guildProvisionName(quote.Provision),
		strconv.FormatUint(uint64(quote.BundleQuantity), 10),
		strconv.FormatUint(uint64(quote.QuantityBefore), 10),
		strconv.Itoa(quote.PricePerBundle),
	}
	for index := range values {
		alignment := etext.AlignCenter
		if index == 0 {
			alignment = etext.AlignStart
		}
		d.drawCatalogCell(screen, rowBounds, d.catalogColumns[index], values[index], textColor, alignment)
	}
}

func (d *GuildShopDialog) drawCatalogCell(
	screen *ebiten.Image,
	rowBounds image.Rectangle,
	column image.Rectangle,
	value string,
	textColor stdcolor.Color,
	align etext.Align,
) {
	inset := d.pixelUnit() * guildShopDoubleInset
	cell := image.Rect(column.Min.X+inset, rowBounds.Min.Y, column.Max.X-inset, rowBounds.Max.Y)
	d.catalogOutput.Reset()
	d.catalogOutput.AppendLine().Text(value, textColor).Done()
	y := cell.Min.Y + max(0, (cell.Dy()-int(math.Ceil(d.catalogOutput.GetFontHeight())))/guildShopCenterDivisor)
	d.catalogOutput.DrawPanelContent(screen, image.Pt(cell.Min.X, y), max(1, cell.Dx()), align)
}

func (d *GuildShopDialog) drawStatus(screen *ebiten.Image) {
	drawMerchantPane(screen, d.statusBounds, d.pixelUnit())
	d.statusOutput.DrawPanelContent(
		screen,
		image.Pt(d.statusText.Min.X, verticallyCenteredOutputY(d.statusText, d.statusOutput)),
		max(1, d.statusText.Dx()),
		etext.AlignStart,
	)
}

func (d *GuildShopDialog) populateWorkspaceOutputs() {
	if d.statusOutput == nil {
		return
	}
	d.statusOutput.Reset()
	statusLine := d.statusOutput.AppendLine()
	if d.preview {
		statusLine.Text("PREVIEW - ", DialogStatusWarningColor)
	}
	if d.receiptText != "" {
		statusLine.Text(d.receiptText, DialogValueColor).WordWrap().Done()

		return
	}
	note, warning := guildSelectionNote(d.selectedQuote())
	noteColor := DialogNavHintColor
	if warning {
		noteColor = DialogStatusWarningColor
	}
	statusLine.Text(note, noteColor).Done()
}

func (d *GuildShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateWorkspaceOutputs()
}

func (d *GuildShopDialog) footerGuidance() string {
	switch d.phase {
	case GuildShopBrowse:
		return "[Up/Down] Select   [Enter] Buy   [Esc] Leave"
	case GuildShopAwaitingQuantity:
		return "Choose purchase quantity"
	case GuildShopAwaitingResult:
		return "Processing purchase"
	case GuildShopError:
		return merchantLeaveFooter
	default:
		panic("GuildShopDialog.footerGuidance: unknown phase")
	}
}

func (d *GuildShopDialog) emitPurchaseIntent() {
	if d.phase != GuildShopBrowse || d.pendingIntent != nil {
		return
	}
	index := d.listState.GetCurrentIndex()
	intent := GuildPurchaseIntent{
		Quote: d.offers[index],
		Pitch: d.prose.OfferPitches[index],
	}
	d.pendingIntent = &intent
	d.awaitingQuote = intent.Quote
	d.transitionTo(GuildShopAwaitingQuantity)
	d.syncPresentation()
}

func (d *GuildShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *GuildShopDialog) selectedQuote() game_state.GuildOfferQuote {
	return d.offers[d.listState.GetCurrentIndex()]
}

func (d *GuildShopDialog) moveSelection(direction int) {
	var changed bool
	if direction < 0 {
		changed = d.listState.MoveUp()
	} else {
		changed = d.listState.MoveDown()
	}
	if changed {
		d.receiptText = ""
		d.populateWorkspaceOutputs()
	}
}

func (d *GuildShopDialog) handleDirectionalNavigation() bool {
	upHeld := d.isKeyPressed(ebiten.KeyUp)
	downHeld := d.isKeyPressed(ebiten.KeyDown)
	if !d.directionKeysReady {
		if upHeld || downHeld {
			return true
		}
		d.directionKeysReady = true
		d.Reset()
	}
	key, owned := d.PollWithKeyState(d.keyboard, d.isKeyPressed)
	if key != nil {
		if delta := listNavigationDelta(*key, 0); delta != 0 {
			d.moveSelection(delta)
		}
	}

	return owned
}

func (d *GuildShopDialog) transitionTo(phase GuildShopDialogPhase) {
	if d.phase == phase {
		return
	}
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
	d.directionKeysReady = !d.isKeyPressed(ebiten.KeyUp) && !d.isKeyPressed(ebiten.KeyDown)
	d.Reset()
}

func (d *GuildShopDialog) clearAwaitingPurchase() {
	var quote game_state.GuildOfferQuote
	var request game_state.GuildPurchaseRequest
	d.awaitingQuote = quote
	d.awaitingRequest = request
}

func (d *GuildShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func validateGuildShopProse(prose GuildShopDialogProse) {
	validateFinalizedMerchantText("greeting", prose.Greeting)
	for _, pitch := range prose.OfferPitches {
		validateFinalizedMerchantText("offer pitch", pitch)
	}
}

func validateFinalizedMerchantText(label, value string) {
	if strings.TrimSpace(value) == "" {
		panic("GuildShopDialog: finalized " + label + " is required")
	}
}

func validatedGuildOffers(state game_state.GuildShopState) [guildShopOfferCount]game_state.GuildOfferQuote {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyGuild {
		panic("GuildShopDialog: detached guild shop state is required")
	}
	canonical := [...]references.Provision{references.Key, references.Gem, references.Torches}
	var ordered [guildShopOfferCount]game_state.GuildOfferQuote
	for target, provision := range canonical {
		found := false
		for _, quote := range state.Offers {
			if quote.Provision != provision {
				continue
			}
			if found {
				panic("GuildShopDialog: duplicate guild offer")
			}
			validateGuildOfferQuote(quote, state.Gold)
			ordered[target] = quote
			found = true
		}
		if !found {
			panic("GuildShopDialog: guild state must contain Keys, Gems, and Torches")
		}
	}

	return ordered
}

func validateGuildOfferQuote(quote game_state.GuildOfferQuote, stateGold uint16) {
	if quote.BundleQuantity == 0 || quote.BasePrice <= 0 || quote.PricePerBundle <= 0 {
		panic("GuildShopDialog: guild quote prices and bundle quantity must be positive")
	}
	if quote.Gold != stateGold || quote.QuantityBefore > party_state.ItemQuantitySmallMax {
		panic("GuildShopDialog: guild quote does not match the detached inventory snapshot")
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
		panic("GuildShopDialog: guild quote maximum is inconsistent with inventory or gold")
	}
}

func guildOfferIndex(
	offers [guildShopOfferCount]game_state.GuildOfferQuote,
	provision references.Provision,
) int {
	for index, quote := range offers {
		if quote.Provision == provision {
			return index
		}
	}
	panic("GuildShopDialog: selected provision missing after refresh")
}

func guildProvisionName(provision references.Provision) string {
	switch provision {
	case references.Key:
		return "Keys"
	case references.Gem:
		return "Gems"
	case references.Torches:
		return "Torches"
	case references.NoProvision, references.SkullKeys, references.Food, references.Gold:
		panic("GuildShopDialog: unsupported guild provision")
	default:
		panic("GuildShopDialog: unknown guild provision")
	}
}

func guildSelectionNote(quote game_state.GuildOfferQuote) (string, bool) {
	if quote.MaxBundles == 0 {
		if quote.QuantityBefore >= party_state.ItemQuantitySmallMax {
			return "Inventory max: 99.", true
		}

		return "Insufficient money.", true
	}

	return "", false
}
