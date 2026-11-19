package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ActionAttackSmallMap resolves mirror breaking, attacks on defenseless NPCs,
// or an eligible NPC's combat encounter in the requested direction.
func (g *GameState) ActionAttackSmallMap(direction references.Direction) bool {
	// Get target position based on direction
	newPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if newPosition == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("What?", true)

		return false
	}

	// Get current map (use helper to get correct map type - dungeons, towns, etc.)
	currentMap := g.GetLayeredMapByCurrentLocation()
	terrainTile := currentMap.GetTileTopMapOnlyTile(newPosition)

	// Town Attack checks intact mirrors before object lookup, so an occupant
	// does not turn mirror breaking into an NPC attack. ref:CRIME-0004
	if terrainTile != nil && (terrainTile.Is(indexes.Mirror) || terrainTile.Is(indexes.MirrorAvatar)) {
		currentMap.SetTileByLayer(map_state.MapOverrideLayer, newPosition, indexes.MirrorBroken)
		g.SystemCallbacks.UI.ShowActionFeedback("Broken!", true)
		g.SystemCallbacks.Audio.Play(SoundGlassBreak).Start()
		g.FinishTurn()

		return true
	}

	targetTile := currentMap.GetTopTile(newPosition)

	// Mirror precedence has been resolved; object lookup now owns NPC attacks.
	npcUnit := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		*newPosition,
		g.MapState.PlayerLocation.Floor,
	)
	if npcUnit != nil {
		switch source := (*npcUnit).(type) {
		case *map_units.NPCFriendly:
			baseTile := currentMap.GetTileTopMapOnlyTile(newPosition)
			if baseTile != nil && g.isDefenselessBaseTile(baseTile.Index) {
				return g.handleDefenselessAttack(*newPosition, source)
			}
			if _, err := g.StartSmallMapCombatEncounter(source, SmallMapEncounterAttack); err != nil {
				logging.Errorf(logging.CategoryGameState, "Unable to attack small-map NPC: %v", err)
				g.SystemCallbacks.UI.ShowActionFeedback("Combat could not begin.", true)

				return false
			}
		case *map_units.NPCEnemy:
			if _, hasIdentity := source.ShadowlordIdentity(); !hasIdentity {
				g.SystemCallbacks.UI.ShowActionFeedback("Nothing to attack!", true)

				return false
			}
			if _, err := g.StartSmallMapCombatEncounter(source, SmallMapEncounterAttack); err != nil {
				logging.Errorf(logging.CategoryGameState, "Unable to attack small-map Shadowlord: %v", err)
				g.SystemCallbacks.UI.ShowActionFeedback("Combat could not begin.", true)

				return false
			}
		default:
			g.SystemCallbacks.UI.ShowActionFeedback("Nothing to attack!", true)

			return false
		}

		return true
	}

	if targetTile == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to attack!", true)

		return false
	}

	// For now, all other attack targets show "Nothing to attack!"
	g.SystemCallbacks.UI.ShowActionFeedback("Nothing to attack!", true)

	return false
}

// isDefenselessBaseTile checks if a base tile is one where NPCs are defenseless.
// Based on original town rules which checks for STOCKS, MANACLES, BED, BROKEN_MIRROR.
// This checks the BASE tile layer (what the NPC is standing on), not the rendered NPC sprite.
// See ref:CRIME-0004.
func (g *GameState) isDefenselessBaseTile(tileIndex indexes.SpriteIndex) bool {
	return tileIndex == indexes.Stocks || tileIndex == indexes.Manacles ||
		tileIndex == indexes.LeftBed || tileIndex == indexes.MirrorBroken
}

