package main

import (
	"image"
	"image/color"
	"math"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func newCombatReportTestScene(t *testing.T) *GameScene {
	t.Helper()
	state, _ := newProductionCombatCastState(t)
	scene := newProductionCastTestScene(state)
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	scene.combatHUD = newCombatHUD()
	_, output := widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size: combatHUDFontSize, LineSpacingMultiplier: 1, MaxLines: combatHUDOutputLines, MaxChars: combatHUDOutputChars,
	})
	scene.combatHUD.eventOutput = output
	scene.combatHUD.sync(state.CombatScene)
	state.SystemCallbacks.Combat = game_state.NewCombatCallbacks(scene.recordCombatEvent, scene.recordCombatConclusion)
	return scene
}

func reportPickupEvent() game_state.CombatEvent {
	return game_state.CombatEvent{Kind: game_state.CombatEventPickup, Pickup: game_state.CombatPickupEvent{
		ActorName: "Iolo", Result: game_state.CombatPickupResult{Handled: true, Acquired: 38, ItemName: "Gold"},
	}}
}

func TestCombatGetUsesReportWithoutPopupOrPresentationTurn(t *testing.T) {
	scene := newCombatReportTestScene(t)
	state := scene.gameState
	index := state.CombatScene.GetActiveCombatantIndex()
	// Bottom-row looting is the obstruction regression, not just a centre-map fixture.
	state.CombatScene.SetPartyMemberPosition(index, references.Position{X: 5, Y: 9})
	position := references.Position{X: 5, Y: 10}
	require.True(t, state.CombatScene.AddObject(game_state.NewCombatLooseItem(position, references.Gold, 38, indexes.ItemMoney)))
	date, calls := state.DateTime, state.GetRNG().Calls()
	scene.resetProvisionChanges()
	scene.combatMapGetSecondary(references.Down)
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Empty(t, scene.transientActionFeedback)
	assert.Equal(t, date, state.DateTime)
	assert.Equal(t, calls, state.GetRNG().Calls())
	require.Equal(t, 2, scene.combatHUD.historyCount)
	entry := scene.combatHUD.history[1]
	assert.Equal(t, game_state.CombatEventPickup, entry.Kind)
	assert.Contains(t, scene.combatEventPlainText(entry), "Gold +38")
	scene.combatMapGetSecondary(references.Down)
	require.Equal(t, 3, scene.combatHUD.historyCount)
	assert.Contains(t, scene.combatEventPlainText(scene.combatHUD.history[2]), "PICKUP FAILED")
	assert.Zero(t, scene.dialogStack.Count())
}

func TestCombatReportMovementNeverConsumesHistory(t *testing.T) {
	hud := newCombatHUD()
	hud.addEvent(reportPickupEvent())
	for index := range combatEventHistoryCapacity * 3 {
		hud.addEvent(game_state.CombatEvent{Kind: game_state.CombatEventMove,
			Attacker: game_state.CombatFeedbackEntity{IsPartyMember: index%2 == 0}})
	}
	assert.Equal(t, 1, hud.historyCount)
	assert.Equal(t, game_state.CombatEventPickup, hud.history[0].Kind)
	assert.Equal(t, 1, hud.pendingResults())
	for _, kind := range []game_state.CombatEventKind{game_state.CombatEventTeleport, game_state.CombatEventFled,
		game_state.CombatEventFieldTriggered, game_state.CombatEventDivision} {
		hud.addEvent(game_state.CombatEvent{Kind: kind})
	}
	assert.Equal(t, 5, hud.historyCount)
}

