package main

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
)

// The scene starts with initialized X origin 96, then reaches X=120 through the
// production successful-step writer. Hydration at that final position uses 112.
func newOutdoorWindowSnapshotScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := game_state.NewIntegrationTestBuilder(t).WithFloor(-1).
		WithPlayerAt(119, 120).WithRandomSeed(35).Build()
	state.CurrentNPCAIController = state.GetCurrentLargeMapNPCAIController()
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	prepareSceneBlinkRay(state, 100, 124, 140)
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	configuration := snapshotTestGameConfig(state.GameConfig.SavedConfigData.DataFilePath)
	configuration.RawDataOvl, err = os.ReadFile(filepath.Join(configuration.SavedConfigData.DataFilePath, "DATA.OVL"))
	require.NoError(t, err)
	scene, err := NewGameSceneFromSaveSnapshot(configuration, snapshot)
	require.NoError(t, err)
	t.Cleanup(scene.Dispose)
	scene.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	scene.moveToNewPositionByDirection(references.Right)
	require.Equal(t, references.Position{X: 120, Y: 120}, scene.gameState.MapState.PlayerLocation.Position)

	return scene
}

func buildOutdoorWindowCandidate(t *testing.T, configuration *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (*GameScene, error) {
	t.Helper()
	candidate, err := NewGameSceneFromSaveSnapshot(configuration, snapshot)
	if err == nil {
		t.Cleanup(candidate.Dispose)
	}

	return candidate, err
}

func assertOutdoorWindowNextBlink(t *testing.T, scene *GameScene, x references.Coordinate) {
	t.Helper()
	require.Equal(t, references.FloorNumber(-1), scene.gameState.MapState.PlayerLocation.Floor)
	require.Equal(t, game_state.BlinkSuccess, scene.gameState.BlinkTeleport(references.Right))
	assert.Equal(t, references.Position{X: x, Y: 120}, scene.gameState.MapState.PlayerLocation.Position)
}

func TestOutdoorWindowPlayerSaveContinueAndLoadNextBlink(t *testing.T) {
	source := newOutdoorWindowSnapshotScene(t)
	game := NewGame(source)
	repository, err := player_save.NewRepository(t.TempDir(), source.ValidateSaveSnapshotCompatibility)
	require.NoError(t, err)
	game.ConfigurePlayerSaveRepository(repository, func(configuration *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		return buildOutdoorWindowCandidate(t, configuration, snapshot)
	})
	saved, err := game.savePlayerSaveSlot(player_save.Slot01, "", "Outdoor window history")
	require.NoError(t, err)
	assertOutdoorWindowNextBlink(t, source, 124)
	require.NoError(t, game.loadPlayerSaveSlot(player_save.Slot01, saved.Revision))
	loaded, ok := game.currentScene.(*GameScene)
	require.True(t, ok)
	assertOutdoorWindowNextBlink(t, loaded, 140)
}

func outdoorWindowSnapshotRecord(t *testing.T, source *GameScene, reference string) save_snapshot.Record {
	t.Helper()
	record := snapshotCommandTestRecord(t, reference, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	snapshot, err := source.CaptureSaveSnapshot()
	require.NoError(t, err)
	record.Envelope.Snapshot = snapshot

	return record
}

func TestOutdoorWindowWorkbenchLoadAndRollbackNextBlink(t *testing.T) {
	source := newOutdoorWindowSnapshotScene(t)
	record := outdoorWindowSnapshotRecord(t, source, "personal:outdoor-window")
	repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{record.Reference.String(): record}}
	game := NewGame(source)
	game.ConfigureSnapshotRepository(repository, func(configuration *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		return buildOutdoorWindowCandidate(t, configuration, snapshot)
	})
	result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{
		"reference": record.Reference.String(), "expectedRevision": record.Revision,
	})
	require.True(t, result.Success, result.ErrorMessage)
	var loaded debugging.SaveSnapshotLoadData
	require.NoError(t, json.Unmarshal(result.Data, &loaded))
	assertOutdoorWindowNextBlink(t, game.currentScene.(*GameScene), 140)
	rollback := repository.records[loaded.RollbackReference]
	result = executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{
		"reference": rollback.Reference.String(), "expectedRevision": rollback.Revision,
	})
	require.True(t, result.Success, result.ErrorMessage)
	assertOutdoorWindowNextBlink(t, game.currentScene.(*GameScene), 140)
}

type outdoorWindowRejectedCandidate struct {
	*GameScene
}

func (*outdoorWindowRejectedCandidate) PreflightActivation() error {
	return errors.New("test rejects the hydrated outdoor candidate")
}

func TestOutdoorWindowFailedReplacementPreservesOldSceneNextBlink(t *testing.T) {
	for _, failActivation := range []bool{false, true} {
		source := newOutdoorWindowSnapshotScene(t)
		record := outdoorWindowSnapshotRecord(t, source, "personal:outdoor-window")
		repository := &snapshotCommandTestRepository{records: map[string]save_snapshot.Record{record.Reference.String(): record}}
		game := NewGame(source)
		game.ConfigureSnapshotRepository(repository, func(configuration *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
			candidate, err := buildOutdoorWindowCandidate(t, configuration, snapshot)
			if err != nil {
				return nil, err
			}
			if failActivation {
				return &outdoorWindowRejectedCandidate{GameScene: candidate}, nil
			}
			candidate.Dispose()

			return nil, errors.New("test rejects candidate after hydration")
		})
		result := executeSnapshotTestCommand(t, game, debugging.CommandSaveSnapshotLoad, map[string]any{
			"reference": record.Reference.String(), "expectedRevision": record.Revision,
		})
		require.False(t, result.Success)
		assert.Same(t, source, game.currentScene)
		assertOutdoorWindowNextBlink(t, source, 124)
	}
}
