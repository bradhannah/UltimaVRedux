package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// createTestPartyStateForResurrection creates a PartyState with test characters for resurrection testing.
func createTestPartyStateForResurrection() *party_state.PartyState {
	state := party_state.NewBlankPartyState()

	// Set up three characters
	char0 := state.GetCharacter(0)
	char0.Name = [9]byte{'A', 'v', 'a', 't', 'a', 'r', 0, 0, 0}
	char0.SetStatus(party_state.Good)
	char0.PartyMap = party_state.PartyMapActive
	char0.SetMaxHp(100)
	char0.SetHp(50)

	char1 := state.GetCharacter(1)
	char1.Name = [9]byte{'S', 'h', 'a', 'm', 'i', 'n', 'o', 0, 0}
	char1.PartyMap = party_state.PartyMapActive
	char1.SetMaxHp(80)
	char1.SetHp(0)                    // Must set HP before status
	char1.SetStatus(party_state.Dead) // SetHp auto-revives, so set Dead AFTER

	char2 := state.GetCharacter(2)
	char2.Name = [9]byte{'I', 'o', 'l', 'o', 0, 0, 0, 0, 0}
	char2.SetStatus(party_state.Good)
	char2.PartyMap = party_state.PartyMapActive
	char2.SetMaxHp(60)
	char2.SetHp(30)

	return state
}

func TestResurrectCharacter_Success(t *testing.T) {
	// Arrange
	ps := createTestPartyStateForResurrection()
	gs := &GameState{
		PartyState: *ps,
	}

	// Verify character 1 is dead
	char := gs.PartyState.GetCharacter(1)
	char.Class = party_state.Bard
	char.Intelligence = 19
	char.Exp = 200
	char.Level = 8
	gs.PartyState.Karma.Value = 97
	if !char.IsDead() {
		t.Fatalf("Expected character 1 to be dead initially")
	}
	if char.GetCurrentHp() != 0 {
		t.Fatalf("Expected dead character to have 0 HP, got %d", char.GetCurrentHp())
	}

	// Act
	success := gs.ResurrectCharacter(1)

	// Assert
	if !success {
		t.Error("ResurrectCharacter should return true for dead character")
	}
	if char.IsDead() {
		t.Error("Character should no longer be dead after resurrection")
	}
	if char.GetStatus() != party_state.Good {
		t.Errorf("Character status should be Good after resurrection, got %v", char.GetStatus())
	}
	if char.GetCurrentHp() != 1 {
		t.Errorf("Character HP should be 1 after resurrection, got %d", char.GetCurrentHp())
	}
	if char.GetCurrentMp() != 9 || char.Exp != 194 || char.Level != 2 || char.GetMaxHp() != 60 {
		t.Errorf("Resurrection recalculation got MP=%d XP=%d level=%d maxHP=%d",
			char.GetCurrentMp(), char.Exp, char.Level, char.GetMaxHp())
	}
}

func TestResurrectCharacter_AlreadyAlive(t *testing.T) {
	// Arrange
	ps := createTestPartyStateForResurrection()
	gs := &GameState{
		PartyState: *ps,
	}

	// Character 0 is alive
	char := gs.PartyState.GetCharacter(0)
	originalHP := char.GetCurrentHp()

	// Act
	success := gs.ResurrectCharacter(0)

	// Assert
	if success {
		t.Error("ResurrectCharacter should return false for alive character")
	}
	if char.GetCurrentHp() != originalHP {
		t.Errorf("HP should not change for alive character, was %d, now %d", originalHP, char.GetCurrentHp())
	}
}

func TestResurrectCharacter_InvalidIndex(t *testing.T) {
	// Arrange
	ps := createTestPartyStateForResurrection()
	gs := &GameState{
		PartyState: *ps,
	}

	// Act - try to resurrect character at invalid index (nil slot)
	success := gs.ResurrectCharacter(15) // Way out of bounds

	// Assert
	if success {
		t.Error("ResurrectCharacter should return false for invalid index")
	}
}

func TestResurrectCharacterOriginalClassAndLevelBoundaries(t *testing.T) {
	tests := []struct {
		name       string
		class      party_state.CharacterClass
		intellect  byte
		currentMP  byte
		experience uint16
		karma      int
		wantMP     byte
		wantXP     uint16
		wantLevel  byte
	}{
		{"avatar restores full MP below first threshold", party_state.Avatar, 20, 1, 99, 98, 20, 99, 1},
		{"wizard restores full MP at threshold", party_state.Wizard, 18, 1, 100, 98, 18, 100, 2},
		{"bard restores half MP", party_state.Bard, 19, 1, 199, 98, 9, 199, 2},
		{"fighter retains MP like original switch", party_state.Fighter, 16, 7, 200, 98, 7, 200, 3},
		{"karma below 98 scales XP", party_state.Wizard, 18, 1, 200, 97, 18, 194, 2},
		{"karma above 98 does not increase XP", party_state.Wizard, 18, 1, 200, 99, 18, 200, 3},
		{"maximum legacy XP reaches level eight", party_state.Avatar, 20, 1, 9999, 100, 20, 9999, 8},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ps := createTestPartyStateForResurrection()
			gs := &GameState{PartyState: *ps}
			character := gs.PartyState.GetCharacter(1)
			character.Class = test.class
			character.Intelligence = test.intellect
			character.SetMp(test.currentMP)
			character.Exp = test.experience
			character.Level = 8
			gs.PartyState.Karma.Value = test.karma

			if !gs.ResurrectCharacter(1) {
				t.Fatal("expected resurrection to succeed")
			}
			if character.GetCurrentMp() != test.wantMP || character.Exp != test.wantXP || character.Level != test.wantLevel {
				t.Fatalf("got MP=%d XP=%d level=%d; want MP=%d XP=%d level=%d",
					character.GetCurrentMp(), character.Exp, character.Level, test.wantMP, test.wantXP, test.wantLevel)
			}
			if character.GetMaxHp() != uint16(test.wantLevel)*30 || character.GetCurrentHp() != 1 || !character.IsGood() {
				t.Fatalf("got maxHP=%d HP=%d status=%v", character.GetMaxHp(), character.GetCurrentHp(), character.GetStatus())
			}
		})
	}
}

func TestCreateFoodIsDeterministicAndReportsSaturatedAmount(t *testing.T) {
	first, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithRandomSeed(42).
		Build()
	second, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithRandomSeed(42).
		Build()
	first.PartyState.Inventory.Provisions.Food.Set(0)
	second.PartyState.Inventory.Provisions.Food.Set(0)

	firstAdded := first.CreateFood()
	secondAdded := second.CreateFood()
	require.NotZero(t, firstAdded)
	assert.LessOrEqual(t, firstAdded, uint16(CreateFoodDiceSize))
	assert.Equal(t, firstAdded, secondAdded)
	assert.Equal(t, firstAdded, first.PartyState.Inventory.Provisions.Food.Get())

	first.PartyState.Inventory.Provisions.Food.Set(9998)
	assert.Equal(t, uint16(1), first.CreateFood())
	assert.Equal(t, uint16(9999), first.PartyState.Inventory.Provisions.Food.Get())
	assert.Equal(t, uint16(0), first.CreateFood())
}
