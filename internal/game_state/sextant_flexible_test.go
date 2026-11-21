package game_state

import (
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/stretchr/testify/require"
)

func TestFlexibleSextantPurePolicyAllHoursAndMaps(t *testing.T) {
	t.Parallel()
	for location := range 256 {
		for floor := references.FloorNumber(-2); floor <= 4; floor++ {
			for hour := range uint8(25) {
				context := SextantContext{HasSextant: true, Flexible: true, Location: references.Location(location), Floor: floor, Hour: hour}
				want := SextantReady
				world := location == 0 && floor == 0
				settlement := location >= 1 && location <= 32 && location != 25 && floor >= -1 && floor <= 3
				if !world && !settlement {
					want = SextantNotOverworld
				} else if hour == 24 {
					want = SextantDaylight
				}
				require.Equal(t, want, EvaluateSextantEligibility(context), "map %d floor %d hour %d", location, floor, hour)
				context.HasSextant = false
				require.Equal(t, SextantMissing, EvaluateSextantEligibility(context))
			}
		}
	}
}

func TestFlexibleSextantReferenceFloorsCanonicalFixAndReadOnlyProjection(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(71, 93).WithRandomSeed(42).WithSystemCallbacks().Build()
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	callbacks.Reset()
	for location := references.Moonglow; location <= references.SerpentsHold; location++ {
		local := state.GameReferences.LocationReferences.GetLocationReference(location)
		require.NotNil(t, local)
		position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(location)
		for floor := references.FloorNumber(-2); floor <= 4; floor++ {
			validFloor := false
			for _, actual := range local.ListOfFloors {
				validFloor = validFloor || floor == actual
			}
			for hour := range uint8(24) {
				state.DateTime.Hour = hour
				state.MapState.PlayerLocation = references.PlayerLocation{Location: location, Floor: floor, Position: references.Position{X: 3, Y: 7}}
				callbacks.Reset()
				clock, calls := state.DateTime, state.GetRNG().Calls()
				fix, ready := state.CurrentSextantFix()
				want := validFloor && location != references.Ararat
				require.Equal(t, want, ready, "map %d floor %d hour %d", location, floor, hour)
				require.Equal(t, ready, state.CurrentSextantEligibility() == SextantReady)
				item := requireUsableItem(t, state.GetUsableItemsForContext(UsageField), references.Sextant)
				require.Equal(t, ready, item.AvailableNow)
				dossier := state.CurrentAtlasDossier()
				if ready {
					require.Equal(t, references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: position}, fix)
					require.Equal(t, moonstoneJournalSite(fix), dossier.CurrentFix.Location)
					require.Equal(t, "Within this settlement", dossier.FixContext)
				} else {
					require.Equal(t, references.PlayerLocation{}, fix)
					require.Equal(t, journal.SurveyedLocation{}, dossier.CurrentFix)
				}
				require.Equal(t, clock, state.DateTime)
				require.Equal(t, calls, state.GetRNG().Calls())
				require.Empty(t, state.atlasLandmarks)
				require.Empty(t, callbacks.TimeAdvanced())
			}
		}
	}
	require.Empty(t, callbacks.Messages)
	require.Empty(t, callbacks.TimeAdvanced())
}

func TestFlexibleSextantRejectsMalformedCanonicalReferences(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	world := state.GameReferences.LocationReferences.WorldLocations.LargeMapLocationPositions
	original := world[references.Britain]
	t.Cleanup(func() { world[references.Britain] = original })
	for _, invalid := range []references.WorldLocation{
		{Location: references.Britain, Floor: -1, Position: original.Position},
		{Location: references.Ararat, Position: original.Position},
		{Location: references.Britain, Position: references.Position{X: 256, Y: 0}},
	} {
		world[references.Britain] = invalid
		fix, ok := state.CurrentSextantFix()
		require.False(t, ok)
		require.Equal(t, references.PlayerLocation{}, fix)
		require.Equal(t, SextantNotOverworld, state.CurrentSextantEligibility())
		require.False(t, requireUsableItem(t, state.GetUsableItemsForContext(UsageField), references.Sextant).AvailableNow)
		require.False(t, state.useSextant())
	}
	delete(world, references.Britain)
	_, ok := state.CurrentSextantFix()
	require.False(t, ok)
}

