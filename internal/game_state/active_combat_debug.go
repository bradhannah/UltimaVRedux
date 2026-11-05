package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

var (
	// ErrDebugCombatUnavailable means combat state and map state do not describe one active encounter.
	ErrDebugCombatUnavailable = errors.New("active combat is unavailable")
	// ErrDebugCombatStale means a reviewed encounter or monster no longer matches live state.
	ErrDebugCombatStale = errors.New("active combat state changed after review")
	// ErrDebugCombatBusy means a gameplay workflow or intermediate turn boundary owns combat state.
	ErrDebugCombatBusy = errors.New("active combat is not at a quiescent player-turn boundary")
	// ErrDebugCombatTarget means the requested monster is not a live active combatant.
	ErrDebugCombatTarget = errors.New("combat monster is unavailable")
	// ErrDebugCombatSpawn means a reinforcement request is invalid or has no capacity.
	ErrDebugCombatSpawn = errors.New("combat reinforcement spawn is unavailable")
	// ErrDebugDungeonRoomRequired means the operation requires authoritative fixed-room provenance.
	ErrDebugDungeonRoomRequired = errors.New("fixed dungeon-room combat is required")
)

const (
	// DebugCombatDamageMinimum bounds raw debug damage independently of adapters.
	DebugCombatDamageMinimum = 1
	// DebugCombatDamageMaximum bounds raw debug damage independently of adapters.
	DebugCombatDamageMaximum = 999
)

// DebugCombatSleepChange describes one explicit monster condition mutation.
type DebugCombatSleepChange struct {
	CombatantID CombatantID
	Before      bool
	After       bool
}

// DebugCombatMonsterSnapshot is the reviewable state of one encounter-scoped monster.
type DebugCombatMonsterSnapshot struct {
	CombatantID  CombatantID
	ManagerIndex int
	CurrentHP    int
	MaxHP        int
	Active       bool
	QueueStatus  CombatTurnStatus
	Hostile      bool
}

// DebugCombatSnapshot is a stable, ordered review precondition for destructive mutations.
type DebugCombatSnapshot struct {
	EncounterID       CombatEncounterID
	RNGSeed           uint64
	Monsters          []DebugCombatMonsterSnapshot
	ActiveCount       int
	QueueMonsterCount int
	ObjectCount       int
	Victory           bool
	FixedDungeonRoom  bool
	DungeonIndex      references.DungeonCombatIndex
	DungeonRoom       int
}

// DebugCombatDamageRequest applies reviewed damage to one stable combatant.
type DebugCombatDamageRequest struct {
	EncounterID CombatEncounterID
	CombatantID CombatantID
	ExpectedHP  int
	Damage      int
}

// DebugCombatKillRequest defeats one reviewed stable combatant.
type DebugCombatKillRequest struct {
	EncounterID CombatEncounterID
	CombatantID CombatantID
	ExpectedHP  int
}

// DebugCombatBatchRequest supplies the exact ordered monster snapshot reviewed by an adapter.
type DebugCombatBatchRequest struct {
	EncounterID CombatEncounterID
	Expected    DebugCombatSnapshot
}

// DebugCombatHPChange reports one committed monster HP transition.
type DebugCombatHPChange struct {
	CombatantID CombatantID
	BeforeHP    int
	AfterHP     int
	Defeated    bool
}

// DebugCombatMutationResult reports authoritative state before and after a mutation.
// Debug defeats intentionally award no XP and create no corpse/treasure objects;
// they do synchronize the DEX queue and trigger normal victory when none remain.
type DebugCombatMutationResult struct {
	EncounterID       CombatEncounterID
	RNGSeed           uint64
	Before            DebugCombatSnapshot
	After             DebugCombatSnapshot
	Changes           []DebugCombatHPChange
	ExperienceAwarded int
	ObjectsCreated    int
	VictoryTriggered  bool
}

// DebugCombatExitResult reports encounter-local state discarded by a debug exit.
type DebugCombatExitResult struct {
	EncounterID        CombatEncounterID
	ActiveMonsterCount int
	TotalMonsterCount  int
	DefeatedExperience int
	ObjectCount        int
	Victory            bool
	ExitContext        CombatExitContext
	ExitRoute          CombatExitRoute
	HadOverworldOrigin bool
	ReturnedLocation   references.PlayerLocation
}