// handleDefenselessAttack applies the initial town-attack crime before
// Blackthorn immunity or additional murder consequences. ref:KARMA-0003
func (g *GameState) handleDefenselessAttack(
	position references.Position,
	victim *map_units.NPCFriendly,
) bool {
	g.validateDefenselessAttack(position, victim)

	g.AdjustPartyKarma(-5)
	if _, err := g.ActivateGuards(); err != nil {
		panic("prevalidated defenseless attack could not activate guards: " + err.Error())
	}
	g.SetCriminalStatus(true)

	if victim.GetKeySpriteIndex() == indexes.Blackthorn1 {
		g.SystemCallbacks.UI.ShowActionFeedback("Missed!", true)
		g.FinishTurn()

		return true
	}

	g.SystemCallbacks.UI.ShowActionFeedback("Murdered!", true)
	g.AdjustPartyKarma(-5)
	g.SystemCallbacks.Visual.PlayKapowAtPosition(position)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundWhiteNoiseDamage)

	location := g.MapState.PlayerLocation.Location
	if murderedNPCDeathPersists(victim) {
		g.PartyState.SetDeadNpc(location, victim.MapUnitDetails().NPCNum)
	}
	if !g.CurrentNPCAIController.GetNpcs().RemoveNPCAtPositionAndFloor(
		position,
		g.MapState.PlayerLocation.Floor,
	) {
		panic("prevalidated defenseless attack victim could not be removed")
	}
	g.FinishTurn()

	return true
}

func (g *GameState) validateDefenselessAttack(
	position references.Position,
	victim *map_units.NPCFriendly,
) {
	if victim == nil || g.MapState.PlayerLocation.Location.GetMapType() != references.SmallMapType ||
		g.IsInCombat() {
		panic("defenseless attack requires a live small-map victim")
	}
	if _, ok := g.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap); !ok {
		panic("defenseless attack requires a small-map NPC controller")
	}
	if !victim.IsVisible() || victim.Pos() != position ||
		victim.Floor() != g.MapState.PlayerLocation.Floor ||
		victim.NPCReference.Location != g.MapState.PlayerLocation.Location ||
		victim.MapUnitDetails().NPCNum < 0 || victim.MapUnitDetails().NPCNum >= party_state.NPcsPerLocation {
		panic("defenseless attack victim has no active town identity")
	}
	live := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		position,
		g.MapState.PlayerLocation.Floor,
	)
	if live == nil || *live != victim {
		panic("defenseless attack victim identity changed")
	}
	player := g.MapState.PlayerLocation.Position
	distance := absInt(int(position.X-player.X)) + absInt(int(position.Y-player.Y))
	baseTile := g.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(&position)
	if distance != 1 || baseTile == nil || !g.isDefenselessBaseTile(baseTile.Index) {
		panic("defenseless attack target is no longer valid")
	}
}

// Persist ordinary NPC removal except Guards; keep the separate quest-object
// exception outside ordinary attack admission. ref:CRIME-0004
func murderedNPCDeathPersists(victim *map_units.NPCFriendly) bool {
	sprite := victim.GetKeySpriteIndex()
	crawlerSprite := indexes.MageKeyIndex + indexes.SpriteIndex(references.MonsterCrawler)*4

	return (sprite != indexes.GuardKeyIndex && sprite < indexes.Seahorse1) || sprite == crawlerSprite
}

// ActionAttackLargeMap starts combat with a visible eligible enemy in the
// adjacent wrapped cardinal square.
func (g *GameState) ActionAttackLargeMap(direction references.Direction) bool {
	if g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

		return false
	}
	if direction != references.Up && direction != references.Right &&
		direction != references.Down && direction != references.Left {
		g.SystemCallbacks.UI.ShowActionFeedback("What?", true)

		return false
	}

	targetPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position).
		GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
	enemy := g.GetCurrentLargeMapNPCAIController().GetNpcs().
		GetVisibleEnemyAtPositionAndFloorOrNil(*targetPosition, g.MapState.PlayerLocation.Floor)
	if enemy == nil || !isResolvableOverworldContact(ClassifyOverworldEnemyContact(enemy)) {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to attack!", true)

		return false
	}

	if err := g.StartOverworldCombatEncounter(enemy); err != nil {
		logging.Errorf(logging.CategoryGameState, "Unable to attack overworld enemy: %v", err)
		g.SystemCallbacks.UI.ShowActionFeedback("Combat could not begin.", true)

		return false
	}

	return true
}

