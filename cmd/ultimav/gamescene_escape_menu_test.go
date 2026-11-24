package main

import (
	"errors"
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

type escapeMenuCaptureScene struct {
	playerSaveTestScene
	beforeCapture func()
}

func TestEscapeMenuButtonOrderAndWidth(t *testing.T) {
	scene := newEscapeMenuTestScene()
	scene.DoEscapeMenu(NewGame(scene))
	menu := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)

	assert.Equal(t, escapeMenuButtonWidthPercent, menu.ButtonWidthPercent())
	assert.Equal(t, []string{
		"Quick Save",
		"Save",
		"Load",
		"Configure",
		"Return to Main Menu",
		"Exit to DOS",
		"Help",
	}, menu.ButtonTexts())
}

func TestEscapeMenuDeparturePromptsOfferSaveChoicesAndCancelBackToMenu(t *testing.T) {
	for _, test := range []struct {
		name         string
		buttonIndex  int
		withoutLabel string
	}{
		{name: "main menu", buttonIndex: 4, withoutLabel: "Main Menu Without Saving"},
		{name: "application", buttonIndex: 5, withoutLabel: "Exit Without Saving"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene := newEscapeMenuTestScene()
			game := NewGame(scene)

			activateEscapeMenuAction(t, scene, game, test.buttonIndex)
			prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
			assert.Equal(t, []string{"Quick Save", "Save", test.withoutLabel, "Cancel"}, prompt.ButtonTexts())

			for range 3 {
				prompt.GoDown()
			}
			pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)
			menu := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
			assert.Equal(t, "Quick Save", menu.ButtonTexts()[0])
		})
	}
}

func TestEscapeMenuDepartureWithoutSaveCompletesDestination(t *testing.T) {
	t.Run("main menu", func(t *testing.T) {
		scene := newEscapeMenuTestScene()
		candidate := &playerSaveTestScene{configuration: scene.gameConfig}
		game := NewGame(scene)
		game.mainMenuSceneBuilder = func(*config.UltimaVConfiguration) (Scene, error) {
			return candidate, nil
		}

		activateEscapeMenuAction(t, scene, game, 4)
		prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
		prompt.GoDown()
		prompt.GoDown()
		pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)

		assert.Same(t, candidate, game.currentScene)
		assert.Equal(t, 1, candidate.activations)
	})

	t.Run("application", func(t *testing.T) {
		scene := newEscapeMenuTestScene()
		game := NewGame(scene)

		activateEscapeMenuAction(t, scene, game, 5)
		prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
		prompt.GoDown()
		prompt.GoDown()
		pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)

		assert.True(t, game.exitRequested)
	})
}

func TestEscapeMenuMainMenuFailureKeepsGameplayAndReopensPrompt(t *testing.T) {
	scene := newEscapeMenuTestScene()
	game := NewGame(scene)
	game.mainMenuSceneBuilder = func(*config.UltimaVConfiguration) (Scene, error) {
		return nil, errors.New("menu unavailable")
	}

	activateEscapeMenuAction(t, scene, game, 4)
	prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	prompt.GoDown()
	prompt.GoDown()
	pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)

	assert.Same(t, scene, game.currentScene)
	reopened := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	assert.Equal(t, "Main Menu Without Saving", reopened.ButtonTexts()[2])
}

func TestEscapeMenuDepartureSaveCancellationReturnsToPrompt(t *testing.T) {
	for _, choice := range []int{0, 1} {
		t.Run(fmt.Sprintf("choice %d", choice), func(t *testing.T) {
			scene := newEscapeMenuTestScene()
			game := NewGame(scene)
			game.ConfigurePlayerSaveRepository(&playerSaveTestRepository{
				records:   make(map[player_save.SlotID]player_save.Record),
				summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
			}, nil)

			activateEscapeMenuAction(t, scene, game, 4)
			prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
			for range choice {
				prompt.GoDown()
			}
			pressEscapeMenuWidgetKey(scene, prompt, ebiten.KeyEnter)
			browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, scene)
			pressEscapeMenuWidgetKey(scene, browser, ebiten.KeyEscape)

			reopened := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
			assert.Equal(t, "Main Menu Without Saving", reopened.ButtonTexts()[2])
		})
	}
}

