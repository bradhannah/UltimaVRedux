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
	innShopModeCount             = 3
	innShopLeaveColumns          = 3
	innShopPickupColumns         = 4
	innShopMaximumRows           = party_state.MaxActiveParty
	innShopMinimumInset          = 4
	innShopInsetDivisor          = 64
	innShopCellInsetUnits        = 2
	innShopDoubleInset           = 2
	innShopLineSpacing           = 1.06
	innShopMinimumTextChars      = 8
	innShopMaximumDetailLines    = 24
	innShopDetailProjectionLines = 9
	innShopModeHeaderMaxLines    = 2
	innShopPickupNumericColumn   = 2
	innShopColumnName            = "NAME"
	innShopColumnStatus          = "STATUS"
	innStatusDead                = "Dead"
	innStatusUnknown             = "Unknown"
)

var _ Widget = (*InnShopDialog)(nil)

// InnShopMode identifies the persistent Inn workspace selected by the player.
type InnShopMode byte

const (
	// InnShopRest shows the complete overnight consequence card.
	InnShopRest InnShopMode = iota
	// InnShopLeave shows active members who may be left at this Inn.
	InnShopLeave
	// InnShopPickup shows guests currently registered at this Inn.
	InnShopPickup
)

func (m InnShopMode) String() string {
	switch m {
	case InnShopRest:
		return "REST"
	case InnShopLeave:
		return "LEAVE"
	case InnShopPickup:
		return "PICK UP"
	default:
		panic("InnShopMode.String: unknown mode")
	}
}

// InnAction identifies the authority operation reviewed by InnActionDialog.
type InnAction byte

const (
	// InnActionRest requests one overnight stay.
	InnActionRest InnAction = iota
	// InnActionLeave requests one active-member transfer into this Inn.
	InnActionLeave
	// InnActionPickup requests one stored-guest transfer into the active party.
	InnActionPickup
)

func (a InnAction) String() string {
	switch a {
	case InnActionRest:
		return "Rest"
	case InnActionLeave:
		return "Leave"
	case InnActionPickup:
		return "Pick Up"
	default:
		panic("InnAction.String: unknown action")
	}
}

// InnShopDialogPhase identifies the workflow-owned parent lifecycle.
type InnShopDialogPhase byte

const (
	// InnShopBrowse accepts mode, row, review, and leave input.
	InnShopBrowse InnShopDialogPhase = iota
	// InnShopAwaitingApproval blocks the parent while InnActionDialog owns input.
	InnShopAwaitingApproval
	// InnShopAwaitingResult blocks the parent while authority owns an approved request.
	InnShopAwaitingResult
	// InnShopSessionError displays a terminal workflow error until acknowledgement.
	InnShopSessionError
)

// InnShopDialogProse contains finalized merchant copy supplied by the workflow.
type InnShopDialogProse struct {
	Greeting     string
	RestPrompt   string
	LeavePrompt  string
	PickupPrompt string
}

// InnShopDialogConfig configures one detached, keyboard-only Inn shop.
type InnShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	State          game_state.InnShopState
	Prose          InnShopDialogProse
	Preview        bool
	IsKeyPressed   func(ebiten.Key) bool
}

// InnActionRequest is the typed union crossing from approval into workflow authority.
type InnActionRequest struct {
	Action InnAction
	Rest   game_state.InnRestRequest
	Leave  game_state.InnLeaveRequest
	Pickup game_state.InnPickupRequest
}

// InnActionIntent contains one detached authority review and its exact comparison request.
type InnActionIntent struct {
	Action         InnAction
	Request        InnActionRequest
	Rest           game_state.InnRestQuote
	Leave          game_state.InnLeaveRow
	Pickup         game_state.InnGuestRow
	InnName        string
	RoomOccupancy  int
	RoomCapacity   int
	PartySize      int
	PartyCapacity  int
	MonthlyRate    uint16
	HasMonthlyRate bool
	Preview        bool
}

// InnActionResultDisplay contains exact finalized receipt values supplied by the workflow.
type InnActionResultDisplay struct {
	Action           InnAction
	MerchantExchange string
	ListedCharge     uint16
	FaulineiTheft    uint16
	FinalGold        uint16
	PartySize        int
	RoomOccupancy    int
	Refused          bool
	CharacterName    string
	Members          []game_state.InnCharacterProjection
}

// InnShopDialog presents persistent Rest, Leave, and Pick Up modes without mutating authority.
type InnShopDialog struct {
	navigation
	keyboard     *input.Keyboard
	isKeyPressed func(ebiten.Key) bool
	state        game_state.InnShopState
	prose        InnShopDialogProse
	preview      bool

	mode            InnShopMode
	selectedLeave   int
	selectedPickup  int
	phase           InnShopDialogPhase
	helpVisible     bool
	visible         bool
	dismissed       bool
	actionKeysReady bool

	latestExchange  string
	receiptText     string
	inlineFeedback  string
	pendingIntent   *InnActionIntent
	awaitingRequest InnActionRequest

	shell        *MerchantDialogShell
	modeOutput   *text.Output
	rowOutput    *text.Output
	detailOutput *text.Output
	rowFont      *text.UltimaFont
	detailFont   *text.UltimaFont

	workspaceContent image.Rectangle
	modeBounds       image.Rectangle
	cardBounds       image.Rectangle
	tableBounds      image.Rectangle
	tableHeader      image.Rectangle
	rowBounds        []image.Rectangle
	columns          [innShopPickupColumns]image.Rectangle
	detailBounds     image.Rectangle
	detailText       image.Rectangle

	lastScreenWidth  int
	lastScreenHeight int
}

// NewInnShopDialog creates a visible Inn dialog from a complete authoritative snapshot.
func NewInnShopDialog(config InnShopDialogConfig) *InnShopDialog {
	if config.Keyboard == nil {
		panic("InnShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("InnShopDialog: merchant sprite is required")
	}
	validateInnShopProse(config.Prose)
	state := validatedInnShopState(config.State)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = ebiten.IsKeyPressed
	}

	dialog := &InnShopDialog{ //nolint:exhaustruct_v5 // Resolution-dependent presentation fields initialize on demand.
		navigation:      newNavigation(config.Keyboard.IsKeyHeld),
		keyboard:        config.Keyboard,
		isKeyPressed:    isKeyPressed,
		state:           state,
		prose:           config.Prose,
		preview:         config.Preview,
		mode:            InnShopRest,
		phase:           InnShopBrowse,
		visible:         true,
		latestExchange:  config.Prose.Greeting,
		actionKeysReady: innActionKeysReleased(isKeyPressed),
		awaitingRequest: emptyInnActionRequest(),
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

// Draw renders the shared 27/73 shell and the active Inn workspace.
func (d *InnShopDialog) Draw(screen *ebiten.Image) {
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
	if d.mode == InnShopRest || d.helpVisible {
		d.drawRestCard(screen)

		return
	}
	d.drawRosterTable(screen)
	d.drawDetailCard(screen)
}

// Update processes keyboard-only mode, row, review, and dismissal input.
func (d *InnShopDialog) Update() {
	if !d.IsVisible() {
		return
	}
	d.ensureLayout()
	if !d.actionKeysReady {
		if innActionKeysReleased(d.isKeyPressed) {
			d.actionKeysReady = true
		}

		return
	}
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}

	switch d.phase {
	case InnShopBrowse:
		d.updateBrowse()
	case InnShopAwaitingApproval, InnShopAwaitingResult:
		return
	case InnShopSessionError:
		if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.Dismiss()
		}
	default:
		panic("InnShopDialog.Update: unknown phase")
	}
}

