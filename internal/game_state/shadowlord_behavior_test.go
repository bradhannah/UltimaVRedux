package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

// Shadowlord Behavior Tests - TDD tests based on original Ultima V behavior
// See docs/gamefacts/SHADOWLORDS.md for complete behavior documentation.
//
// KEY DISTINCTION (from original source code):
//   - YELL SUMMONING: NPCs are NOT affected (shadow_effects() never called)
//   - AUTOMATIC PRESENCE: NPCs ARE affected (shadow_effects() called on town entry)
//
// These tests validate the 6 behavior combinations documented in SHADOWLORDS.md.

// =============================================================================
// YELL SUMMONING TESTS - NPCs should NOT be affected
// =============================================================================

// Town Yell summons a Shadowlord without its automatic-presence effects: NPC AI
// is untouched and no automatic presence (or vegetation pass) is recorded.
// See ref:SHADOW-0002.
func TestYellSummoningAppliesNoTownEffects(t *testing.T) {
	for _, name := range []string{"ASTAROTH", "FAULINEI", "NOSFENTOR"} {
		gs, _ := newBootstrapSaveTestState(t)
		gs.ShadowlordState.Reset()
		gs.TransitionToLocation(references.Lycaeum, 0, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)
		controller, ok := gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
		require.True(t, ok)
		before := countNPCsWithOverriddenAI(controller)

		require.True(t, gs.TrySummonShadowlord(name), name)
		assert.Equal(t, before, countNPCsWithOverriddenAI(controller), "%s must not override NPC AI", name)
		assert.Equal(t, references.Shadowlord(-1), gs.ShadowlordState.AutomaticPresence,
			"%s must not set automatic presence", name)
	}
}

// =============================================================================
// AUTOMATIC PRESENCE TESTS - NPCs SHOULD be affected
// =============================================================================

// TestAutomaticPresence_Astaroth_MakesNPCsHostile validates that automatic
// Astaroth presence (moon-based) DOES apply behavior effects to NPCs.
// ~50% of regular NPCs should become hostile (DrudgeWorthThing/action 7 AI).
//
// Source: original town rules - case 1 sets hostile schedule override and tlk=0xfe
// See ref:SHADOW-0002.
func TestAutomaticPresence_Astaroth_MakesNPCsHostile(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	// Set fixed seed for deterministic 50% selection
	gs.SetRandomSeed(12345)

	// Transition to Lycaeum (Keep) to properly set up small map controller
	gs.TransitionToLocation(references.Lycaeum, 0, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)

	smallMapController, ok := gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok, "Should have small map controller")

	// Simulate AUTOMATIC presence (not yell) by directly calling the effect
	// This simulates what should happen on town entry with moon-based presence
	gs.ApplyShadowlordEffectsToNPCs(references.Astaroth)

	// Count hostile NPCs
	hostileCount := countNPCsWithAIType(smallMapController, references.DrudgeWorthThing)

	// With 50% chance and deterministic seed, we should have some hostile NPCs
	assert.Greater(t, hostileCount, 0,
		"Automatic Astaroth presence should make some NPCs hostile")
}

// TestAutomaticPresence_Nosfentor_MakesNPCsFlee validates that automatic
// Nosfentor presence (moon-based) DOES apply flee behavior to NPCs.
// ~50% of regular NPCs should flee (FleeFromAvatar AI).
//
// Source: original town rules - case 2 sets fleeing schedule override and tlk=0xfd
// See ref:SHADOW-0002.
func TestAutomaticPresence_Nosfentor_MakesNPCsFlee(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)
	gs.ShadowlordState.Reset()

	gs.SetRandomSeed(12345)

	// Transition to Lycaeum (Keep) to properly set up small map controller
	gs.TransitionToLocation(references.Lycaeum, 0, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)

	smallMapController, ok := gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok, "Should have small map controller")

	// Simulate AUTOMATIC presence
	gs.ApplyShadowlordEffectsToNPCs(references.Nosfentor)

	// Count fleeing NPCs
	fleeingCount := countNPCsWithAIType(smallMapController, references.FleeFromAvatar)

	assert.Greater(t, fleeingCount, 0,
		"Automatic Nosfentor presence should make some NPCs flee")
}

// TestAutomaticPresence_Faulinei_NoNPCBehaviorChange validates that automatic
// Faulinei presence does NOT change NPC behavior (only affects shops).
//
// Source: original town rules - case 0 has no NPC behavior code
// See ref:SHADOW-0002.
func TestAutomaticPresence_Faulinei_NoNPCBehaviorChange(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	// Transition to Lycaeum (Keep) to properly set up small map controller
	gs.TransitionToLocation(references.Lycaeum, 0, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)

	smallMapController, ok := gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok, "Should have small map controller")

	npcsBefore := countNPCsWithOverriddenAI(smallMapController)

	// Simulate AUTOMATIC presence
	gs.ApplyShadowlordEffectsToNPCs(references.Faulinei)

	npcsAfter := countNPCsWithOverriddenAI(smallMapController)

	assert.Equal(t, npcsBefore, npcsAfter,
		"Faulinei automatic presence should NOT affect NPC behavior (only shops)")
}

