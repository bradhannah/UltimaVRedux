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

func TestTavernActionDialogNormalActionsEmitExactRequestsUnchanged(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	tests := []struct {
		name   string
		intent TavernActionIntent
		text   string
	}{
		{name: "meal", intent: testTavernMealIntent(t), text: "Order Mutton 4 gold each for 3 diners " +
			"Have Then Gold 1000 -12 988 [Enter] Order [Esc] Back"},
		{name: "drink", intent: testTavernOrdinaryDrinkIntent(t), text: "Order Ale 1 gold each for 3 drinks " +
			"Have Then Gold 1000 -3 997 [Enter] Order [Esc] Back"},
		{name: "provisions", intent: testTavernProvisionIntent(t), text: "Packs < 1 > of "},
		{name: "information", intent: testTavernInformationIntent(t), text: "Have Then Gold 1000 -50 950 " +
			"[Enter] Pay [Esc] Back"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			keyboard := newWidgetTestKeyboard(0)
			dialog := NewTavernActionDialog(TavernActionDialogConfig{
				Keyboard: keyboard, Intent: test.intent, IsKeyPressed: func(ebiten.Key) bool { return false },
			})
			require.Contains(t, dialog.window.flatText(), test.text)
			requireAmountWindowHoldsItsText(t, dialog.window, test.name)
			if test.intent.Action == TavernActionProvision {
				require.Contains(t, dialog.window.flatText(), "[Left/Right/0-9] Packs [M] Most [Enter] Buy 1 [Esc] Back")
			}
			require.Equal(t, test.intent, dialog.GetIntent())
			if test.intent.Action == TavernActionProvision {
				detached := dialog.GetIntent()
				detached.Provision.Approvals[0].TotalCost++
				require.NotEqual(t, detached, dialog.GetIntent())
			}

			pressTavernActionKey(dialog, keyboard, ebiten.KeyEnter)
			request, ok := dialog.ConsumePendingRequest()
			require.True(t, ok)
			require.Equal(t, test.intent.Request, request)
			_, duplicate := dialog.ConsumePendingRequest()
			require.False(t, duplicate)
			require.True(t, dialog.IsComplete())
			require.False(t, dialog.IsVisible())
		})
	}
}

func TestTavernActionDialogWarningUsesImmersiveEnterOrEscapeChoice(t *testing.T) {
	state := testTavernState(references.TavernMenuMuttonAleRations)
	setTestTavernDrunk(&state, 3)
	intent := testTavernIntentForOrderKey(t, state, ebiten.KeyA)
	require.True(t, intent.Drink.Warning)

	t.Run("Enter accepts another drink", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := NewTavernActionDialog(TavernActionDialogConfig{
			Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
		})
		body := dialog.window.flatText()
		require.Contains(t, body, "One More Drink? 1 gold each for 3 drinks")
		require.Contains(t, body, "But haven't ye had enough to drink?")
		require.Contains(t, body, "Have Then Gold 1000 -3 997")
		require.Contains(t, body, "[Enter] Another Drink [Esc] I've Had Enough")
		requireAmountWindowHoldsItsText(t, dialog.window, "warned drink")
		require.NotContains(t, body, "smashed")
		require.NotContains(t, body, "karma")
		require.NotContains(t, body, "customer")
		pressTavernActionKey(dialog, keyboard, ebiten.KeyEnter)
		request, ok := dialog.ConsumePendingRequest()
		require.True(t, ok)
		require.Equal(t, intent.Request, request)
		require.Equal(t, game_state.TavernDrinkApprovalContinue, request.Drink.Approval)
	})

	t.Run("Escape has enough", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := NewTavernActionDialog(TavernActionDialogConfig{
			Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
		})
		pressTavernActionKey(dialog, keyboard, ebiten.KeyEscape)
		request, ok := dialog.ConsumePendingRequest()
		require.True(t, ok)
		require.Equal(t, intent.StopRequest, request)
		require.Equal(t, game_state.TavernStopRequest{
			ReviewedMerchantID: state.Merchant.ID, ReviewedDrunk: 3,
		}, request.Stop)
	})

	t.Run("letter shortcuts are inert", func(t *testing.T) {
		keyboard := newWidgetTestKeyboard(0)
		dialog := NewTavernActionDialog(TavernActionDialogConfig{
			Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
		})
		pressTavernActionKey(dialog, keyboard, ebiten.KeyS)
		pressTavernActionKey(dialog, keyboard, ebiten.KeyC)
		require.False(t, dialog.HasPendingRequest())
		require.False(t, dialog.IsDismissed())
	})
}

