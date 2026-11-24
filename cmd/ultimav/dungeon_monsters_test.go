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
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestDungeonWandererRenderPlanUsesWrappedSourceGeometry(t *testing.T) {
	player := dungeonTestPlayer(references.Position{})
	groundY := [...]int{0, 72, 82, 84}
	ceilingY := [...]int{0, 26, 56, 71}
	leftX := [...]int{0, 56, 64, 72}
	for depth := 1; depth <= 3; depth++ {
		for _, onCeiling := range []bool{false, true} {
			t.Run(map[bool]string{false: "ground", true: "ceiling"}[onCeiling], func(t *testing.T) {
				snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
				target := dungeonRelativePosition(player.Position, snapshot.Facing, depth, 0)
				snapshot.Wanderer = &game_state.DungeonWanderer{
					Monster: references.MonsterRat, Floor: player.Floor, Position: target, OnCeiling: onCeiling,
				}

				plan := buildDungeonRenderPlan(&snapshot, player, true, false, true, 0)
				operations := dungeonMonsterPlanOps(plan)

				require.Len(t, operations, 2, "depth=%d ceiling=%t", depth, onCeiling)
				wantY := groundY[depth]
				if onCeiling {
					wantY = ceilingY[depth]
				}
				assert.Equal(t, target, operations[0].position)
				assert.Equal(t, depth, operations[0].depth)
				assert.Equal(t, 0, operations[0].spriteIndex)
				assert.Equal(t, image.Pt(leftX[depth], wantY), image.Pt(operations[0].x, operations[0].y))
				assert.Equal(t, image.Pt(80, wantY), image.Pt(operations[1].x, operations[1].y))
				assert.False(t, operations[0].mirror)
				assert.True(t, operations[1].mirror)
				assert.Equal(t, onCeiling, operations[0].top)
				assert.Equal(t, onCeiling, operations[1].top)
				assert.Equal(t, byte(depth+2), operations[0].phase)
				assert.Equal(t, byte(depth+2), operations[1].phase)
			})
		}
	}
}

func TestDungeonWandererRenderPlanUsesCenterRayVisibilityAndOcclusion(t *testing.T) {
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	centerDepthTwo := references.Position{X: 4, Y: 2}
	tests := []struct {
		name         string
		position     references.Position
		floor        references.FloorNumber
		lit          bool
		subdued      bool
		occludingPos *references.Position
	}{
		{name: "depth zero", position: player.Position, floor: player.Floor, lit: true},
		{name: "side cell", position: references.Position{X: 3, Y: 2}, floor: player.Floor, lit: true},
		{name: "beyond depth three", position: references.Position{X: 4, Y: 0}, floor: player.Floor, lit: true},
		{name: "different floor", position: centerDepthTwo, floor: 1, lit: true},
		{name: "true darkness", position: centerDepthTwo, floor: player.Floor},
		{name: "subdued unlit geometry", position: centerDepthTwo, floor: player.Floor, subdued: true},
		{
			name: "opaque center structure", position: centerDepthTwo, floor: player.Floor, lit: true,
			occludingPos: &references.Position{X: 4, Y: 3},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
			snapshot.Wanderer = &game_state.DungeonWanderer{
				Monster: references.MonsterRat, Floor: test.floor, Position: test.position,
			}
			if test.occludingPos != nil {
				require.True(t, snapshot.Cells.SetCell(player.Floor, *test.occludingPos, references.DungeonCell(0xb0)))
			}

			plan := buildDungeonRenderPlan(&snapshot, player, test.lit, test.subdued, false, 0)

			assert.Empty(t, dungeonMonsterPlanOps(plan))
		})
	}

	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	snapshot.Wanderer = &game_state.DungeonWanderer{
		Monster: references.MonsterRat, Floor: player.Floor, Position: centerDepthTwo,
	}
	assert.Len(t, dungeonMonsterPlanOps(buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)), 2)
}

func TestDungeonWandererVisibilityUsesOriginalCenterCellClassBoundary(t *testing.T) {
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	for kind := references.DungeonPassage; kind <= references.DungeonLiveRoom; kind++ {
		kind := kind
		t.Run(dungeonCellKindName(kind), func(t *testing.T) {
			t.Parallel()
			snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
			between := references.Position{X: 4, Y: 3}
			monsterPosition := references.Position{X: 4, Y: 2}
			require.True(t, snapshot.Cells.SetCell(
				player.Floor,
				between,
				references.DungeonCell(byte(kind)<<4),
			))
			snapshot.Wanderer = &game_state.DungeonWanderer{
				Monster: references.MonsterRat, Floor: player.Floor, Position: monsterPosition,
			}

			plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
			monsterOperations := dungeonMonsterPlanOps(plan)
			if kind < references.DungeonClearedRoom {
				assert.Len(t, monsterOperations, 2)
			} else {
				assert.Empty(t, monsterOperations)
			}
		})
	}
}

