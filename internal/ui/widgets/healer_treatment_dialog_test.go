package widgets

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestHealerTreatmentDialogShowsIndividualTargetEffectAndCharge(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	keyboard := newWidgetTestKeyboard(0)
	shop := newTestHealerShopDialog(keyboard, testHealerShopState())
	pressHealerShopKey(shop, keyboard, ebiten.KeyEnter)
	pressHealerShopKey(shop, keyboard, ebiten.KeyEnter)
	intent, ok := shop.ConsumePendingTreatmentIntent()
	require.True(t, ok)

	dialog := NewHealerTreatmentDialog(HealerTreatmentDialogConfig{Keyboard: keyboard, Intent: intent})
	words := dialog.window.flatText()
	lowBounds := dialog.GetBounds()

	assert.Equal(t, "Cure Shamino Poisoned becomes Good. Free care. No payment. [Enter] Cure [Esc] Back", words)
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog.window.ensureLayout()
	require.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	require.Equal(t, words, dialog.window.flatText())
	requireAmountWindowHoldsItsText(t, dialog.window, "cure")
	require.Equal(t, intent, dialog.GetIntent())
	pressHealerTreatmentKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, intent.Request, request)
}

func TestHealerTreatmentDialogAllIsShortcutTargetNotPartyRow(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	shop := newTestHealerShopDialog(keyboard, testHealerShopState())
	require.Len(t, shop.recipientRows, healerShopMaximumRecipients)
	pressHealerShopKey(shop, keyboard, ebiten.KeyA)
	intent, ok := shop.ConsumePendingTreatmentIntent()
	require.True(t, ok)

	dialog := NewHealerTreatmentDialog(HealerTreatmentDialogConfig{Keyboard: keyboard, Intent: intent})

	assert.Equal(t, "Cure All 1 party member needs it Free care. No payment. [Enter] Cure [Esc] Back",
		dialog.window.flatText())
}

// A paid treatment shows what it changes and what it costs; one the party cannot
// pay for is still sent, so Enter stays live and says what asking will bring.
func TestHealerTreatmentDialogShowsEffectRowsAndShortfall(t *testing.T) {
	state := testHealerShopState()
	resurrect := func(gold uint16, affordable bool) HealerTreatmentIntent {
		recipient := state.Recipients[2]
		quote := state.Services[references.HealerResurrect]
		quote.Affordable = affordable

		return HealerTreatmentIntent{
			Request:    game_state.HealerServiceRequest{Service: references.HealerResurrect, CharacterIndex: 2},
			Service:    quote,
			AllService: state.AllServices[references.HealerResurrect],
			Recipient:  &recipient,
			Gold:       gold,
		}
	}
	heal := resurrect(312, true)
	heal.Request.Service = references.HealerHeal
	heal.Service, heal.AllService = state.Services[references.HealerHeal], state.AllServices[references.HealerHeal]
	recipient := state.Recipients[1]
	heal.Recipient, heal.Request.CharacterIndex = &recipient, 1
	all := resurrect(312, true)
	all.Request.CharacterIndex, all.Recipient = game_state.HealerAllRecipientsIndex, nil

	tests := []struct{ name, text string }{
		{"resurrect", "Resurrect Dupre Have Then HP 0 +60 60 Max HP 90 -30 60 Gold 312 -200 112 " +
			"Dead becomes Good. [Enter] Resurrect [Esc] Back"},
		{"short", "Resurrect Dupre Thou hast not enough gold. It costs 200 and the party has 50. " +
			"The healer will refuse. [Enter] Resurrect anyway [Esc] Back"},
		{"heal", "Heal Iolo Have Then HP 18 +42 60 Still Sleeping. Free care. No payment. [Enter] Heal [Esc] Back"},
		{"all", "Resurrect All 1 party member needs it Have Then Gold 312 -200 112 [Enter] Resurrect [Esc] Back"},
	}
	intents := map[string]HealerTreatmentIntent{
		"resurrect": resurrect(312, true), "short": resurrect(50, false), "heal": heal, "all": all,
	}
	for _, size := range config.SupportedWindowResolutions() {
		setMerchantShellTestResolution(t, size.X, size.Y)
		for _, test := range tests {
			keyboard := newWidgetTestKeyboard(0)
			dialog := NewHealerTreatmentDialog(HealerTreatmentDialogConfig{Keyboard: keyboard, Intent: intents[test.name]})
			require.Equal(t, test.text, dialog.window.flatText(), test.name)
			requireAmountWindowHoldsItsText(t, dialog.window, fmt.Sprintf("%s at %dx%d", test.name, size.X, size.Y))
			pressHealerTreatmentKey(dialog, keyboard, ebiten.KeyEnter)
			request, ok := dialog.ConsumePendingRequest()
			require.True(t, ok, "%s: Enter must send the request", test.name)
			require.Equal(t, intents[test.name].Request, request)
		}
	}
}

func TestHealerTreatmentDialogConfirmAndCancelLifecycle(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	shop := newTestHealerShopDialog(keyboard, testHealerShopState())
	pressHealerShopKey(shop, keyboard, ebiten.KeyEnter)
	pressHealerShopKey(shop, keyboard, ebiten.KeyEnter)
	intent, ok := shop.ConsumePendingTreatmentIntent()
	require.True(t, ok)

	confirmed := NewHealerTreatmentDialog(HealerTreatmentDialogConfig{Keyboard: keyboard, Intent: intent})
	pressHealerTreatmentKey(confirmed, keyboard, ebiten.KeyEnter)
	require.True(t, confirmed.IsDismissed())
	request, ok := confirmed.ConsumePendingRequest()
	require.True(t, ok)
	assert.Equal(t, game_state.HealerServiceRequest{
		Service:        references.HealerCure,
		CharacterIndex: 0,
	}, request)

	cancelled := NewHealerTreatmentDialog(HealerTreatmentDialogConfig{Keyboard: keyboard, Intent: intent})
	pressHealerTreatmentKey(cancelled, keyboard, ebiten.KeyEscape)
	assert.True(t, cancelled.IsDismissed())
	assert.True(t, cancelled.IsCancelled())
	_, ok = cancelled.ConsumePendingRequest()
	assert.False(t, ok)
}

func pressHealerTreatmentKey(
	dialog *HealerTreatmentDialog,
	keyboard *input.Keyboard,
	key ebiten.Key,
) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}
