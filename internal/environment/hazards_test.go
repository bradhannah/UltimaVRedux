package environment

import (
	"testing"

	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// MockSystemCallbacks for testing
type MockSystemCallbacks struct {
	Messages      []string
	ScreenFlashes []ScreenFlashCall
}

type ScreenFlashCall struct {
	R, G, B    float64
	Intensity  float64
	DurationMs int
}

type scriptedHazardRNG struct {
	values []int
	calls  int
	bounds []int
}

func (r *scriptedHazardRNG) Intn(n int) int {
	if r.calls >= len(r.values) {
		panic("scripted hazard RNG exhausted")
	}
	r.bounds = append(r.bounds, n)
	value := r.values[r.calls]
	r.calls++

	return value
}

func (m *MockSystemCallbacks) AddRowStr(message string) {
	m.Messages = append(m.Messages, message)
}

func (m *MockSystemCallbacks) Reset() {
	m.Messages = m.Messages[:0]
	m.ScreenFlashes = m.ScreenFlashes[:0]
}

func createTestPartyWithStats(stats []TestCharacterStats) *party_state.PartyState {
	// Start from a blank roster, then set the character stats.
	party := party_state.NewBlankPartyState()

	// Now set the character stats
	for i, stat := range stats {
		if i >= party_state.MaxActiveParty {
			break // Don't exceed array size
		}
		char := party.GetCharacter(i)
		char.Dexterity = byte(stat.Dexterity)
		char.SetMaxHp(uint16(stat.HitPoints)) // Set max HP first
		char.SetHp(uint16(stat.HitPoints))    // Then set current HP (will be clamped to max)
		char.SetStatus(stat.Status)
		char.PartyMap = party_state.PartyMapActive
	}
	return party
}

type TestCharacterStats struct {
	Dexterity int
	HitPoints int
	Status    party_state.CharacterStatus
}

func TestHazardDetection(t *testing.T) {
	rng := beradrng.New(12345)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	testCases := []struct {
		name         string
		tileIndex    indexes.SpriteIndex
		location     references.GeneralMapType
		expectedType HazardType
	}{
		{"Swamp tile", indexes.Swamp, references.LargeMapType, PoisonSwamp},
		{"Swamp in small map", indexes.Swamp, references.SmallMapType, PoisonSwamp},
		{"Lava tile", indexes.Lava, references.LargeMapType, LavaBurn},
		{"Fireplace in small map", indexes.Fireplace, references.SmallMapType, FireplaceBurn},
		{"Fireplace on large map", indexes.Fireplace, references.LargeMapType, NoHazard},
		{"Regular floor", indexes.BrickFloor, references.SmallMapType, NoHazard},
		{"Nil tile", 0, references.LargeMapType, NoHazard},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			var tile *references.Tile
			if tc.tileIndex != 0 {
				tile = &references.Tile{Index: tc.tileIndex}
			}

			hazardType := hazards.detectHazardType(tile, tc.location)
			assert.Equal(t, tc.expectedType, hazardType)
		})
	}
}

func TestSwampPoisonWithLowDexterity(t *testing.T) {
	// Setup deterministic RNG
	rng := beradrng.New(12345)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	// Create party with low dexterity character
	party := createTestPartyWithStats([]TestCharacterStats{
		{Dexterity: 5, HitPoints: 20, Status: party_state.Good},
		{Dexterity: 25, HitPoints: 20, Status: party_state.Good},
	})

	swampTile := &references.Tile{Index: indexes.Swamp}
	result := hazards.CheckTileHazards(swampTile, party, references.LargeMapType)

	// Verify poison mechanics
	assert.Equal(t, PoisonSwamp, result.Type)
	assert.True(t, len(result.Affected) > 0, "Low dex character should be poisoned")
	assert.True(t, result.MessageShown)
	assert.Contains(t, mockCallbacks.Messages, "Poisoned!")

	// Verify poisoned character status
	poisonedFound := false
	for i := 0; i < party_state.MaxActiveParty; i++ {
		member := party.GetCharacter(i)
		if member.IsPoisoned() {
			poisonedFound = true
			break
		}
	}
	assert.True(t, poisonedFound, "At least one character should be poisoned")
}

