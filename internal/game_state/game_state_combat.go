package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Static errors for combat operations
var (
	ErrRoomIndexOutOfRange = errors.New("room index out of range")
)

// CombatEntryDirection maps the 4 CBT direction indices to meaningful entry directions.
// In CBT files, rows 1-4 define player positions for each entry direction.
// The mapping follows the original Ultima V dungeon encounter setup conventions:
//   - Row 1 (index 0): West entry
//   - Row 2 (index 1): East entry
//   - Row 3 (index 2): North entry (default for outdoor combat)
//   - Row 4 (index 3): South entry
//
// See ref:COMBAT-0002.
type CombatEntryDirection int

// Combat entry direction constants matching CBT file row indices.
// The constant values directly index into PlayerPositions[dir][player].
const (
	CombatEntryWest  CombatEntryDirection = 0 // Row 1 in CBT file
	CombatEntryEast  CombatEntryDirection = 1 // Row 2 in CBT file
	CombatEntryNorth CombatEntryDirection = 2 // Row 3 in CBT file (default)
	CombatEntrySouth CombatEntryDirection = 3 // Row 4 in CBT file
)

// combatMapCenterCoord is the center of the 11x11 combat map (used as default position)
const combatMapCenterCoord = 5

// CombatEncounterID identifies one tactical encounter within a GameState lifetime.
type CombatEncounterID uint64

// CombatantID identifies one monster and is never reused within its encounter.
type CombatantID uint64

// Monster turn delay constants — separate values for attacks and moves so they
// can be independently tweaked. Both start at 1000ms.
const (
	// MonsterAttackDelayMs is the delay before a monster executes an attack.
	// Longer delay gives the player time to see the red highlight on the target PC.
	MonsterAttackDelayMs = 1000

	// MonsterMoveDelayMs is the anticipation before a monster's visible movement animation.
	MonsterMoveDelayMs = 250
)

// Sentinel values for combat state indices.
const (
	// NoActiveMonster indicates no monster is currently taking its turn.
	NoActiveMonster = -1

	// NoTargetPC indicates no PC is currently being targeted by a monster attack.
	NoTargetPC = -1
)

// CombatMoveResult describes the outcome of a combat movement attempt.
type CombatMoveResult int

const (
	// CombatMoveBlocked means the move was invalid (out of bounds blocked by terrain/occupant).
	CombatMoveBlocked CombatMoveResult = iota
	// CombatMoveMoved means the character successfully moved to the new position.
	CombatMoveMoved
	// CombatMoveFled means the character walked off the map edge and fled combat.
	CombatMoveFled
)

// CombatEndReason describes why combat ended.
type CombatEndReason int

const (
	// CombatNotEnded means combat is still in progress.
	CombatNotEnded CombatEndReason = iota
	// CombatVictory means all monsters are dead — PCs won.
	CombatVictory
	// CombatDefeat means all PCs are dead/fled with monsters remaining.
	CombatDefeat
	// CombatEvacuated means all PCs fled and all monsters are also gone.
	CombatEvacuated
)

// CombatState manages the state of an active combat encounter.
// It tracks party member positions on the 11x11 combat grid,
// the current turn order, and the combat map reference.
type CombatState struct {
	// EncounterID prevents delayed debug requests from targeting a replacement encounter.
	EncounterID CombatEncounterID

	// nextCombatantID allocates immutable monster identities for this encounter.
	nextCombatantID CombatantID

	// CombatMapRef is the loaded combat map with tile data and position arrays
	CombatMapRef *references.CombatMapReference

	// PartyPositions tracks each party member's current position on the combat map.
	// Indexed by party member index (0-5).
	PartyPositions [party_state.MaxActiveParty]references.Position

	// PartyActive tracks which party members are participating in combat.
	// Dead or absent characters are marked false.
	PartyActive [party_state.MaxActiveParty]bool
	// partyClones are encounter-local combat proxies. They deliberately point at
	// the same persistent character through their queue provider, but own a
	// distinct position and DEX schedule like original prepared-spell rules's duplicated combat queue slot.
	// See ref:COMBAT-0002.
	partyClones     []combatPCClone
	nextPCCloneSlot int
	// PartyInvisible tracks encounter-local Sanct Lor and invisibility-ring state.
	PartyInvisible [party_state.MaxActiveParty]bool
	// PartySleeping mirrors the per-combat-entry SLEEP_MASK for normal PC slots.
	PartySleeping [party_state.MaxActiveParty]bool
	// PartyUnderground mirrors the encounter-local UNDER_MASK for normal PC slots.
	PartyUnderground [party_state.MaxActiveParty]bool
	// PartyPossessed is encounter-local daemon charm allegiance. The saved party
	// record remains shared, while clones own an independent condition below.
	PartyPossessed          [party_state.MaxActiveParty]bool
	possessions             map[CombatTurnIdentity]combatPossessionEpisode
	nextPossessionEpisode   uint64
	nextPossessionTurn      uint64
	completedPossessionTurn uint64
	possessionTurn          combatPossessionTurn

	// EntryDirection is the direction from which the party entered combat
	EntryDirection CombatEntryDirection
	// ExitContext records whether physical exits must be shared by the party.
	ExitContext CombatExitContext
	// ExitRoute records the shared physical or Doom-mirror departure route.
	ExitRoute CombatExitRoute

	// CurrentTurnIndex is the party member index whose turn it currently is (0-5)
	CurrentTurnIndex int
	// CurrentTurnIdentity is the authoritative queue actor. CurrentTurnIndex remains
	// the party index used by established PC-only systems.
	CurrentTurnIdentity    CombatTurnIdentity
	currentTurnIdentitySet bool

	// PreCombatPlayerLocation stores the full player location for restoration
	PreCombatPlayerLocation references.PlayerLocation
	// World selection is independent of combat-local turn and solo selection.
	preCombatActivePlayerIndex int

	// OverworldOrigin is non-nil only for normal-play large-map encounters.
	OverworldOrigin *OverworldEncounterOrigin
	// SmallMapOrigin is non-nil only for source-backed town NPC encounters.
	SmallMapOrigin *SmallMapEncounterOrigin
	// DungeonRoomOrigin is non-nil only for a room entered from an authoritative A*/F* cell.
	DungeonRoomOrigin *DungeonRoomEncounterOrigin
	// DungeonWandererOrigin is non-nil only for a generated roaming-monster encounter.
	DungeonWandererOrigin *DungeonWandererEncounterOrigin
	// CampingOrigin is non-nil only for a source-less camping ambush.
	CampingOrigin *CampingCombatOrigin
	// DoomEntranceOrigin is non-nil only for the generic Shadowlord blocking rejected Doom entry.
	DoomEntranceOrigin *DoomEntranceEncounterOrigin

	// MonsterManager manages all monsters in the combat encounter
	MonsterManager *CombatMonsterManager

	// TurnQueue is the dex-based priority queue for combat turn ordering.
	// Replaces the simple round-robin AdvanceTurn() / findNextActiveIndex()
	// with the original Ultima V countdown scheduler behavior from original combat scheduler.
	// nil until combat begins (populated in NewCombatState).
	// See ref:COMBAT-0002.
	TurnQueue *CombatTurnQueue

	// Victory is set true when all hostile monsters are dead (original: victory flag).
	// When true, Escape allows leaving combat via ActionEscapeCombatMap.
	Victory bool

	// DebugOutcomeSuppressed keeps an explicitly outcome-free debug clear from
	// turning into victory when the next combat turn checks for zero monsters.
	DebugOutcomeSuppressed bool

	// SoloMoveMode - when true, only SoloMoveCharacterIndex moves per round
	SoloMoveMode bool

	// SoloMoveCharacterIndex is the character index (0-5) that moves in solo mode
	SoloMoveCharacterIndex int

	// ActiveMonsterIndex is the monster index currently taking its turn, or NoActiveMonster (-1)
	// if no monster is active. Used by the renderer to draw a yellow highlight on the active monster.
	ActiveMonsterIndex int

	// ActiveTargetPCIndex is the party member index being targeted by the active monster's
	// planned attack, or NoTargetPC (-1) if the monster is not attacking. Used by the renderer
	// to draw a red highlight on the target PC during the monster turn delay.
	ActiveTargetPCIndex int
	// ActiveTargetIdentity identifies either a threatened PC or party-loyal monster.
	ActiveTargetIdentity    CombatTurnIdentity
	ActiveTargetIdentitySet bool

	turnsStarted              bool
	presentTurns              bool
	nextPresentationSequence  uint64
	pendingPresentationStep   *pendingCombatPresentationStep
	lastCompletedPresentation CombatPresentationToken

	// AttackTargeting holds the active PC aiming sequence, or nil outside aiming mode.
	AttackTargeting *CombatAttackTargeting
	// SpellTargeting holds an active combat spell crosshair, or nil outside spell aiming.
	SpellTargeting *CombatSpellTargeting
	// ConeTargeting holds an active directional spell preview, or nil outside cone aiming.
	ConeTargeting *CombatConeTargeting
	// AdjacentSpellTargeting holds an active adjacent-square spell preview.
	AdjacentSpellTargeting *CombatAdjacentSpellTargeting
	pendingSpell           *pendingCombatSpell
	pendingAttack          *pendingCombatAttack
	pendingMonsterMissile  *pendingMonsterMissile

	// LastAttackTargets remembers each PC's most recent monster target. Original
	// plraim() reuses object.misc while that target lives, is not magically
	// invisible or underground, and is in range, even on concealing terrain.
	LastAttackTargets [party_state.MaxActiveParty]int
	// meleeEngagers remembers the last adjacent attacker for each exact
	// encounter identity; clones do not alias their source PC. ref:EQUIP-0016
	meleeEngagers map[CombatTurnIdentity]CombatTurnIdentity

	// objects contains encounter-local chests and loose treasure. It is discarded
	// with CombatState and never shares ownership with world ItemStacksMap entries.
	objects []CombatObject
	// lastObjectID is encounter-local and never resets when objects are removed.
	lastObjectID CombatObjectID
	// Collection accounting is independent of the presentation's bounded history.
	collectedItems []CombatCollectedItem
	// fields contains encounter-local magical fields in original creation order.
	fields []CombatField

	specialRingsInitialized bool
}

