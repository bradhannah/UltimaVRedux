package main

import (
	"fmt"
	"math"
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// combatTimingTestRates lists the animated rates; Ludicrous has its own tests.
func combatTimingTestRates() []float64 {
	return slices.DeleteFunc(config.CombatSpeedSpec().Choices(), func(rate float64) bool {
		return rate == config.CombatSpeedLudicrous
	})
}

func setCombatTimingTestRate(scene *GameScene, rate float64) {
	if scene.gameConfig == nil {
		scene.gameConfig = &config.UltimaVConfiguration{}
	}
	if scene.gameConfig.GameplayConfiguration == nil {
		scene.gameConfig.GameplayConfiguration = config.DefaultGameplayConfiguration()
	}
	scene.gameConfig.GameplayConfiguration.CombatSpeedFactor = rate
}

func TestCombatTimingAuthoredBudgetsEveryRate(t *testing.T) {
	bases := [...]int64{250, 1000, 650, 333, 75, 175, 1250, 100}
	wants := [...][8]int64{
		{500, 2000, 1300, 666, 150, 350, 2500, 200},
		{250, 1000, 650, 333, 75, 175, 1250, 100},
		{167, 667, 433, 222, 50, 117, 833, 67},
		{125, 500, 325, 167, 38, 88, 625, 50},
		{83, 333, 217, 111, 25, 58, 417, 33},
		{63, 250, 163, 83, 19, 44, 313, 25},
		{42, 167, 108, 56, 13, 29, 208, 17},
		{31, 125, 81, 42, 9, 22, 156, 13},
		{21, 83, 54, 28, 6, 15, 104, 8},
		{16, 63, 41, 21, 5, 11, 78, 6},
		{13, 50, 33, 17, 4, 9, 63, 5},
	}
	require.Len(t, combatTimingTestRates(), len(wants))
	for index, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			timing := newCombatPresentationTiming(rate)
			for column, base := range bases {
				assert.Equal(t, wants[index][column], timing.durationMs(base))
				assert.Equal(t, uint64(wants[index][column]), timing.duration(uint64(base)))
			}
			assert.Zero(t, timing.duration(0))
			assert.Zero(t, timing.durationMs(0))
			assert.Equal(t, int64(-1), timing.durationMs(-1), "signed sentinels are not positive work")
			assert.GreaterOrEqual(t, timing.duration(1), uint64(1))
		})
	}
	assert.Equal(t, int64(333), newCombatPresentationTiming(0).durationMs(333), "zero-valued fixture policy is intentionally 1x")
	for _, rate := range []float64{-1, 0.25, 0.75, 2.5, 4.5, 10, 20.5, math.NaN(), math.Inf(1)} {
		assert.Panics(t, func() { newCombatPresentationTiming(rate) }, "invalid explicit rates are programming errors")
	}
	assert.Equal(t, uint64(math.MaxUint64), newCombatPresentationTiming(0.5).duration(math.MaxUint64))
	assert.Equal(t, int64(math.MaxInt64), newCombatPresentationTiming(0.5).durationMs(math.MaxInt64))
}

func TestCombatImpactLabelBudgetScalesAboveReadableFloor(t *testing.T) {
	for _, test := range []struct{ rate, want float64 }{{0.5, 2.5}, {1, 1.25}, {2, 0.625}, {4, 0.313}, {8, 0.3}, {20, 0.3}} {
		assert.InDelta(t, test.want, newCombatPresentationTiming(test.rate).impactLabelSeconds(), 1e-9, "%gx", test.rate)
	}
	assert.InDelta(t, 1.25, combatPresentationTiming{}.impactLabelSeconds(), 1e-9, "zero policy is authored 1x")
}

func TestCombatTimingSynchronousScopeRestoresAndDoesNotScaleSharedClocks(t *testing.T) {
	scene := &GameScene{}
	setCombatTimingTestRate(scene, 2)
	scene.gameConfig.DebugConfiguration = &config.DebugConfiguration{CombatMovementSpeedFactor: 4}
	assert.Equal(t, int64(50), scene.captureCombatTiming().durationMs(100), "legacy debug factor must not multiply the master")
	scene.withCombatTiming(newCombatPresentationTiming(4), func() {
		assert.Equal(t, int64(25), scene.captureCombatTiming().durationMs(100))
		assert.Equal(t, int64(100), scene.capturePresentationTiming(false).durationMs(100))
		scene.withCombatTiming(newCombatPresentationTiming(0.5), func() {
			assert.Equal(t, int64(200), scene.captureCombatTiming().durationMs(100))
		})
		assert.Equal(t, int64(25), scene.captureCombatTiming().durationMs(100))
	})
	assert.Nil(t, scene.combatResolutionTiming)
	assert.Panics(t, func() {
		scene.withCombatTiming(newCombatPresentationTiming(4), func() { panic("resolution failure") })
	})
	assert.Nil(t, scene.combatResolutionTiming)
	assert.Equal(t, int64(50), scene.captureCombatTiming().durationMs(100))
}

