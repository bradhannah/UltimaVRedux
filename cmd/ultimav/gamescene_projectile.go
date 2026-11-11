package main

import (
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// Projectile presentation speeds follow ref:COMBAT-0012.
const (
	// ProjectilePixelsPerMs is the ordinary speed outside towns.
	ProjectilePixelsPerMs     = 0.325
	projectileRockPixelsPerMs = 0.2  // incr=8 per 40ms outside towns.
	projectileTownPixelsPerMs = 0.15 // incr=6 per 40ms for every town missile.

	// REDUX DEVIATION: COMBAT-0012: original missiles step a pixel path with a 40ms delay and no
	// duration floor. Smooth Euclidean interpolation retains an authored 100ms
	// minimum before the combat action's timing policy is applied.
	ProjectileMinDurationMs = 100
)

// ActiveProjectile holds current projectile state for rendering.
// Position updated via Animate() callback - no separate Update() loop needed.
// Uses tile-relative coordinates (0-10 for visible screen tiles) for proper scaling.
type ActiveProjectile struct {
	// Start/end positions (tile-relative: 0-10 for visible screen tiles)
	fromTileX, fromTileY float64
	toTileX, toTileY     float64

	// Current interpolated tile position
	currentTileX, currentTileY float64

	// Projectile type (determines sprite)
	projectileType references.MissileType

	// Is this projectile currently animating?
	active bool
}

type projectileLineSegment struct {
	FromX float64
	FromY float64
	ToX   float64
	ToY   float64
}

type physicalProjectileGeometry struct {
	Segments [5]projectileLineSegment
	Count    int
}

// CalculateProjectileDuration returns duration in ms for constant-speed travel.
// Longer distances take proportionally longer (authentic U5 behavior).
func CalculateProjectileDuration(
	fromX, fromY, toX, toY float64,
	missile references.MissileType,
	location references.Location,
) uint64 {
	dx := toX - fromX
	dy := toY - fromY
	distance := math.Sqrt(dx*dx + dy*dy)
	speed := ProjectilePixelsPerMs
	if location >= references.Moonglow && location <= references.SerpentsHold {
		speed = projectileTownPixelsPerMs
	} else if missile == references.MissileRock {
		speed = projectileRockPixelsPerMs
	}
	durationMs := uint64(distance / speed)

	if durationMs < ProjectileMinDurationMs {
		durationMs = ProjectileMinDurationMs
	}

	return durationMs
}

// UpdatePosition interpolates tile position based on progress (0.0 to 1.0).
// Called from Animate() callback each frame.
func (p *ActiveProjectile) UpdatePosition(progress float64) {
	if !p.active {
		return
	}

	p.currentTileX = p.fromTileX + (p.toTileX-p.fromTileX)*progress
	p.currentTileY = p.fromTileY + (p.toTileY-p.fromTileY)*progress
}

func physicalProjectileSourceGeometry(
	projectileType references.MissileType,
	deltaX, deltaY float64,
) (physicalProjectileGeometry, bool) {
	geometry := physicalProjectileGeometry{}
	length := math.Hypot(deltaX, deltaY)
	if length == 0 {
		return geometry, false
	}
	directionX := deltaX / length
	directionY := deltaY / length
	perpendicularX := -directionY
	perpendicularY := directionX
	addSegment := func(fromAlong, fromAcross, toAlong, toAcross float64) {
		geometry.Segments[geometry.Count] = projectileLineSegment{
			FromX: fromAlong*directionX + fromAcross*perpendicularX,
			FromY: fromAlong*directionY + fromAcross*perpendicularY,
			ToX:   toAlong*directionX + toAcross*perpendicularX,
			ToY:   toAlong*directionY + toAcross*perpendicularY,
		}
		geometry.Count++
	}

	switch projectileType {
	case references.MissileArrow:
		addSegment(-5, 0, 5, 0)
		addSegment(5, 0, 2, 2)
		addSegment(5, 0, 2, -2)
	case references.MissileAxe:
		addSegment(-5, 0, 3, 0)
		addSegment(2, -3, 4, -2)
		addSegment(4, -2, 4, 2)
		addSegment(4, 2, 2, 3)
	case references.MissileCannonBall:
		addSegment(-1, -2, 1, -2)
		addSegment(-2, -1, 2, -1)
		addSegment(-2, 0, 2, 0)
		addSegment(-2, 1, 2, 1)
		addSegment(-1, 2, 1, 2)
	case references.MissileRock:
		addSegment(0, -3, 3, 0)
		addSegment(3, 0, 0, 3)
		addSegment(0, 3, -3, 0)
		addSegment(-3, 0, 0, -3)
	case references.MissileNone, references.MissileViolet,
		references.MissileRed, references.MissileBlue, references.MissileGreen:
		return geometry, false
	default:
		return geometry, false
	}

	return geometry, true
}

// startProjectile begins a projectile animation, returns duration for Animate().
// Uses tile-relative coordinates (0-10 for visible screen tiles) for proper scaling.
func (g *GameScene) startProjectile(
	fromPos, toPos references.Position,
	projectileType references.MissileType,
) uint64 {
	// Get player position (center of view)
	playerPos := g.gameState.MapState.PlayerLocation.Position

	wrapped := g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType
	fromTileX, fromTileY, toTileX, toTileY := projectileTileCoordinates(fromPos, toPos, playerPos, wrapped)
	if g.gameState.IsInCombat() {
		fromTileX = float64(combatViewportStartColumn) + float64(fromPos.X)
		fromTileY = float64(combatViewportStartRow) + float64(fromPos.Y)
		toTileX = float64(combatViewportStartColumn) + float64(toPos.X)
		toTileY = float64(combatViewportStartRow) + float64(toPos.Y)
	}

	// Create projectile with tile-relative coordinates
	g.activeProjectile = &ActiveProjectile{
		fromTileX:      fromTileX,
		fromTileY:      fromTileY,
		toTileX:        toTileX,
		toTileY:        toTileY,
		currentTileX:   fromTileX,
		currentTileY:   fromTileY,
		projectileType: projectileType,
		active:         true,
	}

	return CalculateProjectileDuration(
		fromTileX*float64(sprites.TileSize),
		fromTileY*float64(sprites.TileSize),
		toTileX*float64(sprites.TileSize),
		toTileY*float64(sprites.TileSize),
		projectileType,
		g.gameState.MapState.PlayerLocation.Location,
	)
}

func projectileTileCoordinates(
	from, to, player references.Position,
	wrapped bool,
) (float64, float64, float64, float64) {
	fromDeltaX := int(from.X - player.X)
	fromDeltaY := int(from.Y - player.Y)
	travelDeltaX := int(to.X - from.X)
	travelDeltaY := int(to.Y - from.Y)
	if wrapped {
		fromDeltaX = shortestWrappedProjectileDelta(player.X, from.X, references.XLargeMapTiles)
		fromDeltaY = shortestWrappedProjectileDelta(player.Y, from.Y, references.YLargeMapTiles)
		travelDeltaX = shortestWrappedProjectileDelta(from.X, to.X, references.XLargeMapTiles)
		travelDeltaY = shortestWrappedProjectileDelta(from.Y, to.Y, references.YLargeMapTiles)
	}
	fromX := float64(int(xCenter) + fromDeltaX)
	fromY := float64(int(yCenter) + fromDeltaY)
	toX := fromX + float64(travelDeltaX)
	toY := fromY + float64(travelDeltaY)

	return fromX, fromY, toX, toY
}

//nolint:unparam // Axis dimensions are currently equal but remain separate map contracts.
func shortestWrappedProjectileDelta(from, to, mapSize references.Coordinate) int {
	delta := int(to - from)
	size := int(mapSize)
	halfSize := size / 2
	if delta > halfSize {
		delta -= size
	} else if delta < -halfSize {
		delta += size
	}

	return delta
}

func (g *GameScene) combatProjectileAnimationSequenceWithTiming(
	timing combatPresentationTiming,
	launch game_state.CombatProjectileLaunch,
	onImpact func(),
) event_queue.Sequence {
	if onImpact == nil {
		panic("projectile impact callback is required")
	}

	return func(sequence *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		// Ludicrous skips the flight; the impact resolves at once.
		if !launch.Animate || !timing.animatesMotion() {
			return sequence.Then(g.combatTimingStep(timing, onImpact))
		}
		var duration uint64

		return sequence.Then(func() {
			// StartProjectile returns the authored flight including its 100ms floor.
			// Shared world cannon/pirate callers keep that unscaled callback contract.
			duration = timing.duration(g.gameState.SystemCallbacks.Visual.StartProjectile(launch.From, launch.Impact, launch.Missile))
		}).ThenSequence(func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			return q.Animate(duration, g.gameState.SystemCallbacks.Visual.UpdateProjectileProgress)
		}).Then(g.gameState.SystemCallbacks.Visual.ClearProjectile).
			Then(g.combatTimingStep(timing, onImpact))
	}
}

