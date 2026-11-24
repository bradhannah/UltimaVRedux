package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestCtrlGThenADirectionTakesTheWholePileThroughUpdate(t *testing.T) {
	scene, held, update, _ := newRunicReferenceTestScene(t, references.Britain)
	state := scene.gameState
	position := *references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	for _, item := range []references.ItemAndQuantity{{Item: references.Gem, Quantity: 2}, {Item: references.Gold, Quantity: 30}} {
		require.NoError(t, state.ItemStacksMap.PushUnder(&position, &references.ItemStack{Items: references.ItemStacks{item}}))
	}
	inventory := &state.PartyState.Inventory
	gems, gold := inventory.Provisions.Gems.Get(), inventory.Gold.Get()

	held[ebiten.KeyControlLeft], held[ebiten.KeyG] = true, true
	update()
	require.Equal(t, GetDirectionInput, scene.secondaryKeyState)
	require.True(t, scene.getAll)
	clear(held)
	update()
	held[ebiten.KeyRight] = true
	for range 10 {
		update()
		if scene.secondaryKeyState == PrimaryInput {
			break
		}
	}
	clear(held)
	update()

	require.False(t, state.ItemStacksMap.HasItemStackAtPosition(&position))
	require.Equal(t, gems+2, inventory.Provisions.Gems.Get())
	require.Equal(t, gold+30, inventory.Gold.Get())

	held[ebiten.KeyG] = true
	update()
	require.Equal(t, GetDirectionInput, scene.secondaryKeyState)
	require.False(t, scene.getAll, "a plain G is an ordinary Get again")
}
