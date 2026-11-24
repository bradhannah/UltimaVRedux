package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestCommitDeathResurrection_XPPenaltyUsesPreDeathKarma(t *testing.T) {
	tests := []struct {
		name          string
		initialKarma  int
		initialXP     uint16
		expectedXP    uint16 // XP after penalty
		expectedKarma int    // Karma after resurrection (minimum 75)
	}{
		{
			name:          "Karma 99 preserves XP",
			initialKarma:  99,
			initialXP:     1000,
			expectedXP:    1000,
			expectedKarma: 99,
		},
		{
			name:          "Karma 98 preserves XP",
			initialKarma:  98,
			initialXP:     1001,
			expectedXP:    1001,
			expectedKarma: 98,
		},
		{
			name:          "Karma 97 scales XP",
			initialKarma:  97,
			initialXP:     1001,
			expectedXP:    970,
			expectedKarma: 97,
		},
		{
			name:          "Medium karma (75) - lose 25% XP",
			initialKarma:  75,
			initialXP:     1000,
			expectedXP:    750, // 1000 * 75 / 100 = 750
			expectedKarma: 75,
		},
		{
			name:          "Low karma (50) - lose 50% XP",
			initialKarma:  50,
			initialXP:     1000,
			expectedXP:    500, // 1000 * 50 / 100 = 500
			expectedKarma: 75,  // Raised to minimum 75
		},
		{
			name:          "Very low karma (25) - lose 75% XP, karma raised",
			initialKarma:  25,
			initialXP:     1000,
			expectedXP:    250, // 1000 * 25 / 100 = 250
			expectedKarma: 75,  // Raised to minimum 75
		},
		{
			name:          "Zero XP - stays zero",
			initialKarma:  50,
			initialXP:     0,
			expectedXP:    0, // 0 * 50 / 100 = 0
			expectedKarma: 75,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := createTestGameStateForDeath(t, tc.initialKarma, tc.initialXP)

			outcome := commitTestDeathResurrection(t, gs)
			assert.Equal(t, tc.initialKarma, outcome.KarmaAtDeath)

			// Verify XP penalty was applied
			avatar := gs.PartyState.GetAvatar()
			assert.Equal(t, tc.expectedXP, avatar.Exp, "XP penalty calculation incorrect")

			// Verify karma was raised if below minimum
			assert.Equal(t, tc.expectedKarma, gs.PartyState.Karma.Value, "Karma minimum not enforced")

			// Verify character was resurrected
			assert.Equal(t, party_state.Good, avatar.GetStatus(), "Character should be resurrected to Good status")
			assert.Equal(t, avatar.GetMaxHp(), avatar.GetCurrentHp(), "HP should be restored to max")
		})
	}
}

func TestCommitDeathResurrection_FoodMinimum(t *testing.T) {
	tests := []struct {
		name         string
		initialFood  uint16
		expectedFood uint16
	}{
		{
			name:         "Nonzero food below 63 - unchanged",
			initialFood:  10,
			expectedFood: 10,
		},
		{
			name:         "Food at minimum - unchanged",
			initialFood:  63,
			expectedFood: 63,
		},
		{
			name:         "Food above minimum - unchanged",
			initialFood:  100,
			expectedFood: 100,
		},
		{
			name:         "Zero food - raised to 63",
			initialFood:  0,
			expectedFood: MinimumFoodAfterDeath,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := createTestGameStateForDeath(t, 75, 100)
			gs.PartyState.Inventory.Provisions.Food.Set(tc.initialFood)

			commitTestDeathResurrection(t, gs)

			actualFood := gs.PartyState.Inventory.Provisions.Food.Get()
			assert.Equal(t, tc.expectedFood, actualFood, "Party recovery food restoration mismatch (ref:RECOVERY-0002)")
		})
	}
}

// TestGetResurrectionLocation tests resurrection location constants
func TestGetResurrectionLocation(t *testing.T) {
	gs := createTestGameStateForDeath(t, 75, 100)

	// Verify resurrection location is Castle Britannia
	assert.Equal(t, references.LordBritishsCastle, gs.GetResurrectionLocation(),
		"Resurrection should be at Castle Britannia")

	// Verify position is (10, 10)
	pos := gs.GetResurrectionPosition()
	assert.Equal(t, references.Coordinate(ResurrectionLocationX), pos.X, "X position should be 10")
	assert.Equal(t, references.Coordinate(ResurrectionLocationY), pos.Y, "Y position should be 10")

	// Verify floor is 1
	assert.Equal(t, references.FloorNumber(ResurrectionFloor), gs.GetResurrectionFloor(),
		"Resurrection floor should be 1")
}

