package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestGetUsableItemsForContextSeparatesCombatAndFieldScrolls(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Potions.Set(references.Purple, 1)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollVasLor, 1)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollRelHur, 1)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollKalXenCorp, 1)

	fieldItems := state.GetUsableItemsForContext(UsageField)
	combatItems := state.GetUsableItemsForContext(UsageCombat)

	assert.True(t, usableItemsContain(fieldItems, references.Purple))
	assert.True(t, usableItemsContain(combatItems, references.Purple))
	assert.True(t, usableItemsContain(fieldItems, references.ScrollVasLor))
	assert.True(t, usableItemsContain(combatItems, references.ScrollVasLor))
	assert.True(t, usableItemsContain(fieldItems, references.ScrollRelHur))
	assert.NotEmpty(t, requireUsableItem(t, combatItems, references.ScrollRelHur).UnavailableReason)
	assert.NotEmpty(t, requireUsableItem(t, fieldItems, references.ScrollKalXenCorp).UnavailableReason)
	assert.True(t, usableItemsContain(combatItems, references.ScrollKalXenCorp))
}

func TestGetUsableItemsForContextIncludesAnytimeQuestItemsInCombat(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.QuestItems.Set(references.Amulet, 1)
	state.PartyState.Inventory.QuestItems.Set(references.Crown, 1)
	state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	state.PartyState.Inventory.QuestItems.Set(references.HMSCape, 1)

	fieldItems := state.GetUsableItemsForContext(UsageField)
	combatItems := state.GetUsableItemsForContext(UsageCombat)

	assert.True(t, usableItemsContain(fieldItems, references.Amulet))
	assert.True(t, usableItemsContain(fieldItems, references.Crown))
	assert.True(t, usableItemsContain(fieldItems, references.Sceptre))
	assert.Empty(t, requireUsableItem(t, combatItems, references.Amulet).UnavailableReason)
	assert.Empty(t, requireUsableItem(t, combatItems, references.Crown).UnavailableReason)
	assert.True(t, usableItemsContain(combatItems, references.Sceptre))
	assert.True(t, usableItemsContain(fieldItems, references.HMSCape))
	assert.True(t, usableItemsContain(combatItems, references.HMSCape))
}

func TestHMSCapeUseIsAvailableAnywhereAndActionDefersMutation(t *testing.T) {
	locations := []struct {
		name    string
		context UsageContext
		place   references.Location
	}{
		{name: "field", context: UsageField, place: references.Britain},
		{name: "combat", context: UsageCombat, place: references.Britain},
		{name: "dungeon", context: UsageDungeon, place: references.Deceit},
	}

	for _, test := range locations {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := NewIntegrationTestBuilder(t).
				WithLocation(test.place).
				WithSystemCallbacks().
				Build()
			state.PartyState.Inventory.QuestItems.Set(references.HMSCape, 1)
			item := requireUsableItem(t, state.GetUsableItemsForContext(test.context), references.HMSCape)

			require.True(t, state.UseItemWithCompletion(item, func() {}))

			assert.NotNil(t, callbacks.HMSCapeUseCommit)
			assert.False(t, state.IsHMSCapeActivated(), "the action must leave mutation to confirmed authority")
			assert.True(t, item.AvailableNow)
			assert.Equal(t, "Consult the plans.", item.EffectSummary)
		})
	}
}

func TestGetUsableItemsForContextDungeonIncludesSourceBackedItems(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Deceit).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Potions.Set(references.Yellow, 1)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollVasLor, 1)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInQuasWis, 1)
	state.PartyState.Inventory.QuestItems.Set(references.Amulet, 1)
	state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	state.PartyState.Inventory.QuestItems.Set(references.PocketWatch, 0)

	items := state.GetUsableItemsForContext(UsageDungeon)

	assert.Len(t, items, 5)
	assert.True(t, usableItemsContain(items, references.ScrollInQuasWis))
	assert.True(t, usableItemsContain(items, references.Sceptre))
	assert.True(t, usableItemsContain(items, references.Yellow))
	assert.True(t, usableItemsContain(items, references.ScrollVasLor))
	assert.True(t, usableItemsContain(items, references.Amulet))
	for _, item := range items {
		assert.NotEmpty(t, item.Description)
		assert.Empty(t, item.UnavailableReason)
	}
}

