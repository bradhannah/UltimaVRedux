package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strings"
	"unicode"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	tavernShopModeCount          = 2
	tavernTopicRuneLimit         = 15
	tavernWineCount              = 6
	tavernMinimumProvisionPacks  = 1
	tavernMaximumProvisionPacks  = 99
	tavernWarningDrunkLevel      = 3
	tavernShopMinimumInset       = 4
	tavernShopInsetDivisor       = 64
	tavernShopLineSpacing        = 1.06
	tavernShopMinimumTextChars   = 8
	tavernShopMaximumBodyLines   = 64
	tavernShopModeHeaderMaxLines = 2
	tavernConversationBaseLines  = 3
	tavernTranscriptMinimumLines = 2
	tavernActionUnknownLabel     = "Unknown"
)

var _ Widget = (*TavernShopDialog)(nil)

// TavernShopMode identifies the persistent Tavern workspace.
type TavernShopMode byte

const (
	// TavernShopOrder shows the location's canonical food and drink choices.
	TavernShopOrder TavernShopMode = iota
	// TavernShopConversation accepts visible, authority-matched information subjects.
	TavernShopConversation
)

// TavernOrderSubflow identifies the active keyboard-only Order interaction.
type TavernOrderSubflow byte

const (
	// TavernOrderMenu shows the canonical location menu.
	TavernOrderMenu TavernOrderSubflow = iota
	// TavernOrderWinePicker shows the six canonical wines in source order.
	TavernOrderWinePicker
)

// TavernShopDialogPhase identifies workflow ownership of the parent.
type TavernShopDialogPhase byte

const (
	// TavernShopBrowse accepts parent mode, list, quantity, and topic input.
	TavernShopBrowse TavernShopDialogPhase = iota
	// TavernShopAwaitingTopicQuote blocks while workflow asks authority to match raw input.
	TavernShopAwaitingTopicQuote
	// TavernShopAwaitingApproval blocks while TavernActionDialog owns input.
	TavernShopAwaitingApproval
	// TavernShopAwaitingResult blocks while workflow invokes Tavern authority.
	TavernShopAwaitingResult
	// TavernShopSessionError displays a terminal workflow error until acknowledgement.
	TavernShopSessionError
)

// TavernOrderItem identifies one canonical parent Order row.
type TavernOrderItem byte

const (
	// TavernOrderMeal is the location's one canonical meal row.
	TavernOrderMeal TavernOrderItem = iota
	// TavernOrderDrink is the ordinary drink or wine-list row.
	TavernOrderDrink
	// TavernOrderProvision is present only where canonical provisions are available.
	TavernOrderProvision
)

// TavernAction identifies one exact request branch crossing into workflow authority.
type TavernAction byte

const (
	// TavernActionUnknown is the empty typed-union discriminator.
	TavernActionUnknown TavernAction = iota
	// TavernActionMeal carries TavernMealRequest.
	TavernActionMeal
	// TavernActionDrink carries TavernDrinkRequest.
	TavernActionDrink
	// TavernActionStop carries the warned TavernStopRequest alternative.
	TavernActionStop
	// TavernActionProvision carries TavernProvisionRequest.
	TavernActionProvision
	// TavernActionInformation carries TavernInformationRequest.
	TavernActionInformation
)

func (a TavernAction) String() string {
	switch a {
	case TavernActionMeal:
		return "Meal"
	case TavernActionDrink:
		return "Drink"
	case TavernActionStop:
		return "Stop"
	case TavernActionProvision:
		return "Provisions"
	case TavernActionInformation:
		return "Information"
	case TavernActionUnknown:
		return tavernActionUnknownLabel
	default:
		panic("TavernAction.String: unknown action")
	}
}

// TavernOrderRow is one detached row in the location-specific Order menu.
type TavernOrderRow struct {
	Item TavernOrderItem
	Name string
	Key  byte
}

// TavernTopicIntent carries exactly the raw input that workflow must quote.
type TavernTopicIntent struct {
	Input string
}

// TavernActionRequest is the typed comparison-request union emitted by approval.
type TavernActionRequest struct {
	Action      TavernAction
	Meal        game_state.TavernMealRequest
	Drink       game_state.TavernDrinkRequest
	Stop        game_state.TavernStopRequest
	Provision   game_state.TavernProvisionRequest
	Information game_state.TavernInformationRequest
}

// TavernActionIntent contains detached approval data and exact request alternatives.
// StopRequest is populated only for a warned drink; Request then contains Continue.
type TavernActionIntent struct {
	Action      TavernAction
	Request     TavernActionRequest
	StopRequest TavernActionRequest
	Meal        game_state.TavernMealQuote
	Drink       game_state.TavernDrinkQuote
	Provision   game_state.TavernProvisionQuote
	Information game_state.TavernInformationQuote
	// ProvisionPitch is source-backed merchant prose shown above quantity approval.
	ProvisionPitch string
	// DrinkWarning is finalized barkeeper prose shown before one more drink.
	DrinkWarning string
	// InformationPitch is source-backed merchant prose shown before approval.
	InformationPitch string
	PackCount        int
	Preview          bool
}

// TavernTranscriptEntry is one successful paid question and workflow-supplied answer.
type TavernTranscriptEntry struct {
	Question string
	Answer   string
}

// TavernActionResultDisplay contains finalized workflow presentation after authority.
type TavernActionResultDisplay struct {
	Action                TavernAction
	MerchantExchange      string
	Receipt               string
	PaidInformation       *TavernTranscriptEntry
	SuppressInlineReceipt bool
	Dismiss               bool
}

// TavernShopDialogProse contains finalized merchant copy supplied by workflow.
type TavernShopDialogProse struct {
	Greeting           string
	OrderPrompt        string
	ConversationPrompt string
	ConversationExit   string
	UnknownTopic       string
}

// TavernShopDialogConfig configures one detached, keyboard-only Tavern session.
type TavernShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	State          game_state.TavernShopState
	Prose          TavernShopDialogProse
	Preview        bool
	IsKeyPressed   func(ebiten.Key) bool
	// IsWindowFocused defaults to the native window; detached scene fixtures
	// supply their input owner's focus source.
	IsWindowFocused func() bool

	// AppendInputChars selects the detached rune-input test adapter. Nil uses
	// a native field, compatible with the save description editor's session.
	AppendInputChars func([]rune) []rune
}

type tavernApprovalReturn struct {
	mode          TavernShopMode
	subflow       TavernOrderSubflow
	topicInput    string
	selectedOrder int
	selectedWine  int
}

