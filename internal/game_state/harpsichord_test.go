package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestHarpsichordRealCastleMelodyPermanentlyOpensPassage(t *testing.T) {
	state, callbacks := newLordBritishHarpsichordTestState(t, 17, 17)
	passageMap := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, lordBritishHarpsichordFloor)
	passagePosition := lordBritishPassagePosition()

	require.True(t, state.CanPlayHarpsichord())
	assert.Equal(t, indexes.SpriteIndex(indexes.StoneBrickWall), passageMap.GetTileByLayer(map_state.MapLayer, &passagePosition).Index)

	playHarpsichordMelody(t, state)

	assert.True(t, state.lordBritishPassageOpen)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), passageMap.GetTileByLayer(map_state.MapLayer, &passagePosition).Index)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Len(t, callbacks.SoundEffectsPlayed, len(harpsichordMelody))
	assert.True(t, state.EventQueueManager.HasWorkflow(harpsichordEarthquakeWorkflowName))
	assert.True(t, callbacks.InputBlocked)

	beforeHP := state.PartyState.GetAvatar().GetCurrentHp()
	beforeRNGCalls := state.rng.Calls()
	drainHarpsichordWorkflow(t, state)
	assert.Equal(t, beforeHP, state.PartyState.GetAvatar().GetCurrentHp())
	assert.Equal(t, beforeRNGCalls, state.rng.Calls(), "presentation workflow changed gameplay RNG state")
	assert.Empty(t, callbacks.Messages)
	assert.Contains(t, callbacks.SoundEffectsPlayed, SoundEarthquake)
	assert.False(t, callbacks.InputBlocked)

	passageMap.SetTileByLayer(map_state.MapLayer, &passagePosition, indexes.StoneBrickWall)
	playHarpsichordMelody(t, state)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), passageMap.GetTileByLayer(map_state.MapLayer, &passagePosition).Index)
	drainHarpsichordWorkflow(t, state)
	assert.Equal(t, 2, countEqual(callbacks.SoundEffectsPlayed, SoundEarthquake))
}

func TestHarpsichordMelodyProgressPreservesSameFloorAndResetsOnTransition(t *testing.T) {
	state, _ := newLordBritishHarpsichordTestState(t, 17, 17)

	require.True(t, state.ActionPlayHarpsichordNote(6))
	assert.Equal(t, 1, state.harpsichordMelodyProgress)

	state.TransitionToLocation(
		references.LordBritishsCastle,
		lordBritishHarpsichordFloor,
		references.Position{X: 17, Y: 17},
		TransitionPositionOnly,
		false,
	)
	assert.Equal(t, 1, state.harpsichordMelodyProgress)
	state.TransitionToLocation(
		references.LordBritishsCastle,
		lordBritishHarpsichordFloor,
		references.Position{X: 17, Y: 17},
		TransitionPositionOnly,
		false,
	)
	require.True(t, state.ActionPlayHarpsichordNote(7))
	assert.Equal(t, 2, state.harpsichordMelodyProgress)

	state.TransitionToLocation(
		references.LordBritishsCastle,
		lordBritishHarpsichordFloor-1,
		state.MapState.PlayerLocation.Position,
		TransitionFloorOnly,
		false,
	)
	assert.Zero(t, state.harpsichordMelodyProgress)
}

func TestHarpsichordOriginalMismatchFallback(t *testing.T) {
	state, _ := newLordBritishHarpsichordTestState(t, 17, 17)

	for _, test := range []struct {
		name     string
		progress int
		digit    int
		want     int
	}{
		{name: "state ten keeps 678 suffix", progress: 10, digit: 8, want: 3},
		{name: "state eleven keeps 67 suffix", progress: 11, digit: 7, want: 2},
		{name: "first note starts new match", progress: 5, digit: 6, want: 1},
		{name: "other mismatch resets", progress: 5, digit: 4, want: 0},
	} {
		t.Run(test.name, func(t *testing.T) {
			state.harpsichordMelodyProgress = test.progress
			require.True(t, state.ActionPlayHarpsichordNote(test.digit))
			assert.Equal(t, test.want, state.harpsichordMelodyProgress)
		})
	}
}

