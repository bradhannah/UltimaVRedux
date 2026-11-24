package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStonegateProductionMovementCommitsBeforeCosmeticDelay(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Stonegate).WithPlayerAt(15, 17).
		WithRandomSeed(701).WithSystemCallbacks().Build()
	scene := newSurfaceCompletionTestScene(state)
	state.SystemCallbacks.Flow.Consequences.TriggerStonegateTrap = scene.StartStonegateTrapWorkflow
	state.SystemCallbacks.Flow.Consequences.TriggerDeathSequence = scene.StartDeathResurrectionWorkflow
	state.DateTime.Hour, state.DateTime.Minute = 12, 0
	before := state.DateTime
	require.NotEmpty(t, *state.CurrentNPCAIController.GetNpcs(), "real Stonegate actors must participate")
	trap := references.Position{X: 15, Y: 16}
	require.True(t, state.GetCurrentLayeredMap().GetTileByLayer(map_state.MapLayer, &trap).Is(indexes.BrickFloorHole))

	scene.smallMapInputHandler(ebiten.KeyUp)

	require.Equal(t, trap, state.MapState.PlayerLocation.Position)
	// Consequences are authoritative before any queued animation tick.
	assert.True(t, state.PartyState.IsAllPartyDead())
	require.Equal(t, before.Minute+1, state.DateTime.Minute)
	require.Equal(t, []int{1}, callbacks.TimeAdvanced())
	assert.Empty(t, *state.CurrentNPCAIController.GetNpcs())
	require.Equal(t, references.FloorNumber(0), state.MapState.PlayerLocation.Floor)
	scene.StartDeathResurrectionWorkflow()
	require.False(t, state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName), "trap owns the deferred death handoff")
	for range 100 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("stonegate_trap_workflow") {
			break
		}
	}
	require.False(t, state.EventQueueManager.HasWorkflow("stonegate_trap_workflow"))
	require.True(t, state.EventQueueManager.HasWorkflow(deathResurrectionWorkflowName))
	require.Equal(t, []int{1}, callbacks.TimeAdvanced())
	state.EventQueueManager.Shutdown()
}

func TestStonegatePassAndOtherApproachesUseOneTownCheckpoint(t *testing.T) {
	for _, test := range []struct {
		name     string
		position references.Position
		key      ebiten.Key
	}{
		{"Pass", references.Position{X: 15, Y: 16}, ebiten.KeySpace},
		{"East", references.Position{X: 13, Y: 14}, ebiten.KeyRight},
		{"West", references.Position{X: 17, Y: 16}, ebiten.KeyLeft},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Stonegate).WithPlayerAt(int(test.position.X), int(test.position.Y)).
				WithRandomSeed(701).WithSystemCallbacks().Build()
			scene := newSurfaceCompletionTestScene(state)
			starts := 0
			var committed game_state.StonegateTrapOutcome
			state.SystemCallbacks.Flow.Consequences.TriggerStonegateTrap = func(outcome game_state.StonegateTrapOutcome) {
				starts++
				committed = outcome
				scene.StartStonegateTrapWorkflow(outcome)
			}
			state.SystemCallbacks.Flow.Consequences.TriggerDeathSequence = scene.StartDeathResurrectionWorkflow
			scene.smallMapInputHandler(test.key)
			require.Equal(t, 1, starts)
			require.True(t, state.PartyState.IsAllPartyDead())
			require.Equal(t, []int{1}, callbacks.TimeAdvanced())
			// Repeat-start and shutdown must release presentation but retain the wipe.
			scene.StartStonegateTrapWorkflow(committed)
			state.EventQueueManager.Update()
			require.True(t, callbacks.InputBlocked)
			state.ElapsedMs += 2000
			state.EventQueueManager.Update()
			state.EventQueueManager.Shutdown()
			require.False(t, callbacks.InputBlocked)
			require.False(t, scene.hideMapUnits)
			require.False(t, scene.hideQuestItems)
			require.False(t, scene.hideItemStacks)
			require.Nil(t, scene.timedOverlayDialog)
			require.True(t, state.PartyState.IsAllPartyDead())
			require.Equal(t, []int{1}, callbacks.TimeAdvanced())
		})
	}
}