// DebugCombatSpawnRequest describes one reviewed reinforcement batch.
type DebugCombatSpawnRequest struct {
	EncounterID    CombatEncounterID
	MonsterType    references.MonsterType
	CountMode      CombatEncounterCountMode
	ExactCount     int
	FriendsEnabled bool
	Seed           *uint64
}

// DebugCombatSpawnedMonster reports one committed reinforcement.
type DebugCombatSpawnedMonster struct {
	CombatantID  CombatantID
	ManagerIndex int
	MonsterType  references.MonsterType
	Friend       bool
	Position     references.Position
}

// DebugCombatSpawnResult reports one atomic reinforcement batch.
type DebugCombatSpawnResult struct {
	EncounterID        CombatEncounterID
	Seed               *uint64
	PrimaryCount       int
	FriendCount        int
	TotalCountBefore   int
	TotalCountAfter    int
	ActiveCountBefore  int
	ActiveCountAfter   int
	AvailablePositions int
	VictoryBefore      bool
	VictoryAfter       bool
	Spawned            []DebugCombatSpawnedMonster
}

// SetDebugCombatMonsterAsleep changes one live monster at a safe combat boundary.
func (g *GameState) SetDebugCombatMonsterAsleep(
	encounterID CombatEncounterID,
	combatantID CombatantID,
	expectedAsleep bool,
	asleep bool,
) (DebugCombatSleepChange, error) {
	if err := g.validateDebugCombatBoundary(encounterID); err != nil {
		return DebugCombatSleepChange{}, err
	}
	combat := g.CombatScene
	monster := combat.MonsterManager.GetMonsterByID(combatantID)
	if monster == nil || !monster.IsActive() || monster.IsDead() {
		return DebugCombatSleepChange{}, ErrDebugCombatTarget
	}
	before := monster.IsAsleep()
	if before != expectedAsleep {
		return DebugCombatSleepChange{}, ErrDebugCombatStale
	}
	monster.SetAsleep(asleep)

	return DebugCombatSleepChange{CombatantID: combatantID, Before: before, After: asleep}, nil
}

// SleepAllDebugCombatMonsters puts every live monster to sleep after exact snapshot review. Monsters
// that already sleep are left alone and omitted from the returned changes.
func (g *GameState) SleepAllDebugCombatMonsters(request DebugCombatBatchRequest) ([]DebugCombatSleepChange, error) {
	reviewed, err := g.reviewDebugCombatBatch(request)
	if err != nil {
		return nil, err
	}
	if len(reviewed) == 0 {
		return nil, ErrDebugCombatTarget
	}
	changes := make([]DebugCombatSleepChange, 0, len(reviewed))
	for _, target := range reviewed {
		if target.monster.IsAsleep() {
			continue
		}
		target.monster.SetAsleep(true)
		changes = append(changes, DebugCombatSleepChange{
			CombatantID: target.monster.CombatantID(), Before: false, After: true,
		})
	}

	return changes, nil
}

// DebugActiveCombatSnapshot returns a quiescent snapshot suitable for reviewed mutation preconditions.
func (g *GameState) DebugActiveCombatSnapshot(encounterID CombatEncounterID) (DebugCombatSnapshot, error) {
	if err := g.validateDebugCombatBoundary(encounterID); err != nil {
		return DebugCombatSnapshot{}, err
	}

	return g.debugCombatSnapshot(), nil
}

// DamageDebugCombatMonster damages one reviewed monster and resolves a debug defeat when lethal.
func (g *GameState) DamageDebugCombatMonster(request DebugCombatDamageRequest) (*DebugCombatMutationResult, error) {
	if request.Damage < DebugCombatDamageMinimum || request.Damage > DebugCombatDamageMaximum {
		return nil, fmt.Errorf(
			"%w: damage must be between %d and %d",
			ErrDebugCombatTarget,
			DebugCombatDamageMinimum,
			DebugCombatDamageMaximum,
		)
	}
	monster, monsterIndex, err := g.reviewDebugCombatMonster(request.EncounterID, request.CombatantID, request.ExpectedHP)
	if err != nil {
		return nil, err
	}

	reviewed := []reviewedDebugCombatMonster{{monster: monster, index: monsterIndex}}

	return g.commitDebugCombatDefeats(request.EncounterID, reviewed, request.Damage), nil
}

