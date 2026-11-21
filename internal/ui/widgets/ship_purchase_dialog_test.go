package widgets

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestShipPurchaseDialogDisplaysVesselDockSkiffsAndPayment(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	state := testShipShopState()
	intent := testShipPurchaseIntent(state, state.FrigateQuote, true)
	dialog := NewShipPurchaseDialog(ShipPurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   intent,
	})
	flat := dialog.window.flatText()

	require.Equal(t, "PREVIEW - Buy Frigate With 2 skiffs aboard Have Then Gold 1000 -600 400 "+
		"Delivered to Jhelom Harbour. [Enter] Buy [Esc] Back", flat)
	require.True(t, dialog.IsPreview())
	requireAmountWindowHoldsItsText(t, dialog.window, "frigate")

	visible := strings.ToLower(flat)
	for _, forbidden := range append(shipDialogForbiddenDetails(), "vessel:", "action:", "dock:", "escort") {
		require.NotContains(t, visible, forbidden)
	}
}

func TestShipPurchaseDialogLoadSkiffSteppingAndUnaffordableRemainConfirmable(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	state := testPendingFrigateShipShopState(true)
	dialog := NewShipPurchaseDialog(ShipPurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   testShipPurchaseIntent(state, state.SkiffQuote, false),
	})
	flat := dialog.window.flatText()
	require.Contains(t, flat, "Buy Skiffs 200 gold each Skiffs < 1 > of 5")
	require.Contains(t, flat, "Have Then Aboard 2 +1 3 Gold 1000 -200 800")
	require.Contains(t, flat, "[Left/Right/0-9] Skiffs [M] Most [Enter] Buy 1 [Esc] Back")
	pressShipPurchaseKey(dialog, dialog.keyboard, ebiten.KeyRight)
	pressShipPurchaseKey(dialog, dialog.keyboard, ebiten.KeyRight)
	require.Equal(t, 3, dialog.GetQuantity())
	require.Contains(t, dialog.window.flatText(), "Aboard 2 +3 5 Gold 1000 -600 400")
	pressShipPurchaseKey(dialog, dialog.keyboard, ebiten.Key2)
	require.Equal(t, 2, dialog.GetQuantity(), "a digit types the amount")
	pressShipPurchaseKey(dialog, dialog.keyboard, ebiten.KeyM)
	require.Equal(t, 5, dialog.GetQuantity(), "maximum is bounded by the party's current gold")
	requireAmountWindowHoldsItsText(t, dialog.window, "skiffs")

	poor := testShipShopState()
	setShipShopGold(&poor, 100)
	intent := testShipPurchaseIntent(poor, poor.FrigateQuote, false)
	keyboard := newWidgetTestKeyboard(0)
	dialog = NewShipPurchaseDialog(ShipPurchaseDialogConfig{Keyboard: keyboard, Intent: intent})
	require.Equal(t, "Buy Frigate With 2 skiffs aboard Not enough gold. It costs 600 and the party has 100. "+
		"Asking anyway ends the talk. [Enter] Ask anyway [Esc] Back", dialog.window.flatText())
	requireAmountWindowHoldsItsText(t, dialog.window, "no gold")

	pressShipPurchaseKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
	request, ok := dialog.ConsumePendingPurchaseRequest()
	require.True(t, ok)
	require.Equal(t, intent.Request, request)
}

func TestShipPurchaseDialogConfirmCancelAndSingleRequestLifecycle(t *testing.T) {
	intent := testShipPurchaseIntent(testShipShopState(), testShipShopState().SkiffQuote, false)

	backKeyboard := newWidgetTestKeyboard(0)
	back := NewShipPurchaseDialog(ShipPurchaseDialogConfig{Keyboard: backKeyboard, Intent: intent})
	pressShipPurchaseKey(back, backKeyboard, ebiten.KeyEscape)
	require.True(t, back.IsDismissed())
	require.True(t, back.IsCancelled())
	require.False(t, back.HasPendingPurchaseRequest())

	confirmKeyboard := newWidgetTestKeyboard(0)
	confirmed := NewShipPurchaseDialog(ShipPurchaseDialogConfig{Keyboard: confirmKeyboard, Intent: intent})
	pressShipPurchaseKey(confirmed, confirmKeyboard, ebiten.KeyNumpadEnter)
	require.True(t, confirmed.IsDismissed())
	require.False(t, confirmed.IsCancelled())
	request, ok := confirmed.ConsumePendingPurchaseRequest()
	require.True(t, ok)
	require.Equal(t, intent.Request, request)
	pressShipPurchaseKey(confirmed, confirmKeyboard, ebiten.KeyEnter)
	_, duplicate := confirmed.ConsumePendingPurchaseRequest()
	require.False(t, duplicate)
}

