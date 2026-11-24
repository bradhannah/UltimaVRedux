package game_state

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const lockedSurfaceChestNumber = 0x80 | 30

func TestBasementOpenAndGetUseSourceEligibleEquipment(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).WithFloor(references.Basement).
		WithRandomSeed(625).WithSystemCallbacks().Build()
	chest, direction := positionBesideSurfaceChest(t, state)
	require.Equal(t, references.Basement, chest.Floor())
	chest.RestoreSurfaceChestNumber(30)
	position := direction.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	karma := state.PartyState.Karma.Value
	callbacks.Reset()
	require.True(t, state.ActionOpenSmallMap(direction))
	assert.Equal(t, max(karma-chestTheftKarmaPenalty, 0), state.PartyState.Karma.Value)
	assert.NotContains(t, callbacks.Messages, "Trapped!")
	assert.Equal(t, []int{DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	count := 0
	for state.ItemStacksMap.HasItemStackAtPosition(position) {
		item := *state.ItemStacksMap.Peek(position)
		if equipment, ok := item.Item.(references.Equipment); ok {
			assert.NotContains(t, []references.Equipment{references.JewelShield, references.Ankh, references.MysticArmour, references.ChaosSword, references.GlassSword, references.JeweledSword, references.MysticSword}, equipment)
			before := state.PartyState.Inventory.Equipment.Get(equipment)
			require.True(t, state.ActionGetSmallMap(direction))
			assert.Equal(t, before+item.Quantity, state.PartyState.Inventory.Equipment.Get(equipment))
		} else {
			require.True(t, state.ActionGetSmallMap(direction))
		}
		count++
		require.LessOrEqual(t, count, 24)
	}
	assert.Nil(t, basementSourceChest(state, chest.MapUnitDetails().NPCNum), "authored parent was consumed")
	assert.NotNil(t, state.BasementChestHistory[0].LastOpened)
}

func TestSmallMapChestJimmyOpenSharePersistentAuthority(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithFloor(references.MainFloor).
		WithRandomSeed(382).
		WithSystemCallbacks().
		Build()
	chest, direction := positionBesideSurfaceChest(t, state)
	chest.RestoreSurfaceChestNumber(0x80 | 2)
	chestNumber := chest.MapUnitDetails().NPCNum
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	setJimmyOutcome(state, true)
	callbacks.Reset()

	require.True(t, state.ActionJimmySmallMap(direction))
	assert.False(t, chest.SurfaceChestLocked())
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, []string{chestDisarmedMessage}, callbacks.Messages)
	assert.Equal(t, []int{DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())

	restored := roundTripSaveSnapshot(t, state)
	restoredChest := surfaceChestByNumber(t, restored, chestNumber)
	assert.False(t, restoredChest.SurfaceChestLocked())
	assert.Equal(t, 2, restoredChest.SurfaceChestTreasureValue())
	assert.Equal(t, uint16(2), restored.PartyState.Inventory.Provisions.Keys.Get())

	restoredCallbacks := NewMockSystemCallbacks(t)
	restored.SystemCallbacks = restoredCallbacks.ToSystemCallbacks()
	restored.SetupEnvironmentalSystemCallbacks()
	restoredCallbacks.Reset()
	beforeOpen := restored.DateTime
	require.True(t, restored.ActionOpenSmallMap(direction))
	assert.Nil(t, restored.surfaceChestAt(direction.GetNewPositionInDirection(&restored.MapState.PlayerLocation.Position)))
	assert.False(t, restored.PartyState.IsNPCDead(references.LordBritishsCastle, chestNumber))
	assert.NotNil(t, restored.BasementChestHistory[0].LastOpened)
	assert.Equal(t, uint16(2), restored.PartyState.Inventory.Provisions.Keys.Get())
	assert.Contains(t, restoredCallbacks.Messages, "Opened! That chest isn't yours Avatar!")
	assert.NotContains(t, restoredCallbacks.Messages, "Trapped!", "successful Jimmy disarms bit 7 before Open")
	assert.Equal(t, beforeOpen.Turn+1, restored.DateTime.Turn)

	openedSnapshot, err := restored.CaptureSaveSnapshot()
	require.NoError(t, err)
	reloaded, err := NewGameStateFromSaveSnapshot(
		openedSnapshot,
		state.GameConfig,
		state.GameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	assert.Nil(t, reloaded.surfaceChestAt(direction.GetNewPositionInDirection(&reloaded.MapState.PlayerLocation.Position)))
	assert.False(t, reloaded.PartyState.IsNPCDead(references.LordBritishsCastle, chestNumber))
	assert.Equal(t, restored.BasementChestHistory, reloaded.BasementChestHistory)
}

func TestDirectTrappedSurfaceChestOpenRemovesThenTrapsThenRevealsLoot(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithRandomSeed(1).
		WithSystemCallbacks().
		Build()
	chest, direction := positionBesideSurfaceChest(t, state)
	chest.RestoreSurfaceChestNumber(lockedSurfaceChestNumber)
	chestNumber := chest.MapUnitDetails().NPCNum
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	require.True(t, state.SetActivePlayer(0))
	beforeKarma := state.PartyState.Karma.Value
	callbacks.Reset()

	require.True(t, state.ActionOpenSmallMap(direction))

	assert.Nil(t, basementSourceChest(state, chestNumber))
	assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, chestNumber))
	assert.Equal(t, max(beforeKarma-chestTheftKarmaPenalty, 0), state.PartyState.Karma.Value)
	trapped := stringIndex(callbacks.Messages, "Trapped!")
	found := stringIndex(callbacks.Messages, "Found:")
	require.NotEqual(t, -1, trapped)
	require.NotEqual(t, -1, found)
	assert.Less(t, trapped, found, "boom must resolve before chest loot")
	assert.Contains(t, callbacks.SoundEffectsPlayed, SoundTrapTrigger)
	assert.Equal(t, []int{DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
}

func TestDirectSurfaceActionsUseActivePoisonedCharacter(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		jimmy    bool
		action   func(*GameState, references.Direction) bool
	}{
		{name: "Jimmy small map", location: references.LordBritishsCastle, jimmy: true, action: (*GameState).ActionJimmySmallMap},
		{name: "Jimmy large map", location: references.BritanniaUnderworld, jimmy: true, action: (*GameState).ActionJimmyLargeMap},
		{name: "Open small map", location: references.LordBritishsCastle, action: (*GameState).ActionOpenSmallMap},
		{name: "Open large map", location: references.BritanniaUnderworld, action: (*GameState).ActionOpenLargeMap},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithFloor(references.MainFloor).
				WithPlayerAt(15, 15).
				WithRandomSeed(382).
				WithSystemCallbacks().
				Build()
			var chest *map_units.NPCFriendly
			direction := references.Right
			if test.location == references.LordBritishsCastle {
				chest, direction = positionBesideSurfaceChest(t, state)
			} else {
				target := direction.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
				chest = map_units.NewNPCFriendlySurfaceChest(test.location, *target, references.MainFloor, 30)
				chest.MapUnitDetails().Position = *target
				chest.SetFloor(references.MainFloor)
				*state.CurrentNPCAIController.GetNpcs() = map_units.MapUnits{chest}
			}
			chest.RestoreSurfaceChestNumber(lockedSurfaceChestNumber)
			for index, status := range []party_state.CharacterStatus{party_state.Good, party_state.Poisoned} {
				character := state.PartyState.GetCharacter(index)
				character.PartyMap = party_state.PartyMapActive
				character.SetMaxHp(100)
				character.SetHp(100)
				character.SetStatus(status)
				copy(character.Name[:], "Actor")
			}
			active := state.PartyState.GetCharacter(1)
			require.True(t, state.SetActivePlayer(1))
			if test.jimmy {
				state.PartyState.Inventory.Provisions.Keys.Set(2)
				// Only the active member can pick this lock, so an unlocked chest
				// shows who made the attempt.
				setJimmyOutcome(state, false)
				active.Dexterity = jimmyAlwaysSucceeds
			} else {
				state.SetRandomSeed(findDungeonTrapSeed(t, state, TrapAcid))
			}
			callbacks.Reset()

			require.True(t, test.action(state, direction))

			if test.jimmy {
				assert.False(t, chest.SurfaceChestLocked())
				assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
			} else {
				assert.Contains(t, callbacks.Messages, "ACID!")
				assert.Equal(t, uint16(100), state.PartyState.GetCharacter(0).GetCurrentHp())
				assert.Less(t, active.GetCurrentHp(), uint16(100))
				if _, daily := dailyBasementChestIndex(chest); daily {
					assert.Nil(t, basementSourceChest(state, chest.MapUnitDetails().NPCNum))
				} else {
					assert.Nil(t, state.surfaceChestAt(direction.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)))
				}
			}
		})
	}
}

