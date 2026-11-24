package main

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestSurfaceReturnUsesMapSpecificRejectionAndCycleOwnership(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		handle   func(*GameScene, ebiten.Key)
		wantRow  string
		wantTime int
	}{
		{name: "town", location: references.Britain, handle: (*GameScene).smallMapInputHandler, wantRow: "What?"},
		{name: "overworld", location: references.BritanniaUnderworld, handle: (*GameScene).largeMapInputHandler, wantRow: "What?", wantTime: game_state.DefaultLargeMapMinutesPerTurn},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)

			test.handle(scene, ebiten.KeyEnter)

			if test.wantRow != "" {
				assert.Equal(t, []string{test.wantRow}, callbacks.Messages)
			}
			if test.wantTime == 0 {
				assert.Empty(t, callbacks.TimeAdvanced())
			} else {
				assert.Equal(t, []int{test.wantTime}, callbacks.TimeAdvanced())
			}
		})
	}
}

func TestSurfacePeriodAndTownReturnAreFreeWhatFallbacks(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		handle   func(*GameScene, ebiten.Key)
		key      ebiten.Key
	}{
		{name: "town return", location: references.Britain, handle: (*GameScene).smallMapInputHandler, key: ebiten.KeyEnter},
		{name: "town period", location: references.Britain, handle: (*GameScene).smallMapInputHandler, key: ebiten.KeyPeriod},
		{name: "outdoor period", location: references.BritanniaUnderworld, handle: (*GameScene).largeMapInputHandler, key: ebiten.KeyPeriod},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)

			test.handle(scene, test.key)

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, []string{"What?"}, callbacks.Messages)
			assert.Empty(t, callbacks.TimeAdvanced())
		})
	}
}

func TestLargeMapJimmyRoutesImmediateDirectionAndCompletesOnce(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithRandomSeed(385).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	state.PartyState.GetCharacter(0).Dexterity = 30
	require.True(t, state.SetActivePlayer(0))
	target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.LockedDoor)
	callbacks.Reset()
	pressed := map[ebiten.Key]bool{ebiten.KeyRight: true}
	scene := newSurfaceCompletionTestScene(state)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)

	scene.largeMapInputHandler(ebiten.KeyJ)

	assert.Equal(t, JimmyDoorDirectionInput, scene.secondaryKeyState)
	assert.Equal(t, []string{"Jimmy-"}, callbacks.CommandPrompts)
	assert.Empty(t, callbacks.TimeAdvanced())

	scene.largeMapHandleSecondaryInput()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Contains(t, callbacks.Messages, "Unlocked!")
}

func TestSurfaceActorShortcutUsesSameGoodOrPoisonedEligibilityAsDialog(t *testing.T) {
	for _, status := range []party_state.CharacterStatus{party_state.Good, party_state.Poisoned} {
		t.Run(characterSelectionStatus(status), func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(50, 50).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			state.PartyState.Inventory.Provisions.Keys.Set(1)
			actor := state.PartyState.GetCharacter(0)
			actor.SetStatus(status)
			actor.Dexterity = 30
			require.True(t, state.SetActivePlayer(0))
			target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.LockedDoor)
			scene := newSurfaceCompletionTestScene(state)

			result := scene.largeMapJimmySecondaryWithDialog(references.Right, func(surfaceCommandState) {})

			assert.Equal(t, surfaceCommandCompleted, result)
			assert.Nil(t, scene.dialogStack.PeekTopModalDialog())
		})
	}
}

