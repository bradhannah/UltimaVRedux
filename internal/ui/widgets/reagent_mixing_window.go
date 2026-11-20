package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// The quantity window is measured in letters and lines of the body font, so
// every sentence, table row and hint row written for it fits at every size.
const (
	mixWindowColumns  = 50 // Letters across the window.
	mixNameColumns    = 18 // Lead space, tick box and the longest ingredient name.
	mixHaveColumns    = 9  // The Have column, ending at letter 27.
	mixCostColumns    = 6  // The Use column and the Left column.
	mixAmountColumn   = 14 // Where the How many and Prepared values start.
	mixStatusLines    = 2  // The tab row and the top line.
	mixFooterLines    = 3  // The Experiment tab's hint rows; Recipe leaves one empty.
	mixRecipeHead     = 4  // How many, Prepared, a blank line and the column titles.
	mixExperimentHead = 3  // How many, a blank line and the column titles.
	mixTabGapLetters  = 2
	mixHintGap        = "   "
)

// mixWindow is the quantity window's four stacked panes, fitted to its content
// and centred on the screen.
type mixWindow struct {
	header, status, body, footer image.Rectangle
	frames                       []image.Rectangle
}

func (tab mixTab) title() string {
	if tab == mixTabRecipe {
		return "Recipe"
	}

	return "Experiment"
}

// tabs lists the window's tabs. A spell with no recorded recipe has nothing to
// show on Recipe, so it gets the Experiment tab alone.
func (d *ReagentMixingDialog) tabs() []mixTab {
	if d.spell.RecipeMask == 0 {
		return []mixTab{mixTabExperiment}
	}

	return []mixTab{mixTabRecipe, mixTabExperiment}
}

func (d *ReagentMixingDialog) otherTab() mixTab {
	if d.tab == mixTabRecipe {
		return mixTabExperiment
	}

	return mixTabRecipe
}

// rows lists the ingredients the open tab shows: the recipe's own on Recipe,
// every ingredient on Experiment.
func (d *ReagentMixingDialog) rows(tab mixTab) []ReagentInfo {
	if tab == mixTabExperiment {
		return d.reagentInventory
	}
	rows := make([]ReagentInfo, 0, len(d.reagentInventory))
	for _, reagent := range d.reagentInventory {
		if d.spell.RecipeMask&(1<<uint(reagent.Reagent)) != 0 {
			rows = append(rows, reagent)
		}
	}

	return rows
}

func (d *ReagentMixingDialog) letters(count int) int {
	width, _ := d.view.font.MeasureString("M")

	return int(math.Ceil(width * float64(count)))
}

// layoutWindow sizes the window for the taller of its two tabs, so changing
// tab or amount never moves it.
func (d *ReagentMixingDialog) layoutWindow() {
	view := &d.view
	width := d.letters(mixWindowColumns) + view.padding*spellBrowserRowLines
	left := (view.width - width) / spellBrowserRowLines
	content := image.Rect(left+view.padding, 0, left+width-view.padding, view.height)
	bodyLines := max(mixRecipeHead+len(d.rows(mixTabRecipe)),
		mixExperimentHead+len(d.rows(mixTabExperiment))+d.populateCandidates(content))
	heights := [...]int{view.headerRect.Dy(), d.paneHeight(mixStatusLines), d.paneHeight(bodyLines),
		d.paneHeight(mixFooterLines)}
	top := view.height
	for _, height := range heights {
		top -= height
	}
	top /= spellBrowserRowLines
	rects := make([]image.Rectangle, len(heights))
	grid := make([][]image.Rectangle, len(heights))
	for index, height := range heights {
		rects[index] = image.Rect(left, top, left+width, top+height)
		grid[index] = rects[index : index+1]
		top += height
	}
	d.window = mixWindow{header: rects[0], status: rects[1], body: rects[2], footer: rects[3],
		frames: connectClassicPaneGrid(grid, getClassicUIPixelUnit(view.width, view.height))}
}

