package game_state

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/require"
)

func TestAtlasSuppliedBaselineAndSettlementFilter(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(42).Build()
	dossier := state.CurrentAtlasDossier()
	require.Len(t, dossier.Entries, 34)
	for _, entry := range dossier.Entries {
		require.True(t, entry.Charted)
		require.False(t, entry.Discovered, "supplied symbols and names are not personal evidence")
		require.False(t, entry.Survey.Measured)
		require.False(t, entry.ObservedAt.Known)
		require.False(t, entry.NamedAt.Known)
		require.Equal(t, entry.Key == "chart-12" || entry.Key == "chart-25" || entry.Kind == "moonstone", entry.NameKnown)
		require.NotContains(t, entry.Name, "Charted")
		require.NotEqual(t, "hut", entry.Kind)
		require.NotEqual(t, "keep", entry.Kind)
		require.NotContains(t, entry.Name, "Stonegate")
		for _, r := range entry.Name {
			require.Less(t, r, rune(128), "atlas labels must use supported ASCII punctuation")
		}
	}
	for _, key := range []string{"chart-09", "chart-10", "chart-11"} {
		require.Equal(t, "stronghold", atlasEntryByKey(t, dossier, key).Kind)
	}
	for _, key := range []string{"chart-12", "chart-13"} {
		require.Equal(t, "castle", atlasEntryByKey(t, dossier, key).Kind)
	}
	buccaneers := atlasEntryByKey(t, dossier, "chart-34")
	require.Equal(t, buccaneers, dossier.Entries[8], "stable category sorting groups the ninth towne with the other eight")
	for i := 1; i < len(dossier.Entries); i++ {
		require.LessOrEqual(t, dossier.Entries[i-1].Category, dossier.Entries[i].Category)
	}
	require.Equal(t, "Towne 9", buccaneers.Name)
	require.Equal(t, "towne", buccaneers.Kind)
	require.Equal(t, journal.AtlasSettlements, buccaneers.Category)
	position, plottable := buccaneers.PlotPosition()
	require.False(t, plottable)
	require.Zero(t, position)
	filters := journal.DefaultAtlasFilters()
	filters.Places = journal.AtlasTownes
	settlements := journal.FilterAtlasEntries(dossier.Entries, filters)
	require.Len(t, settlements, 9)
	require.Contains(t, settlements, buccaneers)
	filters.Places = journal.AtlasDungeonPlaces
	require.Empty(t, journal.FilterAtlasEntries(dossier.Entries, filters), "unknown dungeons stay absent")
	for filter, count := range []int{34, 9, 3, 2, 8, 4, 0, 8} {
		selected := journal.AtlasPlaceFilter(filter)
		require.Len(t, journal.FilterAtlasEntries(dossier.Entries, journal.AtlasFilters{Places: selected}), count, selected.String())
	}
	ordinary := dossier
	ordinary.Entries = nil
	for _, entry := range dossier.Entries {
		point, plotted := entry.PlotPosition()
		require.False(t, plotted, "even supplied special sites require measurement")
		require.Zero(t, point)
		if entry.Kind == "moonstone" {
			require.Equal(t, references.ChartedMoonstoneOrigins()[entry.Phase].Label, entry.OriginReference)
		} else {
			ordinary.Entries = append(ordinary.Entries, entry)
		}
	}
	encoded, err := json.Marshal(ordinary)
	require.NoError(t, err)
	for _, hidden := range []string{"Moonglow", "Britain", "Jhelom", "Yew", "Minoc", "Trinsic", "Skara", "Magincia",
		"Lycaeum", "Empath", "Hold", "Blackthorn", "Fogsbane", "Stormcrow", "Greyhaven", "Waveguide", "Buccaneer", "Honesty", "Stonegate", "Vraal"} {
		require.NotContains(t, string(encoded), hidden, "no proper names in any public field before introduction")
	}
	require.Empty(t, state.atlasLandmarks)
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	require.Equal(t, "Only at night!", state.CurrentAtlasDossier().FixUnavailable)
}