func TestCombatReportEmphasisPreservesHiddenReadingTimeAndDrawIsPure(t *testing.T) {
	scene := newCombatReportTestScene(t)
	state, hud := scene.gameState, scene.combatHUD
	scene.recordCombatEvent(reportPickupEvent())
	state.ElapsedMs += 400
	scene.updateCombatReportAttention()
	assert.Equal(t, combatReportEmphasisMs-400, hud.notices[1].remainingMs)
	modal := &countingDialog{}
	scene.dialogStack.PushModalDialog(modal)
	state.ElapsedMs += 9000
	scene.updateCombatReportAttention()
	assert.Equal(t, combatReportEmphasisMs-400, hud.notices[1].remainingMs)
	scene.dialogStack.MustRemoveWidgetInstance(modal)
	state.PartyState.Inventory.Spells.Set(references.Mani, 1)
	require.True(t, state.BeginCombatPartySpellTargeting(game_state.CastRequest{
		Spell: references.Mani, CasterIdx: state.CombatScene.GetActiveCombatantIndex(),
	}).Valid)
	state.ElapsedMs += 9000
	scene.updateCombatReportAttention()
	assert.Equal(t, combatReportEmphasisMs-400, hud.notices[1].remainingMs)
	state.CancelCombatSpellTargeting()
	before := hud.notices
	screen := newPresentationRaster(1280, 720)
	_, bounds := combatHUDStackedRectangles(screen.Bounds(), combatHUDRectangle(screen.Bounds()))
	for range 3 {
		scene.drawCombatEventTape(screen, combatHUDContentBounds(screen.Bounds(), bounds))
	}
	assert.Equal(t, before, hud.notices, "rendering cannot age or acknowledge arrivals")
	state.ElapsedMs += combatReportEmphasisMs - 400
	scene.updateCombatReportAttention()
	assert.Zero(t, hud.notices[1].remainingMs)
	assert.Zero(t, hud.pendingResults())
}

func TestCombatReportBurstReviewAndHistoryRemainStable(t *testing.T) {
	scene := newCombatReportTestScene(t)
	state, hud := scene.gameState, scene.combatHUD
	scene.recordCombatEvent(reportPickupEvent())
	for range 25 {
		hud.addEvent(game_state.CombatEvent{Kind: game_state.CombatEventBattleBegun})
	}
	state.ElapsedMs += 10000
	scene.updateCombatReportAttention()
	assert.Equal(t, combatReportEmphasisMs, hud.notices[1].remainingMs, "offscreen pickup stays pending")
	date, calls, actor := state.DateTime, state.GetRNG().Calls(), state.CombatScene.GetCurrentTurnIdentity()
	require.True(t, scene.handleCombatHUDInput(ebiten.KeyF3))
	assert.Equal(t, 25, hud.historyOffset)
	assert.False(t, hud.notices[1].pending)
	hud.addEvent(game_state.CombatEvent{Kind: game_state.CombatEventChestTrap,
		ChestTrap: game_state.CombatChestTrapResult{ActorName: "Shamino", Type: game_state.TrapPoison}})
	assert.Equal(t, 26, hud.historyOffset, "new arrival preserves the browsed endpoint")
	assert.Equal(t, 1, hud.pendingResults())
	require.True(t, scene.handleCombatHUDInput(ebiten.KeyF3))
	assert.Zero(t, hud.historyOffset)
	assert.Zero(t, hud.unreadEvents, "reviewing through the live endpoint clears its old-arrival badge")
	assert.Zero(t, hud.pendingResults())
	assert.Equal(t, date, state.DateTime)
	assert.Equal(t, calls, state.GetRNG().Calls())
	assert.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())
}

