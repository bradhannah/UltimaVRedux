package party_state

import (
	"reflect"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestMartyrSecondLivingMovesCompleteRecordStably(t *testing.T) {
	var roster [NRosterSlots]*PlayerCharacter
	for index := range roster {
		character := &PlayerCharacter{PartyMap: PartyMapUnrecruited, Strength: byte(index + 1)}
		copy(character.Name[:], []byte("slot"))
		roster[index] = character
	}
	for index := range 4 {
		roster[index].PartyMap = PartyMapActive
		roster[index].SetStatus(Good)
	}
	roster[1].SetStatus(Dead)
	removedBefore := *roster[2]
	slotThreeStrength := roster[3].Strength
	party := NewPartyStateFromRoster(roster)

	martyr := party.MartyrSecondLiving()

	removedBefore.PartyMap = PartyMapMartyr
	if !reflect.DeepEqual(removedBefore, *martyr) {
		t.Fatalf("martyr record changed: got %#v, want %#v", *martyr, removedBefore)
	}
	if party.GetCharacter(2).Strength != slotThreeStrength {
		t.Fatal("later roster records were not shifted left stably")
	}
	if party.GetActivePartyCount() != 3 {
		t.Fatalf("active count = %d, want 3", party.GetActivePartyCount())
	}
	if martyr.IsDead() {
		t.Fatal("Blackthorn martyr was incorrectly marked dead")
	}
}

func TestMartyrNamesScansSlotsOneThroughFifteen(t *testing.T) {
	var roster [NRosterSlots]*PlayerCharacter
	for index := range roster {
		character := &PlayerCharacter{PartyMap: PartyMapUnrecruited}
		roster[index] = character
	}
	copy(roster[0].Name[:], "Avatar")
	roster[0].PartyMap = PartyMapMartyr
	copy(roster[3].Name[:], "Iolo")
	roster[3].PartyMap = PartyMapMartyr
	copy(roster[12].Name[:], "Julia")
	roster[12].PartyMap = PartyMapMartyr

	party := NewPartyStateFromRoster(roster)
	if got := party.MartyrNames(); !reflect.DeepEqual(got, []string{"Iolo", "Julia"}) {
		t.Fatalf("MartyrNames() = %v", got)
	}
}

func TestNPCFlagSettersValidatePersistentIdentityBoundaries(t *testing.T) {
	party := newPartyState()
	party.SetDeadNpc(references.Moonglow, 0)
	party.SetDeadNpc(references.SerpentsHold, NPcsPerLocation-1)

	for _, test := range []struct {
		name     string
		location references.Location
		npc      int
	}{
		{name: "world map", location: references.BritanniaUnderworld, npc: 0},
		{name: "after last location", location: references.Deceit, npc: 0},
		{name: "negative NPC", location: references.Moonglow, npc: -1},
		{name: "after last NPC", location: references.Moonglow, npc: NPcsPerLocation},
	} {
		t.Run(test.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Fatal("SetDeadNpc did not reject invalid identity")
				}
			}()
			party.SetDeadNpc(test.location, test.npc)
		})
	}
}

// createTestPartyState creates a PartyState with test characters for unit testing.
// Creates characters at indices 0-5 with names "Avatar", "Shamino", "Iolo", "Dupre", "Jaana", "Julia"
func createTestPartyState() *PartyState {
	ps := newPartyState()

	// Create test characters
	names := []string{"Avatar", "Shamino", "Iolo", "Dupre", "Jaana", "Julia"}
	for i, name := range names {
		char := &PlayerCharacter{}
		copy(char.Name[:], name)
		char.PartyMap = PartyMapActive
		ps.characters[i] = char
	}

	return ps
}

func TestSwapMembers_Success(t *testing.T) {
	ps := createTestPartyState()

	// Verify initial order
	if ps.GetCharacter(1).GetNameAsString() != "Shamino" {
		t.Fatalf("Expected Shamino at index 1, got %s", ps.GetCharacter(1).GetNameAsString())
	}
	if ps.GetCharacter(2).GetNameAsString() != "Iolo" {
		t.Fatalf("Expected Iolo at index 2, got %s", ps.GetCharacter(2).GetNameAsString())
	}

	// Swap Shamino (1) and Iolo (2)
	err := ps.SwapMembers(1, 2)
	if err != nil {
		t.Fatalf("SwapMembers failed: %v", err)
	}

	// Verify swap occurred
	if ps.GetCharacter(1).GetNameAsString() != "Iolo" {
		t.Errorf("Expected Iolo at index 1 after swap, got %s", ps.GetCharacter(1).GetNameAsString())
	}
	if ps.GetCharacter(2).GetNameAsString() != "Shamino" {
		t.Errorf("Expected Shamino at index 2 after swap, got %s", ps.GetCharacter(2).GetNameAsString())
	}
}

