package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestSignHeldDismissalProductionHandoff(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyI, ebiten.KeySpace, ebiten.KeyUp, ebiten.KeyEscape} {
		t.Run(key.String(), func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).WithPlayerAt(10, 10).WithRandomSeed(42).WithSystemCallbacks().Build()
			state.DebugOptions.MonsterGen = false
			state.PartyState.Inventory.Provisions.Torches.Set(3)
			start := state.MapState.PlayerLocation.Position
			target := references.Up.GetNewPositionInDirection(&start)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.BrickFloor)
			scene := newSurfaceCompletionTestScene(state)
			initializeDungeonUpdateTestScene(scene, state)
			state.SystemCallbacks.Visual.BlockAllInput = scene.blockAllInput
			held := make(map[ebiten.Key]bool)
			scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
			game := &Game{currentScene: scene}
			update := func() { state.ElapsedMs += DialogPollingRateMs; require.NoError(t, scene.Update(game)) }
			scene.ShowSignDialog(nil)
			for range 10 {
				update()
				if scene.dialogStack.HasModalDialog() {
					break
				}
			}
			sign, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.SignDialog)
			require.True(t, ok)
			callbacks.Reset()
			held[key] = true
			update()
			require.True(t, sign.IsDismissed())
			for range 10 {
				update()
			}
			require.False(t, state.EventQueueManager.HasWorkflow("sign_dialog_workflow"))
			require.False(t, scene.inputBlocked)
			require.Zero(t, scene.dialogStack.Count())
			require.Equal(t, start, state.MapState.PlayerLocation.Position)
			require.EqualValues(t, 3, state.PartyState.Inventory.Provisions.Torches.Get())
			require.Empty(t, callbacks.TimeAdvanced())
			delete(held, key)
			update()
			held[key] = true
			update()
			switch key {
			case ebiten.KeyI:
				require.EqualValues(t, 2, state.PartyState.Inventory.Provisions.Torches.Get())
			case ebiten.KeySpace:
				require.Len(t, callbacks.TimeAdvanced(), 1)
			case ebiten.KeyUp:
				require.Equal(t, *target, state.MapState.PlayerLocation.Position)
			case ebiten.KeyEscape:
				_, menu := scene.dialogStack.PeekTopModalDialog().(*widgets.ButtonListModal)
				require.True(t, menu)
			}
		})
	}
}

func TestDefaultSignTextIncludesOriginalFrame(t *testing.T) {
	renderer := text.NewSignRenderer(1)

	result := renderer.RenderSign([]byte(defaultSignText))
	t.Cleanup(result.Image.Deallocate)
	width, height := result.Width, result.Height

	if width != 16*text.BitmapCharWidth || height != 6*text.BitmapCharHeight {
		t.Errorf("default sign dimensions = %dx%d, want original 16x6 frame", width, height)
	}
}
