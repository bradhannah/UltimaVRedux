// Package party_state holds the party roster, inventory and karma of the original save.
package party_state

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// SwapMembers error values
var (
	ErrAvatarMustLead    = errors.New("avatar must lead")
	ErrIndexOutOfBounds  = errors.New("index out of active party bounds")
	ErrNoCharacterAtSlot = errors.New("no character at index")
)

// Roster recruitment error values.
var (
	ErrNPCNotInRoster       = errors.New("NPC not found in roster")
	ErrRosterIndexInvalid   = errors.New("roster index out of bounds")
	ErrPartyFull            = errors.New("party is full")
	ErrNoCharacterAtRoster  = errors.New("no character at roster index")
	ErrAlreadyInActiveParty = errors.New("already in the party")
)

// PartyState holds the 16-slot character roster, the shared inventory, karma and the
// per-location met and dead NPC flags of the original SAVED.GAM party block.
type PartyState struct {
	characters [NRosterSlots]*PlayerCharacter // private - use GetCharacter()/ForEachCharacter()
	Inventory  Inventory
	Karma      Karma
	metNpcs    map[references.Location][]bool
	deadNpcs   map[references.Location][]bool
}

const (
	// NRosterSlots is the total number of character roster slots (all companions).
	NRosterSlots = 16
	// MaxActiveParty is the maximum number of active party members at once.
	MaxActiveParty = 6
	// NPcsPerLocation is the number of NPCs tracked per location.
	NPcsPerLocation = 32
)

// Character position constants
const (
	AvatarCharacterIndex = 0 // Avatar is always at index 0 in the party
)

// GetCharacter returns a character by index with bounds checking
// Panics if index is out of bounds or character is nil (programming error - data corruption)
func (p *PartyState) GetCharacter(index int) *PlayerCharacter {
	if index < 0 || index >= NRosterSlots {
		panic(fmt.Sprintf("character index %d out of bounds [0,%d)", index, NRosterSlots))
	}
	if p.characters[index] == nil {
		panic(fmt.Sprintf("character at index %d is nil - data corruption", index))
	}

	return p.characters[index]
}

// HasCharacter reports whether an in-range roster slot contains a record.
func (p *PartyState) HasCharacter(index int) bool {
	return index >= 0 && index < NRosterSlots && p.characters[index] != nil
}

// GetCharacterCount returns the total number of character roster slots (always 16)
func (p *PartyState) GetCharacterCount() int {
	return NRosterSlots
}

// ForEachCharacter iterates over all non-nil characters
// Safe for loops - automatically skips nil entries
func (p *PartyState) ForEachCharacter(fn func(index int, char *PlayerCharacter)) {
	for i := range p.characters {
		if p.characters[i] != nil {
			fn(i, p.characters[i])
		}
	}
}

// GetAvatar returns the avatar character (always at index 0)
// Panics if avatar is nil (data corruption)
func (p *PartyState) GetAvatar() *PlayerCharacter {
	return p.GetCharacter(AvatarCharacterIndex)
}

func newPartyState() *PartyState {
	partyState := new(PartyState)
	partyState.metNpcs = make(map[references.Location][]bool)
	partyState.deadNpcs = make(map[references.Location][]bool)
	partyState.Inventory = *NewInventory()

	// Original npc_dead[onmap-1] stores 32 NPC bits for map IDs 1 through 32.
	for location := references.Moonglow; location <= references.SerpentsHold; location++ {
		partyState.metNpcs[location] = make([]bool, NPcsPerLocation)
		partyState.deadNpcs[location] = make([]bool, NPcsPerLocation)
	}

	return partyState
}

// NewPartyStateFromRoster constructs an ordered roster without relying on the
// packed legacy SAVED.GAM memory layout. Character values are copied.
func NewPartyStateFromRoster(roster [NRosterSlots]*PlayerCharacter) *PartyState {
	partyState := newPartyState()
	for index, character := range roster {
		if character == nil {
			continue
		}
		copyOfCharacter := *character
		partyState.characters[index] = &copyOfCharacter
	}

	return partyState
}

// NPCFlagsSnapshot returns detached met and dead NPC flags in stable location/NPC order.
func (p *PartyState) NPCFlagsSnapshot() (map[references.Location][]bool, map[references.Location][]bool) {
	clone := func(source map[references.Location][]bool) map[references.Location][]bool {
		result := make(map[references.Location][]bool, len(source))
		for location, flags := range source {
			result[location] = append([]bool(nil), flags...)
		}

		return result
	}

	return clone(p.metNpcs), clone(p.deadNpcs)
}