func (d *InnShopDialog) updateBrowse() {
	if d.helpVisible {
		d.updateHelp()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()

		return
	}
	if d.updateBrowseMode() {
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
		d.reviewSelection()
	}
}

func (d *InnShopDialog) updateHelp() {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeyH) &&
		!d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		return
	}
	d.helpVisible = false
	d.calculateLayout()
	d.syncPresentation()
}

func (d *InnShopDialog) updateBrowseMode() bool {
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyH):
		d.helpVisible = true
		d.clearTransientPresentation()
		d.calculateLayout()
		d.syncPresentation()
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft):
		d.setMode(previousInnShopMode(d.mode))
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight):
		d.setMode(nextInnShopMode(d.mode))
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab):
		d.setMode(nextInnShopMode(d.mode))
	default:
		mode, requested := d.requestedDirectMode()
		if !requested {
			return false
		}
		d.setMode(mode)
	}

	return true
}

// ConsumePendingActionIntent returns and clears one nested approval intent.
func (d *InnShopDialog) ConsumePendingActionIntent() (InnActionIntent, bool) {
	if d.pendingIntent == nil {
		return emptyInnActionIntent(), false
	}
	intent := cloneInnActionIntent(*d.pendingIntent)
	d.pendingIntent = nil

	return intent, true
}

// CancelActionApproval returns to the same parent mode after Escape in InnActionDialog.
func (d *InnShopDialog) CancelActionApproval() {
	if d.phase != InnShopAwaitingApproval {
		panic("InnShopDialog.CancelActionApproval: dialog is not awaiting approval")
	}
	d.clearAwaitingAction()
	d.transitionTo(InnShopBrowse)
	d.syncPresentation()
}

// AcceptActionApproval records the exact modal request and awaits workflow authority.
func (d *InnShopDialog) AcceptActionApproval(request InnActionRequest) {
	if d.phase != InnShopAwaitingApproval || request != d.awaitingRequest {
		panic("InnShopDialog.AcceptActionApproval: request does not match pending approval")
	}
	d.pendingIntent = nil
	d.transitionTo(InnShopAwaitingResult)
	d.syncPresentation()
}

// ApplyResult installs refreshed authority and displays an exact structured receipt.
func (d *InnShopDialog) ApplyResult(
	request InnActionRequest,
	refreshed game_state.InnShopState,
	display InnActionResultDisplay,
) {
	if d.phase != InnShopAwaitingResult || request != d.awaitingRequest || display.Action != request.Action {
		panic("InnShopDialog.ApplyResult: result does not match accepted request")
	}
	validateInnFinalizedText("result exchange", display.MerchantExchange)
	refreshed = validatedInnShopState(refreshed)
	validateInnResultDisplay(request, refreshed, display)
	d.applyState(refreshed)
	d.latestExchange = display.MerchantExchange
	d.receiptText = FormatInnReceipt(display)
	d.inlineFeedback = ""
	d.clearAwaitingAction()
	d.transitionTo(InnShopBrowse)
	d.syncPresentation()
}

// PreviewStop returns to the same mode without invoking Inn authority.
func (d *InnShopDialog) PreviewStop(request InnActionRequest) {
	if d.phase != InnShopAwaitingResult || request != d.awaitingRequest {
		panic("InnShopDialog.PreviewStop: request does not match accepted Preview")
	}
	d.latestExchange = "Preview stopped before Inn authority."
	d.receiptText = "PREVIEW: gold, party, rooms, spells, and roster are unchanged."
	d.inlineFeedback = ""
	d.clearAwaitingAction()
	d.transitionTo(InnShopBrowse)
	d.syncPresentation()
}

// ApplySessionError disables Inn actions until Enter or Escape acknowledges the error.
func (d *InnShopDialog) ApplySessionError(message string) {
	validateInnFinalizedText("session error", message)
	d.clearAwaitingAction()
	d.latestExchange = message
	d.receiptText = ""
	d.inlineFeedback = message
	d.transitionTo(InnShopSessionError)
	d.syncPresentation()
}

// GetSelectedIndex returns the selected row index, or zero in Rest mode.
func (d *InnShopDialog) GetSelectedIndex() int {
	switch d.mode {
	case InnShopRest:
		return 0
	case InnShopLeave:
		return d.selectedLeave
	case InnShopPickup:
		return d.selectedPickup
	default:
		panic("InnShopDialog.GetSelectedIndex: unknown mode")
	}
}

// GetLeaveRowText returns the full player-facing values for one Leave row.
func (d *InnShopDialog) GetLeaveRowText(index int) [innShopLeaveColumns]string {
	if index < 0 || index >= len(d.state.LeaveRows) {
		panic("InnShopDialog.GetLeaveRowText: index out of range")
	}
	row := d.state.LeaveRows[index]
	eligibility := "Eligible"
	if !row.Eligible {
		eligibility = innLeaveBlockLabel(row.BlockReason)
	} else if innHasWarning(row.Warnings, game_state.InnWarningLastLivingMember) {
		eligibility = "SEVERE WARNING"
	}

	return [innShopLeaveColumns]string{row.Identity.Name, innStatusName(row.Status), eligibility}
}

// GetPickupRowText returns the full player-facing values for one Pick Up row.
func (d *InnShopDialog) GetPickupRowText(index int) [innShopPickupColumns]string {
	if index < 0 || index >= len(d.state.GuestRows) {
		panic("InnShopDialog.GetPickupRowText: index out of range")
	}
	row := d.state.GuestRows[index]

	return [innShopPickupColumns]string{
		row.Identity.Name,
		innStatusName(row.Status),
		fmt.Sprintf("%d/%d", row.MonthsAtInn, row.BilledMonths),
		strconv.FormatUint(uint64(row.Charge), 10),
	}
}

// GetLatestExchange returns the current finalized merchant exchange.
func (d *InnShopDialog) GetLatestExchange() string { return d.latestExchange }

// GetReceiptText returns the current workflow-supplied result presentation.
func (d *InnShopDialog) GetReceiptText() string { return d.receiptText }

// GetShell returns the unchanged shared merchant shell.
func (d *InnShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// IsDismissed reports whether the workflow may remove the parent.
func (d *InnShopDialog) IsDismissed() bool { return d.dismissed }

// IsVisible reports whether the parent currently owns presentation and input.
func (d *InnShopDialog) IsVisible() bool { return d.visible && !d.dismissed }

// Dismiss closes the parent without emitting another action.
func (d *InnShopDialog) Dismiss() {
	d.visible = false
	d.dismissed = true
}

func (d *InnShopDialog) requestedDirectMode() (InnShopMode, bool) {
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyR):
		return InnShopRest, true
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyL):
		return InnShopLeave, true
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyP):
		return InnShopPickup, true
	default:
		return InnShopRest, false
	}
}

func previousInnShopMode(mode InnShopMode) InnShopMode {
	switch mode {
	case InnShopRest:
		return InnShopPickup
	case InnShopLeave:
		return InnShopRest
	case InnShopPickup:
		return InnShopLeave
	default:
		panic("previousInnShopMode: unknown mode")
	}
}

