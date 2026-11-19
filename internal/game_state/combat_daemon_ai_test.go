package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

func setupHostileDaemonTest(t *testing.T) (*GameState, *MockSystemCallbacks, *CombatMonster, int) {
	t.Helper()

	return setupHostileSpecialMonsterTest(t, references.MonsterDevil)
}

func setupHostileSpecialMonsterTest(
	t *testing.T,
	monsterType references.MonsterType,
) (*GameState, *MockSystemCallbacks, *CombatMonster, int) {
	t.Helper()
	gs, mock := newBritainCombatTestState(t, 1)
	clearCombatMonstersForTest(gs.CombatScene)
	monsterRef := &(*gs.GameReferences.EnemyReferences)[monsterType]
	monster := NewCombatMonster(monsterType, monsterRef, references.Position{X: 0, Y: 0}, newFixedRNG(1))
	monsterIndex := gs.CombatScene.AddMonsterToQueue(monster)
	for index := 1; index < party_state.MaxActiveParty; index++ {
		gs.CombatScene.PartyActive[index] = false
		gs.CombatScene.TurnQueue.MarkFled(index, true)
	}
	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.PartyPositions[0] = references.Position{X: 10, Y: 10}
	gs.PartyState.GetCharacter(0).SetStatus(party_state.Good)
	mock.Reset()

	return gs, mock, monster, monsterIndex
}

func TestHostileDaemonPossessionRawSlotEligibilityAndSave(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)

	assert.False(t, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex,
		newSequenceRNG(6, 255)), "raw slot 6 is not a PC and must continue past possession")
	assert.False(t, gs.CombatScene.PartyPossessed[0])
	assert.Empty(t, mock.CombatEvents)

	// A low d30 roll saves against the daemon's INT under exact saveint semantics.
	gs.PartyState.GetCharacter(0).Intelligence = 30
	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex,
		newSequenceRNG(0, 0)))
	assert.False(t, gs.CombatScene.PartyPossessed[0])
	assert.Empty(t, mock.CombatEvents, "the original successful save is silent")
	assert.True(t, daemon.IsActive(), "a saved possession does not retire the daemon")
}

func TestShadowlordPossessionTargetsExactCloneSourceSlot(t *testing.T) {
	gs, mock, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)
	source := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	sourceEntry := gs.CombatScene.TurnQueue.GetEntryByIdentity(source)
	require.NotNil(t, sourceEntry)
	sourceEntry.turnCountdown = 13
	clonePosition := references.Position{X: 8, Y: 8}
	clone := gs.CombatScene.addPCCloneFromSource(
		source, 0, clonePosition, NewCombatPCProvider(gs.PartyState.GetCharacter(0)),
	)
	cloneEntry := gs.CombatScene.TurnQueue.GetEntryByIdentity(clone)
	require.NotNil(t, cloneEntry)
	assert.Equal(t, 1, cloneEntry.sourceSlot)
	assert.Equal(t, sourceEntry.turnCountdown, cloneEntry.turnCountdown)
	gs.PartyState.GetCharacter(0).Intelligence = 1
	mock.Reset()

	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(
		shadowlord, shadowlordIndex, newSequenceRNG(cloneEntry.sourceSlot, 29),
	))

	assert.True(t, gs.CombatScene.isCombatPCPossessed(clone))
	assert.False(t, gs.CombatScene.isCombatPCPossessed(source),
		"a clone's CHARM condition must not alias its source entry")
	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(0).GetStatus())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, clonePosition, mock.CombatEvents[0].Target.Position)
}

func TestLoneDaemonPossessionRemainsHostileWhilePartyLoyalPCRemains(t *testing.T) {
	gs, _, daemon, daemonIndex := setupHostileDaemonTest(t)
	second := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}
	secondEntry := gs.CombatScene.TurnQueue.GetEntryByIdentity(second)
	require.NotNil(t, secondEntry)
	secondEntry.status = turnQueueActive
	gs.CombatScene.PartyActive[1] = true
	gs.PartyState.GetCharacter(0).Intelligence = 1

	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(
		daemon, daemonIndex, newSequenceRNG(0, 29),
	))
	assert.False(t, daemon.IsActive())

	assert.Equal(t, CombatNotEnded, gs.checkCombatEndConditions())
	assert.True(t, gs.CombatScene.PartyPossessed[0],
		"zero ordinary monsters is not victory while a possessed PC remains monster-loyal")
	assert.False(t, gs.CombatScene.isCombatPCSleeping(
		CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0},
	))
}

