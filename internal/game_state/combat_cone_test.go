package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func newOpenCombatConeMap() *references.CombatMapReference {
	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	rawData := brickFloorCombatRaw()
	combatMap.SetRawData(rawData)

	return combatMap
}

func newSyntheticCombatConeState(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	gs, mock := newSyntheticDungeonTriggerCombatState(t)
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 5, Y: 5})
	caster := gs.PartyState.GetCharacter(0)
	caster.Intelligence = 255
	caster.SetMaxHp(100)
	caster.SetHp(100)
	gs.SetRandomSeed(1)
	mock.Reset()

	return gs, mock
}

func addSyntheticConeMonster(
	gs *GameState,
	monsterType references.MonsterType,
	position references.Position,
	intelligence int,
) (*CombatMonster, int) {
	reference := references.EnemyReference{
		MonsterType:  monsterType,
		Name:         "Cone victim",
		NamePlural:   "Cone victims",
		Strength:     10,
		Dexterity:    1,
		Intelligence: intelligence,
		Armour:       1,
		HitPoints:    100,
		MaxPerMap:    8,
	}
	monster := NewCombatMonster(monsterType, &reference, position, newFixedRNG(1))

	return monster, gs.CombatScene.AddMonsterToQueue(monster)
}

func (g *GameState) applyCombatInFlamHurAtPosition(casterIdx int, position references.Position) bool {
	processed := make(map[CombatTurnIdentity]struct{}, 1)
	target, found := g.CombatScene.combatConeVictimAt(position, processed)

	return found && g.applyCombatInFlamHurToTarget(casterIdx, target).applied()
}

func (g *GameState) applyCombatInNoxHurAtPosition(casterIdx int, position references.Position) (bool, bool) {
	processed := make(map[CombatTurnIdentity]struct{}, 1)
	if target, found := g.CombatScene.combatConeVictimAt(position, processed); found {
		hit := g.rollCombatDexterityThreshold(g.effectiveCombatDexterity(target))

		result := g.applyCombatPoisonWindToxify(casterIdx, target, hit)

		return result.applied(), result.Outcome == CombatSpellPoisoned
	}

	return false, false
}

func (g *GameState) applyCombatInVasGravCorpAtPosition(casterIdx int, position references.Position) bool {
	processed := make(map[CombatTurnIdentity]struct{}, 1)
	target, found := g.CombatScene.combatConeVictimAt(position, processed)

	return found && g.applyCombatInVasGravCorpToTarget(casterIdx, target).applied()
}

func TestTraceCombatCone_ClipsFootprintAtBlockingTerrain(t *testing.T) {
	combatMap := newOpenCombatConeMap()
	rawData := brickFloorCombatRaw()
	rawData[5][6] = byte(indexes.StoneBrickWall)
	combatMap.SetRawData(rawData)
	spec, found := CombatConeSpecForSpell(references.InZu)
	require.True(t, found)

	positions := TraceCombatCone(combatMap, references.Position{X: 5, Y: 5}, references.Down, spec)

	assert.Contains(t, positions, references.Position{X: 5, Y: 6},
		"the first even-Y plot retains spelldir's adjacent tile before collision sampling")
	assert.NotContains(t, positions, references.Position{X: 5, Y: 7})
	assert.NotContains(t, positions, references.Position{X: 4, Y: 6})
}

func TestTraceCombatCone_UsesOriginalTwentyOneRayFootprintInFourDirections(t *testing.T) {
	spec, found := CombatConeSpecForSpell(references.InZu)
	require.True(t, found)
	origin := references.Position{X: 5, Y: 5}
	tests := []struct {
		name      string
		direction references.Direction
		outer     [2]references.Position
		edge      references.Position
	}{
		{name: "north", direction: references.Up, outer: [2]references.Position{{X: 2, Y: 2}, {X: 8, Y: 2}}, edge: references.Position{X: 5, Y: 0}},
		{name: "south", direction: references.Down, outer: [2]references.Position{{X: 2, Y: 8}, {X: 8, Y: 8}}, edge: references.Position{X: 5, Y: 10}},
		{name: "west", direction: references.Left, outer: [2]references.Position{{X: 2, Y: 2}, {X: 2, Y: 8}}, edge: references.Position{X: 0, Y: 5}},
		{name: "east", direction: references.Right, outer: [2]references.Position{{X: 8, Y: 2}, {X: 8, Y: 8}}, edge: references.Position{X: 10, Y: 5}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			positions := TraceCombatCone(newOpenCombatConeMap(), origin, test.direction, spec)

			assert.Contains(t, positions, test.outer[0])
			assert.Contains(t, positions, test.outer[1])
			assert.Contains(t, positions, test.edge)
		})
	}
}

