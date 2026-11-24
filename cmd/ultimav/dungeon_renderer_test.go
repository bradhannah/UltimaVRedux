package main

import (
	"fmt"
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	gametext "github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

func TestDungeonLadderOriginalJoinGeometry(t *testing.T) {
	source, err := sprites.LoadDungeonSprites(originaldata.Directory(t))
	require.NoError(t, err)
	for depth := range dungeonViewDepth {
		t.Run(fmt.Sprintf("depth_%d", depth), func(t *testing.T) {
			item, ok := source.Item(depth)
			require.True(t, ok)
			var opaque []int
			for x := range item.Bounds().Dx() {
				_, _, _, alpha := item.At(x, 0).RGBA()
				if alpha != 0 {
					opaque = append(opaque, x)
				}
			}
			t.Logf("ladder size=%v source row 0 opaque columns=%v", item.Bounds().Size(), opaque)
			require.NotEmpty(t, opaque, "authored rail mask reaches the join")
			var plan dungeonRenderPlan
			appendDungeonLadders(&plan, references.DungeonLadderBoth, references.Position{}, depth)
			canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
			for _, op := range plan.ops[:plan.count] {
				drawDungeonSprite(canvas, item, op, 1)
			}
			// The join must preserve the source mask, not draw an opaque bar.
			for x := range dungeonCanvasWidth {
				assert.Equal(t, canvas.At(x, 80), canvas.At(x, 81), "join mask/color at x=%d", x)
			}
			for _, upper := range plan.ops[:2] {
				for _, x := range opaque {
					destinationX := upper.x + x
					if upper.mirror {
						destinationX = upper.x + item.Bounds().Dx() - 1 - x
					}
					for y := 80; y <= 82; y++ {
						_, _, _, alpha := canvas.At(destinationX, y).RGBA()
						assert.NotZero(t, alpha, "rail must join at (%d,%d)", destinationX, y)
					}
				}
			}
			for _, scale := range []float64{1, 1.5, 2, 3} {
				scaled := ebiten.NewImage(int(dungeonCanvasWidth*scale), int(dungeonCanvasHeight*scale))
				options := ebiten.DrawImageOptions{}
				options.GeoM.Scale(scale, scale)
				scaled.DrawImage(canvas, &options)
				for _, upper := range plan.ops[:2] {
					x := upper.x + opaque[len(opaque)/2]
					if upper.mirror {
						x = upper.x + item.Bounds().Dx() - 1 - opaque[len(opaque)/2]
					}
					for y := int(80 * scale); y < int(83*scale); y++ {
						_, _, _, alpha := scaled.At(int((float64(x)+0.5)*scale), y).RGBA()
						assert.NotZero(t, alpha, "scale=%g row=%d", scale, y)
					}
				}
			}
			for _, kind := range []references.DungeonCellKind{references.DungeonLadderUp, references.DungeonLadderDown} {
				var single dungeonRenderPlan
				appendDungeonLadders(&single, kind, references.Position{}, depth)
				require.Equal(t, 2, single.count)
				wantY := 82
				if kind == references.DungeonLadderUp {
					wantY = []int{1, 25, 57, 73}[depth]
				}
				for _, op := range single.ops[:single.count] {
					assert.Equal(t, wantY, op.y, "single-direction placement stays unchanged")
					assert.Equal(t, depth, op.spriteIndex, "shared original masked artwork")
				}
			}
		})
	}
}

func TestDungeonWallSetForLocation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		location references.Location
		wallSet  int
	}{
		{references.Despise, 0},
		{references.Destard, 0},
		{references.Doom, 0},
		{references.Shame, 1},
		{references.Hythloth, 1},
		{references.Deceit, 2},
		{references.Wrong, 2},
		{references.Covetous, 2},
	}
	for _, test := range tests {
		wallSet, ok := dungeonWallSetForLocation(test.location)
		assert.True(t, ok, test.location.String())
		assert.Equal(t, test.wallSet, wallSet, test.location.String())
	}

	_, ok := dungeonWallSetForLocation(references.Britain)
	assert.False(t, ok)
}

