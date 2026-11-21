package widgets

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestInnShopDialogModesTabsAndNavigation(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, testInnShopState())

	require.Equal(t, InnShopRest, dialog.GetMode())
	require.Contains(t, dialog.GetModeHeaderText(), "Rest")
	require.Contains(t, dialog.GetModeHeaderText(), "Leave")
	require.Contains(t, dialog.GetModeHeaderText(), "Pick Up")
	require.NotContains(t, dialog.GetModeHeaderText(), "[", "the tab titles carry no key letters")
	require.Equal(t, []shopTab{{"Rest", true}, {"Leave", false}, {"Pick Up", false}}, dialog.tabs())
	require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[R] Rest   [L] Leave   [P] Pick Up")
	require.True(t, dialog.GetTableBounds().Empty(), "Rest is a consequence card, not a list")
	require.False(t, dialog.GetCardBounds().Empty())

	pressInnShopKey(dialog, keyboard, ebiten.KeyLeft)
	require.Equal(t, InnShopPickup, dialog.GetMode())
	pressInnShopKey(dialog, keyboard, ebiten.KeyRight)
	require.Equal(t, InnShopRest, dialog.GetMode())
	pressInnShopKey(dialog, keyboard, ebiten.KeyL)
	require.Equal(t, InnShopLeave, dialog.GetMode())
	require.Equal(t, []string{"Avatar", "Iolo", "Shamino", "Mariahxxx"}, innLeaveNames(dialog.GetLeaveRows()))
	pressInnShopKey(dialog, keyboard, ebiten.KeyP)
	require.Equal(t, InnShopPickup, dialog.GetMode())
	pressInnShopKey(dialog, keyboard, ebiten.KeyR)
	require.Equal(t, InnShopRest, dialog.GetMode())

	pressInnShopKey(dialog, keyboard, ebiten.KeyTab)
	require.Equal(t, InnShopLeave, dialog.GetMode())
	require.NotContains(t, dialog.GetShell().GetFooterView().Guidance, "Left/Right")

	pressInnShopKey(dialog, keyboard, ebiten.KeyL)
	for range 10 {
		pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
	}
	require.Equal(t, len(dialog.GetLeaveRows())-1, dialog.GetSelectedIndex())
	for range 10 {
		pressInnShopKey(dialog, keyboard, ebiten.KeyUp)
	}
	require.Zero(t, dialog.GetSelectedIndex(), "Up/Down must clamp rather than wrap")

	require.Equal(t, []shopTab{{"Rest", false}, {"Leave", true}, {"Pick Up", false}}, dialog.tabs())
}

func TestInnShopDialogRestCardShowsConciseDecisionDetails(t *testing.T) {
	state := testInnShopState()
	dialog := newTestInnShopDialog(newWidgetTestKeyboard(0), state)
	detail := compactInnText(dialog.GetDetailText())

	require.Contains(t, detail, "Nightly charge: 1234 gold")
	require.Contains(t, detail, "fully recover HP and class MP")
	require.Contains(t, detail, "6:03AM on 2-4-139")
	require.Contains(t, detail, "next qualifying 6 o'clock hour")
	require.Contains(t, detail, "Poisoned: Shamino, Mariahxxx")
	require.Contains(t, detail, "Will die during Rest")
	require.NotContains(t, detail, "Guest rooms:")
	require.NotContains(t, detail, "Active party:")
	require.NotContains(t, detail, "Gold after listed payment")
	require.NotContains(t, detail, "4321 -> 3087")

	visible := strings.Join([]string{
		dialog.GetModeHeaderText(), dialog.GetDetailText(), dialog.GetLatestExchange(),
		dialog.GetShell().GetHeaderView().ShopIdentity, dialog.GetShell().GetHeaderView().Owner,
	}, " ")
	require.NotContains(t, visible, state.PricingCharacterName)
	require.NotContains(t, strings.ToLower(visible), "intelligence")
}

