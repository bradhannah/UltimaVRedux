package journal

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestNotesValidationLimitsAndRules(t *testing.T) {
	require.NoError(t, ValidateNotes(NotesState{}), "the zero value is a fresh notebook")
	require.NoError(t, ValidateNotes(DefaultNotes()))
	valid := DefaultNotes()
	valid.LastNoteID = 1
	valid.Notes = []NoteRecord{{ID: 1, SectionID: NoteSectionToDo, Title: "", Text: "Ask Iolo\nabout the Shards",
		Place: "Britain", RecordedAt: GameTime{Year: 139, Month: 4, Day: 12, Hour: 9, Minute: 15}, Done: false}}
	require.NoError(t, ValidateNotes(valid))
	require.Equal(t, "Ask Iolo", valid.Notes[0].ListTitle(), "an untitled note lists its first line")
	for _, site := range []SiteLocation{{Map: references.Britain}, {Map: references.Deceit},
		{Map: references.BritanniaUnderworld, X: 17, Y: 29}} {
		linked := valid.Clone()
		linked.Notes[0].Site = &site
		require.NoError(t, ValidateNotes(linked), "%+v", site)
	}
	valid.Notes[0].Site = &SiteLocation{Map: references.Britain}
	copied := valid.Clone()
	copied.Notes[0].Site.Map = references.Yew
	require.Equal(t, references.Britain, valid.Notes[0].Site.Map, "Clone detaches the map link")
	require.False(t, valid.Notes[0].SurfaceSpot())

	for name, broken := range map[string]func(*NotesState){
		"Places renamed":       func(s *NotesState) { s.Sections[1].Name = "Spots" },
		"note in no section":   func(s *NotesState) { s.Notes[0].SectionID = 9 },
		"duplicate name":       func(s *NotesState) { s.Sections = append(s.Sections, NoteSection{ID: 3, Name: "to do"}) },
		"unallocated section":  func(s *NotesState) { s.Sections = append(s.Sections, NoteSection{ID: 7, Name: "Clues"}) },
		"long section name":    func(s *NotesState) { s.Sections[0].Name = strings.Repeat("x", MaxNoteSectionNameRunes+1) },
		"long title":           func(s *NotesState) { s.Notes[0].Title = strings.Repeat("x", MaxNoteTitleRunes+1) },
		"long text":            func(s *NotesState) { s.Notes[0].Text = strings.Repeat("x", MaxNoteTextRunes+1) },
		"empty note":           func(s *NotesState) { s.Notes[0].Text = "  " },
		"not the game font":    func(s *NotesState) { s.Notes[0].Text = "café" },
		"impossible date":      func(s *NotesState) { s.Notes[0].RecordedAt.Month = 14 },
		"identity beyond mark": func(s *NotesState) { s.LastNoteID = 0 },
		"room inside a town":   func(s *NotesState) { s.Notes[0].Site = &SiteLocation{Map: references.Britain, X: 3} },
		"link to a battle map": func(s *NotesState) { s.Notes[0].Site = &SiteLocation{Map: references.CombatLocation} },
		"underworld spot":      func(s *NotesState) { s.Notes[0].Site = &SiteLocation{Map: references.BritanniaUnderworld, Floor: -1} },
	} {
		candidate := valid.Clone()
		candidate.Sections = append([]NoteSection(nil), valid.Sections...)
		broken(&candidate)
		require.ErrorIs(t, ValidateNotes(candidate), ErrInvalidNotes, name)
	}
}