func TestCombatTimingScheduledMovementEveryRateSnapshot(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		for _, kind := range []references.MonsterType{references.MonsterOrc, references.MonsterBat, references.MonsterMongbat} {
			t.Run(fmt.Sprintf("%g/%s", rate, kind.Name()), func(t *testing.T) {
				scene, callbacks, step := newScheduledProjectileTestScene(t, kind, 6)
				setCombatTimingTestRate(scene, rate)
				state := scene.gameState
				t.Cleanup(state.EventQueueManager.Shutdown)
				state.SetRandomSeed(42)
				anticipation, movement := int64(250), int64(650)
				if kind == references.MonsterBat || kind == references.MonsterMongbat {
					anticipation, movement = 75, 175
				}
				timing := newCombatPresentationTiming(rate)
				anticipation, movement = timing.durationMs(anticipation), timing.durationMs(movement)
				state.ElapsedMs = 0
				calls := state.GetRNG().Calls()
				scene.startScheduledMonsterTurn(step)
				setCombatTimingTestRate(scene, 4)
				drainCombatPacingAt(scene, 0)
				drainCombatPacingAt(scene, anticipation-1)
				require.False(t, scene.combatMovement.Active)
				assert.Equal(t, calls, state.GetRNG().Calls(), "queue-only anticipation must not preroll AI")
				drainCombatPacingAt(scene, anticipation)
				require.True(t, scene.combatMovement.Active)
				drainCombatPacingAt(scene, anticipation+1)
				assert.InDelta(t, 1/float64(movement), scene.combatMovement.Progress, 0.00001)
				drainCombatPacingAt(scene, anticipation+movement-1)
				require.True(t, callbacks.InputBlocked)
				drainCombatPacingAt(scene, anticipation+movement)
				assert.False(t, callbacks.InputBlocked)
				assert.False(t, scene.combatMovement.Active)
				after := snapshotBlockedCombatState(t, state)
				drainCombatPacingAt(scene, anticipation+movement+1)
				after.ElapsedMs++
				assert.Equal(t, after, snapshotBlockedCombatState(t, state))
				assert.ErrorIs(t, state.CompleteScheduledCombatStep(step.Token), game_state.ErrCombatPresentationDuplicate)
			})
		}
	}
}

func TestCombatTimingAutoAttackHoldKeepsLabelsReadableEveryRate(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			scene, actor := setupCombatAutoAttackInputTest(t)
			setCombatTimingTestRate(scene, rate)
			state := scene.gameState
			t.Cleanup(state.EventQueueManager.Shutdown)
			start := state.ElapsedMs
			hold := newCombatPresentationTiming(rate).durationMs(combatAutoAttackHoldDurationMs)
			scene.combatMapInputHandler(ebiten.KeyTab)
			setCombatTimingTestRate(scene, 4)
			drainCombatPacingAt(scene, start)
			require.Len(t, scene.combatImpactEffects, 1)
			first := scene.combatImpactEffects[0]
			drainCombatPacingAt(scene, start+hold-1)
			require.Len(t, scene.combatImpactEffects, 1)
			assert.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())
			drainCombatPacingAt(scene, start+hold)
			require.Len(t, scene.combatImpactEffects, 2, "second source uses the captured action hold")
			assert.Equal(t, first.timing, scene.combatImpactEffects[1].timing)
			drainCombatPacingAt(scene, start+2*hold-1)
			assert.True(t, scene.inputBlocked)
			drainCombatPacingAt(scene, start+2*hold)
			assert.False(t, scene.inputBlocked)
			// Independently advance label age: the label budget follows the
			// captured rate above its floor, however many sources have advanced.
			scene.combatImpactEffects = nil
			scene.showCombatImpact(first.Feedback)
			require.Len(t, scene.combatImpactEffects, 1)
			labelMs := max(scene.combatImpactEffects[0].timing.durationMs(combatImpactDurationMs), combatImpactLabelFloorMs)
			state.ElapsedMs += labelMs - 1
			scene.updateCombatImpactEffects(0)
			require.Len(t, scene.combatImpactEffects, 1)
			state.ElapsedMs++
			scene.updateCombatImpactEffects(0)
			assert.Empty(t, scene.combatImpactEffects)
		})
	}
}

