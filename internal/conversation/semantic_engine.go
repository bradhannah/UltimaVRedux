package conversation

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
)

const (
	maxSemanticExecutionSteps = 10000
	sourcePartyNameMatchBytes = 4
	fallbackNPCName           = "NPC"
	profanityResponse         = "With language like that, how did you become an Avatar?"
)

var (
	// ErrInsufficientPayment is an ordinary refusal, not a resumable action failure.
	ErrInsufficientPayment       = errors.New("insufficient conversation payment")
	errJoinPartyFailed           = errors.New("join party failed without an error")
	errJoinPartyOutcomeUnknown   = errors.New("unknown join party outcome")
	errSemanticCallbacksNil      = errors.New("semantic conversation callbacks are nil")
	errConversationInactive      = errors.New("conversation not active")
	errSemanticQuestionMissing   = errors.New("semantic question is missing")
	errSemanticItemMissing       = errors.New("semantic item has no runtime mapping")
	errSemanticCallbacksMissing  = errors.New("semantic action callbacks are unavailable")
	errSemanticActionUnsupported = errors.New("unsupported semantic action")
	errSemanticStepLimit         = errors.New("semantic conversation execution step limit exceeded")
	errSemanticCompletionUnknown = errors.New("unknown semantic completion")
)

func originalProfanityKeywords() []string {
	return []string{
		"FUCK", "SHIT", "DAMN", "DICK", "PRICK", "PUSSY", "CUNT", "ASS", "BUTT", "BOOGER",
		"PISS", "JACK OFF", "MASTURBATE", "SUCK", "FART", "TITS", "BOOB", "MELONS", "BLOW",
		"PENIS", "BREAST", "CLIT", "BALLS", "SCROTUM", "NUTS", "BULLSHIT", "CUM", "CROTCH",
		"MOTHERFUCKER",
	}
}

type semanticCompletion uint8

const (
	semanticCompletionTopics semanticCompletion = iota
	semanticCompletionBootstrapDescription
	semanticCompletionBootstrapGreeting
	semanticCompletionQuestionPrompt
	semanticCompletionFarewell
)

type semanticFrame struct {
	body  semantic.Body
	index int
}

type semanticOutput struct {
	segments  []TextSegment
	quoteOpen bool
}

// SemanticActionCallbacks handles actions that exist only in semantic conversations.
// Legacy-only ActionCallbacks implementations do not need to implement this interface.
type SemanticActionCallbacks interface {
	DiscoverKnowledge(knowledge semantic.KnowledgeID) error
}

// DiscoveryBoundary addresses the original semantic output, including pause markers.
type DiscoveryBoundary struct {
	Response     uint64
	SourceOffset int // UTF-8 bytes, including formatting and pause tokens.
}

// DeferredKnowledgeCallbacks stages typed discoveries for mounted presentation.
type DeferredKnowledgeCallbacks interface {
	QueueKnowledge(knowledge semantic.KnowledgeID, boundary DiscoveryBoundary) error
}

// SemanticCallbacks separates name recognition from Avatar text substitution.
type SemanticCallbacks interface {
	ActionCallbacks
	GetActivePartyNames() []string
}

// SemanticConversationEngine executes validated semantic conversation actions.
type SemanticConversationEngine struct {
	conversation        *semantic.Conversation
	callbacks           SemanticCallbacks
	alwaysShowGreeting  bool
	actorID             int
	hasMet              bool
	active              bool
	waitingForName      bool
	waitingForKey       bool
	waitContinuesOutput bool
	currentQuestion     string
	frames              []semanticFrame
	completion          semanticCompletion
	output              semanticOutput
	responseSerial      uint64
	discoverKnowledge   func(semantic.KnowledgeID, DiscoveryBoundary) error
}

