package game_state

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func deedExperienceState(t *testing.T, tier int) *GameState {
	t.Helper()
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	gs.GameConfig.GameplayConfiguration.DeedExperience = tier
	require.GreaterOrEqual(t, gs.PartyState.GetActivePartyCount(), 3)

	return gs
}

func activeExperience(gs *GameState) []uint16 {
	var experience []uint16
	for index := range party_state.MaxActiveParty {
		if member := gs.PartyState.GetCharacter(index); member != nil && member.PartyMap == party_state.PartyMapActive {
			experience = append(experience, member.Exp)
		}
	}

	return experience
}

// The Lean table is the one the owner settled: 2,670 for every deed in the game.
func TestDeedCatalogPaysTheSettledLeanTotalUnderStableKeys(t *testing.T) {
	keyPattern := regexp.MustCompile(`^[a-z]+(\.[a-z-]+)+$`)
	seen := map[string]bool{}
	total := 0
	for _, entry := range deedCatalog() {
		assert.Regexp(t, keyPattern, entry.key)
		assert.False(t, seen[entry.key], "deed key %q repeats", entry.key)
		seen[entry.key] = true
		total += entry.experience
	}
	assert.Len(t, seen, 35)
	assert.Equal(t, 2670, total)
	for _, key := range []string{
		"codex.honesty", "shrine.humility", "codex.revelation", "dungeon.doom",
		"item.sandalwood-box", "item.shard.cowardice", "shadowlord.nosfentor",
	} {
		assert.True(t, seen[key], "saved key %q must not be renamed", key)
	}
}

func TestShrineQuestPaysOnceAtTheCodexAndOnceOnTheReturn(t *testing.T) {
	gs := deedExperienceState(t, config.DeedExperienceLean)
	before := activeExperience(gs)
	gs.PartyState.GetCharacter(1).SetStatus(party_state.Dead)

	assert.False(t, gs.SettleDeedExperience().Paid(), "a quest merely given pays nothing")
	gs.shrineQuestsActive[0] = true
	assert.False(t, gs.SettleDeedExperience().Paid())

	gs.shrineQuestsCompleted[0] = true
	answer := gs.SettleDeedExperience()
	assert.Equal(t, 20, answer.Each)
	assert.Equal(t, len(before), answer.Recipients, "the dead are paid with the living")
	assert.Equal(t, 1, answer.Deeds)
	assert.False(t, answer.BackPay)
	assert.False(t, gs.SettleDeedExperience().Paid(), "the Codex answer pays once")

	gs.shrineQuestsActive[0] = false
	assert.Equal(t, 20, gs.SettleDeedExperience().Each)
	assert.False(t, gs.SettleDeedExperience().Paid(), "an offering visit pays nothing")
	for index, experience := range activeExperience(gs) {
		assert.Equal(t, before[index]+40, experience)
	}
}

func TestFinalRevelationNeedsEveryQuestAndTheWord(t *testing.T) {
	gs := deedExperienceState(t, config.DeedExperienceLean)
	for index := range TotalShrines {
		gs.shrineQuestsCompleted[index] = true
	}
	assert.Equal(t, 16*20, gs.SettleDeedExperience().Each, "eight answers and eight returns")
	gs.DiscoverWordOfPowerKnowledge("doom", RevelationKnowledgeSource("Codex of Ultimate Wisdom"))
	assert.Equal(t, 200, gs.SettleDeedExperience().Each)
}

func TestDeedsDoneWhilePayingNothingArePaidBackOnceAtTheNewRate(t *testing.T) {
	gs := deedExperienceState(t, config.DeedExperienceNone)
	before := activeExperience(gs)
	gs.DungeonStates[0] = true
	gs.ShadowlordState.Kill(references.Astaroth)

	assert.False(t, gs.SettleDeedExperience().Paid(), "None pays nothing")
	assert.Empty(t, gs.deedExperiencePaidKeys(), "and leaves the deeds unpaid")

	gs.GameConfig.GameplayConfiguration.DeedExperience = config.DeedExperienceFuller
	backPay := gs.SettlePastDeedExperience()
	// A destroyed Shadowlord proves its shard was recovered: 25 + 300 + 150, doubled.
	assert.Equal(t, 2*(25+300+150), backPay.Each)
	assert.Equal(t, 3, backPay.Deeds)
	assert.True(t, backPay.BackPay)
	assert.Equal(t, []string{"dungeon.deceit", "item.shard.hatred", "shadowlord.astaroth"}, gs.deedExperiencePaidKeys())
	assert.Equal(t, before[0]+950, activeExperience(gs)[0])

	gs.DungeonStates[0] = false
	gs.GameConfig.GameplayConfiguration.DeedExperience = config.DeedExperienceLean
	assert.False(t, gs.SettlePastDeedExperience().Paid(), "a paid deed never pays again, whatever the setting")
}

