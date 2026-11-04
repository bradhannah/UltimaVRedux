package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	armsShopColumnCount        = 4
	armsShopMaximumVisibleRows = 7
	armsShopMinimumInset       = 4
	armsShopInsetDivisor       = 64
	armsShopLineSpacing        = 1.05
	armsShopMinimumTextChars   = 8
	armsShopDetailLines        = 4
	armsShopCellInsetUnits     = 2
	armsShopIconInsetUnits     = 1
	armsShopBottomInsetUnits   = 2
	armsShopCenterDivisor      = 2
	armsShopSortCount          = 3
)

var _ Widget = (*ArmsShopDialog)(nil)

// ArmsShopMode identifies the persistent Arms table being browsed.
type ArmsShopMode int

const (
	// ArmsShopBuy browses equipment offered by the merchant.
	ArmsShopBuy ArmsShopMode = iota
	// ArmsShopSell browses equipment carried by the party.
	ArmsShopSell
)

func (m ArmsShopMode) String() string {
	switch m {
	case ArmsShopBuy:
		return "BUY"
	case ArmsShopSell:
		return "SELL"
	default:
		panic("ArmsShopMode.String: unknown mode")
	}
}

// ArmsShopSort identifies one presentation-only table ordering.
type ArmsShopSort int

const (
	// ArmsSortName orders rows alphabetically by equipment name.
	ArmsSortName ArmsShopSort = iota
	// ArmsSortQuantity orders rows by descending carried quantity.
	ArmsSortQuantity
	// ArmsSortValue orders rows by descending price or offer.
	ArmsSortValue
)

func (s ArmsShopSort) String() string {
	switch s {
	case ArmsSortName:
		return "NAME A-Z"
	case ArmsSortQuantity:
		return "QUANTITY HIGH-LOW"
	case ArmsSortValue:
		return "VALUE HIGH-LOW"
	default:
		panic("ArmsShopSort.String: unknown sort")
	}
}

// ArmsShopDialogPhase identifies the workflow-owned Arms phase.
type ArmsShopDialogPhase int

const (
	// ArmsShopBrowse accepts table navigation and trade selection.
	ArmsShopBrowse ArmsShopDialogPhase = iota
	// ArmsShopAwaitingTrade blocks the shop while the nested trade dialog owns input.
	ArmsShopAwaitingTrade
	// ArmsShopAwaitingResult blocks the shop while authority processes a request.
	ArmsShopAwaitingResult
	// ArmsShopTerminalRefusal shows a terminal authority refusal until acknowledged.
	ArmsShopTerminalRefusal
	// ArmsShopSessionError shows an unrecoverable session error until acknowledged.
	ArmsShopSessionError
)

// ArmsShopDialogProse contains finalized merchant copy used by the shop shell.
type ArmsShopDialogProse struct {
	Greeting        string
	BuyPrompt       string
	SellPrompt      string
	CapacityRefusal string
	FundsRefusal    string
}

// ArmsShopDialogConfig configures one detached, presentation-only Arms shop.
type ArmsShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	State          game_state.ArmsShopState
	Prose          ArmsShopDialogProse
	Preview        bool
	IsKeyPressed   func(ebiten.Key) bool
}

// ArmsTradeIntent carries exactly one trusted quote into the nested modal.
type ArmsTradeIntent struct {
	Mode      ArmsShopMode
	BuyQuote  game_state.ArmsBuyQuote
	SellQuote game_state.ArmsSellQuote
}

// Equipment returns the stable equipment identity selected in the shop.
func (i ArmsTradeIntent) Equipment() references.Equipment {
	if i.Mode == ArmsShopBuy {
		return i.BuyQuote.Equipment
	}

	return i.SellQuote.Equipment
}

// ArmsTradeRequest is the concrete buy-or-sell request emitted by ArmsTradeDialog.
type ArmsTradeRequest struct {
	Mode ArmsShopMode
	Buy  game_state.ArmsBuyRequest
	Sell game_state.ArmsSellRequest
}

// Equipment returns the request's stable equipment identity.
func (r ArmsTradeRequest) Equipment() references.Equipment {
	if r.Mode == ArmsShopBuy {
		return r.Buy.Equipment
	}

	return r.Sell.Equipment
}

// Quantity returns the requested positive transaction quantity.
func (r ArmsTradeRequest) Quantity() uint16 {
	if r.Mode == ArmsShopBuy {
		return r.Buy.Quantity
	}

	return r.Sell.Quantity
}

// ArmsTradeResultDisplay is finalized authority/workflow text for the shell and receipt strip.
type ArmsTradeResultDisplay struct {
	MerchantExchange string
	Receipt          string
}

// ArmsShopRowView exposes one sorted presentation row without exposing authority mutation.
type ArmsShopRowView struct {
	Equipment references.Equipment
	Name      string
	Quantity  uint16
	Readied   uint16
	Value     int
	Maximum   uint16
	ValueText string
	Available bool
	Reason    string
	Selected  bool
	IconColor stdcolor.Color
}

type armsShopModeState struct {
	sort     ArmsShopSort
	list     *ScrollableListState
	buyRows  []game_state.ArmsBuyQuote
	sellRows []game_state.ArmsSellQuote
}

// ArmsShopDialog presents sorted Buy and Sell tables and emits trade intents.
type ArmsShopDialog struct {
	navigation
	keyboard     *input.Keyboard
	isKeyPressed func(ebiten.Key) bool
	state        game_state.ArmsShopState
	prose        ArmsShopDialogProse
	preview      bool
	mode         ArmsShopMode
	modes        [2]armsShopModeState
	phase        ArmsShopDialogPhase
	dismissed    bool

	latestExchange  string
	receiptText     string
	pendingIntent   *ArmsTradeIntent
	awaitingIntent  ArmsTradeIntent
	awaitingRequest ArmsTradeRequest

	shell        *MerchantDialogShell
	headerOutput *text.Output
	sortOutput   *text.Output
	rowOutput    *text.Output
	detailOutput *text.Output
	rowFont      *text.UltimaFont

	workspaceContent image.Rectangle
	modeHeader       image.Rectangle
	tableHeader      image.Rectangle
	tableBounds      image.Rectangle
	sortBounds       image.Rectangle
	rowBounds        []image.Rectangle
	columns          [armsShopColumnCount]image.Rectangle
	detailBounds     image.Rectangle
	detailTextBounds image.Rectangle

	lastScreenWidth  int
	lastScreenHeight int
	directionsReady  bool
}

