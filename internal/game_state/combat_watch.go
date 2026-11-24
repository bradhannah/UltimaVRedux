package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// CombatPocketWatchUseCommit retains a single selected watch and its exact owner.
type CombatPocketWatchUseCommit struct {
	state     *GameState
	owner     CombatUseOwner
	quantity  uint16
	committed bool
}

// BeginCombatPocketWatchUse reviews ownership without spending an action or time.
func (g *GameState) BeginCombatPocketWatchUse(owner CombatUseOwner) *CombatPocketWatchUseCommit {
	quantity := g.PartyState.Inventory.QuestItems.Get(references.PocketWatch)
	if quantity == 0 || !g.IsCurrentCombatUseOwner(owner) {
		return nil
	}

	return &CombatPocketWatchUseCommit{state: g, owner: owner, quantity: quantity}
}

// CommitCombatPocketWatchUse reads current time at commitment without consuming
// the watch. Its owning Use workflow awaits dismissal before completing the
// retained actor action; only the combat scheduler advances time.
// REDUX DEVIATION: TIME-0005: combat Use exposes the owned watch and reads the time
// when the selection commits. Redux adds no independent calendar charge. See ref:TIME-0005.
func (g *GameState) CommitCombatPocketWatchUse(commit *CombatPocketWatchUseCommit) (string, bool) {
	if commit == nil || commit.state != g || commit.committed {
		return "", false
	}
	commit.committed = true
	if !g.IsCurrentCombatUseOwner(commit.owner) ||
		g.PartyState.Inventory.QuestItems.Get(references.PocketWatch) != commit.quantity {
		return "", false
	}
	g.CommitCastUseAttempt()

	return g.DateTime.GetTimeAsString(), true
}