func TestCombatTimingReturningSourcesUseOneSnapshotAndBaseFlightFloor(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			scene, actor := setupCombatAutoAttackInputTest(t)
			setCombatTimingTestRate(scene, rate)
			state := scene.gameState
			t.Cleanup(state.EventQueueManager.Shutdown)
			character := state.PartyState.GetCharacter(actor.EntityIndex)
			character.SetEquippedItem(references.SlotWeapon, references.MagicAxe)
			character.SetEquippedItem(references.SlotShield, references.MagicAxe)
			launches, impacts := 0, 0
			state.SystemCallbacks.Visual.StartProjectile = func(_, _ references.Position, missile references.MissileType) uint64 {
				launches++
				return CalculateProjectileDuration(0, 0, 1, 0, missile, references.CombatLocation)
			}
			state.SystemCallbacks.Combat.PublishEvent = func(event game_state.CombatEvent) {
				if event.Kind == game_state.CombatEventRangedAttack {
					impacts++
				}
			}
			start := state.ElapsedMs
			duration := newCombatPresentationTiming(rate).durationMs(100)
			scene.combatMapInputHandler(ebiten.KeyTab)
			drainCombatPacingAt(scene, start)
			require.Equal(t, 1, launches)
			setCombatTimingTestRate(scene, 0.5)
			for leg := int64(1); leg <= 4; leg++ {
				drainCombatPacingAt(scene, start+leg*duration-1)
				assert.Equal(t, int(leg), launches)
				assert.Equal(t, int(leg/2), impacts)
				assert.True(t, scene.inputBlocked)
				drainCombatPacingAt(scene, start+leg*duration)
			}
			assert.Equal(t, 4, launches)
			assert.Equal(t, 2, impacts)
			assert.False(t, scene.inputBlocked)
			assert.Nil(t, state.CombatScene.AttackTargeting)
		})
	}
}

func TestCombatTimingFieldLeadingWaitRemainsIneffective(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			scene, _ := newCombatUseTestScene(t)
			timing := newCombatPresentationTiming(rate)
			finished := 0
			// Exact dynamic insertion shape used after field-spell arrival. A lone
			// leading Wait has no action node to anchor; adding one changes 1x.
			scene.gameState.EventQueueManager.Workflow("field_leading_wait_boundary").
				Priority(event_queue.PriorityHigh).Sequential().First(func() {}).
				ThenSequence(func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
					return q.Wait(timing.durationMs(combatFieldCreationHoldDurationMs))
				}).Finally(func() { finished++ }).Start()
			drainCombatPacingAt(scene, scene.gameState.ElapsedMs)
			assert.Equal(t, 1, finished, "documented existing delay must not become a new baseline hold")
			scene.gameState.EventQueueManager.Shutdown()
			assert.Equal(t, 1, finished)
		})
	}
}

