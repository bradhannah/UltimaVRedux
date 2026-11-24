package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Enhanced climbing mechanics constants
const (
	climbingFallWorkflowName = "climbing_fall_effect"

	// Every climbing fall flashes gray, shakes the screen and plays white noise.
	fallShakeDurationMs = 300
	fallShakePixels     = 10.0
)

// Klimb floor transition timing constants (2x faster than Enter transitions)
const (
	KlimbFadeOutMs = 250 // Fade to black duration (2x faster than Enter's 500ms)
	KlimbFadeInMs  = 250 // Fade from black duration (2x faster than Enter's 500ms)
)

// SmallMapKlimbStartOutcome describes whether town Klimb resolves immediately
// or needs an adjacent target direction.
type SmallMapKlimbStartOutcome uint8

const (
	// SmallMapKlimbRejected means the command was refused before directional input.
	SmallMapKlimbRejected SmallMapKlimbStartOutcome = iota
	// SmallMapKlimbNeedsDirection requests one adjacent town target.
	SmallMapKlimbNeedsDirection
	// SmallMapKlimbCompleted means an underfoot floor transition started.
	SmallMapKlimbCompleted
)

// BeginSmallMapKlimb handles source-ordered rider rejection and underfoot exits
// without using an invalid direction as a prompting probe.
func (g *GameState) BeginSmallMapKlimb() SmallMapKlimbStartOutcome {
	if g.PartyVehicle.GetVehicleDetails().VehicleType == references.HorseVehicle {
		g.SystemCallbacks.Message.AddRowStr("On foot!")

		return SmallMapKlimbRejected
	}
	if present, completed := g.actionKlimbSmallMapUnderfoot(); present {
		if completed {
			return SmallMapKlimbCompleted
		}

		return SmallMapKlimbRejected
	}

	return SmallMapKlimbNeedsDirection
}

func (g *GameState) actionKlimbSmallMapUnderfoot() (bool, bool) {
	// Check current position for ladder/grate first (player stands on these)
	currentTile := g.MapState.LayeredMaps.GetTileRefByPosition(
		references.SmallMapType,
		map_state.MapLayer,
		&g.MapState.PlayerLocation.Position,
		g.MapState.PlayerLocation.Floor)

	//nolint:exhaustive // Only ladder tiles are actionable here.
	switch currentTile.Index {
	case indexes.LadderDown, indexes.Grate:
		if g.CanGoDownOneFloor() {
			g.StartKlimbFloorWorkflow(false) // false = going down

			return true, true
		}

		g.SystemCallbacks.Message.AddRowStr("Can't go lower!")

		return true, false

	case indexes.LadderUp:
		if g.CanGoUpOneFloor() {
			g.StartKlimbFloorWorkflow(true) // true = going up

			return true, true
		}

		g.SystemCallbacks.Message.AddRowStr("Can't go higher!")

		return true, false
	}

	return false, false
}

// ActionKlimbSmallMap commits a directional town climb or an underfoot transition.
func (g *GameState) ActionKlimbSmallMap(direction references.Direction) bool {
	if g.PartyVehicle.GetVehicleDetails().VehicleType == references.HorseVehicle {
		g.SystemCallbacks.Message.AddRowStr("On foot!")

		return false
	}
	if present, completed := g.actionKlimbSmallMapUnderfoot(); present {
		return completed
	}

	// Check target position for source climbable town terrain.
	newPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if newPosition == nil {
		// Invalid direction or out of bounds
		g.SystemCallbacks.Message.AddRowStr("What?")

		return false
	}
	targetTile := g.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, g.MapState.PlayerLocation.Floor).GetTileTopMapOnlyTile(newPosition)
	if targetTile != nil && (targetTile.Is(indexes.SmallRockWall) || targetTile.Is(indexes.FenceHoriz) || targetTile.Is(indexes.FenceVert)) {
		g.MapState.PlayerLocation.Position = *newPosition
		g.SystemCallbacks.UI.ShowActionFeedback("Klimb!", true)
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true
	}

	g.SystemCallbacks.Message.AddRowStr("What?")

	return false
}

