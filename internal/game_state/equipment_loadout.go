package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ErrInvalidEquipmentLoadout indicates that a requested loadout would violate party or inventory invariants.
var ErrInvalidEquipmentLoadout = errors.New("invalid equipment loadout")

// EquipmentSource identifies where a newly equipped item comes from.
type EquipmentSource string

const (
	// EquipmentSourceInventory consumes one carried item.
	EquipmentSourceInventory EquipmentSource = "inventory"
	// EquipmentSourceManufacture creates the equipped item without consuming carried inventory.
	EquipmentSourceManufacture EquipmentSource = "manufacture"
)

// EquipmentSlotChange describes one slot in a partial loadout alteration.
// NoEquipment explicitly unequips the slot; omitted slots remain unchanged.
type EquipmentSlotChange struct {
	Slot      references.EquipmentSlot
	Equipment references.Equipment
	Source    EquipmentSource
}

// EquipmentLoadoutAlteration describes one atomic partial-loadout update.
type EquipmentLoadoutAlteration struct {
	CharacterSlot    int
	ExpectedOccupant string
	Changes          []EquipmentSlotChange
}

// EquipmentLoadoutStateChange describes one authoritative loadout or inventory value that changed.
type EquipmentLoadoutStateChange struct {
	Path   string
	Before int
	After  int
}

// EquipmentLoadoutAlterationResult reports the target and exact applied slot and inventory changes.
type EquipmentLoadoutAlterationResult struct {
	CharacterSlot int
	CharacterName string
	Changes       []EquipmentLoadoutStateChange
}

type equipmentLoadoutPlan struct {
	input         EquipmentLoadoutAlteration
	character     *party_state.PlayerCharacter
	current       map[references.EquipmentSlot]references.Equipment
	requested     map[references.EquipmentSlot]references.Equipment
	changeSources map[references.EquipmentSlot]EquipmentSource
	changedSlots  []references.EquipmentSlot
}

// AlterEquipmentLoadout validates and atomically applies a partial character loadout.
func (g *GameState) AlterEquipmentLoadout(
	input EquipmentLoadoutAlteration,
) (EquipmentLoadoutAlterationResult, error) {
	result := EquipmentLoadoutAlterationResult{
		CharacterSlot: input.CharacterSlot,
		CharacterName: "",
		Changes:       nil,
	}
	plan, err := g.planEquipmentLoadout(input)
	if err != nil {
		return result, err
	}

	inventoryDeltas := make(map[references.Equipment]int)
	for _, slot := range plan.changedSlots {
		if oldEquipment := plan.current[slot]; oldEquipment != references.NoEquipment {
			inventoryDeltas[oldEquipment]++
		}
		newEquipment := plan.requested[slot]
		if newEquipment != references.NoEquipment && plan.changeSources[slot] == EquipmentSourceInventory {
			inventoryDeltas[newEquipment]--
		}
	}

	resultingInventory := make(map[references.Equipment]uint16, len(inventoryDeltas))
	for equipment, delta := range inventoryDeltas {
		before := int(g.PartyState.Inventory.Equipment.Get(equipment))
		after := before + delta
		if after < 0 {
			return result, fmt.Errorf("%w: %d more carried %s required", ErrInvalidEquipmentLoadout, -after, equipment)
		}
		if after > int(party_state.ItemQuantitySmallMax) {
			return result, fmt.Errorf("%w: carried %s cannot exceed %d", ErrInvalidEquipmentLoadout, equipment, party_state.ItemQuantitySmallMax)
		}
		resultingInventory[equipment] = uint16(after)
	}

	result.CharacterName = plan.character.GetNameAsString()
	result.Changes = make([]EquipmentLoadoutStateChange, 0, len(plan.changedSlots))
	for _, slot := range plan.changedSlots {
		before := plan.current[slot]
		after := plan.requested[slot]
		plan.character.SetEquippedItem(slot, after)
		result.Changes = append(result.Changes, EquipmentLoadoutStateChange{
			Path:   fmt.Sprintf("party.roster.%d.equipment.%s", plan.input.CharacterSlot, equipmentSlotID(slot)),
			Before: before.ID(),
			After:  after.ID(),
		})
	}
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		after, changed := resultingInventory[equipment]
		if !changed {
			continue
		}
		before := g.PartyState.Inventory.Equipment.Get(equipment)
		if before == after {
			continue
		}
		g.PartyState.Inventory.Equipment.Set(equipment, after)
		result.Changes = append(result.Changes, EquipmentLoadoutStateChange{
			Path:   fmt.Sprintf("inventory.equipment.%d.quantity", equipment.ID()),
			Before: int(before),
			After:  int(after),
		})
	}

	return result, nil
}

