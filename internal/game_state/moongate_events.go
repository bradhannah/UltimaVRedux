package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// MoongateTeleportWorkflow names the gate travel workflow that GameScene starts.
const MoongateTeleportWorkflow = "moongate_teleport"

const (
	moongateTransitionWorkflow = "moongate_transition_workflow"
	moongateStageStepMs        = int64(3000 / int(MoongateStageFull))
)

// MoongateStage is the authoritative transient materialization stage.
type MoongateStage uint8

const (
	// MoongateStageHidden means moongates do not exist in runtime gameplay.
	MoongateStageHidden MoongateStage = 0
	// MoongateStageFull means moongates are fully materialized.
	MoongateStageFull MoongateStage = 16
)

// MoongateStage returns the current authoritative runtime stage.
func (gs *GameState) MoongateStage() MoongateStage {
	return gs.moongateStage
}

func moongatesShouldBeActive(hour byte) bool {
	return hour >= 20 || hour < 5
}

func (gs *GameState) desiredMoongateStage() MoongateStage {
	if moongatesShouldBeActive(gs.DateTime.Hour) {
		return MoongateStageFull
	}

	return MoongateStageHidden
}

func (gs *GameState) initializeMoongateStage() {
	gs.moongateStage = gs.desiredMoongateStage()
}

// isSourceMoongateInSight reports whether a placed source gate on the current map
// and floor is in the party's line of sight, whatever the current stage.
func (gs *GameState) isSourceMoongateInSight() bool {
	location := gs.MapState.PlayerLocation
	gates := gs.moonstoneSystem.GetActiveMoongatePositions(location.Location, location.Floor)
	if len(gates) == 0 {
		return false
	}
	visible := gs.GetCurrentLayeredMap().GetVisibilityFlags()
	for _, gate := range gates {
		if visible[gate.X][gate.Y] {
			return true
		}
	}

	return false
}

// IsMoongateTransitionActive reports whether stage progression is in flight.
func (gs *GameState) IsMoongateTransitionActive() bool {
	return gs.EventQueueManager != nil && gs.EventQueueManager.HasWorkflow(moongateTransitionWorkflow)
}

// UpdateMoongateTransition starts growth at hour 20 or shrink at hour 5.
// GameScene calls this from the deterministic elapsed/event update path.
func (gs *GameState) UpdateMoongateTransition() {
	if gs.IsTeleportationActive() || gs.HasActiveDurationEffect(DurationEffectTimeStop) {
		return
	}
	if gs.moongateStage == gs.desiredMoongateStage() || gs.IsMoongateTransitionActive() {
		return
	}
	gs.StartMoongateTransitionWorkflow()
}

// StartMoongateTransitionWorkflow advances the shared stage one deterministic
// step at a time. Time Stop leaves the workflow active but freezes each step.
func (gs *GameState) StartMoongateTransitionWorkflow() {
	if gs.EventQueueManager == nil {
		panic("GameState.StartMoongateTransitionWorkflow: EventQueueManager is nil - critical system component missing")
	}
	if gs.SystemCallbacks == nil {
		panic("GameState.StartMoongateTransitionWorkflow: SystemCallbacks is nil - critical system component missing")
	}
	if gs.moongateStage == gs.desiredMoongateStage() || gs.IsMoongateTransitionActive() {
		return
	}
	// REDUX DEVIATION: MOON-0003: the original steps every gate silently through display
	// updates. Redux animates and narrates only a source gate in the party's sight; an
	// unseen change completes at once so it never interrupts held commands.
	if !gs.isSourceMoongateInSight() {
		gs.moongateStage = gs.desiredMoongateStage()

		return
	}

	growing := gs.desiredMoongateStage() > gs.moongateStage
	logging.Debugf(logging.CategoryMoongate, "Starting moongate stage transition - stage=%d growing=%v", gs.moongateStage, growing)
	// Below gate travel: entering a partial gate must preempt the stage steps,
	// which pause while teleportation is active. ref:MOON-0003
	gs.EventQueueManager.Workflow(moongateTransitionWorkflow).
		Priority(event_queue.PriorityMedium).
		Sequential().
		First(func() {
			if growing {
				gs.SystemCallbacks.Message.AddRowStr("Instantly, a shimmering blue door springs up!")
			} else {
				gs.SystemCallbacks.Message.AddRowStr("The shimmering door fades from view.")
			}
		}).
		While(
			func() bool { return gs.moongateStage != gs.desiredMoongateStage() },
			func() {
				if gs.HasActiveDurationEffect(DurationEffectTimeStop) || gs.IsTeleportationActive() {
					return
				}
				if gs.desiredMoongateStage() > gs.moongateStage {
					gs.moongateStage++
				} else {
					gs.moongateStage--
				}
			},
			moongateStageStepMs,
		).
		Finally(func() {
			logging.Debugf(logging.CategoryMoongate, "Moongate stage transition complete - stage=%d", gs.moongateStage)
		}).
		Start()
}