// ActionKlimbLargeMap attempts a grapple climb and resolves any party falls.
func (g *GameState) ActionKlimbLargeMap(direction references.Direction) bool {
	// Large map Klimb = mountain climbing with grapple (overworld only)
	if g.EventQueueManager.HasWorkflow(climbingFallWorkflowName) {
		return false
	}

	// Check if player has grapple
	grappleQuantity := g.PartyState.Inventory.QuestItems.GetQuantity(references.Grapple)
	if grappleQuantity == nil || !(*grappleQuantity).HasSome() {
		g.SystemCallbacks.UI.ShowActionFeedback("With what?", true)

		return false
	}

	// Check if player is on foot (no vehicles)
	if g.PartyVehicle.GetVehicleDetails().VehicleType != references.NoPartyVehicle {
		g.SystemCallbacks.Message.AddRowStr("On foot!")

		return false
	}

	// Get target position and tile
	newPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if newPosition == nil {
		g.SystemCallbacks.Message.AddRowStr("Not climbable!")

		return false
	}
	newPosition.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
	targetTile := g.MapState.LayeredMaps.GetLayeredMap(references.LargeMapType, g.MapState.PlayerLocation.Floor).GetTopTile(newPosition)

	// Reject impassable peaks before other climbing checks. ref:ENV-0003
	if targetTile.Is(indexes.TallMountains) {
		g.SystemCallbacks.Message.AddRowStr("Impassable!")

		return false
	}

	// Require climbable mountain terrain at the target. ref:ENV-0003
	if !targetTile.Is(indexes.SmallMountains) {
		g.SystemCallbacks.Message.AddRowStr("Not climbable!")

		return false
	}

	// Enhanced climbing mechanics with staged audio-visual effects
	injuredMembers := g.performClimbingDexterityChecks()

	// IMMEDIATE movement - player reaches destination regardless of falls
	g.MovePlayerOneStep(direction)

	if len(injuredMembers) == 0 {
		// Successful climbing - immediate feedback
		g.SystemCallbacks.Audio.Play(SoundClimbingSuccess).Start()
		g.SystemCallbacks.UI.ShowActionFeedback("Klimb!", true)
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true
	}

	// Failed climbing - start staged damage workflow
	g.StartClimbingFallWorkflow(injuredMembers)

	return true
}

// CombatKlimbStartOutcome describes whether combat Klimb resolves immediately
// or needs one of its two original input contracts.
type CombatKlimbStartOutcome uint8

// Combat Klimb start outcomes keep ordinary direction and dual-ladder input distinct.
const (
	CombatKlimbRejected CombatKlimbStartOutcome = iota
	CombatKlimbNeedsDirection
	CombatKlimbNeedsLadderDirection
	CombatKlimbCompleted
)

// CombatKlimbTargetKind distinguishes ordinary adjacent-rock input from a
// selected ladder exit attempt.
type CombatKlimbTargetKind uint8

const (
	// CombatKlimbRockTarget retains ordinary adjacent-rock direction input.
	CombatKlimbRockTarget CombatKlimbTargetKind = iota
	// CombatKlimbLadderTarget identifies a selected vertical exit attempt.
	CombatKlimbLadderTarget
)

// BeginCombatKlimb resolves single-route ladders immediately and leaves the
// scene to collect either ordinary rock direction or dedicated dual-ladder input.
func (g *GameState) BeginCombatKlimb() CombatKlimbStartOutcome {
	if g.CombatScene == nil {
		return CombatKlimbRejected
	}
	identity := g.CombatScene.GetCurrentTurnIdentity()
	if !identity.IsPartyMember || g.CombatScene.isCombatPCUnderground(identity) {
		return CombatKlimbRejected
	}
	position := g.CombatScene.GetCombatantPosition(identity)
	tile := g.MapState.LayeredMaps.GetTileRefByPosition(
		references.CombatMapType,
		map_state.MapLayer,
		&position,
		0,
	)
	if tile.Index == indexes.LadderUp && g.CombatScene.CombatMapRef.IsDungeonDualLadder(position) {
		return CombatKlimbNeedsLadderDirection
	}
	if tile.Index != indexes.LadderUp && tile.Index != indexes.LadderDown &&
		(tile.Index != indexes.Grate || g.CombatScene.ExitContext != CombatExitDungeonRoom) {
		return CombatKlimbNeedsDirection
	}
	if !g.ActionKlimbCombatMap(references.Direction(0)) {
		return CombatKlimbRejected
	}

	return CombatKlimbCompleted
}