func (g *GameState) planEquipmentLoadout(input EquipmentLoadoutAlteration) (equipmentLoadoutPlan, error) {
	plan := equipmentLoadoutPlan{input: input, character: nil, current: nil, requested: nil, changeSources: nil, changedSlots: nil}
	character, err := g.validateEquipmentLoadoutTarget(input)
	if err != nil {
		return plan, err
	}
	plan.character = character

	current := equipmentLoadout(character)
	requested := make(map[references.EquipmentSlot]references.Equipment, len(current))
	for slot, equipment := range current {
		requested[slot] = equipment
	}
	changeSources := make(map[references.EquipmentSlot]EquipmentSource, len(input.Changes))
	for _, change := range input.Changes {
		if err := validateEquipmentSlotChange(change); err != nil {
			return plan, err
		}
		if _, duplicate := changeSources[change.Slot]; duplicate {
			return plan, fmt.Errorf("%w: slot %s was requested more than once", ErrInvalidEquipmentLoadout, change.Slot)
		}
		changeSources[change.Slot] = change.Source
		requested[change.Slot] = change.Equipment
	}
	if len(changeSources) == 0 {
		return plan, fmt.Errorf("%w: at least one equipment slot change is required", ErrInvalidEquipmentLoadout)
	}
	if err := g.validateResultingEquipmentLoadout(character, current, requested); err != nil {
		return plan, err
	}

	changedSlots := make([]references.EquipmentSlot, 0, len(changeSources))
	for _, slot := range references.EquipmentSlots() {
		newEquipment, requestedChange := requested[slot]
		if !requestedChange || current[slot] == newEquipment {
			continue
		}
		if _, included := changeSources[slot]; !included {
			continue
		}
		changedSlots = append(changedSlots, slot)
	}

	plan.current = current
	plan.requested = requested
	plan.changeSources = changeSources
	plan.changedSlots = changedSlots

	return plan, nil
}

func (g *GameState) validateEquipmentLoadoutTarget(
	input EquipmentLoadoutAlteration,
) (*party_state.PlayerCharacter, error) {
	if input.CharacterSlot < 0 || input.CharacterSlot >= party_state.MaxActiveParty {
		return nil, fmt.Errorf("%w: character position %d is outside the active party", ErrInvalidEquipmentLoadout, input.CharacterSlot+1)
	}
	character := g.PartyState.GetCharacter(input.CharacterSlot)
	if character == nil || !character.IsInParty() {
		return nil, fmt.Errorf("%w: character position %d is not occupied", ErrInvalidEquipmentLoadout, input.CharacterSlot+1)
	}
	if input.ExpectedOccupant == "" || character.GetNameAsString() != input.ExpectedOccupant {
		return nil, fmt.Errorf("%w: character position %d changed after selection", ErrInvalidEquipmentLoadout, input.CharacterSlot+1)
	}
	if !character.IsAlive() {
		return nil, fmt.Errorf("%w: %s is not able to ready equipment", ErrInvalidEquipmentLoadout, character.GetNameAsString())
	}

	return character, nil
}

