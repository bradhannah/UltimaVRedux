package semantic

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

const (
	rootPath         = "$"
	yamlStringTag    = "!!str"
	yamlIntegerTag   = "!!int"
	mappingPairSize  = 2
	fieldDialog      = "dialog"
	fieldKeywords    = "keywords"
	fieldResponse    = "response"
	fieldQuestions   = "questions"
	fieldDescription = "description"
	fieldOrigin      = "origin"
	fieldSource      = "source"
	fieldKind        = "kind"
	fieldPrompt      = "prompt"
	fieldAnswers     = "answers"
	fieldDefault     = "default"
)

type sourcePosition struct {
	line   int
	column int
}

type documentParser struct {
	positions map[string]sourcePosition
}

// LoadFile strictly loads and validates one semantic conversation YAML file.
func LoadFile(filename string) (*Conversation, error) {
	file, err := os.Open(filename) //nolint:gosec // The caller explicitly selects the conversation file.
	if err != nil {
		return nil, fmt.Errorf("opening semantic conversation %q: %w", filename, err)
	}
	defer file.Close() //nolint:errcheck // The read result is authoritative for a read-only file.

	conversation, err := Load(filename, file)
	if err != nil {
		return nil, err
	}

	return conversation, nil
}

// Load strictly decodes and validates one semantic conversation YAML document.
// The source name is included in returned errors and need not be a filesystem path.
func Load(source string, reader io.Reader) (*Conversation, error) {
	if reader == nil {
		err := &documentError{diagnostic: Diagnostic{
			Path: rootPath, Line: 0, Column: 0, Message: "reader is nil",
		}}

		return nil, fmt.Errorf("loading semantic conversation %q: %w", source, err)
	}
	root, err := decodeOneDocument(reader)
	if err != nil {
		return nil, fmt.Errorf("loading semantic conversation %q: %w", source, err)
	}

	if err := inspectYAML(root, rootPath); err != nil {
		return nil, fmt.Errorf("loading semantic conversation %q: %w", source, err)
	}

	parser := documentParser{positions: make(map[string]sourcePosition)}
	conversation, err := parser.parseConversation(root)
	if err != nil {
		return nil, fmt.Errorf("loading semantic conversation %q: %w", source, err)
	}

	if err := validate(conversation, parser.positions); err != nil {
		return nil, fmt.Errorf("loading semantic conversation %q: %w", source, err)
	}

	return conversation, nil
}

func decodeOneDocument(reader io.Reader) (*yaml.Node, error) {
	decoder := yaml.NewDecoder(reader)
	var document yaml.Node
	if err := decoder.Decode(&document); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, &documentError{diagnostic: Diagnostic{
				Path: rootPath, Line: 0, Column: 0, Message: "document is empty",
			}}
		}

		return nil, fmt.Errorf("decoding YAML: %w", err)
	}

	var extra yaml.Node
	err := decoder.Decode(&extra)
	if err == nil {
		return nil, &documentError{diagnostic: diagnosticForNode(&extra, rootPath, "multiple YAML documents are not allowed")}
	}
	if !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("checking for additional YAML documents: %w", err)
	}

	if document.Kind != yaml.DocumentNode || len(document.Content) != 1 {
		return nil, &documentError{diagnostic: diagnosticForNode(&document, rootPath, "expected one YAML document")}
	}

	return document.Content[0], nil
}