func TestCombatTimingPreemptedSequenceRetainsSnapshotWithoutLeakingContext(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	setCombatTimingTestRate(scene, 2)
	state := scene.gameState
	t.Cleanup(state.EventQueueManager.Shutdown)
	state.ElapsedMs = 0
	launches, impacts, finalized := 0, 0, 0
	state.SystemCallbacks.Visual.StartProjectile = func(_, _ references.Position, _ references.MissileType) uint64 {
		launches++
		return 100
	}
	state.EventQueueManager.Workflow("preempted_timed_flight").Priority(event_queue.PriorityNormal).
		Sequential().BlockInput().First(func() {}).Wait(50).
		ThenSequence(scene.combatProjectileAnimationSequenceWithTiming(scene.captureCombatTiming(), game_state.CombatProjectileLaunch{Animate: true}, func() {
			impacts++
			assert.Equal(t, int64(50), scene.captureCombatTiming().durationMs(100))
		})).Finally(func() { finalized++ }).Start()
	drainCombatPacingAt(scene, 0)
	drainCombatPacingAt(scene, 10)
	setCombatTimingTestRate(scene, 4)
	state.EventQueueManager.Workflow("timing_preemption").Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().First(func() {
		assert.Equal(t, int64(25), scene.captureCombatTiming().durationMs(100), "unrelated work sees its own rate")
	}).Wait(200).Then(func() {}).Start()
	drainCombatPacingAt(scene, 10)
	drainCombatPacingAt(scene, 210)
	drainCombatPacingAt(scene, 249)
	assert.Zero(t, launches)
	drainCombatPacingAt(scene, 250)
	require.Equal(t, 1, launches)
	drainCombatPacingAt(scene, 299)
	assert.Zero(t, impacts)
	assert.True(t, callbacks.InputBlocked)
	drainCombatPacingAt(scene, 300)
	assert.Equal(t, 1, impacts)
	assert.Equal(t, 1, finalized)
	assert.False(t, callbacks.InputBlocked)
	assert.Nil(t, scene.combatResolutionTiming)
	state.EventQueueManager.Shutdown()
	assert.Equal(t, 1, finalized)
}

func TestCombatTimingVictoryDepartureSnapshotEveryRate(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			scene, callbacks := newCombatUseTestScene(t)
			setCombatTimingTestRate(scene, rate)
			state := scene.gameState
			t.Cleanup(state.EventQueueManager.Shutdown)
			state.CombatScene.Victory = true
			members := state.ActiveCombatPartyMembers()
			require.NotEmpty(t, members)
			start := state.ElapsedMs
			duration := newCombatPresentationTiming(rate).durationMs(combatVictoryExitDelayMs)
			scene.startCombatVictoryExitWorkflow()
			setCombatTimingTestRate(scene, 4)
			drainCombatPacingAt(scene, start)
			for index := range members {
				drainCombatPacingAt(scene, start+int64(index+1)*duration-1)
				require.NotNil(t, state.CombatScene)
				assert.Len(t, state.ActiveCombatPartyMembers(), len(members)-index-1)
				assert.True(t, callbacks.InputBlocked)
				drainCombatPacingAt(scene, start+int64(index+1)*duration)
			}
			assert.Nil(t, state.CombatScene)
			assert.False(t, callbacks.InputBlocked)
			assert.False(t, state.EventQueueManager.HasWorkflow("victory_exit_workflow"))
		})
	}
}

func TestCombatTimingManualSourcesRetainSnapshotUntilActionEnds(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	setCombatTimingTestRate(scene, 2)
	state := scene.gameState
	require.True(t, state.BeginCombatAttack())
	targeting := state.CombatScene.AttackTargeting
	target := state.CombatScene.MonsterManager.GetMonster(0).Position()
	// Admission is through the same keyboard-confirmation adapter as a manual A.
	targeting.Cursor = target
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	scene.handleCombatAttackConfirmation(targeting)
	require.Len(t, scene.combatImpactEffects, 1)
	require.NotNil(t, state.CombatScene.AttackTargeting)
	setCombatTimingTestRate(scene, 4)
	targeting.Cursor = target
	scene.keyboard.ResetReleaseDebouncing()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	scene.handleCombatAttackConfirmation(targeting)
	require.Len(t, scene.combatImpactEffects, 2)
	assert.Equal(t, scene.combatImpactEffects[0].motionDurations(), scene.combatImpactEffects[1].motionDurations())
	assert.Equal(t, 0.16, scene.combatImpactEffects[1].motionDurations().jolt)
	assert.Nil(t, state.CombatScene.AttackTargeting)
	assert.Nil(t, scene.combatAttackTiming.targeting)
	assert.NotEqual(t, actor, state.CombatScene.GetCurrentTurnIdentity())
}

func TestCombatTimingVictoryCancellationCannotEndReplacementEncounter(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	setCombatTimingTestRate(scene, 4)
	state := scene.gameState
	state.CombatScene.Victory = true
	scene.startCombatVictoryExitWorkflow()
	replacement := *state.CombatScene
	state.CombatScene = &replacement
	members := state.ActiveCombatPartyMembers()
	state.EventQueueManager.Shutdown()
	assert.Same(t, &replacement, state.CombatScene)
	assert.Equal(t, members, state.ActiveCombatPartyMembers())
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
	state.EventQueueManager.Shutdown()
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
}
