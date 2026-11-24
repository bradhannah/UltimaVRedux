package widgets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/stretchr/testify/require"
)

func TestQuestJournalCompleteCodexSearchAndPaging(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithPlayerAt(10, 20).
		WithSystemCallbacks().Build()
	state.DiscoverWordOfPowerKnowledge("doom", game_state.RevelationKnowledgeSource("Codex of Ultimate Wisdom"))
	partial := NewQuestJournalDialog(state.CurrentQuestJournalDossier(), testQuestJournalConfig(newWidgetTestKeyboard(0)))
	for query, tabs := range map[string][]QuestJournalTab{
		"Shame egress": nil,
		"Final Gate":   {QuestJournalDungeonsTab},
		"VERAMOCOR":    {QuestJournalChronicleTab, QuestJournalDungeonsTab},
		"Codex":        {QuestJournalChronicleTab, QuestJournalDungeonsTab},
	} {
		require.Equal(t, tabs, codexSearchResultTabs(partial, query), "word knowledge must not disclose the complete clue: %s", query)
	}
	partial.beginSearch()
	partial.setSearchQuery("Final Gate")
	partial.openSearchResult()
	require.Equal(t, QuestJournalDungeonsTab, partial.GetActiveTab())
	gate := codexRenderedDetail(partial)
	require.Contains(t, gate, "Final Gate Word of Power: VERAMOCOR Source: Codex of Ultimate Wisdom")
	require.NotContains(t, gate, "Entrance", "an unobserved gate has no entrance facts")
	require.NotContains(t, gate, "Location")

	require.True(t, state.RecordCompleteCodexRevelation())
	for _, size := range [][2]int{{1280, 720}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		t.Run(fmt.Sprintf("%dx%d", size[0], size[1]), func(t *testing.T) {
			setQuestJournalTestResolution(t, size[0], size[1])
			dialog := NewQuestJournalDialog(state.CurrentQuestJournalDossier(), testQuestJournalConfig(newWidgetTestKeyboard(0)))
			for _, query := range []string{"Shame", "Underworld", "darkness", "gate", "VERAMOCOR", "stealers souls", "awaits coming"} {
				require.Equal(t, []QuestJournalTab{QuestJournalChronicleTab, QuestJournalDungeonsTab},
					codexSearchResultTabs(dialog, query), query)
				for result := range 2 {
					dialog.beginSearch()
					dialog.setSearchQuery(query)
					dialog.searchSelection = result
					dialog.openSearchResult()
					require.Zero(t, dialog.GetSelectedIndex())
					page := dialog.detailPages()[dialog.detailPage]
					found := false
					for _, line := range page {
						found = found || journalHasMatch(line.value, query)
					}
					require.True(t, found, "%s opens the page holding its match", query)
				}
			}
			for _, tab := range []QuestJournalTab{QuestJournalChronicleTab, QuestJournalDungeonsTab} {
				dialog.activeTab = tab
				dialog.cachedDetailPages = nil
				require.Contains(t, codexRenderedDetail(dialog), "awaits thy coming.")
			}
			for _, hidden := range []string{"Doom", "dungeon.doom.word_of_power"} {
				require.Empty(t, codexSearchResultTabs(dialog, hidden), hidden)
			}
		})
	}
}

func codexSearchResultTabs(dialog *QuestJournalDialog, query string) []QuestJournalTab {
	dialog.beginSearch()
	dialog.setSearchQuery(query)
	var tabs []QuestJournalTab
	for _, result := range dialog.searchResults {
		tabs = append(tabs, dialog.searchEntries[result].tab)
	}
	dialog.leaveSearch()

	return tabs
}

func codexRenderedDetail(dialog *QuestJournalDialog) string {
	var rendered []string
	for _, page := range dialog.detailPages() {
		for _, line := range page {
			if line.label != "" {
				rendered = append(rendered, line.label+": "+line.value)
			} else if line.value != "" {
				rendered = append(rendered, line.value)
			}
		}
	}

	return strings.Join(rendered, " ")
}
