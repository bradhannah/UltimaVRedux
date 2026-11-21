package widgets

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestShipTravelDialogDisplaysAndConfirmsExactReviewedTravel(t *testing.T) {
	state := testPendingFrigateShipShopState(true)
	intent := ShipTravelIntent{Review: state.Travel, Preview: true}
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewShipTravelDialog(ShipTravelDialogConfig{Keyboard: keyboard, Intent: intent})
	body := compactShipText(dialog.window.flatText())

	require.Equal(t, "PREVIEW - Escort the Frigate", compactShipText(dialog.GetTitleText()))
	require.Contains(t, body, "To Jhelom Harbour")
	require.Contains(t, body, "Berth: Pending arrival")
	require.Contains(t, body, "Travel time: 3 minutes")
	require.Contains(t, body, "The party leaves town beside it.")
	require.Equal(t, "[Enter] Travel [Esc] Back", compactShipText(dialog.GetFooterGuidance()))
	requireAmountWindowHoldsItsText(t, dialog.window, "travel")
	require.Equal(t, intent, dialog.GetIntent())
	require.True(t, dialog.IsPreview())

	visible := strings.ToLower(body)
	for _, forbidden := range shipDialogForbiddenDetails() {
		require.NotContains(t, visible, forbidden)
	}

	pressShipTravelKey(dialog, keyboard, ebiten.KeyNumpadEnter)
	require.True(t, dialog.IsDismissed())
	require.False(t, dialog.IsCancelled())
	review, ok := dialog.ConsumePendingTravelReview()
	require.True(t, ok)
	require.Equal(t, state.Travel, review)
	pressShipTravelKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingTravelReview()
	require.False(t, duplicate)
}

func TestShipTravelDialogEscapeBackAndStrictAvailableReview(t *testing.T) {
	state := testPendingFrigateShipShopState(true)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewShipTravelDialog(ShipTravelDialogConfig{
		Keyboard: keyboard,
		Intent:   ShipTravelIntent{Review: state.Travel, Preview: false},
	})
	pressShipTravelKey(dialog, keyboard, ebiten.KeyEscape)
	require.True(t, dialog.IsDismissed())
	require.True(t, dialog.IsCancelled())
	require.False(t, dialog.HasPendingTravelReview())

	require.Panics(t, func() {
		NewShipTravelDialog(ShipTravelDialogConfig{
			Keyboard: nil,
			Intent:   ShipTravelIntent{Review: state.Travel, Preview: false},
		})
	})
	blocked := state.Travel
	blocked.BlockReason = game_state.HarbourTravelBlockArrivalOccupied
	require.Panics(t, func() {
		NewShipTravelDialog(ShipTravelDialogConfig{
			Keyboard: newWidgetTestKeyboard(0),
			Intent:   ShipTravelIntent{Review: blocked, Preview: false},
		})
	})
}

func TestShipTravelDialogSemanticBackingAndResizePreserveReview(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	state := testPendingFrigateShipShopState(true)
	intent := ShipTravelIntent{Review: state.Travel, Preview: false}
	dialog := NewShipTravelDialog(ShipTravelDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   intent,
	})
	lowBounds := dialog.GetBounds()
	lowBody := compactShipText(dialog.GetBodyText())

	setResponsiveDialogTestResolution(t, 1280, 720)
	screen := ebiten.NewImage(1280, 720)
	dialog.Draw(screen)
	require.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	require.Greater(t, dialog.GetBounds().Dy(), lowBounds.Dy())
	require.Equal(t, lowBody, compactShipText(dialog.GetBodyText()))
	require.Equal(t, intent, dialog.GetIntent())
	requireImageRegionContainsColor(t, screen, dialog.GetBodyBounds(), MerchantDialogBackgroundColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlKeyColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlHintColor)
}

func pressShipTravelKey(dialog *ShipTravelDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}
