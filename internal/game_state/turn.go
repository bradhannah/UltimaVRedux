package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const (
	// RowingSpeed is the fixed time cost when rowing (sails furled)
	// Original Ultima V: Rowing always takes 2 minutes per tile regardless of wind
	RowingSpeed = 2

	// SkiffSpeed is the fixed time cost for skiff movement
	// Original Ultima V: Skiffs always take 2 minutes per tile (no wind effect, no HMS Cape)
	SkiffSpeed = 2

	// AutoWakeOdds sets the Redux one-in-sixteen natural-wake test.
	// REDUX DEVIATION: ENV-0002: original selects draw 15; Redux retains zero
	// to preserve its deterministic wake convention. This does not claim original RNG-stream parity.
	AutoWakeOdds = 16

	// CombatTimeAdvanceMinutes is the requested calendar increment after
	// every ten completed combat actions. One minute across ten actions is
	// an average six seconds per action, not a six-second increment. ref:TIME-0003
	CombatTimeAdvanceMinutes = 1

	beggarKarmaRewardThreshold = 100
	maximumBeggarKarmaCounter  = 255
)

// FinishTurn completes the owning map context's command cycle once.
func (g *GameState) FinishTurn() {
	if g.outdoorActorContinuation != nil {
		return
	}
	if g.surfaceCycleOwned && !g.completingSurfaceCycle {
		return
	}
	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	if mapType == references.CombatMapType {
		// Tactical completion has its own endaction boundary. Exploration player
		// updates would incorrectly apply poison, food, and duration work here.
		return
	}
	if g.IsWaterfallHazardActive() {
		return
	}
	if !g.completingSurfaceCycle && (mapType == references.SmallMapType || mapType == references.LargeMapType) {
		largeMapMinutes := DefaultLargeMapMinutesPerTurn
		if mapType == references.LargeMapType {
			largeMapMinutes = g.calculateMovementTimeCost(g.GetCurrentAvatarGroundTile())
		}
		g.completeSurfaceCycle(mapType, largeMapMinutes)

		return
	}
	// Whirlpool check removed - caller handles whirlpool detection and transportation
	// (prevents nested event queue violation when called from WhileAwaitingInput)

	// Capture before duration decrement so Time Stop's final action remains frozen.
	timeWasStopped := g.HasActiveDurationEffect(DurationEffectTimeStop)

	// Process all turn effects (NPCs and hazards) - no time advancement
	g.ProcessTurnEffects()

	// Time Stop freezes calendar advancement and torch consumption here;
	// action completion can still decrement its duration. ref:TIME-0001
	if timeWasStopped {
		return // Time frozen - no minute/hour advancement
	}

	// Advance time based on map type and movement cost
	switch mapType {
	case references.SmallMapType:
		g.advanceTimeSmallMap()
	case references.LargeMapType:
		g.advanceTimeLargeMap()
	case references.CombatMapType:
		// The DEX scheduler requests a time advance when selecting every tenth
		// actor, before that actor acts. ref:TIME-0003
	case references.DungeonMapType, references.CampingMapType:
		// Preserve queue-driven time for non-tactical contexts that complete turns
		// through FinishTurn rather than AdvanceCombatTurn.
		if g.CombatScene != nil && g.CombatScene.TurnQueue != nil &&
			g.CombatScene.TurnQueue.ShouldAdvanceTime() {
			g.AdvanceGameplayMinutes(CombatTimeAdvanceMinutes)
		}
	}

	if g.EventQueueManager.HasActiveWorkflows() {
		return
	}
	if err := g.ResolvePendingOverworldContact(); err != nil {
		logging.Errorf(logging.CategoryGameState, "Unable to start overworld combat: %v", err)
		g.SystemCallbacks.Message.AddRowStr("Combat could not begin.")
	}
}

// CompleteSurfaceCommandCycle runs the normal endplayer cycle for a completed
// town or overworld command. Overworld movement costs are deliberately absent.
func (g *GameState) CompleteSurfaceCommandCycle(originMapType references.GeneralMapType) {
	if g.IsWaterfallHazardActive() {
		return
	}
	if g.deferSurfaceCommandCycleForShipRepair(originMapType) {
		return
	}
	if g.deferSurfaceCommandCycleForCampingCombat(originMapType) {
		return
	}
	g.completeSurfaceCycle(originMapType, DefaultLargeMapMinutesPerTurn)
}

