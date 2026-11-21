package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/hajimehoshi/ebiten/v2"
)

const (
	commandLabelJimmy  = "Jimmy"
	commandLabelSearch = "Search"
)

type pendingCombatCommand struct {
	active      bool
	key         ebiten.Key
	opportunity game_state.CombatInputOpportunity
}

func bufferedCombatVerb(key ebiten.Key) string {
	switch key { //nolint:exhaustive // Deliberate, reviewed direction-only allowlist.
	case ebiten.KeyG:
		return "Get"
	case ebiten.KeyO:
		return "Open"
	case ebiten.KeyJ:
		return commandLabelJimmy
	case ebiten.KeyP:
		return "Push"
	case ebiten.KeyS:
		return commandLabelSearch
	default:
		return ""
	}
}

func isEarlyCombatDirection(key ebiten.Key) bool {
	normalized := primaryMovementKey(key)

	return normalized == ebiten.KeyUp || normalized == ebiten.KeyDown ||
		normalized == ebiten.KeyLeft || normalized == ebiten.KeyRight
}

func bufferedGameplayKey(key ebiten.Key) bool {
	return key >= ebiten.KeyA && key <= ebiten.KeyZ || key == ebiten.KeySpace || key == ebiten.KeyTab
}

func (g *GameScene) combatInputWaitOwned() bool {
	manager := g.gameState.EventQueueManager

	return manager.HasOnlyWorkflow("monster_turn_delay") || manager.HasOnlyWorkflow("combat_pc_auto_pass")
}

func (g *GameScene) clearPendingCombatCommand() {
	g.transientCommandInput.pending = pendingCombatCommand{}
}

func (g *GameScene) validatePendingCombatCommand() bool {
	pending := &g.transientCommandInput.pending
	if !pending.active {
		return false
	}
	projection, eligible := g.gameState.ProjectSoloCombatInput()
	if !eligible || projection.Opportunity != pending.opportunity || g.transientInputOwnedElsewhere() ||
		projection.Waiting && !g.combatInputWaitOwned() ||
		projection.Ready && g.inputBlocked && !g.combatInputWaitOwned() {
		g.clearPendingCombatCommand()

		return false
	}

	return true
}

// capturePendingCombatCommand applies all edits before readiness is reconsidered.
// Same-sample ties: Escape wins, then any unsupported action, then alphabetically
// first supported verb. All losing action edges are consumed, never postponed.
func (g *GameScene) capturePendingCombatCommand() {
	g.validatePendingCombatCommand()
	state := &g.transientCommandInput
	projection, eligible := g.gameState.ProjectSoloCombatInput()
	waiting := eligible && projection.Waiting && g.combatInputWaitOwned()
	readyPending := eligible && projection.Ready && state.pending.active
	if g.inputBlocked && !waiting && !readyPending && g.gameState.CombatScene != nil {
		// A letter during the player's own committed presentation is neither
		// buffered nor allowed to bleed into a later ready boundary.
		for _, key := range boundKeysGame {
			if bufferedCombatVerb(key) != "" {
				g.keyboard.SuppressUntilRelease(key)
			}
		}
	}
	if (!waiting && !readyPending) || state.sample.HasModifiers() {
		return
	}
	state.combatWaitSampled = waiting
	if waiting {
		for _, key := range boundKeysGame {
			if bufferedCombatVerb(key) != "" && state.sample.Held[key] && !state.sample.Pressed[key] {
				g.keyboard.SuppressUntilRelease(key)
			}
		}
	}
	hasCandidate := false
	for _, key := range boundKeysGame {
		hasCandidate = hasCandidate || state.sample.Pressed[key] && bufferedCombatVerb(key) != ""
	}
	if !state.pending.active && !hasCandidate {
		return
	}
	var winner ebiten.Key
	hasWinner, unsupported, cancel := false, false, false
	for _, key := range boundKeysGame {
		if !state.sample.Pressed[key] {
			continue
		}
		if key == ebiten.KeyEscape {
			cancel = true
			g.keyboard.SuppressUntilRelease(key)

			continue
		}
		if !bufferedGameplayKey(key) {
			continue
		}
		verb := bufferedCombatVerb(key)
		if !g.keyboard.ConsumePressEdge(state.sample, key) {
			continue
		}
		g.keyboard.SuppressUntilRelease(key)
		if verb == "" {
			unsupported = true
		} else if !hasWinner || key < winner {
			winner, hasWinner = key, true
		}
	}
	hadIntent := state.pending.active || hasWinner
	switch {
	case cancel:
		g.clearPendingCombatCommand()
		if hadIntent {
			g.output.AddText("Queued command dropped.")
		}
	case unsupported:
		g.clearPendingCombatCommand()
		g.output.AddText("Command not queued; queued command dropped.")
	case hasWinner:
		state.pending = pendingCombatCommand{active: true, key: winner, opportunity: projection.Opportunity}
	}
	// Waiting is the entry-frame authority. Completing an enemy later in this
	// Update never makes these directions timely. Clearing intent cannot free them.
	if hadIntent && (waiting || g.inputBlocked) {
		for _, key := range boundKeysGame {
			if state.sample.Held[key] && isEarlyCombatDirection(key) {
				g.keyboard.SuppressUntilRelease(key)
			}
		}
	}
}

func (g *GameScene) acceptPendingCombatCommand() {
	if !g.validatePendingCombatCommand() || g.inputBlocked {
		return
	}
	projection, eligible := g.gameState.ProjectSoloCombatInput()
	if !eligible || !projection.Ready || g.gameState.EventQueueManager.HasInputBlockingWorkflows() {
		return
	}
	key := g.transientCommandInput.pending.key
	g.clearPendingCombatCommand()
	g.combatMapInputHandler(key)
	g.appendOrdinaryDirectionHint(references.CombatMapType)
}
