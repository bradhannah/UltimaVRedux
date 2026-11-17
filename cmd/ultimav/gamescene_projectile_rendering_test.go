package main

import (
	"bytes"
	"fmt"
	"image"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestEveryRangedEnemyPresentsRenderedScheduledFlight(t *testing.T) {
	enemies := references.NewAllEnemyReferences(references.NewTileReferences())
	verified := 0
	for _, reference := range *enemies {
		if reference.AttackRange <= 1 {
			continue
		}
		verified++
		t.Run(reference.MonsterType.Name(), func(t *testing.T) {
			scene, _, step := newScheduledProjectileTestScene(t, reference.MonsterType, 2)
			state := scene.gameState
			state.SetRandomSeed(projectileReferenceSeed(t, reference))
			state.ElapsedMs = 0
			scene.startScheduledMonsterTurn(step)
			state.EventQueueManager.Update()
			state.ElapsedMs = game_state.MonsterMoveDelayMs
			state.EventQueueManager.Update()
			require.NotNil(t, scene.activeProjectile, "normal scheduled enemy must reach its projectile callback")
			flight := scene.activeProjectile
			require.Equal(t, reference.LargeMapMissile, flight.projectileType)
			require.Equal(t, flight.fromTileX, flight.currentTileX)
			rngBefore := state.GetRNG().Calls()
			state.ElapsedMs += 50
			state.EventQueueManager.Update()
			require.Same(t, flight, scene.activeProjectile)
			require.Greater(t, flight.currentTileX, flight.fromTileX)
			require.Less(t, flight.currentTileX, flight.toTileX)
			assert.Equal(t, flight.fromTileY, flight.currentTileY)
			assert.Equal(t, rngBefore, state.GetRNG().Calls())
		})
	}
	require.Equal(t, 19, verified, "inventory must keep covering every production ranged reference")
}

func TestProjectileFamiliesContrastAndScaling(t *testing.T) {
	// Cover each background and all scales without rasterizing their Cartesian
	// product. Missile identity, interpolation and endpoints retain full CPU coverage.
	for _, sample := range []struct {
		size    image.Point
		terrain indexes.SpriteIndex
	}{
		{image.Pt(960, 540), indexes.Grass},
		{image.Pt(1280, 720), indexes.BrickFloor},
		{image.Pt(1920, 1080), indexes.Water2},
		{image.Pt(1280, 720), indexes.BlackSquare},
	} {
		size, terrain := sample.size, sample.terrain
		t.Run(fmt.Sprintf("%dx%d/terrain%d", size.X, size.Y, terrain), func(t *testing.T) {
			scene, _, _ := newScheduledProjectileTestScene(t, references.MonsterEttin, 2)
			screen := newProjectileRenderTarget(t, scene, size.X, size.Y, terrain)
			for _, missile := range enemyProjectileRenderFamilies() {
				t.Run(missile.GetStringName(), func(t *testing.T) {
					scene.startProjectile(references.Position{X: 2, Y: 3}, references.Position{X: 4, Y: 3}, missile)
					for _, progress := range []float32{0, 0.25, 0.5, 0.75, 1} {
						scene.updateProjectileProgress(progress)
						flight := scene.activeProjectile
						assert.InDelta(t, flight.fromTileX+(flight.toTileX-flight.fromTileX)*float64(progress), flight.currentTileX, 0.000001)
						assert.Equal(t, flight.fromTileY, flight.currentTileY)
						if progress == 0.25 || progress == 0.75 {
							assertProjectileDrawAtCurrentPosition(t, scene, screen)
						}
					}
					scene.updateProjectileProgress(0.75)
					scene.screenShakeOffsetX, scene.screenShakeOffsetY = 6, -4
					assertProjectileDrawAtCurrentPosition(t, scene, screen)
					scene.screenShakeOffsetX, scene.screenShakeOffsetY = 0, 0
				})
			}
		})
	}
}

func TestProjectileFamiliesRespectSurfaceVisibility(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		t.Run(fmt.Sprintf("floor%d", floor), func(t *testing.T) {
			assertProjectileFamiliesRespectSurfaceVisibility(t, floor)
		})
	}
}

func assertProjectileFamiliesRespectSurfaceVisibility(t *testing.T, floor references.FloorNumber) {
	t.Helper()
	player := references.Position{X: 100, Y: 100}
	scene := newTerrainDitherTestScene(t, references.BritanniaUnderworld, floor, player)
	screen := newProjectileRenderTarget(t, scene, 1280, 720, indexes.Grass)
	layer := scene.gameState.GetLayeredMapByCurrentLocation()
	for _, missile := range enemyProjectileRenderFamilies() {
		t.Run(missile.GetStringName(), func(t *testing.T) {
			scene.startProjectile(references.Position{X: 98, Y: 100}, player, missile)
			scene.updateProjectileProgress(0.5)
			position := references.Position{X: 99, Y: 100}
			for _, visible := range []bool{true, false} {
				terrainDitherTestVisibility(scene, player)
				if visible {
					terrainDitherTestVisibility(scene, player, position)
				}
				scene.prepareTerrainDither(layer)
				require.True(t, scene.terrainDither.enabled)
				before, rngBefore := *scene.activeProjectile, scene.gameState.GetRNG().Calls()
				screen.Clear()
				scene.drawActiveProjectile(screen)
				assert.Equal(t, visible, opaquePixelCount(screen) > 0)
				// The center guard still applies when the shared world transform
				// contains a fractional camera offset and a screen-space shake.
				scene.surfaceCamera.x, scene.surfaceCamera.y = 0.5, -0.25
				scene.screenShakeOffsetX, scene.screenShakeOffsetY = 6, -4
				screen.Clear()
				scene.drawActiveProjectile(screen)
				assert.Equal(t, visible, opaquePixelCount(screen) > 0)
				scene.surfaceCamera.x, scene.surfaceCamera.y = 0, 0
				scene.screenShakeOffsetX, scene.screenShakeOffsetY = 0, 0
				assert.Equal(t, before, *scene.activeProjectile)
				assert.Equal(t, rngBefore, scene.gameState.GetRNG().Calls())
			}
		})
	}
}

