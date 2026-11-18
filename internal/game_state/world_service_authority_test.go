package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/shrine"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestWishingWellCommitRejectsStaleAndFailedRequestsWithoutPartialEffects(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Paws).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(10)
	wellPosition := findWishingWellForTest(t, state)

	review, err := state.ReviewWishingWell(wellPosition)
	require.NoError(t, err)
	review.Gold = 0
	review.Message = "detached"
	assert.Equal(t, uint16(10), state.PartyState.Inventory.Gold.Get())
	review, err = state.ReviewWishingWell(wellPosition)
	require.NoError(t, err)

	state.PartyState.Inventory.Gold.Set(9)
	dateBefore := state.DateTime
	_, err = state.CommitWishingWellWish(WishingWellCommitRequest{
		WellPosition: wellPosition, Wish: "nothing", ExpectedFingerprint: review.Fingerprint,
	})
	require.ErrorIs(t, err, ErrWishingWellRequestStale)
	assert.Equal(t, uint16(9), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, dateBefore, state.DateTime)
	assert.Equal(t, references.NoPartyVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)

}

func TestWishingWellCommitIsSingleUseAndReturnsDetachedPresentationOutcome(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Paws).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(10)
	wellPosition := findWishingWellForTest(t, state)
	review, err := state.ReviewWishingWell(wellPosition)
	require.NoError(t, err)

	outcome, err := state.CommitWishingWellWish(WishingWellCommitRequest{
		WellPosition: wellPosition, Wish: "nothing", ExpectedFingerprint: review.Fingerprint,
	})
	require.NoError(t, err)
	assert.Equal(t, WishingWellNoEffect, outcome.Effect)
	assert.Equal(t, "No effect!", outcome.FeedbackMessage)
	assert.Equal(t, uint16(9), state.PartyState.Inventory.Gold.Get())
	assert.Zero(t, outcome.ElapsedMinutes)

	dateAfter := state.DateTime
	_, err = state.CommitWishingWellWish(WishingWellCommitRequest{
		WellPosition: wellPosition, Wish: "nothing", ExpectedFingerprint: review.Fingerprint,
	})
	require.ErrorIs(t, err, ErrWishingWellRequestStale)
	assert.Equal(t, uint16(9), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, dateAfter, state.DateTime)
}

func TestWishingWellHorseWishOutsideOriginalLocationsConsumesCoinWithoutHorse(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.IolosHut).
		WithPlayerAt(8, 3).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(10)
	wellPosition := findWishingWellForTest(t, state)

	review, err := state.ReviewWishingWell(wellPosition)
	require.NoError(t, err)
	outcome, err := state.CommitWishingWellWish(WishingWellCommitRequest{
		WellPosition: wellPosition, Wish: "horse", ExpectedFingerprint: review.Fingerprint,
	})

	require.NoError(t, err)
	assert.Equal(t, WishingWellNoEffect, outcome.Effect)
	assert.Equal(t, "No effect!", outcome.FeedbackMessage)
	assert.Equal(t, uint16(9), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, references.NoPartyVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)
	assert.Zero(t, outcome.ElapsedMinutes)
}

func TestWishingWellHorseCommitBoardsDirectlyWithoutMapUnitsOrExtraTime(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Paws).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(10)
	units := state.CurrentNPCAIController.GetNpcs()
	units.ClearAllMapUnits()
	for i := 0; units.HasCapacity(); i++ {
		vehicle := map_units.NewNPCFriendlyVehiceNewRef(
			references.HorseVehicle,
			references.Position{X: references.Coordinate(i), Y: 1},
			state.MapState.PlayerLocation.Floor,
		)
		require.NotNil(t, units.InsertVehicle(*vehicle))
	}
	unitCountBefore := len(*units)
	wellPosition := findWishingWellForTest(t, state)
	review, err := state.ReviewWishingWell(wellPosition)
	require.NoError(t, err)

	outcome, err := state.CommitWishingWellWish(WishingWellCommitRequest{
		WellPosition: wellPosition, Wish: "horse", ExpectedFingerprint: review.Fingerprint,
	})
	require.NoError(t, err)
	assert.Equal(t, WishingWellHorseEffect, outcome.Effect)
	assert.Equal(t, uint16(9), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, references.HorseVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)
	assert.Len(t, *units, unitCountBefore)
	assert.Zero(t, outcome.ElapsedMinutes)
	assert.Equal(t, "A horse appears!", outcome.AppearanceMessage)
}

