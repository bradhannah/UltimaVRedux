package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// NoCombatTarget indicates that a PC has no remembered monster target.
const NoCombatTarget = -1

// CombatAimDisposition describes only combatants currently visible to the player.
type CombatAimDisposition int

const (
	// CombatAimEmpty means no visible combatant occupies the aimed square.
	CombatAimEmpty CombatAimDisposition = iota
	// CombatAimFriendly means an active party member occupies the aimed square.
	CombatAimFriendly
	// CombatAimEnemy means an active, visible monster occupies the aimed square.
	CombatAimEnemy
)

// CombatAttackSource identifies one equipped item used by pc_attack().
type CombatAttackSource struct {
	Slot      references.EquipmentSlot
	Equipment references.Equipment
	MaxRange  int
	Missile   references.MissileType
}

// CombatAttackTargeting tracks the current per-item plraim() sequence.
type CombatAttackTargeting struct {
	AttackerIndex int
	Attacker      CombatTurnIdentity
	Sources       []CombatAttackSource
	SourceIndex   int
	Origin        references.Position
	Cursor        references.Position
	autoTarget    *CombatMonster
}

// CombatAttackTargetOutcome describes an aiming confirmation, cancellation, or impact resolution.
type CombatAttackTargetOutcome int

const (
	// CombatAttackTargetIgnored means the input did not change targeting state.
	CombatAttackTargetIgnored CombatAttackTargetOutcome = iota
	// CombatAttackSourceResolved means one attack source was resolved.
	CombatAttackSourceResolved
	// CombatAttackSourcePending means one projectile was committed and awaits impact.
	CombatAttackSourcePending
	// CombatAttackSourceCancelled means one attack source was skipped.
	CombatAttackSourceCancelled
	// CombatAttackSequenceCancelled means targeting ended before any source was resolved.
	CombatAttackSequenceCancelled
	// CombatAttackSequenceComplete means all attack sources have finished.
	CombatAttackSequenceComplete
)

// CombatAttackTargetResult carries authoritative launch facts separately from sequence status.
type CombatAttackTargetResult struct {
	Outcome      CombatAttackTargetOutcome
	Launch       *CombatProjectileLaunch
	ReturnLaunch *CombatProjectileLaunch
	ShowImpact   bool
}

// CurrentSource returns the attack item currently being aimed.
func (targeting *CombatAttackTargeting) CurrentSource() CombatAttackSource {
	if targeting == nil || targeting.SourceIndex < 0 || targeting.SourceIndex >= len(targeting.Sources) {
		panic("CombatAttackTargeting.CurrentSource called without an active source")
	}

	return targeting.Sources[targeting.SourceIndex]
}

// BeginCombatAttack starts the active PC's per-item targeting sequence.
// Original source: player attack and attack attempt, original attack rules.
// See ref:COMBAT-0004.
func (g *GameState) BeginCombatAttack() bool {
	if g.CombatScene == nil || g.CombatScene.hasPlayerActionOwner() {
		return false
	}

	if monster, monsterIndex := g.CombatScene.GetControlledMonster(); monster != nil {
		maxRange := max(monster.EnemyRef().AttackRange, monsterAdjacentDistance)
		origin := monster.Position()
		g.CombatScene.AttackTargeting = &CombatAttackTargeting{
			AttackerIndex: monsterIndex,
			Attacker: CombatTurnIdentity{
				IsPartyMember: false,
				EntityIndex:   monsterIndex,
			},
			Sources: []CombatAttackSource{{
				Slot:      references.SlotNone,
				Equipment: references.NoEquipment,
				MaxRange:  maxRange,
				Missile:   monster.EnemyRef().LargeMapMissile,
			}},
			SourceIndex: 0,
			Origin:      origin,
			Cursor:      origin,
			autoTarget:  nil,
		}

		return true
	}

	attacker := g.CombatScene.GetCurrentTurnIdentity()
	attackerIndex := attacker.EntityIndex
	if !attacker.IsPartyMember || attackerIndex < 0 || attackerIndex >= party_state.MaxActiveParty ||
		!g.CombatScene.isCombatPCActive(attacker) || g.CombatScene.isCombatPCUnderground(attacker) {
		return false
	}

	character := g.PartyState.GetCharacter(attackerIndex)
	sources := collectCombatAttackSources(character)
	origin := g.CombatScene.GetCombatantPosition(attacker)

	g.CombatScene.AttackTargeting = &CombatAttackTargeting{
		AttackerIndex: attackerIndex,
		Attacker:      attacker,
		Sources:       sources,
		SourceIndex:   0,
		Origin:        origin,
		Cursor:        origin,
		autoTarget:    nil,
	}
	g.initializeCombatAttackCursor()

	return true
}

