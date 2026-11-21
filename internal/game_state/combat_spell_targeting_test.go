package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func setupVasFlamTargeting(t *testing.T) (*GameState, *MockSystemCallbacks, *CombatMonster) {
	t.Helper()

	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	caster.Level = 3
	caster.Intelligence = 30
	caster.SetMp(30)
	gs.PartyState.Inventory.Spells.Set(references.VasFlam, 2)
	rawData := brickFloorCombatRaw()
	gs.CombatScene.CombatMapRef.SetRawData(rawData)
	mock.Reset()

	return gs, mock, monster
}

func TestBeginCombatSpellTargeting_RequiresActiveCaster(t *testing.T) {
	gs, _, _ := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	request := CastRequest{Spell: references.VasFlam, CasterIdx: activeIdx + 1}
	result := gs.BeginCombatSpellTargeting(request)

	assert.False(t, result.Valid)
	assert.Equal(t, "Not thy turn!", result.ErrorMsg)
	assert.Nil(t, gs.CombatScene.SpellTargeting)
}

func TestCombatActionTargetingBeginsRejectEveryExistingActionOwner(t *testing.T) {
	tests := []struct {
		name string
		own  func(*CombatState)
	}{
		{name: "attack targeting", own: func(combat *CombatState) { combat.AttackTargeting = &CombatAttackTargeting{} }},
		{name: "spell targeting", own: func(combat *CombatState) { combat.SpellTargeting = &CombatSpellTargeting{} }},
		{name: "cone targeting", own: func(combat *CombatState) { combat.ConeTargeting = &CombatConeTargeting{} }},
		{name: "adjacent targeting", own: func(combat *CombatState) { combat.AdjacentSpellTargeting = &CombatAdjacentSpellTargeting{} }},
		{name: "pending attack", own: func(combat *CombatState) { combat.pendingAttack = &pendingCombatAttack{} }},
		{name: "pending spell", own: func(combat *CombatState) { combat.pendingSpell = &pendingCombatSpell{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, _, _ := setupVasFlamTargeting(t)
			casterIdx := gs.CombatScene.GetActiveCombatantIndex()
			caster := gs.PartyState.GetCharacter(casterIdx)
			caster.Level = 8
			gs.PartyState.Inventory.Spells.Set(references.AnYlem, 1)
			gs.PartyState.Inventory.Spells.Set(references.InZu, 1)
			test.own(gs.CombatScene)

			assert.False(t, gs.BeginCombatAttack())
			assert.Equal(t, "Thou canst not cast now!", gs.BeginCombatSpellTargeting(CastRequest{
				Spell: references.VasFlam, CasterIdx: casterIdx,
			}).ErrorMsg)
			assert.Equal(t, "Thou canst not cast now!", gs.BeginCombatAdjacentSpellTargeting(CastRequest{
				Spell: references.AnYlem, CasterIdx: casterIdx,
			}).ErrorMsg)
			assert.Equal(t, "Thou canst not cast now!", gs.BeginCombatConeTargeting(CastRequest{
				Spell: references.InZu, CasterIdx: casterIdx,
			}).ErrorMsg)
		})
	}
}

func TestDeferredCombatSpellPreservesCloneCasterIdentity(t *testing.T) {
	gs, mock, monster := setupVasFlamTargeting(t)
	source := gs.CombatScene.GetCurrentTurnIdentity()
	clonePosition := references.Position{X: 1, Y: 1}
	clone := gs.CombatScene.addPCCloneFromSource(
		source,
		source.EntityIndex,
		clonePosition,
		NewCombatPCProvider(gs.PartyState.GetCharacter(source.EntityIndex)),
	)
	gs.CombatScene.CurrentTurnIdentity = clone
	gs.CombatScene.currentTurnIdentitySet = true
	require.True(t, gs.BeginCombatSpellTargeting(CastRequest{
		Spell: references.VasFlam, CasterIdx: source.EntityIndex,
	}).Valid)
	gs.CombatScene.SpellTargeting.Cursor = monster.Position()
	require.Equal(t, CombatSpellTargetResolved, gs.ConfirmCombatSpellTarget().Outcome)
	mock.Reset()

	require.True(t, gs.ResolvePendingCombatSpell())

	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, clone.Slot, mock.CombatEvents[0].Attacker.Slot)
	assert.Equal(t, clonePosition, mock.CombatEvents[0].Attacker.Position)
}

