package main

import (
	"errors"
	"image"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func newDungeonInputTestScene(
	t *testing.T,
) (*GameScene, *game_state.GameState, *game_state.MockSystemCallbacks) {
	t.Helper()

	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Deceit).
		WithFloor(0).
		WithPlayerAt(1, 1).
		WithRandomSeed(253).
		WithSystemCallbacks().
		Build()
	state.DateTime.Hour = 12
	if state.MoongateStage() > game_state.MoongateStageHidden {
		state.DateTime.Hour = 20
	}
	state.DateTime.Minute = 0
	state.PartyState.Inventory.Provisions.Torches.Set(1)
	setDungeonInputTestCell(t, state, references.Position{X: 1, Y: 1}, 0x00)
	setDungeonInputTestCell(t, state, references.Position{X: 2, Y: 1}, 0x00)
	setDungeonInputTestCell(t, state, references.Position{X: 0, Y: 1}, 0x00)
	callbacks.Reset()

	return &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet: &sprites.SpriteSheet{},
	}, state, callbacks
}

func setDungeonInputTestCell(
	t *testing.T,
	state *game_state.GameState,
	position references.Position,
	cell references.DungeonCell,
) {
	setDungeonInputTestCellAtFloor(t, state, 0, position, cell)
}

func setDungeonInputTestCellAtFloor(
	t *testing.T,
	state *game_state.GameState,
	floor references.FloorNumber,
	position references.Position,
	cell references.DungeonCell,
) {
	t.Helper()

	_, err := state.TransformDungeonCell(floor, position, func(references.DungeonCell) references.DungeonCell {
		return cell
	})
	require.NoError(t, err)
}

func dungeonInputCellAt(
	t *testing.T,
	state *game_state.GameState,
	position references.Position,
) references.DungeonCell {
	t.Helper()
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	cell, ok := snapshot.Cells.Cell(state.MapState.PlayerLocation.Floor, position)
	require.True(t, ok)

	return cell
}

func dungeonInputFacing(t *testing.T, state *game_state.GameState) references.DungeonFacing {
	t.Helper()

	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)

	return snapshot.Facing
}

func TestDungeonPrimaryCommandsCompleteAtCommandBoundary(t *testing.T) {
	tests := []struct {
		name         string
		key          ebiten.Key
		prompt       string
		wantPosition references.Position
		wantFacing   references.DungeonFacing
		wantTorches  uint16
	}{
		{
			name: "advance", key: ebiten.KeyUp, prompt: "Advance",
			wantPosition: references.Position{X: 2, Y: 1}, wantFacing: references.DungeonFacingEast, wantTorches: 1,
		},
		{
			name: "back up", key: ebiten.KeyDown, prompt: "Back up",
			wantPosition: references.Position{X: 0, Y: 1}, wantFacing: references.DungeonFacingEast, wantTorches: 1,
		},
		{
			name: "turn left", key: ebiten.KeyLeft, prompt: "Turn left",
			wantPosition: references.Position{X: 1, Y: 1}, wantFacing: references.DungeonFacingNorth, wantTorches: 1,
		},
		{
			name: "turn right", key: ebiten.KeyRight, prompt: "Turn right",
			wantPosition: references.Position{X: 1, Y: 1}, wantFacing: references.DungeonFacingSouth, wantTorches: 1,
		},
		{
			name: "return turns around", key: ebiten.KeyEnter, prompt: "Turn around.",
			wantPosition: references.Position{X: 1, Y: 1}, wantFacing: references.DungeonFacingWest, wantTorches: 1,
		},
		{
			name: "period turns around", key: ebiten.KeyPeriod, prompt: "Turn around.",
			wantPosition: references.Position{X: 1, Y: 1}, wantFacing: references.DungeonFacingWest, wantTorches: 1,
		},
		{
			name: "ignite", key: ebiten.KeyI, prompt: "Ignite torch!",
			wantPosition: references.Position{X: 1, Y: 1}, wantFacing: references.DungeonFacingEast, wantTorches: 0,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)

			scene.dungeonMapInputHandler(test.key)

			assert.Equal(t, byte(1), state.DateTime.Minute, "each accepted key must precharge exactly once")
			assert.Equal(t, test.wantPosition, state.MapState.PlayerLocation.Position)
			assert.Equal(t, test.wantFacing, dungeonInputFacing(t, state))
			assert.Equal(t, test.wantTorches, state.PartyState.Inventory.Provisions.Torches.Get())
			if test.prompt == "" {
				assert.Empty(t, callbacks.CommandPrompts)
			} else {
				assert.Equal(t, []string{test.prompt}, callbacks.CommandPrompts)
			}
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestDungeonCommandPrechargeHonorsQuicknessAndTimeStop(t *testing.T) {
	t.Run("Quickness alternates exact precharge", func(t *testing.T) {
		scene, state, _ := newDungeonInputTestScene(t)
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType:     game_state.DurationEffectQuickness,
			TurnsRemaining: 10,
		}

		scene.dungeonMapInputHandler(ebiten.KeyLeft)
		assert.Equal(t, byte(1), state.DateTime.Minute)
		assert.True(t, state.IsAtDungeonCommandBoundary())

		scene.dungeonMapInputHandler(ebiten.KeyRight)
		assert.Equal(t, byte(1), state.DateTime.Minute, "second Quickness cycle must be free")
		assert.Equal(t, 10, state.ActiveDurationEffect.TurnsRemaining, "turns are command-only")
		assert.True(t, state.IsAtDungeonCommandBoundary())
	})

	t.Run("Time Stop suppresses precharge", func(t *testing.T) {
		scene, state, _ := newDungeonInputTestScene(t)
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType:     game_state.DurationEffectTimeStop,
			TurnsRemaining: 5,
		}

		scene.dungeonMapInputHandler(ebiten.KeyUp)

		assert.Zero(t, state.DateTime.Minute)
		assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining, "advance still receives full completion")
		assert.True(t, state.IsAtDungeonCommandBoundary())
	})
}

func TestDungeonBlockedAdvanceReceivesFullCompletion(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	setDungeonInputTestCell(t, state, references.Position{X: 2, Y: 1}, 0xb0)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	callbacks.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyUp)

	assert.Equal(t, references.Position{X: 1, Y: 1}, state.MapState.PlayerLocation.Position)
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, []string{"Advance"}, callbacks.CommandPrompts)
	assert.Contains(t, callbacks.Messages, "Blocked!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonDoorwayRefusalReceivesCommandOnlyCompletion(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	setDungeonInputTestCell(t, state, references.Position{X: 1, Y: 1}, 0xe8)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	callbacks.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyLeft)

	assert.Equal(t, references.DungeonFacingEast, dungeonInputFacing(t, state))
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
	assert.Empty(t, callbacks.CommandPrompts)
	assert.Contains(t, callbacks.Messages, "Not in doorway!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonIgniteSuccessAndFailureReceiveFullCompletion(t *testing.T) {
	for _, torches := range []uint16{1, 0} {
		name := "success"
		if torches == 0 {
			name = "failure"
		}
		t.Run(name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			state.PartyState.Inventory.Provisions.Torches.Set(torches)
			state.MapState.Lighting.RestoreTorchMinutes(10)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.dungeonMapInputHandler(ebiten.KeyI)

			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, []string{"Ignite torch!"}, callbacks.CommandPrompts,
				"ActionIgnite must own the only command prompt")
			assert.True(t, state.IsAtDungeonCommandBoundary())
			if torches == 0 {
				assert.Equal(t, 9, state.MapState.Lighting.TorchMinutesRemaining())
				assert.Contains(t, callbacks.Messages, "None owned!")
			} else {
				assert.Zero(t, state.PartyState.Inventory.Provisions.Torches.Get())
				assert.GreaterOrEqual(t, state.MapState.Lighting.TorchMinutesRemaining(), 121)
				assert.LessOrEqual(t, state.MapState.Lighting.TorchMinutesRemaining(), 136)
			}
		})
	}
}