func TestDungeonRenderPlanRepresentsEveryCellKindStructurally(t *testing.T) {
	t.Parallel()

	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	for kind := references.DungeonPassage; kind <= references.DungeonLiveRoom; kind++ {
		kind := kind
		t.Run(dungeonCellKindName(kind), func(t *testing.T) {
			t.Parallel()

			snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
			leftPosition := references.Position{X: 3, Y: 4}
			require.True(t, snapshot.Cells.SetCell(player.Floor, leftPosition, references.DungeonCell(byte(kind)<<4)))
			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
			left := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.role == dungeonRenderLeft && op.depth == 0 && op.position == leftPosition
			})

			require.Len(t, left, 1)
			assert.Equal(t, dungeonSidePanelIndex(kind, 0), left[0].spriteIndex)
		})
	}
}

func TestDungeonRenderPlanUsesOriginalCenterPanelIndices(t *testing.T) {
	t.Parallel()

	expected := map[references.DungeonCellKind][dungeonViewDepth]int{
		references.DungeonClearedRoom:  {12, 13, 14, 15},
		references.DungeonWall:         {12, 9, 10, 11},
		references.DungeonSpecialWall:  {12, 25, 26, 27},
		references.DungeonHiddenDoor:   {12, 9, 10, 11},
		references.DungeonRevealedDoor: {12, 13, 14, 15},
		references.DungeonLiveRoom:     {12, 13, 14, 15},
	}
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	for kind, panels := range expected {
		for depth, panel := range panels {
			snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
			position := references.Position{X: 4, Y: references.Coordinate(4 - depth)}
			require.True(t, snapshot.Cells.SetCell(player.Floor, position, references.DungeonCell(byte(kind)<<4)))
			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
			caps := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.role == dungeonRenderCenter && op.depth == depth && op.position == position
			})

			require.Len(t, caps, 2, "kind=%s depth=%d", dungeonCellKindName(kind), depth)
			assert.Equal(t, panel, caps[0].spriteIndex)
			assert.Equal(t, panel, caps[1].spriteIndex)
			assert.False(t, caps[0].mirror)
			assert.True(t, caps[1].mirror)
		}
	}
}

func TestDungeonRenderPlanOnlySuppressesCurrentSidesForExactE0Doorway(t *testing.T) {
	t.Parallel()

	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	for _, test := range []struct {
		name      string
		cell      references.DungeonCell
		wantSides bool
	}{
		{name: "plain doorway", cell: 0xe0, wantSides: false},
		{name: "doorway with bit 08", cell: 0xe8, wantSides: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
			require.True(t, snapshot.Cells.SetCell(player.Floor, player.Position, test.cell))

			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
			sides := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.depth == 0 && (op.role == dungeonRenderLeft || op.role == dungeonRenderRight)
			})
			if test.wantSides {
				assert.Len(t, sides, 2)
			} else {
				assert.Empty(t, sides)
			}
		})
	}
}

func TestDungeonRenderPlanStartsDoorwayClipAfterCurrentCenter(t *testing.T) {
	t.Parallel()

	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	for _, cell := range []references.DungeonCell{0xe0, 0xe8} {
		snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
		require.True(t, snapshot.Cells.SetCell(player.Floor, player.Position, cell))
		require.True(t, snapshot.Cells.SetCell(
			player.Floor,
			dungeonRelativePosition(player.Position, snapshot.Facing, 1, 0),
			references.DungeonCell(0x10),
		))

		plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
		require.True(t, plan.clipEnabled, "cell=%#x", cell)
		assert.Equal(t, 2, plan.clipFrom, "cell=%#x", cell)
		require.Greater(t, plan.count, plan.clipFrom, "cell=%#x", cell)
		for index := 0; index < plan.clipFrom; index++ {
			assert.Equal(t, dungeonRenderCenter, plan.ops[index].role, "cell=%#x op=%d", cell, index)
			assert.False(t, plan.clipsOperation(index), "cell=%#x op=%d", cell, index)
		}
		for index := plan.clipFrom; index < plan.count; index++ {
			assert.True(t, plan.clipsOperation(index), "cell=%#x op=%d", cell, index)
		}
		assert.NotEmpty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
			return op.role == dungeonRenderContents
		}), "cell=%#x", cell)
	}

	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	assert.False(t, plan.clipEnabled)
	for index := range plan.count {
		assert.False(t, plan.clipsOperation(index), "op=%d", index)
	}
}

