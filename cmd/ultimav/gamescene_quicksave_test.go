package main

import (
	"errors"
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setQuickSaveConfirmationForTest(scene *GameScene, confirm bool) {
	configuration := *scene.gameState.GameConfig
	gameplay := config.GameplayConfiguration{ConfirmQuickSave: confirm}
	if configuration.GameplayConfiguration != nil {
		gameplay = *configuration.GameplayConfiguration
		gameplay.ConfirmQuickSave = confirm
	}
	configuration.GameplayConfiguration = &gameplay
	scene.gameConfig = &configuration
}

func quickSaveGameForTest(scene *GameScene) (*Game, *playerSaveTestRepository) {
	repository := &playerSaveTestRepository{records: map[player_save.SlotID]player_save.Record{
		player_save.Slot06: playerSaveTestRecord(player_save.Slot06, "reviewed", validCommandTestSnapshot()),
		player_save.Slot01: playerSaveTestRecord(player_save.Slot01, "unrelated", validCommandTestSnapshot()),
	}}
	scene.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID, userDescription: "Keep this checkpoint note"})
	scene.setPlayerSaveProvenance(player_save.Slot06, "reviewed")
	game := &Game{currentScene: scene, playerSaveRepository: repository,
		playerSaveNow: func() time.Time { return time.Unix(1000, 0) }, playerSaveGameVersion: "test"}

	return game, repository
}

func TestQuickSaveSurfaceConfirmationMatrixCapturesOutsideCommandCycle(t *testing.T) {
	for _, place := range []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
	}{
		{"town", references.Britain, 0}, {"world", references.BritanniaUnderworld, 0},
		{"underworld", references.BritanniaUnderworld, -1},
	} {
		for _, confirm := range []bool{true, false} {
			t.Run(place.name+"/"+boolLabel(confirm), func(t *testing.T) {
				state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
					WithLocation(place.location).WithFloor(place.floor).WithPlayerAt(15, 15).
					WithRandomSeed(42).WithSystemCallbacks().Build()
				scene := newOwnedSurfaceCompletionTestScene(t, state)
				setQuickSaveConfirmationForTest(scene, confirm)
				game, repository := quickSaveGameForTest(scene)
				before, err := scene.CaptureSaveSnapshot()
				require.NoError(t, err)
				callbacks.Reset()
				if place.location == references.Britain {
					scene.smallMapInputHandlerWithGame(game, ebiten.KeyQ)
				} else {
					scene.largeMapInputHandlerWithGame(game, ebiten.KeyQ)
				}
				if confirm {
					require.Equal(t, SurfaceQuitInput, scene.secondaryKeyState)
					require.Empty(t, repository.saves)
					require.NotNil(t, scene.saveQuestionDialog)
					scene.keyboard.SimulateKeyPress(ebiten.KeyY)
					scene.surfaceQuitSecondaryInput(game)
				}
				require.Len(t, repository.saves, 1, "capture must not see an owned surface command cycle")
				saved := repository.saves[0]
				assert.Equal(t, player_save.Slot06, saved.slot)
				assert.Equal(t, "reviewed", saved.expectedRevision)
				assert.Equal(t, "Keep this checkpoint note", saved.envelope.Metadata.UserDescription)
				assert.Equal(t, "unrelated", repository.records[player_save.Slot01].Revision)
				assert.Equal(t, before.World, saved.envelope.Snapshot.World)
				assert.Equal(t, before.Random, saved.envelope.Snapshot.Random)
				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.Empty(t, callbacks.TimeAdvanced())
				assert.Nil(t, scene.saveQuestionDialog)
				if confirm {
					assert.Equal(t, "Game saved to Slot 6.", requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene).GetMessage())
				} else {
					assert.Zero(t, scene.dialogStack.Count())
					assert.Equal(t, []string{"Game saved to Slot 6."}, callbacks.Messages)
					_, err = scene.CaptureSaveSnapshot()
					assert.NoError(t, err, "plain success output must leave another save immediately eligible")
				}
			})
		}
	}
}

func boolLabel(value bool) string {
	if value {
		return "On"
	}

	return "Off"
}

func TestQuickSaveDefaultSurfaceQuestionPhysicalCancelDoesNotSaveOrSpendTime(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
		t.Run(location.String(), func(t *testing.T) {
			scene, held, _, callbacks := newRunicReferenceTestScene(t, location)
			require.Nil(t, scene.gameConfig, "nil fixtures must retain confirmation by default")
			game, repository := quickSaveGameForTest(scene)
			date, calls := scene.gameState.DateTime, scene.gameState.GetRNG().Calls()
			held[ebiten.KeyCapsLock], held[ebiten.KeyQ] = true, true
			for range 3 {
				require.NoError(t, scene.Update(game))
			}
			require.Equal(t, SurfaceQuitInput, scene.secondaryKeyState)
			require.NotNil(t, scene.saveQuestionDialog)
			delete(held, ebiten.KeyQ)
			require.NoError(t, scene.Update(game))
			held[ebiten.KeyN] = true
			for range 3 {
				require.NoError(t, scene.Update(game))
			}
			assert.Empty(t, repository.saves)
			assert.Nil(t, scene.saveQuestionDialog)
			assert.Zero(t, scene.dialogStack.Count(), "held N must not become New Order")
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, date, scene.gameState.DateTime)
			assert.Equal(t, calls, scene.gameState.GetRNG().Calls())
			assert.Contains(t, callbacks.Messages, "Save game? No")
		})
	}
}