func TestInnShopDialogRestUnaffordableConfirmsButNoVacancyBlocksInline(t *testing.T) {
	poor := testInnShopState()
	setTestInnGold(&poor, 100)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, poor)
	detail := compactInnText(dialog.GetDetailText())
	require.Contains(t, detail, "1134 short")

	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok, "unaffordable Rest must still reach approval")
	require.Equal(t, InnActionRest, intent.Action)
	require.False(t, intent.Rest.Affordable)
	require.Equal(t, uint16(1234), intent.Request.Rest.ReviewedCharge)
	dialog.CancelActionApproval()

	full := testInnShopState()
	full.RoomOccupancy = full.RoomCapacity
	full.Rest.BlockReason = game_state.InnRestBlockNoVacancy
	full.GuestRows = append(full.GuestRows, testInnGuest(
		7, "Gornagain", party_state.Good, 1, true, game_state.InnPickupBlockNone, nil,
	))
	keyboard = newWidgetTestKeyboard(0)
	dialog = newTestInnShopDialog(keyboard, full)
	require.Contains(t, compactInnText(dialog.GetDetailText()), "We have no room available")

	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, ok = dialog.ConsumePendingActionIntent()
	require.False(t, ok)
	require.Equal(t, InnShopBrowse, dialog.GetPhase())
	require.Contains(t, dialog.GetInlineFeedback(), "Pick up a guest")
	require.False(t, dialog.IsDismissed(), "no-vacancy feedback must stay inline")
}

func TestInnShopDialogLeaveRowsKeepFullNamesReasonsAndLastLivingReview(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, testInnShopState())
	pressInnShopKey(dialog, keyboard, ebiten.KeyL)

	require.Equal(t, [3]string{"Avatar", "Dead", "Cannot stay"}, dialog.GetLeaveRowText(0))
	require.Equal(t, [3]string{"Iolo", "Dead", "Dead"}, dialog.GetLeaveRowText(1))
	require.Equal(t, "Shamino", dialog.GetLeaveRowText(2)[0])
	require.Equal(t, "Last one living", dialog.GetLeaveRowText(2)[2])
	require.Equal(t, "Mariahxxx", dialog.GetLeaveRowText(3)[0], "full nine-character active names stay intact")
	require.Contains(t, compactInnText(dialog.GetDetailText()), "Thy friends will not leave thee!")

	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, ok := dialog.ConsumePendingActionIntent()
	require.False(t, ok)
	require.Contains(t, dialog.GetInlineFeedback(), "Thy friends")

	pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, ok = dialog.ConsumePendingActionIntent()
	require.False(t, ok)
	require.Contains(t, dialog.GetInlineFeedback(), "dead")

	pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
	detail := compactInnText(dialog.GetDetailText())
	require.Contains(t, detail, "No charge today")
	require.Contains(t, detail, "Rate: 1234 gold per month")
	require.Contains(t, detail, "Status: Poisoned")
	require.Contains(t, detail, "last living companion")
	require.NotContains(t, detail, "Minimum stay")
	require.NotContains(t, detail, "Future rate")
	require.NotContains(t, detail, "Guest rooms")

	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok, "last living member remains confirmable")
	require.Equal(t, InnActionLeave, intent.Action)
	require.Equal(t, "Shamino", intent.Request.Leave.Character.Name)
	require.Contains(t, intent.Leave.Warnings, game_state.InnWarningLastLivingMember)
}

func TestInnShopDialogLeaveMinimumPartyAndNoVacancyAreVisibleBlocked(t *testing.T) {
	tests := []struct {
		name   string
		reason game_state.InnLeaveBlockReason
		text   string
	}{
		{name: "minimum party", reason: game_state.InnLeaveBlockMinimumParty, text: "At least one active party member"},
		{name: "no vacancy", reason: game_state.InnLeaveBlockNoVacancy, text: "We have no room available"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := testInnShopState()
			row := &state.LeaveRows[3]
			row.Eligible = false
			row.BlockReason = test.reason
			row.PartySizeAfter = state.PartySize
			row.OccupancyAfter = state.RoomOccupancy
			keyboard := newWidgetTestKeyboard(0)
			dialog := newTestInnShopDialog(keyboard, state)
			pressInnShopKey(dialog, keyboard, ebiten.KeyL)
			for range 3 {
				pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
			}

			require.Contains(t, compactInnText(dialog.GetDetailText()), test.text)
			pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
			_, ok := dialog.ConsumePendingActionIntent()
			require.False(t, ok)
			require.Contains(t, dialog.GetInlineFeedback(), test.text)
		})
	}
}

