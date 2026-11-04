package main

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestPartyEquipmentLoadoutUpdateAppliesReviewedMixedSourceChanges(t *testing.T) {
	state := newApprovedToolTestState()
	initializeEmptyInventoryLockerRoster(state)
	require.NoError(t, state.SetAvatarName("Avatar"))
	avatar := state.PartyState.GetCharacter(0)
	prepareEquipmentTestCharacter(avatar)
	avatar.Strength = 30
	avatar.SetEquippedItem(references.SlotWeapon, references.LongSword)
	avatar.SetEquippedItem(references.SlotShield, references.SmallShield)
	state.PartyState.Inventory.Equipment.Set(references.MagicAxe, 2)
	status := newInventoryLockerResult(state)

	result := executePartyEquipmentLoadoutUpdate(debugging.Request{
		SessionID: "test", ID: "loadout", CommandID: debugging.CommandPartyEquipmentLoadoutUpdate,
		Arguments: map[string]json.RawMessage{
			"character":                       mustDebugJSON(activeCharacterToken(0, avatar.GetNameAsString())),
			"weapon":                          json.RawMessage(`"inventory:38"`),
			"shield":                          json.RawMessage(`"unequip"`),
			"armour":                          json.RawMessage(`"manufacture:14"`),
			inventoryExpectedSnapshotArgument: mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, references.MagicAxe, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotShield))
	assert.Equal(t, references.PlateMail, avatar.GetEquippedItem(references.SlotArmour))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.MagicAxe))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.LongSword))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.SmallShield))
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Equipment.Get(references.PlateMail))
	assert.Contains(t, stateChangePaths(result.Changes), "party.roster.0.equipment.weapon")
	assert.Contains(t, stateChangePaths(result.Changes), "inventory.equipment.38.quantity")
}

func TestPartyEquipmentLoadoutUpdateRejectsStaleReviewWithoutMutation(t *testing.T) {
	state := newApprovedToolTestState()
	initializeEmptyInventoryLockerRoster(state)
	require.NoError(t, state.SetAvatarName("Avatar"))
	avatar := state.PartyState.GetCharacter(0)
	prepareEquipmentTestCharacter(avatar)
	avatar.Strength = 30
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 1)

	result := executePartyEquipmentLoadoutUpdate(debugging.Request{
		SessionID: "test", ID: "stale", CommandID: debugging.CommandPartyEquipmentLoadoutUpdate,
		Arguments: map[string]json.RawMessage{
			"character":                       mustDebugJSON(activeCharacterToken(0, avatar.GetNameAsString())),
			"weapon":                          json.RawMessage(`"inventory:30"`),
			inventoryExpectedSnapshotArgument: json.RawMessage(`"stale"`),
		},
	}, state)

	require.False(t, result.Success)
	assert.Equal(t, "stale_equipment_state", result.ErrorCode)
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.LongSword))
}

func TestPartyEquipmentLoadoutUpdateRejectsManufacturedAmmo(t *testing.T) {
	state := newApprovedToolTestState()
	initializeEmptyInventoryLockerRoster(state)
	require.NoError(t, state.SetAvatarName("Avatar"))
	avatar := state.PartyState.GetCharacter(0)
	prepareEquipmentTestCharacter(avatar)
	avatar.Strength = 30
	status := newInventoryLockerResult(state)

	result := executePartyEquipmentLoadoutUpdate(debugging.Request{
		SessionID: "test", ID: "ammo", CommandID: debugging.CommandPartyEquipmentLoadoutUpdate,
		Arguments: map[string]json.RawMessage{
			"character":                       mustDebugJSON(activeCharacterToken(0, avatar.GetNameAsString())),
			"weapon":                          json.RawMessage(`"manufacture:27"`),
			inventoryExpectedSnapshotArgument: mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)

	require.False(t, result.Success)
	assert.Equal(t, "invalid_loadout", result.ErrorCode)
	assert.Equal(t, references.NoEquipment, avatar.GetEquippedItem(references.SlotWeapon))
}

func TestPartyEquipmentLoadoutUpdateAppliesAllActiveMembersWithExactChanges(t *testing.T) {
	state := newApprovedToolTestState()
	initializeEmptyInventoryLockerRoster(state)
	require.NoError(t, state.SetAvatarName("Avatar"))
	avatar := state.PartyState.GetCharacter(0)
	shamino := state.PartyState.GetCharacter(1)
	copy(shamino.Name[:], "Shamino")
	for slot := 2; slot < party_state.MaxActiveParty; slot++ {
		state.PartyState.GetCharacter(slot).PartyMap = party_state.PartyMapUnrecruited
	}
	for _, character := range []*party_state.PlayerCharacter{avatar, shamino} {
		prepareEquipmentTestCharacter(character)
		character.Strength = 30
	}
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 2)
	status := newInventoryLockerResult(state)

	result := executePartyEquipmentLoadoutUpdate(debugging.Request{
		SessionID: "test", ID: "all-loadout", CommandID: debugging.CommandPartyEquipmentLoadoutUpdate,
		Arguments: map[string]json.RawMessage{
			"character": json.RawMessage(`"all"`), "weapon": json.RawMessage(`"inventory:16"`),
			inventoryExpectedSnapshotArgument: mustDebugJSON(status.SnapshotFingerprint),
		},
	}, state)

	require.True(t, result.Success, result.ErrorMessage)
	assert.Equal(t, references.Dagger, avatar.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.Dagger, shamino.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Equipment.Get(references.Dagger))
	assert.Contains(t, result.Summary, "2 active party members")
	assert.ElementsMatch(t, []string{
		"party.roster.0.equipment.weapon", "party.roster.1.equipment.weapon", "inventory.equipment.16.quantity",
	}, stateChangePaths(result.Changes))
	var data partyEquipmentLoadoutResult
	require.NoError(t, json.Unmarshal(result.Data, &data))
	assert.Equal(t, 2, data.TargetCount)
	assert.Equal(t, []string{"Avatar", shamino.GetNameAsString()}, data.CharacterNames)
}

func TestPartyEquipmentLoadoutUpdateRejectsDialogAndWorkflowContexts(t *testing.T) {
	t.Run("dialog", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		scene.dialogStack.PushModalDialog(&blockingDebugWorkflowDialog{})

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandPartyEquipmentLoadoutUpdate})

		assert.Equal(t, "dialog_active", result.ErrorCode)
	})

	t.Run("active workflow", func(t *testing.T) {
		scene := newCommandWorkflowTestScene(t)
		queued := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandTestTelescope})
		require.True(t, queued.Success, queued.ErrorMessage)

		result := scene.ExecuteDebugCommand(debugging.Request{CommandID: debugging.CommandPartyEquipmentLoadoutUpdate})

		assert.Equal(t, "gameplay_workflow_active", result.ErrorCode)
	})
}

func prepareEquipmentTestCharacter(character *party_state.PlayerCharacter) {
	character.PartyMap = party_state.PartyMapActive
	character.SetStatus(party_state.Good)
	character.SetMaxHp(100)
	character.SetHp(100)
}
