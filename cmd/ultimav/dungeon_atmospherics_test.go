package main

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

func TestDungeonGraffitiRenderPlanMatchesEveryOriginalSign(t *testing.T) {
	tests := []struct {
		location references.Location
		payload  byte
		message  string
		column   int
		row      int
	}{
		{references.Deceit, 1, "BOTTOMLESS\n   PIT    ", 7, 10},
		{references.Despise, 1, "[E MAZE\n OF LO_\n SOULS ", 8, 9},
		{references.Wrong, 1, "[E PRISON\n   WRO]  ", 8, 10},
		{references.Covetous, 1, "[E CRYPT", 8, 11},
		{references.Covetous, 2, "UPPER \nCRYPTS", 9, 10},
		{references.Covetous, 3, "LOWER \nCRYPTS", 9, 10},
		{references.Covetous, 4, "DEBTERS\n ALLY  ", 9, 10},
		{references.Shame, 1, "D\\P", 10, 11},
		{references.Shame, 2, "D\\PER", 9, 11},
		{references.Shame, 3, "D\\PE_", 9, 11},
		{references.Hythloth, 1, "MO[ER LODE\n   MAZE   ", 7, 10},
	}
	target := references.Position{X: 4, Y: 3}
	for signIndex, test := range tests {
		t.Run(test.location.String(), func(t *testing.T) {
			snapshot, player := dungeonAtmosphereTestState(test.location)
			require.True(t, snapshot.Cells.SetCell(player.Floor, target, references.DungeonCell(0xb0|test.payload)))

			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
			operations := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.kind == dungeonRenderGraffiti
			})

			require.Len(t, operations, 1)
			operation := operations[0]
			assert.Equal(t, signIndex, operation.spriteIndex)
			assert.Equal(t, dungeonRenderCenter, operation.role)
			assert.Equal(t, 1, operation.depth)
			assert.Equal(t, target, operation.position)
			assert.Equal(t, image.Pt(test.column*8-16, test.row*8-14), image.Pt(operation.x, operation.y))
			assert.Equal(t, test.message, dungeonGraffitiSigns[signIndex].message)
		})
	}
}

func TestDungeonGraffitiRejectsInvalidWallContexts(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		position references.Position
		cell     references.DungeonCell
	}{
		{"zero payload", references.Deceit, references.Position{X: 4, Y: 3}, 0xb0},
		{"payload exceeds dungeon count", references.Deceit, references.Position{X: 4, Y: 3}, 0xb2},
		{"dungeon has no signs", references.Destard, references.Position{X: 4, Y: 3}, 0xb1},
		{"side wall", references.Deceit, references.Position{X: 3, Y: 4}, 0xb1},
		{"wrong center depth", references.Deceit, references.Position{X: 4, Y: 2}, 0xb1},
		{"wrong wall kind", references.Deceit, references.Position{X: 4, Y: 3}, 0xd1},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, player := dungeonAtmosphereTestState(test.location)
			require.True(t, snapshot.Cells.SetCell(player.Floor, test.position, test.cell))

			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)

			assert.Empty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.kind == dungeonRenderGraffiti
			}))
		})
	}
}

func TestDungeonSkeletonEyesUseEverySpecialWallPayloadAndFourOfSixtyFiveCadence(t *testing.T) {
	target := references.Position{X: 4, Y: 3}
	for _, location := range []references.Location{references.Deceit, references.Wrong, references.Covetous} {
		for payload := byte(0); payload < 16; payload++ {
			cell := references.DungeonCell(0xc0 | payload)
			samplePlan := dungeonRenderPlan{location: location, floor: 0}
			visibleCount := 0
			visibleFrame := uint64(0)
			for frame := uint64(0); frame < dungeonEyeRandomRange; frame++ {
				samplePlan.atmosphereFrame = frame
				if dungeonEyeSample(samplePlan, cell, target) < dungeonEyeVisibleSamples {
					visibleCount++
					visibleFrame = frame
				}
			}
			assert.Equal(t, dungeonEyeVisibleSamples, visibleCount, "location=%s payload=%d", location, payload)

			snapshot, player := dungeonAtmosphereTestState(location)
			require.True(t, snapshot.Cells.SetCell(player.Floor, target, cell))
			plan := buildDungeonRenderPlan(
				&snapshot,
				player,
				true,
				false,
				false,
				int64(visibleFrame*dungeonAtmosphereFrameMs),
			)
			operations := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.kind == dungeonRenderSkeletonEyes
			})

			require.Len(t, operations, 1, "location=%s payload=%d", location, payload)
			assert.Equal(t, image.Pt(75, 73), image.Pt(operations[0].x, operations[0].y))
		}
	}
}

