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
)

// NotesIntent is one My Notes request for the scene to carry out.
type NotesIntent struct {
	Action    NotesAction
	NoteID    uint64
	SectionID uint64
	Folded    bool
}

// notesDoneLabel names a ticked note in rows, details and the footer.
const notesDoneLabel = "Done"

type notesConfirm uint8

const (
	notesConfirmNone notesConfirm = iota
	notesConfirmNote
	notesConfirmSection
)

// notesRow is one list row: a section heading (note < 0) or a note under it.
type notesRow struct{ section, note int }

// notesState is the My Notes part of the Journal; the scene owns the notebook.
type notesState struct {
	notes      journal.NotesState
	rows       []notesRow
	onIntent   func(NotesIntent)
	status     string
	confirm    notesConfirm
	moving     bool
	moveChoice int
}

func (d *QuestJournalDialog) notesOnly() bool { return len(d.tabs) == 1 }

// NotesChanged replaces the detached notebook after the scene applied a request.
// A nonzero note or section identity is selected; status explains the result.
func (d *QuestJournalDialog) NotesChanged(notes journal.NotesState, noteID, sectionID uint64, status string) {
	d.notesView.notes = notes.Normalized()
	d.notesView.status = status
	d.notesView.confirm, d.notesView.moving = notesConfirmNone, false
	d.rebuildNoteRows()
	if row := d.noteRowFor(noteID, sectionID); row >= 0 {
		d.selections[QuestJournalNotesTab] = row
	}
	d.clampCurrentSelection()
	d.cachedDetailPages = nil
	d.detailPage = 0
	d.buildSearchEntries()
}

func (d *QuestJournalDialog) rebuildNoteRows() {
	view := &d.notesView
	view.rows = view.rows[:0]
	for sectionIndex, section := range view.notes.Sections {
		view.rows = append(view.rows, notesRow{section: sectionIndex, note: -1})
		if section.Folded {
			continue
		}
		start := len(view.rows)
		for noteIndex, note := range view.notes.Notes {
			if note.SectionID == section.ID {
				view.rows = append(view.rows, notesRow{section: sectionIndex, note: noteIndex})
			}
		}
		// Newest first; ticked notes sink below the rest of their section.
		slices.SortFunc(view.rows[start:], func(a, b notesRow) int {
			left, right := view.notes.Notes[a.note], view.notes.Notes[b.note]
			if left.Done != right.Done {
				if left.Done {
					return 1
				}

				return -1
			}

			return cmp.Compare(right.ID, left.ID)
		})
	}
}

func (d *QuestJournalDialog) noteRowFor(noteID, sectionID uint64) int {
	for index, row := range d.notesView.rows {
		if noteID != 0 && row.note >= 0 && d.notesView.notes.Notes[row.note].ID == noteID {
			return index
		}
		if noteID == 0 && sectionID != 0 && row.note < 0 && d.notesView.notes.Sections[row.section].ID == sectionID {
			return index
		}
	}

	return -1
}

// selectedNotesRow returns the selected row; the list always has two headings.
func (d *QuestJournalDialog) selectedNotesRow() notesRow {
	rows := d.notesView.rows
	if len(rows) == 0 {
		return notesRow{section: 0, note: -1}
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

	return total, done
}

func (d *QuestJournalDialog) notesRowText(index int) string {
	row := d.notesView.rows[index]
	if row.note >= 0 {
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
	case row.note < 0:
		return DialogLabelColor
	case d.notesView.notes.Notes[row.note].Done:
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
	if row.note < 0 {
		return append(lines, d.sectionDetailLines(section)...)
	}
	note := d.notesView.notes.Notes[row.note]
	if note.Title != "" {
		lines = append(lines, detailLine{label: "", value: note.Title, color: DialogHeaderColor, header: true})
	}
	for paragraph := range strings.SplitSeq(note.Text, "\n") {
		lines = append(lines, detailLine{label: "", value: paragraph, color: DialogValueColor, header: false})
	}
	lines = append(lines, blankDetailLine(),
		detailLine{label: "Section", value: section.Name, color: DialogValueColor, header: false},
		detailLine{label: "Recorded", value: noteDate(note.RecordedAt), color: DialogValueColor, header: false})
	if note.Place != "" {
		lines = append(lines, detailLine{label: "Place", value: note.Place, color: DialogValueColor, header: false})
	}
	if note.Done {
		lines = append(lines, detailLine{label: "State", value: notesDoneLabel, color: DialogEmptyColor, header: false})
	}

	return lines
}

func (d *QuestJournalDialog) sectionDetailLines(section journal.NoteSection) []detailLine {
	total, done := d.sectionNoteCounts(section.ID)
	about := "A section you made. Deleting it moves its notes to To Do."
	switch section.ID {
	case journal.NoteSectionToDo:
		about = "Built in. You can rename it."
	case journal.NoteSectionPlaces:
		about = "Built in."
	}
	lines := []detailLine{
		{label: "", value: section.Name, color: DialogHeaderColor, header: true},
		{label: "Notes", value: strconv.Itoa(total), color: DialogValueColor, header: false},
		{label: notesDoneLabel, value: strconv.Itoa(done), color: DialogValueColor, header: false},
		blankDetailLine(),
		{label: "", value: about, color: DialogControlHintColor, header: false},
	}
	if len(d.notesView.notes.Notes) == 0 {
		lines = append(lines, blankDetailLine(),
			detailLine{label: "", value: "Your notebook is empty.", color: DialogValueColor, header: false},
			detailLine{label: "N", value: "write a note in the selected section.", color: DialogValueColor, header: false},
			detailLine{label: "Shift+N", value: "add a section of your own.", color: DialogValueColor, header: false})
	}

	return lines
}

func blankDetailLine() detailLine {
	return detailLine{label: "", value: "", color: DialogValueColor, header: false}
}

func noteDate(at journal.GameTime) string {
	return fmt.Sprintf("%d-%d-%d %02d:%02d", at.Month, at.Day, at.Year, at.Hour, at.Minute)
}

func (d *QuestJournalDialog) requestNotes(intent NotesIntent) {
	if d.notesView.onIntent == nil {
		d.notesView.status = "Notes are unavailable."

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
	case row.note >= 0:
		return d.handleNoteKeys(d.notesView.notes.Notes[row.note])
	default:
		return d.handleSectionKeys(d.selectedSection())
	}

	return true
}

func (d *QuestJournalDialog) handleNotesNew(shift bool) {
	if shift {
		d.requestNotes(NotesIntent{Action: NotesNewSection, NoteID: 0, SectionID: 0, Folded: false})

		return
	}
	if len(d.notesView.notes.Notes) >= journal.MaxNotes {
		d.notesView.status = fmt.Sprintf("Notebook full: %d notes. Delete one to write another.", journal.MaxNotes)
		d.cachedDetailPages = nil

		return
	}
	d.requestNotes(NotesIntent{Action: NotesNew, NoteID: 0, SectionID: d.selectedSection().ID, Folded: false})
}

func (d *QuestJournalDialog) handleNoteKeys(note journal.NoteRecord) bool {
	keyboard := d.keyboard
	intent := NotesIntent{Action: NotesEdit, NoteID: note.ID, SectionID: note.SectionID, Folded: false}
	switch {
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyE), keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter),
		keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
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

func (d *QuestJournalDialog) handleSectionKeys(section journal.NoteSection) bool {
	keyboard := d.keyboard
	intent := NotesIntent{Action: NotesFold, NoteID: 0, SectionID: section.ID, Folded: false}
	switch {
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft), keyboard.IsKeyPressedAndRegister(ebiten.KeyRight):
		intent.Folded = !section.Folded
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyR):
		if section.ID == journal.NoteSectionPlaces {
			d.notesView.status = "Places keeps its name."
			d.cachedDetailPages = nil

			return true
		}
		intent.Action = NotesRenameSection
		d.requestNotes(intent)
	case keyboard.IsKeyPressedAndRegister(ebiten.KeyD), keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete):
		if section.ID == journal.NoteSectionToDo || section.ID == journal.NoteSectionPlaces {
			d.notesView.status = "To Do and Places can't be deleted."
			d.cachedDetailPages = nil

			return true
		}
		d.notesView.confirm = notesConfirmSection
	default:
		return false
	}

	return true
}