func TestLargeMapJimmyWithoutActiveCharacterDefersToSelection(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.ClearActivePlayer()
	for index := 0; index < 2; index++ {
		state.PartyState.GetCharacter(index).SetStatus(party_state.Good)
		state.PartyState.GetCharacter(index).Dexterity = 30
	}
	target := references.Right.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.LockedDoor)
	callbacks.Reset()
	pressed := map[ebiten.Key]bool{ebiten.KeyRight: true}
	scene := newSurfaceCompletionTestScene(state)
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)

	scene.largeMapInputHandler(ebiten.KeyJ)
	scene.largeMapHandleSecondaryInput()

	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok, "multiple eligible actors require original whowill ownership")
	assert.Equal(t, indexes.SpriteIndex(indexes.LockedDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
	assert.Empty(t, callbacks.TimeAdvanced())
	dialog.HandleEnter()

	assert.Equal(t, indexes.SpriteIndex(indexes.RegularDoor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
	assert.Equal(t, []string{"Unlocked!"}, callbacks.Messages)
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestLargeMapOpenRoutesAuthoritativeSuccessAndExplicitRejection(t *testing.T) {
	tests := []struct {
		name       string
		tile       indexes.SpriteIndex
		wantOpened bool
		wantRow    string
	}{
		{name: "door", tile: indexes.RegularDoor, wantOpened: true, wantRow: "Opened!"},
		{name: "invalid", tile: indexes.Grass, wantRow: "Nothing to open!"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(50, 50).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			target := references.Up.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, test.tile)
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)
			pressed := map[ebiten.Key]bool{ebiten.KeyUp: true}
			scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
			scene.keyboard.SetLastKeyPressedNowPlusMs(0)

			scene.largeMapInputHandler(ebiten.KeyO)
			assert.Equal(t, OpenDirectionInput, scene.secondaryKeyState)
			assert.Empty(t, callbacks.TimeAdvanced())
			scene.largeMapHandleSecondaryInput()

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Contains(t, callbacks.Messages, test.wantRow)
			assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
			if test.wantOpened {
				assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(target).Index)
			}
		})
	}
}

func TestOpenAndJimmyDirectionEscapeCancelsWithoutCost(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		state    InputState
		handle   func(*GameScene)
	}{
		{name: "town Jimmy", location: references.Britain, state: JimmyDoorDirectionInput, handle: (*GameScene).smallMapHandleSecondaryInput},
		{name: "town Open", location: references.Britain, state: OpenDirectionInput, handle: (*GameScene).smallMapHandleSecondaryInput},
		{name: "overworld Jimmy", location: references.BritanniaUnderworld, state: JimmyDoorDirectionInput, handle: (*GameScene).largeMapHandleSecondaryInput},
		{name: "overworld Open", location: references.BritanniaUnderworld, state: OpenDirectionInput, handle: (*GameScene).largeMapHandleSecondaryInput},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			state.CurrentNPCAIController.GetNpcs().ClearAllMapUnits()
			state.PartyState.Inventory.Provisions.Keys.Set(2)
			beforeTime, beforeRNG := state.DateTime, state.GetRNG().Calls()
			beforeFood := state.PartyState.Inventory.Provisions.Food.Get()
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)
			scene.secondaryKeyState = test.state
			scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)

			test.handle(scene)

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Empty(t, callbacks.Messages)
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Equal(t, beforeTime, state.DateTime)
			assert.Equal(t, beforeRNG, state.GetRNG().Calls())
			assert.EqualValues(t, 2, state.PartyState.Inventory.Provisions.Keys.Get())
			assert.Equal(t, beforeFood, state.PartyState.Inventory.Provisions.Food.Get())
			assert.Zero(t, scene.dialogStack.Count())
			assert.False(t, state.EventQueueManager.HasActiveWorkflows())
			minutes := game_state.DefaultLargeMapMinutesPerTurn
			if test.location.GetMapType() == references.SmallMapType {
				scene.smallMapInputHandler(ebiten.KeySpace)
				minutes = game_state.DefaultSmallMapMinutesPerTurn
			} else {
				scene.largeMapInputHandler(ebiten.KeySpace)
			}
			expectedTime := beforeTime
			expectedTime.Advance(minutes)
			assert.Equal(t, expectedTime, state.DateTime, "the alternative command owns only its own cycle")
			callbacks.AssertTimeAdvanced(minutes)
			assert.EqualValues(t, 2, state.PartyState.Inventory.Provisions.Keys.Get())
		})
	}
}

func TestSmallMapChestJimmyDialogCancellationDefersCompletion(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithSystemCallbacks().
		Build()
	chest, direction := positionSceneBesideChest(t, state)
	chest.RestoreSurfaceChestNumber(0x80 | 30)
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	state.ClearActivePlayer()
	state.DebugOptions.MonsterGen = false
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	pressed := map[ebiten.Key]bool{directionKey(direction): true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)

	scene.smallMapInputHandler(ebiten.KeyJ)
	scene.smallMapHandleSecondaryInput()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Empty(t, callbacks.TimeAdvanced())
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleEscape()

	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.True(t, chest.SurfaceChestLocked())
	assert.Zero(t, scene.dialogStack.Count())
}

