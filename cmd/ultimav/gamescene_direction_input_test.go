package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSurfaceDirectionPromptStaysConciseUntilResolution(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 0, 42)
	state.SystemCallbacks.Message.ShowCommandPrompt = scene.showCommandPrompt
	held := map[ebiten.Key]bool{ebiten.KeyL: true}
	scene.keyboard = input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
	calls := state.GetRNG().Calls()
	require.NoError(t, scene.Update(nil))
	require.Equal(t, LookDirectionInput, scene.secondaryKeyState)
	before := scene.output.GetOutputStr(true)
	require.Equal(t, "Look-", strings.TrimSpace(before))
	delete(held, ebiten.KeyL)
	for range 5 {
		require.NoError(t, scene.Update(nil))
	}
	held[ebiten.KeyX] = true
	require.NoError(t, scene.Update(nil))
	require.Equal(t, LookDirectionInput, scene.secondaryKeyState)
	require.Equal(t, before, scene.output.GetOutputStr(true), "idle and invalid keys cannot add guidance")
	require.Equal(t, calls, state.GetRNG().Calls())
	require.Empty(t, callbacks.TimeAdvanced())
	delete(held, ebiten.KeyX)
	scene.keyboard.ClearFrameConsumption()
	held[ebiten.KeyUp] = true
	require.Equal(t, ordinaryDirectionSelected, scene.ordinaryDirectionInput().Outcome)
	require.Contains(t, scene.output.GetOutputStr(true), "Look-North")
	require.NotContains(t, scene.output.GetOutputStr(true), ordinaryDirectionHint)
	// A retry remains just as concise as the first prompt.
	scene.showCommandPrompt("Look-")
	scene.appendOrdinaryDirectionHint(references.SmallMapType)
	require.NotContains(t, scene.output.GetOutputStr(true), ordinaryDirectionHint)
}

func TestOrdinaryDirectionHintExcludesOtherTargetingProtocols(t *testing.T) {
	scene := &GameScene{output: text.NewOutput(nil, 1, 14, 80)}
	for _, state := range []InputState{CombatAttackTargetInput, CombatSpellTargetInput, CombatConeTargetInput,
		CombatAdjacentSpellTargetInput, CombatKlimbLadderDirectionInput, SurfaceQuitInput, PrimaryInput} {
		scene.secondaryKeyState = state
		scene.appendOrdinaryDirectionHint(references.CombatMapType)
		require.Empty(t, scene.output.GetOutputStr(false))
	}
	scene.secondaryKeyState = LookDirectionInput
	scene.appendOrdinaryDirectionHint(references.DungeonMapType)
	require.Empty(t, scene.output.GetOutputStr(false), "dungeon Look uses its own Ahead/Here and Escape rules")
	scene.secondaryKeyState = OpenDirectionInput
	for range 2 {
		scene.appendOrdinaryDirectionHint(references.CombatMapType)
	}
	require.Equal(t, 1, strings.Count(scene.output.GetOutputStr(false), ordinaryDirectionHint),
		"ordinary combat direction guidance is unchanged")
}

