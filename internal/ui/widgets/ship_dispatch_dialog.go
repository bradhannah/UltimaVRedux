package widgets

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	shipDispatchStartPercentX = 0.25
	shipDispatchEndPercentX   = 0.75
	shipDispatchStartPercentY = 0.18
	shipDispatchEndPercentY   = 0.82
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
	frame            *merchantModalFrame

	headerOutput *text.Output
	bodyOutput   *text.Output
	footerOutput *text.Output
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
		frame:    newShipMerchantModalFrame(shipDispatchPlacement()),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.ensureLayout()

	return dialog
}

// Draw renders the opaque semantic merchant dispatch modal.
func (d *ShipDispatchDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
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

// GetFooterGuidance returns the available keyboard-only dispatch choices.
func (d *ShipDispatchDialog) GetFooterGuidance() string {
	if d.travelAvailable() {
		return "[Enter] Return to showroom   [D] Escort"
	}

	return "[Enter] Return to showroom"
}

// GetBounds returns the centered dispatch modal bounds.
func (d *ShipDispatchDialog) GetBounds() image.Rectangle {
	d.ensureLayout()

	return d.frame.bounds()
}

// IsDismissed reports whether the workflow can remove the modal.
func (d *ShipDispatchDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the dispatch modal without choosing an action.
func (d *ShipDispatchDialog) Dismiss() { d.dismissed = true }

func (d *ShipDispatchDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	d.headerOutput, d.bodyOutput, d.footerOutput = newShipMerchantModalOutputs(d.frame)
	d.populateOutputs()
}

func (d *ShipDispatchDialog) populateOutputs() {
	d.headerOutput.Reset()
	d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter).
		Text("Vessel Dispatched", DialogHeaderColor).Done()
	body := d.bodyOutput
	body.Reset()
	body.AppendLine().LightGray("Harbour: ").White(d.state.Travel.HarbourName).Done()
	body.AppendLine().LightGray("Berth: ").White(shipBerthLabel(d.state.Berth)).Done()
	body.AppendLine().LightGray("Vessel: ").White(shipVehicleName(d.state.Berth.VehicleType)).Done()
	if d.state.Berth.VehicleType == references.FrigateVehicle {
		body.AppendLine().LightGray("Skiffs aboard: ").Cyanf(
			"%d/%d", d.state.Berth.SkiffCount, game_state.ShipShopMaximumSkiffs,
		).Done()
	} else {
		body.AppendLine().LightGray("Skiffs: ").Cyan("1 standalone").Done()
	}
	escort := fmt.Sprintf("Escort: unavailable, %d minutes", d.state.Travel.TimeCostMinutes)
	color := DialogEmptyColor
	if d.travelAvailable() {
		escort = fmt.Sprintf("[D] Escort available, %d minutes", d.state.Travel.TimeCostMinutes)
		color = DialogDeltaPositive
	}
	body.AppendLine().Text(escort, color).Done()
	d.footerOutput.Reset()
	appendControlGuidance(d.footerOutput, d.GetFooterGuidance(), d.frame.footerText.Dx())
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

func shipDispatchPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: shipDispatchStartPercentX,
		EndPercentX:   shipDispatchEndPercentX,
		StartPercentY: shipDispatchStartPercentY,
		EndPercentY:   shipDispatchEndPercentY,
	}
}
