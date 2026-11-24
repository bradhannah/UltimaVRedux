package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestLargeMapLookCompletesOneNormalOutdoorCycle(t *testing.T) {
	for _, floor := range []references.FloorNumber{
		references.FloorNumber(references.OVERWORLD),
		references.FloorNumber(references.UNDERWORLD),
	} {
		for _, open := range []bool{false, true} {
			t.Run(fmt.Sprintf("floor=%d/open=%t", floor, open), func(t *testing.T) {
				state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
					WithLocation(references.BritanniaUnderworld).
					WithFloor(floor).
					WithSystemCallbacks().
					Build()
				state.DebugOptions.MonsterGen = false
				position := state.GetDungeonEntranceCoordinates()[references.Deceit]
				state.MapState.PlayerLocation.Position = position.Down()
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.DoomEntrance)
				state.GetCurrentLayeredMap().UnSetTileByLayer(map_state.MapUnitLayer, &position)
				state.SetDungeonState(references.Deceit, open)
				callbacks.Reset()
				scene := newSurfaceCompletionTestScene(state)
				scene.secondaryKeyState = LookDirectionInput
				scene.keyboard.SimulateKeyPress(ebiten.KeyArrowUp)

				scene.largeMapHandleSecondaryInput()

				want := "Look-Thou dost see the collapsed entrance to the dungeon Deceit"
				if open {
					want = "Look-Thou dost see the entrance to the dungeon Deceit"
				}
				assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
				assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
				assert.Equal(t, []string{want}, callbacks.Messages)
			})
		}
	}
}