// =============================================================================
// NPC EFFECT PERSISTENCE TESTS
// =============================================================================

// TestNPCEffects_PersistAcrossFloorChanges validates that NPC Shadowlord effects
// persist when changing floors within the same location.
//
// Source: scheduled NPC placement does NOT reset loaded actor dialogue state values
// Implementation: AutomaticPresence tracks moon-based presence, which is checked
// by hardResetSmallMapAfterFloorChange to preserve overridden AI types.
func TestNPCEffects_PersistAcrossFloorChanges(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.SetRandomSeed(12345)

	// Transition to Lycaeum (Keep) to properly set up small map controller
	gs.TransitionToLocation(references.Lycaeum, 0, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)

	smallMapController, ok := gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok, "Should have small map controller")

	// Apply Astaroth effects
	gs.ShadowlordState.AutomaticPresence = references.Astaroth
	gs.ApplyShadowlordEffectsToNPCs(references.Astaroth)

	hostileCountBefore := countNPCsWithAIType(smallMapController, references.DrudgeWorthThing)
	require.Greater(t, hostileCountBefore, 0, "Should have some hostile NPCs")
	provenanceCountBefore := countNPCsWithAIOverrideProvenance(
		smallMapController,
		references.AIOverrideProvenanceAutomaticAstaroth,
	)
	require.Equal(t, hostileCountBefore, provenanceCountBefore)

	// Change floor within same location (preserveOverriddenAI should be true)
	gs.TransitionToLocation(references.Lycaeum, 1, gs.MapState.PlayerLocation.Position, TransitionFloorOnly, false)

	// Get new controller reference after transition
	smallMapController, ok = gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok, "Should still have small map controller")

	hostileCountAfter := countNPCsWithAIType(smallMapController, references.DrudgeWorthThing)
	provenanceCountAfter := countNPCsWithAIOverrideProvenance(
		smallMapController,
		references.AIOverrideProvenanceAutomaticAstaroth,
	)

	// Effects should persist
	assert.Equal(t, hostileCountBefore, hostileCountAfter,
		"NPC Shadowlord effects should persist across floor changes")
	assert.Equal(t, provenanceCountBefore, provenanceCountAfter,
		"NPC Shadowlord provenance should persist across floor changes")
}

// =============================================================================
// FAULINEI SHOP THEFT TESTS
// =============================================================================

// TestFaulineiShopTheft_AutomaticPresence validates that Faulinei steals gold
// during shop transactions when automatically present.
//
// Source: original shop rules - Faulinei shop theft function
// See ref:SHADOW-0002.
func TestFaulineiShopTheft_AutomaticPresence(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.SetRandomSeed(12345)

	// Set up automatic Faulinei presence (NOT yell summoning)
	gs.ShadowlordState.AutomaticPresence = references.Faulinei

	// Give the party some gold
	initialGold := uint16(100)
	gs.PartyState.Inventory.Gold.Set(initialGold)

	// Apply shop theft
	stolenAmount := gs.ApplyFaulineiShopTheft()

	// Verify gold was stolen (1-64 gold)
	assert.Greater(t, stolenAmount, uint16(0), "Should have stolen some gold")
	assert.LessOrEqual(t, stolenAmount, uint16(64), "Should steal at most 64 gold")

	// Verify gold was actually deducted
	remainingGold := gs.PartyState.Inventory.Gold.Get()
	assert.Equal(t, initialGold-stolenAmount, remainingGold,
		"Remaining gold should be initial minus stolen")
}

// TestFaulineiShopTheft_YellSummoning_NoTheft validates that yell-summoned
// Faulinei does NOT cause shop theft.
//
// Source: Yell summoning sets last_called but NOT shadowlord global
func TestFaulineiShopTheft_YellSummoning_NoTheft(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.SetRandomSeed(12345)

	// Set up YELL summoning (NOT automatic presence)
	gs.ShadowlordState.Summoned = true
	gs.ShadowlordState.LastCalled = references.Faulinei
	// AutomaticPresence should remain -1 (not set)

	// Give the party some gold
	initialGold := uint16(100)
	gs.PartyState.Inventory.Gold.Set(initialGold)

	// Apply shop theft - should return 0 because it's yell summoning
	stolenAmount := gs.ApplyFaulineiShopTheft()

	// Verify NO gold was stolen
	assert.Equal(t, uint16(0), stolenAmount, "Should NOT steal gold during yell summoning")

	// Verify gold is unchanged
	remainingGold := gs.PartyState.Inventory.Gold.Get()
	assert.Equal(t, initialGold, remainingGold, "Gold should be unchanged")
}