func TestDungeonUseProjectionDescribesPreTargetConsumptionAndWhitePotionRejection(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Deceit).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Potions.Set(references.White, 1)
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInManiCorp, 1)
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)
	state.PartyState.Inventory.QuestItems.Set(references.WoodenBox, 1)

	items := state.GetUsableItemsForContext(UsageDungeon)
	whitePotion := requireUsableItem(t, items, references.White)
	resurrection := requireUsableItem(t, items, references.ScrollInManiCorp)
	shard := requireUsableItem(t, items, references.ShardOfFalsehood)
	woodenBox := requireUsableItem(t, items, references.WoodenBox)

	assert.False(t, whitePotion.AvailableNow)
	assert.Contains(t, whitePotion.Warning, "still be consumed")
	assert.Equal(t, "Single use.", resurrection.Consumption)
	assert.False(t, shard.AvailableNow)
	assert.Equal(t, "You cannot use it here.", shard.UnavailableReason)
	assert.Equal(t, "You cannot use it here.", woodenBox.UnavailableReason)
}

func TestGetUsableItemsForContextProjectsCanonicalNamesKeysAndApplicability(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.QuestItems.Set(references.Crown, 1)
	state.PartyState.Inventory.Potions.Set(references.Yellow, 1)
	state.PartyState.Inventory.Potions.Set(references.Purple, 1)
	state.PartyState.Inventory.Provisions.SkullKeys.Set(2)

	fieldItems := state.GetUsableItemsForContext(UsageField)
	combatItems := state.GetUsableItemsForContext(UsageCombat)
	crown := requireUsableItem(t, fieldItems, references.Crown)
	fieldPurple := requireUsableItem(t, fieldItems, references.Purple)
	fieldYellow := requireUsableItem(t, fieldItems, references.Yellow)
	combatPurple := requireUsableItem(t, combatItems, references.Purple)
	combatYellow := requireUsableItem(t, combatItems, references.Yellow)
	keys := requireUsableItem(t, fieldItems, references.SkullKeys)

	assert.Equal(t, "Crown of Lord British", crown.FullName)
	assert.False(t, fieldPurple.AvailableNow)
	assert.Contains(t, fieldPurple.Warning, "may have no effect")
	assert.Empty(t, fieldYellow.EffectSummary, "the shared description already says what a potion is for")
	assert.True(t, combatPurple.AvailableNow)
	assert.Empty(t, combatYellow.Requirement, "colour does not promise an effect")
	assert.Equal(t, UseItemCategoryKeys, keys.Category)
	assert.Equal(t, "Skull Keys", keys.FullName)
	assert.Equal(t, 2, keys.Quantity)
	assert.True(t, keys.AvailableNow, "Britain is a small-map location")
	assert.Contains(t, keys.Consumption, "Each attempt uses one key")
}

func TestRevalidateUseItemSelectionRejectsRemovedInventory(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Potions.Set(references.Yellow, 1)
	selection := requireUsableItem(t, state.GetUsableItemsForContext(UsageField), references.Yellow)

	current, ok := state.RevalidateUseItemSelection(selection, UsageField)
	require.True(t, ok)
	assert.Equal(t, 1, current.Quantity)

	state.PartyState.Inventory.Potions.Set(references.Yellow, 0)
	_, ok = state.RevalidateUseItemSelection(selection, UsageField)
	assert.False(t, ok)
}

