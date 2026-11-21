package widgets

import (
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestHealerShopDialogUsesExistingShellAndBoundedTwoPaneWorkspace(t *testing.T) {
	for _, size := range [][2]int{{1280, 720}, {320, 180}} {
		setMerchantShellTestResolution(t, size[0], size[1])
		dialog := newTestHealerShopDialog(newWidgetTestKeyboard(0), testHealerShopState())
		shellWorkspace := dialog.GetShell().GetWorkspaceBounds()

		assert.True(t, dialog.GetServicePaneBounds().In(shellWorkspace))
		assert.True(t, dialog.GetRecipientPaneBounds().In(shellWorkspace))
		assert.True(t, dialog.GetDetailBounds().In(shellWorkspace))
		assert.Less(t, dialog.GetServicePaneBounds().Max.X, dialog.GetRecipientPaneBounds().Min.X)
		assert.LessOrEqual(t, dialog.GetRecipientPaneBounds().Max.Y, dialog.GetDetailBounds().Min.Y)
		assert.Greater(t, dialog.GetRecipientPaneBounds().Dx(), dialog.GetServicePaneBounds().Dx()*2)
	}
}

func TestHealerShopDialogLowResolutionReservesReadableStatus(t *testing.T) {
	setMerchantShellTestResolution(t, 320, 180)
	dialog := newTestHealerShopDialog(newWidgetTestKeyboard(0), testHealerShopStateWithRecipientCount(6))
	workspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(healerShopMinimumInset, min(workspace.Dx(), workspace.Dy())/healerShopInsetDivisor)
	naturalRowHeight := int(math.Ceil(dialog.cellOutput.GetFontHeight())) +
		max(inset, dialog.pixelUnit()*healerShopDoubleInset)

	requireHealerStatusConsumesRemainingSpace(t, dialog)
	require.Less(t, dialog.serviceRows[0].Dy(), naturalRowHeight)
	require.Positive(t, dialog.serviceRows[0].Dy())
}

func TestHealerShopDialogResizePreservesSelectionPhaseAndIntent(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHealerShopDialog(keyboard, testHealerShopState())
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, HealerShopAwaitingApproval, dialog.GetPhase())
	fullDetail := dialog.GetDetailBounds()

	setMerchantShellTestResolution(t, 320, 180)
	dialog.GetDetailBounds()

	require.Equal(t, HealerShopRecipients, dialog.GetFocus())
	require.Equal(t, references.HealerCure, dialog.GetSelectedService())
	require.Equal(t, 0, dialog.GetSelectedRecipient().CharacterIndex)
	require.Equal(t, HealerShopAwaitingApproval, dialog.GetPhase())
	require.Contains(t, dialog.GetDetailText(), "Confirm or cancel")
	require.Less(t, dialog.GetDetailBounds().Dx(), fullDetail.Dx())
	requireHealerStatusConsumesRemainingSpace(t, dialog)
	intent, ok := dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	require.Equal(t, game_state.HealerServiceRequest{
		Service: references.HealerCure, CharacterIndex: 0,
	}, intent.Request)
}

func TestHealerShopDialogRecipientCountRefreshRecalculatesGeometry(t *testing.T) {
	setMerchantShellTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	state := testHealerShopStateWithRecipientCount(6)
	dialog := newTestHealerShopDialog(keyboard, state)
	largePane := dialog.GetRecipientPaneBounds()
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	dialog.AcceptTreatmentApproval(intent.Request)
	refreshed := state
	refreshed.Recipients = append([]game_state.HealerRecipientState(nil), state.Recipients[:1]...)
	dialog.ApplyRefusal(refreshed, intent.Request, HealerServiceResultDisplay{
		MerchantExchange: "The party has changed.",
		Receipt:          "No treatment was completed.",
	})

	require.Equal(t, HealerShopBrowse, dialog.GetPhase())
	require.Equal(t, 0, dialog.GetSelectedRecipient().CharacterIndex)
	require.Less(t, dialog.GetRecipientPaneBounds().Dy(), largePane.Dy())
	require.Contains(t, dialog.GetDetailText(), "No treatment was completed")
	requireHealerStatusConsumesRemainingSpace(t, dialog)
}

