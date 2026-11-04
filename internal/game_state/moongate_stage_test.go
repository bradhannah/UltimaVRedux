package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var moongateStageTestGate = references.Position{X: 224, Y: 133}

func newMoongateStageTestState(t *testing.T, hour byte, stage MoongateStage) *GameState {
	t.Helper()
	state := &GameState{
		DateTime: datetime.UltimaDate{Hour: hour}, moongateStage: stage, moonstoneSystem: NewMoonstoneSystem(),
	}
	state.MapState.LayeredMaps = *map_state.NewLayeredMaps(
		references.NewTileReferences(), &references.LargeMapReference{}, &references.LargeMapReference{}, 19, 13,
	)
	// Beside the Moonglow source gate with it in sight, stage changes animate.
	state.MapState.PlayerLocation.Position = references.Position{X: 224, Y: 134}
	state.GetCurrentLayeredMap().SetVisible(true, &moongateStageTestGate)
	state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	state.EventQueueManager = event_queue.NewEventQueueManager(func() int64 { return state.ElapsedMs }, nil)

	return state
}

func advanceMoongateWorkflow(state *GameState, updates int) {
	for range updates {
		state.EventQueueManager.Update()
		state.ElapsedMs += moongateStageStepMs
	}
	if state.MoongateStage() == state.desiredMoongateStage() {
		state.EventQueueManager.Update()
	}
}

func TestInitializeMoongateStageUsesActiveWindow(t *testing.T) {
	for _, test := range []struct {
		hour byte
		want MoongateStage
	}{
		{hour: 4, want: MoongateStageFull},
		{hour: 5, want: MoongateStageHidden},
		{hour: 19, want: MoongateStageHidden},
		{hour: 20, want: MoongateStageFull},
	} {
		state := &GameState{DateTime: datetime.UltimaDate{Hour: test.hour}}
		state.initializeMoongateStage()
		assert.Equal(t, test.want, state.MoongateStage(), "hour %d", test.hour)
	}
}

func TestMoongateTransitionGrowsAndShrinksSharedStage(t *testing.T) {
	state := newMoongateStageTestState(t, 20, MoongateStageHidden)
	state.UpdateMoongateTransition()
	require.True(t, state.IsMoongateTransitionActive())

	advanceMoongateWorkflow(state, 8)
	assert.Equal(t, MoongateStage(8), state.MoongateStage())
	advanceMoongateWorkflow(state, 8)
	assert.Equal(t, MoongateStageFull, state.MoongateStage())
	assert.False(t, state.IsMoongateTransitionActive())

	state.DateTime.Hour = 5
	state.UpdateMoongateTransition()
	require.True(t, state.IsMoongateTransitionActive())
	advanceMoongateWorkflow(state, 16)
	assert.Equal(t, MoongateStageHidden, state.MoongateStage())
	assert.False(t, state.IsMoongateTransitionActive())
}

func TestMoongateTransitionTimeStopFreezesStage(t *testing.T) {
	state := newMoongateStageTestState(t, 20, MoongateStageHidden)
	state.UpdateMoongateTransition()
	require.True(t, state.ActivateTimeStopEffect(20))

	advanceMoongateWorkflow(state, 20)
	assert.Equal(t, MoongateStageHidden, state.MoongateStage())
	assert.True(t, state.IsMoongateTransitionActive())

	state.ActiveDurationEffect = DurationEffect{}
	advanceMoongateWorkflow(state, 16)
	assert.Equal(t, MoongateStageFull, state.MoongateStage())
	assert.False(t, state.IsMoongateTransitionActive())
}

func TestMoongateTeleportPreemptsPartialStageTransition(t *testing.T) {
	state := newMoongateStageTestState(t, 20, MoongateStageHidden)
	state.UpdateMoongateTransition()
	advanceMoongateWorkflow(state, 3)
	teleported := false
	state.EventQueueManager.Workflow(MoongateTeleportWorkflow).
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() { teleported = true }).
		Start()

	advanceMoongateWorkflow(state, 2)
	assert.True(t, teleported)
	assert.False(t, state.IsTeleportationActive())
	advanceMoongateWorkflow(state, 16)
	assert.Equal(t, MoongateStageFull, state.MoongateStage())
	assert.False(t, state.EventQueueManager.HasActiveWorkflows())
}

