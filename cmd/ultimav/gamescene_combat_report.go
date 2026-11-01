package main

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

const combatReportEmphasisMs int64 = 2400

type combatReportNotice struct {
	remainingMs int64
	pending     bool
}

type combatReportSpan struct {
	slot, firstLine, lines int
}

func combatReportNoteworthy(event game_state.CombatEvent) bool {
	return event.Kind == game_state.CombatEventPickup || event.Kind == game_state.CombatEventChestTrap ||
		event.Kind == game_state.CombatEventLootRevealed
}

func (hud *CombatHUD) pendingResults() int {
	count := 0
	for _, notice := range hud.notices {
		if notice.pending {
			count++
		}
	}

	return count
}

func (hud *CombatHUD) reviewNextResult() {
	chosen := -1
	for index := range hud.historyCount {
		slot := (hud.historyStart + index) % len(hud.history)
		if hud.notices[slot].pending {
			chosen = index

			break
		}
		if combatReportNoteworthy(hud.history[slot]) {
			chosen = index
		}
	}
	if chosen < 0 {
		return
	}
	hud.historyOffset = hud.historyCount - chosen - 1
	if hud.historyOffset == 0 {
		hud.unreadEvents = 0
	}
	slot := (hud.historyStart + chosen) % len(hud.history)
	hud.notices[slot] = combatReportNotice{remainingMs: combatReportEmphasisMs, pending: false}
}

func (g *GameScene) appendCombatReportHeader() {
	hud := g.combatHUD
	hud.eventOutput.Reset()
	line := hud.eventOutput.AppendLine().Text("BATTLE REPORT", widgets.DialogHeaderColor).
		LightCyan(combatDetailHeaderSuffix(hud.detail))
	if hud.historyOffset > 0 {
		line.Text(" History", widgets.DialogLabelColor)
	}
	line.Done()
	if pending := hud.pendingResults(); pending > 0 {
		line = hud.eventOutput.AppendLine()
		widgets.AppendControlHints(line, fmt.Sprintf("[F3] Results (%d)", pending)).Done()
	}
	if hud.historyOffset > 0 {
		line = hud.eventOutput.AppendLine()
		widgets.AppendControlHints(line, "[End] Live")
		if hud.unreadEvents > 0 {
			line.Textf(" +%d", widgets.DialogValueColor, hud.unreadEvents)
		}
		line.Done()
	}
}

// Highlight budgets advance only while their complete entry is exposed. Neither
// a modal, party targeting, nor browsing elsewhere can spend its reading time.
func (g *GameScene) updateCombatReportAttention() {
	hud := g.combatHUD
	if hud == nil {
		return
	}
	now := g.gameState.ElapsedMs
	delta := max(int64(0), now-hud.attentionAt)
	hud.attentionAt = now
	if hud.activeCombat != g.gameState.CombatScene || g.gameState.CombatScene == nil ||
		hud.eventOutput == nil || g.dialogStack.HasModalDialog() || g.gameState.IsCombatPartySpellTargeting() {
		return
	}
	width, height := display.GetManager().GetCurrentSize()
	screen := image.Rect(0, 0, width, height)
	_, panel := combatHUDStackedRectangles(screen, combatHUDRectangle(screen))
	bounds := combatHUDContentBounds(screen, panel)
	spans := g.prepareCombatEventTape(bounds)
	capacity := hud.eventOutput.GetLineCapacity(bounds.Dy())
	for _, span := range spans {
		if span.firstLine+span.lines > capacity {
			continue
		}
		notice := &hud.notices[span.slot]
		notice.remainingMs = max(int64(0), notice.remainingMs-delta)
		if notice.remainingMs == 0 {
			notice.pending = false
		}
	}
}

func (g *GameScene) drawCombatReportEmphasis(screen *ebiten.Image, bounds image.Rectangle, spans []combatReportSpan) {
	hud := g.combatHUD
	unit := widgets.ClassicUIPixelUnit(screen.Bounds().Dx(), screen.Bounds().Dy())
	for _, span := range spans {
		remaining := hud.notices[span.slot].remainingMs
		if remaining <= 0 {
			continue
		}
		top := bounds.Min.Y + int(math.Round(float64(span.firstLine)*hud.eventOutput.GetLineSpacing()))
		bottom := min(bounds.Max.Y, top+int(math.Ceil(hud.eventOutput.GetTotalHeightForLines(span.lines))))
		if bottom <= top {
			continue
		}
		accent := ucolor.CombatReportLoot
		event := hud.history[span.slot]
		if event.Kind == game_state.CombatEventChestTrap ||
			(event.Kind == game_state.CombatEventPickup && event.Pickup.Result.Acquired == 0) {
			accent = ucolor.CombatReportWarning
		}
		alpha := float32(remaining) / float32(combatReportEmphasisMs)
		// The report has an opaque black backing. Shade RGB directly: passing
		// unpremultiplied bright RGBA to vector would wash out the text at low alpha.
		vector.FillRect(screen, float32(bounds.Min.X), float32(top), float32(bounds.Dx()), float32(bottom-top),
			ucolor.From(accent).Intensity(alpha*0.14).Opaque().RGBA(), false)
		vector.FillRect(screen, float32(bounds.Min.X-unit), float32(top), float32(unit), float32(bottom-top),
			ucolor.From(accent).Intensity(alpha).Opaque().RGBA(), false)
	}
}

func combatPickupText(event game_state.CombatPickupEvent) string {
	switch {
	case event.Result.Acquired > 0:
		return fmt.Sprintf("LOOT: %s collected %s +%d", event.ActorName, event.Result.ItemName, event.Result.Acquired)
	case event.Result.Succeeded:
		return "PICKUP: " + event.ActorName + ": " + event.Result.Message
	default:
		return "PICKUP FAILED: " + event.ActorName + ": " + event.Result.Message
	}
}

func combatLootRevealText(reveal game_state.CombatLootReveal) string {
	if reveal.Description == "" {
		return "The chest is empty."
	}

	return reveal.Description + " On the ground."
}

func (g *GameScene) appendCombatPickup(event game_state.CombatPickupEvent) {
	line := g.newCombatEventLine()
	if event.Result.Acquired == 0 {
		label, labelColor := "PICKUP FAILED: ", ucolor.CombatReportWarning
		if event.Result.Succeeded {
			// A borrowed wall torch changes the map and the party's light
			// without adding stock, so it is a pickup, not a failure.
			label, labelColor = "PICKUP: ", ucolor.CombatReportLoot
		}
		line.Text(label, labelColor).
			Text(event.ActorName+": ", widgets.DialogLabelColor).Text(event.Result.Message, widgets.DialogValueColor).Done()

		return
	}
	line.Text("LOOT: ", ucolor.CombatReportLoot).Text(event.ActorName, widgets.DialogLabelColor).
		White(" collected ").Text(event.Result.ItemName, widgets.DialogValueColor).
		Textf(" +%d", ucolor.CombatReportLoot, event.Result.Acquired).Done()
}

func combatChestTrapText(result game_state.CombatChestTrapResult) string {
	switch result.Type { //nolint:exhaustive // Combat chests admit acid and poison only.
	case game_state.TrapAcid:
		if result.Fatal {
			return fmt.Sprintf("%s took %d acid damage and died!", result.ActorName, result.HPLost)
		}

		return fmt.Sprintf("%s took %d acid damage!", result.ActorName, result.HPLost)
	case game_state.TrapPoison:
		return result.ActorName + " was poisoned!"
	default:
		return ""
	}
}
