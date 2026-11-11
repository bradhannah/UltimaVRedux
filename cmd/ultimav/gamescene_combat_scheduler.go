package main

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const combatAutoPassPCDurationMs = 250

// combatTurnHandOffMs is the frame a guarded scheduler workflow formerly spent
// releasing input before the next actor could start. It is now an explicit,
// speed-scaled pause: one 60 TPS frame, rounded down so 1x never slips a frame.
const combatTurnHandOffMs = 16

// combatTurnHandOffMark records when a scheduler workflow finished, so only a
// next actor reviewed in that same frame receives the scaled hand-off pause.
type combatTurnHandOffMark struct {
	encounter  *game_state.CombatState
	finishedMs int64
}

func (g *GameScene) markCombatTurnHandOff() {
	g.combatTurnHandOff = combatTurnHandOffMark{encounter: g.gameState.CombatScene, finishedMs: g.gameState.ElapsedMs}
}

// takeCombatTurnHandOff consumes the mark. A release delayed by a held key, a
// player action or a new encounter has already spent its frames, so adds none.
func (g *GameScene) takeCombatTurnHandOff(timing combatPresentationTiming) int64 {
	mark := g.combatTurnHandOff
	g.combatTurnHandOff = combatTurnHandOffMark{}
	if mark.encounter == nil || mark.encounter != g.gameState.CombatScene || mark.finishedMs != g.gameState.ElapsedMs {
		return 0
	}

	return timing.pauseMs(combatTurnHandOffMs)
}

// reviewScheduledCombatStep claims typed GameState work and supplies only its
// delay, highlights, movement, projectile, and input-blocking presentation.
func (g *GameScene) reviewScheduledCombatStep() bool {
	step, ok := g.gameState.ReviewScheduledCombatStep()
	if !ok {
		return false
	}

	switch step.Kind {
	case game_state.CombatPresentationMonsterTurn:
		g.startScheduledMonsterTurn(step)
	case game_state.CombatPresentationAutoPass:
		g.startScheduledCombatAutoPass(step)
	case game_state.CombatPresentationTurnCompletion:
		panic("workflow-owned combat completion was exposed for scene review")
	default:
		panic("unknown scheduled combat presentation kind")
	}

	return true
}

func (g *GameScene) startScheduledMonsterTurn(step game_state.CombatPresentationStep) {
	pacing, fastMovement := g.fastScheduledMonsterPacing(step.Actor)
	g.startScheduledMonsterTurnWithPacing(step, pacing, fastMovement)
}

