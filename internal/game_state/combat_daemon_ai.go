package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	daemonRawCombatSlots    = 32
	monsterSpecialRollRange = 256
	monsterSpecialThreshold = 32
	daemonMissileThreshold  = 128
	daemonRandomSquareRange = 16
)

type possessedPCTurnPlan struct {
	Target    CombatTurnIdentity
	HasTarget bool
}

func (g *GameState) activateChaosSwordControl(identity CombatTurnIdentity) bool {
	character := g.PartyState.GetCharacter(identity.EntityIndex)
	if character.GetEquippedItem(references.SlotWeapon) != references.ChaosSword &&
		character.GetEquippedItem(references.SlotShield) != references.ChaosSword {
		return false
	}

	g.CombatScene.setCombatPCPossessed(identity, true)
	g.clearInactiveCombatPCSelection(identity.EntityIndex)

	return true
}

func (g *GameState) hostileMonsterMagicIsNegated() bool {
	return g.IsCrownActive() || g.HasActiveDurationEffect(DurationEffectNegateMagic)
}

// executeHostileMonsterSpecialMove ports hostile special actions. A party-bound
// monster never reaches this hostile AI path.
// See ref:COMBAT-0008.
func (g *GameState) executeHostileMonsterSpecialMove(monster *CombatMonster, monsterIndex int) bool {
	return g.executeHostileMonsterSpecialMoveWithRNG(monster, monsterIndex, g.AsRandomProvider())
}

func (g *GameState) executeHostileMonsterSpecialMoveWithRNG(
	monster *CombatMonster,
	monsterIndex int,
	rng references.RandomProvider,
) bool {
	if g.hostileMonsterMagicIsNegated() {
		return false
	}

	if monster.HasAbility(references.PossessCharm) {
		victimSlot := rng.Intn(daemonRawCombatSlots)
		entry := g.CombatScene.TurnQueue.getActiveEntryAtSourceSlot(victimSlot)
		if entry != nil && entry.Combatant.IsPC() {
			victim := turnQueueIdentity(entry)
			if g.combatPCEligibleForPossession(victim) {
				g.resolveMonsterPossession(monster, monsterIndex, victim, rng)

				return true
			}
		}
	}
	if monster.HasAbility(references.Invisibility) &&
		rng.Intn(monsterSpecialRollRange) < monsterSpecialThreshold {
		visibility := CombatVisibilityHidden
		if monster.IsInvisible() {
			monster.SetInvisible(false)
			visibility = CombatVisibilityRevealed
		} else {
			monster.SetInvisible(true)
		}
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind:       CombatEventVisibility,
			Attacker:   combatMonsterFeedbackEntity(monsterIndex, monster.Position()),
			From:       monster.Position(),
			To:         monster.Position(),
			Visibility: visibility,
		})

		return true
	}
	if monster.HasAbility(references.GatesInDaemon) &&
		rng.Intn(monsterSpecialRollRange) < monsterSpecialThreshold {
		position := references.Position{
			X: references.Coordinate(rng.Intn(daemonRandomSquareRange)),
			Y: references.Coordinate(rng.Intn(daemonRandomSquareRange)),
		}
		if position.X < references.XCombatMapTiles && position.Y < references.YCombatMapTiles &&
			g.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
			daemonRef, err := g.enemyReferenceForCombat(references.MonsterDevil)
			if err != nil {
				panic("daemon reference is unavailable: " + err.Error())
			}
			// Validate the candidate using Devil terrain permissions rather than
			// the summoner’s appearance. ref:COMBAT-0008
			probe := &CombatMonster{monsterType: references.MonsterDevil, enemyRef: daemonRef}
			if isCombatPositionValidForMonster(g, position, probe, -1) {
				created := NewCombatMonster(references.MonsterDevil, daemonRef, position, rng)
				created.summoned = true
				if !g.CombatScene.hasActiveObjectSlot() {
					// REDUX DEVIATION: Original combatant allocation consumes the DEX roll and leaves
					// a partial combat queue entry when object slots is full. Redux preserves the RNG
					// consumption but does not retain an unusable queue entry.
					// See ref:COMBAT-0008.
					return false
				}
				createdIndex := g.CombatScene.AddMonsterToQueue(created)
				g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
					Kind:     CombatEventGating,
					Attacker: combatMonsterFeedbackEntity(monsterIndex, monster.Position()),
					Created: CombatSummonedMonster{
						CombatantID: created.CombatantID(), Index: createdIndex,
						MonsterType: references.MonsterDevil, Position: position,
					},
					From: monster.Position(), To: position,
				})

				return true
			}
		}
	}

	return false
}