func (d *QuestJournalDialog) handleNotesConfirm() {
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
		d.notesView.confirm = notesConfirmNone
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter), d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		row := d.selectedNotesRow()
		intent := NotesIntent{Action: NotesDeleteSection, NoteID: 0, SectionID: d.selectedSection().ID, Folded: false}
		if d.notesView.confirm == notesConfirmNote && row.note >= 0 {
			intent.Action, intent.NoteID = NotesDelete, d.notesView.notes.Notes[row.note].ID
		}
		d.notesView.confirm = notesConfirmNone
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
				SectionID: sections[d.notesView.moveChoice].ID, Folded: false})
		}
	}
}

func (d *QuestJournalDialog) notesFooterGuidance(tabs string) string {
	view := &d.notesView
	switch {
	case view.confirm == notesConfirmNote:
		return "Permanently delete this note?  [Enter] Delete  [Esc] Cancel"
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
	row := d.selectedNotesRow()
	if row.note >= 0 {
		done := notesDoneLabel
		if view.notes.Notes[row.note].Done {
			done = "Not done"
		}

		return "[Up/Down] Select  [N] New  [E] Edit  [X] " + done + "  [S] Section  [D] Delete  " +
			"[Shift+N] New section  [/] Search  " + tabs + paging + "[Esc/Shift+J] Close"
	}
	section := d.selectedSection()
	keys := "[Up/Down] Select  [Left/Right] Fold  [N] New  "
	if section.ID != journal.NoteSectionPlaces {
		keys += "[R] Rename  "
	}
	if section.ID != journal.NoteSectionToDo && section.ID != journal.NoteSectionPlaces {
		keys += "[D] Delete  "
	}

	return keys + "[Shift+N] New section  [/] Search  " + tabs + paging + "[Esc/Shift+J] Close"
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

// appendNoteSearchEntries indexes every note, including those in folded sections.
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
			tab: QuestJournalNotesTab, row: -1, noteID: note.ID, title: note.ListTitle(),
			text: strings.ToLower(searchable), source: source, body: body, snippet: "",
		})
	}
}

// openNoteSearchResult unfolds the note's section if needed and selects it.
func (d *QuestJournalDialog) openNoteSearchResult(noteID uint64) {
	index := d.notesView.notes.NoteIndex(noteID)
	if index < 0 {
		return
	}
	sectionID := d.notesView.notes.Notes[index].SectionID
	if sectionIndex := d.notesView.notes.SectionIndex(sectionID); d.notesView.notes.Sections[sectionIndex].Folded {
		d.notesView.notes.Sections[sectionIndex].Folded = false
		d.rebuildNoteRows()
		d.requestNotes(NotesIntent{Action: NotesFold, NoteID: noteID, SectionID: sectionID, Folded: false})
	}
	if row := d.noteRowFor(noteID, 0); row >= 0 {
		d.selections[QuestJournalNotesTab] = row
	}
}

func fillJournalRow(screen *ebiten.Image, rect image.Rectangle) {
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()),
		DialogSelectedBackground, false)
}
