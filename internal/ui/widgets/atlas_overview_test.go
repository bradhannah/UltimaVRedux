package widgets

import (
	"fmt"
	"image"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasOverviewRowsNavigationRefreshAndReopen(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	dossier := atlasCleanupDossier()
	dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
	require.Equal(t, len(dialog.entries), dialog.presentationRowCount())
	require.Equal(t, 3, dialog.mappedCount())
	for filter := journal.AtlasTownes; filter < journal.AtlasPlaceFilterCount; filter++ {
		view.Filters.Places = filter
		view.Overview = false
		dialog.Refresh(dossier)
		require.Equal(t, len(dialog.entries)+1, dialog.presentationRowCount())
		for range len(dialog.entries) + 1 {
			dialog.navigate(ebiten.KeyDown)
		}
		require.True(t, view.Overview)
		require.Equal(t, len(dialog.entries), dialog.selection)
		before := view
		dialog.handleKey(ebiten.KeyEnter)
		require.Equal(t, before, view)
		dialog.Refresh(dossier)
		require.True(t, view.Overview)
		reopened := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
		require.True(t, view.Overview)
		require.Equal(t, len(reopened.entries), reopened.selection)
		if len(dialog.entries) > 0 {
			dialog.navigate(ebiten.KeyUp)
			require.False(t, view.Overview)
		}
	}
	view.Filters.Places = journal.AtlasAllPlaces
	dialog.Refresh(dossier)
	require.False(t, view.Overview)
	require.Equal(t, len(dialog.entries), dialog.presentationRowCount())
}

func TestAtlasOverviewEveryAuthorizedVisibleNameWithStableCrowding(t *testing.T) {
	dialog := atlasRenderingDialog()
	dialog.view.Overview = true
	dialog.view.Filters.Places = journal.AtlasTownes
	for index := range 24 {
		dialog.entries = append(dialog.entries, journal.AtlasEntry{Key: fmt.Sprintf("place-%d", index), Name: fmt.Sprintf("Unnamed place %d", index),
			Category: journal.AtlasSettlements, Kind: "towne", Survey: atlasMeasuredSite(125+index%3, 125+index/3)})
	}
	dialog.entries = append(dialog.entries, journal.AtlasEntry{Key: "private", Name: "Unmeasured", Category: journal.AtlasSettlements, Kind: "towne"})
	callouts := dialog.mapCallouts()
	require.Len(t, callouts, 24)
	require.Equal(t, callouts, dialog.mapCallouts(), "bounded placement is deterministic")
	for index, callout := range callouts {
		center, ok := dialog.atlasMarkerCenter(dialog.entries[index])
		require.True(t, ok)
		require.Equal(t, center, callout.marker)
		require.True(t, callout.plate.In(dialog.mapRect))
		require.Equal(t, dialog.entries[index].Name, callout.name)
	}
	dialog.handleKey(ebiten.KeyL)
	require.Empty(t, dialog.mapCallouts())
	dialog.view.Overview = false
	dialog.view.SelectedKey = dialog.entries[0].Key
	require.Empty(t, dialog.mapCallouts(), "selection must not override hidden preference")
	dialog.handleKey(ebiten.KeyL)
	require.Len(t, dialog.mapCallouts(), 1)
	dialog.view.Overview = true
	dialog.view.Zoom = 4
	dialog.view.CenterX = 250
	dialog.view.CenterY = 250
	require.Empty(t, dialog.mapCallouts())
	require.Equal(t, 24, dialog.mappedCount())
}

func TestAtlasCoordinateBadgeColumnAndOverviewResponsiveLayout(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		setQuestJournalTestResolution(t, size.X, size.Y)
		view := NewAtlasViewState()
		view.Filters.Places = journal.AtlasTownes
		dialog := NewAtlasDialog(atlasCleanupDossier(), nil, newWidgetTestKeyboard(0), &view, func() {})
		rect := image.Rect(dialog.listRect.Min.X, dialog.listRect.Min.Y, dialog.listRect.Max.X, dialog.listRect.Min.Y+dialog.directoryRowHeight())
		screen := ebiten.NewImageWithOptions(rect, nil)
		for _, selected := range []bool{false, true} {
			screen.Clear()
			dialog.drawPlaceRow(screen, rect, dialog.entries[0], selected)
			cell := dialog.atlasSymbolCell()
			badgeRect := image.Rect(rect.Max.X-cell-8, rect.Min.Y, rect.Max.X, rect.Max.Y)
			with := atlasMarkerPixels(atlasSubImage(screen, badgeRect))
			screen.Clear()
			entry := dialog.entries[0]
			entry.Survey = journal.SurveyedLocation{}
			dialog.drawPlaceRow(screen, rect, entry, selected)
			without := atlasMarkerPixels(atlasSubImage(screen, badgeRect))
			require.NotEqual(t, with.Pix, without.Pix, "badge must be visible at a fixed reserved column")
		}
		before := dialog.mapRect
		dialog.view.Overview = true
		dialog.refilter()
		dialog.preparePlacesHeading()
		dialog.buildFooter()
		require.Equal(t, before, dialog.mapRect)
		require.Equal(t, 2, dialog.footer.GetPanelContentLineCount())
		require.NotContains(t, dialog.footer.GetOutputStr(false), "Enter Show")
		require.Contains(t, dialog.panelHeader.GetOutputStr(false), "3 total, 2 mapped")
		require.LessOrEqual(t, dialog.placesHeaderRect.Max.Y, dialog.listRect.Min.Y)
		require.LessOrEqual(t, dialog.detailRect.Max.Y, dialog.positionRect.Min.Y)
		screen.Deallocate()
	}
}

