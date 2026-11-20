package conversation

import (
	"errors"
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type semanticTestCallbacks struct {
	avatarName  string
	hasMet      bool
	karma       int
	joined      int
	guards      int
	jail        int
	studies     int
	karmaUp     int
	karmaDown   int
	pauses      int
	items       []int
	gold        []int
	discoveries []semantic.KnowledgeID
	discoverErr error
	joinOutcome JoinPartyOutcome
	joinErr     error
	metActorIDs []int
}

func (callbacks *semanticTestCallbacks) JoinParty() (JoinPartyOutcome, error) {
	callbacks.joined++
	return callbacks.joinOutcome, callbacks.joinErr
}

func (callbacks *semanticTestCallbacks) CallGuards() { callbacks.guards++ }
func (callbacks *semanticTestCallbacks) IncreaseKarma() {
	callbacks.karmaUp++
}
func (callbacks *semanticTestCallbacks) DecreaseKarma() {
	callbacks.karmaDown++
}
func (callbacks *semanticTestCallbacks) GoToJail()         { callbacks.jail++ }
func (callbacks *semanticTestCallbacks) BeginCircleStudy() { callbacks.studies++ }
func (callbacks *semanticTestCallbacks) GiveItem(itemID int) {
	callbacks.items = append(callbacks.items, itemID)
}
func (callbacks *semanticTestCallbacks) DeductGoldSimple(amount int, _ string) error {
	callbacks.gold = append(callbacks.gold, amount)

	return nil
}
func (callbacks *semanticTestCallbacks) TimedPause() { callbacks.pauses++ }
func (callbacks *semanticTestCallbacks) HasMet(int) bool {
	return callbacks.hasMet
}
func (callbacks *semanticTestCallbacks) GetAvatarName() string { return callbacks.avatarName }
func (callbacks *semanticTestCallbacks) GetActivePartyNames() []string {
	return []string{callbacks.avatarName, "Iolo"}
}
func (callbacks *semanticTestCallbacks) GetKarmaLevel() int { return callbacks.karma }
func (callbacks *semanticTestCallbacks) SetMet(actorID int) {
	callbacks.hasMet = true
	callbacks.metActorIDs = append(callbacks.metActorIDs, actorID)
}
func (callbacks *semanticTestCallbacks) DiscoverKnowledge(knowledge semantic.KnowledgeID) error {
	callbacks.discoveries = append(callbacks.discoveries, knowledge)

	return callbacks.discoverErr
}

func TestSemanticConversationEngineProtocolFlow(t *testing.T) {
	definition := semanticTestConversation()
	callbacks := &semanticTestCallbacks{avatarName: "Avatar", karma: 60}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)

	start := engine.Start(7)
	assert.Equal(t, "You see a careful keeper\n\n", start.Output)
	assert.Equal(t, promptInterest, start.InputPrompt)
	assert.False(t, start.IsLabelQuestion)

	name := engine.ProcessInput("what is thy name")
	assert.Equal(t, "\n\n\"My name is Mara\"\n\n", name.Output)
	assert.True(t, name.ContinuesPreviousOutput)

	job := engine.ProcessInput("work")
	assert.Equal(t, "\n\n\"I keep this gate.\"\n\n", job.Output)
	assert.True(t, job.ContinuesPreviousOutput)

	topic := engine.ProcessInput("please tell me of maps")
	assert.Equal(t, "\n\n\"The road is open.\"\n\n", topic.Output)
	assert.True(t, topic.ContinuesPreviousOutput)
	assert.Equal(t, []int{5}, callbacks.gold)
	unknown := engine.ProcessInput("unknown")
	assert.Equal(t, "\n\n\"I cannot help thee with that.\"\n\n", unknown.Output)
	assert.True(t, unknown.ContinuesPreviousOutput)

	question := engine.ProcessInput("riddle")
	assert.Equal(t, "\n\n\"Answer me.\"\n\n", question.Output)
	assert.True(t, question.ContinuesPreviousOutput)
	assert.True(t, question.IsLabelQuestion)
	assert.Equal(t, promptRespond, question.InputPrompt)
	emptyAnswer := engine.ProcessInput("")
	assert.Equal(t, "\n\n\"What didst thou say?\"\n\n", emptyAnswer.Output)
	assert.True(t, emptyAnswer.ContinuesPreviousOutput)
	assert.True(t, emptyAnswer.IsLabelQuestion)

	answer := engine.ProcessInput("yes")
	assert.Equal(t, "\n\n\"Correct, Avatar.\"", answer.Output)
	assert.True(t, answer.ContinuesPreviousOutput)
	assert.Equal(t, promptInterest, answer.InputPrompt)
	assert.False(t, answer.IsLabelQuestion)

	bye := engine.ProcessInput("thanks")
	assert.Equal(t, "\n\n\"Farewell.\"\n", bye.Output)
	assert.True(t, bye.ContinuesPreviousOutput)
	assert.True(t, bye.IsCompleteAwaitingDismissal)
	assert.False(t, engine.IsActive())
}