func enemyProjectileRenderFamilies() []references.MissileType {
	return []references.MissileType{
		references.MissileRock, references.MissileArrow, references.MissileAxe, references.MissileCannonBall,
		references.MissileRed, references.MissileBlue, references.MissileGreen, references.MissileViolet,
	}
}

// Skip optional special actions using a fixed seed rather than editing any
// production ability or missile mapping. The actual scheduler still consumes
// these draws and makes the launch decision itself.
func projectileReferenceSeed(t *testing.T, reference references.EnemyReference) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 100000; seed++ {
		rng := beradrng.New(seed)
		if reference.HasAbility(references.PossessCharm) && rng.Intn(32) < 6 {
			continue
		}
		if reference.HasAbility(references.Invisibility) && rng.Intn(256) < 32 {
			continue
		}
		if reference.HasAbility(references.GatesInDaemon) && rng.Intn(256) < 32 {
			continue
		}
		if reference.MonsterType != references.MonsterMimic && rng.Intn(256) >= 128 {
			continue
		}
		if rng.Intn(61) >= 58 {
			return seed
		}
	}
	t.Fatal("no deterministic missile seed for ", reference.MonsterType.Name())

	return 0
}

func newProjectileRenderTarget(t *testing.T, scene *GameScene, width, height int, terrain indexes.SpriteIndex) *ebiten.Image {
	t.Helper()
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(width, height)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.mapImage = ebiten.NewImage(304, 208)
	t.Cleanup(scene.mapImage.Dispose)
	for y := 0; y < 208; y += sprites.TileSize {
		for x := 0; x < 304; x += sprites.TileSize {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Translate(float64(x), float64(y))
			scene.mapImage.DrawImage(scene.spriteSheet.GetSprite(terrain), op)
		}
	}
	screen := newPresentationRaster(width, height)
	t.Cleanup(screen.Dispose)

	return screen
}

// Read real Ebitengine pixels over terrain and check their bounded location,
// not a helper's geometry or a broad platform-sensitive golden screenshot.
func assertProjectileDrawAtCurrentPosition(t *testing.T, scene *GameScene, screen *ebiten.Image) {
	t.Helper()
	flight := scene.activeProjectile
	require.NotNil(t, flight)
	screen.Clear()
	op := sprites.GetDrawOptionsFromPercentsForWholeScreen(scene.mapImage, scene.mapScreenPlacement())
	if scene.terrainDither.enabled {
		op = scene.worldDrawOptions()
	} else {
		op.GeoM.Translate(scene.screenShakeOffsetX, scene.screenShakeOffsetY)
	}
	screen.DrawImage(scene.mapImage, op)
	before := make([]byte, screen.Bounds().Dx()*screen.Bounds().Dy()*4)
	after := make([]byte, len(before))
	screen.ReadPixels(before)
	scene.drawActiveProjectile(screen)
	screen.ReadPixels(after)
	drawn := image.Rectangle{}
	pixelCount := 0
	for i := 0; i < len(before); i += 4 {
		if bytes.Equal(before[i:i+4], after[i:i+4]) {
			continue
		}
		x, y := i/4%screen.Bounds().Dx(), i/4/screen.Bounds().Dx()
		drawn = drawn.Union(image.Rect(x, y, x+1, y+1))
		pixelCount++
	}
	require.Greater(t, pixelCount, 4, "projectile must contribute visible terrain-contrasting pixels")
	x := flight.currentTileX*sprites.TileSize - 1 + sprites.TileSize/2
	y := flight.currentTileY*sprites.TileSize - 1 + sprites.TileSize/2
	if scene.gameState.IsInCombat() {
		x += float64(scene.combatLayout().battlefieldLeft())
	}
	x, y = op.GeoM.Apply(x, y)
	unitX, unitY := op.GeoM.Apply(1, 1)
	originX, originY := op.GeoM.Apply(0, 0)
	assert.InDelta(t, x, float64(drawn.Min.X+drawn.Max.X)/2, math.Abs(unitX-originX)*3+1, "flight raster follows interpolated X")
	assert.InDelta(t, y, float64(drawn.Min.Y+drawn.Max.Y)/2, math.Abs(unitY-originY)*3+1, "flight raster follows interpolated Y")
	assert.LessOrEqual(t, float64(drawn.Dx()), math.Abs(unitX-originX)*18+2)
	assert.LessOrEqual(t, float64(drawn.Dy()), math.Abs(unitY-originY)*18+2)
}
