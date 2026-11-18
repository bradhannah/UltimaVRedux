package widgets

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestDungeonViewClassificationAndCompleteKey(t *testing.T) {
	variants := []struct {
		cell references.DungeonCell
		icon dungeonViewIcon
	}{
		{0x00, dungeonViewIconNone}, {0x08, dungeonViewIconCeilingPit},
		{0x10, dungeonViewIconLadderUp}, {0x20, dungeonViewIconLadderDown}, {0x30, dungeonViewIconLadderBoth},
		{0x4f, dungeonViewIconChest}, {0x50, dungeonViewIconFountain},
		{0x60, dungeonViewIconOpenPit}, {0x61, dungeonViewIconHiddenPit}, {0x62, dungeonViewIconTrap},
		{0x68, dungeonViewIconPitCeiling}, {0x69, dungeonViewIconHiddenPitCeiling},
		{0x70, dungeonViewIconNone}, {0x83, dungeonViewIconField}, {0x90, dungeonViewIconNone},
		{0xa2, dungeonViewIconRoom}, {0xff, dungeonViewIconRoom},
		{0xb0, dungeonViewIconNone}, {0xb1, dungeonViewIconPlaque},
		{0xc0, dungeonViewIconNone}, {0xc1, dungeonViewIconNone}, {0xc8, dungeonViewIconNone},
		{0xd0, dungeonViewIconHiddenDoor}, {0xe0, dungeonViewIconDoor},
	}
	for _, variant := range variants {
		assert.Equal(t, variant.icon, dungeonViewCellPlanFor(variant.cell).icon, "%02x", variant.cell.Raw())
	}
	// Every packed variant either uses a fixed terrain entry or a unique feature key.
	for raw := range 256 {
		cell := dungeonViewCellPlanFor(references.DungeonCell(raw))
		if raw >= 0xb0 && raw < 0xe0 {
			assert.Equal(t, dungeonViewSurfaceWall, cell.surface, "%02x", raw)
		}
		if cell.icon == dungeonViewIconNone {
			continue
		}
		matches := 0
		for _, entry := range dungeonViewFeatureKey() {
			if entry.plan.icon == cell.icon {
				matches++
				assert.Equal(t, entry.plan.surface, cell.surface)
			}
		}
		assert.Equal(t, 1, matches, "unexplained or ambiguous feature %02x", raw)
	}
	wall := dungeonViewSymbolImage(dungeonViewCellPlanFor(0xb0), references.DungeonFacingNorth)
	for raw := 0xc0; raw <= 0xcf; raw++ {
		assert.Equal(t, wall.Pix, dungeonViewSymbolImage(dungeonViewCellPlanFor(references.DungeonCell(raw)),
			references.DungeonFacingNorth).Pix, "decorative blocker %02x must be an ordinary wall", raw)
	}
}

func TestDungeonViewPreservesFullRevealBeforeCollapsingToFloor(t *testing.T) {
	model, err := game_state.BuildDungeonViewModel(game_state.DungeonStateSnapshot{
		Location: references.Deceit, Facing: references.DungeonFacingWest,
	}, references.PlayerLocation{Location: references.Deceit, Position: references.Position{X: 3, Y: 4}})
	require.NoError(t, err)
	require.Len(t, model.Cells, 22)
	assert.Equal(t, 484, model.VisibleCount)
	assert.True(t, model.Cells[11][11].Player)
	plan := buildDungeonViewRenderPlan(model)
	assert.Equal(t, 64, visibleDungeonViewAtlasCells(plan))
	assert.True(t, plan.cells[4][3].player)
	players := 0
	for _, row := range plan.cells {
		for _, cell := range row {
			if cell.player {
				players++
			}
		}
	}
	assert.Equal(t, 1, players)
}

func TestDungeonViewAtlasCollapsesVisibleOccurrencesUniquely(t *testing.T) {
	var first, second game_state.DungeonViewModel
	setDungeonViewDisplayCell(&first, 4, 5, 2, 3, 0x4f, false)
	setDungeonViewDisplayCell(&first, 12, 13, 2, 3, 0x4f, true)
	setDungeonViewDisplayCell(&second, 17, 18, 2, 3, 0x4f, true)
	plan := buildDungeonViewRenderPlan(first)
	assert.Equal(t, buildDungeonViewRenderPlan(second).cells, plan.cells)
	assert.Equal(t, 1, visibleDungeonViewAtlasCells(plan))
	key := dungeonViewLegendFor(plan, first.Facing)
	// The party, the hatching, then the one group with something revealed.
	assert.Equal(t, []string{"You: facing north", "Not revealed", "FINDS", "Chest"}, dungeonViewKeyLabels(key))
}