// TavernShopDialog presents persistent Order and Conversation modes without gameplay mutation.
type TavernShopDialog struct {
	navigation
	keyboard         *input.Keyboard
	isKeyPressed     func(ebiten.Key) bool
	appendInputChars func([]rune) []rune
	state            game_state.TavernShopState
	prose            TavernShopDialogProse
	preview          bool

	mode                 TavernShopMode
	subflow              TavernOrderSubflow
	phase                TavernShopDialogPhase
	selectedOrder        int
	selectedWine         int
	topicInput           []rune // Used only by the explicitly supplied rune adapter.
	topicField           nativeTextField
	modalFocused         bool
	isWindowFocused      func() bool
	selectionReversed    bool
	topicControlBlocked  [ebiten.KeyMax + 1]bool
	topicInputSuspended  bool
	topicBounds          image.Rectangle
	topicCaretBounds     image.Rectangle
	topicSelectionBounds image.Rectangle
	rowSelectionBounds   image.Rectangle
	inputKeysReady       bool
	visible              bool
	dismissed            bool

	latestExchange  string
	receiptText     string
	inlineFeedback  string
	pendingTopic    *TavernTopicIntent
	quotedTopic     TavernTopicIntent
	pendingIntent   *TavernActionIntent
	awaitingIntent  TavernActionIntent
	awaitingRequest TavernActionRequest
	approvalReturn  tavernApprovalReturn

	transcript       []TavernTranscriptEntry
	transcriptScroll int

	shell      *MerchantDialogShell
	modeOutput *text.Output
	bodyOutput *text.Output
	bodyFont   *text.UltimaFont

	workspaceContent image.Rectangle
	modeBounds       image.Rectangle
	bodyBounds       image.Rectangle
	bodyText         image.Rectangle

	lastScreenWidth  int
	lastScreenHeight int
}

// NewTavernShopDialog creates a visible Tavern dialog from one complete authoritative snapshot.
func NewTavernShopDialog(config TavernShopDialogConfig) *TavernShopDialog {
	var field nativeTextField
	if config.AppendInputChars == nil {
		field = newNativeTextField()
	}

	return newTavernShopDialog(config, field)
}

func newTavernShopDialog(config TavernShopDialogConfig, field nativeTextField) *TavernShopDialog {
	if config.Keyboard == nil {
		panic("TavernShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("TavernShopDialog: merchant sprite is required")
	}
	validateTavernShopProse(config.Prose)
	state := validatedTavernShopState(config.State)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = ebiten.IsKeyPressed
	}
	isWindowFocused := config.IsWindowFocused
	if isWindowFocused == nil {
		isWindowFocused = ebiten.IsFocused
	}
	if config.AppendInputChars == nil && field == nil {
		panic("TavernShopDialog: native text field is required")
	}
	if config.AppendInputChars != nil && field != nil {
		panic("TavernShopDialog: text input must have exactly one source")
	}

	dialog := &TavernShopDialog{ //nolint:exhaustruct_v5 // Resolution-dependent presentation fields initialize on demand.
		navigation:       newNavigation(config.Keyboard.IsKeyHeld),
		keyboard:         config.Keyboard,
		isKeyPressed:     isKeyPressed,
		appendInputChars: config.AppendInputChars,
		topicField:       field,
		isWindowFocused:  isWindowFocused,
		state:            state,
		prose:            config.Prose,
		preview:          config.Preview,
		mode:             TavernShopOrder,
		subflow:          TavernOrderMenu,
		phase:            TavernShopBrowse,
		visible:          true,
		latestExchange:   config.Prose.Greeting,
		inputKeysReady:   tavernParentControlsReleased(isKeyPressed),
		quotedTopic:      TavernTopicIntent{Input: ""},
		awaitingIntent:   emptyTavernActionIntent(),
		awaitingRequest:  emptyTavernActionRequest(),
	}
	dialog.shell = NewMerchantDialogShell(MerchantDialogShellConfig{
		Header:         MerchantDialogHeaderView{ShopIdentity: "", Owner: "", PartyGold: ""},
		Exchange:       MerchantDialogExchangeView{Text: ""},
		Footer:         MerchantDialogFooterView{Guidance: ""},
		MerchantSprite: config.MerchantSprite,
	})
	config.Keyboard.PrepareForTransition(0)
	dialog.ensureLayout()
	dialog.syncPresentation()

	return dialog
}

// Draw renders the shared merchant shell and active Tavern workspace.
func (d *TavernShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || !d.IsVisible() {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	d.modeOutput.DrawPanelContent(
		screen,
		image.Pt(d.modeBounds.Min.X, verticallyCenteredOutputY(d.modeBounds, d.modeOutput)),
		d.modeBounds.Dx(),
		etext.AlignCenter,
	)
	d.drawTopicSelection(screen)
	fillSelectedRow(screen, d.rowSelectionBounds)
	d.bodyOutput.DrawPanelContent(screen, d.bodyText.Min, max(1, d.bodyText.Dx()), etext.AlignStart)
	d.drawTopicCaret(screen)
}

// Update processes keyboard-only Tavern input.
func (d *TavernShopDialog) Update() {
	if d.topicField != nil {
		if !d.modalFocused {
			d.syncTopicFocus()

			return
		}
		if !d.isWindowFocused() {
			d.SuspendTextInput()

			return
		}
	}
	if !d.IsVisible() {
		return
	}
	d.topicInputSuspended = false
	d.ensureLayout()
	if !d.inputKeysReady {
		if !tavernParentControlsReleased(d.isKeyPressed) {
			return
		}
		d.inputKeysReady = true
	}
	if d.updateNativeTopic() {
		return
	}
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}

	d.updatePhase()
}

func (d *TavernShopDialog) updatePhase() {
	switch d.phase {
	case TavernShopBrowse:
		d.updateBrowse()
	case TavernShopAwaitingTopicQuote, TavernShopAwaitingApproval, TavernShopAwaitingResult:
		return
	case TavernShopSessionError:
		if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.Dismiss()
		}
	default:
		panic("TavernShopDialog.Update: unknown phase")
	}
}

func (d *TavernShopDialog) updateBrowse() {
	if d.handleModeInput() {
		return
	}
	if d.mode == TavernShopConversation {
		d.updateConversation()

		return
	}
	if d.informationShortcutPressed() {
		d.setMode(TavernShopConversation)

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyO) {
		if d.subflow != TavernOrderMenu {
			d.setOrderSubflow(TavernOrderMenu)
		}

		return
	}

	switch d.subflow {
	case TavernOrderMenu:
		d.updateOrderMenu()
	case TavernOrderWinePicker:
		d.updateWinePicker()
	default:
		panic("TavernShopDialog.updateBrowse: unknown Order subflow")
	}
}

func (d *TavernShopDialog) handleModeInput() bool {
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab):
		d.setMode(TavernShopMode((int(d.mode) + 1) % tavernShopModeCount))
	case (d.topicField == nil || d.mode != TavernShopConversation || !d.state.CustomerEligible) &&
		(d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight)):
		d.setMode(TavernShopMode((int(d.mode) + 1) % tavernShopModeCount))
	default:
		return false
	}

	return true
}

func (d *TavernShopDialog) updateOrderMenu() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()

		return
	}
	if key, owned := d.Poll(d.keyboard); owned {
		if key != nil {
			if delta := listNavigationDelta(*key, 0); delta != 0 {
				d.moveOrderSelection(delta)
			}
		}

		return
	}
	if index, ok := d.directOrderSelection(); ok {
		d.selectedOrder = index
		d.activateOrderSelection()

		return
	}
	if d.enterPressed() {
		d.activateOrderSelection()
	}
}

func (d *TavernShopDialog) updateWinePicker() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.setOrderSubflow(TavernOrderMenu)

		return
	}
	if key, owned := d.Poll(d.keyboard); owned {
		if key != nil {
			d.selectedWine = max(0, min(len(d.state.Wines)-1, d.selectedWine+listNavigationDelta(*key, 0)))
			d.clearTransientPresentation()
			d.populateBodyOutput()
		}

		return
	}
	for index, key := range tavernWineKeys() {
		if d.keyboard.IsKeyPressedAndRegister(key) {
			d.selectedWine = index
			d.beginActionApproval(d.drinkIntent(d.state.Wines[index]))

			return
		}
	}
	if d.enterPressed() {
		d.beginActionApproval(d.drinkIntent(d.state.Wines[d.selectedWine]))
	}
}

