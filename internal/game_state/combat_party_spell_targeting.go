package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// CombatPartySpellPreview keeps exact battlefield selection separate from shared
// roster effects. Hidden waking-copy positions are never included in this view.
type CombatPartySpellPreview struct {
	Valid                     bool
	Identity                  CombatTurnIdentity
	Name, Status, Consequence string
	HP, MaxHP                 uint16
	Occupant, Occupants       int
	WakingName                string
	WakingPosition            references.Position
	ShowWakingPosition        bool
}

// IsCombatPartySpellTargeting identifies the party-recipient use of the crosshair.
func (g *GameState) IsCombatPartySpellTargeting() bool {
	return g.CombatScene != nil && g.CombatScene.SpellTargeting != nil &&
		references.GetSpellTargetingProfile(g.CombatScene.SpellTargeting.Request.Spell, references.ContextCombat).Selection == references.TargetCombatPartyMember
}

// BeginCombatPartySpellTargeting focuses the exact caster without commitment.
func (g *GameState) BeginCombatPartySpellTargeting(request CastRequest) SpellValidationResult {
	if g.CombatScene == nil || g.CombatScene.hasPlayerActionOwner() {
		return SpellValidationResult{ErrorMsg: "Thou canst not cast now!"}
	}
	if references.GetSpellTargetingProfile(request.Spell, references.ContextCombat).Selection != references.TargetCombatPartyMember {
		return SpellValidationResult{ErrorMsg: "Nothing there!"}
	}
	validation := g.ValidateSpellTargetingPrerequisites(request)
	if !validation.Valid {
		return validation
	}
	caster := g.CombatScene.GetCurrentTurnIdentity()
	origin := g.CombatScene.GetCombatantPosition(caster)
	g.CombatScene.SpellTargeting = &CombatSpellTargeting{
		Request: request, Caster: caster, Origin: origin, Cursor: origin,
		MaxRange: int(references.XCombatMapTiles + references.YCombatMapTiles), Recipient: caster, RecipientSet: true,
	}

	return validation
}

func (g *GameState) visiblePartySpellRecipient(identity CombatTurnIdentity) bool {
	combat := g.CombatScene
	if combat == nil || !identity.IsPartyMember || !g.PartyState.HasCharacter(identity.EntityIndex) {
		return false
	}
	pc := g.PartyState.GetCharacter(identity.EntityIndex)
	entry := combat.TurnQueue.GetEntryByIdentity(identity)
	if entry == nil || !combat.isLivingCombatPCEntry(entry, identity.EntityIndex) || !pc.IsInParty() ||
		combat.isCombatPCUnderground(identity) {
		return false
	}
	position := combat.GetCombatantPosition(identity)

	return position.X >= 0 && position.X < references.XCombatMapTiles && position.Y >= 0 && position.Y < references.YCombatMapTiles &&
		g.CombatTerrainAllowsObjectDisplay(position)
}

// CombatPartySpellRecipients follows source slot order, including sleeping,
// shimmered, possessed and co-located PCs. Friendly monsters are not recipients.
func (g *GameState) CombatPartySpellRecipients() []CombatTurnIdentity {
	var recipients []CombatTurnIdentity
	if g.CombatScene == nil {
		return recipients
	}
	for slot := range sourceCombatQueueSlots {
		entry := g.CombatScene.TurnQueue.getActiveEntryAtSourceSlot(slot)
		if entry == nil {
			continue
		}
		identity := turnQueueIdentity(entry)
		if g.visiblePartySpellRecipient(identity) {
			recipients = append(recipients, identity)
		}
	}

	return recipients
}

// CycleCombatPartySpellRecipient selects the next eligible exact entry, wrapping.
func (g *GameState) CycleCombatPartySpellRecipient() {
	if !g.IsCombatPartySpellTargeting() {
		return
	}
	recipients := g.CombatPartySpellRecipients()
	if len(recipients) == 0 {
		return
	}
	targeting := g.CombatScene.SpellTargeting
	next := 0
	if targeting.RecipientSet {
		for index, identity := range recipients {
			if identity == targeting.Recipient {
				next = (index + 1) % len(recipients)

				break
			}
		}
	}
	targeting.Recipient, targeting.RecipientSet = recipients[next], true
	targeting.Cursor = g.CombatScene.GetCombatantPosition(targeting.Recipient)
}

func (g *GameState) combatRecipientName(identity CombatTurnIdentity) string {
	name := g.PartyState.GetCharacter(identity.EntityIndex).GetNameAsString()
	if identity.Slot != 0 {
		return fmt.Sprintf("%s - Clone %d", name, identity.Slot)
	}

	return name + " - Original"
}

