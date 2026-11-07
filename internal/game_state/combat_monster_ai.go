package game_state

// Monster AI owns target selection, integer distance, legal movement,
// melee attempts, damage/status effects and action orchestration. Ranged
// missiles, turning and special actions are composed in companion files.
// Movement and occupancy remain authoritative gameplay checks. ref:COMBAT-0008

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Monster AI magic numbers from original source code.
//

const (
	// monsterRandomDirectionAttempts permits four fallback direction attempts
	// after preferred movement is blocked. ref:COMBAT-0008
	monsterRandomDirectionAttempts = 4
	monsterSeekAxisRollRange       = 256
	monsterSeekHorizontalThreshold = 127
	monsterTeleportChanceRange     = 4
	monsterTeleportSkipRoll        = 3
	monsterTeleportCoordinateRange = 16
	monsterNoOpponentCenter        = 5

	// monsterAdjacentDistance permits melee only at distance one. ref:COMBAT-0008
	monsterAdjacentDistance = 1
)

// integerDistance returns floor(sqrt(dx*dx + dy*dy)). It subtracts
// successive odd numbers to compute the integer square root without floats. ref:COMBAT-0008
func integerDistance(from, to references.Position) int {
	dx := int(to.X) - int(from.X)
	dy := int(to.Y) - int(from.Y)
	distSqr := dx*dx + dy*dy

	// Integer square root via successive odd-number subtraction
	// (exact replication of original chkdistabs algorithm)
	odd := 1
	sqrt := 0

	for distSqr >= odd {
		distSqr -= odd
		odd += 2
		sqrt++
	}

	return sqrt
}

// nearestPCResult holds the result of finding the nearest PC to a monster.
type nearestPCResult struct {
	// pcIndex is the party member index (0-5), or -1 if no PC found.
	pcIndex int
	// dirX is the step direction toward the PC (-1, 0, or +1).
	dirX int
	// dirY is the step direction toward the PC (-1, 0, or +1).
	dirY int
	// distance is the integer Pythagorean distance to the nearest PC.
	distance int
	// identity supports the original opposite-loyalty search across PCs and monsters.
	identity     CombatTurnIdentity
	found        bool
	hasDirection bool
}

// findNearestOpponent ports whosnear's loyalty search while using Redux's split
// PC and monster stores. Reverse iteration preserves the original queue tie order.
func (g *GameState) findNearestOpponent(
	attacker CombatTurnIdentity,
	attackerLoyalty CombatMonsterLoyalty,
	attackerPosition references.Position,
) nearestPCResult {
	result := nearestPCResult{pcIndex: NoTargetPC}
	bestDistance := -1
	// Original combat movement rules lets everyone select invisible opponents in Doom and lets
	// Shadowlords do so on every combat map.
	// See ref:COMBAT-0008.
	attackerSeesInvisible := !g.CombatScene.invisibilityProtectsTargets()
	if !attacker.IsPartyMember {
		attackerMonster := g.CombatScene.MonsterManager.GetMonster(attacker.EntityIndex)
		attackerSeesInvisible = attackerSeesInvisible ||
			(attackerMonster != nil && attackerMonster.MonsterType() == references.MonsterShadowlord)
	}
	consider := func(identity CombatTurnIdentity, position references.Position) {
		distance := integerDistance(attackerPosition, position)
		if bestDistance >= 0 && distance >= bestDistance {
			return
		}
		bestDistance = distance
		result.identity = identity
		result.found = true
		result.hasDirection = true
		result.distance = distance
		result.dirX = signum(int(position.X) - int(attackerPosition.X))
		result.dirY = signum(int(position.Y) - int(attackerPosition.Y))
		if identity.IsPartyMember {
			result.pcIndex = identity.EntityIndex
		} else {
			result.pcIndex = NoTargetPC
		}
	}

	for sourceSlot := sourceCombatQueueSlots - 1; sourceSlot >= 0; sourceSlot-- {
		entry := g.CombatScene.TurnQueue.getActiveEntryAtSourceSlot(sourceSlot)
		if entry == nil {
			continue
		}
		identity := turnQueueIdentity(entry)
		if identity == attacker || g.combatIdentityLoyalty(identity) == attackerLoyalty {
			continue
		}
		if identity.IsPartyMember {
			if !g.CombatScene.isCombatPCActive(identity) || g.CombatScene.isCombatPCUnderground(identity) {
				continue
			}
			if g.CombatScene.isCombatPCInvisible(identity) && !attackerSeesInvisible {
				continue
			}
			consider(identity, g.CombatScene.GetCombatantPosition(identity))

			continue
		}
		monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
		if monster == nil || !monster.IsActive() || monster.IsDead() ||
			(monster.IsInvisible() && !attackerSeesInvisible) {
			continue
		}
		consider(identity, monster.Position())
	}

	return result
}

