package semantic

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

const canonicalIndent = 2

// Marshal validates a conversation and returns canonical v1 YAML.
func Marshal(conversation *Conversation) ([]byte, error) {
	if err := Validate(conversation); err != nil {
		return nil, fmt.Errorf("validating semantic conversation for YAML: %w", err)
	}

	var buffer bytes.Buffer
	encoder := yaml.NewEncoder(&buffer)
	encoder.SetIndent(canonicalIndent)
	if err := encoder.Encode(conversationYAMLNode(conversation)); err != nil {
		return nil, fmt.Errorf("encoding semantic conversation YAML: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return nil, fmt.Errorf("closing semantic conversation YAML encoder: %w", err)
	}

	return buffer.Bytes(), nil
}

func conversationYAMLNode(conversation *Conversation) *yaml.Node {
	root := mappingNode()
	appendField(root, "schema", stringNode(conversation.Schema))
	appendField(root, "protocol", stringNode(conversation.Protocol))
	appendField(root, "id", stringNode(conversation.ID))
	appendField(root, "origin", originYAMLNode(conversation.Origin))
	appendField(root, "name", bodyYAMLNode(conversation.Name, ActionKindSay))
	appendField(root, "description", bodyYAMLNode(conversation.Description, ActionKindNarrate))
	appendField(root, "greeting", bodyYAMLNode(conversation.Greeting, ActionKindSay))
	appendField(root, "job", bodyYAMLNode(conversation.Job, ActionKindSay))
	appendField(root, "farewell", bodyYAMLNode(conversation.Farewell, ActionKindSay))
	appendField(root, "topics", topicsYAMLNode(conversation.Topics))
	appendField(root, "questions", questionsYAMLNode(conversation.Questions))

	return root
}

func originYAMLNode(origin Origin) *yaml.Node {
	node := mappingNode()
	switch value := origin.(type) {
	case *OriginalOrigin:
		appendField(node, fieldKind, stringNode(string(OriginKindOriginal)))
		appendField(node, fieldSource, stringNode(string(value.Source)))
		appendField(node, fieldDialog, integerNode(value.Dialog))
	case *ReduxOrigin:
		appendField(node, fieldKind, stringNode(string(OriginKindRedux)))
	}

	return node
}

func topicsYAMLNode(topics []Topic) *yaml.Node {
	node := sequenceNode()
	for _, topic := range topics {
		entry := mappingNode()
		appendField(entry, fieldKeywords, stringsYAMLNode(topic.Keywords))
		appendField(entry, fieldResponse, bodyYAMLNode(topic.Response, ActionKindSay))
		node.Content = append(node.Content, entry)
	}

	return node
}

func questionsYAMLNode(questions map[string]Question) *yaml.Node {
	node := mappingNode()
	names := make([]string, 0, len(questions))
	for name := range questions {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		appendField(node, name, questionYAMLNode(questions[name]))
	}

	return node
}

func questionYAMLNode(question Question) *yaml.Node {
	node := mappingNode()
	appendField(node, fieldPrompt, bodyYAMLNode(question.Prompt, ActionKindSay))
	answers := sequenceNode()
	for _, answer := range question.Answers {
		entry := mappingNode()
		appendField(entry, fieldKeywords, stringsYAMLNode(answer.Keywords))
		appendField(entry, fieldResponse, bodyYAMLNode(answer.Response, ActionKindSay))
		answers.Content = append(answers.Content, entry)
	}
	appendField(node, fieldAnswers, answers)
	appendField(node, fieldDefault, bodyYAMLNode(question.Default, ActionKindSay))

	return node
}

func bodyYAMLNode(body Body, shorthandKind ActionKind) *yaml.Node {
	if text, compact := compactBodyText(body, shorthandKind); compact {
		return stringNode(text)
	}

	return explicitBodyYAMLNode(body)
}

func compactBodyText(body Body, shorthandKind ActionKind) (string, bool) {
	if len(body) != 1 {
		return "", false
	}
	var content RichText
	switch value := body[0].(type) {
	case *SayAction:
		if shorthandKind != ActionKindSay {
			return "", false
		}
		content = value.Content
	case *NarrateAction:
		if shorthandKind != ActionKindNarrate {
			return "", false
		}
		content = value.Content
	default:
		return "", false
	}
	if len(content) != 1 {
		return "", false
	}
	text, ok := content[0].(*TextPart)
	if !ok {
		return "", false
	}

	return text.Text, true
}

func explicitBodyYAMLNode(body Body) *yaml.Node {
	node := sequenceNode()
	for _, action := range body {
		node.Content = append(node.Content, actionYAMLNode(action))
	}

	return node
}

//nolint:cyclop // The type switch exhaustively serializes the closed action union.
func actionYAMLNode(action Action) *yaml.Node {
	switch value := action.(type) {
	case *SayAction:
		return singleFieldMapping(string(ActionKindSay), richTextYAMLNode(value.Content))
	case *NarrateAction:
		return singleFieldMapping(string(ActionKindNarrate), richTextYAMLNode(value.Content))
	case *AskAction:
		return singleFieldMapping(string(ActionKindAsk), stringNode(value.Question))
	case *BranchAction:
		return singleFieldMapping(string(ActionKindBranch), branchYAMLNode(value))
	case *TakeGoldAction:
		return singleFieldMapping(string(ActionKindTakeGold), integerNode(value.Amount))
	case *GiveItemAction:
		return singleFieldMapping(string(ActionKindGiveItem), itemGrantYAMLNode(value))
	case *AdjustKarmaAction:
		return singleFieldMapping(string(ActionKindAdjustKarma), integerNode(value.Delta))
	case *DiscoverKnowledgeAction:
		return singleFieldMapping(string(ActionKindDiscoverKnowledge), stringNode(string(value.Knowledge)))
	case *PauseAction:
		return stringNode(string(ActionKindPause))
	case *WaitForKeyAction:
		return stringNode(string(ActionKindWaitForKey))
	case *AskNameAction:
		return stringNode(string(ActionKindAskName))
	case *JoinPartyAction:
		return stringNode(string(ActionKindJoinParty))
	case *CallGuardsAction:
		return stringNode(string(ActionKindCallGuards))
	case *GoToJailAction:
		return stringNode(string(ActionKindGoToJail))
	case *ReturnToTopicsAction:
		return stringNode(string(ActionKindReturnToTopics))
	case *EndAction:
		return stringNode(string(ActionKindEnd))
	default:
		return nil
	}
}

func branchYAMLNode(branch *BranchAction) *yaml.Node {
	node := mappingNode()
	appendField(node, "condition", conditionYAMLNode(branch.Condition))
	appendField(node, "then", explicitBodyYAMLNode(branch.Then))
	if branch.Otherwise != nil {
		appendField(node, "otherwise", explicitBodyYAMLNode(*branch.Otherwise))
	}

	return node
}

func conditionYAMLNode(condition Condition) *yaml.Node {
	switch value := condition.(type) {
	case *NPCKnowsPartyNameCondition:
		return stringNode(string(ConditionKindNPCKnowsPartyName))
	case *KarmaAtLeastCondition:
		return singleFieldMapping(string(ConditionKindKarmaAtLeast), integerNode(value.Minimum))
	default:
		return nil
	}
}

func itemGrantYAMLNode(action *GiveItemAction) *yaml.Node {
	if action.Quantity == 1 {
		return stringNode(action.Item)
	}
	node := mappingNode()
	appendField(node, "item", stringNode(action.Item))
	appendField(node, "quantity", integerNode(action.Quantity))

	return node
}

func richTextYAMLNode(text RichText) *yaml.Node {
	if len(text) == 1 {
		if part, ok := text[0].(*TextPart); ok {
			return stringNode(part.Text)
		}
	}
	node := sequenceNode()
	for _, part := range text {
		switch value := part.(type) {
		case *TextPart:
			node.Content = append(node.Content, singleFieldMapping(string(InlineKindText), stringNode(value.Text)))
		case *RunicPart:
			node.Content = append(node.Content, singleFieldMapping(string(InlineKindRunic), stringNode(value.Text)))
		case *LineBreakPart:
			node.Content = append(node.Content, stringNode(string(InlineKindLineBreak)))
		}
	}

	return node
}

func stringsYAMLNode(values []string) *yaml.Node {
	node := sequenceNode()
	for _, value := range values {
		node.Content = append(node.Content, stringNode(value))
	}

	return node
}

func mappingNode() *yaml.Node {
	return &yaml.Node{ //nolint:exhaustruct_v5 // YAML emitter metadata uses zero values.
		Kind: yaml.MappingNode,
		Tag:  "!!map",
	}
}

func sequenceNode() *yaml.Node {
	return &yaml.Node{ //nolint:exhaustruct_v5 // YAML emitter metadata uses zero values.
		Kind: yaml.SequenceNode,
		Tag:  "!!seq",
	}
}

func singleFieldMapping(name string, value *yaml.Node) *yaml.Node {
	node := mappingNode()
	appendField(node, name, value)

	return node
}

func appendField(node *yaml.Node, name string, value *yaml.Node) {
	node.Content = append(node.Content, stringNode(name), value)
}

func stringNode(value string) *yaml.Node {
	node := &yaml.Node{ //nolint:exhaustruct_v5 // YAML emitter metadata uses zero values.
		Kind:  yaml.ScalarNode,
		Tag:   yamlStringTag,
		Value: value,
	}
	if strings.ContainsAny(value, "\r\n") {
		node.Style = yaml.DoubleQuotedStyle
	}

	return node
}

func integerNode(value int) *yaml.Node {
	return &yaml.Node{ //nolint:exhaustruct_v5 // YAML emitter metadata uses zero values.
		Kind:  yaml.ScalarNode,
		Tag:   yamlIntegerTag,
		Value: strconv.Itoa(value),
	}
}
