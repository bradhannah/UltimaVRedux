package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const vasFlamMaxDamage = 30

// CombatSpellTargeting tracks a combat spell's crosshair without consuming resources.
type CombatSpellTargeting struct {
	Request      CastRequest
	Caster       CombatTurnIdentity
	Origin       references.Position
	Cursor       references.Position
	MaxRange     int
	Recipient    CombatTurnIdentity
	RecipientSet bool
}

// CombatSpellTargetOutcome describes a combat spell targeting action.
type CombatSpellTargetOutcome int

const (
	// CombatSpellTargetIgnored means no spell targeting action was active.
	CombatSpellTargetIgnored CombatSpellTargetOutcome = iota
	// CombatSpellTargetInvalid means the selected square is not a visible foe.
	CombatSpellTargetInvalid
	// CombatSpellTargetCancelled is terminal for the accepted Cast action but preserves resources.
	CombatSpellTargetCancelled
	// CombatSpellTargetValidationFailed means final resource validation rejected the cast.
	CombatSpellTargetValidationFailed
	// CombatSpellTargetResolved means the spell was launched and consumed the action.
	CombatSpellTargetResolved
)

// CombatSpellTargetResult returns authoritative targeting and cast information to the UI adapter.
type CombatSpellTargetResult struct {
	Outcome    CombatSpellTargetOutcome
	Validation SpellValidationResult
	Spell      references.Spell
	Launch     CombatProjectileLaunch
}

type pendingCombatSpell struct {
	spell          references.Spell
	casterIdx      int
	caster         CombatTurnIdentity
	monsterIdx     int
	target         CombatTurnIdentity
	targetPosition references.Position
	targetSet      bool
	hit            CombatHitResult
	launch         CombatProjectileLaunch
}

// BeginCombatSpellTargeting validates and starts an area-targeted spell for the active PC.
// Resources remain untouched until a valid target is confirmed. This deliberately
// softens the original game's punitive pre-target resource consumption.
// REDUX DEVIATION: original prepared-spell rules charges before targeting. Redux rejects
// prerequisites and cancellation free; confirmed effect failure remains paid.
// See ref:SPELL-0001.
func (g *GameState) BeginCombatSpellTargeting(request CastRequest) SpellValidationResult {
	if g.CombatScene == nil || g.CombatScene.hasPlayerActionOwner() {
		return SpellValidationResult{Valid: false, ErrorMsg: "Thou canst not cast now!"}
	}

	activeIdx, active := g.activeCombatPartyMemberIndex()
	if !active || request.CasterIdx != activeIdx {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not thy turn!"}
	}

	validation := g.ValidateSpellResources(request)
	if !validation.Valid {
		return validation
	}
	profile := references.GetSpellTargetingProfile(request.Spell, references.ContextCombat)
	if profile.Selection != references.TargetCombatantCrosshair && profile.Selection != references.TargetTileCrosshair {
		return SpellValidationResult{Valid: false, ErrorMsg: "Nothing there!"}
	}
	if !isCombatProjectileSpell(request.Spell) && !IsCombatFieldSpell(request.Spell) {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not here!"}
	}

	casterIdentity := g.CombatScene.GetCurrentTurnIdentity()
	origin := g.CombatScene.GetCombatantPosition(casterIdentity)
	g.CombatScene.SpellTargeting = &CombatSpellTargeting{
		Request:  request,
		Caster:   casterIdentity,
		Origin:   origin,
		Cursor:   origin,
		MaxRange: combatProjectileMaxRange,
	}

	return validation
}

// MoveCombatSpellCursor moves the active spell crosshair within its original range and map bounds.
func (g *GameState) MoveCombatSpellCursor(deltaX, deltaY int) bool {
	if g.CombatScene == nil || g.CombatScene.SpellTargeting == nil || !isTargetingStep(deltaX, deltaY) {
		return false
	}

	targeting := g.CombatScene.SpellTargeting
	candidate := references.Position{
		X: targeting.Cursor.X + references.Coordinate(deltaX),
		Y: targeting.Cursor.Y + references.Coordinate(deltaY),
	}
	if candidate.X < 0 || candidate.X >= references.XCombatMapTiles ||
		candidate.Y < 0 || candidate.Y >= references.YCombatMapTiles ||
		integerDistance(targeting.Origin, candidate) > targeting.MaxRange {
		return false
	}

	targeting.Cursor = candidate
	if g.IsCombatPartySpellTargeting() {
		targeting.RecipientSet = false
		for _, identity := range g.CombatPartySpellRecipients() {
			if g.CombatScene.GetCombatantPosition(identity) == candidate {
				targeting.Recipient, targeting.RecipientSet = identity, true

				break
			}
		}
	}

	return true
}

