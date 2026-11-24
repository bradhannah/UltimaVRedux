package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceJimmyWithoutKeysRejectsBeforeDirectionPrompt(t *testing.T) {
	for _, test := range []struct {
		name     string
		location references.Location
		handle   func(*GameScene, ebiten.Key)
		minutes  int
	}{
		{"town", references.Britain, (*GameScene).smallMapInputHandler, 1},
		{"overworld", references.BritanniaUnderworld, (*GameScene).largeMapInputHandler, game_state.DefaultLargeMapMinutesPerTurn},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithLocation(test.location).
				WithPlayerAt(15, 15).WithSystemCallbacks().Build()
			state.DebugOptions.MonsterGen = false
			state.PartyState.Inventory.Provisions.Keys.Set(0)
			scene := newSurfaceCompletionTestScene(state)
			callbacks.Reset()

			test.handle(scene, ebiten.KeyJ)

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, []string{"No Keys!"}, callbacks.Messages)
			assert.Equal(t, []string{"Jimmy-"}, callbacks.CommandPrompts)
			assert.Zero(t, scene.dialogStack.Count())
			assert.Equal(t, []int{test.minutes}, callbacks.TimeAdvanced())
		})
	}
}

func TestCombatJimmyCallerDoesNotAppendNoTargetForHandledFailure(t *testing.T) {
	for _, test := range []struct {
		name     string
		chest    bool
		locked   bool
		tile     indexes.SpriteIndex
		keys     uint16
		wantKeys uint16
		message  string
	}{
		{"chest no keys", true, true, indexes.BrickFloor, 0, 0, "No keys!"},
		{"chest failed", true, true, indexes.BrickFloor, 1, 0, "Key broke!"},
		{"chest already unlocked", true, false, indexes.BrickFloor, 1, 0, "Key broke!"},
		{"door no keys", false, false, indexes.LockedDoor, 0, 0, "No keys!"},
		{"magical door", false, false, indexes.MagicLockDoor, 1, 0, "Key broke!"},
		{"no target", false, false, indexes.BrickFloor, 1, 1, "Nothing to jimmy!"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, callbacks := newCombatUseTestScene(t)
			state := scene.gameState
			index := state.CombatScene.GetActiveCombatantIndex()
			state.CombatScene.SetPartyMemberPosition(index, references.Position{X: 5, Y: 5})
			target := references.Position{X: 6, Y: 5}
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, test.tile)
			state.PartyState.GetCharacter(index).Dexterity = 1
			state.PartyState.Inventory.Provisions.Keys.Set(test.keys)
			if test.chest {
				state.CombatScene.AddObject(game_state.NewCombatChest(target, 127, test.locked))
			}
			callbacks.Reset()

			scene.combatMapJimmySecondary(references.Right)

			if test.name == "no target" {
				assert.Empty(t, callbacks.Messages)
				assert.Contains(t, scene.output.GetOutputStr(false), test.message)
			} else {
				assert.Equal(t, []string{test.message}, callbacks.Messages)
				assert.NotContains(t, scene.output.GetOutputStr(false), "Nothing to jimmy!")
			}
			assert.Equal(t, test.wantKeys, state.PartyState.Inventory.Provisions.Keys.Get())
		})
	}
}

func TestCombatOpenMagicalDoorUsesLockedFeedback(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	state := scene.gameState
	index := state.CombatScene.GetActiveCombatantIndex()
	state.CombatScene.SetPartyMemberPosition(index, references.Position{X: 5, Y: 5})
	target := references.Position{X: 6, Y: 5}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, indexes.MagicLockDoor)
	callbacks.Reset()

	scene.combatMapOpenSecondary(references.Right)

	assert.Equal(t, []string{"Locked!"}, callbacks.Messages)
}