func TestSemanticConversationJobEntersDefaultOnlyQuestion(t *testing.T) {
	definition := semanticTestConversation()
	definition.Job = semantic.Body{&semantic.AskAction{Question: "question_02"}}
	definition.Questions["question_02"] = semantic.Question{
		Prompt:  semantic.Body{&semantic.SayAction{Content: semanticText("What dost thou seek?")}},
		Answers: []semantic.Answer{},
		Default: semantic.Body{&semantic.SayAction{Content: semanticText("Seek elsewhere.")}},
	}
	engine, err := NewSemanticConversationEngine(definition, &semanticTestCallbacks{avatarName: "Avatar"}, false)
	require.NoError(t, err)
	engine.Start(71)
	question := engine.ProcessInput("job")
	require.NoError(t, question.Error)
	assert.True(t, question.IsLabelQuestion)
	assert.Equal(t, promptRespond, question.InputPrompt)
	assert.Contains(t, question.Output, "What dost thou seek?")
	answer := engine.ProcessInput("anything")
	require.NoError(t, answer.Error)
	assert.Contains(t, answer.Output, "Seek elsewhere.")
	assert.False(t, answer.IsLabelQuestion)
	assert.Equal(t, promptInterest, answer.InputPrompt)
}

func TestSemanticConversationEngineNameBranchWaitAndActions(t *testing.T) {
	definition := semanticTestConversation()
	definition.Description = semantic.Body{
		&semantic.NarrateAction{Content: semanticText("a stranger")},
		&semantic.AskNameAction{},
		&semantic.BranchAction{
			Condition: &semantic.NPCKnowsPartyNameCondition{},
			Then: semantic.Body{
				&semantic.SayAction{Content: semanticText("Welcome, {avatar_name}.")},
				&semantic.WaitForKeyAction{},
				&semantic.GiveItemAction{Item: "food", Quantity: 2},
				&semantic.AdjustKarmaAction{Delta: -2},
				&semantic.CallGuardsAction{},
				&semantic.GoToJailAction{},
				&semantic.EndAction{},
			},
		},
	}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)

	namePrompt := engine.Start(13)
	assert.Equal(t, "You see a stranger\"What is thy name?\"", namePrompt.Output)
	assert.True(t, namePrompt.IsLabelQuestion)

	wait := engine.ProcessInput("Avatar")
	assert.Contains(t, wait.Output, "A pleasure!")
	assert.Contains(t, wait.Output, "Welcome, Avatar.")
	assert.NotContains(t, wait.Output, "A pleasure!\n\nWelcome")
	assert.NotContains(t, wait.Output, "Welcome, Avatar.\"")
	assert.True(t, wait.WaitingForKeypress)
	assert.Equal(t, []int{13}, callbacks.metActorIDs)

	complete := engine.ProcessInput("")
	assert.True(t, complete.IsCompleteAwaitingDismissal)
	assert.Equal(t, quoteClose, complete.Output)
	assert.Equal(t, []int{65, 65}, callbacks.items)
	assert.Equal(t, 2, callbacks.karmaDown)
	assert.Equal(t, 1, callbacks.guards)
	assert.Equal(t, 1, callbacks.jail)
}

func TestSemanticConversationEngineBeginCircleStudyEndsTheTalkOnce(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"study"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semanticText("Then let us begin.")},
			&semantic.BeginCircleStudyAction{},
		},
	}}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(13)
	require.Zero(t, callbacks.studies, "nothing is asked of the host before the keyword")

	complete := engine.ProcessInput("study")

	assert.Contains(t, complete.Output, "Then let us begin.")
	assert.True(t, complete.IsCompleteAwaitingDismissal, "the talk ends so the host can open the study")
	assert.Equal(t, 1, callbacks.studies)
}