func TestWishingWellAliasesMatchCompleteCanonicalTextInsideResponse(t *testing.T) {
	tests := []struct {
		name string
		wish string
	}{
		{name: "Corvette exact", wish: "Corvette"},
		{name: "Ferrari contained", wish: "a Ferrari"},
		{name: "Lamborghini contained at max input", wish: "xLamborghini"},
		{name: "Lotus contained", wish: "Lotus please"},
		{name: "Porsche exact", wish: "Porsche"},
		{name: "Horse contained", wish: "a horse now"},
		{name: "Mustang Redux extension", wish: "Mustang"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Paws).
				WithPlayerAt(10, 10).
				WithSystemCallbacks().
				Build()
			state.PartyState.Inventory.Gold.Set(2)
			wellPosition := findWishingWellForTest(t, state)
			review, err := state.ReviewWishingWell(wellPosition)
			require.NoError(t, err)

			outcome, err := state.CommitWishingWellWish(WishingWellCommitRequest{
				WellPosition: wellPosition, Wish: test.wish, ExpectedFingerprint: review.Fingerprint,
			})
			require.NoError(t, err)
			assert.Equal(t, WishingWellHorseEffect, outcome.Effect)
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Gold.Get())
		})
	}
}

func TestWishingWellSubmittedFailuresConsumeCoinAndPreserveCurrentVehicle(t *testing.T) {
	tests := []struct {
		name string
		wish string
	}{
		{name: "empty", wish: ""},
		{name: "wrong", wish: "dragon"},
		{name: "alias fragment", wish: "hors"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.IolosHut).
				WithPlayerAt(8, 3).
				WithSystemCallbacks().
				Build()
			state.PartyState.Inventory.Gold.Set(2)
			wellPosition := findWishingWellForTest(t, state)
			review, err := state.ReviewWishingWell(wellPosition)
			require.NoError(t, err)

			outcome, err := state.CommitWishingWellWish(WishingWellCommitRequest{
				WellPosition: wellPosition, Wish: test.wish, ExpectedFingerprint: review.Fingerprint,
			})
			require.NoError(t, err)
			assert.Equal(t, WishingWellNoEffect, outcome.Effect)
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Gold.Get())
		})
	}

	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Paws).
		WithPlayerAt(10, 10).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Gold.Set(2)
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.CarpetVehicle,
		state.MapState.PlayerLocation.Position,
		state.MapState.PlayerLocation.Floor,
	)
	wellPosition := findWishingWellForTest(t, state)
	review, err := state.ReviewWishingWell(wellPosition)
	require.NoError(t, err)
	outcome, err := state.CommitWishingWellWish(WishingWellCommitRequest{
		WellPosition: wellPosition, Wish: "horse", ExpectedFingerprint: review.Fingerprint,
	})
	require.NoError(t, err)
	assert.Equal(t, WishingWellNoEffect, outcome.Effect)
	assert.Equal(t, references.CarpetVehicle, state.PartyVehicle.GetVehicleDetails().VehicleType)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Gold.Get())
}

func TestShrineQuestCompletionRevalidatesAndCommitsRewardsOnce(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	enterShrineServiceTestContext(state, 6)
	state.SetShrineQuestCompleted(6, true)
	state.SetShrineQuestActive(6, true)
	avatar := state.PartyState.GetAvatar()
	avatar.Strength = 15
	avatar.Dexterity = 15
	avatar.Intelligence = 15
	state.PartyState.Karma.Value = 50
	state.PartyState.Inventory.Gold.Set(100)

	review, err := state.ReviewShrineMeditation(6)
	require.NoError(t, err)
	state.PartyState.Inventory.Gold.Set(99)
	_, err = state.CommitShrineQuestCompletion(6, review.Fingerprint)
	require.ErrorIs(t, err, ErrShrineServiceStale)
	assert.True(t, state.IsShrineQuestActive(6))
	assert.Equal(t, byte(15), avatar.Strength)
	assert.Equal(t, 50, state.PartyState.Karma.Value)

	review, err = state.ReviewShrineMeditation(6)
	require.NoError(t, err)
	outcome, err := state.CommitShrineQuestCompletion(6, review.Fingerprint)
	require.NoError(t, err)
	assert.False(t, state.IsShrineQuestActive(6))
	assert.Equal(t, byte(16), avatar.Strength)
	assert.Equal(t, byte(16), avatar.Dexterity)
	assert.Equal(t, byte(16), avatar.Intelligence)
	assert.Equal(t, 53, state.PartyState.Karma.Value)
	assert.Equal(t, []string{"Strength +1", "Dexterity +1", "Intelligence +1"}, outcome.StatBonuses)

	_, err = state.CommitShrineQuestCompletion(6, review.Fingerprint)
	require.ErrorIs(t, err, ErrShrineServiceStale)
	assert.Equal(t, byte(16), avatar.Strength)
	assert.Equal(t, 53, state.PartyState.Karma.Value)
}

