package main

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func TestGuardResourcesProjectionUsesExactGettersForEveryStatus(t *testing.T) {
	scene := newCharacterSelectionProfileScene(t)
	statuses := []party_state.CharacterStatus{
		party_state.Good, party_state.Poisoned, party_state.Sleep,
		party_state.Dead, party_state.Charmed, party_state.Good,
	}
	for index, status := range statuses {
		character := scene.gameState.PartyState.GetCharacter(index)
		copy(character.Name[:], "Companion")
		character.PartyMap = party_state.PartyMapActive
		character.SetMaxHp(65535)
		character.SetHp(uint16(index))
		character.Intelligence = 255
		character.SetMp(byte(index))
		if index == 0 {
			character.Class = party_state.Fighter
			character.SetHp(65535)
			character.SetMp(255)
		}
		if index == 3 {
			character.SetHp(0)
		}
		if index == 2 {
			character.SetMp(0)
		}
		if index == 5 {
			character.Intelligence = 0
			character.SetMp(0)
		}
		character.SetStatus(status)
	}
	options := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionGuard})
	require.Len(t, options, 6)
	for index, option := range options {
		character := scene.gameState.PartyState.GetCharacter(index)
		want := widgets.CharacterSelectionResources{
			CurrentHP: character.GetCurrentHp(), MaximumHP: character.GetMaxHp(),
			CurrentMP: character.GetCurrentMp(), MaximumMP: character.GetMaxMp(),
		}
		require.NotNil(t, option.Resources)
		require.Equal(t, want, *option.Resources)
		require.Equal(t, []widgets.CharacterSelectionFact{
			{Label: "HP", Value: fmt.Sprintf("%d/%d", want.CurrentHP, want.MaximumHP)},
			{Label: "MP", Value: fmt.Sprintf("%d/%d", want.CurrentMP, want.MaximumMP)},
		}, option.Facts, "detail facts and roster share the opening snapshot")
		require.Nil(t, option.Health, "watch resources must not activate healing layout")
		require.Equal(t, statuses[index], option.StatusKind)
		wantState := widgets.CharacterSelectionUnavailable
		if character.IsGood() {
			wantState = widgets.CharacterSelectionAvailable
		} else if character.IsPoisoned() {
			wantState = widgets.CharacterSelectionWarning
		}
		require.Equal(t, wantState, option.State)
		character.SetMaxHp(100)
		character.SetHp(50)
		character.Intelligence = 30
		character.SetMp(20)
		require.Equal(t, want, *option.Resources, "opening projection is detached")
	}
	fresh := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionGuard})
	for _, option := range fresh {
		require.Equal(t, &widgets.CharacterSelectionResources{CurrentHP: 50, MaximumHP: 100, CurrentMP: 20, MaximumMP: 30}, option.Resources)
	}
	require.Nil(t, characterSelectionNoOneOption().Resources)
	ordinary := scene.characterSelectionOptions(characterSelectionContext{Profile: characterSelectionOpen})
	for _, option := range ordinary {
		require.Nil(t, option.Resources)
	}
}

func requireGuardResourceSnapshot(t *testing.T, state *game_state.GameState, dialog *widgets.CharacterSelectionDialog) {
	t.Helper()
	var expectedIndices []int
	state.PartyState.ForEachCharacter(func(index int, character *party_state.PlayerCharacter) {
		if character != nil && character.PartyMap == party_state.PartyMapActive && character.GetNameAsString() != "" {
			expectedIndices = append(expectedIndices, index)
		}
	})
	options := dialog.GetOptions()
	require.Len(t, options, len(expectedIndices)+1)
	for row, option := range options {
		if option.CharacterIndex == game_state.NoGuardAssigned {
			require.Equal(t, len(expectedIndices), row, "No One is appended after the unchanged party order")
			require.Nil(t, option.Resources)
			require.Empty(t, option.Facts)
			require.Empty(t, option.Status)
			require.Equal(t, 0, option.PartyPosition)
			continue
		}
		require.Less(t, row, len(expectedIndices))
		require.Equal(t, expectedIndices[row], option.CharacterIndex)
		character := state.PartyState.GetCharacter(option.CharacterIndex)
		require.Equal(t, character.GetNameAsString(), option.Name)
		require.Equal(t, character.GetStatus(), option.StatusKind)
		require.Equal(t, &widgets.CharacterSelectionResources{
			CurrentHP: character.GetCurrentHp(), MaximumHP: character.GetMaxHp(),
			CurrentMP: character.GetCurrentMp(), MaximumMP: character.GetMaxMp(),
		}, option.Resources)
	}
	require.Equal(t, game_state.NoGuardAssigned, options[len(options)-1].CharacterIndex)
}