func (g *GameState) activeCombatPartyMemberIndex() (int, bool) {
	if g.CombatScene == nil {
		return -1, false
	}
	identity := g.CombatScene.GetCurrentTurnIdentity()
	if identity.IsPartyMember && g.CombatScene.isCombatPCUnderground(identity) {
		return -1, false
	}
	if identity.IsPartyMember && g.CombatScene.isCombatPCActive(identity) &&
		!g.CombatScene.isCombatPCUnderground(identity) {
		return identity.EntityIndex, true
	}

	activeIndex := g.CombatScene.GetActiveCombatantIndex()
	activeIdentity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: activeIndex}
	active := activeIndex >= 0 && activeIndex < party_state.MaxActiveParty &&
		g.CombatScene.IsPartyMemberActive(activeIndex) &&
		!g.CombatScene.isCombatPCUnderground(activeIdentity)

	return activeIndex, active
}

func (g *GameState) activeCombatPCIdentity() (CombatTurnIdentity, bool) {
	if g.CombatScene == nil {
		return CombatTurnIdentity{}, false
	}
	identity := g.CombatScene.GetCurrentTurnIdentity()
	if identity.IsPartyMember && g.CombatScene.isCombatPCActive(identity) &&
		!g.CombatScene.isCombatPCUnderground(identity) {
		return identity, true
	}

	return CombatTurnIdentity{}, false
}

// resolveCombatAttackSourceAtPosition resolves one item from the per-source
// pc_attack() sequence against an explicit combat-map square.
func (g *GameState) resolveCombatAttackSourceAtPosition(
	attackerIndex int,
	source CombatAttackSource,
	targetPos references.Position,
) {
	monster, monsterIndex := g.CombatScene.MonsterManager.GetMonsterIndexAtPosition(targetPos)
	if monster == nil || !monster.IsHostile() || monster.IsCorpserHidden() {
		return
	}
	// Physical attack resolution sounds its swing before the hit check. A separate
	// result event owns miss/graze/impact feedback once this source resolves.
	// See ref:COMBAT-0004.
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundCombatSwing)

	character := g.PartyState.GetCharacter(attackerIndex)
	rng := g.AsRandomProvider()
	if source.Equipment == references.NoEquipment {
		g.performBareHandsAttack(character, monster, monsterIndex, attackerIndex, rng)
	} else {
		g.performSingleAttack(
			character,
			source.Equipment,
			source.Slot,
			monster,
			monsterIndex,
			attackerIndex,
			rng,
		)
	}
}

// performSingleAttack resolves one attack from an equipment slot against a monster.
// Replicates the original attack attempt → hit check → damage calculation → damage application pipeline.
//
// See ref:COMBAT-0004.

func (g *GameState) performSingleAttack(
	character *party_state.PlayerCharacter,
	equipment references.Equipment,
	slot references.EquipmentSlot,
	monster *CombatMonster,
	monsterIdx int,
	pcIdx int,
	rng references.RandomProvider,
) {
	// Determine attacker stat: STR for bludgeoning weapons, DEX for all others
	// Original: attack attribute selection → if attack category[weapon] == 8 → STR, else DEX
	// See ref:COMBAT-0004.
	attackerStat := g.combatPhysicalAttackAttribute(g.combatPartyAttackerIdentity(pcIdx), equipment)

	defense := g.combatTargetDefense(CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIdx})

	// Determine never-miss: Glass Sword, Jeweled Sword, Chaos Sword
	neverMiss := equipment.HasAbility(references.AbilityShatters) ||
		equipment.HasAbility(references.AbilityNeverMisses) ||
		equipment.HasAbility(references.AbilityCharmsWielder)

	// Hit check (replicates hit check from original combat damage rules)
	// See ref:COMBAT-0004.
	hitResult := CombatHitCheck(attackerStat, defense.EffectiveDex, neverMiss, rng)
	diagnostics := CombatDiagnostics{
		Hit:           hitResult,
		Source:        equipment,
		TargetBaseDex: defense.BaseDex,
		UsedStrength:  equipment.IsBludgeoning(),
	}
	if hitResult.Outcome == CombatHitMiss {
		g.showPlayerCombatFeedback(pcIdx, monsterIdx, CombatFeedbackMiss, 0, false, diagnostics)

		return
	}
	g.resolvePlayerWeaponHitAgainstMonster(
		character, equipment, slot, monster, monsterIdx, pcIdx, diagnostics, CombatEventAttack,
	)
}

