package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const originalDungeonNPCBaseTile = byte(0x40)

var errDungeonRoomEncounterUnavailable = errors.New("fixed dungeon-room encounter is unavailable")

type dungeonRoomContinuation struct {
	encounterPending bool
	chainRooms       bool
	finishEndPlayer  bool
	wandererReturn   dungeonRoomWandererReturn
}

type dungeonRoomWandererReturn byte

const (
	dungeonRoomPreserveWanderer dungeonRoomWandererReturn = iota
	dungeonRoomClearWanderer
	dungeonRoomGenerateWanderer
)

// DungeonRoomEncounterOrigin identifies the authoritative dungeon cell behind one fixed room.
type DungeonRoomEncounterOrigin struct {
	Location                references.Location
	Floor                   references.FloorNumber
	Position                references.Position
	DungeonIndex            references.DungeonCombatIndex
	Room                    int
	Live                    bool
	chainRoomsOnReturn      bool
	finishEndPlayerOnReturn bool
	wandererReturn          dungeonRoomWandererReturn
}

// HasPendingDungeonRoomEncounter reports whether stable dungeon state is waiting on an A*/F* room handoff.
func (g *GameState) HasPendingDungeonRoomEncounter() bool {
	if g.CombatScene != nil || g.activeDungeon == nil || g.activeDungeonCommand != nil ||
		g.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType ||
		g.EventQueueManager.HasActiveWorkflows() {
		return false
	}
	cell, ok := g.activeDungeon.Cell(g.MapState.PlayerLocation.Floor, g.MapState.PlayerLocation.Position)
	if !ok {
		panic("current dungeon position is invalid")
	}

	return g.activeDungeon.roomContinuation.encounterPending && isDungeonRoomCell(cell)
}

// ResolvePendingDungeonRoomEncounter enters the fixed tactical room at the current dungeon cell.
func (g *GameState) ResolvePendingDungeonRoomEncounter() error {
	if !g.HasPendingDungeonRoomEncounter() {
		return errDungeonRoomEncounterUnavailable
	}
	dungeon := g.activeDungeon
	location := g.MapState.PlayerLocation.Location
	dungeonIndex, ok := references.DungeonLocationToCombatIndex(location)
	if !ok {
		return fmt.Errorf("%w: %s has no DUNGEON.CBT records", errDungeonRoomEncounterUnavailable, location)
	}
	position := g.MapState.PlayerLocation.Position
	floor := g.MapState.PlayerLocation.Floor
	cell, _ := dungeon.Cell(floor, position)
	room := int(cell.Payload())
	continuation := dungeon.roomContinuation
	if dungeon.generateWandererAfterRoom {
		continuation.wandererReturn = dungeonRoomGenerateWanderer
	}
	mapIndex := references.CalculateDungeonCombatMapIndex(dungeonIndex, room)
	combatMap, err := references.LoadCombatMap(mapIndex, references.DungeonCombatTerritory, g.GameConfig)
	if err != nil {
		return fmt.Errorf("loading fixed dungeon room %d: %w", room, err)
	}
	combatState := NewCombatState(combatMap, dungeonRoomEntryDirection(dungeon.lastDirection), &g.PartyState)
	combatState.DungeonRoomOrigin = &DungeonRoomEncounterOrigin{
		Location: location, Floor: floor, Position: position,
		DungeonIndex: dungeonIndex, Room: room, Live: cell.Kind() == references.DungeonLiveRoom,
		chainRoomsOnReturn:      continuation.chainRooms,
		finishEndPlayerOnReturn: continuation.finishEndPlayer,
		wandererReturn:          continuation.wandererReturn,
	}
	g.configureCombatTurnQueue(combatState)
	if combatState.DungeonRoomOrigin.Live {
		if err = g.populateFixedDungeonRoom(combatState, floor); err != nil {
			return err
		}
	} else {
		g.initializeCombatPartyRings(combatState)
		combatState.Victory = true
	}
	g.commitCombatTransition(combatState, references.DungeonCombatTerritory)
	combatState.ExitContext = CombatExitDungeonRoom
	dungeon.generateWandererAfterRoom = false
	dungeon.roomContinuation.encounterPending = false
	g.SystemCallbacks.Message.AddRowStr("Entering room...")

	return nil
}

func isDungeonRoomCell(cell references.DungeonCell) bool {
	return cell.Kind() == references.DungeonClearedRoom || cell.Kind() == references.DungeonLiveRoom
}

