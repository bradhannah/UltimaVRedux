package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestCurrentPartyRosterSnapshotIsDetachedAndFingerprinted(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 2)
	original := state.CurrentPartyRosterSnapshot()

	original.Characters[0].Name = "Changed"
	original.Characters[0].CurrentHP = 0
	current := state.CurrentPartyRosterSnapshot()

	assert.NotEqual(t, "Changed", current.Characters[0].Name)
	assert.NotZero(t, current.Characters[0].CurrentHP)
	assert.Equal(t, current.Fingerprint, state.CurrentPartyRosterSnapshot().Fingerprint)
}

func TestSetAvatarNameRestoresSelectableIdentity(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 1)
	avatar := state.PartyState.GetAvatar()
	avatar.Name = [party_state.NMaxPlayerNameSize]byte{}
	assert.False(t, state.SetActivePlayer(0))

	require.NoError(t, state.SetAvatarName("Custom"))
	assert.Equal(t, "Custom", avatar.GetNameAsString())
	assert.True(t, state.SetActivePlayer(0), "restored Avatar identity must remain selectable with key 1")
	assert.Error(t, state.SetAvatarName(""))
	assert.Error(t, state.SetAvatarName("TooLongName"))
}

func TestMaximizeActivePartyUsesProgressionAndRecoveryRules(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 3)
	first := state.PartyState.GetCharacter(0)
	avatarName := first.GetNameAsString()
	first.Level, first.Exp = 2, 50
	first.SetMaxHp(60)
	first.SetHp(0)
	first.SetStatus(party_state.Dead)
	first.Class = party_state.Wizard
	first.Intelligence = 18
	second := state.PartyState.GetCharacter(1)
	second.Exp = 9000
	second.Class = party_state.Fighter
	second.SetMp(4)
	before := state.CurrentPartyRosterSnapshot()

	result, err := state.MaximizeActiveParty(before.Fingerprint)

	require.NoError(t, err)
	assert.Equal(t, before, result.Before)
	assert.NotEqual(t, result.Before.Fingerprint, result.After.Fingerprint)
	for index := range 3 {
		character := state.PartyState.GetCharacter(index)
		assert.Equal(t, byte(8), character.Level)
		assert.GreaterOrEqual(t, character.Exp, uint16(6400))
		assert.Equal(t, uint16(240), character.GetMaxHp())
		assert.Equal(t, uint16(240), character.GetCurrentHp())
	}
	assert.Equal(t, uint16(6400), first.Exp)
	assert.Equal(t, party_state.Good, first.GetStatus())
	assert.Equal(t, byte(18), first.GetCurrentMp())
	assert.Equal(t, avatarName, first.GetNameAsString(), "progression changes must preserve Avatar identity")
	assert.Equal(t, uint16(9000), second.Exp)
	assert.Equal(t, byte(4), second.GetCurrentMp())
}

func TestRestoreActivePartyHealsCuresAndRevivesOnlyTheActiveParty(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 3)
	dead := state.PartyState.GetCharacter(0)
	dead.Class, dead.Intelligence, dead.Level = party_state.Wizard, 20, 3
	dead.SetMaxHp(90)
	dead.SetHp(0)
	dead.SetMp(1)
	dead.SetStatus(party_state.Dead)
	poisoned := state.PartyState.GetCharacter(1)
	poisoned.SetMaxHp(60)
	poisoned.SetHp(5)
	poisoned.SetStatus(party_state.Poisoned)
	asleep := state.PartyState.GetCharacter(2)
	asleep.SetMaxHp(40)
	asleep.SetHp(40)
	asleep.SetStatus(party_state.Sleep)
	bench := state.PartyState.GetCharacter(3)
	bench.SetMaxHp(50)
	bench.SetHp(7)
	bench.SetStatus(party_state.Poisoned)
	before := state.CurrentPartyRosterSnapshot()

	result, err := state.RestoreActiveParty(before.Fingerprint)

	require.NoError(t, err)
	assert.Equal(t, before, result.Before)
	for index, maximum := range []uint16{90, 60, 40} {
		character := state.PartyState.GetCharacter(index)
		assert.Equal(t, maximum, character.GetCurrentHp(), "member %d", index)
		assert.Equal(t, party_state.Good, character.GetStatus(), "member %d", index)
	}
	assert.Equal(t, byte(20), dead.GetCurrentMp())
	assert.Equal(t, byte(3), dead.Level, "restoring does not change progression")
	assert.Equal(t, uint16(7), bench.GetCurrentHp(), "inactive companions are not part of the party")
	assert.Equal(t, party_state.Poisoned, bench.GetStatus())
}

