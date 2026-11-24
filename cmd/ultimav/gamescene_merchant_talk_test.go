package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestMerchantTalkRouteAcceptsOnlyExactProductionCodes(t *testing.T) {
	for code := references.DialogWeaponDealer; code <= references.DialogInnkeeper; code++ {
		require.True(t, isMerchantTalkDialog(code), "dialog code 0x%02x", byte(code))
	}
	for _, code := range []references.DialogNumber{0x80, 0x89, 0xfe, 0xff} {
		require.False(t, isMerchantTalkDialog(code), "dialog code 0x%02x", byte(code))
	}
}

func TestNPCAutoTalkOrdinaryGuardPreservesLinearConversationRouting(t *testing.T) {
	state := new(game_state.GameState)
	scene := &GameScene{gameState: state}
	ordinaryReference := references.NPCReference{DialogNumber: 1}
	ordinaryReference.SetKeyIndex(indexes.GuardKeyIndex)
	ordinaryGuard := map_units.NewNPCFriendly(ordinaryReference, 0)

	require.Equal(t, TalkTypeNormal, scene.determineTalkType(ordinaryGuard, true))
	require.Equal(t, TalkTypeNormal, scene.determineTalkType(ordinaryGuard, false),
		"ordinary Guard conversations are not procedural tribute")

	state.SetCriminalStatus(true)
	require.Equal(t, TalkTypeGuard, scene.determineTalkType(ordinaryGuard, true),
		"criminal auto-talk guards must still arrest")

	state.SetCriminalStatus(false)
	extortingReference := references.NPCReference{DialogNumber: byte(references.DialogGuard)}
	extortingReference.SetKeyIndex(indexes.GuardKeyIndex)
	extortingGuard := map_units.NewNPCFriendly(extortingReference, 0)
	require.Equal(t, TalkTypeGuard, scene.determineTalkType(extortingGuard, true),
		"DialogGuard auto-talk must still extort")

	extortingCivilianReference := references.NPCReference{DialogNumber: byte(references.DialogGuard)}
	extortingCivilianReference.SetKeyIndex(indexes.BardKeyIndex)
	extortingCivilian := map_units.NewNPCFriendly(extortingCivilianReference, 1)
	require.Equal(t, TalkTypeGuard, scene.determineTalkType(extortingCivilian, false),
		"selector 0xff, not actor shape, owns procedural tribute")
}

func TestManualGuildTalkPurchaseRepeatCloseOwnsOneMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	keysBefore := fixture.state.PartyState.Inventory.Provisions.Keys.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	pricePerBundle := dialog.GetSelectedQuote().PricePerBundle
	require.Equal(t, 1, fixture.scene.dialogStack.Count())

	completeGuildPurchase(t, fixture, dialog, 1)
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, fmt.Sprintf(
		"Bought: 1 bundle of keys for %d gold. Thou hast now %d.",
		pricePerBundle,
		keysBefore+3,
	), dialog.GetReceiptText())
	require.Equal(t, strconv.Itoa(int(goldBefore)-pricePerBundle), dialog.GetShell().GetHeaderView().PartyGold)
	require.Equal(t, keysBefore+3, dialog.GetSelectedQuote().QuantityBefore)

	secondPurchase := completeGuildPurchase(t, fixture, dialog, 1)
	require.Equal(t, keysBefore+3, secondPurchase.GetQuote().QuantityBefore)
	require.Equal(t, goldBefore-uint16(pricePerBundle), secondPurchase.GetQuote().Gold)
	require.Equal(t, keysBefore+6, fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	require.Equal(t, goldBefore-uint16(pricePerBundle*2), fixture.state.PartyState.Inventory.Gold.Get())
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, fmt.Sprintf(
		"Bought: 1 bundle of keys for %d gold. Thou hast now %d.",
		pricePerBundle,
		keysBefore+6,
	), dialog.GetReceiptText(), "the latest receipt replaces the prior exchange detail")
	require.Equal(t, "\"Sold!\" says Braunam. \"What else, m'lord?\"", dialog.GetLatestExchange())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, 0, fixture.scene.dialogStack.Count())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
}

func TestManualGuildTalkCancellationOwnsOneMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	keysBefore := fixture.state.PartyState.Inventory.Provisions.Keys.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, keysBefore, fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestGuildQuantityModalUsesSelectedPitchAndCancelResumesBrowse(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	gemsBefore := fixture.state.PartyState.Inventory.Provisions.Gems.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	greeting := dialog.GetLatestExchange()
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	quote := dialog.GetSelectedQuote()
	require.Equal(t, references.Gem, quote.Provision)
	require.Equal(t, greeting, dialog.GetLatestExchange(), "row navigation must not add the original pitch")

	purchaseDialog := openMerchantModal[*widgets.GuildPurchaseDialog](t, fixture, dialog, widgets.GuildShopAwaitingQuantity)
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, purchaseDialog, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, quote, purchaseDialog.GetQuote())
	require.Contains(t, purchaseDialog.GetPitch(), strconv.Itoa(quote.PricePerBundle))
	require.NotContains(t, dialog.GetLatestExchange(), purchaseDialog.GetPitch())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	require.True(t, purchaseDialog.IsCancelled())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.GuildShopBrowse
	})
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, greeting, dialog.GetLatestExchange())
	require.Empty(t, dialog.GetReceiptText())
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, gemsBefore, fixture.state.PartyState.Inventory.Provisions.Gems.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestGuildQuantityModalCommitsExactBatchRequestAndResult(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	quote := dialog.GetSelectedQuote()
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	keysBefore := fixture.state.PartyState.Inventory.Provisions.Keys.Get()

	purchaseDialog := completeGuildPurchase(t, fixture, dialog, 3)

	require.Equal(t, uint16(3), purchaseDialog.GetBundleCount())
	require.False(t, purchaseDialog.HasPendingPurchaseRequest(), "the workflow must consume the request once")
	require.Equal(t, keysBefore+quote.BundleQuantity*3, fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	require.Equal(t, goldBefore-uint16(quote.PricePerBundle*3), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, "\"Sold!\" says Braunam. \"What else, m'lord?\"", dialog.GetLatestExchange())
	require.Equal(t, fmt.Sprintf(
		"Bought: 3 bundles of keys for %d gold. Thou hast now %d.",
		quote.PricePerBundle*3,
		keysBefore+quote.BundleQuantity*3,
	), dialog.GetReceiptText())
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestManualMerchantTalkAppliesTalk3OneShotBeforeDispatch(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	arrived := fixture.npc.ArrivedScheduleSlot()
	fixture.npc.NPCReference.Schedule.Ai[arrived] = byte(references.Talk3)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	require.Equal(t, references.AiType(references.Wander3), references.AiType(fixture.npc.NPCReference.Schedule.Ai[arrived]))
	require.False(t, fixture.npc.ApplyTalk3OneShot(), "shared Talk dispatch must apply the transition exactly once")

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestNPCAutoTalkGuildUsesSharedWorkflowWithoutSecondMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.npc.NPCReference.Schedule.Ai[1] = byte(references.Talk3)
	fixture.npc.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionTalk

	require.True(t, fixture.scene.processNPCAutoTalk())
	require.Equal(t, map_units.PendingNPCInteractionNone, fixture.npc.MapUnitDetails().PendingInteraction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	require.Equal(t, references.AiType(references.Wander3), references.AiType(fixture.npc.NPCReference.Schedule.Ai[1]))
	require.False(t, fixture.npc.ApplyTalk3OneShot(), "TALK3 transition must be one-shot")

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.True(t, dialog.IsDismissed())
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.False(t, fixture.scene.processNPCAutoTalk())
}

func TestActualTalk3MerchantsUseSharedShellOnceWithoutSecondMinute(t *testing.T) {
	seed := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	type talk3Merchant struct {
		definition references.MerchantDefinition
		slot       references.NPCScheduleSlot
	}
	records := make([]talk3Merchant, 0, 14)
	for _, definition := range seed.state.GameReferences.MerchantReferences.Definitions() {
		npcs := seed.state.GameReferences.NPCReferences.GetNPCReferencesByLocation(definition.Location)
		npc := (*npcs)[definition.NPCIndex]
		for slot, aiType := range npc.Schedule.Ai {
			if references.AiType(aiType) == references.Talk3 {
				records = append(records, talk3Merchant{definition: definition, slot: references.NPCScheduleSlot(slot)})
				break
			}
		}
	}
	require.Len(t, records, 14, "canonical NPC schedules must expose the fourteen actual TALK3 merchant records")

	for _, record := range records {
		t.Run(string(record.definition.ID), func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, record.definition.Location, record.definition.ID)
			fixture.npc.PlaceAtScheduleSlot(record.slot)
			fixture.npc.SetVisible(true)
			setMerchantTalkDesiredSlot(t, fixture.state, fixture.npc, record.slot)
			fixture.direction = placePlayerNextToMerchant(fixture.state, fixture.npc)
			fixture.npc.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionTalk

			require.True(t, fixture.scene.processNPCAutoTalk())
			shell, dismiss := waitForConcreteMerchantDialog(t, fixture, record.definition.Family)
			require.Equal(t, record.definition.ShopName, shell.GetHeaderView().ShopIdentity)
			require.Equal(t, record.definition.OwnerName, shell.GetHeaderView().Owner)
			require.Equal(t, references.AiType(references.Wander3), references.AiType(fixture.npc.NPCReference.Schedule.Ai[record.slot]))
			require.False(t, fixture.npc.ApplyTalk3OneShot(), "TALK3 transition must be one-shot")

			dismiss()
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.Empty(t, fixture.callbacks.TimeAdvanced())
			require.False(t, fixture.scene.processNPCAutoTalk())
		})
	}
}

func TestMerchantTalkClosedAndMountedShowBoundedFeedback(t *testing.T) {
	tests := []struct {
		name    string
		prepare func(*merchantTalkFixture)
		message string
	}{
		{
			name: "closed",
			prepare: func(fixture *merchantTalkFixture) {
				setMerchantTalkDesiredSlot(t, fixture.state, fixture.npc, 0)
			},
			message: "This shop is closed.",
		},
		{
			name: "mounted",
			prepare: func(fixture *merchantTalkFixture) {
				fixture.state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
					references.HorseVehicle,
					references.Position{},
					0,
				)
			},
			message: "Dismount before trading here.",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
			test.prepare(fixture)
			fixture.callbacks.BindGameState(fixture.state)
			fixture.direction = placePlayerNextToMerchant(fixture.state, fixture.npc)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			overlay := waitForMerchantOverlay(t, fixture)

			require.Equal(t, test.message, overlay.GetMessage())
			require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))
			overlay.Dismiss()
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
		})
	}
}

func TestTavernTalkSetupCancellationMealRefreshAndOneMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
	rngBefore := fixture.state.GetRNG().Calls()
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	foodBefore := fixture.state.PartyState.Inventory.Provisions.Food.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
	require.False(t, dialog.IsPreview())
	require.Equal(t, references.MerchantFamilyTavern, dialog.GetState().Merchant.Family)
	require.Equal(t, "The Wayfarer Tavern", dialog.GetShell().GetHeaderView().ShopIdentity)
	require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))
	require.Equal(t, rngBefore, fixture.state.GetRNG().Calls(), "cosmetic Tavern setup must not consume gameplay RNG")

	action := openMerchantModal[*widgets.TavernActionDialog](t, fixture, dialog, widgets.TavernShopAwaitingApproval)
	require.Equal(t, widgets.TavernActionMeal, action.GetIntent().Action)
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.TavernShopBrowse
	})
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, foodBefore, fixture.state.PartyState.Inventory.Provisions.Food.Get())

	quote := dialog.GetState().Meal
	action = openMerchantModal[*widgets.TavernActionDialog](t, fixture, dialog, widgets.TavernShopAwaitingApproval)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	resultDialog := waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
	require.Equal(t, "Meal Served", resultDialog.GetView().Title)
	require.Equal(t, quote.Meal.Name, resultDialog.GetView().Item)
	require.Contains(t, resultDialog.GetBodyText(), "Food")
	refreshed := dialog.GetState()
	require.Equal(t, fixture.state.PartyState.Inventory.Gold.Get(), refreshed.Gold)
	require.Equal(t, fixture.state.PartyState.Inventory.Provisions.Food.Get(), refreshed.Food)
	require.Equal(t, refreshed.Gold, refreshed.Meal.GoldBefore)
	require.Equal(t, refreshed.Food, refreshed.Meal.FoodBefore)
	require.Equal(t, foodBefore+uint16(quote.DinerCount), refreshed.Food)
	require.Empty(t, dialog.GetReceiptText(), "meal results belong to the modal")
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return fixture.scene.dialogStack.PeekTopModalDialog() == dialog
	})

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestTavernWarningStopAndContinueUseTypedAuthority(t *testing.T) {
	tests := []struct {
		name        string
		decision    ebiten.Key
		wantMode    widgets.TavernShopMode
		wantDrunk   int
		wantSmashed int
		karmaDelta  int
	}{
		{name: "Had enough", decision: ebiten.KeyEscape, wantMode: widgets.TavernShopOrder, wantDrunk: 3},
		{name: "Another drink", decision: ebiten.KeyEnter, wantMode: widgets.TavernShopOrder, wantDrunk: 4, wantSmashed: 25, karmaDelta: -1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
			selectTavernOrderItem(t, fixture, dialog, widgets.TavernOrderDrink)
			for range 3 {
				completeTavernAction(t, fixture, dialog, ebiten.KeyEnter)
			}
			require.Empty(t, dialog.GetReceiptText())
			require.Contains(t, dialog.GetWorkspaceText(), "ORDER")
			require.Contains(t, dialog.GetLatestExchange(), "Enjoy!")
			require.Contains(t, dialog.GetLatestExchange(), "one of ye")
			before := dialog.GetState()
			require.Equal(t, 3, before.Drunk)
			action := openMerchantModal[*widgets.TavernActionDialog](t, fixture, dialog, widgets.TavernShopAwaitingApproval)
			require.True(t, action.GetIntent().Drink.Warning)
			require.Contains(t, action.GetIntent().DrinkWarning, "haven't ye had enough")
			require.Equal(t, widgets.TavernActionStop, action.GetIntent().StopRequest.Action)

			pressMerchantDialogKey(fixture, test.decision)
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return dialog.GetPhase() == widgets.TavernShopBrowse
			})
			after := dialog.GetState()
			require.Equal(t, test.wantMode, dialog.GetMode())
			require.Equal(t, test.wantDrunk, after.Drunk)
			require.Equal(t, test.wantSmashed, after.Smashed)
			require.Equal(t, before.Karma+test.karmaDelta, after.Karma)
			if test.decision == ebiten.KeyEscape {
				require.Equal(t, before.Gold, after.Gold, "Stop must not charge gold")
				require.True(t, after.CustomerEligible)
				require.Contains(t, dialog.GetLatestExchange(), "A wise choice")
				require.Empty(t, dialog.GetReceiptText())
			}

			dialog.Dismiss()
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			if test.decision == ebiten.KeyEnter {
				overlay, ok := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
				require.True(t, ok)
				require.Contains(t, overlay.GetContentText(), "The party is officially smashed.")
			}
			require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
		})
	}
}