func (d *ReagentMixingDialog) paneHeight(lines int) int {
	return d.view.lineHeight*lines + d.view.padding*spellBrowserRowLines
}

// populateCandidates writes an uncertain formula's untested clues, below a
// blank line, and returns how many lines that takes.
func (d *ReagentMixingDialog) populateCandidates(content image.Rectangle) int {
	output := d.view.body
	prepareSpellOutput(output, content)
	if len(d.spell.ReagentCandidates) == 0 {
		return 0
	}
	output.AppendLine().Text("", DialogValueColor).Done()
	for _, candidate := range d.spell.ReagentCandidates {
		output.AppendLine().Text("Untested formula: ", DialogLabelColor).
			Text(strings.Join(candidate, " + "), DialogWarningColor).WordWrap().Done()
	}

	return output.GetPanelContentLineCount()
}

func (d *ReagentMixingDialog) drawWindow(screen *ebiten.Image) {
	view, window := &d.view, &d.window
	unit := getClassicUIPixelUnit(view.width, view.height)
	fillClassicPaneBackgrounds(screen, window.frames, unit, pkgcolor.DialogBackground.RGBA())
	header := window.header.Inset(view.padding)
	prepareSpellOutput(view.title, header)
	view.title.AppendLine().Text("MIX REAGENTS", DialogHeaderColor).Done()
	label := FitDialogLine(d.spell.Name+" - "+d.spell.EffectLabel, view.title.Font, header.Dx())
	view.title.AppendLine().Text(label, DialogValueColor).Done()
	drawSpellOutput(screen, view.title, header, 0, spellBrowserRowLines)
	d.drawStatus(screen, unit)
	d.drawBody(screen)
	d.drawWindowFooter(screen)
	drawClassicPaneFrames(screen, window.frames, unit, DialogRuleColor)
}

// drawStatus draws the tab row, the open tab on the shared plate, and under it
// the top line.
func (d *ReagentMixingDialog) drawStatus(screen *ebiten.Image, unit int) {
	view := &d.view
	rect := d.window.status.Inset(view.padding)
	fontHeight := int(math.Ceil(view.heading.GetFontHeight()))
	left := rect.Min.X + d.letters(1)
	for _, tab := range d.tabs() {
		width, _ := view.font.MeasureString(tab.title())
		cell := image.Rect(left, rect.Min.Y, left+int(math.Ceil(width)), rect.Min.Y+view.lineHeight)
		textColor := DialogTabColor
		if tab == d.tab {
			textColor = DialogActiveTabText
			DrawActiveTabPlate(screen, cell, width, cell.Min.Y, fontHeight, unit)
		}
		prepareSpellOutput(view.heading, cell)
		view.heading.AppendLine().Text(tab.title(), textColor).Done()
		drawSpellOutput(screen, view.heading, cell, 0, 1)
		left = cell.Max.X + d.letters(mixTabGapLetters)
	}
	rect.Min.Y += view.lineHeight
	sentence, sentenceColor := d.topLine()
	prepareSpellOutput(view.search, rect)
	view.search.AppendLine().Text(sentence, sentenceColor).Done()
	drawSpellOutput(screen, view.search, rect, 0, 1)
}

// topLine is the one sentence that says whether Enter is wise. A refusal or a
// loss is written in the readable warning red; everything else in label yellow.
func (d *ReagentMixingDialog) topLine() (string, stdcolor.Color) {
	mask := d.mask()
	switch {
	case d.notice != "":
		return d.notice, DialogWarningColor
	case mask == 0 && d.spell.RecipeMask == 0:
		return "No recipe recorded. Choose ingredients to try.", DialogLabelColor
	case mask == 0:
		return "Choose ingredients.", DialogLabelColor
	case d.maximum() == 0:
		return d.missingSentence(), DialogWarningColor
	case mask != d.spell.RecipeMask:
		return "Untested mix: it may be wasted and may hurt.", DialogWarningColor
	case d.room() == 0:
		return fmt.Sprintf("Already %d prepared. These would be thrown away.", spellInventoryLimit), DialogWarningColor
	case d.quantity > d.room():
		return fmt.Sprintf("Only %d will fit. %d of %d would be thrown away.",
			d.room(), d.quantity-d.room(), d.quantity), DialogWarningColor
	default:
		return "Ready to mix.", DialogLabelColor
	}
}

