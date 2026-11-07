package game_state

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestValidateSaveSnapshotCompatibilityUsesDetachedReferences(t *testing.T) {
	snapshot, gameReferences := detachedCompatibilityFixture()

	if err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences); err != nil {
		t.Fatalf("ValidateSaveSnapshotCompatibility() error = %v", err)
	}
}

func TestSaveSnapshotHydrationNormalizesStaleInQuasXenRecipe(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	canonical := references.GetSpellReagents(references.InQuasXen)
	stale := canonical | 1<<uint(references.NightShade)
	state.MagicState.RecordRecipe(references.InQuasXen, stale)
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.Equal(t, stale, snapshot.Progression.Recipes[references.InQuasXen].Mask)

	restored, err := NewGameStateFromSaveSnapshot(
		snapshot,
		state.GameConfig,
		state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen,
		state.MapState.YTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	mask, known := restored.MagicState.KnownRecipes[references.InQuasXen]
	assert.True(t, known)
	assert.Equal(t, canonical, mask)

	recaptured, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.True(t, recaptured.Progression.Recipes[references.InQuasXen].Known)
	assert.Equal(t, canonical, recaptured.Progression.Recipes[references.InQuasXen].Mask)
}

func TestValidateSaveSnapshotCompatibilityChecksSelectedPlayerAvailability(t *testing.T) {
	snapshot, gameReferences := detachedCompatibilityFixture()
	snapshot.Continuation.ActivePlayerIndex = 0
	snapshot.Party.Roster[0].CurrentHP = 30
	snapshot.Party.Roster[0].PartyMap = int(party_state.PartyMapActive)
	require.NoError(t, ValidateSaveSnapshotCompatibility(snapshot, gameReferences))

	tests := []struct {
		name   string
		mutate func(*save_snapshot.CharacterSnapshot)
	}{
		{name: "vacant", mutate: func(character *save_snapshot.CharacterSnapshot) { character.Occupied = false }},
		{name: "inactive", mutate: func(character *save_snapshot.CharacterSnapshot) {
			character.PartyMap = int(party_state.PartyMapUnrecruited)
		}},
		{name: "dead", mutate: func(character *save_snapshot.CharacterSnapshot) { character.Status = int(party_state.Dead) }},
		{name: "sleeping", mutate: func(character *save_snapshot.CharacterSnapshot) { character.Status = int(party_state.Sleep) }},
		{name: "zero hp", mutate: func(character *save_snapshot.CharacterSnapshot) { character.CurrentHP = 0 }},
		{name: "unnamed", mutate: func(character *save_snapshot.CharacterSnapshot) { character.Name = "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			invalid := snapshot
			test.mutate(&invalid.Party.Roster[0])

			assert.Error(t, ValidateSaveSnapshotCompatibility(invalid, gameReferences))
		})
	}
}

func TestNPCFlagSnapshotKeepsNativeV5ZeroBasedWireLocations(t *testing.T) {
	flags := map[references.Location][]bool{
		references.Moonglow:     make([]bool, party_state.NPcsPerLocation),
		references.SerpentsHold: make([]bool, party_state.NPcsPerLocation),
	}
	flags[references.Moonglow][0] = true
	flags[references.SerpentsHold][party_state.NPcsPerLocation-1] = true

	wire := flattenNPCFlags(flags)

	assert.Equal(t, []save_snapshot.NPCFlagSnapshot{
		{Location: 0, NPC: 0},
		{Location: 31, NPC: party_state.NPcsPerLocation - 1},
	}, wire)
	roundTrip := expandNPCFlags(wire)
	assert.True(t, roundTrip[references.Moonglow][0])
	assert.True(t, roundTrip[references.SerpentsHold][party_state.NPcsPerLocation-1])
}

func TestNewGameStateFromSaveSnapshotRejectsCompatibilityErrorsBeforeHydration(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	snapshot.World.LastLargeMapPosition.Location = int(references.Britain)

	if _, err = NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen); err == nil {
		t.Fatal("NewGameStateFromSaveSnapshot() hydrated an incompatible last large-map position")
	}
}

func TestValidateSaveSnapshotCompatibilityRejectsIncompatibleLastLargeMapPosition(t *testing.T) {
	snapshot, gameReferences := detachedCompatibilityFixture()
	snapshot.World.LastLargeMapPosition.Location = int(references.Britain)

	err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences)
	if err == nil {
		t.Fatal("ValidateSaveSnapshotCompatibility() accepted a small-map last large-map position")
	}
}

func TestValidateSaveSnapshotCompatibilityRejectsAdversarialWindPhases(t *testing.T) {
	for _, test := range []struct {
		name       string
		elapsedMs  int64
		nextTickMs int64
	}{
		{name: "far behind", elapsedMs: 1_000_000_000_000, nextTickMs: 55},
		{name: "equal to elapsed", elapsedMs: 1_000, nextTickMs: 1_000},
		{name: "too far ahead", elapsedMs: 1_000, nextTickMs: 1_056},
		{name: "overflow reproduction", elapsedMs: math.MaxInt64 - 100, nextTickMs: math.MaxInt64 - 99},
		{name: "above persisted domain", elapsedMs: save_snapshot.MaxElapsedMs + 1, nextTickMs: save_snapshot.MaxElapsedMs + 2},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot, gameReferences := detachedCompatibilityFixture()
			snapshot.World.ElapsedMs = test.elapsedMs
			snapshot.Wind.NextTickMs = test.nextTickMs

			if err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences); err == nil {
				t.Fatal("ValidateSaveSnapshotCompatibility() accepted an invalid wind phase")
			}
		})
	}
}

func TestSaveSnapshotElapsedLimitPreservesWindPhaseAndRandom(t *testing.T) {
	snapshot, gameReferences := detachedCompatibilityFixture()
	snapshot.World.ElapsedMs = save_snapshot.MaxElapsedMs
	snapshot.Wind.NextTickMs = save_snapshot.MaxElapsedMs + environment.WindTickIntervalMs
	before := snapshot
	require.NoError(t, ValidateSaveSnapshotCompatibility(snapshot, gameReferences))
	assert.Equal(t, before, snapshot)
}

func TestSaveSnapshotElapsedLimitHydrationContinues(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).WithSystemCallbacks().WithRandomSeed(12345).Build()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	for _, phase := range []int64{1, environment.WindTickIntervalMs} {
		t.Run(fmt.Sprintf("phase/%d", phase), func(t *testing.T) {
			var normalWind environment.WindSnapshot
			var normalRandom save_snapshot.RandomSnapshot
			for _, offset := range []int64{0, save_snapshot.MaxElapsedMs} {
				input := snapshot
				input.World.ElapsedMs, input.Wind.NextTickMs = offset, offset+phase
				restored, err := NewGameStateFromSaveSnapshot(input, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
				require.NoError(t, err)
				restored.SystemCallbacks = state.SystemCallbacks
				restored.SetupEnvironmentalSystemCallbacks()
				recaptured, err := restored.CaptureSaveSnapshot()
				require.NoError(t, err)
				require.Equal(t, input, recaptured, "hydration must not rebase clock or wind phase")
				for _, elapsed := range []int64{60, 5500} {
					restored.ElapsedMs = offset + elapsed
					restored.windSystem.AdvanceThrough(restored.ElapsedMs)
					require.Greater(t, restored.windSystem.Snapshot().NextTickMs, restored.ElapsedMs)
				}
				wind := restored.windSystem.Snapshot()
				wind.NextTickMs -= offset
				random := save_snapshot.RandomSnapshot{Seed: restored.GetRNG().Seed(), SourceCalls: restored.GetRNG().Calls()}
				if offset == 0 {
					normalWind, normalRandom = wind, random
				} else {
					assert.Equal(t, normalWind, wind)
					assert.Equal(t, normalRandom, random)
				}
			}
		})
	}
}

