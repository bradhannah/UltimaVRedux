package main

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/conversation"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestGameActionCallbacksJoinPartyTypedFailures(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	for index := range party_state.MaxActiveParty {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapActive
	}
	source := map_units.NewNPCFriendly(references.NPCReference{Location: references.Britain}, 1)
	callbacks := NewGameActionCallbacks(&GameScene{gameState: state}, references.NPCReference{}, source)

	outcome, err := callbacks.JoinParty()
	require.NoError(t, err)
	assert.Equal(t, conversation.JoinPartyCapacityRefused, outcome)
	assert.Contains(t, callbacks.GetAccumulatedOutput(), "Thou hast no room")

	unexpectedCallbacks := NewGameActionCallbacks(
		&GameScene{gameState: state}, references.NPCReference{}, nil,
	)
	outcome, err = unexpectedCallbacks.JoinParty()
	require.Error(t, err)
	assert.Equal(t, conversation.JoinPartyUnexpectedError, outcome)
	assert.NotEmpty(t, unexpectedCallbacks.GetAccumulatedOutput())
}

func TestGameActionCallbacksDiscoverKnowledgeMapsSpellAndDialogueSource(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	callbacks := NewGameActionCallbacks(
		&GameScene{gameState: state},
		references.NPCReference{},
		nil,
	)
	callbacks.SetNPCName("Goeth")

	tests := []struct {
		name      string
		knowledge semantic.KnowledgeID
		spell     references.Spell
	}{
		{name: "An Ylem", knowledge: semantic.KnowledgeSpellAnYlem, spell: references.AnYlem},
		{name: "In Xen Mani", knowledge: semantic.KnowledgeSpellInXenMani, spell: references.InXenMani},
		{name: "Rel Xen Bet", knowledge: semantic.KnowledgeSpellRelXenBet, spell: references.RelXenBet},
	}

	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			require.NoError(t, callbacks.DiscoverKnowledge(test.knowledge))

			entry, found := state.SpellKnowledge(test.spell)
			require.True(t, found)
			assert.Equal(t, game_state.DialogueKnowledgeSource("Goeth"), entry.Source)
			assert.Len(t, state.KnowledgeSnapshot(), index+1)
		})
	}
}

func TestGameActionCallbacksDiscoverKnowledgeRejectsUnsupportedID(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	callbacks := NewGameActionCallbacks(
		&GameScene{gameState: state},
		references.NPCReference{},
		nil,
	)

	err := callbacks.DiscoverKnowledge(semantic.KnowledgeID("spell.in_lor"))

	assert.ErrorIs(t, err, errUnsupportedConversationKnowledge)
	assert.EqualError(t, err, `unsupported conversation knowledge "spell.in_lor"`)
	assert.Empty(t, state.KnowledgeSnapshot())
}

func TestGameActionCallbacksRoutesAllEightWordsOfPower(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	scene := &GameScene{gameState: state}
	_, scene.output = newMainOutput(1280)
	state.SystemCallbacks.UI.JournalChanged = scene.journalChanged
	callbacks := NewGameActionCallbacks(scene, references.NPCReference{}, nil)
	callbacks.SetNPCName("Word keeper")
	for index, test := range []struct {
		knowledge semantic.KnowledgeID
		dungeon   string
		word      string
	}{
		{semantic.KnowledgeDungeonDeceitWordOfPower, "deceit", "FALLAX"},
		{semantic.KnowledgeDungeonDespiseWordOfPower, "despise", "VILIS"},
		{semantic.KnowledgeDungeonDestardWordOfPower, "destard", "AIPONI"},
		{semantic.KnowledgeDungeonWrongWordOfPower, "wrong", "MALUM"},
		{semantic.KnowledgeDungeonCovetousWordOfPower, "covetous", "AVIDUS"},
		{semantic.KnowledgeDungeonShameWordOfPower, "shame", "INFAMA"},
		{semantic.KnowledgeDungeonHythlothWordOfPower, "hythloth", "IGNAVUS"},
		{semantic.KnowledgeDungeonDoomWordOfPower, "doom", "VERAMOCOR"},
	} {
		t.Run(test.dungeon, func(t *testing.T) {
			require.NoError(t, callbacks.DiscoverKnowledge(test.knowledge))
			entry, learned := state.WordOfPowerKnowledge(test.dungeon)
			require.True(t, learned)
			require.Equal(t, game_state.DialogueKnowledgeSource("Word keeper"), entry.Source)
			require.Len(t, state.QuestJournalEvents(), index+1)
			require.Equal(t, game_state.QuestJournalSubject(test.knowledge), state.QuestJournalEvents()[index].Subject)
			require.Contains(t, state.CurrentQuestJournalDossier().Chronicle[index].Summary, test.word)
			revision := scene.journalVisits.Revision
			scene.updateJournalNotice()
			history := scene.output.JournalHistory()
			require.Equal(t, index+1, strings.Count(history, "Journal updated."))
			require.NoError(t, callbacks.DiscoverKnowledge(test.knowledge))
			require.Equal(t, revision, scene.journalVisits.Revision)
			require.False(t, scene.journalNotice.pending)
			scene.updateJournalNotice()
			require.Equal(t, history, scene.output.JournalHistory(), "repeated knowledge must not replay its notice")
		})
	}
	before := state.KnowledgeSnapshot()
	for _, invalid := range []semantic.KnowledgeID{
		"dungeon.britain.word_of_power", "dungeon.Destard.word_of_power", "dungeon.destard",
		"dungeon.destard.mantra", "dungeon.destard.word_of_power.extra", "dungeon.aiponi.word_of_power",
	} {
		require.ErrorIs(t, callbacks.DiscoverKnowledge(invalid), errUnsupportedConversationKnowledge)
	}
	require.Equal(t, before, state.KnowledgeSnapshot())
	require.False(t, scene.journalNotice.pending)
}

