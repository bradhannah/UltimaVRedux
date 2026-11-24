package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestCombatTimingConePropagationHoldAndCancellationEveryRate(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		for _, cancel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%g/cancel=%t", rate, cancel), func(t *testing.T) {
				scene, callbacks, _ := newAreaSpellOutcomeScene(t, references.InZu, references.MonsterSnake)
				setCombatTimingTestRate(scene, rate)
				state := scene.gameState
				t.Cleanup(state.EventQueueManager.Shutdown)
				caster := state.CombatScene.GetActiveCombatantIndex()
				require.True(t, state.BeginCombatConeTargeting(game_state.CastRequest{Spell: references.InZu, CasterIdx: caster}).Valid)
				state.SetCombatConeDirection(references.Up)
				result := state.ConfirmCombatConeTarget()
				require.Equal(t, game_state.CombatConeTargetResolved, result.Outcome)
				timing := newCombatPresentationTiming(rate)
				start := state.ElapsedMs
				scene.startCombatConeSpellEffect(result)
				setCombatTimingTestRate(scene, 1)
				drainCombatPacingAt(scene, start)
				calls := state.GetRNG().Calls()
				drainCombatPacingAt(scene, start+timing.durationMs(450)-1)
				require.NotNil(t, scene.combatConeCast)
				assert.Less(t, scene.combatConeCast.progress, float32(1))
				assert.Equal(t, calls, state.GetRNG().Calls(), "presentation does not resolve early")
				if cancel {
					state.EventQueueManager.Shutdown()
					assert.Nil(t, scene.combatConeCast)
					assert.False(t, callbacks.InputBlocked)
					return
				}
				drainCombatPacingAt(scene, start+timing.durationMs(450))
				require.NotNil(t, scene.combatConeCast)
				assert.Equal(t, float32(1), scene.combatConeCast.progress)
				drainCombatPacingAt(scene, start+timing.durationMs(450)+timing.durationMs(150)-1)
				require.NotNil(t, scene.combatConeCast)
				assert.Equal(t, calls, state.GetRNG().Calls())
				drainCombatPacingAt(scene, start+timing.durationMs(450)+timing.durationMs(150))
				assert.Nil(t, scene.combatConeCast)
				assert.False(t, callbacks.InputBlocked)
				assert.False(t, state.EventQueueManager.HasWorkflow("combat_cone_spell_effect"))
			})
		}
	}
}

func TestCombatTimingSharedPulseExplicitScopeAndSnapshot(t *testing.T) {
	for _, combat := range []bool{false, true} {
		for _, rate := range combatTimingTestRates() {
			t.Run(fmt.Sprintf("combat=%t/%g", combat, rate), func(t *testing.T) {
				scene, _ := newCombatUseTestScene(t)
				if !combat {
					scene.gameState.TransitionFromCombatMap()
				}
				setCombatTimingTestRate(scene, rate)
				state := scene.gameState
				t.Cleanup(state.EventQueueManager.Shutdown)
				intensity := 0.0
				state.SystemCallbacks.Visual.SetScreenFlashIntensity = func(value float64) { intensity = value }
				completed := false
				state.EventQueueManager.Workflow("shared_pulse_timing").Priority(event_queue.PriorityNormal).
					Sequential().First(func() {}).ThenSequence(scene.thenScreenPulseWithTiming(scene.capturePresentationTiming(combat), 1, 1, 1, 0.4)).
					Finally(func() { completed = true }).Start()
				setCombatTimingTestRate(scene, 4)
				duration := int64(180)
				if combat {
					duration = newCombatPresentationTiming(rate).durationMs(duration)
				}
				start := state.ElapsedMs
				drainCombatPacingAt(scene, start)
				drainCombatPacingAt(scene, start+duration/2)
				// At 12x and above each pulse segment rounds to a few whole milliseconds.
				assert.InDelta(t, 0.4, intensity, 0.01)
				drainCombatPacingAt(scene, start+duration-1)
				assert.False(t, completed)
				drainCombatPacingAt(scene, start+duration)
				assert.True(t, completed)
				assert.Zero(t, intensity)
			})
		}
	}
}