func TestDungeonSearchPromptsForCharacterBeforeDarknessAndDirection(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	state.MapState.Lighting.RestoreTorchMinutes(0)
	state.MagicLightMinutesRemaining = 0
	state.MapState.Lighting.SetMagicLight(false)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	callbacks.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyS)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleEnter()

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
	assert.Equal(t, []string{"You find darkness."}, callbacks.Messages)
	assert.Equal(t, []string{"Search..."}, callbacks.CommandPrompts)
}

func TestDungeonAttackInputStartsCombatAgainstAdjacentWanderer(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)
	target := references.Position{X: 2, Y: 1}
	setDungeonInputTestCell(t, state, target, 0x00)
	scene, state, callbacks := hydrateDungeonInputSceneWithWanderer(t, scene, state, references.MonsterRat, target)
	callbacks.Reset()
	beforeReturn := state.MapState.PlayerLocation

	scene.dungeonMapInputHandler(ebiten.KeyA)

	require.NotNil(t, state.CombatScene)
	require.NotNil(t, state.CombatScene.DungeonWandererOrigin)
	assert.Equal(t, game_state.DungeonWandererEncounterDirect, state.CombatScene.DungeonWandererOrigin.Kind)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, []string{"Attack"}, callbacks.CommandPrompts)
	assert.NotContains(t, callbacks.Messages, "What?")
	beforeReturnRNG := state.GetRNG().Calls()
	state.CombatScene.ExitRoute = game_state.CombatExitWest
	state.EndCombat(game_state.CombatVictory)

	assert.Equal(t, beforeReturn, state.MapState.PlayerLocation, "direct Attack ignores cardinal combat exits")
	assert.Greater(t, state.GetRNG().Calls(), beforeReturnRNG, "return attempts replacement generation")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonAdvanceInputMovesWanderer(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)
	wandererPosition := references.Position{X: 4, Y: 4}
	for _, position := range []references.Position{
		{X: 4, Y: 3}, {X: 5, Y: 4}, {X: 4, Y: 5}, {X: 3, Y: 4},
	} {
		setDungeonInputTestCell(t, state, position, 0x00)
	}
	scene, state, _ = hydrateDungeonInputSceneWithWanderer(
		t, scene, state, references.MonsterRat, wandererPosition,
	)
	state.SetRandomSeed(265)

	scene.dungeonMapInputHandler(ebiten.KeyUp)

	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)
	require.NotNil(t, snapshot.Wanderer)
	assert.NotEqual(t, wandererPosition, snapshot.Wanderer.Position)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonUpdateRoutesElectricFieldBounceAndDamage(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	target := references.Position{X: 2, Y: 1}
	setDungeonInputTestCell(t, state, target, 0x83)
	beforeHP := make(map[int]uint16)
	for index := 0; index < state.PartyState.GetCharacterCount(); index++ {
		character := state.PartyState.GetCharacter(index)
		if character.PartyMap != party_state.PartyMapActive {
			continue
		}
		character.SetMaxHp(100)
		character.SetHp(100)
		character.SetStatus(party_state.Good)
		beforeHP[index] = character.GetCurrentHp()
	}
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	require.NoError(t, scene.Update(nil))

	assert.Equal(t, references.Position{X: 1, Y: 1}, state.MapState.PlayerLocation.Position)
	assert.Contains(t, callbacks.Messages, "Ouch!")
	assert.Contains(t, callbacks.Messages, "Electric field!")
	assert.NotContains(t, callbacks.Messages, "Blocked!")
	for index, hp := range beforeHP {
		assert.Less(t, state.PartyState.GetCharacter(index).GetCurrentHp(), hp)
	}
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonFullCommandInputStartsCombatWhenWandererMakesContact(t *testing.T) {
	const seed = uint64(3265)
	scene, state, _ := newDungeonInputTestScene(t)
	attackOrigin := references.Position{X: 3, Y: 3}
	party := dungeonInputContactPartyPosition(seed, attackOrigin)
	_, err := state.DebugSetDungeonPosition(0, party)
	require.NoError(t, err)
	setDungeonInputTestCell(t, state, party, 0x00)
	setDungeonInputTestCell(t, state, attackOrigin, 0x00)
	scene, state, callbacks := hydrateDungeonInputSceneWithWanderer(
		t, scene, state, references.MonsterGhost, attackOrigin,
	)
	state.SetRandomSeed(seed)
	callbacks.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyI)

	assert.Nil(t, state.CombatScene)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	for range 10 {
		state.EventQueueManager.Update()
		state.ElapsedMs += 100
		if state.CombatScene != nil {
			break
		}
	}

	require.NotNil(t, state.CombatScene)
	require.NotNil(t, state.CombatScene.DungeonWandererOrigin)
	assert.Equal(t, game_state.DungeonWandererEncounterContact, state.CombatScene.DungeonWandererOrigin.Kind)
	require.NotEmpty(t, callbacks.Messages)
	assert.Contains(t, callbacks.Messages[0], "Attacked from the")
	beforeReturnRNG := state.GetRNG().Calls()
	state.CombatScene.ExitRoute = game_state.CombatExitWest
	state.EndCombat(game_state.CombatVictory)
	wantPosition := references.Left.GetNewPositionInDirection(&party).GetWrapped(
		references.XDungeonMapTiles,
		references.YDungeonMapTiles,
	)

	assert.Equal(t, *wantPosition, state.MapState.PlayerLocation.Position)
	assert.Greater(t, state.GetRNG().Calls(), beforeReturnRNG, "return attempts replacement generation")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func hydrateDungeonInputSceneWithWanderer(
	t *testing.T,
	scene *GameScene,
	state *game_state.GameState,
	monster references.MonsterType,
	position references.Position,
) (*GameScene, *game_state.GameState, *game_state.MockSystemCallbacks) {
	t.Helper()

	saved, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	require.NotNil(t, saved.Dungeon)
	saved.Dungeon.Wanderer = &save_snapshot.DungeonWandererSnapshot{
		Monster: int(monster), Floor: int(state.MapState.PlayerLocation.Floor),
		X: int(position.X), Y: int(position.Y), PreviousX: int(position.X), PreviousY: int(position.Y),
		OnCeiling: false,
	}
	hydrated, err := game_state.NewGameStateFromSaveSnapshot(
		saved,
		state.GameConfig,
		state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen,
		state.MapState.YTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	callbacks := game_state.NewMockSystemCallbacks(t)
	hydrated.SystemCallbacks = callbacks.ToSystemCallbacks()
	hydrated.SetupEnvironmentalSystemCallbacks()
	scene.gameState = hydrated

	return scene, hydrated, callbacks
}

func dungeonInputContactPartyPosition(seed uint64, origin references.Position) references.Position {
	// This maps the first source movement roll to its cardinal direction so the
	// hydrated wanderer attempts contact during the next full command cycle.
	switch beradrng.New(seed).Intn(4) {
	case 0:
		return references.Position{X: origin.X, Y: origin.Y - 1}
	case 1:
		return references.Position{X: origin.X + 1, Y: origin.Y}
	case 2:
		return references.Position{X: origin.X, Y: origin.Y + 1}
	case 3:
		return references.Position{X: origin.X - 1, Y: origin.Y}
	default:
		panic("invalid deterministic dungeon wanderer direction")
	}
}

func TestDungeonSearchUsesActiveCharacterWithoutSelector(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	require.True(t, state.SetActivePlayer(1))
	state.MapState.Lighting.RestoreTorchMinutes(10)
	callbacks.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyS)

	assert.Nil(t, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, 1, scene.dungeonSearchCharacterIndex)
	assert.Equal(t, SearchDirectionInput, scene.secondaryKeyState)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, []string{"Search..."}, callbacks.CommandPrompts)
}