func (d *TavernShopDialog) updateConversation() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.setTopicInput("")
		d.setMode(TavernShopOrder)

		return
	}
	if !d.state.CustomerEligible {
		return
	}
	if d.topicField != nil && d.editNativeTopic() {
		return
	}

	if d.topicField == nil {
		d.appendTopicRunes(d.appendInputChars(nil))
	}
	if d.topicField == nil && d.keyboard.IsKeyPressedAndRegister(ebiten.KeyBackspace) {
		if len(d.topicInput) > 0 {
			d.topicInput = d.topicInput[:len(d.topicInput)-1]
			d.inlineFeedback = ""
			d.populateBodyOutput()
		}

		return
	}
	if key, owned := d.Poll(d.keyboard); owned {
		if key != nil {
			if *key == ebiten.KeyPageUp || *key == ebiten.KeyPageDown {
				d.scrollTranscript(listNavigationDelta(*key, 1))
			}
		}

		return
	}
	if d.enterPressed() {
		d.submitTopic()
	}
}

// ConsumePendingTopicIntent returns and clears one raw topic submission.
func (d *TavernShopDialog) ConsumePendingTopicIntent() (TavernTopicIntent, bool) {
	if d.pendingTopic == nil {
		return TavernTopicIntent{Input: ""}, false
	}
	intent := *d.pendingTopic
	d.pendingTopic = nil

	return intent, true
}

// ApplyInformationQuote installs an authority-recognized quote and opens compact approval.
func (d *TavernShopDialog) ApplyInformationQuote(
	topic TavernTopicIntent,
	quote game_state.TavernInformationQuote,
	pitch string,
) {
	if d.phase != TavernShopAwaitingTopicQuote || topic != d.quotedTopic {
		panic("TavernShopDialog.ApplyInformationQuote: topic does not match pending submission")
	}
	validateTavernInformationQuote(quote, d.state)
	intent := newTavernActionIntent(
		TavernActionInformation,
		tavernInformationRequest(d.state, quote),
		d.preview,
	)
	intent.Information = quote
	intent.InformationPitch = strings.TrimSpace(pitch)
	d.pendingTopic = nil
	d.pendingIntent = &intent
	d.awaitingIntent = intent
	d.approvalReturn = d.captureApprovalReturn()
	d.transitionTo(TavernShopAwaitingApproval)
	d.syncPresentation()
}

// RejectInformationTopic returns an unknown authority response and repeats the lore prompt.
func (d *TavernShopDialog) RejectInformationTopic(topic TavernTopicIntent) {
	if d.phase != TavernShopAwaitingTopicQuote || topic != d.quotedTopic {
		panic("TavernShopDialog.RejectInformationTopic: topic does not match pending submission")
	}
	d.pendingTopic = nil
	d.quotedTopic = TavernTopicIntent{Input: ""}
	d.setTopicInput("")
	d.inlineFeedback = ""
	d.latestExchange = d.prose.UnknownTopic + "\n\n" + d.prose.ConversationPrompt
	d.transcriptScroll = d.transcriptTailStart()
	d.transitionTo(TavernShopBrowse)
	d.syncPresentation()
}

// ConsumePendingActionIntent returns and clears one fully detached nested approval intent.
func (d *TavernShopDialog) ConsumePendingActionIntent() (TavernActionIntent, bool) {
	if d.pendingIntent == nil {
		return emptyTavernActionIntent(), false
	}
	intent := cloneTavernActionIntent(*d.pendingIntent)
	d.pendingIntent = nil

	return intent, true
}

// CancelActionApproval returns to the exact Order subflow or topic input that opened approval.
func (d *TavernShopDialog) CancelActionApproval() {
	if d.phase != TavernShopAwaitingApproval {
		panic("TavernShopDialog.CancelActionApproval: dialog is not awaiting approval")
	}
	d.restoreApprovalReturn()
	d.clearAwaitingAction()
	d.transitionTo(TavernShopBrowse)
	d.syncPresentation()
}

// AcceptActionApproval records one exact request emitted by TavernActionDialog.
func (d *TavernShopDialog) AcceptActionApproval(request TavernActionRequest) {
	if d.phase != TavernShopAwaitingApproval || !tavernIntentAllowsRequest(d.awaitingIntent, request) {
		panic("TavernShopDialog.AcceptActionApproval: request does not match pending approval")
	}
	d.pendingIntent = nil
	d.awaitingRequest = request
	d.transitionTo(TavernShopAwaitingResult)
	d.syncPresentation()
}

// ApplyResult installs complete refreshed authority and exact finalized workflow presentation.
func (d *TavernShopDialog) ApplyResult(
	request TavernActionRequest,
	refreshed game_state.TavernShopState,
	display TavernActionResultDisplay,
) {
	if d.phase != TavernShopAwaitingResult || request != d.awaitingRequest || display.Action != request.Action {
		panic("TavernShopDialog.ApplyResult: result does not match accepted request")
	}
	validateTavernResultDisplay(request, display)
	d.applyState(refreshed)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	if display.SuppressInlineReceipt {
		d.receiptText = ""
	}
	d.inlineFeedback = ""
	if display.PaidInformation != nil {
		d.appendPaidTranscript(*display.PaidInformation)
	}
	d.clearAwaitingAction()
	d.restoreAfterResult(request.Action)
	d.transitionTo(TavernShopBrowse)
	d.syncPresentation()
	if display.Dismiss {
		d.Dismiss()
	}
}

func (d *TavernShopDialog) restoreAfterResult(action TavernAction) {
	switch action {
	case TavernActionStop:
		if !d.state.CustomerEligible {
			panic("TavernShopDialog.ApplyResult: accepted Stop must establish customer eligibility")
		}
		d.mode = TavernShopOrder
		d.subflow = TavernOrderMenu
		d.setTopicInput("")
	case TavernActionInformation:
		d.mode = TavernShopConversation
		d.setTopicInput("")
	case TavernActionDrink:
		d.mode = TavernShopOrder
		d.subflow = TavernOrderMenu
		d.setTopicInput("")
	case TavernActionMeal, TavernActionProvision:
		d.restoreApprovalReturn()
	case TavernActionUnknown:
		panic("TavernShopDialog.ApplyResult: empty action cannot produce a result")
	default:
		panic("TavernShopDialog.ApplyResult: unknown action")
	}
}

// PreviewStop returns to the prior subflow without invoking Tavern authority or changing state.
func (d *TavernShopDialog) PreviewStop(request TavernActionRequest) {
	if d.phase != TavernShopAwaitingResult || request != d.awaitingRequest {
		panic("TavernShopDialog.PreviewStop: request does not match accepted Preview")
	}
	d.restoreApprovalReturn()
	d.latestExchange = "Preview stopped before Tavern authority."
	d.receiptText = "PREVIEW: gold, food, intoxication, karma, and customer state are unchanged."
	d.inlineFeedback = ""
	d.clearAwaitingAction()
	d.transitionTo(TavernShopBrowse)
	d.syncPresentation()
}

