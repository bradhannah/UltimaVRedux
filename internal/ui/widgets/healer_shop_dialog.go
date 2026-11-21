package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strconv"

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
	healerShopServiceCount          = 3
	healerShopServiceColumns        = 2
	healerShopMaximumRecipients     = party_state.MaxActiveParty
	healerShopRecipientColumns      = 3
	healerShopServiceWidthRatio     = 0.30
	healerShopMinimumInset          = 4
	healerShopInsetDivisor          = 64
	healerShopCenterDivisor         = 2
	healerShopDoubleInset           = 2
	healerShopCellInsetUnits        = 2
	healerShopLineSpacing           = 1.05
	healerShopMinimumTextChars      = 8
	healerShopDetailLines           = 5
	healerShopServiceLines          = 1
	healerShopWorkspaceVerticalGaps = 1
)

var _ Widget = (*HealerShopDialog)(nil)

// HealerShopFocus identifies the active keyboard pane.
type HealerShopFocus byte

const (
	// HealerShopServices focuses Cure, Heal, and Resurrect.
	HealerShopServices HealerShopFocus = iota
	// HealerShopRecipients focuses active-party members.
	HealerShopRecipients
)

// HealerShopDialogPhase identifies the inline treatment lifecycle.
type HealerShopDialogPhase byte

const (
	// HealerShopBrowse accepts service and recipient navigation.
	HealerShopBrowse HealerShopDialogPhase = iota
	// HealerShopAwaitingApproval waits for the workflow-owned treatment modal.
	HealerShopAwaitingApproval
	// HealerShopAwaitingResult blocks input while authority processes the request.
	HealerShopAwaitingResult
	// HealerShopSessionError displays an unrecoverable session error until dismissal.
	HealerShopSessionError
)

// HealerShopDialogProse contains finalized merchant copy used by the dialog.
type HealerShopDialogProse struct {
	Greeting     string
	NoNeed       string
	FundsRefusal string
}

// HealerShopDialogConfig configures one detached, keyboard-only healer shop.
type HealerShopDialogConfig struct {
	Keyboard       *input.Keyboard
	MerchantSprite *ebiten.Image
	State          game_state.HealerShopState
	Prose          HealerShopDialogProse
	Preview        bool
}

// HealerServiceResultDisplay is finalized workflow text after authority returns.
type HealerServiceResultDisplay struct {
	MerchantExchange string
	Receipt          string
}

// HealerTreatmentIntent describes one detached treatment awaiting modal approval.
type HealerTreatmentIntent struct {
	Request    game_state.HealerServiceRequest
	Service    game_state.HealerServiceQuote
	AllService game_state.HealerAllServiceQuote
	Recipient  *game_state.HealerRecipientState
	Gold       uint16
}

// IsAll reports whether this intent targets every currently eligible recipient.
func (i HealerTreatmentIntent) IsAll() bool {
	return i.Request.CharacterIndex == game_state.HealerAllRecipientsIndex
}

// HealerShopDialog presents a two-pane service and recipient workspace.
type HealerShopDialog struct {
	navigation
	keyboard *input.Keyboard
	state    game_state.HealerShopState
	prose    HealerShopDialogProse
	preview  bool

	focus             HealerShopFocus
	selectedService   int
	selectedRecipient int
	phase             HealerShopDialogPhase
	dismissed         bool
	latestExchange    string
	receiptText       string
	receiptColor      stdcolor.Color
	pendingIntent     *HealerTreatmentIntent
	awaitingRequest   game_state.HealerServiceRequest

	shell        *MerchantDialogShell
	cellOutput   *text.Output
	detailOutput *text.Output
	cellFont     *text.UltimaFont
	detailFont   *text.UltimaFont

	workspaceContent image.Rectangle
	serviceBounds    image.Rectangle
	serviceHeader    image.Rectangle
	serviceRows      [healerShopServiceCount]image.Rectangle
	serviceColumns   [healerShopServiceColumns]image.Rectangle
	recipientBounds  image.Rectangle
	recipientHeader  image.Rectangle
	recipientRows    [healerShopMaximumRecipients]image.Rectangle
	recipientColumns [healerShopRecipientColumns]image.Rectangle
	detailBounds     image.Rectangle
	detailTextBounds image.Rectangle

	lastScreenWidth  int
	lastScreenHeight int
}

