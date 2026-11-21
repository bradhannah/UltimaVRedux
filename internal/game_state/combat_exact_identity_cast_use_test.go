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

func TestCombatCastUsesExactAwakeCloneIdentity(t *testing.T) {
	state, source, clone := newSyntheticExactIdentityCastUseState(t)
	character := state.PartyState.GetCharacter(source.EntityIndex)
	state.setCombatPCSleeping(source, true)
	state.CombatScene.CurrentTurnIdentity = clone
	state.CombatScene.CurrentTurnIndex = clone.EntityIndex
	state.PartyState.Inventory.Spells.Set(references.InXenMani, 1)
	mpBefore := character.GetCurrentMp()
	commitBefore := state.CastUseCommitSequence()

	result := state.ValidateAndConsumeSpellResources(CastRequest{
		Spell: references.InXenMani, CasterIdx: clone.EntityIndex,
	})

	require.True(t, result.Valid)
	assert.True(t, state.CombatScene.isCombatPCSleeping(source))
	assert.False(t, state.CombatScene.isCombatPCSleeping(clone))
	assert.True(t, character.IsAsleep(), "shared roster status remains authoritative")
	assert.Equal(t, mpBefore-byte(result.SpellInfo.Circle), character.GetCurrentMp())
	assert.Zero(t, state.PartyState.Inventory.Spells.Get(references.InXenMani))
	assert.Equal(t, commitBefore+1, state.CastUseCommitSequence())
}

func TestCombatCastRejectsExactIncapacitatedOrUncontrolledClone(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GameState, CombatTurnIdentity)
	}{
		{
			name: "sleeping",
			mutate: func(state *GameState, clone CombatTurnIdentity) {
				state.setCombatPCSleeping(clone, true)
			},
		},
		{
			name: "underground",
			mutate: func(state *GameState, clone CombatTurnIdentity) {
				state.CombatScene.setCombatPCUnderground(clone, true)
			},
		},
		{
			name: "possessed",
			mutate: func(state *GameState, clone CombatTurnIdentity) {
				state.CombatScene.setCombatPCPossessed(clone, true)
			},
		},
		{
			name: "inactive",
			mutate: func(state *GameState, clone CombatTurnIdentity) {
				state.CombatScene.TurnQueue.markFledIdentity(clone)
			},
		},
		{
			name: "shared roster death",
			mutate: func(state *GameState, clone CombatTurnIdentity) {
				state.PartyState.GetCharacter(clone.EntityIndex).SetStatus(party_state.Dead)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _, clone := newSyntheticExactIdentityCastUseState(t)
			state.CombatScene.CurrentTurnIdentity = clone
			state.CombatScene.CurrentTurnIndex = clone.EntityIndex
			state.PartyState.Inventory.Spells.Set(references.InXenMani, 1)
			character := state.PartyState.GetCharacter(clone.EntityIndex)
			mpBefore := character.GetCurrentMp()
			commitBefore := state.CastUseCommitSequence()
			test.mutate(state, clone)

			result := state.ValidateAndConsumeSpellResources(CastRequest{
				Spell: references.InXenMani, CasterIdx: clone.EntityIndex,
			})

			require.False(t, result.Valid)
			assert.Equal(t, "Thou canst not cast now!", result.ErrorMsg)
			assert.Equal(t, mpBefore, character.GetCurrentMp())
			assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InXenMani))
			assert.Equal(t, commitBefore, state.CastUseCommitSequence())
		})
	}
}

func TestNonCombatCastStillUsesSharedRosterSleep(t *testing.T) {
	state, source, _ := newSyntheticExactIdentityCastUseState(t)
	state.CombatScene = nil
	state.MapState.PlayerLocation.Location = references.Britain
	state.PartyState.GetCharacter(source.EntityIndex).SetStatus(party_state.Sleep)
	state.PartyState.Inventory.Spells.Set(references.InXenMani, 1)

	result := state.ValidateAndConsumeSpellResources(CastRequest{
		Spell: references.InXenMani, CasterIdx: source.EntityIndex,
	})

	require.False(t, result.Valid)
	assert.Equal(t, "Thou canst not cast now!", result.ErrorMsg)
	assert.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InXenMani))
}