func TestTavernInformationUnknownCancelSuccessRepeatAndInsufficient(t *testing.T) {
	t.Run("unknown cancel success and repeat", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
		input := newMerchantRuneQueue()
		fixture.scene.appendInputChars = input.Append
		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
		completeTavernAction(t, fixture, dialog, ebiten.KeyEnter)
		require.True(t, dialog.GetState().CustomerEligible)
		pressMerchantDialogKey(fixture, ebiten.KeyC)
		require.Equal(t, widgets.TavernShopConversation, dialog.GetMode())

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		require.Equal(t, widgets.TavernShopOrder, dialog.GetMode(), "blank information returns to Order")
		require.Empty(t, dialog.GetInlineFeedback())
		require.Equal(t, `"Anything else for thee?"`, dialog.GetLatestExchange())
		pressMerchantDialogKey(fixture, ebiten.KeyC)
		input.Push("something unknown")
		goldBeforeAdvice := fixture.state.PartyState.Inventory.Gold.Get()
		fixture.scene.updateDialogs()
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return dialog.GetPhase() == widgets.TavernShopBrowse
		})
		notice := waitForMerchantDialog[*widgets.TavernAdviceNotice](t, fixture)
		require.Contains(t, notice.GetContentText(), dialog.GetState().Merchant.OwnerName+" says")
		require.Contains(t, notice.GetContentText(), `"That, I cannot help thee with."`)
		pane, bounds := dialog.GetWorkspaceBounds(), notice.GetBounds()
		require.True(t, bounds.In(pane))
		require.InDelta(t, pane.Min.X+pane.Max.X, bounds.Min.X+bounds.Max.X, 1)
		require.InDelta(t, pane.Min.Y+pane.Max.Y, bounds.Min.Y+bounds.Max.Y, 1)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return fixture.scene.dialogStack.PeekTopModalDialog() == dialog
		})
		require.Contains(t, dialog.GetLatestExchange(), `"That, I cannot help thee with."`)
		require.Contains(t, dialog.GetLatestExchange(), "Of what wouldst thou hear my lore")
		require.False(t, dialog.IsDismissed())
		require.Equal(t, goldBeforeAdvice, fixture.state.PartyState.Inventory.Gold.Get())
		require.Empty(t, fixture.callbacks.TimeAdvanced(), "the notice remains inside the owning Talk minute")

		rawQuestion := "honesty please"
		input.Push(rawQuestion)
		fixture.scene.updateDialogs()
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		action := waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
		require.Equal(t, "hone", action.GetIntent().Information.Topic.Key)
		require.NotEmpty(t, action.GetIntent().InformationPitch)
		pressMerchantDialogKey(fixture, ebiten.KeyEscape)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return dialog.GetPhase() == widgets.TavernShopBrowse
		})
		require.Equal(t, rawQuestion, dialog.GetTopicInput(), "approval cancellation must restore raw input")

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		action = waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return dialog.GetPhase() == widgets.TavernShopBrowse
		})
		transcript := dialog.GetTranscript()
		require.Len(t, transcript, 1)
		require.Equal(t, rawQuestion, transcript[0].Question)
		require.NotEmpty(t, transcript[0].Answer)
		require.Empty(t, dialog.GetReceiptText())
		require.Equal(t, widgets.TavernShopConversation, dialog.GetMode())

		secondQuestion := "compassion"
		input.Push(secondQuestion)
		fixture.scene.updateDialogs()
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return dialog.GetPhase() == widgets.TavernShopBrowse
		})
		transcript = dialog.GetTranscript()
		require.Len(t, transcript, 2)
		require.Equal(t, secondQuestion, transcript[1].Question)

		dialog.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	})

	t.Run("insufficient information payment waits for acknowledgement before dismissal", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
		review, err := fixture.state.ReviewTavernShopByMerchantID(references.MerchantTavernBritain)
		require.NoError(t, err)
		fixture.state.PartyState.Inventory.Gold.Set(review.State.Meal.Charge)
		input := newMerchantRuneQueue()
		fixture.scene.appendInputChars = input.Append
		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
		completeTavernAction(t, fixture, dialog, ebiten.KeyEnter)
		pressMerchantDialogKey(fixture, ebiten.KeyC)
		input.Push("honesty")
		fixture.scene.updateDialogs()
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		resultDialog := waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
		require.True(t, resultDialog.IsVisible())
		require.False(t, dialog.IsDismissed())
		require.Equal(t, "Information Refused", resultDialog.GetView().Title)
		require.Equal(t, strings.Fields(dialog.GetLatestExchange()), strings.Fields(resultDialog.GetView().Note))
		require.Contains(t, resultDialog.GetBodyText(), "Sorry")
		require.Equal(t, strings.Fields(resultDialog.GetView().Note),
			strings.Fields(strings.TrimPrefix(resultDialog.GetBodyText(), resultDialog.GetView().Item)))
		require.Empty(t, fixture.callbacks.TimeAdvanced())
		require.Equal(t, uint16(0), fixture.state.PartyState.Inventory.Gold.Get())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		require.True(t, dialog.IsDismissed())
		require.Empty(t, dialog.GetTranscript())
		require.Empty(t, dialog.GetReceiptText())
		require.Nil(t, fixture.scene.dialogStack.PeekTopModalDialog())
		require.Equal(t, uint16(0), fixture.state.PartyState.Inventory.Gold.Get())
		require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	})
}

func TestTavernProvisionPartialAndTerminalOutcomes(t *testing.T) {
	t.Run("partial purchase continues with refreshed state", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
		review, err := fixture.state.ReviewTavernShopByMerchantID(references.MerchantTavernBritain)
		require.NoError(t, err)
		fixture.state.PartyState.Inventory.Gold.Set(uint16(review.State.Provision.PricePerPack))
		foodBefore := fixture.state.PartyState.Inventory.Provisions.Food.Get()
		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
		selectTavernOrderItem(t, fixture, dialog, widgets.TavernOrderProvision)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		action := waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyRight)
		require.Equal(t, 2, action.GetProvisionPackCount())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		resultDialog := waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
		require.Equal(t, "Partial Order", resultDialog.GetView().Title)
		require.Equal(t, widgets.TavernResultCaution, resultDialog.GetView().Tone)
		require.Contains(t, resultDialog.GetView().Detail, "1 of 2")
		require.False(t, dialog.IsDismissed())
		require.Empty(t, dialog.GetReceiptText())
		require.Equal(t, uint16(0), dialog.GetState().Gold)
		require.Equal(t, foodBefore+review.State.Provision.Provision.PackQuantity, dialog.GetState().Food)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return fixture.scene.dialogStack.PeekTopModalDialog() == dialog
		})
		dialog.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})

	for _, test := range []struct {
		name      string
		food      uint16
		wantTitle string
	}{
		{name: "charity", food: 0, wantTitle: "A Little Charity"},
		{name: "ejection", food: 3, wantTitle: "Order Refused"},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
			fixture.state.PartyState.Inventory.Gold.Set(0)
			fixture.state.PartyState.Inventory.Provisions.Food.Set(test.food)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
			selectTavernOrderItem(t, fixture, dialog, widgets.TavernOrderProvision)
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			resultDialog := waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
			require.Equal(t, test.wantTitle, resultDialog.GetView().Title)
			require.Equal(t, widgets.TavernResultFailure, resultDialog.GetView().Tone)
			require.False(t, dialog.IsDismissed(), "terminal result waits for acknowledgement")
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.True(t, dialog.IsDismissed())
			require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
		})
	}
}

func TestTavernMealAndDrinkInsufficientFundsDismiss(t *testing.T) {
	for _, test := range []struct {
		name string
		item widgets.TavernOrderItem
	}{
		{name: "meal", item: widgets.TavernOrderMeal},
		{name: "drink", item: widgets.TavernOrderDrink},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
			fixture.state.PartyState.Inventory.Gold.Set(0)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
			selectTavernOrderItem(t, fixture, dialog, test.item)
			if test.item == widgets.TavernOrderMeal {
				action := openMerchantModal[*widgets.TavernActionDialog](t, fixture, dialog, widgets.TavernShopAwaitingApproval)
				pressMerchantDialogKey(fixture, ebiten.KeyEnter)
				require.True(t, action.IsDismissed())
				resultDialog := waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
				require.Equal(t, "Meal Refused", resultDialog.GetView().Title)
				pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			} else {
				completeTavernAction(t, fixture, dialog, ebiten.KeyEnter)
			}
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.True(t, dialog.IsDismissed())
			if test.item == widgets.TavernOrderDrink {
				require.Empty(t, dialog.GetReceiptText())
				require.NotEmpty(t, dialog.GetLatestExchange())
			}
			require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
		})
	}
}

func TestTavernProseUsesStableSourceRowsWithoutGameplayRNG(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
	merchantReferences := fixture.state.GameReferences.MerchantReferences
	topics := merchantReferences.TavernInformationTopics()
	require.NotEmpty(t, topics)

	for _, definition := range merchantReferences.Definitions() {
		if definition.Family != references.MerchantFamilyTavern {
			continue
		}
		beforeRNG := fixture.state.GetRNG().Calls()
		review, err := fixture.state.ReviewTavernShopByMerchantID(definition.ID)
		require.NoError(t, err)
		prose, err := fixture.scene.resolveTavernShopProse(review.State)
		require.NoError(t, err)
		baseIndex := deterministicMerchantProseIndex(definition.ID, 4)
		values := references.MerchantTextValues{
			OwnerName: definition.OwnerName, ShopName: definition.ShopName,
			ItemName: review.State.Menu.Meal.Name, Price: int(review.State.Meal.Charge),
			Quantity: review.State.DinerCount, InformantName: review.State.PricingCharacterName,
			LocationName: definition.Location.String(), TimeOfDay: merchantTimeOfDay(fixture.state.DateTime.Hour),
		}
		greeting, err := merchantReferences.FormatText(
			references.MerchantTextID("BAR_TXT_"+strconv.Itoa(baseIndex)), values,
		)
		require.NoError(t, err)
		order, found := merchantReferences.Text(references.MerchantTextID(
			"BAR_PITCH_" + strconv.Itoa(int(review.State.Menu.Variant)),
		))
		require.True(t, found)
		require.Equal(t, quoteMerchantConversation(greeting), prose.dialog.Greeting)
		require.Equal(t, quoteMerchantConversation(order), prose.dialog.OrderPrompt)
		require.Equal(t, `"Of what wouldst thou hear my lore, sir?"`, prose.dialog.ConversationPrompt)
		require.Equal(t, `"Anything else for thee?"`, prose.dialog.ConversationExit)
		require.Equal(t, `"That, I cannot help thee with."`, prose.dialog.UnknownTopic)
		require.Contains(t, prose.drinkWarning, `"I beg thy pardon, sir,"`)
		require.Contains(t, prose.drinkWarning, `"But haven't ye had enough to drink?"`)
		require.Contains(t, prose.prudentChoice, `"A wise choice, sir.`)
		require.Contains(t, prose.prudentChoice, prose.dialog.ConversationExit)

		information, err := fixture.scene.resolveTavernInformationProse(review.State, topics[0])
		require.NoError(t, err)
		answer, err := merchantReferences.FormatText(
			references.MerchantTextID("SENS_"+strconv.Itoa(baseIndex)),
			references.MerchantTextValues{Price: topics[0].Price, InformantName: topics[0].Informant, LocationName: topics[0].Location},
		)
		require.NoError(t, err)
		require.Equal(t, quoteMerchantConversation(answer), information.answer)
		for index, topic := range topics {
			information, err = formatTavernInformationProse(merchantReferences, review.State, topic, index)
			require.NoError(t, err)
			answer, err = merchantReferences.FormatText(
				references.MerchantTextID("SENS_"+strconv.Itoa((baseIndex+index)%4)),
				references.MerchantTextValues{Price: topic.Price, InformantName: topic.Informant, LocationName: topic.Location},
			)
			require.NoError(t, err)
			require.Equal(t, quoteMerchantConversation(answer), information.answer)
		}
		require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
	}
}

func TestArmsMerchantRouteOpensProductionDialogWithoutLinearTalk(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)

	exchange := dialog.GetShell().GetExchangeView().Text
	require.Contains(t, exchange, "Buy or Sell")
	require.True(t, strings.HasPrefix(exchange, `"`), "merchant speech must start with a double quote")
	require.True(t, strings.HasSuffix(exchange, `"`), "merchant speech must end with a double quote")
	require.Equal(t, "Iolo's Bows", dialog.GetShell().GetHeaderView().ShopIdentity)
	require.Equal(t, "Gwenneth", dialog.GetShell().GetHeaderView().Owner)
	require.Equal(t, "9999", dialog.GetShell().GetHeaderView().PartyGold)
	require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHorseMerchantRoutePurchasesOneHorseAndOwnsOneManualMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
	fixture.state.PartyState.Inventory.Gold.Set(500)
	horsesBefore := countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs())

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
	price := dialog.GetQuote().Price
	require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))
	completeHorsePurchase(t, fixture, dialog)

	require.Equal(t, widgets.HorseShopSuccess, dialog.GetPhase())
	require.Contains(t, dialog.GetReceiptText(), fmt.Sprintf("Bought: one horse for %d gold.", price))
	require.Equal(t, uint16(500-price), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, horsesBefore+1, countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs()))
	require.Empty(t, fixture.callbacks.TimeAdvanced())

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
}

func TestHorseMerchantAllowsMountedDealer(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
	fixture.state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.HorseVehicle,
		fixture.state.MapState.PlayerLocation.Position,
		fixture.state.MapState.PlayerLocation.Floor,
	)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
	require.Equal(t, "Horse & Rider", dialog.GetShell().GetHeaderView().ShopIdentity)
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHorseMerchantInitialPlacementFailureShowsStableClosed(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{
		indexes.NoSprites,
		indexes.StoneBrickWall,
		indexes.DryStone,
		indexes.Water1,
	})

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)

	require.Equal(t, widgets.HorseShopStableClosed, dialog.GetPhase())
	require.Equal(t, "The stables are closed.", dialog.GetLatestExchange())
	status := strings.ToLower(dialog.GetStatusText())
	require.NotContains(t, status, "placement")
	require.NotContains(t, status, "capacity")
	require.Empty(t, dialog.GetShowroomText())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHorseMerchantUnaffordableConfirmationUsesTerminalSourceRefusal(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
	fixture.state.PartyState.Inventory.Gold.Set(1)
	horsesBefore := countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs())
	rngBefore := fixture.state.GetRNG().Calls()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
	completeHorsePurchase(t, fixture, dialog)

	require.Equal(t, widgets.HorseShopFundsRefusal, dialog.GetPhase())
	require.Equal(t, `"Thou couldst not afford to feed it!" yells Hettar.`, dialog.GetLatestExchange())
	require.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, horsesBefore, countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs()))
	require.Equal(t, rngBefore, fixture.state.GetRNG().Calls())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHorsePurchaseChildBackAndStalePlacementBoundaries(t *testing.T) {
	t.Run("Back returns to Browse", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
		setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
		goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
		horsesBefore := countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs())

		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
		purchaseDialog := openMerchantModal[*widgets.HorsePurchaseDialog](t, fixture, dialog, widgets.HorseShopAwaitingApproval)
		pressMerchantDialogKey(fixture, ebiten.KeyEscape)
		require.True(t, purchaseDialog.IsCancelled())
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return dialog.GetPhase() == widgets.HorseShopBrowse
		})

		require.Equal(t, 1, fixture.scene.dialogStack.Count())
		require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())
		require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
		require.Equal(t, horsesBefore, countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs()))
		require.Empty(t, fixture.callbacks.TimeAdvanced())
		dialog.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})

	t.Run("stale placement closes stable", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
		setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
		fixture.state.PartyState.Inventory.Gold.Set(500)
		goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
		horsesBefore := countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs())
		rngBefore := fixture.state.GetRNG().Calls()

		fixture.scene.StartTalkWorkflow(fixture.direction)
		dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
		purchaseDialog := openMerchantModal[*widgets.HorsePurchaseDialog](t, fixture, dialog, widgets.HorseShopAwaitingApproval)
		intent := purchaseDialog.GetIntent()
		blocker := map_units.NewNPCFriendlyVehiceNewRef(
			references.CarpetVehicle,
			intent.Quote.Placement.Position,
			intent.Quote.Placement.Floor,
		)
		require.True(t, fixture.state.CurrentNPCAIController.GetNpcs().AddVehicle(*blocker))
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return dialog.GetPhase() == widgets.HorseShopStableClosed
		})

		require.Equal(t, "The stables are closed.", dialog.GetLatestExchange())
		require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
		require.Equal(t, horsesBefore, countHorseTalkVehicles(fixture.state.CurrentNPCAIController.GetNpcs()))
		require.Equal(t, rngBefore, fixture.state.GetRNG().Calls())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})
}

func TestNPCAutoTalkHorseUsesSharedWorkflowWithoutSecondMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
	fixture.npc.NPCReference.Schedule.Ai[1] = byte(references.Talk3)
	fixture.npc.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionTalk

	require.True(t, fixture.scene.processNPCAutoTalk())
	dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
}

