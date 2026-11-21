package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func newPhysicalCampingInputScene(t *testing.T, dungeon bool) (*GameScene, map[ebiten.Key]bool, func()) {
	t.Helper()
	location := references.BritanniaUnderworld
	if dungeon {
		location = references.Deceit
	}
	scene, held, _, _ := newRunicReferenceTestScene(t, location)
	state := scene.gameState
	state.SystemCallbacks.Flow.Rest.RequestCampDuration = scene.RequestCampDuration
	state.SystemCallbacks.Visual.IsAnyKeyPressed = scene.IsAnyKeyPressed
	state.SystemCallbacks.Screen.CreateAndPushCampingResultDialog = scene.createAndPushCampingResultDialog
	state.SystemCallbacks.Screen.CreateAndPushMessageDialog = scene.createAndPushMessageDialog
	state.SystemCallbacks.Screen.ShowPressAnyKeyPrompt = scene.showPressAnyKeyPrompt
	state.SystemCallbacks.Screen.ClearMessagePrompt = scene.clearMessagePrompt
	state.SystemCallbacks.Screen.HideMessageDialog = scene.hideMessageDialog
	state.SystemCallbacks.Screen.PopMessageDialog = scene.popMessageDialog
	state.CurrentNPCAIController.GetNpcs().ClearAllMapUnits()
	if dungeon {
		setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, 0x00)
	} else {
		position := state.MapState.PlayerLocation.Position
		state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
	}
	for index := range party_state.MaxActiveParty {
		state.PartyState.GetCharacter(index).Class = party_state.Fighter
	}
	state.DateTime.Minute = 0
	scene.keyboard.SetElapsedMillisecondsSource(scene.getElapsedMs)
	update := func() {
		t.Helper()
		state.ElapsedMs += 16
		require.NoError(t, scene.Update(nil))
	}
	t.Cleanup(func() {
		clear(held)
		scene.ResetTransientCommandInput()
		state.EventQueueManager.Shutdown()
	})
	return scene, held, update
}

func beginPhysicalOneHourCamp(t *testing.T, scene *GameScene, held map[ebiten.Key]bool, update func()) {
	t.Helper()
	workflow := "camping_workflow"
	if scene.gameState.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType {
		workflow = "dungeon_hole_up"
	}
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyH, func() bool {
		return scene.gameState.EventQueueManager.HasWorkflow(workflow)
	})
	var duration *widgets.HoursPrompt
	for range 200 {
		update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.HoursPrompt); ok {
			duration = dialog
			break
		}
	}
	require.NotNil(t, duration, "camping did not reach duration input")
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.Key1, func() bool {
		return duration.GetHours() == 1
	})
	require.Equal(t, 1, duration.GetHours())
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyEnter, func() bool {
		return !scene.dialogStack.ContainsWidgetInstance(duration)
	})
	guard := waitForCampingGuard(t, scene, update)
	initialGuard := guard.GetCurrentSelection()
	require.NotEqual(t, game_state.NoGuardAssigned, initialGuard,
		"the detached key must select No One rather than inheriting it")
	guardShownAt := scene.getElapsedMs()
	// Construction installs DialogDismissalDelayMs. A visible selector is not
	// immediately input-ready: keep the physical press alive until the normal
	// Update route accepts it, rather than tapping twice inside its 200ms guard.
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.Key0, func() bool {
		if scene.getElapsedMs()-guardShownAt < int64(widgets.DialogDismissalDelayMs) {
			require.Equal(t, initialGuard, guard.GetCurrentSelection(), "the opening input guard is still active")
		}
		return guard.GetCurrentSelection() == game_state.NoGuardAssigned
	})
	require.GreaterOrEqual(t, scene.getElapsedMs()-guardShownAt, int64(widgets.DialogDismissalDelayMs),
		"physical selection must honor the constructor's input guard")
	require.Equal(t, game_state.NoGuardAssigned, guard.GetCurrentSelection())
	require.Same(t, guard, scene.dialogStack.PeekTopModalDialog(), "number selection does not confirm")
	require.Nil(t, scene.gameState.CampingScene, "No One must be confirmed before camp is created")
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyEnter, func() bool {
		return !scene.dialogStack.ContainsWidgetInstance(guard)
	})
	for range 300 {
		update()
		if camp := scene.gameState.CampingScene; camp != nil {
			require.Equal(t, 1, camp.Hours)
			require.Equal(t, camping.CyclesPerHour, camp.TotalCycles)
			require.Equal(t, game_state.NoGuardAssigned, camp.GuardIndex)
			require.Zero(t, camp.CurrentCycle, "duration/watch selection must finish before sleeping")
			require.False(t, camp.IsSleeping())
			require.False(t, scene.dialogStack.HasModalDialog())
			return
		}
		require.True(t, scene.gameState.EventQueueManager.HasWorkflow(workflow),
			"camping ended before the confirmed camp was created")
	}
	t.Fatalf("confirmed camp was not created; modal=%T blocked=%t", scene.dialogStack.PeekTopModalDialog(), scene.inputBlocked)
}

