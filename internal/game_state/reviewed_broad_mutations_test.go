package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsableInventoryPresetIsCatalogDerivedExactAndIdempotent(t *testing.T) {
	state := newReviewedMutationTestState(t)
	state.PartyState.Inventory.QuestItems.Set(references.Crown, 2)
	state.PartyState.Inventory.Potions.Set(references.Blue, 9)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollVasLor, 1)

	review, err := state.ReviewUsableInventoryPreset()
	require.NoError(t, err)
	require.Len(t, review.Changes, references.NumberOfQuestItems+references.NumberOfMoonstones+references.NumberOfPotions+references.NumberOfScrolls)
	assertInventoryChange(t, review.Changes, "quest_items.1", 2, UsableInventoryQuestItemTarget)
	assertInventoryChange(t, review.Changes, "potions.0", 9, UsableInventoryPotionTarget)
	assertInventoryChange(t, review.Changes, "scrolls.0", 1, UsableInventoryScrollTarget)

	result, err := state.ApplyUsableInventoryPreset(review.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, review.Fingerprint, result.BeforeFingerprint)
	assert.NotEqual(t, result.BeforeFingerprint, result.AfterFingerprint)
	assert.Equal(t, review.Changes, result.Changes)
	for _, item := range references.QuestItemCatalog() {
		assert.Equal(t, UsableInventoryQuestItemTarget, state.PartyState.Inventory.QuestItems.Get(item))
	}
	for _, moonstone := range references.MoonstoneCatalog() {
		assert.Equal(t, UsableInventoryMoonstoneTarget, state.PartyState.Inventory.Moonstones.Get(moonstone))
	}
	for _, potion := range references.PotionCatalog() {
		assert.Equal(t, UsableInventoryPotionTarget, state.PartyState.Inventory.Potions.Get(potion))
	}
	for _, scroll := range references.ScrollCatalog() {
		assert.Equal(t, UsableInventoryScrollTarget, state.PartyState.Inventory.Scrolls.Get(scroll))
	}

	idempotentReview, err := state.ReviewUsableInventoryPreset()
	require.NoError(t, err)
	assert.Empty(t, idempotentReview.Changes)
	idempotent, err := state.ApplyUsableInventoryPreset(idempotentReview.Fingerprint)
	require.NoError(t, err)
	assert.Empty(t, idempotent.Changes)
	assert.Equal(t, idempotent.BeforeFingerprint, idempotent.AfterFingerprint)
}

func TestUsableInventoryPresetRejectsStaleReviewWithoutPartialMutation(t *testing.T) {
	state := newReviewedMutationTestState(t)
	review, err := state.ReviewUsableInventoryPreset()
	require.NoError(t, err)
	state.PartyState.Inventory.QuestItems.Set(references.Crown, 7)

	_, err = state.ApplyUsableInventoryPreset(review.Fingerprint)
	require.ErrorIs(t, err, ErrUsableInventoryPresetStale)
	assert.Equal(t, uint16(7), state.PartyState.Inventory.QuestItems.Get(references.Crown))
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Potions.Get(references.Blue))
	assert.Equal(t, uint16(0), state.PartyState.Inventory.Scrolls.Get(references.ScrollAnTym))
}

func TestWipeActivePartyChangesEveryActiveMemberWithConsistentHPAndStatus(t *testing.T) {
	state := newReviewedMutationTestState(t)
	activateMutationTestCharacter(state, 0, "Avatar", 20, party_state.Good)
	activateMutationTestCharacter(state, 1, "Iolo", 0, party_state.Good)
	activateMutationTestCharacter(state, 2, "Shamino", 8, party_state.Poisoned)
	inactive := state.PartyState.GetCharacter(3)
	inactive.SetMaxHp(30)
	inactive.SetHp(11)
	inactive.SetStatus(party_state.Sleep)

	review := state.ReviewActivePartyWipe()
	require.Len(t, review.Members, 3)
	result, err := state.WipeActiveParty(review.Fingerprint)
	require.NoError(t, err)
	require.Len(t, result.Changes, 3)
	for slot := 0; slot < 3; slot++ {
		character := state.PartyState.GetCharacter(slot)
		assert.Zero(t, character.GetCurrentHp(), "slot %d HP", slot)
		assert.Equal(t, party_state.Dead, character.GetStatus(), "slot %d status", slot)
		assert.Equal(t, slot, result.Changes[slot].Before.Slot)
		assert.Zero(t, result.Changes[slot].After.HP)
		assert.Equal(t, party_state.Dead, result.Changes[slot].After.Status)
	}
	assert.Equal(t, uint16(11), inactive.GetCurrentHp())
	assert.Equal(t, party_state.Sleep, inactive.GetStatus())
	assert.True(t, state.PartyState.IsAllPartyDead())

	idempotentReview := state.ReviewActivePartyWipe()
	idempotent, err := state.WipeActiveParty(idempotentReview.Fingerprint)
	require.NoError(t, err)
	assert.Empty(t, idempotent.Changes)
}

func TestWipeActivePartyRejectsStaleReviewWithoutPartialMutation(t *testing.T) {
	state := newReviewedMutationTestState(t)
	activateMutationTestCharacter(state, 0, "Avatar", 20, party_state.Good)
	activateMutationTestCharacter(state, 1, "Iolo", 20, party_state.Good)
	review := state.ReviewActivePartyWipe()
	state.PartyState.GetCharacter(0).SetHp(19)

	_, err := state.WipeActiveParty(review.Fingerprint)
	require.ErrorIs(t, err, ErrActivePartyWipeStale)
	assert.Equal(t, uint16(19), state.PartyState.GetCharacter(0).GetCurrentHp())
	assert.Equal(t, party_state.Good, state.PartyState.GetCharacter(0).GetStatus())
	assert.Equal(t, uint16(20), state.PartyState.GetCharacter(1).GetCurrentHp())
	assert.Equal(t, party_state.Good, state.PartyState.GetCharacter(1).GetStatus())
}