func TestSwapMembers_AvatarCannotBeSwapped(t *testing.T) {
	ps := createTestPartyState()

	// Try to swap Avatar with Shamino
	err := ps.SwapMembers(0, 1)
	if err == nil {
		t.Error("Expected error when swapping Avatar, got nil")
	}
	if err.Error() != "avatar must lead" {
		t.Errorf("Expected avatar-must-lead error, got: %s", err.Error())
	}

	// Try with reversed order
	err = ps.SwapMembers(1, 0)
	if err == nil {
		t.Error("Expected error when swapping with Avatar, got nil")
	}
	if err.Error() != "avatar must lead" {
		t.Errorf("Expected avatar-must-lead error, got: %s", err.Error())
	}

	// Verify Avatar is still at position 0
	if ps.GetCharacter(0).GetNameAsString() != "Avatar" {
		t.Errorf("Avatar should still be at position 0, got %s", ps.GetCharacter(0).GetNameAsString())
	}
}

func TestSwapMembers_SameIndex(t *testing.T) {
	ps := createTestPartyState()

	// Swap same index should be no-op without error
	err := ps.SwapMembers(2, 2)
	if err != nil {
		t.Errorf("Same index swap should not error, got: %v", err)
	}

	// Verify no change
	if ps.GetCharacter(2).GetNameAsString() != "Iolo" {
		t.Errorf("Character at index 2 should still be Iolo, got %s", ps.GetCharacter(2).GetNameAsString())
	}
}

func TestSwapMembers_OutOfBounds(t *testing.T) {
	ps := createTestPartyState()

	// Index too high
	err := ps.SwapMembers(1, 6)
	if err == nil {
		t.Error("Expected error for index 6 (out of bounds)")
	}

	// Negative index
	err = ps.SwapMembers(-1, 2)
	if err == nil {
		t.Error("Expected error for negative index")
	}
}

func TestSwapMembers_NilCharacter(t *testing.T) {
	ps := createTestPartyState()

	// Clear character at index 5
	ps.characters[5] = nil

	// Try to swap with nil slot
	err := ps.SwapMembers(1, 5)
	if err == nil {
		t.Error("Expected error when swapping with nil character slot")
	}
}

func TestSwapMembers_DoubleSwapRestoresOriginal(t *testing.T) {
	ps := createTestPartyState()

	// Record original order
	original1 := ps.GetCharacter(1).GetNameAsString()
	original2 := ps.GetCharacter(2).GetNameAsString()

	// Swap twice
	_ = ps.SwapMembers(1, 2)
	_ = ps.SwapMembers(1, 2)

	// Should be back to original
	if ps.GetCharacter(1).GetNameAsString() != original1 {
		t.Errorf("Double swap should restore original: expected %s at 1, got %s",
			original1, ps.GetCharacter(1).GetNameAsString())
	}
	if ps.GetCharacter(2).GetNameAsString() != original2 {
		t.Errorf("Double swap should restore original: expected %s at 2, got %s",
			original2, ps.GetCharacter(2).GetNameAsString())
	}
}

func TestGetActivePartyCount(t *testing.T) {
	ps := createTestPartyState()

	// Full party of 6
	count := ps.GetActivePartyCount()
	if count != 6 {
		t.Errorf("Expected 6 active party members, got %d", count)
	}

	// Remove one from party
	ps.GetCharacter(5).PartyMap = PartyMapUnrecruited
	count = ps.GetActivePartyCount()
	if count != 5 {
		t.Errorf("Expected 5 active party members after removal, got %d", count)
	}
}

func TestHasDeadMembers_NoDeadMembers(t *testing.T) {
	ps := createTestPartyState()

	// All characters are alive by default
	for i := 0; i < 6; i++ {
		ps.GetCharacter(i).SetStatus(Good)
	}

	if ps.HasDeadMembers() {
		t.Error("HasDeadMembers should return false when no party members are dead")
	}
}

func TestHasDeadMembers_OneDeadMember(t *testing.T) {
	ps := createTestPartyState()

	// Set all to Good first
	for i := 0; i < 6; i++ {
		ps.GetCharacter(i).SetStatus(Good)
	}

	// Kill one character
	ps.GetCharacter(2).SetStatus(Dead)

	if !ps.HasDeadMembers() {
		t.Error("HasDeadMembers should return true when one party member is dead")
	}
}

func TestHasDeadMembers_AllDeadMembers(t *testing.T) {
	ps := createTestPartyState()

	// Kill all characters
	for i := 0; i < 6; i++ {
		ps.GetCharacter(i).SetStatus(Dead)
	}

	if !ps.HasDeadMembers() {
		t.Error("HasDeadMembers should return true when all party members are dead")
	}
}

func TestHasDeadMembers_DeadCharacterNotInParty(t *testing.T) {
	ps := createTestPartyState()

	// Set all to Good
	for i := 0; i < 6; i++ {
		ps.GetCharacter(i).SetStatus(Good)
	}

	// Kill character 5 but remove from party
	ps.GetCharacter(5).SetStatus(Dead)
	ps.GetCharacter(5).PartyMap = PartyMapUnrecruited

	// Should return false because dead character is not in active party
	if ps.HasDeadMembers() {
		t.Error("HasDeadMembers should return false when dead character is not in active party")
	}
}