func TestRestoreActivePartyRejectsStaleReview(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 2)
	snapshot := state.CurrentPartyRosterSnapshot()
	state.PartyState.GetCharacter(0).SetHp(1)

	_, err := state.RestoreActiveParty(snapshot.Fingerprint)

	require.ErrorIs(t, err, ErrStalePartyRoster)
	assert.Equal(t, uint16(1), state.PartyState.GetCharacter(0).GetCurrentHp())
}

func TestMaximizeActivePartySupportsVacantNativeSlots(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 1)
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	roster[0] = state.PartyState.GetCharacter(0)
	state.PartyState = *party_state.NewPartyStateFromRoster(roster)
	before := state.CurrentPartyRosterSnapshot()

	_, err := state.MaximizeActiveParty(before.Fingerprint)

	require.NoError(t, err)
	assert.Equal(t, byte(maximumCharacterLevel), state.PartyState.GetAvatar().Level)
}

func TestPartyMutationRejectsStaleAndCombatBeforeMutation(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 2)
	snapshot := state.CurrentPartyRosterSnapshot()
	state.PartyState.GetCharacter(0).Strength++
	changed := state.CurrentPartyRosterSnapshot()

	_, err := state.MaximizeActiveParty(snapshot.Fingerprint)
	require.ErrorIs(t, err, ErrStalePartyRoster)
	assert.Equal(t, changed, state.CurrentPartyRosterSnapshot())

	state.MapState.PlayerLocation.Location = references.CombatLocation
	_, err = state.FillActiveParty(changed.Fingerprint)
	require.ErrorIs(t, err, ErrPartyMutationInCombat)
	assert.Equal(t, changed, state.CurrentPartyRosterSnapshot())
}

func TestSetPartyRosterStatusesIsAtomicAndMaintainsHPConsistency(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 3)
	state.PartyState.GetCharacter(0).SetHp(20)
	state.PartyState.GetCharacter(1).SetHp(30)
	snapshot := state.CurrentPartyRosterSnapshot()

	deadResult, err := state.SetPartyRosterStatuses(PartyStatusMutationRequest{
		ExpectedFingerprint: snapshot.Fingerprint,
		RosterIndices:       []int{0, 1},
		Status:              party_state.Dead,
	})
	require.NoError(t, err)
	for _, index := range []int{0, 1} {
		character := state.PartyState.GetCharacter(index)
		assert.Equal(t, party_state.Dead, character.GetStatus())
		assert.Zero(t, character.GetCurrentHp())
	}

	_, err = state.SetPartyRosterStatuses(PartyStatusMutationRequest{
		ExpectedFingerprint: deadResult.After.Fingerprint,
		RosterIndices:       []int{0, party_state.NRosterSlots},
		Status:              party_state.Good,
	})
	require.ErrorIs(t, err, ErrInvalidPartyMutation)
	assert.Zero(t, state.PartyState.GetCharacter(0).GetCurrentHp(), "validation must precede every write")

	goodResult, err := state.SetPartyRosterStatuses(PartyStatusMutationRequest{
		ExpectedFingerprint: deadResult.After.Fingerprint,
		RosterIndices:       []int{0, 1},
		Status:              party_state.Good,
	})
	require.NoError(t, err)
	for _, index := range []int{0, 1} {
		character := state.PartyState.GetCharacter(index)
		assert.Equal(t, party_state.Good, character.GetStatus())
		assert.Equal(t, uint16(1), character.GetCurrentHp())
	}
	assert.Equal(t, goodResult.After, state.CurrentPartyRosterSnapshot())
}

