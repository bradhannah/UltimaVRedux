package game_state

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

func TestEnvironmentalHazardsIntegrationSwamp(t *testing.T) {
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(12345).
		Build()

	// Setup party with low dexterity for guaranteed poison
	char := gameState.PartyState.GetCharacter(0)
	char.Dexterity = 5 // Very low dexterity
	char.SetHp(20)
	char.SetMaxHp(20)
	char.SetStatus(party_state.Good)
	char.PartyMap = party_state.PartyMapActive

	// Find or create a swamp tile position for testing
	testPos := references.Position{X: 10, Y: 10}
	layeredMap := gameState.MapState.GetLayeredMapByCurrentLocation()

	// Set a swamp tile at the test position using map_state constants
	layeredMap.SetTileByLayer(0, &testPos, indexes.Swamp) // 0 = MapLayer

	// Move player to swamp tile
	gameState.MapState.PlayerLocation.Position = testPos

	// Process environmental hazards
	gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()
	drainTurnHazardFeedback(gameState)

	// Verify integration results
	assert.True(t, len(mockCallbacks.Messages) > 0, "Should show hazard message")
	// Check that a poison message was shown (character names come from SAVED.GAM)
	foundPoisonMessage := false
	for _, msg := range mockCallbacks.Messages {
		if len(msg) >= 9 && msg[:9] == "Poisoned:" {
			foundPoisonMessage = true
			break
		}
	}
	assert.True(t, foundPoisonMessage, "Should show poison message with character name")
	assert.Equal(t, party_state.Poisoned, gameState.PartyState.GetCharacter(0).GetStatus(), "Character should be poisoned")
	assert.Equal(t, []TransientIconKind{TransientIconPoisonOnset}, mockCallbacks.TransientIconCalls)
}

// A carpet lying on a swamp square is not ridden, so it does not keep a party on
// foot out of the swamp. See ref:ENV-0002.
func TestSwampPoisonsAFootPartyStandingOnAParkedCarpet(t *testing.T) {
	for _, location := range []references.Location{references.Britain, references.BritanniaUnderworld} {
		gameState, _ := NewIntegrationTestBuilder(t).WithLocation(location).WithFloor(0).WithPlayerAt(10, 10).
			WithSystemCallbacks().WithRandomSeed(12345).Build()
		character := gameState.PartyState.GetCharacter(0)
		character.Dexterity = 0 // Every swamp roll fails.
		character.SetStatus(party_state.Good)
		character.PartyMap = party_state.PartyMapActive
		position := gameState.MapState.PlayerLocation.Position
		gameState.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.Swamp)
		carpet := map_units.NewNPCFriendlyVehiceNewRef(references.CarpetVehicle, position, 0)
		require.True(t, gameState.CurrentNPCAIController.GetNpcs().AddVehicle(*carpet))
		gameState.CurrentNPCAIController.FreshenExistingNPCsOnMap()

		gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()

		assert.Equal(t, party_state.Poisoned, character.GetStatus(), "%s", location)
	}
}

// Lava, and a fireplace on a small map, burn whoever stands on them.
func TestEnvironmentalHazardsIntegrationBurningTiles(t *testing.T) {
	for _, test := range []struct {
		name     string
		tile     indexes.SpriteIndex
		seed     uint64
		position references.Position
	}{
		{name: "lava", tile: indexes.Lava, seed: 54321, position: references.Position{X: 15, Y: 15}},
		{name: "fireplace in town", tile: indexes.Fireplace, seed: 99999, position: references.Position{X: 12, Y: 12}},
	} {
		gameState, mockCallbacks := NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithSystemCallbacks().
			WithRandomSeed(test.seed).
			Build()
		const initialHP = uint16(30)
		char := gameState.PartyState.GetCharacter(0)
		char.SetHp(initialHP)
		char.SetMaxHp(initialHP)
		char.SetStatus(party_state.Good)
		char.PartyMap = party_state.PartyMapActive
		gameState.MapState.GetLayeredMapByCurrentLocation().SetTileByLayer(0, &test.position, test.tile)
		gameState.MapState.PlayerLocation.Position = test.position

		gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()
		drainTurnHazardFeedback(gameState)

		assert.True(t, slices.ContainsFunc(mockCallbacks.Messages, func(message string) bool {
			return strings.HasPrefix(message, "Burning:")
		}), "%s shows a burning message with character names", test.name)
		assert.Less(t, char.GetCurrentHp(), initialHP, test.name)
	}
}

