package widgets

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestSpellSearchStripFieldsStayBoundedAcrossConsumersAndResolutions(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, size.X, size.Y)
			dossier := testZStatsDossier()
			dossier.Magic.Spells = testMixCatalog()[:47]
			dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			t.Cleanup(dialog.outerBorder.Dispose)
			dialog.switchTab(ZStatsDossierSpellsTab)
			view, browser := &dialog.spellView, dialog.spellBrowser
			for _, rect := range []image.Rectangle{
				dialog.layout.spellSearchRect.Inset(dialog.layout.padding),
				view.searchRect.Inset(view.padding), // unchanged Cast/Mix full-width placement
			} {
				var screen *ebiten.Image
				if size.X == 1366 || size.X == 2560 {
					screen = ebiten.NewImageWithOptions(rect, nil)
					t.Cleanup(screen.Dispose)
				}
				for _, count := range []int{0, 1, 9, 10, 47} {
					browser.filtered = make([]int, count)
					for _, mode := range []spellBrowserFilter{spellFilterAll, spellFilterPrepared, spellFilterMixable} {
						browser.filterMode = mode
						for _, query := range []string{"", "poison", "poison energy", strings.Repeat("écho words ", 24) + "latest words"} {
							browser.query, browser.searching = query, true
							inputRect, filterRect, matchesRect := view.prepareSearch(browser, rect)
							require.Equal(t, query, browser.query)
							shown := view.search.GetOutputStr(false)
							require.True(t, strings.HasPrefix(shown, "Search: "))
							require.True(t, strings.HasSuffix(shown, "_"))
							require.True(t, utf8.ValidString(shown))
							if len(query) > 128 {
								require.Contains(t, shown, "Search: ...")
								require.True(t, strings.HasSuffix(shown, "latest words_"))
							} else {
								require.Equal(t, "Search: "+query+"_", shown)
							}
							require.Equal(t, "Filter: "+mode.label(), view.filter.GetOutputStr(false))
							require.Equal(t, fmt.Sprintf("Matches: %d", count), view.matches.GetOutputStr(false))
							require.Equal(t, 1, view.search.GetPanelContentLineCount())
							require.Equal(t, 1, view.filter.GetPanelContentLineCount())
							require.Equal(t, 1, view.matches.GetPanelContentLineCount())
							require.Less(t, filterRect.Max.X, matchesRect.Min.X)
							require.LessOrEqual(t, inputRect.Max.Y, filterRect.Min.Y)
							for _, field := range []struct {
								value  string
								bounds image.Rectangle
							}{
								{shown, inputRect}, {view.filter.GetOutputStr(false), filterRect}, {view.matches.GetOutputStr(false), matchesRect},
							} {
								width, _ := view.font.MeasureString(field.value)
								require.LessOrEqual(t, width, float64(field.bounds.Dx()))
								require.True(t, field.bounds.In(rect))
							}
							// Draw the count alone to prove right alignment within its independent field.
							// Its alignment depends on count width, not the query or filter.
							if screen == nil || mode != spellFilterAll || query != "" {
								continue
							}
							screen.Clear()
							drawSpellOutput(screen, view.matches, matchesRect, 0, 1)
							pixels := renderedContentBounds(t, screen)
							require.True(t, pixels.In(matchesRect))
							require.LessOrEqual(t, matchesRect.Max.X-pixels.Max.X, view.padding)
						}
					}
				}
				browser.syllables, browser.searching = "IL", false
				view.prepareSearch(browser, rect)
				require.Contains(t, view.search.GetOutputStr(false), "Spell: IL")
				require.Contains(t, view.search.GetOutputStr(false), "In Lor")
			}
		})
	}
}