func TestDungeonRenderPlanSamplesEveryFacingWithWrapping(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		facing   references.DungeonFacing
		left     references.Position
		right    references.Position
		depthTwo references.Position
	}{
		{"north", references.DungeonFacingNorth, references.Position{X: 7, Y: 0}, references.Position{X: 1, Y: 0}, references.Position{X: 0, Y: 6}},
		{"east", references.DungeonFacingEast, references.Position{X: 0, Y: 7}, references.Position{X: 0, Y: 1}, references.Position{X: 2, Y: 0}},
		{"south", references.DungeonFacingSouth, references.Position{X: 1, Y: 0}, references.Position{X: 7, Y: 0}, references.Position{X: 0, Y: 2}},
		{"west", references.DungeonFacingWest, references.Position{X: 0, Y: 1}, references.Position{X: 0, Y: 7}, references.Position{X: 6, Y: 0}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			snapshot := dungeonTestSnapshot(test.facing)
			player := dungeonTestPlayer(references.Position{})
			require.True(t, snapshot.Cells.SetCell(player.Floor, test.depthTwo, references.DungeonCell(0xb0)))
			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)

			left := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.role == dungeonRenderLeft && op.depth == 0
			})
			right := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.role == dungeonRenderRight && op.depth == 0
			})
			caps := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.role == dungeonRenderCenter && op.depth == 2
			})

			require.Len(t, left, 1)
			require.Len(t, right, 1)
			require.Len(t, caps, 2)
			assert.Equal(t, test.left, left[0].position)
			assert.Equal(t, test.right, right[0].position)
			assert.Equal(t, test.depthTwo, caps[0].position)
			assert.Empty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool { return op.depth == 3 }))
		})
	}
}

func TestDungeonRenderPlanStopsAtOpaqueCenterAndOrdersContentsFarToNear(t *testing.T) {
	t.Parallel()

	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	require.True(t, snapshot.Cells.SetCell(player.Floor, references.Position{X: 4, Y: 4}, references.DungeonCell(0x10)))
	require.True(t, snapshot.Cells.SetCell(player.Floor, references.Position{X: 4, Y: 3}, references.DungeonCell(0x40)))
	require.True(t, snapshot.Cells.SetCell(player.Floor, references.Position{X: 4, Y: 2}, references.DungeonCell(0xb0)))
	require.True(t, snapshot.Cells.SetCell(player.Floor, references.Position{X: 4, Y: 1}, references.DungeonCell(0x80)))

	plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	contents := dungeonPlanOps(plan, func(op dungeonRenderOp) bool { return op.role == dungeonRenderContents })
	require.Len(t, contents, 4)
	assert.Equal(t, []int{1, 1, 0, 0}, []int{contents[0].depth, contents[1].depth, contents[2].depth, contents[3].depth})
	assert.Empty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool { return op.depth == 3 }))

	firstContent := plan.count
	for index := 0; index < plan.count; index++ {
		if plan.ops[index].role == dungeonRenderContents {
			firstContent = index
			break
		}
	}
	for index := 0; index < firstContent; index++ {
		assert.NotEqual(t, dungeonRenderContents, plan.ops[index].role)
	}
	for index := firstContent; index < plan.count; index++ {
		assert.Equal(t, dungeonRenderContents, plan.ops[index].role)
	}
}

