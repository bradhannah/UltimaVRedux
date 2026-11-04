package game_state

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCombatDoorSpellsSynchronizeEffectiveTerrainAndOpen(t *testing.T) {
	for _, territory := range []references.CombatMapTerritory{
		references.BritanniaCombatTerritory, references.DungeonCombatTerritory,
	} {
		for _, test := range []struct {
			spell references.Spell
			from  indexes.SpriteIndex
			to    indexes.SpriteIndex
		}{
			{references.InExPor, indexes.MagicLockDoor, indexes.RegularDoor},
			{references.InExPor, indexes.MagicLockDoorWithView, indexes.RegularDoorView},
			{references.AnSanct, indexes.LockedDoor, indexes.RegularDoor},
			{references.AnSanct, indexes.LockedDoorView, indexes.RegularDoorView},
			{references.AnExPor, indexes.RegularDoor, indexes.MagicLockDoor},
			{references.AnExPor, indexes.RegularDoorView, indexes.MagicLockDoorWithView},
			{references.AnExPor, indexes.LockedDoor, indexes.MagicLockDoor},
			{references.AnExPor, indexes.LockedDoorView, indexes.MagicLockDoorWithView},
		} {
			t.Run(fmt.Sprintf("%v/%v/%v", territory, test.spell, test.from), func(t *testing.T) {
				state, source, _ := newSyntheticExactIdentityCastUseState(t)
				mock := NewMockSystemCallbacks(t)
				state.SystemCallbacks = mock.ToSystemCallbacks()
				state.CombatScene.CombatMapRef.Territory = territory
				state.CombatScene.setCombatantPosition(source, references.Position{X: 5, Y: 5})
				target := references.Position{X: 6, Y: 5}
				state.setCombatTerrain(target, test.from)
				// An existing override must not survive a newer committed mutation.
				state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, test.from)
				state.PartyState.Inventory.Spells.Set(test.spell, 2)
				mpBefore := state.PartyState.GetCharacter(source.EntityIndex).GetCurrentMp()
				rngBefore, commitBefore := state.GetRNG().Calls(), state.CastUseCommitSequence()
				result := confirmCombatDoorSpell(t, state, test.spell, references.Right)
				require.True(t, state.StartCombatUtilitySpell(result.Utility))
				require.False(t, state.StartCombatUtilitySpell(result.Utility), "only one effect owner")
				require.True(t, state.ResolveCombatUtilitySpell(result.Utility))
				require.False(t, state.ResolveCombatUtilitySpell(result.Utility))
				assertCombatDoorTerrain(t, state, target, test.to)
				require.Len(t, mock.CombatEvents, 1)
				assert.Equal(t, commitBefore+1, state.CastUseCommitSequence())
				assert.Equal(t, mpBefore-byte(references.GetSpellCircle(test.spell)),
					state.PartyState.GetCharacter(source.EntityIndex).GetCurrentMp())
				assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(test.spell))
				assert.Equal(t, CombatAdjacentSpellTargetIgnored, state.ConfirmCombatAdjacentSpellTarget().Outcome)
				require.True(t, state.ActionOpenCombatMap(references.Right).Handled)
				if test.spell == references.AnExPor {
					assertCombatDoorTerrain(t, state, target, test.to)
					assert.Contains(t, mock.Messages, "Locked!")
				} else {
					assertCombatDoorTerrain(t, state, target, indexes.BrickFloor)
					assert.Contains(t, mock.Messages, "Opened!")
				}
				assert.Equal(t, rngBefore, state.GetRNG().Calls())
			})
		}
	}
}