func inspectYAML(node *yaml.Node, path string) error {
	if node.Anchor != "" {
		return &documentError{diagnostic: diagnosticForNode(node, path, "YAML anchors are not allowed")}
	}
	if node.Kind == yaml.AliasNode {
		return &documentError{diagnostic: diagnosticForNode(node, path, "YAML aliases are not allowed")}
	}
	if !isCoreYAMLTag(node.Tag) {
		return &documentError{diagnostic: diagnosticForNode(node, path, "custom YAML tags are not allowed")}
	}

	switch node.Kind {
	case yaml.MappingNode:
		return inspectYAMLMapping(node, path)
	case yaml.SequenceNode:
		for index, child := range node.Content {
			if err := inspectYAML(child, indexPath(path, index)); err != nil {
				return err
			}
		}
	case yaml.ScalarNode:
		return nil
	case yaml.DocumentNode:
		return &documentError{diagnostic: diagnosticForNode(node, path, "unsupported YAML node")}
	case yaml.AliasNode:
		return &documentError{diagnostic: diagnosticForNode(node, path, "YAML aliases are not allowed")}
	case 0:
		return &documentError{diagnostic: diagnosticForNode(node, path, "unsupported YAML node")}
	}

	return nil
}

func inspectYAMLMapping(node *yaml.Node, path string) error {
	seen := make(map[string]*yaml.Node, len(node.Content)/mappingPairSize)
	for index := 0; index < len(node.Content); index += mappingPairSize {
		key := node.Content[index]
		value := node.Content[index+1]
		if key.Anchor != "" {
			return &documentError{diagnostic: diagnosticForNode(key, path, "YAML anchors are not allowed")}
		}
		if key.Value == "<<" || key.Tag == "!!merge" {
			diagnostic := diagnosticForNode(
				key,
				fieldPath(path, key.Value),
				"YAML merge keys are not allowed",
			)

			return &documentError{diagnostic: diagnostic}
		}
		if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag {
			return &documentError{diagnostic: diagnosticForNode(key, path, "mapping keys must be strings")}
		}
		childPath := fieldPath(path, key.Value)
		if first, exists := seen[key.Value]; exists {
			message := fmt.Sprintf(
				"duplicate mapping key %q (first declared at line %d, column %d)",
				key.Value,
				first.Line,
				first.Column,
			)

			return &documentError{diagnostic: diagnosticForNode(key, childPath, message)}
		}
		seen[key.Value] = key
		if err := inspectYAML(value, childPath); err != nil {
			return err
		}
	}

	return nil
}

func isCoreYAMLTag(tag string) bool {
	switch tag {
	case "", "!!map", "!!seq", yamlStringTag, yamlIntegerTag, "!!null", "!!bool", "!!float", "!!timestamp":
		return true
	default:
		return false
	}
}

func (parser *documentParser) parseConversation(node *yaml.Node) (*Conversation, error) {
	rootFields := []string{
		"schema", "protocol", "id", fieldOrigin, "name", fieldDescription,
		"greeting", "job", "farewell", "topics", fieldQuestions,
	}
	fields, err := parser.mapping(node, rootPath, rootFields, rootFields)
	if err != nil {
		return nil, err
	}

	conversation := &Conversation{
		Schema: "", Protocol: "", ID: "", Origin: nil,
		Name: nil, Description: nil, Greeting: nil, Job: nil, Farewell: nil,
		Topics: nil, Questions: nil,
	}
	if conversation.Schema, err = parser.stringValue(fields["schema"], "$.schema"); err != nil {
		return nil, err
	}
	if conversation.Protocol, err = parser.stringValue(fields["protocol"], "$.protocol"); err != nil {
		return nil, err
	}
	if conversation.ID, err = parser.stringValue(fields["id"], "$.id"); err != nil {
		return nil, err
	}
	if conversation.Origin, err = parser.parseOrigin(fields[fieldOrigin], "$.origin"); err != nil {
		return nil, err
	}
	if conversation.Name, err = parser.parseBody(fields["name"], "$.name", ActionKindSay); err != nil {
		return nil, err
	}
	if conversation.Description, err = parser.parseBody(
		fields[fieldDescription],
		"$.description",
		ActionKindNarrate,
	); err != nil {
		return nil, err
	}
	if conversation.Greeting, err = parser.parseBody(fields["greeting"], "$.greeting", ActionKindSay); err != nil {
		return nil, err
	}
	if conversation.Job, err = parser.parseBody(fields["job"], "$.job", ActionKindSay); err != nil {
		return nil, err
	}
	if conversation.Farewell, err = parser.parseBody(fields["farewell"], "$.farewell", ActionKindSay); err != nil {
		return nil, err
	}
	if conversation.Topics, err = parser.parseTopics(fields["topics"], "$.topics"); err != nil {
		return nil, err
	}
	if conversation.Questions, err = parser.parseQuestions(fields[fieldQuestions], "$.questions"); err != nil {
		return nil, err
	}

	return conversation, nil
}