func (g *GameState) combatPCEligibleForPossession(identity CombatTurnIdentity) bool {
	entry := g.CombatScene.TurnQueue.GetEntryByIdentity(identity)
	if !identity.IsPartyMember || entry == nil || entry.status != turnQueueActive ||
		!g.CombatScene.isCombatPCActive(identity) ||
		g.CombatScene.isCombatPCInvisible(identity) ||
		g.CombatScene.isCombatPCUnderground(identity) ||
		g.CombatScene.isCombatPCPossessed(identity) || g.CombatScene.isCombatPCSleeping(identity) {
		return false
	}
	character := g.PartyState.GetCharacter(identity.EntityIndex)

	return !character.IsDead() && character.GetStatus() != party_state.Charmed
}

func (g *GameState) resolveMonsterPossession(
	daemon *CombatMonster,
	daemonIndex int,
	victimIdentity CombatTurnIdentity,
	rng references.RandomProvider,
) {
	victim := g.PartyState.GetCharacter(victimIdentity.EntityIndex)
	if !g.combatPCEligibleForPossession(victimIdentity) {
		return
	}
	failedSave := CombatHitCheck(
		daemon.EnemyRef().Intelligence, int(victim.Intelligence), false, rng,
	)
	if failedSave.Outcome == CombatHitMiss {
		return
	}

	if !g.CombatScene.acquireCombatPossession(victimIdentity, daemon.CombatantID()) {
		return
	}
	g.clearInactiveCombatPCSelection(victimIdentity.EntityIndex)
	victimPosition := g.CombatScene.GetCombatantPosition(victimIdentity)
	event := CombatEvent{
		Kind:     CombatEventPossession,
		Attacker: combatMonsterFeedbackEntity(daemonIndex, daemon.Position()),
		Target:   combatFeedbackIdentity(victimIdentity, victimPosition),
		Result:   CombatFeedbackDamage,
		From:     daemon.Position(), To: victimPosition,
		Diagnostics:         CombatDiagnostics{Hit: failedSave},
		PossessionEpisodeID: g.CombatScene.possessions[victimIdentity].id,
	}
	g.SystemCallbacks.Combat.PublishEvent(event)
	if daemon.MonsterType() == references.MonsterDevil {
		g.retirePossessingDaemon(daemon, daemonIndex)
	}
}

func (g *GameState) retirePossessingDaemon(daemon *CombatMonster, daemonIndex int) {
	daemon.SetActive(false)
	g.CombatScene.TurnQueue.MarkFled(daemonIndex, false)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind:     CombatEventRetired,
		Attacker: combatMonsterFeedbackEntity(daemonIndex, daemon.Position()),
		From:     daemon.Position(), To: daemon.Position(),
	})
}

func (g *GameState) monsterAttackWithRNG(
	monster *CombatMonster,
	monsterIndex int,
	nearest nearestPCResult,
	rng references.RandomProvider,
) bool {
	_, attacked := g.prepareMonsterAttackWithRNG(monster, monsterIndex, nearest, rng)
	if attacked && g.CombatScene.pendingMonsterMissile != nil {
		g.ResolvePendingMonsterMissile()
	}

	return attacked
}

func (g *GameState) prepareMonsterAttack(
	monster *CombatMonster,
	monsterIndex int,
	nearest nearestPCResult,
) (*CombatProjectileLaunch, bool) {
	return g.prepareMonsterAttackWithRNG(monster, monsterIndex, nearest, g.AsRandomProvider())
}

