package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// combatRenderFrame shares read-only projections across presentation consumers.
// Its lifetime is one admitted Draw, never an Update, turn, or targeting action.
type combatRenderFrame struct {
	active            bool
	previewReady      bool
	preview           game_state.CombatAttackPreview
	insectStacksReady bool
	insectStacks      map[references.Position][]int
	coneAimReady      bool
	coneAimShown      bool
	coneAim           combatConeAim
}

func (g *GameScene) beginCombatRenderFrame() {
	g.combatRenderFrame = combatRenderFrame{active: true}
}

func (g *GameScene) endCombatRenderFrame() {
	g.combatRenderFrame = combatRenderFrame{}
}

// Build in manager order to preserve the representative sprite and each health
// segment's identity. Ordinary crowds do not allocate a stack map or slices.
func (g *GameScene) buildCombatPartyInsectStacks() map[references.Position][]int {
	var stacks map[references.Position][]int
	combat := g.gameState.CombatScene
	if combat == nil {
		return nil
	}
	combat.MonsterManager.ForEachActive(func(index int, monster *game_state.CombatMonster) {
		if monster.MonsterType() != references.MonsterInsect {
			return
		}
		actor, disclosed := g.gameState.ProjectCombatActor(game_state.CombatTurnIdentity{EntityIndex: index}, monster.CombatantID())
		if !disclosed || !g.combatAllegiancePresented(actor) || actor.Loyalty != game_state.CombatMonsterLoyaltyParty {
			return
		}
		if stacks == nil {
			stacks = make(map[references.Position][]int)
		}
		stacks[actor.Position] = append(stacks[actor.Position], index)
	})

	return stacks
}