// ApplySessionError disables Tavern actions until Enter or Escape acknowledges the error.
func (d *TavernShopDialog) ApplySessionError(message string) {
	validateTavernFinalizedText("session error", message)
	d.pendingTopic = nil
	d.clearAwaitingAction()
	d.latestExchange = message
	d.receiptText = ""
	d.inlineFeedback = message
	d.transitionTo(TavernShopSessionError)
	d.syncPresentation()
}

// GetTopicInput returns a defensive string copy of the raw input.
func (d *TavernShopDialog) GetTopicInput() string {
	if d.topicField != nil {
		return d.topicField.Text()
	}

	return string(d.topicInput)
}

// GetInformationTabLabel returns the exact location-specific persistent mode label.
func (d *TavernShopDialog) GetInformationTabLabel() string {
	return fmt.Sprintf("[%c]%s", d.state.Menu.Information.Key, d.state.Menu.Information.Name[1:])
}

// GetShell returns the shared merchant shell.
func (d *TavernShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// GetWorkspaceBounds returns the responsive family-owned workspace.
func (d *TavernShopDialog) GetWorkspaceBounds() image.Rectangle {
	d.ensureLayout()

	return d.shell.GetWorkspaceBounds()
}

// IsDismissed reports whether workflow may remove the parent.
func (d *TavernShopDialog) IsDismissed() bool { return d.dismissed }

// IsVisible reports whether the parent currently owns presentation and input.
func (d *TavernShopDialog) IsVisible() bool { return d.visible && !d.dismissed }

// Dismiss closes the parent without emitting another request.
func (d *TavernShopDialog) Dismiss() {
	if d.dismissed {
		return
	}
	d.keyboard.PrepareForTransition(0)
	d.visible = false
	d.dismissed = true
	d.syncTopicFocus()
}

func (d *TavernShopDialog) activateOrderSelection() {
	rows := tavernOrderRows(d.state)
	if len(rows) == 0 {
		return
	}
	switch rows[d.selectedOrder].Item {
	case TavernOrderMeal:
		d.beginActionApproval(d.mealIntent())
	case TavernOrderDrink:
		if d.state.Menu.Drink.IsWineList {
			d.setOrderSubflow(TavernOrderWinePicker)
		} else {
			d.beginActionApproval(d.drinkIntent(d.state.Drink))
		}
	case TavernOrderProvision:
		d.beginActionApproval(d.provisionIntent())
	default:
		panic("TavernShopDialog.activateOrderSelection: unknown Order item")
	}
}

func (d *TavernShopDialog) beginActionApproval(intent TavernActionIntent) {
	if d.phase != TavernShopBrowse || d.pendingIntent != nil {
		return
	}
	d.clearTransientPresentation()
	d.approvalReturn = d.captureApprovalReturn()
	d.pendingIntent = &intent
	d.awaitingIntent = intent
	d.transitionTo(TavernShopAwaitingApproval)
	d.syncPresentation()
}

func (d *TavernShopDialog) mealIntent() TavernActionIntent {
	intent := newTavernActionIntent(TavernActionMeal, tavernMealRequest(d.state), d.preview)
	intent.Meal = d.state.Meal

	return intent
}

func (d *TavernShopDialog) drinkIntent(quote game_state.TavernDrinkQuote) TavernActionIntent {
	approval := game_state.TavernDrinkApprovalPurchase
	stopRequest := emptyTavernActionRequest()
	if quote.Warning {
		approval = game_state.TavernDrinkApprovalContinue
		stopRequest = tavernStopRequest(d.state)
	}

	intent := newTavernActionIntent(
		TavernActionDrink,
		tavernDrinkRequest(d.state, quote, approval),
		d.preview,
	)
	intent.StopRequest = stopRequest
	intent.Drink = quote

	return intent
}

func (d *TavernShopDialog) provisionIntent() TavernActionIntent {
	intent := newTavernActionIntent(
		TavernActionProvision,
		tavernProvisionRequest(d.state, tavernMinimumProvisionPacks),
		d.preview,
	)
	intent.Provision = d.state.Provision
	intent.PackCount = tavernMinimumProvisionPacks

	return intent
}

func (d *TavernShopDialog) submitTopic() {
	if d.phase != TavernShopBrowse || d.pendingTopic != nil {
		return
	}
	intent := TavernTopicIntent{Input: d.GetTopicInput()}
	if intent.Input == "" {
		d.setMode(TavernShopOrder)
		d.latestExchange = d.prose.ConversationExit
		d.syncPresentation()

		return
	}
	d.pendingTopic = &intent
	d.quotedTopic = intent
	d.inlineFeedback = ""
	d.transitionTo(TavernShopAwaitingTopicQuote)
	d.syncPresentation()
}

func (d *TavernShopDialog) appendTopicRunes(events []rune) {
	changed := false
	for _, event := range events {
		if len(d.topicInput) >= tavernTopicRuneLimit {
			break
		}
		if !unicode.IsPrint(event) {
			continue
		}
		d.topicInput = append(d.topicInput, event)
		changed = true
	}
	if changed {
		d.inlineFeedback = ""
		d.populateBodyOutput()
	}
}

func (d *TavernShopDialog) setMode(mode TavernShopMode) {
	d.Reset()
	if mode < TavernShopOrder || mode > TavernShopConversation {
		panic("TavernShopDialog.setMode: unknown mode")
	}
	if d.mode == mode {
		return
	}
	d.mode = mode
	d.clearTransientPresentation()
	switch {
	case mode == TavernShopOrder:
		d.latestExchange = d.prose.OrderPrompt
		d.setTopicInput("")
	case !d.state.CustomerEligible:
		// Original tavern information silently ignores Chat before the
		// customer gate. Redux keeps the tab selectable as a read-only explanation.
		d.latestExchange = d.prose.OrderPrompt
	default:
		d.latestExchange = d.prose.ConversationPrompt
	}
	d.prepareForTransition()
	d.syncPresentation()
}

func (d *TavernShopDialog) setOrderSubflow(subflow TavernOrderSubflow) {
	d.Reset()
	if subflow < TavernOrderMenu || subflow > TavernOrderWinePicker {
		panic("TavernShopDialog.setOrderSubflow: unknown subflow")
	}
	if d.subflow == subflow {
		return
	}
	d.subflow = subflow
	d.clearTransientPresentation()
	d.prepareForTransition()
	d.syncPresentation()
}

func (d *TavernShopDialog) moveOrderSelection(direction int) {
	rows := tavernOrderRows(d.state)
	if len(rows) == 0 {
		return
	}
	selected := max(0, min(len(rows)-1, d.selectedOrder+direction))
	if selected != d.selectedOrder {
		d.selectedOrder = selected
		d.clearTransientPresentation()
		d.populateBodyOutput()
	}
}

func (d *TavernShopDialog) directOrderSelection() (int, bool) {
	for index, row := range tavernOrderRows(d.state) {
		if d.keyboard.IsKeyPressedAndRegister(tavernASCIIKey(row.Key)) {
			return index, true
		}
	}

	return 0, false
}

func (d *TavernShopDialog) informationShortcutPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(tavernASCIIKey(d.state.Menu.Information.Key))
}

