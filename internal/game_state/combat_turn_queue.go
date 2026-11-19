package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// CombatantProvider abstracts the stats the turn queue needs from a combatant.
// Implemented by both PC wrappers and CombatMonster, avoiding data duplication.
type CombatantProvider interface {
	// GetDexterity returns current effective dexterity for turn ordering.
	GetDexterity() int

	// IsAlive returns true while this combatant remains structurally eligible
	// for the DEX rotation. Temporary conditions do not make a combatant dead.
	IsAlive() bool

	// CanTakeTurn returns true if this combatant can act this turn.
	// False for temporary conditions such as sleep.
	CanTakeTurn() bool

	// IsPC returns true if this combatant is a player character.
	IsPC() bool
}

// turnQueueEntryStatus tracks the lifecycle of a queue entry.
type turnQueueEntryStatus int

const (
	turnQueueActive turnQueueEntryStatus = iota
	turnQueueDead
	turnQueueFled
	turnQueueAbsorbed
)

// CombatTurnStatus describes whether a combatant remains in the turn rotation.
type CombatTurnStatus int

const (
	// CombatTurnActive means the combatant remains eligible for turns.
	CombatTurnActive CombatTurnStatus = iota
	// CombatTurnDead means the combatant died and left the rotation.
	CombatTurnDead
	// CombatTurnFled means the combatant escaped the battlefield.
	CombatTurnFled
	// CombatTurnAbsorbed means the combatant entered Doom's final mirror.
	CombatTurnAbsorbed
)

// CombatTurnIdentity identifies a combatant without exposing its implementation.
type CombatTurnIdentity struct {
	IsPartyMember bool
	EntityIndex   int
	// Slot distinguishes encounter-local PC clones that share EntityIndex.
	// Slot zero is the character's normal combat entry.
	Slot int
}

// CombatTurnForecastEntry describes an actor at its forecasted action.
// Countdown is the live projected countdown after that action resets it.
type CombatTurnForecastEntry struct {
	CombatTurnIdentity
	Dexterity       int
	Countdown       int
	CanTakeTurn     bool
	ActionUncertain bool
	Status          CombatTurnStatus
}

// CombatTemporalEffects describes the scheduling effect active at a queue boundary.
type CombatTemporalEffects struct {
	EffectType     DurationEffectType
	TurnsRemaining int
	IsMonsterLoyal func(CombatTurnIdentity) bool
}

// TurnQueueEntry pairs a combatant reference with turn-specific countdown state.
// The queue does NOT duplicate stats — it reads them via CombatantProvider.
type TurnQueueEntry struct {
	// lifetimeID changes on registration/replacement, never on dispatch or death.
	// It lets detached results reject a reused identity without retaining providers.
	lifetimeID uint64

	// Combatant is the PC or monster this entry refers to.
	Combatant CombatantProvider

	// EntityIndex is the party member index (0-5) for PCs,
	// or the monster manager index for monsters.
	EntityIndex int
	Slot        int
	// sourceSlot preserves the original combat queue[32] slot used by source-order
	// scans, random victim selection, and lowest-free-slot creation.
	// See ref:COMBAT-0002.
	sourceSlot         int
	sourceSlotReleased bool
	// originalObjectSlot is one-based so the zero value means no object slots slot.
	// It is independent of sourceSlot: combat queue[] and object slots have separate
	// 32-slot allocation and lifetime rules in the original game.
	// See ref:COMBAT-0002.
	originalObjectSlot uint8

	// turnCountdown is decremented each pass through the queue.
	// When it reaches 0, this combatant gets to act.
	// Reset to (turnCountdownBase - dex) after acting.
	turnCountdown int

	// dispatchGeneration counts this exact entry's actual scheduled opportunities,
	// including turns later skipped by sleep, captivity, loyalty or temporal rules.
	// It is encounter-local input identity, not a forecast or a gameplay clock.
	dispatchGeneration uint64

	// status tracks whether this entry remains active or how it left combat.
	status turnQueueEntryStatus

	ratAppearance bool
}

