package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// MapTransitionType defines the scope of a map transition
type MapTransitionType int

// TransitionResult contains information needed by caller for post-transition effects.
// Used to pass Shadowlord presence data back to the event queue workflow without
// creating nested event queues.
type TransitionResult struct {
	// ShadowlordsPresent contains any Shadowlords present at the new location.
	// Empty if none. Handles both towns (single lord) and Stonegate (multiple lords).
	ShadowlordsPresent []references.Shadowlord
}

const (
	// TransitionFullLocation - Complete location change (town entry, teleport, whirlpool)
	// Updates: Location, Floor, Position, AI Controller, Light Cache, Background Music
	TransitionFullLocation MapTransitionType = iota

	// TransitionFloorOnly - Floor change within same location (klimb stairs/ladders)
	// Updates: Floor, conditional map refresh (NO light cache, NO music change)
	TransitionFloorOnly

	// TransitionPositionOnly - Position change only (movement, push)
	// Updates: Position only (NO other updates)
	TransitionPositionOnly
)

// TransitionToLocation handles all map transition logic in one centralized place
// Eliminates duplicate code across action_exit, action_enter, whirlpool, debug commands
//
// Parameters:
//   - location: New location (use current location if transitionType is TransitionFloorOnly or TransitionPositionOnly)
//   - floor: New floor number
//   - position: New position coordinates
//   - transitionType: Scope of transition (full location, floor-only, or position-only)
//   - saveLastLargeMap: If true, saves current large map position/floor before transition (for town entry)
//
// Usage Examples:
//
//	// Whirlpool teleport to underworld
//	gs.TransitionToLocation(references.BritanniaUnderworld, -1, references.Position{X: 34, Y: 18}, TransitionFullLocation, false)
//
//	// Exit town back to overworld
//	gs.TransitionToLocation(references.BritanniaUnderworld, gs.LastLargeMapFloor, gs.LastLargeMapPosition, TransitionFullLocation, false)
//
//	// Enter town from overworld
//	gs.TransitionToLocation(references.Britain, 0, references.Position{X: 10, Y: 10}, TransitionFullLocation, true)
func (g *GameState) TransitionToLocation(
	location references.Location,
	floor references.FloorNumber,
	position references.Position,
	transitionType MapTransitionType,
	saveLastLargeMap bool,
) TransitionResult {
	result := TransitionResult{}
	previousLocation := g.MapState.PlayerLocation.Location
	previousFloor := g.MapState.PlayerLocation.Floor
	if g.activeDungeonCommand == nil && g.CombatScene == nil &&
		g.activeDungeon.hasPendingRoomContinuation() {
		panic("cannot transition maps while a dungeon room continuation is pending")
	}
	if transitionType != TransitionPositionOnly && location.GetMapType() != references.DungeonMapType {
		if g.activeDungeonCommand != nil {
			panic("cannot leave dungeon during an active dungeon command cycle")
		}
		g.activeDungeon = nil
	}
	if transitionType != TransitionPositionOnly &&
		(g.MapState.PlayerLocation.Location != location || g.MapState.PlayerLocation.Floor != floor) {
		g.resetHarpsichordMelody()
	}
	if transitionType != TransitionPositionOnly &&
		(g.MapState.PlayerLocation.Location != location || g.MapState.PlayerLocation.Floor != floor) {
		g.ItemStacksMap = *references.NewItemStacksMap()
		g.discardSurfaceChestLootOnDeparture()
	}
	// Save previous large map state if requested (for town entry)
	// Allows player to return to exact position when exiting town
	if saveLastLargeMap && g.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType {
		g.LastLargeMapPosition = g.MapState.PlayerLocation.Position
		g.LastLargeMapFloor = g.MapState.PlayerLocation.Floor
	}

	// Clear Shadowlord summoned state when leaving a Keep or changing floors
	// The Shadowlord spawns at a specific position on a specific floor - player must stay there
	g.clearShadowlordIfLeavingKeep(location, floor, transitionType)

	// Update player location based on transition type
	switch transitionType {
	case TransitionFullLocation:
		g.MapState.CloseOpenDoor()
		g.MapState.LayeredMaps.DiscardDepartedSmallMap(location)
		if location != previousLocation && location.GetMapType() == references.SmallMapType {
			// Original town rules initializes local ride/turn on each new townloop visit.
			// See ref:NPC-0002.
			g.townMountedActorSkip = false
			g.townQuicknessActorSkip = false
		}
		// Complete location change - update everything
		g.MapState.PlayerLocation.Location = location
		g.MapState.PlayerLocation.Floor = floor
		g.MapState.PlayerLocation.Position = position

		// Update map system (calls UpdateLargeMap or UpdateSmallMap)
		g.updateMapAfterFullTransition()
		g.InitializeOutdoorWindow()
		if previousLocation != location || previousFloor != floor {
			g.MapState.ResetTransientDoor()
		}

		// Small-map authority is installed by UpdateSmallMap. Dungeons have no
		// world-map NPC authority and must not retain the source controller.
		switch g.MapState.PlayerLocation.Location.GetMapType() {
		case references.LargeMapType:
			g.CurrentNPCAIController = g.GetCurrentLargeMapNPCAIController()
		case references.SmallMapType:
			// UpdateSmallMap installed the destination controller above.
		case references.DungeonMapType:
			g.CurrentNPCAIController = nil
		case references.CombatMapType, references.CampingMapType:
			// These map types retain their existing scene-owned authority behavior.
		}

		// Collect Shadowlord presence data for caller to handle UI effects
		// This avoids nested event queues - caller uses ThenSequence to chain effects
		// See ref:NPC-0002.
		result.ShadowlordsPresent = g.collectShadowlordPresenceData(location, position)

		// Invalidate light cache (location changed - different ambient light, light sources)
		if currentMap := g.GetCurrentLayeredMap(); currentMap != nil {
			currentMap.InvalidateLightSourceCache()
		}

		// Update background music (new location has different music theme)
		g.SystemCallbacks.Audio.UpdateLocationMusic()
		if previousLocation != location {
			g.OnMapTransition()
		}

	case TransitionFloorOnly:
		// Floor change within same location (klimb stairs/ladders)
		g.MapState.PlayerLocation.Floor = floor

		// Conditional updates based on map type
		// Small maps: Reset NPCs (authentic Ultima V behavior)
		// Large maps: Refresh quest items (shards)
		g.updateMapAfterFloorChange()
		g.InitializeOutdoorWindow()

		// NO light cache invalidation (same location, just different floor)
		// NO music update (same location theme)

	case TransitionPositionOnly:
		// Simple position update (movement, push objects, klimb fences)
		g.MapState.PlayerLocation.Position = position
		// No other updates needed - same location, floor, lighting, music
	}
	if transitionType != TransitionPositionOnly {
		g.restoreSurfaceCorpsesForCurrentMap()
		if previousLocation != location || previousFloor != floor {
			g.reconcileBasementChestsOnEntry()
		}
	}
	g.ActivatePendingBritanniaDockVehicle()
	if g.activeDungeonCommand == nil && g.CombatScene == nil &&
		location.GetMapType() == references.DungeonMapType {
		g.synchronizeDungeonRoomEncounterForStablePosition()
	}
	if transitionType != TransitionPositionOnly &&
		(previousLocation.GetMapType() == references.DungeonMapType || location.GetMapType() == references.DungeonMapType) {
		// Reconcile dungeon-owned remnants after authority replacement, including
		// fresh same-coordinate visits, before another transition can return here.
		g.SystemCallbacks.Screen.UpdateDisplay()
	}

	return result
}