// TestFaulineiShopTheft_OtherShadowlord_NoTheft validates that Astaroth and
// Nosfentor presence does NOT cause shop theft.
func TestFaulineiShopTheft_OtherShadowlord_NoTheft(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.SetRandomSeed(12345)

	// Test Astaroth
	gs.ShadowlordState.AutomaticPresence = references.Astaroth
	gs.PartyState.Inventory.Gold.Set(100)

	stolenAmount := gs.ApplyFaulineiShopTheft()
	assert.Equal(t, uint16(0), stolenAmount, "Astaroth should NOT steal gold")
	assert.Equal(t, uint16(100), gs.PartyState.Inventory.Gold.Get(), "Gold unchanged with Astaroth")

	// Test Nosfentor
	gs.ShadowlordState.AutomaticPresence = references.Nosfentor

	stolenAmount = gs.ApplyFaulineiShopTheft()
	assert.Equal(t, uint16(0), stolenAmount, "Nosfentor should NOT steal gold")
	assert.Equal(t, uint16(100), gs.PartyState.Inventory.Gold.Get(), "Gold unchanged with Nosfentor")
}

// TestFaulineiShopTheft_NoGold validates behavior when party has no gold.
func TestFaulineiShopTheft_NoGold(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.SetRandomSeed(12345)

	// Set up automatic Faulinei presence
	gs.ShadowlordState.AutomaticPresence = references.Faulinei

	// Set gold to 0
	gs.PartyState.Inventory.Gold.Set(0)

	// Apply shop theft
	stolenAmount := gs.ApplyFaulineiShopTheft()

	// Verify nothing stolen
	assert.Equal(t, uint16(0), stolenAmount, "Cannot steal from empty wallet")
	assert.Equal(t, uint16(0), gs.PartyState.Inventory.Gold.Get(), "Gold remains 0")
}

// TestFaulineiShopTheft_LimitedGold validates that theft is limited to available gold.
func TestFaulineiShopTheft_LimitedGold(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.SetRandomSeed(12345)

	// Set up automatic Faulinei presence
	gs.ShadowlordState.AutomaticPresence = references.Faulinei

	// Set gold to a small amount (less than max theft of 64)
	smallAmount := uint16(5)
	gs.PartyState.Inventory.Gold.Set(smallAmount)

	// Apply shop theft
	stolenAmount := gs.ApplyFaulineiShopTheft()

	// Verify theft is limited to available gold
	assert.LessOrEqual(t, stolenAmount, smallAmount,
		"Cannot steal more than available gold")

	// Verify gold didn't go negative
	remainingGold := gs.PartyState.Inventory.Gold.Get()
	assert.GreaterOrEqual(t, remainingGold, uint16(0), "Gold should not be negative")
	assert.Equal(t, smallAmount-stolenAmount, remainingGold,
		"Remaining gold should equal initial minus stolen")
}

// =============================================================================
// MAP EFFECTS TESTS - Crops/Trees destruction during automatic presence
// =============================================================================

// TestMapEffects_AutomaticPresence_DestroysCrops validates that automatic
// Shadowlord presence destroys crops (wheat fields -> plowed fields).
//
// Source: original town rules - vegetation effects function
// Note: 7/8 (87.5%) chance per affected tile
// See ref:SHADOW-0002.
func TestMapEffects_AutomaticPresence_DestroysCrops(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	// Set deterministic seed for 7/8 probability testing
	gs.SetRandomSeed(12345)

	// Transition to a town with crops (Britain has farms)
	gs.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 15}, TransitionFullLocation, false)

	// Apply map effects for any Shadowlord (all 3 cause same map effects)
	destroyed, total := gs.ApplyShadowlordMapEffects(references.Astaroth)

	// With 7/8 probability and deterministic seed, most crops should be destroyed
	// We just verify the function works - actual crop count depends on map data
	t.Logf("Map effects: destroyed %d of %d crop/tree tiles", destroyed, total)

	// If there were any crops/trees, some should be destroyed (7/8 chance)
	if total > 0 {
		assert.Greater(t, destroyed, 0, "Should destroy some crops/trees with 7/8 probability")
	}
}

func TestMapEffectsUseDaySeededYMajorCandidateOnlyStream(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	const day = byte(12)
	gs.DateTime.Day = day
	positions := []references.Position{
		{X: 5, Y: 1},
		{X: 2, Y: 2},
		{X: 9, Y: 2},
		{X: 1, Y: 7},
	}
	prepareShadowlordEffectCandidates(gs, positions)
	gs.SetRandomSeed(444)
	gs.RandomIntInRange(1, 99)
	sharedCalls := gs.GetRNG().Calls()
	oracle := beradrng.New(uint64(day))

	destroyed, total := gs.ApplyShadowlordMapEffects(references.Astaroth)

	assert.Equal(t, len(positions), total)
	assert.Equal(t, sharedCalls, gs.GetRNG().Calls())
	wantDestroyed := 0
	for index, position := range positions {
		want := indexes.SpriteIndex(indexes.WheatInField)
		if index%2 != 0 {
			want = indexes.Tree
		}
		if oracle.Intn(8) != 0 {
			wantDestroyed++
			want = getShadowlordMapEffectReplacement(want)
		}
		assert.Equal(t, want, gs.GetCurrentLayeredMap().GetTileByLayer(map_state.MapLayer, &position).Index,
			"candidate %d must use the next Y-major/X-major day-seeded draw", index)
	}
	assert.Equal(t, wantDestroyed, destroyed)
}

