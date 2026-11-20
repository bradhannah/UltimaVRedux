package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestCurrentZStatsDossierIncludesCompleteZeroQuantityCatalogs(t *testing.T) {
	state := newZStatsTestState(newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 1, 0))

	dossier := state.CurrentZStatsDossier()

	require.Len(t, dossier.Armory, 48)
	for id, item := range dossier.Armory {
		assert.Equal(t, id, item.ID)
		assert.Zero(t, item.Available)
		assert.Zero(t, item.Readied)
		assert.NotEmpty(t, item.Name)
		assert.NotZero(t, item.SpriteIndex)
	}
	require.Len(t, dossier.Magic.Spells, 45)
	for _, hidden := range []references.Spell{references.AnYlem, references.InXenMani, references.RelXenBet} {
		assert.Nil(t, zStatsSpellByID(dossier.Magic.Spells, hidden.ID()))
	}
	inLor := requireZStatsSpell(t, dossier.Magic.Spells, references.InLor)
	assert.Equal(t, state.GameReferences.InventoryItemReferences.GetReferenceByItem(references.InLor).ItemSprite, inLor.SpriteIndex)
	assert.Equal(t, "The whole party", inLor.Targeting)
	assert.True(t, inLor.Unaimed, "In Lor needs no aim, so its Target line says who it falls on")
	assert.Equal(t, "Outside battle", inLor.Contexts)
	assert.Equal(t, string(SpellFormulaConfirmed), inLor.FormulaState)
	assert.Equal(t, []string{"Sulfurous Ash"}, inLor.Reagents)
	assertCompleteZeroZStatsInventory(t, dossier.Magic.Reagents)
	assertCompleteZeroZStatsInventory(t, dossier.Magic.Potions)
	assertCompleteZeroZStatsInventory(t, dossier.Magic.Scrolls)
	require.Equal(t, []string{"Food", "Gold", "Keys", "Gems", "Torches", "Skull Keys"}, zStatsProvisionNames(dossier.Provisions))
	expectedDescriptionTerms := map[string][]string{
		"Food":       {"three times a day", "starves"},
		"Gold":       {"goods", "payments"},
		"Keys":       {"Jimmy", "failure breaks one"},
		"Gems":       {"from above", "dungeon level", "one gem"},
		"Torches":    {"Lights the dark", "less time in a dungeon"},
		"Skull Keys": {"In Ex Por", "magically locked door"},
	}
	for _, provision := range dossier.Provisions {
		assert.Zero(t, provision.Quantity)
		assert.NotEmpty(t, provision.Description)
		for _, term := range expectedDescriptionTerms[provision.Name] {
			assert.Contains(t, provision.Description, term, provision.Name)
		}
	}
	assert.Empty(t, dossier.Items)
	assert.Zero(t, dossier.UnreadHiddenSpellCount)
}

func TestCurrentZStatsDossierHiddenSpellVisibilityAndUnreadCount(t *testing.T) {
	state := newZStatsTestState(newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 8, 6400))
	state.PartyState.GetCharacter(0).SetMp(30)
	for _, spell := range []references.Spell{references.AnYlem, references.InXenMani, references.RelXenBet} {
		state.PartyState.Inventory.Spells.Set(spell, 1)
	}
	assert.Len(t, state.CurrentZStatsDossier().Magic.Spells, 45, "prepared hidden spells are not knowledge")

	state.DateTime = datetime.UltimaDate{Year: 139, Month: 4, Day: 12, Hour: 9, Minute: 30}
	require.True(t, state.DiscoverSpellKnowledge(references.AnYlem, DialogueKnowledgeSource("Mariah")))
	require.True(t, state.DiscoverSpellKnowledge(references.InXenMani, DialogueKnowledgeSource("Toshi")))
	require.True(t, state.DiscoverSpellKnowledge(references.RelXenBet, ExperimentationKnowledgeSource()))

	dossier := state.CurrentZStatsDossier()
	require.Len(t, dossier.Magic.Spells, references.TotalSpellCount)
	for id, spell := range dossier.Magic.Spells {
		assert.Equal(t, id, spell.ID)
	}
	assert.Equal(t, 3, dossier.UnreadHiddenSpellCount)
	anYlem := requireZStatsSpell(t, dossier.Magic.Spells, references.AnYlem)
	assert.True(t, anYlem.Hidden)
	assert.True(t, anYlem.Unread)
	assert.Equal(t, string(SpellFormulaUncertain), anYlem.FormulaState)
	assert.Equal(t, "Mariah", anYlem.LearningSource)
	assert.Equal(t, "4-12-139", anYlem.LearningDate)
	require.Len(t, anYlem.ReagentCandidates, 2)
	inXenMani := requireZStatsSpell(t, dossier.Magic.Spells, references.InXenMani)
	assert.Equal(t, string(SpellFormulaConfirmed), inXenMani.FormulaState)
	assert.Equal(t, []string{"Ginseng", "Garlic", "Mandrake Root"}, inXenMani.Reagents)

}

