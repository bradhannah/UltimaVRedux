package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestReadyExplorationRoutesRemainCharacterNavigable(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	clearReadyTestEquipment(state)
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)
	callbacks.Reset()

	assert.True(t, state.ActionReadySmallMap())
	require.True(t, callbacks.ReadyDialogShown)
	assert.False(t, callbacks.ReadyDialogRequest.CombatMode)
	assert.False(t, callbacks.ReadyDialogRequest.LockCharacter)
	assert.Equal(t, []string{"Ready..."}, callbacks.CommandPrompts)
}

func TestReadyCombatBindsNonAvatarAndExactCloneIdentity(t *testing.T) {
	state, callbacks := newReadyCombatTestState(t)
	clearReadyTestEquipment(state)
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)

	nonAvatar := firstReadyActivePC(state, 1)
	require.NotZero(t, nonAvatar.EntityIndex)
	state.CombatScene.CurrentTurnIdentity = nonAvatar
	state.CombatScene.CurrentTurnIndex = nonAvatar.EntityIndex
	callbacks.Reset()
	request, outcome := state.BeginReadyEquipment(true)
	require.Equal(t, ReadyBeginDialog, outcome)
	assert.Equal(t, nonAvatar, request.CombatIdentity)
	assert.Equal(t, nonAvatar.EntityIndex, request.CharacterIndex)
	assert.True(t, request.LockCharacter)

	clone := state.CombatScene.addPCCloneFromSource(
		nonAvatar,
		nonAvatar.EntityIndex,
		references.Position{X: 1, Y: 1},
		NewCombatPCProvider(state.PartyState.GetCharacter(nonAvatar.EntityIndex)),
	)
	state.CombatScene.CurrentTurnIdentity = clone
	state.CombatScene.CurrentTurnIndex = clone.EntityIndex
	request, outcome = state.BeginReadyEquipment(true)
	require.Equal(t, ReadyBeginDialog, outcome)
	assert.Equal(t, clone, request.CombatIdentity)
	assert.Equal(t, clone.EntityIndex, request.CharacterIndex,
		"a clone readies the underlying persistent PC")
	state.CombatScene.presentTurns = true
	token := state.FinishCombatActionDeferredFor(clone)
	require.NotNil(t, state.CombatScene.pendingPresentationStep)
	assert.Equal(t, token, state.CombatScene.pendingPresentationStep.step.Token)
	assert.Equal(t, clone, state.CombatScene.pendingPresentationStep.step.Actor,
		"deferred completion retains the exact clone action identity")
}

func TestReadyCombatRejectsNonPCWithoutActionOrRNG(t *testing.T) {
	state, callbacks := newReadyCombatTestState(t)
	clearReadyTestEquipment(state)
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 1)
	state.CombatScene.CurrentTurnIdentity = CombatTurnIdentity{IsPartyMember: false, EntityIndex: 0}
	state.CombatScene.CurrentTurnIndex = 0
	beforeCalls := state.GetRNG().Calls()
	beforeTime := state.DateTime
	callbacks.Reset()

	_, outcome := state.BeginReadyEquipment(true)

	assert.Equal(t, ReadyBeginRejected, outcome)
	assert.False(t, callbacks.ReadyDialogShown)
	assert.Contains(t, callbacks.Messages, "Can't!")
	assert.Equal(t, beforeCalls, state.GetRNG().Calls())
	assert.Equal(t, beforeTime, state.DateTime)
}

func TestApplyReadyEquipmentCombatSlotAndVictoryPolicy(t *testing.T) {
	state, _ := newReadyCombatTestState(t)
	clearReadyTestEquipment(state)
	request := readyCombatRequest(t, state)
	character := state.PartyState.GetCharacter(request.CharacterIndex)
	character.Strength = 30

	legal := []references.Equipment{
		references.LeatherHelm,
		references.Dagger,
		references.SmallShield,
		references.RingProtection,
		references.AmuletOfTurning,
	}
	for _, equipment := range legal {
		state.PartyState.Inventory.Equipment.Set(equipment, 1)
		result, err := state.ApplyReadyEquipment(readyTransaction(request, character, equipment))
		require.NoError(t, err, equipment.String())
		assert.True(t, result.Changed)
		assert.Equal(t, equipment, character.GetEquippedItem(equipment.GetSlotType()))
	}

	state.PartyState.Inventory.Equipment.Set(references.ClothArmour, 1)
	_, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.ClothArmour))
	require.ErrorIs(t, err, ErrReadyEquipment)
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotArmour))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.ClothArmour))

	state.CombatScene.Victory = true
	_, err = state.ApplyReadyEquipment(readyTransaction(request, character, references.ClothArmour))
	require.NoError(t, err)
	assert.Equal(t, references.ClothArmour, character.GetEquippedItem(references.SlotArmour))

	state.CombatScene.Victory = false
	_, err = state.ApplyReadyEquipment(readyTransaction(request, character, references.ClothArmour))
	require.ErrorIs(t, err, ErrReadyEquipment, "body armour cannot be removed before victory")
	state.CombatScene.Victory = true
	_, err = state.ApplyReadyEquipment(readyTransaction(request, character, references.ClothArmour))
	require.NoError(t, err)
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotArmour))
}

