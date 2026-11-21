package widgets

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestGuildPurchaseDialogConstructorContracts(t *testing.T) {
	quote := testGuildShopState().Offers[0]
	valid := GuildPurchaseDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Quote:    quote,
		Pitch:    "Three keys for one hundred ninety gold.",
	}

	tests := []struct {
		name   string
		mutate func(*GuildPurchaseDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *GuildPurchaseDialogConfig) { config.Keyboard = nil }},
		{name: "pitch", mutate: func(config *GuildPurchaseDialogConfig) { config.Pitch = " " }},
		{name: "provision", mutate: func(config *GuildPurchaseDialogConfig) {
			config.Quote.Provision = references.NoProvision
		}},
		{name: "bundle", mutate: func(config *GuildPurchaseDialogConfig) { config.Quote.BundleQuantity = 0 }},
		{name: "price", mutate: func(config *GuildPurchaseDialogConfig) { config.Quote.PricePerBundle = 0 }},
		{name: "maximum", mutate: func(config *GuildPurchaseDialogConfig) { config.Quote.MaxBundles++ }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.Panics(t, func() { NewGuildPurchaseDialog(config) })
		})
	}

	require.NotPanics(t, func() { NewGuildPurchaseDialog(valid) })
}

func TestGuildPurchaseDialogQuantityBoundsProjectionAndSingleRequest(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	quote := testGuildShopState().Offers[2]
	dialog := NewGuildPurchaseDialog(GuildPurchaseDialogConfig{
		Keyboard: keyboard,
		Quote:    quote,
		Pitch:    "Five torches for twelve gold.",
	})

	require.Equal(t, uint16(1), dialog.GetBundleCount())
	require.Equal(t, "Five torches for twelve gold.", dialog.GetPitch())
	flat := dialog.window.flatText()
	require.Contains(t, flat, `Buy Torches 12 gold for a bundle of 5 "Five torches for twelve gold."`)
	require.Contains(t, flat, "Bundles < 1 > of 14")
	require.Contains(t, flat, "Have Then Carried 30 +5 35 Gold 500 -12 488")
	require.Contains(t, flat, "[Left/Right/0-9] Bundles [M] Most [Enter] Buy 1 [Esc] Back")
	for _, dropped := range []string{"TOTAL PRICE", "BEFORE THEFT", "per bundle", "Up/Down", "will not fit"} {
		require.NotContains(t, flat, dropped)
	}
	requireAmountWindowHoldsItsText(t, dialog.window, "one bundle")

	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyLeft)
	require.Equal(t, uint16(1), dialog.GetBundleCount())
	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyDown)
	require.Equal(t, uint16(2), dialog.GetBundleCount())
	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyUp)
	require.Equal(t, uint16(1), dialog.GetBundleCount())
	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyRight)
	require.Equal(t, uint16(2), dialog.GetBundleCount())
	pressGuildPurchaseKey(dialog, keyboard, ebiten.Key3)
	require.Equal(t, uint16(3), dialog.GetBundleCount(), "a digit types the amount")
	require.Contains(t, dialog.window.flatText(), "Carried 30 +15 45 Gold 500 -36 464")
	require.Contains(t, dialog.window.flatText(), "[Enter] Buy 3")
	before := dialog.GetBounds()
	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyM)
	require.Equal(t, quote.MaxBundles, dialog.GetBundleCount())
	flat = dialog.window.flatText()
	require.Contains(t, flat, "Bundles < 14 > of 14")
	require.Contains(t, flat, "Carried 30 +69 99 Gold 500 -168 332")
	require.Contains(t, flat, "1 will not fit. The price stays.", "a bundle that will not all fit says what is lost")
	require.Equal(t, before, dialog.GetBounds(), "the warning has its room kept")
	requireAmountWindowHoldsItsText(t, dialog.window, "the most")

	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
	require.False(t, dialog.IsCancelled())
	require.True(t, dialog.HasPendingPurchaseRequest())
	request, ok := dialog.ConsumePendingPurchaseRequest()
	require.True(t, ok)
	require.Equal(t, game_state.GuildPurchaseRequest{
		Provision:   references.Torches,
		BundleCount: quote.MaxBundles,
	}, request)

	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingPurchaseRequest()
	require.False(t, duplicate, "dismissed modal must not emit duplicate requests")
}

func TestGuildPurchaseDialogSaysWhatAFinalBundleLosesAt99(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	quote := testGuildQuote(references.Key, 3, 10, 98, 100)
	dialog := NewGuildPurchaseDialog(GuildPurchaseDialogConfig{
		Keyboard: keyboard,
		Quote:    quote,
		Pitch:    "A final bundle of keys.",
	})

	require.Equal(t, uint16(1), quote.MaxBundles)
	flat := dialog.window.flatText()
	require.Contains(t, flat, "Carried 98 +1 99 Gold 100 -10 90")
	require.Contains(t, flat, "2 will not fit. The price stays.")
	_, dim := dialog.window.footer.FirstPanelLineWithColor(0, 1, DialogUnavailableItemColor)
	require.True(t, dim, "one bundle leaves nothing for the amount keys to choose")
}

