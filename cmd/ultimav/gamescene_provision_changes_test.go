package main

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestProvisionFeedbackRetainsTypographyAcrossUpdateAndDraw(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	for _, size := range []image.Point{{1280, 720}, {1281, 800}, {1788, 1033}, {1500, 1000}, {1366, 768}} {
		t.Run(size.String(), func(t *testing.T) {
			scene, state, _ := newSmashedInputTestScene(t, 0, 1)
			game := &Game{currentScene: scene}
			width, height := game.Layout(size.X, size.Y)
			screen := newPresentationRaster(width, height)
			defer screen.Deallocate()
			modal := &blockingDebugWorkflowDialog{}
			scene.dialogStack.PushModalDialog(modal)
			scene.resetProvisionChanges()
			for _, visible := range []bool{false, true} {
				if visible {
					state.PartyState.Inventory.Gold.IncrementByOne()
				}
				require.NoError(t, scene.Update(nil))
				prepared := scene.provisionChanges.panel
				rows := scene.provisionChanges.changes.Rows()
				for range 3 {
					scene.drawDialogs(screen)
					require.True(t, prepared == scene.provisionChanges.panel,
						"Draw must retain fonts, outputs and placement at a fixed logical size")
					require.NoError(t, scene.Update(nil))
					require.True(t, prepared == scene.provisionChanges.panel,
						"Update must use the same logical dimensions as Draw")
					assert.Equal(t, rows, scene.provisionChanges.changes.Rows())
					assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
				}
			}
		})
	}
}

func TestProvisionFeedbackBritainRationsActualCommitWhileMerchantOpen(t *testing.T) {
	for _, test := range []struct {
		name       string
		food, gold uint16
		packs      int
		cancel     bool
	}{
		{name: "purchase two packs", food: 100, gold: 5000, packs: 2},
		{name: "capped purchase", food: 9997, gold: 5000, packs: 1},
		{name: "full food still charges committed price", food: 9999, gold: 5000, packs: 1},
		{name: "failed payment", food: 3, gold: 0, packs: 1},
		{name: "cancel approval", food: 100, gold: 5000, packs: 1, cancel: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := newMerchantTalkFixture(t, references.Britain, references.MerchantTavernBritain)
			inventory := &fixture.state.PartyState.Inventory
			inventory.Provisions.Food.Set(test.food)
			inventory.Gold.Set(test.gold)
			fixture.scene.observeProvisionChanges()
			assertNoSceneProvisionChanges(t, fixture.scene)
			review, err := fixture.state.ReviewTavernShopByMerchantID(references.MerchantTavernBritain)
			require.NoError(t, err)
			fixture.scene.StartTalkWorkflow(fixture.direction)
			dialog := waitForMerchantDialog[*widgets.TavernShopDialog](t, fixture)
			selectTavernOrderItem(t, fixture, dialog, widgets.TavernOrderProvision)
			pressMerchantDialogKey(fixture, ebiten.KeyEnter)
			approval := waitForMerchantDialog[*widgets.TavernActionDialog](t, fixture)
			for pack := 1; pack < test.packs; pack++ {
				pressMerchantDialogKey(fixture, ebiten.KeyRight)
			}
			require.Equal(t, test.packs, approval.GetProvisionPackCount())
			if test.cancel {
				pressMerchantDialogKey(fixture, ebiten.KeyEscape)
				pumpMerchantWorkflowUntil(t, fixture, func() bool {
					return fixture.scene.dialogStack.PeekTopModalDialog() == dialog
				})
			} else {
				pressMerchantDialogKey(fixture, ebiten.KeyEnter)
				waitForMerchantDialog[*widgets.TavernResultDialog](t, fixture)
			}
			top := fixture.scene.dialogStack.PeekTopModalDialog()
			count := fixture.scene.dialogStack.Count()
			rngCalls := fixture.state.GetRNG().Calls()
			fixture.scene.observeProvisionChanges()
			rows := fixture.scene.provisionChanges.changes.Rows()
			if test.cancel || test.gold == 0 {
				assertNoSceneProvisionChanges(t, fixture.scene)
				assert.Equal(t, test.food, inventory.Provisions.Food.Get())
				assert.Equal(t, test.gold, inventory.Gold.Get())
			} else {
				wantFood := min(int(party_state.ItemQuantityLargeMax-test.food), test.packs*int(review.State.Provision.Provision.PackQuantity))
				wantGold := test.packs * review.State.Provision.PricePerPack
				if wantFood == 0 {
					assert.Empty(t, rows[0].Name, "zero actual receipt must not claim food gained")
				} else {
					assert.Equal(t, "Food", rows[0].Name)
					assert.Equal(t, rows[0].ExpiresMs, rows[1].ExpiresMs, "transaction shares a readable window")
				}
				assert.Equal(t, uint64(wantFood), rows[0].Gained)
				assert.Zero(t, rows[0].Lost)
				assert.Equal(t, inventory.Provisions.Food.Get(), rows[0].Total)
				assert.Equal(t, "Gold", rows[1].Name)
				assert.Equal(t, uint64(wantGold), rows[1].Lost)
				assert.Zero(t, rows[1].Gained)
				assert.Equal(t, inventory.Gold.Get(), rows[1].Total)
			}
			fixture.scene.drawDialogs(ebiten.NewImage(1280, 720))
			assert.True(t, fixture.scene.provisionChangesAboveDialogs(), "merchant parent and result modal keep provision feedback visible")
			assert.Equal(t, rngCalls, fixture.state.GetRNG().Calls())
			assert.Empty(t, fixture.callbacks.TimeAdvanced(), "feedback cannot complete Talk or advance time")
			assert.Equal(t, count, fixture.scene.dialogStack.Count())
			assert.Same(t, top, fixture.scene.dialogStack.PeekTopModalDialog(), "feedback cannot take focus")
			assert.False(t, dialog.IsDismissed(), "purchase remains readable during the merchant interaction")
			fixture.state.EventQueueManager.Shutdown()
		})
	}
}

