package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// BuildMoongateTeleportationSequence preserves the source gate during departure
// and fully materializes the destination gate before the map and Avatar return.
func (gs *GameState) BuildMoongateTeleportationSequence(destination MoongateDestination) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return gs.buildTeleportationSequence(destination, journal.MoonstoneSourceGate)
}

// BuildMoonstoneSpellTeleportationSequence carries Vas Rel Por's captured phase
// through success, without giving generic teleports moonstone knowledge.
func (gs *GameState) BuildMoonstoneSpellTeleportationSequence(destination MoongateDestination) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	return gs.buildTeleportationSequence(destination, journal.MoonstoneSourceSpell)
}

func (gs *GameState) buildTeleportationSequence(
	destination MoongateDestination,
	source journal.MoonstoneSource,
) func(*event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
	var preparedDungeon *DungeonState
	if destination.Location.GetMapType() == references.DungeonMapType &&
		(gs.activeDungeon == nil || gs.activeDungeon.location != destination.Location) {
		facing := references.DungeonFacingNorth
		lastDirection := references.DungeonArrivedFromAbove
		if gs.activeDungeon != nil {
			facing = gs.activeDungeon.facing
			lastDirection = gs.activeDungeon.lastDirection
		}
		var err error
		preparedDungeon, err = gs.PrepareDungeonState(destination.Location, facing, lastDirection)
		if err != nil {
			panic(fmt.Errorf("prepare teleport destination %s: %w", destination.Location, err))
		}
	}

	return func(seb *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		logging.Debugf(logging.CategoryMoongate, "Building teleportation sequence to location %d", destination.Location)

		return seb.
			Then(func() {
				// Initialize avatar dissolve effect and play initial sound
				gs.SystemCallbacks.Visual.InitializeAvatarDissolveEffect()
				gs.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
				logging.Debugf(logging.CategoryMoongate, "Phase 1: Starting avatar dissolve out")
			}).
			Animate(1000, func(progress float32) {
				// Phase 1: Avatar dissolve out (0.0 → 1.0 over 1 second)
				gs.SystemCallbacks.Visual.SetAvatarDissolveLevel(float64(progress))
			}).
			Then(func() {
				logging.Debugf(logging.CategoryMoongate, "Phase 2: Starting map fade to black")
			}).
			Animate(2000, func(progress float32) {
				// Phase 2: Map fade to black (0.0 → 1.0 over 2 seconds)
				gs.SystemCallbacks.Visual.SetMapFadeLevel(float64(progress))
			}).
			Then(func() {
				// Phase 3: execute the authoritative transition between renderer handoffs.
				logging.Debugf(logging.CategoryMoongate, "Phase 3: Executing teleportation")
				gs.SystemCallbacks.Visual.ResetTeleportationSourceRenderer()
				if preparedDungeon != nil {
					gs.installPreparedDungeonState(preparedDungeon)
				}
				gs.TransitionToLocation(
					destination.Location,
					references.FloorNumber(destination.Level),
					references.Position{X: references.Coordinate(destination.X), Y: references.Coordinate(destination.Y)},
					TransitionFullLocation,
					false,
				)
				if source != journal.MoonstoneSourceNone {
					gs.observeMoonstone(int(destination.Phase), moonstoneJournalSite(gs.MapState.PlayerLocation), source)
				}
				if source == journal.MoonstoneSourceGate {
					// REDUX DEVIATION: gate entry leaves gate stage at zero and
					// gate-stage updates resumes growth after gate travel. Present the arrival
					// gate fully formed beneath the Avatar dissolve instead.
					// See ref:MOON-0005.
					gs.moongateStage = MoongateStageFull
				}
				gs.SystemCallbacks.Visual.StabilizeTeleportationDestinationRenderer(destination)
				gs.SystemCallbacks.Screen.UpdateDisplay()
				if destination.Location == references.SkaraBrae {
					gs.SystemCallbacks.Message.AddRowStr("The moongate whisks you to the ruins of Skara Brae!")
				} else {
					gs.SystemCallbacks.Message.AddRowStr("The moongate whisks you away!")
				}

				// Keep dissolve and fade at maximum during teleportation
				gs.SystemCallbacks.Visual.SetAvatarDissolveLevel(1.0)
				gs.SystemCallbacks.Visual.SetMapFadeLevel(1.0)

				logging.Debugf(logging.CategoryMoongate, "Phase 4: Starting map fade from black")
			}).
			Animate(2000, func(progress float32) {
				// Phase 4: Map fade from black (1.0 → 0.0 over 2 seconds)
				gs.SystemCallbacks.Visual.SetMapFadeLevel(1.0 - float64(progress))
			}).
			Then(func() {
				// Play materialization sound before final dissolve
				gs.SystemCallbacks.Audio.PlaySoundEffect(SoundMoongateTravel)
				logging.Debugf(logging.CategoryMoongate, "Phase 5: Starting avatar dissolve in")
			}).
			Animate(1000, func(progress float32) {
				// Phase 5: Avatar dissolve in (1.0 → 0.0 over 1 second)
				gs.SystemCallbacks.Visual.SetAvatarDissolveLevel(1.0 - float64(progress))
			}).
			Then(func() {
				// Cleanup teleportation effects
				gs.SystemCallbacks.Visual.SetAvatarDissolveLevel(0.0)
				gs.SystemCallbacks.Visual.SetMapFadeLevel(0.0)
				gs.SystemCallbacks.Visual.CleanupAvatarDissolveEffect()
				logging.Debugf(logging.CategoryMoongate, "Teleportation sequence completed")
			})
	}
}

// IsTeleportationActive checks if a teleportation sequence is currently running
// Returns true when the moongate_teleport event queue is active
func (gs *GameState) IsTeleportationActive() bool {
	if gs.EventQueueManager == nil {
		return false
	}

	return gs.EventQueueManager.HasWorkflow(MoongateTeleportWorkflow)
}