func (combatState *CombatState) hasPlayerActionOwner() bool {
	return combatState.AttackTargeting != nil || combatState.SpellTargeting != nil ||
		combatState.ConeTargeting != nil || combatState.AdjacentSpellTargeting != nil ||
		combatState.pendingAttack != nil || combatState.pendingSpell != nil
}

// NewCombatState creates a new CombatState for a combat encounter.
// It initializes party positions from the combat map's CBT data for the given entry direction.
func NewCombatState(
	combatMapRef *references.CombatMapReference,
	entryDir CombatEntryDirection,
	party *party_state.PartyState,
) *CombatState {
	combatState := &CombatState{
		EncounterID:      0,
		nextCombatantID:  1,
		CombatMapRef:     combatMapRef,
		PartyPositions:   [party_state.MaxActiveParty]references.Position{},
		PartyActive:      [party_state.MaxActiveParty]bool{},
		partyClones:      make([]combatPCClone, 0),
		nextPCCloneSlot:  1,
		PartyInvisible:   [party_state.MaxActiveParty]bool{},
		PartySleeping:    [party_state.MaxActiveParty]bool{},
		PartyUnderground: [party_state.MaxActiveParty]bool{},
		PartyPossessed:   [party_state.MaxActiveParty]bool{},
		EntryDirection:   entryDir,
		ExitContext:      CombatExitSurface,
		ExitRoute:        CombatExitNone,
		CurrentTurnIndex: 0,
		CurrentTurnIdentity: CombatTurnIdentity{
			IsPartyMember: true,
			EntityIndex:   0,
		},
		currentTurnIdentitySet: true,
		PreCombatPlayerLocation: references.PlayerLocation{
			Location: 0,
		},
		preCombatActivePlayerIndex: -1,
		OverworldOrigin:            nil,
		SmallMapOrigin:             nil,
		DungeonRoomOrigin:          nil,
		DungeonWandererOrigin:      nil,
		CampingOrigin:              nil,
		DoomEntranceOrigin:         nil,
		MonsterManager:             NewCombatMonsterManager(),
		ActiveMonsterIndex:         NoActiveMonster,
		ActiveTargetPCIndex:        NoTargetPC,
		meleeEngagers:              make(map[CombatTurnIdentity]CombatTurnIdentity),
		objects:                    make([]CombatObject, 0),
		fields:                     make([]CombatField, 0, maxCombatFields),
	}
	for i := range combatState.LastAttackTargets {
		combatState.LastAttackTargets[i] = NoCombatTarget
	}

	// Initialize party positions from CBT data and determine active members
	for i := range party_state.MaxActiveParty {
		character := party.GetCharacter(i)
		if character.IsInParty() && !character.IsDead() {
			combatState.PartyActive[i] = true
			combatState.PartySleeping[i] = character.IsAsleep()
			combatState.PartyPositions[i] = combatMapRef.PlayerPositions[int(entryDir)][i]
		}
	}

	// Build the dex-based turn queue (replicates original combat scheduler countdown scheduler).
	// PCs are added first (slots 0-5), matching the original combat queue[] layout.
	// See ref:COMBAT-0002.
	combatState.TurnQueue = NewCombatTurnQueue()

	for i := range party_state.MaxActiveParty {
		if combatState.PartyActive[i] {
			pcProvider := NewCombatPCProvider(party.GetCharacter(i))
			pcProvider.sleeping = &combatState.PartySleeping[i]
			pcProvider.underground = &combatState.PartyUnderground[i]
			combatState.TurnQueue.AddCombatant(pcProvider, i)
			if !combatState.registerCombatantObject(CombatTurnIdentity{IsPartyMember: true, EntityIndex: i}) {
				panic("NewCombatState: original object slots exhausted by party")
			}
		}
	}

	// Find the first active party member for the initial turn
	combatState.CurrentTurnIndex = combatState.findNextActiveIndex(0)
	combatState.CurrentTurnIdentity = CombatTurnIdentity{
		IsPartyMember: true,
		EntityIndex:   combatState.CurrentTurnIndex,
	}

	logging.Debugf(logging.CategoryGameState,
		"Created CombatState: map=%q entry=%d activeMembers=%v firstTurn=%d",
		combatMapRef.MapName, entryDir, combatState.PartyActive, combatState.CurrentTurnIndex)

	return combatState
}

// GetPartyMemberPosition returns the current position of a party member on the combat map
func (combatState *CombatState) GetPartyMemberPosition(memberIndex int) references.Position {
	if memberIndex < 0 || memberIndex >= party_state.MaxActiveParty {
		return references.Position{X: combatMapCenterCoord, Y: combatMapCenterCoord}
	}

	return combatState.PartyPositions[memberIndex]
}

type combatPCClone struct {
	partyIndex       int
	slot             int
	position         references.Position
	possessed        bool
	invisible        bool
	sleeping         *bool
	underground      *bool
	lastAttackTarget int
}

// GetCombatantPosition returns the position owned by an exact queue identity.
func (combatState *CombatState) GetCombatantPosition(identity CombatTurnIdentity) references.Position {
	if !identity.IsPartyMember {
		monster := combatState.MonsterManager.GetMonster(identity.EntityIndex)
		if monster == nil {
			return references.Position{}
		}

		return monster.Position()
	}
	if identity.Slot == 0 {
		return combatState.GetPartyMemberPosition(identity.EntityIndex)
	}
	for _, clone := range combatState.partyClones {
		if clone.partyIndex == identity.EntityIndex && clone.slot == identity.Slot {
			return clone.position
		}
	}

	return references.Position{}
}

// IsPCClone reports whether identity is an encounter-local PC proxy.
func (combatState *CombatState) IsPCClone(identity CombatTurnIdentity) bool {
	return identity.IsPartyMember && identity.Slot != 0
}

// IsCombatPCRat reports the Purple potion's appearance without changing actor identity.
func (combatState *CombatState) IsCombatPCRat(identity CombatTurnIdentity) bool {
	entry := combatState.TurnQueue.GetEntryByIdentity(identity)

	return identity.IsPartyMember && entry != nil && entry.ratAppearance
}

func (combatState *CombatState) combatPCClone(identity CombatTurnIdentity) *combatPCClone {
	if !combatState.IsPCClone(identity) {
		return nil
	}
	for index := range combatState.partyClones {
		clone := &combatState.partyClones[index]
		if clone.partyIndex == identity.EntityIndex && clone.slot == identity.Slot {
			return clone
		}
	}

	return nil
}

func (combatState *CombatState) isCombatPCPossessed(identity CombatTurnIdentity) bool {
	if !identity.IsPartyMember {
		return false
	}
	if identity.Slot == 0 {
		return combatState.PartyPossessed[identity.EntityIndex]
	}
	clone := combatState.combatPCClone(identity)

	return clone != nil && clone.possessed
}

func (combatState *CombatState) isCombatPCActive(identity CombatTurnIdentity) bool {
	if !identity.IsPartyMember {
		return false
	}
	if identity.Slot == 0 {
		return combatState.PartyActive[identity.EntityIndex]
	}

	return combatState.combatPCClone(identity) != nil
}

// IsCombatPCActive reports whether an exact normal PC or clone remains present.
func (combatState *CombatState) IsCombatPCActive(identity CombatTurnIdentity) bool {
	return combatState.isCombatPCActive(identity)
}

func (combatState *CombatState) setCombatPCPossessed(identity CombatTurnIdentity, possessed bool) {
	if !possessed {
		delete(combatState.possessions, identity)
	}
	if !identity.IsPartyMember {
		panic("setCombatPCPossessed called for monster")
	}
	if identity.Slot == 0 {
		combatState.PartyPossessed[identity.EntityIndex] = possessed

		return
	}
	clone := combatState.combatPCClone(identity)
	if clone == nil {
		panic("setCombatPCPossessed called for unknown PC clone")
	}
	clone.possessed = possessed
}

// IsCombatPCPossessed reports entry-local charm for a normal PC or clone.
func (combatState *CombatState) IsCombatPCPossessed(identity CombatTurnIdentity) bool {
	return combatState.isCombatPCPossessed(identity)
}