func TestAtlasProjectionPrivacyAndIndependentSurvey(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().WithRandomSeed(42).Build()
	state.DateTime.Hour = 12
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 0)
	before, calls := state.DateTime, state.GetRNG().Calls()
	fresh := state.CurrentAtlasDossier()
	require.Len(t, fresh.Entries, 34)
	require.Empty(t, state.atlasLandmarks)
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.SinVraalsHut)
	state.observeAtlasLook(position, indexes.Hut)
	public := state.CurrentAtlasDossier()
	require.Len(t, public.Entries, 35)
	key := fmt.Sprintf("observed-%d", references.SinVraalsHut)
	entry := atlasEntryByKey(t, public, key)
	require.True(t, entry.Discovered, "a successful unnamed Look is personal discovery")
	require.Equal(t, "Unnamed hut", entry.Name)
	require.False(t, entry.NameKnown)
	require.False(t, entry.Survey.Measured)
	_, plottable := entry.PlotPosition()
	require.False(t, plottable)
	encoded, err := json.Marshal(entry)
	require.NoError(t, err)
	require.NotContains(t, string(encoded), "Vraal")
	require.NotContains(t, string(encoded), "ChartPosition")
	require.Contains(t, journal.FilterAtlasEntries(public.Entries, journal.DefaultAtlasFilters()), entry)
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	require.False(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), key).Survey.Measured, "tool acquisition cannot backfill")
	state.observeAtlasLook(position, indexes.Hut)
	survey := state.atlasLandmarks[0].Survey
	require.True(t, survey.Measured)
	require.False(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), key).NameKnown, "surveying never learns a name")
	require.EqualValues(t, position.X, survey.Location.X)
	state.DateTime.Hour = 12
	state.observeAtlasLook(position, indexes.Hut)
	require.Equal(t, survey, state.atlasLandmarks[0].Survey)
	state.observeAtlasLandmark(position, "hut", "Sin'Vraal's Hut", journal.AtlasSourceEnter, state.sampleDungeonObservation())
	require.Equal(t, "Sin'Vraal's Hut", atlasEntryByKey(t, state.CurrentAtlasDossier(), key).Name)
	require.True(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), key).NameKnown)
	state.observeAtlasLook(position, indexes.Hut)
	require.Equal(t, "Sin'Vraal's Hut", state.atlasLandmarks[0].Name)
	for index := range public.Entries {
		public.Entries[index].Name = "mutated"
	}
	require.NotEqual(t, "mutated", state.atlasLandmarks[0].Name)
	require.Equal(t, before, state.DateTime)
	require.Equal(t, calls, state.GetRNG().Calls())
	require.NoError(t, journal.ValidateAtlasLandmarks(state.atlasLandmarks))
}

func TestAtlasMoonOriginNeverFollowsPrivatePlacement(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	origin := atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-27")
	// Private recognition never supplies coordinates or changes chart metadata.
	for _, p := range []journal.SiteLocation{atlasMoonOriginSite(1), {Map: references.BritanniaUnderworld, X: 71, Y: 91}} {
		state.moonstoneJournal[1] = journal.MoonstoneObservation{Known: true, SiteKnown: true, Site: p, Placed: true}
		got := atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-27")
		require.Equal(t, origin, got)
	}
}

func TestAtlasMoonSurveysUseStableChartKeysForEveryPhase(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(42).Build()
	state.DateTime.Hour = 19
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	baseline := state.CurrentAtlasDossier()
	for phase := range references.ChartedMoonstoneOrigins() {
		state.observeMoonstone(phase, atlasMoonOriginSite(phase), journal.MoonstoneSourceSearch)
	}
	dossier := state.CurrentAtlasDossier()
	require.Len(t, dossier.Entries, 34)
	for phase := range references.ChartedMoonstoneOrigins() {
		key := references.AtlasMoonstoneChartKey(phase)
		entry := atlasEntryByKey(t, dossier, key)
		require.Equal(t, datetime.MoonPhase(phase).String(), entry.Name)
		require.Equal(t, phase, entry.Phase)
		require.True(t, entry.LastObservedDestination)
		require.True(t, entry.Discovered)
		require.Equal(t, atlasMoonOriginSite(phase), entry.Survey.Location)
		require.True(t, entry.Survey.Measured)
	}
	require.Equal(t, atlasEntryByKey(t, baseline, "chart-34"), atlasEntryByKey(t, dossier, "chart-34"))
}

