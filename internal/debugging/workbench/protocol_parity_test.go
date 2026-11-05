package workbench

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

type wireFieldContract struct {
	Required map[string]bool `json:"required"`
	Optional map[string]bool `json:"optional"`
}

type protocolParityContract struct {
	ProtocolVersion int                          `json:"protocolVersion"`
	ParameterKinds  map[string]bool              `json:"parameterKinds"`
	ExecutionModes  map[string]bool              `json:"executionModes"`
	MessageTypes    map[string]bool              `json:"messageTypes"`
	Fields          map[string]wireFieldContract `json:"fields"`
}

func TestGoProtocolMatchesTypeScriptContract(t *testing.T) {
	raw, err := os.ReadFile("../webui/client/src/protocol_contract.json")
	require.NoError(t, err)
	var actual protocolParityContract
	require.NoError(t, json.Unmarshal(raw, &actual))

	expected := protocolParityContract{
		ProtocolVersion: ProtocolVersion,
		ParameterKinds: stringSet(
			debugging.ParameterString, debugging.ParameterInteger,
			debugging.ParameterBoolean, debugging.ParameterEnum, debugging.ParameterCharacter,
			debugging.ParameterLocation, debugging.ParameterMonster, debugging.ParameterEquipment,
		),
		ExecutionModes: stringSet(
			debugging.ExecutionQuery, debugging.ExecutionLiveSafe, debugging.ExecutionSynchronized,
			debugging.ExecutionWorkflow, debugging.ExecutionAlteration, debugging.ExecutionSceneRestart,
			debugging.ExecutionPresentation,
		),
		MessageTypes: stringSet(
			messageSessionHello, messageSessionReady, messageCatalogRequest, messageCatalogSnapshot,
			messageCommandExecute, messageCommandAccepted, messageCommandCompleted, messageCommandFailed,
			messageCommandAck, messageOperationStatus, messageProtocolError,
		),
		Fields: map[string]wireFieldContract{
			"Choice":              reflectedWireFields(debugging.Choice{}),
			"OptionSet":           reflectedWireFields(debugging.OptionSet{}),
			"ParameterDefinition": reflectedWireFields(debugging.ParameterDefinition{}),
			"CommandDefinition":   reflectedWireFields(debugging.CommandDefinition{}),
			"CommandInvocation":   reflectedWireFields(debugging.CommandInvocation{}),
			"StateChange":         reflectedWireFields(debugging.StateChange{}),
			"CommandResult":       reflectedWireFields(debugging.Result{}),
			"Envelope":            reflectedWireFields(envelope{}),
			"SessionHello":        reflectedWireFields(sessionHelloPayload{}),
			"SessionReady":        reflectedWireFields(sessionReadyPayload{}),
			"CatalogSnapshot":     reflectedWireFields(catalogSnapshotPayload{}),
			"CommandExecute":      reflectedWireFields(commandExecutePayload{}),
			"CommandAccepted":     reflectedWireFields(commandAcceptedPayload{}),
			"CommandAck":          reflectedWireFields(commandAckPayload{}),
			"OperationStatus":     reflectedWireFields(operationStatusPayload{}),
			"ProtocolError":       reflectedWireFields(protocolErrorPayload{}),
		},
	}

	assert.Equal(t, expected, actual)
}

func stringSet[T ~string](values ...T) map[string]bool {
	result := make(map[string]bool, len(values))
	for _, value := range values {
		result[string(value)] = true
	}

	return result
}

func reflectedWireFields(value any) wireFieldContract {
	result := wireFieldContract{Required: map[string]bool{}, Optional: map[string]bool{}}
	typeOf := reflect.TypeOf(value)
	for index := range typeOf.NumField() {
		field := typeOf.Field(index)
		name, options := parseJSONTag(field.Tag.Get("json"))
		if name == "" || name == "-" {
			continue
		}
		if options["omitempty"] {
			result.Optional[name] = true
		} else {
			result.Required[name] = true
		}
	}

	return result
}

func parseJSONTag(tag string) (string, map[string]bool) {
	parts := make([]string, 0, 2)
	start := 0
	for index := 0; index <= len(tag); index++ {
		if index == len(tag) || tag[index] == ',' {
			parts = append(parts, tag[start:index])
			start = index + 1
		}
	}
	options := make(map[string]bool, len(parts)-1)
	for _, option := range parts[1:] {
		options[option] = true
	}

	return parts[0], options
}