// CancelCombatSpellTargeting clears an unresolved spell without consuming resources.
// Cancellation preserves the current combat action as well as MP and mixtures.
func (g *GameState) CancelCombatSpellTargeting() CombatSpellTargetOutcome {
	if g.CombatScene == nil || g.CombatScene.SpellTargeting == nil {
		return CombatSpellTargetIgnored
	}

	g.CombatScene.SpellTargeting = nil

	return CombatSpellTargetCancelled
}

// ConfirmCombatSpellTarget resolves a projectile combat spell at its crosshair.
func (g *GameState) ConfirmCombatSpellTarget() CombatSpellTargetResult {
	return g.confirmCombatSpellTargetWithRNG(g.AsRandomProvider())
}

func (g *GameState) confirmCombatSpellTargetWithRNG(rng references.RandomProvider) CombatSpellTargetResult {
	if g.CombatScene == nil || g.CombatScene.SpellTargeting == nil {
		return CombatSpellTargetResult{Outcome: CombatSpellTargetIgnored}
	}

	targeting := g.CombatScene.SpellTargeting
	result := CombatSpellTargetResult{
		Outcome: CombatSpellTargetInvalid,
		Spell:   targeting.Request.Spell,
		Launch: CombatProjectileLaunch{
			From:    targeting.Origin,
			Aim:     targeting.Cursor,
			Impact:  targeting.Cursor,
			Missile: references.MissileNone,
			Animate: true,
		},
	}
	if targeting.Cursor == targeting.Origin {
		return result
	}
	monster, monsterIdx := g.CombatScene.MonsterManager.GetMonsterIndexAtPosition(targeting.Cursor)
	target, occupied := g.combatSpellTargetAt(targeting.Cursor, monster, monsterIdx)
	damageProjectile := isCombatDamageProjectileSpell(targeting.Request.Spell)
	if !IsCombatFieldSpell(targeting.Request.Spell) &&
		((!occupied && !damageProjectile) ||
			(occupied && !g.isCombatSpellTargetValid(targeting.Request.Spell, monster, target))) {
		return result
	}

	if g.CombatScene.GetCurrentTurnIdentity() != targeting.Caster {
		g.CombatScene.SpellTargeting = nil
		result.Outcome = CombatSpellTargetValidationFailed
		result.Validation = SpellValidationResult{ErrorMsg: "Not thy turn!"}

		return result
	}
	validation := g.ValidateAndConsumeSpellResources(targeting.Request)
	result.Validation = validation
	if !validation.Valid {
		result.Outcome = CombatSpellTargetValidationFailed
		g.CombatScene.SpellTargeting = nil

		return result
	}

	if monster != nil {
		g.CombatScene.setLastCombatAttackTarget(targeting.Caster, monsterIdx)
	}
	hit := CombatHitResult{Outcome: CombatHitNeverMiss, AutomaticHit: true}
	aim := targeting.Cursor
	if targeting.Request.Spell == references.XenCorp && occupied {
		var targetIntelligence int
		if target.IsPartyMember {
			targetIntelligence = int(g.PartyState.GetCharacter(target.EntityIndex).Intelligence)
		} else {
			targetIntelligence = monster.EnemyRef().Intelligence
		}
		hit = rollCombatIntelligenceSaveWithRNG(
			int(g.PartyState.GetCharacter(targeting.Request.CasterIdx).Intelligence),
			targetIntelligence,
			rng,
		)
		if hit.Outcome == CombatHitMiss {
			aim = randomAdjacentCombatMissAimWithRNG(targeting.Cursor, targeting.Origin, rng)
		}
	}
	launch := traceCombatProjectileLaunch(
		g.combatMissileTerrainAt,
		targeting.Origin,
		aim,
		references.MissileNone,
	)
	// Original prepared-spell rules applies these directly after player aiming; their flight is cosmetic.
	// See ref:SPELL-0001.
	if targeting.Request.Spell == references.AnXenEx || targeting.Request.Spell == references.RelXenBet || targeting.Request.Spell == references.InQuasXen {
		launch = result.Launch
	}
	g.CombatScene.pendingSpell = &pendingCombatSpell{
		spell:          targeting.Request.Spell,
		casterIdx:      targeting.Request.CasterIdx,
		caster:         targeting.Caster,
		monsterIdx:     monsterIdx,
		target:         target,
		targetPosition: targeting.Cursor,
		targetSet:      occupied,
		hit:            hit,
		launch:         launch,
	}
	g.CombatScene.SpellTargeting = nil
	result.Launch = launch
	result.Outcome = CombatSpellTargetResolved

	return result
}