// NewArmsShopDialog creates a visible shop from a complete authoritative snapshot.
func NewArmsShopDialog(config ArmsShopDialogConfig) *ArmsShopDialog {
	if config.Keyboard == nil {
		panic("ArmsShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("ArmsShopDialog: merchant sprite is required")
	}
	validateArmsShopProse(config.Prose)
	state := validatedArmsShopState(config.State)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = config.Keyboard.IsKeyHeld
	}

	dialog := &ArmsShopDialog{ //nolint:exhaustruct_v5 // Layout and transient input fields are initialized on demand.
		navigation:      newNavigation(isKeyPressed),
		keyboard:        config.Keyboard,
		isKeyPressed:    isKeyPressed,
		state:           state,
		prose:           config.Prose,
		preview:         config.Preview,
		mode:            ArmsShopBuy,
		phase:           ArmsShopBrowse,
		latestExchange:  config.Prose.Greeting,
		directionsReady: !isKeyPressed(ebiten.KeyUp) && !isKeyPressed(ebiten.KeyDown),
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
	dialog.rebuildModeRows(ArmsShopBuy, references.NoEquipment, 0)
	dialog.rebuildModeRows(ArmsShopSell, references.NoEquipment, 0)
	dialog.ensureLayout()
	dialog.syncPresentation()

	return dialog
}

// Draw renders the shared merchant shell and active Arms table.
func (d *ArmsShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	d.drawWorkspace(screen)
}

// Update processes keyboard-only table input.
func (d *ArmsShopDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}

	switch d.phase {
	case ArmsShopAwaitingTrade, ArmsShopAwaitingResult:
		return
	case ArmsShopTerminalRefusal, ArmsShopSessionError:
		d.updateAcknowledgement()
	case ArmsShopBrowse:
		d.updateBrowse()
	default:
		panic("ArmsShopDialog.Update: unknown phase")
	}
}

func (d *ArmsShopDialog) updateAcknowledgement() {
	if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()
	}
}

func (d *ArmsShopDialog) updateBrowse() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()

		return
	}
	if d.handleModeInput() || d.handleSortInput() || d.handleDirectionalNavigation() {
		return
	}
	if d.enterPressed() {
		d.emitTradeIntent()
	}
}

// ConsumePendingTradeIntent returns and clears the one nested-modal intent.
func (d *ArmsShopDialog) ConsumePendingTradeIntent() (ArmsTradeIntent, bool) {
	if d.pendingIntent == nil {
		return emptyArmsTradeIntent(), false
	}
	intent := *d.pendingIntent
	d.pendingIntent = nil

	return intent, true
}

// CancelPendingTrade returns from the nested modal without changing the snapshot.
func (d *ArmsShopDialog) CancelPendingTrade() {
	if d.phase != ArmsShopAwaitingTrade {
		panic("ArmsShopDialog.CancelPendingTrade: dialog is not awaiting trade")
	}
	d.pendingIntent = nil
	d.awaitingIntent = emptyArmsTradeIntent()
	d.transitionTo(ArmsShopBrowse)
	d.syncPresentation()
}

// AcceptTradeIntent records the nested modal's typed request and awaits authority.
func (d *ArmsShopDialog) AcceptTradeIntent(request ArmsTradeRequest) {
	if d.phase != ArmsShopAwaitingTrade {
		panic("ArmsShopDialog.AcceptTradeIntent: dialog is not awaiting trade")
	}
	validateArmsRequestForIntent(d.awaitingIntent, request)
	d.pendingIntent = nil
	d.awaitingRequest = request
	d.transitionTo(ArmsShopAwaitingResult)
	d.syncPresentation()
}

// ApplyBuyResult refreshes authority data and displays the finalized purchase receipt.
func (d *ArmsShopDialog) ApplyBuyResult(
	result game_state.ArmsBuyResult,
	refreshed game_state.ArmsShopState,
	display ArmsTradeResultDisplay,
) {
	if d.phase != ArmsShopAwaitingResult || d.awaitingRequest.Mode != ArmsShopBuy ||
		result.Equipment != d.awaitingRequest.Buy.Equipment ||
		result.Quantity == 0 || result.Quantity > d.awaitingRequest.Buy.Quantity {
		panic("ArmsShopDialog.ApplyBuyResult: result does not match the accepted request")
	}
	if result.Quantity < d.awaitingRequest.Buy.Quantity {
		display.Receipt += fmt.Sprintf(" Batch stopped: %d of %d purchased.", result.Quantity, d.awaitingRequest.Buy.Quantity)
	}
	d.applyTransactionRefresh(refreshed, display, result.Equipment)
}

// ApplySellResult refreshes authority data and displays the finalized sale receipt.
func (d *ArmsShopDialog) ApplySellResult(
	result game_state.ArmsSellResult,
	refreshed game_state.ArmsShopState,
	display ArmsTradeResultDisplay,
) {
	if d.phase != ArmsShopAwaitingResult || d.awaitingRequest.Mode != ArmsShopSell ||
		result.Equipment != d.awaitingRequest.Sell.Equipment || result.Quantity != d.awaitingRequest.Sell.Quantity {
		panic("ArmsShopDialog.ApplySellResult: result does not match the accepted request")
	}
	d.applyTransactionRefresh(refreshed, display, result.Equipment)
}

// ApplyPreviewStopped returns to browsing and records that authority was not invoked.
func (d *ArmsShopDialog) ApplyPreviewStopped(request ArmsTradeRequest) {
	if d.phase != ArmsShopAwaitingResult || request != d.awaitingRequest {
		panic("ArmsShopDialog.ApplyPreviewStopped: request does not match the accepted preview")
	}
	d.latestExchange = "Preview stopped before Arms authority."
	d.receiptText = "Preview only: gold and equipment are unchanged."
	d.clearAwaiting()
	d.transitionTo(ArmsShopBrowse)
	d.syncPresentation()
}

// ApplyRefusal returns to browsing with finalized refusal text and no state mutation.
func (d *ArmsShopDialog) ApplyRefusal(display ArmsTradeResultDisplay) {
	if d.phase != ArmsShopAwaitingTrade && d.phase != ArmsShopAwaitingResult {
		panic("ArmsShopDialog.ApplyRefusal: dialog is not awaiting trade")
	}
	validateArmsResultDisplay(display)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaiting()
	d.transitionTo(ArmsShopBrowse)
	d.syncPresentation()
}

// ApplyRefusalWithRefresh returns to browsing after replacing stale authority state.
func (d *ArmsShopDialog) ApplyRefusalWithRefresh(
	refreshed game_state.ArmsShopState,
	display ArmsTradeResultDisplay,
) {
	if d.phase != ArmsShopAwaitingResult {
		panic("ArmsShopDialog.ApplyRefusalWithRefresh: dialog is not awaiting authority")
	}
	d.applyStateRefresh(refreshed, display, d.awaitingRequest.Equipment(), ArmsShopBrowse)
}