// collectShadowlordPresenceData gathers Shadowlord presence information for a location.
// Returns slice of Shadowlords present (empty if none).
// Handles both:
//   - Stonegate: ALL living Shadowlords (atmospheric presence, no NPCs)
//   - Towns (1-8): Single persistently assigned Shadowlord + applies NPC effects
//
// See ref:NPC-0002.
func (g *GameState) collectShadowlordPresenceData(
	location references.Location,
	position references.Position,
) []references.Shadowlord {
	// Check Stonegate first - ALL living Shadowlords (atmospheric only, no NPC effects)
	if references.IsStonegateLocation(location) {
		lords := g.ShadowlordState.GetLivingShadowlords()
		if len(lords) > 0 {
			logging.Infof(logging.CategoryGameState,
				"collectShadowlordPresenceData: Stonegate - %d living Shadowlords present",
				len(lords))
		}

		return lords
	}

	// Regular towns (1-8): Single persistently assigned Shadowlord
	if !IsShadowlordTown(location) {
		return nil
	}

	// Skip if player entered via jail (y==4)
	// Original: original town rules - "been thrown in jail lately? (entered @ py=4)"
	// See ref:NPC-0002.
	if position.Y == 4 {
		logging.Debugf(logging.CategoryGameState,
			"collectShadowlordPresenceData: skipping - player entered via jail (y=4)")
		g.clearPhysicalShadowlords()
		g.ShadowlordState.ClearAutomaticPresence()

		return nil
	}

	lord := g.GetShadowlordForLocation(location)
	if lord == references.NoShadowlord {
		logging.Debugf(logging.CategoryGameState,
			"collectShadowlordPresenceData: no Shadowlord assigned to %s",
			location.String())

		g.clearPhysicalShadowlords()
		g.ShadowlordState.ClearAutomaticPresence()

		return nil
	}

	logging.Infof(logging.CategoryGameState,
		"collectShadowlordPresenceData: %s is present at %s",
		lord.String(), location.String())

	// The physical encounter is authoritative. Do not mutate automatic state,
	// NPC behavior, map terrain, or presentation data unless insertion succeeds.
	if !g.ensureSingleAutomaticShadowlordNPC(lord, location) {
		logging.Errorf(logging.CategoryGameState,
			"collectShadowlordPresenceData: failed to insert %s at %s", lord.String(), location.String())
		g.ShadowlordState.ClearAutomaticPresence()

		return nil
	}
	g.ShadowlordState.AutomaticPresence = lord

	// Apply non-UI effects for regular towns (NOT Stonegate)
	// Source: Original NPC influence affects NPCs, vegetation effects destroys crops
	// See ref:NPC-0002.
	g.ApplyShadowlordEffectsToNPCs(lord)

	destroyed, total := g.ApplyShadowlordMapEffects(lord)
	if destroyed > 0 {
		logging.Infof(logging.CategoryGameState,
			"collectShadowlordPresenceData: %s destroyed %d/%d crops/trees",
			lord.String(), destroyed, total)
	}

	return []references.Shadowlord{lord}
}

