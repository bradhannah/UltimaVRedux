package main

import (
	"math"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

type journalSession struct {
	dialog *widgets.QuestJournalDialog
	// child is the note editor or section-name box open over the Journal.
	child              widgets.Widget
	openingRevision    uint64
	frozenMs           int64
	closing            bool
	resumePresentation []func()
}

// ShowQuestJournalDossier opens the read-only Quest Journal as the active modal.
func (g *GameScene) ShowQuestJournalDossier(dossier game_state.QuestJournalDossier) {
	if g.journalSession != nil {
		return
	}
	g.clearProvisionFeedback()
	visits := g.journalVisits.Clone()
	var newEntries [3][]bool
	for _, entry := range dossier.Chronicle {
		if entry.Sequence > math.MaxInt {
			panic("journal event sequence exceeds the presentation key range")
		}
		newEntries[0] = append(newEntries[0], visits.IsNew(0, int(entry.Sequence)))
	}
	for _, entry := range dossier.Virtues {
		newEntries[1] = append(newEntries[1], visits.IsNew(1, entry.Index))
	}
	for index := range dossier.Shadowlords {
		newEntries[2] = append(newEntries[2], visits.IsNew(2, index))
	}
	g.keyboard.PrepareForTransition(0)
	excerpts, excerptErr := g.loadExcerpts()
	excerptError := ""
	if excerptErr != nil {
		logging.Errorf(logging.CategoryConversation, "Unable to load conversation excerpts: %v", excerptErr)
		excerptError = "Excerpt notebook unavailable."
	}
	session := &journalSession{openingRevision: visits.Revision, frozenMs: g.gameState.ElapsedMs}
	if g.audioManager != nil {
		g.audioManager.GetMusicManager().SetSuppressed(audio.MusicJournal, true)
	}
	for _, widget := range g.dialogStack.Dialogs() {
		if clock, ok := widget.(interface{ SuspendPresentationClock() func() }); ok {
			session.resumePresentation = append(session.resumePresentation, clock.SuspendPresentationClock())
		}
	}
	session.dialog = widgets.NewQuestJournalDialog(dossier, widgets.QuestJournalDialogConfig{
		Keyboard:         g.keyboard,
		NewEntries:       newEntries,
		ShadowlordSprite: g.spriteSheet.GetSprite(indexes.ShadowLord1),
		OnDismiss:        func() { g.closeJournalSession(session, true) },
		LastConversation: g.journalConversationForDisplay(),
		Excerpts:         excerpts,
		KeepExcerpt:      g.keepExcerpt,
		DeleteExcerpt:    g.deleteExcerpt,
		ArchiveExcerpt:   g.archiveExcerpt,
		ExcerptError:     excerptError,
		Notes:            g.gameState.PlayerNotes(),
		OnNotesIntent:    g.handleNotesIntent,
	})
	g.dialogStack.PushModalDialog(session.dialog)
	g.journalSession = session
}

func (g *GameScene) closeJournalSession(session *journalSession, acknowledge bool) {
	if session == nil || g.journalSession != session {
		return
	}
	if session.closing {
		if !acknowledge {
			g.finishJournalPause(session)
		}

		return
	}
	if acknowledge {
		g.journalVisits.Acknowledge(session.openingRevision, session.dialog.VisitedTabs())
	}
	if session.child != nil {
		g.dialogStack.MustRemoveWidgetInstance(session.child)
		session.child = nil
	}
	g.dialogStack.MustRemoveWidgetInstance(session.dialog)
	g.keyboard.PrepareForTransition(0)
	session.closing = true
	if acknowledge && g.keyboard.IsWaitingForRelease() {
		return
	}
	g.finishJournalPause(session)
}

func (g *GameScene) finishJournalPause(session *journalSession) {
	if g.audioManager != nil {
		g.audioManager.GetMusicManager().SetSuppressed(audio.MusicJournal, false)
	}
	for _, resume := range session.resumePresentation {
		resume()
	}
	g.elapsedBaseMs = session.frozenMs
	g.startTime = g.sceneNow()
	g.lastEffectsUpdate = g.startTime
	g.journalSession = nil
}

func (g *GameScene) sceneNow() time.Time {
	if g.now != nil {
		return g.now()
	}

	return time.Now()
}

func (g *GameScene) handleQuestJournalShortcut(key ebiten.Key, shiftPressed bool) bool {
	if key != ebiten.KeyJ || !shiftPressed {
		return false
	}
	switch g.gameState.MapState.PlayerLocation.Location.GetMapType() {
	case references.LargeMapType, references.SmallMapType, references.DungeonMapType,
		references.CombatMapType, references.CampingMapType:
		g.gameState.SystemCallbacks.Message.ShowCommandPrompt("Journal...")
		g.gameState.ActionQuestJournal()

		return true
	default:
		return false
	}
}