func TestApplyReadyEquipmentAmmoPreflightDoesNotMutateOrConsume(t *testing.T) {
	state, _ := newReadyCombatTestState(t)
	clearReadyTestEquipment(state)
	request := readyCombatRequest(t, state)
	character := state.PartyState.GetCharacter(request.CharacterIndex)
	character.Strength = 30

	tests := []struct {
		weapon references.Equipment
		ammo   references.Equipment
	}{
		{weapon: references.Bow, ammo: references.Arrows},
		{weapon: references.MagicBow, ammo: references.Arrows},
		{weapon: references.Crossbow, ammo: references.Quarrels},
	}
	for _, test := range tests {
		t.Run(test.weapon.String(), func(t *testing.T) {
			character.SetEquippedItem(references.SlotWeapon, references.NoEquipment)
			state.PartyState.Inventory.Equipment.Set(test.weapon, 1)
			state.PartyState.Inventory.Equipment.Set(test.ammo, 0)
			beforeCalls := state.GetRNG().Calls()

			_, err := state.ApplyReadyEquipment(readyTransaction(request, character, test.weapon))

			require.ErrorIs(t, err, ErrReadyEquipment)
			assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(test.weapon))
			assert.Equal(t, beforeCalls, state.GetRNG().Calls())

			state.PartyState.Inventory.Equipment.Set(test.ammo, 2)
			_, err = state.ApplyReadyEquipment(readyTransaction(request, character, test.weapon))
			require.NoError(t, err)
			assert.Equal(t, uint16(2), state.PartyState.Inventory.Equipment.Get(test.ammo),
				"Ready validates but never consumes ammunition")
		})
	}
}

func TestApplyReadyEquipmentUsesFirstAvailableGenericHand(t *testing.T) {
	tests := []struct {
		name      string
		hand0     references.Equipment
		hand1     references.Equipment
		candidate references.Equipment
		wantSlot  references.EquipmentSlot
		wantError string
	}{
		{name: "both empty weapon uses hand0", hand0: references.NoEquipment, hand1: references.NoEquipment, candidate: references.Dagger, wantSlot: references.SlotWeapon},
		{name: "both empty shield uses hand0", hand0: references.NoEquipment, hand1: references.NoEquipment, candidate: references.SmallShield, wantSlot: references.SlotWeapon},
		{name: "empty hand0 wins", hand0: references.NoEquipment, hand1: references.Dagger, candidate: references.SmallShield, wantSlot: references.SlotWeapon},
		{name: "occupied hand0 uses hand1", hand0: references.SmallShield, hand1: references.NoEquipment, candidate: references.Dagger, wantSlot: references.SlotShield},
		{name: "two handed hand0 needs confirmation", hand0: references.Halberd, hand1: references.NoEquipment, candidate: references.SmallShield, wantError: "Confirm the swap first!"},
		{name: "both occupied need confirmation", hand0: references.Dagger, hand1: references.SmallShield, candidate: references.LongSword, wantError: "Confirm the swap first!"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, character, request := newReadyExplorationTestState(t)
			character.SetEquippedItem(references.SlotWeapon, test.hand0)
			character.SetEquippedItem(references.SlotShield, test.hand1)
			state.PartyState.Inventory.Equipment.Set(test.candidate, 1)

			result, err := state.ApplyReadyEquipment(readyTransaction(request, character, test.candidate))
			if test.wantError != "" {
				require.EqualError(t, err, test.wantError)
				assert.False(t, result.Changed)
				assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(test.candidate))
				assert.Equal(t, test.hand0, character.GetEquippedItem(references.SlotWeapon))
				assert.Equal(t, test.hand1, character.GetEquippedItem(references.SlotShield))

				return
			}
			require.NoError(t, err)
			assert.Equal(t, ReadyEquipmentEquip, result.Selection)
			assert.Equal(t, test.candidate, character.GetEquippedItem(test.wantSlot))
			assert.Zero(t, state.PartyState.Inventory.Equipment.Get(test.candidate))
		})
	}
}