// ApplyTerminalRefusal shows a finalized authority refusal until Enter or Escape acknowledges it.
func (d *ArmsShopDialog) ApplyTerminalRefusal(display ArmsTradeResultDisplay) {
	if d.phase != ArmsShopAwaitingResult {
		panic("ArmsShopDialog.ApplyTerminalRefusal: dialog is not awaiting authority")
	}
	validateArmsResultDisplay(display)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaiting()
	d.transitionTo(ArmsShopTerminalRefusal)
	d.syncPresentation()
}

// ApplySessionError disables trading until the shop is dismissed.
func (d *ArmsShopDialog) ApplySessionError(message string) {
	validateArmsFinalizedText("session error", message)
	d.latestExchange = message
	d.receiptText = ""
	d.clearAwaiting()
	d.transitionTo(ArmsShopSessionError)
	d.syncPresentation()
}

// GetSort returns the active mode's independent ordering.
func (d *ArmsShopDialog) GetSort() ArmsShopSort { return d.activeMode().sort }

// GetSelectedIndex returns the selected index in the active sorted table.
func (d *ArmsShopDialog) GetSelectedIndex() int { return d.activeMode().list.GetCurrentIndex() }

// GetShell returns the common merchant shell.
func (d *ArmsShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// IsDismissed reports whether the shop can be removed.
func (d *ArmsShopDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the shop without emitting a request.
func (d *ArmsShopDialog) Dismiss() { d.dismissed = true }

func (d *ArmsShopDialog) activeMode() *armsShopModeState { return &d.modes[d.mode] }

func (d *ArmsShopDialog) rowCount() int {
	if d.mode == ArmsShopBuy {
		return len(d.activeMode().buyRows)
	}

	return len(d.activeMode().sellRows)
}

func (d *ArmsShopDialog) selectedEquipment() references.Equipment {
	mode := d.activeMode()
	index := mode.list.GetCurrentIndex()
	if d.mode == ArmsShopBuy {
		if len(mode.buyRows) == 0 {
			return references.NoEquipment
		}

		return mode.buyRows[index].Equipment
	}
	if len(mode.sellRows) == 0 {
		return references.NoEquipment
	}

	return mode.sellRows[index].Equipment
}

func (d *ArmsShopDialog) rebuildModeRows(mode ArmsShopMode, selected references.Equipment, fallbackIndex int) {
	modeState := &d.modes[mode]
	if modeState.list == nil {
		modeState.sort = ArmsSortName
	}
	if mode == ArmsShopBuy {
		modeState.buyRows = append(modeState.buyRows[:0], d.state.BuyQuotes[:]...)
		sortArmsBuyRows(modeState.buyRows, modeState.sort)
	} else {
		modeState.sellRows = append(modeState.sellRows[:0], d.state.SellQuotes...)
		sortArmsSellRows(modeState.sellRows, modeState.sort)
	}
	count := len(modeState.buyRows)
	if mode == ArmsShopSell {
		count = len(modeState.sellRows)
	}
	visible := armsShopMaximumVisibleRows
	if len(d.rowBounds) > 0 {
		visible = len(d.rowBounds)
	}
	modeState.list = NewScrollableListState(count, max(1, visible))
	index := min(fallbackIndex, count-1)
	if selected != references.NoEquipment {
		if found := d.equipmentIndex(mode, selected); found >= 0 {
			index = found
		}
	}
	modeState.list.SetIndex(max(0, index))
}

func (d *ArmsShopDialog) equipmentIndex(mode ArmsShopMode, equipment references.Equipment) int {
	modeState := &d.modes[mode]
	if mode == ArmsShopBuy {
		for index, quote := range modeState.buyRows {
			if quote.Equipment == equipment {
				return index
			}
		}
	} else {
		for index, quote := range modeState.sellRows {
			if quote.Equipment == equipment {
				return index
			}
		}
	}

	return -1
}

func (d *ArmsShopDialog) rowView(index int, selected bool) ArmsShopRowView {
	view := ArmsShopRowView{
		Equipment: references.NoEquipment,
		Name:      "",
		Quantity:  0,
		Readied:   0,
		Value:     0,
		Maximum:   0,
		ValueText: "",
		Available: false,
		Reason:    "",
		Selected:  selected,
		IconColor: pkgcolor.EGAYellow,
	}
	if selected {
		view.IconColor = pkgcolor.EGABlack
	}
	if d.mode == ArmsShopBuy {
		quote := d.activeMode().buyRows[index]
		view.Equipment = quote.Equipment
		view.Name = quote.Equipment.String()
		view.Quantity = quote.QuantityBefore
		view.Readied = quote.ReadiedQuantity
		view.Value = quote.UnitPrice
		view.Maximum = quote.MaxQuantity
		view.ValueText = strconv.Itoa(quote.UnitPrice)
		view.Available = quote.MaxQuantity > 0
		view.Reason = armsBuyUnavailableReason(quote)
		if !view.Available {
			view.IconColor = pkgcolor.EGADarkGray
		}

		return view
	}
	quote := d.activeMode().sellRows[index]
	view.Equipment = quote.Equipment
	view.Name = quote.Equipment.String()
	view.Quantity = quote.QuantityBefore
	view.Readied = quote.ReadiedQuantity
	view.Value = quote.UnitOffer
	view.Maximum = quote.MaxQuantity
	view.Available = quote.Sellable && quote.MaxQuantity > 0
	view.Reason = armsSellUnavailableReason(quote)
	switch {
	case quote.Equipment.IsAmmo():
		view.ValueText = "REFUSED"
	case quote.BasePrice == 0 || quote.UnitOffer == 0:
		view.ValueText = "NO OFFER"
	default:
		view.ValueText = strconv.Itoa(quote.UnitOffer)
	}
	if !view.Available {
		view.IconColor = pkgcolor.EGADarkGray
	}

	return view
}

func armsBuyUnavailableReason(quote game_state.ArmsBuyQuote) string {
	switch {
	case quote.MaxQuantity > 0:
		return ""
	case quote.QuantityBefore >= party_state.ItemQuantitySmallMax:
		return "Cannot buy: carried quantity is already 99."
	case int(quote.Gold) < quote.UnitPrice:
		return fmt.Sprintf("Cannot buy: need %d gold; party has %d.", quote.UnitPrice, quote.Gold)
	default:
		return "Cannot buy this equipment."
	}
}

func armsSellUnavailableReason(quote game_state.ArmsSellQuote) string {
	switch {
	case quote.Equipment.IsAmmo():
		return "Cannot sell: used ammunition is refused."
	case quote.BasePrice == 0 || quote.UnitOffer == 0 || !quote.Sellable:
		return "Cannot sell: this equipment has no offer."
	case quote.MaxQuantity == 0:
		return "Cannot sell: party gold has no room."
	default:
		return ""
	}
}

func (d *ArmsShopDialog) requestedMode() (ArmsShopMode, bool) {
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyB):
		return ArmsShopBuy, true
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyS):
		return ArmsShopSell, true
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab):
		if d.mode == ArmsShopBuy {
			return ArmsShopSell, true
		}

		return ArmsShopBuy, true
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft),
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight):
		if d.mode == ArmsShopBuy {
			return ArmsShopSell, true
		}

		return ArmsShopBuy, true
	default:
		return ArmsShopBuy, false
	}
}

