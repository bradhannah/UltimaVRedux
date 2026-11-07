package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestCombatChestUncertainJimmyAndOpenSequences(t *testing.T) {
	for _, armed := range []bool{false, true} {
		for _, disarm := range []bool{false, true} {
			t.Run(fmt.Sprintf("armed=%t/disarm=%t", armed, disarm), func(t *testing.T) {
				state, callbacks := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
				require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
				active := state.CombatScene.GetActiveCombatantIndex()
				actor := state.PartyState.GetCharacter(active)
				actor.Intelligence, actor.Dexterity = 1, 1
				target := references.Down.GetNewPositionInDirection(&state.CombatScene.PartyPositions[active])
				require.NotNil(t, target)
				require.True(t, state.CombatScene.AddObject(NewCombatChest(*target, 30, armed)))
				state.SetRandomSeed(chestSeedForRoll(t, 1))
				require.True(t, state.ActionSearchCombatMap(references.Down))
				selected, found := state.CombatScene.objectAtMatching(*target, CombatObject.IsChest)
				require.True(t, found)
				assert.Equal(t, references.ChestUncertain, selected.ChestAssessment())
				assert.Equal(t, uint64(1), state.GetRNG().Calls())
				actor.Intelligence = 30
				require.True(t, state.ActionSearchCombatMap(references.Down))
				assert.Equal(t, uint64(1), state.GetRNG().Calls())
				state.PartyState.Inventory.Provisions.Keys.Set(2)
				for attempt := range 2 {
					state.SetRandomSeed(chestSeedForRoll(t, 1))
					callbacks.Reset()
					require.True(t, state.ActionJimmyCombatMap(references.Down))
					assert.Equal(t, []string{"Key broke!"}, callbacks.Messages)
					assert.Equal(t, uint16(1-attempt), state.PartyState.Inventory.Provisions.Keys.Get())
					assert.Equal(t, armed, state.CombatScene.liveChest(selected).IsLockedAndTrapped())
					assert.Equal(t, references.ChestUncertain, state.CombatScene.liveChest(selected).ChestAssessment())
				}
				state.SetRandomSeed(655)
				require.True(t, state.ActionJimmyCombatMap(references.Down))
				assert.Zero(t, state.GetRNG().Calls(), "no key means no roll or new knowledge")
				if disarm && armed {
					actor.Dexterity = 30
					state.PartyState.Inventory.Provisions.Keys.Set(1)
					state.SetRandomSeed(chestSeedForRoll(t, 30))
					require.True(t, state.ActionJimmyCombatMap(references.Down))
					assert.Equal(t, references.ChestSafe, state.CombatScene.liveChest(selected).ChestAssessment())
					assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
					calls := state.GetRNG().Calls()
					require.True(t, state.ActionJimmyCombatMap(references.Down))
					require.True(t, state.ActionSearchCombatMap(references.Down))
					assert.Equal(t, calls, state.GetRNG().Calls())
					assert.Equal(t, uint16(1), state.PartyState.Inventory.Provisions.Keys.Get())
				}
				state.SetRandomSeed(firstCombatTrapSeed(state, TrapPoison))
				callbacks.Reset()
				require.True(t, state.ActionOpenCombatMap(references.Down).Handled)
				assert.Equal(t, armed && !disarm, stringIndex(callbacks.Messages, "Trapped!") >= 0)
			})
		}
	}
}

