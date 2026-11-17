package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

type combatReportIdentity struct {
	Identity  game_state.CombatTurnIdentity
	Name      string
	Color     ucolor.EGAColor
	Disclosed bool
}

type combatReportIdentities struct {
	Attacker, Target   combatReportIdentity
	DetailsDisclosed   bool
	VanishedDisclosed  bool
	CreatedDisclosed   bool
	AggregateDisclosed bool
}

// Freeze names/allegiance at admission. An original monster slot can be reused
// later in this encounter; history must not rename its earlier possession/gate.
// Historical names already seen are retained; invisible arrivals stay Unknown
// even after revelation. The live turn HUD independently hides invisible actors.
func (g *GameScene) captureCombatReportIdentity(entity game_state.CombatFeedbackEntity) combatReportIdentity {
	identity := combatTurnIdentityForFeedback(entity)
	result := combatReportIdentity{Identity: identity, Color: ucolor.CombatAllegianceEnemy}
	if entity.IsPartyMember {
		if entity.Index >= 0 && entity.Index < g.gameState.PartyState.GetCharacterCount() {
			result.Name = g.combatEntityEventName(identity)
			result.Disclosed = true
		}
		result.Color = ucolor.EGAYellow
	} else if g.gameState.CombatScene != nil {
		monster := g.gameState.CombatScene.MonsterManager.GetMonster(entity.Index)
		if g.combatMonsterDisclosed(entity.Index, 0, entity.Position) {
			result.Name = monster.MonsterType().Name()
			result.Disclosed = true
			if monster.IsPartyLoyal() {
				result.Color = ucolor.CombatAllegianceAlly
			}
		}
	}
	if result.Name == "" {
		result.Name = combatUnknownEntityLabel
	}

	return result
}

// Death and retirement are not concealment. These events publish after state
// removal, while the original monster record still owns its visibility flag.
func (g *GameScene) combatMonsterDisclosed(index int, id game_state.CombatantID, position references.Position) bool {
	if g.gameState.CombatScene == nil {
		return false
	}
	monster := g.gameState.CombatScene.MonsterManager.GetMonster(index)

	return monster != nil && (id == 0 || id == monster.CombatantID()) && !monster.IsInvisible() &&
		g.gameState.CombatTerrainAllowsObjectDisplay(position)
}

func (g *GameScene) captureCombatReportIdentities(event game_state.CombatEvent) combatReportIdentities {
	snapshot := combatReportIdentities{
		Target:           g.captureCombatReportIdentity(event.Target),
		DetailsDisclosed: true, AggregateDisclosed: true,
	}
	// Field triggers have only a target. Their zero-valued attacker is absent,
	// not monster slot zero at (0,0), whose disclosure can differ from the victim.
	if event.Kind != game_state.CombatEventFieldTriggered {
		snapshot.Attacker = g.captureCombatReportIdentity(event.Attacker)
	}
	// The core snapshot predates survivor effects and presentation callbacks.
	// Do not let a healed/replaced/revealed live slot disclose or rename this hit.
	if event.MonsterInjury.Severity != game_state.CombatInjuryNone && !event.Target.IsPartyMember {
		snapshot.Target.Name = combatUnknownEntityLabel
		snapshot.Target.Disclosed = event.MonsterInjury.Disclosed
		snapshot.Target.Color = ucolor.CombatAllegianceEnemy
		if event.MonsterInjury.Disclosed {
			snapshot.Target.Name = event.MonsterInjury.MonsterType.Name()
			if event.MonsterInjury.Loyalty == game_state.CombatMonsterLoyaltyParty {
				snapshot.Target.Color = ucolor.CombatAllegianceAlly
			}
		}
	}
	visible := g.gameState.CombatTerrainAllowsObjectDisplay
	switch event.Kind { //nolint:exhaustive // Events without actor/position diagnostics need no extra disclosure gate.
	case game_state.CombatEventAttack, game_state.CombatEventPoison, game_state.CombatEventSpell,
		game_state.CombatEventRangedAttack, game_state.CombatEventPossession, game_state.CombatEventInterruption:
		snapshot.DetailsDisclosed = snapshot.Attacker.Disclosed && snapshot.Target.Disclosed
	case game_state.CombatEventMove, game_state.CombatEventTeleport:
		snapshot.DetailsDisclosed = snapshot.Attacker.Disclosed && visible(event.From) && visible(event.To)
	case game_state.CombatEventDivision:
		snapshot.DetailsDisclosed = snapshot.Attacker.Disclosed && visible(event.To)
	case game_state.CombatEventRetired, game_state.CombatEventFled:
		snapshot.DetailsDisclosed = snapshot.Attacker.Disclosed
	case game_state.CombatEventRecovery, game_state.CombatEventPossessionResistance, game_state.CombatEventPossessionRelease:
		snapshot.DetailsDisclosed = snapshot.Target.Disclosed
	case game_state.CombatEventFieldTriggered:
		snapshot.DetailsDisclosed = snapshot.Target.Disclosed
	case game_state.CombatEventGating:
		snapshot.CreatedDisclosed = g.combatMonsterDisclosed(event.Created.Index, event.Created.CombatantID, event.Created.Position)
		snapshot.DetailsDisclosed = snapshot.Attacker.Disclosed && snapshot.CreatedDisclosed
	case game_state.CombatEventVanished:
		snapshot.VanishedDisclosed = g.combatMonsterDisclosed(event.Vanished.Index, event.Vanished.CombatantID, event.Vanished.Position)
		snapshot.DetailsDisclosed = snapshot.VanishedDisclosed
	case game_state.CombatEventUtilitySpell:
		snapshot.DetailsDisclosed = snapshot.Attacker.Disclosed && visible(event.To)
		if event.UtilityResult == game_state.CombatUtilityTeleported {
			snapshot.DetailsDisclosed = snapshot.DetailsDisclosed && visible(event.From)
		}
	case game_state.CombatEventFieldCreated, game_state.CombatEventFieldDispelled, game_state.CombatEventFieldExpired:
		snapshot.DetailsDisclosed = visible(event.To)
	case game_state.CombatEventSummon:
		for _, created := range event.Summon.Created {
			snapshot.AggregateDisclosed = snapshot.AggregateDisclosed &&
				g.combatMonsterDisclosed(created.Index, created.CombatantID, created.Position)
		}
		snapshot.DetailsDisclosed = snapshot.AggregateDisclosed
	case game_state.CombatEventRepelUndead:
		for _, targets := range [][]game_state.CombatSpellMonster{event.AnXenCorp.Result.Affected, event.AnXenCorp.Result.Resisted, event.AnXenCorp.Result.Immune} {
			for _, target := range targets {
				snapshot.AggregateDisclosed = snapshot.AggregateDisclosed &&
					g.combatMonsterDisclosed(target.Index, target.CombatantID, target.Position)
			}
		}
		snapshot.DetailsDisclosed = snapshot.AggregateDisclosed
	default:
	}

	return snapshot
}