func TestCurrentZStatsDossierStartsOnActingCombatPartyMember(t *testing.T) {
	state := newZStatsTestState(
		newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 4, 500),
		newZStatsTestCharacter("Iolo", party_state.Bard, party_state.Good, 4, 500),
	)
	state.activePlayerIndex = 0
	state.CombatScene = &CombatState{
		CurrentTurnIndex:       1,
		CurrentTurnIdentity:    CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1},
		currentTurnIdentitySet: true,
	}

	assert.Equal(t, 1, state.CurrentZStatsDossier().InitialCharacterIndex)

	state.CombatScene.CurrentTurnIdentity = CombatTurnIdentity{IsPartyMember: false, EntityIndex: 0}
	assert.Equal(t, 0, state.CurrentZStatsDossier().InitialCharacterIndex)

	state.activePlayerIndex = -1
	assert.Equal(t, 0, state.CurrentZStatsDossier().InitialCharacterIndex)
}

func TestCurrentZStatsDossierCountsReadiedArmoryAcrossAllActiveMembers(t *testing.T) {
	avatar := newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 5, 900)
	deadCompanion := newZStatsTestCharacter("Geoffrey", party_state.Fighter, party_state.Dead, 4, 500)
	avatar.SetEquippedItem(references.SlotWeapon, references.LongSword)
	deadCompanion.SetEquippedItem(references.SlotWeapon, references.LongSword)
	state := newZStatsTestState(avatar, deadCompanion)
	state.PartyState.Inventory.Equipment.Set(references.LongSword, 4)

	dossier := state.CurrentZStatsDossier()
	longSword := dossier.Armory[references.LongSword.ID()]
	assert.Equal(t, uint16(4), longSword.Available)
	assert.Equal(t, uint16(2), longSword.Readied)
	require.Len(t, dossier.Characters, 2)
	assert.Equal(t, "Dead", dossier.Characters[1].Status)
	assert.Equal(t, references.LongSword.GetAttackValue(), dossier.Characters[0].EquipmentAttack)
	require.Len(t, dossier.Characters[0].Equipped, 6)
	assert.Equal(t, []string{"Helm", "Armour", "Hand 1", "Hand 2", "Ring", "Amulet"}, zStatsSlotNames(dossier.Characters[0]))
}

func TestCurrentZStatsDossierProjectsEveryCharacterStatus(t *testing.T) {
	state := newZStatsTestState(
		newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 4, 500),
		newZStatsTestCharacter("Iolo", party_state.Bard, party_state.Poisoned, 4, 500),
		newZStatsTestCharacter("Shamino", party_state.Fighter, party_state.Sleep, 4, 500),
		newZStatsTestCharacter("Mariah", party_state.Wizard, party_state.Charmed, 4, 500),
		newZStatsTestCharacter("Geoffrey", party_state.Fighter, party_state.Dead, 4, 500),
	)

	characters := state.CurrentZStatsDossier().Characters
	require.Len(t, characters, 5)
	assert.Equal(t, []string{"Good", "Poisoned", "Sleep", "Charmed", "Dead"}, zStatsStatusNames(characters))
	assert.Equal(t, "Male", characters[0].Gender)
	assert.Equal(t, "Avatar", characters[0].Class)
	assert.Equal(t, state.PartyState.GetCharacter(0).GetKeySpriteIndex(), characters[0].SpriteIndex)
}

