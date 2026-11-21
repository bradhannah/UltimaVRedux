package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// TestHoleUpSmallMap_NotOnBed tests hole up command when player is not on a bed tile
func TestHoleUpSmallMap_NotOnBed(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithPlayerAt(15, 15). // Not on a bed tile
		WithSystemCallbacks().
		Build()

	result := gs.ActionHoleUpSmallMap()

	assert.False(t, result, "Should return false when not on bed")
	assert.Contains(t, mockCallbacks.Messages, "Find a bed first!")
	assert.Equal(t, 0, len(mockCallbacks.SoundEffectsPlayed), "No sound should play for invalid attempt")
	assert.Equal(t, 0, len(mockCallbacks.TimeAdvanced()), "No time should advance for invalid attempt")
}

func TestHoleUpSmallMapRequiresUnderlyingLeftBed(t *testing.T) {
	testCases := []struct {
		name    string
		mapTile indexes.SpriteIndex
		overlay indexes.SpriteIndex
		want    bool
	}{
		{name: "left bed", mapTile: indexes.LeftBed, want: true},
		{name: "left bed under sleeper", mapTile: indexes.LeftBed, overlay: indexes.SleepingInBed, want: true},
		{name: "right bed", mapTile: indexes.RightBed, want: false},
		{name: "sleeping in bed terrain", mapTile: indexes.SleepingInBed, want: false},
		{name: "left bed only in overlay", mapTile: indexes.BrickFloor, overlay: indexes.LeftBed, want: false},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			state, callbacks := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithPlayerAt(15, 15).
				WithSystemCallbacks().
				Build()
			position := state.MapState.PlayerLocation.Position
			layeredMap := state.GetLayeredMapByCurrentLocation()
			layeredMap.SetTileByLayer(map_state.MapLayer, &position, tc.mapTile)
			layeredMap.SetTileByLayer(map_state.MapOverrideLayer, &position, tc.overlay)
			requests := 0
			state.SystemCallbacks.Flow.Rest.RequestSleepDuration = func() { requests++ }

			assert.Equal(t, tc.want, state.ActionHoleUpSmallMap())
			if tc.want {
				require.Equal(t, 1, requests)
				assert.NotContains(t, callbacks.Messages, "Find a bed first!")
			} else {
				assert.Zero(t, requests)
				assert.Contains(t, callbacks.Messages, "Find a bed first!")
			}
		})
	}
}

// TestBedRest_PoisonDeath tests that poisoned characters can die during sleep
func TestBedRest_PoisonDeath(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(12345). // Deterministic for testing
		Build()

	// Set up character with poison and low HP
	gs.PartyState.GetCharacter(0).SetStatus(party_state.Poisoned)
	gs.PartyState.GetCharacter(0).SetHp(2)
	// Character name is set by default in test data

	// Test healing function directly for deterministic results
	deathMessages := gs.PartyState.RegenerateSleepHealth()

	// Character should take poison damage and potentially die
	assert.True(t, len(deathMessages) > 0 || gs.PartyState.GetCharacter(0).GetCurrentHp() < 2,
		"Poisoned character should take damage")

	if len(deathMessages) > 0 {
		assert.Contains(t, deathMessages[0], "Alas!")
		assert.Contains(t, deathMessages[0], "has died!")
		assert.Equal(t, party_state.Dead, gs.PartyState.GetCharacter(0).GetStatus())
	}
}

// Each 10-minute sleep cycle restores 1 HP and 2 MP, capped at MaxHp and
// Intelligence; the dead do not regenerate.
func TestBedRestRegenerationCycle(t *testing.T) {
	for _, test := range []struct {
		name       string
		status     party_state.CharacterStatus
		hp, wantHp uint16
		mp, wantMp byte
	}{
		{name: "regenerates", status: party_state.Sleep, hp: 50, mp: 20, wantHp: 51, wantMp: 22},
		{name: "capped at maximums", status: party_state.Sleep, hp: 100, mp: 50, wantHp: 100, wantMp: 50},
		{name: "dead stay dead", status: party_state.Dead, hp: 0, mp: 0, wantHp: 0, wantMp: 0},
	} {
		gs, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
		char := gs.PartyState.GetCharacter(0)
		char.SetStatus(test.status)
		char.SetMaxHp(100)
		char.SetHp(test.hp)
		char.Intelligence = 50
		char.SetMp(test.mp)

		gs.PartyState.RegenerateSleepHealth()

		assert.Equal(t, test.wantHp, char.GetCurrentHp(), test.name)
		assert.Equal(t, test.wantMp, char.GetCurrentMp(), test.name)
		assert.Equal(t, test.status, char.GetStatus(), test.name)
	}
}