// NewHealerShopDialog creates a visible healer dialog from authoritative detached state.
func NewHealerShopDialog(config HealerShopDialogConfig) *HealerShopDialog {
	if config.Keyboard == nil {
		panic("HealerShopDialog: keyboard is required")
	}
	if config.MerchantSprite == nil {
		panic("HealerShopDialog: merchant sprite is required")
	}
	validateHealerShopState(config.State)
	validateHealerShopProse(config.Prose)
	var emptyBounds image.Rectangle

	dialog := &HealerShopDialog{
		navigation:        newNavigation(config.Keyboard.IsKeyHeld),
		keyboard:          config.Keyboard,
		state:             config.State,
		prose:             config.Prose,
		preview:           config.Preview,
		focus:             HealerShopServices,
		selectedService:   0,
		selectedRecipient: 0,
		phase:             HealerShopBrowse,
		dismissed:         false,
		latestExchange:    config.Prose.Greeting,
		receiptText:       "",
		receiptColor:      DialogValueColor,
		pendingIntent:     nil,
		awaitingRequest:   emptyHealerServiceRequest(),
		shell:             nil,
		cellOutput:        nil,
		detailOutput:      nil,
		cellFont:          nil,
		detailFont:        nil,
		workspaceContent:  emptyBounds,
		serviceBounds:     emptyBounds,
		serviceHeader:     emptyBounds,
		serviceRows:       [healerShopServiceCount]image.Rectangle{},
		serviceColumns:    [healerShopServiceColumns]image.Rectangle{},
		recipientBounds:   emptyBounds,
		recipientHeader:   emptyBounds,
		recipientRows:     [healerShopMaximumRecipients]image.Rectangle{},
		recipientColumns:  [healerShopRecipientColumns]image.Rectangle{},
		detailBounds:      emptyBounds,
		detailTextBounds:  emptyBounds,
		lastScreenWidth:   0,
		lastScreenHeight:  0,
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

// Draw renders the shared shell and concrete healer workspace.
func (d *HealerShopDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.shell.DrawBackground(screen)
	d.shell.DrawCommonRegions(screen)
	d.drawServicePane(screen)
	d.drawRecipientPane(screen)
	d.drawDetailPane(screen)
}

// Update processes keyboard-only service and recipient input.
func (d *HealerShopDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()
	if handleMerchantSpeechPaging(d.shell, d.keyboard) {
		return
	}

	switch d.phase {
	case HealerShopBrowse:
		d.updateBrowse()
	case HealerShopAwaitingApproval, HealerShopAwaitingResult:
		return
	case HealerShopSessionError:
		if d.enterPressed() || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
			d.Dismiss()
		}
	default:
		panic("HealerShopDialog.Update: unknown phase")
	}
}

func (d *HealerShopDialog) updateBrowse() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()

		return
	}
	if d.selectServiceShortcut() {
		return
	}
	if d.selectAllShortcut() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) {
		d.focus = HealerShopServices
		d.syncPresentation()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight) {
		d.focus = HealerShopRecipients
		d.syncPresentation()

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
		if d.focus == HealerShopServices {
			d.focus = HealerShopRecipients
			d.receiptText = ""
			d.syncPresentation()

			return
		}
		d.reviewSelection()
	}
}

// ConsumePendingTreatmentIntent returns and clears one modal-approval intent.
func (d *HealerShopDialog) ConsumePendingTreatmentIntent() (HealerTreatmentIntent, bool) {
	if d.pendingIntent == nil {
		return emptyHealerTreatmentIntent(), false
	}
	intent := *d.pendingIntent
	d.pendingIntent = nil

	return intent, true
}

// CancelTreatmentApproval returns to browsing after the nested modal is cancelled.
func (d *HealerShopDialog) CancelTreatmentApproval() {
	if d.phase != HealerShopAwaitingApproval {
		panic("HealerShopDialog.CancelTreatmentApproval: dialog is not awaiting approval")
	}
	d.clearAwaitingRequest()
	d.transitionTo(HealerShopBrowse)
	d.syncPresentation()
}

// AcceptTreatmentApproval advances an approved modal request to authority.
func (d *HealerShopDialog) AcceptTreatmentApproval(request game_state.HealerServiceRequest) {
	if d.phase != HealerShopAwaitingApproval || request != d.awaitingRequest {
		panic("HealerShopDialog.AcceptTreatmentApproval: request does not match pending approval")
	}
	d.transitionTo(HealerShopAwaitingResult)
	d.syncPresentation()
}

// ApplyServiceResult applies an authoritative result, refreshed state, and finalized text.
func (d *HealerShopDialog) ApplyServiceResult(
	result game_state.HealerServiceResult,
	refreshed game_state.HealerShopState,
	display HealerServiceResultDisplay,
) {
	if d.phase != HealerShopAwaitingResult {
		panic("HealerShopDialog.ApplyServiceResult: dialog is not awaiting a result")
	}
	if result.Service != d.awaitingRequest.Service || result.CharacterIndex != d.awaitingRequest.CharacterIndex {
		panic("HealerShopDialog.ApplyServiceResult: result does not match the accepted request")
	}
	d.applyRefreshedState(refreshed, d.awaitingRequest)
	validateFinalizedMerchantText("healer result exchange", display.MerchantExchange)
	validateFinalizedMerchantText("healer result receipt", display.Receipt)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.receiptColor = DialogDeltaPositive
	d.clearAwaitingRequest()
	d.transitionTo(HealerShopBrowse)
	d.syncPresentation()
}

// ApplyAllServiceResult applies one authoritative aggregate result and refreshed state.
func (d *HealerShopDialog) ApplyAllServiceResult(
	result game_state.HealerAllServiceResult,
	refreshed game_state.HealerShopState,
	display HealerServiceResultDisplay,
) {
	if d.phase != HealerShopAwaitingResult {
		panic("HealerShopDialog.ApplyAllServiceResult: dialog is not awaiting a result")
	}
	if result.Service != d.awaitingRequest.Service ||
		d.awaitingRequest.CharacterIndex != game_state.HealerAllRecipientsIndex {
		panic("HealerShopDialog.ApplyAllServiceResult: result does not match the accepted request")
	}
	count := d.state.AllServices[result.Service].RecipientCount
	if len(result.CharacterIndexes) < count {
		display.Receipt += fmt.Sprintf(" Batch stopped: %d of %d treated.", len(result.CharacterIndexes), count)
	}
	d.applyRefreshedState(refreshed, d.awaitingRequest)
	validateFinalizedMerchantText("aggregate healer result exchange", display.MerchantExchange)
	validateFinalizedMerchantText("aggregate healer result receipt", display.Receipt)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.receiptColor = DialogDeltaPositive
	d.clearAwaitingRequest()
	d.transitionTo(HealerShopBrowse)
	d.syncPresentation()
}

