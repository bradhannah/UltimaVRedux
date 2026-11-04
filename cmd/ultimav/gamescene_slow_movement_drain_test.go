package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestSlowMovementProductionDrainIgnoresTimeStoppedMoongateRegrowth(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 0, 123)
	state.TransitionToLocation(references.BritanniaUnderworld, 0, references.Position{X: 100, Y: 100}, game_state.TransitionFullLocation, false)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	state.DebugOptions.MonsterGen = false
	state.DateTime.Hour, state.DateTime.Minute = 21, 0
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Forest3)
	enemy := map_units.NewEnemyNPC((*state.GameReferences.EnemyReferences)[int(references.MonsterWhirlpool)], 1)
	enemy.SetPos(*references.Right.GetNewPositionInDirection(&position))
	enemy.SetFloor(0)
	enemy.SetVisible(true)
	require.True(t, state.CurrentNPCAIController.GetNpcs().AddEnemy(enemy))
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	require.True(t, state.EventQueueManager.HasInputBlockingWorkflows())
	require.True(t, state.HasPendingOverworldContact())
	// The real contact suspends the slow step. Its completion must not wait on
	// a subsequent unrelated, indefinitely Time-Stopped regrowth workflow.
	for i := 0; state.EventQueueManager.HasInputBlockingWorkflows(); i++ {
		require.Less(t, i, 100)
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}
	state.CurrentNPCAIController.GetNpcs().RemoveAllEnemies()
	state.ApplyMoongateTravelEffects(false)
	// A source gate in sight makes regrowth run as a workflow instead of completing at once.
	moonglowGate := references.Position{X: 224, Y: 133}
	state.GetCurrentLayeredMap().SetVisible(true, &moonglowGate)
	state.UpdateMoongateTransition()
	state.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectTimeStop, TurnsRemaining: 2}
	state.EventQueueManager.Update()
	require.True(t, state.IsMoongateTransitionActive())
	state.ProcessWindThroughElapsed(state.ElapsedMs)
	state.SetRandomSeed(123)
	before := state.DateTime
	require.NoError(t, scene.Update(nil))
	assert.False(t, state.HasPendingOverworldContact(), "production drain must resume the remaining slow step")
	assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, before, state.DateTime)
	assert.Zero(t, state.GetRNG().Calls())
	assert.Equal(t, game_state.MoongateStageHidden, state.MoongateStage())
	assert.True(t, state.IsMoongateTransitionActive())
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	require.NoError(t, scene.Update(nil))
	assert.False(t, state.HasActiveDurationEffect(game_state.DurationEffectTimeStop))
	assert.Equal(t, before, state.DateTime)
	assert.Zero(t, state.GetRNG().Calls())
	for range 20 {
		state.ElapsedMs += 200
		state.EventQueueManager.Update()
	}
	assert.Equal(t, game_state.MoongateStageFull, state.MoongateStage())
	assert.False(t, state.IsMoongateTransitionActive())
	assert.NoError(t, state.ValidateSaveSnapshotEligibility())
}
