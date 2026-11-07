package game_state

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestInterceptSmashedCommandZeroConsumesNoRNG(t *testing.T) {
	callbacks := NewMockSystemCallbacks(t)
	state := &GameState{SystemCallbacks: callbacks.ToSystemCallbacks()}
	state.SetRandomSeed(239)

	result, err := state.InterceptSmashedCommand()

	require.NoError(t, err)
	assert.Equal(t, SmashedCommandInterception{}, result)
	assert.Zero(t, state.GetRNG().Calls())
	assert.Empty(t, callbacks.Messages)
}

func TestInterceptSmashedCommandMissConsumesOnlyChanceRoll(t *testing.T) {
	callbacks := NewMockSystemCallbacks(t)
	state := &GameState{
		smashed:         3,
		SystemCallbacks: callbacks.ToSystemCallbacks(),
	}
	state.SetRandomSeed(smashedMissSeed())

	result, err := state.InterceptSmashedCommand()

	require.NoError(t, err)
	assert.False(t, result.Intercepted)
	assert.Equal(t, 3, result.SmashedBefore)
	assert.Equal(t, 3, result.SmashedAfter)
	assert.Equal(t, references.NoneDirection, result.Direction)
	assert.Equal(t, 3, state.smashed)
	assert.Equal(t, uint64(1), state.GetRNG().Calls())
	assert.Empty(t, callbacks.Messages)
}

func TestInterceptSmashedCommandPreservesOrderedGuardWorkBeforeDirection(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	controller, ok := state.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok)

	guard := guardActivationTestNPC(0, indexes.GuardKeyIndex, 0, false)
	guard.SetPos(references.Position{X: 14, Y: 15})
	firstCivilian := guardActivationTestNPC(1, indexes.TownsPersonKeyIndex, 1, true)
	secondCivilian := guardActivationTestNPC(2, indexes.MerchantKeyIndex, 1, true)
	controller.GetNpcs().ReplaceWith([]map_units.MapUnit{secondCivilian, guard, firstCivilian})
	state.smashed = 2
	state.SetCriminalStatus(false)

	const civilianRolls = 2
	seed := smashedHitSeedForDirection(2, civilianRolls)
	expectedRNG := beradrng.New(seed)
	require.Equal(t, 1, expectedRNG.Intn(2))
	for range civilianRolls {
		expectedRNG.Intn(guardActivationFleeRollMaximum + 1)
	}
	expectedDirection := smashedDirectionForSourceRoll(expectedRNG.Intn(4))

	messageRNGCalls := uint64(0)
	messageSmashed := 0
	guardHostileAtMessage := false
	originalAddRow := state.SystemCallbacks.Message.AddRowStr
	state.SystemCallbacks.Message.AddRowStr = func(message string) {
		messageRNGCalls = state.GetRNG().Calls()
		messageSmashed = state.smashed
		guardHostileAtMessage = guardActivationNPCIsHostile(guard)
		originalAddRow(message)
	}
	state.SetRandomSeed(seed)

	result, err := state.InterceptSmashedCommand()

	require.NoError(t, err)
	assert.True(t, result.Intercepted)
	assert.Equal(t, expectedDirection, result.Direction)
	assert.Equal(t, 2, result.SmashedBefore)
	assert.Equal(t, 1, result.SmashedAfter)
	assert.Equal(t, 2, result.GuardActivation.CivilianRollCount)
	assert.Equal(t, uint64(1+civilianRolls), messageRNGCalls,
		"chance and civilian rolls must precede feedback, while direction follows it")
	assert.Equal(t, 1, messageSmashed)
	assert.True(t, guardHostileAtMessage)
	assert.Equal(t, uint64(2+civilianRolls), state.GetRNG().Calls())
	assert.Equal(t, []string{"Hic!"}, callbacks.Messages)
	assert.False(t, state.IsCriminal())
	assert.Equal(t, expectedRNG.Intn(1000), state.RandomIntInRange(0, 999),
		"the next caller must follow chance, civilian, and direction rolls")

	controller.AdvanceNextTurnCalcAndMoveNPCs()
	assert.Equal(t, map_units.PendingNPCInteractionGuardArrest, guard.MapUnitDetails().PendingInteraction,
		"smashed activation must hand adjacent hostile Guard contact to arrest")
}

func TestInterceptSmashedCommandMapsAllSourceDirectionsAndReachesZero(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)

	for sourceRoll, expected := range []references.Direction{
		references.Up,
		references.Down,
		references.Right,
		references.Left,
	} {
		t.Run(expected.GetDirectionCompassName(), func(t *testing.T) {
			state.smashed = 1
			state.SetCriminalStatus(false)
			state.SetRandomSeed(smashedHitSeedForDirection(sourceRoll, 0))
			callbacks.Reset()

			result, err := state.InterceptSmashedCommand()

			require.NoError(t, err)
			assert.True(t, result.Intercepted)
			assert.Equal(t, expected, result.Direction)
			assert.Equal(t, 1, result.SmashedBefore)
			assert.Zero(t, result.SmashedAfter)
			assert.Zero(t, state.smashed)
			assert.Equal(t, uint64(2), state.GetRNG().Calls())
			assert.Equal(t, []string{"Hic!"}, callbacks.Messages)
			assert.False(t, state.IsCriminal())
		})
	}
}