func TestHorseProseUsesCanonicalDeterministicTextsWithoutConsumingRNG(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
	review, err := fixture.state.ReviewHorseShopByMerchantID(references.MerchantHorseTrinsic)
	require.NoError(t, err)
	fixture.state.SetRandomSeed(214)
	rngBefore := fixture.state.GetRNG().Calls()

	prose, err := fixture.scene.resolveHorseShopProse(review.State)
	require.NoError(t, err)
	again, err := fixture.scene.resolveHorseShopProse(review.State)
	require.NoError(t, err)
	values := references.MerchantTextValues{
		OwnerName:    review.State.Merchant.OwnerName,
		ShopName:     review.State.Merchant.ShopName,
		ItemName:     "horse",
		Price:        review.State.Quote.Price,
		Quantity:     1,
		LocationName: review.State.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(fixture.state.DateTime.Hour),
	}
	baseIndex := deterministicMerchantProseIndex(review.State.Merchant.ID, 4)
	greeting, err := fixture.state.GameReferences.MerchantReferences.FormatText(
		references.MerchantTextID("HORSE_TXT_"+strconv.Itoa(baseIndex)), values,
	)
	require.NoError(t, err)
	rude, err := fixture.state.GameReferences.MerchantReferences.FormatText(
		references.MerchantTextID("RUDE_HORSE_"+strconv.Itoa((baseIndex+1)%4)), values,
	)
	require.NoError(t, err)
	polite, err := fixture.state.GameReferences.MerchantReferences.FormatText(
		references.MerchantTextID("POLITE_HORSE_"+strconv.Itoa((baseIndex+2)%4)), values,
	)
	require.NoError(t, err)

	require.Equal(t, prose, again)
	require.Equal(t, quoteMerchantConversation(greeting), prose.Greeting)
	require.Equal(t, quoteMerchantConversation(rude), prose.RudeFarewell)
	require.Equal(t, quoteMerchantConversation(polite), prose.PoliteFarewell)
	require.Equal(t, `"Thou couldst not afford to feed it!" yells Hettar.`, prose.FundsRefusal)
	require.Equal(t, rngBefore, fixture.state.GetRNG().Calls())
}

func TestHorseTalkFinallyRemovesOnlyOwnedChildAndParent(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHorseTrinsic)
	setHorseTalkCandidateTiles(t, fixture, [4]indexes.SpriteIndex{indexes.Grass})
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)
	ownedPurchase := openMerchantModal[*widgets.HorsePurchaseDialog](t, fixture, ownedShop, widgets.HorseShopAwaitingApproval)
	review, err := fixture.state.ReviewHorseShopByMerchantID(references.MerchantHorseTrinsic)
	require.NoError(t, err)
	prose, err := fixture.scene.resolveHorseShopProse(review.State)
	require.NoError(t, err)
	decoyShop := widgets.NewHorseShopDialog(widgets.HorseShopDialogConfig{
		Keyboard:       fixture.scene.keyboard,
		MerchantSprite: fixture.scene.spriteSheet.GetSprite(review.MerchantSpriteIndex),
		HorseSprite:    fixture.scene.spriteSheet.GetSprite(indexes.HorseRight),
		State:          review.State,
		Prose:          prose,
	})
	decoyPurchase := widgets.NewHorsePurchaseDialog(widgets.HorsePurchaseDialogConfig{
		Keyboard: fixture.scene.keyboard,
		Intent:   ownedPurchase.GetIntent(),
	})
	fixture.scene.dialogStack.PushModalDialog(decoyShop)
	fixture.scene.dialogStack.PushModalDialog(decoyPurchase)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedPurchase.IsDismissed())
	require.True(t, ownedShop.IsDismissed())
	require.False(t, decoyPurchase.IsDismissed())
	require.False(t, decoyShop.IsDismissed())
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, decoyPurchase, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestManualHealerTalkRepeatsFreeServicesWithinOneMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Minoc, references.MerchantHealerMinoc)
	avatar := fixture.state.PartyState.GetCharacter(0)
	avatar.SetMaxHp(60)
	avatar.SetHp(18)
	avatar.SetStatus(party_state.Poisoned)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)
	require.Equal(t, "The Healers Mission", dialog.GetShell().GetHeaderView().ShopIdentity)
	require.Equal(t, "Regina", dialog.GetShell().GetHeaderView().Owner)
	require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))

	completeHealerService(t, fixture, dialog)
	require.Equal(t, party_state.Good, avatar.GetStatus())
	require.Equal(t, uint16(18), avatar.GetCurrentHp())
	require.Contains(t, dialog.GetReceiptText(), "cured")
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyH)
	completeHealerService(t, fixture, dialog)
	require.Equal(t, party_state.Good, avatar.GetStatus())
	require.Equal(t, uint16(60), avatar.GetCurrentHp())
	require.Contains(t, dialog.GetReceiptText(), "healed")
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestManualHealerAllTreatsEveryEligibleRecipientAtomically(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Minoc, references.MerchantHealerMinoc)
	avatar := fixture.state.PartyState.GetCharacter(0)
	avatar.SetMaxHp(60)
	avatar.SetHp(18)
	avatar.SetStatus(party_state.Poisoned)
	iolo := fixture.state.PartyState.GetCharacter(1)
	iolo.PartyMap = party_state.PartyMapActive
	iolo.SetMaxHp(60)
	iolo.SetHp(42)
	iolo.SetStatus(party_state.Poisoned)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyA)
	completeHealerService(t, fixture, dialog)

	require.Equal(t, party_state.Good, avatar.GetStatus())
	require.Equal(t, party_state.Good, iolo.GetStatus())
	require.Contains(t, dialog.GetReceiptText(), "Cured 2 party members")
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHealerTreatmentApprovalCancelReturnsToBrowseWithoutMutation(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Minoc, references.MerchantHealerMinoc)
	avatar := fixture.state.PartyState.GetCharacter(0)
	avatar.SetMaxHp(60)
	avatar.SetHp(18)
	avatar.SetStatus(party_state.Poisoned)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.Equal(t, widgets.HealerShopAwaitingApproval, dialog.GetPhase())
	treatmentDialog := waitForMerchantDialog[*widgets.HealerTreatmentDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	require.True(t, treatmentDialog.IsCancelled())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.HealerShopBrowse
	})

	require.Equal(t, party_state.Poisoned, avatar.GetStatus())
	require.Equal(t, uint16(18), avatar.GetCurrentHp())
	require.Empty(t, fixture.callbacks.TimeAdvanced())

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHealerInsufficientFundsReturnsToBrowseWithoutMutation(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Trinsic, references.MerchantHealerTrinsic)
	avatar := fixture.state.PartyState.GetCharacter(0)
	avatar.SetMaxHp(60)
	avatar.SetHp(18)
	avatar.SetStatus(party_state.Good)
	fixture.state.PartyState.Inventory.Gold.Set(1)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyH)
	completeHealerService(t, fixture, dialog)

	require.Equal(t, widgets.HealerShopBrowse, dialog.GetPhase())
	require.Equal(t, uint16(18), avatar.GetCurrentHp())
	require.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Gold.Get())
	require.Contains(t, strings.ToLower(dialog.GetLatestExchange()), "charity")
	require.Contains(t, dialog.GetReceiptText(), "need 40 gold")

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestGuildResultProseUsesCapturedBuyer(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.state.PartyState.GetCharacter(0).Gender = party_state.Female
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	pricePerBundle := dialog.GetSelectedQuote().PricePerBundle

	completeGuildPurchase(t, fixture, dialog, 1)
	require.Equal(t, "\"Sold!\" says Braunam. \"What else, m'lady?\"", dialog.GetLatestExchange())
	require.Equal(t, fmt.Sprintf(
		"Bought: 1 bundle of keys for %d gold. Thou hast now 13.",
		pricePerBundle,
	), dialog.GetReceiptText())

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestGuildBatchReceiptSaturatesWithoutOverflowWarning(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.state.PartyState.Inventory.Gold.Set(5000)
	fixture.state.PartyState.Inventory.Provisions.Keys.Set(94)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	pricePerBundle := dialog.GetSelectedQuote().PricePerBundle

	completeGuildPurchase(t, fixture, dialog, 2)

	require.Equal(t, uint16(99), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
	require.Equal(t, fmt.Sprintf(
		"Bought: 2 bundles of keys for %d gold. Thou hast now 99.",
		pricePerBundle*2,
	), dialog.GetReceiptText())
	lowerReceipt := strings.ToLower(dialog.GetReceiptText())
	require.NotContains(t, lowerReceipt, "overflow")
	require.NotContains(t, lowerReceipt, "waste")
	require.NotContains(t, lowerReceipt, "full charge")

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestGuildBatchReceiptAppendsFaulineiTheft(t *testing.T) {
	const seed = 240211

	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.state.SetRandomSeed(seed)
	fixture.state.ShadowlordState.AutomaticPresence = references.Faulinei
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.state.PartyState.Inventory.Provisions.Torches.Set(5)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	quote := dialog.GetSelectedQuote()
	require.Equal(t, references.Torches, quote.Provision)
	expectedTheft := uint16(beradrng.New(seed).Intn(64) + 1)

	completeGuildPurchase(t, fixture, dialog, 4)

	require.Equal(t, uint16(25), fixture.state.PartyState.Inventory.Provisions.Torches.Get())
	require.Equal(t, uint16(1000-quote.PricePerBundle*4)-expectedTheft,
		fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, fmt.Sprintf(
		"Bought: 4 bundles of torches for %d gold. Thou hast now 25. %d more gold was stolen!",
		quote.PricePerBundle*4,
		expectedTheft,
	), dialog.GetReceiptText())

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestGuildAuthorityErrorRemovesQuantityModalAndUsesBoundedSessionError(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	keysBefore := fixture.state.PartyState.Inventory.Provisions.Keys.Get()
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	purchaseDialog := openMerchantModal[*widgets.GuildPurchaseDialog](t, fixture, dialog, widgets.GuildShopAwaitingQuantity)
	fixture.state.PartyState.GetCharacter(0).SetStatus(party_state.Dead)

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.GuildShopError
	})

	require.True(t, purchaseDialog.IsDismissed())
	require.False(t, purchaseDialog.HasPendingPurchaseRequest(), "the failed authority request must be consumed once")
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, "The guild transaction could not be completed.", dialog.GetLatestExchange())
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, keysBefore, fixture.state.PartyState.Inventory.Provisions.Keys.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestGuildCosmeticProseSelectionDoesNotConsumeGameStateRNG(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	session, err := fixture.state.BeginMerchantSession(fixture.npc)
	require.NoError(t, err)
	state, err := fixture.state.GetGuildShopState(session)
	require.NoError(t, err)

	fixture.state.SetRandomSeed(8675309)
	prose, err := fixture.scene.resolveGuildShopProse(state)
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(prose.Greeting, `"`))
	require.True(t, strings.HasSuffix(prose.Greeting, `"`))
	require.False(t, strings.HasPrefix(prose.Greeting, `""`))
	require.False(t, strings.HasSuffix(prose.Greeting, `""`))
	afterProse := fixture.state.RollDice(1000)
	fixture.state.SetRandomSeed(8675309)
	require.Equal(t, fixture.state.RollDice(1000), afterProse)
}

func TestNormalizeOriginalGuildGreetingBalancesOnlyOuterQuotes(t *testing.T) {
	for _, test := range []struct {
		name     string
		greeting string
		expected string
	}{
		{
			name:     "missing opening quote",
			greeting: `Welcome to the guild."`,
			expected: `"Welcome to the guild."`,
		},
		{
			name:     "already balanced",
			greeting: `"Welcome to the guild."`,
			expected: `"Welcome to the guild."`,
		},
		{
			name:     "internal quotation preserved",
			greeting: `He says "hail" to every member."`,
			expected: `"He says "hail" to every member."`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.expected, normalizeOriginalGuildGreeting(test.greeting))
		})
	}
}

func TestGuildTalkFinallyRemovesOnlyOwnedDialogInstance(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.NewMagincia, references.MerchantGuildNewMagincia)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
	ownedPurchase := openMerchantModal[*widgets.GuildPurchaseDialog](t, fixture, ownedShop, widgets.GuildShopAwaitingQuantity)
	decoy := widgets.NewGuildPurchaseDialog(widgets.GuildPurchaseDialogConfig{
		Keyboard: fixture.scene.keyboard,
		Quote:    ownedPurchase.GetQuote(),
		Pitch:    "Decoy purchase pitch.",
	})
	fixture.scene.dialogStack.PushModalDialog(decoy)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedPurchase.IsDismissed())
	require.True(t, ownedShop.IsDismissed())
	require.False(t, decoy.IsDismissed())
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, decoy, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestTavernTalkFinallyRemovesOnlyOwnedDialogInstances(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
	ownedAction := openMerchantModal[*widgets.TavernActionDialog](t, fixture, ownedShop, widgets.TavernShopAwaitingApproval)
	review, err := fixture.state.ReviewTavernShopByMerchantID(references.MerchantTavernBritain)
	require.NoError(t, err)
	prose, err := fixture.scene.resolveTavernShopProse(review.State)
	require.NoError(t, err)
	decoyShop := widgets.NewTavernShopDialog(widgets.TavernShopDialogConfig{
		Keyboard: fixture.scene.keyboard, MerchantSprite: fixture.scene.spriteSheet.GetSprite(review.MerchantSpriteIndex),
		State: review.State, Prose: prose.dialog,
	})
	decoyAction := widgets.NewTavernActionDialog(widgets.TavernActionDialogConfig{
		Keyboard: fixture.scene.keyboard, Intent: ownedAction.GetIntent(),
	})
	fixture.scene.dialogStack.PushModalDialog(decoyShop)
	fixture.scene.dialogStack.PushModalDialog(decoyAction)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedShop.IsDismissed())
	require.True(t, ownedAction.IsDismissed())
	require.False(t, decoyShop.IsDismissed())
	require.False(t, decoyAction.IsDismissed())
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, decoyAction, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestArmsNestedTradeCancellationReturnsToOwningShop(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Bow)
	trade := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
	require.Equal(t, 2, fixture.scene.dialogStack.Count())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ArmsShopBrowse
	})

	require.True(t, trade.IsCancelled())
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestManualArmsTalkRepeatedBatchBuySellModeSwitchesOwnOneMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.state.PartyState.Inventory.Equipment.Set(references.Bow, 2)
	fixture.state.PartyState.GetCharacter(0).SetEquippedItem(references.SlotWeapon, references.Bow)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Bow)
	buyQuote := armsBuyRow(t, shop, references.Bow)
	require.Equal(t, uint16(1), buyQuote.Readied)

	completeArmsTrade(t, fixture, shop, 3)
	require.Equal(t, uint16(5), fixture.state.PartyState.Inventory.Equipment.Get(references.Bow))
	require.Equal(t, uint16(1000-buyQuote.Value*3), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, fmt.Sprintf(
		"Bought: Bow x3 for %d gold. Thou hast now 5.",
		buyQuote.Value*3,
	), shop.GetReceiptText())
	require.Equal(t, uint16(5), armsBuyRow(t, shop, references.Bow).Quantity)
	require.Equal(t, strconv.Itoa(1000-buyQuote.Value*3), shop.GetShell().GetHeaderView().PartyGold)

	secondTrade := completeArmsTrade(t, fixture, shop, 1)
	require.Equal(t, uint16(5), secondTrade.GetIntent().BuyQuote.QuantityBefore)
	require.Equal(t, uint16(1000-buyQuote.Value*3), secondTrade.GetIntent().BuyQuote.Gold)
	require.Equal(t, uint16(6), fixture.state.PartyState.Inventory.Equipment.Get(references.Bow))
	goldAfterBuys := uint16(1000 - buyQuote.Value*4)
	require.Equal(t, goldAfterBuys, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, strconv.Itoa(int(goldAfterBuys)), shop.GetShell().GetHeaderView().PartyGold)
	require.Equal(t, uint16(1), armsBuyRow(t, shop, references.Bow).Readied)
	pressMerchantDialogKey(fixture, ebiten.KeyS)
	require.Equal(t, widgets.ArmsShopSell, shop.GetMode())
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopSell, references.Bow)
	sellQuote := armsSellRow(t, shop, references.Bow)
	completeArmsTrade(t, fixture, shop, 2)
	require.Equal(t, uint16(4), fixture.state.PartyState.Inventory.Equipment.Get(references.Bow))
	goldAfterSale := goldAfterBuys + uint16(sellQuote.Value*2)
	require.Equal(t, goldAfterSale, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, strconv.Itoa(int(goldAfterSale)), shop.GetShell().GetHeaderView().PartyGold)
	require.Equal(t, fmt.Sprintf(
		"Sold: Bow x2 for %d gold. Thou hast now 4.",
		sellQuote.Value*2,
	), shop.GetReceiptText())
	require.Equal(t, uint16(4), armsSellRow(t, shop, references.Bow).Quantity)
	require.Equal(t, uint16(1), armsSellRow(t, shop, references.Bow).Readied)

	pressMerchantDialogKey(fixture, ebiten.KeyTab)
	require.Equal(t, widgets.ArmsShopBuy, shop.GetMode())
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
}

