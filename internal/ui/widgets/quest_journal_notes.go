package widgets

import (
	"cmp"
	"fmt"
	"image"
	stdcolor "image/color"
	"slices"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

// NotesAction identifies a My Notes request. The scene owns every change.
type NotesAction uint8

// My Notes requests carry stable identities, never list positions.
const (
	NotesNew NotesAction = iota
	NotesEdit
	NotesDelete
	NotesToggleDone
	NotesMove
	NotesNewSection
	NotesRenameSection
	NotesDeleteSection
	NotesFold
	NotesShowOnMap
)

// NotesIntent is one My Notes request for the scene to carry out. MapNote means
// NoteID names a Traveller's Map note, which Places lists beside Journal notes.
type NotesIntent struct {
	Action    NotesAction
	NoteID    uint64
	SectionID uint64
	Folded    bool
	MapNote   bool
}

// notesDoneLabel names a ticked note in rows, details and the footer.
const notesDoneLabel = "Done"

type notesConfirm uint8

const (
	notesConfirmNone notesConfirm = iota
	notesConfirmNote
	notesConfirmMapNote
	notesConfirmSection
)

// notesRow is one list row: a section heading, a Journal note under it, or a
// Map note under Places. note and mapNote index their lists, or are -1.
type notesRow struct{ section, note, mapNote int }

func (r notesRow) heading() bool { return r.note < 0 && r.mapNote < 0 }

// notesState is the My Notes part of the Journal; the scene owns the notebook
// and the Map notes.
type notesState struct {
	notes    journal.NotesState
	mapNotes []journal.AtlasNoteRecord
	rows     []notesRow
	onIntent func(NotesIntent)
	status   string
	confirm  notesConfirm
	moving   bool
	// hiddenDone holds the notes already done when the Journal opened. They stay
	// out of the list unless showDone; a note ticked during this visit stays in
	// view, so a mistaken tick is easy to undo.
	hiddenDone map[uint64]bool
	showDone   bool
	moveChoice int
}

func (d *QuestJournalDialog) notesOnly() bool { return len(d.tabs) == 1 }

func (d *QuestJournalDialog) initNotes(notes journal.NotesState, mapNotes []journal.AtlasNoteRecord,
	onIntent func(NotesIntent)) {
	view := &d.notesView
	view.notes, view.mapNotes, view.onIntent = notes.Normalized(), slices.Clone(mapNotes), onIntent
	view.hiddenDone = make(map[uint64]bool)
	for _, note := range view.notes.Notes {
		if note.Done {
			view.hiddenDone[note.ID] = true
		}
	}
	d.rebuildNoteRows()
}

// NotesChanged replaces the detached notebook after the scene applied a request.
// A nonzero note or section identity is selected; status explains the result.
func (d *QuestJournalDialog) NotesChanged(notes journal.NotesState, noteID, sectionID uint64, status string) {
	view := &d.notesView
	view.notes = notes.Normalized()
	for id := range view.hiddenDone {
		if index := view.notes.NoteIndex(id); index < 0 || !view.notes.Notes[index].Done {
			delete(view.hiddenDone, id)
		}
	}
	view.status = status
	view.confirm, view.moving = notesConfirmNone, false
	d.rebuildNoteRows()
	if row := d.noteRowFor(noteID, sectionID); row >= 0 {
		d.selections[QuestJournalNotesTab] = row
	}
	d.notesListChanged()
}

// MapNotesChanged replaces the Map notes listed in Places after the scene edited
// one. A nonzero mapNoteID is selected, otherwise the Places heading.
func (d *QuestJournalDialog) MapNotesChanged(records []journal.AtlasNoteRecord, mapNoteID uint64, status string) {
	view := &d.notesView
	view.mapNotes = slices.Clone(records)
	view.status, view.confirm = status, notesConfirmNone
	d.rebuildNoteRows()
	row := d.mapNoteRowFor(mapNoteID)
	if row < 0 {
		row = d.noteRowFor(0, journal.NoteSectionPlaces)
	}
	if row >= 0 {
		d.selections[QuestJournalNotesTab] = row
	}
	d.notesListChanged()
}

// SetNotesStatus shows a sentence in the detail pane, such as why the Map cannot
// show a note.
func (d *QuestJournalDialog) SetNotesStatus(status string) {
	d.notesView.status = status
	d.cachedDetailPages = nil
}

func (d *QuestJournalDialog) notesListChanged() {
	d.clampCurrentSelection()
	d.cachedDetailPages = nil
	d.detailPage = 0
	d.buildSearchEntries()
}

func (d *QuestJournalDialog) rebuildNoteRows() {
	view := &d.notesView
	view.rows = view.rows[:0]
	for sectionIndex, section := range view.notes.Sections {
		view.rows = append(view.rows, notesRow{section: sectionIndex, note: -1, mapNote: -1})
		if section.Folded {
			continue
		}
		start := len(view.rows)
		for noteIndex, note := range view.notes.Notes {
			if note.SectionID == section.ID && (view.showDone || !view.hiddenDone[note.ID]) {
				view.rows = append(view.rows, notesRow{section: sectionIndex, note: noteIndex, mapNote: -1})
			}
		}
		if section.ID == journal.NoteSectionPlaces {
			for mapIndex := range view.mapNotes {
				view.rows = append(view.rows, notesRow{section: sectionIndex, note: -1, mapNote: mapIndex})
			}
		}
		slices.SortStableFunc(view.rows[start:], view.compareNoteRows)
	}
}

// compareNoteRows lists newest first; ticked notes sink below the rest of their section.
func (view *notesState) compareNoteRows(left, right notesRow) int {
	leftDone, rightDone := view.rowDone(left), view.rowDone(right)
	if leftDone != rightDone {
		if leftDone {
			return 1
		}

		return -1
	}
	leftAt, leftID := view.rowStamp(left)
	rightAt, rightID := view.rowStamp(right)
	if order := compareNoteTimes(rightAt, leftAt); order != 0 {
		return order
	}

	return cmp.Compare(rightID, leftID)
}

func (view *notesState) rowDone(row notesRow) bool {
	return row.note >= 0 && view.notes.Notes[row.note].Done
}

func (view *notesState) rowStamp(row notesRow) (journal.GameTime, uint64) {
	if row.mapNote >= 0 {
		note := view.mapNotes[row.mapNote]

		return note.RecordedAt, note.ID
	}
	note := view.notes.Notes[row.note]

	return note.RecordedAt, note.ID
}

func compareNoteTimes(left, right journal.GameTime) int {
	return cmp.Or(cmp.Compare(left.Year, right.Year), cmp.Compare(left.Month, right.Month),
		cmp.Compare(left.Day, right.Day), cmp.Compare(left.Hour, right.Hour), cmp.Compare(left.Minute, right.Minute))
}

func (d *QuestJournalDialog) noteRowFor(noteID, sectionID uint64) int {
	for index, row := range d.notesView.rows {
		if noteID != 0 && row.note >= 0 && d.notesView.notes.Notes[row.note].ID == noteID {
			return index
		}
		if noteID == 0 && sectionID != 0 && row.heading() && d.notesView.notes.Sections[row.section].ID == sectionID {
			return index
		}
	}

	return -1
}

func (d *QuestJournalDialog) mapNoteRowFor(mapNoteID uint64) int {
	if mapNoteID == 0 {
		return -1
	}

	return slices.IndexFunc(d.notesView.rows, func(row notesRow) bool {
		return row.mapNote >= 0 && d.notesView.mapNotes[row.mapNote].ID == mapNoteID
	})
}

// keepNotesSelection rebuilds the rows and keeps the selected note, or its section.
func (d *QuestJournalDialog) keepNotesSelection() {
	view, row := &d.notesView, d.selectedNotesRow()
	sectionID := view.notes.Sections[row.section].ID
	var noteID, mapNoteID uint64
	if row.note >= 0 {
		noteID = view.notes.Notes[row.note].ID
	}
	if row.mapNote >= 0 {
		mapNoteID = view.mapNotes[row.mapNote].ID
	}
	d.rebuildNoteRows()
	index := max(d.mapNoteRowFor(mapNoteID), d.noteRowFor(noteID, 0))
	if index < 0 {
		index = d.noteRowFor(0, sectionID)
	}
	if index >= 0 {
		d.selections[QuestJournalNotesTab] = index
	}
	d.cachedDetailPages = nil
}

// selectedNotesRow returns the selected row; the list always has two headings.
func (d *QuestJournalDialog) selectedNotesRow() notesRow {
	rows := d.notesView.rows
	if len(rows) == 0 {
		return notesRow{section: 0, note: -1, mapNote: -1}
	}

	return rows[max(0, min(d.selections[QuestJournalNotesTab], len(rows)-1))]
}

func (d *QuestJournalDialog) selectedSection() journal.NoteSection {
	return d.notesView.notes.Sections[d.selectedNotesRow().section]
}

func (d *QuestJournalDialog) sectionNoteCounts(sectionID uint64) (int, int) {
	total, done := 0, 0
	for _, note := range d.notesView.notes.Notes {
		if note.SectionID == sectionID {
			total++
			if note.Done {
				done++
			}
		}
	}
	if sectionID == journal.NoteSectionPlaces {
		total += len(d.notesView.mapNotes)
	}

	return total, done
}

// mapNoteTitle is the Map label, or the first line of an unlabelled description.
func mapNoteTitle(note journal.AtlasNoteRecord) string {
	if note.Label != "" {
		return note.Label
	}
	first, _, _ := strings.Cut(strings.TrimSpace(note.Description), "\n")

	return strings.TrimSpace(first)
}

func (d *QuestJournalDialog) notesRowText(index int) string {
	row := d.notesView.rows[index]
	switch {
	case row.mapNote >= 0:
		return "  [Map] " + mapNoteTitle(d.notesView.mapNotes[row.mapNote])
	case row.note >= 0:
		note := d.notesView.notes.Notes[row.note]
		// Prefix the state, as Excerpts does, so a clipped title still says it.
		if note.Done {
			return "  [Done] " + note.ListTitle()
		}

		return "  " + note.ListTitle()
	}
	section := d.notesView.notes.Sections[row.section]
	mark := "-"
	if section.Folded {
		mark = "+"
	}
	total, _ := d.sectionNoteCounts(section.ID)

	return fmt.Sprintf("%s %s (%d)", mark, strings.ToUpper(section.Name), total)
}

func (d *QuestJournalDialog) notesRowColor(index int) stdcolor.Color {
	row := d.notesView.rows[index]
	switch {
	case row.heading():
		return DialogLabelColor
	case d.notesView.rowDone(row):
		return DialogEmptyColor
	default:
		return DialogItemNameColor
	}
}

func (d *QuestJournalDialog) notesDetailLines() []detailLine {
	var lines []detailLine
	if d.notesView.status != "" {
		lines = append(lines, detailLine{label: "", value: d.notesView.status, color: DialogLabelColor, header: false},
			blankDetailLine())
	}
	row := d.selectedNotesRow()
	section := d.notesView.notes.Sections[row.section]
	switch {
	case row.mapNote >= 0:
		return append(lines, mapNoteDetailLines(d.notesView.mapNotes[row.mapNote])...)
	case row.heading():
		return append(lines, d.sectionDetailLines(section)...)
	}
	note := d.notesView.notes.Notes[row.note]
	if note.Title != "" {
		lines = append(lines, detailLine{label: "", value: note.Title, color: DialogHeaderColor, header: true})
	}
	for paragraph := range strings.SplitSeq(note.Text, "\n") {
		lines = append(lines, detailLine{label: "", value: paragraph, color: DialogValueColor, header: false})
	}
	lines = append(lines, noteStampLines(section.Name, note.RecordedAt)...)
	if note.Place != "" {
		lines = append(lines, detailLine{label: "Place", value: note.Place, color: DialogValueColor, header: false})
	}
	if note.SurfaceSpot() {
		lines = append(lines, detailLine{label: atlasCoordinatesLabel, value: atlasCoordinates(*note.Site),
			color: DialogValueColor, header: false})
	}
	if note.Done {
		lines = append(lines, detailLine{label: "State", value: notesDoneLabel, color: DialogEmptyColor, header: false})
	}

	return lines
}

func mapNoteDetailLines(note journal.AtlasNoteRecord) []detailLine {
	var lines []detailLine
	if note.Label != "" {
		lines = append(lines, detailLine{label: "", value: note.Label, color: DialogHeaderColor, header: true})
	}
	if note.Description != "" {
		for paragraph := range strings.SplitSeq(note.Description, "\n") {
			lines = append(lines, detailLine{label: "", value: paragraph, color: DialogValueColor, header: false})
		}
	}

	lines = append(lines, noteStampLines(journal.NoteSectionPlacesName, note.RecordedAt)...)

	return append(lines,
		detailLine{label: atlasCoordinatesLabel, value: atlasCoordinates(note.Position), color: DialogValueColor,
			header: false},
		blankDetailLine(),
		detailLine{label: "", value: "A note on the Traveller's Map. It stays in Places.",
			color: DialogControlHintColor, header: false})
}

func (d *QuestJournalDialog) sectionDetailLines(section journal.NoteSection) []detailLine {
	total, done := d.sectionNoteCounts(section.ID)
	about := "A section you made. Deleting it moves its notes to To Do."
	switch section.ID {
	case journal.NoteSectionToDo:
		about = "Built in. You can rename it."
	case journal.NoteSectionPlaces:
		about = "Built in. Your Map notes are listed here too."
	}
	lines := []detailLine{
		{label: "", value: section.Name, color: DialogHeaderColor, header: true},
		{label: "Notes", value: strconv.Itoa(total), color: DialogValueColor, header: false},
		{label: notesDoneLabel, value: strconv.Itoa(done), color: DialogValueColor, header: false},
		blankDetailLine(),
		{label: "", value: about, color: DialogControlHintColor, header: false},
	}
	if len(d.notesView.notes.Notes) == 0 && len(d.notesView.mapNotes) == 0 {
		lines = append(lines, blankDetailLine(),
			detailLine{label: "", value: "Your notebook is empty.", color: DialogValueColor, header: false},
			detailLine{label: "N", value: "write a note in the selected section.", color: DialogValueColor, header: false},
			detailLine{label: "Shift+N", value: "add a section of your own.", color: DialogValueColor, header: false})
	}

	return lines
}

// noteStampLines are the Section and Recorded rows every note shows after its words.
func noteStampLines(section string, recorded journal.GameTime) []detailLine {
	return []detailLine{blankDetailLine(),
		{label: "Section", value: section, color: DialogValueColor, header: false},
		{label: "Recorded", value: noteDate(recorded), color: DialogValueColor, header: false}}
}

func blankDetailLine() detailLine {
	return detailLine{label: "", value: "", color: DialogValueColor, header: false}
}

func noteDate(at journal.GameTime) string {
	return fmt.Sprintf("%d-%d-%d %02d:%02d", at.Month, at.Day, at.Year, at.Hour, at.Minute)
}

func (d *QuestJournalDialog) requestNotes(intent NotesIntent) {
	if d.notesView.onIntent == nil {
		d.SetNotesStatus("Notes are unavailable.")

		return
	}
	d.notesView.onIntent(intent)
}

// handleNotesInput runs before dismiss and search, like handleExcerptInput.
func (d *QuestJournalDialog) handleNotesInput() bool {
	if d.activeTab != QuestJournalNotesTab || d.searching || d.runicReferenceOpen {
		return false
	}
	switch {
	case d.notesView.confirm != notesConfirmNone:
		d.handleNotesConfirm()

		return true
	case d.notesView.moving:
		d.handleNotesMove()

		return true
	}
	keyboard, row := d.keyboard, d.selectedNotesRow()
	shift := keyboard.IsModifierPressed(input.ModifierShift)
	switch {
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyN):
		d.handleNotesNew(shift)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyH):
		d.notesView.showDone = !d.notesView.showDone
		d.keepNotesSelection()
	case row.note >= 0:
		return d.handleNoteKeys(d.notesView.notes.Notes[row.note])
	case row.mapNote >= 0:
		return d.handleMapNoteKeys(d.notesView.mapNotes[row.mapNote])
	default:
		return d.handleSectionKeys(d.selectedSection())
	}

	return true
}

