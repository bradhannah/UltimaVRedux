package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ErrReadyEquipment indicates that a Ready selection was rejected without mutation.
var ErrReadyEquipment = errors.New("ready equipment rejected")

// ErrReadyEquipmentStrength identifies a refusal explained by the Strength budget.
var ErrReadyEquipmentStrength = errors.New("ready equipment exceeds Strength")

type readyEquipmentError struct {
	message  string
	strength bool
}

func (e readyEquipmentError) Error() string { return e.message }
func (e readyEquipmentError) Unwrap() error { return ErrReadyEquipment }
func (e readyEquipmentError) Is(target error) bool {
	return e.strength && target == ErrReadyEquipmentStrength
}

func rejectReadyEquipment(message string) error {
	return readyEquipmentError{message: message, strength: false}
}

// ReadyBeginOutcome describes whether Ready was rejected, completed immediately,
// or retained by the equipment dialog.
type ReadyBeginOutcome int

const (
	// ReadyBeginRejected leaves the current action active.
	ReadyBeginRejected ReadyBeginOutcome = iota
	// ReadyBeginCompleted consumes the action without opening a dialog.
	ReadyBeginCompleted
	// ReadyBeginDialog transfers action ownership to the Ready workflow.
	ReadyBeginDialog
)

// ReadyEquipmentRequest binds a Ready dialog to its authoritative target.
// Combat clones retain their exact action identity while targeting the shared PC.
type ReadyEquipmentRequest struct {
	CombatMode     bool
	CharacterIndex int
	LockCharacter  bool
	CombatIdentity CombatTurnIdentity
}

// ReadyEquipmentTransaction describes one equip or unready selection.
type ReadyEquipmentTransaction struct {
	Request          ReadyEquipmentRequest
	CharacterIndex   int
	ExpectedOccupant string
	Equipment        references.Equipment
	// TargetHand is SlotNone for the first available hand, or an explicit hand.
	TargetHand references.EquipmentSlot
	// Confirmed binds a replacement to the exact preview the player reviewed.
	Confirmed *ReadyEquipmentPreview
}

// ReadyEquipmentResult reports a committed Ready mutation and whether its
// source-authentic ring result closes the dialog immediately.
type ReadyEquipmentResult struct {
	Selection ReadyEquipmentSelection
	Changed   bool
	Vanished  bool
	Dismiss   bool
}

// ReadyEquipmentSelection describes the effect of one Ready selection.
type ReadyEquipmentSelection int

const (
	// ReadyEquipmentNoChange covers selectable entries such as arrows and quarrels.
	ReadyEquipmentNoChange ReadyEquipmentSelection = iota
	// ReadyEquipmentEquip moves one carried item into a readied slot.
	ReadyEquipmentEquip
	// ReadyEquipmentUnready clears the first matching readied slot.
	ReadyEquipmentUnready
)

// ReadyEquipmentPreview is the authoritative, non-mutating result of validating a selection.
type ReadyEquipmentPreview struct {
	Selection ReadyEquipmentSelection
	Slot      references.EquipmentSlot
	Before    [references.SlotAmulet + 1]references.Equipment
	After     [references.SlotAmulet + 1]references.Equipment
	Current   ReadyEquipmentTotals
	Resulting ReadyEquipmentTotals
	Strength  int
}

type readyEquipmentPlan struct {
	preview   ReadyEquipmentPreview
	character *party_state.PlayerCharacter
	equipment references.Equipment
}

// BeginReadyEquipment validates action ownership and avoids opening an empty dialog.
func (g *GameState) BeginReadyEquipment(combatMode bool) (ReadyEquipmentRequest, ReadyBeginOutcome) {
	request := ReadyEquipmentRequest{
		CombatMode: combatMode, CharacterIndex: 0, LockCharacter: combatMode,
	}
	if combatMode {
		if g.CombatScene == nil || !g.CombatScene.CombatTurnsStarted() {
			return request, ReadyBeginRejected
		}
		identity := g.CombatScene.GetCurrentTurnIdentity()
		if !g.readyCombatActorValid(identity) {
			g.SystemCallbacks.Message.AddRowStr("Can't!")
			g.SystemCallbacks.Message.ShowCommandPrompt("")

			return request, ReadyBeginRejected
		}
		request.CharacterIndex = identity.EntityIndex
		request.CombatIdentity = identity
	}

	if !g.hasReadyEquipmentOptions(request) {
		g.SystemCallbacks.Message.AddRowStr("Thou art empty-\nhanded!")

		return request, ReadyBeginCompleted
	}

	return request, ReadyBeginDialog
}

