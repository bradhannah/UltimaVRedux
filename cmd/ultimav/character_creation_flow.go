package main

import (
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/character_creation"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/new_game"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

const (
	characterCreationSaveTitle  = "Create New Game"
	characterCreationSaveAction = "Create"
)

var errCharacterCreationServicesUnavailable = errors.New("character creation services are unavailable")

type preparedCharacterCreationSave struct {
	snapshot    save_snapshot.SaveSnapshot
	playthrough playthroughMetadata
	draft       playerSaveDescriptionDraft
}

type characterCreationSeedProvider func() (uint64, error)

type characterCreationSceneBuilder func(
	*config.UltimaVConfiguration,
	*rng.Source,
	CharacterCreationCallbacks,
	*characterCreationIdentityPresentation,
) (Scene, error)

type characterCreationSnapshotPreparer func(
	string,
	character_creation.Result,
) (save_snapshot.SaveSnapshot, error)

func newCharacterCreationSeed() (uint64, error) {
	// REDUX DEVIATION: The original seeds one process-wide stream from randtime.
	// Redux uses fresh OS entropy per attempt and persists exact stream provenance.
	// See ref:CREATE-0002.
	var seedBytes [8]byte
	if _, err := rand.Read(seedBytes[:]); err != nil {
		return 0, fmt.Errorf("generating character creation seed: %w", err)
	}

	return binary.LittleEndian.Uint64(seedBytes[:]), nil
}

func prepareCharacterCreationSnapshot(
	originalDataDirectory string,
	result character_creation.Result,
) (save_snapshot.SaveSnapshot, error) {
	conversion, err := new_game.PrepareInitialSnapshot(originalDataDirectory, result)
	if err != nil {
		return save_snapshot.SaveSnapshot{}, fmt.Errorf("preparing initial native snapshot: %w", err)
	}
	if len(conversion.Warnings) > 0 {
		logging.Warnf(
			logging.CategoryUI,
			"New-game template conversion completed with %d warnings",
			len(conversion.Warnings),
		)
	}

	return conversion.Snapshot, nil
}

func (g *Game) startCharacterCreation() error {
	if g.characterCreationSeedProvider == nil || g.characterCreationSceneBuilder == nil {
		return fmt.Errorf("starting character creation: %w", errCharacterCreationServicesUnavailable)
	}
	configuration := g.currentScene.GetUltimaConfiguration()
	if configuration == nil {
		return fmt.Errorf(
			"starting character creation: %w: configuration is missing",
			errCharacterCreationServicesUnavailable,
		)
	}
	seed, err := g.characterCreationSeedProvider()
	if err != nil {
		return fmt.Errorf("starting character creation: %w", err)
	}
	callbacks := CharacterCreationCallbacks{
		OnComplete: func(result character_creation.Result) {
			g.completeCharacterCreation(result)
		},
		OnCancel: func() {
			g.cancelCharacterCreation(configuration)
		},
	}
	intro, ok := g.currentScene.(*IntroMenuScene)
	if !ok {
		return fmt.Errorf(
			"starting character creation: %w: title menu is not current",
			errCharacterCreationServicesUnavailable,
		)
	}
	identityPresentation := intro.characterCreationIdentityPresentation()

	return g.ReplaceScene(func() (Scene, error) {
		scene, buildErr := g.characterCreationSceneBuilder(
			configuration,
			rng.New(seed),
			callbacks,
			identityPresentation,
		)
		if buildErr != nil {
			return nil, fmt.Errorf("building character creation scene: %w", buildErr)
		}
		if scene == nil {
			return nil, fmt.Errorf("building character creation scene: %w", errCharacterCreationServicesUnavailable)
		}

		return scene, nil
	})
}

func (g *Game) cancelCharacterCreation(configuration *config.UltimaVConfiguration) {
	scene := g.currentCharacterCreationScene()
	var retry func()
	retry = func() {
		if err := g.returnToMainMenu(configuration); err != nil {
			logging.Warnf(logging.CategoryUI, "Returning from character creation failed: %v", err)
			scene.configureTerminalAction(retry, "The main menu could not be restored.", "[Enter] Retry")
		}
	}
	retry()
}

func (g *Game) completeCharacterCreation(result character_creation.Result) {
	scene := g.currentCharacterCreationScene()
	if g.characterCreationSnapshotPreparer == nil {
		scene.configureTerminalAction(
			func() { g.completeCharacterCreation(result) },
			"New-game preparation is unavailable.", "[Enter] Retry",
		)

		return
	}
	var retry func()
	retry = func() {
		snapshot, err := g.characterCreationSnapshotPreparer(
			scene.config.SavedConfigData.DataFilePath,
			result,
		)
		if err != nil {
			logging.Warnf(logging.CategoryUI, "Preparing completed character creation failed: %v", err)
			scene.configureTerminalAction(retry, "New game could not be prepared.", "[Enter] Retry")

			return
		}
		g.configureCharacterCreationSaveSelection(scene, snapshot)
	}
	scene.configureTerminalAction(retry, "", "")
	retry()
}

func (g *Game) configureCharacterCreationSaveSelection(
	scene *CharacterCreationScene,
	snapshot save_snapshot.SaveSnapshot,
) {
	metadata, err := freshPlaythroughMetadata(g.playthroughIDProvider)
	if err != nil {
		logging.Warnf(logging.CategoryUI, "Preparing new playthrough identity failed: %v", err)
		scene.configureTerminalAction(func() { g.configureCharacterCreationSaveSelection(scene, snapshot) },
			"New game could not be prepared.", "[Enter] Retry")

		return
	}
	prepared := &preparedCharacterCreationSave{
		snapshot: snapshot, playthrough: metadata,
		draft: playerSaveDescriptionDraft{slot: "", text: "", set: false},
	}
	// REDUX DEVIATION: The original overwrites SAVED.GAM without warning and
	// returns to the menu. Redux reviews a native slot, confirms overwrites, and
	// enters gameplay only after a compensating persistence/activation transaction.
	// See ref:CREATE-0001.
	reopen := func() {
		g.openCharacterCreationSaveBrowser(scene, prepared, "", "")
	}
	scene.configureTerminalAction(reopen, "", "")
	reopen()
}

func (g *Game) openCharacterCreationSaveBrowser(
	scene *CharacterCreationScene,
	prepared *preparedCharacterCreationSave,
	selectionID string,
	notice string,
) {
	if g.currentScene != scene {
		panic("Game.openCharacterCreationSaveBrowser: character creation scene is not current")
	}
	summaries, err := g.listPlayerSaveSlots()
	if err != nil {
		logging.Warnf(logging.CategoryUI, "Listing character creation save slots failed: %v", err)
		scene.setTerminalNotice("Save slots could not be listed.", "[Enter] Retry")

		return
	}
	model := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserWrite, prepared.playthrough.id)
	if selectionID != "" {
		model.initialSelectionID = selectionID
		model.requireExplicitSelection = false
	}
	var browser *widgets.SaveSlotBrowser
	browser = widgets.NewSaveSlotBrowser(model.views, widgets.SaveSlotBrowserConfig{
		Title:                    characterCreationSaveTitle,
		PrimaryActionLabel:       characterCreationSaveAction,
		CancelActionLabel:        "",
		ReviewLabel:              "",
		InitialSelectionID:       model.initialSelectionID,
		NewestLegend:             model.newestLegend,
		RequireExplicitSelection: model.requireExplicitSelection,
		Notice:                   notice,
		Keyboard:                 scene.keyboard,
		Placement: sprites.PercentBasedPlacement{
			StartPercentX: 0,
			EndPercentX:   0,
			StartPercentY: 0,
			EndPercentY:   0,
		},
		OnPrimary: func(view widgets.SaveSlotView) {
			summary, found := model.summary(view)
			if !found {
				panic("Game.openCharacterCreationSaveBrowser: browser returned an unknown slot")
			}
			g.selectCharacterCreationSaveDestination(scene, browser, summary, prepared)
		},
		OnCancel: func() {
			scene.dialogStack.MustRemoveWidgetInstance(browser)
			scene.setTerminalNotice("Character complete.", "[Enter] Choose a save slot")
		},
		IsKeyPressed: nil,
	})
	scene.terminalNotice = ""
	scene.dialogStack.PushModalDialog(browser)
}