// monsterTargetLoyaltyWithRNG applies Quas An Wis each time a monster selects
// an opponent. Attack and seek therefore consume independent source-order rolls.
func (g *GameState) monsterTargetLoyaltyWithRNG(
	monster *CombatMonster,
	rng references.RandomProvider,
) CombatMonsterLoyalty {
	if monster.IsHostile() && g.HasActiveDurationEffect(DurationEffectConfusion) &&
		rollOriginalD30(rng) > monster.EnemyRef().Intelligence { // Closest-opponent selection calls transformed d30.
		// See ref:RNG-0001.
		return CombatMonsterLoyaltyParty
	}

	return monster.Loyalty()
}

func (g *GameState) selectMonsterOpponentWithRNG(
	monster *CombatMonster,
	monsterIdx int,
	rng references.RandomProvider,
) nearestPCResult {
	loyalty := g.monsterTargetLoyaltyWithRNG(monster, rng)
	nearest := g.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIdx},
		loyalty, monster.Position(),
	)
	if !nearest.found && g.recoverOnePossessedPC() {
		nearest = g.findNearestOpponent(
			CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIdx},
			loyalty, monster.Position(),
		)
	}
	if !nearest.found {
		nearest = g.noOpponentMovement(CombatTurnIdentity{
			IsPartyMember: false,
			EntityIndex:   monsterIdx,
		})
	}

	return nearest
}

func (g *GameState) noOpponentMovement(attacker CombatTurnIdentity) nearestPCResult {
	for _, candidate := range g.CombatScene.MonsterManager.monsters {
		if candidate == nil || !candidate.IsActive() || candidate.IsDead() {
			continue
		}
		candidate.SetCurrentHP(1)
		candidate.SetFleeing(true)
	}
	position := g.CombatScene.GetCombatantPosition(attacker)

	return nearestPCResult{
		pcIndex:      NoTargetPC,
		dirX:         signum(monsterNoOpponentCenter - int(position.X)),
		dirY:         signum(monsterNoOpponentCenter - int(position.Y)),
		hasDirection: true,
	}
}

// signum returns -1, 0, or 1 for the sign of v.
func signum(v int) int {
	switch {
	case v < 0:
		return -1
	case v > 0:
		return 1
	default:
		return 0
	}
}

// isCombatPositionValidForMonster checks whether a position is valid for monster movement.
// Replicates original monster movement validation from original combat action rules.
//
// Checks:
//  1. Position within 11x11 combat map bounds
//  2. Terrain passability for this monster's original base shape
//  3. Not occupied by a PC
//  4. Not occupied by another monster (excluding the moving monster)
//
// See ref:COMBAT-0008.
func isCombatPositionValidForMonster(
	g *GameState,
	pos references.Position,
	monster *CombatMonster,
	monsterIdx int,
) bool {
	// The original monster movement validation permits only fleeing monsters to cross an edge.
	// See ref:COMBAT-0008.
	if pos.X < 0 || pos.X >= references.XCombatMapTiles ||
		pos.Y < 0 || pos.Y >= references.YCombatMapTiles {
		return monster.IsFleeing()
	}

	// Tactical movement uses the original base-shape class, which differs from
	// the exploration flags consumed by EnemyReference.CanMoveToTile.
	theMap := g.GetCurrentLayeredMap()
	topTile := theMap.GetTopTile(&pos)

	if topTile == nil {
		return false
	}

	if !g.combatTerrainAllowsDestination(pos) ||
		!combatTerrainAllowsOriginalShape(originalCombatMonsterShape(monster.MonsterType()), topTile) {
		return false
	}

	// Check PC occupancy
	if g.CombatScene.isPartyMemberAtPosition(pos, -1) {
		return false
	}
	if g.CombatScene.hasMonsterBlockingObjectAt(pos) {
		return false
	}
	if g.CombatScene.HasProtectionFieldAt(pos) {
		return false
	}

	// Check monster occupancy (exclude the moving monster)
	for i, other := range g.CombatScene.MonsterManager.monsters {
		if i == monsterIdx || !other.IsActive() || other.IsDead() {
			continue
		}

		if other.Position() == pos {
			return false
		}
	}

	return true
}

