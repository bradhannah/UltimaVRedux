package main

import (
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

const (
	combatPlayerMovementDurationMs uint64 = 333
	combatEnemyMovementDurationMs  uint64 = 650
	combatBatAnticipationMs        uint64 = 75
	combatBatMovementDurationMs    uint64 = 175
)

const (
	combatMovementSmoothstepScale = 3.0
	combatMovementSmoothstepBias  = 2.0
)

// CombatMovementAnimation visually interpolates an entity whose logical move is complete.
type CombatMovementAnimation struct {
	Active        bool
	IsPartyMember bool
	EntityIndex   int
	Slot          int
	From          references.Position
	To            references.Position
	Progress      float32
}

func (g *GameScene) startCombatMovement(
	isPartyMember bool,
	entityIndex, slot int,
	fromPos, toPos references.Position,
) {
	if g.combatMovement.Active {
		g.clearCombatMovement()
	}
	g.combatMovement = CombatMovementAnimation{
		Active:        true,
		IsPartyMember: isPartyMember,
		EntityIndex:   entityIndex,
		Slot:          slot,
		From:          fromPos,
		To:            toPos,
		Progress:      0,
	}
	g.bindCombatFireMovement()
}

func (g *GameScene) updateCombatMovementProgress(progress float32) {
	if g.combatMovement.Active {
		g.combatMovement.Progress = progress
	}
}

func (g *GameScene) clearCombatMovement() {
	g.finishCombatFireMovement(g.combatMovement.Active && g.combatMovement.Progress >= 1)
	g.combatMovement = CombatMovementAnimation{
		Active:        false,
		IsPartyMember: false,
		EntityIndex:   0,
		Slot:          0,
		From:          references.Position{X: 0, Y: 0},
		To:            references.Position{X: 0, Y: 0},
		Progress:      0,
	}
}

func (g *GameScene) startPlayerCombatMovementWorkflow(
	identity game_state.CombatTurnIdentity,
	fromPos, toPos references.Position,
	token game_state.CombatPresentationToken,
) {
	queue := g.gameState.EventQueueManager.Workflow("player_combat_movement").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {})
	g.animateCombatMovement(queue, true, identity.EntityIndex, identity.Slot, fromPos, toPos).Finally(func() {
		g.clearCombatMovement()
		g.completeScheduledCombatStep("player movement", token)
	}).Start()
}

func (g *GameScene) startControlledMonsterCombatMovementWorkflow(
	entityIndex int,
	fromPos, toPos references.Position,
	token game_state.CombatPresentationToken,
) {
	queue := g.gameState.EventQueueManager.Workflow("controlled_monster_combat_movement").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {})
	g.animateCombatMovement(queue, false, entityIndex, 0, fromPos, toPos).Finally(func() {
		g.clearCombatMovement()
		g.completeScheduledCombatStep("controlled-monster movement", token)
	}).Start()
}

// animateCombatMovement adds the player-paced glide for a completed move.
// Ludicrous adds none: the sprite is already drawn on its new square.
func (g *GameScene) animateCombatMovement(
	queue *event_queue.SequentialEventBuilder,
	isPartyMember bool,
	entityIndex, slot int,
	fromPos, toPos references.Position,
) *event_queue.SequentialEventBuilder {
	timing := g.captureCombatTiming()
	if !timing.animatesMotion() {
		return queue
	}
	g.startCombatMovement(isPartyMember, entityIndex, slot, fromPos, toPos)

	return queue.Animate(timing.duration(combatPlayerMovementDurationMs), g.updateCombatMovementProgress)
}

type combatMonsterPacing struct {
	anticipationMs uint64
	movementMs     uint64
}

// fastScheduledMonsterPacing captures both durations before any deferred queue
// stage runs. Only AI bats and mongbats use this profile; possessed PCs also
// enter the monster workflow, so reject party identities before monster lookup.
func (g *GameScene) fastScheduledMonsterPacing(actor game_state.CombatTurnIdentity) (combatMonsterPacing, bool) {
	if actor.IsPartyMember || g.gameState.CombatScene == nil {
		return combatMonsterPacing{}, false
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(actor.EntityIndex)
	if monster == nil || (monster.MonsterType() != references.MonsterBat &&
		monster.MonsterType() != references.MonsterMongbat) {
		return combatMonsterPacing{}, false
	}

	timing := g.captureCombatTiming()
	if !timing.animatesMotion() {
		// Ludicrous: no anticipation and no glide.
		return combatMonsterPacing{}, true
	}

	return combatMonsterPacing{
		anticipationMs: timing.duration(combatBatAnticipationMs),
		movementMs:     timing.duration(combatBatMovementDurationMs),
	}, true
}

func (g *GameScene) combatMovementOffset(isPartyMember bool, entityIndex, slot int) (float64, float64) {
	movement := g.combatMovement
	if !movement.Active || movement.IsPartyMember != isPartyMember || movement.EntityIndex != entityIndex ||
		(isPartyMember && movement.Slot != slot) {
		return 0, 0
	}

	fromX, fromY := sprites.ConvertPosition16to18(int(movement.From.X), int(movement.From.Y))
	toX, toY := sprites.ConvertPosition16to18(int(movement.To.X), int(movement.To.Y))
	progress := float64(movement.Progress)
	eased := progress * progress *
		(combatMovementSmoothstepScale - combatMovementSmoothstepBias*progress)
	remaining := 1 - eased

	return math.Round((fromX - toX) * remaining), math.Round((fromY - toY) * remaining)
}