func (d *QuestJournalDialog) handleNotesNew(shift bool) {
	if shift {
		d.requestNotes(NotesIntent{Action: NotesNewSection, NoteID: 0, SectionID: 0, Folded: false, MapNote: false})

		return
	}
	if len(d.notesView.notes.Notes) >= journal.MaxNotes {
		d.SetNotesStatus(fmt.Sprintf("Notebook full: %d notes. Delete one to write another.", journal.MaxNotes))

		return
	}
	d.requestNotes(NotesIntent{Action: NotesNew, NoteID: 0, SectionID: d.selectedSection().ID, Folded: false,
		MapNote: false})
}

func (d *QuestJournalDialog) handleNoteKeys(note journal.NoteRecord) bool {
	keyboard := d.keyboard
	intent := NotesIntent{Action: NotesEdit, NoteID: note.ID, SectionID: note.SectionID, Folded: false, MapNote: false}
	switch {
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyE), keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter),
		keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyM):
		if note.Site == nil {
			d.SetNotesStatus("This note has no place on the Map.")

			return true
		}
		intent.Action = NotesShowOnMap
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyX):
		intent.Action = NotesToggleDone
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyS):
		d.notesView.moving = true
		d.notesView.moveChoice = max(0, d.notesView.notes.SectionIndex(note.SectionID))
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyD), keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete):
		d.notesView.confirm = notesConfirmNote
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft):
		intent.Action, intent.NoteID, intent.Folded = NotesFold, 0, true
		d.requestNotes(intent)
	default:
		return false
	}

	return true
}