func (combatState *CombatState) isCombatPCInvisible(identity CombatTurnIdentity) bool {
	if !identity.IsPartyMember {
		return false
	}
	if identity.Slot == 0 {
		return combatState.PartyInvisible[identity.EntityIndex]
	}
	clone := combatState.combatPCClone(identity)

	return clone != nil && clone.invisible
}

func (combatState *CombatState) setCombatPCInvisible(identity CombatTurnIdentity, invisible bool) {
	if !identity.IsPartyMember {
		panic("setCombatPCInvisible called for monster")
	}
	if identity.Slot == 0 {
		combatState.PartyInvisible[identity.EntityIndex] = invisible

		return
	}
	clone := combatState.combatPCClone(identity)
	if clone == nil {
		panic("setCombatPCInvisible called for unknown PC clone")
	}
	clone.invisible = invisible
}

// IsCombatPCInvisible reports entry-local invisibility for rendering adapters.
func (combatState *CombatState) IsCombatPCInvisible(identity CombatTurnIdentity) bool {
	return combatState.isCombatPCInvisible(identity)
}

func (combatState *CombatState) isCombatPCSleeping(identity CombatTurnIdentity) bool {
	if !identity.IsPartyMember {
		return false
	}
	if identity.Slot == 0 {
		return combatState.PartySleeping[identity.EntityIndex]
	}
	clone := combatState.combatPCClone(identity)

	return clone != nil && clone.sleeping != nil && *clone.sleeping
}

func (combatState *CombatState) setCombatPCSleeping(identity CombatTurnIdentity, sleeping bool) {
	if !identity.IsPartyMember {
		panic("setCombatPCSleeping called for monster")
	}
	if identity.Slot == 0 {
		combatState.PartySleeping[identity.EntityIndex] = sleeping

		return
	}
	clone := combatState.combatPCClone(identity)
	if clone == nil || clone.sleeping == nil {
		panic("setCombatPCSleeping called for unknown PC clone")
	}
	*clone.sleeping = sleeping
}

// IsCombatPCSleeping reports entry-local sleep for rendering adapters.
func (combatState *CombatState) IsCombatPCSleeping(identity CombatTurnIdentity) bool {
	return combatState.isCombatPCSleeping(identity)
}

func (combatState *CombatState) isCombatPCUnderground(identity CombatTurnIdentity) bool {
	if !identity.IsPartyMember {
		return false
	}
	if identity.Slot == 0 {
		return combatState.PartyUnderground[identity.EntityIndex]
	}
	clone := combatState.combatPCClone(identity)

	return clone != nil && clone.underground != nil && *clone.underground
}

func (combatState *CombatState) setCombatPCUnderground(identity CombatTurnIdentity, underground bool) {
	if !identity.IsPartyMember {
		panic("setCombatPCUnderground called for monster")
	}
	if identity.Slot == 0 {
		combatState.PartyUnderground[identity.EntityIndex] = underground

		return
	}
	clone := combatState.combatPCClone(identity)
	if clone == nil || clone.underground == nil {
		panic("setCombatPCUnderground called for unknown PC clone")
	}
	*clone.underground = underground
}

// IsCombatPCUnderground reports entry-local Corpser captivity for rendering adapters.
func (combatState *CombatState) IsCombatPCUnderground(identity CombatTurnIdentity) bool {
	return combatState.isCombatPCUnderground(identity)
}

func (g *GameState) setCombatPCSleeping(identity CombatTurnIdentity, sleeping bool) {
	g.CombatScene.setCombatPCSleeping(identity, sleeping)
	character := g.PartyState.GetCharacter(identity.EntityIndex)
	if character.IsDead() {
		return
	}
	if sleeping {
		character.SetStatus(party_state.Sleep)

		return
	}
	if character.GetStatus() == party_state.Sleep {
		character.SetStatus(party_state.Good)
	}
}

// ForEachPCClone exposes encounter-local clone positions to the rendering adapter.
func (combatState *CombatState) ForEachPCClone(fn func(CombatTurnIdentity, references.Position)) {
	for _, clone := range combatState.partyClones {
		fn(CombatTurnIdentity{IsPartyMember: true, EntityIndex: clone.partyIndex, Slot: clone.slot}, clone.position)
	}
}

func (combatState *CombatState) setCombatantPosition(identity CombatTurnIdentity, position references.Position) {
	if !identity.IsPartyMember {
		panic("setCombatantPosition called for monster")
	}
	if identity.Slot == 0 {
		combatState.SetPartyMemberPosition(identity.EntityIndex, position)

		return
	}
	for index := range combatState.partyClones {
		clone := &combatState.partyClones[index]
		if clone.partyIndex == identity.EntityIndex && clone.slot == identity.Slot {
			clone.position = position

			return
		}
	}
	panic("setCombatantPosition called for unknown PC clone")
}

func (combatState *CombatState) addPCCloneFromSource(
	source CombatTurnIdentity,
	partyIndex int,
	position references.Position,
	provider *CombatPCProvider,
) CombatTurnIdentity {
	if !combatState.TurnQueue.hasFreeSourceSlot(0) || !combatState.hasFreeOriginalObjectSlot() {
		return CombatTurnIdentity{}
	}
	identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: partyIndex, Slot: combatState.nextPCCloneSlot}
	combatState.nextPCCloneSlot++
	sleeping := combatState.isCombatPCSleeping(source)
	underground := combatState.isCombatPCUnderground(source)
	provider.sleeping = &sleeping
	provider.underground = &underground
	combatState.partyClones = append(combatState.partyClones, combatPCClone{
		partyIndex:       partyIndex,
		slot:             identity.Slot,
		position:         position,
		possessed:        combatState.isCombatPCPossessed(source),
		invisible:        combatState.isCombatPCInvisible(source),
		sleeping:         &sleeping,
		underground:      &underground,
		lastAttackTarget: combatState.lastCombatAttackTarget(source),
	})
	combatState.TurnQueue.AddClonedCombatant(provider, partyIndex, identity.Slot, source)
	combatState.cloneCombatPossession(source, identity)
	if !combatState.registerCombatantObject(identity) {
		panic("addPCCloneFromSource: preflighted original object slot is unavailable")
	}

	return identity
}

func (combatState *CombatState) lastCombatAttackTarget(identity CombatTurnIdentity) int {
	if identity.Slot == 0 {
		return combatState.LastAttackTargets[identity.EntityIndex]
	}
	for _, clone := range combatState.partyClones {
		if clone.partyIndex == identity.EntityIndex && clone.slot == identity.Slot {
			return clone.lastAttackTarget
		}
	}

	return NoCombatTarget
}

func (combatState *CombatState) setLastCombatAttackTarget(identity CombatTurnIdentity, target int) {
	if identity.Slot == 0 {
		combatState.LastAttackTargets[identity.EntityIndex] = target

		return
	}
	for index := range combatState.partyClones {
		clone := &combatState.partyClones[index]
		if clone.partyIndex == identity.EntityIndex && clone.slot == identity.Slot {
			clone.lastAttackTarget = target

			return
		}
	}
}

func (combatState *CombatState) removePCClone(identity CombatTurnIdentity) {
	delete(combatState.possessions, identity)
	for index, clone := range combatState.partyClones {
		if clone.partyIndex == identity.EntityIndex && clone.slot == identity.Slot {
			combatState.partyClones = append(combatState.partyClones[:index], combatState.partyClones[index+1:]...)

			return
		}
	}
}

// SetPartyMemberPosition updates a party member's position on the combat map
func (combatState *CombatState) SetPartyMemberPosition(memberIndex int, pos references.Position) {
	if memberIndex < 0 || memberIndex >= party_state.MaxActiveParty {
		return
	}
	combatState.PartyPositions[memberIndex] = pos
}

// IsPartyMemberActive returns whether a party member is participating in combat
func (combatState *CombatState) IsPartyMemberActive(memberIndex int) bool {
	if memberIndex < 0 || memberIndex >= party_state.MaxActiveParty {
		return false
	}

	return combatState.PartyActive[memberIndex]
}

func (g *GameState) markCombatPCDead(pcIndex int) {
	for identity := range g.CombatScene.possessions {
		if identity.EntityIndex == pcIndex {
			g.CombatScene.setCombatPCPossessed(identity, false)
		}
	}
	type defeatedPCEntry struct {
		identity CombatTurnIdentity
		position references.Position
	}
	defeated := make([]defeatedPCEntry, 0, 1)
	for _, entry := range g.CombatScene.TurnQueue.entries {
		if entry.status != turnQueueActive || !entry.Combatant.IsPC() || entry.EntityIndex != pcIndex {
			continue
		}
		identity := turnQueueIdentity(entry)
		defeated = append(defeated, defeatedPCEntry{
			identity: identity,
			position: g.CombatScene.GetCombatantPosition(identity),
		})
	}
	for _, combatant := range defeated {
		g.CombatScene.TurnQueue.markDeadIdentity(combatant.identity)
		if !g.CombatScene.transferCombatantObjectTo(combatant.identity, NewCombatDeadPC(combatant.position)) {
			panic("markCombatPCDead: dead PC has no original object slot")
		}
	}
	g.CombatScene.PartyActive[pcIndex] = false
	g.CombatScene.PartyUnderground[pcIndex] = false
	for index := len(g.CombatScene.partyClones) - 1; index >= 0; index-- {
		if g.CombatScene.partyClones[index].partyIndex == pcIndex {
			g.CombatScene.partyClones = append(
				g.CombatScene.partyClones[:index], g.CombatScene.partyClones[index+1:]...,
			)
		}
	}
	g.clearInactiveCombatPCSelection(pcIndex)
}