func TestZStatsSpellStripTracksKeyboardAndSurvivesTabsResizeAndMixRefresh(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	dossier := testZStatsDossier()
	dossier.Magic.Spells = testMixCatalog()[:47]
	for index := range dossier.Magic.Spells {
		dossier.Magic.Spells[index].Prepared, dossier.Magic.Spells[index].CanMix = 2, 3
	}
	dialog := newTestZStatsDossierDialog(dossier, keyboard)
	t.Cleanup(dialog.outerBorder.Dispose)
	outer := dialog.outerBorder.GetBounds()
	ordinary := dialog.layout.globalDetailRect
	dialog.switchTab(ZStatsDossierSpellsTab)
	strip, list, detail := dialog.layout.spellSearchRect, dialog.layout.globalListRect, dialog.layout.globalDetailRect
	require.Equal(t, strip.Min.X, list.Min.X)
	require.Equal(t, strip.Max.X, detail.Max.X)
	require.Equal(t, strip.Max.Y-dialog.pixelUnit(), list.Min.Y)
	require.Equal(t, list.Min.Y, detail.Min.Y)
	require.Greater(t, detail.Min.Y, ordinary.Min.Y)
	prepare := func() {
		dialog.ensureLayout()
		dialog.spellView.prepareSearch(dialog.spellBrowser, dialog.layout.spellSearchRect.Inset(dialog.layout.padding))
		dialog.prepareSpellDetail(dialog.layout.globalDetailRect)
	}
	press := func(keys ...ebiten.Key) {
		for _, key := range keys {
			held[key] = true
		}
		dialog.Update()
		clear(held)
		keyboard.ClearFrameConsumption()
		prepare()
		require.Equal(t, fmt.Sprintf("Matches: %d", len(dialog.spellBrowser.filtered)), dialog.spellView.matches.GetOutputStr(false))
	}
	prepare()
	require.Equal(t, "Spell: _  "+spellInitialsPrompt, dialog.spellView.search.GetOutputStr(false))
	require.Equal(t, "Matches: 47", dialog.spellView.matches.GetOutputStr(false))
	press(ebiten.KeySlash)
	for _, key := range []ebiten.Key{ebiten.KeyP, ebiten.KeyO, ebiten.KeyI, ebiten.KeyS, ebiten.KeyO, ebiten.KeyN} {
		press(key)
	}
	require.Equal(t, "Search: POISON_", dialog.spellView.search.GetOutputStr(false))
	press(ebiten.KeySpace)
	press(ebiten.KeyQ)
	require.Contains(t, dialog.spellView.search.GetOutputStr(false), "POISON Q_")
	require.Equal(t, "Matches: 0", dialog.spellView.matches.GetOutputStr(false))
	require.Nil(t, dialog.selectedSpell())
	require.Empty(t, dialog.detailOutput.GetOutputStr(false))
	require.Equal(t, "No matching spells.", dialog.emptyListText())
	press(ebiten.KeyBackspace)
	require.Equal(t, "Search: POISON _", dialog.spellView.search.GetOutputStr(false))
	for _, mode := range []spellBrowserFilter{spellFilterPrepared, spellFilterMixable, spellFilterAll} {
		press(ebiten.KeyControl, ebiten.KeyO)
		require.Equal(t, mode, dialog.spellBrowser.filterMode)
		require.Equal(t, "Search: POISON _", dialog.spellView.search.GetOutputStr(false))
	}
	press(ebiten.KeyControl, ebiten.KeyU)
	require.Equal(t, "Search: _", dialog.spellView.search.GetOutputStr(false))
	require.Equal(t, list, dialog.layout.globalListRect)
	require.Equal(t, detail, dialog.layout.globalDetailRect)
	require.Equal(t, outer, dialog.outerBorder.GetBounds())
	press(ebiten.KeyPageDown)
	require.Equal(t, dialog.visibleListRows(list), dialog.GetSelectedListIndex())
	selected := dialog.selectedSpell().ID
	dialog.switchTab(ZStatsDossierArmoryTab)
	require.Equal(t, ordinary, dialog.layout.globalDetailRect)
	require.Equal(t, outer, dialog.outerBorder.GetBounds())
	dialog.switchTab(ZStatsDossierSpellsTab)
	require.True(t, dialog.spellBrowser.searching)
	require.Equal(t, selected, dialog.selectedSpell().ID)
	press(ebiten.KeyControl, ebiten.KeyM)
	require.Equal(t, references.Spell(selected), dialog.ConsumePendingMixSpell())
	dialog.RefreshDossier(dossier)
	require.Equal(t, selected, dialog.selectedSpell().ID)
	require.True(t, dialog.spellBrowser.searching)
	setResponsiveDialogTestResolution(t, 1920, 1080)
	prepare()
	require.Equal(t, selected, dialog.selectedSpell().ID)
	t.Cleanup(dialog.outerBorder.Dispose)
	require.Equal(t, "Search: _", dialog.spellView.search.GetOutputStr(false))
	require.Greater(t, dialog.layout.spellSearchRect.Dx(), strip.Dx())
	press(ebiten.KeyEscape)
	require.False(t, dialog.spellBrowser.searching)
	require.False(t, dialog.IsDismissed())
}

