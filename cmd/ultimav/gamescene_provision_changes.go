package main

import (
	"image"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

type provisionChangePresentation struct {
	state      *game_state.GameState
	changes    mainscreen.ProvisionChanges
	panel      mainscreen.ProvisionChangePanel
	ammunition mainscreen.AmmunitionCounts
	location   references.PlayerLocation
	combat     *game_state.CombatState
	modalOpen  bool
	suppressed bool
}

func (g *GameScene) resetProvisionChanges() {
	g.provisionChanges = provisionChangePresentation{}
	if g.gameState != nil && !g.disposed {
		g.provisionChanges.state = g.gameState
		g.provisionChanges.changes.Reset(mainscreen.SnapshotProvisions(&g.gameState.PartyState.Inventory), g.gameState.ElapsedMs)
		g.provisionChanges.ammunition = mainscreen.SnapshotAmmunition(&g.gameState.PartyState.Inventory)
		g.provisionChanges.location = g.gameState.MapState.PlayerLocation
		g.provisionChanges.combat = g.gameState.CombatScene
		g.provisionChanges.modalOpen = g.dialogStack.PeekTopModalDialog() != nil
	}
}

// clearProvisionFeedback forgets receipts without changing ammunition observation.
// Call at explicit presentation boundaries, before the new interaction mutates inventory.
func (g *GameScene) clearProvisionFeedback() {
	if g.gameState == nil {
		return
	}
	if g.provisionChanges.state != g.gameState {
		g.resetProvisionChanges()
	}
	g.provisionChanges.changes.Reset(mainscreen.SnapshotProvisions(&g.gameState.PartyState.Inventory), g.gameState.ElapsedMs)
	width, height := display.GetManager().GetCurrentSize()
	g.layoutProvisionChanges(image.Rect(0, 0, width, height))
}

// observeProvisionChanges brackets scene updates, including early modal returns.
// Gross stack counters preserve transactions and consumption within the same
// update; no core callbacks, transaction ordering, RNG, or queue lanes are changed.
func (g *GameScene) observeProvisionChanges() {
	if g.disposed || g.gameState == nil {
		return
	}
	if g.provisionChanges.state != g.gameState {
		g.resetProvisionChanges()
	}
	location := g.gameState.MapState.PlayerLocation
	modalOpen := g.dialogStack.PeekTopModalDialog() != nil
	if location.Location != g.provisionChanges.location.Location || location.Floor != g.provisionChanges.location.Floor ||
		g.gameState.CombatScene != g.provisionChanges.combat || g.provisionChanges.suppressed ||
		g.atlasSession != nil || g.journalSession != nil || g.mapFadeLevel > 0 || g.finalePresentation != nil {
		g.clearProvisionFeedback()
	} else if modalOpen && !g.provisionChanges.modalOpen {
		// Discard old rows, but retain unobserved activity from the interaction
		// opening this modal (such as a purchase or gem use).
		g.provisionChanges.changes.ClearFeedback()
	}
	g.provisionChanges.location = location
	g.provisionChanges.combat = g.gameState.CombatScene
	g.provisionChanges.modalOpen = modalOpen
	// Obscured receipts expire normally; dismissal never grants them a new lifetime.
	g.provisionChanges.changes.Observe(mainscreen.SnapshotProvisions(&g.gameState.PartyState.Inventory), g.gameState.ElapsedMs)
	// Ammo deliberately observes final balances, not gross spent counters: an
	// atomic refill/recovery must not manufacture an intermediate shortage.
	current := mainscreen.SnapshotAmmunition(&g.gameState.PartyState.Inventory)
	for _, notice := range current.ShortageNotices(g.provisionChanges.ammunition) {
		g.ShowActionFeedback(notice, true)
	}
	g.provisionChanges.ammunition = current
	width, height := display.GetManager().GetCurrentSize()
	// GetCurrentSize already returns logical dimensions; refitting can round
	// them differently from Draw and rebuild the panel's fonts every frame.
	g.layoutProvisionChanges(image.Rect(0, 0, width, height))
}

func (g *GameScene) layoutProvisionChanges(screen image.Rectangle) {
	viewport := display.GetManager().MapPopupViewport(screen)
	if viewport.Empty() {
		viewport = screen
		viewport.Max.X = viewport.Min.X + viewport.Dx()*3/4
	}
	g.provisionChanges.panel.Update(g.provisionChanges.changes.Rows(), viewport, screen.Dx())
	bounds := g.provisionChanges.panel.Bounds()
	if bounds.Empty() {
		return
	}
	padding := max(2, viewport.Dx()/128)
	top := viewport.Min.Y + viewport.Dy()*7/10 - bounds.Dy()/2
	// Modal/merchant views and active action receipts reserve the lower map.
	if g.dialogStack.HasOpenDialog() || len(g.transientActionFeedback) > 0 {
		top = viewport.Min.Y + padding
	}
	top = max(viewport.Min.Y+padding, min(top, viewport.Max.Y-padding-bounds.Dy()))
	g.provisionChanges.panel.Place(image.Pt(viewport.Min.X+(viewport.Dx()-bounds.Dx())/2, top))
}

// provisionChangesAboveDialogs admits feedback above the active modal only when
// its measured bounds prove there is room. Merchant parents expose their shell;
// nested transaction dialogs and Dungeon View expose their own bounds. Unknown
// modal layouts conservatively render over the feedback instead.
func (g *GameScene) provisionChangesAboveDialogs() bool {
	modal := g.dialogStack.PeekTopModalDialog()
	if modal == nil {
		return true
	}
	var bounds image.Rectangle
	switch dialog := modal.(type) {
	case interface {
		GetShell() *widgets.MerchantDialogShell
	}:
		bounds = dialog.GetShell().GetBounds()
	case interface{ GetBounds() image.Rectangle }:
		bounds = dialog.GetBounds()
	default:
		return false
	}

	return !bounds.Empty() && !g.provisionChanges.panel.Bounds().Overlaps(bounds)
}