func TestMapEffectsSameDayIgnoreSharedGameplayHistoryAndDifferentDaysCanDiffer(t *testing.T) {
	positions := []references.Position{
		{X: 1, Y: 1}, {X: 2, Y: 1}, {X: 3, Y: 1}, {X: 4, Y: 1},
		{X: 5, Y: 1}, {X: 6, Y: 1}, {X: 7, Y: 1}, {X: 8, Y: 1},
	}
	patterns := make([][]indexes.SpriteIndex, 3)
	for index, fixture := range []struct {
		day         byte
		sharedDraws int
		sharedSeed  uint64
	}{{day: 11, sharedDraws: 0, sharedSeed: 1}, {day: 11, sharedDraws: 19, sharedSeed: 999}, {day: 12, sharedDraws: 7, sharedSeed: 1}} {
		gs, _ := NewIntegrationTestBuilder(t).
			WithLocation(references.Britain).
			WithSystemCallbacks().
			Build()
		gs.DateTime.Day = fixture.day
		prepareShadowlordEffectCandidates(gs, positions)
		gs.SetRandomSeed(fixture.sharedSeed)
		for range fixture.sharedDraws {
			gs.RollDice(30)
		}
		callsBefore := gs.GetRNG().Calls()

		gs.ApplyShadowlordMapEffects(references.Nosfentor)

		assert.Equal(t, callsBefore, gs.GetRNG().Calls())
		patterns[index] = shadowlordEffectPattern(gs, positions)
	}

	assert.Equal(t, patterns[0], patterns[1])
	assert.NotEqual(t, patterns[0], patterns[2])
}

func TestShadowlordTownReentryRebuildsSourceTerrainBeforeSameDayEffectPass(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	gs.DateTime.Day = 9
	gs.ShadowlordState.Reset()
	gs.ShadowlordState.TownAssignments[references.Astaroth] = references.Britain

	gs.TransitionToLocation(references.Britain, references.MainFloor, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)
	first := currentSmallMapTerrain(gs)
	gs.TransitionToLocation(references.BritanniaUnderworld, references.MainFloor, references.Position{X: 10, Y: 10}, TransitionFullLocation, false)
	gs.TransitionToLocation(references.Britain, references.MainFloor, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)

	assert.Equal(t, first, currentSmallMapTerrain(gs))
}

func prepareShadowlordEffectCandidates(gs *GameState, positions []references.Position) {
	layeredMap := gs.GetCurrentLayeredMap()
	for y := references.Coordinate(0); y < references.YSmallMapTiles; y++ {
		for x := references.Coordinate(0); x < references.XSmallMapTiles; x++ {
			position := references.Position{X: x, Y: y}
			layeredMap.SetTileByLayer(map_state.MapLayer, &position, indexes.Grass)
		}
	}
	for index, position := range positions {
		tile := indexes.SpriteIndex(indexes.WheatInField)
		if index%2 != 0 {
			tile = indexes.Tree
		}
		layeredMap.SetTileByLayer(map_state.MapLayer, &position, tile)
	}
}

func shadowlordEffectPattern(gs *GameState, positions []references.Position) []indexes.SpriteIndex {
	pattern := make([]indexes.SpriteIndex, len(positions))
	for index, position := range positions {
		pattern[index] = gs.GetCurrentLayeredMap().GetTileByLayer(map_state.MapLayer, &position).Index
	}

	return pattern
}

func currentSmallMapTerrain(gs *GameState) []indexes.SpriteIndex {
	layeredMap := gs.GetCurrentLayeredMap()
	terrain := make([]indexes.SpriteIndex, 0, references.XSmallMapTiles*references.YSmallMapTiles)
	for y := references.Coordinate(0); y < references.YSmallMapTiles; y++ {
		for x := references.Coordinate(0); x < references.XSmallMapTiles; x++ {
			position := references.Position{X: x, Y: y}
			terrain = append(terrain, layeredMap.GetTileByLayer(map_state.MapLayer, &position).Index)
		}
	}

	return terrain
}

// TestMapEffects_SevenEighthsProbability validates the 7/8 (87.5%) probability.
// We test this by running multiple iterations and checking the distribution.
func TestMapEffects_SevenEighthsProbability(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	// Test the probability function directly
	// Run 1000 iterations and check it's close to 87.5%
	gs.SetRandomSeed(42)
	successes := 0
	iterations := 1000

	for i := 0; i < iterations; i++ {
		// 7/8 = NOT (1 in 8)
		if !gs.OneInXOdds(8) {
			successes++
		}
	}

	// Expected: 87.5% = 875 successes
	// Allow 5% tolerance: 825-925
	successRate := float64(successes) / float64(iterations)
	assert.InDelta(t, 0.875, successRate, 0.05,
		"7/8 probability should be approximately 87.5%% (got %.1f%%)", successRate*100)
}