// handleMapNoteKeys offers the Map note's own actions; it stays in Places and is never done.
func (d *QuestJournalDialog) handleMapNoteKeys(note journal.AtlasNoteRecord) bool {
	keyboard := d.keyboard
	intent := NotesIntent{Action: NotesEdit, NoteID: note.ID, SectionID: journal.NoteSectionPlaces, Folded: false,
		MapNote: true}
	switch {
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyE), keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter),
		keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyM):
		intent.Action = NotesShowOnMap
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyD), keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete):
		d.notesView.confirm = notesConfirmMapNote
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyX):
		d.SetNotesStatus("Map notes can't be marked done.")
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyS):
		d.SetNotesStatus("Map notes stay in Places.")
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft):
		intent.Action, intent.NoteID, intent.Folded, intent.MapNote = NotesFold, 0, true, false
		d.requestNotes(intent)
	default:
		return false
	}

	return true
}

func (d *QuestJournalDialog) handleSectionKeys(section journal.NoteSection) bool {
	keyboard := d.keyboard
	intent := NotesIntent{Action: NotesFold, NoteID: 0, SectionID: section.ID, Folded: false, MapNote: false}
	switch {
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft), keyboard.IsKeyPressedAndRegister(ebiten.KeyRight):
		intent.Folded = !section.Folded
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyR):
		if section.ID == journal.NoteSectionPlaces {
			d.SetNotesStatus("Places keeps its name.")

			return true
		}
		intent.Action = NotesRenameSection
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyD), keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete):
		if section.ID == journal.NoteSectionToDo || section.ID == journal.NoteSectionPlaces {
			d.SetNotesStatus("To Do and Places can't be deleted.")

			return true
		}
		d.notesView.confirm = notesConfirmSection
	default:
		return false
	}

	return true
}

