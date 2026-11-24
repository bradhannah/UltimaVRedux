package main

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const (
	escapeDepartureButtonWidthPercent = 0.28
	escapeDepartureBorderWidthPercent = 0.396
)

type escapeDeparture uint8

const (
	escapeDepartureMainMenu escapeDeparture = iota
	escapeDepartureApplication
	// escapeDepartureWindowClose is Exit to DOS reached from the window's close
	// button; Cancel returns straight to play instead of to the Escape menu.
	escapeDepartureWindowClose
)

type playerSaveWriteFlow struct {
	draft     playerSaveDescriptionDraft
	onCancel  func()
	onSaved   func(player_save.Record)
	onFailure func(string, error)
}

func (g *GameScene) openEscapeDeparturePrompt(game *Game, departure escapeDeparture, notice string) {
	var prompt *widgets.ButtonListModal
	prompt = widgets.NewButtonListModal(
		escapeDepartureTitle(departure),
		func() {
			g.dialogStack.MustRemoveWidgetInstance(prompt)
			g.cancelEscapeDeparture(game, departure)
		},
		g.keyboard,
		&gameScreenPercents,
	)
	prompt.SetButtonWidthPercent(escapeDepartureButtonWidthPercent)
	prompt.SetBorderWidthPercent(escapeDepartureBorderWidthPercent)
	description := escapeDepartureDescription(departure)
	if notice != "" {
		description = notice + " " + description
	}
	prompt.SetDescription(description)
	prompt.AddButton("Quick Save", func() {
		g.dialogStack.MustRemoveWidgetInstance(prompt)
		g.quickSaveBeforeDeparture(game, departure)
	})
	prompt.AddButton("Save", func() {
		g.dialogStack.MustRemoveWidgetInstance(prompt)
		g.openPlayerSaveBrowserWithState(
			game,
			playerSaveBrowserWrite,
			"",
			"",
			g.departureSaveWriteFlow(game, departure),
		)
	})
	prompt.AddButton(escapeDepartureWithoutSaveLabel(departure), func() {
		g.dialogStack.MustRemoveWidgetInstance(prompt)
		g.completeEscapeDeparture(game, departure)
	})
	prompt.AddButton("Cancel", func() {
		g.dialogStack.MustRemoveWidgetInstance(prompt)
		g.cancelEscapeDeparture(game, departure)
	})
	g.dialogStack.PushModalDialog(prompt)
	g.keyboard.PrepareForTransition(0)
}

func (g *GameScene) quickSaveBeforeDeparture(game *Game, departure escapeDeparture) {
	record, err := game.quickSavePlayerSave()
	if err == nil {
		g.addRowStr("Quick saved " + playerSaveSlotName(record.Slot) + ".")
		g.completeEscapeDeparture(game, departure)

		return
	}
	if errors.Is(err, ErrNoActivePlayerSaveSlot) {
		g.openPlayerSaveBrowserWithState(
			game,
			playerSaveBrowserWrite,
			"",
			"Choose a slot before continuing.",
			g.departureSaveWriteFlow(game, departure),
		)

		return
	}
	g.openEscapeDeparturePrompt(
		game,
		departure,
		"Quick save failed: "+playerSaveErrorDetail(err),
	)
}

func (g *GameScene) departureSaveWriteFlow(game *Game, departure escapeDeparture) *playerSaveWriteFlow {
	return &playerSaveWriteFlow{
		onCancel: func() {
			g.openEscapeDeparturePrompt(game, departure, "Save cancelled.")
		},
		onSaved: func(record player_save.Record) {
			g.addRowStr("Saved " + playerSaveSlotName(record.Slot) + ".")
			g.completeEscapeDeparture(game, departure)
		},
		onFailure: func(action string, err error) {
			g.openEscapeDeparturePrompt(
				game,
				departure,
				fmt.Sprintf("%s failed: %s", action, playerSaveErrorDetail(err)),
			)
		},
	}
}

func (g *GameScene) completeEscapeDeparture(game *Game, departure escapeDeparture) {
	switch departure {
	case escapeDepartureMainMenu:
		if err := game.returnToMainMenu(g.gameConfig); err != nil {
			logging.Warnf(logging.CategoryUI, "Returning to the main menu failed: %v", err)
			g.openEscapeDeparturePrompt(game, departure,
				"The main menu could not be opened. The game log has the details.")
		}
	case escapeDepartureApplication, escapeDepartureWindowClose:
		game.requestExit()
	default:
		panic("GameScene.completeEscapeDeparture: unsupported destination")
	}
}

func (g *GameScene) cancelEscapeDeparture(game *Game, departure escapeDeparture) {
	if departure == escapeDepartureWindowClose {
		g.windowCloseDepartureOpen = false

		return
	}
	g.DoEscapeMenu(game)
}

// HandleWindowCloseRequest offers the Exit to DOS save choices on the first
// window close request. A repeated request while those choices are open, or a
// request after the game has ended, closes the window at once.
func (g *GameScene) HandleWindowCloseRequest(game *Game) bool {
	if g.windowCloseDepartureOpen || g.gameState.IsGameplayTerminal() {
		return false
	}
	g.closeAtlasSession(true)
	if g.journalSession != nil {
		g.closeJournalSession(g.journalSession, false)
	}
	g.windowCloseDepartureOpen = true
	g.openEscapeDeparturePrompt(game, escapeDepartureWindowClose, "")

	return true
}

func escapeDepartureTitle(departure escapeDeparture) string {
	if departure == escapeDepartureMainMenu {
		return "Return to Main Menu"
	}

	return "Exit to DOS"
}

func escapeDepartureDescription(departure escapeDeparture) string {
	if departure == escapeDepartureMainMenu {
		return "Save before returning to the main menu?"
	}

	return "Unsaved progress will be lost. Save before leaving Ultima V Redux?"
}

func escapeDepartureWithoutSaveLabel(departure escapeDeparture) string {
	if departure == escapeDepartureMainMenu {
		return "Main Menu Without Saving"
	}

	return "Exit Without Saving"
}
