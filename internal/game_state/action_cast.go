package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	// SpellNoMagicAbilityMessage distinguishes classes that can never cast spells.
	SpellNoMagicAbilityMessage = "No magical ability!"
	// SpellNoMPMessage distinguishes a spellcaster whose MP is fully depleted.
	SpellNoMPMessage = "No M.P. remaining!"
	// SpellMPTooLowMessage matches the original U5 insufficient-MP message.
	SpellMPTooLowMessage = "M.P. too low!"
	// SpellNotImplementedMessage identifies an originally legal context that Redux cannot execute yet.
	SpellNotImplementedMessage = "Spell not implemented!"
	// VasRelPorShipMessage explains the original frigate-only travel restriction.
	VasRelPorShipMessage = "Not aboard ship!"
)

// CastRequest represents a complete spell casting request with all targeting resolved.
// Created by the spell casting workflow after caster selection, spell selection,
// and target selection (if needed based on spell's TargetingType).
type CastRequest struct {
	Spell     references.Spell     // The spell to cast
	CasterIdx int                  // Party member index (0 = Avatar)
	TargetIdx int                  // Target party member index for TargetPartyMember.
	Direction references.Direction // Target direction (for TargetDirection spells)
	MoonPhase datetime.MoonPhase   // Selected destination phase for TargetMoonPhase.
}

// SpellRefusal names the refusals a screen words for itself. Any other refusal
// is described by ErrorMsg alone.
type SpellRefusal int

// Refusals the Cast list explains in its own sentence.
const (
	SpellRefusalOther SpellRefusal = iota
	SpellRefusalNoMagic
	SpellRefusalNotHere
	SpellRefusalNotUntilVictory
	SpellRefusalNoneMixed
	SpellRefusalNoMP
	SpellRefusalLevelTooLow
)

// SpellValidationResult contains the result of spell validation.
// Used by the spell casting workflow to determine how to proceed.
type SpellValidationResult struct {
	Valid     bool   // Whether the spell can be cast
	ErrorMsg  string // Error message if invalid (empty if valid)
	SpellInfo *references.SpellInfo
	IsAbsorb  bool         // Whether magic was absorbed (special case for sound)
	Refusal   SpellRefusal // Which refusal, when a screen words the reason itself
}

// ValidateAndConsumeSpellResources validates a spell cast request and consumes resources.
// Call this BEFORE dispatching to spell effect builders.
// Returns validation result indicating success or error.
//
// REDUX DEVIATION: original prepared-spell rules consume before MP/level checks and targeting.
// Redux validates prerequisites and the final target first; confirmed effect
// failures remain paid. Selection and cancellation must use validation only.
// On success: Spell count and MP are consumed unless the debug resource override applies.
// On failure: No resources consumed, caller should show error message from result.
// See ref:SPELL-0001.
func (g *GameState) ValidateAndConsumeSpellResources(request CastRequest) SpellValidationResult {
	return g.validateSpellResources(request, true)
}

// ValidateSpellResources validates a spell cast without mutating party resources.
func (g *GameState) ValidateSpellResources(request CastRequest) SpellValidationResult {
	return g.validateSpellResources(request, false)
}

// ValidateSpellTargetingPrerequisites validates a prepared cast before opening
// an effect-specific target selector. It never commits MP or a mixed spell.
func (g *GameState) ValidateSpellTargetingPrerequisites(request CastRequest) SpellValidationResult {
	return g.validateSpellResourcesWithoutTarget(request)
}

// ValidateVasRelPorPrerequisites validates everything required before asking
// for a destination phase. It never commits MP or a prepared spell.
func (g *GameState) ValidateVasRelPorPrerequisites(request CastRequest) SpellValidationResult {
	if request.Spell != references.VasRelPor {
		panic("ValidateVasRelPorPrerequisites called for another spell")
	}
	// REDUX DEVIATION: prepared casting commits resources before moonstone teleportation
	// validates the ship, phase input, and planted moonstone. Redux validates all
	// three before atomic commitment in every noncombat context.
	// See ref:SPELL-0001.
	vehicle := g.PartyVehicle.GetVehicleDetails().VehicleType
	if vehicle == references.FrigateVehicle || vehicle == references.SkiffVehicle {
		return SpellValidationResult{Valid: false, ErrorMsg: VasRelPorShipMessage}
	}

	return g.ValidateSpellResources(request)
}