func (g *GameState) clearInactiveCombatPCSelection(pcIndex int) {
	if g.CombatScene.IsSoloMoveActive() && g.CombatScene.GetSoloMoveCharacter() == pcIndex {
		g.CombatScene.ClearSoloMoveMode()
	}
	if g.GetActivePlayerIndex() == pcIndex {
		g.ClearActivePlayer()
	}
}

// GetCurrentTurnMemberIndex returns the index of the party member whose turn it is
func (combatState *CombatState) GetCurrentTurnMemberIndex() int {
	return combatState.CurrentTurnIndex
}

// AdvanceTurn moves to the next active party member's turn.
// Returns the new active member index.
func (combatState *CombatState) AdvanceTurn() int {
	nextIndex := combatState.findNextActiveIndex(combatState.CurrentTurnIndex + 1)
	combatState.CurrentTurnIndex = nextIndex

	logging.Debugf(logging.CategoryGameState,
		"Combat turn advanced to party member %d", combatState.CurrentTurnIndex)

	return combatState.CurrentTurnIndex
}

// findNextActiveIndex finds the next active party member starting from startIndex,
// wrapping around if necessary. Returns the index of the next active member.
func (combatState *CombatState) findNextActiveIndex(startIndex int) int {
	for i := range party_state.MaxActiveParty {
		idx := (startIndex + i) % party_state.MaxActiveParty
		if combatState.PartyActive[idx] {
			return idx
		}
	}
	// No active members (should not happen in normal gameplay)
	return 0
}

// --- Solo Move Mode Methods ---

// SetSoloMoveCharacter enables solo mode for a specific character.
// In solo mode, only this character moves; others skip their turns.
func (combatState *CombatState) SetSoloMoveCharacter(index int) {
	combatState.SoloMoveMode = true
	combatState.SoloMoveCharacterIndex = index
}

// ClearSoloMoveMode disables solo mode, allowing all party members to move.
func (combatState *CombatState) ClearSoloMoveMode() {
	combatState.SoloMoveMode = false
}

// IsSoloMoveActive returns whether solo move mode is currently active.
func (combatState *CombatState) IsSoloMoveActive() bool {
	return combatState.SoloMoveMode
}

// GetSoloMoveCharacter returns the index of the solo move character.
// Only valid when IsSoloMoveActive() returns true.
func (combatState *CombatState) GetSoloMoveCharacter() int {
	return combatState.SoloMoveCharacterIndex
}

// GetActiveCombatantIndex returns the party member whose queue turn is current.
// Solo selection controls future input eligibility, never present action authority.
func (combatState *CombatState) GetActiveCombatantIndex() int {
	return combatState.CurrentTurnIndex
}

// GetCurrentTurnIdentity returns the exact queue member currently authorized to act.
func (combatState *CombatState) GetCurrentTurnIdentity() CombatTurnIdentity {
	if !combatState.currentTurnIdentitySet {
		if combatState.ActiveMonsterIndex != NoActiveMonster {
			return CombatTurnIdentity{IsPartyMember: false, EntityIndex: combatState.ActiveMonsterIndex}
		}

		return CombatTurnIdentity{IsPartyMember: true, EntityIndex: combatState.CurrentTurnIndex}
	}

	return combatState.CurrentTurnIdentity
}

// GetControlledMonster returns the party-loyal monster currently receiving player input.
func (combatState *CombatState) GetControlledMonster() (*CombatMonster, int) {
	identity := combatState.CurrentTurnIdentity
	if identity.IsPartyMember {
		return nil, NoActiveMonster
	}
	monster := combatState.MonsterManager.GetMonster(identity.EntityIndex)
	if monster == nil || !monster.IsActive() || monster.IsDead() || !monster.IsPartyLoyal() {
		return nil, NoActiveMonster
	}

	return monster, identity.EntityIndex
}

// IsPartyTurnAutoPass reports whether this party turn is being skipped while
// the DEX queue advances toward the selected solo character.
func (combatState *CombatState) IsPartyTurnAutoPass(pcIndex int) bool {
	return combatState.IsSoloMoveActive() && pcIndex != combatState.GetSoloMoveCharacter()
}

// AddMonsterToQueue registers a monster with both the MonsterManager and TurnQueue.
// This must be used instead of calling MonsterManager.AddMonster directly when the
// turn queue is active, to keep both systems synchronized.
func (combatState *CombatState) AddMonsterToQueue(monster *CombatMonster) int {
	if monster == nil {
		panic("cannot register nil combat monster")
	}
	if monster.combatantID != 0 {
		panic("combat monster is already registered")
	}
	if combatState.TurnQueue != nil && (!combatState.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) ||
		!combatState.hasFreeOriginalObjectSlot()) {
		panic("cannot register combat monster without queue and object capacity")
	}
	monster.combatantID = combatState.nextCombatantID
	combatState.nextCombatantID++
	index := combatState.MonsterManager.AddMonster(monster)
	if combatState.TurnQueue != nil {
		combatState.TurnQueue.AddCombatant(monster, index)
		if !combatState.registerCombatantObject(CombatTurnIdentity{EntityIndex: index}) {
			panic("AddMonsterToQueue: preflighted original object slot is unavailable")
		}
	}

	return index
}

func (combatState *CombatState) addClonedMonsterToQueue(
	source CombatTurnIdentity,
	monster *CombatMonster,
) int {
	if monster == nil || monster.combatantID != 0 {
		panic("cannot register invalid combat monster clone")
	}
	if !combatState.TurnQueue.hasFreeSourceSlot(0) || !combatState.hasFreeOriginalObjectSlot() {
		panic("cannot register combat monster clone without queue and object capacity")
	}
	monster.combatantID = combatState.nextCombatantID
	combatState.nextCombatantID++
	index := combatState.MonsterManager.AddMonster(monster)
	combatState.TurnQueue.AddClonedCombatant(monster, index, 0, source)
	if !combatState.registerCombatantObject(CombatTurnIdentity{EntityIndex: index}) {
		panic("addClonedMonsterToQueue: preflighted original object slot is unavailable")
	}

	return index
}

// Capture removed identity before releasing storage; GameState decides whether
// this replacement is an authoritative controller-release outcome.
func (combatState *CombatState) replaceCombatantWithMonster(identity CombatTurnIdentity, monster *CombatMonster) (int, CombatantID) {
	var controller CombatantID
	entry := combatState.TurnQueue.GetEntryByIdentity(identity)
	if entry == nil {
		panic("CombatState.ReplaceCombatantWithMonster: missing queue entry")
	}
	entry.status = turnQueueFled
	entry.sourceSlotReleased = true
	entry.originalObjectSlot = 0
	if identity.IsPartyMember {
		if identity.Slot == 0 {
			combatState.setCombatPCPossessed(identity, false)
			combatState.PartyActive[identity.EntityIndex] = false
		} else {
			combatState.removePCClone(identity)
		}
	} else {
		replaced := combatState.MonsterManager.GetMonster(identity.EntityIndex)
		if replaced != nil {
			controller = replaced.CombatantID()
			replaced.SetActive(false)
		}
	}

	return combatState.AddMonsterToQueue(monster), controller
}

func (combatState *CombatState) canReplaceCombatantWithMonster(identity CombatTurnIdentity) bool {
	entry := combatState.TurnQueue.GetEntryByIdentity(identity)
	if entry == nil || entry.status != turnQueueActive {
		return false
	}

	return entry.sourceSlot >= sourceMonsterSlotStart ||
		combatState.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart)
}

// --- GameState combat methods ---

// IsInCombat returns true if the player is currently in combat mode
func (g *GameState) IsInCombat() bool {
	return g.MapState.PlayerLocation.Location == references.CombatLocation
}

// getActiveCombatantPosition returns the current position of the active combatant.
// Panics if called outside of combat (CombatScene == nil).
func (g *GameState) getActiveCombatantPosition() references.Position {
	if g.CombatScene == nil {
		panic("getActiveCombatantPosition called outside of combat")
	}

	if monster, _ := g.CombatScene.GetControlledMonster(); monster != nil {
		return monster.Position()
	}

	return g.CombatScene.GetCombatantPosition(g.CombatScene.GetCurrentTurnIdentity())
}

// CombatState field - added to GameState struct via game_state_combat.go
// Access via g.CombatScene

