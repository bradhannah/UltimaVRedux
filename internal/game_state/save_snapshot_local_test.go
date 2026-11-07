package game_state

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/stretchr/testify/require"
)

// Sources are opt-in, read-only and never embedded in repository fixtures.
func TestLocalNativeSaveHydration(t *testing.T) {
	paths := filepath.SplitList(os.Getenv("U5_NATIVE_SAVE_TEST_PATHS"))
	if len(paths) == 0 {
		// Private saves are an optional extra proof, not a provisioned-data gate.
		t.Log("no local saves requested; set U5_NATIVE_SAVE_TEST_PATHS for the opt-in proof")
		return
	}
	refs := getBaseGameReferences()
	require.NotNil(t, refs, "opt-in hydration requires installed original game data")
	require.NotNil(t, cachedIntegrationCfg)
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			before := sha256.Sum256(data)
			copyRoot := t.TempDir()
			copyPath := filepath.Join(copyRoot, "slot-01.json")
			require.NoError(t, os.WriteFile(copyPath, data, 0o600))
			t.Cleanup(func() {
				after, readErr := os.ReadFile(path)
				require.NoError(t, readErr)
				require.Equal(t, before, sha256.Sum256(after), "source bytes must remain untouched")
			})
			envelope, err := save_snapshot.Decode(data)
			require.NoError(t, err)
			repository, err := player_save.NewRepository(copyRoot, func(snapshot save_snapshot.SaveSnapshot) error {
				return ValidateSaveSnapshotCompatibility(snapshot, refs)
			})
			require.NoError(t, err)
			require.Equal(t, player_save.SlotReady, repository.List()[0].Status, "actual slot must not be marked corrupt or incompatible")
			record, err := repository.Load(player_save.Slot01)
			require.NoError(t, err)
			require.Equal(t, envelope, record.Envelope)
			var originalMetadata struct {
				Metadata save_snapshot.Metadata `json:"metadata"`
			}
			require.NoError(t, json.Unmarshal(data, &originalMetadata))
			require.Equal(t, originalMetadata.Metadata, envelope.Metadata, "original metadata and IDs survive promotion")
			state, err := NewGameStateFromSaveSnapshot(envelope.Snapshot, cachedIntegrationCfg, refs, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
			require.NoError(t, err)
			state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
			state.SetupEnvironmentalSystemCallbacks()
			recaptured, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			// These two records are scene-owned and carried alongside GameState.
			recaptured.Presentation = envelope.Snapshot.Presentation
			recaptured.LastConversation = envelope.Snapshot.LastConversation
			require.Equal(t, envelope.Snapshot, recaptured, "full authoritative hydration/recapture parity")
			assertHistoricalPersistedBase(t, data, recaptured)
			envelope.Snapshot = recaptured
			encoded, err := save_snapshot.EncodeCanonical(envelope)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(copyPath, encoded, 0o600))
			encoded, err = os.ReadFile(copyPath)
			require.NoError(t, err)
			reloaded, err := save_snapshot.Decode(encoded)
			require.NoError(t, err)
			require.Equal(t, envelope, reloaded, "metadata, presentation and gameplay all survive current encoding")
			require.Equal(t, player_save.SlotReady, repository.List()[0].Status, "recaptured current slot remains loadable")
		})
	}
}

// Compare against the original wire document, not merely against the migrated
// snapshot: a promotion that silently drops an old field must fail this test.
func assertHistoricalPersistedBase(t *testing.T, original []byte, recaptured save_snapshot.SaveSnapshot) {
	t.Helper()
	var document map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(original, &document))
	var old map[string]any
	decoder := json.NewDecoder(bytes.NewReader(document["snapshot"]))
	decoder.UseNumber() // Preserve all 64 bits of RNG counters and visit revisions.
	require.NoError(t, decoder.Decode(&old))
	version := persistedInteger(t, old["version"])
	if version < save_snapshot.MinimumVersion || version > save_snapshot.CurrentVersion {
		t.Fatalf("local base parity fixture requires v%d-current, got %d", save_snapshot.MinimumVersion, version)
	}
	encoded, err := json.Marshal(recaptured)
	require.NoError(t, err)
	var current map[string]any
	decoder = json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	require.NoError(t, decoder.Decode(&current))
	current["version"] = old["version"]
	if version < 25 {
		delete(current, "excerpts") // Notebook excerpts are imported outside the snapshot.
	}
	if version < 26 {
		delete(current, "notes") // Older saves start with a fresh notebook.
	}
	require.Equal(t, old, current, "every persisted field must survive promotion and hydration")
	t.Logf("v%d: full persisted base, hydration/recapture and metadata preserved", version)
}

func persistedInteger(t *testing.T, value any) int64 {
	t.Helper()
	number, ok := value.(json.Number)
	require.True(t, ok)
	integer, err := number.Int64()
	require.NoError(t, err)
	return integer
}
