package widgets

import (
	"image"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func dungeonViewSampleNames() []string {
	return []string{"simple", "dense", "partly-revealed", "wrapping", "all-wrapping"}
}

func dungeonViewTestFacings() []references.DungeonFacing {
	return []references.DungeonFacing{
		references.DungeonFacingNorth, references.DungeonFacingEast,
		references.DungeonFacingSouth, references.DungeonFacingWest,
	}
}

func dungeonViewSamplePlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{StartPercentX: 0.03, EndPercentX: 0.97, StartPercentY: 0.04, EndPercentY: 0.96}
}

// Synthetic display observations exercise presentation independently of original
// data. They are deliberately not offered as real-floor or flood acceptance.
func dungeonViewSampleModel(name string) game_state.DungeonViewModel {
	var source [8][8]references.DungeonCell
	for coordinate := range 8 {
		source[0][coordinate], source[7][coordinate] = 0xb0, 0xb0
		source[coordinate][0], source[coordinate][7] = 0xb0, 0xb0
	}
	source[1][1], source[5][5], source[4][2] = 0x10, 0x20, 0x4f
	source[3][2], source[3][3], source[3][4] = 0xb0, 0xe0, 0xb0
	if name != "simple" {
		features := []references.DungeonCell{
			0x10, 0x20, 0x30, 0x4f, 0x50, 0x08, 0x60, 0x61, 0x68, 0x69, 0x62, 0x83, 0xa0, 0xb1, 0xd0, 0xe0,
		}
		for index, cell := range features {
			source[1+index/6][1+index%6] = cell
		}
		source[4][1], source[4][2], source[4][3] = 0xc0, 0xc1, 0xc8
	}
	if name == "wrapping" {
		source[3][0], source[3][7] = 0x00, 0x60
		source[0][6], source[7][6] = 0x00, 0x80
	}
	if name == "all-wrapping" {
		for coordinate := range 8 {
			source[0][coordinate], source[7][coordinate] = 0x00, 0x00
			source[coordinate][0], source[coordinate][7] = 0x00, 0x00
		}
	}
	model := game_state.DungeonViewModel{Location: references.Deceit, Floor: 2, Facing: references.DungeonFacingEast}
	for displayY := range game_state.DungeonViewSize {
		for displayX := range game_state.DungeonViewSize {
			sourceX, sourceY := wrappedDungeonViewAxis(displayX-8), wrappedDungeonViewAxis(displayY-8)
			visible := true
			if name == "partly-revealed" {
				visible = sourceX < 5 && sourceY < 5
			}
			model.Cells[displayY][displayX] = game_state.DungeonViewCell{
				Visible: visible, Player: displayX == 11 && displayY == 11,
				SourcePosition: references.Position{X: references.Coordinate(sourceX), Y: references.Coordinate(sourceY)},
				SourceCell:     source[sourceY][sourceX],
			}
			if visible {
				model.VisibleCount++
			}
		}
	}
	return model
}

// Geometry-only fixture: no screen-sized render targets are allocated by the
// resolution matrix. Raster checks use small native symbols separately.
func dungeonViewLayoutFixture(size image.Point, model game_state.DungeonViewModel) *DungeonViewDialog {
	plan := buildDungeonViewRenderPlan(model)
	dialog := &DungeonViewDialog{
		model: model, plan: plan, legend: dungeonViewLegendFor(plan, model.Facing),
		continuations: dungeonViewContinuations(plan), placement: dungeonViewSamplePlacement(),
	}
	dialog.layOut(size.X, size.Y)
	return dialog
}

// dungeonViewFeatureKey lists every feature entry of the key, across its groups.
func dungeonViewFeatureKey() []dungeonViewLegendEntry {
	var entries []dungeonViewLegendEntry
	for _, group := range dungeonViewFeatureGroups() {
		entries = append(entries, group.entries...)
	}
	return entries
}