// ApplyRefusal returns to Browse with refreshed state and finalized refusal text.
func (d *HealerShopDialog) ApplyRefusal(
	refreshed game_state.HealerShopState,
	request game_state.HealerServiceRequest,
	display HealerServiceResultDisplay,
) {
	if d.phase != HealerShopAwaitingResult || request != d.awaitingRequest {
		panic("HealerShopDialog.ApplyRefusal: refusal does not match the accepted request")
	}
	d.applyRefreshedState(refreshed, request)
	validateFinalizedMerchantText("healer refusal exchange", display.MerchantExchange)
	validateFinalizedMerchantText("healer refusal receipt", display.Receipt)
	d.latestExchange = display.MerchantExchange
	d.receiptText = display.Receipt
	d.receiptColor = DialogLabelColor
	d.clearAwaitingRequest()
	d.transitionTo(HealerShopBrowse)
	d.syncPresentation()
}

// ApplyPreviewStopped returns to Browse without committing the accepted request.
func (d *HealerShopDialog) ApplyPreviewStopped(request game_state.HealerServiceRequest) {
	if d.phase != HealerShopAwaitingResult || request != d.awaitingRequest {
		panic("HealerShopDialog.ApplyPreviewStopped: request does not match the accepted preview")
	}
	d.latestExchange = "Preview stopped before healer authority."
	d.receiptText = "Preview only: no treatment, payment, theft, or character change was committed."
	d.receiptColor = DialogControlHintColor
	d.clearAwaitingRequest()
	d.transitionTo(HealerShopBrowse)
	d.syncPresentation()
}

// ApplySessionError replaces the exchange and disables further treatments.
func (d *HealerShopDialog) ApplySessionError(message string) {
	validateFinalizedMerchantText("healer session error", message)
	d.pendingIntent = nil
	d.clearAwaitingRequest()
	d.latestExchange = message
	d.receiptText = ""
	d.transitionTo(HealerShopSessionError)
	d.syncPresentation()
}

// GetSelectedService returns the current stable service identity.
func (d *HealerShopDialog) GetSelectedService() references.HealerService {
	return d.state.Services[d.selectedService].Service
}

// GetSelectedRecipient returns the current detached active-party row.
func (d *HealerShopDialog) GetSelectedRecipient() game_state.HealerRecipientState {
	return d.state.Recipients[d.selectedRecipient]
}

// GetShell returns the shared merchant presentation shell.
func (d *HealerShopDialog) GetShell() *MerchantDialogShell { return d.shell }

// IsDismissed reports whether the dialog may be removed.
func (d *HealerShopDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the healer without producing a treatment request.
func (d *HealerShopDialog) Dismiss() { d.dismissed = true }

func (d *HealerShopDialog) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if d.cellOutput != nil && screenWidth == d.lastScreenWidth && screenHeight == d.lastScreenHeight {
		return
	}
	d.lastScreenWidth = screenWidth
	d.lastScreenHeight = screenHeight
	d.shell.GetBounds()

	d.cellFont, d.cellOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacingMultiplier: healerShopLineSpacing,
		MaxLines:              healerShopServiceLines,
		MaxChars:              healerShopMinimumTextChars,
	})
	d.detailFont, d.detailOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacingMultiplier: healerShopLineSpacing,
		MaxLines:              healerShopDetailLines,
		MaxChars:              healerShopMinimumTextChars,
	})
	d.calculateLayout()
	d.populateDetailOutput()
}