func (g *GameState) prepareMonsterAttackWithRNG(
	monster *CombatMonster,
	monsterIndex int,
	nearest nearestPCResult,
	rng references.RandomProvider,
) (*CombatProjectileLaunch, bool) {
	if !nearest.found {
		return nil, false
	}
	forcedMiss := false
	if nearest.identity.IsPartyMember && monster.HasAbility(references.RangedMagic) {
		character := g.PartyState.GetCharacter(nearest.identity.EntityIndex)
		if character.GetEquippedItem(references.SlotAmulet) == references.AmuletOfTurning {
			forcedMiss = rng.Intn(monsterSpecialRollRange) < daemonMissileThreshold
		}
	}
	if nearest.distance > monster.EnemyRef().AttackRange {
		return nil, false
	}
	if nearest.distance == monsterAdjacentDistance {
		return nil, g.monsterMeleeAttack(monster, monsterIndex, nearest)
	}
	if monster.MonsterType() != references.MonsterMimic &&
		rng.Intn(monsterSpecialRollRange) >= daemonMissileThreshold {
		return nil, false
	}
	if monster.HasAbility(references.RangedMagic) && g.hostileMonsterMagicIsNegated() {
		return nil, false
	}
	if g.CombatScene.pendingMonsterMissile != nil {
		panic("hostile monster missile prepared while another missile is pending")
	}

	from := monster.Position()
	to := g.combatTurnPosition(nearest.identity)
	diagnostics := CombatDiagnostics{
		Missile:      monster.EnemyRef().LargeMapMissile,
		ForcedMiss:   forcedMiss,
		UsedStrength: monster.HasAbility(references.Bludgeons),
	}
	aim := to
	if !forcedMiss {
		defense := g.combatTargetDefense(nearest.identity)
		attackerStat := g.combatPhysicalAttackAttribute(CombatTurnIdentity{EntityIndex: monsterIndex}, references.NoEquipment)
		hit := CombatHitCheck(attackerStat, defense.EffectiveDex, false, rng)
		diagnostics.Hit = hit
		diagnostics.TargetBaseDex = defense.BaseDex
		if hit.Outcome == CombatHitMiss {
			aim = randomAdjacentCombatAttackMissAimWithRNG(to, from, monster.EnemyRef().AttackRange, rng)
		}
	} else {
		aim = randomAdjacentCombatAttackMissAimWithRNG(to, from, monster.EnemyRef().AttackRange, rng)
	}
	launch := traceCombatProjectileLaunch(g.combatMissileTerrainAt, from, aim, diagnostics.Missile)
	diagnostics.ProjectileBlocked = launch.Blocked
	pending := &pendingMonsterMissile{
		attackerIndex:     monsterIndex,
		directedTarget:    nearest.identity,
		directedTargetSet: true,
		launch:            launch,
		diagnostics:       diagnostics,
	}
	if !launch.Blocked {
		if identity, found := g.combatMissileImpactOccupantAt(launch.Impact); found &&
			identity != (CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIndex}) {
			pending.target = identity
			pending.targetSet = true
		}
	}
	g.CombatScene.pendingMonsterMissile = pending

	return &launch, true
}

type pendingMonsterMissile struct {
	attackerIndex        int
	directedTarget       CombatTurnIdentity
	directedTargetSet    bool
	launch               CombatProjectileLaunch
	target               CombatTurnIdentity
	targetSet            bool
	diagnostics          CombatDiagnostics
	resolveAttackerField bool
}