func (d *ArmsShopDialog) handleModeInput() bool {
	next, requested := d.requestedMode()
	if !requested {
		return false
	}
	if next != d.mode {
		d.mode = next
		d.latestExchange = d.prose.BuyPrompt
		if next == ArmsShopSell {
			d.latestExchange = d.prose.SellPrompt
		}
		d.clearReceipt()
		d.calculateLayout()
		d.syncPresentation()
	}

	return true
}

func (d *ArmsShopDialog) requestedSort() (ArmsShopSort, bool) {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeyR) {
		return ArmsSortName, false
	}

	return ArmsShopSort((int(d.GetSort()) + 1) % armsShopSortCount), true
}

func (d *ArmsShopDialog) handleSortInput() bool {
	next, requested := d.requestedSort()
	if !requested {
		return false
	}
	if next != d.GetSort() {
		selected := d.selectedEquipment()
		fallback := d.GetSelectedIndex()
		d.activeMode().sort = next
		d.rebuildModeRows(d.mode, selected, fallback)
		d.clearReceipt()
		d.populateWorkspaceOutput()
	}

	return true
}

func (d *ArmsShopDialog) handleDirectionalNavigation() bool {
	upHeld := d.isKeyPressed(ebiten.KeyUp)
	downHeld := d.isKeyPressed(ebiten.KeyDown)
	if !d.directionsReady {
		if upHeld || downHeld {
			return true
		}
		d.directionsReady = true
		d.Reset()
	}
	key, owned := d.PollWithKeyState(d.keyboard, d.isKeyPressed)
	if key != nil && navigateScrollableList(d.activeMode().list, *key) {
		d.clearReceipt()
		d.populateWorkspaceOutput()
	}

	return owned
}

func (d *ArmsShopDialog) emitTradeIntent() {
	if d.phase != ArmsShopBrowse || d.pendingIntent != nil || d.rowCount() == 0 {
		return
	}
	if d.mode == ArmsShopBuy && d.activeMode().buyRows[d.GetSelectedIndex()].MaxQuantity == 0 {
		d.applyUnavailableBuyRefusal(d.activeMode().buyRows[d.GetSelectedIndex()])

		return
	}
	intent := ArmsTradeIntent{
		Mode:      d.mode,
		BuyQuote:  emptyArmsBuyQuote(),
		SellQuote: emptyArmsSellQuote(),
	}
	if d.mode == ArmsShopBuy {
		intent.BuyQuote = d.activeMode().buyRows[d.GetSelectedIndex()]
	} else {
		intent.SellQuote = d.activeMode().sellRows[d.GetSelectedIndex()]
	}
	d.pendingIntent = &intent
	d.awaitingIntent = intent
	d.transitionTo(ArmsShopAwaitingTrade)
	d.syncPresentation()
}

func (d *ArmsShopDialog) applyUnavailableBuyRefusal(quote game_state.ArmsBuyQuote) {
	if quote.QuantityBefore >= party_state.ItemQuantitySmallMax {
		d.latestExchange = d.prose.CapacityRefusal
		d.syncPresentation()

		return
	}

	d.latestExchange = d.prose.FundsRefusal
	d.transitionTo(ArmsShopTerminalRefusal)
	d.syncPresentation()
}

func (d *ArmsShopDialog) applyTransactionRefresh(
	refreshed game_state.ArmsShopState,
	display ArmsTradeResultDisplay,
	selected references.Equipment,
) {
	d.applyStateRefresh(refreshed, display, selected, ArmsShopBrowse)
}

func (d *ArmsShopDialog) applyStateRefresh(
	refreshed game_state.ArmsShopState,
	display ArmsTradeResultDisplay,
	selected references.Equipment,
	nextPhase ArmsShopDialogPhase,
) {
	validateArmsResultDisplay(display)
	refreshed = validatedArmsShopState(refreshed)
	indices := [2]int{d.modes[ArmsShopBuy].list.GetCurrentIndex(), d.modes[ArmsShopSell].list.GetCurrentIndex()}
	selectedEquipment := [2]references.Equipment{
		d.modeSelectedEquipment(ArmsShopBuy),
		d.modeSelectedEquipment(ArmsShopSell),
	}
	selectedEquipment[d.mode] = selected
	d.state = refreshed
	d.rebuildModeRows(ArmsShopBuy, selectedEquipment[ArmsShopBuy], indices[ArmsShopBuy])
	d.rebuildModeRows(ArmsShopSell, selectedEquipment[ArmsShopSell], indices[ArmsShopSell])
	d.calculateLayout()
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.clearAwaiting()
	d.transitionTo(nextPhase)
	d.syncPresentation()
}

func (d *ArmsShopDialog) modeSelectedEquipment(mode ArmsShopMode) references.Equipment {
	modeState := &d.modes[mode]
	if mode == ArmsShopBuy && len(modeState.buyRows) > 0 {
		return modeState.buyRows[modeState.list.GetCurrentIndex()].Equipment
	}
	if mode == ArmsShopSell && len(modeState.sellRows) > 0 {
		return modeState.sellRows[modeState.list.GetCurrentIndex()].Equipment
	}

	return references.NoEquipment
}

func (d *ArmsShopDialog) clearReceipt() {
	if d.receiptText != "" {
		d.receiptText = ""
	}
}

func (d *ArmsShopDialog) clearAwaiting() {
	d.pendingIntent = nil
	d.awaitingIntent = emptyArmsTradeIntent()
	d.awaitingRequest = emptyArmsTradeRequest()
}

func emptyArmsTradeIntent() ArmsTradeIntent {
	return ArmsTradeIntent{
		Mode:      ArmsShopBuy,
		BuyQuote:  emptyArmsBuyQuote(),
		SellQuote: emptyArmsSellQuote(),
	}
}