func TestDungeonViewUnknownContentsCannotChangeMapKeyOrWrap(t *testing.T) {
	var model game_state.DungeonViewModel
	setDungeonViewDisplayCell(&model, 10, 10, 7, 3, 0x00, true)
	setDungeonViewDisplayCell(&model, 15, 15, 0, 3, 0x00, false) // Visible elsewhere is not a known seam.
	before := buildDungeonViewRenderPlan(model)
	require.Empty(t, dungeonViewContinuations(before))
	for _, raw := range []references.DungeonCell{0x08, 0x30, 0x4f, 0x68, 0x69, 0x80, 0xc0, 0xd0, 0xe0} {
		model.Cells[10][11] = game_state.DungeonViewCell{
			Visible: false, Player: true, SourceCell: raw, SourcePosition: references.Position{X: 0, Y: 3},
		}
		after := buildDungeonViewRenderPlan(model)
		assert.Equal(t, before, after)
		assert.Equal(t, dungeonViewLegendFor(before, model.Facing), dungeonViewLegendFor(after, model.Facing))
		assert.Empty(t, dungeonViewContinuations(after))
	}
	// Unknown positions themselves are not information authority either.
	model.Cells[10][11].SourcePosition = references.Position{X: 255, Y: 255}
	assert.Equal(t, before, buildDungeonViewRenderPlan(model))
}

func TestDungeonViewWrapRequiresObservedNonWallPairsOnBothAxes(t *testing.T) {
	var model game_state.DungeonViewModel
	setDungeonViewDisplayCell(&model, 10, 10, 7, 3, 0x00, false)
	setDungeonViewDisplayCell(&model, 11, 10, 0, 3, 0x60, false) // A pit is shown, not promised safe.
	setDungeonViewDisplayCell(&model, 14, 10, 4, 7, 0xe0, false)
	setDungeonViewDisplayCell(&model, 14, 11, 4, 0, 0x80, false)
	setDungeonViewDisplayCell(&model, 17, 10, 2, 5, 0x00, false)
	setDungeonViewDisplayCell(&model, 18, 10, 4, 5, 0x00, false) // Wrong source neighbor.
	plan := buildDungeonViewRenderPlan(model)
	assert.False(t, plan.edges.east[5][2])
	assert.Equal(t, []dungeonViewContinuation{
		{axis: dungeonViewAxisHorizontal, index: 3, label: "W-E4"},
		{axis: dungeonViewAxisVertical, index: 4, label: "N-S5"},
	}, dungeonViewContinuations(plan))
	for _, blocked := range []references.DungeonCell{0xb0, 0xc0, 0xc8, 0xd0} {
		model.Cells[10][11].SourceCell = blocked
		model.Cells[11][14].SourceCell = blocked
		assert.Empty(t, dungeonViewContinuations(buildDungeonViewRenderPlan(model)))
	}
}

func TestDungeonViewMapAndKeyHaveExactRGBAArtworkAndBackground(t *testing.T) {
	model := dungeonViewSampleModel("dense")
	plan := buildDungeonViewRenderPlan(model)
	key := dungeonViewLegendFor(plan, model.Facing)
	// Expose each feature independently of the marker, plus the two fixed entries.
	for row := range plan.cells {
		for column := range plan.cells[row] {
			plan.cells[row][column].player = false
		}
	}
	plan.cells[6][6].player = true
	plan.cells[7][7] = dungeonViewCellPlan{}
	raster := dungeonViewMapImage(plan, model.Facing)
	for _, entry := range key {
		if entry.heading {
			continue // A group title has no symbol.
		}
		symbol := dungeonViewSymbolImage(entry.plan, model.Facing)
		matched := false
		for row, cells := range plan.cells {
			for column, cell := range cells {
				if cell == entry.plan {
					// The outer pixel of a map cell may carry a wall's lit edge; the key has none.
					inner := symbol.Bounds().Inset(1)
					at := image.Pt(column*dungeonViewCellSize, row*dungeonViewCellSize)
					assert.Equal(t, dungeonViewCropPixels(symbol, inner), dungeonViewCropPixels(raster, inner.Add(at)), entry.label)
					matched = true
				}
			}
		}
		require.True(t, matched, "sample lacks key entry %s", entry.label)
		for pixel := 3; pixel < len(symbol.Pix); pixel += 4 {
			require.Equal(t, byte(255), symbol.Pix[pixel], "key backgrounds must be opaque: %s", entry.label)
		}
	}
}