func (g *GameState) resolvePlayerWeaponHitAgainstMonster(
	character *party_state.PlayerCharacter,
	equipment references.Equipment,
	slot references.EquipmentSlot,
	monster *CombatMonster,
	monsterIdx int,
	pcIdx int,
	diagnostics CombatDiagnostics,
	eventKind CombatEventKind,
) {
	// Calculate damage (replicates damage calculation from original combat damage rules)
	// See ref:COMBAT-0004.
	damageResult := CombatCalcDamage(equipment, monster.EnemyRef().Armour, g.AsRandomProvider())
	diagnostics.Damage = damageResult

	// Glass Sword shatters after use (original: unready weapon, set to NOTHING)
	if damageResult.WeaponShattered {
		character.SetEquippedItem(slot, references.NoEquipment)
	}

	// Apply undead/immortal damage modifiers (replicates damage application parts)
	// See ref:COMBAT-0004.
	finalDamage := monster.ApplyDamageModifiers(damageResult.Damage, equipment)
	diagnostics.FinalDamage = finalDamage
	diagnostics.TargetUndead = monster.MonsterType().IsUndead()
	diagnostics.TargetImmortal = monster.HasAbility(references.Immortal)
	diagnostics.MagicWeapon = IsMagicWeapon(equipment)
	if damageResult.WeaponShattered {
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind: CombatEventEquipmentShattered,
			Attacker: CombatFeedbackEntity{
				IsPartyMember: true,
				Index:         pcIdx,
				Position:      g.combatPartyAttackerPosition(pcIdx),
			},
			Target: CombatFeedbackEntity{
				IsPartyMember: false,
				Index:         monsterIdx,
				Position:      monster.Position(),
			},
			Diagnostics: diagnostics,
		})
	}

	// REDUX DEVIATION: Original damage application can invoke division after a successful
	// zero-damage hit. Redux treats full deflection
	// as an unsuccessful hit and skips post-hit effects.
	// See ref:COMBAT-0009.
	if damageResult.Deflected && !damageResult.InstantKill {
		g.publishPlayerCombatFeedback(eventKind, pcIdx, monsterIdx, CombatFeedbackGraze, 0, false, diagnostics)

		return
	}

	// Apply damage to the monster
	g.applyCombatDamageToMonsterWithEventKind(
		monster,
		monsterIdx,
		finalDamage,
		damageResult.InstantKill,
		character,
		pcIdx,
		diagnostics,
		eventKind,
	)
}

