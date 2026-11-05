package main

import (
	"errors"
	"fmt"
	"sort"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	activeCombatForecastSize = 10
	activeCombatMonsterKind  = "monster"
	activeCombatDeadStatus   = "dead"
	activeCombatBritannia    = "britannia"
)

var (
	errInvalidCombatEncounter = errors.New("invalid encounter identity")
	errInvalidCombatMonster   = errors.New("invalid monster type")
	errInvalidCombatCountMode = errors.New("unsupported combat count mode")
	errMissingExactCount      = errors.New("exact count mode requires exactCount")
	errUnexpectedExactCount   = errors.New("exactCount is only valid with exact count mode")
)

type activeCombatResult struct {
	Active                   bool                            `json:"active"`
	UnavailableReason        string                          `json:"unavailableReason,omitempty"`
	SnapshotFingerprint      string                          `json:"snapshotFingerprint"`
	EncounterID              string                          `json:"encounterId,omitempty"`
	Map                      *activeCombatMapResult          `json:"map,omitempty"`
	Party                    []activeCombatPartyResult       `json:"party"`
	Monsters                 []activeCombatMonsterResult     `json:"monsters"`
	Forecast                 []activeCombatForecastResult    `json:"forecast"`
	ActiveMonsterCount       int                             `json:"activeMonsterCount"`
	ActiveHostileCount       int                             `json:"activeHostileMonsterCount"`
	FixedDungeonRoom         bool                            `json:"fixedDungeonRoom"`
	DungeonIndex             int                             `json:"dungeonIndex,omitempty"`
	DungeonRoom              int                             `json:"dungeonRoom,omitempty"`
	TotalMonsterCount        int                             `json:"totalMonsterCount"`
	DefeatedExperience       int                             `json:"defeatedExperience"`
	RNGSeed                  uint64                          `json:"rngSeed"`
	WorkflowActive           bool                            `json:"workflowActive"`
	PresentationSpeedPercent int                             `json:"presentationSpeedPercent"`
	ConfigurationEditing     bool                            `json:"configurationEditing"`
	MonsterTypes             []activeCombatMonsterTypeResult `json:"monsterTypes"`
}

type activeCombatMonsterTypeResult struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type activeCombatMapResult struct {
	Name               string                   `json:"name"`
	Index              int                      `json:"index"`
	Territory          string                   `json:"territory"`
	EntryDirection     string                   `json:"entryDirection"`
	Victory            bool                     `json:"victory"`
	TurnStarted        bool                     `json:"turnStarted"`
	PendingAdvanceTurn bool                     `json:"pendingAdvanceTurn"`
	SoloMoveMode       bool                     `json:"soloMoveMode"`
	CurrentPartyIndex  int                      `json:"currentPartyIndex"`
	ActiveMonsterIndex int                      `json:"activeMonsterIndex"`
	TargetPartyIndex   int                      `json:"targetPartyIndex"`
	ObjectCount        int                      `json:"objectCount"`
	Cells              []activeCombatCellResult `json:"cells"`
}

type activeCombatCellResult struct {
	X            int    `json:"x"`
	Y            int    `json:"y"`
	TileID       int    `json:"tileId"`
	OccupantKind string `json:"occupantKind,omitempty"`
	OccupantID   string `json:"occupantId,omitempty"`
	OccupantName string `json:"occupantName,omitempty"`
}

type activeCombatPartyResult struct {
	Index          int    `json:"index"`
	Name           string `json:"name"`
	X              int    `json:"x"`
	Y              int    `json:"y"`
	Active         bool   `json:"active"`
	CurrentActor   bool   `json:"currentActor"`
	CurrentTarget  bool   `json:"currentTarget"`
	CurrentHP      int    `json:"currentHp"`
	MaximumHP      int    `json:"maximumHp"`
	CurrentMP      int    `json:"currentMp"`
	MaximumMP      int    `json:"maximumMp"`
	Dexterity      int    `json:"dexterity"`
	Status         string `json:"status"`
	QueueCountdown int    `json:"queueCountdown"`
	QueueStatus    string `json:"queueStatus"`
	CanAct         bool   `json:"canAct"`
}

type activeCombatMonsterResult struct {
	ID             string   `json:"id"`
	ManagerIndex   int      `json:"managerIndex"`
	TypeID         int      `json:"typeId"`
	Name           string   `json:"name"`
	X              int      `json:"x"`
	Y              int      `json:"y"`
	CurrentHP      int      `json:"currentHp"`
	MaximumHP      int      `json:"maximumHp"`
	CurrentDex     int      `json:"currentDexterity"`
	BaseDex        int      `json:"baseDexterity"`
	Active         bool     `json:"active"`
	Dead           bool     `json:"dead"`
	Invisible      bool     `json:"invisible"`
	Asleep         bool     `json:"asleep"`
	Poisoned       bool     `json:"poisoned"`
	CurrentActor   bool     `json:"currentActor"`
	QueueCountdown int      `json:"queueCountdown"`
	QueueStatus    string   `json:"queueStatus"`
	CanAct         bool     `json:"canAct"`
	Abilities      []string `json:"abilities"`
	Hostile        bool     `json:"hostile"`
}

