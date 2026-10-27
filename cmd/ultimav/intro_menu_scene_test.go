package main

import (
	"errors"
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type introMenuTestDialog struct {
	updates int
}

func (*introMenuTestDialog) Draw(*ebiten.Image) {}
func (dialog *introMenuTestDialog) Update()     { dialog.updates++ }

type introMenuObservingRepository struct {
	*playerSaveTestRepository
	scene            *IntroMenuScene
	loadDialogCounts []int
	saveDialogCounts []int
}

func TestCharacterCreationIdentityPresentationContinuesLogicalFlame(t *testing.T) {
	logo := ebiten.NewImage(1, 1)
	t.Cleanup(logo.Deallocate)
	logo.Fill(ucolor.EGABlue)
	firstFlame := ebiten.NewImage(1, 1)
	t.Cleanup(firstFlame.Deallocate)
	firstFlame.Fill(ucolor.EGARed)
	secondFlame := ebiten.NewImage(1, 1)
	t.Cleanup(secondFlame.Deallocate)
	secondFlame.Fill(ucolor.EGACyan)
	introSprites := &sprites.IntroSprites{
		OriginFrames:     nil,
		Presents:         nil,
		ArticleA:         nil,
		Production:       nil,
		BritishSignature: nil,
		BritishPaths:     nil,
		Ultima16Logo:     logo,
		FlameFizzBase:    nil,
		ReduxLogo:        nil,
		FlameAnimation:   sprites.NewImageAnimation([]*ebiten.Image{firstFlame, secondFlame}, 100),
	}

	presentation := newCharacterCreationIdentityPresentation(introSprites, 50)
	canvas := ebiten.NewImage(introLogicalWidth, introLogicalHeight)
	t.Cleanup(canvas.Deallocate)
	presentation.draw(canvas)

	assertCharacterCreationPixel(t, canvas, 0, 0, ucolor.EGABlue)
	assertCharacterCreationPixel(t, canvas, introFlameX, introFlameY, ucolor.EGARed)
	for range 4 {
		presentation.advance()
	}
	canvas.Fill(ucolor.EGABlack)
	presentation.draw(canvas)
	assertCharacterCreationPixel(t, canvas, introFlameX, introFlameY, ucolor.EGACyan)
	assertCharacterCreationPixel(t, canvas, 319, 199, ucolor.EGABlack)
}

func (repository *introMenuObservingRepository) Load(slot player_save.SlotID) (player_save.Record, error) {
	repository.loadDialogCounts = append(repository.loadDialogCounts, repository.scene.dialogStack.Count())

	return repository.playerSaveTestRepository.Load(slot)
}

func (repository *introMenuObservingRepository) Save(
	slot player_save.SlotID,
	envelope save_snapshot.Envelope,
	expectedRevision string,
) (player_save.Record, error) {
	repository.saveDialogCounts = append(repository.saveDialogCounts, repository.scene.dialogStack.Count())

	return repository.playerSaveTestRepository.Save(slot, envelope, expectedRevision)
}

func TestIntroMenuRepeatedDownPressesDoNotBecomeStuck(t *testing.T) {
	scene := newIntroInputTestScene()

	scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
	require.NoError(t, scene.Update(nil))
	require.NoError(t, scene.Update(nil))
	scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
	require.NoError(t, scene.Update(nil))

	assert.Equal(t, 2, scene.nCurrentSelection)
}

func TestIntroMenuNavigationClampsAtEdges(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	require.NoError(t, scene.Update(nil))
	assert.Zero(t, scene.nCurrentSelection)

	scene.nCurrentSelection = len(text.IntroChoices) - 1
	require.NoError(t, scene.Update(nil))
	scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, len(text.IntroChoices)-1, scene.nCurrentSelection)
}

func TestIntroMenuVersionHiddenUntilVToggles(t *testing.T) {
	scene := newIntroInputTestScene()
	assert.False(t, scene.showVersion, "the version is a debug aid, hidden by default")
	for _, want := range []bool{true, false} {
		scene.keyboard.SimulateKeyPress(ebiten.KeyV)
		require.NoError(t, scene.Update(nil))
		require.NoError(t, scene.Update(nil))
		assert.Equal(t, want, scene.showVersion)
	}
	assert.Zero(t, scene.nCurrentSelection, "V does not move or activate the selection")
}