func TestDungeonSearchRetainsCycleUntilDirectionOrPass(t *testing.T) {
	for _, test := range []struct {
		name        string
		key         ebiten.Key
		wantCell    references.DungeonCell
		wantMessage string
	}{
		{name: "ahead", key: ebiten.KeyUp, wantCell: 0xe0, wantMessage: "Search-a hidden door!"},
		{name: "keypad ahead", key: ebiten.KeyNumpad8, wantCell: 0xe0, wantMessage: "Search-a hidden door!"},
		{name: "pass", key: ebiten.KeySpace, wantCell: 0xd0, wantMessage: "Pass"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			state.MapState.Lighting.RestoreTorchMinutes(10)
			target := references.Position{X: 2, Y: 1}
			setDungeonInputTestCell(t, state, target, 0xd0)
			callbacks.Reset()

			initializeDungeonUpdateTestScene(scene, state)
			scene.keyboard.SimulateKeyPress(ebiten.KeyS)
			require.NoError(t, scene.Update(nil))
			_, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
			require.True(t, ok)
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			require.NoError(t, scene.Update(nil))
			assert.Equal(t, SearchDirectionInput, scene.secondaryKeyState)
			assert.False(t, state.IsAtDungeonCommandBoundary())
			assert.Equal(t, byte(1), state.DateTime.Minute)

			scene.keyboard.SimulateKeyPress(test.key)
			require.NoError(t, scene.Update(nil))

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
			assert.Contains(t, callbacks.Messages, test.wantMessage)
			snapshot, active := state.DungeonStateSnapshot()
			require.True(t, active)
			cell, ok := snapshot.Cells.Cell(0, target)
			require.True(t, ok)
			assert.Equal(t, test.wantCell, cell)
		})
	}
}

func TestDungeonChestCommandsRouteFromRealInput(t *testing.T) {
	t.Run("Open retains cycle through character selection", func(t *testing.T) {
		scene, state, _ := newDungeonInputTestScene(t)
		initializeDungeonUpdateTestScene(scene, state)
		setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, 0x40)

		scene.keyboard.SimulateKeyPress(ebiten.KeyO)
		require.NoError(t, scene.Update(nil))
		assert.False(t, state.IsAtDungeonCommandBoundary())
		assert.IsType(t, &widgets.CharacterSelectionDialog{}, scene.dialogStack.PeekTopModalDialog())

		scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
		require.NoError(t, scene.Update(nil))

		assert.True(t, state.IsAtDungeonCommandBoundary())
		assert.Equal(t, references.DungeonCell(0x70), dungeonInputCellAt(t, state, state.MapState.PlayerLocation.Position))
		assert.Equal(t, byte(1), state.DateTime.Minute)
		assert.Zero(t, scene.dialogStack.Count())
	})

	t.Run("Jimmy retains key after successful disarm", func(t *testing.T) {
		scene, state, _ := newDungeonInputTestScene(t)
		setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, 0x47)
		state.PartyState.GetCharacter(0).Dexterity = 255
		state.PartyState.Inventory.Provisions.Keys.Set(1)

		scene.dungeonMapInputHandler(ebiten.KeyJ)
		dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
		require.True(t, ok)
		dialog.HandleEnter()

		assert.True(t, state.IsAtDungeonCommandBoundary())
		assert.Equal(t, references.DungeonCell(0x40), dungeonInputCellAt(t, state, state.MapState.PlayerLocation.Position))
		assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
	})

	t.Run("Get immediately consumes open chest", func(t *testing.T) {
		scene, state, _ := newDungeonInputTestScene(t)
		setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, 0x70)

		scene.dungeonMapInputHandler(ebiten.KeyG)

		assert.True(t, state.IsAtDungeonCommandBoundary())
		assert.Equal(t, references.DungeonCell(0x00), dungeonInputCellAt(t, state, state.MapState.PlayerLocation.Position))
		assert.Equal(t, byte(1), state.DateTime.Minute)
	})
}

func TestDungeonLookRetainsCycleThroughFountainChoice(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	state.MapState.Lighting.RestoreTorchMinutes(10)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Poisoned)
	target := references.Position{X: 2, Y: 1}
	setDungeonInputTestCell(t, state, target, 0x50)

	scene.dungeonMapInputHandler(ebiten.KeyL)
	characterDialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	characterDialog.HandleEnter()
	assert.Equal(t, LookDirectionInput, scene.secondaryKeyState)
	assert.False(t, state.IsAtDungeonCommandBoundary())

	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	scene.dungeonMapHandleSecondaryInput(nil)
	var fountainDialog *widgets.InteractionTranscript
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if current, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			current.ActivePrompt() == "Drink?" {
			fountainDialog = current

			break
		}
	}
	require.NotNil(t, fountainDialog)
	assert.Contains(t, fountainDialog.TranscriptText(), dungeonFountainIntroduction)
	assert.NotContains(t, fountainDialog.TranscriptText(), "You see:")
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, 1, callbacks.BlockInputCalls)

	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyY)
	fountainDialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	fountainDialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if strings.Contains(fountainDialog.TranscriptText(), "Cured!") && !fountainDialog.InputComplete() {
			break
		}
	}

	assert.True(t, state.PartyState.GetCharacter(0).IsGood())
	assert.Contains(t, fountainDialog.TranscriptText(), "Drink?  > Yes")
	assert.Contains(t, fountainDialog.TranscriptText(), "Yes. Gulp!")
	assert.Contains(t, fountainDialog.TranscriptText(), "Cured!")
	assert.False(t, state.IsAtDungeonCommandBoundary())
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	fountainDialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow(dungeonFountainWorkflowName) {
			break
		}
	}
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, references.DungeonCell(0x50), dungeonInputCellAt(t, state, target))
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestDungeonLookFountainRefusalCompletesWithoutDrinking(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	state.MapState.Lighting.RestoreTorchMinutes(10)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Poisoned)
	target := references.Position{X: 2, Y: 1}
	setDungeonInputTestCell(t, state, target, 0x50)

	scene.dungeonMapInputHandler(ebiten.KeyL)
	characterDialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	characterDialog.HandleEnter()
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	scene.dungeonMapHandleSecondaryInput(nil)

	var fountainDialog *widgets.InteractionTranscript
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if current, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			current.ActivePrompt() == "Drink?" {
			fountainDialog = current

			break
		}
	}
	require.NotNil(t, fountainDialog)
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyN)
	fountainDialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	fountainDialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if fountainDialog.ActivePrompt() == "" && !fountainDialog.InputComplete() {
			break
		}
	}

	require.True(t, state.PartyState.GetCharacter(0).IsPoisoned())
	require.Contains(t, fountainDialog.TranscriptText(), "Drink?  > No")
	require.NotContains(t, callbacks.Messages, "Yes. Gulp!")
	require.False(t, state.IsAtDungeonCommandBoundary())
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	fountainDialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow(dungeonFountainWorkflowName) {
			break
		}
	}

	require.True(t, state.IsAtDungeonCommandBoundary())
	require.Equal(t, references.DungeonCell(0x50), dungeonInputCellAt(t, state, target))
	require.False(t, scene.dialogStack.HasOpenDialog())
}