//nolint:ireturn // Origin is a closed sum type represented by an interface.
func (parser *documentParser) parseOrigin(node *yaml.Node, path string) (Origin, error) {
	originFields := []string{fieldKind, fieldSource, fieldDialog}
	baseFields, err := parser.mapping(node, path, originFields, []string{fieldKind})
	if err != nil {
		return nil, err
	}
	kind, err := parser.stringValue(baseFields[fieldKind], fieldPath(path, fieldKind))
	if err != nil {
		return nil, err
	}

	switch OriginKind(kind) {
	case OriginKindRedux:
		if len(baseFields) != 1 {
			return nil, parser.error(node, path, "redux origin only accepts the kind field")
		}

		return &ReduxOrigin{}, nil
	case OriginKindOriginal:
		fields, mappingErr := parser.mapping(node, path, originFields, originFields)
		if mappingErr != nil {
			return nil, mappingErr
		}
		source, sourceErr := parser.stringValue(fields[fieldSource], fieldPath(path, fieldSource))
		if sourceErr != nil {
			return nil, sourceErr
		}
		dialog, dialogErr := parser.integerValue(fields[fieldDialog], fieldPath(path, fieldDialog))
		if dialogErr != nil {
			return nil, dialogErr
		}

		return &OriginalOrigin{Source: OriginalSource(source), Dialog: dialog}, nil
	default:
		return nil, parser.error(
			baseFields[fieldKind],
			fieldPath(path, fieldKind),
			fmt.Sprintf("unknown origin kind %q", kind),
		)
	}
}

func (parser *documentParser) parseTopics(node *yaml.Node, path string) ([]Topic, error) {
	entries, err := parser.parseResponses(node, path)
	if err != nil {
		return nil, err
	}
	topics := make([]Topic, 0, len(entries))
	for _, entry := range entries {
		topics = append(topics, Topic{Keywords: entry.keywords, Response: entry.response})
	}

	return topics, nil
}

func (parser *documentParser) parseQuestions(node *yaml.Node, path string) (map[string]Question, error) {
	if err := parser.requireKind(node, path, yaml.MappingNode, "mapping"); err != nil {
		return nil, err
	}
	questions := make(map[string]Question, len(node.Content)/mappingPairSize)
	for index := 0; index < len(node.Content); index += mappingPairSize {
		nameNode := node.Content[index]
		questionNode := node.Content[index+1]
		if nameNode.Kind != yaml.ScalarNode || nameNode.Tag != yamlStringTag {
			return nil, parser.error(nameNode, path, "question names must be strings")
		}
		questionPath := fieldPath(path, nameNode.Value)
		parser.mark(questionPath, nameNode)
		question, err := parser.parseQuestion(questionNode, questionPath)
		if err != nil {
			return nil, err
		}
		questions[nameNode.Value] = question
	}

	return questions, nil
}

func (parser *documentParser) parseQuestion(node *yaml.Node, path string) (Question, error) {
	questionFields := []string{fieldPrompt, fieldAnswers, fieldDefault}
	fields, err := parser.mapping(node, path, questionFields, questionFields)
	if err != nil {
		return Question{}, err
	}
	prompt, err := parser.parseBody(fields[fieldPrompt], fieldPath(path, fieldPrompt), ActionKindSay)
	if err != nil {
		return Question{}, err
	}
	answers, err := parser.parseAnswers(fields[fieldAnswers], fieldPath(path, fieldAnswers))
	if err != nil {
		return Question{}, err
	}
	defaultBody, err := parser.parseBody(fields[fieldDefault], fieldPath(path, fieldDefault), ActionKindSay)
	if err != nil {
		return Question{}, err
	}

	return Question{Prompt: prompt, Answers: answers, Default: defaultBody}, nil
}

