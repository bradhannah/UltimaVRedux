// Package ai provides NPC artificial intelligence and pathfinding systems for Ultima 5 Redux.
//
// The package implements schedule-based AI for friendly NPCs and pathfinding
// for both friendly and enemy units. Key components:
//
//   - NPCAIControllerSmallMap: Manages NPC behavior in towns and buildings
//   - NPCAIControllerLargeMap: Handles overworld NPC movement
//   - RNGProvider interface: Enables deterministic AI behavior for testing
//   - A* pathfinding integration for intelligent movement
//
// NPC behavior is driven by schedules that specify location, time, and AI type.
// All randomness uses injected RNGProvider for reproducible behavior, ensuring
// consistent gameplay and enabling reliable testing.
//
// AI Types supported include:
//   - Wander: Random movement within area
//   - Fixed: Stationary NPC
//   - Follow: Track and follow the Avatar
//   - Attack: Hostile behavior toward player
//   - ExtortOrAttackOrFollow: Guard behavior
package ai

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/pkg/helpers"
)

// RNGProvider provides deterministic random number generation for AI
type RNGProvider interface {
	OneInXOdds(odds int) bool
	RandomIntInRange(minimum, maximum int) int
	ShuffleDirections(directions []references.Position)
}

// NPCDeadChecker allows checking if an NPC is dead without importing party_state.
// This enables filtering out dead NPCs (freed prisoners, killed NPCs) when loading a location.
type NPCDeadChecker interface {
	IsNPCDead(location references.Location, npcID int) bool
}

// shadowlordPursuitRange is the exclusive distance at which a Shadowlord pursues the Avatar.
// Based on the original nearby-attack schedule action (6); ref:NPC-0003:
// The Shadowlord only moves when Manhattan distance is less than four.
const (
	shadowlordPursuitRange           = 4
	originalWanderRadius             = 3
	originalPursuitRange             = 4
	originalRandomGateMaximum        = 255
	originalRandomDirectionMaximum   = 64
	originalAttackVariationMaximum   = 63
	originalAttackVariationThreshold = 16
	civilianFleeRange                = 8
	interactionApproachRange         = 5
	smallWanderDistance              = 2
	largeWanderDistance              = 4
	variationOdds                    = 4
	replacementOdds                  = 2
)

//nolint:gochecknoglobals // Source cardinal order is an immutable gameplay contract.
var originalNPCDirections = [...]references.Position{
	{X: 1, Y: 0},
	{X: 0, Y: -1},
	{X: -1, Y: 0},
	{X: 0, Y: 1},
}

type npcMovementPurpose int

const (
	npcMovementWander npcMovementPurpose = iota
	npcMovementApproach
	npcMovementSchedule
)

// NPCAIControllerSmallMap owns scheduled movement and contacts on local maps.
type NPCAIControllerSmallMap struct {
	tileRefs *references.Tiles
	slr      *references.SmallLocationReference
	dateTime *datetime.UltimaDate
	mapState *map_state.MapState
	mapUnits map_units.MapUnits

	// Dependency injection for deterministic RNG
	rngProvider RNGProvider

	// Dependency injection for checking dead NPCs (freed prisoners, etc.)
	deadChecker NPCDeadChecker
}

// NewNPCAIControllerSmallMap creates an AI controller for NPCs in small maps (towns, buildings, dungeons).
// The controller manages NPC movement, schedules, and behaviors based on time of day and AI types.
//
// Parameters:
//   - slr: Small location reference containing NPC definitions and schedules
//   - tileRefs: Tile reference data for map passability checks
//   - mapState: Current map state for NPC positioning and movement
//   - dateTime: Game time for schedule-based NPC behavior
//   - rngProvider: Random number generator for deterministic AI decisions
//   - deadChecker: Checker for filtering out dead NPCs (freed prisoners, killed NPCs)
//
// The controller uses schedule data to position NPCs and determine their behavior patterns.
// All randomness uses the provided RNGProvider for reproducible gameplay.
func NewNPCAIControllerSmallMap(
	slr *references.SmallLocationReference,
	tileRefs *references.Tiles,
	mapState *map_state.MapState,
	dateTime *datetime.UltimaDate,
	rngProvider RNGProvider,
	deadChecker NPCDeadChecker,
) *NPCAIControllerSmallMap {
	var controller NPCAIControllerSmallMap
	npcsAiCont := &controller
	npcsAiCont.dateTime = dateTime
	npcsAiCont.tileRefs = tileRefs
	npcsAiCont.slr = slr
	npcsAiCont.mapState = mapState
	npcsAiCont.rngProvider = rngProvider
	npcsAiCont.deadChecker = deadChecker

	npcsAiCont.mapUnits = make(map_units.MapUnits, 0, map_units.MaximumNpcsPerMap)

	return npcsAiCont
}

// GetNpcs returns the collection of NPCs managed by this controller.
func (n *NPCAIControllerSmallMap) GetNpcs() *map_units.MapUnits {
	return &n.mapUnits
}

// Location is the source location owned by this controller, independent of a
// player's destination already being installed during a map transition.
func (n *NPCAIControllerSmallMap) Location() references.Location {
	return n.slr.Location
}

// PopulateMapFirstLoad initializes and places all NPCs when entering a small map location.
// This method generates NPCs based on their schedules and positions them according to
// the current time of day.
func (n *NPCAIControllerSmallMap) PopulateMapFirstLoad() {
	n.generateNPCs()

	for _, npc := range n.mapUnits {
		if npc.IsEmptyMapUnit() || !npc.IsVisible() {
			continue
		}

		switch mapUnit := npc.(type) {
		case *map_units.NPCFriendly:
			slot := mapUnit.NPCReference.Schedule.DesiredSlot(*n.dateTime)
			mapUnit.PlaceAtScheduleSlot(slot)
		case *map_units.NPCEnemy:
			// do not support NPC Enemy on small map
		}
	}
	n.placeNPCsOnLayeredMap()
}

// SetFleeFromAvatar applies the original shape-based flee eligibility; ref:SHADOW-0003.
// The caller owns any random roll; false means the attempted flee was rejected.
func (n *NPCAIControllerSmallMap) SetFleeFromAvatar(npc *map_units.NPCFriendly) bool {
	if npc == nil {
		return false
	}

	sprite := npc.GetKeySpriteIndex()
	if sprite < indexes.Wizard1 || sprite >= indexes.Apparation1 {
		return false
	}

	movable := false
	for _, transitionTime := range npc.NPCReference.Schedule.Time {
		if transitionTime != 0 {
			movable = true

			break
		}
	}
	const originalFleeTalkState = 0xfe
	if npc.NPCReference.DialogNumber != originalFleeTalkState && !movable {
		return false
	}

	npc.MapUnitDetails().SetLawAIOverride(references.FleeFromAvatar)
	npc.MapUnitDetails().SetCurrentPath(nil)
	npc.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone

	return true
}