func TestSmallMapChestJimmyThenOpenProductionPathCompletesOnceEach(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithRandomSeed(382).
		WithSystemCallbacks().
		Build()
	chest, direction := positionSceneBesideChest(t, state)
	chest.RestoreSurfaceChestNumber(0x80 | 30)
	chestNumber := chest.MapUnitDetails().NPCNum
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	state.PartyState.GetCharacter(0).Dexterity = 30
	require.True(t, state.SetActivePlayer(0))
	state.DebugOptions.MonsterGen = false
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	pressed := map[ebiten.Key]bool{directionKey(direction): true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)

	scene.smallMapInputHandler(ebiten.KeyJ)
	scene.smallMapHandleSecondaryInput()

	assert.False(t, chest.SurfaceChestLocked())
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, []string{"Trap disarmed!"}, callbacks.Messages)
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, references.ChestSafe, chest.SurfaceChestAssessment())

	// A known-safe refusal, including without keys, still completes one command.
	state.PartyState.Inventory.Provisions.Keys.Set(0)
	callbacks.Reset()
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)
	scene.smallMapInputHandler(ebiten.KeyJ)
	scene.smallMapHandleSecondaryInput()
	assert.Equal(t, []string{"Already safe - nothing to disarm."}, callbacks.Messages)
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, state.PartyState.Inventory.Provisions.Keys.Get())
	state.PartyState.Inventory.Provisions.Keys.Set(2)

	// The safe parent makes the source loot draws the first RNG use on Open.
	// Exercise a real child rather than confusing its reused slot with the parent.
	var seed uint64
	var childValue byte
	for ; seed < 1000; seed++ {
		for _, reward := range references.RollChestLoot(30, beradrng.New(seed)) {
			if reward.ChestValue != 0 {
				childValue = reward.ChestValue
			}
		}
		if childValue != 0 {
			break
		}
	}
	require.NotZero(t, childValue)
	state.SetRandomSeed(seed)
	dateBefore := state.DateTime.DateOnly()
	karmaBefore := state.PartyState.Karma.Value
	callbacks.Reset()
	pressed = map[ebiten.Key]bool{directionKey(direction): true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)
	scene.smallMapInputHandler(ebiten.KeyO)
	scene.smallMapHandleSecondaryInput()

	assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, chestNumber))
	assert.NotContains(t, *state.CurrentNPCAIController.GetNpcs(), chest, "the authored parent is gone")
	index, daily := references.BasementChestIndex(chest.NPCReference, chestNumber)
	require.True(t, daily)
	require.NotNil(t, state.BasementChestHistory[index].LastOpened)
	assert.Equal(t, dateBefore, *state.BasementChestHistory[index].LastOpened)
	assert.Equal(t, max(karmaBefore-2, 0), state.PartyState.Karma.Value)
	assert.NotContains(t, callbacks.Messages, "Trapped!", "Jimmy disarmed the actual parent")
	assert.Equal(t, uint16(2), state.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
	assert.Equal(t, 1, countEqual(callbacks.Messages, "Opened! That chest isn't yours Avatar!"))
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	children := state.CurrentNPCAIController.GetNpcs().GetFriendliesMatching(func(candidate *map_units.NPCFriendly) bool {
		return candidate.IsTransientSurfaceChest() && candidate.Pos() == chest.Pos() && candidate.Floor() == chest.Floor()
	})
	require.Len(t, children, 1)
	child := children[0]
	assert.True(t, state.CanOpenSurfaceChest(direction), "the generated child remains an actionable world object")
	assert.False(t, child.IsAuthoredSurfaceChest())
	assert.False(t, child.SurfaceChestLocked())
	assert.Equal(t, int(childValue), child.SurfaceChestTreasureValue())
	assert.Equal(t, references.ChestUninspected, child.SurfaceChestAssessment())
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	savedDate := snapshot.BasementChestHistory[index].LastOpened
	require.NotNil(t, savedDate)
	assert.Equal(t, int(dateBefore.Year), savedDate.Year)
	assert.Equal(t, int(dateBefore.Month), savedDate.Month)
	assert.Equal(t, int(dateBefore.Day), savedDate.Day)
	childCaptured := false
	for _, unit := range snapshot.MapUnits {
		if unit.Position.Location == int(references.LordBritishsCastle) && unit.NPCNum == child.MapUnitDetails().NPCNum {
			childCaptured = true
			assert.True(t, unit.SurfaceChestTransient)
			assert.False(t, unit.SurfaceChestAuthored)
			require.NotNil(t, unit.SurfaceChestNumber)
			assert.Equal(t, int(childValue), *unit.SurfaceChestNumber)
		}
	}
	assert.True(t, childCaptured)
}

