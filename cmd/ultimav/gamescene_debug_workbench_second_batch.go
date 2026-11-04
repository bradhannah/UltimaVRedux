package main

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	debugDirectionEastName  = "east"
	debugDirectionNorthName = "north"
	debugDirectionSouthName = "south"
	debugDirectionWestName  = "west"

	debugCombatContentsCustom   = "custom"
	debugCombatContentsAuthored = "authored"
	debugVehicleNone            = "none"
	debugVehicleHorse           = "horse"
	debugVehicleCarpet          = "carpet"
	debugVehicleSkiff           = "skiff"
	debugVehicleFrigate         = "frigate"
	debugVehicleAvatar          = "avatar"
	debugVehicleDock            = "dock"
)

var errInvalidSecondBatchDebugArgument = errors.New("invalid second-batch debug argument")

type debugPositionResult struct {
	X int `json:"x"`
	Y int `json:"y"`
}

type combatEncounterOptionsResult struct {
	BritanniaMaps   []combatEncounterMapResult     `json:"britanniaMaps"`
	Dungeons        []combatEncounterDungeonResult `json:"dungeons"`
	EntryDirections []debugOptionResult            `json:"entryDirections"`
	CountModes      []debugOptionResult            `json:"countModes"`
	Monsters        []combatEncounterMonsterResult `json:"monsters"`
}

type debugOptionResult struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type combatEncounterMapResult struct {
	Index   int                          `json:"index"`
	Name    string                       `json:"name"`
	Preview combatEncounterPreviewResult `json:"preview"`
}

type combatEncounterDungeonResult struct {
	ID    int                         `json:"id"`
	Name  string                      `json:"name"`
	Rooms []combatEncounterRoomResult `json:"rooms"`
}

// combatEncounterRoomResult omits floor and cell for rooms no dungeon level places.
type combatEncounterRoomResult struct {
	Room    int                          `json:"room"`
	Floor   *int                         `json:"floor,omitempty"`
	Cell    *debugPositionResult         `json:"cell,omitempty"`
	Preview combatEncounterPreviewResult `json:"preview"`
}

// combatEncounterPreviewResult carries row-major tiles and party slots keyed by entry direction.
type combatEncounterPreviewResult struct {
	Tiles        []int                            `json:"tiles"`
	Party        map[string][]debugPositionResult `json:"party"`
	MonsterSlots []debugPositionResult            `json:"monsterSlots"`
	Entities     []combatEncounterEntityResult    `json:"entities"`
	Triggers     []debugPositionResult            `json:"triggers"`
}

type combatEncounterEntityResult struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Sprite    int    `json:"sprite"`
	X         int    `json:"x"`
	Y         int    `json:"y"`
	Concealed bool   `json:"concealed"`
}

type combatEncounterMonsterResult struct {
	ID           int                          `json:"id"`
	Name         string                       `json:"name"`
	Strength     int                          `json:"strength"`
	Dexterity    int                          `json:"dexterity"`
	Intelligence int                          `json:"intelligence"`
	Armour       int                          `json:"armour"`
	Damage       int                          `json:"damage"`
	HitPoints    int                          `json:"hitPoints"`
	MaxGroup     int                          `json:"maxGroup"`
	Friend       *combatEncounterFriendResult `json:"friend,omitempty"`
}

type combatEncounterFriendResult struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// combatEncounterStartResult omits the monster for an authored room.
type combatEncounterStartResult struct {
	Territory    string                `json:"territory"`
	Contents     string                `json:"contents"`
	MapIndex     int                   `json:"mapIndex"`
	Dungeon      int                   `json:"dungeon"`
	DungeonRoom  int                   `json:"dungeonRoom"`
	MonsterType  *int                  `json:"monsterType,omitempty"`
	MonsterName  string                `json:"monsterName,omitempty"`
	Seed         *uint64               `json:"seed,omitempty"`
	PrimaryCount int                   `json:"primaryCount"`
	FriendCount  int                   `json:"friendCount"`
	TotalCount   int                   `json:"totalCount"`
	Positions    []debugPositionResult `json:"positions"`
}

type monsterEcosystemResult struct {
	LocationID         int                  `json:"locationId"`
	LocationName       string               `json:"locationName"`
	MapType            string               `json:"mapType"`
	Floor              int                  `json:"floor"`
	GenerationEnabled  bool                 `json:"generationEnabled"`
	GenerationBaseOdds int                  `json:"generationBaseOdds"`
	Capacity           int                  `json:"capacity"`
	CurrentUnitCount   int                  `json:"currentUnitCount"`
	EnemyCount         int                  `json:"enemyCount"`
	FriendlyCount      int                  `json:"friendlyCount"`
	VehicleCount       int                  `json:"vehicleCount"`
	Enemies            []monsterEnemyResult `json:"enemies"`
}

