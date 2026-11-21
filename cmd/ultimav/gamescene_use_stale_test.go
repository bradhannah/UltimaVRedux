package main

import (
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestCombatUseRejectsChangesAfterSequenceConstruction(t *testing.T) {
	for _, item := range []references.Item{
		references.Crown, references.Yellow, references.ScrollInSanct, references.ScrollKalXenCorp, references.Sceptre,
	} {
		for _, change := range []string{"quantity", "actor", "clone", "encounter", "location"} {
			t.Run(fmt.Sprintf("%T/%v/%s", item, item, change), func(t *testing.T) {
				scene, _ := newStartedCombatPushScene(t)
				clearCombatUseTestInventory(scene.gameState)
				clearCombatUseQuestItems(scene.gameState)
				setStaleUseTestQuantity(scene.gameState, item, 1)
				items := scene.gameState.GetUsableItemsForContext(game_state.UsageCombat)
				require.Len(t, items, 1)
				owner := scene.gameState.CaptureCombatUseOwner()
				origin := scene.gameState.MapState.PlayerLocation
				var confirmation *hmsCapeConfirmationState
				didCommit := false
				sequence := scene.thenUseCombatItem(items[0], &confirmation, func() bool {
					return scene.validateCombatUseSelection(items[0], owner, origin)
				}, &didCommit)
				builder := scene.gameState.EventQueueManager.Workflow("use_item_workflow").
					Priority(event_queue.PriorityNormal).
					Sequential().BlockInput().First(func() {})
				// Construct the production steps now, then change state before their deferred Then runs.
				sequence(builder)
				changeStaleCombatUseState(t, scene, item, change)
				before := snapshotBlockedCombatState(t, scene.gameState)
				commits := scene.gameState.CastUseCommitSequence()
				builder.Start()
				drainDismissingOverlays(t, scene, "use_item_workflow")
				before.ElapsedMs = scene.gameState.ElapsedMs
				require.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
				require.Equal(t, commits, scene.gameState.CastUseCommitSequence())
				require.False(t, didCommit)
				require.False(t, scene.gameState.IsCrownActive())
				require.Equal(t, game_state.DurationEffect{}, scene.gameState.ActiveDurationEffect)
				require.Zero(t, scene.dialogStack.Count())
			})
		}
	}
}

func TestCombatUseDelayedConfirmationRetainsOriginalSelection(t *testing.T) {
	for _, item := range []references.QuestItem{references.HMSCape, references.Sceptre} {
		for _, change := range []string{"quantity", "actor", "clone", "encounter", "location"} {
			t.Run(fmt.Sprintf("%v/%s", item, change), func(t *testing.T) {
				scene, _ := newStartedCombatPushScene(t)
				clearCombatUseTestInventory(scene.gameState)
				clearCombatUseQuestItems(scene.gameState)
				scene.gameState.PartyState.Inventory.QuestItems.Set(item, 1)
				scene.combatMapInputHandler(ebiten.KeyU)
				dialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
				dialog.HandleEnter()
				var confirm func()
				if item == references.HMSCape {
					capeDialog := waitForTopModal[*widgets.InputBox](t, scene)
					confirm = func() { confirmHMSCapeDialog(t, scene, capeDialog) }
				} else {
					waitForCombatSceptreDirectionPrompt(t, scene)
					confirm = func() { resolveCombatSceptreDirection(t, scene, ebiten.KeyArrowDown) }
				}
				changeStaleCombatUseState(t, scene, item, change)
				before := snapshotBlockedCombatState(t, scene.gameState)
				commits := scene.gameState.CastUseCommitSequence()
				confirm()
				drainDismissingOverlays(t, scene, "use_item_workflow")
				before.ElapsedMs = scene.gameState.ElapsedMs
				require.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
				require.Equal(t, commits, scene.gameState.CastUseCommitSequence())
				require.False(t, scene.gameState.IsHMSCapeActivated())
				require.Zero(t, scene.dialogStack.Count())
			})
		}
	}
}

func TestPaidDaemonScrollDoesNotSummonIntoReplacementEncounter(t *testing.T) {
	scene, callbacks := newStartedCombatPushScene(t)
	state := scene.gameState
	clearCombatUseTestInventory(state)
	clearCombatUseQuestItems(state)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollKalXenCorp, 1)
	playSound := state.SystemCallbacks.Audio.PlaySoundEffect
	state.SystemCallbacks.Audio.PlaySoundEffect = func(sound game_state.SoundEffect) {
		playSound(sound)
		if sound == game_state.SoundScrollInvoke {
			// Consumption has happened, but the deferred summon has not resolved yet.
			replacement := *state.CombatScene
			state.CombatScene = &replacement
		}
	}
	rngCalls := state.GetRNG().Calls()
	commits := state.CastUseCommitSequence()
	monsters := state.CombatScene.MonsterManager.Count()
	scene.combatMapInputHandler(ebiten.KeyU)
	waitForTopModal[*widgets.ItemSelectionDialog](t, scene).HandleEnter()
	drainDismissingOverlays(t, scene, "use_item_workflow")
	require.Zero(t, state.PartyState.Inventory.Scrolls.Get(references.ScrollKalXenCorp))
	require.Equal(t, commits+1, state.CastUseCommitSequence(), "confirmed consumption remains paid")
	require.Equal(t, monsters, state.CombatScene.MonsterManager.Count())
	require.Equal(t, rngCalls, state.GetRNG().Calls())
	require.Empty(t, callbacks.CombatEvents)
	require.Nil(t, scene.combatSummonEffect)
	require.Zero(t, scene.dialogStack.Count())
}