func TestLordBritishChestExistingLootIsPreservedThroughTrappedOpenAndOneCommandCycle(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.LordBritishsCastle).
		WithRandomSeed(1).
		WithSystemCallbacks().
		Build()
	chest, direction := positionSceneBesideChest(t, state)
	require.Equal(t, references.Basement, chest.Floor())
	chest.RestoreSurfaceChestNumber(0x80 | 30)
	chestNumber := chest.MapUnitDetails().NPCNum
	character := state.PartyState.GetCharacter(0)
	character.SetStatus(party_state.Good)
	require.True(t, state.SetActivePlayer(0))
	target := direction.GetNewPositionInDirection(&state.MapState.PlayerLocation.Position)
	require.NotNil(t, target)
	existing := references.ItemStack{Items: references.ItemStacks{{Item: references.Gold, Quantity: 7}}}
	require.NoError(t, state.ItemStacksMap.Push(target, &existing))
	stackBefore, found := state.ItemStacksMap.Snapshot(target)
	require.True(t, found)
	karmaBefore := state.PartyState.Karma.Value
	goldBefore := state.PartyState.Inventory.Gold.Get()
	dateBefore := state.DateTime.DateOnly()
	hpBefore := character.GetCurrentHp()
	statusBefore := character.GetStatus()
	state.DebugOptions.MonsterGen = false
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	pressed := map[ebiten.Key]bool{directionKey(direction): true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(60_000, func(key ebiten.Key) bool { return pressed[key] })
	scene.keyboard.SetLastKeyPressedNowPlusMs(0)

	scene.smallMapInputHandler(ebiten.KeyO)
	scene.smallMapHandleSecondaryInput()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.NotContains(t, *state.CurrentNPCAIController.GetNpcs(), chest)
	assert.False(t, state.PartyState.IsNPCDead(references.LordBritishsCastle, chestNumber))
	index, daily := references.BasementChestIndex(chest.NPCReference, chestNumber)
	require.True(t, daily)
	require.NotNil(t, state.BasementChestHistory[index].LastOpened)
	assert.Equal(t, dateBefore, *state.BasementChestHistory[index].LastOpened)
	assert.Equal(t, max(karmaBefore-2, 0), state.PartyState.Karma.Value)
	assert.Equal(t, goldBefore, state.PartyState.Inventory.Gold.Get(), "Open leaves loot on the ground")
	assert.True(t, character.GetCurrentHp() < hpBefore || character.GetStatus() != statusBefore, "the actual trap affects the selected actor")
	stackAfter, found := state.ItemStacksMap.Snapshot(target)
	require.True(t, found)
	require.Greater(t, len(stackAfter.Items), len(stackBefore.Items))
	assert.Equal(t, stackBefore.Items, stackAfter.Items[len(stackAfter.Items)-len(stackBefore.Items):], "new allocations retain the existing pile and its pickup priority")
	assert.Equal(t, references.ItemAndQuantity{Item: references.Gold, Quantity: 7}, *state.ItemStacksMap.Peek(target))
	assert.Equal(t, 1, countEqual(callbacks.Messages, "Trapped!"))
	require.Contains(t, callbacks.Messages, "Found:")
	assert.Less(t, slices.Index(callbacks.Messages, "Trapped!"), slices.Index(callbacks.Messages, "Found:"))
	assert.Contains(t, callbacks.SoundEffectsPlayed, game_state.SoundTrapTrigger)
	assert.Contains(t, callbacks.SoundEffectsPlayed, game_state.SoundKarmaDown)
	assert.Equal(t, 1, countEqual(callbacks.Messages, "Opened! That chest isn't yours Avatar!"))
	assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
}

func positionSceneBesideChest(
	t *testing.T,
	state *game_state.GameState,
) (*map_units.NPCFriendly, references.Direction) {
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
	t.Fatal("test map has no surface chest")

	return nil, references.NoneDirection
}

func directionKey(direction references.Direction) ebiten.Key {
	switch direction {
	case references.Up:
		return ebiten.KeyUp
	case references.Down:
		return ebiten.KeyDown
	case references.Left:
		return ebiten.KeyLeft
	case references.Right:
		return ebiten.KeyRight
	case references.NoneDirection:
		panic("direction key requested for no direction")
	default:
		panic("direction key requested for invalid direction")
	}
}
