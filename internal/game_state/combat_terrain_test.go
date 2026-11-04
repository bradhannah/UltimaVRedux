package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCombatTerrainConcealsObjectsUntilTriggerReveal(t *testing.T) {
	trigger := references.Position{X: 6, Y: 5}
	hidden := references.Position{X: 7, Y: 5}
	state, callbacks := newSyntheticDungeonTriggerCombatState(t, indexedDungeonTrigger{
		index: 0,
		trigger: references.DungeonCombatMapTrigger{
			ReplacementTile: indexes.BrickFloor,
			Trigger:         trigger,
			Destinations:    [2]references.Position{hidden, trigger},
		},
	})
	layer := state.GetCurrentLayeredMap()
	layer.SetTileByLayer(map_state.MapLayer, &hidden, indexes.BlackSquare)
	state.CombatScene.AddObject(NewCombatChest(hidden, 10, false))

	object, found := state.CombatScene.ObjectAt(hidden)
	require.True(t, found)
	assert.Equal(t, indexes.SpriteIndex(indexes.Chest), object.Sprite())
	assert.False(t, state.CombatTerrainAllowsObjectDisplay(hidden))
	assert.Equal(t, indexes.SpriteIndex(indexes.BlackSquare), state.GetEffectiveTile(&hidden))

	require.True(t, state.CheckTrap(&trigger))
	assert.True(t, state.CombatTerrainAllowsObjectDisplay(hidden))
	assert.Equal(t, indexes.SpriteIndex(indexes.Chest), state.GetEffectiveTile(&hidden))
	objectAfter, found := state.CombatScene.ObjectAt(hidden)
	require.True(t, found)
	assert.Equal(t, object, objectAfter, "terrain reveal must not replace the authoritative object")
	assert.Equal(t, 1, callbacks.DisplayUpdates)
}

func TestCombatTerrainObjectDisplayUsesLiveMapOnlyUnderlay(t *testing.T) {
	state, _ := newSyntheticDungeonTriggerCombatState(t)
	layer := state.GetCurrentLayeredMap()
	position := references.Position{X: 4, Y: 4}
	state.CombatScene.AddObject(NewCombatChest(position, 10, false))

	tests := []struct {
		name          string
		base          indexes.SpriteIndex
		override      indexes.SpriteIndex
		wantAllowed   bool
		wantEffective indexes.SpriteIndex
	}{
		{name: "nothing", base: indexes.BlackSquare, override: indexes.NoSprites, wantAllowed: false, wantEffective: indexes.BlackSquare},
		{name: "arch", base: indexes.BrickWallArchway, override: indexes.NoSprites, wantAllowed: false, wantEffective: indexes.BrickWallArchway},
		{name: "ordinary wall", base: indexes.StoneBrickWall, override: indexes.NoSprites, wantAllowed: true, wantEffective: indexes.Chest},
		{name: "floor", base: indexes.BrickFloor, override: indexes.NoSprites, wantAllowed: true, wantEffective: indexes.Chest},
		{name: "floor override reveals", base: indexes.BlackSquare, override: indexes.BrickFloor, wantAllowed: true, wantEffective: indexes.Chest},
		{name: "nothing override conceals", base: indexes.BrickFloor, override: indexes.BlackSquare, wantAllowed: false, wantEffective: indexes.BlackSquare},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			layer.SetTileByLayer(map_state.MapLayer, &position, test.base)
			layer.SetTileByLayer(map_state.MapOverrideLayer, &position, test.override)
			layer.SetTileByLayer(map_state.MapUnitLayer, &position, indexes.Orc1)

			assert.Equal(t, test.wantAllowed, state.CombatTerrainAllowsObjectDisplay(position))
			assert.Equal(t, test.wantEffective, state.GetEffectiveTile(&position))
			_, found := state.CombatScene.ObjectAt(position)
			assert.True(t, found, "display admission must not mutate object storage")
		})
	}
}

func TestCombatTerrainNothingRejectsDestinationsButArchDoesNot(t *testing.T) {
	state, activeIndex := setupCombatAutoActionTest(t)
	identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: activeIndex}
	position := references.Position{X: 4, Y: 4}
	layer := state.GetCurrentLayeredMap()

	layer.SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)
	assert.False(t, state.isCombatPositionValid(&position, activeIndex))
	assert.False(t, state.isCombatPositionValidForPC(&position, identity))

	layer.SetTileByLayer(map_state.MapLayer, &position, indexes.BrickWallArchway)
	assert.True(t, state.isCombatPositionValid(&position, activeIndex))
	assert.True(t, state.isCombatPositionValidForPC(&position, identity))
}

func TestCombatActorMayLeaveNothingForOrdinaryFloor(t *testing.T) {
	state, activeIndex := setupCombatAutoActionTest(t)
	from := references.Position{X: 3, Y: 3}
	to := references.Position{X: 4, Y: 3}
	state.CombatScene.SetPartyMemberPosition(activeIndex, from)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &from, indexes.BlackSquare)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &to, indexes.BrickFloor)

	assert.Equal(t, CombatMoveMoved, state.MoveCombatCharacter(references.Right))
	assert.Equal(t, to, state.CombatScene.GetPartyMemberPosition(activeIndex))
}

