package game_state

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/files"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestDungeonRoomEntryDirectionMatchesOriginalDirconv(t *testing.T) {
	t.Parallel()

	want := map[references.DungeonLastDirection]CombatEntryDirection{
		references.DungeonLastNorth:        CombatEntryNorth,
		references.DungeonLastEast:         CombatEntryEast,
		references.DungeonLastSouth:        CombatEntrySouth,
		references.DungeonLastWest:         CombatEntryWest,
		references.DungeonArrivedFromBelow: CombatEntryNorth,
		references.DungeonArrivedFromAbove: CombatEntrySouth,
	}
	for direction, entry := range want {
		assert.Equal(t, entry, dungeonRoomEntryDirection(direction))
	}
}

func TestResolvePendingDungeonRoomEncounterPopulatesOriginalSlots(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	data := make([]byte, references.CombatMapDataSize)
	data[references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset] = 0x90
	data[references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset+1] = 0x01
	data[references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 4
	data[references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 5
	data[references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset+1] = 6
	data[references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset+1] = 7
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, files.DungeonCbt), data, 0o600))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xf0))
	attachDungeonStateForTest(t, state, cells)
	callbacks.Reset()

	require.True(t, state.HasPendingDungeonRoomEncounter())
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())

	require.NotNil(t, state.CombatScene)
	require.NotNil(t, state.CombatScene.DungeonRoomOrigin)
	assert.Equal(t, CombatExitDungeonRoom, state.CombatScene.ExitContext)
	assert.Equal(t, references.MonsterRat, state.CombatScene.MonsterManager.GetMonster(0).MonsterType())
	assert.Equal(t, references.Position{X: 4, Y: 5}, state.CombatScene.MonsterManager.GetMonster(0).Position())
	assert.Equal(t, 1, state.CombatScene.ObjectCount())
	assert.Equal(t, references.CombatLocation, state.MapState.PlayerLocation.Location)
	assert.Contains(t, callbacks.Messages, "Entering room...")
}

func TestResolvePendingDungeonRoomEncounterHydratesSeededFieldsInSlotOrder(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	descriptors := [...]byte{0xea, 0x02, 0xe8, 0x90, 0xeb, 0xe9}
	positions := [...]references.Position{
		{X: 2, Y: 3}, {X: 3, Y: 4}, {X: 4, Y: 5},
		{X: 5, Y: 6}, {X: 6, Y: 7}, {X: 8, Y: 9},
	}
	data := make([]byte, references.CombatMapDataSize)
	for slot, descriptor := range descriptors {
		data[references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset+slot] = descriptor
		data[references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset+slot] = byte(positions[slot].X)
		data[references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset+slot] = byte(positions[slot].Y)
	}
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, files.DungeonCbt), data, 0o600))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	require.True(t, cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xf0)))
	attachDungeonStateForTest(t, state, cells)
	rngCalls := state.GetRNG().Calls()
	expectedRNG := beradrng.Restore(state.RandomSeed(), rngCalls)
	for range 4 {
		expectedRNG.Intn(8)
	}
	expectedObjectNumber := expectedRNG.Intn(10) + 1
	expectedDexModifier := expectedRNG.Intn(8) - 4

	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())

	fields := state.CombatScene.Fields()
	require.Len(t, fields, 4)
	wantKinds := [...]CombatFieldKind{
		CombatFieldFire, CombatFieldPoison, CombatFieldProtection, CombatFieldSleep,
	}
	wantPositions := [...]references.Position{positions[0], positions[2], positions[4], positions[5]}
	for index := range fields {
		assert.Equal(t, wantKinds[index], fields[index].Kind())
		assert.Equal(t, wantPositions[index], fields[index].Position())
		assert.False(t, fields[index].createdThisAction)
	}
	assert.False(t, fields[0].BlocksMovement())
	assert.False(t, fields[1].BlocksMovement())
	assert.True(t, fields[2].BlocksMovement())
	assert.False(t, fields[3].BlocksMovement())
	assert.True(t, state.CombatScene.HasProtectionFieldAt(positions[4]))
	require.Len(t, state.CombatScene.objects, 1)
	item, loose := state.CombatScene.objects[0].LooseItem()
	require.True(t, loose)
	assert.Equal(t, references.Gold, item.Item)
	assert.Equal(t, uint16(expectedObjectNumber), item.Quantity)
	monster := state.CombatScene.MonsterManager.GetMonster(0)
	require.NotNil(t, monster)
	expectedDexterity := monster.EnemyRef().Dexterity + expectedDexModifier
	if expectedDexterity < 0 || expectedDexterity > maxDexterity {
		expectedDexterity = monster.EnemyRef().Dexterity
	}
	assert.Equal(t, expectedDexterity, monster.CurrentDex())
	assert.Equal(t, rngCalls+6, state.GetRNG().Calls(),
		"interleaved fields must not shift object-value or monster-DEX RNG")
}