func TestIntroMenuVersionDrawsInsideLowerBorderBand(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	retirePresentationFixtureCaches(t)
	for _, height := range []int{720, 1080} {
		t.Run(fmt.Sprint(height), func(t *testing.T) {
			width := height * 16 / 9
			manager.PublishLogicalSize(width, height)
			font := text.NewUltimaFont(text.GetIntroFontSize())
			render := func(show bool) *ebiten.Image {
				scene := &IntroMenuScene{
					introSprites: &sprites.IntroSprites{
						Ultima16Logo:   ebiten.NewImage(1, 1),
						ReduxLogo:      ebiten.NewImage(1, 1),
						FlameAnimation: sprites.NewImageAnimation([]*ebiten.Image{ebiten.NewImage(1, 1)}, 100),
					},
					presentationCanvas: ebiten.NewImage(introLogicalWidth, introLogicalHeight),
					borderSprites:      sprites.NewBorderSprites(),
					ultimaFont:         font,
					versionOutput:      newIntroVersionOutput(),
					showVersion:        show,
				}
				screen := newPresentationRaster(width, height)
				scene.drawMenu(screen)

				return screen
			}
			hidden, shown := render(false), render(true)
			scale := introMenuBorderScale(width)
			bandBottom := float64(height) * text.IntroMenuFramePlacement().EndPercentY
			bandTop := bandBottom - introMenuBorderBandReferencePixels*scale
			changed := 0
			for y := range height {
				for x := range width {
					if hidden.At(x, y) == shown.At(x, y) {
						continue
					}
					changed++
					require.GreaterOrEqual(t, float64(y), bandTop, "version pixel above the band at %d,%d", x, y)
					require.Less(t, float64(y), bandBottom, "version pixel below the band at %d,%d", x, y)
					require.Greater(t, x, width/2, "version is right-aligned")
				}
			}
			assert.Positive(t, changed, "the shown version draws pixels")
		})
	}
}

func TestIntroMenuEscapeAsksBeforeExiting(t *testing.T) {
	scene := newIntroInputTestScene()
	game := NewGame(scene)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))
	prompt, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)
	assert.Equal(t, []string{"Exit", "Cancel"}, prompt.ButtonTexts())

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))
	assert.Zero(t, scene.dialogStack.Count(), "Escape on the prompt cancels")
	assert.False(t, game.exitRequested)

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(game))
	assert.True(t, game.exitRequested)
}

func TestIntroMenuSetDataDirectoryStartsGuidedSetup(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	scene := newIntroInputTestScene()
	game := NewGame(scene)
	openIntroSubmenu(scene, introChoiceConfigure, introConfigureData)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(game))

	setup, ok := game.currentScene.(*dataSetupScene)
	require.True(t, ok)
	assert.Equal(t, dataSetupFromTitle, setup.mode)
	assert.Equal(t, scene.config, setup.configuration)
	setup.Dispose()
}

func TestIntroMenuConfigureOpensDisplayAndReturnsToMenu(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.nCurrentSelection = introChoiceConfigure
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(nil))
	assert.True(t, scene.submenuOpen, "Enter on Configure opens its sub-menu first")
	assert.Equal(t, introConfigureDisplay, scene.submenuSelection)
	require.Zero(t, scene.dialogStack.Count())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(nil))
	require.Equal(t, 1, scene.dialogStack.Count())
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*ConfigurationDialog)
	require.True(t, ok)
	assert.Equal(t, audioSettingsDisplayTab, dialog.GetActiveTab())
	assert.Equal(t, 0.0, dialog.placement.StartPercentX)
	assert.Equal(t, 1.0, dialog.placement.EndPercentX)
	dialog.ensureLayout(1280, 720)
	assert.InDelta(t, 640, dialog.layout.cardRect.Min.X+dialog.layout.cardRect.Dx()/2, 1)

	scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, introChoiceConfigure, scene.nCurrentSelection, "modal input must not move the title selection")

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(nil))
	assert.Zero(t, scene.dialogStack.Count())
	assert.True(t, scene.menuVisible)
	assert.True(t, scene.submenuOpen, "closing the dialog returns to the open Configure sub-menu")

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(nil))
	assert.False(t, scene.submenuOpen, "Escape closes the sub-menu before offering to exit")
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, introChoiceConfigure, scene.nCurrentSelection)
}

func TestIntroMenuConfigureAudioOpensSoundTab(t *testing.T) {
	scene := newIntroInputTestScene()
	openIntroSubmenu(scene, introChoiceConfigure, introConfigureAudio)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(nil))

	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*ConfigurationDialog)
	require.True(t, ok)
	assert.Equal(t, audioSettingsAudioTab, dialog.GetActiveTab())
}

