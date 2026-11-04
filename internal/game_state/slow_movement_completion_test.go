package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func newSlowMovementFixture(t *testing.T, tile indexes.SpriteIndex) *GameState {
	t.Helper()
	state := newSourceHitFixture(t, references.NoPartyVehicle, 100)
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.GetNpcs().RemoveAllEnemies()
	state.DateTime.Hour, state.DateTime.Minute = 12, 0
	state.lastProcessedHour = 12
	state.initializeMoongateStage()
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &state.MapState.PlayerLocation.Position, tile)
	state.SetRandomSeed(123)

	return state
}

func TestSlowMovementResolvesEachSandtrapHitBeforeNextPhase(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Desert1)
	addOverworldTestEnemy(t, state, references.MonsterSandtrap,
		*references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position))
	wantRNG := beradrng.New(123)
	firstHP := uint16(100 - (wantRNG.Intn(8) + 1))
	secondHP := firstHP - uint16(wantRNG.Intn(8)+1)
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)

	assert.Equal(t, secondHP, state.PartyState.GetAvatar().GetCurrentHp())
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
	assert.Equal(t, byte(4), state.DateTime.Minute)
	assert.Equal(t, uint32(2), state.DateTime.Turn)
	assert.Nil(t, state.activeSlowMovement)
	assert.False(t, state.HasPendingOverworldContact())
}

func TestSlowMovementRetainsRemainingPhasesAcrossTacticalCombat(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Forest3)
	adjacent := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, adjacent, indexes.Grass)
	addOverworldTestEnemy(t, state, references.MonsterOrc, *adjacent)
	before := state.DateTime
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)

	require.NotNil(t, state.CombatScene)
	require.NotNil(t, state.activeSlowMovement)
	assert.Equal(t, 1, state.activeSlowMovement.remainingActorPhases)
	assert.Equal(t, before, state.DateTime, "combat interrupts before any slow-step time cost")
	assert.False(t, state.HasPendingOverworldContact(), "retained movement must not intercept tactical input")
	require.NoError(t, state.ResolvePendingOverworldContact())
	assert.Equal(t, before, state.DateTime)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)

	state.EndCombat(CombatVictory)
	drainSourceHitWorkflows(t, state)
	require.Nil(t, state.CombatScene)
	require.NotNil(t, state.activeSlowMovement)
	assert.True(t, state.HasPendingOverworldContact(), "existing input drain resumes after combat")
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
	addOverworldTestEnemy(t, state, references.MonsterSandtrap, *adjacent)
	state.SetRandomSeed(123)
	require.NoError(t, state.ResolvePendingOverworldContact())

	// Resume runs only the second extra phase, then the ordinary phase.
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
	assert.Equal(t, byte(6), state.DateTime.Minute)
	assert.Equal(t, uint32(2), state.DateTime.Turn)
	assert.Nil(t, state.activeSlowMovement)
	finished := state.DateTime
	require.NoError(t, state.ResolvePendingOverworldContact())
	assert.Equal(t, finished, state.DateTime, "completion cannot recharge costs")
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
}

func TestSlowMovementWaitsForWorkflowBeforeRemainingPhases(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Forest3)
	addOverworldTestEnemy(t, state, references.MonsterSandtrap,
		*references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position))
	state.EventQueueManager.Workflow("slow_contact_interaction").
		Priority(event_queue.PriorityHigh).Sequential().BlockInput().
		First(func() {}).Wait(100).Start()
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	require.NotNil(t, state.activeSlowMovement)
	assert.Equal(t, 2, state.activeSlowMovement.remainingActorPhases)
	assert.Zero(t, state.GetRNG().Calls())
	assert.Zero(t, state.DateTime.Minute)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
	require.NoError(t, state.ResolvePendingOverworldContact())
	assert.Zero(t, state.GetRNG().Calls())
	drainSourceHitWorkflows(t, state)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable,
		"pending movement is transient even after the workflow releases input")
	require.NoError(t, state.ResolvePendingOverworldContact())
	assert.Equal(t, byte(6), state.DateTime.Minute)
	assert.Equal(t, uint64(3), state.GetRNG().Calls())
	assert.Equal(t, uint32(2), state.DateTime.Turn)
	assert.Nil(t, state.activeSlowMovement)
}

