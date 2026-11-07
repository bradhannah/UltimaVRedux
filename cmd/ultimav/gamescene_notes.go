package main

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// handleNotesIntent carries out one My Notes request. Game state owns every
// change; the Journal only shows the detached result.
func (g *GameScene) handleNotesIntent(intent widgets.NotesIntent) {
	session := g.journalSession
	if session == nil || session.child != nil || session.closing {
		return
	}
	switch intent.Action {
	case widgets.NotesNew, widgets.NotesEdit:
		g.openNoteEditor(session, intent)
	case widgets.NotesNewSection, widgets.NotesRenameSection:
		g.openNoteSectionPrompt(session, intent)
	case widgets.NotesDelete:
		state, err := g.gameState.DeletePlayerNote(intent.NoteID)
		g.notesResult(session, state, err, 0, intent.SectionID, "Note deleted.")
	case widgets.NotesToggleDone:
		state, err := g.gameState.TogglePlayerNoteDone(intent.NoteID)
		g.notesResult(session, state, err, intent.NoteID, 0, "")
	case widgets.NotesMove:
		state, err := g.gameState.MovePlayerNote(intent.NoteID, intent.SectionID)
		g.notesResult(session, state, err, intent.NoteID, 0, "Note moved.")
	case widgets.NotesDeleteSection:
		state, err := g.gameState.DeletePlayerNoteSection(intent.SectionID)
		g.notesResult(session, state, err, 0, journal.NoteSectionToDo, "Section deleted. Its notes are in To Do.")
	case widgets.NotesFold:
		state, err := g.gameState.SetPlayerNoteSectionFolded(intent.SectionID, intent.Folded)
		g.notesResult(session, state, err, intent.NoteID, intent.SectionID, "")
	}
}

func (g *GameScene) notesResult(
	session *journalSession, state journal.NotesState, err error, noteID, sectionID uint64, status string,
) {
	if err != nil {
		status = notesMessage(err)
	}
	session.dialog.NotesChanged(state, noteID, sectionID, status)
}

// noteSentenceError is a player-facing sentence the note editor shows as is.
type noteSentenceError string

func (e noteSentenceError) Error() string { return string(e) }

// notesMessage turns a notebook error into a sentence for the player.
func notesMessage(err error) string {
	if errors.Is(err, journal.ErrNotesFull) {
		return fmt.Sprintf("Notebook full: %d notes. Delete one to write another.", journal.MaxNotes)
	}
	message := strings.TrimPrefix(err.Error(), journal.ErrInvalidNotes.Error()+": ")
	if message == "" {
		return "That change could not be made."
	}

	return strings.ToUpper(message[:1]) + message[1:] + "."
}

// notePlace names where the party really is, as the Save screen does; combat,
// camping and shrines report the place they were entered from.
func (g *GameScene) notePlace() string {
	origin := g.skyOrigin()

	return playerSaveLocationLabel(int(origin.Location), int(origin.Floor))
}

func (g *GameScene) noteNow() string {
	date := g.gameState.DateTime

	return fmt.Sprintf("%d-%d-%d %02d:%02d", date.Month, date.Day, date.Year, date.Hour, date.Minute)
}

