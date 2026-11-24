package main

import (
	"image"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSmallMapMovementKeyMapsSmashedDirections(t *testing.T) {
	for direction, expected := range map[references.Direction]ebiten.Key{
		references.Up:    ebiten.KeyUp,
		references.Down:  ebiten.KeyDown,
		references.Right: ebiten.KeyRight,
		references.Left:  ebiten.KeyLeft,
	} {
		assert.Equal(t, expected, smallMapMovementKey(direction))
	}
}

func TestSmallMapAttackDirectionEscapeCancelsWithoutSmashedInterception(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 1, smashedInputHitSeed(0))
	callbacks.Reset()
	scene.secondaryKeyState = AttackDirectionInput
	beforeDate, beforeRNG := state.DateTime, state.GetRNG().Calls()

	pressSmashedInputKey(t, scene, ebiten.KeyEscape)

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Equal(t, beforeDate, state.DateTime)
	assert.Equal(t, beforeRNG, state.GetRNG().Calls())
	assert.Equal(t, 1, captureSmashed(t, state))
	assert.NotContains(t, callbacks.Messages, "Hic!")
	assert.Zero(t, scene.dialogStack.Count(), "secondary Escape must not open the menu")
}

func TestSmallMapSmashedHitSuppressesIntendedCommand(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 1, smashedInputHitSeed(0))

	pressSmashedInputKey(t, scene, ebiten.KeyL)

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.NotContains(t, callbacks.CommandPrompts, "Look-")
	assert.Contains(t, callbacks.CommandPrompts, "North")
	assert.Equal(t, []string{"Hic!"}, callbacks.Messages)
	assert.Zero(t, captureSmashed(t, state))
	require.Equal(t, 1, scene.dialogStack.Count())
	overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
	require.True(t, ok)
	require.Contains(t, overlay.GetContentText(), "The party has sobered up.")
	require.True(t, overlay.IsTimed())
	require.Equal(t, tavernStatusOverlayDurationMs, overlay.GetDurationMs())
	state.ElapsedMs = scene.timedOverlayDismissMs
	scene.updateTimedOverlay()
	require.Nil(t, scene.timedOverlayDialog)
	require.Zero(t, scene.dialogStack.Count())
}

func TestSmallMapSmashedMissRetainsIntendedCommand(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 1, smashedInputMissSeed())

	pressSmashedInputKey(t, scene, ebiten.KeyL)

	assert.Equal(t, LookDirectionInput, scene.secondaryKeyState)
	assert.Contains(t, callbacks.CommandPrompts, "Look-")
	assert.NotContains(t, callbacks.Messages, "Hic!")
	assert.Equal(t, 1, captureSmashed(t, state))
	assert.Equal(t, uint64(1), state.GetRNG().Calls())
}

func TestSmallMapSmashedDoesNotInterceptSecondaryInputState(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 1, smashedInputHitSeed(2))
	scene.secondaryKeyState = LookDirectionInput
	rngCallsBefore := state.GetRNG().Calls()

	require.NoError(t, scene.Update(nil))

	assert.Equal(t, LookDirectionInput, scene.secondaryKeyState)
	assert.Equal(t, rngCallsBefore, state.GetRNG().Calls())
	assert.Equal(t, 1, captureSmashed(t, state))
}

func TestSmallMapHeldNonArrowCanInterceptOnlyOncePerAcceptedPress(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 2, smashedInputHitSeed(3))
	scene.keyboard.SimulateKeyPress(ebiten.KeyL)
	require.NoError(t, scene.Update(nil))
	assert.Equal(t, 1, captureSmashed(t, state))

	require.NoError(t, scene.Update(nil))

	assert.Equal(t, 1, captureSmashed(t, state))
}

func TestSmallMapSmashedBlockedMovementUsesNormalTurnPath(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 1, smashedInputHitSeed(0))
	origin, direction := findBlockedSmallMapStep(t, state)
	state.MapState.PlayerLocation.Position = origin
	state.SetRandomSeed(smashedInputHitSeed(smashedInputSourceRoll(direction)))
	before := state.DateTime
	expected := before
	expected.Advance(game_state.DefaultSmallMapMinutesPerTurn)

	pressSmashedInputKey(t, scene, ebiten.KeyL)

	assert.Equal(t, origin, state.MapState.PlayerLocation.Position)
	assert.Equal(t, expected, state.DateTime)
	assert.Contains(t, scene.output.GetOutputStr(true), "Blocked!")
	assert.Zero(t, captureSmashed(t, state))
}

