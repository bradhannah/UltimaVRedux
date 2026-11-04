package widgets

import (
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const dungeonViewFloorCells = 8

type dungeonViewSurface uint8

const (
	dungeonViewSurfaceNone dungeonViewSurface = iota
	dungeonViewSurfaceFloor
	dungeonViewSurfaceWall
)

type dungeonViewIcon uint8

const (
	dungeonViewIconNone dungeonViewIcon = iota
	dungeonViewIconCeilingPit
	dungeonViewIconLadderUp
	dungeonViewIconLadderDown
	dungeonViewIconLadderBoth
	dungeonViewIconChest
	dungeonViewIconFountain
	dungeonViewIconOpenPit
	dungeonViewIconHiddenPit
	dungeonViewIconPitCeiling
	dungeonViewIconHiddenPitCeiling
	dungeonViewIconTrap
	dungeonViewIconField
	dungeonViewIconRoom
	dungeonViewIconPlaque
	dungeonViewIconHiddenDoor
	dungeonViewIconDoor
)

type dungeonViewAxis uint8

const (
	dungeonViewAxisHorizontal dungeonViewAxis = iota
	dungeonViewAxisVertical
)

type dungeonViewCellPlan struct {
	visible bool
	player  bool
	surface dungeonViewSurface
	icon    dungeonViewIcon
}

type dungeonViewKnownEdges struct {
	east  [dungeonViewFloorCells][dungeonViewFloorCells]bool
	south [dungeonViewFloorCells][dungeonViewFloorCells]bool
}

type dungeonViewRenderPlan struct {
	cells [dungeonViewFloorCells][dungeonViewFloorCells]dungeonViewCellPlan
	edges dungeonViewKnownEdges
}

type dungeonViewLegendEntry struct {
	label string
	plan  dungeonViewCellPlan
}

//nolint:cyclop,funlen,mnd,exhaustruct_v5 // Packed cell variants are presentation input, not new disclosure authority.
func dungeonViewCellPlanFor(cell references.DungeonCell) dungeonViewCellPlan {
	plan := dungeonViewCellPlan{visible: true, surface: dungeonViewSurfaceFloor}
	switch cell.Kind() {
	case references.DungeonPassage:
		if cell.HasBit08() {
			plan.icon = dungeonViewIconCeilingPit
		}
	case references.DungeonLadderUp:
		plan.icon = dungeonViewIconLadderUp
	case references.DungeonLadderDown:
		plan.icon = dungeonViewIconLadderDown
	case references.DungeonLadderBoth:
		plan.icon = dungeonViewIconLadderBoth
	case references.DungeonClosedChest:
		plan.icon = dungeonViewIconChest
	case references.DungeonFountain:
		plan.icon = dungeonViewIconFountain
	case references.DungeonPitOrTrap:
		switch cell.Raw() {
		case 0x60:
			plan.icon = dungeonViewIconOpenPit
		case 0x61:
			plan.icon = dungeonViewIconHiddenPit
		case 0x68:
			plan.icon = dungeonViewIconPitCeiling
		case 0x69:
			plan.icon = dungeonViewIconHiddenPitCeiling
		default:
			plan.icon = dungeonViewIconTrap
		}
	case references.DungeonOpenChest, references.DungeonReserved:
	case references.DungeonEnergyField:
		plan.icon = dungeonViewIconField
	case references.DungeonClearedRoom, references.DungeonLiveRoom:
		plan.icon = dungeonViewIconRoom
	case references.DungeonWall:
		plan.surface = dungeonViewSurfaceWall
		if cell.Raw() != 0xb0 {
			plan.icon = dungeonViewIconPlaque
		}
	case references.DungeonSpecialWall:
		// Stalactites, skeletons and cave-ins retain their blocking wall, without decoration.
		plan.surface = dungeonViewSurfaceWall
	case references.DungeonHiddenDoor:
		plan.surface = dungeonViewSurfaceWall
		plan.icon = dungeonViewIconHiddenDoor
	case references.DungeonRevealedDoor:
		plan.icon = dungeonViewIconDoor
	}

	return plan
}

// The order is independent of encounter order. Only revealed, collapsed symbols
// select entries; raw contents of unknown cells never participate.
//
//nolint:mnd // Representative packed cells identify the shared symbol definitions.
func dungeonViewFeatureKey() []dungeonViewLegendEntry {
	return []dungeonViewLegendEntry{
		{label: "Doorway", plan: dungeonViewCellPlanFor(0xe0)},
		{label: "Secret door (wall)", plan: dungeonViewCellPlanFor(0xd0)},
		{label: "Room entrance", plan: dungeonViewCellPlanFor(0xa0)},
		{label: "Ladder up", plan: dungeonViewCellPlanFor(0x10)},
		{label: "Ladder down", plan: dungeonViewCellPlanFor(0x20)},
		{label: "Ladder both ways", plan: dungeonViewCellPlanFor(0x30)},
		{label: "Ceiling opening", plan: dungeonViewCellPlanFor(0x08)},
		{label: "Open pit", plan: dungeonViewCellPlanFor(0x60)},
		{label: "Hidden pit", plan: dungeonViewCellPlanFor(0x61)},
		{label: "Pit + ceiling opening", plan: dungeonViewCellPlanFor(0x68)},
		{label: "Hidden pit + ceiling", plan: dungeonViewCellPlanFor(0x69)},
		{label: "Trap / bomb", plan: dungeonViewCellPlanFor(0x62)},
		{label: "Energy field", plan: dungeonViewCellPlanFor(0x80)},
		{label: "Chest", plan: dungeonViewCellPlanFor(0x4f)},
		{label: "Fountain", plan: dungeonViewCellPlanFor(0x50)},
		{label: "Wall writing", plan: dungeonViewCellPlanFor(0xb1)},
	}
}

//nolint:exhaustruct_v5,mnd // The unknown key has no disclosed surface or contents.
func dungeonViewLegendFor(
	plan dungeonViewRenderPlan,
	facing references.DungeonFacing,
) []dungeonViewLegendEntry {
	player := dungeonViewCellPlanFor(0x00)
	player.player = true
	entries := []dungeonViewLegendEntry{
		{label: "You: facing " + strings.ToLower(facing.Direction().GetDirectionCompassName()), plan: player},
		{label: "Solid wall", plan: dungeonViewCellPlanFor(0xb0)},
		{label: "Passage", plan: dungeonViewCellPlanFor(0x00)},
		{label: "Not revealed", plan: dungeonViewCellPlan{}},
	}
	shown := make(map[dungeonViewIcon]bool)
	for _, row := range plan.cells {
		for _, cell := range row {
			if cell.visible {
				shown[cell.icon] = true
			}
		}
	}
	for _, entry := range dungeonViewFeatureKey() {
		if shown[entry.plan.icon] {
			entries = append(entries, entry)
		}
	}

	return entries
}

func buildDungeonViewRenderPlan(model game_state.DungeonViewModel) dungeonViewRenderPlan {
	var plan dungeonViewRenderPlan
	for displayY := range game_state.DungeonViewSize {
		for displayX := range game_state.DungeonViewSize {
			cell := model.Cells[displayY][displayX]
			if !cell.Visible {
				continue
			}
			sourceX, sourceY := dungeonViewSourceCoordinates(cell)
			if !plan.cells[sourceY][sourceX].visible {
				plan.cells[sourceY][sourceX] = dungeonViewCellPlanFor(cell.SourceCell)
			}
			plan.cells[sourceY][sourceX].player = plan.cells[sourceY][sourceX].player || cell.Player
			if dungeonViewNeighborIsKnown(model, displayX, displayY, 1, 0) {
				plan.edges.east[sourceY][sourceX] = true
			}
			if dungeonViewNeighborIsKnown(model, displayX, displayY, 0, 1) {
				plan.edges.south[sourceY][sourceX] = true
			}
		}
	}

	return plan
}

func dungeonViewSourceCoordinates(cell game_state.DungeonViewCell) (int, int) {
	sourceX, sourceY := int(cell.SourcePosition.X), int(cell.SourcePosition.Y)
	if sourceX < 0 || sourceX >= dungeonViewFloorCells || sourceY < 0 || sourceY >= dungeonViewFloorCells {
		panic("visible dungeon view cell has invalid source coordinates")
	}

	return sourceX, sourceY
}

func wrappedDungeonViewAxis(value int) int {
	value %= dungeonViewFloorCells
	if value < 0 {
		value += dungeonViewFloorCells
	}

	return value
}

func dungeonViewNeighborIsKnown(model game_state.DungeonViewModel, displayX, displayY, offsetX, offsetY int) bool {
	neighborX, neighborY := displayX+offsetX, displayY+offsetY
	if neighborX < 0 || neighborX >= game_state.DungeonViewSize ||
		neighborY < 0 || neighborY >= game_state.DungeonViewSize {
		return false
	}
	current, neighbor := model.Cells[displayY][displayX], model.Cells[neighborY][neighborX]
	if !current.Visible || !neighbor.Visible {
		return false
	}

	return int(neighbor.SourcePosition.X) == wrappedDungeonViewAxis(int(current.SourcePosition.X)+offsetX) &&
		int(neighbor.SourcePosition.Y) == wrappedDungeonViewAxis(int(current.SourcePosition.Y)+offsetY)
}

// A known seam describes observation, not reachability or safety. Hazards can be
// endpoints. A hidden door remains a wall and never advertises a passage.
func dungeonViewHasContinuation(
	plan dungeonViewRenderPlan,
	axis dungeonViewAxis,
	sourceIndex int,
) bool {
	if sourceIndex < 0 || sourceIndex >= dungeonViewFloorCells {
		return false
	}
	var first, second dungeonViewCellPlan
	var known bool
	switch axis {
	case dungeonViewAxisHorizontal:
		first, second = plan.cells[sourceIndex][dungeonViewFloorCells-1], plan.cells[sourceIndex][0]
		known = plan.edges.east[sourceIndex][dungeonViewFloorCells-1]
	case dungeonViewAxisVertical:
		first, second = plan.cells[dungeonViewFloorCells-1][sourceIndex], plan.cells[0][sourceIndex]
		known = plan.edges.south[dungeonViewFloorCells-1][sourceIndex]
	default:
		panic("invalid dungeon view continuation axis")
	}

	return known && first.visible && second.visible &&
		first.surface != dungeonViewSurfaceWall && second.surface != dungeonViewSurfaceWall
}

type dungeonViewContinuation struct {
	axis  dungeonViewAxis
	index int
	label string
}

func dungeonViewContinuations(plan dungeonViewRenderPlan) []dungeonViewContinuation {
	var labels []dungeonViewContinuation
	for _, axis := range []dungeonViewAxis{dungeonViewAxisHorizontal, dungeonViewAxisVertical} {
		prefix := "W-E"
		if axis == dungeonViewAxisVertical {
			prefix = "N-S"
		}
		for index := range dungeonViewFloorCells {
			if dungeonViewHasContinuation(plan, axis, index) {
				labels = append(labels, dungeonViewContinuation{
					axis: axis, index: index, label: fmt.Sprintf("%s%d", prefix, index+1),
				})
			}
		}
	}

	return labels
}
