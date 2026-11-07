package game_state

import (
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	beradrng "github.com/bradhannah/berad-sdk/modules/berad-rng"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckForWandererAppearance_Integration(t *testing.T) {
	tests := []struct {
		name              string
		location          references.Location
		lastWandererMonth int
		currentMonth      int
		forceWanderer     bool
		expectedResult    bool
		expectedMessage   string
	}{
		{
			name:              "Outdoor camping, new month, should have 25% chance",
			location:          references.Britain,
			lastWandererMonth: 1,
			currentMonth:      2,
			forceWanderer:     false,
			expectedResult:    false, // Replaced below with the deterministic d100 result.
		},
		{
			name:              "Outdoor camping, same month still rolls",
			location:          references.Britain,
			lastWandererMonth: 2,
			currentMonth:      2,
			forceWanderer:     false,
			expectedResult:    false,
		},
		{
			name:              "Dungeon location, should not appear",
			location:          references.Deceit,
			lastWandererMonth: 1,
			currentMonth:      2,
			forceWanderer:     false,
			expectedResult:    false,
		},
		{
			name:              "Force wanderer, should always appear",
			location:          references.Britain,
			lastWandererMonth: 2,
			currentMonth:      2,
			forceWanderer:     true,
			expectedResult:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, _ := NewIntegrationTestBuilder(t).
				WithLocation(tt.location).
				WithSystemCallbacks().
				Build()

			// Set deterministic seed for predictable d100 chance result
			gs.SetRandomSeed(12345)

			// Set up test conditions
			gs.LastWandererMonth = tt.lastWandererMonth
			gs.DateTime.Month = byte(tt.currentMonth)

			// Test the function
			result := gs.CheckForWandererAppearance(tt.forceWanderer)

			expected := tt.expectedResult
			if !tt.forceWanderer && tt.location.GetMapType() != references.DungeonMapType {
				expected = beradrng.New(12345).Intn(100) < 25
			}
			assert.Equal(t, expected, result, "CheckForWandererAppearance result mismatch")
		})
	}
}

func TestCheckForWandererAppearanceDoesNotSuppressSameMonth(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	gs.DateTime.Month = 2
	gs.LastWandererMonth = 2
	for seed := uint64(0); seed < 1000; seed++ {
		if beradrng.New(seed).Intn(100) < 25 {
			gs.SetRandomSeed(seed)
			assert.True(t, gs.CheckForWandererAppearance(false))
			return
		}
	}
	t.Fatal("unable to find Wanderer seed")
}

func TestResolveCampingRestAndWandererUsesPartyOrderedD63ThenD100(t *testing.T) {
	for _, test := range []struct {
		name         string
		seed         uint64
		wantWanderer bool
	}{
		{name: "no wanderer", seed: 1, wantWanderer: false},
		{name: "wanderer", seed: 6, wantWanderer: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).
				WithSystemCallbacks().
				Build()
			for index := 0; index < gs.PartyState.GetCharacterCount(); index++ {
				gs.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
			}
			for index, intelligence := range []byte{18, 22} {
				character := gs.PartyState.GetCharacter(index)
				character.PartyMap = party_state.PartyMapActive
				character.SetStatus(party_state.Good)
				character.SetMaxHp(100)
				character.SetHp(1)
				character.Class = party_state.Wizard
				character.Intelligence = intelligence
				character.SetMp(0)
			}
			gs.LastCamped = 0
			gs.SetRandomSeed(test.seed)
			oracle := beradrng.New(test.seed)
			wantHP := [2]uint16{
				uint16(1 + WandererHealMin + oracle.Intn(WandererHealMax-WandererHealMin+1)),
				uint16(1 + WandererHealMin + oracle.Intn(WandererHealMax-WandererHealMin+1)),
			}
			assert.Equal(t, test.wantWanderer, oracle.Intn(100) < 25)
			scene := camping.NewCampingSceneWithCombatRef(6, NoGuardAssigned, gs.MapState.PlayerLocation, testCampingMap(t))
			campState := gs.newCampingState(scene, false, false)

			gs.resolveCampingRestAndWanderer(scene, campState)

			assert.Equal(t, wantHP[0], gs.PartyState.GetCharacter(0).GetCurrentHp())
			assert.Equal(t, wantHP[1], gs.PartyState.GetCharacter(1).GetCurrentHp())
			assert.Equal(t, byte(18), gs.PartyState.GetCharacter(0).GetCurrentMp())
			assert.Equal(t, byte(22), gs.PartyState.GetCharacter(1).GetCurrentMp())
			assert.Equal(t, test.wantWanderer, campState.WandererEncountered)
			assert.Equal(t, uint64(3), gs.GetRNG().Calls())
			assert.Equal(t, oracle.Intn(97), gs.RandomIntInRange(0, 96),
				"camping must leave the shared stream immediately after two d63 rolls and one d100 roll")
		})
	}
}