func TestEscapeMenuDepartureQuickSaveSuccessExitsAfterDialogFreeCapture(t *testing.T) {
	ui := newEscapeMenuTestScene()
	capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
		playthrough: playthroughMetadata{id: testPlaythroughID},
		snapshot:    validCommandTestSnapshot(),
		slot:        player_save.Slot02,
		revision:    "reviewed",
	}}
	capture.beforeCapture = func() {
		assert.Zero(t, ui.dialogStack.Count())
	}
	repository := &playerSaveTestRepository{records: map[player_save.SlotID]player_save.Record{
		player_save.Slot02: playerSaveTestRecord(player_save.Slot02, "reviewed", capture.snapshot),
	}}
	game := NewGame(capture)
	game.ConfigurePlayerSaveRepository(repository, nil)

	activateEscapeMenuAction(t, ui, game, 5)
	prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, ui)
	pressEscapeMenuWidgetKey(ui, prompt, ebiten.KeyEnter)

	require.Len(t, repository.saves, 1)
	assert.True(t, game.exitRequested)
}

func TestEscapeMenuDepartureExplicitSaveSuccessReturnsToMainMenu(t *testing.T) {
	ui := newEscapeMenuTestScene()
	capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
		playthrough: playthroughMetadata{id: testPlaythroughID},
		snapshot:    validCommandTestSnapshot(),
	}}
	capture.beforeCapture = func() {
		assert.Zero(t, ui.dialogStack.Count())
	}
	repository := &playerSaveTestRepository{
		records:   make(map[player_save.SlotID]player_save.Record),
		summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
	}
	candidate := &playerSaveTestScene{configuration: ui.gameConfig}
	game := NewGame(capture)
	game.ConfigurePlayerSaveRepository(repository, nil)
	game.mainMenuSceneBuilder = func(*config.UltimaVConfiguration) (Scene, error) {
		return candidate, nil
	}

	activateEscapeMenuAction(t, ui, game, 4)
	prompt := requireEscapeMenuDialog[*widgets.ButtonListModal](t, ui)
	prompt.GoDown()
	pressEscapeMenuWidgetKey(ui, prompt, ebiten.KeyEnter)
	browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
	pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEnter)
	requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, ui).Submit()

	require.Len(t, repository.saves, 1)
	assert.Same(t, candidate, game.currentScene)
}

func TestEscapeMenuConfigureSupportsNoSound(t *testing.T) {
	scene := newEscapeMenuTestScene()
	scene.gameConfig.AudioConfiguration = &config.AudioConfiguration{}
	game := NewGame(scene)

	require.NotPanics(t, func() {
		activateEscapeMenuAction(t, scene, game, 3)
	})
	requireEscapeMenuDialog[*ConfigurationDialog](t, scene)
}

func TestEscapeMenuConfigurationReturnsToPausedEscapeMenu(t *testing.T) {
	scene := newEscapeMenuTestScene()
	manager, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	scene.audioManager = manager
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	game := NewGame(scene)

	activateEscapeMenuAction(t, scene, game, 3)
	require.Equal(t, 2, scene.dialogStack.Count())
	configurationDialog := requireEscapeMenuDialog[*ConfigurationDialog](t, scene)
	assert.Equal(t, audioSettingsGameplayTab, configurationDialog.GetActiveTab())
	pressEscapeMenuWidgetKey(scene, configurationDialog, ebiten.KeyEscape)

	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.False(t, manager.PlaybackSnapshot().Enabled)
	escapeMenu := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	pressEscapeMenuWidgetKey(scene, escapeMenu, ebiten.KeyEscape)
	assert.Zero(t, scene.dialogStack.Count())
	assert.True(t, manager.PlaybackSnapshot().Enabled)
}

func (scene *escapeMenuCaptureScene) CaptureSaveSnapshot() (save_snapshot.SaveSnapshot, error) {
	if scene.beforeCapture != nil {
		scene.beforeCapture()
	}

	return scene.playerSaveTestScene.CaptureSaveSnapshot()
}

