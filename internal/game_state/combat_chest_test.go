package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestActionOpenCombatMapRemovesEmptyChest(t *testing.T) {
	gs, mock := newBritainCombatTestState(t, 169)
	activePosition := gs.CombatScene.GetPartyMemberPosition(gs.CombatScene.GetActiveCombatantIndex())
	targetPosition := references.Down.GetNewPositionInDirection(&activePosition)
	require.NotNil(t, targetPosition)
	gs.CombatScene.AddObject(NewCombatChest(*targetPosition, 1, false))
	mock.Reset()

	result := gs.ActionOpenCombatMap(references.Down)
	assert.True(t, result.Handled)
	assert.Empty(t, result.RevealLines)
	_, chestFound := gs.CombatScene.objectAtMatching(*targetPosition, CombatObject.IsChest)
	assert.False(t, chestFound)
	assert.Contains(t, mock.Messages, "Chest empty!")
	assert.Contains(t, mock.SoundEffectsPlayed, SoundOpenDoor)
}

func TestActionJimmyCombatMapUnlocksChestWithoutTriggeringTrap(t *testing.T) {
	gs, mock := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	activeIndex := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIndex)
	character.Dexterity = 99
	gs.PartyState.Inventory.Provisions.Keys.Set(1)
	position := references.Down.GetNewPositionInDirection(
		&gs.CombatScene.PartyPositions[activeIndex],
	)
	require.NotNil(t, position)
	gs.CombatScene.AddObject(NewCombatChest(*position, 1, true))
	var feedback string
	gs.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) { feedback = message }
	mock.Reset()

	assert.True(t, gs.ActionJimmyCombatMap(references.Down))
	chest, found := gs.CombatScene.objectAtMatching(*position, CombatObject.IsChest)
	require.True(t, found)
	assert.False(t, chest.IsLockedAndTrapped())
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.Provisions.Keys.Get())
	assert.NotContains(t, mock.Messages, "Trapped!")
	assert.Equal(t, chestDisarmedMessage, feedback)
}

func TestActionJimmyCombatMapFailureConsumesKeyAndLeavesChestLocked(t *testing.T) {
	gs, mock := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	activeIndex := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIndex)
	character.Dexterity = 1
	gs.PartyState.Inventory.Provisions.Keys.Set(1)
	position := references.Down.GetNewPositionInDirection(
		&gs.CombatScene.PartyPositions[activeIndex],
	)
	require.NotNil(t, position)
	gs.CombatScene.AddObject(NewCombatChest(*position, 127, true))
	var feedback string
	gs.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) { feedback = message }
	mock.Reset()

	assert.True(t, gs.ActionJimmyCombatMap(references.Down), "failed attempt is handled")
	chest, found := gs.CombatScene.objectAtMatching(*position, CombatObject.IsChest)
	require.True(t, found)
	assert.True(t, chest.IsLockedAndTrapped())
	assert.Zero(t, gs.PartyState.Inventory.Provisions.Keys.Get())
	assert.Equal(t, "Key broke!", feedback)
	assert.NotContains(t, mock.Messages, "Trapped!")
}

func TestActionJimmyCombatMapWithoutKeysDoesNotChangeChest(t *testing.T) {
	gs, mock := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	activeIndex := gs.CombatScene.GetActiveCombatantIndex()
	position := references.Down.GetNewPositionInDirection(
		&gs.CombatScene.PartyPositions[activeIndex],
	)
	require.NotNil(t, position)
	gs.CombatScene.AddObject(NewCombatChest(*position, 10, true))
	gs.PartyState.Inventory.Provisions.Keys.Set(0)
	var feedback string
	gs.SystemCallbacks.UI.ShowActionFeedback = func(message string, _ bool) { feedback = message }
	mock.Reset()

	assert.True(t, gs.ActionJimmyCombatMap(references.Down), "missing keys is handled")
	chest, found := gs.CombatScene.objectAtMatching(*position, CombatObject.IsChest)
	require.True(t, found)
	assert.True(t, chest.IsLockedAndTrapped())
	assert.Equal(t, "No keys!", feedback)
}

func TestCombatChestAcidDeathSynchronizesCombatState(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	activeIndex := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIndex)
	character.SetHp(1)
	character.SetStatus(party_state.Good)

	seed := firstCombatTrapSeed(gs, TrapAcid)
	gs.SetRandomSeed(seed)
	assert.Equal(t, TrapAcid, gs.triggerCombatChestTrap(activeIndex).Type)

	assert.True(t, character.IsDead())
	assert.False(t, gs.CombatScene.IsPartyMemberActive(activeIndex))
	deadPC, found := gs.CombatScene.ObjectAt(gs.CombatScene.GetPartyMemberPosition(activeIndex))
	require.True(t, found)
	assert.Equal(t, indexes.SpriteIndex(indexes.DeadBody), deadPC.Sprite())
	assert.False(t, deadPC.IsCorpse(), "dead PCs must not use monster-corpse Search behavior")
}

func TestCombatChestPoisonKeepsCombatantActive(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	activeIndex := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIndex)
	character.SetStatus(party_state.Good)

	seed := firstCombatTrapSeed(gs, TrapPoison)
	gs.SetRandomSeed(seed)
	assert.Equal(t, TrapPoison, gs.triggerCombatChestTrap(activeIndex).Type)

	assert.True(t, character.IsPoisoned())
	assert.True(t, gs.CombatScene.IsPartyMemberActive(activeIndex))
}

