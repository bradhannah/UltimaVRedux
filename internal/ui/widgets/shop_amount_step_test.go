package widgets

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// The longest goods name and four-digit gold must keep every table row on one
// line at every supported window size.
func TestShopAmountStepTableRowsStayWholeAtEverySupportedSize(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		buy := testArmsShopState().BuyQuotes[0]
		buy.Equipment, buy.MaxQuantity, buy.UnitPrice, buy.Gold = references.TwoHHammer, 3, 1000, 9999
		arms := newTestArmsTradeDialog(newWidgetTestKeyboard(0), ArmsTradeIntent{Mode: ArmsShopBuy, BuyQuote: buy})
		reagent := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
			Quote: testReagentQuote(references.MandrakeRoot, "Mandrake Root", 5, 130, 7, 9999)})
		for name, body := range map[string]string{"arms": arms.GetBodyText(), "reagent": reagent.GetBodyText()} {
			rows := 0
			for _, line := range strings.Split(body, "\n") {
				if strings.HasPrefix(line, " Two-Handed Hammer") || strings.HasPrefix(line, " Mandrake Root") {
					rows++
					require.Regexp(t, `\d+ +\d+ *$`, line, "%s row wrapped at %dx%d", name, size.X, size.Y)
				}
			}
			require.Equal(t, 1, rows, name)
		}
	}
}

// With a single amount to choose, the keys that choose one are dimmed.
func TestShopAmountStepDimsAmountKeysWhenOnlyOneAmountExists(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	one := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
		Quote: testReagentQuote(references.SpiderSilk, "Spider Silk", 4, 150, 5, 200)})
	require.Equal(t, uint16(1), one.quote.MaxBundles)
	_, dim := one.footerOutput.FirstPanelLineWithColor(0, 1, DialogUnavailableItemColor)
	_, live := one.footerOutput.FirstPanelLineWithColor(0, 1, DialogControlKeyColor)
	require.True(t, dim)
	require.False(t, live)

	many := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
		Quote: testReagentShopState().Offers[0]})
	_, dim = many.footerOutput.FirstPanelLineWithColor(0, 1, DialogUnavailableItemColor)
	require.False(t, dim)
}

// The window is as tall as its content, and keeps its size when the overflow
// warning appears or the amount changes.
func TestShopAmountStepWindowFitsItsContentAndKeepsItsSize(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		dialog := NewReagentPurchaseDialog(ReagentPurchaseDialogConfig{Keyboard: newWidgetTestKeyboard(0),
			Quote: testReagentShopState().Offers[0]})
		before := dialog.GetBounds()
		require.Less(t, before.Dy(), size.Y/2, "%dx%d: the window must not be mostly empty", size.X, size.Y)
		dialog.setBundleCount(dialog.quote.MaxBundles)
		require.True(t, dialog.GetApprovalView().PaidOverflow)
		require.Equal(t, before, dialog.GetBounds(), "%dx%d", size.X, size.Y)
		require.LessOrEqual(t, dialog.bodyOutput.GetPanelContentLineCount(),
			dialog.bodyOutput.GetLineCapacity(dialog.frame.bodyText.Dy()), "%dx%d: a body line is cut off", size.X, size.Y)
	}
}
