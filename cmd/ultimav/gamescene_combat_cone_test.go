package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestCombatAimArrowRowsNarrowAwayFromTheCaster(t *testing.T) {
	for _, test := range []struct {
		direction references.Direction
		rows      int
		base, tip image.Rectangle
	}{
		{references.Up, combatAimArrowChosenRows, image.Rect(3, -2, 15, -1), image.Rect(8, -7, 10, -6)},
		{references.Down, combatAimArrowChosenRows, image.Rect(3, 19, 15, 20), image.Rect(8, 24, 10, 25)},
		{references.Left, combatAimArrowChosenRows, image.Rect(-2, 3, -1, 15), image.Rect(-7, 8, -6, 10)},
		{references.Right, combatAimArrowChosenRows, image.Rect(19, 3, 20, 15), image.Rect(24, 8, 25, 10)},
		{references.Up, combatAimArrowIdleRows, image.Rect(5, -2, 13, -1), image.Rect(8, -5, 10, -4)},
		{references.Right, combatAimArrowIdleRows, image.Rect(19, 5, 20, 13), image.Rect(22, 8, 23, 10)},
	} {
		assert.Equal(t, test.base, combatAimArrowRow(test.direction, test.rows, 0), "%v base", test.direction)
		assert.Equal(t, test.tip, combatAimArrowRow(test.direction, test.rows, test.rows-1), "%v tip", test.direction)
	}
	square := image.Rect(combatSourceTileInset, combatSourceTileInset, combatAimArrowFarEdge, combatAimArrowFarEdge)
	for _, direction := range []references.Direction{references.Up, references.Down, references.Left, references.Right} {
		edge := combatAimArrowRow(direction, combatAimArrowChosenRows, 0).Inset(-combatMarkerBorderPixels)
		assert.False(t, edge.Overlaps(square), "%v arrow and its black edge stay off the caster's square", direction)
	}
	assert.Panics(t, func() { combatAimArrowRow(references.NoneDirection, combatAimArrowIdleRows, 0) })
}

func TestCombatDirectionalFootprintOpacityFadesWithDistance(t *testing.T) {
	visual := combatDirectionalFootprintVisualForSpell(references.InZu)

	assert.GreaterOrEqual(t,
		combatDirectionalFootprintOpacity(visual, 1, 10)-combatDirectionalFootprintOpacity(visual, 10, 10),
		float32(0.65),
		"cast spray must retain a clearly visible near-to-far opacity drop",
	)
}

func TestCombatDirectionalFootprintTileUsesDeterministicVariants(t *testing.T) {
	visual := combatDirectionalFootprintVisual{
		Tiles: [2]indexes.SpriteIndex{indexes.MagicField, indexes.FireField}, TileCount: 2,
	}
	position := references.Position{X: 3, Y: 4}

	assert.Equal(t,
		combatDirectionalFootprintTile(visual, position),
		combatDirectionalFootprintTile(visual, position),
	)
}

func TestCombatDirectionalFootprintDistanceUsesCastDirection(t *testing.T) {
	origin := references.Position{X: 5, Y: 5}

	assert.Equal(t, 3, combatDirectionalFootprintDistance(origin, references.Position{X: 5, Y: 8}, references.Down))
	assert.Equal(t, 3, combatDirectionalFootprintDistance(origin, references.Position{X: 2, Y: 5}, references.Left))
}

// newCombatConeAimScene readies a caster at the centre of an open brick floor
// with In Zu being aimed, before any direction is chosen.
func newCombatConeAimScene(t *testing.T, size image.Point) (*GameScene, references.Position) {
	t.Helper()
	setSquareVisualTestDisplay(t, size.X, size.Y)
	scene, _, _ := newAreaSpellOutcomeScene(t, references.InZu, references.MonsterOrc)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.mapImage = ebiten.NewImage(304, 208)
	t.Cleanup(scene.mapImage.Deallocate)
	state := scene.gameState
	require.True(t, state.BeginCombatConeTargeting(game_state.CastRequest{
		Spell: references.InZu, CasterIdx: state.CombatScene.GetActiveCombatantIndex(),
	}).Valid)

	return scene, state.CombatScene.ConeTargeting.Origin
}

