package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrCombatEncounterAlreadyActive indicates that encounter composition was requested in combat.
	ErrCombatEncounterAlreadyActive = errors.New("combat encounter already active")
	// ErrInvalidCombatEncounter indicates that a composer request cannot form a complete encounter.
	ErrInvalidCombatEncounter = errors.New("invalid combat encounter")
)

// CombatEncounterCountMode controls how the composer resolves the group size.
type CombatEncounterCountMode int

const (
	// CombatEncounterCountAutomatic uses the monster's original group-size rule.
	CombatEncounterCountAutomatic CombatEncounterCountMode = iota
	// CombatEncounterCountExact uses CombatEncounterRequest.ExactCount.
	CombatEncounterCountExact
	// CombatEncounterCountMaximum fills up to the monster and available-position limits.
	CombatEncounterCountMaximum
)

// CombatEncounterMap identifies either a Britannia combat map or a dungeon room.
type CombatEncounterMap struct {
	Territory    references.CombatMapTerritory
	BritanniaMap references.BritanniaCombatMap
	Dungeon      references.DungeonCombatIndex
	DungeonRoom  int
}

// CombatEncounterRequest describes one complete encounter to start outside combat.
type CombatEncounterRequest struct {
	Map            CombatEncounterMap
	EntryDirection CombatEntryDirection
	MonsterType    references.MonsterType
	CountMode      CombatEncounterCountMode
	ExactCount     int
	FriendsEnabled bool
	// Authored loads a fixed dungeon room exactly as walking into it would, always live.
	// MonsterType, CountMode, ExactCount and FriendsEnabled are then unused.
	Authored bool
	// Seed is optional. When non-nil, composing the encounter calls SetRandomSeed,
	// which resets the centralized GameState RNG sequence before any encounter rolls.
	Seed *uint64
}

// CombatEncounterResult reports the encounter that was committed.
type CombatEncounterResult struct {
	Territory    references.CombatMapTerritory
	MapIndex     int
	Dungeon      references.DungeonCombatIndex
	DungeonRoom  int
	Seed         *uint64
	PrimaryCount int
	FriendCount  int
	Positions    []references.Position
}

type preparedCombatEncounter struct {
	mapIndex    int
	primaryRef  *references.EnemyReference
	friendType  references.MonsterType
	combatState *CombatState
	positions   []references.Position
	capacity    int
}

// StartCombatEncounter validates, composes, and atomically starts one tactical encounter.
func (g *GameState) StartCombatEncounter(request CombatEncounterRequest) (*CombatEncounterResult, error) {
	return g.startCombatEncounter(request, nil)
}

func (g *GameState) startCombatEncounter(
	request CombatEncounterRequest,
	beforeComposition func(*CombatState) error,
) (*CombatEncounterResult, error) {
	if err := g.validateCombatEncounterStartState(); err != nil {
		return nil, err
	}
	if request.Authored {
		return g.startAuthoredDungeonRoom(request)
	}

	prepared, err := g.prepareCombatEncounter(request)
	if err != nil {
		return nil, err
	}

	seed := cloneEncounterSeed(request.Seed)
	if seed != nil {
		g.SetRandomSeed(*seed)
	}
	if beforeComposition != nil {
		if err = beforeComposition(prepared.combatState); err != nil {
			return nil, err
		}
	}
	g.initializeCombatPartyRings(prepared.combatState)
	count := g.resolveCombatEncounterCount(request, prepared.primaryRef, prepared.capacity)
	primaryCount, friendCount := g.populatePreparedCombatEncounter(request, prepared, count)

	g.commitCombatTransition(prepared.combatState, request.Map.Territory)
	if request.Map.Territory == references.DungeonCombatTerritory {
		g.CombatScene.ExitContext = CombatExitDungeonRoom
	}

	return &CombatEncounterResult{
		Territory:    request.Map.Territory,
		MapIndex:     prepared.mapIndex,
		Dungeon:      request.Map.Dungeon,
		DungeonRoom:  request.Map.DungeonRoom,
		Seed:         seed,
		PrimaryCount: primaryCount,
		FriendCount:  friendCount,
		Positions:    append([]references.Position(nil), prepared.positions[:count]...),
	}, nil
}