func TestSemanticConversationEngineRunicParagraphsAndPause(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"rune"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semantic.RichText{
				&semantic.TextPart{Text: "The word is "},
				&semantic.RunicPart{Text: "VILIS"},
			}},
			&semantic.NarrateAction{Content: semantic.RichText{
				&semantic.LineBreakPart{},
				&semantic.LineBreakPart{},
			}},
			&semantic.SayAction{Content: semanticText("Remember it.")},
			&semantic.PauseAction{},
			&semantic.ReturnToTopicsAction{},
		},
	}}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("rune")
	assert.Equal(t, "\n\n\"The word is VILIS\"\n\n\"Remember it."+PauseMarker+"\"", response.Output)
	require.Len(t, response.Segments, 3)
	assert.False(t, response.Segments[0].IsRunic)
	assert.True(t, response.Segments[1].IsRunic)
	assert.False(t, response.Segments[2].IsRunic)
	assert.Equal(t, 1, callbacks.pauses)
}

func TestSemanticConversationEngineDoesNotInventTextActionSeparators(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"plain"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semanticText("First")},
			&semantic.SayAction{Content: semanticText("Second")},
			&semantic.ReturnToTopicsAction{},
		},
	}}
	engine, err := NewSemanticConversationEngine(
		definition,
		&semanticTestCallbacks{avatarName: "Avatar"},
		false,
	)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("plain")
	assert.Equal(t, "\n\n\"FirstSecond\"", response.Output)
}

func TestSemanticConversationEngineMarksMidSentenceKeyWaitContinuation(t *testing.T) {
	definition := semanticTestConversation()
	definition.Description = semantic.Body{
		&semantic.NarrateAction{Content: semanticText("a speaker")},
		&semantic.SayAction{Content: semanticText("Before ")},
		&semantic.WaitForKeyAction{},
		&semantic.SayAction{Content: semanticText("after.")},
		&semantic.EndAction{},
	}
	engine, err := NewSemanticConversationEngine(
		definition,
		&semanticTestCallbacks{avatarName: "Avatar"},
		false,
	)
	require.NoError(t, err)

	wait := engine.Start(1)
	assert.Equal(t, "You see a speaker\"Before ", wait.Output)
	assert.True(t, wait.WaitingForKeypress)
	assert.False(t, wait.ContinuesPreviousOutput)

	complete := engine.ProcessInput("")
	assert.Equal(t, "after.\"", complete.Output)
	assert.True(t, complete.ContinuesPreviousOutput)
	assert.True(t, complete.IsCompleteAwaitingDismissal)
}

func TestSemanticConversationEngineCarriesContinuationAcrossEmptyKeyWait(t *testing.T) {
	definition := semanticTestConversation()
	definition.Description = semantic.Body{
		&semantic.NarrateAction{Content: semanticText("a speaker")},
		&semantic.SayAction{Content: semanticText("Before ")},
		&semantic.WaitForKeyAction{},
		&semantic.WaitForKeyAction{},
		&semantic.SayAction{Content: semanticText("after.")},
		&semantic.EndAction{},
	}
	engine, err := NewSemanticConversationEngine(
		definition,
		&semanticTestCallbacks{avatarName: "Avatar"},
		false,
	)
	require.NoError(t, err)

	firstWait := engine.Start(1)
	assert.True(t, firstWait.WaitingForKeypress)
	secondWait := engine.ProcessInput("")
	assert.Empty(t, secondWait.Output)
	assert.True(t, secondWait.WaitingForKeypress)
	assert.True(t, secondWait.ContinuesPreviousOutput)
	complete := engine.ProcessInput("")
	assert.Equal(t, "after.\"", complete.Output)
	assert.True(t, complete.ContinuesPreviousOutput)
}

func TestSemanticConversationEngineJoinEndsConversation(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"join"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semanticText("I shall join thee.")},
			&semantic.JoinPartyAction{},
		},
	}}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("join")
	assert.True(t, response.IsCompleteAwaitingDismissal)
	assert.Equal(t, 1, callbacks.joined)
}

func TestSemanticConversationEngineJoinCapacityRefusalRemainsActive(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"join"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semanticText("I shall join thee.")},
			&semantic.JoinPartyAction{},
		},
	}}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar", joinOutcome: JoinPartyCapacityRefused}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("join")
	require.NoError(t, response.Error)
	assert.Equal(t, "\n\n\"I shall join thee.\"", response.Output)
	assert.True(t, response.WaitingForKeypress)
	assert.False(t, response.NeedsInput)
	assert.False(t, response.IsCompleteAwaitingDismissal)
	assert.True(t, engine.IsActive())
	assert.Equal(t, 1, callbacks.joined)

	continued := engine.ProcessInput("")
	require.NoError(t, continued.Error)
	assert.True(t, continued.NeedsInput)
	assert.Equal(t, promptInterest, continued.InputPrompt)
}