func openPhysicalGuardResourceDialog(
	t *testing.T, scene *GameScene, held map[ebiten.Key]bool, update func(), workflow string,
) *widgets.CharacterSelectionDialog {
	t.Helper()
	state := scene.gameState
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyH, func() bool {
		return state.EventQueueManager.HasWorkflow(workflow)
	})
	var duration *widgets.InputBox
	for range 200 {
		update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InputBox); ok {
			duration = dialog
			break
		}
	}
	require.NotNil(t, duration)
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.Key1, func() bool { return duration.GetText() == "1" })
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyEnter, func() bool {
		return !scene.dialogStack.ContainsWidgetInstance(duration)
	})

	return waitForCampingGuard(t, scene, update)
}

func TestGuardResourcesDialogOpeningSnapshotsRefreshOnlyOnReopen(t *testing.T) {
	scene, held, update := newPhysicalCampingInputScene(t, false)
	state := scene.gameState
	character := state.PartyState.GetAvatar()
	character.SetMaxHp(500)
	character.SetHp(123)
	character.Intelligence = 30
	character.SetMp(12)
	open := func() *widgets.CharacterSelectionDialog {
		return openPhysicalGuardResourceDialog(t, scene, held, update, "camping_workflow")
	}
	guard := open()
	before := selectionOption(t, guard.GetOptions(), 0).Resources
	require.Equal(t, &widgets.CharacterSelectionResources{CurrentHP: 123, MaximumHP: 500, CurrentMP: 12, MaximumMP: 30}, before)
	character.SetMaxHp(600)
	character.SetHp(234)
	character.Intelligence = 40
	character.SetMp(23)
	require.Equal(t, before, selectionOption(t, guard.GetOptions(), 0).Resources)
	pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyEscape, func() bool {
		return !scene.dialogStack.ContainsWidgetInstance(guard)
	})
	for range 200 {
		update()
		if !state.EventQueueManager.HasWorkflow("camping_workflow") {
			break
		}
	}
	require.False(t, state.EventQueueManager.HasWorkflow("camping_workflow"))
	fresh := open()
	require.NotSame(t, guard, fresh)
	require.Equal(t, &widgets.CharacterSelectionResources{CurrentHP: 234, MaximumHP: 600, CurrentMP: 23, MaximumMP: 40},
		selectionOption(t, fresh.GetOptions(), 0).Resources)
}