// KillDebugCombatMonster defeats one reviewed monster under the documented debug policy.
func (g *GameState) KillDebugCombatMonster(request DebugCombatKillRequest) (*DebugCombatMutationResult, error) {
	monster, monsterIndex, err := g.reviewDebugCombatMonster(request.EncounterID, request.CombatantID, request.ExpectedHP)
	if err != nil {
		return nil, err
	}

	reviewed := []reviewedDebugCombatMonster{{monster: monster, index: monsterIndex}}

	return g.commitDebugCombatDefeats(request.EncounterID, reviewed, monster.CurrentHP()), nil
}

// KillAllDebugCombatMonsters atomically defeats every live monster after exact snapshot review.
func (g *GameState) KillAllDebugCombatMonsters(request DebugCombatBatchRequest) (*DebugCombatMutationResult, error) {
	reviewed, err := g.reviewDebugCombatBatch(request)
	if err != nil {
		return nil, err
	}

	return g.commitDebugCombatDefeats(request.EncounterID, reviewed, 0), nil
}

// ClearDebugDungeonRoomEnemies defeats reviewed hostile enemies and declares room victory.
func (g *GameState) ClearDebugDungeonRoomEnemies(request DebugCombatBatchRequest) (*DebugCombatMutationResult, error) {
	reviewed, err := g.reviewDebugCombatBatch(request)
	if err != nil {
		return nil, err
	}
	combat := g.CombatScene
	if combat.ExitContext != CombatExitDungeonRoom || combat.DungeonRoomOrigin == nil {
		return nil, ErrDebugDungeonRoomRequired
	}
	if combat.Victory {
		return nil, ErrDebugCombatTarget
	}
	hostile := reviewed[:0]
	for _, target := range reviewed {
		if target.monster.IsHostile() {
			hostile = append(hostile, target)
		}
	}
	if len(hostile) == 0 {
		return nil, ErrDebugCombatTarget
	}
	combat.DebugOutcomeSuppressed = false
	result := g.commitDebugCombatDefeats(request.EncounterID, hostile, 0)
	if !result.VictoryTriggered || !combat.Victory {
		panic("clearing fixed dungeon-room enemies did not trigger victory")
	}

	return result, nil
}

// ClearDebugCombatMonsters atomically removes all monsters without XP, objects, or victory.
func (g *GameState) ClearDebugCombatMonsters(request DebugCombatBatchRequest) (*DebugCombatMutationResult, error) {
	if _, err := g.reviewDebugCombatBatch(request); err != nil {
		return nil, err
	}
	before := g.debugCombatSnapshot()
	combat := g.CombatScene
	combat.MonsterManager.Clear()
	combat.TurnQueue.RemoveAllMonsters()
	combat.ActiveMonsterIndex = NoActiveMonster
	combat.ActiveTargetPCIndex = NoTargetPC
	combat.AttackTargeting = nil
	combat.SpellTargeting = nil
	combat.ConeTargeting = nil
	combat.AdjacentSpellTargeting = nil
	combat.pendingSpell = nil
	combat.Victory = false
	combat.DebugOutcomeSuppressed = true
	for index := range combat.LastAttackTargets {
		combat.LastAttackTargets[index] = NoCombatTarget
	}
	for index := range combat.partyClones {
		combat.partyClones[index].lastAttackTarget = NoCombatTarget
	}

	return &DebugCombatMutationResult{
		EncounterID: request.EncounterID, RNGSeed: g.RandomSeed(), Before: before, After: g.debugCombatSnapshot(),
		Changes: make([]DebugCombatHPChange, 0), ExperienceAwarded: 0, ObjectsCreated: 0, VictoryTriggered: false,
	}, nil
}

type reviewedDebugCombatMonster struct {
	monster *CombatMonster
	index   int
}

