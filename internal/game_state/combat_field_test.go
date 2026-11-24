package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func TestCombatFieldsStackExpireAndDispelInCreationOrder(t *testing.T) {
	combat := &CombatState{fields: make([]CombatField, 0, maxCombatFields)}
	position := references.Position{X: 4, Y: 4}
	require.True(t, combat.AddField(NewCombatField(CombatFieldFire, position)))
	require.True(t, combat.AddField(NewCombatField(CombatFieldPoison, position)))

	removed, found := combat.removeFirstFieldAt(position)
	require.True(t, found)
	assert.Equal(t, CombatFieldFire, removed.Kind())
	assert.Equal(t, CombatFieldPoison, combat.Fields()[0].Kind())

	expired := combat.expireFields(func(int) int { return 0 })
	require.Len(t, expired, 1)
	assert.Empty(t, combat.Fields())
}

func TestCombatProtectionFieldBlocksOnlyMovement(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	position := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	position.Y++
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldProtection, position)))

	assert.False(t, gs.isCombatPositionValid(&position, activeIdx))
}

func TestCombatSleepFieldUsesExactCloneEntry(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	source := gs.CombatScene.GetCurrentTurnIdentity()
	clonePosition := references.Position{X: 1, Y: 1}
	clone := gs.CombatScene.addPCCloneFromSource(
		source,
		source.EntityIndex,
		clonePosition,
		NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
	)
	mock.Reset()

	gs.resolveCombatFieldAtPosition(NewCombatField(CombatFieldSleep, clonePosition), clonePosition, NoTargetPC, false)

	assert.True(t, gs.CombatScene.isCombatPCSleeping(clone))
	assert.False(t, gs.CombatScene.isCombatPCSleeping(source))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, clonePosition, mock.CombatEvents[0].Target.Position)
	assert.True(t, gs.AwakenCharacter(source.EntityIndex))
	assert.False(t, gs.CombatScene.isCombatPCSleeping(clone))
}

func TestCombatFieldSpellCreatesPoisonFieldAndAffectsGoodPC(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	position := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	gs.resolveCombatFieldSpell(pendingCombatSpell{
		spell:     references.InNoxGrav,
		casterIdx: activeIdx,
		caster:    CombatTurnIdentity{IsPartyMember: true, EntityIndex: activeIdx},
		launch:    CombatProjectileLaunch{From: position, Impact: position},
	})

	require.Len(t, gs.CombatScene.Fields(), 1)
	assert.Equal(t, CombatFieldPoison, gs.CombatScene.Fields()[0].Kind())
	assert.True(t, gs.PartyState.GetCharacter(activeIdx).IsPoisoned())
}

func TestCombatMovementOntoPoisonFieldPoisonsPC(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.SetRandomSeed(1)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	target := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	target.Y--
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldPoison, target)))

	assert.Equal(t, CombatMoveMoved, gs.MoveCombatCharacter(references.Up))
	assert.True(t, gs.PartyState.GetCharacter(activeIdx).IsPoisoned())
}

func TestCombatSwampAutomaticallyPoisonsOnlyActingEligiblePCWithoutRNG(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	identity := gs.CombatScene.GetCurrentTurnIdentity()
	character := gs.PartyState.GetCharacter(identity.EntityIndex)
	character.SetStatus(party_state.Good)
	otherIndex := (identity.EntityIndex + 1) % party_state.MaxActiveParty
	other := gs.PartyState.GetCharacter(otherIndex)
	other.SetStatus(party_state.Good)
	position := gs.CombatScene.GetCombatantPosition(identity)
	gs.CombatScene.CombatMapRef.SetTileNumber(position, indexes.Swamp)
	gs.SetRandomSeed(445)
	callsBefore := gs.GetRNG().Calls()
	mock.Reset()

	gs.resolveCombatPCFieldEffects(identity)

	assert.True(t, character.IsPoisoned())
	assert.True(t, other.IsGood())
	assert.Equal(t, callsBefore, gs.GetRNG().Calls())
	assert.Contains(t, mock.Messages, character.GetNameAsString()+" is poisoned!")
}

func TestCombatSwampSkipsDeadActingPCWithoutRNG(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	identity := gs.CombatScene.GetCurrentTurnIdentity()
	character := gs.PartyState.GetCharacter(identity.EntityIndex)
	character.SetStatus(party_state.Dead)
	position := gs.CombatScene.GetCombatantPosition(identity)
	gs.CombatScene.CombatMapRef.SetTileNumber(position, indexes.Swamp)
	gs.SetRandomSeed(445)
	callsBefore := gs.GetRNG().Calls()
	mock.Reset()

	gs.resolveCombatPCFieldEffects(identity)

	assert.True(t, character.IsDead())
	assert.Equal(t, callsBefore, gs.GetRNG().Calls())
	assert.Empty(t, mock.Messages)
}

