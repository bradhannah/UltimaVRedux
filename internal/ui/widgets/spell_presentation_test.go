package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestMixFilterCyclesAndCombinesWithSearchWithoutTypingChord(t *testing.T) {
	catalog := testMixCatalog()
	catalog[0].CanMix = 6
	catalog[1].Prepared = 2
	catalog[2].Prepared, catalog[2].CanMix = 99, 6
	catalog[3].Prepared, catalog[3].CanMix = 2, 6
	catalog[4].FormulaState, catalog[4].CanMix = "unknown", 6
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	dialog := NewReagentMixingDialog(keyboard, catalog, nil)
	press := func(keys ...ebiten.Key) {
		for _, key := range keys {
			held[key] = true
		}
		dialog.Update()
		dialog.Update() // A held chord must change the filter only once.
		clear(held)
		keyboard.ClearFrameConsumption()
	}
	require.Equal(t, spellFilterMixable, dialog.browser.filterMode, "Mix opens on what can be mixed")
	require.Equal(t, []int{0, 3}, dialog.browser.filtered)
	press(ebiten.KeyControl, ebiten.KeyO)
	require.Equal(t, spellFilterAll, dialog.browser.filterMode)
	press(ebiten.KeyControl, ebiten.KeyO)
	require.Equal(t, spellFilterPrepared, dialog.browser.filterMode)
	require.Equal(t, []int{1, 2, 3}, dialog.browser.filtered)
	dialog.browser.selectID(3)
	press(ebiten.KeyControl, ebiten.KeyO)
	require.Equal(t, spellFilterMixable, dialog.browser.filterMode)
	require.Equal(t, []int{0, 3}, dialog.browser.filtered)
	require.Equal(t, 3, dialog.browser.current().ID, "retain a selection that survives the filter")
	press(ebiten.KeySlash)
	for _, key := range []ebiten.Key{ebiten.KeyP, ebiten.KeyO, ebiten.KeyI, ebiten.KeyS, ebiten.KeyO, ebiten.KeyN} {
		press(key)
	}
	require.Equal(t, []int{3}, dialog.browser.filtered)
	press(ebiten.KeyControl, ebiten.KeyO)
	require.Equal(t, spellFilterAll, dialog.browser.filterMode)
	require.Len(t, dialog.browser.filtered, 3)
	require.Equal(t, "poison", strings.ToLower(dialog.browser.query))
	press(ebiten.KeyControl, ebiten.KeyO)
	press(ebiten.KeyControl, ebiten.KeyO)
	catalog[3].Prepared = 99
	dialog.browser.refresh(catalog)
	require.Equal(t, spellFilterAll, dialog.browser.filterMode, "words that match nothing mixable widen the list")
	require.Len(t, dialog.browser.filtered, 3)
	press(ebiten.KeyZ)
	require.Empty(t, dialog.browser.filtered)
	press(ebiten.KeyEnter)
	require.False(t, dialog.preparing, "an empty search must not enter manual experimentation")
	require.False(t, dialog.HasPendingMixRequest())
}

func TestSpellMixabilityExplainsUnconfirmedMissingAndFullStock(t *testing.T) {
	catalog := testMixCatalog()
	spell := catalog[0]
	spell.CanMix, spell.Prepared = 6, 98
	require.True(t, spellMixable(spell))
	require.Equal(t, 1, usefulSpellMixCount(spell))
	spell.Prepared = 99
	require.False(t, spellMixable(spell))
	require.Equal(t, "Stock full", spellMixReason(spell))
	spell.Prepared, spell.CanMix = 0, 0
	spell.MixMissing = []string{"Sulfurous Ash"}
	require.Equal(t, "Missing Sulfurous Ash", spellMixReason(spell))
	spell.CanMix, spell.FormulaState = 6, "uncertain"
	require.False(t, spellMixable(spell))
	require.Equal(t, "Formula unconfirmed", spellMixReason(spell))
}