// SetNPCsReactToShadowlord applies Shadowlord presence effects to eligible NPCs.
// Original Shadowlord influence rules; ref:SHADOW-0003:
//   - Astaroth (Hatred): ~50% of regular NPCs become hostile (attack player)
//   - Nosfentor (Cowardice): ~50% of regular NPCs flee from player
//   - Faulinei (Falsehood): No NPC behavior change
//
// The shouldAffect function provides the 50% random chance per NPC.
func (n *NPCAIControllerSmallMap) SetNPCsReactToShadowlord(lord references.Shadowlord, shouldAffect func() bool) {
	// Faulinei has no NPC behavior effect (only steals gold in shops)
	if lord == references.Faulinei {
		return
	}

	for _, mapUnit := range n.mapUnits {
		friendly, ok := mapUnit.(*map_units.NPCFriendly)
		if !ok {
			continue
		}

		// Check if this NPC can be affected by Shadowlord presence
		if !n.isAffectedByShadowlord(friendly) {
			continue
		}

		// 50% random chance per NPC (matching original game)
		if !shouldAffect() {
			continue
		}

		// Apply effect based on Shadowlord type
		switch lord {
		case references.NoShadowlord, references.Faulinei:
		case references.Astaroth:
			// Hatred: NPCs become hostile and attack the player
			friendly.MapUnitDetails().SetAIOverride(
				references.DrudgeWorthThing,
				references.AIOverrideProvenanceAutomaticAstaroth,
			)
			friendly.MapUnitDetails().SetCurrentPath(nil)
			friendly.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone
		case references.Nosfentor:
			// Cowardice: NPCs flee from the player
			friendly.MapUnitDetails().SetAIOverride(
				references.FleeFromAvatar,
				references.AIOverrideProvenanceAutomaticNosfentor,
			)
		}
	}
}

// isAffectedByShadowlord determines if an NPC can be affected by Shadowlord presence.
// Original Shadowlord influence eligibility; ref:SHADOW-0003:
//   - Must have a schedule (movable NPC, not static)
//   - Must use an ordinary NPC shape
func (n *NPCAIControllerSmallMap) isAffectedByShadowlord(npc *map_units.NPCFriendly) bool {
	npcRef := npc.NPCReference

	// REDUX DEVIATION: The original influence check tests an NPC shape after
	// the schedule loop has advanced its index to 4, rather than testing the current actor's shape.
	// Redux applies the routine's stated per-candidate ordinary-NPC rule so an
	// unrelated array element cannot decide every candidate's eligibility.
	// See ref:SHADOW-0003.
	shape := npc.GetKeySpriteIndex()
	if shape < indexes.Wizard1 || shape >= indexes.Apparation1 {
		return false
	}

	// Check if NPC has a movable schedule (at least one time slot set)
	// Static NPCs (all times zero) are not affected
	hasSchedule := false
	for _, transitionTime := range npcRef.Schedule.Time {
		if transitionTime != 0 {
			hasSchedule = true

			break
		}
	}

	return hasSchedule
}

// calculateNextEnemyPosition handles movement for enemy NPCs on small maps.
// Currently only supports Shadowlords, which pursue the player using greedy movement.
// Based on the original nearby-attack schedule action (6); ref:NPC-0003:
// The Shadowlord only pursues at Manhattan distances 1 through 3.
func (n *NPCAIControllerSmallMap) calculateNextEnemyPosition(enemy *map_units.NPCEnemy) {
	// Only handle Shadowlords for now - other enemies not supported on small maps
	if enemy.Variant() != map_units.EnemyVariantShadowlord {
		return
	}

	avatarPos := n.mapState.PlayerLocation.Position
	enemyPos := enemy.PosPtr()

	// Calculate Manhattan distance to Avatar
	distance := helpers.AbsInt(int(enemyPos.X-avatarPos.X)) + helpers.AbsInt(int(enemyPos.Y-avatarPos.Y))

	// Original action 6 engages below Manhattan distance four; ref:NPC-0003.
	if distance >= shadowlordPursuitRange {
		logging.Debugf(logging.CategoryAI,
			"Shadowlord at (%d, %d) not pursuing - player too far (distance %d >= %d)",
			enemyPos.X, enemyPos.Y, distance, shadowlordPursuitRange)

		return
	}

	// Check if Shadowlord is adjacent to Avatar
	if enemyPos.IsNextTo(avatarPos, enemy.Floor(), n.mapState.PlayerLocation.Floor) {
		logging.Debugf(logging.CategoryAI,
			"Shadowlord adjacent to player at (%d, %d) - awaiting encounter handoff",
			enemyPos.X, enemyPos.Y)

		return
	}

	// Pursue the Avatar using greedy movement algorithm
	// This matches the original Ultima V behavior where Shadowlords actively hunt the player
	n.moveEnemyOneTileCloserGreedy(enemy, avatarPos)
}

// moveEnemyOneTileCloserGreedy moves an enemy NPC one tile closer to the target position.
// This is similar to moveOneTileCloserGreedy but works with NPCEnemy instead of NPCFriendly.
// Used for Shadowlord pursuit of the player.
func (n *NPCAIControllerSmallMap) moveEnemyOneTileCloserGreedy(
	enemy *map_units.NPCEnemy, targetPos references.Position,
) bool {
	currentPos := enemy.Pos()

	// Calculate current distance to target (Manhattan distance)
	currentDist := helpers.AbsInt(int(currentPos.X-targetPos.X)) + helpers.AbsInt(int(currentPos.Y-targetPos.Y))

	// Define possible moves: up, down, left, right
	directions := []references.Position{
		{X: 1, Y: 0},  // East
		{X: 0, Y: -1}, // North
		{X: -1, Y: 0}, // West
		{X: 0, Y: 1},  // South
	}

	// Test each direction to find valid moves that reduce distance
	type validMove struct {
		newPos  references.Position
		newDist int
	}

	validMoves := make([]validMove, 0, len(originalNPCDirections))

	for _, dir := range directions {
		newPos := references.Position{
			X: currentPos.X + dir.X,
			Y: currentPos.Y + dir.Y,
		}

		// Boundary check
		if newPos.X < 0 || newPos.Y < 0 || newPos.X >= references.XSmallMapTiles || newPos.Y >= references.YSmallMapTiles {
			continue
		}

		// Can't occupy same space as Avatar
		if n.mapState.PlayerLocation.Position.Equals(&newPos) {
			continue
		}

		// Original actual steps let Shadowlords bypass terrain admission (ref:NPC-0004),
		// but dynamic objects on the same floor still block movement.
		if existingNPC := n.mapUnits.GetMapUnitAtPositionAndFloorOrNil(newPos, enemy.Floor()); existingNPC != nil {
			continue
		}

		// Calculate distance from this potential position to target
		newDist := helpers.AbsInt(int(newPos.X-targetPos.X)) + helpers.AbsInt(int(newPos.Y-targetPos.Y))

		// Only consider moves that reduce distance (greedy algorithm)
		if newDist < currentDist {
			validMoves = append(validMoves, validMove{
				newPos:  newPos,
				newDist: newDist,
			})
		}
	}

	// No valid moves found
	if len(validMoves) == 0 {
		return false
	}

	// Pick first valid move that reduces distance (greedy)
	chosenMove := validMoves[0]
	enemy.SetPos(chosenMove.newPos)

	return true
}