func TestCombatDoorSpellRejectsRetainedOwnerChangesBeforeAndAfterPayment(t *testing.T) {
	for _, paid := range []bool{false, true} {
		for _, change := range []string{
			"actor", "clone", "same slot", "provider", "character", "later dispatch",
			"encounter", "moved", "map", "terrain map", "layered map",
		} {
			t.Run(fmt.Sprintf("paid=%t/%s", paid, change), func(t *testing.T) {
				state, source, clone := newSyntheticExactIdentityCastUseState(t)
				mock := NewMockSystemCallbacks(t)
				state.SystemCallbacks = mock.ToSystemCallbacks()
				state.CombatScene.setCombatantPosition(source, references.Position{X: 5, Y: 5})
				target := references.Position{X: 6, Y: 5}
				state.setCombatTerrain(target, indexes.MagicLockDoor)
				state.PartyState.Inventory.Spells.Set(references.InExPor, 2)
				require.True(t, state.BeginCombatAdjacentSpellTargeting(CastRequest{
					Spell: references.InExPor, CasterIdx: source.EntityIndex,
				}).Valid)
				state.SetCombatAdjacentSpellDirection(references.Right)
				var result CombatAdjacentSpellTargetResult
				if paid {
					result = state.ConfirmCombatAdjacentSpellTarget()
					require.Equal(t, CombatAdjacentSpellTargetResolved, result.Outcome)
					require.True(t, state.StartCombatUtilitySpell(result.Utility))
				}
				changeCombatKeyWatchOwner(t, state, source, clone, references.SkullKeys, change)
				inventory, rng, sequence := state.PartyState.Inventory.LockerSnapshot(), state.GetRNG().Calls(), state.CombatScene.nextPossessionTurn
				mp := state.PartyState.GetCharacter(source.EntityIndex).GetCurrentMp()
				if paid {
					assert.False(t, state.ResolveCombatUtilitySpell(result.Utility))
					state.FinishCombatUtilitySpell(result.Utility)
					state.FinishCombatUtilitySpell(result.Utility)
				} else {
					assert.Equal(t, CombatAdjacentSpellTargetValidationFailed, state.ConfirmCombatAdjacentSpellTarget().Outcome)
				}
				assert.Equal(t, inventory, state.PartyState.Inventory.LockerSnapshot())
				assert.Equal(t, mp, state.PartyState.GetCharacter(source.EntityIndex).GetCurrentMp())
				assert.Equal(t, rng, state.GetRNG().Calls())
				assert.Equal(t, sequence, state.CombatScene.nextPossessionTurn, "stale finalizer cannot finish an action")
				assert.EqualValues(t, indexes.MagicLockDoor, state.CombatScene.CombatMapRef.GetTileNumber(target))
				assert.Empty(t, mock.CombatEvents)
			})
		}
	}
}

