package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// BridgeTrollEncounterResult represents the outcome of a bridge crossing attempt.
// This is returned from CheckBridgeTrollEncounter and processed by the UI layer.
type BridgeTrollEncounterResult struct {
	Triggered bool // Did trolls appear? (passed 1/8 chance check)
}

// CheckBridgeTrollEncounter reviews a bridge encounter without presentation.
// Roll 0..7 first; only zero continues, then require travel on foot.
// This Redux generator makes the gate one in eight; external original RNG
// distribution is not established by the original call bounds. ref:ENV-0004
func (g *GameState) CheckBridgeTrollEncounter() *BridgeTrollEncounterResult {
	// Draw the encounter gate before checking travel mode; only zero of
	// 0..7 continues. ref:ENV-0004
	if !g.OneInXOdds(8) {
		return nil
	}

	// Original consumes the random roll before checking whether the party is on foot.
	if g.PartyVehicle.GetVehicleDetails().VehicleType != references.NoPartyVehicle {
		return nil
	}

	return &BridgeTrollEncounterResult{
		Triggered: true,
	}
}

// CalculateTrollToll uses the first active Good or Poisoned character, or the
// caught character when none is Good or Poisoned (for example when every awake
// member is Charmed; the original then reads a stale index). The toll is 99 minus three times Strength; edited values are not clamped.
// Strength 10, 20, 30 and 35 yield 69, 39, 9 and -6 respectively. ref:ENV-0004
func (g *GameState) CalculateTrollToll(caught *party_state.PlayerCharacter) int {
	character := g.trollTollCharacter()
	if character == nil {
		character = caught
	}

	return 99 - (3 * int(character.Strength))
}

// PayTrollToll attempts to subtract the toll from party gold.
// An unaffordable positive toll restores the balance and reports failure;
// a successful payment reports true. ref:ENV-0004
func (g *GameState) PayTrollToll(amount int) bool {
	if amount < 0 {
		g.PartyState.Inventory.Gold.IncrementBy(uint16(-amount))

		return true
	}
	if amount == 0 {
		return true // Nothing to pay
	}

	return g.PartyState.Inventory.Gold.DecrementBy(uint16(amount))
}

// QueueBridgeTrollCombat defers the source-less special encounter until the
// bridge dialog workflow releases event-queue ownership.
func (g *GameState) QueueBridgeTrollCombat() {
	if g.CombatScene != nil || g.pendingCombatEncounter != nil {
		panic("bridge troll combat queued while another combat encounter is active or pending")
	}

	g.pendingCombatEncounter = &CombatEncounterRequest{
		Map: CombatEncounterMap{
			Territory:    references.BritanniaCombatTerritory,
			BritanniaMap: references.BigBridgeCombatMap,
		},
		EntryDirection: CombatEntryNorth,
		MonsterType:    references.MonsterTroll,
		CountMode:      CombatEncounterCountAutomatic,
		FriendsEnabled: true,
	}
}

// PerformSneakCheck reports whether a party member is caught.
// A draw in 1..30 above Dexterity means failure. ref:ENV-0004
func (g *GameState) PerformSneakCheck(character *party_state.PlayerCharacter) bool {
	// random(1,30) > dex means CAUGHT
	// If roll is higher than dex, they fail to sneak
	roll := g.RandomIntInRange(1, 30)

	return roll > int(character.Dexterity)
}

// GetLivingAwakePartyMembers selects active members who are neither Dead
// nor Sleeping for the bridge sneak checks. ref:ENV-0004
func (g *GameState) GetLivingAwakePartyMembers() []*party_state.PlayerCharacter {
	var members []*party_state.PlayerCharacter

	g.PartyState.ForEachCharacter(func(_ int, char *party_state.PlayerCharacter) {
		if char.PartyMap != party_state.PartyMapActive {
			return
		}
		if char.IsDead() {
			return
		}
		if char.IsAsleep() {
			return
		}
		members = append(members, char)
	})

	return members
}

func (g *GameState) trollTollCharacter() *party_state.PlayerCharacter {
	for index := range party_state.MaxActiveParty {
		character := g.PartyState.GetCharacter(index)
		if character == nil || character.PartyMap != party_state.PartyMapActive {
			continue
		}
		status := character.GetStatus()
		if status == party_state.Good || status == party_state.Poisoned {
			return character
		}
	}

	return nil
}
