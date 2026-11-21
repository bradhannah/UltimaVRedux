package widgets

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	healerWillRefuse = "The healer will refuse."
	healerHPRow      = "HP"
	healerMaxHPRow   = "Max HP"
	healerLevelRow   = "Level"
)

var _ Widget = (*HealerTreatmentDialog)(nil)

// HealerTreatmentDialogConfig configures one nested treatment approval modal.
type HealerTreatmentDialogConfig struct {
	Keyboard *input.Keyboard
	Intent   HealerTreatmentIntent
}

// HealerTreatmentDialog presents target, effect, and charge before authority.
type HealerTreatmentDialog struct {
	keyboard  *input.Keyboard
	intent    HealerTreatmentIntent
	dismissed bool
	cancelled bool
	pending   *game_state.HealerServiceRequest

	window *amountWindow
}

// NewHealerTreatmentDialog creates a centered keyboard-only approval modal.
func NewHealerTreatmentDialog(config HealerTreatmentDialogConfig) *HealerTreatmentDialog {
	if config.Keyboard == nil {
		panic("HealerTreatmentDialog: keyboard is required")
	}
	validateHealerTreatmentIntent(config.Intent)
	intent := config.Intent
	if config.Intent.Recipient != nil {
		recipient := *config.Intent.Recipient
		intent.Recipient = &recipient
	}
	dialog := &HealerTreatmentDialog{
		keyboard:  config.Keyboard,
		intent:    intent,
		dismissed: false,
		cancelled: false,
		pending:   nil,
		window:    newAmountWindow(),
	}
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the nested approval modal.
func (d *HealerTreatmentDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update confirms or cancels the treatment approval.
func (d *HealerTreatmentDialog) Update() {
	if d.dismissed {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.cancelled = true
		d.dismissed = true

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		request := d.intent.Request
		d.pending = &request
		d.dismissed = true
	}
}

// ConsumePendingRequest returns and clears the approved treatment request.
func (d *HealerTreatmentDialog) ConsumePendingRequest() (game_state.HealerServiceRequest, bool) {
	if d.pending == nil {
		return emptyHealerServiceRequest(), false
	}
	request := *d.pending
	d.pending = nil

	return request, true
}

// GetBounds returns the centered modal bounds.
func (d *HealerTreatmentDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape closed the modal.
func (d *HealerTreatmentDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the modal can be removed.
func (d *HealerTreatmentDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the modal without approving treatment.
func (d *HealerTreatmentDialog) Dismiss() { d.dismissed = true }

// step describes the window: the service and who it is for in the title, what it
// changes as rows, and what it costs. A party that cannot pay still sends the
// request, which the healer refuses, so Enter stays and says what it will do.
func (d *HealerTreatmentDialog) step() amountStep {
	service := d.intent.Request.Service
	step := emptyAmountStep()
	step.verb = service.String()
	charge, kind, affordable := d.quote()
	paid := kind == game_state.HealerChargePaid
	if d.intent.IsAll() {
		step.title = service.String() + " All"
		step.unit = healerAllRecipients(d.intent.AllService.RecipientCount)
	} else {
		step.title = service.String() + " " + d.intent.Recipient.Name
	}
	if paid && !affordable {
		step.verb += tavernAnyway
		step.notes = shortOfGoldNotes("It costs", charge, int(d.intent.Gold), healerWillRefuse)

		return step
	}
	if !d.intent.IsAll() {
		step.rows, step.notes = healerEffect(service, *d.intent.Recipient)
	}
	if paid {
		step.rows = append(step.rows, amountRow{
			label: amountGoldRow, have: int(d.intent.Gold), then: int(d.intent.Gold) - charge,
		})

		return step
	}
	step.notes = append(step.notes, amountNote{text: freeCareText(kind), tone: amountPlain})

	return step
}

// quote gives the price, how it is charged and whether the party can pay it.
func (d *HealerTreatmentDialog) quote() (int, game_state.HealerChargeKind, bool) {
	if d.intent.IsAll() {
		all := d.intent.AllService

		return int(all.Charge), all.ChargeKind, all.Affordable
	}
	one := d.intent.Service

	return int(one.Charge), one.ChargeKind, one.Affordable
}

func freeCareText(kind game_state.HealerChargeKind) string {
	if kind == game_state.HealerChargeCharitable {
		return "Charitable care. No payment."
	}

	return "Free care. No payment."
}

func healerAllRecipients(count int) string {
	if count == 1 {
		return "1 party member needs it"
	}

	return fmt.Sprintf("%d party members need it", count)
}

// healerEffect gives the rows a treatment changes and a note for a status change.
func healerEffect(service references.HealerService, recipient game_state.HealerRecipientState,
) ([]amountRow, []amountNote) {
	projection := recipient.Treatments[int(service)]
	var rows []amountRow
	if service == references.HealerResurrect && projection.LevelAfter != recipient.Level {
		rows = append(rows, amountRow{label: healerLevelRow, have: int(recipient.Level), then: int(projection.LevelAfter)})
	}
	if service != references.HealerCure {
		rows = append(rows, amountRow{label: healerHPRow, have: int(recipient.HP), then: int(projection.HPAfter)})
		if projection.MaxHPAfter != recipient.MaxHP {
			rows = append(rows, amountRow{
				label: healerMaxHPRow, have: int(recipient.MaxHP), then: int(projection.MaxHPAfter),
			})
		}
	}
	switch {
	case recipient.Status != projection.StatusAfter:
		return rows, []amountNote{{
			text: fmt.Sprintf("%s becomes %s.", healerStatusName(recipient.Status),
				healerStatusName(projection.StatusAfter)),
			tone: amountPlain,
		}}
	case projection.StatusAfter != party_state.Good:
		return rows, []amountNote{{
			text: "Still " + healerStatusName(projection.StatusAfter) + ".", tone: amountPlain,
		}}
	default:
		return rows, nil
	}
}

func validateHealerTreatmentIntent(intent HealerTreatmentIntent) {
	if intent.Request.Service != intent.Service.Service ||
		intent.Request.Service != intent.AllService.Service {
		panic("HealerTreatmentDialog: service quotes do not match request")
	}
	if intent.IsAll() {
		if intent.Recipient != nil || intent.AllService.RecipientCount == 0 {
			panic("HealerTreatmentDialog: aggregate treatment requires eligible recipients")
		}

		return
	}
	if intent.Recipient == nil || intent.Recipient.CharacterIndex != intent.Request.CharacterIndex ||
		!intent.Recipient.Treatments[int(intent.Request.Service)].Eligibility.IsEligible() {
		panic("HealerTreatmentDialog: eligible detached recipient is required")
	}
}