//nolint:funlen // The shared column and pane geometry is measured together before publication.
func (d *HealerShopDialog) calculateLayout() {
	workspace := d.shell.GetWorkspaceBounds()
	inset := max(healerShopMinimumInset, min(workspace.Dx(), workspace.Dy())/healerShopInsetDivisor)
	d.workspaceContent = insetRectangle(workspace, inset)
	gap := max(inset, d.pixelUnit()*healerShopDoubleInset)
	fontHeight := max(1, int(math.Ceil(d.cellOutput.GetFontHeight())))
	naturalHeaderHeight := fontHeight + inset*healerShopDoubleInset
	naturalRowHeight := fontHeight + max(inset, d.pixelUnit()*healerShopDoubleInset)
	rowSlots := max(healerShopServiceCount, min(len(d.state.Recipients), healerShopMaximumRecipients))
	minimumDetailHeight := int(math.Ceil(d.detailOutput.GetTotalHeightForLines(healerShopDetailLines))) +
		inset*healerShopDoubleInset
	maximumPaneHeight := max(
		rowSlots+1,
		d.workspaceContent.Dy()-minimumDetailHeight-gap*healerShopWorkspaceVerticalGaps,
	)
	paneHeight := min(naturalHeaderHeight+naturalRowHeight*rowSlots, maximumPaneHeight)
	headerHeight := min(naturalHeaderHeight, max(1, paneHeight-rowSlots))
	rowHeight := max(1, (paneHeight-headerHeight)/rowSlots)
	paneHeight = headerHeight + rowHeight*rowSlots
	panes := image.Rect(
		d.workspaceContent.Min.X,
		d.workspaceContent.Min.Y,
		d.workspaceContent.Max.X,
		min(d.workspaceContent.Max.Y, d.workspaceContent.Min.Y+paneHeight),
	)
	serviceWidth := max(1, int(math.Round(float64(panes.Dx())*healerShopServiceWidthRatio)))
	d.serviceBounds = image.Rect(panes.Min.X, panes.Min.Y, panes.Min.X+serviceWidth, panes.Max.Y)
	d.recipientBounds = image.Rect(min(panes.Max.X, d.serviceBounds.Max.X+gap), panes.Min.Y, panes.Max.X, panes.Max.Y)
	d.detailBounds = image.Rect(
		d.workspaceContent.Min.X,
		min(d.workspaceContent.Max.Y, panes.Max.Y+gap),
		d.workspaceContent.Max.X,
		d.workspaceContent.Max.Y,
	)
	d.detailTextBounds = insetRectangle(d.detailBounds, inset)

	d.serviceHeader = image.Rect(
		d.serviceBounds.Min.X,
		d.serviceBounds.Min.Y,
		d.serviceBounds.Max.X,
		d.serviceBounds.Min.Y+headerHeight,
	)
	d.recipientHeader = image.Rect(
		d.recipientBounds.Min.X,
		d.recipientBounds.Min.Y,
		d.recipientBounds.Max.X,
		d.recipientBounds.Min.Y+headerHeight,
	)
	d.recipientRows = [healerShopMaximumRecipients]image.Rectangle{}
	for index := range len(d.state.Recipients) {
		top := d.recipientHeader.Max.Y + index*rowHeight
		d.recipientRows[index] = image.Rect(d.recipientBounds.Min.X, top, d.recipientBounds.Max.X, top+rowHeight)
	}
	for index := range healerShopServiceCount {
		top := d.serviceHeader.Max.Y + index*rowHeight
		d.serviceRows[index] = image.Rect(d.serviceBounds.Min.X, top, d.serviceBounds.Max.X, top+rowHeight)
	}
	d.configureColumns()
	d.detailOutput = text.NewOutput(
		d.detailFont,
		d.detailOutput.GetLineSpacing(),
		max(1, d.detailOutput.GetLineCapacity(max(1, d.detailTextBounds.Dy()))),
		max(healerShopMinimumTextChars, d.detailFont.CharsForWidth(max(1, d.detailTextBounds.Dx()))),
	)
}

func (d *HealerShopDialog) configureColumns() {
	serviceRatios := [...]float64{0, 0.58, 1}
	for index := range healerShopServiceColumns {
		left := d.serviceBounds.Min.X + int(math.Round(float64(d.serviceBounds.Dx())*serviceRatios[index]))
		right := d.serviceBounds.Min.X + int(math.Round(float64(d.serviceBounds.Dx())*serviceRatios[index+1]))
		d.serviceColumns[index] = image.Rect(left, d.serviceBounds.Min.Y, right, d.serviceBounds.Max.Y)
	}

	recipientRatios := [...]float64{0, 0.46, 0.70, 1}
	for index := range healerShopRecipientColumns {
		left := d.recipientBounds.Min.X + int(math.Round(float64(d.recipientBounds.Dx())*recipientRatios[index]))
		right := d.recipientBounds.Min.X + int(math.Round(float64(d.recipientBounds.Dx())*recipientRatios[index+1]))
		d.recipientColumns[index] = image.Rect(left, d.recipientBounds.Min.Y, right, d.recipientBounds.Max.Y)
	}
}

func (d *HealerShopDialog) drawServicePane(screen *ebiten.Image) {
	drawMerchantPane(screen, d.serviceBounds, d.pixelUnit())
	d.drawCell(screen, d.serviceHeader, d.serviceColumns[0], "Svc", DialogLabelColor, etext.AlignCenter)
	d.drawCell(screen, d.serviceHeader, d.serviceColumns[1], "Price", DialogLabelColor, etext.AlignCenter)
	d.drawHeaderRule(screen, d.serviceHeader)
	for index, quote := range d.state.Services {
		row := d.serviceRows[index]
		focused := d.focus == HealerShopServices && index == d.selectedService && d.phase == HealerShopBrowse
		retained := index == d.selectedService && !focused
		if focused {
			fillSelectedRow(screen, row)
		}
		textColor := DialogItemNameColor
		if retained {
			textColor = DialogLabelColor
		}
		if focused {
			textColor = DialogSelectedText
		}
		d.drawHotkeyCell(
			screen, row, d.serviceColumns[0], healerServiceDisplayName(quote.Service), textColor, focused,
		)
		d.drawCell(
			screen, row, d.serviceColumns[1], healerServicePriceLabel(quote),
			textColor, etext.AlignCenter,
		)
		if index < healerShopServiceCount-1 {
			d.drawRowRule(screen, row)
		}
	}
}

func (d *HealerShopDialog) drawRecipientPane(screen *ebiten.Image) {
	drawMerchantPane(screen, d.recipientBounds, d.pixelUnit())
	headings := [...]string{"Name", "State", "HP"}
	for index, heading := range headings {
		d.drawCell(screen, d.recipientHeader, d.recipientColumns[index], heading, DialogLabelColor, etext.AlignStart)
	}
	d.drawHeaderRule(screen, d.recipientHeader)
	for index, recipient := range d.state.Recipients {
		d.drawRecipientRow(screen, index, recipient)
		if index < len(d.state.Recipients)-1 {
			d.drawRowRule(screen, d.recipientRows[index])
		}
	}
}