// ActionKlimbCombatMap attempts a ladder exit or adjacent-rock climb for the active combatant.
func (g *GameState) ActionKlimbCombatMap(direction references.Direction) bool {
	if g.CombatScene == nil {
		return false
	}
	identity := g.CombatScene.GetCurrentTurnIdentity()
	if !identity.IsPartyMember || g.CombatScene.isCombatPCUnderground(identity) {
		return false
	}
	combatantIdx := identity.EntityIndex
	combatantPos := g.CombatScene.GetCombatantPosition(identity)
	currentTile := g.MapState.LayeredMaps.GetTileRefByPosition(
		references.CombatMapType,
		map_state.MapLayer,
		&combatantPos,
		0,
	)
	var route CombatExitRoute
	var message string
	//nolint:exhaustive // Only combat ladder tiles establish exit routes.
	switch currentTile.Index {
	case indexes.LadderUp:
		if g.CombatScene.CombatMapRef.IsDungeonDualLadder(combatantPos) {
			switch direction {
			case references.Up:
				route = CombatExitUp
				message = "Klimb-Up!"
			case references.Down:
				route = CombatExitDown
				message = "Klimb-Down!"
			default:
				g.SystemCallbacks.Message.AddRowStr("What?")

				return false
			}
		} else {
			route = CombatExitUp
			message = "Klimb-Up!"
		}
	case indexes.LadderDown:
		route = CombatExitDown
		message = "Klimb-Down!"
	case indexes.Grate:
		if g.CombatScene.ExitContext != CombatExitDungeonRoom {
			g.SystemCallbacks.Message.AddRowStr("What?")

			return false
		}
		route = CombatExitDown
		message = "Klimb-Down!"
	default:
		target := direction.GetNewPositionInDirection(&combatantPos)
		if target == nil || target.X < 0 || target.X >= references.XCombatMapTiles ||
			target.Y < 0 || target.Y >= references.YCombatMapTiles {
			g.SystemCallbacks.Message.AddRowStr("What?")

			return false
		}
		targetTile := g.MapState.LayeredMaps.GetTileRefByPosition(
			references.CombatMapType,
			map_state.MapLayer,
			target,
			0,
		)
		if targetTile.Index != indexes.SmallRockWall ||
			g.CombatScene.isCombatPCAtPositionExcept(*target, identity) ||
			g.CombatScene.MonsterManager.IsPositionOccupied(*target) ||
			g.CombatScene.HasObjectAt(*target) || g.CombatScene.hasFieldAt(*target) {
			g.SystemCallbacks.Message.AddRowStr("What?")

			return false
		}

		g.CombatScene.setCombatantPosition(identity, *target)
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind: CombatEventMove,
			Attacker: CombatFeedbackEntity{
				IsPartyMember: true,
				Index:         combatantIdx,
				Slot:          identity.Slot,
				Position:      *target,
			},
			From: combatantPos,
			To:   *target,
		})
		g.CheckTrap(target)

		return true
	}
	if !g.fleePCFromCombat(identity, route) {
		return false
	}
	g.SystemCallbacks.Message.AddRowStr(message)

	return true
}

// CombatKlimbTargetKind reports the active combatant's retained Klimb contract.
func (g *GameState) CombatKlimbTargetKind() CombatKlimbTargetKind {
	if g.CombatScene == nil {
		return CombatKlimbRockTarget
	}
	identity := g.CombatScene.GetCurrentTurnIdentity()
	position := g.CombatScene.GetCombatantPosition(identity)
	tile := g.MapState.LayeredMaps.GetTileRefByPosition(
		references.CombatMapType,
		map_state.MapLayer,
		&position,
		0,
	)
	if tile.Index == indexes.LadderUp || tile.Index == indexes.LadderDown || tile.Index == indexes.Grate {
		return CombatKlimbLadderTarget
	}

	return CombatKlimbRockTarget
}

// Floor transition helper methods - abstracted for reuse across different systems
// These functions handle the complete floor change workflow including validation and map updates

// CanGoUpOneFloor checks if the player can ascend one floor level
func (g *GameState) CanGoUpOneFloor() bool {
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType:
		locationRef := g.GameReferences.LocationReferences.GetLocationReference(g.MapState.PlayerLocation.Location)

		return locationRef.CanGoUpOneFloor(g.MapState.PlayerLocation.Floor)
	case references.CombatMapType, references.DungeonMapType:
		// Combat and dungeon floor bounds are not validated by this helper.
		return true
	case references.LargeMapType, references.CampingMapType:
		return false
	default:
		return false
	}
}