func TestSaveSnapshotMutationReferencesAndSentinels(t *testing.T) {
	for _, kind := range []save_snapshot.MapMutationKind{save_snapshot.MapMutationSetTile, save_snapshot.MapMutationSetObject} {
		for _, value := range []int{-2, -1, 0, 1, int(indexes.RegularDoor), 999999} {
			t.Run(fmt.Sprintf("%s/%d", kind, value), func(t *testing.T) {
				snapshot, gameReferences := detachedCompatibilityFixture()
				// Deliberately sparse: identity lookup, not a numeric upper bound.
				registry := references.NewTileReferences()
				tiles := references.Tiles{
					indexes.NoSprites:   registry.GetTile(indexes.NoSprites),
					0:                   registry.GetTile(0),
					indexes.RegularDoor: registry.GetTile(indexes.RegularDoor),
				}
				gameReferences.TileReferences = &tiles
				snapshot.MapMutations = []save_snapshot.MapMutationSnapshot{{Position: snapshot.World.Position, Kind: kind, Value: value}}
				err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences)
				if value == int(indexes.RegularDoor) || value == -1 || value == 0 {
					require.NoError(t, err)
					delete(tiles, indexes.SpriteIndex(value))
					err = ValidateSaveSnapshotCompatibility(snapshot, gameReferences)
					if kind == save_snapshot.MapMutationSetObject && (value == -1 || value == 0) {
						require.NoError(t, err, "empty overrides are not installed")
					} else {
						require.ErrorContains(t, err, "tile reference", "installed tiles must resolve even for -1 and 0")
					}
				} else {
					require.Error(t, err)
					_, err = NewGameStateFromSaveSnapshot(snapshot, &config.UltimaVConfiguration{}, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
					require.ErrorContains(t, err, "mapMutations", "reject before hydration uses incomplete runtime dependencies")
				}
			})
		}
	}
}

func TestSaveSnapshotMutationLayersRoundTrip(t *testing.T) {
	for _, kind := range []save_snapshot.MapMutationKind{save_snapshot.MapMutationSetTile, save_snapshot.MapMutationSetObject} {
		for _, value := range []indexes.SpriteIndex{indexes.RegularDoor, 0, indexes.NoSprites} {
			t.Run(fmt.Sprintf("%s/%d", kind, value), func(t *testing.T) {
				state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
				position := references.Position{X: 1, Y: 1}
				layer := map_state.MapLayer
				layeredMap := state.GetLayeredMapByCurrentLocation()
				if kind == save_snapshot.MapMutationSetObject {
					layer = map_state.MapOverrideLayer
					layeredMap.SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
				} else {
					require.NotEqual(t, value, state.baseTile(references.Britain, state.MapState.PlayerLocation.Floor, position))
				}
				layeredMap.SetTileByLayer(layer, &position, value)
				require.Equal(t, value, layeredMap.GetTileByLayer(layer, &position).Index)

				before, err := state.CaptureSaveSnapshot()
				require.NoError(t, err)
				mutation := save_snapshot.MapMutationSnapshot{
					Position: snapshotPositionAt(before.World.Position, int(position.X), int(position.Y)),
					Kind:     kind, Value: int(value),
				}
				input := before
				if kind == save_snapshot.MapMutationSetObject && value <= 0 {
					require.NotContains(t, before.MapMutations, mutation)
					// Older snapshots explicitly stored empty override cells.
					input.MapMutations = append(input.MapMutations, mutation)
				} else {
					require.Contains(t, before.MapMutations, mutation)
				}
				require.NoError(t, state.ValidateSaveSnapshotCompatibility(input))
				restored, err := NewGameStateFromSaveSnapshot(input, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
				require.NoError(t, err)
				if kind == save_snapshot.MapMutationSetTile || value > 0 {
					assert.Equal(t, value, restored.GetLayeredMapByCurrentLocation().GetTileByLayer(layer, &position).Index)
				}
				after, err := restored.CaptureSaveSnapshot()
				require.NoError(t, err)
				assert.Equal(t, before, after)
			})
		}
	}
}

func TestSaveSnapshotRejectsUnsupportedControllerGroups(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	before, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.NoError(t, state.ValidateSaveSnapshotCompatibility(before))
	for _, test := range []struct {
		name     string
		location references.Location
		world    int
		floor    int
	}{
		{"inactive town", references.Moonglow, 0, 0},
		{"town wrong world", references.Britain, -1, 0},
		{"town nonexistent floor", references.Britain, 0, 99},
		{"overworld wrong floor", references.BritanniaUnderworld, 0, -1},
		{"underworld wrong floor", references.BritanniaUnderworld, -1, 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			snapshot := before
			chest := 0
			snapshot.MapUnits = []save_snapshot.MapUnitSnapshot{{
				Kind: save_snapshot.MapUnitKindChest, World: test.world,
				Position:           save_snapshot.PositionSnapshot{Location: int(test.location), X: 10, Y: 10, Floor: test.floor},
				SurfaceChestNumber: &chest, AIOverrideProvenance: string(references.AIOverrideProvenanceNone),
			}}
			require.Error(t, state.ValidateSaveSnapshotCompatibility(snapshot))
			_, err := NewGameStateFromSaveSnapshot(snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
			require.Error(t, err)
		})
	}
	after, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, before, after)
	// Defense in depth if a future hydration caller bypasses compatibility.
	require.ErrorContains(t, state.restoreMapUnits([]save_snapshot.MapUnitSnapshot{{Position: save_snapshot.PositionSnapshot{Location: int(references.Moonglow)}}}, false), "unconsumed controller groups")
}

func TestSaveSnapshotAllAcceptedControllerGroupsRoundTrip(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithPlayerAt(15, 15).WithSystemCallbacks().Build()
	for _, world := range []references.World{references.OVERWORLD, references.UNDERWORLD} {
		vehicle := map_units.NewNPCFriendlyVehiceNewRef(references.SkiffVehicle, references.Position{X: 10, Y: 10}, references.FloorNumber(world))
		state.LargeMapNPCAIController[world].GetNpcs().ReplaceWith([]map_units.MapUnit{vehicle})
	}
	before, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	groups := make(map[[2]int]bool)
	for _, unit := range before.MapUnits {
		groups[[2]int{unit.Position.Location, unit.World}] = true
	}
	require.Len(t, groups, 3, "both worlds and the active town must be represented")
	require.NoError(t, state.ValidateSaveSnapshotCompatibility(before))
	restored, err := NewGameStateFromSaveSnapshot(before, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	require.NoError(t, err)
	after, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestValidateSaveSnapshotCompatibilityRequiresCanonicalPendingDeliveryDock(t *testing.T) {
	docks := []references.DockReference{
		{Location: references.Jhelom, Position: references.Position{X: 11, Y: 21}},
		{Location: references.Minoc, Position: references.Position{X: 12, Y: 22}},
		{Location: references.EastBritanny, Position: references.Position{X: 13, Y: 23}},
		{Location: references.BuccaneersDen, Position: references.Position{X: 14, Y: 24}},
	}
	for _, dock := range docks {
		t.Run(dock.Location.String(), func(t *testing.T) {
			snapshot, gameReferences := detachedCompatibilityFixture()
			gameReferences.DockReferences = &references.DockReferences{Docks: docks}
			snapshot.World.Position.Floor = int(references.UNDERWORLD)
			snapshot.World.LastLargeMapPosition = snapshot.World.Position
			snapshot.MapUnits = []save_snapshot.MapUnitSnapshot{{
				World: int(references.OVERWORLD), Kind: "vehicle", Position: save_snapshot.PositionSnapshot{
					Location: int(references.BritanniaUnderworld), X: int(dock.Position.X), Y: int(dock.Position.Y), Floor: int(references.OVERWORLD),
				}, Visible: true, AIOverrideProvenance: string(references.AIOverrideProvenanceNone), Vehicle: &save_snapshot.VehicleSnapshot{
					Type: int(references.FrigateVehicle), CurrentDirection: int(references.Right),
					PreviousDirection: int(references.Right), Skiffs: 2, Hull: 99, PendingDelivery: true,
				},
			}}
			if err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences); err != nil {
				t.Fatalf("ValidateSaveSnapshotCompatibility() error = %v", err)
			}
		})
	}

	snapshot, gameReferences := detachedCompatibilityFixture()
	gameReferences.DockReferences = &references.DockReferences{Docks: docks}
	snapshot.World.Position.Floor = int(references.UNDERWORLD)
	snapshot.World.LastLargeMapPosition = snapshot.World.Position
	snapshot.MapUnits = []save_snapshot.MapUnitSnapshot{{
		World: int(references.OVERWORLD), Kind: "vehicle", Position: save_snapshot.PositionSnapshot{
			Location: int(references.BritanniaUnderworld), X: 15, Y: 25, Floor: int(references.OVERWORLD),
		}, Visible: true, AIOverrideProvenance: string(references.AIOverrideProvenanceNone), Vehicle: &save_snapshot.VehicleSnapshot{
			Type: int(references.SkiffVehicle), CurrentDirection: int(references.Right),
			PreviousDirection: int(references.Right), Hull: 99, PendingDelivery: true,
		},
	}}
	if err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences); err == nil {
		t.Fatal("ValidateSaveSnapshotCompatibility() accepted a non-canonical pending-delivery position")
	}
	gameReferences.DockReferences.Docks = append(gameReferences.DockReferences.Docks, references.DockReference{
		Location: references.Jhelom,
		Position: references.Position{X: 15, Y: 25},
	})
	if err := ValidateSaveSnapshotCompatibility(snapshot, gameReferences); err == nil {
		t.Fatal("ValidateSaveSnapshotCompatibility() accepted a duplicate non-canonical dock record")
	}
}