func (g *GameState) queueCurrentDungeonRoomEncounter(
	finishEndPlayer, chainRooms, allowDeadParty bool,
	wandererReturn dungeonRoomWandererReturn,
) bool {
	if g.activeDungeon == nil ||
		g.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType {
		return false
	}
	cell, ok := g.activeDungeon.Cell(g.MapState.PlayerLocation.Floor, g.MapState.PlayerLocation.Position)
	if !ok {
		panic("current dungeon position is invalid")
	}
	if !isDungeonRoomCell(cell) || !allowDeadParty && g.PartyState.IsAllPartyDead() {
		return false
	}
	g.activeDungeon.roomContinuation = dungeonRoomContinuation{
		encounterPending: true,
		chainRooms:       chainRooms,
		finishEndPlayer:  finishEndPlayer,
		wandererReturn:   wandererReturn,
	}

	return true
}

func (g *GameState) synchronizeDungeonRoomEncounterForStablePosition() {
	if g.activeDungeon == nil ||
		g.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType ||
		g.activeDungeon.location != g.MapState.PlayerLocation.Location {
		return
	}
	g.activeDungeon.roomContinuation = dungeonRoomContinuation{}
	g.queueCurrentDungeonRoomEncounter(false, false, true, dungeonRoomPreserveWanderer)
}

func (g *GameState) generateDungeonWandererAfterFreshEntry() {
	dungeon := g.requireActiveDungeon()
	if dungeon.roomContinuation.encounterPending {
		dungeon.roomContinuation.wandererReturn = dungeonRoomGenerateWanderer

		return
	}
	g.generateDungeonWanderer()
}

func (g *GameState) resumeDungeonRoomContinuation(origin *DungeonRoomEncounterOrigin) {
	if origin == nil {
		return
	}
	if g.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType {
		dungeon := g.requireActiveDungeon()
		dungeon.roomContinuation = dungeonRoomContinuation{}
		if origin.chainRoomsOnReturn && !g.PartyState.IsAllPartyDead() &&
			g.queueCurrentDungeonRoomEncounter(
				origin.finishEndPlayerOnReturn,
				true,
				false,
				origin.wandererReturn,
			) {
			return
		}
		switch origin.wandererReturn {
		case dungeonRoomPreserveWanderer:
			// Resumed startup rooms restore the SAVED.GAM roaming occupant unchanged.
		case dungeonRoomClearWanderer:
			dungeon.replaceWanderer(nil)
		case dungeonRoomGenerateWanderer:
			g.generateDungeonWanderer()
		default:
			panic("invalid dungeon room wanderer return policy")
		}
	}
	if origin.finishEndPlayerOnReturn {
		g.processDungeonEndPlayerFinalEffects()

		return
	}
	if g.PartyState.IsAllPartyDead() {
		g.checkForPartyWipe()
	}
}

func (g *GameState) clearDungeonRoomContinuationForFinale(origin *DungeonRoomEncounterOrigin) {
	if origin == nil || g.activeDungeon == nil || g.activeDungeon.location != origin.Location {
		panic("Doom finale handoff requires its active dungeon origin")
	}
	g.activeDungeon.roomContinuation = dungeonRoomContinuation{}
	g.activeDungeon.generateWandererAfterRoom = false
}

func dungeonRoomEntryDirection(direction references.DungeonLastDirection) CombatEntryDirection {
	switch direction {
	case references.DungeonLastNorth, references.DungeonArrivedFromBelow:
		return CombatEntryNorth
	case references.DungeonLastEast:
		return CombatEntryEast
	case references.DungeonLastSouth, references.DungeonArrivedFromAbove:
		return CombatEntrySouth
	case references.DungeonLastWest:
		return CombatEntryWest
	}
	panic("invalid dungeon room entry direction")
}