func TestShrineQuestCompletionCapsAvatarStatsAndLeavesCompanionsAlone(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	enterShrineServiceTestContext(state, 6)
	state.SetShrineQuestCompleted(6, true)
	state.SetShrineQuestActive(6, true)
	avatar := state.PartyState.GetAvatar()
	avatar.Strength, avatar.Dexterity, avatar.Intelligence = MaxStatValue, MaxStatValue-1, 12
	companion := state.PartyState.GetCharacter(1)
	require.NotNil(t, companion)
	companionStats := [3]byte{companion.Strength, companion.Dexterity, companion.Intelligence}

	review, err := state.ReviewShrineMeditation(6)
	require.NoError(t, err)
	_, err = state.CommitShrineQuestCompletion(6, review.Fingerprint)
	require.NoError(t, err)
	assert.Equal(t, [3]byte{MaxStatValue, MaxStatValue, 13}, [3]byte{avatar.Strength, avatar.Dexterity, avatar.Intelligence})
	assert.Equal(t, companionStats, [3]byte{companion.Strength, companion.Dexterity, companion.Intelligence})
}

// Completion adds 3 karma, Humility another 3, each capped at 99. ref:KARMA-0009
func TestShrineQuestCompletionKarmaAwardIsCappedAt99(t *testing.T) {
	for _, test := range []struct{ shrine, before, after int }{
		{shrine: 6, before: 98, after: 99},
		{shrine: 7, before: 50, after: 56},
		{shrine: 7, before: 98, after: 99},
	} {
		state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
		enterShrineServiceTestContext(state, test.shrine)
		state.SetShrineQuestCompleted(test.shrine, true)
		state.SetShrineQuestActive(test.shrine, true)
		state.PartyState.Karma.Value = test.before

		review, err := state.ReviewShrineMeditation(test.shrine)
		require.NoError(t, err)
		_, err = state.CommitShrineQuestCompletion(test.shrine, review.Fingerprint)
		require.NoError(t, err)
		assert.Equal(t, test.after, state.PartyState.Karma.Value, "shrine %d from karma %d", test.shrine, test.before)
	}
}

func TestShrineQuestAssignmentRevalidatesAndCommitsOnce(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	enterShrineServiceTestContext(state, 2)
	state.PartyState.Karma.Value = 50

	review, err := state.ReviewShrineMeditation(2)
	require.NoError(t, err)
	state.PartyState.Karma.Value = 51
	_, err = state.CommitShrineQuestAssignment(2, review.Fingerprint)
	require.ErrorIs(t, err, ErrShrineServiceStale)
	assert.False(t, state.IsShrineQuestActive(2))

	review, err = state.ReviewShrineMeditation(2)
	require.NoError(t, err)
	outcome, err := state.CommitShrineQuestAssignment(2, review.Fingerprint)
	require.NoError(t, err)
	assert.True(t, state.IsShrineQuestActive(2))
	assert.True(t, outcome.Assigned)
	assert.Equal(t, "The Altar speaks and a Quest is ordained!", outcome.PrimaryMessage)

	_, err = state.CommitShrineQuestAssignment(2, review.Fingerprint)
	require.ErrorIs(t, err, ErrShrineServiceStale)
	assert.True(t, state.IsShrineQuestActive(2))
}

func TestFinishShrineVisitRestoresWorldAndOwnsCalendarCommit(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(12, 13).
		WithSystemCallbacks().
		Build()
	original := state.MapState.PlayerLocation
	state.ClearDurationEffect()
	enterShrineServiceTestContext(state, 0)
	dateBefore := state.DateTime

	outcome, err := state.FinishShrineVisit()
	require.NoError(t, err)
	assert.Equal(t, original, state.MapState.PlayerLocation)
	assert.Equal(t, ShrineTimeCostMinutes, outcome.ElapsedMinutes)
	assert.Equal(t, dateBefore, outcome.DateTimeBefore)
	assert.Equal(t, state.DateTime, outcome.DateTimeAfter)

	locationAfter := state.MapState.PlayerLocation
	dateAfter := state.DateTime
	_, err = state.FinishShrineVisit()
	require.ErrorIs(t, err, ErrShrineServiceInvalid)
	assert.Equal(t, locationAfter, state.MapState.PlayerLocation)
	assert.Equal(t, dateAfter, state.DateTime)
}