func TestSwampPoisonWithHighDexterity(t *testing.T) {
	// Setup deterministic RNG
	rng := beradrng.New(99999)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	// Create party with high dexterity characters
	party := createTestPartyWithStats([]TestCharacterStats{
		{Dexterity: 30, HitPoints: 20, Status: party_state.Good},
		{Dexterity: 30, HitPoints: 20, Status: party_state.Good},
	})

	swampTile := &references.Tile{Index: indexes.Swamp}
	result := hazards.CheckTileHazards(swampTile, party, references.LargeMapType)

	// High dex should have lower chance of poison (though not guaranteed)
	assert.Equal(t, PoisonSwamp, result.Type)
}

func TestSwampSkipsDeadAndPoisonedCharacters(t *testing.T) {
	rng := beradrng.New(12345)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	// Create party with dead and already poisoned characters
	party := createTestPartyWithStats([]TestCharacterStats{
		{Dexterity: 1, HitPoints: 0, Status: party_state.Dead},
		{Dexterity: 1, HitPoints: 20, Status: party_state.Poisoned},
		{Dexterity: 5, HitPoints: 20, Status: party_state.Good},
	})

	swampTile := &references.Tile{Index: indexes.Swamp}
	result := hazards.CheckTileHazards(swampTile, party, references.LargeMapType)

	// Only living, non-poisoned characters should be affected
	assert.Equal(t, PoisonSwamp, result.Type)

	// Verify dead and poisoned characters were skipped
	assert.Equal(t, party_state.Dead, party.GetCharacter(0).GetStatus(), "Dead character should remain dead")
	assert.Equal(t, party_state.Poisoned, party.GetCharacter(1).GetStatus(), "Poisoned character should remain poisoned")
}

func TestSwampPoisonUsesContextSpecificUniformEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		location   references.GeneralMapType
		actualRoll int
		sourceRoll int
		dexterity  int
		poisoned   bool
	}{
		{name: "town roll zero", location: references.SmallMapType, actualRoll: 0, sourceRoll: 0, dexterity: 10},
		{name: "town roll equals DEX", location: references.SmallMapType, actualRoll: 10, sourceRoll: 10, dexterity: 10},
		{name: "town roll DEX plus one", location: references.SmallMapType, actualRoll: 11, sourceRoll: 11, dexterity: 10, poisoned: true},
		{name: "town roll 29", location: references.SmallMapType, actualRoll: 29, sourceRoll: 29, dexterity: 10, poisoned: true},
		{name: "world roll one", location: references.LargeMapType, actualRoll: 1, sourceRoll: 0, dexterity: 10},
		{name: "world roll equals DEX", location: references.LargeMapType, actualRoll: 10, sourceRoll: 9, dexterity: 10},
		{name: "world roll DEX plus one", location: references.LargeMapType, actualRoll: 11, sourceRoll: 10, dexterity: 10, poisoned: true},
		{name: "world roll 30", location: references.LargeMapType, actualRoll: 30, sourceRoll: 29, dexterity: 10, poisoned: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rng := &scriptedHazardRNG{values: []int{test.sourceRoll}}
			hazards := NewEnvironmentalHazards(rng, &MockSystemCallbacks{})
			party := createTestPartyWithStats(nil)
			for index := range party.GetCharacterCount() {
				party.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
			}
			character := party.GetCharacter(0)
			character.PartyMap = party_state.PartyMapActive
			character.SetStatus(party_state.Good)
			character.Dexterity = byte(test.dexterity)

			result := hazards.ResolveTileHazards(&references.Tile{Index: indexes.Swamp}, party, test.location)

			assert.Equal(t, test.poisoned, character.IsPoisoned(), "actual roll %d", test.actualRoll)
			assert.Equal(t, 1, rng.calls)
			assert.Equal(t, []int{30}, rng.bounds)
			if test.poisoned {
				assert.Equal(t, []int{0}, result.Affected)
			} else {
				assert.Empty(t, result.Affected)
			}
		})
	}
}

func TestSwampPoisonIneligibleCharactersConsumeNoRNG(t *testing.T) {
	tests := []struct {
		name   string
		status party_state.CharacterStatus
		active bool
	}{
		{name: "dead", status: party_state.Dead, active: true},
		{name: "already poisoned", status: party_state.Poisoned, active: true},
		{name: "inactive", status: party_state.Good},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			rng := &scriptedHazardRNG{}
			hazards := NewEnvironmentalHazards(rng, &MockSystemCallbacks{})
			party := createTestPartyWithStats(nil)
			for index := range party.GetCharacterCount() {
				party.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
			}
			character := party.GetCharacter(0)
			character.SetStatus(test.status)
			if test.active {
				character.PartyMap = party_state.PartyMapActive
			}

			result := hazards.ResolveTileHazards(
				&references.Tile{Index: indexes.Swamp}, party, references.LargeMapType,
			)

			assert.Empty(t, result.Affected)
			assert.Zero(t, rng.calls)
		})
	}
}

