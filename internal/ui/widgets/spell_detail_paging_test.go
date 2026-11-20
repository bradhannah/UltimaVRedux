package widgets

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestSpellDetailPagingUsesRealContentInBothDialogs(t *testing.T) {
	for _, consumer := range []string{"Cast", "Mix"} {
		for _, size := range config.SupportedWindowResolutions() {
			t.Run(fmt.Sprint(consumer, size), func(t *testing.T) {
				setResponsiveDialogTestResolution(t, size.X, size.Y)
				keyboard := newWidgetTestKeyboard(0)
				catalog := testMixCatalog()[:2]
				catalog[0].Description = "Light."
				catalog[0].CanMix, catalog[1].CanMix = 2, 2 // Both are on the list Mix opens on.
				catalog[1].Description = "First page.\n"
				for index := range 20 {
					catalog[1].Description += fmt.Sprintf("Detail %02d: %s\n", index,
						strings.Repeat("More words force measured wrapping. ", 4))
				}
				catalog[1].Description += "Last page."
				var widget Widget
				var browser *spellBrowser
				var view *spellPresentation
				var prepare func()
				var hasPendingRequest func() bool
				if consumer == "Cast" {
					_, cast, _ := newCastBrowserTest(t)
					setResponsiveDialogTestResolution(t, size.X, size.Y)
					cast.keyboard = keyboard
					cast.RefreshCatalog(catalog)
					widget, browser, view = cast, cast.browser, &cast.view
					prepare = cast.ensureLayout
					hasPendingRequest = cast.HasPendingCastRequest
				} else {
					mix := NewReagentMixingDialog(keyboard, catalog, fullTestReagentInventory())
					widget, browser, view = mix, mix.browser, &mix.view
					prepare = mix.ensureLayout
					hasPendingRequest = mix.HasPendingMixRequest
				}
				press := func(key ebiten.Key) {
					keyboard.SimulateKeyPress(key)
					widget.Update()
					prepare()
				}
				prepare()
				require.Equal(t, 1, browser.pages)
				require.NotContains(t, view.footer.GetOutputStr(false), "[Space]")
				require.Empty(t, view.pageIndicator.GetOutputStr(false))
				press(ebiten.KeySpace)
				require.Zero(t, browser.page)
				require.False(t, hasPendingRequest())
				press(ebiten.KeyDown)
				require.Greater(t, browser.pages, 1)
				require.Contains(t, view.footer.GetOutputStr(false), "[Space] Next detail page")
				pages, capacity := browser.pages, view.detailCapacity
				lines := strings.Split(view.body.GetOutputStr(false), "\n")
				require.Greater(t, len(lines), 40, "source paragraphs must actually wrap")
				var accessible []string
				for page := range pages {
					require.Equal(t, page, browser.page)
					require.Equal(t, fmt.Sprintf("Page %d/%d", page+1, pages), view.pageIndicator.GetOutputStr(false))
					require.Equal(t, capacity, view.detailCapacity)
					require.LessOrEqual(t, view.body.GetTotalHeightForLines(capacity), float64(view.detailBodyRect.Dy()))
					require.LessOrEqual(t, view.detailBodyRect.Max.Y, view.detailPageRect.Min.Y)
					start, end := page*capacity, min(len(lines), (page+1)*capacity)
					accessible = append(accessible, lines[start:end]...)
					// Geometry and reachability cover every preset/page; raster checks
					// cover the first and last slices at the tight and fractional sizes.
					if (size.X == 1280 || size.X == 1366) && (page == 0 || page == pages-1) {
						assertSpellDetailPagePixels(t, view, browser)
					}
					press(ebiten.KeySpace)
				}
				require.Equal(t, lines, accessible, "every wrapped line is reachable exactly once")
				require.Zero(t, browser.page, "last page wraps to first")
				press(ebiten.KeySpace)
				press(ebiten.KeyUp)
				require.Zero(t, browser.page)
				require.Equal(t, 1, browser.pages)
				require.Empty(t, view.pageIndicator.GetOutputStr(false))
				require.NotContains(t, view.footer.GetOutputStr(false), "[Space]")
				press(ebiten.KeySlash)
				press(ebiten.KeyQ)
				press(ebiten.KeySpace)
				require.Equal(t, "Q ", browser.query)
				require.Nil(t, browser.current())
				require.Empty(t, view.body.GetOutputStr(false))
				require.Equal(t, 1, browser.pages)
				require.Empty(t, view.pageIndicator.GetOutputStr(false))
				require.NotContains(t, view.footer.GetOutputStr(false), "[Space]")
				press(ebiten.KeyEscape)
				press(ebiten.KeyDown)
				require.Greater(t, browser.pages, 1)
				require.Contains(t, view.footer.GetOutputStr(false), "[Space] Next detail page")
				browser.refresh(catalog[:1])
				prepare()
				require.Equal(t, 1, browser.pages)
				require.Zero(t, browser.page)
				require.NotContains(t, view.footer.GetOutputStr(false), "[Space]")
			})
		}
	}
}