func TestEnvironmentalHazardsIntegrationSafeTiles(t *testing.T) {
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(12345).
		Build()

	// Setup party member
	initialHP := uint16(20)
	char := gameState.PartyState.GetCharacter(0)
	char.SetHp(initialHP)
	char.SetMaxHp(initialHP)
	char.SetStatus(party_state.Good)
	char.PartyMap = party_state.PartyMapActive

	// Find or create a safe tile position for testing
	testPos := references.Position{X: 20, Y: 20}
	layeredMap := gameState.MapState.GetLayeredMapByCurrentLocation()

	// Set a safe tile at the test position
	layeredMap.SetTileByLayer(0, &testPos, indexes.BrickFloor) // MapLayer = 0

	// Move player to safe tile
	gameState.MapState.PlayerLocation.Position = testPos

	// Process environmental hazards
	gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()

	// Verify no hazard effects
	assert.Empty(t, mockCallbacks.Messages, "Should show no hazard messages")
	assert.Equal(t, party_state.Good, gameState.PartyState.GetCharacter(0).GetStatus(), "Character should remain in good status")
	assert.Equal(t, initialHP, gameState.PartyState.GetCharacter(0).GetCurrentHp(), "Character should not take damage")
}

func TestEnvironmentalHazardsSkipInactivePartyMembers(t *testing.T) {
	gameState, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(12345).
		Build()

	// Setup party with inactive member
	char0 := gameState.PartyState.GetCharacter(0)
	char0.Dexterity = 5 // Low dex for guaranteed poison if active
	char0.SetHp(20)
	char0.SetMaxHp(20)
	char0.SetStatus(party_state.Good)
	char0.PartyMap = party_state.PartyMapAtInn(references.Britain)

	char1 := gameState.PartyState.GetCharacter(1)
	char1.Dexterity = 5
	char1.SetHp(20)
	char1.SetMaxHp(20)
	char1.SetStatus(party_state.Good)
	char1.PartyMap = party_state.PartyMapActive // Active party member

	// Set swamp tile
	testPos := references.Position{X: 25, Y: 25}
	layeredMap := gameState.MapState.GetLayeredMapByCurrentLocation()
	layeredMap.SetTileByLayer(0, &testPos, indexes.Swamp)
	gameState.MapState.PlayerLocation.Position = testPos

	// Process environmental hazards
	gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()

	// Verify only active party member is affected
	assert.Equal(t, party_state.Good, gameState.PartyState.GetCharacter(0).GetStatus(), "Inactive member should not be poisoned")
	assert.Equal(t, party_state.Poisoned, gameState.PartyState.GetCharacter(1).GetStatus(), "Active member should be poisoned")
}

