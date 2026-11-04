package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Original push_it turns chairs and cannons to face the push direction.
func TestSmallMapPushTurnsFurnitureAwayAndCannonFiresThatWay(t *testing.T) {
	for _, test := range []struct {
		direction     references.Direction
		chair, cannon indexes.SpriteIndex
	}{
		{references.Up, indexes.ChairBackForward, indexes.CannonUp},
		{references.Right, indexes.ChairBackLeft, indexes.CannonRight},
		{references.Down, indexes.ChairBackBack, indexes.CannonDown},
		{references.Left, indexes.ChairBackRight, indexes.CannonLeft},
	} {
		for _, object := range []struct {
			start, floor, want indexes.SpriteIndex
		}{
			{indexes.ChairBackBack, indexes.BrickFloor, test.chair},
			{indexes.CannonUp, indexes.MetalFloor, test.cannon},
		} {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).
				WithPlayerAt(12, 22).WithSystemCallbacks().Build()
			state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
			layer := state.GetCurrentLayeredMap()
			objectPosition := *test.direction.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
			destination := *test.direction.GetNewPositionInDirection(&objectPosition)
			layer.SetTileByLayer(map_state.MapLayer, &objectPosition, object.start)
			layer.SetTileByLayer(map_state.MapLayer, &destination, object.floor)

			require.True(t, state.ActionPushSmallMap(test.direction))

			pushed := layer.GetTileByLayer(map_state.MapLayer, &destination)
			assert.Equal(t, object.want, pushed.Index, "push %v", test.direction)
			if pushed.IsCannon() {
				assert.Equal(t, test.direction, pushed.GetCannonDirection(), "fire %v", test.direction)
			}
		}
	}
}