// commitCombatTransition installs a fully prepared combat state. Callers must
// complete all recoverable validation before invoking it.
func (g *GameState) commitCombatTransition(
	combatState *CombatState,
	territory references.CombatMapTerritory,
) {
	if !combatState.specialRingsInitialized {
		panic("combat transition committed before special ring initialization")
	}
	g.reconcileWaterfallCombatEntry(combatState)
	g.StopAutomaticSailingForCombat()
	g.resetHarpsichordMelody()
	g.combatEncounterSequence++
	combatState.EncounterID = CombatEncounterID(g.combatEncounterSequence)
	combatState.PreCombatPlayerLocation = g.MapState.SavePlayerLocation()
	combatState.preCombatActivePlayerIndex = g.GetActivePlayerIndex()
	if territory == references.DungeonCombatTerritory {
		combatState.ExitContext = CombatExitDungeonEncounter
	}
	g.CombatScene = combatState

	// Create LayeredMap for combat map
	g.MapState.LayeredMaps.ResetAndCreateCombatMap(
		combatState.CombatMapRef,
		g.GameReferences.TileReferences,
		g.MapState.XTilesVisibleOnGameScreen,
		g.MapState.YTilesVisibleOnGameScreen,
	)

	// Set location to CombatLocation (virtual location for combat)
	g.MapState.PlayerLocation.Location = references.CombatLocation
	g.MapState.PlayerLocation.Floor = 0 // Combat maps are always floor 0

	// Set player position to the CENTER of the combat map (fixed camera)
	// The camera stays fixed at (5,5) - actors move relative to this
	g.MapState.PlayerLocation.Position = references.Position{X: combatMapCenterCoord, Y: combatMapCenterCoord}

	// Overworld selection and combat solo selection have different meanings.
	// Combat starts in round-robin mode with no explicit party member selected.
	g.ClearActivePlayer()
	g.SystemCallbacks.Screen.UpdateDisplay()
}

const combatVictoryExitFadeMs int64 = 750

// TransitionFromCombatMap clears combat mode and returns to the pre-combat map
func (g *GameState) TransitionFromCombatMap() {
	if g.CombatScene == nil {
		logging.Warnf(logging.CategoryGameState,
			"TransitionFromCombatMap called with no active combat")

		return
	}
	if g.CombatScene.Victory {
		g.SystemCallbacks.Audio.Stop(SoundCombatVictory).FadeOut(combatVictoryExitFadeMs).Now()
	}

	// Charm is represented on PartyState for status UI, but is strictly encounter-local.
	g.clearAllCombatPossession()

	// Restore pre-combat location and apply the shared dungeon exit route.
	exitContext := g.CombatScene.ExitContext
	exitRoute := g.CombatScene.ExitRoute
	dungeonWandererOrigin := g.CombatScene.DungeonWandererOrigin
	campingOrigin := g.CombatScene.CampingOrigin
	doomEntranceOrigin := g.CombatScene.DoomEntranceOrigin
	g.MapState.RestorePlayerLocation(g.CombatScene.PreCombatPlayerLocation)
	if dungeonWandererOrigin == nil && campingOrigin == nil && doomEntranceOrigin == nil {
		g.applyCombatExitRoute(exitContext, exitRoute)
	}

	// Clear combat state
	combatMapName := g.CombatScene.CombatMapRef.MapName
	activePlayerIndex := g.CombatScene.preCombatActivePlayerIndex
	if !g.combatRecoveryReviewDeferred {
		g.cancelOutdoorActorPhase()
	}
	g.CombatScene = nil
	// Restore after clearing combat so the stats callback sees exploration state.
	g.restorePreCombatActivePlayer(activePlayerIndex)
	g.SystemCallbacks.Screen.UpdateDisplay()

	logging.Debugf(logging.CategoryGameState,
		"Transitioned from combat map %q back to normal view", combatMapName)
}

// EndCombat restores the pre-combat map and resolves normal-play encounter origin state.
func (g *GameState) EndCombat(reason CombatEndReason) {
	if g.CombatScene == nil {
		logging.Warnf(logging.CategoryGameState, "EndCombat called with no active combat")

		return
	}
	if g.combatRecoveryReviewDeferred {
		panic("EndCombat called while combat recovery review is already deferred")
	}
	g.combatRecoveryReviewDeferred = true
	defer func() {
		g.combatRecoveryReviewDeferred = false
	}()

	origin := g.CombatScene.OverworldOrigin
	encounterID := g.CombatScene.EncounterID
	smallMapOrigin := g.CombatScene.SmallMapOrigin
	dungeonRoomOrigin := g.CombatScene.DungeonRoomOrigin
	dungeonWandererOrigin := g.CombatScene.DungeonWandererOrigin
	campingOrigin := g.CombatScene.CampingOrigin
	doomEntranceOrigin := g.CombatScene.DoomEntranceOrigin
	exitRoute := g.CombatScene.ExitRoute
	conclusion := CombatConclusion{
		Reason: reason, Special: CombatSpecialConclusionNone,
		Collected: append([]CombatCollectedItem(nil), g.CombatScene.collectedItems...),
	}
	if reason == CombatVictory && exitRoute == CombatExitDoomMirror && g.CombatScene.isVerifiedDoomFinalRoom() {
		conclusion.Special = CombatSpecialConclusionDoomMirror
	}
	if reason == CombatVictory && conclusion.Special == CombatSpecialConclusionNone {
		g.resolveDungeonRoomVictory(dungeonRoomOrigin)
	}
	g.TransitionFromCombatMap()
	if origin != nil {
		g.resolveOverworldEncounterOrigin(origin, reason)
	}
	if smallMapOrigin != nil {
		g.resolveSmallMapEncounterOrigin(smallMapOrigin)
	}
	if conclusion.Special == CombatSpecialConclusionNone {
		g.resumeDungeonRoomContinuation(dungeonRoomOrigin)
		g.resumeDungeonWandererEncounter(dungeonWandererOrigin, exitRoute)
		g.resolveCampingCombatOrigin(campingOrigin)
		g.resolveDoomEntranceEncounterOrigin(doomEntranceOrigin)
	} else {
		g.clearDungeonRoomContinuationForFinale(dungeonRoomOrigin)
	}
	g.ProcessPendingProvisionHours()
	if conclusion.Special != CombatSpecialConclusionNone {
		if g.PartyState.IsAllPartyDead() {
			conclusion.Special = CombatSpecialConclusionNone
		} else {
			g.finaleHandoffPending = true
		}
	}
	g.SystemCallbacks.Combat.PublishConclusion(conclusion)
	g.finishOutdoorCombatReturn(origin, encounterID, reason)
	// Review the ending encounter only after restoration. Lower outdoor slots
	// may create their own fatal effects, whose workflows own recovery; resume
	// them after this review so their death presentation is not duplicated.
	g.combatRecoveryReviewDeferred = false
	g.checkForPartyWipe()
	g.resumeOutdoorActorPhase()
}

// isPartyMemberAtPosition checks if any active party member (other than excludeIndex)
// occupies the given position on the combat map.
func (combatState *CombatState) isPartyMemberAtPosition(
	pos references.Position, excludeIndex int,
) bool {
	for i := range party_state.MaxActiveParty {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: i}
		if i == excludeIndex || !combatState.PartyActive[i] || combatState.isCombatPCUnderground(identity) {
			continue
		}
		if combatState.PartyPositions[i] == pos {
			return true
		}
	}
	for _, clone := range combatState.partyClones {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: clone.partyIndex, Slot: clone.slot}
		if combatState.isCombatPCUnderground(identity) {
			continue
		}
		if clone.position == pos {
			return true
		}
	}

	return false
}

func (combatState *CombatState) isCombatPCAtPositionExcept(
	position references.Position,
	excluded CombatTurnIdentity,
) bool {
	for partyIndex, active := range combatState.PartyActive {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: partyIndex}
		if active && identity != excluded && !combatState.isCombatPCUnderground(identity) &&
			combatState.PartyPositions[partyIndex] == position {
			return true
		}
	}
	for _, clone := range combatState.partyClones {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: clone.partyIndex, Slot: clone.slot}
		if identity != excluded && !combatState.isCombatPCUnderground(identity) && clone.position == position {
			return true
		}
	}

	return false
}

func (combatState *CombatState) combatPCAtPosition(position references.Position) (CombatTurnIdentity, bool) {
	for sourceSlot := sourceCombatQueueSlots - 1; sourceSlot >= 0; sourceSlot-- {
		entry := combatState.TurnQueue.getActiveEntryAtSourceSlot(sourceSlot)
		if entry == nil || !entry.Combatant.IsPC() {
			continue
		}
		identity := turnQueueIdentity(entry)
		if combatState.GetCombatantPosition(identity) == position {
			return identity, true
		}
	}

	return CombatTurnIdentity{}, false
}

