package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Circle study lets a member cast spells of one Circle above their level. Lady
// Janell teaches it at the Lycaeum, once for each member, for gold.
// REDUX DEVIATION: PROGRESS-0005.

// CircleStudyPrice is the gold one member pays, once, for circle study.
const CircleStudyPrice = 3000

var (
	// ErrCircleStudyUnavailable reports a member who cannot take circle study.
	ErrCircleStudyUnavailable = errors.New("circle study is unavailable to this member")
	// ErrCircleStudyInsufficientGold reports a party that cannot pay for circle study.
	ErrCircleStudyInsufficientGold = errors.New("insufficient gold for circle study")
)

// CircleStudyEligibility says whether a member may study and, if not, why.
type CircleStudyEligibility byte

const (
	// CircleStudyEligible identifies a member who may study now.
	CircleStudyEligible CircleStudyEligibility = iota
	// CircleStudyNoMagic identifies a class that never casts spells.
	CircleStudyNoMagic
	// CircleStudyDead identifies a member who must be raised first.
	CircleStudyDead
	// CircleStudyAlreadyStudied identifies a member who has studied before.
	CircleStudyAlreadyStudied
	// CircleStudyNothingToLearn identifies a member who already casts the eighth Circle.
	CircleStudyNothingToLearn
)

// CircleStudyCandidate is one active member as the teacher sees them.
type CircleStudyCandidate struct {
	RosterIndex int
	Name        string
	Class       string
	Level       int
	// Circle is the highest Circle the member casts now; zero without magic.
	Circle      int
	Eligibility CircleStudyEligibility
}

// CircleStudyState is the detached view a study screen presents.
type CircleStudyState struct {
	Price      int
	Gold       int
	Candidates []CircleStudyCandidate
}

// GetCircleStudyState lists the active party in order with each member's eligibility.
func (g *GameState) GetCircleStudyState() CircleStudyState {
	state := CircleStudyState{
		Price:      CircleStudyPrice,
		Gold:       int(g.PartyState.Inventory.Gold.Get()),
		Candidates: make([]CircleStudyCandidate, 0, party_state.MaxActiveParty),
	}
	for rosterIndex := range g.PartyState.GetActivePartyCount() {
		member := g.PartyState.GetCharacter(rosterIndex)
		candidate := CircleStudyCandidate{
			RosterIndex: rosterIndex,
			Name:        member.GetNameAsString(),
			Class:       member.Class.String(),
			Level:       int(member.Level),
			Eligibility: circleStudyEligibility(member),
		}
		if member.CanCastSpells() {
			candidate.Circle = member.SpellCircleLimit()
		}
		state.Candidates = append(state.Candidates, candidate)
	}

	return state
}

func circleStudyEligibility(member *party_state.PlayerCharacter) CircleStudyEligibility {
	switch {
	case !member.CanCastSpells():
		return CircleStudyNoMagic
	case member.IsDead():
		return CircleStudyDead
	case member.HasCircleStudy():
		return CircleStudyAlreadyStudied
	case member.SpellCircleLimit() >= references.MaxSpellCircle:
		return CircleStudyNothingToLearn
	default:
		return CircleStudyEligible
	}
}

// StudyCircle charges the price and marks one active member as studied. It
// changes nothing when the member is ineligible or the party cannot pay.
func (g *GameState) StudyCircle(rosterIndex int) error {
	if rosterIndex < 0 || rosterIndex >= g.PartyState.GetActivePartyCount() {
		return fmt.Errorf("%w: roster index %d is not in the active party", ErrCircleStudyUnavailable, rosterIndex)
	}
	member := g.PartyState.GetCharacter(rosterIndex)
	if circleStudyEligibility(member) != CircleStudyEligible {
		return fmt.Errorf("%w: %s", ErrCircleStudyUnavailable, member.GetNameAsString())
	}
	if !g.PartyState.Inventory.Gold.DecrementBy(CircleStudyPrice) {
		return ErrCircleStudyInsufficientGold
	}
	member.SetCircleStudy(true)

	return nil
}

// circleStudyRosterIndices returns the saved, ascending record of members who
// have studied. No member is the zero value.
func (g *GameState) circleStudyRosterIndices() []int {
	var indices []int
	g.PartyState.ForEachCharacter(func(index int, member *party_state.PlayerCharacter) {
		if member.HasCircleStudy() {
			indices = append(indices, index)
		}
	})

	return indices
}

// restoreCircleStudy marks the restored roster from a saved record.
func (g *GameState) restoreCircleStudy(indices []int) error {
	for _, index := range indices {
		if !g.PartyState.HasCharacter(index) {
			return fmt.Errorf("%w: roster slot %d", ErrCircleStudyUnavailable, index)
		}
		g.PartyState.GetCharacter(index).SetCircleStudy(true)
	}

	return nil
}