// startAuthoredDungeonRoom runs the same fixed-room setup as walking into the room. The
// encounter has no dungeon origin, so its victory leaves the save's room progress untouched.
func (g *GameState) startAuthoredDungeonRoom(request CombatEncounterRequest) (*CombatEncounterResult, error) {
	if request.Map.Territory != references.DungeonCombatTerritory {
		return nil, fmt.Errorf("%w: authored contents need a dungeon room", ErrInvalidCombatEncounter)
	}
	mapIndex, err := resolveCombatEncounterMap(request.Map)
	if err != nil {
		return nil, err
	}
	if err = validateCombatEncounterOptions(request); err != nil {
		return nil, err
	}
	placement, placed := g.DungeonRoomPlacement(request.Map.Dungeon, request.Map.DungeonRoom)
	if !placed {
		return nil, fmt.Errorf("%w: dungeon room %d is not placed on any level",
			ErrInvalidCombatEncounter, request.Map.DungeonRoom)
	}
	combatMap, err := references.LoadCombatMap(mapIndex, request.Map.Territory, g.GameConfig)
	if err != nil {
		return nil, fmt.Errorf("loading authored dungeon room %d: %w", mapIndex, err)
	}
	combatState := NewCombatState(combatMap, request.EntryDirection, &g.PartyState)
	g.configureCombatTurnQueue(combatState)
	seed := cloneEncounterSeed(request.Seed)
	if seed != nil {
		g.SetRandomSeed(*seed)
	}
	if err = g.populateFixedDungeonRoom(combatState, placement.Floor); err != nil {
		return nil, err
	}
	g.commitCombatTransition(combatState, request.Map.Territory)
	g.CombatScene.ExitContext = CombatExitDungeonRoom

	positions := make([]references.Position, 0, combatState.MonsterManager.Count())
	for index := range combatState.MonsterManager.Count() {
		positions = append(positions, combatState.MonsterManager.GetMonster(index).Position())
	}

	return &CombatEncounterResult{
		Territory:    request.Map.Territory,
		MapIndex:     mapIndex,
		Dungeon:      request.Map.Dungeon,
		DungeonRoom:  request.Map.DungeonRoom,
		Seed:         seed,
		PrimaryCount: len(positions),
		FriendCount:  0,
		Positions:    positions,
	}, nil
}

func (g *GameState) populatePreparedCombatEncounter(
	request CombatEncounterRequest,
	prepared *preparedCombatEncounter,
	count int,
) (int, int) {
	primaryCount := 0
	friendCount := 0
	for index := range count {
		// Original combat initialization interleaves each friend roll with combatant allocation's DEX roll.
		// See ref:ENEMY-0003.
		monsterType := g.composeCombatMonsterType(request, prepared.friendType, index, count)
		enemyRef := prepared.primaryRef
		if monsterType != request.MonsterType {
			friendRef, err := g.enemyReferenceForCombat(monsterType)
			if err != nil {
				panic("prevalidated combat friend reference became invalid")
			}
			enemyRef = friendRef
			friendCount++
		} else {
			primaryCount++
		}
		prepared.combatState.AddMonsterToQueue(NewCombatMonster(
			monsterType,
			enemyRef,
			prepared.positions[index],
			g.AsRandomProvider(),
		))
	}
	_, queuedMonsters := prepared.combatState.TurnQueue.CountRemaining()
	if queuedMonsters != prepared.combatState.MonsterManager.Count() {
		panic("composed combat monster manager and turn queue are out of sync")
	}

	return primaryCount, friendCount
}