func (g *GameState) reviewDebugCombatMonster(
	encounterID CombatEncounterID,
	combatantID CombatantID,
	expectedHP int,
) (*CombatMonster, int, error) {
	if err := g.validateDebugCombatBoundary(encounterID); err != nil {
		return nil, 0, err
	}
	monster := g.CombatScene.MonsterManager.GetMonsterByID(combatantID)
	if monster == nil || !monster.IsActive() || monster.IsDead() {
		return nil, 0, ErrDebugCombatTarget
	}
	if monster.CurrentHP() != expectedHP {
		return nil, 0, ErrDebugCombatStale
	}
	monsterIndex := -1
	g.CombatScene.MonsterManager.ForEach(func(index int, candidate *CombatMonster) {
		if candidate == monster {
			monsterIndex = index
		}
	})
	if monsterIndex < 0 {
		panic("reviewed combat monster has no manager index")
	}

	return monster, monsterIndex, nil
}

func (g *GameState) reviewDebugCombatBatch(request DebugCombatBatchRequest) ([]reviewedDebugCombatMonster, error) {
	if err := g.validateDebugCombatBoundary(request.EncounterID); err != nil {
		return nil, err
	}
	combat := g.CombatScene
	live := g.debugCombatSnapshot()
	if request.Expected.EncounterID != request.EncounterID ||
		request.Expected.ActiveCount != live.ActiveCount ||
		request.Expected.QueueMonsterCount != live.QueueMonsterCount ||
		request.Expected.ObjectCount != live.ObjectCount ||
		request.Expected.Victory != live.Victory ||
		request.Expected.FixedDungeonRoom != live.FixedDungeonRoom ||
		request.Expected.DungeonIndex != live.DungeonIndex ||
		request.Expected.DungeonRoom != live.DungeonRoom ||
		len(request.Expected.Monsters) != len(live.Monsters) {
		return nil, ErrDebugCombatStale
	}
	reviewed := make([]reviewedDebugCombatMonster, 0, len(request.Expected.Monsters))
	stale := false
	combat.MonsterManager.ForEach(func(index int, monster *CombatMonster) {
		expected := request.Expected.Monsters[index]
		if expected != live.Monsters[index] {
			stale = true

			return
		}
		if monster.IsActive() && !monster.IsDead() {
			reviewed = append(reviewed, reviewedDebugCombatMonster{monster: monster, index: index})
		}
	})
	if stale {
		return nil, ErrDebugCombatStale
	}

	return reviewed, nil
}

func (g *GameState) commitDebugCombatDefeats(
	encounterID CombatEncounterID,
	reviewed []reviewedDebugCombatMonster,
	damage int,
) *DebugCombatMutationResult {
	before := g.debugCombatSnapshot()
	result := &DebugCombatMutationResult{
		EncounterID: encounterID, RNGSeed: g.RandomSeed(), Before: before,
		Changes: make([]DebugCombatHPChange, 0, len(reviewed)), ExperienceAwarded: 0, ObjectsCreated: 0,
		After: DebugCombatSnapshot{
			EncounterID: 0, RNGSeed: 0, Monsters: nil, ActiveCount: 0, QueueMonsterCount: 0,
			ObjectCount: 0, Victory: false,
		}, VictoryTriggered: false,
	}
	for _, target := range reviewed {
		hpBefore := target.monster.CurrentHP()
		appliedDamage := damage
		if appliedDamage == 0 {
			appliedDamage = hpBefore
		}
		target.monster.TakeDamage(appliedDamage)
		defeated := target.monster.IsDead()
		if defeated {
			g.finalizeCombatMonsterDeathWithPolicy(target.monster, target.index, nil, -1, false, false)
		}
		result.Changes = append(result.Changes, DebugCombatHPChange{
			CombatantID: target.monster.CombatantID(), BeforeHP: hpBefore,
			AfterHP: target.monster.CurrentHP(), Defeated: defeated,
		})
	}
	result.VictoryTriggered = g.checkCombatEndConditions() == CombatVictory
	result.After = g.debugCombatSnapshot()
	result.ObjectsCreated = result.After.ObjectCount - result.Before.ObjectCount

	return result
}