func TestCombatMonsterCannotEnterNothing(t *testing.T) {
	state, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 169)
	// The loaded camp formation can occupy (4, 4); select an empty square.
	position := references.Position{X: 0, Y: 0}
	for position.X < references.XCombatMapTiles &&
		(state.CombatScene.isPartyMemberAtPosition(position, -1) || position == monster.Position()) {
		position.X++
	}
	require.True(t, position.X < references.XCombatMapTiles, "fixture must provide an unoccupied target")
	layer := state.GetCurrentLayeredMap()
	layer.SetTileByLayer(map_state.MapLayer, &position, indexes.BrickFloor)
	layer.UnSetTileByLayer(map_state.MapOverrideLayer, &position)
	layer.UnSetTileByLayer(map_state.MapUnitLayer, &position)
	require.True(t, isCombatPositionValidForMonster(state, position, monster, 0),
		"floor control must be unoccupied and passable")
	layer.SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)

	assert.False(t, isCombatPositionValidForMonster(state, position, monster, 0))

	layer.SetTileByLayer(map_state.MapLayer, &position, indexes.BrickFloor)
	assert.True(t, isCombatPositionValidForMonster(state, position, monster, 0))
}

func TestCombatSummonPlacementRetriesAfterNothingWithoutChangingDrawOrder(t *testing.T) {
	state, _ := newSyntheticDungeonTriggerCombatState(t)
	hidden := references.Position{X: 1, Y: 1}
	visible := references.Position{X: 3, Y: 3}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &hidden, indexes.BlackSquare)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &visible, indexes.BrickFloor)
	rng := newSequenceRNG(int(hidden.X), int(hidden.Y), int(visible.X), int(visible.Y))

	position, attempts, found := state.findCombatSummonPosition(references.MonsterRat, rng)

	require.True(t, found)
	assert.Equal(t, visible, position)
	assert.Equal(t, 2, attempts)
	assert.Equal(t, 4, rng.index, "each rejected candidate must retain the original X/Y draw order")
}

func TestCombatClonePlacementRejectsNothingForPCAndMonster(t *testing.T) {
	for _, targetMonster := range []bool{false, true} {
		name := "PC"
		if targetMonster {
			name = "monster"
		}
		t.Run(name, func(t *testing.T) {
			state, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
			target := state.CombatScene.GetCurrentTurnIdentity()
			if targetMonster {
				_, monsterIndex := state.CombatScene.MonsterManager.GetMonsterIndexAtPosition(monster.Position())
				target = CombatTurnIdentity{EntityIndex: monsterIndex}
			}
			setAllCombatTerrainForTest(state, indexes.BlackSquare)
			state.SetRandomSeed(42)

			_, found := state.findCombatClonePosition(target)

			assert.False(t, found)
			assert.Equal(t, uint64(512), state.GetRNG().Calls(), "256 candidates must retain two coordinate draws each")
			setAllCombatTerrainForTest(state, indexes.BrickFloor)
			state.SetRandomSeed(42)
			position, found := state.findCombatClonePosition(target)
			require.True(t, found)
			assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), state.combatMapOnlyTerrain(position))
		})
	}
}

func TestCombatMonsterDivisionRejectsNothingCandidates(t *testing.T) {
	state, _, parent := setupCombatWithMonster(t, references.MonsterSlime, references.Down, 42)
	parent.SetPosition(references.Position{X: 5, Y: 5})
	for y := references.Coordinate(4); y <= 6; y++ {
		for x := references.Coordinate(4); x <= 6; x++ {
			if x == 5 && y == 5 {
				continue
			}
			position := references.Position{X: x, Y: y}
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.NoSprites)
		}
	}
	countBefore := state.CombatScene.MonsterManager.Count()
	state.SetRandomSeed(42)

	state.tryDivideCombatMonster(parent, 0)

	assert.Equal(t, countBefore, state.CombatScene.MonsterManager.Count())
	setAllCombatTerrainForTest(state, indexes.BrickFloor)
	state.SetRandomSeed(42)
	state.tryDivideCombatMonster(parent, 0)
	assert.Equal(t, countBefore+1, state.CombatScene.MonsterManager.Count())
}

func TestCombatDaemonGatingRejectsNothingWithoutConsumingCreationDEX(t *testing.T) {
	state, callbacks, daemon, daemonIndex := setupHostileDaemonTest(t)
	position := firstLegalDaemonGatePosition(t, state)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.NoSprites)
	rng := newSequenceRNG(31, 31, int(position.X), int(position.Y), 1)

	assert.False(t, state.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex, rng))
	assert.Equal(t, 4, rng.index, "rejected terrain must not consume the created daemon's DEX draw")
	assert.Empty(t, callbacks.CombatEvents)
	assert.Equal(t, 1, state.CombatScene.MonsterManager.Count())

	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BrickFloor)
	rng = newSequenceRNG(31, 31, int(position.X), int(position.Y), 1)
	assert.True(t, state.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex, rng))
	assert.Equal(t, 5, rng.index)
	assert.Equal(t, 2, state.CombatScene.MonsterManager.Count())
}

