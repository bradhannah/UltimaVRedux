package game_state

import (
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/stretchr/testify/require"
)

func TestCampingAccelerationPreservesControlledCycleTraceAndCleanup(t *testing.T) {
	for _, dungeon := range []bool{false, true} {
		for _, increment := range []int64{16, 50, 200, 1300} {
			t.Run(fmt.Sprintf("dungeon=%t/update=%d", dungeon, increment), func(t *testing.T) {
				var baseline []bardCampingCycleSnapshot
				var baselineParty [party_state.MaxActiveParty]party_state.PlayerCharacter
				var baselineNext int
				for _, accelerateAt := range []int{-1, 0, 6} {
					state, callbacks, scene, completions := startPacingCamp(t, dungeon)
					var trace []bardCampingCycleSnapshot
					for updates := 0; updates < 20000 && state.EventQueueManager.HasActiveWorkflows(); updates++ {
						if scene.IsSleeping() && accelerateAt >= 0 && scene.CurrentCycle >= accelerateAt {
							before := state.GetRNG().Calls()
							require.True(t, state.RequestCampingAcceleration())
							require.True(t, state.RequestCampingAcceleration())
							require.Equal(t, before, state.GetRNG().Calls())
						}
						state.ElapsedMs += increment
						state.EventQueueManager.Update()
						if scene.CurrentCycle > len(trace) {
							require.Equal(t, len(trace)+1, scene.CurrentCycle, "even a hitch executes at most one sleeping cycle")
							trace = append(trace, bardCampingCycleSnapshot{
								cycle: scene.CurrentCycle, hour: state.DateTime.Hour, minute: state.DateTime.Minute,
								rngCalls: state.GetRNG().Calls(), guard: scene.GetPartyMemberPosition(scene.GuardIndex),
								hp: state.PartyState.GetCharacter(1).GetCurrentHp(),
							})
						}
					}
					require.False(t, state.EventQueueManager.HasActiveWorkflows())
					require.Len(t, trace, camping.CyclesPerHour)
					require.Equal(t, 1, *completions)
					require.False(t, scene.IsSleeping())
					require.False(t, scene.IsAccelerated())
					require.False(t, state.RequestCampingAcceleration())
					require.Equal(t, 1, callbacks.StopRequestCount(SoundWind))
					require.Equal(t, 1, callbacks.StopRequestCount(SoundFireCrackle))
					var party [party_state.MaxActiveParty]party_state.PlayerCharacter
					for index := range party {
						party[index] = *state.PartyState.GetCharacter(index)
					}
					next := state.GetRNG().Intn(10000)
					if accelerateAt == -1 {
						baseline, baselineParty, baselineNext = trace, party, next
					} else {
						require.Equal(t, baseline, trace, "identical camping inputs preserve ordered decisions")
						require.Equal(t, baselineParty, party)
						require.Equal(t, baselineNext, next)
					}
				}
			})
		}
	}
}

func TestCampingSkipSleepsThroughInOneUpdateWithTheSameOrderedDecisions(t *testing.T) {
	for _, dungeon := range []bool{false, true} {
		var baseline [3]uint64
		for _, skip := range []bool{false, true} {
			state, _, scene, completions := startPacingCamp(t, dungeon)
			require.False(t, state.RequestCampingSkip(), "setup is not sleep")
			for updates := 0; updates < 20000 && state.EventQueueManager.HasActiveWorkflows(); updates++ {
				if skip && scene.IsSleeping() && scene.CurrentCycle == 3 {
					before := state.GetRNG().Calls()
					require.True(t, state.RequestCampingSkip())
					require.Equal(t, before, state.GetRNG().Calls())
					state.ElapsedMs += CampingCycleDelayMs
					state.EventQueueManager.Update()
					require.Equal(t, scene.TotalCycles, scene.CurrentCycle, "one update runs every remaining cycle")
					require.False(t, scene.IsSleeping())
					require.False(t, scene.IsSkipping())
				}
				state.ElapsedMs += 50
				state.EventQueueManager.Update()
			}
			require.Equal(t, 1, *completions)
			got := [3]uint64{
				state.GetRNG().Calls(), uint64(state.PartyState.GetCharacter(1).GetCurrentHp()),
				uint64(state.DateTime.Hour)*60 + uint64(state.DateTime.Minute),
			}
			if !skip {
				baseline = got

				continue
			}
			require.Equal(t, baseline, got, "skipping changes pacing, never the ordered camp decisions")
		}
	}
}

func startPacingCamp(t *testing.T, dungeon bool) (*GameState, *MockSystemCallbacks, *camping.CampingScene, *int) {
	t.Helper()
	if !dungeon {
		state, callbacks, scene, count := startBardCampingTest(t, 1, false)
		state.DateTime.Hour, state.DateTime.Minute = 23, 17
		return state, callbacks, scene, count
	}
	state, callbacks := newDungeonCampingTestStateWithCallbacks(t)
	state.DateTime.Hour, state.DateTime.Minute = 23, 17
	state.PartyState.GetCharacter(0).Class = party_state.Bard
	token, _ := state.BeginDungeonCommandCycle()
	state.ActionHoleUpDungeonMap(token)
	scene, plan, err := state.PrepareDungeonCampingPlan(1, 0, token)
	require.NoError(t, err)
	state.SetRandomSeed(12345)
	installAutomaticAnyKey(state)
	count := 0
	queue := state.EventQueueManager.Workflow("pacing_dungeon_test").Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().First(func() {})
	state.AddDungeonCampingSequenceToBuilder(queue, scene, plan, func() {
		count++
		state.CompleteDungeonCampingCommand(token)
	})
	queue.Start()
	t.Cleanup(state.EventQueueManager.Shutdown)
	return state, callbacks, scene, &count
}

