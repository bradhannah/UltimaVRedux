package save_snapshot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

// deleteExperienceRecords removes the members version 27 added, so a document
// can stand in for an older save.
func deleteExperienceRecords(body map[string]any) {
	delete(body, "deedExperiencePaid")
	delete(body, "sharedExperienceRemainder")
}

func experienceSnapshot() SaveSnapshot {
	snapshot := validSnapshot()
	snapshot.DeedExperiencePaid = []string{"codex.honesty", "item.shard.hatred", "shrine.honesty"}
	snapshot.SharedExperienceRemainder = 5

	return snapshot
}

func TestV27ExperienceRecordsPromoteRoundTripAndReject(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	requireV23RoundTrip(t, schema, validSnapshot())
	requireV23RoundTrip(t, schema, experienceSnapshot())

	data, err := EncodeCanonical(validEnvelope(experienceSnapshot()))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	body := document["snapshot"].(map[string]any)
	for _, member := range []string{"deedExperiencePaid", "sharedExperienceRemainder"} {
		kept := body[member]
		delete(body, member)
		missing, marshalErr := json.Marshal(document)
		require.NoError(t, marshalErr)
		_, decodeErr := Decode(missing)
		require.Error(t, decodeErr, "current saves must carry %s", member)
		require.Error(t, schema.Validate(document))
		body[member] = kept
	}

	body["version"] = 26
	admitted, err := json.Marshal(document)
	require.NoError(t, err)
	_, err = Decode(admitted)
	require.Error(t, err, "frozen v26 must not admit experience records")
	deleteExperienceRecords(body)
	old, err := json.Marshal(document)
	require.NoError(t, err)
	decoded, err := Decode(old)
	require.NoError(t, err)
	require.Equal(t, CurrentVersion, decoded.Snapshot.Version)
	require.Empty(t, decoded.Snapshot.DeedExperiencePaid, "an older save has paid no deeds")
	require.Zero(t, decoded.Snapshot.SharedExperienceRemainder)

	for name, mutate := range map[string]func(*SaveSnapshot){
		"unsorted":      func(s *SaveSnapshot) { s.DeedExperiencePaid = []string{"shrine.honesty", "codex.honesty"} },
		"repeated":      func(s *SaveSnapshot) { s.DeedExperiencePaid = []string{"codex.honesty", "codex.honesty"} },
		"malformed key": func(s *SaveSnapshot) { s.DeedExperiencePaid = []string{"Codex Honesty"} },
		"remainder":     func(s *SaveSnapshot) { s.SharedExperienceRemainder = 6 },
	} {
		t.Run(name, func(t *testing.T) {
			bad := experienceSnapshot()
			mutate(&bad)
			_, encodeErr := EncodeCanonical(validEnvelope(bad))
			require.Error(t, encodeErr)
			raw, marshalErr := json.Marshal(validEnvelope(bad))
			require.NoError(t, marshalErr)
			_, decodeErr := Decode(raw)
			require.Error(t, decodeErr)
			require.Error(t, schema.Validate(decodeJSONSchemaInstance(t, raw)))
		})
	}
}