// turnCountdownBase is the constant from original combat scheduler.
// Turn countdown = turnCountdownBase - dexterity.
// Higher dex → lower countdown → more frequent turns.
//
// See ref:COMBAT-0002.

const turnCountdownBase = 36

const (
	sourceCombatQueueSlots = 32
	sourceMonsterSlotStart = 6
)

// actionsPerTimeAdvance counts completed actions before requesting
// one calendar minute: ten actions per request. ref:TIME-0003

const actionsPerTimeAdvance = 10

// CombatTurnQueue implements the original Ultima V dex-based turn ordering system.
//
// The original countdown scheduler (original combat scheduler) iterates a 32-slot array each pass.
// For each slot: skip empty/dead, decrement countdown, and if countdown reaches 0,
// that combatant acts. This gives higher-dex combatants more frequent turns.
//
// This Go implementation uses a dynamic slice instead of a fixed array,
// but produces identical turn ordering for the same set of combatants and dex values.
// See ref:COMBAT-0002.
type CombatTurnQueue struct {
	entries []*TurnQueueEntry
	// Monotonic within this queue, including after removal of retired entries.
	nextEntryLifetimeID uint64

	// actionCount tracks total actions taken. Every actionsPerTimeAdvance actions,
	// the caller should advance game time by 1 unit (6 seconds in original).
	actionCount int

	// currentIndex retains the position within the current queue pass.
	// After the final entry, scanning resumes at slot zero. ref:COMBAT-0002
	currentIndex int

	temporalEffectsResolver func() CombatTemporalEffects
}

// NewCombatTurnQueue creates a new empty turn queue.
func NewCombatTurnQueue() *CombatTurnQueue {
	return &CombatTurnQueue{
		entries:             make([]*TurnQueueEntry, 0),
		actionCount:         0,
		currentIndex:        0,
		nextEntryLifetimeID: 0,
	}
}

// SetTemporalEffectsResolver connects the queue to encounter-owned spell state.
func (q *CombatTurnQueue) SetTemporalEffectsResolver(resolver func() CombatTemporalEffects) {
	q.temporalEffectsResolver = resolver
}

// AddCombatant adds a combatant to the turn queue.
// PCs should be added first (indices 0-5), then monsters, matching the original
// combat queue[] layout where slots 0-5 are PCs and 6-31 are monsters.
// The initial countdown is set to (turnCountdownBase - dex).
// See ref:COMBAT-0002.
func (q *CombatTurnQueue) AddCombatant(combatant CombatantProvider, entityIndex int) {
	q.AddCombatantAtSlot(combatant, entityIndex, 0)
}

// AddCombatantAtSlot adds a combatant using an encounter-local identity slot.
func (q *CombatTurnQueue) AddCombatantAtSlot(combatant CombatantProvider, entityIndex, slot int) {
	start := sourceMonsterSlotStart
	if combatant.IsPC() {
		start = 0
	}
	sourceSlot, found := q.claimSourceSlot(start)
	if !found {
		panic("CombatTurnQueue.AddCombatantAtSlot: source combat queue is full")
	}
	q.addCombatant(combatant, entityIndex, slot, sourceSlot, nil)
}

// AddClonedCombatant gives a clone the lowest free encounter slot and
// copies its source actor’s current countdown. ref:SPELL-0005
func (q *CombatTurnQueue) AddClonedCombatant(
	combatant CombatantProvider,
	entityIndex, slot int,
	source CombatTurnIdentity,
) {
	sourceEntry := q.GetEntryByIdentity(source)
	if sourceEntry == nil {
		panic("CombatTurnQueue.AddClonedCombatant: missing source entry")
	}
	sourceSlot, found := q.claimSourceSlot(0)
	if !found {
		panic("CombatTurnQueue.AddClonedCombatant: source combat queue is full")
	}
	countdown := sourceEntry.turnCountdown
	q.addCombatant(combatant, entityIndex, slot, sourceSlot, &countdown)
	q.GetEntryByIdentity(CombatTurnIdentity{IsPartyMember: combatant.IsPC(), EntityIndex: entityIndex, Slot: slot}).ratAppearance = sourceEntry.ratAppearance
}

