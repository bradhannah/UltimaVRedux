package semantic_test

import (
	"encoding/json"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const minimalValidYAML = `schema: ultima5redux.conversation/v1
protocol: ultima5
id: redux.test_keeper
origin: {kind: redux}
name: Keeper
description: A keeper waits beside an empty shelf.
greeting: Welcome.
job: I keep the shelf in order.
farewell: Farewell.
topics:
  - keywords: [shelf]
    response:
      - ask: question_01
questions:
  question_01:
    prompt: Is the shelf empty?
    answers:
      - keywords: [yes]
        response:
          - say: So it is.
          - return_to_topics
    default: I did not understand.
`

func TestLoadCanonicalFixture(t *testing.T) {
	t.Parallel()

	conversation, err := semantic.LoadFile("testdata/canonical_valid.yaml")
	require.NoError(t, err)
	assert.Equal(t, semantic.SchemaVersion, conversation.Schema)
	assert.Equal(t, "redux.mara_cartographer", conversation.ID)
	assert.IsType(t, &semantic.ReduxOrigin{}, conversation.Origin)
	require.Len(t, conversation.Greeting, 1)
	say, ok := conversation.Greeting[0].(*semantic.SayAction)
	require.True(t, ok)
	require.Len(t, say.Content, 3)
	assert.IsType(t, &semantic.LineBreakPart{}, say.Content[1])
	assert.Len(t, conversation.Topics, 2)
	assert.Contains(t, conversation.Questions, "question_01")
}

func TestLoadOriginalOriginAndShorthand(t *testing.T) {
	t.Parallel()

	document := strings.Replace(minimalValidYAML, "origin: {kind: redux}", "origin: {kind: original, source: towne, dialog: 12}", 1)
	conversation, err := semantic.Load("original.yaml", strings.NewReader(document))
	require.NoError(t, err)
	origin, ok := conversation.Origin.(*semantic.OriginalOrigin)
	require.True(t, ok)
	assert.Equal(t, semantic.OriginalSourceTowne, origin.Source)
	assert.Equal(t, 12, origin.Dialog)
	assert.IsType(t, &semantic.SayAction{}, conversation.Name[0])
	assert.IsType(t, &semantic.NarrateAction{}, conversation.Description[0])
}

func TestLoadDiscoverKnowledge(t *testing.T) {
	t.Parallel()

	document := strings.Replace(
		minimalValidYAML,
		"          - return_to_topics",
		"          - discover_knowledge: spell.an_ylem\n          - return_to_topics",
		1,
	)
	conversation, err := semantic.Load("knowledge.yaml", strings.NewReader(document))
	require.NoError(t, err)
	response := conversation.Questions["question_01"].Answers[0].Response
	require.Len(t, response, 3)
	discovery, ok := response[1].(*semantic.DiscoverKnowledgeAction)
	require.True(t, ok)
	assert.Equal(t, semantic.KnowledgeSpellAnYlem, discovery.Knowledge)
}

func TestLoadJournalKnowledgeScalarKeys(t *testing.T) {
	for _, id := range []string{
		"journal.domain.falsehood", "journal.domain.hatred", "journal.domain.cowardice",
		"journal.flame.truth", "journal.flame.love", "journal.flame.courage", "journal.banishment.method",
		"dungeon.deceit.word_of_power", "dungeon.despise.word_of_power", "dungeon.destard.word_of_power",
		"dungeon.wrong.word_of_power", "dungeon.covetous.word_of_power", "dungeon.shame.word_of_power",
		"dungeon.hythloth.word_of_power", "dungeon.doom.word_of_power",
	} {
		t.Run(id, func(t *testing.T) {
			document := strings.Replace(minimalValidYAML, "          - return_to_topics",
				"          - discover_knowledge: "+id+"\n          - return_to_topics", 1)
			conversation, err := semantic.Load("knowledge.yaml", strings.NewReader(document))
			require.NoError(t, err)
			serialized, err := semantic.Marshal(conversation)
			require.NoError(t, err)
			assert.Contains(t, string(serialized), "discover_knowledge: "+id)
		})
	}
}

func TestLoadRejectsNilReader(t *testing.T) {
	t.Parallel()

	conversation, err := semantic.Load("nil.yaml", nil)
	assert.Nil(t, conversation)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "nil.yaml")
	assert.Contains(t, err.Error(), "reader is nil")
}

func TestLoadRejectsUnknownWordOfPowerKnowledge(t *testing.T) {
	for _, id := range []string{"dungeon.Destard.word_of_power", "dungeon.britain.word_of_power", "dungeon.destard.word", "dungeon.inopia.word_of_power"} {
		document := strings.Replace(minimalValidYAML, "          - return_to_topics",
			"          - discover_knowledge: "+id+"\n          - return_to_topics", 1)
		_, err := semantic.Load("knowledge.yaml", strings.NewReader(document))
		require.Error(t, err, id)
	}
}