// NewSemanticConversationEngine creates an inactive semantic conversation session.
func NewSemanticConversationEngine(
	definition *semantic.Conversation,
	callbacks SemanticCallbacks,
	alwaysShowGreeting bool,
) (*SemanticConversationEngine, error) {
	if err := semantic.Validate(definition); err != nil {
		return nil, fmt.Errorf("validating semantic conversation: %w", err)
	}
	if callbacks == nil {
		return nil, errSemanticCallbacksNil
	}

	engine := &SemanticConversationEngine{
		conversation: definition, callbacks: callbacks, alwaysShowGreeting: alwaysShowGreeting,
		actorID: 0, hasMet: false, active: false, waitingForName: false, waitingForKey: false,
		waitContinuesOutput: false,
		currentQuestion:     "", frames: nil, completion: semanticCompletionTopics, output: newSemanticOutput(),
		responseSerial: 0, discoverKnowledge: nil,
	}
	// Select the capability once. Headless callbacks explicitly retain immediate semantics.
	if deferred, ok := callbacks.(DeferredKnowledgeCallbacks); ok {
		engine.discoverKnowledge = deferred.QueueKnowledge
	} else {
		actions, supported := callbacks.(SemanticActionCallbacks)
		engine.discoverKnowledge = func(id semantic.KnowledgeID, _ DiscoveryBoundary) error {
			if !supported {
				return fmt.Errorf("%w: %s", errSemanticCallbacksMissing, id)
			}

			return actions.DiscoverKnowledge(id)
		}
	}

	return engine, nil
}

// NewDeferredSemanticConversationEngine requires the production presentation capability.
func NewDeferredSemanticConversationEngine(
	definition *semantic.Conversation, callbacks SemanticCallbacks, greeting bool,
) (*SemanticConversationEngine, error) {
	if _, ok := callbacks.(DeferredKnowledgeCallbacks); !ok {
		return nil, errSemanticCallbacksMissing
	}

	return NewSemanticConversationEngine(definition, callbacks, greeting)
}

// Start begins the conversation using the actor's persistent map-unit slot.
func (engine *SemanticConversationEngine) Start(actorID int) *ConversationResponse {
	engine.actorID = actorID
	engine.hasMet = engine.callbacks.HasMet(actorID)
	engine.active = true
	engine.waitingForName = false
	engine.waitingForKey = false
	engine.waitContinuesOutput = false
	engine.currentQuestion = ""
	engine.frames = nil
	engine.resetOutput()
	engine.output.append("You see ", false)
	engine.startBody(engine.conversation.Description, semanticCompletionBootstrapDescription)

	return engine.execute()
}

// ProcessInput advances the active semantic conversation.
func (engine *SemanticConversationEngine) ProcessInput(input string) *ConversationResponse {
	if !engine.active {
		response := newConversationResponse()
		response.Error = errConversationInactive

		return response
	}
	continueOpenQuote := engine.waitingForKey && engine.output.quoteOpen
	engine.resetOutput()
	engine.output.quoteOpen = continueOpenQuote
	normalized := strings.TrimSpace(strings.ToUpper(input))

	if engine.waitingForKey {
		engine.waitingForKey = false
		continuesPrevious := engine.waitContinuesOutput
		engine.waitContinuesOutput = false
		response := engine.execute()
		response.ContinuesPreviousOutput = continuesPrevious
		if response.WaitingForKeypress && response.Output == "" {
			engine.waitContinuesOutput = continuesPrevious
		}

		return response
	}
	// Original conversation emits two newlines (ref:TALK-0004) after entered interest, question,
	// and name responses. Key waits are deliberately excluded because 0x8F
	// itself emits no output.
	engine.output.append("\n\n", false)
	var response *ConversationResponse
	switch {
	case engine.waitingForName:
		response = engine.processNameInput(strings.ToUpper(input))
	case engine.currentQuestion != "":
		response = engine.processQuestionInput(normalized)
	default:
		response = engine.processTopicInput(normalized)
	}
	response.ContinuesPreviousOutput = true

	return response
}

// IsActive reports whether the session accepts input.
func (engine *SemanticConversationEngine) IsActive() bool { return engine.active }

