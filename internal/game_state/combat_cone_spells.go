package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func (g *GameState) castCommittedCombatCone(
	spell references.Spell,
	casterIdx int,
	direction references.Direction,
) []references.Position {
	spec, ok := CombatConeSpecForSpell(spell)
	if !ok {
		panic("combat cone configuration missing")
	}
	origin := g.combatPartyAttackerPosition(casterIdx)
	candidates := g.collectCommittedCombatCone(origin, direction, spec)

	return g.resolveCommittedCombatCone(spell, casterIdx, candidates)
}

// ResolveCommittedCombatCone applies the candidate collection retained at paid
// confirmation without rerunning or rerandomizing the source ray traversal.
func (g *GameState) ResolveCommittedCombatCone(result CombatConeTargetResult) CombatAreaSpellResult {
	if result.Outcome != CombatConeTargetResolved || !result.committed {
		panic("combat cone resolution requires a committed target result")
	}

	return g.resolveCombatConeResults(result.Request.Spell, result.Request.CasterIdx, result.candidates)
}

func (g *GameState) resolveCommittedCombatCone(
	spell references.Spell,
	casterIdx int,
	candidates []references.Position,
) []references.Position {
	//nolint:exhaustive // Only configured combat cone spells can reach resolution.
	switch spell {
	case references.InZu:
		return g.resolveCombatInZu(casterIdx, candidates)
	case references.InNoxHur:
		return g.resolveCombatInNoxHur(casterIdx, candidates)
	case references.InFlamHur:
		return g.resolveCombatInFlamHur(casterIdx, candidates)
	case references.InVasGravCorp:
		return g.resolveCombatInVasGravCorp(casterIdx, candidates)
	default:
		panic("unsupported committed combat cone spell")
	}
}

func (g *GameState) resolveCombatInNoxHur(
	casterIdx int,
	candidates []references.Position,
) []references.Position {
	return g.resolveCombatConeResults(references.InNoxHur, casterIdx, candidates).processedPositions()
}

func (g *GameState) resolveCombatInFlamHur(
	casterIdx int,
	candidates []references.Position,
) []references.Position {
	return g.resolveCombatConeResults(references.InFlamHur, casterIdx, candidates).processedPositions()
}

func (g *GameState) resolveCombatInVasGravCorp(
	casterIdx int,
	candidates []references.Position,
) []references.Position {
	return g.resolveCombatConeResults(references.InVasGravCorp, casterIdx, candidates).processedPositions()
}

func (g *GameState) applyCombatInFlamHurToTarget(casterIdx int, target CombatTurnIdentity) CombatAreaSpellTargetResult {
	result := g.combatSpellTargetResult(target)
	if !target.IsPartyMember {
		monsterIdx := target.EntityIndex
		monster := g.CombatScene.MonsterManager.GetMonster(monsterIdx)
		damage := g.GetRNG().Intn(31)
		hpBefore := monster.CurrentHP()
		finalDamage := CombatApplyMagicDamageModifiers(
			damage,
			monster.HasAbility(references.Immortal),
		)
		monster.TakeDamage(finalDamage)
		hpLost := hpBefore - monster.CurrentHP()
		result.HPLost, result.Fatal = hpLost, monster.IsDead()
		result.MonsterInjury = g.captureCombatMonsterInjury(target, hpLost)
		result = g.publishCombatWindDamageResult(references.InFlamHur, CombatHitResult{}, casterIdx, result, damage, finalDamage)
		if hpBefore > 0 && monster.IsDead() {
			g.finalizeCombatMonsterDeath(monster, monsterIdx, g.PartyState.GetCharacter(casterIdx), casterIdx)
		} else if hpLost > 0 {
			g.tryDivideCombatMonster(monster, monsterIdx)
		}

		return result
	}
	pcIdx := target.EntityIndex
	character := g.PartyState.GetCharacter(pcIdx)
	damage := g.GetRNG().Intn(31)
	hpBefore := character.GetCurrentHp()
	character.SubtractHp(uint16(damage))
	result.HPLost, result.Fatal = int(hpBefore-character.GetCurrentHp()), character.IsDead()
	if character.IsDead() {
		g.markCombatPCDead(pcIdx)
	}

	return g.publishCombatWindDamageResult(references.InFlamHur, CombatHitResult{}, casterIdx, result, damage, damage)
}

func (g *GameState) applyCombatInVasGravCorpToTarget(casterIdx int, target CombatTurnIdentity) CombatAreaSpellTargetResult {
	caster := g.PartyState.GetCharacter(casterIdx)
	result := g.combatSpellTargetResult(target)
	result.Outcome = CombatSpellResisted
	if !target.IsPartyMember {
		monsterIdx := target.EntityIndex
		monster := g.CombatScene.MonsterManager.GetMonster(monsterIdx)
		hit := g.rollCombatIntelligenceSave(int(caster.Intelligence), monster.EnemyRef().Intelligence)
		if isCombatSpellImmuneMonster(monster) || monster.HasAbility(references.Immortal) {
			hit.Outcome = CombatHitMiss
			result.Outcome = CombatSpellImmune
		}
		if hit.Outcome == CombatHitMiss {
			return g.publishCombatEnergyVortexResult(casterIdx, result, hit)
		}
		hpBefore := monster.CurrentHP()
		monster.SetCurrentHP(0)
		result.Outcome, result.HPLost, result.Fatal = CombatSpellDamaged, hpBefore, true
		result = g.publishCombatEnergyVortexResult(casterIdx, result, hit)
		g.finalizeCombatMonsterDeath(monster, monsterIdx, caster, casterIdx)

		return result
	}
	pcIdx := target.EntityIndex
	character := g.PartyState.GetCharacter(pcIdx)
	hit := g.rollCombatIntelligenceSave(int(caster.Intelligence), int(character.Intelligence))
	if hit.Outcome == CombatHitMiss {
		return g.publishCombatEnergyVortexResult(casterIdx, result, hit)
	}
	hpBefore := character.GetCurrentHp()
	character.SetHp(0)
	character.MarkAsDead()
	g.markCombatPCDead(pcIdx)
	result.Outcome, result.HPLost, result.Fatal = CombatSpellDamaged, int(hpBefore), true

	return g.publishCombatEnergyVortexResult(casterIdx, result, hit)
}

