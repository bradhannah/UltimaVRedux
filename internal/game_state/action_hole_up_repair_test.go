package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestActionHoleUpRepairShip_SailsMustBeLowered verifies repair requires
// a furled, anchored frigate. ref:SHIP-0004
func TestActionHoleUpRepairShip_SailsMustBeLowered(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	// Setup frigate with sails RAISED (cannot repair)
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		references.Position{X: 100, Y: 100},
		0,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailRaised)
	frigate.GetVehicleDetails().SetHullHP(50) // Damaged hull
	gs.PartyVehicle = *frigate

	mockCallbacks.Messages = []string{}

	// Attempt repair with sails raised
	result := gs.ActionHoleUpRepairShip()

	// Should fail - sails must be lowered
	assert.False(t, result, "Should fail repair when sails are raised")
	assert.Contains(t, mockCallbacks.Messages, "Sails must be",
		"Should show 'Sails must be' message")
	assert.Contains(t, mockCallbacks.Messages, "lowered!",
		"Should show 'lowered!' message")

	// Hull should be unchanged
	assert.Equal(t, 50, gs.PartyVehicle.GetVehicleDetails().GetHullHP(),
		"Hull should not be repaired when sails are raised")
}

// TestActionHoleUpRepairShip_SuccessfulRepair verifies that ship repair
// works correctly when sails are furled (anchored)
// See ref:SHIP-0004.
func TestActionHoleUpRepairShip_SuccessfulRepair(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	// Setup frigate with sails FURLED (can repair)
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		references.Position{X: 100, Y: 100},
		0,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50) // Damaged hull
	gs.PartyVehicle = *frigate

	mockCallbacks.Messages = []string{}

	// Set deterministic seed for reproducible results
	gs.SetRandomSeed(12345)

	// Attempt repair with sails furled
	result := gs.ActionHoleUpRepairShip()

	// Should succeed
	assert.True(t, result, "Should succeed repair when sails are furled")

	// Process event queue for repair to complete
	for gs.EventQueueManager.HasActiveWorkflows() {
		gs.ElapsedMs += 16 // Simulate ~60fps frame advancement
		gs.EventQueueManager.Update()
	}

	// Check messages after event queue completes (Repairing... is added by event queue)
	assert.Contains(t, mockCallbacks.Messages, "Repairing...",
		"Should show 'Repairing...' message")

	// Hull should be repaired (at least original + some repair)
	finalHull := gs.PartyVehicle.GetVehicleDetails().GetHullHP()
	assert.Greater(t, finalHull, 50, "Hull should be repaired")

	// Check for "Hull now XX!" message in the final messages
	foundHullMessage := false
	for _, msg := range mockCallbacks.Messages {
		if len(msg) > 9 && msg[:9] == "Hull now " {
			foundHullMessage = true
			break
		}
	}
	assert.True(t, foundHullMessage, "Should show final hull status message")
}

func TestShipRepairRunsActorPhasesBeforeOneOrdinaryHazardCycle(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50)
	gs.PartyVehicle = *frigate
	for index := range gs.PartyState.GetCharacterCount() {
		member := gs.PartyState.GetCharacter(index)
		member.PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	member := gs.PartyState.GetCharacter(0)
	member.PartyMap = party_state.PartyMapActive
	member.SetMaxHp(100)
	member.SetHp(100)
	position := gs.MapState.PlayerLocation.Position
	gs.GetCurrentLayeredMap().SetTileByLayer(0, &position, indexes.Lava)
	gs.ActiveDurationEffect = DurationEffect{
		EffectType:     DurationEffectProtection,
		TurnsRemaining: 3,
	}
	before := gs.DateTime
	hpBefore := member.GetCurrentHp()

	require.True(t, gs.ActionHoleUpRepairShip())
	for gs.EventQueueManager.HasActiveWorkflows() {
		gs.ElapsedMs += ShipRepairVisualDelayMs
		gs.EventQueueManager.Update()
	}

	afterRepair := before
	for range ShipRepairCycles {
		afterRepair.Advance(ShipRepairMinutesPerCycle)
	}
	assert.Equal(t, afterRepair, gs.DateTime)
	assert.Equal(t, hpBefore, member.GetCurrentHp(), "internal actor phases must not apply standing hazards")
	assert.Equal(t, 3, gs.ActiveDurationEffect.TurnsRemaining)

	gs.CompleteSurfaceCommandCycle(references.LargeMapType)
	afterCompletion := afterRepair
	afterCompletion.Advance(DefaultLargeMapMinutesPerTurn)
	assert.Equal(t, afterCompletion, gs.DateTime)
	assert.Less(t, member.GetCurrentHp(), hpBefore, "ordinary completion applies one standing hazard")
	assert.Equal(t, 2, gs.ActiveDurationEffect.TurnsRemaining)
	hammeringMessages := 0
	for _, message := range mockCallbacks.Messages {
		if message == "*hammering*" {
			hammeringMessages++
		}
	}
	assert.Equal(t, ShipRepairCycles, hammeringMessages)
}

