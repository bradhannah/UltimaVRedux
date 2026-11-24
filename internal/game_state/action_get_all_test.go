package game_state

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestGetAllTakesTheWholePileAndLeavesAChestAmongItClosed(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	place := state.MapState.PlayerLocation
	position := *references.Right.GetNewPositionInDirection(&place.Position)
	for _, item := range []references.ItemAndQuantity{{Item: references.Gem, Quantity: 2}, {Item: references.Gold, Quantity: 30}} {
		require.NoError(t, state.ItemStacksMap.PushUnder(&position, &references.ItemStack{Items: references.ItemStacks{item}}))
	}
	chest := state.CurrentNPCAIController.GetNpcs().InsertTransientSurfaceChest(place.Location, position, place.Floor, 10)
	require.NotNil(t, chest)
	inventory := &state.PartyState.Inventory
	gems, gold, karma := inventory.Provisions.Gems.Get(), inventory.Gold.Get(), state.PartyState.Karma.Value
	callbacks.BindGameState(state)

	require.True(t, state.ActionGetAllSmallMap(references.Right))

	assert.Equal(t, gems+2, inventory.Provisions.Gems.Get())
	assert.Equal(t, gold+30, inventory.Gold.Get())
	assert.False(t, state.ItemStacksMap.HasItemStackAtPosition(&position))
	assert.Same(t, chest, state.surfaceChestAt(&position), "a chest among the loot stays closed")
	assert.Equal(t, karma, state.PartyState.Karma.Value)
	require.Len(t, callbacks.SearchFeedbackMessages, 1, "one card names the lot")
	assert.Len(t, strings.Split(callbacks.SearchFeedbackMessages[0], "\n"), 2)
	assert.Equal(t, 1, callbacks.PlaybackRequestCount(SoundItemPickup, SoundPlaybackOneShot), "the lot sounds once")
	callbacks.AssertTimeAdvanced(2)
}
