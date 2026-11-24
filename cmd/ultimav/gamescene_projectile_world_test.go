package main

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestWorldEnemyProjectilesRenderThroughProductionWorkflow(t *testing.T) {
	for _, kind := range []references.MonsterType{references.MonsterSeaSerpent, references.MonsterDragon, references.MonsterStocks} {
		for _, blocked := range []bool{false, true} {
			for _, cancel := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/blocked%v/cancel%v", kind.Name(), blocked, cancel), func(t *testing.T) {
					scene, callbacks := newWorldProjectileTestScene(t, kind, blocked)
					state := scene.gameState
					screen := newProjectileRenderTarget(t, scene, 1280, 720, indexes.Water2)
					// Pass completion runs the real outdoor actor phase, including the
					// firing roll, trace, source hit, and serialized pirate workflow.
					state.BeginSurfaceCommandRouting()
					state.ActionPass()
					state.EndSurfaceCommandRouting()
					state.CompleteSurfacePassCycle(references.LargeMapType)
					require.True(t, state.EventQueueManager.HasWorkflow("pirate_attack"))
					hpAfterResolution := state.PartyState.GetCharacter(0).GetCurrentHp()
					if blocked {
						assert.Equal(t, uint16(100), hpAfterResolution)
					} else {
						assert.Less(t, hpAfterResolution, uint16(100))
					}
					rngAfterResolution := state.GetRNG().Calls()
					state.EventQueueManager.Update()
					require.NotNil(t, scene.activeProjectile)
					flight := scene.activeProjectile
					missile := references.MissileRed
					if kind == references.MonsterStocks {
						missile = references.MissileCannonBall
					}
					assert.Equal(t, missile, flight.projectileType)
					assert.Equal(t, float64(xCenter-2), flight.fromTileX, "origin uses wrapped world copy")
					endpointX := float64(xCenter)
					if blocked {
						endpointX--
					}
					assert.Equal(t, endpointX, flight.toTileX)
					assert.Equal(t, flight.fromTileX, flight.currentTileX)
					assertProjectileDrawAtCurrentPosition(t, scene, screen)
					state.ElapsedMs += 50
					state.EventQueueManager.Update()
					require.Same(t, flight, scene.activeProjectile)
					assert.Greater(t, flight.currentTileX, flight.fromTileX)
					assert.Less(t, flight.currentTileX, flight.toTileX)
					assertProjectileDrawAtCurrentPosition(t, scene, screen)
					if cancel {
						state.EventQueueManager.Shutdown()
					} else {
						for range 40 {
							state.ElapsedMs += 50
							state.EventQueueManager.Update()
						}
					}
					assert.Nil(t, scene.activeProjectile)
					assert.False(t, callbacks.InputBlocked)
					assert.False(t, state.EventQueueManager.HasWorkflow("pirate_attack"))
					assert.Equal(t, hpAfterResolution, state.PartyState.GetCharacter(0).GetCurrentHp(), "presentation never replays source damage")
					assert.Equal(t, rngAfterResolution, state.GetRNG().Calls(), "travel/flash/shake consume no gameplay RNG")
				})
			}
		}
	}
}

func newWorldProjectileTestScene(t *testing.T, kind references.MonsterType, blocked bool) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithFloor(0).WithPlayerAt(0, 0).WithSystemCallbacks().Build()
	require.NotNil(t, state)
	state.CurrentNPCAIController = state.GetCurrentLargeMapNPCAIController()
	state.CurrentNPCAIController.RemoveAllEnemies()
	state.DebugOptions.MonsterGen = false
	state.DateTime.Hour, state.DateTime.Minute = 12, 0
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.NoPartyVehicle, references.Position{}, 0)
	for index := range state.PartyState.GetCharacterCount() {
		member := state.PartyState.GetCharacter(index)
		member.PartyMap = party_state.PartyMapAtInn(references.Britain)
		member.SetStatus(party_state.Good)
		member.SetMaxHp(100)
		member.SetHp(100)
		member.SetEquippedItem(references.SlotRing, references.NoEquipment)
	}
	state.PartyState.GetCharacter(0).PartyMap = party_state.PartyMapActive
	for dy := references.Coordinate(-9); dy <= 9; dy++ {
		for dx := references.Coordinate(-9); dx <= 9; dx++ {
			pos := references.Position{X: dx, Y: dy}
			pos = *pos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &pos, indexes.Water2)
		}
	}
	if blocked {
		wall := references.Position{X: 255, Y: 0}
		state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &wall, indexes.StoneBrickWall)
	}
	reference := (*state.GameReferences.EnemyReferences)[kind]
	if kind == references.MonsterStocks {
		reference = references.NewPirateShipEnemyReference(state.GameReferences.TileReferences.GetTile(indexes.PirateShipUp))
	}
	enemy := map_units.NewEnemyNPC(reference, 0)
	enemy.SetPos(references.Position{X: 254, Y: 0})
	enemy.SetFloor(0)
	enemy.SetVisible(true)
	require.True(t, state.CurrentNPCAIController.GetNpcs().AddEnemy(enemy))
	for seed := uint64(0); seed < 10000; seed++ {
		if beradrng.New(seed).Intn(8) == 0 {
			state.SetRandomSeed(seed)
			break
		}
		require.Less(t, seed, uint64(9999), "no deterministic world firing seed")
	}
	scene := &GameScene{gameState: state}
	state.SystemCallbacks.Visual.StartProjectile = scene.startProjectile
	state.SystemCallbacks.Visual.UpdateProjectileProgress = scene.updateProjectileProgress
	state.SystemCallbacks.Visual.ClearProjectile = scene.clearProjectile
	callbacks.Reset()
	return scene, callbacks
}