func TestSemanticConversationEngineJoinUnexpectedErrorIsReported(t *testing.T) {
	joinErr := errors.New("recruitment failed")
	definition := semanticTestConversation()
	definition.Description = semantic.Body{&semantic.JoinPartyAction{}}
	callbacks := &semanticTestCallbacks{joinOutcome: JoinPartyUnexpectedError, joinErr: joinErr}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)

	response := engine.Start(1)
	require.ErrorIs(t, response.Error, joinErr)
	assert.True(t, engine.IsActive())
}

func TestSemanticConversationEngineOriginalProfanityProtocol(t *testing.T) {
	require.Equal(t, []string{
		"FUCK", "SHIT", "DAMN", "DICK", "PRICK", "PUSSY", "CUNT", "ASS", "BUTT", "BOOGER",
		"PISS", "JACK OFF", "MASTURBATE", "SUCK", "FART", "TITS", "BOOB", "MELONS", "BLOW",
		"PENIS", "BREAST", "CLIT", "BALLS", "SCROTUM", "NUTS", "BULLSHIT", "CUM", "CROTCH",
		"MOTHERFUCKER",
	}, originalProfanityKeywords())

	for _, profanity := range originalProfanityKeywords() {
		t.Run(profanity, func(t *testing.T) {
			definition := semanticTestConversation()
			definition.Topics = []semantic.Topic{{
				Keywords: []string{profanity},
				Response: semantic.Body{&semantic.SayAction{Content: semanticText("Authored response.")}},
			}}
			callbacks := &semanticTestCallbacks{}
			engine, err := NewSemanticConversationEngine(definition, callbacks, false)
			require.NoError(t, err)
			engine.Start(1)

			response := engine.ProcessInput(profanity)
			require.NoError(t, response.Error)
			assert.Equal(t, "\n\n\""+profanityResponse+"\"\n\n"+PauseMarker, response.Output)
			assert.NotContains(t, response.Output, "Authored response.")
			assert.True(t, response.NeedsInput)
			assert.True(t, engine.IsActive())
			assert.Equal(t, 1, callbacks.pauses)
		})
	}
}

func TestSemanticConversationEngineProfanityPreservesQuestionAndReprompts(t *testing.T) {
	definition := semanticTestConversation()
	definition.Questions["question_01"] = semantic.Question{
		Prompt: semantic.Body{&semantic.SayAction{Content: semanticText("Answer me.")}},
		Answers: []semantic.Answer{{
			Keywords: []string{"FUCK"},
			Response: semantic.Body{&semantic.SayAction{Content: semanticText("Authored response.")}},
		}},
		Default: semantic.Body{&semantic.SayAction{Content: semanticText("No.")}},
	}
	callbacks := &semanticTestCallbacks{}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)
	require.True(t, engine.ProcessInput("riddle").IsLabelQuestion)

	response := engine.ProcessInput("FUCK")
	require.NoError(t, response.Error)
	assert.Equal(t, "\n\n\""+profanityResponse+"\"\n\n"+PauseMarker+"\"Answer me.\"\n\n", response.Output)
	assert.NotContains(t, response.Output, "Authored response.")
	assert.True(t, response.NeedsInput)
	assert.True(t, response.IsLabelQuestion)
	assert.Equal(t, "question_01", engine.currentQuestion)
	assert.Equal(t, 1, callbacks.pauses)
}

func TestSemanticConversationEngineElectronicArtsRemainsAuthoredKeyword(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"ELECTRONIC ARTS"},
		Response: semantic.Body{&semantic.SayAction{Content: semanticText("Authored response.")}},
	}}
	callbacks := &semanticTestCallbacks{}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("ELECTRONIC ARTS")
	require.NoError(t, response.Error)
	assert.Contains(t, response.Output, "Authored response.")
	assert.NotContains(t, response.Output, profanityResponse)
	assert.Zero(t, callbacks.pauses)
}

func TestSemanticConversationEngineStandardProtocolPrecedesProfanity(t *testing.T) {
	definition := semanticTestConversation()
	callbacks := &semanticTestCallbacks{}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("NAME FUCK")
	require.NoError(t, response.Error)
	assert.Contains(t, response.Output, "My name is Mara")
	assert.NotContains(t, response.Output, profanityResponse)
	assert.Zero(t, callbacks.pauses)
}

