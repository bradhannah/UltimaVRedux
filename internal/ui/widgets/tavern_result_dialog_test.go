package widgets

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
)

func TestTavernResultDialogRendersSemanticPurchaseSummary(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	view := TavernResultView{
		Title: "Meal Served", Item: "Mutton", Detail: "3 diners at 4 gold each",
		ChargeLabel: "Total", Charge: 12,
		ShowFood: true, FoodBefore: 40, FoodAfter: 43,
		ShowGold: true, GoldBefore: 100, GoldAfter: 83,
		Theft: 5, Note: "The table is cleared.", Tone: TavernResultSuccess,
	}
	dialog := NewTavernResultDialog(TavernResultDialogConfig{
		Keyboard:     newWidgetTestKeyboard(0),
		View:         view,
		IsKeyPressed: func(ebiten.Key) bool { return false },
	})

	lowBounds := dialog.GetBounds()
	words := dialog.window.flatText()
	require.Equal(t, "Meal Served Mutton Have Then Food 40 +3 43 Gold 100 -17 83 3 diners at 4 gold each "+
		"Total: 12 gold. 5 more gold was stolen! The table is cleared. [Enter/Esc] Continue", words)

	setResponsiveDialogTestResolution(t, 1280, 720)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Dispose)
	require.NotPanics(t, func() { dialog.Draw(screen) })
	require.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	require.Equal(t, words, dialog.window.flatText())
	require.Equal(t, view, dialog.GetView())
	require.True(t, dialog.IsVisible())
	requireImageRegionContainsColor(t, screen, dialog.GetBodyBounds(), MerchantDialogBackgroundColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.bodyText, DialogLabelColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.bodyText, DialogControlHintColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.bodyText, DialogValueColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.bodyText, DialogWarningColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlKeyColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlHintColor)
}

// A receipt reads on one window at every size, whether the order went through, ended
// short, or was refused, and a refusal that charges nothing still names the price.
func TestTavernResultDialogHoldsEveryReceiptAtEverySize(t *testing.T) {
	views := map[string]TavernResultView{
		"refused": {
			Title: "Meal Refused", Item: "Mutton", Detail: "3 diners at 4 gold each", ChargeLabel: "Required",
			Charge: 12, ShowFood: true, FoodBefore: 40, FoodAfter: 40, ShowGold: true, GoldBefore: 5, GoldAfter: 5,
			Note: "The party cannot pay for this meal.", Tone: TavernResultFailure,
		},
		"information": {
			Title: "Information Refused", Item: "Ankh", Note: "Come back with gold, friend.", Tone: TavernResultFailure,
		},
		"caution": {
			Title: "Partial Order", Item: "Rations", ChargeLabel: "Paid", Charge: 20,
			ShowFood: true, FoodBefore: 9990, FoodAfter: 9999, ShowGold: true, GoldBefore: 9999, GoldAfter: 9979,
			Note: "Carried food has reached 9999.", Tone: TavernResultCaution,
		},
	}
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		for name, view := range views {
			dialog := NewTavernResultDialog(TavernResultDialogConfig{
				Keyboard: newWidgetTestKeyboard(0), View: view, IsKeyPressed: func(ebiten.Key) bool { return false },
			})
			requireAmountWindowHoldsItsText(t, dialog.window, fmt.Sprintf("%s at %dx%d", name, size.X, size.Y))
		}
	}
	refused := NewTavernResultDialog(TavernResultDialogConfig{
		Keyboard: newWidgetTestKeyboard(0), View: views["refused"], IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	require.Contains(t, refused.window.flatText(), "Food 40 +0 40 Gold 5 +0 5")
	require.Contains(t, refused.window.flatText(), "Required: 12 gold.")
}

func TestTavernResultDialogDrawsAFailureNoteInTheReadableRed(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	for _, test := range []struct {
		tone TavernResultTone
		want bool
	}{{TavernResultSuccess, false}, {TavernResultCaution, false}, {TavernResultFailure, true}} {
		dialog := NewTavernResultDialog(TavernResultDialogConfig{
			Keyboard: newWidgetTestKeyboard(0),
			View: TavernResultView{
				Title: "Provision Result", Item: "Provisions", Note: "Final authority result.", Tone: test.tone,
			},
			IsKeyPressed: func(ebiten.Key) bool { return false },
		})
		_, red := dialog.window.body.FirstPanelLineWithColor(0, dialog.window.body.GetPanelContentLineCount(), DialogWarningColor)
		require.Equal(t, test.want, red, "tone %d", test.tone)
	}
}

func TestTavernResultDialogWaitsForReleaseAndExplicitAcknowledgement(t *testing.T) {
	held := true
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewTavernResultDialog(TavernResultDialogConfig{
		Keyboard: keyboard,
		View:     TavernResultView{Title: "Meal Result", Item: "Mutton", Tone: TavernResultSuccess},
		IsKeyPressed: func(key ebiten.Key) bool {
			return held && key == ebiten.KeyEnter
		},
	})

	dialog.Update()
	require.True(t, dialog.IsVisible())
	require.False(t, dialog.IsComplete())
	held = false
	dialog.Update()
	require.True(t, dialog.IsVisible(), "release frame must not acknowledge the result")
	keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	require.True(t, dialog.IsDismissed())
	require.True(t, dialog.IsComplete())
	require.False(t, dialog.IsVisible())
	dialog.ShowWithoutAnimation()
	require.False(t, dialog.IsVisible(), "acknowledged results cannot reopen")
}

func TestTavernResultDialogRejectsIncompleteConfiguration(t *testing.T) {
	require.Panics(t, func() {
		NewTavernResultDialog(TavernResultDialogConfig{
			Keyboard: newWidgetTestKeyboard(0), View: TavernResultView{Item: "Mutton", Tone: TavernResultSuccess},
		})
	})
	require.Panics(t, func() {
		NewTavernResultDialog(TavernResultDialogConfig{
			Keyboard: newWidgetTestKeyboard(0),
			View:     TavernResultView{Title: "Meal Result", Item: "Mutton", Tone: TavernResultTone(99)},
		})
	})
}
