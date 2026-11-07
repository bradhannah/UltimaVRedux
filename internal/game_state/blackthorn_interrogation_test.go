package game_state

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestBlackthornCorrectAnswerClosesShrineAndMartyrsSecondLiving(t *testing.T) {
	state, _ := newBlackthornTestState(t, 3)
	state.PartyState.Karma.Value = 3
	state.PartyState.Inventory.Provisions.Keys.Set(9)
	state.SetWindDirection(environment.East, false)
	removedBefore := *state.PartyState.GetCharacter(1)

	review, err := state.BeginBlackthornInterrogation()
	require.NoError(t, err)
	require.Equal(t, 0, review.ShrineIndex)
	require.Equal(t, references.BlackthornPrison, state.MapState.PlayerLocation.Location)

	outcome, err := state.SubmitBlackthornAnswer(review, 0, "I say AHM with conviction")
	require.NoError(t, err)
	require.True(t, outcome.Correct)
	require.True(t, outcome.Resolved)
	require.Equal(t, removedBefore.GetNameAsString(), outcome.MartyrName)
	require.True(t, state.IsShrineRuined(0))
	require.Zero(t, state.PartyState.Karma.Value)
	removedBefore.PartyMap = party_state.PartyMapMartyr
	require.True(t, reflect.DeepEqual(removedBefore, *state.PartyState.GetCharacter(15)))
	require.False(t, state.PartyState.GetCharacter(15).IsDead())

	state.FinishBlackthornInterrogation()
	require.Equal(t, references.PalaceOfBlackthorn, state.MapState.PlayerLocation.Location)
	require.Equal(t, references.Basement, state.MapState.PlayerLocation.Floor)
	require.Equal(t, references.Position{X: 10, Y: 7}, state.MapState.PlayerLocation.Position)
	require.Zero(t, state.PartyState.Inventory.Provisions.Keys.Get())
	require.Equal(t, environment.East, state.GetCurrentWind(), "setwind(-1) displays but does not alter wind")
}

func TestBlackthornWrongAnswersUseOriginalTortureTiming(t *testing.T) {
	state, _ := newBlackthornTestState(t, 2)
	state.DateTime.Hour, state.DateTime.Minute = 5, 55
	state.lastProcessedHour = 5
	state.PartyState.Inventory.Provisions.Food.Set(100)
	foodBefore := state.PartyState.Inventory.Provisions.Food.Get()
	review, err := state.BeginBlackthornInterrogation()
	require.NoError(t, err)
	before := state.DateTime

	var elapsed []int
	for attempt := range blackthornAnswerCount {
		outcome, submitErr := state.SubmitBlackthornAnswer(review, attempt, "wrong")
		require.NoError(t, submitErr)
		elapsed = append(elapsed, outcome.ElapsedMinutes)
		if attempt < blackthornAnswerCount-1 {
			require.False(t, outcome.Resolved)
		} else {
			require.True(t, outcome.Resolved)
			require.NotEmpty(t, outcome.MartyrName)
		}
	}
	require.Equal(t, []int{0, 2, 2, 2}, elapsed)
	require.Equal(t, before.Year, state.DateTime.Year)
	require.Equal(t, before.Month, state.DateTime.Month)
	require.Equal(t, before.Day, state.DateTime.Day)
	require.Equal(t, int(before.Hour)*60+int(before.Minute)+6, int(state.DateTime.Hour)*60+int(state.DateTime.Minute))
	require.Equal(t, before.Turn+3, state.DateTime.Turn)
	require.Equal(t, 1, state.PartyState.GetActivePartyCount())
	require.NotEmpty(t, state.pendingProvisionHours)

	state.FinishBlackthornInterrogation()

	require.Empty(t, state.pendingProvisionHours)
	require.Less(t, state.PartyState.Inventory.Provisions.Food.Get(), foodBefore)
}

func TestBlackthornLoneLivingWrongAnswerExitsImmediately(t *testing.T) {
	state, callbacks := newBlackthornTestState(t, 1)
	review, err := state.BeginBlackthornInterrogation()
	require.NoError(t, err)
	before := state.DateTime

	outcome, err := state.SubmitBlackthornAnswer(review, 0, "wrong")
	require.NoError(t, err)
	require.True(t, outcome.Resolved)
	require.True(t, outcome.LoneLiving)
	require.Zero(t, outcome.ElapsedMinutes)
	require.Equal(t, before, state.DateTime)
	require.Empty(t, callbacks.TimeAdvanced())
	require.Empty(t, state.PartyState.MartyrNames())
}

func TestBlackthornAllShrinesClosedSkipsQuestionButRestoresExit(t *testing.T) {
	state, _ := newBlackthornTestState(t, 2)
	for index := range TotalShrines {
		state.SetShrineRuined(index, true)
	}
	state.PartyState.Inventory.Provisions.Keys.Set(4)

	review, err := state.BeginBlackthornInterrogation()
	require.NoError(t, err)
	require.False(t, review.NeedsAnswer)
	require.Equal(t, -1, review.ShrineIndex)
	require.Equal(t, references.PalaceOfBlackthorn, state.MapState.PlayerLocation.Location)

	state.FinishBlackthornInterrogation()
	require.Equal(t, references.Position{X: 10, Y: 7}, state.MapState.PlayerLocation.Position)
	require.Zero(t, state.PartyState.Inventory.Provisions.Keys.Get())
}

func TestBlackthornMartyrSurvivesNativeSnapshotHydration(t *testing.T) {
	state, _ := newBlackthornTestState(t, 3)
	review, err := state.BeginBlackthornInterrogation()
	require.NoError(t, err)
	_, err = state.SubmitBlackthornAnswer(review, 0, review.Mantra)
	require.NoError(t, err)
	state.FinishBlackthornInterrogation()
	want := *state.PartyState.GetCharacter(15)

	hydrated := roundTripSaveSnapshot(t, state)

	got := *hydrated.PartyState.GetCharacter(15)
	require.True(t, reflect.DeepEqual(want, got))
	require.Equal(t, party_state.PartyMapMartyr, got.PartyMap)
	require.False(t, got.IsDead())
}

func newBlackthornTestState(t *testing.T, living int) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.PalaceOfBlackthorn).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()
	for index := range party_state.NRosterSlots {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapUnrecruited
		character.SetStatus(party_state.Dead)
	}
	for index := range living {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapActive
		character.SetStatus(party_state.Good)
		character.SetMaxHp(100)
		character.SetHp(100)
	}
	for index := range TotalShrines {
		state.SetShrineRuined(index, false)
	}

	return state, callbacks
}
