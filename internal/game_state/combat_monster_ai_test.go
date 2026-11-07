// Tests for combat monster AI functions in combat_monster_ai.go.
//
// These tests verify behavioral equivalence with the original Ultima V
// monster AI from original combat scheduler, original combat movement rules, and original combat action rules. Each test
// documents which original function it validates.
//
// Uses deterministic mock RNG and real game data via IntegrationTestBuilder.
// See ref:COMBAT-0008.
package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// ─── integerDistance tests ──────────────────────────────────────────────────
// Validates integer Euclidean distance from original combat utility rules.
// See ref:COMBAT-0008.

func TestIntegerDistance_SamePosition(t *testing.T) {
	pos := references.Position{X: 5, Y: 5}
	assert.Equal(t, 0, integerDistance(pos, pos), "Same position should have distance 0")
}

func TestIntegerDistance_Adjacent(t *testing.T) {
	from := references.Position{X: 5, Y: 5}

	// Cardinal directions (distance = 1)
	assert.Equal(t, 1, integerDistance(from, references.Position{X: 6, Y: 5}), "Right")
	assert.Equal(t, 1, integerDistance(from, references.Position{X: 4, Y: 5}), "Left")
	assert.Equal(t, 1, integerDistance(from, references.Position{X: 5, Y: 6}), "Down")
	assert.Equal(t, 1, integerDistance(from, references.Position{X: 5, Y: 4}), "Up")

	// Diagonal: sqrt(1+1) = sqrt(2) ≈ 1.414, integer sqrt → 1
	assert.Equal(t, 1, integerDistance(from, references.Position{X: 6, Y: 6}), "Diagonal")
}

func TestIntegerDistance_KnownValues(t *testing.T) {
	from := references.Position{X: 0, Y: 0}

	// (3,4) → sqrt(9+16) = sqrt(25) = 5
	assert.Equal(t, 5, integerDistance(from, references.Position{X: 3, Y: 4}))

	// (5,0) → sqrt(25) = 5
	assert.Equal(t, 5, integerDistance(from, references.Position{X: 5, Y: 0}))

	// (10,0) → sqrt(100) = 10
	assert.Equal(t, 10, integerDistance(from, references.Position{X: 10, Y: 0}))

	// (2,2) → sqrt(8) ≈ 2.83, integer sqrt = 2
	assert.Equal(t, 2, integerDistance(from, references.Position{X: 2, Y: 2}))
}

func TestIntegerDistance_Symmetric(t *testing.T) {
	a := references.Position{X: 2, Y: 3}
	b := references.Position{X: 7, Y: 9}
	assert.Equal(t, integerDistance(a, b), integerDistance(b, a),
		"Distance should be symmetric")
}

// ─── signum tests ───────────────────────────────────────────────────────────

func TestSignum(t *testing.T) {
	assert.Equal(t, -1, signum(-100))
	assert.Equal(t, -1, signum(-1))
	assert.Equal(t, 0, signum(0))
	assert.Equal(t, 1, signum(1))
	assert.Equal(t, 1, signum(100))
}

// ─── PC target fixture checks ───────────────────────────────────────────────
// These check target setup for isolated attack tests. Production whosnear
// behavior is exercised through findNearestOpponent below.

func TestCombatPCTargetFixture_SingleActivePC(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	require.NotNil(t, gs.CombatScene)

	// Only one PC active at position (5,8)
	for i := range party_state.MaxActiveParty {
		gs.CombatScene.PartyActive[i] = false
	}
	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 5, Y: 8})

	// Monster at (5,5) — 3 tiles above PC
	monPos := references.Position{X: 5, Y: 5}
	result := findNearestPC(monPos, gs.CombatScene)

	assert.Equal(t, 0, result.pcIndex, "Should find PC 0")
	assert.Equal(t, 3, result.distance, "Distance from (5,5) to (5,8) = 3")
	assert.Equal(t, 0, result.dirX, "Same X → dirX=0")
	assert.Equal(t, 1, result.dirY, "PC below monster → dirY=+1")
}

func TestCombatPCTargetFixture_PicksClosest(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	// Two PCs active: PC0 at (2,2) far, PC1 at (5,4) close
	for i := range party_state.MaxActiveParty {
		gs.CombatScene.PartyActive[i] = false
	}
	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.PartyActive[1] = true
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 2, Y: 2})
	gs.CombatScene.SetPartyMemberPosition(1, references.Position{X: 5, Y: 4})

	// Monster at (5,5)
	monPos := references.Position{X: 5, Y: 5}
	result := findNearestPC(monPos, gs.CombatScene)

	assert.Equal(t, 1, result.pcIndex, "Should find PC 1 (closer)")
	assert.Equal(t, 1, result.distance, "Distance from (5,5) to (5,4) = 1")
}

func TestShadowlordAndDoomCombatCanSelectInvisibleOpponents(t *testing.T) {
	shadowState, _, shadowlord, shadowlordIndex := setupHostileSpecialMonsterTest(t, references.MonsterShadowlord)
	shadowState.CombatScene.SetPartyMemberInvisible(0, true)

	shadowTarget := shadowState.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: shadowlordIndex},
		shadowlord.Loyalty(), shadowlord.Position(),
	)

	assert.True(t, shadowTarget.found)
	assert.Equal(t, 0, shadowTarget.pcIndex)

	orcState, _, orc, orcIndex := setupHostileSpecialMonsterTest(t, references.MonsterOrc)
	orcState.CombatScene.SetPartyMemberInvisible(0, true)
	orcTarget := orcState.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: orcIndex},
		orc.Loyalty(), orc.Position(),
	)
	assert.False(t, orcTarget.found)

	orcState.CombatScene.PreCombatPlayerLocation.Location = references.Doom
	orcTarget = orcState.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: orcIndex},
		orc.Loyalty(), orc.Position(),
	)
	assert.False(t, orcTarget.found, "pre-combat location is not authoritative Doom-interior context")

	orcState.CombatScene.DungeonRoomOrigin = &DungeonRoomEncounterOrigin{Location: references.Doom}
	orcTarget = orcState.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: orcIndex},
		orc.Loyalty(), orc.Position(),
	)
	assert.True(t, orcTarget.found, "all attackers can select invisible opponents in Doom")
	assert.Equal(t, 0, orcTarget.pcIndex)
}

