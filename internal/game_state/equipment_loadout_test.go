package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func newEquipmentLoadoutTestState(t *testing.T) *GameState {
	t.Helper()
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	character := state.PartyState.GetCharacter(0)
	character.SetStatus(party_state.Good)
	character.SetHp(max(character.GetCurrentHp(), 1))
	character.Strength = 30
	for _, slot := range references.EquipmentSlots() {
		character.SetEquippedItem(slot, references.NoEquipment)
	}

	return state
}

func TestAlterEquipmentLoadoutAppliesMixedMultiSlotSourcesAtomically(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	avatar.SetEquippedItem(references.SlotWeapon, references.LongSword)
	avatar.SetEquippedItem(references.SlotShield, references.SmallShield)
	state.PartyState.Inventory.Equipment.Set(references.MagicAxe, 2)

	result, err := state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{
			{Slot: references.SlotArmour, Equipment: references.PlateMail, Source: EquipmentSourceManufacture},
			{Slot: references.SlotWeapon, Equipment: references.MagicAxe, Source: EquipmentSourceInventory},
			{Slot: references.SlotShield, Equipment: references.NoEquipment},
		},
	})
	require.NoError(t, err)
	assert.Equal(t, references.PlateMail, avatar.GetEquippedItem(references.SlotArmour))
	assert.Equal(t, references.MagicAxe, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotShield))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.MagicAxe))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.LongSword))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.SmallShield))
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Equipment.Get(references.PlateMail))
	assert.Len(t, result.Changes, 6)
}

func TestAlterEquipmentLoadoutRejectsInvalidFinalStateWithoutMutation(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	state.PartyState.Inventory.Equipment.Set(references.Halberd, 1)
	state.PartyState.Inventory.Equipment.Set(references.SmallShield, 1)

	_, err := state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{
			{Slot: references.SlotWeapon, Equipment: references.Halberd, Source: EquipmentSourceInventory},
			{Slot: references.SlotShield, Equipment: references.SmallShield, Source: EquipmentSourceInventory},
		},
	})
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotShield))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.Halberd))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.SmallShield))
}

func TestAlterEquipmentLoadoutAcceptsReadyGenericHandStates(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	state.PartyState.Inventory.Equipment.Set(references.SmallShield, 1)
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)

	_, err := state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{
			{Slot: references.SlotWeapon, Equipment: references.SmallShield, Source: EquipmentSourceInventory},
			{Slot: references.SlotShield, Equipment: references.Dagger, Source: EquipmentSourceInventory},
		},
	})

	require.NoError(t, err)
	assert.Equal(t, references.SmallShield, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.Dagger, avatar.GetEquippedItem(references.SlotShield))
}

func TestAlterEquipmentLoadoutRejectsUnavailableOrOverflowingInventoryAtomically(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	avatar.SetEquippedItem(references.SlotWeapon, references.LongSword)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, party_state.ItemQuantitySmallMax)

	_, err := state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{{
			Slot: references.SlotWeapon, Equipment: references.MagicAxe, Source: EquipmentSourceManufacture,
		}},
	})
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)
	assert.Equal(t, references.LongSword, avatar.GetEquippedItem(references.SlotWeapon))

	state.PartyState.Inventory.Equipment.Set(references.LongSword, 0)
	_, err = state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{{
			Slot: references.SlotWeapon, Equipment: references.MagicAxe, Source: EquipmentSourceInventory,
		}},
	})
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)
	assert.Equal(t, references.LongSword, avatar.GetEquippedItem(references.SlotWeapon))
}

func TestAlterEquipmentLoadoutAggregatesReturnAndConsumptionBeforeCapacityValidation(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	avatar.SetEquippedItem(references.SlotWeapon, references.LongSword)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, party_state.ItemQuantitySmallMax)

	result, err := state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{{
			Slot: references.SlotWeapon, Equipment: references.LongSword, Source: EquipmentSourceInventory,
		}},
	})
	require.NoError(t, err)
	assert.Empty(t, result.Changes)
	assert.Equal(t, uint16(party_state.ItemQuantitySmallMax), state.PartyState.Inventory.Equipment.Get(references.LongSword))
}

func TestAlterEquipmentLoadoutRejectsChangedDeadOrCombatRestrictedTargetWithoutMutation(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	state.PartyState.Inventory.Equipment.Set(references.ChainMail, 1)
	request := EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{{
			Slot: references.SlotArmour, Equipment: references.ChainMail, Source: EquipmentSourceInventory,
		}},
	}

	request.ExpectedOccupant = "Changed"
	_, err := state.AlterEquipmentLoadout(request)
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)
	request.ExpectedOccupant = avatar.GetNameAsString()

	avatar.SetStatus(party_state.Dead)
	_, err = state.AlterEquipmentLoadout(request)
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)
	avatar.SetStatus(party_state.Good)

	state.MapState.PlayerLocation.Location = references.CombatLocation
	_, err = state.AlterEquipmentLoadout(request)
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)

	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotArmour))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.ChainMail))
}

func TestAlterEquipmentLoadoutRejectsInsufficientCarriedInventoryBeforeMutation(t *testing.T) {
	state := newEquipmentLoadoutTestState(t)
	avatar := state.PartyState.GetCharacter(0)
	for _, slot := range references.EquipmentSlots() {
		avatar.SetEquippedItem(slot, references.NoEquipment)
	}
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 0)

	_, err := state.AlterEquipmentLoadout(EquipmentLoadoutAlteration{
		CharacterSlot: 0, ExpectedOccupant: avatar.GetNameAsString(),
		Changes: []EquipmentSlotChange{{Slot: references.SlotWeapon, Equipment: references.LongSword, Source: EquipmentSourceInventory}},
	})
	require.ErrorIs(t, err, ErrInvalidEquipmentLoadout)
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Equipment.Get(references.LongSword))
}