func TestCombatWarningPotionStillCommitsAndCanSubstitute(t *testing.T) {
	for _, substitute := range []bool{false, true} {
		scene, _ := newStartedCombatPushScene(t)
		expected, _ := newStartedCombatPushScene(t)
		seed := findPotionRollSeed(t, scene.gameState, func(roll int) bool {
			if substitute {
				return roll == potionTestRandomEffectSleep
			}

			return roll >= 2
		})
		for _, candidate := range []*GameScene{scene, expected} {
			clearCombatUseTestInventory(candidate.gameState)
			clearCombatUseQuestItems(candidate.gameState)
			candidate.gameState.PartyState.Inventory.Potions.Set(references.White, 1)
			candidate.gameState.SetRandomSeed(seed)
		}
		items := scene.gameState.GetUsableItemsForContext(game_state.UsageCombat)
		require.Len(t, items, 1)
		require.False(t, items[0].AvailableNow)
		require.Empty(t, items[0].UnavailableReason)
		require.Contains(t, items[0].Warning, "spent all the same")
		scene.combatMapInputHandler(ebiten.KeyU)
		waitForTopModal[*widgets.ItemSelectionDialog](t, scene).HandleEnter()
		drainDismissingOverlays(t, scene, "use_item_workflow")

		actor := expected.gameState.CombatScene.GetCurrentTurnIdentity()
		commit, _ := expected.gameState.BeginPotionUse(references.White, game_state.UsageCombat)
		outcome, committed := expected.gameState.CommitPotionUse(commit)
		require.True(t, committed)
		effect := expected.gameState.CommitPotionEffect(outcome, actor.EntityIndex)
		if substitute {
			require.Equal(t, game_state.PotionEffectSlept, effect)
			require.True(t, outcome.Mislabelled)
		} else {
			require.Equal(t, game_state.PotionEffectNoNotice, effect)
		}
		expected.gameState.FinishCombatAction()
		expected.gameState.ElapsedMs = scene.gameState.ElapsedMs
		require.Equal(t, snapshotBlockedCombatState(t, expected.gameState), snapshotBlockedCombatState(t, scene.gameState))
		require.Equal(t, expected.gameState.CastUseCommitSequence(), scene.gameState.CastUseCommitSequence())
		require.Zero(t, scene.gameState.PartyState.Inventory.Potions.Get(references.White))
	}
}

func TestCombatUseDoesNotCompleteAnotherCommit(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		scene, _ := newStartedCombatPushScene(t)
		state := scene.gameState
		clearCombatUseTestInventory(state)
		clearCombatUseQuestItems(state)
		state.PartyState.Inventory.Scrolls.Set(references.ScrollInSanct, 2)
		scene.combatMapInputHandler(ebiten.KeyU)
		dialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
		// A separate authoritative operation pays without completing this actor's action.
		require.True(t, state.CommitScrollUse(references.ScrollInSanct))
		before := snapshotBlockedCombatState(t, state)
		commits := state.CastUseCommitSequence()
		if cancel {
			dialog.Dismiss()
		} else {
			dialog.HandleEnter() // The displayed quantity is now stale.
		}
		drainDismissingOverlays(t, scene, "use_item_workflow")
		before.ElapsedMs = state.ElapsedMs
		require.Equal(t, before, snapshotBlockedCombatState(t, state))
		require.Equal(t, commits, state.CastUseCommitSequence())
		require.Equal(t, game_state.DurationEffect{}, state.ActiveDurationEffect)
	}
}

func TestCombatCapeStaleConfirmationDoesNotCompleteAnotherCommit(t *testing.T) {
	scene, _ := newStartedCombatPushScene(t)
	state := scene.gameState
	clearCombatUseTestInventory(state)
	clearCombatUseQuestItems(state)
	state.PartyState.Inventory.QuestItems.Set(references.HMSCape, 1)
	scene.combatMapInputHandler(ebiten.KeyU)
	waitForTopModal[*widgets.ItemSelectionDialog](t, scene).HandleEnter()
	dialog := waitForTopModal[*widgets.InputBox](t, scene)
	externalCommit, _ := state.BeginHMSCapeUse()
	require.Equal(t, game_state.HMSCapeUseEnabled, state.CommitHMSCapeUse(externalCommit))
	before := snapshotBlockedCombatState(t, state)
	commits := state.CastUseCommitSequence()
	confirmHMSCapeDialog(t, scene, dialog)
	drainDismissingOverlays(t, scene, "use_item_workflow")
	before.ElapsedMs = state.ElapsedMs
	require.Equal(t, before, snapshotBlockedCombatState(t, state))
	require.Equal(t, commits, state.CastUseCommitSequence())
	require.True(t, state.IsHMSCapeActivated(), "the stale confirmation must not undo the separate commitment")
}

