package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// CombatEventKind identifies a durable event shown in the combat event tape.
type CombatEventKind int

const (
	// CombatEventBattleBegun marks the start of a new combat encounter.
	CombatEventBattleBegun CombatEventKind = iota
	// CombatEventAttack records one resolved attack source.
	CombatEventAttack
	// CombatEventSpell records one resolved combat spell.
	CombatEventSpell
	// CombatEventPoison records a successful poison application.
	CombatEventPoison
	// CombatEventCondition records a combatant sleeping through or waking on a turn.
	CombatEventCondition
	// CombatEventMove records a completed battlefield movement.
	CombatEventMove
	// CombatEventEquipmentShattered records consumed breakable equipment.
	CombatEventEquipmentShattered
	// CombatEventFled records a party member leaving the battlefield.
	CombatEventFled
	// CombatEventVictory records all hostile combatants being defeated.
	CombatEventVictory
	// CombatEventDefeat records all party combatants being defeated or fleeing.
	CombatEventDefeat
	// CombatEventDivision records a surviving monster creating an adjacent copy.
	CombatEventDivision
	// CombatEventFieldCreated records a magical field reaching its target square.
	CombatEventFieldCreated
	// CombatEventFieldTriggered records a field or equivalent terrain applying its effect to an actor.
	CombatEventFieldTriggered
	// CombatEventFieldDispelled records combat An Grav removing one field.
	CombatEventFieldDispelled
	// CombatEventFieldExpired records an independently dissipating field.
	CombatEventFieldExpired
	// CombatEventUtilitySpell records a battlefield object or teleport mutation.
	CombatEventUtilitySpell
	// CombatEventSummon records monsters created by a combat summoning spell.
	CombatEventSummon
	// CombatEventPossession records a monster successfully possessing an eligible PC.
	CombatEventPossession
	// CombatEventGating records a hostile daemon created by a gating monster.
	CombatEventGating
	// CombatEventRangedAttack records a physical projectile resolution.
	CombatEventRangedAttack
	// CombatEventRetired records a daemon disappearing without death or a corpse.
	CombatEventRetired
	// CombatEventRecovery records badtrip clearing possession and applying sleep.
	CombatEventRecovery
	// CombatEventRepelUndead records one aggregate An Xen Corp resolution.
	CombatEventRepelUndead
	// CombatEventVisibility records a monster disappearing or reappearing.
	CombatEventVisibility
	// CombatEventTeleport records a monster moving instantly to another square.
	CombatEventTeleport
	// CombatEventVanished records a dying monster disappearing without a corpse.
	CombatEventVanished
	// CombatEventPickup records an attempted Get and its committed inventory gain.
	CombatEventPickup
	// CombatEventLootRevealed presents one authoritative discovery at reveal time.
	CombatEventLootRevealed
	// CombatEventChestTrap records a chest's actual damage or poison consequence.
	CombatEventChestTrap
	// CombatEventInterruption records one action/source rejected by melee interference.
	CombatEventInterruption
	// CombatEventPossessionResistance records a surviving episode milestone.
	CombatEventPossessionResistance
	// CombatEventPossessionRelease records freedom without emergency pass-out.
	CombatEventPossessionRelease
)

// CombatInterruptedAction identifies the attempted action, before any spell or
// physical projectile has been committed.
type CombatInterruptedAction int

const (
	// CombatInterruptedAttack identifies a rejected physical attack source.
	CombatInterruptedAttack CombatInterruptedAction = iota
	// CombatInterruptedCast identifies rejection before the spell workflow opens.
	CombatInterruptedCast
)

// CombatSpellMonster identifies one monster considered by an aggregate combat spell.
type CombatSpellMonster struct {
	CombatantID CombatantID
	Index       int
	MonsterType references.MonsterType
	Position    references.Position
	Save        CombatHitResult
}

// CombatAnXenCorpResult is the authoritative result of one no-target cast.
type CombatAnXenCorpResult struct {
	Affected []CombatSpellMonster
	Resisted []CombatSpellMonster
	Immune   []CombatSpellMonster
}