// monsterMeleeAttack attempts a melee attack from a monster against the nearest PC.
// Replicates the melee portion of monster melee attack from original combat scheduler combined with
// hit status and damage from original combat damage rules.
//
// Only attempts attack if a PC is adjacent (distance == 1).
// Returns true if an attack was made (hit or miss), false if no valid target.
//
// See ref:COMBAT-0008.

func (g *GameState) monsterMeleeAttack(
	monster *CombatMonster,
	monsterIdx int,
	nearest nearestPCResult,
) bool {
	if !nearest.found || nearest.distance != monsterAdjacentDistance {
		return false
	}
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundCombatSwing)
	monster.SetCorpserExposed(true)
	g.rememberMeleeEngager(
		nearest.identity,
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIdx},
	)
	if !nearest.identity.IsPartyMember {
		target := g.CombatScene.MonsterManager.GetMonster(nearest.identity.EntityIndex)
		g.resolveMonsterMeleeAgainstMonster(monster, monsterIdx, target, nearest.identity.EntityIndex)

		return true
	}

	targetPCIdx := nearest.pcIndex
	defer g.finishDirectedCombatAttack(nearest.identity, g.CombatScene.isCombatPCSleeping(nearest.identity))
	character := g.PartyState.GetCharacter(targetPCIdx)
	rng := g.AsRandomProvider()

	// Use Strength for bludgeoning monsters and Dexterity otherwise. ref:COMBAT-0004
	attackerStat := g.combatPhysicalAttackAttribute(CombatTurnIdentity{EntityIndex: monsterIdx}, references.NoEquipment)

	defense := g.combatTargetDefense(nearest.identity)

	// Hit check (replicates hit check from original combat damage rules)
	// See ref:COMBAT-0008.
	hitResult := CombatHitCheck(attackerStat, defense.EffectiveDex, false, rng)
	diagnostics := CombatDiagnostics{
		Hit:           hitResult,
		TargetBaseDex: defense.BaseDex,
		UsedStrength:  monster.HasAbility(references.Bludgeons),
	}
	if hitResult.Outcome == CombatHitMiss {
		g.showMonsterCombatFeedback(
			monsterIdx,
			nearest.identity,
			CombatFeedbackMiss,
			0,
			false,
			diagnostics,
		)

		return true
	}
	resolution := g.resolveSuccessfulMonsterHit(monster, nearest.identity, true, rng, &diagnostics)
	if resolution.replacedDamage {
		if resolution.damageSelected {
			if resolution.damage == 0 {
				g.showMonsterCombatFeedback(
					monsterIdx, nearest.identity, resolution.feedback, 0, false, diagnostics,
				)
			} else {
				g.applyCombatDamageToPC(
					character, nearest.identity, resolution.damage, monster, monsterIdx, diagnostics,
				)
			}
		} else {
			g.showMonsterCombatFeedback(
				monsterIdx, nearest.identity, resolution.feedback, 0, false, diagnostics,
			)
		}

		return true
	}

	// Calculate damage (replicates getdamage() for monsters)
	// Monster weapon attack = EnemyRef.Damage; defender armor = PC total defense
	damageResult := CombatCalcDamageRaw(
		monster.EnemyRef().Damage,
		g.combatPartyDefense(character),
		rng,
	)
	diagnostics.Damage = damageResult
	diagnostics.FinalDamage = damageResult.Damage

	if damageResult.Deflected {
		g.showMonsterCombatFeedback(
			monsterIdx,
			nearest.identity,
			CombatFeedbackGraze,
			0,
			false,
			diagnostics,
		)

		return true
	}

	// Apply damage to PC
	g.applyCombatDamageToPC(
		character,
		nearest.identity,
		damageResult.Damage,
		monster,
		monsterIdx,
		diagnostics,
	)

	return true
}