func TestResolveCampingRestAndWandererForcedAppearanceConsumesNoD100(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	for index := 0; index < gs.PartyState.GetCharacterCount(); index++ {
		gs.PartyState.GetCharacter(index).PartyMap = party_state.PartyMapUnrecruited
	}
	character := gs.PartyState.GetCharacter(0)
	character.PartyMap = party_state.PartyMapActive
	character.SetStatus(party_state.Good)
	character.SetMaxHp(100)
	character.SetHp(1)
	gs.LastCamped = 0
	const seed = uint64(812)
	gs.SetRandomSeed(seed)
	oracle := beradrng.New(seed)
	wantHP := uint16(1 + WandererHealMin + oracle.Intn(WandererHealMax-WandererHealMin+1))
	scene := camping.NewCampingSceneWithCombatRef(6, NoGuardAssigned, gs.MapState.PlayerLocation, testCampingMap(t))
	campState := gs.newCampingState(scene, true, false)

	gs.resolveCampingRestAndWanderer(scene, campState)

	assert.Equal(t, wantHP, character.GetCurrentHp())
	assert.True(t, campState.WandererEncountered)
	assert.Equal(t, uint64(1), gs.GetRNG().Calls())
	assert.Equal(t, oracle.Intn(97), gs.RandomIntInRange(0, 96),
		"forced Wanderer must not consume the appearance roll")
}

func TestProcessWandererHealingForCharacter_Integration(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set up a character with damage and low MP
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.SetMaxHp(100)
	char.SetHp(50) // Damaged
	char.Intelligence = 20
	char.SetMp(5)                   // Low MP
	char.Class = party_state.Wizard // Mage for full MP restoration
	char.Level = 1                  // Start at level 1
	char.Exp = 50                   // Under 100, stays at level 1 (0-99 = level 1 per log2 formula)

	// Test healing without level up
	outcome := gs.ProcessWandererHealingForCharacter(0)

	// Verify healing occurred
	assert.Equal(t, char.GetMaxHp(), char.GetCurrentHp(), "Character should be fully healed")
	assert.Equal(t, char.Intelligence, char.GetCurrentMp(), "Mage should have full MP restored")
	assert.Equal(t, byte(1), outcome.LevelBefore)
	assert.Equal(t, byte(1), outcome.LevelAfter)
	assert.Empty(t, outcome.StatBoosts, "No stat boosts without level up")

	// Verify no callbacks were triggered (this is pure logic, no UI feedback)
	assert.Equal(t, 0, len(mockCallbacks.Messages), "No messages should be sent from this function")
}