// startScheduledMonsterTurnWithPacing also lets deterministic fixtures compare
// the former presentation profile through the same resolve/complete workflow.
func (g *GameScene) startScheduledMonsterTurnWithPacing(
	step game_state.CombatPresentationStep, pacing combatMonsterPacing, fastMovement bool,
) {
	timing := g.captureCombatTiming()
	delayMs := timing.pauseMs(game_state.MonsterMoveDelayMs)
	if fastMovement {
		delayMs = int64(pacing.anticipationMs) //nolint:gosec // Validated speed bounds the 75ms base to 0-150ms.
	}
	if step.HasTarget {
		delayMs = timing.pauseMs(game_state.MonsterAttackDelayMs)
	}
	delayMs += g.takeCombatTurnHandOff(timing)
	animate := timing.animatesMotion()

	logging.Debugf(logging.CategoryGameState,
		"Combat scheduler: monster=%d actor=%+v target=%+v valid=%t delay=%dms",
		step.MonsterIndex, step.Actor, step.Target, step.HasTarget, delayMs)

	var fromPos, toPos references.Position
	result := game_state.MonsterTurnExecutionResult{}
	resolved := false

	queue := g.gameState.EventQueueManager.Workflow("monster_turn_delay").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		InputReleaseReady(func() bool { return !g.keyboard.HasUnprotectedHeldKey() }).
		First(func() {}).
		Wait(delayMs).
		Then(g.combatTimingStep(timing, func() {
			combatScene := g.gameState.CombatScene
			if combatScene != nil {
				fromPos = combatScene.GetCombatantPosition(step.Actor)
			}

			var err error
			result, err = g.gameState.ResolveScheduledCombatStep(step.Token)
			if err != nil {
				if combatPresentationWasSuperseded(err) {
					logging.Debugf(logging.CategoryGameState, "Combat scheduler resolution superseded: %v", err)

					return
				}

				panic(fmt.Sprintf("combat scheduler resolution contract violated: %v", err))
			}
			resolved = true

			combatScene = g.gameState.CombatScene
			if combatScene != nil && result.Moved {
				toPos = combatScene.GetCombatantPosition(step.Actor)
				if animate && shouldAnimateScheduledMonsterMovement(result, fromPos, toPos) {
					g.startCombatMovement(
						step.Actor.IsPartyMember, step.Actor.EntityIndex, step.Actor.Slot, fromPos, toPos,
					)
				}
			}
		})).ThenSequence(func(sequence *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		if result.ProjectileLaunch != nil {
			return g.combatProjectileAnimationSequenceWithTiming(timing, *result.ProjectileLaunch, func() {
				g.gameState.ResolvePendingMonsterMissile()
			})(sequence)
		}
		if !animate || !shouldAnimateScheduledMonsterMovement(result, fromPos, toPos) {
			return sequence
		}

		movementMs := timing.duration(combatEnemyMovementDurationMs)
		if fastMovement {
			movementMs = pacing.movementMs
		}

		return sequence.Animate(movementMs, func(progress float32) {
			g.updateCombatMovementProgress(progress)
		})
	}).While(g.hasCombatSignatureEffects, func() {}, DialogPollingRateMs)

	queue.Finally(func() {
		g.clearCombatMovement()
		g.clearProjectile()
		g.clearCombatSignatureEffects()
		if result.ProjectileLaunch != nil {
			g.withCombatTiming(timing, func() { g.gameState.ResolvePendingMonsterMissile() })
		}
		if !resolved {
			return
		}
		g.completeScheduledCombatStep("monster turn", step.Token)
		g.markCombatTurnHandOff()
	}).Start()
}

func shouldAnimateScheduledMonsterMovement(
	result game_state.MonsterTurnExecutionResult,
	fromPos, toPos references.Position,
) bool {
	return result.Moved && !result.Teleported && fromPos != toPos
}

func (g *GameScene) startScheduledCombatAutoPass(step game_state.CombatPresentationStep) {
	timing := g.captureCombatTiming()
	delayMs := timing.pauseMs(combatAutoPassPCDurationMs) + g.takeCombatTurnHandOff(timing)
	g.gameState.EventQueueManager.Workflow("combat_pc_auto_pass").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		InputReleaseReady(func() bool { return !g.keyboard.HasUnprotectedHeldKey() }).
		First(func() {}).
		Wait(delayMs).
		Then(func() {}). // Preserve Wait's delay before While installs its immediate loop entry.
		While(g.hasCombatSignatureEffects, func() {}, DialogPollingRateMs).
		Finally(func() {
			g.clearCombatSignatureEffects()
			g.completeScheduledCombatStep("auto-pass", step.Token)
			g.markCombatTurnHandOff()
		}).Start()
}

func (g *GameScene) completeScheduledCombatStep(context string, token game_state.CombatPresentationToken) {
	err := g.gameState.CompleteScheduledCombatStep(token)
	if err == nil {
		return
	}
	if combatPresentationWasSuperseded(err) {
		logging.Debugf(logging.CategoryGameState, "Combat %s completion superseded: %v", context, err)

		return
	}

	panic(fmt.Sprintf("combat %s completion contract violated: %v", context, err))
}

func combatPresentationWasSuperseded(err error) bool {
	return errors.Is(err, game_state.ErrCombatPresentationInactive) ||
		errors.Is(err, game_state.ErrCombatPresentationStale)
}
