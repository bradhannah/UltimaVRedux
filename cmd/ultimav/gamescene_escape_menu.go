package main

import (
	"errors"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const (
	escapeMenuButtonWidthPercent = 0.22
)

func (g *GameScene) DoEscapeMenu(game *Game) {
	if game == nil {
		panic("GameScene.DoEscapeMenu: game is required")
	}

	// Pause all music and sound effects when entering escape menu
	if g.audioManager != nil {
		g.audioManager.GetMusicManager().SetSuppressed(audio.MusicMenu, true)
		g.audioManager.SetEnabled(false)
	}

	// Declare buttonList variable first so it can be captured in closures
	var buttonList *widgets.ButtonListModal

	buttonList = widgets.NewButtonListModal(
		"Ultima V Redux",
		func() {
			g.dialogStack.MustRemoveWidgetInstance(buttonList)
			g.resumeAudio()
			// The closing key must be released before gameplay sees input again.
			g.keyboard.PrepareForTransition(0)
		},
		g.keyboard,
		&gameScreenPercents) // Center on map area, not full screen
	buttonList.SetButtonWidthPercent(escapeMenuButtonWidthPercent)

	g.dialogStack.PushModalDialog(buttonList)
	buttonList.AddButton("Quick Save", func() {
		g.dialogStack.MustRemoveWidgetInstance(buttonList)
		record, err := game.quickSavePlayerSave()
		if errors.Is(err, ErrNoActivePlayerSaveSlot) {
			g.openPlayerSaveBrowser(game, playerSaveBrowserWrite)

			return
		}
		if err != nil {
			g.finishPlayerSaveFailure("Quick save", err)

			return
		}
		g.resumeAudio()
		g.addRowStr("Quick saved " + playerSaveSlotName(record.Slot) + ".")
	})
	buttonList.AddButton("Save", func() {
		g.dialogStack.MustRemoveWidgetInstance(buttonList)
		g.openPlayerSaveBrowser(game, playerSaveBrowserWrite)
	})
	buttonList.AddButton("Load", func() {
		g.dialogStack.MustRemoveWidgetInstance(buttonList)
		g.openPlayerSaveBrowser(game, playerSaveBrowserLoad)
	})
	buttonList.AddButton("Configure", func() {
		// Keep escape menu on stack, push Configuration on top.
		g.openConfigurationDialog()
		// Wait for the opening key to release without delaying the next input.
		g.keyboard.PrepareForTransition(0)
	})
	buttonList.AddButton("Return to Main Menu", func() {
		g.dialogStack.MustRemoveWidgetInstance(buttonList)
		g.openEscapeDeparturePrompt(game, escapeDepartureMainMenu, "")
	})
	buttonList.AddButton("Exit to DOS", func() {
		g.dialogStack.MustRemoveWidgetInstance(buttonList)
		g.openEscapeDeparturePrompt(game, escapeDepartureApplication, "")
	})
	buttonList.AddButton("Help", func() {
		g.showCommandHelp()
	})
}

func (g *GameScene) openPlayerSaveBrowser(game *Game, mode playerSaveBrowserMode) {
	var writeFlow *playerSaveWriteFlow
	if mode == playerSaveBrowserWrite {
		writeFlow = g.gameplaySaveWriteFlow()
	}
	g.openPlayerSaveBrowserWithState(game, mode, "", "", writeFlow)
}

func (g *GameScene) openPlayerSaveBrowserWithState(
	game *Game,
	mode playerSaveBrowserMode,
	selectionID string,
	notice string,
	writeFlow *playerSaveWriteFlow,
) {
	summaries, err := game.listPlayerSaveSlots()
	if err != nil {
		if writeFlow != nil {
			writeFlow.onFailure(playerSaveBrowserAction(mode), err)
		} else {
			g.finishPlayerSaveFailure(playerSaveBrowserAction(mode), err)
		}

		return
	}
	model := buildPlayerSaveBrowserModel(summaries, mode, g.activePlaythrough.id)
	if selectionID != "" {
		model.initialSelectionID = selectionID
		model.requireExplicitSelection = false
	}
	var browser *widgets.SaveSlotBrowser
	browser = widgets.NewSaveSlotBrowser(model.views, widgets.SaveSlotBrowserConfig{
		Title:                    playerSaveBrowserTitle(mode),
		PrimaryActionLabel:       playerSaveBrowserAction(mode),
		InitialSelectionID:       model.initialSelectionID,
		NewestLegend:             model.newestLegend,
		RequireExplicitSelection: model.requireExplicitSelection,
		Notice:                   notice,
		Keyboard:                 g.keyboard,
		Placement:                gameScreenPercents,
		OnPrimary: func(view widgets.SaveSlotView) {
			summary, found := model.summary(view)
			if !found {
				panic("GameScene.openPlayerSaveBrowser: selected slot summary is missing")
			}
			if mode == playerSaveBrowserLoad {
				g.loadPlayerSaveSummary(game, browser, mode, summary)

				return
			}
			g.writePlayerSaveSummary(game, browser, mode, summary, writeFlow)
		},
		OnCancel: func() {
			g.dialogStack.MustRemoveWidgetInstance(browser)
			if writeFlow != nil {
				writeFlow.onCancel()
			} else {
				g.resumeAudio()
			}
		},
		IsKeyPressed: nil,
	})
	g.dialogStack.PushModalDialog(browser)
}

func (g *GameScene) writePlayerSaveSummary(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	summary player_save.SlotSummary,
	writeFlow *playerSaveWriteFlow,
) {
	switch summary.Status {
	case player_save.SlotEmpty, player_save.SlotReady, player_save.SlotIncompatible, player_save.SlotCorrupt:
		g.confirmPlayerSaveOverwrite(game, browser, mode, summary, writeFlow)
	case player_save.SlotUnavailable:
		// The browser disables these entries, so their callback is not reachable.
	}
}

func (g *GameScene) confirmPlayerSaveOverwrite(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	summary player_save.SlotSummary,
	writeFlow *playerSaveWriteFlow,
) {
	if writeFlow == nil {
		panic("GameScene.confirmPlayerSaveOverwrite: write flow is required")
	}
	showPlayerSaveDescriptionEditor(&g.dialogStack, g.keyboard, "Save To", summary,
		g.activePlaythrough.id, &writeFlow.draft, func(note string) {
			g.savePlayerSaveSummary(game, browser, mode, summary, writeFlow, note)
		})
}

func (g *GameScene) savePlayerSaveSummary(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	summary player_save.SlotSummary,
	writeFlow *playerSaveWriteFlow,
	note string,
) {
	g.dialogStack.MustRemoveWidgetInstance(browser)
	record, err := game.savePlayerSaveSlot(summary.Slot, summary.Revision, note)
	if err != nil {
		g.openPlayerSaveBrowserWithState(
			game,
			mode,
			string(summary.Slot),
			playerSaveFailureMessage("Save", err),
			writeFlow,
		)

		return
	}
	if writeFlow == nil {
		panic("GameScene.savePlayerSaveSummary: write flow is required")
	}
	writeFlow.onSaved(record)
}

func (g *GameScene) loadPlayerSaveSummary(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	summary player_save.SlotSummary,
) {
	if err := game.loadPlayerSaveSlot(summary.Slot, summary.Revision); err != nil {
		g.refreshPlayerSaveBrowserAfterFailure(game, browser, mode, summary.Slot, "Load", err)

		return
	}
	g.dialogStack.MustRemoveWidgetInstance(browser)

	showPlayerSaveLoadSuccess(game, summary.Slot)
}

func (g *GameScene) refreshPlayerSaveBrowserAfterFailure(
	game *Game,
	browser *widgets.SaveSlotBrowser,
	mode playerSaveBrowserMode,
	slot player_save.SlotID,
	action string,
	err error,
) {
	g.dialogStack.MustRemoveWidgetInstance(browser)
	g.openPlayerSaveBrowserWithState(
		game,
		mode,
		string(slot),
		playerSaveFailureMessage(action, err),
		nil,
	)
}

func (g *GameScene) gameplaySaveWriteFlow() *playerSaveWriteFlow {
	return &playerSaveWriteFlow{
		onCancel: g.resumeAudio,
		onSaved: func(record player_save.Record) {
			g.resumeAudio()
			g.addRowStr("Saved " + playerSaveSlotName(record.Slot) + ".")
		},
		onFailure: g.finishPlayerSaveFailure,
	}
}

func showPlayerSaveLoadSuccess(game *Game, slot player_save.SlotID) {
	loadedScene, ok := game.currentScene.(*GameScene)
	if !ok {
		return
	}
	loadedScene.addRowStr("Loaded " + playerSaveSlotName(slot) + ".")
}

func (g *GameScene) finishPlayerSaveFailure(action string, err error) {
	g.resumeAudio()
	g.addRowStr(playerSaveFailureMessage(action, err))
}

func playerSaveBrowserTitle(mode playerSaveBrowserMode) string {
	if mode == playerSaveBrowserLoad {
		return "Load Game"
	}

	return "Save Game"
}

func playerSaveBrowserAction(mode playerSaveBrowserMode) string {
	if mode == playerSaveBrowserLoad {
		return "Load"
	}

	return "Save"
}

// resumeAudio resumes music and sound effects after closing escape menu
func (g *GameScene) resumeAudio() {
	if g.audioManager != nil {
		g.audioManager.SetEnabled(true)
		g.audioManager.GetMusicManager().SetSuppressed(audio.MusicMenu, false)
		// Refresh the intended location without resetting playback/history.
		g.updateMusicForCurrentLocation()
	}
}
