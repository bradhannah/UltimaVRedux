package game_state

// Combat formula helpers calculate hit, effective attributes, damage and
// XP without mutating state. Their callers own mutation and feedback.
// The public rules scope original behavior and unresolved paths rather than
// claiming byte-for-byte equivalence for every input. ref:COMBAT-0004

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// CombatHitOutcome describes the outcome of a hit check.
type CombatHitOutcome int

const (
	// CombatHitMiss means the attack missed entirely.
	CombatHitMiss CombatHitOutcome = iota
	// CombatHitNormal means the attack connected normally.
	CombatHitNormal
	// CombatHitNeverMiss means the weapon always hits (Jeweled Sword, Glass Sword, Chaos Sword).
	CombatHitNeverMiss
)

// CombatHitResult holds the hit outcome and the values used to calculate it.
type CombatHitResult struct {
	Outcome              CombatHitOutcome
	AttackerStat         int
	DefenderEffectiveDex int
	Threshold            int
	D30Roll              int
	Rolled               bool
	AutomaticHit         bool
}

// CombatDamageResult holds the calculated damage and metadata.
type CombatDamageResult struct {
	// Damage is the final damage to apply (0 = grazed/deflected, 99 = instant kill).
	Damage int
	// Deflected is true when damage was reduced to 0 or below by armor (original: DEFLECT_WND).
	Deflected bool
	// InstantKill is true for any source damage rating of 99.
	InstantKill bool
	// WeaponShattered is true if the weapon should be destroyed after this hit (Glass Sword).
	WeaponShattered bool
	// AttackMax is the maximum attack value supplied by the weapon or attacker.
	AttackMax int
	// AttackRoll is the effective attack value used before armor.
	AttackRoll int
	// AttackRolled is true when AttackRoll consumed an RNG value.
	AttackRolled bool
	// ArmorValue is the defender's supplied armor value.
	ArmorValue int
	// ArmorRoll is the armor reduction applied to the attack.
	ArmorRoll int
	// ArmorRolled is true when ArmorRoll consumed an RNG value.
	ArmorRolled bool
	// PreArmorDamage is damage before armor reduction.
	PreArmorDamage int
	// PostArmorDamage is damage after armor reduction, before clamping.
	PostArmorDamage int
}

// combatToHitBase is the base term of the hit threshold. Add defender
// Dexterity, subtract the attacking attribute, then halve with integer
// division. The transformed d30 must meet or exceed the result. ref:EQUIP-0012

const combatToHitBase = 30

// combatToHitDivisor divides the to-hit calculation.
//

const combatToHitDivisor = 2

// combatInstantKillDamage is the source damage value that causes instant death.
//

const combatInstantKillDamage = 99

// CombatHitCheck determines whether a physical attack hits.
// Use Strength for bludgeoning, otherwise Dexterity; use effective defender
// Dexterity. The threshold is integer (30 + defenderDex - attackerStat) / 2.
// The transformed d30 must meet or exceed it, unless the weapon never misses.
// The random provider is deterministic. ref:EQUIP-0012
func CombatHitCheck(
	attackerStat int,
	defenderDex int,
	neverMiss bool,
	rng references.RandomProvider,
) CombatHitResult {
	toHit := (combatToHitBase + defenderDex - attackerStat) / combatToHitDivisor
	result := CombatHitResult{
		Outcome:              CombatHitNeverMiss,
		AttackerStat:         attackerStat,
		DefenderEffectiveDex: defenderDex,
		Threshold:            toHit,
		D30Roll:              0,
		Rolled:               false,
		AutomaticHit:         neverMiss,
	}

	if neverMiss {
		return result
	}

	// Original: hit check calls transformed d30.
	// See ref:RNG-0001.
	roll := rollOriginalD30(rng)
	result.D30Roll = roll
	result.Rolled = true
	result.AutomaticHit = false

	if roll >= toHit {
		result.Outcome = CombatHitNormal

		return result
	}

	result.Outcome = CombatHitMiss

	return result
}

// CombatCalcDamage applies player-weapon damage and armor reduction.
// Glass Sword supplies the special 99 result; Jeweled Sword supplies zero.
// Bare hands use rating one; ordinary ratings above one are randomized before
// subtracting a draw from positive armor. Callers resolve special-result and
// creature modifiers separately. ref:EQUIP-0013
func CombatCalcDamage(
	weapon references.Equipment,
	armorValue int,
	rng references.RandomProvider,
) CombatDamageResult {
	result := CombatDamageResult{
		AttackMax:  weapon.GetAttackValue(),
		ArmorValue: armorValue,
	}

	weap := weapon.GetAttackValue()
	switch {
	case weapon.HasAbility(references.AbilityShatters):
		weap = combatInstantKillDamage
		result.WeaponShattered = true
	case weapon.HasAbility(references.AbilityNeverMisses):
		weap = 0
	case weapon == references.NoEquipment:
		weap = 1
		result.AttackMax = 1
	case weap > 1 && weap != combatInstantKillDamage:
		// Randomize an ordinary player weapon’s positive damage rating
		// within its inclusive range before armor reduction. ref:EQUIP-0013
		weap = rng.Intn(weap) + 1
		result.AttackRolled = true
	}
	result.AttackRoll = weap
	result.PreArmorDamage = weap
	result.PostArmorDamage = weap

	// Check for instant kill damage (Chaos Sword has atkval 99)
	if weap == combatInstantKillDamage {
		result.Damage = combatInstantKillDamage
		result.InstantKill = true

		return result
	}

	// Apply armor reduction
	damage := weap
	if armorValue > 0 {
		// Subtract an independent draw from 1 through positive armor. ref:EQUIP-0013
		armorReduction := rng.Intn(armorValue) + 1
		result.ArmorRoll = armorReduction
		result.ArmorRolled = true
		damage -= armorReduction
	}
	result.PostArmorDamage = damage

	if damage < 1 {
		result.Damage = 0
		result.Deflected = true

		return result
	}

	result.Damage = damage

	return result
}