func nextInnShopMode(mode InnShopMode) InnShopMode {
	switch mode {
	case InnShopRest:
		return InnShopLeave
	case InnShopLeave:
		return InnShopPickup
	case InnShopPickup:
		return InnShopRest
	default:
		panic("nextInnShopMode: unknown mode")
	}
}

func (d *InnShopDialog) setMode(mode InnShopMode) {
	if mode < InnShopRest || mode > InnShopPickup {
		panic("InnShopDialog.setMode: unknown mode")
	}
	if d.mode == mode && !d.helpVisible {
		return
	}
	d.helpVisible = false
	d.mode = mode
	switch mode {
	case InnShopRest:
		d.latestExchange = d.prose.RestPrompt
	case InnShopLeave:
		d.latestExchange = d.prose.LeavePrompt
	case InnShopPickup:
		d.latestExchange = d.prose.PickupPrompt
	}
	d.clearTransientPresentation()
	d.calculateLayout()
	d.syncPresentation()
}

func (d *InnShopDialog) moveSelection(direction int) {
	old := d.GetSelectedIndex()
	switch d.mode {
	case InnShopRest:
		return
	case InnShopLeave:
		if len(d.state.LeaveRows) == 0 {
			return
		}
		d.selectedLeave = max(0, min(len(d.state.LeaveRows)-1, d.selectedLeave+direction))
	case InnShopPickup:
		if len(d.state.GuestRows) == 0 {
			return
		}
		d.selectedPickup = max(0, min(len(d.state.GuestRows)-1, d.selectedPickup+direction))
	default:
		panic("InnShopDialog.moveSelection: unknown mode")
	}
	if old != d.GetSelectedIndex() {
		d.clearTransientPresentation()
		d.populateDetailOutput()
	}
}

func (d *InnShopDialog) reviewSelection() {
	if d.phase != InnShopBrowse || d.pendingIntent != nil {
		return
	}
	intent, blocked := d.selectedIntent()
	if blocked != "" {
		d.receiptText = ""
		d.inlineFeedback = blocked
		d.populateDetailOutput()

		return
	}
	d.clearTransientPresentation()
	d.pendingIntent = &intent
	d.awaitingRequest = intent.Request
	d.transitionTo(InnShopAwaitingApproval)
	d.syncPresentation()
}

func (d *InnShopDialog) selectedIntent() (InnActionIntent, string) {
	monthlyRate, hasMonthlyRate := trustedInnMonthlyRate(d.state)
	base := InnActionIntent{
		Action:         InnActionRest,
		Request:        emptyInnActionRequest(),
		Rest:           cloneInnRestQuote(d.state.Rest),
		Leave:          emptyInnLeaveRow(),
		Pickup:         emptyInnGuestRow(),
		InnName:        d.state.Merchant.ShopName,
		RoomOccupancy:  d.state.RoomOccupancy,
		RoomCapacity:   d.state.RoomCapacity,
		PartySize:      d.state.PartySize,
		PartyCapacity:  d.state.PartyCapacity,
		MonthlyRate:    monthlyRate,
		HasMonthlyRate: hasMonthlyRate,
		Preview:        d.preview,
	}
	switch d.mode {
	case InnShopRest:
		if d.state.Rest.BlockReason != game_state.InnRestBlockNone {
			return emptyInnActionIntent(), innRestBlockedText(d.state.Rest.BlockReason)
		}
		base.Action = InnActionRest
		base.Request = innRestRequest(d.state)
	case InnShopLeave:
		if len(d.state.LeaveRows) == 0 {
			return emptyInnActionIntent(), "No active party members are available to leave."
		}
		row := cloneInnLeaveRow(d.state.LeaveRows[d.selectedLeave])
		if !row.Eligible {
			return emptyInnActionIntent(), innLeaveBlockedText(row)
		}
		base.Action = InnActionLeave
		base.Leave = row
		base.Request = innLeaveRequest(d.state, row)
	case InnShopPickup:
		if len(d.state.GuestRows) == 0 {
			return emptyInnActionIntent(), "No guests are registered at this Inn."
		}
		row := cloneInnGuestRow(d.state.GuestRows[d.selectedPickup])
		if !row.Confirmable || row.BlockReason != game_state.InnPickupBlockNone {
			return emptyInnActionIntent(), innPickupBlockedText(row)
		}
		base.Action = InnActionPickup
		base.Pickup = row
		base.Request = innPickupRequest(d.state, row)
	default:
		panic("InnShopDialog.selectedIntent: unknown mode")
	}
	base.Request.Action = base.Action

	return cloneInnActionIntent(base), ""
}

func (d *InnShopDialog) applyState(refreshed game_state.InnShopState) {
	refreshed = validatedInnShopState(refreshed)
	if refreshed.Merchant.ID != d.state.Merchant.ID {
		panic("InnShopDialog: refreshed state belongs to a different Inn")
	}
	leaveIdentity, leaveIdentityOK := innLeaveIdentityAt(d.state.LeaveRows, d.selectedLeave)
	pickupIdentity, pickupIdentityOK := innPickupIdentityAt(d.state.GuestRows, d.selectedPickup)
	leaveFallback, pickupFallback := d.selectedLeave, d.selectedPickup
	d.state = refreshed
	d.selectedLeave = preservedInnLeaveIndex(refreshed.LeaveRows, leaveIdentity, leaveIdentityOK, leaveFallback)
	d.selectedPickup = preservedInnPickupIndex(refreshed.GuestRows, pickupIdentity, pickupIdentityOK, pickupFallback)
	d.calculateLayout()
}

func (d *InnShopDialog) clearTransientPresentation() {
	d.receiptText = ""
	d.inlineFeedback = ""
}

func (d *InnShopDialog) clearAwaitingAction() {
	d.pendingIntent = nil
	d.awaitingRequest = emptyInnActionRequest()
}

func (d *InnShopDialog) transitionTo(phase InnShopDialogPhase) {
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
	d.actionKeysReady = innActionKeysReleased(d.isKeyPressed)
}

func (d *InnShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *InnShopDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if width <= 0 || height <= 0 {
		return
	}
	if d.detailOutput != nil && width == d.lastScreenWidth && height == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth, d.lastScreenHeight = width, height
	d.shell.GetBounds()
	_, d.modeOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: innShopLineSpacing,
		MaxLines: innShopModeHeaderMaxLines, MaxChars: innShopMinimumTextChars,
	})
	d.rowFont, d.rowOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: innShopLineSpacing,
		MaxLines: 1, MaxChars: innShopMinimumTextChars,
	})
	d.detailFont, d.detailOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: innShopLineSpacing,
		MaxLines: innShopMaximumDetailLines, MaxChars: innShopMinimumTextChars,
	})
	d.calculateLayout()
	d.populateOutputs()
}