func (parser *documentParser) parseAnswers(node *yaml.Node, path string) ([]Answer, error) {
	entries, err := parser.parseResponses(node, path)
	if err != nil {
		return nil, err
	}
	answers := make([]Answer, 0, len(entries))
	for _, entry := range entries {
		answers = append(answers, Answer{Keywords: entry.keywords, Response: entry.response})
	}

	return answers, nil
}

type responseEntry struct {
	keywords []string
	response Body
}

func (parser *documentParser) parseResponses(node *yaml.Node, path string) ([]responseEntry, error) {
	if err := parser.requireKind(node, path, yaml.SequenceNode, "sequence"); err != nil {
		return nil, err
	}
	entries := make([]responseEntry, 0, len(node.Content))
	responseFields := []string{fieldKeywords, fieldResponse}
	for index, responseNode := range node.Content {
		responsePath := indexPath(path, index)
		fields, err := parser.mapping(responseNode, responsePath, responseFields, responseFields)
		if err != nil {
			return nil, err
		}
		keywords, err := parser.parseKeywords(fields[fieldKeywords], fieldPath(responsePath, fieldKeywords))
		if err != nil {
			return nil, err
		}
		response, err := parser.parseBody(fields[fieldResponse], fieldPath(responsePath, fieldResponse), ActionKindSay)
		if err != nil {
			return nil, err
		}
		entries = append(entries, responseEntry{keywords: keywords, response: response})
	}

	return entries, nil
}

func (parser *documentParser) parseKeywords(node *yaml.Node, path string) ([]string, error) {
	if err := parser.requireKind(node, path, yaml.SequenceNode, "sequence"); err != nil {
		return nil, err
	}
	keywords := make([]string, 0, len(node.Content))
	for index, keywordNode := range node.Content {
		keyword, err := parser.stringValue(keywordNode, indexPath(path, index))
		if err != nil {
			return nil, err
		}
		keywords = append(keywords, keyword)
	}

	return keywords, nil
}

func (parser *documentParser) parseBody(node *yaml.Node, path string, shorthandKind ActionKind) (Body, error) {
	parser.mark(path, node)
	if node.Kind == yaml.ScalarNode {
		text, err := parser.stringValue(node, path)
		if err != nil {
			return nil, err
		}
		actionPath := indexPath(path, 0)
		parser.mark(actionPath, node)
		parser.mark(fieldPath(actionPath, string(shorthandKind)), node)
		parser.mark(indexPath(fieldPath(actionPath, string(shorthandKind)), 0), node)
		parser.mark(fieldPath(indexPath(fieldPath(actionPath, string(shorthandKind)), 0), string(InlineKindText)), node)
		content := RichText{&TextPart{Text: text}}
		if shorthandKind == ActionKindNarrate {
			return Body{&NarrateAction{Content: content}}, nil
		}

		return Body{&SayAction{Content: content}}, nil
	}

	return parser.parseExplicitBody(node, path)
}

func (parser *documentParser) parseExplicitBody(node *yaml.Node, path string) (Body, error) {
	if err := parser.requireKind(node, path, yaml.SequenceNode, "action sequence"); err != nil {
		return nil, err
	}
	body := make(Body, 0, len(node.Content))
	for index, actionNode := range node.Content {
		action, err := parser.parseAction(actionNode, indexPath(path, index))
		if err != nil {
			return nil, err
		}
		body = append(body, action)
	}

	return body, nil
}