type monsterEnemyResult struct {
	RuntimeToken         int                 `json:"runtimeToken"`
	MonsterType          int                 `json:"monsterType"`
	Name                 string              `json:"name"`
	Position             debugPositionResult `json:"position"`
	Floor                int                 `json:"floor"`
	Visible              bool                `json:"visible"`
	ActivelyAttacks      bool                `json:"activelyAttacks"`
	CanFlyOverWater      bool                `json:"canFlyOverWater"`
	CanPassThroughWalls  bool                `json:"canPassThroughWalls"`
	DoNotMove            bool                `json:"doNotMove"`
	HasWhirlpoolMovement bool                `json:"hasWhirlpoolMovement"`
	SkipNextTurn         bool                `json:"skipNextTurn"`
	IsSandEnemy          bool                `json:"isSandEnemy"`
	IsWaterEnemy         bool                `json:"isWaterEnemy"`
}

type monsterClearResult struct {
	Removed           int    `json:"removed"`
	Supported         bool   `json:"supported"`
	UnsupportedReason string `json:"unsupportedReason,omitempty"`
}

type monsterSpawnResult struct {
	Spawned         bool                 `json:"spawned"`
	Position        *debugPositionResult `json:"position,omitempty"`
	RejectionReason string               `json:"rejectionReason,omitempty"`
}

type vehicleStatusResult struct {
	VehicleType             int                 `json:"vehicleType"`
	VehicleName             string              `json:"vehicleName"`
	CurrentDirection        string              `json:"currentDirection"`
	PreviousDirection       string              `json:"previousDirection"`
	OwnedByNPC              bool                `json:"ownedByNpc"`
	HullHP                  int                 `json:"hullHp"`
	MaximumHullHP           int                 `json:"maximumHullHp"`
	SailState               string              `json:"sailState"`
	SkiffQuantity           int                 `json:"skiffQuantity"`
	Position                debugPositionResult `json:"position"`
	Floor                   int                 `json:"floor"`
	HMSCapeActive           bool                `json:"hmsCapeActive"`
	RawHullDamageAvailable  bool                `json:"rawHullDamageAvailable"`
	FullHullRepairAvailable bool                `json:"fullHullRepairAvailable"`
}

type vehicleSpawnResult struct {
	VehicleType  int                 `json:"vehicleType"`
	VehicleName  string              `json:"vehicleName"`
	Placement    string              `json:"placement"`
	Dock         string              `json:"dock,omitempty"`
	LocationID   int                 `json:"locationId"`
	LocationName string              `json:"locationName"`
	Position     debugPositionResult `json:"position"`
	Floor        int                 `json:"floor"`
	UnitCount    int                 `json:"unitCount"`
}

type vehicleForceBoardResult struct {
	VehicleStatus   vehicleStatusResult `json:"vehicleStatus"`
	TerrainBypassed bool                `json:"terrainBypassed"`
	MapUnitCount    int                 `json:"mapUnitCount"`
}

func executeSecondBatchStateDebugCommand(request debugging.Request, state *game_state.GameState) debugging.Result {
	switch request.CommandID {
	case debugging.CommandCombatEncounterOptions:
		return executeCombatEncounterOptions(request, state)
	case debugging.CommandCombatEncounterStart:
		return executeCombatEncounterStart(request, state)
	case debugging.CommandMonsterEcosystemStatus:
		return debugSuccessResult(
			request,
			"Monster ecosystem status loaded.",
			newMonsterEcosystemResult(state.CurrentMonsterEcosystemSnapshot()),
		)
	case debugging.CommandMonsterGenerationSet:
		return executeMonsterGenerationSet(request, state)
	case debugging.CommandMonsterClearMapEnemies:
		return executeMonsterClearMapEnemies(request, state)
	case debugging.CommandMonsterSpawnWhirlpool:
		return executeMonsterSpawn(request, state, true)
	case debugging.CommandMonsterSpawnPirate:
		return executeMonsterSpawn(request, state, false)
	case debugging.CommandVehicleStatus:
		return debugSuccessResult(
			request,
			"Vehicle status loaded.",
			newVehicleStatusResult(state.CurrentPartyVehicleSnapshot()),
		)
	case debugging.CommandVehicleSpawn:
		return executeVehicleSpawn(request, state)
	case debugging.CommandVehicleForceBoard:
		return executeVehicleForceBoard(request, state)
	case debugging.CommandShipDamageRaw:
		return executeShipDamageRaw(request, state)
	case debugging.CommandShipRepairFull:
		return executeShipRepairFull(request, state)
	case debugging.CommandShipSkiffSet:
		return executeShipSkiffSet(request, state)
	default:
		panic("unhandled second-batch state command")
	}
}