func TestInnShopDialogPickupRowsFourDigitChargesAndConfirmableWarnings(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, testInnShopState())
	pressInnShopKey(dialog, keyboard, ebiten.KeyP)

	require.Equal(t, [4]string{"Geoffreyy", "Poisoned", "0/1", "1234"}, dialog.GetPickupRowText(0))
	require.Equal(t, [4]string{"Katrina", "Good", "4/4", "4936"}, dialog.GetPickupRowText(2))
	require.Equal(t, DialogLabelColor, innPickupRowColor(dialog.state.GuestRows[0], false))
	require.Equal(t, DialogEmptyColor, innPickupRowColor(dialog.state.GuestRows[1], false))
	require.Equal(t, DialogLabelColor, innPickupRowColor(dialog.state.GuestRows[2], false))

	detail := compactInnText(dialog.GetDetailText())
	require.Contains(t, detail, "PICK UP: Geoffreyy")
	require.Contains(t, detail, "Months stayed / to pay for: 0 / 1")
	require.Contains(t, detail, "Minimum charge is one month")
	require.Contains(t, detail, "Rate: 1234 gold per month")
	require.Contains(t, detail, "That will be 1234 gold, please.")
	require.Contains(t, detail, "Geoffreyy is poisoned and will die")
	require.NotContains(t, detail, "Gold after listed payment")
	require.NotContains(t, detail, "Guest rooms")

	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok, "poisoned guest remains confirmable")
	require.Equal(t, "Geoffreyy", intent.Request.Pickup.Character.Name)
	dialog.CancelActionApproval()

	dialog.receiptText = "Old receipt"
	dialog.populateDetailOutput()
	pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Empty(t, dialog.GetReceiptText(), "row selection clears the old receipt")
	require.Contains(t, compactInnText(dialog.GetDetailText()), "dead and remains stored")
	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, ok = dialog.ConsumePendingActionIntent()
	require.False(t, ok, "dead stored guests never reach approval")

	pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
	detail = compactInnText(dialog.GetDetailText())
	require.Contains(t, detail, "That will be 4936 gold, please.")
	require.Contains(t, detail, "615 short")
	require.Contains(t, detail, "the guards are called")
	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok = dialog.ConsumePendingActionIntent()
	require.True(t, ok, "insufficient funds remains consequential and confirmable")
	require.False(t, intent.Pickup.Affordable)
}

func TestInnShopDialogPartyFullPickupIsBlocked(t *testing.T) {
	state := testInnShopState()
	row := &state.GuestRows[0]
	row.Status = party_state.Good
	row.Projection.StatusBefore = party_state.Good
	row.Projection.StatusAfter = party_state.Good
	row.Warnings = nil
	row.Projection.Warnings = nil
	row.Confirmable = false
	row.BlockReason = game_state.InnPickupBlockPartyFull
	row.PartySizeAfter = state.PartySize
	row.RoomOccupancyAfter = state.RoomOccupancy
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, state)
	pressInnShopKey(dialog, keyboard, ebiten.KeyP)

	require.Contains(t, compactInnText(dialog.GetDetailText()), "active party is full")
	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	_, ok := dialog.ConsumePendingActionIntent()
	require.False(t, ok)
	require.Contains(t, dialog.GetInlineFeedback(), "Leave a companion")
}

func TestInnShopDialogPreviewLabelsKeyboardOnlyFooterAndSemanticBacking(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	config := testInnShopDialogConfig(newWidgetTestKeyboard(0), testInnShopState())
	config.Preview = true
	dialog := NewInnShopDialog(config)

	require.True(t, dialog.IsPreview())
	require.Contains(t, dialog.GetModeHeaderText(), "PREVIEW")
	guidance := dialog.GetShell().GetFooterView().Guidance
	require.Contains(t, guidance, "[H] Help")
	require.NotContains(t, guidance, "R/L/P")
	require.NotContains(t, strings.ToLower(guidance), "tab")
	require.NotContains(t, strings.ToLower(guidance), "mouse")
	require.NotContains(t, strings.ToLower(guidance), "click")
	requireKeyedGuidance(t, guidance)

	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Dispose)
	dialog.Draw(screen)
	requireImageRegionContainsColor(t, screen, dialog.GetCardBounds(), MerchantDialogBackgroundColor)
	requireImageRegionContainsColor(t, screen, dialog.modeBounds, DialogActiveTabBackground)
	requireImageRegionContainsColor(t, screen, dialog.modeBounds, DialogTabColor)
	requireImageRegionContainsColor(t, screen, dialog.GetShell().footerText, DialogControlKeyColor)
	requireImageRegionContainsColor(t, screen, dialog.GetShell().footerText, DialogControlHintColor)
	backgroundInset := dialog.GetShell().pixelUnit() * 2
	point := dialog.GetShell().GetWorkspaceBounds().Min.Add(image.Pt(backgroundInset, backgroundInset))
	requireColorRGBA(t, MerchantDialogBackgroundColor, screen.At(point.X, point.Y))
}