func TestSpellHintsFieldsAndSelectionMarksHaveDistinctColours(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), testMixCatalog(),
		[]ReagentInfo{{Reagent: references.SulfurAsh, Name: "Sulfurous Ash", Quantity: 6}})
	dialog.ChooseSpell(references.InLor, false)
	dialog.handleQuantityInput(5)
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Dispose)
	dialog.Draw(screen)
	view := &dialog.view
	footer := func() []string {
		lines := strings.Split(view.footer.GetOutputStr(false), "\n")
		for index := range lines {
			lines[index] = strings.TrimSpace(lines[index])
		}

		return lines
	}
	require.Equal(t, []string{"[Left/Right/0-9] How many   [M] Most", "[Enter] Mix 5   [Esc] Back",
		"[Tab] Experiment"}, footer(), "Tab has the row Recipe leaves free")
	dialog.tab = mixTabExperiment
	dialog.Draw(screen)
	require.Equal(t, []string{"[Up/Down] Move   [Space] Add or remove", "[Left/Right/0-9] How many   [M] Most",
		"[Enter] Mix 5  [Esc] Back  [Tab] Recipe"}, footer(), "the gap that still fits the longest amount")
	for _, row := range footer() {
		require.LessOrEqual(t, len(row), mixWindowColumns, row)
	}
	hint := appendMixHint(view.footer.AppendLine(), "Enter", " Mix 5", true).GetSegments()
	require.Equal(t, "[Enter]", hint[0].Text)
	require.Equal(t, DialogControlKeyColor, hint[0].Color)
	require.Equal(t, " Mix 5", hint[1].Text)
	require.Equal(t, DialogControlHintColor, hint[1].Color)
	// A key that cannot work now is dimmed whole; the top line says why in words.
	dead := appendMixHint(view.footer.AppendLine(), "Enter", " Mix", false).GetSegments()
	require.Len(t, dead, 1)
	require.Equal(t, "[Enter] Mix", dead[0].Text)
	require.Equal(t, DialogUnavailableItemColor, dead[0].Color)
	field := appendSpellField(view.heading.AppendLine(), "Prepared", "5").GetSegments()
	require.Equal(t, DialogLabelColor, field[0].Color)
	require.Equal(t, DialogValueColor, field[1].Color)
	for _, selected := range []bool{false, true} {
		segments := appendReagentMark(view.row.AppendLine(), true, selected).GetSegments()
		mark, bracket := DialogCheckedMarkColor, DialogControlHintColor
		if selected {
			mark, bracket = DialogSelectedCheckedMarkColor, DialogSelectedText
		}
		require.Equal(t, "[", segments[0].Text)
		require.Equal(t, bracket, segments[0].Color)
		require.Equal(t, "X", segments[1].Text)
		require.Equal(t, mark, segments[1].Color)
		require.Equal(t, "] ", segments[2].Text)
		require.Equal(t, bracket, segments[2].Color)
		require.NotEqual(t, mark, bracket)
	}
}

// Every spell's name and meaning is shown whole in the Cast and Z-stats lists at
// every supported size: no dots and nothing cut. The tightest cases are a long
// name marked unavailable in Cast and a newly learned spell's "New:" meaning.
func TestSpellRowsShowEveryNameAndMeaningWhole(t *testing.T) {
	state, cast, _ := newCastBrowserTest(t)
	requireWhole := func(output, count *text.Output, rect image.Rectangle, spell game_state.ZStatsSpell, available bool) {
		t.Helper()
		effect := spell.EffectLabel
		if spell.Unread {
			effect = "New: " + effect
		}
		prepareSpellCatalogRow(output, count, rect, spell, false, available)
		lines := strings.Split(output.GetOutputStr(false), "\n")
		require.Len(t, lines, 2)
		require.Equal(t, unavailableRowLabel(spell.Name, available), strings.TrimRight(lines[0], " "))
		require.Equal(t, rowDetailIndent+effect, strings.TrimRight(lines[1], " "))
	}
	for _, size := range config.SupportedWindowResolutions() {
		setResponsiveDialogTestResolution(t, size.X, size.Y)
		cast.ensureLayout()
		castRow := cast.view.listRect.Inset(cast.view.padding)
		castRow.Max.X -= cast.view.scrollGutter()
		zstats := newTestZStatsDossierDialog(testZStatsDossier(), newWidgetTestKeyboard(0))
		zstats.switchTab(ZStatsDossierSpellsTab)
		zstatsRow := zstats.panelBodyRect(zstats.layout.globalListRect)
		zstatsRow.Min.X += zstats.layout.padding
		zstatsRow.Max.X -= zstats.scrollIndicatorWidth()
		for _, spell := range state.CurrentSpellCatalog() {
			spell.Prepared = 99
			id := references.Spell(spell.ID)
			// Only the three spells learned in play can be newly learned.
			spell.Unread = id == references.AnYlem || id == references.InXenMani || id == references.RelXenBet
			requireWhole(cast.view.row, cast.view.count, castRow, spell, true)
			requireWhole(cast.view.row, cast.view.count, castRow, spell, false)
			// Z-stats never marks a spell unavailable.
			requireWhole(zstats.rowOutput, zstats.spellView.count, zstatsRow, spell, true)
		}
		zstats.outerBorder.Dispose()
	}
}