func executeCombatEncounterOptions(request debugging.Request, state *game_state.GameState) debugging.Result {
	if state.GameReferences == nil || state.GameReferences.EnemyReferences == nil ||
		state.GameReferences.LocationReferences == nil {
		return debugging.FailureResult(request, "combat_references_unavailable", "enemy or location references are not loaded")
	}
	result := combatEncounterOptionsResult{
		BritanniaMaps:   make([]combatEncounterMapResult, 0, references.TotalCombatMaps),
		Dungeons:        make([]combatEncounterDungeonResult, 0, references.MaxDungeonCombatIndex+1),
		EntryDirections: make([]debugOptionResult, 0, references.NumDirections),
		CountModes: []debugOptionResult{
			{ID: int(game_state.CombatEncounterCountAutomatic), Name: "automatic"},
			{ID: int(game_state.CombatEncounterCountExact), Name: "exact"},
			{ID: int(game_state.CombatEncounterCountMaximum), Name: "maximum"},
		},
		Monsters: make([]combatEncounterMonsterResult, 0),
	}
	for _, entry := range debugCombatEntryDirections() {
		result.EntryDirections = append(result.EntryDirections, debugOptionResult{ID: int(entry.direction), Name: entry.name})
	}
	for index := range references.TotalCombatMaps {
		encounterMap := game_state.CombatEncounterMap{
			Territory: references.BritanniaCombatTerritory, BritanniaMap: references.BritanniaCombatMap(index),
			Dungeon: 0, DungeonRoom: 0,
		}
		preview, err := state.CombatEncounterPreview(encounterMap)
		if err != nil {
			return debugging.FailureResult(request, "combat_map_unavailable", err.Error())
		}
		name := references.NewCombatMapReference(index, references.BritanniaCombatTerritory).MapName
		result.BritanniaMaps = append(result.BritanniaMaps, combatEncounterMapResult{
			Index: index, Name: name, Preview: newCombatEncounterPreviewResult(preview),
		})
	}
	for dungeon, name := range references.DungeonNames() {
		rooms, err := combatEncounterRooms(state, references.DungeonCombatIndex(dungeon))
		if err != nil {
			return debugging.FailureResult(request, "combat_map_unavailable", err.Error())
		}
		result.Dungeons = append(result.Dungeons, combatEncounterDungeonResult{ID: dungeon, Name: name, Rooms: rooms})
	}
	for _, enemy := range *state.GameReferences.EnemyReferences {
		if enemy.MaxPerMap < 1 {
			continue
		}
		monster := combatEncounterMonsterResult{
			ID: enemy.MonsterType.Index(), Name: enemy.Name, Strength: enemy.Strength, Dexterity: enemy.Dexterity,
			Intelligence: enemy.Intelligence, Armour: enemy.Armour, Damage: enemy.Damage, HitPoints: enemy.HitPoints,
			MaxGroup: enemy.MaxPerMap, Friend: nil,
		}
		if enemy.Friend != nil && enemy.Friend.MonsterType.IsValid() {
			monster.Friend = &combatEncounterFriendResult{ID: enemy.Friend.MonsterType.Index(), Name: enemy.Friend.Name}
		}
		result.Monsters = append(result.Monsters, monster)
	}

	return debugSuccessResult(request, "Combat encounter options loaded.", result)
}

func combatEncounterRooms(
	state *game_state.GameState,
	dungeon references.DungeonCombatIndex,
) ([]combatEncounterRoomResult, error) {
	rooms := make([]combatEncounterRoomResult, 0, references.MaxDungeonRooms)
	for room := range references.MaxDungeonRooms {
		preview, err := state.CombatEncounterPreview(game_state.CombatEncounterMap{
			Territory: references.DungeonCombatTerritory, BritanniaMap: 0, Dungeon: dungeon, DungeonRoom: room,
		})
		if err != nil {
			return nil, fmt.Errorf("previewing dungeon %d room %d: %w", dungeon, room, err)
		}
		result := combatEncounterRoomResult{Room: room, Floor: nil, Cell: nil, Preview: newCombatEncounterPreviewResult(preview)}
		if placement, placed := state.DungeonRoomPlacement(dungeon, room); placed {
			floor, cell := int(placement.Floor), newDebugPosition(placement.Cell)
			result.Floor, result.Cell = &floor, &cell
		}
		rooms = append(rooms, result)
	}

	return rooms, nil
}

func newCombatEncounterPreviewResult(preview game_state.CombatEncounterPreview) combatEncounterPreviewResult {
	result := combatEncounterPreviewResult{
		Tiles:        make([]int, 0, references.XCombatMapTiles*references.YCombatMapTiles),
		Party:        make(map[string][]debugPositionResult, references.NumDirections),
		MonsterSlots: make([]debugPositionResult, 0, len(preview.MonsterSlots)),
		Entities:     make([]combatEncounterEntityResult, 0, len(preview.Entities)),
		Triggers:     make([]debugPositionResult, 0, len(preview.Triggers)),
	}
	for _, row := range preview.Tiles {
		for _, tile := range row {
			result.Tiles = append(result.Tiles, int(tile))
		}
	}
	for _, entry := range debugCombatEntryDirections() {
		positions := make([]debugPositionResult, 0, references.MaxPlayersPerMap)
		for _, position := range preview.PartyPositions[entry.direction] {
			positions = append(positions, newDebugPosition(position))
		}
		result.Party[entry.name] = positions
	}
	for _, position := range preview.MonsterSlots {
		result.MonsterSlots = append(result.MonsterSlots, newDebugPosition(position))
	}
	for _, entity := range preview.Entities {
		result.Entities = append(result.Entities, combatEncounterEntityResult{
			Kind: debugCombatEncounterEntityKindName(entity.Kind), Name: entity.Name, Sprite: int(entity.Sprite),
			X: int(entity.Position.X), Y: int(entity.Position.Y), Concealed: entity.Concealed,
		})
	}
	for _, position := range preview.Triggers {
		result.Triggers = append(result.Triggers, newDebugPosition(position))
	}

	return result
}

