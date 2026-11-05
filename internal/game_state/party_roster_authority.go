package game_state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
)

const (
	maximumCharacterLevel       = 8
	minimumLevelEightExperience = 6400
	maximumHPPerLevel           = 30
)

var (
	// ErrPartyMutationInCombat indicates that party state cannot be synchronized with active combat.
	ErrPartyMutationInCombat = errors.New("party mutation unavailable during combat")
	// ErrInvalidPartyMutation indicates that a requested party mutation violates roster invariants.
	ErrInvalidPartyMutation = errors.New("invalid party mutation")
	// ErrStalePartyRoster indicates that the roster changed after it was reviewed.
	ErrStalePartyRoster = errors.New("stale party roster")
	// ErrPartyCapacity indicates that the active party is already full.
	ErrPartyCapacity = errors.New("active party is full")
)

// PartyRosterCharacterSnapshot is a detached view of one stable roster slot.
type PartyRosterCharacterSnapshot struct {
	RosterIndex  int                         `json:"rosterIndex"`
	Name         string                      `json:"name"`
	Gender       party_state.CharacterGender `json:"gender"`
	Class        party_state.CharacterClass  `json:"class"`
	Status       party_state.CharacterStatus `json:"status"`
	PartyMap     party_state.PartyMap        `json:"partyMap"`
	Strength     byte                        `json:"strength"`
	Dexterity    byte                        `json:"dexterity"`
	Intelligence byte                        `json:"intelligence"`
	CurrentMP    byte                        `json:"currentMp"`
	CurrentHP    uint16                      `json:"currentHp"`
	MaximumHP    uint16                      `json:"maximumHp"`
	Experience   uint16                      `json:"experience"`
	Level        byte                        `json:"level"`
	MonthsAtInn  byte                        `json:"monthsAtInn"`
	ArmorClass   byte                        `json:"armorClass"`
	Helmet       byte                        `json:"helmet"`
	Armor        byte                        `json:"armor"`
	Weapon       byte                        `json:"weapon"`
	Shield       byte                        `json:"shield"`
	Ring         byte                        `json:"ring"`
	Amulet       byte                        `json:"amulet"`
}

// PartyRosterSnapshot captures the complete roster and its stale-review fingerprint.
type PartyRosterSnapshot struct {
	Fingerprint string
	Characters  []PartyRosterCharacterSnapshot
}

// PartyRosterMutationResult reports the complete roster before and after an authoritative mutation.
type PartyRosterMutationResult struct {
	Before PartyRosterSnapshot
	After  PartyRosterSnapshot
}

// PartyStatusMutationRequest describes one atomic status change for one or more reviewed roster slots.
type PartyStatusMutationRequest struct {
	ExpectedFingerprint string
	RosterIndices       []int
	Status              party_state.CharacterStatus
}

// AddInactiveRosterMemberRequest identifies one reviewed inactive roster slot to add.
type AddInactiveRosterMemberRequest struct {
	ExpectedFingerprint string
	RosterIndex         int
}

// CurrentPartyRosterSnapshot returns a detached roster snapshot with a deterministic fingerprint.
func (g *GameState) CurrentPartyRosterSnapshot() PartyRosterSnapshot {
	characters := make([]PartyRosterCharacterSnapshot, party_state.NRosterSlots)
	for index := range party_state.NRosterSlots {
		var character *party_state.PlayerCharacter
		if g.PartyState.HasCharacter(index) {
			character = g.PartyState.GetCharacter(index)
		}
		characters[index] = snapshotRosterCharacter(index, character)
	}

	payload, err := json.Marshal(characters)
	if err != nil {
		panic("fingerprinting party roster: " + err.Error())
	}
	digest := sha256.Sum256(payload)

	return PartyRosterSnapshot{
		Fingerprint: hex.EncodeToString(digest[:]),
		Characters:  characters,
	}
}

// MaximizeActiveParty raises every active member to level eight using authoritative progression rules.
func (g *GameState) MaximizeActiveParty(expectedFingerprint string) (PartyRosterMutationResult, error) {
	before, err := g.validatePartyMutation(expectedFingerprint)
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	activeCount, err := g.validateContiguousActiveParty()
	if err != nil {
		return PartyRosterMutationResult{}, err
	}

	for index := range activeCount {
		character := g.PartyState.GetCharacter(index)
		character.Level = maximumCharacterLevel
		if character.Exp < minimumLevelEightExperience {
			character.Exp = minimumLevelEightExperience
		}
		character.SetMaxHp(uint16(maximumCharacterLevel) * maximumHPPerLevel)
		character.SetHp(character.GetMaxHp())
		if character.IsDead() {
			character.SetStatus(party_state.Good)
		}
		character.RestoreMpByClass()
	}

	return g.finishPartyMutation(before), nil
}