func TestCombatStandingLavaTargetsActingPCBeforeColocatedField(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	identity := gs.CombatScene.GetCurrentTurnIdentity()
	character := gs.PartyState.GetCharacter(identity.EntityIndex)
	position := monster.Position()
	gs.CombatScene.PartyPositions[identity.EntityIndex] = position
	gs.CombatScene.CombatMapRef.SetTileNumber(position, indexes.Lava)
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, position)))
	seed := findCombatWakeSeed(t, gs, 10, func(roll int) bool { return roll == 10 })
	gs.SetRandomSeed(seed)
	hpBefore := character.GetCurrentHp()
	monsterHPBefore := monster.CurrentHP()
	mock.Reset()

	gs.resolveCombatPCStandingEffects(identity)

	assert.Equal(t, hpBefore-10, character.GetCurrentHp())
	assert.Equal(t, monsterHPBefore, monster.CurrentHP())
	assert.False(t, gs.CombatScene.isCombatPCSleeping(identity), "recognized terrain suppresses field scanning")
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventFieldTriggered, mock.CombatEvents[0].Kind)
	assert.Equal(t, CombatFieldFire, mock.CombatEvents[0].FieldKind)
	assert.True(t, mock.CombatEvents[0].Target.IsPartyMember)
}

func TestCombatStandingSwampDamagesNonGoodPCThroughInclusiveRange(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	identity := gs.CombatScene.GetCurrentTurnIdentity()
	character := gs.PartyState.GetCharacter(identity.EntityIndex)
	character.SetStatus(party_state.Poisoned)
	position := gs.CombatScene.GetCombatantPosition(identity)
	gs.CombatScene.CombatMapRef.SetTileNumber(position, indexes.Swamp)
	seed := findCombatWakeSeed(t, gs, 20, func(roll int) bool { return roll == 20 })
	gs.SetRandomSeed(seed)
	hpBefore := character.GetCurrentHp()
	mock.Reset()

	gs.resolveCombatPCStandingEffects(identity)

	assert.Equal(t, hpBefore-20, character.GetCurrentHp())
	assert.True(t, character.IsPoisoned())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatFieldPoison, mock.CombatEvents[0].FieldKind)
}

func TestCombatStandingPoisonUsesOriginalMonsterShapeGate(t *testing.T) {
	for _, test := range []struct {
		name         string
		monsterType  references.MonsterType
		expectDamage bool
	}{
		{name: "NPC shape", monsterType: references.MonsterGuard, expectDamage: true},
		{name: "monster shape", monsterType: references.MonsterOrc, expectDamage: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, mock, monster := setupCombatWithMonster(t, test.monsterType, references.Down, 42)
			_, monsterIndex := gs.CombatScene.MonsterManager.GetMonsterIndexAtPosition(monster.Position())
			require.NotEqual(t, NoActiveMonster, monsterIndex)
			require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldPoison, monster.Position())))
			seed := findCombatWakeSeed(t, gs, 20, func(roll int) bool { return roll == 20 })
			gs.SetRandomSeed(seed)
			hpBefore := monster.CurrentHP()
			callsBefore := gs.GetRNG().Calls()
			mock.Reset()

			gs.resolveCombatMonsterFieldEffects(monster, monsterIndex)

			if test.expectDamage {
				assert.Equal(t, hpBefore-20, monster.CurrentHP())
				assert.Equal(t, callsBefore+1, gs.GetRNG().Calls())
				require.Len(t, mock.CombatEvents, 1)
				assert.Equal(t, CombatFieldPoison, mock.CombatEvents[0].FieldKind)
			} else {
				assert.Equal(t, hpBefore, monster.CurrentHP())
				assert.Equal(t, callsBefore, gs.GetRNG().Calls())
				assert.Empty(t, mock.CombatEvents)
			}
		})
	}
}