func TestShipRepairRetainsPirateAttackFromEveryActorPhase(t *testing.T) {
	gs, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(98)
	gs.PartyVehicle = *frigate
	for index := range gs.PartyState.GetCharacterCount() {
		gs.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	member := gs.PartyState.GetCharacter(0)
	member.PartyMap = party_state.PartyMapActive
	member.SetMaxHp(100)
	member.SetHp(100)
	addPirateTestEnemy(t, gs, references.Position{X: 102, Y: 100}, indexes.PirateShipUp)
	before := gs.DateTime
	callbacks.Reset()

	require.True(t, gs.ActionHoleUpRepairShip())
	drainGameplayWorkflows(t, gs)
	assert.Equal(t, ShipRepairCycles, countEqual(callbacks.Messages, "* BOOOM! *"))

	gs.CompleteSurfaceCommandCycle(references.LargeMapType)
	drainGameplayWorkflows(t, gs)
	assert.Equal(t, ShipRepairCycles+1, countEqual(callbacks.Messages, "* BOOOM! *"))
	want := before
	for range ShipRepairCycles {
		want.Advance(ShipRepairMinutesPerCycle)
	}
	want.Advance(DefaultLargeMapMinutesPerTurn)
	assert.Equal(t, want, gs.DateTime)
}

func TestShipRepairPausesForAdjacentCombatBeforeRepairing(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50)
	gs.PartyVehicle = *frigate
	addPirateTestEnemy(t, gs, references.Position{X: 101, Y: 100}, indexes.PirateShipUp)
	before := gs.DateTime

	require.True(t, gs.ActionHoleUpRepairShip())
	drainGameplayWorkflows(t, gs)
	assert.NotNil(t, gs.activeShipRepair)
	assert.True(t, gs.activeShipRepair.awaitingCombat)
	assert.Equal(t, before, gs.DateTime)
	assert.Equal(t, 50, gs.PartyVehicle.GetVehicleDetails().GetHullHP())

	gs.CompleteSurfaceCommandCycle(references.LargeMapType)
	assert.Equal(t, before, gs.DateTime, "ordinary completion waits for the repair continuation")
	require.NoError(t, gs.ResolvePendingOverworldContact())
	require.NotNil(t, gs.CombatScene)
	assert.Equal(t, 50, gs.PartyVehicle.GetVehicleDetails().GetHullHP())

	gs.EndCombat(CombatVictory)
	drainGameplayWorkflows(t, gs)
	assert.Nil(t, gs.activeShipRepair)
	assert.Greater(t, gs.PartyVehicle.GetVehicleDetails().GetHullHP(), 50)
	want := before
	for range ShipRepairCycles {
		want.Advance(ShipRepairMinutesPerCycle)
	}
	want.Advance(DefaultLargeMapMinutesPerTurn)
	assert.Equal(t, want, gs.DateTime)
}

func TestShipRepairMinutesDoNotProcessMealsBeforeOrdinaryCompletion(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50)
	gs.PartyVehicle = *frigate
	for index := range gs.PartyState.GetCharacterCount() {
		gs.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapAtInn(references.Britain)
	}
	gs.PartyState.GetCharacter(0).PartyMap = party_state.PartyMapActive
	gs.PartyState.Inventory.Provisions.Food.Set(100)
	gs.DateTime.Hour, gs.DateTime.Minute = 5, 55
	gs.lastProcessedHour = 5

	require.True(t, gs.ActionHoleUpRepairShip())
	drainGameplayWorkflows(t, gs)
	assert.Equal(t, uint16(100), gs.PartyState.Inventory.Provisions.Food.Get())

	gs.CompleteSurfaceCommandCycle(references.LargeMapType)
	assert.Equal(t, uint16(100), gs.PartyState.Inventory.Provisions.Food.Get(),
		"ordinary completion overwrites a repair crossing before observing hunger")
}