type activeCombatForecastResult struct {
	Order          int    `json:"order"`
	Kind           string `json:"kind"`
	ID             string `json:"id"`
	Name           string `json:"name"`
	Dexterity      int    `json:"dexterity"`
	ResetCountdown int    `json:"resetCountdown"`
	CanAct         bool   `json:"canAct"`
	Status         string `json:"status"`
}

func executeActiveCombatDebugCommand(
	request debugging.Request,
	state *game_state.GameState,
) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandCombatStatus:
		return debugSuccessResult(request, "Active combat status loaded.", newActiveCombatResult(state)), true
	case debugging.CommandCombatMonsterSleepSet:
		return executeActiveCombatSleepSet(request, state), true
	case debugging.CommandCombatMonsterSleepAll:
		return executeActiveCombatSleepAll(request, state), true
	case debugging.CommandCombatExitDebug:
		return executeActiveCombatExit(request, state), true
	case debugging.CommandCombatMonsterSpawn:
		return executeActiveCombatMonsterSpawn(request, state), true
	case debugging.CommandRNGSeedSet:
		return executeActiveCombatRNGSeedSet(request, state), true
	case debugging.CommandCombatMonsterDamageRaw:
		return executeActiveCombatMonsterDamage(request, state), true
	case debugging.CommandCombatMonsterKill:
		return executeActiveCombatMonsterKill(request, state), true
	case debugging.CommandCombatMonsterKillAll:
		return executeActiveCombatMonsterBatch(request, state, false), true
	case debugging.CommandCombatDungeonRoomClear:
		return executeActiveCombatDungeonRoomClear(request, state), true
	case debugging.CommandCombatMonsterClear:
		return executeActiveCombatMonsterBatch(request, state, true), true
	default:
		return debugging.Result{}, false
	}
}

func executeActiveCombatDungeonRoomClear(request debugging.Request, state *game_state.GameState) debugging.Result {
	_, encounterID, failure := reviewedActiveCombatRequest(request, state)
	if failure != nil {
		return *failure
	}
	expected, err := state.DebugActiveCombatSnapshot(encounterID)
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	mutation, err := state.ClearDebugDungeonRoomEnemies(game_state.DebugCombatBatchRequest{
		EncounterID: encounterID,
		Expected:    expected,
	})
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}

	return activeCombatMutationSuccess(
		request,
		"clear-room",
		"All reviewed fixed-room enemies defeated; room victory declared.",
		mutation,
	)
}

type activeCombatRNGSeedResult struct {
	EncounterID string `json:"encounterId"`
	BeforeSeed  uint64 `json:"beforeSeed"`
	AfterSeed   uint64 `json:"afterSeed"`
	Scope       string `json:"scope"`
}

type activeCombatHPChangeResult struct {
	MonsterID string `json:"monsterId"`
	BeforeHP  int    `json:"beforeHp"`
	AfterHP   int    `json:"afterHp"`
	Defeated  bool   `json:"defeated"`
}

type activeCombatMutationResult struct {
	EncounterID       string                       `json:"encounterId"`
	Operation         string                       `json:"operation"`
	RNGSeed           uint64                       `json:"rngSeed"`
	Changes           []activeCombatHPChangeResult `json:"changes"`
	ActiveCountBefore int                          `json:"activeCountBefore"`
	ActiveCountAfter  int                          `json:"activeCountAfter"`
	QueueCountBefore  int                          `json:"queueMonsterCountBefore"`
	QueueCountAfter   int                          `json:"queueMonsterCountAfter"`
	ObjectsBefore     int                          `json:"objectCountBefore"`
	ObjectsAfter      int                          `json:"objectCountAfter"`
	VictoryBefore     bool                         `json:"victoryBefore"`
	VictoryAfter      bool                         `json:"victoryAfter"`
	ExperienceAwarded int                          `json:"experienceAwarded"`
	ObjectsCreated    int                          `json:"objectsCreated"`
	VictoryTriggered  bool                         `json:"victoryTriggered"`
	Policy            string                       `json:"policy"`
}

func executeActiveCombatRNGSeedSet(request debugging.Request, state *game_state.GameState) debugging.Result {
	status, encounterID, failure := reviewedActiveCombatRequest(request, state)
	if failure != nil {
		return *failure
	}
	if _, err := state.DebugActiveCombatSnapshot(encounterID); err != nil {
		return activeCombatMutationFailure(request, err)
	}
	before := state.RandomSeed()
	after := uint64(mustDebugIntegerArgument(request, "seed")) //nolint:gosec // Catalog bounds require a positive integer.
	state.SetRandomSeed(after)
	data := activeCombatRNGSeedResult{
		EncounterID: status.EncounterID, BeforeSeed: before, AfterSeed: after,
		Scope: "global_gameplay_rng_including_post_combat",
	}
	result := debugSuccessResultWithMode(
		request, debugging.ExecutionSynchronized,
		"Global gameplay RNG reset; subsequent combat and post-combat randomness use the new seed.", data,
	)
	appendSemanticChange(&result.Changes, "gameplay.rng.seed", before, after)

	return result
}