func TestValidateSpellResources_RejectsNonActiveCombatCasterWithoutConsumption(t *testing.T) {
	gs, _, _ := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	otherIdx := activeIdx + 1
	other := gs.PartyState.GetCharacter(otherIdx)
	require.NotNil(t, other)
	other.Level = 3
	other.Intelligence = 30
	other.SetMp(30)
	mpBefore := other.GetCurrentMp()
	spellsBefore := gs.PartyState.Inventory.Spells.Get(references.VasFlam)

	result := gs.ValidateAndConsumeSpellResources(CastRequest{
		Spell:     references.VasFlam,
		CasterIdx: otherIdx,
	})

	assert.False(t, result.Valid)
	assert.Equal(t, "Not thy turn!", result.ErrorMsg)
	assert.Equal(t, mpBefore, other.GetCurrentMp())
	assert.Equal(t, spellsBefore, gs.PartyState.Inventory.Spells.Get(references.VasFlam))
}

func TestConfirmCombatSpellTarget_GravPorAndXenCorpUseProjectileResolution(t *testing.T) {
	for _, spell := range []references.Spell{references.GravPor, references.XenCorp} {
		t.Run(references.GetSpellName(spell), func(t *testing.T) {
			gs, mock, monster := setupVasFlamTargeting(t)
			activeIdx := gs.CombatScene.GetActiveCombatantIndex()
			caster := gs.PartyState.GetCharacter(activeIdx)
			caster.Level = 8
			caster.Intelligence = 255
			caster.SetMp(30)
			gs.PartyState.Inventory.Spells.Set(spell, 1)
			mock.Reset()
			require.True(t, gs.BeginCombatSpellTargeting(CastRequest{Spell: spell, CasterIdx: activeIdx}).Valid)
			gs.CombatScene.SpellTargeting.Cursor = monster.Position()
			assert.Equal(t, CombatSpellTargetResolved, gs.ConfirmCombatSpellTarget().Outcome)
			require.True(t, gs.ResolvePendingCombatSpell())
			require.Len(t, mock.CombatEvents, 1)
			assert.Equal(t, spell, mock.CombatEvents[0].Diagnostics.Spell)
			if spell == references.XenCorp {
				assert.True(t, monster.IsDead())
				assert.Equal(t, combatInstantKillDamage, mock.CombatEvents[0].Diagnostics.FinalDamage)
			}
		})
	}
}

func TestCombatSpellTargeting_MovesAcrossWholeCombatMap(t *testing.T) {
	gs, _, _ := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	origin := references.Position{X: 5, Y: 5}
	gs.CombatScene.SetPartyMemberPosition(activeIdx, origin)
	require.True(t, gs.BeginCombatSpellTargeting(CastRequest{
		Spell:     references.VasFlam,
		CasterIdx: activeIdx,
	}).Valid)

	for range 5 {
		assert.True(t, gs.MoveCombatSpellCursor(1, 1))
	}
	assert.Equal(t, references.Position{X: 10, Y: 10}, gs.CombatScene.SpellTargeting.Cursor)
	assert.False(t, gs.MoveCombatSpellCursor(1, 0), "combat-map bounds constrain original range 15")
}

func TestConfirmCombatSpellTarget_SelfTargetKeepsAimingAndResources(t *testing.T) {
	gs, _, _ := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	mpBefore := caster.GetCurrentMp()
	spellsBefore := gs.PartyState.Inventory.Spells.Get(references.VasFlam)
	require.True(t, gs.BeginCombatSpellTargeting(CastRequest{
		Spell:     references.VasFlam,
		CasterIdx: activeIdx,
	}).Valid)

	result := gs.ConfirmCombatSpellTarget()

	assert.Equal(t, CombatSpellTargetInvalid, result.Outcome)
	assert.NotNil(t, gs.CombatScene.SpellTargeting)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, spellsBefore, gs.PartyState.Inventory.Spells.Get(references.VasFlam))
}