func TestPopulateFixedDungeonRoomDecodesOrdinaryLootInSlotOrder(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	combatMap := references.NewCombatMapReference(0, references.DungeonCombatTerritory)
	for slot := range 15 {
		combatMap.DungeonEntityTiles[slot] = byte(slot + 1)
		combatMap.MonsterPositions[slot] = references.Position{X: references.Coordinate(slot % 11), Y: 2}
	}
	combat := NewCombatState(combatMap, CombatEntryNorth, &state.PartyState)
	floor := references.FloorNumber(3)
	rngCalls := state.GetRNG().Calls()
	expectedRNG := beradrng.Restore(state.RandomSeed(), rngCalls)
	for range 4 {
		expectedRNG.Intn(8)
	}
	expected := []references.ItemAndQuantity{
		{Item: references.Gold, Quantity: uint16(expectedRNG.Intn(40) + 1)},
		{Item: references.Potion(expectedRNG.Intn(8)), Quantity: 1},
		{Item: references.Scroll(expectedRNG.Intn(8)), Quantity: 1},
		{Item: references.Equipment(expectedRNG.Intn(4) + int(references.LongSword)), Quantity: 1},
		{Item: references.Equipment(expectedRNG.Intn(3) + int(references.SmallShield)), Quantity: 1},
		{Item: references.Key, Quantity: uint16(expectedRNG.Intn(8) + 1)},
		{Item: references.Gem, Quantity: uint16(expectedRNG.Intn(8) + 1)},
		{Item: references.Equipment(expectedRNG.Intn(4) + int(references.LeatherHelm)), Quantity: 1},
		{Item: references.Equipment(expectedRNG.Intn(3) + int(references.RingInvisibility)), Quantity: 1},
		{Item: references.Equipment(expectedRNG.Intn(6) + int(references.ClothArmour)), Quantity: 1},
		{Item: references.Equipment(expectedRNG.Intn(3) + int(references.AmuletOfTurning)), Quantity: 1},
		{Item: references.Torches, Quantity: uint16(expectedRNG.Intn(8) + 1)},
		{Item: references.WoodenBox, Quantity: uint16(expectedRNG.Intn(1) + 1)},
		{Item: references.Food, Quantity: uint16(expectedRNG.Intn(8) + 1)},
	}

	require.NoError(t, state.populateFixedDungeonRoom(combat, floor))

	require.Len(t, combat.objects, 15)
	assert.True(t, combat.objects[0].IsChest())
	assert.Equal(t, 16, combat.objects[0].TreasureValue())
	assert.False(t, combat.objects[0].IsLockedAndTrapped())
	for index, want := range expected {
		object := combat.objects[index+1]
		item, loose := object.LooseItem()
		require.Truef(t, loose, "descriptor %d must be collectible", index+2)
		assert.Equal(t, want, item)
		assert.Equal(t, indexes.SpriteIndex(int(indexes.StarPattern)+index+2), object.Sprite())
		assert.False(t, object.BlocksMovement())
	}
	assert.Equal(t, rngCalls+18, state.GetRNG().Calls())
}