func TestHealerShopDialogKeyboardMovesBetweenServiceAndRecipientPanes(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHealerShopDialog(keyboard, testHealerShopState())
	require.Equal(t, HealerShopServices, dialog.GetFocus())
	require.Equal(t, references.HealerCure, dialog.GetSelectedService())
	require.Equal(t, 0, dialog.GetSelectedRecipient().CharacterIndex)
	assert.False(t, dialog.recipientRowFocused(0), "service focus must not highlight a hidden recipient selection")

	pressHealerShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Equal(t, references.HealerHeal, dialog.GetSelectedService())
	pressHealerShopKey(dialog, keyboard, ebiten.KeyR)
	assert.Equal(t, references.HealerResurrect, dialog.GetSelectedService())
	pressHealerShopKey(dialog, keyboard, ebiten.KeyRight)
	assert.Equal(t, HealerShopRecipients, dialog.GetFocus())
	assert.True(t, dialog.recipientRowFocused(0))
	pressHealerShopKey(dialog, keyboard, ebiten.KeyDown)
	assert.Equal(t, 1, dialog.GetSelectedRecipient().CharacterIndex)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyLeft)
	assert.Equal(t, HealerShopServices, dialog.GetFocus())

	pressHealerShopKey(dialog, keyboard, ebiten.KeyC)
	assert.Equal(t, references.HealerCure, dialog.GetSelectedService())
	assert.Equal(t, 1, dialog.GetSelectedRecipient().CharacterIndex, "pane selection remains independent")
}

func TestHealerShopDialogIneligibleSelectionSpeaksWithoutRequest(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHealerShopDialog(keyboard, testHealerShopState())
	pressHealerShopKey(dialog, keyboard, ebiten.KeyRight)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyDown)
	require.Equal(t, 1, dialog.GetSelectedRecipient().CharacterIndex)

	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)

	assert.Equal(t, HealerShopBrowse, dialog.GetPhase())
	_, pending := dialog.ConsumePendingTreatmentIntent()
	assert.False(t, pending)
	assert.Equal(t, "\"Thou hast no need of this art!\"", dialog.GetLatestExchange())
	detail := strings.Join(strings.Fields(dialog.GetDetailText()), " ")
	assert.Contains(t, detail, "would not help")
	assert.Contains(t, detail, "Poisoned")
}

func TestHealerShopDialogEnterOnServiceMovesToPartyWithoutReviewingHiddenSelection(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHealerShopDialog(keyboard, testHealerShopState())
	require.Contains(t, dialog.GetDetailText(), "Choose a party member")

	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)

	assert.Equal(t, HealerShopRecipients, dialog.GetFocus())
	assert.Equal(t, HealerShopBrowse, dialog.GetPhase())
	assert.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Enter] Review")
	_, pending := dialog.ConsumePendingTreatmentIntent()
	assert.False(t, pending)
	assert.True(t, dialog.recipientRowFocused(0))
	assert.Contains(t, dialog.GetDetailText(), "Shamino")
	pressHealerShopKey(dialog, keyboard, ebiten.KeyLeft)
	assert.Contains(t, dialog.GetShell().GetFooterView().Guidance, "[Enter] Choose party member")
}

func TestHealerShopDialogApprovalIntentCancelAndAcceptLifecycle(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHealerShopDialog(keyboard, testHealerShopState())

	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	assert.Equal(t, HealerShopRecipients, dialog.GetFocus())
	assert.Equal(t, HealerShopBrowse, dialog.GetPhase())
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	assert.Equal(t, HealerShopAwaitingApproval, dialog.GetPhase())
	intent, ok := dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	assert.Equal(t, game_state.HealerServiceRequest{
		Service:        references.HealerCure,
		CharacterIndex: 0,
	}, intent.Request)
	dialog.CancelTreatmentApproval()
	assert.Equal(t, HealerShopBrowse, dialog.GetPhase())

	pressHealerShopKey(dialog, keyboard, ebiten.KeyH)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyDown)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	require.Equal(t, HealerShopAwaitingApproval, dialog.GetPhase())
	intent, ok = dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	assert.Equal(t, game_state.HealerServiceRequest{
		Service:        references.HealerHeal,
		CharacterIndex: 1,
	}, intent.Request)
	_, ok = dialog.ConsumePendingTreatmentIntent()
	assert.False(t, ok)
	dialog.AcceptTreatmentApproval(intent.Request)
	require.Equal(t, HealerShopAwaitingResult, dialog.GetPhase())
	require.True(t, dialog.IsAwaitingResult())
}