func executeCombatEncounterStart(request debugging.Request, state *game_state.GameState) debugging.Result {
	input, err := combatEncounterRequestFromDebug(request)
	if err != nil {
		return debugging.FailureResult(request, "invalid_combat_encounter", err.Error())
	}
	encounter, err := state.StartCombatEncounter(input)
	if err != nil {
		switch {
		case errors.Is(err, game_state.ErrCombatEncounterAlreadyActive):
			return debugging.FailureResult(request, "combat_encounter_active", err.Error())
		case errors.Is(err, game_state.ErrInvalidCombatEncounter):
			return debugging.FailureResult(request, "invalid_combat_encounter", err.Error())
		default:
			return debugging.FailureResult(request, "combat_encounter_start_failed", err.Error())
		}
	}
	positions := make([]debugPositionResult, 0, len(encounter.Positions))
	for _, position := range encounter.Positions {
		positions = append(positions, newDebugPosition(position))
	}
	result := combatEncounterStartResult{
		Territory: debugCombatTerritoryName(encounter.Territory), Contents: debugCombatContentsCustom,
		MapIndex: encounter.MapIndex, Dungeon: int(encounter.Dungeon), DungeonRoom: encounter.DungeonRoom,
		MonsterType: nil, MonsterName: "", Seed: encounter.Seed,
		PrimaryCount: encounter.PrimaryCount, FriendCount: encounter.FriendCount,
		TotalCount: encounter.PrimaryCount + encounter.FriendCount, Positions: positions,
	}
	if input.Authored {
		result.Contents = debugCombatContentsAuthored
	} else {
		monsterType := input.MonsterType.Index()
		result.MonsterType, result.MonsterName = &monsterType, input.MonsterType.Name()
	}
	debugResult := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		"Combat encounter started.",
		result,
	)
	appendSemanticChange(&debugResult.Changes, "combat.active", false, true)

	return debugResult
}

func combatEncounterRequestFromDebug(request debugging.Request) (game_state.CombatEncounterRequest, error) {
	input := game_state.CombatEncounterRequest{
		Map: game_state.CombatEncounterMap{
			Territory: 0, BritanniaMap: 0, Dungeon: 0, DungeonRoom: 0,
		},
		EntryDirection: 0, MonsterType: references.MonsterNone,
		CountMode: 0, ExactCount: 0, FriendsEnabled: true, Seed: nil,
	}
	switch mustDebugStringArgument(request, "territory") {
	case "britannia":
		value, supplied, err := optionalDebugIntegerArgument(request, "britanniaMap")
		if err != nil || !supplied {
			return input, invalidSecondBatchArgument("Britannia territory requires britanniaMap")
		}
		input.Map.Territory = references.BritanniaCombatTerritory
		input.Map.BritanniaMap = references.BritanniaCombatMap(value)
	case debugMapDungeon:
		dungeon, dungeonSet, err := optionalDebugIntegerArgument(request, "dungeon")
		if err != nil {
			return input, err
		}
		room, roomSet, err := optionalDebugIntegerArgument(request, "dungeonRoom")
		if err != nil {
			return input, err
		}
		if !dungeonSet || !roomSet {
			return input, invalidSecondBatchArgument("dungeon territory requires dungeon and dungeonRoom")
		}
		input.Map.Territory = references.DungeonCombatTerritory
		input.Map.Dungeon, input.Map.DungeonRoom = references.DungeonCombatIndex(dungeon), room
	default:
		return input, invalidSecondBatchArgument("unsupported combat territory")
	}
	entry, found := debugCombatEntryDirection(mustDebugStringArgument(request, "entryDirection"))
	if !found {
		return input, invalidSecondBatchArgument("unsupported combat entry direction")
	}
	input.EntryDirection = entry
	contents, _, err := optionalDebugStringArgument(request, "contents")
	if err != nil {
		return input, err
	}
	switch contents {
	case "", debugCombatContentsCustom:
		err = applyCustomEncounterArguments(request, &input)
	case debugCombatContentsAuthored:
		input.Authored = true
		err = rejectCustomEncounterArguments(request)
	default:
		err = invalidSecondBatchArgument("unsupported combat contents")
	}
	if err != nil {
		return input, err
	}
	if seed, set, optionalErr := optionalDebugIntegerArgument(request, "seed"); optionalErr != nil {
		return input, optionalErr
	} else if set {
		if seed < 0 {
			return input, invalidSecondBatchArgument("seed cannot be negative")
		}
		value := uint64(seed)
		input.Seed = &value
	}

	return input, nil
}