func (g *GameState) debugCombatSnapshot() DebugCombatSnapshot {
	combat := g.CombatScene
	snapshot := DebugCombatSnapshot{
		EncounterID: combat.EncounterID, RNGSeed: g.RandomSeed(), ActiveCount: combat.MonsterManager.CountActive(),
		QueueMonsterCount: 0, ObjectCount: combat.ObjectCount(), Victory: combat.Victory,
		Monsters: make([]DebugCombatMonsterSnapshot, 0, combat.MonsterManager.Count()),
	}
	if combat.DungeonRoomOrigin != nil {
		snapshot.FixedDungeonRoom = true
		snapshot.DungeonIndex = combat.DungeonRoomOrigin.DungeonIndex
		snapshot.DungeonRoom = combat.DungeonRoomOrigin.Room
	}
	combat.MonsterManager.ForEach(func(index int, monster *CombatMonster) {
		queue, found := combat.TurnQueue.GetEntrySnapshot(CombatTurnIdentity{IsPartyMember: false, EntityIndex: index})
		if !found {
			panic("active combat monster manager and turn queue are out of sync")
		}
		snapshot.Monsters = append(snapshot.Monsters, DebugCombatMonsterSnapshot{
			CombatantID: monster.CombatantID(), ManagerIndex: index, CurrentHP: monster.CurrentHP(),
			MaxHP: monster.MaxHP(), Active: monster.IsActive() && !monster.IsDead(), QueueStatus: queue.Status,
			Hostile: monster.IsHostile(),
		})
	})
	_, snapshot.QueueMonsterCount = combat.TurnQueue.CountRemaining()

	return snapshot
}

// ExitDebugCombat discards tactical state without resolving a normal combat outcome.
func (g *GameState) ExitDebugCombat(encounterID CombatEncounterID) (DebugCombatExitResult, error) {
	if err := g.validateDebugCombatBoundary(encounterID); err != nil {
		return DebugCombatExitResult{}, err
	}
	combat := g.CombatScene
	if combat.DungeonRoomOrigin != nil {
		return DebugCombatExitResult{}, fmt.Errorf(
			"%w: fixed dungeon rooms must resolve through their normal exit",
			ErrDebugCombatBusy,
		)
	}
	if combat.DungeonWandererOrigin != nil {
		return DebugCombatExitResult{}, fmt.Errorf(
			"%w: dungeon wanderer encounters must resolve through their normal exit",
			ErrDebugCombatBusy,
		)
	}
	if combat.CampingOrigin != nil {
		return DebugCombatExitResult{}, fmt.Errorf(
			"%w: camping encounters must resolve through their normal exit",
			ErrDebugCombatBusy,
		)
	}
	if combat.DoomEntranceOrigin != nil {
		return DebugCombatExitResult{}, fmt.Errorf(
			"%w: Doom entrance encounters must resolve through their normal exit",
			ErrDebugCombatBusy,
		)
	}
	result := DebugCombatExitResult{
		EncounterID: encounterID, ActiveMonsterCount: combat.MonsterManager.CountActive(),
		TotalMonsterCount: combat.MonsterManager.Count(), DefeatedExperience: combat.MonsterManager.GetTotalExperience(),
		ObjectCount: combat.ObjectCount(), Victory: combat.Victory, ExitContext: combat.ExitContext,
		ExitRoute: combat.ExitRoute, HadOverworldOrigin: combat.OverworldOrigin != nil,
		ReturnedLocation: references.PlayerLocation{
			Location: 0, Position: references.Position{X: 0, Y: 0}, Floor: 0,
		},
	}
	g.TransitionFromCombatMap()
	result.ReturnedLocation = g.MapState.PlayerLocation

	return result, nil
}

