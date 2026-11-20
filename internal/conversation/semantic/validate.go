package semantic

import (
	"fmt"
	"sort"
	"strings"
)

const (
	maxKarma        = 99
	maxGold         = 65535
	maxItemQuantity = 255
)

type validationContext struct {
	conversation *Conversation
	positions    map[string]sourcePosition
	diagnostics  []Diagnostic
}

// Validate checks a programmatically constructed conversation for all v1
// semantic invariants. YAML structural checks are performed by Load.
func Validate(conversation *Conversation) error {
	return validate(conversation, nil)
}

func validate(conversation *Conversation, positions map[string]sourcePosition) error {
	if conversation == nil {
		return &ValidationError{Diagnostics: []Diagnostic{{
			Path: rootPath, Line: 0, Column: 0, Message: "conversation is nil",
		}}}
	}
	context := validationContext{conversation: conversation, positions: positions, diagnostics: nil}
	context.validateConversation()
	if len(context.diagnostics) > 0 {
		return &ValidationError{Diagnostics: context.diagnostics}
	}

	return nil
}

func (context *validationContext) validateConversation() {
	conversation := context.conversation
	if conversation.Schema != SchemaVersion {
		context.add("$.schema", fmt.Sprintf("must equal %q", SchemaVersion))
	}
	if conversation.Protocol != ProtocolUltima5 {
		context.add("$.protocol", fmt.Sprintf("must equal %q", ProtocolUltima5))
	}
	if !isStableID(conversation.ID) {
		context.add("$.id", "must match ^(original|redux)\\.[a-z0-9][a-z0-9_.-]*$")
	}
	context.validateOrigin(conversation.Origin, "$.origin")
	context.validateRequiredBody(conversation.Name, "$.name")
	context.validateRequiredBody(conversation.Description, "$.description")
	context.validateRequiredBody(conversation.Greeting, "$.greeting")
	context.validateRequiredBody(conversation.Job, "$.job")
	context.validateRequiredBody(conversation.Farewell, "$.farewell")
	if conversation.Topics == nil {
		context.add("$.topics", "required field must not be nil")
	} else {
		context.validateTopics(conversation.Topics)
	}
	if conversation.Questions == nil {
		context.add("$.questions", "required field must not be nil")
	} else {
		context.validateQuestions(conversation.Questions)
	}
}

func (context *validationContext) validateOrigin(origin Origin, path string) {
	switch value := origin.(type) {
	case *OriginalOrigin:
		if value == nil {
			context.add(path, "origin must not be nil")

			return
		}
		switch value.Source {
		case OriginalSourceCastle, OriginalSourceTowne, OriginalSourceDwelling, OriginalSourceKeep:
		default:
			context.add(fieldPath(path, "source"), fmt.Sprintf("unknown original source %q", value.Source))
		}
		if value.Dialog < 1 {
			context.add(fieldPath(path, "dialog"), "must be at least 1")
		}
	case *ReduxOrigin:
		if value == nil {
			context.add(path, "origin must not be nil")
		}
	case nil:
		context.add(path, "origin is required")
	default:
		context.add(path, "unsupported origin variant")
	}
}

func (context *validationContext) validateTopics(topics []Topic) {
	seen := make(map[string]string)
	for index, topic := range topics {
		path := indexPath("$.topics", index)
		context.validateKeywords(topic.Keywords, fieldPath(path, "keywords"), seen)
		context.validateRequiredBody(topic.Response, fieldPath(path, "response"))
	}
}

func (context *validationContext) validateQuestions(questions map[string]Question) {
	names := make([]string, 0, len(questions))
	for name := range questions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		question := questions[name]
		path := fieldPath("$.questions", name)
		if !isQuestionName(name) {
			context.add(path, "name must be question_01 through question_15")
		}
		context.validateRequiredBody(question.Prompt, fieldPath(path, "prompt"))
		if question.Answers == nil {
			context.add(fieldPath(path, "answers"), "required field must not be nil")
		}
		seen := make(map[string]string)
		for index, answer := range question.Answers {
			answerPath := indexPath(fieldPath(path, "answers"), index)
			context.validateKeywords(answer.Keywords, fieldPath(answerPath, "keywords"), seen)
			context.validateRequiredBody(answer.Response, fieldPath(answerPath, "response"))
		}
		context.validateRequiredBody(question.Default, fieldPath(path, "default"))
	}
}

