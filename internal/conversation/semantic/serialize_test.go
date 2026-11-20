package semantic_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMarshalRoundTripAllVariants(t *testing.T) {
	t.Parallel()

	conversation := comprehensiveConversation(false)
	data, err := semantic.Marshal(conversation)
	require.NoError(t, err)
	require.NotEmpty(t, data)
	assert.Equal(t, 1, strings.Count(string(data), "question_01:"))
	assert.Less(t, strings.Index(string(data), "question_01:"), strings.Index(string(data), "question_15:"))
	assert.NotContains(t, string(data), "\r")
	assert.True(t, bytes.HasSuffix(data, []byte("\n")))
	assert.False(t, bytes.HasSuffix(data, []byte("\n\n")))
	assert.Contains(t, string(data), "name: Orin\n")
	assert.Contains(t, string(data), "description: A methodical keeper records invented routes.\n")
	assert.Contains(t, string(data), "farewell: []\n")
	assert.Contains(t, string(data), "  - pause\n")
	assert.Contains(t, string(data), "  - discover_knowledge: spell.an_ylem\n")
	assert.NotContains(t, string(data), "$schema")

	loaded, err := semantic.Load("round-trip.yaml", bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, conversation, loaded)

	second, err := semantic.Marshal(loaded)
	require.NoError(t, err)
	assert.Equal(t, data, second)
}

func TestMarshalDeterministicAcrossQuestionMapOrder(t *testing.T) {
	t.Parallel()

	first, err := semantic.Marshal(comprehensiveConversation(false))
	require.NoError(t, err)
	second, err := semantic.Marshal(comprehensiveConversation(true))
	require.NoError(t, err)
	assert.Equal(t, first, second)
}

func TestMarshalRoundTripsReduxOrigin(t *testing.T) {
	t.Parallel()

	conversation := comprehensiveConversation(false)
	conversation.ID = "redux.orin"
	conversation.Origin = &semantic.ReduxOrigin{}
	data, err := semantic.Marshal(conversation)
	require.NoError(t, err)
	assert.Contains(t, string(data), "origin:\n  kind: redux\n")
	assert.NotContains(t, string(data), "  source:")
	assert.NotContains(t, string(data), "  dialog:")

	loaded, err := semantic.Load("redux-round-trip.yaml", bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, conversation, loaded)
}

func TestMarshalRoundTripsFallbackOnlyQuestion(t *testing.T) {
	t.Parallel()

	conversation := comprehensiveConversation(false)
	question := conversation.Questions["question_15"]
	question.Answers = []semantic.Answer{}
	question.Default = semantic.Body{simpleSay("No custom route matched.")}
	conversation.Questions["question_15"] = question

	data, err := semantic.Marshal(conversation)
	require.NoError(t, err)
	assert.Contains(t, string(data), "  question_15:\n    prompt: End the survey?\n    answers: []\n")

	loaded, err := semantic.Load("fallback-only.yaml", bytes.NewReader(data))
	require.NoError(t, err)
	assert.Equal(t, conversation, loaded)
	assert.Empty(t, loaded.Questions["question_15"].Answers)
}

func TestMarshalRejectsInvalidConversation(t *testing.T) {
	t.Parallel()

	conversation := comprehensiveConversation(false)
	conversation.ID = "invalid"
	data, err := semantic.Marshal(conversation)
	assert.Nil(t, data)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "validating semantic conversation for YAML")
	assert.Contains(t, err.Error(), "$.id")
}

