package game_state

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// PlayerNotes returns a detached notebook; a fresh one shows the built-in sections.
func (g *GameState) PlayerNotes() journal.NotesState { return g.playerNotes.Normalized() }

// updatePlayerNotes edits a candidate and replaces the notebook only when the
// whole result validates, like CommitAtlasNote. Failures leave it unchanged.
func (g *GameState) updatePlayerNotes(edit func(*journal.NotesState) error) (journal.NotesState, error) {
	candidate := g.playerNotes.Normalized()
	if err := edit(&candidate); err != nil {
		return g.PlayerNotes(), err
	}
	if err := journal.ValidateNotes(candidate); err != nil {
		return g.PlayerNotes(), err
	}
	g.playerNotes = candidate

	return g.PlayerNotes(), nil
}

func (g *GameState) playerNoteTime() journal.GameTime {
	return journal.GameTime{Year: int(g.DateTime.Year), Month: int(g.DateTime.Month), Day: int(g.DateTime.Day),
		Hour: int(g.DateTime.Hour), Minute: int(g.DateTime.Minute)}
}

func playerNoteIndex(state *journal.NotesState, id uint64) (int, error) {
	index := state.NoteIndex(id)
	if index < 0 {
		return index, fmt.Errorf("%w: note no longer exists", journal.ErrInvalidNotes)
	}

	return index, nil
}

func playerSectionIndex(state *journal.NotesState, id uint64) (int, error) {
	index := state.SectionIndex(id)
	if index < 0 {
		return index, fmt.Errorf("%w: section no longer exists", journal.ErrInvalidNotes)
	}

	return index, nil
}

// PlayerNoteSite is where a note written at origin links on the Traveller's Map:
// the settlement or dungeon the party is in, or outdoors the spot a sextant
// could measure right now. The Map never learns an unmeasured position, so a
// note written outdoors without a sextant reading, underground or in a battle
// away from a settlement has no link (nil). origin is where the party really is
// (the place a battle, camp or shrine was entered from).
func (g *GameState) PlayerNoteSite(origin references.PlayerLocation) *journal.SiteLocation {
	if origin.Location >= references.Moonglow && origin.Location <= references.Doom {
		return &journal.SiteLocation{Map: origin.Location, Floor: 0, X: 0, Y: 0}
	}
	if origin != g.MapState.PlayerLocation {
		return nil
	}
	fix, ok := g.CurrentSextantFix()
	if !ok {
		return nil
	}
	if site := moonstoneJournalSite(fix); journal.ValidAtlasSurfacePosition(site) {
		return &site
	}

	return nil
}

// AtlasPlaceKey names the Traveller's Map entry for a settlement or dungeon. The
// entry exists only once the Map knows the place, so callers look it up.
func (g *GameState) AtlasPlaceKey(location references.Location) string {
	if location.GetMapType() == references.DungeonMapType {
		return "dungeon-" + location.String()
	}
	for _, point := range references.SuppliedAtlasChart(g.GameReferences.LocationReferences.WorldLocations) {
		if point.SiteKey != 0 && references.Location(point.SiteKey) == location {
			return point.Key
		}
	}

	return fmt.Sprintf("observed-%d", int(location))
}

// AddPlayerNote writes a note into a section, stamped with the game time, place
// and Map link (see PlayerNoteSite).
func (g *GameState) AddPlayerNote(
	sectionID uint64, title, text, place string, site *journal.SiteLocation,
) (journal.NotesState, uint64, error) {
	var id uint64
	state, err := g.updatePlayerNotes(func(notes *journal.NotesState) error {
		if len(notes.Notes) >= journal.MaxNotes || notes.LastNoteID >= journal.MaxAtlasIdentity {
			return fmt.Errorf("%w: %d notes", journal.ErrNotesFull, journal.MaxNotes)
		}
		if _, err := playerSectionIndex(notes, sectionID); err != nil {
			return err
		}
		notes.LastNoteID++
		id = notes.LastNoteID
		notes.Notes = append(notes.Notes, journal.NoteRecord{ID: id, SectionID: sectionID,
			Title: strings.TrimSpace(title), Text: text, Place: strings.TrimSpace(place),
			RecordedAt: g.playerNoteTime(), Done: false, Site: nil})
		if site != nil {
			copied := *site
			notes.Notes[len(notes.Notes)-1].Site = &copied
		}

		return nil
	})

	return state, id, err
}