func TestArmsAmmunitionPurchaseRefillsAndReportsAggregateResult(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.state.PartyState.Inventory.Equipment.Set(references.Arrows, 5)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Arrows)
	quote := armsBuyRow(t, shop, references.Arrows)

	trade := completeArmsTrade(t, fixture, shop, 1)

	require.True(t, trade.GetIntent().BuyQuote.AmmoRefill)
	require.Equal(t, uint16(99), fixture.state.PartyState.Inventory.Equipment.Get(references.Arrows))
	require.Equal(t, fmt.Sprintf(
		"Bought: Arrows x1 ammunition refill for %d gold. Thou hast now 99.",
		quote.Value,
	), shop.GetReceiptText())
	require.Equal(t, uint16(99), armsBuyRow(t, shop, references.Arrows).Quantity)
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestArmsSellStateExcludesWornOnlyEquipment(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)
	fixture.state.PartyState.GetCharacter(0).SetEquippedItem(references.SlotWeapon, references.LongSword)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyS)

	for _, row := range shop.GetRows() {
		require.NotEqual(t, references.LongSword, row.Equipment)
	}
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestArmsBuyCapacityRefusalReturnsToBuy(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Bow)
	trade := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
	fixture.state.PartyState.Inventory.Equipment.Set(references.Bow, 99)

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ArmsShopBrowse
	})

	require.False(t, trade.HasPendingRequest())
	require.Equal(t, widgets.ArmsShopBuy, shop.GetMode())
	require.Equal(t, "No Bow bought: thou canst not carry any more!", shop.GetReceiptText())
	refreshed := armsBuyRow(t, shop, references.Bow)
	require.Equal(t, uint16(99), refreshed.Quantity)
	require.Zero(t, refreshed.Maximum)
	require.Equal(t, "1000", shop.GetShell().GetHeaderView().PartyGold)
	require.False(t, shop.IsDismissed())
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestArmsCurrentBuyBlockersUseMerchantRefusalsBeforeTradeModal(t *testing.T) {
	t.Run("capacity stays in Buy", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
		fixture.state.PartyState.Inventory.Gold.Set(1000)
		fixture.state.PartyState.Inventory.Equipment.Set(references.Arrows, 99)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
		selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Arrows)

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)

		require.Equal(t, widgets.ArmsShopBrowse, shop.GetPhase())
		require.Contains(t, shop.GetLatestExchange(), "Thou canst not carry any more")
		require.Contains(t, strings.Join(strings.Fields(shop.GetDetailText()), " "), "Thou canst not carry any more!")
		require.Equal(t, 1, fixture.scene.dialogStack.Count())
		shop.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})

	t.Run("insufficient funds is terminal after acknowledgement", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
		fixture.state.PartyState.Inventory.Gold.Set(0)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)

		require.Equal(t, widgets.ArmsShopTerminalRefusal, shop.GetPhase())
		require.Contains(t, shop.GetLatestExchange(), "Can't pay?! Out with thee!")
		require.False(t, shop.IsDismissed())
		require.Equal(t, 1, fixture.scene.dialogStack.Count())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		require.True(t, shop.IsDismissed())
		require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	})
}

func TestArmsInsufficientFundsRefusalEndsTalk(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
		t.Run(key.String(), func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
			fixture.state.PartyState.Inventory.Gold.Set(1000)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
			selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Bow)
			openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
			fixture.state.PartyState.Inventory.Gold.Set(0)

			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return shop.GetPhase() == widgets.ArmsShopTerminalRefusal
			})

			require.False(t, shop.IsDismissed(), "authority refusal must remain visible")
			require.Equal(t, "No Bow bought: thou hast not enough gold!", shop.GetReceiptText())
			require.Contains(t, shop.GetLatestExchange(), "Out with thee")
			require.Equal(t, 1, fixture.scene.dialogStack.Count())
			require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
			require.Empty(t, fixture.callbacks.TimeAdvanced())

			pressMerchantDialogKey(fixture, key)
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.True(t, shop.IsDismissed())
			require.Zero(t, fixture.scene.dialogStack.Count())
			require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
		})
	}
}

func TestArmsSellRefusalTerminalAndNonterminalPolicies(t *testing.T) {
	t.Run("used ammunition is terminal", func(t *testing.T) {
		for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
			t.Run(key.String(), func(t *testing.T) {
				fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
				fixture.state.PartyState.Inventory.Equipment.Set(references.Arrows, 5)
				fixture.scene.StartTalkWorkflow(fixture.direction)
				shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
				selectArmsEquipment(t, fixture, shop, widgets.ArmsShopSell, references.Arrows)
				trade := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
				require.Equal(t, widgets.ArmsTradeUsedAmmunitionRefusal, trade.GetRefusalKind())

				pressMerchantDialogKey(fixture, key)
				require.False(t, trade.IsCancelled(), "refusal acknowledgement is not cancellation")
				pumpMerchantWorkflowUntil(t, fixture, func() bool {
					return !fixture.state.EventQueueManager.HasActiveWorkflows()
				})

				require.True(t, shop.IsDismissed())
				require.Contains(t, shop.GetReceiptText(), "We don't deal in used ammunition!")
				require.Equal(t, uint16(5), fixture.state.PartyState.Inventory.Equipment.Get(references.Arrows))
				require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
			})
		}
	})

	t.Run("zero price returns to Sell", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
		fixture.state.PartyState.Inventory.Equipment.Set(references.JewelShield, 1)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
		selectArmsEquipment(t, fixture, shop, widgets.ArmsShopSell, references.JewelShield)
		trade := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
		require.Equal(t, widgets.ArmsTradeZeroPriceRefusal, trade.GetRefusalKind())

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return shop.GetPhase() == widgets.ArmsShopBrowse
		})

		require.Equal(t, widgets.ArmsShopSell, shop.GetMode())
		require.False(t, shop.IsDismissed())
		require.Equal(t, "No Jewel Shield sold: \"That, I cannot buy from thee.\"", shop.GetReceiptText())
		shop.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})

	t.Run("stale gold capacity returns to Sell", func(t *testing.T) {
		fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
		fixture.state.PartyState.Inventory.Gold.Set(100)
		fixture.state.PartyState.Inventory.Equipment.Set(references.LongSword, 2)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
		selectArmsEquipment(t, fixture, shop, widgets.ArmsShopSell, references.LongSword)
		trade := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
		require.Equal(t, widgets.ArmsTradeNoRefusal, trade.GetRefusalKind())
		fixture.state.PartyState.Inventory.Gold.Set(9999)

		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return shop.GetPhase() == widgets.ArmsShopBrowse
		})

		require.Equal(t, widgets.ArmsShopSell, shop.GetMode())
		require.False(t, shop.IsDismissed())
		require.Equal(t, "No Long Sword sold: thou canst not carry any more gold!", shop.GetReceiptText())
		require.Equal(t, "9999", shop.GetShell().GetHeaderView().PartyGold)
		refreshed := armsSellRow(t, shop, references.LongSword)
		require.Equal(t, uint16(2), refreshed.Quantity)
		require.Zero(t, refreshed.Maximum)
		require.Equal(t, references.LongSword, shop.GetSelectedEquipment())
		require.Equal(t, uint16(2), fixture.state.PartyState.Inventory.Equipment.Get(references.LongSword))
		shop.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})
}

func TestArmsPreviewBuyAndSellStopBeforeAuthorityWithoutMutation(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.state.PartyState.Inventory.Equipment.Set(references.Bow, 3)
	session, err := fixture.state.BeginMerchantSession(fixture.npc)
	require.NoError(t, err)
	review, err := fixture.state.ReviewArmsShopByMerchantID(references.MerchantArmsBritain)
	require.NoError(t, err)
	beforeDate := fixture.state.DateTime
	beforeGold := fixture.state.PartyState.Inventory.Gold.Get()
	beforeBow := fixture.state.PartyState.Inventory.Equipment.Get(references.Bow)
	beforeRNG := fixture.state.GetRNG().Calls()

	fixture.scene.startArmsMerchantPreviewWorkflow(review)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Bow)
	completeArmsTrade(t, fixture, shop, 2)
	require.Contains(t, shop.GetReceiptText(), "gold and equipment are unchanged")
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopSell, references.Bow)
	completeArmsTrade(t, fixture, shop, 2)

	require.Equal(t, beforeGold, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, beforeBow, fixture.state.PartyState.Inventory.Equipment.Get(references.Bow))
	require.Equal(t, beforeDate, fixture.state.DateTime)
	require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
	_, err = fixture.state.GetArmsShopState(session)
	require.NoError(t, err, "Preview must not issue, supersede, or end a MerchantSession")
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.Zero(t, fixture.scene.dialogStack.Count())
	fixture.state.EndMerchantSession(session)
}

func TestNPCAutoTalkArmsUsesSharedWorkflowWithoutSecondMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.npc.NPCReference.Schedule.Ai[1] = byte(references.Talk3)
	fixture.npc.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionTalk

	require.True(t, fixture.scene.processNPCAutoTalk())
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	require.Equal(t, references.AiType(references.Wander3), references.AiType(fixture.npc.NPCReference.Schedule.Ai[1]))
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.Zero(t, fixture.scene.dialogStack.Count())
}

func TestArmsProseResolutionIsCanonicalDeterministicAndDoesNotConsumeRNG(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.state.PartyState.Inventory.Equipment.Set(references.Bow, 2)
	review, err := fixture.state.ReviewArmsShopByMerchantID(references.MerchantArmsBritain)
	require.NoError(t, err)
	fixture.state.SetRandomSeed(8675309)
	beforeRNG := fixture.state.GetRNG().Calls()

	shopProse, err := fixture.scene.resolveArmsShopProse(review.State)
	require.NoError(t, err)
	buyProse, err := fixture.scene.resolveArmsTradeProse(review.State, widgets.ArmsTradeIntent{
		Mode: widgets.ArmsShopBuy, BuyQuote: review.State.BuyQuotes[2],
	})
	require.NoError(t, err)
	sellProse, err := fixture.scene.resolveArmsTradeProse(review.State, widgets.ArmsTradeIntent{
		Mode: widgets.ArmsShopSell, SellQuote: review.State.SellQuotes[0],
	})
	require.NoError(t, err)
	again, err := fixture.scene.resolveArmsTradeProse(review.State, widgets.ArmsTradeIntent{
		Mode: widgets.ArmsShopSell, SellQuote: review.State.SellQuotes[0],
	})
	require.NoError(t, err)

	require.Contains(t, shopProse.Greeting, "Iolo's Bows")
	require.NotEmpty(t, buyProse.Pitch)
	require.NotEmpty(t, buyProse.Offer)
	require.NotEmpty(t, buyProse.Refusal)
	require.Equal(t, sellProse, again)
	require.NotEmpty(t, sellProse.Offer)
	require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
}

func TestArmsTalkFinallyRemovesOnlyOwnedDialogInstances(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, ownedShop, widgets.ArmsShopBuy, references.Bow)
	ownedTrade := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, ownedShop, widgets.ArmsShopAwaitingTrade)
	decoy := widgets.NewArmsTradeDialog(widgets.ArmsTradeDialogConfig{
		Keyboard: fixture.scene.keyboard,
		Intent:   ownedTrade.GetIntent(),
		Prose: widgets.ArmsTradeDialogProse{
			Pitch: "Decoy pitch.", Offer: "Decoy offer.", Refusal: "Decoy refusal.",
		},
	})
	fixture.scene.dialogStack.PushModalDialog(decoy)
	review, err := fixture.state.ReviewArmsShopByMerchantID(references.MerchantArmsBritain)
	require.NoError(t, err)
	prose, err := fixture.scene.resolveArmsShopProse(review.State)
	require.NoError(t, err)
	decoyShop := widgets.NewArmsShopDialog(widgets.ArmsShopDialogConfig{
		Keyboard:       fixture.scene.keyboard,
		MerchantSprite: fixture.scene.spriteSheet.GetSprite(review.MerchantSpriteIndex),
		State:          review.State,
		Prose:          prose,
	})
	fixture.scene.dialogStack.PushModalDialog(decoyShop)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedTrade.IsDismissed())
	require.True(t, ownedShop.IsDismissed())
	require.False(t, decoy.IsDismissed())
	require.False(t, decoyShop.IsDismissed())
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, decoyShop, fixture.scene.dialogStack.PeekTopModalDialog())
}

func TestManualReagentTalkRepeatedPurchasesRefreshWithinOneMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
	fixture.state.PartyState.Inventory.Gold.Set(500)
	fixture.state.PartyState.Inventory.Reagent.Set(references.SpiderSilk, 0)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	selectReagent(t, fixture, shop, references.SpiderSilk)
	quote := shop.GetSelectedQuote()

	first := completeReagentPurchase(t, fixture, shop, 1)
	require.False(t, first.HasPendingPurchaseRequest())
	require.Equal(t, quote.BundleQuantity, fixture.state.PartyState.Inventory.Reagent.Get(references.SpiderSilk))
	require.Equal(t, quote.BundleQuantity, shop.GetSelectedQuote().QuantityBefore)
	require.Equal(t, strconv.Itoa(500-quote.PricePerBundle), shop.GetShell().GetHeaderView().PartyGold)
	requireReagentDialogMatchesAuthority(t, fixture, shop, references.MerchantReagentYew)

	second := completeReagentPurchase(t, fixture, shop, 2)
	require.Equal(t, quote.BundleQuantity, second.GetQuote().QuantityBefore)
	require.Equal(t, uint16(500-quote.PricePerBundle), second.GetQuote().Gold)
	expectedQuantity := quote.BundleQuantity * 3
	expectedGold := uint16(500 - quote.PricePerBundle*3)
	require.Equal(t, expectedQuantity, fixture.state.PartyState.Inventory.Reagent.Get(references.SpiderSilk))
	require.Equal(t, expectedGold, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, expectedQuantity, shop.GetSelectedQuote().QuantityBefore)
	require.Equal(t, strconv.Itoa(int(expectedGold)), shop.GetShell().GetHeaderView().PartyGold)
	requireReagentDialogMatchesAuthority(t, fixture, shop, references.MerchantReagentYew)
	require.Equal(t, fmt.Sprintf(
		"Bought: 2 bundles of Spider Silk for %d gold. Thou hast now %d. Thou hast %d gold left.",
		quote.PricePerBundle*2,
		expectedQuantity,
		expectedGold,
	), shop.GetReceiptText())
	require.Equal(t, "\"I thank thee!\" says Madam Pendra. \"Anything else?\"", shop.GetLatestExchange())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	require.Equal(t, widgets.ReagentShopFarewell, shop.GetPhase())
	require.True(t, fixture.state.EventQueueManager.HasActiveWorkflows())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
}

func TestMerchantTalkFixtureShiftDoesNotLeakIntoScriptedNavigation(t *testing.T) {
	for _, polluted := range []bool{true, false} {
		t.Run(fmt.Sprintf("foreignShift=%t", polluted), func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
			t.Cleanup(fixture.state.EventQueueManager.Shutdown)
			held := true
			if polluted {
				fixture.scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool {
					return held && key == ebiten.KeyShiftLeft
				})
			}
			fixture.scene.StartTalkWorkflow(fixture.direction)
			shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
			quotes := shop.GetCatalogQuotes()
			require.GreaterOrEqual(t, len(quotes), 2)
			require.Equal(t, quotes[0].Reagent, shop.GetSelectedReagent())
			pressMerchantDialogKey(fixture, ebiten.KeyDown)
			if polluted {
				// Shift converts the scripted Down into PageDown; this list has no paging.
				require.Equal(t, quotes[0].Reagent, shop.GetSelectedReagent())
				held = false
				pressMerchantDialogKey(fixture, ebiten.KeyDown)
			}
			require.Equal(t, quotes[1].Reagent, shop.GetSelectedReagent())
			pressMerchantDialogKey(fixture, ebiten.KeyEscape)
			require.Equal(t, widgets.ReagentShopFarewell, shop.GetPhase())
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			require.True(t, shop.IsDismissed(), "detached fixtures still accept scripted controls")
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
		})
	}
}

