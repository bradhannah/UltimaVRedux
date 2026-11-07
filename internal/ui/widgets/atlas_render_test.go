package widgets

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasRenderingClassicFrameAndIntegerElbowAtPresets(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			dialog := atlasRenderingDialog()
			dialog.width, dialog.height = size.X, size.Y
			unit := ClassicUIPixelUnit(size.X, size.Y)
			for _, edge := range []image.Point{image.Pt(160, 190), image.Pt(190, 160)} {
				callout := atlasMapCallout{marker: image.Pt(100, 100), edge: edge, name: "Castle",
					plate: image.Rect(160, 160, 340, 260)}
				screen := ebiten.NewImage(528, 528)
				dialog.drawMapCallout(screen, callout)
				pixels := atlasMarkerPixels(screen)
				screen.Deallocate()
				yellow := color.RGBAModel.Convert(DialogLabelColor).(color.RGBA)
				// Corner steps use the exact classic silhouette, not a smooth arc.
				require.Zero(t, pixels.RGBAAt(160, 160).A)
				require.Equal(t, yellow, pixels.RGBAAt(160+3*unit, 160))
				require.Equal(t, yellow, pixels.RGBAAt(160+unit, 160+unit))
				require.Equal(t, yellow, pixels.RGBAAt(edge.X, edge.Y))
				require.Equal(t, yellow, pixels.RGBAAt(100, 100), "leader originates at the exact marker centre")
				for y := 90; y < 150; y++ {
					for x := 90; x < 150; x++ {
						pixel := pixels.RGBAAt(x, y)
						require.True(t, pixel.A == 0 || pixel == yellow || pixel == ucolor.EGABlack.ToRGBA(),
							"integer segments must not produce antialiased pixels")
						if pixel == yellow {
							if edge.X == 160 {
								require.True(t, x >= 100-unit/2 && x < 100+(unit+1)/2)
							} else {
								require.True(t, y >= 100-unit/2 && y < 100+(unit+1)/2)
							}
						}
					}
				}
			}
		})
	}
}

func atlasRenderingDialog() *AtlasDialog {
	view := NewAtlasViewState()
	font := text.NewUltimaFont(16)

	return &AtlasDialog{view: &view, font: font, label: text.NewOutput(font, 20, 4, 128),
		mapRect: image.Rect(8, 8, 520, 520), rowHeight: 20, width: 1280, height: 720}
}

func TestAtlasRenderingCalloutGeometry(t *testing.T) {
	dialog := atlasRenderingDialog()
	for _, position := range []journal.SiteLocation{
		{X: 128, Y: 128}, {X: 0, Y: 0}, {X: 255, Y: 255}, {X: 0, Y: 255}, {X: 255, Y: 0},
	} {
		entry := journal.AtlasEntry{Key: "selected", Name: "Unnamed castle", Kind: "castle", Charted: true, Survey: atlasMeasuredSite(position.X, position.Y)}
		dialog.entries = []journal.AtlasEntry{entry}
		dialog.view.SelectedKey = entry.Key
		callout := dialog.selectedMapCallout()
		require.False(t, callout.plate.Empty(), "position %v", position)
		x, y, visible := dialog.view.project(float64(position.X)+.5, float64(position.Y)+.5, dialog.mapRect)
		require.True(t, visible)
		require.Equal(t, image.Pt(int(x), int(y)), callout.marker, "leader starts at the actual symbol centre")
		require.True(t, callout.plate.In(dialog.mapRect))
		require.False(t, callout.plate.Overlaps(dialog.selectionBounds(callout.marker).Inset(-2)))
		require.True(t, callout.edge.X >= callout.plate.Min.X && callout.edge.X <= callout.plate.Max.X)
		require.True(t, callout.edge.Y >= callout.plate.Min.Y && callout.edge.Y <= callout.plate.Max.Y)
		require.True(t, callout.edge.X == callout.plate.Min.X || callout.edge.X == callout.plate.Max.X-1 ||
			callout.edge.Y == callout.plate.Min.Y || callout.edge.Y == callout.plate.Max.Y-1, "leader terminates on the plate boundary")
		require.Equal(t, entry.Name, callout.name, "an unlearned name stays generic")
	}
}

func TestAtlasRenderingCalloutAvoidsOtherMarkers(t *testing.T) {
	marker, size := image.Pt(200, 200), image.Pt(100, 24)
	pane := image.Rect(0, 0, 400, 400)
	obstacle := image.Rect(235, 192, 252, 209)
	selected := atlasRenderingDialog().selectionBounds(marker)
	plate := atlasCalloutPlate(marker, size, pane, []image.Rectangle{obstacle}, selected, 4)
	require.False(t, plate.Empty())
	require.False(t, plate.Overlaps(obstacle), "left side remains available when the right side is occupied")
	require.Less(t, plate.Max.X, marker.X)
	crowded := atlasCalloutPlate(marker, size, pane, []image.Rectangle{pane}, selected, 4)
	require.False(t, crowded.Empty(), "crowded maps retain the selected nameplate")
	require.False(t, crowded.Overlaps(selected.Inset(-2)), "even the fallback protects selection")
}

