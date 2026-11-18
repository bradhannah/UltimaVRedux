package widgets

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasCoordinateDetailsRequirePersonalDiscoveryAndMeasurement(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, kind := range []string{"towne", "stronghold", "castle", "lighthouse", "guardians", atlasMoonstoneKind} {
		dossier := journal.AtlasDossier{Entries: []journal.AtlasEntry{{
			Key: "point", Name: "Displayed name", Kind: kind, NameKnown: true,
			Charted: true,
		}}}
		view := NewAtlasViewState()
		dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
		text := atlasFieldsText(dialog.detailFields())
		require.Equal(t, ": Not yet visited.\n", text)
		require.NotContains(t, text, "Displayed name")
		require.NotContains(t, text, atlasCoordinates(journal.SiteLocation{X: 64, Y: 80}))
		dossier.Entries[0].Survey = journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{X: 111, Y: 222}}
		dialog.Refresh(dossier)
		require.NotContains(t, atlasFieldsText(dialog.detailFields()), "Coordinates", "supplied identity is not a visit")
		dossier.Entries[0].Discovered = true
		dossier.Entries[0].Survey = journal.SurveyedLocation{}
		dialog.Refresh(dossier)
		require.Contains(t, atlasFieldsText(dialog.detailFields()), "Coordinates: Not measured")
		dossier.Entries[0].Survey = journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{X: 111, Y: 222}}
		dialog.Refresh(dossier)
		text = atlasFieldsText(dialog.detailFields())
		require.Contains(t, text, atlasCoordinates(dossier.Entries[0].Survey.Location))
		require.NotContains(t, text, atlasCoordinates(journal.SiteLocation{X: 64, Y: 80}))
		require.NotContains(t, text, "supplied map")
	}
}

func atlasFieldsText(fields [][2]string) string {
	var result strings.Builder
	for _, field := range fields {
		result.WriteString(field[0] + ": " + field[1] + "\n")
	}

	return result.String()
}

func TestAtlasPanModeUsesPlainArrowsAndStaysVisibleWithTwoHelpRows(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	keys := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
	view := NewAtlasViewState()
	dialog := NewAtlasDialog(atlasCleanupDossier(), nil, keyboard, &view, func() {})
	press := func(key ebiten.Key) {
		clear(keys)
		keyboard.ClearFrameConsumption()
		dialog.Update()
		keys[key] = true
		keyboard.ClearFrameConsumption()
		dialog.Update()
		clear(keys)
		keyboard.ClearFrameConsumption()
		dialog.Update()
	}
	press(ebiten.KeyP)
	require.True(t, dialog.panning)
	press(ebiten.KeyDown)
	require.Equal(t, "Zoom in to pan.", dialog.status)
	press(ebiten.KeyEqual)
	before := view.CenterY
	selected := view.SelectedKey
	press(ebiten.KeyDown)
	require.NotEqual(t, before, view.CenterY)
	require.Equal(t, selected, view.SelectedKey)
	press(ebiten.KeyTab)
	press(ebiten.KeyTab)
	require.True(t, dialog.panning, "changing categories keeps the current pan mode explicit")
	dialog.buildFooter()
	require.Equal(t, 2, dialog.footer.GetPanelContentLineCount())
	require.Contains(t, dialog.footer.GetOutputStr(false), "[P] Places")
	require.NotContains(t, dialog.footer.GetOutputStr(false), "Ctrl")
	dialog.prepareHeading()
	require.Contains(t, dialog.zoom.GetOutputStr(false), "PAN MODE")
	press(ebiten.KeyP)
	require.False(t, dialog.panning)
	press(ebiten.KeyDown)
	require.Equal(t, "britain", view.SelectedKey)
	panView := view
	press(ebiten.KeyP)
	reopened := NewAtlasDialog(atlasCleanupDossier(), nil, newWidgetTestKeyboard(0), &view, func() {})
	require.False(t, reopened.panning, "opening starts in normal place-selection mode")
	require.Equal(t, panView, view, "pan mode is not saved preference or gameplay state")
}