func TestCommitDeathResurrection_UpdatesEveryActiveMemberAndPreservesInactiveRecords(t *testing.T) {
	gs := createTestGameStateForDeath(t, 50, 100)
	first := gs.PartyState.GetCharacter(0)
	second := gs.PartyState.GetCharacter(1)
	inactive := gs.PartyState.GetCharacter(2)
	for index := range party_state.MaxActiveParty {
		character := gs.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapUnrecruited
	}
	first.PartyMap = party_state.PartyMapActive
	first.Exp = 1001
	first.SetStatus(party_state.Dead)
	first.SetHp(0)
	second.PartyMap = party_state.PartyMapActive
	second.Exp = 2003
	second.SetStatus(party_state.Dead)
	second.SetHp(0)
	inactive.Exp = 3007
	inactive.Level = 7
	inactive.SetMaxHp(210)
	inactive.SetHp(17)
	inactive.SetStatus(party_state.Poisoned)

	outcome := commitTestDeathResurrection(t, gs)

	require.Len(t, outcome.Members, 2)
	assert.Equal(t, uint16(500), first.Exp)
	assert.Equal(t, uint16(1001), second.Exp)
	for _, character := range []*party_state.PlayerCharacter{first, second} {
		assert.Equal(t, party_state.Good, character.GetStatus())
		assert.Equal(t, character.GetMaxHp(), character.GetCurrentHp())
	}
	assert.Equal(t, uint16(3007), inactive.Exp)
	assert.Equal(t, byte(7), inactive.Level)
	assert.Equal(t, uint16(210), inactive.GetMaxHp())
	assert.Equal(t, uint16(17), inactive.GetCurrentHp())
	assert.Equal(t, party_state.Poisoned, inactive.GetStatus())
}

func TestCommitDeathResurrection_RestoresClassMPInAuthoritativeAndDetachedOutcomes(t *testing.T) {
	gs := createTestGameStateForDeath(t, 75, 100)
	for index := range party_state.MaxActiveParty {
		gs.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}
	tests := []struct {
		slot      int
		class     party_state.CharacterClass
		currentMP byte
		wantMP    byte
	}{
		{slot: 0, class: party_state.Fighter, currentMP: 7, wantMP: 7},
		{slot: 1, class: party_state.Bard, currentMP: 3, wantMP: 9},
		{slot: 2, class: party_state.Wizard, currentMP: 2, wantMP: 18},
	}
	for _, test := range tests {
		character := gs.PartyState.GetCharacter(test.slot)
		character.PartyMap = party_state.PartyMapActive
		character.Class = test.class
		character.Intelligence = 18
		character.SetMp(test.currentMP)
		character.SetStatus(party_state.Dead)
		character.SetHp(0)
	}

	outcome := commitTestDeathResurrection(t, gs)
	require.Len(t, outcome.Members, len(tests))
	for index, test := range tests {
		assert.Equal(t, test.wantMP, gs.PartyState.GetCharacter(test.slot).GetCurrentMp())
		assert.Equal(t, test.currentMP, outcome.Members[index].Before.CurrentMP)
		assert.Equal(t, test.wantMP, outcome.Members[index].After.CurrentMP)
	}

	gs.PartyState.GetCharacter(tests[0].slot).SetMp(1)
	assert.Equal(t, tests[0].wantMP, outcome.Members[0].After.CurrentMP, "outcome snapshots must remain detached")
}

// TestIsAllPartyDead tests party wipe detection
func TestCommitDeathResurrection_RevivesAShipCrewOnFoot(t *testing.T) {
	gs := createTestGameStateForDeath(t, 99, 0)
	frigate := map_units.NewNPCFriendlyVehiceNewRef(
		references.FrigateVehicle, gs.MapState.PlayerLocation.Position, gs.MapState.PlayerLocation.Floor,
	)
	frigate.GetVehicleDetails().SetSailState(map_units.SailRaised)
	gs.PartyVehicle = *frigate

	commitTestDeathResurrection(t, gs)

	assert.Equal(t, references.NoPartyVehicle, gs.PartyVehicle.GetVehicleDetails().VehicleType,
		"a ship under sail would leave the revived party unable to move or exit")
}