// Dark stone: a wall is lit only on a side known to face a passage, and only on
// the map. Wall-to-wall seams and the key's wall swatch stay plain.
func TestDungeonViewWallsShowALitEdgeOnlyFacingPassages(t *testing.T) {
	model := dungeonViewSampleModel("simple")
	plan := buildDungeonViewRenderPlan(model)
	require.Equal(t, dungeonViewSurfaceWall, plan.cells[1][0].surface, "fixture: outer wall")
	require.Equal(t, dungeonViewSurfaceFloor, plan.cells[1][1].surface, "fixture: passage beside it")
	require.True(t, plan.edges.east[1][0], "fixture: the seam between them is known")
	raster := dungeonViewMapImage(plan, model.Facing)
	const last, middle = dungeonViewCellSize - 1, dungeonViewCellSize / 2
	lit, stone := pkgcolor.EGALightGray.ToRGBA(), pkgcolor.EGADarkGray.ToRGBA()
	assert.Equal(t, lit, raster.RGBAAt(last, dungeonViewCellSize+middle), "the side facing the passage")
	assert.Equal(t, stone, raster.RGBAAt(last-1, dungeonViewCellSize+middle), "the edge is one pixel deep")
	assert.Equal(t, stone, raster.RGBAAt(middle, dungeonViewCellSize), "the side facing another wall")
	assert.NotEqual(t, lit, raster.RGBAAt(dungeonViewCellSize, dungeonViewCellSize+middle), "the passage itself is not edged")
	swatch := dungeonViewSymbolImage(dungeonViewCellPlanFor(0xb0), model.Facing)
	assert.Equal(t, stone, swatch.RGBAAt(last, middle), "the key's wall has no edge")
}

func TestDungeonViewPartyPreservesEveryFeatureInAllFacings(t *testing.T) {
	for _, entry := range dungeonViewFeatureKey() {
		original := dungeonViewSymbolImage(entry.plan, references.DungeonFacingNorth)
		background := image.NewRGBA(original.Bounds())
		dungeonViewInk{canvas: background}.surface(entry.plan)
		featurePixels := 0
		for row := range dungeonViewCellSize {
			for column := range dungeonViewCellSize {
				if original.RGBAAt(column, row) != background.RGBAAt(column, row) {
					featurePixels++
					require.True(t, image.Pt(column, row).In(image.Rect(4, 4, 16, 16)),
						"%s artwork enters the party frame", entry.label)
				}
			}
		}
		require.Positive(t, featurePixels, "%s has no identifying artwork", entry.label)
		for _, facing := range dungeonViewTestFacings() {
			marked := entry.plan
			marked.player = true
			raster := dungeonViewSymbolImage(marked, facing)
			playerKey := dungeonViewCellPlanFor(0x00)
			playerKey.player = true
			keyRaster := dungeonViewSymbolImage(playerKey, facing)
			for _, edge := range []image.Rectangle{
				image.Rect(0, 0, 20, 4), image.Rect(0, 16, 20, 20),
				image.Rect(0, 4, 4, 16), image.Rect(16, 4, 20, 16),
			} {
				assert.Equal(t, dungeonViewCropPixels(keyRaster, edge), dungeonViewCropPixels(raster, edge))
			}
			assert.Equal(t, dungeonViewCropPixels(original, image.Rect(4, 4, 16, 16)),
				dungeonViewCropPixels(raster, image.Rect(4, 4, 16, 16)), "%s facing %d", entry.label, facing)
			// Independent tip expectations protect orientation, not just rotation implementation.
			tips := map[references.DungeonFacing]image.Point{
				references.DungeonFacingNorth: image.Pt(9, 0), references.DungeonFacingEast: image.Pt(19, 9),
				references.DungeonFacingSouth: image.Pt(10, 19), references.DungeonFacingWest: image.Pt(0, 10),
			}
			for direction, tip := range tips {
				if direction == facing {
					assert.Equal(t, pkgcolor.EGAYellow.ToRGBA(), raster.RGBAAt(tip.X, tip.Y))
				} else {
					assert.Equal(t, pkgcolor.EGABlack.ToRGBA(), raster.RGBAAt(tip.X, tip.Y))
				}
			}
		}
	}
}