//nolint:cyclop,ireturn // Action is a closed sum type and this switch is its grammar dispatch.
func (parser *documentParser) parseAction(node *yaml.Node, path string) (Action, error) {
	parser.mark(path, node)
	if node.Kind == yaml.ScalarNode {
		return parser.parseBareAction(node, path)
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return nil, parser.error(node, path, "actions must be a bare action name or a one-key mapping")
	}
	key := node.Content[0]
	value := node.Content[1]
	if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag {
		return nil, parser.error(key, path, "action names must be strings")
	}
	actionPath := fieldPath(path, key.Value)
	parser.mark(actionPath, key)

	switch ActionKind(key.Value) {
	case ActionKindSay:
		content, err := parser.parseRichText(value, actionPath)

		return &SayAction{Content: content}, err
	case ActionKindNarrate:
		content, err := parser.parseRichText(value, actionPath)

		return &NarrateAction{Content: content}, err
	case ActionKindAsk:
		question, err := parser.stringValue(value, actionPath)

		return &AskAction{Question: question}, err
	case ActionKindBranch:
		return parser.parseBranch(value, actionPath)
	case ActionKindTakeGold:
		amount, err := parser.integerValue(value, actionPath)

		return &TakeGoldAction{Amount: amount}, err
	case ActionKindGiveItem:
		return parser.parseGiveItem(value, actionPath)
	case ActionKindAdjustKarma:
		delta, err := parser.integerValue(value, actionPath)

		return &AdjustKarmaAction{Delta: delta}, err
	case ActionKindDiscoverKnowledge:
		knowledge, err := parser.stringValue(value, actionPath)

		return &DiscoverKnowledgeAction{Knowledge: KnowledgeID(knowledge)}, err
	case ActionKindPause, ActionKindWaitForKey, ActionKindAskName, ActionKindJoinParty,
		ActionKindCallGuards, ActionKindGoToJail, ActionKindBeginCircleStudy, ActionKindReturnToTopics,
		ActionKindEnd:
		return nil, parser.error(key, actionPath, fmt.Sprintf("action %q must use its bare form", key.Value))
	default:
		return nil, parser.error(key, actionPath, fmt.Sprintf("unknown action %q", key.Value))
	}
}

//nolint:ireturn // Action is a closed sum type and this switch enumerates every bare variant.
func (parser *documentParser) parseBareAction(node *yaml.Node, path string) (Action, error) {
	name, err := parser.stringValue(node, path)
	if err != nil {
		return nil, err
	}
	switch ActionKind(name) {
	case ActionKindPause:
		return &PauseAction{}, nil
	case ActionKindWaitForKey:
		return &WaitForKeyAction{}, nil
	case ActionKindAskName:
		return &AskNameAction{}, nil
	case ActionKindJoinParty:
		return &JoinPartyAction{}, nil
	case ActionKindCallGuards:
		return &CallGuardsAction{}, nil
	case ActionKindGoToJail:
		return &GoToJailAction{}, nil
	case ActionKindBeginCircleStudy:
		return &BeginCircleStudyAction{}, nil
	case ActionKindReturnToTopics:
		return &ReturnToTopicsAction{}, nil
	case ActionKindEnd:
		return &EndAction{}, nil
	case ActionKindSay, ActionKindNarrate, ActionKindAsk, ActionKindBranch,
		ActionKindTakeGold, ActionKindGiveItem, ActionKindAdjustKarma, ActionKindDiscoverKnowledge:
		return nil, parser.error(node, path, fmt.Sprintf("action %q requires a mapping value", name))
	default:
		return nil, parser.error(node, path, fmt.Sprintf("unknown bare action %q", name))
	}
}

//nolint:ireturn // Action is a closed sum type represented by an interface.
func (parser *documentParser) parseBranch(node *yaml.Node, path string) (Action, error) {
	fields, err := parser.mapping(node, path, []string{"condition", "then", "otherwise"}, []string{"condition", "then"})
	if err != nil {
		return nil, err
	}
	condition, err := parser.parseCondition(fields["condition"], fieldPath(path, "condition"))
	if err != nil {
		return nil, err
	}
	thenBody, err := parser.parseExplicitBody(fields["then"], fieldPath(path, "then"))
	if err != nil {
		return nil, err
	}
	var otherwise *Body
	if otherwiseNode, exists := fields["otherwise"]; exists {
		body, bodyErr := parser.parseExplicitBody(otherwiseNode, fieldPath(path, "otherwise"))
		if bodyErr != nil {
			return nil, bodyErr
		}
		otherwise = &body
	}

	return &BranchAction{Condition: condition, Then: thenBody, Otherwise: otherwise}, nil
}