func (q *CombatTurnQueue) addCombatant(
	combatant CombatantProvider,
	entityIndex, slot, sourceSlot int,
	countdownOverride *int,
) {
	dex := combatant.GetDexterity()
	countdown := resetTurnCountdown(dex)
	if countdownOverride != nil {
		countdown = *countdownOverride
	}

	q.nextEntryLifetimeID++
	entry := &TurnQueueEntry{
		lifetimeID:         q.nextEntryLifetimeID,
		Combatant:          combatant,
		EntityIndex:        entityIndex,
		Slot:               slot,
		sourceSlot:         sourceSlot,
		sourceSlotReleased: false,
		originalObjectSlot: 0,
		turnCountdown:      countdown,
		dispatchGeneration: 0,
		status:             turnQueueActive,
		ratAppearance:      false,
	}

	insertAt := len(q.entries)
	for index, existing := range q.entries {
		if existing.sourceSlot > sourceSlot {
			insertAt = index

			break
		}
	}
	q.entries = append(q.entries, nil)
	copy(q.entries[insertAt+1:], q.entries[insertAt:])
	q.entries[insertAt] = entry
	// currentIndex points at the next source slot to scan. Inserting behind that
	// point must not grant the new entry an extra action in the current pass.
	sourceSlotAlreadyScanned := false
	for index := range insertAt {
		if index < q.currentIndex && q.entries[index].sourceSlot == sourceSlot {
			sourceSlotAlreadyScanned = true

			break
		}
	}
	if insertAt < q.currentIndex || sourceSlotAlreadyScanned {
		q.currentIndex++
	}

	pcLabel := "monster"
	if combatant.IsPC() {
		pcLabel = "PC"
	}

	logging.Debugf(logging.CategoryGameState,
		"CombatTurnQueue: added %s entityIndex=%d sourceSlot=%d dex=%d countdown=%d",
		pcLabel, entityIndex, sourceSlot, dex, countdown)
}

// firstFreeSourceSlot returns the lowest queue slot a new combatant may take.
//
// REDUX DEVIATION: COMBAT-0001: the original keeps a dead monster's queue entry
// as the object for its corpse or chest, so the slot stays taken and a full
// fight stays full after a kill. Redux counts only the living: once no slot is
// empty, the lowest slot held only for a dead monster's remains is offered. The
// remains stay on the battlefield. Slot choice is unchanged while any slot is
// empty, so ordinary fights keep the original creation order.
func (q *CombatTurnQueue) firstFreeSourceSlot(start int) (int, bool) {
	for sourceSlot := start; sourceSlot < sourceCombatQueueSlots; sourceSlot++ {
		if q.getRetainedEntryAtSourceSlot(sourceSlot) == nil {
			return sourceSlot, true
		}
	}
	for sourceSlot := start; sourceSlot < sourceCombatQueueSlots; sourceSlot++ {
		if q.getRetainedEntryAtSourceSlot(sourceSlot).holdsSlotOnlyForRemains() {
			return sourceSlot, true
		}
	}

	return 0, false
}

// holdsSlotOnlyForRemains reports a dead monster whose queue slot is kept solely
// because it left a corpse or chest behind.
func (entry *TurnQueueEntry) holdsSlotOnlyForRemains() bool {
	return entry.status == turnQueueDead && !entry.sourceSlotReleased && !entry.Combatant.IsPC()
}

// claimSourceSlot takes the slot firstFreeSourceSlot offers, releasing it from
// a dead monster's remains when that is what made it available.
func (q *CombatTurnQueue) claimSourceSlot(start int) (int, bool) {
	sourceSlot, found := q.firstFreeSourceSlot(start)
	if !found {
		return 0, false
	}
	if held := q.getRetainedEntryAtSourceSlot(sourceSlot); held != nil {
		held.sourceSlotReleased = true
	}

	return sourceSlot, true
}

func (q *CombatTurnQueue) hasFreeSourceSlot(start int) bool {
	_, found := q.firstFreeSourceSlot(start)

	return found
}