func TestSpellRowsCenterBothLinesAndCountsTogether(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	var view spellPresentation
	view.layout(1280, 720)
	spell := testMixCatalog()[references.InLor]
	spell.Prepared = 9
	height := int(math.Ceil(view.row.GetTotalHeightForLines(2)))
	tight := ebiten.NewImage(400, height)
	padded := ebiten.NewImage(400, height+20)
	t.Cleanup(tight.Dispose)
	t.Cleanup(padded.Dispose)
	drawSpellCatalogRow(tight, view.row, view.count, tight.Bounds(), spell, false, true)
	drawSpellCatalogRow(padded, view.row, view.count, padded.Bounds(), spell, false, true)
	tightPixels, paddedPixels := renderedContentBounds(t, tight), renderedContentBounds(t, padded)
	require.Equal(t, tightPixels.Dy(), paddedPixels.Dy(), "no glyphs or descenders are clipped")
	require.Equal(t, tightPixels.Add(image.Pt(0, 10)), paddedPixels, "both lines and count move down by half the extra height")
}

func TestSpellAndIngredientLastRowsFullyRenderAcrossResolutions(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, size.X, size.Y)
			catalog := testMixCatalog()
			dialog := NewReagentMixingDialog(newWidgetTestKeyboard(0), catalog, fullTestReagentInventory())
			view := &dialog.view
			// Retain production absolute geometry, but allocate only the pane under test.
			screen := ebiten.NewImageWithOptions(view.listRect, nil)
			t.Cleanup(screen.Dispose)
			dialog.browser.selected = len(catalog) - 1
			dialog.Draw(screen)
			require.Less(t, view.listRect.Dx(), view.detailRect.Dx())
			require.Less(t, view.footer.GetPanelContentLineCount(), spellBrowserFooterLines,
				"finder hints must not reserve four full lines")
			list := view.listRect.Inset(view.padding)
			list.Min.Y += view.lineHeight
			start, end := zStatsVisibleRange(dialog.browser.selected, len(catalog), dialog.browser.visible)
			require.Equal(t, len(catalog), end)
			require.Contains(t, view.row.GetOutputStr(false), "An Tym")
			last := lastSpellRowRect(view, list, end-start)
			requireCompleteSpellRow(t, screen, last, view, catalog[len(catalog)-1], true, false)

			// The quantity window lists all eight ingredients with no scrolling.
			dialog.ChooseSpell(references.AnTym, false)
			dialog.tab, dialog.cursor = mixTabExperiment, 7
			dialog.ensureLayout()
			window := ebiten.NewImageWithOptions(dialog.window.body, nil)
			t.Cleanup(window.Dispose)
			dialog.Draw(window)
			body := dialog.window.body.Inset(view.padding)
			top := body.Min.Y + (mixExperimentHead+7)*view.lineHeight
			last = image.Rect(body.Min.X, top, body.Max.X, top+view.lineHeight)
			require.True(t, last.In(body), "the last ingredient is inside the pane")
			require.Contains(t, view.row.GetOutputStr(false), "Mandrake Root")
			expected := ebiten.NewImageWithOptions(last, nil)
			t.Cleanup(expected.Dispose)
			dialog.drawIngredient(expected, last, dialog.reagentInventory[7], dialog.amount(), true)
			requireSameRowPixels(t, window, expected, last)
		})
	}
}

func fullTestReagentInventory() []ReagentInfo {
	names := []string{"Sulfurous Ash", "Ginseng", "Garlic", "Spider Silk", "Blood Moss", "Black Pearl", "Nightshade", "Mandrake Root"}
	items := make([]ReagentInfo, len(names))
	for index, name := range names {
		items[index] = ReagentInfo{Reagent: references.Reagent(index), Name: name, Quantity: 8}
	}
	return items
}

func lastSpellRowRect(view *spellPresentation, body image.Rectangle, rows int) image.Rectangle {
	pitch := spellRowHeight(view.row)
	top := body.Min.Y + (rows-1)*pitch
	return image.Rect(body.Min.X, top, body.Max.X-view.scrollGutter(), top+pitch)
}

func requireCompleteSpellRow(t *testing.T, screen *ebiten.Image, rect image.Rectangle,
	view *spellPresentation, spell game_state.ZStatsSpell, selected, available bool,
) {
	t.Helper()
	expected := ebiten.NewImageWithOptions(rect, nil)
	t.Cleanup(expected.Dispose)
	drawSpellCatalogRow(expected, view.row, view.count, rect, spell, selected, available)
	requireSameRowPixels(t, screen, expected, rect)
}

