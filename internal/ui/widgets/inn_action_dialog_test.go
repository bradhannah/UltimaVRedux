package widgets

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestInnActionDialogRestAffordableAndUnaffordableAreConfirmable(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	affordableIntent := testInnRestIntent(t, testInnShopState(), true)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard:     keyboard,
		Intent:       affordableIntent,
		IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body := compactInnText(dialog.window.flatText())

	require.Equal(t, "PREVIEW - Rest", compactInnText(dialog.GetTitleText()))
	require.Contains(t, body, "Gold 4321 -1234 3087")
	require.Contains(t, body, "Shamino and Mariahxxx will die during the rest.")
	require.Contains(t, body, "Wake at 6:03AM on 2-4-139.")
	require.Contains(t, body, "Active spells end.")
	require.Contains(t, body, "The unaffected heal fully.")
	require.Contains(t, body, "Dead and unchanged: Avatar.")
	require.NotContains(t, body, "Guest rooms")
	require.NotContains(t, body, "Active party")
	require.NotContains(t, body, "HP 40 ->")
	require.NotContains(t, body, "Approve")
	require.NotContains(t, body, "Confirm")
	require.NotContains(t, body, "PREVIEW: authority")
	require.Equal(t, "[Enter] Rest [Esc] Back", compactInnText(dialog.GetFooterGuidance()))
	require.True(t, dialog.IsPreview())
	requireAmountWindowHoldsItsText(t, dialog.window, "rest")

	pressInnActionKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, affordableIntent.Request, request)

	poor := testInnShopState()
	setTestInnGold(&poor, 100)
	poorIntent := testInnRestIntent(t, poor, false)
	keyboard = newWidgetTestKeyboard(0)
	dialog = NewInnActionDialog(InnActionDialogConfig{
		Keyboard: keyboard, Intent: poorIntent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body = compactInnText(dialog.window.flatText())
	require.Contains(t, body, "Thou hast not enough gold. It costs 1234 and the party has 100.")
	require.Contains(t, body, "The keeper turns the party out.")
	require.NotContains(t, body, "will die")
	require.Contains(t, compactInnText(dialog.GetFooterGuidance()), "[Enter] Rest anyway")
	pressInnActionKey(dialog, keyboard, ebiten.KeyNumpadEnter)
	request, ok = dialog.ConsumePendingRequest()
	require.True(t, ok, "unaffordable Rest approval must still emit its exact request")
	require.Equal(t, poorIntent.Request, request)
}

func TestInnActionDialogRestSummarizesUniversalHealing(t *testing.T) {
	state := testInnShopState()
	for index := range state.Rest.MemberProjections {
		projection := &state.Rest.MemberProjections[index]
		projection.StatusBefore = party_state.Good
		projection.StatusAfter = party_state.Good
		projection.HPBefore = 1
		projection.HPAfter = projection.MaxHP
		projection.Warnings = nil
	}
	state.Rest.Warnings = nil
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: newWidgetTestKeyboard(0), Intent: testInnRestIntent(t, state, false),
		IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body := compactInnText(dialog.GetBodyText())

	require.Contains(t, body, "Everyone fully heals.")
	require.NotContains(t, body, "Avatar:")
	require.NotContains(t, body, "Iolo:")
}

func TestInnActionDialogLeaveShowsExactDetailsAndSevereWarning(t *testing.T) {
	intent := testInnLeaveIntent(t, testInnShopState(), 2, false)
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: newWidgetTestKeyboard(0), Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body := compactInnText(dialog.window.flatText())

	require.Equal(t, "Leave Shamino", compactInnText(dialog.GetTitleText()))
	require.Contains(t, body, "No charge today.")
	require.Contains(t, body, "Monthly rate: 1234 gold.")
	require.Contains(t, body, "At least one month is billed.")
	require.Contains(t, body, "The rate is worked out again at check-out.")
	require.Contains(t, body, "Shamino stays poisoned.")
	require.Contains(t, body, "Party of 6 4 -1 3")
	require.Contains(t, body, "Rooms of 4 3 +1 4")
	require.Contains(t, body, "This removes the last living companion from the active party.")
	require.Contains(t, body, "[Enter] Leave [Esc] Back")
	requireAmountWindowHoldsItsText(t, dialog.window, "leave")
}

func TestInnActionDialogPickupShowsPoisonAndInsufficientGuardWarnings(t *testing.T) {
	poison := testInnPickupIntent(t, testInnShopState(), 0, true)
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: newWidgetTestKeyboard(0), Intent: poison, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body := compactInnText(dialog.window.flatText())
	require.Contains(t, body, "PREVIEW - Pick Up Geoffreyy")
	require.Contains(t, body, "1234 gold a month for 1 month")
	require.Contains(t, body, "Gold 4321 -1234 3087")
	require.Contains(t, body, "Geoffreyy is poisoned and will die when picked up.")
	require.NotContains(t, body, "Geoffreyy is poisoned.", "the death warning already says it")
	require.Contains(t, body, "Stayed 0 months.")
	require.Contains(t, body, "One month is the least billed.")
	require.Contains(t, body, "[Enter] Pick Up")
	requireAmountWindowHoldsItsText(t, dialog.window, "pick up")

	guard := testInnPickupIntent(t, testInnShopState(), 2, false)
	keyboard := newWidgetTestKeyboard(0)
	dialog = NewInnActionDialog(InnActionDialogConfig{
		Keyboard: keyboard, Intent: guard, IsKeyPressed: func(ebiten.Key) bool { return false },
	})
	body = compactInnText(dialog.window.flatText())
	require.Contains(t, body, "Thou hast not enough gold. It costs 4936 and the party has 4321.")
	require.Contains(t, body, "Asking anyway calls the guards.")
	require.Contains(t, body, "[Enter] Pick Up anyway")
	requireAmountWindowHoldsItsText(t, dialog.window, "pick up short")
	pressInnActionKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok, "guard-risk Pick Up remains confirmable")
	require.Equal(t, guard.Request, request)
}

