package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

type combatTurnStartResult int

const (
	combatTurnReady combatTurnStartResult = iota
	combatTurnSkipped
)

const (
	combatPCWakeRollMaximum      = 255
	combatPCWakeThreshold        = 16
	combatMonsterWakeRollMaximum = 16
)

// finishDirectedCombatAttack is deferred by authoritative attack resolution,
// with wasSleeping captured before any effect. Initial Gazer sleep therefore
// survives; a later attack on that sleeper wakes only the exact tactical copy.
// REDUX DEVIATION: hit status and damage/damage application preserve sleep through attacks.
// Redux wakes a living PC after any attack directed at them, even a miss or
// armor deflection, for intuitive threat response. This consumes no RNG or turn.
// Area effects and ongoing hazards do not call this helper.
// See ref:COMBAT-0006.
func (g *GameState) finishDirectedCombatAttack(target CombatTurnIdentity, wasSleeping bool) {
	if !target.IsPartyMember || !wasSleeping ||
		!g.CombatScene.isCombatPCSleeping(target) ||
		!g.CombatScene.isCombatPCActive(target) ||
		g.PartyState.GetCharacter(target.EntityIndex).IsDead() {
		return
	}
	g.setCombatPCSleeping(target, false)
	g.publishCombatPCConditionEvent(target, CombatConditionWoke)
}

// processCombatTurnStart resolves temporary conditions after the DEX queue
// selects a living combatant. Waking consumes the current turn in the original.
func (g *GameState) processCombatTurnStart(entry *TurnQueueEntry) combatTurnStartResult {
	if entry.Combatant.IsPC() {
		identity := turnQueueIdentity(entry)
		if g.combatIdentityLoyalty(identity) == CombatMonsterLoyaltyMonster || g.activateChaosSwordControl(identity) {
			return g.processPossessedPCTurnStart(identity)
		}

		return g.processCombatPCTurnStart(identity)
	}

	return g.processCombatMonsterTurnStart(entry.EntityIndex)
}

func (g *GameState) processPossessedPCTurnStart(identity CombatTurnIdentity) combatTurnStartResult {
	// Original world-state rules loyalty and original combat scheduler countdown scheduler send monster-loyal PCs (including
	// unpossessed Saduj) and initial Chaos interception through monster action,
	// whose entry gate rejects the underground-state mask before sleep or action handling.
	// See ref:COMBAT-0006.
	if g.CombatScene.isCombatPCUnderground(identity) {
		return combatTurnSkipped
	}
	if !g.CombatScene.isCombatPCSleeping(identity) {
		return combatTurnReady
	}
	condition := CombatConditionSleeping
	if g.RandomIntInRange(0, combatMonsterWakeRollMaximum) == combatMonsterWakeRollMaximum {
		g.setCombatPCSleeping(identity, false)
		condition = CombatConditionWoke
	}
	g.SystemCallbacks.Message.AddRowStr("Zzzzz...")
	g.publishCombatPCConditionEvent(identity, condition)

	return combatTurnSkipped
}

func (g *GameState) processCombatPCTurnStart(identity CombatTurnIdentity) combatTurnStartResult {
	if g.CombatScene.isCombatPCUnderground(identity) {
		return g.processUndergroundPCTurnStart(identity)
	}
	if !g.CombatScene.isCombatPCSleeping(identity) {
		return combatTurnReady
	}

	condition := CombatConditionSleeping
	if g.RandomIntInRange(0, combatPCWakeRollMaximum) < combatPCWakeThreshold {
		g.setCombatPCSleeping(identity, false)
		condition = CombatConditionWoke
	}

	g.SystemCallbacks.Message.AddRowStr("Zzzzz...")
	g.publishCombatPCConditionEvent(identity, condition)
	logging.Debugf(logging.CategoryGameState,
		"Combat PC %d/%d sleep turn resolved: condition=%d",
		identity.EntityIndex, identity.Slot, condition)

	return combatTurnSkipped
}