func TestTraceCombatCone_CarriesAdjacentTileOnEvenYAndClipsOutwardEdges(t *testing.T) {
	spec, found := CombatConeSpecForSpell(references.InZu)
	require.True(t, found)
	buffer := combatConeTileBuffer{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			buffer[x][y] = indexes.BrickFloor
		}
	}
	ray := newCombatConeRay(references.Position{X: 5, Y: 5}, references.Right, combatConeCenterRay)
	for !ray.stopped {
		position, plotted := ray.plot(buffer, spec.BlockedByTile)
		if !plotted {
			break
		}
		assert.Equal(t, references.Position{X: 6, Y: 5}, position,
			"the horizontal center ray retains spelldir's adjacent tile because absolute Y remains even")
		ray.advance()
	}

	tests := []struct {
		origin    references.Position
		direction references.Direction
	}{
		{origin: references.Position{X: 0, Y: 5}, direction: references.Left},
		{origin: references.Position{X: 10, Y: 5}, direction: references.Right},
		{origin: references.Position{X: 5, Y: 0}, direction: references.Up},
		{origin: references.Position{X: 5, Y: 10}, direction: references.Down},
	}
	for _, test := range tests {
		assert.Empty(t, TraceCombatCone(newOpenCombatConeMap(), test.origin, test.direction, spec))
	}
}

func TestTraceCombatCone_ProducesUniquePositionsWithinCombatBounds(t *testing.T) {
	spec, found := CombatConeSpecForSpell(references.InZu)
	require.True(t, found)

	positions := TraceCombatCone(newOpenCombatConeMap(), references.Position{X: 5, Y: 5}, references.Up, spec)
	seen := make(map[references.Position]struct{}, len(positions))
	for _, position := range positions {
		assert.True(t, isCombatMapPosition(position))
		_, duplicate := seen[position]
		assert.False(t, duplicate, "each combatant must save at most once")
		seen[position] = struct{}{}
	}
}

func TestCombatConePreviewIsPureAndCommitRandomizesOneCollection(t *testing.T) {
	gs, _ := newSyntheticCombatConeState(t)
	caster := gs.PartyState.GetCharacter(0)
	caster.Level = 8
	caster.SetMp(20)
	gs.PartyState.Inventory.Spells.Set(references.InZu, 1)
	gs.SetRandomSeed(7)
	require.True(t, gs.BeginCombatConeTargeting(CastRequest{Spell: references.InZu, CasterIdx: 0}).Valid)
	gs.SetCombatConeDirection(references.Up)
	callsBefore := gs.GetRNG().Calls()

	firstPreview := gs.CombatConePreview()
	secondPreview := gs.CombatConePreview()

	assert.Equal(t, firstPreview, secondPreview)
	assert.Equal(t, callsBefore, gs.GetRNG().Calls())
	result := gs.ConfirmCombatConeTarget()
	require.Equal(t, CombatConeTargetResolved, result.Outcome)
	assert.Equal(t, firstPreview, result.Positions)
	assert.Greater(t, gs.GetRNG().Calls(), callsBefore)
	assert.NotEqual(t, result.Positions, result.candidates,
		"the possible footprint must not promise the randomized committed order")
	seen := make(map[references.Position]struct{}, len(result.candidates))
	for _, position := range result.candidates {
		seen[position] = struct{}{}
	}
	assert.Len(t, seen, len(result.candidates))
	assert.Equal(t, byte(15), caster.GetCurrentMp())
	assert.Zero(t, gs.PartyState.Inventory.Spells.Get(references.InZu))
}

