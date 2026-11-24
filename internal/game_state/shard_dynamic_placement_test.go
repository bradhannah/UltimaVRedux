package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/stretchr/testify/assert"
)

// TestShardOverlaySystem_PositionDetection tests that shard positions are correctly detected
func TestShardOverlaySystem_PositionDetection(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(-1).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	shardPos := references.Position{X: 192, Y: 80} // Shard of Falsehood location

	// Test that shard position is correctly detected
	hasShardHere, questItem := gs.IsShardAtPosition(shardPos)
	assert.True(t, hasShardHere, "Shard position should be detected")
	assert.Equal(t, references.ShardOfFalsehood, questItem, "Correct quest item should be identified")

	t.Logf("Overlay system position detection working: shard detected at (%d,%d)", shardPos.X, shardPos.Y)
}

// TestShardOverlaySystem_MultiplePositions tests that all shard positions are correctly detected
func TestShardOverlaySystem_MultiplePositions(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(-1).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	shardPositions := []struct {
		pos       references.Position
		name      string
		questItem references.QuestItem
	}{
		{references.Position{X: 192, Y: 80}, "Falsehood", references.ShardOfFalsehood},
		{references.Position{X: 130, Y: 65}, "Hatred", references.ShardOfHatred},
		{references.Position{X: 176, Y: 184}, "Cowardice", references.ShardOfCowardice},
	}

	// Test all shard positions are correctly detected by overlay system
	for _, shard := range shardPositions {
		hasShardHere, questItem := gs.IsShardAtPosition(shard.pos)
		assert.True(t, hasShardHere, "Shard %s should be detected at position", shard.name)
		assert.Equal(t, shard.questItem, questItem, "Correct quest item should be identified for shard %s", shard.name)

		t.Logf("Shard %s overlay position detected at (%d,%d)", shard.name, shard.pos.X, shard.pos.Y)
	}
}

// TestShardOverlaySystem_InventoryVisibility tests overlay visibility logic with inventory
func TestShardOverlaySystem_InventoryVisibility(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(-1).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	shardPos := references.Position{X: 192, Y: 80} // Shard of Falsehood location
	layeredMap := gs.GetLayeredMapByCurrentLocation()

	// Add shard to inventory (simulates collection)
	gs.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)

	// Base tile exists (we don't care what it is since overlays handle visual rendering)
	tile := layeredMap.GetTopTile(&shardPos)
	assert.NotNil(t, tile, "Base tile should exist")

	// Test that overlay visibility logic works correctly
	hasShardHere, questItem := gs.IsShardAtPosition(shardPos)
	assert.True(t, hasShardHere, "Shard position should be recognized")
	assert.Equal(t, references.ShardOfFalsehood, questItem, "Correct quest item should be identified")

	isVisible := gs.IsShardVisible(shardPos, questItem)
	assert.False(t, isVisible, "Shard should not be visible when in inventory")

	t.Logf("Overlay visibility logic working: shard hidden when in inventory at (%d,%d)", shardPos.X, shardPos.Y)
}

// TestShardOverlaySystem_LocationRestriction tests that shards only appear on underworld
func TestShardOverlaySystem_LocationRestriction(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain). // Not underworld
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()

	// Test that shard positions are only valid in underworld
	shardPos := references.Position{X: 192, Y: 80} // Shard of Falsehood location
	hasShardHere, _ := gs.IsShardAtPosition(shardPos)
	assert.False(t, hasShardHere, "Shards should not be detected outside underworld")

	// Should not affect the current map (Britain)
	t.Logf("Overlay system correctly restricted to underworld, current location: %v", gs.MapState.PlayerLocation.Location)
}

// TestShardOverlaySystem_Integration tests the complete overlay system integration
func TestShardOverlaySystem_Integration(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(-1).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	shardPos := references.Position{X: 192, Y: 80}
	layeredMap := gs.GetLayeredMapByCurrentLocation()
	tile := layeredMap.GetTopTile(&shardPos)

	// Test base tile exists (overlay system handles visual rendering)
	assert.NotNil(t, tile, "Base tile should exist")

	// Test overlay logic integration
	hasShardHere, questItem := gs.IsShardAtPosition(shardPos)
	assert.True(t, hasShardHere, "Shard position should be recognized")
	assert.Equal(t, references.ShardOfFalsehood, questItem, "Correct quest item should be identified")

	// Test visibility when not in inventory
	isVisible := gs.IsShardVisible(shardPos, questItem)
	assert.True(t, isVisible, "Shard should be visible when not in inventory")

	t.Logf("Overlay system integration working: shard visible at (%d,%d)", shardPos.X, shardPos.Y)
}