func TestPopulateFixedDungeonRoomDecodesQuestLootAndPreservesOtherObjects(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	combatMap := references.NewCombatMapReference(0, references.DungeonCombatTerritory)
	descriptors := [...]byte{0xb4, 0xb5, 0xb6, 0xb7, 0x3c, 0x1e, 0x1f}
	for slot, descriptor := range descriptors {
		combatMap.DungeonEntityTiles[slot] = descriptor
		combatMap.MonsterPositions[slot] = references.Position{X: references.Coordinate(slot + 1), Y: 2}
	}
	combat := NewCombatState(combatMap, CombatEntryNorth, &state.PartyState)
	rngCalls := state.GetRNG().Calls()

	require.NoError(t, state.populateFixedDungeonRoom(combat, 0))

	require.Len(t, combat.objects, len(descriptors))
	wantQuestItems := [...]references.QuestItem{
		references.ShardOfFalsehood, references.Crown, references.Sceptre, references.Amulet,
	}
	for index, want := range wantQuestItems {
		item, loose := combat.objects[index].LooseItem()
		require.True(t, loose)
		assert.Equal(t, want, item.Item)
		assert.Equal(t, uint16(1), item.Quantity)
		assert.Equal(t, indexes.SpriteIndex(int(indexes.StarPattern)+int(descriptors[index])), combat.objects[index].Sprite())
	}
	_, loose := combat.objects[4].LooseItem()
	assert.False(t, loose, "Doom's mirror descriptor remains a blocking tactical object")
	assert.True(t, combat.objects[4].BlocksMovement())
	assert.Equal(t, int(0x3c), combat.objects[4].number)
	assert.False(t, combat.objects[5].BlocksMovement(), "authored DEAD_PC is walkable")
	assert.False(t, combat.objects[6].BlocksMovement(), "authored DEAD_MONST is walkable")
	assert.Equal(t, rngCalls+4, state.GetRNG().Calls())
}

func TestPopulateFixedDungeonRoomRejectsMalformedDataBeforeRNG(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()

	t.Run("out of bounds position", func(t *testing.T) {
		combatMap := references.NewCombatMapReference(0, references.DungeonCombatTerritory)
		combatMap.DungeonEntityTiles[0] = 2
		combatMap.MonsterPositions[0] = references.Position{X: references.XCombatMapTiles, Y: 1}
		combat := NewCombatState(combatMap, CombatEntryNorth, &state.PartyState)
		rngCalls := state.GetRNG().Calls()

		err := state.populateFixedDungeonRoom(combat, 0)

		require.ErrorContains(t, err, "out of bounds")
		assert.Empty(t, combat.objects)
		assert.Equal(t, rngCalls, state.GetRNG().Calls())
	})

	t.Run("original object capacity", func(t *testing.T) {
		combatMap := references.NewCombatMapReference(0, references.DungeonCombatTerritory)
		combatMap.DungeonEntityTiles[0] = 2
		combatMap.MonsterPositions[0] = references.Position{X: 1, Y: 1}
		combat := NewCombatState(combatMap, CombatEntryNorth, &state.PartyState)
		for combat.hasFreeOriginalObjectSlot() {
			combat.AddObject(NewCombatLooseItem(
				references.Position{X: 1, Y: 1}, references.Food, 1, indexes.ItemFood,
			))
		}
		objectsBefore := combat.ObjectCount()
		rngCalls := state.GetRNG().Calls()

		err := state.populateFixedDungeonRoom(combat, 0)

		require.ErrorContains(t, err, "original object slots")
		assert.Equal(t, objectsBefore, combat.ObjectCount())
		assert.Equal(t, rngCalls, state.GetRNG().Calls())
	})
}