// applyCombatDamageToPC applies damage to a player character and handles death.
// Replicates the PC-death portion of damage application from original combat damage rules.
// See ref:COMBAT-0008.
func (g *GameState) applyCombatDamageToPC(
	character *party_state.PlayerCharacter,
	target CombatTurnIdentity,
	damage int,
	monster *CombatMonster,
	monsterIdx int,
	diagnostics CombatDiagnostics,
) {
	pcIdx := target.EntityIndex
	monsterName := monster.MonsterType().Name()
	pcName := character.GetNameAsString()

	hpBefore := character.GetCurrentHp()
	character.SubtractHp(uint16(combatMonsterDamageToApply(
		int(hpBefore), damage, diagnostics.Damage.InstantKill,
	)))
	hpLost := int(hpBefore - character.GetCurrentHp())
	if hpLost > 0 {
		g.showMonsterCombatFeedback(
			monsterIdx,
			target,
			CombatFeedbackDamage,
			hpLost,
			character.IsDead(),
			diagnostics,
		)
	}

	if character.IsDead() {
		g.markCombatPCDead(pcIdx)

		logging.Infof(logging.CategoryGameState,
			"PC %d (%s) killed by monster %s (idx=%d)",
			pcIdx, pcName, monsterName, monsterIdx)
	} else {
		g.tryDragCombatPCUnder(monster, target, true, hpLost, false)
		logging.Debugf(logging.CategoryGameState,
			"Monster %s hit PC %d (%s) for %d damage, remainingHP=%d",
			monsterName, pcIdx, pcName, damage, character.GetCurrentHp())
	}
}

func combatMonsterDamageToApply(currentHP, damage int, instantKill bool) int {
	if instantKill && damage == combatInstantKillDamage {
		return currentHP
	}

	return damage
}

func (g *GameState) tryDragCombatPCUnder(
	attacker *CombatMonster,
	target CombatTurnIdentity,
	adjacent bool,
	hpLost int,
	fatal bool,
) {
	if attacker == nil || attacker.MonsterType() != references.MonsterCorpser ||
		!target.IsPartyMember || !adjacent || hpLost <= 0 || fatal ||
		g.CombatScene.isCombatPCUnderground(target) {
		return
	}
	g.CombatScene.setCombatPCUnderground(target, true)
	character := g.PartyState.GetCharacter(target.EntityIndex)
	g.SystemCallbacks.Message.AddRowStr(character.GetNameAsString() + " dragged under!")
	g.publishCombatPCConditionEvent(target, CombatConditionDraggedUnder)
}

func (g *GameState) showMonsterCombatFeedback(
	monsterIdx int,
	targetIdentity CombatTurnIdentity,
	result CombatFeedbackResult,
	hpLost int,
	fatal bool,
	diagnostics CombatDiagnostics,
) {
	pcIdx := targetIdentity.EntityIndex
	monster := g.CombatScene.MonsterManager.GetMonster(monsterIdx)
	feedback := CombatAttackFeedback{
		Attacker: CombatFeedbackEntity{
			IsPartyMember: false,
			Index:         monsterIdx,
			Position:      monster.Position(),
		},
		Target: CombatFeedbackEntity{
			IsPartyMember: true,
			Index:         pcIdx,
			Slot:          targetIdentity.Slot,
			Position:      g.CombatScene.GetCombatantPosition(targetIdentity),
		},
		Result: result,
		HPLost: hpLost,
		Fatal:  fatal,
		Presentation: CombatAttackPresentation{
			Delivery: CombatAttackDeliveryMelee,
			Source:   references.NoEquipment,
		},
	}
	g.SystemCallbacks.Visual.ShowCombatImpact(feedback)
	eventKind := CombatEventAttack
	if result == CombatFeedbackPoison {
		eventKind = CombatEventPoison
	}
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind:        eventKind,
		Attacker:    feedback.Attacker,
		Target:      feedback.Target,
		Result:      result,
		HPLost:      hpLost,
		Fatal:       fatal,
		From:        feedback.Attacker.Position,
		To:          feedback.Target.Position,
		Diagnostics: diagnostics,
	})
}