func TestDungeonRenderPlanUsesOriginalContentItemsForEveryCellKind(t *testing.T) {
	t.Parallel()

	type expectedContent struct {
		kind  dungeonRenderOpKind
		item  int
		top   bool
		count int
	}
	tests := []struct {
		kind     references.DungeonCellKind
		cell     references.DungeonCell
		expected []expectedContent
	}{
		{references.DungeonPassage, 0x00, nil},
		{references.DungeonLadderUp, 0x10, []expectedContent{{dungeonRenderItem, 2, true, 2}}},
		{references.DungeonLadderDown, 0x20, []expectedContent{{dungeonRenderItem, 2, false, 2}}},
		{references.DungeonLadderBoth, 0x30, []expectedContent{{dungeonRenderItem, 2, true, 2}, {dungeonRenderItem, 2, false, 2}}},
		{references.DungeonClosedChest, 0x40, []expectedContent{{dungeonRenderItem, 14, false, 2}}},
		{references.DungeonFountain, 0x50, []expectedContent{{dungeonRenderItem, 6, false, 2}}},
		{references.DungeonPitOrTrap, 0x60, []expectedContent{{dungeonRenderItem, 10, false, 2}}},
		{references.DungeonOpenChest, 0x70, []expectedContent{{dungeonRenderItem, 18, false, 2}}},
		{references.DungeonEnergyField, 0x82, []expectedContent{{dungeonRenderField, 0, false, 1}}},
		{references.DungeonReserved, 0x90, nil},
		{references.DungeonClearedRoom, 0xa0, nil},
		{references.DungeonWall, 0xb0, nil},
		{references.DungeonSpecialWall, 0xc0, nil},
		{references.DungeonHiddenDoor, 0xd0, nil},
		{references.DungeonRevealedDoor, 0xe0, nil},
		{references.DungeonLiveRoom, 0xf0, nil},
	}
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	target := references.Position{X: 4, Y: 2}
	for _, test := range tests {
		t.Run(dungeonCellKindName(test.kind), func(t *testing.T) {
			t.Parallel()

			snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
			require.True(t, snapshot.Cells.SetCell(player.Floor, target, test.cell))
			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
			contents := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.role == dungeonRenderContents && op.position == target && op.depth == 2
			})

			offset := 0
			for _, expected := range test.expected {
				require.GreaterOrEqual(t, len(contents), offset+expected.count)
				for _, op := range contents[offset : offset+expected.count] {
					assert.Equal(t, expected.kind, op.kind)
					if expected.kind == dungeonRenderItem {
						assert.Equal(t, expected.item, op.spriteIndex)
						assert.Equal(t, expected.top, op.top)
					}
				}
				offset += expected.count
			}
			assert.Len(t, contents, offset)
		})
	}
}

func TestDungeonRenderPlanOpeningAboveAndHiddenPit(t *testing.T) {
	t.Parallel()

	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	target := references.Position{X: 4, Y: 2}

	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	require.True(t, snapshot.Cells.SetCell(player.Floor, target, references.DungeonCell(0x08)))
	plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	opening := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
		return op.kind == dungeonRenderItem && op.position == target && op.depth == 2
	})
	require.Len(t, opening, 2)
	assert.Equal(t, 10, opening[0].spriteIndex)
	assert.True(t, opening[0].top)

	snapshot = dungeonTestSnapshot(references.DungeonFacingNorth)
	require.True(t, snapshot.Cells.SetCell(player.Floor, target, references.DungeonCell(0x61)))
	plan = buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	assert.Empty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
		return op.role == dungeonRenderContents && op.position == target
	}))
}

func TestDungeonRenderPlanDarknessHasNoOperations(t *testing.T) {
	t.Parallel()

	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	for y := range references.YDungeonMapTiles {
		for x := range references.XDungeonMapTiles {
			require.True(t, snapshot.Cells.SetCell(0, references.Position{X: x, Y: y}, references.DungeonCell(0xb0)))
		}
	}
	plan := buildDungeonRenderPlan(&snapshot, dungeonTestPlayer(references.Position{}), false, false, false, 0)
	assert.True(t, plan.dark)
	assert.Zero(t, plan.count)
}