func TestCombatDoorSpellCloneAndChestRetainExactTargets(t *testing.T) {
	state, source, clone := newSyntheticExactIdentityCastUseState(t)
	mock := NewMockSystemCallbacks(t)
	state.SystemCallbacks = mock.ToSystemCallbacks()
	state.CombatScene.setCombatantPosition(source, references.Position{X: 2, Y: 2})
	state.CombatScene.setCombatantPosition(clone, references.Position{X: 7, Y: 7})
	state.CombatScene.CurrentTurnIdentity = clone
	target, decoy := references.Position{X: 7, Y: 6}, references.Position{X: 2, Y: 1}
	state.setCombatTerrain(target, indexes.MagicLockDoorWithView)
	state.setCombatTerrain(decoy, indexes.MagicLockDoor)
	state.PartyState.Inventory.Spells.Set(references.InExPor, 1)
	result := confirmCombatDoorSpell(t, state, references.InExPor, references.Up)
	require.Equal(t, target, result.Target)
	require.True(t, state.StartCombatUtilitySpell(result.Utility))
	require.True(t, state.ResolveCombatUtilitySpell(result.Utility))
	assertCombatDoorTerrain(t, state, target, indexes.RegularDoorView)
	assertCombatDoorTerrain(t, state, decoy, indexes.MagicLockDoor)
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, clone.Slot, mock.CombatEvents[0].Attacker.Slot)

	t.Run("changed chest never disarms another slot", func(t *testing.T) {
		chestState, _, _ := newSyntheticExactIdentityCastUseState(t)
		actor := chestState.CombatScene.GetCurrentTurnIdentity()
		chestState.CombatScene.setCombatantPosition(actor, references.Position{X: 5, Y: 5})
		position := references.Position{X: 6, Y: 5}
		require.True(t, chestState.CombatScene.AddObject(NewCombatChest(position, 10, true)))
		require.True(t, chestState.CombatScene.AddObject(NewCombatChest(position, 20, true)))
		chestState.PartyState.Inventory.Spells.Set(references.AnSanct, 1)
		chestResult := confirmCombatDoorSpell(t, chestState, references.AnSanct, references.Right)
		require.True(t, chestState.StartCombatUtilitySpell(chestResult.Utility))
		require.True(t, chestResult.Utility.hasChest)
		require.Equal(t, 20, chestResult.Utility.chest.TreasureValue(), "An Sanct selects the last matching slot")
		selected, removed := chestState.CombatScene.takeObjectAt(position, CombatObject.IsChest)
		require.True(t, removed)
		require.Equal(t, chestResult.Utility.chest, selected, "remove the retained target, not the first overlapping chest")
		assert.False(t, chestState.ResolveCombatUtilitySpell(chestResult.Utility))
		remaining, found := chestState.CombatScene.ObjectAt(position)
		require.True(t, found)
		assert.Equal(t, 10, remaining.TreasureValue())
		assert.True(t, remaining.IsLockedAndTrapped())
	})

	t.Run("removing another chest preserves the retained target", func(t *testing.T) {
		chestState, _, _ := newSyntheticExactIdentityCastUseState(t)
		actor := chestState.CombatScene.GetCurrentTurnIdentity()
		chestState.CombatScene.setCombatantPosition(actor, references.Position{X: 5, Y: 5})
		position := references.Position{X: 6, Y: 5}
		require.True(t, chestState.CombatScene.AddObject(NewCombatChest(position, 10, true)))
		require.True(t, chestState.CombatScene.AddObject(NewCombatChest(position, 20, true)))
		chestState.PartyState.Inventory.Spells.Set(references.AnSanct, 1)
		chestResult := confirmCombatDoorSpell(t, chestState, references.AnSanct, references.Right)
		require.True(t, chestState.StartCombatUtilitySpell(chestResult.Utility))
		retained := chestResult.Utility.chest
		other, removed := chestState.CombatScene.takeFirstObjectAt(position, CombatObject.IsChest)
		require.True(t, removed)
		require.NotEqual(t, retained.originalObjectSlot, other.originalObjectSlot)
		require.Equal(t, 10, other.TreasureValue())
		require.True(t, chestState.ResolveCombatUtilitySpell(chestResult.Utility))
		remaining, found := chestState.CombatScene.ObjectAt(position)
		require.True(t, found)
		assert.Equal(t, retained.originalObjectSlot, remaining.originalObjectSlot)
		assert.Equal(t, 20, remaining.TreasureValue())
		assert.False(t, remaining.IsLockedAndTrapped())
		assert.Equal(t, retained, chestResult.Utility.chest, "the token holds a value snapshot, not a live slice element")
		assert.True(t, chestResult.Utility.chest.IsLockedAndTrapped())
	})
}

func TestCombatDoorSpellEdgesInvalidSelectionAndCancelAreFree(t *testing.T) {
	for _, origin := range []references.Position{{}, {X: 10}, {Y: 10}, {X: 10, Y: 10}, {X: 5, Y: 5}} {
		t.Run(fmt.Sprint(origin), func(t *testing.T) {
			state, source, _ := newSyntheticExactIdentityCastUseState(t)
			state.CombatScene.setCombatantPosition(source, origin)
			state.PartyState.Inventory.Spells.Set(references.InExPor, 1)
			beforeMP, beforeRNG := state.PartyState.GetCharacter(source.EntityIndex).GetCurrentMp(), state.GetRNG().Calls()
			beforeSequence := state.CastUseCommitSequence()
			require.True(t, state.BeginCombatAdjacentSpellTargeting(CastRequest{
				Spell: references.InExPor, CasterIdx: source.EntityIndex,
			}).Valid)
			for _, direction := range []references.Direction{references.Up, references.Down, references.Left, references.Right, references.Direction(99)} {
				state.SetCombatAdjacentSpellDirection(direction)
				position, _, active := state.CombatAdjacentSpellPreview()
				require.True(t, active)
				assert.True(t, isCombatMapPosition(position))
				assert.NotEqual(t, references.NoneDirection, directionBetween(origin, position), "only a cardinal neighbour")
				assert.Equal(t, CombatAdjacentSpellTargetInvalid, state.ConfirmCombatAdjacentSpellTarget().Outcome)
			}
			// Corrupted/stale coordinates are also rejected before raw map indexing.
			state.CombatScene.AdjacentSpellTargeting.Origin = references.Position{X: -10, Y: -10}
			assert.NotPanics(t, func() {
				assert.Equal(t, CombatAdjacentSpellTargetInvalid, state.ConfirmCombatAdjacentSpellTarget().Outcome)
			})
			assert.Equal(t, CombatAdjacentSpellTargetCancelled, state.CancelCombatAdjacentSpellTargeting())
			assert.Equal(t, beforeMP, state.PartyState.GetCharacter(source.EntityIndex).GetCurrentMp())
			assert.Equal(t, beforeRNG, state.GetRNG().Calls())
			assert.Equal(t, beforeSequence, state.CastUseCommitSequence())
			assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(references.InExPor))
		})
	}
}

