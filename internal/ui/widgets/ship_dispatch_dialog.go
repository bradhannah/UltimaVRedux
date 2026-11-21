package widgets

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	shipDispatchShowroomHint = "[Enter] Showroom"
	shipDispatchEscortHint   = "   [D] Escort"
)

var _ Widget = (*ShipDispatchDialog)(nil)

// ShipDispatchDialogConfig configures the first-delivery choice shown after a purchase refresh.
type ShipDispatchDialogConfig struct {
	Keyboard *input.Keyboard
	State    game_state.ShipShopState
}

// ShipDispatchDialog presents the refreshed berth and chooses shopping, escort, or town.
type ShipDispatchDialog struct {
	keyboard         *input.Keyboard
	state            game_state.ShipShopState
	dismissed        bool
	continueShopping bool
	pendingTravel    *ShipTravelIntent
	window           *amountWindow
}

// NewShipDispatchDialog creates a centered keyboard-only first-delivery choice.
func NewShipDispatchDialog(config ShipDispatchDialogConfig) *ShipDispatchDialog {
	if config.Keyboard == nil {
		panic("ShipDispatchDialog: keyboard is required")
	}
	validateShipShopState(config.State)
	if !shipBerthHasVessel(config.State.Berth) {
		panic("ShipDispatchDialog: refreshed vessel berth is required")
	}
	dialog := &ShipDispatchDialog{ //nolint:exhaustruct_v5 // Transient choice fields start empty.
		keyboard: config.Keyboard,
		state:    config.State,
		window:   newAmountWindow(),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.window.set(dialog.step())

	return dialog
}

// Draw renders the opaque semantic merchant dispatch modal.
func (d *ShipDispatchDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.window.draw(screen)
}

// Update chooses exactly one post-delivery action after release debounce.
func (d *ShipDispatchDialog) Update() {
	if d.dismissed {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyD) && d.travelAvailable() {
		intent := ShipTravelIntent{Review: d.state.Travel, Preview: false}
		d.pendingTravel = &intent
		d.dismissed = true

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		d.continueShopping = true
		d.dismissed = true
	}
}

// ConsumePendingTravelIntent returns and clears the one reviewed escort choice.
func (d *ShipDispatchDialog) ConsumePendingTravelIntent() (ShipTravelIntent, bool) {
	if d.pendingTravel == nil {
		return emptyShipTravelIntent(), false
	}
	intent := *d.pendingTravel
	d.pendingTravel = nil

	return intent, true
}

// ShouldContinueShopping reports whether Enter acknowledged the refreshed showroom.
func (d *ShipDispatchDialog) ShouldContinueShopping() bool { return d.continueShopping }

// GetBounds returns the centered dispatch modal bounds.
func (d *ShipDispatchDialog) GetBounds() image.Rectangle {
	return d.window.bounds()
}

// IsDismissed reports whether the workflow can remove the modal.
func (d *ShipDispatchDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the dispatch modal without choosing an action.
func (d *ShipDispatchDialog) Dismiss() { d.dismissed = true }

// step describes the window: the vessel that was ordered and where it waits,
// and whether the party can be escorted there. Enter and D are all it offers,
// so its footer replaces the usual Enter and Esc.
func (d *ShipDispatchDialog) step() amountStep {
	berth := d.state.Berth
	step := emptyAmountStep()
	step.title = shipVehicleName(berth.VehicleType) + " Dispatched"
	step.unit = "To " + d.state.Travel.HarbourName
	skiffs := "A standalone skiff."
	if berth.VehicleType == references.FrigateVehicle {
		skiffs = fmt.Sprintf("Skiffs aboard: %d of %d", berth.SkiffCount, game_state.ShipShopMaximumSkiffs)
	}
	step.notes = []amountNote{
		{text: "Berth: " + shipBerthLabel(berth), tone: amountPlain},
		{text: skiffs, tone: amountPlain},
		{text: "", tone: amountPlain},
	}
	step.footer = shipDispatchShowroomHint
	if d.travelAvailable() {
		step.notes = append(step.notes, amountNote{
			text: fmt.Sprintf("An escort there takes %s.", countedNoun(d.state.Travel.TimeCostMinutes, "minute")),
			tone: amountPlain,
		})
		step.footer += shipDispatchEscortHint
	} else {
		step.notes = append(step.notes, amountNote{text: "No escort is available now.", tone: amountPlain})
	}

	return step
}

func (d *ShipDispatchDialog) travelAvailable() bool {
	return !d.state.Travel.BlockReason.IsBlocked() && d.state.Travel.Fingerprint != ""
}

func shipBerthHasVessel(berth game_state.ShipBerthReview) bool {
	switch berth.State {
	case game_state.ShipBerthPendingFrigate, game_state.ShipBerthPendingSkiff,
		game_state.ShipBerthMooredFrigate, game_state.ShipBerthMooredSkiff:
		return true
	case game_state.ShipBerthEmpty, game_state.ShipBerthObstructed:
		return false
	default:
		panic("shipBerthHasVessel: unknown berth state")
	}
}