// CompleteSurfaceMovementCycle runs movement completion separately from
// command completion. The calculator supplies the total overworld movement cost.
func (g *GameState) CompleteSurfaceMovementCycle(originMapType references.GeneralMapType, moved bool) {
	if g.IsWaterfallHazardActive() {
		return
	}
	largeMapMinutes := DefaultLargeMapMinutesPerTurn
	if originMapType == references.LargeMapType {
		if !moved {
			return
		}
		largeMapMinutes = g.calculateMovementTimeCost(g.GetCurrentAvatarGroundTile())
		// Terrain slowdown runs extra monster phases before its calendar advancement,
		// then the ordinary endplayer tail advances another two minutes.
		// See ref:TIME-0002.
		extraMinutes := max(largeMapMinutes-DefaultLargeMapMinutesPerTurn, 0)
		if extraMinutes > 0 {
			if g.activeSlowMovement != nil {
				panic("slow movement completion already pending")
			}
			g.activeSlowMovement = &slowMovementState{
				remainingActorPhases: extraMinutes / DefaultLargeMapMinutesPerTurn,
				extraMinutes:         extraMinutes,
			}
			if err := g.resumeSlowMovement(); err != nil {
				logging.Errorf(logging.CategoryGameState, "Unable to resolve slow movement contact: %v", err)
				g.SystemCallbacks.Message.AddRowStr("Combat could not begin.")
			}

			return
		}
	}
	g.completeSurfaceCycle(originMapType, largeMapMinutes)
}

// slowMovementState retains check_slow's stack-local continuation while a
// monster phase hands control to tactical combat or a gameplay workflow.
type slowMovementState struct {
	remainingActorPhases int
	extraMinutes         int
}

func (g *GameState) resumeSlowMovement() error {
	state := g.activeSlowMovement
	for {
		if g.CombatScene != nil || g.ShouldDeferOverworldContact() {
			return nil
		}
		// Resolve the preceding phase before another can replace its contact.
		if err := g.resolvePendingOverworldContact(); err != nil {
			g.activeSlowMovement = nil

			return err
		}
		if g.CombatScene != nil || g.outdoorActorContinuation != nil || g.activeSlowMovement != state {
			return nil
		}
		if g.PartyState.IsAllPartyDead() || g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
			g.activeSlowMovement = nil

			return nil
		}
		if g.ShouldDeferOverworldContact() {
			return nil
		}
		if state.remainingActorPhases == 0 {
			g.AdvanceGameplayMinutes(state.extraMinutes)
			g.activeSlowMovement = nil
			g.completeSurfaceCycle(references.LargeMapType, DefaultLargeMapMinutesPerTurn)

			return nil
		}
		state.remainingActorPhases--
		g.processNonCombatWorldActorPhase()
	}
}

// CompleteSurfacePassCycle preserves the town Space exception to mounted cadence.
func (g *GameState) CompleteSurfacePassCycle(originMapType references.GeneralMapType) {
	g.completeSurfaceCycleForCommand(originMapType, DefaultLargeMapMinutesPerTurn, true, true)
}

func (g *GameState) completeSurfaceCycle(originMapType references.GeneralMapType, largeMapMinutes int) {
	g.completeSurfaceCycleWithActorPhase(originMapType, largeMapMinutes, true)
}

func (g *GameState) completeSurfaceCycleWithActorPhase(
	originMapType references.GeneralMapType,
	largeMapMinutes int,
	processActorPhase bool,
) {
	g.completeSurfaceCycleForCommand(originMapType, largeMapMinutes, processActorPhase, false)
}

