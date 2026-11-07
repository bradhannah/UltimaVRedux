// Integration tests for Fire command workflows with real game data
// These tests validate the fire command flow including cannon mechanics, ship broadside firing,
// vehicle requirements, and SystemCallbacks integration across different map types.
package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestSmallMapCannonOwnsCriminalStatusAndContinuesWhenGuardActivationFails(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.CurrentNPCAIController = nil
	cannon := state.MapState.PlayerLocation.Position
	target := *references.Right.GetNewPositionInDirection(&cannon)

	state.StartSmallMapCannonFireWorkflow(cannon, target, references.Right, false, nil)
	require.NotPanics(t, func() {
		state.EventQueueManager.Update()
	})
	assert.True(t, state.IsCriminal(), "criminal status is caller-owned and precedes guard activation")

	for range 4 {
		if len(callbacks.ProjectilesStarted) > 0 {
			break
		}
		state.ElapsedMs++
		state.EventQueueManager.Update()
	}
	assert.Len(t, callbacks.ProjectilesStarted, 1, "guard activation errors must not abort cannon presentation")
}

// TestFireCommand_SmallMap_Integration tests fire command in small maps (towns, buildings)
// Small map Fire auto-detects adjacent cannon - no direction input needed.
// Original: original command rules town cannon fire
// See ref:KARMA-0004.
func TestFireCommand_SmallMap_Integration(t *testing.T) {
	// Test fire command in Lord British's Castle (small map environment)
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithPlayerAt(12, 12).
		WithSystemCallbacks().
		Build()

	t.Run("FireWithoutAdjacentCannon", func(t *testing.T) {
		mockCallbacks.Reset()

		// Test fire without adjacent cannon - should append the original "What?" fallback.
		result := gameState.ActionFireSmallMap()

		// Should fail - no cannon adjacent to player at (12,12)
		assert.False(t, result, "Fire should fail without adjacent cannon")

		assert.Empty(t, mockCallbacks.Messages, "The fallback appends to the Fire- prompt")
		assert.Equal(t, "What?", mockCallbacks.CurrentRowContent, "Should append the original fallback")
	})

	t.Run("FireAtDifferentPositions", func(t *testing.T) {
		// Test fire behavior at various positions in the castle
		testPositions := []references.Position{
			{X: 5, Y: 5}, {X: 10, Y: 10}, {X: 15, Y: 15}, {X: 8, Y: 12},
		}

		originalPosition := gameState.MapState.PlayerLocation.Position

		for _, pos := range testPositions {
			gameState.MapState.PlayerLocation.Position = pos
			mockCallbacks.Reset()

			result := gameState.ActionFireSmallMap()

			// Most positions won't have adjacent cannons, so should fail with "What?"
			// The test verifies consistent behavior regardless of position
			assert.Empty(t, mockCallbacks.Messages, "The fallback appends at position (%d,%d)", pos.X, pos.Y)
			assert.Equal(t, "What?", mockCallbacks.CurrentRowContent, "Should show 'What?' at position (%d,%d)", pos.X, pos.Y)
			assert.False(t, result, "Fire should fail at position (%d,%d) without cannon", pos.X, pos.Y)
		}

		// Restore original position
		gameState.MapState.PlayerLocation.Position = originalPosition
	})
}

// TestFireCommand_LargeMap_Integration tests fire command on large maps (overworld/ship)
func TestFireCommand_LargeMap_Integration(t *testing.T) {
	// Test fire command on large map (Britannia Underworld)
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	t.Run("FireWithoutVehicle", func(t *testing.T) {
		mockCallbacks.Reset()

		// Ensure player is not in a vehicle
		gameState.PartyVehicle.GetVehicleDetails().VehicleType = references.NoPartyVehicle

		result := gameState.ActionFireLargeMap(references.Up)

		// Should fail without vehicle (ship required for firing)
		assert.False(t, result, "Large map fire should fail without vehicle")

		// Validate SystemCallbacks integration
		assert.Equal(t, 1, len(mockCallbacks.Messages), "Should have error message")
		assert.Equal(t, "What?", mockCallbacks.Messages[0], "Should show 'What?' for no vehicle")

		// Should not advance time on failure
		assert.Equal(t, 0, len(mockCallbacks.TimeAdvanced()), "Failed fire should not advance time")
	})
}

// TestFireCommand_ErrorHandling_Integration tests edge cases and error handling
func TestFireCommand_ErrorHandling_Integration(t *testing.T) {
	// Test fire command error handling
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithPlayerAt(1, 1). // Near edge to test boundary conditions
		WithSystemCallbacks().
		Build()

	t.Run("FireAtMapEdges", func(t *testing.T) {
		// Test fire near map edges
		edgePositions := []references.Position{
			{X: 0, Y: 0}, {X: 1, Y: 1}, {X: 30, Y: 30},
		}

		originalPosition := gameState.MapState.PlayerLocation.Position

		for _, pos := range edgePositions {
			gameState.MapState.PlayerLocation.Position = pos
			mockCallbacks.Reset()

			result := gameState.ActionFireSmallMap()

			// Should handle edge cases gracefully with the original "What?" fallback.
			assert.False(t, result, "Fire should fail at edge positions without cannon at (%d,%d)", pos.X, pos.Y)
			assert.Empty(t, mockCallbacks.Messages, "The fallback appends at edge positions")
			assert.Equal(t, "What?", mockCallbacks.CurrentRowContent, "Should show 'What?' at edge positions")
		}

		// Restore original position
		gameState.MapState.PlayerLocation.Position = originalPosition
	})
}

// TestFireCommand_Regression_Integration tests for future implementation
func TestFireCommand_Regression_Integration(t *testing.T) {
	// This test validates the current implementation and will catch regressions
	// when the full cannon/broadside system is added
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithSystemCallbacks().
		Build()

	t.Run("CurrentImplementationBaseline", func(t *testing.T) {
		mockCallbacks.Reset()

		// Ensure no vehicle for large map testing
		gameState.PartyVehicle.GetVehicleDetails().VehicleType = references.NoPartyVehicle

		// Test small map fire (auto-detects adjacent cannon)
		mockCallbacks.Reset()
		smallMapResult := gameState.ActionFireSmallMap()
		// Without adjacent cannon, should fail with "What?"
		assert.False(t, smallMapResult, "Small map fire should fail without adjacent cannon")
		assert.Empty(t, mockCallbacks.Messages, "The fallback appends to the prompt")
		assert.Equal(t, "What?", mockCallbacks.CurrentRowContent, "Should show 'What?'")

		// Test large map fire (requires vehicle)
		mockCallbacks.Reset()
		largeMapResult := gameState.ActionFireLargeMap(references.Up)
		assert.False(t, largeMapResult, "Large map fire should fail without vehicle")
		assert.Equal(t, 1, len(mockCallbacks.Messages), "Should have 'What?' message")
		assert.Equal(t, "What?", mockCallbacks.Messages[0], "Large map fire should require vehicle")

		t.Logf("ℹ️ Fire command baseline tested - small map cannon auto-detection implemented")
	})
}