//nolint:funlen // Rest and roster geometry share one responsive workspace budget and are easiest to verify together.
func (d *InnShopDialog) calculateLayout() {
	if d.detailOutput == nil {
		return
	}
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(innShopMinimumInset, min(workspace.Dx(), workspace.Dy())/innShopInsetDivisor)
	d.workspaceContent = insetRectangle(workspace, inset)
	modeLines := 1
	if d.preview {
		modeLines = innShopModeHeaderMaxLines
	}
	modeHeight := max(1, int(math.Ceil(d.modeOutput.GetTotalHeightForLines(modeLines)))+inset)
	d.modeBounds = image.Rect(
		d.workspaceContent.Min.X, d.workspaceContent.Min.Y,
		d.workspaceContent.Max.X, min(d.workspaceContent.Max.Y, d.workspaceContent.Min.Y+modeHeight),
	)
	bodyTop := d.modeBounds.Max.Y
	if d.mode == InnShopRest || d.helpVisible {
		d.cardBounds = image.Rect(
			d.workspaceContent.Min.X, bodyTop,
			d.workspaceContent.Max.X, d.workspaceContent.Max.Y,
		)
		d.tableBounds = image.Rect(0, 0, 0, 0)
		d.tableHeader = image.Rect(0, 0, 0, 0)
		d.rowBounds = nil
		d.detailBounds = d.cardBounds
		d.configureDetailOutput(inset)

		return
	}

	gap := inset
	rowCount := len(d.state.LeaveRows)
	if d.mode == InnShopPickup {
		rowCount = len(d.state.GuestRows)
	}
	rowCount = min(rowCount, innShopMaximumRows)
	fontHeight := max(1, int(math.Ceil(d.rowOutput.GetFontHeight())))
	naturalRowHeight := fontHeight + inset*innShopDoubleInset
	minimumDetailHeight := min(
		d.workspaceContent.Dy(),
		int(math.Ceil(d.detailOutput.GetTotalHeightForLines(innShopDetailProjectionLines)))+
			inset*innShopDoubleInset,
	)
	tableBudget := max(1, d.workspaceContent.Max.Y-bodyTop-gap-minimumDetailHeight)
	headerHeight := min(naturalRowHeight, tableBudget)
	rowHeight := naturalRowHeight
	if rowCount > 0 {
		rowHeight = max(1, min(naturalRowHeight, (tableBudget-headerHeight)/rowCount))
	}
	d.tableHeader = image.Rect(
		d.workspaceContent.Min.X, bodyTop,
		d.workspaceContent.Max.X, min(d.workspaceContent.Max.Y, bodyTop+headerHeight),
	)
	d.rowBounds = make([]image.Rectangle, rowCount)
	for index := range rowCount {
		top := d.tableHeader.Max.Y + index*rowHeight
		d.rowBounds[index] = image.Rect(
			d.workspaceContent.Min.X, top,
			d.workspaceContent.Max.X, min(d.workspaceContent.Max.Y, top+rowHeight),
		)
	}
	tableBottom := d.tableHeader.Max.Y
	if len(d.rowBounds) > 0 {
		tableBottom = d.rowBounds[len(d.rowBounds)-1].Max.Y
	}
	d.tableBounds = image.Rect(
		d.workspaceContent.Min.X, d.tableHeader.Min.Y,
		d.workspaceContent.Max.X, tableBottom,
	)
	d.detailBounds = image.Rect(
		d.workspaceContent.Min.X, min(d.workspaceContent.Max.Y, d.tableBounds.Max.Y+gap),
		d.workspaceContent.Max.X, d.workspaceContent.Max.Y,
	)
	d.cardBounds = d.detailBounds
	d.configureColumns()
	d.configureDetailOutput(inset)
}

func (d *InnShopDialog) configureColumns() {
	if d.configureMeasuredColumns() {
		return
	}
	ratios := [...]float64{0, 0.30, 0.50, 0.77, 1}
	if d.mode == InnShopLeave {
		ratios = [...]float64{0, 0.30, 0.50, 1, 1}
	}
	for index := range innShopPickupColumns {
		left := d.tableBounds.Min.X + int(math.Round(float64(d.tableBounds.Dx())*ratios[index]))
		right := d.tableBounds.Min.X + int(math.Round(float64(d.tableBounds.Dx())*ratios[index+1]))
		d.columns[index] = image.Rect(left, d.tableBounds.Min.Y, right, d.tableBounds.Max.Y)
	}
}

func (d *InnShopDialog) configureMeasuredColumns() bool {
	if d.rowFont == nil || d.tableBounds.Dx() <= 0 {
		return false
	}
	values := [innShopPickupColumns][]string{
		{innShopColumnName},
		{innShopColumnStatus},
		{"MONTHS"},
		{"CHARGE"},
	}
	columnCount := innShopPickupColumns
	if d.mode == InnShopLeave {
		values[2] = []string{"ELIGIBILITY"}
		values[3] = nil
		columnCount = innShopLeaveColumns
		for index := range d.state.LeaveRows {
			row := d.GetLeaveRowText(index)
			for column, value := range row {
				values[column] = append(values[column], value)
			}
		}
	} else {
		for index := range d.state.GuestRows {
			row := d.GetPickupRowText(index)
			for column, value := range row {
				values[column] = append(values[column], value)
			}
		}
	}

	widths := [innShopPickupColumns]int{}
	total := 0
	for column := range columnCount {
		widths[column] = d.measuredColumnWidth(values[column])
		total += widths[column]
	}
	if total > d.tableBounds.Dx() {
		return false
	}
	// Decision explanations receive spare Leave width; names receive spare Pick Up width.
	spareColumn := 0
	if d.mode == InnShopLeave {
		spareColumn = 2
	}
	widths[spareColumn] += d.tableBounds.Dx() - total
	left := d.tableBounds.Min.X
	for column := range columnCount {
		right := left + widths[column]
		d.columns[column] = image.Rect(left, d.tableBounds.Min.Y, right, d.tableBounds.Max.Y)
		left = right
	}
	for column := columnCount; column < innShopPickupColumns; column++ {
		d.columns[column] = image.Rect(left, d.tableBounds.Min.Y, left, d.tableBounds.Max.Y)
	}

	return true
}

func (d *InnShopDialog) measuredColumnWidth(values []string) int {
	width := 1
	for _, value := range values {
		measured, _ := d.rowFont.MeasureString(value)
		width = max(width, int(math.Ceil(measured)))
	}

	return width + max(1, d.pixelUnit()*innShopCellInsetUnits)*innShopDoubleInset
}

func (d *InnShopDialog) configureDetailOutput(inset int) {
	d.detailText = insetRectangle(d.detailBounds, inset)
	d.detailOutput = text.NewOutput(
		d.detailFont,
		d.detailOutput.GetLineSpacing(),
		innShopMaximumDetailLines,
		max(innShopMinimumTextChars, d.detailFont.CharsForWidth(max(1, d.detailText.Dx()))),
	)
	d.modeOutput.SetMaxCharsPerLine(max(
		innShopMinimumTextChars,
		d.modeOutput.Font.CharsForWidth(max(1, d.modeBounds.Dx())),
	))
}

func (d *InnShopDialog) drawRestCard(screen *ebiten.Image) {
	drawMerchantPane(screen, d.cardBounds, d.pixelUnit())
	d.detailOutput.DrawPanelContent(
		screen,
		d.detailText.Min,
		max(1, d.detailText.Dx()),
		etext.AlignStart,
	)
}

