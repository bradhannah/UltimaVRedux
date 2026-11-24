package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestDungeonGemViewRetainsCommandThroughDismissal(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Provisions.Gems.Set(1)
	beforeTime := state.DateTime
	expectedTime := beforeTime
	expectedTime.Advance(1)
	beforeLocation := state.MapState.PlayerLocation
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyV)

	assert.True(t, state.EventQueueManager.HasWorkflow("dungeon_view_workflow"))
	assert.False(t, state.IsAtDungeonCommandBoundary())
	dialog := waitForTopModal[*widgets.DungeonViewDialog](t, scene)
	assert.Same(t, dialog, scene.dialogStack.PeekTopModalDialog())
	assert.Zero(t, state.PartyState.Inventory.Provisions.Gems.Get())
	dialog.Dismiss()
	drainSceneWorkflow(t, scene, "dungeon_view_workflow", DialogPollingRateMs)

	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonGemViewWithoutGemCompletesImmediately(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Provisions.Gems.Set(0)
	beforeTime := state.DateTime
	expectedTime := beforeTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyV)

	assert.False(t, state.EventQueueManager.HasWorkflow("dungeon_view_workflow"))
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Contains(t, mock.Messages, "You have none!")
}

func TestDungeonGemViewTwiceRestoresImmediateDungeonInput(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	state.PartyState.Inventory.Provisions.Gems.Set(2)
	held := map[ebiten.Key]bool{}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })

	for range 2 {
		scene.dungeonMapInputHandler(ebiten.KeyV)
		dialog := waitForTopModal[*widgets.DungeonViewDialog](t, scene)
		held[ebiten.KeyEscape] = true
		scene.keyboard.SetAllowKeyPressImmediately()
		dialog.Update()
		held[ebiten.KeyEscape] = false
		require.True(t, dialog.IsDismissed())
		drainSceneWorkflow(t, scene, "dungeon_view_workflow", DialogPollingRateMs)

		assert.False(t, mock.InputBlocked)
		assert.False(t, state.EventQueueManager.HasActiveWorkflows())
		assert.False(t, scene.keyboard.IsWaitingForRelease())
		assert.True(t, state.IsAtDungeonCommandBoundary())
	}

	before, ok := state.DungeonStateSnapshot()
	require.True(t, ok)
	scene.dungeonMapInputHandler(ebiten.KeyLeft)
	after, ok := state.DungeonStateSnapshot()
	require.True(t, ok)

	assert.Equal(t, before.Facing.TurnLeft(), after.Facing)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonPreparedInQuasWisUsesSharedViewWorkflow(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(10)
	clearPreparedSpellsForDungeonView(state)
	state.PartyState.Inventory.Spells.Set(references.InQuasWis, 1)
	beforeTime := state.DateTime
	expectedTime := beforeTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	for _, key := range []ebiten.Key{ebiten.KeyI, ebiten.KeyQ, ebiten.KeyW} {
		scene.keyboard.SimulateKeyPress(key)
		spellDialog.Update()
	}
	assert.Equal(t, references.InQuasWis, spellDialog.GetSelectedSpell())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	spellDialog.Update()
	dungeonView := waitForTopModal[*widgets.DungeonViewDialog](t, scene)

	assert.Equal(t, byte(3), caster.GetCurrentMp())
	assert.Zero(t, state.PartyState.Inventory.Spells.Get(references.InQuasWis))
	assert.False(t, state.IsAtDungeonCommandBoundary())
	dungeonView.Dismiss()
	drainProductionCastWorkflow(t, scene)

	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonPreparedCastCancellationPreservesResourcesAndCompletesCommand(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	require.True(t, state.SetActivePlayer(0))
	caster := state.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 8
	caster.SetMp(10)
	clearPreparedSpellsForDungeonView(state)
	state.PartyState.Inventory.Spells.Set(references.InQuasWis, 1)
	expectedTime := state.DateTime
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyC)
	spellDialog := waitForTopModal[*widgets.SpellCastingDialog](t, scene)
	spellDialog.Dismiss()
	drainProductionCastWorkflow(t, scene)

	assert.Equal(t, byte(10), caster.GetCurrentMp())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InQuasWis))
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
}

func TestDungeonInQuasWisScrollUsesSharedViewWorkflow(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInQuasWis, 1)
	beforeTime := state.DateTime
	expectedTime := beforeTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	dungeonView := waitForTopModal[*widgets.DungeonViewDialog](t, scene)

	assert.Zero(t, state.PartyState.Inventory.Scrolls.Get(references.ScrollInQuasWis))
	assert.False(t, state.IsAtDungeonCommandBoundary())
	dungeonView.Dismiss()
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)

	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonInQuasWisScrollShutdownCompletesCommand(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInQuasWis, 1)
	expectedTime := state.DateTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	waitForTopModal[*widgets.DungeonViewDialog](t, scene)

	require.False(t, state.IsAtDungeonCommandBoundary())
	require.NotPanics(t, state.EventQueueManager.Shutdown)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.False(t, mock.InputBlocked)
	assert.Equal(t, mock.BlockInputCalls, mock.UnblockInputCalls)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonSceptreClearsCurrentFieldAndCompletesCommand(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	position := state.MapState.PlayerLocation.Position
	_, err := state.TransformDungeonCell(
		state.MapState.PlayerLocation.Floor,
		position,
		func(references.DungeonCell) references.DungeonCell { return 0x8b },
	)
	require.NoError(t, err)
	expectedTime := state.DateTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)

	snapshot, ok := state.DungeonStateSnapshot()
	require.True(t, ok)
	cell, ok := snapshot.Cells.Cell(state.MapState.PlayerLocation.Floor, position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0x08), cell)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
	assert.Contains(t, mock.Messages, "Field destroyed!")
	assert.NotContains(t, mock.Messages, "Field dissolved!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonSceptreClearsWrappedFacingFieldThroughProductionUse(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	position := state.MapState.PlayerLocation.Position
	snapshot, ok := state.DungeonStateSnapshot()
	require.True(t, ok)
	facing := snapshot.Facing.Direction().GetNewPositionInDirection(&position)
	require.NotNil(t, facing)
	facing = facing.GetWrapped(references.XDungeonMapTiles, references.YDungeonMapTiles)
	_, err := state.TransformDungeonCell(
		state.MapState.PlayerLocation.Floor,
		position,
		func(references.DungeonCell) references.DungeonCell { return 0x00 },
	)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(
		state.MapState.PlayerLocation.Floor,
		*facing,
		func(references.DungeonCell) references.DungeonCell { return 0x89 },
	)
	require.NoError(t, err)
	expectedTime := state.DateTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)

	snapshot, ok = state.DungeonStateSnapshot()
	require.True(t, ok)
	cell, ok := snapshot.Cells.Cell(state.MapState.PlayerLocation.Floor, *facing)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0x08), cell)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
	assert.Contains(t, mock.Messages, "Field destroyed!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
}