func TestSaveSnapshotVehiclePendingDeliveryRoundTrip(t *testing.T) {
	source := map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, references.Position{}, references.FloorNumber(references.OVERWORLD))
	source.GetVehicleDetails().MarkPendingDelivery()
	saved := captureVehicle(source)
	if saved == nil || !saved.PendingDelivery {
		t.Fatalf("captureVehicle() = %#v, want pending delivery", saved)
	}

	restored := map_units.NewVehicleDetails(references.FrigateVehicle)
	restoreVehicle(&restored, *saved)
	if !restored.IsPendingDelivery() {
		t.Fatal("restoreVehicle() did not restore pending delivery")
	}
}

func TestSaveSnapshotVehicleHorseRefusalRoundTrip(t *testing.T) {
	for _, owned := range []bool{false, true} {
		source := map_units.NewNPCFriendlyVehiceNewRef(
			references.HorseVehicle,
			references.Position{X: 10, Y: 10},
			references.MainFloor,
		)
		source.GetVehicleDetails().IsOwnedByNPC = owned

		saved := captureVehicle(source)
		restored := map_units.NewVehicleDetails(references.HorseVehicle)
		restoreVehicle(&restored, *saved)

		assert.Equal(t, owned, restored.IsOwnedByNPC)
	}
}

func TestCaptureEffectsIncludesPrivateIntoxicationState(t *testing.T) {
	state := &GameState{drunk: 3, smashed: 25}
	var snapshot save_snapshot.SaveSnapshot

	state.captureEffects(&snapshot)

	if snapshot.Effects.Drunk != 3 || snapshot.Effects.Smashed != 25 {
		t.Fatalf("captured intoxication = drunk %d smashed %d, want 3/25", snapshot.Effects.Drunk, snapshot.Effects.Smashed)
	}
}

func TestPlayerKnowledgeSnapshotConversionIsDetachedAndExact(t *testing.T) {
	date := datetime.BritannianDate{Year: 105, Month: 13, Day: 28}
	state := &GameState{
		DateTime:        datetime.UltimaDate{Year: 105, Month: 13, Day: 28},
		SystemCallbacks: journalTestCallbacks(),
		playerKnowledge: newPlayerKnowledge(),
		questJournal:    newQuestJournal(),
	}
	require.True(t, state.DiscoverShadowlordKnowledge(
		references.Astaroth,
		ShadowlordKnowledgeName,
		DialogueKnowledgeSource("Savant"),
	))
	require.True(t, state.DiscoverShrineMantraKnowledge(0, DialogueKnowledgeSource("Malifora")))
	state.playerKnowledge.entries[KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeRelXenBet}] = KnowledgeEntry{
		Key:    KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeRelXenBet},
		Source: KnowledgeSource{Kind: KnowledgeSourceMigration, DisplayName: migratedKnowledgeDisplayName},
	}
	state.playerKnowledge.entries[KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeAnYlem}] = KnowledgeEntry{
		Key:          KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeAnYlem},
		Source:       DialogueKnowledgeSource("Goeth"),
		DiscoveredOn: &date,
		Unread:       true,
		SpellFormula: &SpellFormulaKnowledge{
			State: SpellFormulaUncertain, CandidateMasks: []uint16{0x81, 0x14},
		},
	}
	state.playerKnowledge.entries[KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeInXenMani}] = KnowledgeEntry{
		Key:          KnowledgeKey{Category: KnowledgeCategorySpell, ID: spellKnowledgeInXenMani},
		Source:       ExperimentationKnowledgeSource(),
		DiscoveredOn: &date,
		SpellFormula: confirmedSpellFormula(references.InXenMani),
	}

	var captured save_snapshot.SaveSnapshot
	state.captureRedux(&captured)
	if len(captured.Redux.PlayerKnowledge) != 5 ||
		captured.Redux.PlayerKnowledge[0].Key != save_snapshot.ShadowlordKnowledgeAstarothName ||
		captured.Redux.PlayerKnowledge[1].Key != save_snapshot.ShrineKnowledgeHonestyMantra ||
		captured.Redux.PlayerKnowledge[2].Key != spellKnowledgeAnYlem ||
		captured.Redux.PlayerKnowledge[3].Key != spellKnowledgeInXenMani ||
		captured.Redux.PlayerKnowledge[4].Key != spellKnowledgeRelXenBet {
		t.Fatalf("captured player knowledge is not sorted: %#v", captured.Redux.PlayerKnowledge)
	}

	restored := &GameState{questJournal: newQuestJournal()}
	restored.restorePlayerKnowledge(captured.Redux.PlayerKnowledge)
	var recaptured save_snapshot.SaveSnapshot
	restored.captureRedux(&recaptured)
	if !reflect.DeepEqual(recaptured.Redux.PlayerKnowledge, captured.Redux.PlayerKnowledge) {
		t.Fatalf("player knowledge changed during conversion\nbefore: %#v\nafter:  %#v", captured.Redux.PlayerKnowledge, recaptured.Redux.PlayerKnowledge)
	}

	detached := restored.KnowledgeSnapshot()
	detached[0].DiscoveredOn.Day = 1
	detached[2].SpellFormula.CandidateMasks[0] = 1
	var afterMutation save_snapshot.SaveSnapshot
	restored.captureRedux(&afterMutation)
	if !reflect.DeepEqual(afterMutation.Redux.PlayerKnowledge, captured.Redux.PlayerKnowledge) {
		t.Fatal("mutating detached knowledge changed restored runtime state")
	}
	_, learned := restored.ShadowlordKnowledge(references.Astaroth, ShadowlordKnowledgeName)
	assert.True(t, learned)
	_, learned = restored.ShrineMantraKnowledge(0)
	assert.True(t, learned)
	assert.False(t, restored.DiscoverShadowlordKnowledge(
		references.Astaroth,
		ShadowlordKnowledgeName,
		DialogueKnowledgeSource("Later speaker"),
	), "hydrated discoveries remain idempotent")
}