func (d *HealerShopDialog) drawRecipientRow(
	screen *ebiten.Image,
	index int,
	recipient game_state.HealerRecipientState,
) {
	row := d.recipientRows[index]
	projection := recipient.Treatments[d.selectedService]
	focused := d.recipientRowFocused(index)
	if focused {
		fillSelectedRow(screen, row)
	}
	textColor := healerRecipientRowColor(
		projection.Eligibility,
		d.state.Services[d.selectedService].Affordable,
		focused,
	)
	values := [...]string{
		recipient.Name,
		healerStatusLabel(recipient.Status),
		fmt.Sprintf("%d/%d", recipient.HP, recipient.MaxHP),
	}
	for column, value := range values {
		d.drawCell(screen, row, d.recipientColumns[column], value, textColor, etext.AlignStart)
	}
}

func (d *HealerShopDialog) recipientRowFocused(index int) bool {
	return d.focus == HealerShopRecipients && index == d.selectedRecipient && d.phase == HealerShopBrowse
}

func healerRecipientRowColor(
	eligibility game_state.HealerEligibility,
	affordable bool,
	focused bool,
) stdcolor.Color {
	if focused {
		return DialogSelectedText
	}
	if eligibility.IsEligible() && !affordable {
		return DialogLabelColor
	}
	if !eligibility.IsEligible() {
		return DialogEmptyColor
	}

	return DialogDeltaPositive
}

func (d *HealerShopDialog) drawDetailPane(screen *ebiten.Image) {
	drawMerchantPane(screen, d.detailBounds, d.pixelUnit())
	d.detailOutput.DrawPanelContent(
		screen,
		image.Pt(d.detailTextBounds.Min.X, verticallyCenteredOutputY(d.detailTextBounds, d.detailOutput)),
		max(1, d.detailTextBounds.Dx()),
		etext.AlignStart,
	)
}

func (d *HealerShopDialog) drawCell(
	screen *ebiten.Image,
	row image.Rectangle,
	column image.Rectangle,
	value string,
	textColor stdcolor.Color,
	alignment etext.Align,
) {
	inset := d.pixelUnit() * healerShopCellInsetUnits
	cell := image.Rect(column.Min.X+inset, row.Min.Y, column.Max.X-inset, row.Max.Y)
	d.cellOutput.Reset()
	d.cellOutput.AppendLine().Text(value, textColor).Done()
	textHeight := int(math.Ceil(d.cellOutput.GetFontHeight()))
	y := cell.Min.Y + max(0, (cell.Dy()-textHeight)/healerShopCenterDivisor)
	d.cellOutput.DrawPanelContent(screen, image.Pt(cell.Min.X, y), max(1, cell.Dx()), alignment)
}

func (d *HealerShopDialog) drawHotkeyCell(
	screen *ebiten.Image,
	row image.Rectangle,
	column image.Rectangle,
	label string,
	textColor stdcolor.Color,
	focused bool,
) {
	inset := d.pixelUnit() * healerShopCellInsetUnits
	cell := image.Rect(column.Min.X+inset, row.Min.Y, column.Max.X-inset, row.Max.Y)
	d.cellOutput.Reset()
	line := d.cellOutput.AppendLine()
	if focused {
		line.Text(label, textColor)
	} else {
		line.Text(label[:1], DialogLabelColor).Text(label[1:], textColor)
	}
	line.Done()
	textHeight := int(math.Ceil(d.cellOutput.GetFontHeight()))
	y := cell.Min.Y + max(0, (cell.Dy()-textHeight)/healerShopCenterDivisor)
	d.cellOutput.DrawPanelContent(screen, image.Pt(cell.Min.X, y), max(1, cell.Dx()), etext.AlignStart)
}

func (d *HealerShopDialog) drawHeaderRule(screen *ebiten.Image, header image.Rectangle) {
	vector.StrokeLine(
		screen,
		float32(header.Min.X),
		float32(header.Max.Y),
		float32(header.Max.X),
		float32(header.Max.Y),
		float32(d.pixelUnit()),
		DialogRuleColor,
		false,
	)
}

func (d *HealerShopDialog) drawRowRule(screen *ebiten.Image, row image.Rectangle) {
	vector.StrokeLine(
		screen,
		float32(row.Min.X),
		float32(row.Max.Y),
		float32(row.Max.X),
		float32(row.Max.Y),
		float32(d.pixelUnit()),
		DialogRuleLightColor,
		false,
	)
}

