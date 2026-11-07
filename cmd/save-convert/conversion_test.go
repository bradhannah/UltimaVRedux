package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	testdata "github.com/bradhannah/UltimaVRedux/test/originaldata"
	"github.com/stretchr/testify/require"
)

const testPlaythroughID = "0123456789abcdef0123456789abcdef"

func TestOriginalReferenceConversionAndPublication(t *testing.T) {
	dataDirectory := testdata.Directory(t)
	source := copyOriginalFixture(t)
	originalGAM := readFile(t, filepath.Join(source, "SAVED.GAM"))
	originalOOL := readFile(t, filepath.Join(source, "SAVED.OOL"))
	originalData := readFile(t, filepath.Join(dataDirectory, "DATA.OVL"))
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	configDirectory := filepath.Join(home, ".ultima_v_redux")
	require.NoError(t, os.Mkdir(configDirectory, 0o700))
	configPath := filepath.Join(configDirectory, "config.yaml")
	configBytes := []byte("invalid: [ configuration must not be read or written\n")
	require.NoError(t, os.WriteFile(configPath, configBytes, 0o600))
	parsed := options{dataDirectory: dataDirectory, source: source, description: "Original adventure"}
	fixedTime := time.Date(2026, time.October, 11, 12, 0, 0, 0, time.FixedZone("test", 3600))
	envelope, warnings, err := convert(context.Background(), filepath.Join(source, "SAVED.GAM"), parsed,
		func() time.Time { return fixedTime }, func() (string, error) { return testPlaythroughID, nil })
	require.NoError(t, err)
	require.NotEmpty(t, warnings)
	require.Equal(t, fixedTime.UTC(), envelope.Metadata.CreatedAt)
	require.Equal(t, importedDescription, envelope.Metadata.Description)
	require.Equal(t, parsed.description, envelope.Metadata.UserDescription)
	require.Equal(t, testPlaythroughID, envelope.Metadata.PlaythroughID)
	require.NotEmpty(t, envelope.Metadata.GameVersion)
	require.Equal(t, save_snapshot.CurrentEnvelopeVersion, envelope.EnvelopeVersion)
	require.Equal(t, save_snapshot.CurrentVersion, envelope.Snapshot.Version)
	require.NotNil(t, envelope.Snapshot.MapUnits)
	require.False(t, envelope.Snapshot.NeedsMapUnitReconstruction())
	bundle, err := legacy_save.ReadBundleContext(t.Context(), filepath.Join(source, "SAVED.GAM"))
	require.NoError(t, err)
	raw, err := legacy_save.Convert(bundle)
	require.NoError(t, err)
	require.Equal(t, raw.Snapshot.Random, envelope.Snapshot.Random)
	require.Equal(t, raw.Snapshot.World, envelope.Snapshot.World)
	require.Equal(t, raw.Snapshot.Presentation, envelope.Snapshot.Presentation)
	require.Equal(t, raw.Snapshot.LastConversation, envelope.Snapshot.LastConversation)
	configuration, err := config.NewUltimaVConfigurationWithPath(dataDirectory)
	require.NoError(t, err)
	gameReferences, err := references.NewGameReferences(configuration)
	require.NoError(t, err)
	validate := func(snapshot save_snapshot.SaveSnapshot) error {
		return game_state.ValidateSaveSnapshotCompatibility(snapshot, gameReferences)
	}
	t.Run("standalone CLI canonical roundtrip", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "original.json")
		var output, diagnostics bytes.Buffer
		require.NoError(t, run(context.Background(), []string{
			"--data-dir", dataDirectory, "--source", source, "--output", path, "--description", "Original adventure",
		}, &output, &diagnostics))
		require.Contains(t, output.String(), "Converted save:")
		require.Contains(t, diagnostics.String(), "warning:")
		encoded := readFile(t, path)
		decoded, err := save_snapshot.Decode(encoded)
		require.NoError(t, err)
		require.Equal(t, envelope.Snapshot, decoded.Snapshot)
		require.True(t, save_snapshot.ValidPlaythroughID(decoded.Metadata.PlaythroughID))
		canonicalPath, err := filepath.EvalSymlinks(path)
		require.NoError(t, err)
		require.Contains(t, output.String(), fmt.Sprintf("Converted save: %q", canonicalPath))
		require.Contains(t, output.String(), "Playthrough ID: "+decoded.Metadata.PlaythroughID)
		require.Contains(t, output.String(), fmt.Sprintf("Format: %s (envelope %d, snapshot %d)",
			decoded.Format, decoded.EnvelopeVersion, decoded.Snapshot.Version))
		require.Contains(t, output.String(), fmt.Sprintf("Warnings: %d", len(warnings)))
		require.NotEqual(t, testPlaythroughID, decoded.Metadata.PlaythroughID)
		require.NoError(t, validate(decoded.Snapshot))
		canonical, err := save_snapshot.EncodeCanonical(decoded)
		require.NoError(t, err)
		require.Equal(t, encoded, canonical)
		state, err := game_state.NewGameStateFromSaveSnapshot(decoded.Snapshot, configuration, gameReferences, viewportWidth, viewportHeight)
		require.NoError(t, err)
		recaptured, err := state.CaptureSaveSnapshot()
		require.NoError(t, err)
		recaptured.Presentation = decoded.Snapshot.Presentation
		recaptured.LastConversation = decoded.Snapshot.LastConversation
		require.Equal(t, decoded.Snapshot, recaptured)
	})
	t.Run("receipt failure follows publication", func(t *testing.T) {
		root := t.TempDir()
		err := run(context.Background(), []string{
			"--data-dir", dataDirectory, "--source", source, "--save-dir", root, "--slot", "slot-08",
		}, failingReceiptWriter{}, io.Discard)
		require.ErrorIs(t, err, io.ErrClosedPipe)
		require.ErrorContains(t, err, "output was published")
		repository, err := player_save.NewRepository(root, validate)
		require.NoError(t, err)
		record, err := repository.Load(player_save.Slot08)
		require.NoError(t, err)
		require.Equal(t, "Slot 8", record.Envelope.Metadata.Name)
		require.Equal(t, envelope.Snapshot, record.Envelope.Snapshot)
	})
	t.Run("slots", func(t *testing.T) { testSlotPublication(t, envelope, validate) })
	require.Equal(t, originalGAM, readFile(t, filepath.Join(source, "SAVED.GAM")))
	require.Equal(t, originalOOL, readFile(t, filepath.Join(source, "SAVED.OOL")))
	require.Equal(t, originalData, readFile(t, filepath.Join(dataDirectory, "DATA.OVL")))
	require.Equal(t, configBytes, readFile(t, configPath))
	entries, err := os.ReadDir(configDirectory)
	require.NoError(t, err)
	require.Len(t, entries, 1)
}