// collectCombatAttackSources follows pc_attack() order: helm, weapon[0],
// weapon[1]. Redux maps weapon[1] to the off-hand shield slot.
func collectCombatAttackSources(character *party_state.PlayerCharacter) []CombatAttackSource {
	slots := [...]references.EquipmentSlot{
		references.SlotHelm,
		references.SlotWeapon,
		references.SlotShield,
	}

	sources := make([]CombatAttackSource, 0, len(slots))
	for _, slot := range slots {
		equipment := character.GetEquippedItem(slot)
		if equipment == references.NoEquipment || equipment.GetAttackValue() == 0 {
			continue
		}

		maxRange := equipment.GetAttackRange()
		if maxRange == 0 {
			maxRange = 1
		}
		sources = append(sources, CombatAttackSource{
			Slot:      slot,
			Equipment: equipment,
			MaxRange:  maxRange,
			Missile:   equipment.GetMissileType(),
		})
	}

	if len(sources) == 0 {
		sources = append(sources, CombatAttackSource{
			Slot:      references.SlotNone,
			Equipment: references.NoEquipment,
			MaxRange:  1,
			Missile:   references.MissileNone,
		})
	}

	return sources
}

// MoveCombatAttackCursor moves the crosshair by one of the eight direction
// vectors while enforcing plraim() map and range limits.
func (g *GameState) MoveCombatAttackCursor(deltaX, deltaY int) bool {
	if g.CombatScene == nil || g.CombatScene.AttackTargeting == nil || !isTargetingStep(deltaX, deltaY) {
		return false
	}

	targeting := g.CombatScene.AttackTargeting
	candidate := references.Position{
		X: targeting.Cursor.X + references.Coordinate(deltaX),
		Y: targeting.Cursor.Y + references.Coordinate(deltaY),
	}
	if !isCombatAttackCursorPositionValid(targeting, candidate) {
		return false
	}

	targeting.Cursor = candidate

	return true
}

func isTargetingStep(deltaX, deltaY int) bool {
	return deltaX >= -1 && deltaX <= 1 && deltaY >= -1 && deltaY <= 1 && (deltaX != 0 || deltaY != 0)
}

func isCombatAttackCursorPositionValid(targeting *CombatAttackTargeting, candidate references.Position) bool {
	return isCombatAttackPositionInRange(targeting.Origin, targeting.CurrentSource(), candidate)
}

func isCombatAttackPositionInRange(origin references.Position, source CombatAttackSource, candidate references.Position) bool {
	if candidate.X < 0 || candidate.X >= references.XCombatMapTiles ||
		candidate.Y < 0 || candidate.Y >= references.YCombatMapTiles {
		return false
	}

	return integerDistance(origin, candidate) <= source.MaxRange
}

