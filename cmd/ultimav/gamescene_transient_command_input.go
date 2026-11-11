package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
)

// transientCommandInputState owns presentation/input intent, never gameplay.
// The zero value supports detached scenes; production installs the focus source.
type transientCommandInputState struct {
	owner             *game_state.GameState
	focused           func() bool
	wasUnfocused      bool
	sample            input.KeySample
	pass              explorationPassRepeat
	combatRepeat      combatKeyRepeat
	pending           pendingCombatCommand
	campAtEntry       *camping.CampingScene
	campRequest       bool
	combatWaitSampled bool
}

func (g *GameScene) initializeTransientCommandInput() {
	g.transientCommandInput.focused = ebiten.IsFocused
	// Keep the keyboard's independent adapter clock. Journal/Atlas freeze game
	// elapsed time, but their fresh key presses must retain normal debounce timing.
	// A newly hydrated/replacement scene must not reinterpret the key that
	// activated it as its own first gameplay press.
	g.ResetTransientCommandInput()
}

func (g *GameScene) commandInputFocused() bool {
	source := g.transientCommandInput.focused

	return source == nil || source()
}

// beginTransientCommandInputFrame is the sole edge sample, before any reader or
// queue advances. Capturing an edge does not steal input from an earlier owner.
func (g *GameScene) beginTransientCommandInputFrame() bool {
	state := &g.transientCommandInput
	if state.owner != nil && state.owner != g.gameState {
		g.ResetTransientCommandInput()
	}
	state.owner = g.gameState
	state.sample = g.keyboard.CaptureKeySample()
	state.campAtEntry = nil
	state.campRequest = false
	state.combatWaitSampled = false
	if !g.commandInputFocused() {
		state.wasUnfocused = true
		g.ResetTransientCommandInput()
		// Leaving the window is a safe pause: the ship must not sail on unattended.
		if g.gameState.IsSailing() {
			g.gameState.CompleteAutomaticSailingInterruption()
		}

		return false
	}
	if state.wasUnfocused {
		state.wasUnfocused = false
		g.ResetTransientCommandInput()

		return false
	}
	if !g.explorationPassEligible() || state.pass.active && state.pass.context != g.passInputContext() {
		g.stopExplorationPass()
	}
	g.observeCombatKeyRepeat()

	return true
}

// captureTransientCommandInput runs only after reader shortcuts have had their
// opportunity, but before wind/queues can turn a blocked sample into ready input.
func (g *GameScene) captureTransientCommandInput() {
	if g.quickLoadChordHeld {
		return
	}
	if g.transientInputOwnedElsewhere() {
		g.clearPendingCombatCommand()

		return
	}
	if g.captureCombatSoloReset() {
		return
	}
	g.capturePendingCombatCommand()
	state := &g.transientCommandInput
	state.campAtEntry = g.sleepingInputCamp()
	if state.campAtEntry != nil && !state.sample.HasModifiers() &&
		g.keyboard.ConsumePressEdge(state.sample, ebiten.KeySpace) {
		state.campRequest = true
		g.keyboard.SuppressUntilRelease(ebiten.KeySpace)
	}
}

// handleCampingAcceleration runs after queue advancement and top-modal routing.
// A completion/ambush in this frame must not accept a stale sleeping request.
func (g *GameScene) handleCampingAcceleration() {
	state := &g.transientCommandInput
	if camp := g.sleepingInputCamp(); camp != nil && camp == state.campAtEntry && state.campRequest {
		g.gameState.RequestCampingAcceleration()
	}
}

// Stop the old hold before a modal newly installed by a queue gets its first
// Update. End-of-frame cleanup alone would be too late for that handoff.
func (g *GameScene) reconcileTransientInputAfterQueues() {
	state := &g.transientCommandInput
	if !g.explorationPassEligible() || state.pass.active && state.pass.context != g.passInputContext() {
		g.stopExplorationPass()
	}
	if g.transientInputOwnedElsewhere() {
		g.clearPendingCombatCommand()
	}
}

func (g *GameScene) sleepingInputCamp() *camping.CampingScene {
	camp := g.gameState.CampingScene
	if !camp.IsSleeping() || g.transientInputOwnedElsewhere() {
		return nil
	}
	manager := g.gameState.EventQueueManager
	if manager.HasOnlyWorkflow("camping_workflow") || manager.HasOnlyWorkflow("dungeon_hole_up") ||
		manager.HasOnlyWorkflow("camping_debug_workflow") {
		return camp
	}

	return nil
}

func (g *GameScene) transientInputOwnedElsewhere() bool {
	return g.secondaryKeyState != PrimaryInput || g.atlasSession != nil || g.journalSession != nil ||
		g.finalePresentation != nil || g.dialogStack.HasModalDialog() ||
		g.autoOpenDoorState != nil && g.autoOpenDoorState.IsActive() ||
		!g.gameState.CanAcceptGameplayCommands()
}

func (g *GameScene) finishTransientCommandInputFrame() {
	state := &g.transientCommandInput
	if !g.commandInputFocused() {
		return
	}
	if g.gameState.CombatScene != nil && g.transientInputOwnedElsewhere() {
		// This owner has already seen its input. A carried 0 must not become
		// an all-party reset after its reader, modal or secondary prompt closes.
		g.keyboard.SuppressUntilRelease(ebiten.Key0)
	}
	if !g.explorationPassEligible() || state.pass.active && state.pass.context != g.passInputContext() {
		g.stopExplorationPass()
	}
	g.validatePendingCombatCommand()
	if projection, eligible := g.gameState.ProjectSoloCombatInput(); eligible && projection.Waiting &&
		g.combatInputWaitOwned() && !g.transientInputOwnedElsewhere() && !state.combatWaitSampled {
		// The enemy owner appeared after this sample (for example, from an
		// own-action finalizer). Its carried letters are not fresh requests.
		for _, key := range boundKeysGame {
			if bufferedCombatVerb(key) != "" {
				g.keyboard.SuppressUntilRelease(key)
			}
		}
	}
	if camp := g.sleepingInputCamp(); camp != nil {
		if camp != state.campAtEntry {
			// A press carried through setup/Bard is not a sleeping press.
			g.keyboard.SuppressUntilRelease(ebiten.KeySpace)
		}
		// Camping acceleration is deliberately silent. Only the sleeping cadence
		// changes; it must not introduce a hint, acknowledgement, or status card.
	}
}

// ResetTransientCommandInput is the common teardown/load/focus boundary. Narrow
// suppression survives clearing intent, so rejected held arrows cannot revive.
func (g *GameScene) ResetTransientCommandInput() {
	state := &g.transientCommandInput
	g.stopExplorationPass()
	state.combatRepeat = combatKeyRepeat{}
	g.clearPendingCombatCommand()
	state.campAtEntry, state.campRequest = nil, false
	if g.keyboard != nil {
		for _, key := range boundKeysGame {
			g.keyboard.SuppressUntilRelease(key)
		}
	}
}