func TestShardVisibilityUsesInventoryAndMatchingShadowlordLife(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.Basement).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()

	shards := []struct {
		name string
		item references.QuestItem
		lord references.Shadowlord
		pos  references.Position
	}{
		{name: "Falsehood", item: references.ShardOfFalsehood, lord: references.Faulinei, pos: references.Position{X: 192, Y: 80}},
		{name: "Hatred", item: references.ShardOfHatred, lord: references.Astaroth, pos: references.Position{X: 130, Y: 65}},
		{name: "Cowardice", item: references.ShardOfCowardice, lord: references.Nosfentor, pos: references.Position{X: 176, Y: 184}},
	}
	for _, shard := range shards {
		gs.PartyState.Inventory.QuestItems.Set(shard.item, 0)
		assert.True(t, gs.IsShardVisible(shard.pos, shard.item), "%s should be available while its Shadowlord lives", shard.name)

		gs.PartyState.Inventory.QuestItems.Set(shard.item, 1)
		assert.False(t, gs.IsShardVisible(shard.pos, shard.item), "%s should be hidden while owned", shard.name)

		gs.PartyState.Inventory.QuestItems.Set(shard.item, 0)
		gs.ShadowlordState.Kill(shard.lord)
		assert.False(t, gs.IsShardVisible(shard.pos, shard.item), "%s should remain absent after destruction", shard.name)
	}
}

func TestDestroyedShardLifeStateIsIndependentAndDoesNotAffectArtifacts(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.Basement).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()
	for _, item := range []references.QuestItem{
		references.ShardOfFalsehood,
		references.ShardOfHatred,
		references.ShardOfCowardice,
		references.Amulet,
	} {
		gs.PartyState.Inventory.QuestItems.Set(item, 0)
	}
	gs.ShadowlordState.Kill(references.Astaroth)

	assert.True(t, gs.IsShardVisible(references.Position{X: 192, Y: 80}, references.ShardOfFalsehood))
	assert.False(t, gs.IsShardVisible(references.Position{X: 130, Y: 65}, references.ShardOfHatred))
	assert.True(t, gs.IsShardVisible(references.Position{X: 176, Y: 184}, references.ShardOfCowardice))
	assert.True(t, gs.IsQuestItemVisible(references.Position{X: 105, Y: 225}, references.Amulet),
		"non-Shard quest items remain inventory-driven")
}

func TestActionGetLargeMapRejectsDestroyedShard(t *testing.T) {
	gs, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.Basement).
		WithPlayerAt(130, 64).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()
	gs.PartyState.Inventory.QuestItems.Set(references.ShardOfHatred, 0)
	gs.ShadowlordState.Kill(references.Astaroth)

	assert.False(t, gs.ActionGetLargeMap(references.Down))
	assert.Zero(t, gs.PartyState.Inventory.QuestItems.Get(references.ShardOfHatred))
	assert.Contains(t, callbacks.Messages, "Nothing to get!")
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestActionGetLargeMapCollectsAvailableShardOnce(t *testing.T) {
	gs, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.Basement).
		WithPlayerAt(192, 79).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()
	gs.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 0)
	position := references.Position{X: 192, Y: 80}

	assert.True(t, gs.ActionGetLargeMap(references.Down))
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.False(t, gs.IsShardVisible(position, references.ShardOfFalsehood))
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())

	callbacks.Reset()
	assert.False(t, gs.ActionGetLargeMap(references.Down))
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.Contains(t, callbacks.Messages, "Nothing to get!")
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestDestroyedShardRemainsAbsentAfterNativeHydration(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.Basement).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()
	gs.PartyState.Inventory.QuestItems.Set(references.ShardOfCowardice, 0)
	gs.ShadowlordState.Kill(references.Nosfentor)
	hydrated := roundTripSaveSnapshot(t, gs)

	assert.False(t, hydrated.IsShardVisible(
		references.Position{X: 176, Y: 184},
		references.ShardOfCowardice,
	))
}
