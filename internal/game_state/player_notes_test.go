package game_state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
)

func TestPlayerNotesStampEditsAndSurviveCaptureRestore(t *testing.T) {
	g := &GameState{}
	g.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 12, Hour: 9, Minute: 15}
	require.Equal(t, journal.DefaultNotes(), g.PlayerNotes(), "a fresh game shows To Do and Places")
	require.True(t, g.CapturePlayerNotes().Fresh(), "an untouched notebook saves as the zero value")

	_, sectionID, err := g.AddPlayerNoteSection(" Clues ")
	require.NoError(t, err)
	notes, noteID, err := g.AddPlayerNote(sectionID, "Hermit", "Watch the moongates.", "Yew")
	require.NoError(t, err)
	note := notes.Notes[notes.NoteIndex(noteID)]
	require.Equal(t, journal.GameTime{Year: 139, Month: 4, Day: 12, Hour: 9, Minute: 15}, note.RecordedAt)
	require.Equal(t, "Clues", notes.Sections[notes.SectionIndex(sectionID)].Name)

	g.DateTime.Hour = 20
	notes, err = g.EditPlayerNote(noteID, "Hermit's riddle", "Moongates at night.")
	require.NoError(t, err)
	require.Equal(t, 9, notes.Notes[0].RecordedAt.Hour, "editing keeps the original stamp")
	_, err = g.TogglePlayerNoteDone(noteID)
	require.NoError(t, err)
	_, err = g.RenamePlayerNoteSection(journal.NoteSectionPlaces, "Spots")
	require.ErrorIs(t, err, journal.ErrInvalidNotes)
	notes, err = g.DeletePlayerNoteSection(sectionID)
	require.NoError(t, err)
	require.Equal(t, journal.NoteSectionToDo, notes.Notes[0].SectionID, "a deleted section's notes move to To Do")
	require.True(t, notes.Notes[0].Done)

	saved := g.CapturePlayerNotes()
	restored := &GameState{}
	require.NoError(t, restored.RestorePlayerNotes(saved))
	require.Equal(t, g.PlayerNotes(), restored.PlayerNotes())
	saved.Notes[0].SectionID = 99
	require.Error(t, restored.RestorePlayerNotes(saved), "an invalid notebook is refused")
	require.Equal(t, g.PlayerNotes(), restored.PlayerNotes(), "and leaves the restored notebook unchanged")
}