func TestProvisionFeedbackProductionUpdateObservesQueueBeforeModalReturn(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 0, 1)
	state.PartyState.Inventory.Gold.Set(100)
	scene.resetProvisionChanges()
	modal := &blockingDebugWorkflowDialog{}
	scene.dialogStack.PushModalDialog(modal)
	state.EventQueueManager.Workflow("provision_observation_test").Priority(event_queue.PriorityNormal).Sequential().First(func() {
		state.PartyState.Inventory.Gold.IncrementBy(20)
		state.PartyState.Inventory.Gold.DecrementBy(20)
		state.PartyState.Inventory.Provisions.Food.IncrementBy(10)
	}).Start()
	require.NoError(t, scene.Update(nil))
	rows := scene.provisionChanges.changes.Rows()
	assert.Equal(t, uint64(20), rows[1].Gained)
	assert.Equal(t, uint64(20), rows[1].Lost)
	assert.Equal(t, uint16(100), rows[1].Total)
	assert.Equal(t, uint64(10), rows[0].Gained)
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, 1, scene.dialogStack.Count())
	state.ElapsedMs += mainscreen.ProvisionChangeHoldMs
	require.NoError(t, scene.Update(nil))
	assertNoSceneProvisionChanges(t, scene)
	assert.Same(t, modal, scene.dialogStack.PeekTopModalDialog())
	scene.dialogStack.MustRemoveWidgetInstance(modal)
	state.ElapsedMs += mainscreen.ProvisionChangeHoldMs + 1
	require.NoError(t, scene.Update(nil))
	assertNoSceneProvisionChanges(t, scene)
}

func TestProvisionFeedbackRealCappedPickupsAllSixResources(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithFloor(0).WithPlayerAt(50, 50).
		WithSystemCallbacks().WithRandomSeed(1).Build()
	state.DateTime.Hour = 9
	state.DateTime.Minute = 0
	scene := &GameScene{gameState: state}
	inventory := &state.PartyState.Inventory
	quantities := []party_state.ItemQuantity{
		&inventory.Provisions.Food, &inventory.Gold, &inventory.Provisions.Keys,
		&inventory.Provisions.Gems, &inventory.Provisions.Torches, &inventory.Provisions.SkullKeys,
	}
	identities := []references.Provision{
		references.Food, references.Gold, references.Key, references.Gem, references.Torches, references.SkullKeys,
	}
	for _, quantity := range quantities {
		quantity.Set(65535)
		quantity.DecrementByOne()
	}
	scene.observeProvisionChanges()
	position := state.MapState.PlayerLocation.Position.Right()
	for index, identity := range identities {
		require.NoError(t, state.ItemStacksMap.Push(&position, &references.ItemStack{Items: references.ItemStacks{
			{Item: identity, Quantity: 10},
		}}))
		require.True(t, state.ActionGetLargeMap(references.Right))
		scene.observeProvisionChanges()
		row := scene.provisionChanges.changes.Rows()[index]
		assert.Equal(t, uint64(1), row.Gained)
		assert.Zero(t, row.Lost)
		assert.Equal(t, quantities[index].Get(), row.Total)
		assert.Equal(t, uint16(9), state.ItemStacksMap.Peek(&position).Quantity, "uncollected overflow remains on the ground")
		assert.False(t, state.ActionGetLargeMap(references.Right), "zero capacity rejects the retained remainder")
		scene.observeProvisionChanges()
		assert.Equal(t, row, scene.provisionChanges.changes.Rows()[index], "rejected remainder creates no provision activity")
		state.ItemStacksMap.Pop(&position) // Clear this verified fixture before placing the next resource.
	}
	before := scene.provisionChanges.changes.Rows()
	assert.False(t, state.ActionGetLargeMap(references.Right))
	scene.observeProvisionChanges()
	assert.Equal(t, before, scene.provisionChanges.changes.Rows(), "empty pickup does not refresh or invent changes")
}