func TestCurrentZStatsDossierCalculatesNextLevelProgressAndReadiness(t *testing.T) {
	state := newZStatsTestState(
		newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 2, 150),
		newZStatsTestCharacter("Shamino", party_state.Fighter, party_state.Good, 2, 250),
	)

	characters := state.CurrentZStatsDossier().Characters
	require.Len(t, characters, 2)
	assert.Equal(t, 200, characters[0].NextLevelExperience)
	assert.Equal(t, 50, characters[0].ExperienceIntoLevel)
	assert.Equal(t, 100, characters[0].ExperienceForNextLevel)
	assert.Equal(t, 50, characters[0].ExperienceToNextLevel)
	assert.InDelta(t, 0.5, characters[0].NextLevelProgress, 0.0001)
	assert.False(t, characters[0].ReadyToAdvance)
	assert.Equal(t, 2, characters[0].HighestUsableSpellCircle)
	assert.True(t, characters[0].CanCast)

	assert.Zero(t, characters[1].ExperienceToNextLevel)
	assert.InDelta(t, 1, characters[1].NextLevelProgress, 0.0001)
	assert.True(t, characters[1].ReadyToAdvance)
	assert.Zero(t, characters[1].HighestUsableSpellCircle)
	assert.False(t, characters[1].CanCast)
}

func TestCurrentZStatsDossierIsDetached(t *testing.T) {
	avatar := newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 8, 6400)
	avatar.SetEquippedItem(references.SlotWeapon, references.MagicAxe)
	state := newZStatsTestState(avatar)
	state.PartyState.Inventory.Equipment.Set(references.MagicAxe, 2)
	state.PartyState.Inventory.Provisions.Food.Set(321)
	state.PartyState.Inventory.Gold.Set(654)
	state.PartyState.Inventory.Provisions.Keys.Set(7)
	state.PartyState.Inventory.Provisions.Gems.Set(8)
	state.PartyState.Inventory.Provisions.Torches.Set(9)
	state.PartyState.Inventory.Provisions.SkullKeys.Set(2)
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
	state.DateTime = datetime.UltimaDate{Year: 139, Month: 1, Day: 1}
	require.True(t, state.DiscoverSpellKnowledge(references.AnYlem, DialogueKnowledgeSource("Mariah")))

	dossier := state.CurrentZStatsDossier()
	anYlem := requireZStatsSpell(t, dossier.Magic.Spells, references.AnYlem)
	require.NotEmpty(t, anYlem.ReagentCandidates[0])
	magicAxe := dossier.Armory[references.MagicAxe.ID()]
	require.NotEmpty(t, magicAxe.Abilities)

	dossier.Characters[0].Name = "Changed"
	dossier.Provisions[0].Name = "Changed"
	dossier.Provisions[0].Quantity = 1
	dossier.Characters[0].Equipped[2].Abilities[0] = "Changed"
	dossier.Armory[references.MagicAxe.ID()].Abilities[0] = "Changed"
	anYlem.ReagentCandidates[0][0] = "Changed"
	dossier.Items[0].Name = "Changed"
	state.PartyState.Inventory.Equipment.Set(references.MagicAxe, 7)
	state.PartyState.Inventory.Provisions.Food.Set(999)
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 2)

	assert.Equal(t, "Avatar", state.PartyState.GetCharacter(0).GetNameAsString())
	assert.Equal(t, uint16(999), state.PartyState.Inventory.Provisions.Food.Get())
	assert.Equal(t, uint16(1), dossier.Provisions[0].Quantity)
	assert.Equal(t, uint16(1), dossier.Items[0].Quantity)
	assert.Equal(t, uint16(2), dossier.Armory[references.MagicAxe.ID()].Available)
	fresh := state.CurrentZStatsDossier()
	assert.Equal(t, "Food", fresh.Provisions[0].Name)
	assert.Equal(t, uint16(999), fresh.Provisions[0].Quantity)
	assert.Equal(t, []uint16{999, 654, 7, 8, 9, 2}, zStatsProvisionQuantities(fresh.Provisions))
	assert.Equal(t, "Returns", fresh.Characters[0].Equipped[2].Abilities[0])
	assert.Equal(t, "Returns", fresh.Armory[references.MagicAxe.ID()].Abilities[0])
	assert.NotEqual(t, "Changed", requireZStatsSpell(t, fresh.Magic.Spells, references.AnYlem).ReagentCandidates[0][0])
	assert.NotEqual(t, "Changed", fresh.Items[0].Name)
}

