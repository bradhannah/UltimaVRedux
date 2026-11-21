package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

func (g *GameScene) currentHelpStatus() widgets.HelpStatusView {
	view := widgets.HelpStatusView{Worn: nil, Effects: nil, Food: g.gameState.PartyState.Inventory.Provisions.Food.Get()}
	if g.gameState.CombatScene != nil {
		view.Effects = append(view.Effects, widgets.StatusHelpEntry{
			Name: "Possession", Description: combatPossessionHelp, Tint: [3]float32{1, 1, 1},
		})
	}
	for _, artifact := range g.gameState.GetWornArtifactStatuses() {
		if artifact.Worn {
			view.Worn = append(view.Worn, widgets.StatusHelpEntry{
				Name: artifact.ID.FriendlyName(), Description: "Currently worn.",
				Icon: wornArtifactImage(g.spriteSheet, artifact.ID), Tint: [3]float32{1, 1, 1},
			})
		}
	}
	for _, effect := range g.gameState.GetPersistentEffectStatuses() {
		if effect.ID == game_state.PersistentEffectCrownSuppression {
			continue // The rail identifies this with the worn Crown, not a duplicate effect icon.
		}
		presentation, ok := persistentEffectPresentationFor(effect.ID)
		if !ok {
			panic("active help effect has no status artwork")
		}
		name, description := effectHelpDescription(effect.ID)
		art := g.spriteSheet.GetSprite(presentation.sprite)
		if presentation.animated {
			art = g.spriteSheet.GetAnimatedSprite(presentation.sprite, persistentEffectAnimationHash(effect.ID), g.gameState.ElapsedMs)
		}
		view.Effects = append(view.Effects, widgets.StatusHelpEntry{
			Name: name, Description: description, Icon: unwrappedStatusSprite(art),
			Tint: [3]float32{presentation.tintR, presentation.tintG, presentation.tintB},
		})
	}

	return view
}

func effectHelpDescription(id game_state.PersistentEffectID) (string, string) {
	switch id {
	case game_state.PersistentEffectTorch:
		return "Torch", "A lit torch illuminates nearby ground."
	case game_state.PersistentEffectMagicLight:
		return "Magic light", "Magical light illuminates nearby ground."
	case game_state.PersistentEffectProtection:
		return "Protection", "A protective spell helps defend the party in battle."
	case game_state.PersistentEffectNegateMagic:
		return "Negate Magic", "A spell is suppressing magic."
	case game_state.PersistentEffectTimeStop:
		return "Time Stop", "Time is magically stopped."
	case game_state.PersistentEffectQuickness:
		return "Quickness", "The party is under a speed spell."
	case game_state.PersistentEffectConfusion:
		return "Confusion", "A confusion spell disrupts enemy actions."
	case game_state.PersistentEffectIntoxication:
		return "Intoxication", "Drinking is impairing commands in town."
	case game_state.PersistentEffectShadowlord:
		return "Ominous influence", "An ominous influence hangs over this town."
	case game_state.PersistentEffectStarvation:
		return "Starvation", "Food has run out. The party can lose health."
	case game_state.PersistentEffectHMSCape:
		return "HMS Cape", "The ship's magical rigging is active."
	case game_state.PersistentEffectCrownSuppression:
		panic("Crown belongs to the worn-item legend")
	default:
		panic("unknown active effect")
	}
}