func (d *InnShopDialog) drawRosterTable(screen *ebiten.Image) {
	drawMerchantPane(screen, d.tableBounds, d.pixelUnit())
	headings := []string{innShopColumnName, innShopColumnStatus, "ELIGIBILITY"}
	if d.mode == InnShopPickup {
		headings = []string{innShopColumnName, innShopColumnStatus, "MONTHS", "CHARGE"}
	}
	for index, heading := range headings {
		d.drawCell(screen, d.tableHeader, d.columns[index], heading, DialogLabelColor, etext.AlignCenter)
	}
	vector.StrokeLine(
		screen,
		float32(d.tableHeader.Min.X), float32(d.tableHeader.Max.Y),
		float32(d.tableHeader.Max.X), float32(d.tableHeader.Max.Y),
		float32(d.pixelUnit()), DialogRuleColor, false,
	)
	for index, bounds := range d.rowBounds {
		selected := index == d.GetSelectedIndex() && d.phase == InnShopBrowse
		if selected {
			fillSelectedRow(screen, bounds)
		}
		if d.mode == InnShopLeave {
			values := d.GetLeaveRowText(index)
			color := innLeaveRowColor(d.state.LeaveRows[index], selected)
			for column, value := range values {
				d.drawCell(screen, bounds, d.columns[column], value, color, etext.AlignStart)
			}
		} else {
			values := d.GetPickupRowText(index)
			color := innPickupRowColor(d.state.GuestRows[index], selected)
			for column, value := range values {
				alignment := etext.AlignStart
				if column >= innShopPickupNumericColumn {
					alignment = etext.AlignCenter
				}
				d.drawCell(screen, bounds, d.columns[column], value, color, alignment)
			}
		}
		if index < len(d.rowBounds)-1 {
			vector.StrokeLine(
				screen,
				float32(bounds.Min.X), float32(bounds.Max.Y),
				float32(bounds.Max.X), float32(bounds.Max.Y),
				float32(d.pixelUnit()), DialogRuleLightColor, false,
			)
		}
	}
}

func (d *InnShopDialog) drawDetailCard(screen *ebiten.Image) {
	drawMerchantPane(screen, d.detailBounds, d.pixelUnit())
	d.detailOutput.DrawPanelContent(
		screen,
		d.detailText.Min,
		max(1, d.detailText.Dx()),
		etext.AlignStart,
	)
}

func (d *InnShopDialog) drawCell(
	screen *ebiten.Image,
	row image.Rectangle,
	column image.Rectangle,
	value string,
	color stdcolor.Color,
	alignment etext.Align,
) {
	inset := max(1, d.pixelUnit()*innShopCellInsetUnits)
	drawMerchantTableCell(screen, d.rowOutput, row, column, inset, value, color, alignment)
}

func (d *InnShopDialog) populateOutputs() {
	d.populateModeOutput()
	d.populateDetailOutput()
}

func (d *InnShopDialog) populateModeOutput() {
	if d.modeOutput == nil {
		return
	}
	d.modeOutput.Reset()
	if d.preview {
		d.modeOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text("PREVIEW", DialogStatusWarningColor).Done()
	}
	colors := d.modeTabColors()
	d.modeOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text("[R]est", colors[InnShopRest]).
		Text("    [L]eave", colors[InnShopLeave]).
		Text("    [P]ick Up", colors[InnShopPickup]).Done()
}

func (d *InnShopDialog) populateDetailOutput() {
	if d.detailOutput == nil {
		return
	}
	d.detailOutput.Reset()
	if d.helpVisible {
		d.populateHelpDetail()

		return
	}
	if d.receiptText != "" {
		if d.preview {
			d.detailOutput.AppendLine().Text("PREVIEW", DialogStatusWarningColor).Done()
		}
		d.detailOutput.AppendLine().Text(d.receiptText, DialogValueColor).WordWrap().Done()

		return
	}
	switch d.mode {
	case InnShopRest:
		d.populateRestDetail()
	case InnShopLeave:
		d.populateLeaveDetail()
	case InnShopPickup:
		d.populatePickupDetail()
	default:
		panic("InnShopDialog.populateDetailOutput: unknown mode")
	}
}

func (d *InnShopDialog) populateRestDetail() {
	quote := d.state.Rest
	d.detailOutput.AppendLine().Text("REST", DialogHeaderColor).Done()
	d.detailOutput.AppendLine().LightGray("Nightly charge: ").Yellowf("%d gold", quote.Charge).Done()
	if !quote.Affordable {
		d.detailOutput.AppendLine().Textf(
			"Gold %d; shortfall %d.", DialogStatusWarningColor,
			quote.GoldBefore, int(quote.Charge)-int(quote.GoldBefore),
		).Done()
	}
	d.detailOutput.AppendLine().LightGray("Wake: ").LightCyan(innWakeText(quote)).Done()
	d.detailOutput.AppendLine().White("Living party members fully recover HP and class MP.").Done()
	d.appendPoisonWarnings(quote.MemberProjections)
	if quote.BlockReason != game_state.InnRestBlockNone {
		d.detailOutput.AppendLine().Text(innRestBlockedText(quote.BlockReason), DialogStatusWarningColor).
			WordWrap().Done()
	}
	d.appendInlineFeedback()
}

func (d *InnShopDialog) populateLeaveDetail() {
	if len(d.state.LeaveRows) == 0 {
		d.detailOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text("No active party members are available.", DialogEmptyColor).Done()
		d.appendInlineFeedback()

		return
	}
	row := d.state.LeaveRows[d.selectedLeave]
	d.detailOutput.AppendLine().Text("LEAVE: "+row.Identity.Name, DialogHeaderColor).Done()
	d.detailOutput.AppendLine().LightGray("Status: ").White(innStatusName(row.Status)).Done()
	d.detailOutput.AppendLine().LightGreen("No charge today.").Done()
	if rate, ok := trustedInnMonthlyRate(d.state); ok {
		d.detailOutput.AppendLine().LightGray("Monthly rate: ").Yellowf("%d gold", rate).Done()
	}
	switch {
	case !row.Eligible:
		d.detailOutput.AppendLine().Text(innLeaveBlockedText(row), DialogStatusWarningColor).WordWrap().Done()
	case innHasWarning(row.Warnings, game_state.InnWarningLastLivingMember):
		d.detailOutput.AppendLine().Text(
			"SEVERE: this is the last living member. Leave remains confirmable.",
			DialogStatusWarningColor,
		).WordWrap().Done()
	default:
		d.detailOutput.AppendLine().Text("Eligible. ", DialogDeltaPositive).
			Text("Enter", DialogControlKeyColor).Text(" to review Leave.", DialogControlHintColor).Done()
	}
	d.appendInlineFeedback()
}

func (d *InnShopDialog) populatePickupDetail() {
	if len(d.state.GuestRows) == 0 {
		d.detailOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text("No guests are registered at this Inn.", DialogEmptyColor).Done()
		d.appendInlineFeedback()

		return
	}
	row := d.state.GuestRows[d.selectedPickup]
	d.detailOutput.AppendLine().Text("PICK UP: "+row.Identity.Name, DialogHeaderColor).Done()
	d.detailOutput.AppendLine().LightGray("Status: ").White(innStatusName(row.Status)).Done()
	d.detailOutput.AppendLine().LightGray("Recorded / billable months: ").Whitef(
		"%d / %d", row.MonthsAtInn, row.BilledMonths,
	).Done()
	if innHasWarning(row.Warnings, game_state.InnWarningMinimumMonthCharge) {
		d.detailOutput.AppendLine().Text("Minimum charge is one month.", DialogLabelColor).Done()
	}
	d.detailOutput.AppendLine().LightGray("Monthly rate: ").Yellowf("%d gold", row.MonthlyRate).Done()
	d.detailOutput.AppendLine().LightGray("Listed charge: ").Yellowf("%d gold", row.Charge).Done()
	if !row.Affordable {
		d.detailOutput.AppendLine().Textf(
			"Gold %d; shortfall %d.", DialogStatusWarningColor,
			row.GoldBefore, int(row.Charge)-int(row.GoldBefore),
		).Done()
	}
	if !row.Confirmable || row.BlockReason != game_state.InnPickupBlockNone {
		d.detailOutput.AppendLine().Text(innPickupBlockedText(row), DialogStatusWarningColor).WordWrap().Done()
	} else {
		if !row.Affordable {
			d.detailOutput.AppendLine().Text(
				"GUARD WARNING: confirmation with insufficient gold activates guards.",
				DialogStatusWarningColor,
			).WordWrap().Done()
		}
		if innHasWarning(row.Warnings, game_state.InnWarningPoisonDeath) {
			d.detailOutput.AppendLine().Text(
				row.Identity.Name+" is Poisoned and WILL DIE when picked up.",
				DialogStatusWarningColor,
			).WordWrap().Done()
		}
	}
	d.appendInlineFeedback()
}