func executeMonsterGenerationSet(request debugging.Request, state *game_state.GameState) debugging.Result {
	before := state.CurrentMonsterEcosystemSnapshot()
	enabled, _, err := optionalDebugBooleanArgument(request, "enabled")
	if err != nil {
		return debugging.FailureResult(request, "invalid_monster_generation", err.Error())
	}
	odds := mustDebugIntegerArgument(request, "odds")
	if err = state.SetMonsterGenerationBaseOdds(odds); err != nil {
		return debugging.FailureResult(request, "invalid_monster_generation", err.Error())
	}
	state.SetMonsterGenerationEnabled(enabled)
	after := state.CurrentMonsterEcosystemSnapshot()
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionLiveSafe,
		"Monster generation updated.",
		newMonsterEcosystemResult(after),
	)
	appendSemanticChange(&result.Changes, "monster.generation.enabled", before.GenerationEnabled, after.GenerationEnabled)
	appendSemanticChange(
		&result.Changes,
		"monster.generation.baseOdds",
		before.GenerationBaseOdds,
		after.GenerationBaseOdds,
	)

	return result
}

func executeMonsterClearMapEnemies(request debugging.Request, state *game_state.GameState) debugging.Result {
	snapshot := state.CurrentMonsterEcosystemSnapshot()
	expectedLocationID := mustDebugIntegerArgument(request, "expectedLocationId")
	expectedFloor := mustDebugIntegerArgument(request, "expectedFloor")
	expectedEnemyCount := mustDebugIntegerArgument(request, "expectedEnemyCount")
	if int(snapshot.Location) != expectedLocationID || int(snapshot.Floor) != expectedFloor ||
		snapshot.EnemyCount != expectedEnemyCount {
		return debugging.FailureResult(
			request,
			"stale_monster_ecosystem",
			"the current map or enemy roster changed after clear review",
		)
	}
	before := snapshot.EnemyCount
	removed, supported := state.ClearCurrentMapEnemies()
	data := monsterClearResult{Removed: removed, Supported: supported, UnsupportedReason: ""}
	if !supported {
		data.UnsupportedReason = "current map controller does not support authoritative enemy clearing"
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionAlteration,
		fmt.Sprintf("Removed %d current-map enemies.", removed),
		data,
	)
	if supported {
		after := state.CurrentMonsterEcosystemSnapshot().EnemyCount
		appendSemanticChange(&result.Changes, "monster.ecosystem.enemyCount", before, after)
	}

	return result
}

func executeMonsterSpawn(request debugging.Request, state *game_state.GameState, whirlpool bool) debugging.Result {
	before := state.CurrentMonsterEcosystemSnapshot().EnemyCount
	var position references.Position
	var spawned bool
	if whirlpool {
		position, spawned = state.ForceSpawnWhirlpoolAtRandomLocationWithPosition()
	} else {
		position, spawned = state.ForceSpawnPirateAtRandomLocationWithPosition()
	}
	data := monsterSpawnResult{Spawned: spawned, Position: nil, RejectionReason: ""}
	name := "Pirate"
	if whirlpool {
		name = "Whirlpool"
	}
	if !spawned {
		data.RejectionReason = monsterSpawnRejection(state, whirlpool)

		return debugSuccessResultWithMode(request, debugging.ExecutionLiveSafe, name+" spawn rejected.", data)
	}
	positionResult := newDebugPosition(position)
	data.Position = &positionResult
	result := debugSuccessResultWithMode(request, debugging.ExecutionLiveSafe, name+" spawned.", data)
	appendSemanticChange(
		&result.Changes,
		"monster.ecosystem.enemyCount",
		before,
		state.CurrentMonsterEcosystemSnapshot().EnemyCount,
	)

	return result
}

func monsterSpawnRejection(state *game_state.GameState, whirlpool bool) string {
	if state.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return "current map is not a Britannia large map"
	}
	if whirlpool && state.HasAnyWhirlpool() {
		return "a whirlpool is already present"
	}

	return "no valid off-screen deep-water spawn position was available"
}

func executeShipDamageRaw(request debugging.Request, state *game_state.GameState) debugging.Result {
	operation, err := state.ApplyRawPartyVehicleHullDamage(mustDebugIntegerArgument(request, "amount"))
	if err != nil {
		return debugging.FailureResult(request, "invalid_ship_hull_operation", err.Error())
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		"Ship hull damaged.",
		newVehicleStatusResult(operation.After),
	)
	appendSemanticChange(&result.Changes, "vehicle.hullHp", operation.Before.HullHP, operation.After.HullHP)

	return result
}

