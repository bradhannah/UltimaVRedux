package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const combatMonsterDivisionAttempts = 8

// tryDivideCombatMonster reproduces the original SPLITS behavior after a monster survives a hit.
func (g *GameState) tryDivideCombatMonster(monster *CombatMonster, monsterIdx int) {
	if monster.IsDead() || !monster.HasAbility(references.DivideOnHit) {
		return
	}
	allowRepeatedDivision := g.GameConfig != nil && g.GameConfig.GameplayConfiguration != nil &&
		g.GameConfig.GameplayConfiguration.AllowRepeatedMonsterDivision
	// REDUX DEVIATION: Original division permits a surviving parent and its
	// descendants to divide repeatedly. Redux defaults to
	// one successful split per encounter-created monster to prevent exponential
	// encounter growth; the gameplay setting restores original behavior.
	// See ref:COMBAT-0009.
	if !allowRepeatedDivision && monster.divisionConsumed {
		return
	}

	for range combatMonsterDivisionAttempts {
		position := g.randomAdjacentCombatPosition(monster.Position())
		if !isCombatPositionValidForMonster(g, position, monster, monsterIdx) {
			continue
		}
		if !g.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
			continue
		}

		child := NewCombatMonster(monster.MonsterType(), monster.EnemyRef(), position, g.AsRandomProvider())
		if !g.CombatScene.hasActiveObjectSlot() {
			continue
		}
		child.SetCurrentHP(monster.CurrentHP())
		if !allowRepeatedDivision {
			monster.divisionConsumed = true
			child.divisionConsumed = true
		}
		childIdx := g.CombatScene.AddMonsterToQueue(child)
		g.SystemCallbacks.Message.AddRowStr(monster.EnemyRef().Name + " divides!")
		g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
			Kind: CombatEventDivision,
			Attacker: CombatFeedbackEntity{
				IsPartyMember: false,
				Index:         monsterIdx,
				Position:      monster.Position(),
			},
			Target: CombatFeedbackEntity{
				IsPartyMember: false,
				Index:         childIdx,
				Position:      child.Position(),
			},
			From: monster.Position(),
			To:   child.Position(),
		})

		return
	}
}

func (g *GameState) randomAdjacentCombatPosition(origin references.Position) references.Position {
	for {
		position := references.Position{
			X: origin.X + references.Coordinate(g.RandomIntInRange(-1, 1)),
			Y: origin.Y + references.Coordinate(g.RandomIntInRange(-1, 1)),
		}
		if position == origin || position.X < 0 || position.X >= references.XCombatMapTiles ||
			position.Y < 0 || position.Y >= references.YCombatMapTiles {
			continue
		}

		return position
	}
}