func TestFixedDungeonRoomCollectedLootPersistsThroughDefeatAndRespawns(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	data := make([]byte, references.CombatMapDataSize)
	data[references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset] = 2
	data[references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 5
	data[references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 6
	for direction := range references.NumDirections {
		rowOffset := (references.FirstDirectionRow + direction) * references.CombatMapBytesPerRow
		data[rowOffset+references.PlayerXCoordOffset] = 5
		data[rowOffset+references.PlayerYCoordOffset] = 5
	}
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, files.DungeonCbt), data, 0o600))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	require.True(t, cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xf0)))
	attachDungeonStateForTest(t, state, cells)
	goldBefore := state.PartyState.Inventory.Gold.Get()

	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	require.True(t, state.ActionGetCombatMap(references.Down))
	goldAfterCollection := state.PartyState.Inventory.Gold.Get()
	assert.Greater(t, goldAfterCollection, goldBefore)
	assert.Zero(t, state.CombatScene.ObjectCount())
	state.EndCombat(CombatDefeat)

	assert.Equal(t, goldAfterCollection, state.PartyState.Inventory.Gold.Get())
	cell, ok := state.activeDungeon.Cell(0, references.Position{X: 1, Y: 1})
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0xf0), cell)
	state.synchronizeDungeonRoomEncounterForStablePosition()
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	assert.Equal(t, 1, state.CombatScene.ObjectCount(), "uncleared room loot must be recreated")
}

func TestSeededDungeonRoomFieldExpiresAndReloadsFromPristineRecord(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	data := make([]byte, references.CombatMapDataSize)
	data[references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset] = 0xe8
	data[references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 4
	data[references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 5
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, files.DungeonCbt), data, 0o600))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	require.True(t, cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xf0)))
	attachDungeonStateForTest(t, state, cells)
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	require.Len(t, state.CombatScene.Fields(), 1)

	state.SetRandomSeed(seedForFirstIntnValue(t, state, 256, 0))
	callbacks.Reset()
	state.resolveCombatPCFieldEffects(state.CombatScene.GetCurrentTurnIdentity())

	assert.Empty(t, state.CombatScene.Fields())
	require.Len(t, callbacks.CombatEvents, 1)
	assert.Equal(t, CombatEventFieldExpired, callbacks.CombatEvents[0].Kind)
	assert.Equal(t, CombatFieldPoison, callbacks.CombatEvents[0].FieldKind)
	assert.Equal(t, references.Position{X: 4, Y: 5}, callbacks.CombatEvents[0].To)
	assert.False(t, callbacks.CombatEvents[0].FieldFizzled)
	state.EndCombat(CombatDefeat)
	state.synchronizeDungeonRoomEncounterForStablePosition()
	callbacks.Reset()

	require.True(t, state.HasPendingDungeonRoomEncounter())
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	require.Len(t, state.CombatScene.Fields(), 1)
	assert.Equal(t, CombatFieldPoison, state.CombatScene.Fields()[0].Kind())
	assert.Equal(t, references.Position{X: 4, Y: 5}, state.CombatScene.Fields()[0].Position())
}

