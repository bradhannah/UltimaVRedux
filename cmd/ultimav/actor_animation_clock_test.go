package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestActorAnimationClockPreservesFractionAcrossRateBoundaries(t *testing.T) {
	for _, activation := range []int64{174, 175, 176, 12345} {
		var clock actorAnimationClock
		clock.update(0, false)
		clock.update(activation, true)
		assert.Equal(t, activation, clock.phaseMs)
		clock.update(activation+19, false)
		assert.Equal(t, activation, clock.phaseMs)
		assert.EqualValues(t, 19, clock.remainder)
		clock.update(activation+32, true)
		assert.Equal(t, activation+13, clock.phaseMs)
		clock.update(activation+33, true)
		assert.Equal(t, activation+14, clock.phaseMs, "fraction survives an intervening normal segment")
		assert.Zero(t, clock.remainder)
		before := clock
		clock.update(activation+33, true)
		assert.Equal(t, before, clock, "recast or duplicate update cannot restart phase")
	}
}

func TestActorAnimationClockSmallUpdatesEqualDirectProgress(t *testing.T) {
	for _, step := range []int64{1, 7, 16} {
		var small, direct actorAnimationClock
		small.update(123, true)
		direct.update(123, true)
		for now := int64(123); now < 4321; now += step {
			small.update(now, true)
		}
		small.update(4321, true)
		direct.update(4321, true)
		assert.Equal(t, direct, small)
		assert.EqualValues(t, 332, small.phaseMs)
		assert.EqualValues(t, 18, small.remainder)
	}
}

func TestActorAnimationClockLongIntervalAndNewEpoch(t *testing.T) {
	var clock actorAnimationClock
	clock.update(100, true)
	clock.update(1_000_000_000_100, false)
	assert.EqualValues(t, 50_000_000_100, clock.phaseMs)
	clock.update(1_000_000_000_101, false)
	assert.EqualValues(t, 50_000_000_101, clock.phaseMs)
	clock.update(7, true)
	assert.EqualValues(t, 7, clock.phaseMs)
	assert.Zero(t, clock.remainder)
	clock.update(27, true)
	assert.EqualValues(t, 8, clock.phaseMs)
}

func TestActorAnimationSurfaceLifecycleOffscreenAndReplacement(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	var reference references.NPCReference
	reference.SetKeyIndex(indexes.RatKeyIndex)
	actor := map_units.NewNPCFriendly(reference, 7)
	actor.SetFloor(state.MapState.PlayerLocation.Floor)
	units := state.CurrentNPCAIController.GetNpcs()
	units.ReplaceWith([]map_units.MapUnit{actor})
	scene := &GameScene{gameState: state}
	refresh := state.SystemCallbacks.Screen.UpdateDisplay
	state.SystemCallbacks.Screen.UpdateDisplay = func() {
		refresh()
		scene.updateActorAnimationClocks()
	}
	state.ElapsedMs = 173
	scene.updateActorAnimationClocks()
	state.ActivateTimeStopEffect(20)
	for range 200 {
		state.ElapsedMs += 10
		scene.updateActorAnimationClocks()
	}
	assert.EqualValues(t, 273, scene.surfaceActorAnimationTime(actor, state.ElapsedMs))
	actor.SetVisible(false)
	actor.SetPos(references.Position{X: 31, Y: 31})
	state.ElapsedMs += 2000
	scene.updateActorAnimationClocks()
	actor.SetVisible(true)
	assert.EqualValues(t, 373, scene.surfaceActorAnimationTime(actor, state.ElapsedMs))
	state.ClearDurationEffect()
	state.ElapsedMs += 100
	scene.updateActorAnimationClocks()
	assert.EqualValues(t, 473, scene.surfaceActorAnimationTime(actor, state.ElapsedMs))
	before := scene.surfaceActorAnimationTime(actor, state.ElapsedMs)
	for range 5 {
		assert.Equal(t, before, scene.surfaceActorAnimationTime(actor, state.ElapsedMs))
	}
	replacement := map_units.NewNPCFriendly(reference, 7)
	replacement.SetFloor(actor.Floor())
	units.ReplaceWith([]map_units.MapUnit{replacement})
	scene.updateActorAnimationClocks()
	require.Len(t, scene.actorAnimationClocks, 1)
	assert.Equal(t, state.ElapsedMs, scene.surfaceActorAnimationTime(replacement, state.ElapsedMs))
	units.ReplaceWith(nil)
	scene.updateActorAnimationClocks()
	assert.Empty(t, scene.actorAnimationClocks)
	scene.clearActorAnimationClocks()
	scene.clearActorAnimationClocks()
	assert.Nil(t, scene.actorAnimationClocks)
}

func TestDungeonActorClockPlannerSeparatesEnvironmentTime(t *testing.T) {
	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	snapshot.Wanderer = &game_state.DungeonWanderer{
		Monster: references.MonsterBat, Floor: player.Floor, Position: references.Position{X: 4, Y: 3},
	}
	var clock actorAnimationClock
	clock.update(199, false)
	clock.update(199, true)
	initial := buildDungeonRenderPlanWithPresentation(&snapshot, player, true, false, dungeonRenderPresentation{
		environmentTimeMs: 199, creatureTimeMs: clock.phaseMs,
	})
	for now := int64(200); now <= 2199; now++ {
		clock.update(now, true)
	}
	later := buildDungeonRenderPlanWithPresentation(&snapshot, player, true, false, dungeonRenderPresentation{
		environmentTimeMs: 2199, creatureTimeMs: clock.phaseMs,
	})
	assert.NotEqual(t, dungeonMonsterPlanOps(initial), dungeonMonsterPlanOps(later), "one slow sampling boundary is crossed")
	assert.EqualValues(t, 2199/dungeonAtmosphereFrameMs, later.atmosphereFrame)
	ordinary := buildDungeonRenderPlan(&snapshot, player, true, false, false, clock.phaseMs)
	assert.Equal(t, dungeonMonsterPlanOps(ordinary), dungeonMonsterPlanOps(later))
}

// buildDungeonRenderPlan is the fixed-epoch entry point rendering tests use; production supplies
// the persistent actor clock to buildDungeonRenderPlanWithPresentation.
func buildDungeonRenderPlan(
	snapshot *game_state.DungeonStateSnapshot,
	player references.PlayerLocation,
	lit bool,
	showSubduedUnlit bool,
	timeStopped bool,
	presentationTimeMs int64,
) dungeonRenderPlan {
	// Compatibility entry point for detached, fixed-epoch callers. Production
	// supplies the persistent actor clock to the extended builder below.
	var clock actorAnimationClock
	clock.update(0, timeStopped)
	clock.update(presentationTimeMs, timeStopped)

	return buildDungeonRenderPlanWithPresentation(snapshot, player, lit, showSubduedUnlit, dungeonRenderPresentation{
		environmentTimeMs: presentationTimeMs, creatureTimeMs: clock.phaseMs,
		state: nil, fieldDispels: nil,
	})
}