// ResolvePendingCombatSpell applies the confirmed spell when its projectile reaches impact.
func (g *GameState) ResolvePendingCombatSpell() bool {
	if g.CombatScene == nil || g.CombatScene.pendingSpell == nil {
		return false
	}

	pending := *g.CombatScene.pendingSpell
	g.CombatScene.pendingSpell = nil
	// Clone and protection spells are not attacks. A successfully delivered
	// sleep field applies fresh sleep, which takes precedence over awakening.
	if pending.targetSet && pending.spell != references.InQuasXen &&
		pending.spell != references.InSanctGrav &&
		(pending.spell != references.InZuGrav || pending.launch.Blocked) {
		defer g.finishDirectedCombatAttack(pending.target, g.CombatScene.isCombatPCSleeping(pending.target))
	}
	if pending.launch.Blocked {
		g.publishBlockedCombatSpell(pending)

		return true
	}
	if combatSpellTriggersDungeonMap(pending.spell) {
		g.CheckTrap(&pending.launch.Impact)
	}

	//nolint:exhaustive // Only pending combat projectile spells reach this switch.
	switch pending.spell {
	case references.GravPor, references.VasFlam, references.XenCorp:
		g.resolveCombatProjectileSpell(pending)
	case references.InFlamGrav, references.InNoxGrav, references.InZuGrav, references.InSanctGrav:
		g.resolveCombatFieldSpell(pending)
	case references.AnXenEx:
		g.CastCombatAnXenEx(pending.casterIdx, pending.target)
	case references.RelXenBet:
		g.CastCombatRelXenBet(pending.casterIdx, pending.target)
	case references.InQuasXen:
		// REDUX DEVIATION: original prepared-spell rules consume before clone placement's random
		// placement. Redux validates capacity/target free, but retains that paid
		// placement boundary with a bounded search rather than free rerolls.
		// See ref:SPELL-0001.
		if !g.CastCombatInQuasXen(pending.casterIdx, pending.target) {
			g.SystemCallbacks.UI.ShowActionFeedback("Failed! No room for a clone.", true)
			g.publishCombatSpellMutation(pending.spell, pending.casterIdx, pending.target, CombatFeedbackMiss)
		}
	default:
		panic("ResolvePendingCombatSpell: unsupported pending spell")
	}

	return true
}

func combatSpellTriggersDungeonMap(spell references.Spell) bool {
	//nolint:exhaustive // projectile launch calls room-trigger check only for these seven player spells.
	// See ref:SPELL-0001.
	switch spell {
	case references.GravPor, references.VasFlam,
		references.InFlamGrav, references.InNoxGrav, references.InZuGrav, references.InSanctGrav,
		references.XenCorp:
		return true
	default:
		return false
	}
}

func (g *GameState) publishBlockedCombatSpell(pending pendingCombatSpell) {
	var target CombatFeedbackEntity
	if pending.targetSet {
		target = combatFeedbackIdentity(pending.target, pending.targetPosition)
	}
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind:     CombatEventSpell,
		Attacker: combatFeedbackIdentity(pending.caster, pending.launch.From),
		Target:   target,
		Result:   CombatFeedbackMiss,
		From:     pending.launch.From,
		To:       pending.launch.Impact,
		Diagnostics: CombatDiagnostics{
			Hit:               pending.hit,
			Spell:             pending.spell,
			MagicDamage:       true,
			ProjectileBlocked: true,
		},
	})
}

// IsCombatFieldSpell reports the spells that place a field at a crosshair target.
func IsCombatFieldSpell(spell references.Spell) bool {
	//nolint:exhaustive // Only field spells return true.
	switch spell {
	case references.InFlamGrav, references.InNoxGrav, references.InZuGrav, references.InSanctGrav:
		return true
	default:
		return false
	}
}

const (
	combatProjectileMaxRange = 15
	gravPorMaxDamage         = 16
)

func isCombatProjectileSpell(spell references.Spell) bool {
	//nolint:exhaustive // Only implemented weapon-like combat spells return true.
	switch spell {
	case references.GravPor, references.VasFlam, references.XenCorp,
		references.AnXenEx, references.RelXenBet, references.InQuasXen:
		return true
	default:
		return false
	}
}

func isCombatDamageProjectileSpell(spell references.Spell) bool {
	//nolint:exhaustive // Only the three weapon-like damage spells select free tiles.
	switch spell {
	case references.GravPor, references.VasFlam, references.XenCorp:
		return true
	default:
		return false
	}
}