func TestInnShopDialogHelpExplainsLeaveAndPickupWithoutLeavingBrowse(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, testInnShopState())

	pressInnShopKey(dialog, keyboard, ebiten.KeyH)

	require.True(t, dialog.IsHelpVisible())
	require.Equal(t, InnShopBrowse, dialog.GetPhase())
	help := compactInnText(dialog.GetDetailText())
	require.Contains(t, help, "[L]eave a companion")
	require.Contains(t, help, "costs nothing today")
	require.Contains(t, help, "At least one month is billed")
	require.Contains(t, help, "rate is worked out again")
	require.Contains(t, help, "[P]ick Up a guest")
	require.Contains(t, help, "Dead guests remain stored")
	require.Contains(t, help, "the guards are called")
	require.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[H] Close Help")

	pressInnShopKey(dialog, keyboard, ebiten.KeyEscape)
	require.False(t, dialog.IsHelpVisible())
	require.False(t, dialog.IsDismissed(), "Escape closes Help before leaving the Inn")
}

func TestInnShopDialogIntentAndStateCopiesAreImmutable(t *testing.T) {
	state := testInnShopState()
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, state)

	state.Rest.MemberProjections[0].Identity.Name = "MUTATED"
	state.Rest.Warnings[0] = game_state.InnWarningNone
	state.GuestRows[0].Warnings[0] = game_state.InnWarningNone
	require.Equal(t, "Avatar", dialog.GetState().Rest.MemberProjections[0].Identity.Name)
	require.Equal(t, game_state.InnWarningPoisonDeath, dialog.GetState().Rest.Warnings[0])
	require.Equal(t, game_state.InnWarningMinimumMonthCharge, dialog.GetPickupRows()[0].Warnings[0])

	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	request := intent.Request
	intent.Rest.MemberProjections[0].Identity.Name = "CHANGED"
	intent.Rest.Warnings[0] = game_state.InnWarningNone
	require.Equal(t, "Avatar", dialog.GetState().Rest.MemberProjections[0].Identity.Name)
	require.Equal(t, game_state.InnWarningPoisonDeath, dialog.GetState().Rest.Warnings[0])
	require.Equal(t, request, dialog.awaitingRequest)
}

func TestInnShopDialogApplyStatePreservesModeIdentityThenNearestRow(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, testInnShopState())
	pressInnShopKey(dialog, keyboard, ebiten.KeyP)
	pressInnShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Equal(t, "Sentri", selectedInnName(t, dialog))
	dialog.receiptText = "Old receipt"

	refreshed := testInnShopState()
	refreshed.GuestRows = []game_state.InnGuestRow{
		refreshed.GuestRows[2], refreshed.GuestRows[0], refreshed.GuestRows[1],
	}
	dialog.ApplyState(refreshed)
	require.Equal(t, InnShopPickup, dialog.GetMode())
	require.Equal(t, 2, dialog.GetSelectedIndex())
	require.Equal(t, "Sentri", selectedInnName(t, dialog))
	require.Empty(t, dialog.GetReceiptText())

	refreshed.GuestRows = append([]game_state.InnGuestRow(nil), refreshed.GuestRows[:2]...)
	refreshed.RoomOccupancy = 2
	dialog.ApplyState(refreshed)
	require.Equal(t, InnShopPickup, dialog.GetMode())
	require.Equal(t, 1, dialog.GetSelectedIndex(), "missing identity falls back to nearest remaining row")
	require.Equal(t, "Geoffreyy", selectedInnName(t, dialog))
}