func executeActiveCombatMonsterDamage(request debugging.Request, state *game_state.GameState) debugging.Result {
	encounterID, combatantID, failure := reviewedActiveCombatMonsterRequest(request, state)
	if failure != nil {
		return *failure
	}
	mutation, err := state.DamageDebugCombatMonster(game_state.DebugCombatDamageRequest{
		EncounterID: encounterID, CombatantID: combatantID,
		ExpectedHP: mustDebugIntegerArgument(request, "expectedCurrentHp"),
		Damage:     mustDebugIntegerArgument(request, "damage"),
	})
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}

	return activeCombatMutationSuccess(
		request, "damage-raw", "Reviewed monster damage applied under the no-reward debug policy.", mutation,
	)
}

func executeActiveCombatMonsterKill(request debugging.Request, state *game_state.GameState) debugging.Result {
	encounterID, combatantID, failure := reviewedActiveCombatMonsterRequest(request, state)
	if failure != nil {
		return *failure
	}
	mutation, err := state.KillDebugCombatMonster(game_state.DebugCombatKillRequest{
		EncounterID: encounterID, CombatantID: combatantID,
		ExpectedHP: mustDebugIntegerArgument(request, "expectedCurrentHp"),
	})
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}

	return activeCombatMutationSuccess(
		request, "kill", "Reviewed monster defeated under the no-reward debug policy.", mutation,
	)
}

func executeActiveCombatMonsterBatch(
	request debugging.Request,
	state *game_state.GameState,
	removeWithoutOutcome bool,
) debugging.Result {
	_, encounterID, failure := reviewedActiveCombatRequest(request, state)
	if failure != nil {
		return *failure
	}
	expected, err := state.DebugActiveCombatSnapshot(encounterID)
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	batch := game_state.DebugCombatBatchRequest{EncounterID: encounterID, Expected: expected}
	var mutation *game_state.DebugCombatMutationResult
	if removeWithoutOutcome {
		mutation, err = state.ClearDebugCombatMonsters(batch)
	} else {
		mutation, err = state.KillAllDebugCombatMonsters(batch)
	}
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	if removeWithoutOutcome {
		return activeCombatMutationSuccess(
			request, "clear", "Reviewed monsters removed without resolving a combat outcome.", mutation,
		)
	}

	return activeCombatMutationSuccess(
		request, "kill-all", "All reviewed monsters defeated under the no-reward debug policy.", mutation,
	)
}

func reviewedActiveCombatMonsterRequest(
	request debugging.Request,
	state *game_state.GameState,
) (game_state.CombatEncounterID, game_state.CombatantID, *debugging.Result) {
	_, encounterID, failure := reviewedActiveCombatRequest(request, state)
	if failure != nil {
		return 0, 0, failure
	}
	monsterID, err := strconv.ParseUint(mustDebugStringArgument(request, "monsterId"), 10, 64)
	if err != nil {
		result := debugging.FailureResult(request, "invalid_combat_target", "invalid monster identity")

		return 0, 0, &result
	}

	return encounterID, game_state.CombatantID(monsterID), nil
}

func reviewedActiveCombatRequest(
	request debugging.Request,
	state *game_state.GameState,
) (activeCombatResult, game_state.CombatEncounterID, *debugging.Result) {
	status := newActiveCombatResult(state)
	if status.SnapshotFingerprint != mustDebugStringArgument(request, "expectedSnapshot") {
		result := debugging.FailureResult(request, "stale_combat", "combat changed after review")

		return status, 0, &result
	}
	encounterID, err := strconv.ParseUint(mustDebugStringArgument(request, "encounterId"), 10, 64)
	if err != nil {
		result := debugging.FailureResult(request, "invalid_combat_target", "invalid encounter identity")

		return status, 0, &result
	}

	return status, game_state.CombatEncounterID(encounterID), nil
}

func activeCombatMutationSuccess(
	request debugging.Request,
	operation string,
	summary string,
	mutation *game_state.DebugCombatMutationResult,
) debugging.Result {
	encounterID := strconv.FormatUint(uint64(mutation.EncounterID), 10)
	policy := "no_xp_no_objects_queue_synchronized_victory_enabled"
	if operation == "clear" {
		policy = "outcome_free_no_xp_no_objects_no_victory"
	}
	data := activeCombatMutationResult{
		EncounterID: encounterID, Operation: operation, RNGSeed: mutation.RNGSeed,
		Changes:           make([]activeCombatHPChangeResult, 0, len(mutation.Changes)),
		ActiveCountBefore: mutation.Before.ActiveCount, ActiveCountAfter: mutation.After.ActiveCount,
		QueueCountBefore: mutation.Before.QueueMonsterCount, QueueCountAfter: mutation.After.QueueMonsterCount,
		ObjectsBefore: mutation.Before.ObjectCount, ObjectsAfter: mutation.After.ObjectCount,
		VictoryBefore: mutation.Before.Victory, VictoryAfter: mutation.After.Victory,
		ExperienceAwarded: mutation.ExperienceAwarded, ObjectsCreated: mutation.ObjectsCreated,
		VictoryTriggered: mutation.VictoryTriggered, Policy: policy,
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionSynchronized, summary, data)
	for _, change := range mutation.Changes {
		monsterID := strconv.FormatUint(uint64(change.CombatantID), 10)
		data.Changes = append(data.Changes, activeCombatHPChangeResult{
			MonsterID: monsterID, BeforeHP: change.BeforeHP, AfterHP: change.AfterHP, Defeated: change.Defeated,
		})
		appendSemanticChange(
			&result.Changes, "combat.encounters."+encounterID+".monsters."+monsterID+".currentHp",
			change.BeforeHP, change.AfterHP,
		)
		if change.Defeated {
			appendSemanticChange(&result.Changes, "combat.encounters."+encounterID+".monsters."+monsterID+".active", true, false)
		}
	}
	appendSemanticChange(&result.Changes, "combat.encounters."+encounterID+".activeMonsterCount",
		mutation.Before.ActiveCount, mutation.After.ActiveCount)
	appendSemanticChange(&result.Changes, "combat.encounters."+encounterID+".queueMonsterCount",
		mutation.Before.QueueMonsterCount, mutation.After.QueueMonsterCount)
	appendSemanticChange(&result.Changes, "combat.encounters."+encounterID+".totalMonsterCount",
		len(mutation.Before.Monsters), len(mutation.After.Monsters))
	appendSemanticChange(&result.Changes, "combat.encounters."+encounterID+".victory",
		mutation.Before.Victory, mutation.After.Victory)
	result.Data = mustDebugJSON(data)

	return result
}

