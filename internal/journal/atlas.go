package journal

import (
	"errors"
	"fmt"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// MaxAtlasLandmarks bounds 31 ordinary sites, seven physical shrines and Guardians.
const MaxAtlasLandmarks = 39

// AtlasGuardiansSiteKey identifies the one exterior Guardians checkpoint.
const AtlasGuardiansSiteKey uint8 = 108

// AtlasObservationSource identifies a successful local acquisition boundary.
type AtlasObservationSource string

// Atlas acquisition sources are deliberately finite, without a generic event ledger.
const (
	AtlasSourceLook               AtlasObservationSource = "look"
	AtlasSourceEnter              AtlasObservationSource = "enter"
	AtlasSourceShrineIntroduction AtlasObservationSource = "shrine-introduction"
	AtlasSourceSextantReceipt     AtlasObservationSource = "sextant-receipt"
)

// ErrInvalidAtlasObservation identifies malformed persisted atlas history.
var ErrInvalidAtlasObservation = errors.New("invalid atlas observation")

const (
	atlasFirstShrineKey = 101
	atlasLastShrineKey  = 107
	atlasGuardiansKind  = "guardians"
)

// AtlasLandmarkObservation is private, bounded earned history. Baseline chart
// facts and current Avatar fixes are never persisted here. Fields have value ownership.
type AtlasLandmarkObservation struct {
	SiteKey      uint8                  `json:"siteKey"`
	Site         SiteLocation           `json:"site"`
	Kind         string                 `json:"kind"`
	Name         string                 `json:"name"`
	Source       AtlasObservationSource `json:"source"`
	NameSource   AtlasObservationSource `json:"nameSource"`
	ObservedAt   OptionalGameTime       `json:"observedAt"`
	NamedAt      OptionalGameTime       `json:"namedAt"`
	Survey       SurveyedLocation       `json:"survey"`
	SurveySource AtlasObservationSource `json:"surveySource"`
}

// ValidateAtlasLandmarks checks domain shape; native validation also validates
// dates and canonical SiteKey->exterior coordinates against immutable references.
func ValidateAtlasLandmarks(records []AtlasLandmarkObservation) error {
	if len(records) > MaxAtlasLandmarks {
		return fmt.Errorf("%w: landmarks exceed %d", ErrInvalidAtlasObservation, MaxAtlasLandmarks)
	}
	seen := make(map[uint8]bool, len(records))
	for _, record := range records {
		if seen[record.SiteKey] {
			return fmt.Errorf("%w: duplicate site key %d", ErrInvalidAtlasObservation, record.SiteKey)
		}
		seen[record.SiteKey] = true
		if err := validateAtlasLandmark(record); err != nil {
			return err
		}
	}

	return nil
}

// atlasSiteClass sorts a site key into the ordinary-settlement, shrine and Guardians categories.
func atlasSiteClass(siteKey uint8) (bool, bool, bool) {
	ordinary := siteKey >= uint8(references.Moonglow) && siteKey <= uint8(references.SerpentsHold) &&
		siteKey != uint8(references.Ararat)
	shrine := siteKey >= atlasFirstShrineKey && siteKey <= atlasLastShrineKey
	guardians := siteKey == AtlasGuardiansSiteKey

	return ordinary, shrine, guardians
}

// validateAtlasLandmark checks the bounded site/source/survey invariants together at the persistence boundary.
func validateAtlasLandmark(record AtlasLandmarkObservation) error {
	ordinary, shrine, guardians := atlasSiteClass(record.SiteKey)
	if !ordinary && !shrine && !guardians {
		return fmt.Errorf("%w: site key %d", ErrInvalidAtlasObservation, record.SiteKey)
	}
	site := record.Site
	if site.Map != references.BritanniaUnderworld || site.Floor != 0 ||
		site.X < 0 || site.X > 255 || site.Y < 0 || site.Y > 255 {
		return fmt.Errorf("%w: exterior %d", ErrInvalidAtlasObservation, record.SiteKey)
	}
	if err := validateAtlasKindAndSource(record); err != nil {
		return err
	}
	if err := validateAtlasCategory(record, ordinary, shrine, guardians); err != nil {
		return err
	}
	if err := validateAtlasReceipt(record); err != nil {
		return err
	}
	if err := validateAtlasName(record, shrine); err != nil {
		return err
	}

	return validateAtlasSurvey(record, shrine)
}

func validateAtlasKindAndSource(record AtlasLandmarkObservation) error {
	switch record.Kind {
	case "towne", "village", "hut", "keep", "castle", "lighthouse", "shrine", "ruins", atlasGuardiansKind:
	default:
		return fmt.Errorf("%w: kind %q", ErrInvalidAtlasObservation, record.Kind)
	}
	if record.Source != AtlasSourceLook && record.Source != AtlasSourceEnter &&
		record.Source != AtlasSourceShrineIntroduction && record.Source != AtlasSourceSextantReceipt {
		return fmt.Errorf("%w: source %q", ErrInvalidAtlasObservation, record.Source)
	}

	return nil
}

func validateAtlasCategory(record AtlasLandmarkObservation, ordinary, shrine, guardians bool) error {
	if shrine && (record.Kind != "shrine" && record.Kind != "ruins" || record.Source == AtlasSourceEnter) ||
		ordinary && record.Source == AtlasSourceShrineIntroduction {
		return fmt.Errorf("%w: source/category mismatch %d", ErrInvalidAtlasObservation, record.SiteKey)
	}

	return validateAtlasGuardians(record, guardians)
}

func validateAtlasGuardians(record AtlasLandmarkObservation, guardians bool) error {
	if guardians && (record.Kind != atlasGuardiansKind || record.Source != AtlasSourceLook ||
		record.Name != "" || record.Site.X != 233 || record.Site.Y != 235 ||
		record.Survey.Measured && record.SurveySource != AtlasSourceLook) || !guardians && record.Kind == atlasGuardiansKind {
		return fmt.Errorf("%w: Guardians evidence", ErrInvalidAtlasObservation)
	}

	return nil
}

func validateAtlasSurvey(record AtlasLandmarkObservation, shrine bool) error {
	var emptySurvey SurveyedLocation
	if record.Survey.Measured && record.Survey.Location != record.Site ||
		!record.Survey.Measured && (record.Survey != emptySurvey || record.SurveySource != "") {
		return fmt.Errorf("%w: survey %d", ErrInvalidAtlasObservation, record.SiteKey)
	}
	validSource := record.SurveySource == AtlasSourceLook ||
		!shrine && record.SurveySource == AtlasSourceEnter ||
		shrine && record.SurveySource == AtlasSourceShrineIntroduction ||
		record.SurveySource == AtlasSourceSextantReceipt
	if record.Survey.Measured && !validSource {
		return fmt.Errorf("%w: survey source %d", ErrInvalidAtlasObservation, record.SiteKey)
	}

	return nil
}

// The receipt source is a finite local exception, not a second survey policy.
// Normal sources retain native date/night validation; Flexible stays truthful.
func validateAtlasReceipt(record AtlasLandmarkObservation) error {
	if record.Source != AtlasSourceSextantReceipt && record.SurveySource != AtlasSourceSextantReceipt {
		return nil
	}
	if record.SiteKey != uint8(references.Greyhaven) || record.Kind != "lighthouse" ||
		record.Site.X != 104 || record.Site.Y != 216 || record.Name == "" || record.NameSource != AtlasSourceEnter ||
		!record.Survey.Measured || record.SurveySource != AtlasSourceSextantReceipt || !validAtlasReceiptTimeline(record) {
		return fmt.Errorf("%w: sextant receipt evidence", ErrInvalidAtlasObservation)
	}

	return nil
}

func validAtlasReceiptTimeline(record AtlasLandmarkObservation) bool {
	// Historical legitimate Enter names may have no recorded date. Preserve that
	// absence rather than fabricate a naming date when the live receipt occurs.
	var noDate OptionalGameTime
	naming := record.NamedAt == noDate || validAtlasReceiptTime(record.NamedAt) &&
		atlasTimeOrdinal(record.NamedAt.Time) <= atlasTimeOrdinal(record.Survey.RecordedAt.Time)

	return validAtlasReceiptTime(record.ObservedAt) && validAtlasReceiptTime(record.Survey.RecordedAt) &&
		naming &&
		atlasTimeOrdinal(record.Survey.RecordedAt.Time) <= atlasTimeOrdinal(record.ObservedAt.Time) &&
		(record.Source != AtlasSourceSextantReceipt || record.ObservedAt == record.Survey.RecordedAt)
}

func validAtlasReceiptTime(at OptionalGameTime) bool {
	t := at.Time

	return at.Known && t.Year >= 1 && t.Year <= 65535 && t.Month >= 1 && t.Month <= 13 &&
		t.Day >= 1 && t.Day <= 28 && t.Hour >= 0 && t.Hour < 24 && t.Minute >= 0 && t.Minute < 60
}

func atlasTimeOrdinal(t GameTime) int64 {
	return (((int64(t.Year)*13+int64(t.Month))*28+int64(t.Day))*24+int64(t.Hour))*60 + int64(t.Minute)
}

func validateAtlasName(record AtlasLandmarkObservation, shrine bool) error {
	if len(record.Name) > 160 || strings.TrimSpace(record.Name) != record.Name ||
		strings.ContainsAny(record.Name, "\r\n\t") {
		return fmt.Errorf("%w: name %d", ErrInvalidAtlasObservation, record.SiteKey)
	}
	var noDate OptionalGameTime
	if record.Name == "" {
		if record.NameSource != "" || record.NamedAt != noDate {
			return fmt.Errorf("%w: absent name provenance %d", ErrInvalidAtlasObservation, record.SiteKey)
		}
	} else if !shrine && record.NameSource != AtlasSourceEnter ||
		shrine && record.NameSource != AtlasSourceShrineIntroduction {
		return fmt.Errorf("%w: naming source %d", ErrInvalidAtlasObservation, record.SiteKey)
	}

	return nil
}

// AtlasCategory is an authored filter group, independent of original file groups.
type AtlasCategory uint8

// Atlas categories contain authorised points and earned unmeasured rows only.
const (
	AtlasSettlements AtlasCategory = iota
	AtlasKeeps
	AtlasCastles
	AtlasShrines
	AtlasLighthouses
	AtlasDungeons
	AtlasMoonstones
	AtlasVessels
	AtlasNotes
)

// AtlasEntry contains only independently public plot authorities. No private site,
// hidden location identity or observer context can escape through this DTO.
type AtlasEntry struct {
	Key, Name, Kind string
	// OriginReference is supplied regional text, never a precise moonstone point.
	OriginReference string
	// NameKnown marks a supplied or genuinely learned identity, never a survey.
	NameKnown bool
	// Discovered requires personal visit/observation evidence, not a supplied symbol or name.
	Discovered              bool
	Carried                 bool // Inventory status for a moonstone, never a destination position.
	Category                AtlasCategory
	Phase                   int
	Charted                 bool
	Survey                  SurveyedLocation
	HistoricalSurvey        SurveyedLocation
	ObservedAt, NamedAt     OptionalGameTime
	NameSource              AtlasObservationSource
	SurveySource            string
	LastObservedDestination bool
	// Notes and vessels have explicit authorities, never invented landmark surveys.
	Note   AtlasNoteRecord
	Vessel AtlasVesselPosition
}

// PlotPosition is the sole entry-interaction authority: supplied artwork never
// authorizes callouts, selection brackets, numeric coordinates or camera focus.
func (e AtlasEntry) PlotPosition() (SiteLocation, bool) {
	if e.Category == AtlasNotes && e.Note.ID != 0 {
		return e.Note.Position, true
	}
	if e.Category == AtlasVessels && e.Vessel.ID != 0 {
		return e.Vessel.Position, true
	}
	if e.Survey.Measured {
		return e.Survey.Location, true
	}
	var absent SiteLocation

	return absent, false
}

// AtlasDossier is detached reader data, including an ephemeral current fix.
type AtlasDossier struct {
	Entries         []AtlasEntry
	Symbols         []AtlasSymbol
	VesselsUnlocked bool
	CurrentFix      SurveyedLocation
	FixUnavailable  string
	// FixNeedsNight reports that a carried sextant is only waiting for night.
	FixNeedsNight bool
	FixContext    string
}

// AtlasPlaceFilter chooses one place category without hiding its known sources.
type AtlasPlaceFilter uint8

// Place filters start with the complete map, followed by its authored categories.
const (
	AtlasAllPlaces AtlasPlaceFilter = iota
	AtlasTownes
	AtlasKeepPlaces
	AtlasCastlePlaces
	AtlasShrinePlaces
	AtlasLighthousePlaces
	AtlasDungeonPlaces
	AtlasMoonstoneSites
	AtlasVesselPlaces
	AtlasNotePlaces
	AtlasPlaceFilterCount
)

// String is the readable category name used by the list and filter selector.
func (filter AtlasPlaceFilter) String() string {
	switch filter {
	case AtlasAllPlaces:
		return "All"
	case AtlasTownes:
		return "Settlements"
	case AtlasKeepPlaces:
		return "Keeps"
	case AtlasCastlePlaces:
		return "Castles"
	case AtlasShrinePlaces:
		return "Shrines"
	case AtlasLighthousePlaces:
		return "Lighthouses"
	case AtlasDungeonPlaces:
		return "Dungeons"
	case AtlasMoonstoneSites:
		return "Moonstones"
	case AtlasVesselPlaces:
		return "Vessels"
	case AtlasNotePlaces:
		return "My Notes"
	case AtlasPlaceFilterCount:
	}
	panic("invalid atlas place filter")
}

// AtlasFilters is presentation-only. All authorised source evidence stays visible.
type AtlasFilters struct {
	Places AtlasPlaceFilter
}

// DefaultAtlasFilters shows every authorised place, including unmeasured rows.
func DefaultAtlasFilters() AtlasFilters {
	return AtlasFilters{Places: AtlasAllPlaces}
}

// FilterAtlasEntries selects place types without changing their source meaning.
func FilterAtlasEntries(entries []AtlasEntry, filters AtlasFilters) []AtlasEntry {
	var out []AtlasEntry
	for _, entry := range entries {
		if entry.Category > AtlasNotes || filters.Places >= AtlasPlaceFilterCount ||
			filters.Places != AtlasAllPlaces && entry.Category != AtlasCategory(filters.Places-1) {
			continue
		}
		out = append(out, entry)
	}

	return out
}