func TestInnShopDialogResultReceiptPreviewStopAndReceiptClearing(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestInnShopDialog(keyboard, testInnShopState())
	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	dialog.AcceptActionApproval(intent.Request)
	require.True(t, dialog.IsAwaitingResult())

	refreshed := testInnShopState()
	setTestInnGold(&refreshed, 3031)
	dialog.ApplyResult(intent.Request, refreshed, InnActionResultDisplay{
		Action:           InnActionRest,
		MerchantExchange: "Rest well, travelers.",
		ListedCharge:     1234,
		FaulineiTheft:    56,
		FinalGold:        3031,
		PartySize:        4,
		RoomOccupancy:    3,
		Refused:          false,
		CharacterName:    "",
		Members:          nil,
	})

	require.Equal(t, InnShopRest, dialog.GetMode())
	require.Equal(t, InnShopBrowse, dialog.GetPhase())
	require.Equal(t, "3031", dialog.GetShell().GetHeaderView().PartyGold)
	receipt := dialog.GetReceiptText()
	require.Equal(t, "You rest until morning. Paid 1234 gold. 56 more gold was stolen! Thou hast 3031 gold left.", receipt)
	require.NotContains(t, receipt, "Faulinei")

	pressInnShopKey(dialog, keyboard, ebiten.KeyL)
	require.Empty(t, dialog.GetReceiptText(), "mode change clears the old receipt")
	dialog.setMode(InnShopRest)
	pressInnShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok = dialog.ConsumePendingActionIntent()
	require.True(t, ok)
	dialog.AcceptActionApproval(intent.Request)
	dialog.PreviewStop(intent.Request)
	require.Contains(t, dialog.GetReceiptText(), "PREVIEW")
	require.Contains(t, dialog.GetReceiptText(), "unchanged")
}

func TestInnShopDialogParentReleaseDebouncesEscapeAndEnter(t *testing.T) {
	held := map[ebiten.Key]bool{ebiten.KeyEscape: true}
	config := testInnShopDialogConfig(newWidgetTestKeyboard(0), testInnShopState())
	config.IsKeyPressed = func(key ebiten.Key) bool { return held[key] }
	dialog := NewInnShopDialog(config)

	dialog.Update()
	require.False(t, dialog.IsDismissed(), "held Escape cannot dismiss a newly shown parent")
	held[ebiten.KeyEscape] = false
	dialog.Update()
	require.False(t, dialog.IsDismissed())
	pressInnShopKey(dialog, config.Keyboard, ebiten.KeyEscape)
	require.True(t, dialog.IsDismissed())

	held = map[ebiten.Key]bool{ebiten.KeyEnter: true}
	config = testInnShopDialogConfig(newWidgetTestKeyboard(0), testInnShopState())
	config.IsKeyPressed = func(key ebiten.Key) bool { return held[key] }
	dialog = NewInnShopDialog(config)
	dialog.Update()
	require.False(t, dialog.HasPendingActionIntent(), "held Enter cannot open approval")
	held[ebiten.KeyEnter] = false
	dialog.Update()
	pressInnShopKey(dialog, config.Keyboard, ebiten.KeyEnter)
	require.True(t, dialog.HasPendingActionIntent())
}

