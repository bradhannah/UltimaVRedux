package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// CombatConeTargeting tracks an uncommitted directional spell preview.
type CombatConeTargeting struct {
	Request   CastRequest
	Caster    CombatTurnIdentity
	Origin    references.Position
	Spec      CombatConeSpec
	Direction references.Direction
}

// CombatConeTargetOutcome describes a directional cone targeting action.
type CombatConeTargetOutcome int

const (
	// CombatConeTargetIgnored means no directional cone targeting action was active.
	CombatConeTargetIgnored CombatConeTargetOutcome = iota
	// CombatConeTargetAwaitingDirection means selection remains active until a cardinal direction is chosen.
	CombatConeTargetAwaitingDirection
	// CombatConeTargetCancelled is terminal for the accepted Cast action but preserves resources.
	CombatConeTargetCancelled
	// CombatConeTargetValidationFailed means final resource validation rejected the cast.
	CombatConeTargetValidationFailed
	// CombatConeTargetResolved means the spell was committed and can begin presentation.
	CombatConeTargetResolved
)

// CombatConeTargetResult returns the confirmed directional cast to the UI adapter.
type CombatConeTargetResult struct {
	Outcome    CombatConeTargetOutcome
	Validation SpellValidationResult
	Request    CastRequest
	Origin     references.Position
	Positions  []references.Position
	committed  bool
	candidates []references.Position
}

// BeginCombatConeTargeting validates and starts a directional combat spell preview.
// Resources and turn ownership remain unchanged until confirmation.
// REDUX DEVIATION: original prepared-spell rules charges before targeting; Redux makes
// cancellation free but pays for a confirmed cone even when it affects nobody.
// See ref:SPELL-0003.
func (g *GameState) BeginCombatConeTargeting(request CastRequest) SpellValidationResult {
	if g.CombatScene == nil || g.CombatScene.hasPlayerActionOwner() {
		return SpellValidationResult{Valid: false, ErrorMsg: "Thou canst not cast now!"}
	}
	activeIdx, active := g.activeCombatPartyMemberIndex()
	if !active || request.CasterIdx != activeIdx {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not thy turn!"}
	}
	validation := g.ValidateSpellTargetingPrerequisites(request)
	if !validation.Valid {
		return validation
	}
	spec, ok := CombatConeSpecForSpell(request.Spell)
	if !ok {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not here!"}
	}
	g.CombatScene.ConeTargeting = &CombatConeTargeting{
		Caster:  g.CombatScene.GetCurrentTurnIdentity(),
		Request: request,
		Origin:  g.CombatScene.GetCombatantPosition(g.CombatScene.GetCurrentTurnIdentity()),
		Spec:    spec,
	}

	return validation
}

// SetCombatConeDirection changes the active preview without consuming resources.
func (g *GameState) SetCombatConeDirection(direction references.Direction) CombatConeTargetOutcome {
	if g.CombatScene == nil || g.CombatScene.ConeTargeting == nil {
		return CombatConeTargetIgnored
	}
	if direction == references.NoneDirection {
		return CombatConeTargetAwaitingDirection
	}
	g.CombatScene.ConeTargeting.Direction = direction

	return CombatConeTargetAwaitingDirection
}

// CancelCombatConeTargeting clears an uncommitted cone spell without consuming resources.
// Cancellation also preserves the current combat action.
func (g *GameState) CancelCombatConeTargeting() CombatConeTargetOutcome {
	if g.CombatScene == nil || g.CombatScene.ConeTargeting == nil {
		return CombatConeTargetIgnored
	}
	g.CombatScene.ConeTargeting = nil

	return CombatConeTargetCancelled
}

// ConfirmCombatConeTarget consumes the spell and returns the same footprint shown in preview.
func (g *GameState) ConfirmCombatConeTarget() CombatConeTargetResult {
	if g.CombatScene == nil || g.CombatScene.ConeTargeting == nil {
		return CombatConeTargetResult{Outcome: CombatConeTargetIgnored}
	}
	targeting := g.CombatScene.ConeTargeting
	if g.CombatScene.GetCurrentTurnIdentity() != targeting.Caster {
		g.CombatScene.ConeTargeting = nil

		return CombatConeTargetResult{Outcome: CombatConeTargetValidationFailed,
			Validation: SpellValidationResult{ErrorMsg: "Not thy turn!"}}
	}
	if targeting.Direction == references.NoneDirection {
		return CombatConeTargetResult{Outcome: CombatConeTargetAwaitingDirection}
	}
	targeting.Request.Direction = targeting.Direction
	validation := g.ValidateAndConsumeSpellResources(targeting.Request)
	if !validation.Valid {
		g.CombatScene.ConeTargeting = nil

		return CombatConeTargetResult{Outcome: CombatConeTargetValidationFailed, Validation: validation}
	}
	result := CombatConeTargetResult{
		Outcome:    CombatConeTargetResolved,
		Validation: validation,
		Request:    targeting.Request,
		Origin:     targeting.Origin,
		Positions:  g.tracePossibleCombatCone(targeting.Origin, targeting.Direction, targeting.Spec),
		committed:  true,
		candidates: g.collectCommittedCombatCone(targeting.Origin, targeting.Direction, targeting.Spec),
	}
	g.CombatScene.ConeTargeting = nil

	return result
}

// CombatConePreview returns the currently selected footprint, or nil before direction selection.
func (g *GameState) CombatConePreview() []references.Position {
	if g.CombatScene == nil || g.CombatScene.ConeTargeting == nil ||
		g.CombatScene.ConeTargeting.Direction == references.NoneDirection {
		return nil
	}
	targeting := g.CombatScene.ConeTargeting

	return g.tracePossibleCombatCone(targeting.Origin, targeting.Direction, targeting.Spec)
}
