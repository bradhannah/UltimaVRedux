package game_state

// Action_hole_up_repair.go - Ship hull repair via Hole Up command
// Ship repair is separate from camping - it's a quick 25-minute repair action
// See ref:SHIP-0004.

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Ship repair constants
// See ref:SHIP-0004.
const (
	shipRepairWorkflowName = "ship_repair_workflow"

	// ShipRepairCycles is the number of repair cycles (original: for i=0; i<5; i++)
	ShipRepairCycles = 5

	// ShipRepairMinutesPerCycle is game minutes advanced per cycle (original: addtime(5))
	ShipRepairMinutesPerCycle = 5

	// ShipRepairVisualDelayMs is the delay between cycles for visual feedback
	ShipRepairVisualDelayMs = 500

	// ShipRepairDiceSides is the dice roll for hull repair amount (original: random(1,3))
	ShipRepairDiceSides = 3

	// ShipRepairMinHull is the minimum hull HP after the repair loop. ref:SHIP-0004
	ShipRepairMinHull = 10

	// ShipRepairMaxHull caps repaired hull HP. ref:SHIP-0004
	ShipRepairMaxHull = 99
)

type shipRepairState struct {
	completedCycles      int
	awaitingCombat       bool
	completeSurfaceCycle bool
}

// ActionHoleUpRepairShip handles the ship repair version of Hole Up command
// - Frigates only (not skiffs)
// - Sails must be lowered (furled/anchored)
// - Quick repair taking 25 game minutes (5 cycles × 5 minutes)
// - No guard selection or hours prompt (unlike camping)
// See ref:SHIP-0004.
func (g *GameState) ActionHoleUpRepairShip() bool {
	if g.activeShipRepair != nil || g.EventQueueManager.HasWorkflow(shipRepairWorkflowName) {
		return false
	}
	// Require lowered sails before repairing the frigate. ref:SHIP-0004
	if !g.PartyVehicle.GetVehicleDetails().IsSailFurled() {
		g.SystemCallbacks.Message.AddRowStr("Sails must be")
		g.SystemCallbacks.Message.AddRowStr("lowered!")

		return false
	}

	// Build repair event queue with visual feedback
	g.StartShipRepairWorkflow()

	return true
}

// StartShipRepairWorkflow creates and starts the ship repair event queue
// Uses event queue for enhanced visual feedback during repair cycles
func (g *GameState) StartShipRepairWorkflow() {
	if g.activeShipRepair != nil || g.EventQueueManager.HasWorkflow(shipRepairWorkflowName) {
		return
	}
	state := &shipRepairState{}
	g.activeShipRepair = state
	g.startShipRepairWorkflow(state, true)
}

func (g *GameState) startShipRepairWorkflow(state *shipRepairState, firstSequence bool) {
	if state == nil || state != g.activeShipRepair {
		panic("ship repair workflow requires its active continuation")
	}
	if g.EventQueueManager.HasWorkflow(shipRepairWorkflowName) {
		panic("ship repair workflow is already active")
	}
	q := g.EventQueueManager.Workflow(shipRepairWorkflowName).
		Priority(event_queue.PriorityNormal).
		Sequential().BlockInput()

	interrupted := false
	pausedForCombat := false

	start := func() {
		logging.Debugf(logging.CategoryGameState, "Ship repair: Starting repair sequence, hull=%d",
			g.PartyVehicle.GetVehicleDetails().GetHullHP())
		if firstSequence {
			g.SystemCallbacks.Message.AddRowStr("Repairing...")
		}
	}
	q.First(start)

	for range ShipRepairCycles - state.completedCycles {
		q.If(func() bool { return !interrupted && !pausedForCombat }, func(cycle *event_queue.SequentialEventBuilder) {
			cycle.Wait(ShipRepairVisualDelayMs).
				Then(func() {
					g.processNonCombatWorldActorPhase()
					if g.HasPendingOverworldContact() {
						state.awaitingCombat = true
						pausedForCombat = true
					}
				}).While(
				func() bool {
					return !pausedForCombat && g.EventQueueManager.HasWorkflow(pirateAttackWorkflowName)
				},
				func() {},
				1,
			).
				Then(func() {
					if pausedForCombat {
						return
					}
					if !g.isOnAnchoredFrigate() {
						interrupted = true

						return
					}
					g.advanceGameplayMinutes(ShipRepairMinutesPerCycle, false)
					state.completedCycles++
					g.SystemCallbacks.Message.AddRowStr("*hammering*")

					logging.Debugf(logging.CategoryGameState, "Ship repair: Cycle %d/%d complete",
						state.completedCycles, ShipRepairCycles)
				})
		})
	}

	q.Then(func() {
		if interrupted {
			g.finishShipRepair(state, false)

			return
		}
		if pausedForCombat {
			return
		}
		if state.completedCycles != ShipRepairCycles {
			panic("ship repair workflow completed without all cycles")
		}
		g.finishShipRepair(state, true)
	}).Finally(func() {
		logging.Debugf(logging.CategoryGameState, "Ship repair: Workflow completed")
	}).
		Start()
}