func (d *QuestJournalDialog) handleNotesConfirm() {
	view := &d.notesView
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
		view.confirm = notesConfirmNone
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter), d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		row := d.selectedNotesRow()
		intent := NotesIntent{Action: NotesDeleteSection, NoteID: 0, SectionID: d.selectedSection().ID, Folded: false,
			MapNote: false}
		switch {
		case view.confirm == notesConfirmNote && row.note >= 0:
			intent.Action, intent.NoteID = NotesDelete, view.notes.Notes[row.note].ID
		case view.confirm == notesConfirmMapNote && row.mapNote >= 0:
			intent.Action, intent.NoteID, intent.MapNote = NotesDelete, view.mapNotes[row.mapNote].ID, true
		case view.confirm != notesConfirmSection:
			view.confirm = notesConfirmNone

			return
		}
		view.confirm = notesConfirmNone
		d.requestNotes(intent)
	}
}

func (d *QuestJournalDialog) handleNotesMove() {
	sections := d.notesView.notes.Sections
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
		d.notesView.moving = false
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyUp):
		d.notesView.moveChoice = max(0, d.notesView.moveChoice-1)
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyDown):
		d.notesView.moveChoice = min(len(sections)-1, d.notesView.moveChoice+1)
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter), d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		row := d.selectedNotesRow()
		d.notesView.moving = false
		if row.note >= 0 {
			note := d.notesView.notes.Notes[row.note]
			d.requestNotes(NotesIntent{Action: NotesMove, NoteID: note.ID,
				SectionID: sections[d.notesView.moveChoice].ID, Folded: false, MapNote: false})
		}
	}
}

