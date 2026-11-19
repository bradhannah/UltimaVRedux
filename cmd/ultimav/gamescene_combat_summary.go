package main

import (
	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Conclusions are captured after authoritative cleanup, but shown only after
// the owning exit/recovery workflow and any higher-priority modal have finished.
func (g *GameScene) showPendingBattleSummary() bool {
	if len(g.pendingBattleSummaries) == 0 || g.battleSummaryDialog != nil ||
		g.dialogStack.HasModalDialog() || g.gameState.EventQueueManager.HasActiveWorkflows() ||
		!g.gameState.CanAcceptGameplayCommands() || g.gameState.PartyState.IsAllPartyDead() {
		return false
	}
	conclusion := g.pendingBattleSummaries[0]
	g.pendingBattleSummaries[0] = game_state.CombatConclusion{}
	g.pendingBattleSummaries = g.pendingBattleSummaries[1:]
	g.lastBattleSummary = &conclusion
	g.showBattleSummary(conclusion)

	return true
}

func (g *GameScene) showBattleSummary(conclusion game_state.CombatConclusion) {
	if g.battleSummaryDialog != nil {
		panic("battle summary already owns a dialog")
	}
	if !g.gameConfig.GetShowBattleExperience() {
		conclusion.Experience = game_state.CombatExperienceSummary{}
	}
	var dialog *widgets.CombatLootDialog
	dialog = widgets.NewCombatSummaryDialog(conclusion, g.keyboard, g.spriteSheet, func() {
		g.dialogStack.MustRemoveWidgetInstance(dialog)
		g.battleSummaryDialog = nil
		g.keyboard.PrepareForTransition(0)
		widgets.AppendControlHints(g.output.AppendLine(), "[F4] Last battle").Done()
	})
	g.battleSummaryDialog = dialog
	g.dialogStack.PushModalDialog(dialog)
}

func (g *GameScene) handleBattleSummaryShortcut(key ebiten.Key) bool {
	if key != ebiten.KeyF4 {
		return false
	}
	if g.lastBattleSummary == nil {
		g.addRowStr("No completed battle to review.")

		return true
	}
	g.showBattleSummary(*g.lastBattleSummary)

	return true
}

func (g *GameScene) clearBattleSummaries() {
	if g.battleSummaryDialog != nil {
		g.dialogStack.RemoveWidgetInstance(g.battleSummaryDialog)
		g.battleSummaryDialog = nil
	}
	g.pendingBattleSummaries = nil
	g.lastBattleSummary = nil
}