// Stop forcefully ends the session.
func (engine *SemanticConversationEngine) Stop() { engine.active = false }

// ExtractNPCName returns the plain semantic name used by gameplay callbacks.
func (engine *SemanticConversationEngine) ExtractNPCName() string {
	name := engine.nameBodyText(engine.conversation.Name)
	if name == "" {
		return fallbackNPCName
	}

	return name
}

func (engine *SemanticConversationEngine) processTopicInput(input string) *ConversationResponse {
	if input == "" || matchesProtocolKeyword(input, "BYE") || matchesProtocolKeyword(input, "THANK") {
		engine.output.openQuote()
		engine.startBody(engine.conversation.Farewell, semanticCompletionFarewell)

		return engine.execute()
	}
	if matchesProtocolKeyword(input, "NAME") {
		engine.output.openQuote()
		engine.output.append(responseNamePrefix, false)
		engine.startBody(engine.conversation.Name, semanticCompletionTopics)

		response := engine.execute()
		if !response.WaitingForKeypress {
			response.RevealedName = engine.ExtractNPCName()
		}

		return response
	}
	if matchesProtocolKeyword(input, "JOB") || matchesProtocolKeyword(input, "WORK") {
		engine.output.openQuote()
		engine.startBody(engine.conversation.Job, semanticCompletionTopics)

		return engine.execute()
	}
	if matchesAnyKeyword(input, originalProfanityKeywords()) {
		return engine.processProfanityInput("")
	}
	for _, topic := range engine.conversation.Topics {
		if matchesAnyKeyword(input, topic.Keywords) {
			engine.output.openQuote()
			engine.startBody(topic.Response, semanticCompletionTopics)

			return engine.execute()
		}
	}

	engine.output.openQuote()
	engine.output.append(responseNoHelp, false)
	engine.output.closeQuote()
	engine.output.append("\n\n", false)

	return engine.topicResponse()
}

func (engine *SemanticConversationEngine) processQuestionInput(input string) *ConversationResponse {
	question, exists := engine.conversation.Questions[engine.currentQuestion]
	if !exists {
		response := newConversationResponse()
		response.Error = fmt.Errorf("%w: %q", errSemanticQuestionMissing, engine.currentQuestion)

		return response
	}
	if input == "" {
		engine.appendProtocolSpeech("What didst thou say?")
		engine.output.closeQuote()
		engine.output.append("\n\n", false)

		return engine.inputResponse(promptRespond, true)
	}
	if matchesAnyKeyword(input, originalProfanityKeywords()) {
		return engine.processProfanityInput(engine.currentQuestion)
	}
	for _, answer := range question.Answers {
		if matchesAnyKeyword(input, answer.Keywords) {
			engine.output.openQuote()
			engine.startBody(answer.Response, semanticCompletionTopics)

			return engine.execute()
		}
	}
	engine.output.openQuote()
	engine.startBody(question.Default, semanticCompletionTopics)

	return engine.execute()
}

func (engine *SemanticConversationEngine) processProfanityInput(question string) *ConversationResponse {
	body := semantic.Body{
		&semantic.SayAction{Content: semantic.RichText{&semantic.TextPart{Text: profanityResponse}}},
		&semantic.NarrateAction{Content: semantic.RichText{&semantic.LineBreakPart{}, &semantic.LineBreakPart{}}},
		&semantic.PauseAction{},
	}
	if question == "" {
		body = append(body, &semantic.ReturnToTopicsAction{})
	} else {
		body = append(body, &semantic.AskAction{Question: question})
	}
	engine.startBody(body, semanticCompletionTopics)

	return engine.execute()
}