// ResolvePendingMonsterMissile applies a prepared hostile missile exactly once.
func (g *GameState) ResolvePendingMonsterMissile() bool {
	if g.CombatScene == nil || g.CombatScene.pendingMonsterMissile == nil {
		return false
	}
	pending := *g.CombatScene.pendingMonsterMissile
	g.CombatScene.pendingMonsterMissile = nil
	attacker := g.CombatScene.MonsterManager.GetMonster(pending.attackerIndex)
	if attacker == nil {
		return false
	}
	if pending.directedTargetSet {
		defer g.finishDirectedCombatAttack(pending.directedTarget, g.CombatScene.isCombatPCSleeping(pending.directedTarget))
	}
	if pending.targetSet && !pending.launch.Blocked {
		defer g.finishDirectedCombatAttack(pending.target, g.CombatScene.isCombatPCSleeping(pending.target))
	}

	diagnostics := pending.diagnostics
	result := CombatFeedbackMiss
	hpLost := 0
	fatal := false
	var injury CombatMonsterInjury
	if !pending.launch.Blocked {
		g.CheckTrap(&pending.launch.Impact)
	}
	if !pending.launch.Blocked && pending.targetSet {
		resolution := g.resolveSuccessfulMonsterHit(
			attacker, pending.target, false, g.AsRandomProvider(), &diagnostics,
		)
		if resolution.replacedDamage {
			result = resolution.feedback
			if resolution.damageSelected {
				hpLost, fatal, injury = g.applyMonsterDamageToTarget(
					pending.target, resolution.damage, attacker, pending.attackerIndex, diagnostics,
				)
			}
		} else {
			var defenderArmor int
			if pending.target.IsPartyMember {
				defenderArmor = g.combatPartyDefense(g.PartyState.GetCharacter(pending.target.EntityIndex))
			} else {
				target := g.CombatScene.MonsterManager.GetMonster(pending.target.EntityIndex)
				defenderArmor = target.EnemyRef().Armour
				diagnostics.TargetUndead = target.MonsterType().IsUndead()
				diagnostics.TargetImmortal = target.HasAbility(references.Immortal)
				// USES_MAGIC controls Turning and Negate Magic, but monster missile effects
				// still resolves through the ordinary physical whack/damage calculation path.
				diagnostics.MagicWeapon = false
			}
			damage := CombatCalcDamageRaw(attacker.EnemyRef().Damage, defenderArmor, g.AsRandomProvider())
			diagnostics.Damage = damage
			diagnostics.FinalDamage = CombatApplyDamageModifiers(
				damage.Damage, diagnostics.TargetUndead, diagnostics.TargetImmortal, diagnostics.MagicWeapon,
			)
			if damage.Deflected {
				result = CombatFeedbackGraze
			} else {
				result = CombatFeedbackDamage
				hpLost, fatal, injury = g.applyMonsterDamageToTarget(
					pending.target, diagnostics.FinalDamage, attacker, pending.attackerIndex, diagnostics,
				)
			}
		}
		g.SystemCallbacks.Visual.ShowCombatImpact(CombatAttackFeedback{
			Attacker: combatMonsterFeedbackEntity(pending.attackerIndex, pending.launch.From),
			Target:   combatFeedbackIdentity(pending.target, pending.launch.Impact),
			Result:   result, HPLost: hpLost, Fatal: fatal,
			Presentation: CombatAttackPresentation{
				Delivery: CombatAttackDeliveryProjectile,
				Source:   references.NoEquipment,
			},
		})
	}

	event := CombatEvent{
		Kind:     CombatEventRangedAttack,
		Attacker: combatMonsterFeedbackEntity(pending.attackerIndex, pending.launch.From),
		Result:   result, HPLost: hpLost, Fatal: fatal,
		From: pending.launch.From, To: pending.launch.Impact, Diagnostics: diagnostics,
		MonsterInjury: injury,
	}
	if pending.targetSet {
		event.Target = combatFeedbackIdentity(pending.target, pending.launch.Impact)
	}
	g.SystemCallbacks.Combat.PublishEvent(event)
	if pending.resolveAttackerField && attacker.IsActive() && !attacker.IsDead() {
		g.resolveCombatMonsterFieldEffects(attacker, pending.attackerIndex)
	}

	return true
}