// MoveCombatCharacter moves the current or solo-selected character.
// Return CombatMoveBlocked for invalid terrain, occupancy or context;
// CombatMoveMoved for a legal interior step; CombatMoveFled for a legal
// step beyond the 11-by-11 battlefield edge. ref:COMBAT-0003
func (g *GameState) MoveCombatCharacter(direction references.Direction) CombatMoveResult {
	if g.CombatScene == nil {
		return CombatMoveBlocked
	}
	if monster, monsterIndex := g.CombatScene.GetControlledMonster(); monster != nil {
		return g.moveControlledCombatMonster(monster, monsterIndex, direction)
	}

	turnIdentity := g.CombatScene.GetCurrentTurnIdentity()
	if turnIdentity.IsPartyMember && g.CombatScene.isCombatPCUnderground(turnIdentity) {
		return CombatMoveBlocked
	}
	// Legacy callers may select a party member by CurrentTurnIndex directly.
	// Preserve that behavior for normal entries while clone turns retain identity.
	if turnIdentity.IsPartyMember && turnIdentity.Slot == 0 && turnIdentity.EntityIndex != g.CombatScene.CurrentTurnIndex {
		turnIdentity.EntityIndex = g.CombatScene.CurrentTurnIndex
	}
	turnIdx := turnIdentity.EntityIndex
	currentPos := g.CombatScene.GetCombatantPosition(turnIdentity)
	newPos := direction.GetNewPositionInDirection(&currentPos)

	if newPos == nil {
		return CombatMoveBlocked
	}

	// A legal step beyond the 11-by-11 battlefield edge flees. ref:COMBAT-0003
	if newPos.X < 0 || newPos.X >= references.XCombatMapTiles ||
		newPos.Y < 0 || newPos.Y >= references.YCombatMapTiles {
		route := combatExitRouteFromDirection(direction)
		if !g.fleePCFromCombat(turnIdentity, route) {
			return CombatMoveBlocked
		}
		// playerflee() returns to playermove(), whose ordinary action boundary
		// still runs endaction() field expiration after removing the actor.
		g.resolveCombatControlledEndAction(turnIdentity)

		return CombatMoveFled
	}

	if !g.isCombatPositionValidForPC(newPos, turnIdentity) {
		return CombatMoveBlocked
	}

	// Move the character
	g.CombatScene.setCombatantPosition(turnIdentity, *newPos)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventMove,
		Attacker: CombatFeedbackEntity{
			IsPartyMember: true,
			Index:         turnIdx,
			Slot:          turnIdentity.Slot,
			Position:      *newPos,
		},
		From: currentPos,
		To:   *newPos,
	})
	g.CheckTrap(newPos)
	g.processCombatPCSpecialRing(turnIdentity)
	g.resolveCombatFieldExpiration()
	g.processDurationEffects()
	absorbed := g.resolveDoomMirrorAbsorption()
	// Original combat scheduler calls standing-field effects after a completed player action.
	// Movement is an action, so harmful fields resolve after entering the tile;
	// protection fields are rejected by isCombatPositionValid before this point.
	// See ref:COMBAT-0002.
	if !absorbed {
		g.resolveCombatPCStandingEffects(turnIdentity)
	}

	// NOTE: Camera stays fixed at center (5,5) - do NOT update PlayerLocation.Position

	logging.Debugf(logging.CategoryGameState,
		"Combat character %d moved %s to (%d,%d)",
		turnIdx, direction.GetDirectionCompassName(), newPos.X, newPos.Y)

	return CombatMoveMoved
}

// fleePCFromCombat removes a PC from combat when they walk off the map edge.
// Replicates original battlefield escape from original combat action rules:
//   - Mark PC inactive in PartyActive
//   - Mark fled in TurnQueue
//   - Show "Escape!" (if monsters remain) or "Leave!" (if no monsters)
//
// See ref:COMBAT-0002.
func (g *GameState) fleePCFromCombat(identity CombatTurnIdentity, route CombatExitRoute) bool {
	if !g.canFleeCombatIndividually() {
		return false
	}
	if !g.CombatScene.establishExitRoute(route) {
		g.SystemCallbacks.Message.AddRowStr("All must use the same exit!")

		return false
	}
	g.departPCFromCombat(identity)

	return true
}

func (g *GameState) canFleeCombatIndividually() bool {
	if g.PartyVehicle.GetVehicleDetails().VehicleType != references.FrigateVehicle {
		return true
	}
	g.SystemCallbacks.Message.AddRowStr("Stay with ship!")

	return false
}

func (g *GameState) departPCFromCombat(identity CombatTurnIdentity) {
	if !g.CombatScene.isCombatPCActive(identity) {
		return
	}
	position := g.CombatScene.GetCombatantPosition(identity)
	pcIndex := identity.EntityIndex
	g.CombatScene.setCombatPCPossessed(identity, false)
	if identity.Slot == 0 {
		g.CombatScene.PartyActive[pcIndex] = false
	} else {
		g.CombatScene.removePCClone(identity)
	}
	g.CombatScene.TurnQueue.markFledIdentity(identity)
	g.clearInactiveCombatPCSelection(pcIndex)

	// Check remaining combatants (original remaining-allegiance count)
	// See ref:COMBAT-0002.
	monsterCount := g.CombatScene.MonsterManager.CountActiveHostile()

	pcName := g.PartyState.GetCharacter(pcIndex).GetNameAsString()
	if monsterCount == 0 {
		g.SystemCallbacks.Message.AddRowStr(pcName + " - Leave!")
	} else {
		g.SystemCallbacks.Message.AddRowStr(pcName + " - Escape!")
	}

	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventFled,
		Attacker: CombatFeedbackEntity{
			IsPartyMember: true,
			Index:         pcIndex,
			Slot:          identity.Slot,
			Position:      position,
		},
	})

	logging.Debugf(logging.CategoryGameState,
		"PC %d/%d (%s) departed combat, route=%d monstersRemaining=%d",
		pcIndex, identity.Slot, pcName, g.CombatScene.ExitRoute, monsterCount)
}

// isCombatPositionValid checks whether a position is valid for combat movement:
// within bounds, passable terrain, and not occupied by another party member.
func (g *GameState) isCombatPositionValid(
	pos *references.Position, excludeMemberIndex int,
) bool {
	// Check bounds (combat maps are 11x11)
	if pos.X < 0 || pos.X >= references.XCombatMapTiles ||
		pos.Y < 0 || pos.Y >= references.YCombatMapTiles {
		return false
	}

	// Fixed-PLAYER placement is independent of the boarded overworld vehicle.
	terrain := g.GetCurrentLayeredMap().GetTopTile(pos)

	// Check terrain, party member collision, and monster collision.
	return g.combatTerrainAllowsDestination(*pos) &&
		combatTerrainAllowsOriginalShape(originalCombatPlayerShape, terrain) &&
		!g.CombatScene.isPartyMemberAtPosition(*pos, excludeMemberIndex) &&
		!g.CombatScene.MonsterManager.IsPositionOccupied(*pos) &&
		!g.CombatScene.HasBlockingObjectAt(*pos) &&
		!g.CombatScene.HasProtectionFieldAt(*pos)
}

func (g *GameState) isCombatPositionValidForPC(
	position *references.Position,
	identity CombatTurnIdentity,
) bool {
	if position.X < 0 || position.X >= references.XCombatMapTiles ||
		position.Y < 0 || position.Y >= references.YCombatMapTiles {
		return false
	}
	terrain := g.GetCurrentLayeredMap().GetTopTile(position)
	shape := originalCombatPlayerShape
	if g.CombatScene.IsCombatPCRat(identity) {
		shape = originalCombatMonsterShape(references.MonsterRat)
	}

	return g.combatTerrainAllowsDestination(*position) &&
		combatTerrainAllowsOriginalShape(shape, terrain) &&
		!g.CombatScene.isCombatPCAtPositionExcept(*position, identity) &&
		!g.CombatScene.MonsterManager.IsPositionOccupied(*position) &&
		!g.CombatScene.HasBlockingObjectAt(*position) &&
		!g.CombatScene.HasProtectionFieldAt(*position)
}

func (combatState *CombatState) countActiveCombatPCs() int {
	count := 0
	for sourceSlot := range sourceCombatQueueSlots {
		entry := combatState.TurnQueue.getActiveEntryAtSourceSlot(sourceSlot)
		if entry != nil && entry.Combatant.IsPC() && combatState.isCombatPCActive(turnQueueIdentity(entry)) {
			count++
		}
	}

	return count
}

// checkCombatEndConditions checks if combat should end after an action.
// Replicates original remaining-allegiance count from original combat action rules and the victory/defeat
// checks in countdown scheduler at original combat scheduler.
//
// Returns CombatEndReason:
//   - CombatNotEnded: combat continues (including when Victory is already set)
//   - CombatVictory: all monsters just died, PCs remain — sets Victory flag
//   - CombatDefeat: all PCs dead/fled, monsters remain — "BATTLE IS LOST!"
//   - CombatEvacuated: all PCs fled and all monsters also gone
//
// See ref:COMBAT-0002.
func (g *GameState) checkCombatEndConditions() CombatEndReason {
	if g.CombatScene == nil {
		return CombatNotEnded
	}

	pcCount, monsterCount := g.countActiveCombatLoyalties()
	if pcCount == 0 && g.recoverOnePossessedPC() {
		return CombatNotEnded
	}
	if g.CombatScene.DebugOutcomeSuppressed && monsterCount == 0 {
		return CombatNotEnded
	}

	// Victory remains active for looting until Escape or the last PC physically leaves.
	if g.CombatScene.Victory {
		if pcCount == 0 {
			g.EndCombat(CombatVictory)

			return CombatVictory
		}

		return CombatNotEnded
	}

	if monsterCount == 0 && pcCount > 0 {
		// All hostile monsters dead — victory
		g.CombatScene.Victory = true
		logging.Infof(logging.CategoryGameState,
			"Combat victory: all monsters defeated, %d PCs remain", pcCount)
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind: CombatEventVictory,
		})

		return CombatVictory
	}

	if pcCount == 0 && monsterCount > 0 {
		// All PCs dead or fled with monsters remaining — defeat
		g.SystemCallbacks.Message.AddRowStr("BATTLE IS LOST!")
		logging.Infof(logging.CategoryGameState,
			"Combat defeat: no PCs remain, %d monsters left", monsterCount)
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind: CombatEventDefeat,
		})
		g.EndCombat(CombatDefeat)

		return CombatDefeat
	}

	if pcCount == 0 && monsterCount == 0 {
		if g.CombatScene.ExitRoute == CombatExitDoomMirror && g.CombatScene.isVerifiedDoomFinalRoom() {
			logging.Infof(logging.CategoryGameState,
				"Combat victory: final Doom mirror absorption completed")
			g.EndCombat(CombatVictory)

			return CombatVictory
		}
		// Everyone gone (all PCs fled, no monsters)
		logging.Infof(logging.CategoryGameState,
			"Combat evacuated: no combatants remain on either side")
		g.EndCombat(CombatEvacuated)

		return CombatEvacuated
	}

	return CombatNotEnded
}