// TestMapEffects_CropsToPlowedFields verifies wheat -> plowed field conversion
func TestMapEffects_CropsToPlowedFields(t *testing.T) {
	// Test the tile conversion logic directly
	assert.Equal(t, indexes.SpriteIndex(indexes.PlowedField), getShadowlordMapEffectReplacement(indexes.WheatInField),
		"Wheat should become plowed field")
}

// TestMapEffects_TreesToStumps verifies tree -> dead tree conversion
func TestMapEffects_TreesToStumps(t *testing.T) {
	// Test the tile conversion logic directly
	assert.Equal(t, indexes.SpriteIndex(indexes.DeadTree), getShadowlordMapEffectReplacement(indexes.Tree),
		"Tree should become dead tree (stump)")
}

// TestMapEffects_OtherTilesUnchanged verifies non-crop/tree tiles are not affected
func TestMapEffects_OtherTilesUnchanged(t *testing.T) {
	// Non-affected tiles should return themselves
	assert.Equal(t, indexes.SpriteIndex(indexes.Grass), getShadowlordMapEffectReplacement(indexes.Grass),
		"Grass should remain grass")
	assert.Equal(t, indexes.SpriteIndex(indexes.Water1), getShadowlordMapEffectReplacement(indexes.Water1),
		"Water should remain water")
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), getShadowlordMapEffectReplacement(indexes.BrickFloor),
		"Brick should remain brick")
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// countNPCsWithOverriddenAI counts NPCs that have any overridden AI type
func countNPCsWithOverriddenAI(controller *ai.NPCAIControllerSmallMap) int {
	count := 0
	npcs := controller.GetNpcs()

	for _, mapUnit := range *npcs {
		if friendly, ok := mapUnit.(*map_units.NPCFriendly); ok {
			// references.Unset (-1) is the default unset value
			if friendly.MapUnitDetails().GetOverriddenAiType() != references.Unset {
				count++
			}
		}
	}

	return count
}

// countNPCsWithAIType counts NPCs with a specific overridden AI type
func countNPCsWithAIType(controller *ai.NPCAIControllerSmallMap, aiType references.AiType) int {
	count := 0
	npcs := controller.GetNpcs()

	for _, mapUnit := range *npcs {
		if friendly, ok := mapUnit.(*map_units.NPCFriendly); ok {
			if friendly.MapUnitDetails().GetOverriddenAiType() == aiType {
				count++
			}
		}
	}

	return count
}

func countNPCsWithAIOverrideProvenance(
	controller *ai.NPCAIControllerSmallMap,
	provenance references.AIOverrideProvenance,
) int {
	count := 0
	for _, mapUnit := range *controller.GetNpcs() {
		friendly, ok := mapUnit.(*map_units.NPCFriendly)
		if ok && friendly.MapUnitDetails().AIOverrideProvenance() == provenance {
			count++
		}
	}

	return count
}

func countShadowlordNPCs(controller *ai.NPCAIControllerSmallMap) int {
	count := 0
	for _, mapUnit := range *controller.GetNpcs() {
		enemy, ok := mapUnit.(*map_units.NPCEnemy)
		if !ok {
			continue
		}
		if _, isShadowlord := enemy.ShadowlordIdentity(); isShadowlord {
			count++
		}
	}

	return count
}

// =============================================================================
// SHADOWLORD SCHEDULE SYSTEM TESTS - Persistent Midnight Assignments
// =============================================================================

func TestShadowlordAssignmentsStartUnassignedAndQueriesDoNotConsumeRNG(t *testing.T) {
	config := helpers.LoadTestConfiguration(t)
	gameRefs := helpers.LoadTestGameReferences(t, config)
	saveData := helpers.LoadBootstrapSaveData(t)
	xTiles, yTiles := helpers.GetTestGameConstants()
	gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)
	gs.ShadowlordState.Reset()
	before := gs.GetRNG().Calls()

	for location := ShadowlordTownMinLocation; location <= ShadowlordTownMaxLocation; location++ {
		assert.Equal(t, references.NoShadowlord, gs.GetShadowlordForLocation(references.Location(location)))
	}
	assert.Empty(t, gs.GetShadowlordPlanetIndices())
	assert.Empty(t, gs.GetAllShadowlordPositions())
	assert.Equal(t, before, gs.GetRNG().Calls())
}