func (context *validationContext) validateKeywords(keywords []string, path string, seen map[string]string) {
	if len(keywords) == 0 {
		context.add(path, "must contain at least one keyword")

		return
	}
	for index, keyword := range keywords {
		keywordPath := indexPath(path, index)
		normalized := normalizeKeyword(keyword)
		if normalized == "" {
			context.add(keywordPath, "keyword must not be empty")

			continue
		}
		if firstPath, exists := seen[normalized]; exists {
			context.add(keywordPath, fmt.Sprintf("duplicate normalized keyword %q; first declared at %s", normalized, firstPath))

			continue
		}
		seen[normalized] = keywordPath
	}
}

func normalizeKeyword(keyword string) string {
	return strings.ToLower(strings.Join(strings.Fields(keyword), " "))
}

func isStableID(identifier string) bool {
	var suffix string
	switch {
	case strings.HasPrefix(identifier, "original."):
		suffix = strings.TrimPrefix(identifier, "original.")
	case strings.HasPrefix(identifier, "redux."):
		suffix = strings.TrimPrefix(identifier, "redux.")
	default:
		return false
	}
	if suffix == "" || !isLowerAlphaNumeric(suffix[0]) {
		return false
	}
	for index := 1; index < len(suffix); index++ {
		if !isLowerAlphaNumeric(suffix[index]) && suffix[index] != '_' && suffix[index] != '.' && suffix[index] != '-' {
			return false
		}
	}

	return true
}