func TestCombatStandingLavaAppliesMonsterDamageModifiers(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterSkeleton, references.Down, 42)
	_, monsterIndex := gs.CombatScene.MonsterManager.GetMonsterIndexAtPosition(monster.Position())
	require.NotEqual(t, NoActiveMonster, monsterIndex)
	gs.CombatScene.CombatMapRef.SetTileNumber(monster.Position(), indexes.Lava)
	seed := findCombatWakeSeed(t, gs, 10, func(roll int) bool { return roll == 10 })
	gs.SetRandomSeed(seed)
	hpBefore := monster.CurrentHP()
	mock.Reset()

	gs.resolveCombatMonsterFieldEffects(monster, monsterIndex)

	assert.Equal(t, hpBefore-5, monster.CurrentHP(), "nonmagical standing fire is halved against undead")
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatFieldFire, mock.CombatEvents[0].FieldKind)
}

func TestSkippedMonsterTurnStillResolvesStandingField(t *testing.T) {
	for _, presented := range []bool{false, true} {
		t.Run(map[bool]string{false: "immediate", true: "presented"}[presented], func(t *testing.T) {
			gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
			_, monsterIndex := gs.CombatScene.MonsterManager.GetMonsterIndexAtPosition(monster.Position())
			require.NotEqual(t, NoActiveMonster, monsterIndex)
			monster.SetAsleep(true)
			require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldFire, monster.Position())))
			forceNextCombatActor(gs.CombatScene.TurnQueue, monsterIndex, false)
			nextPC := firstActiveCombatPC(gs)
			require.NotEqual(t, NoTargetPC, nextPC)
			gs.CombatScene.TurnQueue.GetEntry(nextPC, true).turnCountdown = 2
			gs.CombatScene.presentTurns = presented
			gs.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}
			mock.Reset()

			gs.AdvanceCombatTurn()
			if presented {
				step, scheduled := gs.ReviewScheduledCombatStep()
				require.True(t, scheduled)
				require.Equal(t, CombatPresentationMonsterTurn, step.Kind)
				_, err := gs.ResolveScheduledCombatStep(step.Token)
				require.NoError(t, err)
			}

			assert.Equal(t, 1, countCombatEvents(mock.CombatEvents, CombatEventFieldTriggered))
			assert.Len(t, gs.CombatScene.Fields(), 1)
			assert.Equal(t, 2, gs.ActiveDurationEffect.TurnsRemaining,
				"hostile boundaries do not invoke endaction")
		})
	}
}

func TestCombatNumericSelectionSkipsFieldExpiration(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	actor := gs.CombatScene.GetCurrentTurnIdentity()
	fieldPosition := references.Position{X: 1, Y: 1}
	require.NotEqual(t, fieldPosition, gs.CombatScene.GetCombatantPosition(actor))
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldProtection, fieldPosition)))
	nextPC := (actor.EntityIndex + 1) % party_state.MaxActiveParty
	require.True(t, gs.CombatScene.PartyActive[nextPC])
	forceNextCombatActor(gs.CombatScene.TurnQueue, nextPC, true)
	gs.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}
	seed := findCombatWakeSeed(t, gs, 255, func(roll int) bool { return roll < 16 })
	gs.SetRandomSeed(seed)
	randomCallsBefore := gs.GetRNG().Calls()

	gs.FinishCombatSelection()

	assert.Len(t, gs.CombatScene.Fields(), 1)
	assert.Equal(t, randomCallsBefore, gs.GetRNG().Calls())
	assert.Equal(t, 2, gs.ActiveDurationEffect.TurnsRemaining)
}

func TestControlledMonsterActionExpiresFieldsBeforeStandingEffect(t *testing.T) {
	gs, mock := setupControlledMonsterTest(t)
	monster, monsterIndex := addControlledTestMonster(
		gs, references.MonsterOrc, references.Position{X: 5, Y: 5}, CombatMonsterLoyaltyParty,
	)
	authorizeControlledMonster(gs, monsterIndex)
	field := NewCombatField(CombatFieldFire, monster.Position())
	field.createdThisAction = true
	require.True(t, gs.CombatScene.AddField(field))
	gs.ActiveDurationEffect = DurationEffect{EffectType: DurationEffectProtection, TurnsRemaining: 2}
	seed := findCombatWakeSeed(t, gs, 255, func(roll int) bool { return roll < 16 })
	gs.SetRandomSeed(seed)
	mock.Reset()

	gs.finishCombatAction()
	assert.Equal(t, 1, gs.ActiveDurationEffect.TurnsRemaining,
		"party-loyal monsters use the player-controlled endaction boundary")

	assert.Empty(t, gs.CombatScene.Fields())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventFieldExpired, mock.CombatEvents[0].Kind)
	assert.True(t, mock.CombatEvents[0].FieldFizzled)
}