func TestPossessedPCConfusionRerollsForAttackAndSeekSelection(t *testing.T) {
	gs, _, daemon, daemonIndex := setupHostileDaemonTest(t)
	attacker := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	ally := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}
	allyEntry := gs.CombatScene.TurnQueue.GetEntryByIdentity(ally)
	require.NotNil(t, allyEntry)
	allyEntry.status = turnQueueActive
	gs.CombatScene.PartyActive[1] = true
	gs.CombatScene.setCombatPCPossessed(attacker, true)
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 5, Y: 5})
	gs.CombatScene.SetPartyMemberPosition(1, references.Position{X: 6, Y: 5})
	daemon.SetPosition(references.Position{X: 5, Y: 6})
	gs.PartyState.GetCharacter(0).Intelligence = 10
	gs.ActivateConfusionEffect(3)
	rng := newSequenceRNG(0, 29)

	plan := gs.planPossessedPCTurn(attacker)
	attackTarget := gs.selectPossessedPCOpponentWithRNG(attacker, rng)
	seekTarget := gs.selectPossessedPCOpponentWithRNG(attacker, rng)

	assert.False(t, plan.HasTarget, "a confusion-dependent target cannot be advertised before its RNG roll")
	assert.Equal(t, ally, attackTarget.identity)
	assert.False(t, seekTarget.identity.IsPartyMember)
	assert.Equal(t, daemonIndex, seekTarget.identity.EntityIndex)
	assert.Equal(t, 2, rng.index)
}

func TestPartyBoundDaemonRoutesToPlayerControlWithoutPossession(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
	daemon.SetLoyalty(CombatMonsterLoyaltyParty)
	forceNextMonsterTurnForTest(t, gs, daemonIndex)

	gs.AdvanceCombatTurn()

	assert.False(t, gs.CombatScene.HasScheduledCombatStep())
	assert.Equal(t, CombatTurnIdentity{IsPartyMember: false, EntityIndex: daemonIndex},
		gs.CombatScene.GetCurrentTurnIdentity())
	assert.Equal(t, daemonIndex, gs.CombatScene.ActiveMonsterIndex)
	assert.False(t, gs.CombatScene.PartyPossessed[0])
	for _, event := range mock.CombatEvents {
		assert.NotEqual(t, CombatEventPossession, event.Kind)
	}
}

func TestDaemonPossessionRejectsInactiveInvisibleDeadSleepingAndCharmedPCs(t *testing.T) {
	for _, test := range []struct {
		name  string
		setup func(*GameState)
	}{
		{name: "inactive", setup: func(gs *GameState) { gs.CombatScene.PartyActive[0] = false }},
		{name: "invisible", setup: func(gs *GameState) { gs.CombatScene.PartyInvisible[0] = true }},
		{name: "dead", setup: func(gs *GameState) { gs.PartyState.GetCharacter(0).SetStatus(party_state.Dead) }},
		{name: "sleeping", setup: func(gs *GameState) {
			gs.setCombatPCSleeping(CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}, true)
		}},
		{name: "charmed", setup: func(gs *GameState) { gs.PartyState.GetCharacter(0).SetStatus(party_state.Charmed) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
			test.setup(gs)
			assert.False(t, gs.executeHostileMonsterSpecialMoveWithRNG(
				daemon, daemonIndex, newSequenceRNG(0, 255),
			))
			assert.False(t, gs.CombatScene.PartyPossessed[0])
			assert.Empty(t, mock.CombatEvents)
		})
	}
}

func TestHostileDaemonSuccessfulPossessionConsumesTurnAndRetiresWithoutCorpse(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
	gs.PartyState.GetCharacter(0).Intelligence = 1

	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex,
		newSequenceRNG(0, 29)))
	assert.True(t, gs.CombatScene.PartyPossessed[0])
	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(0).GetStatus(),
		"possession is an encounter-entry condition, not persistent character status")
	assert.False(t, daemon.IsActive())
	assert.False(t, daemon.IsDead(), "possession retires rather than kills the daemon")
	assert.Zero(t, gs.CombatScene.ObjectCount())
	require.Len(t, mock.CombatEvents, 2)
	assert.Equal(t, CombatEventPossession, mock.CombatEvents[0].Kind)
	assert.Equal(t, CombatEventRetired, mock.CombatEvents[1].Kind)
}

func TestShadowlordSuccessfulPossessionConsumesTurnWithoutRetiring(t *testing.T) {
	gs, mock, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)
	gs.PartyState.GetCharacter(0).Intelligence = 1

	rng := newSequenceRNG(0, 29)
	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(shadowlord, shadowlordIndex, rng))
	assert.Equal(t, 2, rng.index, "successful possession must consume no later special-action RNG")
	assert.True(t, gs.CombatScene.PartyPossessed[0])
	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(0).GetStatus())
	assert.True(t, shadowlord.IsActive(), "only Daemons retire after successful possession")
	assert.False(t, shadowlord.IsDead())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventPossession, mock.CombatEvents[0].Kind)
}