func (g *GameState) resolveMonsterMeleeAgainstMonster(
	attacker *CombatMonster,
	attackerIndex int,
	target *CombatMonster,
	targetIndex int,
) {
	if target == nil {
		return
	}
	rng := g.AsRandomProvider()
	targetIdentity := CombatTurnIdentity{IsPartyMember: false, EntityIndex: targetIndex}
	defense := g.combatTargetDefense(targetIdentity)
	hit := CombatHitCheck(g.combatPhysicalAttackAttribute(CombatTurnIdentity{EntityIndex: attackerIndex}, references.NoEquipment), defense.EffectiveDex, false, rng)
	diagnostics := CombatDiagnostics{Hit: hit, TargetBaseDex: defense.BaseDex, UsedStrength: attacker.HasAbility(references.Bludgeons)}
	if hit.Outcome == CombatHitMiss {
		g.publishMonsterAttackEvent(CombatEventAttack, attackerIndex,
			CombatTurnIdentity{IsPartyMember: false, EntityIndex: targetIndex},
			CombatFeedbackMiss, 0, false, diagnostics, CombatMonsterInjury{})

		return
	}
	resolution := g.resolveSuccessfulMonsterHit(attacker, targetIdentity, true, rng, &diagnostics)
	if resolution.replacedDamage {
		hpLost := 0
		fatal := false
		var injury CombatMonsterInjury
		if resolution.damageSelected {
			hpLost, fatal, injury = g.applyMonsterDamageToTarget(
				targetIdentity, resolution.damage, attacker, attackerIndex, diagnostics,
			)
		}
		g.publishMonsterAttackEvent(
			CombatEventAttack, attackerIndex, targetIdentity,
			resolution.feedback, hpLost, fatal, diagnostics, injury,
		)

		return
	}
	damage := CombatCalcDamageRaw(attacker.EnemyRef().Damage, target.EnemyRef().Armour, rng)
	diagnostics.Damage = damage
	diagnostics.FinalDamage = CombatApplyDamageModifiers(
		damage.Damage, target.HasAbility(references.Undead), target.HasAbility(references.Immortal), false,
	)
	result := CombatFeedbackDamage
	if damage.Deflected {
		result = CombatFeedbackGraze
	}
	hpLost, fatal, injury := g.applyMonsterDamageToTarget(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: targetIndex},
		diagnostics.FinalDamage, attacker, attackerIndex, diagnostics,
	)
	g.publishMonsterAttackEvent(CombatEventAttack, attackerIndex,
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: targetIndex}, result, hpLost, fatal, diagnostics, injury)
}

func (g *GameState) applyMonsterDamageToTarget(
	target CombatTurnIdentity,
	damage int,
	_ *CombatMonster,
	_ int,
	diagnostics CombatDiagnostics,
) (int, bool, CombatMonsterInjury) {
	if target.IsPartyMember {
		character := g.PartyState.GetCharacter(target.EntityIndex)
		hpBefore := character.GetCurrentHp()
		character.SubtractHp(uint16(combatMonsterDamageToApply(
			int(hpBefore), damage, diagnostics.Damage.InstantKill,
		)))
		if character.IsDead() {
			g.markCombatPCDead(target.EntityIndex)
		}

		return int(hpBefore - character.GetCurrentHp()), character.IsDead(), CombatMonsterInjury{}
	}
	monster := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
	hpBefore := monster.CurrentHP()
	monster.TakeDamage(combatMonsterDamageToApply(
		hpBefore, damage, diagnostics.Damage.InstantKill,
	))
	hpLost, fatal := hpBefore-monster.CurrentHP(), monster.IsDead()
	injury := g.captureCombatMonsterInjury(target, hpLost)
	if hpBefore > 0 && monster.IsDead() {
		g.finalizeCombatMonsterDeathWithPolicy(monster, target.EntityIndex, nil, NoTargetPC, false, true)
	} else if !monster.IsDead() {
		g.resolveSurvivingCombatMonsterHit(monster, target.EntityIndex, !diagnostics.MagicDamage)
	}

	return hpLost, fatal, injury
}

func (g *GameState) publishMonsterAttackEvent(
	kind CombatEventKind,
	attackerIndex int,
	target CombatTurnIdentity,
	result CombatFeedbackResult,
	hpLost int,
	fatal bool,
	diagnostics CombatDiagnostics,
	injury CombatMonsterInjury,
) {
	attacker := g.CombatScene.MonsterManager.GetMonster(attackerIndex)
	feedback := CombatAttackFeedback{
		Attacker: combatMonsterFeedbackEntity(attackerIndex, attacker.Position()),
		Target:   combatFeedbackIdentity(target, g.combatTurnPosition(target)),
		Result:   result, HPLost: hpLost, Fatal: fatal,
		Presentation: combatAttackPresentationForEvent(
			kind,
			diagnostics,
			false,
		),
	}
	g.SystemCallbacks.Visual.ShowCombatImpact(feedback)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: kind, Attacker: feedback.Attacker, Target: feedback.Target,
		Result: result, HPLost: hpLost, Fatal: fatal,
		From: feedback.Attacker.Position, To: feedback.Target.Position, Diagnostics: diagnostics,
		MonsterInjury: injury,
	})
}