func TestAtlasSuppliedDecorationCannotBecomeNamedSelection(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, zoom := range []int{1, 2, 4} {
		view := NewAtlasViewState()
		view.Zoom = zoom
		view.CenterX = 255
		view.CenterY = 0
		entry := journal.AtlasEntry{Key: "chart-18", Name: "Known shrine", Kind: "shrine", Category: journal.AtlasShrines, NameKnown: true, Discovered: true}
		symbol := journal.AtlasSymbol{Key: entry.Key, Kind: entry.Kind, Category: entry.Category, Phase: -1, Position: journal.SiteLocation{X: 255, Y: 0}}
		dialog := NewAtlasDialog(journal.AtlasDossier{Entries: []journal.AtlasEntry{entry}, Symbols: []journal.AtlasSymbol{symbol}}, nil, newWidgetTestKeyboard(0), &view, func() {})
		screen := ebiten.NewImage(1280, 720)
		dialog.drawSuppliedSymbols(screen)
		baseline := atlasMarkerPixels(screen)
		require.NotEqual(t, image.NewRGBA(screen.Bounds()).Pix, baseline.Pix, "approved artwork is visible without a measurement")
		dialog.drawPoint(screen, entry, true)
		require.Equal(t, baseline.Pix, atlasMarkerPixels(screen).Pix, "same neutral key is not selection authority")
		before := view
		dialog.handleKey(ebiten.KeyEnter)
		require.Equal(t, before, view)
		require.Equal(t, "Take a sextant reading there to place it on the Map.", dialog.status)
		require.Empty(t, dialog.mapCallouts())
		require.Zero(t, dialog.mappedCount())
		view.Filters.Places = journal.AtlasShrinePlaces
		dialog.refilter()
		dialog.navigate(ebiten.KeyDown)
		require.True(t, view.Overview)
		require.Empty(t, dialog.mapCallouts())
		view.Filters.Places = journal.AtlasCastlePlaces
		dialog.refilter()
		screen.Clear()
		dialog.drawSuppliedSymbols(screen)
		require.Equal(t, image.NewRGBA(screen.Bounds()).Pix, atlasMarkerPixels(screen).Pix)
		view.Filters.Places = journal.AtlasShrinePlaces
		entry.Survey = atlasMeasuredSite(255, 0)
		dialog.Refresh(journal.AtlasDossier{Entries: []journal.AtlasEntry{entry}, Symbols: []journal.AtlasSymbol{symbol}})
		screen.Clear()
		dialog.drawSuppliedSymbols(screen)
		require.Equal(t, image.NewRGBA(screen.Bounds()).Pix, atlasMarkerPixels(screen).Pix, "coincident earned artwork replaces the baseline")
		require.Len(t, dialog.mapCallouts(), 1)
		screen.Deallocate()
	}
}
