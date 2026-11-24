package game_state

import (
	"fmt"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSmallMapDoorVisitUnlockMethods(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.LordBritishsCastle).
		WithRandomSeed(527).WithSystemCallbacks().Build()
	for _, method := range []string{"Jimmy", "An Sanct", "In Ex Por", "Skull Key"} {
		for _, sourceSprite := range []indexes.SpriteIndex{indexes.LockedDoor, indexes.LockedDoorView, indexes.StoneBrickWallSecret} {
			t.Run(fmt.Sprintf("%s/%d", method, sourceSprite), func(t *testing.T) {
				target := findDoorVisitSource(t, state, sourceSprite)
				state.TransitionToLocation(references.BritanniaUnderworld, 0, references.Position{X: 100, Y: 100}, TransitionFullLocation, false)
				state.TransitionToLocation(target.Location, target.Floor, target.Position.Down(), TransitionFullLocation, false)
				layer := state.GetCurrentLayeredMap()
				layer.UnSetTileByLayer(map_state.MapUnitLayer, &target.Position)
				if sourceSprite == indexes.StoneBrickWallSecret {
					state.SearchManager.handleSecretDoorSearch(&target.Position, state.GameReferences.TileReferences.GetTile(sourceSprite))
					require.Equal(t, indexes.SpriteIndex(indexes.LockedDoor), layer.GetTileTopMapOnlyTile(&target.Position).Index)
				}
				want := indexes.SpriteIndex(indexes.RegularDoor)
				if sourceSprite == indexes.LockedDoorView {
					want = indexes.RegularDoorView
				}
				state.PartyState.Inventory.Provisions.Keys.Set(2)
				state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
				switch method {
				case "Jimmy":
					setJimmyOutcome(state, true)
					require.Equal(t, JimmyUnlocked, state.JimmyDoor(references.Up, state.PartyState.GetCharacter(0)))
				case "An Sanct":
					require.Equal(t, AnSanctDoorUnlocked, state.CastAnSanct(&target.Position))
				case "In Ex Por":
					require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&target.Position))
					require.Equal(t, MagicUnlockSuccess, state.MagicUnlockDoor(&target.Position))
				case "Skull Key":
					require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&target.Position))
					state.PartyState.Inventory.Provisions.SkullKeys.Set(1)
					commit, _ := state.BeginSkullKeyUse()
					require.Equal(t, SkullKeyUseConsumed, state.CommitSkullKeyUse(commit))
					review := state.ReviewSkullKeyUnlock(commit, references.Up)
					require.Equal(t, SkullKeyUnlockReady, review.Result)
					require.Equal(t, SkullKeyUnlockSuccess, state.CommitSkullKeyUnlockForSurfaceUse(review))
					assert.Zero(t, state.PartyState.Inventory.Provisions.SkullKeys.Get())
				}
				require.Equal(t, want, layer.GetTileTopMapOnlyTile(&target.Position).Index)
				assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
				date, calls := state.DateTime, state.GetRNG().Calls()
				source := state.GetCurrentSmallLocationReference()
				for range 2 {
					for _, floor := range source.ListOfFloors {
						state.TransitionToLocation(target.Location, floor, state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
						retained := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, target.Floor)
						assert.Equal(t, want, retained.GetTileTopMapOnlyTile(&target.Position).Index)
					}
				}
				assert.Equal(t, sourceSprite, source.GetTileNumber(target.Position, target.Floor), "immutable source is not edited")
				assert.Equal(t, date, state.DateTime)
				assert.Equal(t, calls, state.GetRNG().Calls(), "reconstruction spends no random draws")
			})
		}
	}
}