func TestCancelCombatSpellTargeting_PreservesResources(t *testing.T) {
	gs, _, _ := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	mpBefore := caster.GetCurrentMp()
	spellsBefore := gs.PartyState.Inventory.Spells.Get(references.VasFlam)
	require.True(t, gs.BeginCombatSpellTargeting(CastRequest{
		Spell:     references.VasFlam,
		CasterIdx: activeIdx,
	}).Valid)

	assert.Equal(t, CombatSpellTargetCancelled, gs.CancelCombatSpellTargeting())
	assert.Nil(t, gs.CombatScene.SpellTargeting)
	assert.Equal(t, mpBefore, caster.GetCurrentMp())
	assert.Equal(t, spellsBefore, gs.PartyState.Inventory.Spells.Get(references.VasFlam))
}

func TestConfirmCombatSpellTarget_VasFlamConsumesOnceAndPublishesMagicDamage(t *testing.T) {
	gs, mock, monster := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(activeIdx)
	mpBefore := caster.GetCurrentMp()
	spellsBefore := gs.PartyState.Inventory.Spells.Get(references.VasFlam)
	target := monster.Position()
	require.True(t, gs.BeginCombatSpellTargeting(CastRequest{
		Spell:     references.VasFlam,
		CasterIdx: activeIdx,
	}).Valid)
	gs.CombatScene.SpellTargeting.Cursor = target
	hpBefore := monster.CurrentHP()

	result := gs.ConfirmCombatSpellTarget()

	assert.Equal(t, CombatSpellTargetResolved, result.Outcome)
	assert.Nil(t, gs.CombatScene.SpellTargeting)
	assert.Equal(t, mpBefore-byte(3), caster.GetCurrentMp())
	assert.Equal(t, spellsBefore-1, gs.PartyState.Inventory.Spells.Get(references.VasFlam))
	assert.Equal(t, hpBefore, monster.CurrentHP(), "damage waits for projectile impact")
	assert.Empty(t, mock.CombatEvents)
	require.True(t, gs.ResolvePendingCombatSpell())
	assert.LessOrEqual(t, monster.CurrentHP(), hpBefore)
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, CombatEventSpell, event.Kind)
	assert.Equal(t, references.VasFlam, event.Diagnostics.Spell)
	assert.True(t, event.Diagnostics.MagicDamage)
	assert.True(t, event.Diagnostics.Hit.AutomaticHit)
	assert.Equal(t, hpBefore-monster.CurrentHP(), event.HPLost)
}

func TestCombatFieldSpellCrosshairCreatesAllFourFieldsOnEmptyAndOccupiedTiles(t *testing.T) {
	tests := []struct {
		spell references.Spell
		kind  CombatFieldKind
	}{
		{references.InFlamGrav, CombatFieldFire},
		{references.InNoxGrav, CombatFieldPoison},
		{references.InZuGrav, CombatFieldSleep},
		{references.InSanctGrav, CombatFieldProtection},
	}
	for _, test := range tests {
		for _, occupied := range []bool{false, true} {
			name := "empty"
			if occupied {
				name = "occupied"
			}
			t.Run(references.GetSpellName(test.spell)+"/"+name, func(t *testing.T) {
				gs, _, monster := setupVasFlamTargeting(t)
				casterIdx := gs.CombatScene.GetActiveCombatantIndex()
				caster := gs.PartyState.GetCharacter(casterIdx)
				caster.Level = 8
				gs.PartyState.Inventory.Spells.Set(test.spell, 1)
				target := monster.Position()
				if !occupied {
					target = references.Position{X: 1, Y: 1}
				}

				require.True(t, gs.BeginCombatSpellTargeting(CastRequest{Spell: test.spell, CasterIdx: casterIdx}).Valid)
				gs.CombatScene.SpellTargeting.Cursor = target
				result := gs.ConfirmCombatSpellTarget()

				require.Equal(t, CombatSpellTargetResolved, result.Outcome)
				assert.Equal(t, byte(30-references.GetSpellCircle(test.spell)), caster.GetCurrentMp())
				assert.Zero(t, gs.PartyState.Inventory.Spells.Get(test.spell))
				require.True(t, gs.ResolvePendingCombatSpell())
				require.Len(t, gs.CombatScene.Fields(), 1)
				assert.Equal(t, test.kind, gs.CombatScene.Fields()[0].Kind())
				assert.Equal(t, target, gs.CombatScene.Fields()[0].Position())
			})
		}
	}
}