func TestDungeonCharacterSelectionExcludesCharmedMembers(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)
	require.GreaterOrEqual(t, state.PartyState.GetCharacterCount(), 2)
	state.MapState.Lighting.RestoreTorchMinutes(10)
	state.PartyState.GetCharacter(0).SetStatus(party_state.Charmed)
	state.PartyState.GetCharacter(1).PartyMap = party_state.PartyMapActive
	state.PartyState.GetCharacter(1).SetStatus(party_state.Good)

	scene.dungeonMapInputHandler(ebiten.KeyS)
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleEnter()

	assert.Equal(t, 1, scene.dungeonSearchCharacterIndex)
	assert.Equal(t, SearchDirectionInput, scene.secondaryKeyState)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	scene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	scene.dungeonMapHandleSecondaryInput(nil)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonCharacterSelectionCancellationCompletesCommand(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)

	scene.dungeonMapInputHandler(ebiten.KeyJ)
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.CharacterSelectionDialog)
	require.True(t, ok)
	dialog.HandleEscape()

	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dialogStack.Count())
	assert.Equal(t, byte(1), state.DateTime.Minute)
}

func TestDungeonContextualImmediateCommandsRouteFromUpdate(t *testing.T) {
	tests := []struct {
		name         string
		key          ebiten.Key
		prompt       string
		messages     []string
		appended     string
		fullComplete bool
	}{
		{name: "Pass", key: ebiten.KeySpace, prompt: "Pass", fullComplete: true},
		{name: "Board", key: ebiten.KeyB, prompt: "Board ", messages: []string{"Not here!"}, fullComplete: true},
		{name: "D", key: ebiten.KeyD, messages: []string{"D-What?"}},
		{name: "Enter", key: ebiten.KeyE, prompt: "Enter what?", fullComplete: true},
		{name: "Fire", key: ebiten.KeyF, prompt: "Fire-", appended: "What?", fullComplete: true},
		{name: "Push", key: ebiten.KeyP, prompt: "Push", messages: []string{"Not here!"}},
		{name: "Talk", key: ebiten.KeyT, prompt: "Talk-", appended: "Funny, no response!", fullComplete: true},
		{name: "X-it", key: ebiten.KeyX, prompt: "X-it ", appended: "what?", fullComplete: true},
		{name: "W", key: ebiten.KeyW, messages: []string{"W-What?"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.keyboard.SimulateKeyPress(test.key)
			require.NoError(t, scene.Update(nil))

			assert.Equal(t, byte(1), state.DateTime.Minute)
			if test.prompt == "" {
				assert.Empty(t, callbacks.CommandPrompts)
			} else {
				assert.Equal(t, []string{test.prompt}, callbacks.CommandPrompts)
			}
			if len(test.messages) == 0 {
				assert.Empty(t, callbacks.Messages)
			} else {
				assert.Equal(t, test.messages, callbacks.Messages)
			}
			assert.Equal(t, test.appended, callbacks.CurrentRowContent)
			if test.fullComplete {
				assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			} else {
				assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
			}
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestDungeonQuitRetainsCommandUntilYesOrNo(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyQ)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, DungeonQuitInput, scene.secondaryKeyState)
	assert.NotZero(t, scene.dungeonQuitCommandToken)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, byte(1), state.DateTime.Minute)

	for _, ignored := range []ebiten.Key{ebiten.KeyA, ebiten.KeyEnter} {
		scene.keyboard.SimulateKeyPress(ignored)
		require.NoError(t, scene.Update(nil))
		assert.Equal(t, DungeonQuitInput, scene.secondaryKeyState)
		assert.NotZero(t, scene.dungeonQuitCommandToken)
		assert.False(t, state.IsAtDungeonCommandBoundary())
		assert.Equal(t, byte(1), state.DateTime.Minute)
	}

	scene.keyboard.SimulateKeyPress(ebiten.KeyN)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dungeonQuitCommandToken)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
	assert.Empty(t, callbacks.CommandPrompts)
	assert.Equal(t, []string{"Save game? No"}, callbacks.Messages)
	assert.Nil(t, scene.saveQuestionDialog)
}

func TestDungeonQuitSavesPrechargedBoundaryBeforeReturning(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	repository := &playerSaveTestRepository{records: map[player_save.SlotID]player_save.Record{
		player_save.Slot02: playerSaveTestRecord(player_save.Slot02, "loaded", validCommandTestSnapshot()),
	}}
	game := &Game{
		currentScene:          scene,
		playerSaveRepository:  repository,
		playerSaveNow:         time.Now,
		playerSaveGameVersion: "test",
	}
	scene.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	scene.setPlayerSaveProvenance(player_save.Slot02, "loaded")
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyQ)
	require.NoError(t, scene.Update(game))
	scene.keyboard.SimulateKeyPress(ebiten.KeyY)
	require.NoError(t, scene.Update(game))

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
	require.Len(t, repository.saves, 1)
	assert.Equal(t, testPlaythroughID, repository.saves[0].envelope.Metadata.PlaythroughID)
	saved := repository.saves[0].envelope.Snapshot
	assert.Equal(t, 1, saved.World.Calendar.Minute)
	assert.Equal(t, 5, saved.Effects.DurationTurns)
	assert.Equal(t, []string{"Save game? Yes", "Saving...", "Game saved to Slot 2."}, callbacks.Messages)
	overlay := requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene)
	assert.Equal(t, "Game saved to Slot 2.", overlay.GetMessage())
}

func TestDungeonQuitSaveFailureRestoresCommandBoundary(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	game := &Game{currentScene: scene}
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyQ)
	require.NoError(t, scene.Update(game))
	scene.keyboard.SimulateKeyPress(ebiten.KeyY)
	require.NoError(t, scene.Update(game))

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Contains(t, callbacks.Messages, "Save failed: Save or load a game first.")
	overlay := requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene)
	assert.Equal(t, "Save failed: Save or load a game first.", overlay.GetMessage())
}

func TestDungeonQuitPersistenceFailureRestoresCommandBoundary(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	repository := &playerSaveTestRepository{
		records: map[player_save.SlotID]player_save.Record{
			player_save.Slot02: playerSaveTestRecord(player_save.Slot02, "loaded", validCommandTestSnapshot()),
		},
		saveErr: errors.New("disk full"),
	}
	game := &Game{
		currentScene:          scene,
		playerSaveRepository:  repository,
		playerSaveNow:         time.Now,
		playerSaveGameVersion: "test",
	}
	scene.setPlaythroughMetadata(playthroughMetadata{id: testPlaythroughID})
	scene.setPlayerSaveProvenance(player_save.Slot02, "loaded")
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyQ)
	require.NoError(t, scene.Update(game))
	scene.keyboard.SimulateKeyPress(ebiten.KeyY)
	require.NoError(t, scene.Update(game))

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	require.Len(t, repository.saves, 1)
	require.NotEmpty(t, callbacks.Messages)
	assert.Equal(t, "Save failed: Please try again. If the problem continues, check the game log for details.",
		callbacks.Messages[len(callbacks.Messages)-1])
	overlay := requireEscapeMenuDialog[*widgets.OverlayDialog](t, scene)
	assert.Equal(t, callbacks.Messages[len(callbacks.Messages)-1], overlay.GetMessage())
	assert.NotContains(t, overlay.GetMessage(), "Game saved")
}