func TestDungeonWandererContentsRemainFarToNear(t *testing.T) {
	snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})
	nearItem := references.Position{X: 4, Y: 3}
	monsterPosition := references.Position{X: 4, Y: 2}
	require.True(t, snapshot.Cells.SetCell(player.Floor, nearItem, references.DungeonCell(0x40)))
	snapshot.Wanderer = &game_state.DungeonWanderer{
		Monster: references.MonsterRat, Floor: player.Floor, Position: monsterPosition,
	}

	plan := buildDungeonRenderPlan(&snapshot, player, true, false, false, 0)
	contents := dungeonPlanOps(plan, func(operation dungeonRenderOp) bool {
		return operation.role == dungeonRenderContents
	})

	require.Len(t, contents, 4)
	assert.Equal(t, []dungeonRenderOpKind{
		dungeonRenderMonster, dungeonRenderMonster, dungeonRenderItem, dungeonRenderItem,
	}, []dungeonRenderOpKind{contents[0].kind, contents[1].kind, contents[2].kind, contents[3].kind})
	assert.Equal(t, []int{2, 2, 1, 1}, []int{contents[0].depth, contents[1].depth, contents[2].depth, contents[3].depth})
}

func TestDungeonMonsterAnimationMatchesSourceInstructionsAndTimeStop(t *testing.T) {
	plan := dungeonRenderPlan{location: references.Deceit, floor: 0}
	wanderer := game_state.DungeonWanderer{Floor: 0, Position: references.Position{X: 4, Y: 2}}

	rat, ratArchive, ok := references.DungeonWanderingMonsterDefinitionFor(references.MonsterRat)
	require.True(t, ok)
	assert.Equal(t, []int{1, 1}, monsterFramesAsSlice(plan, wanderer, ratArchive, rat.Animation, false, 0))
	assert.Equal(t, []int{1, 0}, monsterFramesAsSlice(plan, wanderer, ratArchive, rat.Animation, false, dungeonMonsterFrameMs))

	bat, batArchive, ok := references.DungeonWanderingMonsterDefinitionFor(references.MonsterBat)
	require.True(t, ok)
	assert.Equal(t, []int{1, 1}, monsterFramesAsSlice(plan, wanderer, batArchive, bat.Animation, false, 0))
	assert.Equal(t, []int{0, 0}, monsterFramesAsSlice(plan, wanderer, batArchive, bat.Animation, false, dungeonMonsterFrameMs))

	for _, monster := range []references.MonsterType{references.MonsterGhost, references.MonsterSlime} {
		definition, archive, found := references.DungeonWanderingMonsterDefinitionFor(monster)
		require.True(t, found)
		for frame := range 16 {
			left, right := dungeonMonsterHalfFrames(
				plan, wanderer, archive, definition.Animation, int64(frame*dungeonMonsterFrameMs),
			)
			if monster == references.MonsterGhost {
				assert.NotEqual(t, left, right)
			} else {
				assert.Equal(t, left, right)
			}
		}
	}

	spider, spiderArchive, ok := references.DungeonWanderingMonsterDefinitionFor(references.MonsterSpider)
	require.True(t, ok)
	seen := make(map[[2]int]struct{})
	visibleByHalf := [2]int{}
	for frame := range dungeonMonsterRandomRange {
		left, right := dungeonMonsterHalfFrames(
			plan, wanderer, spiderArchive, spider.Animation, int64(frame*dungeonMonsterFrameMs),
		)
		seen[[2]int{left, right}] = struct{}{}
		visibleByHalf[0] += left
		visibleByHalf[1] += right
	}
	assert.Greater(t, len(seen), 1, "ElapsedMs must advance deterministic independent half-frames")
	assert.Equal(t, [2]int{dungeonMonsterVisibleSamples, dungeonMonsterVisibleSamples}, visibleByHalf)

	for archive := range references.DungeonWanderingMonsterCount {
		definition, found := references.DungeonWanderingMonsterDefinitionAt(archive)
		require.True(t, found)
		var clock actorAnimationClock
		clock.update(0, true)
		seen := make(map[[2]int]bool)
		for elapsed := int64(0); elapsed <= 20_000; elapsed += 10 {
			clock.update(elapsed, true)
			left, right := dungeonMonsterHalfFrames(plan, wanderer, archive, definition.Animation, clock.phaseMs)
			normal := monsterFramesAsSlice(plan, wanderer, archive, definition.Animation, false,
				elapsed/timeStopAnimationDivisor)
			assert.Equal(t, normal, []int{left, right}, "archive=%d elapsed=%d", archive, elapsed)
			seen[[2]int{left, right}] = true
		}
		assert.Greater(t, len(seen), 1, "Time Stop must retain each pattern, not hard-freeze archive %d", archive)
	}
}