func combatBoardRaster(t *testing.T, board image.Rectangle) *ebiten.Image {
	t.Helper()
	screen := ebiten.NewImageWithOptions(board, &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(screen.Deallocate)

	return screen
}

// Every pixel of the battlefield is accounted for: tinted in the spell's path,
// shaded outside it, untouched on the caster's own square. An exact alpha per
// pixel proves squares neither gap nor overlap at each supported window size.
func TestCombatConeAimTintsThePathAndShadesTheRest(t *testing.T) {
	for _, size := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {2560, 1440}} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			scene, origin := newCombatConeAimScene(t, size)
			state := scene.gameState
			board := scene.combatBattlefieldScreenRect()
			under, over := combatBoardRaster(t, board), combatBoardRaster(t, board)
			scene.drawCombatConeOverlay(under)
			scene.drawCombatConeShade(over)
			assert.Zero(t, opaquePixelCount(under), "nothing is tinted before a direction is chosen")
			assert.Zero(t, opaquePixelCount(over), "nothing is shaded before a direction is chosen")

			state.SetCombatConeDirection(references.Up)
			path := map[references.Position]bool{}
			for _, position := range state.CombatConePreview() {
				path[position] = true
			}
			require.True(t, path[references.Position{X: origin.X, Y: origin.Y - 1}])
			require.False(t, path[references.Position{X: origin.X, Y: origin.Y + 1}])
			scene.drawCombatConeOverlay(under)
			scene.drawCombatConeShade(over)

			lines := scene.combatGridLines()
			last := len(lines.x) - 1
			require.Equal(t, board, image.Rect(int(lines.x[0]), int(lines.y[0]), int(lines.x[last]), int(lines.y[last])),
				"the grid's outer lines are the battlefield's own edges")
			tint := combatSquareWash(ucolor.EGALightMagenta, combatConeAimTintOpacity).A
			shade := combatSquareWash(ucolor.EGABlack, combatConeAimShadeOpacity).A
			tinted, shaded := signaturePixels(under), signaturePixels(over)
			for row := range references.YCombatMapTiles {
				for column := range references.XCombatMapTiles {
					square := references.Position{X: column, Y: row}
					wantTint, wantShade := byte(0), byte(0)
					switch {
					case path[square]:
						wantTint = tint
					case square != origin:
						wantShade = shade
					}
					for y := int(lines.y[row]); y < int(lines.y[row+1]); y++ {
						for x := int(lines.x[column]); x < int(lines.x[column+1]); x++ {
							alpha := ((y-board.Min.Y)*board.Dx()+(x-board.Min.X))*4 + 3
							if tinted[alpha] != wantTint || shaded[alpha] != wantShade {
								t.Fatalf("square %v pixel (%d,%d): tint %d shade %d, want %d and %d",
									square, x, y, tinted[alpha], shaded[alpha], wantTint, wantShade)
							}
						}
					}
				}
			}

			scene.combatConeCast = &combatConeCastEffect{spell: references.InZu, origin: origin,
				direction: references.Up, positions: state.CombatConePreview(), progress: 1}
			over.Clear()
			scene.drawCombatConeShade(over)
			assert.Zero(t, opaquePixelCount(over), "the confirmed cast is shown without the aim's shade")
		})
	}
}

func TestCombatDirectionArrowsMarkTheChosenWayWithoutPulsing(t *testing.T) {
	scene, _ := newCombatConeAimScene(t, image.Pt(1280, 720))
	state := scene.gameState
	screen := combatBoardRaster(t, scene.combatBattlefieldScreenRect())
	scene.drawCombatDirectionIndicators(screen)
	idle := countExactColor(screen, ucolor.CombatAimFrameFace)
	require.Positive(t, idle)
	assert.Zero(t, countExactColor(screen, ucolor.CombatAimArrowChosen), "no arrow is chosen yet")

	state.SetCombatConeDirection(references.Left)
	screen.Clear()
	scene.drawCombatDirectionIndicators(screen)
	chosen, others := countExactColor(screen, ucolor.CombatAimArrowChosen), countExactColor(screen, ucolor.CombatAimFrameFace)
	assert.Less(t, others, idle, "the chosen arrow leaves three idle ones")
	assert.Greater(t, chosen, others/3, "the chosen arrow is larger than an idle one")

	before := signaturePixels(screen)
	state.ElapsedMs += 450
	screen.Clear()
	scene.drawCombatDirectionIndicators(screen)
	assert.Equal(t, before, signaturePixels(screen), "arrows hold still")
}