// RestoreActiveParty heals and cures every active member: full hit points and
// magic points by class, status Good, and the dead revived as Maximize does.
func (g *GameState) RestoreActiveParty(expectedFingerprint string) (PartyRosterMutationResult, error) {
	before, err := g.validatePartyMutation(expectedFingerprint)
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	activeCount, err := g.validateContiguousActiveParty()
	if err != nil {
		return PartyRosterMutationResult{}, err
	}

	for index := range activeCount {
		character := g.PartyState.GetCharacter(index)
		if character.GetMaxHp() == 0 {
			continue
		}
		character.SetHp(character.GetMaxHp())
		character.SetStatus(party_state.Good)
		character.RestoreMpByClass()
	}

	return g.finishPartyMutation(before), nil
}

// SetPartyRosterStatuses atomically updates status and HP for reviewed active roster slots.
func (g *GameState) SetPartyRosterStatuses(request PartyStatusMutationRequest) (PartyRosterMutationResult, error) {
	before, err := g.validatePartyMutation(request.ExpectedFingerprint)
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	if err = validateRosterStatus(request.Status); err != nil {
		return PartyRosterMutationResult{}, err
	}
	if len(request.RosterIndices) == 0 {
		return PartyRosterMutationResult{}, fmt.Errorf("%w: at least one roster index is required", ErrInvalidPartyMutation)
	}

	seen := make(map[int]struct{}, len(request.RosterIndices))
	for _, index := range request.RosterIndices {
		if index < 0 || index >= party_state.NRosterSlots {
			return PartyRosterMutationResult{}, fmt.Errorf(
				"%w: roster index %d is out of bounds", ErrInvalidPartyMutation, index,
			)
		}
		if _, duplicate := seen[index]; duplicate {
			return PartyRosterMutationResult{}, fmt.Errorf("%w: roster index %d is duplicated", ErrInvalidPartyMutation, index)
		}
		seen[index] = struct{}{}
		if !g.PartyState.HasCharacter(index) {
			return PartyRosterMutationResult{}, fmt.Errorf(
				"%w: roster index %d is vacant", ErrInvalidPartyMutation, index,
			)
		}
		character := g.PartyState.GetCharacter(index)
		if !character.IsInParty() {
			return PartyRosterMutationResult{}, fmt.Errorf("%w: roster index %d is not active", ErrInvalidPartyMutation, index)
		}
		if request.Status != party_state.Dead && character.GetCurrentHp() == 0 && character.GetMaxHp() == 0 {
			return PartyRosterMutationResult{}, fmt.Errorf(
				"%w: roster index %d cannot receive a living status with zero maximum HP",
				ErrInvalidPartyMutation, index,
			)
		}
	}

	for _, index := range request.RosterIndices {
		character := g.PartyState.GetCharacter(index)
		if request.Status == party_state.Dead {
			character.SetHp(0)
			character.SetStatus(party_state.Dead)

			continue
		}
		if character.GetCurrentHp() == 0 {
			character.SetHp(1)
		}
		character.SetStatus(request.Status)
	}

	return g.finishPartyMutation(before), nil
}

// FillActiveParty adds inactive roster members in roster order until the party is full.
func (g *GameState) FillActiveParty(expectedFingerprint string) (PartyRosterMutationResult, error) {
	before, err := g.validatePartyMutation(expectedFingerprint)
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	activeCount, err := g.validateContiguousActiveParty()
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	if activeCount == party_state.MaxActiveParty {
		return PartyRosterMutationResult{}, ErrPartyCapacity
	}

	for activeCount < party_state.MaxActiveParty {
		candidateIndex := -1
		for index := activeCount; index < party_state.NRosterSlots; index++ {
			if g.PartyState.HasCharacter(index) && !g.PartyState.GetCharacter(index).IsInParty() {
				candidateIndex = index

				break
			}
		}
		if candidateIndex == -1 {
			break
		}
		if err = g.PartyState.AddCharacterToActiveParty(candidateIndex); err != nil {
			panic("validated party fill failed: " + err.Error())
		}
		activeCount++
	}

	return g.finishPartyMutation(before), nil
}