func TestAtlasDisplayNameUsesOnlyLearnedText(t *testing.T) {
	for _, test := range []struct{ learned, displayed string }{
		{"FOGSBANE", "Fogsbane"},
		{"BUCCANEER'S DEN", "Buccaneer's Den"},
		{"NEW MAGINCIA", "New Magincia"},
		{"Sin'Vraal's Hut", "Sin'Vraal's Hut"},
		{"the shrine of Honesty", "the shrine of Honesty"},
		{"the Castle of Lord British!", "the Castle of Lord British!"},
	} {
		t.Run(test.learned, func(t *testing.T) {
			require.Equal(t, test.displayed, atlasDisplayName(test.learned))
		})
	}
}

func TestAtlasNameRequiresGenuineSource(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(42).Build()
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Moonglow)
	state.observeAtlasLandmark(position, "towne", "MOONGLOW", journal.AtlasSourceLook, state.sampleDungeonObservation())
	require.Empty(t, state.atlasLandmarks[0].Name, "even supplied naming text cannot make Look a naming source")
	baseline := atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-01")
	require.False(t, baseline.NameKnown)
	require.Equal(t, "Towne 1", baseline.Name)
	for _, source := range []journal.AtlasObservationSource{"", journal.AtlasSourceLook, journal.AtlasSourceShrineIntroduction} {
		state.atlasLandmarks[0].Name = "MOONGLOW"
		state.atlasLandmarks[0].NameSource = source
		state.atlasLandmarks[0].NamedAt = state.sampleDungeonObservation().at
		require.Equal(t, baseline, atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-01"), "invalid provenance must not escape via name or metadata")
	}
	state.atlasLandmarks[0].NameSource = journal.AtlasSourceEnter
	entry := atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-01")
	require.True(t, entry.NameKnown)
	require.Equal(t, "Moonglow", entry.Name)
	require.Equal(t, "MOONGLOW", state.atlasLandmarks[0].Name)
}

func TestAtlasKindsUseSpecificCategories(t *testing.T) {
	for kind, category := range map[string]journal.AtlasCategory{
		"towne": journal.AtlasSettlements, "village": journal.AtlasSettlements, "hut": journal.AtlasSettlements,
		"keep": journal.AtlasKeeps, "stronghold": journal.AtlasKeeps, "castle": journal.AtlasCastles,
		"shrine": journal.AtlasShrines, "ruins": journal.AtlasShrines, "lighthouse": journal.AtlasLighthouses,
		"dungeon": journal.AtlasDungeons, "moonstone": journal.AtlasMoonstones,
	} {
		require.Equal(t, category, atlasCategory(kind), kind)
	}
}

func TestAtlasEnterSourceCaptureAndFailure(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	state.SystemCallbacks.UI.AtlasUpdated = func(journal.AtlasChange) { t.Fatal("cancelled precommit Enter notified") }
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Fogsbane)
	state.MapState.PlayerLocation.Position = position
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	state.StartEnterLocationWorkflow(state.GameReferences.LocationReferences.GetLocationReference(references.Fogsbane))
	require.Empty(t, state.atlasLandmarks, "workflow admission isn't a successful transition")
	state.EventQueueManager.Shutdown()
	require.Empty(t, state.atlasLandmarks, "precommit cancellation earns nothing")
}

func TestAtlasLookUsesSuccessfulTargetAndEnterUsesCapturedExterior(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(42).Build()
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Fogsbane)
	state.MapState.PlayerLocation.Position = position.Down()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour, state.DateTime.Minute = 5, 58
	currentMap := state.GetCurrentLayeredMap()
	currentMap.SetTileByLayer(map_state.MapUnitLayer, &position, indexes.HorseRight)
	require.True(t, state.ActionLookLargeMap(references.Up))
	require.Empty(t, state.atlasLandmarks, "occupying object was described instead of the landmark")
	currentMap.SetTileByLayer(map_state.MapUnitLayer, &position, 0)
	require.True(t, state.ActionLookLargeMap(references.Up))
	require.Len(t, state.atlasLandmarks, 1)
	observed := state.atlasLandmarks[0]
	require.Empty(t, observed.Name, "Look must not import a hidden catalog name")
	require.True(t, observed.Survey.Measured)
	require.Equal(t, 5, observed.Survey.RecordedAt.Time.Hour)
	require.Equal(t, 59, observed.Survey.RecordedAt.Time.Minute)
	require.EqualValues(t, position.X, observed.Survey.Location.X)
	require.EqualValues(t, position.Y, observed.Survey.Location.Y)
	require.EqualValues(t, 6, state.DateTime.Hour)
	state.MapState.PlayerLocation.Position = position
	callbacks.Reset()
	require.Equal(t, EnterLargeMapTransition, state.ResolveEnterLargeMap())
	require.Empty(t, state.atlasLandmarks[0].Name)
	runDungeonWorkflow(t, state, "enter_location")
	require.Equal(t, references.Fogsbane, state.MapState.PlayerLocation.Location)
	require.Equal(t, "FOGSBANE", state.atlasLandmarks[0].Name, "retain the introduction's displayed name")
	require.Equal(t, "Fogsbane", atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-14").Name)
	require.Equal(t, observed.Survey, state.atlasLandmarks[0].Survey, "daylight entry preserves genuine date")
	require.Equal(t, journal.AtlasSourceLook, state.atlasLandmarks[0].SurveySource)
	require.Equal(t, journal.AtlasSourceEnter, state.atlasLandmarks[0].NameSource)
	require.NoError(t, journal.ValidateAtlasLandmarks(state.atlasLandmarks))
}

