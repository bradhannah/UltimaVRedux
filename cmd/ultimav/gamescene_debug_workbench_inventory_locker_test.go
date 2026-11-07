package main

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestInventoryLockerStatusReturnsCompleteDetachedCamelCaseSnapshot(t *testing.T) {
	state := newApprovedToolTestState()
	initializeEmptyInventoryLockerRoster(state)
	state.PartyState.Inventory.Potions.Set(0, 3)
	before := state.GetDebugInventorySnapshot()

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "status", CommandID: debugging.CommandInventoryLockerStatus,
	}, state)

	require.True(t, handled)
	require.True(t, result.Success)
	assert.Empty(t, result.Changes)
	assert.Contains(t, string(result.Data), `"snapshotFingerprint"`)
	assert.NotContains(t, string(result.Data), `"SnapshotFingerprint"`)
	var status inventoryLockerResult
	require.NoError(t, json.Unmarshal(result.Data, &status))
	assert.Len(t, status.Provisions, 6)
	assert.Len(t, status.Reagents, 8)
	assert.Len(t, status.Potions, 8)
	assert.Len(t, status.Scrolls, 8)
	assert.NotEmpty(t, status.Equipment)
	assert.Len(t, status.Equipped, 16)
	assert.Len(t, status.Equipped[0].Slots, 6)
	assert.Equal(t, uint16(3), status.Potions[0].Quantity)
	assert.Equal(t, inventoryLockerFingerprint(status), status.SnapshotFingerprint)
	assert.Equal(t, before, state.GetDebugInventorySnapshot())
}

func TestInventoryProvisionSetEnforcesReviewedQuantityAndReportsSemanticChange(t *testing.T) {
	state := newApprovedToolTestState()
	state.PartyState.Inventory.Provisions.Food.Set(4)

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "set", CommandID: debugging.CommandInventoryProvisionSet,
		Arguments: map[string]json.RawMessage{
			"provision": json.RawMessage(`"food"`), "targetQuantity": json.RawMessage(`9`), "expectedQuantity": json.RawMessage(`4`),
		},
	}, state)

	require.True(t, handled)
	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionSynchronized, result.EffectiveExecutionMode)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "inventory.provisions.food.quantity", result.Changes[0].Path)
	assert.Equal(t, uint16(9), state.PartyState.Inventory.Provisions.Food.Get())
}

func TestInventoryReagentSetEnforcesReviewedQuantityAndReturnsRefreshedLocker(t *testing.T) {
	state := newApprovedToolTestState()
	state.PartyState.Inventory.Reagent.Set(references.Garlic, 4)

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "set", CommandID: debugging.CommandInventoryReagentSet,
		Arguments: map[string]json.RawMessage{
			"reagent": json.RawMessage(`"2"`), "targetQuantity": json.RawMessage(`9`), "expectedQuantity": json.RawMessage(`4`),
		},
	}, state)

	require.True(t, handled)
	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionSynchronized, result.EffectiveExecutionMode)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "inventory.reagents.2.quantity", result.Changes[0].Path)
	assert.JSONEq(t, `4`, string(result.Changes[0].Before))
	assert.JSONEq(t, `9`, string(result.Changes[0].After))
	var after inventoryLockerResult
	require.NoError(t, json.Unmarshal(result.Data, &after))
	assert.Equal(t, uint16(9), after.Reagents[references.Garlic].Quantity)
}

func TestInventoryEquipmentSetRejectsStaleQuantityWithoutMutation(t *testing.T) {
	state := newApprovedToolTestState()
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 4)

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "set", CommandID: debugging.CommandInventoryEquipmentSet,
		Arguments: map[string]json.RawMessage{
			"equipment": json.RawMessage(`"30"`), "targetQuantity": json.RawMessage(`9`), "expectedQuantity": json.RawMessage(`3`),
		},
	}, state)

	require.True(t, handled)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_inventory", result.ErrorCode)
	assert.Equal(t, uint16(4), state.PartyState.Inventory.Equipment.Get(references.LongSword))
}

func TestInventoryEquipmentSetReportsExactSemanticChange(t *testing.T) {
	state := newApprovedToolTestState()
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 4)

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "set", CommandID: debugging.CommandInventoryEquipmentSet,
		Arguments: map[string]json.RawMessage{
			"equipment": json.RawMessage(`"30"`), "targetQuantity": json.RawMessage(`9`), "expectedQuantity": json.RawMessage(`4`),
		},
	}, state)

	require.True(t, handled)
	require.True(t, result.Success)
	require.Len(t, result.Changes, 1)
	assert.Equal(t, "inventory.equipment.30.quantity", result.Changes[0].Path)
	assert.JSONEq(t, `4`, string(result.Changes[0].Before))
	assert.JSONEq(t, `9`, string(result.Changes[0].After))
}

func TestInventoryLockerRejectsStaleReviewedStateWithoutMutation(t *testing.T) {
	state := newApprovedToolTestState()
	status := newInventoryLockerResult(state)
	state.PartyState.Inventory.Potions.Set(0, 2)

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "preset", CommandID: debugging.CommandInventoryPotionPreset,
		Arguments: map[string]json.RawMessage{
			"preset": json.RawMessage(`"potions_all99"`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)

	require.True(t, handled)
	assert.False(t, result.Success)
	assert.Equal(t, "stale_inventory", result.ErrorCode)
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Potions.Get(0))
}

