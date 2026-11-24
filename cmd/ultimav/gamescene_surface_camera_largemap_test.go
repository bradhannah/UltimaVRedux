package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func newLargeMapCameraTestScene(t *testing.T, floor references.FloorNumber, origin references.Position) *GameScene {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithFloor(floor).
		WithPlayerAt(int(origin.X), int(origin.Y)).WithRandomSeed(469).WithSystemCallbacks().Build()
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	layer := state.GetLayeredMapByCurrentLocation()
	for x := references.Coordinate(-2); x <= 2; x++ {
		for y := references.Coordinate(-2); y <= 2; y++ {
			pos := references.Position{X: origin.X + x, Y: origin.Y + y}
			layer.SetTileByLayer(map_state.MapOverrideLayer, pos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles), indexes.Grass)
		}
	}
	scene := newSurfaceCompletionTestScene(state)
	scene.gameConfig = &config.UltimaVConfiguration{
		GameplayConfiguration: &config.GameplayConfiguration{SmoothSurfaceMovement: true},
		DebugConfiguration:    &config.DebugConfiguration{},
	}
	require.True(t, scene.canScrollSurface())
	return scene
}

func TestSurfaceCameraLargeMapSeamsAndGameplayParity(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		for _, step := range []struct {
			origin, destination, delta references.Position
			key                        ebiten.Key
		}{
			{references.Position{X: 255, Y: 50}, references.Position{X: 0, Y: 50}, references.Position{X: 1}, ebiten.KeyRight},
			{references.Position{X: 0, Y: 50}, references.Position{X: 255, Y: 50}, references.Position{X: -1}, ebiten.KeyLeft},
			{references.Position{X: 50, Y: 255}, references.Position{X: 50, Y: 0}, references.Position{Y: 1}, ebiten.KeyDown},
			{references.Position{X: 50, Y: 0}, references.Position{X: 50, Y: 255}, references.Position{Y: -1}, ebiten.KeyUp},
			{references.Position{X: 50, Y: 50}, references.Position{X: 51, Y: 50}, references.Position{X: 1}, ebiten.KeyRight},
		} {
			off := newLargeMapCameraTestScene(t, floor, step.origin)
			on := newLargeMapCameraTestScene(t, floor, step.origin)
			off.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = false
			for _, scene := range []*GameScene{off, on} {
				scene.gameState.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3}
				scene.largeMapInputHandler(step.key)
				assert.Equal(t, step.destination, scene.gameState.MapState.PlayerLocation.Position)
				assert.Equal(t, uint32(1), scene.gameState.DateTime.Turn)
				assert.Equal(t, 2, scene.gameState.ActiveDurationEffect.TurnsRemaining)
				assert.False(t, scene.inputBlocked)
				assert.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
			}
			require.True(t, on.surfaceCamera.active)
			assert.False(t, off.surfaceCamera.active)
			assert.Equal(t, step.delta, on.surfaceCamera.delta)
			assert.Equal(t, off.gameState.DateTime, on.gameState.DateTime)
			assert.Equal(t, off.gameState.GetRNG().Calls(), on.gameState.GetRNG().Calls())
			assert.Equal(t, off.gameState.PartyState, on.gameState.PartyState)
			on.updateSurfaceCamera()
			assert.Equal(t, float64(step.delta.X)*16, on.surfaceCamera.x)
			assert.Equal(t, float64(step.delta.Y)*16, on.surfaceCamera.y)
			on.largeMapInputHandler(ebiten.KeySpace)
			assert.Zero(t, on.surfaceCamera)
		}
	}
}

func TestSurfaceCameraLargeMapBlockedAndCodexPushback(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		scene := newLargeMapCameraTestScene(t, 0, references.Position{X: 255, Y: 50})
		scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = enabled
		target := references.Position{X: 0, Y: 50}
		scene.gameState.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &target, indexes.TallMountains)
		scene.surfaceCamera = surfaceCameraProjection{active: true}
		scene.largeMapInputHandler(ebiten.KeyRight)
		assert.Equal(t, references.Position{X: 255, Y: 50}, scene.gameState.MapState.PlayerLocation.Position)
		assert.Zero(t, scene.gameState.DateTime.Turn)
		assert.Zero(t, scene.surfaceCamera)

		scene = newLargeMapCameraTestScene(t, 0, references.Position{X: 0xe9, Y: 0xec})
		scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = enabled
		scene.largeMapInputHandler(ebiten.KeyUp)
		assert.Equal(t, references.Position{X: 0xe9, Y: 0xec}, scene.gameState.MapState.PlayerLocation.Position)
		assert.Equal(t, uint32(1), scene.gameState.DateTime.Turn)
		assert.Zero(t, scene.surfaceCamera, "synchronous completion pushback must not animate")
	}
}