func TestDungeonSkeletonEyesRejectUnsupportedPositionsAndDungeons(t *testing.T) {
	cell := references.DungeonCell(0xcf)
	tests := []struct {
		name     string
		location references.Location
		position references.Position
	}{
		{"unsupported dungeon", references.Shame, references.Position{X: 4, Y: 3}},
		{"side wall", references.Deceit, references.Position{X: 3, Y: 4}},
		{"wrong center depth", references.Deceit, references.Position{X: 4, Y: 2}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot, player := dungeonAtmosphereTestState(test.location)
			require.True(t, snapshot.Cells.SetCell(player.Floor, test.position, cell))
			visibleFrame := dungeonVisibleEyeFrame(test.location, player.Floor, test.position, cell)

			plan := buildDungeonRenderPlan(
				&snapshot, player, true, false, false, int64(visibleFrame*dungeonAtmosphereFrameMs),
			)

			assert.Empty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
				return op.kind == dungeonRenderSkeletonEyes
			}))
		})
	}
}

func TestDungeonStalactiteRenderPlanUsesOriginalPositionsMirroringAndOcclusion(t *testing.T) {
	type placement struct {
		name     string
		role     dungeonRenderRole
		depth    int
		position references.Position
		x        int
		y        [dungeonStalactiteVisiblePhases]int
		mirror   bool
	}
	placements := []placement{
		{"center depth one", dungeonRenderCenter, 1, references.Position{X: 4, Y: 3}, 79, [5]int{40, 47, 66, 100, 146}, false},
		{"center depth two", dungeonRenderCenter, 2, references.Position{X: 4, Y: 2}, 79, [5]int{46, 50, 62, 82, 109}, false},
		{"left depth zero", dungeonRenderLeft, 0, references.Position{X: 3, Y: 4}, 17, [5]int{14, 23, 50, 98, 159}, false},
		{"right depth zero", dungeonRenderRight, 0, references.Position{X: 5, Y: 4}, 141, [5]int{14, 23, 50, 98, 159}, true},
		{"left depth one", dungeonRenderLeft, 1, references.Position{X: 3, Y: 3}, 51, [5]int{40, 45, 60, 84, 119}, false},
		{"right depth one", dungeonRenderRight, 1, references.Position{X: 5, Y: 3}, 107, [5]int{40, 45, 60, 84, 119}, true},
	}
	cell := references.DungeonCell(0xc3)
	for _, placement := range placements {
		t.Run(placement.name, func(t *testing.T) {
			for phase := byte(0); phase < dungeonStalactitePhaseCount; phase++ {
				snapshot, player := dungeonAtmosphereTestState(references.Despise)
				require.True(t, snapshot.Cells.SetCell(player.Floor, placement.position, cell))
				frame := dungeonFrameForStalactitePhase(
					references.Despise, player.Floor, placement.position, cell, phase,
				)
				plan := buildDungeonRenderPlan(
					&snapshot, player, true, false, false, int64(frame*dungeonAtmosphereFrameMs),
				)
				operations := dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
					return op.kind == dungeonRenderStalactite && op.position == placement.position
				})

				require.Len(t, operations, 1, "phase=%d", phase)
				operation := operations[0]
				assert.Equal(t, placement.role, operation.role)
				assert.Equal(t, placement.depth, operation.depth)
				assert.Equal(t, placement.mirror, operation.mirror)
				assert.Equal(t, phase, operation.phase)
				if phase < dungeonStalactiteVisiblePhases {
					assert.Equal(t, image.Pt(placement.x, placement.y[phase]), image.Pt(operation.x, operation.y))
				} else {
					assert.Equal(t, image.Pt(0, -1), image.Pt(operation.x, operation.y))
				}

				operationIndex := dungeonPlanOpIndex(plan, operation)
				require.Greater(t, operationIndex, 0)
				owner := plan.ops[operationIndex-1]
				assert.Equal(t, dungeonRenderPanel, owner.kind)
				assert.Equal(t, operation.role, owner.role)
				assert.Equal(t, operation.position, owner.position)
				assert.Equal(t, operation.depth, owner.depth)
			}
		})
	}
}

