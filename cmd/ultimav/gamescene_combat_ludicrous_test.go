package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestLudicrousTimingSkipsMotionAndPausesButKeepsEffectsAt20x(t *testing.T) {
	ludicrous := newCombatPresentationTiming(config.CombatSpeedLudicrous)
	fastest := newCombatPresentationTiming(config.CombatSpeedFastestAnimated)
	assert.False(t, ludicrous.animatesMotion())
	assert.True(t, fastest.animatesMotion())
	assert.True(t, combatPresentationTiming{}.animatesMotion(), "zero policy is authored 1x")
	for _, base := range []int64{100, 250, 1000, 1250} {
		assert.Equal(t, fastest.durationMs(base), ludicrous.durationMs(base), "effects keep 20x")
		assert.Equal(t, fastest.durationMs(base), fastest.pauseMs(base))
		assert.Zero(t, ludicrous.pauseMs(base))
	}
}

// playCombatFrames advances 60 TPS frames until enemyTurns scheduled enemy
// turns have started; party members pass at once. It returns each turn's
// starting frame, and checks every frame with inspect.
func playCombatFrames(
	t *testing.T, scene *GameScene, callbacks *game_state.MockSystemCallbacks, enemyTurns int, inspect func(),
) []int {
	t.Helper()
	state := scene.gameState
	startMs := state.ElapsedMs
	starts := make([]int, 0, enemyTurns)
	for frame := 0; len(starts) < enemyTurns; frame++ {
		require.Less(t, frame, enemyTurns*120, "combat stalled")
		state.ElapsedMs = startMs + int64(frame)*1000/60
		state.EventQueueManager.Update()
		for range 16 {
			if callbacks.InputBlocked || state.EventQueueManager.HasActiveWorkflows() {
				break
			}
			require.NotNil(t, state.CombatScene, "the fixture fight must not end")
			if scene.reviewScheduledCombatStep() {
				starts = append(starts, frame)

				break
			}
			state.FinishCombatAction()
		}
		inspect()
	}

	return starts
}

func TestLudicrousCombatSpeedRunsOneEnemyTurnPerFrameWithTheSameOutcome(t *testing.T) {
	// Trolls throw axes once in range, so missile flight is exercised too.
	kinds := []references.MonsterType{references.MonsterOrc, references.MonsterTroll}
	animated, animatedCallbacks := newCombatPacingSwarm(t, kinds)
	ludicrous, ludicrousCallbacks := newCombatPacingSwarm(t, kinds)
	t.Cleanup(animated.gameState.EventQueueManager.Shutdown)
	t.Cleanup(ludicrous.gameState.EventQueueManager.Shutdown)
	setCombatTimingTestRate(animated, config.CombatSpeedFastestAnimated)
	setCombatTimingTestRate(ludicrous, config.CombatSpeedLudicrous)

	const enemyTurns = 48
	glided := false
	animatedStarts := playCombatFrames(t, animated, animatedCallbacks, enemyTurns, func() {
		glided = glided || animated.combatMovement.Active
	})
	require.True(t, glided, "the 20x fight must show glides")
	require.NotEmpty(t, animatedCallbacks.ProjectilesStarted, "the 20x fight must show a thrown axe")
	sounds := 0
	ludicrousStarts := playCombatFrames(t, ludicrous, ludicrousCallbacks, enemyTurns, func() {
		require.False(t, ludicrous.combatMovement.Active, "Ludicrous never glides")
		played := ludicrousCallbacks.SoundEffectsPlayed[sounds:]
		for index, effect := range played {
			assert.NotContains(t, played[:index], effect, "at most one of each sound per frame")
		}
		sounds = len(ludicrousCallbacks.SoundEffectsPlayed)
	})

	for index := 1; index < len(ludicrousStarts); index++ {
		require.Equal(t, 1, ludicrousStarts[index]-ludicrousStarts[index-1], "enemy turn %d", index)
	}
	assert.Greater(t, animatedStarts[len(animatedStarts)-1], 2*ludicrousStarts[len(ludicrousStarts)-1])
	assert.Empty(t, ludicrousCallbacks.ProjectilesStarted, "Ludicrous never draws missile flight")
	assert.NotEmpty(t, ludicrousCallbacks.CombatEvents)
	assert.Equal(t, animatedCallbacks.CombatEvents, ludicrousCallbacks.CombatEvents)
	assertCombatPacingBoundaryEquivalent(t, animated, ludicrous)
}
