package widgets

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	shipTravelStartPercentX = 0.28
	shipTravelEndPercentX   = 0.72
	shipTravelStartPercentY = 0.22
	shipTravelEndPercentY   = 0.78
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
	frame     *merchantModalFrame

	headerOutput *text.Output
	bodyOutput   *text.Output
	footerOutput *text.Output
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
		frame:    newShipMerchantModalFrame(shipTravelPlacement()),
	}
	config.Keyboard.PrepareForTransition(0)
	dialog.ensureLayout()

	return dialog
}

// Draw renders the opaque semantic merchant travel modal.
func (d *ShipTravelDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()
	d.frame.draw(screen)
	d.frame.drawOutputs(screen, d.headerOutput, d.bodyOutput, d.footerOutput)
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

// GetFooterGuidance returns the keyboard-only travel controls.
func (d *ShipTravelDialog) GetFooterGuidance() string { return "[Enter] Travel   [Esc] Back" }

// GetBounds returns the centered travel modal bounds.
func (d *ShipTravelDialog) GetBounds() image.Rectangle {
	d.ensureLayout()

	return d.frame.bounds()
}

// IsCancelled reports whether Escape closed the approval.
func (d *ShipTravelDialog) IsCancelled() bool { return d.cancelled }

// IsDismissed reports whether the workflow can remove the modal.
func (d *ShipTravelDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the travel modal without emitting a review.
func (d *ShipTravelDialog) Dismiss() { d.dismissed = true }

func (d *ShipTravelDialog) ensureLayout() {
	if !d.frame.ensureLayout() {
		return
	}
	d.headerOutput, d.bodyOutput, d.footerOutput = newShipMerchantModalOutputs(d.frame)
	d.populateOutputs()
}

func (d *ShipTravelDialog) populateOutputs() {
	review := d.intent.Review
	d.headerOutput.Reset()
	titleLine := d.headerOutput.AppendLine().WithAlignment(etext.AlignCenter)
	if d.intent.Preview {
		titleLine.Text("PREVIEW - ", DialogStatusWarningColor)
	}
	titleLine.Text("Harbour Escort", DialogHeaderColor).Done()
	body := d.bodyOutput
	body.Reset()
	body.AppendLine().LightGray("Harbour: ").White(review.HarbourName).Done()
	berth := "Moored"
	if review.PendingDelivery {
		berth = "Pending arrival"
	}
	body.AppendLine().LightGray("Berth: ").White(berth).Done()
	body.AppendLine().LightGray("Vessel: ").White(shipVehicleName(review.VehicleType)).Done()
	body.AppendLine().LightGray("Travel time: ").Cyanf("%d minutes", review.TimeCostMinutes).Done()
	body.AppendLine().Text(
		fmt.Sprintf("Escort to %s and leave town beside the vessel.", review.HarbourName),
		DialogValueColor,
	).WordWrap().Done()
	d.footerOutput.Reset()
	appendControlGuidance(d.footerOutput, d.GetFooterGuidance(), d.frame.footerText.Dx())
}

func shipTravelPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: shipTravelStartPercentX,
		EndPercentX:   shipTravelEndPercentX,
		StartPercentY: shipTravelStartPercentY,
		EndPercentY:   shipTravelEndPercentY,
	}
}