func TestInnActionDialogDeadBlockedActionIsRejected(t *testing.T) {
	state := testInnShopState()
	intent := InnActionIntent{
		Action:         InnActionPickup,
		Request:        innPickupRequest(state, state.GuestRows[1]),
		Rest:           state.Rest,
		Pickup:         state.GuestRows[1],
		InnName:        state.Merchant.ShopName,
		RoomOccupancy:  state.RoomOccupancy,
		RoomCapacity:   state.RoomCapacity,
		PartySize:      state.PartySize,
		PartyCapacity:  state.PartyCapacity,
		MonthlyRate:    state.GuestRows[1].MonthlyRate,
		HasMonthlyRate: true,
		Preview:        false,
	}

	require.Panics(t, func() {
		NewInnActionDialog(InnActionDialogConfig{Keyboard: newWidgetTestKeyboard(0), Intent: intent})
	}, "dead stored guests must never reach the approval widget")
}

func TestInnActionDialogRequestAndIntentAreDefensiveCopies(t *testing.T) {
	intent := testInnRestIntent(t, testInnShopState(), false)
	wantRequest := intent.Request
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: keyboard, Intent: intent, IsKeyPressed: func(ebiten.Key) bool { return false },
	})

	intent.Rest.MemberProjections[0].Identity.Name = "MUTATED"
	intent.Rest.Warnings[0] = game_state.InnWarningNone
	copyIntent := dialog.GetIntent()
	require.Equal(t, "Avatar", copyIntent.Rest.MemberProjections[0].Identity.Name)
	require.Equal(t, game_state.InnWarningPoisonDeath, copyIntent.Rest.Warnings[0])
	copyIntent.Rest.MemberProjections[0].Identity.Name = "CHANGED AGAIN"
	require.Equal(t, "Avatar", dialog.GetIntent().Rest.MemberProjections[0].Identity.Name)

	pressInnActionKey(dialog, keyboard, ebiten.KeyEnter)
	request, ok := dialog.ConsumePendingRequest()
	require.True(t, ok)
	require.Equal(t, wantRequest, request)
	pressInnActionKey(dialog, keyboard, ebiten.KeyEnter)
	_, duplicate := dialog.ConsumePendingRequest()
	require.False(t, duplicate)
}

func TestInnActionDialogEscapeReturnsToParentAndReleaseDoesNotDismissParent(t *testing.T) {
	held := map[ebiten.Key]bool{}
	isPressed := func(key ebiten.Key) bool { return held[key] }
	keyboard := newWidgetTestKeyboard(0)
	parentConfig := testInnShopDialogConfig(keyboard, testInnShopState())
	parentConfig.IsKeyPressed = isPressed
	parent := NewInnShopDialog(parentConfig)
	pressInnShopKey(parent, keyboard, ebiten.KeyEnter)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	action := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: keyboard, Intent: intent, IsKeyPressed: isPressed,
	})
	pressInnActionKey(action, keyboard, ebiten.KeyEscape)
	require.True(t, action.IsCancelled())
	require.True(t, action.IsComplete())
	require.False(t, action.HasPendingRequest())

	held[ebiten.KeyEscape] = true
	parent.CancelActionApproval()
	parent.Update()
	require.False(t, parent.IsDismissed(), "the Escape that closed approval cannot bleed into the parent")
	held[ebiten.KeyEscape] = false
	parent.Update()
	require.False(t, parent.IsDismissed())
}