func (g *GameState) validateCombatEncounterStartState() error {
	if g.CombatScene != nil || g.IsInCombat() {
		return ErrCombatEncounterAlreadyActive
	}
	if g.EventQueueManager.HasActiveWorkflows() {
		return fmt.Errorf("%w: gameplay workflow is active", ErrInvalidCombatEncounter)
	}

	return nil
}

func (g *GameState) prepareCombatEncounter(
	request CombatEncounterRequest,
) (*preparedCombatEncounter, error) {
	mapIndex, err := resolveCombatEncounterMap(request.Map)
	if err != nil {
		return nil, err
	}
	if err = validateCombatEncounterOptions(request); err != nil {
		return nil, err
	}

	primaryRef, friendType, err := g.combatEncounterReferences(request)
	if err != nil {
		return nil, err
	}
	combatMap, err := references.LoadCombatMap(mapIndex, request.Map.Territory, g.GameConfig)
	if err != nil {
		return nil, fmt.Errorf("loading encounter combat map %d: %w", mapIndex, err)
	}
	combatState := NewCombatState(combatMap, request.EntryDirection, &g.PartyState)
	g.configureCombatTurnQueue(combatState)
	positions, err := availableEncounterPositions(combatState)
	if err != nil {
		return nil, err
	}
	if len(positions) == 0 {
		return nil, fmt.Errorf("%w: combat map has no unoccupied monster positions", ErrInvalidCombatEncounter)
	}

	capacity := min(references.MaxMonstersInCombat, len(positions))
	if request.CountMode == CombatEncounterCountExact &&
		(request.ExactCount < 1 || request.ExactCount > capacity) {
		return nil, fmt.Errorf("%w: exact count %d is outside 1-%d",
			ErrInvalidCombatEncounter, request.ExactCount, capacity)
	}

	return &preparedCombatEncounter{
		mapIndex:    mapIndex,
		primaryRef:  primaryRef,
		friendType:  friendType,
		combatState: combatState,
		positions:   positions,
		capacity:    capacity,
	}, nil
}

func validateCombatEncounterOptions(request CombatEncounterRequest) error {
	if request.EntryDirection < CombatEntryWest || request.EntryDirection > CombatEntrySouth {
		return fmt.Errorf("%w: entry direction %d", ErrInvalidCombatEncounter, request.EntryDirection)
	}
	if request.CountMode < CombatEncounterCountAutomatic || request.CountMode > CombatEncounterCountMaximum {
		return fmt.Errorf("%w: count mode %d", ErrInvalidCombatEncounter, request.CountMode)
	}

	return nil
}

func (g *GameState) combatEncounterReferences(
	request CombatEncounterRequest,
) (*references.EnemyReference, references.MonsterType, error) {
	primaryRef, err := g.enemyReferenceForCombat(request.MonsterType)
	if err != nil {
		return nil, references.MonsterNone, fmt.Errorf("%w: %w", ErrInvalidCombatEncounter, err)
	}
	if primaryRef.MaxPerMap < 1 {
		return nil, references.MonsterNone, fmt.Errorf("%w: monster %s has maximum group size %d",
			ErrInvalidCombatEncounter, request.MonsterType, primaryRef.MaxPerMap)
	}

	friendType := request.MonsterType
	if request.FriendsEnabled && primaryRef.Friend != nil {
		friendType = primaryRef.Friend.MonsterType
		if _, friendErr := g.enemyReferenceForCombat(friendType); friendErr != nil {
			return nil, references.MonsterNone,
				fmt.Errorf("%w: friend: %w", ErrInvalidCombatEncounter, friendErr)
		}
	}

	return primaryRef, friendType, nil
}