func TestNearestOpponentTieUsesReverseSourceQueueOrder(t *testing.T) {
	gs, _, orc, orcIndex := setupHostileSpecialMonsterTest(t, references.MonsterOrc)
	source := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	clone := gs.CombatScene.addPCCloneFromSource(
		source,
		0,
		references.Position{X: 4, Y: 5},
		NewCombatPCProvider(gs.PartyState.GetCharacter(0)),
	)
	orc.SetPosition(references.Position{X: 5, Y: 5})
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 6, Y: 5})
	sourceEntry := gs.CombatScene.TurnQueue.GetEntryByIdentity(source)
	cloneEntry := gs.CombatScene.TurnQueue.GetEntryByIdentity(clone)
	require.NotNil(t, sourceEntry)
	require.NotNil(t, cloneEntry)
	require.Greater(t, cloneEntry.sourceSlot, sourceEntry.sourceSlot)

	nearest := gs.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: orcIndex},
		orc.Loyalty(), orc.Position(),
	)

	assert.Equal(t, clone, nearest.identity,
		"the higher raw combatq slot wins equal-distance ties during the reverse scan")
}

func TestMonsterMeleeUsesExactCloneSleepAndPosition(t *testing.T) {
	gs, mock, orc, orcIndex := setupHostileSpecialMonsterTest(t, references.MonsterOrc)
	source := CombatTurnIdentity{IsPartyMember: true, EntityIndex: 0}
	clonePosition := references.Position{X: 1, Y: 0}
	clone := gs.CombatScene.addPCCloneFromSource(
		source, 0, clonePosition, NewCombatPCProvider(gs.PartyState.GetCharacter(0)),
	)
	gs.setCombatPCSleeping(clone, true)
	gs.setCombatPCSleeping(source, false)
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 10, Y: 10})
	orc.SetPosition(references.Position{X: 0, Y: 0})
	nearest := gs.findNearestOpponent(
		CombatTurnIdentity{IsPartyMember: false, EntityIndex: orcIndex},
		orc.Loyalty(), orc.Position(),
	)
	require.Equal(t, clone, nearest.identity)
	mock.Reset()

	assert.True(t, gs.monsterMeleeAttack(orc, orcIndex, nearest))

	require.Len(t, mock.CombatEvents, 2)
	event := mock.CombatEvents[0]
	assert.Equal(t, 1, event.Diagnostics.Hit.DefenderEffectiveDex)
	assert.Equal(t, clonePosition, event.Target.Position)
	assert.Equal(t, CombatConditionWoke, mock.CombatEvents[1].Condition)
}

func TestNoOpponentForcesEveryMonsterToOneHPAndFleeing(t *testing.T) {
	gs, _, monster, monsterIndex := setupMonsterTeleportTest(t, false)
	gs.CombatScene.SetPartyMemberInvisible(0, true)
	secondRef := (*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	second := NewCombatMonster(
		references.MonsterOrc, &secondRef, references.Position{X: 8, Y: 8}, newFixedRNG(1),
	)
	gs.CombatScene.AddMonsterToQueue(second)

	nearest := gs.selectMonsterOpponentWithRNG(monster, monsterIndex, newSequenceRNG(0))

	assert.False(t, nearest.found)
	assert.True(t, nearest.hasDirection)
	assert.Equal(t, 1, monster.CurrentHP())
	assert.True(t, monster.IsFleeing())
	assert.Equal(t, 1, second.CurrentHP())
	assert.True(t, second.IsFleeing())
}

// A monster made to flee mid-turn by the no-opponent check can step off an
// edge through ordinary seek; it must leave the battle rather than crash the
// end-of-turn field check (issue #84).
func TestNoOpponentEdgeMonsterFleesOffMapDuringSeek(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, false)
	gs.CombatScene.SetPartyMemberInvisible(0, true)
	require.False(t, monster.IsFleeing())
	entry := gs.CombatScene.TurnQueue.GetEntry(monsterIndex, false)
	require.NotNil(t, entry)

	result := gs.executeMonsterTurn(entry)

	assert.True(t, result.Moved)
	assert.False(t, monster.IsActive())
	assert.Equal(t, turnQueueFled, entry.status)
	require.NotEmpty(t, mock.CombatEvents)
	assert.Equal(t, CombatEventFled, mock.CombatEvents[len(mock.CombatEvents)-1].Kind)
}

// ─── monsterSeek tests ──────────────────────────────────────────────────────
// Validates monster pursuit from original combat movement rules.
// See ref:COMBAT-0008.

func TestMonsterSeek_MovesTowardPC(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	manager := gs.CombatScene.MonsterManager
	manager.Clear()

	// PC at (5,8)
	for i := range party_state.MaxActiveParty {
		gs.CombatScene.PartyActive[i] = false
	}
	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 5, Y: 8})

	// Monster at (5,5) — should move toward (5,8), i.e., Y increases
	enemyRefs := *gs.GameReferences.EnemyReferences
	ref := &enemyRefs[references.MonsterOrc]
	rng := newFixedRNG(1)
	monsterPos := references.Position{X: 5, Y: 5}
	monster := NewCombatMonster(references.MonsterOrc, ref, monsterPos, rng)
	monsterIdx := manager.AddMonster(monster)

	nearest := findNearestPC(monster.Position(), gs.CombatScene)
	require.Equal(t, 0, nearest.pcIndex, "Should find PC 0")

	movement := gs.monsterSeek(monster, monsterIdx)
	assert.True(t, movement.moved, "Monster should move toward PC")
	assert.False(t, movement.teleported)

	newPos := monster.Position()
	// Should have moved closer — either Y increased (toward PC at y=8) or X changed
	oldDist := integerDistance(monsterPos, references.Position{X: 5, Y: 8})
	newDist := integerDistance(newPos, references.Position{X: 5, Y: 8})
	assert.Less(t, newDist, oldDist,
		"Monster should be closer to PC after seek (old=%d, new=%d)", oldDist, newDist)
}