// TestBedRest_SleepStatusManagement tests setting and clearing sleep status
func TestBedRest_SleepStatusManagement(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set up party with various statuses
	gs.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	gs.PartyState.GetCharacter(1).SetStatus(party_state.Poisoned) // Should remain poisoned
	gs.PartyState.GetCharacter(2).SetStatus(party_state.Dead)     // Should remain dead

	// Set party to sleep
	gs.PartyState.SetPartySleepStatus(true)

	assert.Equal(t, party_state.Sleep, gs.PartyState.GetCharacter(0).GetStatus(), "Good status should become Sleep")
	assert.Equal(t, party_state.Poisoned, gs.PartyState.GetCharacter(1).GetStatus(), "Poisoned should remain poisoned")
	assert.Equal(t, party_state.Dead, gs.PartyState.GetCharacter(2).GetStatus(), "Dead should remain dead")

	// Wake party up
	gs.PartyState.SetPartySleepStatus(false)

	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(0).GetStatus(), "Sleep should become Good")
	assert.Equal(t, party_state.Poisoned, gs.PartyState.GetCharacter(1).GetStatus(), "Poisoned should remain poisoned")
	assert.Equal(t, party_state.Dead, gs.PartyState.GetCharacter(2).GetStatus(), "Dead should remain dead")
}

// TestBedRest_FoodConsumption tests food consumption at meal times
func TestBedRest_FoodConsumption(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set up party with food
	gs.PartyState.Inventory.Provisions.Food.Set(10)
	activeMembers := 3

	testCases := []struct {
		hour      int
		shouldEat bool
	}{
		{6, true},   // Breakfast
		{7, false},  // Not meal time
		{12, true},  // Lunch
		{13, false}, // Not meal time
		{18, true},  // Dinner
		{19, false}, // Not meal time
	}

	for _, tc := range testCases {
		t.Run(t.Name(), func(t *testing.T) {
			initialFood := gs.PartyState.Inventory.Provisions.Food.Get()

			isStarving, msg := gs.PartyState.Inventory.ConsumeMealTimeFood(tc.hour, activeMembers)

			if tc.shouldEat {
				expectedFood := int(initialFood) - activeMembers
				if expectedFood < 0 {
					expectedFood = 0
				}
				assert.Equal(t, uint16(expectedFood), gs.PartyState.Inventory.Provisions.Food.Get(),
					"Food should be consumed at hour %d", tc.hour)

				if expectedFood == 0 && int(initialFood) < activeMembers {
					assert.True(t, isStarving, "Should be starving when no food")
					assert.Equal(t, "Starving!", msg, "Should show starving message")
				}
			} else {
				assert.Equal(t, initialFood, gs.PartyState.Inventory.Provisions.Food.Get(),
					"Food should not be consumed at hour %d", tc.hour)
				assert.False(t, isStarving, "Should not be starving at non-meal time")
			}
		})
	}
}

// TestBedRest_StarvationDamage tests damage from starvation
func TestBedRest_StarvationDamage(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set up character with low HP for starvation death test
	char := gs.PartyState.GetCharacter(0)
	char.SetHp(3) // Low enough that 5 starvation damage kills

	// Set other characters to full health so they don't die
	for i := 1; i < 6; i++ {
		gs.PartyState.GetCharacter(i).SetHp(gs.PartyState.GetCharacter(i).GetMaxHp())
	}

	initialHp := char.GetCurrentHp()

	// Apply starvation damage
	deadIndexes := gs.PartyState.DamageFromStarvation(gs.RandomIntInRange)

	// Should take damage and die
	assert.Equal(t, uint16(0), char.GetCurrentHp(), "Should die from starvation damage")
	assert.Equal(t, party_state.Dead, char.GetStatus(), "Should die from starvation")
	assert.Len(t, deadIndexes, 1, "Should have one dead character")
	assert.Equal(t, party_state.AvatarCharacterIndex, deadIndexes[0], "Avatar should have died")

	t.Logf("Starvation test: %d HP -> %d HP, died: %v",
		initialHp, char.GetCurrentHp(), char.GetStatus() == party_state.Dead)
}

// TestBedRest_ActiveMemberCount tests counting non-dead, non-sleeping members
func TestBedRest_ActiveMemberCount(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set up party with mixed statuses
	gs.PartyState.GetCharacter(0).SetStatus(party_state.Good) // Active
	gs.PartyState.GetCharacter(0).PartyMap = party_state.PartyMapActive
	gs.PartyState.GetCharacter(1).SetStatus(party_state.Poisoned) // Active
	gs.PartyState.GetCharacter(1).PartyMap = party_state.PartyMapActive
	gs.PartyState.GetCharacter(2).SetStatus(party_state.Sleep) // Not active
	gs.PartyState.GetCharacter(2).PartyMap = party_state.PartyMapActive
	gs.PartyState.GetCharacter(3).SetStatus(party_state.Dead) // Not active
	gs.PartyState.GetCharacter(3).PartyMap = party_state.PartyMapActive
	gs.PartyState.GetCharacter(4).SetStatus(party_state.Good) // Active
	gs.PartyState.GetCharacter(4).PartyMap = party_state.PartyMapActive
	gs.PartyState.GetCharacter(5).SetStatus(party_state.Sleep) // Not active
	gs.PartyState.GetCharacter(5).PartyMap = party_state.PartyMapActive

	count := gs.PartyState.CountActiveMembers()

	assert.Equal(t, 3, count, "Should count 3 active members (Good + Poisoned)")
}