func TestSmallMapKlimbProductionRoutingRejectsHorseBeforePrompt(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 0, 42)
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
	horse := map_units.NewNPCFriendlyVehiceNewRef(references.HorseVehicle, position, state.MapState.PlayerLocation.Floor)
	state.PartyVehicle = *horse
	callbacks.Reset()

	scene.smallMapInputHandler(ebiten.KeyK)

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, []string{"On foot!"}, callbacks.Messages)
	assert.Empty(t, callbacks.CommandPrompts)
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestSmallMapKlimbProductionRoutingPromptsWithoutWhat(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 0, 42)
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
	callbacks.Reset()

	scene.smallMapInputHandler(ebiten.KeyK)

	assert.Equal(t, KlimbDirectionInput, scene.secondaryKeyState)
	assert.Equal(t, []string{"Klimb-"}, callbacks.CommandPrompts)
	assert.NotContains(t, callbacks.Messages, "What?")
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestOrdinaryDirectionInputMatrix(t *testing.T) {
	tests := []struct {
		name      string
		key       ebiten.Key
		outcome   ordinaryDirectionOutcome
		direction references.Direction
	}{
		{name: "arrow up", key: ebiten.KeyUp, outcome: ordinaryDirectionSelected, direction: references.Up},
		{name: "arrow down", key: ebiten.KeyDown, outcome: ordinaryDirectionSelected, direction: references.Down},
		{name: "arrow left", key: ebiten.KeyLeft, outcome: ordinaryDirectionSelected, direction: references.Left},
		{name: "arrow right", key: ebiten.KeyRight, outcome: ordinaryDirectionSelected, direction: references.Right},
		{name: "keypad up", key: ebiten.KeyNumpad8, outcome: ordinaryDirectionSelected, direction: references.Up},
		{name: "keypad down", key: ebiten.KeyNumpad2, outcome: ordinaryDirectionSelected, direction: references.Down},
		{name: "keypad left", key: ebiten.KeyNumpad4, outcome: ordinaryDirectionSelected, direction: references.Left},
		{name: "keypad right", key: ebiten.KeyNumpad6, outcome: ordinaryDirectionSelected, direction: references.Right},
		{name: "space", key: ebiten.KeySpace, outcome: ordinaryDirectionPassed},
		{name: "escape", key: ebiten.KeyEscape, outcome: ordinaryDirectionActive},
		{name: "invalid letter", key: ebiten.KeyX, outcome: ordinaryDirectionActive},
		{name: "keypad diagonal 1", key: ebiten.KeyNumpad1, outcome: ordinaryDirectionActive},
		{name: "keypad diagonal 3", key: ebiten.KeyNumpad3, outcome: ordinaryDirectionActive},
		{name: "keypad diagonal 7", key: ebiten.KeyNumpad7, outcome: ordinaryDirectionActive},
		{name: "keypad diagonal 9", key: ebiten.KeyNumpad9, outcome: ordinaryDirectionActive},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene := &GameScene{
				keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
				output:   text.NewOutput(nil, 1, 10, 80),
			}
			scene.keyboard.SimulateKeyPress(test.key)

			result := scene.ordinaryDirectionInput()

			assert.Equal(t, test.outcome, result.Outcome)
			assert.Equal(t, test.direction, result.Direction)
		})
	}
}