func (d *TavernShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *TavernShopDialog) captureApprovalReturn() tavernApprovalReturn {
	return tavernApprovalReturn{
		mode:          d.mode,
		subflow:       d.subflow,
		topicInput:    d.GetTopicInput(),
		selectedOrder: d.selectedOrder,
		selectedWine:  d.selectedWine,
	}
}

func (d *TavernShopDialog) restoreApprovalReturn() {
	d.mode = d.approvalReturn.mode
	d.subflow = d.approvalReturn.subflow
	d.setTopicInput(d.approvalReturn.topicInput)
	d.selectedOrder = d.approvalReturn.selectedOrder
	d.selectedWine = d.approvalReturn.selectedWine
}

func (d *TavernShopDialog) applyState(refreshed game_state.TavernShopState) {
	refreshed = validatedTavernShopState(refreshed)
	if refreshed.Merchant.ID != d.state.Merchant.ID {
		panic("TavernShopDialog: refreshed state belongs to a different Tavern")
	}
	becameEligible := !d.state.CustomerEligible && refreshed.CustomerEligible
	d.state = refreshed
	if becameEligible && d.mode == TavernShopConversation {
		d.latestExchange = d.prose.ConversationPrompt
	}
	d.selectedOrder = min(d.selectedOrder, len(tavernOrderRows(refreshed))-1)
	d.selectedWine = min(d.selectedWine, max(0, len(refreshed.Wines)-1))
	d.calculateLayout()
}

func (d *TavernShopDialog) appendPaidTranscript(entry TavernTranscriptEntry) {
	validateTavernFinalizedText("paid information question", entry.Question)
	validateTavernFinalizedText("paid information answer", entry.Answer)
	d.transcript = append(d.transcript, entry)
	d.transcriptScroll = d.transcriptTailStart()
}

func (d *TavernShopDialog) scrollTranscript(direction int) {
	if len(d.transcript) == 0 || direction == 0 {
		return
	}
	start, end := d.transcriptViewportRange()
	if direction > 0 {
		if end < len(d.transcript) {
			d.transcriptScroll = min(end, d.transcriptTailStart())
		}
	} else if start > 0 {
		d.transcriptScroll = d.transcriptPageStartEndingAt(start)
	}
	d.populateBodyOutput()
}

func (d *TavernShopDialog) transcriptViewportRange() (int, int) {
	if len(d.transcript) == 0 {
		return 0, 0
	}
	start := max(0, min(len(d.transcript)-1, d.transcriptScroll))
	end := start
	used := 0
	capacity := d.transcriptLineCapacity()
	for end < len(d.transcript) {
		lines := d.transcriptEntryLineCount(d.transcript[end])
		if end > start && used+lines > capacity {
			break
		}
		used += lines
		end++
	}

	return start, end
}

func (d *TavernShopDialog) transcriptTailStart() int {
	return d.transcriptPageStartEndingAt(len(d.transcript))
}

func (d *TavernShopDialog) transcriptPageStartEndingAt(end int) int {
	start := max(0, min(len(d.transcript), end))
	used := 0
	capacity := d.transcriptLineCapacity()
	for start > 0 {
		lines := d.transcriptEntryLineCount(d.transcript[start-1])
		if start < end && used+lines > capacity {
			break
		}
		start--
		used += lines
	}

	return start
}

func (d *TavernShopDialog) transcriptLineCapacity() int {
	if d.bodyOutput == nil || d.bodyText.Dy() <= 0 {
		return 1
	}
	capacity := d.bodyOutput.GetLineCapacity(max(1, d.bodyText.Dy()))

	return max(1, capacity-d.conversationNonTranscriptLineCount())
}

func (d *TavernShopDialog) conversationNonTranscriptLineCount() int {
	if d.bodyFont == nil || d.bodyOutput == nil {
		return tavernConversationBaseLines
	}
	lines := d.conversationLineCount(strings.ToUpper(d.state.Menu.Information.Name))
	lines += d.conversationLineCount("Subject: " + d.topicRenderingText() + "_")
	lines += d.conversationLineCount(d.receiptText)
	lines += d.conversationLineCount(d.inlineFeedback)

	return lines
}

func (d *TavernShopDialog) conversationLineCount(value string) int {
	if strings.TrimSpace(value) == "" {
		return 0
	}
	maxChars := d.bodyFont.CharsForWidth(max(1, d.bodyText.Dx()))

	return max(1, text.MeasureText(value, d.bodyFont, d.bodyOutput.GetLineSpacing(), max(1, maxChars)).Lines)
}

func (d *TavernShopDialog) transcriptEntryLineCount(entry TavernTranscriptEntry) int {
	if d.bodyFont == nil || d.bodyOutput == nil {
		return tavernTranscriptMinimumLines
	}
	maxChars := d.bodyFont.CharsForWidth(max(1, d.bodyText.Dx()))
	question := text.MeasureText("Q: "+entry.Question, d.bodyFont, d.bodyOutput.GetLineSpacing(), max(1, maxChars))
	answer := text.MeasureText("A: "+entry.Answer, d.bodyFont, d.bodyOutput.GetLineSpacing(), max(1, maxChars))

	return max(tavernTranscriptMinimumLines, question.Lines+answer.Lines)
}

func (d *TavernShopDialog) clearTransientPresentation() {
	d.receiptText = ""
	d.inlineFeedback = ""
}

func (d *TavernShopDialog) clearAwaitingAction() {
	d.pendingIntent = nil
	d.awaitingIntent = emptyTavernActionIntent()
	d.awaitingRequest = emptyTavernActionRequest()
	d.quotedTopic = TavernTopicIntent{Input: ""}
}

func (d *TavernShopDialog) transitionTo(phase TavernShopDialogPhase) {
	d.phase = phase
	d.prepareForTransition()
}

func (d *TavernShopDialog) prepareForTransition() {
	d.keyboard.PrepareForTransition(0)
	d.inputKeysReady = !d.topicInputSuspended && tavernParentControlsReleased(d.isKeyPressed)
	d.syncTopicFocus()
}

func (d *TavernShopDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return
	}
	if d.bodyOutput != nil && width == d.lastScreenWidth && height == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth, d.lastScreenHeight = width, height
	d.shell.GetBounds()
	_, d.modeOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: tavernShopLineSpacing,
		MaxLines: tavernShopModeHeaderMaxLines, MaxChars: tavernShopMinimumTextChars,
	})
	d.bodyFont, d.bodyOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: tavernShopLineSpacing,
		MaxLines: tavernShopMaximumBodyLines, MaxChars: tavernShopMinimumTextChars,
	})
	d.calculateLayout()
	d.populateOutputs()
}