func TestZStatsSpellTitleNeverAcknowledgesUnreadAndCustomPlacementIsHonored(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	for _, unread := range []int{0, 1, 2} {
		dossier := testZStatsDossier()
		dossier.UnreadHiddenSpellCount = unread
		dossier.Magic.Spells[0].Unread = true
		original := cloneZStatsDossier(dossier)
		configuration := testZStatsDossierConfig(newWidgetTestKeyboard(0))
		configuration.Placement = sprites.PercentBasedPlacement{StartPercentX: .05, StartPercentY: .05, EndPercentX: .95, EndPercentY: .95}
		dialog := NewZStatsDossierDialog(dossier, configuration)
		t.Cleanup(dialog.outerBorder.Dispose)
		require.Equal(t, *sprites.GetRectangleFromPercents(configuration.Placement), dialog.outerBorder.GetBounds())
		require.Equal(t, dialog.bodyFont.TextFace.Size, dialog.tabOutput.Font.TextFace.Size)
		dialog.switchTab(ZStatsDossierSpellsTab)
		require.Equal(t, "Spells", dialog.tabTitle(ZStatsDossierSpellsTab))
		dialog.prepareListRow(image.Rect(100, 100, 900, 150), 0, false)
		require.Contains(t, dialog.rowOutput.GetOutputStr(false), "New:")
		require.Equal(t, original, dossier)
		require.Equal(t, original, dialog.dossier)
	}
}

func TestZStatsLastSpellRowUsesTheFinalStripAdjustedCapacity(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setZStatsTestResolution(t, size.X, size.Y)
		dossier := testZStatsDossier()
		dossier.Magic.Spells = testMixCatalog()
		dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
		t.Cleanup(dialog.outerBorder.Dispose)
		dialog.switchTab(ZStatsDossierSpellsTab)
		dialog.tabSelections[ZStatsDossierSpellsTab] = len(dossier.Magic.Spells) - 1
		body := dialog.panelBodyRect(dialog.layout.globalListRect)
		capacity := dialog.visibleListRows(dialog.layout.globalListRect)
		pitch := spellRowHeight(dialog.rowOutput)
		row := image.Rect(body.Min.X+dialog.layout.padding, body.Min.Y+(capacity-1)*pitch,
			body.Max.X-dialog.scrollIndicatorWidth(), body.Min.Y+capacity*pitch)
		require.True(t, row.In(body))
		if size.X != 1366 && size.X != 2560 {
			continue
		}
		actual := ebiten.NewImageWithOptions(row, nil)
		expected := ebiten.NewImageWithOptions(row, nil)
		dialog.drawListPane(actual, dialog.layout.globalListRect)
		drawSpellCatalogRow(expected, dialog.rowOutput, dialog.spellView.count, row, dossier.Magic.Spells[len(dossier.Magic.Spells)-1], true, true)
		requireSameRowPixels(t, actual, expected, row)
		actual.Dispose()
		expected.Dispose()
	}
}
