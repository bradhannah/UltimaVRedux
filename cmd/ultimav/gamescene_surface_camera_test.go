package main

import (
	"fmt"
	"image"
	"image/color"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceCameraCurves(t *testing.T) {
	for _, delta := range []references.Position{{X: 1}, {X: -1}, {Y: 1}, {Y: -1}} {
		for _, smooth := range []bool{false, true} {
			for _, sample := range []struct {
				now       int64
				remaining float64
			}{
				{50, 1}, {100, 1}, {125, .75}, {150, .5}, {200, 0}, {500, 0},
			} {
				p := surfaceCameraProjection{active: true, delta: delta, startedMs: 100, durationMs: 100, smoothstep: smooth}
				p.sample(sample.now)
				remaining := sample.remaining
				if smooth && sample.now == 125 {
					remaining = .84375
				}
				assert.InDelta(t, float64(delta.X)*16*remaining, p.x, 1e-9)
				assert.InDelta(t, float64(delta.Y)*16*remaining, p.y, 1e-9)
				assert.Equal(t, sample.now < 200, p.active)
			}
		}
	}
	p := surfaceCameraProjection{active: true, durationMs: -1}
	p.sample(0)
	assert.Zero(t, p)
}

func newSurfaceCameraTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithPlayerAt(15, 15).
		WithRandomSeed(469).WithSystemCallbacks().Build()
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	layer := state.GetLayeredMapByCurrentLocation()
	for _, pos := range []references.Position{{X: 15, Y: 15}, {X: 16, Y: 15}, {X: 16, Y: 16}} {
		layer.SetTileByLayer(map_state.MapOverrideLayer, &pos, indexes.Grass)
	}
	scene := newSurfaceCompletionTestScene(state)
	t.Cleanup(scene.outerTerrainDither.Dispose)
	scene.gameConfig = &config.UltimaVConfiguration{
		GameplayConfiguration: &config.GameplayConfiguration{SmoothSurfaceMovement: true},
		DebugConfiguration:    &config.DebugConfiguration{},
	}
	require.True(t, scene.canScrollSurface())
	return scene
}

func TestSurfaceCameraMovementCompletesImmediately(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		scene := newSurfaceCameraTestScene(t)
		scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = enabled
		state := scene.gameState
		turn := state.DateTime.Turn
		state.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3}
		scene.smallMapInputHandler(ebiten.KeyRight)
		assert.Equal(t, references.Position{X: 16, Y: 15}, state.MapState.PlayerLocation.Position)
		assert.Equal(t, turn+1, state.DateTime.Turn)
		assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
		assert.False(t, state.EventQueueManager.HasActiveWorkflows())
		assert.False(t, scene.inputBlocked)
		assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
		require.Equal(t, enabled, scene.surfaceCamera.active)
		if enabled {
			assert.Equal(t, int64(125), scene.surfaceCamera.durationMs)
			assert.False(t, scene.surfaceCamera.smoothstep)
			scene.updateSurfaceCamera()
			assert.Equal(t, 16.0, scene.surfaceCamera.x)
			state.ElapsedMs += 25
			scene.smallMapInputHandler(ebiten.KeyDown)
			scene.updateSurfaceCamera()
			assert.Zero(t, scene.surfaceCamera.x, "a fast corner replaces rather than accumulates projection")
			assert.Equal(t, 16.0, scene.surfaceCamera.y)
			scene.smallMapInputHandler(ebiten.KeySpace)
			assert.Zero(t, scene.surfaceCamera, "the next nonmovement command snaps projection")
		}
	}
}

