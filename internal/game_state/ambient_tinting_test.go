package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestAmbientTintDarknessOnlyOverridesUnderworldExploration(t *testing.T) {
	for _, context := range []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		camp     *references.PlayerLocation
		night    bool
	}{
		{"underworld", references.BritanniaUnderworld, -1, nil, true},
		{"overworld", references.BritanniaUnderworld, 0, nil, false},
		{"town", references.LordBritishsCastle, 0, nil, false},
		{"basement", references.LordBritishsCastle, -1, nil, false},
		{"dungeon", references.Deceit, -1, nil, false},
		{"combat", references.CombatLocation, -1, nil, false},
		{"camping", references.CampingLocation, -1, nil, false},
		{"surface camp", references.CampingLocation, 0, &references.PlayerLocation{Location: references.BritanniaUnderworld}, false},
		{
			"underworld camp", references.CampingLocation, 0,
			&references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: -1}, true,
		},
		{"dungeon camp", references.CampingLocation, 0, &references.PlayerLocation{Location: references.Deceit, Floor: 3}, true},
		// An ambush keeps the camp scene, but combat owns its own presentation.
		{
			"underworld camp ambush", references.CombatLocation, 0,
			&references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: -1}, false,
		},
	} {
		t.Run(context.name, func(t *testing.T) {
			state := &GameState{}
			state.MapState.PlayerLocation = references.PlayerLocation{Location: context.location, Floor: context.floor}
			if context.camp != nil {
				state.CampingScene = &camping.CampingScene{OriginalPlayerLocation: *context.camp}
			}
			for _, clock := range []struct{ hour, minute byte }{{0, 0}, {5, 0}, {5, 15}, {5, 30}, {12, 0}, {19, 30}, {19, 45}, {20, 0}} {
				state.DateTime.Hour, state.DateTime.Minute = clock.hour, clock.minute
				want := state.DateTime.GetAmbientTintDarkness()
				if context.night {
					want = 1
				}
				assert.Equal(t, want, state.AmbientTintDarkness())
			}
		})
	}
}

// A dungeon rest that spans the surface day must never brighten: the camp map
// replaces the player location, so the dark rule has to follow the camp's origin.
func TestDungeonCampStaysDarkThroughSurfaceDay(t *testing.T) {
	state, _ := newDungeonCampingTestStateWithCallbacks(t)
	state.DateTime.Hour, state.DateTime.Minute = 9, 0
	token, _ := state.BeginDungeonCommandCycle()
	state.ActionHoleUpDungeonMap(token)
	scene, plan, err := state.PrepareDungeonCampingPlan(6, NoGuardAssigned, token)
	require.NoError(t, err)
	state.SetRandomSeed(dungeonCampingNoAmbushSeed(t, 6))
	installAutomaticAnyKey(state)
	workflow := state.EventQueueManager.Workflow("dungeon_camp_darkness_test").
		Priority(event_queue.PriorityNormal).Sequential().BlockInput().First(func() {})
	state.AddDungeonCampingSequenceToBuilder(workflow, scene, plan, func() {
		state.CompleteDungeonCampingCommand(token)
	})
	workflow.Start()

	campedUpdates := 0
	for update := 0; update < 2000 && state.EventQueueManager.HasActiveWorkflows(); update++ {
		state.ElapsedMs += 500
		state.EventQueueManager.Update()
		if state.IsInCamping() {
			campedUpdates++
			require.Zero(t, state.DateTime.GetAmbientTintDarkness(), "the rest must stay inside surface daytime")
			require.Equal(t, float32(1), state.AmbientTintDarkness(), "camp brightened at %02d:%02d",
				state.DateTime.Hour, state.DateTime.Minute)
		}
	}
	require.False(t, state.EventQueueManager.HasActiveWorkflows(), "camping workflow did not complete")
	require.Positive(t, campedUpdates)
	assert.Equal(t, references.Deceit, state.MapState.PlayerLocation.Location)
	assert.Nil(t, state.CampingScene)
}

func TestPlayerLightSelectionOnlyOverridesUnderworldExploration(t *testing.T) {
	for _, context := range []struct {
		location references.Location
		floor    references.FloorNumber
		ambient  bool
	}{
		{references.BritanniaUnderworld, -1, true},
		{references.BritanniaUnderworld, 0, false},
		{references.LordBritishsCastle, -1, false},
		{references.Deceit, -1, false},
		{references.CombatLocation, -1, false},
		{references.CampingLocation, -1, false},
	} {
		state := &GameState{}
		state.MapState.PlayerLocation = references.PlayerLocation{Location: context.location, Floor: context.floor}
		state.DateTime.Hour = 12
		want := map_state.PlayerLightNone
		if context.ambient {
			want = map_state.PlayerLightAmbient
		}
		assert.Equal(t, want, state.GetPlayerLightType())
		state.MapState.Lighting.LightTorch()
		assert.Equal(t, map_state.PlayerLightTorch, state.GetPlayerLightType())
		state.MapState.Lighting.SetMagicLight(true)
		assert.Equal(t, map_state.PlayerLightInLor, state.GetPlayerLightType())
	}
}