func testInnShopState() game_state.InnShopState {
	const gold uint16 = 4321
	identities := []game_state.InnCharacterIdentity{
		testInnIdentity("Avatar", party_state.Avatar),
		testInnIdentity("Iolo", party_state.Bard),
		testInnIdentity("Shamino", party_state.Fighter),
		testInnIdentity("Mariahxxx", party_state.Wizard),
	}
	statuses := []party_state.CharacterStatus{
		party_state.Dead, party_state.Dead, party_state.Poisoned, party_state.Poisoned,
	}
	projections := make([]game_state.InnCharacterProjection, len(identities))
	for index := range projections {
		projections[index] = testInnProjection(identities[index], index, statuses[index])
	}
	projections[1].StatusBefore = party_state.Good
	projections[1].StatusAfter = party_state.Good
	projections[1].HPBefore = 40
	projections[1].HPAfter = 100
	projections[1].MaxHP = 100
	projections[1].MPBefore = 4
	projections[1].MPAfter = 20
	for _, index := range []int{2, 3} {
		projections[index].StatusAfter = party_state.Dead
		projections[index].HPBefore = 35
		projections[index].HPAfter = 0
		projections[index].MaxHP = 90
		projections[index].MPBefore = 3
		projections[index].MPAfter = 12
		projections[index].Warnings = []game_state.InnWarning{game_state.InnWarningPoisonDeath}
	}

	state := game_state.InnShopState{
		Merchant: references.MerchantDefinition{
			ID:         references.MerchantInnBritain,
			Family:     references.MerchantFamilyInn,
			Location:   references.Britain,
			NPCIndex:   2,
			CatalogRow: 0,
			OwnerName:  "Donya",
			ShopName:   "The Wayfarer Inn",
		},
		PricingCharacterIndex: 1,
		PricingCharacterName:  "SECRETBUYER",
		Gold:                  gold,
		RoomOccupancy:         3,
		RoomCapacity:          4,
		PartySize:             4,
		PartyCapacity:         party_state.MaxActiveParty,
		MonthlyRate:           1234,
		RosterFingerprint:     "inn-roster-v1",
		Rest: game_state.InnRestQuote{
			Charge:           1234,
			Affordable:       true,
			BlockReason:      game_state.InnRestBlockNone,
			GoldBefore:       gold,
			GoldAfterPayment: gold - 1234,
			SleepLocation: references.PlayerLocation{
				Location: references.Britain, Position: references.Position{X: 21, Y: 10}, Floor: 0,
			},
			WakeLocation: references.PlayerLocation{
				Location: references.Britain, Position: references.Position{X: 22, Y: 10}, Floor: 0,
			},
			DateTimeBefore:    datetime.UltimaDate{Year: 139, Month: 2, Day: 3, Hour: 20, Minute: 15},
			DateTimeAfter:     datetime.UltimaDate{Year: 139, Month: 2, Day: 4, Hour: 6, Minute: 3},
			TimeStepsMinutes:  []int{5, 5, 9},
			MemberProjections: projections,
			Warnings:          []game_state.InnWarning{game_state.InnWarningPoisonDeath},
		},
		LeaveRows: []game_state.InnLeaveRow{
			testInnLeaveRow(projections[0], false, game_state.InnLeaveBlockAvatar, nil, 4, 3),
			testInnLeaveRow(testInnProjection(identities[1], 1, party_state.Dead), false, game_state.InnLeaveBlockDead, nil, 4, 3),
			testInnLeaveRow(projections[2], true, game_state.InnLeaveBlockNone,
				[]game_state.InnWarning{game_state.InnWarningLastLivingMember}, 3, 4),
			testInnLeaveRow(projections[3], true, game_state.InnLeaveBlockNone, nil, 3, 4),
		},
		GuestRows: []game_state.InnGuestRow{
			testInnGuest(4, "Geoffreyy", party_state.Poisoned, 0, true, game_state.InnPickupBlockNone,
				[]game_state.InnWarning{game_state.InnWarningMinimumMonthCharge, game_state.InnWarningPoisonDeath}),
			testInnGuest(5, "Sentri", party_state.Dead, 2, false, game_state.InnPickupBlockDead, nil),
			testInnGuest(6, "Katrina", party_state.Good, 4, true, game_state.InnPickupBlockNone,
				[]game_state.InnWarning{game_state.InnWarningInsufficientFundsActivatesGuards}),
		},
	}

	return state
}

func testInnIdentity(name string, class party_state.CharacterClass) game_state.InnCharacterIdentity {
	return game_state.InnCharacterIdentity{Name: name, Gender: party_state.Male, Class: class}
}

func testInnProjection(
	identity game_state.InnCharacterIdentity,
	index int,
	status party_state.CharacterStatus,
) game_state.InnCharacterProjection {
	return game_state.InnCharacterProjection{
		Identity:          identity,
		RosterIndexBefore: index,
		RosterIndexAfter:  index,
		StatusBefore:      status,
		StatusAfter:       status,
		HPBefore:          0,
		HPAfter:           0,
		MaxHP:             100,
		MPBefore:          0,
		MPAfter:           0,
		PartyMapBefore:    party_state.PartyMapActive,
		PartyMapAfter:     party_state.PartyMapActive,
		MonthsBefore:      0,
		MonthsAfter:       0,
		Warnings:          nil,
	}
}