// SpawnDebugCombatMonsters validates and commits one atomic reinforcement batch.
func (g *GameState) SpawnDebugCombatMonsters(request DebugCombatSpawnRequest) (*DebugCombatSpawnResult, error) {
	if err := g.validateDebugCombatBoundary(request.EncounterID); err != nil {
		return nil, err
	}
	if request.CountMode < CombatEncounterCountAutomatic || request.CountMode > CombatEncounterCountMaximum {
		return nil, fmt.Errorf("%w: invalid count mode", ErrDebugCombatSpawn)
	}
	primaryRef, err := g.enemyReferenceForCombat(request.MonsterType)
	if err != nil || primaryRef.MaxPerMap < 1 {
		return nil, fmt.Errorf("%w: invalid monster type", ErrDebugCombatSpawn)
	}
	friendType := request.MonsterType
	if request.FriendsEnabled && primaryRef.Friend != nil {
		friendType = primaryRef.Friend.MonsterType
		if _, friendErr := g.enemyReferenceForCombat(friendType); friendErr != nil {
			return nil, fmt.Errorf("%w: invalid friend reference", ErrDebugCombatSpawn)
		}
	}
	combat := g.CombatScene
	assertDebugCombatMonsterPopulation(combat)
	positions, err := availableActiveCombatSpawnPositions(combat)
	if err != nil {
		return nil, err
	}
	capacity := min(references.MaxMonstersInCombat, debugCombatReinforcementCapacity(combat), len(positions))
	if capacity < 1 {
		return nil, fmt.Errorf("%w: encounter has no remaining capacity", ErrDebugCombatSpawn)
	}
	if request.CountMode == CombatEncounterCountExact && (request.ExactCount < 1 || request.ExactCount > capacity) {
		return nil, fmt.Errorf("%w: exact count %d is outside 1-%d", ErrDebugCombatSpawn, request.ExactCount, capacity)
	}

	seed := cloneEncounterSeed(request.Seed)
	if seed != nil {
		g.SetRandomSeed(*seed)
	}
	countRequest := CombatEncounterRequest{
		Map: CombatEncounterMap{
			Territory: references.BritanniaCombatTerritory, BritanniaMap: 0, Dungeon: 0, DungeonRoom: 0,
		}, EntryDirection: CombatEntryWest, MonsterType: request.MonsterType,
		CountMode: request.CountMode, ExactCount: request.ExactCount, FriendsEnabled: false, Seed: nil,
	}
	count := g.resolveCombatEncounterCount(countRequest, primaryRef, capacity)
	typeRequest := CombatEncounterRequest{
		Map: CombatEncounterMap{
			Territory: references.BritanniaCombatTerritory, BritanniaMap: 0, Dungeon: 0, DungeonRoom: 0,
		}, EntryDirection: CombatEntryWest, MonsterType: request.MonsterType,
		CountMode: CombatEncounterCountAutomatic, ExactCount: 0, FriendsEnabled: request.FriendsEnabled, Seed: nil,
	}
	monsterTypes := g.resolveCombatEncounterTypes(typeRequest, friendType, count)
	g.ShuffleDirections(positions)
	result := &DebugCombatSpawnResult{
		EncounterID: request.EncounterID, Seed: seed, TotalCountBefore: combat.MonsterManager.Count(),
		ActiveCountBefore: combat.MonsterManager.CountActive(), AvailablePositions: len(positions),
		VictoryBefore: combat.Victory, VictoryAfter: false,
		PrimaryCount: 0, FriendCount: 0, TotalCountAfter: 0, ActiveCountAfter: 0,
		Spawned: make([]DebugCombatSpawnedMonster, 0, count),
	}
	for index, monsterType := range monsterTypes {
		enemyRef := primaryRef
		friend := monsterType != request.MonsterType
		if friend {
			enemyRef, err = g.enemyReferenceForCombat(monsterType)
			if err != nil {
				panic("prevalidated combat friend reference became invalid")
			}
			result.FriendCount++
		} else {
			result.PrimaryCount++
		}
		monster := NewCombatMonster(monsterType, enemyRef, positions[index], g.AsRandomProvider())
		managerIndex := combat.AddMonsterToQueue(monster)
		result.Spawned = append(result.Spawned, DebugCombatSpawnedMonster{
			CombatantID: monster.CombatantID(), ManagerIndex: managerIndex,
			MonsterType: monsterType, Friend: friend, Position: positions[index],
		})
	}
	combat.Victory = false
	combat.DebugOutcomeSuppressed = false
	result.TotalCountAfter = combat.MonsterManager.Count()
	result.ActiveCountAfter = combat.MonsterManager.CountActive()
	result.VictoryAfter = combat.Victory
	assertDebugCombatMonsterPopulation(combat)

	return result, nil
}