// performBareHandsAttack resolves a bare-hands punch (no weapon equipped).
// Original: weaponAttack = 1 for bare hands.
func (g *GameState) performBareHandsAttack(
	character *party_state.PlayerCharacter,
	monster *CombatMonster,
	monsterIdx int,
	pcIdx int,
	rng references.RandomProvider,
) {
	// Bare-hands always use DEX for hit roll
	attackerStat := g.combatPhysicalAttackAttribute(g.combatPartyAttackerIdentity(pcIdx), references.NoEquipment)
	defense := g.combatTargetDefense(CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIdx})

	hitResult := CombatHitCheck(attackerStat, defense.EffectiveDex, false, rng)
	diagnostics := CombatDiagnostics{
		Hit:           hitResult,
		Source:        references.NoEquipment,
		TargetBaseDex: defense.BaseDex,
	}
	if hitResult.Outcome == CombatHitMiss {
		g.showPlayerCombatFeedback(pcIdx, monsterIdx, CombatFeedbackMiss, 0, false, diagnostics)

		return
	}

	damageResult := CombatCalcDamage(references.NoEquipment, monster.EnemyRef().Armour, rng)
	diagnostics.Damage = damageResult

	finalDamage := monster.ApplyDamageModifiers(damageResult.Damage, references.NoEquipment)
	diagnostics.FinalDamage = finalDamage
	diagnostics.TargetUndead = monster.MonsterType().IsUndead()
	diagnostics.TargetImmortal = monster.HasAbility(references.Immortal)

	// REDUX DEVIATION: Original damage application can invoke division after a successful
	// zero-damage hit. Redux treats full deflection
	// as an unsuccessful hit and skips post-hit effects.
	// See ref:COMBAT-0009.
	if damageResult.Deflected {
		g.showPlayerCombatFeedback(pcIdx, monsterIdx, CombatFeedbackGraze, 0, false, diagnostics)

		return
	}

	g.applyCombatDamageToMonster(monster, monsterIdx, finalDamage, false, character, pcIdx, diagnostics)
}

// applyCombatDamageToMonster applies final damage to a monster and handles death.
// Replicates the kill/XP portion of damage application from original combat damage rules.
//
// See ref:COMBAT-0004.

func (g *GameState) applyCombatDamageToMonster(
	monster *CombatMonster,
	monsterIdx int,
	damage int,
	instantKill bool,
	attacker *party_state.PlayerCharacter,
	pcIdx int,
	diagnostics CombatDiagnostics,
) {
	delivery := CombatAttackDeliveryMelee
	if diagnostics.MagicDamage {
		delivery = CombatAttackDeliveryEnvironmental
	}
	g.applyCombatDamageToMonsterWithPresentation(
		monster, monsterIdx, damage, instantKill, attacker, pcIdx, diagnostics, CombatEventAttack, delivery,
	)
}

func (g *GameState) applyCombatDamageToMonsterWithEventKind(
	monster *CombatMonster,
	monsterIdx int,
	damage int,
	instantKill bool,
	attacker *party_state.PlayerCharacter,
	pcIdx int,
	diagnostics CombatDiagnostics,
	eventKind CombatEventKind,
) {
	g.applyCombatDamageToMonsterWithPresentation(
		monster, monsterIdx, damage, instantKill, attacker, pcIdx, diagnostics, eventKind,
		combatAttackDeliveryForEvent(eventKind),
	)
}

func (g *GameState) applyCombatDamageToMonsterWithPresentation(
	monster *CombatMonster,
	monsterIdx int,
	damage int,
	instantKill bool,
	attacker *party_state.PlayerCharacter,
	pcIdx int,
	diagnostics CombatDiagnostics,
	eventKind CombatEventKind,
	delivery CombatAttackDelivery,
) {
	hpBefore := monster.CurrentHP()
	if instantKill && damage == combatInstantKillDamage {
		// Instant kill: set HP to 0 directly (Glass Sword / Chaos Sword damage 99)
		monster.SetCurrentHP(0)
	} else {
		monster.TakeDamage(damage)
	}
	hpLost := hpBefore - monster.CurrentHP()
	g.publishPlayerCombatFeedbackWithDelivery(
		eventKind,
		pcIdx,
		monsterIdx,
		CombatFeedbackDamage,
		hpLost,
		monster.IsDead(),
		diagnostics,
		delivery,
	)

	if hpBefore > 0 && monster.IsDead() {
		g.finalizeCombatMonsterDeath(monster, monsterIdx, attacker, pcIdx)
	} else if !monster.IsDead() {
		logging.Debugf(logging.CategoryGameState,
			"Monster hit: type=%s damage=%d remainingHP=%d",
			monster.MonsterType().Name(), damage, monster.CurrentHP())
		g.resolveSurvivingCombatMonsterHit(monster, monsterIdx, !diagnostics.MagicDamage)
	}
}