func combatMonsterFeedbackEntity(index int, position references.Position) CombatFeedbackEntity {
	return CombatFeedbackEntity{IsPartyMember: false, Index: index, Position: position}
}

func combatFeedbackIdentity(identity CombatTurnIdentity, position references.Position) CombatFeedbackEntity {
	return CombatFeedbackEntity{
		IsPartyMember: identity.IsPartyMember,
		Index:         identity.EntityIndex,
		Slot:          identity.Slot,
		Position:      position,
	}
}

func (g *GameState) combatTurnPosition(identity CombatTurnIdentity) references.Position {
	return g.CombatScene.GetCombatantPosition(identity)
}

type combatTargetDefenseResult struct {
	BaseDex      int
	EffectiveDex int
	Armor        int
}

// Original combat damage rules use encounter DEX for both physical attributes and saves.
// See ref:COMBAT-0008.
func (g *GameState) effectiveCombatDexterity(identity CombatTurnIdentity) int {
	if identity.IsPartyMember {
		return CombatGetEffectiveDex(int(g.PartyState.GetCharacter(identity.EntityIndex).Dexterity), g.CombatScene.isCombatPCSleeping(identity))
	}
	monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)

	return CombatGetEffectiveDex(monster.CurrentDex(), monster.IsAsleep() || monster.MonsterType() == references.MonsterMimic || g.HasActiveDurationEffect(DurationEffectTimeStop))
}

func (g *GameState) combatPhysicalAttackAttribute(identity CombatTurnIdentity, weapon references.Equipment) int {
	if identity.IsPartyMember {
		if weapon.IsBludgeoning() {
			return int(g.PartyState.GetCharacter(identity.EntityIndex).Strength)
		}
	} else {
		monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
		if monster.HasAbility(references.Bludgeons) {
			return monster.EnemyRef().Strength
		}
	}

	return g.effectiveCombatDexterity(identity)
}

func (g *GameState) combatTargetDefense(identity CombatTurnIdentity) combatTargetDefenseResult {
	if identity.IsPartyMember {
		character := g.PartyState.GetCharacter(identity.EntityIndex)
		baseDex := int(character.Dexterity)

		return combatTargetDefenseResult{
			BaseDex:      baseDex,
			EffectiveDex: g.effectiveCombatDexterity(identity),
			Armor:        g.combatPartyDefense(character),
		}
	}
	monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
	baseDex := monster.BaseDex()

	return combatTargetDefenseResult{
		BaseDex:      baseDex,
		EffectiveDex: g.effectiveCombatDexterity(identity),
		Armor:        monster.EnemyRef().Armour,
	}
}

func (g *GameState) planPossessedPCTurn(identity CombatTurnIdentity) possessedPCTurnPlan {
	if g.HasActiveDurationEffect(DurationEffectConfusion) {
		return possessedPCTurnPlan{}
	}
	nearest := g.findNearestOpponent(
		identity, CombatMonsterLoyaltyMonster, g.CombatScene.GetCombatantPosition(identity),
	)

	return possessedPCTurnPlan{Target: nearest.identity, HasTarget: nearest.found && nearest.distance == 1}
}

func (g *GameState) executePossessedPCTurn(identity CombatTurnIdentity) {
	rng := g.AsRandomProvider()
	attackTarget := g.selectPossessedPCOpponentWithRNG(identity, rng)
	if attackTarget.found && attackTarget.distance == monsterAdjacentDistance {
		g.possessedPCMeleeAttack(identity, attackTarget.identity)

		return
	}
	if g.combatPCBoxedIn(identity) {
		return
	}
	seekTarget := g.selectPossessedPCOpponentWithRNG(identity, rng)
	if seekTarget.hasDirection {
		g.possessedPCSeek(identity, seekTarget, rng)
	}
}

