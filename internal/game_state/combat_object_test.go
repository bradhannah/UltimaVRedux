package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCombatChestPreservesOriginalNumberEncoding(t *testing.T) {
	t.Parallel()

	position := references.Position{X: 4, Y: 7}
	chest := NewCombatChest(position, 20, true)

	assert.Equal(t, position, chest.Position())
	assert.Equal(t, indexes.SpriteIndex(indexes.Chest), chest.Sprite())
	assert.True(t, chest.IsChest())
	assert.Equal(t, 20, chest.TreasureValue())
	assert.True(t, chest.IsLockedAndTrapped())
	assert.True(t, chest.BlocksMovement())
}

func TestCombatStateObjectStorageIsPositionLocal(t *testing.T) {
	t.Parallel()

	combatState := &CombatState{}
	position := references.Position{X: 3, Y: 5}
	combatState.AddObject(NewCombatChest(position, 10, false))

	object, found := combatState.ObjectAt(position)
	require.True(t, found)
	assert.Equal(t, 10, object.TreasureValue())
	assert.False(t, object.IsLockedAndTrapped())
	assert.True(t, combatState.HasObjectAt(position))
	assert.False(t, combatState.HasObjectAt(references.Position{X: 5, Y: 3}))
	assert.Equal(t, 1, combatState.ObjectCount())
}

func TestCombatStateSupportsMultipleObjectsAtSamePosition(t *testing.T) {
	t.Parallel()

	combatState := &CombatState{}
	position := references.Position{X: 2, Y: 2}
	combatState.AddObject(NewCombatChest(position, 5, false))
	combatState.AddObject(NewCombatChest(position, 12, true))

	object, found := combatState.ObjectAt(position)
	require.True(t, found)
	assert.Equal(t, 12, object.TreasureValue())
	assert.True(t, object.IsLockedAndTrapped())
	assert.Equal(t, 2, combatState.ObjectCount())
}

func TestCombatObjectLifetimeIDsSurviveCopiesButNeverReadmission(t *testing.T) {
	combat := &CombatState{}
	position := references.Position{X: 2, Y: 2}
	chest := NewCombatChest(position, 20, true)
	require.Zero(t, chest.LifetimeID())
	require.True(t, combat.AddObject(chest))
	first, found := combat.ObjectAt(position)
	require.True(t, found)
	require.NotZero(t, first.LifetimeID())
	require.True(t, combat.AddObject(first), "admitting a value copy creates a distinct lifetime")
	second, found := combat.ObjectAt(position)
	require.True(t, found)
	require.Greater(t, second.LifetimeID(), first.LifetimeID())
	firstIdentity := CombatChestIdentity{Position: position, ObjectID: first.LifetimeID()}
	secondIdentity := CombatChestIdentity{Position: position, ObjectID: second.LifetimeID()}
	removed, found := combat.takeFirstObjectAt(position, CombatObject.IsChest)
	require.True(t, found)
	require.Equal(t, first, removed)
	assert.False(t, combat.HasChestIdentity(firstIdentity))
	assert.True(t, combat.HasChestIdentity(secondIdentity), "slice compaction retains the surviving lifetime")
	compacted, found := combat.ObjectAt(position)
	require.True(t, found)
	assert.Equal(t, second, compacted)
	copyOfCombat := *combat
	assert.True(t, copyOfCombat.HasChestIdentity(secondIdentity), "value copying does not re-admit objects")
	require.True(t, combat.AddObject(removed))
	replacement := combat.objects[0]
	assert.Equal(t, first.originalObjectSlot, replacement.originalObjectSlot)
	assert.Greater(t, replacement.LifetimeID(), second.LifetimeID())
	assert.False(t, combat.HasChestIdentity(firstIdentity), "identical slot and contents cannot resurrect an owner")
	assert.Panics(t, func() { combat.liveChest(first) }, "stale live mutation cannot target a reused slot")
	assert.True(t, replacement.IsLockedAndTrapped())
}