func (g *GameState) completeSurfaceCycleForCommand(
	originMapType references.GeneralMapType, largeMapMinutes int, processActorPhase, pass bool,
) {
	// Repeated reviews cannot recharge the initiating command while its cascade
	// is queued, active, or awaiting input release.
	if g.IsWaterfallHazardActive() {
		return
	}
	g.completingSurfaceCycle = true
	defer func() { g.completingSurfaceCycle = false }()

	// The request still replaces the hour observation under Time Stop; the
	// central clock helper suppresses advancement before duration is decremented.
	switch originMapType {
	case references.SmallMapType:
		g.AdvanceGameplayMinutes(DefaultSmallMapMinutesPerTurn)
	case references.LargeMapType:
		g.AdvanceGameplayMinutes(largeMapMinutes)
	case references.CombatMapType, references.DungeonMapType, references.CampingMapType:
		panic("surface cycle requires a small-map or overworld origin")
	}
	g.processNonCombatPlayerTurnEffects()
	// Waterfalls are a consumed surface-cycle checkpoint, including stationary
	// Pass. The initiating charge/player update occurs once; forced descents own
	// only their existing fall consequences. The initiating actor tail runs after
	// the first complete descent, before later automatic falls. A retained logical
	// cascade releases its workflow for actor/contact handoffs and resumes before
	// ordinary input, without replaying this cycle. ref:ENV-0003, TIME-0002.
	if g.startWaterfallHazard(func() { g.completeSurfaceActorPhase(processActorPhase, pass) }) {
		return
	}
	g.completeSurfaceActorPhase(processActorPhase, pass)
}

func (g *GameState) completeSurfaceActorPhase(processActorPhase, pass bool) {
	if processActorPhase {
		g.processNonCombatWorldActorPhaseForCommand(pass)
	}

	if g.ShouldDeferOverworldContact() {
		return
	}
	if err := g.ResolvePendingOverworldContact(); err != nil {
		logging.Errorf(logging.CategoryGameState, "Unable to start overworld combat: %v", err)
		g.SystemCallbacks.Message.AddRowStr("Combat could not begin.")
	}
}

// BeginSurfaceCommandRouting transfers ordinary cycle ownership from Action*
// methods to the GameScene surface dispatcher until EndSurfaceCommandRouting.
func (g *GameState) BeginSurfaceCommandRouting() {
	if g.surfaceCycleOwned {
		panic("surface command routing ownership is already active")
	}
	g.surfaceCycleOwned = true
}

// EndSurfaceCommandRouting returns ordinary time ownership to Action* methods.
func (g *GameState) EndSurfaceCommandRouting() {
	if !g.surfaceCycleOwned {
		panic("surface command routing ownership is not active")
	}
	g.surfaceCycleOwned = false
}

// FinishCombatAction processes one combat action and advances the dex-based turn queue.
func (g *GameState) FinishCombatAction() {
	g.finishCombatAction()
	if g.CombatScene != nil {
		g.completeCombatPossessionTurn(g.CombatScene.possessionTurn)
		g.AdvanceCombatTurn()
	}
}

// FinishCombatActionDeferred resolves one player action while presentation delays queue advancement.
func (g *GameState) FinishCombatActionDeferred() CombatPresentationToken {
	if g.CombatScene == nil {
		panic("FinishCombatActionDeferred called outside combat")
	}

	return g.FinishCombatActionDeferredFor(g.CombatScene.GetCurrentTurnIdentity())
}

// FinishCombatActionDeferredFor completes the exact actor that opened a retained workflow.
func (g *GameState) FinishCombatActionDeferredFor(identity CombatTurnIdentity) CombatPresentationToken {
	if g.CombatScene == nil {
		panic("FinishCombatActionDeferredFor called outside combat")
	}
	if g.CombatScene.GetCurrentTurnIdentity() != identity {
		panic("FinishCombatActionDeferredFor actor changed before completion")
	}
	g.finishCombatAction()
	if g.CombatScene == nil {
		return CombatPresentationToken{}
	}

	return g.scheduleCombatPresentationStep(
		CombatPresentationTurnCompletion,
		combatPresentationAdvance,
		identity,
		NoActiveMonster,
		CombatTurnIdentity{},
		false,
		nil,
	)
}

