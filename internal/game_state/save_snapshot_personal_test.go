package game_state

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/require"
)

func roundTripPersonalSnapshot(t *testing.T, state *GameState) (*GameState, save_snapshot.SaveSnapshot) {
	t.Helper()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	envelope := save_snapshot.Envelope{Format: save_snapshot.EnvelopeFormat, EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
		Metadata: save_snapshot.Metadata{Name: "Personal checkpoint", CreatedAt: time.Date(2026, 10, 4, 0, 0, 0, 0, time.UTC)}, Snapshot: snapshot}
	encoded, err := save_snapshot.EncodeCanonical(envelope)
	require.NoError(t, err)
	decoded, err := save_snapshot.Decode(encoded)
	require.NoError(t, err)
	require.Equal(t, snapshot, decoded.Snapshot)
	restored, err := NewGameStateFromSaveSnapshot(decoded.Snapshot, state.GameConfig, state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen, state.MapState.YTilesVisibleOnGameScreen)
	require.NoError(t, err)
	restored.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	restored.SetupEnvironmentalSystemCallbacks()
	recaptured, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, snapshot, recaptured, "complete capture/codec/hydration/recapture parity")
	envelope.Snapshot = recaptured
	again, err := save_snapshot.EncodeCanonical(envelope)
	require.NoError(t, err)
	require.Equal(t, encoded, again)
	return restored, snapshot
}

func TestSaveSnapshotV24NotesCheckpointIsolationAndStaleDraft(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(17, 29).WithRandomSeed(688).WithSystemCallbacks().Build()
	state.DateTime.Hour = 19
	state.initializeMoongateStage()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	draft, err := state.BeginAtlasNoteDraft(0)
	require.NoError(t, err)
	note, err := state.CommitAtlasNote(draft, " Landing ", "Walk north.\n\nBring  food.")
	require.NoError(t, err)
	stale, err := state.BeginAtlasNoteDraft(note.ID)
	require.NoError(t, err)
	restored, checkpoint := roundTripPersonalSnapshot(t, state)
	_, err = restored.CommitAtlasNote(stale, "Never committed", "")
	require.ErrorContains(t, err, "stale")
	require.Equal(t, []journal.AtlasNoteRecord{note}, restored.CaptureAtlasNotes().Records)
	_, err = state.CommitAtlasNote(stale, "Later edit", "")
	require.NoError(t, err)
	require.Equal(t, note, checkpoint.AtlasNotes.Records[0], "capture is detached from later edits")
	checkpoint.AtlasNotes.Records[0].Description = "external mutation"
	require.Equal(t, note, restored.CaptureAtlasNotes().Records[0], "hydration detaches note storage")
	deletion, err := restored.BeginAtlasNoteDraft(note.ID)
	require.NoError(t, err)
	require.NoError(t, restored.DeleteAtlasNote(deletion))
	deleted, _ := roundTripPersonalSnapshot(t, restored)
	require.Empty(t, deleted.CaptureAtlasNotes().Records)
	require.Equal(t, note.ID, deleted.CaptureAtlasNotes().LastID)
	require.Len(t, state.CaptureAtlasNotes().Records, 1, "different hydrated checkpoints remain isolated")
}