func TestDungeonViewCompactLayoutRetainsReadableKey(t *testing.T) {
	setResponsiveDialogTestResolution(t, 320, 560)
	dialog := dungeonViewLayoutFixture(image.Pt(320, 560), dungeonViewSampleModel("simple"))
	layout := dialog.layout
	assert.GreaterOrEqual(t, dialog.titleOutput.Font.TextFace.Size, float64(dungeonViewMinimumTextSize))
	assert.Equal(t, dialog.titleOutput.Font.TextFace.Size, dialog.legendOutput.Font.TextFace.Size)
	assert.GreaterOrEqual(t, layout.legend.Min.Y, layout.mapArea.Max.Y)
	assert.True(t, layout.legend.In(localDungeonViewBounds(dialog.border)))
	assert.False(t, layout.legend.Overlaps(layout.footer))
	for index, entry := range dialog.legend {
		row := layout.legendRow(index)
		dialog.legendOutput.Reset()
		dialog.legendOutput.AppendLine().White(entry.label).Done()
		label := dialog.legendOutput.WrappedCopyForWidth(row.Dx() - dungeonViewCellSize - layout.gap)
		assert.LessOrEqual(t, label.GetTotalHeightForLines(label.GetPanelContentLineCount()), float64(row.Dy()))
	}
}

func TestDungeonViewIntegerScalingPreservesSymbolRGBA(t *testing.T) {
	plan := dungeonViewCellPlanFor(0x69)
	plan.player = true
	symbol := dungeonViewSymbolImage(plan, references.DungeonFacingWest)
	for _, scale := range []int{2, 3} {
		side := dungeonViewCellSize*scale + 4
		canvas := ebiten.NewImage(side, side)
		drawDungeonViewRaster(canvas, symbol, image.Pt(2, 2), scale)
		pixels := dungeonViewImagePixels(canvas)
		canvas.Deallocate()
		for row := range dungeonViewCellSize * scale {
			for column := range dungeonViewCellSize * scale {
				pixel := symbol.RGBAAt(column/scale, row/scale)
				offset := ((row+2)*side + column + 2) * 4
				require.Equal(t, []byte{pixel.R, pixel.G, pixel.B, pixel.A}, pixels[offset:offset+4])
			}
		}
	}
}

func TestDungeonViewReducedColourRetainsTerrainAndFeatureDistinctions(t *testing.T) {
	entries := dungeonViewLegendFor(buildDungeonViewRenderPlan(dungeonViewSampleModel("dense")), references.DungeonFacingNorth)
	seen := make(map[string]string)
	for _, entry := range entries {
		if entry.heading {
			continue // A group title has no symbol.
		}
		pixels := dungeonViewSymbolImage(entry.plan, references.DungeonFacingNorth).Pix
		dungeonViewReduceColour(pixels)
		previous, duplicate := seen[string(pixels)]
		assert.False(t, duplicate, "%s collapses to %s in four-level greyscale", entry.label, previous)
		seen[string(pixels)] = entry.label
	}
	// Solid wall coverage, plain passage and patterned unknown remain different shapes.
	counts := make([]int, 0, 3)
	for _, terrain := range []dungeonViewCellPlan{dungeonViewCellPlanFor(0xb0), dungeonViewCellPlanFor(0x00), {}} {
		pixels := dungeonViewSymbolImage(terrain, references.DungeonFacingNorth).Pix
		dungeonViewReduceColour(pixels)
		lit := 0
		for offset := 0; offset < len(pixels); offset += 4 {
			if pixels[offset] != 0 {
				lit++
			}
		}
		counts = append(counts, lit)
	}
	assert.Greater(t, counts[0], 300)
	assert.Less(t, counts[1], 10)
	assert.Greater(t, counts[2], 50)
	assert.Less(t, counts[2], 150)
}