func (d *InnShopDialog) populateHelpDetail() {
	d.detailOutput.AppendLine().Text("INN HELP", DialogHeaderColor).Done()
	d.detailOutput.AppendLine().
		Text("[L]", DialogControlKeyColor).Text("eave a companion", DialogLabelColor).Done()
	d.detailOutput.AppendLine().White("Leaving costs nothing today and stores the companion at this Inn.").
		WordWrap().Done()
	d.detailOutput.AppendLine().White(
		"At least one month is billed. The rate is recalculated when the companion is picked up.",
	).
		WordWrap().Done()
	d.detailOutput.AppendLine().
		Text("[P]", DialogControlKeyColor).Text("ick Up a guest", DialogLabelColor).Done()
	d.detailOutput.AppendLine().White("Pick Up pays the listed stay charge and returns the guest to the active party.").
		WordWrap().Done()
	d.detailOutput.AppendLine().White("Dead guests remain stored. The active party must have room.").WordWrap().Done()
	d.detailOutput.AppendLine().Text(
		"Confirming without enough gold activates guards.", DialogStatusWarningColor,
	).WordWrap().Done()
}

func (d *InnShopDialog) appendPoisonWarnings(projections []game_state.InnCharacterProjection) {
	names := innPoisonedNames(projections)
	if len(names) == 0 {
		return
	}
	d.detailOutput.AppendLine().Text("Poisoned: "+strings.Join(names, ", "), DialogStatusWarningColor).
		WordWrap().Done()
	d.detailOutput.AppendLine().Text("WILL DIE during Rest.", DialogStatusWarningColor).Done()
}

func (d *InnShopDialog) appendInlineFeedback() {
	if d.inlineFeedback != "" {
		d.detailOutput.AppendLine().Text(d.inlineFeedback, DialogStatusWarningColor).WordWrap().Done()
	}
}

func (d *InnShopDialog) modeTabColors() [innShopModeCount]stdcolor.Color {
	colors := [innShopModeCount]stdcolor.Color{DialogEmptyColor, DialogEmptyColor, DialogEmptyColor}
	colors[d.mode] = DialogLabelColor

	return colors
}

func (d *InnShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateOutputs()
}

func (d *InnShopDialog) footerGuidance() string {
	if d.helpVisible {
		return "[H] Close Help   [Esc] Back"
	}
	switch d.phase {
	case InnShopBrowse:
		if d.mode == InnShopRest {
			return "[H] Help\n[Enter] Review Rest   [Esc] Leave"
		}

		return "[H] Help\n[Up/Down] Select   [Enter] Review   [Esc] Leave"
	case InnShopAwaitingApproval:
		return "Complete or cancel the Inn action dialog"
	case InnShopAwaitingResult:
		return "Processing Inn action"
	case InnShopSessionError:
		return merchantLeaveFooter
	default:
		panic("InnShopDialog.footerGuidance: unknown phase")
	}
}

