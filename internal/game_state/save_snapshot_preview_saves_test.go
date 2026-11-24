package game_state

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

// Each file was written by a released preview's own save code. Previews that
// wrote byte-identical saves share one file, so the five files cover all twelve.
func TestEveryPreviewSaveStillLoads(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).
		WithSystemCallbacks().Build()
	writtenAs := map[string]int{
		"preview-01-02.json": 24, "preview-03-07.json": 25, "preview-08.json": 25,
		"preview-09.json": 26, "preview-10-12.json": 28,
	}
	files, err := filepath.Glob(filepath.Join("testdata", "preview_saves", "*.json"))
	require.NoError(t, err)
	require.Len(t, files, len(writtenAs))
	for _, file := range files {
		t.Run(filepath.Base(file), func(t *testing.T) {
			data, err := os.ReadFile(file)
			require.NoError(t, err)
			require.Contains(t, string(data), "\"version\": "+strconv.Itoa(writtenAs[filepath.Base(file)])+",")

			envelope, err := save_snapshot.Decode(data)
			require.NoError(t, err)
			require.Equal(t, save_snapshot.CurrentVersion, envelope.Snapshot.Version)
			require.NoError(t, ValidateSaveSnapshotCompatibility(envelope.Snapshot, state.GameReferences))
			restored, err := NewGameStateFromSaveSnapshot(envelope.Snapshot, state.GameConfig, state.GameReferences,
				state.MapState.XTilesVisibleOnGameScreen, state.MapState.YTilesVisibleOnGameScreen)
			require.NoError(t, err)
			again, err := restored.CaptureSaveSnapshot()
			require.NoError(t, err)
			require.Equal(t, envelope.Snapshot.Inventory.Gold, again.Inventory.Gold)
			require.Equal(t, envelope.Snapshot.World.Position, again.World.Position)
		})
	}
}
