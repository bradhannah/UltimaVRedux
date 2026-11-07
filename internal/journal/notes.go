package journal

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"unicode/utf8"
)

const (
	// MaxNotes bounds the player's notebook in one saved game.
	MaxNotes = 512
	// MaxNoteSections includes the built-in To Do and Places sections.
	MaxNoteSections = 16
	// MaxNoteTitleRunes matches a Map label, so any note could later be pinned.
	MaxNoteTitleRunes = MaxAtlasNoteLabelRunes
	// MaxNoteTextRunes includes intentional paragraph line feeds.
	MaxNoteTextRunes = 2000
	// MaxNoteSectionNameRunes keeps a heading and its count on one list row.
	MaxNoteSectionNameRunes = 14
	// MaxNotePlaceRunes bounds the stamped place label.
	MaxNotePlaceRunes = 64
	// NoteSectionToDo identifies the built-in To Do section; it can be renamed.
	NoteSectionToDo uint64 = 1
	// NoteSectionPlaces identifies the built-in Places section; the Map relies on it.
	NoteSectionPlaces uint64 = 2
	// NoteSectionPlacesName is fixed because the Traveller's Map relies on it.
	NoteSectionPlacesName = "Places"
	noteSectionToDoName   = "To Do"
	builtInNoteSections   = 2
)

// ErrInvalidNotes identifies malformed notebook state or authoring input.
var ErrInvalidNotes = errors.New("invalid player notes")

// ErrNotesFull means the player must delete a note before writing another.
var ErrNotesFull = errors.New("notebook is full")

// NoteSection is one player-visible divider. Order is the stored order.
type NoteSection struct {
	ID     uint64 `json:"id"`
	Name   string `json:"name"`
	Folded bool   `json:"folded"`
}

// NoteRecord is one player-written note. Place and RecordedAt are stamped once.
type NoteRecord struct {
	ID         uint64   `json:"id"`
	SectionID  uint64   `json:"sectionId"`
	Title      string   `json:"title"`
	Text       string   `json:"text"`
	Place      string   `json:"place"`
	RecordedAt GameTime `json:"recordedAt"`
	Done       bool     `json:"done"`
}

// NotesState is checkpointed player authorship. The zero value is a fresh
// notebook, so older saves and imported games need no migration data.
type NotesState struct {
	LastSectionID uint64        `json:"lastSectionId"`
	LastNoteID    uint64        `json:"lastNoteId"`
	Sections      []NoteSection `json:"sections"`
	Notes         []NoteRecord  `json:"notes"`
}

// DefaultNotes returns the notebook every game starts with.
func DefaultNotes() NotesState {
	return NotesState{
		LastSectionID: NoteSectionPlaces,
		LastNoteID:    0,
		Sections: []NoteSection{
			{ID: NoteSectionToDo, Name: noteSectionToDoName, Folded: false},
			{ID: NoteSectionPlaces, Name: NoteSectionPlacesName, Folded: false},
		},
		Notes: []NoteRecord{},
	}
}

// Fresh reports the canonical zero notebook that older saves carry.
func (s NotesState) Fresh() bool {
	return len(s.Sections) == 0 && len(s.Notes) == 0 && s.LastSectionID == 0 && s.LastNoteID == 0
}

// Clone returns a detached copy that preserves nil and empty collections.
func (s NotesState) Clone() NotesState {
	s.Sections = slices.Clone(s.Sections)
	s.Notes = slices.Clone(s.Notes)

	return s
}

// Normalized returns a detached copy in which a fresh notebook has its built-in sections.
func (s NotesState) Normalized() NotesState {
	if s.Fresh() {
		return DefaultNotes()
	}

	return s.Clone()
}

// SectionIndex returns the stored position of a section, or -1.
func (s NotesState) SectionIndex(id uint64) int {
	return slices.IndexFunc(s.Sections, func(section NoteSection) bool { return section.ID == id })
}

// NoteIndex returns the stored position of a note, or -1.
func (s NotesState) NoteIndex(id uint64) int {
	return slices.IndexFunc(s.Notes, func(note NoteRecord) bool { return note.ID == id })
}