func TestCombatReportWrappedLabelsFitAndAreRecoverable(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}, {2560, 1440}} {
		manager.PublishLogicalSize(size.X, size.Y)
		scene := newCombatReportTestScene(t)
		entry := reportPickupEvent()
		entry.Pickup.Result.ItemName = "Sandalwood box"
		scene.recordCombatEvent(entry)
		screenBounds := image.Rectangle{Max: size}
		_, panel := combatHUDStackedRectangles(screenBounds, combatHUDRectangle(screenBounds))
		bounds := combatHUDContentBounds(screenBounds, panel)
		spans := scene.prepareCombatEventTape(bounds)
		if size.X == 1280 {
			screen := newPresentationRaster(size.X, size.Y)
			t.Cleanup(screen.Deallocate)
			scene.drawCombatEventTape(screen, bounds)
			for _, span := range spans {
				if scene.combatHUD.history[span.slot].Kind == game_state.CombatEventPickup {
					y := bounds.Min.Y + int(math.Round(float64(span.firstLine)*scene.combatHUD.eventOutput.GetLineSpacing()))
					background := color.RGBAModel.Convert(screen.At(bounds.Max.X-1, y)).(color.RGBA)
					assert.Positive(t, background.G)
					assert.Less(t, background.R, uint8(50))
					assert.Less(t, background.G, uint8(50), "arrival backing must remain dark enough for normal text")
					assert.Less(t, background.B, uint8(50))
				}
			}
		}
		output := scene.combatHUD.eventOutput
		assert.LessOrEqual(t, output.GetPanelContentLineCount(), output.GetLineCapacity(bounds.Dy()))
		assert.Contains(t, output.GetOutputStr(false), "LOOT:")
		assert.Contains(t, output.GetOutputStr(false), "+38")
	}
}

func TestCombatReportDiscoveryRemainsAcknowledgedAndFitsBattlefield(t *testing.T) {
	scene := newCombatReportTestScene(t)
	state := scene.gameState
	previous := display.GetManager().SetMapPopupViewportProvider(scene.mapPopupViewport)
	t.Cleanup(func() { display.GetManager().SetMapPopupViewportProvider(previous) })
	entries := []game_state.CombatLootReveal{
		{Sprite: indexes.ItemMoney, Description: "a sack of gold!"},
		{Sprite: indexes.ItemPotion, Description: "a potion!"},
		{Sprite: indexes.ItemWeapon, Description: "a weapon!"},
	}
	result := game_state.CombatOpenResult{RevealItems: entries,
		Trap: game_state.CombatChestTrapResult{Type: game_state.TrapAcid, ActorName: "Shamino", HPLost: 7}}
	token := state.FinishCombatActionDeferred()
	scene.startCombatChestRevealWorkflow(result, token)
	state.EventQueueManager.Update()
	require.Equal(t, 1, scene.dialogStack.Count())
	assert.Less(t, scene.combatHUD.historyCount, len(entries)+1, "discovery is not published in bulk before the reveal")
	for range 8 {
		state.ElapsedMs += 500
		state.EventQueueManager.Update()
	}
	require.Equal(t, len(entries)+1, scene.combatHUD.historyCount)
	for index, entry := range entries {
		assert.Equal(t, entry, scene.combatHUD.history[index+1].LootReveal)
		assert.Contains(t, scene.combatEventPlainText(scene.combatHUD.history[index+1]), "On the ground")
	}
	dialog := scene.dialogStack.PeekTopModalDialog().(*widgets.CombatLootDialog)
	assert.False(t, dialog.IsDismissed(), "revealing does not auto-dismiss discovery")
	screen := newPresentationRaster(1280, 720)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	assert.True(t, dialog.GetBounds().In(combatBattlefieldRectangle(screen.Bounds())))
	state.EventQueueManager.Shutdown()
	assert.Zero(t, scene.dialogStack.Count())
	assert.Nil(t, scene.combatLootPreview)
}

func TestCombatPickupTextDistinguishesBorrowedTorchFromFailure(t *testing.T) {
	borrowed := game_state.CombatPickupEvent{ActorName: "Mariah", Result: game_state.CombatPickupResult{
		Handled: true, Succeeded: true, Message: "Borrowed! The wall torch lights thy way.",
	}}
	refused := game_state.CombatPickupEvent{ActorName: "Mariah", Result: game_state.CombatPickupResult{
		Message: "Nothing to get!",
	}}

	assert.Equal(t, "PICKUP: Mariah: Borrowed! The wall torch lights thy way.", combatPickupText(borrowed))
	assert.Equal(t, "PICKUP FAILED: Mariah: Nothing to get!", combatPickupText(refused))
	assert.Equal(t, "LOOT: Iolo collected Gold +38", combatPickupText(reportPickupEvent().Pickup))
}