func (g *GameState) combatSpellTargetAt(
	position references.Position,
	monster *CombatMonster,
	monsterIndex int,
) (CombatTurnIdentity, bool) {
	if monster != nil && !monster.IsCorpserHidden() {
		return CombatTurnIdentity{EntityIndex: monsterIndex}, true
	}
	for index := range g.CombatScene.PartyActive {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: index}
		if g.CombatScene.PartyActive[index] && !g.CombatScene.isCombatPCUnderground(identity) &&
			g.CombatScene.PartyPositions[index] == position {
			return identity, true
		}
	}
	for _, clone := range g.CombatScene.partyClones {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: clone.partyIndex, Slot: clone.slot}
		if clone.position == position && !g.CombatScene.isCombatPCUnderground(identity) {
			return identity, true
		}
	}

	return CombatTurnIdentity{}, false
}

func (g *GameState) isCombatSpellTargetValid(
	spell references.Spell,
	monster *CombatMonster,
	target CombatTurnIdentity,
) bool {
	if monster != nil && monster.IsCorpserHidden() {
		return false
	}
	if target.IsPartyMember && g.CombatScene.isCombatPCUnderground(target) {
		return false
	}
	if isCombatDamageProjectileSpell(spell) {
		if target.IsPartyMember {
			return g.CombatScene.isCombatPCActive(target) &&
				g.combatIdentityLoyalty(target) == CombatMonsterLoyaltyMonster
		}

		return monster != nil && monster.IsActive() && !monster.IsDead() &&
			g.combatIdentityLoyalty(target) == CombatMonsterLoyaltyMonster
	}
	//nolint:exhaustive // This switch owns only the crosshair spell family.
	switch spell {
	case references.AnXenEx:
		return (monster != nil && monster.IsHostile()) ||
			(target.IsPartyMember && g.CombatScene.isCombatPCPossessed(target))
	case references.RelXenBet:
		return g.CombatScene.canReplaceCombatantWithMonster(target) &&
			(target.IsPartyMember || (monster != nil && monster.IsActive() && !monster.IsDead()))
	case references.InQuasXen:
		return g.CombatScene.TurnQueue.hasFreeSourceSlot(0) && g.CombatScene.hasActiveObjectSlot() &&
			(target.IsPartyMember || (monster != nil && monster.IsActive() && !monster.IsDead()))
	default:
		return false
	}
}

func (g *GameState) resolveCombatProjectileSpell(
	pending pendingCombatSpell,
) {
	if !pending.targetSet {
		return
	}
	if pending.spell == references.XenCorp && pending.hit.Outcome == CombatHitMiss {
		g.publishCombatProjectileSpellResult(
			pending.spell, pending.caster, pending.target, pending.targetPosition,
			pending.hit, CombatDamageResult{}, 0, 0, false, false, false,
		)

		return
	}

	if pending.target.IsPartyMember {
		g.resolveCombatProjectileSpellAgainstPC(pending)

		return
	}
	monster := g.CombatScene.MonsterManager.GetMonster(pending.target.EntityIndex)
	if monster == nil || !monster.IsActive() || monster.IsDead() {
		panic("ResolvePendingCombatSpell: confirmed target is no longer active")
	}
	if pending.spell == references.XenCorp {
		damage := CombatDamageResult{
			Damage:          combatInstantKillDamage,
			InstantKill:     true,
			AttackMax:       combatInstantKillDamage,
			AttackRoll:      combatInstantKillDamage,
			PreArmorDamage:  combatInstantKillDamage,
			PostArmorDamage: combatInstantKillDamage,
		}
		finalDamage := CombatApplyMagicDamageModifiers(damage.Damage, monster.MonsterType().IsUndead(), monster.HasAbility(references.Immortal))
		g.applyCombatProjectileDamage(pending, monster, damage, finalDamage)

		return
	}
	maxDamage := gravPorMaxDamage
	if pending.spell == references.VasFlam {
		maxDamage = vasFlamMaxDamage
	}
	damage := CombatCalcMagicDamage(maxDamage, monster.EnemyRef().Armour, g.AsRandomProvider())
	finalDamage := CombatApplyMagicDamageModifiers(
		damage.Damage,
		monster.MonsterType().IsUndead(),
		monster.HasAbility(references.Immortal),
	)
	g.applyCombatProjectileDamage(
		pending, monster, damage, finalDamage,
	)
}