type monsterMovementResult struct {
	moved      bool
	teleported bool
	nearest    nearestPCResult
}

// monsterSeek moves or teleports a monster according to the original monster pursuit order.
// Replicates original monster pursuit from original combat movement rules.
//
// Algorithm:
//  1. Skip if the monster has DoNotMove.
//  2. Teleport-capable monsters make the boxed/chance and one-candidate attempt.
//  3. A boxed monster whose teleport fails remains stationary.
//  4. Rolls above 127 try horizontal movement; otherwise horizontal is skipped.
//  5. If horizontal was skipped or blocked, try vertical movement.
//  6. If still blocked, try four source-ordered random directions.
//
// See ref:COMBAT-0008.
func (g *GameState) monsterSeek(
	monster *CombatMonster,
	monsterIdx int,
) monsterMovementResult {
	return g.monsterSeekWithRNG(monster, monsterIdx, g.AsRandomProvider())
}

func (g *GameState) monsterSeekWithRNG(
	monster *CombatMonster,
	monsterIdx int,
	rng references.RandomProvider,
) monsterMovementResult {
	// Skip if monster can't move (Reaper, Mimic equivalents)
	if monster.EnemyRef().DoNotMove {
		return monsterMovementResult{}
	}

	boxedIn := g.monsterBoxedIn(monster, monsterIdx)
	if g.tryMonsterTeleportWithRNG(monster, monsterIdx, boxedIn, rng) {
		return monsterMovementResult{moved: true, teleported: true}
	}
	if boxedIn {
		return monsterMovementResult{}
	}

	nearest := g.selectMonsterOpponentWithRNG(monster, monsterIdx, rng)
	if !nearest.hasDirection {
		return monsterMovementResult{nearest: nearest}
	}
	if monster.IsFleeing() {
		nearest.dirX *= -1
		nearest.dirY *= -1
	}

	result := g.monsterSeekTowardWithRNG(monster, monsterIdx, nearest, rng)
	result.nearest = nearest

	return result
}

func (g *GameState) monsterSeekTowardWithRNG(
	monster *CombatMonster,
	monsterIdx int,
	nearest nearestPCResult,
	rng references.RandomProvider,
) monsterMovementResult {
	monPos := monster.Position()

	var moved bool
	// Original combat movement rules tries horizontal only on rolls above 127, then tries vertical
	// whenever horizontal was skipped or blocked.
	// See ref:COMBAT-0008.
	if rng.Intn(monsterSeekAxisRollRange) > monsterSeekHorizontalThreshold {
		moved = g.tryMonsterMoveAxis(monster, monsterIdx, monPos, nearest.dirX, 0)
	}
	if !moved {
		moved = g.tryMonsterMoveAxis(monster, monsterIdx, monPos, 0, nearest.dirY)
	}

	// Fallback: try random directions
	if !moved {
		moved = g.tryMonsterRandomDirections(monster, monsterIdx, monPos, rng)
	}

	return monsterMovementResult{moved: moved}
}

func (g *GameState) monsterBoxedIn(monster *CombatMonster, monsterIdx int) bool {
	position := monster.Position()
	for _, direction := range [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
		candidate := references.Position{
			X: position.X + references.Coordinate(direction[0]),
			Y: position.Y + references.Coordinate(direction[1]),
		}
		if isCombatPositionValidForMonster(g, candidate, monster, monsterIdx) {
			return false
		}
	}

	return true
}

