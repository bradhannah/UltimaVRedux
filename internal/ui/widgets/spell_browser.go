package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"slices"
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
	spellBrowserInsetDivisor   = 3 // The inset is a third of a letter.
	spellBrowserTopInsets      = 2 // The header and the list start under the top line.
	spellBrowserListRatio      = 0.40
	spellInventoryLimit        = 99
	spellFormulaUnknown        = "Unknown"
)

type spellBrowserFilter int

const (
	spellFilterAll spellBrowserFilter = iota
	spellFilterPrepared
	spellFilterMixable
	// spellFilterReady lists what the chosen caster can cast right now. It exists
	// only where that can be judged: the Cast list.
	spellFilterReady
)

func (filter spellBrowserFilter) label() string {
	switch filter {
	case spellFilterAll:
		return "All known"
	case spellFilterPrepared:
		return "Prepared"
	case spellFilterMixable:
		return "Mixable"
	case spellFilterReady:
		return "Ready"
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
	navigation input.ListNavigation
	catalog    []game_state.ZStatsSpell
	filtered   []int
	selected   int
	syllables  string
	query      string
	searching  bool
	filterMode spellBrowserFilter
	ready      func(game_state.ZStatsSpell) bool // Nil where no caster is chosen.
	// readyFirst lists what the caster can cast now ahead of the rest of All known.
	readyFirst bool
	// Mix's list only: Mixable widens like Ready, and a finished mix keeps its
	// spell listed (keepID, -1 for none).
	widenMixable bool
	keepID       int
	searchInput  *TextInput
	page         int
	pages        int
	visible      int
}

// The initials field says what it finds and what to type into it.
const (
	spellInitialsLabel  = "Spell"
	spellInitialsPrompt = "type its initials, such as VF for Vas Flam"
)

func newSpellBrowser(catalog []game_state.ZStatsSpell) *spellBrowser {
	//nolint:exhaustruct_v5 // Search starts empty; refresh installs the detached catalog.
	b := &spellBrowser{pages: 1, visible: 1, keepID: -1}
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

// refilter applies the filter again and keeps the selected spell when it is
// still listed, for when the filter or what it depends on has changed.
func (b *spellBrowser) refilter() {
	spellID := -1
	if spell := b.current(); spell != nil {
		spellID = spell.ID
	}
	b.filter()
	b.selectID(spellID)
}

// nextFilter is where Ctrl+O goes from the current filter. A list with a caster
// offers Ready and leaves Mixable to Mix.
func (b *spellBrowser) nextFilter() spellBrowserFilter {
	order := []spellBrowserFilter{spellFilterAll, spellFilterPrepared, spellFilterMixable}
	if b.ready != nil {
		order = []spellBrowserFilter{spellFilterAll, spellFilterReady, spellFilterPrepared}
	}

	return order[(slices.Index(order, b.filterMode)+1)%len(order)]
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

// filteredOut reports whether the current filter leaves the spell off the list.
func (b *spellBrowser) filteredOut(spell game_state.ZStatsSpell) bool {
	if spell.ID == b.keepID {
		return false
	}
	switch b.filterMode {
	case spellFilterAll:
		return false
	case spellFilterPrepared:
		return spell.Prepared == 0
	case spellFilterMixable:
		return !spellMixable(spell)
	case spellFilterReady:
		return !b.ready(spell)
	default:
		panic("invalid spell browser filter")
	}
}

// filter lists the spells the filter and the typed words allow. Ready is a
// convenience, not a wall: when the words match none of the ready spells, the
// list widens to every known spell, so a spell the caster knows can always be
// found by name and its row says why it cannot be cast. Mix's list treats
// Mixable the same way.
func (b *spellBrowser) filter() {
	b.applyFilter()
	typed := b.syllables != "" || (b.searching && b.query != "")
	convenience := b.filterMode == spellFilterReady || b.widenMixable && b.filterMode == spellFilterMixable
	if convenience && typed && len(b.filtered) == 0 {
		b.filterMode = spellFilterAll
		b.applyFilter()
	}
}

func (b *spellBrowser) applyFilter() {
	b.filtered = b.filtered[:0]
	for index, spell := range b.catalog {
		if b.filteredOut(spell) {
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
	if b.readyFirst && b.filterMode == spellFilterAll {
		b.filtered = slices.Concat(b.partition(true), b.partition(false))
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

// partition keeps the listed spells the caster can, or cannot, cast now.
func (b *spellBrowser) partition(ready bool) []int {
	var kept []int
	for _, index := range b.filtered {
		if b.ready(b.catalog[index]) == ready {
			kept = append(kept, index)
		}
	}

	return kept
}

// clearTyped empties the initials and leaves any search, for a list that starts over.
func (b *spellBrowser) clearTyped() {
	b.syllables, b.query, b.searching = "", "", false
	if b.searchInput != nil {
		b.searchInput.Clear()
	}
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
		b.filterMode = b.nextFilter()
		b.refilter()

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

// spellFooter writes a dialog's hint rows; paged asks for the detail paging key.
type spellFooter func(output *text.Output, width int, paged bool)

// packedSpellFooter wraps complete hints into as few rows as the width allows.
func packedSpellFooter(controls []ControlHint) spellFooter {
	return func(output *text.Output, width int, paged bool) {
		if paged {
			controls = append(controls, ControlHint{hintKeySpace, hintActionDetails})
		}
		PackControlHints(output, controls, width, etext.AlignStart)
	}
}

type spellPresentation struct {
	// warning is the red of a shortage in the spell details: the shared status
	// red unless the owner asks for another before the first layout.
	warning stdcolor.Color
	// entry writes the selected spell's details under its name. Nil writes the
	// full entry that Mix and Z-Stats share.
	entry                          func(output *text.Output, spell game_state.ZStatsSpell)
	font                           *text.UltimaFont
	row, count, body, heading      *text.Output
	search, footer, title          *text.Output
	filter, matches, casters       *text.Output
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
	// inset keeps text a third of a letter off its frame: at the sides of every
	// pane and under the top line of the header, list and details.
	inset int
}

// inner is the room for text inside a pane's frame. top also leaves the inset
// under the top line, where a heading would otherwise touch it.
func (p *spellPresentation) inner(pane image.Rectangle, top bool) image.Rectangle {
	rect := pane.Inset(p.padding)
	rect.Min.X, rect.Max.X = rect.Min.X+p.inset, rect.Max.X-p.inset
	if top {
		rect.Min.Y += p.inset
	}

	return rect
}

func (p *spellPresentation) layout(width, height int) {
	if p.warning == nil {
		p.warning = DialogStatusWarningColor
	}
	if p.font != nil && p.width == width && p.height == height {
		return
	}
	p.width, p.height = width, height
	p.font, p.row = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines: spellBrowserRowLines, MaxChars: spellBrowserTextCapacity,
	})
	spacing := p.row.GetLineSpacing()
	p.lineHeight = max(1, int(math.Ceil(spacing)))
	p.padding = max(zStatsDossierMinimumPadding, width/spellBrowserPaddingDivisor)
	letter, _ := p.font.MeasureString("M")
	p.inset = max(1, int(math.Round(letter/spellBrowserInsetDivisor)))
	p.count = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.indicator = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.body = text.NewOutput(p.font, spacing, spellBrowserDetailCapacity, spellBrowserTextCapacity)
	p.heading = text.NewOutput(p.font, spacing, spellBrowserRowLines, spellBrowserTextCapacity)
	p.search = text.NewOutput(p.font, spacing, spellBrowserRowLines, spellBrowserTextCapacity)
	p.filter = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.matches = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.casters = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	p.pageIndicator = text.NewOutput(p.font, spacing, 1, spellBrowserTextCapacity)
	// The two hint rows stand a little apart.
	p.footer = text.NewOutput(p.font, spacing+float64(p.inset),
		spellBrowserFooterLines*spellBrowserRowLines, spellBrowserTextCapacity)
	_, p.title = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeTitle, LineSpacingMultiplier: zStatsDossierLineSpacingMultiplier,
		MaxLines: spellBrowserRowLines, MaxChars: spellBrowserTextCapacity,
	})
	marginX, marginY := int(float64(width)*zStatsDossierDefaultMarginX), int(float64(height)*zStatsDossierDefaultMarginY)
	// The window grows by what the two top insets and the gap between two hint
	// rows take, so the list and details keep their rows and lines.
	marginY -= (spellBrowserTopInsets*p.inset + p.inset + 1) / spellBrowserRowLines
	rect := image.Rect(marginX, marginY, width-marginX, height-marginY)
	headerHeight := int(math.Ceil(p.title.GetTotalHeightForLines(spellBrowserRowLines))) +
		p.padding*spellBrowserRowLines + p.inset
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

// configureControls measures the styled hint rows before row capacity is computed.
// Detail paging is shown only when Space actually pages details in this mode.
func (p *spellPresentation) configureControls(footer spellFooter, detailPaging bool) {
	rect := p.inner(p.footerRect, false)
	prepareSpellOutput(p.footer, rect)
	footer(p.footer, rect.Dx(), detailPaging)
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
	label, value := spellInitialsLabel, browser.syllables
	if browser.searching {
		label, value = "Search", browser.query
	} else if value != "" {
		words := make([]string, 0, len(value))
		for _, letter := range value {
			words = append(words, references.GetSyllableMap()[letter])
		}
		value += " -> " + strings.Join(words, " ")
	}
	line := appendSpellField(p.search.AppendLine(), label, spellInputViewport(p.font, label, value, inputRect.Dx()))
	prompt := "  " + spellInitialsPrompt
	if width, _ := p.font.MeasureString(label + ": _" + prompt); value == "" && !browser.searching &&
		width <= float64(inputRect.Dx()) {
		line.Text(prompt, DialogControlHintColor)
	}
	line.Done()
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

// prepareSpellCatalogRow lays out a spell's two lines and its prepared count. The
// count sits at the right of the first line only, so the name stops short of the
// number and the meaning below has the whole row: neither gives up room to the
// width of the column heading. Text that still cannot fit is cut plainly.
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
	count := strconv.Itoa(int(spell.Prepared))
	font := rowOutput.Font
	nameColor, effectColor := spellRowColours(selected, available)
	rect = spellRowContentRect(rowOutput, rect)
	countWidth, _ := font.MeasureString(count)
	nameWidth := rect.Dx() - int(math.Ceil(countWidth)) - spellBrowserCountGap
	prepareSpellOutput(rowOutput, rect)
	rowOutput.AppendLine().Text(
		CutDialogLine(font, unavailableRowLabel(spell.Name, available), "", nameWidth), nameColor).Done()
	rowOutput.AppendLine().Text(CutDialogLine(font, rowDetailIndent+effect, "", rect.Dx()), effectColor).Done()
	headingWidth, _ := font.MeasureString("Prepared")
	countRect := image.Rect(rect.Max.X-int(math.Ceil(headingWidth)), rect.Min.Y, rect.Max.X, rect.Max.Y)
	prepareSpellOutput(countOutput, countRect)
	countOutput.AppendLine().WithAlignment(etext.AlignEnd).Text(count, nameColor).Done()

	return rect, countRect
}

func appendSpellDetails(output *text.Output, spell game_state.ZStatsSpell, warning stdcolor.Color) {
	output.AppendLine().Text(spell.Description, DialogValueColor).WordWrap().Done()
	appendSpellMetrics(output, spell, warning)
	appendSpellFormula(output, spell, warning)
}

func appendSpellMetrics(output *text.Output, spell game_state.ZStatsSpell, warning stdcolor.Color) {
	line := appendSpellField(output.AppendLine(), "Prepared", strconv.Itoa(int(spell.Prepared)))
	line.Text("   ", DialogValueColor)
	appendSpellField(line, "Can mix", strconv.Itoa(usefulSpellMixCount(spell))).WordWrap().Done()
	line = appendSpellField(output.AppendLine(), "Circle", strconv.Itoa(spell.Circle))
	line.Text("   ", DialogValueColor)
	appendSpellField(line, "MP cost", strconv.Itoa(spell.MPCost)).WordWrap().Done()
	if reason := spellMixReason(spell); reason != "" {
		appendSpellWarningField(output.AppendLine(), "Mixing", reason, warning).WordWrap().Done()
	}
}

func appendSpellFormula(output *text.Output, spell game_state.ZStatsSpell, warning stdcolor.Color) {
	var formula string
	switch spell.FormulaState {
	case string(game_state.SpellFormulaConfirmed):
		formula = "Known"
		if !spell.Hidden {
			formula = "Book of Lore"
		}
	case string(game_state.SpellFormulaUncertain):
		formula = "Untested"
	default:
		formula = spellFormulaUnknown
	}
	appendSpellField(output.AppendLine(), "Formula", formula).WordWrap().Done()
	if len(spell.Reagents) == 0 && len(spell.ReagentCandidates) == 0 {
		appendSpellWarningField(output.AppendLine(), "Recipe", "Unknown", warning).Done()
	} else {
		output.AppendLine().Text("Recipe:", DialogLabelColor).Done()
	}
	for index, candidate := range spell.ReagentCandidates {
		appendSpellWarningField(output.AppendLine(), fmt.Sprintf("  Untested %d", index+1),
			strings.Join(candidate, " + "), warning).WordWrap().Done()
	}
	if len(spell.RecipeInventory) == 0 {
		for _, reagent := range spell.Reagents {
			output.AppendLine().Text("  "+reagent, DialogValueColor).WordWrap().Done()
		}
	} else {
		for _, ingredient := range spell.RecipeInventory {
			appendSpellIngredient(output.AppendLine(), ingredient, warning).WordWrap().Done()
		}
	}
	if len(spell.MixMissing) > 0 {
		appendSpellWarningField(output.AppendLine(), "Missing", strings.Join(spell.MixMissing, ", "), warning).
			WordWrap().Done()
	}
	appendSpellAim(output, spell)
}

// appendSpellAim says what the spell is aimed at, what it reaches and where it
// may be cast: the lines every spell entry ends with.
func appendSpellAim(output *text.Output, spell game_state.ZStatsSpell) {
	appendSpellField(output.AppendLine(), "Target", spell.Targeting).WordWrap().Done()
	if !spell.Unaimed {
		appendSpellField(output.AppendLine(), "Affects", spellScopeLabel(spell.Scope)).WordWrap().Done()
	}
	appendSpellField(output.AppendLine(), "Where", spell.Contexts).WordWrap().Done()
}

func appendSpellIngredient(
	line *text.LineBuilder, ingredient game_state.SpellReagentStock, warning stdcolor.Color,
) *text.LineBuilder {
	amountColor := DialogStatusOKColor
	if ingredient.Quantity == 0 {
		amountColor = warning
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
		return "One creature"
	case references.ScopeSingleTile:
		return "One square"
	case references.ScopeCone:
		return "All before you"
	case references.ScopeGlobal:
		return "Everyone"
	case references.ScopeRandomPlacement:
		return "Somewhere nearby"
	case references.ScopeWorldTransition:
		return "The party"
	default:
		return "Unknown"
	}
}

// Always start with the unpaged footer. Only genuine overflow adds the paging
// control and its indicator, so footer wrapping cannot feed back across frames.
func (p *spellPresentation) configureDetails(browser *spellBrowser, footer spellFooter) {
	p.configureControls(footer, false)
	p.measureDetails(browser, false)
	if browser.pages > 1 {
		p.configureControls(footer, !browser.searching)
		p.measureDetails(browser, true)
	}
	browser.page = max(0, min(browser.page, browser.pages-1))
	p.pageIndicator.Reset()
	if browser.pages > 1 {
		prepareSpellOutput(p.pageIndicator, p.detailPageRect)
		p.pageIndicator.AppendLine().Textf("Page %d/%d", DialogControlHintColor, browser.page+1, browser.pages).Done()
	}
	list := p.inner(p.listRect, true)
	browser.visible = max(1, (list.Dy()-p.lineHeight)/spellRowHeight(p.row))
}

func (p *spellPresentation) measureDetails(browser *spellBrowser, paged bool) {
	rect := p.inner(p.detailRect, true)
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
	if p.entry != nil {
		p.entry(p.body, *spell)
	} else {
		appendSpellDetails(p.body, *spell, p.warning)
	}
	p.detailCapacity = max(1, p.body.GetLineCapacity(rect.Dy()))
	browser.pages = max(1, (p.body.GetPanelContentLineCount()+p.detailCapacity-1)/p.detailCapacity)
}

func (p *spellPresentation) drawDetails(screen *ebiten.Image, browser *spellBrowser) {
	spell := browser.current()
	if spell == nil {
		return
	}
	rect := p.inner(p.detailRect, true)
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
	rect := p.inner(p.footerRect, false)
	drawSpellOutput(screen, p.footer, rect, 0, p.footer.GetPanelContentLineCount())
}