func TestCombatObjectLifetimeIDsCoverLootReplacementAndTransferredCorpses(t *testing.T) {
	combat := &CombatState{}
	position := references.Position{X: 2, Y: 2}
	require.True(t, combat.addLootObject(NewCombatChest(position, 20, true)))
	loot, found := combat.takeObjectAt(position, CombatObject.IsChest)
	require.True(t, found)
	require.NotZero(t, loot.LifetimeID())
	combat.replaceObjectInOriginalSlot(loot, NewCombatCorpse(position))
	corpse, found := combat.ObjectAt(position)
	require.True(t, found)
	assert.Equal(t, loot.originalObjectSlot, corpse.originalObjectSlot)
	assert.Greater(t, corpse.LifetimeID(), loot.LifetimeID())
	queue := NewCombatTurnQueue()
	queue.AddCombatant(newMockMonster(15), 0)
	combat.TurnQueue = queue
	actor := CombatTurnIdentity{EntityIndex: 0}
	require.True(t, combat.registerCombatantObject(actor))
	require.True(t, combat.transferCombatantObjectTo(actor, NewCombatCorpse(references.Position{X: 3, Y: 2})))
	transferred, found := combat.ObjectAt(references.Position{X: 3, Y: 2})
	require.True(t, found)
	assert.Greater(t, transferred.LifetimeID(), corpse.LifetimeID())
	combat.lastObjectID = ^CombatObjectID(0)
	count := combat.ObjectCount()
	assert.Panics(t, func() { combat.AddObject(NewCombatChest(position, 20, true)) })
	assert.Equal(t, count, combat.ObjectCount(), "exhaustion must not wrap and reuse an ID")
}

func TestCombatStateOnlyBlocksMovementForBlockingObjects(t *testing.T) {
	t.Parallel()

	position := references.Position{X: 6, Y: 4}
	combatState := &CombatState{}
	combatState.AddObject(CombatObject{
		position:       position,
		sprite:         indexes.ItemMoney,
		blocksMovement: false,
	})

	assert.True(t, combatState.HasObjectAt(position))
	assert.False(t, combatState.HasBlockingObjectAt(position))

	combatState.AddObject(NewCombatChest(position, 5, false))
	assert.True(t, combatState.HasBlockingObjectAt(position))
}

// fillCombatObjectTableWithFields exhausts the object table with entries that
// never yield their slot, unlike loot and corpses.
func fillCombatObjectTableWithFields(t *testing.T, combat *CombatState, position references.Position) {
	t.Helper()
	for combat.hasActiveObjectSlot() {
		require.True(t, combat.AddField(NewCombatField(CombatFieldFire, position)))
	}
}

// REDUX DEVIATION: COMBAT-0001. Objects on the ground give up their table slot
// to the living only once no slot is empty, lowest slot first, and stay put.
func TestGroundObjectsYieldTheirSlotOnlyWhenTheTableIsFull(t *testing.T) {
	queue := NewCombatTurnQueue()
	queue.AddCombatant(newMockPC(15), 0)
	combatState := &CombatState{TurnQueue: queue}
	require.True(t, combatState.registerCombatantObject(CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}))
	corpseAt, lootAt := references.Position{X: 2, Y: 2}, references.Position{X: 1, Y: 1}
	require.True(t, combatState.AddObject(NewCombatCorpse(corpseAt)))
	for range sourceCombatQueueSlots - 3 {
		require.True(t, combatState.AddObject(NewCombatLooseItem(lootAt, references.Food, 1, indexes.ItemFood)))
	}
	corpse := combatState.objects[0]
	require.Equal(t, uint8(2), corpse.originalObjectSlot)

	slot, found := combatState.claimActiveObjectSlot()
	require.True(t, found)
	assert.Equal(t, uint8(sourceCombatQueueSlots), slot, "an empty slot is taken before anything yields")
	assert.Equal(t, uint8(2), combatState.objects[0].originalObjectSlot)
	require.True(t, combatState.AddField(NewCombatField(CombatFieldFire, lootAt)))
	require.Equal(t, sourceCombatQueueSlots, combatState.originalObjectSlotCount())
	assert.False(t, combatState.AddObject(NewCombatCorpse(corpseAt)), "ground objects never displace each other")

	slot, found = combatState.claimActiveObjectSlot()
	require.True(t, found)
	assert.Equal(t, uint8(2), slot, "the lowest-slotted object yields first")
	yielded := combatState.objects[0]
	assert.Zero(t, yielded.originalObjectSlot)
	assert.Equal(t, corpse.LifetimeID(), yielded.LifetimeID())
	assert.Equal(t, corpseAt, yielded.Position())
	assert.Equal(t, sourceCombatQueueSlots-2, combatState.ObjectCount())
	sprite, _, isField := combatState.combatGroundDisplayAt(corpseAt)
	assert.Equal(t, indexes.SpriteIndex(indexes.Splat), sprite, "a corpse without a slot still shows when alone")
	assert.False(t, isField)

	taken, found := combatState.takeObjectAt(corpseAt, CombatObject.IsCorpse)
	require.True(t, found)
	combatState.replaceObjectInOriginalSlot(taken, NewCombatLooseItem(corpseAt, references.Gold, 1, indexes.ItemMoney))
	assert.Zero(t, combatState.objects[0].originalObjectSlot, "loot found on a slotless corpse holds no slot either")
	assert.Equal(t, corpseAt, combatState.objects[0].Position())
}