func (g *GameScene) openNoteEditor(session *journalSession, intent widgets.NotesIntent) {
	notes := g.gameState.PlayerNotes()
	heading, sectionID, place, recorded := "NEW NOTE", intent.SectionID, g.notePlace(), g.noteNow()
	initial := [2]string{"", ""}
	if intent.Action == widgets.NotesEdit {
		index := notes.NoteIndex(intent.NoteID)
		if index < 0 {
			session.dialog.NotesChanged(notes, 0, 0, "That note no longer exists.")

			return
		}
		note := notes.Notes[index]
		heading, sectionID, place = "EDIT NOTE", note.SectionID, note.Place
		recorded = fmt.Sprintf("%d-%d-%d %02d:%02d", note.RecordedAt.Month, note.RecordedAt.Day,
			note.RecordedAt.Year, note.RecordedAt.Hour, note.RecordedAt.Minute)
		initial = [2]string{note.Title, note.Text}
	}
	section := notes.SectionIndex(sectionID)
	if section < 0 {
		session.dialog.NotesChanged(notes, 0, 0, "That section no longer exists.")

		return
	}
	stamp := []widgets.EditorInfo{{Label: "Recorded", Value: recorded}}
	if place != "" {
		stamp = append(stamp, widgets.EditorInfo{Label: "Place", Value: place})
	}
	g.keyboard.PrepareForTransition(0)
	session.child = widgets.NewAtlasNoteEditor(widgets.AtlasNoteEditorConfig{
		Keyboard: g.keyboard, Cancel: g.closeNotesChild, AppendInputChars: g.appendInputChars,
		Text: &widgets.NoteEditorText{
			Heading:  heading,
			Info:     [2][]widgets.EditorInfo{{{Label: "Section", Value: notes.Sections[section].Name}}, stamp},
			Captions: [2]string{"Title (optional)", "Text"},
			Limits:   [2]int{journal.MaxNoteTitleRunes, journal.MaxNoteTextRunes},
			Initial:  initial,
			Validate: journal.ValidateNoteText,
		},
		Submit: func(title, text string) error {
			id := intent.NoteID
			var state journal.NotesState
			var err error
			if intent.Action == widgets.NotesEdit {
				state, err = g.gameState.EditPlayerNote(id, title, text)
			} else {
				state, id, err = g.gameState.AddPlayerNote(sectionID, title, text, place)
			}
			if err != nil {
				return noteSentenceError(notesMessage(err))
			}
			session.dialog.NotesChanged(state, id, 0, "Note saved.")
			g.closeNotesChild()

			return nil
		},
	})
	g.dialogStack.PushModalDialog(session.child)
}

func (g *GameScene) openNoteSectionPrompt(session *journalSession, intent widgets.NotesIntent) {
	notes := g.gameState.PlayerNotes()
	title, initial := "NEW SECTION", ""
	if intent.Action == widgets.NotesRenameSection {
		index := notes.SectionIndex(intent.SectionID)
		if index < 0 {
			session.dialog.NotesChanged(notes, 0, 0, "That section no longer exists.")

			return
		}
		title, initial = "RENAME SECTION", notes.Sections[index].Name
	} else if len(notes.Sections) >= journal.MaxNoteSections {
		session.dialog.NotesChanged(notes, 0, intent.SectionID,
			fmt.Sprintf("You already have the most sections allowed (%d).", journal.MaxNoteSections))

		return
	}
	g.keyboard.PrepareForTransition(0)
	session.child = widgets.NewSaveDescriptionEntryModal(widgets.SaveDescriptionEntryModalConfig{
		Title: title, Prompt: "Name", Notice: "", InitialText: initial, Keyboard: g.keyboard,
		MaxRunes: journal.MaxNoteSectionNameRunes, IsKeyPressed: nil,
		Validate: func(value string) string { return noteSectionNameProblem(notes, intent.SectionID, value) },
		OnResult: func(value string, submitted bool) {
			g.closeNotesChild()
			if !submitted {
				return
			}
			if intent.Action == widgets.NotesRenameSection {
				state, err := g.gameState.RenamePlayerNoteSection(intent.SectionID, value)
				g.notesResult(session, state, err, 0, intent.SectionID, "Section renamed.")

				return
			}
			state, id, err := g.gameState.AddPlayerNoteSection(value)
			g.notesResult(session, state, err, 0, id, "Section added.")
		},
	})
	g.dialogStack.PushModalDialog(session.child)
}

// noteSectionNameProblem explains a refused section name in words, or returns "".
func noteSectionNameProblem(notes journal.NotesState, renaming uint64, value string) string {
	name := strings.TrimSpace(value)
	if err := journal.ValidateNoteSectionName(name); err != nil {
		return fmt.Sprintf("Use 1 to %d letters, numbers or punctuation.", journal.MaxNoteSectionNameRunes)
	}
	for _, section := range notes.Sections {
		if section.ID != renaming && strings.EqualFold(section.Name, name) {
			return "You already have a section called " + section.Name + "."
		}
	}

	return ""
}

// closeNotesChild removes the editor or name box and returns focus to the Journal.
func (g *GameScene) closeNotesChild() {
	session := g.journalSession
	if session == nil || session.child == nil {
		return
	}
	g.dialogStack.MustRemoveWidgetInstance(session.child)
	session.child = nil
	g.keyboard.PrepareForTransition(0)
}
