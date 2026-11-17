package widgets

import (
	"image"
	"image/color"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
)

const atlasStampSize = 5

// The art grid is independent of the standard message-box border unit. It fits
// inside the compact body budget; the outline is about one artwork cell wide.
//
//nolint:funlen,goconst // Keep the complete pixel-art table readable as silhouettes, including repeated rows.
func atlasMarkerStamp(kind string) [atlasStampSize]string {
	switch kind {
	case "frigate":
		return [atlasStampSize]string{"  #  ", "  ## ", "  #  ", "#####", " ### "}
	case "skiff":
		return [atlasStampSize]string{"     ", "   # ", "  #  ", "#####", " ### "}
	case "note":
		// A pennant: the player's own flag, unlike any place family.
		return [atlasStampSize]string{"###  ", "#####", "###  ", "#    ", "#    "}
	case "towne", "village":
		return [atlasStampSize]string{
			"  #  ",
			" # # ",
			"#####",
			"#   #",
			"## ##",
		}
	case "hut":
		return [atlasStampSize]string{
			"     ",
			"  #  ",
			" ### ",
			" # # ",
			"     ",
		}
	case "keep":
		return [atlasStampSize]string{
			"# # #",
			"#####",
			" # # ",
			" # # ",
			" ### ",
		}
	case "stronghold":
		return [atlasStampSize]string{
			"#   #",
			"#   #",
			"#####",
			"#   #",
			"#   #",
		}
	case "castle":
		return [atlasStampSize]string{
			"# # #",
			"# # #",
			"#####",
			"#   #",
			"## ##",
		}
	case "shrine", "ruins", "guardians":
		return [atlasStampSize]string{
			"  #  ",
			" # # ",
			"#   #",
			" # # ",
			"  #  ",
		}
	case "lighthouse":
		return [atlasStampSize]string{
			"  #  ",
			"#####",
			"  #  ",
			"  #  ",
			" ### ",
		}
	case "dungeon":
		return [atlasStampSize]string{
			" ### ",
			"#   #",
			"#   #",
			"#   #",
			"#   #",
		}
	default:
		return [atlasStampSize]string{}
	}
}

func atlasCoordinateStamp() [atlasStampSize]string {
	return [atlasStampSize]string{" ### ", "#   #", "# # #", "#   #", " ### "}
}

// Compact atlas-only phases keep the eight identities without an oversized
// bitmap-font cell. Waxing illuminates the right; waning mirrors it on the left.
//
//nolint:goconst // The eight ordered phase silhouettes are literal pixel art.
func atlasMoonStamp(phase int) [atlasStampSize]string {
	phases := [8][atlasStampSize]string{
		{" ### ", "#   #", "#   #", "#   #", " ### "},
		{"  #  ", "   # ", "    #", "   # ", "  #  "},
		{"  ## ", "  ###", "  ###", "  ###", "  ## "},
		{" ### ", " ####", " ####", " ####", " ### "},
		{" ### ", "#####", "#####", "#####", " ### "},
		{" ### ", "#### ", "#### ", "#### ", " ### "},
		{" ##  ", "###  ", "###  ", "###  ", " ##  "},
		{"  #  ", " #   ", "#    ", " #   ", "  #  "},
	}
	if phase < 0 || phase >= len(phases) {
		return [atlasStampSize]string{}
	}

	return phases[phase]
}

// Artwork stays two border units across. Tiny unsupported resolutions retain at least one pixel
// per art cell so distinct silhouettes and moon phases cannot disappear.
//
//nolint:mnd // Two border units are the deliberately separate artwork size budget.
func atlasMarkerBodySize(unit int) int {
	return max(atlasStampSize, 2*unit)
}

// Round one artwork cell to the nearest screen pixel, rather than using the
// much larger message-box border unit for the icon's contrast edge.
func atlasMarkerOutlineWidth(unit int) int {
	return max(1, (atlasMarkerBodySize(unit)+atlasStampSize/2)/atlasStampSize)
}

// Two passes keep the dark surround behind every cell, including stepped edges.
// The outline follows the artwork's fine grid; body sizing remains independent
// so tuning contrast never enlarges or shrinks the coloured silhouette.
//
//nolint:mnd // Integer half-size centres the compact body on the projected point.
func drawAtlasStamp(screen *ebiten.Image, stamp [atlasStampSize]string, center image.Point,
	unit int, colour color.Color, outlined bool) {
	bodySize := atlasMarkerBodySize(unit)
	origin := center.Sub(image.Pt(bodySize/2, bodySize/2))
	if outlined {
		paintAtlasStamp(screen, stamp, origin, bodySize, atlasMarkerOutlineWidth(unit), ucolor.EGABlack)
	}
	paintAtlasStamp(screen, stamp, origin, bodySize, 0, colour)
}

func paintAtlasStamp(screen *ebiten.Image, stamp [atlasStampSize]string, origin image.Point,
	bodySize, surround int, colour color.Color) {
	for row, cells := range stamp {
		for column := range len(cells) {
			if cells[column] != '#' {
				continue
			}
			cell := atlasArtCell(column, row, bodySize).Add(origin)
			fillAtlasRect(screen, cell.Inset(-surround), colour)
		}
	}
}

// Round both cell edges onto the body grid, without smoothing or fractional
// rectangles. Paired edge rounding keeps symmetric art symmetric at odd scales.
func atlasArtCell(column, row, bodySize int) image.Rectangle {
	return image.Rect((column*bodySize+atlasStampSize/2)/atlasStampSize,
		(row*bodySize+atlasStampSize/2)/atlasStampSize,
		((column+1)*bodySize+atlasStampSize/2)/atlasStampSize,
		((row+1)*bodySize+atlasStampSize/2)/atlasStampSize)
}
