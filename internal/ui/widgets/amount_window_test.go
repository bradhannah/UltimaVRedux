package widgets

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

// requireAmountWindowHoldsItsText checks that nothing an amount window shows is
// wider than the window or lower than its body, so no line wraps or is cut.
func requireAmountWindowHoldsItsText(t *testing.T, window *amountWindow, context string) {
	t.Helper()
	letters := window.body.Font.CharsForWidth(window.frame.bodyText.Dx())
	require.GreaterOrEqual(t, letters, amountWindowChars, "%s: the window is narrower than its grid", context)
	for _, shown := range []string{outputText(window.unit), outputText(window.body), outputText(window.footer)} {
		for _, line := range strings.Split(shown, "\n") {
			require.LessOrEqual(t, utf8.RuneCountInString(strings.TrimRight(line, " ")), amountWindowChars,
				"%s: %q is wider than the window", context, line)
		}
	}
	title, _ := window.header.Font.MeasureString(outputText(window.header))
	require.LessOrEqual(t, int(title), window.frame.headerText.Dx(), "%s: the title is wider than the window", context)
	require.LessOrEqual(t, window.body.GetPanelContentLineCount(),
		window.body.GetLineCapacity(window.frame.bodyText.Dy()), "%s: a body line is cut off", context)
}

// The longest name and four-digit gold keep every grid row on one line at every
// supported window size.
func TestAmountWindowRowsStayWholeAtEverySupportedSize(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		buy := testArmsShopState().BuyQuotes[0]
		buy.Equipment, buy.MaxQuantity, buy.UnitPrice, buy.Gold = references.TwoHHammer, 9, 1000, 9999
		arms := newTestArmsTradeDialog(newWidgetTestKeyboard(0), ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: buy})
		pressArmsTradeKey(arms, arms.keyboard, ebiten.KeyM)
		reagent := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
			Quote: testReagentQuote(references.MandrakeRoot, "Mandrake Root", 5, 130, 7, 9999)})
		for name, window := range map[string]*amountWindow{"arms": arms.window, "reagent": reagent.window} {
			requireAmountWindowHoldsItsText(t, window, fmt.Sprintf("%s at %dx%d", name, size.X, size.Y))
			rows := 0
			for _, line := range strings.Split(outputText(window.body), "\n") {
				if strings.HasPrefix(line, " "+amountCarriedRow) || strings.HasPrefix(line, " "+amountGoldRow) {
					rows++
					require.Regexp(t, `^ \w+ +\d+ +[+-]\d+ +\d+ *$`, line, "%s row at %dx%d", name, size.X, size.Y)
				}
			}
			require.Equal(t, 2, rows, name)
		}
		require.Contains(t, arms.window.flatText(), "Carried 8 +9 17 Gold 9999 -9000 999")
	}
}

// The chosen number is the label yellow between its arrows, with no selection bar,
// and an arrow that cannot move is drawn as unavailable.
func TestAmountStepperChosenNumberStandsOutWithoutABar(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	output := text.NewOutput(text.NewUltimaFont(16), 1, 4, amountWindowChars)
	colours := func(count, most int) map[string]color.Color {
		found := map[string]color.Color{}
		for _, segment := range appendAmountStepper(output.AppendLine(), count, most).GetSegments() {
			found[strings.TrimSpace(segment.Text)] = segment.Color
		}

		return found
	}
	first := colours(1, 9)
	require.Equal(t, DialogLabelColor, first["1"])
	require.Equal(t, DialogUnavailableItemColor, first["<"], "nothing is below one")
	require.Equal(t, DialogValueColor, first[">"])
	require.Equal(t, DialogControlHintColor, first["of 9"])
	last := colours(9, 9)
	require.Equal(t, DialogLabelColor, last["9"])
	require.Equal(t, DialogValueColor, last["<"])
	require.Equal(t, DialogUnavailableItemColor, last[">"], "nothing is above the most")

	dialog := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
		Quote: testReagentShopState().Offers[0]})
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	body := dialog.window.frame.bodyText
	amountRow := image.Rect(body.Min.X, body.Min.Y, body.Max.X, body.Min.Y+int(dialog.window.body.GetFontHeight()))
	requireImageRegionContainsColor(t, screen, amountRow, DialogLabelColor)
	requireColorRGBA(t, MerchantDialogBackgroundColor, screen.At(amountRow.Min.X+1, amountRow.Min.Y+1))
}