func (g *GameState) deferSurfaceCommandCycleForShipRepair(originMapType references.GeneralMapType) bool {
	if g.activeShipRepair == nil {
		return false
	}
	if originMapType != references.LargeMapType {
		panic("ship repair completion requires a large-map origin")
	}
	g.activeShipRepair.completeSurfaceCycle = true

	return true
}

func (g *GameState) resumeShipRepairAfterCombat() {
	state := g.activeShipRepair
	if state == nil || !state.awaitingCombat {
		return
	}
	state.awaitingCombat = false
	if g.PartyState.IsAllPartyDead() || !g.isOnAnchoredFrigate() {
		g.finishShipRepair(state, false)

		return
	}

	g.advanceGameplayMinutes(ShipRepairMinutesPerCycle, false)
	state.completedCycles++
	g.SystemCallbacks.Message.AddRowStr("*hammering*")
	if state.completedCycles == ShipRepairCycles {
		g.finishShipRepair(state, true)

		return
	}
	g.startShipRepairWorkflow(state, false)
}

func (g *GameState) finishShipRepair(state *shipRepairState, applyRepair bool) {
	if state == nil || state != g.activeShipRepair {
		panic("finishing a stale ship repair continuation")
	}
	completeSurfaceCycle := state.completeSurfaceCycle
	g.activeShipRepair = nil
	if applyRepair {
		g.applyShipRepair()
	}
	if completeSurfaceCycle {
		g.CompleteSurfaceCommandCycle(references.LargeMapType)
	}
}

func (g *GameState) abortShipRepairAfterUnresolvedCombat() {
	state := g.activeShipRepair
	if state == nil || !state.awaitingCombat {
		return
	}
	completeSurfaceCycle := state.completeSurfaceCycle
	g.activeShipRepair = nil
	if completeSurfaceCycle && !g.PartyState.IsAllPartyDead() {
		// The actor phase already ran before it queued the contact. Do not rerun
		// that same invalid contact while completing the interrupted command.
		g.completeSurfaceCycleWithActorPhase(references.LargeMapType, DefaultLargeMapMinutesPerTurn, false)
	}
}

// applyShipRepair always makes an initial 1..3 hull-repair draw, caps hull
// at 99, and repeats until hull reaches at least 10. ref:SHIP-0004
func (g *GameState) applyShipRepair() {
	vehicleDetails := g.PartyVehicle.GetVehicleDetails()
	hull := vehicleDetails.GetHullHP()
	initialHull := hull

	// Original do-while consumes at least one roll, even when hull is already 99.
	for {
		repairAmount := g.RollDice(ShipRepairDiceSides)
		hull += repairAmount
		if hull > ShipRepairMaxHull {
			hull = ShipRepairMaxHull
		}
		if hull >= ShipRepairMinHull {
			break
		}
	}

	vehicleDetails.SetHullHP(hull)

	// Report the resulting hull value after repair. ref:SHIP-0004
	g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Hull now %d!", hull))

	logging.Debugf(logging.CategoryGameState, "Ship repair: Hull repaired from %d to %d",
		initialHull, hull)
}

// isOnFrigate checks if the player is currently on a frigate
// Used to route to ship repair instead of camping
func (g *GameState) isOnFrigate() bool {
	return g.PartyVehicle.GetVehicleDetails().VehicleType == references.FrigateVehicle
}

func (g *GameState) isOnAnchoredFrigate() bool {
	vehicle := g.PartyVehicle.GetVehicleDetails()

	return vehicle.VehicleType == references.FrigateVehicle && vehicle.IsSailFurled()
}