func (d *HealerShopDialog) populateDetailOutput() {
	if d.detailOutput == nil || len(d.state.Recipients) == 0 {
		return
	}
	service := d.state.Services[d.selectedService]
	d.detailOutput.Reset()
	if d.focus == HealerShopServices && d.phase == HealerShopBrowse && d.receiptText == "" {
		d.populateServiceDetail(service)

		return
	}
	recipient := d.state.Recipients[d.selectedRecipient]
	projection := recipient.Treatments[d.selectedService]
	line := d.detailOutput.AppendLine()
	if d.preview {
		line.Text("Preview - ", DialogStatusWarningColor)
	}
	line.Text(service.Service.String()+": "+recipient.Name, DialogHeaderColor).Done()
	detailText := healerProjectionText(service.Service, recipient, projection)
	detailColor := healerProjectionColor(projection, service.Affordable)
	if d.receiptText != "" {
		detailText = d.receiptText
		detailColor = d.receiptColor
	}
	d.detailOutput.AppendLine().Text(detailText, detailColor).Done()

	priceText, priceColor := healerChargeText(service, d.state.Gold)
	d.detailOutput.AppendLine().Text(priceText, priceColor).Done()
	statusText, statusColor := d.detailStatusText(service, projection)
	line = d.detailOutput.AppendLine()
	switch {
	case d.phase == HealerShopSessionError:
		line.Text("Enter", DialogControlKeyColor).Text(" or ", DialogControlHintColor).
			Text("Esc", DialogControlKeyColor).Text(" to leave.", DialogControlHintColor)
	case d.phase == HealerShopBrowse && d.receiptText == "" && projection.Eligibility.IsEligible() && service.Affordable:
		line.Text("Enter", DialogControlKeyColor).Text(" to review.", DialogControlHintColor)
	default:
		line.Text(statusText, statusColor)
	}
	line.Done()
}
func (d *HealerShopDialog) populateServiceDetail(service game_state.HealerServiceQuote) {
	line := d.detailOutput.AppendLine()
	if d.preview {
		line.Text("Preview - ", DialogStatusWarningColor)
	}
	line.Text(service.Service.String(), DialogHeaderColor).Done()
	d.detailOutput.AppendLine().Text(healerServiceDescription(service.Service), DialogValueColor).Done()
	priceText, priceColor := healerChargeText(service, d.state.Gold)
	d.detailOutput.AppendLine().Text(priceText, priceColor).Done()
	d.detailOutput.AppendLine().Text(
		"Choose a party member.",
		DialogControlHintColor,
	).Done()
}

func (d *HealerShopDialog) detailStatusText(
	service game_state.HealerServiceQuote,
	projection game_state.HealerTreatmentProjection,
) (string, stdcolor.Color) {
	return d.detailActionText(projection.Eligibility.IsEligible(), service.Affordable)
}

func (d *HealerShopDialog) detailActionText(eligible bool, affordable bool) (string, stdcolor.Color) {
	switch d.phase {
	case HealerShopBrowse:
		if d.receiptText != "" {
			return "Choose another service or party member.", DialogControlHintColor
		}
		if !eligible {
			return "Choose another party member.", DialogControlHintColor
		}
		if !affordable {
			return "Not enough gold for this treatment.", DialogLabelColor
		}

		return "Enter to review.", DialogControlKeyColor
	case HealerShopAwaitingApproval:
		return "Confirm or cancel the treatment.", DialogControlHintColor
	case HealerShopAwaitingResult:
		return "", DialogNavHintColor
	case HealerShopSessionError:
		return "Enter or Esc to leave.", DialogStatusWarningColor
	default:
		panic("HealerShopDialog.detailStatusText: unknown phase")
	}
}

func (d *HealerShopDialog) syncPresentation() {
	d.shell.RefreshShop(d.state.Merchant, d.state.Gold, d.latestExchange, d.footerGuidance())
	d.populateDetailOutput()
}

func (d *HealerShopDialog) footerGuidance() string {
	switch d.phase {
	case HealerShopBrowse:
		action := "[Enter] Review"
		if d.focus == HealerShopServices {
			action = "[Enter] Choose party member"
		}

		return "[Left/Right] Pane   [Up/Down] Select\n[A] All   " + action + "   [Esc] Leave"
	case HealerShopAwaitingApproval:
		return "Confirm or cancel the treatment."
	case HealerShopAwaitingResult:
		return ""
	case HealerShopSessionError:
		return merchantLeaveFooter
	default:
		panic("HealerShopDialog.footerGuidance: unknown phase")
	}
}

func (d *HealerShopDialog) selectServiceShortcut() bool {
	keys := [...]ebiten.Key{ebiten.KeyC, ebiten.KeyH, ebiten.KeyR}
	for index, key := range keys {
		if !d.keyboard.IsKeyPressedAndRegister(key) {
			continue
		}
		d.selectedService = index
		d.receiptText = ""
		d.populateDetailOutput()

		return true
	}

	return false
}

func (d *HealerShopDialog) selectAllShortcut() bool {
	if !d.keyboard.IsKeyPressedAndRegister(ebiten.KeyA) {
		return false
	}
	quote := d.state.AllServices[d.selectedService]
	if quote.RecipientCount == 0 {
		d.latestExchange = d.prose.NoNeed
		d.receiptText = "No party members need " + quote.Service.String() + "."
		d.receiptColor = DialogControlHintColor
		d.syncPresentation()

		return true
	}
	d.emitTreatmentIntent(HealerTreatmentIntent{
		Request: game_state.HealerServiceRequest{
			Service:        d.GetSelectedService(),
			CharacterIndex: game_state.HealerAllRecipientsIndex,
		},
		Service:    d.state.Services[d.selectedService],
		AllService: quote,
		Recipient:  nil,
		Gold:       d.state.Gold,
	})

	return true
}

func (d *HealerShopDialog) moveSelection(direction int) {
	if d.focus == HealerShopServices {
		d.selectedService = max(0, min(healerShopServiceCount-1, d.selectedService+direction))
	} else {
		d.selectedRecipient = max(0, min(len(d.state.Recipients)-1, d.selectedRecipient+direction))
	}
	d.receiptText = ""
	d.populateDetailOutput()
}

func (d *HealerShopDialog) reviewSelection() {
	projection := d.selectedProjection()
	if !projection.Eligibility.IsEligible() {
		d.latestExchange = d.prose.NoNeed
		d.receiptText = healerIneligibleText(
			d.GetSelectedService(),
			d.GetSelectedRecipient(),
			projection.Eligibility,
		)
		d.receiptColor = DialogControlHintColor
		d.syncPresentation()

		return
	}
	recipient := d.GetSelectedRecipient()
	d.emitTreatmentIntent(HealerTreatmentIntent{
		Request:    d.selectedRequest(),
		Service:    d.state.Services[d.selectedService],
		AllService: d.state.AllServices[d.selectedService],
		Recipient:  &recipient,
		Gold:       d.state.Gold,
	})
}

