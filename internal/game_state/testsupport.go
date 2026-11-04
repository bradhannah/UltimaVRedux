package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ActionEnterLargeMap handles entering buildings from the large map.
// Deprecated callers needing only broad command acceptance may use this wrapper;
// orchestration that depends on the destination must use ResolveEnterLargeMap.
func (g *GameState) ActionEnterLargeMap() bool {
	return g.ResolveEnterLargeMap().Accepted()
}

// Creating distinguishes a newly captured point from an existing-note edit.
func (d *AtlasNoteDraft) Creating() bool { return d.creating }

// ResetAllFoundStatus clears all found flags (useful for new games)
func (aslm *AuthenticSearchLocationManager) ResetAllFoundStatus() {
	aslm.objFound = [24]byte{}
}

// SearchAtPosition preserves the complete lookup-and-commit operation used by
// direct callers. Production world placement uses FindAtPosition and CommitFound.
func (aslm *AuthenticSearchLocationManager) SearchAtPosition(x, y references.Coordinate, level references.FloorNumber, onmap references.Location) *AuthenticSearchResult {
	result := aslm.FindAtPosition(x, y, level, onmap)
	if result != nil {
		aslm.CommitFound(result)
	}

	return result
}

// SetPoisoned sets the monster's poisoned state.
func (m *CombatMonster) SetPoisoned(poisoned bool) {
	m.conditions.isPoisoned = poisoned
}

// RemoveMonster removes a monster by index.
// Returns the removed monster, or nil if index is invalid.
func (m *CombatMonsterManager) RemoveMonster(index int) *CombatMonster {
	if index < 0 || index >= len(m.monsters) {
		return nil
	}

	removed := m.monsters[index]
	m.monsters = append(m.monsters[:index], m.monsters[index+1:]...)

	logging.Debugf(logging.CategoryGameState,
		"Removed combat monster: type=%s index=%d",
		removed.MonsterType().Name(), index)

	return removed
}

// GetMonsterTypeCount returns the count of a specific monster type.
func (m *CombatMonsterManager) GetMonsterTypeCount(monsterType references.MonsterType) int {
	count := 0
	for _, monster := range m.monsters {
		if monster.MonsterType() == monsterType {
			count++
		}
	}

	return count
}

// ReplaceCombatant updates a live queue entry without changing its queue slot.
func (q *CombatTurnQueue) ReplaceCombatant(identity CombatTurnIdentity, replacement CombatTurnIdentity, combatant CombatantProvider) {
	entry := q.GetEntryByIdentity(identity)
	if entry == nil {
		panic("CombatTurnQueue.ReplaceCombatant: missing queue entry")
	}
	q.nextEntryLifetimeID++
	entry.lifetimeID = q.nextEntryLifetimeID
	entry.Combatant = combatant
	entry.EntityIndex = replacement.EntityIndex
	entry.Slot = replacement.Slot
	entry.turnCountdown = resetTurnCountdown(combatant.GetDexterity())
	entry.status = turnQueueActive
}

// TotalEntryCount returns the total number of entries (including dead/fled).
func (q *CombatTurnQueue) TotalEntryCount() int {
	return len(q.entries)
}

// SetDungeonState sets the open/closed state of a specific dungeon
func (g *GameState) SetDungeonState(dungeon references.Location, isOpen bool) {
	index, exists := dungeonIndex(dungeon)
	if !exists {
		return // Not a valid dungeon
	}
	g.DungeonStates[index] = isOpen
}

// IsPartyMemberInvisible reports whether monsters can see the party member.
func (combatState *CombatState) IsPartyMemberInvisible(memberIndex int) bool {
	if memberIndex < 0 || memberIndex >= party_state.MaxActiveParty {
		return false
	}

	return combatState.PartyInvisible[memberIndex]
}

// ReplaceCombatantWithMonster ports Rel Xen Bet's deleteobj()+createentry()
// sequence: the target slot is released, then the rat uses the lowest free
// monster-region source slot with a fresh countdown.
func (combatState *CombatState) ReplaceCombatantWithMonster(identity CombatTurnIdentity, monster *CombatMonster) int {
	index, _ := combatState.replaceCombatantWithMonster(identity, monster)

	return index
}

// TransitionToCombatMap loads a combat map and sets up combat mode.
// mapIndex: the combat map index (0-15 for Britannia, varies for dungeon)
// territory: BritanniaCombatTerritory or DungeonCombatTerritory
// entryDir: the direction from which the party enters combat
func (g *GameState) TransitionToCombatMap(
	mapIndex int,
	territory references.CombatMapTerritory,
	entryDir CombatEntryDirection,
) error {
	// Load the combat map
	combatMap, err := references.LoadCombatMap(mapIndex, territory, g.GameConfig)
	if err != nil {
		return fmt.Errorf("loading combat map %d: %w", mapIndex, err)
	}
	g.transitionToCombatMapReference(combatMap, territory, entryDir)

	return nil
}