// AdvanceNextTurnCalcAndMoveNPCs processes one turn of NPC AI behavior.
// This method:
//   - Calculates next moves for all NPCs based on their AI types
//   - Handles pathfinding for NPCs with destinations
//   - Executes movement decisions
//   - Updates the map with new NPC positions
//
// The method is called each game turn to update NPC behavior and movement.
func (n *NPCAIControllerSmallMap) AdvanceNextTurnCalcAndMoveNPCs() {
	n.advanceNextTurnCalcAndMoveNPCs(nil)
}

// AdvanceNextTurnCalcAndMoveNPCsExcluding processes one NPC phase without
// touching the source entity retained by a queued encounter handoff.
func (n *NPCAIControllerSmallMap) AdvanceNextTurnCalcAndMoveNPCsExcluding(excluded map_units.MapUnit) {
	n.advanceNextTurnCalcAndMoveNPCs(excluded)
}

func (n *NPCAIControllerSmallMap) advanceNextTurnCalcAndMoveNPCs(excluded map_units.MapUnit) {
	n.mapState.GetLayeredMapByCurrentLocation().ClearMapUnitTiles()
	n.clearPendingNPCInteractions()

	for _, mapUnit := range n.mapUnits {
		if mapUnit == excluded {
			continue
		}
		// very lazy approach - but making sure every NPC is in correct spot on map
		// for every iteration makes sure next NPC doesn't assign the same tile space
		n.FreshenExistingNPCsOnMap()

		switch npc := mapUnit.(type) {
		case *map_units.NPCFriendly:
			n.calculateNextNPCPosition(npc)
		case *map_units.NPCEnemy:
			n.calculateNextEnemyPosition(npc)
		}
	}
	n.FreshenExistingNPCsOnMap()
}

// FreshenExistingNPCsOnMap updates visible NPC occupancy on the current layer.
func (n *NPCAIControllerSmallMap) FreshenExistingNPCsOnMap() {
	n.mapState.GetLayeredMapByCurrentLocation().ClearMapUnitTiles()
	n.placeNPCsOnLayeredMap()
}

func (n *NPCAIControllerSmallMap) generateNPCs() {
	npcs := make([]map_units.MapUnit, 0)
	// get the correct schedule
	npcsRefs := n.slr.GetNPCReferences()
	location := n.mapState.PlayerLocation.Location

	for nNpc, npcRef := range *npcsRefs {
		if npcRef.IsEmptyNPC() {
			continue
		}

		// Skip dead NPCs (freed prisoners, killed NPCs, etc.)
		// Original removal marks the actor's persistent death bit; ref:CRIME-0004.
		if n.deadChecker != nil && n.deadChecker.IsNPCDead(location, nNpc) {
			continue
		}

		if npcRef.GetNPCType() == references.Vehicle {
			vehicle := map_units.NewNPCFriendlyVehicle(
				npcRef.GetVehicleType(), npcRef)
			vehicle.MapUnitDetails().NPCNum = nNpc
			// Boarding refuses only an active NPC-linked horse with a nonzero talk number; ref:VEH-0002.
			vehicle.GetVehicleDetails().IsOwnedByNPC = npcRef.DialogNumber != 0
			if vehicle.GetVehicleDetails().VehicleType == references.CarpetVehicle {
				// REDUX DEVIATION: Original Get accepts any carpet object (ref:VEH-0004). Redux reserves authored
				// world carpets as scenery; only Lord British's and inventory-deployed carpets are collectible.
				vehicle.GetVehicleDetails().IsOwnedByNPC = location != references.LordBritishsCastle ||
					nNpc != references.LordBritishMagicCarpetNPC
			}
			npcs = append(npcs, vehicle)
		} else {
			friendly := map_units.NewNPCFriendly(npcRef, nNpc)
			if !friendly.IsEmptyMapUnit() {
				npcs = append(npcs, friendly)
			}
		}
	}
	n.mapUnits = npcs
}

func (n *NPCAIControllerSmallMap) placeNPCsOnLayeredMap() {
	layeredMap := n.mapState.GetLayeredMapByCurrentLocation()

	for _, mapUnit := range n.mapUnits {
		if !mapUnit.IsVisible() || n.mapState.PlayerLocation.Floor != mapUnit.Floor() {
			continue
		}

		switch npc := mapUnit.(type) {
		case *map_units.NPCEnemy:
			layeredMap.SetTileByLayer(map_state.MapUnitLayer, npc.PosPtr(), npc.GetKeySpriteIndex())
		case *map_units.NPCFriendly:
			layeredMap.SetTileByLayer(map_state.MapUnitLayer, npc.PosPtr(), npc.GetKeySpriteIndex())
		}
	}
}

