package game_state

import (
	"slices"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// SextantContext is an explicit, value-only sample. Callers choose the sampling
// boundary: dungeon Enter uses its source; a moonstone arrival uses its destination.
type SextantContext struct {
	HasSextant bool
	Flexible   bool
	Location   references.Location
	Floor      references.FloorNumber
	Hour       uint8
}

// SextantEligibility preserves inventory, location, then hour rejection order.
type SextantEligibility uint8

// Eligibility outcomes are ordered by the original possession/context checks.
const (
	SextantReady SextantEligibility = iota
	SextantMissing
	SextantNotOverworld
	SextantDaylight
)

// EvaluateSextantEligibility is callback-free and acquires no knowledge.
// Default eligibility requires the surface world, a nonnegative floor,
// and hours 19 through 05; item possession is required by Use. ref:ASTRO-0006
func EvaluateSextantEligibility(context SextantContext) SextantEligibility {
	if !context.HasSextant {
		return SextantMissing
	}
	world := context.Location == references.BritanniaUnderworld && context.Floor == 0
	settlement := context.Flexible && isSurfaceSextantSettlement(context.Location) && context.Floor >= -1 && context.Floor <= 3
	if !world && !settlement {
		return SextantNotOverworld
	}
	// REDUX DEVIATION: original item-use rules permit the sextant only on the surface
	// world map at night (19..05). The optional flexible policy permits daytime
	// and surface settlements, using their canonical exterior rather than local XY.
	// See ref:ASTRO-0006.
	if context.Hour >= 24 || !context.Flexible && !isUseAstronomyNight(context.Hour) {
		return SextantDaylight
	}

	return SextantReady
}

func isSurfaceSextantSettlement(location references.Location) bool {
	return location >= references.Moonglow && location <= references.SerpentsHold && location != references.Ararat
}

func isUseAstronomyNight(hour uint8) bool {
	return hour < 6 || hour >= 19 && hour < 24
}

// CurrentSextantContext captures authoritative inventory, world and game hour.
func (g *GameState) CurrentSextantContext() SextantContext {
	return SextantContext{
		HasSextant: g.PartyState.Inventory.QuestItems.Get(references.Sextant) > 0,
		Flexible:   g.GameConfig.GameplayConfiguration.FlexibleSextantUse,
		Location:   g.MapState.PlayerLocation.Location,
		Floor:      g.MapState.PlayerLocation.Floor,
		Hour:       g.DateTime.Hour,
	}
}

// CurrentSextantEligibility is a pure adapter; journal/map readers may call it.
func (g *GameState) CurrentSextantEligibility() SextantEligibility {
	context := g.CurrentSextantContext()
	eligibility := EvaluateSextantEligibility(context)
	if eligibility != SextantMissing && context.Flexible && isSurfaceSextantSettlement(context.Location) {
		if _, ok := g.sextantSettlementExterior(context); !ok {
			return SextantNotOverworld
		}
	}

	return eligibility
}

// Reference-backed adapters reject nonexistent floors and malformed exteriors.
func (g *GameState) sextantSettlementExterior(context SextantContext) (references.PlayerLocation, bool) {
	refs := g.GameReferences.LocationReferences
	local := refs.GetLocationReference(context.Location)
	if local == nil || local.Location != context.Location || !slices.Contains(local.ListOfFloors, context.Floor) {
		return references.PlayerLocation{}, false
	}
	exterior, ok := refs.WorldLocations.LargeMapLocationPositions[context.Location]
	if !ok || exterior.Location != context.Location || exterior.Floor != 0 ||
		exterior.Position.X < 0 || exterior.Position.X > 255 || exterior.Position.Y < 0 || exterior.Position.Y > 255 {
		return references.PlayerLocation{}, false
	}

	return references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0, Position: exterior.Position}, true
}

// CurrentSextantFix returns the actual surface Avatar position or an eligible
// settlement's canonical surface exterior. It never remembers a last-known fix
// or surveys nearby landmarks. The absent value is zero.
func (g *GameState) CurrentSextantFix() (references.PlayerLocation, bool) {
	if g.CurrentSextantEligibility() != SextantReady {
		return references.PlayerLocation{}, false
	}

	context := g.CurrentSextantContext()
	if isSurfaceSextantSettlement(context.Location) {
		return g.sextantSettlementExterior(context)
	}

	return g.MapState.PlayerLocation, true
}

func (g *GameState) sextantUnavailableReason() string {
	switch g.CurrentSextantEligibility() {
	case SextantMissing:
		return "No sextant carried."
	case SextantNotOverworld:
		location := g.MapState.PlayerLocation
		if location.Location == references.Ararat || location.Location.GetMapType() == references.DungeonMapType ||
			location.Location == references.BritanniaUnderworld && location.Floor == -1 {
			return "Unavailable underground."
		}

		return "Use on the world map."
	case SextantDaylight:
		if g.DateTime.Hour >= 24 {
			return "Invalid game hour."
		}

		return "Only at night!"
	case SextantReady:
		return ""
	}
	panic("unknown sextant eligibility")
}