// reinitializeTownShadowlordAfterCombat reproduces the non-presentation portion
// of init_shadow_lords after load_level(FALSE). Existing per-NPC reaction causes
// survive; only assignment, physical presence, and map effects are reevaluated.
func (g *GameState) reinitializeTownShadowlordAfterCombat(
	location references.Location,
	position references.Position,
) {
	if g.ShadowlordState == nil || !IsShadowlordTown(location) || position.Y == 4 {
		g.clearPhysicalShadowlords()
		if g.ShadowlordState != nil {
			g.ShadowlordState.ClearAutomaticPresence()
		}

		return
	}

	lord := g.GetShadowlordForLocation(location)
	if lord == references.NoShadowlord || !g.ensureSingleAutomaticShadowlordNPC(lord, location) {
		g.clearPhysicalShadowlords()
		g.ShadowlordState.ClearAutomaticPresence()

		return
	}

	g.ShadowlordState.AutomaticPresence = lord
	g.ApplyShadowlordMapEffects(lord)
}

// updateMapAfterFullTransition calls appropriate map update method based on current map type
// Handles large map, small map, dungeon, and combat map transitions
func (g *GameState) updateMapAfterFullTransition() {
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.LargeMapType:
		g.MapState.UpdateLargeMap()
		// Initialize lighthouse state for large maps (overworld has lighthouse buildings)
		// Positions populated via UpdateLighthouseForLargeMap() which scans visible viewport
		// Original: original outdoor effect rules scan for LIGHTHOUSE tile in 32x32 map buffer
		// See ref:NPC-0002.
		g.UpdateLighthouseForLargeMap()
	case references.SmallMapType:
		g.resetSmashedForFreshTown()
		g.UpdateSmallMap(g.GameReferences.TileReferences, g.GameReferences.LocationReferences)
	case references.DungeonMapType:
		g.requireActiveDungeon()
		// Dungeon semantic authority and projections are installed before this transition.
		g.LighthouseState = nil
		g.ClearLightSourceTinting()
	case references.CombatMapType, references.CampingMapType:
		// Combat and camping retain their existing scene-owned transition behavior.
		// Clear lighthouse state - lighthouses are not in dungeons/combat/camping
		g.LighthouseState = nil
		// Clear light source tinting - will be recalculated on next frame
		g.ClearLightSourceTinting()
	}
}