// updateProjectileProgress called from Animate() callback to update position.
func (g *GameScene) updateProjectileProgress(progress float32) {
	if g.activeProjectile != nil {
		g.activeProjectile.UpdatePosition(float64(progress))
	}
}

// clearProjectile removes the active projectile.
func (g *GameScene) clearProjectile() {
	if g.activeProjectile != nil {
		g.activeProjectile.active = false
		g.activeProjectile = nil
	}
}

// drawActiveProjectile renders the current projectile if active.
// Uses the same overlay rendering pattern as renderMapUnitSprite() for proper scaling.
// Called from the main draw loop after other effects.
func (g *GameScene) drawActiveProjectile(screen *ebiten.Image) {
	if g.activeProjectile == nil || !g.activeProjectile.active {
		return
	}

	// Convert tile-relative position to unscaled screen coordinates
	// Same pattern as renderMapUnitSprite() in gamescene_draw_map.go
	baseTileX := int(g.activeProjectile.currentTileX)
	baseTileY := int(g.activeProjectile.currentTileY)
	screenX, screenY := sprites.ConvertPosition16to18(baseTileX, baseTileY)

	// Add fractional offset for smooth interpolation between tiles
	fracX := g.activeProjectile.currentTileX - float64(baseTileX)
	fracY := g.activeProjectile.currentTileY - float64(baseTileY)
	screenX += fracX * sprites.TileSize
	screenY += fracY * sprites.TileSize
	if g.gameState.IsInCombat() {
		screenX += combatLeftGutterSourcePixels
	}
	mapOp := sprites.GetDrawOptionsFromPercentsForWholeScreen(g.mapImage, g.mapScreenPlacement())
	if g.terrainDither.enabled {
		mapOp = g.worldDrawOptions()
	} else if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
		mapOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	}

	// Center sprite on tile (accounting for 16x16 or 18x18 sprite sizes)
	halfTile := float64(sprites.TileSize) / 2.0
	if frame := &g.terrainDither; frame.enabled {
		// Admission follows the drawn center, not the interpolation's starting cell.
		x := int(math.Floor((screenX+halfTile)/sprites.TileSize)) + frame.margin
		y := int(math.Floor((screenY+halfTile)/sprites.TileSize)) + frame.margin
		if x < 0 || y < 0 || x >= frame.width || y >= frame.height || !frame.cells[y*frame.width+x].visible {
			return
		}
	}
	if geometry, ok := physicalProjectileSourceGeometry(
		g.activeProjectile.projectileType,
		g.activeProjectile.toTileX-g.activeProjectile.fromTileX,
		g.activeProjectile.toTileY-g.activeProjectile.fromTileY,
	); ok {
		drawPhysicalProjectile(screen, screenX+halfTile, screenY+halfTile, mapOp.GeoM, geometry)

		return
	}

	// Magic projectiles use tinted tile sprites.
	sprite := g.spriteSheet.GetSprite(indexes.MagicField)
	if sprite == nil {
		return
	}
	screenX += halfTile - float64(sprite.Bounds().Dx())/2.0
	screenY += halfTile - float64(sprite.Bounds().Dy())/2.0

	// Setup draw options with unscaled position
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Translate(screenX, screenY)
	red, green, blue, alpha := projectileColorMultipliers(g.activeProjectile.projectileType)
	op.ColorScale.Scale(red, green, blue, alpha)

	// Concatenate transforms: position first, then scale/translate to screen
	op.GeoM.Concat(mapOp.GeoM)

	screen.DrawImage(sprite, op)
}

