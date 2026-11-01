package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Issue 56: wall torches in dungeon rooms must be collectible in combat so the
// party can stand in the sconce square and push the wall behind it.
func TestCombatGetBorrowsWallTorchWithoutAddingStock(t *testing.T) {
	for _, sconce := range []indexes.SpriteIndex{indexes.LeftSconce, indexes.RightSconce} {
		fixture := newCombatPushFixture(t, sconce, references.Up)
		state, callbacks := fixture.state, fixture.callbacks
		state.PartyState.Inventory.Provisions.Torches.Set(3)
		state.MapState.Lighting.LightTorch()
		callbacks.Reset()

		result := state.GetCombatItem(references.Up)

		require.True(t, result.Handled)
		assert.True(t, result.Succeeded)
		assert.Zero(t, result.Acquired)
		assert.Equal(t, "Borrowed!", result.Message)
		assertCombatPushTerrain(t, state, fixture.object, indexes.BrickFloor)
		assert.Equal(t, uint16(3), state.PartyState.Inventory.Provisions.Torches.Get(),
			"a borrowed wall torch is carried lit, not pocketed")
		assert.Equal(t, map_state.BorrowedWallTorchDurationMinutes, state.MapState.Lighting.TorchMinutesRemaining(),
			"the wall torch replaces, not extends, carried fuel")
		assert.Equal(t, []string{result.Message}, callbacks.Messages)
		assert.Empty(t, state.CombatScene.collectedItems, "nothing enters the encounter loot tally")
		require.Len(t, callbacks.CombatEvents, 1)
		assert.Equal(t, CombatEventPickup, callbacks.CombatEvents[0].Kind)
		assert.True(t, callbacks.CombatEvents[0].Pickup.Result.Succeeded)
	}
}

func TestCombatGetTableFoodFollowsReachAndKarmaRule(t *testing.T) {
	t.Run("reachable plate", func(t *testing.T) {
		fixture := newCombatPushFixture(t, indexes.TableFoodBoth, references.Down)
		state := fixture.state
		food, karma := state.PartyState.Inventory.Provisions.Food.Get(), state.PartyState.Karma.Value
		fixture.callbacks.Reset()

		result := state.GetCombatItem(references.Down)

		require.True(t, result.Handled)
		assert.True(t, result.Succeeded)
		assert.Equal(t, uint16(1), result.Acquired)
		assert.Equal(t, "Mmmmm...! But that food isn't yours!", result.Message)
		assertCombatPushTerrain(t, state, fixture.object, indexes.TableFoodBottom)
		assert.Equal(t, food+1, state.PartyState.Inventory.Provisions.Food.Get())
		assert.Equal(t, karma-1, state.PartyState.Karma.Value)
		require.Len(t, state.CombatScene.collectedItems, 1)
		assert.Equal(t, uint64(1), state.CombatScene.collectedItems[0].Quantity)
	})

	t.Run("unreachable plate", func(t *testing.T) {
		fixture := newCombatPushFixture(t, indexes.TableFoodBoth, references.Right)
		state := fixture.state
		food, karma := state.PartyState.Inventory.Provisions.Food.Get(), state.PartyState.Karma.Value
		fixture.callbacks.Reset()

		result := state.GetCombatItem(references.Right)

		require.True(t, result.Handled)
		assert.False(t, result.Succeeded)
		assert.Equal(t, "Can't reach plate!", result.Message)
		assertCombatPushTerrain(t, state, fixture.object, indexes.TableFoodBoth)
		assert.Equal(t, food, state.PartyState.Inventory.Provisions.Food.Get())
		assert.Equal(t, karma, state.PartyState.Karma.Value)
	})
}

func TestCombatGetIgnoresNonCollectibleTerrain(t *testing.T) {
	fixture := newCombatPushFixture(t, indexes.Brazier, references.Up)
	fixture.callbacks.Reset()

	result := fixture.state.GetCombatItem(references.Up)

	assert.False(t, result.Handled)
	assert.False(t, result.Succeeded)
	assert.Equal(t, "Nothing to get!", result.Message)
	assertCombatPushTerrain(t, fixture.state, fixture.object, indexes.Brazier)
}

func TestActionGetSmallMapBorrowsWallTorchWithoutAddingStock(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	state.PartyState.Inventory.Provisions.Torches.Set(2)
	target := references.Up.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	require.NotNil(t, target)
	theMap := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, state.MapState.PlayerLocation.Floor)
	theMap.SetTileByLayer(map_state.MapLayer, target, indexes.RightSconce)
	callbacks.Reset()

	require.True(t, state.ActionGetSmallMap(references.Up))

	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), theMap.SpriteIndexByLayer(map_state.MapLayer, *target))
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Torches.Get())
	// The town command then advances one minute, which burns one of the 100.
	assert.Equal(t, map_state.BorrowedWallTorchDurationMinutes-1, state.MapState.Lighting.TorchMinutesRemaining())
	assert.Contains(t, callbacks.Messages, "Borrowed!")
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
}