// AddInactiveRosterMember adds one reviewed inactive roster slot through the party composition API.
func (g *GameState) AddInactiveRosterMember(request AddInactiveRosterMemberRequest) (PartyRosterMutationResult, error) {
	before, err := g.validatePartyMutation(request.ExpectedFingerprint)
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	if request.RosterIndex < 0 || request.RosterIndex >= party_state.NRosterSlots {
		return PartyRosterMutationResult{}, fmt.Errorf(
			"%w: roster index %d is out of bounds", ErrInvalidPartyMutation, request.RosterIndex,
		)
	}
	activeCount, err := g.validateContiguousActiveParty()
	if err != nil {
		return PartyRosterMutationResult{}, err
	}
	if activeCount == party_state.MaxActiveParty {
		return PartyRosterMutationResult{}, ErrPartyCapacity
	}
	if !g.PartyState.HasCharacter(request.RosterIndex) {
		return PartyRosterMutationResult{}, fmt.Errorf(
			"%w: roster index %d is vacant", ErrInvalidPartyMutation, request.RosterIndex,
		)
	}
	if g.PartyState.GetCharacter(request.RosterIndex).IsInParty() {
		return PartyRosterMutationResult{}, fmt.Errorf(
			"%w: roster index %d is already active", ErrInvalidPartyMutation, request.RosterIndex,
		)
	}
	if err = g.PartyState.AddCharacterToActiveParty(request.RosterIndex); err != nil {
		panic("validated party addition failed: " + err.Error())
	}

	return g.finishPartyMutation(before), nil
}

func (g *GameState) validatePartyMutation(expectedFingerprint string) (PartyRosterSnapshot, error) {
	if g.IsInCombat() || g.CombatScene != nil {
		return PartyRosterSnapshot{}, ErrPartyMutationInCombat
	}
	if g.EventQueueManager.HasActiveWorkflows() {
		return PartyRosterSnapshot{}, ErrPartyMutationInCombat
	}
	if expectedFingerprint == "" {
		return PartyRosterSnapshot{}, fmt.Errorf("%w: expected roster fingerprint is required", ErrInvalidPartyMutation)
	}
	before := g.CurrentPartyRosterSnapshot()
	if before.Fingerprint != expectedFingerprint {
		return PartyRosterSnapshot{}, ErrStalePartyRoster
	}

	return before, nil
}

func (g *GameState) validateContiguousActiveParty() (int, error) {
	activeCount := 0
	seenInactive := false
	for index := range party_state.NRosterSlots {
		if !g.PartyState.HasCharacter(index) {
			seenInactive = true

			continue
		}
		character := g.PartyState.GetCharacter(index)
		if err := validateRosterCharacterIdentity(character); err != nil {
			return 0, fmt.Errorf("%w: roster slot %d: %w", ErrInvalidPartyMutation, index, err)
		}
		active := character.IsInParty()
		if active && (seenInactive || activeCount == party_state.MaxActiveParty) {
			return 0, fmt.Errorf("%w: active party slots are not contiguous", ErrInvalidPartyMutation)
		}
		if active {
			activeCount++
		} else {
			seenInactive = true
		}
	}
	if activeCount == 0 {
		return 0, fmt.Errorf("%w: the Avatar must be active", ErrInvalidPartyMutation)
	}

	return activeCount, nil
}

func validateRosterCharacterIdentity(character *party_state.PlayerCharacter) error {
	switch character.Gender {
	case party_state.Male, party_state.Female:
	default:
		return errors.New("invalid character gender")
	}
	switch character.Class {
	case party_state.Avatar, party_state.Fighter, party_state.Bard, party_state.Wizard:
	default:
		return errors.New("invalid character class")
	}

	return validateRosterStatus(character.GetStatus())
}

func (g *GameState) finishPartyMutation(before PartyRosterSnapshot) PartyRosterMutationResult {
	return PartyRosterMutationResult{Before: before, After: g.CurrentPartyRosterSnapshot()}
}

func validateRosterStatus(status party_state.CharacterStatus) error {
	switch status {
	case party_state.Good, party_state.Poisoned, party_state.Sleep, party_state.Charmed, party_state.Dead:
		return nil
	default:
		return fmt.Errorf("%w: unsupported character status %q", ErrInvalidPartyMutation, byte(status))
	}
}

func snapshotRosterCharacter(index int, character *party_state.PlayerCharacter) PartyRosterCharacterSnapshot {
	if character == nil {
		return PartyRosterCharacterSnapshot{RosterIndex: index}
	}

	return PartyRosterCharacterSnapshot{
		RosterIndex:  index,
		Name:         character.GetNameAsString(),
		Gender:       character.Gender,
		Class:        character.Class,
		Status:       character.GetStatus(),
		PartyMap:     character.PartyMap,
		Strength:     character.Strength,
		Dexterity:    character.Dexterity,
		Intelligence: character.Intelligence,
		CurrentMP:    character.GetCurrentMp(),
		CurrentHP:    character.GetCurrentHp(),
		MaximumHP:    character.GetMaxHp(),
		Experience:   character.Exp,
		Level:        character.Level,
		MonthsAtInn:  character.MonthsAtInn,
		ArmorClass:   character.ArmorClass,
		Helmet:       character.Helmet,
		Armor:        character.Armor,
		Weapon:       character.Weapon,
		Shield:       character.Shield,
		Ring:         character.Ring,
		Amulet:       character.Amulet,
	}
}