func TestSaveSnapshotV24FleetParkedAndOccupiedContinuation(t *testing.T) {
	state, _ := newExitFidelityState(t, references.Position{X: 50, Y: 50})
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	setAllExitTerrain(state, indexes.Water1)
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, state.MapState.PlayerLocation.Position, 0)
	parked, result := state.exitVehicle()
	require.Equal(t, exitVehicleSuccess, result)
	shipID, skiffID := parked.GetVehicleDetails().VesselID, state.PartyVehicle.GetVehicleDetails().VesselID
	require.NotZero(t, shipID)
	require.NotZero(t, skiffID)
	require.NotEqual(t, shipID, skiffID)
	restored, checkpoint := roundTripPersonalSnapshot(t, state)
	require.Equal(t, skiffID, checkpoint.PartyVehicle.VesselID)
	entries := atlasFleetEntries(restored)
	require.Len(t, entries, 1)
	require.Equal(t, shipID, entries[0].Vessel.ID)
	require.Equal(t, state.CaptureAtlasFleet(), restored.CaptureAtlasFleet())
	require.Equal(t, state.GetRNG().Calls(), restored.GetRNG().Calls())
	setAllExitTerrain(restored, indexes.Grass)
	leftSkiff, result := restored.exitVehicle()
	require.Equal(t, exitVehicleSuccess, result)
	require.Equal(t, skiffID, leftSkiff.GetVehicleDetails().VesselID)
	parkedState, _ := roundTripPersonalSnapshot(t, restored)
	require.Len(t, atlasFleetEntries(parkedState), 2)
	// A legacy/native historical occupied boat remains unregistered on load,
	// and can qualify only on a subsequent committed local exit.
	checkpoint.PartyVehicle.VesselID = 0
	checkpoint.AtlasFleet.LastID = shipID
	old, err := NewGameStateFromSaveSnapshot(checkpoint, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	require.NoError(t, err)
	require.Zero(t, old.PartyVehicle.GetVehicleDetails().VesselID)
	require.Equal(t, shipID, old.CaptureAtlasFleet().LastID)
	old.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	old.SetupEnvironmentalSystemCallbacks()
	setAllExitTerrain(old, indexes.Grass)
	newlyRegistered, result := old.exitVehicle()
	require.Equal(t, exitVehicleSuccess, result)
	require.Greater(t, newlyRegistered.GetVehicleDetails().VesselID, shipID)
}

func TestSaveSnapshotV24BasementChestLifetimesAndMidnightContinuation(t *testing.T) {
	state, _ := basementFixture(t)
	state.DateTime = datetime.UltimaDate{Year: 200, Month: 13, Day: 28, Hour: 23, Minute: 59}
	state.initializeMoongateStage()
	openedOn := state.DateTime.DateOnly()
	openBasementSource(t, state, 23)
	require.NotEqual(t, openedOn, state.DateTime.DateOnly(), "Open crosses midnight")
	require.NotEmpty(t, state.pendingProvisionHours)
	committedTime, committedLocation := state.DateTime, state.MapState.PlayerLocation
	state.ProcessPendingProvisionHours()
	require.Empty(t, state.pendingProvisionHours, "finish the direct Open's deferred provision boundary before capture")
	require.Equal(t, committedTime, state.DateTime, "provision processing must not charge another command")
	require.Equal(t, committedLocation, state.MapState.PlayerLocation, "provision processing is not basement entry")
	survivor := basementSourceChest(state, 24)
	require.NotNil(t, survivor)
	survivor.RestoreSurfaceChestNumber(0x80 | 12)
	survivor.RestoreSurfaceChestAssessment(references.ChestTrapped)
	state.BasementChestHistory[1].LastOpened = &datetime.BritannianDate{Year: 200, Month: 13, Day: 27}
	// A generated child reuses an unrelated dead NPC number and shares its
	// square with the parent's already-generated, only partially collected loot.
	units := state.CurrentNPCAIController.GetNpcs()
	units.RemoveNPCByNumber(26)
	state.PartyState.SetDeadNpc(references.LordBritishsCastle, 26)
	child := map_units.NewTransientSurfaceChest(references.LordBritishsCastle,
		references.Position{X: 16, Y: 21}, references.Basement, 26, 7)
	child.RestoreSurfaceChestAssessment(references.ChestSafe)
	*units = append(*units, child)
	restored, checkpoint := roundTripPersonalSnapshot(t, state)
	require.Nil(t, basementSourceChest(restored, 23), "load is continuation, not next-day arrival")
	require.Equal(t, openedOn, *restored.BasementChestHistory[0].LastOpened)
	require.Equal(t, references.ChestTrapped, basementSourceChest(restored, 24).SurfaceChestAssessment())
	var restoredChild *map_units.NPCFriendly
	for _, unit := range *restored.CurrentNPCAIController.GetNpcs() {
		if friendly, ok := unit.(*map_units.NPCFriendly); ok && friendly.IsTransientSurfaceChest() && unit.MapUnitDetails().NPCNum == 26 {
			restoredChild = friendly
		}
	}
	require.NotNil(t, restoredChild, "unrelated death flag cannot suppress a generated child")
	require.False(t, restoredChild.IsAuthoredSurfaceChest())
	require.Equal(t, references.ChestSafe, restoredChild.SurfaceChestAssessment())
	require.True(t, restored.PartyState.IsNPCDead(references.LordBritishsCastle, 26))
	checkpoint.BasementChestHistory[0].LastOpened.Day = 1
	require.Equal(t, openedOn, *restored.BasementChestHistory[0].LastOpened)
	require.True(t, restored.GoUpOneFloor())
	require.True(t, restored.GoDownOneFloor())
	require.NotNil(t, basementSourceChest(restored, 23), "real re-entry can replace the next-day chest")
}