func (g *GameState) finalizeCombatMonsterDeath(
	monster *CombatMonster,
	monsterIdx int,
	attacker *party_state.PlayerCharacter,
	pcIdx int,
) {
	g.finalizeCombatMonsterDeathWithPolicy(monster, monsterIdx, attacker, pcIdx, true, true)
}

func (g *GameState) finalizeCombatMonsterDeathWithPolicy(
	monster *CombatMonster,
	monsterIdx int,
	attacker *party_state.PlayerCharacter,
	pcIdx int,
	awardExperience bool,
	createDeathObject bool,
) {
	nominalExperience := 0
	if awardExperience {
		nominalExperience = combatCalcExperience(monster.MaxHP())
	}
	disappears := monster.HasAbility(references.DisappearsOnDeath)
	position := monster.Position()
	g.CombatScene.TurnQueue.MarkDead(monsterIdx, false)
	sourceSlotRetained := false
	if disappears {
		vanished := CombatVanishedMonster{
			CombatantID: monster.CombatantID(),
			Index:       monsterIdx,
			MonsterType: monster.MonsterType(),
			Position:    position,
		}
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind:     CombatEventVanished,
			Attacker: combatMonsterFeedbackEntity(monsterIdx, position),
			From:     position,
			To:       position,
			Vanished: vanished,
		})
	} else if createDeathObject {
		if g.resolveSpecialCombatMonsterDeath(monster, monsterIdx) {
			sourceSlotRetained = monster.MonsterType() == references.MonsterGazer
		} else {
			sourceSlotRetained = g.placeCombatDeathObject(monster, monsterIdx)
		}
	}
	if !sourceSlotRetained {
		g.CombatScene.TurnQueue.releaseSourceSlot(CombatTurnIdentity{
			IsPartyMember: false,
			EntityIndex:   monsterIdx,
		})
	}
	g.releaseCombatController(monster.CombatantID(), CombatPossessionReleaseControllerDefeated)

	experienceAwarded := 0
	if awardExperience {
		// Return the snapshotted kill award only after deletion and post-death
		// consequences have completed. ref:PROGRESS-0002
		experienceAwarded = g.awardKillExperience(attacker, pcIdx, nominalExperience)
	}

	logging.Debugf(logging.CategoryGameState,
		"Monster killed: type=%s monsterIdx=%d xp=%d awarded to PC %d treasureObjects=%d",
		monster.MonsterType().Name(), monsterIdx, experienceAwarded, pcIdx, g.CombatScene.ObjectCount())
}

func (g *GameState) showPlayerCombatFeedback(
	pcIdx, monsterIdx int,
	result CombatFeedbackResult,
	hpLost int,
	fatal bool,
	diagnostics CombatDiagnostics,
) {
	g.publishPlayerCombatFeedback(CombatEventAttack, pcIdx, monsterIdx, result, hpLost, fatal, diagnostics)
}

func (g *GameState) publishPlayerCombatFeedback(
	eventKind CombatEventKind,
	pcIdx, monsterIdx int,
	result CombatFeedbackResult,
	hpLost int,
	fatal bool,
	diagnostics CombatDiagnostics,
) {
	g.publishPlayerCombatFeedbackWithDelivery(
		eventKind, pcIdx, monsterIdx, result, hpLost, fatal, diagnostics,
		combatAttackDeliveryForEvent(eventKind),
	)
}