func TestQuestJournalSnapshotConversionIsDetachedAndExact(t *testing.T) {
	state := &GameState{
		DateTime:        datetime.UltimaDate{Year: 139, Month: 13, Day: 28},
		SystemCallbacks: journalTestCallbacks(),
		playerKnowledge: newPlayerKnowledge(),
		questJournal:    newQuestJournal(),
	}
	if !state.RecordShadowlordDefeatEvent(references.Nosfentor) {
		t.Fatal("failed to record Nosfentor defeat")
	}

	var captured save_snapshot.SaveSnapshot
	state.captureRedux(&captured)
	if len(captured.Redux.QuestJournal) != 1 {
		t.Fatalf("captured quest journal = %#v", captured.Redux.QuestJournal)
	}
	assert.Equal(t, save_snapshot.QuestJournalSubjectNosfentor, captured.Redux.QuestJournal[0].Subject)

	restored := &GameState{playerKnowledge: newPlayerKnowledge()}
	restored.restoreQuestJournal(captured.Redux.QuestJournal)
	var recaptured save_snapshot.SaveSnapshot
	restored.captureRedux(&recaptured)
	assert.Equal(t, captured.Redux.QuestJournal, recaptured.Redux.QuestJournal)

	detached := restored.QuestJournalEvents()
	detached[0].OccurredOn.Day = 1
	var afterMutation save_snapshot.SaveSnapshot
	restored.captureRedux(&afterMutation)
	assert.Equal(t, byte(28), byte(afterMutation.Redux.QuestJournal[0].OccurredOn.Day))
}

func TestVersionSevenHydrationTreatsSavedPlayerKnowledgeAsAuthoritative(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	state := getBritain2GameState()
	state.playerKnowledge = newPlayerKnowledge()
	state.PartyState.Inventory.Spells.Set(references.RelXenBet, 2)
	state.MagicState.RecordRecipe(references.RelXenBet, references.GetSpellReagents(references.RelXenBet))
	if !state.DiscoverSpellKnowledge(references.AnYlem, DialogueKnowledgeSource("Goeth")) {
		t.Fatal("failed to arrange source player knowledge")
	}

	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	if len(snapshot.Redux.PlayerKnowledge) != 1 {
		t.Fatalf("captured knowledge = %#v, want only saved An Ylem entry", snapshot.Redux.PlayerKnowledge)
	}
	hydrated, err := NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}
	if _, inferred := hydrated.SpellKnowledge(references.RelXenBet); inferred {
		t.Fatal("v7 hydration inferred knowledge over the authoritative saved array")
	}
	recaptured, err := hydrated.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("hydrated CaptureSaveSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(recaptured.Redux.PlayerKnowledge, snapshot.Redux.PlayerKnowledge) {
		t.Fatalf("hydrated knowledge changed\nbefore: %#v\nafter:  %#v", snapshot.Redux.PlayerKnowledge, recaptured.Redux.PlayerKnowledge)
	}
}

func TestCaptureSaveSnapshotPreservesPendingDeliveryInDormantOverworldController(t *testing.T) {
	state := getBritain2GameState()
	if state == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.Britain,
		Floor:    0,
		Position: references.Position{X: 15, Y: 15},
	}
	state.initializeSnapshotMapsAndControllers()
	if state.MapState.PlayerLocation.Location.GetMapType() != references.SmallMapType {
		t.Fatal("test requires the party to be in town")
	}
	delivery := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		references.Position{X: 10, Y: 20},
		references.FloorNumber(references.OVERWORLD),
	)
	delivery.SetVisible(true)
	delivery.GetVehicleDetails().MarkPendingDelivery()
	state.LargeMapNPCAIController[references.OVERWORLD].GetNpcs().ReplaceWith([]map_units.MapUnit{delivery})

	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	for _, unit := range snapshot.MapUnits {
		if unit.Vehicle != nil && unit.Vehicle.PendingDelivery {
			return
		}
	}
	t.Fatal("CaptureSaveSnapshot() dropped pending delivery from dormant overworld controller")
}

func detachedCompatibilityFixture() (save_snapshot.SaveSnapshot, *references.GameReferences) {
	snapshot := save_snapshot.SaveSnapshot{
		Presentation: journal.EmptyPresentation(),
		Version:      save_snapshot.CurrentVersion,
		Continuation: save_snapshot.ContinuationSnapshot{ActivePlayerIndex: -1},
		Redux: save_snapshot.ReduxSnapshot{
			DungeonDiscoveries: make([]save_snapshot.DungeonDiscoverySnapshot, 0),
			PlayerKnowledge:    make([]save_snapshot.PlayerKnowledgeSnapshot, 0),
			QuestJournal:       make([]save_snapshot.QuestJournalEventSnapshot, 0),
		},
	}
	snapshot.Party.Roster[0] = save_snapshot.CharacterSnapshot{
		Occupied: true, Name: "Avatar", Gender: int(party_state.Male), Class: int(party_state.Avatar),
		Status: int(party_state.Good), Level: 1, MaximumHP: 30,
	}
	snapshot.World.Position = save_snapshot.PositionSnapshot{
		Location: int(references.BritanniaUnderworld), X: 10, Y: 20, Floor: int(references.OVERWORLD),
	}
	snapshot.World.LastLargeMapPosition = snapshot.World.Position
	snapshot.World.Calendar = save_snapshot.CalendarSnapshot{Year: 1, Month: 1, Day: 1}
	snapshot.Wind.NextTickMs = environment.WindTickIntervalMs
	snapshot.Progression.Shadowlords.LastCalled = -1
	snapshot.Progression.Shadowlords.AutomaticPresence = -1
	enemies := references.EnemyReferences{}

	return snapshot, &references.GameReferences{
		LocationReferences: &references.LocationReferences{},
		EnemyReferences:    &enemies,
	}
}

// TestSaveSnapshotRoundTripWithRealData guards against silently dropping runtime
// state while rebuilding maps and managers from an actual SAVED.GAM-backed state.
func TestSaveSnapshotRoundTripWithRealData(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	state.SetRandomSeed(8675309)
	state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	for range 17 {
		state.RollDice(30)
	}
	state.PartyState.Inventory.Provisions.SkullKeys.Set(7)
	state.PartyState.Inventory.Equipment.Set(references.Ankh, 2)
	state.PartyState.Inventory.Spells.Set(references.AnTym, 4)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollAnTym, 5)
	state.PartyState.Inventory.Potions.Set(references.White, 6)
	state.PartyState.Inventory.Reagent.Set(references.MandrakeRoot, 8)
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
	state.PartyState.Inventory.Moonstones.Set(references.CrescentWaning, 1)
	state.PartyState.SetMet(references.Britain, 7)
	state.PartyState.SetDeadNpc(references.Britain, 8)
	state.SearchManager.authenticLocationManager.markAsFound(29)
	state.SearchManager.authenticLocationManager.skullKeysFoundDay = 12
	state.SearchManager.reagentCachesFoundDay = [3]byte{2, 4, 6}
	state.MagicState.RecordRecipe(references.AnTym, references.GetSpellReagents(references.AnTym))
	state.SetShrineQuestActive(1, true)
	state.SetShrineQuestCompleted(2, true)
	state.SetShrineRuined(3, true)
	state.DungeonStates[4] = true
	state.SetHMSCapeActivated(true)
	state.LastCamped = 9
	state.LastWandererMonth = 5
	state.ElapsedMs = 12_345
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectQuickness, TurnsRemaining: 37}
	state.quicknessWorldActorSkip = true
	state.MagicLightMinutesRemaining = 83
	state.drunk = 3
	state.smashed = 25
	state.MapState.Lighting.SetMagicLight(true)
	state.MapState.Lighting.RestoreTorchMinutes(73)
	state.AmuletState = ToggleItemState{IsActive: true, TurnsRemaining: ToggleItemDuration}
	state.CrownState = ToggleItemState{}
	state.BadgeState = ToggleItemState{IsActive: true, TurnsRemaining: ToggleItemDuration}
	state.windSystem.Restore(environment.WindSnapshot{Direction: environment.East, UpdateCounter: 2, NextTickMs: 12_377})
	addSnapshotEnemyVariants(t, state)

	itemPosition := references.Position{X: state.MapState.PlayerLocation.Position.X + 1, Y: state.MapState.PlayerLocation.Position.Y}
	if err := state.ItemStacksMap.Push(&itemPosition, &references.ItemStack{Items: references.ItemStacks{{Item: references.Gold, Quantity: 23}, {Item: references.Ankh, Quantity: 1}}}); err != nil {
		t.Fatalf("placing test stack: %v", err)
	}
	mutationPosition := references.Position{X: 1, Y: 1}
	state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &mutationPosition, indexes.RegularDoor)

	before, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	for index, mutation := range before.MapMutations {
		if mutation.Kind == save_snapshot.MapMutationSetObject && mutation.Value <= 0 {
			t.Fatalf("map mutation %d captured empty override value %d", index, mutation.Value)
		}
	}
	hydrated, err := NewGameStateFromSaveSnapshot(before, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}
	after, err := hydrated.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("hydrated CaptureSaveSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("snapshot changed after hydration\nbefore: %#v\nafter:  %#v", before, after)
	}
	if !hydrated.MapState.Lighting.HasMagicLight() {
		t.Fatal("hydrated simultaneous active spell and magical light did not restore lighting")
	}
	if hydrated.MapState.Lighting.TorchMinutesRemaining() != 73 || hydrated.PartyState.Inventory.Provisions.Torches.Get() != state.PartyState.Inventory.Provisions.Torches.Get() {
		t.Fatal("hydrated carried torch fuel changed fuel or inventory")
	}
	if hydrated.drunk != 3 || hydrated.smashed != 25 {
		t.Fatalf("hydrated intoxication = drunk %d smashed %d, want 3/25", hydrated.drunk, hydrated.smashed)
	}
	if !hydrated.IsAmuletActive() || hydrated.IsCrownActive() || !hydrated.IsBadgeActive() {
		t.Fatal("hydrated native save did not preserve Amulet and Badge alongside Quickness without Crown")
	}
	for index := range 20 {
		if got, want := hydrated.RollDice(1000), state.RollDice(1000); got != want {
			t.Fatalf("RNG continuation roll %d = %d, want %d", index, got, want)
		}
	}
}