func TestDungeonRenderPlanUsesFixedSubduedBrightnessOnlyWithoutGameplayLight(t *testing.T) {
	t.Parallel()

	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	require.True(t, snapshot.Cells.SetCell(player.Floor, player.Position, references.DungeonCell(0xb0)))

	originalDark := buildDungeonRenderPlan(&snapshot, player, false, false, false, 0)
	assert.True(t, originalDark.dark)
	assert.Zero(t, originalDark.count)

	subdued := buildDungeonRenderPlan(&snapshot, player, false, true, false, 0)
	assert.False(t, subdued.dark)
	assert.NotZero(t, subdued.count)
	assert.Equal(t, dungeonSubduedUnlitBrightness, subdued.brightness)

	lit := buildDungeonRenderPlan(&snapshot, player, true, true, false, 0)
	assert.False(t, lit.dark)
	assert.Equal(t, float32(1), lit.brightness)
	assert.Equal(t, buildDungeonRenderPlan(&snapshot, player, true, false, false, 0), lit)
}

func TestDungeonSurfaceEntryRenderPlanShowsUpperLadderAndWallAhead(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	floor, position, facing, lastDirection := game_state.DungeonEntrySetup(
		references.Deceit,
		references.FloorNumber(references.OVERWORLD),
	)
	_, err := state.DebugSetupDungeonRoute(references.Deceit, floor, position, facing, lastDirection)
	require.NoError(t, err)
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	player := state.MapState.PlayerLocation

	current, ok := snapshot.Cells.Cell(player.Floor, player.Position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0x10), current)
	aheadPosition := dungeonRelativePosition(player.Position, snapshot.Facing, 1, 0)
	ahead, ok := snapshot.Cells.Cell(player.Floor, aheadPosition)
	require.True(t, ok)
	assert.Equal(t, references.DungeonWall, ahead.Kind())

	plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	currentContents := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
		return op.role == dungeonRenderContents && op.depth == 0 && op.position == player.Position
	})
	require.Len(t, currentContents, 2)
	for _, op := range currentContents {
		assert.Equal(t, dungeonRenderItem, op.kind)
		assert.Zero(t, op.spriteIndex)
		assert.True(t, op.top)
	}
	aheadCaps := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
		return op.role == dungeonRenderCenter && op.depth == 1 && op.position == aheadPosition
	})
	require.Len(t, aheadCaps, 2)
	assert.Equal(t, 9, aheadCaps[0].spriteIndex)
	assert.Equal(t, 9, aheadCaps[1].spriteIndex)
}

func TestDungeonCanvasAspectFitUsesUniformScaleAndLetterboxing(t *testing.T) {
	t.Parallel()

	wideViewport := dungeonMapViewport(image.Rect(0, 0, 1000, 500))
	assert.InDelta(t, 760.0, wideViewport.width, 0.0001)
	wide := dungeonCanvasAspectFit(wideViewport)
	assert.InDelta(t, 500.0/164.0, wide.scale, 0.0001)
	assert.InDelta(t, (760.0-160.0*wide.scale)/2.0, wide.x, 0.0001)
	assert.InDelta(t, 0.0, wide.y, 0.0001)

	tallViewport := dungeonMapViewport(image.Rect(0, 0, 1000, 1000))
	tall := dungeonCanvasAspectFit(tallViewport)
	assert.InDelta(t, 760.0/160.0, tall.scale, 0.0001)
	assert.InDelta(t, 0.0, tall.x, 0.0001)
	assert.InDelta(t, (1000.0-164.0*tall.scale)/2.0, tall.y, 0.0001)
}