func TestImmortalDungeonRoomFieldAndLootReloadOnFreshVisit(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Wrong).
		WithSystemCallbacks().
		Build()
	room := 1
	mapIndex := references.CalculateDungeonCombatMapIndex(references.WrongDungeon, room)
	data := make([]byte, (mapIndex+1)*references.CombatMapDataSize)
	recordOffset := mapIndex * references.CombatMapDataSize
	data[recordOffset+references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset] = 0xea
	data[recordOffset+references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 4
	data[recordOffset+references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset] = 5
	data[recordOffset+references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset+1] = 2
	data[recordOffset+references.MonsterXCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset+1] = 0
	data[recordOffset+references.MonsterYCoordRow*references.CombatMapBytesPerRow+references.MonsterCoordOffset+1] = 1
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, files.DungeonCbt), data, 0o600))
	useDungeonCombatTestDataPath(state, dataPath)
	position := references.Position{X: 2, Y: 3}
	floor := references.FloorNumber(4)
	var cells references.DungeonCells
	require.True(t, cells.SetCell(floor, position, references.DungeonCell(0xf0|room)))
	state.MapState.PlayerLocation = references.PlayerLocation{Location: references.Wrong, Floor: floor, Position: position}
	dungeon, err := newDungeonState(
		references.Wrong,
		cells,
		references.DungeonFacingNorth,
		references.DungeonLastNorth,
	)
	require.NoError(t, err)
	state.installDungeonState(dungeon)
	goldBefore := state.PartyState.Inventory.Gold.Get()
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	require.Len(t, state.CombatScene.Fields(), 1)
	require.True(t, state.ActionGetCombatMap(references.Down))
	assert.Greater(t, state.PartyState.Inventory.Gold.Get(), goldBefore)
	assert.Zero(t, state.CombatScene.ObjectCount())

	_, removed := state.CombatScene.removeFirstFieldAt(references.Position{X: 4, Y: 5})
	require.True(t, removed)
	state.EndCombat(CombatVictory)
	cell, ok := state.activeDungeon.Cell(floor, position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0xa0|room), cell)
	byteIndex, mask := dungeonRoomProgressBit(references.WrongDungeon, room)
	assert.Zero(t, state.deferredDungeonRoomProgress[byteIndex]&mask)

	var freshCells references.DungeonCells
	require.True(t, freshCells.SetCell(floor, position, references.DungeonCell(0xf0|room)))
	fresh, err := newDungeonState(
		references.Wrong,
		freshCells,
		references.DungeonFacingNorth,
		references.DungeonLastNorth,
	)
	require.NoError(t, err)
	state.installDungeonState(fresh)
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())

	require.Len(t, state.CombatScene.Fields(), 1)
	assert.Equal(t, CombatFieldFire, state.CombatScene.Fields()[0].Kind())
	assert.Equal(t, references.Position{X: 4, Y: 5}, state.CombatScene.Fields()[0].Position())
	assert.Equal(t, 1, state.CombatScene.ObjectCount(), "immortal room loot must return on a fresh visit")
}

func TestResolveClearedDungeonRoomStartsVictoriousEmptyTraversal(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, 8*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xa7))
	attachDungeonStateForTest(t, state, cells)

	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())

	assert.True(t, state.CombatScene.Victory)
	assert.Zero(t, state.CombatScene.MonsterManager.Count())
	assert.Equal(t, 7, state.CombatScene.DungeonRoomOrigin.Room)
	assert.False(t, state.CombatScene.DungeonRoomOrigin.Live)
}

func TestPitLandingRoomRegeneratesWandererOnlyAfterCombatReturns(t *testing.T) {
	const seed = 2665
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, 8*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	position := references.Position{X: 1, Y: 1}
	require.True(t, cells.SetCell(0, position, references.DungeonCell(0xa7)))
	attachDungeonStateForTest(t, state, cells)
	state.MapState.PlayerLocation.Position = position
	state.activeDungeon.generateWandererAfterRoom = true
	state.SetRandomSeed(seed)

	expected := newDungeonStateTestGame(t, cells)
	expected.MapState.PlayerLocation.Position = position
	expected.SetRandomSeed(seed)
	expected.generateDungeonWanderer()
	expectedSnapshot, active := expected.DungeonStateSnapshot()
	require.True(t, active)

	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	require.NotNil(t, state.CombatScene.DungeonRoomOrigin)
	assert.Equal(t, dungeonRoomGenerateWanderer, state.CombatScene.DungeonRoomOrigin.wandererReturn)
	assert.False(t, state.activeDungeon.generateWandererAfterRoom)
	assert.Nil(t, state.activeDungeon.wanderer)
	assert.Zero(t, state.GetRNG().Calls(), "cleared-room setup must precede regeneration without RNG")

	state.EndCombat(CombatVictory)

	actualSnapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	assert.Equal(t, expectedSnapshot.Wanderer, actualSnapshot.Wanderer)
	assert.Equal(t, expected.GetRNG().Calls(), state.GetRNG().Calls())
}