func TestLordBritishPassageSurvivesMapReconstructionAndSnapshot(t *testing.T) {
	state, _ := newLordBritishHarpsichordTestState(t, 17, 17)
	passagePosition := lordBritishPassagePosition()
	playHarpsichordMelody(t, state)
	drainHarpsichordWorkflow(t, state)

	state.TransitionToLocation(
		references.BritanniaUnderworld,
		references.FloorNumber(references.OVERWORLD),
		references.Position{X: 100, Y: 100},
		TransitionFullLocation,
		false,
	)
	state.TransitionToLocation(
		references.LordBritishsCastle,
		lordBritishHarpsichordFloor,
		references.Position{X: 17, Y: 17},
		TransitionFullLocation,
		false,
	)
	passageMap := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, lordBritishHarpsichordFloor)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), passageMap.GetTileByLayer(map_state.MapLayer, &passagePosition).Index)

	state.ElapsedMs = 0
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	assert.True(t, snapshot.Redux.LordBritishPassageOpen)
	for _, mutation := range snapshot.MapMutations {
		assert.False(t, mutation.Kind == save_snapshot.MapMutationSetTile &&
			mutation.Position.Location == int(references.LordBritishsCastle) &&
			mutation.Position.Floor == int(lordBritishHarpsichordFloor) &&
			mutation.Position.X == lordBritishPassageX &&
			mutation.Position.Y == lordBritishPassageY,
			"durable passage must not also be captured as a generic map mutation")
	}

	hydrated, err := NewGameStateFromSaveSnapshot(
		snapshot,
		state.GameConfig,
		state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen,
		state.MapState.YTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	assert.True(t, hydrated.lordBritishPassageOpen)
	hydratedMap := hydrated.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, lordBritishHarpsichordFloor)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), hydratedMap.GetTileByLayer(map_state.MapLayer, &passagePosition).Index)
}

func TestGetSandalwoodBoxUsesCastleNPC31AndPersists(t *testing.T) {
	state, callbacks := newLordBritishHarpsichordTestState(t, 18, 13)
	boxPosition := references.Position{X: 18, Y: 12}
	mapUnit := state.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		boxPosition,
		lordBritishHarpsichordFloor,
	)
	require.NotNil(t, mapUnit)
	friendly, ok := (*mapUnit).(*map_units.NPCFriendly)
	require.True(t, ok)
	assert.Equal(t, lordBritishSandalwoodBoxNPC, friendly.MapUnitDetails().NPCNum)
	assert.Equal(t, indexes.SpriteIndex(indexes.ItemSandalwoodBox), friendly.NPCReference.GetSpriteIndex())

	require.True(t, state.ActionGetSmallMap(references.Up))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.WoodenBox))
	assert.True(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, lordBritishSandalwoodBoxNPC))
	assert.Nil(t, state.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(boxPosition, lordBritishHarpsichordFloor))
	assert.Equal(t, []string{"A sandalwood box!"}, callbacks.Messages)
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())

	state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)
	assert.Nil(t, state.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(boxPosition, lordBritishHarpsichordFloor))
	assert.False(t, state.ActionGetSmallMap(references.Up))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.WoodenBox))

	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	hydrated, err := NewGameStateFromSaveSnapshot(
		snapshot,
		state.GameConfig,
		state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen,
		state.MapState.YTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	assert.Equal(t, uint16(1), hydrated.PartyState.Inventory.QuestItems.Get(references.WoodenBox))
	assert.True(t, hydrated.PartyState.IsNPCDead(references.LordBritishsCastle, lordBritishSandalwoodBoxNPC))
	assert.True(t, hydrated.lordBritishPassageOpen)
	assert.Nil(t, hydrated.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(boxPosition, lordBritishHarpsichordFloor))
}

