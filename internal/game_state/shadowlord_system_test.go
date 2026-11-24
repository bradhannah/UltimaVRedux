package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Test Shadowlord system implementation for planned work and planned work.
// Tests cover:
// - Shadowlord summoning at Keeps via Yell command
// - Shard usage at Flames to destroy Shadowlords
// - State management (alive/dead/summoned)
// - Failure paths (wrong position, wrong shard, etc.)

func TestShadowlordState_Initialization(t *testing.T) {
	state := references.NewShadowlordState()

	// All Shadowlords should start alive
	assert.True(t, state.IsAlive(references.Faulinei), "Faulinei should be alive")
	assert.True(t, state.IsAlive(references.Astaroth), "Astaroth should be alive")
	assert.True(t, state.IsAlive(references.Nosfentor), "Nosfentor should be alive")

	// None should be summoned
	assert.False(t, state.Summoned, "No Shadowlord should be summoned initially")
	assert.Equal(t, references.Shadowlord(-1), state.LastCalled, "LastCalled should be -1 initially")

	// Count
	assert.Equal(t, 3, state.CountAlive(), "All 3 Shadowlords should be alive")
	assert.False(t, state.AllDefeated(), "Shadowlords should not all be defeated")
}

func TestShadowlordState_Summoning(t *testing.T) {
	state := references.NewShadowlordState()

	// Summon Faulinei
	success := state.Summon(references.Faulinei)
	assert.True(t, success, "Should be able to summon Faulinei")
	assert.True(t, state.Summoned, "Summoned flag should be set")
	assert.Equal(t, references.Faulinei, state.LastCalled, "LastCalled should be Faulinei")

	// Cannot summon another while one is summoned
	success = state.Summon(references.Astaroth)
	assert.False(t, success, "Should not be able to summon while another is summoned")
	assert.Equal(t, references.Faulinei, state.LastCalled, "LastCalled should still be Faulinei")

	// Clear summoned state
	state.ClearSummoned()
	assert.False(t, state.Summoned, "Summoned should be cleared")
	assert.Equal(t, references.Shadowlord(-1), state.LastCalled, "LastCalled should be cleared")

	// Now can summon another
	success = state.Summon(references.Astaroth)
	assert.True(t, success, "Should be able to summon Astaroth after clearing")
}

func TestShadowlordState_Kill(t *testing.T) {
	state := references.NewShadowlordState()

	// Summon and kill Faulinei
	state.Summon(references.Faulinei)
	state.Kill(references.Faulinei)

	assert.False(t, state.IsAlive(references.Faulinei), "Faulinei should be dead")
	assert.True(t, state.IsAlive(references.Astaroth), "Astaroth should still be alive")
	assert.True(t, state.IsAlive(references.Nosfentor), "Nosfentor should still be alive")
	assert.False(t, state.Summoned, "Kill should clear summoned state")
	assert.Equal(t, references.Shadowlord(-1), state.LastCalled, "Kill should clear LastCalled")

	// Cannot summon dead Shadowlord
	success := state.Summon(references.Faulinei)
	assert.False(t, success, "Should not be able to summon dead Shadowlord")

	// Kill all
	state.Summon(references.Astaroth)
	state.Kill(references.Astaroth)
	state.Summon(references.Nosfentor)
	state.Kill(references.Nosfentor)

	assert.True(t, state.AllDefeated(), "All Shadowlords should be defeated")
	assert.Equal(t, 0, state.CountAlive(), "No Shadowlords should be alive")
}

func TestShadowlordState_Reset(t *testing.T) {
	state := references.NewShadowlordState()

	// Kill some and summon
	state.Kill(references.Faulinei)
	state.Summon(references.Astaroth)

	// Reset
	state.Reset()

	assert.True(t, state.IsAlive(references.Faulinei), "Faulinei should be alive after reset")
	assert.True(t, state.IsAlive(references.Astaroth), "Astaroth should be alive after reset")
	assert.True(t, state.IsAlive(references.Nosfentor), "Nosfentor should be alive after reset")
	assert.False(t, state.Summoned, "Summoned should be cleared after reset")
}

func TestFlameLocations(t *testing.T) {
	// Verify flame locations match original item-use rules
	// See ref:SHADOW-0002.
	tests := []struct {
		flame    references.FlameOfPrinciple
		location references.Location
		x, y     int
		floor    int
	}{
		{references.FlameOfTruth, references.Lycaeum, 15, 9, 2},
		{references.FlameOfLove, references.EmpathAbbey, 15, 3, 1},
		{references.FlameOfCourage, references.SerpentsHold, 15, 16, -1}, // -1 = basement
	}

	for _, tc := range tests {
		t.Run(tc.flame.String(), func(t *testing.T) {
			loc := references.FlameLocations[tc.flame]
			assert.Equal(t, tc.location, loc.Location, "Wrong location for %s", tc.flame)
			assert.Equal(t, tc.x, loc.X, "Wrong X for %s", tc.flame)
			assert.Equal(t, tc.y, loc.Y, "Wrong Y for %s", tc.flame)
			assert.Equal(t, tc.floor, loc.Floor, "Wrong floor for %s", tc.flame)
		})
	}
}