// CombatCalcDamageRaw calculates damage from a monster's fixed damage rating.
// Original damage calculation randomizes PC weapon ratings only; monster ratings are
// applied directly before the defender's random armor reduction.
// See ref:EQUIP-0013.
func CombatCalcDamageRaw(
	attackValue int,
	armorValue int,
	rng references.RandomProvider,
) CombatDamageResult {
	return combatCalcDamageRaw(attackValue, armorValue, rng, false)
}

func combatCalcDamageRaw(
	attackValue int,
	armorValue int,
	rng references.RandomProvider,
	randomizeAttack bool,
) CombatDamageResult {
	result := CombatDamageResult{
		AttackMax:  attackValue,
		ArmorValue: armorValue,
	}

	// Determine effective damage
	weap := attackValue
	if randomizeAttack && weap < 1 {
		weap = 1
	} else if randomizeAttack && weap > 1 {
		weap = rng.Intn(weap) + 1
		result.AttackRolled = true
	}
	result.AttackRoll = weap
	result.PreArmorDamage = weap
	result.PostArmorDamage = weap

	// Check for instant kill damage (rare case for monsters with damage 99)
	if weap == combatInstantKillDamage {
		result.Damage = combatInstantKillDamage
		result.InstantKill = true

		return result
	}

	// Apply armor reduction
	damage := weap
	if armorValue > 0 {
		// Subtract an independent draw from 1 through positive armor. ref:EQUIP-0013
		armorReduction := rng.Intn(armorValue) + 1
		result.ArmorRoll = armorReduction
		result.ArmorRolled = true
		damage -= armorReduction
	}
	result.PostArmorDamage = damage

	if damage < 1 {
		result.Damage = 0
		result.Deflected = true

		return result
	}

	result.Damage = damage

	return result
}

// CombatCalcMagicDamage calculates spell damage using the original weapon-spell
// path: random attack damage minus a random armor reduction.
func CombatCalcMagicDamage(
	attackValue int,
	armorValue int,
	rng references.RandomProvider,
) CombatDamageResult {
	return combatCalcDamageRaw(attackValue, armorValue, rng, true)
}

// CombatApplyDamageModifiers adjusts damage for undead and immortal targets.
// Replicates the relevant parts of damage application from original combat damage rules.
//
// Parameters:
//   - damage: raw damage from CombatCalcDamage
//   - isUndead: target has Undead ability (half damage from non-magic weapons)
//   - isImmortal: target has Immortal ability (takes 0 damage)
//   - isMagicWeapon: weapon is magical (Silver Sword, Mystic Sword, etc.)
//
// Original behavior:
//   - Undead: damage /= 2 if not magic weapon
//   - Immortal: damage = 0
//
// See ref:EQUIP-0013.
func CombatApplyDamageModifiers(damage int, isUndead, isImmortal, isMagicWeapon bool) int {
	if isUndead && !isMagicWeapon {
		damage /= 2
	}
	if isImmortal {
		damage = 0
	}

	return damage
}

// CombatApplyMagicDamageModifiers bypasses the physical undead penalty while
// preserving the original immunity of immortal monsters.
func CombatApplyMagicDamageModifiers(damage int, isImmortal bool) int {
	if isImmortal {
		return 0
	}

	return damage
}

// combatCalcExperience returns integer maximum HP divided by four,
// plus one, for the kill award. ref:PROGRESS-0002
func combatCalcExperience(monsterMaxHP int) int {
	return monsterMaxHP/4 + 1
}

// CombatGetEffectiveDex returns a combatant's effective dexterity for hit checks.
// Replicates original effective Dexterity from original combat damage rules.
//
// Returns 1 when an identity-aware caller selects the sleep, Mimic, or An Tym
// override. Otherwise returns the target's base dexterity.
// See ref:EQUIP-0013.
func CombatGetEffectiveDex(baseDex int, useMinimum bool) int {
	if useMinimum {
		return 1
	}

	return baseDex
}

// IsMagicWeapon returns true for weapon classification's contiguous magical
// weapon range from Chaos Sword through Mystic Sword.
// See ref:EQUIP-0013.
func IsMagicWeapon(weapon references.Equipment) bool {
	return weapon >= references.ChaosSword && weapon <= references.MysticSword
}