func TestCollectCommittedCombatConeCapsOriginalCandidateBuffer(t *testing.T) {
	spec, found := CombatConeSpecForSpell(references.InZu)
	require.True(t, found)
	positions := collectCommittedCombatCone(
		combatConeTileBuffer{},
		references.Position{X: 5, Y: 10},
		references.Up,
		spec,
		newFixedRNG(7),
	)

	assert.Len(t, positions, combatConeCandidateCap)
	seen := make(map[references.Position]struct{}, len(positions))
	for _, position := range positions {
		seen[position] = struct{}{}
	}
	assert.Len(t, seen, combatConeCandidateCap)
}

func TestCombatConeCandidateProjectionHandlesInvisibleColocatedRawZeroAndBlockingMarkers(t *testing.T) {
	target := references.Position{X: 5, Y: 3}
	t.Run("isolated invisible monster on ordinary terrain is excluded", func(t *testing.T) {
		gs, _ := newSyntheticCombatConeState(t)
		monster, _ := addSyntheticConeMonster(gs, references.MonsterOrc, target, 1)
		monster.SetInvisible(true)

		assert.Empty(t, gs.CastCombatInZu(0, references.Up))
		assert.False(t, monster.IsAsleep())
	})

	t.Run("visible colocated object admits invisible monster", func(t *testing.T) {
		gs, _ := newSyntheticCombatConeState(t)
		monster, _ := addSyntheticConeMonster(gs, references.MonsterOrc, target, 1)
		monster.SetInvisible(true)
		require.True(t, gs.CombatScene.AddObject(NewCombatChest(target, 1, false)))

		assert.Equal(t, []references.Position{target}, gs.CastCombatInZu(0, references.Up))
		assert.True(t, monster.IsAsleep())
	})

	t.Run("visible actor marker opens otherwise blocking terrain", func(t *testing.T) {
		gs, _ := newSyntheticCombatConeState(t)
		monster, _ := addSyntheticConeMonster(gs, references.MonsterOrc, target, 1)
		gs.setCombatTerrain(target, indexes.StoneBrickWall)

		assert.Equal(t, []references.Position{target}, gs.CastCombatInZu(0, references.Up))
		assert.True(t, monster.IsAsleep())
	})

	t.Run("invisible PC still contributes its source marker", func(t *testing.T) {
		gs, _ := newSyntheticCombatConeState(t)
		source := gs.CombatScene.GetCurrentTurnIdentity()
		clone := gs.CombatScene.addPCCloneFromSource(
			source, source.EntityIndex, target, NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
		)
		require.NotZero(t, clone.Slot)
		gs.CombatScene.setCombatPCInvisible(clone, true)

		assert.Equal(t, []references.Position{target}, gs.CastCombatInZu(0, references.Up))
		assert.True(t, gs.CombatScene.isCombatPCSleeping(clone))
	})
}

func TestCombatConeSelectsHighestSourceSlotFromFourStackedInsects(t *testing.T) {
	gs, _ := newSyntheticCombatConeState(t)
	target := references.Position{X: 5, Y: 3}
	insects := make([]*CombatMonster, 0, inBetXenSummonCount)
	for range inBetXenSummonCount {
		monster, _ := addSyntheticConeMonster(gs, references.MonsterInsect, target, 1)
		insects = append(insects, monster)
	}

	assert.Equal(t, []references.Position{target}, gs.CastCombatInZu(0, references.Up))
	for index, insect := range insects {
		assert.Equal(t, index == len(insects)-1, insect.IsAsleep(), "insect %d", index)
	}
}

func TestCombatConeHighestStackedVictimStopsSelectionOnSave(t *testing.T) {
	gs, _ := newSyntheticCombatConeState(t)
	target := references.Position{X: 5, Y: 3}
	low, _ := addSyntheticConeMonster(gs, references.MonsterInsect, target, 0)
	high, _ := addSyntheticConeMonster(gs, references.MonsterInsect, target, 255)
	gs.PartyState.GetCharacter(0).Intelligence = 0

	assert.Empty(t, gs.resolveCombatInZu(0, []references.Position{target}))
	assert.False(t, high.IsAsleep())
	assert.False(t, low.IsAsleep(), "a successful save by the highest slot must not fall through")
}