func TestSpellDetailSpaceHoldAdvancesOnlyOnce(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	catalog := testMixCatalog()[:1]
	catalog[0].Description = strings.Repeat("Overflow text. ", 100)
	mix := NewReagentMixingDialog(keyboard, catalog, nil)
	held[ebiten.KeySpace] = true
	for range 120 {
		mix.Update()
		keyboard.ClearFrameConsumption()
	}
	require.Equal(t, 1, mix.browser.page)
	clear(held)
	keyboard.ClearFrameConsumption()
	mix.Update()
	held[ebiten.KeySpace] = true
	mix.Update()
	require.Equal(t, 2%mix.browser.pages, mix.browser.page)
}

func assertSpellDetailPagePixels(t *testing.T, view *spellPresentation, browser *spellBrowser) {
	t.Helper()
	rect := view.detailBodyRect
	actual := ebiten.NewImageWithOptions(rect, nil)
	expected := ebiten.NewImageWithOptions(rect, nil)
	view.drawDetails(actual, browser)
	drawSpellOutput(expected, view.body, rect, browser.page*view.detailCapacity, view.detailCapacity)
	requireSameRowPixels(t, actual, expected, rect)
	actual.Dispose()
	expected.Dispose()
}

// Space pages a long description in the list and ticks an ingredient on the
// Experiment tab; neither meaning leaks into the other screen.
func TestMixSpacePagesTheListAndTicksInTheExperiment(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	catalog := testMixCatalog()
	catalog[0].Description = strings.Repeat("Overflow detail. ", 100)
	dialog := NewReagentMixingDialog(keyboard, catalog, fullTestReagentInventory())
	press := func(key ebiten.Key) {
		keyboard.SimulateKeyPress(key)
		dialog.Update()
		dialog.ensureLayout()
	}
	press(ebiten.KeySpace)
	require.Equal(t, 1, dialog.browser.page)
	press(ebiten.KeyEnter)
	require.True(t, dialog.preparing)
	press(ebiten.KeyTab)
	require.Equal(t, mixTabExperiment, dialog.tab)
	selection := dialog.selection
	press(ebiten.KeySpace)
	require.NotEqual(t, selection, dialog.selection)
	require.False(t, dialog.HasPendingMixRequest())
	press(ebiten.KeyEscape)
	require.False(t, dialog.preparing)
	require.Contains(t, dialog.view.footer.GetOutputStr(false), "[Space] Next detail page")
}

func TestSpellPagingResizeThresholdIsStableWithWrappingFooter(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	var view spellPresentation
	browser := newSpellBrowser([]game_state.ZStatsSpell{testMixCatalog()[0]})
	controls := spellFinderControls("Choose spell", "Close", false)
	// At fixed typography a taller viewport crosses an actual overflow boundary.
	// This also exercises a shorter logical viewport than canonical window sizing.
	for _, height := range []int{720, 360, 720, 360, 720} {
		view.layout(1280, height)
		for range 3 {
			view.configureDetails(browser, packedSpellFooter(controls))
			pages, footer, body := browser.pages, view.footerRect, view.detailBodyRect
			view.configureDetails(browser, packedSpellFooter(controls))
			require.Equal(t, pages, browser.pages)
			require.Equal(t, footer, view.footerRect)
			require.Equal(t, body, view.detailBodyRect)
			require.Less(t, browser.page, browser.pages)
			require.True(t, footer.In(image.Rect(0, 0, 1280, height)))
			if height == 720 {
				require.Equal(t, 1, pages)
				require.Empty(t, view.pageIndicator.GetOutputStr(false))
			} else {
				require.Greater(t, pages, 1)
				require.Contains(t, view.footer.GetOutputStr(false), "[Space] Next detail page")
				require.Greater(t, view.footer.GetPanelContentLineCount(), 1)
				browser.page = pages - 1
			}
		}
	}
}