func drawPhysicalProjectile(
	screen *ebiten.Image,
	centerX, centerY float64,
	transform ebiten.GeoM,
	geometry physicalProjectileGeometry,
) {
	originX, originY := transform.Apply(centerX, centerY)
	unitX, unitY := transform.Apply(centerX+1, centerY)
	strokeWidth := float32(max(math.Hypot(unitX-originX, unitY-originY), 1))
	for _, segment := range geometry.Segments[:geometry.Count] {
		fromX, fromY := transform.Apply(centerX+segment.FromX, centerY+segment.FromY)
		toX, toY := transform.Apply(centerX+segment.ToX, centerY+segment.ToY)
		vector.StrokeLine(
			screen,
			float32(fromX),
			float32(fromY),
			float32(toX),
			float32(toY),
			strokeWidth,
			ucolor.EGAWhite,
			false,
		)
	}
}

//nolint:unparam // Alpha remains part of the color-scale contract for future projectile effects.
func projectileColorMultipliers(projectileType references.MissileType) (float32, float32, float32, float32) {
	switch projectileType {
	case references.MissileRed:
		return 1, 0, 0, 1
	case references.MissileGreen:
		return 0, 1, 0, 1
	case references.MissileBlue:
		return 0, 0, 1, 1
	case references.MissileViolet:
		return 1, 0, 1, 1
	case references.MissileNone, references.MissileArrow, references.MissileAxe,
		references.MissileRock, references.MissileCannonBall:
		return 1, 1, 1, 1
	default:
		return 1, 1, 1, 1
	}
}