func TestCombatConeStackedMonsterOutranksPCClone(t *testing.T) {
	gs, _ := newSyntheticCombatConeState(t)
	target := references.Position{X: 5, Y: 3}
	source := gs.CombatScene.GetCurrentTurnIdentity()
	clone := gs.CombatScene.addPCCloneFromSource(
		source, source.EntityIndex, target, NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
	)
	require.NotZero(t, clone.Slot)
	monster, _ := addSyntheticConeMonster(gs, references.MonsterOrc, target, 1)

	assert.Equal(t, []references.Position{target}, gs.CastCombatInZu(0, references.Up))
	assert.True(t, monster.IsAsleep())
	assert.False(t, gs.CombatScene.isCombatPCSleeping(clone))
}

func TestCombatConeTargeting_DefersConsumptionUntilConfirmed(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	caster.Level = 5
	caster.SetMp(20)
	gs.PartyState.Inventory.Spells.Set(references.InZu, 1)
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	request := CastRequest{Spell: references.InZu, CasterIdx: activeIdx}
	mpBefore := caster.GetCurrentMp()

	require.True(t, gs.BeginCombatConeTargeting(request).Valid)
	assert.Equal(t, CombatConeTargetAwaitingDirection, gs.SetCombatConeDirection(references.Down))
	preview := gs.CombatConePreview()
	assert.NotEmpty(t, preview)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.Spells.Get(references.InZu))

	result := gs.ConfirmCombatConeTarget()

	assert.Equal(t, CombatConeTargetResolved, result.Outcome)
	assert.Equal(t, preview, result.Positions)
	assert.Equal(t, mpBefore-byte(5), caster.GetCurrentMp())
	assert.Zero(t, gs.PartyState.Inventory.Spells.Get(references.InZu))
	assert.Nil(t, gs.CombatScene.ConeTargeting)
}

func TestCombatConeResolutionUsesExactCloneOrigin(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	source := gs.CombatScene.GetCurrentTurnIdentity()
	clonePosition := references.Position{X: 5, Y: 5}
	targetPosition := references.Position{X: 5, Y: 6}
	clone := gs.CombatScene.addPCCloneFromSource(
		source,
		source.EntityIndex,
		clonePosition,
		NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
	)
	gs.CombatScene.CurrentTurnIdentity = clone
	gs.CombatScene.currentTurnIdentitySet = true
	gs.CombatScene.SetPartyMemberPosition(source.EntityIndex, references.Position{X: 1, Y: 1})
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	monster.SetPosition(targetPosition)
	mock.Reset()

	affected := gs.CastCombatInFlamHur(source.EntityIndex, references.Down)

	assert.Contains(t, affected, targetPosition)
	var monsterEvent *CombatEvent
	for index := range mock.CombatEvents {
		if !mock.CombatEvents[index].Target.IsPartyMember {
			monsterEvent = &mock.CombatEvents[index]
			break
		}
	}
	require.NotNil(t, monsterEvent)
	assert.Equal(t, clone.Slot, monsterEvent.Attacker.Slot)
	assert.Equal(t, clonePosition, monsterEvent.Attacker.Position)
}

func TestCombatInZu_LeavesOriginalImmuneMonsterAwake(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterBlackthorn, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	gs.PartyState.GetCharacter(activeIdx).Intelligence = 255
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	mock.Reset()

	affected := gs.CastCombatInZu(activeIdx, references.Down)

	assert.False(t, monster.IsAsleep())
	assert.NotContains(t, affected, monster.Position())
	require.NotEmpty(t, mock.CombatEvents)
	for _, event := range mock.CombatEvents {
		require.NotNil(t, event.SpellTarget)
		assert.Equal(t, CombatSpellImmune, event.SpellTarget.Outcome)
	}
}

