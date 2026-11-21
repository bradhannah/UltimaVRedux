package widgets

import (
	"image"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasUnmeasuredSelectionNeverDrawsOrCentersAnySite(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, kind := range []string{"towne", "village", "hut", "keep", "stronghold", "castle", "shrine", "ruins", "lighthouse", "dungeon", "moonstone", "guardians"} {
		for _, zoom := range []int{1, 2, 4} {
			view := NewAtlasViewState()
			view.Zoom, view.CenterX, view.CenterY = zoom, 255, 0
			entry := journal.AtlasEntry{Key: "selected", Name: "Known place", Kind: kind, Discovered: true, Charted: true}
			dialog := NewAtlasDialog(journal.AtlasDossier{Entries: []journal.AtlasEntry{entry}}, nil, newWidgetTestKeyboard(0), &view, func() {})
			before := view
			dialog.handleKey(ebiten.KeyEnter)
			require.Equal(t, before, view, kind)
			require.Contains(t, dialog.status, "Take a sextant reading there")
			require.Contains(t, atlasFieldsText(dialog.detailFields()), "Coordinates: Not measured")
			_, visible := dialog.atlasMarkerCenter(entry)
			require.False(t, visible)
			require.True(t, dialog.selectedMapCallout().plate.Empty())
			// All kinds/zooms retain visibility and camera checks. Raster absence
			// samples settlement, item and guardian symbols at both zoom extremes.
			if zoom == 2 || (kind != "towne" && kind != "moonstone" && kind != "guardians") {
				continue
			}
			screen := ebiten.NewImageWithOptions(dialog.mapRect, nil)
			dialog.drawPoint(screen, entry, true)
			require.Equal(t, image.NewRGBA(screen.Bounds()).Pix, atlasMarkerPixels(screen).Pix, "no point or selection brackets")
			screen.Deallocate()
		}
	}
}