func TestSurfaceCameraMountedMovement(t *testing.T) {
	for _, vehicle := range []references.VehicleType{references.HorseVehicle, references.CarpetVehicle} {
		for _, terrain := range []string{"town", "overworld", "underworld"} {
			t.Run(vehicle.GetLowerCaseName()+"/"+terrain, func(t *testing.T) {
				var scenes [2]*GameScene
				for index, enabled := range []bool{false, true} {
					var scene *GameScene
					if terrain == "town" {
						scene = newSurfaceCameraTestScene(t)
					} else {
						floor := references.FloorNumber(0)
						if terrain == "underworld" {
							floor = -1
						}
						scene = newLargeMapCameraTestScene(t, floor, references.Position{X: 255, Y: 50})
					}
					scenes[index] = scene
					state := scene.gameState
					origin := state.MapState.PlayerLocation
					state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, origin.Position, origin.Floor)
					scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = enabled
					if terrain == "town" {
						scene.smallMapInputHandler(ebiten.KeyRight)
					} else {
						scene.largeMapInputHandler(ebiten.KeyRight)
					}
					assert.NotEqual(t, origin.Position, state.MapState.PlayerLocation.Position)
					assert.False(t, scene.inputBlocked)
					assert.False(t, state.EventQueueManager.HasActiveWorkflows())
					require.Equal(t, enabled, scene.surfaceCamera.active)
				}
				off, on := scenes[0], scenes[1]
				assert.Equal(t, off.gameState.MapState.PlayerLocation, on.gameState.MapState.PlayerLocation)
				assert.Equal(t, off.gameState.DateTime, on.gameState.DateTime)
				assert.Equal(t, off.gameState.GetRNG().Calls(), on.gameState.GetRNG().Calls())
				assert.Equal(t, off.gameState.PartyState, on.gameState.PartyState)
				assert.Equal(t, int64(100), on.surfaceCamera.durationMs)
				on.updateSurfaceCamera()
				assert.Equal(t, 16.0, on.surfaceCamera.x)
				on.gameState.ElapsedMs += 50
				on.updateSurfaceCamera()
				assert.Equal(t, 8.0, on.surfaceCamera.x)
				on.gameState.ElapsedMs += 50
				on.updateSurfaceCamera()
				assert.Zero(t, on.surfaceCamera)
			})
		}
	}
}

func TestSurfaceCameraMountedTuning(t *testing.T) {
	for _, vehicle := range []references.VehicleType{references.HorseVehicle, references.CarpetVehicle} {
		for _, duration := range []int{0, 100, 125, 150} {
			t.Run(fmt.Sprintf("%s/%d", vehicle.GetLowerCaseName(), duration), func(t *testing.T) {
				scene := newSurfaceCameraTestScene(t)
				origin := scene.gameState.MapState.PlayerLocation
				scene.gameState.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, origin.Position, origin.Floor)
				scene.gameConfig.DebugConfiguration.SurfaceMovementDurationMs = duration
				scene.smallMapInputHandler(ebiten.KeyRight)
				if duration == 0 {
					duration = 125
				}
				assert.Equal(t, int64(duration*4/5), scene.surfaceCamera.durationMs)
			})
		}
	}
}

func TestSurfaceCameraLifecycle(t *testing.T) {
	for _, change := range []string{"position", "map", "floor", "layer", "disabled", "workflow", "projectile", "debug overlay", "dispose", "replacement", "elapsed"} {
		t.Run(change, func(t *testing.T) {
			scene := newSurfaceCameraTestScene(t)
			scene.smallMapInputHandler(ebiten.KeyRight)
			require.True(t, scene.surfaceCamera.active)
			switch change {
			case "position":
				scene.gameState.MapState.PlayerLocation.Position.X++
			case "map":
				scene.gameState.MapState.PlayerLocation.Location = references.BritanniaUnderworld
			case "floor":
				scene.gameState.MapState.PlayerLocation.Floor++
			case "layer":
				scene.surfaceCamera.layer = &map_state.LayeredMap{}
			case "disabled":
				scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = false
			case "workflow":
				scene.gameState.EventQueueManager.Workflow("camera_test").Priority(event_queue.PriorityLow).Sequential().First(func() {}).Wait(1000).Start()
			case "projectile":
				scene.activeProjectile = &ActiveProjectile{}
			case "debug overlay":
				scene.gameState.DebugOptions.LightOverlay = true
			case "dispose":
				scene.Dispose()
			case "replacement":
				scene.PrepareForSceneReplacement()
			case "elapsed":
				scene.gameState.ElapsedMs += 125
			}
			scene.updateSurfaceCamera()
			assert.Zero(t, scene.surfaceCamera)
			assert.Zero(t, scene.surfaceOverscanTiles())
		})
	}
}

func TestSurfaceCameraExcludesMovementProcessingTime(t *testing.T) {
	for _, processingMs := range []int64{25, 125, 200} {
		scene := newSurfaceCameraTestScene(t)
		state := scene.gameState
		state.ElapsedMs = 100
		state.SystemCallbacks.Message.ShowCommandPrompt = func(string) {
			state.ElapsedMs += processingMs
		}
		scene.smallMapInputHandler(ebiten.KeyRight)
		assert.Equal(t, references.Position{X: 16, Y: 15}, state.MapState.PlayerLocation.Position)
		require.True(t, scene.surfaceCamera.active)
		assert.Equal(t, 100+processingMs, scene.surfaceCamera.startedMs)
		assert.Equal(t, int64(125), scene.surfaceCamera.durationMs)
		scene.updateSurfaceCamera()
		assert.Equal(t, 16.0, scene.surfaceCamera.x, "processing time must not advance the animation")
		state.ElapsedMs += 25
		scene.updateSurfaceCamera()
		assert.InDelta(t, 12.8, scene.surfaceCamera.x, 1e-9)
		state.ElapsedMs = 100 + processingMs + 124
		scene.updateSurfaceCamera()
		assert.True(t, scene.surfaceCamera.active)
		state.ElapsedMs++
		scene.updateSurfaceCamera()
		assert.Zero(t, scene.surfaceCamera, "animation lasts the full 125 ms after processing")
	}
}

