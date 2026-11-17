package main

import (
	"fmt"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestCombatSignatureAdmissionAfterHitchRetainsFullScheduledLifetime(t *testing.T) {
	for _, kind := range []references.MonsterType{references.MonsterDragon, references.MonsterDevil} {
		t.Run(kind.Name(), func(t *testing.T) { testProductionGateTiming(t, kind) })
	}
}

func testProductionGateTiming(t *testing.T, kind references.MonsterType) {
	testProductionGateTimingAtRate(t, kind, 1)
}

func TestCombatSignatureProductionEveryRateWindTimeline(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) { testProductionGateTimingAtRate(t, references.MonsterDevil, rate) })
	}
}

func testProductionGateTimingAtRate(t *testing.T, kind references.MonsterType, rate float64) {
	scene, callbacks := newCombatUseTestScene(t)
	setCombatTimingTestRate(scene, rate)
	timing := newCombatPresentationTiming(rate)
	arrival := timing.durationMs(combatGateArrivalMs)
	materialize := timing.durationMs(combatGateMaterializeMs)
	settle := timing.durationMs(combatGateSettleMs)
	duration := arrival + materialize + settle
	state := scene.gameState
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.mapImage = ebiten.NewImage(304, 208)
	t.Cleanup(scene.mapImage.Dispose)
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Dispose)
	scene.windDisplay = &WindDisplay{}
	scene.moongateVisualSystem = NewMoongateVisualSystem(state)
	state.SystemCallbacks.Combat = game_state.NewCombatCallbacks(scene.recordCombatEvent, scene.recordCombatConclusion)
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			setCombatConcealmentTerrain(state, references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}, indexes.BrickFloor)
		}
	}
	for index, active := range state.CombatScene.PartyActive {
		if active {
			state.CombatScene.SetPartyMemberPosition(index, references.Position{X: references.Coordinate(index), Y: 10})
		}
	}
	reference := (*state.GameReferences.EnemyReferences)[kind]
	monster := game_state.NewCombatMonster(kind, &reference,
		references.Position{X: 8, Y: 1}, state.AsRandomProvider())
	index := state.CombatScene.AddMonsterToQueue(monster)
	step := nextSignatureMonsterStep(t, scene, index)
	possesses := reference.HasAbility(references.PossessCharm)
	seed := gateAfterHitchSeed(t, possesses)
	state.SetRandomSeed(seed)
	// Wind legitimately shares the gameplay stream and advances before queue
	// work. Account for it explicitly rather than claiming a seed-only replay or
	// that a longer presentation interval cannot change later gameplay rolls.
	oracleRNG := beradrng.New(seed)
	oracleWind := environment.NewWindSystem(oracleRNG, signatureWindObserver{})
	now := time.Unix(100, 0)
	scene.startTime, scene.lastEffectsUpdate, scene.now = now, now, func() time.Time { return now }
	state.ElapsedMs = 0
	scene.startScheduledMonsterTurn(step)
	setCombatTimingTestRate(scene, 4) // Deferred publication must retain the action's rate.
	scene.updateWindAndEffects()      // Schedule the authoritative action after anticipation.
	now = now.Add(700 * time.Millisecond)
	scene.updateWindAndEffects() // Publish the gate, then updateVisualEffects in the SAME frame.
	oracleWind.AdvanceThrough(700)
	if possesses {
		require.GreaterOrEqual(t, oracleRNG.Intn(32), 8, "original daemon possession probes a non-PC slot")
	}
	require.Less(t, oracleRNG.Intn(256), 32)
	gatePosition := references.Position{X: references.Coordinate(oracleRNG.Intn(16)), Y: references.Coordinate(oracleRNG.Intn(16))}
	oracleRNG.Intn(8) // Creation DEX draw after successful placement (ref:ENEMY-0003).
	require.Len(t, scene.combatSignatureEffects, 1)
	effect := scene.combatSignatureEffects[0]
	gated := state.CombatScene.MonsterManager.GetMonster(effect.Actor.EntityIndex)
	gated.SetCurrentHP(gated.MaxHP() - 1) // The battlefield marks only the injured.
	assert.Equal(t, duration, effect.DurationMs)
	require.Equal(t, game_state.CombatEventGating, effect.Kind)
	assert.Equal(t, gatePosition, effect.To)
	assert.Equal(t, oracleRNG.Calls(), state.GetRNG().Calls(), "only due wind ticks and committed gate creation consumed RNG")
	assert.Equal(t, int64(700), effect.StartedMs)
	assert.Zero(t, effect.progress(state.ElapsedMs))
	assert.True(t, effect.Dissolve.IsActive())
	require.True(t, callbacks.InputBlocked)
	// Pump the complete production ordering repeatedly at each fixed clock value.
	for _, ageMs := range []int64{0, arrival - 1, arrival, arrival + materialize/10, arrival + materialize/2, arrival + materialize - 1, arrival + materialize, duration - 1} {
		now = scene.startTime.Add(time.Duration(700+ageMs) * time.Millisecond)
		oracleWind.AdvanceThrough(700 + ageMs)
		for range 8 {
			scene.updateWindAndEffects()
			assert.Equal(t, oracleRNG.Calls(), state.GetRNG().Calls(), "effect progress and repeated same-time updates consume no extra RNG")
			assert.Equal(t, oracleWind.GetCurrentWind(), state.GetCurrentWind())
			require.True(t, state.EventQueueManager.HasWorkflow("monster_turn_delay"))
			require.True(t, callbacks.InputBlocked)
			require.Equal(t, step.Actor, state.CombatScene.GetCurrentTurnIdentity())
			require.True(t, state.CombatScene.HasScheduledCombatStep())
			_, err := state.ResolveScheduledCombatStep(step.Token)
			require.ErrorIs(t, err, game_state.ErrCombatPresentationDuplicate, "the exact resolved token is still pending")
			require.Len(t, scene.combatSignatureEffects, 1)
		}
		screen.Clear()
		scene.drawCombatMonsterOverlay(screen)
		assert.Zero(t, opaquePixelCount(combatSignatureRegion(scene, screen, effect.To)), "no ordinary actor under the real %s gate", kind.Name())
		scene.drawCombatSignatureEffects(screen)
		assert.Positive(t, opaquePixelCount(combatSignatureRegion(scene, screen, effect.To)))
		screen.Clear()
		scene.drawAvatarOverlay(screen)
		if ageMs < arrival {
			assert.Zero(t, countExactColor(combatSignatureRegion(scene, screen, effect.To), ucolor.CombatAllegianceEnemy), "foreground health frames must not disclose the hidden prelude")
		} else {
			assert.Positive(t, countExactColor(combatSignatureRegion(scene, screen, effect.To), ucolor.CombatAllegianceEnemy))
		}
		if ageMs == arrival+materialize/10 || ageMs == arrival+materialize/2 {
			assert.Positive(t, opaquePixelCount(effect.Highlight), "real %s gate has leading silhouette fragments", kind.Name())
		}
		captureCombatPolishFrame(t, scene, kind.Name(), ageMs)
	}
	// The workflow polls at its declared interval. It cannot release early, and
	// completes at the first polling boundary at/after the effect's own deadline.
	now = scene.startTime.Add(time.Duration(700+duration) * time.Millisecond)
	scene.updateWindAndEffects() // Effects complete AFTER this frame's workflow poll.
	now = now.Add(DialogPollingRateMs * time.Millisecond)
	oracleWind.AdvanceThrough(700 + duration + DialogPollingRateMs)
	for range 8 {
		scene.updateWindAndEffects()
	}
	assert.False(t, state.EventQueueManager.HasWorkflow("monster_turn_delay"))
	assert.False(t, callbacks.InputBlocked)
	assert.Empty(t, scene.combatSignatureEffects)
	assert.Nil(t, effect.Frame)
	assert.False(t, effect.Dissolve.IsActive())
	assert.ErrorIs(t, state.CompleteScheduledCombatStep(step.Token), game_state.ErrCombatPresentationDuplicate)
	assert.Equal(t, oracleRNG.Calls(), state.GetRNG().Calls())
	assert.Equal(t, oracleRNG.Intn(1000000), state.GetRNG().Intn(1000000), "the full update/queue/effect ordering preserves the next gameplay value")
	gateSounds := 0
	for _, sound := range callbacks.SoundEffectsPlayed {
		if sound == game_state.SoundMonsterGate {
			gateSounds++
		}
	}
	assert.Equal(t, 1, gateSounds, "one request at materialization onset, independent of redraws")
}

type signatureWindObserver struct{}

func (signatureWindObserver) ShowWindUI(environment.WindDirection) {}
func (signatureWindObserver) AddRowStr(string)                     {}

func gateAfterHitchSeed(t *testing.T, possesses bool) uint64 {
	t.Helper()
	// Choose a deterministic fixture with no intervening wind changes and a
	// successful central gate. Production still processes every elapsed wind tick.
	for seed := uint64(0); seed < 10000; seed++ {
		rng := beradrng.New(seed)
		noWindChange := true
		for tick := environment.WindTickIntervalMs; tick <= 700; tick += environment.WindTickIntervalMs {
			if rng.Intn(64) == 0 {
				noWindChange = false
				break
			}
		}
		if !noWindChange || (possesses && rng.Intn(32) < 8) || rng.Intn(256) >= 32 {
			continue
		}
		x, y := rng.Intn(16), rng.Intn(16)
		if x >= 2 && x <= 7 && y >= 2 && y <= 7 {
			return seed
		}
	}
	t.Fatal("no gate-after-hitch seed")

	return 0
}