func (g *GameState) validateSpellResources(request CastRequest, consume bool) SpellValidationResult {
	return g.validateSpellResourcesWithTargetPolicy(request, consume, true)
}

func (g *GameState) validateSpellResourcesWithoutTarget(request CastRequest) SpellValidationResult {
	return g.validateSpellResourcesWithTargetPolicy(request, false, false)
}

func (g *GameState) validateSpellResourcesWithTargetPolicy(
	request CastRequest,
	consume bool,
	validateTarget bool,
) SpellValidationResult {
	spellInfo := references.GetSpellInfo(request.Spell)
	if spellInfo == nil {
		return SpellValidationResult{Valid: false, ErrorMsg: "Unknown spell!"}
	}

	if !g.PartyState.HasCharacter(request.CasterIdx) {
		return SpellValidationResult{Valid: false, ErrorMsg: "Invalid caster!"}
	}
	caster := g.PartyState.GetCharacter(request.CasterIdx)
	if !caster.IsInParty() || !caster.IsAlive() {
		return SpellValidationResult{ErrorMsg: "Invalid caster!"}
	}
	if g.CombatScene != nil {
		identity := g.CombatScene.GetCurrentTurnIdentity()
		if !identity.IsPartyMember || identity.EntityIndex != request.CasterIdx {
			return SpellValidationResult{Valid: false, ErrorMsg: "Not thy turn!"}
		}
		if !g.isActiveCombatIdentity(identity) || g.CombatScene.isCombatPCSleeping(identity) ||
			g.CombatScene.isCombatPCUnderground(identity) ||
			g.combatIdentityLoyalty(identity) != CombatMonsterLoyaltyParty {
			return SpellValidationResult{ErrorMsg: "Invalid caster!"}
		}
	} else if caster.IsAsleep() {
		return SpellValidationResult{ErrorMsg: "Invalid caster!"}
	}
	if !caster.CanCastSpells() {
		return SpellValidationResult{Valid: false, ErrorMsg: SpellNoMagicAbilityMessage, Refusal: SpellRefusalNoMagic}
	}
	unlimitedResources := g.DebugOptions.UnlimitedCastingResources && caster.GetMaxMp() > 0

	// Check for magic absorption at special locations
	if g.isMagicAbsorbed() {
		return SpellValidationResult{Valid: false, ErrorMsg: "Absorbed!", IsAbsorb: true}
	}

	// Check spell context (overworld, town, dungeon, combat)
	currentContext := g.GetCurrentSpellContext()
	if !spellInfo.Context.HasContext(currentContext) {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not here!", Refusal: SpellRefusalNotHere}
	}
	if !IsImplementedSpellInContext(request.Spell, currentContext) {
		return SpellValidationResult{Valid: false, ErrorMsg: SpellNotImplementedMessage}
	}
	if request.Spell == references.AnTym && g.isAnTymAbsorbed() {
		// REDUX DEVIATION: prepared casting commits resources before Time Stop
		// scans for a Shadowlord. Redux treats that scan as effect validation and
		// preserves the prepared spell and MP when the magic is absorbed.
		// See ref:SPELL-0001.
		return SpellValidationResult{Valid: false, ErrorMsg: "Absorbed!", IsAbsorb: true}
	}
	if currentContext == references.ContextCombat && g.HasActiveDurationEffect(DurationEffectNegateMagic) {
		return SpellValidationResult{Valid: false, ErrorMsg: "Absorbed!", IsAbsorb: true}
	}
	if currentContext == references.ContextCombat && request.Spell == references.VasMani && !g.CombatScene.Victory {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not until victory!", Refusal: SpellRefusalNotUntilVictory}
	}
	if currentContext == references.ContextCombat && request.Spell == references.InPor &&
		g.CombatScene.ExitContext != CombatExitSurface {
		return SpellValidationResult{Valid: false, ErrorMsg: "Not here!", Refusal: SpellRefusalNotHere}
	}
	if !unlimitedResources && caster.GetCurrentMp() == 0 {
		return SpellValidationResult{Valid: false, ErrorMsg: SpellNoMPMessage, Refusal: SpellRefusalNoMP}
	}

	// Check mixed spell count
	mixedCount := g.PartyState.Inventory.Spells.Get(request.Spell)
	if !unlimitedResources && mixedCount == 0 {
		return SpellValidationResult{Valid: false, ErrorMsg: "None mixed!", Refusal: SpellRefusalNoneMixed}
	}

	// Check MP - cost equals circle number
	circle := spellInfo.Circle
	if !unlimitedResources && int(caster.GetCurrentMp()) < circle {
		return SpellValidationResult{Valid: false, ErrorMsg: SpellMPTooLowMessage, Refusal: SpellRefusalNoMP}
	}

	// Check caster level - must be >= spell circle.
	// REDUX DEVIATION: SPELL-0001: circle study lifts the limit by one; ref:PROGRESS-0005.
	if !g.DebugOptions.BypassSpellLevelRestrictions && caster.SpellCircleLimit() < circle {
		return SpellValidationResult{Valid: false, ErrorMsg: "Level too low!", Refusal: SpellRefusalLevelTooLow}
	}
	if validateTarget && !g.isPreparedSpellEffectTargetValid(request) {
		message := "Invalid target!"
		if currentContext == references.ContextDungeon &&
			(isDungeonPackedSpell(request.Spell) || isDungeonFloorSpell(request.Spell)) {
			message = "Failed!"
		}

		return SpellValidationResult{Valid: false, ErrorMsg: message}
	}

	if consume && !unlimitedResources {
		g.PartyState.Inventory.Spells.DecrementByOne(request.Spell)
		caster.SubtractMp(byte(circle))
	}
	if consume {
		g.CommitCastUseAttempt()
	}

	return SpellValidationResult{Valid: true, SpellInfo: spellInfo}
}

