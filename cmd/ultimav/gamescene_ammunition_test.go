package main

import (
	"image"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestAmmunitionNoticesRetainBothTransitionsWithoutPerShotNoise(t *testing.T) {
	scene, state := newTransientFeedbackTestScene(t, references.Britain)
	_, scene.output = newMainOutput(1280)
	t.Cleanup(scene.clearTransientActionFeedback)
	inventory := &state.PartyState.Inventory
	inventory.Equipment.Set(references.Arrows, 21)
	inventory.Equipment.Set(references.Quarrels, 35)
	scene.observeProvisionChanges()
	inventory.Equipment.Set(references.Arrows, 20)
	inventory.Equipment.Set(references.Quarrels, 18)
	scene.observeProvisionChanges()
	require.Equal(t, "Arrows running low: 20 left.\nQuarrels running low: 18 left.", scene.output.JournalHistory())
	require.Len(t, scene.transientActionFeedback, 2)
	require.Nil(t, scene.dialogStack.PeekTopModalDialog())
	require.False(t, scene.inputBlocked)
	for range 3 {
		inventory.Equipment.DecrementByOne(references.Arrows)
		scene.observeProvisionChanges()
	}
	require.Len(t, scene.transientActionFeedback, 2, "shots below the threshold only update the row")
	scene.clearTransientActionFeedback()
	inventory.Equipment.Set(references.Arrows, 0)
	inventory.Equipment.Set(references.Quarrels, 0)
	scene.observeProvisionChanges()
	require.Len(t, scene.transientActionFeedback, 2)
	require.True(t, strings.HasSuffix(scene.output.JournalHistory(), "Out of arrows!\nOut of quarrels!"))
	scene.observeProvisionChanges()
	require.Equal(t, 1, strings.Count(scene.output.JournalHistory(), "Out of arrows!"))
	for _, row := range scene.provisionChanges.changes.Rows() {
		require.Empty(t, row.Name, "ammo is not routine Recent Provisions feedback")
	}
}

func TestAmmunitionLastShotNoticeSurvivesAutomaticUnready(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	state := scene.gameState
	scene.output.EnableJournalHistory()
	t.Cleanup(scene.clearTransientActionFeedback)
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotWeapon, references.Bow)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 1)
	scene.resetProvisionChanges()
	require.True(t, state.BeginCombatAttack())
	state.CombatScene.AttackTargeting.Cursor = state.CombatScene.MonsterManager.GetMonster(0).Position()
	result := state.ConfirmCombatAttackTarget()
	require.Equal(t, game_state.CombatAttackSourcePending, result.Outcome)
	require.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
	scene.observeProvisionChanges()
	require.Zero(t, mainscreen.SnapshotAmmunition(&state.PartyState.Inventory)[0])
	require.Equal(t, 1, strings.Count(scene.output.JournalHistory(), "Out of arrows!"))
	scene.observeProvisionChanges()
	require.Len(t, scene.transientActionFeedback, 1)
}

func TestAmmunitionProductionUpdateObservesFinalWorkflowBalanceBeforeModalReturn(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 0, 1)
	scene.output.EnableJournalHistory()
	t.Cleanup(scene.clearTransientActionFeedback)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 21)
	state.PartyState.Inventory.Equipment.Set(references.Quarrels, 21)
	scene.resetProvisionChanges()
	modal := &blockingDebugWorkflowDialog{}
	scene.dialogStack.PushModalDialog(modal)
	state.EventQueueManager.Workflow("ammo_observation_test").Priority(event_queue.PriorityNormal).Sequential().First(func() {
		state.PartyState.Inventory.Equipment.Set(references.Arrows, 0)
		state.PartyState.Inventory.Equipment.Set(references.Arrows, 99)
		state.PartyState.Inventory.Equipment.Set(references.Quarrels, 20)
	}).Start()
	require.NoError(t, scene.Update(nil))
	require.NotContains(t, scene.output.JournalHistory(), "arrows!")
	require.Contains(t, scene.output.JournalHistory(), "Quarrels running low: 20 left.")
	require.Len(t, scene.transientActionFeedback, 1)
	require.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
	require.NoError(t, scene.Update(nil))
	require.Len(t, scene.transientActionFeedback, 1)
}