func TestCombatBluePotionUsesRetainedDrinkingIdentity(t *testing.T) {
	t.Run("awake clone does not wake sleeping source", func(t *testing.T) {
		state, source, clone := newSyntheticExactIdentityCastUseState(t)
		character := state.PartyState.GetCharacter(source.EntityIndex)
		state.setCombatPCSleeping(source, true)
		state.CombatScene.CurrentTurnIdentity = clone
		state.CombatScene.CurrentTurnIndex = clone.EntityIndex
		state.PartyState.Inventory.Potions.Set(references.Blue, 1)
		state.SetRandomSeed(42)

		commit, review := state.BeginPotionUse(references.Blue, UsageCombat)
		require.True(t, review.Ready)
		outcome, committed := state.CommitPotionUse(commit)
		require.True(t, committed)
		effect := state.CommitPotionEffect(outcome, source.EntityIndex)

		assert.Equal(t, PotionEffectNoEffect, effect)
		assert.Equal(t, references.Blue, outcome.FinalPotion)
		assert.False(t, outcome.Mislabelled)
		assert.Zero(t, state.PartyState.Inventory.Potions.Get(references.Blue))
		assert.True(t, state.CombatScene.isCombatPCSleeping(source))
		assert.False(t, state.CombatScene.isCombatPCSleeping(clone))
		assert.True(t, character.IsGood(), "Blue clears shared roster sleep before checking the drinking entry")
		assert.Equal(t, uint64(1), state.GetRNG().Calls())
	})

	t.Run("sleeping clone wakes without changing awake source", func(t *testing.T) {
		state, source, clone := newSyntheticExactIdentityCastUseState(t)
		state.setCombatPCSleeping(clone, true)
		state.CombatScene.CurrentTurnIdentity = clone
		state.CombatScene.CurrentTurnIndex = clone.EntityIndex
		state.PartyState.Inventory.Potions.Set(references.Blue, 1)
		state.SetRandomSeed(42)

		commit, review := state.BeginPotionUse(references.Blue, UsageCombat)
		require.True(t, review.Ready)
		outcome, committed := state.CommitPotionUse(commit)
		require.True(t, committed)

		assert.Equal(t, PotionEffectAwakened, state.CommitPotionEffect(outcome, source.EntityIndex))
		assert.False(t, state.CombatScene.isCombatPCSleeping(source))
		assert.False(t, state.CombatScene.isCombatPCSleeping(clone))
		assert.True(t, state.PartyState.GetCharacter(source.EntityIndex).IsGood())
	})
}

func TestCombatOrangePotionAndAnZuKeepTheirIdentityScopes(t *testing.T) {
	t.Run("Orange sleeps only retained clone", func(t *testing.T) {
		state, source, clone := newSyntheticExactIdentityCastUseState(t)
		state.CombatScene.CurrentTurnIdentity = clone
		state.CombatScene.CurrentTurnIndex = clone.EntityIndex
		state.PartyState.Inventory.Potions.Set(references.Orange, 1)
		state.SetRandomSeed(42)

		commit, review := state.BeginPotionUse(references.Orange, UsageCombat)
		require.True(t, review.Ready)
		outcome, committed := state.CommitPotionUse(commit)
		require.True(t, committed)

		assert.Equal(t, PotionEffectSlept, state.CommitPotionEffect(outcome, source.EntityIndex))
		assert.False(t, state.CombatScene.isCombatPCSleeping(source))
		assert.True(t, state.CombatScene.isCombatPCSleeping(clone))
	})

	t.Run("mislabelled Blue applies Orange to retained clone", func(t *testing.T) {
		state, source, clone := newSyntheticExactIdentityCastUseState(t)
		state.CombatScene.CurrentTurnIdentity = clone
		state.CombatScene.CurrentTurnIndex = clone.EntityIndex
		state.PartyState.Inventory.Potions.Set(references.Blue, 1)
		seed := exactIdentityPotionSeed(t, state, func(roll int) bool { return roll == 0 })
		state.SetRandomSeed(seed)

		commit, review := state.BeginPotionUse(references.Blue, UsageCombat)
		require.True(t, review.Ready)
		outcome, committed := state.CommitPotionUse(commit)
		require.True(t, committed)

		assert.Equal(t, PotionEffectSlept, state.CommitPotionEffect(outcome, source.EntityIndex))
		assert.Equal(t, references.Orange, outcome.FinalPotion)
		assert.True(t, outcome.Mislabelled)
		assert.False(t, state.CombatScene.isCombatPCSleeping(source))
		assert.True(t, state.CombatScene.isCombatPCSleeping(clone))
		assert.Zero(t, state.PartyState.Inventory.Potions.Get(references.Blue))
		assert.Equal(t, uint64(1), state.GetRNG().Calls())
	})

	t.Run("An Zu searches sleeping entries for roster target", func(t *testing.T) {
		state, source, clone := newSyntheticExactIdentityCastUseState(t)
		state.setCombatPCSleeping(clone, true)

		require.True(t, state.AwakenCharacter(source.EntityIndex))
		assert.False(t, state.CombatScene.isCombatPCSleeping(source))
		assert.False(t, state.CombatScene.isCombatPCSleeping(clone))
		assert.True(t, state.PartyState.GetCharacter(source.EntityIndex).IsGood())
	})
}

