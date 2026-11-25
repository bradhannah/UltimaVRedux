package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCombatCollectionAccountsActualGainsThroughDeparture(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	combat := state.CombatScene
	actor := combat.GetPartyMemberPosition(combat.GetActiveCombatantIndex())
	position := *references.Down.GetNewPositionInDirection(&actor)
	date, rng := state.DateTime, state.GetRNG().Calls()
	state.PartyState.Inventory.Provisions.Gems.Set(98)
	require.True(t, state.addCombatLooseItem(position, references.Gem, 5))
	require.Equal(t, uint16(1), state.GetCombatItem(references.Down).Acquired)
	// Spending collected stock must not reduce the gross collection total.
	state.PartyState.Inventory.Provisions.Gems.Set(90)
	combat.Victory = true
	require.True(t, state.addCombatLooseItem(position, references.Gem, 2))
	require.Equal(t, uint16(2), state.GetCombatItem(references.Down).Acquired)
	require.True(t, state.addCombatLooseItem(position, references.Gold, 38))
	require.Equal(t, uint16(38), state.GetCombatItem(references.Down).Acquired)
	require.Len(t, combat.collectedItems, 2)
	assert.Equal(t, uint64(3), combat.collectedItems[0].Quantity)
	assert.Equal(t, uint64(38), combat.collectedItems[1].Quantity)
	assert.Equal(t, date, state.DateTime)
	assert.Equal(t, rng, state.GetRNG().Calls())
	assert.Empty(t, callbacks.CombatConclusions, "victory alone does not finalize acquisitions")
	require.Len(t, callbacks.CombatEvents, 3)
	for _, event := range callbacks.CombatEvents {
		assert.Equal(t, CombatEventPickup, event.Kind)
		assert.NotEmpty(t, event.Pickup.ActorName)
		assert.Equal(t, position, event.Pickup.Position)
	}
	state.EndCombat(CombatVictory)
	require.Nil(t, state.CombatScene)
	require.Len(t, callbacks.CombatConclusions, 1)
	conclusion := callbacks.CombatConclusions[0]
	assert.Equal(t, combat.collectedItems, conclusion.Collected)
	combat.collectedItems[0].Quantity = 0
	assert.Equal(t, uint64(3), conclusion.Collected[0].Quantity, "handoff owns a detached snapshot")
	state.EndCombat(CombatVictory)
	assert.Len(t, callbacks.CombatConclusions, 1)
}

func TestCombatCollectionRefusalsAndUniqueItems(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	actor := state.CombatScene.GetPartyMemberPosition(state.CombatScene.GetActiveCombatantIndex())
	position := *references.Down.GetNewPositionInDirection(&actor)
	assert.False(t, state.GetCombatItem(references.Down).Handled)
	state.PartyState.Inventory.Provisions.Gems.Set(99)
	require.True(t, state.addCombatLooseItem(position, references.Gem, 2))
	assert.Zero(t, state.GetCombatItem(references.Down).Acquired)
	require.True(t, state.addCombatLooseItem(position, references.Crown, 1))
	state.PartyState.Inventory.QuestItems.SetHasOne(references.Crown)
	assert.Zero(t, state.GetCombatItem(references.Down).Acquired)
	require.True(t, state.CombatScene.AddObject(NewCombatChest(position, 10, false)))
	assert.Equal(t, "Open it first!", state.GetCombatItem(references.Down).Message)
	assert.Empty(t, state.CombatScene.collectedItems)
	require.Len(t, callbacks.CombatEvents, 4)
	for _, event := range callbacks.CombatEvents {
		assert.Zero(t, event.Pickup.Result.Acquired)
		assert.NotEmpty(t, event.Pickup.Result.Message)
	}
	state.CombatScene.takeFirstObjectAt(position, CombatObject.IsChest)
	state.PartyState.Inventory.QuestItems.Set(references.Crown, 0)
	require.True(t, state.addCombatLooseItem(position, references.Crown, 1))
	require.Equal(t, uint16(1), state.GetCombatItem(references.Down).Acquired)
	require.Len(t, state.CombatScene.collectedItems, 1)
	assert.Equal(t, references.ItemType(references.ItemTypeQuestItem), state.CombatScene.collectedItems[0].ItemType)
	assert.Equal(t, indexes.SpriteIndex(indexes.Crown), state.CombatScene.collectedItems[0].Sprite)
}

func TestCombatChestTrapPublishesActualConsequence(t *testing.T) {
	for _, kind := range []TrapType{TrapAcid, TrapPoison} {
		state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
		require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
		index := state.CombatScene.GetActiveCombatantIndex()
		character := state.PartyState.GetCharacter(index)
		character.SetHp(1)
		character.SetStatus(party_state.Good)
		state.SetRandomSeed(firstCombatTrapSeed(state, kind))
		result := state.triggerCombatChestTrap(index)
		assert.Equal(t, kind, result.Type)
		assert.Equal(t, character.GetNameAsString(), result.ActorName)
		assert.Equal(t, uint16(1)-character.GetCurrentHp(), result.HPLost)
		assert.Equal(t, character.IsDead(), result.Fatal)
		require.Len(t, callbacks.CombatEvents, 1)
		assert.Equal(t, CombatEventChestTrap, callbacks.CombatEvents[0].Kind)
		assert.Equal(t, result, callbacks.CombatEvents[0].ChestTrap)
	}
}

func TestGetAllCombatItemsClearsTheSquareAndOpensOnlyAChestFromAChest(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(42).Build()
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	combat := state.CombatScene
	actor := combat.GetPartyMemberPosition(combat.GetActiveCombatantIndex())
	position := *references.Down.GetNewPositionInDirection(&actor)
	gems, gold := state.PartyState.Inventory.Provisions.Gems.Get(), state.PartyState.Inventory.Gold.Get()
	require.True(t, state.addCombatLooseItem(position, references.Gem, 2))
	require.True(t, state.admitCombatChestLoot(position, references.ChestLoot{ChestValue: 10}))
	require.True(t, state.addCombatLooseItem(position, references.Gold, 38))

	state.GetAllCombatItems(references.Down)

	_, left := combat.firstGettableObjectAt(position)
	require.False(t, left, "the loose items, the inner chest and its loot are all gone")
	assert.Equal(t, gems+2, state.PartyState.Inventory.Provisions.Gems.Get())
	assert.GreaterOrEqual(t, state.PartyState.Inventory.Gold.Get(), gold+38)
	assert.Equal(t, 1, callbacks.PlaybackRequestCount(SoundItemPickup, SoundPlaybackOneShot), "the lot sounds once")
	for _, event := range callbacks.CombatEvents {
		assert.Equal(t, CombatEventPickup, event.Kind)
		assert.True(t, event.Pickup.Result.Handled, "an emptied square is not asked for more")
	}

	// A chest worth 1 holds nothing; opening it is the whole outcome.
	require.True(t, state.admitCombatChestLoot(position, references.ChestLoot{ChestValue: 1}))
	assert.True(t, state.GetAllCombatItems(references.Down).Handled)
	assert.True(t, callbacks.MessagesContain("Chest empty!"))
	assert.False(t, callbacks.MessagesContain("Nothing to get!"))

	// A chest of any other origin keeps its trap for the player to face.
	require.True(t, combat.AddObject(NewCombatChest(position, 10, true)))
	assert.Equal(t, "Open it first!", state.GetAllCombatItems(references.Down).Message)
	chest, found := combat.firstGettableObjectAt(position)
	require.True(t, found)
	assert.True(t, chest.IsLockedAndTrapped())
}