func TestSurfaceCameraManualBoatsUseOrdinaryTuning(t *testing.T) {
	for _, vehicleType := range []references.VehicleType{references.SkiffVehicle, references.FrigateVehicle} {
		t.Run(vehicleType.GetLowerCaseName(), func(t *testing.T) {
			for _, step := range []struct {
				name      string
				origin    references.Position
				direction references.Direction
				key       ebiten.Key
				delta     references.Position
			}{
				{name: "ordinary", origin: references.Position{X: 50, Y: 50}, direction: references.Right, key: ebiten.KeyRight, delta: references.Position{X: 1}},
				{name: "wrap x", origin: references.Position{X: 255, Y: 50}, direction: references.Right, key: ebiten.KeyRight, delta: references.Position{X: 1}},
				{name: "wrap y", origin: references.Position{X: 50, Y: 255}, direction: references.Down, key: ebiten.KeyDown, delta: references.Position{Y: 1}},
			} {
				t.Run(step.name, func(t *testing.T) {
					var scenes [2]*GameScene
					for index, enabled := range []bool{false, true} {
						scene := newLargeMapCameraTestScene(t, 0, step.origin)
						scenes[index] = scene
						state := scene.gameState
						origin := state.MapState.PlayerLocation
						target := step.direction.GetNewPositionInDirection(&origin.Position).
							GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
						state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Water2)
						vehicle := map_units.NewNPCFriendlyVehiceNewRef(vehicleType, origin.Position, origin.Floor)
						vehicle.GetVehicleDetails().SetPartyVehicleDirection(step.direction)
						vehicle.GetVehicleDetails().SetSailState(map_units.SailFurled)
						state.PartyVehicle = *vehicle
						scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = enabled
						scene.gameConfig.DebugConfiguration.SurfaceMovementDurationMs = 150
						scene.gameConfig.DebugConfiguration.SurfaceMovementCurve = "smoothstep"

						scene.largeMapInputHandler(step.key)
						assert.Equal(t, *target, state.MapState.PlayerLocation.Position)
					}
					off, on := scenes[0], scenes[1]
					assert.Zero(t, off.surfaceCamera)
					require.True(t, on.surfaceCamera.active)
					assert.False(t, on.surfaceCamera.automaticSailing)
					assert.Equal(t, int64(150), on.surfaceCamera.durationMs, "boats do not use the mount speed factor")
					assert.True(t, on.surfaceCamera.smoothstep)
					assert.Equal(t, step.delta, on.surfaceCamera.delta)
					assert.Equal(t, off.gameState.DateTime, on.gameState.DateTime)
					assert.Equal(t, off.gameState.PartyState, on.gameState.PartyState)
					assert.Equal(t, off.gameState.GetRNG().Calls(), on.gameState.GetRNG().Calls())
					assert.Equal(t, off.gameState.PartyVehicle.GetVehicleDetails().Snapshot(), on.gameState.PartyVehicle.GetVehicleDetails().Snapshot())
				})
			}
		})
	}
}

func TestSurfaceCameraFurledFrigateHeadingOnlyClearsWithoutTween(t *testing.T) {
	scene := newLargeMapCameraTestScene(t, 0, references.Position{X: 50, Y: 50})
	state := scene.gameState
	vehicle := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle, state.MapState.PlayerLocation.Position, state.MapState.PlayerLocation.Floor,
	)
	vehicle.GetVehicleDetails().SetSailState(map_units.SailFurled)
	vehicle.GetVehicleDetails().SetPartyVehicleDirection(references.Right)
	state.PartyVehicle = *vehicle
	scene.surfaceCamera = surfaceCameraProjection{active: true}
	origin := state.MapState.PlayerLocation.Position

	scene.largeMapInputHandler(ebiten.KeyUp)

	assert.Equal(t, origin, state.MapState.PlayerLocation.Position)
	assert.Equal(t, references.Up, state.PartyVehicle.GetVehicleDetails().CurrentDirection())
	assert.Zero(t, scene.surfaceCamera)
}