func TestMonsterInvisibilityFollowsPossessionAndPrecedesGating(t *testing.T) {
	gs, mock, blackthorn, blackthornIndex := setupHostileSpecialMonsterTest(t, references.MonsterBlackthorn)
	require.True(t, blackthorn.HasAbility(references.PossessCharm))
	require.True(t, blackthorn.HasAbility(references.Invisibility))
	require.True(t, blackthorn.HasAbility(references.GatesInDaemon))
	monstersBefore := gs.CombatScene.MonsterManager.Count()

	rng := newSequenceRNG(31, monsterSpecialThreshold-1)
	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(blackthorn, blackthornIndex, rng))
	assert.Equal(t, 2, rng.index, "a visibility toggle must consume the turn before gating RNG")
	assert.True(t, blackthorn.IsInvisible())
	assert.Equal(t, monstersBefore, gs.CombatScene.MonsterManager.Count())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventVisibility, mock.CombatEvents[0].Kind)
	assert.Equal(t, CombatVisibilityHidden, mock.CombatEvents[0].Visibility)

	mock.Reset()
	rng = newSequenceRNG(31, monsterSpecialThreshold-1)
	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(blackthorn, blackthornIndex, rng))
	assert.False(t, blackthorn.IsInvisible())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatVisibilityRevealed, mock.CombatEvents[0].Visibility)
}

func TestMonsterInvisibilityBoundaryAndPossessionPrecedence(t *testing.T) {
	gs, mock, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)

	rng := newSequenceRNG(31, monsterSpecialThreshold)
	assert.False(t, gs.executeHostileMonsterSpecialMoveWithRNG(shadowlord, shadowlordIndex, rng))
	assert.Equal(t, 2, rng.index)
	assert.False(t, shadowlord.IsInvisible())
	assert.Empty(t, mock.CombatEvents)

	gs.PartyState.GetCharacter(0).Intelligence = 30
	rng = newSequenceRNG(0, 0, monsterSpecialThreshold-1)
	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(shadowlord, shadowlordIndex, rng))
	assert.Equal(t, 2, rng.index, "an eligible possession save consumes the turn before invisibility")
	assert.False(t, shadowlord.IsInvisible())
	assert.Empty(t, mock.CombatEvents, "a successful possession save is silent")
}

func TestMagicSuppressionSkipsMonsterSpecialRNG(t *testing.T) {
	for _, test := range []struct {
		name     string
		activate func(*GameState)
	}{
		{name: "crown", activate: func(gs *GameState) { gs.ToggleCrown() }},
		{name: "negate magic", activate: func(gs *GameState) { gs.ActivateNegateMagicEffect(3) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, mock, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)
			test.activate(gs)
			rng := newSequenceRNG(0)

			assert.False(t, gs.executeHostileMonsterSpecialMoveWithRNG(shadowlord, shadowlordIndex, rng))
			assert.Zero(t, rng.index)
			assert.False(t, shadowlord.IsInvisible())
			assert.Empty(t, mock.CombatEvents)
		})
	}
}

func TestOrdinaryMonsterDoesNotConsumeSpecialRNG(t *testing.T) {
	gs, mock, orc, orcIndex := setupHostileSpecialMonsterTest(t, references.MonsterOrc)
	rng := newSequenceRNG(0)

	assert.False(t, gs.executeHostileMonsterSpecialMoveWithRNG(orc, orcIndex, rng))
	assert.Zero(t, rng.index)
	assert.Empty(t, mock.CombatEvents)
}

func TestPossessedPCAIUsesOnlyAdjacentTwoHandedSwordOrMovement(t *testing.T) {
	gs, mock, _, _ := setupHostileDaemonTest(t)
	gs.CombatScene.setCombatPCPossessed(CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}, true)
	gs.CombatScene.PartyPositions[0] = references.Position{X: 5, Y: 5}
	gs.CombatScene.PartyActive[1] = true
	gs.CombatScene.TurnQueue.GetEntry(1, true).status = turnQueueActive
	gs.CombatScene.PartyPositions[1] = references.Position{X: 6, Y: 5}
	gs.PartyState.GetCharacter(1).SetStatus(party_state.Good)
	hpBefore := gs.PartyState.GetCharacter(1).GetCurrentHp()
	gs.SetRandomSeed(2)

	gs.executePossessedPCTurn(CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0})

	assert.Equal(t, []SoundEffect{SoundCombatSwing}, mock.SoundEffectsPlayed)
	require.NotEmpty(t, mock.CombatEvents)
	event := mock.CombatEvents[len(mock.CombatEvents)-1]
	assert.Equal(t, CombatEventAttack, event.Kind)
	assert.Equal(t, references.TwoHSword, event.Diagnostics.Source)
	assert.LessOrEqual(t, gs.PartyState.GetCharacter(1).GetCurrentHp(), hpBefore)
	assert.True(t, event.Attacker.IsPartyMember)
	assert.True(t, event.Target.IsPartyMember)
}