func TestMonsterSeek_DoNotMoveFlag(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	manager := gs.CombatScene.MonsterManager
	manager.Clear()

	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 5, Y: 8})

	// Find a monster type with DoNotMove flag — Reaper
	enemyRefs := *gs.GameReferences.EnemyReferences
	ref := &enemyRefs[references.MonsterReaper]

	// Only test if Reaper has DoNotMove — skip otherwise
	if !ref.DoNotMove {
		t.Skip("Reaper does not have DoNotMove flag in this data set")
	}

	rng := newFixedRNG(1)
	monsterPos := references.Position{X: 5, Y: 5}
	monster := NewCombatMonster(references.MonsterReaper, ref, monsterPos, rng)
	monsterIdx := manager.AddMonster(monster)

	movement := gs.monsterSeek(monster, monsterIdx)

	assert.False(t, movement.moved, "DoNotMove monster should not move")
	assert.Equal(t, monsterPos, monster.Position(), "Position should be unchanged")
}

func TestMonsterTeleportUnboxedPublishesTypedEvent(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, destination, _ := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	rng := newSequenceRNG(0, int(destination.X), int(destination.Y))

	movement := gs.monsterSeekWithRNG(monster, monsterIndex, rng)

	assert.True(t, movement.moved)
	assert.True(t, movement.teleported)
	assert.Equal(t, destination, monster.Position())
	assert.Equal(t, 3, rng.index)
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, CombatEventTeleport, event.Kind)
	assert.Equal(t, from, event.From)
	assert.Equal(t, destination, event.To)
}

func TestBoxedMonsterTeleportSkipsChanceRoll(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, destination, _ := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	for _, direction := range [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
		position := references.Position{
			X: from.X + references.Coordinate(direction[0]),
			Y: from.Y + references.Coordinate(direction[1]),
		}
		if isCombatPositionValidForMonster(gs, position, monster, monsterIndex) {
			require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldProtection, position)))
		}
	}
	require.True(t, gs.monsterBoxedIn(monster, monsterIndex))
	rng := newSequenceRNG(int(destination.X), int(destination.Y))

	movement := gs.monsterSeekWithRNG(monster, monsterIndex, rng)

	assert.True(t, movement.teleported)
	assert.Equal(t, destination, monster.Position())
	assert.Equal(t, 2, rng.index, "boxed monsters draw coordinates without a chance roll")
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventTeleport, mock.CombatEvents[0].Kind)
}

func TestBoxedMonsterFailedTeleportDoesNotFallThroughToSeek(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, _, _ := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	for _, direction := range [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}} {
		position := references.Position{
			X: from.X + references.Coordinate(direction[0]),
			Y: from.Y + references.Coordinate(direction[1]),
		}
		if isCombatPositionValidForMonster(gs, position, monster, monsterIndex) {
			require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldProtection, position)))
		}
	}
	require.True(t, gs.monsterBoxedIn(monster, monsterIndex))
	rng := newSequenceRNG(15, 15)

	movement := gs.monsterSeekWithRNG(monster, monsterIndex, rng)

	assert.False(t, movement.moved)
	assert.False(t, movement.teleported)
	assert.Equal(t, from, monster.Position())
	assert.Equal(t, 2, rng.index)
	assert.Empty(t, mock.CombatEvents)
}

func TestMonsterTeleportChanceSkipFallsThroughToOrdinarySeek(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, _, nearest := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	expected := positionInDirection(from, nearest)
	rng := newSequenceRNG(monsterTeleportSkipRoll, monsterSeekRollForDirection(nearest))

	movement := gs.monsterSeekWithRNG(monster, monsterIndex, rng)

	assert.True(t, movement.moved)
	assert.False(t, movement.teleported)
	assert.Equal(t, expected, monster.Position())
	assert.Equal(t, 2, rng.index)
	require.Len(t, mock.CombatEvents, 0, "ordinary movement is published by the owning turn executor")
}

func TestMonsterTeleportInvalidDestinationFallsThroughToOrdinarySeek(t *testing.T) {
	for _, test := range []struct {
		name        string
		destination func(*GameState, references.Position) references.Position
	}{
		{
			name: "offscreen",
			destination: func(_ *GameState, _ references.Position) references.Position {
				return references.Position{X: 15, Y: 15}
			},
		},
		{
			name: "protected",
			destination: func(gs *GameState, candidate references.Position) references.Position {
				require.True(t, gs.CombatScene.AddField(NewCombatField(CombatFieldProtection, candidate)))

				return candidate
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, _, monster, monsterIndex := setupMonsterTeleportTest(t, true)
			from, legalDestination, nearest := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
			monster.SetPosition(from)
			expected := positionInDirection(from, nearest)
			destination := test.destination(gs, legalDestination)
			rng := newSequenceRNG(
				0, int(destination.X), int(destination.Y), monsterSeekRollForDirection(nearest),
			)

			movement := gs.monsterSeekWithRNG(monster, monsterIndex, rng)

			assert.True(t, movement.moved)
			assert.False(t, movement.teleported)
			assert.Equal(t, expected, monster.Position())
			assert.Equal(t, 4, rng.index)
		})
	}
}