func (d *HealerShopDialog) selectedRequest() game_state.HealerServiceRequest {
	return game_state.HealerServiceRequest{
		Service:        d.GetSelectedService(),
		CharacterIndex: d.GetSelectedRecipient().CharacterIndex,
	}
}

func (d *HealerShopDialog) emitTreatmentIntent(intent HealerTreatmentIntent) {
	if d.pendingIntent != nil {
		panic("HealerShopDialog: treatment approval is already pending")
	}
	d.receiptText = ""
	d.pendingIntent = &intent
	d.awaitingRequest = intent.Request
	d.transitionTo(HealerShopAwaitingApproval)
	d.syncPresentation()
}

func (d *HealerShopDialog) selectedProjection() game_state.HealerTreatmentProjection {
	return d.GetSelectedRecipient().Treatments[d.selectedService]
}

func (d *HealerShopDialog) applyRefreshedState(
	refreshed game_state.HealerShopState,
	selection game_state.HealerServiceRequest,
) {
	validateHealerShopState(refreshed)
	d.state = refreshed
	d.selectedService = int(selection.Service)
	d.selectedRecipient = 0
	d.calculateLayout()
	if selection.CharacterIndex == game_state.HealerAllRecipientsIndex {
		return
	}
	for index, recipient := range refreshed.Recipients {
		if recipient.CharacterIndex == selection.CharacterIndex {
			d.selectedRecipient = index

			break
		}
	}
}

func (d *HealerShopDialog) transitionTo(phase HealerShopDialogPhase) {
	if d.phase == phase {
		return
	}
	d.phase = phase
	d.keyboard.PrepareForTransition(0)
}

func (d *HealerShopDialog) clearAwaitingRequest() {
	d.pendingIntent = nil
	d.awaitingRequest = emptyHealerServiceRequest()
}

func emptyHealerServiceRequest() game_state.HealerServiceRequest {
	return game_state.HealerServiceRequest{Service: references.HealerCure, CharacterIndex: 0}
}

func emptyHealerTreatmentIntent() HealerTreatmentIntent {
	return HealerTreatmentIntent{
		Request: emptyHealerServiceRequest(),
		Service: game_state.HealerServiceQuote{
			Service: references.HealerCure, SourcePrice: 0, Charge: 0,
			ChargeKind: game_state.HealerChargePaid, Affordable: false,
		},
		AllService: game_state.HealerAllServiceQuote{
			Service: references.HealerCure, SourcePrice: 0, Charge: 0,
			ChargeKind: game_state.HealerChargePaid, Affordable: false, RecipientCount: 0,
		},
		Recipient: nil,
		Gold:      0,
	}
}

func (d *HealerShopDialog) enterPressed() bool {
	return d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter)
}

func (d *HealerShopDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func healerServiceDisplayName(service references.HealerService) string {
	if service == references.HealerResurrect {
		return "Ress."
	}

	return service.String()
}

func healerServicePriceLabel(quote game_state.HealerServiceQuote) string {
	switch quote.ChargeKind {
	case game_state.HealerChargePaid:
		return strconv.FormatUint(uint64(quote.Charge), 10)
	case game_state.HealerChargeFree:
		return "Free"
	case game_state.HealerChargeCharitable:
		return "Charity"
	default:
		panic("HealerShopDialog: unknown charge kind")
	}
}

func healerChargeText(quote game_state.HealerServiceQuote, gold uint16) (string, stdcolor.Color) {
	switch quote.ChargeKind {
	case game_state.HealerChargeFree:
		return "Free care. No payment.", DialogDeltaPositive
	case game_state.HealerChargeCharitable:
		return "Charitable care. No payment.", DialogDeltaPositive
	case game_state.HealerChargePaid:
		if !quote.Affordable {
			return fmt.Sprintf("Need %d gold; party has %d.", quote.Charge, gold), DialogLabelColor
		}

		return fmt.Sprintf("Cost: %d gold; %d after payment.", quote.Charge, gold-quote.Charge), DialogLabelColor
	default:
		panic("HealerShopDialog: unknown charge kind")
	}
}

func healerProjectionText(
	service references.HealerService,
	recipient game_state.HealerRecipientState,
	projection game_state.HealerTreatmentProjection,
) string {
	if !projection.Eligibility.IsEligible() {
		return healerIneligibleText(service, recipient, projection.Eligibility)
	}
	switch service {
	case references.HealerCure:
		return fmt.Sprintf(
			"%s: %s to %s. HP stays %d/%d.",
			recipient.Name,
			healerStatusName(recipient.Status),
			healerStatusName(projection.StatusAfter),
			recipient.HP,
			recipient.MaxHP,
		)
	case references.HealerHeal:
		return fmt.Sprintf(
			"%s: HP %d/%d to %d/%d. Status stays %s.",
			recipient.Name,
			recipient.HP,
			recipient.MaxHP,
			projection.HPAfter,
			projection.MaxHPAfter,
			healerStatusName(projection.StatusAfter),
		)
	case references.HealerResurrect:
		return fmt.Sprintf(
			"%s: Dead to Good, level %d, HP %d/%d.",
			recipient.Name,
			projection.LevelAfter,
			projection.HPAfter,
			projection.MaxHPAfter,
		)
	default:
		panic("HealerShopDialog: unknown service projection")
	}
}

func healerProjectionColor(
	projection game_state.HealerTreatmentProjection,
	affordable bool,
) stdcolor.Color {
	if !projection.Eligibility.IsEligible() {
		return DialogControlHintColor
	}
	if !affordable {
		return DialogLabelColor
	}

	return DialogDeltaPositive
}

func healerServiceDescription(service references.HealerService) string {
	switch service {
	case references.HealerCure:
		return "Removes poison without changing HP."
	case references.HealerHeal:
		return "Restores an injured living party member to full HP."
	case references.HealerResurrect:
		return "Returns a dead party member to life."
	default:
		panic("HealerShopDialog: unknown service description")
	}
}

func healerStatusLabel(status party_state.CharacterStatus) string {
	switch status {
	case party_state.Good:
		return "G"
	case party_state.Poisoned:
		return "P"
	case party_state.Charmed:
		return "C"
	case party_state.Sleep:
		return "S"
	case party_state.Dead:
		return "D"
	default:
		return "?"
	}
}

func healerStatusName(status party_state.CharacterStatus) string {
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
		return "Dead"
	default:
		return "Unknown status"
	}
}