func (n *NPCAIControllerSmallMap) calculateNextNPCPosition(friendly *map_units.NPCFriendly) {
	if friendly.GetVehicleDetails().VehicleType == references.HorseVehicle {
		n.calculateNextHorsePosition(friendly)

		return
	}
	if friendly.NPCReference.GetNPCType() == references.Vehicle {
		return
	}

	// Arrival is separate from the desired slot. Record it before wandering
	// away from an anchor and after movement that may reach a destination.
	friendly.RecordScheduleArrival(*n.dateTime)
	defer friendly.RecordScheduleArrival(*n.dateTime)

	if friendly.MapUnitDetails().GetOverriddenAiType() != references.Unset {
		friendly.MapUnitDetails().SetCurrentPath(nil)
		n.performAiMovementOnAssignedPosition(friendly)

		return
	}

	slot := friendly.NPCReference.Schedule.DesiredSlot(*n.dateTime)
	refBehaviour := friendly.NPCReference.Schedule.GetIndividualNPCBehaviourBySlot(slot)
	// Incoming travel retains the prior arrival even after changing actual floor.
	// Returning to that slot still requires floor travel; same-floor wandering
	// remains settled without requiring the actor to stand on its anchor.
	if friendly.ArrivedScheduleSlot() == slot && friendly.Floor() == refBehaviour.Floor {
		friendly.MapUnitDetails().SetCurrentPath(nil)
		if friendly.Floor() == n.mapState.PlayerLocation.Floor {
			n.performOriginalScheduleAction(friendly, refBehaviour)
		}

		return
	}
	// NPC-0002: hidden schedules relocate directly, including a new destination
	// on the same hidden floor. Never try visible-floor A* or unseen wandering.
	observedFloor := n.mapState.PlayerLocation.Floor
	if friendly.Floor() != observedFloor && refBehaviour.Floor != observedFloor {
		friendly.PlaceAtScheduleSlot(slot)

		return
	}
	if n.followCurrentSchedulePath(friendly, refBehaviour) {
		return
	}

	if friendly.Floor() != refBehaviour.Floor {
		if friendly.Floor() == n.mapState.PlayerLocation.Floor {
			n.performAiMovementFromCurrentFloorToDifferentFloor(friendly)
		} else {
			n.performAiMovementFromDifferentFloorToOurFloor(friendly)
		}

		return
	}
	if n.createFreshPathToScheduledLocation(friendly) && n.moveNPCOnCalculatedPath(friendly) {
		return
	}
	if friendly.Floor() == n.mapState.PlayerLocation.Floor {
		// REDUX DEVIATION: NPC-0002: original retries retain pause/shared-search
		// budgets. Redux replans and attempts one bounded recovery step per turn.
		n.randomStepOriginalForPurpose(friendly, refBehaviour.Position, 0, npcMovementSchedule)
	}
}

func (n *NPCAIControllerSmallMap) followCurrentSchedulePath(
	friendly *map_units.NPCFriendly, refBehaviour references.IndividualNPCBehaviour,
) bool {
	if friendly.Floor() == n.mapState.PlayerLocation.Floor && friendly.MapUnitDetails().HasAPathAlreadyCalculated() {
		path := friendly.MapUnitDetails().CurrentPath
		goal := path[len(path)-1]
		validGoal := goal == refBehaviour.Position
		if friendly.Floor() != refBehaviour.Floor {
			// Keep the selected ladder through detours, even if another becomes
			// geometrically closer. It must still lead in the desired direction.
			validGoal = isSmallMapPosition(goal) && references.IsSpecificLadderOrStairs(
				n.npcTerrainAt(goal).Index, references.GetLadderOfStairsType(friendly.Floor(), refBehaviour.Floor))
		}
		// REDUX DEVIATION: Original routing retains active destination/ladder travel (ref:NPC-0002).
		// Redux reclassifies travel from the latest schedule and discards routes
		// to obsolete goals before applying that schedule's terrain permissions.
		if !validGoal {
			friendly.MapUnitDetails().SetCurrentPath(nil)
		} else if n.moveNPCOnCalculatedPath(friendly) {
			return true
		}
	}

	return false
}

//nolint:cyclop // The source action switch is clearer as one literal dispatch table.
func (n *NPCAIControllerSmallMap) performOriginalScheduleAction(
	friendly *map_units.NPCFriendly,
	behavior references.IndividualNPCBehaviour,
) {
	if friendly.IsEmptyMapUnit() || !friendly.IsVisible() || friendly.Floor() != n.mapState.PlayerLocation.Floor {
		return
	}
	avatar := n.mapState.PlayerLocation.Position

	switch behavior.Ai {
	case references.Unset:
		panic("unset original NPC schedule action")
	case references.Fixed:
	case references.Wander3:
		n.randomStepOriginal(friendly, behavior.Position, originalWanderRadius)
	case references.BigWander:
		n.randomStepOriginal(friendly, behavior.Position, 0)
	case references.ChildRunAway:
		if n.manhattanDistance(friendly.Pos(), avatar) < originalPursuitRange {
			n.attractOriginal(friendly, references.ChildRunAway)
		}
	case references.Talk3:
		if n.manhattanDistance(behavior.Position, avatar) < originalPursuitRange {
			n.attractOriginal(friendly, references.Talk3)
		} else {
			n.randomStepOriginal(friendly, behavior.Position, originalWanderRadius)
		}
	case references.TalkAlways:
		n.attractOriginal(friendly, references.TalkAlways)
	case references.ExtortOrAttackOrFollow:
		if n.manhattanDistance(friendly.Pos(), avatar) < originalPursuitRange {
			n.attractOriginal(friendly, references.ExtortOrAttackOrFollow)
		}
	case references.DrudgeWorthThing:
		n.attractOriginal(friendly, references.DrudgeWorthThing)
	default:
		panic(fmt.Sprintf("unknown original NPC schedule action %d", behavior.Ai))
	}
}

func (n *NPCAIControllerSmallMap) randomStepOriginal(
	friendly *map_units.NPCFriendly,
	anchor references.Position,
	maximumDistance int,
) {
	n.randomStepOriginalForPurpose(friendly, anchor, maximumDistance, npcMovementWander)
}

func (n *NPCAIControllerSmallMap) randomStepOriginalForPurpose(
	friendly *map_units.NPCFriendly,
	anchor references.Position,
	maximumDistance int,
	purpose npcMovementPurpose,
) {
	if n.rngProvider.RandomIntInRange(0, originalRandomGateMaximum)&0x08 == 0 {
		return
	}

	direction := originalNPCDirections[n.rngProvider.RandomIntInRange(0, originalRandomDirectionMaximum)&0x03]
	destination := references.Position{X: friendly.Pos().X + direction.X, Y: friendly.Pos().Y + direction.Y}
	if maximumDistance > 0 && n.manhattanDistance(destination, anchor) > maximumDistance {
		return
	}
	if !n.canOriginalNPCMoveTo(friendly, destination, purpose) {
		return
	}

	friendly.SetPos(destination)
}

func (n *NPCAIControllerSmallMap) attractOriginal(friendly *map_units.NPCFriendly, action references.AiType) {
	avatar := n.mapState.PlayerLocation.Position
	current := friendly.Pos()
	currentDistance := n.manhattanDistance(current, avatar)
	if currentDistance == 1 && action > references.ChildRunAway {
		n.handleOriginalAdjacentAction(friendly, action)

		return
	}

	if action == references.ChildRunAway {
		n.fleeOriginal(friendly, avatar, currentDistance)

		return
	}

	chosen := n.firstOriginalAttractionDestination(friendly, current, avatar, currentDistance)

	if action == references.TalkAlways || action == references.DrudgeWorthThing {
		chosen = n.originalAttackVariation(friendly, current, chosen)
	}

	if chosen == nil {
		return
	}
	friendly.SetPos(*chosen)
}

