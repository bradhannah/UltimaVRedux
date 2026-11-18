package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// TestStonegateTrapBehavior_Integration exercises the consumed-town-turn hazard
// checkpoint independently of movement and the presentation callback.
func TestStonegateTrapBehavior_Integration(t *testing.T) {
	gs, mockCallbacks := newBootstrapSaveTestState(t)

	// Position player in Stonegate at the reported trap location
	gs.MapState.PlayerLocation.Location = references.Stonegate
	gs.MapState.PlayerLocation.Floor = references.FloorNumber(0)
	gs.MapState.PlayerLocation.Position = references.Position{X: 15, Y: 16}
	gs.UpdateSmallMap(gs.GameReferences.TileReferences, gs.GameReferences.LocationReferences)

	// Verify we're on a TRAP tile (BrickFloorHole)
	currentTile := gs.GetCurrentAvatarGroundTile()
	require.True(t, currentTile.Is(indexes.BrickFloorHole))

	// Movement detection leaves Stonegate for the normal turn checkpoint.
	gs.ProcessTrapFallThrough()
	require.False(t, mockCallbacks.StonegateTrapTriggered)
	gs.CompleteSurfacePassCycle(references.SmallMapType)

	// Verify TriggerStonegateTrap callback was invoked
	// The callback receives the already committed consequence.
	assert.True(t, mockCallbacks.StonegateTrapTriggered,
		"TriggerStonegateTrap callback should be invoked for Stonegate TRAP tiles")

	require.True(t, gs.PartyState.IsAllPartyDead())
}

// TestNormalTrapBehavior_Integration tests normal TRAP tiles outside of Stonegate
func TestNormalTrapBehavior_Integration(t *testing.T) {
	gs, mockCallbacks := newBootstrapSaveTestState(t)

	// Position player in Britain (not Stonegate) on a multi-floor building
	gs.MapState.PlayerLocation.Location = references.Britain
	gs.MapState.PlayerLocation.Floor = references.FloorNumber(1) // Upper floor
	gs.MapState.PlayerLocation.Position = references.Position{X: 15, Y: 15}
	gs.UpdateSmallMap(gs.GameReferences.TileReferences, gs.GameReferences.LocationReferences)

	// Manually place a TRAP tile for testing
	currentMap := gs.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, gs.MapState.PlayerLocation.Floor)
	currentMap.SetTileByLayer(map_state.MapLayer, &gs.MapState.PlayerLocation.Position, indexes.BrickFloorHole)

	// Verify we're on a TRAP tile
	currentTile := gs.GetCurrentAvatarGroundTile()
	assert.True(t, currentTile.Is(indexes.BrickFloorHole), "Should be on TRAP tile")

	// Trigger the trap fall-through
	gs.ProcessTrapFallThrough()

	// For normal traps, we should start a fall sequence (not instant death)
	assert.True(t, gs.IsFalling(), "Should be in falling state for normal traps")

	// Process event queue to execute initial fall message
	gs.EventQueueManager.Update()

	// Verify trap message was displayed (after queue processing)
	mockCallbacks.AssertMessageContains("A TRAPDOOR!")
	assert.Equal(t, []SoundEffect{SoundFallingSevere}, mockCallbacks.SoundEffectsPlayed)

	// Verify no lava flooding (that's Stonegate-specific)
	lavaCount := 0
	for x := 0; x < 32; x++ {
		for y := 0; y < 32; y++ {
			pos := &references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			tile := currentMap.GetTopTile(pos)
			if tile.Is(indexes.Lava) {
				lavaCount++
			}
		}
	}
	assert.Equal(t, 0, lavaCount, "Normal traps should not flood with lava")

	t.Log("Normal trap behavior working correctly")
}

// Note: Placeholder test TestTrapFallCarpetImmunity_Integration was removed.
// Carpet immunity testing will be implemented when vehicle setup for test environment
// is properly understood and PartyVehicle.vehicleDetails can be correctly initialized.