func TestActionOpenCombatMapTriggersLockedChestTrap(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	activeIndex := gs.CombatScene.GetActiveCombatantIndex()
	activePosition := gs.CombatScene.GetPartyMemberPosition(activeIndex)
	targetPosition := references.Down.GetNewPositionInDirection(&activePosition)
	require.NotNil(t, targetPosition)
	gs.CombatScene.AddObject(NewCombatChest(*targetPosition, 1, true))
	character := gs.PartyState.GetCharacter(activeIndex)
	character.SetStatus(party_state.Good)
	seed := firstCombatTrapSeed(gs, TrapPoison)
	gs.SetRandomSeed(seed)

	assert.True(t, gs.ActionOpenCombatMap(references.Down).Handled)

	assert.True(t, character.IsPoisoned())
	_, chestFound := gs.CombatScene.objectAtMatching(*targetPosition, CombatObject.IsChest)
	assert.False(t, chestFound)
}

func TestGenerateCombatChestContentsCreatesCollectibleObjects(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 169)
	position := references.Position{X: 5, Y: 5}
	revealLines := gs.generateCombatChestContents(position, combatTreasureDiceSides)

	assert.NotEmpty(t, revealLines)
	assert.Equal(t, len(revealLines), gs.CombatScene.ObjectCount())
	for _, object := range gs.CombatScene.objects {
		_, looseItem := object.LooseItem()
		assert.True(t, object.IsChest() || looseItem)
		assert.Equal(t, position, object.Position())
	}
	for _, line := range revealLines {
		assert.Contains(t, []string{
			"a chest!",
			"a sack of gold!",
			"a potion!",
			"a scroll!",
			"a ring of keys!",
			"a gem!",
			"some torches!",
			"some food!",
			"a helm!",
			"a shield!",
			"some armour!",
			"a weapon!",
			"a ring!",
			"an amulet!",
		}, line)
	}
}

func TestCombatChestRevealLineUsesOriginalWording(t *testing.T) {
	item := func(item references.Item) references.ChestLoot {
		return references.ChestLoot{Item: references.ItemAndQuantity{Item: item, Quantity: 1}}
	}
	testCases := []struct {
		entry    references.ChestLoot
		expected string
	}{
		{entry: references.ChestLoot{ChestValue: 1}, expected: "a chest!"},
		{entry: item(references.Gold), expected: "a sack of gold!"},
		{entry: item(references.Potion(0)), expected: "a potion!"},
		{entry: item(references.Scroll(0)), expected: "a scroll!"},
		{entry: item(references.Key), expected: "a ring of keys!"},
		{entry: item(references.Gem), expected: "a gem!"},
		{entry: item(references.Torches), expected: "some torches!"},
		{entry: item(references.Food), expected: "some food!"},
		{entry: item(references.LeatherHelm), expected: "a helm!"},
		{entry: item(references.SmallShield), expected: "a shield!"},
		{entry: item(references.ClothArmour), expected: "some armour!"},
		{entry: item(references.Dagger), expected: "a weapon!"},
		{entry: item(references.RingInvisibility), expected: "a ring!"},
		{entry: item(references.AmuletOfTurning), expected: "an amulet!"},
	}

	for _, test := range testCases {
		assert.Equal(t, test.expected, combatChestRevealLine(test.entry))
	}
}

func TestCombatChestGenerationConsumesRNGButAnnouncesOnlyAdmittedLoot(t *testing.T) {
	newState := func(full bool) *GameState {
		state := &GameState{CombatScene: &CombatState{}}
		state.SetRandomSeed(169)
		state.GameReferences = &references.GameReferences{
			InventoryItemReferences: references.NewInventoryItemsReferences(),
		}
		if full {
			for range sourceCombatQueueSlots {
				require.True(t, state.CombatScene.AddObject(NewCombatLooseItem(
					references.Position{X: 1, Y: 1}, references.Food, 1, indexes.ItemFood,
				)))
			}
		}

		return state
	}
	position := references.Position{X: 5, Y: 5}
	admitted := newState(false)
	full := newState(true)

	admittedLines := admitted.generateCombatChestContents(position, combatTreasureDiceSides)
	fullLines := full.generateCombatChestContents(position, combatTreasureDiceSides)

	assert.NotEmpty(t, admittedLines)
	assert.Empty(t, fullLines)
	assert.Equal(t, admitted.GetRNG().Calls(), full.GetRNG().Calls(),
		"capacity exhaustion must not skip required selection or value draws")
	assert.Equal(t, admitted.GetRNG().Intn(1000), full.GetRNG().Intn(1000),
		"both generation paths must leave the deterministic RNG at the same continuation")
}

func firstCombatTrapSeed(gs *GameState, wanted TrapType) uint64 {
	for seed := uint64(1); ; seed++ {
		gs.SetRandomSeed(seed)
		rolled := TrapType(gs.RandomIntInRange(int(TrapAcid), int(TrapPoison)))
		if rolled == wanted {
			return seed
		}
	}
}