func (n *NPCAIControllerSmallMap) handleOriginalAdjacentAction(
	friendly *map_units.NPCFriendly,
	action references.AiType,
) {
	switch action {
	case references.Unset:
		panic("unset original adjacent NPC action")
	case references.Fixed:
	case references.Talk3, references.TalkAlways:
		if friendly.NPCReference.DialogNumber > 0 {
			n.setPendingNPCInteraction(friendly, map_units.PendingNPCInteractionTalk)
		}
	case references.ExtortOrAttackOrFollow, references.DrudgeWorthThing:
		monster, combatCapable := friendly.NPCReference.SmallMapCombatMonsterType()
		switch {
		// Original contact handles the Astaroth marker (ref:NPC-0005) before
		// guard arrest or combat. Redux retains that marker as override provenance.
		case friendly.MapUnitDetails().AIOverrideProvenance() == references.AIOverrideProvenanceAutomaticAstaroth:
			n.setPendingNPCInteraction(friendly, map_units.PendingNPCInteractionTalk)
		case monster == references.MonsterGuard:
			n.setPendingNPCInteraction(friendly, map_units.PendingNPCInteractionGuardArrest)
		case combatCapable:
			n.setPendingNPCInteraction(friendly, map_units.PendingNPCInteractionHostileCombat)
		}
	default:
	}
}

func (n *NPCAIControllerSmallMap) firstOriginalAttractionDestination(
	friendly *map_units.NPCFriendly,
	current, avatar references.Position,
	currentDistance int,
) *references.Position {
	for _, direction := range originalNPCDirections {
		destination := references.Position{X: current.X + direction.X, Y: current.Y + direction.Y}
		if n.canOriginalNPCMoveTo(friendly, destination, npcMovementApproach) &&
			n.manhattanDistance(destination, avatar) < currentDistance {
			return &destination
		}
	}

	return nil
}

func (n *NPCAIControllerSmallMap) originalAttackVariation(
	friendly *map_units.NPCFriendly,
	current references.Position,
	chosen *references.Position,
) *references.Position {
	if n.rngProvider.RandomIntInRange(0, originalAttackVariationMaximum) >= originalAttackVariationThreshold {
		return chosen
	}

	foundAlternative := false
	preferred := chosen
	for _, direction := range originalNPCDirections {
		destination := references.Position{X: current.X + direction.X, Y: current.Y + direction.Y}
		if !n.canOriginalNPCMoveTo(friendly, destination, npcMovementApproach) ||
			(preferred != nil && destination == *preferred) {
			continue
		}
		if !foundAlternative {
			chosen = &destination
			foundAlternative = true

			continue
		}
		if n.rngProvider.RandomIntInRange(0, originalAttackVariationMaximum) < originalAttackVariationThreshold {
			chosen = &destination
		}
	}

	return chosen
}

func (n *NPCAIControllerSmallMap) fleeOriginal(
	friendly *map_units.NPCFriendly,
	avatar references.Position,
	currentDistance int,
) {
	var chosen *references.Position
	for _, direction := range originalNPCDirections {
		destination := references.Position{X: friendly.Pos().X + direction.X, Y: friendly.Pos().Y + direction.Y}
		if !n.canOriginalNPCMoveTo(friendly, destination, npcMovementApproach) ||
			n.manhattanDistance(destination, avatar) <= currentDistance {
			continue
		}
		if chosen == nil || n.rngProvider.RandomIntInRange(0, 1) != 0 {
			candidate := destination
			chosen = &candidate
		}
	}
	if chosen == nil {
		return
	}
	friendly.SetPos(*chosen)
}

func (n *NPCAIControllerSmallMap) canOriginalNPCMoveTo(
	friendly *map_units.NPCFriendly,
	destination references.Position,
	purpose npcMovementPurpose,
) bool {
	if friendly.Floor() != n.mapState.PlayerLocation.Floor ||
		!isSmallMapPosition(destination) || n.mapState.PlayerLocation.Position == destination ||
		n.isPositionOccupiedOnFloor(destination, friendly.Floor()) {
		return false
	}

	// Shadowlords bypass terrain, not object occupancy; ref:NPC-0004.
	if monster, ok := friendly.NPCReference.SmallMapCombatMonsterType(); ok && monster == references.MonsterShadowlord {
		return true
	}
	terrain := n.npcTerrainAt(destination)
	behavior := friendly.NPCReference.Schedule.GetIndividualNPCBehaviourByUltimaDate(*n.dateTime)
	// Actual steps allow beaches before the ordinary terrain check, and reject chairs (ref:NPC-0004)
	// unless activity is MOVE (not inactive wandering/attraction or TOLAD).
	if terrain.Index >= indexes.OutsidePath1 && terrain.Index <= indexes.OutsidePath4 {
		return true
	}
	if terrain.IsChair() && (purpose != npcMovementSchedule || behavior.Floor != friendly.Floor()) {
		return false
	}
	// Both randstep and scheduled travel supply a schedule destination to
	// teststep; attract supplies -1. Occupancy and the chair rule still win.
	if purpose != npcMovementApproach && behavior.Floor == friendly.Floor() && behavior.Position == destination {
		return true
	}

	return terrain.IsNPCTerrainPassable()
}

func (n *NPCAIControllerSmallMap) npcTerrainAt(position references.Position) *references.Tile {
	layeredMap := n.mapState.GetLayeredMapByCurrentLocation()

	return layeredMap.GetSmallMapPersistentTerrainTile(&position, n.dateTime.Hour)
}

func (n *NPCAIControllerSmallMap) manhattanDistance(left, right references.Position) int {
	return helpers.AbsInt(int(left.X-right.X)) + helpers.AbsInt(int(left.Y-right.Y))
}

func (n *NPCAIControllerSmallMap) calculateNextHorsePosition(horse *map_units.NPCFriendly) {
	if horse.Floor() != n.mapState.PlayerLocation.Floor || n.rngProvider.RandomIntInRange(0, 1) != 0 {
		return
	}
	if n.isHorseHitched(horse.Pos()) {
		return
	}

	destination := n.randomHorseDestination(horse.Pos())
	if !n.canHorseMoveTo(horse, destination) {
		return
	}

	horse.SetPos(destination)
}

func (n *NPCAIControllerSmallMap) randomHorseDestination(position references.Position) references.Position {
	horizontal := n.rngProvider.RandomIntInRange(0, 1) != 0
	positive := n.rngProvider.RandomIntInRange(0, 1) != 0
	switch {
	case horizontal && positive:
		return position.Right()
	case horizontal:
		return position.Left()
	case positive:
		return position.Down()
	default:
		return position.Up()
	}
}

