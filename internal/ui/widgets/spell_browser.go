package widgets

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	spellBrowserRowLines       = 2
	spellBrowserFooterLines    = 4
	spellBrowserTextCapacity   = 128
	spellBrowserDetailCapacity = 512
	spellBrowserCountGap       = 4
	spellBrowserPaddingDivisor = 160
	spellBrowserListRatio      = 0.40
	spellInventoryLimit        = 99
)

type spellBrowserFilter int

const (
	spellFilterAll spellBrowserFilter = iota
	spellFilterPrepared
	spellFilterMixable
)

func (filter spellBrowserFilter) label() string {
	switch filter {
	case spellFilterAll:
		return "All known"
	case spellFilterPrepared:
		return "Prepared"
	case spellFilterMixable:
		return "Mixable"
	default:
		panic("invalid spell browser filter")
	}
}

func usefulSpellMixCount(spell game_state.ZStatsSpell) int {
	if spell.FormulaState != string(game_state.SpellFormulaConfirmed) || spell.RecipeMask == 0 {
		return 0
	}

	return max(0, min(spell.CanMix, spellInventoryLimit-int(spell.Prepared)))
}

func spellMixable(spell game_state.ZStatsSpell) bool { return usefulSpellMixCount(spell) > 0 }

func spellMixReason(spell game_state.ZStatsSpell) string {
	if spell.FormulaState != string(game_state.SpellFormulaConfirmed) || spell.RecipeMask == 0 {
		return "Formula unconfirmed"
	}
	if spell.Prepared >= spellInventoryLimit {
		return "Stock full"
	}
	if spell.CanMix == 0 {
		if len(spell.MixMissing) > 0 {
			return "Missing " + strings.Join(spell.MixMissing, ", ")
		}

		return "Insufficient ingredients"
	}

	return ""
}

// spellBrowser owns only selection/search state. All three spell entry points
// consume the same detached catalog and use the same rows and detail vocabulary.
type spellBrowser struct {
	navigation  input.ListNavigation
	catalog     []game_state.ZStatsSpell
	filtered    []int
	selected    int
	syllables   string
	query       string
	searching   bool
	filterMode  spellBrowserFilter
	searchInput *TextInput
	page        int
	pages       int
	visible     int
}

func newSpellBrowser(catalog []game_state.ZStatsSpell) *spellBrowser {
	//nolint:exhaustruct_v5 // Search starts empty; refresh installs the detached catalog.
	b := &spellBrowser{pages: 1, visible: 1}
	b.refresh(catalog)

	return b
}

func (b *spellBrowser) current() *game_state.ZStatsSpell {
	if b.selected < 0 || b.selected >= len(b.filtered) {
		return nil
	}

	return &b.catalog[b.filtered[b.selected]]
}

func (b *spellBrowser) refresh(catalog []game_state.ZStatsSpell) {
	spellID := -1
	if selected := b.current(); selected != nil {
		spellID = selected.ID
	}
	b.catalog = cloneSpellCatalog(catalog)
	b.filter()
	b.selectID(spellID)
}

func (b *spellBrowser) selectID(id int) {
	for position, index := range b.filtered {
		if b.catalog[index].ID == id {
			if b.selected != position {
				b.page, b.pages = 0, 1
			}
			b.selected = position

			return
		}
	}
}

func (b *spellBrowser) filter() {
	b.filtered = b.filtered[:0]
	for index, spell := range b.catalog {
		if b.filterMode == spellFilterPrepared && spell.Prepared == 0 ||
			b.filterMode == spellFilterMixable && !spellMixable(spell) {
			continue
		}
		matches := true
		if b.searching {
			haystack := strings.ToLower(spell.Name + " " + spell.Description + " " + spell.EffectLabel)
			for _, word := range strings.Fields(strings.ToLower(b.query)) {
				matches = matches && strings.Contains(haystack, word)
			}
		} else {
			matches = strings.HasPrefix(spellInitials(spell.Name), b.syllables)
		}
		if matches {
			b.filtered = append(b.filtered, index)
		}
	}
	b.selected = min(b.selected, max(0, len(b.filtered)-1))
	if !b.searching && b.syllables != "" {
		for position, index := range b.filtered {
			if spellInitials(b.catalog[index].Name) == b.syllables {
				b.selected = position

				break
			}
		}
	}
	b.page, b.pages = 0, 1
}