func TestCurrentZStatsDossierDoesNotAcknowledgeUnreadKnowledge(t *testing.T) {
	state := newZStatsTestState(newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 8, 6400))
	require.True(t, state.DiscoverSpellKnowledge(references.AnYlem, DialogueKnowledgeSource("Mariah")))
	before := state.KnowledgeSnapshot()

	dossier := state.CurrentZStatsDossier()

	require.Equal(t, before, state.KnowledgeSnapshot())
	require.True(t, requireZStatsSpell(t, dossier.Magic.Spells, references.AnYlem).Unread)
}

func TestCurrentZStatsDossierProjectsOwnedItems(t *testing.T) {
	poisoned := newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Poisoned, 5, 900)
	state := newZStatsTestState(poisoned)
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
	state.PartyState.Inventory.Moonstones.Set(references.FullMoon, 2)

	dossier := state.CurrentZStatsDossier()
	assert.Equal(t, "Poisoned", dossier.Characters[0].Status)
	require.Len(t, dossier.Items, 2)
	assert.Equal(t, "Quest Item", dossier.Items[0].Category)
	assert.Equal(t, "Moonstone", dossier.Items[1].Category)
	assert.Equal(
		t,
		state.GameReferences.InventoryItemReferences.GetReferenceByItem(references.Grapple).ItemSprite,
		dossier.Items[0].SpriteIndex,
	)
	assert.Equal(
		t,
		state.GameReferences.InventoryItemReferences.GetReferenceByItem(references.FullMoon).ItemSprite,
		dossier.Items[1].SpriteIndex,
	)
	assert.NotEmpty(t, dossier.Items[0].Description)
	assert.NotEmpty(t, dossier.Items[1].Description)
	assert.False(t, dossier.Items[0].MoonPhase.Present)
	assert.Equal(t, ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhaseFull}, dossier.Items[1].MoonPhase)
}

func TestCurrentZStatsDossierMoonstonePhasesUseStableIdentity(t *testing.T) {
	state := newZStatsTestState(newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 5, 900))
	items := state.GameReferences.InventoryItemReferences
	state.PartyState.Inventory.QuestItems.Set(references.Grapple, 1)
	// A quest item can have the same sprite and label as a stone without a phase.
	quest := items.QuestItem[references.Grapple]
	quest.ItemDisplayName = "New Moon Moonstone"
	quest.ItemSprite = items.Moonstone[references.NewMoon].ItemSprite
	items.QuestItem[references.Grapple] = quest
	for _, stone := range references.MoonstoneCatalog() {
		state.PartyState.Inventory.Moonstones.Set(stone, uint16(stone.ID()+1))
		item := items.Moonstone[stone]
		item.ItemName = "UnrelatedInternalName"
		item.ItemDisplayName = fmt.Sprintf("Renamed possession %d", 7-stone.ID())
		items.Moonstone[stone] = item
	}
	dossier := state.CurrentZStatsDossier()
	require.Len(t, dossier.Items, 9)
	require.False(t, dossier.Items[0].MoonPhase.Present)
	for index, item := range dossier.Items[1:] {
		require.Equal(t, index, item.ID)
		require.Equal(t, ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhase(index)}, item.MoonPhase)
		require.Equal(t, uint16(index+1), item.Quantity)
		require.Equal(t, fmt.Sprintf("Renamed possession %d", 7-index), item.Name)
	}
	// Filtered offsets are not phase identities, and phase zero remains present.
	state.PartyState.Inventory.Moonstones.Set(references.CrescentWaxing, 0)
	state.PartyState.Inventory.Moonstones.Set(references.FirstQuarter, 0)
	filtered := state.CurrentZStatsDossier().Items
	require.Len(t, filtered, 7)
	require.Equal(t, ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhaseNew}, filtered[1].MoonPhase)
	require.Equal(t, ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhaseGibbousWaxing}, filtered[2].MoonPhase)
	dossier.Items[1].MoonPhase.Phase = datetime.MoonPhaseFull
	require.Equal(t, datetime.MoonPhaseNew, state.CurrentZStatsDossier().Items[1].MoonPhase.Phase)
}