func (d *TavernShopDialog) calculateLayout() {
	if d.bodyOutput == nil {
		return
	}
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(tavernShopMinimumInset, min(workspace.Dx(), workspace.Dy())/tavernShopInsetDivisor)
	d.workspaceContent = insetRectangle(workspace, inset)
	modeLines := 1
	if d.preview {
		modeLines = tavernShopModeHeaderMaxLines
	}
	modeHeight := max(1, int(math.Ceil(d.modeOutput.GetTotalHeightForLines(modeLines)))+inset)
	d.modeBounds = image.Rect(
		d.workspaceContent.Min.X, d.workspaceContent.Min.Y,
		d.workspaceContent.Max.X, min(d.workspaceContent.Max.Y, d.workspaceContent.Min.Y+modeHeight),
	)
	d.bodyBounds = image.Rect(
		d.workspaceContent.Min.X, d.modeBounds.Max.Y,
		d.workspaceContent.Max.X, d.workspaceContent.Max.Y,
	)
	d.bodyText = insetRectangle(d.bodyBounds, inset)
	d.modeOutput.SetMaxCharsPerLine(max(
		tavernShopMinimumTextChars,
		d.modeOutput.Font.CharsForWidth(max(1, d.modeBounds.Dx())),
	))
	capacity := max(1, d.bodyOutput.GetLineCapacity(max(1, d.bodyText.Dy())))
	d.bodyOutput = text.NewOutput(
		d.bodyFont,
		d.bodyOutput.GetLineSpacing(),
		capacity,
		max(tavernShopMinimumTextChars, d.bodyFont.CharsForWidth(max(1, d.bodyText.Dx()))),
	)
	d.transcriptScroll = min(d.transcriptScroll, max(0, len(d.transcript)-1))
}

func (d *TavernShopDialog) populateOutputs() {
	d.populateModeOutput()
	d.populateBodyOutput()
}

func (d *TavernShopDialog) populateModeOutput() {
	if d.modeOutput == nil {
		return
	}
	d.modeOutput.Reset()
	if d.preview {
		d.modeOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text("PREVIEW", DialogStatusWarningColor).Done()
	}
	colors := d.modeTabColors()
	orderLabel := "[O]rder"
	informationLabel := d.GetInformationTabLabel()
	if d.mode == TavernShopConversation {
		orderLabel = "Order"
		informationLabel = strings.NewReplacer("[", "", "]", "").Replace(informationLabel)
	}
	d.modeOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text(orderLabel, colors[TavernShopOrder]).
		Text("    "+informationLabel, colors[TavernShopConversation]).Done()
}

func (d *TavernShopDialog) populateBodyOutput() {
	if d.bodyOutput == nil {
		return
	}
	d.bodyOutput.Reset()
	d.topicBounds = image.Rect(0, 0, 0, 0)
	d.topicCaretBounds = image.Rect(0, 0, 0, 0)
	d.topicSelectionBounds = image.Rect(0, 0, 0, 0)
	d.rowSelectionBounds = image.Rect(0, 0, 0, 0)
	if d.receiptText != "" && d.mode != TavernShopConversation {
		if d.preview {
			d.bodyOutput.AppendLine().Text("PREVIEW", DialogStatusWarningColor).Done()
		}
		d.bodyOutput.AppendLine().Text(d.receiptText, DialogValueColor).WordWrap().Done()

		return
	}
	if d.mode == TavernShopConversation {
		d.populateConversationBody()

		return
	}
	switch d.subflow {
	case TavernOrderMenu:
		d.populateOrderBody()
	case TavernOrderWinePicker:
		d.populateWineBody()
	default:
		panic("TavernShopDialog.populateBodyOutput: unknown Order subflow")
	}
}

func (d *TavernShopDialog) populateOrderBody() {
	d.bodyOutput.AppendLine().Text("ORDER", DialogHeaderColor).Done()
	for index, row := range tavernOrderRows(d.state) {
		charge, affordable := uint16(0), true
		switch row.Item {
		case TavernOrderMeal:
			charge, affordable = d.state.Meal.Charge, d.state.Meal.Affordable
		case TavernOrderDrink:
			charge, affordable = d.state.Drink.Charge, d.state.Drink.Affordable || d.state.Menu.Drink.IsWineList
		case TavernOrderProvision: // Charity can still answer an empty purse.
		}
		d.appendTavernRow(index == d.selectedOrder,
			fmt.Sprintf("[%c] %s%s", row.Key, row.Name, d.orderRowPrice(row.Item)), charge, affordable)
	}
	d.bodyOutput.AppendLine().LightGrayf(
		"Meals and ordinary drinks serve %d party members.", d.state.DinerCount,
	).Done()
	d.bodyOutput.AppendLine().LightGrayf("Food carried: %d", d.state.Food).Done()
	if d.inlineFeedback != "" {
		d.bodyOutput.AppendLine().Text(d.inlineFeedback, DialogStatusWarningColor).WordWrap().Done()
	}
}

func (d *TavernShopDialog) populateWineBody() {
	d.bodyOutput.AppendLine().Text("CHOOSE WINE", DialogHeaderColor).Done()
	for index, quote := range d.state.Wines {
		d.appendTavernRow(index == d.selectedWine,
			fmt.Sprintf("[%c] %-10s %d gold", 'A'+rune(index), quote.Wine.Name, quote.Charge), quote.Charge, quote.Affordable)
	}
}

// appendTavernRow uses the shared selection bar and, like Reagent and Arms,
// dims an unaffordable choice and states the gold it needs.
func (d *TavernShopDialog) appendTavernRow(selected bool, label string, charge uint16, affordable bool) {
	color := DialogValueColor
	if !affordable {
		label += fmt.Sprintf(" (need %d gold)", charge)
		color = DialogEmptyColor
	}
	if selected && d.phase == TavernShopBrowse {
		spacing := d.bodyOutput.GetLineSpacing()
		top := d.bodyText.Min.Y + int(float64(d.bodyOutput.GetPanelContentLineCount())*spacing)
		d.rowSelectionBounds = image.Rect(d.bodyText.Min.X, top, d.bodyText.Max.X, top+int(math.Ceil(spacing)))
		color = DialogSelectedText
	}
	d.bodyOutput.AppendLine().Text(label, color).Done()
}

func (d *TavernShopDialog) populateConversationBody() {
	d.bodyOutput.AppendLine().Text(strings.ToUpper(d.state.Menu.Information.Name), DialogHeaderColor).Done()
	if !d.state.CustomerEligible {
		d.bodyOutput.AppendLine().Text(
			d.state.Menu.Information.Name+" becomes available after an order.",
			DialogEmptyColor,
		).WordWrap().Done()

		return
	}
	if d.receiptText != "" {
		d.bodyOutput.AppendLine().Text(d.receiptText, DialogValueColor).WordWrap().Done()
	}
	start, end := d.transcriptViewportRange()
	for _, entry := range d.transcript[start:end] {
		d.bodyOutput.AppendLine().Text("Q: "+entry.Question, DialogLabelColor).WordWrap().Done()
		d.bodyOutput.AppendLine().Text("A: "+entry.Answer, DialogValueColor).WordWrap().Done()
	}
	if d.inlineFeedback != "" {
		d.bodyOutput.AppendLine().Text(d.inlineFeedback, DialogControlHintColor).WordWrap().Done()
	}
	d.populateTopicLine()
}

func (d *TavernShopDialog) orderRowPrice(item TavernOrderItem) string {
	switch item {
	case TavernOrderMeal:
		return fmt.Sprintf(
			" - %d each / %d total", d.state.Menu.Meal.BasePrice, d.state.Meal.Charge,
		)
	case TavernOrderDrink:
		if d.state.Menu.Drink.IsWineList {
			return " - choose bottle"
		}

		return fmt.Sprintf(
			" - %d each / %d total", d.state.Menu.Drink.UnitPrice, d.state.Drink.Charge,
		)
	case TavernOrderProvision:
		return fmt.Sprintf(" - %d gold/pack", d.state.Provision.PricePerPack)
	default:
		panic("TavernShopDialog.orderRowPrice: unknown Order item")
	}
}