func TestActiveObjectSlotsRunOutWhenNothingOnTheGroundCanYield(t *testing.T) {
	combatState := &CombatState{TurnQueue: NewCombatTurnQueue()}
	fillCombatObjectTableWithFields(t, combatState, references.Position{X: 1, Y: 1})

	_, found := combatState.claimActiveObjectSlot()
	assert.False(t, found)
	assert.False(t, combatState.AddField(NewCombatField(CombatFieldSleep, references.Position{X: 2, Y: 2})))
}

func TestOriginalObjectSlotsAreIndependentFromQueueSlotsAndReuseAscending(t *testing.T) {
	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	party := syntheticSinglePCParty()
	combat := NewCombatState(combatMap, CombatEntryNorth, party)
	monster := NewCombatMonster(
		references.MonsterOrc,
		&references.EnemyReference{MonsterType: references.MonsterOrc, Dexterity: 10, HitPoints: 10},
		references.Position{X: 8, Y: 8},
		newFixedRNG(0),
	)
	monsterIndex := combat.AddMonsterToQueue(monster)
	monsterEntry := combat.TurnQueue.GetEntry(monsterIndex, false)
	require.NotNil(t, monsterEntry)
	assert.Equal(t, sourceMonsterSlotStart, monsterEntry.sourceSlot)
	assert.Equal(t, uint8(2), monsterEntry.originalObjectSlot,
		"one PC occupies object slot zero, so the first monster uses object slot one")

	position := references.Position{X: 4, Y: 4}
	require.True(t, combat.AddField(NewCombatField(CombatFieldFire, position)))
	require.True(t, combat.AddField(NewCombatField(CombatFieldSleep, position)))
	removed, found := combat.removeFirstFieldAt(position)
	require.True(t, found)
	assert.Equal(t, CombatFieldFire, removed.Kind())
	require.True(t, combat.AddField(NewCombatField(CombatFieldPoison, position)))
	require.Len(t, combat.Fields(), 2)
	assert.Equal(t, CombatFieldPoison, combat.Fields()[0].Kind(), "the lowest freed object slot is reused first")
	assert.Equal(t, CombatFieldSleep, combat.Fields()[1].Kind())
}

func TestCombatLootRejectsOnlyFreeSlotZero(t *testing.T) {
	combat := &CombatState{}
	position := references.Position{X: 3, Y: 3}
	for range sourceCombatQueueSlots {
		require.True(t, combat.AddObject(NewCombatLooseItem(position, references.Food, 1, indexes.ItemFood)))
	}
	_, found := combat.takeFirstObjectAt(position, func(CombatObject) bool { return true })
	require.True(t, found)
	assert.True(t, combat.hasActiveObjectSlot())
	assert.False(t, combat.addLootObject(NewCombatLooseItem(position, references.Gold, 1, indexes.ItemMoney)),
		"findempt returns zero as the additem failure sentinel")
	assert.True(t, combat.AddObject(NewCombatLooseItem(position, references.Gold, 1, indexes.ItemMoney)),
		"ordinary createentry-style admission may reuse slot zero")
}