// Historical manager records and queue entries survive death, flight, and replacement.
// Their exact identities must still agree, and only live monsters remain in rotation.
func assertDebugCombatMonsterPopulation(combat *CombatState) {
	combat.MonsterManager.ForEach(func(index int, monster *CombatMonster) {
		entry := combat.TurnQueue.GetEntryByIdentity(CombatTurnIdentity{EntityIndex: index})
		if entry == nil || entry.Combatant != monster || (entry.status == turnQueueActive) != monster.IsAlive() {
			panic("active combat monster manager and turn queue identities are out of sync")
		}
	})
	queuedMonsters := 0
	for _, entry := range combat.TurnQueue.entries {
		if !entry.Combatant.IsPC() {
			queuedMonsters++
		}
	}
	if queuedMonsters != combat.MonsterManager.Count() {
		panic("active combat monster manager and turn queue populations are out of sync")
	}
}

// Source combat queue slots and shared object slots have independent lifetimes. In
// particular, a corpse can retain a dead source slot while flight releases it.
// See ref:CMD-0001.
func debugCombatReinforcementCapacity(combat *CombatState) int {
	freeSourceSlots := 0
	for slot := sourceMonsterSlotStart; slot < sourceCombatQueueSlots; slot++ {
		if combat.TurnQueue.getRetainedEntryAtSourceSlot(slot) == nil {
			freeSourceSlots++
		}
	}

	return min(freeSourceSlots, sourceCombatQueueSlots-combat.originalObjectSlotCount())
}

func (g *GameState) validateDebugCombatBoundary(encounterID CombatEncounterID) error {
	if g.CombatScene == nil || !g.IsInCombat() {
		return ErrDebugCombatUnavailable
	}
	combat := g.CombatScene
	if combat.EncounterID != encounterID {
		return ErrDebugCombatStale
	}
	if g.EventQueueManager.HasActiveWorkflows() || combat.HasScheduledCombatStep() ||
		combat.ActiveMonsterIndex != NoActiveMonster || combat.ActiveTargetPCIndex != NoTargetPC ||
		combat.AttackTargeting != nil || combat.SpellTargeting != nil || combat.ConeTargeting != nil ||
		combat.AdjacentSpellTargeting != nil || combat.pendingSpell != nil || combat.pendingAttack != nil ||
		combat.pendingMonsterMissile != nil {
		return ErrDebugCombatBusy
	}

	return nil
}

func availableActiveCombatSpawnPositions(combat *CombatState) ([]references.Position, error) {
	occupied := make(map[references.Position]struct{}, sourceCombatQueueSlots)
	for _, entry := range combat.TurnQueue.entries {
		if entry.status != turnQueueActive || !entry.Combatant.IsAlive() {
			continue
		}
		identity := turnQueueIdentity(entry)
		if identity.IsPartyMember && (!combat.isCombatPCActive(identity) || combat.isCombatPCUnderground(identity)) {
			continue
		}
		occupied[combat.GetCombatantPosition(identity)] = struct{}{}
	}
	for _, object := range combat.objects {
		occupied[object.position] = struct{}{}
	}
	for _, field := range combat.fields {
		if field.BlocksMovement() {
			occupied[field.Position()] = struct{}{}
		}
	}
	positions := make([]references.Position, 0, len(combat.CombatMapRef.MonsterPositions))
	for index, position := range combat.CombatMapRef.MonsterPositions {
		if position.X < 0 || position.X >= references.XCombatMapTiles ||
			position.Y < 0 || position.Y >= references.YCombatMapTiles {
			return nil, fmt.Errorf("%w: monster position %d is outside combat map", ErrDebugCombatSpawn, index)
		}
		if _, found := occupied[position]; found {
			continue
		}
		occupied[position] = struct{}{}
		positions = append(positions, position)
	}

	return positions, nil
}