// Original combat damage rules poisons a Good PC; every other target takes damage.
// See ref:SPELL-0003.
func (g *GameState) applyCombatPoisonWindToxify(casterIdx int, target CombatTurnIdentity, hit CombatHitResult) CombatAreaSpellTargetResult {
	result := g.combatSpellTargetResult(target)
	if hit.Outcome == CombatHitMiss {
		result.Outcome = CombatSpellAvoided

		return g.publishCombatAreaSpellTarget(casterIdx, references.InNoxHur, result, CombatDiagnostics{Hit: hit})
	}
	if target.IsPartyMember && g.PartyState.GetCharacter(target.EntityIndex).IsGood() {
		g.PartyState.GetCharacter(target.EntityIndex).SetStatus(party_state.Poisoned)
		result.Outcome = CombatSpellPoisoned

		return g.publishCombatAreaSpellTarget(casterIdx, references.InNoxHur, result, CombatDiagnostics{Hit: hit})
	}
	damage := g.GetRNG().Intn(monsterPoisonDamageRange)
	finalDamage := damage
	var hpLost int
	var fatal bool
	if target.IsPartyMember {
		character := g.PartyState.GetCharacter(target.EntityIndex)
		hpBefore := character.GetCurrentHp()
		character.SubtractHp(uint16(damage))
		hpLost, fatal = int(hpBefore-character.GetCurrentHp()), character.IsDead()
		if fatal {
			g.markCombatPCDead(target.EntityIndex)
		}
	} else {
		monster := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
		finalDamage = CombatApplyMagicDamageModifiers(damage, monster.HasAbility(references.Immortal))
		hpBefore := monster.CurrentHP()
		monster.TakeDamage(finalDamage)
		hpLost, fatal = hpBefore-monster.CurrentHP(), monster.IsDead()
		result.MonsterInjury = g.captureCombatMonsterInjury(target, hpLost)
	}
	result.HPLost, result.Fatal = hpLost, fatal
	result = g.publishCombatWindDamageResult(references.InNoxHur, hit, casterIdx, result, damage, finalDamage)
	if !target.IsPartyMember {
		monster := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
		if fatal {
			g.finalizeCombatMonsterDeath(monster, target.EntityIndex, g.PartyState.GetCharacter(casterIdx), casterIdx)
		} else if hpLost > 0 {
			g.tryDivideCombatMonster(monster, target.EntityIndex)
		}
	}

	return result
}

func (g *GameState) rollCombatDexterityThreshold(threshold int) CombatHitResult {
	// Original: cone-ray effect calls transformed d30 for poison wind.
	// See ref:RNG-0001.
	roll := g.RollOriginalD30()

	return combatDexterityThresholdResult(roll, threshold)
}

func combatDexterityThresholdResult(roll, threshold int) CombatHitResult {
	result := CombatHitResult{
		Outcome:              CombatHitMiss,
		DefenderEffectiveDex: threshold,
		Threshold:            threshold,
		D30Roll:              roll,
		Rolled:               true,
	}
	if roll >= threshold {
		result.Outcome = CombatHitNormal
	}

	return result
}

func (g *GameState) rollCombatIntelligenceSave(casterIntelligence, targetIntelligence int) CombatHitResult {
	// Original: Intelligence saving throw calls transformed d30.
	// See ref:RNG-0001.
	return rollCombatIntelligenceSaveWithRNG(casterIntelligence, targetIntelligence, g.AsRandomProvider())
}

func rollCombatIntelligenceSaveWithRNG(
	casterIntelligence, targetIntelligence int,
	rng references.RandomProvider,
) CombatHitResult {
	roll := rollOriginalD30(rng)
	threshold := (30 + targetIntelligence - casterIntelligence) / 2
	result := CombatHitResult{
		Outcome:      CombatHitNormal,
		AttackerStat: casterIntelligence,
		Threshold:    threshold,
		D30Roll:      roll,
		Rolled:       true,
	}
	if roll < threshold {
		result.Outcome = CombatHitMiss
	}

	return result
}

func (g *GameState) publishCombatWindDamageResult(
	spell references.Spell,
	hit CombatHitResult,
	casterIdx int,
	result CombatAreaSpellTargetResult,
	damage, finalDamage int,
) CombatAreaSpellTargetResult {
	attackMax := 30
	if spell == references.InNoxHur {
		attackMax = monsterPoisonDamageRange - 1
	}
	result.Outcome = combatSpellDamageOutcome(result.HPLost)

	return g.publishCombatAreaSpellTarget(casterIdx, spell, result, CombatDiagnostics{
		Hit: hit, FinalDamage: finalDamage,
		Damage: CombatDamageResult{Damage: damage, AttackMax: attackMax, AttackRoll: damage, AttackRolled: true},
	})
}

func (g *GameState) publishCombatEnergyVortexResult(
	casterIdx int,
	result CombatAreaSpellTargetResult,
	hit CombatHitResult,
) CombatAreaSpellTargetResult {
	return g.publishCombatAreaSpellTarget(casterIdx, references.InVasGravCorp, result, CombatDiagnostics{
		Hit: hit, FinalDamage: combatInstantKillDamage,
	})
}
