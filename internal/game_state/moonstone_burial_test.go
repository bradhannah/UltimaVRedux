package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
)

// TestMoonstoneBurialAndSearch tests the complete burial and search workflow
// Based on original Search/Get/Open/Jimmy rules buried-moonstone search behavior
// See ref:MOON-0004.
func TestMoonstoneBurialAndSearch(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	// Set player to overworld at a specific location
	gs.MapState.PlayerLocation.Location = references.BritanniaUnderworld // Overworld = 0
	gs.MapState.PlayerLocation.Floor = 0
	gs.MapState.PlayerLocation.Position = references.Position{X: 100, Y: 100}

	// Initialize moonstone system
	moonstoneSystem := gs.GetMoonstoneSystem()

	// Verify moonstone 0 starts buried at its initial location
	moonstone0 := moonstoneSystem.GetMoonstoneByIndex(0)
	t.Logf("Initial moonstone 0: X=%d, Y=%d, Map=%d, State=%d, IsFound=%v",
		moonstone0.X, moonstone0.Y, moonstone0.Map, moonstone0.State, moonstone0.IsFound)

	// Add moonstone to inventory first (simulate player has collected one)
	gs.PartyState.Inventory.Moonstones.IncrementByOne(references.Moonstone(0))

	// Bury moonstone at position (101, 100) on overworld (map 0)
	// This is one tile east of the player so they can search east to find it
	moonstoneSystem.BuryMoonstone(0, 101, 100, 0, 0)

	// Verify burial worked
	moonstone0 = moonstoneSystem.GetMoonstoneByIndex(0)
	t.Logf("After burial moonstone 0: X=%d, Y=%d, Map=%d, Level=%d, State=%d, IsFound=%v",
		moonstone0.X, moonstone0.Y, moonstone0.Map, moonstone0.Level, moonstone0.State, moonstone0.IsFound)

	if moonstone0.X != 101 || moonstone0.Y != 100 {
		t.Errorf("Burial location wrong: expected (101,100), got (%d,%d)", moonstone0.X, moonstone0.Y)
	}
	if moonstone0.Map != 0 {
		t.Errorf("Burial map wrong: expected 0 (overworld), got %d", moonstone0.Map)
	}
	if moonstone0.State != MoonstoneStateBuried {
		t.Errorf("State wrong: expected MoonstoneStateBuried (0), got %d", moonstone0.State)
	}

	// Test: GetMoonstoneAtPositionOnMap should find the moonstone
	// Player is at (100,100) on map 0, floor 0
	// Moonstone is at (101,100) on map 0, floor 0
	foundIndex := moonstoneSystem.GetMoonstoneAtPositionOnMap(101, 100, 0, 0)
	t.Logf("GetMoonstoneAtPositionOnMap(101,100,0,0) returned: %d", foundIndex)
	if foundIndex != 0 {
		t.Errorf("Expected to find moonstone 0, got %d", foundIndex)
	}

	// Test: Should NOT find moonstone on different map
	foundOnWrongMap := moonstoneSystem.GetMoonstoneAtPositionOnMap(101, 100, 0, 1) // map 1 instead of 0
	t.Logf("GetMoonstoneAtPositionOnMap(101,100,0,1) returned: %d", foundOnWrongMap)
	if foundOnWrongMap >= 0 {
		t.Error("Should NOT find moonstone on different map!")
	}

	// Test: Should NOT find moonstone on different level
	foundOnWrongLevel := moonstoneSystem.GetMoonstoneAtPositionOnMap(101, 100, 1, 0) // level 1 instead of 0
	t.Logf("GetMoonstoneAtPositionOnMap(101,100,1,0) returned: %d", foundOnWrongLevel)
	if foundOnWrongLevel >= 0 {
		t.Error("Should NOT find moonstone on different level!")
	}

	// Test: Original game behavior - search finds moonstone regardless of time of day
	gs.DateTime.Hour = 12 // Noon - daytime
	t.Logf("Testing search at noon (hour=%d) - should still find moonstone!", gs.DateTime.Hour)

	foundAtNoon := moonstoneSystem.GetMoonstoneAtPositionOnMap(101, 100, 0, 0)
	if foundAtNoon < 0 {
		t.Error("Original game behavior: should find moonstone even during daytime!")
	}

	gs.DateTime.Hour = 22 // Night
	foundAtNight := moonstoneSystem.GetMoonstoneAtPositionOnMap(101, 100, 0, 0)
	if foundAtNight < 0 {
		t.Error("Should find moonstone at night!")
	}

	t.Log("Moonstone burial and search behavior passed (ref:MOON-0004)")
}

// TestMoonstoneMapMatching tests that search only finds moonstones on the correct map
// Verifies fix for map matching based on original Search/Get/Open/Jimmy rules buried-moonstone search
// See ref:MOON-0004.
func TestMoonstoneMapMatching(t *testing.T) {
	config := helpers.LoadTestConfiguration(t)
	gameRefs := helpers.LoadTestGameReferences(t, config)
	saveData := helpers.LoadBootstrapSaveData(t)
	xTiles, yTiles := helpers.GetTestGameConstants()

	gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)

	mockCallbacks := NewMockSystemCallbacks(t)
	gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()

	moonstoneSystem := gs.GetMoonstoneSystem()

	// Bury moonstone at (50, 50) on map 1 (Britain)
	moonstoneSystem.BuryMoonstone(0, 50, 50, 1, 0)

	moonstone0 := moonstoneSystem.GetMoonstoneByIndex(0)
	t.Logf("Moonstone buried at X=%d, Y=%d, Map=%d", moonstone0.X, moonstone0.Y, moonstone0.Map)

	// Player is on overworld (map 0) at same coordinates
	gs.MapState.PlayerLocation.Location = references.BritanniaUnderworld // Map 0
	gs.MapState.PlayerLocation.Floor = 0
	gs.MapState.PlayerLocation.Position = references.Position{X: 50, Y: 50}

	// GetMoonstoneAtPositionOnMap should NOT find moonstone on wrong map
	foundOnMap0 := moonstoneSystem.GetMoonstoneAtPositionOnMap(50, 50, 0, 0) // Player's map
	t.Logf("GetMoonstoneAtPositionOnMap(50,50,0,0) - player on overworld: returned %d", foundOnMap0)
	if foundOnMap0 >= 0 {
		t.Error("Should NOT find moonstone buried on map 1 when searching from map 0!")
	}

	// GetMoonstoneAtPositionOnMap should find moonstone on correct map
	foundOnMap1 := moonstoneSystem.GetMoonstoneAtPositionOnMap(50, 50, 0, 1) // Moonstone's map
	t.Logf("GetMoonstoneAtPositionOnMap(50,50,0,1) - moonstone's map: returned %d", foundOnMap1)
	if foundOnMap1 < 0 {
		t.Error("Should find moonstone when searching on correct map!")
	}

	t.Log("Map matching test passed - search now correctly requires map match")
}