func (engine *SemanticConversationEngine) processNameInput(input string) *ConversationResponse {
	engine.waitingForName = false
	recognized := false
	// Name matching uses up to four bytes, stopping at an embedded NUL; ref:TALK-0003.
	for _, name := range engine.callbacks.GetActivePartyNames() {
		name = name[:min(len(name), sourcePartyNameMatchBytes)]
		if end := strings.IndexByte(name, 0); end >= 0 {
			name = name[:end]
		}
		offset := strings.Index(input, strings.ToUpper(name))
		if name != "" && offset >= 0 && (offset == 0 || input[offset-1] == ' ') {
			recognized = true

			break
		}
	}
	if recognized {
		engine.hasMet = true
		engine.callbacks.SetMet(engine.actorID)
		engine.appendProtocolSpeech(responsePleasure)
	} else {
		engine.appendProtocolSpeech(responseIfYouSay)
	}
	engine.output.markControl()

	return engine.execute()
}

func (engine *SemanticConversationEngine) startQuestion(name string) error {
	question, exists := engine.conversation.Questions[name]
	if !exists {
		return fmt.Errorf("%w: %q", errSemanticQuestionMissing, name)
	}
	engine.currentQuestion = name
	engine.frames = nil
	engine.output.openQuote()
	engine.startBody(question.Prompt, semanticCompletionQuestionPrompt)

	return nil
}

func (engine *SemanticConversationEngine) startBody(
	body semantic.Body,
	completion semanticCompletion,
) {
	engine.completion = completion
	engine.frames = append(engine.frames, semanticFrame{body: body, index: 0})
}

//nolint:cyclop,funlen // This switch is the closed semantic action interpreter.
func (engine *SemanticConversationEngine) execute() *ConversationResponse {
	for range maxSemanticExecutionSteps {
		if len(engine.frames) == 0 {
			if response := engine.completeBody(); response != nil {
				return response
			}

			continue
		}
		frameIndex := len(engine.frames) - 1
		frame := &engine.frames[frameIndex]
		if frame.index >= len(frame.body) {
			engine.frames = engine.frames[:frameIndex]

			continue
		}
		action := frame.body[frame.index]
		frame.index++
		switch value := action.(type) {
		case *semantic.SayAction:
			engine.output.appendTextAction(semantic.ActionKindSay, value.Content, engine.substitute)
		case *semantic.NarrateAction:
			engine.output.appendTextAction(semantic.ActionKindNarrate, value.Content, engine.substitute)
		case *semantic.AskAction:
			if err := engine.startQuestion(value.Question); err != nil {
				return engine.errorResponse(err)
			}
		case *semantic.BranchAction:
			selected := engine.selectBranch(value)
			if selected != nil {
				engine.frames = append(engine.frames, semanticFrame{body: selected, index: 0})
			}
		case *semantic.TakeGoldAction:
			engine.output.markControl()
			if response := engine.takeGold(value.Amount); response != nil {
				return response
			}
		case *semantic.GiveItemAction:
			engine.output.markControl()
			if err := engine.giveItem(value); err != nil {
				return engine.errorResponse(err)
			}
		case *semantic.AdjustKarmaAction:
			engine.output.markControl()
			engine.adjustKarma(value.Delta)
		case *semantic.DiscoverKnowledgeAction:
			if err := engine.discoverKnowledgeAtOutputEnd(value); err != nil {
				return engine.errorResponse(err)
			}
		case *semantic.PauseAction:
			engine.output.markControl()
			engine.output.append(PauseMarker, false)
			engine.callbacks.TimedPause()
		case *semantic.WaitForKeyAction:
			engine.output.markControl()
			engine.waitingForKey = true
			response := engine.waitResponse()
			engine.waitContinuesOutput = response.Output != "" && !strings.HasSuffix(response.Output, "\n")

			return response
		case *semantic.AskNameAction:
			engine.output.markControl()
			// Reuse the protocol's open speech quote. Closing and reopening it
			// creates an empty quotation when AskName is the first action (Phillip).
			engine.output.openQuote()
			engine.output.append(promptName, false)
			engine.output.closeQuote()
			engine.waitingForName = true

			return engine.inputResponse(promptRespond, true)
		case *semantic.JoinPartyAction:
			engine.output.markControl()

			return engine.joinParty()
		case *semantic.CallGuardsAction:
			engine.output.markControl()
			engine.callbacks.CallGuards()
		case *semantic.GoToJailAction:
			engine.output.markControl()
			engine.callbacks.GoToJail()
		case *semantic.BeginCircleStudyAction:
			engine.output.markControl()
			engine.callbacks.BeginCircleStudy()
			engine.active = false
			engine.frames = nil

			return engine.dismissalResponse()
		case *semantic.ReturnToTopicsAction:
			engine.output.markControl()
			engine.frames = nil
			engine.currentQuestion = ""

			return engine.topicResponse()
		case *semantic.EndAction:
			engine.output.markControl()
			engine.active = false
			engine.frames = nil

			return engine.dismissalResponse()
		default:
			return engine.errorResponse(fmt.Errorf("%w: %T", errSemanticActionUnsupported, action))
		}
	}

	return engine.errorResponse(fmt.Errorf("%w: %d", errSemanticStepLimit, maxSemanticExecutionSteps))
}