func TestSmallMapDoorVisitRebuildRetainsOnlyLatestDoorOverrides(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).
		WithPlayerAt(10, 14).WithSystemCallbacks().Build()
	position := references.Position{X: 11, Y: 14}
	source := state.GetCurrentSmallLocationReference()
	require.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), source.GetTileNumber(position, 0))
	ground := state.GetCurrentLayeredMap()
	upper := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 1)
	basement := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, -1)
	upper.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.MagicLockDoorWithView)
	basement.SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.RegularDoorView)
	other := references.Position{X: 2, Y: 2}
	ground.SetTileByLayer(map_state.MapOverrideLayer, &other, indexes.PoisonField)
	ground.SetTileByLayer(map_state.MapLayer, &other, indexes.Chest)
	// A map-unit sprite must not hide the raw override from reconstruction.
	basement.SetTileByLayer(map_state.MapUnitLayer, &position, indexes.TownsPersonKeyIndex)
	require.Equal(t, map_state.OpenDoorOpened, state.MapState.OpenDoor(references.Right))
	state.TransitionToLocation(references.SerpentsHold, 1, state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
	assert.False(t, state.MapState.HasTransientDoor())
	for _, test := range []struct {
		floor references.FloorNumber
		want  indexes.SpriteIndex
	}{
		{-1, indexes.RegularDoorView}, {0, indexes.RegularDoor}, {1, indexes.MagicLockDoorWithView},
	} {
		layer := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, test.floor)
		assert.Equal(t, test.want, layer.SpriteIndexByLayer(map_state.MapOverrideLayer, position), "floor %d", test.floor)
	}
	ground = state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 0)
	untouched := references.Position{X: 17, Y: 6}
	require.Equal(t, indexes.SpriteIndex(indexes.LockedDoorView), source.GetTileNumber(untouched, 0))
	assert.Equal(t, indexes.SpriteIndex(indexes.LockedDoorView), ground.GetTileTopMapOnlyTile(&untouched).Index)
	assert.LessOrEqual(t, ground.SpriteIndexByLayer(map_state.MapOverrideLayer, other), indexes.SpriteIndex(0))
	assert.Equal(t, source.GetTileNumber(other, 0), ground.SpriteIndexByLayer(map_state.MapLayer, other))
	state.TransitionToLocation(references.SerpentsHold, 0, state.MapState.PlayerLocation.Position, TransitionFloorOnly, false)
	require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&position))
	upper = state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 1)
	controller := state.CurrentNPCAIController
	state.reloadCurrentSmallMapTerrain()
	assert.Same(t, upper, state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 1), "single-floor reload leaves other floors intact")
	assert.Same(t, controller, state.CurrentNPCAIController)
	assert.Equal(t, indexes.SpriteIndex(indexes.MagicLockDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&position).Index)
	require.Equal(t, MagicUnlockSuccess, state.MagicUnlockDoor(&position))
	state.reloadCurrentSmallMapTerrain()
	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&position).Index)
}

func TestSmallMapDoorVisitPhysicalExitAndSameLocationRebuild(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.SerpentsHold).
		WithPlayerAt(10, 14).WithSystemCallbacks().Build()
	door := references.Position{X: 11, Y: 14}
	require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&door))
	_, err := state.DebugSetWorldPosition(references.SerpentsHold, 0, state.MapState.PlayerLocation.Position)
	require.NoError(t, err)
	assert.Equal(t, indexes.SpriteIndex(indexes.MagicLockDoor), state.GetEffectiveTile(&door))
	for _, destination := range []references.Location{references.BritanniaUnderworld, references.Britain} {
		state.TransitionToLocation(destination, 0, references.Position{X: 15, Y: 15}, TransitionFullLocation, false)
		if destination == references.BritanniaUnderworld {
			assert.Nil(t, state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, 0))
		}
		state.TransitionToLocation(references.SerpentsHold, 0, references.Position{X: 10, Y: 14}, TransitionFullLocation, false)
		assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), state.GetEffectiveTile(&door), "a true exit ends the prior visit")
		require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&door))
	}
}