func TestIsAllPartyDead(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(gs *GameState)
		expected bool
	}{
		{
			name: "Avatar alive - not all dead",
			setup: func(gs *GameState) {
				gs.PartyState.GetAvatar().SetStatus(party_state.Good)
			},
			expected: false,
		},
		{
			name: "Avatar dead - all dead (when only avatar in party)",
			setup: func(gs *GameState) {
				// First, remove all other party members from the party
				for i := 1; i < party_state.MaxActiveParty; i++ {
					char := gs.PartyState.GetCharacter(i)
					if char != nil {
						char.PartyMap = party_state.PartyMapUnrecruited
					}
				}
				// Then mark avatar as dead
				gs.PartyState.GetAvatar().SetStatus(party_state.Dead)
			},
			expected: true,
		},
		{
			name: "Avatar poisoned - not dead",
			setup: func(gs *GameState) {
				gs.PartyState.GetAvatar().SetStatus(party_state.Poisoned)
			},
			expected: false,
		},
		{
			name: "Avatar sleeping - not dead",
			setup: func(gs *GameState) {
				gs.PartyState.GetAvatar().SetStatus(party_state.Sleep)
			},
			expected: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := createTestGameStateForDeath(t, 75, 100)
			tc.setup(gs)

			result := gs.PartyState.IsAllPartyDead()
			assert.Equal(t, tc.expected, result, "IsAllPartyDead result incorrect")
		})
	}
}

// TestRecalculateLevelFromExperience tests level recalculation after XP loss
func TestRecalculateLevelFromExperience(t *testing.T) {
	tests := []struct {
		name          string
		xp            uint16
		expectedLevel byte
	}{
		{name: "Level 1 (0 XP)", xp: 0, expectedLevel: 1},
		{name: "Level 1 (99 XP)", xp: 99, expectedLevel: 1},
		{name: "Level 2 (100 XP)", xp: 100, expectedLevel: 2},
		{name: "Level 2 (199 XP)", xp: 199, expectedLevel: 2},
		{name: "Level 3 (200 XP)", xp: 200, expectedLevel: 3},
		{name: "Level 4 (400 XP)", xp: 400, expectedLevel: 4},
		{name: "Level 5 (800 XP)", xp: 800, expectedLevel: 5},
		{name: "Level 6 (1600 XP)", xp: 1600, expectedLevel: 6},
		{name: "Level 7 (3200 XP)", xp: 3200, expectedLevel: 7},
		{name: "Level 8 (6400 XP)", xp: 6400, expectedLevel: 8},
		{name: "Level 8 cap (10000 XP)", xp: 10000, expectedLevel: 8},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			gs := createTestGameStateForDeath(t, 75, tc.xp)
			avatar := gs.PartyState.GetAvatar()

			// Manually call recalculate to test directly
			gs.recalculateLevelFromExperience(avatar)

			assert.Equal(t, tc.expectedLevel, avatar.Level, "Level calculation incorrect for XP %d", tc.xp)

			// Verify max HP = level * 30
			expectedMaxHP := uint16(tc.expectedLevel) * 30
			assert.Equal(t, expectedMaxHP, avatar.GetMaxHp(), "Max HP should be level * 30")
		})
	}
}

// createTestGameStateForDeath creates a minimal GameState for death/resurrection testing
func createTestGameStateForDeath(t *testing.T, karma int, xp uint16) *GameState {
	t.Helper()

	// Use the existing test builder infrastructure
	builder := NewIntegrationTestBuilder(t)
	gs, _ := builder.WithSystemCallbacks().Build()
	require.NotNil(t, gs.PartyState, "PartyState should not be nil")

	// Set up initial state
	gs.PartyState.Karma.Value = karma
	avatar := gs.PartyState.GetAvatar()
	require.NotNil(t, avatar, "Avatar should not be nil")
	avatar.Exp = xp
	avatar.PartyMap = party_state.PartyMapActive

	return gs
}

func commitTestDeathResurrection(t *testing.T, gs *GameState) DeathResurrectionOutcome {
	t.Helper()
	markActivePartyDeadForTest(gs)
	review, err := gs.ReviewDeathResurrection()
	require.NoError(t, err)
	outcome, err := gs.CommitDeathResurrection(review.Fingerprint)
	require.NoError(t, err)

	return outcome
}