func testInnLeaveRow(
	projection game_state.InnCharacterProjection,
	eligible bool,
	reason game_state.InnLeaveBlockReason,
	warnings []game_state.InnWarning,
	partyAfter int,
	occupancyAfter int,
) game_state.InnLeaveRow {
	projection.Warnings = append([]game_state.InnWarning(nil), warnings...)
	projection.RosterIndexAfter = 15

	return game_state.InnLeaveRow{
		Identity:       projection.Identity,
		RosterIndex:    projection.RosterIndexBefore,
		Status:         projection.StatusBefore,
		Eligible:       eligible,
		BlockReason:    reason,
		Projection:     projection,
		Warnings:       append([]game_state.InnWarning(nil), warnings...),
		PartySizeAfter: partyAfter,
		OccupancyAfter: occupancyAfter,
	}
}

func testInnGuest(
	index int,
	name string,
	status party_state.CharacterStatus,
	months byte,
	confirmable bool,
	reason game_state.InnPickupBlockReason,
	warnings []game_state.InnWarning,
) game_state.InnGuestRow {
	identity := testInnIdentity(name, party_state.Fighter)
	billed := max(int(months), 1)
	charge := uint16(1234 * billed)
	affordable := charge <= 4321
	goldAfter := uint16(4321)
	if affordable {
		goldAfter -= charge
	}
	projection := testInnProjection(identity, index, status)
	projection.RosterIndexAfter = 4
	projection.Warnings = append([]game_state.InnWarning(nil), warnings...)
	if status == party_state.Poisoned {
		projection.StatusAfter = party_state.Dead
		projection.HPAfter = 0
	}
	partyAfter, occupancyAfter := 5, 2
	if !confirmable {
		partyAfter, occupancyAfter = 4, 3
	}

	return game_state.InnGuestRow{
		Identity:           identity,
		RosterIndex:        index,
		Status:             status,
		MonthsAtInn:        months,
		BilledMonths:       billed,
		MonthlyRate:        1234,
		Charge:             charge,
		Affordable:         affordable,
		Confirmable:        confirmable,
		BlockReason:        reason,
		Projection:         projection,
		Warnings:           append([]game_state.InnWarning(nil), warnings...),
		GoldBefore:         4321,
		GoldAfterPayment:   goldAfter,
		PartySizeAfter:     partyAfter,
		RoomOccupancyAfter: occupancyAfter,
	}
}

func setTestInnGold(state *game_state.InnShopState, gold uint16) {
	state.Gold = gold
	state.Rest.GoldBefore = gold
	state.Rest.Affordable = state.Rest.Charge <= gold
	state.Rest.GoldAfterPayment = gold
	if state.Rest.Affordable {
		state.Rest.GoldAfterPayment -= state.Rest.Charge
	}
	for index := range state.GuestRows {
		row := &state.GuestRows[index]
		row.GoldBefore = gold
		row.Affordable = row.Charge <= gold
		row.GoldAfterPayment = gold
		if row.Affordable {
			row.GoldAfterPayment -= row.Charge
		}
	}
}

func testInnShopDialogConfig(
	keyboard *input.Keyboard,
	state game_state.InnShopState,
) InnShopDialogConfig {
	return InnShopDialogConfig{
		Keyboard:       keyboard,
		MerchantSprite: ebiten.NewImage(16, 16),
		State:          state,
		Prose: InnShopDialogProse{
			Greeting:     "Welcome to the Inn.",
			RestPrompt:   "Wouldst thou rest?",
			LeavePrompt:  "Who shall remain here?",
			PickupPrompt: "Who shall rejoin thee?",
		},
		Preview:      false,
		IsKeyPressed: func(ebiten.Key) bool { return false },
	}
}

func newTestInnShopDialog(keyboard *input.Keyboard, state game_state.InnShopState) *InnShopDialog {
	return NewInnShopDialog(testInnShopDialogConfig(keyboard, state))
}

func pressInnShopKey(dialog *InnShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func selectedInnName(t *testing.T, dialog *InnShopDialog) string {
	t.Helper()
	identity, ok := dialog.GetSelectedIdentity()
	require.True(t, ok)

	return identity.Name
}

func innLeaveNames(rows []game_state.InnLeaveRow) []string {
	names := make([]string, len(rows))
	for index, row := range rows {
		names[index] = row.Identity.Name
	}

	return names
}

func compactInnText(value string) string { return strings.Join(strings.Fields(value), " ") }