func (g *GameState) finishCombatAction() {
	if g.CombatScene == nil {
		panic("finishCombatAction called outside combat")
	}

	identity := g.CombatScene.GetCurrentTurnIdentity()
	if controlledMonster, controlledMonsterIndex := g.CombatScene.GetControlledMonster(); controlledMonster != nil {
		g.resolveCombatMonsterActionEffects(controlledMonster, controlledMonsterIndex)

		return
	}
	g.resolveCombatControlledEndAction(identity)
	absorbed := g.resolveDoomMirrorAbsorption()
	if !absorbed {
		g.resolveCombatPCStandingEffects(identity)
	}
}

// resolveCombatControlledEndAction owns tactical action completion work. It is
// intentionally separate from exploration's party status update path.
// See ref:TIME-0002.
func (g *GameState) resolveCombatControlledEndAction(identity CombatTurnIdentity) {
	if identity.IsPartyMember {
		g.processCombatPCSpecialRing(identity)
	}
	g.resolveCombatFieldExpiration()
	g.processDurationEffects()
}

// FinishCombatTurnDeferred consumes a movement turn and returns the exact token
// the scene must complete after its visual interpolation.
func (g *GameState) FinishCombatTurnDeferred() CombatPresentationToken {
	if g.CombatScene == nil {
		panic("FinishCombatTurnDeferred called outside combat")
	}

	return g.scheduleCombatPresentationStep(
		CombatPresentationTurnCompletion,
		combatPresentationAdvance,
		g.CombatScene.GetCurrentTurnIdentity(),
		NoActiveMonster,
		CombatTurnIdentity{},
		false,
		nil,
	)
}

// FinishCombatSelection completes a numeric active-character selection without a mirror check.
func (g *GameState) FinishCombatSelection() {
	if g.CombatScene == nil {
		panic("FinishCombatSelection called outside combat")
	}
	if monster, monsterIndex := g.CombatScene.GetControlledMonster(); monster != nil {
		g.resolveCombatMonsterFieldEffects(monster, monsterIndex)
	} else {
		// Numeric actor selection bypasses ordinary action completion, but the
		// scheduler still applies actor-local standing effects. ref:COMBAT-0002
		g.resolveCombatPCStandingEffects(g.CombatScene.GetCurrentTurnIdentity())
	}
	g.completeCombatPossessionTurn(g.CombatScene.possessionTurn)
	g.AdvanceCombatTurn()
}

// ProcessTurnEffects processes the non-calendar work for one ordinary turn.
// Its world-actor phase maps to monster_main(); hazards, statuses, durations,
// toggle items, and party-wipe handling belong to the surrounding player turn.
func (g *GameState) ProcessTurnEffects() {
	g.processNonCombatPlayerTurnEffects()
	g.processNonCombatWorldActorPhase()
}

func (g *GameState) processNonCombatPlayerTurnEffects() {
	mapType := g.MapState.PlayerLocation.Location.GetMapType()

	// Combat resolves sleep only when the sleeping combatant reaches its DEX
	// boundary. Running town auto-wake here would consume a duplicate RNG roll.
	if mapType == references.SmallMapType {
		g.processAutoWake()
	}
	// Town status processing wakes sleepers before terrain exposure.
	// See ref:TIME-0002.
	stonegateTrap := mapType == references.SmallMapType && g.processStonegateLavaTrap()
	if !stonegateTrap {
		g.processDamageOnAdvanceTimeNonCombat()
	}
	if mapType == references.LargeMapType {
		g.processCodexGuardianCheckpoint()
		g.checkForUnderworldEarthquake()
	}

	statusCache := g.cachePlayerUpdateStatuses()
	g.processPoisonedCharactersForPlayerUpdate(statusCache)
	g.checkHourlyFoodConsumptionForDiners(statusCache.Diners)
	if mapType == references.SmallMapType || mapType == references.LargeMapType {
		g.advanceBeggarKarmaCounter()
	}

	// Process duration effects (scroll/spell effects like Protection, Negate Magic, Time Stop)
	// From original party-effect rules - decrements duration counter each turn
	// NOTE: Duration effects ALWAYS decrement, including time stop itself
	// See ref:TIME-0002.
	g.processDurationEffects()

	// Process toggle items (Amulet, Crown, Badge) - each can be active independently
	g.processToggleItems()

	// Dungeon completion has its own ordered player-update path. Combat ring
	// timing is owned separately from ordinary noncombat turn completion.
	if mapType == references.SmallMapType || mapType == references.LargeMapType {
		g.ProcessRegenerationRingSweep()
	}

	// Check for a total party wipe after the turn effects. Original fallback
	// selection accepts Good/Poisoned, distinguishes Sleeping, and does not
	// classify every other status as a living-capable actor. ref:RECOVERY-0002
	if !stonegateTrap {
		g.checkForPartyWipe()
	}
}