func TestDungeonYellRetainsCommandThroughEditableInput(t *testing.T) {
	tests := []struct {
		name     string
		keys     []ebiten.Key
		finish   ebiten.Key
		wantText string
		message  string
	}{
		{
			name:     "nonempty edited text",
			keys:     []ebiten.Key{ebiten.KeyH, ebiten.KeyX, ebiten.KeyLeft, ebiten.KeyI, ebiten.KeySpace, ebiten.KeyMinus},
			finish:   ebiten.KeyEnter,
			wantText: "HI -",
			message:  "No effect!",
		},
		{name: "empty", finish: ebiten.KeyEnter, message: "Nothing"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.keyboard.SimulateKeyPress(ebiten.KeyY)
			require.NoError(t, scene.Update(nil))
			assert.False(t, state.IsAtDungeonCommandBoundary())
			assert.True(t, state.EventQueueManager.HasWorkflow("dungeon_yell"))
			waitForDungeonInputDialog(t, scene, state)

			for _, key := range test.keys {
				scene.keyboard.SimulateKeyPress(key)
				require.NoError(t, scene.Update(nil))
			}
			dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InputBox)
			require.True(t, ok)
			assert.Equal(t, dungeonYellPrompt, dialog.Question)
			assert.Equal(t, test.wantText, dialog.GetText())
			scene.keyboard.SimulateKeyPress(test.finish)
			require.NoError(t, scene.Update(nil))
			drainDungeonInputWorkflowThroughUpdate(t, scene, state, "dungeon_yell")

			assert.Equal(t, []string{"Yell "}, callbacks.CommandPrompts)
			assert.Contains(t, callbacks.Messages, test.message)
			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			assert.Zero(t, scene.dialogStack.Count())
			assert.False(t, callbacks.InputBlocked)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestDungeonYellEscapeClearsTextAndRetainsCommand(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyY)
	require.NoError(t, scene.Update(nil))
	waitForDungeonInputDialog(t, scene, state)
	scene.keyboard.SimulateKeyPress(ebiten.KeyH)
	require.NoError(t, scene.Update(nil))

	scene.keyboard.SimulateKeyPress(ebiten.KeyTab)
	require.NoError(t, scene.Update(nil))
	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InputBox)
	require.True(t, ok)
	assert.Equal(t, "H", dialog.GetText(), "Tab must be ignored by source-style text input")

	scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.NoError(t, scene.Update(nil))
	dialog, ok = scene.dialogStack.PeekTopModalDialog().(*widgets.InputBox)
	require.True(t, ok)
	assert.Empty(t, dialog.GetText())
	assert.True(t, state.EventQueueManager.HasWorkflow("dungeon_yell"))
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Empty(t, callbacks.Messages)

	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(nil))
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "dungeon_yell")
	assert.Contains(t, callbacks.Messages, "Nothing")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonUnsupportedBoundCommandsDoNotStartCycle(t *testing.T) {
	supported := map[ebiten.Key]bool{
		ebiten.KeyUp: true, ebiten.KeyDown: true, ebiten.KeyLeft: true, ebiten.KeyRight: true,
		ebiten.KeySpace: true, ebiten.KeyEnter: true, ebiten.KeyPeriod: true,
		ebiten.KeyA: true, ebiten.KeyB: true, ebiten.KeyC: true, ebiten.KeyD: true, ebiten.KeyE: true, ebiten.KeyF: true,
		ebiten.KeyG: true, ebiten.KeyI: true, ebiten.KeyJ: true, ebiten.KeyK: true, ebiten.KeyL: true,
		ebiten.KeyH: true,
		ebiten.KeyM: true, ebiten.KeyN: true, ebiten.KeyO: true, ebiten.KeyP: true, ebiten.KeyQ: true,
		ebiten.KeyR: true, ebiten.KeyS: true, ebiten.KeyT: true,
		ebiten.KeyU: true, ebiten.KeyV: true, ebiten.KeyW: true, ebiten.KeyX: true, ebiten.KeyY: true, ebiten.KeyZ: true,
	}
	global := map[ebiten.Key]bool{
		ebiten.Key0: true, ebiten.Key1: true, ebiten.Key2: true, ebiten.Key3: true,
		ebiten.Key4: true, ebiten.Key5: true, ebiten.Key6: true, ebiten.KeyEscape: true,
	}

	for _, key := range boundKeysGame {
		if supported[key] || global[key] {
			continue
		}
		t.Run(key.String(), func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}

			scene.dungeonMapInputHandler(key)

			assert.Zero(t, state.DateTime.Minute)
			assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, []string{dungeonUnboundKeyFeedback}, callbacks.Messages)
			assert.Empty(t, callbacks.CommandPrompts)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestDungeonHoleUpZeroAndSpaceCancelRetainedCommand(t *testing.T) {
	for _, testCase := range []struct {
		name string
		keys []ebiten.Key
	}{
		// Zero cancels at once, as Space does: the hours are stepped, not typed and entered.
		{name: "zero", keys: []ebiten.Key{ebiten.Key0}},
		{name: "space", keys: []ebiten.Key{ebiten.KeySpace}},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.keyboard.SimulateKeyPress(ebiten.KeyH)
			require.NoError(t, scene.Update(nil))
			assert.False(t, state.IsAtDungeonCommandBoundary())
			assert.True(t, state.EventQueueManager.HasWorkflow("dungeon_hole_up"))
			waitForDungeonInputDialog(t, scene, state)
			for _, key := range testCase.keys {
				scene.keyboard.SimulateKeyPress(key)
				require.NoError(t, scene.Update(nil))
			}
			drainDungeonInputWorkflowThroughUpdate(t, scene, state, "dungeon_hole_up")

			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			assert.Nil(t, state.CampingScene)
			assert.Nil(t, state.CombatScene)
			assert.True(t, state.IsAtDungeonCommandBoundary())
			assert.False(t, callbacks.InputBlocked)
		})
	}
}

func TestDungeonHoleUpOneHourCompletesProductionWorkflow(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	for index := 1; index < party_state.MaxActiveParty; index++ {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	anyKeyChecks := 0
	state.SystemCallbacks.Visual.IsAnyKeyPressed = func() bool {
		anyKeyChecks++

		return anyKeyChecks%2 == 0
	}
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyH)
	require.NoError(t, scene.Update(nil))
	waitForDungeonInputDialog(t, scene, state)
	scene.keyboard.SimulateKeyPress(ebiten.Key1)
	require.NoError(t, scene.Update(nil))
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	require.NoError(t, scene.Update(nil))
	for range 2000 {
		state.ElapsedMs += 500
		require.NoError(t, scene.Update(nil))
		if !state.EventQueueManager.HasWorkflow("dungeon_hole_up") {
			break
		}
	}

	assert.False(t, state.EventQueueManager.HasWorkflow("dungeon_hole_up"))
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Nil(t, state.CampingScene)
	assert.Nil(t, state.CombatScene)
	assert.Equal(t, game_state.DurationEffectNone, state.ActiveDurationEffect.EffectType)
	assert.Zero(t, scene.dialogStack.Count())
	assert.False(t, callbacks.InputBlocked)
}