func TestCombatDoorSpellRejectsIdenticalChestReplacement(t *testing.T) {
	state, actor, _ := newSyntheticExactIdentityCastUseState(t)
	mock := NewMockSystemCallbacks(t)
	state.SystemCallbacks = mock.ToSystemCallbacks()
	state.CombatScene.setCombatantPosition(actor, references.Position{X: 5, Y: 5})
	position := references.Position{X: 6, Y: 5}
	require.True(t, state.CombatScene.AddObject(NewCombatChest(position, 20, true)))
	state.PartyState.Inventory.Spells.Set(references.AnSanct, 2)
	result := confirmCombatDoorSpell(t, state, references.AnSanct, references.Right)
	require.True(t, state.StartCombatUtilitySpell(result.Utility))
	selected, found := state.CombatScene.takeObjectAt(position, CombatObject.IsChest)
	require.True(t, found)
	require.Equal(t, selected, result.Utility.chest)
	require.True(t, state.CombatScene.AddObject(selected), "readmit the exact removed value")
	replacement, found := state.CombatScene.ObjectAt(position)
	require.True(t, found)
	require.Equal(t, selected.originalObjectSlot, replacement.originalObjectSlot)
	require.Greater(t, replacement.LifetimeID(), selected.LifetimeID())
	sameContents := replacement
	sameContents.lifetimeID = selected.LifetimeID()
	require.Equal(t, selected, sameContents, "only lifetime identity differs")
	mp, rng, sequence := state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentMp(), state.GetRNG().Calls(), state.CastUseCommitSequence()
	assert.False(t, state.ResolveCombatUtilitySpell(result.Utility))
	assert.False(t, state.ResolveCombatUtilitySpell(result.Utility))
	after, found := state.CombatScene.ObjectAt(position)
	require.True(t, found)
	assert.Equal(t, replacement, after)
	assert.True(t, after.IsLockedAndTrapped())
	assert.Empty(t, mock.CombatEvents)
	assert.Equal(t, mp, state.PartyState.GetCharacter(actor.EntityIndex).GetCurrentMp())
	assert.Equal(t, rng, state.GetRNG().Calls())
	assert.Equal(t, sequence, state.CastUseCommitSequence())
	assert.EqualValues(t, 1, state.PartyState.Inventory.Spells.Get(references.AnSanct))
}

func TestCombatJimmyAndOpenReturnDistinctColocatedChestLifetimes(t *testing.T) {
	state, actor, _ := newSyntheticExactIdentityCastUseState(t)
	state.CombatScene.setCombatantPosition(actor, references.Position{X: 5, Y: 5})
	state.PartyState.GetCharacter(actor.EntityIndex).Dexterity = 99
	state.PartyState.Inventory.Provisions.Keys.Set(2)
	position := references.Position{X: 6, Y: 5}
	require.True(t, state.CombatScene.AddObject(NewCombatChest(position, 1, false)))
	first, found := state.CombatScene.ObjectAt(position)
	require.True(t, found)
	require.True(t, state.CombatScene.AddObject(NewCombatChest(position, 1, true)))
	last, found := state.CombatScene.ObjectAt(position)
	require.True(t, found)
	handled, disarmed := state.ActionJimmyCombatMapWithDisarm(references.Right)
	require.True(t, handled)
	require.NotNil(t, disarmed)
	assert.Equal(t, CombatChestIdentity{Position: position, ObjectID: last.LifetimeID()}, *disarmed)
	state.SetRandomSeed(169)
	opened := state.ActionOpenCombatMap(references.Right)
	require.True(t, opened.OpenedChest)
	assert.Equal(t, first.LifetimeID(), opened.ChestID)
	assert.True(t, state.CombatScene.HasChestIdentity(*disarmed))
	state.SetRandomSeed(169)
	opened = state.ActionOpenCombatMap(references.Right)
	require.True(t, opened.OpenedChest)
	assert.Equal(t, disarmed.ObjectID, opened.ChestID)
	assert.Equal(t, disarmed.Position, opened.Position)
	assert.False(t, state.CombatScene.HasChestIdentity(*disarmed))
	assert.EqualValues(t, 2, state.PartyState.Inventory.Provisions.Keys.Get())
}