func TestMoveShadowlordsAtMidnightUsesGameplayRNGAndPersistentUniqueAssignments(t *testing.T) {
	config := helpers.LoadTestConfiguration(t)
	gameRefs := helpers.LoadTestGameReferences(t, config)
	saveData := helpers.LoadBootstrapSaveData(t)
	xTiles, yTiles := helpers.GetTestGameConstants()
	gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)
	gs.ShadowlordState.Reset()
	gs.SetRandomSeed(310)
	gs.MapState.PlayerLocation.Location = references.Britain
	gs.ShadowlordState.TownAssignments = [references.ShadowlordCount]references.Location{
		references.Moonglow, references.Jhelom, references.Yew,
	}

	gs.moveShadowlordsAtMidnight()

	assert.Equal(t, [references.ShadowlordCount]references.Location{
		references.NewMagincia, references.SkaraBrae, references.Moonglow,
	}, gs.ShadowlordState.TownAssignments)
	assert.Equal(t, uint64(6), gs.GetRNG().Calls())
	assert.Len(t, gs.GetAllShadowlordPositions(), references.ShadowlordCount)
	assert.Len(t, gs.GetShadowlordPlanetIndices(), references.ShadowlordCount)
}

func TestAstronomyOmitsDefeatedShadowlord(t *testing.T) {
	config := helpers.LoadTestConfiguration(t)
	gameRefs := helpers.LoadTestGameReferences(t, config)
	saveData := helpers.LoadBootstrapSaveData(t)
	xTiles, yTiles := helpers.GetTestGameConstants()
	gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)
	gs.ShadowlordState.Reset()
	gs.ShadowlordState.TownAssignments = [references.ShadowlordCount]references.Location{
		references.Moonglow, references.Jhelom, references.Yew,
	}
	gs.ShadowlordState.Kill(references.Astaroth)

	assert.Equal(t, []int{0, 3}, gs.GetShadowlordPlanetIndices())
	assert.Equal(t, map[references.Location]references.Shadowlord{
		references.Moonglow: references.Faulinei,
		references.Yew:      references.Nosfentor,
	}, gs.GetAllShadowlordPositions())
	assert.Equal(t, references.NoShadowlord, gs.GetShadowlordForLocation(references.Jhelom))
}

func TestMidnightMovesLivingShadowlordsWithoutRefreshingLoadedPresence(t *testing.T) {
	config := helpers.LoadTestConfiguration(t)
	gameRefs := helpers.LoadTestGameReferences(t, config)
	saveData := helpers.LoadBootstrapSaveData(t)
	xTiles, yTiles := helpers.GetTestGameConstants()
	gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)
	mockCallbacks := NewMockSystemCallbacks(t)
	gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()
	gs.ShadowlordState.Reset()
	gs.ShadowlordState.TownAssignments = [references.ShadowlordCount]references.Location{references.Britain, references.Jhelom, references.Yew}
	result := gs.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 10}, TransitionFullLocation, false)
	require.Equal(t, []references.Shadowlord{references.Faulinei}, result.ShadowlordsPresent)
	require.Equal(t, references.Faulinei, gs.ShadowlordState.AutomaticPresence)
	controller, ok := gs.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok)
	shadowlordNPCs := countShadowlordNPCs(controller)
	require.Equal(t, 1, shadowlordNPCs)
	gs.SetRandomSeed(310)
	gs.DateTime.Hour, gs.DateTime.Minute = 23, 59
	gs.ShadowlordState.Kill(references.Nosfentor)

	gs.advanceElapsedGameplayMinutes(1, false)

	assert.Equal(t, byte(0), gs.DateTime.Hour)
	assert.Equal(t, references.Faulinei, gs.ShadowlordState.AutomaticPresence)
	assert.Equal(t, shadowlordNPCs, countShadowlordNPCs(controller))
	assert.NotEqual(t, references.Britain, gs.ShadowlordState.TownAssignments[references.Faulinei])
	assert.NotEqual(t, references.Jhelom, gs.ShadowlordState.TownAssignments[references.Astaroth])
	assert.Zero(t, gs.ShadowlordState.TownAssignments[references.Nosfentor])
}

// TestShadowlordTownEntry_DetectsPresence validates that entering a town
// where a Shadowlord is assigned triggers automatic presence effects.
// Source: Original town initialization in original town rules check Shadowlord assignment state array on entry.
// See ref:SHADOW-0002.
func TestShadowlordTownEntry_DetectsPresence(t *testing.T) {
	for _, lord := range []references.Shadowlord{references.Astaroth, references.Nosfentor} {
		gs, _ := newBootstrapSaveTestState(t)
		gs.ShadowlordState.Reset()
		gs.ShadowlordState.TownAssignments[lord] = references.Britain
		require.False(t, gs.ShadowlordState.HasAutomaticPresence())

		// Any entry row except the jail row (Y=4) applies presence.
		result := gs.TransitionToLocation(
			references.Britain, references.MainFloor, references.Position{X: 15, Y: 10}, TransitionFullLocation, false,
		)
		assert.Equal(t, []references.Shadowlord{lord}, result.ShadowlordsPresent)
		assert.Equal(t, lord, gs.ShadowlordState.AutomaticPresence)
	}
}

