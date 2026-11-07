package widgets

import (
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/hajimehoshi/ebiten/v2"
)

// AtlasNoteAction identifies the scene-owned authoring workflow to open.
type AtlasNoteAction uint8

// Note actions never carry arbitrary coordinates or mutate the detached dossier.
const (
	AtlasNoteMark AtlasNoteAction = iota
	AtlasNoteEdit
	AtlasNoteDelete
)

// AtlasNoteIntent contains only a stable selection, never caller-authored coordinates.
type AtlasNoteIntent struct {
	Action AtlasNoteAction
	ID     uint64
}

// SetNoteIntentHandler enables authoring controls with an explicit scene owner.
func (d *AtlasDialog) SetNoteIntentHandler(handler func(AtlasNoteIntent)) {
	if handler == nil {
		panic("atlas note intent owner is required")
	}
	d.onNoteIntent = handler
}

func (d *AtlasDialog) selectedNoteID() uint64 {
	if d.view.Overview || d.selection < 0 || d.selection >= len(d.entries) {
		return 0
	}

	return d.entries[d.selection].Note.ID
}

func (d *AtlasDialog) requestNoteIntent(key ebiten.Key) {
	if d.onNoteIntent == nil {
		return
	} // Standalone read-only atlas consumers have no authoring owner.
	if key == ebiten.KeyN {
		if !d.dossier.CurrentFix.Measured {
			d.status = d.dossier.FixUnavailable

			return
		}
		d.onNoteIntent(AtlasNoteIntent{Action: AtlasNoteMark, ID: 0})

		return
	}
	noteID := d.selectedNoteID()
	if noteID == 0 {
		return
	}
	action := AtlasNoteEdit
	if key == ebiten.KeyD {
		action = AtlasNoteDelete
	}
	d.onNoteIntent(AtlasNoteIntent{Action: action, ID: noteID})
}

// NoteCommitted changes only list selection; focus/zoom remain explicit controls.
func (d *AtlasDialog) NoteCommitted(dossier journal.AtlasDossier, noteID uint64) {
	d.view.Filters.Places = journal.AtlasNotePlaces
	d.view.Overview = false
	d.view.SelectedKey = journal.AtlasNoteKey(noteID)
	d.Refresh(dossier)
	d.detailPage = 0
	d.status = "Note saved."
}

// AtlasJournalNoteKind marks a Journal note shown on the Map for one visit from the Journal.
const AtlasJournalNoteKind = "journal note"

// Focus opens one entry from outside the Map, such as a Journal note: it shows
// the entry's category, selects it and centres on it when its position is known
// (the Map says so otherwise). Zoom stays the player's choice. It reports false
// when the category has no such entry.
func (d *AtlasDialog) Focus(key string, places journal.AtlasPlaceFilter) bool {
	d.view.Filters.Places = places
	d.view.Overview = false
	d.view.SelectedKey = key
	d.refilter()
	d.detailPage = 0
	if d.view.Overview || len(d.entries) == 0 || d.entries[d.selection].Key != key {
		return false
	}
	d.centerOnSelection()

	return true
}

// SetStatus displays concise authoring feedback in the atlas's existing status area.
func (d *AtlasDialog) SetStatus(status string) { d.status = status }