func TestInterceptSmashedCommandReactivatesGuardsOnRepeatedHits(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	controller, ok := state.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok)
	guard := guardActivationTestNPC(0, indexes.GuardKeyIndex, 0, false)
	controller.GetNpcs().ReplaceWith([]map_units.MapUnit{guard})
	state.smashed = 2
	seed := smashedHitSeedForDirection(0, 0)

	state.SetRandomSeed(seed)
	first, err := state.InterceptSmashedCommand()
	require.NoError(t, err)
	state.SetRandomSeed(seed)
	second, err := state.InterceptSmashedCommand()
	require.NoError(t, err)

	assert.Zero(t, first.GuardActivation.HostileBefore)
	assert.Equal(t, 1, first.GuardActivation.HostileAfter)
	assert.Equal(t, 1, second.GuardActivation.HostileBefore)
	assert.Equal(t, 1, second.GuardActivation.HostileAfter)
	assert.Zero(t, state.smashed)
	assert.Equal(t, []string{"Hic!", "Hic!"}, callbacks.Messages)
}

func TestInterceptSmashedCommandGuardFailureStopsBeforeDecrementAndDirection(t *testing.T) {
	callbacks := NewMockSystemCallbacks(t)
	state := &GameState{
		smashed:         1,
		SystemCallbacks: callbacks.ToSystemCallbacks(),
	}
	state.MapState.PlayerLocation.Location = references.Britain
	state.SetRandomSeed(smashedHitSeedForDirection(0, 0))

	result, err := state.InterceptSmashedCommand()

	require.Error(t, err)
	assert.True(t, errors.Is(err, ErrInvalidGuardActivation))
	assert.False(t, result.Intercepted)
	assert.Equal(t, 1, result.SmashedBefore)
	assert.Equal(t, 1, result.SmashedAfter)
	assert.Equal(t, 1, state.smashed)
	assert.Equal(t, uint64(1), state.GetRNG().Calls())
	assert.Empty(t, callbacks.Messages)
}

func TestSmashedNativeSnapshotPreservesNextInterception(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15).
		WithSystemCallbacks().
		Build()
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	state.smashed = 1
	state.SetRandomSeed(smashedHitSeedForDirection(3, 0))
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
	hydratedCallbacks := NewMockSystemCallbacks(t)
	hydrated.SystemCallbacks = hydratedCallbacks.ToSystemCallbacks()
	hydrated.SetupEnvironmentalSystemCallbacks()
	callbacks.Reset()

	want, err := state.InterceptSmashedCommand()
	require.NoError(t, err)
	got, err := hydrated.InterceptSmashedCommand()
	require.NoError(t, err)

	assert.Equal(t, want, got)
	assert.Equal(t, state.smashed, hydrated.smashed)
	assert.Equal(t, state.GetRNG().Calls(), hydrated.GetRNG().Calls())
	assert.Equal(t, callbacks.Messages, hydratedCallbacks.Messages)
}

func TestSmashedLegacyTownContinuationPreservesNextInterception(t *testing.T) {
	gameReferences := getBaseGameReferences()
	if gameReferences == nil || cachedIntegrationCfg == nil {
		t.Skip("integration test requires original Ultima V game data")
	}
	// Patch a valid party save into Iolo's hut with the party smashed and no
	// saved town objects (32 eight-byte records at 0x6B4).
	rawSaveData := helpers.LoadBootstrapSaveData(t)
	clear(rawSaveData[0x6B4 : 0x6B4+32*8])
	rawSaveData[0x2ED] = byte(references.IolosHut)
	rawSaveData[0x2EF] = 0
	rawSaveData[0x2F0] = 16
	rawSaveData[0x2F1] = 16
	rawSaveData[0x03B1] = 1

	left := mustNewGameStateFromOriginalSaveBytes(
		rawSaveData, cachedIntegrationCfg, gameReferences,
		xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
	)
	right := mustNewGameStateFromOriginalSaveBytes(
		rawSaveData, cachedIntegrationCfg, gameReferences,
		xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
	)
	leftCallbacks := NewMockSystemCallbacks(t)
	rightCallbacks := NewMockSystemCallbacks(t)
	left.SystemCallbacks = leftCallbacks.ToSystemCallbacks()
	right.SystemCallbacks = rightCallbacks.ToSystemCallbacks()
	left.SetupEnvironmentalSystemCallbacks()
	right.SetupEnvironmentalSystemCallbacks()
	left.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	right.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	seed := smashedHitSeedForDirection(1, 0)
	left.SetRandomSeed(seed)
	right.SetRandomSeed(seed)

	want, err := left.InterceptSmashedCommand()
	require.NoError(t, err)
	got, err := right.InterceptSmashedCommand()
	require.NoError(t, err)

	assert.Equal(t, want, got)
	assert.Equal(t, 1, want.SmashedBefore)
	assert.Zero(t, left.smashed)
	assert.Zero(t, right.smashed)
	assert.Equal(t, left.GetRNG().Calls(), right.GetRNG().Calls())
	assert.Equal(t, leftCallbacks.Messages, rightCallbacks.Messages)
}

func smashedMissSeed() uint64 {
	for seed := uint64(0); ; seed++ {
		if beradrng.New(seed).Intn(2) == 0 {
			return seed
		}
	}
}

func smashedHitSeedForDirection(sourceRoll, civilianRolls int) uint64 {
	for seed := uint64(0); ; seed++ {
		rng := beradrng.New(seed)
		if rng.Intn(2) == 0 {
			continue
		}
		for range civilianRolls {
			rng.Intn(guardActivationFleeRollMaximum + 1)
		}
		if rng.Intn(4) == sourceRoll {
			return seed
		}
	}
}

func smashedDirectionForSourceRoll(sourceRoll int) references.Direction {
	switch sourceRoll {
	case 0:
		return references.Up
	case 1:
		return references.Down
	case 2:
		return references.Right
	case 3:
		return references.Left
	default:
		panic("invalid smashed direction source roll")
	}
}
