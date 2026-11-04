// Package atlasterrain composes a terrain-only chart from immutable loaded inputs.
// It has no gameplay, graphics-device, clock, or original binary-loader dependency.
package atlasterrain

import (
	"errors"
	"fmt"
	"image/color"
)

// Fixed dimensions and version form part of every cache identity.
const (
	WorldSize             = 256
	TileSize              = 16
	ImageSize             = WorldSize * TileSize
	PolicyVersion         = "britannia-cartographic-v1-4096"
	reviewedSubstitutions = 69
)

// ErrUnsupportedSource reports an unreviewed terrain or artwork input.
var ErrUnsupportedSource = errors.New("unsupported atlas source")

// Terrain is the existing loaded immutable reference, in [x][y] order.
type Terrain [WorldSize][WorldSize]uint16

type siteGuard struct{ x, y, source, replacement int }

//nolint:gochecknoglobals // Immutable reviewed substrate policy, not runtime state.
var siteGuards = [...]siteGuard{
	{15, 160, 18, 12}, {36, 229, 25, 12}, {45, 62, 16, 8}, {64, 240, 18, 8}, {73, 11, 25, 5}, {81, 207, 25, 12},
	{128, 92, 25, 12}, {148, 74, 18, 12}, {195, 244, 58, 12}, {195, 245, 61, 12}, {196, 244, 59, 12}, {196, 245, 57, 12},
	{197, 244, 60, 12}, {197, 245, 63, 12}, {201, 59, 16, 7}, {205, 45, 25, 7},
	{231, 216, 25, 12}, {233, 66, 25, 8}, {248, 8, 18, 12},
}

// Capture uses the existing reference's fixed-frame GetSpriteIndex, in [x][y]
// orientation. Reading the loaded reference cannot reveal player overlays.
//
//nolint:varnamelen // x/y are source image axes.
func Capture(tileAt func(x, y int) int) (Terrain, error) {
	var terrain Terrain
	for x := range WorldSize {
		for y := range WorldSize {
			id := tileAt(x, y)
			if !supportedTile(id) {
				return Terrain{}, fmt.Errorf("%w: tile %d at (%d,%d)", ErrUnsupportedSource, id, x, y)
			}
			//nolint:gosec // supportedTile bounds the input to 1..212.
			terrain[x][y] = uint16(id)
		}
	}

	return terrain, nil
}

//nolint:cyclop // This finite audited allowlist deliberately rejects unknown art families.
func supportedTile(id int) bool {
	// Natural land/water, retained roads/corners, streams/bridges, barriers,
	// waterfalls and the finite feature families observed in the reviewed map.
	return id >= 1 && id <= 25 || id == 27 || id == 28 || id >= 30 && id <= 38 || id == 47 ||
		id >= 48 && id <= 55 || id >= 57 && id <= 63 || id == 71 ||
		id >= 94 && id <= 111 || id == 143 || id == 164 || id == 212
}

//nolint:mnd,varnamelen // Numeric source tile IDs are the reviewed policy's identity.
func substrate(id uint16) uint16 {
	switch id {
	case 16, 18, 19, 20, 21, 25, 27, 57, 58, 59, 60, 61, 62, 63, 94, 95, 164:
		return 5
	case 22, 23, 24:
		return 12
	case 17, 28:
		return 7
	default:
		return id
	}
}

// Normalize enforces all nineteen reviewed source guards before publication.
// Substrates are authored cartographic choices, not reconstructed gameplay ground.
func Normalize(terrain Terrain) (Terrain, int, error) {
	for _, guard := range siteGuards {
		if int(terrain[guard.x][guard.y]) != guard.source {
			return Terrain{}, 0, fmt.Errorf("%w at (%d,%d): want %d, got %d",
				ErrUnsupportedSource, guard.x, guard.y, guard.source, terrain[guard.x][guard.y])
		}
	}
	normalized := terrain
	for x := range WorldSize {
		for y := range WorldSize {
			if !supportedTile(int(terrain[x][y])) {
				return Terrain{}, 0, fmt.Errorf("%w: tile %d at (%d,%d)", ErrUnsupportedSource, terrain[x][y], x, y)
			}
			normalized[x][y] = substrate(terrain[x][y])
		}
	}
	for _, guard := range siteGuards {
		//nolint:gosec // All immutable guard replacements are 5, 7, 8 or 12.
		normalized[guard.x][guard.y] = uint16(guard.replacement)
	}
	changed := 0
	for x := range WorldSize {
		for y := range WorldSize {
			if normalized[x][y] != terrain[x][y] {
				changed++
			}
		}
	}
	if changed != reviewedSubstitutions {
		return Terrain{}, changed, fmt.Errorf("%w: want %d feature cells, got %d",
			ErrUnsupportedSource, reviewedSubstitutions, changed)
	}

	return normalized, changed, nil
}

// cartographicColour preserves source pixel positions and the approved palette.
//
//nolint:mnd,gocyclo,cyclop,varnamelen,funlen // Exact reviewed RGB thresholds preserve prototype pixel classes.
func cartographicColour(tile uint16, p color.NRGBA) color.RGBA {
	r, g, b := int(p.R), int(p.G), int(p.B)
	water := tile >= 1 && tile <= 3
	forest := tile == 6 || tile >= 8 && tile <= 10
	sand := tile == 7 || tile == 30 || tile == 31 || tile == 47
	base := rgb(111, 143, 73)
	switch {
	case tile == 1:
		base = rgb(18, 53, 91)
	case tile == 2:
		base = rgb(24, 71, 110)
	case tile == 3:
		base = rgb(39, 104, 142)
	case forest:
		base = rgb(36, 76, 46)
	case tile == 12 || tile == 13:
		base = rgb(105, 109, 96)
	case tile == 11 || tile == 14 || tile == 15:
		base = rgb(108, 127, 71)
	case sand:
		base = rgb(188, 155, 86)
	case tile == 4:
		base = rgb(73, 100, 74)
	}
	switch {
	case p.A == 0 || max(r, g, b) < 40:
		return base
	case tile == 143:
		if r > 150 {
			return rgb(184, 85, 43)
		}

		return rgb(113, 67, 51)
	case b*10 > r*12 && b*10 > g*11:
		if water {
			return rgb(30, 85, 130)
		}

		return rgb(29, 86, 134)
	case b > 110 && g > 110 && r < 100:
		return rgb(88, 154, 168)
	case g*10 > r*12 && g*10 > b*12:
		if forest {
			if g < 200 {
				return rgb(62, 112, 61)
			}

			return rgb(83, 133, 65)
		}
		if sand {
			return rgb(118, 139, 70)
		}

		return rgb(91, 127, 62)
	case r > 80 && r*100 > g*125 && b < 100:
		return rgb(150, 107, 64)
	case r > 110 && g > 60 && b < 90:
		if sand {
			return rgb(220, 190, 112)
		}

		return rgb(170, 137, 78)
	case max(r, g, b)-min(r, g, b) < 40:
		if max(r, g, b) > 180 {
			return rgb(201, 202, 177)
		}

		return rgb(141, 148, 130)
	default:
		return rgb(max(60, r*7/10), max(70, g*7/10), max(50, b*55/100))
	}
}

func rgb(r, g, b int) color.RGBA {
	const maxChannel = 255

	return color.RGBA{
		R: uint8(max(0, min(maxChannel, r))), G: uint8(max(0, min(maxChannel, g))),
		B: uint8(max(0, min(maxChannel, b))), A: maxChannel,
	}
}