func TestReagentPurchaseCancelReturnsToBrowseWithoutMutation(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
	fixture.state.PartyState.Inventory.Gold.Set(500)
	fixture.state.PartyState.Inventory.Reagent.Set(references.Ginseng, 7)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	selectReagent(t, fixture, shop, references.Ginseng)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
	reagentBefore := fixture.state.PartyState.Inventory.Reagent.Get(references.Ginseng)
	purchase := openMerchantModal[*widgets.ReagentPurchaseDialog](t, fixture, shop, widgets.ReagentShopAwaitingQuantity)

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ReagentShopBrowse
	})

	require.True(t, purchase.IsCancelled())
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, reagentBefore, fixture.state.PartyState.Inventory.Reagent.Get(references.Ginseng))
	require.Empty(t, fixture.callbacks.TimeAdvanced())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	require.Equal(t, widgets.ReagentShopFarewell, shop.GetPhase())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestReagentAggregatePurchaseRefreshesSaturationAndCompleteReceipt(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Cove, references.MerchantReagentCove)
	fixture.state.PartyState.Inventory.Gold.Set(100)
	fixture.state.PartyState.Inventory.Reagent.Set(references.BloodMoss, 94)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	selectReagent(t, fixture, shop, references.BloodMoss)
	quote := shop.GetSelectedQuote()
	require.Equal(t, uint16(2), quote.BundleQuantity)

	completeReagentPurchase(t, fixture, shop, 3)

	require.Equal(t, uint16(99), fixture.state.PartyState.Inventory.Reagent.Get(references.BloodMoss))
	require.Equal(t, uint16(100-quote.PricePerBundle*3), fixture.state.PartyState.Inventory.Gold.Get())
	refreshed := shop.GetSelectedQuote()
	require.Equal(t, uint16(99), refreshed.QuantityBefore)
	require.Equal(t, game_state.ReagentOfferBlockInventoryFull, refreshed.BlockReason)
	require.Equal(t, fmt.Sprintf(
		"Bought: 3 bundles of Blood Moss for %d gold. Thou hast now 99. Thou hast %d gold left.",
		quote.PricePerBundle*3,
		100-quote.PricePerBundle*3,
	), shop.GetReceiptText())

	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestReagentResultDisplayAppendsFaulineiTheftAndBalancesSpeech(t *testing.T) {
	display := resolveReagentPurchaseResultDisplay(references.MerchantDefinition{
		OwnerName: "Enlor",
	}, game_state.ReagentPurchaseResult{
		Reagent:          references.BloodMoss,
		ReagentName:      "Blood Moss",
		BundleCount:      3,
		QuantityBefore:   94,
		QuantityAfter:    99,
		ActualReceipt:    5,
		TotalPrice:       39,
		Theft:            7,
		GoldAfterTheft:   54,
		GoldAfterPayment: 61,
	})

	require.Equal(t, "\"I thank thee!\" says Enlor. \"Anything else?\"", display.MerchantExchange)
	require.Equal(t, 4, strings.Count(display.MerchantExchange, `"`))
	require.Equal(t,
		"Bought: 3 bundles of Blood Moss for 39 gold. Thou hast now 99. 7 more gold was stolen! "+
			"Thou hast 54 gold left.",
		display.Receipt,
	)
}

func TestReagentBlockedOfferDoesNotOpenPurchaseChild(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
	fixture.state.PartyState.Inventory.Gold.Set(500)
	fixture.state.PartyState.Inventory.Reagent.Set(references.Ginseng, 99)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	selectReagent(t, fixture, shop, references.Ginseng)
	require.Equal(t, game_state.ReagentOfferBlockInventoryFull, shop.GetSelectedQuote().BlockReason)

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)

	require.Equal(t, widgets.ReagentShopBrowse, shop.GetPhase())
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, uint16(99), fixture.state.PartyState.Inventory.Reagent.Get(references.Ginseng))
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestReagentAuthorityErrorRemovesChildAndUsesBoundedSessionError(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
	fixture.state.PartyState.Inventory.Gold.Set(500)
	fixture.state.PartyState.Inventory.Reagent.Set(references.Ginseng, 7)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	selectReagent(t, fixture, shop, references.Ginseng)
	purchase := openMerchantModal[*widgets.ReagentPurchaseDialog](t, fixture, shop, widgets.ReagentShopAwaitingQuantity)
	fixture.state.PartyState.GetCharacter(0).SetStatus(party_state.Dead)

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ReagentShopSessionError
	})

	require.True(t, purchase.IsDismissed())
	require.False(t, purchase.HasPendingPurchaseRequest())
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, "The reagent transaction could not be completed.", shop.GetLatestExchange())
	require.Equal(t, uint16(500), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, uint16(7), fixture.state.PartyState.Inventory.Reagent.Get(references.Ginseng))
	require.Equal(t, uint16(7), shop.GetSelectedQuote().QuantityBefore, "failed authority must not patch stale local state")

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestReagentProseUsesDeterministicSourceTextWithoutGameplayRNG(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Moonglow, references.MerchantReagentMoonglow)
	review, err := fixture.state.ReviewReagentShopByMerchantID(references.MerchantReagentMoonglow)
	require.NoError(t, err)
	fixture.state.SetRandomSeed(8675309)
	beforeRNG := fixture.state.GetRNG().Calls()

	prose, err := fixture.scene.resolveReagentShopProse(review.State)
	require.NoError(t, err)
	greetingID := references.MerchantTextID(
		"REAG_TXT_" + strconv.Itoa(deterministicMerchantProseIndex(review.State.Merchant.ID, 4)),
	)
	source, err := fixture.state.GameReferences.MerchantReferences.FormatText(greetingID, references.MerchantTextValues{
		OwnerName:    review.State.Merchant.OwnerName,
		ShopName:     review.State.Merchant.ShopName,
		LocationName: review.State.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(fixture.state.DateTime.Hour),
	})
	require.NoError(t, err)

	require.Equal(t, quoteMerchantConversation(source), prose.Greeting)
	rudeID := references.MerchantTextID(
		"RUDE_REAG_" + strconv.Itoa(deterministicReagentFarewellProseIndex(review.State.Merchant.ID, false)),
	)
	rude, err := fixture.state.GameReferences.MerchantReferences.FormatText(rudeID, references.MerchantTextValues{
		OwnerName:    review.State.Merchant.OwnerName,
		ShopName:     review.State.Merchant.ShopName,
		LocationName: review.State.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(fixture.state.DateTime.Hour),
	})
	require.NoError(t, err)
	politeID := references.MerchantTextID(
		"POLITE_REAG_" + strconv.Itoa(deterministicReagentFarewellProseIndex(review.State.Merchant.ID, true)),
	)
	polite, err := fixture.state.GameReferences.MerchantReferences.FormatText(politeID, references.MerchantTextValues{
		OwnerName:    review.State.Merchant.OwnerName,
		ShopName:     review.State.Merchant.ShopName,
		LocationName: review.State.Merchant.Location.String(),
		TimeOfDay:    merchantTimeOfDay(fixture.state.DateTime.Hour),
	})
	require.NoError(t, err)
	require.Equal(t, quoteMerchantConversation(rude), prose.RudeFarewell)
	require.Equal(t, quoteMerchantConversation(polite), prose.PoliteFarewell)
	require.True(t, strings.HasPrefix(prose.Greeting, `"`))
	require.True(t, strings.HasSuffix(prose.Greeting, `"`))
	require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
}

func TestReagentPreviewConfirmedRequestIsPureAndKeepsExistingSession(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
	fixture.state.PartyState.Inventory.Gold.Set(500)
	fixture.state.PartyState.Inventory.Reagent.Set(references.SpiderSilk, 4)
	session, err := fixture.state.BeginMerchantSession(fixture.npc)
	require.NoError(t, err)
	review, err := fixture.state.ReviewReagentShopByMerchantID(references.MerchantReagentYew)
	require.NoError(t, err)
	beforeLocation := fixture.state.MapState.PlayerLocation
	beforeDate := fixture.state.DateTime
	beforeRNG := fixture.state.GetRNG().Calls()
	beforeInventory := fixture.state.PartyState.Inventory.LockerSnapshot()

	fixture.scene.startReagentMerchantPreviewWorkflow(review)
	shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	require.True(t, shop.IsPreview())
	require.Contains(t, shop.GetDetailText(), "PREVIEW")
	selectReagent(t, fixture, shop, references.SpiderSilk)
	callbacksBeforeRequest := *fixture.callbacks
	purchase := openMerchantModal[*widgets.ReagentPurchaseDialog](t, fixture, shop, widgets.ReagentShopAwaitingQuantity)
	require.True(t, purchase.IsPreview())
	pressMerchantDialogKey(fixture, ebiten.KeyRight)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ReagentShopBrowse
	})

	require.Equal(t, beforeLocation, fixture.state.MapState.PlayerLocation)
	require.Equal(t, beforeDate, fixture.state.DateTime)
	require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
	require.Equal(t, beforeInventory, fixture.state.PartyState.Inventory.LockerSnapshot())
	require.Equal(t, callbacksBeforeRequest, *fixture.callbacks)
	require.Contains(t, shop.GetReceiptText(), "no purchase was committed")
	_, err = fixture.state.GetReagentShopState(session)
	require.NoError(t, err, "Preview must not issue, supersede, or end a MerchantSession")

	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.Equal(t, 1, fixture.callbacks.BlockInputCalls)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls)
	fixture.state.EndMerchantSession(session)
}

func TestReagentTalkFinallyRemovesOnlyOwnedChildThenParent(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Yew, references.MerchantReagentYew)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
	ownedPurchase := openMerchantModal[*widgets.ReagentPurchaseDialog](t, fixture, ownedShop, widgets.ReagentShopAwaitingQuantity)
	review, err := fixture.state.ReviewReagentShopByMerchantID(references.MerchantReagentYew)
	require.NoError(t, err)
	prose, err := fixture.scene.resolveReagentShopProse(review.State)
	require.NoError(t, err)
	decoyShop := widgets.NewReagentShopDialog(widgets.ReagentShopDialogConfig{
		Keyboard:       fixture.scene.keyboard,
		MerchantSprite: fixture.scene.spriteSheet.GetSprite(review.MerchantSpriteIndex),
		State:          review.State,
		Prose:          prose,
	})
	decoyPurchase := widgets.NewReagentPurchaseDialog(widgets.ReagentPurchaseDialogConfig{
		Keyboard: fixture.scene.keyboard,
		Quote:    ownedPurchase.GetQuote(),
	})
	fixture.scene.dialogStack.PushModalDialog(decoyShop)
	fixture.scene.dialogStack.PushModalDialog(decoyPurchase)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedPurchase.IsDismissed())
	require.True(t, ownedShop.IsDismissed())
	require.False(t, decoyPurchase.IsDismissed())
	require.False(t, decoyShop.IsDismissed())
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, decoyPurchase, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestShipMerchantRouteOpensProductionShowroomAndOwnsOneManualMinute(t *testing.T) {
	fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)

	require.Equal(t, "Island Shipwrights", shop.GetShell().GetHeaderView().ShopIdentity)
	require.Equal(t, "Bantral", shop.GetShell().GetHeaderView().Owner)
	require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))
	require.NotNil(t, fixture.scene.spriteSheet.GetSprite(indexes.ShipNoSailsRight))
	require.NotNil(t, fixture.scene.spriteSheet.GetSprite(indexes.SkiffRight))

	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestShipFirstDeliveryDispatchReturnsToShowroom(t *testing.T) {
	for _, test := range []struct {
		name        string
		selectSkiff bool
		wantBerth   game_state.ShipBerthState
	}{
		{name: "frigate", wantBerth: game_state.ShipBerthPendingFrigate},
		{name: "standalone skiff", selectSkiff: true,
			wantBerth: game_state.ShipBerthPendingSkiff},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
			fixture.state.PartyState.Inventory.Gold.Set(2000)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
			if test.selectSkiff {
				pressMerchantDialogKey(fixture, ebiten.KeyRight)
			}
			goldBefore := fixture.state.PartyState.Inventory.Gold.Get()
			price := shop.GetSelectedQuote().Price

			completeShipPurchase(t, fixture, shop)
			dispatch := waitForMerchantDialog[*widgets.ShipDispatchDialog](t, fixture)
			require.Equal(t, widgets.ShipShopSuccessReceipt, shop.GetPhase())
			require.Equal(t, 2, fixture.scene.dialogStack.Count())
			require.Equal(t, goldBefore-uint16(price), fixture.state.PartyState.Inventory.Gold.Get())
			require.Equal(t, test.wantBerth, shop.GetState().Berth.State)
			require.True(t, shop.GetState().Travel.PendingDelivery)
			require.Equal(t, shop.GetState().Gold, shop.GetState().FrigateQuote.GoldBefore)
			require.Equal(t, shop.GetState().Gold, shop.GetState().SkiffQuote.GoldBefore)
			require.Contains(t, shop.GetReceiptText(), "It awaits thee in the harbour")
			require.Empty(t, fixture.callbacks.TimeAdvanced())

			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return shop.GetPhase() == widgets.ShipShopBrowse
			})
			require.True(t, dispatch.ShouldContinueShopping())
			require.Equal(t, 1, fixture.scene.dialogStack.Count())
			require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
			require.Empty(t, fixture.callbacks.TimeAdvanced())
			shop.Dismiss()
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
			require.Equal(t, references.Jhelom, fixture.state.MapState.PlayerLocation.Location)
			require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
			require.Zero(t, fixture.scene.dialogStack.Count())
		})
	}
}

func TestShipFirstDeliveryDispatchTravelCleansTalkBeforeEscort(t *testing.T) {
	fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
	fixture.state.PartyState.Inventory.Gold.Set(2000)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
	completeShipPurchase(t, fixture, shop)
	dispatch := waitForMerchantDialog[*widgets.ShipDispatchDialog](t, fixture)
	review := dispatch.GetState().Travel
	units := shipTalkOverworldUnits(t, fixture)
	unitCount := len(*units)
	frigate := units.GetVehicleAtPositionAndFloorOrNil(review.BerthPosition, references.MainFloor)
	require.NotNil(t, frigate)
	require.True(t, frigate.GetVehicleDetails().IsPendingDelivery())

	pressMerchantDialogKey(fixture, ebiten.KeyD)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.False(t, dispatch.HasPendingTravelIntent(), "workflow must consume dispatch travel once")
	require.Equal(t, []int{1, game_state.HarbourTravelTimeCostMinutes}, fixture.callbacks.TimeAdvanced())
	require.Equal(t, references.BritanniaUnderworld, fixture.state.MapState.PlayerLocation.Location)
	require.Equal(t, references.MainFloor, fixture.state.MapState.PlayerLocation.Floor)
	require.Equal(t, review.ArrivalPosition, fixture.state.MapState.PlayerLocation.Position)
	require.Equal(t, references.NoPartyVehicle, fixture.state.PartyVehicle.GetVehicleDetails().VehicleType)
	require.Len(t, *units, unitCount)
	require.Same(t, frigate, units.GetVehicleAtPositionAndFloorOrNil(review.BerthPosition, references.MainFloor))
	require.False(t, frigate.GetVehicleDetails().IsPendingDelivery())
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.NotEmpty(t, fixture.callbacks.MapFadeLevels)
	require.Zero(t, fixture.callbacks.MapFadeLevels[len(fixture.callbacks.MapFadeLevels)-1])
}

func TestShipPersistentHarbourTravelApprovalCancelThenTravel(t *testing.T) {
	fixture := newShipMerchantTalkFixture(t, references.Minoc, references.MerchantShipMinoc)
	frigate, err := fixture.state.PlaceVehicleAtBritanniaDock(
		references.FrigateVehicle,
		references.Minoc,
		false,
	)
	require.NoError(t, err)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
	require.Equal(t, game_state.ShipBerthMooredFrigate, shop.GetState().Berth.State)

	pressMerchantDialogKey(fixture, ebiten.KeyD)
	firstApproval := waitForMerchantDialog[*widgets.ShipTravelDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	require.True(t, firstApproval.IsCancelled())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ShipShopBrowse
	})
	require.Equal(t, references.Minoc, fixture.state.MapState.PlayerLocation.Location)
	require.Empty(t, fixture.callbacks.TimeAdvanced())

	pressMerchantDialogKey(fixture, ebiten.KeyD)
	approval := waitForMerchantDialog[*widgets.ShipTravelDialog](t, fixture)
	review := approval.GetIntent().Review
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.Equal(t, []int{1, game_state.HarbourTravelTimeCostMinutes}, fixture.callbacks.TimeAdvanced())
	require.Equal(t, references.BritanniaUnderworld, fixture.state.MapState.PlayerLocation.Location)
	require.Equal(t, review.ArrivalPosition, fixture.state.MapState.PlayerLocation.Position)
	require.Equal(t, references.NoPartyVehicle, fixture.state.PartyVehicle.GetVehicleDetails().VehicleType)
	require.Same(t, frigate, shipTalkOverworldUnits(t, fixture).GetVehicleAtPositionAndFloorOrNil(
		review.BerthPosition,
		references.MainFloor,
	))
}