func TestQuickSaveDungeonPhysicalHoldAndQuicknessBoundary(t *testing.T) {
	for _, confirm := range []bool{true, false} {
		for _, quickness := range []bool{true, false} {
			t.Run(boolLabel(confirm)+"/quickness-"+boolLabel(quickness), func(t *testing.T) {
				scene, state, callbacks := newDungeonInputTestScene(t)
				retirePresentationFixtureCaches(t)
				t.Cleanup(scene.Dispose)
				initializeDungeonUpdateTestScene(scene, state)
				setQuickSaveConfirmationForTest(scene, confirm)
				state.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectProtection, TurnsRemaining: 8}
				if quickness {
					state.ActiveDurationEffect.EffectType = game_state.DurationEffectQuickness
				}
				game, repository := quickSaveGameForTest(scene)
				held := map[ebiten.Key]bool{ebiten.KeyCapsLock: true}
				scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
				for save := 1; save <= 2; save++ {
					held[ebiten.KeyQ] = true
					require.NoError(t, scene.Update(game))
					for range 3 {
						require.NoError(t, scene.Update(game))
					}
					delete(held, ebiten.KeyQ)
					require.NoError(t, scene.Update(game))
					if confirm {
						require.Equal(t, DungeonQuitInput, scene.secondaryKeyState)
						require.Len(t, repository.saves, save-1)
						held[ebiten.KeyY] = true
						require.NoError(t, scene.Update(game))
						delete(held, ebiten.KeyY)
						require.NoError(t, scene.Update(game))
						held[ebiten.KeyEnter] = true
						require.NoError(t, scene.Update(game))
						delete(held, ebiten.KeyEnter)
						require.NoError(t, scene.Update(game))
					}
					require.Len(t, repository.saves, save)
					saved := repository.saves[save-1].envelope.Snapshot
					wantMinutes := save
					if quickness {
						wantMinutes = 1
					}
					assert.Equal(t, wantMinutes, saved.World.Calendar.Minute)
					assert.Equal(t, byte(wantMinutes), state.DateTime.Minute)
					assert.Equal(t, 8, saved.Effects.DurationTurns, "Q never runs full end-player processing")
					assert.Equal(t, 8, state.ActiveDurationEffect.TurnsRemaining)
					assert.Equal(t, quickness && save == 1, saved.Dungeon.QuicknessSkipNext)
					assert.True(t, state.IsAtDungeonCommandBoundary())
					assert.Zero(t, scene.dungeonQuitCommandToken)
					assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
					assert.Zero(t, scene.dialogStack.Count())
					assert.Equal(t, "Keep this checkpoint note", repository.saves[save-1].envelope.Metadata.UserDescription)
				}
				assert.Contains(t, callbacks.Messages, "Game saved to Slot 6.")
			})
		}
	}
}

func TestQuickSaveOffFailuresRemainAcknowledgedAndRetainProvenance(t *testing.T) {
	for _, failure := range []string{"no slot", "unavailable", "in flight", "stale", "filesystem"} {
		t.Run(failure, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			retirePresentationFixtureCaches(t)
			t.Cleanup(scene.Dispose)
			initializeDungeonUpdateTestScene(scene, state)
			setQuickSaveConfirmationForTest(scene, false)
			game, repository := quickSaveGameForTest(scene)
			switch failure {
			case "no slot":
				scene.activePlayerSaveSlot, scene.activePlayerSaveRevision = "", ""
			case "unavailable":
				game.playerSaveRepository = nil
			case "in flight":
				scene.visualEffects = append(scene.visualEffects, VisualEffect{Duration: 100})
			case "stale":
				scene.activePlayerSaveRevision = "stale"
			case "filesystem":
				repository.saveErr = errors.New("disk full")
			}
			slot, revision, active := scene.playerSaveProvenance()
			scene.dungeonMapInputHandlerWithGame(game, ebiten.KeyQ)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
			assert.Nil(t, scene.saveQuestionDialog)
			dialog := requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene)
			assert.Contains(t, dialog.GetMessage(), "Save failed:")
			assert.Contains(t, callbacks.Messages, dialog.GetMessage())
			gotSlot, gotRevision, gotActive := scene.playerSaveProvenance()
			assert.Equal(t, slot, gotSlot)
			assert.Equal(t, revision, gotRevision)
			assert.Equal(t, active, gotActive)
			assert.Equal(t, "reviewed", repository.records[player_save.Slot06].Revision)
		})
	}
}
