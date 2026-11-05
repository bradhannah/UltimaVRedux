package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func waitForCampingGuard(t *testing.T, scene *GameScene, update func()) *widgets.CharacterSelectionDialog {
	t.Helper()
	for range 100 {
		update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog); ok {
			return dialog
		}
	}
	t.Fatal("camping did not reach guard selection")
	return nil
}

func enterCampingDuration(t *testing.T, scene *GameScene, update func()) {
	t.Helper()
	for range 100 {
		update()
		if _, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InputBox); ok {
			for _, key := range []ebiten.Key{ebiten.Key1, ebiten.KeyEnter} {
				scene.keyboard.SimulateKeyPress(key)
				update()
			}
			return
		}
	}
	t.Fatal("camping did not reach duration input")
}

func TestSurfaceCampingGuardCancellationAndExplicitChoices(t *testing.T) {
	for _, choice := range []string{"cancel", "no one", "guard", "poisoned"} {
		t.Run(choice, func(t *testing.T) {
			scene, _, advance, callbacks := newRunicReferenceTestScene(t, references.BritanniaUnderworld)
			windOnly, _, _, _ := newRunicReferenceTestScene(t, references.BritanniaUnderworld)
			update := func() {
				advance()
				windOnly.gameState.ProcessWindThroughElapsed(scene.gameState.ElapsedMs)
			}
			state := scene.gameState
			if choice == "poisoned" {
				state.PartyState.GetCharacter(1).SetStatus(party_state.Poisoned)
			}
			state.SystemCallbacks.Flow.Rest.RequestCampDuration = scene.RequestCampDuration
			state.CurrentNPCAIController.GetNpcs().ClearAllMapUnits()
			position := state.MapState.PlayerLocation.Position
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
			state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.CarpetVehicle, position, state.MapState.PlayerLocation.Floor)
			require.Greater(t, state.PartyState.CountActiveMembers(), 1)
			beforeDate := state.DateTime
			before, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			callbacks.Reset()
			scene.keyboard.SimulateKeyPress(ebiten.KeyH)
			update()
			require.True(t, state.EventQueueManager.HasWorkflow("camping_workflow"))
			require.False(t, state.EventQueueManager.HasWorkflow("surface_command_completion"))
			enterCampingDuration(t, scene, update)
			guard := waitForCampingGuard(t, scene, update)
			requireGuardResourceSnapshot(t, state, guard)
			selected := guard.GetCurrentSelection()
			switch choice {
			case "poisoned":
				scene.keyboard.SimulateKeyPress(ebiten.Key2)
				update()
				require.Equal(t, 1, guard.GetCurrentSelection())
				require.Equal(t, "A poisoned member cannot keep watch.", guard.GetCurrentOption().Details[0])
				require.Same(t, guard, scene.dialogStack.PeekTopModalDialog())
				require.Nil(t, state.CampingScene)
				// The original turns a non-Good guard into No One. ref:CAMP-0001
				selected = game_state.NoGuardAssigned
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			case "cancel":
				scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
			case "no one":
				scene.keyboard.SimulateKeyPress(ebiten.Key0)
				update()
				require.Equal(t, game_state.NoGuardAssigned, guard.GetCurrentSelection())
				selected = game_state.NoGuardAssigned
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			case "guard":
				require.NotEqual(t, game_state.NoGuardAssigned, selected)
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			}
			update()
			for range 100 {
				update()
				if state.CampingScene != nil || !state.EventQueueManager.HasWorkflow("camping_workflow") {
					break
				}
			}
			if choice != "cancel" {
				require.NotNil(t, state.CampingScene)
				require.Equal(t, selected, state.CampingScene.GuardIndex)
				require.Equal(t, 1, state.CampingScene.Hours)
				anyKeyChecks := 0
				state.SystemCallbacks.Visual.IsAnyKeyPressed = func() bool {
					anyKeyChecks++
					return anyKeyChecks%2 == 0
				}
				for range 2000 {
					state.ElapsedMs += 500
					require.NoError(t, scene.Update(nil))
					if !state.EventQueueManager.HasWorkflow("camping_workflow") {
						break
					}
				}
				require.False(t, state.EventQueueManager.HasWorkflow("camping_workflow"))
				require.Nil(t, state.CampingScene)
				require.Zero(t, scene.dialogStack.Count())
				require.False(t, state.EventQueueManager.HasWorkflow("surface_command_completion"))
				require.False(t, state.HasPendingOverworldContact())
				require.Nil(t, state.CombatScene)
				// TimeAdvanced derives an aggregate from calendar/turn state; it is
				// not a callback log containing one entry per two-minute completion.
				expectedDate := beforeDate
				for range camping.CyclesPerHour {
					expectedDate.Advance(camping.MinutesPerCycle)
				}
				expectedDate.Advance(game_state.DefaultLargeMapMinutesPerTurn)
				require.Equal(t, expectedDate, state.DateTime, "twelve camp cycles plus exactly one ordinary completion")
				callbacks.AssertTimeAdvanced(datetime.MinutesPerHour + game_state.DefaultLargeMapMinutesPerTurn)
				for range 3 {
					require.NoError(t, scene.Update(nil))
				}
				require.Equal(t, expectedDate, state.DateTime, "later input frames cannot charge completion again")
				return
			}
			require.False(t, state.EventQueueManager.HasActiveWorkflows())
			require.False(t, scene.inputBlocked)
			require.False(t, callbacks.InputBlocked)
			require.Zero(t, scene.dialogStack.Count())
			require.Nil(t, state.CampingScene)
			require.Nil(t, state.CombatScene)
			require.Equal(t, windOnly.gameState.GetRNG().Calls(), state.GetRNG().Calls(), "setup only consumes independently scheduled wind ticks")
			require.Empty(t, callbacks.TimeAdvanced())
			after, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			after.World.ElapsedMs = before.World.ElapsedMs // Presentation polling advances the injected frame clock.
			after.Random, after.Wind = before.Random, before.Wind
			require.Equal(t, before, after, "cancelled setup changes neither party/resources nor vehicle/world state")
			scene.keyboard.SimulateKeyPress(ebiten.KeyA)
			update()
			require.Equal(t, AttackDirectionInput, scene.secondaryKeyState)
		})
	}
}

func TestDungeonCampingGuardEscapeCleansUpRetainedCycle(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	state.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5}
	update := func() {
		state.ElapsedMs += DialogPollingRateMs
		require.NoError(t, scene.Update(nil))
	}
	scene.keyboard.SimulateKeyPress(ebiten.KeyH)
	update()
	enterCampingDuration(t, scene, update)
	guard := waitForCampingGuard(t, scene, update)
	requireGuardResourceSnapshot(t, state, guard)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	update()
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "dungeon_hole_up")
	require.True(t, state.IsAtDungeonCommandBoundary())
	require.Equal(t, byte(1), state.DateTime.Minute)
	require.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining, "retain established dungeon duration-cycle contract")
	require.Nil(t, state.CampingScene)
	require.Zero(t, scene.dialogStack.Count())
	require.False(t, callbacks.InputBlocked)
}