func TestSuccessfulCombatFleeStillExpiresFields(t *testing.T) {
	t.Run("PC", func(t *testing.T) {
		gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
		identity := gs.CombatScene.GetCurrentTurnIdentity()
		gs.CombatScene.SetPartyMemberPosition(identity.EntityIndex, references.Position{X: 0, Y: 5})
		require.True(t, gs.CombatScene.AddField(NewCombatField(
			CombatFieldProtection, references.Position{X: 1, Y: 1},
		)))
		seed := findCombatWakeSeed(t, gs, 255, func(roll int) bool { return roll < 16 })
		gs.SetRandomSeed(seed)
		mock.Reset()

		assert.Equal(t, CombatMoveFled, gs.MoveCombatCharacter(references.Left))

		assert.Empty(t, gs.CombatScene.Fields())
		assert.Equal(t, 1, countCombatEvents(mock.CombatEvents, CombatEventFieldExpired))
	})

	t.Run("controlled monster", func(t *testing.T) {
		gs, mock := setupControlledMonsterTest(t)
		_, monsterIndex := addControlledTestMonster(
			gs, references.MonsterOrc, references.Position{X: 0, Y: 5}, CombatMonsterLoyaltyParty,
		)
		authorizeControlledMonster(gs, monsterIndex)
		require.True(t, gs.CombatScene.AddField(NewCombatField(
			CombatFieldProtection, references.Position{X: 1, Y: 1},
		)))
		seed := findCombatWakeSeed(t, gs, 255, func(roll int) bool { return roll < 16 })
		gs.SetRandomSeed(seed)
		mock.Reset()

		assert.Equal(t, CombatMoveFled, gs.MoveCombatCharacter(references.Left))

		assert.Empty(t, gs.CombatScene.Fields())
		assert.Equal(t, 1, countCombatEvents(mock.CombatEvents, CombatEventFieldExpired))
	})
}

func TestCombatMovementOntoSleepFieldSleepsPC(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.SetRandomSeed(1)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	target := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	target.Y--
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, target)))

	assert.Equal(t, CombatMoveMoved, gs.MoveCombatCharacter(references.Up))
	assert.True(t, gs.PartyState.GetCharacter(activeIdx).IsAsleep())
}

func TestCombatMovementOntoFireFieldDamagesPC(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.SetRandomSeed(1)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIdx)
	hpBefore := character.GetCurrentHp()
	target := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	target.Y--
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldFire, target)))

	assert.Equal(t, CombatMoveMoved, gs.MoveCombatCharacter(references.Up))
	hpLost := int(hpBefore - character.GetCurrentHp())
	assert.Greater(t, hpLost, 0)
	require.NotEmpty(t, mock.CombatEvents)
	trigger := mock.CombatEvents[len(mock.CombatEvents)-1]
	assert.Equal(t, CombatEventFieldTriggered, trigger.Kind)
	assert.Equal(t, CombatFieldFire, trigger.FieldKind)
	assert.Equal(t, hpLost, trigger.HPLost)
}

func TestStandingPoisonFieldIgnoresMonster(t *testing.T) {
	gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	hpBefore := monster.CurrentHP()
	field := NewCombatField(CombatFieldPoison, monster.Position())
	gs.resolveCombatFieldAtPosition(field, monster.Position(), NoTargetPC, false)

	assert.Equal(t, hpBefore, monster.CurrentHP())
}

func TestCombatAnGravRemovesFirstFieldOnly(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	origin := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	position := references.Position{X: origin.X, Y: origin.Y + 1}
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldFire, position)))
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, position)))

	require.True(t, gs.CastCombatAnGrav(activeIdx, references.Down))
	require.Len(t, gs.CombatScene.Fields(), 1)
	assert.Equal(t, CombatFieldSleep, gs.CombatScene.Fields()[0].Kind())
}

func TestCombatAnGravUsesExactCloneOriginAndFeedback(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	source := gs.CombatScene.GetCurrentTurnIdentity()
	clonePosition := references.Position{X: 2, Y: 2}
	target := references.Position{X: 2, Y: 3}
	clone := gs.CombatScene.addPCCloneFromSource(
		source,
		source.EntityIndex,
		clonePosition,
		NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
	)
	gs.CombatScene.CurrentTurnIdentity = clone
	gs.CombatScene.currentTurnIdentitySet = true
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldFire, target)))
	mock.Reset()

	require.True(t, gs.CastCombatAnGrav(source.EntityIndex, references.Down))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, clone.Slot, mock.CombatEvents[0].Attacker.Slot)
	assert.Equal(t, clonePosition, mock.CombatEvents[0].Attacker.Position)
}