// ConfirmCombatAttackTarget resolves melee immediately or commits one ranged launch.
func (g *GameState) ConfirmCombatAttackTarget() CombatAttackTargetResult {
	if g.CombatScene == nil || g.CombatScene.AttackTargeting == nil {
		return combatAttackResult(CombatAttackTargetIgnored)
	}

	targeting := g.CombatScene.AttackTargeting
	if targeting.Cursor == targeting.Origin {
		return combatAttackResult(CombatAttackTargetIgnored)
	}

	source := targeting.CurrentSource()
	targetIdentity, monster, monsterIndex, targetFound := g.combatAttackOccupantAt(targeting.Cursor)
	isRanged := targeting.Attacker.IsPartyMember && source.Equipment.GetAttackRange() > 0
	if !targeting.Attacker.IsPartyMember {
		isRanged = source.Missile != references.MissileNone &&
			integerDistance(targeting.Origin, targeting.Cursor) > monsterAdjacentDistance
	}
	if targetFound && g.combatIdentityLoyalty(targetIdentity) == g.combatIdentityLoyalty(targeting.Attacker) {
		g.SystemCallbacks.UI.ShowActionFeedback("Can't!", true)

		return g.completeCombatAttackSource(CombatAttackSourceResolved)
	}
	if targetFound && monster != nil && targeting.Attacker.IsPartyMember {
		g.CombatScene.setLastCombatAttackTarget(targeting.Attacker, monsterIndex)
	} else if isRanged && targeting.Attacker.IsPartyMember {
		// Original pcmissile forgets the aim when a shot finds no target; melee keeps it.
		g.CombatScene.setLastCombatAttackTarget(targeting.Attacker, NoCombatTarget)
	}
	if isRanged {
		return g.preparePendingCombatAttack(targeting, source, targetIdentity, targetFound)
	}
	switch {
	case !targetFound:
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing!", true)
	case targetIdentity.IsPartyMember && !targeting.Attacker.IsPartyMember:
		g.resolveControlledMonsterAttackAgainstPC(
			targeting.AttackerIndex, targetIdentity, targeting.Cursor,
		)
	case targetIdentity.IsPartyMember:
		g.resolveCombatPCAttackSourceAgainstPC(
			targeting.Attacker, source, targetIdentity,
		)
	case !targeting.Attacker.IsPartyMember:
		g.resolveControlledMonsterAttack(targeting.AttackerIndex, monster, monsterIndex, targeting.Cursor)
	default:
		g.resolveCombatAttackSourceAtPosition(targeting.AttackerIndex, source, targeting.Cursor)
	}

	return g.completeCombatAttackSource(CombatAttackSourceResolved)
}

// CancelCombatAttackSource skips only the current attack item, matching
// plraim() returning zero while pc_attack() continues with later items.
func (g *GameState) CancelCombatAttackSource() CombatAttackTargetResult {
	if g.CombatScene == nil || g.CombatScene.AttackTargeting == nil {
		return combatAttackResult(CombatAttackTargetIgnored)
	}

	return g.completeCombatAttackSource(CombatAttackSourceCancelled)
}

// CancelInitialCombatAttack ends targeting without consuming the active PC's
// turn. Once any attack source has been resolved or skipped, cancellation must
// continue through CancelCombatAttackSource instead.
func (g *GameState) CancelInitialCombatAttack() CombatAttackTargetResult {
	if g.CombatScene == nil || g.CombatScene.AttackTargeting == nil ||
		g.CombatScene.AttackTargeting.SourceIndex != 0 {
		return combatAttackResult(CombatAttackTargetIgnored)
	}

	g.CombatScene.AttackTargeting = nil

	return combatAttackResult(CombatAttackSequenceCancelled)
}

func combatAttackResult(outcome CombatAttackTargetOutcome) CombatAttackTargetResult {
	return CombatAttackTargetResult{Outcome: outcome}
}

func (g *GameState) completeCombatAttackSource(outcome CombatAttackTargetOutcome) CombatAttackTargetResult {
	if g.advanceCombatAttackSource() {
		outcome = CombatAttackSequenceComplete
	}

	return combatAttackResult(outcome)
}

func (g *GameState) advanceCombatAttackSource() bool {
	targeting := g.CombatScene.AttackTargeting
	targeting.SourceIndex++
	if targeting.SourceIndex >= len(targeting.Sources) {
		g.CombatScene.AttackTargeting = nil

		return true
	}

	g.initializeCombatAttackCursor()

	return false
}