func TestLoadRejectsUnsafeOrAmbiguousYAML(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		document string
		message  string
	}{
		{
			name:     "duplicate mapping key",
			document: strings.Replace(minimalValidYAML, "protocol: ultima5", "protocol: ultima5\nprotocol: ultima5", 1),
			message:  "duplicate mapping key",
		},
		{
			name:     "anchor",
			document: strings.Replace(minimalValidYAML, "name: Keeper", "name: &shared Keeper", 1),
			message:  "anchors are not allowed",
		},
		{
			name:     "alias",
			document: strings.Replace(minimalValidYAML, "name: Keeper", "name: &shared Keeper\ngreeting: *shared", 1),
			message:  "anchors are not allowed",
		},
		{
			name:     "merge key",
			document: strings.Replace(minimalValidYAML, "origin: {kind: redux}", "origin: {<<: {kind: redux}}", 1),
			message:  "merge keys are not allowed",
		},
		{
			name:     "custom tag",
			document: strings.Replace(minimalValidYAML, "name: Keeper", "name: !dialogue Keeper", 1),
			message:  "custom YAML tags are not allowed",
		},
		{
			name:     "multiple documents",
			document: minimalValidYAML + "---\n" + minimalValidYAML,
			message:  "multiple YAML documents are not allowed",
		},
		{
			name:     "null fixed entry",
			document: strings.Replace(minimalValidYAML, "greeting: Welcome.", "greeting: null", 1),
			message:  "$.greeting",
		},
		{
			name:     "non-string scalar",
			document: strings.Replace(minimalValidYAML, "name: Keeper", "name: true", 1),
			message:  "expected a string",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			conversation, err := semantic.Load("unsafe.yaml", strings.NewReader(testCase.document))
			assert.Nil(t, conversation)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "unsafe.yaml")
			assert.Contains(t, err.Error(), testCase.message)
		})
	}
}

func TestLoadRejectsUnknownVocabulary(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		document string
		message  string
	}{
		{
			name:     "top-level field",
			document: strings.Replace(minimalValidYAML, "id: redux.test_keeper", "id: redux.test_keeper\nmusic: silence", 1),
			message:  `unknown field "music"`,
		},
		{
			name:     "action",
			document: strings.Replace(minimalValidYAML, "      - ask: question_01", "      - teleport: elsewhere", 1),
			message:  `unknown action "teleport"`,
		},
		{
			name:     "inline part",
			document: strings.Replace(minimalValidYAML, "greeting: Welcome.", "greeting:\n  - say:\n      - emphasis: Welcome.", 1),
			message:  `unknown inline part "emphasis"`,
		},
		{
			name:     "condition",
			document: strings.Replace(minimalValidYAML, "      - ask: question_01", "      - branch:\n          condition: has_key\n          then: []", 1),
			message:  `unknown bare condition "has_key"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := semantic.Load("vocabulary.yaml", strings.NewReader(testCase.document))
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.message)
		})
	}
}

func TestLoadRejectsInvalidDocumentContract(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		document string
		message  string
	}{
		{
			name:     "missing required field",
			document: strings.Replace(minimalValidYAML, "job: I keep the shelf in order.\n", "", 1),
			message:  `required field "job" is missing`,
		},
		{
			name:     "unsupported schema",
			document: strings.Replace(minimalValidYAML, semantic.SchemaVersion, "ultima5redux.conversation/v2", 1),
			message:  `must equal "ultima5redux.conversation/v1"`,
		},
		{
			name:     "unsupported protocol",
			document: strings.Replace(minimalValidYAML, "protocol: ultima5", "protocol: another_game", 1),
			message:  `must equal "ultima5"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := semantic.Load("contract.yaml", strings.NewReader(testCase.document))
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.message)
		})
	}
}

func TestQuestionNameBoundaries(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		valid    bool
		question string
	}{
		{name: "zero", valid: false, question: "question_00"},
		{name: "first", valid: true, question: "question_01"},
		{name: "last", valid: true, question: "question_15"},
		{name: "above range", valid: false, question: "question_16"},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			document := strings.ReplaceAll(minimalValidYAML, "question_01", testCase.question)
			_, err := semantic.Load("question-boundary.yaml", strings.NewReader(document))
			if testCase.valid {
				require.NoError(t, err)

				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), "name must be question_01 through question_15")
		})
	}
}