func TestCombatFieldDirectContactResolvesWhenObjectPoolIsFull(t *testing.T) {
	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	party := syntheticSinglePCParty()
	combat := NewCombatState(combatMap, CombatEntryNorth, party)
	target := references.Position{X: 5, Y: 5}
	monster := NewCombatMonster(
		references.MonsterOrc,
		&references.EnemyReference{MonsterType: references.MonsterOrc, Dexterity: 10, HitPoints: 10},
		target,
		newFixedRNG(0),
	)
	combat.AddMonsterToQueue(monster)
	fillCombatObjectTableWithFields(t, combat, references.Position{X: 1, Y: 1})
	callbacks := NewMockSystemCallbacks(t)
	state := &GameState{
		PartyState: *party, CombatScene: combat, SystemCallbacks: callbacks.ToSystemCallbacks(),
	}

	state.resolveCombatFieldSpell(pendingCombatSpell{
		spell: references.InZuGrav, casterIdx: 0,
		caster: CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0},
		launch: CombatProjectileLaunch{Impact: target},
	})

	assert.True(t, monster.IsAsleep(), "allocation failure must not suppress valid direct contact")
	assert.True(t, callbacks.MessagesContain("No room for field!"))
	assert.Zero(t, countCombatEvents(callbacks.CombatEvents, CombatEventFieldCreated))
	assert.Equal(t, 1, countCombatEvents(callbacks.CombatEvents, CombatEventFieldTriggered))
}

func syntheticSinglePCParty() *party_state.PartyState {
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	for index := range roster {
		roster[index] = &party_state.PlayerCharacter{PartyMap: party_state.PartyMapUnrecruited}
	}
	roster[0].PartyMap = party_state.PartyMapActive
	roster[0].SetHp(30)

	return party_state.NewPartyStateFromRoster(roster)
}

func TestCombatLooseItemsStackAtSamePosition(t *testing.T) {
	t.Parallel()

	position := references.Position{X: 7, Y: 7}
	combatState := &CombatState{}
	combatState.AddObject(NewCombatLooseItem(position, references.Food, 2, indexes.ItemFood))
	combatState.AddObject(NewCombatLooseItem(position, references.Gold, 3, indexes.ItemMoney))

	assert.Equal(t, 2, combatState.ObjectCount())
	topObject, found := combatState.ObjectAt(position)
	require.True(t, found)
	item, isLooseItem := topObject.LooseItem()
	require.True(t, isLooseItem)
	assert.Equal(t, references.Gold, item.Item)
	assert.Equal(t, uint16(3), item.Quantity)
	assert.False(t, combatState.HasBlockingObjectAt(position))
	assert.True(t, combatState.hasMonsterBlockingObjectAt(position),
		"original legalmove2 rejects loose-item squares for monsters")
	corpsePosition := references.Position{X: 6, Y: 7}
	combatState.AddObject(NewCombatCorpse(corpsePosition))
	assert.False(t, combatState.hasMonsterBlockingObjectAt(corpsePosition))
}

func TestCombatObjectOverridesCombatTerrain(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	position := references.Position{X: 5, Y: 5}
	gs.CombatScene.AddObject(NewCombatChest(position, 10, false))

	assert.Equal(t, indexes.SpriteIndex(indexes.Chest), gs.GetEffectiveTile(&position))
	assert.False(t, gs.isCombatPositionValid(&position, -1))
}

func TestCombatChestBlocksMonsterMovement(t *testing.T) {
	gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 169)

	monsterIndex := 0
	var destination references.Position
	foundDestination := false
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			candidate := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			if isCombatPositionValidForMonster(gs, candidate, monster, monsterIndex) {
				destination = candidate
				foundDestination = true

				break
			}
		}
		if foundDestination {
			break
		}
	}
	require.True(t, foundDestination, "combat fixture must contain a valid monster destination")

	gs.CombatScene.AddObject(NewCombatChest(destination, 10, false))

	assert.False(t, isCombatPositionValidForMonster(gs, destination, monster, monsterIndex))
}