func TestHealerShopDialogAppliesResultAndPreservesLogicalSelection(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	state := testHealerShopState()
	dialog := newTestHealerShopDialog(keyboard, state)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyH)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyRight)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyDown)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	dialog.AcceptTreatmentApproval(intent.Request)

	refreshed := healedHealerShopState(state, intent.Request.CharacterIndex)
	dialog.ApplyServiceResult(game_state.HealerServiceResult{
		Service:          references.HealerHeal,
		CharacterIndex:   1,
		CharacterName:    "Iolo",
		ChargeKind:       game_state.HealerChargeFree,
		SourcePrice:      35,
		Charge:           0,
		GoldBefore:       312,
		GoldAfterPayment: 312,
		Theft:            0,
		GoldAfterTheft:   312,
		StatusBefore:     party_state.Sleep,
		StatusAfter:      party_state.Sleep,
		HPBefore:         18,
		HPAfter:          60,
		MaxHPBefore:      60,
		MaxHPAfter:       60,
		MPBefore:         0,
		MPAfter:          0,
		ExpBefore:        100,
		ExpAfter:         100,
		LevelBefore:      2,
		LevelAfter:       2,
	}, refreshed, HealerServiceResultDisplay{
		MerchantExchange: "\"Thy wounds are mended.\"",
		Receipt:          "Iolo is fully healed; party gold remains 312.",
	})

	assert.Equal(t, HealerShopBrowse, dialog.GetPhase())
	assert.Equal(t, references.HealerHeal, dialog.GetSelectedService())
	assert.Equal(t, 1, dialog.GetSelectedRecipient().CharacterIndex)
	assert.Equal(t, uint16(60), dialog.GetSelectedRecipient().HP)
	assert.Equal(t, "312", dialog.GetShell().GetHeaderView().PartyGold)
	assert.Equal(t, "\"Thy wounds are mended.\"", dialog.GetLatestExchange())
	assert.Contains(t, dialog.GetReceiptText(), "fully healed")
}

func TestHealerShopDialogPreviewStopsAtAuthorityWithoutChangingDetachedState(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	config := testHealerShopDialogConfig(keyboard, testHealerShopState())
	config.Preview = true
	dialog := NewHealerShopDialog(config)
	require.True(t, dialog.IsPreview())
	require.Contains(t, dialog.GetDetailText(), "Preview")

	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	pressHealerShopKey(dialog, keyboard, ebiten.KeyEnter)
	intent, ok := dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	dialog.AcceptTreatmentApproval(intent.Request)
	dialog.ApplyPreviewStopped(intent.Request)

	assert.Equal(t, HealerShopBrowse, dialog.GetPhase())
	assert.Equal(t, uint16(312), dialog.state.Gold)
	assert.Equal(t, party_state.Poisoned, dialog.GetSelectedRecipient().Status)
	assert.Contains(t, dialog.GetReceiptText(), "no treatment")
}

func TestHealerShopDialogUsesKeyboardOnlyGuidance(t *testing.T) {
	dialog := newTestHealerShopDialog(newWidgetTestKeyboard(0), testHealerShopState())
	guidance := strings.ToLower(dialog.GetShell().GetFooterView().Guidance)

	assert.Contains(t, guidance, "left/right")
	assert.Contains(t, guidance, "up/down")
	assert.Contains(t, guidance, "[a] all")
	assert.NotContains(t, guidance, "c/h/r")
	assert.NotContains(t, guidance, "mouse")
	assert.NotContains(t, guidance, "click")
	requireKeyedGuidance(t, dialog.GetShell().GetFooterView().Guidance)
}

func TestHealerShopDialogAllShortcutBuildsAggregateRequest(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	dialog := newTestHealerShopDialog(keyboard, testHealerShopState())

	pressHealerShopKey(dialog, keyboard, ebiten.KeyA)
	require.Equal(t, HealerShopServices, dialog.GetFocus(), "All is a shortcut, not a Party row")
	require.Equal(t, HealerShopAwaitingApproval, dialog.GetPhase())
	intent, ok := dialog.ConsumePendingTreatmentIntent()
	require.True(t, ok)
	require.True(t, intent.IsAll())
	require.Nil(t, intent.Recipient)
	require.Equal(t, 1, intent.AllService.RecipientCount)
	assert.Equal(t, game_state.HealerServiceRequest{
		Service:        references.HealerCure,
		CharacterIndex: game_state.HealerAllRecipientsIndex,
	}, intent.Request)
}