func (d *InnShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func innRestRequest(state game_state.InnShopState) InnActionRequest {
	request := emptyInnActionRequest()
	request.Action = InnActionRest
	request.Rest = game_state.InnRestRequest{
		ReviewedMerchantID:            state.Merchant.ID,
		ReviewedPricingCharacterIndex: state.PricingCharacterIndex,
		ReviewedRosterFingerprint:     state.RosterFingerprint,
		ReviewedCharge:                state.Rest.Charge,
		ReviewedGoldBefore:            state.Rest.GoldBefore,
		ReviewedDateTimeBefore:        state.Rest.DateTimeBefore,
		ReviewedDateTimeAfter:         state.Rest.DateTimeAfter,
	}

	return request
}

func innLeaveRequest(state game_state.InnShopState, row game_state.InnLeaveRow) InnActionRequest {
	request := emptyInnActionRequest()
	request.Action = InnActionLeave
	request.Leave = game_state.InnLeaveRequest{
		ReviewedMerchantID:            state.Merchant.ID,
		ReviewedPricingCharacterIndex: state.PricingCharacterIndex,
		ReviewedRosterFingerprint:     state.RosterFingerprint,
		Character:                     row.Identity,
		ReviewedCharacterIndex:        row.RosterIndex,
	}

	return request
}

func innPickupRequest(state game_state.InnShopState, row game_state.InnGuestRow) InnActionRequest {
	request := emptyInnActionRequest()
	request.Action = InnActionPickup
	request.Pickup = game_state.InnPickupRequest{
		ReviewedMerchantID:            state.Merchant.ID,
		ReviewedPricingCharacterIndex: state.PricingCharacterIndex,
		ReviewedRosterFingerprint:     state.RosterFingerprint,
		Character:                     row.Identity,
		ReviewedCharacterIndex:        row.RosterIndex,
		ReviewedCharge:                row.Charge,
		ReviewedGoldBefore:            row.GoldBefore,
	}

	return request
}

func emptyInnActionRequest() InnActionRequest {
	return InnActionRequest{
		Action: InnActionRest,
		Rest:   game_state.InnRestRequest{},   //nolint:exhaustruct_v5 // Inactive typed-union branch.
		Leave:  game_state.InnLeaveRequest{},  //nolint:exhaustruct_v5 // Inactive typed-union branch.
		Pickup: game_state.InnPickupRequest{}, //nolint:exhaustruct_v5 // Inactive typed-union branch.
	}
}

func emptyInnActionIntent() InnActionIntent {
	return InnActionIntent{
		Action:         InnActionRest,
		Request:        emptyInnActionRequest(),
		Rest:           emptyInnRestQuote(),
		Leave:          emptyInnLeaveRow(),
		Pickup:         emptyInnGuestRow(),
		InnName:        "",
		RoomOccupancy:  0,
		RoomCapacity:   0,
		PartySize:      0,
		PartyCapacity:  0,
		MonthlyRate:    0,
		HasMonthlyRate: false,
		Preview:        false,
	}
}

func emptyInnCharacterIdentity() game_state.InnCharacterIdentity {
	return game_state.InnCharacterIdentity{Name: "", Gender: 0, Class: 0}
}

func emptyInnRestQuote() game_state.InnRestQuote {
	return game_state.InnRestQuote{} //nolint:exhaustruct_v5 // Empty typed-union branch and consume result.
}

func emptyInnLeaveRow() game_state.InnLeaveRow {
	return game_state.InnLeaveRow{} //nolint:exhaustruct_v5 // Empty typed-union branch and consume result.
}

func emptyInnGuestRow() game_state.InnGuestRow {
	return game_state.InnGuestRow{} //nolint:exhaustruct_v5 // Empty typed-union branch and consume result.
}

func cloneInnActionIntent(intent InnActionIntent) InnActionIntent {
	clone := intent
	clone.Rest = cloneInnRestQuote(intent.Rest)
	clone.Leave = cloneInnLeaveRow(intent.Leave)
	clone.Pickup = cloneInnGuestRow(intent.Pickup)

	return clone
}

func cloneInnShopState(state game_state.InnShopState) game_state.InnShopState {
	clone := state
	clone.Rest = cloneInnRestQuote(state.Rest)
	clone.LeaveRows = make([]game_state.InnLeaveRow, len(state.LeaveRows))
	for index, row := range state.LeaveRows {
		clone.LeaveRows[index] = cloneInnLeaveRow(row)
	}
	clone.GuestRows = make([]game_state.InnGuestRow, len(state.GuestRows))
	for index, row := range state.GuestRows {
		clone.GuestRows[index] = cloneInnGuestRow(row)
	}

	return clone
}

func cloneInnRestQuote(quote game_state.InnRestQuote) game_state.InnRestQuote {
	clone := quote
	clone.TimeStepsMinutes = append([]int(nil), quote.TimeStepsMinutes...)
	clone.Warnings = append([]game_state.InnWarning(nil), quote.Warnings...)
	clone.MemberProjections = make([]game_state.InnCharacterProjection, len(quote.MemberProjections))
	for index, projection := range quote.MemberProjections {
		clone.MemberProjections[index] = cloneInnProjection(projection)
	}

	return clone
}

func cloneInnLeaveRow(row game_state.InnLeaveRow) game_state.InnLeaveRow {
	clone := row
	clone.Warnings = append([]game_state.InnWarning(nil), row.Warnings...)
	clone.Projection = cloneInnProjection(row.Projection)

	return clone
}

func cloneInnGuestRow(row game_state.InnGuestRow) game_state.InnGuestRow {
	clone := row
	clone.Warnings = append([]game_state.InnWarning(nil), row.Warnings...)
	clone.Projection = cloneInnProjection(row.Projection)

	return clone
}

func cloneInnProjection(projection game_state.InnCharacterProjection) game_state.InnCharacterProjection {
	clone := projection
	clone.Warnings = append([]game_state.InnWarning(nil), projection.Warnings...)

	return clone
}

func validatedInnShopState(state game_state.InnShopState) game_state.InnShopState {
	validateInnShopState(state)

	return cloneInnShopState(state)
}

//nolint:cyclop // This validates one cohesive detached authority snapshot at the presentation boundary.
func validateInnShopState(state game_state.InnShopState) {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyInn ||
		strings.TrimSpace(state.Merchant.OwnerName) == "" || strings.TrimSpace(state.Merchant.ShopName) == "" {
		panic("InnShopDialog: complete detached Inn merchant is required")
	}
	if state.PricingCharacterIndex < 0 || strings.TrimSpace(state.PricingCharacterName) == "" ||
		strings.TrimSpace(state.RosterFingerprint) == "" {
		panic("InnShopDialog: complete detached Inn review identity is required")
	}
	if state.RoomCapacity <= 0 || state.RoomOccupancy < 0 || state.RoomOccupancy > state.RoomCapacity ||
		state.PartyCapacity <= 0 || state.PartySize <= 0 || state.PartySize > state.PartyCapacity {
		panic("InnShopDialog: invalid detached party or room capacity")
	}
	if len(state.LeaveRows) != state.PartySize || len(state.GuestRows) != state.RoomOccupancy {
		panic("InnShopDialog: detached Inn rows do not match party or occupancy")
	}
	validateInnRestQuote(state)
	validateInnLeaveRows(state)
	validateInnGuestRows(state)
}

func validateInnRestQuote(state game_state.InnShopState) {
	quote := state.Rest
	if quote.Charge == 0 || quote.GoldBefore != state.Gold || len(quote.MemberProjections) != state.PartySize {
		panic("InnShopDialog: invalid detached Rest quote")
	}
	wantAffordable := quote.Charge <= quote.GoldBefore
	wantAfter := quote.GoldBefore
	if wantAffordable {
		wantAfter -= quote.Charge
	}
	if quote.Affordable != wantAffordable || quote.GoldAfterPayment != wantAfter ||
		quote.BlockReason > game_state.InnRestBlockNoVacancy {
		panic("InnShopDialog: inconsistent detached Rest consequence")
	}
	for _, projection := range quote.MemberProjections {
		validateInnIdentity(projection.Identity)
		validateInnStatus(projection.StatusBefore)
		validateInnStatus(projection.StatusAfter)
		validateInnWarnings(projection.Warnings)
	}
	validateInnWarnings(quote.Warnings)
}

func validateInnLeaveRows(state game_state.InnShopState) {
	seen := make(map[game_state.InnCharacterIdentity]struct{}, len(state.LeaveRows))
	for _, row := range state.LeaveRows {
		validateInnIdentity(row.Identity)
		validateInnStatus(row.Status)
		validateInnWarnings(row.Warnings)
		if _, exists := seen[row.Identity]; exists {
			panic("InnShopDialog: duplicate Leave identity")
		}
		seen[row.Identity] = struct{}{}
		if row.Eligible != (row.BlockReason == game_state.InnLeaveBlockNone) ||
			row.BlockReason > game_state.InnLeaveBlockNoVacancy || row.Projection.Identity != row.Identity {
			panic("InnShopDialog: inconsistent detached Leave row")
		}
	}
}

func validateInnGuestRows(state game_state.InnShopState) {
	seen := make(map[game_state.InnCharacterIdentity]struct{}, len(state.GuestRows))
	for _, row := range state.GuestRows {
		validateInnIdentity(row.Identity)
		validateInnStatus(row.Status)
		validateInnWarnings(row.Warnings)
		if _, exists := seen[row.Identity]; exists {
			panic("InnShopDialog: duplicate Pick Up identity")
		}
		seen[row.Identity] = struct{}{}
		if row.BilledMonths < 1 || row.MonthlyRate == 0 || row.Charge == 0 || row.GoldBefore != state.Gold ||
			row.BlockReason > game_state.InnPickupBlockPartyFull || row.Projection.Identity != row.Identity {
			panic("InnShopDialog: invalid detached Pick Up row")
		}
		if row.Confirmable != (row.BlockReason == game_state.InnPickupBlockNone) {
			panic("InnShopDialog: inconsistent Pick Up confirmation gate")
		}
		wantAfter := row.GoldBefore
		if row.Affordable {
			wantAfter -= row.Charge
		}
		if row.Affordable != (row.Charge <= row.GoldBefore) || row.GoldAfterPayment != wantAfter {
			panic("InnShopDialog: inconsistent Pick Up gold consequence")
		}
	}
}

func validateInnIdentity(identity game_state.InnCharacterIdentity) {
	if strings.TrimSpace(identity.Name) == "" {
		panic("Inn widgets: character identity name is required")
	}
	switch identity.Gender {
	case party_state.Male, party_state.Female:
	default:
		panic("Inn widgets: known character gender is required")
	}
	switch identity.Class {
	case party_state.Avatar, party_state.Fighter, party_state.Bard, party_state.Wizard:
	default:
		panic("Inn widgets: known character class is required")
	}
}

func validateInnStatus(status party_state.CharacterStatus) {
	switch status {
	case party_state.Good, party_state.Poisoned, party_state.Charmed, party_state.Sleep, party_state.Dead:
	default:
		panic("Inn widgets: known character status is required")
	}
}

func validateInnWarnings(warnings []game_state.InnWarning) {
	for _, warning := range warnings {
		if warning < game_state.InnWarningNone ||
			warning > game_state.InnWarningInsufficientFundsActivatesGuards {
			panic("Inn widgets: unknown warning")
		}
	}
}

func validateInnShopProse(prose InnShopDialogProse) {
	validateInnFinalizedText("greeting", prose.Greeting)
	validateInnFinalizedText("Rest prompt", prose.RestPrompt)
	validateInnFinalizedText("Leave prompt", prose.LeavePrompt)
	validateInnFinalizedText("Pick Up prompt", prose.PickupPrompt)
}

func validateInnFinalizedText(label, value string) {
	if strings.TrimSpace(value) == "" {
		panic("Inn widgets: finalized " + label + " is required")
	}
}

func validateInnResultDisplay(
	request InnActionRequest,
	refreshed game_state.InnShopState,
	display InnActionResultDisplay,
) {
	wantCharge := uint16(0)
	switch request.Action {
	case InnActionRest:
		wantCharge = request.Rest.ReviewedCharge
	case InnActionLeave:
	case InnActionPickup:
		wantCharge = request.Pickup.ReviewedCharge
	default:
		panic("InnShopDialog.ApplyResult: unknown result action")
	}
	if display.ListedCharge != wantCharge || display.FinalGold != refreshed.Gold ||
		display.PartySize != refreshed.PartySize || display.RoomOccupancy != refreshed.RoomOccupancy {
		panic("InnShopDialog.ApplyResult: receipt values do not match refreshed authority")
	}
}

func innLeaveRowColor(row game_state.InnLeaveRow, selected bool) stdcolor.Color {
	if selected {
		return DialogSelectedText
	}
	if !row.Eligible {
		return DialogEmptyColor
	}
	if innHasWarning(row.Warnings, game_state.InnWarningLastLivingMember) {
		return DialogLabelColor
	}

	return DialogDeltaPositive
}

func innPickupRowColor(row game_state.InnGuestRow, selected bool) stdcolor.Color {
	if selected {
		return DialogSelectedText
	}
	if !row.Confirmable || row.BlockReason != game_state.InnPickupBlockNone {
		return DialogEmptyColor
	}
	if !row.Affordable || innHasWarning(row.Warnings, game_state.InnWarningPoisonDeath) {
		return DialogLabelColor
	}

	return DialogDeltaPositive
}

func innStatusName(status party_state.CharacterStatus) string {
	switch status {
	case party_state.Good:
		return "Good"
	case party_state.Poisoned:
		return "Poisoned"
	case party_state.Charmed:
		return "Charmed"
	case party_state.Sleep:
		return "Sleeping"
	case party_state.Dead:
		return innStatusDead
	default:
		return innStatusUnknown
	}
}

func innLeaveBlockLabel(reason game_state.InnLeaveBlockReason) string {
	switch reason {
	case game_state.InnLeaveBlockAvatar:
		return "Avatar stays"
	case game_state.InnLeaveBlockDead:
		return innStatusDead
	case game_state.InnLeaveBlockMinimumParty:
		return "Minimum party"
	case game_state.InnLeaveBlockNoVacancy:
		return "No vacancy"
	case game_state.InnLeaveBlockNone:
		return "Eligible"
	default:
		return "Blocked"
	}
}

func innRestBlockedText(reason game_state.InnRestBlockReason) string {
	switch reason {
	case game_state.InnRestBlockNoVacancy:
		return "BLOCKED: no guest room is vacant. Pick Up a guest before Rest."
	case game_state.InnRestBlockNone:
		return ""
	default:
		return "BLOCKED: Rest is unavailable."
	}
}

func innLeaveBlockedText(row game_state.InnLeaveRow) string {
	switch row.BlockReason {
	case game_state.InnLeaveBlockAvatar:
		return row.Identity.Name + " is the Avatar and must remain in the active party."
	case game_state.InnLeaveBlockDead:
		return row.Identity.Name + " is Dead and cannot be left at the Inn."
	case game_state.InnLeaveBlockMinimumParty:
		return "At least one active party member must remain."
	case game_state.InnLeaveBlockNoVacancy:
		return "No guest room is vacant. Pick Up a guest before leaving a companion."
	case game_state.InnLeaveBlockNone:
		return ""
	default:
		return "This party member cannot be left at the Inn."
	}
}

func innPickupBlockedText(row game_state.InnGuestRow) string {
	switch row.BlockReason {
	case game_state.InnPickupBlockDead:
		return row.Identity.Name + " is Dead and remains stored at this Inn."
	case game_state.InnPickupBlockPartyFull:
		return "The active party is full. Leave a companion before Pick Up."
	case game_state.InnPickupBlockNone:
		return ""
	default:
		return "This guest cannot be picked up."
	}
}

func innWakeText(quote game_state.InnRestQuote) string {
	wake := quote.DateTimeAfter

	return fmt.Sprintf(
		"%s on %s, in the next qualifying 6 o'clock hour.",
		strings.TrimSpace(wake.GetTimeAsString()),
		wake.GetDateAsString(),
	)
}

func innPoisonedNames(projections []game_state.InnCharacterProjection) []string {
	names := make([]string, 0, len(projections))
	for _, projection := range projections {
		if innHasWarning(projection.Warnings, game_state.InnWarningPoisonDeath) {
			names = append(names, projection.Identity.Name)
		}
	}

	return names
}

func innHasWarning(warnings []game_state.InnWarning, wanted game_state.InnWarning) bool {
	for _, warning := range warnings {
		if warning == wanted {
			return true
		}
	}

	return false
}

func trustedInnMonthlyRate(state game_state.InnShopState) (uint16, bool) {
	if state.MonthlyRate == 0 {
		return 0, false
	}

	return state.MonthlyRate, true
}

func innLeaveIdentityAt(
	rows []game_state.InnLeaveRow,
	index int,
) (game_state.InnCharacterIdentity, bool) {
	if index < 0 || index >= len(rows) {
		return emptyInnCharacterIdentity(), false
	}

	return rows[index].Identity, true
}

func innPickupIdentityAt(
	rows []game_state.InnGuestRow,
	index int,
) (game_state.InnCharacterIdentity, bool) {
	if index < 0 || index >= len(rows) {
		return emptyInnCharacterIdentity(), false
	}

	return rows[index].Identity, true
}

func preservedInnLeaveIndex(
	rows []game_state.InnLeaveRow,
	identity game_state.InnCharacterIdentity,
	hasIdentity bool,
	fallback int,
) int {
	if hasIdentity {
		for index, row := range rows {
			if row.Identity == identity {
				return index
			}
		}
	}
	if len(rows) == 0 {
		return 0
	}

	return max(0, min(len(rows)-1, fallback))
}

func preservedInnPickupIndex(
	rows []game_state.InnGuestRow,
	identity game_state.InnCharacterIdentity,
	hasIdentity bool,
	fallback int,
) int {
	if hasIdentity {
		for index, row := range rows {
			if row.Identity == identity {
				return index
			}
		}
	}
	if len(rows) == 0 {
		return 0
	}

	return max(0, min(len(rows)-1, fallback))
}

func innActionKeysReleased(isKeyPressed func(ebiten.Key) bool) bool {
	return !isKeyPressed(ebiten.KeyEnter) &&
		!isKeyPressed(ebiten.KeyNumpadEnter) &&
		!isKeyPressed(ebiten.KeyEscape)
}
