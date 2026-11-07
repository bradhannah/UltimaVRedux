package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestCommandHelpPrimaryRouteAcrossMapContexts(t *testing.T) {
	for _, location := range []references.Location{references.BritanniaUnderworld, references.Deceit} {
		t.Run(location.String(), func(t *testing.T) {
			state, callbacks := game_state.NewIntegrationTestBuilder(t).WithLocation(location).WithPlayerAt(1, 1).WithRandomSeed(42).WithSystemCallbacks().Build()
			scene := newSurfaceCompletionTestScene(state)
			initializeDungeonUpdateTestScene(scene, state)
			scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return key == ebiten.KeySlash })
			calls := state.GetRNG().Calls()
			require.NoError(t, scene.Update(nil))
			_, help := scene.dialogStack.PeekTopModalDialog().(*widgets.CommandHelpDialog)
			require.True(t, help)
			require.Equal(t, calls, state.GetRNG().Calls())
			require.Empty(t, callbacks.TimeAdvanced())
		})
	}
	t.Run("combat", func(t *testing.T) {
		state, callbacks := newProductionCombatCastState(t)
		scene := newSurfaceCompletionTestScene(state)
		initializeDungeonUpdateTestScene(scene, state)
		scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return key == ebiten.KeySlash })
		identity, calls := state.CombatScene.GetCurrentTurnIdentity(), state.GetRNG().Calls()
		require.NoError(t, scene.Update(nil))
		_, help := scene.dialogStack.PeekTopModalDialog().(*widgets.CommandHelpDialog)
		require.True(t, help)
		require.Equal(t, identity, state.CombatScene.GetCurrentTurnIdentity())
		require.Equal(t, calls, state.GetRNG().Calls())
		require.Empty(t, callbacks.TimeAdvanced())
	})
}

func TestCommandHelpCannotInterceptWorkflowOwnership(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 2, smashedInputHitSeed(1))
	state.EventQueueManager.Workflow("test_owned_input").Priority(event_queue.PriorityNormal).Sequential().BlockInput().First(func() {}).Wait(10000).Start()
	t.Cleanup(state.EventQueueManager.Shutdown)
	require.False(t, scene.handleCommandHelpShortcut(ebiten.KeySlash))
	require.Zero(t, scene.dialogStack.Count())
}

func TestCommandHelpPrimaryRoutePrecedesIntoxication(t *testing.T) {
	for _, shifted := range []bool{false, true} {
		scene, state, callbacks := newSmashedInputTestScene(t, 2, smashedInputHitSeed(1))
		scene.spriteSheet = sharedPresentationTestSpriteSheet()
		held := map[ebiten.Key]bool{ebiten.KeySlash: true, ebiten.KeyShift: shifted}
		scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
		before, err := state.CaptureSaveSnapshot()
		require.NoError(t, err)
		require.NoError(t, scene.Update(nil))
		_, help := scene.dialogStack.PeekTopModalDialog().(*widgets.CommandHelpDialog)
		require.True(t, help, "/ and shifted / must route to Help")
		after, err := state.CaptureSaveSnapshot()
		require.NoError(t, err)
		require.Equal(t, before.Random, after.Random)
		require.Equal(t, before.Effects.Smashed, after.Effects.Smashed)
		require.Equal(t, before.Party, after.Party)
		require.Empty(t, callbacks.TimeAdvanced())
	}
}

func TestCommandHelpDoesNotStealSecondaryDirectionInput(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 2, smashedInputHitSeed(1))
	scene.secondaryKeyState = LookDirectionInput
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return key == ebiten.KeySlash })
	require.NoError(t, scene.Update(nil))
	require.Zero(t, scene.dialogStack.Count())
	require.Equal(t, LookDirectionInput, scene.secondaryKeyState)
	require.Equal(t, 2, captureSmashed(t, state))
	require.Empty(t, callbacks.TimeAdvanced())
}