func TestVersionTenNativeSnapshotIsUnsupported(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithRandomSeed(382).
		WithSystemCallbacks().
		Build()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	envelope := save_snapshot.Envelope{
		Format:          save_snapshot.EnvelopeFormat,
		EnvelopeVersion: save_snapshot.CurrentEnvelopeVersion,
		Metadata: save_snapshot.Metadata{
			Name:      "version 10 chest hydration",
			CreatedAt: time.Date(2026, time.August, 27, 12, 0, 0, 0, time.UTC),
		},
		Snapshot: snapshot,
	}
	encoded, err := json.Marshal(envelope)
	require.NoError(t, err)
	var document map[string]any
	require.NoError(t, json.Unmarshal(encoded, &document))
	snapshotDocument := document["snapshot"].(map[string]any)
	snapshotDocument["version"] = float64(10)
	encoded, err = json.Marshal(document)
	require.NoError(t, err)

	_, err = save_snapshot.Decode(encoded)

	var unsupported *save_snapshot.UnsupportedVersionError
	require.ErrorAs(t, err, &unsupported)
	assert.ErrorIs(t, err, save_snapshot.ErrUnsupportedSnapshotVersion)
	assert.Equal(t, 10, unsupported.Version)
}

func stringIndex(values []string, target string) int {
	for index, value := range values {
		if value == target {
			return index
		}
	}

	return -1
}

