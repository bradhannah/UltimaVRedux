package main

import (
	"image"
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	combatConeCastPropagationDurationMs uint64 = 450
)

const combatConeCastHoldDurationMs int64 = 150

// While a cone is aimed its squares carry a flat wash of the spell's colour under
// the figures, and every other square of the battlefield is shaded over them.
// Whoever stays lit stands in the spell's path, friend or foe.
const (
	combatConeAimTintOpacity  = 0.35
	combatConeAimShadeOpacity = 0.6
)

// Direction arrows are solid, one source-pixel row at a time, on the centre line
// of the caster's 18-pixel frame. The chosen arrow is the larger one.
const (
	combatAimArrowChosenRows = 6
	combatAimArrowIdleRows   = 4
	combatAimArrowGapPixels  = 2
	combatAimArrowCenter     = combatSourceTileInset + combatSourceTilePixels/2
	combatAimArrowFarEdge    = combatSourceTileInset + combatSourceTilePixels
)

// combatDirectionalFootprintVisual configures reusable presentation for directional combat effects.
type combatDirectionalFootprintVisual struct {
	Tiles               [2]indexes.SpriteIndex
	TileCount           int
	Color               ucolor.EGAColor
	CastNearOpacity     float32
	CastFarOpacity      float32
	PropagationDuration uint64
	HoldDuration        int64
}

type combatConeCastEffect struct {
	spell     references.Spell
	origin    references.Position
	direction references.Direction
	positions []references.Position
	progress  float32
}

// combatConeAim is the footprint a chosen direction previews: a pure projection
// of the squares a confirmed cast would use, with the square it starts from.
type combatConeAim struct {
	spell   references.Spell
	origin  references.Position
	squares [references.XCombatMapTiles][references.YCombatMapTiles]bool
}

// combatGridLines are the battlefield's square boundaries in whole screen pixels.
// Filling between shared lines leaves neither a gap nor a doubled strip where two
// squares meet, at any window size.
type combatGridLines struct {
	x [references.XCombatMapTiles + 1]float32
	y [references.YCombatMapTiles + 1]float32
}

func combatDirectionalFootprintVisualForSpell(spell references.Spell) combatDirectionalFootprintVisual {
	//nolint:exhaustive // Only implemented cone spells require presentation configuration.
	switch spell {
	case references.InZu:
		return combatDirectionalFootprintVisual{
			Tiles:     [2]indexes.SpriteIndex{indexes.MagicField},
			TileCount: 1,
			Color:     ucolor.EGALightMagenta,
			// MagicField has a sparse silhouette, so a wide cast range is
			// needed for the spray to visibly thin as it expands.
			CastNearOpacity:     0.95,
			CastFarOpacity:      0.18,
			PropagationDuration: combatConeCastPropagationDurationMs,
			HoldDuration:        combatConeCastHoldDurationMs,
		}
	case references.InNoxHur:
		return combatDirectionalFootprintVisual{
			Tiles:               [2]indexes.SpriteIndex{indexes.PoisonField},
			TileCount:           1,
			Color:               ucolor.EGALightGreen,
			CastNearOpacity:     0.95,
			CastFarOpacity:      0.24,
			PropagationDuration: combatConeCastPropagationDurationMs,
			HoldDuration:        combatConeCastHoldDurationMs,
		}
	case references.InFlamHur:
		return combatDirectionalFootprintVisual{
			Tiles:               [2]indexes.SpriteIndex{indexes.FireField},
			TileCount:           1,
			Color:               ucolor.EGALightRed,
			CastNearOpacity:     0.95,
			CastFarOpacity:      0.25,
			PropagationDuration: combatConeCastPropagationDurationMs,
			HoldDuration:        combatConeCastHoldDurationMs,
		}
	case references.InVasGravCorp:
		return combatDirectionalFootprintVisual{
			Tiles:               [2]indexes.SpriteIndex{indexes.MagicField},
			TileCount:           1,
			Color:               ucolor.EGALightBlue,
			CastNearOpacity:     0.95,
			CastFarOpacity:      0.25,
			PropagationDuration: combatConeCastPropagationDurationMs,
			HoldDuration:        combatConeCastHoldDurationMs,
		}
	default:
		panic("combat cone visual configuration missing")
	}
}