// CombatAnXenCorpEvent is the durable aggregate event for one completed cast.
type CombatAnXenCorpEvent struct {
	Caster CombatFeedbackEntity
	Result CombatAnXenCorpResult
}

// CombatSummonOutcome describes whether a combat summon changed the encounter.
type CombatSummonOutcome int

const (
	// CombatSummonSucceeded means at least one monster was created.
	CombatSummonSucceeded CombatSummonOutcome = iota
	// CombatSummonNoLegalPosition means all eight original placement attempts failed.
	CombatSummonNoLegalPosition
	// CombatSummonCapacityFull means every combat slot is held by the living or by
	// a field; remains and loot do not count. See ref:COMBAT-0001.
	CombatSummonCapacityFull
	// CombatSummonInvalidCaster means the requested daemon caster does not exist.
	CombatSummonInvalidCaster
)

// CombatSummonBindingOutcome describes which side controls newly summoned monsters.
type CombatSummonBindingOutcome int

const (
	// CombatSummonBindingNotApplicable means no monster was created or binding has not resolved.
	CombatSummonBindingNotApplicable CombatSummonBindingOutcome = iota
	// CombatSummonBoundToParty means every created monster supports the party.
	CombatSummonBoundToParty
	// CombatSummonBindingFailed means the daemon remained hostile (the original "Oops" outcome).
	CombatSummonBindingFailed
)

// CombatSummonedMonster identifies one newly registered encounter combatant.
type CombatSummonedMonster struct {
	CombatantID CombatantID
	Index       int
	MonsterType references.MonsterType
	Position    references.Position
}

// CombatVanishedMonster identifies one monster removed by the original disappearance death path.
type CombatVanishedMonster struct {
	CombatantID CombatantID
	Index       int
	MonsterType references.MonsterType
	Position    references.Position
}

// CombatSummonResult is the authoritative result of one no-target summon cast.
type CombatSummonResult struct {
	Spell    references.Spell
	Outcome  CombatSummonOutcome
	Created  []CombatSummonedMonster
	Count    int
	Binding  CombatSummonBindingOutcome
	Attempts int
}

// CombatSummonEvent is the durable HUD/VFX payload for a completed summon mutation.
type CombatSummonEvent struct {
	Spell   references.Spell
	Created []CombatSummonedMonster
	Count   int
	Binding CombatSummonBindingOutcome
}

// CombatUtilityResult identifies the concrete result of a utility spell.
type CombatUtilityResult int

const (
	// CombatUtilityDoorUnlocked records An Sanct opening a mundane lock.
	CombatUtilityDoorUnlocked CombatUtilityResult = iota
	// CombatUtilityChestDisarmed records An Sanct removing a chest trap and lock.
	CombatUtilityChestDisarmed
	// CombatUtilityDoorMagicLocked records An Ex Por sealing a door.
	CombatUtilityDoorMagicLocked
	// CombatUtilityMagicLockRemoved records In Ex Por releasing a sealed door.
	CombatUtilityMagicLockRemoved
	// CombatUtilityTeleported records In Por moving a party member.
	CombatUtilityTeleported
)

// CombatConditionResult describes a condition outcome at a scheduled turn boundary.
type CombatConditionResult int

const (
	// CombatConditionSleeping means the combatant remained asleep and lost the turn.
	CombatConditionSleeping CombatConditionResult = iota
	// CombatConditionWoke means the combatant woke but still lost the current turn.
	CombatConditionWoke
	// CombatConditionQuicknessSuppressed means Rel Tym prevented a monster action.
	CombatConditionQuicknessSuppressed
	// CombatConditionDraggedUnder means a surviving PC was pulled beneath a Corpser.
	CombatConditionDraggedUnder
	// CombatConditionUnderground means a PC failed to escape and lost the turn.
	CombatConditionUnderground
	// CombatConditionRegurgitated means a PC escaped underground but still lost the turn.
	CombatConditionRegurgitated
)

// CombatVisibilityResult describes a monster visibility toggle.
type CombatVisibilityResult int

