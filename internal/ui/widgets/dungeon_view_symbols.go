package widgets

import (
	"image"
	"image/color"
	"image/draw"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const dungeonViewCellSize = 20

// Map and key use this one raster vocabulary, including opaque terrain backgrounds.
// The central 12x12 feature area is never touched by the party's peripheral marker.
// Pixel geometry is authored here; it does not use original game artwork or fonts.
func dungeonViewSymbolImage(plan dungeonViewCellPlan, facing references.DungeonFacing) *image.RGBA {
	canvas := image.NewRGBA(image.Rect(0, 0, dungeonViewCellSize, dungeonViewCellSize))
	ink := dungeonViewInk{canvas: canvas}
	ink.surface(plan)
	if plan.visible {
		ink.feature(plan.icon)
		if plan.player {
			ink.player(facing)
		}
	}

	return canvas
}

func dungeonViewMapImage(plan dungeonViewRenderPlan, facing references.DungeonFacing) *image.RGBA {
	const size = dungeonViewCellSize * dungeonViewFloorCells
	canvas := image.NewRGBA(image.Rect(0, 0, size, size))
	for row, cells := range plan.cells {
		for column, cell := range cells {
			symbol := dungeonViewSymbolImage(cell, facing)
			at := image.Pt(column*dungeonViewCellSize, row*dungeonViewCellSize)
			draw.Draw(canvas, symbol.Bounds().Add(at), symbol, image.Pt(0, 0), draw.Src)
		}
	}
	// Dark stone: a wall shows a lit edge on each side known to face a passage, so
	// corridors read at a glance. Only the map carries it; key swatches do not.
	for row := range dungeonViewFloorCells {
		for column := range dungeonViewFloorCells {
			if plan.edges.east[row][column] {
				dungeonViewLightWallEdge(canvas, plan, image.Pt(column, row), image.Pt((column+1)%dungeonViewFloorCells, row))
			}
			if plan.edges.south[row][column] {
				dungeonViewLightWallEdge(canvas, plan, image.Pt(column, row), image.Pt(column, (row+1)%dungeonViewFloorCells))
			}
		}
	}

	return canvas
}

// dungeonViewLightWallEdge lights the side of a wall that faces a passage. The
// second cell is the first's east or south neighbour, which may be across the
// floor's wrap.
func dungeonViewLightWallEdge(canvas *image.RGBA, plan dungeonViewRenderPlan, first, second image.Point) {
	const last = dungeonViewCellSize - 1
	near, far := plan.cells[first.Y][first.X], plan.cells[second.Y][second.X]
	nearIsWall, farIsWall := near.surface == dungeonViewSurfaceWall, far.surface == dungeonViewSurfaceWall
	if !near.visible || !far.visible || nearIsWall == farIsWall {
		return
	}
	// The line sits just inside the wall: on its far side when the wall comes first.
	wall, offset := second, 0
	if nearIsWall {
		wall, offset = first, last
	}
	edge := image.Rect(0, offset, dungeonViewCellSize, offset+1)
	if first.Y == second.Y {
		edge = image.Rect(offset, 0, offset+1, dungeonViewCellSize)
	}
	at := image.Pt(wall.X*dungeonViewCellSize, wall.Y*dungeonViewCellSize)
	draw.Draw(canvas, edge.Add(at), image.NewUniform(pkgcolor.EGALightGray), image.Pt(0, 0), draw.Src)
}

type dungeonViewInk struct {
	canvas *image.RGBA
}

func (ink dungeonViewInk) rect(x, y, width, height int, shade color.Color) {
	draw.Draw(ink.canvas, image.Rect(x, y, x+width, y+height), image.NewUniform(shade), image.Pt(0, 0), draw.Src)
}

//nolint:mnd // Native pixel geometry: dark stone walls, plain passages, diagonal unknown hatching.
func (ink dungeonViewInk) surface(plan dungeonViewCellPlan) {
	ink.rect(0, 0, dungeonViewCellSize, dungeonViewCellSize, pkgcolor.EGABlack)
	if !plan.visible {
		for row := range dungeonViewCellSize {
			for column := range dungeonViewCellSize {
				if (column-row+dungeonViewCellSize)%5 == 0 {
					ink.rect(column, row, 1, 1, pkgcolor.EGADarkGray)
				}
			}
		}

		return
	}
	switch plan.surface {
	case dungeonViewSurfaceWall:
		// Walls recede as one dark mass, with no line round each tile, so the
		// passages and the symbols on them are the brightest things in view.
		ink.rect(0, 0, dungeonViewCellSize, dungeonViewCellSize, pkgcolor.EGADarkGray)
	case dungeonViewSurfaceFloor:
		ink.rect(0, 0, dungeonViewCellSize, dungeonViewCellSize, pkgcolor.From(pkgcolor.EGABlue).Intensity(0.3).RGBA())
	case dungeonViewSurfaceNone:
	}
}

//nolint:cyclop // All shared artwork dispatch is deliberately visible in one exhaustive switch.
func (ink dungeonViewInk) feature(icon dungeonViewIcon) {
	switch icon {
	case dungeonViewIconNone:
	case dungeonViewIconCeilingPit:
		ink.ceiling()
	case dungeonViewIconLadderUp:
		ink.ladder(true, false)
	case dungeonViewIconLadderDown:
		ink.ladder(false, true)
	case dungeonViewIconLadderBoth:
		ink.ladder(true, true)
	case dungeonViewIconChest:
		ink.chest()
	case dungeonViewIconFountain:
		ink.fountain()
	case dungeonViewIconOpenPit:
		ink.pit(false)
	case dungeonViewIconHiddenPit:
		ink.pit(true)
	case dungeonViewIconPitCeiling:
		ink.ceiling()
		ink.pit(false)
	case dungeonViewIconHiddenPitCeiling:
		ink.ceiling()
		ink.pit(true)
	case dungeonViewIconTrap:
		ink.trap()
	case dungeonViewIconField:
		ink.field()
	case dungeonViewIconRoom:
		ink.room()
	case dungeonViewIconPlaque:
		ink.plaque()
	case dungeonViewIconHiddenDoor:
		ink.door(true)
	case dungeonViewIconDoor:
		ink.door(false)
	}
}

//nolint:mnd // Paired rails, rungs and arrowheads stay inside the feature area.
func (ink dungeonViewInk) ladder(ascending, descending bool) {
	ink.rect(6, 6, 1, 8, pkgcolor.EGALightGray)
	ink.rect(13, 6, 1, 8, pkgcolor.EGALightGray)
	for rung := 7; rung <= 13; rung += 3 {
		ink.rect(6, rung, 8, 1, pkgcolor.EGAWhite)
	}
	if ascending {
		ink.rect(9, 4, 2, 1, pkgcolor.EGAYellow)
		ink.rect(8, 5, 4, 1, pkgcolor.EGAYellow)
		ink.rect(7, 6, 2, 1, pkgcolor.EGAYellow)
		ink.rect(11, 6, 2, 1, pkgcolor.EGAYellow)
	}
	if descending {
		ink.rect(7, 13, 2, 1, pkgcolor.EGAYellow)
		ink.rect(11, 13, 2, 1, pkgcolor.EGAYellow)
		ink.rect(8, 14, 4, 1, pkgcolor.EGAYellow)
		ink.rect(9, 15, 2, 1, pkgcolor.EGAYellow)
	}
}

//nolint:mnd // Chest silhouette with a raised lid, seam and latch.
func (ink dungeonViewInk) chest() {
	ink.rect(6, 5, 8, 2, pkgcolor.EGAYellow)
	ink.rect(5, 7, 10, 8, pkgcolor.EGAYellow)
	ink.rect(6, 7, 8, 7, pkgcolor.EGABrown)
	ink.rect(5, 9, 10, 1, pkgcolor.EGAYellow)
	ink.rect(9, 9, 2, 3, pkgcolor.EGAWhite)
}

//nolint:mnd // Water jet, falling spray, bowl and foot form a non-square silhouette.
func (ink dungeonViewInk) fountain() {
	ink.rect(9, 5, 2, 8, pkgcolor.EGALightCyan)
	ink.rect(7, 4, 6, 1, pkgcolor.EGAWhite)
	ink.rect(6, 5, 2, 2, pkgcolor.EGALightCyan)
	ink.rect(12, 5, 2, 2, pkgcolor.EGALightCyan)
	ink.rect(5, 7, 1, 2, pkgcolor.EGALightCyan)
	ink.rect(14, 7, 1, 2, pkgcolor.EGALightCyan)
	ink.rect(4, 11, 2, 2, pkgcolor.EGAWhite)
	ink.rect(14, 11, 2, 2, pkgcolor.EGAWhite)
	ink.rect(5, 13, 10, 1, pkgcolor.EGAWhite)
	ink.rect(7, 15, 6, 1, pkgcolor.EGALightGray)
}

//nolint:mnd // A high, hollow opening leaves space for a separate floor pit beneath it.
func (ink dungeonViewInk) ceiling() {
	ink.rect(6, 4, 8, 1, pkgcolor.EGALightCyan)
	ink.rect(4, 5, 2, 2, pkgcolor.EGALightCyan)
	ink.rect(14, 5, 2, 2, pkgcolor.EGALightCyan)
	ink.rect(6, 7, 8, 1, pkgcolor.EGALightCyan)
}

//nolint:mnd // A low dark opening; hidden pits have a visibly broken cover, never a bare passage.
func (ink dungeonViewInk) pit(hidden bool) {
	ink.rect(6, 9, 8, 7, pkgcolor.EGALightGray)
	ink.rect(4, 11, 12, 3, pkgcolor.EGALightGray)
	ink.rect(6, 10, 8, 5, pkgcolor.EGABlack)
	ink.rect(5, 11, 10, 3, pkgcolor.EGABlack)
	if hidden {
		ink.rect(6, 11, 4, 1, pkgcolor.EGABrown)
		ink.rect(11, 11, 3, 1, pkgcolor.EGABrown)
		ink.rect(6, 13, 3, 1, pkgcolor.EGAYellow)
		ink.rect(10, 13, 4, 1, pkgcolor.EGAYellow)
	}
}

//nolint:mnd // A crossed hazard silhouette is distinguishable without hue.
func (ink dungeonViewInk) trap() {
	for step := range 10 {
		ink.rect(5+step, 5+step, 1, 1, pkgcolor.EGALightRed)
		ink.rect(14-step, 5+step, 1, 1, pkgcolor.EGALightRed)
	}
	ink.rect(9, 9, 2, 2, pkgcolor.EGAYellow)
}

//nolint:mnd // Three lightning traces, not a floor square.
func (ink dungeonViewInk) field() {
	for column := 5; column <= 13; column += 4 {
		for row := 4; row < 16; row++ {
			offset := (row / 3) % 2
			ink.rect(column+offset, row, 1, 1, pkgcolor.EGALightMagenta)
		}
	}
}

//nolint:mnd // A broad arch with threshold steps distinguishes a room entrance from a door leaf.
func (ink dungeonViewInk) room() {
	ink.rect(7, 4, 6, 2, pkgcolor.EGAYellow)
	ink.rect(5, 6, 2, 7, pkgcolor.EGAYellow)
	ink.rect(13, 6, 2, 7, pkgcolor.EGAYellow)
	ink.rect(7, 12, 6, 1, pkgcolor.EGAWhite)
	ink.rect(6, 14, 8, 1, pkgcolor.EGALightGray)
	ink.rect(4, 15, 12, 1, pkgcolor.EGALightGray)
}

//nolint:mnd // A written plaque retains its solid wall background.
func (ink dungeonViewInk) plaque() {
	ink.rect(4, 5, 12, 10, pkgcolor.EGABrown)
	ink.rect(6, 7, 8, 1, pkgcolor.EGAWhite)
	ink.rect(6, 9, 6, 1, pkgcolor.EGAWhite)
	ink.rect(6, 11, 8, 1, pkgcolor.EGAWhite)
	ink.rect(6, 13, 4, 1, pkgcolor.EGAWhite)
}

//nolint:mnd // Secret doors are closed inset wall panels; doorways have jambs and an angled open leaf.
func (ink dungeonViewInk) door(hidden bool) {
	if hidden {
		ink.rect(5, 4, 10, 12, pkgcolor.EGABlack)
		ink.rect(6, 5, 8, 10, pkgcolor.EGADarkGray)
		for row := 6; row < 15; row += 3 {
			ink.rect(9, row, 1, 2, pkgcolor.EGALightCyan)
		}
		ink.rect(12, 11, 1, 1, pkgcolor.EGAWhite)

		return
	}
	ink.rect(5, 4, 10, 2, pkgcolor.EGALightGray)
	ink.rect(5, 6, 2, 10, pkgcolor.EGALightGray)
	ink.rect(13, 6, 2, 10, pkgcolor.EGALightGray)
	for step := range 5 {
		ink.rect(8+step, 6+step, 1, 6, pkgcolor.EGAYellow)
	}
}

//nolint:mnd // A backed frame and filled edge arrow preserve every central feature pixel for all facings.
func (ink dungeonViewInk) player(facing references.DungeonFacing) {
	ink.rect(0, 0, 20, 4, pkgcolor.EGABlack)
	ink.rect(0, 16, 20, 4, pkgcolor.EGABlack)
	ink.rect(0, 4, 4, 12, pkgcolor.EGABlack)
	ink.rect(16, 4, 4, 12, pkgcolor.EGABlack)
	ink.rect(1, 1, 18, 1, pkgcolor.EGALightGreen)
	ink.rect(1, 18, 18, 1, pkgcolor.EGALightGreen)
	ink.rect(1, 1, 1, 18, pkgcolor.EGALightGreen)
	ink.rect(18, 1, 1, 18, pkgcolor.EGALightGreen)
	for depth := range 4 {
		for across := 9 - depth; across <= 10+depth; across++ {
			point := dungeonViewMarkerPoint(across, depth, facing)
			ink.rect(point.X, point.Y, 1, 1, pkgcolor.EGAYellow)
		}
	}
}

//nolint:varnamelen // x/y are native pixel coordinates before cardinal rotation.
func dungeonViewMarkerPoint(x, y int, facing references.DungeonFacing) image.Point {
	const last = dungeonViewCellSize - 1
	switch facing {
	case references.DungeonFacingNorth:
		return image.Pt(x, y)
	case references.DungeonFacingEast:
		return image.Pt(last-y, x)
	case references.DungeonFacingSouth:
		return image.Pt(last-x, last-y)
	case references.DungeonFacingWest:
		return image.Pt(y, last-x)
	default:
		panic("invalid dungeon View facing")
	}
}
