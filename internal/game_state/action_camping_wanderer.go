package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// CheckForWandererAppearance checks if the wanderer should appear after successful camping
// Returns true if wanderer should appear based on:
// - Must be outdoor camping (not dungeon)
// - 25% chance (or forced)
func (g *GameState) CheckForWandererAppearance(forceWanderer bool) bool {
	logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: Starting check - forceWanderer=%t", forceWanderer)

	// Force wanderer for debug/testing
	if forceWanderer {
		logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: Force wanderer enabled - returning true")

		return true
	}

	// Check if outdoor camping (not dungeon)
	currentLocation := g.MapState.PlayerLocation.Location
	mapType := currentLocation.GetMapType()
	logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: Location=%v, MapType=%v", currentLocation, mapType)
	if mapType == references.DungeonMapType {
		logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: In dungeon - wanderer will not appear")

		return false
	}

	// Camp restoration uses random(0,99) < 25 after ordinary camp restoration.
	// See ref:PROGRESS-0003.
	randomResult := g.RandomIntInRange(0, 99) < 25
	logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: Random d100 check result=%t", randomResult)

	if randomResult {
		logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: All conditions met - wanderer will appear!")
	} else {
		logging.Debugf(logging.CategoryGameState, "CheckForWandererAppearance: Random chance failed - wanderer will not appear")
	}

	return randomResult
}

// CalculateLevelFromExperience maps XP to the repeatedly doubled thresholds.
// XP below 100 gives level 1. Otherwise level = 2 + floor(log2(XP / 100)).
// Thus levels 2 through 8 begin at 100, 200, 400, 800, 1600, 3200, 6400;
// 12800 begins level 9. This helper does not impose an eighth-level cap. ref:PROGRESS-0001
func CalculateLevelFromExperience(experience uint16) byte {
	// Start at level one, then add one level for each positive halving
	// of integer XP divided by 100. ref:PROGRESS-0001

	level := byte(1)
	expDivided := int(experience) / 100

	for expDivided > 0 {
		level++
		expDivided >>= 1 // Right shift by 1 (divide by 2)
	}

	return level
}

// WandererCharacterOutcome reports healing and progression without conflating
// upward rewards, downward reconciliation, and dead-record MP refresh.
type WandererCharacterOutcome struct {
	Restored    bool
	WasDead     bool
	HPIncreased bool
	PoisonCured bool
	LevelBefore byte
	LevelAfter  byte
	StatBoosts  []string
}

// ProcessWandererHealingForCharacter processes one active Wanderer party record.
func (g *GameState) ProcessWandererHealingForCharacter(characterIndex int) WandererCharacterOutcome {
	if characterIndex < 0 || characterIndex >= g.PartyState.GetCharacterCount() {
		return WandererCharacterOutcome{}
	}

	char := g.PartyState.GetCharacter(characterIndex)
	outcome := WandererCharacterOutcome{LevelBefore: char.Level, LevelAfter: char.Level}

	if char.IsDead() {
		char.RestoreMpByClass()
		outcome.WasDead = true

		return outcome
	}

	hpBefore := char.GetCurrentHp()
	wasWounded := hpBefore < char.GetMaxHp()
	statusBefore := char.GetStatus()
	char.SetHp(char.GetMaxHp())
	char.SetStatus(party_state.Good)
	outcome.Restored = true

	newLevel := CalculateLevelFromExperience(char.Exp)
	if newLevel != char.Level {
		char.Level = newLevel
		char.SetMaxHp(uint16(newLevel) * 30)
		char.SetHp(char.GetMaxHp())

		if newLevel > outcome.LevelBefore {
			levelsGained := int(newLevel) - int(outcome.LevelBefore)
			outcome.StatBoosts = make([]string, 0, levelsGained)
			// REDUX DEVIATION: original Wanderer rules grant one stat roll for any mismatch.
			// Redux retains one roll per upward level gained, while downward correction grants none.
			// See ref:PROGRESS-0003.
			for range levelsGained {
				if boost, awarded := g.awardWandererStat(char); awarded {
					outcome.StatBoosts = append(outcome.StatBoosts, boost)
				}
			}
		}
	}
	outcome.LevelAfter = char.Level
	outcome.HPIncreased = wasWounded && char.GetCurrentHp() > hpBefore
	outcome.PoisonCured = statusBefore == party_state.Poisoned && char.IsGood()
	char.RestoreMpByClass()

	return outcome
}

// awardWandererStat raises one of STR, DEX or INT by one.
//
// REDUX DEVIATION: PROGRESS-0003: the original rolls R(1,3) and wastes the award
// on a stat already at 30. Redux rolls only among stats below 30, so an earned
// level is never wasted, and awards nothing once all three are at 30. With no
// stat at 30 the roll and outcome match the original.
func (g *GameState) awardWandererStat(char *party_state.PlayerCharacter) (string, bool) {
	type statAward struct {
		value *byte
		boost string
	}
	open := make([]statAward, 0, 3)
	for _, award := range [...]statAward{
		{&char.Strength, "stronger"}, {&char.Dexterity, "quicker"}, {&char.Intelligence, "wiser"},
	} {
		if *award.value < MaxStatValue {
			open = append(open, award)
		}
	}
	if len(open) == 0 {
		return "", false
	}
	award := open[g.RollDice(len(open))-1]
	*award.value++

	return award.boost, true
}