func (n *NPCAIControllerSmallMap) canHorseMoveTo(
	horse *map_units.NPCFriendly,
	destination references.Position,
) bool {
	if !isSmallMapPosition(destination) {
		return false
	}
	terrain := n.npcTerrainAt(destination)
	if terrain == nil || !terrain.IsHorsePassable() {
		return false
	}
	if horse.Floor() == n.mapState.PlayerLocation.Floor && destination == n.mapState.PlayerLocation.Position {
		return false
	}

	return !n.isPositionOccupiedOnFloor(destination, horse.Floor())
}

func (n *NPCAIControllerSmallMap) isHorseHitched(position references.Position) bool {
	probes := [...]references.Position{position.Down(), position.Right(), position.Up(), position.Left()}
	for _, probe := range probes {
		// REDUX DEVIATION: Original hitching aliases off-map probes to (31,31); ref:NPC-0006.
		// Redux ignores them so unrelated southeast terrain cannot hitch an edge horse.
		if !isSmallMapPosition(probe) {
			continue
		}
		tile := n.mapState.GetLayeredMapByCurrentLocation().GetTileByLayer(map_state.MapLayer, &probe)
		if tile != nil && (tile.Index == indexes.HitchingPost || tile.Index == indexes.WoodFloorShipEdge) {
			return true
		}
	}

	return false
}

func isSmallMapPosition(position references.Position) bool {
	return position.X >= 0 && position.X < references.XSmallMapTiles &&
		position.Y >= 0 && position.Y < references.YSmallMapTiles
}

// Incoming travel enters at a receiving-floor connection, retaining last arrival
// until the actor physically reaches the destination; ref:NPC-0002.
func (n *NPCAIControllerSmallMap) performAiMovementFromDifferentFloorToOurFloor(friendly *map_units.NPCFriendly) bool {
	refBehaviour := friendly.GetIndividualBehaviourByUltimaData(*n.dateTime)
	friendly.MapUnitDetails().SetCurrentPath(nil)
	// Reverse the direction at the receiving end: arriving from above needs up.
	for _, entry := range n.scheduleLadders(refBehaviour.Position, friendly.Floor()) {
		if n.schedulePositionOccupied(friendly, entry) {
			continue
		}
		path := n.findScheduledPath(friendly, entry, refBehaviour.Position)
		if len(path) == 0 {
			continue
		}
		friendly.SetFloor(n.mapState.PlayerLocation.Floor)
		friendly.SetPos(entry)
		friendly.MapUnitDetails().SetCurrentPath(path[1:])

		return true
	}
	// A finite candidate scan per update; no recovery RNG or destination teleport.
	return false
}

// performAiMovementFromCurrentFloorToDifferentFloor From OUR floor to DIFFERENT floor
func (n *NPCAIControllerSmallMap) performAiMovementFromCurrentFloorToDifferentFloor(
	friendly *map_units.NPCFriendly,
) bool {
	refBehaviour := friendly.GetIndividualBehaviourByUltimaData(*n.dateTime)

	currentNpcMapTile := n.npcTerrainAt(friendly.Pos())
	if references.IsSpecificLadderOrStairs(currentNpcMapTile.Index,
		references.GetLadderOfStairsType(friendly.Floor(), refBehaviour.Floor)) {
		// Departure reaches its connection before relocating to the hidden anchor.
		slot := friendly.NPCReference.Schedule.DesiredSlot(*n.dateTime)
		friendly.PlaceAtScheduleSlot(slot)

		return true
	}

	return n.createFreshPathToScheduledLocation(friendly)
}

func (n *NPCAIControllerSmallMap) performAiMovementOnAssignedPosition(friendly *map_units.NPCFriendly) bool {
	refBehaviour := friendly.GetIndividualBehaviourByUltimaData(*n.dateTime)

	nWanderDistance := n.getWanderDistanceByAiType(refBehaviour.Ai)

	switch refBehaviour.Ai {
	case references.Unset:
		panic("unset assigned NPC behavior")
	case references.BlackthornGuardFixed, references.Fixed:
	case references.MerchantBuyingSellingCustom, references.MerchantBuyingSellingWander, references.Wander:
		n.wanderOneTileWithinN(friendly, refBehaviour.Position, nWanderDistance)

		return true
	case references.BigWander, references.BlackthornGuardWander:
		n.wanderOneTileWithinN(friendly, refBehaviour.Position, nWanderDistance)

		return true
	case references.ChildRunAway:
		return true
	case references.MerchantBuyingSelling:
		// Merchants stay in place
		return true
	case references.TalkAlways, references.ExtortOrAttackOrFollow, references.DrudgeWorthThing:
		n.performOriginalScheduleAction(friendly, refBehaviour)

		return true
	case references.FleeFromAvatar:
		n.fleeCivilian(friendly)

		return true
	case references.StoneGargoyleTrigger:
		// if they are within 4 then change their AI to Drudgeworth (follow)
	case references.FixedExceptAttackWhenIsWantedByThePoPo:
		// if avatar is a wanted man/woman - then follow and get close
	case references.CustomAi, references.Begging, references.GenericExtortingGuard,
		references.HalfYourGoldExtortingGuard, references.SmallWanderWantsToChat:
		return n.approachForInteraction(friendly, refBehaviour, nWanderDistance)
	case references.FollowAroundAndBeAnnoyingThenNeverSeeAgain:
		avatarPos := n.mapState.PlayerLocation.Position
		if !friendly.PosPtr().IsNextTo(avatarPos, friendly.Floor(), n.mapState.PlayerLocation.Floor) {
			n.moveOneTileCloserGreedy(friendly, avatarPos, true)
		}

		return true
	default:
		panic(fmt.Sprintf("unknown aiType %d", refBehaviour.Ai))
	}

	return false
}

func (n *NPCAIControllerSmallMap) fleeCivilian(friendly *map_units.NPCFriendly) {
	avatar := n.mapState.PlayerLocation.Position
	if friendly.PosPtr().IsWithinN(&avatar, civilianFleeRange) {
		n.moveOneTileAwayGreedy(friendly, avatar)
	}
}