func TestSmallMapSmashedGuardFailureIsReturnedByUpdate(t *testing.T) {
	scene, state, callbacks := newSmashedInputTestScene(t, 1, smashedInputHitSeed(0))
	controller := state.CurrentNPCAIController
	state.CurrentNPCAIController = nil
	rngCallsBefore := state.GetRNG().Calls()
	scene.keyboard.SimulateKeyPress(ebiten.KeyL)

	err := scene.Update(nil)

	require.Error(t, err)
	assert.ErrorContains(t, err, "intercepting intoxicated small-map command")
	assert.Equal(t, rngCallsBefore+1, state.GetRNG().Calls())
	assert.NotContains(t, callbacks.Messages, "Hic!")
	assert.Empty(t, callbacks.CommandPrompts)
	state.CurrentNPCAIController = controller
	assert.Equal(t, 1, captureSmashed(t, state))
}

func newSmashedInputTestScene(
	t *testing.T,
	smashed int,
	seed uint64,
) (*GameScene, *game_state.GameState, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)
	snapshot.Effects.Smashed = smashed
	snapshot.Random.Seed = seed
	snapshot.Random.SourceCalls = 0

	state, err = game_state.NewGameStateFromSaveSnapshot(
		snapshot,
		state.GameConfig,
		state.GameReferences,
		state.MapState.XTilesVisibleOnGameScreen,
		state.MapState.YTilesVisibleOnGameScreen,
	)
	require.NoError(t, err)
	callbacks := game_state.NewMockSystemCallbacks(t)
	state.SystemCallbacks = callbacks.ToSystemCallbacks()
	state.SetupEnvironmentalSystemCallbacks()
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	state.DateTime.Hour = 12

	keyboard := input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)
	scene := &GameScene{
		gameState:            state,
		keyboard:             keyboard,
		output:               text.NewOutput(nil, 1, 64, 128),
		autoOpenDoorState:    &AutoOpenDoorState{},
		windDisplay:          &WindDisplay{},
		moongateVisualSystem: NewMoongateVisualSystem(state),
		activeEffectsOverlay: NewActiveEffectsOverlay(nil),
		lastEffectsUpdate:    time.Now(),
		secondaryKeyState:    PrimaryInput,
	}
	scene.activeEffectsPanel = NewActiveEffectsPanel(
		state,
		nil,
		image.Rectangle{},
	)

	return scene, state, callbacks
}

func pressSmashedInputKey(t *testing.T, scene *GameScene, key ebiten.Key) {
	t.Helper()
	scene.keyboard.SimulateKeyPress(key)
	require.NoError(t, scene.Update(nil))
}

func captureSmashed(t *testing.T, state *game_state.GameState) int {
	t.Helper()
	snapshot, err := state.CaptureSaveSnapshot()
	require.NoError(t, err)

	return snapshot.Effects.Smashed
}

func smashedInputMissSeed() uint64 {
	for seed := uint64(0); ; seed++ {
		if beradrng.New(seed).Intn(2) == 0 {
			return seed
		}
	}
}

func smashedInputHitSeed(sourceRoll int) uint64 {
	for seed := uint64(0); ; seed++ {
		rng := beradrng.New(seed)
		if rng.Intn(2) != 0 && rng.Intn(4) == sourceRoll {
			return seed
		}
	}
}

func smashedInputSourceRoll(direction references.Direction) int {
	switch direction {
	case references.Up:
		return 0
	case references.Down:
		return 1
	case references.Right:
		return 2
	case references.Left:
		return 3
	case references.NoneDirection:
		panic("no smashed movement direction")
	default:
		panic("invalid smashed movement direction")
	}
}

func findBlockedSmallMapStep(
	t *testing.T,
	state *game_state.GameState,
) (references.Position, references.Direction) {
	t.Helper()
	for y := range int(references.YSmallMapTiles) {
		for x := range int(references.XSmallMapTiles) {
			origin := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			if !state.IsPassable(&origin) {
				continue
			}
			for _, direction := range []references.Direction{
				references.Up,
				references.Down,
				references.Right,
				references.Left,
			} {
				target := direction.GetNewPositionInDirection(&origin)
				if target == nil || state.IsOutOfBounds(*target) || state.IsPassable(target) ||
					state.CanAutoOpenDoor(target) ||
					state.MapState.LayeredMaps.GetSmallMapFloorKlimbOffset(
						*target,
						state.MapState.PlayerLocation.Floor,
					) != 0 {
					continue
				}

				return origin, direction
			}
		}
	}
	t.Fatal("Britain fixture has no ordinary blocked adjacent movement")

	return references.Position{}, references.NoneDirection
}
