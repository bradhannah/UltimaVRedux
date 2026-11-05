package debugging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
)

var (
	// ErrCommandNotFound indicates that a request references an unknown command ID.
	ErrCommandNotFound = errors.New("debug command not found")
	// ErrInvalidRequest indicates that a request does not satisfy its command schema.
	ErrInvalidRequest = errors.New("invalid debug command request")
	// ErrInvalidCatalog indicates that a command definition is malformed.
	ErrInvalidCatalog = errors.New("invalid debug command catalog")
	// ErrInvalidArgument indicates that an argument does not satisfy its parameter schema.
	ErrInvalidArgument = errors.New("invalid debug command argument")
)

var commandIDPattern = regexp.MustCompile(`^[a-z][a-z0-9_-]*(\.[a-z][a-z0-9_-]*)+$`)

// Catalog stores validated command definitions in stable registration order.
type Catalog struct {
	definitions []CommandDefinition
	byID        map[string]CommandDefinition
}

// NewCatalog validates and constructs a command catalog.
func NewCatalog(definitions []CommandDefinition) (*Catalog, error) {
	catalog := &Catalog{
		definitions: make([]CommandDefinition, 0, len(definitions)),
		byID:        make(map[string]CommandDefinition, len(definitions)),
	}

	for _, definition := range definitions {
		if err := validateDefinition(definition); err != nil {
			return nil, err
		}
		if _, exists := catalog.byID[definition.ID]; exists {
			return nil, fmt.Errorf("%w: duplicate command ID %q", ErrInvalidCatalog, definition.ID)
		}

		definition = cloneDefinition(definition)
		catalog.definitions = append(catalog.definitions, definition)
		catalog.byID[definition.ID] = definition
	}

	return catalog, nil
}

// Definitions returns a defensive copy of the catalog in registration order.
func (c *Catalog) Definitions() []CommandDefinition {
	definitions := make([]CommandDefinition, len(c.definitions))
	for i, definition := range c.definitions {
		definitions[i] = cloneDefinition(definition)
	}

	return definitions
}

// Get returns a defensive copy of one command definition.
func (c *Catalog) Get(commandID string) (CommandDefinition, bool) {
	definition, ok := c.byID[commandID]
	if !ok {
		var empty CommandDefinition

		return empty, false
	}

	return cloneDefinition(definition), true
}

// NormalizeRequest validates a request, deep-copies it, and applies parameter defaults.
func (c *Catalog) NormalizeRequest(request Request) (Request, error) {
	request = cloneRequest(request)
	if err := validateRequestIdentity(request); err != nil {
		return Request{}, err
	}

	definition, ok := c.byID[request.CommandID]
	if !ok {
		return Request{}, fmt.Errorf("%w: %s", ErrCommandNotFound, request.CommandID)
	}

	return normalizeArguments(request, definition)
}

func validateRequestIdentity(request Request) error {
	if request.SessionID == "" {
		return fmt.Errorf("%w: session ID is required", ErrInvalidRequest)
	}
	if request.ID == "" {
		return fmt.Errorf("%w: request ID is required", ErrInvalidRequest)
	}

	return nil
}

func normalizeArguments(request Request, definition CommandDefinition) (Request, error) {
	parameters := make(map[string]ParameterDefinition, len(definition.Parameters))
	for _, parameter := range definition.Parameters {
		parameters[parameter.ID] = parameter
	}
	for argumentID := range request.Arguments {
		if _, ok := parameters[argumentID]; !ok {
			return Request{}, fmt.Errorf("%w: unknown parameter %q", ErrInvalidRequest, argumentID)
		}
	}

	for _, parameter := range definition.Parameters {
		value, supplied := request.Arguments[parameter.ID]
		if !supplied {
			if len(parameter.Default) > 0 {
				if request.Arguments == nil {
					request.Arguments = make(map[string]json.RawMessage, len(definition.Parameters))
				}
				request.Arguments[parameter.ID] = append(json.RawMessage(nil), parameter.Default...)
			} else if parameter.Required {
				return Request{}, fmt.Errorf("%w: parameter %q is required", ErrInvalidRequest, parameter.ID)
			}

			continue
		}
		if err := validateArgument(parameter, value); err != nil {
			return Request{}, fmt.Errorf("%w: parameter %q: %w", ErrInvalidRequest, parameter.ID, err)
		}
	}

	return request, nil
}