func TestSurfaceCameraAutomaticSailingUsesPhaseCadenceAndSelectedCurve(t *testing.T) {
	fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
	enableAutomaticSailingCamera(fixture.scene, true)
	fixture.scene.gameConfig.DebugConfiguration.SurfaceMovementDurationMs = 150
	fixture.scene.gameConfig.DebugConfiguration.SurfaceMovementCurve = "smoothstep"
	before := fixture.state.MapState.PlayerLocation.Position

	fixture.scene.largeMapInputHandler(ebiten.KeyDown)
	fixture.advancePhases(1)
	assert.Zero(t, fixture.scene.surfaceCamera, "wind wait phases do not scroll")
	fixture.advancePhases(1)

	assert.Equal(t, *references.Down.GetNewPositionInDirection(&before), fixture.state.MapState.PlayerLocation.Position)
	require.True(t, fixture.scene.surfaceCamera.active)
	assert.True(t, fixture.scene.surfaceCamera.automaticSailing)
	assert.Equal(t, int64(automaticSailingPhaseIntervalMs), fixture.scene.surfaceCamera.durationMs)
	assert.True(t, fixture.scene.surfaceCamera.smoothstep)
	projection := fixture.scene.surfaceCamera
	sampleOffset := projection.durationMs / 2
	projection.sample(projection.startedMs + sampleOffset)
	progress := float64(sampleOffset) / float64(projection.durationMs)
	progress = progress * progress * (3 - 2*progress)
	assert.InDelta(t, float64(sprites.TileSize)*(1-progress), projection.y, 1e-9)
	projection.sample(projection.startedMs + projection.durationMs)
	assert.Zero(t, projection, "the tween completes at the next phase boundary")

	fixture.advancePhases(1)
	assert.Zero(t, fixture.scene.surfaceCamera, "the following wind wait clears the completed tween")
}

func TestSurfaceCameraAutomaticSailingWrapsAtWorldSeam(t *testing.T) {
	for _, test := range []struct {
		name      string
		origin    references.Position
		direction references.Direction
		key       ebiten.Key
		phases    int
		delta     references.Position
	}{
		{name: "x", origin: references.Position{X: 255, Y: 50}, direction: references.Right, key: ebiten.KeyRight, phases: 1, delta: references.Position{X: 1}},
		{name: "y", origin: references.Position{X: 50, Y: 255}, direction: references.Down, key: ebiten.KeyDown, phases: 2, delta: references.Position{Y: 1}},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
			enableAutomaticSailingCamera(fixture.scene, true)
			fixture.state.SetPlayerPosition(test.origin)
			fixture.state.PartyVehicle.SetPos(test.origin)

			fixture.scene.largeMapInputHandler(test.key)
			fixture.advancePhases(test.phases)

			want := test.direction.GetNewPositionInDirection(&test.origin).
				GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			assert.Equal(t, *want, fixture.state.MapState.PlayerLocation.Position)
			require.True(t, fixture.scene.surfaceCamera.active)
			assert.Equal(t, test.delta, fixture.scene.surfaceCamera.delta)
		})
	}
}

func TestSurfaceCameraAutomaticSailingNoMotionPhasesClearTween(t *testing.T) {
	t.Run("calm wait", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.Calm)
		enableAutomaticSailingCamera(fixture.scene, true)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)
		fixture.scene.surfaceCamera = surfaceCameraProjection{active: true}

		fixture.advancePhases(1)

		assert.Zero(t, fixture.scene.surfaceCamera)
	})

	t.Run("heading change", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		enableAutomaticSailingCamera(fixture.scene, true)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)
		fixture.scene.surfaceCamera = surfaceCameraProjection{active: true}
		fixture.held[ebiten.KeyUp] = true

		fixture.advancePhases(1)

		assert.Equal(t, references.Up, fixture.state.PartyVehicle.GetVehicleDetails().CurrentDirection())
		assert.Zero(t, fixture.scene.surfaceCamera)
	})

	t.Run("blocked", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		enableAutomaticSailingCamera(fixture.scene, true)
		fixture.state.DebugOptions.FreeMove = false
		target := references.Right.GetNewPositionInDirection(&fixture.state.MapState.PlayerLocation.Position)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.TallMountains)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		fixture.advancePhases(1)

		assert.Zero(t, fixture.scene.surfaceCamera)
		assert.False(t, fixture.state.IsSailing())
	})

	t.Run("hazard workflow", func(t *testing.T) {
		fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
		enableAutomaticSailingCamera(fixture.scene, true)
		target := references.Right.GetNewPositionInDirection(&fixture.state.MapState.PlayerLocation.Position)
		waterfall := target.GetPositionDown()
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Water1)
		fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, waterfall, indexes.Waterfall1)
		fixture.scene.largeMapInputHandler(ebiten.KeyRight)

		fixture.advancePhases(1)

		assert.True(t, fixture.state.EventQueueManager.HasWorkflow("waterfall_hazard"))
		assert.Zero(t, fixture.scene.surfaceCamera)
	})
}