func TestSaveSnapshotV24BasementHistoryOutsideCastle(t *testing.T) {
	state, _ := basementFixture(t)
	state.initializeMoongateStage()
	openBasementSource(t, state, 23)
	date := *state.BasementChestHistory[0].LastOpened
	state.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 15}, TransitionPositionOnly, false)
	restored, checkpoint := roundTripPersonalSnapshot(t, state)
	require.Equal(t, date, *restored.BasementChestHistory[0].LastOpened)
	state.BasementChestHistory[0].LastOpened.Day++
	require.Equal(t, int(date.Day), checkpoint.BasementChestHistory[0].LastOpened.Day, "capture date is detached")
	restored.TransitionToLocation(references.LordBritishsCastle, references.Basement, references.Position{X: 15, Y: 21}, TransitionPositionOnly, false)
	require.Nil(t, basementSourceChest(restored, 23), "saving outside cannot bypass the consumed day")
	require.NotNil(t, basementSourceChest(restored, 24))
}

func TestSaveSnapshotV24CodexPartialAndComplete(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	date := &save_snapshot.BritannianDateSnapshot{Year: 139, Month: 4, Day: 5}
	snapshot.Redux.PlayerKnowledge = []save_snapshot.PlayerKnowledgeSnapshot{{Category: save_snapshot.KnowledgeCategoryDungeon,
		Key: save_snapshot.DungeonKnowledgeDoomWordOfPower, DiscoveredOn: date,
		Source: save_snapshot.KnowledgeSourceSnapshot{Kind: save_snapshot.KnowledgeSourceRevelation, DisplayName: "Codex of Ultimate Wisdom"}}}
	snapshot.Redux.QuestJournal = []save_snapshot.QuestJournalEventSnapshot{{Sequence: 1,
		Kind: save_snapshot.QuestJournalEventKnowledgeLearned, Subject: "dungeon.doom.word_of_power", OccurredOn: date,
		Source: save_snapshot.QuestJournalSourceRevelation, SourceName: "Codex of Ultimate Wisdom", Acknowledged: true}}
	for _, complete := range []bool{false, true} {
		snapshot.CodexRevelationComplete = complete
		restored, hydrateErr := NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
		require.NoError(t, hydrateErr)
		restored.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
		restored.SetupEnvironmentalSystemCallbacks()
		again, captured := roundTripPersonalSnapshot(t, restored)
		require.Equal(t, complete, again.CodexRevelationComplete)
		require.Equal(t, snapshot.Redux.PlayerKnowledge, captured.Redux.PlayerKnowledge)
		require.Equal(t, snapshot.Redux.QuestJournal, captured.Redux.QuestJournal, "enrichment preserves the existing event and discovery provenance")
	}
}