// ReviewCombatPartySpellRecipient is a read-only query shared with confirmation.
func (g *GameState) ReviewCombatPartySpellRecipient() CombatPartySpellPreview {
	preview := CombatPartySpellPreview{}
	if !g.IsCombatPartySpellTargeting() {
		return preview
	}
	targeting := g.CombatScene.SpellTargeting
	identity := targeting.Recipient
	if !targeting.RecipientSet || !g.visiblePartySpellRecipient(identity) || g.CombatScene.GetCombatantPosition(identity) != targeting.Cursor {
		return preview
	}
	pc := g.PartyState.GetCharacter(identity.EntityIndex)
	preview.Valid, preview.Identity = true, identity
	preview.Name, preview.HP, preview.MaxHP = g.combatRecipientName(identity), pc.GetCurrentHp(), pc.GetMaxHp()
	preview.Status = "Good"
	if pc.IsPoisoned() {
		preview.Status = "Poisoned"
	}
	if pc.IsAsleep() {
		preview.Status = "Asleep"
	}
	if g.CombatScene.isCombatPCSleeping(identity) {
		preview.Status += "; sleeping copy"
	}
	if g.CombatScene.isCombatPCPossessed(identity) {
		preview.Status += "; possessed"
	}
	for _, candidate := range g.CombatPartySpellRecipients() {
		if g.CombatScene.GetCombatantPosition(candidate) == targeting.Cursor {
			preview.Occupants++
			if candidate == identity {
				preview.Occupant = preview.Occupants
			}
		}
	}
	switch targeting.Request.Spell { //nolint:exhaustive // Only the four party-recipient spells enter this selector.
	case references.Mani, references.VasMani:
		preview.Consequence = "Heals wounds, not poison."
		if preview.HP == preview.MaxHP {
			preview.Consequence = "No need of this art! The spell would still be spent."
		}
	case references.AnNox:
		preview.Consequence = "Cures poison, not wounds."
		if !pc.IsPoisoned() {
			preview.Consequence = "Not poisoned: no effect! The spell would still be spent."
		}
	case references.AnZu:
		preview.Consequence = "Awakens the first eligible sleeping copy."
		if !pc.IsAsleep() {
			preview.Consequence = "Not asleep: no effect! The spell would still be spent."

			break
		}
		for slot := range sourceCombatQueueSlots {
			entry := g.CombatScene.TurnQueue.getActiveEntryAtSourceSlot(slot)
			if entry == nil || !entry.Combatant.IsPC() || entry.EntityIndex != identity.EntityIndex {
				continue
			}
			waking := turnQueueIdentity(entry)
			if !g.CombatScene.isCombatPCSleeping(waking) {
				continue
			}
			preview.WakingName = g.combatRecipientName(waking)
			if g.visiblePartySpellRecipient(waking) {
				preview.WakingPosition, preview.ShowWakingPosition = g.CombatScene.GetCombatantPosition(waking), true
			}

			break
		}
	}

	return preview
}

// CombatPartySpellResult describes one committed roster effect and its receipt.
type CombatPartySpellResult struct {
	Outcome    CombatSpellTargetOutcome
	Validation SpellValidationResult
	Target     CombatTurnIdentity
	Spell      references.Spell
	Applied    bool
	Healed     uint16
}

// ConfirmCombatPartySpellRecipient revalidates exact identities then commits and
// applies the existing roster effect once. No projectile/LOS rules participate.
func (g *GameState) ConfirmCombatPartySpellRecipient() CombatPartySpellResult {
	result := CombatPartySpellResult{Outcome: CombatSpellTargetIgnored}
	if !g.IsCombatPartySpellTargeting() {
		return result
	}
	targeting := g.CombatScene.SpellTargeting
	result.Spell = targeting.Request.Spell
	if g.CombatScene.GetCurrentTurnIdentity() != targeting.Caster {
		g.CombatScene.SpellTargeting = nil
		result.Outcome, result.Validation = CombatSpellTargetValidationFailed, SpellValidationResult{ErrorMsg: "Casting context changed!"}

		return result
	}
	preview := g.ReviewCombatPartySpellRecipient()
	if !preview.Valid {
		result.Outcome = CombatSpellTargetInvalid

		return result
	}
	request := targeting.Request
	request.TargetIdx = preview.Identity.EntityIndex
	result.Validation = g.ValidateAndConsumeSpellResources(request)
	if !result.Validation.Valid {
		g.CombatScene.SpellTargeting = nil
		result.Outcome = CombatSpellTargetValidationFailed

		return result
	}
	g.CombatScene.SpellTargeting = nil
	result.Outcome, result.Target = CombatSpellTargetResolved, preview.Identity
	pc := g.PartyState.GetCharacter(request.TargetIdx)
	before := pc.GetCurrentHp()
	switch request.Spell { //nolint:exhaustive // Validated party-spell profile.
	case references.Mani:
		result.Applied = g.CastMani(request.TargetIdx).Applied
	case references.VasMani:
		result.Applied = g.CastVasMani(request.TargetIdx).Applied
	case references.AnNox:
		result.Applied = g.CastAnNox(request.TargetIdx).Applied
	case references.AnZu:
		result.Applied = g.AwakenCharacter(request.TargetIdx)
	}
	result.Healed = pc.GetCurrentHp() - before

	return result
}