func TestProcessWandererHealingForCharacter_SingleLevelGain_Integration(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set deterministic seed for predictable stat boost
	gs.SetRandomSeed(12345)

	// Set up character ready to level up (1 → 2)
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.Level = 1 // Start at level 1
	char.Exp = 100 // Exactly at level 2 threshold (100-199 = level 2 per log2 formula)
	char.SetMaxHp(30)
	char.SetHp(15) // Damaged
	char.Intelligence = 20
	char.SetMp(5)                   // Low MP
	char.Class = party_state.Wizard // Mage for full MP restoration

	// Track starting stats to verify boost
	oldStrength := char.Strength
	oldDexterity := char.Dexterity
	oldIntelligence := char.Intelligence

	outcome := gs.ProcessWandererHealingForCharacter(0)

	// Verify level up occurred
	assert.Equal(t, byte(1), outcome.LevelBefore)
	assert.Equal(t, byte(2), outcome.LevelAfter)
	assert.Equal(t, byte(2), char.Level, "Character should now be level 2")

	// Verify SINGLE stat boost for single level gain
	assert.Len(t, outcome.StatBoosts, 1, "Should have exactly one stat boost for single level gain")
	assert.Contains(t, []string{"stronger", "quicker", "wiser"}, outcome.StatBoosts[0], "Stat boost should be valid")

	// Verify exactly one stat was increased
	strengthIncreased := char.Strength > oldStrength
	dexterityIncreased := char.Dexterity > oldDexterity
	intelligenceIncreased := char.Intelligence > oldIntelligence

	boostCount := 0
	if strengthIncreased {
		boostCount++
	}
	if dexterityIncreased {
		boostCount++
	}
	if intelligenceIncreased {
		boostCount++
	}
	assert.Equal(t, 1, boostCount, "Exactly one stat should be increased")

	// Verify HP recalculated correctly (level * 30)
	assert.Equal(t, uint16(60), char.GetMaxHp(), "Max HP should be 60 (level 2 * 30)")
	assert.Equal(t, char.GetMaxHp(), char.GetCurrentHp(), "Character should be fully healed")

	assert.Equal(t, char.Intelligence, char.GetCurrentMp(), "Mage MP should use post-boost Intelligence")

	// Verify no callbacks were triggered (this is pure logic, no UI feedback)
	assert.Equal(t, 0, len(mockCallbacks.Messages), "No messages should be sent from this function")
}

func TestProcessWandererHealingForCharacter_MultiLevelGain_Integration(t *testing.T) {
	gs, mockCallbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	// Set deterministic seed for predictable stat boosts
	gs.SetRandomSeed(12345)

	// Set up character ready to gain multiple levels (1 → 4)
	// Level progression: 1=0-99, 2=100-199, 3=200-399, 4=400-799
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.Level = 1 // Start at level 1
	char.Exp = 400 // At level 4 threshold (400-799 = level 4 per log2 formula)
	char.SetMaxHp(30)
	char.SetHp(15) // Damaged
	char.Intelligence = 20
	char.SetMp(5)                   // Low MP
	char.Class = party_state.Wizard // Mage for full MP restoration

	// Track starting stats to verify boosts
	oldStrength := char.Strength
	oldDexterity := char.Dexterity
	oldIntelligence := char.Intelligence

	outcome := gs.ProcessWandererHealingForCharacter(0)

	// Verify multi-level up occurred (1 → 4 = 3 levels gained)
	assert.Equal(t, byte(1), outcome.LevelBefore)
	assert.Equal(t, byte(4), outcome.LevelAfter)
	assert.Equal(t, byte(4), char.Level, "Character should now be level 4")

	// CRITICAL: Verify THREE stat boosts for three levels gained
	assert.Len(t, outcome.StatBoosts, 3, "Should have exactly three stat boosts for three level gain")

	// Verify all boosts are valid
	for i, boost := range outcome.StatBoosts {
		assert.Contains(t, []string{"stronger", "quicker", "wiser"}, boost,
			"Stat boost %d should be valid, got: %s", i, boost)
	}

	// Verify at least one stat was increased (may be multiple if RNG varies)
	strengthIncreased := char.Strength > oldStrength
	dexterityIncreased := char.Dexterity > oldDexterity
	intelligenceIncreased := char.Intelligence > oldIntelligence

	assert.True(t, strengthIncreased || dexterityIncreased || intelligenceIncreased,
		"At least one stat should be increased")

	// Calculate total stat increases (could be 1-3 depending on RNG distribution)
	totalStatIncrease := (char.Strength - oldStrength) +
		(char.Dexterity - oldDexterity) +
		(char.Intelligence - oldIntelligence)
	assert.Equal(t, byte(3), totalStatIncrease, "Total stat increases should equal levels gained (3)")

	// Verify HP recalculated correctly (level * 30)
	assert.Equal(t, uint16(120), char.GetMaxHp(), "Max HP should be 120 (level 4 * 30)")
	assert.Equal(t, char.GetMaxHp(), char.GetCurrentHp(), "Character should be fully healed")

	assert.Equal(t, char.Intelligence, char.GetCurrentMp(), "Mage MP should use post-boost Intelligence")
	actualNextRoll := gs.RollDice(3)
	gs.SetRandomSeed(12345)
	for range 3 {
		gs.RollDice(3)
	}
	assert.Equal(t, gs.RollDice(3), actualNextRoll, "upward reconciliation must consume one roll per gained level")

	// Verify no callbacks were triggered (this is pure logic, no UI feedback)
	assert.Equal(t, 0, len(mockCallbacks.Messages), "No messages should be sent from this function")
}