func TestApplyReadyEquipmentAllowsTwoWeaponsTwoShieldsAndEitherOrder(t *testing.T) {
	tests := []struct {
		name  string
		items [2]references.Equipment
		hands [2]references.Equipment
	}{
		{name: "two weapons", items: [2]references.Equipment{references.Dagger, references.LongSword}, hands: [2]references.Equipment{references.Dagger, references.LongSword}},
		{name: "two shields", items: [2]references.Equipment{references.SmallShield, references.LargeShield}, hands: [2]references.Equipment{references.SmallShield, references.LargeShield}},
		{name: "weapon then shield", items: [2]references.Equipment{references.Dagger, references.SmallShield}, hands: [2]references.Equipment{references.Dagger, references.SmallShield}},
		{name: "shield then weapon", items: [2]references.Equipment{references.SmallShield, references.Dagger}, hands: [2]references.Equipment{references.SmallShield, references.Dagger}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, character, request := newReadyExplorationTestState(t)
			for _, equipment := range test.items {
				state.PartyState.Inventory.Equipment.Set(equipment, 1)
				_, err := state.ApplyReadyEquipment(readyTransaction(request, character, equipment))
				require.NoError(t, err)
			}
			assert.Equal(t, test.hands[0], character.GetEquippedItem(references.SlotWeapon))
			assert.Equal(t, test.hands[1], character.GetEquippedItem(references.SlotShield))
		})
	}
}

func TestApplyReadyEquipmentTwoHandedRequiresBothHandsAndUsesOnlyHand0(t *testing.T) {
	state, character, request := newReadyExplorationTestState(t)
	state.PartyState.Inventory.Equipment.Set(references.Halberd, 1)

	result, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.Halberd))
	require.NoError(t, err)
	assert.Equal(t, ReadyEquipmentEquip, result.Selection)
	assert.Equal(t, references.Halberd, character.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotShield))

	for _, occupiedSlot := range []references.EquipmentSlot{references.SlotWeapon, references.SlotShield} {
		clearReadyTestEquipment(state)
		character.SetEquippedItem(occupiedSlot, references.Dagger)
		state.PartyState.Inventory.Equipment.Set(references.Halberd, 1)
		_, err = state.ApplyReadyEquipment(readyTransaction(request, character, references.Halberd))
		require.EqualError(t, err, "Confirm the swap first!")
		assert.Equal(t, references.Dagger, character.GetEquippedItem(occupiedSlot))
		assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.Halberd))
	}
}

func TestApplyReadyEquipmentRejectsOccupiedNonHandSlotsWithoutMutation(t *testing.T) {
	tests := []struct {
		slot      references.EquipmentSlot
		readied   references.Equipment
		candidate references.Equipment
		message   string
	}{
		{slot: references.SlotHelm, readied: references.LeatherHelm, candidate: references.ChainCoif, message: "Confirm the swap first!"},
		{slot: references.SlotArmour, readied: references.ClothArmour, candidate: references.LeatherArmour, message: "Confirm the swap first!"},
		{slot: references.SlotRing, readied: references.RingProtection, candidate: references.RingRegeneration, message: "Confirm the swap first!"},
		{slot: references.SlotAmulet, readied: references.Ankh, candidate: references.SpikedCollar, message: "Confirm the swap first!"},
	}
	for _, test := range tests {
		t.Run(test.slot.String(), func(t *testing.T) {
			state, character, request := newReadyExplorationTestState(t)
			character.SetEquippedItem(test.slot, test.readied)
			state.PartyState.Inventory.Equipment.Set(test.candidate, 1)

			_, err := state.ApplyReadyEquipment(readyTransaction(request, character, test.candidate))

			require.EqualError(t, err, test.message)
			assert.Equal(t, test.readied, character.GetEquippedItem(test.slot))
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(test.candidate))
		})
	}
}