func TestOwnedSandalwoodBoxSuppressesNPCAndOpensPassageOnCastleEntry(t *testing.T) {
	state, _ := newLordBritishHarpsichordTestState(t, 18, 13)
	boxPosition := references.Position{X: 18, Y: 12}
	passagePosition := lordBritishPassagePosition()
	require.NotNil(t, state.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		boxPosition,
		lordBritishHarpsichordFloor,
	))

	state.PartyState.Inventory.QuestItems.SetHasOne(references.WoodenBox)
	state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)

	assert.True(t, state.lordBritishPassageOpen)
	assert.True(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, lordBritishSandalwoodBoxNPC))
	assert.Nil(t, state.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		boxPosition,
		lordBritishHarpsichordFloor,
	))
	passageMap := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, lordBritishHarpsichordFloor)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), passageMap.GetTileByLayer(map_state.MapLayer, &passagePosition).Index)
}

func TestLordBritishSecretRoomFurniturePullsClearPathToShelf(t *testing.T) {
	state, callbacks := newLordBritishHarpsichordTestState(t, 15, 12)
	secretRoomMap := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, lordBritishHarpsichordFloor)
	chairPosition := references.Position{X: 14, Y: 12}
	endTablePosition := references.Position{X: 13, Y: 12}
	shelfPosition := references.Position{X: 12, Y: 12}
	chairDestination := references.Position{X: 15, Y: 12}

	require.Equal(t, indexes.SpriteIndex(indexes.ChairBackRight), secretRoomMap.GetTileByLayer(map_state.MapLayer, &chairPosition).Index)
	require.Equal(t, indexes.SpriteIndex(indexes.CoffeeTable), secretRoomMap.GetTileByLayer(map_state.MapLayer, &endTablePosition).Index)
	require.Equal(t, indexes.SpriteIndex(indexes.Bookcase), secretRoomMap.GetTileByLayer(map_state.MapLayer, &shelfPosition).Index)

	require.True(t, state.ActionPushSmallMap(references.Left))
	assert.Equal(t, references.Position{X: 14, Y: 12}, state.MapState.PlayerLocation.Position)
	require.True(t, state.ActionPushSmallMap(references.Left))
	assert.Equal(t, references.Position{X: 13, Y: 12}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, indexes.SpriteIndex(indexes.CoffeeTable), secretRoomMap.GetTileByLayer(map_state.MapLayer, &chairPosition).Index)
	assert.Equal(t, indexes.SpriteIndex(indexes.ChairBackLeft), secretRoomMap.GetTileByLayer(map_state.MapLayer, &chairDestination).Index,
		"a pulled chair faces away from the puller")
	assert.Equal(t, indexes.SpriteIndex(indexes.Bookcase), secretRoomMap.GetTileByLayer(map_state.MapLayer, &shelfPosition).Index)
	assert.Equal(t, []string{"Pulled!", "Pulled!"}, callbacks.Messages)
}

func newLordBritishHarpsichordTestState(t *testing.T, playerX, playerY int) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithFloor(lordBritishHarpsichordFloor).
		WithPlayerAt(playerX, playerY).
		WithSystemCallbacks().
		WithRandomSeed(79).
		Build()

	met, _ := state.PartyState.NPCFlagsSnapshot()
	state.PartyState.RestoreNPCFlags(met, nil)
	state.PartyState.Inventory.QuestItems.Set(references.WoodenBox, 0)
	state.lordBritishPassageOpen = false
	state.harpsichordMelodyProgress = 0
	state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)

	return state, callbacks
}

func playHarpsichordMelody(t *testing.T, state *GameState) {
	t.Helper()
	for _, digit := range []byte(harpsichordMelody) {
		require.True(t, state.ActionPlayHarpsichordNote(int(digit-'0')))
	}
}

func drainHarpsichordWorkflow(t *testing.T, state *GameState) {
	t.Helper()
	for range 100 {
		state.EventQueueManager.Update()
		state.ElapsedMs += 20
		if !state.EventQueueManager.HasWorkflow(harpsichordEarthquakeWorkflowName) {
			return
		}
	}

	t.Fatal("harpsichord earthquake workflow did not finish")
}
