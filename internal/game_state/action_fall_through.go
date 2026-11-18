package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// TRAP fall-through constants based on original Ultima V behavior
const (
	// Fall damage - 1d8 HP per fall like original game party damage function
	// See ref:ENV-0002.
	TrapFallDamageMin = 1
	TrapFallDamageMax = 8
)

// processTrapFallThrough handles automatic fall-through mechanics for TRAP tiles (BrickFloorHole)
// Based on original Ultima V town rules behavior
// This simplified version just starts the fall timer instead of immediately executing falls
// See ref:ENV-0002.
func (g *GameState) processTrapFallThrough() {
	// Stonegate is resolved after waking at the consumed town-turn checkpoint,
	// including Pass. Movement must not review it before ordinary completion.
	if g.MapState.PlayerLocation.Location == references.Stonegate {
		return
	}
	currentTile := g.GetCurrentAvatarGroundTile()

	// Only TRAP tiles (BrickFloorHole) trigger automatic fall-through
	// GRATE tiles require manual Klimb command (already implemented correctly)
	if !currentTile.Is(indexes.BrickFloorHole) {
		return
	}

	// Riding the carpet bypasses this floor-hole fall. ref:ENV-0002
	if g.PartyVehicle.GetVehicleDetails().VehicleType == references.CarpetVehicle {
		return
	}

	// Skip if already falling (prevent double-triggers)
	if g.IsFalling() {
		return
	}

	currentPos := &g.MapState.PlayerLocation.Position
	logging.Debugf(logging.CategoryGameState, "TRAP tile detected at position (%d,%d), floor %d",
		currentPos.X, currentPos.Y, g.MapState.PlayerLocation.Floor)

	// Start the normal fall timer workflow
	g.StartTrapFallWorkflow()
}

// StartTrapFallWorkflow initiates a TRAP fall workflow using event queue system
// Replaces manual timer polling with coordinated While() loop for cleaner code
func (g *GameState) StartTrapFallWorkflow() {
	if g.EventQueueManager == nil {
		panic("GameState.StartTrapFallWorkflow: EventQueueManager is nil - critical system component missing")
	}

	if g.SystemCallbacks == nil {
		panic("GameState.StartTrapFallWorkflow: SystemCallbacks is nil - critical system component missing")
	}

	// Check if fall sequence is already running
	if g.EventQueueManager.HasWorkflow("trap_fall_sequence") {
		logging.Debugf(logging.CategoryGameState, "TRAP fall sequence already running")

		return
	}

	logging.Debugf(logging.CategoryGameState, "Starting TRAP fall sequence at floor %d", g.MapState.PlayerLocation.Floor)

	// Block input immediately to prevent movement during fall (before event queue starts)
	g.SystemCallbacks.Visual.BlockAllInput(true)

	// Create event queue with While() polling pattern to check for more falls during execution
	g.EventQueueManager.Workflow("trap_fall_sequence").
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {
			g.SystemCallbacks.UI.ShowActionFeedback("A TRAPDOOR!", true)

			if !g.GoDownOneFloor() {
				return
			}
			// Original trap falls use generic damage noise through
			// damageparty(). Redux distinguishes the severe fall event semantically.
			g.SystemCallbacks.Audio.PlaySoundEffect(SoundFallingSevere)

			if g.environmentalHazards == nil {
				g.environmentalHazards = environment.NewEnvironmentalHazards(g.rng, &messageCallbacksAdapter{g.SystemCallbacks})
			}
			config := environment.DamageConfig{
				DamageType: environment.TrapFallDamage,
				MinDamage:  TrapFallDamageMin,
				MaxDamage:  TrapFallDamageMax,
				Message:    "",
			}
			g.environmentalHazards.ApplyDamageToParty(&g.PartyState, config)

			logging.Debugf(logging.CategoryGameState, "TRAP fall: initial message displayed, first fall executed")
		}).
		While(
			func() bool { return g.canContinueFalling() },
			func() {
				g.SystemCallbacks.UI.ShowActionFeedback("Falling!", true)

				if !g.GoDownOneFloor() {
					return
				}
				g.SystemCallbacks.Audio.PlaySoundEffect(SoundFallingSevere)

				if g.environmentalHazards == nil {
					g.environmentalHazards = environment.NewEnvironmentalHazards(g.rng, &messageCallbacksAdapter{g.SystemCallbacks})
				}
				config := environment.DamageConfig{
					DamageType: environment.TrapFallDamage,
					MinDamage:  TrapFallDamageMin,
					MaxDamage:  TrapFallDamageMax,
					Message:    "",
				}
				g.environmentalHazards.ApplyDamageToParty(&g.PartyState, config)
			},
			1000,
		).ThenSequence(

		func(q *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
			currentTile := g.GetCurrentAvatarGroundTile()
			hazardType := g.detectTileHazardType(currentTile, g.MapState.PlayerLocation.Location.GetMapType())

			g.thenApplyHazardEffects(hazardType)(q)

			return q
		}).
		Then(func() {
			g.AdvanceGameplayMinutes(1)
		}).Finally(

		func() {
			g.cleanupFallSequence()
			g.ProcessPendingProvisionHours()
		}).
		Start()

	logging.Debugf(logging.CategoryGameState, "TRAP fall sequence started")
}

