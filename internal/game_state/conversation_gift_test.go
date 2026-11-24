package game_state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestGiveConversationItemOutcomes(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	inventory := &state.PartyState.Inventory

	gold := inventory.Gold.Get()
	name, outcome := state.GiveConversationItem(ConversationProvisionFirst + ConversationGiftGold)
	require.Equal(t, "Gold", name)
	require.Equal(t, GiftGranted, outcome)
	require.Equal(t, gold+1, inventory.Gold.Get())

	inventory.Provisions.Keys.Set(99)
	name, outcome = state.GiveConversationItem(ConversationProvisionFirst + ConversationGiftKeys)
	require.Equal(t, "Keys", name)
	require.Equal(t, GiftAtLimit, outcome)
	require.Equal(t, uint16(99), inventory.Provisions.Keys.Get())

	_, outcome = state.GiveConversationItem(ConversationProvisionFirst + ConversationGiftSkullKeys + 1)
	require.Equal(t, GiftUnknown, outcome)
}