func TestTavernActionDialogEscapeCancelsWithoutRequestAndParentReturnsToWine(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	parent := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuWineCheese), nil)
	pressTavernShopKey(parent, keyboard, ebiten.KeyW)
	pressTavernShopKey(parent, keyboard, ebiten.KeyD)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	action := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	pressTavernActionKey(action, keyboard, ebiten.KeyEscape)
	require.True(t, action.IsCancelled())
	require.False(t, action.HasPendingRequest())
	parent.CancelActionApproval()
	require.Equal(t, TavernOrderWinePicker, parent.GetOrderSubflow())
	require.Equal(t, 3, parent.GetSelectedWineIndex())
	require.False(t, parent.IsDismissed())
}

func TestTavernActionDialogInformationAppearsOnlyAfterAuthorityQuote(t *testing.T) {
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	keyboard := newWidgetTestKeyboard(0)
	parent := newTestTavernShopDialog(keyboard, state, queue.Append)
	pressTavernShopKey(parent, keyboard, ebiten.KeyC)
	queue.Push("honesty")
	parent.Update()
	queue.Push("")
	require.Contains(t, strings.ToLower(parent.GetWorkspaceText()), "honesty")
	pressTavernShopKey(parent, keyboard, ebiten.KeyEnter)
	topic, ok := parent.ConsumePendingTopicIntent()
	require.True(t, ok)
	parent.ApplyInformationQuote(topic, testTavernInformationQuote(parent.GetState(), "hone", 50), "I may know someone.")
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	action := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body := action.window.flatText()
	require.Contains(t, body, "Buy Information I may know someone. Have Then Gold 1000 -50 950")
	require.NotContains(t, body, "Recognized subject")
	require.NotContains(t, body, "hone")
	require.NotContains(t, body, "honesty", "approval exposes authority's recognized subject, not unmatched raw input")
}

func TestTavernActionDialogHeldControlsWaitForReleaseWithoutAccidentalChoice(t *testing.T) {
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true, ebiten.KeyEscape: true, ebiten.KeyS: true, ebiten.KeyC: true}
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: keyboard, Intent: testTavernMealIntent(t),
		IsKeyPressed: func(key ebiten.Key) bool { return held[key] },
	})
	dialog.Update()
	require.False(t, dialog.IsDismissed())
	require.False(t, dialog.HasPendingRequest())

	for key := range held {
		held[key] = false
	}
	dialog.Update()
	require.False(t, dialog.IsDismissed(), "release frame must not confirm or cancel")
	pressTavernActionKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.HasPendingRequest())
}

func TestTavernActionDialogPreviewResponsiveSemanticFooterAndLifecycle(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	intent := testTavernMealIntent(t)
	intent.Preview = true
	dialog := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: newWidgetTestKeyboard(0), Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	require.True(t, dialog.IsPreview())
	require.Contains(t, compactTavernText(dialog.GetTitleText()), "PREVIEW - Order Mutton")
	require.False(t, dialog.GetBounds().Empty())

	requireKeyedGuidance(t, dialog.GetFooterGuidance())
	require.True(t, dialog.IsVisible())
	dialog.Dismiss()
	require.True(t, dialog.IsComplete())
	require.False(t, dialog.IsVisible())
	dialog.ShowWithoutAnimation()
	require.False(t, dialog.IsVisible())
}