func (g *GameState) processUndergroundPCTurnStart(identity CombatTurnIdentity) combatTurnStartResult {
	character := g.PartyState.GetCharacter(identity.EntityIndex)
	g.SystemCallbacks.Message.AddRowStr("ARGH!")
	condition := CombatConditionUnderground
	if g.RollOriginalD30() < int(character.Dexterity) {
		g.CombatScene.setCombatPCUnderground(identity, false)
		g.relocateRegurgitatedPCIfOccupied(identity)
		g.SystemCallbacks.Message.AddRowStr(character.GetNameAsString() + " regurgitated!")
		condition = CombatConditionRegurgitated
	}
	g.publishCombatPCConditionEvent(identity, condition)
	logging.Debugf(logging.CategoryGameState,
		"Combat PC %d/%d underground turn resolved: condition=%d",
		identity.EntityIndex, identity.Slot, condition)

	return combatTurnSkipped
}

// relocateRegurgitatedPCIfOccupied moves a freshly regurgitated party character to the
// nearest free combat square when another party member has taken its square meanwhile.
//
// REDUX DEVIATION: COMBAT-0005. The original ignores underground combatants when it
// tests a square, so another member may step onto a swallowed member's square, and
// regurgitation clears the swallowed condition without moving the member: both then
// share the square. Redux brings the member back on the nearest free square instead,
// so no two combatants ever share one.
func (g *GameState) relocateRegurgitatedPCIfOccupied(identity CombatTurnIdentity) {
	origin := g.CombatScene.GetCombatantPosition(identity)
	if !g.CombatScene.isCombatPCAtPositionExcept(origin, identity) {
		return
	}
	best, bestDistance, found := origin, 0, false
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			candidate := references.Position{X: x, Y: y}
			distance := absoluteInt(int(candidate.X-origin.X)) + absoluteInt(int(candidate.Y-origin.Y))
			if (found && distance >= bestDistance) || !g.isCombatPositionValidForPC(&candidate, identity) {
				continue
			}
			best, bestDistance, found = candidate, distance, true
		}
	}
	if found {
		g.CombatScene.setCombatantPosition(identity, best)
	}
}

func (g *GameState) publishCombatPCConditionEvent(identity CombatTurnIdentity, condition CombatConditionResult) {
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventCondition,
		Target: CombatFeedbackEntity{
			IsPartyMember: true,
			Index:         identity.EntityIndex,
			Slot:          identity.Slot,
			Position:      g.CombatScene.GetCombatantPosition(identity),
		},
		Condition: condition,
	})
}

func (g *GameState) processCombatMonsterTurnStart(monsterIndex int) combatTurnStartResult {
	monster := g.CombatScene.MonsterManager.GetMonster(monsterIndex)
	if monster == nil {
		return combatTurnReady
	}
	if !monster.IsAsleep() {
		monster.SetCorpserExposed(false)

		return combatTurnReady
	}

	condition := CombatConditionSleeping
	if g.RandomIntInRange(0, combatMonsterWakeRollMaximum) == combatMonsterWakeRollMaximum {
		monster.SetAsleep(false)
		condition = CombatConditionWoke
	}

	g.publishCombatConditionEvent(monsterIndex, false, condition)
	logging.Debugf(logging.CategoryGameState,
		"Combat monster %d sleep turn resolved: condition=%d", monsterIndex, condition)

	return combatTurnSkipped
}

func (g *GameState) publishCombatConditionEvent(
	entityIndex int,
	isPartyMember bool,
	condition CombatConditionResult,
) {
	position := g.CombatScene.GetPartyMemberPosition(entityIndex)
	if !isPartyMember {
		position = g.CombatScene.MonsterManager.GetMonster(entityIndex).Position()
	}

	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventCondition,
		Target: CombatFeedbackEntity{
			IsPartyMember: isPartyMember,
			Index:         entityIndex,
			Position:      position,
		},
		Condition: condition,
	})
}