func TestAtlasRenderingOnlySelectedCalloutAndPublicName(t *testing.T) {
	dialog := atlasRenderingDialog()
	dialog.entries = []journal.AtlasEntry{
		{Key: "one", Name: "Towne 1", Kind: "towne", Charted: true, Survey: atlasMeasuredSite(80, 80)},
		{Key: "two", Name: "Unnamed castle", Kind: "castle", Charted: true, Survey: atlasMeasuredSite(128, 128)},
		{Key: "three", Name: "Keep 1", Kind: "keep", Charted: true, Survey: atlasMeasuredSite(180, 180)},
	}
	dialog.view.SelectedKey = "two"
	screen := ebiten.NewImage(528, 528)
	t.Cleanup(screen.Deallocate)
	dialog.drawGeography(screen)
	require.Equal(t, "Unnamed castle", dialog.selectedMapCallout().name)
	require.Contains(t, dialog.label.GetOutputStr(false), "Unnamed castle")
	require.NotContains(t, dialog.label.GetOutputStr(false), "Towne 1")
	before := atlasMarkerPixels(screen)
	dialog.entries[0].Name, dialog.entries[2].Name = "Other settlement's much longer name", "Another keep"
	screen.Clear()
	dialog.drawGeography(screen)
	require.Equal(t, before.Pix, atlasMarkerPixels(screen).Pix, "no unselected nameplate is rendered among multiple entries")
	dialog.entries[1].Name, dialog.entries[1].NameKnown = "Learned name", true
	require.Equal(t, "Learned name", dialog.selectedMapCallout().name)
	dialog.view.Labels = AtlasLabelsNone
	require.True(t, dialog.selectedMapCallout().plate.Empty())
	dialog.view.Labels = AtlasLabelsSelected
	dialog.view.Zoom, dialog.view.CenterX, dialog.view.CenterY = 4, 0, 0
	require.True(t, dialog.selectedMapCallout().plate.Empty(), "offscreen selection must not invent an edge marker")
	dialog.entries[1].Survey = journal.SurveyedLocation{}
	require.True(t, dialog.selectedMapCallout().plate.Empty(), "unmeasured selection must not invent coordinates")
	dialog.entries[1].Survey = atlasMeasuredSite(255, 255)
	require.False(t, dialog.selectedMapCallout().plate.Empty(), "wrapped visible selection still gets a callout")
}

func TestAtlasRenderingSelectedSymbolAboveLeaderAndPaneClipped(t *testing.T) {
	for _, status := range []struct {
		name       string
		discovered bool
	}{{"charted", false}, {"discovered", true}} {
		t.Run(status.name, func(t *testing.T) {
			dialog := atlasRenderingDialog()
			entry := journal.AtlasEntry{Key: "selected", Name: "Castle", Kind: "castle", Charted: true,
				Discovered: status.discovered, Survey: atlasMeasuredSite(128, 128)}
			dialog.entries, dialog.view.SelectedKey = []journal.AtlasEntry{entry}, entry.Key
			screen, symbol := ebiten.NewImage(528, 528), ebiten.NewImage(528, 528)
			t.Cleanup(screen.Deallocate)
			t.Cleanup(symbol.Deallocate)
			screen.Fill(ucolor.EGABlue)
			dialog.drawGeography(screen)
			dialog.drawPoint(symbol, entry, true)
			composed, isolated := atlasMarkerPixels(screen), atlasMarkerPixels(symbol)
			ink := color.RGBAModel.Convert(atlasPlaceColor(entry)).(color.RGBA)
			highlight := color.RGBAModel.Convert(DialogLabelColor).(color.RGBA)
			for y := range 528 {
				for x := range 528 {
					if isolated.RGBAAt(x, y) == ink || isolated.RGBAAt(x, y) == highlight {
						require.Equal(t, isolated.RGBAAt(x, y), composed.RGBAAt(x, y),
							"selected symbol and yellow ring must remain above the leader at %d,%d", x, y)
					}
					if !image.Pt(x, y).In(dialog.mapRect) {
						require.Equal(t, ucolor.EGABlue.ToRGBA(), composed.RGBAAt(x, y), "map rendering must stay clipped")
					}
				}
			}
			callout := dialog.selectedMapCallout()
			require.Equal(t, ink, composed.RGBAAt(callout.marker.X, callout.marker.Y))
			leader := ebiten.NewImage(528, 528)
			t.Cleanup(leader.Deallocate)
			leader.Fill(ucolor.EGABlue)
			dialog.drawMapCallout(leader, callout)
			calloutOnly := atlasMarkerPixels(leader)
			_, yellowText := dialog.label.FirstPanelLineWithColor(0, 1, DialogLabelColor)
			require.True(t, yellowText, "callout text shares the yellow ring/frame highlight")
			_, whiteText := dialog.label.FirstPanelLineWithColor(0, 1, DialogSelectedBackground)
			require.False(t, whiteText)
			// The gap between marker and plate contains both the foreground and black contrast stroke.
			gap := image.Rect(dialog.selectionBounds(callout.marker).Max.X+2, callout.marker.Y-4,
				callout.plate.Min.X, callout.marker.Y+4)
			var contrast, foreground bool
			for y := gap.Min.Y; y < gap.Max.Y; y++ {
				for x := gap.Min.X; x < gap.Max.X; x++ {
					pixel := composed.RGBAAt(x, y)
					require.Equal(t, calloutOnly.RGBAAt(x, y), pixel, "leader remains at its exact projected geometry")
					contrast = contrast || pixel == ucolor.EGABlack.ToRGBA()
					foreground = foreground || pixel.R > pixel.B && pixel.R == pixel.G
				}
			}
			require.True(t, contrast)
			require.True(t, foreground)
		})
	}
}

