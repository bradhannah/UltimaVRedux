package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceUsePotionShutdownBeforeTargetPreservesInventoryAndCompletesOnce(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Potions.Set(references.Yellow, 1)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
	}
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)
	drainUseSelectionWorkflow(t, scene)

	assert.Equal(t, uint16(1), state.PartyState.Inventory.Potions.Get(references.Yellow))
	assert.Empty(t, callbacks.TimeAdvanced(), "selection cleanup must not complete before the child mutation")
	require.True(t, state.EventQueueManager.HasWorkflow("potion_workflow"))

	state.EventQueueManager.Update()
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Potions.Get(references.Yellow))
	assert.Empty(t, callbacks.TimeAdvanced(), "target selection must precede commitment")

	require.NotPanics(t, state.EventQueueManager.Shutdown)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
	assert.Zero(t, scene.dialogStack.Count())
	state.EventQueueManager.Shutdown()
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestSurfaceUseLightScrollAppliesEffectBeforeSingleCycle(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Scrolls.Set(references.ScrollVasLor, 1)
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)
	drainUseSelectionWorkflow(t, scene)

	assert.Zero(t, state.PartyState.Inventory.Scrolls.Get(references.ScrollVasLor))
	assert.Zero(t, state.MagicLightMinutesRemaining)
	assert.Empty(t, callbacks.TimeAdvanced())
	require.True(t, state.EventQueueManager.HasWorkflow("light_scroll"))

	effectObserved := false
	for range 20 {
		state.ElapsedMs += 100
		state.EventQueueManager.Update()
		if state.MagicLightMinutesRemaining > 0 && !effectObserved {
			effectObserved = true
		}
		if !state.EventQueueManager.HasWorkflow("light_scroll") {
			break
		}
	}
	require.True(t, effectObserved)
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, 239, state.MagicLightMinutesRemaining)
}

func TestSurfaceUseRejectedStonegateTimeStopConsumesAndCompletesOneCycle(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipSceneAt(
		t,
		references.Stonegate,
		0,
		references.Position{X: 12, Y: 12},
	)
	state := scene.gameState
	state.PartyState.Inventory.Scrolls.Set(references.ScrollAnTym, 1)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 3,
	}
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)
	drainUseSelectionWorkflow(t, scene)
	overlay := waitForTimedOverlay(t, scene)

	require.Zero(t, state.PartyState.Inventory.Scrolls.Get(references.ScrollAnTym))
	assert.Equal(t, "No effect!", overlay.GetMessage())
	assert.Equal(t, []game_state.SoundEffect{game_state.SoundScrollInvoke, game_state.SoundSpellFailure}, callbacks.SoundEffectsPlayed)
	assert.Equal(t, game_state.DurationEffectProtection, state.ActiveDurationEffect.EffectType)
	assert.Equal(t, 3, state.ActiveDurationEffect.TurnsRemaining)
	assert.Empty(t, callbacks.TimeAdvanced())

	state.ElapsedMs = scene.timedOverlayDismissMs
	drainNamedSurfaceUseWorkflow(t, scene, "timestop_scroll")

	assert.Equal(t, game_state.DurationEffectProtection, state.ActiveDurationEffect.EffectType)
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	for range 3 {
		state.EventQueueManager.Update()
	}
	assert.Len(t, callbacks.TimeAdvanced(), 1)
}

func TestSurfaceUseSceptreMutatesBeforeSingleCycle(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	barrier := references.Position{X: 14, Y: 15}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &barrier, indexes.ShadowlordBoundary1)
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)
	drainUseSelectionWorkflow(t, scene)

	assert.Equal(t, indexes.SpriteIndex(indexes.ShadowlordBoundary1),
		state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapLayer, barrier))
	assert.Empty(t, callbacks.TimeAdvanced())
	drainNamedSurfaceUseWorkflow(t, scene, "sceptre_workflow")

	assert.Equal(t, indexes.SpriteIndex(indexes.Grass),
		state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapLayer, barrier))
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
}

func TestSurfaceUsePocketWatchCompletesNormalCycleAfterDialog(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.QuestItems.Set(references.PocketWatch, 1)
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)
	drainUseSelectionWorkflow(t, scene)

	require.True(t, state.EventQueueManager.HasWorkflow("pocket_watch"))
	assert.Empty(t, callbacks.TimeAdvanced())
	var overlay *widgets.OverlayDialog
	for range 10 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		if candidate, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay = candidate

			break
		}
	}
	require.NotNil(t, overlay)
	overlay.Dismiss()
	drainNamedSurfaceUseWorkflow(t, scene, "pocket_watch")

	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestSurfaceUseImmediateAmuletChildWaitsForParentBeforeSingleCycle(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.QuestItems.Set(references.Amulet, 1)
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)

	assert.True(t, state.AmuletState.IsActive)
	assert.Empty(t, callbacks.TimeAdvanced(), "early child completion must wait for selection cleanup")
	drainUseSelectionWorkflow(t, scene)

	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestSurfaceKeyURoutesBlackBadgeEffectAndCompletesOnce(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.QuestItems.Set(references.BlackBadge, 1)
	expectedTime := state.DateTime
	expectedTime.Advance(game_state.DefaultSmallMapMinutesPerTurn)
	callbacks.Reset()

	scene.smallMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	drainUseSelectionWorkflow(t, scene)

	assert.True(t, state.IsBadgeActive())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.BlackBadge))
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, callbacks.BlockInputCalls, callbacks.UnblockInputCalls)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dialogStack.Count())

	for range 3 {
		state.EventQueueManager.Update()
	}
	assert.Len(t, callbacks.TimeAdvanced(), 1)
}

