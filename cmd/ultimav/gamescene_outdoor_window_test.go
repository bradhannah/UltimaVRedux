package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func prepareSceneBlinkRay(state *game_state.GameState, grass ...references.Coordinate) {
	layer := state.GetCurrentLayeredMap()
	for x := references.Coordinate(0); x < 256; x++ {
		position := references.Position{X: x, Y: 120}
		layer.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.BrickFloor)
	}
	for _, x := range grass {
		position := references.Position{X: x, Y: 120}
		layer.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.Grass)
	}
}

func TestOutdoorWindowProductionMovementAndCameraIndependence(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		for _, vehicleType := range []references.VehicleType{references.NoPartyVehicle, references.HorseVehicle, references.CarpetVehicle} {
			for _, smooth := range []bool{false, true} {
				scene := newLargeMapCameraTestScene(t, floor, references.Position{X: 119, Y: 120})
				state := scene.gameState
				prepareSceneBlinkRay(state, 100, 124, 140)
				scene.gameConfig.GameplayConfiguration.SmoothSurfaceMovement = smooth
				state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
				if vehicleType != references.NoPartyVehicle {
					state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicleType, state.MapState.PlayerLocation.Position, floor)
				}
				scene.largeMapInputHandler(ebiten.KeyRight)
				require.Equal(t, references.Position{X: 120, Y: 120}, state.MapState.PlayerLocation.Position)
				for range 5 {
					state.ElapsedMs += 50
					scene.updateSurfaceCamera()
				}
				require.Equal(t, game_state.BlinkSuccess, state.BlinkTeleport(references.Right))
				assert.Equal(t, references.Position{X: 124, Y: 120}, state.MapState.PlayerLocation.Position)
			}
		}
	}
}

func TestOutdoorWindowBlockedProductionStepPreservesHistory(t *testing.T) {
	scene := newLargeMapCameraTestScene(t, 0, references.Position{X: 119, Y: 120})
	state := scene.gameState
	prepareSceneBlinkRay(state, 100, 124, 140)
	state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	for range 3 {
		scene.moveToNewPositionByDirection(references.Right)
	}
	blocked := references.Position{X: 123, Y: 120}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &blocked, indexes.TallMountains)
	scene.largeMapInputHandler(ebiten.KeyRight)
	require.Equal(t, references.Position{X: 122, Y: 120}, state.MapState.PlayerLocation.Position)
	require.Equal(t, game_state.BlinkSuccess, state.BlinkTeleport(references.Right))
	assert.Equal(t, references.Position{X: 124, Y: 120}, state.MapState.PlayerLocation.Position)
}

func TestOutdoorWindowAutomaticSailingMovesOnlyOnActualStep(t *testing.T) {
	fixture := newAutomaticSailingWorkflowFixture(t, environment.North)
	state := fixture.state
	state.SetPlayerPosition(references.Position{X: 119, Y: 120})
	state.InitializeOutdoorWindow()
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	prepareSceneBlinkRay(state, 100, 124, 140)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &references.Position{X: 120, Y: 120}, indexes.Water2)
	fixture.scene.largeMapInputHandler(ebiten.KeyRight)
	require.Equal(t, references.Position{X: 119, Y: 120}, state.MapState.PlayerLocation.Position)
	fixture.advancePhases(1)
	require.Equal(t, references.Position{X: 120, Y: 120}, state.MapState.PlayerLocation.Position)
	require.Equal(t, game_state.BlinkSuccess, state.BlinkTeleport(references.Right))
	assert.Equal(t, references.Position{X: 124, Y: 120}, state.MapState.PlayerLocation.Position)
	state.EventQueueManager.Shutdown()
}

func TestOutdoorWindowProductionCastCostsCancellationAndFailureText(t *testing.T) {
	for _, floor := range []references.FloorNumber{0, -1} {
		for _, outcome := range []string{"success", "failure", "Escape", "Space"} {
			t.Run(outcome, func(t *testing.T) {
				scene, callbacks := newSurfaceUseOwnershipSceneAt(t, references.BritanniaUnderworld, floor, references.Position{X: 119, Y: 120})
				state := scene.gameState
				state.CurrentNPCAIController = state.GetCurrentLargeMapNPCAIController()
				state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
				state.MovePlayerOneStep(references.Right)
				prepareSceneBlinkRay(state, 100, 140)
				if outcome == "success" {
					state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &references.Position{X: 124, Y: 120}, indexes.Grass)
				}
				require.True(t, state.SetActivePlayer(0))
				caster := state.PartyState.GetCharacter(0)
				caster.Class, caster.Level = party_state.Wizard, 8
				caster.SetMp(8)
				state.PartyState.Inventory.Spells.Set(references.InPor, 2)
				before, calls := state.DateTime, state.GetRNG().Calls()
				callbacks.Reset()
				scene.startSpellCastingWorkflow()
				dialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
				enterSpellSyllables(scene, dialog, []ebiten.Key{ebiten.KeyI, ebiten.KeyP})
				scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
				dialog.Update()
				for range 20 {
					state.ElapsedMs += DialogPollingRateMs
					state.EventQueueManager.Update()
					if scene.castUseDirectionDialog != nil {
						break
					}
				}
				require.NotNil(t, scene.castUseDirectionDialog)
				assert.Equal(t, byte(8), caster.GetCurrentMp())
				assert.Equal(t, uint16(2), state.PartyState.Inventory.Spells.Get(references.InPor))
				key := ebiten.KeyRight
				if outcome == "Escape" {
					key = ebiten.KeyEscape
				} else if outcome == "Space" {
					key = ebiten.KeySpace
				}
				scene.keyboard.SimulateKeyPress(key)
				paid := outcome == "success" || outcome == "failure"
				if paid {
					var feedback *widgets.OverlayDialog
					for range 30 {
						state.ElapsedMs += 1000
						state.EventQueueManager.Update()
						feedback, _ = scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
						if feedback != nil {
							break
						}
					}
					require.NotNil(t, feedback)
					if outcome == "failure" {
						assert.Equal(t, "Failed!", feedback.GetMessage())
					} else {
						assert.Equal(t, "Teleport!", feedback.GetMessage())
					}
				}
				drainProductionCastWorkflow(t, scene)
				if paid {
					assert.Equal(t, byte(5), caster.GetCurrentMp())
					assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InPor))
					assert.Equal(t, uint64(1), state.CastUseCommitSequence())
					assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
				} else {
					assert.Equal(t, byte(8), caster.GetCurrentMp())
					assert.Equal(t, uint16(2), state.PartyState.Inventory.Spells.Get(references.InPor))
					assert.Zero(t, state.CastUseCommitSequence())
					assert.Equal(t, before, state.DateTime)
					assert.Equal(t, calls, state.GetRNG().Calls())
					assert.Empty(t, callbacks.TimeAdvanced())
				}
				if outcome == "success" {
					assert.Equal(t, references.Position{X: 124, Y: 120}, state.MapState.PlayerLocation.Position)
				} else {
					assert.Equal(t, references.Position{X: 120, Y: 120}, state.MapState.PlayerLocation.Position)
					require.Equal(t, game_state.BlinkSuccess, state.BlinkTeleport(references.Left))
					assert.Equal(t, references.Position{X: 100, Y: 120}, state.MapState.PlayerLocation.Position)
				}
				assert.Zero(t, scene.dialogStack.Count())
			})
		}
	}
}