func executeVehicleSpawn(request debugging.Request, state *game_state.GameState) debugging.Result {
	vehicleType, ok := debugVehicleType(mustDebugStringArgument(request, "type"))
	if !ok || (vehicleType != references.FrigateVehicle && vehicleType != references.SkiffVehicle) {
		return vehicleDebugFailure(request, game_state.ErrDebugVehicleUnsupportedType)
	}
	placementValue := mustDebugStringArgument(request, "placement")
	spawnRequest := game_state.DebugVehicleSpawnRequest{VehicleType: vehicleType}
	switch placementValue {
	case debugVehicleAvatar:
		spawnRequest.Placement = game_state.DebugVehicleSpawnAtAvatar
	case debugVehicleDock:
		spawnRequest.Placement = game_state.DebugVehicleSpawnAtDock
		dockValue, exists := request.Arguments["dock"]
		if !exists || len(dockValue) == 0 || string(dockValue) == "null" {
			return debugging.FailureResult(request, "vehicle_dock_required", "dock is required when placement is dock")
		}
		dock, found := debugVehicleDockLocation(mustDebugStringArgument(request, "dock"))
		if !found {
			return vehicleDebugFailure(request, game_state.ErrDebugVehicleInvalidLocation)
		}
		spawnRequest.DockLocation = dock
	default:
		return vehicleDebugFailure(request, game_state.ErrDebugVehicleInvalidLocation)
	}

	operation, err := state.SpawnDebugVehicle(spawnRequest)
	if err != nil {
		return vehicleDebugFailure(request, err)
	}
	data := vehicleSpawnResult{
		VehicleType: int(vehicleType), VehicleName: debugVehicleName(vehicleType), Placement: placementValue,
		LocationID: int(operation.After.Location), LocationName: operation.After.Location.String(),
		Position: newDebugPosition(operation.After.Position), Floor: int(operation.After.Floor),
		UnitCount: operation.After.UnitCount,
	}
	if placementValue == debugVehicleDock {
		data.Dock = debugVehicleDockName(operation.After.DockLocation)
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionSynchronized, "Vehicle spawned.", data)
	appendSemanticChange(&result.Changes, "vehicle.map.unitCount", operation.Before.UnitCount, operation.After.UnitCount)
	appendSemanticChange(&result.Changes, "vehicle.map.targetVehicleType", nil, debugVehicleName(vehicleType))

	return result
}

func executeVehicleForceBoard(request debugging.Request, state *game_state.GameState) debugging.Result {
	vehicleType, ok := debugVehicleType(mustDebugStringArgument(request, "type"))
	if !ok {
		return vehicleDebugFailure(request, game_state.ErrDebugVehicleUnsupportedType)
	}
	operation, err := state.ForceBoardDebugVehicle(game_state.DebugForceBoardRequest{VehicleType: vehicleType})
	if err != nil {
		return vehicleDebugFailure(request, err)
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionSynchronized, "Party force-boarded vehicle; terrain restrictions bypassed.", vehicleForceBoardResult{
		VehicleStatus: newVehicleStatusResult(operation.PartyAfter), TerrainBypassed: true, MapUnitCount: operation.MapAfter.UnitCount,
	})
	appendSemanticChange(&result.Changes, "vehicle.type", debugVehicleName(operation.PartyBefore.VehicleType), debugVehicleName(operation.PartyAfter.VehicleType))
	appendSemanticChange(&result.Changes, "vehicle.skiffQuantity", operation.PartyBefore.SkiffQuantity, operation.PartyAfter.SkiffQuantity)
	appendSemanticChange(&result.Changes, "vehicle.map.unitCount", operation.MapBefore.UnitCount, operation.MapAfter.UnitCount)

	return result
}

func vehicleDebugFailure(request debugging.Request, err error) debugging.Result {
	switch {
	case errors.Is(err, game_state.ErrDebugVehicleUnsupportedType):
		return debugging.FailureResult(request, "unsupported_vehicle_type", "vehicle type is not supported by this operation")
	case errors.Is(err, game_state.ErrDebugVehicleUnsupportedContext):
		return debugging.FailureResult(request, "unsupported_vehicle_context", "vehicle operation is not supported in the current gameplay context")
	case errors.Is(err, game_state.ErrDebugVehicleInvalidLocation):
		return debugging.FailureResult(request, "invalid_vehicle_location", "vehicle destination is invalid")
	case errors.Is(err, game_state.ErrDebugVehicleMapUnavailable):
		return debugging.FailureResult(request, "vehicle_map_unavailable", "authoritative vehicle map controller is unavailable")
	case errors.Is(err, game_state.ErrDebugVehiclePositionOccupied):
		return debugging.FailureResult(request, "vehicle_position_occupied", "vehicle destination is occupied")
	case errors.Is(err, game_state.ErrDebugVehicleMapFull):
		return debugging.FailureResult(request, "vehicle_map_full", "vehicle destination map has no free unit slot")
	default:
		panic("unhandled debug vehicle error: " + err.Error())
	}
}

func executeShipRepairFull(request debugging.Request, state *game_state.GameState) debugging.Result {
	operation, err := state.RepairPartyVehicleHullToMaximum()
	if err != nil {
		return debugging.FailureResult(request, "invalid_ship_hull_operation", err.Error())
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		"Ship hull fully repaired.",
		newVehicleStatusResult(operation.After),
	)
	appendSemanticChange(&result.Changes, "vehicle.hullHp", operation.Before.HullHP, operation.After.HullHP)

	return result
}