func (g *GameState) initializeCombatAttackCursor() {
	targeting := g.CombatScene.AttackTargeting
	targeting.Cursor = targeting.Origin
	if !targeting.Attacker.IsPartyMember {
		return
	}

	lastTarget := g.CombatScene.lastCombatAttackTarget(targeting.Attacker)
	monster := g.CombatScene.MonsterManager.GetMonster(lastTarget)
	if monster == nil || !monster.IsActive() || monster.IsDead() ||
		monster.IsCorpserHidden() ||
		(monster.IsInvisible() && g.CombatScene.invisibilityProtectsTargets()) {
		return
	}

	// Original plraim has no terrain test: the cursor follows a remembered target onto
	// concealing Nothing or Arch, revealing where it moved. See ref:COMBAT-0004.
	position := monster.Position()
	if integerDistance(targeting.Origin, position) <= targeting.CurrentSource().MaxRange {
		targeting.Cursor = position
	}
}

// GetCombatAimDisposition classifies an aimed square without revealing hidden
// creatures. Invisible, inactive, and dead monsters deliberately appear empty.
func (g *GameState) GetCombatAimDisposition(position references.Position) CombatAimDisposition {
	if g.CombatScene == nil || !g.CombatTerrainAllowsObjectDisplay(position) {
		return CombatAimEmpty
	}
	for partyIndex := range g.CombatScene.PartyActive {
		if !g.CombatScene.PartyActive[partyIndex] || g.CombatScene.PartyPositions[partyIndex] != position {
			continue
		}
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: partyIndex}
		if g.CombatScene.isCombatPCUnderground(identity) {
			continue
		}
		if g.combatIdentityLoyalty(identity) == CombatMonsterLoyaltyMonster && g.CombatScene.AttackTargeting != nil {
			return CombatAimEnemy
		}

		return CombatAimFriendly
	}
	for _, clone := range g.CombatScene.partyClones {
		if clone.position != position {
			continue
		}
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: clone.partyIndex, Slot: clone.slot}
		if g.CombatScene.isCombatPCUnderground(identity) {
			continue
		}
		if g.combatIdentityLoyalty(identity) == CombatMonsterLoyaltyMonster && g.CombatScene.AttackTargeting != nil {
			return CombatAimEnemy
		}

		return CombatAimFriendly
	}

	disposition := CombatAimEmpty
	for _, monster := range g.CombatScene.MonsterManager.monsters {
		if !monster.IsActive() || monster.IsDead() ||
			monster.IsCorpserHidden() ||
			(monster.IsInvisible() && g.CombatScene.invisibilityProtectsTargets()) ||
			monster.Position() != position {
			continue
		}
		if monster.IsPartyLoyal() {
			return CombatAimFriendly
		}
		disposition = CombatAimEnemy
	}

	return disposition
}

func (g *GameState) combatAttackOccupantAt(
	position references.Position,
) (CombatTurnIdentity, *CombatMonster, int, bool) {
	for partyIndex := range g.CombatScene.PartyActive {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: partyIndex}
		if g.CombatScene.PartyActive[partyIndex] && !g.CombatScene.isCombatPCUnderground(identity) &&
			g.CombatScene.PartyPositions[partyIndex] == position {
			return identity, nil, NoCombatTarget, true
		}
	}
	for _, clone := range g.CombatScene.partyClones {
		identity := CombatTurnIdentity{
			IsPartyMember: true,
			EntityIndex:   clone.partyIndex,
			Slot:          clone.slot,
		}
		if clone.position == position && !g.CombatScene.isCombatPCUnderground(identity) {
			return identity, nil, NoCombatTarget, true
		}
	}
	for monsterIndex, monster := range g.CombatScene.MonsterManager.monsters {
		if monster.IsActive() && !monster.IsDead() && !monster.IsCorpserHidden() && monster.Position() == position {
			return CombatTurnIdentity{IsPartyMember: false, EntityIndex: monsterIndex}, monster, monsterIndex, true
		}
	}

	return CombatTurnIdentity{}, nil, NoCombatTarget, false
}

// IsMonsterAttacker reports whether this targeting sequence belongs to a controlled summon.
func (targeting *CombatAttackTargeting) IsMonsterAttacker() bool {
	return targeting != nil && !targeting.Attacker.IsPartyMember
}