func TestFastCampingAmbushAndCancellationClosePacingBeforeHandoff(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		state, callbacks, scene, completions := startBardCampingTest(t, 2, true)
		for range 1000 {
			state.ElapsedMs += 50
			state.EventQueueManager.Update()
			if scene.IsSleeping() {
				break
			}
			require.False(t, state.RequestCampingAcceleration(), "setup and Bard performance retain pacing")
		}
		require.True(t, state.RequestCampingAcceleration())
		if cancel {
			state.EventQueueManager.Shutdown()
			state.EventQueueManager.Shutdown()
		} else {
			runCampingWorkflowToCompletion(t, state)
			require.NotNil(t, state.pendingCampingCombat)
			require.LessOrEqual(t, scene.CurrentCycle, DebugAmbushCycle)
		}
		require.False(t, scene.IsSleeping())
		require.False(t, scene.IsAccelerated())
		require.Equal(t, 1, *completions)
		require.Equal(t, 1, callbacks.StopRequestCount(SoundBardLute))
		require.Equal(t, 1, callbacks.StopRequestCount(SoundWind))
		require.Nil(t, state.CampingScene)
	}
}

func TestFastCampingRuntimeIntervalsUseOneQuarterPresentationTime(t *testing.T) {
	durations := make([]int64, 0, 2)
	for _, fast := range []bool{false, true} {
		state, _, scene, _ := startBardCampingTest(t, 1, false)
		var firstAt, lastAt int64
		for range 2000 {
			if fast && scene.IsSleeping() {
				state.RequestCampingAcceleration()
			}
			state.ElapsedMs += 10
			state.EventQueueManager.Update()
			if firstAt == 0 && scene.CurrentCycle == 1 {
				firstAt = state.ElapsedMs
			}
			if scene.CurrentCycle == scene.TotalCycles {
				lastAt = state.ElapsedMs
				break
			}
		}
		require.Positive(t, firstAt)
		require.Greater(t, lastAt, firstAt)
		durations = append(durations, lastAt-firstAt)
	}
	require.Equal(t, durations[0], 4*durations[1], "only intervals between logical cycles are compared")
}

func TestFastEightHourWandererClosesSleepBeforeMessagesAndNextCampStartsNormally(t *testing.T) {
	state, callbacks := newCampingAmbushTestState(t)
	installAutomaticAnyKey(state)
	require.True(t, state.ActionHoleUpLargeMapWithDebug(8, NoGuardAssigned, true, false))
	state.EventQueueManager.Update()
	scene := state.CampingScene
	require.NotNil(t, scene)
	sawWanderer := false
	for update := 0; update < 10000 && state.EventQueueManager.HasActiveWorkflows(); update++ {
		if scene.IsSleeping() {
			require.True(t, state.RequestCampingAcceleration())
		}
		state.ElapsedMs += 50
		state.EventQueueManager.Update()
		if scene.WandererPresent {
			sawWanderer = true
			require.False(t, scene.IsSleeping())
			require.False(t, state.RequestCampingAcceleration())
		}
	}
	require.False(t, state.EventQueueManager.HasActiveWorkflows())
	require.True(t, sawWanderer)
	require.Equal(t, 8*camping.CyclesPerHour, scene.CurrentCycle)
	require.Equal(t, 1, callbacks.StopRequestCount(SoundWind))
	require.True(t, state.ActionHoleUpLargeMapWithDebug(1, NoGuardAssigned, false, false))
	state.EventQueueManager.Update()
	require.NotNil(t, state.CampingScene)
	require.NotSame(t, scene, state.CampingScene)
	require.False(t, state.CampingScene.IsAccelerated())
	require.False(t, state.RequestCampingAcceleration(), "second camp setup starts normally")
	state.EventQueueManager.Shutdown()
}

func TestRuntimeDelayFixedAPIStillOwnsTrailingWaitAndFinalization(t *testing.T) {
	// This core caller exercises the preserved fixed-delay API independently of
	// camping's dynamic provider, with the same injected GameState clock.
	state, _ := newCampingAmbushTestState(t)
	var cycles, continued, finalized int
	state.EventQueueManager.Workflow("fixed_delay_compatibility").Priority(event_queue.PriorityNormal).
		Sequential().First(func() {}).RepeatWithDelay(2, 200, func(int) bool {
		cycles++
		return true
	}).Wait(50).Then(func() { continued++ }).Finally(func() { finalized++ }).Start()
	start := state.ElapsedMs
	state.EventQueueManager.Update()
	state.ElapsedMs = start + 200
	state.EventQueueManager.Update()
	require.Equal(t, 1, cycles)
	state.ElapsedMs = start + 400
	state.EventQueueManager.Update()
	require.Equal(t, 2, cycles)
	require.Zero(t, continued)
	state.ElapsedMs = start + 450
	for range 3 {
		state.EventQueueManager.Update()
	}
	require.Equal(t, 1, continued)
	require.Equal(t, 1, finalized)
}