func TestSurfaceCameraAutomaticSailingCleanupAndInputPreservation(t *testing.T) {
	fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
	enableAutomaticSailingCamera(fixture.scene, true)
	fixture.scene.largeMapInputHandler(ebiten.KeyRight)
	fixture.advancePhases(1)
	require.True(t, fixture.scene.surfaceCamera.active)
	projection := fixture.scene.surfaceCamera

	fixture.scene.clearSurfaceCameraForInput(ebiten.KeyRight)
	assert.Equal(t, projection, fixture.scene.surfaceCamera,
		"same-frame movement routing preserves the sailing-owned projection")

	fixture.held[ebiten.KeyL] = true

	fixture.advancePhases(1)

	assert.Zero(t, fixture.scene.surfaceCamera)
	assert.False(t, fixture.state.IsSailing())
	assert.False(t, fixture.scene.keyboard.IsWaitingForRelease(), "non-stop input remains available to normal routing")
	fixture.scene.largeMapInputHandler(ebiten.KeyL)
	assert.Equal(t, LookDirectionInput, fixture.scene.secondaryKeyState)
}

func TestSurfaceCameraAutomaticSailingGameplayParity(t *testing.T) {
	tests := []struct {
		name       string
		wind       environment.WindDirection
		key        ebiten.Key
		phases     int
		wantMotion bool
		wantDelta  references.Position
		prepare    func(*automaticSailingWorkflowFixture)
	}{
		{name: "ordinary", wind: environment.North, key: ebiten.KeyUp, phases: 3, wantMotion: true, wantDelta: references.Position{Y: -1}},
		{
			name: "HMS Cape", wind: environment.North, key: ebiten.KeyUp, phases: 3,
			wantMotion: true, wantDelta: references.Position{Y: -1},
			prepare: func(fixture *automaticSailingWorkflowFixture) {
				fixture.state.SetHMSCapeActivated(true)
			},
		},
		{name: "calm", wind: environment.Calm, key: ebiten.KeyRight, phases: 3},
		{
			name: "wrap x", wind: environment.North, key: ebiten.KeyRight, phases: 1,
			wantMotion: true, wantDelta: references.Position{X: 1},
			prepare: func(fixture *automaticSailingWorkflowFixture) {
				position := references.Position{X: 255, Y: 50}
				fixture.state.SetPlayerPosition(position)
				fixture.state.PartyVehicle.SetPos(position)
			},
		},
		{
			name: "wrap y", wind: environment.North, key: ebiten.KeyDown, phases: 2,
			wantMotion: true, wantDelta: references.Position{Y: 1},
			prepare: func(fixture *automaticSailingWorkflowFixture) {
				position := references.Position{X: 50, Y: 255}
				fixture.state.SetPlayerPosition(position)
				fixture.state.PartyVehicle.SetPos(position)
			},
		},
		{
			name: "interruption", wind: environment.North, key: ebiten.KeyRight, phases: 1,
			prepare: func(fixture *automaticSailingWorkflowFixture) {
				fixture.held[ebiten.KeySpace] = true
			},
		},
		{
			name: "waterfall hazard", wind: environment.North, key: ebiten.KeyRight, phases: 1,
			prepare: func(fixture *automaticSailingWorkflowFixture) {
				target := references.Right.GetNewPositionInDirection(&fixture.state.MapState.PlayerLocation.Position)
				waterfall := target.GetPositionDown()
				fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Water1)
				fixture.state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, waterfall, indexes.Waterfall1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var fixtures [2]automaticSailingWorkflowFixture
			for index, enabled := range []bool{false, true} {
				fixtures[index] = newAutomaticSailingWorkflowFixture(t, test.wind)
				enableAutomaticSailingCamera(fixtures[index].scene, enabled)
				fixtures[index].state.ActiveDurationEffect = game_state.DurationEffect{
					EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
				}
				if test.prepare != nil {
					test.prepare(&fixtures[index])
				}
				fixtures[index].scene.largeMapInputHandler(test.key)
				fixtures[index].advancePhases(test.phases)
			}
			off, on := fixtures[0], fixtures[1]
			assert.Zero(t, off.scene.surfaceCamera)
			if test.wantMotion {
				require.True(t, on.scene.surfaceCamera.active)
				assert.Equal(t, test.wantDelta, on.scene.surfaceCamera.delta)
			} else {
				assert.Zero(t, on.scene.surfaceCamera)
			}
			assert.Equal(t, off.state.MapState.PlayerLocation, on.state.MapState.PlayerLocation)
			assert.Equal(t, off.state.DateTime, on.state.DateTime)
			assert.Equal(t, off.state.PartyState, on.state.PartyState)
			assert.Equal(t, off.state.GetRNG().Calls(), on.state.GetRNG().Calls())
			assert.Equal(t, off.callbacks.TimeAdvanced(), on.callbacks.TimeAdvanced())
			assert.Equal(t, off.callbacks.Messages, on.callbacks.Messages)
			assert.Equal(t, off.state.ActiveDurationEffect, on.state.ActiveDurationEffect)
			assert.Equal(t, off.state.PartyVehicle.GetVehicleDetails().Snapshot(), on.state.PartyVehicle.GetVehicleDetails().Snapshot())
			assert.Equal(t, off.state.IsSailing(), on.state.IsSailing())
			assert.Equal(t, off.state.EventQueueManager.HasWorkflow("waterfall_hazard"),
				on.state.EventQueueManager.HasWorkflow("waterfall_hazard"))
			assert.Equal(t, off.scene.keyboard.IsWaitingForRelease(), on.scene.keyboard.IsWaitingForRelease())
		})
	}
}