func (d *QuestJournalDialog) notesFooterGuidance(tabs string) string {
	view := &d.notesView
	switch {
	case view.confirm == notesConfirmNote:
		return "Permanently delete this note?  [Enter] Delete  [Esc] Cancel"
	case view.confirm == notesConfirmMapNote:
		return "Permanently delete this Map note?  [Enter] Delete  [Esc] Cancel"
	case view.confirm == notesConfirmSection:
		section := d.selectedSection()
		total, _ := d.sectionNoteCounts(section.ID)

		return fmt.Sprintf("Delete section %s? Its %d notes move to To Do.  [Enter] Delete  [Esc] Keep", section.Name, total)
	case view.moving:
		return "[Up/Down] Select  [Enter] Move  [Esc] Cancel"
	}
	paging := ""
	if pages := d.GetDetailPageCount(); pages > 1 {
		paging = fmt.Sprintf("[Space] Detail %d/%d  ", d.detailPage+1, pages)
	}
	common := d.showDoneHint() + "[Shift+N] New section  [/] Search  " + tabs + paging + "[Esc/Shift+J] Close"
	row := d.selectedNotesRow()
	switch {
	case row.mapNote >= 0:
		return "[Up/Down] Select  [N] New  [E] Edit  [M] Map  [D] Delete  " + common
	case row.note >= 0:
		note := view.notes.Notes[row.note]
		keys := "[Up/Down] Select  [N] New  [E] Edit  "
		if note.Site != nil {
			keys += "[M] Map  "
		}
		done := notesDoneLabel
		if note.Done {
			done = "Not done"
		}

		return keys + "[X] " + done + "  [S] Section  [D] Delete  " + common
	}
	section := d.selectedSection()
	keys := "[Up/Down] Select  [Left/Right] Fold  [N] New  "
	if section.ID != journal.NoteSectionPlaces {
		keys += "[R] Rename  "
	}
	if section.ID != journal.NoteSectionToDo && section.ID != journal.NoteSectionPlaces {
		keys += "[D] Delete  "
	}

	return keys + common
}

