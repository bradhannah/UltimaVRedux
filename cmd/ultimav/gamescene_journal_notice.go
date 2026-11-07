package main

import (
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

type journalNotice struct {
	pending bool
	queue   *knowledgeNoticeQueue
}

type knowledgeNoticeQueue struct {
	pending []journal.AtlasChange // Empty EntryKey denotes a coalesced journal notice.
}

// journalChanged records reader badges immediately; presentation uses the shared lane.
func (g *GameScene) journalChanged(tab, entry int) {
	g.journalVisits.Record(tab, entry)
	if !g.journalNoticeSuppressed && !g.disposed {
		g.journalNotice.pending = true
	}
}

func (g *GameScene) atlasUpdated(change journal.AtlasChange) {
	if g.disposed || !change.Discovered && !change.Named && !change.Surveyed {
		return
	}
	queue := g.ensureKnowledgeNotices()
	for index := range queue.pending {
		pending := &queue.pending[index]
		if pending.EntryKey == change.EntryKey {
			pending.Name, pending.Kind, pending.Measured = change.Name, change.Kind, change.Measured
			pending.Discovered = pending.Discovered || change.Discovered
			pending.Named = pending.Named || change.Named
			pending.Surveyed = pending.Surveyed || change.Surveyed

			return
		}
	}
	// Journal callbacks may have arrived in an earlier blocked frame. Keep every
	// pending map update ahead of that coalesced journal notice.
	index := len(queue.pending)
	if index > 0 && queue.pending[index-1].EntryKey == "" {
		index--
	}
	queue.pending = append(queue.pending, journal.AtlasChange{})
	copy(queue.pending[index+1:], queue.pending[index:])
	queue.pending[index] = change
}

func (g *GameScene) ensureKnowledgeNotices() *knowledgeNoticeQueue {
	if g.journalNotice.queue == nil {
		g.journalNotice.queue = &knowledgeNoticeQueue{}
	}

	return g.journalNotice.queue
}

func (g *GameScene) knowledgeNoticeBlocked() bool {
	return g.secondaryKeyState != PrimaryInput || g.inputBlocked || g.mapFadeLevel > 0 || g.atlasSession != nil || g.journalSession != nil ||
		g.dialogStack.PeekTopModalDialog() != nil || g.finalePresentation != nil || g.miniMapScene != nil ||
		g.autoOpenDoorState != nil && g.autoOpenDoorState.IsActive() ||
		g.gameState != nil && (g.gameState.ShrineScene != nil ||
			g.gameState.EventQueueManager != nil && g.gameState.EventQueueManager.HasActiveWorkflows())
}

// Entry and exit blockers both count: a dismissed workflow or secondary prompt
// finishes its own output before incidental notices can append to the stream.
func (g *GameScene) updateJournalNoticeAfterFrame(blockedAtStart bool) {
	if g.disposed || g.journalNotice.queue == nil && !g.journalNotice.pending {
		return
	}
	queue := g.ensureKnowledgeNotices()
	if g.journalNotice.pending {
		g.journalNotice.pending = false
		queued := false
		for _, pending := range queue.pending {
			queued = queued || pending.EntryKey == ""
		}
		if !queued {
			// Flushed after live map callbacks: simultaneous changes show map first.
			queue.pending = append(queue.pending, journal.AtlasChange{})
		}
	}
	if blockedAtStart || g.knowledgeNoticeBlocked() {
		return
	}
	for _, change := range queue.pending {
		g.writeKnowledgeNotice(change)
	}
	queue.pending = nil
}

func (g *GameScene) writeKnowledgeNotice(change journal.AtlasChange) {
	if change.EntryKey == "" {
		if g.gameConfig.GetSimpleNotesMode() {
			return // The update is recorded, but it belongs to tabs Simple Notes Mode hides.
		}
		g.output.AppendLine().Text("Journal updated.", widgets.DialogLabelColor).Done()
		g.output.AppendLine().Text("[Shift+J]", widgets.DialogControlKeyColor).
			Text(" View journal", widgets.DialogControlHintColor).Done()

		return
	}
	g.output.AppendLine().Text("Map updated: ", widgets.DialogLabelColor).
		Text(strings.TrimPrefix(atlasNoticeHistory(change), "Map updated: "), widgets.DialogValueColor).Done()
	g.output.AppendLine().Text("[Shift+M]", widgets.DialogControlKeyColor).
		Text(" View map", widgets.DialogControlHintColor).Done()
}

func atlasNoticeHistory(change journal.AtlasChange) string {
	if !change.Discovered && !change.Named {
		return "Map updated: Coordinates added for " + change.Name + "."
	}
	action := "recorded"
	if !change.Discovered {
		action = "identified"
	}
	if change.Measured {
		if change.Discovered {
			return "Map updated: " + change.Name + " and its coordinates recorded."
		}

		return "Map updated: " + change.Name + " " + action + ". Coordinates recorded."
	}

	return "Map updated: " + change.Name + " " + action + ". Coordinates not measured."
}

func (g *GameScene) clearKnowledgeNotices() {
	g.journalNotice = journalNotice{}
}