// combatConeAim reports the aimed footprint. There is none before a direction is
// chosen, nor once the confirmed cast is being shown.
func (g *GameScene) combatConeAim() (combatConeAim, bool) {
	frame := &g.combatRenderFrame
	if !frame.active {
		return g.computeCombatConeAim()
	}
	if !frame.coneAimReady {
		frame.coneAim, frame.coneAimShown = g.computeCombatConeAim()
		frame.coneAimReady = true
	}

	return frame.coneAim, frame.coneAimShown
}

func (g *GameScene) computeCombatConeAim() (combatConeAim, bool) {
	targeting := g.gameState.CombatScene.ConeTargeting
	if g.combatConeCast != nil || targeting == nil || targeting.Direction == references.NoneDirection {
		return combatConeAim{}, false
	}
	aim := combatConeAim{spell: targeting.Request.Spell, origin: targeting.Origin}
	for _, position := range g.gameState.CombatConePreview() {
		aim.squares[position.X][position.Y] = true
	}

	return aim, true
}

// drawCombatConeOverlay draws what lies under the figures: the aimed footprint's
// tint, or the spreading spray once the cast is confirmed.
func (g *GameScene) drawCombatConeOverlay(screen *ebiten.Image) {
	if g.combatConeCast != nil {
		g.drawCombatConeCast(screen, g.combatConeCast)

		return
	}
	aim, shown := g.combatConeAim()
	if !shown {
		return
	}
	tint := combatSquareWash(combatDirectionalFootprintVisualForSpell(aim.spell).Color, combatConeAimTintOpacity)
	g.fillCombatSquares(screen, tint, func(position references.Position) bool {
		return aim.squares[position.X][position.Y]
	})
}

// drawCombatConeShade darkens every square outside the aimed footprint, figures
// and their health marks included. The caster's own square stays lit.
func (g *GameScene) drawCombatConeShade(screen *ebiten.Image) {
	aim, shown := g.combatConeAim()
	if !shown {
		return
	}
	shade := combatSquareWash(ucolor.EGABlack, combatConeAimShadeOpacity)
	g.fillCombatSquares(screen, shade, func(position references.Position) bool {
		return position != aim.origin && !aim.squares[position.X][position.Y]
	})
}

func (g *GameScene) drawCombatConeCast(screen *ebiten.Image, cast *combatConeCastEffect) {
	if len(cast.positions) == 0 {
		return
	}
	visual := combatDirectionalFootprintVisualForSpell(cast.spell)
	maxDistance := combatDirectionalFootprintMaxDistance(cast.positions, cast.origin, cast.direction)
	for _, position := range cast.positions {
		distance := combatDirectionalFootprintDistance(cast.origin, position, cast.direction)
		if float32(distance) > cast.progress*float32(maxDistance) {
			continue
		}
		drawOptions := g.combatSquareMarkerDrawOptions(position)
		drawOptions.ColorScale.ScaleWithColor(visual.Color.ToRGBA())
		drawOptions.ColorScale.ScaleAlpha(combatDirectionalFootprintOpacity(visual, distance, maxDistance))
		screen.DrawImage(g.spriteSheet.GetSprite(combatDirectionalFootprintTile(visual, position)), drawOptions)
	}
}

// combatSquareWash is a translucent flat colour for whole-square fills.
func combatSquareWash(base ucolor.EGAColor, opacity float64) color.NRGBA {
	return color.NRGBA{R: base.R, G: base.G, B: base.B, A: uint8(math.Round(opacity * ucolor.AlphaOpaque))}
}

func (g *GameScene) combatGridLines() combatGridLines {
	lines := combatGridLines{}
	for index := range lines.x {
		corner := references.Position{X: references.Coordinate(index)}
		x, _ := g.combatSquareMarkerDrawOptions(corner).GeoM.Apply(combatSourceTileInset, combatSourceTileInset)
		lines.x[index] = float32(math.Round(x))
	}
	for index := range lines.y {
		corner := references.Position{Y: references.Coordinate(index)}
		_, y := g.combatSquareMarkerDrawOptions(corner).GeoM.Apply(combatSourceTileInset, combatSourceTileInset)
		lines.y[index] = float32(math.Round(y))
	}

	return lines
}