func (g *GameState) advanceBeggarKarmaCounter() {
	if g.beggarKarmaCounter < maximumBeggarKarmaCounter {
		g.beggarKarmaCounter++
	}
}

// processNonCombatWorldActorPhase is the Redux equivalent of one monster_main()
// phase without ordinary player hazards, statuses, durations, or toggle checks.
func (g *GameState) processNonCombatWorldActorPhase() {
	g.processNonCombatWorldActorPhaseForCommand(false)
}

func (g *GameState) processNonCombatWorldActorPhaseForCommand(pass bool) {
	if g.PartyState.IsAllPartyDead() || g.outdoorActorContinuation != nil || g.pendingOverworldEncounter != nil {
		return
	}
	if g.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType {
		// Door timing precedes all actor gates, including Time Stop.
		if g.MapState.SmallMapProcessTurnDoors(g.isSmallMapDoorOccupied) {
			g.SystemCallbacks.Audio.Play(SoundCloseDoor).Start()
		}
	}
	if !g.shouldProcessNonCombatWorldActorsForCommand(pass) {
		return
	}

	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType:
		g.smallMapProcessTurnEffects()
	case references.LargeMapType:
		g.largeMapProcessTurnEffects()
	case references.CombatMapType:
		// Combat turn effects handled by the combat system (CombatTurnQueue)
		// Future: process combat-specific per-turn effects (field damage, spell durations)
	case references.DungeonMapType, references.CampingMapType:
		logging.Warnf(logging.CategoryGameState, "ProcessTurnEffects called with unsupported map type: %v",
			g.MapState.PlayerLocation.Location.GetMapType())
	default:
		logging.Warnf(logging.CategoryGameState, "ProcessTurnEffects called with unsupported map type: %v",
			g.MapState.PlayerLocation.Location.GetMapType())
	}
}

func (g *GameState) shouldProcessNonCombatWorldActors() bool {
	return g.shouldProcessNonCombatWorldActorsForCommand(false)
}

func (g *GameState) shouldProcessNonCombatWorldActorsForCommand(pass bool) bool {
	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	if mapType != references.SmallMapType && mapType != references.LargeMapType {
		return false
	}
	vehicle := g.PartyVehicle.GetVehicleDetails().VehicleType
	mounted := vehicle == references.HorseVehicle || vehicle == references.CarpetVehicle
	// Original town rules gates riding before Time Stop and Quickness.
	// See ref:TIME-0002.
	if mapType == references.SmallMapType && mounted && !pass {
		g.townMountedActorSkip = !g.townMountedActorSkip
		if g.townMountedActorSkip {
			return false
		}
	}
	if g.HasActiveDurationEffect(DurationEffectTimeStop) {
		return false
	}
	if g.HasActiveDurationEffect(DurationEffectQuickness) {
		phase := &g.quicknessWorldActorSkip
		if mapType == references.SmallMapType {
			phase = &g.townQuicknessActorSkip
		}
		*phase = !*phase
		if *phase {
			return false
		}
	}
	// Original world-creature rules gates outdoor riding after Quickness, even on Pass.
	// See ref:TIME-0002.
	if mapType == references.LargeMapType && mounted {
		g.outdoorMountedActorSkip = !g.outdoorMountedActorSkip

		return !g.outdoorMountedActorSkip
	}

	return true
}