func TestDungeonStalactitesUseOnlyTypeOneDungeonsAndSourceDepths(t *testing.T) {
	centerDepthOne := references.Position{X: 4, Y: 3}
	for _, location := range []references.Location{references.Despise, references.Destard, references.Doom} {
		snapshot, player := dungeonAtmosphereTestState(location)
		require.True(t, snapshot.Cells.SetCell(player.Floor, centerDepthOne, references.DungeonCell(0xc0)))
		plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
		assert.Len(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
			return op.kind == dungeonRenderStalactite
		}), 1, location.String())
	}

	for _, test := range []struct {
		location references.Location
		position references.Position
	}{
		{references.Shame, centerDepthOne},
		{references.Despise, references.Position{X: 4, Y: 4}},
		{references.Despise, references.Position{X: 4, Y: 1}},
		{references.Despise, references.Position{X: 3, Y: 2}},
	} {
		snapshot, player := dungeonAtmosphereTestState(test.location)
		require.True(t, snapshot.Cells.SetCell(player.Floor, test.position, references.DungeonCell(0xc0)))
		plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
		assert.Empty(t, dungeonPlanOps(plan, func(op dungeonRenderOp) bool {
			return op.kind == dungeonRenderStalactite
		}), "location=%s position=%v", test.location, test.position)
	}
}

func TestDungeonAtmosphericsAreSuppressedInGameplayDarknessIncludingSubduedGeometry(t *testing.T) {
	snapshot, player := dungeonAtmosphereTestState(references.Despise)
	require.True(t, snapshot.Cells.SetCell(player.Floor, references.Position{X: 3, Y: 4}, references.DungeonCell(0xc0)))
	require.True(t, snapshot.Cells.SetCell(player.Floor, references.Position{X: 4, Y: 3}, references.DungeonCell(0xb1)))

	plan := buildDungeonRenderPlan(&snapshot, player, false, true, false, 0)

	assert.False(t, plan.dark)
	assert.NotZero(t, plan.count)
	assert.Equal(t, dungeonSubduedUnlitBrightness, plan.brightness)
	assert.Empty(t, dungeonPlanOps(plan, dungeonAtmosphereOperation))

	snapshot, player = dungeonAtmosphereTestState(references.Deceit)
	target := references.Position{X: 4, Y: 3}
	cell := references.DungeonCell(0xc0)
	require.True(t, snapshot.Cells.SetCell(player.Floor, target, cell))
	visibleFrame := dungeonVisibleEyeFrame(player.Location, player.Floor, target, cell)
	plan = buildDungeonRenderPlan(
		&snapshot, player, false, true, false, int64(visibleFrame*dungeonAtmosphereFrameMs),
	)
	assert.NotZero(t, plan.count)
	assert.Empty(t, dungeonPlanOps(plan, dungeonAtmosphereOperation))
}

func TestDrawDungeonAtmosphericsUsesSourcePixelsAndScaledPresentation(t *testing.T) {
	canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	canvas.Fill(color.Black)

	drawDungeonSkeletonEyes(canvas, dungeonRenderOp{
		kind: dungeonRenderSkeletonEyes, x: 75, y: 73,
	})
	lightRed := ucolor.EGALightRed.ToRGBA()
	for _, point := range []image.Point{
		image.Pt(76, 73), image.Pt(77, 73), image.Pt(75, 74), image.Pt(76, 74), image.Pt(77, 74),
		image.Pt(81, 73), image.Pt(82, 73), image.Pt(81, 74), image.Pt(82, 74), image.Pt(83, 74),
	} {
		assertDungeonPixel(t, canvas, point, lightRed)
	}
	for _, point := range []image.Point{image.Pt(75, 73), image.Pt(78, 74), image.Pt(80, 73)} {
		assertDungeonPixel(t, canvas, point, color.RGBA{A: 0xff})
	}

	drawDungeonStalactite(canvas, dungeonRenderOp{
		kind: dungeonRenderStalactite, x: 79, y: 40, phase: 0,
	})
	assertDungeonPixel(t, canvas, image.Pt(78, 40), ucolor.EGABlue.ToRGBA())
	assertDungeonPixel(t, canvas, image.Pt(79, 40), ucolor.EGALightBlue.ToRGBA())
	assertDungeonPixel(t, canvas, image.Pt(79, 39), ucolor.EGABlue.ToRGBA())

	drawDungeonStalactite(canvas, dungeonRenderOp{
		kind: dungeonRenderStalactite, x: 90, y: 40, phase: 4,
	})
	assertDungeonPixel(t, canvas, image.Pt(90, 40), ucolor.EGALightCyan.ToRGBA())
	drawDungeonStalactite(canvas, dungeonRenderOp{
		kind: dungeonRenderStalactite, x: 100, y: 40, phase: 5,
	})
	assertDungeonPixel(t, canvas, image.Pt(100, 40), color.RGBA{A: 0xff})

	scaled := ebiten.NewImage(dungeonCanvasWidth*2, dungeonCanvasHeight*2)
	var options ebiten.DrawImageOptions
	options.GeoM.Scale(2, 2)
	scaled.DrawImage(canvas, &options)
	for y := 146; y < 148; y++ {
		for x := 152; x < 154; x++ {
			assertDungeonPixel(t, scaled, image.Pt(x, y), lightRed)
		}
	}
}

