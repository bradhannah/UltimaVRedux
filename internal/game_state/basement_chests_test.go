package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func basementFixture(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.LordBritishsCastle).
		WithFloor(references.Basement).WithPlayerAt(15, 21).WithRandomSeed(683).WithSystemCallbacks().Build()
	state.DateTime = datetime.UltimaDate{Year: 200, Month: 1, Day: 1, Hour: 12}
	return state, callbacks
}

func basementSourceChest(state *GameState, number int) *map_units.NPCFriendly {
	return state.CurrentNPCAIController.GetNpcs().GetFirstFriendlyMatching(func(chest *map_units.NPCFriendly) bool {
		_, daily := dailyBasementChestIndex(chest)
		return daily && chest.MapUnitDetails().NPCNum == number
	})
}

func openBasementSource(t *testing.T, state *GameState, number int) {
	t.Helper()
	chest := basementSourceChest(state, number)
	require.NotNil(t, chest)
	state.MapState.PlayerLocation.Position = references.Position{X: chest.Pos().X - 1, Y: chest.Pos().Y}
	require.True(t, state.ActionOpenSmallMap(references.Right))
}

func TestBasementChestOriginalIdentitiesAndIndependentDates(t *testing.T) {
	state, _ := basementFixture(t)
	refs := *state.GameReferences.LocationReferences.GetLocationReference(references.LordBritishsCastle).GetNPCReferences()
	for index, source := range references.LordBritishBasementChests() {
		assert.Equal(t, source.Position.X, references.Coordinate(refs[source.NPCNumber].Schedule.X[0]))
		assert.Equal(t, source.Position.Y, references.Coordinate(refs[source.NPCNumber].Schedule.Y[0]))
		assert.Equal(t, references.Basement, references.FloorNumber(refs[source.NPCNumber].Schedule.Floor[0]))
		assert.Equal(t, indexes.SpriteIndex(indexes.Chest), refs[source.NPCNumber].GetSpriteIndex())
		chest := basementSourceChest(state, source.NPCNumber)
		require.NotNil(t, chest)
		number, _ := chest.SurfaceChestNumber()
		assert.Equal(t, byte(30), number)
		assert.Equal(t, references.ChestUninspected, chest.SurfaceChestAssessment())
		assert.Nil(t, state.BasementChestHistory[index].LastOpened)
	}
	openBasementSource(t, state, 23)
	assert.Equal(t, state.DateTime.DateOnly(), *state.BasementChestHistory[0].LastOpened)
	assert.Nil(t, state.BasementChestHistory[1].LastOpened)
	assert.Nil(t, state.BasementChestHistory[2].LastOpened)
	assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, 23))
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	assert.Nil(t, basementSourceChest(state, 23))
	assert.NotNil(t, basementSourceChest(state, 24))
	assert.NotNil(t, basementSourceChest(state, 25))
}

func TestBasementChestRequiresRealEntryAfterDateChange(t *testing.T) {
	state, _ := basementFixture(t)
	openBasementSource(t, state, 23)
	survivor := basementSourceChest(state, 24)
	survivor.RestoreSurfaceChestNumber(0x80 | 12)
	survivor.RestoreSurfaceChestAssessment(references.ChestTrapped)
	state.DateTime.Day = 7
	state.CurrentNPCAIController.HardResetNPCs(true)
	state.reloadCurrentSmallMapTerrain()
	state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)
	assert.Nil(t, basementSourceChest(state, 23), "refresh is not arrival")
	assert.Same(t, survivor, basementSourceChest(state, 24))
	assert.Equal(t, references.ChestTrapped, survivor.SurfaceChestAssessment())
	state.TransitionToLocation(references.LordBritishsCastle, references.Basement,
		state.MapState.PlayerLocation.Position, TransitionPositionOnly, false)
	assert.Nil(t, basementSourceChest(state, 23))
	before := state.GetRNG().Calls()
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	replacement := basementSourceChest(state, 23)
	require.NotNil(t, replacement)
	number, _ := replacement.SurfaceChestNumber()
	assert.Equal(t, byte(30), number)
	assert.Equal(t, references.ChestUninspected, replacement.SurfaceChestAssessment())
	assert.Equal(t, before, state.GetRNG().Calls(), "entry must not roll loot")
	assert.Same(t, survivor, basementSourceChest(state, 24))
	openBasementSource(t, state, 23)
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	assert.Nil(t, basementSourceChest(state, 23), "missed days never bank multiple chests")
}