func TestDeedExperienceNamesTheLivingWhoseLevelIsWaitingAndStopsAtTheCap(t *testing.T) {
	gs := deedExperienceState(t, config.DeedExperienceLean)
	climber, capped, fallen := gs.PartyState.GetCharacter(0), gs.PartyState.GetCharacter(1), gs.PartyState.GetCharacter(2)
	climber.Exp, climber.Level = 190, 2
	capped.Exp, capped.Level = MaxExperienceValue-5, 8
	fallen.Exp, fallen.Level = 190, 2
	fallen.SetStatus(party_state.Dead)

	gs.DungeonStates[3] = true
	award := gs.SettleDeedExperience()

	require.Equal(t, 25, award.Each)
	assert.Equal(t, uint16(215), climber.Exp)
	assert.Equal(t, uint16(MaxExperienceValue), capped.Exp)
	assert.Equal(t, uint16(215), fallen.Exp)
	assert.Equal(t, []ExperienceLevelReady{{Name: climber.GetNameAsString(), Level: 3}}, award.Ready,
		"only a living member crossing a level line is named")
	assert.Equal(t, byte(2), climber.Level, "the level itself still waits for Lord British's spirit")
}

func TestPaidDeedsSurviveASaveAndAnUnknownDeedIsRejected(t *testing.T) {
	gs := deedExperienceState(t, config.DeedExperienceLean)
	assert.Nil(t, gs.deedExperiencePaidKeys(), "no payments is the zero value")
	gs.DungeonStates[6] = true
	gs.DungeonStates[0] = true
	gs.SettleDeedExperience()
	keys := gs.deedExperiencePaidKeys()
	require.Equal(t, []string{"dungeon.deceit", "dungeon.hythloth"}, keys)

	restored := deedExperienceState(t, config.DeedExperienceLean)
	require.NoError(t, restored.restoreDeedExperiencePaid(keys))
	restored.DungeonStates[6], restored.DungeonStates[0] = true, true
	assert.False(t, restored.SettlePastDeedExperience().Paid())

	require.ErrorIs(t, restored.restoreDeedExperiencePaid([]string{"dungeon.atlantis"}), ErrUnknownDeed)
}

func TestKillExperienceGoesToTheKillerUnlessShared(t *testing.T) {
	gs := deedExperienceState(t, config.DeedExperienceNone)
	gs.CombatScene = &CombatState{}
	for index := 3; index < party_state.MaxActiveParty; index++ {
		if member := gs.PartyState.GetCharacter(index); member != nil {
			member.PartyMap = party_state.PartyMap(0xFF)
		}
	}
	first, second, third := gs.PartyState.GetCharacter(0), gs.PartyState.GetCharacter(1), gs.PartyState.GetCharacter(2)
	first.Exp, second.Exp, third.Exp = 100, 100, 100

	assert.Equal(t, 25, gs.awardKillExperience(second, 1, 25))
	assert.Equal(t, []uint16{100, 125, 100}, activeExperience(gs))
	summary := gs.combatExperienceSummary()
	assert.False(t, summary.Shared)
	assert.Equal(t, 25, summary.Total())
	assert.Equal(t, 25, summary.Members[1].Amount)

	gs.GameConfig.GameplayConfiguration.ShareKillExperience = true
	gs.CombatScene = &CombatState{}
	assert.Equal(t, 24, gs.awardKillExperience(second, 1, 25), "25 among three pays 8 each")
	assert.Equal(t, 1, gs.sharedExperienceRemainder, "and carries the odd point")
	assert.Equal(t, 6, gs.awardKillExperience(first, 0, 5), "which the next kill shares out")
	assert.Zero(t, gs.sharedExperienceRemainder)
	assert.Equal(t, []uint16{110, 135, 110}, activeExperience(gs))
	summary = gs.combatExperienceSummary()
	assert.True(t, summary.Shared)
	assert.Equal(t, 10, summary.Each)

	third.SetStatus(party_state.Dead)
	assert.Equal(t, 4, gs.awardKillExperience(first, 0, 4), "the dead take no share of a kill")
	summary = gs.combatExperienceSummary()
	assert.True(t, summary.Shared, "the living still earned the same")
	assert.Equal(t, 12, summary.Each)
	assert.Equal(t, 10, summary.Members[2].Amount)
}
