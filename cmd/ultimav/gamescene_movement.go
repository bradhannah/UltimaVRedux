package main

import (
	"fmt"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const mapExitPromptWorkflowName = "map_exit_prompt"

// Keyboard.GetPressedArrowKey resolves simultaneous arrows in Down, Up, Right,
// Left priority order. GameScene.getKeyAsDirection maps an accepted key to the
// movement direction.

func (g *GameScene) moveToNewPositionByDirection(direction references.Direction) {
	before := g.gameState.MapState.PlayerLocation.Position
	g.gameState.MovePlayerOneStep(direction)
	after := g.gameState.MapState.PlayerLocation.Position

	// GetEffectiveTile() handles all tile overrides dynamically - no layer refresh needed
	if g.gameState.PartyVehicle.GetVehicleDetails().VehicleType != references.NoPartyVehicle {
		// Surface movement has already wrapped/clamped both axes to the map.
		// Reject an impossible position rather than truncating its schedule bytes.
		scheduleX, scheduleY := after.X, after.Y
		if scheduleX < 0 || scheduleX > math.MaxUint8 || scheduleY < 0 || scheduleY > math.MaxUint8 {
			panic("party vehicle movement has invalid schedule coordinates")
		}
		g.gameState.PartyVehicle.SetPos(after)
		g.gameState.PartyVehicle.NPCReference.Schedule.OverrideAllPositions(byte(scheduleX), byte(scheduleY))
		g.gameState.PartyVehicle.SetDirection(direction)
	}
	if before != after {
		logging.Debugf(logging.CategoryMovement,
			"Avatar moved: location=%s floor=%d direction=%s from=(%d,%d) to=(%d,%d) vehicle=%s",
			g.gameState.MapState.PlayerLocation.Location,
			g.gameState.MapState.PlayerLocation.Floor,
			direction.GetDirectionCompassName(),
			before.X, before.Y,
			after.X, after.Y,
			g.gameState.PartyVehicle.GetVehicleDetails().VehicleType.GetLowerCaseName())
	}
}

func (g *GameScene) checkAndAutoKlimbStairs(position *references.Position) bool {
	floorKlimbOffset := g.gameState.MapState.LayeredMaps.GetSmallMapFloorKlimbOffset(*position, g.gameState.MapState.PlayerLocation.Floor)
	if floorKlimbOffset != 0 {
		// Use same fade workflow as manual klimb for consistent UX
		goUp := floorKlimbOffset > 0
		g.gameState.StartKlimbFloorWorkflow(goUp)

		return true
	}

	return false
}

// handleMovement
// Handles the movement of the player in the game scene
// Returns an explicit movement completion result; the input handler owns applying it.
func (g *GameScene) handleMovement(directionStr string, key ebiten.Key) surfaceCommandResult {
	return g.handleMovementWithBlockedFeedback(directionStr, key, true)
}

func (g *GameScene) handleAutomaticSailingMovement(direction references.Direction) surfaceCommandResult {
	return g.handleMovementWithBlockedFeedback(
		direction.GetDirectionCompassName(),
		automaticSailingDirectionKey(direction),
		false,
	)
}

//nolint:maintidx // Surface collision, transition, hazard, and encounter ordering remains one cohesive route.
func (g *GameScene) handleMovementWithBlockedFeedback(
	directionStr string,
	key ebiten.Key,
	reportBlocked bool,
) surfaceCommandResult {
	originMapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()

	// Check if any moongate is animating within visible range - only block movement if moongate is visible
	// Moongates can exist on both large maps (overworld) and small maps (towns)
	if g.moongateVisualSystem == nil {
		panic("MoongateVisualSystem is nil")
	}

	// Block movement during TRAP fall sequences
	if g.gameState.IsFalling() {
		currentTile := g.gameState.GetCurrentAvatarGroundTile()
		if currentTile.Is(indexes.BrickFloorHole) {
			// Block movement silently during fall sequence
			return newSurfaceCommandResult(surfaceCommandStillActive, originMapType)
		}
	}

	vehicle := g.gameState.PartyVehicle.GetVehicleDetails()
	prefix := vehicle.VehicleType.GetMovementPrefix()
	if vehicle.VehicleType == references.FrigateVehicle && vehicle.GetSailState() == map_units.SailRaised {
		prefix = vehicle.GetSailState().GetMovementPrefix()
	}
	g.gameState.SystemCallbacks.Message.ShowCommandPrompt(fmt.Sprintf(
		"%s%s",
		prefix,
		directionStr,
	))
	// moveResultsInMovement
	direction := g.getKeyAsDirection(key)
	newPosition := direction.GetNewPositionInDirection(&g.gameState.MapState.PlayerLocation.Position)
	mapType := originMapType
	moved := false

	if mapType == references.LargeMapType && g.gameState.IsLostInDarkness() && !g.gameState.DebugOptions.FreeMove {
		// The move is accepted and takes its turn, but the party goes nowhere.
		// See ref:QUEST-0004.
		g.addRowStr(game_state.LostInDarknessMessage)

		return newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
	}

	if mapType == references.LargeMapType {
		if g.gameState.PartyVehicle.GetVehicleDetails().VehicleType == references.FrigateVehicle && !g.gameState.PartyVehicle.GetVehicleDetails().DoesMoveResultInMovement(direction) {
			g.gameState.PartyVehicle.GetVehicleDetails().SetPartyVehicleDirection(direction)
			g.output.AddText("Head " + direction.GetDirectionCompassName())

			return newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
		}

		newPosition = newPosition.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
	}
	g.gameState.PartyVehicle.GetVehicleDetails().SetPartyVehicleDirection(direction)
	if mapType == references.LargeMapType && g.gameState.QueueOverworldEncounterAt(*newPosition) {
		result := newSurfaceCommandResult(surfaceCommandRejected, originMapType)
		result.ConsumesNormalCycle = false

		return result
	}

	if mapType == references.SmallMapType && g.gameState.IsOutOfBounds(*newPosition) {
		if g.gameState.EventQueueManager.HasWorkflow(mapExitPromptWorkflowName) {
			return newSurfaceCommandResult(surfaceCommandStillActive, originMapType)
		}
		// Use event queue with YesNo helper for exit confirmation
		transitioned := false
		g.gameState.EventQueueManager.Workflow(mapExitPromptWorkflowName).
			Priority(event_queue.PriorityLow).
			Sequential().BlockInput().ThenSequence(

			g.thenShowYesNoDialog(
				"Dost thou wish to leave?",
				func(yesQ *event_queue.SequentialEventBuilder) {
					yesQ.Then(func() { transitioned = true }).
						ThenSequence(g.gameState.ThenExitLocationTransition())
				},
				func(_ *event_queue.SequentialEventBuilder) {
					// NO - stay in building (no action needed)
				},
			)).Finally(func() {
			state := surfaceCommandCancelled
			if transitioned {
				state = surfaceCommandCompleted
			}
			result := newSurfaceCommandResult(state, originMapType)
			result.MapTransitioned = transitioned
			g.completeSurfaceMovement(result)
		}).
			Start()

		logging.Debugf(logging.CategoryMovement,
			"Small-map boundary exit requested: location=%s position=(%d,%d) target=(%d,%d)",
			g.gameState.MapState.PlayerLocation.Location,
			g.gameState.MapState.PlayerLocation.Position.X,
			g.gameState.MapState.PlayerLocation.Position.Y,
			newPosition.X,
			newPosition.Y)

		return newSurfaceCommandResult(surfaceCommandDeferred, originMapType)
	}

	if g.gameState.IsPassable(newPosition) || g.gameState.DebugOptions.FreeMove {
		g.moveToNewPositionByDirection(direction)
		moved = true
		avatarTopTile := g.gameState.GetCurrentAvatarGroundTile()
		extraMovementStr := avatarTopTile.GetExtraMovementString()

		// Play movement sound based on vehicle type or walking surface
		if g.audioManager != nil {
			vehicleType := g.gameState.PartyVehicle.GetVehicleDetails().VehicleType
			switch vehicleType {
			case references.NoPartyVehicle:
				// On foot - play walking sound based on tile surface
				err := g.audioManager.PlayWalkingSound(avatarTopTile)
				if err != nil {
					logging.Warnf(logging.CategoryAudio, "Walking sound failed: %v", err)
				}
			case references.HorseVehicle:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMoveHorse)
			case references.SkiffVehicle:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMoveSkiff)
			case references.FrigateVehicle:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMoveFrigate)
			case references.CarpetVehicle:
				g.gameState.SystemCallbacks.Audio.PlaySoundEffect(game_state.SoundMoveCarpet)
			case references.NPC:
				// NPCs are not party vehicles and have no party movement sound.
			default:
				// Unknown types retain the silent fallback.
			}
		}

		if extraMovementStr != "" {
			g.output.AddText(extraMovementStr)
		}

		// Update lighthouse positions on large map movement
		// Rescan lighthouse emitters when the map buffer changes (ref:LIGHT-0003).
		switch mapType {
		case references.LargeMapType:
			g.gameState.UpdateLighthouseForLargeMap()
		case references.SmallMapType:
			// Update lighthouse for small maps - scans current floor only
			// Handles floor changes via klimb and player moving in/out of beacon range
			g.gameState.UpdateLighthouseForSmallMap()
		case references.DungeonMapType, references.CombatMapType, references.CampingMapType:
			// Only surface maps have lighthouse emitters to refresh.
		}

		// Whirlpool contact owns the source hit and teleport; do not also apply
		// underlying deep-water or waterfall movement damage.
		if g.gameState.IsPlayerCurrentlyOnWhirlpool() {
			q := g.gameState.EventQueueManager.Workflow("whirlpool_transportation").
				Priority(event_queue.PriorityHigh).
				Sequential()
			g.gameState.BuildWhirlpoolTransportationQueue(q)
			q.Start()

			result := newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
			result.MovementOccurred = true

			return result
		}

		// Process skiff/carpet deep-water damage (ref:ENV-0003).
		g.gameState.ProcessSkiffDeepWaterDamage(newPosition)

		// Process TRAP fall-through after movement (only in small maps)
		if g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType {
			g.gameState.ProcessTrapFallThrough()
		}

		// Check for moongate entry (can happen on both large and small maps)
		g.checkMoongateEntry()

		// REDUX DEVIATION: ENV-0004: original bridge checks run after every
		// completed outdoor turn. Redux ties the check to successful movement so
		// the encounter represents crossing the bridge rather than waiting on it.
		if mapType == references.LargeMapType {
			if g.checkBridgeTrollEncounter(avatarTopTile) {
				result := newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
				result.MovementOccurred = true

				return result
			}
		}
	} else {
		// Check if we can auto-open a door at this position
		if mapType == references.SmallMapType && g.gameState.CanAutoOpenDoor(newPosition) {
			// Start auto-open sequence
			g.autoOpenDoorState.StartAutoOpenSequence(direction, newPosition, g.getElapsedMs())
		} else {
			// Movement blocked
			if reportBlocked {
				g.addRowStr("Blocked!")
			}
			if mapType == references.SmallMapType {
				// Original town movement clears keyboard input after a blocked step. Redux
				// requires physical release so a held direction cannot cross after an
				// NPC moves.
				g.keyboard.PrepareForTransition(0)
			}
			result := newSurfaceCommandResult(surfaceCommandRejected, originMapType)
			if mapType == references.LargeMapType {
				result.ConsumesNormalCycle = false
			}

			return result
		}
	}

	if g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType {
		g.checkAndAutoKlimbStairs(newPosition)
	}

	result := newSurfaceCommandResult(surfaceCommandCompleted, originMapType)
	result.MovementOccurred = moved
	if mapType == references.SmallMapType && g.autoOpenDoorState.IsActive() {
		result.State = surfaceCommandStillActive
		result.ConsumesNormalCycle = false
		result.MovementOccurred = false
	}

	return result
}

// getKeyAsDirection converts UI input keys to game logic directions
// Moved from internal/map_state to maintain package boundaries (UI layer handles input mapping)
func (g *GameScene) getKeyAsDirection(key ebiten.Key) references.Direction {
	//nolint:exhaustive // Only arrow keys represent movement here; other keys return NoneDirection.
	switch key {
	case ebiten.KeyUp:
		return references.Up
	case ebiten.KeyDown:
		return references.Down
	case ebiten.KeyLeft:
		return references.Left
	case ebiten.KeyRight:
		return references.Right
	}

	return references.NoneDirection
}