func TestTavernActionDialogProvisionResizePreservesQuantityAndRequest(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: keyboard,
		Intent:   testTavernProvisionIntent(t),
		IsKeyPressed: func(ebiten.Key) bool {
			return false
		},
	})
	pressTavernActionKey(dialog, keyboard, ebiten.KeyRight)
	lowBounds := dialog.GetBounds()
	wantBody := dialog.GetBodyText()
	require.Equal(t, 2, dialog.provisionPacks)

	setResponsiveDialogTestResolution(t, 1280, 720)
	require.Greater(t, dialog.GetBounds().Dx(), lowBounds.Dx())
	require.Equal(t, 2, dialog.provisionPacks)
	require.Equal(t, wantBody, dialog.GetBodyText())

	pressTavernActionKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, 2, request.Provision.PackCount)
}

func TestTavernActionDialogRejectsTamperedTypedIntent(t *testing.T) {
	valid := testTavernMealIntent(t)
	tests := []struct {
		name   string
		mutate func(*TavernActionDialogConfig)
	}{
		{name: "missing keyboard", mutate: func(config *TavernActionDialogConfig) { config.Keyboard = nil }},
		{name: "wrong action", mutate: func(config *TavernActionDialogConfig) {
			config.Intent.Request.Action = TavernActionDrink
		}},
		{name: "changed charge", mutate: func(config *TavernActionDialogConfig) {
			config.Intent.Request.Meal.ReviewedCharge++
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := TavernActionDialogConfig{
				Keyboard: newWidgetTestKeyboard(0), Intent: valid, IsKeyPressed: func(ebiten.Key) bool { return false },
			}
			test.mutate(&config)
			require.Panics(t, func() { NewTavernActionDialog(config) })
		})
	}
}

func TestTavernActionDialogRejectsTamperedProvisionProjection(t *testing.T) {
	intent := testTavernProvisionIntent(t)
	intent.Provision.Approvals[0].TotalCost++

	require.Panics(t, func() {
		NewTavernActionDialog(TavernActionDialogConfig{
			Keyboard: newWidgetTestKeyboard(0), Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
		})
	})
}

func testTavernMealIntent(t *testing.T) TavernActionIntent {
	t.Helper()

	return testTavernIntentForOrderKey(t, testTavernState(references.TavernMenuMuttonAleRations), ebiten.KeyM)
}

func testTavernOrdinaryDrinkIntent(t *testing.T) TavernActionIntent {
	t.Helper()

	return testTavernIntentForOrderKey(t, testTavernState(references.TavernMenuMuttonAleRations), ebiten.KeyA)
}

func testTavernProvisionIntent(t *testing.T) TavernActionIntent {
	t.Helper()
	keyboard := newWidgetTestKeyboard(0)
	parent := newTestTavernShopDialog(keyboard, testTavernState(references.TavernMenuStoutFruitProvisions), nil)
	pressTavernShopKey(parent, keyboard, ebiten.KeyP)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)
	intent.ProvisionPitch = `"Fresh provisions for the road."`

	return intent
}

func testTavernInformationIntent(t *testing.T) TavernActionIntent {
	t.Helper()
	queue := newTavernRuneQueue()
	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	keyboard := newWidgetTestKeyboard(0)
	parent := newTestTavernShopDialog(keyboard, state, queue.Append)
	pressTavernShopKey(parent, keyboard, ebiten.KeyC)
	queue.Push("honesty")
	parent.Update()
	queue.Push("")
	pressTavernShopKey(parent, keyboard, ebiten.KeyEnter)
	topic, ok := parent.ConsumePendingTopicIntent()
	require.True(t, ok)
	parent.ApplyInformationQuote(topic, testTavernInformationQuote(parent.GetState(), "hone", 50), "I may know someone.")
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	return intent
}

func testTavernIntentForOrderKey(
	t *testing.T,
	state game_state.TavernShopState,
	key ebiten.Key,
) TavernActionIntent {
	t.Helper()
	keyboard := newWidgetTestKeyboard(0)
	parent := newTestTavernShopDialog(keyboard, state, nil)
	pressTavernShopKey(parent, keyboard, key)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)
	if intent.Drink.Warning {
		intent.DrinkWarning = `"I beg thy pardon, sir," says Tika. "But haven't ye had enough to drink?"`
	}

	return intent
}