func TestIntroMenuSubmenuNavigationClampsAndClosesWithLeft(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.nCurrentSelection = introChoiceConfigure
	scene.keyboard.SimulateKeyPress(ebiten.KeyRight)
	require.NoError(t, scene.Update(nil))
	require.True(t, scene.submenuOpen, "Right opens the selected sub-menu")

	for range 5 {
		scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
		require.NoError(t, scene.Update(nil))
	}
	assert.Equal(t, introConfigureData, scene.submenuSelection, "sub-menu rows clamp at the last child")
	assert.Equal(t, introChoiceConfigure, scene.nCurrentSelection, "the parent does not move while open")
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, introConfigureAudio, scene.submenuSelection)

	scene.keyboard.SimulateKeyPress(ebiten.KeyLeft)
	require.NoError(t, scene.Update(nil))
	assert.False(t, scene.submenuOpen, "Left closes the sub-menu")
	assert.Zero(t, scene.submenuSelection)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestIntroMenuRightOnLeafDoesNothing(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.nCurrentSelection = introChoiceLoad
	scene.keyboard.SimulateKeyPress(ebiten.KeyRight)

	require.NoError(t, scene.Update(nil))

	assert.False(t, scene.submenuOpen)
	assert.Equal(t, introChoiceLoad, scene.nCurrentSelection)
}

func TestIntroMenuNewGameImportStartsLegacyImport(t *testing.T) {
	scene := newIntroInputTestScene()
	game := NewGame(scene)
	openIntroSubmenu(scene, introChoiceNewGame, introNewGameImport)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(game))

	require.Equal(t, 1, scene.dialogStack.Count(), "an unset data folder reports the missing original save")
	_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	assert.True(t, ok)
	assert.True(t, scene.submenuOpen, "the New Game sub-menu stays open behind the report")
}

func TestIntroMenuConfigureRestoresTitleAudioOwnership(t *testing.T) {
	scene := newIntroInputTestScene()
	manager, err := audio.NewGameAudioManagerWithConfig(nil)
	require.NoError(t, err)
	scene.audioManager = manager
	t.Cleanup(func() { require.NoError(t, manager.Close()) })

	scene.openConfigurationDialogForTab(audioSettingsDisplayTab)
	assert.False(t, manager.PlaybackSnapshot().Enabled)
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*ConfigurationDialog)
	require.True(t, ok)

	dialog.onCancelClicked()

	assert.True(t, manager.PlaybackSnapshot().Enabled)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestIntroMenuNarrativeLoadFailureReturnsRecoverableFeedback(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.config.SavedConfigData.DataFilePath = t.TempDir()
	scene.nCurrentSelection = introChoiceNarrative
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(nil))

	assert.True(t, scene.menuVisible)
	assert.Nil(t, scene.narrative)
	assert.Equal(t, "Introduction could not be loaded.", scene.notice)
}

func TestIntroMenuReturnToViewChoiceRunsUntilAnyKey(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.returnViewData = fakeReturnViewData(t)
	scene.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return scene.elapsedMs }, nil)
	scene.nCurrentSelection = introChoiceReturnView
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(nil))
	require.NotNil(t, scene.returnView)
	assert.False(t, scene.menuVisible)
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	require.NoError(t, scene.Update(nil))

	assert.True(t, scene.menuVisible)
	assert.Nil(t, scene.returnView)
	assert.False(t, scene.eventQueueManager.HasActiveWorkflows())
}

func TestIntroMenuIdleStartsReturnToView(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.returnViewData = fakeReturnViewData(t)
	scene.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return scene.elapsedMs }, nil)
	scene.elapsedMs = 10_000

	require.NoError(t, scene.Update(nil))

	assert.False(t, scene.menuVisible)
	assert.NotNil(t, scene.returnView)
}

func TestIntroMenuUnboundKeyResetsReturnToViewIdleTimer(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.returnViewData = fakeReturnViewData(t)
	scene.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return scene.elapsedMs }, nil)
	scene.elapsedMs = 10_000
	scene.keyboard.SimulateKeyPress(ebiten.KeyF1)

	require.NoError(t, scene.Update(nil))

	assert.True(t, scene.menuVisible)
	assert.Nil(t, scene.returnView)
}

func TestIntroMenuUninterruptedStartupRunsReturnToView(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.menuVisible = false
	scene.returnViewData = fakeReturnViewData(t)
	scene.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return scene.elapsedMs }, nil)

	scene.finishStartupPresentation()

	assert.False(t, scene.menuVisible)
	assert.NotNil(t, scene.returnView)
}

func TestIntroMenuSkippedStartupGoesStraightToMenu(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.menuVisible = false
	scene.startupSkipped = true

	scene.finishStartupPresentation()

	assert.True(t, scene.menuVisible)
	assert.Nil(t, scene.returnView)
}