func emptyArmsTradeRequest() ArmsTradeRequest {
	return ArmsTradeRequest{
		Mode: ArmsShopBuy,
		Buy: game_state.ArmsBuyRequest{
			Equipment: references.Equipment(0),
			Quantity:  0,
		},
		Sell: game_state.ArmsSellRequest{
			Equipment: references.Equipment(0),
			Quantity:  0,
		},
	}
}

func emptyArmsBuyQuote() game_state.ArmsBuyQuote {
	return game_state.ArmsBuyQuote{
		Equipment:       references.Equipment(0),
		BasePrice:       0,
		UnitPrice:       0,
		Gold:            0,
		QuantityBefore:  0,
		ReadiedQuantity: 0,
		MaxQuantity:     0,
		AmmoRefill:      false,
	}
}

func emptyArmsSellQuote() game_state.ArmsSellQuote {
	return game_state.ArmsSellQuote{
		Equipment:       references.Equipment(0),
		BasePrice:       0,
		UnitOffer:       0,
		Gold:            0,
		QuantityBefore:  0,
		ReadiedQuantity: 0,
		MaxQuantity:     0,
		Sellable:        false,
	}
}

func (d *ArmsShopDialog) transitionTo(phase ArmsShopDialogPhase) {
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
	d.directionsReady = !d.isKeyPressed(ebiten.KeyUp) && !d.isKeyPressed(ebiten.KeyDown)
}

func (d *ArmsShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *ArmsShopDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return
	}
	if d.rowOutput != nil && width == d.lastScreenWidth && height == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth, d.lastScreenHeight = width, height
	d.shell.GetBounds()
	_, d.headerOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: armsShopLineSpacing,
		MaxLines: 1, MaxChars: armsShopMinimumTextChars,
	})
	_, d.sortOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: armsShopLineSpacing,
		MaxLines: 1, MaxChars: armsShopMinimumTextChars,
	})
	d.rowFont, d.rowOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: armsShopLineSpacing,
		MaxLines: armsShopDetailLines, MaxChars: armsShopMinimumTextChars,
	})
	_, d.detailOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: armsShopLineSpacing,
		MaxLines: armsShopDetailLines, MaxChars: armsShopMinimumTextChars,
	})
	d.calculateLayout()
	d.populateWorkspaceOutput()
}

//nolint:funlen // One geometry pass budgets header, complete item rows, sort strip and remaining detail space together.
func (d *ArmsShopDialog) calculateLayout() {
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(armsShopMinimumInset, min(workspace.Dx(), workspace.Dy())/armsShopInsetDivisor)
	d.workspaceContent = insetRectangle(workspace, inset)
	d.configureTableColumns()
	fontHeight := max(1, int(math.Ceil(d.rowOutput.GetFontHeight())))
	lineHeight := fontHeight + inset
	modeHeight := lineHeight
	minimumDetailHeight := min(d.workspaceContent.Dy(), max(
		int(math.Ceil(d.detailOutput.GetTotalHeightForLines(armsShopDetailLines)))+inset*2,
		lineHeight*armsShopDetailLines,
	))
	d.modeHeader = image.Rect(d.workspaceContent.Min.X, d.workspaceContent.Min.Y,
		d.workspaceContent.Max.X, min(d.workspaceContent.Max.Y, d.workspaceContent.Min.Y+modeHeight))
	gap := inset
	sortHeight := lineHeight
	visible := min(d.rowCount(), armsShopMaximumVisibleRows)
	minimumVisible := min(1, visible)
	tableBudget := max(1,
		d.workspaceContent.Max.Y-minimumDetailHeight-gap-sortHeight-d.modeHeader.Max.Y)
	bottomInset := 0
	if visible > 0 {
		bottomInset = min(
			d.pixelUnit()*armsShopBottomInsetUnits,
			max(0, tableBudget-(minimumVisible+1)),
		)
	}
	rowSpace := max(1, tableBudget-bottomInset)
	rowHeight := max(lineHeight, d.measuredItemRowHeight()+inset)
	visible = min(visible, max(1, (rowSpace-lineHeight)/rowHeight))
	// Below supported sizes retain bounded geometry; supported sizes preserve
	// the full measured row height and scroll whole item rows.
	rowHeight = min(rowHeight, max(1, (rowSpace-lineHeight)/max(1, visible)))
	d.tableHeader = image.Rect(d.workspaceContent.Min.X, d.modeHeader.Max.Y,
		d.workspaceContent.Max.X, d.modeHeader.Max.Y+min(lineHeight, rowSpace))
	d.rowBounds = make([]image.Rectangle, visible)
	for row := range visible {
		top := d.tableHeader.Max.Y + row*rowHeight
		d.rowBounds[row] = image.Rect(d.workspaceContent.Min.X, top, d.workspaceContent.Max.X,
			top+rowHeight)
	}
	tableMaxY := d.tableHeader.Max.Y
	if len(d.rowBounds) > 0 {
		tableMaxY = d.rowBounds[len(d.rowBounds)-1].Max.Y + bottomInset
	}
	d.tableBounds = image.Rect(d.workspaceContent.Min.X, d.tableHeader.Min.Y, d.workspaceContent.Max.X, tableMaxY)
	d.sortBounds = image.Rect(
		d.tableBounds.Min.X,
		d.tableBounds.Max.Y,
		d.tableBounds.Max.X,
		d.tableBounds.Max.Y+sortHeight,
	)
	d.detailBounds = image.Rect(
		d.workspaceContent.Min.X,
		d.sortBounds.Max.Y+gap,
		d.workspaceContent.Max.X,
		d.workspaceContent.Max.Y,
	)
	d.detailTextBounds = insetRectangle(d.detailBounds, inset)
	d.detailTextBounds.Min.X = min(
		d.detailTextBounds.Max.X-1,
		d.detailTextBounds.Min.X+d.pixelUnit()*armsShopIconInsetUnits,
	)
	d.configureTableLayout(inset)
}

func (d *ArmsShopDialog) measuredItemRowHeight() int {
	nameLines := 1
	for index := range d.rowCount() {
		row := d.rowView(index, false)
		d.prepareCellOutput(" "+row.Name, DialogItemNameColor, row.Equipment)
		nameLines = max(nameLines, d.rowOutput.GetPanelContentLineCount())
	}

	return int(math.Ceil(d.rowOutput.GetTotalHeightForLines(nameLines)))
}