func TestChaosSwordInterceptsEitherHandBeforePlayerInput(t *testing.T) {
	for _, slot := range []references.EquipmentSlot{references.SlotWeapon, references.SlotShield} {
		t.Run(slot.String(), func(t *testing.T) {
			gs, mock, _, _ := setupHostileDaemonTest(t)
			identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
			gs.PartyState.GetCharacter(0).SetEquippedItem(slot, references.ChaosSword)
			forceNextCombatActor(gs.CombatScene.TurnQueue, 0, true)
			gs.CombatScene.presentTurns = true
			mock.Reset()

			gs.AdvanceCombatTurn()
			step, scheduled := gs.ReviewScheduledCombatStep()

			require.True(t, scheduled)
			assert.Equal(t, CombatPresentationMonsterTurn, step.Kind)
			assert.Equal(t, identity, step.Actor)
			assert.True(t, gs.CombatScene.isCombatPCPossessed(identity))
			assert.Equal(t, -1, gs.GetActivePlayerIndex())
			assert.Empty(t, mock.CommandPrompts)
		})
	}
}

func TestChaosSwordDoesNotInterceptSoloModeAutoPass(t *testing.T) {
	gs, mock, _, _ := setupHostileDaemonTest(t)
	identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	second := gs.CombatScene.TurnQueue.GetEntry(1, true)
	require.NotNil(t, second)
	second.status = turnQueueActive
	second.sourceSlotReleased = false
	gs.CombatScene.PartyActive[1] = true
	gs.PartyState.GetCharacter(1).SetStatus(party_state.Good)
	gs.PartyState.GetCharacter(0).SetEquippedItem(references.SlotWeapon, references.ChaosSword)
	gs.CombatScene.SetSoloMoveCharacter(1)
	forceNextCombatActor(gs.CombatScene.TurnQueue, 0, true)
	second.turnCountdown = 1
	gs.CombatScene.presentTurns = true
	mock.Reset()

	gs.AdvanceCombatTurn()

	assert.False(t, gs.CombatScene.HasScheduledCombatStep())
	assert.Equal(t, CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1},
		gs.CombatScene.GetCurrentTurnIdentity())
	assert.False(t, gs.CombatScene.isCombatPCPossessed(identity))
	assert.Equal(t, references.ChaosSword,
		gs.PartyState.GetCharacter(0).GetEquippedItem(references.SlotWeapon))
}

func TestPossessedPCMonsterSeekRejectsLooseItemSquare(t *testing.T) {
	gs, _, _, _ := setupHostileDaemonTest(t)
	identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	from := references.Position{X: 5, Y: 5}
	to := references.Position{X: 5, Y: 6}
	gs.CombatScene.SetPartyMemberPosition(identity.EntityIndex, from)
	gs.CombatScene.AddObject(NewCombatLooseItem(to, references.Food, 1, indexes.ItemFood))

	assert.False(t, gs.canPossessedPCMove(identity, 0, 1))
}

func TestCrownAndNegateMagicSuppressDaemonSpecialsAndRangedMagic(t *testing.T) {
	for _, test := range []struct {
		name     string
		activate func(*GameState)
	}{
		{name: "crown", activate: func(gs *GameState) { gs.ToggleCrown() }},
		{name: "negate magic", activate: func(gs *GameState) { gs.ActivateNegateMagicEffect(3) }},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
			test.activate(gs)
			from, to := clearRangeNinePositions(t, gs.CombatScene.CombatMapRef)
			daemon.SetPosition(from)
			gs.CombatScene.PartyPositions[0] = to
			assert.False(t, gs.executeHostileMonsterSpecialMove(daemon, daemonIndex))
			nearest := gDaemonNearestPC(gs, daemon, daemonIndex)
			assert.False(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(0)))
			assert.Empty(t, mock.CombatEvents)

			gs.CombatScene.PartyPositions[0] = references.Position{X: from.X + 1, Y: from.Y}
			nearest = gDaemonNearestPC(gs, daemon, daemonIndex)
			gs.SetRandomSeed(2)
			assert.True(t, gs.monsterAttack(daemon, daemonIndex, nearest),
				"magic suppression must not disable adjacent melee")
			require.NotEmpty(t, mock.CombatEvents)
			assert.Equal(t, CombatEventAttack, mock.CombatEvents[len(mock.CombatEvents)-1].Kind)
		})
	}
}