//nolint:ireturn // Condition is a closed sum type represented by an interface.
func (parser *documentParser) parseCondition(node *yaml.Node, path string) (Condition, error) {
	parser.mark(path, node)
	if node.Kind == yaml.ScalarNode {
		condition, err := parser.stringValue(node, path)
		if err != nil {
			return nil, err
		}
		if ConditionKind(condition) != ConditionKindNPCKnowsPartyName {
			return nil, parser.error(node, path, fmt.Sprintf("unknown bare condition %q", condition))
		}

		return &NPCKnowsPartyNameCondition{}, nil
	}
	if node.Kind != yaml.MappingNode || len(node.Content) != 2 {
		return nil, parser.error(node, path, "condition must be npc_knows_party_name or a one-key mapping")
	}
	key := node.Content[0]
	if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag || ConditionKind(key.Value) != ConditionKindKarmaAtLeast {
		return nil, parser.error(key, path, "unknown branch condition")
	}
	minimum, err := parser.integerValue(node.Content[1], fieldPath(path, key.Value))
	if err != nil {
		return nil, err
	}

	return &KarmaAtLeastCondition{Minimum: minimum}, nil
}

//nolint:ireturn // Action is a closed sum type represented by an interface.
func (parser *documentParser) parseGiveItem(node *yaml.Node, path string) (Action, error) {
	parser.mark(path, node)
	if node.Kind == yaml.ScalarNode {
		item, err := parser.stringValue(node, path)

		return &GiveItemAction{Item: item, Quantity: 1}, err
	}
	fields, err := parser.mapping(node, path, []string{"item", "quantity"}, []string{"item", "quantity"})
	if err != nil {
		return nil, err
	}
	item, err := parser.stringValue(fields["item"], fieldPath(path, "item"))
	if err != nil {
		return nil, err
	}
	quantity, err := parser.integerValue(fields["quantity"], fieldPath(path, "quantity"))
	if err != nil {
		return nil, err
	}

	return &GiveItemAction{Item: item, Quantity: quantity}, nil
}

//nolint:cyclop,funlen // The loop exhaustively decodes the closed inline-part grammar.
func (parser *documentParser) parseRichText(node *yaml.Node, path string) (RichText, error) {
	parser.mark(path, node)
	if node.Kind == yaml.ScalarNode {
		text, err := parser.stringValue(node, path)
		if err != nil {
			return nil, err
		}
		parser.mark(indexPath(path, 0), node)
		parser.mark(fieldPath(indexPath(path, 0), string(InlineKindText)), node)

		return RichText{&TextPart{Text: text}}, nil
	}
	if err := parser.requireKind(node, path, yaml.SequenceNode, "string or inline-part sequence"); err != nil {
		return nil, err
	}
	parts := make(RichText, 0, len(node.Content))
	for index, partNode := range node.Content {
		partPath := indexPath(path, index)
		parser.mark(partPath, partNode)
		if partNode.Kind == yaml.ScalarNode {
			name, err := parser.stringValue(partNode, partPath)
			if err != nil {
				return nil, err
			}
			if InlineKind(name) != InlineKindLineBreak {
				return nil, parser.error(partNode, partPath, "the only bare inline part is line_break")
			}
			parts = append(parts, &LineBreakPart{})

			continue
		}
		if partNode.Kind != yaml.MappingNode || len(partNode.Content) != 2 {
			return nil, parser.error(partNode, partPath, "inline parts must be line_break or a one-key mapping")
		}
		key := partNode.Content[0]
		value := partNode.Content[1]
		if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag {
			return nil, parser.error(key, partPath, "inline part names must be strings")
		}
		text, err := parser.stringValue(value, fieldPath(partPath, key.Value))
		if err != nil {
			return nil, err
		}
		switch InlineKind(key.Value) {
		case InlineKindText:
			parts = append(parts, &TextPart{Text: text})
		case InlineKindRunic:
			parts = append(parts, &RunicPart{Text: text})
		case InlineKindLineBreak:
			return nil, parser.error(key, partPath, "line_break must use its bare form")
		default:
			return nil, parser.error(key, partPath, fmt.Sprintf("unknown inline part %q", key.Value))
		}
	}

	return parts, nil
}