func TestHealerShopDialogUsesCompactMixedCaseLabelsAndSemanticEligibilityColors(t *testing.T) {
	state := testHealerShopState()

	assert.Equal(t, "Cure", state.Services[references.HealerCure].Service.String())
	assert.Equal(t, "Free", healerServicePriceLabel(state.Services[references.HealerCure]))
	assert.Equal(t, "Ress.", healerServiceDisplayName(references.HealerResurrect))
	assert.Equal(t, "P", healerStatusLabel(party_state.Poisoned))
	assert.Equal(t, "D", healerStatusLabel(party_state.Dead))
	assert.Equal(t, DialogDeltaPositive, healerRecipientRowColor(game_state.HealerEligible, true, false))
	assert.Equal(t, DialogLabelColor, healerRecipientRowColor(game_state.HealerEligible, false, false))
	assert.Equal(t, DialogEmptyColor, healerRecipientRowColor(game_state.HealerNotInjured, true, false))
	assert.Equal(t, DialogSelectedText, healerRecipientRowColor(game_state.HealerEligible, true, true))
}

func testHealerShopState() game_state.HealerShopState {
	return game_state.HealerShopState{
		Merchant: references.MerchantDefinition{
			ID:         references.MerchantHealerMinoc,
			Family:     references.MerchantFamilyHealer,
			Location:   references.Minoc,
			NPCIndex:   2,
			CatalogRow: 0,
			OwnerName:  "Regina",
			ShopName:   "The Healers Mission",
		},
		PricingCharacterIndex: 0,
		PricingCharacterName:  "Shamino",
		Gold:                  312,
		Services: [3]game_state.HealerServiceQuote{
			{Service: references.HealerCure, SourcePrice: 20, Charge: 0, ChargeKind: game_state.HealerChargeFree, Affordable: true},
			{Service: references.HealerHeal, SourcePrice: 35, Charge: 0, ChargeKind: game_state.HealerChargeFree, Affordable: true},
			{Service: references.HealerResurrect, SourcePrice: 200, Charge: 200, ChargeKind: game_state.HealerChargePaid, Affordable: true},
		},
		AllServices: [3]game_state.HealerAllServiceQuote{
			{Service: references.HealerCure, SourcePrice: 20, Charge: 0, ChargeKind: game_state.HealerChargeFree, Affordable: true, RecipientCount: 1},
			{Service: references.HealerHeal, SourcePrice: 35, Charge: 0, ChargeKind: game_state.HealerChargeFree, Affordable: true, RecipientCount: 2},
			{Service: references.HealerResurrect, SourcePrice: 200, Charge: 200, ChargeKind: game_state.HealerChargePaid, Affordable: true, RecipientCount: 1},
		},
		Recipients: []game_state.HealerRecipientState{
			testHealerRecipient(0, "Shamino", party_state.Poisoned, 42, 60),
			testHealerRecipient(1, "Iolo", party_state.Sleep, 18, 60),
			testHealerRecipient(2, "Dupre", party_state.Dead, 0, 90),
		},
	}
}

func testHealerShopStateWithRecipientCount(count int) game_state.HealerShopState {
	state := testHealerShopState()
	state.Recipients = append([]game_state.HealerRecipientState(nil), state.Recipients[:min(count, len(state.Recipients))]...)
	for index := len(state.Recipients); index < count; index++ {
		state.Recipients = append(state.Recipients, testHealerRecipient(
			index,
			"Companion "+string(rune('A'+index)),
			party_state.Good,
			60,
			60,
		))
	}

	return state
}