func TestShrineOfferingAndCodexCommitsRejectStaleReviewsAtomically(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	enterShrineServiceTestContext(state, 0)
	state.SetShrineQuestCompleted(0, true)
	state.SetShrineQuestActive(0, false)
	state.PartyState.Inventory.Gold.Set(200)
	state.PartyState.Karma.Value = 50
	_, err := state.ReviewShrineOffering(0, 0)
	require.ErrorIs(t, err, ErrShrineServiceInvalid)
	assert.Equal(t, uint16(200), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, 50, state.PartyState.Karma.Value)

	unaffordable, err := state.ReviewShrineOffering(0, 3)
	require.NoError(t, err)
	assert.False(t, unaffordable.Affordable)
	_, err = state.CommitShrineOffering(unaffordable)
	require.ErrorIs(t, err, ErrShrineServiceInvalid)
	assert.Equal(t, uint16(200), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, 50, state.PartyState.Karma.Value)

	state.PartyState.Inventory.Gold.Set(500)
	offering, err := state.ReviewShrineOffering(0, 3)
	require.NoError(t, err)
	state.PartyState.Inventory.Gold.Set(499)
	_, err = state.CommitShrineOffering(offering)
	require.ErrorIs(t, err, ErrShrineServiceStale)
	assert.Equal(t, uint16(499), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, 50, state.PartyState.Karma.Value)

	offering, err = state.ReviewShrineOffering(0, 3)
	require.NoError(t, err)
	offerOutcome, err := state.CommitShrineOffering(offering)
	require.NoError(t, err)
	assert.Equal(t, uint16(199), state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, 53, state.PartyState.Karma.Value)
	assert.Equal(t, "ALAKAZAM!", offerOutcome.Message)

	state.MapState.PlayerLocation.Location = references.CodexLocation
	state.ShrineScene = shrine.NewShrineScene(-1, state.MapState.PlayerLocation)
	state.SetShrineQuestActive(1, true)
	state.SetShrineQuestCompleted(1, false)
	codexReview, err := state.ReviewCodexQuest()
	require.NoError(t, err)
	state.SetShrineQuestActive(2, true)
	_, err = state.CommitCodexQuestAnswer(codexReview)
	require.ErrorIs(t, err, ErrCodexServiceStale)
	assert.False(t, state.IsShrineQuestCompleted(1))

	codexReview, err = state.ReviewCodexQuest()
	require.NoError(t, err)
	codexOutcome, err := state.CommitCodexQuestAnswer(codexReview)
	require.NoError(t, err)
	assert.True(t, state.IsShrineQuestCompleted(codexOutcome.QuestIndex))
}

func TestDeathResurrectionRejectsStaleReviewAndCommitsCompleteStateOnce(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	markActivePartyDeadForTest(state)
	state.PartyState.Karma.Value = 50
	state.PartyState.Inventory.Provisions.Food.Set(0)
	state.DateTime.Hour = 19
	state.DateTime.Minute = 37

	review, err := state.ReviewDeathResurrection()
	require.NoError(t, err)
	avatar := state.PartyState.GetAvatar()
	avatar.Exp++
	locationBefore := state.MapState.PlayerLocation
	dateBefore := state.DateTime
	_, err = state.CommitDeathResurrection(review.Fingerprint)
	require.ErrorIs(t, err, ErrDeathResurrectionStale)
	assert.Equal(t, party_state.Dead, avatar.GetStatus())
	assert.Equal(t, uint16(0), avatar.GetCurrentHp())
	assert.Equal(t, locationBefore, state.MapState.PlayerLocation)
	assert.Equal(t, dateBefore, state.DateTime)
	assert.Equal(t, 50, state.PartyState.Karma.Value)

	review, err = state.ReviewDeathResurrection()
	require.NoError(t, err)
	expectedDate := state.DateTime
	for expectedDate.Hour != ResurrectionHour {
		expectedDate.Advance(resurrectionTimeStepMinutes)
	}
	outcome, err := state.CommitDeathResurrection(review.Fingerprint)
	require.NoError(t, err)
	require.NotEmpty(t, outcome.Members)
	assert.Equal(t, references.LordBritishsCastle, state.MapState.PlayerLocation.Location)
	assert.Equal(t, expectedDate, state.DateTime)
	assert.Equal(t, expectedDate, outcome.DateTimeAfter)
	assert.Equal(t, MinimumKarmaAfterDeath, state.PartyState.Karma.Value)
	assert.Equal(t, uint16(MinimumFoodAfterDeath), state.PartyState.Inventory.Provisions.Food.Get())
	for _, member := range outcome.Members {
		character := state.PartyState.GetCharacter(member.Slot)
		assert.Equal(t, party_state.Good, character.GetStatus())
		assert.Equal(t, character.GetMaxHp(), character.GetCurrentHp())
	}

	dateAfter := state.DateTime
	_, err = state.CommitDeathResurrection(review.Fingerprint)
	require.ErrorIs(t, err, ErrDeathResurrectionInvalid)
	assert.Equal(t, dateAfter, state.DateTime)
}