func TestDungeonPresentationCoordinatesNormalizeOriginalViewport(t *testing.T) {
	t.Parallel()

	plan := dungeonRenderPlan{}
	position := references.Position{X: 4, Y: 4}
	appendDungeonPanel(&plan, 12, dungeonRenderCenter, position, 0, false)
	appendDungeonPanel(&plan, 12, dungeonRenderCenter, position, 0, true)
	appendDungeonPanel(&plan, 3, dungeonRenderLeft, position, 3, false)
	appendDungeonPanel(&plan, 3, dungeonRenderRight, position, 3, true)

	assert.Equal(t, image.Pt(0, 0), image.Pt(plan.ops[0].x, plan.ops[0].y))
	assert.Equal(t, image.Pt(80, 0), image.Pt(plan.ops[1].x, plan.ops[1].y))
	assert.Equal(t, image.Pt(72, 0), image.Pt(plan.ops[2].x, plan.ops[2].y))
	assert.Equal(t, image.Pt(80, 0), image.Pt(plan.ops[3].x, plan.ops[3].y))

	assert.Equal(t, image.Pt(40, 1), itemPoint(0, 0, true, false))
	assert.Equal(t, image.Pt(80, 1), itemPoint(0, 0, true, true))
	assert.Equal(t, image.Pt(40, 138), itemPoint(12, 0, false, false))
	assert.Equal(t, image.Pt(72, 82), itemPoint(11, 3, false, false))

	minimumX, maximumX, minimumY, maximumY, count, lineLength := dungeonFieldGeometry(0)
	assert.Equal(t, []int{0, 151, 2, 153, 300, 7}, []int{
		minimumX, maximumX, minimumY, maximumY, count, lineLength,
	})
	assert.Equal(t, image.Rect(24, 28, 137, 164), dungeonDoorwayClipBounds())
}

func TestCurrentDungeonRenderStatePanicsWithoutActiveSnapshot(t *testing.T) {
	t.Parallel()

	state := &game_state.GameState{
		MapState: map_state.MapState{
			PlayerLocation: dungeonTestPlayer(references.Position{}),
		},
	}
	scene := &GameScene{gameState: state}
	assert.PanicsWithValue(t, "dungeon map has no matching active dungeon snapshot", func() {
		_, _, _ = scene.currentDungeonRenderState()
	})
}

func TestDrawDungeonRenderPlanOriginalBlackAndDeterministicSubduedPixels(t *testing.T) {
	panel := ebiten.NewImage(80, dungeonCanvasHeight)
	panel.Fill(color.RGBA{R: 0xff, A: 0xff})
	source := syntheticDungeonSpriteSource{panel: panel, item: panel}
	graffitiOutput := dungeonTestGraffitiOutput()
	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	require.True(t, snapshot.Cells.SetCell(player.Floor, player.Position, references.DungeonCell(0xb0)))

	canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	clipLayer := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	t.Cleanup(panel.Deallocate)
	t.Cleanup(canvas.Deallocate)
	t.Cleanup(clipLayer.Deallocate)
	point := image.Pt(10, 10)
	draw := func(plan dungeonRenderPlan, presentationTimeMs int64) color.RGBA {
		drawDungeonRenderPlan(canvas, clipLayer, source, graffitiOutput, plan, presentationTimeMs)

		return color.RGBAModel.Convert(canvas.At(point.X, point.Y)).(color.RGBA)
	}
	dark := draw(buildDungeonRenderPlan(&snapshot, player, false, false, false, 0), 0)
	lit := draw(buildDungeonRenderPlan(&snapshot, player, true, true, false, 0), 0)
	subduedAtNoon := draw(buildDungeonRenderPlan(&snapshot, player, false, true, false, 0), 0)
	subduedAtMidnight := draw(
		buildDungeonRenderPlan(&snapshot, player, false, true, false, 12*60*60*1000),
		12*60*60*1000,
	)

	assert.Equal(t, color.RGBA{A: 0xff}, dark)
	assert.Equal(t, color.RGBA{R: 0xff, A: 0xff}, lit)
	assert.Equal(t, color.RGBA{R: 0x2b, A: 0xff}, subduedAtNoon)
	assert.Equal(t, subduedAtNoon, subduedAtMidnight)
}

