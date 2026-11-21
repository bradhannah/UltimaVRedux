package widgets

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*ShipTravelDialog)(nil)

// ShipTravelDialogConfig configures one persistent harbour-travel approval.
type ShipTravelDialogConfig struct {
	Keyboard *input.Keyboard
	Intent   ShipTravelIntent
}

// ShipTravelDialog displays an available detached travel review and emits it unchanged.
type ShipTravelDialog struct {
	keyboard  *input.Keyboard
	intent    ShipTravelIntent
	dismissed bool
	cancelled bool
	pending   *game_state.HarbourTravelReview
	window    *amountWindow
}

// NewShipTravelDialog creates a centered keyboard-only harbour approval.
func NewShipTravelDialog(config ShipTravelDialogConfig) *ShipTravelDialog {
	if config.Keyboard == nil {
		panic("ShipTravelDialog: keyboard is required")
	}
	validateShipTravelReview(config.Intent.Review, true)
	dialog := &ShipTravelDialog{ //nolint:exhaustruct_v5 // Transient lifecycle fields start empty.
		keyboard: config.Keyboard,
		intent:   config.Intent,
		window:   newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque semantic merchant travel modal.
func (d *ShipTravelDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update confirms or cancels the exact reviewed travel after release debounce.
func (d *ShipTravelDialog) Update() {
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
		review := d.intent.Review
		d.pending = &review
		d.dismissed = true
	}
}

// ConsumePendingTravelReview returns and clears the one confirmed comparison-only review.
func (d *ShipTravelDialog) ConsumePendingTravelReview() (game_state.HarbourTravelReview, bool) {
	if d.pending == nil {
		return emptyHarbourTravelReview(), false
	}
	review := *d.pending
	d.pending = nil

	return review, true
}

// GetBounds returns the centered travel modal bounds.
func (d *ShipTravelDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsCancelled reports whether Escape closed the approval.
func (d *ShipTravelDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the workflow can remove the modal.
func (d *ShipTravelDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the travel modal without emitting a review.
func (d *ShipTravelDialog) Dismiss() { d.dismissed = true }

// step describes the window: the vessel to escort, where, how long it takes, and
// that the party leaves town beside it.
func (d *ShipTravelDialog) step() amountStep {
	review := d.intent.Review
	berth := "Moored"
	if review.PendingDelivery {
		berth = "Pending arrival"
	}
	step := emptyAmountStep()
	step.preview, step.verb = d.intent.Preview, "Travel"
	step.title = "Escort the " + shipVehicleName(review.VehicleType)
	step.unit = "To " + review.HarbourName
	step.notes = []amountNote{
		{text: "Berth: " + berth, tone: amountPlain},
		{text: "Travel time: " + countedNoun(review.TimeCostMinutes, "minute"), tone: amountPlain},
		{text: "", tone: amountPlain},
		{text: "The party leaves town beside it.", tone: amountPlain},
	}

	return step
}
