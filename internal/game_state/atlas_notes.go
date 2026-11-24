package game_state

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// AtlasNoteDraft is a captured authority token, not a caller-supplied coordinate.
// It belongs to one GameState/checkpoint, can commit once, and is never saved.
type AtlasNoteDraft struct {
	owner        *GameState
	session      *atlasNoteSession
	original     journal.AtlasNoteRecord
	context      SextantContext
	location     references.PlayerLocation
	creating     bool
	consumed     bool
	contextLabel string
}

// Nonzero-sized identity token survives neither hydration nor in-place replacement.
type atlasNoteSession struct{ active bool }

// Record returns detached initial text, position and creation time for the editor.
func (d *AtlasNoteDraft) Record() journal.AtlasNoteRecord { return d.original }

// ContextLabel explains settlement-exterior fixes without exposing room coordinates.
func (d *AtlasNoteDraft) ContextLabel() string { return d.contextLabel }

// Discard invalidates an abandoned editor/confirmation without changing gameplay.
func (d *AtlasNoteDraft) Discard() { d.consumed = true }

// BeginAtlasNoteDraft captures a new fix for id zero, or existing text for editing.
// A duplicate current position opens its existing note instead of allocating a pin.
func (g *GameState) BeginAtlasNoteDraft(id uint64) (*AtlasNoteDraft, error) {
	if g.atlasNoteSession == nil {
		g.atlasNoteSession = &atlasNoteSession{active: true}
	}
	draft := &AtlasNoteDraft{owner: g, session: g.atlasNoteSession}
	if id != 0 {
		for _, record := range g.atlasNotes.Records {
			if record.ID == id {
				draft.original = record

				return draft, nil
			}
		}

		return nil, fmt.Errorf("%w: note no longer exists", journal.ErrInvalidAtlasPersonal)
	}
	fix, ok := g.CurrentSextantFix()
	if !ok {
		return nil, fmt.Errorf("%w: %s", journal.ErrInvalidAtlasPersonal, g.sextantUnavailableReason())
	}
	position := moonstoneJournalSite(fix)
	if !journal.ValidAtlasSurfacePosition(position) {
		return nil, fmt.Errorf("%w: this position cannot be marked", journal.ErrInvalidAtlasPersonal)
	}
	draft.context, draft.location = g.CurrentSextantContext(), g.MapState.PlayerLocation
	if isSurfaceSextantSettlement(draft.context.Location) {
		draft.contextLabel = "Settlement entrance on Britannia"
	}
	for _, record := range g.atlasNotes.Records {
		if record.Position == position {
			draft.original = record

			return draft, nil
		}
	}
	if len(g.atlasNotes.Records) >= journal.MaxAtlasNotes || g.atlasNotes.LastID == journal.MaxAtlasIdentity {
		return nil, fmt.Errorf("%w: the map is full of notes (%d); edit or delete one", journal.ErrInvalidAtlasPersonal, journal.MaxAtlasNotes)
	}
	draft.creating = true
	draft.original = journal.AtlasNoteRecord{Position: position, RecordedAt: g.sampleDungeonObservation().at.Time}

	return draft, nil
}

func (g *GameState) validateAtlasDraft(draft *AtlasNoteDraft) error {
	if draft == nil || draft.owner != g || draft.session != g.atlasNoteSession || !draft.session.active || draft.consumed {
		return fmt.Errorf("%w: this note was opened too long ago; open it again", journal.ErrInvalidAtlasPersonal)
	}
	if draft.creating {
		fix, ok := g.CurrentSextantFix()
		if !ok || draft.context != g.CurrentSextantContext() || draft.location != g.MapState.PlayerLocation ||
			draft.original.RecordedAt != g.sampleDungeonObservation().at.Time || moonstoneJournalSite(fix) != draft.original.Position {
			return fmt.Errorf("%w: the party has moved; mark the position again", journal.ErrInvalidAtlasPersonal)
		}

		return nil
	}
	for _, record := range g.atlasNotes.Records {
		if record == draft.original {
			return nil
		}
	}

	return fmt.Errorf("%w: note changed or was deleted", journal.ErrInvalidAtlasPersonal)
}

// CommitAtlasNote edits text atomically and preserves the initial position/date.
func (g *GameState) CommitAtlasNote(draft *AtlasNoteDraft, label, description string) (journal.AtlasNoteRecord, error) {
	var absent journal.AtlasNoteRecord
	if err := g.validateAtlasDraft(draft); err != nil {
		return absent, err
	}
	for _, r := range label {
		if r < ' ' || r > '~' {
			return absent, fmt.Errorf("%w: use only letters the game can show", journal.ErrInvalidAtlasPersonal)
		}
	}
	label = strings.TrimSpace(label)
	if err := journal.ValidateAtlasNoteText(label, description); err != nil {
		return absent, err
	}
	record := draft.original
	record.Label, record.Description = label, description
	if draft.creating {
		if len(g.atlasNotes.Records) >= journal.MaxAtlasNotes || g.atlasNotes.LastID >= journal.MaxAtlasIdentity {
			return absent, fmt.Errorf("%w: the map is full of notes; edit or delete one", journal.ErrInvalidAtlasPersonal)
		}
		for _, existing := range g.atlasNotes.Records {
			if existing.Position == record.Position {
				return absent, fmt.Errorf("%w: this spot already has a note", journal.ErrInvalidAtlasPersonal)
			}
		}
		record.ID = g.atlasNotes.LastID + 1
		candidate := g.CaptureAtlasNotes()
		candidate.LastID = record.ID
		candidate.Records = append(candidate.Records, record)
		if err := journal.ValidateAtlasNotes(candidate); err != nil {
			return absent, err
		}
		g.atlasNotes = candidate
	} else {
		for i := range g.atlasNotes.Records {
			if g.atlasNotes.Records[i].ID == record.ID {
				g.atlasNotes.Records[i] = record

				break
			}
		}
	}
	draft.consumed = true
	g.atlasNoteSession = &atlasNoteSession{active: true}

	return record, nil
}

// DeleteAtlasNote requires a captured selected record, rejecting stale confirmations.
func (g *GameState) DeleteAtlasNote(draft *AtlasNoteDraft) error {
	if err := g.validateAtlasDraft(draft); err != nil {
		return err
	}
	if draft.creating {
		return fmt.Errorf("%w: cannot delete an uncommitted note", journal.ErrInvalidAtlasPersonal)
	}
	for i, record := range g.atlasNotes.Records {
		if record.ID == draft.original.ID {
			g.atlasNotes.Records = append(g.atlasNotes.Records[:i], g.atlasNotes.Records[i+1:]...)
			draft.consumed = true
			g.atlasNoteSession = &atlasNoteSession{active: true}

			return nil
		}
	}
	panic("validated note disappeared")
}

// CaptureAtlasNotes returns checkpoint-owned values; drafts are deliberately absent.
func (g *GameState) CaptureAtlasNotes() journal.AtlasNotesState {
	state := g.atlasNotes
	state.Records = append([]journal.AtlasNoteRecord(nil), state.Records...)

	return state
}

// RestoreAtlasNotes validates before mutation and invalidates outstanding drafts.
// Historical native/legacy adapters pass the zero state (no invented notes).
func (g *GameState) RestoreAtlasNotes(state journal.AtlasNotesState) error {
	if err := journal.ValidateAtlasNotes(state); err != nil {
		return fmt.Errorf("restore atlas notes: %w", err)
	}
	state.Records = append([]journal.AtlasNoteRecord(nil), state.Records...)
	g.atlasNotes = state
	g.atlasNoteSession = &atlasNoteSession{active: true}

	return nil
}