func TestSurfaceCameraPreservesDrawLimit(t *testing.T) {
	for _, elapsed := range []int64{16, 125} {
		scene := newSurfaceCameraTestScene(t)
		scene.gameConfig.DisplayConfiguration = &config.DisplayConfiguration{EnableFpsLimiting: true, TargetFPS: 30}
		scene.smallMapInputHandler(ebiten.KeyRight)
		require.True(t, scene.surfaceCamera.active)
		scene.gameState.ElapsedMs = elapsed
		scene.now = func() time.Time { return time.UnixMilli(elapsed) }
		scene.lastDrawTime = elapsed - 16
		// No render target is necessary: active and settling frames must both
		// return through the existing 33 ms draw gate before touching it.
		require.NotPanics(t, func() { scene.Draw(nil) })
		assert.Equal(t, elapsed-16, scene.lastDrawTime)
		assert.Equal(t, elapsed < 125, scene.surfaceCamera.active)
	}
}

func TestSurfaceCameraRejectsStaleCompletion(t *testing.T) {
	for _, change := range []string{"pushback", "map", "floor", "teleport"} {
		t.Run(change, func(t *testing.T) {
			scene := newSurfaceCameraTestScene(t)
			origin := scene.gameState.MapState.PlayerLocation
			expected := origin
			expected.Position.X++
			scene.gameState.MapState.PlayerLocation = expected
			switch change {
			case "pushback":
				scene.gameState.MapState.PlayerLocation = origin
			case "map":
				scene.gameState.MapState.PlayerLocation.Location = references.BritanniaUnderworld
			case "floor":
				scene.gameState.MapState.PlayerLocation.Floor++
			case "teleport":
				expected.Position.X += 3
				scene.gameState.MapState.PlayerLocation = expected
			}
			scene.installSurfaceCamera(origin, expected)
			assert.Zero(t, scene.surfaceCamera)
		})
	}
}

func TestSurfaceCameraTuningAndWorldTransform(t *testing.T) {
	scene := newSurfaceCameraTestScene(t)
	for _, duration := range []int{-1, 0, 100, 125, 150, 900} {
		scene.gameConfig.DebugConfiguration.SurfaceMovementDurationMs = duration
		scene.gameConfig.DebugConfiguration.SurfaceMovementCurve = "smoothstep"
		origin := scene.gameState.MapState.PlayerLocation
		expected := origin
		expected.Position.X++
		scene.gameState.MapState.PlayerLocation = expected
		scene.installSurfaceCamera(origin, expected)
		want := int64(duration)
		if duration != 100 && duration != 125 && duration != 150 {
			want = 125
		}
		assert.Equal(t, want, scene.surfaceCamera.durationMs)
		assert.True(t, scene.surfaceCamera.smoothstep)
		scene.gameState.MapState.PlayerLocation = origin
	}
	scene.surfaceCamera.sample(scene.getElapsedMs())
	scene.mapImage = ebiten.NewImage(xTilesVisibleOnGameScreen*sprites.TileSize, yTilesVisibleOnGameScreen*sprites.TileSize)
	t.Cleanup(scene.mapImage.Deallocate)
	scene.screenShakeOffsetX, scene.screenShakeOffsetY = 3, -2
	// Layout helpers use the global display size, as all existing world renderers do.
	layout := sprites.GetDrawOptionsFromPercentsForWholeScreen(scene.mapImage, scene.mapScreenPlacement())
	x, y := scene.worldDrawOptions().GeoM.Apply(31, 47)
	wantX, wantY := layout.GeoM.Apply(31+scene.surfaceCamera.x, 47+scene.surfaceCamera.y)
	assert.InDelta(t, wantX+3, x, 1e-9)
	assert.InDelta(t, wantY-2, y, 1e-9)
	assert.Equal(t, 19*16, scene.mapImage.Bounds().Dx())
	assert.Equal(t, 13*16, scene.mapImage.Bounds().Dy())
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.avatarDrawOptions = &ebiten.DrawImageOptions{}
	// Only the published transform is asserted here; the raster is covered by
	// the camera rendering tests. Reuse a clipped target for both actor passes.
	target := ebiten.NewImage(1, 1)
	t.Cleanup(target.Deallocate)
	scene.drawAvatarOverlay(target)
	avatarTransform := scene.avatarDrawOptions.GeoM
	scene.surfaceCamera = surfaceCameraProjection{}
	scene.drawAvatarOverlay(target)
	assert.Equal(t, avatarTransform, scene.avatarDrawOptions.GeoM, "Avatar must not inherit world camera translation")
}