func TestShipRepairAbortsCleanlyWhenDeferredContactBecomesStale(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50)
	gs.PartyVehicle = *frigate
	enemy := addPirateTestEnemy(t, gs, references.Position{X: 101, Y: 100}, indexes.PirateShipUp)
	before := gs.DateTime

	require.True(t, gs.ActionHoleUpRepairShip())
	drainGameplayWorkflows(t, gs)
	gs.CompleteSurfaceCommandCycle(references.LargeMapType)
	enemy.SetPos(references.Position{X: 110, Y: 100})
	require.NoError(t, gs.ResolvePendingOverworldContact())

	assert.Nil(t, gs.activeShipRepair)
	assert.Nil(t, gs.CombatScene)
	assert.False(t, gs.HasPendingOverworldContact())
	assert.Equal(t, 50, gs.PartyVehicle.GetVehicleDetails().GetHullHP())
	want := before
	want.Advance(DefaultLargeMapMinutesPerTurn)
	assert.Equal(t, want, gs.DateTime)
}

func TestShipRepairAbortsCleanlyWhenDeferredCombatCannotStart(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50)
	gs.PartyVehicle = *frigate
	enemy := addOverworldTestEnemy(t, gs, references.MonsterSeaHorse, references.Position{X: 101, Y: 100})
	before := gs.DateTime

	require.True(t, gs.ActionHoleUpRepairShip())
	drainGameplayWorkflows(t, gs)
	gs.CompleteSurfaceCommandCycle(references.LargeMapType)
	enemy.EnemyReference.MonsterType = references.MonsterType(255)
	require.Error(t, gs.ResolvePendingOverworldContact())

	assert.Nil(t, gs.activeShipRepair)
	assert.Nil(t, gs.CombatScene)
	assert.False(t, gs.HasPendingOverworldContact())
	assert.Equal(t, 50, gs.PartyVehicle.GetVehicleDetails().GetHullHP())
	want := before
	want.Advance(DefaultLargeMapMinutesPerTurn)
	assert.Equal(t, want, gs.DateTime)
}

func TestShipRepairShutdownClearsTransientContinuation(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	gs.PartyVehicle = *frigate
	require.True(t, gs.ActionHoleUpRepairShip())
	require.NotNil(t, gs.activeShipRepair)

	gs.EventQueueManager.Shutdown()

	assert.Nil(t, gs.activeShipRepair)
}

func TestShipRepairShutdownClearsContinuationAfterWorkflowReleasesForCombat(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		gs.MapState.PlayerLocation.Position,
		gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	gs.PartyVehicle = *frigate
	addPirateTestEnemy(t, gs, references.Position{X: 101, Y: 100}, indexes.PirateShipUp)
	require.True(t, gs.ActionHoleUpRepairShip())
	drainGameplayWorkflows(t, gs)
	require.NotNil(t, gs.activeShipRepair)
	require.True(t, gs.activeShipRepair.awaitingCombat)

	gs.EventQueueManager.Shutdown()

	assert.Nil(t, gs.activeShipRepair)
}

func drainGameplayWorkflows(t *testing.T, state *GameState) {
	t.Helper()
	for update := 0; update < 200 && state.EventQueueManager.HasActiveWorkflows(); update++ {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
	}
	require.False(t, state.EventQueueManager.HasActiveWorkflows())
}

// TestActionHoleUpRepairShip_MinimumHullTen verifies repair repeats
// until hull reaches at least ten. ref:SHIP-0004
func TestActionHoleUpRepairShip_MinimumHullTen(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	// Setup frigate with very low hull (below minimum threshold)
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		references.Position{X: 100, Y: 100},
		0,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(5) // Very low hull
	gs.PartyVehicle = *frigate

	gs.SetRandomSeed(12345)

	// Start repair
	gs.ActionHoleUpRepairShip()

	// Process event queue
	for gs.EventQueueManager.HasActiveWorkflows() {
		gs.ElapsedMs += 16
		gs.EventQueueManager.Update()
	}

	// Hull should be at least ShipRepairMinHull (10)
	finalHull := gs.PartyVehicle.GetVehicleDetails().GetHullHP()
	assert.GreaterOrEqual(t, finalHull, ShipRepairMinHull,
		"Hull should be at least %d after repair", ShipRepairMinHull)
}