func TestBlockedCombatFieldTrajectoryCommitsButSuppressesAllFourFields(t *testing.T) {
	for _, spell := range []references.Spell{
		references.InFlamGrav, references.InNoxGrav, references.InZuGrav, references.InSanctGrav,
	} {
		t.Run(references.GetSpellName(spell), func(t *testing.T) {
			gs, mock, _ := setupVasFlamTargeting(t)
			casterIdx := gs.CombatScene.GetActiveCombatantIndex()
			caster := gs.PartyState.GetCharacter(casterIdx)
			caster.Level = 8
			origin := references.Position{X: 3, Y: 3}
			target := references.Position{X: 6, Y: 3}
			gs.CombatScene.SetPartyMemberPosition(casterIdx, origin)
			raw := brickFloorCombatRaw()
			raw[4][3] = byte(indexes.StoneBrickWall)
			gs.CombatScene.CombatMapRef.SetRawData(raw)
			gs.PartyState.Inventory.Spells.Set(spell, 1)
			mock.Reset()

			require.True(t, gs.BeginCombatSpellTargeting(CastRequest{Spell: spell, CasterIdx: casterIdx}).Valid)
			gs.CombatScene.SpellTargeting.Cursor = target
			result := gs.ConfirmCombatSpellTarget()

			require.Equal(t, CombatSpellTargetResolved, result.Outcome)
			assert.True(t, result.Launch.Blocked)
			require.True(t, gs.ResolvePendingCombatSpell())
			assert.Empty(t, gs.CombatScene.Fields())
			require.Len(t, mock.CombatEvents, 1)
			assert.True(t, mock.CombatEvents[0].Diagnostics.ProjectileBlocked)
		})
	}
}

func TestCombatCalcMagicDamage_UsesOriginalDamageAndArmorRolls(t *testing.T) {
	rng := newSequenceRNG(29, 4)

	result := CombatCalcMagicDamage(30, 5, rng)

	assert.Equal(t, 30, result.AttackRoll)
	assert.Equal(t, 5, result.ArmorRoll)
	assert.Equal(t, 25, result.Damage)
	assert.False(t, result.Deflected)
	assert.Equal(t, 2, rng.index, "magic damage must retain its attack and armor RNG draws")
}

func TestCombatApplyMagicDamageModifiers_BypassesUndeadButNotImmortal(t *testing.T) {
	assert.Equal(t, 20, CombatApplyMagicDamageModifiers(20, true, false),
		"Vas Flam must bypass the physical undead halving rule")
	assert.Zero(t, CombatApplyMagicDamageModifiers(20, true, true),
		"original immortal monsters remain immune to magic damage")
}

// A missile stops on the first sampled blocking tile of its original pixel
// path; the final sample is not collision tested, so it still reaches a walled
// target square.
func TestCombatMissileImpactPosition(t *testing.T) {
	for _, test := range []struct {
		name                 string
		wall, from, to, want references.Position
		reached              bool
	}{
		{
			name: "stops at blocking terrain", wall: references.Position{X: 6, Y: 5},
			from: references.Position{X: 5, Y: 5}, to: references.Position{X: 7, Y: 5}, want: references.Position{X: 6, Y: 5},
		},
		{
			name: "follows the sampled pixel path", wall: references.Position{X: 0, Y: 1},
			from: references.Position{X: 0, Y: 0}, to: references.Position{X: 1, Y: 2}, want: references.Position{X: 0, Y: 1},
		},
		{
			name: "final sample not collision tested", wall: references.Position{X: 1, Y: 0},
			from: references.Position{X: 0, Y: 0}, to: references.Position{X: 1, Y: 0}, want: references.Position{X: 1, Y: 0},
			reached: true,
		},
	} {
		combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
		rawData := brickFloorCombatRaw()
		rawData[test.wall.X][test.wall.Y] = byte(indexes.StoneBrickWall)
		combatMap.SetRawData(rawData)

		impact, reached := combatMissileImpactPosition(combatMap.GetTileNumber, test.from, test.to, references.MissileNone)

		assert.Equal(t, test.reached, reached, test.name)
		assert.Equal(t, test.want, impact, test.name)
	}
}

