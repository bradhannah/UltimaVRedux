package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSkiffMovementEntersAndLeavesRiverBridge(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
		for _, test := range []struct {
			name      string
			bridge    indexes.SpriteIndex
			direction references.Direction
			key       ebiten.Key
		}{
			{"north under horizontal bridge", indexes.TrollBridgeHoriz, references.Up, ebiten.KeyUp},
			{"south under horizontal bridge", indexes.TrollBridgeHoriz, references.Down, ebiten.KeyDown},
			{"west under vertical bridge", indexes.TrollBridgeVert, references.Left, ebiten.KeyLeft},
			{"east under vertical bridge", indexes.TrollBridgeVert, references.Right, ebiten.KeyRight},
		} {
			t.Run(location.String()+"/"+test.name, func(t *testing.T) {
				state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(location).
					WithPlayerAt(15, 15).WithSystemCallbacks().WithRandomSeed(42).Build()
				state.DebugOptions.MonsterGen = false
				state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
				origin := state.MapState.PlayerLocation.Position
				bridge := *test.direction.GetNewPositionInDirection(&origin)
				destination := *test.direction.GetNewPositionInDirection(&bridge)
				layer := state.GetCurrentLayeredMap()
				layer.SetTileByLayer(map_state.MapOverrideLayer, &origin, indexes.Water2)
				layer.SetTileByLayer(map_state.MapOverrideLayer, &bridge, test.bridge)
				layer.SetTileByLayer(map_state.MapOverrideLayer, &destination, indexes.Water2)
				state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.SkiffVehicle, origin, 0)
				scene := newSurfaceCompletionTestScene(state)
				first := scene.handleMovement(test.direction.GetDirectionCompassName(), test.key)
				require.True(t, first.MovementOccurred)
				assert.Equal(t, bridge, state.MapState.PlayerLocation.Position)
				assert.False(t, state.EventQueueManager.HasWorkflow("bridge_troll_encounter"))
				second := scene.handleMovement(test.direction.GetDirectionCompassName(), test.key)
				require.True(t, second.MovementOccurred)
				assert.Equal(t, destination, state.MapState.PlayerLocation.Position)
				assert.Equal(t, references.SkiffVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)
			})
		}
	}
}