func TestGuardResourcesFullPartyThroughSurfaceAndDungeonInput(t *testing.T) {
	for _, dungeon := range []bool{false, true} {
		t.Run(fmt.Sprintf("dungeon=%t", dungeon), func(t *testing.T) {
			scene, held, update := newPhysicalCampingInputScene(t, dungeon)
			state := scene.gameState
			for index, status := range []party_state.CharacterStatus{
				party_state.Good, party_state.Poisoned, party_state.Sleep,
				party_state.Dead, party_state.Charmed, party_state.Good,
			} {
				character := state.PartyState.GetCharacter(index)
				character.Name = [party_state.NMaxPlayerNameSize]byte{}
				copy(character.Name[:], fmt.Sprintf("Member%d", index+1))
				character.PartyMap = party_state.PartyMapActive
				character.SetMaxHp(100)
				character.SetHp(uint16(10 * index))
				character.Intelligence = byte(5 * index)
				character.SetMp(byte(index))
				if index == 0 {
					character.SetHp(100)
				}
				if status == party_state.Dead {
					character.SetHp(0)
				}
				character.SetStatus(status)
			}
			workflow := "camping_workflow"
			if dungeon {
				workflow = "dungeon_hole_up"
			}
			guard := openPhysicalGuardResourceDialog(t, scene, held, update, workflow)
			require.Len(t, guard.GetOptions(), 7)
			requireGuardResourceSnapshot(t, state, guard)
			pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.Key4, func() bool {
				return guard.GetCurrentSelection() == 3
			})
			require.Equal(t, widgets.CharacterSelectionUnavailable, guard.GetCurrentOption().State)
			require.Same(t, guard, scene.dialogStack.PeekTopModalDialog())
			pressPhysicalCampingKeyUntil(t, scene, held, update, ebiten.KeyEscape, func() bool {
				return !scene.dialogStack.ContainsWidgetInstance(guard)
			})
			for range 200 {
				update()
				if !state.EventQueueManager.HasWorkflow(workflow) {
					break
				}
			}
			require.False(t, state.EventQueueManager.HasWorkflow(workflow))
			require.Nil(t, state.CampingScene)
			require.False(t, scene.inputBlocked)
			require.Zero(t, scene.dialogStack.Count())
			if dungeon {
				require.True(t, state.IsAtDungeonCommandBoundary())
			}
		})
	}
}

func TestDungeonGuardResourcesExplicitChoicesKeepWorkflowOwnership(t *testing.T) {
	for _, choice := range []string{"guard", "no one", "poisoned"} {
		t.Run(choice, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			for index := 0; index < 2; index++ {
				character := state.PartyState.GetCharacter(index)
				character.SetStatus(party_state.Good)
				character.Class = party_state.Fighter
			}
			if choice == "poisoned" {
				state.PartyState.GetCharacter(1).SetStatus(party_state.Poisoned)
			}
			update := func() {
				state.ElapsedMs += DialogPollingRateMs
				require.NoError(t, scene.Update(nil))
			}
			scene.keyboard.SimulateKeyPress(ebiten.KeyH)
			update()
			enterCampingDuration(t, scene, update)
			guard := waitForCampingGuard(t, scene, update)
			requireGuardResourceSnapshot(t, state, guard)
			before := state.DateTime
			selected, key := 0, ebiten.Key1
			if choice == "no one" {
				selected, key = game_state.NoGuardAssigned, ebiten.Key0
			} else if choice == "poisoned" {
				selected, key = 1, ebiten.Key2
			}
			scene.keyboard.SimulateKeyPress(key)
			update()
			require.Equal(t, selected, guard.GetCurrentSelection())
			require.Same(t, guard, scene.dialogStack.PeekTopModalDialog())
			require.Nil(t, state.CampingScene)
			require.Equal(t, before, state.DateTime, "inspection does not charge another minute")
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			update()
			if choice == "poisoned" {
				// The original turns a non-Good guard into No One. ref:CAMP-0001
				selected = game_state.NoGuardAssigned
			}
			for range 100 {
				update()
				if state.CampingScene != nil {
					break
				}
			}
			require.NotNil(t, state.CampingScene)
			require.Equal(t, selected, state.CampingScene.GuardIndex)
			require.False(t, scene.dialogStack.ContainsWidgetInstance(guard))
			anyKeyChecks := 0
			state.SystemCallbacks.Visual.IsAnyKeyPressed = func() bool {
				anyKeyChecks++
				return anyKeyChecks%2 == 0
			}
			for range 2000 {
				state.ElapsedMs += 500
				require.NoError(t, scene.Update(nil))
				if !state.EventQueueManager.HasWorkflow("dungeon_hole_up") {
					break
				}
			}
			require.False(t, state.EventQueueManager.HasWorkflow("dungeon_hole_up"))
			require.True(t, state.IsAtDungeonCommandBoundary())
			require.Zero(t, scene.dialogStack.Count())
			require.False(t, callbacks.InputBlocked)
			require.Nil(t, state.CampingScene)
		})
	}
}