func (g *GameState) hasReadyEquipmentOptions(request ReadyEquipmentRequest) bool {
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		if g.PartyState.Inventory.Equipment.Get(equipment) > 0 {
			return true
		}
	}

	start, end := 0, party_state.MaxActiveParty
	if request.LockCharacter {
		start, end = request.CharacterIndex, request.CharacterIndex+1
	}
	for index := start; index < end; index++ {
		character := g.PartyState.GetCharacter(index)
		if character == nil || !character.IsInParty() || !character.IsAlive() {
			continue
		}
		for _, slot := range references.EquipmentSlots() {
			if character.GetEquippedItem(slot) != references.NoEquipment {
				return true
			}
		}
	}

	return false
}

// ApplyReadyEquipment is the single authoritative Ready transaction and policy.
func (g *GameState) ApplyReadyEquipment(
	transaction ReadyEquipmentTransaction,
) (ReadyEquipmentResult, error) {
	result := ReadyEquipmentResult{}
	plan, err := g.planReadyEquipment(transaction)
	if err != nil {
		return result, err
	}
	if transaction.Confirmed != nil && *transaction.Confirmed != plan.preview {
		return result, rejectReadyEquipment("Thy gear has changed. Look it over again.")
	}
	if plan.preview.RequiresConfirmation() && transaction.Confirmed == nil {
		return result, rejectReadyEquipment("Confirm the swap first!")
	}
	result.Selection = plan.preview.Selection
	if plan.preview.Selection == ReadyEquipmentNoChange {
		return result, nil
	}

	if plan.preview.Selection == ReadyEquipmentUnready {
		plan.character.SetEquippedItem(plan.preview.Slot, references.NoEquipment)
		if transaction.Request.CombatMode && plan.equipment == references.RingInvisibility {
			// Original combat initialization clears only the acting combat entry.
			// See ref:EQUIP-0010.
			g.CombatScene.setCombatPCInvisible(transaction.Request.CombatIdentity, false)
		}
		// Original equipment rules silently discards the refund when the carried stack is 99.
		// See ref:EQUIP-0010.
		g.PartyState.Inventory.Equipment.IncrementByOne(plan.equipment)
		result.Changed = true
		// REDUX DEVIATION: equipment selection has no ordinary equipment sound.
		// Redux makes this committed inventory change authorable as a quiet cue.
		// See ref:EQUIP-0010.
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnequip)

		return result, nil
	}

	// The plan verified inventory before mutation; a failure now is an impossible state.
	if !g.PartyState.Inventory.Equipment.DecrementByOne(plan.equipment) {
		panic("ready equipment inventory changed after validation")
	}
	// REDUX DEVIATION: EQUIP-0011: equipment selection requires manual unready before replacement.
	// A reviewed swap returns every displaced item and applies the final loadout
	// atomically, so two-handed transitions cannot leave partially removed gear.
	// See ref:EQUIP-0011, ref:EQUIP-0010.
	for _, slot := range references.EquipmentSlots() {
		before, after := plan.preview.Before[slot], plan.preview.After[slot]
		if before == after {
			continue
		}
		if before != references.NoEquipment {
			g.PartyState.Inventory.Equipment.IncrementByOne(before)
			if transaction.Request.CombatMode && before == references.RingInvisibility {
				g.CombatScene.setCombatPCInvisible(transaction.Request.CombatIdentity, false)
			}
		}
		plan.character.SetEquippedItem(slot, after)
	}
	result.Changed = true

	if (plan.equipment == references.RingInvisibility || plan.equipment == references.RingRegeneration) &&
		g.OneInXOdds(16) {
		// Original equipment rules consume the carried ring first, then clears the equipped slot
		// without returning it when random(0,15) is zero.
		// See ref:EQUIP-0010.
		plan.character.SetEquippedItem(references.SlotRing, references.NoEquipment)
		g.SystemCallbacks.Message.AddRowStr("\n\nRing vanishes!\n")
		result.Vanished = true
		result.Dismiss = true
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundRingVanish)
	}
	if !result.Vanished {
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundEquip)
	}

	return result, nil
}

// PreviewReadyEquipment validates one selection without inventory, equipment, RNG, or UI mutation.
func (g *GameState) PreviewReadyEquipment(
	transaction ReadyEquipmentTransaction,
) (ReadyEquipmentPreview, error) {
	plan, err := g.planReadyEquipment(transaction)

	return plan.preview, err
}