func TestSmallMapDoorVisitPrisonArrestAndResurrection(t *testing.T) {
	t.Run("virtual Palace prison suspends the visit", func(t *testing.T) {
		state, _ := newBlackthornTestState(t, 3)
		target := findDoorVisitSourceAtLocation(t, state, references.PalaceOfBlackthorn, indexes.LockedDoor)
		state.TransitionToLocation(target.Location, target.Floor, target.Position.Down(), TransitionFullLocation, false)
		require.Equal(t, AnSanctDoorUnlocked, state.CastAnSanct(&target.Position))
		_, err := state.BeginBlackthornInterrogation()
		require.NoError(t, err)
		require.Equal(t, references.BlackthornPrison, state.MapState.PlayerLocation.Location)
		state.FinishBlackthornInterrogation()
		layer := state.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, target.Floor)
		assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), layer.GetTileTopMapOnlyTile(&target.Position).Index)
	})
	for _, origin := range []references.Location{references.Yew, references.LordBritishsCastle} {
		t.Run(fmt.Sprintf("arrest from %s", origin), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(origin).WithSystemCallbacks().Build()
			target := findDoorVisitSourceAtLocation(t, state, origin, indexes.LockedDoorView)
			state.TransitionToLocation(origin, target.Floor, target.Position.Down(), TransitionFullLocation, false)
			require.Equal(t, AnSanctDoorUnlocked, state.CastAnSanct(&target.Position))
			state.DateTime.Hour = references.JailReleaseHour
			state.ExecuteJailSentence()
			state.TransitionToLocation(origin, target.Floor, target.Position.Down(), TransitionFullLocation, false)
			want := indexes.SpriteIndex(indexes.LockedDoorView)
			if origin == references.Yew {
				want = indexes.RegularDoorView
			}
			assert.Equal(t, want, state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&target.Position).Index)
		})
		t.Run(fmt.Sprintf("resurrection from %s", origin), func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(origin).WithSystemCallbacks().Build()
			target := findDoorVisitSourceAtLocation(t, state, origin, indexes.LockedDoorView)
			state.TransitionToLocation(origin, target.Floor, target.Position.Down(), TransitionFullLocation, false)
			require.Equal(t, AnSanctDoorUnlocked, state.CastAnSanct(&target.Position))
			state.DateTime.Hour = ResurrectionHour
			commitTestDeathResurrection(t, state)
			state.TransitionToLocation(origin, target.Floor, target.Position.Down(), TransitionFullLocation, false)
			want := indexes.SpriteIndex(indexes.LockedDoorView)
			if origin == references.LordBritishsCastle {
				want = indexes.RegularDoorView
			}
			assert.Equal(t, want, state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&target.Position).Index)
		})
	}
}

func TestSmallMapDoorVisitFailedJimmyDoesNotRetainUnlock(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.LordBritishsCastle).WithSystemCallbacks().Build()
	target := findDoorVisitSourceAtLocation(t, state, references.LordBritishsCastle, indexes.LockedDoor)
	state.TransitionToLocation(target.Location, target.Floor, target.Position.Down(), TransitionFullLocation, false)
	setJimmyOutcome(state, false)
	for _, keys := range []uint16{0, 2} {
		state.PartyState.Inventory.Provisions.Keys.Set(keys)
		want, remaining := JimmyDoorNoLockPicks, uint16(0)
		if keys > 0 {
			want, remaining = JimmyBrokenPick, keys-1
		}
		assert.Equal(t, want, state.JimmyDoor(references.Up, state.PartyState.GetCharacter(0)))
		assert.Equal(t, remaining, state.PartyState.Inventory.Provisions.Keys.Get())
		state.reloadCurrentSmallMapTerrain()
		assert.Equal(t, indexes.SpriteIndex(indexes.LockedDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&target.Position).Index)
	}
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&target.Position))
	assert.Equal(t, JimmyBrokenPick, state.JimmyDoor(references.Up, state.PartyState.GetCharacter(0)))
	state.reloadCurrentSmallMapTerrain()
	assert.Equal(t, indexes.SpriteIndex(indexes.MagicLockDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&target.Position).Index)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
	secret := findDoorVisitSource(t, state, indexes.StoneBrickWallSecret)
	state.TransitionToLocation(secret.Location, secret.Floor, secret.Position.Down(), TransitionFullLocation, false)
	state.SearchManager.handleSecretDoorSearch(&secret.Position, state.GameReferences.TileReferences.GetTile(indexes.StoneBrickWallSecret))
	state.reloadCurrentSmallMapTerrain()
	assert.Equal(t, indexes.SpriteIndex(indexes.StoneBrickWallSecret), state.GetEffectiveTile(&secret.Position), "searched but never unlocked doors are outside retention scope")
}