func TestInventoryPotionAndPresetMutationsReturnFullLockerStatus(t *testing.T) {
	state := newApprovedToolTestState()
	add, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "add", CommandID: debugging.CommandInventoryPotionAdd,
		Arguments: map[string]json.RawMessage{"potion": json.RawMessage(`"0"`), "amount": json.RawMessage(`5`)},
	}, state)

	require.True(t, handled)
	require.True(t, add.Success)
	assert.Equal(t, debugging.ExecutionLiveSafe, add.EffectiveExecutionMode)
	require.Len(t, add.Changes, 1)
	assert.Equal(t, "inventory.potions.0.quantity", add.Changes[0].Path)
	var afterAdd inventoryLockerResult
	require.NoError(t, json.Unmarshal(add.Data, &afterAdd))
	assert.Equal(t, uint16(5), afterAdd.Potions[0].Quantity)

	preset, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "preset", CommandID: debugging.CommandInventoryPotionPreset,
		Arguments: map[string]json.RawMessage{
			"preset": json.RawMessage(`"potions_all1"`), "expectedSnapshot": mustDebugJSON(afterAdd.SnapshotFingerprint),
		},
	}, state)

	require.True(t, handled)
	require.True(t, preset.Success)
	assert.Equal(t, debugging.ExecutionAlteration, preset.EffectiveExecutionMode)
	require.NotEmpty(t, preset.Changes)
	var afterPreset inventoryLockerResult
	require.NoError(t, json.Unmarshal(preset.Data, &afterPreset))
	assert.Equal(t, uint16(1), afterPreset.Potions[0].Quantity)
}

func TestInventoryLockerMapsCoreOperationErrors(t *testing.T) {
	state := newApprovedToolTestState()

	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "invalid", CommandID: debugging.CommandInventoryPotionAdd,
		Arguments: map[string]json.RawMessage{"potion": json.RawMessage(`"0"`), "amount": json.RawMessage(`0`)},
	}, state)

	require.True(t, handled)
	assert.False(t, result.Success)
	assert.Equal(t, "invalid_inventory_operation", result.ErrorCode)
}

func TestInventoryEquipmentGrantPresetRequiresReviewedSnapshotAndReportsChanges(t *testing.T) {
	state := newApprovedToolTestState()
	status := newInventoryLockerResult(state)
	state.PartyState.Inventory.Equipment.Set(0, 1)

	stale, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "stale", CommandID: debugging.CommandInventoryEquipmentGrantPreset,
		Arguments: map[string]json.RawMessage{
			"preset": json.RawMessage(`"weapons5"`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)
	require.True(t, handled)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_inventory", stale.ErrorCode)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(0))

	status = newInventoryLockerResult(state)
	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "grant", CommandID: debugging.CommandInventoryEquipmentGrantPreset,
		Arguments: map[string]json.RawMessage{
			"preset": json.RawMessage(`"bows_no_ammo5"`), "expectedSnapshot": mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)
	require.True(t, handled)
	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionAlteration, result.EffectiveExecutionMode)
	require.Len(t, result.Changes, 3)
	assert.Equal(t, fmt.Sprintf("inventory.equipment.%d.quantity", references.Bow.ID()), result.Changes[0].Path)
	var after inventoryLockerResult
	require.NoError(t, json.Unmarshal(result.Data, &after))
	assert.Equal(t, uint16(5), after.Equipment[references.Bow-references.LeatherHelm].Quantity)
}

func TestInventoryEquipmentClearRequiresCompleteReviewedStateAndReportsExactChanges(t *testing.T) {
	state := newApprovedToolTestState()
	initializeEmptyInventoryLockerRoster(state)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 3)
	state.PartyState.GetAvatar().SetEquippedItem(references.SlotWeapon, references.LongSword)
	status := newInventoryLockerResult(state)

	state.PartyState.GetAvatar().SetEquippedItem(references.SlotRing, references.RingProtection)
	stale, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "stale", CommandID: debugging.CommandInventoryEquipmentClear,
		Arguments: map[string]json.RawMessage{
			inventoryExpectedSnapshotArgument: mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)
	require.True(t, handled)
	assert.False(t, stale.Success)
	assert.Equal(t, "stale_equipment_state", stale.ErrorCode)
	assert.Equal(t, uint16(3), state.PartyState.Inventory.Equipment.Get(references.LongSword))

	status = newInventoryLockerResult(state)
	result, handled := executeInventoryLockerDebugCommand(debugging.Request{
		SessionID: "test", ID: "clear", CommandID: debugging.CommandInventoryEquipmentClear,
		Arguments: map[string]json.RawMessage{
			inventoryExpectedSnapshotArgument: mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)
	require.True(t, handled)
	require.True(t, result.Success)
	assert.Equal(t, debugging.ExecutionAlteration, result.EffectiveExecutionMode)
	require.Len(t, result.Changes, 3)
	assert.Equal(t, "inventory.equipment.30.quantity", result.Changes[0].Path)
	assert.Equal(t, "party.roster.0.equipment.weapon", result.Changes[1].Path)
	assert.Equal(t, "party.roster.0.equipment.ring", result.Changes[2].Path)
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Equipment.Get(references.LongSword))
	assert.Equal(t, references.NoEquipment, state.PartyState.GetAvatar().GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.NoEquipment, state.PartyState.GetAvatar().GetEquippedItem(references.SlotRing))
}

func initializeEmptyInventoryLockerRoster(state *game_state.GameState) {
	state.PartyState = *party_state.NewBlankPartyState()
	state.PartyState.ForEachCharacter(func(_ int, character *party_state.PlayerCharacter) {
		for _, slot := range references.EquipmentSlots() {
			character.SetEquippedItem(slot, references.NoEquipment)
		}
	})
}