func TestShipHarbourTravelStaleArrivalRefreshesBrowseWithoutExtraTime(t *testing.T) {
	fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
	_, err := fixture.state.PlaceVehicleAtBritanniaDock(references.FrigateVehicle, references.Jhelom, false)
	require.NoError(t, err)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyD)
	approval := waitForMerchantDialog[*widgets.ShipTravelDialog](t, fixture)
	review := approval.GetIntent().Review
	blocker := map_units.NewNPCFriendlyVehiceNewRef(
		references.CarpetVehicle,
		review.ArrivalPosition,
		references.MainFloor,
	)
	require.True(t, shipTalkOverworldUnits(t, fixture).AddVehicle(*blocker))

	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return shop.GetPhase() == widgets.ShipShopNonterminalRefusal
	})

	require.Equal(t, references.Jhelom, fixture.state.MapState.PlayerLocation.Location)
	require.Equal(t, game_state.HarbourTravelBlockArrivalOccupied, shop.GetState().Travel.BlockReason)
	require.Contains(t, shop.GetReceiptText(), "No travel time was charged")
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.Equal(t, widgets.ShipShopBrowse, shop.GetPhase())
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestShipPurchaseFundsStaleRefreshAndLoadedSkiffBoundaries(t *testing.T) {
	t.Run("insufficient funds is terminal", func(t *testing.T) {
		fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
		fixture.state.PartyState.Inventory.Gold.Set(1)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
		completeShipPurchase(t, fixture, shop)

		require.Equal(t, widgets.ShipShopFundsRefusal, shop.GetPhase())
		require.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Gold.Get())
		require.Empty(t, *shipTalkOverworldUnits(t, fixture))
		require.Contains(t, shop.GetReceiptText(), "party has 1")
		require.Empty(t, fixture.callbacks.TimeAdvanced())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	})

	t.Run("stale gold refreshes full state nonterminally", func(t *testing.T) {
		fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
		fixture.state.PartyState.Inventory.Gold.Set(2000)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
		purchase := openMerchantModal[*widgets.ShipPurchaseDialog](t, fixture, shop, widgets.ShipShopAwaitingPurchaseApproval)
		fixture.state.PartyState.Inventory.Gold.Set(1999)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return shop.GetPhase() == widgets.ShipShopNonterminalRefusal
		})

		require.False(t, purchase.HasPendingPurchaseRequest())
		require.Equal(t, uint16(1999), shop.GetState().Gold)
		require.Equal(t, uint16(1999), shop.GetState().FrigateQuote.GoldBefore)
		require.Equal(t, uint16(1999), shop.GetState().SkiffQuote.GoldBefore)
		require.Contains(t, shop.GetReceiptText(), "No frigate bought")
		require.Empty(t, fixture.callbacks.TimeAdvanced())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		require.Equal(t, widgets.ShipShopBrowse, shop.GetPhase())
		shop.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})

	t.Run("loading skiff uses normal receipt without dispatch", func(t *testing.T) {
		fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
		frigate, err := fixture.state.PlaceVehicleAtBritanniaDock(
			references.FrigateVehicle,
			references.Jhelom,
			false,
		)
		require.NoError(t, err)
		fixture.state.PartyState.Inventory.Gold.Set(1000)
		fixture.scene.StartTalkWorkflow(fixture.direction)
		shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyRight)
		require.Equal(t, references.SkiffVehicle, shop.GetSelectedVehicle())
		completeShipPurchase(t, fixture, shop)

		require.Equal(t, widgets.ShipShopBrowse, shop.GetPhase())
		require.Equal(t, 1, fixture.scene.dialogStack.Count())
		require.Same(t, shop, fixture.scene.dialogStack.PeekTopModalDialog())
		require.Equal(t, 3, frigate.GetVehicleDetails().GetSkiffQuantity())
		require.Contains(t, shop.GetReceiptText(), "The frigate now carries 3.")
		require.NotContains(t, shop.GetShell().GetFooterView().Guidance, "Continue shopping")
		shop.Dismiss()
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
	})
}

func TestShipPreviewPurchaseAndTravelStopWithoutGameplayMutation(t *testing.T) {
	t.Run("purchase", func(t *testing.T) {
		fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
		fixture.state.PartyState.Inventory.Gold.Set(2000)
		review, err := fixture.state.ReviewShipShopByMerchantID(references.MerchantShipJhelom)
		require.NoError(t, err)
		beforeLocation := fixture.state.MapState.PlayerLocation
		beforeDate := fixture.state.DateTime
		beforeGold := fixture.state.PartyState.Inventory.Gold.Get()
		beforeRNG := fixture.state.GetRNG().Calls()

		fixture.scene.startShipMerchantPreviewWorkflow(review)
		shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
		require.True(t, shop.IsPreview())
		completeShipPurchase(t, fixture, shop)

		require.Equal(t, widgets.ShipShopPreviewStopped, shop.GetPhase())
		require.Equal(t, beforeLocation, fixture.state.MapState.PlayerLocation)
		require.Equal(t, beforeDate, fixture.state.DateTime)
		require.Equal(t, beforeGold, fixture.state.PartyState.Inventory.Gold.Get())
		require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
		require.Empty(t, *shipTalkOverworldUnits(t, fixture))
		require.Empty(t, fixture.callbacks.TimeAdvanced())
		require.Equal(t, 1, fixture.scene.dialogStack.Count(), "Preview must not open dispatch")
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
		require.Empty(t, fixture.callbacks.TimeAdvanced())
	})

	t.Run("travel", func(t *testing.T) {
		fixture := newShipMerchantTalkFixture(t, references.Minoc, references.MerchantShipMinoc)
		frigate, err := fixture.state.PlaceVehicleAtBritanniaDock(
			references.FrigateVehicle,
			references.Minoc,
			true,
		)
		require.NoError(t, err)
		review, err := fixture.state.ReviewShipShopByMerchantID(references.MerchantShipMinoc)
		require.NoError(t, err)
		beforeLocation := fixture.state.MapState.PlayerLocation
		beforeDate := fixture.state.DateTime
		beforeRNG := fixture.state.GetRNG().Calls()

		fixture.scene.startShipMerchantPreviewWorkflow(review)
		shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyD)
		approval := waitForMerchantDialog[*widgets.ShipTravelDialog](t, fixture)
		require.True(t, approval.IsPreview())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return shop.GetPhase() == widgets.ShipShopPreviewStopped
		})

		require.Equal(t, beforeLocation, fixture.state.MapState.PlayerLocation)
		require.Equal(t, beforeDate, fixture.state.DateTime)
		require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
		require.True(t, frigate.GetVehicleDetails().IsPendingDelivery())
		require.Equal(t, references.NoPartyVehicle, fixture.state.PartyVehicle.GetVehicleDetails().VehicleType)
		require.Empty(t, fixture.callbacks.TimeAdvanced())
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			return !fixture.state.EventQueueManager.HasActiveWorkflows()
		})
	})
}

func TestNPCAutoTalkShipTravelPreservesNoSecondTalkMinute(t *testing.T) {
	fixture := newShipMerchantTalkFixture(t, references.Minoc, references.MerchantShipMinoc)
	_, err := fixture.state.PlaceVehicleAtBritanniaDock(references.FrigateVehicle, references.Minoc, false)
	require.NoError(t, err)
	fixture.npc.NPCReference.Schedule.Ai[1] = byte(references.Talk3)
	fixture.npc.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionTalk

	require.True(t, fixture.scene.processNPCAutoTalk())
	shop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyD)
	waitForMerchantDialog[*widgets.ShipTravelDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, shop.IsDismissed())
	require.Equal(t, []int{game_state.HarbourTravelTimeCostMinutes}, fixture.callbacks.TimeAdvanced())
}

func TestShipProseUsesDeterministicSourceFamiliesWithoutGameplayRNG(t *testing.T) {
	merchantIDs := []references.MerchantID{
		references.MerchantShipJhelom,
		references.MerchantShipMinoc,
		references.MerchantShipEastBritanny,
		references.MerchantShipBuccaneersDen,
	}
	fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
	fixture.state.SetRandomSeed(8675309)
	beforeRNG := fixture.state.GetRNG().Calls()

	for _, merchantID := range merchantIDs {
		review, err := fixture.state.ReviewShipShopByMerchantID(merchantID)
		require.NoError(t, err)
		prose, err := fixture.scene.resolveShipShopProse(review.State)
		require.NoError(t, err)
		again, err := fixture.scene.resolveShipShopProse(review.State)
		require.NoError(t, err)
		require.Equal(t, prose, again)

		values := references.MerchantTextValues{
			OwnerName:    review.State.Merchant.OwnerName,
			ShopName:     review.State.Merchant.ShopName,
			ItemName:     "vessel",
			Price:        review.State.FrigateQuote.Price,
			Quantity:     1,
			LocationName: review.State.Merchant.Location.String(),
			TimeOfDay:    merchantTimeOfDay(fixture.state.DateTime.Hour),
		}
		baseIndex := deterministicMerchantProseIndex(merchantID, 4)
		greeting, err := fixture.state.GameReferences.MerchantReferences.FormatText(
			references.MerchantTextID("SHIP_TXT_"+strconv.Itoa(baseIndex)), values,
		)
		require.NoError(t, err)
		rude, err := fixture.state.GameReferences.MerchantReferences.FormatText(
			references.MerchantTextID("RUDE_SHIP_"+strconv.Itoa((baseIndex+1)%4)), values,
		)
		require.NoError(t, err)
		polite, err := fixture.state.GameReferences.MerchantReferences.FormatText(
			references.MerchantTextID("POLITE_SHIP_"+strconv.Itoa((baseIndex+2)%4)), values,
		)
		require.NoError(t, err)
		require.Equal(t, quoteMerchantConversation(greeting), prose.dialog.Greeting)
		require.Equal(t, quoteMerchantConversation(rude), prose.dialog.Farewell)
		require.Equal(t, quoteMerchantConversation(polite), prose.politeFarewell)
	}
	require.Equal(t, beforeRNG, fixture.state.GetRNG().Calls())
}

func TestShipTalkFinallyRemovesOnlyOwnedPurchaseAndParent(t *testing.T) {
	fixture := newShipMerchantTalkFixture(t, references.Jhelom, references.MerchantShipJhelom)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)
	ownedPurchase := openMerchantModal[*widgets.ShipPurchaseDialog](t, fixture, ownedShop, widgets.ShipShopAwaitingPurchaseApproval)
	review, err := fixture.state.ReviewShipShopByMerchantID(references.MerchantShipJhelom)
	require.NoError(t, err)
	prose, err := fixture.scene.resolveShipShopProse(review.State)
	require.NoError(t, err)
	decoyShop := widgets.NewShipShopDialog(widgets.ShipShopDialogConfig{
		Keyboard:       fixture.scene.keyboard,
		MerchantSprite: fixture.scene.spriteSheet.GetSprite(review.MerchantSpriteIndex),
		FrigateSprite:  fixture.scene.spriteSheet.GetSprite(indexes.ShipNoSailsRight),
		SkiffSprite:    fixture.scene.spriteSheet.GetSprite(indexes.SkiffRight),
		State:          review.State,
		Prose:          prose.dialog,
	})
	decoyPurchase := widgets.NewShipPurchaseDialog(widgets.ShipPurchaseDialogConfig{
		Keyboard: fixture.scene.keyboard,
		Intent:   ownedPurchase.GetIntent(),
	})
	fixture.scene.dialogStack.PushModalDialog(decoyShop)
	fixture.scene.dialogStack.PushModalDialog(decoyPurchase)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedPurchase.IsDismissed())
	require.True(t, ownedShop.IsDismissed())
	require.False(t, decoyPurchase.IsDismissed())
	require.False(t, decoyShop.IsDismissed())
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, decoyPurchase, fixture.scene.dialogStack.PeekTopModalDialog())
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestInnMerchantRouteOpensProductionDialogWithDeterministicProse(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 2)
	fixture.state.SetRandomSeed(213)
	rngBefore := fixture.state.GetRNG().Calls()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)

	require.Equal(t, "The Wayfarer Inn", dialog.GetShell().GetHeaderView().ShopIdentity)
	require.Equal(t, "Donya", dialog.GetShell().GetHeaderView().Owner)
	require.True(t, strings.HasPrefix(dialog.GetLatestExchange(), `"`))
	require.True(t, strings.HasSuffix(dialog.GetLatestExchange(), `"`))
	require.Equal(t, rngBefore, fixture.state.GetRNG().Calls())
	require.False(t, hasLinearTalkDialog(&fixture.scene.dialogStack))
	require.NotContains(t, dialog.GetLatestExchange(), dialog.GetState().PricingCharacterName)

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestInnActionCancellationReturnsToSameModeWithoutMutation(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 2)
	before := fixture.state.CurrentPartyRosterSnapshot()
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyL)
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	action := openMerchantModal[*widgets.InnActionDialog](t, fixture, dialog, widgets.InnShopAwaitingApproval)
	require.Equal(t, widgets.InnActionLeave, action.GetIntent().Action)

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.InnShopBrowse
	})

	require.True(t, action.IsCancelled())
	require.Equal(t, widgets.InnShopLeave, dialog.GetMode())
	require.Equal(t, before, fixture.state.CurrentPartyRosterSnapshot())
	require.Equal(t, goldBefore, fixture.state.PartyState.Inventory.Gold.Get())
	require.Same(t, dialog, fixture.scene.dialogStack.PeekTopModalDialog())
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestInnRestNoVacancyBlocksBeforeApproval(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Paws, references.MerchantInnPaws)
	prepareInnTalkRoster(fixture, 1)
	fixture.state.PartyState.GetCharacter(8).PartyMap = party_state.PartyMapAtInn(references.Paws)
	fixture.state.PartyState.GetCharacter(9).PartyMap = party_state.PartyMapAtInn(references.Paws)
	before := fixture.state.CurrentPartyRosterSnapshot()
	dateBefore := fixture.state.DateTime

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	require.Equal(t, game_state.InnRestBlockNoVacancy, dialog.GetState().Rest.BlockReason)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)

	require.Equal(t, widgets.InnShopBrowse, dialog.GetPhase())
	require.Contains(t, strings.ToLower(dialog.GetInlineFeedback()), "no room available")
	require.Equal(t, 1, fixture.scene.dialogStack.Count())
	require.Equal(t, before, fixture.state.CurrentPartyRosterSnapshot())
	require.Equal(t, dateBefore, fixture.state.DateTime)
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestInnPickupDeadAndPartyFullBlockBeforeApproval(t *testing.T) {
	tests := []struct {
		name        string
		activeCount int
		prepare     func(*party_state.PlayerCharacter)
		wantReason  string
	}{
		{
			name:        "dead guest",
			activeCount: 1,
			prepare: func(guest *party_state.PlayerCharacter) {
				guest.SetHp(0)
				guest.SetStatus(party_state.Dead)
			},
			wantReason: "dead and remains stored",
		},
		{
			name:        "party full",
			activeCount: party_state.MaxActiveParty,
			prepare:     func(*party_state.PlayerCharacter) {},
			wantReason:  "active party is full",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
			prepareInnTalkRoster(fixture, test.activeCount)
			guest := fixture.state.PartyState.GetCharacter(8)
			guest.PartyMap = party_state.PartyMapAtInn(references.Britain)
			guest.MonthsAtInn = 1
			test.prepare(guest)
			before := fixture.state.CurrentPartyRosterSnapshot()
			rngBefore := fixture.state.GetRNG().Calls()

			fixture.scene.StartTalkWorkflow(fixture.direction)
			dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
			pressMerchantDialogKey(fixture, ebiten.KeyP)
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)

			require.Equal(t, widgets.InnShopBrowse, dialog.GetPhase())
			require.Contains(t, strings.ToLower(dialog.GetInlineFeedback()), test.wantReason)
			require.Equal(t, 1, fixture.scene.dialogStack.Count())
			require.Equal(t, before, fixture.state.CurrentPartyRosterSnapshot())
			require.Equal(t, rngBefore, fixture.state.GetRNG().Calls())
			dialog.Dismiss()
			pumpMerchantWorkflowUntil(t, fixture, func() bool {
				return !fixture.state.EventQueueManager.HasActiveWorkflows()
			})
		})
	}
}