// showDoneHint offers H only while some notes done before this visit exist.
func (d *QuestJournalDialog) showDoneHint() string {
	hidden := len(d.notesView.hiddenDone)
	switch {
	case hidden == 0:
		return ""
	case d.notesView.showDone:
		return "[H] Show Done: On  "
	default:
		return fmt.Sprintf("[H] Show Done: Off (%d hidden)  ", hidden)
	}
}

// drawNotesChooser replaces the list while a note is being moved.
func (d *QuestJournalDialog) drawNotesChooser(screen *ebiten.Image) {
	content := d.listContentRect()
	rowHeight := d.rowHeight()
	line := func(index int) image.Rectangle {
		top := content.Min.Y + index*rowHeight

		return image.Rect(content.Min.X, top, content.Max.X, min(content.Max.Y, top+rowHeight))
	}
	d.drawSingleLine(screen, line(0), "MOVE TO SECTION", DialogLabelColor)
	if row := d.selectedNotesRow(); row.note >= 0 {
		d.drawSingleLine(screen, line(1), d.notesView.notes.Notes[row.note].ListTitle(), DialogControlHintColor)
	}
	current := d.selectedSection().ID
	const chooserHeaderRows = 3
	for index, section := range d.notesView.notes.Sections {
		rect := line(chooserHeaderRows + index)
		name, textColor := section.Name, DialogItemNameColor
		if section.ID == current {
			name, textColor = name+" - current", DialogEmptyColor
		}
		if index == d.notesView.moveChoice {
			fillJournalRow(screen, rect)
			textColor = DialogSelectedText
		}
		d.drawSingleLine(screen, rect, name, textColor)
	}
}