func TestDaemonGateOneEighthBoundaryAndCapacity(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
	position := firstLegalDaemonGatePosition(t, gs)

	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex,
		newSequenceRNG(31, 31, int(position.X), int(position.Y), 1)))
	assert.Equal(t, 2, gs.CombatScene.MonsterManager.Count())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventGating, mock.CombatEvents[0].Kind)
	assert.True(t, mock.CombatEvents[0].Created.Index >= 0)
	assert.True(t, gs.CombatScene.MonsterManager.GetMonster(mock.CombatEvents[0].Created.Index).IsHostile())

	gs2, mock2, daemon2, daemonIndex2 := setupHostileDaemonTest(t)
	assert.False(t, gs2.executeHostileMonsterSpecialMoveWithRNG(daemon2, daemonIndex2,
		newSequenceRNG(31, 32)))
	assert.Empty(t, mock2.CombatEvents)

	for gs2.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart) {
		ref := &(*gs2.GameReferences.EnemyReferences)[references.MonsterOrc]
		monster := NewCombatMonster(references.MonsterOrc, ref, references.Position{X: 5, Y: 5}, newFixedRNG(1))
		gs2.CombatScene.AddMonsterToQueue(monster)
	}
	assert.False(t, gs2.executeHostileMonsterSpecialMoveWithRNG(daemon2, daemonIndex2,
		newSequenceRNG(31, 31, int(position.X), int(position.Y))))
	assert.False(t, gs2.CombatScene.TurnQueue.hasFreeSourceSlot(sourceMonsterSlotStart))
}

func TestDaemonGateUsesDevilTerrainAndOccupancy(t *testing.T) {
	for _, test := range []struct {
		name     string
		tile     indexes.SpriteIndex
		occupant string
		wantGate bool
	}{
		{name: "water", tile: indexes.Water1, wantGate: true},
		{name: "river", tile: indexes.WaterStream1, wantGate: true},
		{name: "chair", tile: indexes.ChairBackForward},
		{name: "floor", tile: indexes.BrickFloor, wantGate: true},
		{name: "occupied by PC", tile: indexes.Water1, occupant: "PC"},
		{name: "occupied by monster", tile: indexes.BrickFloor, occupant: "monster"},
		{name: "occupied by object", tile: indexes.BrickFloor, occupant: "object"},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
			require.NotNil(t, gs)
			position := references.Position{X: 5, Y: 5}
			gs.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, test.tile)
			switch test.occupant {
			case "PC":
				gs.CombatScene.SetPartyMemberPosition(0, position)
			case "monster":
				daemon.SetPosition(position)
			case "object":
				gs.CombatScene.AddObject(NewCombatLooseItem(position, references.Food, 1, indexes.ItemFood))
			}
			rolls := []int{31, 0, int(position.X), int(position.Y)}
			wantRequests := []int{32, 256, 16, 16}
			if test.wantGate {
				rolls = append(rolls, 1)
				wantRequests = append(wantRequests, 8)
			}
			rng := newStrictSummonRNG(t, rolls...)
			assert.Equal(t, test.wantGate, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex, rng))
			assert.Equal(t, wantRequests, rng.requests, "only accepted placement consumes creation DEX")
			if test.wantGate {
				require.Len(t, mock.CombatEvents, 1)
				created := gs.CombatScene.MonsterManager.GetMonster(mock.CombatEvents[0].Created.Index)
				assert.Equal(t, references.MonsterDevil, created.MonsterType())
				assert.Equal(t, position, created.Position())
			} else {
				assert.Empty(t, mock.CombatEvents)
				assert.Equal(t, 1, gs.CombatScene.MonsterManager.Count())
			}
		})
	}
}

func TestDaemonGateTestHelperUsesDevilRatherThanDefaultMage(t *testing.T) {
	gs, _, _, _ := setupHostileDaemonTest(t)
	require.NotNil(t, gs)
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			gs.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, indexes.ChairBackForward)
		}
	}
	water := references.Position{X: 5, Y: 5}
	gs.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &water, indexes.Water1)
	assert.Equal(t, water, firstLegalDaemonGatePosition(t, gs), "chairs admit a Mage, but only water admits this Devil probe")
}

func TestDaemonGateCanExceedInitialSixteenMonsterGroup(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
	position := firstLegalDaemonGatePosition(t, gs)
	for gs.CombatScene.MonsterManager.Count() < references.MaxMonstersInCombat {
		ref := &(*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
		monster := NewCombatMonster(
			references.MonsterOrc, ref, references.Position{X: 5, Y: 5}, newFixedRNG(1),
		)
		gs.CombatScene.AddMonsterToQueue(monster)
	}
	mock.Reset()

	assert.True(t, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex,
		newSequenceRNG(31, 31, int(position.X), int(position.Y))))
	assert.Equal(t, references.MaxMonstersInCombat+1, gs.CombatScene.MonsterManager.Count())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventGating, mock.CombatEvents[0].Kind)
}