func executeShipSkiffSet(request debugging.Request, state *game_state.GameState) debugging.Result {
	operation, err := state.SetPartyFrigateSkiffQuantity(mustDebugIntegerArgument(request, "quantity"))
	if err != nil {
		return debugging.FailureResult(request, "invalid_ship_skiff_operation", err.Error())
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		"Frigate skiff quantity updated.",
		newVehicleStatusResult(operation.After),
	)
	appendSemanticChange(
		&result.Changes,
		"vehicle.skiffQuantity",
		operation.Before.SkiffQuantity,
		operation.After.SkiffQuantity,
	)

	return result
}

func newMonsterEcosystemResult(snapshot game_state.MonsterEcosystemSnapshot) monsterEcosystemResult {
	result := monsterEcosystemResult{
		LocationID: int(snapshot.Location), LocationName: snapshot.Location.String(),
		MapType: debugMapTypeName(snapshot.MapType),
		Floor:   int(snapshot.Floor), GenerationEnabled: snapshot.GenerationEnabled,
		GenerationBaseOdds: snapshot.GenerationBaseOdds, Capacity: snapshot.Capacity,
		CurrentUnitCount: snapshot.CurrentUnitCount, EnemyCount: snapshot.EnemyCount,
		FriendlyCount: snapshot.FriendlyCount, VehicleCount: snapshot.VehicleCount,
		Enemies: make([]monsterEnemyResult, 0, len(snapshot.Enemies)),
	}
	for _, enemy := range snapshot.Enemies {
		result.Enemies = append(result.Enemies, monsterEnemyResult{
			RuntimeToken: enemy.RuntimeToken, MonsterType: enemy.MonsterType.Index(), Name: enemy.Name,
			Position: newDebugPosition(enemy.Position), Floor: int(enemy.Floor), Visible: enemy.Visible,
			ActivelyAttacks: enemy.ActivelyAttacks, CanFlyOverWater: enemy.CanFlyOverWater,
			CanPassThroughWalls: enemy.CanPassThroughWalls, DoNotMove: enemy.DoNotMove,
			HasWhirlpoolMovement: enemy.HasWhirlpoolMovement, SkipNextTurn: enemy.SkipNextTurn,
			IsSandEnemy: enemy.IsSandEnemy, IsWaterEnemy: enemy.IsWaterEnemy,
		})
	}

	return result
}

func newVehicleStatusResult(snapshot game_state.PartyVehicleSnapshot) vehicleStatusResult {
	return vehicleStatusResult{
		VehicleType: int(snapshot.VehicleType), VehicleName: debugVehicleName(snapshot.VehicleType),
		CurrentDirection:  debugDirectionName(snapshot.CurrentDirection),
		PreviousDirection: debugDirectionName(snapshot.PreviousDirection),
		OwnedByNPC:        snapshot.IsOwnedByNPC, HullHP: snapshot.HullHP, MaximumHullHP: snapshot.MaxHullHP,
		SailState: snapshot.SailState.String(), SkiffQuantity: snapshot.SkiffQuantity,
		Position: newDebugPosition(snapshot.Position), Floor: int(snapshot.Floor), HMSCapeActive: snapshot.HMSCapeActive,
		RawHullDamageAvailable: snapshot.RawHullDamageAvailable, FullHullRepairAvailable: snapshot.FullHullRepairAvailable,
	}
}

func newDebugPosition(position references.Position) debugPositionResult {
	return debugPositionResult{X: int(position.X), Y: int(position.Y)}
}

func debugCombatEntryDirection(value string) (game_state.CombatEntryDirection, bool) {
	switch value {
	case debugDirectionWestName:
		return game_state.CombatEntryWest, true
	case debugDirectionEastName:
		return game_state.CombatEntryEast, true
	case debugDirectionNorthName:
		return game_state.CombatEntryNorth, true
	case debugDirectionSouthName:
		return game_state.CombatEntrySouth, true
	default:
		return 0, false
	}
}

// applyCustomEncounterArguments reads the monster family and count for custom contents.
func applyCustomEncounterArguments(request debugging.Request, input *game_state.CombatEncounterRequest) error {
	monsterArgument, monsterSet, err := optionalDebugStringArgument(request, "monsterType")
	if err != nil {
		return err
	}
	monsterValue, convertErr := strconv.Atoi(monsterArgument)
	if !monsterSet || convertErr != nil || !references.MonsterType(monsterValue).IsValid() {
		return invalidSecondBatchArgument("custom contents require a valid monsterType")
	}
	input.MonsterType = references.MonsterType(monsterValue)
	countArgument, _, err := optionalDebugStringArgument(request, "countMode")
	if err != nil {
		return err
	}
	countMode, ok := debugCombatCountMode(countArgument)
	if !ok {
		return invalidSecondBatchArgument("custom contents require a supported countMode")
	}
	input.CountMode = countMode
	exactCount, exactSet, err := optionalDebugIntegerArgument(request, "exactCount")
	if err != nil {
		return err
	}
	if countMode == game_state.CombatEncounterCountExact && !exactSet {
		return invalidSecondBatchArgument("exact count mode requires exactCount")
	}
	if countMode != game_state.CombatEncounterCountExact && exactSet {
		return invalidSecondBatchArgument("exactCount is only valid with exact count mode")
	}
	input.ExactCount = exactCount
	friends, friendsSet, err := optionalDebugBooleanArgument(request, "friends")
	if err != nil {
		return err
	}
	if friendsSet {
		input.FriendsEnabled = friends
	}

	return nil
}

