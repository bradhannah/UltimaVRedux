package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func TestHealerAllReviewsPaidThenSeparateCharity(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.SkaraBrae, references.MerchantHealerSkaraBrae)
	fixture.state.PartyState.Inventory.Gold.Set(50)
	for index := range 2 {
		character := fixture.state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapActive
		character.SetMaxHp(60)
		character.SetHp(10)
		character.SetStatus(party_state.Good)
	}
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyH)
	pressMerchantDialogKey(fixture, ebiten.KeyA)
	approval := waitForMerchantDialog[*widgets.HealerTreatmentDialog](t, fixture)
	require.Contains(t, approval.GetBodyText(), "-45")
	require.Equal(t, uint16(50), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, uint16(10), fixture.state.PartyState.GetCharacter(0).GetCurrentHp())
	completeHealerService(t, fixture, dialog)
	require.Equal(t, uint16(5), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, uint16(60), fixture.state.PartyState.GetCharacter(0).GetCurrentHp())
	require.Equal(t, uint16(10), fixture.state.PartyState.GetCharacter(1).GetCurrentHp())
	pressMerchantDialogKey(fixture, ebiten.KeyA)
	charity := waitForMerchantDialog[*widgets.HealerTreatmentDialog](t, fixture)
	require.NotContains(t, charity.GetBodyText(), "45")
	require.Contains(t, charity.GetBodyText(), "No payment")
	pressMerchantDialogKey(fixture, ebiten.KeyEscape)
	pumpMerchantWorkflowUntil(t, fixture, func() bool { return dialog.GetPhase() == widgets.HealerShopBrowse })
	require.Equal(t, uint16(10), fixture.state.PartyState.GetCharacter(1).GetCurrentHp())
	pressMerchantDialogKey(fixture, ebiten.KeyA)
	completeHealerService(t, fixture, dialog)
	require.Equal(t, uint16(5), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, uint16(60), fixture.state.PartyState.GetCharacter(1).GetCurrentHp())
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.state.EventQueueManager.HasActiveWorkflows() })
	require.Equal(t, []int{1}, fixture.callbacks.TimeAdvanced())
}

func TestHealerAllRejectsChangedReviewBeforeCharityMutation(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.SkaraBrae, references.MerchantHealerSkaraBrae)
	fixture.state.PartyState.Inventory.Gold.Set(50)
	character := fixture.state.PartyState.GetCharacter(0)
	character.SetMaxHp(60)
	character.SetHp(10)
	character.SetStatus(party_state.Good)
	fixture.scene.StartTalkWorkflow(fixture.direction)
	dialog := waitForMerchantDialog[*widgets.HealerShopDialog](t, fixture)
	pressMerchantDialogKey(fixture, ebiten.KeyH)
	pressMerchantDialogKey(fixture, ebiten.KeyA)
	waitForMerchantDialog[*widgets.HealerTreatmentDialog](t, fixture)
	fixture.state.PartyState.Inventory.Gold.Set(5)
	calls := fixture.state.GetRNG().Calls()
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool { return dialog.GetPhase() == widgets.HealerShopSessionError })
	require.Equal(t, uint16(5), fixture.state.PartyState.Inventory.Gold.Get())
	require.Equal(t, uint16(10), character.GetCurrentHp())
	require.Equal(t, calls, fixture.state.GetRNG().Calls())
	dialog.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.state.EventQueueManager.HasActiveWorkflows() })
}