func TestSurfaceDirectionPassCompletionOwnershipAndNextCommand(t *testing.T) {
	tests := []struct {
		name        string
		location    references.Location
		state       InputState
		handle      func(*GameScene)
		primary     func(*GameScene, ebiten.Key)
		wantMinutes []int
	}{
		{name: "town Open", location: references.Britain, state: OpenDirectionInput, handle: (*GameScene).smallMapHandleSecondaryInput, primary: (*GameScene).smallMapInputHandler, wantMinutes: []int{game_state.DefaultSmallMapMinutesPerTurn}},
		{name: "town Jimmy", location: references.Britain, state: JimmyDoorDirectionInput, handle: (*GameScene).smallMapHandleSecondaryInput, primary: (*GameScene).smallMapInputHandler, wantMinutes: []int{game_state.DefaultSmallMapMinutesPerTurn}},
		{name: "town Push", location: references.Britain, state: PushDirectionInput, handle: (*GameScene).smallMapHandleSecondaryInput, primary: (*GameScene).smallMapInputHandler, wantMinutes: []int{game_state.DefaultSmallMapMinutesPerTurn}},
		{name: "town Attack", location: references.Britain, state: AttackDirectionInput, handle: (*GameScene).smallMapHandleSecondaryInput, primary: (*GameScene).smallMapInputHandler, wantMinutes: []int{game_state.DefaultSmallMapMinutesPerTurn}},
		{name: "overworld Open", location: references.BritanniaUnderworld, state: OpenDirectionInput, handle: (*GameScene).largeMapHandleSecondaryInput, primary: (*GameScene).largeMapInputHandler, wantMinutes: []int{game_state.DefaultLargeMapMinutesPerTurn}},
		{name: "overworld Jimmy", location: references.BritanniaUnderworld, state: JimmyDoorDirectionInput, handle: (*GameScene).largeMapHandleSecondaryInput, primary: (*GameScene).largeMapInputHandler, wantMinutes: []int{game_state.DefaultLargeMapMinutesPerTurn}},
		{name: "overworld Push", location: references.BritanniaUnderworld, state: PushDirectionInput, handle: (*GameScene).largeMapHandleSecondaryInput, primary: (*GameScene).largeMapInputHandler, wantMinutes: []int{game_state.DefaultLargeMapMinutesPerTurn}},
		{name: "overworld Attack exception", location: references.BritanniaUnderworld, state: AttackDirectionInput, handle: (*GameScene).largeMapHandleSecondaryInput, primary: (*GameScene).largeMapInputHandler},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithPlayerAt(50, 50).
				WithSystemCallbacks().
				Build()
			state.DebugOptions.MonsterGen = false
			callbacks.Reset()
			scene := newSurfaceCompletionTestScene(state)
			scene.secondaryKeyState = test.state
			scene.keyboard.SimulateKeyPress(ebiten.KeySpace)

			test.handle(scene)

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, test.wantMinutes, callbacks.TimeAdvanced())
			beforePrompts := len(callbacks.CommandPrompts)
			test.primary(scene, ebiten.KeyZ)
			require.Len(t, callbacks.CommandPrompts, beforePrompts+1)
			assert.Equal(t, "Ztats...", callbacks.CommandPrompts[beforePrompts])
			assert.Equal(t, test.wantMinutes, callbacks.TimeAdvanced(), "next command must not inherit the prior cycle")
		})
	}
}

func TestRejectedOrdinaryDirectionInputDoesNotConsumeNextKey(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	scene.secondaryKeyState = GetDirectionInput

	for _, key := range []ebiten.Key{ebiten.KeyX, ebiten.KeyNumpad7} {
		scene.keyboard.SimulateKeyPress(key)
		scene.largeMapHandleSecondaryInput()
		assert.Equal(t, GetDirectionInput, scene.secondaryKeyState)
		assert.Empty(t, callbacks.TimeAdvanced())
	}

	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	scene.largeMapHandleSecondaryInput()
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
}

func TestOverworldPushProductionInputAcceptsKeypadAndSpaceDoesNotMutate(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	state.CurrentNPCAIController.GetNpcs().ClearAllMapUnits()
	actor := state.MapState.PlayerLocation.Position
	target := references.Position{X: actor.X + 1, Y: actor.Y}
	forward := references.Position{X: actor.X + 2, Y: actor.Y}
	largeMap := state.GetCurrentLayeredMap()
	largeMap.SetTileByLayer(map_state.MapLayer, &actor, indexes.BrickFloor)
	largeMap.SetTileByLayer(map_state.MapLayer, &target, indexes.Barrel)
	largeMap.SetTileByLayer(map_state.MapLayer, &forward, indexes.BrickFloor)
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	scene.secondaryKeyState = PushDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)

	scene.largeMapHandleSecondaryInput()

	assert.Equal(t, actor, state.MapState.PlayerLocation.Position)
	assert.Equal(t, indexes.SpriteIndex(indexes.Barrel), largeMap.SpriteIndexByLayer(map_state.MapLayer, target))
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())

	callbacks.Reset()
	scene.secondaryKeyState = PushDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeyNumpad6)
	scene.largeMapHandleSecondaryInput()
	assert.Equal(t, target, state.MapState.PlayerLocation.Position)
	assert.Equal(t, indexes.SpriteIndex(indexes.Barrel), largeMap.SpriteIndexByLayer(map_state.MapLayer, forward))
	assert.Equal(t, []int{game_state.DefaultLargeMapMinutesPerTurn}, callbacks.TimeAdvanced())
}