func TestEscapeMenuAllowsImmediateFirstNavigation(t *testing.T) {
	scene := newEscapeMenuTestScene()
	scene.DoEscapeMenu(NewGame(scene))

	assert.True(t, scene.keyboard.TryToRegisterKeyPress(ebiten.KeyDown))
}

func TestEscapeMenuConfigureAcceptsFirstTabAfterOpeningKeyRelease(t *testing.T) {
	scene := newEscapeMenuTestScene()
	pressed := make(map[ebiten.Key]bool)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return pressed[key] })
	scene.DoEscapeMenu(NewGame(scene))
	menu := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	for range 3 {
		menu.GoDown()
	}
	pressed[ebiten.KeyEnter] = true
	menu.Update()
	dialog := requireEscapeMenuDialog[*ConfigurationDialog](t, scene)
	require.Equal(t, audioSettingsGameplayTab, dialog.GetActiveTab())

	dialog.Update()
	require.True(t, dialog.IsVisible(), "Held opening Enter must not apply or dismiss Configure")
	require.Equal(t, 2, scene.dialogStack.Count())
	pressed[ebiten.KeyEnter] = false
	scene.keyboard.ClearFrameConsumption()
	dialog.Update()

	for _, expectedTab := range []int{audioSettingsAudioTab, audioSettingsDisplayTab, audioSettingsGameplayTab} {
		pressed[ebiten.KeyTab] = true
		dialog.Update()
		require.Equal(t, expectedTab, dialog.GetActiveTab(), "Each fresh Tab must advance immediately")
		dialog.Update()
		require.Equal(t, expectedTab, dialog.GetActiveTab(), "Held Tab must not advance again")
		pressed[ebiten.KeyTab] = false
		scene.keyboard.ClearFrameConsumption()
		dialog.Update()
	}
	assert.Same(t, dialog, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, 2, scene.dialogStack.Count(), "Configure must remain above the Escape menu")
}

func TestEscapeMenuQuickSaveRemovesMenuAndFallsBackToWriteBrowser(t *testing.T) {
	t.Run("active slot", func(t *testing.T) {
		ui := newEscapeMenuTestScene()
		capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
			playthrough: playthroughMetadata{id: testPlaythroughID},
			snapshot:    validCommandTestSnapshot(),
			slot:        player_save.Slot07,
			revision:    "reviewed",
		}}
		capture.beforeCapture = func() {
			assert.Zero(t, ui.dialogStack.Count(), "quick save must remove the escape menu before capture")
		}
		repository := &playerSaveTestRepository{records: map[player_save.SlotID]player_save.Record{
			player_save.Slot07: playerSaveTestRecord(player_save.Slot07, "reviewed", capture.snapshot),
		}}
		game := NewGame(capture)
		game.ConfigurePlayerSaveRepository(repository, nil)

		activateEscapeMenuAction(t, ui, game, 0)

		assert.Zero(t, ui.dialogStack.Count())
		assert.Equal(t, 1, capture.captures)
		require.Len(t, repository.saves, 1)
		assert.Equal(t, "reviewed", repository.saves[0].expectedRevision)
		assert.Contains(t, ui.output.GetOutputStr(false), "Quick saved Slot 7.")
	})

	t.Run("no active slot", func(t *testing.T) {
		ui := newEscapeMenuTestScene()
		capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
			playthrough: playthroughMetadata{id: testPlaythroughID},
			snapshot:    validCommandTestSnapshot(),
		}}
		capture.beforeCapture = func() {
			assert.Zero(t, ui.dialogStack.Count(), "write browser must be removed before capture")
		}
		repository := &playerSaveTestRepository{
			records:   make(map[player_save.SlotID]player_save.Record),
			summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
		}
		game := NewGame(capture)
		game.ConfigurePlayerSaveRepository(repository, nil)

		activateEscapeMenuAction(t, ui, game, 0)

		assert.Zero(t, capture.captures)
		browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
		pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEnter)
		requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, ui).Submit()
		assert.Equal(t, 1, capture.captures)
		require.Len(t, repository.saves, 1)
		assert.Empty(t, repository.saves[0].expectedRevision)
	})
}