func TestGuildPurchaseDialogZeroMaxStatesAllowOnlyEscape(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	tests := []struct {
		name   string
		quote  game_state.GuildOfferQuote
		reason string
	}{
		{
			name:   "inventory full",
			quote:  testGuildQuote(references.Key, 3, 10, 99, 100),
			reason: "No room. The party carries 99, the most it can.",
		},
		{
			name:   "insufficient money",
			quote:  testGuildQuote(references.Key, 3, 10, 10, 1),
			reason: "Not enough gold. A bundle costs 10 and the party has 1.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keyboard := newWidgetTestKeyboard(0)
			dialog := NewGuildPurchaseDialog(GuildPurchaseDialogConfig{
				Keyboard: keyboard,
				Quote:    test.quote,
				Pitch:    "Original merchant pitch.",
			})

			require.Zero(t, dialog.GetBundleCount())
			require.Zero(t, test.quote.MaxBundles)
			flat := dialog.window.flatText()
			require.Contains(t, flat, "Buy Keys 10 gold for a bundle of 3 "+test.reason+" [Enter] Buy [Esc] Back")
			require.Equal(t, 1, dialog.window.footer.GetPanelContentLineCount())
			_, dim := dialog.window.footer.FirstPanelLineWithColor(0, 1, DialogUnavailableItemColor)
			require.True(t, dim, "Enter is shown as unavailable")
			requireAmountWindowHoldsItsText(t, dialog.window, test.name)
			screen := ebiten.NewImage(1280, 720)
			t.Cleanup(screen.Dispose)
			dialog.Draw(screen)
			requireImageRegionContainsColor(t, screen, dialog.window.frame.bodyText, DialogWarningColor)
			pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyEnter)
			require.False(t, dialog.IsDismissed())
			require.False(t, dialog.HasPendingPurchaseRequest())

			pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyEscape)
			require.True(t, dialog.IsDismissed())
			require.True(t, dialog.IsCancelled())
		})
	}
}

func TestGuildPurchaseDialogIsCenteredOverGuildShopAndRebuilds(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	shop := newTestGuildShopDialog(newWidgetTestKeyboard(0), testGuildShopState())
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewGuildPurchaseDialog(GuildPurchaseDialogConfig{
		Keyboard: keyboard,
		Quote:    testGuildShopState().Offers[0],
		Pitch:    "Original keys pitch.",
	})
	pressGuildPurchaseKey(dialog, keyboard, ebiten.KeyRight)
	initial := dialog.GetBounds()
	wantBody := dialog.GetBodyText()

	require.True(t, initial.In(shop.GetShell().GetBounds()))
	require.InDelta(t, 640, initial.Min.X+initial.Dx()/2, 1)
	require.InDelta(t, 360, initial.Min.Y+initial.Dy()/2, 1)
	require.Contains(t, dialog.GetBodyText(), "Original keys pitch.")

	setResponsiveDialogTestResolution(t, 1920, 1080)
	require.Greater(t, dialog.GetBounds().Dx(), initial.Dx())
	require.Greater(t, dialog.GetBounds().Dy(), initial.Dy())
	require.Equal(t, uint16(2), dialog.GetBundleCount())
	require.Equal(t, wantBody, dialog.GetBodyText())
}

func TestGuildPurchaseDialogQuotesPitchExactlyOnceWithoutChangingStoredPitch(t *testing.T) {
	for _, test := range []struct {
		name  string
		pitch string
	}{
		{name: "unquoted", pitch: "Three keys for one hundred ninety gold."},
		{name: "already quoted", pitch: `"Three keys for one hundred ninety gold."`},
		{name: "closing quote only", pitch: `Three keys for one hundred ninety gold."`},
	} {
		t.Run(test.name, func(t *testing.T) {
			dialog := NewGuildPurchaseDialog(GuildPurchaseDialogConfig{
				Keyboard: newWidgetTestKeyboard(0),
				Quote:    testGuildShopState().Offers[0],
				Pitch:    test.pitch,
			})
			body := dialog.GetBodyText()
			pitchEnd := strings.Index(body, "Bundles")
			require.Positive(t, pitchEnd)
			presentedPitch := body[:pitchEnd]

			require.Equal(t, test.pitch, dialog.GetPitch())
			require.Equal(t, 2, strings.Count(presentedPitch, `"`))
			require.True(t, strings.HasPrefix(strings.TrimSpace(presentedPitch), `"`))
			require.Contains(t, presentedPitch, `gold."`)
		})
	}
}

func requireOutputVerticallyCentered(t *testing.T, bounds image.Rectangle, output interface {
	GetPanelContentLineCount() int
	GetTotalHeightForLines(int) float64
}) {
	t.Helper()
	const halves = 2
	lineCount := output.GetPanelContentLineCount()
	textHeight := int(math.Ceil(output.GetTotalHeightForLines(lineCount)))
	startY := bounds.Min.Y + max(0, (bounds.Dy()-textHeight)/halves)
	topSpace := startY - bounds.Min.Y
	bottomSpace := bounds.Max.Y - (startY + textHeight)
	require.InDelta(t, topSpace, bottomSpace, 1)
}

func pressGuildPurchaseKey(dialog *GuildPurchaseDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func TestGuildPurchaseDialogPhysicalZeroMaximumCannotConfirm(t *testing.T) {
	keys := newPhysicalQuantityKeys()
	dialog := NewGuildPurchaseDialog(GuildPurchaseDialogConfig{
		Keyboard: keys.keyboard, Quote: testGuildQuote(references.Key, 3, 10, 99, 100),
		Pitch: "A bundle of keys.",
	})
	for _, key := range []ebiten.Key{ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyM, ebiten.KeyEnter} {
		keys.held[key] = true
		for range 60 {
			keys.tick(dialog)
		}
		require.Zero(t, dialog.GetBundleCount())
		require.False(t, dialog.HasPendingPurchaseRequest())
		require.False(t, dialog.IsDismissed())
		keys.release(dialog)
	}
	keys.held[ebiten.KeyEscape] = true
	keys.tick(dialog)
	require.True(t, dialog.IsCancelled())
}