func comprehensiveConversation(reverseQuestions bool) *semantic.Conversation {
	returnToTopics := semantic.Body{&semantic.ReturnToTopicsAction{}}
	end := semantic.Body{&semantic.EndAction{}}
	questionOne := semantic.Question{
		Prompt: semantic.Body{simpleSay("Choose a route.")},
		Answers: []semantic.Answer{
			{
				Keywords: []string{"north", "upward"},
				Response: semantic.Body{
					&semantic.SayAction{Content: semantic.RichText{
						&semantic.TextPart{Text: "Read "},
						&semantic.RunicPart{Text: "north"},
						&semantic.LineBreakPart{},
						&semantic.TextPart{Text: "then proceed, {avatar_name}."},
					}},
					&semantic.ReturnToTopicsAction{},
				},
			},
			{
				Keywords: []string{"south"},
				Response: semantic.Body{
					&semantic.BranchAction{
						Condition: &semantic.KarmaAtLeastCondition{Minimum: 50},
						Then:      semantic.Body{simpleSay("The bright road opens.")},
						Otherwise: &returnToTopics,
					},
					&semantic.ReturnToTopicsAction{},
				},
			},
		},
		Default: semantic.Body{simpleSay("That route is not marked.")},
	}
	questionFifteen := semantic.Question{
		Prompt: semantic.Body{simpleSay("End the survey?")},
		Answers: []semantic.Answer{
			{Keywords: []string{"yes"}, Response: end},
			{Keywords: []string{"no"}, Response: semantic.Body{simpleSay("Continue, then."), &semantic.ReturnToTopicsAction{}}},
		},
		Default: semantic.Body{},
	}
	questions := make(map[string]semantic.Question, 2)
	if reverseQuestions {
		questions["question_15"] = questionFifteen
		questions["question_01"] = questionOne
	} else {
		questions["question_01"] = questionOne
		questions["question_15"] = questionFifteen
	}

	return &semantic.Conversation{
		Schema:   semantic.SchemaVersion,
		Protocol: semantic.ProtocolUltima5,
		ID:       "original.towne_42",
		Origin: &semantic.OriginalOrigin{
			Source: semantic.OriginalSourceTowne,
			Dialog: 42,
		},
		Name:        semantic.Body{simpleSay("Orin")},
		Description: semantic.Body{simpleNarrate("A methodical keeper records invented routes.")},
		Greeting: semantic.Body{&semantic.SayAction{Content: semantic.RichText{
			&semantic.TextPart{Text: "Greetings, {avatar_name}. I am "},
			&semantic.RunicPart{Text: "Orin"},
			&semantic.LineBreakPart{},
			&semantic.TextPart{Text: "The ledger is ready."},
		}}},
		Job: semantic.Body{
			simpleSay("I test every semantic action."),
			&semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: "A clean page turns."}}},
			&semantic.PauseAction{},
			&semantic.WaitForKeyAction{},
			&semantic.AskNameAction{},
			&semantic.JoinPartyAction{},
			&semantic.CallGuardsAction{},
			&semantic.GoToJailAction{},
			&semantic.TakeGoldAction{Amount: 7},
			&semantic.GiveItemAction{Item: "map_token", Quantity: 1},
			&semantic.GiveItemAction{Item: "chalk", Quantity: 5},
			&semantic.AdjustKarmaAction{Delta: -1},
			&semantic.DiscoverKnowledgeAction{Knowledge: semantic.KnowledgeSpellAnYlem},
			&semantic.BranchAction{
				Condition: &semantic.KarmaAtLeastCondition{Minimum: 25},
				Then:      semantic.Body{simpleSay("The optional path is recorded.")},
				Otherwise: nil,
			},
			&semantic.BranchAction{
				Condition: &semantic.NPCKnowsPartyNameCondition{},
				Then:      semantic.Body{&semantic.AskAction{Question: "question_15"}},
				Otherwise: &end,
			},
		},
		Farewell: semantic.Body{},
		Topics: []semantic.Topic{
			{Keywords: []string{"survey", "routes"}, Response: semantic.Body{&semantic.AskAction{Question: "question_01"}}},
			{Keywords: []string{"ledger"}, Response: semantic.Body{simpleSay("The entries remain ordered.")}},
			{Keywords: []string{"leave"}, Response: semantic.Body{&semantic.EndAction{}}},
			{Keywords: []string{"study"}, Response: semantic.Body{&semantic.BeginCircleStudyAction{}}},
		},
		Questions: questions,
	}
}

func simpleSay(text string) *semantic.SayAction {
	return &semantic.SayAction{Content: semantic.RichText{&semantic.TextPart{Text: text}}}
}

func simpleNarrate(text string) *semantic.NarrateAction {
	return &semantic.NarrateAction{Content: semantic.RichText{&semantic.TextPart{Text: text}}}
}