// missingSentence names the one ingredient the mixture lacks, or counts them
// when there are several; the rows below mark each with "none".
func (d *ReagentMixingDialog) missingSentence() string {
	mask := d.mask()
	var missing []string
	for _, reagent := range d.reagentInventory {
		if mask&(1<<uint(reagent.Reagent)) != 0 && reagent.Quantity == 0 {
			missing = append(missing, reagent.Name)
		}
	}
	if len(missing) == 1 {
		return fmt.Sprintf("Cannot mix %s: no %s.", d.spell.Name, missing[0])
	}

	return fmt.Sprintf("Cannot mix %s: lacks %d ingredients.", d.spell.Name, max(1, len(missing)))
}

// mixInk is the colour of each kind of text on a line. The white selection bar
// changes all of them, so every kind keeps its contrast.
type mixInk struct {
	label, value, hint, dim, warning stdcolor.Color
}

func mixLineInk(selected bool) mixInk {
	if selected {
		return mixInk{label: DialogSelectedLabelColor, value: DialogSelectedText, hint: DialogSelectedLabelColor,
			dim: DialogSelectedText, warning: DialogStatusWarningColor}
	}

	return mixInk{label: DialogLabelColor, value: DialogValueColor, hint: DialogControlHintColor,
		dim: DialogUnavailableItemColor, warning: DialogWarningColor}
}

// drawLine draws one line of the window's table, on the white bar when selected.
func (d *ReagentMixingDialog) drawLine(screen *ebiten.Image, rect image.Rectangle, selected bool,
	fill func(line *text.LineBuilder, ink mixInk),
) {
	output := d.view.row
	if selected {
		fillSelectedRow(screen, rect)
	}
	rect.Min.Y += max(0, (rect.Dy()-int(math.Ceil(output.GetFontHeight())))/spellBrowserRowLines)
	prepareSpellOutput(output, rect)
	line := output.AppendLine()
	fill(line, mixLineInk(selected))
	line.Done()
	drawSpellOutput(screen, output, rect, 0, 1)
}

func (d *ReagentMixingDialog) drawBody(screen *ebiten.Image) {
	view := &d.view
	content := d.window.body.Inset(view.padding)
	amount := d.amount()
	next := func() image.Rectangle {
		line := image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+view.lineHeight)
		content.Min.Y = line.Max.Y

		return line
	}
	d.drawLine(screen, next(), d.tab == mixTabRecipe, func(line *text.LineBuilder, ink mixInk) {
		line.Text(fmt.Sprintf(" %-*s", mixAmountColumn-1, "How many"), ink.label)
		if amount == 0 {
			line.Text("none possible", ink.value)

			return
		}
		line.Text(fmt.Sprintf("<  %-3d>", amount), ink.value).Text(fmt.Sprintf("    of %d", d.maximum()), ink.hint)
	})
	title := " INGREDIENTS"
	if d.tab == mixTabRecipe {
		title = " USES"
		d.drawLine(screen, next(), false, func(line *text.LineBuilder, ink mixInk) {
			prepared := strconv.Itoa(int(d.spell.Prepared))
			if amount > 0 {
				prepared += " -> " + strconv.Itoa(min(spellInventoryLimit, int(d.spell.Prepared)+amount))
			}
			line.Text(fmt.Sprintf(" %-*s", mixAmountColumn-1, "Prepared"), ink.label).Text(prepared, ink.value)
		})
	}
	next()
	d.drawLine(screen, next(), false, func(line *text.LineBuilder, ink mixInk) {
		columns := fmt.Sprintf("%*s", mixHaveColumns, "Have")
		if amount > 0 {
			columns += fmt.Sprintf("%*s%*s", mixCostColumns, "Use", mixCostColumns, "Left")
		}
		line.Text(fmt.Sprintf("%-*s", mixNameColumns, title), DialogHeaderColor).Text(columns, ink.label)
	})
	for index, reagent := range d.rows(d.tab) {
		d.drawIngredient(screen, next(), reagent, amount, d.tab == mixTabExperiment && index == d.cursor)
	}
	if d.tab == mixTabExperiment {
		drawSpellOutput(screen, view.body, content, 0, d.populateCandidates(content))
	}
}