func (q *CombatTurnQueue) getActiveEntryAtSourceSlot(sourceSlot int) *TurnQueueEntry {
	for index := len(q.entries) - 1; index >= 0; index-- {
		entry := q.entries[index]
		if entry.sourceSlot == sourceSlot && entry.status == turnQueueActive && entry.Combatant.IsAlive() {
			return entry
		}
	}

	return nil
}

func (q *CombatTurnQueue) getRetainedEntryAtSourceSlot(sourceSlot int) *TurnQueueEntry {
	for index := len(q.entries) - 1; index >= 0; index-- {
		entry := q.entries[index]
		if entry.sourceSlot == sourceSlot && !entry.sourceSlotReleased {
			return entry
		}
	}

	return nil
}

// AdvanceToNextActor scans active queue slots in persistent order,
// decrements each countdown, and selects the first reaching zero. Reset that
// actor to 36 minus Dexterity before handing off its action. Empty/dead slots
// are skipped; nil means no active combatants remain. ref:COMBAT-0002
func (q *CombatTurnQueue) AdvanceToNextActor() *TurnQueueEntry {
	if len(q.entries) == 0 {
		return nil
	}

	// Safety: limit iterations to prevent infinite loop if all combatants are inactive.
	// In the worst case, we need to pass through the entire slice multiple times
	// (up to turnCountdownBase passes for the slowest combatant).
	maxIterations := len(q.entries) * turnCountdownBase

	for range maxIterations {
		entry := q.entries[q.currentIndex]

		// Advance to next slot for the next call
		q.currentIndex++
		if q.currentIndex >= len(q.entries) {
			q.currentIndex = 0
		}

		// Skip inactive entries (dead, fled, empty)
		if entry.status != turnQueueActive {
			continue
		}

		// Temporary conditions are handled only after the actor reaches its
		// scheduled boundary, matching the original countdown scheduler flow.
		// See ref:COMBAT-0002.
		if !entry.Combatant.IsAlive() {
			continue
		}

		// Decrement the countdown and skip a slot while it remains nonzero. ref:COMBAT-0002
		entry.turnCountdown--
		if entry.turnCountdown > 0 {
			continue
		}

		// At zero, reset the countdown to 36 minus Dexterity before resolving
		// the actor’s scheduled boundary. ref:COMBAT-0002
		dex := entry.Combatant.GetDexterity()
		entry.turnCountdown = resetTurnCountdown(dex)

		// Track action count for time advancement
		q.actionCount++
		// Record the reached opportunity before turn-start rules may skip it.
		// Other entries' dispatches never advance this entry's generation.
		entry.dispatchGeneration++

		logging.Debugf(logging.CategoryGameState,
			"CombatTurnQueue: actor entityIndex=%d isPC=%t dex=%d (action #%d)",
			entry.EntityIndex, entry.Combatant.IsPC(), dex, q.actionCount)

		return entry
	}

	// No active combatants found after full scan
	logging.Warnf(logging.CategoryGameState,
		"CombatTurnQueue: no active combatants found after %d iterations", maxIterations)

	return nil
}

