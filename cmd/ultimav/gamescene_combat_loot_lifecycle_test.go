package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestCombatLootShutdownCleansDeferredActionOnce(t *testing.T) {
	for _, updates := range []int{0, 1, 4} {
		state, callbacks := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
		require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
		scene := newProductionCastTestScene(state)
		scene.spriteSheet = sharedPresentationTestSpriteSheet()
		token := state.FinishCombatTurnDeferred()
		scene.startCombatChestRevealWorkflow(game_state.CombatOpenResult{RevealItems: []game_state.CombatLootReveal{
			{Sprite: indexes.ItemGem, Description: "a gem!"},
		}}, token)
		for range updates {
			state.ElapsedMs += 1000
			state.EventQueueManager.Update()
		}
		state.EventQueueManager.Shutdown()
		assert.Nil(t, scene.combatLootPreview)
		assert.Zero(t, scene.dialogStack.Count())
		assert.False(t, callbacks.InputBlocked)
		assert.False(t, state.CombatScene.HasScheduledCombatStep())
		identity, date, calls := state.CombatScene.GetCurrentTurnIdentity(), state.DateTime, state.GetRNG().Calls()
		state.EventQueueManager.Shutdown()
		assert.Equal(t, identity, state.CombatScene.GetCurrentTurnIdentity())
		assert.Equal(t, date, state.DateTime)
		assert.Equal(t, calls, state.GetRNG().Calls())
	}
}

func TestCombatPartyTargetAndLootRevealSurviveJournalPause(t *testing.T) {
	for _, loot := range []bool{false, true} {
		state, _ := newProductionCombatCastState(t)
		scene := newProductionCastTestScene(state)
		scene.spriteSheet = sharedPresentationTestSpriteSheet()
		now := time.Unix(100, 0)
		scene.now = func() time.Time { return now }
		scene.startTime = now
		if loot {
			token := state.FinishCombatActionDeferred()
			scene.startCombatChestRevealWorkflow(game_state.CombatOpenResult{RevealItems: []game_state.CombatLootReveal{
				{Sprite: indexes.ItemGem, Description: "a gem!"},
			}}, token)
			state.EventQueueManager.Update()
		} else {
			state.PartyState.Inventory.Spells.Set(references.Mani, 1)
			require.True(t, state.BeginCombatPartySpellTargeting(game_state.CastRequest{
				Spell: references.Mani, CasterIdx: state.CombatScene.GetActiveCombatantIndex(),
			}).Valid)
			scene.secondaryKeyState = CombatSpellTargetInput
		}
		targeting, preview := state.CombatScene.SpellTargeting, scene.combatLootPreview
		elapsed, calls := state.ElapsedMs, state.GetRNG().Calls()
		scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
		for range 3 {
			now = now.Add(10 * time.Second)
			require.NoError(t, scene.Update(nil))
		}
		assert.Equal(t, elapsed, state.ElapsedMs)
		assert.Equal(t, calls, state.GetRNG().Calls())
		assert.Same(t, targeting, state.CombatScene.SpellTargeting)
		assert.Same(t, preview, scene.combatLootPreview)
		scene.closeJournalSession(scene.journalSession, true)
		assert.Nil(t, scene.journalSession)
		if loot {
			state.EventQueueManager.Shutdown()
		} else {
			state.CancelCombatSpellTargeting()
		}
	}
}