func TestEscapeMenuSaveUsesReviewedSummaryAndOverwriteConfirmation(t *testing.T) {
	for _, status := range []player_save.SlotStatus{player_save.SlotReady, player_save.SlotIncompatible} {
		t.Run(string(status), func(t *testing.T) {
			ui := newEscapeMenuTestScene()
			capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
				playthrough: playthroughMetadata{id: testPlaythroughID},
				snapshot:    validCommandTestSnapshot(),
			}}
			capture.beforeCapture = func() {
				assert.Zero(t, ui.dialogStack.Count(), "browser must be removed before capture")
			}
			repository := &playerSaveTestRepository{
				records: map[player_save.SlotID]player_save.Record{
					player_save.Slot02: playerSaveTestRecord(player_save.Slot02, "reviewed", capture.snapshot),
				},
				summaries: []player_save.SlotSummary{{
					Slot: player_save.Slot02, Status: status, Revision: "reviewed",
				}},
			}
			game := NewGame(capture)
			game.ConfigurePlayerSaveRepository(repository, nil)

			activateEscapeMenuAction(t, ui, game, 1)
			browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
			pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyDown)
			pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEnter)

			assert.Zero(t, capture.captures)
			assert.Equal(t, 2, ui.dialogStack.Count())
			confirmation := requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, ui)
			confirmation.Submit()

			assert.Equal(t, 1, capture.captures)
			require.Len(t, repository.saves, 1)
			assert.Equal(t, "reviewed", repository.saves[0].expectedRevision)
			assert.Zero(t, ui.dialogStack.Count())
		})
	}
}

func TestEscapeMenuSaveWritesEmptyDirectlyAndDisablesUnsafeSlots(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		ui := newEscapeMenuTestScene()
		capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
			snapshot: validCommandTestSnapshot(), playthrough: playthroughMetadata{id: testPlaythroughID},
		}}
		capture.beforeCapture = func() {
			assert.Zero(t, ui.dialogStack.Count())
		}
		repository := &playerSaveTestRepository{
			records:   make(map[player_save.SlotID]player_save.Record),
			summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
		}
		game := NewGame(capture)
		game.ConfigurePlayerSaveRepository(repository, nil)

		activateEscapeMenuAction(t, ui, game, 1)
		browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
		pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEnter)
		requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, ui).Submit()

		assert.Equal(t, 1, capture.captures)
		assert.Zero(t, ui.dialogStack.Count())
		assert.Contains(t, ui.output.GetOutputStr(false), "Saved Slot 1.")
	})

	t.Run("corrupt and unavailable", func(t *testing.T) {
		ui := newEscapeMenuTestScene()
		capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
			snapshot: validCommandTestSnapshot(), playthrough: playthroughMetadata{id: testPlaythroughID},
		}}
		repository := &playerSaveTestRepository{
			records: make(map[player_save.SlotID]player_save.Record),
			summaries: []player_save.SlotSummary{
				{Slot: player_save.Slot01, Status: player_save.SlotCorrupt, Problem: "corrupt"},
				{Slot: player_save.Slot02, Status: player_save.SlotUnavailable, Problem: "unavailable"},
			},
		}
		game := NewGame(capture)
		game.ConfigurePlayerSaveRepository(repository, nil)

		activateEscapeMenuAction(t, ui, game, 1)
		browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
		pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEnter)
		pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyDown)
		pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEnter)

		assert.Zero(t, capture.captures)
		assert.Empty(t, repository.saves)
		assert.Same(t, browser, ui.dialogStack.PeekTopModalDialog())
	})
}

func TestEscapeMenuSaveRemovesBrowserBeforeRealGameSceneCapture(t *testing.T) {
	for _, status := range []player_save.SlotStatus{player_save.SlotEmpty, player_save.SlotReady} {
		t.Run(string(status), func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(100, 100).
				WithSystemCallbacks().
				Build()
			scene := newEscapeMenuTestScene()
			scene.gameState = state
			repository := &playerSaveTestRepository{
				records: make(map[player_save.SlotID]player_save.Record),
				summaries: []player_save.SlotSummary{{
					Slot: player_save.Slot02, Status: status,
				}},
			}
			if status == player_save.SlotReady {
				repository.summaries[0].Revision = "reviewed"
				repository.records[player_save.Slot02] = playerSaveTestRecord(
					player_save.Slot02,
					"reviewed",
					validCommandTestSnapshot(),
				)
			}
			game := NewGame(scene)
			game.ConfigurePlayerSaveRepository(repository, nil)

			activateEscapeMenuAction(t, scene, game, 1)
			browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, scene)
			pressEscapeMenuWidgetKey(scene, browser, ebiten.KeyDown)
			pressEscapeMenuWidgetKey(scene, browser, ebiten.KeyEnter)
			requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, scene).Submit()

			require.Len(t, repository.saves, 1)
			assert.Equal(t, player_save.Slot02, repository.saves[0].slot)
			assert.Zero(t, scene.dialogStack.Count())
			assert.Contains(t, scene.output.GetOutputStr(false), "Saved Slot 2.")
		})
	}
}

