package main

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQuickLoadReplacementProtectsHeldChordAndFreshQStillSaves(t *testing.T) {
	for _, firstReleased := range []ebiten.Key{ebiten.KeyQ, ebiten.KeyControlLeft} {
		t.Run(firstReleased.String(), func(t *testing.T) {
			old, held, _, _ := newRunicReferenceTestScene(t, references.Deceit)
			candidate, _, _, _ := newRunicReferenceTestScene(t, references.Britain)
			setQuickSaveConfirmationForTest(candidate, false)
			candidate.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
			setQuickSaveConfirmationForTest(old, false)
			game, repository := quickLoadGameForTest(old, candidate)
			builds := 0
			game.playerSaveSceneBuilder = func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error) {
				builds++
				return candidate, nil
			}
			held[ebiten.KeyQ], held[ebiten.KeyControlLeft], held[ebiten.KeyCapsLock] = true, true, true
			require.NoError(t, old.Update(game))
			require.Same(t, candidate, game.currentScene)
			assert.Contains(t, candidate.output.GetOutputStr(true), "Loaded Slot 6.")
			assert.NotContains(t, old.output.GetOutputStr(true), "Loaded Slot 6.")
			require.Zero(t, candidate.dialogStack.Count())
			for range 3 {
				require.NoError(t, candidate.Update(game))
			}
			delete(held, firstReleased)
			for range 3 {
				require.NoError(t, candidate.Update(game))
			}
			// Repress Q while Ctrl is still carried: not a second shortcut.
			if firstReleased == ebiten.KeyQ {
				held[ebiten.KeyQ] = true
				require.NoError(t, candidate.Update(game))
			}
			assert.Equal(t, 1, builds)
			assert.Len(t, repository.loads, 1)
			assert.Empty(t, repository.saves)
			delete(held, ebiten.KeyControlLeft)
			delete(held, ebiten.KeyQ)
			require.NoError(t, candidate.Update(game))
			held[ebiten.KeyQ] = true
			require.NoError(t, candidate.Update(game))
			require.Len(t, repository.saves, 1, "new plain Q keeps its independent save route")
			assert.Equal(t, player_save.Slot06, repository.saves[0].slot)
			assert.Zero(t, candidate.dialogStack.Count())
			for range 3 {
				require.NoError(t, candidate.Update(game))
			}
			assert.Len(t, repository.saves, 1, "held plain Q cannot repeat")
			clear(held)
			candidate.Dispose()
		})
	}
}

func TestQuickLoadNoSlotFailureAndHeldReleaseRecovery(t *testing.T) {
	for _, firstReleased := range []ebiten.Key{ebiten.KeyQ, ebiten.KeyControlRight} {
		t.Run(firstReleased.String(), func(t *testing.T) {
			scene, held, _, callbacks := newRunicReferenceTestScene(t, references.Deceit)
			game, repository := quickLoadGameForTest(scene, &playerSaveTestScene{})
			scene.activePlayerSaveSlot, scene.activePlayerSaveRevision = "", ""
			before, calls := scene.gameState.DateTime, scene.gameState.GetRNG().Calls()
			held[ebiten.KeyControlRight], held[ebiten.KeyQ] = true, true
			require.NoError(t, scene.Update(game))
			assert.Contains(t, callbacks.Messages, "Load failed: Save or load a game first.")
			require.Zero(t, scene.dialogStack.Count())
			delete(held, firstReleased)
			held[ebiten.KeyEnter] = true
			for range 3 {
				require.NoError(t, scene.Update(game))
				require.Zero(t, scene.dialogStack.Count())
			}
			clear(held)
			require.NoError(t, scene.Update(game))
			assert.Equal(t, before, scene.gameState.DateTime)
			assert.Equal(t, calls, scene.gameState.GetRNG().Calls())
			assert.Empty(t, repository.loads)
			assert.Empty(t, repository.saves)
			held[ebiten.KeyQ] = true
			require.NoError(t, scene.Update(game))
			assert.Equal(t, DungeonQuitInput, scene.secondaryKeyState)
			assert.NotNil(t, scene.saveQuestionDialog)
		})
	}
}

func TestQuickLoadCannotEnterCombatTargetingOrEnableCombatQSave(t *testing.T) {
	for _, confirm := range []bool{true, false} {
		t.Run(boolLabel(confirm), func(t *testing.T) {
			scene, held, callbacks := newCombatDirectionUpdateScene(t, "first_actor")
			setQuickSaveConfirmationForTest(scene, confirm)
			game, repository := quickLoadGameForTest(scene, &playerSaveTestScene{})
			held[ebiten.KeyA] = true
			require.NoError(t, scene.Update(game))
			require.Equal(t, CombatAttackTargetInput, scene.secondaryKeyState)
			clear(held)
			require.NoError(t, scene.Update(game))
			before := snapshotBlockedCombatState(t, scene.gameState)
			held[ebiten.KeyControlLeft], held[ebiten.KeyQ] = true, true
			require.NoError(t, scene.Update(game))
			assert.Empty(t, repository.loads)
			assert.Equal(t, CombatAttackTargetInput, scene.secondaryKeyState)
			assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
			clear(held)
			require.NoError(t, scene.Update(game))
			held[ebiten.KeyEscape] = true
			require.NoError(t, scene.Update(game))
			require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			clear(held)
			require.NoError(t, scene.Update(game))
			callbacks.Reset()
			before = snapshotBlockedCombatState(t, scene.gameState)
			held[ebiten.KeyQ] = true
			require.NoError(t, scene.Update(game))
			assert.Equal(t, "Quit-Not here", callbacks.CurrentRowContent)
			assert.Empty(t, repository.saves)
			assert.Empty(t, repository.loads)
			assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
			assert.True(t, scene.gameState.CombatScene.CombatTurnsStarted())
		})
	}
}

func TestQuickLoadPolicyDoesNotAcceptHostileCombatActors(t *testing.T) {
	scene, _, _ := newCombatDirectionUpdateScene(t, "first_actor")
	combat := scene.gameState.CombatScene
	identity := combat.GetCurrentTurnIdentity()
	require.True(t, scene.quickLoadInputEligible())
	combat.PartyPossessed[identity.EntityIndex] = true
	assert.False(t, scene.quickLoadInputEligible())
	combat.PartyPossessed[identity.EntityIndex] = false
	combat.CurrentTurnIdentity = game_state.CombatTurnIdentity{IsPartyMember: false, EntityIndex: 0}
	assert.False(t, scene.quickLoadInputEligible(), "ordinary hostile monster is not a player-controlled boundary")
}
