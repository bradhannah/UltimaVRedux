package widgets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
	"golang.org/x/image/font/sfnt"
)

func atlasCleanupDossier() journal.AtlasDossier {
	return journal.AtlasDossier{FixUnavailable: "Requires night (19:00-05:59)", Entries: []journal.AtlasEntry{
		{Key: "moonglow", Name: "Towne 1", Kind: "towne", Category: journal.AtlasSettlements,
			Charted: true, Survey: atlasMeasuredSite(232, 135)},
		{Key: "britain", Name: "Towne 2", Kind: "towne", Category: journal.AtlasSettlements,
			Charted: true, Survey: atlasMeasuredSite(81, 106)},
		{Key: "shrine", Name: "Shrine 1", Kind: "shrine", Category: journal.AtlasShrines,
			Charted: true, Survey: atlasMeasuredSite(233, 66)},
		{Key: "hut", Name: "Unnamed hut", Kind: "hut", Category: journal.AtlasSettlements},
	}}
}

func TestAtlasOpeningPhysicalKeysSelectWithoutTabAndResetWithZero(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	keys := map[ebiten.Key]bool{ebiten.KeyShiftLeft: true, ebiten.KeyM: true}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
	keyboard.PrepareForTransition(0)
	view := AtlasViewState{}
	dialog := NewAtlasDialog(atlasCleanupDossier(), nil, keyboard, &view, func() {})
	dialog.onDialogFocusChanged(true)
	step := func(held ...ebiten.Key) {
		clear(keys)
		for _, key := range held {
			keys[key] = true
		}
		keyboard.ClearFrameConsumption()
		dialog.Update()
	}
	press := func(key ebiten.Key) { step(); step(key); step() }
	step(ebiten.KeyShiftLeft, ebiten.KeyM)
	require.False(t, dialog.closed, "held opener must not dismiss the new reader")
	step()
	require.Equal(t, "moonglow", view.SelectedKey)
	press(ebiten.KeyDown)
	require.Equal(t, "britain", view.SelectedKey, "first arrow after opener release selects immediately")
	press(ebiten.KeyUp)
	require.Equal(t, "moonglow", view.SelectedKey)
	press(ebiten.KeyTab)
	require.Equal(t, journal.AtlasNotePlaces, view.Filters.Places, "one Tab selects the next category: the player's notes")
	press(ebiten.KeyTab)
	require.Equal(t, journal.AtlasTownes, view.Filters.Places)
	press(ebiten.KeyEqual)
	before := view.CenterY
	press(ebiten.KeyP)
	require.True(t, dialog.panning)
	step(ebiten.KeyDown)
	require.NotEqual(t, before, view.CenterY)
	require.Equal(t, "moonglow", view.SelectedKey, "pan mode preserves place selection")
	step()
	press(ebiten.KeyP)
	require.False(t, dialog.panning)
	press(ebiten.KeyDown)
	require.Equal(t, "britain", view.SelectedKey, "plain arrows always select, even after panning")
	step(ebiten.KeyShiftRight, ebiten.KeyTab)
	require.Equal(t, journal.AtlasNotePlaces, view.Filters.Places)
	step()
	step(ebiten.KeyShiftRight, ebiten.KeyTab)
	require.Equal(t, journal.AtlasAllPlaces, view.Filters.Places)
	step()
	for _, zero := range []ebiten.Key{ebiten.KeyDigit0, ebiten.KeyNumpad0} {
		view.Zoom, view.CenterX, view.CenterY = 4, 17, 29
		press(zero)
		require.Equal(t, 1, view.Zoom)
		require.Equal(t, float64(128), view.CenterX)
		require.Equal(t, float64(128), view.CenterY)
		require.Equal(t, "britain", view.SelectedKey)
	}
}