func TestDaemonGateRequiresFreeOriginalObjectSlot(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
	position := firstLegalDaemonGatePosition(t, gs)
	fillCombatObjectTableWithFields(t, gs.CombatScene, references.Position{X: 0, Y: 0})
	mock.Reset()

	assert.False(t, gs.executeHostileMonsterSpecialMoveWithRNG(daemon, daemonIndex,
		newSequenceRNG(31, 31, int(position.X), int(position.Y))))
	assert.Empty(t, mock.CombatEvents)
}

func TestDaemonRangeNineRedMissileChoiceObstructionTurningAndAlliedMonsterTarget(t *testing.T) {
	gs, mock, daemon, daemonIndex := setupHostileDaemonTest(t)
	from, to := clearRangeNinePositions(t, gs.CombatScene.CombatMapRef)
	daemon.SetPosition(from)
	gs.CombatScene.PartyPositions[0] = to
	nearest := gDaemonNearestPC(gs, daemon, daemonIndex)
	assert.Equal(t, 9, nearest.distance)

	assert.False(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(128)))
	assert.Empty(t, mock.CombatEvents)
	assert.True(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(127, 29, 0, 0)))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventRangedAttack, mock.CombatEvents[0].Kind)
	assert.Equal(t, references.MissileRed, mock.CombatEvents[0].Diagnostics.Missile)

	mock.Reset()
	gs.PartyState.GetCharacter(0).SetEquippedItem(references.SlotAmulet, references.AmuletOfTurning)
	assert.True(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(127, 2, 1)))
	require.Len(t, mock.CombatEvents, 1)
	assert.True(t, mock.CombatEvents[0].Diagnostics.ForcedMiss)
	assert.Equal(t, CombatFeedbackMiss, mock.CombatEvents[0].Result)
	mock.Reset()
	assert.True(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(128, 0, 29, 0, 0)))
	require.Len(t, mock.CombatEvents, 1)
	assert.False(t, mock.CombatEvents[0].Diagnostics.ForcedMiss)

	gs.CombatScene.PartyActive[0] = false
	allyRef := &(*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	ally := NewCombatMonster(references.MonsterOrc, allyRef, to, newFixedRNG(1))
	ally.SetLoyalty(CombatMonsterLoyaltyParty)
	allyIndex := gs.CombatScene.AddMonsterToQueue(ally)
	nearest = gDaemonNearestPC(gs, daemon, daemonIndex)
	assert.False(t, nearest.identity.IsPartyMember)
	assert.Equal(t, allyIndex, nearest.identity.EntityIndex)
	mock.Reset()
	assert.True(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(0, 29, 0, 0)))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, allyIndex, mock.CombatEvents[0].Target.Index)
	assert.False(t, mock.CombatEvents[0].Target.IsPartyMember)

	blockedMap, blockedFrom, blockedTo := obstructedCombatPath(t, gs)
	gs.CombatScene.CombatMapRef = blockedMap
	ally.SetActive(true)
	ally.SetCurrentHP(ally.EnemyRef().HitPoints)
	gs.CombatScene.TurnQueue.GetEntry(allyIndex, false).status = turnQueueActive
	daemon.SetPosition(blockedFrom)
	ally.SetPosition(blockedTo)
	nearest = gDaemonNearestPC(gs, daemon, daemonIndex)
	mock.Reset()
	assert.True(t, gs.monsterAttackWithRNG(daemon, daemonIndex, nearest, newSequenceRNG(0, 29)))
	require.Len(t, mock.CombatEvents, 1)
	assert.True(t, mock.CombatEvents[0].Diagnostics.ProjectileBlocked)
}

func TestDaemonBadtripRecoversOneAtATimeAndCharmNeverPersists(t *testing.T) {
	gs, mock, daemon, _ := setupHostileDaemonTest(t)
	daemon.SetActive(false)
	for index := 0; index < 2; index++ {
		gs.CombatScene.PartyActive[index] = true
		gs.CombatScene.PartyPossessed[index] = true
	}
	gs.PartyState.GetCharacter(0).SetEquippedItem(references.SlotWeapon, references.ChaosSword)
	chaosSwordsBefore := gs.PartyState.Inventory.Equipment.Get(references.ChaosSword)
	mock.Reset()

	assert.Equal(t, CombatNotEnded, gs.checkCombatEndConditions())
	assert.False(t, gs.CombatScene.PartyPossessed[0])
	assert.True(t, gs.CombatScene.PartyPossessed[1])
	assert.Equal(t, party_state.Sleep, gs.PartyState.GetCharacter(0).GetStatus())
	assert.Equal(t, references.NoEquipment, gs.PartyState.GetCharacter(0).GetEquippedItem(references.SlotWeapon))
	assert.Equal(t, chaosSwordsBefore, gs.PartyState.Inventory.Equipment.Get(references.ChaosSword))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventRecovery, mock.CombatEvents[0].Kind)

	gs.TransitionFromCombatMap()
	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(1).GetStatus())
}