// implementedSpellContexts is the authoritative prepared-spell support table.
// Original legal contexts remain in references.SpellInfo.Context.
var implementedSpellContexts = [references.TotalSpellCount]references.SpellContext{ //nolint:gochecknoglobals // Immutable support catalog.
	references.InLor:         references.ContextNonCombat,
	references.GravPor:       references.ContextCombat,
	references.AnZu:          references.ContextAll,
	references.AnNox:         references.ContextAll,
	references.Mani:          references.ContextAll,
	references.AnYlem:        references.ContextTown | references.ContextCombat,
	references.AnSanct:       references.ContextAll,
	references.AnXenCorp:     references.ContextCombat,
	references.RelHur:        references.ContextOverworld,
	references.InWis:         references.ContextOverworld,
	references.KalXen:        references.ContextCombat,
	references.InXenMani:     references.ContextAll,
	references.VasLor:        references.ContextNonCombat,
	references.VasFlam:       references.ContextCombat,
	references.InFlamGrav:    references.ContextDungeon | references.ContextCombat,
	references.InNoxGrav:     references.ContextDungeon | references.ContextCombat,
	references.InZuGrav:      references.ContextDungeon | references.ContextCombat,
	references.InPor:         references.ContextOverworld | references.ContextCombat,
	references.AnGrav:        references.ContextDungeon | references.ContextCombat,
	references.InSanct:       references.ContextAll,
	references.InSanctGrav:   references.ContextDungeon | references.ContextCombat,
	references.UusPor:        references.ContextDungeon,
	references.DesPor:        references.ContextDungeon,
	references.WisQuas:       references.ContextCombat,
	references.InBetXen:      references.ContextCombat,
	references.AnExPor:       references.ContextTown | references.ContextCombat,
	references.InExPor:       references.ContextTown | references.ContextCombat,
	references.VasMani:       references.ContextAll,
	references.InZu:          references.ContextCombat,
	references.RelTym:        references.ContextAll,
	references.InVasPorYlem:  references.ContextCombat,
	references.QuasAnWis:     references.ContextCombat,
	references.InAn:          references.ContextAll,
	references.WisAnYlem:     references.ContextOverworld | references.ContextTown,
	references.AnXenEx:       references.ContextCombat,
	references.RelXenBet:     references.ContextCombat,
	references.SanctLor:      references.ContextCombat,
	references.XenCorp:       references.ContextCombat,
	references.InQuasXen:     references.ContextCombat,
	references.InQuasWis:     references.ContextNonCombat,
	references.InNoxHur:      references.ContextCombat,
	references.InQuasCorp:    references.ContextCombat,
	references.InManiCorp:    references.ContextNonCombat,
	references.KalXenCorp:    references.ContextCombat,
	references.InVasGravCorp: references.ContextCombat,
	references.InFlamHur:     references.ContextCombat,
	references.VasRelPor:     references.ContextNonCombat,
	references.AnTym:         references.ContextAll,
}