func TestDungeonRoomChainDefersEndPlayerEffectsUntilFinalExit(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, references.MaxDungeonRooms*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	firstRoom := references.Position{X: 2, Y: 1}
	secondRoom := references.Position{X: 3, Y: 1}
	bombAfterRooms := references.Position{X: 4, Y: 1}
	require.True(t, cells.SetCell(0, firstRoom, references.DungeonCell(0xa1)))
	require.True(t, cells.SetCell(0, secondRoom, references.DungeonCell(0xa2)))
	require.True(t, cells.SetCell(0, bombAfterRooms, references.DungeonCell(0x62)))
	attachDungeonStateForTest(t, state, cells)
	state.activeDungeon.wanderer = &DungeonWanderer{
		Monster: references.MonsterReaper, Floor: 0,
		Position: references.Position{X: 6, Y: 6}, PreviousPosition: references.Position{X: 6, Y: 6},
	}
	wandererBefore := *state.activeDungeon.wanderer
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}

	token, _ := state.BeginDungeonCommandCycle()
	move := state.TryDungeonAdvance()
	require.Equal(t, DungeonMoveMoved, move.Outcome)
	state.CompleteDungeonCommandCycle(token, move.Completion)

	assert.True(t, state.HasPendingDungeonRoomEncounter())
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	state.CombatScene.ExitRoute = CombatExitEast
	state.EndCombat(CombatVictory)

	assert.Equal(t, secondRoom, state.MapState.PlayerLocation.Position)
	assert.True(t, state.HasPendingDungeonRoomEncounter(), "ordinary end-player processing chains adjacent rooms")
	assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
	require.NotNil(t, state.activeDungeon.wanderer)
	assert.Equal(t, wandererBefore, *state.activeDungeon.wanderer,
		"ordinary end-player processing preserves the wanderer through intermediate rooms")
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	state.CombatScene.ExitRoute = CombatExitEast
	state.EndCombat(CombatVictory)

	assert.Equal(t, bombAfterRooms, state.MapState.PlayerLocation.Position)
	assert.False(t, state.HasPendingDungeonRoomEncounter())
	assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining,
		"the complete room chain finishes end-player effects exactly once")
	assert.Nil(t, state.activeDungeon.wanderer)
	bomb, ok := state.activeDungeon.Cell(0, bombAfterRooms)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0x62), bomb,
		"room exits do not process the destination hazard in the same end-player branch")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestPitRoomReturnDoesNotChainAdjacentRoom(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, references.MaxDungeonRooms*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	position := references.Position{X: 1, Y: 1}
	adjacentRoom := references.Position{X: 2, Y: 1}
	require.True(t, cells.SetCell(0, position, references.DungeonCell(0xa1)))
	require.True(t, cells.SetCell(0, adjacentRoom, references.DungeonCell(0xa2)))
	attachDungeonStateForTest(t, state, cells)
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}
	require.True(t, state.queueCurrentDungeonRoomEncounter(
		true, false, true, dungeonRoomGenerateWanderer,
	))

	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	state.CombatScene.ExitRoute = CombatExitEast
	state.EndCombat(CombatVictory)

	assert.Equal(t, adjacentRoom, state.MapState.PlayerLocation.Position)
	assert.False(t, state.HasPendingDungeonRoomEncounter(), "pit_trap calls dng_room only once")
	assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonRoomDefeatWithDeadPartyStopsChainAndFinishesEndPlayer(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, references.MaxDungeonRooms*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	room := references.Position{X: 2, Y: 1}
	require.True(t, cells.SetCell(0, room, references.DungeonCell(0xa1)))
	attachDungeonStateForTest(t, state, cells)
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}
	deathSequences := 0
	state.SystemCallbacks.Flow.Consequences.TriggerDeathSequence = func() { deathSequences++ }

	token, _ := state.BeginDungeonCommandCycle()
	move := state.TryDungeonAdvance()
	state.CompleteDungeonCommandCycle(token, move.Completion)
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		if character != nil && character.PartyMap == party_state.PartyMapActive {
			character.SetStatus(party_state.Dead)
		}
	}
	state.EndCombat(CombatDefeat)

	assert.False(t, state.HasPendingDungeonRoomEncounter())
	assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, 1, deathSequences)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonRoomPopulationFailureConsumesNoRNGAndRemainsPending(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	data := make([]byte, references.CombatMapDataSize)
	data[references.DungeonEntityRow*references.CombatMapBytesPerRow+references.DungeonEntityOffset] = 0x90
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dataPath, files.DungeonCbt), data, 0o600))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	require.True(t, cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xf0)))
	attachDungeonStateForTest(t, state, cells)
	emptyEnemyReferences := references.EnemyReferences{}
	gameReferences := *state.GameReferences
	gameReferences.EnemyReferences = &emptyEnemyReferences
	state.GameReferences = &gameReferences
	rngCalls := state.GetRNG().Calls()

	err := state.ResolvePendingDungeonRoomEncounter()

	require.Error(t, err)
	assert.Equal(t, rngCalls, state.GetRNG().Calls())
	assert.True(t, state.HasPendingDungeonRoomEncounter())
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
	assert.Nil(t, state.CombatScene)
}

