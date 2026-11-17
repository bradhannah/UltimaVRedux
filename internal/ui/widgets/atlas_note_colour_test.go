package widgets

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAtlasNoteMapInkPreservesSelectionAndDirectoryColours(t *testing.T) {
	dialog := atlasRenderingDialog()
	entry := journal.AtlasEntry{Kind: "note", Category: journal.AtlasNotes,
		Note: journal.AtlasNoteRecord{ID: 1, Position: atlasMeasuredSite(128, 128).Location}}
	screen := ebiten.NewImage(528, 528)
	t.Cleanup(screen.Deallocate)
	for _, selected := range []bool{false, true} {
		screen.Clear()
		dialog.drawPoint(screen, entry, selected)
		pixels := atlasMarkerPixels(screen)
		require.Positive(t, atlasExactInkCount(pixels, ucolor.EGALightMagenta), "a note is a flag in its own ink")
		require.Positive(t, atlasExactInkCount(pixels, ucolor.EGABlack))
		require.Zero(t, atlasExactInkCount(pixels, ucolor.EGAWhite))
		if selected {
			require.Positive(t, atlasExactInkCount(pixels, DialogLabelColor))
		}
		require.Equal(t, DialogValueColor, atlasPlaceColor(entry), "map ink must not recolour the Places label")
		require.False(t, entry.Discovered)
	}
	// An independently positioned vessel retains its existing white map artwork.
	entry = journal.AtlasEntry{Kind: "frigate", Category: journal.AtlasVessels,
		Vessel: journal.AtlasVesselPosition{ID: 1, Position: atlasMeasuredSite(128, 128).Location}}
	require.Equal(t, DialogValueColor, atlasPlaceColor(entry))
}