func TestArmsOriginalPartialBatchReceipt(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	gameplay := fixture.state.GameConfig.GameplayConfiguration
	before := gameplay.BatchMerchantTheft
	gameplay.BatchMerchantTheft = false
	t.Cleanup(func() { gameplay.BatchMerchantTheft = before })
	fixture.state.SetRandomSeed(208)
	fixture.state.ShadowlordState.AutomaticPresence = references.Faulinei
	fixture.state.PartyState.GetCharacter(0).Intelligence = 10
	fixture.state.PartyState.Inventory.Equipment.Set(references.Bow, 0)
	price := game_state.CalculateMerchantBuyPrice(references.Bow.GetBasePrice(), 10)
	fixture.state.PartyState.Inventory.Gold.Set(uint16(price * 2))
	fixture.scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Bow)
	require.Equal(t, price, armsBuyRow(t, shop, references.Bow).Value)
	completeArmsTrade(t, fixture, shop, 2)
	require.Equal(t, uint16(1), fixture.state.PartyState.Inventory.Equipment.Get(references.Bow))
	require.Contains(t, shop.GetReceiptText(), "Batch stopped: 1 of 2 purchased.")
	require.Contains(t, shop.GetReceiptText(), fmt.Sprintf("for %d gold", price))
	shop.Dismiss()
	pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.state.EventQueueManager.HasActiveWorkflows() })
}

func TestGuildAndReagentOriginalPartialBatchReceipts(t *testing.T) {
	for _, guild := range []bool{true, false} {
		t.Run(fmt.Sprintf("guild=%t", guild), func(t *testing.T) {
			location, merchant := references.Yew, references.MerchantReagentYew
			if guild {
				location, merchant = references.NewMagincia, references.MerchantGuildNewMagincia
			}
			fixture := newMerchantTalkFixture(t, location, merchant)
			gameplay := fixture.state.GameConfig.GameplayConfiguration
			before := gameplay.BatchMerchantTheft
			gameplay.BatchMerchantTheft = false
			t.Cleanup(func() { gameplay.BatchMerchantTheft = before })
			fixture.state.SetRandomSeed(208)
			fixture.state.ShadowlordState.AutomaticPresence = references.Faulinei
			fixture.state.PartyState.Inventory.Gold.Set(9999)
			if guild {
				review, err := fixture.state.ReviewGuildShopByMerchantID(merchant)
				require.NoError(t, err)
				price := review.State.Offers[0].PricePerBundle
				fixture.state.PartyState.Inventory.Gold.Set(uint16(price * 2))
				fixture.state.PartyState.Inventory.Provisions.Keys.Set(0)
				fixture.scene.StartTalkWorkflow(fixture.direction)
				shop := waitForMerchantDialog[*widgets.GuildShopDialog](t, fixture)
				completeGuildPurchase(t, fixture, shop, 2)
				require.Equal(t, uint16(3), fixture.state.PartyState.Inventory.Provisions.Keys.Get())
				require.Contains(t, shop.GetReceiptText(), "Batch stopped: 1 of 2 bundles purchased.")
				require.Contains(t, shop.GetReceiptText(), fmt.Sprintf("for %d gold", price))
				shop.Dismiss()
			} else {
				review, err := fixture.state.ReviewReagentShopByMerchantID(merchant)
				require.NoError(t, err)
				quote := review.State.Offers[0]
				fixture.state.PartyState.Inventory.Gold.Set(uint16(quote.PricePerBundle * 2))
				fixture.state.PartyState.Inventory.Reagent.Set(quote.Reagent, 0)
				fixture.scene.StartTalkWorkflow(fixture.direction)
				shop := waitForMerchantDialog[*widgets.ReagentShopDialog](t, fixture)
				completeReagentPurchase(t, fixture, shop, 2)
				require.Equal(t, quote.BundleQuantity, fixture.state.PartyState.Inventory.Reagent.Get(quote.Reagent))
				require.Contains(t, shop.GetReceiptText(), "Batch stopped: 1 of 2 bundles purchased.")
				require.Contains(t, shop.GetReceiptText(), fmt.Sprintf("Paid %d gold", quote.PricePerBundle))
				shop.Dismiss()
			}
			pumpMerchantWorkflowUntil(t, fixture, func() bool { return !fixture.state.EventQueueManager.HasActiveWorkflows() })
		})
	}
}