func (g *GameScene) combatReportDisclosure(event game_state.CombatEvent) combatReportIdentities {
	if g.combatHUD != nil && g.combatHUD.presentingIdentities != nil {
		return *g.combatHUD.presentingIdentities
	}
	// Actor-free HUD history can be measured before a gameplay scene is bound.
	if g.gameState == nil {
		return combatReportIdentities{}
	}

	return g.captureCombatReportIdentities(event)
}

func (g *GameScene) combatVanishedReportName(event game_state.CombatEvent) string {
	if !g.combatReportDisclosure(event).VanishedDisclosed {
		return combatUnknownEntityLabel
	}

	return event.Vanished.MonsterType.Name()
}

func (g *GameScene) combatGateReportResult(event game_state.CombatEvent) string {
	if !g.combatReportDisclosure(event).CreatedDisclosed {
		return " gates in an unseen creature"
	}

	return " gates in a daemon"
}

func (g *GameScene) combatSummonReportResult(event game_state.CombatEvent) string {
	if !g.combatReportDisclosure(event).AggregateDisclosed {
		return "unseen creatures"
	}

	return combatSummonAggregateText(event.Summon)
}

func (g *GameScene) combatRepelReportResult(event game_state.CombatEvent) (string, string) {
	spell, result := combatFearOrRepelText(event)
	if !g.combatReportDisclosure(event).AggregateDisclosed {
		result = "affects unseen creatures"
	}

	return spell, result
}

func (hud *CombatHUD) reportIdentity(identity game_state.CombatTurnIdentity) (combatReportIdentity, bool) {
	if hud.presentingIdentities != nil {
		for _, snapshot := range []combatReportIdentity{hud.presentingIdentities.Attacker, hud.presentingIdentities.Target} {
			// Empty names denote absent roles; concealed actors retain "Unknown".
			if snapshot.Name != "" && identity == snapshot.Identity {
				return snapshot, true
			}
		}
	}

	return combatReportIdentity{}, false
}

func (g *GameScene) withCombatHistoryEvent(slot int, present func(game_state.CombatEvent)) {
	hud := g.combatHUD
	previous := hud.presentingIdentities
	if hud.identities[slot].Attacker.Name != "" || hud.identities[slot].Target.Name != "" {
		hud.presentingIdentities = &hud.identities[slot]
	}
	defer func() { hud.presentingIdentities = previous }()
	present(hud.history[slot])
}

func (g *GameScene) combatHistoryEventLineCount(slot int) int {
	var count int
	g.withCombatHistoryEvent(slot, func(event game_state.CombatEvent) { count = g.combatEventLineCount(event) })

	return count
}