func TestApplyReadyEquipmentDuplicateUnreadyUsesSlotOrderAndDiscardsAt99(t *testing.T) {
	state, character, request := newReadyExplorationTestState(t)
	character.SetEquippedItem(references.SlotWeapon, references.Dagger)
	character.SetEquippedItem(references.SlotShield, references.Dagger)
	state.PartyState.Inventory.Equipment.Set(references.Dagger, 98)

	result, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.Dagger))
	require.NoError(t, err)
	assert.Equal(t, ReadyEquipmentUnready, result.Selection)
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.Dagger, character.GetEquippedItem(references.SlotShield))
	assert.Equal(t, uint16(99), state.PartyState.Inventory.Equipment.Get(references.Dagger))

	result, err = state.ApplyReadyEquipment(readyTransaction(request, character, references.Dagger))
	require.NoError(t, err)
	assert.Equal(t, ReadyEquipmentUnready, result.Selection)
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotShield))
	assert.Equal(t, uint16(99), state.PartyState.Inventory.Equipment.Get(references.Dagger))
}

func TestApplyReadyEquipmentUnreadySearchesAllSixSlotsInPersistedOrder(t *testing.T) {
	state, character, request := newReadyExplorationTestState(t)
	for _, slot := range references.EquipmentSlots() {
		character.SetEquippedItem(slot, references.Dagger)
	}

	for index, wantSlot := range references.EquipmentSlots() {
		result, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.Dagger))
		require.NoError(t, err)
		assert.Equal(t, ReadyEquipmentUnready, result.Selection)
		assert.Equal(t, references.NoEquipment, character.GetEquippedItem(wantSlot))
		for _, remainingSlot := range references.EquipmentSlots()[index+1:] {
			assert.Equal(t, references.Dagger, character.GetEquippedItem(remainingSlot))
		}
	}
}

func TestApplyReadyEquipmentStrengthIncludesOccupiedHandWithoutReplacement(t *testing.T) {
	state, character, request := newReadyExplorationTestState(t)
	character.Strength = 9
	character.SetEquippedItem(references.SlotWeapon, references.LargeShield)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 1)

	_, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.LongSword))

	require.EqualError(t, err, "Needs 3 more Strength.")
	assert.Equal(t, references.LargeShield, character.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotShield))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.LongSword))
}

func TestApplyReadyEquipmentUnreadyLauncherDoesNotRequireAmmo(t *testing.T) {
	state, character, request := newReadyExplorationTestState(t)
	character.SetEquippedItem(references.SlotWeapon, references.Bow)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 0)

	result, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.Bow))

	require.NoError(t, err)
	assert.Equal(t, ReadyEquipmentUnready, result.Selection)
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Equipment.Get(references.Bow))
}

func TestApplyReadyEquipmentAmmoSelectionIsNoOp(t *testing.T) {
	state, character, request := newReadyExplorationTestState(t)
	state.PartyState.Inventory.Equipment.Set(references.Arrows, 7)

	result, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.Arrows))

	require.NoError(t, err)
	assert.Equal(t, ReadyEquipmentNoChange, result.Selection)
	assert.False(t, result.Changed)
	assert.Equal(t, uint16(7), state.PartyState.Inventory.Equipment.Get(references.Arrows))
}

func TestApplyReadyEquipmentRingVanishUsesExactAuthoritativeRoll(t *testing.T) {
	tests := []struct {
		name     string
		vanishes bool
	}{
		{name: "vanishes", vanishes: true},
		{name: "remains", vanishes: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := newReadyCombatTestState(t)
			clearReadyTestEquipment(state)
			state.SetRandomSeed(readyRingSeed(t, test.vanishes))
			request := readyCombatRequest(t, state)
			character := state.PartyState.GetCharacter(request.CharacterIndex)
			state.PartyState.Inventory.Equipment.Set(references.RingInvisibility, 1)
			callbacks.Reset()
			beforeCalls := state.GetRNG().Calls()

			result, err := state.ApplyReadyEquipment(
				readyTransaction(request, character, references.RingInvisibility),
			)

			require.NoError(t, err)
			assert.Equal(t, beforeCalls+1, state.GetRNG().Calls())
			assert.Zero(t, state.PartyState.Inventory.Equipment.Get(references.RingInvisibility))
			assert.Equal(t, test.vanishes, result.Vanished)
			assert.Equal(t, test.vanishes, result.Dismiss)
			if test.vanishes {
				assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotRing))
				assert.Contains(t, callbacks.Messages, "\n\nRing vanishes!\n")
			} else {
				assert.Equal(t, references.RingInvisibility, character.GetEquippedItem(references.SlotRing))
			}
		})
	}
}