func (d *ArmsShopDialog) configureTableLayout(inset int) {
	for index := range armsShopColumnCount {
		d.columns[index].Min.Y = d.tableBounds.Min.Y
		d.columns[index].Max.Y = d.tableBounds.Max.Y
	}
	for mode := ArmsShopBuy; mode <= ArmsShopSell; mode++ {
		selected := d.modeSelectedEquipment(mode)
		fallback := d.modes[mode].list.GetCurrentIndex()
		d.rebuildModeRows(mode, selected, fallback)
	}
	d.headerOutput.SetMaxCharsPerLine(max(armsShopMinimumTextChars,
		d.headerOutput.Font.CharsForWidth(max(1, d.modeHeader.Dx()-inset*2))))
	d.sortOutput.SetMaxCharsPerLine(max(armsShopMinimumTextChars,
		d.sortOutput.Font.CharsForWidth(max(1, d.sortBounds.Dx()-inset*2))))
	d.detailOutput.SetMaxCharsPerLine(max(armsShopMinimumTextChars,
		d.detailOutput.Font.CharsForWidth(max(1, d.detailTextBounds.Dx()))))
}

// Numeric columns reserve measured headings and worst-case values. The item
// column gets all remaining space; long identities wrap without reducing type.
func (d *ArmsShopDialog) configureTableColumns() {
	labels := [...]string{"", "CARRIED", "READY", "NO OFFER"}
	right := d.workspaceContent.Max.X
	inset := max(1, d.pixelUnit()*armsShopCellInsetUnits)
	for index := armsShopColumnCount - 1; index > 0; index-- {
		width, _ := d.rowFont.MeasureString(labels[index])
		columnWidth := int(math.Ceil(width)) + inset*armsShopCenterDivisor
		left := max(d.workspaceContent.Min.X, right-columnWidth)
		d.columns[index] = image.Rect(left, d.workspaceContent.Min.Y, right, d.workspaceContent.Max.Y)
		right = left
	}
	d.columns[0] = image.Rect(d.workspaceContent.Min.X, d.workspaceContent.Min.Y,
		max(d.workspaceContent.Min.X+1, right), d.workspaceContent.Max.Y)
}

func (d *ArmsShopDialog) drawWorkspace(screen *ebiten.Image) {
	DrawClassicPixelRoundedFrame(screen, d.tableBounds, d.pixelUnit(), DialogRuleLightColor)
	DrawClassicPixelRoundedFrame(screen, d.detailBounds, d.pixelUnit(), DialogRuleLightColor)
	d.headerOutput.DrawPanelContent(screen,
		image.Pt(d.modeHeader.Min.X, verticallyCenteredOutputY(d.modeHeader, d.headerOutput)),
		d.modeHeader.Dx(), etext.AlignCenter)
	d.sortOutput.DrawPanelContent(screen,
		image.Pt(d.sortBounds.Min.X, verticallyCenteredOutputY(d.sortBounds, d.sortOutput)),
		d.sortBounds.Dx()-max(1, d.pixelUnit()*armsShopCellInsetUnits), etext.AlignEnd)
	headings := [armsShopColumnCount]string{"ITEM", "CARRIED", "READY", "PRICE"}
	if d.mode == ArmsShopSell {
		headings[3] = "OFFER"
	}
	headerContent := d.tableHeaderContentBounds()
	for index, heading := range headings {
		d.drawCell(
			screen, headerContent, d.columns[index], heading,
			DialogLabelColor, etext.AlignCenter, references.NoEquipment,
		)
	}
	start, end := d.activeMode().list.GetVisibleRange()
	for index := start; index < end; index++ {
		screenRow := index - start
		bounds := d.rowBounds[screenRow]
		selected := index == d.GetSelectedIndex()
		if selected {
			fillSelectedRow(screen, bounds)
		}
		view := d.rowView(index, selected)
		textColor := DialogItemNameColor
		if !view.Available {
			textColor = DialogEmptyColor
		}
		if selected && view.Available {
			textColor = DialogSelectedText
		}
		d.drawCell(screen, bounds, d.columns[0], " "+view.Name, textColor, etext.AlignStart, view.Equipment)
		d.drawCell(
			screen, bounds, d.columns[1], strconv.FormatUint(uint64(view.Quantity), 10),
			textColor, etext.AlignCenter, references.NoEquipment,
		)
		d.drawCell(
			screen, bounds, d.columns[2], strconv.FormatUint(uint64(view.Readied), 10),
			textColor, etext.AlignCenter, references.NoEquipment,
		)
		d.drawCell(screen, bounds, d.columns[3], view.ValueText, textColor, etext.AlignCenter, references.NoEquipment)
	}
	d.detailOutput.DrawPanelContent(screen,
		image.Pt(d.detailTextBounds.Min.X, verticallyCenteredOutputY(d.detailTextBounds, d.detailOutput)),
		max(1, d.detailTextBounds.Dx()), etext.AlignStart)
}

func (d *ArmsShopDialog) drawCell(
	screen *ebiten.Image,
	row, column image.Rectangle,
	value string,
	color stdcolor.Color,
	align etext.Align,
	equipment references.Equipment,
) {
	cell := d.cellContentBounds(row, column, equipment)
	d.prepareCellOutput(value, color, equipment)
	height := int(math.Ceil(d.rowOutput.GetTotalHeightForLines(d.rowOutput.GetPanelContentLineCount())))
	y := cell.Min.Y + max(
		0,
		(cell.Dy()-height)/armsShopCenterDivisor,
	)
	d.rowOutput.DrawPanelContent(screen, image.Pt(cell.Min.X, y), max(1, cell.Dx()), align)
}

func (d *ArmsShopDialog) prepareCellOutput(value string, color stdcolor.Color, equipment references.Equipment) {
	d.rowOutput.SetMaxCharsPerLine(max(1, len(value)+1))
	d.rowOutput.Reset()
	line := d.rowOutput.AppendLine()
	if equipment != references.NoEquipment {
		iconColor := color
		if color == DialogItemNameColor {
			iconColor = pkgcolor.EGAYellow
		}
		line.EquipmentIconWithColor(equipment, iconColor)
		glyphWidth, _ := d.rowFont.MeasureString("M")
		glyphWidth = max(glyphWidth, d.rowOutput.MeasureLineSegments(line.GetSegments()))
		cell := d.cellContentBounds(d.workspaceContent, d.columns[0], equipment)
		d.rowOutput.SetMaxCharsPerLine(max(1, int(float64(cell.Dx())/glyphWidth)))
	}
	line.Text(value, color).WordWrap().Done()
}

func (d *ArmsShopDialog) cellContentBounds(
	row, column image.Rectangle,
	equipment references.Equipment,
) image.Rectangle {
	inset := max(1, d.pixelUnit()*armsShopCellInsetUnits)
	cell := image.Rect(column.Min.X+inset, row.Min.Y, max(column.Min.X+inset+1, column.Max.X-inset), row.Max.Y)
	if equipment != references.NoEquipment {
		cell.Min.X = min(cell.Max.X-1, cell.Min.X+d.pixelUnit()*armsShopIconInsetUnits)
	}

	return cell
}