func TestFlexibleSextantInteriorUseRequiresEarnedEntryAndMeasuresOnlySelf(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithRandomSeed(42).WithSystemCallbacks().Build()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 0)
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Moonglow)
	state.MapState.PlayerLocation.Position = position
	require.Equal(t, EnterLargeMapTransition, state.ResolveEnterLargeMap())
	runDungeonWorkflow(t, state, "enter_location")
	require.Len(t, state.atlasLandmarks, 1)
	require.Equal(t, journal.SurveyedLocation{}, state.atlasLandmarks[0].Survey)
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 12
	before := append([]journal.AtlasLandmarkObservation(nil), state.atlasLandmarks...)
	clock, calls := state.DateTime, state.GetRNG().Calls()
	callbacks.Reset()
	dossier := state.CurrentAtlasDossier()
	require.Equal(t, "Within Moonglow", dossier.FixContext)
	require.Equal(t, before, state.atlasLandmarks, "setting and map opening never backfill")
	require.True(t, state.useSextant())
	require.Equal(t, []string{"Position:", fmt.Sprintf("%s, %s", encodeSextantCoordinate(position.Y), encodeSextantCoordinate(position.X))}, callbacks.Messages)
	observation := state.atlasLandmarks[0]
	require.True(t, observation.Survey.Measured)
	require.True(t, observation.Survey.Flexible)
	require.Equal(t, observation.Site, observation.Survey.Location)
	require.Equal(t, 12, observation.Survey.RecordedAt.Time.Hour)
	require.Equal(t, before[0].Name, observation.Name)
	require.Equal(t, observation.Survey.RecordedAt, observation.ObservedAt)
	require.Equal(t, before[0].NamedAt, observation.NamedAt, "a reading does not rediscover the name")
	require.NoError(t, journal.ValidateAtlasLandmarks(state.atlasLandmarks))
	require.Equal(t, clock, state.DateTime)
	require.Equal(t, calls, state.GetRNG().Calls())
	require.Empty(t, callbacks.TimeAdvanced())
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = false
	require.Equal(t, observation.Survey, state.CurrentAtlasDossier().Entries[0].Survey)

	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
	state.MapState.PlayerLocation.Location = references.Britain
	require.Equal(t, "Within this settlement", state.CurrentAtlasDossier().FixContext)
	require.True(t, state.useSextant(), "debug-loaded interior can report a fix")
	require.Len(t, state.atlasLandmarks, 1, "but cannot invent acquisition or names")
}

func TestFlexibleSextantAcceptedSourcePolicyAndSurfaceOnlySurveys(t *testing.T) {
	for _, hour := range []uint8{12, 19} {
		t.Run(fmt.Sprintf("hour-%d", hour), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
				WithRandomSeed(42).WithSystemCallbacks().Build()
			state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
			state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
			state.DateTime.Hour = hour
			sample := state.sampleDungeonObservation()
			state.GameConfig.GameplayConfiguration.FlexibleSextantUse = false
			state.DateTime.Hour = 6
			state.observeDungeonEntrance(references.Deceit, state.GetDungeonEntranceCoordinates()[references.Deceit], sample, false)
			survey := state.dungeonJournal[DeceitIndex].Sites[0].Survey
			require.True(t, survey.Measured)
			require.True(t, survey.Flexible, "accepted setting retained even at night")
			require.EqualValues(t, hour, survey.RecordedAt.Time.Hour)

			position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Moonglow)
			state.observeAtlasLandmark(position, "towne", "MOONGLOW", journal.AtlasSourceEnter, sample)
			require.True(t, state.atlasLandmarks[0].Survey.Flexible)
			require.True(t, state.atlasLandmarks[0].Survey.Measured)

			state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
			state.DateTime.Hour = hour
			state.observeMoonstone(0, moonstoneJournalSite(state.MapState.PlayerLocation), journal.MoonstoneSourceBurial)
			require.True(t, state.moonstoneJournal[0].Survey.Flexible)
			require.True(t, state.moonstoneJournal[0].Survey.Measured)
			for _, location := range []references.PlayerLocation{
				{Location: references.Moonglow, Floor: 0, Position: references.Position{X: 3, Y: 7}},
				{Location: references.BritanniaUnderworld, Floor: -1},
			} {
				state.MapState.PlayerLocation = location
				state.observeMoonstone(0, moonstoneJournalSite(location), journal.MoonstoneSourceBurial)
				require.True(t, state.moonstoneJournal[0].SiteKnown)
				require.Equal(t, journal.SurveyedLocation{}, state.moonstoneJournal[0].Survey)
			}
			state.MapState.PlayerLocation = references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: -1}
			state.observeDungeonEntrance(references.Deceit, state.GetDungeonEntranceCoordinates()[references.Deceit], state.sampleDungeonObservation(), false)
			require.Equal(t, journal.SurveyedLocation{}, state.dungeonJournal[DeceitIndex].Sites[1].Survey)
		})
	}
}

