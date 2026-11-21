package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func combatPossessionEventSuffix(event game_state.CombatEvent) string {
	if event.Kind == game_state.CombatEventPossessionResistance {
		switch event.PossessionTurns {
		case 3:
			return " struggles against the enchantment."
		case 7:
			return "'s resistance grows stronger."
		case 10:
			return " is close to breaking free!"
		default:
			return " resists the enchantment."
		}
	}
	switch event.PossessionReleaseReason {
	case game_state.CombatPossessionReleaseNatural:
		return " breaks free of the enchantment!"
	case game_state.CombatPossessionReleaseControllerDefeated:
		return " regains control as the possessor is defeated!"
	case game_state.CombatPossessionReleaseControllerEscaped:
		return " regains control as the possessor flees!"
	case game_state.CombatPossessionReleaseControllerReplaced:
		return " regains control as the transformed possessor's hold breaks!"
	default:
		panic("unknown possession release reason")
	}
}

// Typed condition feedback already describes this exact transition. Update only
// its observation so the independent allegiance poll cannot announce it again.
func (g *GameScene) observeCombatPossessionAllegiance(event game_state.CombatEvent) {
	cured := event.Kind == game_state.CombatEventSpell && event.Diagnostics.Spell == references.AnXenEx &&
		event.Target.IsPartyMember && event.Result == game_state.CombatFeedbackDamage
	if event.Kind != game_state.CombatEventPossession && event.Kind != game_state.CombatEventPossessionRelease &&
		event.Kind != game_state.CombatEventRecovery && !cured {
		return
	}
	state := &g.combatAllegiance
	if state.encounter != g.gameState.CombatScene {
		*state = combatAllegianceState{encounter: g.gameState.CombatScene}
	}
	actor, ok := g.gameState.ProjectCombatActor(combatTurnIdentityForFeedback(event.Target), 0)
	if !ok {
		return
	}
	if state.actors == nil {
		state.actors = make(map[combatAllegianceKey]combatAllegianceObservation)
	}
	state.actors[combatAllegianceKey{identity: actor.Identity, id: actor.CombatantID}] = combatAllegianceObservation{
		loyalty: actor.Loyalty, seen: true,
	}
}

const combatPossessionHelp = "A possessed member fights the hold and always breaks free in time; the clever break free sooner. An Xen Ex can free them early. The hold also breaks when the possessor is slain, flees or is turned into a rat. Breaking free does not heal or wake them."
