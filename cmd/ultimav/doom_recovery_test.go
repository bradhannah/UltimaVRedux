package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/doom_recovery"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

type doomRecoveryTestRepository struct {
	record       doom_recovery.Record
	loadErr      error
	replaceErr   error
	replacements []save_snapshot.Envelope
}

func (repository *doomRecoveryTestRepository) Load() (doom_recovery.Record, error) {
	return repository.record, repository.loadErr
}

func (repository *doomRecoveryTestRepository) Replace(envelope save_snapshot.Envelope) (doom_recovery.Record, error) {
	repository.replacements = append(repository.replacements, envelope)
	if repository.replaceErr != nil {
		return doom_recovery.Record{}, repository.replaceErr
	}
	repository.record = doom_recovery.Record{Envelope: envelope, Revision: "checkpoint-revision"}

	return repository.record, nil
}

func TestDoomEntryCapturesBeforeRoutingAndPersistsOnlyAcceptedTransition(t *testing.T) {
	scene, _ := newDoomEntryRecoveryScene(t, 0)
	seedSceneJournalPresentation(t, scene)
	scene.activePlaythrough.userDescription = "Before Doom"
	repository := &doomRecoveryTestRepository{}
	game := &Game{
		doomRecoveryRepository: repository,
		doomRecoveryNow:        func() time.Time { return time.Date(2026, 8, 20, 1, 2, 3, 0, time.UTC) },
		doomRunID:              func() (string, error) { return "accepted-run", nil },
		playerSaveGameVersion:  "test",
	}
	before, err := scene.CaptureSaveSnapshot()
	require.NoError(t, err)

	scene.largeMapInputHandlerWithGame(game, ebiten.KeyE)

	require.Len(t, repository.replacements, 1)
	checkpoint := repository.replacements[0]
	assert.Equal(t, "accepted-run", checkpoint.Metadata.DoomRunID)
	assert.Equal(t, testPlaythroughID, checkpoint.Metadata.PlaythroughID)
	assert.Equal(t, "Before Doom", checkpoint.Metadata.UserDescription)
	assert.Equal(t, doomRecoveryDescription, checkpoint.Metadata.Description)
	assert.Equal(t, before, checkpoint.Snapshot)
	assert.Equal(t, "accepted-run", scene.doomRunAssociation())
	assert.True(t, scene.gameState.EventQueueManager.HasWorkflow("enter_location"))
}

func TestDoomEntryRejectionsAndEntranceCombatDoNotReplaceCheckpoint(t *testing.T) {
	tests := []struct {
		name   string
		living int
		mutate func(*GameScene)
	}{
		{name: "sealed admission", mutate: func(scene *GameScene) { scene.gameState.SetDungeonState(references.Doom, false) }},
		{name: "invalid terrain", mutate: func(scene *GameScene) {
			position := scene.gameState.MapState.PlayerLocation.Position
			scene.gameState.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
		}},
		{name: "vehicle rejection", mutate: func(scene *GameScene) {
			npc := references.NPCReference{NpcType: references.Vehicle}
			scene.gameState.PartyVehicle = *map_units.NewNPCFriendlyVehicle(references.HorseVehicle, npc)
		}},
		{name: "Shadowlord entrance combat", living: 1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, _ := newDoomEntryRecoveryScene(t, test.living)
			if test.mutate != nil {
				test.mutate(scene)
			}
			repository := &doomRecoveryTestRepository{
				record: doom_recovery.Record{Revision: "existing"},
			}
			game := &Game{
				doomRecoveryRepository: repository,
				doomRecoveryNow:        time.Now,
				doomRunID:              func() (string, error) { return "must-not-run", nil },
			}

			scene.largeMapInputHandlerWithGame(game, ebiten.KeyE)

			assert.Empty(t, repository.replacements)
			assert.Equal(t, "existing", repository.record.Revision)
			assert.Empty(t, scene.doomRunAssociation())
		})
	}
}