func TestIntroMenuFailedJourneyOnwardPreservesIntro(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.config.SavedConfigData = &config.UltimaVConfigurationFlags{DataFilePath: t.TempDir()}
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(&playerSaveTestRepository{
		summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotReady}},
		latestErr: errors.New("load failed"),
	}, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{}, nil
	})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, game.Update())

	assert.Same(t, scene, game.currentScene)
	assert.Equal(t, "Load failed. Please try again. If the problem continues, check the game log for details.", scene.notice)
}

func TestIntroMenuModalInputPreemptsMenuNavigation(t *testing.T) {
	scene := newIntroInputTestScene()
	dialog := new(introMenuTestDialog)
	scene.dialogStack.PushModalDialog(dialog)
	scene.keyboard.SimulateKeyPress(ebiten.KeyDown)

	require.NoError(t, scene.Update(nil))

	assert.Equal(t, 1, dialog.updates)
	assert.Zero(t, scene.nCurrentSelection)
}

func TestIntroMenuJourneyOnwardLoadsLatestNativeSave(t *testing.T) {
	snapshot := validCommandTestSnapshot()
	record := playerSaveTestRecord(player_save.Slot04, "latest", snapshot)
	repository := &playerSaveTestRepository{
		latest: record,
		summaries: []player_save.SlotSummary{{
			Slot: player_save.Slot04, Status: player_save.SlotReady, Revision: "latest",
		}},
	}
	scene := newIntroInputTestScene()
	loadedScene := &playerSaveTestScene{}
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		loadedScene.snapshot = canonicalPlayerSaveTestSnapshotFrom(snapshot)

		return loadedScene, nil
	})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, game.Update())

	assert.Same(t, loadedScene, game.currentScene)
	assert.Equal(t, player_save.Slot04, loadedScene.slot)
	assert.Equal(t, "latest", loadedScene.revision)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestIntroMenuJourneyOnwardIsDisabledWhenNoReadySaveExists(t *testing.T) {
	scene := newIntroInputTestScene()
	repository := &playerSaveTestRepository{
		latestErr: &player_save.NotFoundError{},
		summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
	}
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{}, nil
	})
	assert.False(t, scene.journeyOnwardEnabled)
	assert.Equal(t, introChoiceLoad, scene.nCurrentSelection)
	scene.nCurrentSelection = introChoiceJourney
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, game.Update())

	assert.Same(t, scene, game.currentScene)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Empty(t, scene.notice)
	assert.Zero(t, repository.latestCalls)
}

func TestIntroMenuReadyCatalogEnablesAndSelectsJourneyOnward(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.nCurrentSelection = introChoiceLoad
	repository := &playerSaveTestRepository{summaries: []player_save.SlotSummary{{
		Slot: player_save.Slot03, Status: player_save.SlotReady,
	}}}
	game := NewGame(scene)

	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{}, nil
	})

	assert.True(t, scene.journeyOnwardEnabled)
	assert.Equal(t, introChoiceJourney, scene.nCurrentSelection)
	assert.Equal(t, 1, repository.listCalls)
	for range 3 {
		require.NoError(t, game.Update())
	}
	assert.Equal(t, 1, repository.listCalls)
}

func TestIntroMenuNavigationSkipsDisabledJourneyOnward(t *testing.T) {
	scene := newIntroInputTestScene()
	scene.nCurrentSelection = introChoiceLoad
	scene.applyPlayerSaveCatalog([]player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}})
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)

	require.NoError(t, scene.Update(nil))

	assert.Equal(t, introChoiceLoad, scene.nCurrentSelection)
}

func TestIntroMenuLoadBrowserUsesReviewedRevisionAfterRemovingDialog(t *testing.T) {
	snapshot := validCommandTestSnapshot()
	record := playerSaveTestRecord(player_save.Slot03, "current", snapshot)
	scene := newIntroInputTestScene()
	repository := &introMenuObservingRepository{
		playerSaveTestRepository: &playerSaveTestRepository{
			records: map[player_save.SlotID]player_save.Record{player_save.Slot03: record},
			summaries: []player_save.SlotSummary{{
				Slot: player_save.Slot03, Status: player_save.SlotReady, Revision: "reviewed",
			}},
		},
		scene: scene,
	}
	builds := 0
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		builds++

		return &playerSaveTestScene{}, nil
	})
	openIntroSaveBrowser(t, game, scene, introChoiceLoad)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, game.Update())

	assert.Same(t, scene, game.currentScene)
	assert.Equal(t, []int{1}, repository.loadDialogCounts)
	assert.Zero(t, builds)
	assert.Empty(t, scene.notice)
	assert.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
}