func TestAtlasTabAndRowSelectionKeepSemanticDiscoveryColours(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	dossier := atlasCleanupDossier()
	dossier.Entries[1].Discovered = true
	dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
	screen := ebiten.NewImage(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.drawCategoryTabs(screen)
	selected := atlasMarkerPixels(atlasSubImage(screen, dialog.tabs[journal.AtlasAllPlaces]))
	require.Positive(t, atlasExactInkCount(selected, DialogActiveTabBackground), "the open tab sits on the shared plate")
	require.Positive(t, atlasExactInkCount(selected, DialogActiveTabText))
	rect := image.Rect(20, 20, 620, 20+dialog.directoryRowHeight())
	for _, entry := range dossier.Entries[:2] {
		screen.Clear()
		dialog.drawPlaceRow(screen, rect, entry, false)
		pixels := atlasMarkerPixels(atlasSubImage(screen, rect))
		want, absent := DialogAtlasUndiscoveredColor, DialogAtlasDiscoveredColor
		if entry.Discovered {
			want, absent = DialogAtlasDiscoveredColor, DialogAtlasUndiscoveredColor
		}
		require.Positive(t, atlasExactInkCount(pixels, want))
		require.Zero(t, atlasExactInkCount(pixels, absent))
		screen.Clear()
		fillAtlasRect(screen, rect, DialogSelectedBackground)
		dialog.drawPlaceRow(screen, rect, entry, true)
		selectedPixels := atlasMarkerPixels(atlasSubImage(screen, rect))
		symbolColour := DialogSelectedLabelColor
		if entry.Discovered {
			symbolColour = DialogSelectedCheckedMarkColor
		}
		require.Positive(t, atlasExactInkCount(selectedPixels, symbolColour), "discovery remains readable against white")
		require.Positive(t, atlasExactInkCount(selectedPixels, DialogSelectedText), "inverted names are black")
		require.Zero(t, atlasExactInkCount(selectedPixels, DialogLabelColor), "no yellow row border")
		require.Equal(t, color.RGBAModel.Convert(DialogSelectedBackground), selectedPixels.At(rect.Max.X-1, rect.Min.Y))
	}
	for selection := range 2 {
		dialog.selection = selection
		screen.Clear()
		dialog.drawDirectory(screen)
		rowHeight := dialog.directoryRowHeight()
		selectedRect := image.Rect(dialog.listRect.Min.X, dialog.listRect.Min.Y+selection*rowHeight,
			dialog.listRect.Max.X, dialog.listRect.Min.Y+(selection+1)*rowHeight)
		selectedPixels := atlasMarkerPixels(atlasSubImage(screen, selectedRect))
		require.Equal(t, color.RGBAModel.Convert(DialogSelectedBackground),
			selectedPixels.At(selectedRect.Min.X, selectedRect.Min.Y), "actual directory uses an inversion bar")
		require.Equal(t, color.RGBAModel.Convert(DialogSelectedBackground),
			selectedPixels.At(selectedRect.Max.X-1, selectedRect.Max.Y-1))
		require.Zero(t, atlasExactInkCount(selectedPixels, DialogLabelColor), "actual selection has no yellow border")
	}
	require.Equal(t, ucolor.EGALightGray, DialogAtlasUndiscoveredColor)
	require.Equal(t, ucolor.EGALightGreen, DialogAtlasDiscoveredColor)
}

func atlasExactInkCount(pixels *image.RGBA, want color.Color) int {
	colour := color.RGBAModel.Convert(want).(color.RGBA)
	count := 0
	for y := pixels.Rect.Min.Y; y < pixels.Rect.Max.Y; y++ {
		for x := pixels.Rect.Min.X; x < pixels.Rect.Max.X; x++ {
			if pixels.RGBAAt(x, y) == colour {
				count++
			}
		}
	}

	return count
}