func TestCombatInNoxHurToxifiesAtDexterityThreshold(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	gs.SetRandomSeed(1)
	mock.Reset()

	affected := gs.resolveCombatInNoxHur(
		gs.CombatScene.GetActiveCombatantIndex(), []references.Position{monster.Position()},
	)

	assert.False(t, monster.IsPoisoned())
	assert.Contains(t, affected, monster.Position())
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, CombatEventSpell, event.Kind)
	assert.Equal(t, CombatFeedbackDamage, event.Result)
	assert.Positive(t, event.HPLost)
	assert.Equal(t, references.InNoxHur, event.Diagnostics.Spell)
	assert.GreaterOrEqual(t, event.Diagnostics.Hit.D30Roll, event.Diagnostics.Hit.Threshold)
	assert.Empty(t, mock.TransientIconCalls, "poisoning monsters does not report a party status change")
}

func TestCombatDexterityThresholdResult(t *testing.T) {
	assert.Equal(t, CombatHitMiss, combatDexterityThresholdResult(14, 15).Outcome)
	assert.Equal(t, CombatHitNormal, combatDexterityThresholdResult(15, 15).Outcome)
	assert.Equal(t, CombatHitNormal, combatDexterityThresholdResult(16, 15).Outcome)
}

func TestCombatInNoxHurSleepingMonsterUsesEffectiveDexterityOne(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	monster.currentDex = 30
	monster.SetAsleep(true)
	gs.SetRandomSeed(1)
	mock.Reset()

	gs.resolveCombatInNoxHur(
		gs.CombatScene.GetActiveCombatantIndex(), []references.Position{monster.Position()},
	)

	assert.False(t, monster.IsPoisoned())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, 1, mock.CombatEvents[0].Diagnostics.Hit.Threshold)
}

func TestCombatConeTargeting_InNoxHurDefersResourcesUntilConfirmation(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	caster.Level = 7
	caster.SetMp(20)
	gs.PartyState.Inventory.Spells.Set(references.InNoxHur, 1)
	mpBefore := caster.GetCurrentMp()

	require.True(t, gs.BeginCombatConeTargeting(CastRequest{Spell: references.InNoxHur, CasterIdx: activeIdx}).Valid)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.Spells.Get(references.InNoxHur))
	gs.SetCombatConeDirection(references.Down)
	result := gs.ConfirmCombatConeTarget()

	assert.Equal(t, CombatConeTargetResolved, result.Outcome)
	assert.Equal(t, mpBefore-byte(7), caster.GetCurrentMp())
	assert.Zero(t, gs.PartyState.Inventory.Spells.Get(references.InNoxHur))
}

func TestCombatInFlamHurDamagesMonsterWithoutArmor(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	monster.enemyRef.Armour = 255
	gs.SetRandomSeed(1)
	mock.Reset()
	hpBefore := monster.CurrentHP()

	affected := gs.CastCombatInFlamHur(gs.CombatScene.GetActiveCombatantIndex(), references.Down)

	assert.Contains(t, affected, monster.Position())
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, references.InFlamHur, event.Diagnostics.Spell)
	assert.Equal(t, event.Diagnostics.Damage.AttackRoll, event.Diagnostics.FinalDamage)
	assert.Greater(t, event.Diagnostics.Damage.AttackRoll, 0)
	assert.Equal(t, hpBefore-event.HPLost, monster.CurrentHP())
	assert.Zero(t, event.Diagnostics.Damage.ArmorRoll)
}

func TestCombatInVasGravCorpInstantlyKillsFailedSave(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	gs.PartyState.GetCharacter(activeIdx).Intelligence = 255
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	hpBefore := monster.CurrentHP()
	mock.Reset()

	affected := gs.CastCombatInVasGravCorp(activeIdx, references.Down)

	assert.Contains(t, affected, monster.Position())
	assert.True(t, monster.IsDead())
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, references.InVasGravCorp, event.Diagnostics.Spell)
	assert.Equal(t, combatInstantKillDamage, event.Diagnostics.FinalDamage)
	assert.True(t, event.Fatal)
	assert.Equal(t, hpBefore, event.HPLost)
}

func TestCombatInVasGravCorpIntelligenceSaveAndImmunityPreventDeath(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterBlackthorn, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	gs.PartyState.GetCharacter(activeIdx).Intelligence = 255
	gs.CombatScene.CombatMapRef = newOpenCombatConeMap()
	mock.Reset()

	affected := gs.CastCombatInVasGravCorp(activeIdx, references.Down)

	assert.NotContains(t, affected, monster.Position())
	assert.False(t, monster.IsDead())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatFeedbackMiss, mock.CombatEvents[0].Result)
	assert.True(t, mock.CombatEvents[0].Diagnostics.Hit.Rolled)
}