func TestDaemonBadtripConsumesEarlierDeadPossessedEntry(t *testing.T) {
	gs, mock, _, _ := setupHostileDaemonTest(t)
	dead := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	living := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 1}
	gs.CombatScene.setCombatPCPossessed(dead, true)
	gs.CombatScene.setCombatPCPossessed(living, true)
	character := gs.PartyState.GetCharacter(dead.EntityIndex)
	character.SetEquippedItem(references.SlotWeapon, references.ChaosSword)
	character.SetHp(0)
	character.MarkAsDead()
	gs.markCombatPCDead(dead.EntityIndex)
	mock.Reset()

	require.True(t, gs.recoverOnePossessedPC())

	assert.False(t, gs.CombatScene.isCombatPCPossessed(dead))
	assert.True(t, gs.CombatScene.isCombatPCPossessed(living))
	assert.True(t, character.IsDead())
	assert.Equal(t, references.NoEquipment, character.GetEquippedItem(references.SlotWeapon))
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, dead.EntityIndex, mock.CombatEvents[0].Target.Index)
}

func TestBadtripConsumesOneChaosSwordInOriginalHandOrder(t *testing.T) {
	for _, test := range []struct {
		name         string
		mainHand     references.Equipment
		offHand      references.Equipment
		wantMainHand references.Equipment
		wantOffHand  references.Equipment
	}{
		{
			name: "off hand", mainHand: references.LongSword, offHand: references.ChaosSword,
			wantMainHand: references.LongSword, wantOffHand: references.NoEquipment,
		},
		{
			name: "main hand first", mainHand: references.ChaosSword, offHand: references.ChaosSword,
			wantMainHand: references.NoEquipment, wantOffHand: references.ChaosSword,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, mock, _, _ := setupHostileDaemonTest(t)
			identity := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
			character := gs.PartyState.GetCharacter(0)
			character.SetEquippedItem(references.SlotWeapon, test.mainHand)
			character.SetEquippedItem(references.SlotShield, test.offHand)
			gs.CombatScene.setCombatPCPossessed(identity, true)
			inventoryBefore := gs.PartyState.Inventory.Equipment.Get(references.ChaosSword)
			mock.Reset()

			require.True(t, gs.recoverOnePossessedPC())

			assert.False(t, gs.CombatScene.isCombatPCPossessed(identity))
			assert.True(t, gs.CombatScene.isCombatPCSleeping(identity))
			assert.Equal(t, test.wantMainHand, character.GetEquippedItem(references.SlotWeapon))
			assert.Equal(t, test.wantOffHand, character.GetEquippedItem(references.SlotShield))
			assert.Equal(t, inventoryBefore, gs.PartyState.Inventory.Equipment.Get(references.ChaosSword))
			assert.Contains(t, mock.Messages, character.GetNameAsString()+" passes out!")
			require.Len(t, mock.CombatEvents, 1)
			assert.Equal(t, CombatEventRecovery, mock.CombatEvents[0].Kind)
		})
	}
}

func TestShadowlordEscapeRecoversPossessedPCWhileOtherMonsterRemains(t *testing.T) {
	gs, mock, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)
	addHostileOrcForShadowlordRecoveryTest(gs)
	victim := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	require.True(t, gs.CombatScene.acquireCombatPossession(victim, shadowlord.CombatantID()))
	opponent := gs.CombatScene.TurnQueue.GetEntry(1, true)
	require.NotNil(t, opponent)
	opponent.status = turnQueueActive
	opponent.sourceSlotReleased = false
	gs.CombatScene.PartyActive[1] = true
	gs.CombatScene.SetPartyMemberPosition(1, references.Position{X: 2, Y: 5})
	shadowlord.SetCurrentHP(1)
	shadowlord.SetFleeing(true)
	shadowlord.SetPosition(references.Position{X: 0, Y: 5})
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 2, Y: 5})
	mock.Reset()

	handled, teleported := gs.executeFleeingMonsterTurnWithRNG(
		shadowlord, shadowlordIndex, newSequenceRNG(0, 3, 255),
	)

	assert.True(t, handled)
	assert.False(t, teleported)
	assert.False(t, shadowlord.IsActive())
	assert.False(t, gs.CombatScene.isCombatPCPossessed(victim))
	assert.False(t, gs.CombatScene.isCombatPCSleeping(victim))
	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(0).GetStatus())
	assert.Equal(t, []CombatEventKind{CombatEventFled, CombatEventPossessionRelease}, combatEventKinds(mock.CombatEvents))
}