func TestZStatsProjectsFullCatalogNames(t *testing.T) {
	state := newZStatsTestState(newZStatsTestCharacter("Avatar", party_state.Avatar, party_state.Good, 5, 900))
	state.PartyState.GetCharacter(0).SetEquippedItem(references.SlotRing, references.RingRegeneration)
	state.PartyState.Inventory.QuestItems.Set(references.Amulet, 1)
	state.PartyState.Inventory.Moonstones.Set(references.CrescentWaning, 1)
	dossier := state.CurrentZStatsDossier()
	items := state.GameReferences.InventoryItemReferences
	for _, item := range dossier.Armory {
		require.Equal(t, references.Equipment(item.ID).String(), item.Name)
	}
	require.Equal(t, references.RingRegeneration.String(), dossier.Characters[0].Equipped[4].Name)
	for _, item := range dossier.Magic.Potions {
		require.Equal(t, items.GetReferenceByItem(references.Potion(item.ID)).FullDisplayName(), item.Name)
	}
	for _, item := range dossier.Magic.Scrolls {
		require.Equal(t, items.GetReferenceByItem(references.Scroll(item.ID)).FullDisplayName(), item.Name)
	}
	require.Equal(t, items.GetReferenceByItem(references.Amulet).FullDisplayName(), dossier.Items[0].Name)
	require.Equal(t, items.GetReferenceByItem(references.CrescentWaning).FullDisplayName(), dossier.Items[1].Name)
}

func newZStatsTestState(characters ...*party_state.PlayerCharacter) *GameState {
	gameReferences := &references.GameReferences{
		InventoryItemReferences: references.NewInventoryItemsReferences(),
	}
	state := initBlankGameState(gameReferences, &config.UltimaVConfiguration{}, 11, 11)
	var roster [party_state.NRosterSlots]*party_state.PlayerCharacter
	copy(roster[:], characters)
	state.PartyState = *party_state.NewPartyStateFromRoster(roster)
	state.PartyVehicle = map_units.NewNPCFriendlyVehiceNoVehicle()
	state.DateTime = datetime.UltimaDate{Year: 139, Month: 1, Day: 1}

	return state
}

func newZStatsTestCharacter(
	name string,
	class party_state.CharacterClass,
	status party_state.CharacterStatus,
	level byte,
	experience uint16,
) *party_state.PlayerCharacter {
	character := &party_state.PlayerCharacter{
		Gender:       party_state.Male,
		Class:        class,
		Strength:     20,
		Dexterity:    18,
		Intelligence: 16,
		Exp:          experience,
		Level:        level,
		PartyMap:     party_state.PartyMapActive,
	}
	copy(character.Name[:], name)
	character.SetMaxHp(uint16(level) * 30)
	character.SetHp(character.GetMaxHp())
	character.SetStatus(status)
	if status == party_state.Dead {
		character.MarkAsDead()
	}
	character.SetMp(12)
	for _, slot := range references.EquipmentSlots() {
		character.SetEquippedItem(slot, references.NoEquipment)
	}

	return character
}

func assertCompleteZeroZStatsInventory(t *testing.T, items []ZStatsInventoryItem) {
	t.Helper()
	require.Len(t, items, 8)
	for id, item := range items {
		assert.Equal(t, id, item.ID)
		assert.Zero(t, item.Quantity)
		assert.NotEmpty(t, item.Name)
		assert.NotEmpty(t, item.Description)
		assert.NotZero(t, item.SpriteIndex)
	}
}

func requireZStatsSpell(t *testing.T, spells []ZStatsSpell, spell references.Spell) *ZStatsSpell {
	t.Helper()
	result := zStatsSpellByID(spells, spell.ID())
	require.NotNil(t, result)

	return result
}

func zStatsSpellByID(spells []ZStatsSpell, id int) *ZStatsSpell {
	for index := range spells {
		if spells[index].ID == id {
			return &spells[index]
		}
	}

	return nil
}

func zStatsSlotNames(character ZStatsCharacter) []string {
	names := make([]string, 0, len(character.Equipped))
	for _, item := range character.Equipped {
		names = append(names, item.Slot)
	}

	return names
}

func zStatsStatusNames(characters []ZStatsCharacter) []string {
	names := make([]string, 0, len(characters))
	for _, character := range characters {
		names = append(names, character.Status)
	}

	return names
}

func zStatsProvisionNames(provisions []ZStatsProvision) []string {
	result := make([]string, 0, len(provisions))
	for _, provision := range provisions {
		result = append(result, provision.Name)
	}

	return result
}

func zStatsProvisionQuantities(provisions []ZStatsProvision) []uint16 {
	result := make([]uint16, 0, len(provisions))
	for _, provision := range provisions {
		result = append(result, provision.Quantity)
	}

	return result
}