func (g *GameState) tryMonsterTeleportWithRNG(
	monster *CombatMonster,
	monsterIdx int,
	boxedIn bool,
	rng references.RandomProvider,
) bool {
	if !monster.IsHostile() || !monster.HasAbility(references.Teleport) ||
		g.hostileMonsterMagicIsNegated() {
		return false
	}
	if !boxedIn && rng.Intn(monsterTeleportChanceRange) == monsterTeleportSkipRoll {
		return false
	}

	from := monster.Position()
	to := references.Position{
		X: references.Coordinate(rng.Intn(monsterTeleportCoordinateRange)),
		Y: references.Coordinate(rng.Intn(monsterTeleportCoordinateRange)),
	}
	if to.X >= references.XCombatMapTiles || to.Y >= references.YCombatMapTiles ||
		to == from || !isCombatPositionValidForMonster(g, to, monster, monsterIdx) {
		return false
	}

	monster.SetPosition(to)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind:     CombatEventTeleport,
		Attacker: combatMonsterFeedbackEntity(monsterIdx, to),
		From:     from,
		To:       to,
	})

	return true
}

// tryMonsterMoveAxis attempts to move a monster by the given delta.
// Returns true if the move succeeded.
func (g *GameState) tryMonsterMoveAxis(
	monster *CombatMonster,
	monsterIdx int,
	from references.Position,
	dx, dy int,
) bool {
	if dx == 0 && dy == 0 {
		return false
	}

	newPos := references.Position{
		X: from.X + references.Coordinate(dx),
		Y: from.Y + references.Coordinate(dy),
	}

	if !isCombatPositionValidForMonster(g, newPos, monster, monsterIdx) {
		return false
	}

	monster.SetPosition(newPos)

	logging.Debugf(logging.CategoryGameState,
		"Monster %s (idx=%d) moved from (%d,%d) to (%d,%d)",
		monster.MonsterType().Name(), monsterIdx,
		from.X, from.Y, newPos.X, newPos.Y)

	return true
}

// tryMonsterRandomDirections tries up to 4 random directions when preferred movement is blocked.
// Replicates original monster pursuit original combat movement rules fallback.
// See ref:COMBAT-0008.
func (g *GameState) tryMonsterRandomDirections(
	monster *CombatMonster,
	monsterIdx int,
	from references.Position,
	rng references.RandomProvider,
) bool {
	// Original random mapping: south, east, north, west.
	directions := [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}

	for range monsterRandomDirectionAttempts {
		dir := directions[rng.Intn(len(directions))]

		newPos := references.Position{
			X: from.X + references.Coordinate(dir[0]),
			Y: from.Y + references.Coordinate(dir[1]),
		}

		if isCombatPositionValidForMonster(g, newPos, monster, monsterIdx) {
			monster.SetPosition(newPos)

			logging.Debugf(logging.CategoryGameState,
				"Monster %s (idx=%d) random-moved from (%d,%d) to (%d,%d)",
				monster.MonsterType().Name(), monsterIdx,
				from.X, from.Y, newPos.X, newPos.Y)

			return true
		}
	}

	return false
}

// MonsterTurnAction describes the planned action for a monster's turn.
type MonsterTurnAction int

const (
	// MonsterTurnIdle means the monster will not act (dead, asleep, no target).
	MonsterTurnIdle MonsterTurnAction = iota
	// MonsterTurnSeek means the monster will move toward the nearest PC.
	MonsterTurnSeek
	// MonsterTurnAttack means the monster will attempt a melee attack on a PC.
	MonsterTurnAttack
)

// MonsterTurnPlan holds the pre-computed plan for a monster's turn.
// Determined by planMonsterTurn() without executing any action, so the UI layer
// can show highlights (yellow on active monster, red on attack target) during the
// delay before the action resolves.
type MonsterTurnPlan struct {
	// Action is the planned action type.
	Action MonsterTurnAction
	// TargetPCIndex is the index of the PC that will be attacked, or NoTargetPC (-1)
	// if the monster is seeking or idle.
	TargetPCIndex int
	Target        CombatTurnIdentity
	HasTarget     bool
}