func (n *NPCAIControllerSmallMap) approachForInteraction(
	friendly *map_units.NPCFriendly, refBehaviour references.IndividualNPCBehaviour, nWanderDistance int,
) bool {
	// These NPCs want to approach Avatar for interaction (dialogue, begging, extortion)
	// CustomAi (4) is used for guards in some locations like Minoc
	// They wander normally, but show interest in getting close to Avatar
	avatarPos := n.mapState.PlayerLocation.Position

	// If already adjacent to Avatar, stay put and request Talk (orthogonal adjacency only).
	if friendly.PosPtr().IsNextTo(avatarPos, friendly.Floor(), n.mapState.PlayerLocation.Floor) {
		// Check if this is orthogonally adjacent (distance==1, not diagonal)
		// Manhattan distance: |x1-x2| + |y1-y2|
		dx := helpers.AbsInt(int(friendly.PosPtr().X - avatarPos.X))
		dy := helpers.AbsInt(int(friendly.PosPtr().Y - avatarPos.Y))
		manhattanDist := dx + dy

		// Original approach dispatches adjacent contact for actions above 3; ref:NPC-0005.
		if manhattanDist == 1 && friendly.NPCReference.DialogNumber > 0 {
			n.setPendingNPCInteraction(friendly, map_units.PendingNPCInteractionTalk)
		}

		return true
	}

	// Clear pending interaction when not adjacent (prevents stale state).
	friendly.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone

	// Actively approach the Avatar when in range; ref:NPC-0003.
	// Original would move EVERY turn toward Avatar when in range, not probabilistically
	// Uses simple greedy algorithm instead of A* for lightweight, natural movement
	if friendly.PosPtr().IsWithinN(&avatarPos, interactionApproachRange) {
		// Use greedy cardinal movement with a safe blocked fallback; ref:NPC-0003.
		// This is lightweight and happens every turn, unlike expensive A* pathfinding
		// Add random variation for AI types that support it (actions 5 and 7 in original)
		addRandomVariation := (refBehaviour.Ai == references.GenericExtortingGuard ||
			refBehaviour.Ai == references.HalfYourGoldExtortingGuard)

		if n.moveOneTileCloserGreedy(friendly, avatarPos, addRandomVariation) {
			return true
		}
		// If greedy movement fails (blocked), fall through to wandering
	}

	// Otherwise just wander normally around their anchor position
	return n.wanderOneTileWithinN(friendly, refBehaviour.Position, nWanderDistance)
}

// setPendingNPCInteraction preserves one pending command and actor identity (ref:NPC-0005):
// each later eligible slot replaces the command selected by an earlier slot.
func (n *NPCAIControllerSmallMap) setPendingNPCInteraction(
	source *map_units.NPCFriendly,
	interaction map_units.PendingNPCInteraction,
) {
	for _, unit := range n.mapUnits {
		if friendly, ok := unit.(*map_units.NPCFriendly); ok {
			details := friendly.MapUnitDetails()
			if details.PendingInteraction != map_units.PendingNPCInteractionNone &&
				details.NPCNum > source.MapUnitDetails().NPCNum {
				return
			}
		}
	}
	n.clearPendingNPCInteractions()
	source.MapUnitDetails().PendingInteraction = interaction
}

func (n *NPCAIControllerSmallMap) clearPendingNPCInteractions() {
	for _, unit := range n.mapUnits {
		if friendly, ok := unit.(*map_units.NPCFriendly); ok {
			friendly.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone
		}
	}
}

func (n *NPCAIControllerSmallMap) moveNPCOnCalculatedPath(friendly *map_units.NPCFriendly) bool {
	if !friendly.MapUnitDetails().HasAPathAlreadyCalculated() {
		return false
	}

	newPos := friendly.MapUnitDetails().DequeueNextPosition()
	if n.manhattanDistance(friendly.Pos(), newPos) == 1 &&
		n.canOriginalNPCMoveTo(friendly, newPos, npcMovementSchedule) {
		friendly.SetPos(newPos)

		return true
	}
	// A blocked step invalidates the remaining route; never skip it next turn.
	friendly.MapUnitDetails().SetCurrentPath(nil)

	return false
}

func (n *NPCAIControllerSmallMap) createFreshPathToScheduledLocation(friendly *map_units.NPCFriendly) bool {
	muDetails := friendly.MapUnitDetails()
	muDetails.SetCurrentPath(nil)
	if friendly.Floor() != n.mapState.PlayerLocation.Floor {
		return false
	}
	npcBehaviour := friendly.NPCReference.Schedule.GetIndividualNPCBehaviourByUltimaDate(*n.dateTime)
	goals := []references.Position{npcBehaviour.Position}
	if npcBehaviour.Floor != friendly.Floor() {
		goals = n.scheduleLadders(friendly.Pos(), npcBehaviour.Floor)
	}
	for _, goal := range goals {
		path := n.findScheduledPath(friendly, friendly.Pos(), goal)
		if len(path) > 1 {
			muDetails.SetCurrentPath(path[1:])

			return true
		}
	}

	return false
}

func (n *NPCAIControllerSmallMap) wanderOneTileWithinN(
	friendly *map_units.NPCFriendly, anchorPos references.Position, withinN int,
) bool {
	// Define possible moves: up, down, left, right
	directions := []references.Position{
		{X: 0, Y: -1}, // Up
		{X: 0, Y: 1},  // Down
		{X: -1, Y: 0}, // Left
		{X: 1, Y: 0},  // Right
	}

	// Shuffle the directions using deterministic RNG
	n.rngProvider.ShuffleDirections(directions)

	muDetails := friendly.MapUnitDetails()

	// Try each direction to find a valid move
	for _, move := range directions {
		newPos := references.Position{
			X: muDetails.Position.X + move.X,
			Y: muDetails.Position.Y + move.Y,
		}
		if !n.canOriginalNPCMoveTo(friendly, newPos, npcMovementWander) {
			continue
		}

		// Check if the new position is within N tiles of the anchorPos
		if helpers.AbsInt(int(newPos.X-anchorPos.X)) <= withinN &&
			helpers.AbsInt(int(newPos.Y-anchorPos.Y)) <= withinN {
			friendly.SetPos(newPos)

			return true
		}
	}
	// If no valid moves are found, stay in the same position
	return false
}