func TestSemanticConversationEngineDiscoversKnowledgeSilentlyInOrder(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"lore"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semanticText("Before")},
			&semantic.DiscoverKnowledgeAction{Knowledge: semantic.KnowledgeSpellAnYlem},
			&semantic.SayAction{Content: semanticText("After")},
			&semantic.ReturnToTopicsAction{},
		},
	}}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar"}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("lore")
	require.NoError(t, response.Error)
	assert.Equal(t, "\n\n\"BeforeAfter\"", response.Output)
	assert.Equal(t, []semantic.KnowledgeID{semantic.KnowledgeSpellAnYlem}, callbacks.discoveries)
}

func TestSemanticConversationEnginePropagatesKnowledgeCallbackError(t *testing.T) {
	callbackErr := errors.New("recording knowledge")
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"lore"},
		Response: semantic.Body{
			&semantic.SayAction{Content: semanticText("Before")},
			&semantic.DiscoverKnowledgeAction{Knowledge: semantic.KnowledgeSpellInXenMani},
			&semantic.SayAction{Content: semanticText("After")},
		},
	}}
	callbacks := &semanticTestCallbacks{avatarName: "Avatar", discoverErr: callbackErr}
	engine, err := NewSemanticConversationEngine(definition, callbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("lore")
	require.ErrorIs(t, response.Error, callbackErr)
	assert.Equal(t, "\n\n\"Before\"", response.Output)
	assert.Equal(t, []semantic.KnowledgeID{semantic.KnowledgeSpellInXenMani}, callbacks.discoveries)
}

func TestSemanticConversationEngineRequiresSemanticCallbacksOnlyForSemanticAction(t *testing.T) {
	definition := semanticTestConversation()
	definition.Topics = []semantic.Topic{{
		Keywords: []string{"lore"},
		Response: semantic.Body{
			&semantic.DiscoverKnowledgeAction{Knowledge: semantic.KnowledgeSpellRelXenBet},
		},
	}}
	legacyCallbacks := struct{ SemanticCallbacks }{
		SemanticCallbacks: &semanticTestCallbacks{avatarName: "Avatar"},
	}
	engine, err := NewSemanticConversationEngine(definition, legacyCallbacks, false)
	require.NoError(t, err)
	engine.Start(1)

	response := engine.ProcessInput("lore")
	require.ErrorIs(t, response.Error, errSemanticCallbacksMissing)
}

func semanticTestConversation() *semantic.Conversation {
	return &semantic.Conversation{
		Schema: semantic.SchemaVersion, Protocol: semantic.ProtocolUltima5,
		ID: "redux.semantic_test", Origin: &semantic.ReduxOrigin{},
		Name:        semantic.Body{&semantic.SayAction{Content: semanticText("Mara")}},
		Description: semantic.Body{&semantic.NarrateAction{Content: semanticText("a careful keeper")}},
		Greeting:    semantic.Body{&semantic.SayAction{Content: semanticText("Welcome back.")}},
		Job:         semantic.Body{&semantic.SayAction{Content: semanticText("I keep this gate.")}},
		Farewell:    semantic.Body{&semantic.SayAction{Content: semanticText("Farewell.")}},
		Topics: []semantic.Topic{
			{Keywords: []string{"map", "maps"}, Response: semantic.Body{
				&semantic.TakeGoldAction{Amount: 5},
				&semantic.BranchAction{
					Condition: &semantic.KarmaAtLeastCondition{Minimum: 50},
					Then:      semantic.Body{&semantic.SayAction{Content: semanticText("The road is open.")}},
					Otherwise: semanticBodyPointer(&semantic.SayAction{Content: semanticText("Not yet.")}),
				},
			},
			},
			{Keywords: []string{"riddle"}, Response: semantic.Body{&semantic.AskAction{Question: "question_01"}}},
		},
		Questions: map[string]semantic.Question{
			"question_01": {
				Prompt: semantic.Body{&semantic.SayAction{Content: semanticText("Answer me.")}},
				Answers: []semantic.Answer{{
					Keywords: []string{"y"}, Response: semantic.Body{
						&semantic.SayAction{Content: semanticText("Correct, {avatar_name}.")},
						&semantic.ReturnToTopicsAction{},
					},
				}},
				Default: semantic.Body{&semantic.SayAction{Content: semanticText("No.")}, &semantic.ReturnToTopicsAction{}},
			},
		},
	}
}

func semanticText(text string) semantic.RichText {
	return semantic.RichText{&semantic.TextPart{Text: text}}
}

func semanticBodyPointer(actions ...semantic.Action) *semantic.Body {
	body := semantic.Body(actions)

	return &body
}

func (callbacks *semanticTestCallbacks) String() string {
	return fmt.Sprintf("met=%v karma=%d", callbacks.hasMet, callbacks.karma)
}