func TestDrawDungeonSpriteAppliesOriginalUpperAndMirroredOrientation(t *testing.T) {
	red := color.RGBA{R: 0xff, A: 0xff}
	green := color.RGBA{G: 0xff, A: 0xff}
	blue := color.RGBA{B: 0xff, A: 0xff}
	yellow := color.RGBA{R: 0xff, G: 0xff, A: 0xff}
	sprite := ebiten.NewImage(2, 2)
	sprite.Set(0, 0, red)
	sprite.Set(1, 0, green)
	sprite.Set(0, 1, blue)
	sprite.Set(1, 1, yellow)

	tests := []struct {
		name     string
		mirror   bool
		top      bool
		expected [4]color.RGBA
	}{
		{name: "lower left", expected: [4]color.RGBA{red, green, blue, yellow}},
		{name: "lower right", mirror: true, expected: [4]color.RGBA{green, red, yellow, blue}},
		{name: "upper left", top: true, expected: [4]color.RGBA{blue, yellow, red, green}},
		{name: "upper right", mirror: true, top: true, expected: [4]color.RGBA{yellow, blue, green, red}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			canvas := ebiten.NewImage(8, 8)
			canvas.Fill(color.Black)
			drawDungeonSprite(canvas, sprite, dungeonRenderOp{
				x: 3, y: 4, mirror: test.mirror, top: test.top,
			}, 1)

			assertDungeonPixel(t, canvas, image.Pt(3, 4), test.expected[0])
			assertDungeonPixel(t, canvas, image.Pt(4, 4), test.expected[1])
			assertDungeonPixel(t, canvas, image.Pt(3, 5), test.expected[2])
			assertDungeonPixel(t, canvas, image.Pt(4, 5), test.expected[3])
		})
	}
}

func TestDrawDungeonRenderPlanClipsDoorwayFarPanels(t *testing.T) {
	doorway := ebiten.NewImage(80, dungeonCanvasHeight)
	doorway.Fill(color.RGBA{R: 0xff, A: 0xff})
	structure := ebiten.NewImage(80, dungeonCanvasHeight)
	structure.Fill(color.RGBA{G: 0xff, A: 0xff})
	source := contrastingDungeonSpriteSource{doorway: doorway, structure: structure}
	canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	clipLayer := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	graffitiOutput := dungeonTestGraffitiOutput()
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})

	for _, cell := range []references.DungeonCell{0xe0, 0xe8} {
		snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
		require.True(t, snapshot.Cells.SetCell(player.Floor, player.Position, cell))

		plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
		drawDungeonRenderPlan(canvas, clipLayer, source, graffitiOutput, plan, 0)

		assertDungeonPixel(t, canvas, image.Pt(150, 50), color.RGBA{R: 0xff, A: 0xff})
		assertDungeonPixel(t, canvas, image.Pt(100, 50), color.RGBA{G: 0xff, A: 0xff})
		assertDungeonPixel(t, canvas, image.Pt(136, 50), color.RGBA{G: 0xff, A: 0xff})
		assertDungeonPixel(t, canvas, image.Pt(137, 50), color.RGBA{R: 0xff, A: 0xff})
		assertDungeonPixel(t, canvas, image.Pt(100, 27), color.RGBA{R: 0xff, A: 0xff})
		assertDungeonPixel(t, canvas, image.Pt(100, 28), color.RGBA{G: 0xff, A: 0xff})
		assertDungeonPixel(t, canvas, image.Pt(100, 163), color.RGBA{G: 0xff, A: 0xff})
	}
}

func TestDrawDungeonRenderPlanDoesNotClipWithoutCurrentDoorway(t *testing.T) {
	structure := ebiten.NewImage(80, dungeonCanvasHeight)
	structure.Fill(color.RGBA{G: 0xff, A: 0xff})
	source := contrastingDungeonSpriteSource{doorway: structure, structure: structure}
	canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	clipLayer := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	graffitiOutput := dungeonTestGraffitiOutput()
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)

	plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	drawDungeonRenderPlan(canvas, clipLayer, source, graffitiOutput, plan, 0)

	assertDungeonPixel(t, canvas, image.Pt(10, 10), color.RGBA{G: 0xff, A: 0xff})
}