func TestBasementThirdChestUsesInstalledSourceThroughOpenAndReentry(t *testing.T) {
	state, _ := basementFixture(t)
	position := references.Position{X: 13, Y: 23}
	chest := basementSourceChest(state, 25)
	require.NotNil(t, chest)
	assert.Equal(t, position, chest.Pos())
	// Snapshot validation consumes the same central mapping as runtime identity.
	_, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	openBasementSource(t, state, 25)
	assert.Nil(t, basementSourceChest(state, 25))
	assert.Nil(t, state.BasementChestHistory[0].LastOpened)
	assert.Nil(t, state.BasementChestHistory[1].LastOpened)
	require.NotNil(t, state.BasementChestHistory[2].LastOpened)
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	assert.Nil(t, basementSourceChest(state, 25))
	state.DateTime.Day++
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	replacement := basementSourceChest(state, 25)
	require.NotNil(t, replacement)
	assert.Equal(t, position, replacement.Pos())
	assert.Nil(t, state.surfaceChestAt(&references.Position{X: 10, Y: 23}))
	_, err = state.CaptureSaveSnapshot()
	require.NoError(t, err)
}

func TestBasementChestPreMidnightCommitAndCalendarOrdering(t *testing.T) {
	state, callbacks := basementFixture(t)
	state.DateTime = datetime.UltimaDate{Year: 200, Month: 13, Day: 28, Hour: 23, Minute: 59}
	before := state.DateTime.DateOnly()
	callbacks.Reset()
	openBasementSource(t, state, 23)
	assert.Equal(t, before, *state.BasementChestHistory[0].LastOpened)
	assert.Equal(t, datetime.BritannianDate{Year: 201, Month: 1, Day: 1}, state.DateTime.DateOnly())
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
	assert.Nil(t, basementSourceChest(state, 23))
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	require.NotNil(t, basementSourceChest(state, 23))
	openBasementSource(t, state, 23)
	state.DateTime.Year = 199
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	assert.Nil(t, basementSourceChest(state, 23))
	for _, test := range []struct {
		now      datetime.BritannianDate
		eligible bool
	}{
		{datetime.BritannianDate{Year: 200, Month: 13, Day: 28}, false},
		{datetime.BritannianDate{Year: 201, Month: 1, Day: 1}, false},
		{datetime.BritannianDate{Year: 201, Month: 2, Day: 1}, true},
	} {
		assert.Equal(t, test.eligible, state.BasementChestHistory[0].eligible(test.now))
	}
}

func TestBasementChestRejectedActorDoesNotSpendOrRoll(t *testing.T) {
	state, callbacks := basementFixture(t)
	before := state.GetRNG().Calls()
	karma := state.PartyState.Karma.Value
	callbacks.Reset()
	assert.False(t, state.ActionOpenSmallMapWithCharacter(references.Right, nil))
	assert.Nil(t, state.BasementChestHistory[0].LastOpened)
	assert.NotNil(t, basementSourceChest(state, 23))
	assert.Equal(t, before, state.GetRNG().Calls())
	assert.Equal(t, karma, state.PartyState.Karma.Value)
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestBasementChestDeferredPlacementAndDepartureCleanup(t *testing.T) {
	for _, obstruction := range []string{"player", "pile", "terrain", "vehicle"} {
		t.Run(obstruction, func(t *testing.T) {
			state, _ := basementFixture(t)
			openBasementSource(t, state, 23)
			require.True(t, state.GoUpOneFloor())
			state.DateTime.Day++
			position := references.Position{X: 16, Y: 21}
			// Finish floor arrival first, then exercise the entry reconciler with
			// an occupied source square (without departure discarding the fixture pile).
			state.MapState.PlayerLocation.Floor = references.Basement
			state.hardResetSmallMapAfterFloorChange()
			switch obstruction {
			case "player":
				state.MapState.PlayerLocation.Position = position
			case "pile":
				require.NoError(t, state.ItemStacksMap.Push(&position, &references.ItemStack{Items: references.ItemStacks{{Item: references.Gold, Quantity: 3}}}))
			case "terrain":
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.StoneBrickWall)
			case "vehicle":
				vehicle := map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, position, references.Basement)
				require.True(t, state.CurrentNPCAIController.GetNpcs().AddVehicle(*vehicle))
			}
			before := state.GetRNG().Calls()
			state.reconcileBasementChestsOnEntry()
			assert.Nil(t, basementSourceChest(state, 23))
			assert.Equal(t, byte(1), state.BasementChestHistory[0].LastOpened.Day)
			assert.Equal(t, before, state.GetRNG().Calls())
			if obstruction == "vehicle" {
				require.True(t, state.CurrentNPCAIController.GetNpcs().RemoveNPCAtPositionAndFloor(position, references.Basement))
			}
			state.MapState.PlayerLocation.Position = references.Position{X: 15, Y: 21}
			require.True(t, state.GoUpOneFloor())
			require.True(t, state.GoDownOneFloor())
			assert.NotNil(t, basementSourceChest(state, 23), "next unobstructed entry admits the deferred replacement")
			assert.Equal(t, byte(1), state.BasementChestHistory[0].LastOpened.Day)
		})
	}
}