// drawIngredient draws one table row: what the party has and, for an ingredient
// in the mixture, what the amount uses and leaves. Nothing is priced at zero:
// when no mixture is possible only Have is shown, with "none" where it is short.
func (d *ReagentMixingDialog) drawIngredient(screen *ebiten.Image, rect image.Rectangle,
	reagent ReagentInfo, amount int, selected bool,
) {
	checked := d.mask()&(1<<uint(reagent.Reagent)) != 0
	d.drawLine(screen, rect, selected, func(line *text.LineBuilder, ink mixInk) {
		line.Text(" ", ink.value)
		nameWidth := mixNameColumns - 1
		if d.tab == mixTabExperiment {
			appendReagentMark(line, checked, selected)
			nameWidth -= len("[X] ")
		}
		name := fmt.Sprintf("%-*s", nameWidth, reagent.Name)
		have := fmt.Sprintf("%*d", mixHaveColumns, reagent.Quantity)
		switch {
		case reagent.Quantity == 0 && checked:
			line.Text(name, ink.value).Text(fmt.Sprintf("%*s", mixHaveColumns, "none"), ink.warning)
		case reagent.Quantity == 0:
			line.Text(name+have+"   none carried", ink.dim)
		case checked && amount > 0:
			line.Text(name+have+fmt.Sprintf("%*d%*d", mixCostColumns, amount, mixCostColumns,
				reagent.Quantity-amount), ink.value)
		default:
			line.Text(name+have, ink.value)
		}
	})
}

// drawWindowFooter writes the hint rows by hand: the window is exactly wide
// enough for them, and "[Left/Right] or [0-9]" must stay one phrase. Keys that
// cannot work now are dimmed whole; the top line says why.
func (d *ReagentMixingDialog) drawWindowFooter(screen *ebiten.Image) {
	view := &d.view
	rect := d.window.footer.Inset(view.padding)
	prepareSpellOutput(view.footer, rect)
	amount := d.amount()
	if d.tab == mixTabExperiment {
		line := appendMixHint(view.footer.AppendLine(), hintKeyUpDown, " Move"+mixHintGap, true)
		appendMixHint(line, hintKeySpace, " Add or remove", true).Done()
	}
	line := appendMixHint(view.footer.AppendLine(), "Left/Right", " or ", amount > 0)
	appendMixHint(line, "0-9", " How many"+mixHintGap, amount > 0)
	appendMixHint(line, "M", " Most", min(d.maximum(), d.room()) > 0).Done()
	mix := " Mix"
	if amount > 0 {
		mix += " " + strconv.Itoa(amount)
	}
	line = appendMixHint(view.footer.AppendLine(), hintKeyEnter, mix+mixHintGap, amount > 0)
	appendMixHint(line, hintKeyEsc, " "+spellBackAction, true)
	if len(d.tabs()) > 1 {
		appendMixHint(line.Text(mixHintGap, DialogControlHintColor), "Tab", " "+d.otherTab().title(), true)
	}
	line.Done()
	drawSpellOutput(screen, view.footer, rect, 0, view.footer.GetPanelContentLineCount())
}

func appendMixHint(line *text.LineBuilder, key, action string, available bool) *text.LineBuilder {
	if !available {
		return line.Text("["+key+"]"+action, DialogUnavailableItemColor)
	}

	return line.Text("["+key+"]", DialogControlKeyColor).Text(action, DialogControlHintColor)
}