// rejectCustomEncounterArguments keeps monster choices out of an authored room. The
// catalog's default friends value is ignored there.
func rejectCustomEncounterArguments(request debugging.Request) error {
	for _, argumentID := range []string{"monsterType", "countMode", "exactCount"} {
		if _, supplied := request.Arguments[argumentID]; supplied {
			return invalidSecondBatchArgument(argumentID + " is only valid with custom contents")
		}
	}

	return nil
}

func debugCombatEntryDirections() []struct {
	direction game_state.CombatEntryDirection
	name      string
} {
	return []struct {
		direction game_state.CombatEntryDirection
		name      string
	}{
		{game_state.CombatEntryWest, debugDirectionWestName},
		{game_state.CombatEntryEast, debugDirectionEastName},
		{game_state.CombatEntryNorth, debugDirectionNorthName},
		{game_state.CombatEntrySouth, debugDirectionSouthName},
	}
}

func debugCombatEncounterEntityKindName(kind game_state.CombatEncounterEntityKind) string {
	switch kind {
	case game_state.CombatEncounterEntityMonster:
		return "monster"
	case game_state.CombatEncounterEntityRandomMonster:
		return "random"
	case game_state.CombatEncounterEntityObject:
		return "object"
	case game_state.CombatEncounterEntityLoot:
		return "loot"
	case game_state.CombatEncounterEntityField:
		return "field"
	default:
		panic("unknown combat encounter entity kind")
	}
}

func debugCombatCountMode(value string) (game_state.CombatEncounterCountMode, bool) {
	switch value {
	case "automatic":
		return game_state.CombatEncounterCountAutomatic, true
	case "exact":
		return game_state.CombatEncounterCountExact, true
	case "maximum":
		return game_state.CombatEncounterCountMaximum, true
	default:
		return 0, false
	}
}

func debugCombatTerritoryName(territory references.CombatMapTerritory) string {
	if territory == references.DungeonCombatTerritory {
		return debugMapDungeon
	}

	return "britannia"
}

func debugVehicleName(vehicleType references.VehicleType) string {
	switch vehicleType {
	case references.NoPartyVehicle:
		return debugVehicleNone
	case references.CarpetVehicle:
		return debugQuestCarpet
	case references.HorseVehicle:
		return debugVehicleHorse
	case references.SkiffVehicle:
		return debugVehicleSkiff
	case references.FrigateVehicle:
		return debugVehicleFrigate
	case references.NPC:
		return "npc"
	default:
		return debugMapUnknown
	}
}

func debugVehicleType(value string) (references.VehicleType, bool) {
	switch value {
	case debugVehicleHorse:
		return references.HorseVehicle, true
	case debugVehicleCarpet:
		return references.CarpetVehicle, true
	case debugVehicleSkiff:
		return references.SkiffVehicle, true
	case debugVehicleFrigate:
		return references.FrigateVehicle, true
	default:
		return references.NoPartyVehicle, false
	}
}

func debugVehicleDockLocation(value string) (references.Location, bool) {
	switch value {
	case "jhelom":
		return references.Jhelom, true
	case "minoc":
		return references.Minoc, true
	case "east_britanny":
		return references.EastBritanny, true
	case "buccaneers_den":
		return references.BuccaneersDen, true
	default:
		return 0, false
	}
}

func debugVehicleDockName(location references.Location) string {
	for _, dock := range [...]struct {
		location references.Location
		name     string
	}{
		{references.Jhelom, "jhelom"},
		{references.Minoc, "minoc"},
		{references.EastBritanny, "east_britanny"},
		{references.BuccaneersDen, "buccaneers_den"},
	} {
		if location == dock.location {
			return dock.name
		}
	}

	return debugMapUnknown
}

func debugDirectionName(direction references.Direction) string {
	switch direction {
	case references.NoneDirection:
		return debugVehicleNone
	case references.Up:
		return debugDirectionNorthName
	case references.Down:
		return debugDirectionSouthName
	case references.Left:
		return debugDirectionWestName
	case references.Right:
		return debugDirectionEastName
	default:
		return debugMapUnknown
	}
}

func invalidSecondBatchArgument(message string) error {
	return fmt.Errorf("%w: %s", errInvalidSecondBatchDebugArgument, message)
}