func TestAtlasShrineNamesOnlyAcceptedIntroductionAndRemembersLocalState(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	position := references.AtlasShrinePositions()[0]
	state.MapState.PlayerLocation.Position = position
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	state.SetShrineRuined(0, true)
	require.Equal(t, EnterLargeMapRejected, state.ResolveEnterLargeMap())
	require.Empty(t, state.atlasLandmarks)
	state.MapState.PlayerLocation.Position = position.Down()
	require.True(t, state.ActionLookLargeMap(references.Up))
	require.Len(t, state.atlasLandmarks, 1)
	require.Equal(t, "ruins", state.atlasLandmarks[0].Kind)
	require.False(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-18").NameKnown)
	require.Empty(t, state.atlasLandmarks[0].Name)
	survey := state.atlasLandmarks[0].Survey
	require.True(t, survey.Measured)
	state.SetShrineRuined(0, false)
	require.Equal(t, "ruins", state.atlasLandmarks[0].Kind, "remote state changes do not rewrite remembered observations")
	state.MapState.PlayerLocation.Position = position
	require.Equal(t, EnterLargeMapInteraction, state.ResolveEnterLargeMap())
	require.Equal(t, "the shrine of Honesty", state.atlasLandmarks[0].Name)
	require.Equal(t, journal.AtlasSourceShrineIntroduction, state.atlasLandmarks[0].NameSource)
	require.True(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-18").NameKnown)
	require.Equal(t, survey, state.atlasLandmarks[0].Survey)
	require.NoError(t, journal.ValidateAtlasLandmarks(state.atlasLandmarks))
}

func TestAtlasMoonSurveyMergeRelocationAndCarrying(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.DateTime.Hour = 19
	origin := atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-27")
	require.Equal(t, "Crescent Waxing", origin.Name)
	state.observeMoonstone(1, atlasMoonOriginSite(1), journal.MoonstoneSourceBurial)
	merged := state.CurrentAtlasDossier()
	require.Len(t, merged.Entries, 34)
	mergedOrigin := atlasEntryByKey(t, merged, "chart-27")
	require.True(t, mergedOrigin.LastObservedDestination)
	require.True(t, mergedOrigin.Survey.Measured)
	require.True(t, mergedOrigin.ObservedAt.Known, "merging must retain the destination's observation date")
	require.Equal(t, state.journalMoonstones()[1].ObservedAt, mergedOrigin.ObservedAt)
	moved := journal.SiteLocation{Map: references.BritanniaUnderworld, X: 71, Y: 91}
	state.observeMoonstone(1, moved, journal.MoonstoneSourceBurial)
	separate := state.CurrentAtlasDossier()
	require.Len(t, separate.Entries, 34, "one stable row per phase")
	separateOrigin := atlasEntryByKey(t, separate, "chart-27")
	require.False(t, separateOrigin.Charted)
	lastSeen := separateOrigin
	require.Equal(t, moved, lastSeen.Survey.Location)
	require.Equal(t, "Crescent Waxing (last seen)", lastSeen.Name)
	state.PartyState.Inventory.Moonstones.Set(references.CrescentWaxing, 1)
	carried := state.CurrentAtlasDossier()
	require.Len(t, carried.Entries, 34)
	carriedOrigin := atlasEntryByKey(t, carried, "chart-27")
	require.False(t, carriedOrigin.Charted)
	require.Zero(t, carriedOrigin.HistoricalSurvey)
	require.Zero(t, carriedOrigin.Survey)
	_, plotted := carriedOrigin.PlotPosition()
	require.False(t, plotted)
	require.False(t, carriedOrigin.LastObservedDestination)
	require.True(t, carriedOrigin.Discovered, "possession does not erase actual personal evidence")
	state.PartyState.Inventory.Moonstones.Set(references.CrescentWaxing, 0)
	state.DateTime.Hour = 12
	state.observeMoonstone(1, atlasMoonOriginSite(1), journal.MoonstoneSourceBurial)
	unmeasured := state.CurrentAtlasDossier()
	require.Len(t, unmeasured.Entries, 34)
	unmeasuredOrigin := atlasEntryByKey(t, unmeasured, "chart-27")
	require.False(t, unmeasuredOrigin.LastObservedDestination, "private equality must not restore the origin destination role")
	require.Zero(t, unmeasuredOrigin.HistoricalSurvey)
	require.False(t, unmeasuredOrigin.Charted, "returning to the same coordinates never resurrects the origin")
	_, plotted = unmeasuredOrigin.PlotPosition()
	require.False(t, plotted)
}

func atlasEntryByKey(t *testing.T, dossier journal.AtlasDossier, key string) journal.AtlasEntry {
	t.Helper()
	for _, entry := range dossier.Entries {
		if entry.Key == key {
			return entry
		}
	}
	t.Fatalf("atlas entry %q is absent", key)
	return journal.AtlasEntry{}
}

func atlasMoonOriginSite(phase int) journal.SiteLocation {
	return moonstoneJournalSite(references.ChartedMoonstoneOrigins()[phase].Location)
}

func TestAtlasDiscoveryRequiresPersonalEvidence(t *testing.T) {
	state, _ := newMoonstoneJournalTestState(t)
	position, _ := state.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(references.Moonglow)
	state.atlasLandmarks = []journal.AtlasLandmarkObservation{{SiteKey: uint8(references.Moonglow), Kind: "towne",
		Site: journal.SiteLocation{X: int(position.X), Y: int(position.Y)}, Name: "MOONGLOW", NameSource: journal.AtlasSourceEnter}}
	entry := atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-01")
	require.True(t, entry.NameKnown)
	require.False(t, entry.Discovered, "naming alone does not replace an actual observation record")
	state.atlasLandmarks[0].Source = journal.AtlasSourceLook
	require.True(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-01").Discovered, "undated historical Look still proves discovery")
	state.atlasLandmarks[0].Source = ""
	state.atlasLandmarks[0].ObservedAt = state.sampleDungeonObservation().at
	require.True(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), "chart-01").Discovered)
	state.atlasLandmarks = nil

	require.True(t, state.DiscoverWordOfPowerKnowledge("destard", DialogueKnowledgeSource("Goeth")))
	dungeon := atlasEntryByKey(t, state.CurrentAtlasDossier(), "dungeon-Destard")
	require.True(t, dungeon.NameKnown)
	require.False(t, dungeon.Discovered, "a learned Word/name is not a surface visit")
	state.dungeonJournal[2] = journal.DungeonObservation{Known: true}
	state.dungeonJournal[2].Sites[journal.DungeonUnderworldSite] = journal.DungeonSiteObservation{Seen: true}
	require.False(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), "dungeon-Destard").Discovered, "Underworld observation is not a surface visit")
	state.dungeonJournal[2].Sites[journal.DungeonSurfaceSite] = journal.DungeonSiteObservation{Seen: true}
	require.True(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), "dungeon-Destard").Discovered, "undated migrated surface evidence remains personal")

	state.moonstoneJournal[0] = journal.MoonstoneObservation{Known: true, FirstSource: journal.MoonstoneSourcePossession, OriginalSiteRetired: true}
	state.PartyState.Inventory.Moonstones.Set(references.NewMoon, 1)
	moon := atlasEntryByKey(t, state.CurrentAtlasDossier(), references.AtlasMoonstoneChartKey(0))
	require.True(t, moon.NameKnown)
	require.False(t, moon.Discovered, "possession-only migration cannot invent a visit")
	_, plotted := moon.PlotPosition()
	require.False(t, plotted)
	state.observeMoonstone(0, state.localMoonstoneSite(position), journal.MoonstoneSourcePickup)
	require.True(t, atlasEntryByKey(t, state.CurrentAtlasDossier(), references.AtlasMoonstoneChartKey(0)).Discovered, "runtime successful pickup supplies personal evidence")
}