func testHealerRecipient(
	index int,
	name string,
	status party_state.CharacterStatus,
	hp uint16,
	maxHP uint16,
) game_state.HealerRecipientState {
	recipient := game_state.HealerRecipientState{
		CharacterIndex: index,
		Name:           name,
		Status:         status,
		HP:             hp,
		MaxHP:          maxHP,
		MP:             0,
		Exp:            100,
		Level:          2,
		Treatments:     [3]game_state.HealerTreatmentProjection{},
	}
	recipient.Treatments[references.HealerCure] = game_state.HealerTreatmentProjection{
		Eligibility: game_state.HealerNotPoisoned,
		StatusAfter: status,
		HPAfter:     hp,
		MaxHPAfter:  maxHP,
		MPAfter:     0,
		ExpAfter:    100,
		LevelAfter:  2,
	}
	recipient.Treatments[references.HealerHeal] = game_state.HealerTreatmentProjection{
		Eligibility: game_state.HealerEligible,
		StatusAfter: status,
		HPAfter:     maxHP,
		MaxHPAfter:  maxHP,
		MPAfter:     0,
		ExpAfter:    100,
		LevelAfter:  2,
	}
	recipient.Treatments[references.HealerResurrect] = game_state.HealerTreatmentProjection{
		Eligibility: game_state.HealerLivingCannotResurrect,
		StatusAfter: status,
		HPAfter:     hp,
		MaxHPAfter:  maxHP,
		MPAfter:     0,
		ExpAfter:    100,
		LevelAfter:  2,
	}
	if status == party_state.Poisoned {
		recipient.Treatments[references.HealerCure].Eligibility = game_state.HealerEligible
		recipient.Treatments[references.HealerCure].StatusAfter = party_state.Good
	}
	if hp == maxHP {
		recipient.Treatments[references.HealerHeal].Eligibility = game_state.HealerNotInjured
	}
	if status == party_state.Dead {
		recipient.Treatments[references.HealerHeal].Eligibility = game_state.HealerDeadCannotHeal
		recipient.Treatments[references.HealerResurrect] = game_state.HealerTreatmentProjection{
			Eligibility: game_state.HealerEligible,
			StatusAfter: party_state.Good,
			HPAfter:     60,
			MaxHPAfter:  60,
			MPAfter:     0,
			ExpAfter:    100,
			LevelAfter:  2,
		}
	}

	return recipient
}

func healedHealerShopState(state game_state.HealerShopState, characterIndex int) game_state.HealerShopState {
	state.Recipients = append([]game_state.HealerRecipientState(nil), state.Recipients...)
	for index := range state.Recipients {
		if state.Recipients[index].CharacterIndex != characterIndex {
			continue
		}
		state.Recipients[index].HP = state.Recipients[index].MaxHP
		state.Recipients[index].Treatments[references.HealerHeal].Eligibility = game_state.HealerNotInjured
		state.Recipients[index].Treatments[references.HealerHeal].HPAfter = state.Recipients[index].MaxHP
		state.AllServices[references.HealerHeal].RecipientCount--
	}

	return state
}

func newTestHealerShopDialog(
	keyboard *input.Keyboard,
	state game_state.HealerShopState,
) *HealerShopDialog {
	return NewHealerShopDialog(testHealerShopDialogConfig(keyboard, state))
}

func testHealerShopDialogConfig(
	keyboard *input.Keyboard,
	state game_state.HealerShopState,
) HealerShopDialogConfig {
	return HealerShopDialogConfig{
		Keyboard:       keyboard,
		MerchantSprite: ebiten.NewImage(16, 16),
		State:          state,
		Prose: HealerShopDialogProse{
			Greeting:     "\"How may I aid thee?\"",
			NoNeed:       "\"Thou hast no need of this art!\"",
			FundsRefusal: "\"I cannot accept charity cases.\"",
		},
		Preview: false,
	}
}

func pressHealerShopKey(dialog *HealerShopDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func requireHealerStatusConsumesRemainingSpace(t *testing.T, dialog *HealerShopDialog) {
	t.Helper()
	workspace := dialog.GetShell().GetWorkspaceBounds()
	inset := max(healerShopMinimumInset, min(workspace.Dx(), workspace.Dy())/healerShopInsetDivisor)
	content := insetRectangle(workspace, inset)
	gap := max(inset, dialog.pixelUnit()*healerShopDoubleInset)
	service := dialog.GetServicePaneBounds()
	recipients := dialog.GetRecipientPaneBounds()
	detail := dialog.GetDetailBounds()

	require.True(t, service.In(content))
	require.True(t, recipients.In(content))
	require.True(t, detail.In(content))
	require.Equal(t, service.Min.Y, recipients.Min.Y)
	require.Equal(t, service.Max.Y, recipients.Max.Y)
	require.Equal(t, service.Max.Y+gap, detail.Min.Y)
	require.Equal(t, content.Max.Y, detail.Max.Y)
	require.Greater(t, service.Dx(), 0)
	require.Greater(t, service.Dy(), 0)
	require.Greater(t, recipients.Dx(), 0)
	require.Greater(t, recipients.Dy(), 0)
	require.Greater(t, detail.Dx(), 0)
	require.Greater(t, detail.Dy(), 0)
	require.GreaterOrEqual(t, dialog.detailTextBounds.Dy(),
		int(math.Ceil(dialog.detailOutput.GetTotalHeightForLines(healerShopDetailLines))))
}