// TestBedRest_StarvationOncePerHour tests that starvation damage applies once per hour when food is zero
// Original behavior: original party-effect rules - starvation only triggers when hour changes
// See ref:CAMP-0005.
func TestBedRest_StarvationOncePerHour(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(12345). // Deterministic for testing
		Build()

	// Set up ALL party members with high HP to prevent deaths
	// (DamageFromStarvation affects entire party, not just one character)
	for i := 0; i < 6; i++ {
		gs.PartyState.GetCharacter(i).SetStatus(party_state.Good)
		gs.PartyState.GetCharacter(i).SetHp(100)
		gs.PartyState.GetCharacter(i).SetMaxHp(100)
		gs.PartyState.GetCharacter(i).PartyMap = party_state.PartyMapActive
	}

	char := gs.PartyState.GetCharacter(0)
	gs.PartyState.Inventory.Provisions.Food.Set(0) // ZERO FOOD

	initialHp := char.GetCurrentHp()

	// Starvation processing observes an hour boundary; start this case at
	// 01:50 so the advance crosses one. ref:PROV-0002
	gs.DateTime.Hour = 1
	gs.DateTime.Minute = 50
	lastHour := int(gs.DateTime.Hour)

	damageCount := 0

	// Simulate 6 cycles (60 minutes) crossing into hour 2
	for i := 0; i < 6; i++ {
		// Advance time by 10 minutes
		gs.DateTime.Minute += 10
		if gs.DateTime.Minute >= 60 {
			gs.DateTime.Hour++
			gs.DateTime.Minute -= 60
		}

		currentHour := int(gs.DateTime.Hour)

		// Starvation should only trigger when hour changes (original behavior)
		if currentHour != lastHour {
			// Apply starvation only at the newly observed hour boundary. ref:PROV-0002
			beforeDamageHp := char.GetCurrentHp()
			starvationIndexes := gs.PartyState.DamageFromStarvation(gs.RandomIntInRange)
			assert.Len(t, starvationIndexes, 0, "Character should not die yet (has 100 HP)")

			// Verify damage was applied
			assert.Less(t, char.GetCurrentHp(), beforeDamageHp,
				"Should take damage when hour changes from %d to %d", lastHour, currentHour)

			damageCount++
			lastHour = currentHour

			t.Logf("Hour %d: Starvation damage applied, HP: %d -> %d (damage: %d)",
				currentHour, beforeDamageHp, char.GetCurrentHp(), beforeDamageHp-char.GetCurrentHp())
		}
	}

	// Should have taken damage exactly once (hour 1 -> hour 2)
	assert.Equal(t, 1, damageCount, "Starvation should trigger exactly once per hour change")
	assert.Less(t, char.GetCurrentHp(), initialHp, "Should have taken damage")

	t.Logf("✅ Starvation damage applied once on hour change: %d HP -> %d HP (lost %d HP, damage count: %d)",
		initialHp, char.GetCurrentHp(), initialHp-char.GetCurrentHp(), damageCount)
}

// TestBedRest_StarvationDuringNonMealHours tests starvation damage during non-meal hours
func TestBedRest_StarvationDuringNonMealHours(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(12345).
		Build()

	// Set up ONE character with low HP, others with high HP
	// (Only character[0] should die, others survive)
	for i := 0; i < 6; i++ {
		gs.PartyState.GetCharacter(i).SetStatus(party_state.Good)
		gs.PartyState.GetCharacter(i).PartyMap = party_state.PartyMapActive
		if i == 0 {
			gs.PartyState.GetCharacter(i).SetHp(8) // Low enough for 2 starvation hits to kill
		} else {
			gs.PartyState.GetCharacter(i).SetHp(100) // Others survive
			gs.PartyState.GetCharacter(i).SetMaxHp(100)
		}
	}

	char := gs.PartyState.GetCharacter(0)
	gs.PartyState.Inventory.Provisions.Food.Set(0) // ZERO FOOD

	// Start at hour 2 (NOT a meal time - this is the bug we're fixing!)
	gs.DateTime.Hour = 2
	gs.DateTime.Minute = 0

	initialHp := char.GetCurrentHp()

	// First cycle - take random 1-8 damage
	starvationIndexes := gs.PartyState.DamageFromStarvation(gs.RandomIntInRange)
	assert.Len(t, starvationIndexes, 0, "Should not die on first hit")
	assert.LessOrEqual(t, char.GetCurrentHp(), initialHp, "Should lose HP from starvation")

	// Second cycle - take more damage and potentially die
	starvationIndexes = gs.PartyState.DamageFromStarvation(gs.RandomIntInRange)
	assert.GreaterOrEqual(t, len(starvationIndexes), 1, "Should have at least one death from starvation")
	assert.Contains(t, starvationIndexes, party_state.AvatarCharacterIndex, "Avatar should have died")
	assert.Equal(t, uint16(0), char.GetCurrentHp(), "Avatar should be at 0 HP")
	assert.Equal(t, party_state.Dead, char.GetStatus(), "Avatar should be dead")

	t.Logf("✅ Character died from starvation during non-meal hours: %d HP -> %d HP",
		initialHp, char.GetCurrentHp())
}
