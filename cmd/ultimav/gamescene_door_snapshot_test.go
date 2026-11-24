package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestDoorPlayerSlotSaveLoadAfterManualAndAutomaticOpening(t *testing.T) {
	for _, automatic := range []bool{false, true} {
		name := "manual O then direction"
		if automatic {
			name = "movement auto-open"
		}
		t.Run(name, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).
				WithPlayerAt(10, 14).WithRandomSeed(673).WithSystemCallbacks().Build()
			// Isolate the opening/save lifecycle from unrelated NPC conversations;
			// retain the real controller and its normal actor-phase execution.
			state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
			door := references.Position{X: 11, Y: 14}
			require.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), state.GetCurrentSmallLocationReference().GetTileNumber(door, 0))
			initial, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			configuration := snapshotTestGameConfig(state.GameConfig.SavedConfigData.DataFilePath)
			configuration.RawDataOvl, err = os.ReadFile(filepath.Join(configuration.SavedConfigData.DataFilePath, "DATA.OVL"))
			require.NoError(t, err)
			source, err := NewGameSceneFromSaveSnapshot(configuration, initial)
			require.NoError(t, err)
			// The native test window is hidden. Input acceptance belongs to this
			// fixture's detached focus source, not the maintainer's desktop focus.
			focused := true
			source.transientCommandInput.focused = func() bool { return focused }
			t.Cleanup(source.Dispose)
			now := time.Unix(1, 0)
			source.now = func() time.Time { return now }
			source.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
			repository, err := player_save.NewRepository(t.TempDir(), source.ValidateSaveSnapshotCompatibility)
			require.NoError(t, err)
			game := NewGame(source)
			game.ConfigurePlayerSaveRepository(repository, func(c *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
				candidate, buildErr := NewGameSceneFromSaveSnapshot(c, snapshot)
				if buildErr == nil {
					candidate.transientCommandInput.focused = func() bool { return true }
					candidate.now = func() time.Time { return now }
					t.Cleanup(candidate.Dispose)
				}
				return candidate, buildErr
			})
			baseline, err := game.savePlayerSaveSlot(player_save.Slot01, "", "Before opening")
			require.NoError(t, err)
			press := func(scene *GameScene, key ebiten.Key) {
				t.Helper()
				scene.keyboard.SimulateKeyPress(key)
				require.NoError(t, scene.Update(game))
			}
			if !automatic {
				press(source, ebiten.KeyO)
				require.Equal(t, OpenDirectionInput, source.secondaryKeyState)
				_, err = game.quickSavePlayerSave()
				require.ErrorIs(t, err, ErrGameSceneSnapshotNotRepresentable)
			}
			press(source, ebiten.KeyRight)
			if automatic {
				require.True(t, source.autoOpenDoorState.IsActive(), "movement input must start the real orchestration")
				// Losing focus after commitment rejects new input, but must not
				// strand the real automatic-door phases or their save restriction.
				focused = false
				for frame := 0; source.autoOpenDoorState.IsActive() && frame < 12; frame++ {
					_, err = game.quickSavePlayerSave()
					require.ErrorIs(t, err, ErrGameSceneSnapshotNotRepresentable)
					require.ErrorContains(t, err, "automatic door animation")
					onDisk, loadErr := repository.Load(player_save.Slot01)
					require.NoError(t, loadErr)
					assert.Equal(t, baseline.Revision, onDisk.Revision, "rejected in-flight saves cannot overwrite the slot")
					now = now.Add(time.Duration(autoOpenPauseDurationMs) * time.Millisecond)
					require.NoError(t, source.Update(game))
				}
				require.False(t, source.autoOpenDoorState.IsActive(), "all auto-open phases must really finish")
				focused = true
			}
			require.Equal(t, PrimaryInput, source.secondaryKeyState)
			require.True(t, source.gameState.MapState.HasTransientDoor())
			require.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), source.gameState.GetEffectiveTile(&door))
			checkpoint, err := source.CaptureSaveSnapshot()
			require.NoError(t, err)
			require.NotNil(t, checkpoint.OpenDoor)
			quick, err := game.quickSavePlayerSave()
			require.NoError(t, err)
			assert.Equal(t, checkpoint, quick.Envelope.Snapshot, "saving cannot change the door, clock, RNG or resources")
			unchanged, err := source.CaptureSaveSnapshot()
			require.NoError(t, err)
			assert.Equal(t, checkpoint, unchanged)
			require.NoError(t, game.loadPlayerSaveSlot(player_save.Slot01, quick.Revision))
			restored, ok := game.currentScene.(*GameScene)
			require.True(t, ok)
			after, err := restored.CaptureSaveSnapshot()
			require.NoError(t, err)
			assert.Equal(t, checkpoint, after, "real repository decode and replacement-scene hydration preserve the checkpoint")
			assert.False(t, restored.autoOpenDoorState.IsActive(), "loading a stable open door must not restart animation")

			// Enter through normal movement input when still beside the doorway,
			// then let actual Pass commands expire its timer while occupied.
			if restored.gameState.MapState.PlayerLocation.Position == door.Left() {
				press(restored, ebiten.KeyRight)
			}
			require.Equal(t, door, restored.gameState.MapState.PlayerLocation.Position)
			for range 3 {
				press(restored, ebiten.KeySpace)
			}
			require.NotNil(t, restored.gameState.MapState.TrackedOpenDoor())
			require.Zero(t, restored.gameState.MapState.TrackedOpenDoor().RemainingTurns)
			occupied, err := game.quickSavePlayerSave()
			require.NoError(t, err, "occupied expiry is a stable, saveable gameplay boundary")
			require.NoError(t, game.loadPlayerSaveSlot(player_save.Slot01, occupied.Revision))
			loaded := game.currentScene.(*GameScene)
			loadedSnapshot, err := loaded.CaptureSaveSnapshot()
			require.NoError(t, err)
			assert.Equal(t, occupied.Envelope.Snapshot, loadedSnapshot)
			press(loaded, ebiten.KeyLeft)
			require.Equal(t, door.Left(), loaded.gameState.MapState.PlayerLocation.Position)
			assert.False(t, loaded.gameState.MapState.HasTransientDoor())
			assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), loaded.gameState.GetEffectiveTile(&door),
				"the next normal movement cycle closes the restored expired doorway after departure")
		})
	}
}
