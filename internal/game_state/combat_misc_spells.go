package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// CastCombatAnYlem removes the original set of removable objects adjacent to
// the caster, leaving the original brick-floor tile behind.
func (g *GameState) CastCombatAnYlem(casterIdx int, direction references.Direction) bool {
	origin := g.combatPartyAttackerPosition(casterIdx)
	target := direction.GetNewPositionInDirection(&origin)
	if target == nil || !isAnYlemObject(g.CombatScene.CombatMapRef.GetTileNumber(*target)) {
		return false
	}
	g.CombatScene.CombatMapRef.SetTileNumber(*target, indexes.BrickFloor)

	return true
}

func isAnYlemObject(tile indexes.SpriteIndex) bool {
	// The original An Ylem switch intentionally excludes other pushable tiles.
	//nolint:exhaustive // Only the original removable object tiles are valid.
	switch tile {
	case indexes.Plant,
		indexes.ChairBackForward, indexes.ChairBackLeft, indexes.ChairBackBack, indexes.ChairBackRight,
		indexes.TableMiddle,
		indexes.Barrel,
		indexes.Vanity,
		indexes.WaterJugTable,
		indexes.Dresser,
		indexes.EndTable,
		indexes.Box,
		indexes.Mirror:
		return true
	default:
		return false
	}
}

// CastCombatWisQuas reveals every active invisible monster in the encounter
// and returns their positions for presentation effects.
func (g *GameState) CastCombatWisQuas() []references.Position {
	revealed := make([]references.Position, 0)
	for monsterIdx := range g.CombatScene.MonsterManager.Count() {
		monster := g.CombatScene.MonsterManager.GetMonster(monsterIdx)
		if monster == nil || !monster.IsActive() || monster.IsDead() || !monster.IsInvisible() {
			continue
		}
		monster.SetInvisible(false)
		revealed = append(revealed, monster.Position())
	}

	return revealed
}

// CastCombatEarthquakeResult resolves each eligible hostile entry once, retaining
// avoidance as well as actual HP damage. Friendly/inactive entries are excluded.
func (g *GameState) CastCombatEarthquakeResult(casterIdx int) CombatAreaSpellResult {
	result := CombatAreaSpellResult{Spell: references.InVasPorYlem}
	// Original prepared-spell rules scan combat queue upward, including possessed PC entries.
	// See ref:SPELL-0002.
	for sourceSlot := range sourceCombatQueueSlots {
		entry := g.CombatScene.TurnQueue.getActiveEntryAtSourceSlot(sourceSlot)
		if entry == nil {
			continue
		}
		target := turnQueueIdentity(entry)
		if g.combatIdentityLoyalty(target) != CombatMonsterLoyaltyMonster {
			continue
		}
		outcome := g.combatSpellTargetResult(target)
		hit := g.rollCombatDexterityThreshold(g.effectiveCombatDexterity(target))
		if hit.Outcome == CombatHitMiss {
			outcome.Outcome = CombatSpellAvoided
			result.Targets = append(result.Targets, g.publishCombatAreaSpellTarget(
				casterIdx, references.InVasPorYlem, outcome, CombatDiagnostics{Hit: hit},
			))

			continue
		}
		damage := g.RollDice(20)
		if target.IsPartyMember {
			character := g.PartyState.GetCharacter(target.EntityIndex)
			hpBefore := character.GetCurrentHp()
			character.SubtractHp(uint16(damage))
			hpLost := int(hpBefore - character.GetCurrentHp())
			outcome.HPLost, outcome.Fatal = hpLost, character.IsDead()
			result.Targets = append(result.Targets, g.publishCombatEarthquakeResult(casterIdx, outcome, hit, damage, damage))
			if character.IsDead() {
				g.markCombatPCDead(target.EntityIndex)
			}

			continue
		}
		monsterIdx := target.EntityIndex
		monster := g.CombatScene.MonsterManager.GetMonster(monsterIdx)
		finalDamage := CombatApplyMagicDamageModifiers(damage, monster.HasAbility(references.Immortal))
		hpBefore := monster.CurrentHP()
		monster.TakeDamage(finalDamage)
		hpLost := hpBefore - monster.CurrentHP()
		outcome.HPLost, outcome.Fatal = hpLost, monster.IsDead()
		outcome.MonsterInjury = g.captureCombatMonsterInjury(target, hpLost)
		result.Targets = append(result.Targets, g.publishCombatEarthquakeResult(casterIdx, outcome, hit, damage, finalDamage))
		if hpBefore > 0 && monster.IsDead() {
			g.finalizeCombatMonsterDeath(monster, monsterIdx, g.PartyState.GetCharacter(casterIdx), casterIdx)
		} else if hpLost > 0 {
			g.tryDivideCombatMonster(monster, monsterIdx)
		}
	}

	return result
}

func (g *GameState) publishCombatEarthquakeResult(
	casterIdx int, result CombatAreaSpellTargetResult, hit CombatHitResult, damage, finalDamage int,
) CombatAreaSpellTargetResult {
	result.Outcome = combatSpellDamageOutcome(result.HPLost)

	return g.publishCombatAreaSpellTarget(casterIdx, references.InVasPorYlem, result, CombatDiagnostics{
		Hit: hit, FinalDamage: finalDamage,
		Damage: CombatDamageResult{Damage: damage, AttackMax: 20, AttackRoll: damage, AttackRolled: true},
	})
}
