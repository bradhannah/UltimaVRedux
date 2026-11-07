package main

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/hajimehoshi/ebiten/v2"
)

type atlasSession struct {
	dialog             *widgets.AtlasDialog
	child              *widgets.AtlasNoteEditor
	noteDraft          *game_state.AtlasNoteDraft
	state              *game_state.GameState
	frozenMs           int64
	closing            bool
	resumePresentation []func()
}

func (g *GameScene) canOpenAtlas() bool {
	if g.secondaryKeyState != PrimaryInput || g.inputBlocked || g.journalSession != nil ||
		g.finalePresentation != nil || g.dialogStack.PeekTopModalDialog() != nil ||
		g.autoOpenDoorState != nil && g.autoOpenDoorState.IsActive() ||
		g.gameState.CombatScene != nil || !g.gameState.CanAcceptGameplayCommands() ||
		g.gameState.EventQueueManager.HasActiveWorkflows() ||
		len(g.pendingBattleSummaries) > 0 ||
		g.gameState.HasPendingOverworldContact() || g.gameState.HasPendingDungeonRoomEncounter() {
		return false
	}
	location := g.gameState.MapState.PlayerLocation.Location

	return location == references.BritanniaUnderworld || location >= references.Moonglow && location <= references.Doom
}

// pollAtlasRequest only detects input. It never advances a workflow or changes a
// command state to make the reader available. The held M latch prevents falling Mix.
func (g *GameScene) pollAtlasRequest() bool {
	if !g.keyboard.IsKeyHeld(ebiten.KeyM) {
		g.atlasChordHeld = false
	}
	if g.keyboard.IsKeyPressedAndRegister(widgets.AtlasFunctionKey) {
		return true
	}
	if !widgets.IsAtlasChord(g.keyboard) {
		return false
	}
	g.atlasChordHeld = true

	return g.keyboard.IsKeyPressedAndRegister(ebiten.KeyM)
}

func (g *GameScene) openAtlas(game *Game) {
	g.clearProvisionFeedback()
	var texture *ebiten.Image
	if game != nil {
		var err error
		texture, err = game.atlasTexture.load(g.gameState.GameReferences.OverworldLargeMapReference)
		if err != nil {
			logging.Warnf(logging.CategoryUI, "Traveller's map geography unavailable: %v", err)
		}
	}
	g.keyboard.PrepareForTransition(0)
	session := &atlasSession{
		dialog: nil, child: nil, noteDraft: nil, state: g.gameState,
		frozenMs: g.gameState.ElapsedMs, closing: false, resumePresentation: nil,
	}
	if g.audioManager != nil {
		g.audioManager.GetMusicManager().SetSuppressed(audio.MusicJournal, true)
	}
	for _, widget := range g.dialogStack.Dialogs() {
		if clock, ok := widget.(interface{ SuspendPresentationClock() func() }); ok {
			session.resumePresentation = append(session.resumePresentation, clock.SuspendPresentationClock())
		}
	}
	session.dialog = widgets.NewAtlasDialog(g.gameState.CurrentAtlasDossier(), texture, g.keyboard, &g.atlasView, func() { g.closeAtlasSession(false) })
	session.dialog.SetNoteIntentHandler(g.handleAtlasNoteIntent)
	session.dialog.SetLighting(g.atlasLighting())
	g.atlasSession = session
	g.dialogStack.PushModalDialog(session.dialog)
}

func (g *GameScene) updateAtlasSession() {
	session := g.atlasSession
	if session.state != g.gameState {
		session.frozenMs = g.gameState.ElapsedMs
		g.closeAtlasSession(true)

		return
	}
	if session.closing {
		if !g.keyboard.IsWaitingForRelease() {
			g.finishAtlasPause()
		}

		return
	}
	session.dialog.Refresh(g.gameState.CurrentAtlasDossier())
	// Freeze non-blocking presentation too; only the actual top modal owns input.
	if top := g.dialogStack.PeekTopModalDialog(); top != nil {
		top.Update()
	}
}

func (g *GameScene) closeAtlasSession(force bool) {
	session := g.atlasSession
	if session == nil {
		return
	}
	if !session.closing {
		g.closeAtlasChild()
		g.dialogStack.MustRemoveWidgetInstance(session.dialog)
		g.keyboard.PrepareForTransition(0)
		session.closing = true
	}
	if force || !g.keyboard.IsWaitingForRelease() {
		g.finishAtlasPause()
	}
}

func (g *GameScene) closeAtlasChild() {
	if g.atlasSession.child == nil {
		return
	}
	g.dialogStack.MustRemoveWidgetInstance(g.atlasSession.child)
	g.atlasSession.noteDraft.Discard()
	g.atlasSession.noteDraft = nil
	g.atlasSession.child = nil
	g.keyboard.PrepareForTransition(0)
}

func (g *GameScene) handleAtlasNoteIntent(intent widgets.AtlasNoteIntent) {
	session := g.atlasSession
	if session == nil || session.child != nil || session.state != g.gameState {
		return
	}
	draft, err := g.gameState.BeginAtlasNoteDraft(intent.ID)
	if err != nil {
		session.dialog.SetStatus(err.Error())

		return
	}
	g.keyboard.PrepareForTransition(0)
	session.noteDraft = draft
	session.child = widgets.NewAtlasNoteEditor(widgets.AtlasNoteEditorConfig{
		Record: draft.Record(), Context: draft.ContextLabel(), Deleting: intent.Action == widgets.AtlasNoteDelete,
		Keyboard: g.keyboard, Cancel: g.closeAtlasChild, AppendInputChars: g.appendInputChars,
		Submit: func(label, description string) error {
			if intent.Action == widgets.AtlasNoteDelete {
				if err := g.gameState.DeleteAtlasNote(draft); err != nil {
					return fmt.Errorf("delete atlas note: %w", err)
				}
				session.dialog.Refresh(g.gameState.CurrentAtlasDossier())
				session.dialog.SetStatus("Note deleted.")
			} else {
				record, err := g.gameState.CommitAtlasNote(draft, label, description)
				if err != nil {
					return fmt.Errorf("save atlas note: %w", err)
				}
				session.dialog.NoteCommitted(g.gameState.CurrentAtlasDossier(), record.ID)
			}
			g.closeAtlasChild()

			return nil
		},
	})
	g.dialogStack.PushModalDialog(session.child)
}

func (g *GameScene) finishAtlasPause() {
	session := g.atlasSession
	if g.audioManager != nil {
		g.audioManager.GetMusicManager().SetSuppressed(audio.MusicJournal, false)
	}
	for _, resume := range session.resumePresentation {
		resume()
	}
	g.elapsedBaseMs = session.frozenMs
	g.startTime = g.sceneNow()
	g.lastEffectsUpdate = g.startTime
	// Reader frames own input and do not poll the overworld shortcut. Forget a
	// released opener now so the next fresh plain N is not mistaken for a held chord.
	g.atlasMarkChordHeld = g.atlasMarkChordHeld && g.keyboard.IsKeyHeld(ebiten.KeyN)
	g.atlasSession = nil
}