func resolveCombatEncounterMap(encounterMap CombatEncounterMap) (int, error) {
	switch encounterMap.Territory {
	case references.BritanniaCombatTerritory:
		if encounterMap.BritanniaMap < references.CampFireCombatMap ||
			encounterMap.BritanniaMap >= references.BritanniaCombatMap(references.TotalCombatMaps) {
			return 0, fmt.Errorf("%w: Britannia map %d", ErrInvalidCombatEncounter, encounterMap.BritanniaMap)
		}

		return int(encounterMap.BritanniaMap), nil
	case references.DungeonCombatTerritory:
		if encounterMap.Dungeon < references.DeceitDungeon ||
			encounterMap.Dungeon > references.DoomDungeon {
			return 0, fmt.Errorf("%w: dungeon %d", ErrInvalidCombatEncounter, encounterMap.Dungeon)
		}
		if encounterMap.DungeonRoom < 0 || encounterMap.DungeonRoom >= references.MaxDungeonRooms {
			return 0, fmt.Errorf("%w: dungeon room %d", ErrInvalidCombatEncounter, encounterMap.DungeonRoom)
		}

		return references.CalculateDungeonCombatMapIndex(encounterMap.Dungeon, encounterMap.DungeonRoom), nil
	default:
		return 0, fmt.Errorf("%w: combat territory %d", ErrInvalidCombatEncounter, encounterMap.Territory)
	}
}

func availableEncounterPositions(combatState *CombatState) ([]references.Position, error) {
	occupied := make(map[references.Position]struct{}, party_state.MaxActiveParty+references.MaxMonstersPerMap)
	for index, active := range combatState.PartyActive {
		if active {
			occupied[combatState.PartyPositions[index]] = struct{}{}
		}
	}

	positions := make([]references.Position, 0, references.MaxMonstersPerMap)
	for index, position := range combatState.CombatMapRef.MonsterPositions {
		if position.X < 0 || position.X >= references.XCombatMapTiles ||
			position.Y < 0 || position.Y >= references.YCombatMapTiles {
			return nil, fmt.Errorf("%w: monster position %d is outside combat map: %v",
				ErrInvalidCombatEncounter, index, position)
		}
		if _, isOccupied := occupied[position]; isOccupied {
			continue
		}
		occupied[position] = struct{}{}
		positions = append(positions, position)
	}

	return positions, nil
}

func (g *GameState) resolveCombatEncounterCount(
	request CombatEncounterRequest,
	primaryRef *references.EnemyReference,
	capacity int,
) int {
	switch request.CountMode {
	case CombatEncounterCountExact:
		return request.ExactCount
	case CombatEncounterCountMaximum:
		return min(primaryRef.MaxPerMap, capacity)
	case CombatEncounterCountAutomatic:
		maximum := min(primaryRef.MaxPerMap, capacity)
		// Original combat initialization fixes counts by reference group bound, not monster identity.
		// See ref:ENEMY-0003.
		if primaryRef.MaxPerMap == 1 || primaryRef.MaxPerMap == 8 || primaryRef.MaxPerMap == 16 {
			return maximum
		}

		count := g.RollDice(maximum)
		if g.hasOpeningCampaignEncounterReduction() {
			count = g.RollDice(count)
		}

		return count
	default:
		panic("prevalidated combat encounter count mode became invalid")
	}
}

func (g *GameState) resolveCombatEncounterTypes(
	request CombatEncounterRequest,
	friendType references.MonsterType,
	count int,
) []references.MonsterType {
	monsterTypes := make([]references.MonsterType, count)
	for index := range monsterTypes {
		monsterTypes[index] = g.composeCombatMonsterType(request, friendType, index, count)
	}

	return monsterTypes
}

func (g *GameState) composeCombatMonsterType(
	request CombatEncounterRequest,
	friendType references.MonsterType,
	index, count int,
) references.MonsterType {
	// The roll precedes the friends[] lookup even when it names the primary type.
	if request.FriendsEnabled && index > 0 && references.ShouldUseFriend(index, count, g.AsRandomProvider()) {
		return friendType
	}

	return request.MonsterType
}

func cloneEncounterSeed(seed *uint64) *uint64 {
	if seed == nil {
		return nil
	}
	seedCopy := *seed

	return &seedCopy
}