func TestSaveSnapshotRoundTripPreservesCrownOrSpellWithIndependentArtifacts(t *testing.T) {
	for _, crownActive := range []bool{false, true} {
		t.Run(fmt.Sprintf("crown=%t", crownActive), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.ToggleBadge()
			state.ToggleAmulet()
			if crownActive {
				state.ToggleCrown()
			} else {
				state.ActivateProtectionEffect(9)
			}
			snapshot, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			restored, err := NewGameStateFromSaveSnapshot(
				snapshot, state.GameConfig, state.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
			)
			require.NoError(t, err)
			assert.Equal(t, state.CrownState, restored.CrownState)
			assert.Equal(t, state.BadgeState, restored.BadgeState)
			assert.Equal(t, state.AmuletState, restored.AmuletState)
			assert.Equal(t, state.ActiveDurationEffect, restored.ActiveDurationEffect)
		})
	}
}

func TestSaveSnapshotRoundTripPreservesContinuationState(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()
	state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle, state.MapState.PlayerLocation.Position, state.MapState.PlayerLocation.Floor,
	)
	state.PartyVehicle.GetVehicleDetails().SetSailState(map_units.SailRaised)
	state.PartyVehicle.GetVehicleDetails().SetHullHP(77)
	require.True(t, state.SetActivePlayer(0))
	state.isCriminal = true
	state.hmsCapeAutomaticWaitActorSkip = true
	state.beggarKarmaCounter = 137
	state.openingCampaignEncounterReduction = true
	state.combatActionTimeCounter = 5
	state.SetHMSCapeActivated(true)

	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.Equal(t, save_snapshot.ContinuationSnapshot{
		ActivePlayerIndex:                 0,
		IsCriminal:                        true,
		HMSCapeAutomaticWaitActorSkip:     true,
		BeggarKarmaCounter:                137,
		OpeningCampaignEncounterReduction: true,
		CombatActionTimeCounter:           5,
	}, snapshot.Continuation)

	restored, err := NewGameStateFromSaveSnapshot(
		snapshot,
		state.GameConfig,
		state.GameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	assert.Equal(t, 0, restored.GetActivePlayerIndex())
	assert.True(t, restored.IsCriminal())
	assert.True(t, restored.hmsCapeAutomaticWaitActorSkip)
	assert.Equal(t, 137, restored.beggarKarmaCounter)
	assert.True(t, restored.openingCampaignEncounterReduction)
	assert.Equal(t, 5, restored.combatActionTimeCounter)
	calls := restored.GetRNG().Calls()
	_, err = restored.overworldEncounterMonsterCount(
		references.MonsterOrc,
		&references.EnemyReference{MaxPerMap: 7},
	)
	require.NoError(t, err)
	assert.Equal(t, calls+2, restored.GetRNG().Calls())
	combat := &CombatState{}
	combat.TurnQueue = NewCombatTurnQueue()
	restored.configureCombatTurnQueue(combat)
	assert.Equal(t, 5, combat.TurnQueue.actionCount)

	restoredCallbacks := NewMockSystemCallbacks(t)
	restored.SystemCallbacks = restoredCallbacks.ToSystemCallbacks()
	restored.SetupEnvironmentalSystemCallbacks()
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectTimeStop, TurnsRemaining: 1}
	restored.ActiveDurationEffect = state.ActiveDurationEffect
	assert.False(t, restored.IsSailing(), "hydration must not restart automatic travel")
	require.True(t, state.StartAutomaticSailing(references.Right))
	require.True(t, restored.StartAutomaticSailing(references.Right))
	assert.True(t, restored.hmsCapeAutomaticWaitActorSkip, "starting sailing must retain the restored Cape cadence")
	callbacks.Reset()
	restoredCallbacks.Reset()
	beforeWait := state.DateTime
	state.CompleteAutomaticSailingWait()
	restored.CompleteAutomaticSailingWait()
	assert.Equal(t, state.hmsCapeAutomaticWaitActorSkip, restored.hmsCapeAutomaticWaitActorSkip)
	assert.False(t, restored.hmsCapeAutomaticWaitActorSkip, "restored cadence must process the same next actor phase")
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Empty(t, restoredCallbacks.TimeAdvanced())
	assert.Equal(t, beforeWait, state.DateTime, "Time Stop suppresses the Cape wait's requested minute")
	assert.Equal(t, state.DateTime, restored.DateTime)
	assert.Equal(t, 77, restored.PartyVehicle.GetVehicleDetails().GetHullHP())
}

func TestMatchLegacyGeneratedObjectsDoesNotGuessAmongSavedVehicles(t *testing.T) {
	position := references.Position{X: 7, Y: 8}
	candidate := map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, position, 0)
	saved := []save_snapshot.MapUnitSnapshot{
		{
			Kind:     save_snapshot.MapUnitKindVehicle,
			Position: save_snapshot.PositionSnapshot{X: 10, Y: 10},
			Vehicle:  &save_snapshot.VehicleSnapshot{Type: int(references.HorseVehicle)},
		},
		{
			Kind:     save_snapshot.MapUnitKindVehicle,
			Position: save_snapshot.PositionSnapshot{X: 11, Y: 11},
			Vehicle:  &save_snapshot.VehicleSnapshot{Type: int(references.HorseVehicle)},
		},
	}

	matches := matchLegacyGeneratedObjects(saved, []*map_units.NPCFriendly{candidate})
	assert.Nil(t, matches[0])
	assert.Nil(t, matches[1])
}

