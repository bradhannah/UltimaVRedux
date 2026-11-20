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
	controls := []ControlHint{{hintKeyUpDown, "Browse"}, {hintKeyShiftPage, hintActionPage}, {"Ctrl+O", "Filter"}}
	if searching {
		controls = append(controls, ControlHint{"Ctrl+U", "Clear search"})
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

func (d *SpellCastingDialog) controls() []ControlHint {
	escape := "Close"
	if d.browser.searching {
		escape = "Leave search"
	}
	controls := spellFinderControls("Cast", escape, d.browser.searching)
	if d.currentContext != references.ContextCombat {
		controls = append(controls, ControlHint{"Ctrl+M", "Mix"})
	}
	if !d.casterSelectionLocked {
		controls = append(controls, ControlHint{hintKeyTabCycle, spellCasterLabel})
		if !d.browser.searching {
			controls = append(controls, ControlHint{"1-6", spellCasterLabel})
		}
	}

	return controls
}