func TestSmallMapDoorVisitEncounterReturn(t *testing.T) {
	for _, reason := range []CombatEndReason{CombatVictory, CombatDefeat, CombatEvacuated} {
		t.Run(fmt.Sprint(reason), func(t *testing.T) {
			state, _, _, direction := newSmallMapCombatSourceTestState(t, false, 318)
			controller := state.CurrentNPCAIController
			location := state.MapState.PlayerLocation
			// Britain ground-floor's source lock is a discovered secret wall,
			// rather than the already-unlocked door used by the older fixture.
			ordinary, _, _ := findSmallMapDoorForCombatReturnTest(t, state)
			source := state.GetCurrentSmallLocationReference()
			sourceDoor := source.GetTileNumber(ordinary, location.Floor)
			require.True(t, sourceDoor.IsUnlockedDoor())
			require.Equal(t, sourceDoor, state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&ordinary).Index)
			wantMagicLock := indexes.SpriteIndex(indexes.MagicLockDoor)
			if sourceDoor.IsWindowedDoor() {
				wantMagicLock = indexes.MagicLockDoorWithView
			}
			target := findDoorVisitSourceAtLocation(t, state, references.Britain, indexes.StoneBrickWallSecret)
			require.Equal(t, location.Floor, target.Floor)
			state.SearchManager.handleSecretDoorSearch(&target.Position, state.GameReferences.TileReferences.GetTile(indexes.StoneBrickWallSecret))
			require.Equal(t, AnSanctDoorUnlocked, state.CastAnSanct(&target.Position))
			require.Equal(t, MagicLockSuccess, state.MagicLockDoor(&ordinary))
			require.Equal(t, wantMagicLock, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapOverrideLayer, ordinary))
			other := references.Position{X: 1, Y: 1}
			otherSource := source.GetTileNumber(other, location.Floor)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &other, indexes.PoisonField)
			require.True(t, state.ActionAttackSmallMap(direction))
			require.NotNil(t, state.CombatScene)
			assert.ErrorIs(t, state.ValidateSaveSnapshotEligibility(), ErrSaveSnapshotNotRepresentable)
			state.EndCombat(reason)
			assert.Same(t, controller, state.CurrentNPCAIController)
			assert.Equal(t, location, state.MapState.PlayerLocation)
			assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&target.Position).Index)
			assert.Equal(t, wantMagicLock, state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&ordinary).Index)
			assert.Equal(t, wantMagicLock, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapOverrideLayer, ordinary))
			assert.Equal(t, sourceDoor, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapLayer, ordinary), "lock overrides do not replace source terrain")
			assert.LessOrEqual(t, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapOverrideLayer, other), indexes.SpriteIndex(0))
			assert.Equal(t, otherSource, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapLayer, other), "unrelated terrain resets to its immutable source")
		})
	}
}

func findDoorVisitSource(t *testing.T, state *GameState, sprite indexes.SpriteIndex) references.PlayerLocation {
	t.Helper()
	for location := references.Moonglow; location <= references.SerpentsHold; location++ {
		if target, ok := doorVisitSourceAtLocation(state, location, sprite); ok {
			return target
		}
	}
	t.Fatalf("no original multi-floor source for door sprite %d", sprite)
	return references.PlayerLocation{}
}

func findDoorVisitSourceAtLocation(t *testing.T, state *GameState, location references.Location, sprite indexes.SpriteIndex) references.PlayerLocation {
	t.Helper()
	target, ok := doorVisitSourceAtLocation(state, location, sprite)
	require.True(t, ok, "no original multi-floor source in %s for sprite %d", location, sprite)
	return target
}

func doorVisitSourceAtLocation(state *GameState, location references.Location, sprite indexes.SpriteIndex) (references.PlayerLocation, bool) {
	source := state.GameReferences.LocationReferences.GetLocationReference(location)
	if len(source.ListOfFloors) < 2 {
		return references.PlayerLocation{}, false
	}
	for _, floor := range source.ListOfFloors {
		for y := references.Coordinate(0); y < references.YSmallMapTiles-1; y++ {
			for x := references.Coordinate(0); x < references.XSmallMapTiles; x++ {
				position := references.Position{X: x, Y: y}
				if source.GetTileNumber(position, floor) == sprite {
					return references.PlayerLocation{Location: location, Floor: floor, Position: position}, true
				}
			}
		}
	}
	return references.PlayerLocation{}, false
}