const (
	// CombatVisibilityHidden means the monster became invisible.
	CombatVisibilityHidden CombatVisibilityResult = iota
	// CombatVisibilityRevealed means the monster became visible.
	CombatVisibilityRevealed
)

// CombatDiagnostics records only values consumed by authoritative combat logic.
type CombatDiagnostics struct {
	Hit                  CombatHitResult
	Damage               CombatDamageResult
	Source               references.Equipment
	FinalDamage          int
	TargetBaseDex        int
	UsedStrength         bool
	TargetUndead         bool
	TargetImmortal       bool
	MagicWeapon          bool
	PoisonRoll           int
	PoisonRollMaximum    int
	PoisonRolled         bool
	PoisonFallbackDamage int
	PoisonFallbackRolled bool
	TheftRoll            int
	TheftRollMaximum     int
	TheftRolled          bool
	FoodStolen           int
	Spell                references.Spell
	MagicDamage          bool
	ProjectileBlocked    bool
	ForcedMiss           bool
	Missile              references.MissileType
}

// CombatEvent describes one meaningful combat result without presentation text.
type CombatEvent struct {
	Kind     CombatEventKind
	Attacker CombatFeedbackEntity
	Target   CombatFeedbackEntity
	Result   CombatFeedbackResult
	HPLost   int
	Fatal    bool
	// MonsterInjury is captured at damage resolution, even when HP bars are shown.
	MonsterInjury CombatMonsterInjury
	From          references.Position
	To            references.Position
	Condition     CombatConditionResult
	Visibility    CombatVisibilityResult
	FieldKind     CombatFieldKind
	// FieldFizzled distinguishes a field expiring during the same action that created it.
	FieldFizzled  bool
	UtilityResult CombatUtilityResult
	Summon        CombatSummonEvent
	AnXenCorp     CombatAnXenCorpEvent
	Created       CombatSummonedMonster
	Vanished      CombatVanishedMonster
	Diagnostics   CombatDiagnostics
	// SpellTarget is the authoritative area-spell outcome, including disclosure
	// captured before damage/death can retire the exact target entry.
	SpellTarget *CombatAreaSpellTargetResult
	// FireInjury contains detached pre-damage disclosure and scalar lifetime IDs,
	// never live combat/provider pointers. Typed Flame Wind keeps its SpellTarget owner.
	FireInjury              *CombatFireInjury
	Pickup                  CombatPickupEvent
	LootReveal              CombatLootReveal
	ChestTrap               CombatChestTrapResult
	InterruptedAction       CombatInterruptedAction
	PossessionEpisodeID     uint64
	PossessionTurns         int
	PossessionReleaseReason CombatPossessionReleaseReason
}

// CombatSpecialConclusion identifies an encounter outcome requiring external orchestration.
type CombatSpecialConclusion int

const (
	// CombatSpecialConclusionNone means ordinary combat cleanup should continue.
	CombatSpecialConclusionNone CombatSpecialConclusion = iota
	// CombatSpecialConclusionDoomMirror hands a successful final-room fight to the finale.
	CombatSpecialConclusionDoomMirror
)

// CombatConclusion is emitted once after authoritative combat cleanup completes.
type CombatConclusion struct {
	Reason    CombatEndReason
	Special   CombatSpecialConclusion
	Collected []CombatCollectedItem
}

// CombatCallbacks publishes structured combat events to the presentation layer.
type CombatCallbacks struct {
	PublishEvent      func(CombatEvent)
	PublishConclusion func(CombatConclusion)
}

// NewCombatCallbacks validates required combat event and conclusion publication.
func NewCombatCallbacks(
	publishEvent func(CombatEvent),
	publishConclusion func(CombatConclusion),
) CombatCallbacks {
	if publishEvent == nil {
		panic("CombatCallbacks.PublishEvent is required - cannot be nil")
	}
	if publishConclusion == nil {
		panic("CombatCallbacks.PublishConclusion is required - cannot be nil")
	}

	return CombatCallbacks{PublishEvent: publishEvent, PublishConclusion: publishConclusion}
}