// FillForecast predicts subsequent actors into dst and returns the number filled.
// It snapshots combatant scalars and simulates on copied countdown state, leaving
// the authoritative queue and combatants unchanged.
func (q *CombatTurnQueue) FillForecast(dst []CombatTurnForecastEntry) int {
	if len(dst) == 0 || len(q.entries) == 0 {
		return 0
	}

	type forecastState struct {
		identity    CombatTurnIdentity
		dexterity   int
		countdown   int
		isAlive     bool
		canTakeTurn bool
		status      turnQueueEntryStatus
	}

	effects := q.temporalEffects()
	states := make([]forecastState, len(q.entries))
	for i, entry := range q.entries {
		states[i] = forecastState{
			identity: CombatTurnIdentity{
				IsPartyMember: entry.Combatant.IsPC(),
				EntityIndex:   entry.EntityIndex,
				Slot:          entry.Slot,
			},
			dexterity:   entry.Combatant.GetDexterity(),
			countdown:   entry.turnCountdown,
			isAlive:     entry.Combatant.IsAlive(),
			canTakeTurn: entry.Combatant.CanTakeTurn(),
			status:      entry.status,
		}
	}

	currentIndex := q.currentIndex
	filled := 0
	maxIterations := len(states) * turnCountdownBase
	for filled < len(dst) {
		foundActor := false
		for range maxIterations {
			state := &states[currentIndex]
			currentIndex++
			if currentIndex >= len(states) {
				currentIndex = 0
			}

			if state.status != turnQueueActive || !state.isAlive {
				continue
			}

			state.countdown--
			if state.countdown > 0 {
				continue
			}

			dexterity := state.dexterity
			state.countdown = resetTurnCountdown(dexterity)
			canTakeTurn := state.canTakeTurn
			monsterLoyal := combatTemporalMonsterLoyal(effects, state.identity)
			actionUncertain := effects.EffectType == DurationEffectQuickness && monsterLoyal
			if effects.EffectType == DurationEffectTimeStop && monsterLoyal {
				canTakeTurn = false
			}
			dst[filled] = CombatTurnForecastEntry{
				CombatTurnIdentity: state.identity,
				Dexterity:          dexterity,
				Countdown:          state.countdown,
				CanTakeTurn:        canTakeTurn,
				ActionUncertain:    actionUncertain,
				Status:             CombatTurnStatus(state.status),
			}
			filled++
			foundActor = true
			if effects.TurnsRemaining > 0 && !monsterLoyal {
				effects.TurnsRemaining--
				if effects.TurnsRemaining == 0 {
					effects.EffectType = DurationEffectNone
				}
			}

			break
		}

		if !foundActor {
			break
		}
	}

	return filled
}

// GetEntrySnapshot returns the current live queue state for one combatant.
func (q *CombatTurnQueue) GetEntrySnapshot(identity CombatTurnIdentity) (CombatTurnForecastEntry, bool) {
	effects := q.temporalEffects()
	for _, entry := range q.entries {
		if entry.EntityIndex != identity.EntityIndex || entry.Slot != identity.Slot || entry.Combatant.IsPC() != identity.IsPartyMember {
			continue
		}

		canTakeTurn := entry.Combatant.CanTakeTurn()
		monsterLoyal := combatTemporalMonsterLoyal(effects, identity)
		if effects.EffectType == DurationEffectTimeStop && monsterLoyal {
			canTakeTurn = false
		}

		return CombatTurnForecastEntry{
			CombatTurnIdentity: identity,
			Dexterity:          entry.Combatant.GetDexterity(),
			Countdown:          entry.turnCountdown,
			CanTakeTurn:        canTakeTurn,
			ActionUncertain:    effects.EffectType == DurationEffectQuickness && monsterLoyal,
			Status:             CombatTurnStatus(entry.status),
		}, true
	}

	var empty CombatTurnForecastEntry

	return empty, false
}

func (q *CombatTurnQueue) temporalEffects() CombatTemporalEffects {
	if q.temporalEffectsResolver == nil {
		return CombatTemporalEffects{}
	}

	return q.temporalEffectsResolver()
}

func turnQueueIdentity(entry *TurnQueueEntry) CombatTurnIdentity {
	return CombatTurnIdentity{
		IsPartyMember: entry.Combatant.IsPC(),
		EntityIndex:   entry.EntityIndex,
		Slot:          entry.Slot,
	}
}

func combatTemporalMonsterLoyal(effects CombatTemporalEffects, identity CombatTurnIdentity) bool {
	return effects.IsMonsterLoyal != nil && effects.IsMonsterLoyal(identity)
}

func resetTurnCountdown(dexterity int) int {
	return max(turnCountdownBase-dexterity, 1)
}

// ShouldAdvanceTime returns true and resets the counter if enough actions
// have occurred to warrant a game time advancement.
// Original combat scheduler: every 10 actions, calendar advancement.
// See ref:COMBAT-0002.
func (q *CombatTurnQueue) ShouldAdvanceTime() bool {
	if q.actionCount >= actionsPerTimeAdvance {
		q.actionCount -= actionsPerTimeAdvance

		return true
	}

	return false
}

