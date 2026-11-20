package widgets

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestArmsTradeDialogOrdinaryBuyQuantityAndZeroMaxRequest(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	quote := testArmsShopState().BuyQuotes[0]
	quote.MaxQuantity = 3
	dialog := newTestArmsTradeDialog(keyboard, ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: quote})

	require.Equal(t, uint16(1), dialog.GetQuantity())
	body := strings.Join(strings.Fields(dialog.GetBodyText()), " ")
	require.Contains(t, body, "Quantity < 1 > of 3")
	require.Contains(t, body, "50 gold each")
	require.Contains(t, body, "Have Then Dagger 8 9 Gold 500 450")
	require.Contains(t, body, "Faulinei may take more.")
	require.Equal(t, "Buy Dagger", dialog.headerOutput.GetOutputStr(false))
	require.Contains(t, dialog.GetFooterGuidance(), "[M] Most")
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyRight)
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyRight)
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyRight)
	require.Equal(t, uint16(3), dialog.GetQuantity())
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyLeft)
	require.Equal(t, uint16(2), dialog.GetQuantity())
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, ArmsTradeRequest{Mode: ArmsShopBuy, Buy: game_state.ArmsBuyRequest{
		Equipment: quote.Equipment, Quantity: 2,
	}}, request)
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingRequest()
	require.False(t, duplicate)

	zeroMax := quote
	zeroMax.MaxQuantity = 0
	zeroDialog := newTestArmsTradeDialog(newWidgetTestKeyboard(0), ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: zeroMax})
	pressArmsTradeKey(zeroDialog, zeroDialog.keyboard, ebiten.KeyEnter)
	request, ok = zeroDialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, uint16(1), request.Buy.Quantity)
}

func TestArmsTradeDialogAmmoBuyIsFixedRefillAndStillRequestsAt99(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	quote := testArmsShopState().BuyQuotes[1]
	quote.QuantityBefore = 99
	quote.MaxQuantity = 0
	dialog := newTestArmsTradeDialog(keyboard, ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: quote})

	require.Contains(t, dialog.GetBodyText(), "Refills to 99 for 10 gold")
	require.Contains(t, strings.Join(strings.Fields(dialog.GetBodyText()), " "), "Arrows 99 99")
	require.NotContains(t, dialog.GetFooterGuidance(), "Quantity")
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyRight)
	require.Equal(t, uint16(1), dialog.GetQuantity())
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, game_state.ArmsBuyRequest{Equipment: references.Arrows, Quantity: 1}, request.Buy)
}

func TestArmsTradeDialogSellQuantitySellAllAndCancellation(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	quote := game_state.ArmsSellQuote{
		Equipment: references.Dagger, BasePrice: 50, UnitOffer: 40, Gold: 500,
		QuantityBefore: 5, MaxQuantity: 3, Sellable: true,
	}
	dialog := newTestArmsTradeDialog(keyboard, ArmsTradeIntent{Mode: ArmsShopSell, SellQuote: quote})
	require.Contains(t, dialog.GetFooterGuidance(), "[M] Most")
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyA)
	require.Equal(t, uint16(3), dialog.GetQuantity())
	body := strings.Join(strings.Fields(dialog.GetBodyText()), " ")
	require.Contains(t, body, "Gold 500 620")
	require.Contains(t, body, "Dagger 5 2")
	require.NotContains(t, body, "Faulinei", "a sale is paid to the party, not taken from it")
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, game_state.ArmsSellRequest{Equipment: references.Dagger, Quantity: 3}, request.Sell)

	cancelled := newTestArmsTradeDialog(newWidgetTestKeyboard(0), ArmsTradeIntent{Mode: ArmsShopSell, SellQuote: quote})
	pressArmsTradeKey(cancelled, cancelled.keyboard, ebiten.KeyEscape)
	require.True(t, cancelled.IsCancelled())
	require.True(t, cancelled.IsDismissed())
	require.False(t, cancelled.HasPendingRequest())
}