func TestDrawDungeonGraffitiUsesInverseRunicCells(t *testing.T) {
	canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	canvas.Fill(color.Black)
	command := dungeonRenderOp{
		kind: dungeonRenderGraffiti, spriteIndex: 0, x: 40, y: 66,
	}

	drawDungeonGraffiti(canvas, dungeonTestGraffitiOutput(), command)

	assertDungeonPixel(t, canvas, image.Pt(40+9*8+4, 66+8+4), ucolor.EGAWhite.ToRGBA())
	blackPixels := 0
	whitePixels := 0
	for y := command.y; y < command.y+dungeonGraffitiCharacterSize; y++ {
		for x := command.x; x < command.x+10*dungeonGraffitiCharacterSize; x++ {
			pixel := color.RGBAModel.Convert(canvas.At(x, y)).(color.RGBA)
			switch pixel {
			case ucolor.EGABlack.ToRGBA():
				blackPixels++
			case ucolor.EGAWhite.ToRGBA():
				whitePixels++
			}
		}
	}
	assert.Positive(t, blackPixels, "runic glyphs must cut black pixels into the inverse cells")
	assert.Positive(t, whitePixels, "inverse character-cell backgrounds must remain visible")
}

func TestDungeonAtmosphericPlanningAndDrawingAreStableAndDoNotMutateState(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		WithRandomSeed(264).
		Build()
	floor, position, facing, lastDirection := game_state.DungeonEntrySetup(
		references.Despise,
		references.FloorNumber(references.OVERWORLD),
	)
	_, err := state.DebugSetupDungeonRoute(references.Despise, floor, position, facing, lastDirection)
	require.NoError(t, err)
	player := state.MapState.PlayerLocation
	target := dungeonRelativePosition(player.Position, facing, 1, 0)
	_, err = state.TransformDungeonCell(player.Floor, target, func(references.DungeonCell) references.DungeonCell {
		return 0xb1
	})
	require.NoError(t, err)
	stalactite := dungeonRelativePosition(player.Position, facing, 0, 1)
	_, err = state.TransformDungeonCell(player.Floor, stalactite, func(references.DungeonCell) references.DungeonCell {
		return 0xc3
	})
	require.NoError(t, err)
	state.ElapsedMs = 12_300
	beforeSave, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	beforeDungeon, active := state.DungeonStateSnapshot()
	require.True(t, active)
	beforeRNGCalls := state.GetRNG().Calls()

	firstPlan := buildDungeonRenderPlan(&beforeDungeon, player, true, false, false, state.ElapsedMs)
	secondPlan := buildDungeonRenderPlan(&beforeDungeon, player, true, false, false, state.ElapsedMs)
	assert.Equal(t, firstPlan, secondPlan)
	require.NotEmpty(t, dungeonPlanOps(firstPlan, func(op dungeonRenderOp) bool {
		return op.kind == dungeonRenderGraffiti
	}))
	require.NotEmpty(t, dungeonPlanOps(firstPlan, func(op dungeonRenderOp) bool {
		return op.kind == dungeonRenderStalactite
	}))
	source := transparentDungeonSpriteSource()
	firstCanvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	secondCanvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	drawDungeonRenderPlan(
		firstCanvas, ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight), source,
		dungeonTestGraffitiOutput(), firstPlan, state.ElapsedMs,
	)
	drawDungeonRenderPlan(
		secondCanvas, ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight), source,
		dungeonTestGraffitiOutput(), secondPlan, state.ElapsedMs,
	)
	assertDungeonImagesEqual(t, firstCanvas, secondCanvas)

	afterSave, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	afterDungeon, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, beforeSave, afterSave)
	assert.Equal(t, beforeDungeon, afterDungeon)
	assert.Equal(t, beforeRNGCalls, state.GetRNG().Calls())
}