func TestMoongateStageAnimatesAndNarratesOnlyGatesInSight(t *testing.T) {
	for _, test := range []struct {
		hour    byte
		stage   MoongateStage
		inSight bool
		want    []string
	}{
		{hour: 20, stage: MoongateStageHidden, inSight: true, want: []string{"Instantly, a shimmering blue door springs up!"}},
		{hour: 5, stage: MoongateStageFull, inSight: true, want: []string{"The shimmering door fades from view."}},
		{hour: 20, stage: MoongateStageHidden, want: []string{}},
		{hour: 5, stage: MoongateStageFull, want: []string{}},
	} {
		state := newMoongateStageTestState(t, test.hour, test.stage)
		callbacks := NewMockSystemCallbacks(t)
		state.SystemCallbacks = callbacks.ToSystemCallbacks()
		state.GetCurrentLayeredMap().SetVisible(test.inSight, &moongateStageTestGate)

		state.UpdateMoongateTransition()
		assert.Equal(t, test.inSight, state.IsMoongateTransitionActive(), "an unseen change holds no workflow")
		advanceMoongateWorkflow(state, 17)

		assert.Equal(t, state.desiredMoongateStage(), state.MoongateStage())
		assert.Equal(t, test.want, callbacks.Messages, "hour %d in sight %v", test.hour, test.inSight)
	}
}

func TestMoongateTransitionWaitsToStartUntilTimeStopExpires(t *testing.T) {
	state := newMoongateStageTestState(t, 20, MoongateStageHidden)
	require.True(t, state.ActivateTimeStopEffect(20))

	state.UpdateMoongateTransition()
	assert.False(t, state.IsMoongateTransitionActive())
	assert.Equal(t, MoongateStageHidden, state.MoongateStage())

	state.ActiveDurationEffect = DurationEffect{}
	state.UpdateMoongateTransition()
	assert.True(t, state.IsMoongateTransitionActive())
}

func TestPartialMoongateStageControlsEntryAndVisibility(t *testing.T) {
	state := &GameState{
		moongateStage:   MoongateStage(1),
		moonstoneSystem: NewMoonstoneSystem(),
	}
	state.MapState.PlayerLocation.Position = references.Position{X: 224, Y: 133}

	assert.True(t, state.IsPlayerOnActiveMoongate())
	assert.Equal(t, float32(1)/float32(MoongateStageFull), state.GetMoongateVisibilityLevel())
	assert.NotEmpty(t, state.GetActiveMoongatePositions())

	state.moongateStage = MoongateStageHidden
	assert.False(t, state.IsPlayerOnActiveMoongate())
	assert.Empty(t, state.GetActiveMoongatePositions())
}

func TestMoongateTransitionBlocksSaveEligibility(t *testing.T) {
	state := getBritain2GameState()
	if state == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	state.DateTime.Hour = 20
	state.moongateStage = MoongateStageHidden
	state.GetCurrentLayeredMap().SetVisible(true, &moongateStageTestGate)
	state.UpdateMoongateTransition()

	err := state.ValidateSaveSnapshotEligibility()
	require.ErrorIs(t, err, ErrSaveSnapshotNotRepresentable)
	assert.Contains(t, err.Error(), "active event queue work")
}

func TestSaveSnapshotLoadReconstructsMoongateStageFromHour(t *testing.T) {
	state := getBritain2GameState()
	gameReferences := getBaseGameReferences()
	if state == nil || gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)

	for _, test := range []struct {
		hour int
		want MoongateStage
	}{
		{hour: 4, want: MoongateStageFull},
		{hour: 5, want: MoongateStageHidden},
		{hour: 20, want: MoongateStageFull},
	} {
		snapshot.World.Calendar.Hour = test.hour
		hydrated, hydrateErr := NewGameStateFromSaveSnapshot(
			snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
		)
		require.NoError(t, hydrateErr)
		assert.Equal(t, test.want, hydrated.MoongateStage(), "hour %d", test.hour)
	}
}