func TestArmsTradeDialogRefusalKindsNeverEmitRequests(t *testing.T) {
	tests := []struct {
		name     string
		quote    game_state.ArmsSellQuote
		kind     ArmsTradeRefusalKind
		terminal bool
	}{
		{
			name:  "used ammunition",
			quote: game_state.ArmsSellQuote{Equipment: references.Arrows, BasePrice: 10, Gold: 500, QuantityBefore: 20},
			kind:  ArmsTradeUsedAmmunitionRefusal, terminal: true,
		},
		{
			name:  "zero price",
			quote: game_state.ArmsSellQuote{Equipment: references.Ankh, Gold: 500, QuantityBefore: 1},
			kind:  ArmsTradeZeroPriceRefusal,
		},
		{
			name: "gold capacity",
			quote: game_state.ArmsSellQuote{
				Equipment: references.Bow, BasePrice: 100, UnitOffer: 80, Gold: 9999,
				QuantityBefore: 1, MaxQuantity: 0, Sellable: true,
			},
			kind: ArmsTradeGoldCapacityRefusal,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
				t.Run(key.String(), func(t *testing.T) {
					keyboard := newWidgetTestKeyboard(0)
					dialog := newTestArmsTradeDialog(
						keyboard,
						ArmsTradeIntent{Mode: ArmsShopSell, SellQuote: test.quote},
					)
					require.Equal(t, test.kind, dialog.GetRefusalKind())
					require.Equal(t, test.terminal, dialog.GetRefusalKind().IsTerminal())
					require.Contains(t, strings.ToLower(dialog.GetBodyText()), "cannot accept")
					require.Contains(t, dialog.GetFooterGuidance(), "Acknowledge")
					require.NotContains(t, dialog.GetFooterGuidance(), "Sell All")
					pressArmsTradeKey(dialog, keyboard, key)
					require.True(t, dialog.IsDismissed())
					require.False(t, dialog.IsCancelled(), "refusal acknowledgement is not cancellation")
					_, ok := dialog.ConsumePendingRequest()
					require.False(t, ok)
				})
			}
		})
	}
}

func TestArmsTradeDialogGeometryIsCenteredAndBounded(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {320, 180}} {
		setMerchantShellTestResolution(t, size[0], size[1])
		dialog := newTestArmsTradeDialog(newWidgetTestKeyboard(0), ArmsTradeIntent{
			Mode: ArmsShopBuy, BuyQuote: testArmsShopState().BuyQuotes[0],
		})
		bounds := dialog.GetBounds()
		require.InDelta(t, size[0]/2, bounds.Min.X+bounds.Dx()/2, 1)
		require.True(t, dialog.frame.bodyBounds.In(bounds))
		require.True(t, dialog.frame.footerBounds.In(bounds))
	}
}

func TestArmsTradeDialogResizePreservesQuantityAndRequest(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	keyboard := newWidgetTestKeyboard(0)
	quote := testArmsShopState().BuyQuotes[0]
	quote.MaxQuantity = 3
	dialog := newTestArmsTradeDialog(keyboard, ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: quote})
	pressArmsTradeKey(dialog, keyboard, ebiten.KeyRight)
	lowBounds := dialog.GetBounds()
	wantBody := dialog.GetBodyText()

	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog.ensureLayout()
	require.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	require.Equal(t, uint16(2), dialog.GetQuantity())
	require.Equal(t, wantBody, dialog.GetBodyText())

	pressArmsTradeKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, uint16(2), request.Buy.Quantity)
}

func newTestArmsTradeDialog(keyboard *input.Keyboard, intent ArmsTradeIntent) *ArmsTradeDialog {
	return NewArmsTradeDialog(ArmsTradeDialogConfig{
		Keyboard: keyboard,
		Intent:   intent,
		Prose: ArmsTradeDialogProse{
			Pitch:   "A sound choice at a fair price.",
			Offer:   "I offer good gold for this equipment.",
			Refusal: "I cannot accept that equipment.",
		},
	})
}

func pressArmsTradeKey(dialog *ArmsTradeDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func TestArmsTradeDialogPhysicalFixedAndZeroMaximum(t *testing.T) {
	for _, index := range []int{0, 1} {
		quote := testArmsShopState().BuyQuotes[index]
		quote.MaxQuantity = 0
		t.Run(quote.Equipment.String(), func(t *testing.T) {
			keys := newPhysicalQuantityKeys()
			dialog := newTestArmsTradeDialog(keys.keyboard, ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: quote})
			keys.held[ebiten.KeyRight] = true
			for range 60 {
				keys.tick(dialog)
			}
			require.Equal(t, uint16(1), dialog.GetQuantity())
			require.False(t, dialog.HasPendingRequest())
			keys.release(dialog)
			keys.held[ebiten.KeyEnter] = true
			keys.tick(dialog)
			request, ok := dialog.ConsumePendingRequest()
			require.True(t, ok, "zero maximum still submits the original one-item attempt")
			require.Equal(t, game_state.ArmsBuyRequest{Equipment: quote.Equipment, Quantity: 1}, request.Buy)
		})
	}
}