func TestCombatCapeConfirmedUseCompletesExactlyOneAction(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		scene, _ := newStartedCombatPushScene(t)
		expected, _ := newStartedCombatPushScene(t)
		for _, candidate := range []*GameScene{scene, expected} {
			clearCombatUseTestInventory(candidate.gameState)
			clearCombatUseQuestItems(candidate.gameState)
			candidate.gameState.PartyState.Inventory.QuestItems.Set(references.HMSCape, 1)
			candidate.gameState.SetHMSCapeActivated(enabled)
		}
		scene.combatMapInputHandler(ebiten.KeyU)
		waitForTopModal[*widgets.ItemSelectionDialog](t, scene).HandleEnter()
		confirmHMSCapeDialog(t, scene, waitForTopModal[*widgets.InputBox](t, scene))
		drainDismissingOverlays(t, scene, "use_item_workflow")
		commit, _ := expected.gameState.BeginHMSCapeUse()
		expected.gameState.CommitHMSCapeUse(commit)
		expected.gameState.FinishCombatAction()
		expected.gameState.ElapsedMs = scene.gameState.ElapsedMs
		require.Equal(t, snapshotBlockedCombatState(t, expected.gameState), snapshotBlockedCombatState(t, scene.gameState))
		require.Equal(t, expected.gameState.CastUseCommitSequence(), scene.gameState.CastUseCommitSequence())
		require.Equal(t, !enabled, scene.gameState.IsHMSCapeActivated())
	}
}

func TestCombatSceptreCompletionDistinguishesPaidFailureAndCancellation(t *testing.T) {
	for _, cancel := range []bool{false, true} {
		scene, _ := newStartedCombatPushScene(t)
		expected, _ := newStartedCombatPushScene(t)
		for _, candidate := range []*GameScene{scene, expected} {
			clearCombatUseTestInventory(candidate.gameState)
			clearCombatUseQuestItems(candidate.gameState)
			candidate.gameState.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
		}
		selectCombatSceptre(t, scene)
		key := ebiten.KeyArrowDown
		if cancel {
			key = ebiten.KeySpace
		} else {
			require.False(t, expected.gameState.UseCombatSceptre(references.Down), "no field is a paid failure")
			expected.gameState.FinishCombatAction()
		}
		resolveCombatSceptreDirection(t, scene, key)
		drainDismissingOverlays(t, scene, "use_item_workflow")
		expected.gameState.ElapsedMs = scene.gameState.ElapsedMs
		require.Equal(t, snapshotBlockedCombatState(t, expected.gameState), snapshotBlockedCombatState(t, scene.gameState))
		require.Equal(t, expected.gameState.CastUseCommitSequence(), scene.gameState.CastUseCommitSequence())
	}
}

func changeStaleCombatUseState(t *testing.T, scene *GameScene, item references.Item, change string) {
	t.Helper()
	state := scene.gameState
	switch change {
	case "quantity":
		setStaleUseTestQuantity(state, item, 2)
	case "actor":
		state.FinishCombatAction()
	case "clone":
		actor := state.CombatScene.GetCurrentTurnIdentity()
		require.True(t, state.CastCombatInQuasXen(actor.EntityIndex, actor))
		var clone game_state.CombatTurnIdentity
		state.CombatScene.ForEachPCClone(func(identity game_state.CombatTurnIdentity, _ references.Position) {
			clone = identity
		})
		require.NotZero(t, clone.Slot)
		state.CombatScene.CurrentTurnIdentity = clone
		state.CombatScene.CurrentTurnIndex = clone.EntityIndex
	case "encounter":
		replacement := *state.CombatScene
		state.CombatScene = &replacement
	case "location":
		state.MapState.PlayerLocation.Position.X++
	default:
		t.Fatalf("unknown stale change %q", change)
	}
}

func setStaleUseTestQuantity(state *game_state.GameState, item references.Item, quantity uint16) {
	switch typed := item.(type) {
	case references.QuestItem:
		state.PartyState.Inventory.QuestItems.Set(typed, quantity)
	case references.Potion:
		state.PartyState.Inventory.Potions.Set(typed, quantity)
	case references.Scroll:
		state.PartyState.Inventory.Scrolls.Set(typed, quantity)
	default:
		panic("unsupported stale Use test item")
	}
}