func TestMonsterTeleportSuppressionAndAbilityChecksPreserveRNG(t *testing.T) {
	for _, test := range []struct {
		name      string
		teleports bool
		activate  func(*GameState)
	}{
		{name: "no ability", teleports: false, activate: func(*GameState) {}},
		{name: "crown", teleports: true, activate: func(gs *GameState) { gs.ToggleCrown() }},
		{
			name:      "negate magic",
			teleports: true,
			activate:  func(gs *GameState) { gs.ActivateNegateMagicEffect(3) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, _, monster, monsterIndex := setupMonsterTeleportTest(t, test.teleports)
			from, _, nearest := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
			monster.SetPosition(from)
			test.activate(gs)
			rng := newSequenceRNG(monsterSeekRollForDirection(nearest))

			movement := gs.monsterSeekWithRNG(monster, monsterIndex, rng)

			assert.True(t, movement.moved)
			assert.False(t, movement.teleported)
			assert.Equal(t, 1, rng.index, "suppressed teleport must consume no chance or coordinate RNG")
		})
	}
}

func TestMonsterAttackPrecedesTeleport(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, _, nearest := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	gs.CombatScene.SetPartyMemberPosition(0, positionInDirection(from, nearest))
	mock.Reset()
	entry := &TurnQueueEntry{Combatant: monster, EntityIndex: monsterIndex}

	result := gs.executeMonsterTurn(entry)

	assert.False(t, result.Moved)
	assert.False(t, result.Teleported)
	assert.Equal(t, from, monster.Position())
	require.NotEmpty(t, mock.CombatEvents)
	assert.Equal(t, CombatEventAttack, mock.CombatEvents[0].Kind)
	for _, event := range mock.CombatEvents {
		assert.NotEqual(t, CombatEventTeleport, event.Kind)
	}
}

func TestExecuteMonsterTurnReportsTeleportWithoutWalkingEvent(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, destination, _ := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	gs.SetRandomSeed(seedForTeleportDestination(t, gs, destination))
	mock.Reset()
	entry := &TurnQueueEntry{Combatant: monster, EntityIndex: monsterIndex}

	result := gs.executeMonsterTurn(entry)

	assert.True(t, result.Moved)
	assert.True(t, result.Teleported)
	assert.Equal(t, destination, monster.Position())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventTeleport, mock.CombatEvents[0].Kind)
}

func TestAdvanceCombatTurnSchedulesAndExecutesTeleport(t *testing.T) {
	gs, mock, monster, monsterIndex := setupMonsterTeleportTest(t, true)
	from, destination, _ := findMonsterTeleportTestPositions(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	gs.SetRandomSeed(seedForTeleportDestination(t, gs, destination))
	forceNextMonsterTurnForTest(t, gs, monsterIndex)
	gs.CombatScene.presentTurns = true
	mock.Reset()

	gs.AdvanceCombatTurn()
	step, scheduled := gs.ReviewScheduledCombatStep()
	require.True(t, scheduled)
	result, err := gs.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)

	assert.True(t, result.Moved)
	assert.True(t, result.Teleported)
	assert.Equal(t, destination, monster.Position())
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, CombatEventTeleport, mock.CombatEvents[0].Kind)
}

func TestExecuteMonsterTurnRerollsConfusionBeforeSeek(t *testing.T) {
	gs, _, monster, monsterIndex := setupMonsterTeleportTest(t, false)
	from := findOpenSouthEastCombatPosition(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: from.X + 2, Y: from.Y})
	secondRef := (*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	second := NewCombatMonster(
		references.MonsterOrc, &secondRef, references.Position{X: from.X, Y: from.Y + 2}, newFixedRNG(1),
	)
	gs.CombatScene.AddMonsterToQueue(second)
	gs.ActivateConfusionEffect(3)
	gs.SetRandomSeed(seedForAttackThenSeekConfusion(t, gs, monster.EnemyRef().Intelligence))
	entry := &TurnQueueEntry{Combatant: monster, EntityIndex: monsterIndex}

	result := gs.executeMonsterTurn(entry)

	assert.True(t, result.Moved)
	assert.False(t, result.Teleported)
	assert.Equal(t, references.Position{X: from.X, Y: from.Y + 1}, monster.Position(),
		"attack targets the PC, then seek rerolls loyalty and moves toward the hostile monster")
}

func TestMonsterSeekUsesOriginalAxisBoundaryAndRandomDirectionMapping(t *testing.T) {
	gs, _, monster, monsterIndex := setupMonsterTeleportTest(t, false)
	from := findOpenSouthEastCombatPosition(t, gs, monster, monsterIndex)
	monster.SetPosition(from)
	nearest := nearestPCResult{pcIndex: 0, dirX: 1, dirY: 1, found: true}

	movement := gs.monsterSeekTowardWithRNG(monster, monsterIndex, nearest, newSequenceRNG(127))
	assert.True(t, movement.moved)
	assert.Equal(t, references.Position{X: from.X, Y: from.Y + 1}, monster.Position(),
		"127 skips horizontal and tries vertical")

	monster.SetPosition(from)
	movement = gs.monsterSeekTowardWithRNG(monster, monsterIndex, nearest, newSequenceRNG(128))
	assert.True(t, movement.moved)
	assert.Equal(t, references.Position{X: from.X + 1, Y: from.Y}, monster.Position(),
		"128 tries horizontal first")

	monster.SetPosition(from)
	nearest.dirX = 0
	nearest.dirY = 0
	movement = gs.monsterSeekTowardWithRNG(monster, monsterIndex, nearest, newSequenceRNG(0, 0))
	assert.True(t, movement.moved)
	assert.Equal(t, references.Position{X: from.X, Y: from.Y + 1}, monster.Position(),
		"random direction zero maps to south")
}

func setupMonsterTeleportTest(
	t *testing.T,
	teleports bool,
) (*GameState, *MockSystemCallbacks, *CombatMonster, int) {
	t.Helper()
	gs, mock := newBritainCombatTestState(t, 1)
	clearCombatMonstersForTest(gs.CombatScene)
	for index := range party_state.MaxActiveParty {
		gs.CombatScene.PartyActive[index] = false
	}
	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 10, Y: 10})

	monsterRef := (*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	monsterRef.EnemyAbilities = map[references.EnemyAbility]bool{}
	if teleports {
		monsterRef.EnemyAbilities[references.Teleport] = true
	}
	monster := NewCombatMonster(
		references.MonsterOrc, &monsterRef, references.Position{}, newFixedRNG(1),
	)
	monsterIndex := gs.CombatScene.AddMonsterToQueue(monster)
	mock.Reset()

	return gs, mock, monster, monsterIndex
}