func TestDoomCheckpointWriteFailureDoesNotBlockEntry(t *testing.T) {
	scene, callbacks := newDoomEntryRecoveryScene(t, 0)
	repository := &doomRecoveryTestRepository{replaceErr: errors.New("injected write failure")}
	game := &Game{
		doomRecoveryRepository: repository, doomRecoveryNow: time.Now,
		doomRunID: func() (string, error) { return "write-failed-run", nil },
	}

	scene.largeMapInputHandlerWithGame(game, ebiten.KeyE)

	assert.True(t, scene.gameState.EventQueueManager.HasWorkflow("enter_location"))
	assert.Equal(t, "write-failed-run", scene.doomRunAssociation())
	assert.Contains(t, callbacks.Messages, "Pre-Doom recovery is unavailable.")
}

func TestLaterAcceptedDoomEntrySupersedesCheckpointAndAssociation(t *testing.T) {
	repository, err := doom_recovery.NewRepository(t.TempDir(), nil)
	require.NoError(t, err)
	runIDs := []string{"first-run", "later-run"}
	game := &Game{
		doomRecoveryRepository: repository,
		doomRecoveryNow:        func() time.Time { return time.Date(2026, 8, 20, 1, 2, 3, 0, time.UTC) },
		doomRunID: func() (string, error) {
			runID := runIDs[0]
			runIDs = runIDs[1:]

			return runID, nil
		},
	}
	first, _ := newDoomEntryRecoveryScene(t, 0)
	first.largeMapInputHandlerWithGame(game, ebiten.KeyE)
	firstRecord, err := repository.Load()
	require.NoError(t, err)
	assert.Equal(t, "first-run", firstRecord.Envelope.Metadata.DoomRunID)

	later, _ := newDoomEntryRecoveryScene(t, 0)
	later.gameState.PartyState.Inventory.Gold.IncrementByOne()
	wantLater, err := later.CaptureSaveSnapshot()
	require.NoError(t, err)
	later.largeMapInputHandlerWithGame(game, ebiten.KeyE)
	laterRecord, err := repository.Load()
	require.NoError(t, err)

	assert.Equal(t, "later-run", laterRecord.Envelope.Metadata.DoomRunID)
	assert.Equal(t, "later-run", later.doomRunAssociation())
	assert.Equal(t, wantLater, laterRecord.Envelope.Snapshot)
	assert.NotEqual(t, firstRecord.Revision, laterRecord.Revision)
}

func TestDoomRunAssociationClearsAfterLeavingDoom(t *testing.T) {
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Doom).
		WithFloor(0).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{gameState: state, activeDoomRunID: "active-run"}

	scene.clearDoomRunAssociationOutsideRun()
	assert.Equal(t, "active-run", scene.doomRunAssociation())
	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	scene.clearDoomRunAssociationOutsideRun()

	assert.Empty(t, scene.doomRunAssociation())
	assert.Empty(t, scene.doomRunAssociationForPlayerSave())
}

func TestReturnBeforeDoomBuildsAndValidatesFreshUnprovenancedScene(t *testing.T) {
	source, snapshot := newDoomRecoverySource(t, "matching-run")
	candidate := &playerSaveTestScene{snapshot: snapshot, slot: "", revision: "", doomRunID: "stale"}
	repository := &doomRecoveryTestRepository{record: doomRecoveryRecord(snapshot, "matching-run")}
	game := NewGame(source)
	game.doomRecoveryRepository = repository
	game.doomRecoverySceneBuilder = func(configuration *config.UltimaVConfiguration, got save_snapshot.SaveSnapshot) (Scene, error) {
		assert.Same(t, source.gameConfig, configuration)
		assert.Equal(t, snapshot, got)

		return candidate, nil
	}

	require.NoError(t, game.returnBeforeDoom(source))

	assert.Same(t, candidate, game.currentScene)
	assert.Equal(t, 2, candidate.preflights, "recovery preflight and standard activation preflight")
	assert.Equal(t, 1, candidate.captures)
	assert.Equal(t, 1, candidate.activations)
	assert.Empty(t, candidate.doomRunID)
	_, _, hasProvenance := candidate.playerSaveProvenance()
	assert.False(t, hasProvenance)
	assert.True(t, source.disposed)
}