// processAutoWake uses the Redux zero-result boundary of a 0..15 draw
// for each sleeping character. This differs from the reviewed original
// upper-boundary selection. ref:ENV-0002
func (g *GameState) processAutoWake() {
	for i := range g.PartyState.GetCharacterCount() {
		if !g.PartyState.HasCharacter(i) {
			continue
		}
		char := g.PartyState.GetCharacter(i)
		if char.PartyMap != party_state.PartyMapActive {
			continue
		}

		if char.IsAsleep() && g.OneInXOdds(AutoWakeOdds) {
			char.SetStatus(party_state.Good)
			g.SystemCallbacks.Message.AddRowStr(char.GetNameAsString() + " wakes up!")
		}
	}
}

// largeMapProcessTurnEffects processes large map turn effects without time advancement.
// Handles source-ordered contact, ranged attacks, and NPC movement.
// Matches original Ultima V's outdoor actor phase behavior.
// See ref:TIME-0002.
func (g *GameState) largeMapProcessTurnEffects() {
	if g.outdoorActorContinuation != nil || g.pendingOverworldEncounter != nil {
		return
	}
	g.GetCurrentLargeMapNPCAIController().AdvanceNextTurnWithActorCheck(g.checkOutdoorActorAttack)
}

// advanceTimeLargeMap advances game time for large map movement.
// Uses movement-based time cost calculation (vehicle type, wind conditions).
func (g *GameState) advanceTimeLargeMap() {
	topTile := g.GetCurrentAvatarGroundTile()

	// Calculate movement time cost based on vehicle type and wind conditions
	timeCost := g.calculateMovementTimeCost(topTile)
	g.AdvanceGameplayMinutes(timeCost)
}

// calculateMovementTimeCost returns the per-move time cost.
func (g *GameState) calculateMovementTimeCost(tile *references.Tile) int {
	vehicle := g.PartyVehicle.GetVehicleDetails()

	// Frigate logic
	if vehicle.VehicleType == references.FrigateVehicle {
		// Raised-sail wind timing belongs to the automatic wait cadence. The
		// eventual accepted movement receives only ordinary outdoor completion.
		if vehicle.SailState == map_units.SailRaised {
			return DefaultLargeMapMinutesPerTurn
		}

		// Rowing (sails furled) retains its fixed movement timing. HMS Cape is
		// restricted to automatic raised-sail waits in sailing.go.
		return RowingSpeed
	}

	// Skiff: Fixed time (no wind effect, no HMS Cape)
	// Original: Skiff displays "Row" and doesn't enter sailing logic
	// See ref:TIME-0002.
	if vehicle.VehicleType == references.SkiffVehicle {
		return SkiffSpeed
	}

	// Terrain slowdown classifies terrain, independently of mounted cadence.
	// See ref:TIME-0002.
	extraPhases := 0
	switch {
	case tile.Is(indexes.Grass):
	case tile.Is(indexes.LeftDesert2) || tile.Is(indexes.RightDesert2):
		extraPhases = 1
	case tile.Index < indexes.Swamp || tile.Index >= indexes.Hut:
	case tile.Index >= indexes.Forest3:
		extraPhases = 2
	default:
		extraPhases = 1
	}

	return DefaultLargeMapMinutesPerTurn * (1 + extraPhases)
}

// smallMapProcessTurnEffects processes small map turn effects without time advancement.
// Handles door closing and NPC movement.
func (g *GameState) smallMapProcessTurnEffects() {
	g.smallMapProcessNPCs()
}

// advanceTimeSmallMap advances game time for small map movement.
// Uses fixed 1 minute per turn.
func (g *GameState) advanceTimeSmallMap() {
	// Small maps are always 1 minute per turn
	g.AdvanceGameplayMinutes(DefaultSmallMapMinutesPerTurn)
}

// AdvanceGameplayMinutes applies addtime semantics and returns actual elapsed minutes.
func (g *GameState) AdvanceGameplayMinutes(minutes int) int {
	return g.advanceGameplayMinutes(minutes, true)
}

// AdvanceGameplayMinutesAndProcessProvisions is for self-contained time costs
// that do not flow into an ordinary player-update phase.
func (g *GameState) AdvanceGameplayMinutesAndProcessProvisions(minutes int) int {
	elapsed := g.AdvanceGameplayMinutes(minutes)
	g.ProcessPendingProvisionHours()

	return elapsed
}