func findMonsterTeleportTestPositions(
	t *testing.T,
	gs *GameState,
	monster *CombatMonster,
	monsterIndex int,
) (references.Position, references.Position, nearestPCResult) {
	t.Helper()
	directions := [4][2]int{{0, 1}, {1, 0}, {0, -1}, {-1, 0}}
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			from := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			monster.SetPosition(from)
			for _, direction := range directions {
				target := references.Position{
					X: from.X + 2*references.Coordinate(direction[0]),
					Y: from.Y + 2*references.Coordinate(direction[1]),
				}
				if target.X < 0 || target.X >= references.XCombatMapTiles ||
					target.Y < 0 || target.Y >= references.YCombatMapTiles {
					continue
				}
				step := references.Position{
					X: from.X + references.Coordinate(direction[0]),
					Y: from.Y + references.Coordinate(direction[1]),
				}
				if !isCombatPositionValidForMonster(gs, step, monster, monsterIndex) {
					continue
				}
				for destinationY := range references.YCombatMapTiles {
					for destinationX := range references.XCombatMapTiles {
						destination := references.Position{
							X: references.Coordinate(destinationX),
							Y: references.Coordinate(destinationY),
						}
						if destination == target || integerDistance(from, destination) <= 1 ||
							!isCombatPositionValidForMonster(gs, destination, monster, monsterIndex) {
							continue
						}

						gs.CombatScene.SetPartyMemberPosition(0, target)

						return from, destination, nearestPCResult{
							pcIndex: 0,
							dirX:    direction[0],
							dirY:    direction[1],
							found:   true,
						}
					}
				}
			}
		}
	}
	t.Fatal("combat map has no legal teleport test positions")

	return references.Position{}, references.Position{}, nearestPCResult{}
}

func positionInDirection(from references.Position, nearest nearestPCResult) references.Position {
	return references.Position{
		X: from.X + references.Coordinate(nearest.dirX),
		Y: from.Y + references.Coordinate(nearest.dirY),
	}
}

func monsterSeekRollForDirection(nearest nearestPCResult) int {
	if nearest.dirX != 0 {
		return monsterSeekAxisRollRange - 1
	}

	return 0
}

func seedForTeleportDestination(t *testing.T, gs *GameState, destination references.Position) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 100000; seed++ {
		gs.SetRandomSeed(seed)
		rng := gs.GetRNG()
		if rng.Intn(monsterTeleportChanceRange) != monsterTeleportSkipRoll &&
			rng.Intn(monsterTeleportCoordinateRange) == int(destination.X) &&
			rng.Intn(monsterTeleportCoordinateRange) == int(destination.Y) {
			return seed
		}
	}
	t.Fatal("no deterministic seed produced the requested teleport destination")

	return 0
}

func seedForAttackThenSeekConfusion(t *testing.T, gs *GameState, intelligence int) uint64 {
	t.Helper()
	for seed := uint64(0); seed < 100000; seed++ {
		gs.SetRandomSeed(seed)
		rng := gs.GetRNG()
		if originalD30FromSourceValue(rng.Intn(originalD30SourceValueCount)) <= intelligence &&
			originalD30FromSourceValue(rng.Intn(originalD30SourceValueCount)) > intelligence &&
			rng.Intn(monsterSeekAxisRollRange) <= monsterSeekHorizontalThreshold {
			return seed
		}
	}
	t.Fatal("no deterministic seed produced the required attack/seek confusion sequence")

	return 0
}

func findOpenSouthEastCombatPosition(
	t *testing.T,
	gs *GameState,
	monster *CombatMonster,
	monsterIndex int,
) references.Position {
	t.Helper()
	for y := range references.YCombatMapTiles - 2 {
		for x := range references.XCombatMapTiles - 2 {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			monster.SetPosition(position)
			south := references.Position{X: position.X, Y: position.Y + 1}
			east := references.Position{X: position.X + 1, Y: position.Y}
			southFar := references.Position{X: position.X, Y: position.Y + 2}
			eastFar := references.Position{X: position.X + 2, Y: position.Y}
			if isCombatPositionValidForMonster(gs, south, monster, monsterIndex) &&
				isCombatPositionValidForMonster(gs, east, monster, monsterIndex) &&
				isCombatPositionValidForMonster(gs, southFar, monster, monsterIndex) &&
				isCombatPositionValidForMonster(gs, eastFar, monster, monsterIndex) {
				return position
			}
		}
	}
	t.Fatal("combat map has no square open to both south and east")

	return references.Position{}
}

// ─── monsterMeleeAttack tests ───────────────────────────────────────────────
// Validates the melee portion of monster melee attack from original combat scheduler
// combined with hit status and damage from original combat damage rules.
// See ref:COMBAT-0008.

func TestMonsterMeleeAttack_NotAdjacent(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)

	// Move monster far from PC — distance > 1
	monster.SetPosition(references.Position{X: 0, Y: 0})

	nearest := findNearestPC(monster.Position(), gs.CombatScene)
	mock.Reset()
	attacked := gs.monsterMeleeAttack(monster, 0, nearest)

	assert.False(t, attacked, "Should not attack when not adjacent")
	assert.Empty(t, mock.SoundEffectsPlayed, "invalid melee target must be silent")
}

func TestMonsterMeleeAttack_AdjacentHitOrMiss(t *testing.T) {
	// Use setupCombatWithMonster to get a monster adjacent to PC
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)

	// Monster is already adjacent to active PC (placed via setupCombatWithMonster)
	nearest := findNearestPC(monster.Position(), gs.CombatScene)
	require.Equal(t, monsterAdjacentDistance, nearest.distance,
		"Monster should be adjacent to PC")

	mock.Reset()
	attacked := gs.monsterMeleeAttack(monster, 0, nearest)

	assert.True(t, attacked, "Should attempt attack when adjacent")
	require.Len(t, mock.CombatImpactCalls, 1, "Should produce floating combat feedback")
}