func (g *GameState) populateFixedDungeonRoom(combat *CombatState, floor references.FloorNumber) error {
	choices := fixedDungeonVariableMonsterChoices()
	entityCount := 0
	// REDUX DEVIATION: Original dungeon encounter setup accepts invalid coordinates and can write
	// before the first object slot when the 32-slot list is full. Redux rejects malformed room
	// data before consuming RNG so authoritative state cannot be corrupted.
	// See ref:DNG-0012.
	for slot, descriptor := range combat.CombatMapRef.DungeonEntityTiles {
		if descriptor == 0 {
			continue
		}
		entityCount++
		position := combat.CombatMapRef.MonsterPositions[slot]
		if position.X < 0 || position.X >= references.XCombatMapTiles ||
			position.Y < 0 || position.Y >= references.YCombatMapTiles {
			return fmt.Errorf("fixed dungeon room slot %d position %v is out of bounds", slot, position)
		}
		switch fixedDungeonEntityKind(descriptor) {
		case CombatEncounterEntityField, CombatEncounterEntityObject, CombatEncounterEntityLoot:
			continue
		case CombatEncounterEntityRandomMonster:
			for _, monsterType := range choices {
				if _, err := g.enemyReferenceForCombat(monsterType); err != nil {
					return fmt.Errorf("fixed dungeon room variable slot %d: %w", slot, err)
				}
			}

			continue
		case CombatEncounterEntityMonster:
		}
		monsterType := fixedDungeonMonsterType(descriptor)
		if _, err := g.enemyReferenceForCombat(monsterType); err != nil {
			return fmt.Errorf("fixed dungeon room slot %d descriptor 0x%02x: %w", slot, descriptor, err)
		}
	}
	if combat.originalObjectSlotCount()+entityCount > sourceCombatQueueSlots {
		return fmt.Errorf(
			"fixed dungeon room has %d entities but only %d original object slots are available",
			entityCount,
			sourceCombatQueueSlots-combat.originalObjectSlotCount(),
		)
	}
	g.initializeCombatPartyRings(combat)

	variableMonsters := [4]references.MonsterType{}
	for index := range variableMonsters {
		variableMonsters[index] = choices[g.RandomIntInRange(0, len(choices)-1)]
	}

	for slot, descriptor := range combat.CombatMapRef.DungeonEntityTiles {
		if descriptor == 0 {
			continue
		}
		position := combat.CombatMapRef.MonsterPositions[slot]
		monsterType := fixedDungeonMonsterType(descriptor)
		switch fixedDungeonEntityKind(descriptor) {
		case CombatEncounterEntityField:
			if !combat.AddField(NewCombatField(CombatFieldKind(descriptor&3), position)) {
				panic("fixed dungeon-room field capacity exceeded")
			}

			continue
		case CombatEncounterEntityObject:
			combat.AddObject(NewCombatDungeonObject(position, fixedDungeonDescriptorSprite(descriptor), int(descriptor)))

			continue
		case CombatEncounterEntityLoot:
			object, err := g.fixedDungeonLootObject(descriptor, floor, position)
			if err != nil {
				return fmt.Errorf("fixed dungeon room slot %d descriptor 0x%02x: %w", slot, descriptor, err)
			}
			combat.AddObject(object)

			continue
		case CombatEncounterEntityRandomMonster:
			monsterType = variableMonsters[descriptor&3]
		case CombatEncounterEntityMonster:
		}
		enemyRef, refErr := g.enemyReferenceForCombat(monsterType)
		if refErr != nil {
			return fmt.Errorf("fixed dungeon room slot %d descriptor 0x%02x: %w", slot, descriptor, refErr)
		}
		combat.AddMonsterToQueue(NewCombatMonster(monsterType, enemyRef, position, g.AsRandomProvider()))
	}
	_, queued := combat.TurnQueue.CountRemaining()
	if queued != combat.MonsterManager.Count() {
		panic("fixed dungeon-room monster manager and turn queue are out of sync")
	}

	return nil
}

func (g *GameState) fixedDungeonLootObject(
	descriptor byte,
	floor references.FloorNumber,
	position references.Position,
) (CombatObject, error) {
	if descriptor == 1 {
		return NewCombatChest(position, int(floor)*3+7, false), nil
	}

	number := g.fixedDungeonObjectNumber(descriptor, floor)
	var item references.Item
	quantity := uint16(1)
	switch descriptor {
	case 2:
		item, quantity = references.Gold, uint16(number)
	case 3:
		item = references.Potion(number)
	case 4:
		item = references.Scroll(number)
	case 5, 6, 9, 10, 11, 12:
		item = references.Equipment(number)
	case 7:
		item, quantity = references.Key, uint16(number)
	case 8:
		item, quantity = references.Gem, uint16(number)
	case 13:
		item, quantity = references.Torches, uint16(number)
	case 14:
		item = references.WoodenBox
	case 15:
		item, quantity = references.Food, uint16(number)
	case 0xb4:
		item = references.ShardOfFalsehood
	case 0xb5:
		item = references.Crown
	case 0xb6:
		item = references.Sceptre
	case 0xb7:
		item = references.Amulet
	default:
		return CombatObject{}, errors.New("unsupported fixed-room loot descriptor")
	}

	return NewCombatLooseItem(position, item, quantity, fixedDungeonDescriptorSprite(descriptor)), nil
}