func TestDungeonSceptreNoEffectPreservesItemAndCompletesCommand(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	position := state.MapState.PlayerLocation.Position
	snapshot, ok := state.DungeonStateSnapshot()
	require.True(t, ok)
	facing := snapshot.Facing.Direction().GetNewPositionInDirection(&position)
	require.NotNil(t, facing)
	facing = facing.GetWrapped(references.XDungeonMapTiles, references.YDungeonMapTiles)
	_, err := state.TransformDungeonCell(
		state.MapState.PlayerLocation.Floor,
		position,
		func(references.DungeonCell) references.DungeonCell { return 0x00 },
	)
	require.NoError(t, err)
	_, err = state.TransformDungeonCell(
		state.MapState.PlayerLocation.Floor,
		*facing,
		func(references.DungeonCell) references.DungeonCell { return 0x00 },
	)
	require.NoError(t, err)
	expectedTime := state.DateTime
	expectedTime.Advance(1)
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)

	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
	assert.Contains(t, mock.Messages, "No effect!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonUseCancellationPreservesScrollAndCompletesCommand(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInQuasWis, 1)
	expectedTime := state.DateTime
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.Dismiss()
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)

	assert.Equal(t, uint16(1), state.PartyState.Inventory.Scrolls.Get(references.ScrollInQuasWis))
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
}

func TestDungeonUseRejectsSelectionRemovedWhileDialogIsOpen(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInQuasWis, 1)
	expectedTime := state.DateTime
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)
	itemDialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	itemDialog.HandleEnter()
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInQuasWis, 0)
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)

	assert.Contains(t, mock.Messages, "You have none!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonUseKeepsOwnedGrappleInspectable(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
	expectedTime := state.DateTime
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)

	dialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	dialog.HandleEnter()
	assert.False(t, dialog.IsDismissed())
	assert.True(t, state.EventQueueManager.HasWorkflow("dungeon_use_item_workflow"))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Grapple))
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.NotContains(t, mock.Messages, "No usable items!")
	dialog.Dismiss()
	drainSceneWorkflow(t, scene, "dungeon_use_item_workflow", DialogPollingRateMs)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
}

func TestDungeonUseWithoutItemsCompletesImmediately(t *testing.T) {
	scene, mock := newDungeonViewTestScene(t)
	state := scene.gameState
	clearUsableItemsForDungeonView(state)
	expectedTime := state.DateTime
	mock.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyU)

	assert.False(t, state.EventQueueManager.HasWorkflow("dungeon_use_item_workflow"))
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, expectedTime, state.DateTime)
	assert.Contains(t, mock.Messages, "No usable items!")
}

func newDungeonViewTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()

	return newDungeonViewTestSceneAt(t, references.Deceit)
}

func newDungeonViewTestSceneAt(
	t *testing.T,
	location references.Location,
) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, mock := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).
		WithFloor(2).
		WithPlayerAt(3, 4).
		WithSystemCallbacks().
		Build()

	return &GameScene{
		gameState:         state,
		output:            text.NewOutput(nil, 1, 10, 80),
		keyboard:          input.NewKeyboardWithKeyStateSource(0, func(ebiten.Key) bool { return false }),
		spriteSheet:       &sprites.SpriteSheet{},
		autoOpenDoorState: &AutoOpenDoorState{},
	}, mock
}

func clearPreparedSpellsForDungeonView(state *game_state.GameState) {
	for spell := references.Spell(0); spell < references.TotalSpellCount; spell++ {
		state.PartyState.Inventory.Spells.Set(spell, 0)
	}
}

func clearUsableItemsForDungeonView(state *game_state.GameState) {
	for _, item := range state.GetUsableItems() {
		switch value := item.ItemType.(type) {
		case references.Potion:
			state.PartyState.Inventory.Potions.Set(value, 0)
		case references.Scroll:
			state.PartyState.Inventory.Scrolls.Set(value, 0)
		case references.QuestItem:
			state.PartyState.Inventory.QuestItems.Set(value, 0)
		case references.Moonstone:
			state.PartyState.Inventory.Moonstones.Set(value, 0)
		case references.Provision:
			if value == references.SkullKeys {
				state.PartyState.Inventory.Provisions.SkullKeys.Set(0)
			}
		}
	}
}