func (g *Game) selectCharacterCreationSaveDestination(
	scene *CharacterCreationScene,
	browser *widgets.SaveSlotBrowser,
	summary player_save.SlotSummary,
	prepared *preparedCharacterCreationSave,
) {
	switch summary.Status {
	case player_save.SlotEmpty, player_save.SlotReady, player_save.SlotIncompatible, player_save.SlotCorrupt:
		showPlayerSaveDescriptionEditor(&scene.dialogStack, scene.keyboard, "Create In", summary,
			prepared.playthrough.id, &prepared.draft, func(note string) {
				prepared.playthrough.userDescription = note
				g.commitCharacterCreationSave(scene, browser, summary, prepared)
			})
	case player_save.SlotUnavailable:
		// These rows are disabled by the reviewed browser model.
	default:
		panic("Game.selectCharacterCreationSaveDestination: unsupported slot status")
	}
}

func (g *Game) commitCharacterCreationSave(
	scene *CharacterCreationScene,
	browser *widgets.SaveSlotBrowser,
	summary player_save.SlotSummary,
	prepared *preparedCharacterCreationSave,
) {
	scene.dialogStack.MustRemoveWidgetInstance(browser)
	err := g.commitPreparedPlayerSave(
		summary.Slot,
		summary.Revision,
		prepared.snapshot,
		playerSaveNewGameDescription,
		prepared.playthrough,
	)
	if err == nil {
		return
	}
	logging.Warnf(logging.CategoryUI, "Creating new game in %s failed: %v", summary.Slot, err)
	g.openCharacterCreationSaveBrowser(
		scene,
		prepared,
		string(summary.Slot),
		"Create failed: "+playerSaveErrorDetail(err),
	)
}

func (g *Game) currentCharacterCreationScene() *CharacterCreationScene {
	scene, ok := g.currentScene.(*CharacterCreationScene)
	if !ok || scene.disposed {
		panic("Game.currentCharacterCreationScene: active character creation scene is required")
	}

	return scene
}