func TestBasementNestedChestAuthorityAndSameTilePile(t *testing.T) {
	state, callbacks := basementFixture(t)
	position := references.Position{X: 16, Y: 21}
	// Select a deterministic source seed yielding a child before committing the
	// production action. No fake RNG or alternate loot route enters GameState.
	var seed uint64
	var childValue byte
	for ; seed < 1000; seed++ {
		found := false
		for _, entry := range references.RollChestLoot(30, beradrng.New(seed)) {
			if entry.ChestValue != 0 {
				found = true
				childValue = entry.ChestValue
			}
		}
		if found {
			break
		}
	}
	require.Less(t, seed, uint64(1000))
	state.SetRandomSeed(seed)
	state.PartyState.Karma.Value = 1
	openBasementSource(t, state, 23)
	child := state.surfaceChestAt(&position)
	require.NotNil(t, child)
	assert.True(t, child.IsTransientSurfaceChest())
	assert.False(t, child.IsAuthoredSurfaceChest())
	assert.False(t, child.SurfaceChestLocked())
	assert.Equal(t, int(childValue), child.SurfaceChestTreasureValue())
	assert.Equal(t, references.ChestUninspected, child.SurfaceChestAssessment())
	assert.Zero(t, state.PartyState.Karma.Value)
	assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, child.MapUnitDetails().NPCNum))
	date := *state.BasementChestHistory[0].LastOpened
	require.NoError(t, state.ItemStacksMap.PushUnder(&position, &references.ItemStack{Items: references.ItemStacks{{Item: references.Gold, Quantity: 1}}}))
	state.PartyState.GetCharacter(0).Intelligence = 30
	require.NotNil(t, state.SearchManager.checkChests(&position, 0))
	child.RestoreSurfaceChestNumber(0x80 | 1)
	state.PartyState.Inventory.Provisions.Keys.Set(1)
	setJimmyOutcome(state, true)
	require.True(t, state.ActionJimmySmallMap(references.Right))
	assert.Equal(t, references.ChestSafe, child.SurfaceChestAssessment())
	assert.Equal(t, 1, child.SurfaceChestTreasureValue())
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
	child.RestoreSurfaceChestNumber(0x80 | 1)
	require.Equal(t, AnSanctChestDisarmed, state.CastAnSanct(&position))
	assert.Equal(t, references.ChestSafe, child.SurfaceChestAssessment())
	callbacks.Reset()
	require.True(t, state.ActionOpenSmallMap(references.Right), "pile must not hide the real child")
	assert.Equal(t, date, *state.BasementChestHistory[0].LastOpened)
	assert.Contains(t, callbacks.Messages, "Chest empty!", "value-one child has no eligible loot")
	assert.Nil(t, state.surfaceChestAt(&position))
	assert.True(t, state.ItemStacksMap.HasItemStackAtPosition(&position))
	karma := state.PartyState.Karma.Value
	require.True(t, state.ActionGetSmallMap(references.Right))
	assert.Equal(t, karma, state.PartyState.Karma.Value, "Get has no second theft penalty")
	require.NotNil(t, state.CurrentNPCAIController.GetNpcs().InsertTransientSurfaceChest(references.LordBritishsCastle, position, references.Basement, 1))
	require.True(t, state.GoUpOneFloor())
	assert.Empty(t, state.ItemStacksMap.Entries())
	assert.Empty(t, state.CurrentNPCAIController.GetNpcs().GetFriendliesMatching((*map_units.NPCFriendly).IsTransientSurfaceChest))
}