func TestSlowMovementShutdownClearsRetainedCompletion(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Desert1)
	state.EventQueueManager.Workflow("slow_step_preceding_workflow").Priority(event_queue.PriorityNormal).Sequential().
		BlockInput().First(func() {}).Wait(100).Start()
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	require.NotNil(t, state.activeSlowMovement)
	state.EventQueueManager.Shutdown()
	assert.Nil(t, state.activeSlowMovement)
	assert.False(t, state.HasPendingOverworldContact())
	assert.Zero(t, state.DateTime.Minute)
}

func TestSlowMovementWhirlpoolWorkflowResumesAtNextExtraPhase(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Forest3)
	require.True(t, state.spawnWhirlpoolAt(*references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)))
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	for remaining := 1; remaining >= 0; remaining-- {
		require.NotNil(t, state.activeSlowMovement)
		assert.Equal(t, remaining, state.activeSlowMovement.remainingActorPhases)
		require.True(t, state.EventQueueManager.HasWorkflow(outdoorWhirlpoolWorkflowName(0)))
		assert.Zero(t, state.DateTime.Minute)
		drainSourceHitWorkflows(t, state)
		assert.Equal(t, uint64(2-remaining), state.GetRNG().Calls(), "each completed contact owns one hit")
		assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
		require.NoError(t, state.ResolvePendingOverworldContact())
	}
	assert.Nil(t, state.activeSlowMovement)
	assert.Equal(t, byte(6), state.DateTime.Minute)
	require.True(t, state.EventQueueManager.HasWorkflow(outdoorWhirlpoolWorkflowName(0)), "ordinary phase still has its own contact")
	drainSourceHitWorkflows(t, state)
	assert.Equal(t, uint64(3), state.GetRNG().Calls())
	assert.False(t, state.HasPendingOverworldContact())
}

func TestSlowMovementFatalExtraHitDoesNotRunLaterPhases(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Desert1)
	state.PartyState.GetAvatar().SetHp(1)
	addOverworldTestEnemy(t, state, references.MonsterSandtrap,
		*references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position))
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	assert.True(t, state.PartyState.IsAllPartyDead())
	assert.Equal(t, uint64(1), state.GetRNG().Calls())
	assert.Zero(t, state.DateTime.Minute)
	assert.Nil(t, state.activeSlowMovement)
}

func TestSlowMovementTimeStopFrozenMoongateDoesNotBlockPlayerTail(t *testing.T) {
	state := newSlowMovementFixture(t, indexes.Desert1)
	state.DateTime.Hour = 21
	state.ApplyMoongateTravelEffects(false)
	state.GetCurrentLayeredMap().SetVisible(true, &moongateStageTestGate)
	state.UpdateMoongateTransition()
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectTimeStop, TurnsRemaining: 2}
	advanceMoongateWorkflow(state, 3)
	require.True(t, state.IsMoongateTransitionActive())
	require.Equal(t, MoongateStageHidden, state.MoongateStage())
	state.SetRandomSeed(123)
	before := state.DateTime
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	assert.Nil(t, state.activeSlowMovement)
	assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, before, state.DateTime)
	assert.Zero(t, state.GetRNG().Calls())
	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	assert.False(t, state.HasActiveDurationEffect(DurationEffectTimeStop))
	assert.Equal(t, before, state.DateTime)
	assert.Zero(t, state.GetRNG().Calls())
	advanceMoongateWorkflow(state, 18)
	assert.Equal(t, MoongateStageFull, state.MoongateStage())
	assert.False(t, state.IsMoongateTransitionActive())
	assert.NoError(t, state.ValidateSaveSnapshotEligibility())
}