// MonsterTurnExecutionResult reports the visual action selected after all
// authoritative turn RNG has run.
type MonsterTurnExecutionResult struct {
	ProjectileLaunch *CombatProjectileLaunch
	Moved            bool
	Teleported       bool
}

// planMonsterTurn pre-computes what a monster will do on its turn without executing it.
// This allows the UI layer to highlight the attack target BEFORE the action resolves.
//
// The preview consumes no gameplay RNG. Specials and confusion can supersede
// its target when the authoritative action executes after the UI delay.
func (g *GameState) planMonsterTurn(entry *TurnQueueEntry) MonsterTurnPlan {
	monster := g.CombatScene.MonsterManager.GetMonster(entry.EntityIndex)
	if monster == nil || !monster.IsActive() || monster.IsDead() {
		return MonsterTurnPlan{Action: MonsterTurnIdle, TargetPCIndex: NoTargetPC}
	}

	// Sleeping monsters: cannot predict wake (RNG), assume idle
	if monster.IsAsleep() {
		return MonsterTurnPlan{Action: MonsterTurnIdle, TargetPCIndex: NoTargetPC}
	}
	if monster.IsFleeing() {
		return MonsterTurnPlan{Action: MonsterTurnSeek, TargetPCIndex: NoTargetPC}
	}

	// Planning must not consume gameplay RNG before original specialmoves runs.
	targetLoyalty := monster.Loyalty()
	nearest := g.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: entry.EntityIndex},
		targetLoyalty, monster.Position(),
	)
	if !nearest.found {
		return MonsterTurnPlan{Action: MonsterTurnIdle, TargetPCIndex: NoTargetPC}
	}

	hasUnresolvedTarget := monster.IsHostile() &&
		(g.HasActiveDurationEffect(DurationEffectConfusion) ||
			(!g.hostileMonsterMagicIsNegated() &&
				(monster.HasAbility(references.PossessCharm) ||
					monster.HasAbility(references.Invisibility) ||
					monster.HasAbility(references.GatesInDaemon))))
	// Only deterministic adjacent attacks receive a target preview. Original
	// specials and confusion resolve after the presentation delay.
	if nearest.distance == monsterAdjacentDistance && !hasUnresolvedTarget {
		return MonsterTurnPlan{
			Action: MonsterTurnAttack, TargetPCIndex: nearest.pcIndex,
			Target: nearest.identity, HasTarget: true,
		}
	}
	// A nonadjacent monster decides whether to fire only after specialmoves has
	// consumed its RNG, so planning cannot advertise a guaranteed attack.
	return MonsterTurnPlan{
		Action: MonsterTurnSeek, TargetPCIndex: NoTargetPC,
	}
}

