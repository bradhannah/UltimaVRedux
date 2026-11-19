package game_state

import "github.com/bradhannah/UltimaVRedux/internal/party_state"

// CombatExperienceShare is one active member's experience from a battle.
type CombatExperienceShare struct {
	Name   string
	Amount int
}

// CombatExperienceSummary reports a finished battle's experience for the
// Battle complete summary.
type CombatExperienceSummary struct {
	// Members lists every active member in party order, earners or not.
	Members []CombatExperienceShare
	// Shared is true when Share Kill Experience paid every living member the
	// same amount, which is then Each.
	Shared bool
	Each   int
	Ready  []ExperienceLevelReady
}

// Total is the experience the whole party earned.
func (summary CombatExperienceSummary) Total() int {
	total := 0
	for _, member := range summary.Members {
		total += member.Amount
	}

	return total
}

// awardKillExperience pays one kill and tallies it for the battle summary.
// The original credits the member who landed the killing blow. ref:PROGRESS-0002
func (g *GameState) awardKillExperience(attacker *party_state.PlayerCharacter, pcIdx, experience int) int {
	if g.GameConfig.GetShareKillExperience() {
		return g.shareKillExperience(experience)
	}
	awarded := min(experience, max(0, MaxExperienceValue-int(attacker.Exp)))
	attacker.Exp += uint16(awarded)
	g.CombatScene.tallyExperience(pcIdx, awarded)

	return awarded
}

// shareKillExperience splits a kill evenly among the living active party.
//
// REDUX DEVIATION: PROGRESS-0002: the original gives a kill's experience to the
// member who landed the killing blow, so members who seldom finish a foe fall
// behind. With the optional Share Kill Experience setting, off by default, every
// living member receives an equal share. Points that do not divide evenly are
// carried to the next kill, so shares stay equal and nothing is lost.
func (g *GameState) shareKillExperience(experience int) int {
	living := make([]int, 0, party_state.MaxActiveParty)
	for index := range party_state.MaxActiveParty {
		member := g.PartyState.GetCharacter(index)
		if member != nil && member.PartyMap == party_state.PartyMapActive && !member.IsDead() {
			living = append(living, index)
		}
	}
	if len(living) == 0 {
		return 0
	}
	pool := g.sharedExperienceRemainder + experience
	each := pool / len(living)
	g.sharedExperienceRemainder = pool % len(living)
	awarded := 0
	for _, index := range living {
		member := g.PartyState.GetCharacter(index)
		paid := min(each, max(0, MaxExperienceValue-int(member.Exp)))
		member.Exp += uint16(paid)
		g.CombatScene.tallyExperience(index, paid)
		awarded += paid
	}

	return awarded
}

func (s *CombatState) tallyExperience(partyIndex, amount int) {
	if partyIndex >= 0 && partyIndex < len(s.experienceEarned) {
		s.experienceEarned[partyIndex] += amount
	}
}

// combatExperienceSummary describes the ending battle's experience while its
// tally still exists. A battle that earned nothing has the zero summary.
func (g *GameState) combatExperienceSummary() CombatExperienceSummary {
	summary := CombatExperienceSummary{Shared: g.GameConfig.GetShareKillExperience()}
	livingSeen := false
	for index := range party_state.MaxActiveParty {
		member := g.PartyState.GetCharacter(index)
		if member == nil || member.PartyMap != party_state.PartyMapActive {
			continue
		}
		earned := g.CombatScene.experienceEarned[index]
		summary.Members = append(summary.Members, CombatExperienceShare{Name: member.GetNameAsString(), Amount: earned})
		if member.IsDead() {
			continue
		}
		if ready, level := experienceLevelReady(member, member.Exp-uint16(earned)); ready {
			summary.Ready = append(summary.Ready, ExperienceLevelReady{Name: member.GetNameAsString(), Level: level})
		}
		if !livingSeen {
			summary.Each, livingSeen = earned, true
		}
		summary.Shared = summary.Shared && earned == summary.Each
	}
	if summary.Total() == 0 {
		return CombatExperienceSummary{}
	}
	if !summary.Shared {
		summary.Each = 0
	}

	return summary
}
