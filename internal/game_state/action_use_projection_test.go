package game_state

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestUseProjectionCarpetAvailableInUnderworld(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithFloor(-1).WithPlayerAt(100, 100).WithSystemCallbacks().Build()
	state.PartyState.Inventory.QuestItems.Set(references.Carpet, 1)
	state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer,
		&state.MapState.PlayerLocation.Position, indexes.Grass)
	for _, item := range state.GetUsableItems() {
		if item.ItemType == references.Carpet {
			require.True(t, item.AvailableNow)
			assert.Empty(t, item.UnavailableReason)
			assert.Empty(t, item.Warning)
			return
		}
	}
	t.Fatal("inventory carpet missing from Use menu")
}

func TestUseProjectionApprovedGuidance(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	for _, shard := range []references.QuestItem{
		references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice,
	} {
		info := state.withUseItemGuidance(UseableItemInfo{ItemType: shard}, UsageField)
		assert.Empty(t, info.Consumption, "browsing must not disclose the destruction ritual")
		assert.Empty(t, info.EffectSummary)
		assert.Empty(t, info.Requirement)
		assert.Empty(t, info.Warning)
		info = state.withUseItemGuidance(info, UsageDungeon)
		assert.False(t, info.AvailableNow)
		assert.Equal(t, "You cannot use it here.", info.UnavailableReason)
	}
	for _, test := range []struct {
		item references.QuestItem
		want string
	}{
		{references.Amulet, "Wear the Amulet."},
		{references.BlackBadge, "Wear the Badge."},
	} {
		info := state.withUseItemGuidance(UseableItemInfo{ItemType: test.item}, UsageField)
		assert.Equal(t, test.want, info.EffectSummary)
	}
	info := state.withUseItemGuidance(UseableItemInfo{ItemType: references.Sceptre}, UsageField)
	assert.Empty(t, info.Requirement, "ownership does not disclose the Sceptre's powers")
	state.addMoonstoneUseGuidance(&info, UsageField)
	assert.Equal(t, "This ground cannot hold a buried moonstone.", info.UnavailableReason)
}

func TestUseOwnershipAndCategoryMatrixAcrossContexts(t *testing.T) {
	for _, test := range []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		context  UsageContext
	}{
		{"town", references.Britain, 0, UsageField},
		{"overworld", references.BritanniaUnderworld, 0, UsageField},
		{"underworld", references.BritanniaUnderworld, -1, UsageField},
		{"dungeon", references.Deceit, 0, UsageDungeon},
		{"combat", references.Britain, 0, UsageCombat},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(test.location).
				WithFloor(test.floor).WithSystemCallbacks().Build()
			if test.context == UsageCombat {
				require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
			}
			expected := make(map[references.Item]int)
			for item := references.Amulet; item <= references.Sextant; item++ {
				quantity := uint16(item) + 1
				state.PartyState.Inventory.QuestItems.Set(item, quantity)
				expected[item] = int(quantity)
			}
			for item := references.Blue; item <= references.White; item++ {
				quantity := uint16(item) + 2
				state.PartyState.Inventory.Potions.Set(item, quantity)
				expected[item] = int(quantity)
			}
			for item := references.ScrollVasLor; item <= references.ScrollAnTym; item++ {
				quantity := uint16(item) + 3
				state.PartyState.Inventory.Scrolls.Set(item, quantity)
				expected[item] = int(quantity)
			}
			for item := references.NewMoon; item <= references.CrescentWaning; item++ {
				quantity := uint16(item) + 4
				state.PartyState.Inventory.Moonstones.Set(item, quantity)
				expected[item] = int(quantity)
			}
			state.PartyState.Inventory.Provisions.SkullKeys.Set(9)
			expected[references.SkullKeys] = 9
			inventory := state.PartyState.Inventory.LockerSnapshot()
			rngCalls := state.GetRNG().Calls()
			sequence := state.CastUseCommitSequence()
			items := state.GetUsableItemsForContext(test.context)
			require.Len(t, items, len(expected))
			seen := make(map[references.Item]int)
			categories := make(map[UseItemCategory]int)
			for _, item := range items {
				require.Contains(t, expected, item.ItemType)
				require.NotContains(t, seen, item.ItemType, "each owned identity appears once")
				seen[item.ItemType] = item.Quantity
				categories[item.Category]++
				require.NotEmpty(t, item.Description)
				assertUseBrowsingDisclosure(t, item)
				if item.UnavailableReason != "" {
					require.False(t, item.AvailableNow)
				}
				if test.context == UsageCombat {
					allowed := combatUseProjectionExpectedAvailable(item.ItemType)
					require.Equal(t, allowed, item.UnavailableReason == "", "combat activation for %v", item.ItemType)
				}
			}
			require.Equal(t, expected, seen)
			require.Equal(t, map[UseItemCategory]int{
				UseItemCategoryQuest: 11, UseItemCategoryTools: 3, UseItemCategoryPotion: 8,
				UseItemCategoryScroll: 8, UseItemCategoryMoonstone: 8, UseItemCategoryKeys: 1,
			}, categories)
			require.Equal(t, inventory, state.PartyState.Inventory.LockerSnapshot())
			require.Equal(t, rngCalls, state.GetRNG().Calls())
			require.Equal(t, sequence, state.CastUseCommitSequence())

			// Use and Z-stats show one description, and it carries no arithmetic.
			dossier := state.CurrentZStatsDossier()
			shared := make(map[string]string)
			for _, entry := range slices.Concat(dossier.Magic.Potions, dossier.Magic.Scrolls) {
				shared[entry.Name] = entry.Description
			}
			for _, provision := range dossier.Provisions {
				shared[provision.Name] = provision.Description
			}
			for _, owned := range dossier.Items {
				shared[owned.Name] = owned.Description
			}
			for _, item := range items {
				require.Equal(t, shared[item.FullName], item.Description, "%v", item.ItemType)
				require.NotRegexp(t, `[0-9]`, item.Description, "%v", item.ItemType)
			}
		})
	}
}

