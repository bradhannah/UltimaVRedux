package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestRecordShadowlordDefeatEventIsExactlyOnceAndDetached(t *testing.T) {
	state := &GameState{
		DateTime:        datetime.UltimaDate{Year: 139, Month: 13, Day: 28},
		ShadowlordState: references.NewShadowlordState(),
		questJournal:    newQuestJournal(),
		SystemCallbacks: journalTestCallbacks(),
	}
	require.True(t, state.RecordShadowlordDefeatEvent(references.Astaroth))

	first := state.QuestJournalEvents()
	require.Len(t, first, 1)
	assert.Equal(t, uint64(1), first[0].Sequence)
	assert.Equal(t, QuestJournalEventShadowlordDefeated, first[0].Kind)
	assert.Equal(t, QuestJournalSubjectAstaroth, first[0].Subject)
	assert.Equal(t, QuestJournalSourceGameplay, first[0].Source)
	assert.False(t, first[0].Acknowledged)
	require.NotNil(t, first[0].OccurredOn)
	assert.Equal(t, datetime.BritannianDate{Year: 139, Month: 13, Day: 28}, *first[0].OccurredOn)

	state.DateTime.Day = 1
	assert.False(t, state.RecordShadowlordDefeatEvent(references.Astaroth))
	assert.False(t, state.RecordShadowlordDefeatEvent(references.NoShadowlord))
	first[0].OccurredOn.Day = 2

	retained := state.QuestJournalEvents()
	require.Len(t, retained, 1)
	assert.Equal(t, byte(28), retained[0].OccurredOn.Day)
}

func TestCurrentQuestJournalDossierPreservesAuthoritativeProgression(t *testing.T) {
	state := &GameState{
		DateTime:        datetime.UltimaDate{Year: 139, Month: 1, Day: 1},
		ShadowlordState: references.NewShadowlordState(),
		playerKnowledge: newPlayerKnowledge(),
		questJournal:    newQuestJournal(),
		SystemCallbacks: journalTestCallbacks(),
	}
	state.SetShrineQuestActive(1, true)
	state.SetShrineRuined(1, true)
	state.SetShrineQuestCompleted(2, true)
	state.SetShrineQuestActive(2, true)
	state.SetShrineQuestCompleted(3, true)

	// Chronicle metadata is deliberately stale relative to living Faulinei.
	require.True(t, state.RecordShadowlordDefeatEvent(references.Faulinei))
	state.ShadowlordState.Kill(references.Astaroth)

	dossier := state.CurrentQuestJournalDossier()
	require.Len(t, dossier.Chronicle, 1)
	require.Len(t, dossier.Virtues, TotalShrines)
	require.Len(t, dossier.Shadowlords, references.ShadowlordCount)

	assert.Equal(t, ShrineProgressNotAssigned, dossier.Virtues[0].QuestState)
	assert.Equal(t, ShrineProgressAssigned, dossier.Virtues[1].QuestState)
	assert.True(t, dossier.Virtues[1].Ruined)
	assert.False(t, dossier.Virtues[1].MantraLearned)
	assert.Empty(t, dossier.Virtues[1].Mantra)
	assert.False(t, dossier.Virtues[1].CodexAnswerLearned)
	assert.Empty(t, dossier.Virtues[1].CodexAnswer)
	assert.Equal(t, ShrineProgressReadyToComplete, dossier.Virtues[2].QuestState)
	assert.True(t, dossier.Virtues[2].CodexAnswerLearned)
	assert.Equal(t, allShrines[2].AnswerText(), dossier.Virtues[2].CodexAnswer)
	assert.Equal(t, ShrineProgressOffering, dossier.Virtues[3].QuestState)
	assert.Equal(t, allShrines[3].AnswerText(), dossier.Virtues[3].CodexAnswer)

	assert.Equal(t, QuestJournalShadowlordLiving, dossier.Shadowlords[references.Faulinei].State)
	assert.Equal(t, QuestJournalShadowlordDefeated, dossier.Shadowlords[references.Astaroth].State)
	assert.Equal(t, QuestJournalShadowlordLiving, dossier.Shadowlords[references.Nosfentor].State)
	assert.Equal(t, "Shadowlord I", dossier.Shadowlords[references.Faulinei].DisplayName)
	assert.Equal(t, "Shadowlord II", dossier.Shadowlords[references.Astaroth].DisplayName)
	assert.Equal(t, "Shadowlord III", dossier.Shadowlords[references.Nosfentor].DisplayName)
	assert.Empty(t, dossier.Shadowlords[references.Faulinei].Association)
	assert.Equal(t, "Shadowlord I", dossier.Chronicle[0].DisplayName)
}

func TestCurrentQuestJournalDossierRedactsIndependentLoreAndReprojectsChronicle(t *testing.T) {
	state := &GameState{
		DateTime:        datetime.UltimaDate{Year: 139, Month: 13, Day: 28},
		ShadowlordState: references.NewShadowlordState(),
		playerKnowledge: newPlayerKnowledge(),
		questJournal:    newQuestJournal(),
		SystemCallbacks: journalTestCallbacks(),
	}
	require.True(t, state.RecordShadowlordDefeatEvent(references.Astaroth))
	assert.Equal(t, "Shadowlord II", state.CurrentQuestJournalDossier().Chronicle[0].DisplayName)

	source := DialogueKnowledgeSource("Archivist")
	require.True(t, state.DiscoverShadowlordKnowledge(references.Faulinei, ShadowlordKnowledgeAssociation, source))
	require.True(t, state.DiscoverShadowlordKnowledge(references.Astaroth, ShadowlordKnowledgeName, source))
	require.True(t, state.DiscoverShadowlordKnowledge(references.Nosfentor, ShadowlordKnowledgeName, source))
	require.True(t, state.DiscoverShadowlordKnowledge(references.Nosfentor, ShadowlordKnowledgeAssociation, source))
	require.True(t, state.DiscoverShrineMantraKnowledge(4, source))

	dossier := state.CurrentQuestJournalDossier()
	assert.Equal(t, "Shadowlord I", dossier.Shadowlords[0].DisplayName)
	assert.Equal(t, "Falsehood", dossier.Shadowlords[0].Association)
	assert.Equal(t, "Astaroth", dossier.Shadowlords[1].DisplayName)
	assert.Empty(t, dossier.Shadowlords[1].Association)
	assert.Equal(t, "Nosfentor", dossier.Shadowlords[2].DisplayName)
	assert.Equal(t, "Cowardice", dossier.Shadowlords[2].Association)
	assert.Equal(t, "Astaroth", dossier.Chronicle[0].DisplayName, "older events reproject using current knowledge")
	assert.True(t, dossier.Virtues[4].MantraLearned)
	assert.Equal(t, "CAH", dossier.Virtues[4].Mantra)
	assert.False(t, dossier.Virtues[3].MantraLearned)
	assert.Empty(t, dossier.Virtues[3].Mantra)

	dossier.Chronicle[0].DisplayName = "Changed"
	dossier.Chronicle[0].OccurredOn.Day = 1
	dossier.Shadowlords[0].Association = "Changed"
	dossier.Virtues[4].Mantra = "Changed"
	retained := state.CurrentQuestJournalDossier()
	assert.Equal(t, "Astaroth", retained.Chronicle[0].DisplayName)
	assert.Equal(t, byte(28), retained.Chronicle[0].OccurredOn.Day)
	assert.Equal(t, "Falsehood", retained.Shadowlords[0].Association)
	assert.Equal(t, "CAH", retained.Virtues[4].Mantra)
}
