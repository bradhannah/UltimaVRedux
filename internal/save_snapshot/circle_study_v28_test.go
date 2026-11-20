package save_snapshot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func circleStudySnapshot() SaveSnapshot {
	snapshot := validSnapshot()
	snapshot.CircleStudy = []int{0}

	return snapshot
}

func TestV28CircleStudyPromotesRoundTripsAndRejects(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	requireV23RoundTrip(t, schema, validSnapshot())
	requireV23RoundTrip(t, schema, circleStudySnapshot())

	data, err := EncodeCanonical(validEnvelope(circleStudySnapshot()))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	body := document["snapshot"].(map[string]any)
	kept := body["circleStudy"]
	delete(body, "circleStudy")
	missing, err := json.Marshal(document)
	require.NoError(t, err)
	_, err = Decode(missing)
	require.Error(t, err, "current saves must carry circleStudy")
	require.Error(t, schema.Validate(document))
	body["circleStudy"] = kept

	body["version"] = 27
	admitted, err := json.Marshal(document)
	require.NoError(t, err)
	_, err = Decode(admitted)
	require.Error(t, err, "frozen v27 must not admit a circle study record")
	delete(body, "circleStudy")
	old, err := json.Marshal(document)
	require.NoError(t, err)
	decoded, err := Decode(old)
	require.NoError(t, err)
	require.Equal(t, CurrentVersion, decoded.Snapshot.Version)
	require.Empty(t, decoded.Snapshot.CircleStudy, "an older save has no member studied")

	for name, slots := range map[string][]int{
		"unsorted":    {1, 0},
		"repeated":    {0, 0},
		"negative":    {-1},
		"past roster": {RosterSlotCount},
		"empty slot":  {RosterSlotCount - 1},
	} {
		t.Run(name, func(t *testing.T) {
			bad := validSnapshot()
			bad.CircleStudy = slots
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