func (g *GameState) selectPossessedPCOpponentWithRNG(
	identity CombatTurnIdentity,
	rng references.RandomProvider,
) nearestPCResult {
	loyalty := g.combatIdentityLoyalty(identity)
	character := g.PartyState.GetCharacter(identity.EntityIndex)
	if loyalty == CombatMonsterLoyaltyMonster && g.HasActiveDurationEffect(DurationEffectConfusion) &&
		rollOriginalD30(rng) > int(character.Intelligence) { // Closest-opponent selection calls transformed d30.
		// See ref:RNG-0001.
		loyalty = CombatMonsterLoyaltyParty
	}
	nearest := g.findNearestOpponent(identity, loyalty, g.CombatScene.GetCombatantPosition(identity))
	if !nearest.found && g.recoverOnePossessedPC() {
		loyalty = g.combatIdentityLoyalty(identity)
		nearest = g.findNearestOpponent(identity, loyalty, g.CombatScene.GetCombatantPosition(identity))
	}
	if !nearest.found {
		nearest = g.noOpponentMovement(identity)
	}

	return nearest
}

func (g *GameState) possessedPCMeleeAttack(attackerIdentity, target CombatTurnIdentity) {
	defer g.finishDirectedCombatAttack(target, g.CombatScene.isCombatPCSleeping(target))
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundCombatSwing)
	attackerPosition := g.CombatScene.GetCombatantPosition(attackerIdentity)
	targetPosition := g.combatTurnPosition(target)
	defense := g.combatTargetDefense(target)
	rng := g.AsRandomProvider()
	hit := CombatHitCheck(g.combatPhysicalAttackAttribute(attackerIdentity, references.TwoHSword), defense.EffectiveDex, false, rng)
	diagnostics := CombatDiagnostics{
		Hit: hit, Source: references.TwoHSword, TargetBaseDex: defense.BaseDex,
	}
	result := CombatFeedbackMiss
	hpLost := 0
	fatal := false
	var injury CombatMonsterInjury
	if hit.Outcome != CombatHitMiss {
		damage := CombatCalcDamage(references.TwoHSword, defense.Armor, rng)
		diagnostics.Damage = damage
		diagnostics.FinalDamage = damage.Damage
		if damage.Deflected {
			result = CombatFeedbackGraze
		} else {
			result = CombatFeedbackDamage
			if target.IsPartyMember {
				victim := g.PartyState.GetCharacter(target.EntityIndex)
				hpBefore := victim.GetCurrentHp()
				victim.SubtractHp(uint16(damage.Damage))
				hpLost = int(hpBefore - victim.GetCurrentHp())
				fatal = victim.IsDead()
				if fatal {
					g.markCombatPCDead(target.EntityIndex)
				}
			} else {
				monster := g.CombatScene.MonsterManager.GetMonster(target.EntityIndex)
				hpBefore := monster.CurrentHP()
				monster.TakeDamage(monster.ApplyDamageModifiers(damage.Damage, references.TwoHSword))
				hpLost = hpBefore - monster.CurrentHP()
				fatal = monster.IsDead()
				injury = g.captureCombatMonsterInjury(target, hpLost)
				if fatal {
					g.finalizeCombatMonsterDeathWithPolicy(monster, target.EntityIndex, nil, NoTargetPC, false, true)
				} else {
					g.resolveSurvivingCombatMonsterHit(monster, target.EntityIndex, true)
				}
			}
		}
	}
	attackerEntity := combatFeedbackIdentity(attackerIdentity, attackerPosition)
	targetEntity := combatFeedbackIdentity(target, targetPosition)
	feedback := CombatAttackFeedback{
		Attacker: attackerEntity, Target: targetEntity, Result: result, HPLost: hpLost, Fatal: fatal,
		Presentation: CombatAttackPresentation{
			Delivery: CombatAttackDeliveryMelee,
			Source:   references.TwoHSword,
		},
	}
	g.SystemCallbacks.Visual.ShowCombatImpact(feedback)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventAttack, Attacker: attackerEntity, Target: targetEntity,
		Result: result, HPLost: hpLost, Fatal: fatal,
		From: attackerEntity.Position, To: targetEntity.Position, Diagnostics: diagnostics,
		MonsterInjury: injury,
	})
}

