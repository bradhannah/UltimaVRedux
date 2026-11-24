package game_state

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func atlasKind(tile indexes.SpriteIndex) string {
	//nolint:exhaustive // Only supported exterior feature artwork is observable.
	switch tile {
	case indexes.Hut:
		return "hut"
	case indexes.Keep:
		return "keep"
	case indexes.Village:
		return "village"
	case indexes.SmallCastle:
		return "towne"
	case indexes.LargeCastle, indexes.EvilCastleEntrance, indexes.CastleBritianEntrace:
		return "castle"
	case indexes.Lighthouse:
		return "lighthouse"
	case indexes.Shrine:
		return "shrine"
	case indexes.BrokenShrine:
		return "ruins"
	default:
		return ""
	}
}

func atlasCategory(kind string) journal.AtlasCategory {
	switch kind {
	case "towne", "village", "hut":
		return journal.AtlasSettlements
	case "keep", "stronghold":
		return journal.AtlasKeeps
	case "castle":
		return journal.AtlasCastles
	case "shrine", "ruins", "guardians":
		return journal.AtlasShrines
	case "lighthouse":
		return journal.AtlasLighthouses
	case "dungeon":
		return journal.AtlasDungeons
	case "moonstone":
		return journal.AtlasMoonstones
	default:
		panic("unsupported atlas kind: " + kind)
	}
}

func atlasNameSourceValid(siteKey uint8, source journal.AtlasObservationSource) bool {
	if siteKey == journal.AtlasGuardiansSiteKey {
		return false // Supplied label only; Look never learns a new name.
	}
	if siteKey >= 101 && siteKey <= 107 {
		return source == journal.AtlasSourceShrineIntroduction
	}

	return source == journal.AtlasSourceEnter
}

func (g *GameState) atlasSiteKey(position references.Position, sample dungeonObservationSample) uint8 {
	if sample.sextant.Location != references.BritanniaUnderworld || sample.sextant.Floor != 0 {
		return 0
	}
	if position == (references.Position{X: codexGuardianX, Y: codexGuardianY}) {
		return journal.AtlasGuardiansSiteKey
	}
	for i, p := range references.AtlasShrinePositions() {
		if p == position {
			return uint8(101 + i)
		}
	}
	for location := references.Moonglow; location <= references.SerpentsHold; location++ {
		if location == references.Ararat {
			continue
		}
		p, floor := g.GameReferences.LocationReferences.WorldLocations.GetWorldLocation(location)
		if floor == 0 && p == position {
			return uint8(location)
		}
	}

	return 0
}

func (g *GameState) observeAtlasLandmark(position references.Position, kind, name string, source journal.AtlasObservationSource, sample dungeonObservationSample) {
	if kind == "" {
		return
	}
	key := g.atlasSiteKey(position, sample)
	if key == 0 || key == journal.AtlasGuardiansSiteKey && (kind != "guardians" || source != journal.AtlasSourceLook) {
		return
	}
	index := -1
	for i := range g.atlasLandmarks {
		if g.atlasLandmarks[i].SiteKey == key {
			index = i

			break
		}
	}
	if index < 0 {
		g.atlasLandmarks = append(g.atlasLandmarks, journal.AtlasLandmarkObservation{SiteKey: key, Site: journal.SiteLocation{Map: sample.sextant.Location, Floor: sample.sextant.Floor, X: int(position.X), Y: int(position.Y)}})
		index = len(g.atlasLandmarks) - 1
	}
	observation := &g.atlasLandmarks[index]
	before := *observation
	observation.Kind, observation.Source, observation.ObservedAt = kind, source, sample.at
	if name != "" && atlasNameSourceValid(key, source) &&
		(observation.Name != name || observation.NameSource != source) {
		observation.Name = name
		observation.NameSource = source
		observation.NamedAt = sample.at
	}
	// REDUX DEVIATION: VIEW-0006: original Use reports only the Avatar on request. Redux
	// remembers exact locally inspected targets under the same sextant gates.
	// See ref:VIEW-0006, ref:ASTRO-0006.
	if !observation.Survey.Measured && EvaluateSextantEligibility(sample.sextant) == SextantReady {
		observation.Survey = journal.SurveyedLocation{Measured: true, Flexible: sample.sextant.Flexible, Location: observation.Site, RecordedAt: sample.at}
		observation.SurveySource = source
	}
	g.publishAtlasChange(before, *observation)
}

func (g *GameState) observeAtlasLook(position references.Position, tile indexes.SpriteIndex) {
	if g.isAtlasGuardiansArtwork(position, tile) {
		g.observeAtlasLandmark(references.Position{X: codexGuardianX, Y: codexGuardianY}, "guardians", "", journal.AtlasSourceLook, g.sampleDungeonObservation())

		return
	}
	g.observeAtlasLandmark(position, atlasKind(tile), "", journal.AtlasSourceLook, g.sampleDungeonObservation())
}