// CanGoDownOneFloor checks if the player can descend one floor level
func (g *GameState) CanGoDownOneFloor() bool {
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType:
		locationRef := g.GameReferences.LocationReferences.GetLocationReference(g.MapState.PlayerLocation.Location)

		return locationRef.CanGoDownOneFloor(g.MapState.PlayerLocation.Floor)
	case references.CombatMapType, references.DungeonMapType:
		// Combat and dungeon floor bounds are not validated by this helper.
		return true
	case references.LargeMapType, references.CampingMapType:
		return false
	default:
		return false
	}
}

// GoUpOneFloor executes a complete floor ascent including map updates
func (g *GameState) GoUpOneFloor() bool {
	if !g.CanGoUpOneFloor() {
		return false
	}
	g.changeFloorBy(1)

	return true
}

// GoDownOneFloor executes a complete floor descent including map updates
func (g *GameState) GoDownOneFloor() bool {
	if !g.CanGoDownOneFloor() {
		return false
	}
	g.changeFloorBy(-1)

	return true
}

// changeFloorBy applies a ladder, stair or trapdoor floor change, including the
// floor-change rules TransitionToLocation applies for TransitionFloorOnly.
func (g *GameState) changeFloorBy(delta references.FloorNumber) {
	floor := g.MapState.PlayerLocation.Floor + delta
	g.clearShadowlordIfLeavingKeep(g.MapState.PlayerLocation.Location, floor, TransitionFloorOnly)
	g.resetHarpsichordMelody()
	g.discardSurfaceChestLootOnDeparture()
	g.ItemStacksMap = *references.NewItemStacksMap()
	g.MapState.PlayerLocation.Floor = floor

	// Update map state based on map type
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType:
		// Perform hard NPC reset to match original Ultima V behavior
		// Original: town floor travel -> terrain reconstruction -> scheduled NPC placement
		// See ref:CMD-0005.
		g.hardResetSmallMapAfterFloorChange()
	case references.LargeMapType:
		// Shards are drawn as overlays, so large maps need no floor refresh here.
	case references.CombatMapType, references.DungeonMapType, references.CampingMapType:
		// This helper changes the floor without rebuilding these map types.
	}
	g.restoreSurfaceCorpsesForCurrentMap()
	g.reconcileBasementChestsOnEntry()
}

// hardResetSmallMapAfterFloorChange reloads terrain and places NPCs at
// their current scheduled destinations while retaining the existing controller.
// Current dialogue state, hostility, fear and Shadowlord overrides survive
// this same-location floor transition. ref:NPC-0002
func (g *GameState) hardResetSmallMapAfterFloorChange() {
	// Step 1: Update the layered map for the new floor
	// This handles floor-specific terrain without recreating the NPC controller
	slr := g.GameReferences.LocationReferences.GetLocationReference(g.MapState.PlayerLocation.Location)
	g.resetAndCreateSmallMap(slr, g.GameReferences.TileReferences)
	// Terrain reconstruction applies vegetation effects once to fresh source terrain,
	// before scheduled NPC placement. Floor travel retains presence even after midnight moves
	// town assignments; it must not reroll influence or reinitialize town entry.
	// See ref:CMD-0005.
	if g.ShadowlordState.HasAutomaticPresence() {
		g.ApplyShadowlordMapEffects(g.ShadowlordState.AutomaticPresence)
	}

	// Step 2: Hard reset NPCs to scheduled positions
	// Original: scheduled NPC placement repositions all NPCs to current scheduled locations
	// Original scheduled NPC placement does not reset loaded actor dialogue state, so current-map hostility,
	// fleeing, and Shadowlord overrides survive floor repositioning.
	// See ref:CMD-0005.
	g.CurrentNPCAIController.HardResetNPCs(true)

	// Step 3: Update lighthouse lighting for current floor
	// Lighthouse beacons only light tiles on the same floor as the beacon
	g.UpdateLighthouseForSmallMap()
}

