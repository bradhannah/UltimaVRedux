package widgets

import (
	"fmt"
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	innNoteChars     = 34 // The longest authored note line; the window is 36 letters wide.
	innTitleChars    = 28 // The longest title, with "PREVIEW - " before it, that the window's title line holds.
	innPreviewChars  = len("PREVIEW - ")
	innRestVerb      = restActionLabel
	innLeaveVerb     = "Leave"
	innPickUpVerb    = "Pick Up"
	innPartyLabel    = "Party of "
	innRoomsLabel    = "Rooms of "
	innNamesAnd      = " and "
	innNamesComma    = ", "
	innPickUpGuards  = "Asking anyway calls the guards."
	innRestTurnedOut = "The keeper turns the party out."
	innShortCost     = "It costs"
)

var _ Widget = (*InnActionDialog)(nil)

// InnActionDialogConfig configures one family-specific Inn approval modal.
type InnActionDialogConfig struct {
	Keyboard     *input.Keyboard
	Intent       InnActionIntent
	IsKeyPressed func(ebiten.Key) bool
}

// InnActionDialog presents exact reviewed consequences and emits the request unchanged.
type InnActionDialog struct {
	keyboard        *input.Keyboard
	isKeyPressed    func(ebiten.Key) bool
	intent          InnActionIntent
	visible         bool
	dismissed       bool
	cancelled       bool
	actionKeysReady bool
	pending         *InnActionRequest
	window          *amountWindow
}