// Installed BRIT.DAT places the two angel artworks either side of the checkpoint.
// Match both the resolved description tile and immutable artwork; a replaced,
// obscured or remote angel (or plain checkpoint terrain) is not this feature.
func (g *GameState) isAtlasGuardiansArtwork(position references.Position, tile indexes.SpriteIndex) bool {
	if g.MapState.PlayerLocation.Location != references.BritanniaUnderworld || g.MapState.PlayerLocation.Floor != 0 {
		return false
	}
	matched := position == (references.Position{X: 232, Y: 235}) && tile == indexes.CodexAngelLeft ||
		position == (references.Position{X: 234, Y: 235}) && tile == indexes.CodexAngelRight

	return matched && g.GameReferences.OverworldLargeMapReference.GetSpriteIndex(position.X, position.Y) == tile
}

func (g *GameState) publishAtlasChange(before, after journal.AtlasLandmarkObservation) {
	change := journal.AtlasChange{Kind: after.Kind, Name: "Unnamed " + after.Kind,
		EntryKey: fmt.Sprintf("observed-%d", after.SiteKey), Discovered: before.Source == "",
		Named: before.Name == "" && after.Name != "", Surveyed: !before.Survey.Measured && after.Survey.Measured,
		Measured: after.Survey.Measured}
	if !change.Discovered && !change.Named && !change.Surveyed {
		return
	}
	for _, point := range references.SuppliedAtlasChart(g.GameReferences.LocationReferences.WorldLocations) {
		if point.SiteKey == after.SiteKey {
			change.EntryKey = point.Key
			if after.SiteKey == journal.AtlasGuardiansSiteKey {
				change.Name = point.Name
			}

			break
		}
	}
	if after.Name != "" && atlasNameSourceValid(after.SiteKey, after.NameSource) {
		change.Name = atlasDisplayName(after.Name)
	}
	g.SystemCallbacks.UI.AtlasUpdated(change)
}

// Naming comes from the introduction actually presented, not another name lookup.
func atlasPresentedEntryName(introduction string) string {
	if _, name, ok := strings.Cut(introduction, "\n\n"); ok {
		return strings.TrimSpace(name)
	}

	return strings.TrimSpace(strings.TrimPrefix(introduction, "Enter "))
}

// atlasDisplayName softens all-capital introductions without consulting hidden
// references or changing the stored evidence. Already mixed-case names stay exact.
func atlasDisplayName(name string) string {
	if name != strings.ToUpper(name) {
		return name
	}
	wordStart := true

	return strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) {
			first := wordStart
			wordStart = false
			if first {
				return unicode.ToUpper(r)
			}

			return unicode.ToLower(r)
		}
		wordStart = unicode.IsSpace(r) || r == '-'

		return r
	}, name)
}