func testSlotPublication(t *testing.T, envelope save_snapshot.Envelope, validate player_save.CompatibilityValidator) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "saves")
	path := filepath.Join(root, "slot-01.json")
	target, err := inspectDestination(path, player_save.Slot01, false)
	require.NoError(t, err)
	_, err = os.Stat(root)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.NoError(t, target.publish(context.Background(), envelope))
	repository, err := player_save.NewRepository(root, validate)
	require.NoError(t, err)
	loaded, err := repository.Load(player_save.Slot01)
	require.NoError(t, err)
	require.Equal(t, envelope, loaded.Envelope)
	_, err = inspectDestination(path, player_save.Slot01, false)
	require.ErrorIs(t, err, errOccupied)
	first, err := inspectDestination(path, player_save.Slot01, true)
	require.NoError(t, err)
	second, err := inspectDestination(path, player_save.Slot01, true)
	require.NoError(t, err)
	envelope.Metadata.UserDescription = "replacement"
	require.NoError(t, first.publish(context.Background(), envelope))
	envelope.Metadata.UserDescription = "stale replacement"
	require.ErrorIs(t, second.publish(context.Background(), envelope), player_save.ErrStaleRevision)
	loaded, err = repository.Load(player_save.Slot01)
	require.NoError(t, err)
	require.Equal(t, "replacement", loaded.Envelope.Metadata.UserDescription)
	// Corrupt and future-version records are reclaimable only after explicit inspection.
	future := envelope
	future.Snapshot.Version = save_snapshot.CurrentVersion + 1
	futureBytes, err := json.Marshal(future)
	require.NoError(t, err)
	for _, old := range [][]byte{[]byte("broken JSON"), futureBytes} {
		require.NoError(t, os.WriteFile(path, old, 0o600))
		_, err = inspectDestination(path, player_save.Slot01, false)
		require.ErrorIs(t, err, errOccupied)
		reviewed, err := inspectDestination(path, player_save.Slot01, true)
		require.NoError(t, err)
		require.NoError(t, reviewed.publish(context.Background(), envelope))
		_, err = repository.Load(player_save.Slot01)
		require.NoError(t, err)
	}
	// --overwrite on an absent slot still means create-only, never blind replacement.
	otherPath := filepath.Join(root, "slot-02.json")
	absent, err := inspectDestination(otherPath, player_save.Slot02, true)
	require.NoError(t, err)
	require.NoError(t, absent.publish(context.Background(), envelope))
	envelope.Metadata.UserDescription = "must not replace new arrival"
	require.ErrorIs(t, absent.publish(context.Background(), envelope), player_save.ErrStaleRevision)
}

func TestInvalidInstallationDoesNotPublish(t *testing.T) {
	source := copyOriginalFixture(t)
	for _, slotMode := range []bool{false, true} {
		root := t.TempDir()
		target := filepath.Join(root, "import.json")
		args := []string{"--source", source, "--data-dir", t.TempDir()}
		if slotMode {
			target = filepath.Join(root, "saves")
			args[1] = filepath.Join(source, "SAVED.GAM")
			args = append(args, "--save-dir", target, "--slot", "slot-01")
		} else {
			args = append(args, "--output", target)
		}
		err := run(context.Background(), args, io.Discard, io.Discard)
		require.ErrorContains(t, err, "validate --data-dir")
		_, err = os.Stat(target)
		require.ErrorIs(t, err, os.ErrNotExist)
		entries, err := os.ReadDir(root)
		require.NoError(t, err)
		require.Empty(t, entries)
	}
}

func TestIdentityIsIndependentAndWellFormed(t *testing.T) {
	first, err := newPlaythroughID()
	require.NoError(t, err)
	second, err := newPlaythroughID()
	require.NoError(t, err)
	require.True(t, save_snapshot.ValidPlaythroughID(first))
	require.True(t, save_snapshot.ValidPlaythroughID(second))
	require.NotEqual(t, first, second)
}

func copyOriginalFixture(t *testing.T) string {
	t.Helper()
	directory := t.TempDir()
	for _, name := range []string{"SAVED.GAM", "SAVED.OOL"} {
		fixture := filepath.Join("..", "..", "internal", "legacy_save", "testdata", "b_carpet", name)
		require.NoError(t, os.WriteFile(filepath.Join(directory, name), readFile(t, fixture), 0o600))
	}
	return directory
}