func isLowerAlphaNumeric(value byte) bool {
	return value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func isQuestionName(name string) bool {
	return len(name) == len("question_01") && strings.HasPrefix(name, "question_") &&
		(name[9] == '0' && name[10] >= '1' && name[10] <= '9' ||
			name[9] == '1' && name[10] >= '0' && name[10] <= '5')
}

func (context *validationContext) validateRequiredBody(body Body, path string) {
	if body == nil {
		context.add(path, "required body must not be nil")

		return
	}
	context.validateBody(body, path)
}

//nolint:cyclop,funlen,gocognit,gocyclo // The type switch exhaustively validates the closed action union.
func (context *validationContext) validateBody(body Body, path string) {
	for index, action := range body {
		actionPath := indexPath(path, index)
		if action == nil {
			context.add(actionPath, "action must not be nil")

			continue
		}
		terminal := false
		switch value := action.(type) {
		case *SayAction:
			if value == nil {
				context.add(actionPath, "say action must not be nil")

				continue
			}
			context.validateRichText(value.Content, fieldPath(actionPath, string(ActionKindSay)), true)
		case *NarrateAction:
			if value == nil {
				context.add(actionPath, "narrate action must not be nil")

				continue
			}
			context.validateRichText(value.Content, fieldPath(actionPath, string(ActionKindNarrate)), false)
		case *AskAction:
			terminal = true
			if value == nil {
				context.add(actionPath, "ask action must not be nil")

				continue
			}
			askPath := fieldPath(actionPath, string(ActionKindAsk))
			if _, exists := context.conversation.Questions[value.Question]; !exists {
				context.add(askPath, fmt.Sprintf("references unknown question %q", value.Question))
			}
		case *BranchAction:
			if value == nil {
				context.add(actionPath, "branch action must not be nil")

				continue
			}
			context.validateBranch(value, fieldPath(actionPath, string(ActionKindBranch)))
		case *TakeGoldAction:
			if value == nil {
				context.add(actionPath, "take_gold action must not be nil")

				continue
			}
			if value.Amount < 1 || value.Amount > maxGold {
				context.add(fieldPath(actionPath, string(ActionKindTakeGold)), fmt.Sprintf("must be between 1 and %d", maxGold))
			}
		case *GiveItemAction:
			if value == nil {
				context.add(actionPath, "give_item action must not be nil")

				continue
			}
			itemPath := fieldPath(actionPath, string(ActionKindGiveItem))
			if strings.TrimSpace(value.Item) == "" {
				context.add(fieldPath(itemPath, "item"), "item must not be empty")
			}
			if value.Quantity < 1 || value.Quantity > maxItemQuantity {
				context.add(fieldPath(itemPath, "quantity"), fmt.Sprintf("must be between 1 and %d", maxItemQuantity))
			}
		case *AdjustKarmaAction:
			if value == nil {
				context.add(actionPath, "adjust_karma action must not be nil")

				continue
			}
			if value.Delta == 0 || value.Delta < -maxKarma || value.Delta > maxKarma {
				context.add(
					fieldPath(actionPath, string(ActionKindAdjustKarma)),
					fmt.Sprintf("must be between -%d and %d, excluding zero", maxKarma, maxKarma),
				)
			}
		case *DiscoverKnowledgeAction:
			if value == nil {
				context.add(actionPath, "discover_knowledge action must not be nil")

				continue
			}
			if !isKnownKnowledgeID(value.Knowledge) {
				context.add(
					fieldPath(actionPath, string(ActionKindDiscoverKnowledge)),
					fmt.Sprintf("unknown knowledge id %q", value.Knowledge),
				)
			}
		case *PauseAction:
			if value == nil {
				context.add(actionPath, "pause action must not be nil")
			}
		case *WaitForKeyAction:
			if value == nil {
				context.add(actionPath, "wait_for_key action must not be nil")
			}
		case *AskNameAction:
			if value == nil {
				context.add(actionPath, "ask_name action must not be nil")
			}
		case *JoinPartyAction:
			if value == nil {
				context.add(actionPath, "join_party action must not be nil")
			}
		case *CallGuardsAction:
			if value == nil {
				context.add(actionPath, "call_guards action must not be nil")
			}
		case *GoToJailAction:
			if value == nil {
				context.add(actionPath, "go_to_jail action must not be nil")
			}
		case *BeginCircleStudyAction:
			terminal = true
			if value == nil {
				context.add(actionPath, "begin_circle_study action must not be nil")
			}
		case *ReturnToTopicsAction:
			terminal = true
			if value == nil {
				context.add(actionPath, "return_to_topics action must not be nil")
			}
		case *EndAction:
			terminal = true
			if value == nil {
				context.add(actionPath, "end action must not be nil")
			}
		default:
			context.add(actionPath, "unsupported action variant")
		}
		if terminal && index != len(body)-1 {
			context.add(actionPath, fmt.Sprintf("terminal action %q must be last in its body", action.Kind()))
		}
	}
}

func isKnownKnowledgeID(identifier KnowledgeID) bool {
	switch identifier {
	case KnowledgeSpellAnYlem, KnowledgeSpellInXenMani, KnowledgeSpellRelXenBet,
		KnowledgeShadowlordFaulineiName, KnowledgeShadowlordFaulineiAssociation,
		KnowledgeShadowlordAstarothName, KnowledgeShadowlordAstarothAssociation,
		KnowledgeShadowlordNosfentorName, KnowledgeShadowlordNosfentorAssociation,
		KnowledgeShrineHonestyMantra, KnowledgeShrineCompassionMantra,
		KnowledgeShrineValourMantra, KnowledgeShrineJusticeMantra,
		KnowledgeShrineSacrificeMantra, KnowledgeShrineHonorMantra,
		KnowledgeShrineSpiritualityMantra, KnowledgeShrineHumilityMantra,
		KnowledgeJournalDomainFalsehood, KnowledgeJournalDomainHatred, KnowledgeJournalDomainCowardice,
		KnowledgeJournalFlameTruth, KnowledgeJournalFlameLove, KnowledgeJournalFlameCourage,
		KnowledgeJournalBanishmentMethod,
		KnowledgeJournalResistancePassword,
		KnowledgeJournalOppressionPassword,
		KnowledgeJournalStonesMelody,
		KnowledgeDungeonDeceitWordOfPower, KnowledgeDungeonDespiseWordOfPower,
		KnowledgeDungeonDestardWordOfPower, KnowledgeDungeonWrongWordOfPower,
		KnowledgeDungeonCovetousWordOfPower, KnowledgeDungeonShameWordOfPower,
		KnowledgeDungeonHythlothWordOfPower, KnowledgeDungeonDoomWordOfPower:
		return true
	default:
		return false
	}
}

func (context *validationContext) validateBranch(branch *BranchAction, path string) {
	conditionPath := fieldPath(path, "condition")
	switch condition := branch.Condition.(type) {
	case *NPCKnowsPartyNameCondition:
		if condition == nil {
			context.add(conditionPath, "condition must not be nil")
		}
	case *KarmaAtLeastCondition:
		if condition == nil {
			context.add(conditionPath, "condition must not be nil")
		} else if condition.Minimum < 0 || condition.Minimum > maxKarma {
			context.add(
				fieldPath(conditionPath, string(ConditionKindKarmaAtLeast)),
				fmt.Sprintf("must be between 0 and %d", maxKarma),
			)
		}
	case nil:
		context.add(conditionPath, "condition is required")
	default:
		context.add(conditionPath, "unsupported condition variant")
	}
	if branch.Then == nil {
		context.add(fieldPath(path, "then"), "required body must not be nil")
	} else {
		context.validateBody(branch.Then, fieldPath(path, "then"))
	}
	if branch.Otherwise != nil {
		context.validateBody(*branch.Otherwise, fieldPath(path, "otherwise"))
	}
}

func (context *validationContext) validateRichText(text RichText, path string, oneParagraph bool) {
	if len(text) == 0 {
		context.add(path, "rich text must contain at least one inline part")

		return
	}
	for index, part := range text {
		partPath := indexPath(path, index)
		switch value := part.(type) {
		case *TextPart:
			if value == nil {
				context.add(partPath, "text part must not be nil")
			} else {
				context.validateText(value.Text, fieldPath(partPath, string(InlineKindText)), oneParagraph)
			}
		case *RunicPart:
			if value == nil {
				context.add(partPath, "runic part must not be nil")
			} else {
				context.validateText(value.Text, fieldPath(partPath, string(InlineKindRunic)), oneParagraph)
			}
		case *LineBreakPart:
			if value == nil {
				context.add(partPath, "line_break part must not be nil")
			}
		case nil:
			context.add(partPath, "inline part must not be nil")
		default:
			context.add(partPath, "unsupported inline part variant")
		}
	}
}

func (context *validationContext) validateText(text, path string, oneParagraph bool) {
	if text == "" {
		context.add(path, "text must not be empty")

		return
	}
	if oneParagraph && strings.ContainsAny(text, "\r\n") {
		context.add(path, "text must be one paragraph; use line_break for inline breaks")
	}
	for index := 0; index < len(text); {
		switch text[index] {
		case '{':
			endOffset := strings.IndexByte(text[index+1:], '}')
			if endOffset < 0 {
				context.add(path, "unterminated substitution")

				return
			}
			end := index + 1 + endOffset
			name := text[index+1 : end]
			if strings.ContainsRune(name, '{') {
				context.add(path, "nested substitution braces are not allowed")

				return
			}
			if name != "avatar_name" && name != "npc_name" {
				context.add(path, fmt.Sprintf("unknown substitution %q", name))
			}
			index = end + 1
		case '}':
			context.add(path, "unmatched substitution closing brace")

			return
		default:
			index++
		}
	}
}

func (context *validationContext) add(path, message string) {
	diagnostic := Diagnostic{Path: path, Line: 0, Column: 0, Message: message}
	if position, exists := nearestPosition(context.positions, path); exists {
		diagnostic.Line = position.line
		diagnostic.Column = position.column
	}
	context.diagnostics = append(context.diagnostics, diagnostic)
}

func nearestPosition(positions map[string]sourcePosition, path string) (sourcePosition, bool) {
	for candidate := path; candidate != ""; candidate = parentPath(candidate) {
		if position, exists := positions[candidate]; exists {
			return position, true
		}
	}

	return sourcePosition{line: 0, column: 0}, false
}

func parentPath(path string) string {
	lastField := strings.LastIndexByte(path, '.')
	lastIndex := strings.LastIndexByte(path, '[')
	separator := max(lastField, lastIndex)
	if separator <= 0 {
		return ""
	}

	return path[:separator]
}