func TestStableIDValidation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		id    string
		valid bool
	}{
		{id: "original.a", valid: true},
		{id: "original.towne_12", valid: true},
		{id: "redux.a", valid: true},
		{id: "redux.test-keeper.v2", valid: true},
		{id: "test_keeper", valid: false},
		{id: "original.", valid: false},
		{id: "redux._keeper", valid: false},
		{id: "redux.Keeper", valid: false},
		{id: "Redux.keeper", valid: false},
		{id: "redux.keeper/name", valid: false},
		{id: " redux.keeper", valid: false},
	}

	for _, testCase := range testCases {
		t.Run(testCase.id, func(t *testing.T) {
			t.Parallel()
			document := strings.Replace(minimalValidYAML, "redux.test_keeper", strconv.Quote(testCase.id), 1)
			_, err := semantic.Load("stable-id.yaml", strings.NewReader(document))
			if testCase.valid {
				require.NoError(t, err)

				return
			}
			require.Error(t, err)
			assert.Contains(t, err.Error(), `must match ^(original|redux)\.[a-z0-9][a-z0-9_.-]*$`)
		})
	}
}

func TestLoadRejectsSemanticErrors(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name     string
		document string
		message  string
	}{
		{
			name:     "unknown substitution",
			document: strings.Replace(minimalValidYAML, "greeting: Welcome.", "greeting: Welcome, {player_name}.", 1),
			message:  `unknown substitution "player_name"`,
		},
		{
			name:     "unknown ask target",
			document: strings.Replace(minimalValidYAML, "ask: question_01", "ask: question_02", 1),
			message:  `references unknown question "question_02"`,
		},
		{
			name:     "terminal action is not last",
			document: strings.Replace(minimalValidYAML, "      - ask: question_01", "      - ask: question_01\n      - say: Too late.", 1),
			message:  "terminal action",
		},
		{
			name:     "invalid question name",
			document: strings.ReplaceAll(minimalValidYAML, "question_01", "question_shelf"),
			message:  "name must be question_01 through question_15",
		},
		{
			name:     "duplicate normalized keywords in entry",
			document: strings.Replace(minimalValidYAML, "keywords: [shelf]", `keywords: [shelf, " SHELF "]`, 1),
			message:  `duplicate normalized keyword "shelf"`,
		},
		{
			name:     "duplicate normalized keywords across ordered topics",
			document: strings.Replace(minimalValidYAML, "questions:\n", "  - keywords: [SHELF]\n    response: Another shelf.\nquestions:\n", 1),
			message:  `duplicate normalized keyword "shelf"`,
		},
		{
			name: "duplicate normalized keywords across ordered answers",
			document: strings.Replace(
				minimalValidYAML,
				"    default: I did not understand.",
				"      - keywords: [YES]\n        response: It remains so.\n    default: I did not understand.",
				1,
			),
			message: `duplicate normalized keyword "yes"`,
		},
		{
			name:     "karma bound",
			document: strings.Replace(minimalValidYAML, "      - ask: question_01", "      - branch:\n          condition: {karma_at_least: 100}\n          then: []", 1),
			message:  "must be between 0 and 99",
		},
		{
			name:     "gold bound",
			document: strings.Replace(minimalValidYAML, "          - say: So it is.", "          - take_gold: 0", 1),
			message:  "must be between 1 and 65535",
		},
		{
			name:     "original dialog bound",
			document: strings.Replace(minimalValidYAML, "origin: {kind: redux}", "origin: {kind: original, source: keep, dialog: 0}", 1),
			message:  "must be at least 1",
		},
		{
			name:     "say is one paragraph",
			document: strings.Replace(minimalValidYAML, "greeting: Welcome.", "greeting: |\n  First line.\n  Second line.\n", 1),
			message:  "text must be one paragraph",
		},
		{
			name:     "unknown knowledge id",
			document: strings.Replace(minimalValidYAML, "          - return_to_topics", "          - discover_knowledge: spell.unknown\n          - return_to_topics", 1),
			message:  `unknown knowledge id "spell.unknown"`,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			_, err := semantic.Load("semantic.yaml", strings.NewReader(testCase.document))
			require.Error(t, err)
			assert.Contains(t, err.Error(), testCase.message)
			assert.Contains(t, err.Error(), "line")
		})
	}
}