func TestFlameShardMatching(t *testing.T) {
	// Verify shard-flame-Shadowlord relationships
	tests := []struct {
		flame      references.FlameOfPrinciple
		shard      references.QuestItem
		shadowlord references.Shadowlord
	}{
		{references.FlameOfTruth, references.ShardOfFalsehood, references.Faulinei},
		{references.FlameOfLove, references.ShardOfHatred, references.Astaroth},
		{references.FlameOfCourage, references.ShardOfCowardice, references.Nosfentor},
	}

	for _, tc := range tests {
		t.Run(tc.flame.String(), func(t *testing.T) {
			assert.Equal(t, tc.shard, tc.flame.GetRequiredShard(), "Wrong shard for %s", tc.flame)
			assert.Equal(t, tc.shadowlord, tc.flame.GetShadowlord(), "Wrong Shadowlord for %s", tc.flame)
		})
	}
}

func TestIsKeepLocation(t *testing.T) {
	keeps := []references.Location{
		references.Lycaeum,
		references.EmpathAbbey,
		references.SerpentsHold,
	}

	for _, keep := range keeps {
		assert.True(t, references.IsKeepLocation(keep), "%s should be a Keep", keep)
	}

	nonKeeps := []references.Location{
		references.Britain,
		references.Trinsic,
		references.Moonglow,
	}

	for _, loc := range nonKeeps {
		assert.False(t, references.IsKeepLocation(loc), "%s should not be a Keep", loc)
	}
}

func TestTrySummonShadowlord_Integration(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Lycaeum).
		WithFloor(2).
		WithPlayerAt(15, 9).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()
	gs.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)

	// Try to summon Faulinei
	result := gs.ActionYellSmallMap("FAULINEI")

	assert.True(t, result, "Summoning should succeed")
	assert.True(t, gs.ShadowlordState.Summoned, "Shadowlord should be summoned")
	assert.Equal(t, references.Faulinei, gs.ShadowlordState.LastCalled, "Faulinei should be LastCalled")
	assert.Empty(t, mockCallbacks.TimeAdvanced(), "successful summoning must suppress the immediate Shadowlord action")
	enemy := gs.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(
		references.Position{X: 15, Y: 7}, 2,
	)
	if assert.NotNil(t, enemy, "Shadowlord should spawn two tiles north on the current floor") {
		lord, ok := enemy.ShadowlordIdentity()
		assert.True(t, ok)
		assert.Equal(t, references.Faulinei, lord)
	}
}

func TestTrySummonShadowlord_SpawnFailureRestoresState(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Lycaeum).
		WithFloor(2).
		WithPlayerAt(15, 9).
		WithSystemCallbacks().
		Build()
	gs.ShadowlordState.Reset()
	gs.CurrentNPCAIController.GetNpcs().ReplaceWith(make([]map_units.MapUnit, map_units.MaximumNpcsPerMap))

	result := gs.ActionYellSmallMap("FAULINEI")

	assert.False(t, result)
	assert.False(t, gs.ShadowlordState.Summoned)
	assert.Equal(t, references.NoShadowlord, gs.ShadowlordState.LastCalled)
	assert.Empty(t, mockCallbacks.ShadowlordSummonedTriggered)
	assert.Contains(t, mockCallbacks.Messages, "No effect!")
}

func TestTrySummonShadowlord_NotAtKeep(t *testing.T) {
	gs, mockCallbacks := newBootstrapSaveTestState(t)

	// Set player at Britain (not a Keep)
	gs.MapState.PlayerLocation.Location = references.Britain
	gs.MapState.PlayerLocation.Floor = 0
	gs.MapState.PlayerLocation.Position = references.Position{X: 15, Y: 10}

	// Try to summon via ActionYellSmallMap (the actual entry point)
	// The location check is in ActionYellSmallMap, not TrySummonShadowlord
	result := gs.ActionYellSmallMap("FAULINEI")

	assert.False(t, result, "Yell should return false outside Keep")
	assert.False(t, gs.ShadowlordState.Summoned, "No Shadowlord should be summoned")
	// Should show "No effect!" message via ShowActionFeedback (which adds to Messages when alsoAddToOutput=true)
	assert.True(t, len(mockCallbacks.Messages) > 0, "Should have feedback message")
	assert.Contains(t, mockCallbacks.Messages[len(mockCallbacks.Messages)-1], "No effect", "Should show 'No effectens' message")
	assert.Empty(t, mockCallbacks.TimeAdvanced(), "the dialog workflow owns failed-Yell turn completion")
}

func TestClearShadowlordOnLocationChange_Integration(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)
	gs.ShadowlordState.Reset()

	// Start at Lycaeum with summoned Shadowlord
	gs.MapState.PlayerLocation.Location = references.Lycaeum
	gs.MapState.PlayerLocation.Floor = 0
	gs.ShadowlordState.Summon(references.Faulinei)

	// Verify summoned
	assert.True(t, gs.ShadowlordState.Summoned, "Shadowlord should be summoned")

	// Transition to Britain (different location)
	gs.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 15}, TransitionFullLocation, false)

	// Summoned state should be cleared
	assert.False(t, gs.ShadowlordState.Summoned, "Shadowlord should be cleared after leaving Keep")
}

func TestClearShadowlordOnFloorChange_Integration(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)
	gs.ShadowlordState.Reset()

	// Start at Lycaeum floor 0 with summoned Shadowlord
	gs.MapState.PlayerLocation.Location = references.Lycaeum
	gs.MapState.PlayerLocation.Floor = 0
	gs.ShadowlordState.Summon(references.Faulinei)

	// Verify summoned
	assert.True(t, gs.ShadowlordState.Summoned, "Shadowlord should be summoned")

	// Climb a ladder or stairs within Lycaeum, the route players actually take.
	require.True(t, gs.GoUpOneFloor())

	// Summoned state should be cleared (different floor)
	assert.False(t, gs.ShadowlordState.Summoned, "Shadowlord should be cleared after floor change")
}