func TestIntegrationDungeonAtmosphericsOverlayOriginalPanels(t *testing.T) {
	dataDir := originaldata.Directory(t)
	source, err := sprites.LoadDungeonSprites(dataDir)
	require.NoError(t, err)
	snapshot, player := dungeonAtmosphereTestState(references.Deceit)
	target := references.Position{X: 4, Y: 3}
	cell := references.DungeonCell(0xc0)
	require.True(t, snapshot.Cells.SetCell(player.Floor, target, cell))
	visibleFrame := dungeonVisibleEyeFrame(player.Location, player.Floor, target, cell)
	plan := buildDungeonRenderPlan(
		&snapshot, player, true, false, false, int64(visibleFrame*dungeonAtmosphereFrameMs),
	)
	canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	drawDungeonRenderPlan(
		canvas,
		ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight),
		source,
		dungeonTestGraffitiOutput(),
		plan,
		int64(visibleFrame*dungeonAtmosphereFrameMs),
	)

	assertDungeonPixel(t, canvas, image.Pt(76, 73), ucolor.EGALightRed.ToRGBA())
	assertDungeonPixel(t, canvas, image.Pt(83, 74), ucolor.EGALightRed.ToRGBA())
}

func dungeonAtmosphereTestState(
	location references.Location,
) (game_state.DungeonStateSnapshot, references.PlayerLocation) {
	snapshot := game_state.DungeonStateSnapshot{
		Location: location,
		Facing:   references.DungeonFacingNorth,
	}
	player := references.PlayerLocation{
		Location: location,
		Position: references.Position{X: 4, Y: 4},
		Floor:    0,
	}

	return snapshot, player
}

func dungeonVisibleEyeFrame(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	cell references.DungeonCell,
) uint64 {
	plan := dungeonRenderPlan{location: location, floor: floor}
	for frame := uint64(0); frame < dungeonEyeRandomRange; frame++ {
		plan.atmosphereFrame = frame
		if dungeonEyeSample(plan, cell, position) < dungeonEyeVisibleSamples {
			return frame
		}
	}
	panic("deterministic eye sample produced no visible frame")
}

func dungeonFrameForStalactitePhase(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	cell references.DungeonCell,
	want byte,
) uint64 {
	plan := dungeonRenderPlan{location: location, floor: floor}
	for frame := uint64(0); frame < dungeonStalactitePhaseCount; frame++ {
		plan.atmosphereFrame = frame
		if dungeonStalactitePhase(plan, cell, position) == want {
			return frame
		}
	}
	panic("deterministic stalactite cycle omitted a phase")
}

func dungeonPlanOpIndex(plan dungeonRenderPlan, want dungeonRenderOp) int {
	for index := range plan.count {
		if plan.ops[index] == want {
			return index
		}
	}

	return -1
}

func dungeonAtmosphereOperation(operation dungeonRenderOp) bool {
	return operation.kind == dungeonRenderGraffiti ||
		operation.kind == dungeonRenderSkeletonEyes ||
		operation.kind == dungeonRenderStalactite
}

func transparentDungeonSpriteSource() syntheticDungeonSpriteSource {
	panel := ebiten.NewImage(80, dungeonCanvasHeight)

	return syntheticDungeonSpriteSource{panel: panel, item: panel}
}

func assertDungeonImagesEqual(t *testing.T, first, second *ebiten.Image) {
	t.Helper()
	require.Equal(t, first.Bounds(), second.Bounds())
	for y := first.Bounds().Min.Y; y < first.Bounds().Max.Y; y++ {
		for x := first.Bounds().Min.X; x < first.Bounds().Max.X; x++ {
			assert.Equal(t, first.At(x, y), second.At(x, y), "pixel at (%d,%d)", x, y)
		}
	}
}