func (d *ArmsShopDialog) tableHeaderContentBounds() image.Rectangle {
	bounds := d.tableHeader
	bounds.Min.Y = min(bounds.Max.Y-1, bounds.Min.Y+max(1, d.pixelUnit()))

	return bounds
}

func (d *ArmsShopDialog) populateWorkspaceOutput() {
	if d.headerOutput == nil || d.sortOutput == nil || d.detailOutput == nil {
		return
	}
	d.headerOutput.Reset()
	header := d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter)
	if d.preview {
		header.Text("PREVIEW   ", DialogStatusWarningColor)
	}
	buyColor, sellColor := d.modeTabColors()
	header.Text("[B] BUY", buyColor).Text("      ", DialogEmptyColor).Text("[S] SELL", sellColor).Done()
	d.sortOutput.Reset()
	d.sortOutput.AppendLine().WithAlignment(etext.AlignEnd).
		Text("[R]", DialogControlKeyColor).Text(" SORT: ", DialogControlHintColor).
		Text(d.GetSort().String(), DialogValueColor).Done()
	d.detailOutput.Reset()
	if d.receiptText != "" {
		d.detailOutput.AppendLine().Text(d.receiptText, DialogValueColor).WordWrap().Done()

		return
	}
	equipment := d.selectedEquipment()
	if equipment == references.NoEquipment {
		d.detailOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text("No equipment carried.", DialogEmptyColor).Done()

		return
	}
	d.detailOutput.AppendLine().
		Yellow(equipment.String()).LightGray("  " + equipment.GetCategory().String()).WordWrap().Done()
	d.detailOutput.AppendLine().Whitef(
		"ATK %d  DEF %d  RNG %d",
		equipment.GetAttackValue(), equipment.GetDefenseValue(), equipment.GetAttackRange(),
	).Done()
	view := d.rowView(d.GetSelectedIndex(), false)
	if view.Reason != "" {
		d.populateUnavailableDetail()

		return
	}
	if d.mode == ArmsShopBuy {
		quote := d.activeMode().buyRows[d.GetSelectedIndex()]
		d.detailOutput.AppendLine().LightGray("Carried ").Cyanf("%d", quote.QuantityBefore).
			LightGray("  Ready ").LightGreenf("%d", quote.ReadiedQuantity).
			LightGray("  Price ").Yellowf("%d", quote.UnitPrice).Done()
	} else {
		quote := d.activeMode().sellRows[d.GetSelectedIndex()]
		offer := d.rowView(d.GetSelectedIndex(), false).ValueText
		d.detailOutput.AppendLine().LightGray("Carried ").Cyanf("%d", quote.QuantityBefore).
			LightGray("  Ready ").LightGreenf("%d", quote.ReadiedQuantity).
			LightGray("  Offer ").Yellow(offer).Done()
	}
}

func (d *ArmsShopDialog) populateUnavailableDetail() {
	if d.mode == ArmsShopBuy {
		quote := d.activeMode().buyRows[d.GetSelectedIndex()]
		if quote.QuantityBefore >= party_state.ItemQuantitySmallMax {
			d.detailOutput.AppendLine().Text("Cannot buy: carried quantity", DialogStatusWarningColor).Done()
			d.detailOutput.AppendLine().Text("is already 99.", DialogStatusWarningColor).Done()

			return
		}
		d.detailOutput.AppendLine().
			Textf("Cannot buy: need %d gold;", DialogStatusWarningColor, quote.UnitPrice).Done()
		d.detailOutput.AppendLine().
			Textf("party has %d.", DialogStatusWarningColor, quote.Gold).Done()

		return
	}

	quote := d.activeMode().sellRows[d.GetSelectedIndex()]
	switch {
	case quote.Equipment.IsAmmo():
		d.detailOutput.AppendLine().Text("Cannot sell: used ammunition", DialogStatusWarningColor).Done()
		d.detailOutput.AppendLine().Text("is refused.", DialogStatusWarningColor).Done()
	case quote.BasePrice == 0 || quote.UnitOffer == 0 || !quote.Sellable:
		d.detailOutput.AppendLine().Text("Cannot sell: this equipment", DialogStatusWarningColor).Done()
		d.detailOutput.AppendLine().Text("has no offer.", DialogStatusWarningColor).Done()
	case quote.MaxQuantity == 0:
		d.detailOutput.AppendLine().Text("Cannot sell: party gold", DialogStatusWarningColor).Done()
		d.detailOutput.AppendLine().Text("has no room.", DialogStatusWarningColor).Done()
	default:
		panic("ArmsShopDialog.populateUnavailableDetail: selected row is available")
	}
}

func (d *ArmsShopDialog) modeTabColors() (stdcolor.Color, stdcolor.Color) {
	if d.mode == ArmsShopBuy {
		return DialogLabelColor, DialogEmptyColor
	}

	return DialogEmptyColor, DialogLabelColor
}

func (d *ArmsShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateWorkspaceOutput()
}

func (d *ArmsShopDialog) footerGuidance() string {
	switch d.phase {
	case ArmsShopBrowse:
		return "[B] Buy  [S] Sell  [R] Sort  [Shift+Up/Down/PgUp/PgDn] Page\n[Up/Down] Select  [Enter] Trade  [Esc] Leave"
	case ArmsShopAwaitingTrade:
		return "Complete or cancel the trade dialog"
	case ArmsShopAwaitingResult:
		return "Processing trade"
	case ArmsShopTerminalRefusal, ArmsShopSessionError:
		return merchantLeaveFooter
	default:
		panic("ArmsShopDialog.footerGuidance: unknown phase")
	}
}

func (d *ArmsShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func sortArmsBuyRows(rows []game_state.ArmsBuyQuote, order ArmsShopSort) {
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		switch order {
		case ArmsSortQuantity:
			if left.QuantityBefore != right.QuantityBefore {
				return left.QuantityBefore > right.QuantityBefore
			}
		case ArmsSortValue:
			if left.UnitPrice != right.UnitPrice {
				return left.UnitPrice > right.UnitPrice
			}
		case ArmsSortName:
		default:
			panic("sortArmsBuyRows: unknown sort")
		}

		return equipmentNameLess(left.Equipment, right.Equipment)
	})
}