func TestSurfaceUseSkullKeyShutdownPreservesInventoryAndTime(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Provisions.SkullKeys.Set(1)
	callbacks.Reset()

	selectSurfaceUseItem(t, scene)
	drainUseSelectionWorkflow(t, scene)

	assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.SkullKeys.Get())
	assert.Empty(t, callbacks.TimeAdvanced())
	state.EventQueueManager.Update()
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.SkullKeys.Get())
	assert.Empty(t, callbacks.TimeAdvanced())

	state.EventQueueManager.Shutdown()
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
	assert.Nil(t, scene.castUseDirectionDialog)
}

func TestSurfaceUseShutdownBeforeFirstUpdateHasMatchedCleanupAndNoCost(t *testing.T) {
	scene, callbacks := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Potions.Set(references.Yellow, 1)
	callbacks.Reset()

	scene.smallMapInputHandler(ebiten.KeyU)
	require.True(t, state.EventQueueManager.HasWorkflow("use_item_workflow"))
	require.NotPanics(t, state.EventQueueManager.Shutdown)

	assert.Equal(t, uint16(1), state.PartyState.Inventory.Potions.Get(references.Yellow))
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func newSurfaceUseOwnershipScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()

	return newSurfaceUseOwnershipSceneAt(t, references.Britain, 0, references.Position{X: 15, Y: 15})
}

func newSurfaceUseOwnershipSceneAt(
	t *testing.T,
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).
		WithFloor(floor).
		WithPlayerAt(int(position.X), int(position.Y)).
		WithRandomSeed(387).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	clearCombatUseTestInventory(state)
	clearCombatUseQuestItems(state)

	scene := newSurfaceCompletionTestScene(state)
	state.SystemCallbacks.Flow.Items.StartPotionWorkflow = scene.startPotionWorkflow
	state.SystemCallbacks.Flow.Items.StartWindChangeScrollWorkflow = scene.startWindChangeScrollWorkflow
	state.SystemCallbacks.Flow.Items.StartLightScrollWorkflow = scene.startLightScrollWorkflow
	state.SystemCallbacks.Flow.Items.StartTimeStopScrollWorkflow = scene.startTimeStopScrollWorkflow
	state.SystemCallbacks.Flow.Items.StartResurrectionScrollWorkflow = scene.startResurrectionScrollWorkflow
	state.SystemCallbacks.Flow.Items.StartMoonstoneBurialWorkflow = scene.startMoonstoneBurialWorkflow
	state.SystemCallbacks.Flow.Items.StartCarpetBoardingWorkflow = scene.startCarpetBoardingWorkflow
	state.SystemCallbacks.Flow.Items.StartSkullKeyWorkflow = scene.startSkullKeyWorkflow
	state.SystemCallbacks.Flow.Items.StartHMSCapeWorkflow = scene.startHMSCapeWorkflow
	state.SystemCallbacks.Flow.Items.StartSceptreWorkflow = scene.startSceptreWorkflow
	state.SystemCallbacks.Flow.Observation.ShowGemViewInteraction = scene.ShowGemViewInteraction
	state.SystemCallbacks.Flow.Observation.ShowSpyglassInteraction = scene.ShowSpyglassInteraction
	state.SystemCallbacks.Flow.Observation.ShowPocketWatchTime = scene.ShowPocketWatchTime
	state.SystemCallbacks.Flow.Shadowlord.ShowShardUsageWorkflow = scene.startShardWorkflow

	return scene, callbacks
}

func selectSurfaceUseItem(t *testing.T, scene *GameScene) {
	t.Helper()
	scene.smallMapInputHandler(ebiten.KeyU)
	dialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	dialog.HandleEnter()
}

func drainUseSelectionWorkflow(t *testing.T, scene *GameScene) {
	t.Helper()
	drainNamedSurfaceUseWorkflow(t, scene, "use_item_workflow")
}

func drainNamedSurfaceUseWorkflow(t *testing.T, scene *GameScene, name string) {
	t.Helper()
	for range 100 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow(name) {
			return
		}
	}
	t.Fatalf("workflow %q did not complete", name)
}