// appendNoteSearchEntries indexes every note, including folded and hidden done
// ones, and the Map notes listed in Places.
func (d *QuestJournalDialog) appendNoteSearchEntries() {
	for _, note := range d.notesView.notes.Notes {
		section := d.notesView.notes.Sections[max(0, d.notesView.notes.SectionIndex(note.SectionID))]
		body := strings.TrimSpace(note.Title + " " + note.Text)
		searchable := body + " " + section.Name + " " + note.Place
		if note.Done {
			searchable += " Done"
		}
		source := section.Name + " - " + fmt.Sprintf("%d-%d-%d", note.RecordedAt.Month, note.RecordedAt.Day,
			note.RecordedAt.Year)
		if note.Place != "" {
			source += " - " + note.Place
		}
		d.searchEntries = append(d.searchEntries, questJournalSearchEntry{
			tab: QuestJournalNotesTab, row: -1, noteID: note.ID, mapNote: false, title: note.ListTitle(),
			text: strings.ToLower(searchable), source: source, body: body, snippet: "",
		})
	}
	for _, note := range d.notesView.mapNotes {
		body := strings.TrimSpace(note.Label + " " + note.Description)
		source := fmt.Sprintf("%s - Map note - %d-%d-%d", journal.NoteSectionPlacesName, note.RecordedAt.Month,
			note.RecordedAt.Day, note.RecordedAt.Year)
		d.searchEntries = append(d.searchEntries, questJournalSearchEntry{
			tab: QuestJournalNotesTab, row: -1, noteID: note.ID, mapNote: true, title: mapNoteTitle(note),
			text: strings.ToLower(body + " " + journal.NoteSectionPlacesName + " Map"), source: source, body: body,
			snippet: "",
		})
	}
}

// openNoteSearchResult shows the note even when its section is folded or done
// notes are hidden, and selects it.
func (d *QuestJournalDialog) openNoteSearchResult(noteID uint64, mapNote bool) {
	sectionID, journalNoteID := journal.NoteSectionPlaces, uint64(0)
	if !mapNote {
		index := d.notesView.notes.NoteIndex(noteID)
		if index < 0 {
			return
		}
		sectionID, journalNoteID = d.notesView.notes.Notes[index].SectionID, noteID
		delete(d.notesView.hiddenDone, noteID)
	}
	if sectionIndex := d.notesView.notes.SectionIndex(sectionID); d.notesView.notes.Sections[sectionIndex].Folded {
		d.notesView.notes.Sections[sectionIndex].Folded = false
		d.requestNotes(NotesIntent{Action: NotesFold, NoteID: journalNoteID, SectionID: sectionID, Folded: false,
			MapNote: false})
	}
	d.rebuildNoteRows()
	row := d.mapNoteRowFor(noteID)
	if !mapNote {
		row = d.noteRowFor(noteID, 0)
	}
	if row >= 0 {
		d.selections[QuestJournalNotesTab] = row
	}
}

func fillJournalRow(screen *ebiten.Image, rect image.Rectangle) {
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()),
		DialogSelectedBackground, false)
}