func healerIneligibleText(
	service references.HealerService,
	recipient game_state.HealerRecipientState,
	eligibility game_state.HealerEligibility,
) string {
	switch eligibility {
	case game_state.HealerEligible:
		return "This treatment can help " + recipient.Name + "."
	case game_state.HealerNotPoisoned:
		return fmt.Sprintf(
			"%s is %s. %s would not help; choose someone Poisoned.",
			recipient.Name,
			healerStatusName(recipient.Status),
			service.String(),
		)
	case game_state.HealerNotInjured:
		return recipient.Name + " already has full HP. Heal would not help."
	case game_state.HealerDeadCannotHeal:
		return recipient.Name + " is Dead. Heal cannot help; choose Resurrect."
	case game_state.HealerLivingCannotResurrect:
		return recipient.Name + " is still living. Resurrect would not help."
	default:
		return "This treatment is unavailable."
	}
}

func validateHealerShopProse(prose HealerShopDialogProse) {
	validateFinalizedMerchantText("healer greeting", prose.Greeting)
	validateFinalizedMerchantText("healer no-need response", prose.NoNeed)
	validateFinalizedMerchantText("healer funds refusal", prose.FundsRefusal)
}

func validateHealerShopState(state game_state.HealerShopState) {
	if state.Merchant.ID == "" || state.Merchant.Family != references.MerchantFamilyHealer {
		panic("HealerShopDialog: detached healer state is required")
	}
	if len(state.Recipients) == 0 || len(state.Recipients) > healerShopMaximumRecipients {
		panic("HealerShopDialog: one to six active recipients are required")
	}
	validateHealerServiceQuotes(state.Services)
	validateHealerAllServiceQuotes(state.AllServices)
	validateHealerRecipients(state.Recipients)
}

func validateHealerServiceQuotes(services [healerShopServiceCount]game_state.HealerServiceQuote) {
	canonical := [...]references.HealerService{
		references.HealerCure,
		references.HealerHeal,
		references.HealerResurrect,
	}
	for index, service := range canonical {
		quote := services[index]
		if quote.Service != service || quote.SourcePrice == 0 {
			panic("HealerShopDialog: Cure, Heal, and Resurrect quotes are required")
		}
		if quote.ChargeKind == game_state.HealerChargePaid && quote.Charge != quote.SourcePrice {
			panic("HealerShopDialog: paid charge must match the fixed source price")
		}
		if quote.ChargeKind != game_state.HealerChargePaid && (quote.Charge != 0 || !quote.Affordable) {
			panic("HealerShopDialog: free and charitable care must be affordable with zero charge")
		}
	}
}

func validateHealerAllServiceQuotes(services [healerShopServiceCount]game_state.HealerAllServiceQuote) {
	for index, service := range services {
		validateHealerAllServiceQuote(index, service)
	}
}

func validateHealerAllServiceQuote(index int, service game_state.HealerAllServiceQuote) {
	if int(service.Service) != index || service.SourcePrice == 0 ||
		service.RecipientCount < 0 || service.RecipientCount > healerShopMaximumRecipients {
		panic("HealerShopDialog: aggregate Cure, Heal, and Resurrect quotes are required")
	}
	if service.ChargeKind == game_state.HealerChargePaid &&
		service.Charge != service.SourcePrice*uint16(service.RecipientCount) {
		panic("HealerShopDialog: paid aggregate charge must equal price times recipients")
	}
	if service.ChargeKind != game_state.HealerChargePaid && (service.Charge != 0 || !service.Affordable) {
		panic("HealerShopDialog: aggregate free and charitable care must be affordable with zero charge")
	}
}

func validateHealerRecipients(recipients []game_state.HealerRecipientState) {
	seen := make(map[int]struct{}, len(recipients))
	for _, recipient := range recipients {
		if recipient.CharacterIndex < 0 || recipient.CharacterIndex >= party_state.MaxActiveParty || recipient.Name == "" {
			panic("HealerShopDialog: invalid active-party recipient")
		}
		if _, exists := seen[recipient.CharacterIndex]; exists {
			panic("HealerShopDialog: duplicate active-party recipient")
		}
		seen[recipient.CharacterIndex] = struct{}{}
	}
}