func TestFreshStartupRoomDefersWandererGenerationUntilReturn(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, references.MaxDungeonRooms*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	require.True(t, cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xa1)))
	attachDungeonStateForTest(t, state, cells)
	state.SetRandomSeed(1267)
	rngCalls := state.GetRNG().Calls()

	state.generateDungeonWandererAfterFreshEntry()

	assert.Equal(t, rngCalls, state.GetRNG().Calls())
	assert.Equal(t, dungeonRoomGenerateWanderer, state.activeDungeon.roomContinuation.wandererReturn)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	state.CombatScene.ExitRoute = CombatExitEast
	state.EndCombat(CombatVictory)

	require.NotNil(t, state.activeDungeon.wanderer)
	assert.Greater(t, state.GetRNG().Calls(), rngCalls)
}

func TestResumedStartupRoomPreservesSavedWandererAndDoesNotChain(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	dataPath := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(dataPath, files.DungeonCbt),
		make([]byte, references.MaxDungeonRooms*references.CombatMapDataSize),
		0o600,
	))
	useDungeonCombatTestDataPath(state, dataPath)
	var cells references.DungeonCells
	position := references.Position{X: 1, Y: 1}
	adjacentRoom := references.Position{X: 2, Y: 1}
	require.True(t, cells.SetCell(0, position, references.DungeonCell(0xa1)))
	require.True(t, cells.SetCell(0, adjacentRoom, references.DungeonCell(0xa2)))
	attachDungeonStateForTest(t, state, cells)
	state.activeDungeon.wanderer = &DungeonWanderer{
		Monster: references.MonsterReaper, Floor: 0,
		Position: references.Position{X: 6, Y: 6}, PreviousPosition: references.Position{X: 6, Y: 6},
	}
	wandererBefore := *state.activeDungeon.wanderer
	rngCalls := state.GetRNG().Calls()

	require.NoError(t, state.ResolvePendingDungeonRoomEncounter())
	state.CombatScene.ExitRoute = CombatExitEast
	state.EndCombat(CombatVictory)

	assert.Equal(t, adjacentRoom, state.MapState.PlayerLocation.Position)
	assert.False(t, state.HasPendingDungeonRoomEncounter())
	require.NotNil(t, state.activeDungeon.wanderer)
	assert.Equal(t, wandererBefore, *state.activeDungeon.wanderer)
	assert.Equal(t, rngCalls, state.GetRNG().Calls())
}

func useDungeonCombatTestDataPath(state *GameState, dataPath string) {
	configCopy := *state.GameConfig
	flagsCopy := *state.GameConfig.SavedConfigData
	flagsCopy.DataFilePath = dataPath
	configCopy.SavedConfigData = &flagsCopy
	state.GameConfig = &configCopy
}

func TestDungeonRoomVictoryClearsCurrentVisitAndPersistentProgress(t *testing.T) {
	var cells references.DungeonCells
	position := references.Position{X: 2, Y: 3}
	cells.SetCell(4, position, references.DungeonCell(0xfa))
	state := newDungeonStateTestGame(t, cells)
	origin := &DungeonRoomEncounterOrigin{
		Location: references.Deceit, Floor: 4, Position: position,
		DungeonIndex: references.DeceitDungeon, Room: 10, Live: true,
	}

	state.resolveDungeonRoomVictory(origin)

	cell, ok := state.activeDungeon.Cell(4, position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0xaa), cell)
	byteIndex, mask := dungeonRoomProgressBit(references.DeceitDungeon, 10)
	assert.NotZero(t, state.deferredDungeonRoomProgress[byteIndex]&mask)
}