// AdvanceOrdinarySurfaceCommandMinutes preserves direct Action* callers while
// allowing production surface routing to own the normal command cycle.
func (g *GameState) AdvanceOrdinarySurfaceCommandMinutes(minutes int) int {
	if g.surfaceCycleOwned && !g.completingSurfaceCycle {
		return 0
	}

	return g.AdvanceGameplayMinutes(minutes)
}

func (g *GameState) advanceDungeonCampingMinutes(minutes int) int {
	return g.advanceGameplayMinutes(minutes, false)
}

// AdvanceBedRestMinutes advances bed-rest time without queueing ordinary provision processing.
func (g *GameState) AdvanceBedRestMinutes(minutes int) int {
	return g.advanceGameplayMinutes(minutes, false)
}

func (g *GameState) advanceGameplayMinutes(minutes int, consumeFood bool) int {
	if minutes <= 0 {
		return 0
	}
	if g.HasActiveDurationEffect(DurationEffectTimeStop) {
		// A nonzero time request replaces the observation even under Time Stop. ref:PROV-0002.
		g.lastProcessedHour = int(g.DateTime.Hour)
		g.pendingProvisionHours = nil

		return 0
	}

	elapsed := minutes
	if g.HasActiveDurationEffect(DurationEffectQuickness) {
		elapsed = max(minutes/2, 1)
	}

	return g.advanceElapsedGameplayMinutes(elapsed, consumeFood)
}

// advanceInnRestMinutes advances the calendar and lighting for paid Inn sleep
// without ordinary food consumption or active-spell time modifiers.
func (g *GameState) advanceInnRestMinutes(minutes int) {
	g.advanceElapsedGameplayMinutes(minutes, false)
}

func (g *GameState) advanceElapsedGameplayMinutes(elapsed int, consumeFood bool) int {
	if elapsed <= 0 {
		return 0
	}
	// Calendar and side effects must share the calendar's accepted interval.
	elapsed = min(elapsed, datetime.MaximumAdvanceMinutes)

	minuteBefore := int(g.DateTime.Minute)
	dateBefore := g.DateTime
	// Each nonzero request replaces the observation, including rest advances.
	// Deferred effects observe one hour pair, not an elapsed-hour backlog. ref:PROV-0002.
	g.lastProcessedHour = int(dateBefore.Hour)
	g.pendingProvisionHours = nil
	g.DateTime.Advance(elapsed)
	if consumeFood && dateBefore.Hour != g.DateTime.Hour {
		g.pendingProvisionHours = []int{int(g.DateTime.Hour)}
	}
	if dateBefore.Month != g.DateTime.Month {
		g.openingCampaignEncounterReduction = false
		g.SearchManager.resetDailyMarkersAtMonthBoundary()
	}
	if dateBefore.DateOnly() != g.DateTime.DateOnly() {
		g.moveShadowlordsAtMidnight()
	}
	g.maintainInnMonths(dateBefore, g.DateTime)
	g.MapState.Lighting.AdvanceMinutes(elapsed)
	g.advanceMagicLightMinutes(elapsed)
	g.GetCurrentLayeredMap().InvalidateLightSourceCache()

	hoursElapsed := (minuteBefore + elapsed) / 60
	if hoursElapsed > 0 && g.LastCamped > 0 {
		g.LastCamped = max(g.LastCamped-hoursElapsed, 0)
	}

	return elapsed
}

func (g *GameState) maintainInnMonths(before, after datetime.UltimaDate) {
	boundaries := britannianMonthOrdinal(after) - britannianMonthOrdinal(before)
	for range max(boundaries, 0) {
		g.PartyState.ForEachCharacter(func(_ int, character *party_state.PlayerCharacter) {
			if character.MonthsAtInn < maximumInnMonths {
				character.MonthsAtInn++
			}
		})
	}
}

func britannianMonthOrdinal(date datetime.UltimaDate) int {
	return int(date.Year)*datetime.MonthsPerYear + int(date.Month)
}

