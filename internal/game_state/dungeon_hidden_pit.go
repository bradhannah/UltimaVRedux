package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	dungeonHiddenPitWorkflowName = "dungeon_hidden_pit"
	dungeonPitDamageCadenceMs    = 100
)

type dungeonHiddenPitFallStep struct {
	sourceFloor      references.FloorNumber
	destinationFloor references.FloorNumber
	bottomFloor      bool
}

func isDungeonHiddenPit(cell references.DungeonCell) bool {
	return cell.Raw() == 0x61 || cell.Raw() == 0x69
}

// ActionDungeonHiddenPitFall retains movement's command token through the complete fall presentation.
func (g *GameState) ActionDungeonHiddenPitFall(token DungeonCommandCycle, result DungeonMoveResult) {
	if result.Outcome != DungeonMoveMoved || !result.TriggersHiddenPit ||
		g.MapState.PlayerLocation.Position != result.Target {
		panic("dungeon hidden-pit fall requires a successful triggering movement")
	}
	cell, ok := g.requireActiveDungeon().Cell(g.MapState.PlayerLocation.Floor, result.Target)
	if !ok || !isDungeonHiddenPit(cell) {
		panic("dungeon hidden-pit trigger no longer matches authoritative state")
	}

	cycle := g.beginDungeonCommandCompletion(token, result.Completion)
	g.startDungeonHiddenPitFallWorkflow(token, cycle, true)
}

func (g *GameState) startCurrentDungeonHiddenPitFall(token DungeonCommandCycle) bool {
	cell, ok := g.requireActiveDungeon().Cell(
		g.MapState.PlayerLocation.Floor,
		g.MapState.PlayerLocation.Position,
	)
	if !ok || !isDungeonHiddenPit(cell) {
		return false
	}
	cycle := g.activeDungeonCommand
	if cycle == nil || token.id == 0 || token.id != cycle.token.id || !cycle.completionStarted {
		panic("hidden-pit continuation requires the active completing dungeon command")
	}
	g.startDungeonHiddenPitFallWorkflow(token, cycle, false)

	return true
}

func (g *GameState) startDungeonHiddenPitFallWorkflow(
	token DungeonCommandCycle,
	cycle *dungeonCommandCycleState,
	processBeforeCell bool,
) {
	if g.EventQueueManager == nil {
		panic("dungeon hidden-pit fall requires an event queue manager")
	}
	if g.SystemCallbacks == nil {
		panic("dungeon hidden-pit fall requires system callbacks")
	}
	if g.EventQueueManager.HasWorkflow(dungeonHiddenPitWorkflowName) {
		panic("dungeon hidden-pit workflow is already active")
	}
	g.EventQueueManager.Workflow(dungeonHiddenPitWorkflowName).
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput().
		First(func() {
			if processBeforeCell {
				g.processDungeonEndPlayerBeforeCell(cycle.dungeonActorsMove)
			}
			g.requireActiveDungeon().setLastDirection(references.DungeonArrivedFromAbove)
		}).
		ThenSequence(g.thenDungeonHiddenPitFalls()).
		Finally(func() {
			if !g.queueCurrentDungeonRoomEncounter(true, false, true, dungeonRoomGenerateWanderer) {
				g.processDungeonEndPlayerAfterCell()
			}
			g.finishDungeonCommandCycle(token)
		}).
		Start()
}