func TestAtlasRenderingRegionsIndependentUnboxedAndYieldToCallout(t *testing.T) {
	dialog := atlasRenderingDialog()
	// Isolate the two-line Valorian Isles: the anchor is an area, with no site symbol.
	dialog.view.Zoom, dialog.view.CenterX, dialog.view.CenterY = 4, 37, 220
	screen := ebiten.NewImage(528, 528)
	t.Cleanup(screen.Deallocate)
	dialog.drawRegionLabels(screen, image.Rectangle{})
	baseline := atlasMarkerPixels(screen)
	require.Equal(t, "Valorian \nIsles", dialog.label.GetOutputStr(false))
	anchorX, anchorY, visible := dialog.view.project(37, 220, dialog.mapRect)
	require.True(t, visible)
	lines, textBounds := dialog.regionLabelLayout("Valorian Isles", image.Pt(int(anchorX), int(anchorY)), image.Rectangle{})
	require.Equal(t, []string{"Valorian", "Isles"}, lines)
	painted := 0
	for y := range 528 {
		for x := range 528 {
			pixel := baseline.RGBAAt(x, y)
			if pixel.A > 0 {
				painted++
				require.True(t, image.Pt(x, y).In(textBounds), "region has no marker or leader outside its complete text block")
				require.Equal(t, pixel.R, pixel.G)
				require.Equal(t, pixel.G, pixel.B)
				require.NotEqual(t, ucolor.EGABlack.ToRGBA(), pixel, "region text must have no black background, leader or outline")
			}
		}
	}
	require.Positive(t, painted)
	require.Less(t, painted, textBounds.Dx()*textBounds.Dy()/2, "glyphs must not become a filled nameplate")
	for _, mode := range []AtlasLabelMode{AtlasLabelsSelected, AtlasLabelsNone} {
		dialog.view.Labels = mode
		for places := journal.AtlasAllPlaces; places < journal.AtlasPlaceFilterCount; places++ {
			dialog.view.Filters.Places = places
			screen.Clear()
			dialog.drawRegionLabels(screen, image.Rectangle{})
			require.Equal(t, baseline.Pix, atlasMarkerPixels(screen).Pix, "region names are independent of L and every Places filter")
		}
	}
	dialog.entries = []journal.AtlasEntry{{Kind: "castle", Discovered: true}}
	screen.Clear()
	dialog.drawRegionLabels(screen, image.Rectangle{})
	require.Equal(t, baseline.Pix, atlasMarkerPixels(screen).Pix, "personal discovery never recolours area typography")
	screen.Clear()
	dialog.drawRegionLabels(screen, image.Rect(240, 240, 300, 290))
	blank := image.NewRGBA(screen.Bounds())
	require.Equal(t, blank.Pix, atlasMarkerPixels(screen).Pix, "overlapping region text is skipped, not moved or boxed")
}

func TestAtlasTorchFlickerMovesOnTheDialogsOwnFrames(t *testing.T) {
	dialog := atlasRenderingDialog()
	require.InDelta(t, 1, dialog.lightFlicker(), 1e-9, "daylight is steady")
	lighting := AtlasDaylight()
	lighting.Flicker = 0.06
	dialog.SetLighting(lighting)
	first := dialog.lightFlicker()
	dialog.lightFrames += 20
	require.NotEqual(t, first, dialog.lightFlicker(), "torchlight flickers")
	require.InDelta(t, 1, dialog.lightFlicker(), 0.06, "within the flicker amplitude")
}
