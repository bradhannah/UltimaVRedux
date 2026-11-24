package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

// The amount window is fitted to its grid: a margin letter, the row's name, what
// the party has, what the choice changes and what it will have, and a margin letter.
const (
	amountWindowChars    = 36
	amountWindowMaxLines = 18
	amountFooterMaxLines = 3
	amountLabelColumns   = 10
	amountStepperGap     = 8 // Puts the chosen number over the column it changes.
	amountNumberColumns  = 5
	amountMostColumns    = 8
	amountHaveColumns    = 7
	amountChangeColumns  = 7
	amountThenColumns    = 8
	amountLineSpacing    = 1.2
	amountMinimumInset   = 6
	amountMinimumChars   = 8
	amountKeys           = "Left/Right/0-9"
	amountMostKey        = "M"
	amountGoldRow        = "Gold"
	amountCarriedRow     = "Carried"
	amountBuyVerb        = "Buy"
)

// amountTone says how a line under the grid is drawn.
type amountTone int

const (
	amountPlain   amountTone = iota // Grey: something worth knowing.
	amountWarning                   // The readable red: something lost, or why not.
	amountVerdict                   // A keeper's refusal label, centred in the status red.
)

// amountNote is one authored line under the grid; it is written to fit the window.
type amountNote struct {
	text string
	tone amountTone
}

// amountRow is one line of the grid: what the party has and what it will have.
type amountRow struct {
	label string
	have  int
	then  int
}

// amountStep is everything one amount window shows. The item is named once, in
// the title, with the price of one under it; each row then reads across as a sum.
type amountStep struct {
	title   string
	preview bool
	unit    string // The price of one, under the title; empty for none.
	pitch   string // The keeper's own line, first in the body; empty for none.
	noun    string // What is counted, such as "Bundles"; empty when nothing is chosen.
	count   int
	most    int
	rows    []amountRow
	notes   []amountNote
	reserve int    // Note lines kept free, so the window keeps its size as the amount changes.
	verb    string // What Enter does, such as "Buy".
	back    string // What Esc does when it is more than going back.
	refused bool   // Enter cannot work; the notes say why.
	footer  string // Replaces the usual hints when a step is only acknowledged.
}

// amountWindow draws an amountStep in a window fitted to it.
type amountWindow struct {
	frame  *merchantModalFrame
	step   amountStep
	header *text.Output
	unit   *text.Output
	body   *text.Output
	footer *text.Output
}

func newAmountWindow() *amountWindow {
	const (
		startX, endX = 0.27, 0.73
		startY, endY = 0.12, 0.88
		headerRatio  = 0.12
		footerRatio  = 0.17
	)

	return &amountWindow{
		frame: newMerchantModalFrame(merchantModalFrameConfig{
			placement: sprites.PercentBasedPlacement{
				StartPercentX: startX, EndPercentX: endX, StartPercentY: startY, EndPercentY: endY,
			},
			headerRatio: headerRatio, footerRatio: footerRatio, minimumInset: amountMinimumInset,
			lineSpacingMultiplier: amountLineSpacing, footerMinimumLines: 1,
		}),
		step:   emptyAmountStep(),
		header: nil, unit: nil, body: nil, footer: nil,
	}
}

func emptyAmountStep() amountStep {
	var step amountStep

	return step
}

// set shows a step. The window is laid out for the first step it is given and
// then keeps its size; later steps only change what is written in it.
func (w *amountWindow) set(step amountStep) {
	w.step = step
	if w.body == nil || w.frame.ensureLayout() {
		w.layout()

		return
	}
	w.populate()
}

func (w *amountWindow) ensureLayout() {
	if w.frame.ensureLayout() {
		w.layout()
	}
}

func (w *amountWindow) layout() {
	w.frame.ensureLayout()
	w.frame.headerNoteLines = 0
	if w.step.unit != "" {
		w.frame.headerNoteLines = 1
	}
	// The width comes first: how the keeper's line and the hints wrap depends on it.
	w.frame.config.footerMinimumLines = 1
	w.frame.fit(amountWindowMaxLines, amountWindowChars)
	w.buildOutputs()
	lines := w.body.GetPanelContentLineCount()
	if spare := w.step.reserve - len(w.step.notes); spare > 0 {
		lines += spare
		if len(w.step.notes) == 0 {
			lines++ // The blank line that would part the notes from the grid.
		}
	}
	w.frame.config.footerMinimumLines = max(1, w.footer.GetPanelContentLineCount())
	w.frame.fit(lines, amountWindowChars)
	w.buildOutputs()
}

func (w *amountWindow) buildOutputs() {
	w.header, w.body, w.footer = w.frame.newOutputs(merchantModalTextBudget{
		lineSpacing: amountLineSpacing, maximumBodyLines: amountWindowMaxLines,
		footerLines: amountFooterMaxLines, minimumChars: amountMinimumChars, fixedBody: false,
	})
	w.unit = w.frame.newHeaderNoteOutput(amountLineSpacing)
	w.populate()
}

func (w *amountWindow) populate() {
	step := w.step
	w.header.Reset()
	title := w.header.AppendLine().WithAlignment(etext.AlignCenter)
	if step.preview {
		title.Text("PREVIEW - ", DialogStatusWarningColor)
	}
	title.Text(step.title, DialogHeaderColor).Done()
	w.unit.Reset()
	if step.unit != "" {
		w.unit.AppendLine().WithAlignment(etext.AlignCenter).Text(step.unit, DialogControlHintColor).Done()
	}
	w.populateBody()
	w.populateFooter()
}