func (g *GameState) thenDungeonHiddenPitFalls() event_queue.Sequence {
	return func(sequence *event_queue.SequentialEventBuilder) *event_queue.SequentialEventBuilder {
		position := g.MapState.PlayerLocation.Position
		for _, step := range g.planDungeonHiddenPitFalls() {
			sequence.Then(func() {
				g.SystemCallbacks.Message.AddRowStr("Pit Trap!")
				g.SystemCallbacks.Message.AddRowStr("Falling...")
				g.transformDungeonHiddenPitSource(step.sourceFloor, position)
			})
			if !step.bottomFloor {
				sequence.Then(func() {
					g.markDungeonHiddenPitDestination(step.destinationFloor, position)
					g.transitionDungeonFloor(step.destinationFloor, references.DungeonArrivedFromAbove)
				})
			}
			sequence.Then(func() {
				g.SystemCallbacks.Message.AddRowStr("      ...splat!")
			})
			for partyIndex := range party_state.MaxActiveParty {
				index := partyIndex
				sequence.If(
					func() bool { return g.isLivingActivePartyMember(index) },
					func(q *event_queue.SequentialEventBuilder) {
						q.Then(func() {
							g.damageDungeonPartyMember(index)
						}).Wait(dungeonPitDamageCadenceMs)
					},
				)
			}
		}

		return sequence.Then(func() {
			dungeon := g.requireActiveDungeon()
			cell, ok := dungeon.Cell(g.MapState.PlayerLocation.Floor, g.MapState.PlayerLocation.Position)
			if !ok {
				panic("dungeon hidden-pit landing is out of bounds")
			}
			if cell.Kind() == references.DungeonClearedRoom || cell.Kind() == references.DungeonLiveRoom {
				dungeon.generateWandererAfterRoom = true

				return
			}
			g.generateDungeonWanderer()
		})
	}
}

func (g *GameState) planDungeonHiddenPitFalls() []dungeonHiddenPitFallStep {
	position := g.MapState.PlayerLocation.Position
	floor := g.MapState.PlayerLocation.Floor
	cells := g.requireActiveDungeon().Snapshot().Cells
	steps := make([]dungeonHiddenPitFallStep, 0, references.DungeonFloorCount-int(floor))

	for {
		source, ok := cells.Cell(floor, position)
		if !ok || !isDungeonHiddenPit(source) {
			panic("dungeon hidden-pit plan lost its triggering source")
		}
		if !cells.SetCell(floor, position, references.DungeonCell(source.Raw()&0xf8)) {
			panic("dungeon hidden-pit source is out of bounds")
		}
		step := dungeonHiddenPitFallStep{sourceFloor: floor, destinationFloor: floor, bottomFloor: false}
		if floor == references.DungeonFloorCount-1 {
			// REDUX DEVIATION: ENV-0005: original dungeon rules increment level seven and reads undefined
			// a ninth dungeon floor. Redux reveals and damages once while clamping to the valid bottom floor.
			// See ref:ENV-0005, ref:DNG-0007.
			step.bottomFloor = true
			steps = append(steps, step)

			return steps
		}

		step.destinationFloor = floor + 1
		destination, ok := cells.Cell(step.destinationFloor, position)
		if !ok {
			panic("dungeon hidden-pit destination is out of bounds")
		}
		if destination.Raw() < 0x90 {
			destination = references.DungeonCell(destination.Raw() | 0x08)
			if !cells.SetCell(step.destinationFloor, position, destination) {
				panic("dungeon hidden-pit destination is out of bounds")
			}
		}
		steps = append(steps, step)
		floor = step.destinationFloor
		if !isDungeonHiddenPit(destination) {
			return steps
		}
	}
}

func (g *GameState) transformDungeonHiddenPitSource(
	floor references.FloorNumber,
	position references.Position,
) {
	_, err := g.TransformDungeonCell(floor, position, func(cell references.DungeonCell) references.DungeonCell {
		if !isDungeonHiddenPit(cell) {
			panic("dungeon hidden-pit source changed during workflow")
		}

		return references.DungeonCell(cell.Raw() & 0xf8)
	})
	if err != nil {
		panic(err)
	}
	g.recordDungeonDiscovery(dungeonDiscoveryHiddenPit, floor, position)
}

func (g *GameState) markDungeonHiddenPitDestination(
	floor references.FloorNumber,
	position references.Position,
) {
	_, err := g.TransformDungeonCell(floor, position, func(cell references.DungeonCell) references.DungeonCell {
		if cell.Raw() >= 0x90 {
			return cell
		}

		return references.DungeonCell(cell.Raw() | 0x08)
	})
	if err != nil {
		panic(err)
	}
}

func (g *GameState) isLivingActivePartyMember(index int) bool {
	character := g.PartyState.GetCharacter(index)

	return character.PartyMap == party_state.PartyMapActive && !character.IsDead()
}

func (g *GameState) damageDungeonPartyMember(index int) {
	if !g.isLivingActivePartyMember(index) {
		panic("dungeon pit damage requires a living active party member")
	}
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundWhiteNoiseDamage)
	g.PartyState.GetCharacter(index).SubtractHp(uint16(g.RollDice(8)))
}