func (g *GameState) planReadyEquipment(transaction ReadyEquipmentTransaction) (readyEquipmentPlan, error) {
	plan := readyEquipmentPlan{equipment: transaction.Equipment}
	character, err := g.validateReadyEquipmentTransaction(transaction)
	if err != nil {
		return plan, err
	}
	plan.character = character
	plan.preview = newReadyEquipmentPreview(character)

	equipment := transaction.Equipment
	if transaction.Request.CombatMode && g.CombatScene != nil && !g.CombatScene.Victory &&
		equipment.GetSlotType() == references.SlotArmour {
		return plan, rejectReadyEquipment("Thou canst not change armour in heated battle!")
	}
	if slot, readied := firstReadiedEquipmentSlot(character, equipment); readied {
		plan.preview.Selection, plan.preview.Slot = ReadyEquipmentUnready, slot
		plan.preview.After[slot] = references.NoEquipment
		plan.preview.Resulting = readyEquipmentTotals(plan.preview.After)

		return plan, nil
	}
	if equipment.IsAmmo() {
		return plan, nil
	}
	if err = plan.preview.selectEquipment(equipment, transaction.TargetHand); err != nil {
		return plan, err
	}
	if g.PartyState.Inventory.Equipment.Get(equipment) == 0 {
		return plan, rejectReadyEquipment(fmt.Sprintf("Thou hast no %s.", equipment.String()))
	}
	if ammo := equipment.GetRequiredAmmo(); ammo != references.NoEquipment &&
		g.PartyState.Inventory.Equipment.Get(ammo) == 0 {
		return plan, rejectReadyEquipment("Thou hast no ammunition for that weapon!")
	}

	if plan.preview.Resulting.Strength > plan.preview.Strength {
		return plan, readyEquipmentError{message: fmt.Sprintf("Needs %d more Strength.",
			plan.preview.Resulting.Strength-plan.preview.Strength), strength: true}
	}

	return plan, g.validateReadyReturns(plan.preview)
}

func firstReadiedEquipmentSlot(
	character *party_state.PlayerCharacter,
	equipment references.Equipment,
) (references.EquipmentSlot, bool) {
	for _, slot := range references.EquipmentSlots() {
		if character.GetEquippedItem(slot) == equipment {
			return slot, true
		}
	}

	return references.SlotNone, false
}

func (g *GameState) validateReadyEquipmentTransaction(
	transaction ReadyEquipmentTransaction,
) (*party_state.PlayerCharacter, error) {
	request := transaction.Request
	if request.LockCharacter && transaction.CharacterIndex != request.CharacterIndex {
		return nil, rejectReadyEquipment("Only the one whose turn it is may ready!")
	}
	if request.CombatMode {
		if g.CombatScene == nil || !g.CombatScene.CombatTurnsStarted() ||
			g.CombatScene.GetCurrentTurnIdentity() != request.CombatIdentity ||
			request.CombatIdentity.EntityIndex != request.CharacterIndex ||
			!g.readyCombatActorValid(request.CombatIdentity) {
			return nil, rejectReadyEquipment("Combat Ready actor changed.")
		}
	}
	if transaction.Equipment < references.LeatherHelm || transaction.Equipment > references.Ankh {
		return nil, rejectReadyEquipment(fmt.Sprintf("%s cannot be readied.", transaction.Equipment))
	}

	character := g.PartyState.GetCharacter(transaction.CharacterIndex)
	if character == nil || !character.IsInParty() || !character.IsAlive() ||
		transaction.ExpectedOccupant == "" || character.GetNameAsString() != transaction.ExpectedOccupant {
		return nil, rejectReadyEquipment("That one can no longer ready!")
	}

	return character, nil
}

func (g *GameState) readyCombatActorValid(identity CombatTurnIdentity) bool {
	if g.CombatScene == nil || !identity.IsPartyMember || identity.EntityIndex < 0 ||
		identity.EntityIndex >= party_state.MaxActiveParty || !g.CombatScene.PartyActive[identity.EntityIndex] ||
		g.CombatScene.TurnQueue == nil {
		return false
	}
	entry := g.CombatScene.TurnQueue.GetEntryByIdentity(identity)

	return entry != nil && entry.status == turnQueueActive && entry.Combatant.IsPC()
}

// ActionReadyCombatMap validates and opens Ready for the exact scheduled PC.
func (g *GameState) ActionReadyCombatMap() ReadyBeginOutcome {
	request, outcome := g.BeginReadyEquipment(true)
	if outcome == ReadyBeginDialog {
		g.SystemCallbacks.Flow.Items.ShowReadyDialog(request)
	}

	return outcome
}