func TestInnActionDialogHeldEnterWaitsForRelease(t *testing.T) {
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true}
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: keyboard,
		Intent:   testInnRestIntent(t, testInnShopState(), false),
		IsKeyPressed: func(key ebiten.Key) bool {
			return held[key]
		},
	})

	dialog.Update()
	require.False(t, dialog.IsDismissed(), "held Enter cannot instantly confirm a newly opened approval")
	require.False(t, dialog.HasPendingRequest())
	held[ebiten.KeyEnter] = false
	dialog.Update()
	require.False(t, dialog.IsDismissed())
	pressInnActionKey(dialog, keyboard, ebiten.KeyEnter)
	require.True(t, dialog.IsDismissed())
	require.True(t, dialog.HasPendingRequest())
}

func TestInnActionDialogSemanticBackgroundFooterColorsAndNoMouseAssumptions(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	dialog := NewInnActionDialog(InnActionDialogConfig{
		Keyboard: newWidgetTestKeyboard(0),
		Intent:   testInnRestIntent(t, testInnShopState(), true),
		IsKeyPressed: func(ebiten.Key) bool {
			return false
		},
	})
	guidance := dialog.GetFooterGuidance()
	visible := strings.ToLower(dialog.GetTitleText() + " " + dialog.GetBodyText() + " " + guidance)
	require.NotContains(t, visible, "mouse")
	require.NotContains(t, visible, "click")
	require.NotContains(t, visible, "tab")
	requireKeyedGuidance(t, guidance)

	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Dispose)
	dialog.Draw(screen)
	requireImageRegionContainsColor(t, screen, dialog.GetBodyBounds(), MerchantDialogBackgroundColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlKeyColor)
	requireImageRegionContainsColor(t, screen, dialog.window.frame.footerText, DialogControlHintColor)
	require.True(t, dialog.IsVisible())
	require.False(t, dialog.IsComplete())
	dialog.ShowWithoutAnimation()
	require.True(t, dialog.IsVisible())
}

func TestInnActionDialogStrictTypedContracts(t *testing.T) {
	valid := InnActionDialogConfig{
		Keyboard:     newWidgetTestKeyboard(0),
		Intent:       testInnRestIntent(t, testInnShopState(), false),
		IsKeyPressed: func(ebiten.Key) bool { return false },
	}
	tests := []struct {
		name   string
		mutate func(*InnActionDialogConfig)
	}{
		{name: "keyboard", mutate: func(config *InnActionDialogConfig) { config.Keyboard = nil }},
		{name: "action", mutate: func(config *InnActionDialogConfig) { config.Intent.Request.Action = InnActionLeave }},
		{name: "charge", mutate: func(config *InnActionDialogConfig) { config.Intent.Request.Rest.ReviewedCharge++ }},
		{name: "blocked rest", mutate: func(config *InnActionDialogConfig) {
			config.Intent.Rest.BlockReason = game_state.InnRestBlockNoVacancy
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			config.Intent = cloneInnActionIntent(valid.Intent)
			test.mutate(&config)
			require.Panics(t, func() { NewInnActionDialog(config) })
		})
	}
}

func testInnRestIntent(
	t *testing.T,
	state game_state.InnShopState,
	preview bool,
) InnActionIntent {
	t.Helper()
	keyboard := newWidgetTestKeyboard(0)
	config := testInnShopDialogConfig(keyboard, state)
	config.Preview = preview
	parent := NewInnShopDialog(config)
	pressInnShopKey(parent, keyboard, ebiten.KeyEnter)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	return intent
}

func testInnLeaveIntent(
	t *testing.T,
	state game_state.InnShopState,
	row int,
	preview bool,
) InnActionIntent {
	t.Helper()
	keyboard := newWidgetTestKeyboard(0)
	config := testInnShopDialogConfig(keyboard, state)
	config.Preview = preview
	parent := NewInnShopDialog(config)
	pressInnShopKey(parent, keyboard, ebiten.KeyL)
	for range row {
		pressInnShopKey(parent, keyboard, ebiten.KeyDown)
	}
	pressInnShopKey(parent, keyboard, ebiten.KeyEnter)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	return intent
}

func testInnPickupIntent(
	t *testing.T,
	state game_state.InnShopState,
	row int,
	preview bool,
) InnActionIntent {
	t.Helper()
	keyboard := newWidgetTestKeyboard(0)
	config := testInnShopDialogConfig(keyboard, state)
	config.Preview = preview
	parent := NewInnShopDialog(config)
	pressInnShopKey(parent, keyboard, ebiten.KeyP)
	for range row {
		pressInnShopKey(parent, keyboard, ebiten.KeyDown)
	}
	pressInnShopKey(parent, keyboard, ebiten.KeyEnter)
	intent, ok := parent.ConsumePendingActionIntent()
	require.True(t, ok)

	return intent
}

func pressInnActionKey(dialog *InnActionDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}