// NewInnActionDialog creates a centered keyboard-only approval for Rest, Leave, or Pick Up.
func NewInnActionDialog(config InnActionDialogConfig) *InnActionDialog {
	if config.Keyboard == nil {
		panic("InnActionDialog: keyboard is required")
	}
	validateInnActionIntent(config.Intent)
	isKeyPressed := config.IsKeyPressed
	if isKeyPressed == nil {
		isKeyPressed = ebiten.IsKeyPressed
	}
	dialog := &InnActionDialog{ //nolint:exhaustruct_v5 // Resolution-dependent presentation fields initialize on demand.
		keyboard:        config.Keyboard,
		isKeyPressed:    isKeyPressed,
		intent:          cloneInnActionIntent(config.Intent),
		visible:         true,
		actionKeysReady: innActionKeysReleased(isKeyPressed),
		window:          newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque semantic merchant approval modal.
func (d *InnActionDialog) Draw(screen *ebiten.Image) {
	if screen == nil || !d.IsVisible() {
		return
	}
	d.window.draw(screen)
}

// Update confirms or cancels the exact reviewed request after transition-key release.
func (d *InnActionDialog) Update() {
	if !d.IsVisible() {
		return
	}
	if !d.actionKeysReady {
		if innActionKeysReleased(d.isKeyPressed) {
			d.actionKeysReady = true
		}

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancelled = true
		d.visible = false
		d.dismissed = true

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		request := d.intent.Request
		d.pending = &request
		d.visible = false
		d.dismissed = true
	}
}

// ConsumePendingRequest returns and clears the exact approved authority request.
func (d *InnActionDialog) ConsumePendingRequest() (InnActionRequest, bool) {
	if d.pending == nil {
		return emptyInnActionRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetBounds returns the centered modal bounds.
func (d *InnActionDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape returned to the parent without a request.
func (d *InnActionDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the workflow may remove the approval.
func (d *InnActionDialog) IsDismissed() bool { return d.dismissed }

// IsVisible reports whether the approval currently owns presentation and input.
func (d *InnActionDialog) IsVisible() bool { return d.visible && !d.dismissed }

// Dismiss closes the approval without emitting a request.
func (d *InnActionDialog) Dismiss() {
	d.visible = false
	d.dismissed = true
}

// step describes the approval window for the action: what it is, what it does
// to the party's gold, and every consequence the player decides by.
func (d *InnActionDialog) step() amountStep {
	step := emptyAmountStep()
	step.preview = d.intent.Preview
	switch d.intent.Action {
	case InnActionRest:
		d.describeRest(&step)
	case InnActionLeave:
		d.describeLeave(&step)
	case InnActionPickup:
		d.describePickup(&step)
	default:
		panic("InnActionDialog.step: unknown action")
	}

	return step
}

func (d *InnActionDialog) describeRest(step *amountStep) {
	quote := d.intent.Rest
	step.verb, step.title = innRestVerb, innRestVerb
	title := innRestVerb + " at " + d.intent.InnName
	if d.intent.Preview {
		title = strings.Repeat(" ", innPreviewChars) + title
	}
	if len(title) <= innTitleChars {
		step.title = strings.TrimSpace(title)
	}
	if !quote.Affordable {
		// Asking without the gold is still a real request: the keeper turns the party out.
		step.verb += tavernAnyway
		step.notes = shortOfGoldNotes(innShortCost, int(quote.Charge), int(quote.GoldBefore), innRestTurnedOut)

		return
	}
	step.rows = []amountRow{{label: amountGoldRow, have: int(quote.GoldBefore), then: int(quote.GoldAfterPayment)}}
	recovering, deadNames, poisoned := classifyInnRestProjections(quote.MemberProjections)
	if names := innPoisonedNames(quote.MemberProjections); len(names) > 0 {
		step.notes = append(step.notes, innNotes(joinInnNames(names)+" will die during the rest.", amountWarning)...)
		step.notes = append(step.notes, amountNote{text: "", tone: amountPlain})
	}
	wake := quote.DateTimeAfter
	step.notes = append(step.notes, innNotes(fmt.Sprintf("Wake at %s on %s.",
		strings.TrimSpace(wake.GetTimeAsString()), wake.GetDateAsString()), amountPlain)...)
	step.notes = append(step.notes, amountNote{text: "Active spells end.", tone: amountPlain})
	switch {
	case recovering == len(quote.MemberProjections):
		step.notes = append(step.notes, amountNote{text: "Everyone fully heals.", tone: amountPlain})
	case recovering > 0 && poisoned:
		step.notes = append(step.notes, amountNote{text: "The unaffected heal fully.", tone: amountPlain})
	case recovering > 0:
		step.notes = append(step.notes, amountNote{text: "Every living member heals fully.", tone: amountPlain})
	}
	if len(deadNames) > 0 {
		step.notes = append(step.notes, innNotes("Dead and unchanged: "+joinInnNames(deadNames)+".", amountPlain)...)
	}
}

func (d *InnActionDialog) describeLeave(step *amountStep) {
	row := d.intent.Leave
	name := row.Identity.Name
	step.verb, step.title = innLeaveVerb, innLeaveVerb+" "+name
	step.rows = d.innPlaceRows(row.PartySizeAfter, row.OccupancyAfter)
	if innHasWarning(row.Warnings, game_state.InnWarningLastLivingMember) {
		step.notes = append(step.notes,
			innNotes("This removes the last living member from the active party.", amountWarning)...)
		step.notes = append(step.notes, amountNote{text: "", tone: amountPlain})
	}
	step.notes = append(step.notes, amountNote{text: "No charge today.", tone: amountPlain})
	if d.intent.HasMonthlyRate {
		step.notes = append(step.notes, amountNote{
			text: fmt.Sprintf("Monthly rate: %d gold.", d.intent.MonthlyRate), tone: amountPlain,
		})
	}
	step.notes = append(step.notes,
		amountNote{text: "At least one month is billed.", tone: amountPlain},
		amountNote{text: "Pick up figures the rate again.", tone: amountPlain})
	status := strings.ToLower(innStatusName(row.Status))
	step.notes = append(step.notes, innNotes(name+" stays "+status+".", amountPlain)...)
}

func (d *InnActionDialog) describePickup(step *amountStep) {
	row := d.intent.Pickup
	name := row.Identity.Name
	step.verb, step.title = innPickUpVerb, innPickUpVerb+" "+name
	step.unit = fmt.Sprintf("%d gold a month for %s", row.MonthlyRate, countedNoun(row.BilledMonths, "month"))
	if !row.Affordable {
		// Asking without the gold is still a real request: the guards are called.
		step.verb += tavernAnyway
		step.notes = shortOfGoldNotes(innShortCost, int(row.Charge), int(row.GoldBefore), innPickUpGuards)

		return
	}
	step.rows = append([]amountRow{{label: amountGoldRow, have: int(row.GoldBefore), then: int(row.GoldAfterPayment)}},
		d.innPlaceRows(row.PartySizeAfter, row.RoomOccupancyAfter)...)
	if innHasWarning(row.Warnings, game_state.InnWarningPoisonDeath) {
		step.notes = append(step.notes, innNotes(name+" is poisoned and will die when picked up.", amountWarning)...)
		step.notes = append(step.notes, amountNote{text: "", tone: amountPlain})
	} else {
		step.notes = append(step.notes, innNotes(name+" is "+strings.ToLower(innStatusName(row.Status))+".", amountPlain)...)
	}
	if int(row.MonthsAtInn) != row.BilledMonths || innHasWarning(row.Warnings, game_state.InnWarningMinimumMonthCharge) {
		step.notes = append(step.notes, amountNote{
			text: "Stayed " + countedNoun(int(row.MonthsAtInn), "month") + ".", tone: amountPlain,
		})
	}
	if innHasWarning(row.Warnings, game_state.InnWarningMinimumMonthCharge) {
		step.notes = append(step.notes, amountNote{text: "One month is the least billed.", tone: amountPlain})
	}
}

// innPlaceRows gives the party's places and the inn's rooms before and after.
func (d *InnActionDialog) innPlaceRows(partyAfter, roomsAfter int) []amountRow {
	intent := d.intent

	return []amountRow{
		{label: fmt.Sprintf("%s%d", innPartyLabel, intent.PartyCapacity), have: intent.PartySize, then: partyAfter},
		{label: fmt.Sprintf("%s%d", innRoomsLabel, intent.RoomCapacity), have: intent.RoomOccupancy, then: roomsAfter},
	}
}

// innNotes breaks one sentence into note lines that each fit the window.
func innNotes(sentence string, tone amountTone) []amountNote {
	var notes []amountNote
	line := ""
	for _, word := range strings.Fields(sentence) {
		if line != "" && len(line)+1+len(word) > innNoteChars {
			notes = append(notes, amountNote{text: line, tone: tone})
			line = ""
		}
		if line != "" {
			line += " "
		}
		line += word
	}

	return append(notes, amountNote{text: line, tone: tone})
}

// joinInnNames words a list of names, such as "Iolo, Shamino and Mariah".
func joinInnNames(names []string) string {
	last := len(names) - 1
	if last < 1 {
		return strings.Join(names, "")
	}

	return strings.Join(names[:last], innNamesComma) + innNamesAnd + names[last]
}

func classifyInnRestProjections(
	projections []game_state.InnCharacterProjection,
) (int, []string, bool) {
	recovering := 0
	deadNames := make([]string, 0)
	poisoned := false
	for _, projection := range projections {
		switch projection.StatusBefore {
		case party_state.Dead:
			deadNames = append(deadNames, projection.Identity.Name)
		case party_state.Poisoned:
			poisoned = true
		case party_state.Good, party_state.Charmed, party_state.Sleep:
			recovering++
		default:
			panic("InnActionDialog: unknown Rest projection status")
		}
	}

	return recovering, deadNames, poisoned
}

func validateInnActionIntent(intent InnActionIntent) {
	if intent.Action != intent.Request.Action || strings.TrimSpace(intent.InnName) == "" ||
		intent.RoomCapacity <= 0 || intent.RoomOccupancy < 0 || intent.RoomOccupancy > intent.RoomCapacity ||
		intent.PartyCapacity <= 0 || intent.PartySize <= 0 || intent.PartySize > intent.PartyCapacity {
		panic("InnActionDialog: complete detached Inn context is required")
	}
	switch intent.Action {
	case InnActionRest:
		validateInnRestIntent(intent)
	case InnActionLeave:
		validateInnLeaveIntent(intent)
	case InnActionPickup:
		validateInnPickupIntent(intent)
	default:
		panic("InnActionDialog: known action is required")
	}
}

func validateInnRestIntent(intent InnActionIntent) {
	request := intent.Request.Rest
	quote := intent.Rest
	if request.ReviewedMerchantID == "" || request.ReviewedPricingCharacterIndex < 0 ||
		request.ReviewedRosterFingerprint == "" || request.ReviewedCharge != quote.Charge ||
		request.ReviewedGoldBefore != quote.GoldBefore || quote.BlockReason != game_state.InnRestBlockNone ||
		quote.Charge == 0 || len(quote.MemberProjections) != intent.PartySize {
		panic("InnActionDialog: available detached Rest review is required")
	}
	wantAfter := quote.GoldBefore
	if quote.Affordable {
		wantAfter -= quote.Charge
	}
	if quote.Affordable != (quote.Charge <= quote.GoldBefore) || quote.GoldAfterPayment != wantAfter {
		panic("InnActionDialog: Rest gold consequence is inconsistent")
	}
}

func validateInnLeaveIntent(intent InnActionIntent) {
	request := intent.Request.Leave
	row := intent.Leave
	if request.ReviewedMerchantID == "" || request.ReviewedPricingCharacterIndex < 0 ||
		request.ReviewedRosterFingerprint == "" || request.Character != row.Identity ||
		request.ReviewedCharacterIndex != row.RosterIndex || !row.Eligible ||
		row.BlockReason != game_state.InnLeaveBlockNone || row.Projection.Identity != row.Identity {
		panic("InnActionDialog: eligible detached Leave review is required")
	}
	validateInnIdentity(row.Identity)
	validateInnStatus(row.Status)
	validateInnWarnings(row.Warnings)
}

func validateInnPickupIntent(intent InnActionIntent) {
	request := intent.Request.Pickup
	row := intent.Pickup
	if request.ReviewedMerchantID == "" || request.ReviewedPricingCharacterIndex < 0 ||
		request.ReviewedRosterFingerprint == "" || request.Character != row.Identity ||
		request.ReviewedCharacterIndex != row.RosterIndex || request.ReviewedCharge != row.Charge ||
		request.ReviewedGoldBefore != row.GoldBefore || !row.Confirmable ||
		row.BlockReason != game_state.InnPickupBlockNone || row.Projection.Identity != row.Identity {
		panic("InnActionDialog: confirmable detached Pick Up review is required")
	}
	validateInnIdentity(row.Identity)
	validateInnStatus(row.Status)
	validateInnWarnings(row.Warnings)
	wantAfter := row.GoldBefore
	if row.Affordable {
		wantAfter -= row.Charge
	}
	if row.Affordable != (row.Charge <= row.GoldBefore) || row.GoldAfterPayment != wantAfter {
		panic("InnActionDialog: Pick Up gold consequence is inconsistent")
	}
}
