package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// CastCombatAnXenEx changes a hostile monster's allegiance or frees a possessed PC.
func (g *GameState) CastCombatAnXenEx(casterIdx int, target CombatTurnIdentity) bool {
	caster := g.PartyState.GetCharacter(casterIdx)
	if target.IsPartyMember {
		if !g.CombatScene.isCombatPCPossessed(target) {
			panic("CastCombatAnXenEx resolved with an ineligible party member")
		}
		if g.rollCombatIntelligenceSave(int(caster.Intelligence), int(g.PartyState.GetCharacter(target.EntityIndex).Intelligence)).Outcome == CombatHitMiss {
			g.publishCombatSpellMutation(references.AnXenEx, casterIdx, target, CombatFeedbackMiss)

			return false
		}
		g.CombatScene.setCombatPCPossessed(target, false)
		g.PartyState.GetCharacter(target.EntityIndex).SetStatus(party_state.Good)
		g.publishCombatSpellMutation(references.AnXenEx, casterIdx, target, CombatFeedbackDamage)

		return true
	}
	monster := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
	if monster == nil || !monster.IsHostile() || isCombatSpellImmuneMonster(monster) {
		g.publishCombatSpellMutation(references.AnXenEx, casterIdx, target, CombatFeedbackMiss)

		return false
	}
	if g.rollCombatIntelligenceSave(int(caster.Intelligence), monster.EnemyRef().Intelligence).Outcome == CombatHitMiss {
		g.publishCombatSpellMutation(references.AnXenEx, casterIdx, target, CombatFeedbackMiss)

		return false
	}
	monster.SetLoyalty(CombatMonsterLoyaltyParty)
	monster.charmed = true
	g.publishCombatSpellMutation(references.AnXenEx, casterIdx, target, CombatFeedbackDamage)

	return true
}

// CastCombatRelXenBet replaces the selected combat entry with a new hostile rat.
func (g *GameState) CastCombatRelXenBet(casterIdx int, target CombatTurnIdentity) bool {
	if target.IsPartyMember {
		if target.Slot == 0 && !g.CombatScene.PartyActive[target.EntityIndex] {
			panic("CastCombatRelXenBet resolved with an inactive party member")
		}
		if !g.CombatScene.canReplaceCombatantWithMonster(target) {
			return false
		}
		if g.rollCombatIntelligenceSave(int(g.PartyState.GetCharacter(casterIdx).Intelligence), int(g.PartyState.GetCharacter(target.EntityIndex).Intelligence)).Outcome == CombatHitMiss {
			g.publishCombatSpellMutation(references.RelXenBet, casterIdx, target, CombatFeedbackMiss)

			return false
		}

		targetPosition := g.CombatScene.GetCombatantPosition(target)
		if !g.replaceCombatTargetWithRat(target, targetPosition) {
			return false
		}
		g.publishCombatSpellMutationAt(
			references.RelXenBet, casterIdx, target, targetPosition, CombatFeedbackDamage,
		)

		return true
	}
	monster := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
	if monster == nil || isCombatSpellImmuneMonster(monster) {
		g.publishCombatSpellMutation(references.RelXenBet, casterIdx, target, CombatFeedbackMiss)

		return false
	}
	if !g.CombatScene.canReplaceCombatantWithMonster(target) {
		return false
	}
	if g.rollCombatIntelligenceSave(int(g.PartyState.GetCharacter(casterIdx).Intelligence), monster.EnemyRef().Intelligence).Outcome == CombatHitMiss {
		g.publishCombatSpellMutation(references.RelXenBet, casterIdx, target, CombatFeedbackMiss)

		return false
	}

	if !g.replaceCombatTargetWithRat(target, monster.Position()) {
		return false
	}
	g.publishCombatSpellMutation(references.RelXenBet, casterIdx, target, CombatFeedbackDamage)

	return true
}

// TransformActivePartyMemberIntoRat applies the Purple potion's encounter-local mutation.
func (g *GameState) TransformActivePartyMemberIntoRat(memberIndex int) bool {
	if g.CombatScene == nil {
		return false
	}
	target := g.CombatScene.GetCurrentTurnIdentity()
	if !target.IsPartyMember || target.EntityIndex != memberIndex {
		return false
	}
	entry := g.CombatScene.TurnQueue.GetEntryByIdentity(target)
	if entry == nil || !g.CombatScene.isCombatPCActive(target) {
		return false
	}

	// Original item-use rules changes only object tile/shape, unlike prepared Rel Xen Bet.
	// See ref:SPELL-0005.
	entry.ratAppearance = true

	return true
}