// moveOneTileCloserGreedy uses cardinal approach with a checked fallback; ref:NPC-0003.
// This simple greedy algorithm tests 4 directions and picks the first one that reduces distance to target.
// It takes a single step with no route search, so it is cheap enough to run every turn.
//
// Algorithm:
// 1. Test all 4 directions (up/down/left/right)
// 2. Calculate distance from each potential position to target
// 3. Pick first direction that reduces distance
// 4. Optional: 25% chance to pick a random valid direction instead (adds natural variation)
//
// Returns true if NPC moved, false if no valid move found.
func (n *NPCAIControllerSmallMap) moveOneTileCloserGreedy(
	friendly *map_units.NPCFriendly, targetPos references.Position, addRandomVariation bool,
) bool {
	muDetails := friendly.MapUnitDetails()
	currentPos := muDetails.Position

	// Calculate current distance to target (Manhattan distance)
	currentDist := helpers.AbsInt(int(currentPos.X-targetPos.X)) + helpers.AbsInt(int(currentPos.Y-targetPos.Y))

	// Define possible moves: up, down, left, right (matching original order)
	directions := []references.Position{
		{X: 1, Y: 0},  // East (direction 1 in original)
		{X: 0, Y: -1}, // North (direction 2 in original)
		{X: -1, Y: 0}, // West (direction 3 in original)
		{X: 0, Y: 1},  // South (direction 4 in original)
	}

	// Test each direction to find valid moves that reduce distance
	type validMove struct {
		direction references.Position
		newPos    references.Position
		newDist   int
	}
	validMoves := make([]validMove, 0, len(originalNPCDirections))

	for _, dir := range directions {
		newPos := references.Position{
			X: currentPos.X + dir.X,
			Y: currentPos.Y + dir.Y,
		}

		if !n.canOriginalNPCMoveTo(friendly, newPos, npcMovementApproach) {
			continue
		}

		// Calculate distance from this potential position to target
		newDist := helpers.AbsInt(int(newPos.X-targetPos.X)) + helpers.AbsInt(int(newPos.Y-targetPos.Y))

		// Only consider moves that reduce distance (greedy algorithm)
		if newDist < currentDist {
			validMoves = append(validMoves, validMove{
				direction: dir,
				newPos:    newPos,
				newDist:   newDist,
			})
		}
	}

	// REDUX DEVIATION: NPC-0003: the original fallback past its four candidates reads outside
	// its distance array, so the outcome is undefined. Redux stays put (the caller then
	// wanders) rather than reproduce that out-of-bounds read. See ref:NPC-0003.
	if len(validMoves) == 0 {
		return false
	}

	// Original Ultima V behavior: 25% chance to move randomly among valid directions
	// Actions 5 and 7 vary direction when an inclusive 0..63 draw is below 16; ref:NPC-0003.
	var chosenMove validMove
	if addRandomVariation && n.rngProvider.OneInXOdds(variationOdds) {
		// Pick random valid move (25% chance)
		randomIndex := 0
		for i := 0; i < len(validMoves) && n.rngProvider.OneInXOdds(replacementOdds); i++ {
			randomIndex = i
		}
		if randomIndex >= len(validMoves) {
			randomIndex = len(validMoves) - 1
		}
		chosenMove = validMoves[randomIndex]
	} else {
		// Pick first valid move that reduces distance (greedy)
		chosenMove = validMoves[0]
	}

	// Execute the move
	friendly.SetPos(chosenMove.newPos)

	return true
}

// moveOneTileAwayGreedy moves an NPC one tile away from the target position (fleeing behavior).
// This is the reverse of moveOneTileCloserGreedy - it selects moves that INCREASE distance.
// Used for civilians fleeing from Avatar when guards are called.
func (n *NPCAIControllerSmallMap) moveOneTileAwayGreedy(
	friendly *map_units.NPCFriendly, targetPos references.Position,
) bool {
	muDetails := friendly.MapUnitDetails()
	currentPos := muDetails.Position

	// Calculate current distance to target (Manhattan distance)
	currentDist := helpers.AbsInt(int(currentPos.X-targetPos.X)) + helpers.AbsInt(int(currentPos.Y-targetPos.Y))

	// Define possible moves: up, down, left, right
	directions := []references.Position{
		{X: 1, Y: 0},  // East
		{X: 0, Y: -1}, // North
		{X: -1, Y: 0}, // West
		{X: 0, Y: 1},  // South
	}

	// Test each direction to find valid moves that INCREASE distance (fleeing)
	type validMove struct {
		direction references.Position
		newPos    references.Position
		newDist   int
	}
	validMoves := make([]validMove, 0, len(originalNPCDirections))

	for _, dir := range directions {
		newPos := references.Position{
			X: currentPos.X + dir.X,
			Y: currentPos.Y + dir.Y,
		}

		if !n.canOriginalNPCMoveTo(friendly, newPos, npcMovementApproach) {
			continue
		}

		// Calculate distance from this potential position to target
		newDist := helpers.AbsInt(int(newPos.X-targetPos.X)) + helpers.AbsInt(int(newPos.Y-targetPos.Y))

		// Only consider moves that INCREASE distance (reverse greedy - fleeing)
		if newDist > currentDist {
			validMoves = append(validMoves, validMove{
				direction: dir,
				newPos:    newPos,
				newDist:   newDist,
			})
		}
	}

	// No valid moves found
	if len(validMoves) == 0 {
		return false
	}

	// Pick first valid move that increases distance (simple greedy fleeing)
	chosenMove := validMoves[0]

	// Execute the move
	friendly.SetPos(chosenMove.newPos)

	return true
}

func (n *NPCAIControllerSmallMap) isPositionOccupiedOnFloor(
	position references.Position,
	floor references.FloorNumber,
) bool {
	return n.mapUnits.GetMapUnitAtPositionAndFloorOrNil(position, floor) != nil
}

func (n *NPCAIControllerSmallMap) getWanderDistanceByAiType(aiType references.AiType) int {
	//nolint:exhaustive // Only wandering behaviors receive a nonzero radius.
	switch aiType {
	case references.Wander:
		return smallWanderDistance
	case references.BigWander, references.BlackthornGuardWander,
		references.MerchantBuyingSellingCustom, references.MerchantBuyingSellingWander:
		return largeWanderDistance
	}

	return 0
}

// RemoveAllEnemies leaves the authored small-map roster intact.
func (n *NPCAIControllerSmallMap) RemoveAllEnemies() {
	// noop
}

// HardResetNPCs performs a complete NPC reset during floor transitions.
// This matches the original Ultima V behavior where climbing stairs/ladders triggers
// initnpcs() which repositions all NPCs to their scheduled locations based on current time.
//
// If preserveOverriddenAI is true, the overridden AI types are preserved (e.g., during Shadowlord encounters).
func (n *NPCAIControllerSmallMap) HardResetNPCs(preserveOverriddenAI bool) {
	// Walk through each existing NPC and reset their position to current scheduled location
	for _, npc := range n.mapUnits {
		if npc.IsEmptyMapUnit() || !npc.IsVisible() {
			continue
		}

		switch mapUnit := npc.(type) {
		case *map_units.NPCFriendly:
			// Get their current scheduled position based on game time
			slot := mapUnit.NPCReference.Schedule.DesiredSlot(*n.dateTime)
			// Set their x, y, floor to that scheduled position
			mapUnit.PlaceAtScheduleSlot(slot)

			// Clear their movement state (path)
			mapUnit.MapUnitDetails().SetCurrentPath(make([]references.Position, 0))

			// Only clear overridden AI if not preserving (e.g., Shadowlord effects should persist)
			if !preserveOverriddenAI {
				mapUnit.MapUnitDetails().SetOverriddenAiType(references.Unset)
			}
		case *map_units.NPCEnemy:
			// Enemy NPCs not supported on small maps - skip
		}
	}

	// Refresh the map placement
	n.placeNPCsOnLayeredMap()
}