func TestSurfaceChestSnapshotRejectsNonChestNPCBeforeHydration(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithFloor(references.MainFloor).
		WithPlayerAt(15, 15).
		Build()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)

	location := state.GameReferences.LocationReferences.GetLocationReference(references.LordBritishsCastle)
	require.NotNil(t, location)
	npcs := *location.GetNPCReferences()
	number := 30
	found := false
	for index := range snapshot.MapUnits {
		unit := &snapshot.MapUnits[index]
		if unit.Kind != "npc" || unit.Position.Location != int(references.LordBritishsCastle) ||
			unit.NPCNum >= len(npcs) || npcs[unit.NPCNum].GetSpriteIndex() == indexes.Chest {
			continue
		}
		unit.SurfaceChestNumber = &number
		found = true

		break
	}
	require.True(t, found, "castle snapshot has no non-chest NPC")

	restored, err := NewGameStateFromSaveSnapshot(
		snapshot,
		state.GameConfig,
		state.GameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	require.ErrorContains(t, err, "surface chest number belongs to a non-chest NPC")
	assert.Nil(t, restored)
}

func TestSmallMapChestJimmyFailureAndNoKeyDoNotSplitState(t *testing.T) {
	tests := []struct {
		name         string
		keys         uint16
		succeeds     bool
		locked       bool
		wantKeys     uint16
		wantFeedback string
	}{
		{name: "failed physical attempt", keys: 2, succeeds: false, locked: true, wantKeys: 1, wantFeedback: "Key broke!"},
		{name: "no key", keys: 0, succeeds: true, locked: true, wantKeys: 0, wantFeedback: "No Keys!"},
		{name: "already unlocked chest", keys: 2, succeeds: true, locked: false, wantKeys: 1, wantFeedback: "Key broke!"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := NewIntegrationTestBuilder(t).
				WithLocation(references.LordBritishsCastle).
				WithFloor(references.MainFloor).
				WithRandomSeed(382).
				WithSystemCallbacks().
				Build()
			chest, direction := positionBesideSurfaceChest(t, state)
			number := byte(30)
			if test.locked {
				number = lockedSurfaceChestNumber
			}
			chest.RestoreSurfaceChestNumber(number)
			state.PartyState.Inventory.Provisions.Keys.Set(test.keys)
			state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
			setJimmyOutcome(state, test.succeeds)
			callbacks.Reset()

			assert.False(t, state.ActionJimmySmallMap(direction))
			assert.Equal(t, test.locked, chest.SurfaceChestLocked())
			assert.Equal(t, test.wantKeys, state.PartyState.Inventory.Provisions.Keys.Get())
			assert.Equal(t, []string{test.wantFeedback}, callbacks.Messages)
		})
	}
}

func TestSmallMapChestTileWithoutObjectAuthorityRejects(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	target := references.Up.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.Chest)
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	setJimmyOutcome(state, true)
	callbacks.Reset()

	assert.False(t, state.ActionJimmySmallMap(references.Up))
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, []string{"No lock!"}, callbacks.Messages)
	callbacks.Reset()
	assert.False(t, state.ActionOpenSmallMap(references.Up))
	assert.Equal(t, []string{"Nothing to open!"}, callbacks.Messages)
	assert.Equal(t, indexes.SpriteIndex(indexes.Chest), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
}

