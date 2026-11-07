package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func (g *GameState) capturePersonalSnapshot(snapshot *save_snapshot.SaveSnapshot) {
	snapshot.AtlasNotes = g.CaptureAtlasNotes()
	// Preserve the accepted empty-array representation without retaining any
	// writable capacity from the runtime's optional collection.
	if g.atlasNotes.Records != nil && len(g.atlasNotes.Records) == 0 {
		snapshot.AtlasNotes.Records = g.atlasNotes.Records[:0:0]
	}
	snapshot.Notes = g.CapturePlayerNotes()
	snapshot.AtlasFleet = g.CaptureAtlasFleet()
	snapshot.CodexRevelationComplete = g.CodexRevelationComplete
	for i, record := range g.BasementChestHistory {
		if date := record.LastOpened; date != nil {
			snapshot.BasementChestHistory[i].LastOpened = &save_snapshot.BritannianDateSnapshot{Year: int(date.Year), Month: int(date.Month), Day: int(date.Day)}
		}
	}
}

func (g *GameState) restoreBasementChestHistory(snapshot save_snapshot.SaveSnapshot) {
	// Install consumption before any controller population, including historical
	// reconstruction of off-floor NPCs. Saved survivors are restored afterward.
	for i, record := range snapshot.BasementChestHistory {
		if date := record.LastOpened; date != nil {
			g.BasementChestHistory[i].LastOpened = &datetime.BritannianDate{Year: uint16(date.Year), Month: byte(date.Month), Day: byte(date.Day)}
		}
	}
}

func (g *GameState) restorePersonalSnapshot(snapshot save_snapshot.SaveSnapshot) error {
	// Restoration is continuation, never a basement arrival or boat interaction.
	g.CodexRevelationComplete = snapshot.CodexRevelationComplete
	if err := g.RestoreAtlasNotes(snapshot.AtlasNotes); err != nil {
		return fmt.Errorf("hydrating atlas notes: %w", err)
	}
	if snapshot.AtlasNotes.Records != nil && len(snapshot.AtlasNotes.Records) == 0 {
		g.atlasNotes.Records = snapshot.AtlasNotes.Records[:0:0]
	}
	if err := g.RestorePlayerNotes(snapshot.Notes); err != nil {
		return fmt.Errorf("hydrating player notes: %w", err)
	}
	if err := g.RestoreAtlasFleet(snapshot.AtlasFleet); err != nil {
		return fmt.Errorf("hydrating atlas fleet: %w", err)
	}

	return nil
}

func validatePersonalSnapshotCompatibility(snapshot save_snapshot.SaveSnapshot, refs *references.GameReferences) error {
	for i, source := range references.LordBritishBasementChests() {
		needed := snapshot.BasementChestHistory[i].LastOpened != nil
		for _, unit := range snapshot.MapUnits {
			needed = needed || unit.SurfaceChestAuthored && unit.Position.Location == int(references.LordBritishsCastle) && unit.NPCNum == source.NPCNumber
		}
		if !needed {
			continue
		}
		castle := refs.LocationReferences.GetLocationReference(references.LordBritishsCastle)
		if castle == nil {
			return errors.New("basementChestHistory: Lord British's Castle reference unavailable")
		}
		npcs := *castle.GetNPCReferences()
		if source.NPCNumber >= len(npcs) {
			return fmt.Errorf("basementChestHistory[%d]: chest reference unavailable", i)
		}
		if index, ok := references.BasementChestIndex(npcs[source.NPCNumber], source.NPCNumber); !ok || index != i {
			return fmt.Errorf("basementChestHistory[%d]: authored identity differs from installed references", i)
		}
	}
	for i, unit := range snapshot.MapUnits {
		if !unit.SurfaceChestAuthored {
			continue
		}
		location := refs.LocationReferences.GetLocationReference(references.Location(unit.Position.Location))
		if location == nil || unit.NPCNum < 0 || unit.NPCNum >= len(*location.GetNPCReferences()) ||
			(*location.GetNPCReferences())[unit.NPCNum].GetSpriteIndex() != indexes.Chest {
			return fmt.Errorf("mapUnits[%d].surfaceChestAuthored: authored chest reference unavailable", i)
		}
	}
	for i, record := range snapshot.AtlasNotes.Records {
		if err := validateSnapshotPosition(refs, fmt.Sprintf("atlasNotes.records[%d].position", i), save_snapshot.PositionSnapshot{
			Location: int(record.Position.Map), Floor: int(record.Position.Floor), X: record.Position.X, Y: record.Position.Y,
		}); err != nil {
			return err
		}
	}

	return nil
}

func (g *GameState) hydrateSurfaceChest(unit save_snapshot.MapUnitSnapshot) *map_units.NPCFriendly {
	location := references.Location(unit.Position.Location)
	position := references.Position{X: references.Coordinate(unit.Position.X), Y: references.Coordinate(unit.Position.Y)}
	floor := references.FloorNumber(unit.Position.Floor)
	if unit.SurfaceChestAuthored {
		refs := *g.GameReferences.LocationReferences.GetLocationReference(location).GetNPCReferences()

		return map_units.NewNPCFriendly(refs[unit.NPCNum], unit.NPCNum)
	}
	if unit.SurfaceChestTransient {
		return map_units.NewTransientSurfaceChest(location, position, floor, unit.NPCNum, byte(*unit.SurfaceChestNumber))
	}

	return map_units.NewNPCFriendlySurfaceChest(location, position, floor, unit.NPCNum)
}