func TestBasementFullLootAdmissionStillConsumesSourceDraws(t *testing.T) {
	state, _ := basementFixture(t)
	position := references.Position{X: 16, Y: 21}
	// Fill the active surface budget after accounting for the soon-removed parent.
	count := map_units.SurfaceLootObjectCapacity - state.surfaceLootObjectCount() + 1
	for range count {
		require.NoError(t, state.ItemStacksMap.PushUnder(&position, &references.ItemStack{Items: references.ItemStacks{{Item: references.Gold, Quantity: 1}}}))
	}
	before, _ := state.ItemStacksMap.Snapshot(&position)
	rng := beradrng.New(683)
	references.RollChestLoot(30, rng)
	state.SetRandomSeed(683)
	openBasementSource(t, state, 23)
	after, _ := state.ItemStacksMap.Snapshot(&position)
	assert.Equal(t, before, after)
	assert.Equal(t, rng.Calls(), state.GetRNG().Calls())
	assert.NotNil(t, state.BasementChestHistory[0].LastOpened)
	assert.Nil(t, state.surfaceChestAt(&position))
}

func TestBasementSameDayCastleExitAndLaterFloorOnlyEntry(t *testing.T) {
	state, _ := basementFixture(t)
	openBasementSource(t, state, 23)
	state.PartyState.SetDeadNpc(references.LordBritishsCastle, lordBritishSandalwoodBoxNPC)
	state.TransitionToLocation(references.BritanniaUnderworld, references.MainFloor,
		references.Position{X: 50, Y: 50}, TransitionFullLocation, false)
	state.TransitionToLocation(references.LordBritishsCastle, references.MainFloor,
		references.Position{X: 15, Y: 21}, TransitionFullLocation, false)
	state.TransitionToLocation(references.LordBritishsCastle, references.Basement,
		state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
	assert.Nil(t, basementSourceChest(state, 23))
	assert.NotNil(t, basementSourceChest(state, 24))
	state.DateTime.Day++
	state.TransitionToLocation(references.LordBritishsCastle, references.Basement,
		state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
	assert.Nil(t, basementSourceChest(state, 23), "same floor is not entry")
	state.TransitionToLocation(references.LordBritishsCastle, references.MainFloor,
		state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
	state.TransitionToLocation(references.LordBritishsCastle, references.Basement,
		state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
	assert.NotNil(t, basementSourceChest(state, 23))
	assert.True(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, lordBritishSandalwoodBoxNPC))
}

func TestBasementCombatReturnDoesNotRefill(t *testing.T) {
	state, _ := basementFixture(t)
	openBasementSource(t, state, 23)
	state.DateTime.Day++
	survivor := basementSourceChest(state, 24)
	survivor.RestoreSurfaceChestAssessment(references.ChestUncertain)
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	state.EndCombat(CombatVictory)
	assert.Nil(t, basementSourceChest(state, 23))
	assert.Same(t, survivor, basementSourceChest(state, 24))
	assert.Equal(t, references.ChestUncertain, survivor.SurfaceChestAssessment())
}

func TestBasementDynamicChestCannotAcquireDailyIdentity(t *testing.T) {
	state, _ := basementFixture(t)
	position := references.Position{X: 16, Y: 21}
	units := state.CurrentNPCAIController.GetNpcs()
	require.True(t, units.RemoveNPCByNumber(23))
	child := map_units.NewTransientSurfaceChest(references.LordBritishsCastle, position, references.Basement, 23, 1)
	*units = append(*units, child)
	state.PartyState.Karma.Value = 10
	require.True(t, state.ActionOpenSmallMap(references.Right))
	assert.Equal(t, 8, state.PartyState.Karma.Value)
	assert.Nil(t, state.BasementChestHistory[0].LastOpened)
	assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, 23))
	assert.Empty(t, state.ItemStacksMap.Entries())
}

func TestNonDailyTownChestStillSetsOnlyItsDeathFlag(t *testing.T) {
	state, _ := basementFixture(t)
	state.MapState.PlayerLocation.Floor = references.MainFloor
	state.MapState.PlayerLocation.Position = references.Position{X: 15, Y: 15}
	position := references.Position{X: 16, Y: 15}
	chest := map_units.NewNPCFriendlySurfaceChest(references.LordBritishsCastle, position, references.MainFloor, 30)
	chest.SetPos(position)
	chest.SetFloor(references.MainFloor)
	*state.CurrentNPCAIController.GetNpcs() = map_units.MapUnits{chest}
	require.True(t, state.ActionOpenSmallMap(references.Right))
	assert.True(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, 30))
	for _, source := range references.LordBritishBasementChests() {
		assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, source.NPCNumber))
	}
}