// takeGold deducts amount for the speaking NPC. Refused payment abandons the
// entire paid continuation and returns to topics; ref:TALK-0003. A nil response
// means the conversation continues.
func (engine *SemanticConversationEngine) takeGold(amount int) *ConversationResponse {
	err := engine.callbacks.DeductGoldSimple(amount, engine.ExtractNPCName())
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrInsufficientPayment) {
		return engine.errorResponse(err)
	}
	engine.frames = nil
	engine.currentQuestion = ""
	engine.waitingForName = false
	engine.waitingForKey = false
	engine.waitContinuesOutput = false
	engine.completion = semanticCompletionTopics

	return engine.topicResponse()
}

// giveItem gives the party Quantity copies of an item known to the original game.
func (engine *SemanticConversationEngine) giveItem(action *semantic.GiveItemAction) error {
	itemID, exists := semantic.OriginalItemID(action.Item)
	if !exists {
		return fmt.Errorf("%w: %q", errSemanticItemMissing, action.Item)
	}
	for range action.Quantity {
		engine.callbacks.GiveItem(itemID)
	}

	return nil
}

// adjustKarma raises or lowers karma one step per unit of delta.
func (engine *SemanticConversationEngine) adjustKarma(delta int) {
	for range abs(delta) {
		if delta > 0 {
			engine.callbacks.IncreaseKarma()
		} else {
			engine.callbacks.DecreaseKarma()
		}
	}
}

// discoverKnowledgeAtOutputEnd records knowledge at the current end of this
// response's output.
//
// REDUX DEVIATION: Original conversations reveal spell lore without
// persisting knowledge. Redux records it for dossier and journal use.
// See ref:TALK-0003.
func (engine *SemanticConversationEngine) discoverKnowledgeAtOutputEnd(
	action *semantic.DiscoverKnowledgeAction,
) error {
	offset := 0
	for _, segment := range engine.output.segments {
		offset += len(segment.Text)
	}
	boundary := DiscoveryBoundary{Response: engine.responseSerial, SourceOffset: offset}

	return engine.discoverKnowledge(action.Knowledge, boundary)
}

// joinParty asks the NPC to join. Success ends the conversation; a full party
// waits for a key and then returns to topics.
func (engine *SemanticConversationEngine) joinParty() *ConversationResponse {
	outcome, err := engine.callbacks.JoinParty()
	if err != nil {
		return engine.errorResponse(err)
	}
	switch outcome {
	case JoinPartySuccess:
		engine.active = false
		engine.frames = nil

		return engine.dismissalResponse()
	case JoinPartyCapacityRefused:
		engine.output.closeQuote()
		engine.frames = []semanticFrame{{body: nil, index: 0}}
		engine.currentQuestion = ""
		engine.completion = semanticCompletionTopics
		engine.waitingForKey = true

		return engine.waitResponse()
	case JoinPartyUnexpectedError:
		return engine.errorResponse(errJoinPartyFailed)
	default:
		return engine.errorResponse(fmt.Errorf("%w: %d", errJoinPartyOutcomeUnknown, outcome))
	}
}