func TestCombatInPorRejectsNothingCandidates(t *testing.T) {
	state, activeIndex := setupCombatAutoActionTest(t)
	setAllCombatTerrainForTest(state, indexes.BlackSquare)
	state.SetRandomSeed(42)

	result := state.PlanCombatInPor(activeIndex)

	assert.Equal(t, CombatInPorFailed, result.Outcome)
	assert.Equal(t, uint64(combatInPorAttempts*2), state.GetRNG().Calls())
	setAllCombatTerrainForTest(state, indexes.BrickFloor)
	state.SetRandomSeed(42)
	result = state.PlanCombatInPor(activeIndex)
	assert.Equal(t, CombatInPorResolved, result.Outcome)
	assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), state.combatMapOnlyTerrain(result.To))
}

func TestConcealedCombatTargetDoesNotLeakIntoAimOrAssistance(t *testing.T) {
	state, _ := newCombatAttackPreviewTest(t, references.Bow)
	target, _ := addSyntheticTriggerMonster(
		state,
		references.Position{X: 5, Y: 2},
		CombatMonsterLoyaltyMonster,
		references.MissileNone,
	)
	position := target.Position()
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)

	assert.Equal(t, CombatAimEmpty, state.GetCombatAimDisposition(position))
	assert.Empty(t, state.PreviewCombatAttack().Targets)
	assert.NotEqual(t, CombatAutoActionAttack, state.PlanCombatAutoAction().Kind)
	assert.False(t, state.BeginCombatAutoAttack(position))
	assert.True(t, target.IsActive(), "concealment must not remove the monster from combat")

	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BrickFloor)
	assert.Equal(t, CombatAimEnemy, state.GetCombatAimDisposition(position))
	assert.Len(t, state.PreviewCombatAttack().Targets, 1)
}

func TestRememberedAimFollowsConcealedTargetAndEmptyShotForgetsIt(t *testing.T) {
	state, _ := newCombatAttackPreviewTest(t, references.Halberd)
	origin := state.CombatScene.GetPartyMemberPosition(0)
	step := references.Coordinate(1)
	if origin.X+2 >= references.XCombatMapTiles {
		step = -1
	}
	position := references.Position{X: origin.X + 2*step, Y: origin.Y}
	_, index := addSyntheticTriggerMonster(state, position, CombatMonsterLoyaltyMonster, references.MissileNone)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.BlackSquare)
	state.CombatScene.LastAttackTargets[0] = index

	require.True(t, state.BeginCombatAttack())
	assert.Equal(t, position, state.CombatScene.AttackTargeting.Cursor, "original aim has no terrain test")

	require.True(t, state.MoveCombatAttackCursor(int(-step), 0))
	state.ConfirmCombatAttackTarget()
	assert.Equal(t, NoCombatTarget, state.CombatScene.LastAttackTargets[0], "an empty reach shot forgets the aim")
}

func TestOriginalCovetousRoomZeroUsesTriggerAuthoredConcealment(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(references.Covetous).
		WithSystemCallbacks().
		Build()

	cell, ok := state.GetCurrentSmallLocationReference().GetDungeonCell(
		0,
		references.Position{X: 3, Y: 2},
	)
	require.True(t, ok)
	assert.Equal(t, references.DungeonLiveRoom, cell.Kind())
	assert.Equal(t, byte(0), cell.Payload())

	mapIndex := references.CalculateDungeonCombatMapIndex(references.CovetousDungeon, int(cell.Payload()))
	require.Equal(t, 48, mapIndex)
	require.NoError(t, state.TransitionToCombatMap(
		mapIndex,
		references.DungeonCombatTerritory,
		CombatEntryNorth,
	))
	assert.Equal(t, indexes.SpriteIndex(indexes.StoneHeadstone),
		state.CombatScene.CombatMapRef.GetTileNumber(references.Position{X: 6, Y: 5}))
	for y := references.Coordinate(4); y <= 6; y++ {
		for x := references.Coordinate(7); x <= 9; x++ {
			assert.Equal(t, indexes.SpriteIndex(indexes.BlackSquare),
				state.CombatScene.CombatMapRef.GetTileNumber(references.Position{X: x, Y: y}))
		}
	}

	triggers, present := state.CombatScene.CombatMapRef.DungeonTriggerSlots()
	require.True(t, present)
	for index, trigger := range triggers {
		assert.Equal(t, references.Position{X: 6, Y: 5}, trigger.Trigger, "trigger slot %d", index)
		if index < 5 {
			assert.Equal(t, indexes.SpriteIndex(indexes.BrickFloor), trigger.ReplacementTile)
		} else {
			assert.Equal(t, indexes.SpriteIndex(indexes.StoneBrickWall), trigger.ReplacementTile)
		}
	}
}

func setAllCombatTerrainForTest(state *GameState, tile indexes.SpriteIndex) {
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, tile)
			state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, &position, indexes.NoSprites)
		}
	}
}
