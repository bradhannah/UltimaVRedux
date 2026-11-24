package widgets

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestHorsePurchaseDialogShowsOnlyHorsePriceAndGoldConsequence(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	intent := testHorsePurchaseIntent(testHorseShopState(), true)
	dialog := NewHorsePurchaseDialog(HorsePurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   intent,
	})
	words := dialog.window.flatText()

	assert.True(t, dialog.IsPreview())
	assert.Equal(t, "PREVIEW - Buy Horse Have Then Gold 500 -170 330 [Enter] Buy [Esc] Back", words)
	requireAmountWindowHoldsItsText(t, dialog.window, "horse")

	visible := strings.ToLower(words)
	for _, forbidden := range horseDialogForbiddenDetails() {
		assert.NotContains(t, visible, forbidden)
	}
	assert.NotContains(t, visible, "mouse")
	assert.NotContains(t, visible, "click")
}

func TestHorsePurchaseDialogUnaffordableRemainsConfirmable(t *testing.T) {
	state := testHorseShopState()
	setHorseShopGold(&state, 50)
	intent := testHorsePurchaseIntent(state, false)
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		keyboard := newWidgetTestKeyboard(0)
		dialog := NewHorsePurchaseDialog(HorsePurchaseDialogConfig{Keyboard: keyboard, Intent: intent})

		assert.Equal(t, "Buy Horse Thou hast not enough gold. It costs 170 and the party has 50. "+
			"Buying anyway ends the talk. [Enter] Buy anyway [Esc] Back", dialog.window.flatText())
		requireAmountWindowHoldsItsText(t, dialog.window, fmt.Sprintf("%dx%d", size.X, size.Y))

		pressHorsePurchaseKey(dialog, keyboard, ebiten.KeyEnter)
		assert.True(t, dialog.IsDismissed())
		request, ok := dialog.ConsumePendingPurchaseRequest()
		require.True(t, ok)
		assert.Equal(t, intent.Request, request)
	}
}

func TestHorsePurchaseDialogApproveBackAndSingleRequestLifecycle(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	parent := newTestHorseShopDialog(keyboard, testHorseShopState())
	pressHorseShopKey(parent, keyboard, ebiten.KeyEnter)
	intent, ok := parent.ConsumePendingPurchaseIntent()
	require.True(t, ok)

	back := NewHorsePurchaseDialog(HorsePurchaseDialogConfig{Keyboard: keyboard, Intent: intent})
	pressHorsePurchaseKey(back, keyboard, ebiten.KeyEscape)
	assert.True(t, back.IsDismissed())
	assert.True(t, back.IsCancelled())
	assert.False(t, back.HasPendingPurchaseRequest())
	parent.CancelPurchaseApproval()
	assert.Equal(t, HorseShopBrowse, parent.GetPhase())

	pressHorseShopKey(parent, keyboard, ebiten.KeyEnter)
	intent, ok = parent.ConsumePendingPurchaseIntent()
	require.True(t, ok)
	approved := NewHorsePurchaseDialog(HorsePurchaseDialogConfig{Keyboard: keyboard, Intent: intent})
	pressHorsePurchaseKey(approved, keyboard, ebiten.KeyNumpadEnter)
	assert.True(t, approved.IsDismissed())
	assert.False(t, approved.IsCancelled())
	request, ok := approved.ConsumePendingPurchaseRequest()
	require.True(t, ok)
	assert.Equal(t, intent.Request, request)
	pressHorsePurchaseKey(approved, keyboard, ebiten.KeyEnter)
	_, duplicate := approved.ConsumePendingPurchaseRequest()
	assert.False(t, duplicate)

	parent.AcceptPurchaseApproval(request)
	assert.Equal(t, HorseShopAwaitingResult, parent.GetPhase())
}

func TestHorsePurchaseDialogSemanticBackgroundAndResizeSafety(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	intent := testHorsePurchaseIntent(testHorseShopState(), true)
	dialog := NewHorsePurchaseDialog(HorsePurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   intent,
	})
	lowBounds := dialog.GetBounds()
	lowText := dialog.window.flatText()
	setMerchantShellTestResolution(t, 1280, 720)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Dispose)
	dialog.Draw(screen)

	assert.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	assert.Greater(t, dialog.GetBounds().Dy(), lowBounds.Dy())
	assert.Equal(t, lowText, dialog.window.flatText())
	assert.Equal(t, intent, dialog.GetIntent())
	requireOutputVerticallyCentered(t, dialog.window.frame.bodyText, dialog.window.body)
	headerBackground := image.Pt(
		dialog.window.frame.headerBounds.Min.X+dialog.window.frame.pixelUnit()*4,
		dialog.window.frame.headerBounds.Min.Y,
	)
	requireColorRGBA(t, DialogRuleColor, screen.At(headerBackground.X, headerBackground.Y))
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlKeyColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlHintColor)
}

func TestHorsePurchaseDialogStrictDetachedIntentContracts(t *testing.T) {
	valid := HorsePurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   testHorsePurchaseIntent(testHorseShopState(), false),
	}
	tests := []struct {
		name   string
		mutate func(*HorsePurchaseDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *HorsePurchaseDialogConfig) { config.Keyboard = nil }},
		{name: "request price", mutate: func(config *HorsePurchaseDialogConfig) {
			config.Intent.Request.ReviewedPrice++
		}},
		{name: "request gold", mutate: func(config *HorsePurchaseDialogConfig) {
			config.Intent.Request.ReviewedGoldBefore++
		}},
		{name: "request placement", mutate: func(config *HorsePurchaseDialogConfig) {
			config.Intent.Request.ReviewedPlacement.Position.X++
		}},
		{name: "gold consequence", mutate: func(config *HorsePurchaseDialogConfig) {
			config.Intent.Quote.GoldAfterPayment++
		}},
		{name: "stable closed", mutate: func(config *HorsePurchaseDialogConfig) {
			config.Intent.Quote.Placement.BlockStatus = game_state.HorseShopBlockMapUnitCapacity
			config.Intent.Request.ReviewedPlacement = config.Intent.Quote.Placement
		}},
		{name: "deferred live", mutate: func(config *HorsePurchaseDialogConfig) {
			config.Intent.Quote.Placement.BlockStatus = game_state.HorseShopBlockAvailabilityDeferred
			config.Intent.Request.ReviewedPlacement = config.Intent.Quote.Placement
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.Panics(t, func() { NewHorsePurchaseDialog(config) })
		})
	}
}

func testHorsePurchaseIntent(state game_state.HorseShopState, preview bool) HorsePurchaseIntent {
	return HorsePurchaseIntent{
		Quote:   state.Quote,
		Request: horsePurchaseRequest(state),
		Preview: preview,
	}
}

func pressHorsePurchaseKey(dialog *HorsePurchaseDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}