func TestShadowlordDeathRecoversPossessedPCWhileOtherMonsterRemains(t *testing.T) {
	gs, mock, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)
	addHostileOrcForShadowlordRecoveryTest(gs)
	victim := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	require.True(t, gs.CombatScene.acquireCombatPossession(victim, shadowlord.CombatantID()))
	shadowlord.SetCurrentHP(0)
	mock.Reset()

	gs.finalizeCombatMonsterDeathWithPolicy(
		shadowlord, shadowlordIndex, nil, NoTargetPC, false, false,
	)

	assert.False(t, gs.CombatScene.isCombatPCPossessed(victim))
	assert.False(t, gs.CombatScene.isCombatPCSleeping(victim))
	assert.Equal(t, party_state.Good, gs.PartyState.GetCharacter(0).GetStatus())
	require.Len(t, mock.CombatEvents, 2)
	assert.Equal(t, CombatEventVanished, mock.CombatEvents[0].Kind)
	assert.Equal(t, CombatEventPossessionRelease, mock.CombatEvents[1].Kind)
}

func addHostileOrcForShadowlordRecoveryTest(gs *GameState) {
	ref := &(*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	orc := NewCombatMonster(
		references.MonsterOrc, ref, references.Position{X: 8, Y: 8}, newFixedRNG(1),
	)
	gs.CombatScene.AddMonsterToQueue(orc)
}

func combatEventKinds(events []CombatEvent) []CombatEventKind {
	kinds := make([]CombatEventKind, len(events))
	for index, event := range events {
		kinds[index] = event.Kind
	}

	return kinds
}

func TestDaemonNoCorpseAndNoPoisonAbility(t *testing.T) {
	gs, _, daemon, _ := setupHostileDaemonTest(t)
	position := daemon.Position()
	terrain := gs.GetCurrentLayeredMap().GetTopTile(&position)
	assert.True(t, daemon.HasAbility(references.NoCorpse))
	assert.False(t, daemon.HasAbility(references.PoisonAtRange))
	assert.False(t, daemon.HasAbility(references.InfectWithPlague))
	assert.False(t, combatDeathObjectEligible(daemon, terrain))
}

func gDaemonNearestPC(gs *GameState, daemon *CombatMonster, daemonIndex int) nearestPCResult {
	return gs.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: daemonIndex},
		daemon.Loyalty(), daemon.Position(),
	)
}

func firstLegalDaemonGatePosition(t *testing.T, gs *GameState) references.Position {
	t.Helper()
	ref := &(*gs.GameReferences.EnemyReferences)[references.MonsterDevil]
	probe := &CombatMonster{monsterType: references.MonsterDevil, enemyRef: ref}
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			if isCombatPositionValidForMonster(gs, position, probe, -1) {
				return position
			}
		}
	}
	t.Fatal("combat map has no legal daemon gate square")

	return references.Position{}
}

func clearRangeNinePositions(t *testing.T, combatMap *references.CombatMapReference) (references.Position, references.Position) {
	t.Helper()
	for y := range references.YCombatMapTiles {
		for x := 0; x+9 < int(references.XCombatMapTiles); x++ {
			from := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			to := references.Position{X: references.Coordinate(x + 9), Y: references.Coordinate(y)}
			impact, clear := combatMissileImpactPosition(combatMap.GetTileNumber, from, to, references.MissileNone)
			if clear && impact == to {
				return from, to
			}
		}
	}
	t.Fatal("combat map has no clear range-nine missile path")

	return references.Position{}, references.Position{}
}

func obstructedRangePositions(combatMap *references.CombatMapReference) (references.Position, references.Position, bool) {
	for y1 := range references.YCombatMapTiles {
		for x1 := range references.XCombatMapTiles {
			from := references.Position{X: references.Coordinate(x1), Y: references.Coordinate(y1)}
			for y2 := range references.YCombatMapTiles {
				for x2 := range references.XCombatMapTiles {
					to := references.Position{X: references.Coordinate(x2), Y: references.Coordinate(y2)}
					distance := integerDistance(from, to)
					if distance < 2 || distance > 9 {
						continue
					}
					_, clear := combatMissileImpactPosition(combatMap.GetTileNumber, from, to, references.MissileNone)
					if !clear {
						return from, to, true
					}
				}
			}
		}
	}

	return references.Position{}, references.Position{}, false
}

func obstructedCombatPath(
	t *testing.T,
	gs *GameState,
) (*references.CombatMapReference, references.Position, references.Position) {
	t.Helper()
	for mapIndex := 0; mapIndex <= int(references.LandSeaCombatMap); mapIndex++ {
		combatMap, err := references.LoadCombatMap(mapIndex, references.BritanniaCombatTerritory, gs.GameConfig)
		require.NoError(t, err)
		from, to, found := obstructedRangePositions(combatMap)
		if found {
			return combatMap, from, to
		}
	}
	t.Fatal("Britannia combat maps have no obstructed daemon missile path")

	return nil, references.Position{}, references.Position{}
}