func TestAmmunitionMerchantRefillRearmsNextCrossing(t *testing.T) {
	fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantArmsBritain)
	scene, state := fixture.scene, fixture.state
	_, scene.output = newMainOutput(1280)
	t.Cleanup(scene.clearTransientActionFeedback)
	state.PartyState.Inventory.Gold.Set(1000)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 18)
	scene.resetProvisionChanges()
	scene.StartTalkWorkflow(fixture.direction)
	shop := waitForMerchantDialog[*widgets.ArmsShopDialog](t, fixture)
	selectArmsEquipment(t, fixture, shop, widgets.ArmsShopBuy, references.Arrows)
	openMerchantModal[*widgets.ArmsTradeDialog](t, fixture, shop, widgets.ArmsShopAwaitingTrade)
	pressMerchantDialogKey(fixture, ebiten.KeyEnter)
	pumpMerchantWorkflowUntil(t, fixture, func() bool {
		return state.PartyState.Inventory.Equipment.Get(references.Arrows) == 99
	})
	scene.observeProvisionChanges()
	require.NotContains(t, scene.output.JournalHistory(), "Arrows running low:")
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 20)
	scene.observeProvisionChanges()
	require.Equal(t, 1, strings.Count(scene.output.JournalHistory(), "Arrows running low: 20 left."))
	state.EventQueueManager.Shutdown()
}

func TestAmmunitionHydrationActivationResetAndPresentationChangesStaySilent(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
		WithPlayerAt(15, 15).WithSystemCallbacks().WithRandomSeed(1).Build()
	scene := &GameScene{gameState: state, gameConfig: state.GameConfig}
	_, scene.output = newMainOutput(1280)
	t.Cleanup(scene.Dispose)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 18)
	scene.observeProvisionChanges()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	loaded, err := game_state.NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen, state.MapState.YTilesVisibleOnGameScreen)
	require.NoError(t, err)
	scene.gameState = loaded
	scene.observeProvisionChanges()
	recaptured, err := loaded.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, snapshot, recaptured, "the observer is read-only and not durable state")
	scene.Activate()
	loaded.PartyState.GetCharacter(0).SetEquippedItem(references.SlotWeapon, references.Bow)
	scene.syncRightRailLayout(image.Rect(0, 0, 1920, 1080))
	scene.observeProvisionChanges()
	loaded.PartyState.GetCharacter(0).SetEquippedItem(references.SlotWeapon, references.NoEquipment)
	scene.PrepareForSceneReplacement()
	scene.observeProvisionChanges()
	require.Empty(t, scene.output.JournalHistory())
	scene.Dispose()
	require.Equal(t, mainscreen.AmmunitionCounts{}, scene.provisionChanges.ammunition)
	loaded.PartyState.Inventory.Equipment.Set(references.Arrows, 0)
	scene.observeProvisionChanges()
	require.Empty(t, scene.output.JournalHistory())
}

func TestAmmunitionReaderCharacterWeaponMapAndResizeDoNotRearmNotice(t *testing.T) {
	scene, _ := newMapNoticeScene(t)
	t.Cleanup(scene.Dispose)
	state := scene.gameState
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 21)
	scene.resetProvisionChanges()
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 20)
	scene.observeProvisionChanges()
	history := scene.output.JournalHistory()
	require.Contains(t, history, "Arrows running low: 20 left.")
	scene.openAtlas(nil)
	require.NoError(t, scene.Update(nil), "reader early-return still brackets the observer")
	scene.closeAtlasSession(true)
	require.True(t, state.SetActivePlayer(1))
	state.PartyState.GetCharacter(1).SetEquippedItem(references.SlotWeapon, references.Bow)
	state.MapState.PlayerLocation.Location = references.Britain
	scene.syncRightRailLayout(image.Rect(0, 0, 3840, 2160))
	scene.observeProvisionChanges()
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 19)
	scene.observeProvisionChanges()
	require.Equal(t, history, scene.output.JournalHistory())
	require.Len(t, scene.transientActionFeedback, 1)
}