func TestActionJimmyCombatMapRestraintOutcomes(t *testing.T) {
	tests := []struct {
		name         string
		restraint    indexes.SpriteIndex
		keys         uint16
		succeeds     bool
		expectedTile indexes.SpriteIndex
		expectedKeys uint16
		message      string
	}{
		{"manacles unlock", indexes.Manacles, 2, true, indexes.BrickFloor, 2, "Unlocked! The empty restraint falls open."},
		{"stocks unlock", indexes.Stocks, 1, true, indexes.BrickFloor, 1, "Unlocked! The empty restraint falls open."},
		{"key breaks", indexes.Manacles, 2, false, indexes.Manacles, 1, "Key broke!"},
		{"no keys", indexes.Stocks, 0, true, indexes.Stocks, 0, "No keys!"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCombatPushFixture(t, test.restraint, references.Left)
			state := fixture.state
			state.PartyState.Inventory.Provisions.Keys.Set(test.keys)
			state.jimmySuccessForTesting = func(*party_state.PlayerCharacter) bool { return test.succeeds }
			fixture.callbacks.Reset()

			handled, chest := state.ActionJimmyCombatMapWithDisarm(references.Left)

			assert.True(t, handled)
			assert.Nil(t, chest)
			assertCombatPushTerrain(t, state, fixture.object, test.expectedTile)
			assert.Equal(t, test.expectedKeys, state.PartyState.Inventory.Provisions.Keys.Get())
			assert.Equal(t, []string{test.message}, fixture.callbacks.Messages)
		})
	}
}

func TestActionOpenCombatMapAnswersFixedTerrain(t *testing.T) {
	tests := []struct {
		name    string
		terrain indexes.SpriteIndex
		message string
	}{
		{"footlocker is already open", indexes.Box, "It's open!"},
		{"portcullis is too heavy", indexes.Portcullis, "Too heavy!"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := newCombatPushFixture(t, test.terrain, references.Right)
			fixture.callbacks.Reset()

			result := fixture.state.ActionOpenCombatMap(references.Right)

			assert.True(t, result.Handled)
			assert.False(t, result.OpenedChest)
			assertCombatPushTerrain(t, fixture.state, fixture.object, test.terrain)
			assert.Equal(t, []string{test.message}, fixture.callbacks.Messages)
		})
	}
}

// TestCovetousRoomTwelveTorchSquareOpensHiddenPassage walks the sequence from
// public issue 56 on the real room: take the top-left wall torch, step into its
// square and push the wall west of it, which fires the room's hidden-passage
// trigger. Before the fix the first step answered "Nothing to get!".
func TestCovetousRoomTwelveTorchSquareOpensHiddenPassage(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	require.NoError(t, state.TransitionToDungeonCombatMap(references.CovetousDungeon, 12, CombatEntryNorth))
	identity := state.CombatScene.GetCurrentTurnIdentity()
	for index := range state.CombatScene.PartyActive {
		state.CombatScene.PartyActive[index] = index == identity.EntityIndex
	}
	clearCombatMonstersForTest(state.CombatScene)
	sconce, wall, standing := references.Position{X: 3, Y: 2}, references.Position{X: 2, Y: 2}, references.Position{X: 4, Y: 2}
	require.Equal(t, indexes.SpriteIndex(indexes.LeftSconce), state.CombatScene.CombatMapRef.GetTileNumber(sconce))
	require.Equal(t, indexes.SpriteIndex(indexes.StoneBrickWall), state.CombatScene.CombatMapRef.GetTileNumber(wall))
	state.CombatScene.SetPartyMemberPosition(identity.EntityIndex, standing)
	callbacks.Reset()

	pickup := state.GetCombatItem(references.Left)
	require.True(t, pickup.Succeeded, pickup.Message)
	assertCombatPushTerrain(t, state, sconce, indexes.BrickFloor)

	state.CombatScene.SetPartyMemberPosition(identity.EntityIndex, sconce)
	require.True(t, state.ActionPushCombatMap(references.Left))
	assert.NotEqual(t, indexes.SpriteIndex(indexes.StoneBrickWall), state.CombatScene.CombatMapRef.GetTileNumber(wall),
		"pushing the wall from the torch square must open the hidden passage")
	assert.NotContains(t, callbacks.Messages, "Won't budge!")
}