// fillCombatSquares washes the battlefield squares include accepts, each row's
// run of neighbouring squares as one rectangle.
func (g *GameScene) fillCombatSquares(screen *ebiten.Image, wash color.Color, include func(references.Position) bool) {
	lines := g.combatGridLines()
	for y := range references.YCombatMapTiles {
		runStart := -1
		for x := 0; x <= int(references.XCombatMapTiles); x++ {
			included := x < int(references.XCombatMapTiles) &&
				include(references.Position{X: references.Coordinate(x), Y: y})
			switch {
			case included && runStart < 0:
				runStart = x
			case !included && runStart >= 0:
				vector.FillRect(screen, lines.x[runStart], lines.y[y],
					lines.x[x]-lines.x[runStart], lines.y[y+1]-lines.y[y], wash, false)
				runStart = -1
			}
		}
	}
}

func combatDirectionalFootprintDistance(
	origin, position references.Position,
	direction references.Direction,
) int {
	//nolint:exhaustive // Only cardinal directions are admitted by combat cone targeting.
	switch direction {
	case references.Up:
		return int(origin.Y - position.Y)
	case references.Down:
		return int(position.Y - origin.Y)
	case references.Left:
		return int(origin.X - position.X)
	case references.Right:
		return int(position.X - origin.X)
	default:
		panic("directional footprint requires a cardinal direction")
	}
}

func combatDirectionalFootprintMaxDistance(
	positions []references.Position,
	origin references.Position,
	direction references.Direction,
) int {
	maxDistance := 1
	for _, position := range positions {
		maxDistance = max(maxDistance, combatDirectionalFootprintDistance(origin, position, direction))
	}

	return maxDistance
}

// combatDirectionalFootprintOpacity thins the cast's spray as it travels.
func combatDirectionalFootprintOpacity(
	visual combatDirectionalFootprintVisual,
	distance, maxDistance int,
) float32 {
	progress := float32(distance) / float32(max(maxDistance, 1))

	return visual.CastNearOpacity + (visual.CastFarOpacity-visual.CastNearOpacity)*progress
}

func combatDirectionalFootprintTile(
	visual combatDirectionalFootprintVisual,
	position references.Position,
) indexes.SpriteIndex {
	if visual.TileCount < 1 || visual.TileCount > len(visual.Tiles) {
		panic("invalid directional footprint tile configuration")
	}
	index := (int(position.X)*31 + int(position.Y)) % visual.TileCount

	return visual.Tiles[index]
}

func (g *GameScene) drawCombatDirectionIndicators(screen *ebiten.Image) {
	combat := g.gameState.CombatScene
	if combat == nil {
		return
	}
	var origin references.Position
	var casterIdx int
	var selectedDirection references.Direction
	switch {
	case combat.ConeTargeting != nil:
		origin = combat.ConeTargeting.Origin
		casterIdx = combat.ConeTargeting.Request.CasterIdx
		selectedDirection = combat.ConeTargeting.Direction
	case combat.AdjacentSpellTargeting != nil:
		if game_state.IsCombatDoorSpell(combat.AdjacentSpellTargeting.Request.Spell) {
			return
		}
		origin = combat.AdjacentSpellTargeting.Origin
		casterIdx = combat.AdjacentSpellTargeting.Request.CasterIdx
		selectedDirection = combat.AdjacentSpellTargeting.Direction
	default:
		return
	}
	if !g.gameState.CombatTerrainAllowsObjectDisplay(origin) {
		return
	}
	drawOptions := g.combatPartyDrawOptions(origin, casterIdx)
	for _, direction := range []references.Direction{
		references.Up, references.Down, references.Left, references.Right,
	} {
		rows, face := combatAimArrowIdleRows, ucolor.CombatAimFrameFace
		if direction == selectedDirection {
			rows, face = combatAimArrowChosenRows, ucolor.CombatAimArrowChosen
		}
		drawCombatAimArrow(screen, drawOptions, direction, rows, face)
	}
}