func TestDungeonModalCommandCancellationCompletesFullCycle(t *testing.T) {
	tests := []struct {
		name  string
		key   ebiten.Key
		setup func(*game_state.GameState)
	}{
		{
			name: "mix",
			key:  ebiten.KeyM,
			setup: func(state *game_state.GameState) {
				state.PartyState.Inventory.Reagent.Set(references.SulfurAsh, 1)
			},
		},
		{name: "new order", key: ebiten.KeyN, setup: func(*game_state.GameState) {}},
		{name: "ready", key: ebiten.KeyR, setup: func(*game_state.GameState) {}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			test.setup(state)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}

			scene.keyboard.SimulateKeyPress(test.key)
			require.NoError(t, scene.Update(nil))
			waitForDungeonInputDialog(t, scene, state)
			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.False(t, state.IsAtDungeonCommandBoundary())

			scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
			require.NoError(t, scene.Update(nil))
			drainDungeonInputWorkflowThroughUpdate(t, scene, state, dungeonModalWorkflowName(test.key))

			assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			assert.True(t, state.IsAtDungeonCommandBoundary())
			assert.False(t, callbacks.InputBlocked)
		})
	}
}

func TestDungeonModalCommandRejectionCompletesFullCycle(t *testing.T) {
	tests := []struct {
		name  string
		key   ebiten.Key
		setup func(*game_state.GameState)
	}{
		{
			name: "mix without reagents",
			key:  ebiten.KeyM,
			setup: func(state *game_state.GameState) {
				for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
					state.PartyState.Inventory.Reagent.Set(reagent, 0)
				}
			},
		},
		{
			name: "new order with one member",
			key:  ebiten.KeyN,
			setup: func(state *game_state.GameState) {
				for index := 1; index < party_state.MaxActiveParty; index++ {
					state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
				}
			},
		},
		{
			name: "new order with only one companion",
			key:  ebiten.KeyN,
			setup: func(state *game_state.GameState) {
				for index := 2; index < party_state.MaxActiveParty; index++ {
					state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
				}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			test.setup(state)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
			}

			pressDungeonInputKey(t, scene, test.key)

			assert.False(t, state.EventQueueManager.HasWorkflow(dungeonModalWorkflowName(test.key)))
			assertDungeonModalFullCompletion(t, state, callbacks)
		})
	}
}

func TestNewOrderWorkflowShutdownRemovesOpenDialog(t *testing.T) {
	scene, _ := newSurfaceUseOwnershipScene(t)
	state := scene.gameState
	for index := range 3 {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapActive
	}

	scene.startNewOrderWorkflowWithCompletion(nil)
	state.EventQueueManager.Update()
	require.IsType(t, &widgets.NewOrderDialog{}, scene.dialogStack.PeekTopModalDialog())

	require.NotPanics(t, state.EventQueueManager.Shutdown)
	assert.Zero(t, scene.dialogStack.Count())
}

func TestDungeonMixSuccessRetainsCycleUntilMixerCloses(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
		state.PartyState.Inventory.Reagent.Set(reagent, 10)
	}
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	spellsBefore := state.PartyState.Inventory.Spells.Get(references.InLor)

	pressDungeonInputKey(t, scene, ebiten.KeyM)
	waitForDungeonInputDialog(t, scene, state)
	mixingDialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ReagentMixingDialog)
	require.True(t, ok)
	pressDungeonInputKey(t, scene, ebiten.KeyI)
	pressDungeonInputKey(t, scene, ebiten.KeyL)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	pressDungeonInputKey(t, scene, ebiten.Key1)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	// A plain success opens no result box: the mixer returns to its list and stays open.
	for range 10 {
		state.ElapsedMs += 20
		require.NoError(t, scene.Update(nil))
		_, box := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
		require.False(t, box, "a plain success opens no result box")
	}
	require.False(t, mixingDialog.IsAwaitingConfirmation())
	require.Same(t, mixingDialog, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, spellsBefore+1, state.PartyState.Inventory.Spells.Get(references.InLor))
	assert.False(t, state.IsAtDungeonCommandBoundary(), "the command cycle is held while the mixer is open")
	scene.keyboard.SetAllowKeyPressImmediately()
	pressDungeonInputKey(t, scene, ebiten.KeyEscape)
	assert.True(t, mixingDialog.IsDismissed())
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "reagent_mixing_workflow")

	assert.Equal(t, spellsBefore+1, state.PartyState.Inventory.Spells.Get(references.InLor))
	assertDungeonModalFullCompletion(t, state, callbacks)
}

func TestDungeonFailedMixCompletesAfterTrapEffects(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	totalBefore := uint16(0)
	for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
		state.PartyState.Inventory.Reagent.Set(reagent, 10)
		totalBefore += 10
	}
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	spellsBefore := state.PartyState.Inventory.Spells.Get(references.AnNox)
	hpBefore := 0
	for index := range state.PartyState.GetActivePartyCount() {
		hpBefore += int(state.PartyState.GetCharacter(index).GetCurrentHp())
	}

	pressDungeonInputKey(t, scene, ebiten.KeyM)
	waitForDungeonInputDialog(t, scene, state)
	pressDungeonInputKey(t, scene, ebiten.KeyA)
	pressDungeonInputKey(t, scene, ebiten.KeyN)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	pressDungeonInputKey(t, scene, ebiten.KeyTab) // The Experiment tab, where the recipe can be spoiled.
	pressDungeonInputKey(t, scene, ebiten.KeySpace)
	pressDungeonInputKey(t, scene, ebiten.Key1)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "reagent_mixing_workflow")

	totalAfter := uint16(0)
	for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
		totalAfter += state.PartyState.Inventory.Reagent.Get(reagent)
	}
	assert.Less(t, totalAfter, totalBefore, "committed failed mix consumes selected reagents")
	assert.Equal(t, spellsBefore, state.PartyState.Inventory.Spells.Get(references.AnNox))
	hpAfter := 0
	for index := range state.PartyState.GetActivePartyCount() {
		hpAfter += int(state.PartyState.GetCharacter(index).GetCurrentHp())
	}
	assert.Less(t, hpAfter, hpBefore, "failed mix trap resolves before command completion")
	assertDungeonModalFullCompletion(t, state, callbacks)
}

func TestDungeonNewOrderSuccessRetainsCycleThroughSwap(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	firstBefore := state.PartyState.GetCharacter(1).GetNameAsString()
	secondBefore := state.PartyState.GetCharacter(2).GetNameAsString()

	pressDungeonInputKey(t, scene, ebiten.KeyN)
	waitForDungeonInputDialog(t, scene, state)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "new_order_workflow")

	assert.Equal(t, secondBefore, state.PartyState.GetCharacter(1).GetNameAsString())
	assert.Equal(t, firstBefore, state.PartyState.GetCharacter(2).GetNameAsString())
	assertDungeonModalFullCompletion(t, state, callbacks)
}

func TestDungeonReadySuccessAllowsBodyArmourAndCompletesOnce(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}
	for equipment := references.LeatherHelm; equipment <= references.Ankh; equipment++ {
		state.PartyState.Inventory.Equipment.Set(equipment, 0)
	}
	character := state.PartyState.GetCharacter(0)
	character.Strength = 30
	for slot := references.SlotHelm; slot <= references.SlotAmulet; slot++ {
		character.SetEquippedItem(slot, references.NoEquipment)
	}
	state.PartyState.Inventory.Equipment.Set(references.LeatherArmour, 1)

	pressDungeonInputKey(t, scene, ebiten.KeyR)
	waitForDungeonInputDialog(t, scene, state)
	pressDungeonInputKey(t, scene, ebiten.KeyEnter)
	pressDungeonInputKey(t, scene, ebiten.KeyEscape)
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "ready_equipment_workflow")

	assert.Equal(t, references.LeatherArmour, character.GetEquippedItem(references.SlotArmour))
	assertDungeonModalFullCompletion(t, state, callbacks)
}