// RestoreNPCFlags replaces authoritative NPC flags from detached values.
func (p *PartyState) RestoreNPCFlags(met, dead map[references.Location][]bool) {
	for location := range p.metNpcs {
		clear(p.metNpcs[location])
		clear(p.deadNpcs[location])
	}
	for location, flags := range met {
		destination, ok := p.metNpcs[location]
		if !ok || len(flags) > len(destination) {
			panic(fmt.Sprintf("invalid met NPC flags for location %d", location))
		}
		copy(destination, flags)
	}
	for location, flags := range dead {
		destination, ok := p.deadNpcs[location]
		if !ok || len(flags) > len(destination) {
			panic(fmt.Sprintf("invalid dead NPC flags for location %d", location))
		}
		copy(destination, flags)
	}
}

// HasRoom returns true if the party can accept another companion.
// A party has room when fewer than MaxActiveParty (6) characters have PartyStatus = InTheParty.
func (p *PartyState) HasRoom() bool {
	activeCount := 0
	for _, c := range p.characters {
		if c != nil && c.PartyMap == PartyMapActive {
			activeCount++
		}
	}

	return activeCount < MaxActiveParty
}

// JoinNPC adds an NPC to the party roster and updates their status.
// Recruitment searches inactive records and moves the selected record into the active prefix; ref:SAVE-0002.
//
// Process:
// 1. Search roster (16 entries) for matching NPC by name (case insensitive, 3-char prefix)
// 2. Mark NPC as in party (PartyStatus = InTheParty)
// 3. Swap roster slots to keep party members at head (slots 0-5)
//
// Parameters:
//   - npcRef: unused; recruitment matches on name only
//   - npcName: NPC name extracted from the semantic conversation definition
//
// Returns error if NPC not found in roster.
func (p *PartyState) JoinNPC(_ references.NPCReference, npcName string) error {
	// 1. Search roster (16 entries) for matching NPC by name (case insensitive, 3-char max)
	// Original Ultima V used 3-byte character arrays for NPC names in SAVED.GAM
	var rosterIndex = -1
	// Original recruitment searches inactive records from slot 15 down to 1; ref:SAVE-0002.
	// Slot zero is always the Avatar and cannot satisfy recruitment.
	for candidateIndex := NRosterSlots - 1; candidateIndex >= 1; candidateIndex-- {
		if p.characters[candidateIndex] != nil {
			charName := p.characters[candidateIndex].GetNameAsString()
			// Match first 3 characters, case insensitive (original game behavior)
			if len(charName) >= 3 && len(npcName) >= 3 {
				if strings.EqualFold(charName[:3], npcName[:3]) {
					rosterIndex = candidateIndex

					break
				}
			}
		}
	}

	if rosterIndex == -1 {
		return fmt.Errorf("%w: %s", ErrNPCNotInRoster, npcName)
	}

	// 2. Mark NPC as in party
	p.characters[rosterIndex].PartyMap = PartyMapActive

	// 3. Swap roster slots - keep party members at head (slots 0-5)
	// Swap complete roster entries to maintain the active party prefix; ref:SAVE-0002.
	// Find first non-party slot after position 0 (Avatar is always slot 0)
	firstNonPartySlot := -1
	for i := 1; i < MaxActiveParty; i++ {
		if p.characters[i] == nil || p.characters[i].PartyMap != PartyMapActive {
			firstNonPartySlot = i

			break
		}
	}

	// If NPC was not in party slots (1-5), swap them to first available party slot
	if firstNonPartySlot != -1 && rosterIndex > firstNonPartySlot {
		p.characters[rosterIndex], p.characters[firstNonPartySlot] =
			p.characters[firstNonPartySlot], p.characters[rosterIndex]
	}

	return nil
}