func validateDefinition(definition CommandDefinition) error {
	if err := validateDefinitionMetadata(definition); err != nil {
		return err
	}

	parameterIDs := make(map[string]struct{}, len(definition.Parameters))
	for _, parameter := range definition.Parameters {
		if _, exists := parameterIDs[parameter.ID]; exists {
			return fmt.Errorf("%w: command %q has duplicate parameter ID %q", ErrInvalidCatalog, definition.ID, parameter.ID)
		}
		parameterIDs[parameter.ID] = struct{}{}
		if err := validateParameterDefinition(definition.ID, parameter); err != nil {
			return err
		}
	}

	return nil
}

func validateDefinitionMetadata(definition CommandDefinition) error {
	if !commandIDPattern.MatchString(definition.ID) {
		return fmt.Errorf("%w: invalid command ID %q", ErrInvalidCatalog, definition.ID)
	}
	if definition.Category == "" || definition.Name == "" || definition.Description == "" {
		return fmt.Errorf("%w: command %q requires category, name, and description", ErrInvalidCatalog, definition.ID)
	}
	if len(definition.ExecutionModes) == 0 {
		return fmt.Errorf("%w: command %q requires an execution mode", ErrInvalidCatalog, definition.ID)
	}
	for _, mode := range definition.ExecutionModes {
		if !isSupportedExecutionMode(mode) {
			return fmt.Errorf("%w: command %q has unsupported execution mode %q", ErrInvalidCatalog, definition.ID, mode)
		}
	}

	return nil
}

func validateParameterDefinition(commandID string, parameter ParameterDefinition) error {
	if parameter.ID == "" || parameter.Name == "" || !isSupportedParameterKind(parameter.Kind) {
		return fmt.Errorf("%w: command %q has an incomplete parameter definition", ErrInvalidCatalog, commandID)
	}
	if err := validateParameterConstraints(commandID, parameter); err != nil {
		return err
	}
	if len(parameter.Default) > 0 {
		if err := validateArgument(parameter, parameter.Default); err != nil {
			return fmt.Errorf(
				"%w: command %q parameter %q has invalid default: %w",
				ErrInvalidCatalog,
				commandID,
				parameter.ID,
				err,
			)
		}
	}

	return nil
}

func validateParameterConstraints(commandID string, parameter ParameterDefinition) error {
	if parameter.Step != nil && (*parameter.Step <= 0 || parameter.Minimum == nil ||
		parameter.Kind != ParameterInteger) {
		return fmt.Errorf("%w: command %q parameter %q requires a positive integer step and minimum", ErrInvalidCatalog, commandID, parameter.ID)
	}
	if parameter.Minimum != nil && parameter.Maximum != nil && *parameter.Minimum > *parameter.Maximum {
		return fmt.Errorf(
			"%w: command %q parameter %q has minimum above maximum",
			ErrInvalidCatalog,
			commandID,
			parameter.ID,
		)
	}
	// Integer choices narrow the bounds and step; each one must satisfy them.
	for _, choice := range parameter.Choices {
		if parameter.Kind != ParameterInteger {
			break
		}
		if err := validateIntegerArgument(parameter, json.RawMessage(choice.Value)); err != nil {
			return fmt.Errorf("%w: command %q parameter %q has invalid integer choice: %w",
				ErrInvalidCatalog, commandID, parameter.ID, err)
		}
	}
	if parameter.Pattern != "" {
		if _, err := regexp.Compile(parameter.Pattern); err != nil {
			return fmt.Errorf(
				"%w: command %q parameter %q has invalid pattern: %w",
				ErrInvalidCatalog,
				commandID,
				parameter.ID,
				err,
			)
		}
	}

	return nil
}

func validateArgument(parameter ParameterDefinition, raw json.RawMessage) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return fmt.Errorf("%w: null is not allowed", ErrInvalidArgument)
	}

	switch parameter.Kind {
	case ParameterInteger:
		return validateIntegerArgument(parameter, raw)
	case ParameterBoolean:
		var value bool
		if err := json.Unmarshal(raw, &value); err != nil {
			return fmt.Errorf("%w: must be a boolean", ErrInvalidArgument)
		}
	case ParameterString, ParameterEnum, ParameterCharacter, ParameterLocation, ParameterMonster, ParameterEquipment:
		return validateStringArgument(parameter, raw)
	default:
		return fmt.Errorf("%w: unsupported parameter kind %q", ErrInvalidArgument, parameter.Kind)
	}

	return nil
}