func TestIntroMenuLegacyImportIntoEmptySlotRemovesDialogsAndUsesEmptyRevision(t *testing.T) {
	legacyPath := writePlayerSaveLegacyBundle(t, false)
	scene := newIntroInputTestScene()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	startup := config.LoadStartupConfiguration()
	require.NotNil(t, startup.Configuration)
	scene.config = startup.Configuration
	settingsPath, err := scene.config.ConfigurationFilePath()
	require.NoError(t, err)
	settingsBefore, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	repository := &introMenuObservingRepository{
		playerSaveTestRepository: &playerSaveTestRepository{
			records:   make(map[player_save.SlotID]player_save.Record),
			summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
		},
		scene: scene,
	}
	loadedScene := &playerSaveTestScene{}
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
		loadedScene.snapshot = canonicalPlayerSaveTestSnapshotFrom(snapshot)

		return loadedScene, nil
	})
	openIntroImportBrowser(t, game, scene, legacyPath)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, game.Update())
	editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
	require.True(t, ok)
	editor.Submit()

	require.Len(t, repository.saves, 1)
	assert.Empty(t, repository.saves[0].expectedRevision)
	assert.Equal(t, []int{0}, repository.saveDialogCounts)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Same(t, loadedScene, game.currentScene)
	settingsAfter, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Equal(t, settingsBefore, settingsAfter, "successful imports must not persist source history")
}

func TestIntroMenuLegacyImportConfirmsOverwriteWithReviewedRevision(t *testing.T) {
	for _, status := range []player_save.SlotStatus{player_save.SlotReady, player_save.SlotIncompatible} {
		t.Run(string(status), func(t *testing.T) {
			legacyPath := writePlayerSaveLegacyBundle(t, false)
			snapshot := validCommandTestSnapshot()
			existing := playerSaveTestRecord(player_save.Slot02, "reviewed", snapshot)
			scene := newIntroInputTestScene()
			repository := &introMenuObservingRepository{
				playerSaveTestRepository: &playerSaveTestRepository{
					records: map[player_save.SlotID]player_save.Record{player_save.Slot02: existing},
					summaries: []player_save.SlotSummary{{
						Slot: player_save.Slot02, Status: status, Revision: "reviewed",
					}},
				},
				scene: scene,
			}
			game := NewGame(scene)
			game.ConfigurePlayerSaveRepository(repository, func(_ *config.UltimaVConfiguration, snapshot save_snapshot.SaveSnapshot) (Scene, error) {
				return &playerSaveTestScene{snapshot: canonicalPlayerSaveTestSnapshotFrom(snapshot)}, nil
			})
			openIntroImportBrowser(t, game, scene, legacyPath)
			scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
			require.NoError(t, game.Update())
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			require.NoError(t, game.Update())

			assert.IsType(t, &widgets.SaveDescriptionEntryModal{}, scene.dialogStack.PeekTopModalDialog())
			assert.Equal(t, 2, scene.dialogStack.Count())
			assert.Empty(t, repository.saves)

			scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal).Submit()

			require.Len(t, repository.saves, 1)
			assert.Equal(t, "reviewed", repository.saves[0].expectedRevision)
			assert.Equal(t, []int{0}, repository.saveDialogCounts)
			assert.Zero(t, scene.dialogStack.Count())
		})
	}
}

func TestIntroMenuLegacyImportUnsafeDestinationsRemainDisabled(t *testing.T) {
	for _, status := range []player_save.SlotStatus{player_save.SlotCorrupt, player_save.SlotUnavailable} {
		t.Run(string(status), func(t *testing.T) {
			legacyPath := writePlayerSaveLegacyBundle(t, false)
			scene := newIntroInputTestScene()
			repository := &playerSaveTestRepository{
				records: make(map[player_save.SlotID]player_save.Record),
				summaries: []player_save.SlotSummary{{
					Slot: player_save.Slot06, Status: status, Problem: "cannot use this slot",
				}},
			}
			game := NewGame(scene)
			game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
				return &playerSaveTestScene{}, nil
			})
			openIntroImportBrowser(t, game, scene, legacyPath)
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

			require.NoError(t, game.Update())

			assert.IsType(t, &widgets.SaveSlotBrowser{}, scene.dialogStack.PeekTopModalDialog())
			assert.Equal(t, 1, scene.dialogStack.Count())
			assert.Empty(t, repository.saves)
		})
	}
}