func spellInitials(name string) string {
	var initials strings.Builder
	for _, word := range strings.Fields(name) {
		initials.WriteByte(word[0])
	}

	return initials.String()
}

// handleInput leaves Enter, plain Escape and screen-specific shortcuts to the owner.
func (b *spellBrowser) handleInput(keyboard *input.Keyboard, font *text.UltimaFont) bool {
	if b.handleSearchControls(keyboard, font) || b.navigate(keyboard) {
		return true
	}
	if b.searching {
		b.searchInput.Update()
		if query := b.searchInput.GetText(); query != b.query {
			b.query = query
			b.selected = 0
			b.filter()
		}

		return true
	}

	return b.handleSyllableInput(keyboard)
}

func (b *spellBrowser) handleSearchControls(keyboard *input.Keyboard, font *text.UltimaFont) bool {
	if b.searching && keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		b.leaveSearch()

		return true
	}
	if keyboard.IsModifierPressed(input.ModifierCtrl) && keyboard.IsTextKeyPressedAndRegister(ebiten.KeyO) {
		spellID := -1
		if spell := b.current(); spell != nil {
			spellID = spell.ID
		}
		b.filterMode = (b.filterMode + 1) % (spellFilterMixable + 1)
		b.filter()
		b.selectID(spellID)

		return true
	}
	if !b.searching && keyboard.IsKeyPressedAndRegister(ebiten.KeySlash) {
		b.searching = true
		if b.searchInput == nil {
			placement := sprites.PercentBasedPlacement{StartPercentX: 0, StartPercentY: 0, EndPercentX: 1, EndPercentY: 1}
			//nolint:exhaustruct_v5 // The browser owns submission, completion and rendering.
			b.searchInput = NewTextInput(placement, font.TextFace.Size, spellBrowserTextCapacity,
				nil, TextInputCallbacks{}, keyboard)
			b.searchInput.SetFreeTextMode()
		}
		b.searchInput.Clear()
		b.query = ""
		b.filter()

		return true
	}

	return false
}

func (b *spellBrowser) leaveSearch() {
	spellID := -1
	if spell := b.current(); spell != nil {
		spellID = spell.ID
		if !strings.HasPrefix(spellInitials(spell.Name), b.syllables) {
			b.syllables = ""
		}
	}
	b.searching = false
	b.filter()
	b.selectID(spellID)
}

func (b *spellBrowser) handleSyllableInput(keyboard *input.Keyboard) bool {
	if keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		b.page = (b.page + 1) % max(1, b.pages)

		return true
	}
	if keyboard.IsKeyPressedAndRegister(ebiten.KeyBackspace) {
		if b.syllables != "" {
			b.syllables = b.syllables[:len(b.syllables)-1]
			b.selected = 0
			b.filter()
		}

		return true
	}
	if keyboard.IsModifierPressed(input.ModifierCtrl) {
		return false
	}
	for key := ebiten.KeyA; key <= ebiten.KeyZ; key++ {
		if keyboard.IsTextKeyPressedAndRegister(key) {
			letter := rune('A' + int(key-ebiten.KeyA)) //nolint:gosec // Key is bounded to A-Z by the loop.
			if references.IsValidSyllable(letter) && len(b.syllables) < references.MaxSpellSyllables {
				b.syllables += string(letter)
				b.selected = 0
				b.filter()
			}

			return true
		}
	}

	return false
}

func (b *spellBrowser) navigate(keyboard *input.Keyboard) bool {
	key, owned := b.navigation.Poll(keyboard)
	if key != nil {
		step := listNavigationDelta(*key, max(1, b.visible))
		b.selected = max(0, min(len(b.filtered)-1, b.selected+step))
		b.page, b.pages = 0, 1
	}

	return owned
}

type spellPresentation struct {
	font                           *text.UltimaFont
	row, count, body, heading      *text.Output
	search, footer, title          *text.Output
	filter, matches, status        *text.Output
	indicator                      *text.Output
	pageIndicator                  *text.Output
	detailBodyRect, detailPageRect image.Rectangle
	detailCapacity                 int
	width, height                  int
	headerRect, searchRect         image.Rectangle
	listRect, detailRect           image.Rectangle
	footerRect                     image.Rectangle
	frames                         []image.Rectangle
	padding, lineHeight            int
}