func (d *TavernShopDialog) modeTabColors() [tavernShopModeCount]stdcolor.Color {
	colors := [tavernShopModeCount]stdcolor.Color{DialogValueColor, DialogEmptyColor}
	if d.state.CustomerEligible {
		colors[TavernShopConversation] = DialogValueColor
	}
	switch {
	case d.mode == TavernShopOrder:
		colors[TavernShopOrder] = DialogLabelColor
	case d.state.CustomerEligible:
		colors[TavernShopConversation] = DialogLabelColor
	default:
		colors[TavernShopConversation] = DialogControlHintColor
	}

	return colors
}

func (d *TavernShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateOutputs()
}

func (d *TavernShopDialog) footerGuidance() string {
	switch d.phase {
	case TavernShopBrowse:
		if d.mode == TavernShopOrder {
			back := "Leave"
			if d.subflow == TavernOrderWinePicker {
				back = "Back to menu"
			}

			return "[Up/Down] Select  [Enter] Order\n[Tab] Conversation  [Esc] " + back
		}
		if !d.state.CustomerEligible {
			return "[Tab/Esc] Back to Order"
		}

		return "[Enter] Ask about subject  [Tab/Esc] Back to Order\n[Shift+Up/Down/PgUp/PgDn] Read history"
	case TavernShopAwaitingTopicQuote:
		return "Checking subject"
	case TavernShopAwaitingApproval:
		return "Complete or cancel the Tavern action dialog"
	case TavernShopAwaitingResult:
		return "Processing Tavern action"
	case TavernShopSessionError:
		return merchantLeaveFooter
	default:
		panic("TavernShopDialog.footerGuidance: unknown phase")
	}
}

func tavernOrderRows(state game_state.TavernShopState) []TavernOrderRow {
	rows := []TavernOrderRow{
		{Item: TavernOrderMeal, Name: state.Menu.Meal.Name, Key: state.Menu.Meal.Key},
		{Item: TavernOrderDrink, Name: state.Menu.Drink.Name, Key: state.Menu.Drink.Key},
	}
	if state.Menu.Provision.Available {
		rows = append(rows, TavernOrderRow{
			Item: TavernOrderProvision, Name: state.Menu.Provision.Name, Key: state.Menu.Provision.Key,
		})
	}

	return rows
}

func tavernMealRequest(state game_state.TavernShopState) TavernActionRequest {
	request := emptyTavernActionRequest()
	request.Action = TavernActionMeal
	request.Meal = game_state.TavernMealRequest{
		ReviewedMerchantID: state.Merchant.ID,
		ReviewedMeal:       state.Meal.Meal,
		ReviewedDinerCount: state.Meal.DinerCount,
		ReviewedCharge:     state.Meal.Charge,
		ReviewedGoldBefore: state.Meal.GoldBefore,
		ReviewedFoodBefore: state.Meal.FoodBefore,
	}

	return request
}

func tavernDrinkRequest(
	state game_state.TavernShopState,
	quote game_state.TavernDrinkQuote,
	approval game_state.TavernDrinkApproval,
) TavernActionRequest {
	request := emptyTavernActionRequest()
	request.Action = TavernActionDrink
	request.Drink = game_state.TavernDrinkRequest{
		ReviewedMerchantID: state.Merchant.ID,
		ReviewedDrink:      quote.Drink,
		ReviewedWineIndex:  quote.WineIndex,
		ReviewedWine:       quote.Wine,
		ReviewedDinerCount: quote.DinerCount,
		ReviewedCharge:     quote.Charge,
		ReviewedGoldBefore: quote.GoldBefore,
		ReviewedDrunk:      quote.Drunk,
		ReviewedSmashed:    quote.Smashed,
		ReviewedKarma:      quote.Karma,
		Approval:           approval,
	}

	return request
}

func tavernStopRequest(state game_state.TavernShopState) TavernActionRequest {
	request := emptyTavernActionRequest()
	request.Action = TavernActionStop
	request.Stop = game_state.TavernStopRequest{
		ReviewedMerchantID: state.Merchant.ID,
		ReviewedDrunk:      state.Drunk,
	}

	return request
}

func tavernProvisionRequest(state game_state.TavernShopState, packCount int) TavernActionRequest {
	request := emptyTavernActionRequest()
	request.Action = TavernActionProvision
	request.Provision = game_state.TavernProvisionRequest{
		ReviewedMerchantID: state.Merchant.ID,
		ReviewedProvision:  state.Provision.Provision,
		ReviewedPrice:      state.Provision.PricePerPack,
		ReviewedGoldBefore: state.Provision.GoldBefore,
		ReviewedFoodBefore: state.Provision.FoodBefore,
		PackCount:          packCount,
	}

	return request
}

func tavernInformationRequest(
	state game_state.TavernShopState,
	quote game_state.TavernInformationQuote,
) TavernActionRequest {
	request := emptyTavernActionRequest()
	request.Action = TavernActionInformation
	request.Information = game_state.TavernInformationRequest{
		ReviewedMerchantID: state.Merchant.ID,
		ReviewedTopic:      quote.Topic,
		ReviewedPrice:      quote.Price,
		ReviewedGoldBefore: quote.GoldBefore,
	}

	return request
}

func emptyTavernActionRequest() TavernActionRequest {
	var request TavernActionRequest
	request.Action = TavernActionUnknown

	return request
}

func emptyTavernActionIntent() TavernActionIntent {
	var intent TavernActionIntent
	intent.Action = TavernActionUnknown
	intent.Request = emptyTavernActionRequest()
	intent.StopRequest = emptyTavernActionRequest()

	return intent
}

func newTavernActionIntent(
	action TavernAction,
	request TavernActionRequest,
	preview bool,
) TavernActionIntent {
	intent := emptyTavernActionIntent()
	intent.Action = action
	intent.Request = request
	intent.Preview = preview

	return intent
}

func tavernIntentAllowsRequest(intent TavernActionIntent, request TavernActionRequest) bool {
	if request == intent.Request {
		return true
	}
	if intent.Action == TavernActionProvision && request.Action == TavernActionProvision {
		expected := intent.Request
		expected.Provision.PackCount = request.Provision.PackCount

		return request == expected && request.Provision.PackCount >= tavernMinimumProvisionPacks &&
			request.Provision.PackCount <= intent.Provision.MaximumUsefulPacks
	}

	return intent.Drink.Warning && request == intent.StopRequest
}

func cloneTavernShopState(state game_state.TavernShopState) game_state.TavernShopState {
	clone := state
	clone.Wines = append([]game_state.TavernDrinkQuote(nil), state.Wines...)
	clone.Provision.Approvals = append(
		[]game_state.TavernProvisionApproval(nil),
		state.Provision.Approvals...,
	)

	return clone
}

func cloneTavernActionIntent(intent TavernActionIntent) TavernActionIntent {
	intent.Provision.Approvals = append(
		[]game_state.TavernProvisionApproval(nil),
		intent.Provision.Approvals...,
	)

	return intent
}

func validatedTavernShopState(state game_state.TavernShopState) game_state.TavernShopState {
	validateTavernShopState(state)

	return cloneTavernShopState(state)
}