// TestApplyShipRepair_MaxHull99 verifies hull saturation at 99 without
// skipping the initial repair draw. ref:SHIP-0004
func TestApplyShipRepair_MaxHull99(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	// Setup frigate with hull already at max
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		references.Position{X: 100, Y: 100},
		0,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(ShipRepairMaxHull) // Already at max
	gs.PartyVehicle = *frigate

	gs.SetRandomSeed(12345)
	rngCallsBefore := gs.GetRNG().Calls()

	gs.applyShipRepair()

	// Hull should not exceed ShipRepairMaxHull (99)
	finalHull := gs.PartyVehicle.GetVehicleDetails().GetHullHP()
	assert.LessOrEqual(t, finalHull, ShipRepairMaxHull,
		"Hull should not exceed %d", ShipRepairMaxHull)
	assert.Equal(t, rngCallsBefore+1, gs.GetRNG().Calls(), "original do-while consumes one repair roll at max hull")
}

// TestActionHoleUpLargeMap_FrigateRoutesToRepair verifies that hole up on
// frigate routes to ship repair instead of camping
func TestActionHoleUpLargeMap_FrigateRoutesToRepair(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	// Setup frigate with sails furled
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle,
		references.Position{X: 100, Y: 100},
		0,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailFurled)
	frigate.GetVehicleDetails().SetHullHP(50)
	gs.PartyVehicle = *frigate

	mockCallbacks.Messages = []string{}
	gs.SetRandomSeed(12345)

	// Call hole up on large map
	result := gs.ActionHoleUpLargeMap()

	// Should succeed and route to ship repair
	assert.True(t, result, "Hole up on frigate should succeed")
	assert.Equal(t, []string{"Hole up & repair..."}, mockCallbacks.CommandPrompts,
		"the echo names ship repair, not camping")
}

// TestActionHoleUpLargeMap_SkiffDoesNotRepair verifies that skiffs don't
// trigger ship repair (original: bitmask check excludes skiffs)
func TestActionHoleUpLargeMap_SkiffDoesNotRepair(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithPlayerAt(100, 100).
		WithSystemCallbacks().
		Build()

	// Setup skiff (NOT a frigate)
	skiff := map_units.NewNPCFriendlyVehiceNewRef(
		references.SkiffVehicle,
		references.Position{X: 100, Y: 100},
		0,
	)
	gs.PartyVehicle = *skiff

	mockCallbacks.Messages = []string{}

	// Call hole up on large map with skiff
	gs.ActionHoleUpLargeMap()

	// Should show "On foot!" (camping requires being on foot, skiff doesn't get repair)
	// Original Ultima V: ship repair bitmask (plrshp & 0xf8) == SHIP excludes skiffs (0x28)
	assert.Contains(t, mockCallbacks.Messages, "On foot!",
		"Skiff should require on foot for camping, not get ship repair")
}

// TestIsOnFrigate verifies the frigate detection helper
func TestIsOnFrigate(t *testing.T) {
	tests := []struct {
		name        string
		vehicleType references.VehicleType
		expected    bool
	}{
		{"Frigate", references.FrigateVehicle, true},
		{"Skiff", references.SkiffVehicle, false},
		{"Horse", references.HorseVehicle, false},
		{"Carpet", references.CarpetVehicle, false},
		{"NoVehicle", references.NoPartyVehicle, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithPlayerAt(100, 100).
				WithSystemCallbacks().
				Build()

			if tt.vehicleType != references.NoPartyVehicle {
				vehicle := map_units.NewNPCFriendlyVehiceNewRef(
					tt.vehicleType,
					references.Position{X: 100, Y: 100},
					0,
				)
				gs.PartyVehicle = *vehicle
			}

			result := gs.isOnFrigate()
			assert.Equal(t, tt.expected, result,
				"isOnFrigate() should return %v for %s", tt.expected, tt.name)
		})
	}
}

// TestIsSailFurled verifies the sail state helper
func TestIsSailFurled(t *testing.T) {
	t.Run("SailFurled returns true", func(t *testing.T) {
		details := map_units.NewVehicleDetails(references.FrigateVehicle)
		details.SetSailState(map_units.SailFurled)
		require.True(t, details.IsSailFurled(), "IsSailFurled should return true when furled")
	})

	t.Run("SailRaised returns false", func(t *testing.T) {
		details := map_units.NewVehicleDetails(references.FrigateVehicle)
		details.SetSailState(map_units.SailRaised)
		require.False(t, details.IsSailFurled(), "IsSailFurled should return false when raised")
	})
}
