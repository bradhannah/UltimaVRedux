package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// CombatAdjacentSpellTargeting tracks an uncommitted adjacent-square combat spell preview.
type CombatAdjacentSpellTargeting struct {
	Request   CastRequest
	Caster    CombatTurnIdentity
	Origin    references.Position
	Direction references.Direction
	owner     CombatUseOwner
	utility   *CombatUtilitySpellCommit
}

// CombatAdjacentSpellTargetOutcome describes an adjacent spell targeting action.
type CombatAdjacentSpellTargetOutcome int

const (
	// CombatAdjacentSpellTargetIgnored means no adjacent targeting action was active.
	CombatAdjacentSpellTargetIgnored CombatAdjacentSpellTargetOutcome = iota
	// CombatAdjacentSpellTargetAwaitingDirection means selection remains active.
	CombatAdjacentSpellTargetAwaitingDirection
	// CombatAdjacentSpellTargetInvalid means the selected square is not a valid spell target.
	CombatAdjacentSpellTargetInvalid
	// CombatAdjacentSpellTargetCancelled ends selection without spending resources or the combat action.
	CombatAdjacentSpellTargetCancelled
	// CombatAdjacentSpellTargetValidationFailed means final resource validation rejected the cast.
	CombatAdjacentSpellTargetValidationFailed
	// CombatAdjacentSpellTargetResolved means the spell is ready to resolve.
	CombatAdjacentSpellTargetResolved
)

// CombatAdjacentSpellTargetResult returns the confirmed adjacent cast to the UI adapter.
type CombatAdjacentSpellTargetResult struct {
	Outcome    CombatAdjacentSpellTargetOutcome
	Validation SpellValidationResult
	Request    CastRequest
	Origin     references.Position
	Target     references.Position
	Utility    *CombatUtilitySpellCommit
}

// BeginCombatAdjacentSpellTargeting validates and starts an adjacent-square combat spell preview.
// REDUX DEVIATION: original prepared-spell rules charges before targeting; Redux waits for
// a structurally valid final confirmation and preserves the action on cancel.
// See ref:SPELL-0006.
func (g *GameState) BeginCombatAdjacentSpellTargeting(request CastRequest) SpellValidationResult {
	if g.CombatScene == nil || g.CombatScene.hasPlayerActionOwner() {
		return SpellValidationResult{Valid: false, ErrorMsg: "Cannot cast now!"}
	}
	activeIdx, active := g.activeCombatPartyMemberIndex()
	if !active || request.CasterIdx != activeIdx {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not thy turn!"}
	}
	validation := g.ValidateSpellTargetingPrerequisites(request)
	if !validation.Valid {
		return validation
	}
	if references.GetSpellTargetingProfile(request.Spell, references.ContextCombat).Selection != references.TargetAdjacentTile {
		return SpellValidationResult{Valid: false, ErrorMsg: "Spell not implemented!"}
	}
	g.CombatScene.AdjacentSpellTargeting = &CombatAdjacentSpellTargeting{
		Caster:  g.CombatScene.GetCurrentTurnIdentity(),
		Request: request,
		Origin:  g.CombatScene.GetCombatantPosition(g.CombatScene.GetCurrentTurnIdentity()),
		owner:   g.CaptureCombatUseOwner(),
	}
	// Pick a geometric neighbour, never a target-dependent default that could
	// disclose concealed terrain or chest state.
	if IsCombatDoorSpell(request.Spell) {
		for _, direction := range []references.Direction{references.Up, references.Right, references.Down, references.Left} {
			if g.SetCombatAdjacentSpellDirection(direction) != CombatAdjacentSpellTargetInvalid {
				break
			}
		}
	}

	return validation
}

// SetCombatAdjacentSpellDirection updates the selected adjacent square without consuming resources.
func (g *GameState) SetCombatAdjacentSpellDirection(direction references.Direction) CombatAdjacentSpellTargetOutcome {
	if g.CombatScene == nil || g.CombatScene.AdjacentSpellTargeting == nil ||
		g.CombatScene.AdjacentSpellTargeting.utility != nil {
		return CombatAdjacentSpellTargetIgnored
	}
	if direction == references.NoneDirection {
		return CombatAdjacentSpellTargetAwaitingDirection
	}
	targeting := g.CombatScene.AdjacentSpellTargeting
	target := direction.GetNewPositionInDirection(&targeting.Origin)
	if target == nil || !isCombatMapPosition(*target) {
		return CombatAdjacentSpellTargetInvalid
	}
	targeting.Direction = direction

	return CombatAdjacentSpellTargetAwaitingDirection
}