func TestCombatTimingScrollInversionDeadlineMatchesWait(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			scene, _ := newCombatUseTestScene(t)
			setCombatTimingTestRate(scene, rate)
			state := scene.gameState
			t.Cleanup(state.EventQueueManager.Shutdown)
			state.PartyState.Inventory.Scrolls.Set(references.ScrollVasLor, 1)
			var invertDuration int
			state.SystemCallbacks.Visual.SetScreenInvert = func(_ float64, duration int) { invertDuration = duration }
			committed, finished := false, false
			state.EventQueueManager.Workflow("scroll_timing").Priority(event_queue.PriorityHigh).
				Sequential().First(func() {}).ThenSequence(scene.thenUseCombatScroll(references.ScrollVasLor, func() bool { return true }, &committed)).
				Finally(func() { finished = true }).Start()
			start := state.ElapsedMs
			drainCombatPacingAt(scene, start)
			require.True(t, committed)
			duration := newCombatPresentationTiming(rate).durationMs(300)
			assert.Equal(t, int(duration), invertDuration)
			setCombatTimingTestRate(scene, 4)
			drainCombatPacingAt(scene, start+duration-1)
			assert.False(t, finished)
			drainCombatPacingAt(scene, start+duration)
			assert.True(t, finished)
			assert.Zero(t, state.PartyState.Inventory.Scrolls.Get(references.ScrollVasLor))
		})
	}
}

func TestCombatTimingLootCadenceAndAcknowledgementRemainSeparate(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		t.Run(fmt.Sprint(rate), func(t *testing.T) {
			scene, callbacks := newCombatUseTestScene(t)
			setCombatTimingTestRate(scene, rate)
			scene.spriteSheet = sharedPresentationTestSpriteSheet()
			state := scene.gameState
			t.Cleanup(state.EventQueueManager.Shutdown)
			token := state.FinishCombatTurnDeferred()
			start := state.ElapsedMs
			scene.startCombatChestRevealWorkflow(game_state.CombatOpenResult{RevealItems: []game_state.CombatLootReveal{
				{Sprite: indexes.ItemGem, Description: "a gem!"}, {Sprite: indexes.ItemGem, Description: "another gem!"},
			}}, token)
			setCombatTimingTestRate(scene, 4)
			duration := newCombatPresentationTiming(rate).durationMs(500)
			drainCombatPacingAt(scene, start)
			drainCombatPacingAt(scene, start+duration-1)
			assert.Equal(t, indexes.NoSprites, scene.combatLootPreview.sprite)
			drainCombatPacingAt(scene, start+duration)
			assert.Equal(t, indexes.SpriteIndex(indexes.ItemGem), scene.combatLootPreview.sprite)
			drainCombatPacingAt(scene, start+duration*2)
			drainCombatPacingAt(scene, start+duration*2+10000)
			assert.True(t, callbacks.InputBlocked, "finished reveal still requires player acknowledgement")
			assert.Equal(t, 1, scene.dialogStack.Count())
			state.EventQueueManager.Shutdown()
			assert.Nil(t, scene.combatLootPreview)
			assert.Zero(t, scene.dialogStack.Count())
			assert.False(t, callbacks.InputBlocked)
			assert.ErrorIs(t, state.CompleteScheduledCombatStep(token), game_state.ErrCombatPresentationDuplicate)
		})
	}
}