// Awards skip stats already at 30 and stop once all three are capped (issue #82).
func TestProcessWandererHealingForCharacter_UpwardRollsSkipCappedStats(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.Level = 1
	char.Exp = 400
	char.Strength, char.Dexterity, char.Intelligence = 30, 30, 28
	gs.SetRandomSeed(12345)

	outcome := gs.ProcessWandererHealingForCharacter(0)

	assert.Equal(t, []string{"wiser", "wiser"}, outcome.StatBoosts, "the third award has no stat below 30")
	assert.Equal(t, byte(30), char.Strength)
	assert.Equal(t, byte(30), char.Dexterity)
	assert.Equal(t, byte(30), char.Intelligence)
}

func TestProcessWandererHealingForCharacter_ClassMPRestore(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	tests := []struct {
		name         string
		class        party_state.CharacterClass
		intelligence byte
		expectedMP   byte
	}{
		{
			name:         "Wizard gets full INT as MP",
			class:        party_state.Wizard,
			intelligence: 20,
			expectedMP:   20,
		},
		{
			name:         "Bard gets half INT as MP",
			class:        party_state.Bard,
			intelligence: 16,
			expectedMP:   8,
		},
		{
			name:         "Fighter gets no MP restoration",
			class:        party_state.Fighter,
			intelligence: 14,
			expectedMP:   5, // Should remain at original low level
		},
		{
			name:         "Avatar gets full INT as MP",
			class:        party_state.Avatar,
			intelligence: 14,
			expectedMP:   14, // Avatar gets full INT like Wizard
			// See ref:PROGRESS-0003.
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			char := gs.PartyState.GetCharacter(0)
			char.PartyMap = party_state.PartyMapActive
			char.SetStatus(party_state.Good)
			char.Class = tt.class
			char.Intelligence = tt.intelligence
			char.SetMp(5) // Start with low MP
			char.SetMaxHp(100)
			char.SetHp(50)

			gs.ProcessWandererHealingForCharacter(0)

			assert.Equal(t, tt.expectedMP, char.GetCurrentMp(), "MP restoration incorrect for class %v", tt.class)
		})
	}
}

func TestProcessWandererHealingForCharacter_ReconcilesDownWithoutRewardOrRNG(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.Level = 6
	char.Exp = 200
	char.SetMaxHp(180)
	char.SetHp(12)
	char.Strength, char.Dexterity, char.Intelligence = 11, 12, 13
	char.Class = party_state.Wizard
	gs.SetRandomSeed(99)

	outcome := gs.ProcessWandererHealingForCharacter(0)
	actualNextRoll := gs.RollDice(30)
	gs.SetRandomSeed(99)

	assert.Equal(t, byte(6), outcome.LevelBefore)
	assert.Equal(t, byte(3), outcome.LevelAfter)
	assert.Empty(t, outcome.StatBoosts)
	assert.Equal(t, byte(3), char.Level)
	assert.Equal(t, uint16(90), char.GetMaxHp())
	assert.Equal(t, uint16(90), char.GetCurrentHp())
	assert.Equal(t, byte(11), char.Strength)
	assert.Equal(t, byte(12), char.Dexterity)
	assert.Equal(t, byte(13), char.Intelligence)
	assert.Equal(t, byte(13), char.GetCurrentMp())
	assert.Equal(t, gs.RollDice(30), actualNextRoll, "downward reconciliation must not consume RNG")
}

func TestProcessWandererCharacter_DownwardFeedbackDoesNotClaimReward(t *testing.T) {
	gs, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.Level = 6
	char.Exp = 200
	char.SetMaxHp(180)
	char.SetHp(100)

	leveledUp, changes := gs.processWandererCharacter(0, &CampingState{WandererEncountered: true})
	assert.False(t, leveledUp)
	assert.False(t, changes.healing, "downward reconciliation must compare final current HP")
	assert.False(t, changes.levelUp)
	assert.Empty(t, callbacks.TransientIconCalls)
	require.Len(t, callbacks.StyledMessages, 2)
	assert.True(t, callbacks.StyledMessages[0].LeadingBlank)
	assert.False(t, callbacks.StyledMessages[1].LeadingBlank)
	feedback := strings.Join(callbacks.Messages, "\n")
	assert.Contains(t, feedback, "Awake")
	assert.Contains(t, feedback, "corrected to 3")
	assert.NotContains(t, strings.ToLower(feedback), "reward")
}

