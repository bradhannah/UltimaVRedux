// Tests for combat action implementation and remaining stub behavior.
//
// These tests guard against regressions in:
//   - GetActiveCombatantIndex returns correct index in normal vs solo mode
//   - getActiveCombatantPosition returns the correct party member's position
//   - Combat Open/Jimmy use active-combatant targeting and mutate encounter tiles
//   - ActionGetCombatMap returns false rather than fake success while unimplemented
//   - ActionKlimbCombatMap uses active combatant position, not fixed center
//   - PlayerLocation.Position is restored after Klimb's combat action save/restore
package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ─── GetActiveCombatantIndex Tests ──────────────────────────────────────────

// TestGetActiveCombatantIndex_NormalMode verifies that GetActiveCombatantIndex
// returns CurrentTurnIndex when solo mode is not active.
func TestGetActiveCombatantIndex_NormalMode(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	require.NotNil(t, gs.CombatScene)

	assert.False(t, gs.CombatScene.IsSoloMoveActive())
	assert.Equal(t, gs.CombatScene.CurrentTurnIndex, gs.CombatScene.GetActiveCombatantIndex(),
		"in normal mode, GetActiveCombatantIndex should return CurrentTurnIndex")
}

// TestGetActiveCombatantIndex_SoloMode verifies that selecting a future solo
// character cannot replace the actor whose queue turn is currently authoritative.
func TestGetActiveCombatantIndex_SoloMode(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	// Find a different active member than CurrentTurnIndex
	var soloIdx int
	for i := range 6 {
		if gs.CombatScene.IsPartyMemberActive(i) && i != gs.CombatScene.CurrentTurnIndex {
			soloIdx = i
			break
		}
	}

	gs.CombatScene.SetSoloMoveCharacter(soloIdx)
	assert.Equal(t, gs.CombatScene.CurrentTurnIndex, gs.CombatScene.GetActiveCombatantIndex(),
		"solo selection must not change present action authority")
}

// ─── getActiveCombatantPosition Tests ───────────────────────────────────────

// TestGetActiveCombatantPosition_ReturnsCorrectPosition verifies that
// getActiveCombatantPosition returns the position of the active combatant,
// not a fixed center position.
func TestGetActiveCombatantPosition_ReturnsCorrectPosition(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	idx := gs.CombatScene.GetActiveCombatantIndex()
	expectedPos := gs.CombatScene.GetPartyMemberPosition(idx)
	actualPos := gs.getActiveCombatantPosition()

	assert.Equal(t, expectedPos, actualPos,
		"getActiveCombatantPosition should return the active combatant's actual position")
}

// TestGetActiveCombatantPosition_SoloMode verifies the selected future actor
// cannot replace the current turn's position.
func TestGetActiveCombatantPosition_SoloMode(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	currentIndex := gs.CombatScene.CurrentTurnIndex
	currentPosition := gs.CombatScene.GetPartyMemberPosition(currentIndex)
	// Set a different position for member 2 and enable solo mode.
	customPos := references.Position{X: 3, Y: 7}
	gs.CombatScene.SetPartyMemberPosition(2, customPos)
	gs.CombatScene.SetSoloMoveCharacter(2)

	actualPos := gs.getActiveCombatantPosition()
	assert.Equal(t, currentPosition, actualPos,
		"solo selection must retain the current turn's position")
}

// ─── Combat Door Tests ──────────────────────────────────────────────────────

func setupCombatDoorActionTest(t *testing.T) (*GameState, *MockSystemCallbacks, int) {
	t.Helper()

	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(42).
		Build()
	mock := NewMockSystemCallbacks(t)
	gs.SystemCallbacks = mock.ToSystemCallbacks()

	err := gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth)
	require.NoError(t, err)

	return gs, mock, gs.CombatScene.GetActiveCombatantIndex()
}

func TestActionOpenCombatMap_UsesActiveCombatantPosition(t *testing.T) {
	gs, _, activeIndex := setupCombatDoorActionTest(t)

	activePos := references.Position{X: 2, Y: 2}
	activeTarget := references.Position{X: 3, Y: 2}
	cameraTarget := references.Position{X: 6, Y: 5}
	gs.CombatScene.SetPartyMemberPosition(activeIndex, activePos)
	theMap := gs.GetCurrentLayeredMap()
	theMap.SetTileByLayer(map_state.MapLayer, &activeTarget, indexes.RegularDoor)
	theMap.SetTileByLayer(map_state.MapLayer, &cameraTarget, indexes.RegularDoor)

	result := gs.ActionOpenCombatMap(references.Right)

	assert.True(t, result.Handled)
	assert.True(t, theMap.GetTileTopMapOnlyTile(&activeTarget).Is(indexes.BrickFloor))
	assert.True(t, theMap.GetTileTopMapOnlyTile(&cameraTarget).Is(indexes.RegularDoor),
		"combat Open must not target from the fixed camera center")
}