func TestEmbeddedSchemaVocabularyMatchesGoModel(t *testing.T) {
	t.Parallel()

	var schemaDocument map[string]any
	require.NoError(t, json.Unmarshal(semantic.JSONSchema(), &schemaDocument))
	definitions := schemaDocument["$defs"].(map[string]any)
	stableIDDefinition := definitions["stableID"].(map[string]any)
	assert.Equal(t, `^(original|redux)\.[a-z0-9][a-z0-9_.-]*$`, stableIDDefinition["pattern"])
	stableIDPattern := regexp.MustCompile(stableIDDefinition["pattern"].(string))
	assert.True(t, stableIDPattern.MatchString("original.a"))
	assert.True(t, stableIDPattern.MatchString("redux.test-keeper.v2"))
	assert.False(t, stableIDPattern.MatchString("redux.Keeper"))
	assert.False(t, stableIDPattern.MatchString("other.keeper"))
	properties := schemaDocument["properties"].(map[string]any)
	questions := properties["questions"].(map[string]any)
	questionNames := questions["propertyNames"].(map[string]any)
	assert.Equal(t, `^question_(0[1-9]|1[0-5])$`, questionNames["pattern"])
	questionPattern := regexp.MustCompile(questionNames["pattern"].(string))
	assert.False(t, questionPattern.MatchString("question_00"))
	assert.True(t, questionPattern.MatchString("question_01"))
	assert.True(t, questionPattern.MatchString("question_15"))
	assert.False(t, questionPattern.MatchString("question_16"))

	actionDefinition := definitions["action"].(map[string]any)
	variants := actionDefinition["oneOf"].([]any)
	actionNames := make([]string, 0, len(variants))
	for _, rawVariant := range variants {
		variant := rawVariant.(map[string]any)
		if constant, exists := variant["const"]; exists {
			actionNames = append(actionNames, constant.(string))
			continue
		}
		properties := variant["properties"].(map[string]any)
		for name := range properties {
			actionNames = append(actionNames, name)
		}
	}
	sort.Strings(actionNames)

	expected := []string{
		string(semantic.ActionKindAdjustKarma),
		string(semantic.ActionKindAsk),
		string(semantic.ActionKindAskName),
		string(semantic.ActionKindBeginCircleStudy),
		string(semantic.ActionKindBranch),
		string(semantic.ActionKindCallGuards),
		string(semantic.ActionKindDiscoverKnowledge),
		string(semantic.ActionKindEnd),
		string(semantic.ActionKindGiveItem),
		string(semantic.ActionKindGoToJail),
		string(semantic.ActionKindJoinParty),
		string(semantic.ActionKindNarrate),
		string(semantic.ActionKindPause),
		string(semantic.ActionKindReturnToTopics),
		string(semantic.ActionKindSay),
		string(semantic.ActionKindTakeGold),
		string(semantic.ActionKindWaitForKey),
	}
	sort.Strings(expected)
	assert.Equal(t, expected, actionNames)
	knowledgeDefinition := definitions["knowledgeID"].(map[string]any)
	assert.Equal(t, []any{
		string(semantic.KnowledgeSpellAnYlem),
		string(semantic.KnowledgeSpellInXenMani),
		string(semantic.KnowledgeSpellRelXenBet),
		string(semantic.KnowledgeShadowlordFaulineiName),
		string(semantic.KnowledgeShadowlordFaulineiAssociation),
		string(semantic.KnowledgeShadowlordAstarothName),
		string(semantic.KnowledgeShadowlordAstarothAssociation),
		string(semantic.KnowledgeShadowlordNosfentorName),
		string(semantic.KnowledgeShadowlordNosfentorAssociation),
		string(semantic.KnowledgeShrineHonestyMantra),
		string(semantic.KnowledgeShrineCompassionMantra),
		string(semantic.KnowledgeShrineValourMantra),
		string(semantic.KnowledgeShrineJusticeMantra),
		string(semantic.KnowledgeShrineSacrificeMantra),
		string(semantic.KnowledgeShrineHonorMantra),
		string(semantic.KnowledgeShrineSpiritualityMantra),
		string(semantic.KnowledgeShrineHumilityMantra),
		string(semantic.KnowledgeJournalDomainFalsehood),
		string(semantic.KnowledgeJournalDomainHatred),
		string(semantic.KnowledgeJournalDomainCowardice),
		string(semantic.KnowledgeJournalFlameTruth),
		string(semantic.KnowledgeJournalFlameLove),
		string(semantic.KnowledgeJournalFlameCourage),
		string(semantic.KnowledgeJournalBanishmentMethod),
		string(semantic.KnowledgeJournalResistancePassword),
		string(semantic.KnowledgeJournalOppressionPassword),
		string(semantic.KnowledgeJournalStonesMelody),
		string(semantic.KnowledgeDungeonDeceitWordOfPower),
		string(semantic.KnowledgeDungeonDespiseWordOfPower),
		string(semantic.KnowledgeDungeonDestardWordOfPower),
		string(semantic.KnowledgeDungeonWrongWordOfPower),
		string(semantic.KnowledgeDungeonCovetousWordOfPower),
		string(semantic.KnowledgeDungeonShameWordOfPower),
		string(semantic.KnowledgeDungeonHythlothWordOfPower),
		string(semantic.KnowledgeDungeonDoomWordOfPower),
	}, knowledgeDefinition["enum"])
}