func (g *GameState) applyCombatProjectileDamage(
	pending pendingCombatSpell,
	monster *CombatMonster,
	damage CombatDamageResult,
	finalDamage int,
) {
	hpBefore := monster.CurrentHP()
	if damage.InstantKill && finalDamage == combatInstantKillDamage {
		monster.SetCurrentHP(0)
	} else {
		monster.TakeDamage(finalDamage)
	}
	hpLost := hpBefore - monster.CurrentHP()
	g.publishCombatProjectileSpellResult(
		pending.spell, pending.caster, pending.target, pending.targetPosition,
		pending.hit, damage, finalDamage, hpLost, monster.IsDead(),
		monster.MonsterType().IsUndead(), monster.HasAbility(references.Immortal),
	)

	if hpBefore > 0 && monster.IsDead() {
		g.finalizeCombatMonsterDeath(
			monster, pending.target.EntityIndex,
			g.PartyState.GetCharacter(pending.caster.EntityIndex), pending.caster.EntityIndex,
		)
	} else {
		g.tryDivideCombatMonster(monster, pending.target.EntityIndex)
	}
}

func (g *GameState) resolveCombatProjectileSpellAgainstPC(pending pendingCombatSpell) {
	if !g.CombatScene.isCombatPCActive(pending.target) {
		panic("ResolvePendingCombatSpell: confirmed PC target is no longer active")
	}
	character := g.PartyState.GetCharacter(pending.target.EntityIndex)
	damage := CombatDamageResult{
		Damage:          combatInstantKillDamage,
		InstantKill:     true,
		AttackMax:       combatInstantKillDamage,
		AttackRoll:      combatInstantKillDamage,
		PreArmorDamage:  combatInstantKillDamage,
		PostArmorDamage: combatInstantKillDamage,
	}
	if pending.spell != references.XenCorp {
		maxDamage := gravPorMaxDamage
		if pending.spell == references.VasFlam {
			maxDamage = vasFlamMaxDamage
		}
		damage = CombatCalcMagicDamage(maxDamage, g.combatPartyDefense(character), g.AsRandomProvider())
	}
	hpBefore := character.GetCurrentHp()
	if damage.InstantKill {
		character.SetHp(0)
		character.MarkAsDead()
	} else {
		character.SubtractHp(uint16(damage.Damage))
	}
	hpLost := int(hpBefore - character.GetCurrentHp())
	fatal := character.IsDead()
	g.publishCombatProjectileSpellResult(
		pending.spell, pending.caster, pending.target, pending.targetPosition,
		pending.hit, damage, damage.Damage, hpLost, fatal, false, false,
	)
	if fatal {
		g.markCombatPCDead(pending.target.EntityIndex)
	}
}

func (g *GameState) publishCombatProjectileSpellResult(
	spell references.Spell,
	casterIdentity CombatTurnIdentity,
	targetIdentity CombatTurnIdentity,
	targetPosition references.Position,
	hit CombatHitResult,
	damage CombatDamageResult,
	finalDamage int,
	hpLost int,
	fatal bool,
	targetUndead bool,
	targetImmortal bool,
) {
	feedbackResult := CombatFeedbackDamage
	injury := g.captureCombatMonsterInjury(targetIdentity, hpLost)
	if hit.Outcome == CombatHitMiss {
		feedbackResult = CombatFeedbackMiss
	} else if hpLost == 0 {
		feedbackResult = CombatFeedbackGraze
	}
	feedback := CombatAttackFeedback{
		Attacker: combatFeedbackIdentity(casterIdentity, g.CombatScene.GetCombatantPosition(casterIdentity)),
		Target:   combatFeedbackIdentity(targetIdentity, targetPosition),
		Result:   feedbackResult,
		HPLost:   hpLost,
		Fatal:    fatal,
		Presentation: CombatAttackPresentation{
			Delivery: CombatAttackDeliverySpell,
			Source:   references.NoEquipment,
		},
	}
	g.SystemCallbacks.Visual.ShowCombatImpact(feedback)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind:          CombatEventSpell,
		Attacker:      feedback.Attacker,
		Target:        feedback.Target,
		Result:        feedback.Result,
		HPLost:        feedback.HPLost,
		Fatal:         feedback.Fatal,
		MonsterInjury: injury,
		From:          feedback.Attacker.Position,
		To:            feedback.Target.Position,
		Diagnostics: CombatDiagnostics{
			Hit:            hit,
			Damage:         damage,
			FinalDamage:    finalDamage,
			TargetUndead:   targetUndead,
			TargetImmortal: targetImmortal,
			Spell:          spell,
			MagicDamage:    true,
		},
	})
}