func enableAutomaticSailingCamera(scene *GameScene, enabled bool) {
	scene.gameConfig = &config.UltimaVConfiguration{
		GameplayConfiguration: &config.GameplayConfiguration{SmoothSurfaceMovement: enabled},
		DebugConfiguration:    &config.DebugConfiguration{},
	}
}

func TestSurfaceCameraLargeMapContactRemainsSynchronous(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		scene := newLargeMapCameraTestScene(t, 0, references.Position{X: 255, Y: 50})
		scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = enabled
		state := scene.gameState
		target := references.Position{X: 0, Y: 50}
		enemyRef := (*state.GameReferences.EnemyReferences)[int(references.MonsterDragon)]
		enemy := map_units.NewEnemyNPC(enemyRef, 1)
		enemy.SetPos(target)
		enemy.SetFloor(state.MapState.PlayerLocation.Floor)
		enemy.SetVisible(true)
		require.True(t, state.GetCurrentLargeMapNPCAIController().GetNpcs().AddEnemy(enemy))
		scene.largeMapInputHandler(ebiten.KeyRight)
		assert.True(t, state.HasPendingOverworldContact())
		assert.Equal(t, references.Position{X: 255, Y: 50}, state.MapState.PlayerLocation.Position)
		assert.Zero(t, state.DateTime.Turn, "contact owns dispatch without an extra movement cycle")
		assert.Zero(t, scene.surfaceCamera)
		assert.False(t, scene.canScrollSurface())
	}
}

func TestSurfaceCameraWrappedWorldProjection(t *testing.T) {
	scene := newLargeMapCameraTestScene(t, 0, references.Position{})
	for _, settings := range []struct{ enabled, active bool }{{false, false}, {false, true}, {true, false}, {true, true}} {
		scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = settings.enabled
		scene.surfaceCamera.active = settings.active
		for _, pair := range []struct{ world, player, want references.Position }{
			{references.Position{X: 255}, references.Position{}, references.Position{X: -1}},
			{references.Position{}, references.Position{X: 255}, references.Position{X: 1}},
			{references.Position{Y: 255}, references.Position{}, references.Position{Y: -1}},
			{references.Position{}, references.Position{Y: 255}, references.Position{Y: 1}},
		} {
			scene.gameState.MapState.PlayerLocation.Position = pair.player
			want := pair.want
			if !settings.enabled {
				want = references.Position{X: pair.world.X - pair.player.X, Y: pair.world.Y - pair.player.Y}
			}
			assert.Equal(t, want, scene.surfaceWorldRelativePosition(pair.world))
		}
	}
}
