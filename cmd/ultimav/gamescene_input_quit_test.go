package main

import (
	"errors"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceQuitSaveResultPreservesDestinationAndReportsFailures(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
		for _, outcome := range []string{"success", "write failure", "no active slot", "unavailable"} {
			t.Run(location.String()+"/"+outcome, func(t *testing.T) {
				state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
					WithLocation(location).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
				scene := newSurfaceCompletionTestScene(state)
				capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
					playthrough: playthroughMetadata{id: testPlaythroughID},
					snapshot:    validCommandTestSnapshot(), slot: player_save.Slot07, revision: "reviewed",
				}}
				capture.beforeCapture = func() {
					assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
					assert.Zero(t, scene.dialogStack.Count(), "completion must follow capture and persistence")
				}
				repository := &playerSaveTestRepository{records: map[player_save.SlotID]player_save.Record{
					player_save.Slot07: playerSaveTestRecord(player_save.Slot07, "reviewed", capture.snapshot),
				}}
				game := NewGame(capture)
				game.ConfigurePlayerSaveRepository(repository, nil)
				switch outcome {
				case "write failure":
					repository.saveErr = errors.New("disk full")
				case "no active slot":
					capture.slot, capture.revision = "", ""
				case "unavailable":
					game = nil
				}
				callbacks.Reset()
				dateBefore := state.DateTime
				if location == references.Britain {
					scene.smallMapInputHandler(ebiten.KeyQ)
				} else {
					scene.largeMapInputHandler(ebiten.KeyQ)
				}
				require.Equal(t, SurfaceQuitInput, scene.secondaryKeyState)
				require.Equal(t, 1, scene.dialogStack.Count())
				require.NotNil(t, scene.saveQuestionDialog)
				assert.Equal(t, "Save game?", scene.saveQuestionDialog.GetMessage())
				scene.keyboard.SimulateKeyPress(ebiten.KeyY)
				scene.surfaceQuitSecondaryInput(game)

				dialog := requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene)
				require.Equal(t, 1, scene.dialogStack.Count())
				assert.True(t, dialog.IsVisible())
				assert.True(t, dialog.IsUserDismissed())
				assert.Contains(t, callbacks.Messages, dialog.GetMessage())
				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.Equal(t, dateBefore, state.DateTime)
				assert.Empty(t, callbacks.TimeAdvanced())
				if outcome == "success" {
					assert.Equal(t, "Game saved to Slot 7.", dialog.GetMessage())
					require.Len(t, repository.saves, 1)
					assert.Equal(t, player_save.Slot07, repository.saves[0].slot)
					assert.Equal(t, "reviewed", repository.saves[0].expectedRevision)
					assert.Equal(t, repository.records[player_save.Slot07].Revision, capture.revision)
				} else {
					assert.Contains(t, dialog.GetMessage(), "Save failed:")
					assert.NotContains(t, dialog.GetMessage(), "Game saved")
					assert.Equal(t, "reviewed", repository.records[player_save.Slot07].Revision)
				}
				if outcome == "no active slot" {
					assert.Contains(t, dialog.GetMessage(), "Save or load a game first.")
					assert.Empty(t, repository.saves)
				}
				savesBefore := len(repository.saves)
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				assert.True(t, scene.updateDialogs(), "dismissal still owns its frame")
				assert.Zero(t, scene.dialogStack.Count())
				assert.Equal(t, savesBefore, len(repository.saves))
			})
		}
	}
}

func TestQuitSaveResultWaitsForReleaseAndDismissalCannotFallThrough(t *testing.T) {
	for _, dismissKey := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
		t.Run(dismissKey.String(), func(t *testing.T) {
			scene, state, _ := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			held := make(map[ebiten.Key]bool)
			scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
			held[ebiten.KeyQ] = true
			require.NoError(t, scene.Update(nil))
			delete(held, ebiten.KeyQ)
			held[ebiten.KeyY], held[dismissKey] = true, true
			// Route Y explicitly: a simultaneous Escape takes precedence in the bound-key list.
			scene.keyboard.SimulateKeyPress(ebiten.KeyY)
			require.NoError(t, scene.Update(nil))
			dialog := requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene)
			position, date := state.MapState.PlayerLocation, state.DateTime

			for range 3 {
				require.NoError(t, scene.Update(nil))
				require.False(t, dialog.IsDismissed(), "held keys cannot dismiss the new result")
			}
			clear(held)
			require.NoError(t, scene.Update(nil))
			held[ebiten.KeyQ] = true
			require.NoError(t, scene.Update(nil))
			require.False(t, dialog.IsDismissed(), "Q cannot start another save behind the result")
			delete(held, ebiten.KeyQ)
			held[dismissKey] = true
			require.NoError(t, scene.Update(nil))
			require.True(t, dialog.IsDismissed())
			require.Zero(t, scene.dialogStack.Count())
			for range 3 {
				require.NoError(t, scene.Update(nil))
			}
			assert.Zero(t, scene.dialogStack.Count(), "held Escape cannot reopen the menu")
			assert.Equal(t, position, state.MapState.PlayerLocation, "held Enter cannot turn the party")
			assert.Equal(t, date, state.DateTime)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
			clear(held)
			require.NoError(t, scene.Update(nil))
			held[ebiten.KeyQ] = true
			require.NoError(t, scene.Update(nil))
			assert.Equal(t, DungeonQuitInput, scene.secondaryKeyState, "fresh gameplay input resumes")
		})
	}
}