// TestShadowlordTownEntry_NoPresenceWithoutShadowlord validates that entering
// a town without a Shadowlord does NOT trigger automatic presence.
func TestShadowlordTownEntry_NoPresenceWithoutShadowlord(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)

	gs.ShadowlordState.TownAssignments = [references.ShadowlordCount]references.Location{}

	result := gs.TransitionToLocation(
		references.Britain,
		references.MainFloor,
		references.Position{X: 15, Y: 10},
		TransitionFullLocation,
		false,
	)
	lords := result.ShadowlordsPresent

	// Should return empty and NOT have automatic presence
	assert.Empty(t, lords, "Should return no Shadowlords when none is assigned to the town")
	assert.False(t, gs.ShadowlordState.HasAutomaticPresence(),
		"Should not have automatic presence when no Shadowlord is assigned")
}

// TestShadowlordTownEntry_NonShadowlordTown validates that non-Shadowlord
// locations (keeps, castles, dungeons) do not trigger presence check.
func TestShadowlordTownEntry_NonShadowlordTown(t *testing.T) {
	// Verify keeps are not valid Shadowlord towns
	assert.False(t, IsShadowlordTown(references.Stonegate),
		"Keeps should not be valid Shadowlord towns")
	assert.False(t, IsShadowlordTown(references.LordBritishsCastle),
		"Castles should not be valid Shadowlord towns")

	// Verify valid Shadowlord towns
	assert.True(t, IsShadowlordTown(references.Britain),
		"Britain should be a valid Shadowlord town")
	assert.True(t, IsShadowlordTown(references.Moonglow),
		"Moonglow should be a valid Shadowlord town")
	assert.True(t, IsShadowlordTown(references.NewMagincia),
		"NewMagincia should be a valid Shadowlord town")
}

// TestShadowlordSpawnPosition_MatchesOriginal checks the eight
// reference town spawn positions. ref:SHADOW-0002
func TestShadowlordSpawnPosition_MatchesOriginal(t *testing.T) {
	// Expected positions from original shadow_ypos[8]
	// Towns 1-8: Moonglow=1, Britain=2, Jhelom=3, Yew=4, Minoc=5, Trinsic=6, SkaraBrae=7, NewMagincia=8
	expectedPositions := map[references.Location]references.Position{
		references.Moonglow:    {X: 15, Y: 4},  // shadow_ypos[0] = 4
		references.Britain:     {X: 15, Y: 9},  // shadow_ypos[1] = 9
		references.Jhelom:      {X: 15, Y: 15}, // shadow_ypos[2] = 15
		references.Yew:         {X: 15, Y: 8},  // shadow_ypos[3] = 8
		references.Minoc:       {X: 15, Y: 17}, // shadow_ypos[4] = 17
		references.Trinsic:     {X: 15, Y: 10}, // shadow_ypos[5] = 10
		references.SkaraBrae:   {X: 15, Y: 11}, // shadow_ypos[6] = 11
		references.NewMagincia: {X: 15, Y: 10}, // shadow_ypos[7] = 10
	}

	for location, expectedPos := range expectedPositions {
		actualPos := GetShadowlordSpawnPosition(location)
		assert.Equal(t, expectedPos.X, actualPos.X,
			"X position for %s should match", location.String())
		assert.Equal(t, expectedPos.Y, actualPos.Y,
			"Y position for %s should match (shadow_ypos)", location.String())
	}
}

// =============================================================================
// JAIL ENTRY BYPASS TESTS - Verifying Y=4 jail entry behavior
// =============================================================================

// TestShadowlordTownEntry_JailEntrySkipsPresence verifies that entry
// at player Y=4 skips automatic presence effects. ref:SHADOW-0002
func TestShadowlordTownEntry_JailEntrySkipsPresence(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)
	gs.ShadowlordState.Reset()

	gs.ShadowlordState.TownAssignments[references.Faulinei] = references.Britain

	// Simulate jail entry: player at Y=4
	// Verify no automatic presence initially
	assert.False(t, gs.ShadowlordState.HasAutomaticPresence(),
		"Should not have automatic presence before town entry check")

	result := gs.TransitionToLocation(
		references.Britain,
		references.MainFloor,
		references.Position{X: 15, Y: 4},
		TransitionFullLocation,
		false,
	)
	lords := result.ShadowlordsPresent

	// Should return empty and NOT have automatic presence because player entered via jail
	assert.Empty(t, lords, "Should return no Shadowlords when entering via jail (Y=4)")
	assert.False(t, gs.ShadowlordState.HasAutomaticPresence(),
		"Should NOT have automatic presence when entering via jail (Y=4)")
}