func TestEscapeMenuSaveFailureReopensReviewedBrowserAfterCapture(t *testing.T) {
	ui := newEscapeMenuTestScene()
	capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
		playthrough: playthroughMetadata{id: testPlaythroughID},
		captureErr:  errors.New("capture failed"),
	}}
	capture.beforeCapture = func() {
		assert.Zero(t, ui.dialogStack.Count(), "failed capture must still run without the browser open")
	}
	repository := &playerSaveTestRepository{
		records:   make(map[player_save.SlotID]player_save.Record),
		summaries: []player_save.SlotSummary{{Slot: player_save.Slot02, Status: player_save.SlotEmpty}},
	}
	game := NewGame(capture)
	game.ConfigurePlayerSaveRepository(repository, nil)

	activateEscapeMenuAction(t, ui, game, 1)
	original := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
	pressEscapeMenuWidgetKey(ui, original, ebiten.KeyEnter)
	requireEscapeMenuDialog[*widgets.SaveDescriptionEntryModal](t, ui).Submit()

	reopened := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
	assert.NotSame(t, original, reopened)
	assert.Equal(t, 1, capture.captures)
	assert.Empty(t, repository.saves)
}

func TestPlayerSaveErrorDetailHidesDiagnosticContext(t *testing.T) {
	sentinel := ErrGameSceneSnapshotNotRepresentable
	detailed := fmt.Errorf("active event queue work: %w", sentinel)
	wrapped := fmt.Errorf("capturing player save: %w", detailed)

	assert.Equal(t, "You cannot save just now. Finish what you are doing and try again.", playerSaveErrorDetail(wrapped))
	assert.Equal(t, "Please try again. If the problem continues, check the game log for details.", playerSaveErrorDetail(errors.New("plain failure")))
}

func TestEscapeMenuLoadUsesReviewedRevisionAndReportsOnReplacementScene(t *testing.T) {
	t.Run("stale reviewed revision", func(t *testing.T) {
		oldScene := newEscapeMenuTestScene()
		oldScene.gameConfig = &config.UltimaVConfiguration{}
		repository := &playerSaveTestRepository{
			records: map[player_save.SlotID]player_save.Record{
				player_save.Slot03: playerSaveTestRecord(player_save.Slot03, "current", validCommandTestSnapshot()),
			},
			summaries: []player_save.SlotSummary{{
				Slot: player_save.Slot03, Status: player_save.SlotReady, Revision: "reviewed",
			}},
		}
		builds := 0
		game := NewGame(oldScene)
		game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
			builds++

			return newEscapeMenuTestScene(), nil
		})

		activateEscapeMenuAction(t, oldScene, game, 2)
		browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, oldScene)
		pressEscapeMenuWidgetKey(oldScene, browser, ebiten.KeyEnter)

		assert.Same(t, oldScene, game.currentScene)
		assert.Zero(t, builds)
		assert.False(t, oldScene.disposed)
		assert.Equal(t, 1, oldScene.dialogStack.Count())
		assert.IsType(t, &widgets.SaveSlotBrowser{}, oldScene.dialogStack.PeekTopModalDialog())
		assert.NotContains(t, oldScene.output.GetOutputStr(false), "Load failed:")
	})

	t.Run("success", func(t *testing.T) {
		oldScene := newEscapeMenuTestScene()
		oldScene.gameConfig = &config.UltimaVConfiguration{}
		loadedScene := newEscapeMenuTestScene()
		loadedScene.gameConfig = &config.UltimaVConfiguration{}
		record := playerSaveTestRecord(player_save.Slot04, "reviewed", validCommandTestSnapshot())
		repository := &playerSaveTestRepository{
			records: map[player_save.SlotID]player_save.Record{player_save.Slot04: record},
			summaries: []player_save.SlotSummary{{
				Slot: player_save.Slot04, Status: player_save.SlotReady, Revision: "reviewed",
			}},
		}
		game := NewGame(oldScene)
		game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
			return loadedScene, nil
		})

		activateEscapeMenuAction(t, oldScene, game, 2)
		browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, oldScene)
		pressEscapeMenuWidgetKey(oldScene, browser, ebiten.KeyEnter)

		assert.Same(t, loadedScene, game.currentScene)
		assert.True(t, oldScene.disposed)
		assert.NotContains(t, oldScene.output.GetOutputStr(false), "Loaded")
		assert.Contains(t, loadedScene.output.GetOutputStr(false), "Loaded Slot 4.")
		assert.Equal(t, player_save.Slot04, loadedScene.activePlayerSaveSlot)
		assert.Equal(t, "reviewed", loadedScene.activePlayerSaveRevision)
	})
}

