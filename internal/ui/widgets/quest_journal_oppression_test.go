package widgets

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/stretchr/testify/require"
)

func TestQuestJournalOppressionSearchRequiresDiscovery(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.SkaraBrae).WithSystemCallbacks().Build()
	config := testQuestJournalConfig(newWidgetTestKeyboard(0))
	dialog := NewQuestJournalDialog(state.CurrentQuestJournalDossier(), config)
	for _, query := range []string{"Oppression", "IMPERA", "Flain"} {
		dialog.beginSearch()
		dialog.setSearchQuery(query)
		require.Empty(t, dialog.searchResults, query)
	}
	require.True(t, state.DiscoverJournalKnowledge("oppression.password", game_state.DialogueKnowledgeSource("Flain")))
	dialog = NewQuestJournalDialog(state.CurrentQuestJournalDossier(), config)
	for _, query := range []string{"Oppression", "IMPERA", "Flain", "Flain IMPERA"} {
		dialog.beginSearch()
		dialog.setSearchQuery(query)
		require.Len(t, dialog.searchResults, 1, query)
		dialog.openSearchResult()
		require.Equal(t, QuestJournalChronicleTab, dialog.GetActiveTab())
		require.Equal(t, 0, dialog.GetSelectedIndex())
	}
}