func TestBasementLootPartialPickupPreservesRemainderAndFullCapacityRejects(t *testing.T) {
	state, callbacks := basementFixture(t)
	position := references.Position{X: 16, Y: 21}
	chest := basementSourceChest(state, 23)
	chest.RestoreSurfaceChestNumber(1) // A committed but empty source chest.
	openBasementSource(t, state, 23)
	require.NoError(t, state.ItemStacksMap.Push(&position, &references.ItemStack{Items: references.ItemStacks{
		{Item: references.Key, Quantity: 2}, {Item: references.Gold, Quantity: 3},
	}}))
	state.PartyState.Inventory.Gold.Set(party_state.ItemQuantityLargeMax - 1)
	child := state.CurrentNPCAIController.GetNpcs().InsertTransientSurfaceChest(references.LordBritishsCastle, position, references.Basement, 1)
	require.NotNil(t, child)
	date := *state.BasementChestHistory[0].LastOpened
	callbacks.Reset()
	require.True(t, state.ActionGetSmallMap(references.Right))
	assert.Equal(t, party_state.ItemQuantityLargeMax, state.PartyState.Inventory.Gold.Get())
	assert.Equal(t, []int{1}, callbacks.TimeAdvanced())
	before, _ := state.ItemStacksMap.Snapshot(&position)
	assert.Equal(t, references.ItemStacks{
		{Item: references.Key, Quantity: 2}, {Item: references.Gold, Quantity: 2},
	}, before.Items, "only the actual grant leaves the top entry; the lower entry is untouched")
	assert.Same(t, child, state.surfaceChestAt(&position))
	assert.Equal(t, date, *state.BasementChestHistory[0].LastOpened)
	callbacks.Reset()
	assert.False(t, state.ActionGetSmallMap(references.Right))
	after, _ := state.ItemStacksMap.Snapshot(&position)
	assert.Equal(t, before, after)
	assert.Equal(t, party_state.ItemQuantityLargeMax, state.PartyState.Inventory.Gold.Get())
	assert.Empty(t, callbacks.TimeAdvanced())
	state.PartyState.Inventory.Gold.Set(0)
	require.True(t, state.ActionGetSmallMap(references.Right))
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Gold.Get(), "the earlier granted unit is not awarded twice")
	assert.Equal(t, references.Key, state.ItemStacksMap.Peek(&position).Item)
	state.PartyState.Inventory.Provisions.Keys.Set(party_state.ItemQuantitySmallMax - 1)
	require.True(t, state.ActionGetSmallMap(references.Right))
	assert.Equal(t, party_state.ItemQuantitySmallMax, state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, uint16(1), state.ItemStacksMap.Peek(&position).Quantity)
	assert.False(t, state.ActionGetSmallMap(references.Right))
	assert.Equal(t, uint16(1), state.ItemStacksMap.Peek(&position).Quantity)
	state.PartyState.Inventory.Provisions.Keys.Set(0)
	require.True(t, state.ActionGetSmallMap(references.Right))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.False(t, state.ItemStacksMap.HasItemStackAtPosition(&position))
	assert.Same(t, child, state.surfaceChestAt(&position), "collecting loose loot never consumes the nested chest")
}

func TestBasementOpenRejectsConflictingOccupantBeforeCommit(t *testing.T) {
	state, callbacks := basementFixture(t)
	position := references.Position{X: 16, Y: 21}
	vehicle := map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, position, references.Basement)
	require.True(t, state.CurrentNPCAIController.GetNpcs().AddVehicle(*vehicle))
	before := state.GetRNG().Calls()
	karma := state.PartyState.Karma.Value
	callbacks.Reset()
	assert.False(t, state.ActionOpenSmallMap(references.Right))
	assert.Equal(t, before, state.GetRNG().Calls())
	assert.Equal(t, karma, state.PartyState.Karma.Value)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Nil(t, state.BasementChestHistory[0].LastOpened)
	assert.NotNil(t, basementSourceChest(state, 23))
}

func TestBasementRefreshPreservesAbsentUnconsumedObject(t *testing.T) {
	state, _ := basementFixture(t)
	// Models a deferred source placement or an authoritative imported absence
	// for which no historical opening can be inferred.
	require.True(t, state.CurrentNPCAIController.GetNpcs().RemoveNPCByNumber(23))
	state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)
	assert.Nil(t, basementSourceChest(state, 23))
	assert.Nil(t, state.BasementChestHistory[0].LastOpened)
	require.True(t, state.GoUpOneFloor())
	require.True(t, state.GoDownOneFloor())
	assert.NotNil(t, basementSourceChest(state, 23))
}
