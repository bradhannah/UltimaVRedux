package widgets

import (
	"image"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	innActionStartPercentX    = 0.22
	innActionEndPercentX      = 0.78
	innActionStartPercentY    = 0.08
	innActionEndPercentY      = 0.92
	innActionHeaderRatio      = 0.11
	innActionFooterRatio      = 0.13
	innActionMinimumInset     = 5
	innActionLineSpacing      = 1.08
	innActionMaximumBodyLines = 24
	innActionFooterLines      = 1
	innActionMinimumTextChars = 8
	innActionFooterGuidance   = "[Enter] Confirm   [Esc] Back"
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

	frame        *merchantModalFrame
	headerOutput *text.Output
	bodyOutput   *text.Output
	footerOutput *text.Output
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
		frame: newMerchantModalFrame(merchantModalFrameConfig{
			placement:             innActionPlacement(),
			headerRatio:           innActionHeaderRatio,
			footerRatio:           innActionFooterRatio,
			minimumInset:          innActionMinimumInset,
			lineSpacingMultiplier: innActionLineSpacing,
			footerMinimumLines:    1,
		}),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.ensureLayout()

	return dialog
}

// Draw renders the opaque semantic merchant approval modal.
func (d *InnActionDialog) Draw(screen *ebiten.Image) {
	if screen == nil || !d.IsVisible() {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
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

// GetFooterGuidance returns the keyboard-only approval controls.
func (d *InnActionDialog) GetFooterGuidance() string { return innActionFooterGuidance }

// GetBounds returns the centered modal bounds.
func (d *InnActionDialog) GetBounds() image.Rectangle {
	d.ensureLayout()

	return d.frame.bounds()
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

func (d *InnActionDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	d.headerOutput, d.bodyOutput, d.footerOutput = d.frame.newOutputs(merchantModalTextBudget{
		lineSpacing: innActionLineSpacing, maximumBodyLines: innActionMaximumBodyLines,
		footerLines: innActionFooterLines, minimumChars: innActionMinimumTextChars, fixedBody: true,
	})
	d.populateOutputs()
}

func (d *InnActionDialog) populateOutputs() {
	if d.headerOutput == nil || d.bodyOutput == nil || d.footerOutput == nil {
		return
	}
	d.headerOutput.Reset()
	title := d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter)
	if d.intent.Preview {
		title.Text("PREVIEW - ", DialogStatusWarningColor)
	}
	title.Text("Approve "+d.intent.Action.String(), DialogHeaderColor).Done()

	d.bodyOutput.Reset()
	if d.intent.Preview {
		d.bodyOutput.AppendLine().WithAlignment(etext.AlignCenter).
			Text("PREVIEW: authority has not run.", DialogStatusWarningColor).Done()
	}
	switch d.intent.Action {
	case InnActionRest:
		d.populateRestBody()
	case InnActionLeave:
		d.populateLeaveBody()
	case InnActionPickup:
		d.populatePickupBody()
	default:
		panic("InnActionDialog.populateOutputs: unknown action")
	}

	d.footerOutput.Reset()
	appendControlGuidance(d.footerOutput, d.GetFooterGuidance(), d.frame.footerText.Dx())
}

func (d *InnActionDialog) populateRestBody() {
	quote := d.intent.Rest
	d.bodyOutput.AppendLine().LightGray("Inn: ").White(d.intent.InnName).Done()
	d.bodyOutput.AppendLine().LightGray("Listed charge: ").Yellowf("%d gold", quote.Charge).Done()
	if quote.Affordable {
		d.bodyOutput.AppendLine().LightGray("Gold after listed payment: ").Cyanf(
			"%d -> %d", quote.GoldBefore, quote.GoldAfterPayment,
		).Done()
	} else {
		d.bodyOutput.AppendLine().Textf(
			"Gold %d; shortfall %d. Confirmation remains available.",
			DialogStatusWarningColor,
			quote.GoldBefore,
			int(quote.Charge)-int(quote.GoldBefore),
		).WordWrap().Done()
	}
	d.bodyOutput.AppendLine().LightGray("Wake: ").White(innWakeText(quote)).Done()
	d.bodyOutput.AppendLine().Text(
		"Successful Rest ends active spells.", DialogControlHintColor,
	).Done()
	d.appendRestRecoverySummary(quote.MemberProjections)
	names := innPoisonedNames(quote.MemberProjections)
	if len(names) > 0 {
		d.bodyOutput.AppendLine().Text(
			"Poisoned: "+strings.Join(names, ", "), DialogStatusWarningColor,
		).WordWrap().Done()
		d.bodyOutput.AppendLine().Text("WILL DIE during Rest.", DialogStatusWarningColor).Done()
	}
}

func (d *InnActionDialog) appendRestRecoverySummary(projections []game_state.InnCharacterProjection) {
	recovering, deadNames, poisoned := classifyInnRestProjections(projections)
	switch {
	case recovering == len(projections):
		d.bodyOutput.AppendLine().LightGreen("Everyone fully heals.").Done()
	case recovering > 0 && poisoned:
		d.bodyOutput.AppendLine().LightGreen("Unaffected living party members fully heal.").Done()
	case recovering > 0:
		d.bodyOutput.AppendLine().LightGreen("All living party members fully heal.").Done()
	}
	if len(deadNames) > 0 {
		d.bodyOutput.AppendLine().LightGray("Dead and unchanged: " + strings.Join(deadNames, ", ") + ".").
			WordWrap().Done()
	}
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

func (d *InnActionDialog) populateLeaveBody() {
	row := d.intent.Leave
	d.bodyOutput.AppendLine().Yellow("Companion: ").White(row.Identity.Name).Done()
	d.bodyOutput.AppendLine().LightGray("Inn: ").White(d.intent.InnName).Done()
	d.bodyOutput.AppendLine().LightGray("Charge today: ").LightGreen("0 gold").Done()
	if d.intent.HasMonthlyRate {
		d.bodyOutput.AppendLine().LightGray("Current monthly rate: ").Yellowf(
			"%d gold", d.intent.MonthlyRate,
		).Done()
	}
	d.bodyOutput.AppendLine().LightGray("Minimum stay: ").White("one billed month").Done()
	d.bodyOutput.AppendLine().LightGray("Future rate: ").White("recalculated when picked up.").Done()
	d.bodyOutput.AppendLine().LightGray("Status retained: ").White(innStatusName(row.Status)).Done()
	d.bodyOutput.AppendLine().LightGray("Party: ").Cyanf(
		"%d/%d -> %d/%d", d.intent.PartySize, d.intent.PartyCapacity,
		row.PartySizeAfter, d.intent.PartyCapacity,
	).Done()
	d.bodyOutput.AppendLine().LightGray("Guest rooms: ").Cyanf(
		"%d/%d -> %d/%d", d.intent.RoomOccupancy, d.intent.RoomCapacity,
		row.OccupancyAfter, d.intent.RoomCapacity,
	).Done()
	if innHasWarning(row.Warnings, game_state.InnWarningLastLivingMember) {
		d.bodyOutput.AppendLine().Text(
			"SEVERE: this removes the last living member from the active party.",
			DialogStatusWarningColor,
		).WordWrap().Done()
	}
}

func (d *InnActionDialog) populatePickupBody() {
	row := d.intent.Pickup
	d.bodyOutput.AppendLine().Yellow("Guest: ").White(row.Identity.Name).Done()
	d.bodyOutput.AppendLine().LightGray("Inn: ").White(d.intent.InnName).Done()
	d.bodyOutput.AppendLine().LightGray("Status: ").White(innStatusName(row.Status)).Done()
	d.bodyOutput.AppendLine().LightGray("Recorded / billable months: ").Whitef(
		"%d / %d", row.MonthsAtInn, row.BilledMonths,
	).Done()
	if innHasWarning(row.Warnings, game_state.InnWarningMinimumMonthCharge) {
		d.bodyOutput.AppendLine().Text("Minimum charge is one month.", DialogLabelColor).Done()
	}
	d.bodyOutput.AppendLine().LightGray("Monthly rate: ").Yellowf("%d gold", row.MonthlyRate).Done()
	d.bodyOutput.AppendLine().LightGray("Listed charge: ").Yellowf("%d gold", row.Charge).Done()
	if row.Affordable {
		d.bodyOutput.AppendLine().LightGray("Gold after listed payment: ").Cyanf(
			"%d -> %d", row.GoldBefore, row.GoldAfterPayment,
		).Done()
	} else {
		d.bodyOutput.AppendLine().Textf(
			"Gold %d; shortfall %d.", DialogStatusWarningColor,
			row.GoldBefore, int(row.Charge)-int(row.GoldBefore),
		).Done()
		d.bodyOutput.AppendLine().Text(
			"GUARD WARNING: confirming without enough gold activates guards.",
			DialogStatusWarningColor,
		).WordWrap().Done()
	}
	d.bodyOutput.AppendLine().LightGray("Party: ").Cyanf(
		"%d/%d -> %d/%d", d.intent.PartySize, d.intent.PartyCapacity,
		row.PartySizeAfter, d.intent.PartyCapacity,
	).Done()
	d.bodyOutput.AppendLine().LightGray("Guest rooms: ").Cyanf(
		"%d/%d -> %d/%d", d.intent.RoomOccupancy, d.intent.RoomCapacity,
		row.RoomOccupancyAfter, d.intent.RoomCapacity,
	).Done()
	if innHasWarning(row.Warnings, game_state.InnWarningPoisonDeath) {
		d.bodyOutput.AppendLine().Text(
			row.Identity.Name+" is Poisoned and WILL DIE when picked up.",
			DialogStatusWarningColor,
		).WordWrap().Done()
	}
}

func innActionPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: innActionStartPercentX,
		EndPercentX:   innActionEndPercentX,
		StartPercentY: innActionStartPercentY,
		EndPercentY:   innActionEndPercentY,
	}
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
