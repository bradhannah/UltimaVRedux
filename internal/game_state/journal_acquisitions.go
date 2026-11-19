package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

func (g *GameState) recordItemAcquired(key string) {
	// REDUX DEVIATION: ground item collection grants the item and prints its identity;
	// Redux additionally retains a first-acquisition reference milestone.
	// See ref:QUEST-0002.
	date := g.DateTime.DateOnly()
	g.appendLiveJournalEvent(QuestJournalEvent{
		Sequence: 0, Kind: QuestJournalEventItemAcquired, Subject: QuestJournalSubject(key),
		OccurredOn: &date, Source: QuestJournalSourceGameplay,
		SourceName: g.MapState.PlayerLocation.Location.String(), Acknowledged: false,
	})
}

func (g *GameState) recordQuestItemAcquired(item references.QuestItem) {
	keys := map[references.QuestItem]string{
		references.Crown: "crown", references.Sceptre: "sceptre", references.Amulet: "amulet",
		references.ShardOfFalsehood: "shard.falsehood", references.ShardOfHatred: "shard.hatred",
		references.ShardOfCowardice: "shard.cowardice", references.WoodenBox: "sandalwood-box",
	}
	if key, ok := keys[item]; ok {
		g.recordItemAcquired(key)
		// The pickup message is already written, so the award follows it.
		if award := g.settleDeedExperience(false); award.Paid() {
			g.SystemCallbacks.UI.ShowExperienceAward(award)
		}
	}
}