func TestProcessWandererHealingForCharacter_DeadClassMPRefreshOnly(t *testing.T) {
	gs, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	tests := []struct {
		class      party_state.CharacterClass
		expectedMP byte
	}{
		{class: party_state.Avatar, expectedMP: 18},
		{class: party_state.Wizard, expectedMP: 18},
		{class: party_state.Bard, expectedMP: 9},
		{class: party_state.Fighter, expectedMP: 5},
	}
	for _, test := range tests {
		char := gs.PartyState.GetCharacter(0)
		char.PartyMap = party_state.PartyMapActive
		char.Class = test.class
		char.Intelligence = 18
		char.Strength, char.Dexterity = 16, 17
		char.Level = 4
		char.Exp = 50
		char.SetMaxHp(120)
		char.SetHp(0)
		char.SetStatus(party_state.Dead)
		char.SetMp(5)
		callbacks.Messages = callbacks.Messages[:0]
		callbacks.StyledMessages = nil

		outcome := gs.ProcessWandererHealingForCharacter(0)

		assert.True(t, outcome.WasDead)
		assert.False(t, outcome.Restored)
		assert.Equal(t, test.expectedMP, char.GetCurrentMp())
		assert.Equal(t, party_state.Dead, char.GetStatus())
		assert.Zero(t, char.GetCurrentHp())
		assert.Equal(t, uint16(120), char.GetMaxHp())
		assert.Equal(t, byte(4), char.Level)
		assert.Equal(t, byte(16), char.Strength)
		assert.Equal(t, byte(17), char.Dexterity)
		assert.Equal(t, byte(18), char.Intelligence)

		campingState := &CampingState{WandererEncountered: true}
		leveledUp, _ := gs.processWandererCharacter(0, campingState)
		assert.False(t, leveledUp)
		assert.Empty(t, callbacks.StyledMessages)
		assert.Empty(t, callbacks.Messages, "dead MP-only refresh must not claim the character awoke or was restored")
	}
}

func TestProcessWandererCharacterPublishesOneLevelUpWithoutMaxHPHealing(t *testing.T) {
	gs, callbacks := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	char := gs.PartyState.GetCharacter(0)
	char.PartyMap = party_state.PartyMapActive
	char.SetStatus(party_state.Good)
	char.Level = 1
	char.Exp = 400
	char.SetMaxHp(30)
	char.SetHp(30)

	leveledUp, changes := gs.processWandererCharacter(0, &CampingState{WandererEncountered: true})

	assert.True(t, leveledUp)
	assert.False(t, changes.healing, "maximum-HP advancement alone is not explicit healing")
	assert.False(t, changes.poisonCure)
	assert.True(t, changes.levelUp)
	require.Len(t, callbacks.StyledMessages, 2)
	assert.Equal(t, StyledMessage{
		LeadingBlank: true,
		Segments: []MessageSegment{
			{Text: "\"", Style: MessageQuotation},
			{Text: "Awake " + char.GetNameAsString() + " and feel restored!", Style: MessageSpoken},
			{Text: "\"", Style: MessageQuotation},
		},
	}, callbacks.StyledMessages[0])
	assert.False(t, callbacks.StyledMessages[1].LeadingBlank)
	require.Len(t, callbacks.StyledMessages[1].Segments, 7)
	assert.Equal(t, MessageSegment{Text: " 4", Style: MessageHighlight}, callbacks.StyledMessages[1].Segments[2])
	assert.Equal(t, MessageHighlight, callbacks.StyledMessages[1].Segments[4].Style)
	assert.Contains(t, callbacks.Messages[1], "Thou art now level 4, and ")
	assert.Empty(t, callbacks.TransientIconCalls, "the Wanderer workflow aggregates character results")
	gs.publishTransientStatusChanges(changes)
	assert.Equal(t, []TransientIconKind{TransientIconLevelUp}, callbacks.TransientIconCalls)
}