func (engine *SemanticConversationEngine) completeBody() *ConversationResponse {
	switch engine.completion {
	case semanticCompletionBootstrapDescription:
		engine.output.closeQuote()
		engine.output.append("\n\n", false)
		// REDUX DEVIATION: Original introduction randomly introduces an unmet NPC by
		// name (ref:TALK-0004). Redux preserves its established first-meeting flow and waits for NAME.
		if (engine.hasMet || engine.alwaysShowGreeting) && len(engine.conversation.Greeting) > 0 {
			engine.output.openQuote()
			engine.startBody(engine.conversation.Greeting, semanticCompletionBootstrapGreeting)

			return nil
		}

		return engine.topicResponse()
	case semanticCompletionBootstrapGreeting:
		engine.output.closeQuote()
		engine.output.append("\n\n", false)

		return engine.topicResponse()
	case semanticCompletionQuestionPrompt:
		engine.output.closeQuote()
		engine.output.append("\n\n", false)

		return engine.inputResponse(promptRespond, true)
	case semanticCompletionFarewell:
		engine.active = false
		engine.output.closeQuote()
		engine.output.append("\n", false)

		return engine.dismissalResponse()
	case semanticCompletionTopics:
		engine.currentQuestion = ""
		engine.output.closeQuote()
		engine.output.append("\n\n", false)

		return engine.topicResponse()
	default:
		return engine.errorResponse(fmt.Errorf("%w: %d", errSemanticCompletionUnknown, engine.completion))
	}
}

func (engine *SemanticConversationEngine) selectBranch(branch *semantic.BranchAction) semantic.Body {
	matched := false
	switch condition := branch.Condition.(type) {
	case *semantic.NPCKnowsPartyNameCondition:
		matched = engine.hasMet
	case *semantic.KarmaAtLeastCondition:
		matched = engine.callbacks.GetKarmaLevel() >= condition.Minimum
	}
	if matched {
		return branch.Then
	}
	if branch.Otherwise != nil {
		return *branch.Otherwise
	}

	return nil
}

func (engine *SemanticConversationEngine) resetOutput() {
	engine.responseSerial++
	engine.output = newSemanticOutput()
}

func (engine *SemanticConversationEngine) topicResponse() *ConversationResponse {
	return engine.inputResponse(promptInterest, false)
}

func (engine *SemanticConversationEngine) waitResponse() *ConversationResponse {
	response := engine.buildResponse(false)
	response.WaitingForKeypress = true

	return response
}

func (engine *SemanticConversationEngine) inputResponse(prompt string, labelQuestion bool) *ConversationResponse {
	response := engine.buildResponse(true)
	response.NeedsInput = true
	response.InputPrompt = prompt
	response.IsLabelQuestion = labelQuestion

	return response
}

func (engine *SemanticConversationEngine) dismissalResponse() *ConversationResponse {
	response := engine.buildResponse(true)
	response.IsCompleteAwaitingDismissal = true

	return response
}

func (engine *SemanticConversationEngine) errorResponse(err error) *ConversationResponse {
	response := engine.buildResponse(true)
	response.Error = err

	return response
}

func (engine *SemanticConversationEngine) buildResponse(closeQuote bool) *ConversationResponse {
	if closeQuote {
		engine.output.closeQuote()
	}
	segments := append([]TextSegment(nil), engine.output.segments...)
	var output strings.Builder
	for _, segment := range segments {
		output.WriteString(segment.Text)
	}

	response := newConversationResponse()
	response.Output = output.String()
	response.Segments = segments
	response.Serial = engine.responseSerial

	return response
}