// Drive one physical press through the production update/input-owner path until
// its visible outcome is observed. The bounded loop advances only the injected
// scene clock; it does not bypass release/debounce gates or invoke widget actions.
func pressPhysicalCampingKeyUntil(
	t *testing.T,
	scene *GameScene,
	held map[ebiten.Key]bool,
	update func(),
	key ebiten.Key,
	accepted func() bool,
) {
	t.Helper()
	require.Empty(t, held, "the preceding setup key must have been released")
	held[key] = true
	defer delete(held, key)
	startedAt := scene.getElapsedMs()
	for range 128 {
		update()
		if accepted() {
			delete(held, key)
			update()
			return
		}
	}
	t.Fatalf("physical %s was not accepted after %dms; modal=%T blocked=%t waitingForRelease=%t",
		key, scene.getElapsedMs()-startedAt, scene.dialogStack.PeekTopModalDialog(),
		scene.inputBlocked, scene.keyboard.IsWaitingForRelease())
}

func TestCampingSpaceThroughUpdateRequiresSleepingEdgeAndLatchesUntilResults(t *testing.T) {
	for _, dungeon := range []bool{false, true} {
		name := "outdoor"
		if dungeon {
			name = "dungeon"
		}
		t.Run(name, func(t *testing.T) {
			scene, held, update := newPhysicalCampingInputScene(t, dungeon)
			beginPhysicalOneHourCamp(t, scene, held, update)
			camp := scene.gameState.CampingScene
			require.False(t, camp.IsSleeping())
			held[ebiten.KeySpace] = true
			for range 1000 {
				update()
				if camp.IsSleeping() {
					break
				}
			}
			require.True(t, camp.IsSleeping())
			require.False(t, camp.IsAccelerated(), "setup hold cannot arm acceleration")
			require.Zero(t, scene.dialogStack.Count(), "sleeping has no hint box")
			delete(held, ebiten.KeySpace)
			update()
			before := scene.gameState.DateTime
			dialogsBefore := scene.dialogStack.Count()
			held[ebiten.KeySpace] = true
			update()
			require.True(t, camp.IsAccelerated())
			require.Equal(t, before, scene.gameState.DateTime, "Space itself adds no logical cycle")
			require.Zero(t, scene.dialogStack.Count(), "acceleration is silent")
			require.Equal(t, dialogsBefore, scene.dialogStack.Count(), "Space creates no feedback or acknowledgement box")
			delete(held, ebiten.KeySpace)
			update()
			require.True(t, camp.IsAccelerated(), "release does not toggle speed")
			require.Zero(t, scene.dialogStack.Count())
			held[ebiten.KeySpace] = true
			for range 1000 {
				update()
				if !camp.IsSleeping() {
					break
				}
			}
			require.Equal(t, camping.CyclesPerHour, camp.CurrentCycle)
			require.False(t, camp.IsAccelerated())
			require.Equal(t, 1, scene.dialogStack.Count(), "only the ordinary results dialog remains")
			require.True(t, scene.dialogStack.HasModalDialog())
			for range 50 {
				update()
			}
			require.NotNil(t, scene.gameState.CampingScene, "held speed-up cannot acknowledge results")
			delete(held, ebiten.KeySpace)
			for range 5 {
				update()
			}
			held[ebiten.KeyEnter] = true
			update()
			delete(held, ebiten.KeyEnter)
			for range 1000 {
				update()
				if scene.gameState.CampingScene == nil && !scene.inputBlocked {
					break
				}
			}
			require.Nil(t, scene.gameState.CampingScene)
			require.False(t, scene.transientCommandInput.pass.active)
		})
	}
}

func TestCampingSpeedIntentPreservesProductionWindTimeline(t *testing.T) {
	scene, held, update := newPhysicalCampingInputScene(t, false)
	control, controlHeld, controlUpdate := newPhysicalCampingInputScene(t, false)
	// Begin matching real workflows with identical setup input and elapsed time.
	beginPhysicalOneHourCamp(t, scene, held, update)
	beginPhysicalOneHourCamp(t, control, controlHeld, controlUpdate)
	for range 1000 {
		if scene.gameState.CampingScene.IsSleeping() {
			break
		}
		update()
		controlUpdate()
	}
	require.True(t, scene.gameState.CampingScene.IsSleeping())
	require.True(t, control.gameState.CampingScene.IsSleeping())
	require.Equal(t, scene.gameState.ElapsedMs, control.gameState.ElapsedMs)
	before := scene.gameState.GetRNG().Calls()
	dateBefore := scene.gameState.DateTime
	// Admit a wind tick before the already-scheduled first 200ms cycle. Both
	// production updates must consume the same wind draws; the Space intent adds
	// none. Different later durations deliberately have no seed-only guarantee.
	scene.gameState.ElapsedMs += 55
	control.gameState.ElapsedMs += 55
	held[ebiten.KeySpace] = true
	require.NoError(t, scene.Update(nil))
	require.NoError(t, control.Update(nil))
	require.True(t, scene.gameState.CampingScene.IsAccelerated())
	require.False(t, control.gameState.CampingScene.IsAccelerated())
	require.Greater(t, scene.gameState.GetRNG().Calls(), before)
	require.Equal(t, control.gameState.GetRNG().Calls(), scene.gameState.GetRNG().Calls())
	require.Equal(t, control.gameState.GetCurrentWind(), scene.gameState.GetCurrentWind())
	require.Equal(t, dateBefore, scene.gameState.DateTime)
}
