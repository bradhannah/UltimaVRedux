package main

import (
	"fmt"
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func (g *GameScene) combatAttackPreview() game_state.CombatAttackPreview {
	frame := &g.combatRenderFrame
	if !frame.active {
		return g.computeCombatAttackPreview()
	}
	if !frame.previewReady {
		frame.preview = g.computeCombatAttackPreview()
		frame.previewReady = true
	}

	return frame.preview
}

func (g *GameScene) computeCombatAttackPreview() game_state.CombatAttackPreview {
	combat := g.gameState.CombatScene
	if combat == nil || !combat.CombatTurnsStarted() || g.inputBlocked ||
		(g.secondaryKeyState != PrimaryInput && g.secondaryKeyState != CombatAttackTargetInput) ||
		combatTurnIsAutoPass(combat, combat.GetCurrentTurnIdentity()) {
		return game_state.CombatAttackPreview{}
	}

	return g.gameState.PreviewCombatAttack()
}

func (g *GameScene) drawCombatAttackEligibility(screen *ebiten.Image) {
	preview := g.combatAttackPreview()
	for _, target := range preview.Targets {
		if target.Actor == nil {
			continue // Assistance legality does not confer renderer disclosure.
		}
		if preview.Aim != nil && target.Position == preview.Aim.Position {
			continue // The existing aiming reticle owns the selected square.
		}
		drawCombatAttackBrackets(screen, g.combatEntityDrawOptions(target.Position, false, target.MonsterIndex, 0))
	}
}

// Approved foreground foot cues stay inside each 16-pixel cell. Their complete
// backing ends before the health-frame outline at y=12 and remains inward of flee/status
// marks. Draw after every actor, but before the aiming reticle, so neighboring
// sprites cannot erase either two-pixel arm or its one-pixel black border.
func combatAttackBracketFaces() [4]image.Rectangle {
	return [4]image.Rectangle{
		image.Rect(5, 9, 6, 11), image.Rect(5, 10, 7, 11),
		image.Rect(11, 9, 12, 11), image.Rect(10, 10, 12, 11),
	}
}

func drawCombatAttackBrackets(screen *ebiten.Image, options *ebiten.DrawImageOptions) {
	faces := combatAttackBracketFaces()
	for _, border := range []bool{true, false} {
		for _, rect := range faces {
			clr := ucolor.CombatAttackEligibleFace
			if border {
				rect = rect.Inset(-1)
				clr = ucolor.CombatAttackEligibleBorder
			}
			left, top := options.GeoM.Apply(float64(rect.Min.X), float64(rect.Min.Y))
			right, bottom := options.GeoM.Apply(float64(rect.Max.X), float64(rect.Max.Y))
			left, top = math.Round(left), math.Round(top)
			right, bottom = math.Round(right), math.Round(bottom)
			vector.FillRect(screen, float32(left), float32(top), float32(right-left), float32(bottom-top), clr, false)
		}
	}
}

// combatReadoutRow is one row of the Turn Order attack readout. Text sits at
// the left; a weapon's reach or a target's distance sits at the right edge.
type combatReadoutRow struct {
	label string // The verdict's label, in the label colour.
	text  string // Cut plainly when the row is too narrow.
	value string // Reach (R) or distance (D).
}

// appendCombatAttackReadoutRows lists each attack source with its own reach
// and, while aiming, the target with its distance and then a verdict.
func appendCombatAttackReadoutRows(rows []combatReadoutRow, preview game_state.CombatAttackPreview) []combatReadoutRow {
	for _, source := range preview.Sources {
		rows = append(rows, combatReadoutRow{
			label: "", text: combatReadoutEquipmentName(source.Equipment), value: fmt.Sprintf("R%d", source.MaxRange),
		})
	}
	if preview.Aim == nil {
		return rows
	}
	target := preview.Aim.Name
	if target == "" {
		target = "No one seen"
	}
	if preview.Aim.Status == game_state.CombatShotChooseTarget {
		target = "Choose target"
	}
	rows = append(rows, combatReadoutRow{label: "", text: target, value: fmt.Sprintf("D%d", preview.Aim.Distance)})
	label := combatAttackActionLabel
	if preview.Aim.Actor != nil {
		label = combatAllegianceText(*preview.Aim.Actor)
	}

	return append(rows, combatReadoutRow{label: label + ": ", text: combatShotStatusText(preview.Aim.Status), value: ""})
}

// combatReadoutEquipmentName is a weapon's name as the narrow readout shows it:
// "Two-Handed" becomes "2H", so those weapons fit their row whole.
func combatReadoutEquipmentName(equipment references.Equipment) string {
	if equipment == references.NoEquipment {
		return "Bare Hands"
	}
	if name, twoHanded := strings.CutPrefix(equipment.String(), "Two-Handed "); twoHanded {
		return "2H " + name
	}

	return equipment.String()
}

// combatReadoutRowText is a row's left-hand text, cut so the row fits width
// with its value at the right edge.
func combatReadoutRowText(font *text.UltimaFont, row combatReadoutRow, width int) string {
	reserved := row.label
	if row.value != "" {
		reserved += " " + row.value
	}
	reservedWidth, _ := font.MeasureString(reserved)

	return widgets.CutDialogLine(font, row.text, "", width-int(math.Ceil(reservedWidth)))
}

// drawCombatAttackReadout draws the readout rows from origin and returns the
// height they used: text from the left, then reach and distance from the right.
func (g *GameScene) drawCombatAttackReadout(screen *ebiten.Image, origin image.Point, width int) int {
	hud := g.combatHUD
	hud.readout = appendCombatAttackReadoutRows(hud.readout[:0], g.combatAttackPreview())
	if len(hud.readout) == 0 {
		return 0
	}
	output := hud.turnOutput
	output.Reset()
	output.SetMaxCharsPerLine(combatHUDOutputChars)
	for _, row := range hud.readout {
		line := output.AppendLine()
		if row.label != "" {
			line.Text(row.label, widgets.DialogLabelColor)
		}
		line.Text(combatReadoutRowText(output.Font, row, width), widgets.DialogValueColor).Done()
	}
	output.DrawPanelContent(screen, origin, width, etext.AlignStart)
	height := int(math.Ceil(output.GetTotalHeightForLines(output.GetPanelContentLineCount())))
	output.Reset()
	for _, row := range hud.readout {
		output.AppendLine().LightGray(row.value).Done()
	}
	output.DrawPanelContent(screen, origin, width, etext.AlignEnd)

	return height
}

func combatShotStatusText(status game_state.CombatShotStatus) string {
	if status == game_state.CombatShotChooseTarget {
		return "Aim"
	}
	if status == game_state.CombatShotTerrainBypass {
		return "Bypass"
	}
	if status == game_state.CombatShotOutOfRange {
		return "Too far"
	}
	if status == game_state.CombatShotInterference {
		return "Engaged"
	}
	if status == game_state.CombatShotUnready {
		return "Unready"
	}

	return string(status)
}