func TestFlexibleSextantDaylightActualEntryCapturesExterior(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithRandomSeed(42).WithSystemCallbacks().Build()
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 12
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Moonglow)
	state.MapState.PlayerLocation.Position = position
	require.Equal(t, EnterLargeMapTransition, state.ResolveEnterLargeMap())
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = false
	runDungeonWorkflow(t, state, "enter_location")
	require.Equal(t, references.Moonglow, state.MapState.PlayerLocation.Location)
	require.Len(t, state.atlasLandmarks, 1)
	survey := state.atlasLandmarks[0].Survey
	require.True(t, survey.Measured)
	require.True(t, survey.Flexible)
	require.Equal(t, journal.SiteLocation{Map: references.BritanniaUnderworld, X: int(position.X), Y: int(position.Y)}, survey.Location)
	require.Equal(t, 12, survey.RecordedAt.Time.Hour)
}

func TestFlexibleSextantStatusReasonsAndOriginalAstronomy(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithRandomSeed(42).WithSystemCallbacks().Build()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	for _, test := range []struct {
		location references.Location
		floor    references.FloorNumber
		hour     uint8
		reason   string
	}{
		{references.Britain, 0, 19, "Use on the world map."},
		{references.BritanniaUnderworld, 0, 12, "Only at night!"},
		{references.BritanniaUnderworld, -1, 19, "Unavailable underground."},
		{references.Deceit, 0, 19, "Unavailable underground."},
		{references.Ararat, 0, 19, "Unavailable underground."},
	} {
		state.MapState.PlayerLocation.Location, state.MapState.PlayerLocation.Floor = test.location, test.floor
		state.DateTime.Hour = test.hour
		require.Equal(t, test.reason, state.CurrentAtlasDossier().FixUnavailable)
		state.PartyState.Inventory.QuestItems.Set(references.Sextant, 0)
		require.Equal(t, "No sextant carried.", state.CurrentAtlasDossier().FixUnavailable)
		state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	}
	state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
	for hour := range uint8(24) {
		state.DateTime.Hour = hour
		require.Equal(t, hour < 6 || hour >= 19, state.IsUseAstronomyNight(), "Spyglass still requires night")
		state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
		state.MapState.PlayerLocation.Floor = 0
		require.Equal(t, SextantReady, state.CurrentSextantEligibility())
		state.MapState.PlayerLocation.Floor = -1
		require.False(t, state.IsSpyglassLocationValid())
	}
}

func TestFlexibleSextantSurveyNativeSavePolicy(t *testing.T) {
	for _, hour := range []uint8{12, 19} {
		t.Run(fmt.Sprintf("hour-%d", hour), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
				WithRandomSeed(42).WithSystemCallbacks().Build()
			state.GameConfig.GameplayConfiguration.FlexibleSextantUse = true
			state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
			state.DateTime.Hour = hour
			position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Moonglow)
			state.observeAtlasLandmark(position, "towne", "MOONGLOW", journal.AtlasSourceEnter, state.sampleDungeonObservation())
			state.observeDungeonEntrance(references.Deceit, state.GetDungeonEntranceCoordinates()[references.Deceit], state.sampleDungeonObservation(), false)
			state.GetMoonstoneSystem().BuryMoonstone(0, 50, 50, 0, 0)
			state.observeMoonstone(0, moonstoneJournalSite(state.MapState.PlayerLocation), journal.MoonstoneSourceBurial)
			state.GameConfig.GameplayConfiguration.FlexibleSextantUse = false
			state.ProcessWindThroughElapsed(state.ElapsedMs)
			restored := restoreAtlasSnapshot(t, state)
			require.Equal(t, state.atlasLandmarks, restored.atlasLandmarks)
			require.Equal(t, state.dungeonJournal, restored.dungeonJournal)
			require.Equal(t, state.moonstoneJournal, restored.moonstoneJournal)
			require.True(t, restored.atlasLandmarks[0].Survey.Flexible)
			require.True(t, restored.dungeonJournal[DeceitIndex].Sites[0].Survey.Flexible)
			require.True(t, restored.moonstoneJournal[0].Survey.Flexible)
		})
	}
}