func TestDungeonViewLayoutFitsRevealedKeyAndPlainEdgeLabels(t *testing.T) {
	for _, size := range []image.Point{image.Pt(1280, 720), image.Pt(1920, 1080), image.Pt(2560, 1440), image.Pt(3840, 2160)} {
		for _, sample := range dungeonViewSampleNames() {
			t.Run(fmt.Sprintf("%s/%dx%d", sample, size.X, size.Y), func(t *testing.T) {
				setResponsiveDialogTestResolution(t, size.X, size.Y)
				dialog := dungeonViewLayoutFixture(size, dungeonViewSampleModel(sample))
				layout := dialog.layout
				bounds := localDungeonViewBounds(dialog.border)
				regions := []image.Rectangle{layout.header, layout.north, layout.mapArea, layout.legend, layout.footer}
				for index, region := range regions {
					require.True(t, region.In(bounds), "%v outside %v", region, bounds)
					for _, other := range regions[index+1:] {
						assert.False(t, region.Overlaps(other), "%v overlaps %v", region, other)
					}
				}
				assert.Equal(t, layout.mapGrid.Dx(), layout.mapGrid.Dy())
				assert.GreaterOrEqual(t, layout.mapScale, 3)
				assert.Less(t, layout.legendScale, layout.mapScale)
				assert.Less(t, layout.legend.Dx()*layout.legend.Dy(), layout.mapGrid.Dx()*layout.mapGrid.Dy(),
					"the revealed key must remain subordinate to the map")
				assert.GreaterOrEqual(t, dungeonViewCellSize*layout.legendScale, 20,
					"key art must retain every native symbol pixel")
				assert.Equal(t, (len(dialog.legend)+layout.legendColumns-1)/layout.legendColumns, layout.legendRows)
				for index, entry := range dialog.legend {
					row := layout.legendRow(index)
					require.True(t, row.In(layout.legend))
					textWidth := row.Dx() - dungeonViewCellSize*layout.legendScale - layout.gap
					width, _ := dialog.legendOutput.Font.MeasureString(entry.label)
					assert.LessOrEqual(t, width, float64(textWidth), entry.label)
					dialog.legendOutput.Reset()
					dialog.legendOutput.AppendLine().White(entry.label).Done()
					label := dialog.legendOutput.WrappedCopyForWidth(textWidth)
					assert.LessOrEqual(t, label.GetTotalHeightForLines(label.GetPanelContentLineCount()), float64(row.Dy()))
				}
				if sample == "all-wrapping" {
					require.Len(t, dialog.continuations, 16, "all eight seams on both axes must be exercised")
					for index := range 8 {
						assert.Contains(t, dialog.continuations, dungeonViewContinuation{
							axis: dungeonViewAxisHorizontal, index: index, label: fmt.Sprintf("W-E%d", index+1),
						})
						assert.Contains(t, dialog.continuations, dungeonViewContinuation{
							axis: dungeonViewAxisVertical, index: index, label: fmt.Sprintf("N-S%d", index+1),
						})
					}
				}
				assertDungeonViewEdgeTextBounds(t, dialog)
				assert.Contains(t, dialog.footerOutput.GetOutputStr(false), "[Any key] Close")
				// The window is only as large as the view needs, inside the room it may take.
				room := newDialogBorder(dialog.placement).GetBounds()
				assert.True(t, dialog.bounds.In(room), "%v outside %v", dialog.bounds, room)
				assert.Less(t, dialog.bounds.Dx(), room.Dx(), "the game stays in view beside the window")
				assert.LessOrEqual(t, dialog.textWidth, bounds.Dx(), "heading and footer fit on one line")
			})
		}
	}
}

func assertDungeonViewEdgeTextBounds(t *testing.T, dialog *DungeonViewDialog) {
	t.Helper()
	layout := dialog.layout
	cellSize := dungeonViewCellSize * layout.mapScale
	assert.GreaterOrEqual(t, dialog.edgeOutput.Font.TextFace.Size, float64(dungeonViewMinimumTextSize))
	var labels []image.Rectangle
	for _, mark := range dialog.continuations {
		pair := layout.continuationBounds(mark)
		for _, label := range pair {
			require.True(t, label.In(layout.mapArea), "%s outside map gutters", mark.label)
			assert.False(t, label.Overlaps(layout.mapGrid))
			width, _ := dialog.edgeOutput.Font.MeasureString(mark.label)
			assert.LessOrEqual(t, width, float64(label.Dx()), mark.label)
			assert.LessOrEqual(t, dialog.edgeOutput.GetFontHeight(), float64(label.Dy()), mark.label)
			if mark.axis == dungeonViewAxisVertical {
				columnStart := layout.mapGrid.Min.X + mark.index*cellSize
				assert.GreaterOrEqual(t, label.Min.X, columnStart)
				assert.LessOrEqual(t, label.Max.X, columnStart+cellSize)
				assert.GreaterOrEqual(t, cellSize-label.Dx(), layout.gap,
					"adjacent N-S labels need readable separation")
				assert.InDelta(t, float64(columnStart)+float64(cellSize)/2,
					float64(label.Min.X+label.Max.X)/2, 1)
			} else {
				rowStart := layout.mapGrid.Min.Y + mark.index*cellSize
				assert.GreaterOrEqual(t, label.Min.Y, rowStart)
				assert.LessOrEqual(t, label.Max.Y, rowStart+cellSize)
				assert.InDelta(t, float64(rowStart)+float64(cellSize)/2,
					float64(label.Min.Y+label.Max.Y)/2, 1)
			}
			labels = append(labels, label)
		}
		if mark.axis == dungeonViewAxisVertical {
			assert.LessOrEqual(t, pair[0].Max.Y, layout.mapGrid.Min.Y-layout.gap)
			assert.GreaterOrEqual(t, pair[1].Min.Y, layout.mapGrid.Max.Y+layout.gap)
		} else {
			assert.LessOrEqual(t, pair[0].Max.X, layout.mapGrid.Min.X-layout.gap)
			assert.GreaterOrEqual(t, pair[1].Min.X, layout.mapGrid.Max.X+layout.gap)
		}
	}
	for index, label := range labels {
		for _, other := range labels[index+1:] {
			assert.False(t, label.Overlaps(other), "%v overlaps %v", label, other)
		}
	}
}