func TestDrawDungeonWandererUsesMirroredHalvesCeilingFlipAndScaling(t *testing.T) {
	red := color.RGBA{R: 0xff, A: 0xff}
	green := color.RGBA{G: 0xff, A: 0xff}
	blue := color.RGBA{B: 0xff, A: 0xff}
	yellow := color.RGBA{R: 0xff, G: 0xff, A: 0xff}
	monster := ebiten.NewImage(2, 2)
	monster.Set(0, 0, red)
	monster.Set(1, 0, green)
	monster.Set(0, 1, blue)
	monster.Set(1, 1, yellow)
	transparent := ebiten.NewImage(80, dungeonCanvasHeight)
	source := syntheticDungeonSpriteSource{panel: transparent, item: monster}
	player := dungeonTestPlayer(references.Position{X: 4, Y: 4})

	draw := func(onCeiling bool) *ebiten.Image {
		snapshot := dungeonTestSnapshot(references.DungeonFacingNorth)
		snapshot.Wanderer = &game_state.DungeonWanderer{
			Monster:   references.MonsterRat,
			Floor:     player.Floor,
			Position:  references.Position{X: 4, Y: 3},
			OnCeiling: onCeiling,
		}
		plan := buildDungeonRenderPlan(&snapshot, player, true, false, true, 0)
		canvas := ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
		drawDungeonRenderPlan(
			canvas,
			ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight),
			source,
			dungeonTestGraffitiOutput(),
			plan,
			0,
		)

		return canvas
	}

	ground := draw(false)
	assertDungeonPixel(t, ground, image.Pt(56, 72), red)
	assertDungeonPixel(t, ground, image.Pt(80, 72), green)
	assertDungeonPixel(t, ground, image.Pt(81, 73), blue)

	ceiling := draw(true)
	assertDungeonPixel(t, ceiling, image.Pt(56, 26), blue)
	assertDungeonPixel(t, ceiling, image.Pt(80, 26), yellow)

	scaled := ebiten.NewImage(dungeonCanvasWidth*2, dungeonCanvasHeight*2)
	var options ebiten.DrawImageOptions
	options.GeoM.Scale(2, 2)
	scaled.DrawImage(ground, &options)
	for y := 144; y < 146; y++ {
		for x := 112; x < 114; x++ {
			assertDungeonPixel(t, scaled, image.Pt(x, y), red)
		}
	}
}

func TestDungeonWandererRenderingUsesDetachedStateWithoutMutation(t *testing.T) {
	sourceState, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		WithRandomSeed(266).
		Build()
	floor, position, facing, lastDirection := game_state.DungeonEntrySetup(
		references.Deceit,
		references.FloorNumber(references.OVERWORLD),
	)
	_, err := sourceState.DebugSetupDungeonRoute(references.Deceit, floor, position, facing, lastDirection)
	require.NoError(t, err)
	player := sourceState.MapState.PlayerLocation
	target := dungeonRelativePosition(player.Position, facing, 1, 0)
	_, err = sourceState.TransformDungeonCell(player.Floor, target, func(references.DungeonCell) references.DungeonCell {
		return 0
	})
	require.NoError(t, err)
	saved, err := sourceState.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.NotNil(t, saved.Dungeon)
	saved.Dungeon.Wanderer = &save_snapshot.DungeonWandererSnapshot{
		Monster: int(references.MonsterRat), Floor: int(player.Floor),
		X: int(target.X), Y: int(target.Y), PreviousX: int(target.X), PreviousY: int(target.Y), OnCeiling: false,
	}
	state, err := game_state.NewGameStateFromSaveSnapshot(
		saved,
		sourceState.GameConfig,
		sourceState.GameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	state.ElapsedMs = 12_345
	beforeSave, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	beforeDungeon, active := state.DungeonStateSnapshot()
	require.True(t, active)
	require.NotNil(t, beforeDungeon.Wanderer)
	beforeRNGCalls := state.GetRNG().Calls()

	firstPlan := buildDungeonRenderPlan(&beforeDungeon, state.MapState.PlayerLocation, true, false, false, state.ElapsedMs)
	secondPlan := buildDungeonRenderPlan(&beforeDungeon, state.MapState.PlayerLocation, true, false, false, state.ElapsedMs)
	assert.Equal(t, firstPlan, secondPlan)
	require.Len(t, dungeonMonsterPlanOps(firstPlan), 2)
	monster := ebiten.NewImage(2, 2)
	monster.Fill(color.White)
	source := syntheticDungeonSpriteSource{
		panel: ebiten.NewImage(80, dungeonCanvasHeight),
		item:  monster,
	}
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

func dungeonMonsterPlanOps(plan dungeonRenderPlan) []dungeonRenderOp {
	return dungeonPlanOps(plan, func(operation dungeonRenderOp) bool {
		return operation.kind == dungeonRenderMonster
	})
}

func monsterFramesAsSlice(
	plan dungeonRenderPlan,
	wanderer game_state.DungeonWanderer,
	monster int,
	instruction byte,
	timeStopped bool,
	presentationTimeMs int64,
) []int {
	var clock actorAnimationClock
	clock.update(0, timeStopped)
	clock.update(presentationTimeMs, timeStopped)
	left, right := dungeonMonsterHalfFrames(
		plan, wanderer, monster, instruction, clock.phaseMs,
	)

	return []int{left, right}
}