func TestStonegateTrapRejectsStaleFloorAndCommitsOneAtomicWipe(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Stonegate).
		WithPlayerAt(15, 16).
		WithSystemCallbacks().
		Build()
	require.True(t, state.GetCurrentAvatarGroundTile().Is(indexes.BrickFloorHole))

	review, err := state.ReviewStonegateTrap()
	require.NoError(t, err)
	review.Members[0].Name = "detached"
	assert.NotEqual(t, "detached", state.PartyState.GetCharacter(review.Members[0].Slot).GetNameAsString())
	changedPosition := references.Position{X: 0, Y: 0}
	currentMap := state.GetCurrentLayeredMap()
	currentMap.SetTileByLayer(map_state.MapLayer, &changedPosition, indexes.BrickFloor)
	unitCountBefore := len(*state.CurrentNPCAIController.GetNpcs())
	_, err = state.CommitStonegateTrap(review.Fingerprint)
	require.ErrorIs(t, err, ErrStonegateTrapStale)
	assert.False(t, state.PartyState.IsAllPartyDead())
	assert.Len(t, *state.CurrentNPCAIController.GetNpcs(), unitCountBefore)

	itemPosition := references.Position{X: 1, Y: 1}
	require.NoError(t, state.ItemStacksMap.Push(&itemPosition, &references.ItemStack{
		Items: references.ItemStacks{{Item: references.Gold, Quantity: 5}},
	}))
	review, err = state.ReviewStonegateTrap()
	require.NoError(t, err)
	extraItemPosition := references.Position{X: 2, Y: 2}
	require.NoError(t, state.ItemStacksMap.Push(&extraItemPosition, &references.ItemStack{
		Items: references.ItemStacks{{Item: references.Torches, Quantity: 1}},
	}))
	_, err = state.CommitStonegateTrap(review.Fingerprint)
	require.ErrorIs(t, err, ErrStonegateTrapStale)
	assert.False(t, state.PartyState.IsAllPartyDead())
	assert.Len(t, state.ItemStacksMap.Entries(), 2)

	review, err = state.ReviewStonegateTrap()
	require.NoError(t, err)
	outcome, err := state.CommitStonegateTrap(review.Fingerprint)
	require.NoError(t, err)
	assert.True(t, state.PartyState.IsAllPartyDead())
	assert.Empty(t, *state.CurrentNPCAIController.GetNpcs())
	assert.Empty(t, state.ItemStacksMap.Entries())
	assert.Equal(t, 2, outcome.ClearedItemStacks)
	assert.Len(t, outcome.KilledMembers, len(review.Members))
	for y := range 32 {
		for x := range 32 {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			assert.True(t, currentMap.GetTileByLayer(map_state.MapLayer, &position).Is(indexes.Lava))
		}
	}
}

func findWishingWellForTest(t *testing.T, state *GameState) references.Position {
	t.Helper()
	currentMap := state.GetLayeredMapByCurrentLocation()
	for y := range 64 {
		for x := range 64 {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			if tile := currentMap.GetTopTile(&position); tile != nil && tile.Is(indexes.Well) {
				return position
			}
		}
	}
	t.Fatal("no wishing well found in test map")

	return references.Position{}
}

func enterShrineServiceTestContext(state *GameState, shrineIndex int) {
	original := state.MapState.PlayerLocation
	state.ShrineScene = shrine.NewShrineScene(shrineIndex, original)
	state.MapState.PlayerLocation.Location = references.ShrineLocation
}

func markActivePartyDeadForTest(state *GameState) {
	for slot := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(slot)
		if character == nil || !character.IsInParty() {
			continue
		}
		character.SetHp(0)
		character.SetStatus(party_state.Dead)
	}
}