// GetImplementedSpellContexts returns the Redux prepared-spell support bitmask.
func GetImplementedSpellContexts(spell references.Spell) references.SpellContext {
	if references.GetSpellInfo(spell) == nil {
		return 0
	}

	return implementedSpellContexts[spell]
}

// IsImplementedSpellInContext reports whether validation may commit resources.
func IsImplementedSpellInContext(spell references.Spell, context references.SpellContext) bool {
	return GetImplementedSpellContexts(spell).HasContext(context)
}

// GetCurrentSpellContext returns the SpellContext for the current map type.
func (g *GameState) GetCurrentSpellContext() references.SpellContext {
	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.LargeMapType:
		return references.ContextOverworld
	case references.SmallMapType:
		return references.ContextTown
	case references.DungeonMapType:
		return references.ContextDungeon
	case references.CombatMapType:
		return references.ContextCombat
	case references.CampingMapType:
		return 0
	default:
		return 0
	}
}

// isMagicAbsorbed checks the context-specific original location gates.
func (g *GameState) isMagicAbsorbed() bool {
	location := g.MapState.PlayerLocation.Location
	combat := g.GetCurrentSpellContext() == references.ContextCombat && g.CombatScene != nil
	if combat {
		location = g.CombatScene.PreCombatPlayerLocation.Location
	}

	// Original prepared-spell rules block Stonegate outside combat. original combat scheduler only carries the
	// Palace gate into an encounter.
	// See ref:SPELL-0001.
	if !combat && references.IsMagicBlockedLocation(location) {
		return true
	}

	// The original crown flag records ownership, not whether Redux has donned it.
	if location == references.PalaceOfBlackthorn &&
		!g.PartyState.Inventory.QuestItems.HasSome(references.Crown) {
		return true
	}

	return false
}

func (g *GameState) isPreparedSpellEffectTargetValid(request CastRequest) bool {
	context := g.GetCurrentSpellContext()
	if context == references.ContextDungeon && isDungeonPackedSpell(request.Spell) {
		// REDUX DEVIATION: prepared casting consumes the mixed spell and MP before
		// these handlers reject their packed target. Redux preserves resources by
		// validating the implicit dungeon target before atomic commitment.
		// See ref:SPELL-0001.
		return g.CanCastDungeonPackedSpell(request.Spell)
	}
	if context == references.ContextDungeon && isDungeonFloorSpell(request.Spell) {
		// REDUX DEVIATION: prepared casting commits resources before vertical dungeon travel
		// rejects Doom or a non-passage destination, and blocked vertical dungeon travel also
		// prints a contradictory final "Success!". Redux validates atomically and
		// reports only failure before committing the prepared spell and MP.
		// See ref:SPELL-0001.
		return g.CanCastDungeonFloorSpell(request.Spell)
	}

	profile := references.GetSpellTargetingProfile(request.Spell, context)
	if profile.Selection == references.TargetDirection || profile.Selection == references.TargetAdjacentTile {
		if request.Direction != references.Up && request.Direction != references.Down &&
			request.Direction != references.Left && request.Direction != references.Right {
			return false
		}
		if context == references.ContextTown {
			target := request.Direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)

			return target != nil && !g.IsOutOfBounds(*target)
		}

		return true
	}
	if profile.Selection != references.TargetPartyMember {
		return true
	}

	return g.PartyState.HasCharacter(request.TargetIdx) && g.PartyState.GetCharacter(request.TargetIdx).IsInParty()
}