// executeMonsterTurn runs a single monster's AI turn.
// Replicates original monster action from original combat scheduler.
//
// Turn flow:
//  1. Skip if the monster cannot act.
//  2. Resolve fleeing behavior, or hostile possession/invisibility/gating.
//  3. Resolve confusion allegiance and find the nearest opponent.
//  4. Attempt melee or ranged attack.
//  5. If no attack occurred, teleport or seek toward the opponent.
//
// Sleep is resolved at the scheduled DEX boundary before this method is called.
// See ref:COMBAT-0008.
func (g *GameState) executeMonsterTurn(entry *TurnQueueEntry, _ ...MonsterTurnPlan) MonsterTurnExecutionResult {
	monster := g.CombatScene.MonsterManager.GetMonster(entry.EntityIndex)
	if monster == nil || !monster.CanAct() {
		return MonsterTurnExecutionResult{}
	}

	monsterIdx := entry.EntityIndex
	defer func() {
		if g.CombatScene != nil && g.CombatScene.pendingMonsterMissile == nil {
			g.resolveCombatMonsterFieldEffects(monster, monsterIdx)
		}
	}()
	if monster.IsFleeing() {
		from := monster.Position()
		handled, teleported := g.executeFleeingMonsterTurn(monster, monsterIdx)
		if handled {
			return MonsterTurnExecutionResult{Moved: monster.Position() != from, Teleported: teleported}
		}
	}

	if monster.IsHostile() && g.executeHostileMonsterSpecialMove(monster, monsterIdx) {
		return MonsterTurnExecutionResult{}
	}

	// Monster melee attack and monster pursuit each perform their own nearest-opponent selection/confusion selection.
	nearest := g.selectMonsterOpponentWithRNG(monster, monsterIdx, g.AsRandomProvider())
	launch, attacked := g.prepareMonsterAttack(monster, monsterIdx, nearest)
	if attacked {
		// End-condition check is handled by the caller (AdvanceCombatTurn loop)
		// after each monster turn. Doing it here would cause double-transition
		// and nil CombatScene on defeat.
		if launch != nil {
			g.CombatScene.pendingMonsterMissile.resolveAttackerField = true
		}

		return MonsterTurnExecutionResult{ProjectileLaunch: launch}
	}

	// Not adjacent — seek toward nearest PC
	fromPosition := monster.Position()
	movement := g.monsterSeek(monster, monsterIdx)
	if movement.moved {
		if g.completeMonsterEdgeEscape(monster, monsterIdx, fromPosition) {
			return MonsterTurnExecutionResult{Moved: true}
		}
		toPosition := monster.Position()
		if !movement.teleported {
			g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
				Kind: CombatEventMove,
				Attacker: CombatFeedbackEntity{
					IsPartyMember: false,
					Index:         monsterIdx,
					Position:      toPosition,
				},
				From: fromPosition,
				To:   toPosition,
			})
		}

		return MonsterTurnExecutionResult{Moved: true, Teleported: movement.teleported}
	}

	return MonsterTurnExecutionResult{}
}

// completeMonsterEdgeEscape removes a monster whose move crossed the map edge.
// Only fleeing monsters may cross, and the no-opponent check can make a monster
// flee in the middle of its own turn, so every seek result must be checked.
func (g *GameState) completeMonsterEdgeEscape(monster *CombatMonster, monsterIdx int, from references.Position) bool {
	to := monster.Position()
	if to.X >= 0 && to.X < references.XCombatMapTiles &&
		to.Y >= 0 && to.Y < references.YCombatMapTiles {
		return false
	}
	monster.SetActive(false)
	g.CombatScene.TurnQueue.MarkFled(monsterIdx, false)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventFled,
		Attacker: CombatFeedbackEntity{
			Index:    monsterIdx,
			Position: to,
		},
		From: from,
		To:   to,
	})
	g.releaseCombatController(monster.CombatantID(), CombatPossessionReleaseControllerEscaped)

	return true
}

func (g *GameState) executeFleeingMonsterTurn(monster *CombatMonster, monsterIdx int) (bool, bool) {
	return g.executeFleeingMonsterTurnWithRNG(monster, monsterIdx, g.AsRandomProvider())
}

func (g *GameState) executeFleeingMonsterTurnWithRNG(
	monster *CombatMonster,
	monsterIdx int,
	rng references.RandomProvider,
) (bool, bool) {
	recoverFleeingMonsterHP(monster, rng)
	applyCombatMonsterCondition(monster, rng)
	from := monster.Position()
	movement := g.monsterSeekWithRNG(monster, monsterIdx, rng)
	if !movement.moved {
		if monster.IsFleeing() {
			nearest := g.selectMonsterOpponentWithRNG(monster, monsterIdx, rng)
			g.monsterAttackWithRNG(monster, monsterIdx, nearest, rng)
		}

		return true, false
	}

	to := monster.Position()
	if movement.teleported {
		return true, true
	}
	if g.completeMonsterEdgeEscape(monster, monsterIdx, from) {
		return true, false
	}
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventMove,
		Attacker: CombatFeedbackEntity{
			Index:    monsterIdx,
			Position: to,
		},
		From: from,
		To:   to,
	})

	return true, false
}

func recoverFleeingMonsterHP(monster *CombatMonster, rng references.RandomProvider) {
	// Original combat scheduler uses random(0,3)==3: exactly the upper boundary heals.
	// See ref:COMBAT-0008.
	if rng.Intn(4) == 3 {
		monster.Heal(1)
	}
}