// MarkDead marks a combatant as dead, removing them from the turn rotation.
func (q *CombatTurnQueue) MarkDead(entityIndex int, isPC bool) {
	for _, entry := range q.entries {
		if entry.EntityIndex == entityIndex && entry.Combatant.IsPC() == isPC {
			entry.status = turnQueueDead

			logging.Debugf(logging.CategoryGameState,
				"CombatTurnQueue: marked dead entityIndex=%d isPC=%t", entityIndex, isPC)

			return
		}
	}
}

func (q *CombatTurnQueue) markDeadIdentity(identity CombatTurnIdentity) {
	entry := q.GetEntryByIdentity(identity)
	if entry == nil {
		return
	}
	entry.status = turnQueueDead
}

func (q *CombatTurnQueue) releaseSourceSlot(identity CombatTurnIdentity) {
	entry := q.GetEntryByIdentity(identity)
	if entry == nil {
		return
	}
	entry.sourceSlotReleased = true
	entry.originalObjectSlot = 0
}

// RemoveAllMonsters removes monster entries while preserving the party rotation.
// Debug clear uses this instead of marking defeats because clear has no outcome.
func (q *CombatTurnQueue) RemoveAllMonsters() {
	retained := q.entries[:0]
	for _, entry := range q.entries {
		if entry.Combatant.IsPC() {
			retained = append(retained, entry)
		}
	}
	q.entries = retained
	q.currentIndex = 0
}

// MarkFled marks a combatant as having fled combat.
// Original combat action rules: deleteobj(-turn-1)
// See ref:COMBAT-0002.
func (q *CombatTurnQueue) MarkFled(entityIndex int, isPC bool) {
	for _, entry := range q.entries {
		if entry.EntityIndex == entityIndex && entry.Combatant.IsPC() == isPC {
			entry.status = turnQueueFled
			entry.sourceSlotReleased = true
			entry.originalObjectSlot = 0

			logging.Debugf(logging.CategoryGameState,
				"CombatTurnQueue: marked fled entityIndex=%d isPC=%t", entityIndex, isPC)

			return
		}
	}
}

func (q *CombatTurnQueue) markFledIdentity(identity CombatTurnIdentity) {
	entry := q.GetEntryByIdentity(identity)
	if entry == nil {
		return
	}
	entry.status = turnQueueFled
	entry.sourceSlotReleased = true
	entry.originalObjectSlot = 0
}

// MarkAbsorbed removes one exact party combatant without changing persistent character status.
func (q *CombatTurnQueue) MarkAbsorbed(identity CombatTurnIdentity) {
	entry := q.GetEntryByIdentity(identity)
	if entry == nil || !identity.IsPartyMember {
		panic("CombatTurnQueue.MarkAbsorbed: missing party entry")
	}
	entry.status = turnQueueAbsorbed
	entry.sourceSlotReleased = true
	entry.originalObjectSlot = 0
	logging.Debugf(logging.CategoryGameState,
		"CombatTurnQueue: marked absorbed entityIndex=%d slot=%d", identity.EntityIndex, identity.Slot)
}

// CountRemaining returns active player-loyal and monster-loyal combatant
// counts separately as (pcCount, monsterCount). ref:COMBAT-0011
func (q *CombatTurnQueue) CountRemaining() (int, int) {
	var pcCount, monsterCount int
	for _, entry := range q.entries {
		if entry.status != turnQueueActive {
			continue
		}

		if !entry.Combatant.IsAlive() {
			// Dead combatants don't count even if status hasn't been updated yet
			continue
		}

		if entry.Combatant.IsPC() {
			pcCount++
		} else {
			monsterCount++
		}
	}

	return pcCount, monsterCount
}

// GetEntryByIdentity returns the entry for an exact combat identity, including clones.
func (q *CombatTurnQueue) GetEntryByIdentity(identity CombatTurnIdentity) *TurnQueueEntry {
	for _, entry := range q.entries {
		if entry.EntityIndex == identity.EntityIndex && entry.Slot == identity.Slot && entry.Combatant.IsPC() == identity.IsPartyMember {
			return entry
		}
	}

	return nil
}