func (p *spellPresentation) layout(width, height int) {
	if p.font != nil && p.width == width && p.height == height {
		return
	}
	p.width, p.height = width, height
	p.font, p.row = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacing: 0, LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines: spellBrowserRowLines, MaxChars: spellBrowserTextCapacity,
	})
	spacing := p.row.GetLineSpacing()
	p.lineHeight = max(1, int(math.Ceil(spacing)))
	p.padding = max(zStatsDossierMinimumPadding, width/spellBrowserPaddingDivisor)
	p.count = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.indicator = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.body = text.NewOutput(p.font, spacing, spellBrowserDetailCapacity, spellBrowserTextCapacity)
	p.heading = text.NewOutput(p.font, spacing, spellBrowserRowLines, spellBrowserTextCapacity)
	p.search = text.NewOutput(p.font, spacing, spellBrowserRowLines, spellBrowserTextCapacity)
	p.filter = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.matches = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.status = text.NewOutput(p.font, spacing, spellBrowserRowLines, spellBrowserTextCapacity)
	p.pageIndicator = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.footer = text.NewOutput(p.font, spacing, spellBrowserFooterLines*spellBrowserRowLines, spellBrowserTextCapacity)
	_, p.title = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeTitle, LineSpacing: 0, LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines: spellBrowserRowLines, MaxChars: spellBrowserTextCapacity,
	})
	marginX, marginY := int(float64(width)*zStatsDossierDefaultMarginX), int(float64(height)*zStatsDossierDefaultMarginY)
	rect := image.Rect(marginX, marginY, width-marginX, height-marginY)
	headerHeight := int(math.Ceil(p.title.GetTotalHeightForLines(spellBrowserRowLines))) + p.padding*spellBrowserRowLines
	searchHeight := p.lineHeight*spellBrowserRowLines + p.padding*spellBrowserRowLines
	footerHeight := int(math.Ceil(p.footer.GetTotalHeightForLines(1))) + p.padding*spellBrowserRowLines
	p.headerRect = image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+headerHeight)
	p.searchRect = image.Rect(rect.Min.X, p.headerRect.Max.Y, rect.Max.X, p.headerRect.Max.Y+searchHeight)
	p.footerRect = image.Rect(rect.Min.X, rect.Max.Y-footerHeight, rect.Max.X, rect.Max.Y)
	split := rect.Min.X + int(math.Round(float64(rect.Dx())*spellBrowserListRatio))
	p.listRect = image.Rect(rect.Min.X, p.searchRect.Max.Y, split, p.footerRect.Min.Y)
	p.detailRect = image.Rect(split, p.searchRect.Max.Y, rect.Max.X, p.footerRect.Min.Y)
	p.frames = connectClassicPaneGrid([][]image.Rectangle{
		{p.headerRect}, {p.searchRect}, {p.listRect, p.detailRect}, {p.footerRect},
	}, getClassicUIPixelUnit(width, height))
}

// configureControls measures wrapped, styled hints before row capacity is computed.
// Detail paging is shown only when Space actually pages details in this mode.
func (p *spellPresentation) configureControls(controls []ControlHint, detailPaging bool) {
	rect := p.footerRect.Inset(p.padding)
	prepareSpellOutput(p.footer, rect)
	if detailPaging {
		controls = append(controls, ControlHint{hintKeySpace, "Next detail page"})
	}
	PackControlHints(p.footer, controls, rect.Dx(), etext.AlignStart)
	height := int(math.Ceil(p.footer.GetTotalHeightForLines(p.footer.GetPanelContentLineCount())))
	p.footerRect.Min.Y = p.footerRect.Max.Y - height - p.padding*spellBrowserRowLines
	p.listRect.Max.Y, p.detailRect.Max.Y = p.footerRect.Min.Y, p.footerRect.Min.Y
	p.frames = connectClassicPaneGrid([][]image.Rectangle{
		{p.headerRect}, {p.searchRect}, {p.listRect, p.detailRect}, {p.footerRect},
	}, getClassicUIPixelUnit(p.width, p.height))
}