func TestEndCombatResolvesFixedDungeonRoomAndClearsWanderer(t *testing.T) {
	var cells references.DungeonCells
	position := references.Position{X: 2, Y: 3}
	cells.SetCell(4, position, references.DungeonCell(0xfa))
	state := newDungeonStateTestGame(t, cells)
	state.activeDungeon.wanderer = &DungeonWanderer{
		Monster: references.MonsterSlime, Floor: 4,
		Position: references.Position{X: 6, Y: 5}, PreviousPosition: references.Position{X: 5, Y: 5},
		OnCeiling: true,
	}
	rngCallsBefore := state.GetRNG().Calls()
	combat := NewCombatState(
		references.NewCombatMapReference(10, references.DungeonCombatTerritory),
		CombatEntrySouth,
		&state.PartyState,
	)
	combat.ExitContext = CombatExitDungeonRoom
	combat.PreCombatPlayerLocation = references.PlayerLocation{
		Location: references.Deceit, Floor: 4, Position: position,
	}
	combat.DungeonRoomOrigin = &DungeonRoomEncounterOrigin{
		Location: references.Deceit, Floor: 4, Position: position,
		DungeonIndex: references.DeceitDungeon, Room: 10, Live: true,
		wandererReturn: dungeonRoomClearWanderer,
	}
	state.CombatScene = combat
	state.MapState.PlayerLocation.Location = references.CombatLocation

	state.EndCombat(CombatVictory)

	assert.Nil(t, state.CombatScene)
	assert.Equal(t, combat.PreCombatPlayerLocation, state.MapState.PlayerLocation)
	cell, ok := state.activeDungeon.Cell(4, position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0xaa), cell)
	assert.Nil(t, state.activeDungeon.wanderer)
	assert.Equal(t, rngCallsBefore, state.GetRNG().Calls(), "fixed-room return must not regenerate")
}

func TestImmortalDungeonRoomClearsOnlyCurrentVisit(t *testing.T) {
	var cells references.DungeonCells
	position := references.Position{X: 2, Y: 3}
	cells.SetCell(4, position, references.DungeonCell(0xf1))
	state := newDungeonStateTestGame(t, cells)
	state.activeDungeon.location = references.Wrong
	origin := &DungeonRoomEncounterOrigin{
		Location: references.Wrong, Floor: 4, Position: position,
		DungeonIndex: references.WrongDungeon, Room: 1, Live: true,
	}

	state.resolveDungeonRoomVictory(origin)

	cell, ok := state.activeDungeon.Cell(4, position)
	require.True(t, ok)
	assert.Equal(t, references.DungeonCell(0xa1), cell)
	byteIndex, mask := dungeonRoomProgressBit(references.WrongDungeon, 1)
	assert.Zero(t, state.deferredDungeonRoomProgress[byteIndex]&mask)
}

func TestPersistentDungeonRoomsReplayOnlyMatchingLiveCells(t *testing.T) {
	var cells references.DungeonCells
	cells.SetCell(0, references.Position{X: 1, Y: 1}, references.DungeonCell(0xf3))
	cells.SetCell(1, references.Position{X: 2, Y: 2}, references.DungeonCell(0xf4))
	state := &GameState{}
	byteIndex, mask := dungeonRoomProgressBit(references.ShameDungeon, 3)
	state.deferredDungeonRoomProgress[byteIndex] |= mask

	state.applyPersistentDungeonRooms(references.Shame, &cells)

	cleared, _ := cells.Cell(0, references.Position{X: 1, Y: 1})
	live, _ := cells.Cell(1, references.Position{X: 2, Y: 2})
	assert.Equal(t, references.DungeonCell(0xa3), cleared)
	assert.Equal(t, references.DungeonCell(0xf4), live)
}