func TestSaveSnapshotV24LegacyBasementResumePreservesActiveAbsence(t *testing.T) {
	state, _ := basementFixture(t)
	bundle, err := legacy_save.ReadBundleContext(t.Context(), filepath.Join("..", "legacy_save", "testdata", "b_carpet", "SAVED.GAM"))
	require.NoError(t, err)
	bundle.Game.Position.Location = legacy_save.MapID(references.LordBritishsCastle)
	bundle.Game.Position.X, bundle.Game.Position.Y, bundle.Game.Position.Floor = 15, 21, 0xff
	bundle.Game.PlayerShape = 0x1c
	for i := range bundle.Game.ActiveObjects {
		bundle.Game.ActiveObjects[i] = legacy_save.ObjectRecord{}
	}
	bundle.Game.ActiveObjects[7] = legacy_save.ObjectRecord{Tile: 1, Shape: 1, X: 17, Y: 22, Level: 0xff, Number: 0x9e}
	bundle.Game.ActiveObjects[8] = legacy_save.ObjectRecord{Tile: 1, Shape: 1, X: 16, Y: 21, Level: 0xff, Number: 7}
	converted, err := legacy_save.Convert(bundle)
	require.NoError(t, err)
	restored, err := NewGameStateFromSaveSnapshot(converted.Snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	require.NoError(t, err)
	restored.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	restored.SetupEnvironmentalSystemCallbacks()
	require.Nil(t, basementSourceChest(restored, 23))
	require.Nil(t, basementSourceChest(restored, 25))
	survivor := basementSourceChest(restored, 24)
	require.NotNil(t, survivor)
	number, ok := survivor.SurfaceChestNumber()
	require.True(t, ok)
	require.Equal(t, byte(0x9e), number)
	require.Equal(t, references.ChestUninspected, survivor.SurfaceChestAssessment())
	child := restored.CurrentNPCAIController.GetNpcs().GetFirstFriendlyMatching(func(chest *map_units.NPCFriendly) bool {
		return chest.IsTransientSurfaceChest() && chest.Pos() == (references.Position{X: 16, Y: 21})
	})
	require.NotNil(t, child, "a known generated value must never inherit its parent's daily identity")
	number, ok = child.SurfaceChestNumber()
	require.True(t, ok)
	require.Equal(t, byte(7), number)
	require.Equal(t, restored.DateTime.DateOnly(), *restored.BasementChestHistory[0].LastOpened)
	again, _ := roundTripPersonalSnapshot(t, restored)
	require.Nil(t, basementSourceChest(again, 23))
	require.True(t, again.GoUpOneFloor())
	require.True(t, again.GoDownOneFloor())
	require.Nil(t, basementSourceChest(again, 23), "legacy consumption blocks same-day re-entry")
	// Advancing the calendar while staying in the room still cannot turn a
	// subsequent native resume into a replenishment trigger.
	advanceSnapshotFixtureOneDay(t, again)
	nextDay, _ := roundTripPersonalSnapshot(t, again)
	require.Nil(t, basementSourceChest(nextDay, 23))
	require.True(t, nextDay.GoUpOneFloor())
	require.True(t, nextDay.GoDownOneFloor())
	require.NotNil(t, basementSourceChest(nextDay, 23))
}

func TestSaveSnapshotReconstructionCannotRecreateConsumedOffFloorChest(t *testing.T) {
	state, _ := basementFixture(t)
	state.initializeMoongateStage()
	require.True(t, state.GoUpOneFloor())
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	// Legacy-import reconstruction state: an absent authored chest has become
	// a saved-date ledger entry, and no active units were persisted.
	snapshot.MapUnits, snapshot.ItemStacks, snapshot.MapMutations = nil, nil, nil
	snapshot.MarkMapUnitsForReconstruction()
	date := snapshot.World.Calendar
	snapshot.BasementChestHistory[0].LastOpened = &save_snapshot.BritannianDateSnapshot{Year: date.Year, Month: date.Month, Day: date.Day}
	restored, err := NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	require.NoError(t, err)
	restored.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	restored.SetupEnvironmentalSystemCallbacks()
	require.Nil(t, basementSourceChest(restored, 23), "controller population must not reconstruct the consumed off-floor chest")
	require.NotNil(t, basementSourceChest(restored, 24), "unconsumed historical definitions remain reconstructible")
	require.True(t, restored.GoDownOneFloor())
	require.Nil(t, basementSourceChest(restored, 23), "same-day descent cannot retain a reconstructed replacement")
	advanceSnapshotFixtureOneDay(t, restored)
	again, _ := roundTripPersonalSnapshot(t, restored)
	require.Nil(t, basementSourceChest(again, 23), "midnight and native resume are not entry triggers")
	require.True(t, again.BasementChestHistory[0].eligible(again.DateTime.DateOnly()), "the next-day entry must actually be eligible")
	require.True(t, again.canPlaceBasementChest(references.LordBritishBasementChests()[0].Position, nil), "fixture must leave the source square free for replenishment")
	require.True(t, again.GoUpOneFloor())
	require.True(t, again.GoDownOneFloor())
	require.NotNil(t, basementSourceChest(again, 23), "a later real entry remains eligible")
}

func advanceSnapshotFixtureOneDay(t *testing.T, state *GameState) {
	t.Helper()
	before, location := state.DateTime, state.MapState.PlayerLocation
	// UltimaDate.Advance clamps each call to nine hours. Fixture-only calendar
	// changes use bounded steps, without inventing a location-entry event.
	for range 3 {
		state.DateTime.Advance(8 * datetime.MinutesPerHour)
	}
	require.Equal(t, before.GetTotalDays()+1, state.DateTime.GetTotalDays())
	require.Equal(t, before.Hour, state.DateTime.Hour)
	require.Equal(t, before.Minute, state.DateTime.Minute)
	require.Equal(t, location, state.MapState.PlayerLocation)
	state.initializeMoongateStage()
}

func TestSaveSnapshotV24CodexFinalAcknowledgementKeepsFirstProvenance(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	for _, source := range []string{save_snapshot.KnowledgeSourceDialogue, save_snapshot.KnowledgeSourceObservation} {
		t.Run(source, func(t *testing.T) {
			snapshot, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			date := &save_snapshot.BritannianDateSnapshot{Year: 139, Month: 4, Day: 5}
			snapshot.Redux.PlayerKnowledge = []save_snapshot.PlayerKnowledgeSnapshot{{Category: save_snapshot.KnowledgeCategoryDungeon,
				Key: save_snapshot.DungeonKnowledgeDoomWordOfPower, DiscoveredOn: date,
				Source: save_snapshot.KnowledgeSourceSnapshot{Kind: source, DisplayName: "First word source"}}}
			snapshot.Redux.QuestJournal = []save_snapshot.QuestJournalEventSnapshot{{Sequence: 1,
				Kind: save_snapshot.QuestJournalEventKnowledgeLearned, Subject: "dungeon.doom.word_of_power", OccurredOn: date,
				Source: source, SourceName: "First word source", Acknowledged: true}}
			restored, err := NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
			require.NoError(t, err)
			restored.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
			restored.SetupEnvironmentalSystemCallbacks()
			require.False(t, restored.CodexRevelationComplete)
			require.True(t, restored.RecordCompleteCodexRevelation())
			require.False(t, restored.RecordCompleteCodexRevelation())
			again, captured := roundTripPersonalSnapshot(t, restored)
			require.True(t, again.CodexRevelationComplete)
			require.Equal(t, snapshot.Redux.PlayerKnowledge, captured.Redux.PlayerKnowledge)
			require.Equal(t, snapshot.Redux.QuestJournal, captured.Redux.QuestJournal)
		})
	}
}