func TestEscapeMenuCancelAndFailureResumeAudio(t *testing.T) {
	for _, test := range []struct {
		name       string
		configure  bool
		cancelSave bool
	}{
		{name: "cancel", configure: true, cancelSave: true},
		{name: "failure"},
	} {
		t.Run(test.name, func(t *testing.T) {
			ui := newEscapeMenuTestScene()
			manager, err := audio.NewGameAudioManagerWithConfig(nil)
			require.NoError(t, err)
			ui.audioManager = manager
			t.Cleanup(func() { require.NoError(t, manager.Close()) })
			capture := &escapeMenuCaptureScene{playerSaveTestScene: playerSaveTestScene{
				snapshot: validCommandTestSnapshot(), playthrough: playthroughMetadata{id: testPlaythroughID},
			}}
			game := NewGame(capture)
			if test.configure {
				game.ConfigurePlayerSaveRepository(&playerSaveTestRepository{
					records:   make(map[player_save.SlotID]player_save.Record),
					summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
				}, nil)
			}

			activateEscapeMenuAction(t, ui, game, 1)
			if test.cancelSave {
				assert.False(t, manager.PlaybackSnapshot().Enabled)
				browser := requireEscapeMenuDialog[*widgets.SaveSlotBrowser](t, ui)
				pressEscapeMenuWidgetKey(ui, browser, ebiten.KeyEscape)
			}

			assert.True(t, manager.PlaybackSnapshot().Enabled)
			assert.Zero(t, ui.dialogStack.Count())
		})
	}
}

func newEscapeMenuTestScene() *GameScene {
	return &GameScene{
		activePlaythrough: playthroughMetadata{id: testPlaythroughID},
		gameConfig: &config.UltimaVConfiguration{
			AudioConfiguration: &config.AudioConfiguration{},
			DisplayConfiguration: &config.DisplayConfiguration{
				Mode:              config.ModeWindowed,
				WindowWidth:       config.MinimumWindowWidth,
				WindowHeight:      config.MinimumWindowHeight,
				TargetFPS:         30,
				EnableFpsLimiting: true,
			},
			GameplayConfiguration: config.DefaultGameplayConfiguration(),
		},
		gameState: &game_state.GameState{},
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 0, 20, 120),
	}
}

func activateEscapeMenuAction(t *testing.T, scene *GameScene, game *Game, buttonIndex int) {
	t.Helper()
	scene.DoEscapeMenu(game)
	menu := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	for range buttonIndex {
		menu.GoDown()
	}
	pressEscapeMenuWidgetKey(scene, menu, ebiten.KeyEnter)
}

func requireEscapeMenuDialog[T widgets.Widget](t *testing.T, scene *GameScene) T {
	t.Helper()
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(T)
	require.True(t, ok, "unexpected top modal %T", scene.dialogStack.PeekTopModalDialog())

	return dialog
}

func pressEscapeMenuWidgetKey(scene *GameScene, widget widgets.Widget, key ebiten.Key) {
	scene.keyboard.SimulateKeyPress(key)
	widget.Update()
}