func TestSurfaceCameraRejectedMovementSnapsWithoutInstalling(t *testing.T) {
	scene := newSurfaceCameraTestScene(t)
	scene.smallMapInputHandler(ebiten.KeyRight)
	require.True(t, scene.surfaceCamera.active)
	origin, direction := findBlockedSmallMapStep(t, scene.gameState)
	scene.gameState.MapState.PlayerLocation.Position = origin
	scene.smallMapInputHandler(smallMapMovementKey(direction))
	assert.Equal(t, origin, scene.gameState.MapState.PlayerLocation.Position)
	assert.Zero(t, scene.surfaceCamera)
}

func TestSurfaceCameraOverscanTerrainIsLive(t *testing.T) {
	scene := newSurfaceCameraTestScene(t)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.xRayVisionActive = true
	scene.smallMapInputHandler(ebiten.KeyRight)
	scene.updateSurfaceCamera()
	require.True(t, scene.surfaceCamera.active)
	rngCalls := scene.gameState.GetRNG().Calls()
	scene.refreshAllMapLayerTiles()
	buffer := scene.surfaceTerrainImage
	require.NotNil(t, buffer)
	assert.Equal(t, (19+4)*16+2, buffer.Bounds().Dx())
	assert.Equal(t, (13+4)*16+2, buffer.Bounds().Dy())
	assert.Equal(t, 19*16+2, scene.unscaledMapImage.Bounds().Dx())
	assert.Equal(t, 13*16+2, scene.unscaledMapImage.Bounds().Dy())

	// Tile -1 is outside the ordinary terrain scan but enters view on an east step.
	pos := scene.gameState.MapState.PlayerLocation.Position
	pos.X -= xCenter + 1
	layer := scene.gameState.GetLayeredMapByCurrentLocation()
	region := image.Rect(16, (int(yCenter)+2)*16, 32, (int(yCenter)+3)*16)
	layer.SetTileByLayer(map_state.MapOverrideLayer, &pos, indexes.Grass)
	scene.refreshAllMapLayerTiles()
	before := make([]byte, 16*16*4)
	buffer.SubImage(region).(*ebiten.Image).ReadPixels(before)
	layer.SetTileByLayer(map_state.MapOverrideLayer, &pos, indexes.BrickFloor)
	scene.refreshAllMapLayerTiles()
	after := make([]byte, len(before))
	buffer.SubImage(region).(*ebiten.Image).ReadPixels(after)
	assert.Same(t, buffer, scene.surfaceTerrainImage, "overscan storage must persist across frames")
	assert.NotEqual(t, before, after, "overscan must render current terrain, not captured viewport pixels")
	scene.gameState.ElapsedMs += 125
	scene.updateSurfaceCamera()
	scene.refreshAllMapLayerTiles()
	assert.False(t, scene.surfaceCamera.active)
	assert.Same(t, buffer, scene.surfaceTerrainImage, "keep the buffer for the next step")
	assert.Equal(t, rngCalls, scene.gameState.GetRNG().Calls())
}

func TestSurfaceCameraWorldClipDoesNotMoveWithProjectionOrShake(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	const width, height = 400, 208
	manager.PublishLogicalSize(width, height)
	scene := newSurfaceCameraTestScene(t)
	screen := newPresentationRaster(width, height)
	defer screen.Deallocate()
	_, scene.borders.surfaceAperture = surfaceMapBorderGeometry()
	assert.Equal(t, scene.borders.surfaceAperture, scene.worldRenderTarget(screen).Bounds(), "settled surface uses the same aperture")
	scene.smallMapInputHandler(ebiten.KeyRight)
	scene.updateSurfaceCamera()
	scene.screenShakeOffsetX, scene.screenShakeOffsetY = 7, -3
	clip := scene.borders.surfaceAperture
	for _, elapsed := range []int64{0, 60, 124} {
		scene.gameState.ElapsedMs = elapsed
		scene.updateSurfaceCamera()
		screen.Fill(color.RGBA{R: 255, A: 255})
		target := scene.worldRenderTarget(screen)
		assert.Equal(t, clip, target.Bounds())
		// Any ordinary world layer can overdraw freely inside this fixed clip.
		target.Fill(color.White)
		assert.Equal(t, color.RGBA{R: 255, G: 255, B: 255, A: 255}, screen.At(clip.Max.X-1, clip.Dy()/2))
		assert.Equal(t, color.RGBA{R: 255, A: 255}, screen.At(clip.Max.X, clip.Dy()/2), "world must not leak into the UI")
	}
}