func (parser *documentParser) mapping(
	node *yaml.Node,
	path string,
	allowed, required []string,
) (map[string]*yaml.Node, error) {
	parser.mark(path, node)
	if err := parser.requireKind(node, path, yaml.MappingNode, "mapping"); err != nil {
		return nil, err
	}
	fields := make(map[string]*yaml.Node, len(node.Content)/mappingPairSize)
	for index := 0; index < len(node.Content); index += mappingPairSize {
		key := node.Content[index]
		if key.Kind != yaml.ScalarNode || key.Tag != yamlStringTag {
			return nil, parser.error(key, path, "field names must be strings")
		}
		if !contains(allowed, key.Value) {
			return nil, parser.error(key, fieldPath(path, key.Value), fmt.Sprintf("unknown field %q", key.Value))
		}
		fields[key.Value] = node.Content[index+1]
		parser.mark(fieldPath(path, key.Value), node.Content[index+1])
	}
	for _, name := range required {
		if _, exists := fields[name]; !exists {
			return nil, parser.error(node, fieldPath(path, name), fmt.Sprintf("required field %q is missing", name))
		}
	}

	return fields, nil
}

func (parser *documentParser) stringValue(node *yaml.Node, path string) (string, error) {
	parser.mark(path, node)
	if node.Kind != yaml.ScalarNode || node.Tag != yamlStringTag {
		return "", parser.error(node, path, "expected a string")
	}

	return node.Value, nil
}

func (parser *documentParser) integerValue(node *yaml.Node, path string) (int, error) {
	parser.mark(path, node)
	if node.Kind != yaml.ScalarNode || node.Tag != yamlIntegerTag || !isDecimalInteger(node.Value) {
		return 0, parser.error(node, path, "expected a decimal integer")
	}
	value, err := strconv.Atoi(node.Value)
	if err != nil {
		return 0, parser.error(node, path, "integer is outside the supported range")
	}

	return value, nil
}

func isDecimalInteger(value string) bool {
	if value == "0" {
		return true
	}
	start := 0
	if len(value) > 0 && value[0] == '-' {
		start = 1
	}
	if start >= len(value) || value[start] == '0' {
		return false
	}
	for index := start; index < len(value); index++ {
		if value[index] < '0' || value[index] > '9' {
			return false
		}
	}

	return true
}

func (parser *documentParser) requireKind(node *yaml.Node, path string, kind yaml.Kind, description string) error {
	parser.mark(path, node)
	if node.Kind != kind {
		return parser.error(node, path, "expected a "+description)
	}

	return nil
}

func (parser *documentParser) mark(path string, node *yaml.Node) {
	if node == nil {
		return
	}
	parser.positions[path] = sourcePosition{line: node.Line, column: node.Column}
}

func (parser *documentParser) error(node *yaml.Node, path, message string) error {
	return &documentError{diagnostic: diagnosticForNode(node, path, message)}
}

func diagnosticForNode(node *yaml.Node, path, message string) Diagnostic {
	diagnostic := Diagnostic{Path: path, Line: 0, Column: 0, Message: message}
	if node != nil {
		diagnostic.Line = node.Line
		diagnostic.Column = node.Column
	}

	return diagnostic
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}

	return false
}

func fieldPath(parent, field string) string { return parent + "." + field }

func indexPath(parent string, index int) string {
	return parent + "[" + strconv.Itoa(index) + "]"
}