func (g *GameState) combatPCBoxedIn(identity CombatTurnIdentity) bool {
	for _, direction := range [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
		if g.canPossessedPCMove(identity, direction[0], direction[1]) {
			return false
		}
	}

	return true
}

func (g *GameState) canPossessedPCMove(identity CombatTurnIdentity, dx, dy int) bool {
	from := g.CombatScene.GetCombatantPosition(identity)
	to := references.Position{X: from.X + references.Coordinate(dx), Y: from.Y + references.Coordinate(dy)}

	return (dx != 0 || dy != 0) && g.isCombatPositionValidForPC(&to, identity) &&
		!g.CombatScene.hasMonsterBlockingObjectAt(to)
}

func (g *GameState) possessedPCSeek(
	identity CombatTurnIdentity,
	nearest nearestPCResult,
	rng references.RandomProvider,
) {
	from := g.CombatScene.GetCombatantPosition(identity)
	tryMove := func(dx, dy int) bool {
		if !g.canPossessedPCMove(identity, dx, dy) {
			return false
		}
		to := references.Position{X: from.X + references.Coordinate(dx), Y: from.Y + references.Coordinate(dy)}
		g.CombatScene.setCombatantPosition(identity, to)
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind:     CombatEventMove,
			Attacker: combatFeedbackIdentity(identity, to),
			From:     from, To: to,
		})

		return true
	}
	if rng.Intn(monsterSeekAxisRollRange) > monsterSeekHorizontalThreshold && tryMove(nearest.dirX, 0) {
		return
	}
	if tryMove(0, nearest.dirY) {
		return
	}
	for range monsterRandomDirectionAttempts {
		direction := [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}[rng.Intn(4)]
		if tryMove(direction[0], direction[1]) {
			return
		}
	}
}

func (g *GameState) recoverOnePossessedPC() bool {
	for sourceSlot := range sourceMonsterSlotStart {
		entry := g.CombatScene.TurnQueue.getRetainedEntryAtSourceSlot(sourceSlot)
		if entry == nil || !entry.Combatant.IsPC() {
			continue
		}
		identity := turnQueueIdentity(entry)
		if !g.CombatScene.isCombatPCPossessed(identity) {
			continue
		}
		g.CombatScene.setCombatPCPossessed(identity, false)
		character := g.PartyState.GetCharacter(identity.EntityIndex)
		g.SystemCallbacks.Message.AddRowStr(character.GetNameAsString() + " passes out!")
		if character.GetEquippedItem(references.SlotWeapon) == references.ChaosSword {
			character.SetEquippedItem(references.SlotWeapon, references.NoEquipment)
		} else if character.GetEquippedItem(references.SlotShield) == references.ChaosSword {
			character.SetEquippedItem(references.SlotShield, references.NoEquipment)
		}
		if !character.IsDead() {
			g.setCombatPCSleeping(identity, true)
		}
		g.clearInactiveCombatPCSelection(identity.EntityIndex)
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind:   CombatEventRecovery,
			Target: combatFeedbackIdentity(identity, g.CombatScene.GetCombatantPosition(identity)),
		})

		return true
	}

	return false
}

func (g *GameState) clearAllCombatPossession() {
	if g.CombatScene == nil {
		return
	}
	clear(g.CombatScene.possessions)
	clear(g.CombatScene.PartyPossessed[:])
	for index := range g.CombatScene.partyClones {
		g.CombatScene.partyClones[index].possessed = false
	}
	g.CombatScene.possessionTurn = combatPossessionTurn{}
}

func (g *GameState) setActiveCombatTarget(target CombatTurnIdentity, hasTarget bool) {
	g.CombatScene.ActiveTargetIdentity = target
	g.CombatScene.ActiveTargetIdentitySet = hasTarget
	g.CombatScene.ActiveTargetPCIndex = NoTargetPC
	if hasTarget && target.IsPartyMember {
		g.CombatScene.ActiveTargetPCIndex = target.EntityIndex
	}
}

func (g *GameState) clearActiveCombatTarget() {
	g.CombatScene.ActiveTargetIdentitySet = false
	g.CombatScene.ActiveTargetPCIndex = NoTargetPC
}