func TestFillActivePartyPreservesContiguousRosterOrder(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 2)
	before := state.CurrentPartyRosterSnapshot()
	wantNames := make([]string, party_state.MaxActiveParty)
	for index := range party_state.MaxActiveParty {
		wantNames[index] = before.Characters[index].Name
	}

	result, err := state.FillActiveParty(before.Fingerprint)

	require.NoError(t, err)
	assert.Equal(t, party_state.MaxActiveParty, state.PartyState.GetActivePartyCount())
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		assert.True(t, character.IsInParty())
		assert.Equal(t, wantNames[index], character.GetNameAsString())
	}
	for index := party_state.MaxActiveParty; index < party_state.NRosterSlots; index++ {
		assert.False(t, state.PartyState.GetCharacter(index).IsInParty())
	}
	assert.Equal(t, state.CurrentPartyRosterSnapshot(), result.After)

	_, err = state.FillActiveParty(result.After.Fingerprint)
	require.ErrorIs(t, err, ErrPartyCapacity)
}

func TestAddInactiveRosterMemberUsesStableIndexAndStalePrecondition(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 2)
	before := state.CurrentPartyRosterSnapshot()
	targetName := before.Characters[8].Name

	result, err := state.AddInactiveRosterMember(AddInactiveRosterMemberRequest{
		ExpectedFingerprint: before.Fingerprint,
		RosterIndex:         8,
	})
	require.NoError(t, err)
	assert.Equal(t, targetName, state.PartyState.GetCharacter(2).GetNameAsString())
	assert.True(t, state.PartyState.GetCharacter(2).IsInParty())
	assert.Equal(t, 3, state.PartyState.GetActivePartyCount())

	current := state.CurrentPartyRosterSnapshot()
	_, err = state.AddInactiveRosterMember(AddInactiveRosterMemberRequest{
		ExpectedFingerprint: before.Fingerprint,
		RosterIndex:         9,
	})
	require.ErrorIs(t, err, ErrStalePartyRoster)
	assert.Equal(t, current, state.CurrentPartyRosterSnapshot())
	assert.Equal(t, result.After, current)
}

func TestAddInactiveRosterMemberRejectsCapacity(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, party_state.MaxActiveParty)
	before := state.CurrentPartyRosterSnapshot()

	_, err := state.AddInactiveRosterMember(AddInactiveRosterMemberRequest{
		ExpectedFingerprint: before.Fingerprint,
		RosterIndex:         8,
	})

	require.ErrorIs(t, err, ErrPartyCapacity)
	assert.Equal(t, before, state.CurrentPartyRosterSnapshot())
}

func TestPartyRosterAuthoritySupportsVacantNativeSlots(t *testing.T) {
	state, _ := newPartyRosterAuthorityTestState(t, 1)
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	roster[0] = state.PartyState.GetCharacter(0)
	roster[8] = state.PartyState.GetCharacter(8)
	state.PartyState = *party_state.NewPartyStateFromRoster(roster)
	before := state.CurrentPartyRosterSnapshot()

	assert.Empty(t, before.Characters[1].Name)
	assert.Equal(t, 1, before.Characters[1].RosterIndex)
	result, err := state.FillActiveParty(before.Fingerprint)

	require.NoError(t, err)
	assert.Equal(t, 2, state.PartyState.GetActivePartyCount())
	assert.Equal(t, before.Characters[8].Name, state.PartyState.GetCharacter(1).GetNameAsString())
	_, err = state.AddInactiveRosterMember(AddInactiveRosterMemberRequest{
		ExpectedFingerprint: result.After.Fingerprint,
		RosterIndex:         8,
	})
	require.ErrorIs(t, err, ErrInvalidPartyMutation)
}

func newPartyRosterAuthorityTestState(t *testing.T, activeCount int) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	for index := range party_state.NRosterSlots {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapAtInn(references.Britain)
		character.SetStatus(party_state.Good)
		character.SetMaxHp(90)
		character.SetHp(90)
	}
	for index := range activeCount {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapActive
	}
	callbacks.Reset()

	return state, callbacks
}