func TestHydrateLegacySmallMapReplacesGeneratedSemanticObjects(t *testing.T) {
	position := references.Position{X: 7, Y: 8}
	vehicle := map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, position, 0)
	vehicle.MapUnitDetails().NPCNum = 22
	vehicle.GetVehicleDetails().IsOwnedByNPC = true
	chest := map_units.NewNPCFriendlySurfaceChest(references.Britain, position, 0, 5)
	itemReference := references.NPCReference{}
	itemReference.SetKeyIndex(indexes.Sceptre)
	itemReference.Schedule = references.CreateNPCScheduledFixedOneLocation(position, 0)
	item := map_units.NewNPCFriendly(itemReference, 3)
	actorReference := references.NPCReference{}
	actorReference.SetKeyIndex(indexes.TownsPerson1)
	actorReference.Schedule = references.CreateNPCScheduledFixedOneLocation(position, 0)
	actor := map_units.NewNPCFriendly(actorReference, 4)
	inactiveChest := map_units.NewNPCFriendlySurfaceChest(references.Britain, position, 1, 2)
	inactiveChest.SetFloor(1)
	generated := map_units.MapUnits{vehicle, chest, item, actor, inactiveChest}
	chestNumber := 0x9e
	saved := []save_snapshot.MapUnitSnapshot{
		{
			Kind: save_snapshot.MapUnitKindVehicle, NPCNum: 1, Visible: true,
			AIOverrideProvenance: string(references.AIOverrideProvenanceNone),
			Position:             save_snapshot.PositionSnapshot{Location: int(references.Britain), X: 9, Y: 10},
			Vehicle: &save_snapshot.VehicleSnapshot{
				Type: int(references.FrigateVehicle), CurrentDirection: int(references.Left),
				PreviousDirection: int(references.Left), Hull: 41,
			},
		},
		{
			Kind: save_snapshot.MapUnitKindChest, NPCNum: 2, Visible: true,
			AIOverrideProvenance: string(references.AIOverrideProvenanceNone),
			Position:             save_snapshot.PositionSnapshot{Location: int(references.Britain), X: 11, Y: 12},
			SurfaceChestNumber:   &chestNumber,
		},
	}

	state := &GameState{}
	state.MapState.PlayerLocation.Floor = 0
	units, err := state.hydrateLegacySmallMapUnits(saved, &generated)
	require.NoError(t, err)
	require.Len(t, units, 4)
	assert.Same(t, actor, units[0])
	assert.Same(t, inactiveChest, units[1])
	restoredVehicle, ok := units[2].(*map_units.NPCFriendly)
	require.True(t, ok)
	assert.Equal(t, references.Position{X: 9, Y: 10}, restoredVehicle.Pos())
	assert.Equal(t, 41, restoredVehicle.GetVehicleDetails().GetHullHP())
	assert.Equal(t, 22, restoredVehicle.MapUnitDetails().NPCNum)
	assert.True(t, restoredVehicle.GetVehicleDetails().IsOwnedByNPC)
	restoredChest, ok := units[3].(*map_units.NPCFriendly)
	require.True(t, ok)
	assert.Equal(t, 0, restoredChest.MapUnitDetails().NPCNum)
	number, ok := restoredChest.SurfaceChestNumber()
	require.True(t, ok)
	assert.Equal(t, byte(chestNumber), number)
}

func TestHydrateUnitsRestoresAuthoredCarpetOwnership(t *testing.T) {
	position := references.Position{X: 9, Y: 10}
	authored := map_units.NewNPCFriendlyVehiceNewRef(references.CarpetVehicle, position, 0)
	authored.MapUnitDetails().NPCNum = 3
	authored.GetVehicleDetails().IsOwnedByNPC = true
	generated := map_units.MapUnits{authored}
	saved := []save_snapshot.MapUnitSnapshot{{
		Kind: save_snapshot.MapUnitKindVehicle, NPCNum: 3, Visible: true,
		AIOverrideProvenance: string(references.AIOverrideProvenanceNone),
		Position:             save_snapshot.PositionSnapshot{Location: int(references.Britain), X: 9, Y: 10, Floor: 0},
		Vehicle:              &save_snapshot.VehicleSnapshot{Type: int(references.CarpetVehicle)},
	}}
	state := &GameState{DateTime: datetime.UltimaDate{}}

	units, err := state.hydrateUnits(saved, &generated)
	require.NoError(t, err)
	require.Len(t, units, 1)
	restored, ok := units[0].(*map_units.NPCFriendly)
	require.True(t, ok)
	require.True(t, restored.GetVehicleDetails().IsOwnedByNPC)
}

func TestSaveSnapshotEligibilityRejectsDirectInFlightState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GameState)
	}{
		{name: "surface command", mutate: func(state *GameState) { state.surfaceCycleOwned = true }},
		{name: "surface completion", mutate: func(state *GameState) { state.completingSurfaceCycle = true }},
		{name: "Blackthorn interrogation", mutate: func(state *GameState) { state.blackthornSession = &blackthornInterrogationSession{} }},
		{name: "pirate attack presentation", mutate: func(state *GameState) { state.pirateAttackQueue = []pirateAttackRequest{{}} }},
		{name: "tavern customer transaction", mutate: func(state *GameState) { state.tavernCustomerSessionSequence = 1 }},
		{name: "moongate transition", mutate: func(state *GameState) { state.moongateStage = MoongateStage(1) }},
		{name: "ship repair", mutate: func(state *GameState) { state.activeShipRepair = &shipRepairState{} }},
		{name: "suspended outdoor actor phase", mutate: func(state *GameState) { state.outdoorActorContinuation = &outdoorActorContinuation{ready: true} }},
		{name: "invalid active player", mutate: func(state *GameState) { state.activePlayerIndex = party_state.MaxActiveParty }},
		{name: "pending provision hours", mutate: func(state *GameState) { state.pendingProvisionHours = []int{6} }},
		{name: "missing event queue", mutate: func(state *GameState) { state.EventQueueManager = nil }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			test.mutate(state)

			assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
		})
	}
}

func TestSaveSnapshotPreservesShadowlordMidnightContinuation(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	state.ShadowlordState.Reset()
	state.questJournal = newQuestJournal()
	state.ShadowlordState.TownAssignments = [references.ShadowlordCount]references.Location{
		references.Moonglow, references.Jhelom, references.Yew,
	}
	state.DateTime.Hour, state.DateTime.Minute = 23, 59
	state.SetRandomSeed(310)

	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	hydrated, err := NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}
	if hydrated.ShadowlordState.TownAssignments != state.ShadowlordState.TownAssignments {
		t.Fatalf("hydrated assignments = %#v, want %#v", hydrated.ShadowlordState.TownAssignments, state.ShadowlordState.TownAssignments)
	}

	state.advanceElapsedGameplayMinutes(1, false)
	hydrated.advanceElapsedGameplayMinutes(1, false)
	if hydrated.ShadowlordState.TownAssignments != state.ShadowlordState.TownAssignments || hydrated.GetRNG().Calls() != state.GetRNG().Calls() {
		t.Fatalf("midnight continuation diverged: source assignments %#v calls %d, hydrated assignments %#v calls %d", state.ShadowlordState.TownAssignments, state.GetRNG().Calls(), hydrated.ShadowlordState.TownAssignments, hydrated.GetRNG().Calls())
	}
}

func TestSaveSnapshotPreservesDefeatedShadowlordState(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	state.ShadowlordState.Reset()
	state.questJournal = newQuestJournal()
	state.ShadowlordState.TownAssignments = [references.ShadowlordCount]references.Location{
		references.Moonglow, references.Jhelom, references.Yew,
	}
	state.ShadowlordState.Kill(references.Astaroth)
	state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	if !state.RecordShadowlordDefeatEvent(references.Astaroth) {
		t.Fatal("failed to record Astaroth defeat")
	}

	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	hydrated, err := NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}

	if hydrated.ShadowlordState.Alive != state.ShadowlordState.Alive {
		t.Fatalf("hydrated life state = %#v, want %#v", hydrated.ShadowlordState.Alive, state.ShadowlordState.Alive)
	}
	if hydrated.ShadowlordState.IsAlive(references.Astaroth) {
		t.Fatal("hydrated Astaroth is alive after being defeated before capture")
	}
	if hydrated.ShadowlordState.TownAssignments[references.Astaroth] != 0 {
		t.Fatalf("hydrated defeated Astaroth assignment = %d, want unassigned", hydrated.ShadowlordState.TownAssignments[references.Astaroth])
	}
	events := hydrated.QuestJournalEvents()
	if len(events) != 1 || events[0].Subject != QuestJournalSubjectAstaroth || events[0].OccurredOn == nil {
		t.Fatalf("hydrated quest journal = %#v", events)
	}
}