// AddCharacterToActiveParty adds a character from the roster to the active party.
// This method marks the character as InTheParty and swaps roster slots to keep
// party members in slots 0-5 for proper display in the party view.
//
// Parameters:
//   - rosterIndex: Index (0-15) of the character in the roster to add
//
// Returns error if:
//   - Party is full (already has 6 members)
//   - Index is out of bounds
//   - Character is already in party
func (p *PartyState) AddCharacterToActiveParty(rosterIndex int) error {
	// Validate index
	if rosterIndex < 0 || rosterIndex >= NRosterSlots {
		return fmt.Errorf("%w: %d [0,%d)", ErrRosterIndexInvalid, rosterIndex, NRosterSlots)
	}

	// Check if party has room
	if !p.HasRoom() {
		return fmt.Errorf("%w (%d/%d members)", ErrPartyFull, MaxActiveParty, MaxActiveParty)
	}

	char := p.characters[rosterIndex]
	if char == nil {
		return fmt.Errorf("%w %d", ErrNoCharacterAtRoster, rosterIndex)
	}

	// Check if already in party
	if char.PartyMap == PartyMapActive {
		return fmt.Errorf("%s is %w", char.GetNameAsString(), ErrAlreadyInActiveParty)
	}

	// Mark character as in party
	char.PartyMap = PartyMapActive

	// Swap roster slots - keep party members at head (slots 0-5)
	// Find first non-party slot after position 0 (Avatar is always slot 0)
	firstNonPartySlot := -1
	for i := 1; i < MaxActiveParty; i++ {
		if p.characters[i] == nil || p.characters[i].PartyMap != PartyMapActive {
			firstNonPartySlot = i

			break
		}
	}

	// If character was not in party slots (1-5), swap them to first available party slot
	if firstNonPartySlot != -1 && rosterIndex > firstNonPartySlot {
		p.characters[rosterIndex], p.characters[firstNonPartySlot] =
			p.characters[firstNonPartySlot], p.characters[rosterIndex]
	}

	return nil
}

// LeaveCharacterAtInn applies complete-record stable removal (ref:MERCH-0011). The
// caller owns all Inn eligibility and capacity validation.
func (p *PartyState) LeaveCharacterAtInn(characterIndex int, location references.Location) {
	character := p.GetCharacter(characterIndex)
	character.PartyMap = PartyMapAtInn(location)
	character.MonthsAtInn = 0
	for index := characterIndex; index < NRosterSlots-1; index++ {
		p.characters[index] = p.characters[index+1]
	}
	p.characters[NRosterSlots-1] = character
}

// PickUpCharacterFromInn applies complete-record stable insertion (ref:MERCH-0011) at
// the end of the packed active prefix. The caller owns all Inn eligibility,
// location, billing, and capacity validation. The returned index is the new
// active slot.
func (p *PartyState) PickUpCharacterFromInn(guestIndex int) int {
	partySize := p.GetActivePartyCount()
	character := p.GetCharacter(guestIndex)
	for index := guestIndex; index > partySize; index-- {
		p.characters[index] = p.characters[index-1]
	}
	p.characters[partySize] = character
	character.PartyMap = PartyMapActive

	return partySize
}

// HasMetNPC reports whether the Avatar has met the NPC with the given index at a location.
func (p *PartyState) HasMetNPC(location references.Location, npcID int) bool {
	if locationNPCs, ok := p.metNpcs[location]; ok {
		if npcID >= 0 && npcID < len(locationNPCs) {
			return locationNPCs[npcID]
		}
	}

	return false
}

// AvatarName returns the Avatar name stored in roster slot 0.
func (p *PartyState) AvatarName() string {
	avatar := p.GetAvatar()

	return avatar.GetNameAsString()
}

// SetMet records that the Avatar has met an NPC; it panics on an unknown location or NPC index.
func (p *PartyState) SetMet(location references.Location, npcID int) {
	validateNPCFlagIdentity(location, npcID)
	p.metNpcs[location][npcID] = true
}

// SetDeadNpc records an NPC as persistently absent; it panics on an unknown location or NPC index.
func (p *PartyState) SetDeadNpc(location references.Location, npcID int) {
	validateNPCFlagIdentity(location, npcID)
	p.deadNpcs[location][npcID] = true
}

func validateNPCFlagIdentity(location references.Location, npcID int) {
	if location < references.Moonglow || location > references.SerpentsHold {
		panic(fmt.Sprintf(
			"NPC flag location %d out of bounds [%d,%d]",
			location,
			references.Moonglow,
			references.SerpentsHold,
		))
	}
	if npcID < 0 || npcID >= NPcsPerLocation {
		panic(fmt.Sprintf("NPC flag index %d out of bounds [0,%d)", npcID, NPcsPerLocation))
	}
}

// IsNPCDead checks if a source NPC is persistently absent at a location.
// The shared original flag stores killed NPCs, freed prisoners, and recruited companions;
// callers remain responsible for presenting those causes distinctly.
// Original NPC removal records persistent actor absence; ref:CRIME-0004.
func (p *PartyState) IsNPCDead(location references.Location, npcID int) bool {
	if deadNpcs, ok := p.deadNpcs[location]; ok {
		if npcID >= 0 && npcID < len(deadNpcs) {
			return deadNpcs[npcID]
		}
	}

	return false
}