// ListTitle is the title, or the first line of an untitled note.
func (n NoteRecord) ListTitle() string {
	if n.Title != "" {
		return n.Title
	}
	first, _, _ := strings.Cut(strings.TrimSpace(n.Text), "\n")

	return strings.TrimSpace(first)
}

// ValidateNotes checks the whole notebook atomically; the zero value is valid.
func ValidateNotes(state NotesState) error {
	if state.Fresh() {
		return nil
	}
	if state.LastSectionID > MaxAtlasIdentity || state.LastNoteID > MaxAtlasIdentity {
		return fmt.Errorf("%w: identity watermark", ErrInvalidNotes)
	}
	if err := validateNoteSections(state); err != nil {
		return err
	}

	return validateNoteRecords(state)
}

func validateNoteSections(state NotesState) error {
	sections := state.Sections
	if len(sections) < builtInNoteSections || len(sections) > MaxNoteSections ||
		sections[0].ID != NoteSectionToDo || sections[1].ID != NoteSectionPlaces ||
		sections[1].Name != NoteSectionPlacesName {
		return fmt.Errorf("%w: sections start with To Do and Places, at most %d", ErrInvalidNotes, MaxNoteSections)
	}
	ids := make(map[uint64]bool, len(sections))
	names := make(map[string]bool, len(sections))
	for _, section := range sections {
		key := strings.ToLower(section.Name)
		if section.ID == 0 || section.ID > state.LastSectionID || ids[section.ID] || names[key] {
			return fmt.Errorf("%w: duplicate or unallocated section", ErrInvalidNotes)
		}
		if err := ValidateNoteSectionName(section.Name); err != nil {
			return err
		}
		ids[section.ID], names[key] = true, true
	}

	return nil
}

func validateNoteRecords(state NotesState) error {
	if len(state.Notes) > MaxNotes {
		return fmt.Errorf("%w: at most %d notes", ErrInvalidNotes, MaxNotes)
	}
	var previous uint64
	for _, note := range state.Notes {
		if note.ID <= previous || note.ID > state.LastNoteID || state.SectionIndex(note.SectionID) < 0 {
			return fmt.Errorf("%w: unordered note identity or missing section", ErrInvalidNotes)
		}
		previous = note.ID
		if !validAtlasReceiptTime(OptionalGameTime{Known: true, Time: note.RecordedAt}) ||
			strings.TrimSpace(note.Place) != note.Place || utf8.RuneCountInString(note.Place) > MaxNotePlaceRunes ||
			!printableNoteText(note.Place, false) {
			return fmt.Errorf("%w: note date or place", ErrInvalidNotes)
		}
		if err := ValidateNoteText(note.Title, note.Text); err != nil {
			return err
		}
	}

	return nil
}

// ValidateNoteSectionName accepts 1-14 printable characters without edge spaces.
func ValidateNoteSectionName(name string) error {
	if strings.TrimSpace(name) == "" || strings.TrimSpace(name) != name ||
		utf8.RuneCountInString(name) > MaxNoteSectionNameRunes || !printableNoteText(name, false) {
		return fmt.Errorf("%w: a section name needs 1-%d printable characters", ErrInvalidNotes, MaxNoteSectionNameRunes)
	}

	return nil
}

// ValidateNoteText accepts an optional single-line title and paragraphs of text.
// A note needs a title or some text, in the game's printable ASCII repertoire.
func ValidateNoteText(title, text string) error {
	if strings.TrimSpace(title) != title || utf8.RuneCountInString(title) > MaxNoteTitleRunes ||
		utf8.RuneCountInString(text) > MaxNoteTextRunes {
		return fmt.Errorf("%w: title at most %d characters; text at most %d",
			ErrInvalidNotes, MaxNoteTitleRunes, MaxNoteTextRunes)
	}
	if title == "" && strings.TrimSpace(text) == "" {
		return fmt.Errorf("%w: write a title or some text", ErrInvalidNotes)
	}
	if !printableNoteText(title, false) || !printableNoteText(text, true) {
		return fmt.Errorf("%w: use printable game-font characters", ErrInvalidNotes)
	}

	return nil
}

func printableNoteText(value string, paragraphs bool) bool {
	for _, char := range value {
		if paragraphs && char == '\n' {
			continue
		}
		if char < ' ' || char > '~' {
			return false
		}
	}

	return true
}