//nolint:cyclop // The complete detached Tavern snapshot is one presentation boundary.
func validateTavernShopState(state game_state.TavernShopState) {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyTavern ||
		strings.TrimSpace(state.Merchant.OwnerName) == "" || strings.TrimSpace(state.Merchant.ShopName) == "" ||
		state.Menu.MerchantID != state.Merchant.ID {
		panic("TavernShopDialog: complete detached Tavern merchant is required")
	}
	if state.PricingCharacterIndex < 0 || strings.TrimSpace(state.PricingCharacterName) == "" ||
		state.DinerCount <= 0 || state.Gold != state.Meal.GoldBefore || state.Food != state.Meal.FoodBefore {
		panic("TavernShopDialog: complete detached Tavern buyer state is required")
	}
	if strings.TrimSpace(state.Menu.Meal.Name) == "" || state.Menu.Meal.Key == 0 ||
		strings.TrimSpace(state.Menu.Drink.Name) == "" || state.Menu.Drink.Key == 0 ||
		strings.TrimSpace(state.Menu.Information.Name) == "" || state.Menu.Information.Key == 0 {
		panic("TavernShopDialog: complete canonical Tavern menu is required")
	}
	if state.Meal.Meal != state.Menu.Meal || state.Meal.DinerCount != state.DinerCount || state.Meal.Charge == 0 {
		panic("TavernShopDialog: detached meal quote is inconsistent")
	}
	validateTavernGoldProjection(
		state.Meal.Charge,
		state.Meal.GoldBefore,
		state.Meal.GoldAfterPayment,
		state.Meal.Affordable,
	)
	if state.Menu.Drink.IsWineList {
		if len(state.Wines) != tavernWineCount {
			panic("TavernShopDialog: wine Tavern requires six source-order quotes")
		}
		for index, quote := range state.Wines {
			validateTavernDrinkQuote(quote, state, index)
		}
	} else {
		if len(state.Wines) != 0 {
			panic("TavernShopDialog: ordinary drink Tavern cannot expose wines")
		}
		validateTavernDrinkQuote(state.Drink, state, game_state.TavernOrdinaryDrinkWineIndex)
	}
	if state.Menu.Provision.Available {
		validateTavernProvisionQuote(state)
	}
}

func validateTavernProvisionQuote(state game_state.TavernShopState) {
	if state.Provision.Provision != state.Menu.Provision || state.Provision.PricePerPack <= 0 ||
		state.Provision.GoldBefore != state.Gold || state.Provision.FoodBefore != state.Food ||
		!state.Provision.HasValidApprovals() {
		panic("TavernShopDialog: detached provision quote is inconsistent")
	}
}

func validateTavernDrinkQuote(
	quote game_state.TavernDrinkQuote,
	state game_state.TavernShopState,
	wineIndex int,
) {
	if quote.Drink != state.Menu.Drink || quote.WineIndex != wineIndex || quote.DinerCount != state.DinerCount ||
		quote.Charge == 0 || quote.GoldBefore != state.Gold || quote.Drunk != state.Drunk ||
		quote.Smashed != state.Smashed || quote.Karma != state.Karma ||
		quote.Warning != (state.Drunk == tavernWarningDrunkLevel) || quote.StopAvailable != quote.Warning {
		panic("TavernShopDialog: detached drink quote is inconsistent")
	}
	if wineIndex >= 0 && (strings.TrimSpace(quote.Wine.Name) == "" || quote.Wine.Price != int(quote.Charge)) {
		panic("TavernShopDialog: detached wine quote is incomplete")
	}
	validateTavernGoldProjection(quote.Charge, quote.GoldBefore, quote.GoldAfterPayment, quote.Affordable)
}

func validateTavernGoldProjection(charge, before, after uint16, affordable bool) {
	wantAffordable := charge <= before
	wantAfter := before
	if wantAffordable {
		wantAfter -= charge
	}
	if affordable != wantAffordable || after != wantAfter {
		panic("TavernShopDialog: detached gold projection is inconsistent")
	}
}

func validateTavernInformationQuote(quote game_state.TavernInformationQuote, state game_state.TavernShopState) {
	if !state.CustomerEligible || strings.TrimSpace(quote.Topic.Key) == "" || quote.Topic.Price <= 0 ||
		quote.Price != quote.Topic.Price || quote.GoldBefore != state.Gold {
		panic("TavernShopDialog: complete authority-recognized information quote is required")
	}
	wantAffordable := int(quote.GoldBefore) >= quote.Price
	wantAfter := quote.GoldBefore
	if wantAffordable {
		wantAfter -= uint16(quote.Price) //nolint:gosec // Validated positive canonical topic price is bounded by authority.
	}
	if quote.Affordable != wantAffordable || quote.GoldAfterPayment != wantAfter {
		panic("TavernShopDialog: information gold projection is inconsistent")
	}
}

func validateTavernResultDisplay(request TavernActionRequest, display TavernActionResultDisplay) {
	validateTavernFinalizedText("result exchange", display.MerchantExchange)
	validateTavernFinalizedText("result receipt", display.Receipt)
	if display.PaidInformation == nil {
		return
	}
	if request.Action != TavernActionInformation {
		panic("TavernShopDialog: paid transcript is valid only for information results")
	}
	validateTavernFinalizedText("paid information question", display.PaidInformation.Question)
	validateTavernFinalizedText("paid information answer", display.PaidInformation.Answer)
}

func validateTavernShopProse(prose TavernShopDialogProse) {
	validateTavernFinalizedText("greeting", prose.Greeting)
	validateTavernFinalizedText("Order prompt", prose.OrderPrompt)
	validateTavernFinalizedText("conversation prompt", prose.ConversationPrompt)
	validateTavernFinalizedText("conversation exit", prose.ConversationExit)
	validateTavernFinalizedText("unknown topic response", prose.UnknownTopic)
}

func validateTavernFinalizedText(label, value string) {
	if strings.TrimSpace(value) == "" {
		panic("TavernShopDialog: finalized " + label + " is required")
	}
}

func tavernParentControlsReleased(isKeyPressed func(ebiten.Key) bool) bool {
	for _, key := range tavernParentControlKeys() {
		if isKeyPressed(key) {
			return false
		}
	}

	return true
}

func tavernParentControlKeys() []ebiten.Key {
	keys := []ebiten.Key{
		ebiten.KeyEscape, ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyBackspace,
		ebiten.KeyTab, ebiten.KeyDelete, ebiten.KeyHome, ebiten.KeyEnd,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight,
		ebiten.KeyPageUp, ebiten.KeyPageDown,
	}
	for key := ebiten.KeyA; key <= ebiten.KeyZ; key++ {
		keys = append(keys, key)
	}

	return keys
}

func tavernWineKeys() [tavernWineCount]ebiten.Key {
	return [tavernWineCount]ebiten.Key{
		ebiten.KeyA, ebiten.KeyB, ebiten.KeyC, ebiten.KeyD, ebiten.KeyE, ebiten.KeyF,
	}
}

func tavernASCIIKey(key byte) ebiten.Key {
	if key >= 'a' && key <= 'z' {
		key -= 'a' - 'A'
	}
	if key < 'A' || key > 'Z' {
		panic("TavernShopDialog: menu shortcut must be an ASCII letter")
	}

	return ebiten.Key(int(ebiten.KeyA) + int(key-'A'))
}