func TestGameActionCallbacksDiscoverKnowledgeMapsAllLoreAndPreservesFirstMetadata(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	callbacks := NewGameActionCallbacks(
		&GameScene{gameState: state},
		references.NPCReference{},
		nil,
	)
	callbacks.SetNPCName("Archivist")
	discoveries := []semantic.KnowledgeID{
		semantic.KnowledgeShadowlordFaulineiName,
		semantic.KnowledgeShadowlordFaulineiAssociation,
		semantic.KnowledgeShadowlordAstarothName,
		semantic.KnowledgeShadowlordAstarothAssociation,
		semantic.KnowledgeShadowlordNosfentorName,
		semantic.KnowledgeShadowlordNosfentorAssociation,
		semantic.KnowledgeShrineHonestyMantra,
		semantic.KnowledgeShrineCompassionMantra,
		semantic.KnowledgeShrineValourMantra,
		semantic.KnowledgeShrineJusticeMantra,
		semantic.KnowledgeShrineSacrificeMantra,
		semantic.KnowledgeShrineHonorMantra,
		semantic.KnowledgeShrineSpiritualityMantra,
		semantic.KnowledgeShrineHumilityMantra,
	}

	for _, knowledge := range discoveries {
		t.Run(string(knowledge), func(t *testing.T) {
			before := state.KnowledgeSnapshot()
			require.NoError(t, callbacks.DiscoverKnowledge(knowledge))
			after := state.KnowledgeSnapshot()
			require.Len(t, after, len(before)+1)

			category, key, found := strings.Cut(string(knowledge), ".")
			require.True(t, found)
			var entry game_state.KnowledgeEntry
			for _, candidate := range after {
				if candidate.Key == (game_state.KnowledgeKey{Category: game_state.KnowledgeCategory(category), ID: key}) {
					entry = candidate
					break
				}
			}
			require.NotEmpty(t, entry.Key.ID)
			assert.Equal(t, game_state.DialogueKnowledgeSource("Archivist"), entry.Source)
			assert.NotNil(t, entry.DiscoveredOn)
			assert.False(t, entry.Unread)
			assert.Nil(t, entry.SpellFormula)

			callbacks.SetNPCName("Later speaker")
			require.NoError(t, callbacks.DiscoverKnowledge(knowledge))
			assert.Equal(t, after, state.KnowledgeSnapshot())
			callbacks.SetNPCName("Archivist")
		})
	}
}

func TestGameActionCallbacksGoToJailDelegatesToCanonicalSentence(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 7
	state.DateTime.Minute = 55
	state.PartyState.Inventory.Provisions.Keys.Set(4)
	state.SetCriminalStatus(true)
	callbacks := NewGameActionCallbacks(
		&GameScene{gameState: state},
		references.NPCReference{},
		nil,
	)
	callbacks.SetDialogOutput(text.NewOutput(nil, 1, 64, 128))

	callbacks.GoToJail()

	assert.Equal(t, references.Yew, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.Position{X: references.JailPositionX, Y: references.JailPositionY},
		state.MapState.PlayerLocation.Position)
	assert.Equal(t, uint8(references.JailReleaseHour), state.DateTime.Hour)
	assert.Equal(t, uint8(15), state.DateTime.Minute)
	assert.Zero(t, state.PartyState.Inventory.Provisions.Keys.Get())
	assert.False(t, state.IsCriminal())
}

func TestGameActionCallbacksCallGuardsOwnsCriminalStatusAndContinuesOnActivationError(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.CurrentNPCAIController = nil
	callbacks := NewGameActionCallbacks(
		&GameScene{gameState: state},
		references.NPCReference{},
		nil,
	)

	assert.NotPanics(t, callbacks.CallGuards)
	assert.True(t, state.IsCriminal(), "criminal status is caller-owned and precedes guard activation")
}

func newGameActionCallbacksKnowledgeState(t *testing.T) *game_state.GameState {
	t.Helper()
	snapshot := validCommandTestSnapshot()
	snapshot.World.Position = saveSnapshotLargeMapPosition(10, 20)
	snapshot.World.LastLargeMapPosition = snapshot.World.Position
	snapshot.Wind.NextTickMs = environment.WindTickIntervalMs
	gameReferences := &references.GameReferences{
		OverworldLargeMapReference:  &references.LargeMapReference{},
		UnderworldLargeMapReference: &references.LargeMapReference{},
		LocationReferences:          &references.LocationReferences{},
		EnemyReferences:             &references.EnemyReferences{},
	}
	state, err := game_state.NewGameStateFromSaveSnapshot(
		snapshot,
		&config.UltimaVConfiguration{},
		gameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)

	state.SystemCallbacks = game_state.NewMockSystemCallbacks(t).ToSystemCallbacks()
	return state
}

func saveSnapshotLargeMapPosition(x, y int) save_snapshot.PositionSnapshot {
	return save_snapshot.PositionSnapshot{
		Location: int(references.BritanniaUnderworld),
		X:        x,
		Y:        y,
		Floor:    int(references.OVERWORLD),
	}
}