func TestSpelldirWorkflowIgnoresEscapeAndDiagonalThenPassesOnSpace(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	scene := newSurfaceCompletionTestScene(state)
	passed := false
	state.EventQueueManager.Workflow("spelldir_input_test").
		Priority(event_queue.PriorityNormal).
		Sequential().
		ThenSequence(scene.thenShowDirectionPrompt(
			"Direction-",
			"Arrow/keypad or Space",
			func(references.Direction) { t.Fatal("unexpected direction") },
			func() { passed = true },
		)).
		Finally(func() {}).
		Start()
	state.EventQueueManager.Update()
	require.True(t, state.EventQueueManager.HasWorkflow("spelldir_input_test"))

	for _, key := range []ebiten.Key{ebiten.KeyEscape, ebiten.KeyX, ebiten.KeyNumpad7} {
		scene.keyboard.SimulateKeyPress(key)
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		assert.True(t, state.EventQueueManager.HasWorkflow("spelldir_input_test"))
		assert.False(t, passed)
	}

	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	for range 5 {
		state.ElapsedMs += DialogPollingRateMs
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("spelldir_input_test") {
			break
		}
	}
	assert.True(t, passed)
	assert.False(t, state.EventQueueManager.HasWorkflow("spelldir_input_test"))
	assert.Zero(t, scene.dialogStack.Count())
}

func TestPrimaryCardinalNumpadMovementNormalization(t *testing.T) {
	assert.Equal(t, ebiten.KeyDown, primaryMovementKey(ebiten.KeyNumpad2))
	assert.Equal(t, ebiten.KeyLeft, primaryMovementKey(ebiten.KeyNumpad4))
	assert.Equal(t, ebiten.KeyRight, primaryMovementKey(ebiten.KeyNumpad6))
	assert.Equal(t, ebiten.KeyUp, primaryMovementKey(ebiten.KeyNumpad8))
	assert.Equal(t, ebiten.KeyNumpad7, primaryMovementKey(ebiten.KeyNumpad7))
	assert.Equal(t, ebiten.Key8, primaryMovementKey(ebiten.Key8))
}

func TestCombatOrdinaryDirectionPassConsumesActionAndKlimbFailureReprompts(t *testing.T) {
	for _, commandState := range []InputState{
		OpenDirectionInput,
		JimmyDoorDirectionInput,
		PushDirectionInput,
		GetDirectionInput,
		SearchDirectionInput,
	} {
		t.Run(commandStateName(commandState), func(t *testing.T) {
			scene, _ := newCombatUseTestScene(t)
			scene.secondaryKeyState = commandState
			scene.keyboard.SimulateKeyPress(ebiten.KeySpace)

			scene.combatMapHandleSecondaryInput()

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
		})
	}

}

func TestCombatKlimbInvalidCardinalRepromptsWithoutCompletingAction(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	identity := scene.gameState.CombatScene.GetCurrentTurnIdentity()
	origin := scene.gameState.CombatScene.GetCombatantPosition(identity)
	target := references.Up.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)
	scene.gameState.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, target, indexes.BrickFloor)
	callbacks.Reset()
	before := snapshotBlockedCombatState(t, scene.gameState)
	scene.secondaryKeyState = KlimbDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeyNumpad8)

	scene.combatMapHandleSecondaryInput()

	assert.Equal(t, []string{"What?"}, callbacks.Messages)
	assert.Equal(t, []string{"Klimb-"}, callbacks.CommandPrompts)
	assert.Equal(t, KlimbDirectionInput, scene.secondaryKeyState)
	assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Empty(t, callbacks.CombatEvents)
}

