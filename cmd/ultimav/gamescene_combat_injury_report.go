package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// Rendering and measurement share the same admission and wording. Severity is
// always resolution-time data; a preference toggle only changes its visibility.
func (g *GameScene) combatInjuryReport(event game_state.CombatEvent) (combatReportIdentity, string) {
	if g.showCombatHPBars() || event.Target.IsPartyMember || event.HPLost <= 0 || event.Fatal ||
		!event.MonsterInjury.Disclosed || event.MonsterInjury.Severity == game_state.CombatInjuryNone {
		return combatReportIdentity{}, ""
	}
	switch event.Kind { //nolint:exhaustive // Only actual damage reports admit an injury suffix.
	case game_state.CombatEventAttack, game_state.CombatEventRangedAttack, game_state.CombatEventSpell:
		if event.Result != game_state.CombatFeedbackDamage || event.Diagnostics.ProjectileBlocked {
			return combatReportIdentity{}, ""
		}
	case game_state.CombatEventFieldTriggered:
		if event.FieldKind != game_state.CombatFieldFire && event.FieldKind != game_state.CombatFieldPoison {
			return combatReportIdentity{}, ""
		}
	default:
		return combatReportIdentity{}, ""
	}
	if event.SpellTarget != nil && (!event.SpellTarget.Disclosed || event.SpellTarget.Outcome != game_state.CombatSpellDamaged) {
		return combatReportIdentity{}, ""
	}
	if event.FireInjury != nil && !event.FireInjury.Disclosed {
		return combatReportIdentity{}, ""
	}
	identity := g.combatReportDisclosure(event).Target
	if !identity.Disclosed {
		return combatReportIdentity{}, ""
	}

	return identity, combatInjuryWording(event.MonsterInjury.Severity)
}

func combatInjuryWording(severity game_state.CombatInjurySeverity) string {
	switch severity {
	case game_state.CombatInjuryNone:
		return ""
	case game_state.CombatInjuryBarelyWounded:
		return "barely wounded!"
	case game_state.CombatInjuryLightlyWounded:
		return "lightly wounded!"
	case game_state.CombatInjuryHeavilyWounded:
		return "heavily wounded!"
	case game_state.CombatInjuryCritical:
		return "critical!"
	default:
		panic("unknown combat injury severity")
	}
}

func (g *GameScene) combatInjurySuffix(event game_state.CombatEvent) string {
	identity, wording := g.combatInjuryReport(event)
	if wording == "" {
		return ""
	}

	return ". " + identity.Name + " is " + wording
}

func (g *GameScene) appendCombatInjurySuffix(line *text.LineBuilder, event game_state.CombatEvent) *text.LineBuilder {
	identity, wording := g.combatInjuryReport(event)
	if wording != "" {
		// The full stop is its own run so a row that has room for it keeps it.
		line.White(".").White(" ").Text(identity.Name, identity.Color).White(" is ").Text(wording, ucolor.CombatDamageText)
	}

	return line
}