func TestReturnBeforeDoomRealSceneHydrationPreservesCanonicalStateAndRNG(t *testing.T) {
	gameDataPath := originaldata.Directory(t)
	legacySavePath := filepath.Join(gameDataPath, "SAVED.GAM")
	if _, err := os.Stat(legacySavePath); err != nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	configuration := snapshotTestGameConfig(gameDataPath)
	rawDataOvl, err := os.ReadFile(filepath.Join(gameDataPath, "DATA.OVL"))
	require.NoError(t, err)
	configuration.RawDataOvl = rawDataOvl
	source, err := newGameSceneFromOriginalSave(configuration, legacySavePath)
	require.NoError(t, err)
	source.keyboard = newPresentationTestKeyboard()
	t.Cleanup(source.Dispose)
	entrance := source.gameState.GetDungeonEntranceCoordinates()[references.Doom]
	_, err = source.gameState.DebugSetWorldPosition(
		references.BritanniaUnderworld,
		references.FloorNumber(references.UNDERWORLD),
		entrance,
	)
	require.NoError(t, err)
	source.gameState.SetRandomSeed(465)
	for range 7 {
		source.gameState.RollDice(100)
	}
	source.gameState.ProcessWindThroughElapsed(54_321)
	source.gameState.ElapsedMs = 54_321
	seedSceneJournalPresentation(t, source)
	snapshot, err := source.CaptureSaveSnapshot()
	require.NoError(t, err)
	source.setPlayerSaveProvenance(player_save.Slot03, "origin-revision")
	source.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	source.setDoomRunAssociation("matching-run")
	_, _, sourceHasProvenance := source.playerSaveProvenance()
	require.True(t, sourceHasProvenance)

	repository, err := doom_recovery.NewRepository(t.TempDir(), source.ValidateSaveSnapshotCompatibility)
	require.NoError(t, err)
	_, err = repository.Replace(doomRecoveryRecord(snapshot, "matching-run").Envelope)
	require.NoError(t, err)
	game := NewGame(source)
	game.doomRecoveryRepository = repository
	source.output.AddText("Only in the departing Doom run.")
	source.journalChanged(2, 0)
	game.doomRecoverySceneBuilder = func(
		configuration *config.UltimaVConfiguration,
		snapshot save_snapshot.SaveSnapshot,
	) (Scene, error) {
		scene, buildErr := NewGameSceneFromSaveSnapshot(configuration, snapshot)
		if buildErr == nil {
			scene.keyboard = newPresentationTestKeyboard()
		}
		return scene, buildErr
	}

	source.setDoomRunAssociation("mismatched-run")
	require.ErrorIs(t, game.returnBeforeDoom(source), ErrDoomRecoveryAssociation)
	assert.Same(t, source, game.currentScene)
	source.setDoomRunAssociation("matching-run")
	require.NoError(t, game.returnBeforeDoom(source))
	restored, ok := game.currentScene.(*GameScene)
	require.True(t, ok)
	t.Cleanup(restored.Dispose)

	recaptured, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, snapshot, recaptured)
	_, _, hasProvenance := restored.playerSaveProvenance()
	assert.False(t, hasProvenance)
	assert.Empty(t, restored.doomRunAssociation())
	expectedRNG := beradrng.Restore(snapshot.Random.Seed, snapshot.Random.SourceCalls)
	assert.Equal(t, expectedRNG.Intn(1000)+1, restored.gameState.RollDice(1000))
}