func TestAtlasCategoryTabsKeepKnownUnmeasuredRowsWithoutExtraSelectors(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	view := NewAtlasViewState()
	dialog := NewAtlasDialog(atlasCleanupDossier(), nil, keyboard, &view, func() {})
	press := func(key ebiten.Key) { keyboard.SimulateKeyPress(key); dialog.Update() }
	order := atlasTabOrder()
	require.Equal(t, journal.AtlasNotePlaces, order[1], "the player's notes follow All")
	press(ebiten.KeyTab)
	press(ebiten.KeyTab)
	require.Equal(t, journal.AtlasTownes, view.Filters.Places)
	require.Len(t, dialog.entries, 3, "supplied places and known unmeasured rows need no extra checkbox")
	_, plottable := dialog.entries[2].PlotPosition()
	require.False(t, plottable, "a discovered hut row must not gain a precise pin")
	for _, choice := range order[3:] {
		press(ebiten.KeyTab)
		require.Equal(t, choice, view.Filters.Places)
	}
	press(ebiten.KeyTab)
	require.Equal(t, journal.AtlasAllPlaces, view.Filters.Places)
	require.Len(t, dialog.entries, 4)
	before := view
	press(ebiten.KeyF)
	press(ebiten.KeySpace)
	require.Equal(t, before, view, "there is no hidden filter submenu or selection mode")
	press(ebiten.KeyL)
	require.Equal(t, AtlasLabelsNone, view.Labels)
	press(ebiten.KeyL)
	require.Equal(t, AtlasLabelsSelected, view.Labels, "there is no Auto label mode")
}

func TestAtlasCategoryTabsReverseWrapAndDoNotRepeatWhileHeld(t *testing.T) {
	for _, shift := range []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyShiftRight} {
		setQuestJournalTestResolution(t, 1280, 720)
		keys := map[ebiten.Key]bool{}
		keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
		view := NewAtlasViewState()
		dialog := NewAtlasDialog(atlasCleanupDossier(), nil, keyboard, &view, func() {})
		order := atlasTabOrder()
		for index := len(order) - 1; index >= 0; index-- {
			clear(keys)
			keyboard.ClearFrameConsumption()
			dialog.Update()
			keys[shift], keys[ebiten.KeyTab] = true, true
			for range 5 {
				keyboard.ClearFrameConsumption()
				dialog.Update()
				require.Equal(t, order[index], view.Filters.Places, "one chord selects once until release")
			}
		}
		require.Equal(t, journal.AtlasAllPlaces, view.Filters.Places)
		require.Equal(t, float64(128), view.CenterX)
		require.Equal(t, float64(128), view.CenterY, "category changes never jump the map camera")
	}
}

func TestAtlasCategoryTabsShowEveryFullLabelAtSupportedSizes(t *testing.T) {
	labels := []string{"All", "Settlements", "Keeps", "Castles", "Shrines", "Lighthouses", "Dungeons", "Moonstones", "Vessels", "My Notes"}
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setQuestJournalTestResolution(t, size.X, size.Y)
			view := NewAtlasViewState()
			dialog := NewAtlasDialog(atlasCleanupDossier(), nil, newWidgetTestKeyboard(0), &view, func() {})
			for choice := journal.AtlasAllPlaces; choice < journal.AtlasPlaceFilterCount; choice++ {
				require.Equal(t, labels[choice], choice.String())
				cell := dialog.tabs[choice]
				width, height := dialog.font.MeasureString(choice.String())
				require.LessOrEqual(t, width, float64(cell.Dx()))
				require.LessOrEqual(t, height, float64(cell.Dy()))
				require.GreaterOrEqual(t, cell.Min.X, 0)
				require.LessOrEqual(t, cell.Max.X, size.X)
				require.LessOrEqual(t, cell.Max.Y, dialog.mapRect.Min.Y)
				if choice > 0 && dialog.tabs[choice-1].Min.Y == cell.Min.Y {
					require.Less(t, dialog.tabs[choice-1].Max.X, cell.Min.X)
				}
			}
		})
	}
}