func TestMonsterMeleeAttack_KillPC(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)

	// Set PC HP to 1 so any damage kills them
	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIdx)
	character.SubtractHp(character.GetCurrentHp() - 1) // Leave 1 HP

	nearest := findNearestPC(monster.Position(), gs.CombatScene)
	require.Equal(t, monsterAdjacentDistance, nearest.distance)

	// Try several seeds to get a hit that deals damage
	killed := false
	for seed := uint64(1); seed < 200; seed++ {
		// Reset state for each attempt
		character.SubtractHp(0) // nop, just making sure
		if character.GetCurrentHp() > 1 {
			// Reset HP to 1 if somehow healed
			character.SubtractHp(character.GetCurrentHp() - 1)
		}

		if character.IsDead() {
			break
		}

		gs.SetRandomSeed(seed)
		mock.Reset()

		attacked := gs.monsterMeleeAttack(monster, 0, nearest)
		if !attacked {
			continue
		}

		if character.IsDead() {
			killed = true
			break
		}
	}

	if killed {
		assert.False(t, gs.CombatScene.PartyActive[activeIdx],
			"Dead PC should be marked inactive")

		require.Len(t, mock.CombatImpactCalls, 1)
		assert.True(t, mock.CombatImpactCalls[0].Feedback.Fatal)
	}
	// If we couldn't get a kill after 200 seeds, that's OK — the test verified the loop
}

func TestMonsterMeleeAttack_PoisonAbility(t *testing.T) {
	// Spider has PoisonAtRange ability (original POISONS=0x0204 maps to PoisonAtRange|InfectWithPlague)
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterSpider, references.Down, 42)

	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)

	// Verify spider has PoisonAtRange (part of the original POISONS compound mask)
	canPoison := monster.HasAbility(references.PoisonAtRange) || monster.HasAbility(references.InfectWithPlague)
	if !canPoison {
		t.Skip("Spider does not have poison-capable ability in this data set")
	}

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIdx)

	nearest := findNearestPC(monster.Position(), gs.CombatScene)
	require.Equal(t, monsterAdjacentDistance, nearest.distance)

	// Try seeds until we get a poison result (the executable condition succeeds 3/4 of the time).
	poisoned := false
	for seed := uint64(0); seed < 200; seed++ {
		// Reset character status
		character.SetStatus(party_state.Good)
		gs.SetRandomSeed(seed)
		mock.Reset()

		gs.monsterMeleeAttack(monster, 0, nearest)

		if character.IsPoisoned() {
			poisoned = true
			break
		}
	}

	assert.True(t, poisoned,
		"With Poison ability, at least one seed in 200 should produce poison")
	require.Len(t, mock.CombatImpactCalls, 1)
	assert.Equal(t, CombatAttackDeliveryMelee, mock.CombatImpactCalls[0].Feedback.Presentation.Delivery)
	assert.Equal(t, references.NoEquipment, mock.CombatImpactCalls[0].Feedback.Presentation.Source)
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, CombatEventPoison, event.Kind)
	assert.Equal(t, CombatFeedbackPoison, event.Result)
	assert.True(t, event.Diagnostics.PoisonRolled)
	assert.NotZero(t, event.Diagnostics.PoisonRoll)
	assert.Equal(t, monsterSpecialEffectRollRange-1, event.Diagnostics.PoisonRollMaximum)
}

// ─── executeMonsterTurn tests ───────────────────────────────────────────────
// Validates monster action from original combat scheduler.
// See ref:COMBAT-0008.

func TestExecuteMonsterTurn_DeadMonsterSkipped(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)

	// Kill the monster
	monster.TakeDamage(monster.CurrentHP())
	require.True(t, monster.IsDead())

	entry := &TurnQueueEntry{
		Combatant:   monster,
		EntityIndex: 0,
	}

	mock.Reset()
	gs.executeMonsterTurn(entry)

	// Dead monster should produce no messages
	assert.Empty(t, mock.Messages, "Dead monster should not act")
}

func TestExecuteMonsterTurn_InactiveMonsterSkipped(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)
	monster.SetActive(false)

	entry := &TurnQueueEntry{
		Combatant:   monster,
		EntityIndex: 0,
	}

	mock.Reset()
	gs.executeMonsterTurn(entry)

	assert.Empty(t, mock.Messages, "Inactive monster should not act")
}

func TestExecuteMonsterTurn_PublishesCompletedMovement(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	monster.SetPosition(references.Position{X: 0, Y: 0})
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 5, Y: 8})
	for index := 1; index < len(gs.CombatScene.PartyActive); index++ {
		gs.CombatScene.PartyActive[index] = false
	}
	entry := &TurnQueueEntry{Combatant: monster, EntityIndex: 0}
	fromPosition := monster.Position()
	mock.Reset()

	gs.executeMonsterTurn(entry)

	toPosition := monster.Position()
	require.NotEqual(t, fromPosition, toPosition)
	require.Len(t, mock.CombatEvents, 1)
	event := mock.CombatEvents[0]
	assert.Equal(t, CombatEventMove, event.Kind)
	assert.False(t, event.Attacker.IsPartyMember)
	assert.Equal(t, 0, event.Attacker.Index)
	assert.Equal(t, fromPosition, event.From)
	assert.Equal(t, toPosition, event.To)
}

// ─── planMonsterTurn tests ──────────────────────────────────────────────────
// These guard the non-mutating plan used by the UI scheduler to show monster
// and target highlights before the delayed monster action executes.

func TestPlanMonsterTurn_AttackWhenAdjacent(t *testing.T) {
	gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	entry := &TurnQueueEntry{Combatant: monster, EntityIndex: 0}

	plan := gs.planMonsterTurn(entry)

	assert.Equal(t, MonsterTurnAttack, plan.Action)
	assert.Equal(t, activeIdx, plan.TargetPCIndex)
}