func (engine *SemanticConversationEngine) appendProtocolSpeech(text string) {
	engine.output.appendTextAction(
		semantic.ActionKindSay,
		semantic.RichText{&semantic.TextPart{Text: text}},
		engine.substitute,
	)
}

func (engine *SemanticConversationEngine) substitute(text string) string {
	text = strings.ReplaceAll(text, "{avatar_name}", engine.callbacks.GetAvatarName())

	return strings.ReplaceAll(text, "{npc_name}", engine.ExtractNPCName())
}

func (engine *SemanticConversationEngine) nameBodyText(body semantic.Body) string {
	var output strings.Builder
	for _, action := range body {
		var content semantic.RichText
		switch value := action.(type) {
		case *semantic.SayAction:
			content = value.Content
		case *semantic.NarrateAction:
			content = value.Content
		default:
			continue
		}
		for _, part := range content {
			switch value := part.(type) {
			case *semantic.TextPart:
				output.WriteString(engine.substituteNameText(value.Text))
			case *semantic.RunicPart:
				output.WriteString(engine.substituteNameText(value.Text))
			case *semantic.LineBreakPart:
				output.WriteByte(' ')
			}
		}
	}

	return strings.TrimSpace(output.String())
}

func (engine *SemanticConversationEngine) substituteNameText(text string) string {
	text = strings.ReplaceAll(text, "{avatar_name}", engine.callbacks.GetAvatarName())

	return strings.ReplaceAll(text, "{npc_name}", fallbackNPCName)
}

func newSemanticOutput() semanticOutput {
	return semanticOutput{segments: nil, quoteOpen: false}
}

func newConversationResponse() *ConversationResponse {
	return &ConversationResponse{
		RevealedName: "", Serial: 0, Output: "", Segments: nil, NeedsInput: false, InputPrompt: "", IsComplete: false,
		IsCompleteAwaitingDismissal: false, Error: nil, WaitingForKeypress: false, IsLabelQuestion: false,
		ContinuesPreviousOutput: false,
	}
}

func (output *semanticOutput) appendTextAction(
	kind semantic.ActionKind,
	content semantic.RichText,
	substitute func(string) string,
) {
	if kind == semantic.ActionKindSay {
		output.openQuote()
	} else {
		output.closeQuote()
	}
	for _, part := range content {
		switch value := part.(type) {
		case *semantic.TextPart:
			output.append(substitute(value.Text), false)
		case *semantic.RunicPart:
			output.append(substitute(value.Text), true)
		case *semantic.LineBreakPart:
			output.append("\n", false)
		}
	}
}

func (*semanticOutput) markControl() {}

func (output *semanticOutput) openQuote() {
	if output.quoteOpen {
		return
	}
	output.append(quoteOpen, false)
	output.quoteOpen = true
}

func (output *semanticOutput) closeQuote() {
	if !output.quoteOpen {
		return
	}
	output.append(quoteClose, false)
	output.quoteOpen = false
}

func (output *semanticOutput) append(text string, runic bool) {
	if text == "" {
		return
	}
	last := len(output.segments) - 1
	if last >= 0 && output.segments[last].IsRunic == runic {
		output.segments[last].Text += text

		return
	}
	output.segments = append(output.segments, TextSegment{Text: text, IsRunic: runic})
}

func matchesAnyKeyword(input string, keywords []string) bool {
	for _, keyword := range keywords {
		if matchesProtocolKeyword(input, strings.ToUpper(strings.TrimSpace(keyword))) {
			return true
		}
	}

	return false
}

func matchesProtocolKeyword(input, keyword string) bool {
	if keyword == "" {
		return false
	}
	for offset := strings.Index(input, keyword); offset >= 0; {
		if offset == 0 || input[offset-1] == ' ' {
			return true
		}
		next := strings.Index(input[offset+1:], keyword)
		if next < 0 {
			return false
		}
		offset += next + 1
	}

	return false
}

func abs(value int) int {
	if value < 0 {
		return -value
	}

	return value
}

var _ Engine = (*SemanticConversationEngine)(nil)