func TestLargeMapJimmyUsesLargeMapAuthority(t *testing.T) {
	tests := []struct {
		name         string
		tile         indexes.SpriteIndex
		keys         uint16
		succeeds     bool
		wantSuccess  bool
		wantTile     indexes.SpriteIndex
		wantKeys     uint16
		wantFeedback string
	}{
		{name: "physical success", tile: indexes.LockedDoor, keys: 2, succeeds: true, wantSuccess: true, wantTile: indexes.RegularDoor, wantKeys: 2, wantFeedback: "Unlocked!"},
		{name: "physical failure", tile: indexes.LockedDoor, keys: 2, succeeds: false, wantTile: indexes.LockedDoor, wantKeys: 1, wantFeedback: "Key broke!"},
		{name: "magical lock", tile: indexes.MagicLockDoor, keys: 2, succeeds: true, wantTile: indexes.MagicLockDoor, wantKeys: 1, wantFeedback: "Key broke!"},
		{name: "no key", tile: indexes.LockedDoor, keys: 0, succeeds: true, wantTile: indexes.LockedDoor, wantKeys: 0, wantFeedback: "No Keys!"},
		{name: "invalid target", tile: indexes.Grass, keys: 2, succeeds: true, wantTile: indexes.Grass, wantKeys: 2, wantFeedback: "No lock!"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(50, 50).
				WithRandomSeed(385).
				WithSystemCallbacks().
				Build()
			target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
			largeMap := state.GetCurrentLayeredMap()
			largeMap.SetTileByLayer(map_state.MapOverrideLayer, target, test.tile)
			smallMap := loadSmallMapControl(state)
			smallTarget := references.Position{X: 10, Y: 10}
			smallMap.SetTileByLayer(map_state.MapOverrideLayer, &smallTarget, indexes.LockedDoorView)
			state.PartyState.Inventory.Provisions.Keys.Set(test.keys)
			state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
			setJimmyOutcome(state, test.succeeds)
			callbacks.Reset()

			assert.Equal(t, test.wantSuccess, state.ActionJimmyLargeMap(references.Right))
			assert.Equal(t, test.wantTile, largeMap.GetTileTopMapOnlyTile(target).Index)
			assert.Equal(t, indexes.SpriteIndex(indexes.LockedDoorView), smallMap.GetTileTopMapOnlyTile(&smallTarget).Index)
			assert.Equal(t, test.wantKeys, state.PartyState.Inventory.Provisions.Keys.Get())
			assert.Equal(t, []string{test.wantFeedback}, callbacks.Messages)
			assert.Equal(t, []int{DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
		})
	}
}

func TestLargeMapJimmyMutationRoundTrips(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithRandomSeed(385).
		WithSystemCallbacks().
		Build()
	target := references.Left.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.LockedDoor)
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	setJimmyOutcome(state, true)
	require.True(t, state.ActionJimmyLargeMap(references.Left))

	restored := roundTripSaveSnapshot(t, state)

	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), restored.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
	assert.Equal(t, uint16(2), restored.PartyState.Inventory.Provisions.Keys.Get())
}

func positionBesideSurfaceChest(t *testing.T, state *GameState) (*map_units.NPCFriendly, references.Direction) {
	t.Helper()
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		chest, ok := unit.(*map_units.NPCFriendly)
		if !ok || !chest.IsSurfaceChest() {
			continue
		}
		state.MapState.PlayerLocation.Floor = chest.Floor()
		if chest.Pos().X > 0 {
			state.MapState.PlayerLocation.Position = references.Position{X: chest.Pos().X - 1, Y: chest.Pos().Y}

			return chest, references.Right
		}
		state.MapState.PlayerLocation.Position = references.Position{X: chest.Pos().X + 1, Y: chest.Pos().Y}

		return chest, references.Left
	}
	t.Fatal("test map has no surface chest on the requested floor")

	return nil, references.NoneDirection
}

func surfaceChestByNumber(t *testing.T, state *GameState, npcNumber int) *map_units.NPCFriendly {
	t.Helper()
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		chest, ok := unit.(*map_units.NPCFriendly)
		if ok && chest.IsSurfaceChest() && chest.MapUnitDetails().NPCNum == npcNumber {
			return chest
		}
	}
	t.Fatalf("surface chest NPC %d was not restored", npcNumber)

	return nil
}

func loadSmallMapControl(state *GameState) *map_state.LayeredMap {
	largeLocation := state.MapState.PlayerLocation
	largeController := state.CurrentNPCAIController
	state.MapState.PlayerLocation.Location = references.Britain
	state.MapState.PlayerLocation.Floor = references.MainFloor
	state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)
	smallMap := state.GetCurrentLayeredMap()
	state.MapState.PlayerLocation = largeLocation
	state.CurrentNPCAIController = largeController

	return smallMap
}