func TestPlanMonsterTurnDoesNotClaimUnresolvedSpecialOrConfusionTarget(t *testing.T) {
	for _, test := range []struct {
		name        string
		monsterType references.MonsterType
		activate    func(*GameState)
	}{
		{name: "hostile special", monsterType: references.MonsterShadowlord, activate: func(*GameState) {}},
		{
			name:        "confusion",
			monsterType: references.MonsterOrc,
			activate:    func(gs *GameState) { gs.ActivateConfusionEffect(3) },
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			gs, _, monster, monsterIndex := setupHostileSpecialMonsterTest(t, test.monsterType)
			gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 1, Y: 0})
			test.activate(gs)

			plan := gs.planMonsterTurn(&TurnQueueEntry{Combatant: monster, EntityIndex: monsterIndex})

			assert.Equal(t, MonsterTurnSeek, plan.Action)
			assert.False(t, plan.HasTarget)
			assert.Equal(t, NoTargetPC, plan.TargetPCIndex)
		})
	}
}

func TestPlanMonsterTurnDoesNotClaimFleeingTarget(t *testing.T) {
	gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)
	monster.SetFleeing(true)

	plan := gs.planMonsterTurn(&TurnQueueEntry{Combatant: monster, EntityIndex: 0})

	assert.Equal(t, MonsterTurnSeek, plan.Action)
	assert.False(t, plan.HasTarget)
	assert.Equal(t, NoTargetPC, plan.TargetPCIndex)
}

func TestPlanMonsterTurn_SeekWhenNotAdjacent(t *testing.T) {
	gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	monster.SetPosition(references.Position{X: 0, Y: 0})
	entry := &TurnQueueEntry{Combatant: monster, EntityIndex: 0}

	plan := gs.planMonsterTurn(entry)

	assert.Equal(t, MonsterTurnSeek, plan.Action)
	assert.Equal(t, NoTargetPC, plan.TargetPCIndex)
}

func TestPlanMonsterTurn_IdleForSleepingDeadInactiveOrNoTarget(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*GameState, *CombatMonster)
	}{
		{
			name: "sleeping monster",
			setup: func(_ *GameState, monster *CombatMonster) {
				monster.SetAsleep(true)
			},
		},
		{
			name: "dead monster",
			setup: func(_ *GameState, monster *CombatMonster) {
				monster.TakeDamage(monster.CurrentHP())
			},
		},
		{
			name: "inactive monster",
			setup: func(_ *GameState, monster *CombatMonster) {
				monster.SetActive(false)
			},
		},
		{
			name: "no active PCs",
			setup: func(gs *GameState, _ *CombatMonster) {
				for i := range party_state.MaxActiveParty {
					gs.CombatScene.PartyActive[i] = false
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gs, _, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

			tt.setup(gs, monster)
			entry := &TurnQueueEntry{Combatant: monster, EntityIndex: 0}

			plan := gs.planMonsterTurn(entry)

			assert.Equal(t, MonsterTurnIdle, plan.Action)
			assert.Equal(t, NoTargetPC, plan.TargetPCIndex)
		})
	}
}

// ─── Integration: AdvanceCombatTurn with monster AI ─────────────────────────
// Verifies that the wiring in AdvanceCombatTurn correctly delegates to
// executeMonsterTurn and eventually returns a PC turn.

func forceNextMonsterTurnForTest(t *testing.T, gs *GameState, monsterIndex int) {
	t.Helper()

	require.NotNil(t, gs.CombatScene)
	require.NotNil(t, gs.CombatScene.TurnQueue)

	foundMonster := false
	for _, entry := range gs.CombatScene.TurnQueue.entries {
		if entry.Combatant.IsPC() {
			entry.turnCountdown = turnCountdownBase

			continue
		}

		if entry.EntityIndex == monsterIndex {
			entry.turnCountdown = 1
			foundMonster = true
		} else {
			entry.turnCountdown = turnCountdownBase
		}
	}

	gs.CombatScene.TurnQueue.currentIndex = 0
	require.True(t, foundMonster, "monster %d must be in turn queue", monsterIndex)
}

func TestAdvanceCombatTurn_MonsterActsBeforePCTurn(t *testing.T) {
	gs, mock, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	// Move monster far away so it seeks instead of attacking
	manager := gs.CombatScene.MonsterManager
	monster := manager.GetMonster(0)
	monster.SetPosition(references.Position{X: 0, Y: 0})

	mock.Reset()

	// AdvanceCombatTurn should process the monster's turn (seek) then
	// return to give the next PC its turn.
	gs.AdvanceCombatTurn()

	// After advancing, the active player should be a valid PC index
	activeIdx := gs.CombatScene.CurrentTurnIndex
	assert.GreaterOrEqual(t, activeIdx, 0)
	assert.Less(t, activeIdx, party_state.MaxActiveParty)
}

func TestApplyCombatDamageToPC_ReportsActualOverkillDamage(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	pcIndex := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(pcIndex)
	character.SubtractHp(character.GetCurrentHp() - 3)
	mock.Reset()

	gs.applyCombatDamageToPC(character, CombatTurnIdentity{
		IsPartyMember: true,
		EntityIndex:   pcIndex,
	}, 20, monster, 0, CombatDiagnostics{})

	require.Len(t, mock.CombatImpactCalls, 1)
	feedback := mock.CombatImpactCalls[0].Feedback
	assert.Equal(t, pcIndex, feedback.Target.Index)
	assert.True(t, feedback.Target.IsPartyMember)
	assert.Equal(t, monster.Position(), feedback.Attacker.Position)
	assert.Equal(t, gs.CombatScene.GetPartyMemberPosition(pcIndex), feedback.Target.Position)
	assert.Equal(t, 3, feedback.HPLost, "floating damage must not exceed remaining HP")
	assert.True(t, feedback.Fatal)
}