func (g *GameState) countActiveCombatLoyalties() (int, int) {
	combatState := g.CombatScene
	partyLoyal := 0
	monsterLoyal := 0
	for sourceSlot := range sourceCombatQueueSlots {
		entry := combatState.TurnQueue.getActiveEntryAtSourceSlot(sourceSlot)
		if entry == nil {
			continue
		}
		identity := turnQueueIdentity(entry)
		if identity.IsPartyMember && !combatState.isCombatPCActive(identity) {
			continue
		}
		if !identity.IsPartyMember {
			monster := combatState.MonsterManager.GetMonster(identity.EntityIndex)
			if monster == nil {
				continue
			}
		}
		if g.combatIdentityLoyalty(identity) == CombatMonsterLoyaltyParty {
			partyLoyal++
		} else {
			monsterLoyal++
		}
	}

	return partyLoyal, monsterLoyal
}

// combatPCTurnPrompt names the actor and attacking equipment. Inspect
// the helm and both hands in order, include positive attack ratings, and
// use the bare-hands description when none qualify. ref:EQUIP-0012
func (g *GameState) combatPCTurnPrompt(pcIndex int) string {
	character := g.PartyState.GetCharacter(pcIndex)
	name := character.GetNameAsString()

	// Collect weapon names for items with attack value > 0
	// Original combat scheduler checks helm, weapon[0], weapon[1]
	// See ref:COMBAT-0002.
	slots := []references.EquipmentSlot{
		references.SlotHelm,
		references.SlotWeapon,
		references.SlotShield,
	}

	var weapons []string
	for _, slot := range slots {
		item := character.GetEquippedItem(slot)
		if item != references.NoEquipment && item.GetAttackValue() > 0 {
			weapons = append(weapons, item.String())
		}
	}

	if len(weapons) == 0 {
		return fmt.Sprintf("%s, armed with bare hands:", name) //nolint:perfsprint // combat prompt, not hot path
	}

	// Join with ", " — matches original comma-separated output
	weaponList := weapons[0]
	for i := 1; i < len(weapons); i++ {
		weaponList += ", " + weapons[i] //nolint:perfsprint // combat prompt, not hot path
	}

	return fmt.Sprintf("%s, armed with %s:", name, weaponList)
}

func (g *GameState) combatTurnIsSoloAutoPass(
	identity CombatTurnIdentity,
	loyalty CombatMonsterLoyalty,
) bool {
	if loyalty != CombatMonsterLoyaltyParty || !g.CombatScene.IsSoloMoveActive() {
		return false
	}

	return !identity.IsPartyMember || identity.EntityIndex != g.CombatScene.GetSoloMoveCharacter()
}

func (g *GameState) combatTemporalActionSuppressed(
	identity CombatTurnIdentity,
	loyalty CombatMonsterLoyalty,
) bool {
	if loyalty != CombatMonsterLoyaltyMonster {
		return false
	}
	if g.HasActiveDurationEffect(DurationEffectTimeStop) {
		return true
	}
	if !g.HasActiveDurationEffect(DurationEffectQuickness) || g.RandomIntInRange(0, 1) != 0 {
		return false
	}
	if identity.IsPartyMember {
		g.publishCombatPCConditionEvent(identity, CombatConditionQuicknessSuppressed)
	} else {
		g.publishCombatConditionEvent(identity.EntityIndex, false, CombatConditionQuicknessSuppressed)
	}

	return true
}

func (g *GameState) resolveCombatActorStandingEffects(identity CombatTurnIdentity) {
	if identity.IsPartyMember {
		g.resolveCombatPCStandingEffects(identity)

		return
	}
	monster := g.CombatScene.MonsterManager.GetMonster(identity.EntityIndex)
	g.resolveCombatMonsterFieldEffects(monster, identity.EntityIndex)
}

// AdvanceCombatTurn advances to the next combatant's turn using the dex-based
// priority queue (replicating original combat scheduler countdown scheduler behavior).
//
// The queue is iterated until a PC turn that should receive player input is found:
//   - Hostile turns: execute AI immediately or yield to an exact presentation step
//   - Solo mode non-active PC turns: auto-passed (original combat scheduler)
//   - Active PC turns: set as CurrentTurnIndex and return
//   - nil from queue: all combatants dead/fled — combat should end
//
// See ref:COMBAT-0002.
func (g *GameState) AdvanceCombatTurn() {
	if g.CombatScene == nil {
		return
	}
	if g.CombatScene.pendingPresentationStep != nil {
		return
	}
	// Direct callers are the headless contract. GameScene starts presented turns
	// explicitly through BeginCombatTurns.
	if !g.CombatScene.turnsStarted {
		g.CombatScene.turnsStarted = true
	}

	// Check if the previous action ended combat (kill, flee, etc.).
	// Replicates original remaining-allegiance count check after every action in countdown scheduler.
	// Victory sets the flag but does NOT stop the turn loop; the player may loot before Escape.
	// Defeat/evacuated immediately transitions out.
	// See ref:COMBAT-0002.
	if endReason := g.checkCombatEndConditions(); g.CombatScene == nil ||
		endReason == CombatDefeat || endReason == CombatEvacuated {
		return
	}

	queue := g.CombatScene.TurnQueue
	if queue == nil {
		// Fallback to old round-robin if queue not initialized (shouldn't happen)
		newIndex := g.CombatScene.AdvanceTurn()
		g.SetActivePlayer(newIndex)

		return
	}

	for {
		g.clearUnreachableCombatSoloSelection()
		// Clear an actor’s remembered attacker after its scheduled action,
		// including skipped actions. A returned actor is cleared when its scheduler
		// next advances. ref:EQUIP-0016
		g.clearRememberedMeleeEngager(g.CombatScene.GetCurrentTurnIdentity())
		entry := queue.AdvanceToNextActor()
		if entry == nil {
			// All combatants dead or fled — force end-condition check
			g.checkCombatEndConditions()

			return
		}
		g.advanceCombatQueueTime()
		g.CombatScene.CurrentTurnIdentity = CombatTurnIdentity{
			IsPartyMember: entry.Combatant.IsPC(),
			EntityIndex:   entry.EntityIndex,
			Slot:          entry.Slot,
		}
		g.CombatScene.currentTurnIdentitySet = true
		identity := turnQueueIdentity(entry)
		loyaltyAtDispatch := g.combatIdentityLoyalty(identity)
		g.CombatScene.dispatchCombatPossessionTurn(identity)

		// Player action applies the active-player filter before Chaos
		// Sword interception. Only currently player-loyal actors participate;
		// hostile PCs still execute AI, while loyal monsters pass in solo mode.
		// See ref:COMBAT-0002.
		if g.combatTurnIsSoloAutoPass(identity, loyaltyAtDispatch) {
			if identity.IsPartyMember {
				g.CombatScene.CurrentTurnIndex = entry.EntityIndex
			}
			g.resolveCombatControlledEndAction(identity)
			g.resolveCombatActorStandingEffects(identity)
			logging.Debugf(logging.CategoryGameState,
				"AdvanceCombatTurn: solo mode auto-pass actor %+v (active=%d)",
				identity, g.CombatScene.GetSoloMoveCharacter())
			// Player action immediately ends non-selected friendly turns.
			// Keep scanning synchronously; hostile and actionable turns still yield
			// through their normal presentation paths.
			// See ref:COMBAT-0002.
			if g.combatEndedAfterCompletedTurn() {
				return
			}

			continue
		}

		var monster *CombatMonster
		if !identity.IsPartyMember {
			monster = g.CombatScene.MonsterManager.GetMonster(entry.EntityIndex)
		}
		if g.combatTemporalActionSuppressed(identity, loyaltyAtDispatch) {
			if g.dispatchSuppressedCombatTurn(entry, identity) {
				return
			}

			continue
		}

		turnStart := g.processCombatTurnStart(entry)
		if turnStart == combatTurnSkipped {
			if g.dispatchSkippedCombatTurn(entry, identity, loyaltyAtDispatch) {
				return
			}

			continue
		}

		loyalty := g.combatIdentityLoyalty(identity)
		// Party-loyal monsters receive player input instead of hostile monster AI.
		if !identity.IsPartyMember && monster != nil && loyalty == CombatMonsterLoyaltyParty {
			g.CombatScene.ActiveMonsterIndex = entry.EntityIndex
			g.CombatScene.ActiveTargetPCIndex = NoTargetPC
			g.ClearActivePlayer()
			name := monster.MonsterType().Name()
			if !g.CombatTerrainAllowsObjectDisplay(monster.Position()) {
				name = "Unknown"
			}
			g.SystemCallbacks.Message.AddRowStr(name + ":")
			g.SystemCallbacks.Message.ShowCommandPrompt("")

			return
		}

		// Monster-loyal PCs retain their queue slot but execute monster-side AI.
		if identity.IsPartyMember && loyalty == CombatMonsterLoyaltyMonster {
			if g.dispatchPossessedCombatTurn(entry, identity, loyaltyAtDispatch) {
				return
			}

			continue
		}

		// Hostile monster turn — delegate to scheduler for delayed execution
		if !identity.IsPartyMember && loyalty == CombatMonsterLoyaltyMonster {
			if g.dispatchHostileMonsterCombatTurn(entry, identity) {
				return
			}

			continue
		}

		// This PC gets player input
		g.CombatScene.CurrentTurnIndex = entry.EntityIndex
		g.CombatScene.ActiveMonsterIndex = NoActiveMonster
		g.CombatScene.ActiveTargetPCIndex = NoTargetPC
		g.SetActivePlayer(entry.EntityIndex)

		// Show turn prompt: "Name, armed with weapons:"
		// Combines the actor name and weapon description for the combat turn prompt.
		// See ref:COMBAT-0002.
		prompt := g.combatPCTurnPrompt(entry.EntityIndex)
		g.SystemCallbacks.Message.AddRowStr(prompt)
		g.SystemCallbacks.Message.ShowCommandPrompt("")

		logging.Debugf(logging.CategoryGameState,
			"AdvanceCombatTurn: PC %d's turn at (%d,%d)",
			entry.EntityIndex,
			g.CombatScene.GetPartyMemberPosition(entry.EntityIndex).X,
			g.CombatScene.GetPartyMemberPosition(entry.EntityIndex).Y)

		return
	}
}