func TestCombatTimingPlayerTeleportPhasesAndCancellation(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		for _, cancel := range []bool{false, true} {
			t.Run(fmt.Sprintf("%g/cancel=%t", rate, cancel), func(t *testing.T) {
				state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
					WithSystemCallbacks().WithRandomSeed(1).Build()
				require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
				t.Cleanup(state.EventQueueManager.Shutdown)
				scene := &GameScene{gameState: state}
				setCombatTimingTestRate(scene, rate)
				caster := state.CombatScene.GetActiveCombatantIndex()
				from := state.CombatScene.GetPartyMemberPosition(caster)
				finished := 0
				state.EventQueueManager.Workflow("timed_teleport").Priority(event_queue.PriorityHigh).
					Sequential().BlockInput().First(func() {}).ThenSequence(scene.thenCastCombatInPor(caster)).
					Finally(func() { scene.clearCombatTeleportEffect(); finished++ }).Start()
				setCombatTimingTestRate(scene, 1)
				at := state.ElapsedMs
				drainCombatPacingAt(scene, at)
				require.NotNil(t, scene.combatTeleportEffect)
				assert.Equal(t, from, state.CombatScene.GetPartyMemberPosition(caster))
				for phase, base := range []int64{180, 80, 200} {
					duration := newCombatPresentationTiming(rate).durationMs(base)
					drainCombatPacingAt(scene, at+duration-1)
					require.NotNil(t, scene.combatTeleportEffect)
					assert.Equal(t, combatTeleportPhase(phase), scene.combatTeleportEffect.Phase)
					if phase == 1 && cancel {
						position := state.CombatScene.GetPartyMemberPosition(caster)
						state.EventQueueManager.Shutdown()
						assert.Equal(t, position, state.CombatScene.GetPartyMemberPosition(caster), "cleanup cannot teleport again")
						break
					}
					at += duration
					drainCombatPacingAt(scene, at)
				}
				assert.Nil(t, scene.combatTeleportEffect)
				assert.Equal(t, 1, finished)
				assert.False(t, callbacks.InputBlocked)
				require.NotEmpty(t, callbacks.ScreenFlashIntensities)
				assert.Zero(t, callbacks.ScreenFlashIntensities[len(callbacks.ScreenFlashIntensities)-1])
			})
		}
	}
}

func TestCombatTimingWorldProjectileAndKapowKeepAuthoredPolicy(t *testing.T) {
	scene, _ := newCombatUseTestScene(t)
	setCombatTimingTestRate(scene, 4)
	state := scene.gameState
	state.TransitionFromCombatMap()
	position := state.MapState.PlayerLocation.Position
	scene.playKapowAtPosition(position)
	require.Len(t, scene.visualEffects, 1)
	assert.Equal(t, 0.5, scene.visualEffects[0].Duration)
	impacts := 0
	state.SystemCallbacks.Visual.StartProjectile = func(_, _ references.Position, _ references.MissileType) uint64 { return 100 }
	state.EventQueueManager.Workflow("world_projectile_timing").Priority(event_queue.PriorityHigh).
		Sequential().First(func() {}).ThenSequence(state.ProjectileAnimationSequence(position, position, references.MissileCannonBall,
		func() { impacts++ })).Start()
	start := state.ElapsedMs
	drainCombatPacingAt(scene, start)
	drainCombatPacingAt(scene, start+99)
	assert.Zero(t, impacts)
	drainCombatPacingAt(scene, start+100)
	assert.Equal(t, 1, impacts)
}