func TestAtlasFooterUsesTwoMeasuredPhysicalRowsAndLargerMap(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setQuestJournalTestResolution(t, size.X, size.Y)
			view := NewAtlasViewState()
			dialog := NewAtlasDialog(atlasCleanupDossier(), nil, newWidgetTestKeyboard(0), &view, func() {})
			require.GreaterOrEqual(t, dialog.mapRect.Dy(), size.Y*3/4, "the map must reclaim the old footer reservation")
			geometry := dialog.mapRect
			for category := journal.AtlasAllPlaces; category < journal.AtlasPlaceFilterCount; category++ {
				for _, zoom := range []int{1, 2, 4} {
					for _, fix := range []bool{true, false} {
						view.Filters.Places, view.Zoom = category, zoom
						dialog.dossier.CurrentFix.Measured = fix
						dialog.status = strings.Repeat("Long status should never enlarge footer. ", 5)
						dialog.buildFooter()
						require.Equal(t, 2, dialog.footer.GetPanelContentLineCount())
						for _, line := range strings.Split(dialog.footer.GetOutputStr(false), "\n") {
							width, _ := dialog.font.MeasureString(line)
							require.LessOrEqual(t, width, float64(dialog.footerRect.Dx()))
						}
						require.Equal(t, geometry, dialog.mapRect)
						require.NotContains(t, dialog.footer.GetOutputStr(false), "Home")
						require.NotContains(t, dialog.footer.GetOutputStr(false), "PgUp")
						if !fix {
							require.NotContains(t, dialog.footer.GetOutputStr(false), "[A] Avatar")
						}
					}
				}
			}
		})
	}
}

func TestAtlasVisibleTextUsesGlyphsPresentInActualFont(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "text", "assets", "ultima-v-warriors-of-destiny", "ultima-v-warriors-of-destiny.ttf"))
	require.NoError(t, err)
	font, err := sfnt.Parse(data)
	require.NoError(t, err)
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	dossier := atlasCleanupDossier()
	dossier.Entries = append(dossier.Entries, journal.AtlasEntry{Key: "moon", Name: "Crescent Waxing",
		Kind: "moonstone", Category: journal.AtlasMoonstones, Charted: true, Phase: 1})
	dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	for _, entry := range dossier.Entries {
		view.SelectedKey = entry.Key
		dialog.Refresh(dossier)
		for category := journal.AtlasAllPlaces; category < journal.AtlasPlaceFilterCount; category++ {
			view.Filters.Places = category
			dialog.Refresh(dossier)
			dialog.Draw(screen)
			for _, output := range []*text.Output{dialog.header, dialog.zoom, dialog.footer, dialog.details, dialog.panelHeader, dialog.position, dialog.tabText} {
				for _, r := range output.GetOutputStr(false) {
					if r == '\n' {
						continue
					}
					glyph, glyphErr := font.GlyphIndex(nil, r)
					require.NoError(t, glyphErr)
					require.NotZero(t, glyph, "unsupported atlas character %q in %q", r, output.GetOutputStr(false))
				}
			}
		}
	}
	truncated := FitDialogLine("A very long location name", dialog.font, 70)
	require.True(t, strings.HasSuffix(truncated, "..."))
	width, _ := dialog.font.MeasureString(truncated)
	require.LessOrEqual(t, width, float64(70))
}

func TestAtlasPlainDetailsDoNotReserveAOnePageIndicator(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	dialog := NewAtlasDialog(atlasCleanupDossier(), nil, newWidgetTestKeyboard(0), &view, func() {})
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.drawDetails(screen)
	require.NotContains(t, dialog.details.GetOutputStr(false), "Source:")
	require.Equal(t, "Not yet visited.", dialog.details.GetOutputStr(false))
	require.NotContains(t, dialog.details.GetOutputStr(false), "Coordinates")
	require.NotContains(t, dialog.row.GetOutputStr(false), "PgUp")
	require.Zero(t, dialog.detailPage)
	require.True(t, dialog.positionRect.In(screen.Bounds()))
	require.True(t, dialog.statusRect.In(screen.Bounds()))
}