func (g *GameScene) drawCombatAdjacentSpellTargetPreview(screen *ebiten.Image) {
	if combat := g.gameState.CombatScene; combat != nil && combat.AdjacentSpellTargeting != nil &&
		game_state.IsCombatDoorSpell(combat.AdjacentSpellTargeting.Request.Spell) {
		return
	}
	target, valid, active := g.gameState.CombatAdjacentSpellPreview()
	if !active || g.gameState.CombatScene.AdjacentSpellTargeting.Direction == references.NoneDirection {
		return
	}
	color := ucolor.EGALightRed
	if !g.gameState.CombatTerrainAllowsObjectDisplay(target) {
		color = ucolor.EGALightGray
	} else if valid {
		color = ucolor.EGALightCyan
	}
	drawCombatSelectionFrame(
		screen,
		g.combatSquareMarkerDrawOptions(target),
		g.getElapsedMs(),
		color,
	)
}

// combatDoorSpellCursor supplies a square to the final aim layer. It shares the
// full-size white attack reticle; eligibility never changes its geometry/color.
func (g *GameScene) combatDoorSpellCursor() (references.Position, bool) {
	combat := g.gameState.CombatScene
	if combat == nil || combat.AdjacentSpellTargeting == nil ||
		!game_state.IsCombatDoorSpell(combat.AdjacentSpellTargeting.Request.Spell) {
		return references.Position{}, false
	}
	target, _, active := g.gameState.CombatAdjacentSpellPreview()

	return target, active
}

// combatAimArrowRow is one row of an arrow pointing away from the caster's
// square, in the source pixels of the caster's frame. Row zero is the base,
// nearest the caster; each row beyond it lies one pixel farther out and is two
// pixels narrower, down to a two-pixel tip.
func combatAimArrowRow(direction references.Direction, rows, row int) image.Rectangle {
	half := rows - row
	low, high := combatAimArrowCenter-half, combatAimArrowCenter+half
	before := combatSourceTileInset - combatAimArrowGapPixels - row
	after := combatAimArrowFarEdge + combatAimArrowGapPixels + row
	switch direction {
	case references.Up:
		return image.Rect(low, before-1, high, before)
	case references.Down:
		return image.Rect(low, after, high, after+1)
	case references.Left:
		return image.Rect(before-1, low, before, high)
	case references.Right:
		return image.Rect(after, low, after+1, high)
	case references.NoneDirection:
	}
	panic("combat aim arrow requires a cardinal direction")
}

// drawCombatAimArrow fills an arrow's rows over a one-pixel black edge.
func drawCombatAimArrow(
	screen *ebiten.Image,
	options *ebiten.DrawImageOptions,
	direction references.Direction,
	rows int,
	face ucolor.EGAColor,
) {
	for row := range rows {
		edge := combatAimArrowRow(direction, rows, row).Inset(-combatMarkerBorderPixels)
		drawCombatMarkerRect(screen, options, edge, ucolor.CombatAimFrameBorder)
	}
	for row := range rows {
		drawCombatMarkerRect(screen, options, combatAimArrowRow(direction, rows, row), face)
	}
}

func (g *GameScene) startCombatConeSpellEffect(result game_state.CombatConeTargetResult) {
	timing := g.captureCombatTiming()
	visual := combatDirectionalFootprintVisualForSpell(result.Request.Spell)
	g.gameState.EventQueueManager.Workflow("combat_cone_spell_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {
			g.combatConeCast = &combatConeCastEffect{
				spell: result.Request.Spell, origin: result.Origin, direction: result.Request.Direction,
				positions: result.Positions, progress: 0,
			}
			g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundWhiteNoiseDamage)
		}).Animate(timing.duration(visual.PropagationDuration), func(progress float32) {
		g.combatConeCast.progress = progress
	}).Wait(timing.durationMs(visual.HoldDuration)).Then(g.combatTimingStep(timing, func() {
		g.presentCombatAreaSpellSummary(g.gameState.ResolveCommittedCombatCone(result))
	})).Finally(func() {
		g.combatConeCast = nil
		g.gameState.FinishCombatAction()
	}).Start()
}