func TestCombatJimmyAndSkullKeyRemainSynchronizedWithDoorSpells(t *testing.T) {
	for _, windowed := range []bool{false, true} {
		t.Run(fmt.Sprint(windowed), func(t *testing.T) {
			state, source, _ := newSyntheticExactIdentityCastUseState(t)
			state.CombatScene.setCombatantPosition(source, references.Position{X: 5, Y: 5})
			target := references.Position{X: 6, Y: 5}
			locked, regular, magic := indexes.SpriteIndex(indexes.LockedDoor), indexes.SpriteIndex(indexes.RegularDoor), indexes.SpriteIndex(indexes.MagicLockDoor)
			if windowed {
				locked, regular, magic = indexes.LockedDoorView, indexes.RegularDoorView, indexes.MagicLockDoorWithView
			}
			state.setCombatTerrain(target, locked)
			state.PartyState.Inventory.Provisions.Keys.Set(2)
			rng := state.GetRNG().Calls()
			require.True(t, state.ActionJimmyCombatMap(references.Right))
			assertCombatDoorTerrain(t, state, target, regular)
			assert.Equal(t, rng+1, state.GetRNG().Calls(), "Jimmy retains one physical-unlock roll")
			assert.EqualValues(t, 2, state.PartyState.Inventory.Provisions.Keys.Get())
			state.CastCombatAnExPor(source.EntityIndex, target)
			assertCombatDoorTerrain(t, state, target, magic)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &target, magic)
			state.PartyState.Inventory.Provisions.SkullKeys.Set(1)
			keyRNG, commits := state.GetRNG().Calls(), state.CastUseCommitSequence()
			key, _ := state.BeginSkullKeyUse()
			result, paid := state.CommitCombatSkullKeyUse(key, references.Right)
			require.True(t, paid)
			require.Equal(t, SkullKeyUnlockSuccess, result.Result)
			assert.Zero(t, state.PartyState.Inventory.Provisions.SkullKeys.Get())
			assert.Equal(t, keyRNG, state.GetRNG().Calls())
			assert.Equal(t, commits+1, state.CastUseCommitSequence())
			_, paid = state.CommitCombatSkullKeyUse(key, references.Right)
			assert.False(t, paid)
			assert.Equal(t, commits+1, state.CastUseCommitSequence())
			assertCombatDoorTerrain(t, state, target, regular)
			require.True(t, state.ActionOpenCombatMap(references.Right).Handled)
			assertCombatDoorTerrain(t, state, target, indexes.BrickFloor)
			assert.False(t, state.isValidCombatAdjacentSpellTarget(references.AnExPor, target))
		})
	}
}

func confirmCombatDoorSpell(t *testing.T, state *GameState, spell references.Spell, direction references.Direction) CombatAdjacentSpellTargetResult {
	t.Helper()
	require.True(t, state.BeginCombatAdjacentSpellTargeting(CastRequest{
		Spell: spell, CasterIdx: state.CombatScene.GetCurrentTurnIdentity().EntityIndex,
	}).Valid)
	state.SetCombatAdjacentSpellDirection(direction)
	result := state.ConfirmCombatAdjacentSpellTarget()
	require.Equal(t, CombatAdjacentSpellTargetResolved, result.Outcome)
	require.NotNil(t, result.Utility)

	return result
}

func assertCombatDoorTerrain(t *testing.T, state *GameState, position references.Position, want indexes.SpriteIndex) {
	t.Helper()
	assert.Equal(t, want, state.CombatScene.CombatMapRef.GetTileNumber(position))
	assert.Equal(t, want, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapLayer, position))
	assert.Equal(t, want, state.GetCurrentLayeredMap().GetTileTopMapOnlyTile(&position).Index)
	assert.Equal(t, want, state.GetEffectiveTile(&position))
	assert.LessOrEqual(t, state.GetCurrentLayeredMap().SpriteIndexByLayer(map_state.MapOverrideLayer, position), indexes.SpriteIndex(0))
}