func (p *spellPresentation) drawFrame(screen *ebiten.Image) {
	unit := getClassicUIPixelUnit(p.width, p.height)
	fillClassicPaneBackgrounds(screen, p.frames, unit, pkgcolor.DialogBackground.RGBA())
}

func (p *spellPresentation) finishFrame(screen *ebiten.Image) {
	drawClassicPaneFrames(screen, p.frames, getClassicUIPixelUnit(p.width, p.height), DialogRuleColor)
}

func drawSpellOutput(screen *ebiten.Image, output *text.Output, rect image.Rectangle, start, count int) {
	clipped := rect.Intersect(screen.Bounds())
	if clipped.Empty() {
		return
	}
	clippedImage, ok := screen.SubImage(clipped).(*ebiten.Image)
	if !ok {
		panic("Spell browser: Ebitengine clip is not an image")
	}
	output.DrawPanelContentRange(clippedImage, rect.Min, rect.Dx(), etext.AlignStart, start, count)
}

func prepareSpellOutput(output *text.Output, rect image.Rectangle) {
	output.Reset()
	output.SetMaxCharsPerLine(max(1, output.Font.CharsForWidth(max(1, rect.Dx()))))
}

func (p *spellPresentation) drawSearch(screen *ebiten.Image, browser *spellBrowser, rect image.Rectangle) {
	inputRect, filterRect, matchesRect := p.prepareSearch(browser, rect)
	drawSpellOutput(screen, p.search, inputRect, 0, 1)
	drawSpellOutput(screen, p.filter, filterRect, 0, 1)
	drawSpellOutput(screen, p.matches, matchesRect, 0, 1)
}

func (p *spellPresentation) prepareSearch(
	browser *spellBrowser, rect image.Rectangle,
) (image.Rectangle, image.Rectangle, image.Rectangle) {
	inputRect, filterRect, matchesRect := p.searchFields(rect, len(browser.filtered))
	prepareSpellOutput(p.search, inputRect)
	label, value := "Words", browser.syllables
	if browser.searching {
		label, value = "Search", browser.query
	} else if value != "" {
		words := make([]string, 0, len(value))
		for _, letter := range value {
			words = append(words, references.GetSyllableMap()[letter])
		}
		value += " -> " + strings.Join(words, " ")
	}
	appendSpellField(p.search.AppendLine(), label, spellInputViewport(p.font, label, value, inputRect.Dx())).Done()
	prepareSpellOutput(p.filter, filterRect)
	appendSpellField(p.filter.AppendLine(), "Filter", browser.filterMode.label()).Done()
	prepareSpellOutput(p.matches, matchesRect)
	appendSpellField(p.matches.AppendLine().WithAlignment(etext.AlignEnd),
		"Matches", strconv.Itoa(len(browser.filtered))).Done()

	return inputRect, filterRect, matchesRect
}

func (p *spellPresentation) searchFields(
	rect image.Rectangle, matches int,
) (image.Rectangle, image.Rectangle, image.Rectangle) {
	inputRect := rect
	inputRect.Max.Y = min(rect.Max.Y, rect.Min.Y+p.lineHeight)
	filterRect := rect
	filterRect.Min.Y = inputRect.Max.Y
	countWidth, _ := p.font.MeasureString("Matches: " + strconv.Itoa(matches))
	matchesRect := filterRect
	matchesRect.Min.X = max(rect.Min.X, rect.Max.X-int(math.Ceil(countWidth)))
	filterRect.Max.X = max(filterRect.Min.X, matchesRect.Min.X-p.padding)

	return inputRect, filterRect, matchesRect
}

// Fit both the measured viewport and Output's conservative wrapping capacity.
// The model retains the complete query; only its presentation scrolls horizontally.
func spellInputViewport(font *text.UltimaFont, label, value string, width int) string {
	query := []rune(value)
	prefix := label + ": "
	marker := ""
	for {
		visible := marker + string(query) + "_"
		measured, _ := font.MeasureString(prefix + visible)
		fits := measured <= float64(width) && utf8.RuneCountInString(prefix+visible) <= font.CharsForWidth(width)
		if len(query) == 0 || fits {
			return visible
		}
		query = query[1:]
		marker = "..."
	}
}

