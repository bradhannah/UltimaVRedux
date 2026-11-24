package main

import (
	"fmt"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestSummonInputReturnsWithoutTownCycleAtAllFlames(t *testing.T) {
	for _, ritual := range []struct {
		location references.Location
		floor    references.FloorNumber
		position references.Position
		lord     references.Shadowlord
		shard    references.QuestItem
	}{
		{references.Lycaeum, 2, references.Position{X: 15, Y: 9}, references.Faulinei, references.ShardOfFalsehood},
		{references.EmpathAbbey, 1, references.Position{X: 15, Y: 3}, references.Astaroth, references.ShardOfHatred},
		{references.SerpentsHold, -1, references.Position{X: 15, Y: 16}, references.Nosfentor, references.ShardOfCowardice},
	} {
		for _, vehicle := range []references.VehicleType{references.NoPartyVehicle, references.CarpetVehicle} {
			for _, priorTurns := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/%v/phase%d", ritual.location, vehicle, priorTurns), func(t *testing.T) {
					state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(ritual.location).
						WithFloor(ritual.floor).WithPlayerAt(int(ritual.position.X), int(ritual.position.Y)).
						WithSystemCallbacks().WithRandomSeed(42).Build()
					scene := newSurfaceCompletionTestScene(state)
					scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false })
					_, scene.output = newMainOutput(1280)
					prepareShadowlordSummonPresentationTest(scene)
					state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
					if vehicle == references.CarpetVehicle {
						state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(vehicle, ritual.position, ritual.floor)
					}
					for range priorTurns {
						state.CompleteSurfaceCommandCycle(references.SmallMapType)
					}
					state.ActiveDurationEffect = game_state.DurationEffect{EffectType: game_state.DurationEffectProtection, TurnsRemaining: 8}
					state.PartyState.Inventory.QuestItems.Set(ritual.shard, 1)
					before, err := state.CaptureSaveSnapshot()
					require.NoError(t, err)
					date := state.DateTime
					scene.smallMapInputHandler(ebiten.KeyY)
					dialog := waitForTopModal[*widgets.InputBox](t, scene)
					submitSmallMapYell(t, scene, dialog, strings.ToUpper(ritual.lord.String()))
					// Drain every workflow, including any erroneously deferred completion.
					for range 100 {
						state.ElapsedMs += 100
						state.EventQueueManager.Update()
						state.EventQueueManager.UpdateFeedback()
					}
					require.False(t, state.EventQueueManager.HasActiveWorkflows())
					require.Nil(t, state.CombatScene)
					after, err := state.CaptureSaveSnapshot()
					require.NoError(t, err)
					require.Equal(t, date, state.DateTime)
					require.Equal(t, before.Effects, after.Effects)
					require.Equal(t, before.Random, after.Random)
					enemy := state.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(ritual.position.Up().Up(), ritual.floor)
					require.NotNil(t, enemy)
					scene.smallMapInputHandler(ebiten.KeySpace)
					require.Nil(t, state.CombatScene, "moving adjacent is not an attack in the same actor step")
					require.Same(t, enemy, state.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(ritual.position.Up(), ritual.floor))
					commit, review := state.BeginShardRitualCommit(ritual.shard)
					require.Equal(t, game_state.ShardUsageSuccess, review.Result)
					require.Equal(t, game_state.ShardUsageSuccess, state.CommitShardRitual(commit).Result)
					require.False(t, state.ShadowlordState.IsAlive(ritual.lord))
					state.EventQueueManager.Shutdown()
				})
			}
		}
	}
}

func TestSummonPreservesTemporalCadenceUntilRealPass(t *testing.T) {
	for _, effect := range []game_state.DurationEffectType{game_state.DurationEffectTimeStop, game_state.DurationEffectQuickness} {
		scene, _ := newShardWorkflowTestScene(t)
		scene.keyboard = newPresentationTestKeyboard()
		prepareShadowlordSummonPresentationTest(scene)
		state := scene.gameState
		state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
		state.ActiveDurationEffect = game_state.DurationEffect{EffectType: effect, TurnsRemaining: 8}
		date, calls := state.DateTime, state.GetRNG().Calls()
		scene.smallMapInputHandler(ebiten.KeyY)
		submitSmallMapYell(t, scene, waitForTopModal[*widgets.InputBox](t, scene), "FAULINEI")
		for range 100 {
			state.ElapsedMs += 100
			state.EventQueueManager.Update()
			state.EventQueueManager.UpdateFeedback()
		}
		require.Equal(t, date, state.DateTime)
		require.Equal(t, calls, state.GetRNG().Calls())
		require.Equal(t, 8, state.ActiveDurationEffect.TurnsRemaining)
		spawn := references.Position{X: 15, Y: 7}
		enemy := state.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(spawn, 2)
		require.NotNil(t, enemy)
		scene.smallMapInputHandler(ebiten.KeySpace)
		require.Same(t, enemy, state.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(spawn, 2))
		state.EventQueueManager.Shutdown()
		require.Equal(t, 7, state.ActiveDurationEffect.TurnsRemaining, "shutdown must not manufacture another turn")
	}
}