func assertUseBrowsingDisclosure(t *testing.T, item UseableItemInfo) {
	t.Helper()
	copy := strings.ToLower(strings.Join([]string{item.Name, item.FullName, item.Description,
		item.EffectSummary, item.Requirement, item.Consumption, item.Warning,
		item.UnavailableReason, item.ItemRef.ItemDescriptionAttribution}, "\n"))
	for _, technical := range []string{"endgame", "global setting", "effect commits", "active combatant", "small map", "verified dos"} {
		require.NotContains(t, copy, technical, "%v browsing copy", item.ItemType)
	}
	require.NotRegexp(t, `\b[a-z]+-[0-9]{4}\b`, copy, "rule identifiers do not belong in browsing copy")
	// A quest item's copy says what it is. How, where and why to use it is for
	// the player to learn from the people of Britannia.
	switch item.ItemType {
	case references.Crown:
		require.NotContains(t, copy, "suppress")
		require.NotContains(t, copy, "magic")
		require.Contains(t, item.Requirement, "worn")
	case references.Sceptre:
		require.NotContains(t, copy, "field")
		require.NotContains(t, copy, "dispel")
		require.NotContains(t, copy, "barrier")
	case references.Amulet:
		require.NotContains(t, copy, "darkness")
		require.NotContains(t, copy, "moongate")
	case references.BlackBadge:
		require.NotContains(t, copy, "palace")
		require.NotContains(t, copy, "guard")
		require.NotContains(t, copy, "password")
	case references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice:
		require.NotContains(t, copy, "ritual")
		require.NotContains(t, copy, "flame")
		require.NotContains(t, copy, "shadowlord")
		require.Equal(t, "The Book of Lore (Ultima V) and UltimaCodex.com", item.ItemRef.ItemDescriptionAttribution)
	case references.HMSCape:
		require.Equal(t, "H.M.S. Cape Plans", item.Name)
	}
}

func combatUseProjectionExpectedAvailable(item references.Item) bool {
	switch typed := item.(type) {
	case references.Potion:
		return true // White remains a permitted attempt with a warning and substitution.
	case references.Scroll:
		return typed != references.ScrollRelHur && typed != references.ScrollInQuasWis && typed != references.ScrollInManiCorp
	case references.QuestItem:
		return typed == references.Crown || typed == references.Amulet || typed == references.BlackBadge ||
			typed == references.Sceptre || typed == references.HMSCape || typed == references.PocketWatch
	case references.Provision:
		return typed == references.SkullKeys
	default:
		return false
	}
}

func TestKeyAndWatchUseGuidancePreservesContextRestrictions(t *testing.T) {
	for _, test := range []struct {
		name     string
		location references.Location
		floor    references.FloorNumber
		context  UsageContext
		keyReady bool
	}{
		{"town", references.Britain, 0, UsageField, true},
		{"surface", references.BritanniaUnderworld, 0, UsageField, true},
		{"underworld", references.BritanniaUnderworld, -1, UsageField, false},
		{"dungeon exploration", references.Deceit, 0, UsageDungeon, false},
		{"combat", references.Britain, 0, UsageCombat, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).WithLocation(test.location).
				WithFloor(test.floor).WithSystemCallbacks().Build()
			if test.context == UsageCombat {
				require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
			}
			state.PartyState.Inventory.Provisions.SkullKeys.Set(2)
			state.PartyState.Inventory.QuestItems.Set(references.PocketWatch, 1)
			items := state.GetUsableItemsForContext(test.context)
			key := requireUsableItem(t, items, references.SkullKeys)
			watch := requireUsableItem(t, items, references.PocketWatch)
			require.Equal(t, test.keyReady, key.UnavailableReason == "")
			require.Empty(t, watch.UnavailableReason)
			_, valid := state.RevalidateUseItemSelection(key, test.context)
			require.Equal(t, test.keyReady, valid)
			_, valid = state.RevalidateUseItemSelection(watch, test.context)
			require.True(t, valid)
			if test.context == UsageCombat {
				require.Contains(t, key.Consumption, "Each attempt uses one key and your turn")
				require.Equal(t, "Uses your turn.", watch.Consumption)
			}
		})
	}
}

func TestUseWarningsDoNotBecomeActivationRestrictions(t *testing.T) {
	for _, context := range []UsageContext{UsageField, UsageDungeon, UsageCombat} {
		location := references.Britain
		if context == UsageDungeon {
			location = references.Deceit
		}
		state, _ := NewIntegrationTestBuilder(t).WithLocation(location).WithSystemCallbacks().Build()
		if context == UsageCombat {
			require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
		}
		for _, potion := range []references.Potion{references.Purple, references.Black, references.White} {
			state.PartyState.Inventory.Potions.Set(potion, 2)
		}
		items := state.GetUsableItemsForContext(context)
		for _, potion := range []references.Potion{references.Purple, references.Black, references.White} {
			item := requireUsableItem(t, items, potion)
			require.Empty(t, item.UnavailableReason, "potion substitution remains possible in every context")
			require.Equal(t, "Single use.", item.Consumption)
			ineffective := (potion == references.White && context != UsageField) ||
				(potion != references.White && context != UsageCombat)
			require.Equal(t, !ineffective, item.AvailableNow)
			if ineffective {
				require.Contains(t, item.Warning, "spent all the same")
			}
		}
	}
}