func validateIntegerArgument(parameter ParameterDefinition, raw json.RawMessage) error {
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%w: must be an integer", ErrInvalidArgument)
	}
	if parameter.Minimum != nil && value < *parameter.Minimum {
		return fmt.Errorf("%w: must be at least %d", ErrInvalidArgument, *parameter.Minimum)
	}
	if parameter.Maximum != nil && value > *parameter.Maximum {
		return fmt.Errorf("%w: must be at most %d", ErrInvalidArgument, *parameter.Maximum)
	}
	// Unsigned subtraction preserves distances spanning the signed integer range.
	if parameter.Step != nil && (uint(value)-uint(*parameter.Minimum))%uint(*parameter.Step) != 0 {
		return fmt.Errorf("%w: must advance from %d in steps of %d", ErrInvalidArgument, *parameter.Minimum, *parameter.Step)
	}
	if len(parameter.Choices) > 0 && !containsChoice(parameter.Choices, strconv.Itoa(value)) {
		return fmt.Errorf("%w: unknown choice %d", ErrInvalidArgument, value)
	}

	return nil
}

func validateStringArgument(parameter ParameterDefinition, raw json.RawMessage) error {
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return fmt.Errorf("%w: must be a string", ErrInvalidArgument)
	}
	if parameter.Pattern != "" && !regexp.MustCompile(parameter.Pattern).MatchString(value) {
		return fmt.Errorf("%w: does not match the required pattern", ErrInvalidArgument)
	}
	if len(parameter.Choices) > 0 && !containsChoice(parameter.Choices, value) {
		return fmt.Errorf("%w: unknown choice %q", ErrInvalidArgument, value)
	}

	return nil
}

func isSupportedParameterKind(kind ParameterKind) bool {
	switch kind {
	case ParameterString, ParameterInteger, ParameterBoolean, ParameterEnum,
		ParameterCharacter, ParameterLocation, ParameterMonster, ParameterEquipment:
		return true
	default:
		return false
	}
}

func isSupportedExecutionMode(mode ExecutionMode) bool {
	switch mode {
	case ExecutionQuery, ExecutionLiveSafe, ExecutionSynchronized, ExecutionWorkflow,
		ExecutionAlteration, ExecutionSceneRestart, ExecutionPresentation:
		return true
	default:
		return false
	}
}

func containsChoice(choices []Choice, value string) bool {
	for _, choice := range choices {
		if choice.Value == value {
			return true
		}
	}

	return false
}

func cloneDefinition(definition CommandDefinition) CommandDefinition {
	definition.Aliases = append([]string(nil), definition.Aliases...)
	definition.SearchTerms = append([]string(nil), definition.SearchTerms...)
	definition.ExecutionModes = append([]ExecutionMode(nil), definition.ExecutionModes...)
	definition.Parameters = append([]ParameterDefinition(nil), definition.Parameters...)
	for i := range definition.Parameters {
		definition.Parameters[i].Default = append(json.RawMessage(nil), definition.Parameters[i].Default...)
		definition.Parameters[i].Choices = cloneChoices(definition.Parameters[i].Choices)
		definition.Parameters[i].Availability = append([]string(nil), definition.Parameters[i].Availability...)
		definition.Parameters[i].Minimum = cloneIntPointer(definition.Parameters[i].Minimum)
		definition.Parameters[i].Maximum = cloneIntPointer(definition.Parameters[i].Maximum)
		definition.Parameters[i].Step = cloneIntPointer(definition.Parameters[i].Step)
	}

	return definition
}

func cloneChoices(choices []Choice) []Choice {
	cloned := make([]Choice, len(choices))
	for index, choice := range choices {
		cloned[index] = Choice{
			Value:    choice.Value,
			Label:    choice.Label,
			Metadata: append(json.RawMessage(nil), choice.Metadata...),
		}
	}

	return cloned
}

func cloneRequest(request Request) Request {
	request.Arguments = cloneArguments(request.Arguments)

	return request
}

func cloneResult(result Result) Result {
	if result.Invocation != nil {
		result.Invocation = &CommandInvocation{
			CommandID: result.Invocation.CommandID,
			Arguments: cloneArguments(result.Invocation.Arguments),
		}
	}
	result.Data = append(json.RawMessage(nil), result.Data...)
	result.Changes = append([]StateChange(nil), result.Changes...)
	for index := range result.Changes {
		result.Changes[index].Before = append(json.RawMessage(nil), result.Changes[index].Before...)
		result.Changes[index].After = append(json.RawMessage(nil), result.Changes[index].After...)
	}

	return result
}

func cloneArguments(arguments map[string]json.RawMessage) map[string]json.RawMessage {
	if arguments == nil {
		return nil
	}

	cloned := make(map[string]json.RawMessage, len(arguments))
	for key, value := range arguments {
		cloned[key] = append(json.RawMessage(nil), value...)
	}

	return cloned
}

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}

	cloned := *value

	return &cloned
}
