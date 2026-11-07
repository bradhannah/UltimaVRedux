package save_snapshot

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/stretchr/testify/require"
)

func excerptSnapshot() SaveSnapshot {
	snapshot := validSnapshot()
	snapshot.Excerpts = journal.Excerpts{{ID: "kept-1", Speaker: "Iolo", Location: "Britain", Input: "WORD", Archived: true,
		Passages: []journal.Passage{{Text: "Whole speech.\n\nKeep spacing.  "}, {Text: "SUMM", Runic: true}}}}
	return snapshot
}

func TestV25ExcerptsCodecSchemaAndFrozenV24(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	snapshot := excerptSnapshot()
	requireV23RoundTrip(t, schema, snapshot)
	for _, excerpts := range []journal.Excerpts{nil, {}} {
		snapshot.Excerpts = excerpts
		requireV23RoundTrip(t, schema, snapshot)
	}
	data, err := EncodeCanonical(validEnvelope(snapshot))
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(data, &document))
	body := document["snapshot"].(map[string]any)
	delete(body, "excerpts")
	missing, err := json.Marshal(document)
	require.NoError(t, err)
	_, err = Decode(missing)
	require.Error(t, err, "current saves must explicitly own even an empty collection")
	require.Error(t, schema.Validate(document))
	delete(body, "notes")
	body["version"] = 24
	old, err := json.Marshal(document)
	require.NoError(t, err)
	frozen, _ := compileNativeSaveSchemaFile(t, "native-save-e2-s24.schema.json")
	require.NoError(t, frozen.Validate(document))
	decoded, err := Decode(old)
	require.NoError(t, err)
	require.True(t, decoded.NeedsExcerptMigration())
	require.Empty(t, decoded.Snapshot.Excerpts)
	require.Equal(t, snapshot.AtlasNotes, decoded.Snapshot.AtlasNotes)
	body["excerpts"] = nil
	old, err = json.Marshal(document)
	require.NoError(t, err)
	_, err = Decode(old)
	require.Error(t, err, "frozen v24 must not admit later fields")
	require.Error(t, frozen.Validate(document))
}

func TestV25ExcerptsRejectMalformedCollections(t *testing.T) {
	schema, _ := compileCurrentNativeSaveSchema(t)
	for name, mutate := range map[string]func(*SaveSnapshot){
		"duplicate":    func(s *SaveSnapshot) { s.Excerpts = append(s.Excerpts, s.Excerpts[0]) },
		"invalid text": func(s *SaveSnapshot) { s.Excerpts[0].Passages[0].Text = "\x00" },
		"empty speech": func(s *SaveSnapshot) { s.Excerpts[0].Passages = nil },
		"oversized exchange": func(s *SaveSnapshot) {
			s.Excerpts[0].Passages[0].Text = strings.Repeat("a", journal.MaxConversationBytes)
		},
		"too many": func(s *SaveSnapshot) { s.Excerpts = make(journal.Excerpts, journal.MaxExcerpts+1) },
	} {
		t.Run(name, func(t *testing.T) {
			snapshot := excerptSnapshot()
			mutate(&snapshot)
			_, err := EncodeCanonical(validEnvelope(snapshot))
			require.Error(t, err)
			data, err := json.Marshal(validEnvelope(snapshot))
			require.NoError(t, err)
			_, err = Decode(data)
			require.Error(t, err)
			require.Error(t, schema.Validate(decodeJSONSchemaInstance(t, data)))
		})
	}
}

func TestV25MaximumExcerptPayloadPersistsInRepository(t *testing.T) {
	snapshot := validSnapshot()
	// Exactly 4 MiB of source across 512 exchanges, including metadata; JSON
	// escaping expands the transport beyond that bound and must still round-trip.
	for i := range journal.MaxExcerpts {
		id := fmt.Sprintf("%03d", i)
		snapshot.Excerpts = append(snapshot.Excerpts, journal.Exchange{ID: id, Speaker: "Iolo", Location: "Britain", Input: "WORD",
			Archived: i%2 == 0, Passages: []journal.Passage{{Text: strings.Repeat("<", journal.MaxNotebookBytes/journal.MaxExcerpts-len(id)-4-7-4)}}})
	}
	require.NoError(t, snapshot.Excerpts.Validate())
	envelope := validEnvelope(snapshot)
	data, err := EncodeCanonical(envelope)
	require.NoError(t, err)
	require.Greater(t, len(data), journal.MaxNotebookBytes)
	decoded, err := Decode(data)
	require.NoError(t, err)
	require.Equal(t, snapshot, decoded.Snapshot)
	repository, _ := newTestRepository(t)
	_, err = repository.Create("personal:full-notebook", envelope)
	require.NoError(t, err)
	reloaded, err := repository.Load("personal:full-notebook")
	require.NoError(t, err)
	require.Equal(t, snapshot, reloaded.Envelope.Snapshot)
	snapshot.Excerpts[0].Passages[0].Text += "x"
	require.ErrorIs(t, snapshot.Excerpts.Validate(), journal.ErrNotebookFull)
}