func (g *GameState) publishPlayerCombatFeedbackWithDelivery(
	eventKind CombatEventKind,
	pcIdx, monsterIdx int,
	result CombatFeedbackResult,
	hpLost int,
	fatal bool,
	diagnostics CombatDiagnostics,
	delivery CombatAttackDelivery,
) {
	monster := g.CombatScene.MonsterManager.GetMonster(monsterIdx)
	attackerIdentity := g.combatPartyAttackerIdentity(pcIdx)
	injury := g.captureCombatMonsterInjury(CombatTurnIdentity{EntityIndex: monsterIdx}, hpLost)
	feedback := CombatAttackFeedback{
		Attacker: CombatFeedbackEntity{
			IsPartyMember: true,
			Index:         pcIdx,
			Slot:          attackerIdentity.Slot,
			Position:      g.combatPartyAttackerPosition(pcIdx),
		},
		Target: CombatFeedbackEntity{
			IsPartyMember: false,
			Index:         monsterIdx,
			Position:      monster.Position(),
		},
		Result:       result,
		HPLost:       hpLost,
		Fatal:        fatal,
		Presentation: combatAttackPresentation(delivery, diagnostics, true),
	}
	g.SystemCallbacks.Visual.ShowCombatImpact(feedback)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind:          eventKind,
		Attacker:      feedback.Attacker,
		Target:        feedback.Target,
		Result:        result,
		HPLost:        hpLost,
		Fatal:         fatal,
		From:          feedback.Attacker.Position,
		To:            feedback.Target.Position,
		Diagnostics:   diagnostics,
		MonsterInjury: injury,
	})
}

func combatAttackDeliveryForEvent(eventKind CombatEventKind) CombatAttackDelivery {
	//nolint:exhaustive // Non-attack event kinds intentionally retain the unknown presentation delivery.
	switch eventKind {
	case CombatEventAttack, CombatEventPoison:
		return CombatAttackDeliveryMelee
	case CombatEventRangedAttack:
		return CombatAttackDeliveryProjectile
	case CombatEventSpell:
		return CombatAttackDeliverySpell
	default:
		return CombatAttackDeliveryUnknown
	}
}

func combatAttackPresentationForEvent(
	eventKind CombatEventKind,
	diagnostics CombatDiagnostics,
	attackerIsPartyMember bool,
) CombatAttackPresentation {
	return combatAttackPresentation(
		combatAttackDeliveryForEvent(eventKind), diagnostics, attackerIsPartyMember,
	)
}

func combatAttackPresentation(
	delivery CombatAttackDelivery,
	diagnostics CombatDiagnostics,
	attackerIsPartyMember bool,
) CombatAttackPresentation {
	source := references.NoEquipment
	if attackerIsPartyMember &&
		(delivery == CombatAttackDeliveryMelee || delivery == CombatAttackDeliveryProjectile) {
		source = diagnostics.Source
	}

	return CombatAttackPresentation{Delivery: delivery, Source: source}
}

func (g *GameState) combatPartyAttackerPosition(pcIdx int) references.Position {
	identity := g.combatPartyAttackerIdentity(pcIdx)
	if identity.IsPartyMember && identity.EntityIndex == pcIdx {
		return g.CombatScene.GetCombatantPosition(identity)
	}

	return g.CombatScene.GetPartyMemberPosition(pcIdx)
}

func (g *GameState) combatPartyAttackerIdentity(pcIdx int) CombatTurnIdentity {
	identity := g.CombatScene.GetCurrentTurnIdentity()
	if identity.IsPartyMember && identity.EntityIndex == pcIdx {
		return identity
	}

	return CombatTurnIdentity{IsPartyMember: true, EntityIndex: pcIdx}
}

// ActionAttackDungeonMap targets only the wrapped cell directly ahead.
//
//goland:noinspection GoUnusedParameter
func (g *GameState) ActionAttackDungeonMap(_ references.Direction) bool {
	dungeon := g.requireActiveDungeon()
	g.claimDungeonCommandAction(DungeonCommandOnly)
	target := dungeon.facing.Direction().GetNewPositionInDirection(&g.MapState.PlayerLocation.Position).
		GetWrapped(references.XDungeonMapTiles, references.YDungeonMapTiles)
	wanderer := dungeon.wanderer
	if wanderer == nil || wanderer.Floor != g.MapState.PlayerLocation.Floor || wanderer.Position != *target {
		g.SystemCallbacks.Message.AddRowStr("What?")

		return false
	}
	if err := g.startDirectDungeonWandererCombat(wanderer.Monster); err != nil {
		logging.Errorf(logging.CategoryGameState, "Unable to attack dungeon wanderer: %v", err)
		g.SystemCallbacks.UI.ShowActionFeedback("Combat could not begin.", true)

		return false
	}

	return true
}