func TestDungeonModalCompletionCanHandOffToFixedRoom(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)
	setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, 0xf1)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 5,
	}

	pressDungeonInputKey(t, scene, ebiten.KeyN)
	waitForDungeonInputDialog(t, scene, state)
	pressDungeonInputKey(t, scene, ebiten.KeyEscape)
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "new_order_workflow")

	require.NotNil(t, state.CombatScene)
	require.NotNil(t, state.CombatScene.DungeonRoomOrigin)
	assert.Equal(t, 1, state.CombatScene.DungeonRoomOrigin.Room)
	assert.False(t, state.IsAtDungeonCommandBoundary(), "room handoff owns the suspended continuation")
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining,
		"room handoff defers final duration processing until combat returns")
	assert.False(t, callbacks.InputBlocked)
}

func pressDungeonInputKey(t *testing.T, scene *GameScene, key ebiten.Key) {
	t.Helper()
	scene.keyboard.SimulateKeyPress(key)
	require.NoError(t, scene.Update(nil))
}

func assertDungeonModalFullCompletion(
	t *testing.T,
	state *game_state.GameState,
	callbacks *game_state.MockSystemCallbacks,
) {
	t.Helper()
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.False(t, callbacks.InputBlocked)
}

func dungeonModalWorkflowName(key ebiten.Key) string {
	switch key {
	case ebiten.KeyM:
		return "reagent_mixing_workflow"
	case ebiten.KeyN:
		return "new_order_workflow"
	case ebiten.KeyR:
		return "ready_equipment_workflow"
	default:
		panic("not a dungeon modal command")
	}
}

func TestDungeonUpdateRoutesPrimaryInput(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)
	initializeDungeonUpdateTestScene(scene, state)

	scene.keyboard.SimulateKeyPress(ebiten.KeyPeriod)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, references.DungeonFacingWest, dungeonInputFacing(t, state))
	assert.Equal(t, byte(1), state.DateTime.Minute)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonUpdateClearsStaleSecondaryInputAtBoundary(t *testing.T) {
	tests := []struct {
		name  string
		state InputState
		set   func(*GameScene, game_state.DungeonCommandCycle)
		get   func(*GameScene) game_state.DungeonCommandCycle
	}{
		{
			name: "Look", state: LookDirectionInput,
			set: func(scene *GameScene, token game_state.DungeonCommandCycle) { scene.dungeonLookCommandToken = token },
			get: func(scene *GameScene) game_state.DungeonCommandCycle { return scene.dungeonLookCommandToken },
		},
		{
			name: "Search", state: SearchDirectionInput,
			set: func(scene *GameScene, token game_state.DungeonCommandCycle) { scene.dungeonSearchCommandToken = token },
			get: func(scene *GameScene) game_state.DungeonCommandCycle { return scene.dungeonSearchCommandToken },
		},
		{
			name: "Klimb", state: KlimbDirectionInput,
			set: func(scene *GameScene, token game_state.DungeonCommandCycle) { scene.dungeonKlimbCommandToken = token },
			get: func(scene *GameScene) game_state.DungeonCommandCycle { return scene.dungeonKlimbCommandToken },
		},
		{
			name: "Quit", state: DungeonQuitInput,
			set: func(scene *GameScene, token game_state.DungeonCommandCycle) { scene.dungeonQuitCommandToken = token },
			get: func(scene *GameScene) game_state.DungeonCommandCycle { return scene.dungeonQuitCommandToken },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			initializeDungeonUpdateTestScene(scene, state)
			token, _ := state.BeginDungeonCommandCycle()
			test.set(scene, token)
			state.CompleteDungeonCommandCycle(token, game_state.DungeonCommandOnly)
			callbacks.Reset()
			minuteBefore := state.DateTime.Minute
			scene.secondaryKeyState = test.state

			require.NoError(t, scene.Update(nil))

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Zero(t, test.get(scene))
			assert.Equal(t, minuteBefore, state.DateTime.Minute)
			assert.Empty(t, callbacks.Messages)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func initializeDungeonUpdateTestScene(scene *GameScene, state *game_state.GameState) {
	_, scene.output = newMainOutput(1280)
	scene.autoOpenDoorState = &AutoOpenDoorState{}
	scene.windDisplay = &WindDisplay{}
	scene.moongateVisualSystem = NewMoongateVisualSystem(state)
	scene.activeEffectsOverlay = NewActiveEffectsOverlay(nil)
	scene.activeEffectsPanel = NewActiveEffectsPanel(state, nil, image.Rectangle{})
	scene.lastEffectsUpdate = time.Now()
}

func TestDungeonPeriodIsBound(t *testing.T) {
	assert.Contains(t, boundKeysGame, ebiten.KeyPeriod)
}

func TestDungeonMovementHiddenPitRetainsCommandUntilWorkflowCleanup(t *testing.T) {
	tests := []struct {
		name    string
		key     ebiten.Key
		target  references.Position
		prompt  string
		pitCell references.DungeonCell
	}{
		{name: "advance", key: ebiten.KeyUp, target: references.Position{X: 2, Y: 1}, prompt: "Advance", pitCell: 0x61},
		{name: "back up", key: ebiten.KeyDown, target: references.Position{X: 0, Y: 1}, prompt: "Back up", pitCell: 0x69},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			setDungeonInputTestCellAtFloor(t, state, 0, test.target, test.pitCell)
			setDungeonInputTestCellAtFloor(t, state, 1, test.target, 0xfa)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.dungeonMapInputHandler(test.key)

			assert.Equal(t, test.target, state.MapState.PlayerLocation.Position)
			assert.Equal(t, references.FloorNumber(0), state.MapState.PlayerLocation.Floor)
			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, []string{test.prompt}, callbacks.CommandPrompts)
			assert.True(t, callbacks.InputBlocked)
			assert.False(t, state.IsAtDungeonCommandBoundary())

			drainDungeonInputWorkflow(t, state, "dungeon_hidden_pit")

			assert.Equal(t, references.FloorNumber(1), state.MapState.PlayerLocation.Floor)
			assert.Equal(t, test.target, state.MapState.PlayerLocation.Position)
			assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining,
				"end-player effects wait until the pit-landed room returns")
			assert.False(t, callbacks.InputBlocked)
			assert.Equal(t, 1, callbacks.BlockInputCalls)
			assert.Equal(t, 1, callbacks.UnblockInputCalls)
			assert.False(t, state.IsAtDungeonCommandBoundary())
			assert.True(t, state.HasPendingDungeonRoomEncounter(), "command token must be released for room handoff")
		})
	}
}