func TestDungeonViewDialogClosesOnAnyFreshKeyOnRepeatedOpening(t *testing.T) {
	setResponsiveDialogTestResolution(t, 1280, 720)
	held := map[ebiten.Key]bool{ebiten.KeyEnter: true}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	for cycle, closeKey := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyEscape} {
		dialog := NewDungeonViewDialog(DungeonViewDialogConfig{
			Placement: dungeonViewSamplePlacement(), Keyboard: keyboard, Model: dungeonViewSampleModel("simple"),
		})
		dialog.Update()
		require.False(t, dialog.IsDismissed(), "held invoking key closed cycle %d", cycle)
		clear(held)
		keyboard.ClearFrameConsumption()
		dialog.Update()
		require.False(t, dialog.IsDismissed())
		held[closeKey] = true
		keyboard.SetAllowKeyPressImmediately()
		dialog.Update()
		require.True(t, dialog.IsDismissed())
		dialog.Dismiss()
		assert.True(t, dialog.IsDismissed())
		dialog.buffer.Deallocate()
	}
}

func setDungeonViewDisplayCell(model *game_state.DungeonViewModel, displayX, displayY, sourceX, sourceY int,
	cell references.DungeonCell, player bool,
) {
	model.Cells[displayY][displayX] = game_state.DungeonViewCell{
		Visible: true, Player: player, SourceCell: cell,
		SourcePosition: references.Position{X: references.Coordinate(sourceX), Y: references.Coordinate(sourceY)},
	}
}

func visibleDungeonViewAtlasCells(plan dungeonViewRenderPlan) int {
	count := 0
	for _, row := range plan.cells {
		for _, cell := range row {
			if cell.visible {
				count++
			}
		}
	}
	return count
}

func dungeonViewKeyLabels(entries []dungeonViewLegendEntry) []string {
	labels := make([]string, len(entries))
	for index, entry := range entries {
		labels[index] = entry.label
	}
	return labels
}

func dungeonViewCropPixels(raster *image.RGBA, bounds image.Rectangle) []byte {
	pixels := make([]byte, 0, bounds.Dx()*bounds.Dy()*4)
	for row := bounds.Min.Y; row < bounds.Max.Y; row++ {
		for column := bounds.Min.X; column < bounds.Max.X; column++ {
			pixel := raster.RGBAAt(column, row)
			pixels = append(pixels, pixel.R, pixel.G, pixel.B, pixel.A)
		}
	}
	return pixels
}

func dungeonViewReduceColour(pixels []byte) {
	for offset := 0; offset < len(pixels); offset += 4 {
		grey := color.GrayModel.Convert(color.RGBA{
			R: pixels[offset], G: pixels[offset+1], B: pixels[offset+2], A: 255,
		}).(color.Gray).Y
		level := byte(min(3, (int(grey)+42)/85) * 85)
		pixels[offset], pixels[offset+1], pixels[offset+2] = level, level, level
	}
}

// Also used by the existing popup resize regression; keep its bounded readback helper.
func dungeonViewImagePixels(img *ebiten.Image) []byte {
	pixels := make([]byte, img.Bounds().Dx()*img.Bounds().Dy()*4)
	img.ReadPixels(pixels)
	return pixels
}
