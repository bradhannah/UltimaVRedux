package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
)

type runicReferenceState struct {
	open               bool
	shortcutHeld       bool
	translationHeld    bool
	translationHandled bool
	owner              widgets.Widget
	panel              widgets.RunicReferencePanel
}

func (g *GameScene) runicReferenceContextAllowed() bool {
	if g.gameState.CombatScene != nil || g.gameState.IsInCombat() || g.finalePresentation != nil || g.miniMapScene != nil ||
		g.journalSession != nil || g.usesCinematicSplitLayout() {
		return false
	}
	switch g.gameState.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType, references.LargeMapType, references.DungeonMapType:
		return true
	case references.CombatMapType, references.CampingMapType:
		return false
	default:
		return false
	}
}

func (g *GameScene) runicReferenceAvailable() bool {
	if !g.runicReferenceContextAllowed() {
		return false
	}
	switch g.dialogStack.PeekTopModalDialog().(type) {
	case *LinearTalkDialog, *widgets.SignDialog:
		// These reading workflows deliberately block ordinary gameplay input.
		return true
	case nil:
		return !g.inputBlocked && g.gameState.CanAcceptGameplayCommands()
	default:
		return false
	}
}

// syncRunicReference observes exact dialog ownership rather than owning or
// dismissing the reading workflow. Other modals temporarily hide the reference.
func (g *GameScene) syncRunicReference() {
	state := &g.runicReference
	if !state.open {
		return
	}
	if !g.runicReferenceContextAllowed() ||
		(state.owner != nil && !g.dialogStack.ContainsWidgetInstance(state.owner)) {
		g.closeRunicReference()

		return
	}
	if state.owner == nil && g.runicReferenceAvailable() {
		state.owner = g.dialogStack.PeekTopModalDialog()
	}
}

// handleRunicReferenceShortcut runs before modal input and ordinary R routing.
// Consume the chord even when unavailable so it never Readies equipment or
// dismisses a press-any-key prompt. Release consumption also protects against
// releasing Ctrl before R, and against repeated toggling while the chord is held.
func (g *GameScene) handleRunicReferenceShortcut() bool {
	if _, journalOpen := g.dialogStack.PeekTopModalDialog().(*widgets.QuestJournalDialog); journalOpen {
		// The Journal places the same passive reference beside its retained text.
		return false
	}
	if g.keyboard.IsKeyPressedAndRegister(widgets.RunicReferenceFunctionKey) {
		g.toggleRunicReference()

		return true
	}
	if !g.keyboard.IsBoundKeyPressed(&[]ebiten.Key{ebiten.KeyR}) {
		g.runicReference.shortcutHeld = false

		return false
	}
	if g.runicReference.shortcutHeld {
		return true
	}
	if !g.keyboard.IsModifierComboPressed(input.ModifierCtrl, ebiten.KeyR) {
		return false
	}
	g.runicReference.shortcutHeld = true
	if g.keyboard.IsKeyPressedAndRegister(ebiten.KeyR) {
		g.toggleRunicReference()
	}

	return true
}

func (g *GameScene) toggleRunicReference() {
	if !g.runicReferenceAvailable() {
		return
	}
	if g.runicReference.open {
		g.closeRunicReference()
	} else {
		g.runicReference.open = true
		g.runicReference.owner = g.dialogStack.PeekTopModalDialog()
	}
}

func (g *GameScene) drawRunicReference(screen *ebiten.Image) {
	if g.runicReference.open && g.runicReferenceAvailable() {
		g.runicReference.panel.Draw(screen, calculateRightRailLayout(screen.Bounds()).railBorder,
			g.output.Font, g.output.GetLineSpacing())
	}
}