func TestReturnBeforeDoomFailuresRetainTerminalSceneAndCheckpoint(t *testing.T) {
	buildErr := errors.New("injected build failure")
	activationCandidate := &playerSaveTestScene{activatePanic: true}
	tests := []struct {
		name      string
		runID     string
		loadErr   error
		mutate    func(*save_snapshot.SaveSnapshot)
		builder   func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error)
		wantError error
	}{
		{name: "missing", runID: "run", loadErr: doom_recovery.ErrCheckpointNotFound, wantError: doom_recovery.ErrCheckpointNotFound},
		{name: "corrupt", runID: "run", loadErr: save_snapshot.ErrInvalidEnvelope, wantError: save_snapshot.ErrInvalidEnvelope},
		{name: "mismatch", runID: "other", wantError: ErrDoomRecoveryAssociation},
		{name: "unassociated", runID: "", wantError: ErrDoomRecoveryUnassociated},
		{name: "unrelated origin", runID: "run", mutate: func(snapshot *save_snapshot.SaveSnapshot) {
			snapshot.World.Position.X++
		}, wantError: ErrDoomRecoveryOrigin},
		{name: "build", runID: "run", builder: func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
			return nil, buildErr
		}, wantError: buildErr},
		{name: "hydration mismatch", runID: "run", builder: func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
			snapshot.Random.SourceCalls++

			return &playerSaveTestScene{snapshot: snapshot}, nil
		}, wantError: ErrDoomRecoveryHydrationMismatch},
		{name: "activation", runID: "run", builder: func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
			activationCandidate.snapshot = snapshot

			return activationCandidate, nil
		}, wantError: errors.New("activate")},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			source, snapshot := newDoomRecoverySource(t, "run")
			if test.name == "unassociated" {
				source.setDoomRunAssociation("")
			}
			if test.mutate != nil {
				test.mutate(&snapshot)
			}
			repository := &doomRecoveryTestRepository{
				record: doomRecoveryRecord(snapshot, test.runID), loadErr: test.loadErr,
			}
			builder := test.builder
			if builder == nil {
				builder = func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
					return &playerSaveTestScene{snapshot: snapshot}, nil
				}
			}
			game := NewGame(source)
			game.doomRecoveryRepository = repository
			game.doomRecoverySceneBuilder = builder

			err := game.returnBeforeDoom(source)

			require.Error(t, err)
			if test.name != "activation" {
				assert.ErrorIs(t, err, test.wantError)
			} else {
				assert.Contains(t, err.Error(), "activate")
			}
			assert.Same(t, source, game.currentScene)
			assert.False(t, source.disposed)
			assert.Equal(t, test.runID, repository.record.Envelope.Metadata.DoomRunID)
			if test.name == "activation" {
				assert.Equal(t, 1, activationCandidate.disposals)
			}
		})
	}
}

func newDoomEntryRecoveryScene(t *testing.T, livingShadowlords int) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.UNDERWORLD)).
		WithRandomSeed(465).
		WithSystemCallbacks().
		Build()
	state.MapState.PlayerLocation.Position = state.GetDungeonEntranceCoordinates()[references.Doom]
	state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	state.SetDungeonState(references.Doom, true)
	for lord := references.Faulinei; lord <= references.Nosfentor; lord++ {
		state.ShadowlordState.Alive[lord] = int(lord) < livingShadowlords
		if !state.ShadowlordState.Alive[lord] {
			state.ShadowlordState.TownAssignments[lord] = 0
		}
	}
	callbacks.Reset()

	return &GameScene{
		activePlaythrough: playthroughMetadata{id: testPlaythroughID},
		gameState:         state, gameConfig: state.GameConfig, keyboard: newPresentationTestKeyboard(),
		output: text.NewOutput(nil, 1, 64, 80),
	}, callbacks
}

func newDoomRecoverySource(t *testing.T, runID string) (*GameScene, save_snapshot.SaveSnapshot) {
	t.Helper()
	state, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithFloor(references.FloorNumber(references.UNDERWORLD)).
		WithSystemCallbacks().
		Build()
	entrance := state.GetDungeonEntranceCoordinates()[references.Doom]
	snapshot := validCommandTestSnapshot()
	snapshot.World.Position = save_snapshot.PositionSnapshot{
		Location: int(references.BritanniaUnderworld), X: int(entrance.X), Y: int(entrance.Y), Floor: int(references.UNDERWORLD),
	}
	snapshot.Random = save_snapshot.RandomSnapshot{Seed: 465, SourceCalls: 73}
	source := &GameScene{
		activePlaythrough: playthroughMetadata{id: testPlaythroughID},
		gameState:         state, gameConfig: state.GameConfig, activeDoomRunID: runID,
	}

	return source, snapshot
}

func doomRecoveryRecord(snapshot save_snapshot.SaveSnapshot, runID string) doom_recovery.Record {
	return doom_recovery.Record{Envelope: save_snapshot.Envelope{
		Format: save_snapshot.EnvelopeFormat, EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
		Metadata: save_snapshot.Metadata{
			Name: doomRecoveryName, Description: doomRecoveryDescription, CreatedAt: time.Now().UTC(), DoomRunID: runID,
			PlaythroughID: testPlaythroughID,
		},
		Snapshot: snapshot,
	}, Revision: "unchanged-checkpoint"}
}
