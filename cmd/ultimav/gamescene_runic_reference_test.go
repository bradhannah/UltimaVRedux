package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func newRunicReferenceTestScene(t *testing.T, location references.Location) (
	*GameScene, map[ebiten.Key]bool, func(), *game_state.MockSystemCallbacks,
) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).WithPlayerAt(1, 1).WithRandomSeed(42).WithSystemCallbacks().Build()
	state.DebugOptions.MonsterGen = false
	scene := newOwnedSurfaceCompletionTestScene(t, state)
	initializeDungeonUpdateTestScene(scene, state)
	state.SystemCallbacks.Visual.BlockAllInput = scene.blockAllInput
	held := make(map[ebiten.Key]bool)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	game := &Game{currentScene: scene}
	update := func() {
		t.Helper()
		state.ElapsedMs += DialogPollingRateMs
		require.NoError(t, scene.Update(game))
	}
	return scene, held, update, callbacks
}

func TestRunicReferenceShortcutCostsNoTurnAndDoesNotReady(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld, references.Deceit} {
		for _, control := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyControlRight} {
			t.Run(location.String()+"/"+control.String(), func(t *testing.T) {
				scene, held, update, callbacks := newRunicReferenceTestScene(t, location)
				before := scene.gameState.DateTime
				position := scene.gameState.MapState.PlayerLocation.Position
				callbacks.Reset()
				held[control], held[ebiten.KeyR] = true, true
				update()
				require.True(t, scene.runicReference.open)
				for range 3 {
					update()
					require.True(t, scene.runicReference.open, "held chord toggles only once")
				}
				delete(held, control)
				update()
				require.Zero(t, scene.dialogStack.Count(), "releasing Ctrl first must not Ready equipment")
				delete(held, ebiten.KeyR)
				update()
				held[control], held[ebiten.KeyR] = true, true
				update()
				require.False(t, scene.runicReference.open)
				require.Equal(t, before, scene.gameState.DateTime)
				require.Equal(t, position, scene.gameState.MapState.PlayerLocation.Position)
				require.Empty(t, callbacks.TimeAdvanced())
				require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			})
		}
	}
}

func TestRunicReferenceSignWorkflowPreservesSourceAndCloses(t *testing.T) {
	scene, held, update, callbacks := newRunicReferenceTestScene(t, references.Britain)
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
	held[ebiten.KeyControlLeft], held[ebiten.KeyR] = true, true
	update()
	require.True(t, scene.runicReference.open)
	require.Same(t, sign, scene.runicReference.owner)
	require.Same(t, sign, scene.dialogStack.PeekTopModalDialog())
	delete(held, ebiten.KeyControlLeft)
	update()
	require.False(t, sign.IsDismissed(), "reference chord cannot dismiss the sign")
	delete(held, ebiten.KeyR)
	update()
	held[ebiten.KeyEscape] = true
	update()
	for range 10 {
		update()
	}
	require.False(t, scene.runicReference.open)
	require.Nil(t, scene.runicReference.owner)
	require.False(t, scene.gameState.EventQueueManager.HasWorkflow("sign_dialog_workflow"))
	require.Empty(t, callbacks.TimeAdvanced())
}

func TestRunicReferenceConversationKeepsTypingAndCloses(t *testing.T) {
	scene, held, update, callbacks := newRunicReferenceTestScene(t, references.SerpentsHold)
	var actor *map_units.NPCFriendly
	for _, unit := range *scene.gameState.CurrentNPCAIController.GetNpcs() {
		if npc, ok := unit.(*map_units.NPCFriendly); ok && npc.MapUnitDetails().NPCNum == 17 {
			actor = npc
			break
		}
	}
	require.NotNil(t, actor)
	dialog := NewLinearTalkDialog(scene, actor)
	scene.PushDialog(dialog)
	held[ebiten.KeyTab] = true
	update()
	delete(held, ebiten.KeyTab)
	update()
	dialog.TextInput.SetText("NA")
	source := dialog.Output.GetOutputStr(true)
	callbacks.Reset()
	held[ebiten.KeyControlRight], held[ebiten.KeyR] = true, true
	update()
	require.True(t, scene.runicReference.open)
	require.Equal(t, "NA", dialog.TextInput.GetText())
	require.Equal(t, source, dialog.Output.GetOutputStr(true))
	delete(held, ebiten.KeyControlRight)
	update()
	require.Equal(t, "NA", dialog.TextInput.GetText())
	delete(held, ebiten.KeyR)
	update()
	held[ebiten.KeyControlRight], held[ebiten.KeyT] = true, true
	update()
	require.Equal(t, "NA", dialog.TextInput.GetText(), "unsupported translation cannot type into Talk")
	delete(held, ebiten.KeyControlRight)
	update()
	require.Equal(t, "NA", dialog.TextInput.GetText())
	delete(held, ebiten.KeyT)
	update()
	for _, key := range []ebiten.Key{ebiten.KeyM, ebiten.KeyE, ebiten.KeyEnter} {
		held[key] = true
		update()
		delete(held, key)
		update()
	}
	require.Contains(t, dialog.currentResponse.Output, "My name is Kristi")
	require.True(t, scene.runicReference.open)
	require.Empty(t, callbacks.TimeAdvanced())
	dialog.Close()
	update()
	require.False(t, scene.runicReference.open)
}

func TestRunicReferenceModalPriorityAndCombatCleanup(t *testing.T) {
	scene, held, update, _ := newRunicReferenceTestScene(t, references.Britain)
	held[ebiten.KeyControlLeft], held[ebiten.KeyR] = true, true
	update()
	require.True(t, scene.runicReference.open)
	clear(held)
	update()
	scene.showCommandHelp()
	help := scene.dialogStack.PeekTopModalDialog()
	require.False(t, scene.runicReferenceAvailable())
	held[ebiten.KeyControlLeft], held[ebiten.KeyR] = true, true
	update()
	require.Same(t, help, scene.dialogStack.PeekTopModalDialog())
	require.True(t, scene.runicReference.open, "unsupported modal temporarily hides the chart")
	scene.dialogStack.MustRemoveWidgetInstance(help)
	require.True(t, scene.runicReferenceAvailable())
	scene.gameState.CombatScene = &game_state.CombatState{}
	update()
	require.False(t, scene.runicReference.open)
	require.False(t, scene.runicReferenceAvailable())
	clear(held)
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetAllowKeyPressImmediately()
	held[ebiten.KeyControlLeft], held[ebiten.KeyR] = true, true
	update()
	require.False(t, scene.runicReference.open, "combat rejects the shortcut")
	require.Zero(t, scene.dialogStack.Count())
}