// EditPlayerNote replaces the words; the original date and place stay.
func (g *GameState) EditPlayerNote(id uint64, title, text string) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		index, err := playerNoteIndex(notes, id)
		if err == nil {
			notes.Notes[index].Title, notes.Notes[index].Text = strings.TrimSpace(title), text
		}

		return err
	})
}

// DeletePlayerNote removes one note permanently.
func (g *GameState) DeletePlayerNote(id uint64) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		index, err := playerNoteIndex(notes, id)
		if err == nil {
			notes.Notes = append(notes.Notes[:index], notes.Notes[index+1:]...)
		}

		return err
	})
}

// MovePlayerNote files a note under another section.
func (g *GameState) MovePlayerNote(id, sectionID uint64) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		index, err := playerNoteIndex(notes, id)
		if err != nil {
			return err
		}
		if _, err = playerSectionIndex(notes, sectionID); err == nil {
			notes.Notes[index].SectionID = sectionID
		}

		return err
	})
}

// TogglePlayerNoteDone ticks or unticks a note.
func (g *GameState) TogglePlayerNoteDone(id uint64) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		index, err := playerNoteIndex(notes, id)
		if err == nil {
			notes.Notes[index].Done = !notes.Notes[index].Done
		}

		return err
	})
}

// AddPlayerNoteSection appends a section of the player's own.
func (g *GameState) AddPlayerNoteSection(name string) (journal.NotesState, uint64, error) {
	var id uint64
	state, err := g.updatePlayerNotes(func(notes *journal.NotesState) error {
		if len(notes.Sections) >= journal.MaxNoteSections || notes.LastSectionID >= journal.MaxAtlasIdentity {
			return fmt.Errorf("%w: at most %d sections", journal.ErrInvalidNotes, journal.MaxNoteSections)
		}
		notes.LastSectionID++
		id = notes.LastSectionID
		notes.Sections = append(notes.Sections, journal.NoteSection{ID: id, Name: strings.TrimSpace(name), Folded: false})

		return nil
	})

	return state, id, err
}

// RenamePlayerNoteSection renames any section except Places.
func (g *GameState) RenamePlayerNoteSection(id uint64, name string) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		if id == journal.NoteSectionPlaces {
			return fmt.Errorf("%w: Places keeps its name", journal.ErrInvalidNotes)
		}
		index, err := playerSectionIndex(notes, id)
		if err == nil {
			notes.Sections[index].Name = strings.TrimSpace(name)
		}

		return err
	})
}

// DeletePlayerNoteSection removes a player-made section; its notes move to To Do.
func (g *GameState) DeletePlayerNoteSection(id uint64) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		if id == journal.NoteSectionToDo || id == journal.NoteSectionPlaces {
			return fmt.Errorf("%w: To Do and Places can't be deleted", journal.ErrInvalidNotes)
		}
		index, err := playerSectionIndex(notes, id)
		if err != nil {
			return err
		}
		notes.Sections = append(notes.Sections[:index], notes.Sections[index+1:]...)
		for i := range notes.Notes {
			if notes.Notes[i].SectionID == id {
				notes.Notes[i].SectionID = journal.NoteSectionToDo
			}
		}

		return nil
	})
}

// SetPlayerNoteSectionFolded remembers whether a section is folded in the list.
func (g *GameState) SetPlayerNoteSectionFolded(id uint64, folded bool) (journal.NotesState, error) {
	return g.updatePlayerNotes(func(notes *journal.NotesState) error {
		index, err := playerSectionIndex(notes, id)
		if err == nil {
			notes.Sections[index].Folded = folded
		}

		return err
	})
}

// CapturePlayerNotes returns checkpoint-owned values; a fresh notebook stays zero.
func (g *GameState) CapturePlayerNotes() journal.NotesState { return g.playerNotes.Clone() }

// RestorePlayerNotes validates before mutation.
func (g *GameState) RestorePlayerNotes(state journal.NotesState) error {
	if err := journal.ValidateNotes(state); err != nil {
		return fmt.Errorf("restore player notes: %w", err)
	}
	g.playerNotes = state.Clone()

	return nil
}