func newSyntheticExactIdentityCastUseState(
	t *testing.T,
) (*GameState, CombatTurnIdentity, CombatTurnIdentity) {
	t.Helper()

	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	for index := range roster {
		roster[index] = &party_state.PlayerCharacter{PartyMap: party_state.PartyMapUnrecruited}
	}
	character := roster[0]
	character.PartyMap = party_state.PartyMapActive
	copy(character.Name[:], "Avatar")
	character.Class = party_state.Wizard
	character.Level = 8
	character.Strength = 30
	character.Dexterity = 30
	character.Intelligence = 30
	character.SetMaxHp(100)
	character.SetHp(100)
	character.SetMp(30)
	character.SetStatus(party_state.Good)
	party := party_state.NewPartyStateFromRoster(roster)

	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	var rawMap [references.XCombatMapTiles][references.YCombatMapTiles]byte
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			rawMap[x][y] = byte(indexes.BrickFloor)
		}
	}
	combatMap.SetRawData(&rawMap)
	combat := NewCombatState(combatMap, CombatEntryNorth, party)
	tiles := references.NewTileReferences()
	callbacks := NewMockSystemCallbacks(t)
	state := &GameState{
		PartyState:  *party,
		CombatScene: combat,
		GameReferences: &references.GameReferences{
			TileReferences: tiles,
		},
		SystemCallbacks: callbacks.ToSystemCallbacks(),
	}
	state.MapState.PlayerLocation.Location = references.CombatLocation
	state.MapState.XTilesVisibleOnGameScreen = 19
	state.MapState.YTilesVisibleOnGameScreen = 13
	state.MapState.LayeredMaps = *map_state.NewLayeredMaps(
		tiles,
		&references.LargeMapReference{},
		&references.LargeMapReference{},
		19,
		13,
	)
	state.MapState.LayeredMaps.ResetAndCreateCombatMap(combatMap, tiles, 19, 13)
	state.SetRandomSeed(1)

	source := combat.GetCurrentTurnIdentity()
	require.True(t, state.CastCombatInQuasXen(source.EntityIndex, source))
	clone := CombatTurnIdentity{}
	combat.ForEachPCClone(func(identity CombatTurnIdentity, _ references.Position) {
		clone = identity
	})
	require.True(t, clone.IsPartyMember)
	require.NotZero(t, clone.Slot)
	callbacks.Reset()

	return state, source, clone
}

func exactIdentityPotionSeed(t *testing.T, state *GameState, matches func(int) bool) uint64 {
	t.Helper()
	for seed := uint64(1); seed < 10000; seed++ {
		state.SetRandomSeed(seed)
		if matches(state.RandomIntInRange(0, potionRandomEffectRollMax-1)) {
			return seed
		}
	}
	t.Fatal("no deterministic potion seed matched")

	return 0
}
