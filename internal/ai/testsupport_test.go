package ai

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// SetAttackAvatar turns every friendly NPC that attacks the Avatar when trouble
// starts hostile: it sets the law-enforcement override and clears the NPC's path
// and pending interaction.
func (n *NPCAIControllerSmallMap) SetAttackAvatar() {
	for _, mapUnit := range n.mapUnits {
		if npc, ok := mapUnit.(*map_units.NPCFriendly); ok {
			if npc.NPCReference.WantsToAttackAvatarWhenBadStuffGoesDown() {
				// Guard activation selects action 7 for NPC tile bytes >= 47; ref:CRIME-0002.
				mapUnit.MapUnitDetails().SetLawAIOverride(references.DrudgeWorthThing)
				mapUnit.MapUnitDetails().SetCurrentPath(nil)
				mapUnit.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone
			}
		}
	}
}