func TestInnRestInsufficientFundsIsConfirmableExactTerminalRefusal(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 1)
	fixture.state.PartyState.Inventory.Gold.Set(0)
	fixture.state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectQuickness, TurnsRemaining: 12,
	}
	before := fixture.state.CurrentPartyRosterSnapshot()
	dateBefore := fixture.state.DateTime
	positionBefore := fixture.state.MapState.PlayerLocation
	rngBefore := fixture.state.GetRNG().Calls()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	require.False(t, dialog.GetState().Rest.Affordable)
	completeInnAction(t, fixture, dialog)
	overlay := waitForMerchantOverlay(t, fixture)

	require.True(t, dialog.IsDismissed())
	require.Equal(t, `"Highwaymen! Cheap, at that! OUT!" screams Donya.`, dialog.GetLatestExchange())
	require.Contains(t, overlay.GetMessage(), dialog.GetLatestExchange())
	require.Contains(t, overlay.GetMessage(), "Thou couldst not stay the night.")
	require.Contains(t, overlay.GetMessage(), "none was paid")
	require.NotContains(t, overlay.GetMessage(), "Faulinei")
	require.NotContains(t, overlay.GetMessage(), "restored")
	require.Equal(t, before, fixture.state.CurrentPartyRosterSnapshot())
	require.Equal(t, positionBefore, fixture.state.MapState.PlayerLocation)
	require.Equal(t, dateBefore, fixture.state.DateTime)
	require.Equal(t, rngBefore, fixture.state.GetRNG().Calls())
	require.Equal(t, game_state.DurationEffectQuickness, fixture.state.ActiveDurationEffect.EffectType)
	require.False(t, fixture.state.EventQueueManager.HasFeedback("inn_rest_feedback"))

	overlay.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestInnRestSuccessUsesAuthorityReceiptThenTerminalFeedback(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 1)
	fixture.state.PartyState.Inventory.Gold.Set(100)
	fixture.state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectTimeStop, TurnsRemaining: 20,
	}
	avatar := fixture.state.PartyState.GetAvatar()
	avatar.SetHp(1)
	goldBefore := fixture.state.PartyState.Inventory.Gold.Get()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	quote := dialog.GetState().Rest
	completeInnAction(t, fixture, dialog)
	require.NotEmpty(t, fixture.callbacks.MapFadeLevels)
	require.Equal(t, 1.0, fixture.callbacks.MapFadeLevels[0], "the committed wake location must remain covered")
	pumpInnRestFeedbackUntilDone(t, fixture)
	overlay := waitForMerchantOverlay(t, fixture)

	require.True(t, dialog.IsDismissed(), "Inn UI must close before terminal presentation")
	require.Equal(t, quote.WakeLocation, fixture.state.MapState.PlayerLocation)
	require.Equal(t, quote.DateTimeAfter, fixture.state.DateTime)
	require.Equal(t, game_state.DurationEffectNone, fixture.state.ActiveDurationEffect.EffectType)
	require.Equal(t, avatar.GetMaxHp(), avatar.GetCurrentHp())
	require.Equal(t, goldBefore-quote.Charge, fixture.state.PartyState.Inventory.Gold.Get())
	require.Contains(t, dialog.GetReceiptText(), "Thou dost rest until morning.")
	require.Contains(t, dialog.GetReceiptText(), "restored.")
	require.Contains(t, dialog.GetReceiptText(), fmt.Sprintf("Paid %d gold.", quote.Charge))
	require.NotContains(t, dialog.GetReceiptText(), "Faulinei")
	require.NotContains(t, dialog.GetReceiptText(), "missing")
	require.NotContains(t, dialog.GetReceiptText(), "rooms")
	require.NotContains(t, dialog.GetReceiptText(), "party 1/6")
	require.Contains(t, overlay.GetMessage(), dialog.GetReceiptText())
	require.False(t, fixture.state.EventQueueManager.HasFeedback("inn_rest_feedback"))
	authorityDate := fixture.state.DateTime

	overlay.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	authorityDate.Advance(1)
	require.Equal(t, authorityDate, fixture.state.DateTime, "Talk cleanup owns exactly one authoritative minute")
	require.Contains(t, fixture.callbacks.SoundEffectsPlayed, game_state.SoundGoodNight)
	require.Contains(t, fixture.callbacks.SoundEffectsPlayed, game_state.SoundRoosterCrow)
	require.Zero(t, fixture.callbacks.MapFadeLevels[len(fixture.callbacks.MapFadeLevels)-1])
}

func TestInnLeaveAndPickupRefreshSameModesWithExactTheftReceipt(t *testing.T) {
	const seed = 240211
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 2)
	fixture.state.PartyState.Inventory.Gold.Set(1000)
	fixture.state.ShadowlordState.AutomaticPresence = references.Faulinei
	fixture.state.SetRandomSeed(seed)
	leftName := fixture.state.PartyState.GetCharacter(1).GetNameAsString()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyL)
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	completeInnAction(t, fixture, dialog)

	require.Equal(t, widgets.InnShopLeave, dialog.GetMode())
	require.Equal(t, widgets.InnShopBrowse, dialog.GetPhase())
	require.Equal(t, 1, dialog.GetState().PartySize)
	require.Equal(t, 1, dialog.GetState().RoomOccupancy)
	require.Equal(t, leftName+" will stay at the inn. No charge today.", dialog.GetReceiptText())
	require.Equal(t, uint16(1000), fixture.state.PartyState.Inventory.Gold.Get())

	pressMerchantDialogKey(fixture, ebiten.KeyP)
	require.Equal(t, leftName, dialog.GetPickupRows()[0].Identity.Name)
	charge := dialog.GetPickupRows()[0].Charge
	wantTheft := uint16(beradrng.New(seed).Intn(64) + 1)
	completeInnAction(t, fixture, dialog)

	wantGold := uint16(1000) - charge - wantTheft
	require.Equal(t, widgets.InnShopPickup, dialog.GetMode())
	require.Equal(t, 2, dialog.GetState().PartySize)
	require.Zero(t, dialog.GetState().RoomOccupancy)
	require.Equal(t, wantGold, fixture.state.PartyState.Inventory.Gold.Get())
	require.Contains(t, dialog.GetReceiptText(), leftName+" has been returned to the party.")
	require.Contains(t, dialog.GetReceiptText(), fmt.Sprintf("Paid %d gold.", charge))
	require.Contains(t, dialog.GetReceiptText(), fmt.Sprintf("%d more gold was stolen!", wantTheft))
	require.Contains(t, dialog.GetReceiptText(), fmt.Sprintf("Thou hast %d gold left.", wantGold))
	require.NotContains(t, dialog.GetReceiptText(), "Faulinei")
	require.True(t, fixture.state.EventQueueManager.HasFeedback("inn_pickup_feedback"))

	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestInnPickupGuardRefusalIsTerminalWithoutPaymentOrRosterChange(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 1)
	guest := fixture.state.PartyState.GetCharacter(8)
	guest.PartyMap = party_state.PartyMapAtInn(references.Britain)
	guest.MonthsAtInn = 3
	fixture.state.PartyState.Inventory.Gold.Set(1)
	fixture.state.ShadowlordState.AutomaticPresence = references.Faulinei
	before := fixture.state.CurrentPartyRosterSnapshot()
	rngBefore := fixture.state.GetRNG().Calls()

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyP)
	row := dialog.GetPickupRows()[0]
	require.False(t, row.Affordable)
	require.True(t, row.Confirmable)
	completeInnAction(t, fixture, dialog)
	overlay := waitForMerchantOverlay(t, fixture)

	require.True(t, dialog.IsDismissed())
	require.Contains(t, overlay.GetMessage(), dialog.GetLatestExchange())
	require.Contains(t, dialog.GetReceiptText(), fmt.Sprintf("Thou hast need of %d gold; none was paid.", row.Charge))
	require.Contains(t, dialog.GetReceiptText(), guest.GetNameAsString()+" remains at the inn.")
	require.Contains(t, dialog.GetReceiptText(), "The guards have been called.")
	require.NotContains(t, dialog.GetReceiptText(), "Faulinei")
	require.NotContains(t, dialog.GetReceiptText(), "restored")
	require.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, before, fixture.state.CurrentPartyRosterSnapshot())
	require.Same(t, guest, fixture.state.PartyState.GetCharacter(8))
	require.Greater(t, fixture.state.GetRNG().Calls(), rngBefore)
	require.False(t, fixture.state.EventQueueManager.HasFeedback("inn_pickup_feedback"))

	overlay.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestInnPickupPoisonDeathIsDisclosedAndRemainsInPickupMode(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 1)
	guest := fixture.state.PartyState.GetCharacter(8)
	guest.PartyMap = party_state.PartyMapAtInn(references.Britain)
	guest.MonthsAtInn = 1
	guest.SetStatus(party_state.Poisoned)
	guest.SetHp(20)
	fixture.state.PartyState.Inventory.Gold.Set(500)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyP)
	completeInnAction(t, fixture, dialog)

	require.Equal(t, party_state.Dead, guest.GetStatus())
	require.Zero(t, guest.GetCurrentHp())
	require.Equal(t, widgets.InnShopPickup, dialog.GetMode())
	require.Contains(t, dialog.GetLatestExchange(), guest.GetNameAsString()+" has died")
	require.Contains(t, dialog.GetReceiptText(), "Paid ")
	require.NotContains(t, dialog.GetReceiptText(), "restored")
	require.False(t, dialog.IsDismissed())
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
}

func TestInnRestAllDeadResultDisclosesDeathBeforeCleanHandoff(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 1)
	avatar := fixture.state.PartyState.GetAvatar()
	avatar.SetStatus(party_state.Poisoned)
	avatar.SetHp(20)
	fixture.state.PartyState.Inventory.Gold.Set(500)
	dateBefore := fixture.state.DateTime

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	completeInnAction(t, fixture, dialog)
	pumpInnRestFeedbackUntilDone(t, fixture)
	overlay := waitForMerchantOverlay(t, fixture)

	require.True(t, fixture.state.PartyState.IsAllPartyDead())
	require.True(t, dialog.IsDismissed())
	require.Contains(t, overlay.GetMessage(), avatar.GetNameAsString()+" passed away during the night")
	require.Contains(t, overlay.GetMessage(), "Paid ")
	require.NotContains(t, overlay.GetMessage(), "restored")
	require.NotContains(t, overlay.GetMessage(), "party 1/6")
	require.False(t, fixture.state.EventQueueManager.HasFeedback("inn_rest_feedback"))
	require.NotEqual(t, dateBefore, fixture.state.DateTime, "Inn authority must complete its overnight time")

	overlay.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return fixture.state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName)
	})
	require.Zero(t, fixture.scene.dialogStack.Count())
}

func TestInnAllDeadPartyCanRemainAndPickupLivingGuest(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 2)
	fixture.state.PartyState.GetCharacter(0).SetStatus(party_state.Dead)
	fixture.state.PartyState.GetCharacter(0).SetHp(0)
	guest := fixture.state.PartyState.GetCharacter(8)
	guest.PartyMap = party_state.PartyMapAtInn(references.Britain)
	guest.MonthsAtInn = 1
	guest.SetStatus(party_state.Good)
	fixture.state.PartyState.Inventory.Gold.Set(1000)

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyL)
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	completeInnAction(t, fixture, dialog)

	require.True(t, fixture.state.PartyState.IsAllPartyDead())
	require.False(t, dialog.IsDismissed())
	require.True(t, fixture.state.EventQueueManager.HasActiveWorkflows())
	pressMerchantDialogKey(fixture, ebiten.KeyP)
	completeInnAction(t, fixture, dialog)

	require.False(t, fixture.state.PartyState.IsAllPartyDead())
	require.Equal(t, widgets.InnShopPickup, dialog.GetMode())
	require.False(t, fixture.state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName))
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestInnAllDeadTerminalExitCleansUpBeforeDeathHandoffAndSkipsTalkMinute(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 2)
	fixture.state.PartyState.GetCharacter(0).SetStatus(party_state.Dead)
	fixture.state.PartyState.GetCharacter(0).SetHp(0)
	dateBefore := fixture.state.DateTime

	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyL)
	pressMerchantDialogKey(fixture, ebiten.KeyDown)
	completeInnAction(t, fixture, dialog)
	require.True(t, fixture.state.PartyState.IsAllPartyDead())

	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return fixture.state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName)
	})

	require.True(t, dialog.IsDismissed())
	require.Zero(t, fixture.scene.dialogStack.Count())
	require.Empty(t, fixture.callbacks.TimeAdvanced())
	require.Equal(t, dateBefore, fixture.state.DateTime)
	require.Equal(t, 1, fixture.callbacks.UnblockInputCalls, "Talk cleanup must run before death takes input")
}

func TestInnTalkFinallyRemovesOnlyOwnedActionAndParent(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantInnBritain)
	prepareInnTalkRoster(fixture, 2)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	ownedShop := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)
	ownedAction := openMerchantModal[*widgets.InnActionDialog](t, fixture, ownedShop, widgets.InnShopAwaitingApproval)
	review, err := fixture.state.ReviewInnShopByMerchantID(references.MerchantInnBritain)
	require.NoError(t, err)
	prose, err := fixture.scene.resolveInnShopProse(review.State)
	require.NoError(t, err)
	decoyShop := widgets.NewInnShopDialog(widgets.InnShopDialogConfig{
		Keyboard:       fixture.scene.keyboard,
		MerchantSprite: fixture.scene.spriteSheet.GetSprite(review.MerchantSpriteIndex),
		State:          review.State,
		Prose:          prose.dialog,
	})
	decoyAction := widgets.NewInnActionDialog(widgets.InnActionDialogConfig{
		Keyboard: fixture.scene.keyboard,
		Intent:   ownedAction.GetIntent(),
	})
	fixture.scene.dialogStack.PushModalDialog(decoyShop)
	fixture.scene.dialogStack.PushModalDialog(decoyAction)

	ownedShop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return !fixture.state.EventQueueManager.HasActiveWorkflows()
	})

	require.True(t, ownedAction.IsDismissed())
	require.True(t, ownedShop.IsDismissed())
	require.False(t, decoyAction.IsDismissed())
	require.False(t, decoyShop.IsDismissed())
	require.Equal(t, 2, fixture.scene.dialogStack.Count())
	require.Same(t, decoyAction, fixture.scene.dialogStack.PeekTopModalDialog())
}

type merchantTalkFixture struct {
	state     *game_state.GameState
	callbacks *game_state.MockSystemCallbacks
	scene     *GameScene
	npc       *map_units.NPCFriendly
	direction references.Direction
}

func newMerchantTalkFixture(
	t *testing.T,
	location references.Location,
	merchantID references.MerchantID,
) *merchantTalkFixture {
	t.Helper()

	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	state.ActiveDurationEffect = game_state.DurationEffect{}
	definition, found := state.GameReferences.MerchantReferences.Definition(merchantID)
	require.True(t, found)
	require.Equal(t, location, definition.Location)

	var npc *map_units.NPCFriendly
	for _, mapUnit := range *state.CurrentNPCAIController.GetNpcs() {
		candidate, ok := mapUnit.(*map_units.NPCFriendly)
		if ok && candidate.MapUnitDetails().NPCNum == definition.NPCIndex {
			npc = candidate

			break
		}
	}
	require.NotNil(t, npc)
	npc.PlaceAtScheduleSlot(1)
	npc.SetVisible(true)
	setMerchantTalkDesiredSlot(t, state, npc, 1)

	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapUnrecruited
		character.SetStatus(party_state.Dead)
	}
	avatar := state.PartyState.GetCharacter(0)
	avatar.PartyMap = party_state.PartyMapActive
	avatar.SetStatus(party_state.Good)
	avatar.Intelligence = 10
	state.PartyState.Inventory.Gold.Set(9999)
	state.PartyState.Inventory.Provisions.Keys.Set(10)
	state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()

	direction := placePlayerNextToMerchant(state, npc)

	// Simulated controls still consult held keys for modifiers and navigation.
	// Keep that source detached from the frame-advancing native test harness.
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false })
	scene := &GameScene{
		gameState: state,
		keyboard:  keyboard,
		// Authority/workflow fixtures use detached text input. Native field
		// ownership and save-editor handoff have their own widget regressions.
		appendInputChars: func(chars []rune) []rune { return chars },
		// Merchant tests need addressable tile bounds, not the production border cache.
		spriteSheet: &sprites.SpriteSheet{SpriteImage: ebiten.NewImage(512, 256)},
	}

	callbacks.BindGameState(state)
	return &merchantTalkFixture{
		state: state, callbacks: callbacks, scene: scene, npc: npc, direction: direction,
	}
}