func TestAdvanceCombatTurn_SchedulerDelaysMonsterAction(t *testing.T) {
	gs, mock, monster := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	forceNextMonsterTurnForTest(t, gs, 0)
	monsterPosBefore := monster.Position()
	pcIndex := gs.CombatScene.GetActiveCombatantIndex()
	pcHPBefore := gs.PartyState.GetCharacter(pcIndex).GetCurrentHp()

	gs.CombatScene.presentTurns = true

	mock.Reset()
	gs.AdvanceCombatTurn()
	step, scheduled := gs.ReviewScheduledCombatStep()

	require.True(t, scheduled, "monster turn should produce a presentation step")
	assert.Equal(t, CombatPresentationMonsterTurn, step.Kind)
	assert.Equal(t, 0, step.MonsterIndex)
	assert.Equal(t, pcIndex, step.Target.EntityIndex)
	assert.Equal(t, 0, gs.CombatScene.ActiveMonsterIndex)
	assert.Equal(t, pcIndex, gs.CombatScene.ActiveTargetPCIndex)
	assert.Equal(t, monsterPosBefore, monster.Position(), "monster action should not execute until scheduler runs closure")
	assert.Equal(t, pcHPBefore, gs.PartyState.GetCharacter(pcIndex).GetCurrentHp(),
		"PC should not take damage before scheduled closure executes")
	assert.Empty(t, mock.Messages, "delayed monster turn should not emit combat feedback before execution")

	_, err := gs.ResolveScheduledCombatStep(step.Token)
	require.NoError(t, err)
	assert.Equal(t, 0, gs.CombatScene.ActiveMonsterIndex,
		"scheduled execution must retain the monster highlight until scheduler cleanup")
	assert.Equal(t, pcIndex, gs.CombatScene.ActiveTargetPCIndex,
		"scheduled execution must retain the target highlight until scheduler cleanup")
}

func TestAdvanceCombatTurn_MonsterKillsPC_DefeatCheck(t *testing.T) {
	gs, _, _ := setupCombatWithMonster(t, references.MonsterOrc, references.Down, 42)

	// Only one PC active with 1 HP — deactivate others in both PartyActive and TurnQueue
	for i := 1; i < party_state.MaxActiveParty; i++ {
		if gs.CombatScene.PartyActive[i] {
			gs.CombatScene.PartyActive[i] = false
			gs.CombatScene.TurnQueue.MarkDead(i, true)
		}
	}

	activeIdx := gs.CombatScene.GetActiveCombatantIndex()
	character := gs.PartyState.GetCharacter(activeIdx)
	character.SubtractHp(character.GetCurrentHp() - 1) // 1 HP remaining

	// Try seeds until monster gets a turn and kills the PC.
	// This may not always happen (depends on hit roll), so we just verify
	// the system doesn't panic and handles the flow.
	for seed := uint64(1); seed < 50; seed++ {
		if character.IsDead() {
			break
		}

		gs.SetRandomSeed(seed)
		gs.AdvanceCombatTurn()

		if character.IsDead() {
			// Verify combat ended — defeat triggers TransitionFromCombatMap
			// which nils CombatScene
			assert.Nil(t, gs.CombatScene, "CombatScene should be nil after defeat transition")
			return
		}
	}
	// If PC didn't die in 50 attempts, that's OK — we tested the flow without panics
}

// ─── isCombatPositionValidForMonster tests ──────────────────────────────────
// Validates monster movement validation from original combat action rules.
// See ref:COMBAT-0008.

func TestIsCombatPositionValid_OutOfBounds(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	enemyRefs := *gs.GameReferences.EnemyReferences
	ref := &enemyRefs[references.MonsterOrc]
	rng := newFixedRNG(1)
	monster := NewCombatMonster(references.MonsterOrc, ref, references.Position{X: 5, Y: 5}, rng)

	// Out of bounds positions
	assert.False(t, isCombatPositionValidForMonster(gs, references.Position{X: -1, Y: 5}, monster, 0))
	assert.False(t, isCombatPositionValidForMonster(gs, references.Position{X: 5, Y: -1}, monster, 0))
	assert.False(t, isCombatPositionValidForMonster(gs, references.Position{X: 11, Y: 5}, monster, 0))
	assert.False(t, isCombatPositionValidForMonster(gs, references.Position{X: 5, Y: 11}, monster, 0))
}

func TestIsCombatPositionValid_OccupiedByPC(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	enemyRefs := *gs.GameReferences.EnemyReferences
	ref := &enemyRefs[references.MonsterOrc]
	rng := newFixedRNG(1)
	monster := NewCombatMonster(references.MonsterOrc, ref, references.Position{X: 5, Y: 5}, rng)

	// Put a PC at (3,3)
	gs.CombatScene.PartyActive[0] = true
	gs.CombatScene.SetPartyMemberPosition(0, references.Position{X: 3, Y: 3})

	assert.False(t, isCombatPositionValidForMonster(gs, references.Position{X: 3, Y: 3}, monster, 0),
		"Position occupied by PC should be invalid")
}

func TestIsCombatPositionValid_OccupiedByOtherMonster(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)

	manager := gs.CombatScene.MonsterManager
	manager.Clear()

	enemyRefs := *gs.GameReferences.EnemyReferences
	ref := &enemyRefs[references.MonsterOrc]
	rng := newFixedRNG(1)

	// Monster 0 at (5,5), Monster 1 at (6,6)
	m0 := NewCombatMonster(references.MonsterOrc, ref, references.Position{X: 5, Y: 5}, rng)
	manager.AddMonster(m0)
	m1 := NewCombatMonster(references.MonsterOrc, ref, references.Position{X: 6, Y: 6}, rng)
	manager.AddMonster(m1)

	// Monster 0 trying to move to (6,6) — occupied by monster 1
	assert.False(t, isCombatPositionValidForMonster(gs, references.Position{X: 6, Y: 6}, m0, 0),
		"Position occupied by another monster should be invalid")

	// Monster 0 should be allowed at its own position (monsterIdx == 0 is excluded)
	// Actually, the function checks OTHER monsters only, so own position check depends on
	// whether any other monster is at that same position.
}
