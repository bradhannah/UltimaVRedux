package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// CombatActorPresentation is a detached, disclosed exact-entry view. Side comes
// from combatIdentityLoyalty; provenance is recorded at the successful producer,
// never guessed from artwork, roster membership, control, or current loyalty.
type CombatActorPresentation struct {
	Identity            CombatTurnIdentity
	CombatantID         CombatantID
	Position            references.Position
	Name                string
	Loyalty             CombatMonsterLoyalty
	Charmed             bool
	Summoned            bool
	Possessed           bool
	PossessionEpisodeID uint64
	PossessionTurns     int
}

// ProjectCombatActor returns no identifying data for an undisclosed or retired
// actor. A nonzero expectedID also rejects a recycled monster slot. Invisible
// PCs retain their established visible shimmer; invisible monsters do not.
func (g *GameState) ProjectCombatActor(identity CombatTurnIdentity, expectedID CombatantID) (CombatActorPresentation, bool) {
	return g.projectCombatActor(identity, expectedID, false)
}

// ProjectCombatTurnActor is ProjectCombatActor for the turn panel, where the
// party is never concealed: the player knows who stands under an arch or was
// dragged under by a Corpser, and the party roster names them regardless.
func (g *GameState) ProjectCombatTurnActor(identity CombatTurnIdentity) (CombatActorPresentation, bool) {
	return g.projectCombatActor(identity, 0, identity.IsPartyMember)
}

func (g *GameState) projectCombatActor(
	identity CombatTurnIdentity, expectedID CombatantID, known bool,
) (CombatActorPresentation, bool) {
	combat := g.CombatScene
	if combat == nil || identity.EntityIndex < 0 || (identity.IsPartyMember && identity.EntityIndex >= len(combat.PartyActive)) {
		return CombatActorPresentation{}, false
	}
	view := CombatActorPresentation{Identity: identity}
	if identity.IsPartyMember {
		if expectedID != 0 || !combat.IsCombatPCActive(identity) || (!known && combat.IsCombatPCUnderground(identity)) {
			return CombatActorPresentation{}, false
		}
		character := g.PartyState.GetCharacter(identity.EntityIndex)
		if character == nil || character.IsDead() || !character.IsInParty() {
			return CombatActorPresentation{}, false
		}
		view.Name = character.GetNameAsString()
		if identity.Slot != 0 {
			view.Name += fmt.Sprintf(" Clone %d", identity.Slot)
		}
		view.Position = combat.GetCombatantPosition(identity)
		view.Possessed = combat.IsCombatPCPossessed(identity)
		view.PossessionEpisodeID, view.PossessionTurns, _ = combat.CombatPossessionProgress(identity)
	} else {
		monster := combat.MonsterManager.GetMonster(identity.EntityIndex)
		if identity.Slot != 0 || monster == nil || !monster.IsAlive() || monster.IsInvisible() || monster.IsCorpserHidden() ||
			(expectedID != 0 && expectedID != monster.CombatantID()) {
			return CombatActorPresentation{}, false
		}
		view.Name, view.Position, view.CombatantID = monster.MonsterType().Name(), monster.Position(), monster.CombatantID()
		view.Charmed, view.Summoned = monster.charmed, monster.summoned
	}
	if !known && !g.CombatTerrainAllowsObjectDisplay(view.Position) {
		return CombatActorPresentation{}, false
	}
	view.Loyalty = g.combatIdentityLoyalty(identity)

	return view, true
}

// ForEachDisclosedCombatActor visits current exact actors in stable encounter
// order. It consumes no gameplay time, RNG, resources, or callback channels.
func (g *GameState) ForEachDisclosedCombatActor(visit func(CombatActorPresentation)) {
	if g.CombatScene == nil {
		return
	}
	project := func(identity CombatTurnIdentity) {
		if view, ok := g.ProjectCombatActor(identity, 0); ok {
			visit(view)
		}
	}
	for index := range g.CombatScene.PartyActive {
		project(CombatTurnIdentity{IsPartyMember: true, EntityIndex: index})
	}
	g.CombatScene.ForEachPCClone(func(identity CombatTurnIdentity, _ references.Position) { project(identity) })
	g.CombatScene.MonsterManager.ForEachActive(func(index int, _ *CombatMonster) {
		project(CombatTurnIdentity{EntityIndex: index})
	})
}

// CombatActorAt projects the first disclosed actor at a square, preserving
// exact clone identity. An empty result says nothing about hidden occupancy.
func (g *GameState) CombatActorAt(position references.Position) (CombatActorPresentation, bool) {
	var result CombatActorPresentation
	found := false
	g.ForEachDisclosedCombatActor(func(view CombatActorPresentation) {
		if !found && view.Position == position {
			result, found = view, true
		}
	})

	return result, found
}