func (p *spellPresentation) drawList(screen *ebiten.Image, browser *spellBrowser,
	rect image.Rectangle, available func(game_state.ZStatsSpell) bool,
) {
	content := rect
	content.Max.X -= p.scrollGutter()
	prepareSpellOutput(p.heading, content)
	p.heading.AppendLine().Text("SPELL", DialogHeaderColor).Done()
	drawSpellOutput(screen, p.heading, content, 0, 1)
	p.drawPreparedHeading(screen, content)
	rect.Min.Y += p.lineHeight
	rowHeight := spellRowHeight(p.row)
	browser.visible = max(1, rect.Dy()/rowHeight)
	start, end := zStatsVisibleRange(browser.selected, len(browser.filtered), browser.visible)
	for position := start; position < end; position++ {
		top := rect.Min.Y + (position-start)*rowHeight
		row := image.Rect(rect.Min.X, top, content.Max.X, min(rect.Max.Y, top+rowHeight))
		spell := browser.catalog[browser.filtered[position]]
		drawSpellCatalogRow(screen, p.row, p.count, row,
			spell, position == browser.selected, available(spell))
	}
	if len(browser.filtered) == 0 {
		prepareSpellOutput(p.row, rect)
		p.row.AppendLine().Text("No matching spells", DialogEmptyColor).Done()
		drawSpellOutput(screen, p.row, rect, 0, 1)
	}
	drawMasterDetailScrollIndicators(screen, rect, start, end, len(browser.filtered), rowHeight,
		p.scrollGutter(), p.font, p.indicator)
}

func (p *spellPresentation) scrollGutter() int {
	width, _ := p.font.MeasureString("M")

	return int(math.Ceil(width)) + p.padding
}

func (p *spellPresentation) drawPreparedHeading(screen *ebiten.Image, rect image.Rectangle) {
	countWidth, _ := p.font.MeasureString("Prepared")
	countRect := image.Rect(rect.Max.X-int(math.Ceil(countWidth)), rect.Min.Y, rect.Max.X, rect.Min.Y+p.lineHeight)
	prepareSpellOutput(p.count, countRect)
	p.count.AppendLine().Text("Prepared", DialogLabelColor).Done()
	drawSpellOutput(screen, p.count, countRect, 0, 1)
}

func drawSpellCatalogRow(screen *ebiten.Image, rowOutput, countOutput *text.Output,
	rect image.Rectangle, spell game_state.ZStatsSpell, selected, available bool,
) {
	if selected {
		fillSelectedRow(screen, rect)
	}
	nameRect, countRect := prepareSpellCatalogRow(rowOutput, countOutput, rect, spell, selected, available)
	drawSpellOutput(screen, rowOutput, nameRect, 0, spellBrowserRowLines)
	drawSpellOutput(screen, countOutput, countRect, 0, 1)
}

func prepareSpellCatalogRow(rowOutput, countOutput *text.Output,
	rect image.Rectangle, spell game_state.ZStatsSpell, selected, available bool,
) (image.Rectangle, image.Rectangle) {
	effect := spell.EffectLabel
	if effect == "" {
		effect = references.SpellEffectLabel(references.Spell(spell.ID))
	}
	if spell.Unread {
		effect = "New: " + effect
	}

	return prepareSpellTextRow(rowOutput, countOutput, rect,
		spell.Name, effect, strconv.Itoa(int(spell.Prepared)), selected, available)
}

func prepareSpellTextRow(rowOutput, countOutput *text.Output,
	rect image.Rectangle, name, effect, count string, selected, available bool,
) (image.Rectangle, image.Rectangle) {
	nameColor, effectColor := spellRowColours(selected, available)
	rect = spellRowContentRect(rowOutput, rect)
	countWidth, _ := rowOutput.Font.MeasureString("Prepared")
	nameRect := rect
	if count != "" {
		nameRect.Max.X -= int(math.Ceil(countWidth)) + spellBrowserCountGap
	}
	prepareSpellOutput(rowOutput, nameRect)
	name = unavailableRowLabel(name, available)
	rowOutput.AppendLine().Text(FitDialogLine(name, rowOutput.Font, nameRect.Dx()), nameColor).Done()
	rowOutput.AppendLine().Text(FitDialogLine(effect, rowOutput.Font, nameRect.Dx()), effectColor).Done()
	if count == "" {
		return nameRect, image.Rect(0, 0, 0, 0)
	}
	countRect := image.Rect(nameRect.Max.X+spellBrowserCountGap, rect.Min.Y, rect.Max.X, rect.Max.Y)
	prepareSpellOutput(countOutput, countRect)
	countOutput.AppendLine().WithAlignment(etext.AlignEnd).Text(count, nameColor).Done()

	return nameRect, countRect
}