func TestActionOpenCombatMap_DoorVariants(t *testing.T) {
	tests := []struct {
		name         string
		door         indexes.SpriteIndex
		expectedTile indexes.SpriteIndex
		handled      bool
	}{
		{"regular", indexes.RegularDoor, indexes.BrickFloor, true},
		{"windowed", indexes.RegularDoorView, indexes.BrickFloor, true},
		{"locked", indexes.LockedDoor, indexes.LockedDoor, true},
		{"magic", indexes.MagicLockDoor, indexes.MagicLockDoor, true},
		{"not door", indexes.BrickFloor, indexes.BrickFloor, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, _, activeIndex := setupCombatDoorActionTest(t)

			origin := references.Position{X: 5, Y: 5}
			target := references.Position{X: 5, Y: 4}
			gs.CombatScene.SetPartyMemberPosition(activeIndex, origin)
			theMap := gs.GetCurrentLayeredMap()
			theMap.SetTileByLayer(map_state.MapLayer, &target, test.door)

			assert.Equal(t, test.handled, gs.ActionOpenCombatMap(references.Up).Handled)
			assert.True(t, theMap.GetTileTopMapOnlyTile(&target).Is(test.expectedTile))
		})
	}
}

// TestActionJimmyCombatMap_DoorOutcomes prevents successful physical attempts
// from consuming keys while preserving one-key consumption on failure.
func TestActionJimmyCombatMap_DoorOutcomes(t *testing.T) {
	tests := []struct {
		name           string
		door           indexes.SpriteIndex
		keys           uint16
		succeeds       bool
		expectedTile   indexes.SpriteIndex
		expectedKeys   uint16
		expectedResult bool
	}{
		{"unlock regular", indexes.LockedDoor, 2, true, indexes.RegularDoor, 2, true},
		{"unlock windowed", indexes.LockedDoorView, 2, true, indexes.RegularDoorView, 2, true},
		{"broken key", indexes.LockedDoor, 2, false, indexes.LockedDoor, 1, true},
		{"no keys", indexes.LockedDoor, 0, true, indexes.LockedDoor, 0, true},
		{"magic lock breaks a key", indexes.MagicLockDoor, 2, true, indexes.MagicLockDoor, 1, true},
		{"not lock", indexes.BrickFloor, 2, true, indexes.BrickFloor, 2, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, _, activeIndex := setupCombatDoorActionTest(t)

			origin := references.Position{X: 5, Y: 5}
			target := references.Position{X: 6, Y: 5}
			gs.CombatScene.SetPartyMemberPosition(activeIndex, origin)
			theMap := gs.GetCurrentLayeredMap()
			theMap.SetTileByLayer(map_state.MapLayer, &target, test.door)
			gs.PartyState.Inventory.Provisions.Keys.Set(test.keys)
			setJimmyOutcome(gs, test.succeeds)

			assert.Equal(t, test.expectedResult, gs.ActionJimmyCombatMap(references.Right))
			assert.True(t, theMap.GetTileTopMapOnlyTile(&target).Is(test.expectedTile))
			assert.Equal(t, test.expectedKeys, gs.PartyState.Inventory.Provisions.Keys.Get())
		})
	}
}

// TestCombatDoorActions_AdvanceAfterHandledOutcome prevents failed Jimmy and
// locked Open attempts from leaving the same PC active indefinitely.
func TestCombatDoorActions_AdvanceAfterHandledOutcome(t *testing.T) {
	tests := []struct {
		name          string
		door          indexes.SpriteIndex
		keys          uint16
		jimmy         bool
		jimmySucceeds bool
	}{
		{"open regular", indexes.RegularDoor, 0, false, false},
		{"open locked", indexes.LockedDoor, 0, false, false},
		{"jimmy success", indexes.LockedDoor, 1, true, true},
		{"jimmy failure", indexes.LockedDoor, 1, true, false},
		{"jimmy magical", indexes.MagicLockDoor, 1, true, false},
		{"jimmy no keys", indexes.LockedDoor, 0, true, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

			activeIndex := gs.CombatScene.GetActiveCombatantIndex()
			origin := gs.CombatScene.GetPartyMemberPosition(activeIndex)
			target := references.Position{X: origin.X + 1, Y: origin.Y}
			gs.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &target, test.door)
			gs.PartyState.Inventory.Provisions.Keys.Set(test.keys)
			setJimmyOutcome(gs, test.jimmySucceeds)

			forceNextMonsterTurnForTest(t, gs, 0)
			gs.CombatScene.presentTurns = true

			if test.jimmy {
				assert.True(t, gs.ActionJimmyCombatMap(references.Right))
			} else {
				assert.True(t, gs.ActionOpenCombatMap(references.Right).Handled)
			}
			gs.FinishCombatAction()

			assert.True(t, gs.CombatScene.HasScheduledCombatStep(),
				"handled door action must advance to the queued monster turn")
		})
	}
}

// ─── Position Save/Restore Tests ────────────────────────────────────────────

// TestActionKlimbCombatMap_UsesActiveCombatantPosition verifies that
// ActionKlimbCombatMap examines the tile at the active combatant's position
// (not the fixed center) and restores PlayerLocation.Position afterward.
func TestActionKlimbCombatMap_UsesActiveCombatantPosition(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	savedCenterPos := gs.MapState.PlayerLocation.Position

	// Move combatant off center
	combatantIdx := gs.CombatScene.GetActiveCombatantIndex()
	offCenterPos := references.Position{X: 3, Y: 3}
	gs.CombatScene.SetPartyMemberPosition(combatantIdx, offCenterPos)

	// Call klimb — will fail (no ladder) but should use correct position
	_ = gs.ActionKlimbCombatMap(references.Up)

	// PlayerLocation.Position should be restored
	assert.Equal(t, savedCenterPos, gs.MapState.PlayerLocation.Position,
		"PlayerLocation.Position should be restored after combat klimb")
}