func TestTraceCombatProjectileLaunch_PreservesAimAndRecordsBlockedImpact(t *testing.T) {
	combatMap := references.NewCombatMapReference(0, references.BritanniaCombatTerritory)
	rawData := brickFloorCombatRaw()
	rawData[6][5] = byte(indexes.StoneBrickWall)
	combatMap.SetRawData(rawData)
	from := references.Position{X: 5, Y: 5}
	aim := references.Position{X: 7, Y: 5}

	launch := traceCombatProjectileLaunch(combatMap.GetTileNumber, from, aim, references.MissileBlue)

	assert.Equal(t, from, launch.From)
	assert.Equal(t, aim, launch.Aim)
	assert.Equal(t, references.Position{X: 6, Y: 5}, launch.Impact)
	assert.Equal(t, references.MissileBlue, launch.Missile)
	assert.True(t, launch.Blocked)
}

func TestConfirmCombatSpellTarget_BlockedVasFlamConsumesCastWithoutDamage(t *testing.T) {
	gs, mock, _ := setupVasFlamTargeting(t)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	origin := references.Position{X: 5, Y: 5}
	targetPos := references.Position{X: 7, Y: 5}
	gs.CombatScene.SetPartyMemberPosition(activeIdx, origin)
	clearCombatMonstersForTest(gs.CombatScene)
	enemyRefs := *gs.GameReferences.EnemyReferences
	monster := NewCombatMonster(
		references.MonsterOrc,
		&enemyRefs[references.MonsterOrc],
		targetPos,
		newFixedRNG(1),
	)
	gs.CombatScene.AddMonsterToQueue(monster)
	rawData := brickFloorCombatRaw()
	rawData[6][5] = byte(indexes.StoneBrickWall)
	gs.CombatScene.CombatMapRef.SetRawData(rawData)
	caster := gs.PartyState.GetCharacter(activeIdx)
	mpBefore := caster.GetCurrentMp()
	spellsBefore := gs.PartyState.Inventory.Spells.Get(references.VasFlam)
	hpBefore := monster.CurrentHP()
	mock.Reset()
	require.True(t, gs.BeginCombatSpellTargeting(CastRequest{
		Spell:     references.VasFlam,
		CasterIdx: activeIdx,
	}).Valid)
	gs.CombatScene.SpellTargeting.Cursor = targetPos

	result := gs.ConfirmCombatSpellTarget()

	assert.Equal(t, CombatSpellTargetResolved, result.Outcome)
	assert.Equal(t, origin, result.Launch.From)
	assert.Equal(t, targetPos, result.Launch.Aim)
	assert.Equal(t, references.Position{X: 6, Y: 5}, result.Launch.Impact)
	assert.Equal(t, references.MissileNone, result.Launch.Missile)
	assert.True(t, result.Launch.Blocked)
	assert.Equal(t, hpBefore, monster.CurrentHP())
	assert.Equal(t, mpBefore-byte(3), caster.GetCurrentMp())
	assert.Equal(t, spellsBefore-1, gs.PartyState.Inventory.Spells.Get(references.VasFlam))
	assert.Empty(t, mock.CombatEvents)
	require.True(t, gs.ResolvePendingCombatSpell())
	require.Len(t, mock.CombatEvents, 1)
	assert.True(t, mock.CombatEvents[0].Diagnostics.ProjectileBlocked)
}

// brickFloorCombatRaw returns combat map raw data covered in brick floor.
func brickFloorCombatRaw() *[references.XCombatMapTiles][references.YCombatMapTiles]byte {
	raw := &[references.XCombatMapTiles][references.YCombatMapTiles]byte{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			raw[x][y] = byte(indexes.BrickFloor)
		}
	}

	return raw
}