func (g *GameState) transitionToCombatMapReference(
	combatMap *references.CombatMapReference,
	territory references.CombatMapTerritory,
	entryDir CombatEntryDirection,
) {
	combatState := NewCombatState(combatMap, entryDir, &g.PartyState)
	g.configureCombatTurnQueue(combatState)
	g.initializeCombatPartyRings(combatState)
	g.commitCombatTransition(combatState, territory)

	logging.Debugf(logging.CategoryGameState,
		"Transitioned to combat map: %s (index=%d, territory=%d, entry=%d)",
		combatMap.MapName, combatMap.MapIndex, territory, entryDir)
}

// TransitionToDungeonCombatMap loads a dungeon room combat map.
// dungeonIndex: the dungeon (0-6, mapping Deceit→0, Destard→1, etc.)
// roomIndex: the room number within the dungeon (0-15)
// entryDir: the direction from which the party enters combat
func (g *GameState) TransitionToDungeonCombatMap(
	dungeonIndex references.DungeonCombatIndex,
	roomIndex int,
	entryDir CombatEntryDirection,
) error {
	if roomIndex < 0 || roomIndex >= references.MaxDungeonRooms {
		return fmt.Errorf("%w: %d (valid: 0-%d)", ErrRoomIndexOutOfRange, roomIndex, references.MaxDungeonRooms-1)
	}

	mapIndex := references.CalculateDungeonCombatMapIndex(dungeonIndex, roomIndex)

	if err := g.TransitionToCombatMap(mapIndex, references.DungeonCombatTerritory, entryDir); err != nil {
		return err
	}
	g.CombatScene.ExitContext = CombatExitDungeonRoom

	return nil
}

// HasPendingLordBritishFinale reports whether verified combat has handed off exactly once.
func (g *GameState) HasPendingLordBritishFinale() bool {
	return g.finaleHandoffPending
}

// LordBritishFinaleOutcome returns the current authoritative outcome.
func (g *GameState) LordBritishFinaleOutcome() LordBritishFinaleOutcome {
	if g.lordBritishFinale == nil {
		return LordBritishFinaleInProgress
	}

	return g.lordBritishFinale.outcome
}

// GetActiveMoongatePositions returns light-source positions while stage is nonzero.
func (g *GameState) GetActiveMoongatePositions() []references.Position {
	if g.moongateStage == MoongateStageHidden {
		return []references.Position{}
	}

	location := g.MapState.PlayerLocation

	return g.moonstoneSystem.GetActiveMoongatePositions(location.Location, location.Floor)
}

// GetMoonstoneIndexForMoonPhase returns the moonstone index for given moon phase
func (ms *MoonstoneSystem) GetMoonstoneIndexForMoonPhase(phase int) int {
	if phase >= 0 && phase <= 7 {
		return phase // Direct 1:1 mapping in original Ultima V
	}

	return 0 // Default to first moonstone
}

// SpawnShadowlordNPCAtPosition creates a Shadowlord enemy NPC at an explicit
// passable position. Source-fixed Yell and automatic placement use their
// unexported terrain policy instead.
//
// Parameters:
//   - lord: Which Shadowlord to spawn
//   - spawnPos: The position to spawn at
//
// Returns true if spawn was successful, false otherwise.
func (g *GameState) SpawnShadowlordNPCAtPosition(lord references.Shadowlord, spawnPos references.Position) bool {
	return g.insertShadowlordNPCAtPosition(lord, spawnPos, false) != nil
}

// BuildTeleportationSequence returns a function that adds teleportation phases to an existing event queue builder.
// This allows the caller to integrate teleportation into a larger sequence without nested queues.
//
// The returned function adds the following phases:
// - Phase 1: Avatar dissolve out (1 second)
// - Phase 2: Map fade to black (2 seconds)
// - Phase 3: Execute actual teleportation (instant)
// - Phase 4: Map fade from black (2 seconds)
// - Phase 5: Avatar dissolve in (1 second)
//
// Total duration: 6 seconds
//
// Usage:
//
//	queue.First(func() { /* setup */ }).
//	    ThenSequence(gs.BuildTeleportationSequence(destination)).
//	    Last(func() { /* cleanup */ })
func (g *GameState) BuildTeleportationSequence(destination MoongateDestination) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return g.buildTeleportationSequence(destination, journal.MoonstoneSourceNone)
}

// ProcessWaterfallHazard starts a cascade without charging an ordinary turn.
// The argument is retained for compatibility; eligibility always uses the current
// authoritative map and position, never a cached movement destination. Production
// commands enter through surface completion, which also retains their actor tail.
// Returns true while the retained logical cascade owns completion. ref:ENV-0003.
func (g *GameState) ProcessWaterfallHazard(_ *references.Position) bool {
	return g.startWaterfallHazard(nil)
}

// DebugSetupDungeon installs a fresh dungeon visit at one exact coordinate and
// replaces carried light with the requested state, without consuming time or RNG.
func (g *GameState) DebugSetupDungeon(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	facing references.DungeonFacing,
	lit bool,
) (DungeonSetupResult, error) {
	return g.debugSetupDungeon(location, floor, position, facing, references.DungeonLastDirection(facing), &lit)
}