// With a single amount to choose, the keys that choose one are dimmed.
func TestAmountWindowDimsAmountKeysWhenOnlyOneAmountExists(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	one := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
		Quote: testReagentQuote(references.SpiderSilk, "Spider Silk", 4, 150, 5, 200)})
	require.Equal(t, uint16(1), one.quote.MaxBundles)
	_, dim := one.window.footer.FirstPanelLineWithColor(0, 1, DialogUnavailableItemColor)
	_, live := one.window.footer.FirstPanelLineWithColor(0, 1, DialogControlKeyColor)
	require.True(t, dim)
	require.False(t, live)

	many := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
		Quote: testReagentShopState().Offers[0]})
	_, dim = many.window.footer.FirstPanelLineWithColor(0, 1, DialogUnavailableItemColor)
	require.False(t, dim)
}

// The window is as large as its content, and keeps its size when the overflow
// warning appears or the amount changes.
func TestAmountWindowFitsItsContentAndKeepsItsSize(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		dialog := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
			Quote: testReagentShopState().Offers[0]})
		before := dialog.GetBounds()
		at := fmt.Sprintf("%dx%d", size.X, size.Y)
		require.Less(t, before.Dy(), size.Y/2, "%s: the window must not be mostly empty", at)
		letter, _ := dialog.window.body.Font.MeasureString("M")
		require.Less(t, dialog.window.frame.bodyText.Dx(), int(letter*(amountWindowChars+1))+1,
			"%s: the window must not be wider than its grid needs", at)
		requireAmountWindowHoldsItsText(t, dialog.window, at)
		pressReagentPurchaseKey(dialog, dialog.keyboard, ebiten.KeyM)
		require.True(t, dialog.GetApprovalView().PaidOverflow)
		require.Equal(t, before, dialog.GetBounds(), at)
		requireAmountWindowHoldsItsText(t, dialog.window, at+" with the warning")
	}
}

// Digits type the amount: a second digit joins the first while the number stays
// within the most, and otherwise starts the number again.
func TestQuantityInputDigitsTypeTheAmount(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	amounts := newQuantityInput(keyboard, keyboard.IsKeyHeld, ebiten.KeyM, ebiten.KeyUp, ebiten.KeyDown)
	current := 1
	press := func(key ebiten.Key, most int) int {
		keyboard.SimulateKeyPress(key)
		next, owned := amounts.pollAmount(current, most)
		require.True(t, owned)
		current = next

		return current
	}
	require.Equal(t, 2, press(ebiten.Key2, 14))
	require.Equal(t, 5, press(ebiten.Key5, 14), "25 is more than the most, so the digit starts again")
	require.Equal(t, 5, press(ebiten.Key0, 14), "50 is more than the most and nothing asks for none")
	require.Equal(t, 1, press(ebiten.Key1, 14))
	require.Equal(t, 12, press(ebiten.Key2, 14))
	require.Equal(t, 13, press(ebiten.KeyRight, 14))
	require.Equal(t, 1, press(ebiten.Key1, 14), "a step ends the number being typed")
	require.Equal(t, 10, press(ebiten.Key0, 14))
	require.Equal(t, 14, press(ebiten.KeyM, 14))
	require.Equal(t, 13, press(ebiten.KeyUp, 14))
	require.Equal(t, 14, press(ebiten.KeyDown, 14))
	require.Equal(t, 3, press(ebiten.Key3, 3))
	require.Equal(t, 3, press(ebiten.Key7, 3), "more than the most changes nothing")

	_, owned := amounts.pollAmount(current, 3)
	require.False(t, owned, "with no key down Enter is free to confirm")
}
