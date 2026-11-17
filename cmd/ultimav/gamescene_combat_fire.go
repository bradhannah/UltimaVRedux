package main

import "github.com/bradhannah/UltimaVRedux/internal/game_state"

const (
	// Smoothstep reaches the destination tile's near edge at its midpoint.
	// Hold the burn there, then grant a full fade/read lifetime at arrival.
	combatFireContactProgress = 0.5
	combatFireImpactCapacity  = 64
)

func (g *GameScene) admitCombatFireImpact(effect CombatImpactEffect, injury game_state.CombatFireInjury) {
	owner := g.gameState.BindCombatFireInjury(injury)
	if _, disclosed := g.gameState.ProjectCombatFireInjury(injury, owner); !disclosed {
		return
	}
	// Own a detached result, not the producer's pointer. The event/report and this
	// one impact remain the only delivery route for field and Flame Wind injuries.
	effect.FireInjury = &injury
	effect.fireOwner = owner
	effect.fireClockAt = g.gameState.ElapsedMs
	effect.fireAdmittedAt = g.gameState.ElapsedMs
	effect.fireMovementBound = effect.Feedback.Presentation.Delivery != game_state.CombatAttackDeliveryEnvironmental
	if combatFireMatchesMovement(effect, g.combatMovement) {
		effect.fireMovement, effect.fireMovementBound = true, true
	}
	g.limitCombatFireImpacts()
	g.combatImpactEffects = append(g.combatImpactEffects, effect)
}

// A new authoritative move closes eligibility for earlier injuries, even when
// several actions share a millisecond or the new move rolls zero damage.
func (g *GameScene) closeCombatFireMovementAdmission() {
	for index := range g.combatImpactEffects {
		if g.combatImpactEffects[index].FireInjury != nil {
			g.combatImpactEffects[index].fireMovementBound = true
		}
	}
}

func (g *GameScene) limitCombatFireImpacts() {
	count, oldest := 0, -1
	for index, effect := range g.combatImpactEffects {
		if effect.FireInjury == nil {
			continue
		}
		count++
		if oldest < 0 {
			oldest = index
		}
	}
	if count < combatFireImpactCapacity {
		return
	}
	copy(g.combatImpactEffects[oldest:], g.combatImpactEffects[oldest+1:])
	last := len(g.combatImpactEffects) - 1
	g.combatImpactEffects[last] = CombatImpactEffect{}
	g.combatImpactEffects = g.combatImpactEffects[:last]
}

func combatFireMatchesMovement(effect CombatImpactEffect, movement CombatMovementAnimation) bool {
	target := effect.Feedback.Target

	return movement.Active && effect.FireInjury != nil &&
		target.IsPartyMember == movement.IsPartyMember && target.Index == movement.EntityIndex &&
		target.Slot == movement.Slot && target.Position == movement.To
}

// Movement damage is resolved before the interpolation exists. Bind only the
// fresh result for this exact actor and destination, never an earlier burn or
// another clone. Mid-movement injuries bind at admission instead.
func (g *GameScene) bindCombatFireMovement() {
	for index := len(g.combatImpactEffects) - 1; index >= 0; index-- {
		effect := &g.combatImpactEffects[index]
		// Death retires clones before the input adapter asks for their destination.
		// Keep the resolved square rather than interpolating a missing clone to 0,0.
		target := effect.Feedback.Target
		if effect.FireInjury != nil && effect.FireInjury.Fatal && !effect.fireMovementBound &&
			effect.fireAdmittedAt == g.gameState.ElapsedMs && g.combatMovement.Active &&
			target.IsPartyMember == g.combatMovement.IsPartyMember && target.Index == g.combatMovement.EntityIndex &&
			target.Slot == g.combatMovement.Slot {
			g.combatMovement.To = target.Position
		}
		if !effect.fireMovementBound && combatFireMatchesMovement(*effect, g.combatMovement) &&
			effect.fireAdmittedAt == g.gameState.ElapsedMs {
			effect.fireMovement, effect.fireMovementBound = true, true
			effect.Elapsed = 0

			return
		}
	}
}

// The existing movement workflow owns cancellation. A completed arrival starts
// a full local lifetime; cancellation discards only effects bound to that move.
func (g *GameScene) finishCombatFireMovement(arrived bool) {
	write := 0
	for _, effect := range g.combatImpactEffects {
		if effect.fireMovement {
			if !arrived {
				continue
			}
			effect.fireMovement = false
			effect.fireClockAt = g.gameState.ElapsedMs
			effect.Elapsed = 0
		}
		g.combatImpactEffects[write] = effect
		write++
	}
	clear(g.combatImpactEffects[write:])
	g.combatImpactEffects = g.combatImpactEffects[:write]
}

func (g *GameScene) combatFireEffectVisible(effect CombatImpactEffect) bool {
	if effect.FireInjury == nil {
		return true
	}
	if _, disclosed := g.gameState.ProjectCombatFireInjury(*effect.FireInjury, effect.fireOwner); !disclosed {
		return false
	}

	return !effect.fireMovement || g.combatMovement.Progress >= combatFireContactProgress
}

// Live fire labels follow the disclosed actor and the same interpolation as its
// silhouette. Fatal labels retain the resolved square after sprite retirement.
func (g *GameScene) combatImpactLabelPosition(effect CombatImpactEffect) (float64, float64) {
	target := effect.Feedback.Target
	position := target.Position
	if effect.FireInjury != nil {
		if current, disclosed := g.gameState.ProjectCombatFireInjury(*effect.FireInjury, effect.fireOwner); disclosed {
			position = current
		}
	}
	x, y := g.combatLayout().positionToSourcePixels(position)
	if effect.FireInjury == nil || effect.FireInjury.Fatal {
		return x, y
	}
	offsetX, offsetY := g.combatMovementOffset(target.IsPartyMember, target.Index, target.Slot)

	return x + offsetX, y + offsetY
}