func TestCombatOverlappingChestsMutateExactSelectedSlot(t *testing.T) {
	for _, lowerArmed := range []bool{false, true} {
		for _, magic := range []bool{false, true} {
			t.Run(fmt.Sprintf("lowerArmed=%t/magic=%t", lowerArmed, magic), func(t *testing.T) {
				state, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
				require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
				active := state.CombatScene.GetActiveCombatantIndex()
				actor := state.PartyState.GetCharacter(active)
				actor.Intelligence, actor.Dexterity = 30, 30
				target := references.Down.GetNewPositionInDirection(&state.CombatScene.PartyPositions[active])
				require.NotNil(t, target)
				require.True(t, state.CombatScene.AddObject(NewCombatChest(*target, 30, lowerArmed)))
				lower, _ := state.CombatScene.ObjectAt(*target)
				require.True(t, state.CombatScene.AddObject(NewCombatChest(*target, 1, true)))
				upper, _ := state.CombatScene.ObjectAt(*target)
				state.SetRandomSeed(chestSeedForRoll(t, 30))
				require.True(t, state.ActionSearchCombatMap(references.Down))
				assert.Equal(t, references.ChestTrapped, state.CombatScene.liveChest(upper).ChestAssessment())
				assert.Equal(t, references.ChestUninspected, state.CombatScene.liveChest(lower).ChestAssessment())
				objects := append([]CombatObject(nil), state.CombatScene.objects...)
				calls := state.GetRNG().Calls()
				for _, elapsed := range []int64{0, 999, 1000, 2000, 60000} {
					state.ElapsedMs = elapsed
					sprite, displayedSlot, field := state.CombatScene.combatGroundDisplayAt(*target)
					assert.Equal(t, lower.originalObjectSlot, displayedSlot, "display's lower slot must not redirect Search/Jimmy/An Sanct")
					assert.Equal(t, lower.Sprite(), sprite)
					assert.False(t, field)
					state.CombatPileSprite(*target)
				}
				assert.Equal(t, calls, state.GetRNG().Calls())
				assert.Equal(t, objects, state.CombatScene.objects)
				if magic {
					require.True(t, isCombatAnSanctTarget(state.CombatScene, *target))
					assert.Equal(t, CombatUtilityChestDisarmed, state.CastCombatAnSanct(active, *target))
				} else {
					state.PartyState.Inventory.Provisions.Keys.Set(1)
					require.True(t, state.ActionJimmyCombatMap(references.Down))
				}
				assert.False(t, state.CombatScene.liveChest(upper).IsLockedAndTrapped())
				assert.Equal(t, references.ChestSafe, state.CombatScene.liveChest(upper).ChestAssessment())
				assert.Equal(t, lowerArmed, state.CombatScene.liveChest(lower).IsLockedAndTrapped())
				assert.Equal(t, references.ChestUninspected, state.CombatScene.liveChest(lower).ChestAssessment())
				assert.False(t, isCombatAnSanctTarget(state.CombatScene, *target), "a lower armed chest is not the selected safe chest")
				state.SetRandomSeed(655)
				state.PartyState.Inventory.Provisions.Keys.Set(0)
				require.True(t, state.ActionJimmyCombatMap(references.Down))
				assert.Zero(t, state.GetRNG().Calls())
				opened := state.ActionOpenCombatMap(references.Down)
				require.True(t, opened.Handled)
				assert.Equal(t, lowerArmed, opened.Trap.Type != TrapNone, "Open preserves its own first-slot selection policy and checks physical state")
				assert.Equal(t, references.ChestSafe, state.CombatScene.liveChest(upper).ChestAssessment())
				_, displayedSlot, _ := state.CombatScene.combatGroundDisplayAt(*target)
				assert.Equal(t, upper.originalObjectSlot, displayedSlot, "Open exposes the surviving chest without changing its knowledge")
			})
		}
	}
}

func TestCombatChestReplacementAndNestedLootStartUninspected(t *testing.T) {
	state := &GameState{CombatScene: &CombatState{}}
	state.SetRandomSeed(655)
	position := references.Position{X: 1, Y: 1}
	old := NewCombatChest(position, 30, false)
	old.UnlockChest()
	require.True(t, state.CombatScene.AddObject(old))
	removed, found := state.CombatScene.takeObjectAt(position, CombatObject.IsChest)
	require.True(t, found)
	require.Equal(t, references.ChestSafe, removed.ChestAssessment())
	require.True(t, state.CombatScene.AddObject(NewCombatChest(position, 30, true)))
	replacement, _ := state.CombatScene.ObjectAt(position)
	assert.Equal(t, removed.originalObjectSlot, replacement.originalObjectSlot)
	assert.Equal(t, references.ChestUninspected, replacement.ChestAssessment())
	assert.True(t, state.admitCombatChestLoot(position, references.ChestLoot{ChestValue: 30}))
	nested, _ := state.CombatScene.ObjectAt(position)
	assert.NotEqual(t, replacement.originalObjectSlot, nested.originalObjectSlot)
	assert.Equal(t, references.ChestUninspected, nested.ChestAssessment())
	assert.False(t, nested.IsLockedAndTrapped())
}