// StartKlimbFloorWorkflow creates event queue for floor transitions with rapid fade effect
// Sequence: fade out → change floor → fade in → advance time
// This provides visual feedback for floor changes without instant jarring transitions
func (g *GameState) StartKlimbFloorWorkflow(goUp bool) {
	routeOwnsCycle := g.surfaceCycleOwned
	if g.EventQueueManager == nil {
		panic("GameState.StartKlimbFloorWorkflow: EventQueueManager is nil")
	}

	if g.SystemCallbacks == nil {
		panic("GameState.StartKlimbFloorWorkflow: SystemCallbacks is nil")
	}

	// Check if klimb sequence is already running
	if g.EventQueueManager.HasWorkflow("klimb_floor") {
		return
	}

	// Determine message based on direction
	message := "Klimb-Down!"
	if goUp {
		message = "Klimb-Up!"
	}

	g.EventQueueManager.Workflow("klimb_floor").
		Priority(event_queue.PriorityCritical).
		Sequential().BlockInput().
		First(func() {
			g.SystemCallbacks.UI.ShowActionFeedback(message, true)
		}).
		Animate(KlimbFadeOutMs, func(progress float32) {
			g.SystemCallbacks.Visual.SetMapFadeLevel(float64(progress))
		}).
		Then(func() {
			if goUp {
				g.GoUpOneFloor()
			} else {
				g.GoDownOneFloor()
			}
		}).
		Animate(KlimbFadeInMs, func(progress float32) {
			g.SystemCallbacks.Visual.SetMapFadeLevel(1.0 - float64(progress))
		}).
		Then(func() {
			if !routeOwnsCycle {
				g.AdvanceGameplayMinutes(1)
			}
		}).Finally(

		func() {
			g.SystemCallbacks.Visual.SetMapFadeLevel(0.0)
			if !routeOwnsCycle {
				g.ProcessPendingProvisionHours()
			}
		}).
		Start()
}

// performClimbingDexterityChecks performs dexterity checks for all living party members
// Returns indices of members who failed their checks and will be injured
func (g *GameState) performClimbingDexterityChecks() []int {
	var injuredMembers []int

	g.PartyState.ForEachCharacter(func(i int, member *party_state.PlayerCharacter) {
		if member.IsDead() {
			return // Skip dead members
		}

		// Dexterity check: member dex vs random(1, 30)
		memberDex := int(member.Dexterity)
		randomRoll := g.RandomIntInRange(1, 30)

		if memberDex < randomRoll {
			injuredMembers = append(injuredMembers, i)
		}
	})

	return injuredMembers
}

// StartClimbingFallWorkflow reports a climbing fall, damages and flashes the
// injured party members with a screen shake, and advances one minute unless
// the surrounding route owns the turn cycle. A fall already in progress is kept.
func (g *GameState) StartClimbingFallWorkflow(injuredMembers []int) {
	routeOwnsCycle := g.surfaceCycleOwned
	if g.EventQueueManager == nil {
		panic("GameState.StartClimbingFallWorkflow: EventQueueManager is nil - critical system component missing")
	}

	if g.SystemCallbacks == nil {
		panic("GameState.StartClimbingFallWorkflow: SystemCallbacks is nil - critical system component missing")
	}
	if g.EventQueueManager.HasWorkflow(climbingFallWorkflowName) {
		return
	}

	builder := NewPartyStateBuilder(g).
		WithRandomDamage(1, 5).       // 1-5 HP damage (climbing falls)
		ToCharacters(injuredMembers). // Only injured members
		FlashGray().
		WithShake(fallShakeDurationMs, fallShakePixels)

	// Build complete event queue
	g.EventQueueManager.Workflow(climbingFallWorkflowName).
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {
			g.SystemCallbacks.Message.AddRowStr("Fell!")
			// REDUX DEVIATION: original outdoor command rules mountain climbing uses generic damage noise.
			// Redux identifies the lower-impact mountain fall semantically.
			// See ref:CMD-0005.
			g.SystemCallbacks.Audio.Play(SoundFallingMinor).Start()
		}).ThenSequence(

		builder.Build()).
		Then(func() {
			g.SystemCallbacks.Audio.Play(SoundWhiteNoiseDamage).Start()
		}).

		// Phase 4: Advance game time
		Then(func() {
			if !routeOwnsCycle {
				g.AdvanceGameplayMinutes(1)
			}
		}).Finally(func() {
		if !routeOwnsCycle {
			g.ProcessPendingProvisionHours()
		}
	}).
		Start()
}