func TestCombatKlimbSpacePassCompletesExactlyOneAction(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	expected, expectedCallbacks := newCombatUseTestScene(t)
	callbacks.Reset()
	scene.secondaryKeyState = KlimbDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)

	scene.combatMapHandleSecondaryInput()
	expected.gameState.FinishCombatAction()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, snapshotBlockedCombatState(t, expected.gameState), snapshotBlockedCombatState(t, scene.gameState))
	assert.Contains(t, scene.output.GetOutputStr(true), "Pass")
	assert.Equal(t, expectedCallbacks.Messages, callbacks.Messages)
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestCombatKlimbSuccessfulRockClimbCompletesExactlyOneAction(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	expected, expectedCallbacks := newCombatUseTestScene(t)
	identity := scene.gameState.CombatScene.GetCurrentTurnIdentity()
	origin := scene.gameState.CombatScene.GetCombatantPosition(identity)
	target := references.Right.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)
	for _, fixture := range []*GameScene{scene, expected} {
		fixture.gameState.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, target, indexes.SmallRockWall)
	}
	callbacks.Reset()
	expectedCallbacks.Reset()
	scene.secondaryKeyState = KlimbDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeyRight)

	scene.combatMapHandleSecondaryInput()
	require.True(t, expected.gameState.ActionKlimbCombatMap(references.Right))
	expected.gameState.FinishCombatAction()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, *target, scene.gameState.CombatScene.GetPartyMemberPosition(identity.EntityIndex))
	assert.Equal(t, snapshotBlockedCombatState(t, expected.gameState), snapshotBlockedCombatState(t, scene.gameState))
	assert.Equal(t, expectedCallbacks.CombatEvents, callbacks.CombatEvents)
	moveEvents := 0
	for _, event := range callbacks.CombatEvents {
		if event.Kind == game_state.CombatEventMove {
			moveEvents++
		}
	}
	assert.Equal(t, 1, moveEvents)
}

func TestCombatKlimbDualLadderUsesDedicatedDirectionInput(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	scene.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
	var cells references.DungeonCells
	source := references.Position{X: 4, Y: 4}
	require.True(t, cells.SetCell(0, source, references.DungeonCell(0x30)))
	combatMap, err := references.BuildGeneratedDungeonCombatMap(
		references.Deceit, &cells, 0, source, references.DungeonFacingNorth,
	)
	require.NoError(t, err)
	scene.gameState.CombatScene.CombatMapRef = combatMap
	identity := scene.gameState.CombatScene.GetCurrentTurnIdentity()
	center := references.Position{X: 5, Y: 5}
	scene.gameState.CombatScene.SetPartyMemberPosition(identity.EntityIndex, center)
	scene.gameState.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &center, indexes.LadderUp)
	callbacks.Reset()

	scene.combatMapInputHandler(ebiten.KeyK)

	assert.Equal(t, CombatKlimbLadderDirectionInput, scene.secondaryKeyState)
	assert.Contains(t, scene.output.GetOutputStr(true), "Klimb-U/D-")
	beforePass := snapshotBlockedCombatState(t, scene.gameState)
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	scene.combatMapHandleSecondaryInput()
	assert.Equal(t, CombatKlimbLadderDirectionInput, scene.secondaryKeyState)
	assert.Equal(t, beforePass, snapshotBlockedCombatState(t, scene.gameState))
	assert.Empty(t, callbacks.Messages)
	assert.Empty(t, callbacks.CommandPrompts)

	scene.keyboard.SimulateKeyPress(ebiten.KeyU)
	scene.combatMapHandleSecondaryInput()
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.NotEqual(t, identity, scene.gameState.CombatScene.GetCurrentTurnIdentity())
	assert.False(t, scene.gameState.CombatScene.PartyActive[identity.EntityIndex])
}