// SwapMembers swaps the positions of two party members in the active party.
// New Order swaps complete active-party records; ref:SAVE-0002.
//
// Parameters:
//   - index1: First character index (1-5, Avatar at 0 cannot be swapped)
//   - index2: Second character index (1-5, must be different from index1)
//
// Returns error if:
//   - Either index is Avatar (0) - "Avatar must lead!"
//   - Either index is out of active party bounds (0-5)
//   - Either character slot is empty
//   - Indices are the same (no-op, but not an error)
func (p *PartyState) SwapMembers(index1, index2 int) error {
	// Avatar cannot be swapped - must always be at position 0
	if index1 == AvatarCharacterIndex || index2 == AvatarCharacterIndex {
		return ErrAvatarMustLead
	}

	// Validate bounds for active party (0-5)
	if index1 < 0 || index1 >= MaxActiveParty {
		return fmt.Errorf("%w: index1 %d [0,%d)", ErrIndexOutOfBounds, index1, MaxActiveParty)
	}

	if index2 < 0 || index2 >= MaxActiveParty {
		return fmt.Errorf("%w: index2 %d [0,%d)", ErrIndexOutOfBounds, index2, MaxActiveParty)
	}

	// Same index is a no-op (not an error)
	if index1 == index2 {
		return nil
	}

	// Validate both slots have characters
	if p.characters[index1] == nil {
		return fmt.Errorf("%w: %d", ErrNoCharacterAtSlot, index1)
	}

	if p.characters[index2] == nil {
		return fmt.Errorf("%w: %d", ErrNoCharacterAtSlot, index2)
	}

	// Swap complete records, preserving character-owned state; ref:SAVE-0002.
	p.characters[index1], p.characters[index2] = p.characters[index2], p.characters[index1]

	return nil
}

// GetActivePartyCount returns the number of active party members (those with InTheParty status).
func (p *PartyState) GetActivePartyCount() int {
	count := 0
	for i := range MaxActiveParty {
		if p.characters[i] != nil && p.characters[i].PartyMap == PartyMapActive {
			count++
		}
	}

	return count
}

// GetLivingPartyCount returns the original status-based live-player count for
// the packed active prefix.
func (p *PartyState) GetLivingPartyCount() int {
	count := 0
	for index := range p.GetActivePartyCount() {
		if !p.GetCharacter(index).IsDead() {
			count++
		}
	}

	return count
}

// MartyrSecondLiving applies the Palace martyrdom rule's complete-record stable
// removal and returns the record moved to slot 15; ref:KARMA-0013.
func (p *PartyState) MartyrSecondLiving() *PlayerCharacter {
	const martyrLivingOrdinal = 2

	activeCount := p.GetActivePartyCount()
	living := 0
	removeIndex := -1
	for index := range activeCount {
		if p.GetCharacter(index).IsDead() {
			continue
		}
		living++
		if living == martyrLivingOrdinal {
			removeIndex = index

			break
		}
	}
	if removeIndex < 0 {
		panic("MartyrSecondLiving requires at least two living party members")
	}

	martyr := p.characters[removeIndex]
	for index := removeIndex; index < NRosterSlots-1; index++ {
		p.characters[index] = p.characters[index+1]
	}
	p.characters[NRosterSlots-1] = martyr
	martyr.PartyMap = PartyMapMartyr

	return martyr
}

// MartyrNames returns Blackthorn's victims from slots 1 through 15 in roster order.
func (p *PartyState) MartyrNames() []string {
	names := make([]string, 0)
	for index := 1; index < NRosterSlots; index++ {
		character := p.characters[index]
		if character != nil && character.PartyMap.IsMartyr() {
			names = append(names, character.GetNameAsString())
		}
	}

	return names
}

// HasDeadMembers returns true if any active party member is dead.
// Used by resurrection scroll to check if there are valid targets.
func (p *PartyState) HasDeadMembers() bool {
	for i := range MaxActiveParty {
		char := p.characters[i]
		if char != nil && char.PartyMap == PartyMapActive && char.IsDead() {
			return true
		}
	}

	return false
}

// IsAllPartyDead returns true if ALL active party members are dead.
// This triggers the death/resurrection sequence (transport to Castle Britannia).
// Returns false if there are no active party members.
func (p *PartyState) IsAllPartyDead() bool {
	activeCount := 0
	deadCount := 0

	for i := range MaxActiveParty {
		char := p.characters[i]
		if char != nil && char.PartyMap == PartyMapActive {
			activeCount++
			if char.IsDead() {
				deadCount++
			}
		}
	}

	// Return false if no active party members (edge case)
	if activeCount == 0 {
		return false
	}

	return deadCount == activeCount
}