// TestShadowlordTownEntry_JailEntryVariousYPositions validates that ONLY Y=4
// triggers the jail entry skip behavior.
func TestShadowlordTownEntry_JailEntryVariousYPositions(t *testing.T) {
	config := helpers.LoadTestConfiguration(t)
	gameRefs := helpers.LoadTestGameReferences(t, config)
	saveData := helpers.LoadBootstrapSaveData(t)
	xTiles, yTiles := helpers.GetTestGameConstants()

	gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)

	// Test Y positions that should NOT skip presence (normal entry)
	normalEntryYPositions := []references.Coordinate{0, 1, 2, 3, 5, 6, 10, 15, 20}

	for _, yPos := range normalEntryYPositions {
		gs := mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)
		mockCallbacks := NewMockSystemCallbacks(t)
		gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()
		gs.ShadowlordState.Reset()

		gs.ShadowlordState.TownAssignments[references.Faulinei] = references.Britain

		result := gs.TransitionToLocation(
			references.Britain,
			references.MainFloor,
			references.Position{X: 15, Y: yPos},
			TransitionFullLocation,
			false,
		)
		lords := result.ShadowlordsPresent

		// Should have presence for normal Y positions
		assert.NotEmpty(t, lords, "Should return Shadowlord at Y=%d (not jail entry)", yPos)
		assert.True(t, gs.ShadowlordState.HasAutomaticPresence(),
			"Should have automatic presence at Y=%d (not jail entry)", yPos)
	}

	// Test Y=4 which should skip presence (jail entry)
	gs = mustNewGameStateFromOriginalSaveBytes(saveData, config, gameRefs, xTiles, yTiles)
	mockCallbacks := NewMockSystemCallbacks(t)
	gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()
	gs.ShadowlordState.Reset()

	gs.ShadowlordState.TownAssignments[references.Faulinei] = references.Britain
	result := gs.TransitionToLocation(
		references.Britain,
		references.MainFloor,
		references.Position{X: 15, Y: 4},
		TransitionFullLocation,
		false,
	)
	lords := result.ShadowlordsPresent

	assert.Empty(t, lords, "Should return no Shadowlords at Y=4 (jail entry)")
	assert.False(t, gs.ShadowlordState.HasAutomaticPresence(),
		"Should NOT have automatic presence at Y=4 (jail entry)")
}

// TestStonegatePresence_AllThreeShadowlordsReturned tests the unique Stonegate behavior
// where ALL THREE living Shadowlords are returned on entry.
// See ref:SHADOW-0002.
func TestStonegatePresence_AllThreeShadowlordsReturned(t *testing.T) {
	gs, _ := newBootstrapSaveTestState(t)
	gs.ShadowlordState.Reset()

	// All Shadowlords should be alive initially
	require.True(t, gs.ShadowlordState.IsAlive(references.Faulinei), "Faulinei should be alive")
	require.True(t, gs.ShadowlordState.IsAlive(references.Astaroth), "Astaroth should be alive")
	require.True(t, gs.ShadowlordState.IsAlive(references.Nosfentor), "Nosfentor should be alive")

	// Arbitrary position for Stonegate
	stonegatePosn := references.Position{X: 15, Y: 15}

	t.Run("AllThreeLivingReturnedOnEntry", func(t *testing.T) {
		// Collect Shadowlord presence data for Stonegate
		lords := gs.collectShadowlordPresenceData(references.Stonegate, stonegatePosn)

		// Should return all 3 living Shadowlords
		require.Len(t, lords, 3, "All 3 living Shadowlords should be returned for Stonegate")

		// Verify all 3 are present
		lordSet := make(map[references.Shadowlord]bool)
		for _, lord := range lords {
			lordSet[lord] = true
		}

		assert.True(t, lordSet[references.Faulinei], "Faulinei should be present")
		assert.True(t, lordSet[references.Astaroth], "Astaroth should be present")
		assert.True(t, lordSet[references.Nosfentor], "Nosfentor should be present")
	})

	t.Run("DeadShadowlordsNotReturned", func(t *testing.T) {
		// Kill Astaroth
		gs.ShadowlordState.Kill(references.Astaroth)
		require.False(t, gs.ShadowlordState.IsAlive(references.Astaroth), "Astaroth should be dead")

		// Collect Shadowlord presence data for Stonegate
		lords := gs.collectShadowlordPresenceData(references.Stonegate, stonegatePosn)

		// Should return only 2 living Shadowlords
		require.Len(t, lords, 2, "Only 2 living Shadowlords should be returned")

		// Verify Astaroth is NOT present
		for _, lord := range lords {
			assert.NotEqual(t, references.Astaroth, lord,
				"Dead Astaroth should not be returned")
		}
	})

	t.Run("AllDeadEmptyResult", func(t *testing.T) {
		// Kill remaining Shadowlords
		gs.ShadowlordState.Kill(references.Faulinei)
		gs.ShadowlordState.Kill(references.Nosfentor)

		// Collect Shadowlord presence data for Stonegate
		lords := gs.collectShadowlordPresenceData(references.Stonegate, stonegatePosn)

		// Should return empty (no living Shadowlords)
		assert.Empty(t, lords, "No Shadowlords should be returned if all are dead")
	})

	t.Run("NonStonegateLocationNoEffect", func(t *testing.T) {
		// Reset Shadowlord states
		gs.ShadowlordState.Reset()

		// Stonegate locations should use IsStonegateLocation
		assert.True(t, references.IsStonegateLocation(references.Stonegate),
			"Stonegate should be identified as Stonegate location")
		assert.False(t, references.IsStonegateLocation(references.Britain),
			"Britain should NOT be identified as Stonegate location")
	})
}
