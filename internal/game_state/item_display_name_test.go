package game_state

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestUseProjectionFullNamesKeepFamilySuffixExactlyOnce(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	for _, item := range references.PotionCatalog() {
		state.PartyState.Inventory.Potions.Set(item, 1)
	}
	for _, item := range references.ScrollCatalog() {
		state.PartyState.Inventory.Scrolls.Set(item, 1)
	}
	for _, item := range references.MoonstoneCatalog() {
		state.PartyState.Inventory.Moonstones.Set(item, 1)
	}
	for _, item := range []references.QuestItem{references.Carpet, references.WoodenBox, references.HMSCape} {
		state.PartyState.Inventory.QuestItems.Set(item, 1)
	}
	before := state.PartyState.Inventory.LockerSnapshot()
	for _, context := range []UsageContext{UsageField, UsageCombat, UsageDungeon} {
		items := state.GetUsableItemsForContext(context)
		for item, want := range map[references.Item]string{
			references.Carpet: "Magic Carpet", references.WoodenBox: "Sandalwood Box", references.HMSCape: "H.M.S. Cape Plans",
			references.Blue: "Blue Potion", references.White: "White Potion", references.ScrollVasLor: "Vas Lor Scroll",
			references.ScrollKalXenCorp: "Kal Xen Corp Scroll", references.CrescentWaxing: "Crescent Waxing Moonstone",
		} {
			projected := requireUsableItem(t, items, item)
			require.Equal(t, want, projected.Name)
			require.Equal(t, want, projected.FullName)
		}
		for _, projected := range items {
			require.Equal(t, projected.ItemRef.FullDisplayName(), projected.Name)
		}
	}
	require.Equal(t, before, state.PartyState.Inventory.LockerSnapshot())
}

func TestSurfaceGetReportsIdentifiedFamilyNameAfterActualPickup(t *testing.T) {
	for item, name := range map[references.Item]string{
		references.Blue: "Blue Potion", references.ScrollVasLor: "Vas Lor Scroll",
		references.ChaosSword: "Sword of Chaos", references.TwoHHammer: "Two-Handed Hammer",
		references.NightShade: "Nightshade", references.SulfurAsh: "Sulfurous Ash",
	} {
		t.Run(name, func(t *testing.T) {
			state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
				WithFloor(0).WithPlayerAt(50, 50).WithSystemCallbacks().Build()
			switch value := item.(type) {
			case references.Potion:
				state.PartyState.Inventory.Potions.Set(value, 0)
			case references.Scroll:
				state.PartyState.Inventory.Scrolls.Set(value, 0)
			case references.Equipment:
				state.PartyState.Inventory.Equipment.Set(value, 0)
			case references.Reagent:
				state.PartyState.Inventory.Reagent.Set(value, 0)
			}
			before := state.combatPickupQuantity(item)
			position := state.MapState.PlayerLocation.Position.Right()
			require.NoError(t, state.ItemStacksMap.Push(&position, &references.ItemStack{Items: references.ItemStacks{
				{Item: item, Quantity: 1},
			}}))
			callbacks.Reset()
			require.True(t, state.ActionGetLargeMap(references.Right))
			require.Equal(t, before+1, state.combatPickupQuantity(item))
			require.Equal(t, []string{name + "!"}, callbacks.Messages)
			require.False(t, state.ItemStacksMap.HasItemStackAtPosition(&position))
		})
	}
}

func TestCombatPickupReceiptUsesFullNameAndActualQuantity(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	active := state.CombatScene.GetPartyMemberPosition(state.CombatScene.GetActiveCombatantIndex())
	position := references.Down.GetNewPositionInDirection(&active)
	require.NotNil(t, position)
	state.PartyState.Inventory.Potions.Set(references.Blue, 98)
	require.True(t, state.addCombatLooseItem(*position, references.Blue, 3))
	callbacks.Reset()
	result := state.getCombatItem(references.Down, true)
	require.True(t, result.Handled)
	require.Equal(t, "Blue Potion", result.ItemName)
	require.Equal(t, uint16(1), result.Acquired)
	require.Equal(t, "Picked up Blue Potion +1", result.Message)
	require.Equal(t, uint16(99), state.PartyState.Inventory.Potions.Get(references.Blue))
	require.Equal(t, []string{result.Message}, callbacks.Messages)
}

func TestSuccessfulScrollUseReportsFullNameAfterEffectAndConsumption(t *testing.T) {
	state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	state.PartyState.Inventory.Scrolls.Set(references.ScrollInSanct, 1)
	callbacks.Reset()

	require.True(t, state.useScrollWithCompletion(references.ScrollInSanct, nil))

	require.Zero(t, state.PartyState.Inventory.Scrolls.Get(references.ScrollInSanct))
	require.True(t, state.HasActiveDurationEffect(DurationEffectProtection))
	require.Equal(t, []string{"Protection!", "In Sanct Scroll!"}, callbacks.Messages)
	require.Equal(t, []int{1}, callbacks.TimeAdvanced())
}
