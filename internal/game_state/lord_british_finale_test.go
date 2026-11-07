package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/minimap"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestBeginLordBritishFinaleStagesSourceMapPartyAndRevival(t *testing.T) {
	state := newLordBritishFinaleTestState(t, party_state.MaxActiveParty)
	avatar := state.PartyState.GetCharacter(0)
	deadCompanion := state.PartyState.GetCharacter(1)
	poisonedCompanion := state.PartyState.GetCharacter(2)
	deadFinalCompanion := state.PartyState.GetCharacter(5)
	avatar.SetHp(70)
	deadCompanion.SetStatus(party_state.Dead)
	deadCompanion.SetHp(0)
	poisonedCompanion.SetStatus(party_state.Poisoned)
	poisonedCompanion.SetHp(40)
	deadFinalCompanion.SetStatus(party_state.Dead)
	deadFinalCompanion.SetHp(0)
	rescueMap := lordBritishFinaleMapFixture()

	review, err := state.beginLordBritishFinaleWithMap(rescueMap)

	require.NoError(t, err)
	assert.False(t, state.HasPendingLordBritishFinale())
	assert.False(t, state.CanAcceptGameplayCommands())
	assert.Equal(t, references.FinaleLocation, state.MapState.PlayerLocation.Location)
	assert.Equal(t, references.Position{X: 5, Y: 5}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, references.Position{X: 5, Y: 8}, review.LordBritishStart)
	assert.Equal(t, references.Position{X: 5, Y: 3}, review.LordBritishEnd)
	assert.Equal(t, references.Position{X: 5, Y: 4}, review.BoxMoongatePosition)
	require.Len(t, review.Party, party_state.MaxActiveParty)
	destinations := make([]references.Position, 0, len(review.Party))
	for _, actor := range review.Party {
		destinations = append(destinations, actor.Destination)
	}
	assert.Equal(t, lordBritishFinalePartyDestinations[:], destinations)
	assert.False(t, review.Party[0].Revived)
	assert.True(t, review.Party[1].Revived)
	assert.False(t, review.Party[2].Revived)
	assert.True(t, review.Party[5].Revived)
	assert.Equal(t, party_state.Good, deadCompanion.GetStatus())
	assert.Equal(t, deadCompanion.GetMaxHp(), deadCompanion.GetCurrentHp())
	assert.Equal(t, party_state.Poisoned, poisonedCompanion.GetStatus())
	assert.Equal(t, uint16(40), poisonedCompanion.GetCurrentHp())
	assert.Equal(t, uint16(70), avatar.GetCurrentHp())
	assert.Equal(t, party_state.Good, deadFinalCompanion.GetStatus())
	assert.Equal(t, deadFinalCompanion.GetMaxHp(), deadFinalCompanion.GetCurrentHp())
	tile, ok := state.LordBritishFinaleTile(references.Position{X: 4, Y: 5})
	require.True(t, ok)
	assert.Equal(t, indexes.SpriteIndex(rescueMap.Tiles[4][5]), tile)
	require.True(t, state.SetLordBritishFinaleTile(references.Position{X: 5, Y: 4}, indexes.Moongate))
	tile, ok = state.LordBritishFinaleTile(references.Position{X: 5, Y: 4})
	require.True(t, ok)
	assert.Equal(t, indexes.SpriteIndex(indexes.Moongate), tile)
	assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
}

func TestLordBritishFinaleExactAnswerTruthTable(t *testing.T) {
	tests := []struct {
		name        string
		answers     string
		hasBox      bool
		wantOutcome LordBritishFinaleOutcome
		wantMissing bool
	}{
		{name: "box yes with ownership", answers: "Y", hasBox: true, wantOutcome: LordBritishFinaleSucceeded},
		{name: "box yes without ownership", answers: "Y", hasBox: false, wantOutcome: LordBritishFinaleFailed, wantMissing: true},
		{name: "clarified yes with ownership", answers: "NY", hasBox: true, wantOutcome: LordBritishFinaleSucceeded},
		{name: "clarified yes without ownership", answers: "NY", hasBox: false, wantOutcome: LordBritishFinaleFailed, wantMissing: true},
		{name: "clarified no", answers: "NN", hasBox: true, wantOutcome: LordBritishFinaleFailed},
		{name: "honest no without ownership", answers: "NN", hasBox: false, wantOutcome: LordBritishFinaleFailed, wantMissing: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state := begunLordBritishFinaleTestState(t, 1)
			state.DateTime = datetime.UltimaDate{Year: 142, Month: 7, Day: 11}
			if test.hasBox {
				state.PartyState.Inventory.QuestItems.Set(references.WoodenBox, 1)
			}

			var result LordBritishFinaleAnswerResult
			for index := range test.answers {
				result = state.SubmitLordBritishFinaleAnswer(test.answers[index])
				require.True(t, result.Accepted)
			}

			assert.Equal(t, test.wantOutcome, result.Outcome)
			assert.Equal(t, test.wantMissing, result.MissingBox)
			assert.Equal(t, test.wantOutcome, state.LordBritishFinaleOutcome())
			assert.True(t, state.IsGameplayTerminal())
			assert.False(t, state.CanAcceptGameplayCommands())
			date, hasDate := state.LordBritishFinaleCompletionDate()
			if test.wantOutcome == LordBritishFinaleSucceeded {
				assert.True(t, hasDate)
				assert.Equal(t, datetime.BritannianDate{Year: 142, Month: 7, Day: 11}, date)
			} else {
				assert.False(t, hasDate)
			}
		})
	}
}

func TestLordBritishFinaleRejectsNonUppercaseAnswersWithoutAdvancing(t *testing.T) {
	state := begunLordBritishFinaleTestState(t, 1)
	state.PartyState.Inventory.QuestItems.Set(references.WoodenBox, 1)

	for _, answer := range []byte{'y', 'n', ' ', '\n'} {
		result := state.SubmitLordBritishFinaleAnswer(answer)
		assert.False(t, result.Accepted)
		assert.Equal(t, LordBritishFinaleBoxPrompt, result.Prompt)
		assert.Equal(t, LordBritishFinaleInProgress, result.Outcome)
	}

	result := state.SubmitLordBritishFinaleAnswer('N')
	require.True(t, result.Accepted)
	assert.Equal(t, LordBritishFinaleSandalwoodPrompt, result.Prompt)
	result = state.SubmitLordBritishFinaleAnswer('y')
	assert.False(t, result.Accepted)
	assert.Equal(t, LordBritishFinaleSandalwoodPrompt, result.Prompt)
	assert.Equal(t, LordBritishFinaleInProgress, result.Outcome)
}

func begunLordBritishFinaleTestState(t *testing.T, partyCount int) *GameState {
	t.Helper()
	state := newLordBritishFinaleTestState(t, partyCount)
	_, err := state.beginLordBritishFinaleWithMap(lordBritishFinaleMapFixture())
	require.NoError(t, err)

	return state
}

func newLordBritishFinaleTestState(t *testing.T, partyCount int) *GameState {
	t.Helper()
	state, _ := newDoomMirrorCombatTestState(t, partyCount, false)
	state.CombatScene = nil
	state.finaleHandoffPending = true
	state.GameReferences = &references.GameReferences{TileReferences: references.NewTileReferences()}

	return state
}

func lordBritishFinaleMapFixture() minimap.MiscMap {
	var result minimap.MiscMap
	for x := range minimap.MiscMapSize {
		for y := range minimap.MiscMapSize {
			result.Tiles[x][y] = byte(indexes.BrickFloor)
		}
	}
	result.Tiles[4][5] = byte(indexes.MetalFloor)

	return result
}
