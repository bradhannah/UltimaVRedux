package save_snapshot

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
)

func notesSnapshot() SaveSnapshot {
	snapshot := validSnapshot()
	notes := journal.DefaultNotes()
	notes.LastSectionID, notes.LastNoteID = 3, 2
	notes.Sections = append(notes.Sections, journal.NoteSection{ID: 3, Name: "Clues", Folded: true})
	notes.Notes = []journal.NoteRecord{
		{ID: 1, SectionID: journal.NoteSectionToDo, Title: "Find the key", Text: "Ask Iolo.\n\nTwice.", Place: "Britain",
			RecordedAt: journal.GameTime{Year: 139, Month: 3, Day: 4, Hour: 12, Minute: 30}, Done: true},
		{ID: 2, SectionID: 3, Text: "Untitled", RecordedAt: journal.GameTime{Year: 139, Month: 3, Day: 5}},
	}
	snapshot.Notes = notes

	return snapshot
}

func TestV26NotesPromoteRoundTripAndReject(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	require.Zero(t, validSnapshot().Notes)
	requireV23RoundTrip(t, schema, validSnapshot())
	snapshot := notesSnapshot()
	requireV23RoundTrip(t, schema, snapshot)

	data, err := EncodeCanonical(validEnvelope(validSnapshot()))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	body := document["snapshot"].(map[string]any)
	delete(body, "notes")
	missing, err := json.Marshal(document)
	require.NoError(t, err)
	_, err = Decode(missing)
	require.Error(t, err, "current saves must carry a notes member")
	require.Error(t, schema.Validate(document))
	for _, version := range []int{24, 25} {
		body["version"] = version
		if version == 24 {
			delete(body, "excerpts")
		} else {
			body["excerpts"] = []any{}
		}
		old, marshalErr := json.Marshal(document)
		require.NoError(t, marshalErr)
		decoded, decodeErr := Decode(old)
		require.NoError(t, decodeErr, "version %d", version)
		require.Equal(t, CurrentVersion, decoded.Snapshot.Version)
		require.Zero(t, decoded.Snapshot.Notes)
		body["notes"] = map[string]any{}
		old, marshalErr = json.Marshal(document)
		require.NoError(t, marshalErr)
		_, decodeErr = Decode(old)
		require.Error(t, decodeErr, "frozen v%d must not admit notes", version)
		delete(body, "notes")
	}

	for name, mutate := range map[string]func(*SaveSnapshot){
		"missing section": func(s *SaveSnapshot) { s.Notes.Notes[0].SectionID = 9 },
		"renamed places":  func(s *SaveSnapshot) { s.Notes.Sections[1].Name = "Sites" },
	} {
		t.Run(name, func(t *testing.T) {
			bad := notesSnapshot()
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