func requireSameRowPixels(t *testing.T, screen, expected *ebiten.Image, rect image.Rectangle) {
	t.Helper()
	actualBytes, expectedBytes := make([]byte, rect.Dx()*rect.Dy()*4), make([]byte, rect.Dx()*rect.Dy()*4)
	screen.SubImage(rect).(*ebiten.Image).ReadPixels(actualBytes)
	expected.ReadPixels(expectedBytes)
	require.Equal(t, expectedBytes, actualBytes, "the complete last row must survive panel/footer clipping")
}

func TestSpellAvailabilityUsesCasterAndKeepsSelectedContrast(t *testing.T) {
	state, dialog, _ := newCastBrowserTest(t)
	state.PartyState.Inventory.Spells.Set(references.InLor, 1)
	spell := dialog.browser.catalog[0]
	require.True(t, dialog.spellAvailable(spell))
	state.PartyState.GetAvatar().SetMp(0)
	require.False(t, dialog.spellAvailable(spell))
	name, _ := spellRowColours(false, dialog.spellAvailable(spell))
	require.Equal(t, DialogUnavailableItemColor, name)
	name, _ = spellRowColours(true, false)
	require.Equal(t, DialogSelectedText, name)
	output := text.NewOutput(dialog.view.font, dialog.view.row.GetLineSpacing(), 2, 100)
	line := appendSpellField(output.AppendLine(), "Prepared", "5")
	segments := line.GetSegments()
	require.Equal(t, DialogLabelColor, segments[0].Color)
	require.Equal(t, DialogValueColor, segments[1].Color)
}

func TestSpellRecipeGroupsIndentedInventoryWithAvailabilityColours(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	font := GetScaledFont(FontSizeBody)
	output := text.NewOutput(font, GetScaledLineHeight(FontSizeBody), 32, 80)
	spell := testMixCatalog()[references.AnNox]
	spell.Reagents = []string{"Ginseng", "Garlic"}
	spell.RecipeInventory = []game_state.SpellReagentStock{
		{Name: "Ginseng", Quantity: 6}, {Name: "Garlic", Quantity: 0},
	}
	spell.Contexts = "Combat"
	appendSpellFormula(output, spell, DialogStatusWarningColor)
	content := output.GetOutputStr(false)
	lines := strings.Split(content, "\n")
	for index := range lines {
		lines[index] = strings.TrimRight(lines[index], " ")
	}
	require.Contains(t, strings.Join(lines, "\n"), "Recipe:\n  Ginseng (6)\n  Garlic (0)")
	require.NotContains(t, content, "Have 6")
	require.Contains(t, content, "Where: Combat")
	require.NotContains(t, content, "When:")
	for _, ingredient := range spell.RecipeInventory {
		segments := appendSpellIngredient(output.AppendLine(), ingredient, DialogStatusWarningColor).GetSegments()
		require.Equal(t, DialogValueColor, segments[0].Color)
		amountColor := DialogStatusOKColor
		if ingredient.Quantity == 0 {
			amountColor = DialogStatusWarningColor
		}
		require.Equal(t, amountColor, segments[1].Color)
	}
}

func TestSpellSpacePagesOutsideSearchAndTypesInsideSearch(t *testing.T) {
	_, dialog, press := newCastBrowserTest(t)
	dialog.browser.catalog[0].Description = strings.Repeat("A long spell description. ", 100)
	dialog.ensureLayout()
	require.Greater(t, dialog.browser.pages, 1)
	press(ebiten.KeySpace)
	require.Equal(t, 1, dialog.browser.page)
	dialog.ensureLayout()
	require.Contains(t, dialog.view.footer.GetOutputStr(false), "[Space] Next detail page")
	require.NotContains(t, dialog.view.footer.GetOutputStr(false), "Ctrl+Space")
	press(ebiten.KeySlash)
	press(ebiten.KeyP)
	press(ebiten.KeySpace)
	require.Equal(t, "P ", dialog.browser.query)
	dialog.ensureLayout()
	require.NotContains(t, dialog.view.footer.GetOutputStr(false), "[Space] Next detail page")
	press(ebiten.KeyEscape)
	dialog.browser.selectID(dialog.browser.catalog[0].ID)
	press(ebiten.KeySpace)
	require.Equal(t, 1, dialog.browser.page)
}