func TestDungeonKlimbNoRouteAndMissingGrappleUseCommandOnlyCompletion(t *testing.T) {
	tests := []struct {
		name        string
		cell        references.DungeonCell
		wantPrompt  string
		wantMessage string
	}{
		{name: "no route", cell: 0x00, wantPrompt: "Klimb-what?"},
		{name: "opening without grapple", cell: 0x08, wantPrompt: "Klimb-", wantMessage: "With What?"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			setDungeonInputTestCell(t, state, state.MapState.PlayerLocation.Position, test.cell)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.dungeonMapInputHandler(ebiten.KeyK)

			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, []string{test.wantPrompt}, callbacks.CommandPrompts)
			if test.wantMessage != "" {
				assert.Contains(t, callbacks.Messages, test.wantMessage)
			}
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestDungeonKlimbSingleRouteUsesWorkflowAndFullCompletion(t *testing.T) {
	tests := []struct {
		name    string
		cell    references.DungeonCell
		grapple bool
	}{
		{name: "ladder", cell: 0x10},
		{name: "grapple opening", cell: 0x08, grapple: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks := newDungeonInputTestScene(t)
			state.MapState.PlayerLocation.Floor = 3
			position := state.MapState.PlayerLocation.Position
			setDungeonInputTestCellAtFloor(t, state, 3, position, test.cell)
			setDungeonInputTestCellAtFloor(t, state, 2, position, 0x00)
			if test.grapple {
				state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
			}
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 5,
			}
			callbacks.Reset()

			scene.dungeonMapInputHandler(ebiten.KeyK)
			assert.True(t, callbacks.InputBlocked)
			assert.False(t, state.IsAtDungeonCommandBoundary())
			drainDungeonInputWorkflow(t, state, "dungeon_klimb")

			assert.Equal(t, references.FloorNumber(2), state.MapState.PlayerLocation.Floor)
			assert.Equal(t, references.DungeonArrivedFromBelow, dungeonInputLastDirection(t, state))
			assert.Equal(t, byte(1), state.DateTime.Minute)
			assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
			assert.Equal(t, []string{"Klimb-"}, callbacks.CommandPrompts)
			assert.Contains(t, callbacks.Messages, "Up!")
			assert.False(t, callbacks.InputBlocked)
			assert.True(t, state.IsAtDungeonCommandBoundary())
		})
	}
}

func TestDungeonKlimbDualRouteRetainsTokenUntilValidChoice(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	state.MapState.PlayerLocation.Floor = 3
	position := state.MapState.PlayerLocation.Position
	setDungeonInputTestCellAtFloor(t, state, 3, position, 0x30)
	setDungeonInputTestCellAtFloor(t, state, 2, position, 0x00)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	initializeDungeonUpdateTestScene(scene, state)
	callbacks.Reset()

	scene.keyboard.SimulateKeyPress(ebiten.KeyK)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, KlimbDirectionInput, scene.secondaryKeyState)
	assert.NotZero(t, scene.dungeonKlimbCommandToken)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, byte(1), state.DateTime.Minute)

	scene.keyboard.SimulateKeyPress(ebiten.KeyA)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, KlimbDirectionInput, scene.secondaryKeyState)
	assert.NotZero(t, scene.dungeonKlimbCommandToken)
	assert.False(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, byte(1), state.DateTime.Minute, "unrelated input must not precharge again")
	assert.Equal(t, 5, state.ActiveDurationEffect.TurnsRemaining)

	scene.keyboard.SimulateKeyPress(ebiten.KeyNumpad8)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Zero(t, scene.dungeonKlimbCommandToken)
	drainDungeonInputWorkflowThroughUpdate(t, scene, state, "dungeon_klimb")

	assert.Equal(t, references.FloorNumber(2), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, []string{"Klimb-U/D-"}, callbacks.CommandPrompts)
	assert.False(t, callbacks.InputBlocked)
	assert.Equal(t, 1, callbacks.BlockInputCalls)
	assert.Equal(t, 1, callbacks.UnblockInputCalls)
	require.NotEmpty(t, callbacks.MapFadeLevels)
	assert.Zero(t, callbacks.MapFadeLevels[len(callbacks.MapFadeLevels)-1])
}

func TestDungeonKlimbDualRoutePassUsesFullCompletion(t *testing.T) {
	// Esc passes like Space, as it does at the Look and Search prompts.
	for _, passKey := range []ebiten.Key{ebiten.KeySpace, ebiten.KeyEscape} {
		scene, state, callbacks := newDungeonInputTestScene(t)
		state.MapState.PlayerLocation.Floor = 3
		position := state.MapState.PlayerLocation.Position
		setDungeonInputTestCellAtFloor(t, state, 3, position, 0x30)
		state.ActiveDurationEffect = game_state.DurationEffect{
			EffectType:     game_state.DurationEffectProtection,
			TurnsRemaining: 5,
		}
		callbacks.Reset()

		scene.dungeonMapInputHandler(ebiten.KeyK)
		scene.keyboard.SimulateKeyPress(passKey)
		scene.dungeonMapHandleSecondaryInput(nil)

		assert.Equal(t, references.FloorNumber(3), state.MapState.PlayerLocation.Floor)
		assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
		assert.Contains(t, callbacks.Messages, "Pass")
		assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
		assert.True(t, state.IsAtDungeonCommandBoundary())
	}
}

func TestDungeonKlimbDualRouteAcceptsLetterDForDown(t *testing.T) {
	scene, state, _ := newDungeonInputTestScene(t)
	state.MapState.PlayerLocation.Floor = 3
	position := state.MapState.PlayerLocation.Position
	setDungeonInputTestCellAtFloor(t, state, 3, position, 0x30)
	setDungeonInputTestCellAtFloor(t, state, 4, position, 0x00)

	scene.dungeonMapInputHandler(ebiten.KeyK)
	scene.keyboard.SimulateKeyPress(ebiten.KeyD)
	scene.dungeonMapHandleSecondaryInput(nil)
	drainDungeonInputWorkflow(t, state, "dungeon_klimb")

	assert.Equal(t, references.FloorNumber(4), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, references.DungeonArrivedFromAbove, dungeonInputLastDirection(t, state))
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func TestDungeonKlimbBlockedSelectedRouteUsesFullCompletion(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	state.MapState.PlayerLocation.Floor = 3
	position := state.MapState.PlayerLocation.Position
	setDungeonInputTestCellAtFloor(t, state, 3, position, 0x30)
	setDungeonInputTestCellAtFloor(t, state, 2, position, 0xb0)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType:     game_state.DurationEffectProtection,
		TurnsRemaining: 5,
	}
	callbacks.Reset()

	scene.dungeonMapInputHandler(ebiten.KeyK)
	scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	scene.dungeonMapHandleSecondaryInput(nil)
	drainDungeonInputWorkflow(t, state, "dungeon_klimb")

	assert.Equal(t, references.FloorNumber(3), state.MapState.PlayerLocation.Floor)
	assert.Equal(t, 4, state.ActiveDurationEffect.TurnsRemaining)
	assert.Contains(t, callbacks.Messages, "Failed!")
	assert.True(t, state.IsAtDungeonCommandBoundary())
}

func dungeonInputLastDirection(t *testing.T, state *game_state.GameState) references.DungeonLastDirection {
	t.Helper()
	snapshot, active := state.DungeonStateSnapshot()
	require.True(t, active)

	return snapshot.LastDirection
}

func drainDungeonInputWorkflow(t *testing.T, state *game_state.GameState, name string) {
	t.Helper()
	for range 20 {
		state.EventQueueManager.Update()
		state.ElapsedMs += 1000
		if !state.EventQueueManager.HasWorkflow(name) {
			return
		}
	}
	require.False(t, state.EventQueueManager.HasWorkflow(name))
}

func drainDungeonInputWorkflowThroughUpdate(
	t *testing.T,
	scene *GameScene,
	state *game_state.GameState,
	name string,
) {
	t.Helper()
	for range 20 {
		state.ElapsedMs += 1000
		require.NoError(t, scene.Update(nil))
		if !state.EventQueueManager.HasWorkflow(name) {
			return
		}
	}
	require.False(t, state.EventQueueManager.HasWorkflow(name))
}

func waitForDungeonInputDialog(t *testing.T, scene *GameScene, state *game_state.GameState) {
	t.Helper()
	for range 10 {
		if scene.dialogStack.PeekTopModalDialog() != nil {
			return
		}
		state.ElapsedMs++
		require.NoError(t, scene.Update(nil))
	}
	require.NotNil(t, scene.dialogStack.PeekTopModalDialog())
}