func sortArmsSellRows(rows []game_state.ArmsSellQuote, order ArmsShopSort) {
	sort.SliceStable(rows, func(i, j int) bool {
		left, right := rows[i], rows[j]
		switch order {
		case ArmsSortQuantity:
			if left.QuantityBefore != right.QuantityBefore {
				return left.QuantityBefore > right.QuantityBefore
			}
		case ArmsSortValue:
			if left.UnitOffer != right.UnitOffer {
				return left.UnitOffer > right.UnitOffer
			}
		case ArmsSortName:
		default:
			panic("sortArmsSellRows: unknown sort")
		}

		return equipmentNameLess(left.Equipment, right.Equipment)
	})
}

func equipmentNameLess(left, right references.Equipment) bool {
	leftName, rightName := strings.ToLower(left.String()), strings.ToLower(right.String())
	if leftName != rightName {
		return leftName < rightName
	}

	return left.ID() < right.ID()
}

func validatedArmsShopState(state game_state.ArmsShopState) game_state.ArmsShopState {
	if !validArmsShopIdentity(state) {
		panic("ArmsShopDialog: complete Arms state is required")
	}
	validateUniqueArmsBuyQuotes(state)
	validateUniqueArmsSellQuotes(state)
	state.SellQuotes = append([]game_state.ArmsSellQuote(nil), state.SellQuotes...)

	return state
}

func validArmsShopIdentity(state game_state.ArmsShopState) bool {
	return state.Merchant.ID != "" &&
		state.Merchant.Family == references.MerchantFamilyArms &&
		strings.TrimSpace(state.Merchant.OwnerName) != "" &&
		strings.TrimSpace(state.Merchant.ShopName) != "" &&
		state.PricingCharacterIndex >= 0 &&
		strings.TrimSpace(state.PricingCharacterName) != ""
}

func validateUniqueArmsBuyQuotes(state game_state.ArmsShopState) {
	seenBuy := make(map[references.Equipment]bool, len(state.BuyQuotes))
	for _, quote := range state.BuyQuotes {
		validateArmsBuyQuote(quote, state.Gold)
		if seenBuy[quote.Equipment] {
			panic("ArmsShopDialog: duplicate buy quote")
		}
		seenBuy[quote.Equipment] = true
	}
}

func validateUniqueArmsSellQuotes(state game_state.ArmsShopState) {
	seenSell := make(map[references.Equipment]bool, len(state.SellQuotes))
	for _, quote := range state.SellQuotes {
		validateArmsSellQuote(quote, state.Gold)
		if seenSell[quote.Equipment] {
			panic("ArmsShopDialog: duplicate sell quote")
		}
		seenSell[quote.Equipment] = true
	}
}

func validateArmsBuyQuote(quote game_state.ArmsBuyQuote, gold uint16) {
	if !validArmsEquipment(quote.Equipment) || !validArmsBuyPriceAndGold(quote, gold) ||
		!validArmsBuyQuantity(quote) {
		panic("ArmsShopDialog: invalid buy quote")
	}
}

func validArmsEquipment(equipment references.Equipment) bool {
	return equipment >= references.LeatherHelm && equipment <= references.Ankh
}

func validArmsBuyPriceAndGold(quote game_state.ArmsBuyQuote, gold uint16) bool {
	return quote.BasePrice > 0 && quote.UnitPrice > 0 && quote.Gold == gold
}

func validArmsBuyQuantity(quote game_state.ArmsBuyQuote) bool {
	return quote.QuantityBefore <= party_state.ItemQuantitySmallMax &&
		quote.MaxQuantity <= party_state.ItemQuantitySmallMax &&
		quote.AmmoRefill == quote.Equipment.IsAmmo() &&
		(!quote.AmmoRefill || quote.MaxQuantity <= 1)
}

func validateArmsSellQuote(quote game_state.ArmsSellQuote, gold uint16) {
	if !validArmsEquipment(quote.Equipment) || !validArmsSellQuantityAndGold(quote, gold) {
		panic("ArmsShopDialog: invalid sell quote")
	}
	unsellable := quote.Equipment.IsAmmo() || quote.BasePrice == 0
	if unsellable && !validRefusedArmsSellQuote(quote) {
		panic("ArmsShopDialog: invalid refused sell quote")
	}
	if !unsellable && !validOfferedArmsSellQuote(quote) {
		panic("ArmsShopDialog: invalid sellable quote")
	}
}

func validArmsSellQuantityAndGold(quote game_state.ArmsSellQuote, gold uint16) bool {
	return quote.Gold == gold && quote.QuantityBefore > 0 &&
		quote.QuantityBefore <= party_state.ItemQuantitySmallMax &&
		quote.MaxQuantity <= quote.QuantityBefore
}

func validRefusedArmsSellQuote(quote game_state.ArmsSellQuote) bool {
	return !quote.Sellable && quote.UnitOffer == 0 && quote.MaxQuantity == 0
}

func validOfferedArmsSellQuote(quote game_state.ArmsSellQuote) bool {
	return quote.Sellable && quote.UnitOffer > 0
}

func validateArmsRequestForIntent(intent ArmsTradeIntent, request ArmsTradeRequest) {
	if request.Mode != intent.Mode || request.Quantity() == 0 || request.Equipment() != intent.Equipment() {
		panic("ArmsShopDialog: request does not match trade intent")
	}
	switch request.Mode {
	case ArmsShopBuy:
		validateArmsBuyRequestQuantity(intent.BuyQuote, request.Buy.Quantity)
	case ArmsShopSell:
		if request.Sell.Quantity > intent.SellQuote.MaxQuantity {
			panic("ArmsShopDialog: invalid sell request quantity")
		}
	default:
		panic("ArmsShopDialog: known request mode is required")
	}
}

func validateArmsBuyRequestQuantity(quote game_state.ArmsBuyQuote, quantity uint16) {
	if quote.AmmoRefill && quantity != 1 ||
		quote.MaxQuantity > 0 && quantity > quote.MaxQuantity ||
		quote.MaxQuantity == 0 && quantity != 1 {
		panic("ArmsShopDialog: invalid buy request quantity")
	}
}

func validateArmsShopProse(prose ArmsShopDialogProse) {
	validateArmsFinalizedText("greeting", prose.Greeting)
	validateArmsFinalizedText("buy prompt", prose.BuyPrompt)
	validateArmsFinalizedText("sell prompt", prose.SellPrompt)
	validateArmsFinalizedText("capacity refusal", prose.CapacityRefusal)
	validateArmsFinalizedText("funds refusal", prose.FundsRefusal)
}

func validateArmsResultDisplay(display ArmsTradeResultDisplay) {
	validateArmsFinalizedText("merchant exchange", display.MerchantExchange)
	validateArmsFinalizedText("receipt", display.Receipt)
}

func validateArmsFinalizedText(label, value string) {
	if strings.TrimSpace(value) == "" {
		panic("Arms widgets: finalized " + label + " is required")
	}
}
