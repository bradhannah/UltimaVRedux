// Package map_units models the NPCs, enemies and vehicles that occupy a map,
// with their AI override, pending path and schedule state.
package map_units

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// MapUnitType identifies the kind of a map unit: NPC, enemy or vehicle.
type MapUnitType int

// NonPlayerCharacter, Enemy and Vehicle are the MapUnitType values; each MapUnit reports one.
const (
	NonPlayerCharacter MapUnitType = 0
	Enemy              MapUnitType = 1
	Vehicle            MapUnitType = 2
)

// PendingNPCInteraction identifies the command an adjacent NPC handed to the
// player interaction layer during its turn.
type PendingNPCInteraction uint8

// PendingNPCInteraction values distinguish no command, ordinary Talk, hostile Guard arrest,
// and source-backed town-monster combat.
const (
	PendingNPCInteractionNone PendingNPCInteraction = iota
	PendingNPCInteractionTalk
	PendingNPCInteractionGuardArrest
	PendingNPCInteractionHostileCombat
)

// MapUnit is the view shared by NPCs, enemies and vehicles on a map.
//
//nolint:interfacebloat // NPCs, enemies and vehicles share this one map-unit contract.
type MapUnit interface {
	GetMapUnitType() MapUnitType

	Pos() references.Position
	PosPtr() *references.Position
	Floor() references.FloorNumber

	SetPos(position references.Position)
	SetFloor(floor references.FloorNumber)

	MapUnitDetails() *MapUnitDetails
	SetVisible(visible bool)
	IsVisible() bool
	IsEmptyMapUnit() bool

	GetKeySpriteIndex() indexes.SpriteIndex
}

// MapUnitDetails is the state every map unit carries: position, floor, visibility,
// AI override and the pending path.
type MapUnitDetails struct {
	Position references.Position
	Floor    references.FloorNumber

	overriddenAiType     references.AiType
	aiOverrideProvenance references.AIOverrideProvenance

	Visible bool

	NPCNum int

	CurrentPath []references.Position

	// PendingInteraction preserves the original Talk-versus-hostile-contact
	// command until the interaction layer consumes it.
	PendingInteraction PendingNPCInteraction

	// SkipNextTurn alternates whirlpool movement eligibility on successive turns.
	// The movement toggle is distinct from contact/transport; ref:SHIP-0003.
	// When true, whirlpool skips its turn (50% movement probability per turn)
	SkipNextTurn bool

	arrivedScheduleSlot       references.NPCScheduleSlot
	consumedTalk3ScheduleSlot *references.NPCScheduleSlot
}

// ArrivedScheduleSlot returns the last schedule destination this unit
// authoritatively reached.
func (mu *MapUnitDetails) ArrivedScheduleSlot() references.NPCScheduleSlot {
	return mu.arrivedScheduleSlot
}

// RestoreArrivedScheduleSlot restores authoritative native snapshot state.
// Gameplay movement should update this state through NPCFriendly arrival APIs.
func (mu *MapUnitDetails) RestoreArrivedScheduleSlot(slot references.NPCScheduleSlot) {
	if !slot.IsValid() {
		panic("invalid arrived NPC schedule slot")
	}
	mu.arrivedScheduleSlot = slot
}

// ConsumedTalk3ScheduleSlot returns the schedule slot whose TALK3 action was
// consumed in this loaded NPC schedule copy.
func (mu *MapUnitDetails) ConsumedTalk3ScheduleSlot() (references.NPCScheduleSlot, bool) {
	if mu.consumedTalk3ScheduleSlot == nil {
		return 0, false
	}

	return *mu.consumedTalk3ScheduleSlot, true
}

func (mu *MapUnitDetails) recordConsumedTalk3ScheduleSlot(slot references.NPCScheduleSlot) {
	if !slot.IsValid() {
		panic("invalid consumed TALK3 schedule slot")
	}
	mu.consumedTalk3ScheduleSlot = &slot
}

// SetOverriddenAiType sets the AI override, recording guard-hostility or generic-flee
// provenance for the AI types that have one.
func (mu *MapUnitDetails) SetOverriddenAiType(oAiType references.AiType) {
	if oAiType == references.Unset {
		mu.SetAIOverride(oAiType, references.AIOverrideProvenanceNone)

		return
	}
	provenance := references.AIOverrideProvenanceNone
	switch oAiType {
	case references.Unset, references.Fixed:
	case references.ExtortOrAttackOrFollow, references.DrudgeWorthThing:
		provenance = references.AIOverrideProvenanceGuardHostility
	case references.FleeFromAvatar:
		provenance = references.AIOverrideProvenanceGenericFlee
	default:
	}
	mu.SetAIOverride(oAiType, provenance)
}

// SetLawAIOverride applies guard/crime behavior without hiding automatic Shadowlord influence.
func (mu *MapUnitDetails) SetLawAIOverride(oAiType references.AiType) {
	switch oAiType {
	case references.ExtortOrAttackOrFollow, references.DrudgeWorthThing, references.FleeFromAvatar:
	case references.Unset, references.Fixed:
		panic("law AI override cannot clear or fix AI")
	default:
		panic("unsupported law AI override")
	}

	switch mu.aiOverrideProvenance {
	case references.AIOverrideProvenanceAutomaticAstaroth,
		references.AIOverrideProvenanceAutomaticNosfentor:
		mu.SetAIOverride(oAiType, mu.aiOverrideProvenance)
	case references.AIOverrideProvenanceNone,
		references.AIOverrideProvenanceGuardHostility,
		references.AIOverrideProvenanceGenericFlee:
		mu.SetOverriddenAiType(oAiType)
	default:
		panic("invalid existing AI override provenance")
	}
}

// SetAIOverride replaces scheduled AI and records its authoritative cause.
func (mu *MapUnitDetails) SetAIOverride(
	oAiType references.AiType,
	provenance references.AIOverrideProvenance,
) {
	if !provenance.IsValid() {
		panic("invalid AI override provenance")
	}
	if oAiType == references.Unset && provenance != references.AIOverrideProvenanceNone {
		panic("unset AI override cannot retain provenance")
	}
	mu.overriddenAiType = oAiType
	mu.aiOverrideProvenance = provenance
}

// GetOverriddenAiType returns the AI override, or references.Unset when none is active.
func (mu *MapUnitDetails) GetOverriddenAiType() references.AiType {
	return mu.overriddenAiType
}

// AIOverrideProvenance returns the authoritative cause of the current override.
func (mu *MapUnitDetails) AIOverrideProvenance() references.AIOverrideProvenance {
	return mu.aiOverrideProvenance
}

// DequeueNextPosition removes and returns the next step of the pending path; it panics when no path is pending.
func (mu *MapUnitDetails) DequeueNextPosition() references.Position {
	if !mu.HasAPathAlreadyCalculated() {
		panic("NPC has no path calculated")
	}
	pos := mu.CurrentPath[0]
	mu.CurrentPath = mu.CurrentPath[1:]

	return pos
}

// HasAPathAlreadyCalculated reports whether the pending path still has steps.
func (mu *MapUnitDetails) HasAPathAlreadyCalculated() bool {
	if mu.CurrentPath == nil {
		return false
	}

	return len(mu.CurrentPath) > 0
}

// SetCurrentPath replaces the pending path; a nil path clears it.
func (mu *MapUnitDetails) SetCurrentPath(path []references.Position) {
	mu.CurrentPath = path
}