func (g *GameState) advanceMagicLightMinutes(minutes int) {
	if g.MagicLightMinutesRemaining <= 0 {
		return
	}

	g.MagicLightMinutesRemaining = max(g.MagicLightMinutesRemaining-minutes, 0)
	if g.MagicLightMinutesRemaining == 0 {
		g.MapState.Lighting.SetMagicLight(false)
	}
}

func (g *GameState) smallMapProcessNPCs() {
	// Original town action processing checks Shadowlord adjacency before action-6 movement.
	// A Shadowlord that moves adjacent is therefore handed off on a later NPC phase.
	source := g.queueAdjacentSmallMapShadowlordEncounter()
	if source != nil {
		controller, ok := g.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
		if !ok {
			panic("queued small-map encounter requires small-map NPC authority")
		}
		controller.AdvanceNextTurnCalcAndMoveNPCsExcluding(source)

		return
	}
	g.CurrentNPCAIController.AdvanceNextTurnCalcAndMoveNPCs()
}

// SmallMapProcessNPCs processes NPC movement without advancing game time
// Used by auto-door system to animate NPCs during door opening sequence
func (g *GameState) SmallMapProcessNPCs() {
	if !g.HasActiveDurationEffect(DurationEffectTimeStop) && g.shouldProcessNonCombatWorldActors() {
		g.smallMapProcessNPCs()
	}
}

// processDamageOnAdvanceTimeNonCombat
// Processes damage from lava, poison, etc. on the non-combat map
func (g *GameState) processDamageOnAdvanceTimeNonCombat() {
	// Process environmental hazards for standing-on-tile effects during turn advancement
	g.ProcessEnvironmentalHazardsOnTurnAdvancement()

	// Note: Wind changes are now handled independently in GameScene.updateFixed()
	// to match original Ultima V behavior where wind updates happen continuously
}

// checkHourlyFoodConsumption observes hour changes after time advances
// and applies meal processing at hours 6, 12 and 18. The recorded last
// processed boundary prevents repeating an already processed meal. ref:PROV-0002
func (g *GameState) checkHourlyFoodConsumption() {
	g.checkHourlyFoodConsumptionForDiners(g.cachePlayerUpdateStatuses().Diners)
}

func (g *GameState) checkHourlyFoodConsumptionForDiners(diners int) {
	currentHour := int(g.DateTime.Hour)
	g.pendingProvisionHours = nil
	if currentHour == g.lastProcessedHour {
		return
	}
	g.lastProcessedHour = currentHour
	if g.PartyState.Inventory.Provisions.Food.Get() == 0 {
		g.applyStarvationDamageWithFeedback()

		return
	}
	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	showEating := mapType == references.LargeMapType || mapType == references.SmallMapType
	g.consumeMealWithFeedback(currentHour, diners, showEating)
}

// ProcessPendingProvisionHours resolves deferred meal or starvation effects at
// a stable workflow boundary.
func (g *GameState) ProcessPendingProvisionHours() {
	if len(g.pendingProvisionHours) == 0 {
		return
	}

	g.checkHourlyFoodConsumption()
	g.checkForPartyWipe()
}

func (g *GameState) consumeMealWithFeedback(hour, diners int, showEating bool) {
	foodBefore := g.PartyState.Inventory.Provisions.Food.Get()
	g.PartyState.Inventory.ConsumeMealTimeFood(hour, diners)
	if showEating && g.PartyState.Inventory.Provisions.Food.Get() < foodBefore {
		g.SystemCallbacks.UI.ShowTransientIcon(TransientIconEating)
	}
}

func (g *GameState) applyStarvationDamageWithFeedback() {
	g.SystemCallbacks.UI.ShowActionFeedback("Starving!", true)
	deadIndexes := g.applyStarvationDamage()
	g.ShowStarvationFeedback()
	for _, index := range deadIndexes {
		character := g.PartyState.GetCharacter(index)
		if character != nil {
			g.SystemCallbacks.Message.AddRowStr("Alas! " + character.GetNameAsString() + " has died!")
			g.ShowDeathFeedback()
		}
	}
}

func (g *GameState) applyStarvationDamage() []int {
	g.SystemCallbacks.UI.ShowTransientIcon(TransientIconStarving)

	return g.PartyState.DamageFromStarvation(g.RandomIntInRange)
}