// Dispatch helpers return true when presentation or combat conclusion stops the
// DEX scan. False means this exact turn completed and scanning may continue.
func (g *GameState) dispatchSuppressedCombatTurn(entry *TurnQueueEntry, identity CombatTurnIdentity) bool {
	if identity.IsPartyMember {
		g.CombatScene.CurrentTurnIndex = identity.EntityIndex
		g.ClearActivePlayer()
	} else {
		g.CombatScene.ActiveMonsterIndex = identity.EntityIndex
	}
	g.clearActiveCombatTarget()
	if g.CombatScene.presentTurns {
		g.scheduleCombatPresentationStep(
			CombatPresentationMonsterTurn,
			combatPresentationIdleMonster,
			identity,
			identity.EntityIndex,
			CombatTurnIdentity{},
			false,
			entry,
		)

		return true
	}
	g.resolveCombatActorStandingEffects(identity)
	g.CombatScene.ActiveMonsterIndex = NoActiveMonster

	return g.combatEndedAfterCompletedTurn()
}

func (g *GameState) dispatchSkippedCombatTurn(
	entry *TurnQueueEntry,
	identity CombatTurnIdentity,
	loyaltyAtDispatch CombatMonsterLoyalty,
) bool {
	if identity.IsPartyMember {
		g.CombatScene.CurrentTurnIndex = entry.EntityIndex
		g.ClearActivePlayer()
	}
	// The recovered sleeping/swallowed/initial-Chaos playermove tails read
	// an uninitialized command byte. Keep Redux's established deterministic
	// player-loyal dispatch policy; this is not defined original behavior.
	if loyaltyAtDispatch == CombatMonsterLoyaltyParty {
		// Countdown scheduler calls standing-field effects even when player action
		// consumes the turn for sleep or Corpser captivity. Field expiration remains
		// a tactical action completion concern, separate from this path. COMBAT-0002.
		g.resolveCombatActorStandingEffects(identity)
		g.processDurationEffects()
		if identity.IsPartyMember && g.CombatScene.presentTurns {
			g.scheduleCombatPresentationStep(
				CombatPresentationAutoPass,
				combatPresentationAdvance,
				identity,
				NoActiveMonster,
				CombatTurnIdentity{},
				false,
				nil,
			)

			return true
		}

		return g.combatEndedAfterCompletedTurn()
	}

	if !identity.IsPartyMember {
		g.CombatScene.ActiveMonsterIndex = entry.EntityIndex
	}
	g.clearActiveCombatTarget()
	if g.CombatScene.presentTurns {
		g.scheduleCombatPresentationStep(
			CombatPresentationMonsterTurn,
			combatPresentationIdleMonster,
			identity,
			entry.EntityIndex,
			CombatTurnIdentity{},
			false,
			entry,
		)

		return true
	}

	g.resolveCombatActorStandingEffects(identity)
	g.CombatScene.ActiveMonsterIndex = NoActiveMonster

	return g.combatEndedAfterCompletedTurn()
}

func (g *GameState) dispatchPossessedCombatTurn(
	entry *TurnQueueEntry,
	identity CombatTurnIdentity,
	loyaltyAtDispatch CombatMonsterLoyalty,
) bool {
	plan := g.planPossessedPCTurn(identity)
	g.setActiveCombatTarget(plan.Target, plan.HasTarget)
	g.ClearActivePlayer()
	if g.CombatScene.presentTurns {
		g.scheduleCombatPresentationStep(
			CombatPresentationMonsterTurn,
			combatPresentationPossessedPC,
			identity,
			NoActiveMonster,
			plan.Target,
			plan.HasTarget,
			entry,
		)
		// The initial Chaos interception entered via player-side dispatch;
		// preserve the same deterministic tail as immediate execution.
		g.CombatScene.pendingPresentationStep.completeUndefinedPlayerTail = loyaltyAtDispatch == CombatMonsterLoyaltyParty

		return true
	}
	g.executePossessedPCTurn(identity)
	g.resolveCombatPCStandingEffects(identity)
	if loyaltyAtDispatch == CombatMonsterLoyaltyParty {
		// Initial Chaos interception has an undefined original endaction tail;
		// retain Redux's deterministic duration handling for that first turn.
		g.processDurationEffects()
	}
	g.clearActiveCombatTarget()

	return g.combatEndedAfterCompletedTurn()
}

func (g *GameState) dispatchHostileMonsterCombatTurn(entry *TurnQueueEntry, identity CombatTurnIdentity) bool {
	g.CombatScene.ActiveMonsterIndex = entry.EntityIndex

	// Pre-compute what the monster will do so the UI can highlight the target.
	plan := g.planMonsterTurn(entry)
	g.setActiveCombatTarget(plan.Target, plan.HasTarget)
	if g.CombatScene.presentTurns {
		g.scheduleCombatPresentationStep(
			CombatPresentationMonsterTurn,
			combatPresentationMonsterAI,
			identity,
			entry.EntityIndex,
			plan.Target,
			plan.HasTarget,
			entry,
		)

		return true
	}

	// Headless turns execute immediately and continue the same DEX scan.
	result := g.executeMonsterTurn(entry, plan)
	if result.ProjectileLaunch != nil {
		g.ResolvePendingMonsterMissile()
	}
	if g.CombatScene != nil {
		g.CombatScene.ActiveMonsterIndex = NoActiveMonster
		g.CombatScene.ActiveTargetPCIndex = NoTargetPC
		g.CombatScene.ActiveTargetIdentitySet = false
	}

	return g.combatEndedAfterCompletedTurn()
}

func (g *GameState) combatEndedAfterCompletedTurn() bool {
	if g.CombatScene != nil {
		g.completeCombatPossessionTurn(g.CombatScene.possessionTurn)
	}
	endReason := g.checkCombatEndConditions()

	return g.CombatScene == nil || endReason == CombatDefeat || endReason == CombatEvacuated
}

func (g *GameState) configureCombatTurnQueue(combat *CombatState) {
	combat.TurnQueue.actionCount = g.combatActionTimeCounter
	combat.TurnQueue.SetTemporalEffectsResolver(func() CombatTemporalEffects {
		return g.combatTemporalEffectsFor(combat)
	})
}

func (g *GameState) combatTemporalEffectsFor(combat *CombatState) CombatTemporalEffects {
	effect := g.ActiveDurationEffect

	return CombatTemporalEffects{
		EffectType:     effect.EffectType,
		TurnsRemaining: effect.TurnsRemaining,
		IsMonsterLoyal: func(identity CombatTurnIdentity) bool {
			if combat == nil {
				return false
			}

			return g.combatIdentityLoyalty(identity) == CombatMonsterLoyaltyMonster
		},
	}
}

func (g *GameState) advanceCombatQueueTime() {
	queue := g.CombatScene.TurnQueue
	advance := queue.ShouldAdvanceTime()
	g.combatActionTimeCounter = queue.actionCount
	if !advance {
		return
	}

	elapsed := g.AdvanceGameplayMinutes(CombatTimeAdvanceMinutes)
	logging.Debugf(logging.CategoryGameState,
		"Combat time advanced by %d minute(s) after %d scheduled actions",
		elapsed, actionsPerTimeAdvance)
}