func TestCombatSceptreRemovesFirstFieldOnlyWithoutConsumption(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	origin := gs.CombatScene.GetPartyMemberPosition(activeIdx)
	position := references.Position{X: origin.X, Y: origin.Y + 1}
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldFire, position)))
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, position)))
	mock.Reset()

	require.True(t, gs.UseCombatSceptre(references.Down))
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.QuestItems.Get(references.Sceptre))
	require.Len(t, gs.CombatScene.Fields(), 1)
	assert.Equal(t, CombatFieldSleep, gs.CombatScene.Fields()[0].Kind())
	assert.Equal(t, []string{"Field dissolved!"}, mock.Messages)
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventFieldDispelled, mock.CombatEvents[0].Kind)
	assert.Equal(t, CombatDiagnostics{}, mock.CombatEvents[0].Diagnostics)
}

func TestCombatSceptreReportsNoEffectWithoutMutatingInventory(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	gs.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	mock.Reset()

	assert.False(t, gs.UseCombatSceptre(references.Down))
	assert.Equal(t, uint16(1), gs.PartyState.Inventory.QuestItems.Get(references.Sceptre))
	assert.Empty(t, gs.CombatScene.Fields())
	assert.Equal(t, []string{"No effect!"}, mock.Messages)
	assert.Empty(t, mock.CombatEvents)
}

func TestCombatInZuSleepsTargetInDirection(t *testing.T) {
	gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	fillCombatMapForDivisionTest(gs)
	gs.SetRandomSeed(1)
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	gs.PartyState.GetCharacter(activeIdx).Intelligence = 255

	gs.CastCombatInZu(activeIdx, references.Down)

	assert.True(t, monster.IsAsleep())
}

func TestCombatFieldSpritesMatchOriginalFieldKinds(t *testing.T) {
	assert.Equal(t, indexes.SpriteIndex(indexes.PoisonField), NewCombatField(CombatFieldPoison, references.Position{}).Sprite())
	assert.Equal(t, indexes.SpriteIndex(indexes.MagicField), NewCombatField(CombatFieldSleep, references.Position{}).Sprite())
	assert.Equal(t, indexes.SpriteIndex(indexes.FireField), NewCombatField(CombatFieldFire, references.Position{}).Sprite())
	assert.Equal(t, indexes.SpriteIndex(indexes.ElectricField), NewCombatField(CombatFieldProtection, references.Position{}).Sprite())
}

func TestCombatFieldExpirationRetainsFirstActionFizzleState(t *testing.T) {
	combat := &CombatState{fields: make([]CombatField, 0, maxCombatFields)}
	field := NewCombatField(CombatFieldSleep, references.Position{X: 4, Y: 4})
	field.createdThisAction = true
	require.True(t, combat.AddField(field))

	expired := combat.expireFields(func(int) int { return 0 })

	require.Len(t, expired, 1)
	assert.True(t, expired[0].createdThisAction)
}

// With nobody able to act, skipped turns must still let fields fade. Otherwise a
// lone sleeper on a sleep field is put back to sleep on every turn, forever.
func TestCombatSleepFieldFadesWhenNobodyCanAct(t *testing.T) {
	gs, _ := NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().WithRandomSeed(5).Build()
	for index := 1; index < party_state.MaxActiveParty; index++ {
		if character := gs.PartyState.GetCharacter(index); character != nil && character.IsInParty() {
			character.PartyMap = party_state.PartyMapUnrecruited
		}
	}
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, CombatEntryNorth))
	avatar := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldSleep, gs.CombatScene.PartyPositions[0])))
	gs.setCombatPCSleeping(avatar, true)

	gs.BeginCombatTurns(CombatTurnsPresented)
	const stepLimit = 2000
	steps := 0
	for ; steps < stepLimit && gs.CombatScene.HasScheduledCombatStep(); steps++ {
		step, ok := gs.ReviewScheduledCombatStep()
		require.True(t, ok)
		require.Equal(t, CombatPresentationAutoPass, step.Kind)
		require.NoError(t, gs.CompleteScheduledCombatStep(step.Token))
	}

	require.Less(t, steps, stepLimit, "the sleeper must get a turn again")
	assert.False(t, gs.CombatScene.isCombatPCSleeping(avatar))
	assert.Empty(t, gs.CombatScene.Fields(), "the sleep field has faded")
}
