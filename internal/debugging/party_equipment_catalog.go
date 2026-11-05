package debugging

import (
	"fmt"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	// CommandPartyEquipmentLoadoutUpdate atomically changes one or more slots for one or all active characters.
	CommandPartyEquipmentLoadoutUpdate = "party.equipment.loadout.update"
	partyEquipmentCategory             = "Party"
	partyEquipmentAmuletID             = "amulet"
)

func partyEquipmentCommands() []CommandDefinition {
	parameters := make([]ParameterDefinition, 0, len(references.EquipmentSlots())+2)
	parameters = append(parameters, partyEquipmentCharacterParameter())
	for _, slot := range references.EquipmentSlots() {
		parameters = append(parameters, ParameterDefinition{
			ID:           partyEquipmentSlotID(slot),
			Name:         slot.String(),
			Description:  "Leave unset to preserve this slot, unequip it, use carried inventory, or manufacture an item.",
			Kind:         ParameterEnum,
			Required:     false,
			Choices:      partyEquipmentSlotChoices(slot),
			ControlHint:  controlHintSelect,
			Availability: []string{contextGameplay},
		})
	}
	parameters = append(parameters, ParameterDefinition{
		ID:           inventoryExpectedSnapshotID,
		Name:         "Expected Equipment State",
		Description:  "Complete carried-inventory and roster-loadout fingerprint reviewed before execution.",
		Kind:         ParameterString,
		Required:     true,
		ControlHint:  activeCombatTextControlHint,
		Availability: []string{contextGameplay},
	})

	return []CommandDefinition{{
		ID:             CommandPartyEquipmentLoadoutUpdate,
		Category:       partyEquipmentCategory,
		Name:           "Equipment Loadout",
		Description:    "Atomically change one or more equipment slots for one active party member using carried or explicitly manufactured items.",
		Aliases:        []string{"ready equipment", "party loadout"},
		SearchTerms:    []string{"weapon", "armour", "armor", "shield", "helm", "ring", partyEquipmentAmuletID},
		Parameters:     parameters,
		ExecutionModes: []ExecutionMode{ExecutionAlteration},
		Destructive:    true,
	}}
}

func partyEquipmentCharacterParameter() ParameterDefinition {
	return ParameterDefinition{
		ID:              "character",
		Name:            "Character",
		Description:     "Select a current active-party member.",
		Kind:            ParameterCharacter,
		Required:        true,
		OptionsProvider: OptionProviderActivePartyCharacters,
		ControlHint:     controlHintSelect,
		Availability:    []string{contextGameplay},
	}
}

func partyEquipmentSlotChoices(slot references.EquipmentSlot) []Choice {
	choices := []Choice{choice("unequip", "Unequip")}
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		if equipment.IsAmmo() || equipment.GetSlotType() != slot {
			continue
		}
		id := strconv.Itoa(equipment.ID())
		choices = append(choices,
			choice("inventory:"+id, fmt.Sprintf("Equip %s from inventory", equipment)),
			choice("manufacture:"+id, fmt.Sprintf("Manufacture and equip %s", equipment)),
		)
	}

	return choices
}

func partyEquipmentSlotID(slot references.EquipmentSlot) string {
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
		return partyEquipmentAmuletID
	case references.SlotNone:
		panic("non-equipment slot has no catalog identifier")
	default:
		panic("unknown equipment slot")
	}
}