func TestSaveSnapshotFixedSeedNextActionParity(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	state.SetRandomSeed(424242)
	state.initializeEnvironmentalSystems()
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.BritanniaUnderworld,
		Floor:    references.FloorNumber(references.OVERWORLD),
		Position: references.Position{X: 100, Y: 100},
	}
	state.initializeSnapshotMapsAndControllers()
	state.DebugOptions.MonsterGen = false
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectQuickness, TurnsRemaining: 20}
	state.quicknessWorldActorSkip = true
	state.ElapsedMs = 1_000
	state.windSystem.Restore(environment.WindSnapshot{Direction: environment.North, UpdateCounter: 1, NextTickMs: 1_025})
	addOrderedStandardEnemies(t, state)

	before, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	hydrated, err := NewGameStateFromSaveSnapshot(before, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}
	hydrated.DebugOptions.MonsterGen = false
	state.windSystem.SetCallbacks(&windSystemCallbacksAdapter{})
	hydrated.windSystem.SetCallbacks(&windSystemCallbacksAdapter{})

	for step := range 2 {
		wantProcess := state.shouldProcessNonCombatWorldActors()
		gotProcess := hydrated.shouldProcessNonCombatWorldActors()
		if gotProcess != wantProcess {
			t.Fatalf("quickness actor phase step %d = %v, want %v", step, gotProcess, wantProcess)
		}
		if wantProcess {
			state.CurrentNPCAIController.AdvanceNextTurnCalcAndMoveNPCs()
			hydrated.CurrentNPCAIController.AdvanceNextTurnCalcAndMoveNPCs()
		}
	}
	state.ElapsedMs, hydrated.ElapsedMs = 1_080, 1_080
	state.ProcessWindThroughElapsed(state.ElapsedMs)
	hydrated.ProcessWindThroughElapsed(hydrated.ElapsedMs)

	want, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("source recapture error = %v", err)
	}
	got, err := hydrated.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("hydrated recapture error = %v", err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("next-action state diverged\nwant: %#v\ngot:  %#v", want, got)
	}
}

func TestSaveSnapshotRoundTripPreservesDeferredDungeonRoomProgress(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	want := [14]byte{0x01, 0x80, 0x55, 0xaa, 0xff, 0x13, 0x37, 0x42, 0x7f, 0x02, 0x04, 0x08, 0x10, 0x20}
	state.deferredDungeonRoomProgress = want

	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	if got := snapshot.Progression.DungeonRooms; got != want {
		t.Fatalf("captured dungeon rooms = %#v, want %#v", got, want)
	}

	hydrated, err := NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}
	if got := hydrated.deferredDungeonRoomProgress; got != want {
		t.Fatalf("hydrated dungeon rooms = %#v, want %#v", got, want)
	}
	recaptured, err := hydrated.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("hydrated CaptureSaveSnapshot() error = %v", err)
	}
	if got := recaptured.Progression.DungeonRooms; got != want {
		t.Fatalf("recaptured dungeon rooms = %#v, want %#v", got, want)
	}
}

func TestSaveSnapshotLegacyMapUnitBridgeReconstructsSmallMapOnce(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	state.MapState.PlayerLocation = references.PlayerLocation{
		Location: references.Paws,
		Floor:    0,
		Position: references.Position{X: 15, Y: 15},
	}
	state.initializeSnapshotMapsAndControllers()
	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	if snapshot.NeedsMapUnitReconstruction() {
		t.Fatal("ordinary capture enabled map-unit reconstruction")
	}
	if references.Location(snapshot.World.Position.Location).GetMapType() != references.SmallMapType {
		t.Fatal("legacy bridge test requires a current small map")
	}
	snapshot.MapUnits = nil
	snapshot.MarkMapUnitsForReconstruction()

	hydrated, err := NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}
	generatedCount := len(*hydrated.CurrentNPCAIController.GetNpcs())
	if generatedCount == 0 {
		t.Fatal("hydration cleared deterministically generated small-map NPC units")
	}

	recaptured, err := hydrated.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("hydrated CaptureSaveSnapshot() error = %v", err)
	}
	if recaptured.NeedsMapUnitReconstruction() {
		t.Fatal("recapture retained the one-time reconstruction hint")
	}
	if len(recaptured.MapUnits) != generatedCount {
		t.Fatalf("recaptured %d semantic map units, want %d", len(recaptured.MapUnits), generatedCount)
	}
}

func addSnapshotEnemyVariants(t *testing.T, state *GameState) {
	t.Helper()
	controller := state.LargeMapNPCAIController[references.OVERWORLD]
	standard := map_units.NewEnemyNPC(*state.enemyReference(references.MonsterOrc), 9)
	standard.SetPos(references.Position{X: 30, Y: 30})
	standard.SetFloor(references.FloorNumber(references.OVERWORLD))
	standard.SetVisible(true)
	standard.SetCurrentHP(17)

	pirateReference := references.NewPirateShipEnemyReference(state.GameReferences.TileReferences.GetTile(indexes.PirateShipLeft))
	pirate := map_units.NewEnemyNPC(pirateReference, 2)
	pirate.SetPos(references.Position{X: 31, Y: 30})
	pirate.SetFloor(references.FloorNumber(references.OVERWORLD))
	pirate.SetVisible(true)
	pirate.SetCurrentHP(43)

	shadowlord := map_units.NewEnemyNPC(*state.enemyReference(references.MonsterShadowlord), 7)
	shadowlord.SetPos(references.Position{X: 32, Y: 30})
	shadowlord.SetFloor(references.FloorNumber(references.OVERWORLD))
	shadowlord.SetVisible(true)
	shadowlord.SetCurrentHP(91)
	shadowlord.SetShadowlordIdentity(references.Astaroth)

	controller.GetNpcs().ReplaceWith([]map_units.MapUnit{&standard, &pirate, &shadowlord})
}

func addOrderedStandardEnemies(t *testing.T, state *GameState) {
	t.Helper()
	position := state.MapState.PlayerLocation.Position
	first := map_units.NewEnemyNPC(*state.enemyReference(references.MonsterOrc), 9)
	first.SetPos(references.Position{X: position.X + 3, Y: position.Y})
	first.SetFloor(state.MapState.PlayerLocation.Floor)
	first.SetVisible(true)
	second := map_units.NewEnemyNPC(*state.enemyReference(references.MonsterTroll), 2)
	second.SetPos(references.Position{X: position.X + 4, Y: position.Y + 1})
	second.SetFloor(state.MapState.PlayerLocation.Floor)
	second.SetVisible(true)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith([]map_units.MapUnit{&first, &second})
}

// TestSaveSnapshotRepeatedHydrationNormalizesLegacyEmptyOverrides prevents a
// loaded scene from becoming ineligible for the rollback required by its next load.
func TestSaveSnapshotRepeatedHydrationNormalizesLegacyEmptyOverrides(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}

	state := getBritain2GameState()
	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	for y := range 32 {
		for x := range 32 {
			snapshot.MapMutations = append(snapshot.MapMutations, save_snapshot.MapMutationSnapshot{
				Position: save_snapshot.PositionSnapshot{Location: snapshot.World.Position.Location, X: x, Y: y, Floor: snapshot.World.Position.Floor},
				Kind:     save_snapshot.MapMutationSetObject,
				Value:    0,
			})
		}
	}

	first, err := NewGameStateFromSaveSnapshot(snapshot, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("first hydration error = %v", err)
	}
	recaptured, err := first.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("capture after first hydration error = %v", err)
	}
	for index, mutation := range recaptured.MapMutations {
		if mutation.Kind == save_snapshot.MapMutationSetObject && mutation.Value <= 0 {
			t.Fatalf("recaptured map mutation %d retained empty override value %d", index, mutation.Value)
		}
	}
	second, err := NewGameStateFromSaveSnapshot(recaptured, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("second hydration error = %v", err)
	}
	if _, err = second.CaptureSaveSnapshot(); err != nil {
		t.Fatalf("capture after second hydration error = %v", err)
	}
}