func TestCommandHelpDetachedContextDescriptions(t *testing.T) {
	for _, context := range []references.GeneralMapType{references.SmallMapType, references.LargeMapType, references.DungeonMapType, references.CombatMapType} {
		entries := commandHelpEntries(context)
		foundJournal, foundMap, foundDirection := false, false, false
		for _, entry := range entries {
			require.NotEmpty(t, entry.Purpose)
			require.True(t, entry.NextInput != "" || entry.Unavailable != "")
			if entry.Key == commandHelpJournalKeys {
				foundJournal = true
			}
			if entry.Key == commandHelpMapKeys {
				foundMap = true
				require.Equal(t, context == references.CombatMapType, entry.Unavailable != "")
			}
			if entry.Key == "Ctrl+N" {
				require.Contains(t, entry.NextInput, "current sextant fix")
				if context == references.LargeMapType {
					require.Empty(t, entry.Unavailable)
				} else {
					require.NotEmpty(t, entry.Unavailable)
				}
			}
			if entry.Key == commandHelpHelpKeys {
				require.Contains(t, entry.NextInput, "Space: Next page")
			}
			if entry.Key == "C" {
				require.Contains(t, entry.NextInput, "Final confirmation spends MP and one mixture.")
				require.Contains(t, entry.NextInput, "Cancelling beforehand costs no MP, mixture, or gameplay time")
				require.NotContains(t, entry.NextInput, "before choice")
				require.NotContains(t, entry.NextInput, "does not refund")
				require.NotContains(t, entry.NextInput, "Cancelling uses the combat action")
				if context == references.CombatMapType {
					require.Contains(t, entry.NextInput, "preserves the combat action")
				}
			}
			if entry.Key == "T" && context == references.CombatMapType {
				require.NotEmpty(t, entry.Unavailable)
			}
			if context == references.DungeonMapType {
				switch entry.Key {
				case "B", "X", "P":
					require.NotEmpty(t, entry.Unavailable)
				case "L", "S":
					require.Contains(t, entry.NextInput, "Up: Ahead, Down: Here")
					require.Contains(t, entry.NextInput, "Space or Esc: Pass")
				}
			}
			if entry.Key == "Y" && context == references.LargeMapType {
				require.Contains(t, entry.NextInput, "toggles sails immediately")
			}
			if entry.Key == "0 / 1-6" && context == references.CombatMapType {
				require.Contains(t, entry.NextInput, "passes the current actor")
			}
			if entry.Key == "A" && context == references.SmallMapType {
				require.Contains(t, entry.NextInput, "Space: Pass")
				foundDirection = true
			}
			if context == references.SmallMapType || context == references.LargeMapType {
				switch entry.Key {
				case "A", "G", "J", "L", "O", "P", "S", "T":
					require.Contains(t, entry.NextInput, "Esc: Cancel without spending a turn")
					require.NotContains(t, entry.NextInput, "not Esc")
				case "K":
					require.Contains(t, entry.NextInput, "Esc cancels an uncommitted direction")
				}
			}
		}
		require.True(t, foundJournal)
		require.True(t, foundMap, "Help lists the Traveller's map")
		if context == references.SmallMapType {
			require.True(t, foundDirection)
		}
	}
}

func TestEscapeMenuHelpRetainsMenuOwner(t *testing.T) {
	scene := newEscapeMenuTestScene()
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.DoEscapeMenu(NewGame(scene))
	menu := requireEscapeMenuDialog[*widgets.ButtonListModal](t, scene)
	for range len(menu.ButtonTexts()) - 1 {
		menu.GoDown()
	}
	pressEscapeMenuWidgetKey(scene, menu, ebiten.KeyEnter)
	help := requireEscapeMenuDialog[*widgets.CommandHelpDialog](t, scene)
	pressEscapeMenuWidgetKey(scene, help, ebiten.KeyEscape)
	require.Same(t, menu, scene.dialogStack.PeekTopModalDialog())
}
