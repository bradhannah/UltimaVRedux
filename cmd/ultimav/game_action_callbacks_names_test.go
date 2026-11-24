package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestConversationGiftsReportActualFullEquipmentIdentities(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	callbacks := NewGameActionCallbacks(&GameScene{gameState: state}, references.NPCReference{}, nil)
	output := text.NewOutput(nil, 1, 64, 100)
	callbacks.SetDialogOutput(output)
	for equipment, name := range map[references.Equipment]string{
		references.ChaosSword: "Sword of Chaos", references.TwoHHammer: "Two-Handed Hammer",
		references.JewelShield: "Jewel Shield", references.JeweledSword: "Jeweled Sword",
		references.MysticArmour: "Mystic Armour", references.RingRegeneration: "Ring of Regeneration",
	} {
		state.PartyState.Inventory.Equipment.Set(equipment, 0)
		output.Reset()
		callbacks.GiveItem(equipment.ID())
		require.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(equipment))
		require.Equal(t, "Received: "+name, strings.TrimSpace(output.GetOutputStr(false)))
	}
	for _, test := range []struct {
		id   int
		item references.QuestItem
		name string
	}{{70, references.Grapple, "Grapple"}, {71, references.Carpet, "Magic Carpet"}} {
		state.PartyState.Inventory.QuestItems.Set(test.item, 0)
		output.Reset()
		callbacks.GiveItem(test.id)
		require.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(test.item))
		require.Equal(t, "Received: "+test.name, strings.TrimSpace(output.GetOutputStr(false)))
	}
}

func TestConversationGiftAtItsLimitSaysNothingWasReceived(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	callbacks := NewGameActionCallbacks(&GameScene{gameState: state}, references.NPCReference{}, nil)
	output := text.NewOutput(nil, 1, 64, 100)
	callbacks.SetDialogOutput(output)
	keys := &state.PartyState.Inventory.Provisions.Keys
	keys.Set(99)

	callbacks.GiveItem(game_state.ConversationProvisionFirst + game_state.ConversationGiftKeys)

	require.Equal(t, uint16(99), keys.Get())
	require.Equal(t, "Thou canst carry no more Keys!", strings.TrimSpace(output.GetOutputStr(false)))
}