func (g *GameState) replaceCombatTargetWithRat(target CombatTurnIdentity, position references.Position) bool {
	ratReference, err := g.enemyReferenceForCombat(references.MonsterRat)
	if err != nil {
		panic("rat reference is unavailable: " + err.Error())
	}
	rat := NewCombatMonster(references.MonsterRat, ratReference, position, g.AsRandomProvider())
	_, controller := g.CombatScene.replaceCombatantWithMonster(target, rat)
	g.releaseCombatController(controller, CombatPossessionReleaseControllerReplaced)

	return true
}

// CastCombatInQuasXen creates an independent encounter-local combat entry.
func (g *GameState) CastCombatInQuasXen(casterIdx int, target CombatTurnIdentity) bool {
	if !g.CombatScene.TurnQueue.hasFreeSourceSlot(0) || !g.CombatScene.hasActiveObjectSlot() {
		return false
	}
	position, found := g.findCombatClonePosition(target)
	if !found {
		return false
	}

	return g.castCombatInQuasXenAt(casterIdx, target, position)
}

func (g *GameState) castCombatInQuasXenAt(casterIdx int, target CombatTurnIdentity, position references.Position) bool {
	if target.IsPartyMember {
		return g.cloneCombatPCAt(casterIdx, target, position)
	}
	source := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
	if source == nil {
		panic("CastCombatInQuasXen resolved without a target monster")
	}
	clone := *source
	clone.combatantID = 0
	clone.SetPosition(position)
	g.CombatScene.addClonedMonsterToQueue(target, &clone)
	g.publishCombatSpellMutation(references.InQuasXen, casterIdx, target, CombatFeedbackDamage)

	return true
}

func (g *GameState) cloneCombatPCAt(casterIdx int, target CombatTurnIdentity, position references.Position) bool {
	if target.Slot == 0 && !g.CombatScene.PartyActive[target.EntityIndex] {
		return false
	}
	character := g.PartyState.GetCharacter(target.EntityIndex)
	identity := g.CombatScene.addPCCloneFromSource(
		target, target.EntityIndex, position, NewCombatPCProvider(character),
	)
	if identity.Slot == 0 {
		return false
	}
	g.publishCombatSpellMutation(references.InQuasXen, casterIdx, target, CombatFeedbackDamage)

	return true
}

func (g *GameState) findCombatClonePosition(target CombatTurnIdentity) (references.Position, bool) {
	if !target.IsPartyMember {
		if g.CombatScene.MonsterManager.GetMonster(target.EntityIndex) == nil {
			return references.Position{}, false
		}
	}
	for range 256 {
		position := references.Position{X: references.Coordinate(g.RandomIntInRange(0, 15)), Y: references.Coordinate(g.RandomIntInRange(0, 15))}
		if position.X >= references.XCombatMapTiles || position.Y >= references.YCombatMapTiles {
			continue
		}
		// Clone placement validates every clone with fixed PLAYER shape,
		// independently of the copied actor's appearance or monster type.
		// See ref:SPELL-0005.
		if g.isCombatPositionValid(&position, -1) {
			return position, true
		}
	}

	return references.Position{}, false
}

func (g *GameState) publishCombatSpellMutation(spell references.Spell, casterIdx int, target CombatTurnIdentity, result CombatFeedbackResult) {
	g.publishCombatSpellMutationAt(spell, casterIdx, target, g.combatTurnPosition(target), result)
}

func (g *GameState) publishCombatSpellMutationAt(
	spell references.Spell,
	casterIdx int,
	target CombatTurnIdentity,
	targetPosition references.Position,
	result CombatFeedbackResult,
) {
	casterIdentity := g.combatPartyAttackerIdentity(casterIdx)
	caster := combatFeedbackIdentity(casterIdentity, g.CombatScene.GetCombatantPosition(casterIdentity))
	targetEntity := combatFeedbackIdentity(target, targetPosition)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventSpell, Attacker: caster, Target: targetEntity, Result: result,
		From: caster.Position, To: targetEntity.Position, Diagnostics: CombatDiagnostics{Spell: spell},
	})
}