type syntheticDungeonSpriteSource struct {
	panel *ebiten.Image
	delta *ebiten.Image
	item  *ebiten.Image
}

type contrastingDungeonSpriteSource struct {
	doorway   *ebiten.Image
	structure *ebiten.Image
}

func (s contrastingDungeonSpriteSource) Panel(_, panel int) (*ebiten.Image, bool) {
	if panel == dungeonCenterPanelBase {
		return s.doorway, s.doorway != nil
	}

	return s.structure, s.structure != nil
}

func (s contrastingDungeonSpriteSource) SpecialWallPanelDelta(_, _ int) (*ebiten.Image, bool) {
	return s.structure, s.structure != nil
}

func (s contrastingDungeonSpriteSource) Item(_ int) (*ebiten.Image, bool) {
	return s.structure, s.structure != nil
}

func (s contrastingDungeonSpriteSource) Monster(_, _ int) (*ebiten.Image, bool) {
	return s.structure, s.structure != nil
}

func (s syntheticDungeonSpriteSource) Panel(_, _ int) (*ebiten.Image, bool) {
	return s.panel, s.panel != nil
}

func (s syntheticDungeonSpriteSource) SpecialWallPanelDelta(_, _ int) (*ebiten.Image, bool) {
	return s.delta, s.delta != nil
}

func (s syntheticDungeonSpriteSource) Item(_ int) (*ebiten.Image, bool) {
	return s.item, s.item != nil
}

func (s syntheticDungeonSpriteSource) Monster(_, _ int) (*ebiten.Image, bool) {
	return s.item, s.item != nil
}

func dungeonTestSnapshot(facing references.DungeonFacing) game_state.DungeonStateSnapshot {
	return game_state.DungeonStateSnapshot{
		Facing: facing,
	}
}

func dungeonTestPlayer(position references.Position) references.PlayerLocation {
	return references.PlayerLocation{Location: references.Deceit, Position: position, Floor: 0}
}

func dungeonPlanOps(plan dungeonRenderPlan, include func(dungeonRenderOp) bool) []dungeonRenderOp {
	result := make([]dungeonRenderOp, 0, plan.count)
	for index := 0; index < plan.count; index++ {
		if include(plan.ops[index]) {
			result = append(result, plan.ops[index])
		}
	}

	return result
}

func dungeonSidePanelIndex(kind references.DungeonCellKind, depth int) int {
	switch kind {
	case references.DungeonPassage, references.DungeonLadderUp, references.DungeonLadderDown,
		references.DungeonLadderBoth, references.DungeonClosedChest, references.DungeonFountain,
		references.DungeonPitOrTrap, references.DungeonOpenChest, references.DungeonEnergyField,
		references.DungeonReserved:
		return 16 + depth
	case references.DungeonClearedRoom, references.DungeonRevealedDoor, references.DungeonLiveRoom:
		return 4 + depth
	case references.DungeonSpecialWall:
		return 20 + depth
	case references.DungeonWall, references.DungeonHiddenDoor:
		return depth
	default:
		panic("unknown dungeon cell kind")
	}
}

func dungeonCellKindName(kind references.DungeonCellKind) string {
	return []string{
		"passage", "ladder-up", "ladder-down", "ladder-both", "closed-chest", "fountain", "pit",
		"open-chest", "field", "reserved", "cleared-room", "wall", "special-wall", "hidden-door",
		"revealed-door", "live-room",
	}[kind]
}

func itemPoint(item, depth int, top, right bool) image.Point {
	x, y := dungeonItemPosition(item, depth, top, right)

	return image.Pt(x, y)
}

func assertDungeonPixel(t *testing.T, canvas *ebiten.Image, point image.Point, expected color.RGBA) {
	t.Helper()

	actual := color.RGBAModel.Convert(canvas.At(point.X, point.Y)).(color.RGBA)
	assert.Equal(t, expected, actual, "pixel at %v", point)
}

func dungeonTestGraffitiOutput() *gametext.Output {
	return newDungeonGraffitiOutput(gametext.NewUltimaFont(dungeonGraffitiCharacterSize))
}