func TestTurnHazardRepeatedLavaAppliesOncePerCallBeforeFeedback(t *testing.T) {
	const seed = uint64(329)
	state, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	for index := range state.PartyState.GetCharacterCount() {
		member := state.PartyState.GetCharacter(index)
		member.PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	member := state.PartyState.GetCharacter(0)
	member.PartyMap = party_state.PartyMapActive
	member.SetMaxHp(100)
	member.SetHp(100)
	position := references.Position{X: 30, Y: 30}
	state.GetCurrentLayeredMap().SetTileByLayer(0, &position, indexes.Lava)
	state.MapState.PlayerLocation.Position = position
	state.SetRandomSeed(seed)
	callbacks.Reset()
	wantRNG := beradrng.New(seed)
	wantHP := uint16(100 - (wantRNG.Intn(8) + 1) - (wantRNG.Intn(8) + 1))

	require.NotPanics(t, func() {
		state.ProcessEnvironmentalHazardsOnTurnAdvancement()
		state.ProcessEnvironmentalHazardsOnTurnAdvancement()
	})

	assert.Equal(t, wantHP, member.GetCurrentHp())
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
	assert.False(t, state.EventQueueManager.HasWorkflow(turnHazardFeedbackName))
	assert.True(t, state.EventQueueManager.HasFeedback(turnHazardFeedbackName))
	assert.Empty(t, callbacks.Messages, "presentation must wait for the feedback lane")
	for range 30 {
		state.ElapsedMs += 20
		state.EventQueueManager.UpdateFeedback()
	}
	assert.Equal(t, wantHP, member.GetCurrentHp(), "feedback must not apply more damage")
	assert.Equal(t, uint64(2), state.GetRNG().Calls(), "feedback must not consume gameplay RNG")
	assert.Len(t, callbacks.Messages, 1, "coalesced feedback presents once")
}

func TestTurnHazardRepeatedSwampMutatesBeforeCoalescedFeedback(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	for index := range state.PartyState.GetCharacterCount() {
		member := state.PartyState.GetCharacter(index)
		member.PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	member := state.PartyState.GetCharacter(0)
	member.PartyMap = party_state.PartyMapActive
	member.Dexterity = 0
	member.SetStatus(party_state.Good)
	position := references.Position{X: 30, Y: 30}
	state.GetCurrentLayeredMap().SetTileByLayer(0, &position, indexes.Swamp)
	state.MapState.PlayerLocation.Position = position
	seed := uint64(0)
	for {
		rng := beradrng.New(seed)
		if rng.Intn(30) > 0 && rng.Intn(30) > 0 {
			break
		}
		seed++
	}
	state.SetRandomSeed(seed)

	state.ProcessEnvironmentalHazardsOnTurnAdvancement()
	assert.Equal(t, party_state.Poisoned, member.GetStatus())
	member.SetStatus(party_state.Good)
	require.NotPanics(t, state.ProcessEnvironmentalHazardsOnTurnAdvancement)

	assert.Equal(t, party_state.Poisoned, member.GetStatus())
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
	assert.True(t, state.EventQueueManager.HasFeedback(turnHazardFeedbackName))
	for range 30 {
		state.ElapsedMs += 20
		state.EventQueueManager.UpdateFeedback()
	}
	assert.Equal(t, party_state.Poisoned, member.GetStatus())
	assert.Equal(t, uint64(2), state.GetRNG().Calls())
}

func TestEnvironmentalHazardsLazyCaching(t *testing.T) {
	gameState, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithRandomSeed(12345).
		Build()

	// Environmental hazards system is now initialized during GameState creation
	assert.NotNil(t, gameState.environmentalHazards, "Environmental hazards should be initialized during GameState creation")

	// Processing environmental hazards should work with initialized system
	gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()
	assert.NotNil(t, gameState.environmentalHazards, "Environmental hazards should remain initialized after processing")

	// Second call should reuse the same system
	firstInstance := gameState.environmentalHazards
	gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()
	assert.Equal(t, firstInstance, gameState.environmentalHazards, "Should reuse existing environmental hazards instance")
}

func TestMovementAppliesStandingHazardOnlyDuringOrdinaryCompletion(t *testing.T) {
	const seed = uint64(3290)
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(30, 30).
		WithSystemCallbacks().
		Build()
	for index := range state.PartyState.GetCharacterCount() {
		state.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	member := state.PartyState.GetCharacter(0)
	member.PartyMap = party_state.PartyMapActive
	member.SetMaxHp(100)
	member.SetHp(100)
	position := state.MapState.PlayerLocation.Position
	state.GetCurrentLayeredMap().SetTileByLayer(0, &position, indexes.Lava)
	state.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectTimeStop, TurnsRemaining: 2}
	state.SetRandomSeed(seed)
	wantRNG := beradrng.New(seed)
	wantHP := uint16(100 - (wantRNG.Intn(8) + 1))

	state.CompleteSurfaceMovementCycle(references.LargeMapType, true)
	assert.Equal(t, wantHP, member.GetCurrentHp())
	assert.Equal(t, uint64(1), state.GetRNG().Calls())
}

func drainTurnHazardFeedback(state *GameState) {
	for range 30 {
		state.ElapsedMs += 20
		state.EventQueueManager.UpdateFeedback()
	}
}

// TestFireplaceAudioVisualEffects_Integration tests the new sound and screen flash effects for fireplace damage
func TestFireplaceAudioVisualEffects_Integration(t *testing.T) {
	gameState, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain). // Small map location
		WithSystemCallbacks().
		WithRandomSeed(42).
		Build()

	// Setup party member
	initialHP := uint16(30)
	char := gameState.PartyState.GetCharacter(0)
	char.SetHp(initialHP)
	char.SetMaxHp(initialHP)
	char.SetStatus(party_state.Good)
	char.PartyMap = party_state.PartyMapActive

	// Set fireplace tile
	testPos := references.Position{X: 16, Y: 16}
	layeredMap := gameState.MapState.GetLayeredMapByCurrentLocation()
	layeredMap.SetTileByLayer(0, &testPos, indexes.Fireplace)
	gameState.MapState.PlayerLocation.Position = testPos

	// Clear previous callbacks
	mockCallbacks.Reset()

	// Gameplay resolves synchronously; visual feedback drains independently.
	gameState.ProcessEnvironmentalHazardsOnTurnAdvancement()

	// Process event queue through animation (pattern from lava_damage_event_queue_test.go)
	// 20 iterations * 20ms = 400ms total, covering the 300ms flash animation
	for i := 0; i < 20; i++ {
		gameState.ElapsedMs += 20
		gameState.EventQueueManager.UpdateFeedback()
	}

	// Verify audio callback was triggered
	assert.True(t, len(mockCallbacks.SoundEffectsPlayed) > 0, "Should have fire sound effect")
	assert.Equal(t, SoundFireBurn, mockCallbacks.SoundEffectsPlayed[0], "Should play semantic fire sound")
	assert.NotContains(t, mockCallbacks.SoundEffectsPlayed, SoundHeal, "Burning must not request healing audio")

	// Verify screen flash callback was triggered with correct parameters
	// Note: Event queue pattern calls SetScreenFlashColor and SetScreenFlashIntensity
	// multiple times during animation, so we check that calls were made
	assert.True(t, len(mockCallbacks.ScreenFlashColors) > 0, "Should have flash color calls")
	assert.True(t, len(mockCallbacks.ScreenFlashIntensities) > 0, "Should have flash intensity calls")

	// Verify flash color has correct RGB values (fire: orange-red)
	flashColor := mockCallbacks.ScreenFlashColors[0]
	assert.Equal(t, 1.0, flashColor.R, "Red component should be 1.0")
	assert.Equal(t, 0.2, flashColor.G, "Green component should be 0.2")
	assert.Equal(t, 0.1, flashColor.B, "Blue component should be 0.1")

	// Verify peak intensity is in expected range (should reach ~0.65-0.70 for fire)
	// Flash animation uses triangle wave (0 → max → 0), so find the maximum intensity
	maxIntensity := 0.0
	for _, intensity := range mockCallbacks.ScreenFlashIntensities {
		if intensity > maxIntensity {
			maxIntensity = intensity
		}
	}
	assert.InDelta(t, 0.7, maxIntensity, 0.1, "Peak intensity should be approximately 0.7")

	// Verify burning message is shown
	assert.True(t, len(mockCallbacks.Messages) > 0, "Should show burning message")
	assert.Contains(t, mockCallbacks.Messages[0], "Burning:", "Should show burning message")

	// Verify damage was taken
	assert.True(t, gameState.PartyState.GetCharacter(0).GetCurrentHp() < initialHP, "Character should have taken damage")
}
