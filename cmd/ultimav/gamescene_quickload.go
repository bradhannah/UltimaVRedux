package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
)

// captureQuickLoadRequest reserves modified Q even when another owner is active.
// Rejected chords never become plain Q or a deferred action. Both Q and its
// modifiers must be released before another chord can be admitted.
func (g *GameScene) captureQuickLoadRequest(inputAllowed bool) bool {
	sample := g.transientCommandInput.sample
	if g.quickLoadChordHeld {
		g.quickLoadChordHeld = sample.Held[ebiten.KeyQ] || sample.HasModifiers()
		g.keyboard.SuppressUntilRelease(ebiten.KeyQ)

		return false
	}
	if !sample.Held[ebiten.KeyQ] || !sample.HasModifiers() {
		return false
	}
	g.quickLoadChordHeld = true
	exact := g.keyboard.IsModifierPressed(input.ModifierCtrl) &&
		!g.keyboard.IsModifierPressed(input.ModifierShift) &&
		!g.keyboard.IsModifierPressed(input.ModifierAlt) &&
		!g.keyboard.IsModifierPressed(input.ModifierMeta)
	accepted := inputAllowed && exact && g.keyboard.ConsumePressEdge(sample, ebiten.KeyQ)
	g.keyboard.SuppressUntilRelease(ebiten.KeyQ)

	return accepted
}

// quickLoadInputEligible is an input-owner check, deliberately not snapshot
// eligibility. Combat and incidental labels/cards are safe to discard on load.
func (g *GameScene) quickLoadInputEligible() bool {
	state := g.gameState
	if g.disposed || g.transientInputOwnedElsewhere() || g.inputBlocked ||
		g.miniMapScene != nil || state.CampingScene != nil || state.ShrineScene != nil ||
		state.EventQueueManager.HasActiveWorkflows() || state.EventQueueManager.HasInputBlockingWorkflows() ||
		state.EventQueueManager.IsShutdown() || state.IsSailing() ||
		len(g.pendingBattleSummaries) != 0 || state.HasPendingOverworldContact() ||
		state.HasPendingDungeonRoomEncounter() || g.activeProjectile != nil || g.combatMovement.Active {
		return false
	}
	if combat := state.CombatScene; combat != nil {
		if !combat.CombatTurnsStarted() || combat.HasScheduledCombatStep() ||
			combat.AttackTargeting != nil || combat.SpellTargeting != nil ||
			combat.ConeTargeting != nil || combat.AdjacentSpellTargeting != nil {
			return false
		}
		actor := combat.GetCurrentTurnIdentity()
		if !actor.IsPartyMember {
			monster, _ := combat.GetControlledMonster()

			return monster != nil
		}

		return combat.IsCombatPCActive(actor) && !combat.IsCombatPCSleeping(actor) &&
			!combat.IsCombatPCUnderground(actor) && !combat.IsCombatPCPossessed(actor) &&
			!combat.IsPartyTurnAutoPass(actor.EntityIndex)
	}
	if state.IsWaterfallHazardActive() {
		return false
	}
	switch state.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType, references.LargeMapType:
		return true
	case references.DungeonMapType:
		return state.IsAtDungeonCommandBoundary()
	case references.CombatMapType, references.CampingMapType:
		return false
	default:
		return false
	}
}

// handleQuickLoad reloads the last-used slot, asking first when Confirm Quick
// Save is on, as Q does before it saves.
func (g *GameScene) handleQuickLoad(game *Game) {
	// REDUX DEVIATION: CMD-0002: the original Q save command has no direct
	// active-slot reload chord. Redux permits Ctrl+Q, including ready combat
	// turns, for quick retries without saving the outgoing adventure. Confirm
	// Quick Save asks first; with it off the reload is immediate.
	g.keyboard.PrepareForTransition(0)
	if game != nil && g.confirmQuickSave() && g.activePlayerSaveSlot != "" {
		g.askQuickLoad(game)

		return
	}
	g.loadLastSave(game)
}

func (g *GameScene) loadLastSave(game *Game) {
	if game == nil {
		g.gameState.SystemCallbacks.Message.AddRowStr("Load failed: " + playerQuickLoadErrorDetail(ErrPlayerSaveUnavailable))

		return
	}
	slot, err := game.quickLoadPlayerSave()
	if err != nil {
		logging.Warnf(logging.CategoryGameState, "Quick load failed: %v", err)
		g.gameState.SystemCallbacks.Message.AddRowStr("Load failed: " + playerQuickLoadErrorDetail(err))

		return
	}
	showPlayerSaveLoadSuccess(game, slot)
}

// quickLoadQuestion is the Y/N prompt that stands between Ctrl+Q and a reload.
// It takes the keys of the Save game? prompt: Y loads; N or Esc leaves the game as it is.
type quickLoadQuestion struct {
	*widgets.OverlayDialog
	scene *GameScene
	game  *Game
}

func (g *GameScene) askQuickLoad(game *Game) {
	dialog := widgets.NewOverlayDialog("Load last save?").AsConfirmation().
		WithPosition(widgets.PositionMapCenterLower).WithSizing(widgets.SizingCompact).
		WithExternallyManagedDuration(0).Build()
	dialog.AppendLineBuilder().Text("Unsaved play is lost.", widgets.DialogControlHintColor).Done()
	dialog.AppendLineBuilder().Text("[Y]", widgets.DialogControlKeyColor).
		Text(" Yes   ", widgets.DialogControlHintColor).Text("[N/Esc]", widgets.DialogControlKeyColor).
		Text(" No", widgets.DialogControlHintColor).Done()
	dialog.ShowWithoutAnimation()
	g.dialogStack.PushModalDialog(&quickLoadQuestion{OverlayDialog: dialog, scene: g, game: game})
}

// Update reads the answer.
func (q *quickLoadQuestion) Update() {
	q.OverlayDialog.Update()
	g := q.scene
	key := g.keyboard.GetBoundKeyPressed(&boundKeysGame)
	if key == nil || !g.keyboard.TryToRegisterKeyPress(*key) {
		return
	}
	if *key != ebiten.KeyY && *key != ebiten.KeyN && *key != ebiten.KeyEscape {
		return
	}
	g.dialogStack.MustRemoveWidgetInstance(q)
	g.keyboard.PrepareForTransition(0)
	if *key != ebiten.KeyY {
		g.gameState.SystemCallbacks.Message.AddRowStr("Load last save? No")

		return
	}
	g.gameState.SystemCallbacks.Message.AddRowStr("Load last save? Yes")
	g.loadLastSave(q.game)
}