func TestProvisionFeedbackRealConsumptionAndEmptyFailures(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithPlayerAt(15, 15).
		WithSystemCallbacks().WithRandomSeed(1).Build()
	state.DateTime.Hour = 9
	state.DateTime.Minute = 0
	inventory := &state.PartyState.Inventory
	inventory.Provisions.Gems.Set(1)
	inventory.Provisions.Torches.Set(1)
	inventory.Provisions.Food.Set(2)
	scene := &GameScene{gameState: state}
	scene.observeProvisionChanges()
	require.True(t, state.ActionViewSurface())
	require.True(t, state.ActionIgnite())
	starving, _ := inventory.ConsumeMealTimeFood(6, 6)
	require.False(t, starving)
	scene.observeProvisionChanges()
	rows := scene.provisionChanges.changes.Rows()
	assert.Equal(t, uint64(2), rows[0].Lost, "only remaining food can be consumed")
	assert.Equal(t, uint64(1), rows[3].Lost)
	assert.Equal(t, uint64(1), rows[4].Lost)
	assert.Zero(t, rows[0].Total)
	assert.Zero(t, rows[3].Total)
	assert.Zero(t, rows[4].Total)
	require.False(t, state.ActionViewSurface())
	require.False(t, state.ActionIgnite())
	starving, _ = inventory.ConsumeMealTimeFood(6, 6)
	require.True(t, starving)
	scene.observeProvisionChanges()
	assert.Equal(t, rows, scene.provisionChanges.changes.Rows())
}

func TestProvisionFeedbackSaveHydrationReplacementAndDisposalAreSilent(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).WithPlayerAt(15, 15).
		WithSystemCallbacks().WithRandomSeed(1).Build()
	scene := &GameScene{gameState: state}
	scene.observeProvisionChanges()
	state.PartyState.Inventory.Gold.Set(456)
	scene.observeProvisionChanges()
	require.NotEmpty(t, scene.provisionChanges.changes.Rows()[1].Name)
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	loaded, err := game_state.NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen, state.MapState.YTilesVisibleOnGameScreen)
	require.NoError(t, err)
	scene.gameState = loaded
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	recaptured, err := loaded.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, snapshot, recaptured, "observation and runtime counters must not enter the save contract")
	loaded.PartyState.Inventory.Gold.DecrementByOne()
	scene.observeProvisionChanges()
	assert.Equal(t, uint64(1), scene.provisionChanges.changes.Rows()[1].Lost)
	scene.gameConfig = loaded.GameConfig
	scene.Activate()
	assertNoSceneProvisionChanges(t, scene)
	loaded.PartyState.Inventory.Gold.DecrementByOne()
	scene.observeProvisionChanges()
	assert.Equal(t, uint64(1), scene.provisionChanges.changes.Rows()[1].Lost, "activation adopts candidate setup as its baseline")
	scene.PrepareForSceneReplacement()
	assertNoSceneProvisionChanges(t, scene)
	scene.Dispose()
	loaded.PartyState.Inventory.Gold.IncrementByOne()
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
}

func assertNoSceneProvisionChanges(t *testing.T, scene *GameScene) {
	t.Helper()
	for _, row := range scene.provisionChanges.changes.Rows() {
		assert.Empty(t, row.Name)
		assert.Zero(t, row.Gained)
		assert.Zero(t, row.Lost)
	}
}

func TestCombatPickupSuppressesOnlyItsOwnProvisionReceipt(t *testing.T) {
	for _, item := range []references.Provision{
		references.Food, references.Gold, references.Key, references.Gem, references.Torches, references.SkullKeys,
	} {
		scene := newCombatReportTestScene(t)
		state := scene.gameState
		position := state.CombatScene.GetPartyMemberPosition(state.CombatScene.GetActiveCombatantIndex())
		target := *references.Down.GetNewPositionInDirection(&position)
		state.PartyState.Inventory.Gold.Set(100)
		state.PartyState.Inventory.Provisions.Food.Set(100)
		scene.resetProvisionChanges()
		ref := state.GameReferences.InventoryItemReferences.GetReferenceByItem(item)
		require.True(t, state.CombatScene.AddObject(game_state.NewCombatLooseItem(target, item, 2, ref.ItemSprite)))
		// Unrelated gain and loss may bracket the pickup within the same Update.
		state.PartyState.Inventory.Gold.IncrementBy(7)
		scene.combatMapGetSecondary(references.Down)
		state.PartyState.Inventory.Provisions.Food.DecrementBy(3)
		scene.observeProvisionChanges()
		rows := scene.provisionChanges.changes.Rows()
		assert.Equal(t, uint64(7), rows[1].Gained)
		assert.Equal(t, uint64(3), rows[0].Lost)
		for index, row := range rows {
			if index != 1 {
				assert.Zero(t, row.Gained)
			}
		}
		scene.observeProvisionChanges()
		assert.Equal(t, rows, scene.provisionChanges.changes.Rows(), "the duplicate stays absorbed on later observations")
	}
}