func TestSetAllShrineQuestStateNormalizesAllEightShrines(t *testing.T) {
	state := newReviewedMutationTestState(t)
	for index := range TotalShrines {
		state.SetShrineQuestCompleted(index, index%3 == 2)
		state.SetShrineQuestActive(index, index%3 != 0)
	}
	review := state.ReviewAllShrineQuestState()
	require.Len(t, review.Shrines, TotalShrines)

	completed, err := state.SetAllShrineQuestState(AllShrineQuestCompleted, review.Fingerprint)
	require.NoError(t, err)
	require.Len(t, completed.Changes, TotalShrines)
	for index, change := range completed.Changes {
		assert.Equal(t, index, change.Index)
		assert.True(t, change.After.Completed)
		assert.False(t, change.After.Active)
		assert.Equal(t, ShrineProgressOffering, change.After.QuestState)
	}
	for index := range TotalShrines {
		progress := state.ShrineProgress(index)
		assert.True(t, progress.Completed)
		assert.False(t, progress.Active)
	}

	nextReview := state.ReviewAllShrineQuestState()
	notAssigned, err := state.SetAllShrineQuestState(AllShrineQuestNotAssigned, nextReview.Fingerprint)
	require.NoError(t, err)
	require.Len(t, notAssigned.Changes, TotalShrines)
	for index := range TotalShrines {
		progress := state.ShrineProgress(index)
		assert.False(t, progress.Completed)
		assert.False(t, progress.Active)
		assert.Equal(t, ShrineProgressNotAssigned, progress.QuestState)
	}
}

func TestSetAllShrineQuestStateRejectsStaleReviewWithoutPartialMutation(t *testing.T) {
	state := newReviewedMutationTestState(t)
	state.SetShrineQuestActive(2, true)
	review := state.ReviewAllShrineQuestState()
	state.SetShrineQuestCompleted(4, true)
	beforeAttempt := state.ReviewAllShrineQuestState()

	_, err := state.SetAllShrineQuestState(AllShrineQuestCompleted, review.Fingerprint)
	require.ErrorIs(t, err, ErrAllShrineQuestStateStale)
	assert.Equal(t, beforeAttempt.Shrines, state.ReviewAllShrineQuestState().Shrines)
}

func TestReviewedBroadMutationsRejectCombatAndWorkflowContextsBeforeMutation(t *testing.T) {
	state := newReviewedMutationTestState(t)
	inventoryReview, err := state.ReviewUsableInventoryPreset()
	require.NoError(t, err)
	state.MapState.PlayerLocation.Location = references.CombatLocation

	_, err = state.ApplyUsableInventoryPreset(inventoryReview.Fingerprint)
	require.ErrorIs(t, err, ErrUsableInventoryPresetInvalid)
	assert.Equal(t, uint16(0), state.PartyState.Inventory.QuestItems.Get(references.Amulet))

	state.MapState.PlayerLocation.Location = references.Britain
	activateMutationTestCharacter(state, 0, "Avatar", 20, party_state.Good)
	wipeReview := state.ReviewActivePartyWipe()
	state.EventQueueManager.Workflow("mutation-test").
		Priority(event_queue.PriorityNormal).
		Sequential().
		First(func() {}).
		Finally(func() {}).
		Start()

	_, err = state.WipeActiveParty(wipeReview.Fingerprint)
	require.ErrorIs(t, err, ErrActivePartyWipeInvalid)
	assert.Equal(t, uint16(20), state.PartyState.GetCharacter(0).GetCurrentHp())
	assert.Equal(t, party_state.Good, state.PartyState.GetCharacter(0).GetStatus())
}

func newReviewedMutationTestState(t *testing.T) *GameState {
	t.Helper()
	party := party_state.NewBlankPartyState()
	for slot := range party_state.NRosterSlots {
		party.GetCharacter(slot).PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	callbacks := NewMockSystemCallbacks(t)

	return &GameState{
		PartyState:        *party,
		EventQueueManager: event_queue.NewEventQueueManager(func() int64 { return 0 }, nil),
		SystemCallbacks:   callbacks.ToSystemCallbacks(),
		MapState:          newMutationTestMapState(),
	}
}

func newMutationTestMapState() map_state.MapState {
	return map_state.MapState{PlayerLocation: references.PlayerLocation{Location: references.Britain}}
}

func activateMutationTestCharacter(
	state *GameState,
	slot int,
	name string,
	hp uint16,
	status party_state.CharacterStatus,
) {
	character := state.PartyState.GetCharacter(slot)
	copy(character.Name[:], name)
	character.PartyMap = party_state.PartyMapActive
	character.SetMaxHp(30)
	character.SetHp(hp)
	character.SetStatus(status)
}

func assertInventoryChange(t *testing.T, changes []DebugInventoryChange, path string, before, after uint16) {
	t.Helper()
	for _, change := range changes {
		if change.Path == path {
			assert.Equal(t, before, change.Before)
			assert.Equal(t, after, change.After)

			return
		}
	}
	t.Fatalf("inventory change %q not found", path)
}