func validateEquipmentSlotChange(change EquipmentSlotChange) error {
	if !isEquipmentSlot(change.Slot) {
		return fmt.Errorf("%w: unsupported equipment slot %d", ErrInvalidEquipmentLoadout, change.Slot)
	}
	if change.Equipment == references.NoEquipment {
		if change.Source != "" {
			return fmt.Errorf("%w: unequipping %s cannot specify an item source", ErrInvalidEquipmentLoadout, change.Slot)
		}

		return nil
	}
	if change.Equipment < references.LeatherHelm || change.Equipment > references.Ankh || change.Equipment.IsAmmo() {
		return fmt.Errorf("%w: %s cannot be equipped", ErrInvalidEquipmentLoadout, change.Equipment)
	}
	if !equipmentCanOccupySlot(change.Equipment, change.Slot) {
		return fmt.Errorf("%w: %s does not belong in the %s slot", ErrInvalidEquipmentLoadout, change.Equipment, change.Slot)
	}
	if change.Source != EquipmentSourceInventory && change.Source != EquipmentSourceManufacture {
		return fmt.Errorf("%w: unsupported source %q", ErrInvalidEquipmentLoadout, change.Source)
	}

	return nil
}

func (g *GameState) validateResultingEquipmentLoadout(
	character *party_state.PlayerCharacter,
	current map[references.EquipmentSlot]references.Equipment,
	requested map[references.EquipmentSlot]references.Equipment,
) error {
	if g.IsInCombat() && (g.CombatScene == nil || !g.CombatScene.Victory) &&
		current[references.SlotArmour] != requested[references.SlotArmour] {
		return fmt.Errorf("%w: body armour cannot be changed during combat before victory", ErrInvalidEquipmentLoadout)
	}

	strengthRequired := 0
	for slot, equipment := range requested {
		if equipment == references.NoEquipment {
			continue
		}
		if equipment < references.LeatherHelm || equipment > references.Ankh || equipment.IsAmmo() ||
			!equipmentCanOccupySlot(equipment, slot) {
			return fmt.Errorf("%w: %s cannot occupy the %s slot", ErrInvalidEquipmentLoadout, equipment, slot)
		}
		strengthRequired += equipment.GetStrengthRequired()
	}
	if strengthRequired > int(character.Strength) {
		return fmt.Errorf("%w: total equipment requires %d strength but %s has %d", ErrInvalidEquipmentLoadout, strengthRequired, character.GetNameAsString(), character.Strength)
	}
	hand0 := requested[references.SlotWeapon]
	hand1 := requested[references.SlotShield]
	if hand1 != references.NoEquipment && hand1.IsTwoHanded() {
		return fmt.Errorf("%w: a two-handed weapon must be stored in hand0", ErrInvalidEquipmentLoadout)
	}
	if hand0 != references.NoEquipment && hand0.IsTwoHanded() && hand1 != references.NoEquipment {
		return fmt.Errorf("%w: a two-handed weapon cannot be readied with hand1 occupied", ErrInvalidEquipmentLoadout)
	}

	return nil
}

func equipmentCanOccupySlot(equipment references.Equipment, slot references.EquipmentSlot) bool {
	equipmentSlot := equipment.GetSlotType()
	if slot != references.SlotWeapon && slot != references.SlotShield {
		return equipmentSlot == slot
	}
	if equipmentSlot != references.SlotWeapon && equipmentSlot != references.SlotShield {
		return false
	}

	return !equipment.IsTwoHanded() || slot == references.SlotWeapon
}

func equipmentLoadout(character *party_state.PlayerCharacter) map[references.EquipmentSlot]references.Equipment {
	loadout := make(map[references.EquipmentSlot]references.Equipment, len(references.EquipmentSlots()))
	for _, slot := range references.EquipmentSlots() {
		loadout[slot] = character.GetEquippedItem(slot)
	}

	return loadout
}

func isEquipmentSlot(slot references.EquipmentSlot) bool {
	return slot >= references.SlotHelm && slot <= references.SlotAmulet
}

func equipmentSlotID(slot references.EquipmentSlot) string {
	switch slot {
	case references.SlotHelm:
		return "helm"
	case references.SlotArmour:
		return "armour"
	case references.SlotWeapon:
		return "weapon"
	case references.SlotShield:
		return "shield"
	case references.SlotRing:
		return "ring"
	case references.SlotAmulet:
		return "amulet"
	case references.SlotNone:
		panic("non-equipment slot has no identifier")
	default:
		panic("unknown equipment slot")
	}
}