func TestCombatTimingAutomaticSpellAnimationsEveryRate(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		for _, spell := range []references.Spell{references.WisQuas, references.InVasPorYlem, references.AnTym} {
			t.Run(fmt.Sprintf("%g/%s", rate, references.GetSpellName(spell)), func(t *testing.T) {
				scene, callbacks := newCombatUseTestScene(t)
				setCombatTimingTestRate(scene, rate)
				state := scene.gameState
				t.Cleanup(state.EventQueueManager.Shutdown)
				var sequence event_queue.Sequence
				var base int64
				switch spell {
				case references.WisQuas:
					base, sequence = 600, scene.thenCastCombatWisQuas()
				case references.InVasPorYlem:
					base, sequence = 1000, scene.thenCastCombatInVasPorYlem(state.CombatScene.GetActiveCombatantIndex())
				case references.AnTym:
					base, sequence = 180, scene.thenCastAnTym()
				}
				finished := 0
				state.EventQueueManager.Workflow("automatic_spell_timing").Priority(event_queue.PriorityHigh).
					Sequential().BlockInput().First(func() {}).ThenSequence(sequence).
					Finally(func() { finished++ }).Start()
				setCombatTimingTestRate(scene, 4)
				start := state.ElapsedMs
				duration := newCombatPresentationTiming(rate).durationMs(base)
				drainCombatPacingAt(scene, start)
				calls := state.GetRNG().Calls()
				drainCombatPacingAt(scene, start+duration-1)
				assert.Zero(t, finished)
				assert.True(t, callbacks.InputBlocked)
				drainCombatPacingAt(scene, start+duration)
				assert.Equal(t, 1, finished)
				assert.False(t, callbacks.InputBlocked)
				assert.Equal(t, calls, state.GetRNG().Calls(), "queue-only animation must not add gameplay draws")
			})
		}
	}
}

func TestCombatTimingPartySpellFlashEveryRate(t *testing.T) {
	for _, rate := range combatTimingTestRates() {
		for _, spell := range []references.Spell{references.Mani, references.VasMani, references.AnNox, references.AnZu} {
			t.Run(fmt.Sprintf("%g/%s", rate, references.GetSpellName(spell)), func(t *testing.T) {
				scene, callbacks := newCombatUseTestScene(t)
				setCombatTimingTestRate(scene, rate)
				state := scene.gameState
				t.Cleanup(state.EventQueueManager.Shutdown)
				actor := state.CombatScene.GetCurrentTurnIdentity()
				start := state.ElapsedMs
				// Healing/cure inversion is authored at 500ms, independently of
				// PartyStateBuilder's generic 200ms damage-flash preset.
				base := int64(500)
				if spell == references.AnZu {
					base = 150
				}
				flash, invert := 0.0, 0.0
				state.SystemCallbacks.Visual.SetScreenFlashIntensity = func(value float64) { flash = value }
				state.SystemCallbacks.Visual.SetScreenInvert = func(value float64, _ int) { invert = value }
				scene.presentCombatPartySpell(game_state.CombatPartySpellResult{Spell: spell, Applied: true, Target: actor, Healed: 1})
				setCombatTimingTestRate(scene, 4)
				duration := newCombatPresentationTiming(rate).durationMs(base)
				drainCombatPacingAt(scene, start)
				drainCombatPacingAt(scene, start+duration/2)
				// Odd rounded durations sample just before the exact midpoint;
				// the builder also quantizes its float32 progress to milliseconds.
				midpointIntensity := 2 * float64(duration/2) / float64(duration)
				oneMillisecondIntensity := 2 / float64(duration)
				if spell == references.AnZu {
					assert.InDelta(t, 0.6*midpointIntensity, flash, 0.6*oneMillisecondIntensity+0.00001,
						"white flash must follow its authored curve on the scaled timeline")
					assert.Zero(t, invert)
				} else {
					assert.InDelta(t, midpointIntensity, invert, oneMillisecondIntensity+0.00001,
						"inversion must follow its authored curve on the scaled timeline")
					assert.Zero(t, flash)
				}
				drainCombatPacingAt(scene, start+duration-1)
				assert.True(t, callbacks.InputBlocked)
				drainCombatPacingAt(scene, start+duration)
				assert.False(t, callbacks.InputBlocked)
				assert.False(t, state.EventQueueManager.HasWorkflow("combat_party_spell_effect"))
				assert.Zero(t, flash)
				assert.Zero(t, invert)
				assert.Equal(t, 1, callbacks.UnblockInputCalls)
				drainCombatPacingAt(scene, start+duration+1)
				assert.Equal(t, 1, callbacks.UnblockInputCalls, "completion and input release stay exact-once")
			})
		}
	}
}