func TestCombatAdjacentSpellTargeting_EmptySquareKeepsResourcesAndTargeting(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	caster.Level = 4
	caster.SetMp(20)
	gs.PartyState.Inventory.Spells.Set(references.AnGrav, 1)
	mpBefore := caster.GetCurrentMp()
	require.True(t, gs.BeginCombatAdjacentSpellTargeting(CastRequest{
		Spell: references.AnGrav, CasterIdx: activeIdx,
	}).Valid)
	gs.SetCombatAdjacentSpellDirection(references.Down)

	result := gs.ConfirmCombatAdjacentSpellTarget()

	assert.Equal(t, CombatAdjacentSpellTargetInvalid, result.Outcome)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.Spells.Get(references.AnGrav))
	assert.NotNil(t, gs.CombatScene.AdjacentSpellTargeting)
}

func TestCombatAdjacentSpellTargeting_ValidFieldConsumesAndResolves(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	caster.Level = 4
	caster.SetMp(20)
	gs.PartyState.Inventory.Spells.Set(references.AnGrav, 1)
	origin := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	target := references.Down.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, *target)))
	require.True(t, gs.BeginCombatAdjacentSpellTargeting(CastRequest{
		Spell: references.AnGrav, CasterIdx: activeIdx,
	}).Valid)
	gs.SetCombatAdjacentSpellDirection(references.Down)

	result := gs.ConfirmCombatAdjacentSpellTarget()

	assert.Equal(t, CombatAdjacentSpellTargetResolved, result.Outcome)
	assert.Equal(t, *target, result.Target)
	assert.Nil(t, gs.CombatScene.AdjacentSpellTargeting)
	assert.Equal(t, byte(16), caster.GetCurrentMp())
	assert.Zero(t, gs.PartyState.Inventory.Spells.Get(references.AnGrav))
}

func TestCombatAdjacentSpellTargeting_ConcealsPreviewWithoutRestrictingManualResolution(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	caster.Level = 4
	caster.SetMp(20)
	gs.PartyState.Inventory.Spells.Set(references.AnGrav, 1)
	origin := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	target := references.Down.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, *target)))
	gs.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, target, indexes.BlackSquare)
	require.True(t, gs.BeginCombatAdjacentSpellTargeting(CastRequest{
		Spell: references.AnGrav, CasterIdx: activeIdx,
	}).Valid)
	gs.SetCombatAdjacentSpellDirection(references.Down)

	previewTarget, valid, active := gs.CombatAdjacentSpellPreview()

	assert.Equal(t, *target, previewTarget)
	assert.False(t, valid, "targeting assistance must not disclose the concealed field")
	assert.True(t, active)
	result := gs.ConfirmCombatAdjacentSpellTarget()
	assert.Equal(t, CombatAdjacentSpellTargetResolved, result.Outcome,
		"concealment must not change manually selected spell resolution")
	assert.Equal(t, *target, result.Target)
	assert.Nil(t, gs.CombatScene.AdjacentSpellTargeting)
	assert.Equal(t, byte(16), caster.GetCurrentMp())
	assert.Zero(t, gs.PartyState.Inventory.Spells.Get(references.AnGrav))
}

// TraceCombatCone returns the pure possible footprint of the original 21 pixel
// rays. It does not consume gameplay RNG or expose committed candidate order.
func TraceCombatCone(
	combatMap *references.CombatMapReference,
	origin references.Position,
	direction references.Direction,
	spec CombatConeSpec,
) []references.Position {
	if combatMap == nil {
		return nil
	}
	projection := combatConeProjection{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			position := references.Position{X: x, Y: y}
			projection.terrain[x][y] = combatMap.GetTileNumber(position)
			projection.display[x][y] = projection.terrain[x][y]
		}
	}

	return tracePossibleCombatCone(projection.display, origin, direction, spec)
}