func TestApplyReadyEquipmentRollsOnlyForSuccessfulVanishableRingEquip(t *testing.T) {
	state, _ := newReadyCombatTestState(t)
	clearReadyTestEquipment(state)
	request := readyCombatRequest(t, state)
	character := state.PartyState.GetCharacter(request.CharacterIndex)
	beforeCalls := state.GetRNG().Calls()

	state.PartyState.Inventory.Equipment.Set(references.RingProtection, 1)
	_, err := state.ApplyReadyEquipment(readyTransaction(request, character, references.RingProtection))
	require.NoError(t, err)
	assert.Equal(t, beforeCalls, state.GetRNG().Calls())

	_, err = state.ApplyReadyEquipment(readyTransaction(request, character, references.RingProtection))
	require.NoError(t, err)
	assert.Equal(t, beforeCalls, state.GetRNG().Calls(), "unready never rolls")

	state.PartyState.Inventory.Equipment.Set(references.RingRegeneration, 1)
	changedRequest := request
	changedRequest.CombatIdentity.Slot++
	_, err = state.ApplyReadyEquipment(ReadyEquipmentTransaction{
		Request: changedRequest, CharacterIndex: request.CharacterIndex,
		ExpectedOccupant: character.GetNameAsString(), Equipment: references.RingRegeneration,
	})
	require.ErrorIs(t, err, ErrReadyEquipment)
	assert.Equal(t, beforeCalls, state.GetRNG().Calls(), "a rejected ring equip never rolls")
}

func TestBeginReadyEquipmentEmptyConsumesWithoutDialog(t *testing.T) {
	state, callbacks := newReadyCombatTestState(t)
	clearReadyTestEquipment(state)
	callbacks.Reset()

	_, outcome := state.BeginReadyEquipment(true)

	assert.Equal(t, ReadyBeginCompleted, outcome)
	assert.False(t, callbacks.ReadyDialogShown)
	assert.Contains(t, callbacks.Messages, "Thou art empty-\nhanded!")
}

func newReadyCombatTestState(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := setupCombatAttackTargeting(t)
	state.BeginCombatTurns(CombatTurnsImmediate)
	state.CombatScene.Victory = false
	callbacks.Reset()

	return state, callbacks
}

func newReadyExplorationTestState(
	t *testing.T,
) (*GameState, *party_state.PlayerCharacter, ReadyEquipmentRequest) {
	t.Helper()
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	clearReadyTestEquipment(state)
	character := state.PartyState.GetCharacter(0)
	character.Strength = 30
	request := ReadyEquipmentRequest{CharacterIndex: 0}

	return state, character, request
}

func clearReadyTestEquipment(state *GameState) {
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		state.PartyState.Inventory.Equipment.Set(equipment, 0)
	}
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		if character == nil {
			continue
		}
		for _, slot := range references.EquipmentSlots() {
			character.SetEquippedItem(slot, references.NoEquipment)
		}
	}
}

func readyCombatRequest(t *testing.T, state *GameState) ReadyEquipmentRequest {
	t.Helper()
	identity := state.CombatScene.GetCurrentTurnIdentity()
	require.True(t, state.readyCombatActorValid(identity))

	return ReadyEquipmentRequest{
		CombatMode: true, CharacterIndex: identity.EntityIndex,
		LockCharacter: true, CombatIdentity: identity,
	}
}

func readyTransaction(
	request ReadyEquipmentRequest,
	character *party_state.PlayerCharacter,
	equipment references.Equipment,
) ReadyEquipmentTransaction {
	return ReadyEquipmentTransaction{
		Request: request, CharacterIndex: request.CharacterIndex,
		ExpectedOccupant: character.GetNameAsString(), Equipment: equipment,
	}
}

func firstReadyActivePC(state *GameState, start int) CombatTurnIdentity {
	for index := start; index < party_state.MaxActiveParty; index++ {
		identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: index}
		if state.CombatScene.TurnQueue.GetEntryByIdentity(identity) != nil {
			return identity
		}
	}

	return CombatTurnIdentity{}
}

func readyRingSeed(t *testing.T, vanishes bool) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 1024; seed++ {
		if (beradrng.New(seed).Intn(16) == 0) == vanishes {
			return seed
		}
	}
	t.Fatal("unable to locate deterministic Ready ring seed")

	return 0
}
