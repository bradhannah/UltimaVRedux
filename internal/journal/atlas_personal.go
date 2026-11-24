package journal

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	// MaxAtlasNotes bounds the checkpointed collection without evicting authored notes.
	MaxAtlasNotes = 256
	// MaxAtlasNoteLabelRunes bounds the short, single-line map label.
	MaxAtlasNoteLabelRunes = 32
	// MaxAtlasNoteDescriptionRunes includes intentional paragraph line feeds.
	MaxAtlasNoteDescriptionRunes = 500
	// MaxAtlasIdentity remains exact in JSON clients and Go; zero means unregistered.
	MaxAtlasIdentity          uint64 = 1<<53 - 1
	atlasSurfaceMaxCoordinate        = 255
)

// ErrInvalidAtlasPersonal identifies malformed personal atlas state or authoring input.
var ErrInvalidAtlasPersonal = errors.New("invalid personal atlas record")

// AtlasSymbol is immutable supplied artwork, with no name or interaction authority.
type AtlasSymbol struct {
	Key      string
	Kind     string
	Category AtlasCategory
	Phase    int
	Position SiteLocation
}

// AtlasNoteRecord is checkpointed player authorship, independent of landmark history.
type AtlasNoteRecord struct {
	ID          uint64       `json:"id"`
	Position    SiteLocation `json:"position"`
	Label       string       `json:"label"`
	Description string       `json:"description"`
	RecordedAt  GameTime     `json:"recordedAt"`
}

// AtlasNotesState preserves the allocation watermark even after deletion.
type AtlasNotesState struct {
	LastID  uint64            `json:"lastId"`
	Records []AtlasNoteRecord `json:"records"`
}

// AtlasFleetState stores only identity allocation, never a second position cache.
// Each existing boat carries its own VesselID through parked/boarded transitions.
type AtlasFleetState struct {
	LastID uint64 `json:"lastId"`
}

// AtlasVesselRecord is the cross-representation validation view of a boat's
// runtime metadata. Positions stay solely on its authoritative map unit.
type AtlasVesselRecord struct {
	ID          uint64
	VehicleType references.VehicleType
}

// ValidateAtlasFleet checks parked and boarded representations together. The
// adapter supplies every vehicle, including unregistered non-boats (ID zero).
func ValidateAtlasFleet(state AtlasFleetState, records []AtlasVesselRecord) error {
	if state.LastID > MaxAtlasIdentity {
		return fmt.Errorf("%w: fleet watermark", ErrInvalidAtlasPersonal)
	}
	seen := make(map[uint64]bool)
	for _, record := range records {
		if record.ID == 0 {
			continue
		}
		if record.ID > state.LastID || seen[record.ID] ||
			record.VehicleType != references.FrigateVehicle && record.VehicleType != references.SkiffVehicle {
			return fmt.Errorf("%w: fleet identity or vehicle type", ErrInvalidAtlasPersonal)
		}
		seen[record.ID] = true
	}

	return nil
}

// AtlasVesselPosition is a detached currently parked, registered surface vessel.
type AtlasVesselPosition struct {
	ID       uint64
	Position SiteLocation
}

// ValidateAtlasNoteText uses rune counts and the game's printable ASCII repertoire.
// Description line feeds are intentional paragraphs; other controls are rejected.
func ValidateAtlasNoteText(label, description string) error {
	if strings.TrimSpace(label) == "" || strings.TrimSpace(label) != label ||
		utf8.RuneCountInString(label) > MaxAtlasNoteLabelRunes ||
		utf8.RuneCountInString(description) > MaxAtlasNoteDescriptionRunes {
		return fmt.Errorf("%w: the map label needs 1 to 32 letters; the description holds 500", ErrInvalidAtlasPersonal)
	}
	for field, value := range []string{label, description} {
		for _, r := range value {
			if field == 1 && r == '\n' {
				continue
			}
			if r < ' ' || r > '~' {
				return fmt.Errorf("%w: use only letters the game can show", ErrInvalidAtlasPersonal)
			}
		}
	}

	return nil
}

// ValidateAtlasNotes checks all persisted authoring facts atomically.
func ValidateAtlasNotes(state AtlasNotesState) error {
	if len(state.Records) > MaxAtlasNotes || state.LastID > MaxAtlasIdentity {
		return fmt.Errorf("%w: note capacity or identity watermark", ErrInvalidAtlasPersonal)
	}
	seenPositions := make(map[SiteLocation]bool)
	var previous uint64
	for _, record := range state.Records {
		if record.ID > state.LastID || record.ID <= previous || seenPositions[record.Position] {
			return fmt.Errorf("%w: duplicate or unordered note identity/position", ErrInvalidAtlasPersonal)
		}
		previous = record.ID
		seenPositions[record.Position] = true
		if !ValidAtlasSurfacePosition(record.Position) ||
			!validAtlasReceiptTime(OptionalGameTime{Known: true, Time: record.RecordedAt}) {
			return fmt.Errorf("%w: note position or date", ErrInvalidAtlasPersonal)
		}
		if err := ValidateAtlasNoteText(record.Label, record.Description); err != nil {
			return err
		}
	}

	return nil
}

// ValidAtlasSurfacePosition excludes room coordinates and underground floors.
func ValidAtlasSurfacePosition(site SiteLocation) bool {
	return site.Map == references.BritanniaUnderworld && site.Floor == 0 &&
		site.X >= 0 && site.X <= atlasSurfaceMaxCoordinate && site.Y >= 0 && site.Y <= atlasSurfaceMaxCoordinate
}

// AtlasNoteKey remains stable through renaming and checkpoint restoration.
func AtlasNoteKey(id uint64) string { return fmt.Sprintf("note-%d", id) }