func TestLavaDamageApplication(t *testing.T) {
	// Test lava damage with known RNG seed
	rng := beradrng.New(54321)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	party := createTestPartyWithStats([]TestCharacterStats{
		{HitPoints: 20, Status: party_state.Good},
		{HitPoints: 15, Status: party_state.Good},
	})
	initialHP := []int{int(party.GetCharacter(0).GetCurrentHp()), int(party.GetCharacter(1).GetCurrentHp())}

	lavaTile := &references.Tile{Index: indexes.Lava}
	result := hazards.CheckTileHazards(lavaTile, party, references.LargeMapType)

	// Verify damage application
	assert.Equal(t, LavaBurn, result.Type)
	assert.True(t, result.DamageDealt > 0)
	assert.Contains(t, mockCallbacks.Messages, "Burning!")

	// Verify actual HP reduction
	for i := 0; i < 2; i++ { // Only check first 2 characters we set up
		character := party.GetCharacter(i)
		if character.PartyMap == party_state.PartyMapActive {
			t.Logf("Character %d: initialHP=%d, currentHP=%d, IsDead=%v, PartyStatus=%d",
				i, initialHP[i], character.GetCurrentHp(), character.IsDead(), character.PartyMap)
			assert.True(t, int(character.GetCurrentHp()) < initialHP[i], "Member %d should have taken damage", i)
			assert.True(t, int(character.GetCurrentHp()) >= 0, "HP should not go below 0")
		}
	}
}

func TestFireplaceDamageOnlyInSmallMaps(t *testing.T) {
	rng := beradrng.New(12345)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	party := createTestPartyWithStats([]TestCharacterStats{
		{HitPoints: 20, Status: party_state.Good},
	})

	fireplaceTile := &references.Tile{Index: indexes.Fireplace}

	// Test in small map (should cause damage)
	mockCallbacks.Reset()
	result := hazards.CheckTileHazards(fireplaceTile, party, references.SmallMapType)
	assert.Equal(t, FireplaceBurn, result.Type)
	assert.Contains(t, mockCallbacks.Messages, "Burning!")

	// Test on large map (should be safe)
	mockCallbacks.Reset()
	result = hazards.CheckTileHazards(fireplaceTile, party, references.LargeMapType)
	assert.Equal(t, NoHazard, result.Type)
	assert.Empty(t, mockCallbacks.Messages)
}

func TestDeadCharacterFromDamage(t *testing.T) {
	rng := beradrng.New(12345)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	// Create character with very low HP
	party := createTestPartyWithStats([]TestCharacterStats{
		{HitPoints: 2, Status: party_state.Good}, // Very low HP, likely to die
	})

	lavaTile := &references.Tile{Index: indexes.Lava}
	result := hazards.CheckTileHazards(lavaTile, party, references.LargeMapType)

	// Verify damage was applied
	assert.Equal(t, LavaBurn, result.Type)
	assert.True(t, result.DamageDealt > 0)

	// Character should be dead if HP dropped to 0
	if party.GetCharacter(0).GetCurrentHp() == 0 {
		assert.Equal(t, party_state.Dead, party.GetCharacter(0).GetStatus())
	}
}

func TestNoHazardForSafeTiles(t *testing.T) {
	rng := beradrng.New(12345)
	mockCallbacks := &MockSystemCallbacks{}
	hazards := NewEnvironmentalHazards(rng, mockCallbacks)

	party := createTestPartyWithStats([]TestCharacterStats{
		{HitPoints: 20, Status: party_state.Good},
	})

	safeTile := &references.Tile{Index: indexes.BrickFloor}
	result := hazards.CheckTileHazards(safeTile, party, references.SmallMapType)

	assert.Equal(t, NoHazard, result.Type)
	assert.Empty(t, result.Affected)
	assert.False(t, result.MessageShown)
	assert.Equal(t, 0, result.DamageDealt)
	assert.Empty(t, mockCallbacks.Messages)
}