// canContinueFalling checks if the fall sequence should continue
// Returns true if player is on TRAP tile and can descend to next floor
func (g *GameState) canContinueFalling() bool {
	// Check if we can fall down to next floor
	if !g.CanGoDownOneFloor() {
		logging.Debugf(logging.CategoryGameState, "Cannot fall further - no lower floor available")

		return false
	}

	// Check tile at current position
	currentTile := g.GetCurrentAvatarGroundTile()

	// Continue falling if on TRAP tile
	if currentTile.Is(indexes.BrickFloorHole) {
		return true
	}

	logging.Debugf(logging.CategoryGameState, "Cascade stopped - no TRAP tile on floor %d",
		g.MapState.PlayerLocation.Floor)

	return false
}

// cleanupFallSequence completes the fall sequence and resets state
// Called by Finally() to ensure cleanup even if sequence fails
func (g *GameState) cleanupFallSequence() {
	logging.Debugf(logging.CategoryGameState, "Fall sequence completed at floor %d", g.MapState.PlayerLocation.Floor)

	// Input automatically unblocked by BlockInput()
}

// ProcessTrapFallThrough is the public interface for TRAP fall-through processing
// Called from movement system after successful movement
// Now initiates timer-based fall sequence instead of immediate execution
func (g *GameState) ProcessTrapFallThrough() {
	g.processTrapFallThrough()
}

// IsFalling returns whether the player is currently in a fall sequence
// Returns true when the trap_fall_sequence event queue is active
func (g *GameState) IsFalling() bool {
	if g.EventQueueManager == nil {
		return false
	}

	return g.EventQueueManager.HasWorkflow("trap_fall_sequence")
}

// FloodFloorWithLava replaces every terrain cell on the current floor
// with lava; the original town floor is 32 by 32. ref:ENV-0002
func (g *GameState) FloodFloorWithLava() {
	currentMap := g.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, g.MapState.PlayerLocation.Floor)
	trackedDoor := g.MapState.TrackedOpenDoor()
	trackedOnFloor := trackedDoor != nil && trackedDoor.Position.Location == g.MapState.PlayerLocation.Location &&
		trackedDoor.Position.Floor == g.MapState.PlayerLocation.Floor
	if trackedOnFloor {
		// Destructive replacement retires the timer without writing its old tile.
		// REDUX DEVIATION: CMD-0006 / ENV-0002: an original pending closure can
		// recreate a door after the flood. Redux retires that superseded door so
		// the lethal lava scene cannot retain or resurrect room-door graphics.
		g.MapState.ResetTransientDoor()
	}

	// Fill all 32-by-32 current-floor terrain cells with lava. ref:ENV-0002
	for x := range 32 {
		for y := range 32 {
			pos := &references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			base := currentMap.SpriteIndexByLayer(map_state.MapLayer, *pos)
			override := currentMap.SpriteIndexByLayer(map_state.MapOverrideLayer, *pos)
			if base.IsDoor() || override.IsDoor() || trackedOnFloor && trackedDoor.Position.Position == *pos {
				currentMap.UnSetTileByLayer(map_state.MapOverrideLayer, pos)
			}
			currentMap.SetTileByLayer(map_state.MapLayer, pos, indexes.Lava)
		}
	}

	logging.Debugf(logging.CategoryGameState, "Floor %d flooded with lava", g.MapState.PlayerLocation.Floor)
}

// ClearAllObjectsInCurrentLocation removes objects across every floor
// of the loaded location, including dropped items stored separately by Redux. ref:ENV-0002
func (g *GameState) ClearAllObjectsInCurrentLocation() {
	// Clear all NPCs and vehicles (including quest items like the Sceptre).
	// The avatar is not in MapUnits - it's tracked separately in PlayerLocation
	if g.CurrentNPCAIController != nil {
		if npcs := g.CurrentNPCAIController.GetNpcs(); npcs != nil {
			npcs.ClearAllMapUnits()
		}
	}

	g.ItemStacksMap = *references.NewItemStacksMap()
	g.clearSurfaceCorpsesInCurrentMap()
	logging.Debugf(logging.CategoryGameState, "All objects cleared from location %d", g.MapState.PlayerLocation.Location)
}
