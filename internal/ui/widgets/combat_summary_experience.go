package widgets

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const (
	combatSummaryExperienceColumns = 2
	combatSummaryExperienceHeading = "Experience"
)

// combatSummaryExperience is the Battle complete summary's experience section:
// a heading, then every member's share in two columns, or one line when an even
// split paid everyone the same. A member whose level is waiting is named below.
type combatSummaryExperience struct {
	summary        game_state.CombatExperienceSummary
	heading, each  *text.Output
	names, amounts []*text.Output
	ready          []*text.Output
	rowHeight      int
	height         int
}

func (e *combatSummaryExperience) initialize(template *text.Output, innerWidth, padding int) {
	e.heading, e.each, e.names, e.amounts, e.ready, e.rowHeight, e.height = nil, nil, nil, nil, nil, 0, 0
	if e.summary.Total() == 0 {
		return
	}
	e.heading = combatSummaryText(combatSummaryExperienceHeading, DialogLabelColor, template, innerWidth)
	// Half a padding between rows keeps the lines from touching.
	e.rowHeight = combatSummaryTextHeight(e.heading) + max(1, padding/combatSummaryExperienceColumns)
	e.height = e.rowHeight
	if e.summary.Shared {
		each := fmt.Sprintf("+%d each", e.summary.Each)
		if len(e.summary.Members) == 1 {
			each = fmt.Sprintf("+%d", e.summary.Each)
		}
		e.each = combatSummaryText(each, ucolor.ExperienceGain, template, innerWidth)
	} else {
		cell := e.cellWidth(innerWidth, padding)
		for _, member := range e.summary.Members {
			e.names = append(e.names, combatSummaryText(member.Name, DialogValueColor, template, cell))
			amount, colour := "-", DialogItemQuantityColor
			if member.Amount > 0 {
				amount, colour = fmt.Sprintf("+%d", member.Amount), ucolor.ExperienceGain
			}
			e.amounts = append(e.amounts, combatSummaryText(amount, colour, template, cell))
		}
		e.height += e.rowHeight * e.gridRows()
	}
	for _, ready := range e.summary.Ready {
		line := combatSummaryText(fmt.Sprintf("%s is ready for level %d.", ready.Name, ready.Level),
			ucolor.ExperienceLevelReady, template, innerWidth)
		e.ready = append(e.ready, line)
		e.height += combatSummaryTextHeight(line) + e.rowHeight - combatSummaryTextHeight(e.heading)
	}
	e.height += padding
}

func (e *combatSummaryExperience) gridRows() int {
	return (len(e.names) + combatSummaryExperienceColumns - 1) / combatSummaryExperienceColumns
}

// cellWidth leaves a gap of two paddings between the columns.
func (e *combatSummaryExperience) cellWidth(innerWidth, padding int) int {
	return max(1, (innerWidth-padding*lootSides)/combatSummaryExperienceColumns)
}

func (e *combatSummaryExperience) draw(screen *ebiten.Image, origin image.Point, innerWidth, padding int) {
	if e.heading == nil {
		return
	}
	e.heading.DrawPanelContent(screen, origin, innerWidth, etext.AlignStart)
	if e.each != nil {
		e.each.DrawPanelContent(screen, origin, innerWidth, etext.AlignEnd)
	}
	top := origin.Y + e.rowHeight
	cell := e.cellWidth(innerWidth, padding)
	for index := range e.names {
		position := image.Pt(
			origin.X+(index%combatSummaryExperienceColumns)*(cell+padding*lootSides),
			top+(index/combatSummaryExperienceColumns)*e.rowHeight,
		)
		e.names[index].DrawPanelContent(screen, position, cell, etext.AlignStart)
		e.amounts[index].DrawPanelContent(screen, position, cell, etext.AlignEnd)
	}
	top += e.rowHeight * e.gridRows()
	for _, line := range e.ready {
		line.DrawPanelContent(screen, image.Pt(origin.X, top), innerWidth, etext.AlignStart)
		top += combatSummaryTextHeight(line) + e.rowHeight - combatSummaryTextHeight(e.heading)
	}
}