// updateMapAfterFloorChange handles conditional updates for floor transitions
// Different map types have different floor change requirements
func (g *GameState) updateMapAfterFloorChange() {
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType:
		// Small maps: Perform hard NPC reset (authentic Ultima V behavior)
		// Original: town floor travel -> terrain reconstruction -> scheduled NPC placement
		// See ref:NPC-0002.
		g.hardResetSmallMapAfterFloorChange()
	case references.LargeMapType:
		// Large maps: switch overworld/underworld authority and refresh floor-specific state.
		g.CurrentNPCAIController = g.GetCurrentLargeMapNPCAIController()
		g.UpdateLighthouseForLargeMap()
		if currentMap := g.GetCurrentLayeredMap(); currentMap != nil {
			currentMap.InvalidateLightSourceCache()
		}
	case references.DungeonMapType:
		g.requireActiveDungeon()
	case references.CombatMapType, references.CampingMapType:
		// Combat and camping do not expose exploration floor transitions.
	}
}

// clearShadowlordIfLeavingKeep clears Shadowlord states when the player
// transitions between locations or floors.
//
// Yell-summoned Shadowlord (Summoned flag):
// - Cleared when leaving a Keep or changing floors within a Keep
// - The summoned Shadowlord exists at a specific position on a specific floor
//
// Automatic presence (AutomaticPresence flag):
// - Cleared when leaving a small map (town) entirely
// - NOT cleared on floor changes - effects persist within the same location
// - Source: Original shadowlord global persists within same location
func (g *GameState) clearShadowlordIfLeavingKeep(
	newLocation references.Location,
	newFloor references.FloorNumber,
	transitionType MapTransitionType,
) {
	currentLocation := g.MapState.PlayerLocation.Location
	currentFloor := g.MapState.PlayerLocation.Floor

	switch transitionType {
	case TransitionFullLocation:
		// Clear yell-summoned if leaving a Keep
		if g.ShadowlordState.Summoned {
			if references.IsKeepLocation(currentLocation) && currentLocation != newLocation {
				g.ShadowlordState.ClearSummoned()
			}
		}
		// Clear automatic presence when leaving a small map entirely
		// The automatic presence is tied to a specific town/location
		if g.ShadowlordState.HasAutomaticPresence() && currentLocation != newLocation {
			g.ShadowlordState.ClearAutomaticPresence()
		}

	case TransitionFloorOnly:
		// Clear yell-summoned if changing floors within a Keep
		// The spawned Shadowlord NPC is at a specific floor position
		if g.ShadowlordState.Summoned {
			if references.IsKeepLocation(currentLocation) && currentFloor != newFloor {
				g.ShadowlordState.ClearSummoned()
			}
		}
		// NOTE: Automatic presence is NOT cleared on floor changes
		// NPC effects persist across floor changes within the same location
		// Source: Original scheduled NPC placement does NOT reset loaded actor dialogue state values
		// See ref:NPC-0002.
	case TransitionPositionOnly:
		// Keep both states while the player remains on the same floor.
	}
}