// CancelCombatAdjacentSpellTargeting clears an unresolved adjacent spell without resource cost.
// Cancellation also preserves the current combat action.
func (g *GameState) CancelCombatAdjacentSpellTargeting() CombatAdjacentSpellTargetOutcome {
	if g.CombatScene == nil || g.CombatScene.AdjacentSpellTargeting == nil ||
		g.CombatScene.AdjacentSpellTargeting.utility != nil {
		return CombatAdjacentSpellTargetIgnored
	}
	g.CombatScene.AdjacentSpellTargeting = nil

	return CombatAdjacentSpellTargetCancelled
}

// CombatAdjacentSpellPreview returns the selected adjacent square and whether the UI may identify it as valid.
func (g *GameState) CombatAdjacentSpellPreview() (references.Position, bool, bool) {
	if g.CombatScene == nil || g.CombatScene.AdjacentSpellTargeting == nil {
		return references.Position{}, false, false
	}
	targeting := g.CombatScene.AdjacentSpellTargeting
	if targeting.utility != nil || !g.IsCurrentCombatUseOwner(targeting.owner) {
		return references.Position{}, false, false
	}
	if targeting.Direction == references.NoneDirection {
		return references.Position{}, false, true
	}
	target := targeting.Direction.GetNewPositionInDirection(&targeting.Origin)
	if target == nil || !isCombatMapPosition(*target) {
		return references.Position{}, false, false
	}

	// Confirmation revalidates the target independently so concealment does not change spell semantics.
	valid := g.CombatTerrainAllowsObjectDisplay(*target) &&
		g.isValidCombatAdjacentSpellTarget(targeting.Request.Spell, *target)

	return *target, valid, true
}

func (g *GameState) isValidCombatAdjacentSpellTarget(spell references.Spell, target references.Position) bool {
	if !isCombatMapPosition(target) {
		return false
	}
	//nolint:exhaustive // Only implemented adjacent combat spells require a target rule.
	switch spell {
	case references.AnGrav:
		return g.CombatScene.hasFieldAt(target)
	case references.AnYlem:
		return isAnYlemObject(g.CombatScene.CombatMapRef.GetTileNumber(target))
	case references.AnSanct:
		return isCombatAnSanctTarget(g.CombatScene, target)
	case references.AnExPor:
		return isCombatAnExPorTarget(g.CombatScene, target)
	case references.InExPor:
		return isCombatInExPorTarget(g.CombatScene, target)
	default:
		panic("combat adjacent spell target rule missing")
	}
}

// ConfirmCombatAdjacentSpellTarget validates the selected target before consuming resources.
func (g *GameState) ConfirmCombatAdjacentSpellTarget() CombatAdjacentSpellTargetResult {
	if g.CombatScene == nil || g.CombatScene.AdjacentSpellTargeting == nil ||
		g.CombatScene.AdjacentSpellTargeting.utility != nil {
		return CombatAdjacentSpellTargetResult{Outcome: CombatAdjacentSpellTargetIgnored}
	}
	targeting := g.CombatScene.AdjacentSpellTargeting
	if !g.IsCurrentCombatUseOwner(targeting.owner) {
		g.CombatScene.AdjacentSpellTargeting = nil

		return CombatAdjacentSpellTargetResult{Outcome: CombatAdjacentSpellTargetValidationFailed,
			Validation: SpellValidationResult{ErrorMsg: "Not thy turn!"}}
	}
	if targeting.Direction == references.NoneDirection {
		return CombatAdjacentSpellTargetResult{Outcome: CombatAdjacentSpellTargetAwaitingDirection}
	}
	target := targeting.Direction.GetNewPositionInDirection(&targeting.Origin)
	if target == nil || !g.isValidCombatAdjacentSpellTarget(targeting.Request.Spell, *target) {
		return CombatAdjacentSpellTargetResult{Outcome: CombatAdjacentSpellTargetInvalid}
	}
	targeting.Request.Direction = targeting.Direction
	validation := g.ValidateAndConsumeSpellResources(targeting.Request)
	if !validation.Valid {
		g.CombatScene.AdjacentSpellTargeting = nil

		return CombatAdjacentSpellTargetResult{Outcome: CombatAdjacentSpellTargetValidationFailed, Validation: validation}
	}
	result := CombatAdjacentSpellTargetResult{
		Outcome: CombatAdjacentSpellTargetResolved, Validation: validation,
		Request: targeting.Request, Origin: targeting.Origin, Target: *target,
	}
	if IsCombatDoorSpell(targeting.Request.Spell) {
		targeting.utility = g.newCombatUtilitySpellCommit(targeting, *target)
		result.Utility = targeting.utility

		return result
	}
	g.CombatScene.AdjacentSpellTargeting = nil

	return result
}

// IsCombatDoorSpell identifies the adjacent lock/disarm family sharing a retained workflow.
func IsCombatDoorSpell(spell references.Spell) bool {
	return spell == references.AnSanct || spell == references.AnExPor || spell == references.InExPor
}