func TestCombatKlimbSelectedLadderFailureRepromptsSameExactAction(t *testing.T) {
	tests := []struct {
		name        string
		dual        bool
		prepare     func(*game_state.GameState)
		wantMessage string
	}{
		{
			name: "single ladder ship allegiance", wantMessage: "Stay with ship!",
			prepare: func(state *game_state.GameState) {
				state.PartyVehicle = *map_units.NewNPCFriendlyVehiceNewRef(
					references.FrigateVehicle,
					state.MapState.PlayerLocation.Position,
					state.MapState.PlayerLocation.Floor,
				)
			},
		},
		{
			name: "dual ladder incompatible route", dual: true, wantMessage: "All must use the same exit!",
			prepare: func(state *game_state.GameState) {
				state.CombatScene.ExitContext = game_state.CombatExitDungeonRoom
				state.CombatScene.ExitRoute = game_state.CombatExitDown
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, callbacks := newCombatUseTestScene(t)
			scene.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
			identity := scene.gameState.CombatScene.GetCurrentTurnIdentity()
			center := references.Position{X: 5, Y: 5}
			scene.gameState.CombatScene.SetPartyMemberPosition(identity.EntityIndex, center)
			scene.gameState.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &center, indexes.LadderUp)
			if test.dual {
				var cells references.DungeonCells
				source := references.Position{X: 4, Y: 4}
				require.True(t, cells.SetCell(0, source, references.DungeonCell(0x30)))
				combatMap, err := references.BuildGeneratedDungeonCombatMap(
					references.Deceit, &cells, 0, source, references.DungeonFacingNorth,
				)
				require.NoError(t, err)
				scene.gameState.CombatScene.CombatMapRef = combatMap
			}
			test.prepare(scene.gameState)
			callbacks.Reset()
			before := snapshotBlockedCombatState(t, scene.gameState)

			scene.combatMapInputHandler(ebiten.KeyK)
			if test.dual {
				require.Equal(t, CombatKlimbLadderDirectionInput, scene.secondaryKeyState)
				scene.keyboard.SimulateKeyPress(ebiten.KeyU)
				scene.combatMapHandleSecondaryInput()
			}

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
			assert.Contains(t, callbacks.Messages, test.wantMessage)
			assert.Equal(t, []string{""}, callbacks.CommandPrompts)
			assert.Empty(t, callbacks.TimeAdvanced())
		})
	}
}

func TestCombatCrosshairKeepsDistinctDiagonalAndEscapeInput(t *testing.T) {
	scene := &GameScene{keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
	scene.keyboard.SimulateKeyPress(ebiten.KeyNumpad7)

	dx, dy, moved := scene.getCombatTargetMovement()

	assert.True(t, moved)
	assert.Equal(t, -1, dx)
	assert.Equal(t, -1, dy)

	combatScene, _ := newCombatUseTestScene(t)
	require.True(t, combatScene.gameState.BeginCombatAttack())
	combatScene.secondaryKeyState = CombatAttackTargetInput
	combatScene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	combatScene.combatMapHandleSecondaryInput()
	assert.Equal(t, PrimaryInput, combatScene.secondaryKeyState)
}

func commandStateName(state InputState) string {
	switch state {
	case OpenDirectionInput:
		return "Open"
	case JimmyDoorDirectionInput:
		return "Jimmy"
	case PushDirectionInput:
		return "Push"
	case GetDirectionInput:
		return "Get"
	case SearchDirectionInput:
		return "Search"
	default:
		return "unknown"
	}
}

func TestOverworldKlimbFallEndsTheTurnAfterItsDamage(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(50, 50).
		WithSystemCallbacks().
		Build()
	state.DebugOptions.MonsterGen = false
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
	avatar := state.PartyState.GetCharacter(0)
	avatar.Dexterity = 0 // Every climbing check fails.
	hitPoints := avatar.GetCurrentHp()
	peak := references.Position{X: 50, Y: 49}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &peak, indexes.SmallMountains)
	callbacks.Reset()
	scene := newSurfaceCompletionTestScene(state)
	scene.secondaryKeyState = KlimbDirectionInput
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)

	scene.largeMapHandleSecondaryInput()

	assert.Empty(t, callbacks.TimeAdvanced(), "the turn waits for the fall")
	assert.Equal(t, hitPoints, avatar.GetCurrentHp())
	for range 200 {
		state.ElapsedMs += 50
		state.EventQueueManager.Update()
	}
	assert.Less(t, avatar.GetCurrentHp(), hitPoints)
	assert.NotEmpty(t, callbacks.TimeAdvanced(), "the turn ends once the damage has landed")
}