func appendSpellDetails(output *text.Output, spell game_state.ZStatsSpell) {
	output.AppendLine().Text(spell.Description, DialogValueColor).WordWrap().Done()
	appendSpellMetrics(output, spell)
	appendSpellFormula(output, spell)
}

func appendSpellMetrics(output *text.Output, spell game_state.ZStatsSpell) {
	line := appendSpellField(output.AppendLine(), "Prepared", strconv.Itoa(int(spell.Prepared)))
	line.Text("   ", DialogValueColor)
	appendSpellField(line, "Can mix", strconv.Itoa(usefulSpellMixCount(spell))).WordWrap().Done()
	line = appendSpellField(output.AppendLine(), "Circle", strconv.Itoa(spell.Circle))
	line.Text("   ", DialogValueColor)
	appendSpellField(line, "MP cost", strconv.Itoa(spell.MPCost)).WordWrap().Done()
	if reason := spellMixReason(spell); reason != "" {
		appendSpellWarningField(output.AppendLine(), "Mixing", reason).WordWrap().Done()
	}
}

func appendSpellFormula(output *text.Output, spell game_state.ZStatsSpell) {
	formula := spell.FormulaState
	if !spell.Hidden && formula == string(game_state.SpellFormulaConfirmed) {
		formula = "Book of Lore"
	}
	appendSpellField(output.AppendLine(), "Formula", formula).WordWrap().Done()
	if len(spell.Reagents) == 0 && len(spell.ReagentCandidates) == 0 {
		appendSpellWarningField(output.AppendLine(), "Recipe", "Unknown").Done()
	} else {
		output.AppendLine().Text("Recipe:", DialogLabelColor).Done()
	}
	for index, candidate := range spell.ReagentCandidates {
		appendSpellWarningField(output.AppendLine(), fmt.Sprintf("  Untested %d", index+1),
			strings.Join(candidate, " + ")).WordWrap().Done()
	}
	if len(spell.RecipeInventory) == 0 {
		for _, reagent := range spell.Reagents {
			output.AppendLine().Text("  "+reagent, DialogValueColor).WordWrap().Done()
		}
	} else {
		for _, ingredient := range spell.RecipeInventory {
			appendSpellIngredient(output.AppendLine(), ingredient).WordWrap().Done()
		}
	}
	if len(spell.MixMissing) > 0 {
		appendSpellWarningField(output.AppendLine(), "Missing", strings.Join(spell.MixMissing, ", ")).WordWrap().Done()
	}
	appendSpellField(output.AppendLine(), "Target", spell.Targeting).WordWrap().Done()
	appendSpellField(output.AppendLine(), "Affects", spellScopeLabel(spell.Scope)).WordWrap().Done()
	appendSpellField(output.AppendLine(), "When", spell.Contexts).WordWrap().Done()
}

func appendSpellIngredient(line *text.LineBuilder, ingredient game_state.SpellReagentStock) *text.LineBuilder {
	amountColor := DialogStatusOKColor
	if ingredient.Quantity == 0 {
		amountColor = DialogStatusWarningColor
	}

	return line.Text("  "+ingredient.Name+" ", DialogValueColor).Textf("(%d)", amountColor, ingredient.Quantity)
}

func spellScopeLabel(scope references.SpellEffectScope) string {
	switch scope {
	case references.ScopeCaster:
		return spellCasterLabel
	case references.ScopeParty:
		return "Party"
	case references.ScopeSingleCharacter:
		return "One character"
	case references.ScopeSingleCombatant:
		return "One combatant"
	case references.ScopeSingleTile:
		return "One tile"
	case references.ScopeCone:
		return "Directional cone"
	case references.ScopeGlobal:
		return "Global"
	case references.ScopeRandomPlacement:
		return "Random placement"
	case references.ScopeWorldTransition:
		return "World travel"
	default:
		return "Unknown"
	}
}