func TestUseItemProjectionLimitsLocationFailureToNonCombatTimeStop(t *testing.T) {
	for _, location := range []references.Location{references.Stonegate, references.PalaceOfBlackthorn} {
		t.Run(location.String()+" normal scroll", func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(location).
				WithSystemCallbacks().
				Build()
			state.PartyState.Inventory.Scrolls.Set(references.ScrollVasLor, 1)

			scroll := requireUsableItem(t, state.GetUsableItemsForContext(UsageField), references.ScrollVasLor)

			assert.True(t, scroll.AvailableNow)
			assert.Empty(t, scroll.Warning)
			assert.Empty(t, scroll.Requirement)
		})
	}

	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Stonegate).
		WithSystemCallbacks().
		Build()
	state.PartyState.Inventory.Scrolls.Set(references.ScrollAnTym, 1)
	timeStop := requireUsableItem(t, state.GetUsableItemsForContext(UsageField), references.ScrollAnTym)
	assert.False(t, timeStop.AvailableNow)
	assert.Equal(t, "It has no effect here, but will still be consumed.", timeStop.Warning)

	for _, origin := range []references.Location{references.Stonegate, references.Doom} {
		t.Run(origin.String()+" combat origin", func(t *testing.T) {
			combatState, _ := NewIntegrationTestBuilder(t).
				WithLocation(references.Britain).
				WithSystemCallbacks().
				Build()
			require.NoError(t, combatState.TransitionToCombatMap(
				0,
				references.BritanniaCombatTerritory,
				CombatEntryNorth,
			))
			combatState.CombatScene.PreCombatPlayerLocation.Location = origin
			for _, scroll := range []references.Scroll{
				references.ScrollVasLor,
				references.ScrollInSanct,
				references.ScrollInAn,
				references.ScrollKalXenCorp,
				references.ScrollAnTym,
			} {
				combatState.PartyState.Inventory.Scrolls.Set(scroll, 1)
				item := requireUsableItem(t, combatState.GetUsableItemsForContext(UsageCombat), scroll)
				assert.True(t, item.AvailableNow, "%v should be available from %v combat", scroll, origin)
				assert.NotContains(t, item.Warning, "no effect here")
			}
		})
	}
}

func TestUseAstronomyNightUsesSourceHourBoundaries(t *testing.T) {
	state := &GameState{}
	tests := []struct {
		hour uint8
		want bool
	}{
		{hour: 0, want: true},
		{hour: 5, want: true},
		{hour: 6, want: false},
		{hour: 18, want: false},
		{hour: 19, want: true},
		{hour: 23, want: true},
	}
	for _, test := range tests {
		state.DateTime.Hour = test.hour
		assert.Equal(t, test.want, state.IsUseAstronomyNight(), "hour %d", test.hour)
	}
}

func TestSpyglassLocationContextsMatchSourceProjection(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		want     bool
	}{
		{name: "Britannia", location: references.BritanniaUnderworld, floor: references.FloorNumber(references.OVERWORLD), want: true},
		{name: "Underworld", location: references.BritanniaUnderworld, floor: references.FloorNumber(references.UNDERWORLD)},
		{name: "above ground small map", location: references.Britain, floor: 0, want: true},
		{name: "small map basement", location: references.Britain, floor: -1},
		{name: "dungeon", location: references.Deceit, floor: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithFloor(test.floor).
				WithSystemCallbacks().
				Build()
			assert.Equal(t, test.want, state.IsSpyglassLocationValid())
		})
	}
}

func TestSextantUsesSourceContextFeedbackAndEncodedYThenX(t *testing.T) {
	t.Run("Britannia night", func(t *testing.T) {
		state, callbacks := NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.OVERWORLD)).
			WithPlayerAt(0x4e, 0xab).
			WithSystemCallbacks().
			Build()
		state.DateTime.Hour = 19
		state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
		callbacks.Reset()

		require.True(t, state.useSextant())
		assert.Equal(t, []string{"Position:", "K'L\", E'O\""}, callbacks.Messages)
	})

	t.Run("day", func(t *testing.T) {
		state, callbacks := NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.OVERWORLD)).
			WithSystemCallbacks().
			Build()
		state.DateTime.Hour = 18
		state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
		callbacks.Reset()

		assert.False(t, state.useSextant())
		assert.Equal(t, []string{"Only at night!"}, callbacks.Messages)
	})

	t.Run("Underworld", func(t *testing.T) {
		state, callbacks := NewIntegrationTestBuilder(t).
			WithLocation(references.BritanniaUnderworld).
			WithFloor(references.FloorNumber(references.UNDERWORLD)).
			WithSystemCallbacks().
			Build()
		state.DateTime.Hour = 0
		state.PartyState.Inventory.QuestItems.Set(references.Sextant, 1)
		callbacks.Reset()

		assert.False(t, state.useSextant())
		assert.Equal(t, []string{"Only outdoors!"}, callbacks.Messages)
	})
}

func usableItemsContain(items []UseableItemInfo, target references.Item) bool {
	for _, item := range items {
		if item.ItemType == target {
			return true
		}
	}

	return false
}

func requireUsableItem(t *testing.T, items []UseableItemInfo, target references.Item) UseableItemInfo {
	t.Helper()
	for _, item := range items {
		if item.ItemType == target {
			return item
		}
	}
	t.Fatalf("usable item %v not found", target)

	return UseableItemInfo{}
}