func TestIntroMenuLegacyImportFailureKeepsSceneActiveAndReportsError(t *testing.T) {
	scene := newIntroInputTestScene()
	repository := &playerSaveTestRepository{
		records:   make(map[player_save.SlotID]player_save.Record),
		summaries: []player_save.SlotSummary{{Slot: player_save.Slot01, Status: player_save.SlotEmpty}},
	}
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{}, nil
	})
	scene.config.SavedConfigData.DataFilePath = t.TempDir()
	scene.startActiveLegacySaveImport(game)
	require.Eventually(t, func() bool {
		require.NoError(t, game.Update())
		_, pending := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)

		return pending && !scene.legacyPrepareActive
	}, time.Second, time.Millisecond)
	resultDialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
	require.True(t, ok)

	assert.Same(t, scene, game.currentScene)
	assert.True(t, scene.active)
	assert.Empty(t, scene.notice)
	assert.Equal(t, []string{"Check Again", "Back"}, resultDialog.ButtonTexts())
	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.Same(t, resultDialog, scene.dialogStack.PeekTopModalDialog())
}

func TestIntroMenuLegacyImportCompatibilityFailureRequiresAcknowledgementBeforeRetry(t *testing.T) {
	snapshot := validCommandTestSnapshot()
	snapshot.World.Position.Location = int(references.BritanniaUnderworld)
	snapshot.World.LastLargeMapPosition = snapshot.World.Position
	snapshot.Wind.NextTickMs = 55
	chest := 0
	snapshot.MapUnits = []save_snapshot.MapUnitSnapshot{{
		Kind: save_snapshot.MapUnitKindChest, World: -1, Position: snapshot.World.Position,
		SurfaceChestNumber: &chest, AIOverrideProvenance: string(references.AIOverrideProvenanceNone),
	}}
	compatibilityErr := game_state.ValidateSaveSnapshotCompatibility(snapshot, &references.GameReferences{
		LocationReferences: &references.LocationReferences{}, EnemyReferences: &references.EnemyReferences{},
		TileReferences: references.NewTileReferences(),
	})
	const cause = "mapUnits[0]: large-map controller world must match the unit floor"
	require.ErrorContains(t, compatibilityErr, cause)
	scene := newIntroInputTestScene()
	pressed := make(map[ebiten.Key]bool)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return pressed[key] })
	repository := &playerSaveTestRepository{
		summaries: []player_save.SlotSummary{
			{Slot: player_save.Slot01, Status: player_save.SlotEmpty},
			{Slot: player_save.Slot07, Status: player_save.SlotEmpty},
		},
		compatibilityErr: compatibilityErr,
	}
	game := NewGame(scene)
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		t.Fatal("incompatible import must not build a scene")
		return nil, nil
	})
	prepared := &preparedLegacyPlayerSave{snapshot: snapshot, playthrough: playthroughMetadata{id: testPlaythroughID}}
	scene.openPlayerSaveBrowserWithState(game, playerSaveBrowserWrite, prepared, string(player_save.Slot07), "")
	for attempt, acknowledgeKey := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
		browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
		require.True(t, ok)
		view, selected := browser.GetSelectedView()
		require.True(t, selected)
		assert.Equal(t, string(player_save.Slot07), view.ID)
		pressed[ebiten.KeyEnter] = true
		scene.keyboard.SetAllowKeyPressImmediately()
		require.NoError(t, game.Update())
		editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
		require.True(t, ok)
		if attempt == 0 {
			editor.SetText("Imported checkpoint")
		} else {
			assert.Equal(t, "Imported checkpoint", editor.GetText())
		}
		pressed[ebiten.KeyEnter] = false
		require.NoError(t, game.Update())
		pressed[ebiten.KeyEnter] = true
		scene.keyboard.SetAllowKeyPressImmediately()
		require.NoError(t, game.Update())
		editor.Submit()
		editor.Submit() // A completed editor cannot commit twice.
		overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
		require.True(t, ok)
		assert.True(t, overlay.IsVisible())
		assert.True(t, overlay.IsUserDismissed())
		assert.Contains(t, overlay.GetMessage(), "Import to Slot 7 failed")
		assert.Contains(t, overlay.GetMessage(), cause)
		assert.Equal(t, strings.Fields(overlay.GetMessage()), strings.Fields(overlay.GetContentText()))
		assert.Equal(t, 2, scene.dialogStack.Count())
		for range 2 {
			require.NoError(t, game.Update())
		}
		assert.True(t, scene.keyboard.IsWaitingForRelease())
		assert.Same(t, overlay, scene.dialogStack.PeekTopModalDialog(), "held submit must not acknowledge the error")
		pressed[ebiten.KeyEnter] = false
		require.NoError(t, game.Update())
		pressed[acknowledgeKey] = true
		scene.keyboard.SetAllowKeyPressImmediately()
		require.NoError(t, game.Update())
		refreshed, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
		require.True(t, ok)
		assert.NotSame(t, browser, refreshed)
		view, selected = refreshed.GetSelectedView()
		require.True(t, selected)
		assert.Equal(t, string(player_save.Slot07), view.ID)
		for range 2 {
			require.NoError(t, game.Update())
		}
		// Acknowledgement is consumed until physical release. The browser can
		// remain protected by key consumption without the separate transition-wait
		// flag; assert the actual no-retry/no-cancel contract below.
		assert.Same(t, refreshed, scene.dialogStack.PeekTopModalDialog(), "held acknowledgement must not retry or cancel")
		assert.Equal(t, 1, scene.dialogStack.Count())
		assert.Equal(t, attempt+1, repository.compatibilityChecks)
		assert.Empty(t, repository.saves)
		assert.Equal(t, "Imported checkpoint", prepared.draft.text)
		assert.Same(t, scene, game.currentScene)
		pressed[acknowledgeKey] = false
		require.NoError(t, game.Update())
	}
}