func fixedDungeonVariableMonsterChoices() [8]references.MonsterType {
	return [...]references.MonsterType{
		references.MonsterRat, references.MonsterBat, references.MonsterSpider, references.MonsterSnake,
		references.MonsterSkeleton, references.MonsterSlime, references.MonsterInsect, references.MonsterSlime,
	}
}

func (g *GameState) fixedDungeonObjectNumber(descriptor byte, floor references.FloorNumber) int {
	switch descriptor {
	case 1:
		return int(floor)*3 + 7
	case 2:
		return g.RandomIntInRange(1, (int(floor)+1)*10)
	}
	if descriptor < 3 || descriptor >= 0x10 {
		return 0
	}
	ranges := [...]int{8, 8, 4, 3, 8, 8, 4, 3, 6, 3, 8, 1, 8}
	bases := [...]int{0, 0, 0x1e, 0x04, 1, 1, 0x00, 0x2a, 0x09, 0x2d, 1, 1, 1}
	index := int(descriptor) - 3

	return g.RandomIntInRange(0, ranges[index]-1) + bases[index]
}

func (g *GameState) resolveDungeonRoomVictory(origin *DungeonRoomEncounterOrigin) {
	if origin == nil || !origin.Live {
		return
	}
	if g.activeDungeon == nil || g.activeDungeon.location != origin.Location {
		panic("fixed dungeon-room origin no longer matches active dungeon")
	}
	cell, ok := g.activeDungeon.cells.Cell(origin.Floor, origin.Position)
	if !ok || cell.Kind() != references.DungeonLiveRoom || int(cell.Payload()) != origin.Room {
		panic("fixed dungeon-room source changed before victory resolution")
	}
	if !g.activeDungeon.cells.SetCell(
		origin.Floor,
		origin.Position,
		references.DungeonCell(0xa0|cell.Payload()),
	) {
		panic("fixed dungeon-room source is out of bounds")
	}
	g.activeDungeon.revision++
	g.MapState.LayeredMaps.RebuildDungeonFloor(g.activeDungeon.cells, origin.Floor)
	if !isImmortalDungeonRoom(origin.Location, origin.Room) {
		byteIndex, mask := dungeonRoomProgressBit(origin.DungeonIndex, origin.Room)
		g.deferredDungeonRoomProgress[byteIndex] |= mask
	}
}

func dungeonRoomProgressBit(dungeon references.DungeonCombatIndex, room int) (int, byte) {
	if dungeon < 0 || dungeon > references.MaxDungeonCombatIndex || room < 0 || room >= references.MaxDungeonRooms {
		panic("fixed dungeon-room progress index is invalid")
	}
	bit := int(dungeon)*references.MaxDungeonRooms + room

	return bit / 8, byte(1 << (bit & 7))
}

func isImmortalDungeonRoom(location references.Location, room int) bool {
	//nolint:exhaustive // Only Wrong and Covetous contain original immortal rooms.
	switch location {
	case references.Wrong:
		return room == 1 || room == 6 || room == 11 || room == 12
	case references.Covetous:
		return room == 0 || room == 11
	default:
		return false
	}
}

func (g *GameState) applyPersistentDungeonRooms(
	location references.Location,
	cells *references.DungeonCells,
) {
	dungeonIndex, ok := references.DungeonLocationToCombatIndex(location)
	if !ok {
		return
	}
	for floor := references.FloorNumber(0); floor < references.DungeonFloorCount; floor++ {
		for y := references.Coordinate(0); y < references.YDungeonMapTiles; y++ {
			for x := references.Coordinate(0); x < references.XDungeonMapTiles; x++ {
				position := references.Position{X: x, Y: y}
				cell, _ := cells.Cell(floor, position)
				if cell.Kind() != references.DungeonLiveRoom {
					continue
				}
				byteIndex, mask := dungeonRoomProgressBit(dungeonIndex, int(cell.Payload()))
				if g.deferredDungeonRoomProgress[byteIndex]&mask != 0 {
					cells.SetCell(floor, position, references.DungeonCell(0xa0|cell.Payload()))
				}
			}
		}
	}
}