func (w *amountWindow) populateBody() {
	step, body := w.step, w.body
	body.Reset()
	part := func() {
		if body.GetPanelContentLineCount() > 0 {
			body.AppendLine().White("").Done()
		}
	}
	if step.pitch != "" {
		body.AppendLine().WithAlignment(etext.AlignCenter).Text(step.pitch, DialogValueColor).WordWrap().Done()
	}
	if step.noun != "" {
		part()
		line := body.AppendLine().Text(
			fmt.Sprintf(" %-*s%*s", amountLabelColumns, step.noun, amountStepperGap, ""), DialogValueColor)
		appendAmountStepper(line, step.count, step.most).Done()
	}
	if len(step.rows) > 0 {
		part()
		body.AppendLine().Text(fmt.Sprintf(" %*s%*s%*s%*s", amountLabelColumns, "", amountHaveColumns, "Have",
			amountChangeColumns+1, "", amountThenColumns, "Then"), DialogLabelColor).Done()
	}
	for _, row := range step.rows {
		body.AppendLine().
			Text(fmt.Sprintf(" %-*s", amountLabelColumns, row.label), DialogValueColor).
			Text(fmt.Sprintf("%*d", amountHaveColumns, row.have), DialogControlHintColor).
			Text(fmt.Sprintf(" %s%*d", centredText(fmt.Sprintf("%+d", row.then-row.have), amountChangeColumns),
				amountThenColumns, row.then), DialogValueColor).Done()
	}
	if len(step.notes) > 0 {
		part()
	}
	for _, note := range step.notes {
		switch note.tone {
		case amountWarning:
			body.AppendLine().Text(" "+note.text, DialogWarningColor).WordWrap().Done()
		case amountVerdict:
			body.AppendLine().WithAlignment(etext.AlignCenter).Text(note.text, DialogStatusWarningColor).WordWrap().Done()
		case amountPlain:
			body.AppendLine().Text(" "+note.text, DialogControlHintColor).WordWrap().Done()
		default:
			panic("amountWindow: unknown note tone")
		}
	}
}

func (w *amountWindow) populateFooter() {
	step, width := w.step, w.frame.footerText.Dx()
	w.footer.Reset()
	if step.footer != "" {
		appendControlGuidance(w.footer, step.footer, width)

		return
	}
	dimmed := func(hint ControlHint) bool {
		if hint.Key == hintKeyEnter {
			return step.refused
		}

		return step.most <= 1 && (hint.Key == amountKeys || hint.Key == amountMostKey)
	}
	enter := ControlHint{Key: hintKeyEnter, Action: step.verb}
	if step.noun != "" {
		enter.Action = fmt.Sprintf("%s %d", step.verb, step.count)
		PackControlHintsDimming(w.footer, []ControlHint{
			{Key: amountKeys, Action: step.noun}, {Key: amountMostKey, Action: "Most"},
		}, width, etext.AlignCenter, dimmed)
	}
	back := ControlHint{Key: hintKeyEsc, Action: "Back"}
	if step.back != "" {
		back.Action = step.back
	}
	PackControlHintsDimming(w.footer, []ControlHint{enter, back}, width, etext.AlignCenter, dimmed)
}

func (w *amountWindow) draw(screen *ebiten.Image) {
	w.ensureLayout()
	w.frame.draw(screen)
	w.frame.drawOutputs(screen, w.header, w.body, w.footer)
	if w.step.unit != "" {
		w.frame.drawHeaderNote(screen, w.unit)
	}
}

func (w *amountWindow) bounds() image.Rectangle {
	w.ensureLayout()

	return w.frame.bounds()
}

// appendAmountStepper writes the part of a line on which an amount is chosen. It is
// the one place that decides how a chosen number looks: bright yellow between its
// arrows with no selection bar, an arrow that cannot move drawn as unavailable, and
// the most that can be had in grey after it.
func appendAmountStepper(line *text.LineBuilder, count, most int) *text.LineBuilder {
	arrow := func(open bool) stdcolor.Color {
		if open {
			return DialogValueColor
		}

		return DialogUnavailableItemColor
	}

	return line.Text("<", arrow(count > 1)).
		Text(centredText(strconv.Itoa(count), amountNumberColumns), DialogLabelColor).
		Text(">", arrow(count < most)).
		Text(fmt.Sprintf("%*s", amountMostColumns, fmt.Sprintf("of %d", most)), DialogControlHintColor)
}

// centredText pads text to width, with the odd space on the left so that numbers
// of different lengths keep their last digit close to the same column.
func centredText(value string, width int) string {
	const sides = 2
	spare := max(0, width-len(value))
	right := spare / sides

	return fmt.Sprintf("%*s%s%*s", spare-right, "", value, right, "")
}

// bundleUnitPrice words the price of one bundle, the line under the title.
func bundleUnitPrice(price, size int) string {
	if size == 1 {
		return fmt.Sprintf("%d gold each", price)
	}

	return fmt.Sprintf("%d gold for a bundle of %d", price, size)
}

// overflowNote says how much of what is paid for will not fit, and that the price
// is not reduced for it. The gold row shows the price.
func overflowNote(lost int) []amountNote {
	return []amountNote{{text: fmt.Sprintf("%d will not fit. The price stays.", lost), tone: amountWarning}}
}

// shortOfGoldNotes says, with its figures, that the party cannot pay for what is
// named, and then what asking anyway will do.
func shortOfGoldNotes(what string, cost, gold int, consequences ...string) []amountNote {
	notes := []amountNote{
		{text: "Thou hast not enough gold. " + what, tone: amountWarning},
		{text: fmt.Sprintf("%d and the party has %d.", cost, gold), tone: amountWarning},
	}
	if len(consequences) > 0 {
		notes = append(notes, amountNote{text: "", tone: amountPlain})
	}
	for _, consequence := range consequences {
		notes = append(notes, amountNote{text: consequence, tone: amountPlain})
	}

	return notes
}