func TestIntroMenuLegacyImportRollbackFailureRetainsFullErrorAtSupportedSizes(t *testing.T) {
	setTalkHistoryTestResolution(t, 1280, 720)
	scene := newIntroInputTestScene()
	game := NewGame(scene)
	repository := &playerSaveTestRepository{
		summaries: []player_save.SlotSummary{{Slot: player_save.Slot07, Status: player_save.SlotEmpty}},
		deleteErr: errors.New("removing /Users/player/Ultima V Redux/player-saves/slot-07.json: permission denied"),
	}
	snapshot := canonicalPlayerSaveTestSnapshot()
	game.ConfigurePlayerSaveRepository(repository, func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
		return &playerSaveTestScene{snapshot: snapshot, activatePanic: true}, nil
	})
	prepared := &preparedLegacyPlayerSave{snapshot: snapshot, playthrough: playthroughMetadata{id: testPlaythroughID}}
	scene.openPlayerSaveBrowser(game, playerSaveBrowserWrite, prepared)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(game))
	editor, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveDescriptionEntryModal)
	require.True(t, ok)
	editor.Submit()
	overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
	require.True(t, ok)
	assert.Contains(t, overlay.GetMessage(), "activating prepared player save slot-07 scene")
	assert.Contains(t, overlay.GetMessage(), "rolling back prepared player save slot-07")
	assert.Contains(t, overlay.GetMessage(), repository.deleteErr.Error())
	assert.NotContains(t, overlay.GetMessage(), "unchanged")
	assert.Len(t, repository.saves, 1)
	assert.Len(t, repository.deletes, 1)
	elapsed := scene.elapsedMs
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setTalkHistoryTestResolution(t, size.X, size.Y)
			overlay.PrepareLayout(image.Rect(0, 0, size.X, size.Y))
			assert.Equal(t, strings.Fields(overlay.GetMessage()), strings.Fields(overlay.GetContentText()), "wrapped output must retain the destination and every cause")
			assert.Greater(t, strings.Count(overlay.GetContentText(), "\n"), strings.Count(overlay.GetMessage(), "\n"))
			assert.Len(t, repository.saves, 1)
			assert.Len(t, repository.deletes, 1)
			assert.Equal(t, elapsed, scene.elapsedMs, "layout preparation must not advance the scene")
			assert.Same(t, overlay, scene.dialogStack.PeekTopModalDialog())
			assert.True(t, overlay.IsVisible())
			assert.False(t, overlay.IsDismissed())
		})
	}
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(game))
	assert.Equal(t, 1, scene.dialogStack.Count())
}

func TestLegacySavePreparationMessageReportsTypedCause(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{
			name: "invalid filename",
			err: &legacy_save.ValidationError{Issues: []legacy_save.ValidationIssue{{
				Code: "invalid_filename", Message: "selected file must be named SAVED.GAM",
			}}},
			want: "The active game location must be a folder containing SAVED.GAM and SAVED.OOL.",
		},
		{
			name: "missing sibling",
			err: &legacy_save.ValidationError{Issues: []legacy_save.ValidationIssue{{
				Code: "missing_sibling", Message: "SAVED.OOL must be present",
			}}},
			want: "SAVED.OOL is missing beside SAVED.GAM in the active game folder.",
		},
		{
			name: "invalid length",
			err: &legacy_save.ValidationError{Issues: []legacy_save.ValidationIssue{{
				Code: "invalid_length", Message: "SAVED.GAM must be exactly 4192 bytes; got 10",
			}}},
			want: "SAVED.GAM must be exactly 4192 bytes; got 10",
		},
		{
			name: "native conversion",
			err: &save_snapshot.ValidationError{Issues: []save_snapshot.Issue{{
				Code: "unsupported", Message: "active spell code is unsupported",
			}}},
			want: "Legacy save state is unsupported: active spell code is unsupported",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, legacySavePreparationMessage(fmt.Errorf("preparing original save: %w", test.err)))
		})
	}
}

