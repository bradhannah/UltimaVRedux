package save_snapshot

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/stretchr/testify/require"
)

func snapshotWithConversationForArchiveBoundary() SaveSnapshot {
	snapshot := validSnapshot()
	snapshot.LastConversation = journal.Conversation{Exchanges: []journal.Exchange{{
		ID: "exchange-1", Speaker: "Iolo", Location: "Britain", Input: "WORD",
		Passages: []journal.Passage{{Text: "A retained passage", Runic: false}},
	}}, Truncated: false}

	return snapshot
}

func TestNativeConversationArchiveMemberCodecSchemaAgreement(t *testing.T) {
	for _, version := range []int{MinimumVersion, CurrentVersion} {
		t.Run(fmt.Sprintf("v%d", version), func(t *testing.T) {
			snapshot := snapshotWithConversationForArchiveBoundary()
			document := currentEnvelopeDocument(t, snapshot)
			body := document["snapshot"].(map[string]any)
			body["version"] = version
			if version < CurrentVersion {
				delete(body, "excerpts")
				delete(body, "notes")
			}
			schema, _ := compileNativeSaveSchemaFile(t, fmt.Sprintf("native-save-e2-s%d.schema.json", version))
			data, err := json.Marshal(document)
			require.NoError(t, err)
			require.NoError(t, schema.Validate(decodeJSONSchemaInstance(t, data)), "baseline must satisfy the exact source schema")
			decoded, err := Decode(data)
			require.NoError(t, err)
			require.Equal(t, snapshot, decoded.Snapshot, "valid v24 data promotes and retains the same conversation")
			exchange := body["lastConversation"].(map[string]any)["exchanges"].([]any)[0].(map[string]any)
			for _, test := range []struct {
				name   string
				member string
				value  any
			}{
				{name: "true", member: "archived", value: true},
				{name: "false", member: "archived", value: false},
				{name: "null", member: "archived", value: nil},
				{name: "string", member: "archived", value: "false"},
				{name: "case alias", member: "Archived", value: false},
			} {
				t.Run(test.name, func(t *testing.T) {
					exchange[test.member] = test.value
					defer delete(exchange, test.member)
					data, err := json.Marshal(document)
					require.NoError(t, err)
					require.Error(t, schema.Validate(decodeJSONSchemaInstance(t, data)))
					_, err = Decode(data)
					require.ErrorIs(t, err, ErrInvalidEnvelope)
					require.Contains(t, err.Error(), "notebook-only")
					// The direct DTO decoder is also a native boundary, not just Decode.
					snapshotJSON, err := json.Marshal(body)
					require.NoError(t, err)
					var direct SaveSnapshot
					require.ErrorIs(t, json.Unmarshal(snapshotJSON, &direct), ErrInvalidEnvelope)
				})
			}
		})
	}
}

func TestNativeConversationEncodeRejectsNotebookArchiveState(t *testing.T) {
	snapshot := snapshotWithConversationForArchiveBoundary()
	snapshot.LastConversation.Exchanges[0].Archived = true
	data, err := EncodeCanonical(validEnvelope(snapshot))
	require.ErrorIs(t, err, ErrInvalidSnapshot)
	require.Empty(t, data)
	var validation *ValidationError
	require.ErrorAs(t, err, &validation)
	require.Contains(t, validation.Issues, Issue{
		Path: "lastConversation", Code: "invalid", Message: "invalid conversation exchange: conversation exchange cannot be archived",
	})

	snapshot.LastConversation.Exchanges[0].Archived = false
	data, err = EncodeCanonical(validEnvelope(snapshot))
	require.NoError(t, err)
	require.NotContains(t, string(data), `"archived"`, "native exchanges must omit even inactive archive metadata")
	schema, _ := compileCurrentNativeSaveSchema(t)
	require.NoError(t, schema.Validate(decodeJSONSchemaInstance(t, data)))
	decoded, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, snapshot, decoded.Snapshot)
}