func TestSaveSnapshotRoundTripWithRealDataAcrossMapShapes(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	for _, test := range []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		position references.Position
		vehicle  bool
	}{
		{name: "underworld large map with frigate", location: references.BritanniaUnderworld, floor: references.FloorNumber(references.UNDERWORLD), position: references.Position{X: 120, Y: 140}, vehicle: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state := getBritain2GameState()
			state.MapState.PlayerLocation = references.PlayerLocation{Location: test.location, Floor: test.floor, Position: test.position}
			state.initializeSnapshotMapsAndControllers()
			state.ItemStacksMap = *references.NewItemStacksMap()
			mutationPosition := references.Position{X: 2, Y: 3}
			state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapLayer, &mutationPosition, indexes.RegularDoor)
			state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &mutationPosition, indexes.Chest)
			if test.vehicle {
				vehicle := map_units.NewNPCFriendlyVehiceNewRef(references.FrigateVehicle, test.position, test.floor)
				vehicle.GetVehicleDetails().Restore(map_units.VehicleDetailsSnapshot{
					VehicleType: references.FrigateVehicle, CurrentDirection: references.Right, PreviousDirection: references.Up,
					HullHP: 73, MaxHullHP: map_units.MaxHullHP, SailState: map_units.SailRaised, SkiffQuantity: 2,
				})
				vehicle.GetVehicleDetails().HasHMSCape = true
				state.PartyVehicle = *vehicle
			}

			before, err := state.CaptureSaveSnapshot()
			if err != nil {
				t.Fatalf("CaptureSaveSnapshot() error = %v", err)
			}
			if test.vehicle && before.PartyVehicle == nil {
				t.Fatal("frigate state was not captured")
			}
			if len(before.MapMutations) < 2 {
				t.Fatalf("captured %d map mutations, want tile and object mutations", len(before.MapMutations))
			}
			hydrated, err := NewGameStateFromSaveSnapshot(before, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
			if err != nil {
				t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
			}
			after, err := hydrated.CaptureSaveSnapshot()
			if err != nil {
				t.Fatalf("hydrated CaptureSaveSnapshot() error = %v", err)
			}
			if !reflect.DeepEqual(after, before) {
				t.Fatalf("snapshot changed after %s hydration\nbefore: %#v\nafter: %#v", test.name, before, after)
			}
		})
	}
}

func TestSaveSnapshotRoundTripPreservesArrivedScheduleSlot(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	state := getBritain2GameState()
	state.MapState.PlayerLocation = references.PlayerLocation{Location: references.Britain, Floor: 0, Position: references.Position{X: 15, Y: 15}}
	state.initializeSnapshotMapsAndControllers()

	var source *map_units.NPCFriendly
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		friendly, ok := unit.(*map_units.NPCFriendly)
		if ok && friendly.GetVehicleDetails().VehicleType == references.NoPartyVehicle {
			source = friendly
			break
		}
	}
	if source == nil {
		t.Fatal("Britain has no friendly NPC for arrived-slot snapshot test")
	}
	source.MapUnitDetails().RestoreArrivedScheduleSlot(2)

	before, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() error = %v", err)
	}
	hydrated, err := NewGameStateFromSaveSnapshot(before, cachedIntegrationCfg, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	if err != nil {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v", err)
	}

	var restored *map_units.NPCFriendly
	for _, unit := range *hydrated.CurrentNPCAIController.GetNpcs() {
		friendly, ok := unit.(*map_units.NPCFriendly)
		if ok && friendly.MapUnitDetails().NPCNum == source.MapUnitDetails().NPCNum {
			restored = friendly
			break
		}
	}
	if restored == nil || restored.ArrivedScheduleSlot() != 2 {
		t.Fatalf("hydrated arrived slot = %v, want 2", restored)
	}
	after, err := hydrated.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("hydrated CaptureSaveSnapshot() error = %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatal("arrived schedule slot snapshot changed after hydration")
	}
}

func TestCaptureSaveSnapshotRejectsTransientCombat(t *testing.T) {
	state := getBritain2GameState()
	if state == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	state.CombatScene = &CombatState{}
	_, err := state.CaptureSaveSnapshot()
	if !errors.Is(err, ErrSaveSnapshotNotRepresentable) {
		t.Fatalf("CaptureSaveSnapshot() error = %v, want ErrSaveSnapshotNotRepresentable", err)
	}
}

func TestCaptureSaveSnapshotRejectsAutomaticSailingButPreservesRaisedSails(t *testing.T) {
	state := getBritain2GameState()
	if state == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		state.MapState.PlayerLocation.Position,
		state.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailRaised)
	state.PartyVehicle = *frigate
	state.isSailing = true
	if _, err := state.CaptureSaveSnapshot(); !errors.Is(err, ErrSaveSnapshotNotRepresentable) {
		t.Fatalf("CaptureSaveSnapshot() error = %v, want ErrSaveSnapshotNotRepresentable", err)
	}

	state.isSailing = false
	snapshot, err := state.CaptureSaveSnapshot()
	if err != nil {
		t.Fatalf("CaptureSaveSnapshot() with raised sails error = %v", err)
	}
	if snapshot.PartyVehicle == nil || snapshot.PartyVehicle.SailState != int(map_units.SailRaised) {
		t.Fatal("raised vehicle sail state was not preserved")
	}
}

func TestSaveSnapshotRoundTripPreservesHMSCapePreference(t *testing.T) {
	for _, enabled := range []bool{false, true} {
		t.Run(fmt.Sprintf("enabled=%v", enabled), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.SetHMSCapeActivated(enabled)

			snapshot, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			assert.Equal(t, enabled, snapshot.Progression.HMSCapeActivated)

			restored, err := NewGameStateFromSaveSnapshot(
				snapshot,
				state.GameConfig,
				state.GameReferences,
				xTilesVisibleOnGameScreen,
				yTilesVisibleOnGameScreen,
			)
			require.NoError(t, err)
			assert.Equal(t, enabled, restored.IsHMSCapeActivated())
		})
	}
}

func TestMapTransitionClearsCurrentScopeItemStacks(t *testing.T) {
	state := getBritain2GameState()
	if state == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	state.SystemCallbacks = NewMockSystemCallbacks(t).ToSystemCallbacks()
	position := references.Position{X: 3, Y: 4}
	if err := state.ItemStacksMap.Push(&position, &references.ItemStack{Items: references.ItemStacks{{Item: references.Gold, Quantity: 1}}}); err != nil {
		t.Fatal(err)
	}
	state.TransitionToLocation(
		references.LordBritishsCastle,
		references.FloorNumber(0),
		references.Position{X: 10, Y: 10},
		TransitionFullLocation,
		false,
	)
	if state.ItemStacksMap.HasItemStackAtPosition(&position) {
		t.Fatal("item stack bled into a different map scope")
	}
}

func TestNewGameStateFromSaveSnapshotValidatesBeforeHydration(t *testing.T) {
	invalid := save_snapshot.SaveSnapshot{Version: save_snapshot.CurrentVersion}
	invalid.World.Calendar.Day = 29
	_, err := NewGameStateFromSaveSnapshot(invalid, nil, nil, 19, 13)
	if !errors.Is(err, save_snapshot.ErrInvalidSnapshot) {
		t.Fatalf("NewGameStateFromSaveSnapshot() error = %v, want ErrInvalidSnapshot", err)
	}
}
