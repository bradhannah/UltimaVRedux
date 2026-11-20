package widgets

import (
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/require"
)

// requireKeyedGuidance asserts that footer guidance is complete "[Key] Action" hints.
func requireKeyedGuidance(t *testing.T, guidance string) {
	t.Helper()
	for _, line := range parseMerchantFooterGuidance(guidance) {
		for _, hint := range line {
			require.NotEmpty(t, hint.Key, guidance)
		}
	}
}

func TestBracketedControlHintsParsePackAndPreserveProse(t *testing.T) {
	require.Equal(t, []ControlHint{{"", "Delete excerpt?"}, {"Enter", "Delete"}, {"Other keys", "Close"},
		{"Arrows", ""}, {"", "[Enter"}}, ParseControlHints("Delete excerpt?  [Enter] Delete [Other keys] Close [Arrows] [Enter"))
	output := text.NewOutput(text.NewUltimaFont(16), 1, 8, 1)
	hints := []ControlHint{{"Up/Down", "Select"}, {"Enter", "Buy"}, {"Esc", "Leave"}}
	width, _ := output.Font.MeasureString("[Up/Down] Select   [Enter] Buy")
	PackControlHints(output, hints, int(width)+1, etext.AlignStart)
	require.Equal(t, "[Up/Down] Select   [Enter] Buy \n[Esc] Leave", output.GetOutputStr(false))
	line := AppendControlHints(output.AppendLine(), "[Esc] Leave")
	require.Equal(t, []text.ColorSegment{{Text: "[Esc]", Color: DialogControlKeyColor},
		{Text: " Leave", Color: DialogControlHintColor}}, line.GetSegments())
}

func TestArmsAndAcknowledgementActualOutputsUseControlColours(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	quote := game_state.ArmsSellQuote{Equipment: references.Dagger, BasePrice: 50, UnitOffer: 40,
		Gold: 500, QuantityBefore: 5, MaxQuantity: 3, Sellable: true}
	dialog := newTestArmsTradeDialog(newWidgetTestKeyboard(0), ArmsTradeIntent{Mode: ArmsShopSell, SellQuote: quote})
	dialog.populateFooter()
	width, _ := dialog.footerOutput.Font.MeasureString(strings.Split(dialog.GetFooterGuidance(), "\n")[0])
	t.Logf("footer first line width %.1f, pane %d, measured characters %d", width,
		dialog.frame.footerText.Dx(), dialog.footerOutput.Font.CharsForWidth(dialog.frame.footerText.Dx()))
	_, keys := dialog.footerOutput.FirstPanelLineWithColor(0, dialog.footerOutput.GetPanelContentLineCount(), DialogControlKeyColor)
	_, hints := dialog.footerOutput.FirstPanelLineWithColor(0, dialog.footerOutput.GetPanelContentLineCount(), DialogControlHintColor)
	require.True(t, keys)
	require.True(t, hints)
	require.Contains(t, strings.Join(strings.Fields(dialog.footerOutput.GetOutputStr(false)), " "), "[M] Sell all")
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	keyPixels, hintPixels := 0, 0
	for y := dialog.frame.footerText.Min.Y; y < dialog.frame.footerText.Max.Y; y++ {
		for x := dialog.frame.footerText.Min.X; x < dialog.frame.footerText.Max.X; x++ {
			red, green, blue, _ := screen.At(x, y).RGBA()
			if red == 65535 && green == red && blue == red {
				keyPixels++
			}
			if red == 43690 && green == red && blue == red {
				hintPixels++
			}
		}
	}
	require.Positive(t, keyPixels)
	require.Positive(t, hintPixels)
	message := NewMessageDialog("Done").WithEffects(false).WithAutoSizeDefault().WithSeparatePromptArea().Build()
	for _, hint := range []string{DialogCloseHint, DialogAnyKeyHint} {
		message.SetAcknowledgementPrompt(hint)
		require.Equal(t, hint, message.promptOutput.GetOutputStr(false))
		_, found := message.promptOutput.FirstPanelLineWithColor(0, message.promptOutput.GetPanelContentLineCount(), DialogControlKeyColor)
		require.True(t, found)
		require.False(t, message.enableInputDismissal)
	}
	message.ClearPrompt()
	require.Empty(t, message.promptOutput.GetOutputStr(false))
}

func TestControlHintsDimmingDimsOnlyChosenHintsWhole(t *testing.T) {
	hints := []ControlHint{{hintKeyEnter, "Use"}, {hintKeyEsc, "Cancel"}}
	line := appendControlHintRow(text.NewOutput(text.NewUltimaFont(16), 1, 2, 80).AppendLine(), hints, controlHintSeparator,
		func(hint ControlHint) bool { return hint.Key == hintKeyEnter })
	require.Equal(t, []text.ColorSegment{{Text: "[Enter] Use", Color: DialogUnavailableItemColor},
		{Text: controlHintSeparator, Color: DialogControlHintColor}, {Text: "[Esc]", Color: DialogControlKeyColor},
		{Text: " Cancel", Color: DialogControlHintColor}}, line.GetSegments())
}