func newShipMerchantTalkFixture(
	t *testing.T,
	location references.Location,
	merchantID references.MerchantID,
) *merchantTalkFixture {
	t.Helper()
	fixture := newMerchantTalkFixture(t, location, merchantID)
	shipTalkOverworldUnits(t, fixture).ClearAllMapUnits()

	return fixture
}

func shipTalkOverworldUnits(t *testing.T, fixture *merchantTalkFixture) *map_units.MapUnits {
	t.Helper()
	controller := fixture.state.LargeMapNPCAIController[references.OVERWORLD]
	require.NotNil(t, controller)
	require.NotNil(t, controller.GetNpcs())

	return controller.GetNpcs()
}

func placePlayerNextToMerchant(state *game_state.GameState, npc *map_units.NPCFriendly) references.Direction {
	playerPosition := npc.Pos()
	direction := references.Right
	if playerPosition.X > 0 {
		playerPosition.X--
	} else {
		playerPosition.X++
		direction = references.Left
	}
	state.MapState.PlayerLocation.Position = playerPosition
	state.MapState.PlayerLocation.Floor = npc.Floor()

	return direction
}

func setMerchantTalkDesiredSlot(
	t *testing.T,
	state *game_state.GameState,
	npc *map_units.NPCFriendly,
	wanted references.NPCScheduleSlot,
) {
	t.Helper()

	for hour := range 24 {
		candidate := state.DateTime
		candidate.Hour = byte(hour)
		if npc.NPCReference.Schedule.DesiredSlot(candidate) == wanted {
			state.DateTime = candidate

			return
		}
	}
	t.Fatalf("merchant schedule has no hour for desired slot %d", wanted)
}

func waitForMerchantOverlay(t *testing.T, fixture *merchantTalkFixture) *widgets.OverlayDialog {
	t.Helper()

	var overlay *widgets.OverlayDialog
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		candidate, ok := fixture.scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
		if ok {
			overlay = candidate
		}

		return ok
	})
	require.NotNil(t, overlay)

	return overlay
}

func waitForConcreteMerchantDialog(
	t *testing.T,
	fixture *merchantTalkFixture,
	family references.MerchantFamily,
) (*widgets.MerchantDialogShell, func()) {
	t.Helper()

	if family == references.MerchantFamilyGuild {
		dialog := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyArms {
		dialog := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyHealer {
		dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyReagent {
		dialog := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyHorse {
		dialog := waitForMerchantDialog[*widgets.HorseShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyShip {
		dialog := waitForMerchantDialog[*widgets.ShipShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyInn {
		dialog := waitForMerchantDialog[*widgets.InnShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	if family == references.MerchantFamilyTavern {
		dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)

		return dialog.GetShell(), dialog.Dismiss
	}
	t.Fatalf("no merchant dialog for family %v", family)

	return nil, nil
}

func hasLinearTalkDialog(stack *widgets.DialogStack) bool {
	for _, widget := range stack.Dialogs() {
		if _, ok := widget.(*LinearTalkDialog); ok {
			return true
		}
	}

	return false
}

func completeGuildPurchase(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.GuildShopDialog,
	bundleCount uint16,
) *widgets.GuildPurchaseDialog {
	t.Helper()
	require.NotZero(t, bundleCount)

	purchaseDialog := openMerchantModal[*widgets.GuildPurchaseDialog](t, fixture, dialog, widgets.GuildShopAwaitingQuantity)
	for selected := uint16(1); selected < bundleCount; selected++ {
		pressMerchantDialogKey(fixture, ebiten.KeyRight)
	}
	require.Equal(t, bundleCount, purchaseDialog.GetBundleCount())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.True(t, purchaseDialog.IsDismissed())
	require.True(t, purchaseDialog.HasPendingPurchaseRequest())

	// A further update without a fresh key must not produce another activation.
	fixture.scene.updateDialogs()
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.GuildShopBrowse
	})

	return purchaseDialog
}

func completeTavernAction(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.TavernShopDialog,
	key ebiten.Key,
) *widgets.TavernActionDialog {
	t.Helper()
	action := openMerchantModal[*widgets.TavernActionDialog](t, fixture, dialog, widgets.TavernShopAwaitingApproval)
	pressMerchantDialogKey(fixture, key)
	require.True(t, action.IsDismissed())
	if !action.IsPreview() && (action.GetIntent().Action == widgets.TavernActionMeal ||
		action.GetIntent().Action == widgets.TavernActionProvision) {
		result := waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
		require.True(t, result.IsDismissed())
		pumpMerchantWorkflowUntil(t, fixture, func() bool {
			for _, widget := range fixture.scene.dialogStack.Dialogs() {
				if widget == result {
					return false
				}
			}

			return true
		})
	}
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.IsDismissed() || dialog.GetPhase() == widgets.TavernShopBrowse ||
			dialog.GetPhase() == widgets.TavernShopSessionError
	})

	return action
}

func completeInnAction(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.InnShopDialog,
) *widgets.InnActionDialog {
	t.Helper()
	action := openMerchantModal[*widgets.InnActionDialog](t, fixture, dialog, widgets.InnShopAwaitingApproval)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.True(t, action.IsDismissed())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.IsDismissed() || dialog.GetPhase() == widgets.InnShopBrowse ||
			dialog.GetPhase() == widgets.InnShopSessionError
	})

	return action
}

func completeHealerService(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.HealerShopDialog,
) {
	t.Helper()
	if dialog.GetPhase() == widgets.HealerShopBrowse {
		if dialog.GetFocus() == widgets.HealerShopServices {
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			require.Equal(t, widgets.HealerShopRecipients, dialog.GetFocus())
		}
		pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	}
	require.Equal(t, widgets.HealerShopAwaitingApproval, dialog.GetPhase())
	treatmentDialog := waitForMerchantDialog[*widgets.HealerTreatmentDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.True(t, treatmentDialog.IsDismissed())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.HealerShopBrowse
	})
}

func selectReagent(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.ReagentShopDialog,
	reagent references.Reagent,
) {
	t.Helper()
	for range len(dialog.GetCatalogQuotes()) {
		if dialog.GetSelectedReagent() == reagent {
			return
		}
		pressMerchantDialogKey(fixture, ebiten.KeyDown)
	}
	t.Fatalf("reagent %d is not present in the shop catalog", reagent)
}

func completeReagentPurchase(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.ReagentShopDialog,
	bundleCount uint16,
) *widgets.ReagentPurchaseDialog {
	t.Helper()
	require.NotZero(t, bundleCount)

	purchaseDialog := openMerchantModal[*widgets.ReagentPurchaseDialog](t, fixture, dialog, widgets.ReagentShopAwaitingQuantity)
	for selected := uint16(1); selected < bundleCount; selected++ {
		pressMerchantDialogKey(fixture, ebiten.KeyRight)
	}
	require.Equal(t, bundleCount, purchaseDialog.GetBundleCount())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.True(t, purchaseDialog.IsDismissed())
	require.True(t, purchaseDialog.HasPendingPurchaseRequest())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.GetPhase() == widgets.ReagentShopBrowse ||
			dialog.GetPhase() == widgets.ReagentShopSessionError
	})

	return purchaseDialog
}

func completeHorsePurchase(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.HorseShopDialog,
) *widgets.HorsePurchaseDialog {
	t.Helper()

	purchaseDialog := openMerchantModal[*widgets.HorsePurchaseDialog](t, fixture, dialog, widgets.HorseShopAwaitingApproval)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.True(t, purchaseDialog.IsDismissed())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.IsTerminal()
	})

	return purchaseDialog
}

func completeShipPurchase(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.ShipShopDialog,
) *widgets.ShipPurchaseDialog {
	t.Helper()
	purchaseDialog := openMerchantModal[*widgets.ShipPurchaseDialog](t, fixture, dialog, widgets.ShipShopAwaitingPurchaseApproval)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.True(t, purchaseDialog.IsDismissed())
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		switch dialog.GetPhase() {
		case widgets.ShipShopSuccessReceipt, widgets.ShipShopNonterminalRefusal,
			widgets.ShipShopFundsRefusal, widgets.ShipShopPreviewStopped, widgets.ShipShopSessionError:
			return true
		case widgets.ShipShopBrowse:
			return dialog.GetReceiptText() != ""
		case widgets.ShipShopAwaitingPurchaseApproval,
			widgets.ShipShopAwaitingPurchaseResult, widgets.ShipShopAwaitingTravelApproval,
			widgets.ShipShopAwaitingTravelResult, widgets.ShipShopFarewell:
			return false
		default:
			return false
		}
	})

	return purchaseDialog
}

func requireReagentDialogMatchesAuthority(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.ReagentShopDialog,
	merchantID references.MerchantID,
) {
	t.Helper()
	review, err := fixture.state.ReviewReagentShopByMerchantID(merchantID)
	require.NoError(t, err)
	require.Equal(t, review.State.Offers, dialog.GetCatalogQuotes())
	require.Equal(t, strconv.Itoa(int(review.State.Gold)), dialog.GetShell().GetHeaderView().PartyGold)
}

func selectArmsEquipment(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.ArmsShopDialog,
	mode widgets.ArmsShopMode,
	equipment references.Equipment,
) {
	t.Helper()
	if mode == widgets.ArmsShopBuy {
		pressMerchantDialogKey(fixture, ebiten.KeyB)
	} else {
		pressMerchantDialogKey(fixture, ebiten.KeyS)
	}
	for range len(dialog.GetRows()) {
		if dialog.GetSelectedEquipment() == equipment {
			return
		}
		pressMerchantDialogKey(fixture, ebiten.KeyDown)
	}
	t.Fatalf("%s is not present in Arms %s rows", equipment, mode)
}

func completeArmsTrade(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.ArmsShopDialog,
	quantity uint16,
) *widgets.ArmsTradeDialog {
	t.Helper()
	require.NotZero(t, quantity)
	tradeDialog := openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, dialog, widgets.ArmsShopAwaitingTrade)
	for selected := uint16(1); selected < quantity; selected++ {
		pressMerchantDialogKey(fixture, ebiten.KeyRight)
	}
	require.Equal(t, quantity, tradeDialog.GetQuantity())
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return dialog.IsDismissed() || dialog.GetPhase() == widgets.ArmsShopBrowse ||
			dialog.GetPhase() == widgets.ArmsShopSessionError
	})

	return tradeDialog
}

func armsBuyRow(
	t *testing.T,
	dialog *widgets.ArmsShopDialog,
	equipment references.Equipment,
) widgets.ArmsShopRowView {
	t.Helper()
	require.Equal(t, widgets.ArmsShopBuy, dialog.GetMode())
	for _, row := range dialog.GetRows() {
		if row.Equipment == equipment {
			return row
		}
	}
	t.Fatalf("%s is not present in Arms Buy rows", equipment)

	return widgets.ArmsShopRowView{}
}

func armsSellRow(
	t *testing.T,
	dialog *widgets.ArmsShopDialog,
	equipment references.Equipment,
) widgets.ArmsShopRowView {
	t.Helper()
	require.Equal(t, widgets.ArmsShopSell, dialog.GetMode())
	for _, row := range dialog.GetRows() {
		if row.Equipment == equipment {
			return row
		}
	}
	t.Fatalf("%s is not present in Arms Sell rows", equipment)

	return widgets.ArmsShopRowView{}
}

func pressMerchantDialogKey(fixture *merchantTalkFixture, key ebiten.Key) {
	fixture.scene.keyboard.SimulateKeyPress(key)
	fixture.scene.updateDialogs()
}

func selectTavernOrderItem(
	t *testing.T,
	fixture *merchantTalkFixture,
	dialog *widgets.TavernShopDialog,
	item widgets.TavernOrderItem,
) {
	t.Helper()
	rows := dialog.GetOrderRows()
	for range len(rows) {
		if rows[dialog.GetSelectedOrderIndex()].Item == item {
			return
		}
		pressMerchantDialogKey(fixture, ebiten.KeyDown)
	}
	t.Fatalf("Tavern order item %d is not available", item)
}

type merchantRuneQueue struct {
	frames [][]rune
}

func newMerchantRuneQueue() *merchantRuneQueue { return &merchantRuneQueue{} }

func (q *merchantRuneQueue) Push(value string) {
	q.frames = append(q.frames, []rune(value))
}

func (q *merchantRuneQueue) Append(destination []rune) []rune {
	if len(q.frames) == 0 {
		return destination
	}
	frame := q.frames[0]
	q.frames = q.frames[1:]

	return append(destination, frame...)
}

func prepareInnTalkRoster(fixture *merchantTalkFixture, activeCount int) {
	for index := range party_state.NRosterSlots {
		character := fixture.state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapUnrecruited
		character.MonthsAtInn = 0xff
		character.SetStatus(party_state.Good)
		character.SetMaxHp(90)
		character.SetHp(40)
		character.SetMp(3)
		character.Intelligence = 10
	}
	for index := range activeCount {
		fixture.state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapActive
	}
}

func setHorseTalkCandidateTiles(
	t *testing.T,
	fixture *merchantTalkFixture,
	tiles [4]indexes.SpriteIndex,
) {
	t.Helper()

	layeredMap := fixture.state.MapState.LayeredMaps.GetLayeredMap(
		references.SmallMapType,
		fixture.state.MapState.PlayerLocation.Floor,
	)
	require.NotNil(t, layeredMap)
	player := fixture.state.MapState.PlayerLocation.Position
	positions := [4]references.Position{
		{X: player.X, Y: player.Y + 1},
		{X: player.X, Y: player.Y - 1},
		{X: player.X + 1, Y: player.Y},
		{X: player.X - 1, Y: player.Y},
	}
	for index, position := range positions {
		layeredMap.SetTileByLayer(map_state.MapLayer, &position, tiles[index])
		layeredMap.UnSetTileByLayer(map_state.MapOverrideLayer, &position)
	}
}

func countHorseTalkVehicles(units *map_units.MapUnits) int {
	count := 0
	for _, unit := range *units {
		friendly, ok := unit.(*map_units.NPCFriendly)
		if ok && friendly.GetVehicleDetails().VehicleType == references.HorseVehicle {
			count++
		}
	}

	return count
}

// openMerchantModal presses Enter on a shop, checks the shop reached phase,
// and returns the modal of type M it opened.
func openMerchantModal[M any, P comparable](
	t *testing.T,
	fixture *merchantTalkFixture,
	shop interface{ GetPhase() P },
	phase P,
) M {
	t.Helper()
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	require.Equal(t, phase, shop.GetPhase())

	return waitForMerchantDialog[M](t, fixture)
}

// waitForMerchantDialog pumps the merchant workflow until a dialog of type D is
// on the stack and returns it.
func waitForMerchantDialog[D any](t *testing.T, fixture *merchantTalkFixture) D {
	t.Helper()

	var dialog D
	found := false
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		for _, widget := range fixture.scene.dialogStack.Dialogs() {
			if candidate, ok := widget.(D); ok {
				dialog, found = candidate, true

				return true
			}
		}

		return false
	})
	require.True(t, found)

	return dialog
}

func pumpMerchantWorkflowUntil(
	t *testing.T,
	fixture *merchantTalkFixture,
	done func() bool,
) {
	t.Helper()

	for range 500 {
		if done() {
			return
		}
		fixture.state.ElapsedMs += merchantDialogPollingRateMs
		fixture.state.EventQueueManager.Update()
	}
	t.Fatal("merchant Talk workflow did not reach expected state")
}

func pumpMerchantFeedbackUntilDone(t *testing.T, fixture *merchantTalkFixture, name string) {
	t.Helper()

	for range 500 {
		if !fixture.state.EventQueueManager.HasFeedback(name) {
			return
		}
		fixture.state.ElapsedMs += merchantDialogPollingRateMs
		fixture.state.EventQueueManager.UpdateFeedback()
	}
	t.Fatalf("merchant feedback %q did not complete", name)
}

func pumpInnRestFeedbackUntilDone(t *testing.T, fixture *merchantTalkFixture) {
	t.Helper()

	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return fixture.state.EventQueueManager.HasFeedback("inn_rest_feedback")
	})
	pumpMerchantFeedbackUntilDone(t, fixture, "inn_rest_feedback")
}