func pressTavernActionKey(dialog *TavernActionDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func TestTavernActionDialogNativeSubjectToOrderQuantityAndConfirmation(t *testing.T) {
	previousTPS := ebiten.TPS()
	ebiten.SetTPS(60)
	t.Cleanup(func() { ebiten.SetTPS(previousTPS) })
	setMerchantShellTestResolution(t, 1280, 720)
	f := newTavernNativeTest(t)
	saveField := &tavernNativeTestField{rawStranded: &f.rawStranded}
	var saved SaveDescriptionEntryResult
	save := newSaveDescriptionEntryModal(SaveDescriptionEntryModalConfig{
		Keyboard: f.keyboard, IsKeyPressed: f.keyboard.IsKeyHeld, MaxRunes: 80,
		OnResult: func(value string, submitted bool) {
			saved = SaveDescriptionEntryResult{Text: value, Submitted: submitted}
		},
	}, saveField).AutoCloseAndPop(&f.stack)
	save.isWindowFocused = func() bool { return true }
	f.stack.PushModalDialog(save)
	f.tick()
	saveField.nextInput = func() { saveField.commit("Before buying rations") }
	f.tick()
	f.press(ebiten.KeyEnter)
	require.Equal(t, SaveDescriptionEntryResult{Text: "Before buying rations", Submitted: true}, saved)
	require.False(t, saveField.IsFocused())
	require.Zero(t, f.stack.Count())
	require.True(t, f.rawStranded, "the save editor has ended a real simulated native input session")

	state := testTavernState(references.TavernMenuMuttonAleRations)
	state.CustomerEligible = true
	config := testTavernShopConfig(f.keyboard, state, nil)
	config.AppendInputChars = nil
	config.IsKeyPressed = f.keyboard.IsKeyHeld
	f.field = &tavernNativeTestField{rawStranded: &f.rawStranded}
	f.dialog = newTavernShopDialog(config, f.field)
	f.dialog.isWindowFocused = func() bool { return true }
	f.stack.PushModalDialog(f.dialog)
	f.tick()
	f.press(ebiten.KeyC)
	f.typeText("honesty")
	require.True(t, f.field.IsFocused())
	require.Equal(t, "honesty", f.dialog.GetTopicInput())
	f.press(ebiten.KeyEscape)
	require.Equal(t, TavernShopOrder, f.dialog.GetMode())
	require.False(t, f.field.IsFocused())
	pumps := f.field.pumps
	f.press(ebiten.KeyR)
	intent, ok := f.dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	require.Equal(t, TavernActionProvision, intent.Action)
	intent.ProvisionPitch = "Fresh rations for the road."
	// The default adapter must use this keyboard, rather than Ebitengine globals.
	action := NewTavernActionDialog(TavernActionDialogConfig{Keyboard: f.keyboard, Intent: intent})
	f.stack.PushModalDialog(action)
	f.held[ebiten.KeyRight] = true
	f.tick()
	require.Equal(t, 2, action.GetProvisionPackCount())
	for range 18 {
		f.tick()
	}
	require.Equal(t, 3, action.GetProvisionPackCount())
	f.held[ebiten.KeyRight] = false
	f.tick()
	f.press(ebiten.KeyM)
	require.Equal(t, state.Provision.MaximumUsefulPacks, action.GetProvisionPackCount())
	require.Greater(t, action.GetProvisionPackCount(), int(state.Gold)/state.Provision.PricePerPack,
		"maximum is useful packs, not affordable packs")
	require.Contains(t, action.window.flatText(), "The gold pays for only")
	require.Contains(t, action.window.flatText(), "[Left/Right/0-9] Packs [M] Most [Enter] Buy ")
	f.press(ebiten.KeyEnter)
	request, ok := action.ConsumePendingRequest()
	require.True(t, ok)
	want := intent.Request
	want.Provision.PackCount = state.Provision.MaximumUsefulPacks
	require.Equal(t, want, request)
	_, duplicate := action.ConsumePendingRequest()
	require.False(t, duplicate)
	f.dialog.AcceptActionApproval(request)
	f.stack.MustRemoveWidgetInstance(action)
	require.Same(t, f.dialog, f.stack.PeekTopModalDialog())
	require.Equal(t, TavernShopAwaitingResult, f.dialog.GetPhase())
	require.Equal(t, state, f.dialog.GetState(), "widgets only emit the reviewed request")
	require.Equal(t, pumps, f.field.pumps, "Order and quantity never revive the native Subject session")
	require.True(t, f.rawStranded, "this path must not depend on restoring raw character delivery")
}

// An order the party cannot pay for is still a real request, which the keeper
// answers by ending service: the window says so and Enter stays.
func TestTavernActionDialogSaysWhatAnUnpaidOrderDoesAndStillSendsIt(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	poor := testTavernState(references.TavernMenuMuttonAleRations)
	setTestTavernGold(&poor, 5)
	tests := map[string]struct {
		key  ebiten.Key
		want string
	}{
		"meal": {key: ebiten.KeyM, want: "Order Mutton 4 gold each for 3 diners Not enough gold. It costs 12 and " +
			"the party has 5. Ordering anyway ends service. [Enter] Order anyway [Esc] Back"},
		"rations": {key: ebiten.KeyR, want: "Buy Rations 15 gold for a pack of 25 food Not enough gold. A pack " +
			"costs 15 and the party has 5. Asking anyway ends service. [Enter] Ask anyway [Esc] Back"},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			intent := testTavernIntentForOrderKey(t, poor, test.key)
			if intent.Action == TavernActionProvision {
				intent.ProvisionPitch = `"Fresh rations for the road."`
			}
			keyboard := newWidgetTestKeyboard(0)
			dialog := NewTavernActionDialog(TavernActionDialogConfig{
				Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
			})
			require.Equal(t, test.want, dialog.window.flatText())
			requireAmountWindowHoldsItsText(t, dialog.window, name)
			pressTavernActionKey(dialog, keyboard, ebiten.KeyRight)
			pressTavernActionKey(dialog, keyboard, ebiten.KeyEnter)
			request, ok := dialog.ConsumePendingRequest()
			require.True(t, ok, "the request still reaches the keeper")
			require.Equal(t, intent.Request, request)
		})
	}

	starving := testTavernState(references.TavernMenuMuttonAleRations)
	starving.Food, starving.Provision.FoodBefore, starving.Meal.FoodBefore = 2, 2, 2
	setTestTavernGold(&starving, 5)
	intent := testTavernIntentForOrderKey(t, starving, ebiten.KeyR)
	intent.ProvisionPitch = `"Fresh rations for the road."`
	dialog := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: newWidgetTestKeyboard(0), Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	require.Contains(t, dialog.window.flatText(), "Asking anyway ends service. The keeper may spare 1 or 2 food.")
	requireAmountWindowHoldsItsText(t, dialog.window, "charity")
}

// Packs past what the gold covers are left out by the keeper; the grid shows what
// will be bought and one line says so, in a window that keeps its size.
func TestTavernActionDialogProvisionSaysWhenTheGoldCoversFewerPacks(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	some := testTavernState(references.TavernMenuStoutFruitProvisions)
	setTestTavernGold(&some, 70)
	intent := testTavernIntentForOrderKey(t, some, ebiten.KeyP)
	intent.ProvisionPitch = `"Fresh provisions for the road."`
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewTavernActionDialog(TavernActionDialogConfig{
		Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	before := dialog.GetBounds()
	require.NotContains(t, dialog.window.flatText(), "pays for only")
	pressTavernActionKey(dialog, keyboard, ebiten.Key3)
	flat := dialog.window.flatText()
	require.Contains(t, flat, "Packs < 3 > of ")
	require.Contains(t, flat, "Food 50 +50 100 Gold 70 -60 10 The gold pays for only 2 packs.")
	require.Equal(t, before, dialog.GetBounds())
	requireAmountWindowHoldsItsText(t, dialog.window, "partly paid")
}
