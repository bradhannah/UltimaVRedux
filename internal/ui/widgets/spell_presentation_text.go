package widgets

import (
	"image"
	stdcolor "image/color"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

const (
	spellBackAction  = "Back"
	spellCasterLabel = "Caster"
	spellAimAction   = "Aim"
	// casterGapLetters separates the casters on Cast's header where there is room.
	casterGapLetters = 3
	spellFilterKey   = "Ctrl+O"
	spellFilterHint  = "Filter"
	spellClearSearch = "Clear search"
	spellClearKey    = "Ctrl+U"
)

func appendSpellField(line *text.LineBuilder, label, value string) *text.LineBuilder {
	return line.Text(label+": ", DialogLabelColor).Text(value, DialogValueColor)
}

func appendSpellWarningField(line *text.LineBuilder, label, value string, warning stdcolor.Color) *text.LineBuilder {
	return line.Text(label+": ", DialogLabelColor).Text(value, warning)
}

// A row has one measured two-line block with equal leading/trailing padding.
// Capacity and rendering must share this pitch rather than round line spacing independently.
func spellRowHeight(output *text.Output) int {
	contentHeight := int(math.Ceil(output.GetTotalHeightForLines(spellBrowserRowLines)))
	padding := max(1, int(math.Ceil(output.GetLineSpacing()-output.GetFontHeight()))/spellBrowserRowLines)

	return contentHeight + padding*spellBrowserRowLines
}

func spellRowContentRect(output *text.Output, rect image.Rectangle) image.Rectangle {
	height := int(math.Ceil(output.GetTotalHeightForLines(spellBrowserRowLines)))
	rect.Min.Y += max(0, (rect.Dy()-height)/spellBrowserRowLines)

	return rect
}

func spellRowColours(selected, available bool) (stdcolor.Color, stdcolor.Color) {
	if selected {
		return DialogSelectedText, DialogSelectedText
	}
	if !available {
		return DialogUnavailableItemColor, DialogControlHintColor
	}

	return DialogItemNameColor, DialogControlHintColor
}

func appendReagentMark(line *text.LineBuilder, checked, selected bool) *text.LineBuilder {
	bracketColor := DialogControlHintColor
	markColor := DialogCheckedMarkColor
	if selected {
		bracketColor = DialogSelectedText
		markColor = DialogSelectedCheckedMarkColor
	}
	mark := " "
	if checked {
		mark = "X"
	}

	return line.Text("[", bracketColor).Text(mark, markColor).Text("] ", bracketColor)
}

func spellFinderControls(action, escape string, searching bool) []ControlHint {
	controls := []ControlHint{
		{hintKeyUpDown, "Browse"}, {hintKeyShiftPage, hintActionPage}, {spellFilterKey, spellFilterHint},
	}
	if searching {
		controls = append(controls, ControlHint{spellClearKey, spellClearSearch})
	} else {
		controls = append(controls, ControlHint{"/", hintActionSearch},
			ControlHint{"Letters", "Spell initials"}, ControlHint{"Backspace", "Remove syllable"})
	}

	return append(controls, ControlHint{hintKeyEnter, action}, ControlHint{hintKeyEsc, escape})
}

// controls are the spell list's hints; the quantity window writes its own.
func (d *ReagentMixingDialog) controls() []ControlHint {
	escape := "Close"
	if d.browser.searching {
		escape = spellBackAction
	}

	return spellFinderControls("Choose spell", escape, d.browser.searching)
}

// spellHint is one hand-placed hint. off dims a key that cannot work now.
type spellHint struct {
	key, action string
	off         bool
}

func appendSpellHints(line *text.LineBuilder, hints ...spellHint) {
	for index, hint := range hints {
		if index > 0 {
			line.Text(controlHintSeparator, DialogControlHintColor)
		}
		appendMixHint(line, hint.key, " "+hint.action, !hint.off)
	}
	line.Done()
}

// writeFooter places Cast's hints by hand so they stay put as the highlight
// moves: one row for getting about the list, one for what Enter and Escape do.
// Enter is named for the selected spell's next step and dimmed when it is refused.
// The keys every list shares (Shift+Up/Down, Backspace, 1-6) work and are in Help.
func (d *SpellCastingDialog) writeFooter(output *text.Output, _ int, paged bool) {
	browse := []spellHint{{hintKeyUpDown, "Browse", false}}
	act := []spellHint{{hintKeyEnter, "Cast", true}}
	if spell := d.browser.current(); spell != nil {
		id := references.Spell(spell.ID)
		act[0] = spellHint{hintKeyEnter, d.enterAction(id), !d.entersWorkflow(id)}
	}
	escape := hintActionClose
	switch {
	case d.browser.searching:
		act, escape = append(act, spellHint{spellClearKey, spellClearSearch, false}), "Leave search"
	case d.currentContext != references.ContextCombat:
		act = append(act, spellHint{"Ctrl+M", "Mix", false})
	}
	if !d.browser.searching {
		browse = append(browse, spellHint{"/", hintActionSearch, false})
	}
	browse = append(browse, spellHint{spellFilterKey, spellFilterHint, false})
	if !d.casterSelectionLocked && len(d.casters()) > 1 {
		browse = append(browse, spellHint{"Tab", "Next caster", false})
	}
	appendSpellHints(output.AppendLine(), browse...)
	appendSpellHints(output.AppendLine(), append(act, spellHint{hintKeyEsc, escape, false})...)
	if paged {
		appendSpellHints(output.AppendLine(), spellHint{hintKeySpace, "Next detail page", false})
	}
}