func TestIntroPresentationSkipEnterDoesNotCarryIntoMenu(t *testing.T) {
	scene := &IntroMenuScene{
		keyboard:     newPresentationTestKeyboard(),
		presentation: newIntroPresentationDriver(nil),
	}
	scene.eventQueueManager = event_queue.NewEventQueueManager(func() int64 { return scene.elapsedMs }, nil)
	startIntroPresentationWorkflow(scene.eventQueueManager, scene.presentation, func() {
		scene.menuVisible = true
		scene.keyboard.PrepareForTransition(0)
	})
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)

	require.NoError(t, scene.Update(nil))
	require.True(t, scene.menuVisible)
	require.NoError(t, scene.Update(nil))
}

func TestOriginFramePositionKeepsFramesCentered(t *testing.T) {
	assert.Equal(t, image.Pt(20, introOriginFullY), originFramePosition(image.Pt(280, 61)))
	assert.Equal(t, image.Pt(148, introOriginFullY+29), originFramePosition(image.Pt(24, 3)))
}

func TestOriginFrameDrawOptionsVerticallyInvertsFlippedStage(t *testing.T) {
	size := image.Pt(2, 2)
	position := originFramePosition(size)
	drawOptions := originFrameDrawOptions(size, true)

	topX, topY := drawOptions.GeoM.Apply(0, 0)
	bottomX, bottomY := drawOptions.GeoM.Apply(0, 1)
	assert.Equal(t, float64(position.X), topX)
	assert.Equal(t, float64(position.X), bottomX)
	assert.Equal(t, float64(position.Y+size.Y), topY)
	assert.Equal(t, float64(position.Y+size.Y-1), bottomY)
}

func newIntroInputTestScene() *IntroMenuScene {
	return &IntroMenuScene{
		keyboard: newPresentationTestKeyboard(),
		config: &config.UltimaVConfiguration{
			SavedConfigData:       &config.UltimaVConfigurationFlags{},
			AudioConfiguration:    &config.AudioConfiguration{},
			DisplayConfiguration:  &config.DisplayConfiguration{Mode: config.ModeWindowed, WindowWidth: 1280, WindowHeight: 720},
			GameplayConfiguration: config.DefaultGameplayConfiguration(),
		},
		nCurrentSelection: 0,
		menuVisible:       true,
	}
}

// Simulated keys still exercise routing and release handling, but unrelated
// native input must not leak into frame-advancing presentation tests.
func newPresentationTestKeyboard() *input.Keyboard {
	return input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false })
}

func fakeReturnViewData(t *testing.T) *minimap.ReturnViewData {
	t.Helper()
	script := minimap.NewMiniMapScript("return-view-test").
		TileCommand(minimap.TileCommand{Type: minimap.TileCommandSetMap, MapID: "return-view-0"}).
		TileCommand(minimap.TileCommand{Type: minimap.TileCommandRefresh, Cycles: 100, CycleMs: 16}).
		TileCommand(minimap.TileCommand{Type: minimap.TileCommandReset}).
		Build()
	require.NoError(t, script.Validate())

	return &minimap.ReturnViewData{
		Maps: map[minimap.FrameID]minimap.TileMap{
			"return-view-0": {
				Width: 1, Height: 1, Tiles: []indexes.SpriteIndex{indexes.Grass}, Caption: "Test",
			},
		},
		Script: script,
	}
}

// openIntroSubmenu places the cursor on a child row as if the player had
// opened its parent with Enter.
func openIntroSubmenu(scene *IntroMenuScene, parent, child int) {
	scene.nCurrentSelection = parent
	scene.submenuOpen = true
	scene.submenuSelection = child
}

func openIntroSaveBrowser(t *testing.T, game *Game, scene *IntroMenuScene, choice int) *widgets.SaveSlotBrowser {
	t.Helper()
	scene.nCurrentSelection = choice
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, game.Update())
	browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
	require.True(t, ok)

	return browser
}

func openIntroImportBrowser(t *testing.T, game *Game, scene *IntroMenuScene, legacyPath string) *widgets.SaveSlotBrowser {
	t.Helper()
	scene.config.SavedConfigData.DataFilePath = filepath.Dir(legacyPath)
	scene.startActiveLegacySaveImport(game)
	require.Eventually(t, func() bool {
		require.NoError(t, game.Update())
		_, ready := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)

		return ready
	}, time.Second, time.Millisecond)
	browser, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SaveSlotBrowser)
	require.True(t, ok)

	return browser
}

var _ playerSaveRepository = (*introMenuObservingRepository)(nil)
