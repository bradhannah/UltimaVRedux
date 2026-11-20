package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func circleStudyState(t *testing.T) *GameState {
	t.Helper()
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Lycaeum).
		WithSystemCallbacks().
		Build()
	require.GreaterOrEqual(t, gs.PartyState.GetActivePartyCount(), 3)

	return gs
}

func TestCircleStudyLiftsCastingByOneCircleOnceForGold(t *testing.T) {
	gs := circleStudyState(t)
	caster := gs.PartyState.GetCharacter(0)
	caster.Class = party_state.Wizard
	caster.Level = 7
	caster.SetMp(8)
	gs.PartyState.Inventory.Spells.Set(references.AnTym, 1)
	eighthCircle := CastRequest{Spell: references.AnTym, CasterIdx: 0}
	require.Equal(t, "Level too low!", gs.ValidateSpellResources(eighthCircle).ErrorMsg)

	gs.PartyState.Inventory.Gold.Set(CircleStudyPrice - 1)
	require.ErrorIs(t, gs.StudyCircle(0), ErrCircleStudyInsufficientGold)
	assert.False(t, caster.HasCircleStudy(), "a party that cannot pay learns nothing")
	assert.Equal(t, uint16(CircleStudyPrice-1), gs.PartyState.Inventory.Gold.Get())

	gs.PartyState.Inventory.Gold.Set(6819)
	require.NoError(t, gs.StudyCircle(0))
	assert.Equal(t, uint16(3819), gs.PartyState.Inventory.Gold.Get())
	assert.Equal(t, byte(7), caster.Level, "study changes no level")
	assert.True(t, gs.ValidateSpellResources(eighthCircle).Valid, "a level 7 member who studied casts the eighth circle")

	require.ErrorIs(t, gs.StudyCircle(0), ErrCircleStudyUnavailable, "each member studies once")
	assert.Equal(t, uint16(3819), gs.PartyState.Inventory.Gold.Get())

	caster.Level = 6
	assert.Equal(t, 7, caster.SpellCircleLimit(), "the extra circle follows the member's level down")
	assert.Equal(t, "Level too low!", gs.ValidateSpellResources(eighthCircle).ErrorMsg)
	caster.Level = 8
	assert.Equal(t, references.MaxSpellCircle, caster.SpellCircleLimit(), "there is no ninth circle")
}

func TestCircleStudyStateSaysWhoMayStudyAndWhyNot(t *testing.T) {
	gs := circleStudyState(t)
	gs.PartyState.Inventory.Gold.Set(6819)
	eligible, fighter, fallen := gs.PartyState.GetCharacter(0), gs.PartyState.GetCharacter(1), gs.PartyState.GetCharacter(2)
	eligible.Class, eligible.Level = party_state.Avatar, 7
	fighter.Class, fighter.Level = party_state.Fighter, 7
	fallen.Class, fallen.Level = party_state.Bard, 5
	fallen.SetStatus(party_state.Dead)

	state := gs.GetCircleStudyState()

	assert.Equal(t, CircleStudyPrice, state.Price)
	assert.Equal(t, 6819, state.Gold)
	require.Len(t, state.Candidates, gs.PartyState.GetActivePartyCount())
	assert.Equal(t, CircleStudyCandidate{
		RosterIndex: 0, Name: eligible.GetNameAsString(), Class: "Avatar", Level: 7, Circle: 7,
		Eligibility: CircleStudyEligible,
	}, state.Candidates[0])
	assert.Equal(t, CircleStudyNoMagic, state.Candidates[1].Eligibility)
	assert.Zero(t, state.Candidates[1].Circle, "a class without magic has no circle")
	assert.Equal(t, CircleStudyDead, state.Candidates[2].Eligibility)
	require.ErrorIs(t, gs.StudyCircle(1), ErrCircleStudyUnavailable)
	require.ErrorIs(t, gs.StudyCircle(2), ErrCircleStudyUnavailable)
	require.ErrorIs(t, gs.StudyCircle(party_state.MaxActiveParty), ErrCircleStudyUnavailable)
	assert.Equal(t, uint16(6819), gs.PartyState.Inventory.Gold.Get(), "a refusal costs nothing")

	require.NoError(t, gs.StudyCircle(0))
	assert.Equal(t, CircleStudyAlreadyStudied, gs.GetCircleStudyState().Candidates[0].Eligibility)
	fallen.SetStatus(party_state.Good)
	fallen.Level = 8
	assert.Equal(t, CircleStudyNothingToLearn, gs.GetCircleStudyState().Candidates[2].Eligibility)
}

func TestCircleStudyFollowsTheMemberThroughPartyOrderAndSaves(t *testing.T) {
	gs := circleStudyState(t)
	assert.Nil(t, gs.circleStudyRosterIndices(), "no member studied is the zero value")
	studied := gs.PartyState.GetCharacter(1)
	studied.SetCircleStudy(true)
	require.NoError(t, gs.SwapPartyMembers(1, 2))
	assert.Same(t, studied, gs.PartyState.GetCharacter(2))
	assert.Equal(t, []int{2}, gs.circleStudyRosterIndices())

	snapshot, err := gs.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, []int{2}, snapshot.CircleStudy)

	restored := circleStudyState(t)
	require.NoError(t, restored.restoreCircleStudy(snapshot.CircleStudy))
	assert.True(t, restored.PartyState.GetCharacter(2).HasCircleStudy())
	assert.False(t, restored.PartyState.GetCharacter(1).HasCircleStudy())
	require.ErrorIs(t, restored.restoreCircleStudy([]int{party_state.NRosterSlots}), ErrCircleStudyUnavailable,
		"a slot with no member cannot have studied")
}