// CurrentAtlasDossier has no private unmeasured sites or live quest-object lookup.
// Opening this detached projection cannot discover, survey or spend time.
func (g *GameState) CurrentAtlasDossier() journal.AtlasDossier {
	dossier := journal.AtlasDossier{}
	chart := references.SuppliedAtlasChart(g.GameReferences.LocationReferences.WorldLocations)
	chartedKeys := make(map[uint8]int)
	chartEntries := make(map[string]int, len(chart))
	for _, p := range chart {
		if references.IsSuppliedAtlasSymbol(p) {
			dossier.Symbols = append(dossier.Symbols, journal.AtlasSymbol{Key: p.Key, Kind: p.Kind, Category: atlasCategory(p.Kind), Phase: p.Phase,
				Position: journal.SiteLocation{Map: references.BritanniaUnderworld, X: int(p.Position.X), Y: int(p.Position.Y)}})
		}
		entry := journal.AtlasEntry{Key: p.Key, Name: p.Name, Kind: p.Kind, Category: atlasCategory(p.Kind), Phase: p.Phase, Charted: true}
		entry.NameKnown = p.SiteKey == uint8(references.LordBritishsCastle) || p.SiteKey == journal.AtlasGuardiansSiteKey || p.SiteKey == 0
		if p.Kind == "moonstone" {
			entry.Category = journal.AtlasMoonstones
			entry.Name = datetime.MoonPhase(p.Phase).String()
			entry.OriginReference = p.Name
		}
		if p.SiteKey != 0 {
			chartedKeys[p.SiteKey] = len(dossier.Entries)
		}
		chartEntries[p.Key] = len(dossier.Entries)
		dossier.Entries = append(dossier.Entries, entry)
	}
	for _, o := range g.atlasLandmarks {
		index, charted := chartedKeys[o.SiteKey]
		if !charted {
			index = len(dossier.Entries)
			dossier.Entries = append(dossier.Entries, journal.AtlasEntry{Key: fmt.Sprintf("observed-%d", o.SiteKey), Name: "Unnamed " + o.Kind, Kind: o.Kind, Category: atlasCategory(o.Kind), Phase: -1})
		}
		e := &dossier.Entries[index]
		if o.Name != "" && atlasNameSourceValid(o.SiteKey, o.NameSource) {
			e.Name = atlasDisplayName(o.Name)
			e.NameKnown = true
			e.NamedAt, e.NameSource = o.NamedAt, o.NameSource
		}
		e.Survey, e.ObservedAt = o.Survey, o.ObservedAt
		e.SurveySource = string(o.SurveySource)
		e.Discovered = o.ObservedAt.Known || o.Source == journal.AtlasSourceLook ||
			o.Source == journal.AtlasSourceEnter && atlasNameSourceValid(o.SiteKey, o.Source) ||
			o.Source == journal.AtlasSourceShrineIntroduction && atlasNameSourceValid(o.SiteKey, o.Source)
		if !e.Discovered {
			e.Survey, e.SurveySource = journal.SurveyedLocation{}, ""
		}
	}
	for _, dungeon := range g.journalDungeons() {
		if dungeon.Name == references.Doom.String() {
			continue
		}
		site := dungeon.Sites[journal.DungeonSurfaceSite]
		if !site.Inspected && dungeon.Sites[journal.DungeonUnderworldSite].Inspected && dungeon.Word == "" {
			continue
		}
		dossier.Entries = append(dossier.Entries, journal.AtlasEntry{Key: "dungeon-" + dungeon.Name, Name: dungeon.Name, NameKnown: true, Discovered: site.Inspected, Kind: "dungeon", Category: journal.AtlasDungeons, Phase: -1, Survey: site.Survey, ObservedAt: site.ObservedAt})
	}
	for _, moon := range g.journalMoonstones() {
		index, ok := chartEntries[references.AtlasMoonstoneChartKey(int(moon.Phase))]
		if !ok {
			panic("atlas chart is missing a moonstone origin")
		}
		entry := &dossier.Entries[index]
		entry.Carried = moon.Carried
		observed := g.moonstoneJournal[int(moon.Phase)]
		entry.Discovered = observed.SiteKnown && observed.LastContext.Source != journal.MoonstoneSourceNone &&
			observed.LastContext.Source != journal.MoonstoneSourcePossession
		entry.ObservedAt = moon.ObservedAt
		if moon.OriginalSiteRetired {
			entry.Charted = false
			entry.OriginReference = ""
			entry.Name = moon.Phase.String() + " (last seen)"
			if moon.Carried {
				entry.Name = moon.Phase.String() + " (carried)"
			}
		}
		if entry.Discovered {
			entry.Survey = moon.LastObservedSurvey
		}
		if entry.Survey.Measured {
			entry.SurveySource = moon.SurveySource.String()
			entry.LastObservedDestination = true
		}
	}
	g.appendPersonalAtlas(&dossier)
	// Stable category grouping retains authored order and neutral selection keys.
	sort.SliceStable(dossier.Entries, func(i, j int) bool {
		return dossier.Entries[i].Category < dossier.Entries[j].Category
	})
	if fix, ok := g.CurrentSextantFix(); ok {
		dossier.CurrentFix = journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{Map: fix.Location, Floor: fix.Floor, X: int(fix.Position.X), Y: int(fix.Position.Y)}}
		if isSurfaceSextantSettlement(g.MapState.PlayerLocation.Location) {
			dossier.FixContext = "Within this settlement"
			if observation := g.currentEarnedAtlasLandmark(); observation != nil && observation.Name != "" && atlasNameSourceValid(observation.SiteKey, observation.NameSource) {
				dossier.FixContext = "Within " + atlasDisplayName(observation.Name)
			}
		}
	} else {
		dossier.FixUnavailable = g.sextantUnavailableReason()
		dossier.FixNeedsNight = g.CurrentSextantEligibility() == SextantDaylight
	}

	return dossier
}

// Interior Use can measure only an existing genuinely entered landmark. A
// supplied chart or debug-loaded current map is not acquisition evidence.
func (g *GameState) currentEarnedAtlasLandmark() *journal.AtlasLandmarkObservation {
	key := uint8(g.MapState.PlayerLocation.Location)
	for i := range g.atlasLandmarks {
		observation := &g.atlasLandmarks[i]
		if observation.SiteKey == key && (atlasNameSourceValid(key, observation.NameSource) ||
			observation.Source == journal.AtlasSourceEnter) {
			return observation
		}
	}

	return nil
}

func (g *GameState) surveyCurrentAtlasSettlement(fix references.PlayerLocation) {
	if !isSurfaceSextantSettlement(g.MapState.PlayerLocation.Location) {
		return
	}
	observation := g.currentEarnedAtlasLandmark()
	site := moonstoneJournalSite(fix)
	if observation == nil || observation.Survey.Measured || observation.Site != site {
		return
	}
	sample := g.sampleDungeonObservation()
	// The party is personally observing its current settlement again. Record
	// that same moment so a later interior reading does not predate its evidence.
	observation.ObservedAt = sample.at
	before := *observation
	observation.Survey = journal.SurveyedLocation{Measured: true, Flexible: sample.sextant.Flexible, Location: site, RecordedAt: sample.at}
	observation.SurveySource = journal.AtlasSourceEnter
	g.publishAtlasChange(before, *observation)
}