// Always start with the unpaged footer. Only genuine overflow adds the paging
// control and its indicator, so footer wrapping cannot feed back across frames.
func (p *spellPresentation) configureDetails(browser *spellBrowser, controls []ControlHint,
	statusHeight int, extra func(*text.Output),
) {
	p.configureControls(controls, false)
	p.measureDetails(browser, statusHeight, extra, false)
	if browser.pages > 1 {
		p.configureControls(controls, !browser.searching)
		p.measureDetails(browser, statusHeight, extra, true)
	}
	browser.page = max(0, min(browser.page, browser.pages-1))
	p.pageIndicator.Reset()
	if browser.pages > 1 {
		prepareSpellOutput(p.pageIndicator, p.detailPageRect)
		p.pageIndicator.AppendLine().Textf("Page %d/%d", DialogControlHintColor, browser.page+1, browser.pages).Done()
	}
	list := p.listRect.Inset(p.padding)
	browser.visible = max(1, (list.Dy()-p.lineHeight)/spellRowHeight(p.row))
}

func (p *spellPresentation) measureDetails(
	browser *spellBrowser, statusHeight int, extra func(*text.Output), paged bool,
) {
	rect := p.detailRect.Inset(p.padding)
	rect.Max.Y -= statusHeight
	spell := browser.current()
	prepareSpellOutput(p.body, rect)
	if spell == nil {
		browser.page, browser.pages = 0, 1

		return
	}
	rect.Min.Y += p.lineHeight * spellBrowserRowLines
	p.detailPageRect = image.Rect(rect.Min.X, rect.Max.Y-p.lineHeight, rect.Max.X, rect.Max.Y)
	if paged {
		rect.Max.Y = p.detailPageRect.Min.Y
	}
	p.detailBodyRect = rect
	appendSpellDetails(p.body, *spell)
	if extra != nil {
		extra(p.body)
	}
	p.detailCapacity = max(1, p.body.GetLineCapacity(rect.Dy()))
	browser.pages = max(1, (p.body.GetPanelContentLineCount()+p.detailCapacity-1)/p.detailCapacity)
}

func (p *spellPresentation) drawDetails(screen *ebiten.Image, browser *spellBrowser) {
	spell := browser.current()
	if spell == nil {
		return
	}
	rect := p.detailRect.Inset(p.padding)
	prepareSpellOutput(p.heading, rect)
	p.heading.AppendLine().Text(FitDialogLine(spell.Name, p.font, rect.Dx()), DialogHeaderColor).Done()
	p.heading.AppendLine().Text(FitDialogLine(spell.EffectLabel, p.font, rect.Dx()), DialogControlHintColor).Done()
	drawSpellOutput(screen, p.heading, rect, 0, spellBrowserRowLines)
	drawSpellOutput(screen, p.body, p.detailBodyRect, browser.page*p.detailCapacity, p.detailCapacity)
	if browser.pages > 1 {
		drawSpellOutput(screen, p.pageIndicator, p.detailPageRect, 0, 1)
	}
}

func cloneSpellCatalog(catalog []game_state.ZStatsSpell) []game_state.ZStatsSpell {
	cloned := append([]game_state.ZStatsSpell(nil), catalog...)
	for index, spell := range catalog {
		cloned[index].Reagents = append([]string(nil), spell.Reagents...)
		cloned[index].RecipeInventory = append([]game_state.SpellReagentStock(nil), spell.RecipeInventory...)
		cloned[index].MixMissing = append([]string(nil), spell.MixMissing...)
		cloned[index].ReagentCandidates = make([][]string, len(spell.ReagentCandidates))
		for candidateIndex, candidate := range spell.ReagentCandidates {
			cloned[index].ReagentCandidates[candidateIndex] = append([]string(nil), candidate...)
		}
	}

	return cloned
}

func (p *spellPresentation) drawFooter(screen *ebiten.Image) {
	rect := p.footerRect.Inset(p.padding)
	drawSpellOutput(screen, p.footer, rect, 0, p.footer.GetPanelContentLineCount())
}