type activeCombatSpawnedMonsterResult struct {
	ID           string `json:"id"`
	ManagerIndex int    `json:"managerIndex"`
	TypeID       int    `json:"typeId"`
	Name         string `json:"name"`
	Friend       bool   `json:"friend"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
}

type activeCombatSpawnResult struct {
	EncounterID        string                             `json:"encounterId"`
	Seed               *uint64                            `json:"seed,omitempty"`
	PrimaryCount       int                                `json:"primaryCount"`
	FriendCount        int                                `json:"friendCount"`
	TotalCountBefore   int                                `json:"totalCountBefore"`
	TotalCountAfter    int                                `json:"totalCountAfter"`
	ActiveCountBefore  int                                `json:"activeCountBefore"`
	ActiveCountAfter   int                                `json:"activeCountAfter"`
	AvailablePositions int                                `json:"availablePositions"`
	VictoryBefore      bool                               `json:"victoryBefore"`
	VictoryAfter       bool                               `json:"victoryAfter"`
	Spawned            []activeCombatSpawnedMonsterResult `json:"spawned"`
}

func executeActiveCombatMonsterSpawn(request debugging.Request, state *game_state.GameState) debugging.Result {
	before := newActiveCombatResult(state)
	if before.SnapshotFingerprint != mustDebugStringArgument(request, "expectedSnapshot") {
		return debugging.FailureResult(request, "stale_combat", "combat changed after review")
	}
	spawnRequest, err := activeCombatSpawnRequestFromDebug(request)
	if err != nil {
		return debugging.FailureResult(request, "invalid_combat_spawn", err.Error())
	}
	spawn, err := state.SpawnDebugCombatMonsters(spawnRequest)
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	data := activeCombatSpawnResult{
		EncounterID: strconv.FormatUint(uint64(spawn.EncounterID), 10), Seed: spawn.Seed,
		PrimaryCount: spawn.PrimaryCount, FriendCount: spawn.FriendCount,
		TotalCountBefore: spawn.TotalCountBefore, TotalCountAfter: spawn.TotalCountAfter,
		ActiveCountBefore: spawn.ActiveCountBefore, ActiveCountAfter: spawn.ActiveCountAfter,
		AvailablePositions: spawn.AvailablePositions, VictoryBefore: spawn.VictoryBefore,
		VictoryAfter: spawn.VictoryAfter, Spawned: make([]activeCombatSpawnedMonsterResult, 0, len(spawn.Spawned)),
	}
	for _, monster := range spawn.Spawned {
		data.Spawned = append(data.Spawned, activeCombatSpawnedMonsterResult{
			ID: strconv.FormatUint(uint64(monster.CombatantID), 10), ManagerIndex: monster.ManagerIndex,
			TypeID: monster.MonsterType.Index(), Name: monster.MonsterType.Name(), Friend: monster.Friend,
			X: int(monster.Position.X), Y: int(monster.Position.Y),
		})
	}
	result := debugSuccessResultWithMode(
		request, debugging.ExecutionSynchronized, "Combat reinforcements spawned atomically.", data,
	)
	appendSemanticChange(&result.Changes, "combat.encounters."+data.EncounterID+".totalMonsterCount",
		data.TotalCountBefore, data.TotalCountAfter)
	appendSemanticChange(&result.Changes, "combat.encounters."+data.EncounterID+".activeMonsterCount",
		data.ActiveCountBefore, data.ActiveCountAfter)
	if data.VictoryBefore != data.VictoryAfter {
		appendSemanticChange(&result.Changes, "combat.encounters."+data.EncounterID+".victory",
			data.VictoryBefore, data.VictoryAfter)
	}

	return result
}

func activeCombatSpawnRequestFromDebug(request debugging.Request) (game_state.DebugCombatSpawnRequest, error) {
	input := game_state.DebugCombatSpawnRequest{
		EncounterID: 0, MonsterType: references.MonsterNone, CountMode: game_state.CombatEncounterCountAutomatic,
		ExactCount: 0, FriendsEnabled: true, Seed: nil,
	}
	encounterID, err := strconv.ParseUint(mustDebugStringArgument(request, "encounterId"), 10, 64)
	if err != nil {
		return input, errInvalidCombatEncounter
	}
	input.EncounterID = game_state.CombatEncounterID(encounterID)
	monsterType := mustDebugIntegerArgument(request, "monsterType")
	if !references.MonsterType(monsterType).IsValid() {
		return input, errInvalidCombatMonster
	}
	input.MonsterType = references.MonsterType(monsterType)
	countMode, found := debugCombatCountMode(mustDebugStringArgument(request, "countMode"))
	if !found {
		return input, errInvalidCombatCountMode
	}
	input.CountMode = countMode
	exactCount, exactSet, err := optionalDebugIntegerArgument(request, "exactCount")
	if err != nil {
		return input, err
	}
	if countMode == game_state.CombatEncounterCountExact && !exactSet {
		return input, errMissingExactCount
	}
	if countMode != game_state.CombatEncounterCountExact && exactSet {
		return input, errUnexpectedExactCount
	}
	input.ExactCount = exactCount
	if friends, set, optionalErr := optionalDebugBooleanArgument(request, "friends"); optionalErr != nil {
		return input, optionalErr
	} else if set {
		input.FriendsEnabled = friends
	}
	if seed, set, optionalErr := optionalDebugIntegerArgument(request, "seed"); optionalErr != nil {
		return input, optionalErr
	} else if set {
		value := uint64(seed) //nolint:gosec // Catalog validation restricts seed to a non-negative integer.
		input.Seed = &value
	}

	return input, nil
}

type activeCombatExitDebugResult struct {
	EncounterID        string                    `json:"encounterId"`
	ActiveMonsterCount int                       `json:"activeMonsterCount"`
	TotalMonsterCount  int                       `json:"totalMonsterCount"`
	DefeatedExperience int                       `json:"defeatedExperience"`
	ObjectCount        int                       `json:"objectCount"`
	Victory            bool                      `json:"victory"`
	ExitContext        string                    `json:"exitContext"`
	ExitRoute          string                    `json:"exitRoute"`
	HadOverworldOrigin bool                      `json:"hadOverworldOrigin"`
	ReturnedLocation   references.PlayerLocation `json:"returnedLocation"`
}

func executeActiveCombatExit(request debugging.Request, state *game_state.GameState) debugging.Result {
	before := newActiveCombatResult(state)
	locationBefore := state.MapState.PlayerLocation
	if before.SnapshotFingerprint != mustDebugStringArgument(request, "expectedSnapshot") {
		return debugging.FailureResult(request, "stale_combat", "combat changed after review")
	}
	encounterID, err := strconv.ParseUint(mustDebugStringArgument(request, "encounterId"), 10, 64)
	if err != nil {
		return debugging.FailureResult(request, "invalid_combat_target", "invalid encounter identity")
	}
	exit, err := state.ExitDebugCombat(game_state.CombatEncounterID(encounterID))
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	data := activeCombatExitDebugResult{
		EncounterID: strconv.FormatUint(uint64(exit.EncounterID), 10), ActiveMonsterCount: exit.ActiveMonsterCount,
		TotalMonsterCount: exit.TotalMonsterCount, DefeatedExperience: exit.DefeatedExperience,
		ObjectCount: exit.ObjectCount, Victory: exit.Victory, ExitContext: activeCombatExitContext(exit.ExitContext),
		ExitRoute: activeCombatExitRoute(exit.ExitRoute), HadOverworldOrigin: exit.HadOverworldOrigin,
		ReturnedLocation: exit.ReturnedLocation,
	}
	result := debugSuccessResultWithMode(
		request, debugging.ExecutionSynchronized,
		"Active encounter discarded without resolving a combat outcome.", data,
	)
	appendSemanticChange(&result.Changes, "combat.activeEncounterId", before.EncounterID, nil)
	appendSemanticChange(&result.Changes, "combat.encounters."+before.EncounterID+".active", true, false)
	appendSemanticChange(&result.Changes, "world.playerLocation", locationBefore, exit.ReturnedLocation)

	return result
}

func executeActiveCombatSleepSet(request debugging.Request, state *game_state.GameState) debugging.Result {
	before := newActiveCombatResult(state)
	if before.SnapshotFingerprint != mustDebugStringArgument(request, "expectedSnapshot") {
		return debugging.FailureResult(request, "stale_combat", "combat changed after review")
	}
	encounterID, err := strconv.ParseUint(mustDebugStringArgument(request, "encounterId"), 10, 64)
	if err != nil {
		return debugging.FailureResult(request, "invalid_combat_target", "invalid encounter identity")
	}
	monsterID, err := strconv.ParseUint(mustDebugStringArgument(request, "monsterId"), 10, 64)
	if err != nil {
		return debugging.FailureResult(request, "invalid_combat_target", "invalid monster identity")
	}
	change, err := state.SetDebugCombatMonsterAsleep(
		game_state.CombatEncounterID(encounterID),
		game_state.CombatantID(monsterID),
		mustActiveCombatBooleanArgument(request, "expectedAsleep"),
		mustActiveCombatBooleanArgument(request, "asleep"),
	)
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	after := newActiveCombatResult(state)
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		"Monster sleep condition updated.",
		after,
	)
	appendSemanticChange(
		&result.Changes,
		fmt.Sprintf("combat.encounters.%d.monsters.%d.asleep", encounterID, change.CombatantID),
		change.Before,
		change.After,
	)

	return result
}

func executeActiveCombatSleepAll(request debugging.Request, state *game_state.GameState) debugging.Result {
	_, encounterID, failure := reviewedActiveCombatRequest(request, state)
	if failure != nil {
		return *failure
	}
	expected, err := state.DebugActiveCombatSnapshot(encounterID)
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	changes, err := state.SleepAllDebugCombatMonsters(game_state.DebugCombatBatchRequest{
		EncounterID: encounterID,
		Expected:    expected,
	})
	if err != nil {
		return activeCombatMutationFailure(request, err)
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionSynchronized,
		"All live monsters put to sleep.",
		newActiveCombatResult(state),
	)
	for _, change := range changes {
		appendSemanticChange(
			&result.Changes,
			fmt.Sprintf("combat.encounters.%d.monsters.%d.asleep", encounterID, change.CombatantID),
			change.Before,
			change.After,
		)
	}

	return result
}

func activeCombatMutationFailure(request debugging.Request, err error) debugging.Result {
	switch {
	case errors.Is(err, game_state.ErrDebugCombatUnavailable):
		return debugging.FailureResult(request, "combat_unavailable", err.Error())
	case errors.Is(err, game_state.ErrDebugCombatBusy):
		return debugging.FailureResult(request, "combat_busy", err.Error())
	case errors.Is(err, game_state.ErrDebugCombatStale):
		return debugging.FailureResult(request, "stale_combat", err.Error())
	case errors.Is(err, game_state.ErrDebugCombatTarget):
		return debugging.FailureResult(request, "invalid_combat_target", err.Error())
	case errors.Is(err, game_state.ErrDebugCombatSpawn):
		return debugging.FailureResult(request, "combat_spawn_unavailable", err.Error())
	case errors.Is(err, game_state.ErrDebugDungeonRoomRequired):
		return debugging.FailureResult(request, "dungeon_room_combat_required", err.Error())
	default:
		panic("unclassified active combat mutation error: " + err.Error())
	}
}

func newActiveCombatResult(state *game_state.GameState) activeCombatResult {
	monsterTypes := make([]activeCombatMonsterTypeResult, 0, references.TotalMonsterTypes)
	for monsterType := references.MonsterType(0); monsterType < references.TotalMonsterTypes; monsterType++ {
		monsterTypes = append(monsterTypes, activeCombatMonsterTypeResult{ID: monsterType.Index(), Name: monsterType.Name()})
	}
	result := activeCombatResult{
		Active: false, UnavailableReason: "No active tactical encounter.", SnapshotFingerprint: "",
		EncounterID: "", Map: nil, Party: []activeCombatPartyResult{}, Monsters: []activeCombatMonsterResult{},
		Forecast: []activeCombatForecastResult{}, ActiveMonsterCount: 0, TotalMonsterCount: 0,
		ActiveHostileCount: 0, FixedDungeonRoom: false, DungeonIndex: 0, DungeonRoom: 0,
		DefeatedExperience: 0, RNGSeed: state.RandomSeed(), WorkflowActive: state.EventQueueManager.HasActiveWorkflows(),
		PresentationSpeedPercent: 0, MonsterTypes: monsterTypes,
	}
	if state.CombatScene == nil || !state.IsInCombat() {
		result.SnapshotFingerprint = activeCombatFingerprint(result)

		return result
	}

	combat := state.CombatScene
	result.Active = true
	result.UnavailableReason = ""
	result.EncounterID = strconv.FormatUint(uint64(combat.EncounterID), 10)
	result.Map = newActiveCombatMapResult(state)
	result.Party = newActiveCombatPartyResults(state)
	result.Monsters = newActiveCombatMonsterResults(combat)
	result.Forecast = newActiveCombatForecastResults(state)
	result.ActiveMonsterCount = combat.MonsterManager.CountActive()
	result.ActiveHostileCount = combat.MonsterManager.CountActiveHostile()
	result.TotalMonsterCount = combat.MonsterManager.Count()
	result.DefeatedExperience = combat.MonsterManager.GetTotalExperience()
	if combat.DungeonRoomOrigin != nil {
		result.FixedDungeonRoom = true
		result.DungeonIndex = int(combat.DungeonRoomOrigin.DungeonIndex)
		result.DungeonRoom = combat.DungeonRoomOrigin.Room
	}
	result.RNGSeed = state.RandomSeed()
	result.SnapshotFingerprint = activeCombatFingerprint(result)

	return result
}

func newActiveCombatMapResult(state *game_state.GameState) *activeCombatMapResult {
	combat := state.CombatScene
	result := &activeCombatMapResult{
		Name: combat.CombatMapRef.MapName, Index: combat.CombatMapRef.MapIndex,
		Territory:      activeCombatTerritory(combat.CombatMapRef.Territory),
		EntryDirection: activeCombatEntryDirection(combat.EntryDirection),
		Victory:        combat.Victory, TurnStarted: combat.CombatTurnsStarted(),
		PendingAdvanceTurn: combat.HasScheduledCombatStep(),
		SoloMoveMode:       combat.SoloMoveMode, CurrentPartyIndex: combat.CurrentTurnIndex,
		ActiveMonsterIndex: combat.ActiveMonsterIndex, TargetPartyIndex: combat.ActiveTargetPCIndex,
		ObjectCount: combat.ObjectCount(),
		Cells:       make([]activeCombatCellResult, 0, references.XCombatMapTiles*references.YCombatMapTiles),
	}
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			position := references.Position{X: x, Y: y}
			cell := activeCombatCellResult{
				X: int(x), Y: int(y), TileID: int(combat.CombatMapRef.GetTileNumber(position)),
				OccupantKind: "", OccupantID: "", OccupantName: "",
			}
			appendActiveCombatOccupant(&cell, state, position)
			result.Cells = append(result.Cells, cell)
		}
	}

	return result
}

func appendActiveCombatOccupant(
	cell *activeCombatCellResult,
	state *game_state.GameState,
	position references.Position,
) {
	combat := state.CombatScene
	for index := range party_state.MaxActiveParty {
		if combat.PartyActive[index] && combat.PartyPositions[index] == position {
			cell.OccupantKind = "party"
			cell.OccupantID = strconv.Itoa(index)
			cell.OccupantName = state.PartyState.GetCharacter(index).GetNameAsString()

			return
		}
	}
	if monster := combat.MonsterManager.GetMonsterAtPosition(position); monster != nil {
		cell.OccupantKind = activeCombatMonsterKind
		cell.OccupantID = strconv.FormatUint(uint64(monster.CombatantID()), 10)
		cell.OccupantName = monster.MonsterType().Name()
	}
}

func newActiveCombatPartyResults(state *game_state.GameState) []activeCombatPartyResult {
	combat := state.CombatScene
	results := make([]activeCombatPartyResult, 0, party_state.MaxActiveParty)
	for index := range party_state.MaxActiveParty {
		queue, found := combat.TurnQueue.GetEntrySnapshot(game_state.CombatTurnIdentity{
			IsPartyMember: true,
			EntityIndex:   index,
		})
		if !found {
			continue
		}
		character := state.PartyState.GetCharacter(index)
		position := combat.PartyPositions[index]
		results = append(results, activeCombatPartyResult{
			Index: index, Name: character.GetNameAsString(), X: int(position.X), Y: int(position.Y),
			Active: combat.PartyActive[index], CurrentActor: combat.ActiveMonsterIndex == game_state.NoActiveMonster &&
				combat.CurrentTurnIndex == index, CurrentTarget: combat.ActiveTargetPCIndex == index,
			CurrentHP: int(character.GetCurrentHp()), MaximumHP: int(character.GetMaxHp()),
			CurrentMP: int(character.GetCurrentMp()), MaximumMP: int(character.GetMaxMp()),
			Dexterity:      int(character.Dexterity),
			Status:         activeCombatPartyStatus(character.GetStatus()),
			QueueCountdown: queue.Countdown, QueueStatus: activeCombatQueueStatus(queue.Status),
			CanAct: queue.CanTakeTurn || queue.ActionUncertain,
		})
	}

	return results
}

func activeCombatPartyStatus(status party_state.CharacterStatus) string {
	name, ok := status.FriendlyName()
	if !ok {
		return fmt.Sprintf("status_%d", status)
	}

	return name
}

func newActiveCombatMonsterResults(combat *game_state.CombatState) []activeCombatMonsterResult {
	results := make([]activeCombatMonsterResult, 0, combat.MonsterManager.Count())
	combat.MonsterManager.ForEach(func(index int, monster *game_state.CombatMonster) {
		queue, _ := combat.TurnQueue.GetEntrySnapshot(game_state.CombatTurnIdentity{
			IsPartyMember: false,
			EntityIndex:   index,
		})
		position := monster.Position()
		results = append(results, activeCombatMonsterResult{
			ID: strconv.FormatUint(uint64(monster.CombatantID()), 10), ManagerIndex: index,
			TypeID: int(monster.MonsterType()), Name: monster.MonsterType().Name(), X: int(position.X), Y: int(position.Y),
			CurrentHP: monster.CurrentHP(), MaximumHP: monster.MaxHP(), CurrentDex: monster.CurrentDex(),
			BaseDex: monster.BaseDex(), Active: monster.IsActive(), Dead: monster.IsDead(),
			Invisible: monster.IsInvisible(), Asleep: monster.IsAsleep(), Poisoned: monster.IsPoisoned(),
			CurrentActor: combat.ActiveMonsterIndex == index, QueueCountdown: queue.Countdown,
			QueueStatus: activeCombatQueueStatus(queue.Status), CanAct: queue.CanTakeTurn || queue.ActionUncertain,
			Abilities: activeCombatAbilityNames(monster),
			Hostile:   monster.IsHostile(),
		})
	})

	return results
}

func newActiveCombatForecastResults(state *game_state.GameState) []activeCombatForecastResult {
	forecast := make([]game_state.CombatTurnForecastEntry, activeCombatForecastSize)
	count := state.CombatScene.TurnQueue.FillForecast(forecast)
	results := make([]activeCombatForecastResult, 0, count)
	for order, entry := range forecast[:count] {
		kind, id, name := activeCombatForecastIdentity(state, entry)
		results = append(results, activeCombatForecastResult{
			Order: order + 1, Kind: kind, ID: id, Name: name, Dexterity: entry.Dexterity,
			ResetCountdown: entry.Countdown, CanAct: entry.CanTakeTurn || entry.ActionUncertain,
			Status: activeCombatQueueStatus(entry.Status),
		})
	}

	return results
}

func activeCombatForecastIdentity(
	state *game_state.GameState,
	entry game_state.CombatTurnForecastEntry,
) (string, string, string) {
	if entry.IsPartyMember {
		return "party", strconv.Itoa(entry.EntityIndex), state.PartyState.GetCharacter(entry.EntityIndex).GetNameAsString()
	}
	monster := state.CombatScene.MonsterManager.GetMonster(entry.EntityIndex)
	if monster == nil {
		return activeCombatMonsterKind, "", "Missing monster"
	}

	return activeCombatMonsterKind, strconv.FormatUint(uint64(monster.CombatantID()), 10), monster.MonsterType().Name()
}

func activeCombatAbilityNames(monster *game_state.CombatMonster) []string {
	names := make([]string, 0, len(monster.EnemyRef().EnemyAbilities))
	for ability := range monster.EnemyRef().EnemyAbilities {
		names = append(names, activeCombatAbilityName(ability))
	}
	sort.Strings(names)

	return names
}

func activeCombatAbilityName(ability references.EnemyAbility) string {
	switch ability {
	case references.Bludgeons:
		return "Bludgeons"
	case references.PossessCharm:
		return "Possess/Charm"
	case references.Undead:
		return "Undead"
	case references.DivideOnHit:
		return "Divide"
	case references.Immortal:
		return "Immortal"
	case references.PoisonAtRange:
		return "Ranged Poison"
	case references.StealsFood:
		return "Steals Food"
	case references.NoCorpse:
		return "No Corpse"
	case references.RangedMagic:
		return "Ranged Magic"
	case references.Teleport:
		return "Teleport"
	case references.DisappearsOnDeath:
		return "Disappears"
	case references.Invisibility:
		return "Invisibility"
	case references.GatesInDaemon:
		return "Gates Daemon"
	case references.Poison:
		return "Poison"
	case references.InfectWithPlague:
		return "Plague"
	default:
		return fmt.Sprintf("Ability %d", ability)
	}
}

func activeCombatQueueStatus(status game_state.CombatTurnStatus) string {
	switch status {
	case game_state.CombatTurnActive:
		return "active"
	case game_state.CombatTurnDead:
		return activeCombatDeadStatus
	case game_state.CombatTurnFled:
		return "fled"
	case game_state.CombatTurnAbsorbed:
		return "absorbed"
	default:
		panic("unknown combat turn status")
	}
}

func activeCombatTerritory(territory references.CombatMapTerritory) string {
	if territory == references.BritanniaCombatTerritory {
		return activeCombatBritannia
	}

	return "dungeon"
}

func activeCombatEntryDirection(direction game_state.CombatEntryDirection) string {
	switch direction {
	case game_state.CombatEntryWest:
		return "west"
	case game_state.CombatEntryEast:
		return "east"
	case game_state.CombatEntryNorth:
		return "north"
	case game_state.CombatEntrySouth:
		return "south"
	default:
		panic("unknown combat entry direction")
	}
}

func activeCombatExitContext(context game_state.CombatExitContext) string {
	switch context {
	case game_state.CombatExitSurface:
		return "surface"
	case game_state.CombatExitDungeonEncounter:
		return "dungeon_encounter"
	case game_state.CombatExitDungeonRoom:
		return "dungeon_room"
	default:
		panic("unknown combat exit context")
	}
}

func activeCombatExitRoute(route game_state.CombatExitRoute) string {
	switch route {
	case game_state.CombatExitNone:
		return debugVehicleNone
	case game_state.CombatExitNorth:
		return "north"
	case game_state.CombatExitEast:
		return "east"
	case game_state.CombatExitSouth:
		return "south"
	case game_state.CombatExitWest:
		return "west"
	case game_state.CombatExitUp:
		return "up"
	case game_state.CombatExitDown:
		return "down"
	case game_state.CombatExitDoomMirror:
		return "doom_mirror"
	default:
		panic("unknown combat exit route")
	}
}

func activeCombatFingerprint(result activeCombatResult) string {
	result.SnapshotFingerprint = ""

	return string(mustDebugJSON(result))
}

func mustActiveCombatBooleanArgument(request debugging.Request, argumentID string) bool {
	value, set, err := optionalDebugBooleanArgument(request, argumentID)
	if err != nil || !set {
		panic("catalog-normalized active combat request omitted boolean argument " + argumentID)
	}

	return value
}