func TestShipPurchaseDialogSemanticBackingAndResizePreserveContent(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	state := testPendingFrigateShipShopState(true)
	intent := testShipPurchaseIntent(state, state.SkiffQuote, true)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewShipPurchaseDialog(ShipPurchaseDialogConfig{
		Keyboard: keyboard,
		Intent:   intent,
	})
	pressShipPurchaseKey(dialog, keyboard, ebiten.KeyRight)
	pressShipPurchaseKey(dialog, keyboard, ebiten.KeyRight)
	lowBounds := dialog.GetBounds()
	lowBody := compactShipText(dialog.GetBodyText())

	setResponsiveDialogTestResolution(t, 1280, 720)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Dispose)
	dialog.Draw(screen)
	require.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	require.Greater(t, dialog.GetBounds().Dy(), lowBounds.Dy())
	require.Equal(t, lowBody, compactShipText(dialog.GetBodyText()))
	require.Equal(t, intent, dialog.GetIntent())
	require.Equal(t, 3, dialog.GetQuantity())
	requireImageRegionContainsColor(t, screen, dialog.GetBodyBounds(), MerchantDialogBackgroundColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlKeyColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlHintColor)
	pressShipPurchaseKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingPurchaseRequest()
	require.True(t, ok)
	require.Equal(t, 3, request.Quantity)
}

func TestShipPurchaseDialogStrictDetachedIntentContracts(t *testing.T) {
	state := testShipShopState()
	valid := ShipPurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   testShipPurchaseIntent(state, state.FrigateQuote, false),
	}
	tests := []struct {
		name   string
		mutate func(*ShipPurchaseDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *ShipPurchaseDialogConfig) { config.Keyboard = nil }},
		{name: "blocked", mutate: func(config *ShipPurchaseDialogConfig) {
			config.Intent.Quote.BlockReason = game_state.ShipPurchaseBlockBerthOccupied
		}},
		{name: "request price", mutate: func(config *ShipPurchaseDialogConfig) {
			config.Intent.Request.ReviewedPrice++
		}},
		{name: "request action", mutate: func(config *ShipPurchaseDialogConfig) {
			config.Intent.Request.Action = game_state.ShipPurchaseCreateStandaloneSkiff
		}},
		{name: "request fingerprint", mutate: func(config *ShipPurchaseDialogConfig) {
			config.Intent.Request.ReviewedFingerprint = "different"
		}},
		{name: "gold consequence", mutate: func(config *ShipPurchaseDialogConfig) {
			config.Intent.Quote.GoldAfterPayment++
		}},
		{name: "harbour", mutate: func(config *ShipPurchaseDialogConfig) {
			config.Intent.HarbourName = ""
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.Panics(t, func() { NewShipPurchaseDialog(config) })
		})
	}
}

func testShipPurchaseIntent(
	state game_state.ShipShopState,
	quote game_state.ShipPurchaseQuote,
	preview bool,
) ShipPurchaseIntent {
	return ShipPurchaseIntent{
		Quote:       quote,
		Request:     shipPurchaseRequestFromState(state, quote),
		HarbourName: state.Travel.HarbourName,
		Travel:      state.Travel,
		Preview:     preview,
	}
}

func pressShipPurchaseKey(dialog *ShipPurchaseDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func TestShipPurchaseDialogPhysicalFixedVesselsKeepReviewedRequest(t *testing.T) {
	state := testShipShopState()
	setShipShopGold(&state, 100)
	for _, quote := range []game_state.ShipPurchaseQuote{state.FrigateQuote, state.SkiffQuote} {
		t.Run(shipVehicleName(quote.VehicleType), func(t *testing.T) {
			keys := newPhysicalQuantityKeys()
			intent := testShipPurchaseIntent(state, quote, false)
			dialog := NewShipPurchaseDialog(ShipPurchaseDialogConfig{Keyboard: keys.keyboard, Intent: intent})
			require.False(t, dialog.GetApprovalView().Affordable)
			for _, key := range []ebiten.Key{ebiten.KeyRight, ebiten.KeyLeft, ebiten.KeyM} {
				keys.held[key] = true
				for range 60 {
					keys.tick(dialog)
				}
				require.Equal(t, 1, dialog.GetQuantity())
				keys.release(dialog)
			}
			keys.held[ebiten.KeyEnter] = true
			keys.tick(dialog)
			request, ok := dialog.ConsumePendingPurchaseRequest()
			require.True(t, ok)
			require.Equal(t, intent.Request, request, "fixed unaffordable attempts still reach authority")
		})
	}
}
