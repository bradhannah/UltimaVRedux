package game_state

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

type shardRitualFixture struct {
	name     string
	location references.Location
	floor    references.FloorNumber
	position references.Position
	shard    references.QuestItem
	lord     references.Shadowlord
}

var shardRitualFixtures = []shardRitualFixture{
	{
		name: "Falsehood at Truth", location: references.Lycaeum, floor: 2,
		position: references.Position{X: 15, Y: 9}, shard: references.ShardOfFalsehood, lord: references.Faulinei,
	},
	{
		name: "Hatred at Love", location: references.EmpathAbbey, floor: 1,
		position: references.Position{X: 15, Y: 3}, shard: references.ShardOfHatred, lord: references.Astaroth,
	},
	{
		name: "Cowardice at Courage", location: references.SerpentsHold, floor: -1,
		position: references.Position{X: 15, Y: 16}, shard: references.ShardOfCowardice, lord: references.Nosfentor,
	},
}

func TestUseItemDispatchesExactSelectedShard(t *testing.T) {
	state, _ := NewIntegrationTestBuilder(t).WithSystemCallbacks().Build()
	for _, fixture := range shardRitualFixtures {
		state.PartyState.Inventory.QuestItems.Set(fixture.shard, 1)
	}
	selected := make([]references.QuestItem, 0, len(shardRitualFixtures))
	state.SystemCallbacks.Flow.Shadowlord.ShowShardUsageWorkflow = func(shard references.QuestItem, _ func()) {
		selected = append(selected, shard)
	}

	for _, fixture := range shardRitualFixtures {
		require.True(t, state.UseItem(UseableItemInfo{ItemType: fixture.shard}))
	}

	assert.Equal(t, []references.QuestItem{
		references.ShardOfFalsehood,
		references.ShardOfHatred,
		references.ShardOfCowardice,
	}, selected)
}

func TestCommitShardRitualConsumesAndDefeatsOnlySelectedIdentity(t *testing.T) {
	for _, fixture := range shardRitualFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			state, enemy := newShardRitualTestState(t, fixture)
			for _, other := range shardRitualFixtures {
				state.PartyState.Inventory.QuestItems.Set(other.shard, 1)
			}
			state.PartyState.Inventory.QuestItems.Set(fixture.shard, 2)
			deadNPCIndex := shadowlordDeadNPCIndex(fixture.lord)
			var journalChanges [][2]int
			state.SystemCallbacks.UI.JournalChanged = func(tab, entry int) {
				journalChanges = append(journalChanges, [2]int{tab, entry})
			}

			review := state.ReviewShardRitual(fixture.shard)
			require.Equal(t, ShardUsageSuccess, review.Result)
			assert.True(t, review.AtMatchingFlame)
			assert.Equal(t, fixture.position.Up(), review.Target)
			assert.Equal(t, fixture.lord, review.Shadowlord)
			assert.False(t, state.PartyState.IsNPCDead(references.Stonegate, deadNPCIndex))

			commit, prepared := state.BeginShardRitualCommit(fixture.shard)
			require.NotNil(t, commit)
			require.Equal(t, ShardUsageSuccess, prepared.Result)
			assert.Equal(t, uint16(2), state.PartyState.Inventory.QuestItems.Get(fixture.shard))
			assert.True(t, state.ShadowlordState.IsAlive(fixture.lord))
			assert.False(t, state.PartyState.IsNPCDead(references.Stonegate, deadNPCIndex))
			assert.Contains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(enemy),
				"Begin must not remove the physical Shadowlord")

			committed := state.CommitShardRitual(commit)
			assert.Equal(t, [][2]int{{journal.ShadowlordsTab, int(fixture.lord)}, {journal.ChronicleTab, 1}}, journalChanges)

			assert.Equal(t, ShardUsageSuccess, committed.Result)
			assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(fixture.shard))
			assert.False(t, state.ShadowlordState.IsAlive(fixture.lord))
			assert.False(t, state.ShadowlordState.Summoned)
			assert.Equal(t, references.NoShadowlord, state.ShadowlordState.LastCalled)
			assert.True(t, state.PartyState.IsNPCDead(references.Stonegate, deadNPCIndex))
			assert.False(t, state.PartyState.IsNPCDead(fixture.location, enemy.MapUnitDetails().NPCNum),
				"the temporary Keep slot must not be persisted as dead")
			events := state.QuestJournalEvents()
			require.Len(t, events, 1)
			expectedSubject, supported := questJournalSubjectForShadowlord(fixture.lord)
			require.True(t, supported)
			assert.Equal(t, QuestJournalEventShadowlordDefeated, events[0].Kind)
			assert.Equal(t, expectedSubject, events[0].Subject)
			assert.Equal(t, QuestJournalSourceGameplay, events[0].Source)
			assert.NotNil(t, events[0].OccurredOn)
			assert.Nil(t, state.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(
				fixture.position.Up(), fixture.floor,
			))
			for _, other := range shardRitualFixtures {
				if other.shard == fixture.shard {
					continue
				}
				assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(other.shard))
				assert.True(t, state.ShadowlordState.IsAlive(other.lord))
			}
		})
	}
}

func TestShardRitualCommitsWithStaleDefeatJournalMetadata(t *testing.T) {
	fixture := shardRitualFixtures[0]
	state, enemy := newShardRitualTestState(t, fixture)
	require.True(t, state.RecordShadowlordDefeatEvent(fixture.lord))
	eventsBefore := state.QuestJournalEvents()

	commit, review := state.BeginShardRitualCommit(fixture.shard)
	require.NotNil(t, commit)
	require.Equal(t, ShardUsageSuccess, review.Result)

	var committed ShardRitualReview
	assert.NotPanics(t, func() { committed = state.CommitShardRitual(commit) })

	assert.Equal(t, ShardUsageSuccess, committed.Result)
	assert.False(t, state.ShadowlordState.IsAlive(fixture.lord))
	assert.True(t, state.PartyState.IsNPCDead(
		references.Stonegate,
		shadowlordDeadNPCIndex(fixture.lord),
	))
	assert.NotContains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(enemy))
	assert.Equal(t, eventsBefore, state.QuestJournalEvents(), "the duplicate event must retain its original metadata")
}

func TestDestroyedShardDoesNotRematerializeAfterRitual(t *testing.T) {
	fixture := shardRitualFixtures[0]
	state, _ := newShardRitualTestState(t, fixture)

	commit, review := state.BeginShardRitualCommit(fixture.shard)
	require.NotNil(t, commit)
	require.Equal(t, ShardUsageSuccess, review.Result)
	state.CommitShardRitual(commit)
	require.Zero(t, state.PartyState.Inventory.QuestItems.Get(fixture.shard))

	state.MapState.PlayerLocation.Location = references.BritanniaUnderworld
	state.MapState.PlayerLocation.Floor = references.Basement
	assert.False(t, state.IsShardVisible(references.Position{X: 192, Y: 80}, fixture.shard))
	assert.True(t, state.IsShardVisible(
		references.Position{X: 130, Y: 65},
		references.ShardOfHatred,
	), "destroying one Shadowlord must not hide another Shard")
}

func TestCommitShardRitualRejectsStaleOrMismatchedStateWithoutMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*GameState, *map_units.NPCEnemy)
	}{
		{
			name: "selected Shard removed",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 0)
			},
		},
		{
			name: "player left Flame",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.MapState.PlayerLocation.Position = state.MapState.PlayerLocation.Position.Right()
			},
		},
		{
			name: "Shadowlord moved away from one tile north",
			mutate: func(_ *GameState, enemy *map_units.NPCEnemy) {
				enemy.SetPos(enemy.Pos().Up())
			},
		},
		{
			name: "Shadowlord moved to another floor",
			mutate: func(_ *GameState, enemy *map_units.NPCEnemy) {
				enemy.SetFloor(1)
			},
		},
		{
			name: "Shadowlord became hidden",
			mutate: func(_ *GameState, enemy *map_units.NPCEnemy) {
				enemy.SetVisible(false)
			},
		},
		{
			name: "physical Shadowlord identity changed",
			mutate: func(_ *GameState, enemy *map_units.NPCEnemy) {
				enemy.SetShadowlordIdentity(references.Astaroth)
			},
		},
		{
			name: "summoned identity cleared",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.ShadowlordState.ClearSummoned()
			},
		},
	}

	fixture := shardRitualFixtures[0]
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, enemy := newShardRitualTestState(t, fixture)
			deadNPCIndex := shadowlordDeadNPCIndex(fixture.lord)
			require.Equal(t, ShardUsageSuccess, state.ReviewShardRitual(fixture.shard).Result)
			test.mutate(state, enemy)
			quantityBefore := state.PartyState.Inventory.QuestItems.Get(fixture.shard)

			commit, result := state.BeginShardRitualCommit(fixture.shard)

			assert.Nil(t, commit)
			assert.NotEqual(t, ShardUsageSuccess, result.Result)
			assert.Equal(t, quantityBefore, state.PartyState.Inventory.QuestItems.Get(fixture.shard))
			assert.True(t, state.ShadowlordState.IsAlive(fixture.lord))
			assert.False(t, state.PartyState.IsNPCDead(references.Stonegate, deadNPCIndex))
			assert.Contains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(enemy))
			assert.Empty(t, state.QuestJournalEvents())
		})
	}
}

func TestCommitShardRitualRemovesMatchingOverlappingEnemy(t *testing.T) {
	fixture := shardRitualFixtures[0]
	state, enemy := newShardRitualTestState(t, fixture)
	reference := state.enemyReference(references.MonsterShadowlord)
	require.NotNil(t, reference)
	decoy := map_units.NewEnemyNPC(*reference, references.DynamicSpawnIndex)
	decoy.SetShadowlordIdentity(references.Astaroth)
	decoy.SetPos(fixture.position.Up())
	decoy.SetFloor(fixture.floor)
	decoy.SetVisible(true)
	state.CurrentNPCAIController.GetNpcs().ReplaceWith([]map_units.MapUnit{&decoy, enemy})

	commit, review := state.BeginShardRitualCommit(fixture.shard)

	require.NotNil(t, commit)
	assert.Equal(t, ShardUsageSuccess, review.Result)
	assert.Contains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(&decoy))
	assert.Contains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(enemy))
	state.CommitShardRitual(commit)
	assert.Contains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(&decoy))
	assert.NotContains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(enemy))
}

func TestCommitShardRitualRejectsStateChangedAfterBeginWithoutMutation(t *testing.T) {
	fixture := shardRitualFixtures[0]
	tests := []struct {
		name   string
		mutate func(*GameState, *map_units.NPCEnemy)
	}{
		{
			name: "selected Shard quantity changed",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.PartyState.Inventory.QuestItems.Set(fixture.shard, 2)
			},
		},
		{
			name: "player left exact location",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.MapState.PlayerLocation.Position = state.MapState.PlayerLocation.Position.Right()
			},
		},
		{
			name: "exact Shadowlord moved",
			mutate: func(_ *GameState, enemy *map_units.NPCEnemy) {
				enemy.SetPos(enemy.Pos().Up())
			},
		},
		{
			name: "exact Shadowlord replaced",
			mutate: func(state *GameState, enemy *map_units.NPCEnemy) {
				reference := state.enemyReference(references.MonsterShadowlord)
				if reference == nil {
					panic("test fixture has no Shadowlord reference")
				}
				replacement := map_units.NewEnemyNPC(*reference, references.DynamicSpawnIndex)
				replacement.SetShadowlordIdentity(fixture.lord)
				replacement.SetPos(enemy.Pos())
				replacement.SetFloor(enemy.Floor())
				replacement.SetVisible(true)
				state.CurrentNPCAIController.GetNpcs().ReplaceWith([]map_units.MapUnit{&replacement})
			},
		},
		{
			name: "summoned progression changed",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.ShadowlordState.ClearSummoned()
			},
		},
		{
			name: "persistent defeat progression changed",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.PartyState.SetDeadNpc(references.Stonegate, shadowlordDeadNPCIndex(fixture.lord))
			},
		},
		{
			name: "gameplay time changed",
			mutate: func(state *GameState, _ *map_units.NPCEnemy) {
				state.DateTime.Advance(1)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, enemy := newShardRitualTestState(t, fixture)
			commit, review := state.BeginShardRitualCommit(fixture.shard)
			require.NotNil(t, commit)
			require.Equal(t, ShardUsageSuccess, review.Result)
			assert.Contains(t, *state.CurrentNPCAIController.GetNpcs(), map_units.MapUnit(enemy))

			test.mutate(state, enemy)
			quantityBefore := state.PartyState.Inventory.QuestItems.Get(fixture.shard)
			aliveBefore := state.ShadowlordState.IsAlive(fixture.lord)
			deadBefore := state.PartyState.IsNPCDead(references.Stonegate, shadowlordDeadNPCIndex(fixture.lord))
			unitsBefore := append([]map_units.MapUnit(nil), (*state.CurrentNPCAIController.GetNpcs())...)
			eventsBefore := state.QuestJournalEvents()

			var result ShardRitualReview
			assert.NotPanics(t, func() { result = state.CommitShardRitual(commit) })

			assert.Equal(t, ShardUsageNoEffect, result.Result)
			assert.Equal(t, quantityBefore, state.PartyState.Inventory.QuestItems.Get(fixture.shard))
			assert.Equal(t, aliveBefore, state.ShadowlordState.IsAlive(fixture.lord))
			assert.Equal(t, deadBefore,
				state.PartyState.IsNPCDead(references.Stonegate, shadowlordDeadNPCIndex(fixture.lord)))
			assert.Equal(t, unitsBefore, []map_units.MapUnit(*state.CurrentNPCAIController.GetNpcs()))
			assert.Equal(t, eventsBefore, state.QuestJournalEvents())
			assert.Panics(t, func() { state.CommitShardRitual(commit) }, "a consumed stale token must not be reusable")
		})
	}
}

func TestBeginShardRitualCommitRejectsMissingPhysicalShadowlordWithoutPanic(t *testing.T) {
	fixture := shardRitualFixtures[0]
	state, enemy := newShardRitualTestState(t, fixture)
	require.True(t, state.CurrentNPCAIController.GetNpcs().RemoveEnemy(enemy))

	var commit *ShardRitualCommit
	var review ShardRitualReview
	assert.NotPanics(t, func() {
		commit, review = state.BeginShardRitualCommit(fixture.shard)
	})

	assert.Nil(t, commit)
	assert.Equal(t, ShardUsageNoEffect, review.Result)
	assert.True(t, state.ShadowlordState.Summoned)
	assert.Equal(t, fixture.lord, state.ShadowlordState.LastCalled)
	assert.True(t, state.ShadowlordState.IsAlive(fixture.lord))
	assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(fixture.shard))
	assert.Empty(t, state.QuestJournalEvents())
}

func TestYellSummonedShadowlordMovesOntoEachFlame(t *testing.T) {
	for _, fixture := range shardRitualFixtures {
		t.Run(fixture.name, func(t *testing.T) {
			state, _ := NewIntegrationTestBuilder(t).
				WithLocation(fixture.location).
				WithFloor(fixture.floor).
				WithPlayerAt(int(fixture.position.X), int(fixture.position.Y)).
				WithSystemCallbacks().
				Build()
			units := state.CurrentNPCAIController.GetNpcs()
			units.ReplaceWith(nil)
			state.ShadowlordState.Reset()
			target := fixture.position.Up()
			tile := state.GetLayeredMapByCurrentLocation().GetTopTile(&target)
			require.NotNil(t, tile)
			require.Equal(t, indexes.SpriteIndex(indexes.BlueFlame), tile.Index)
			require.True(t, state.TrySummonShadowlord(fixture.lord.String()))
			enemy := matchingShadowlordAt(units, target.Up(), fixture.floor, fixture.lord)
			require.NotNil(t, enemy)

			state.CurrentNPCAIController.AdvanceNextTurnCalcAndMoveNPCs()

			assert.Equal(t, target, enemy.Pos(), "Shadowlords bypass terrain passability for the Flame step")
		})
	}
}

func TestYellSummonedShadowlordCannotEnterOccupiedFlame(t *testing.T) {
	fixture := shardRitualFixtures[0]
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(fixture.location).
		WithFloor(fixture.floor).
		WithPlayerAt(int(fixture.position.X), int(fixture.position.Y)).
		WithSystemCallbacks().
		Build()
	units := state.CurrentNPCAIController.GetNpcs()
	units.ReplaceWith(nil)
	state.ShadowlordState.Reset()
	require.True(t, state.TrySummonShadowlord(fixture.lord.String()))
	target := fixture.position.Up()
	shadowlord := matchingShadowlordAt(units, target.Up(), fixture.floor, fixture.lord)
	require.NotNil(t, shadowlord)
	reference := state.enemyReference(references.MonsterOrc)
	require.NotNil(t, reference)
	blocker := map_units.NewEnemyNPC(*reference, references.DynamicSpawnIndex)
	blocker.SetPos(target)
	blocker.SetFloor(fixture.floor)
	blocker.SetVisible(true)
	require.True(t, units.AddEnemy(blocker))

	state.CurrentNPCAIController.AdvanceNextTurnCalcAndMoveNPCs()

	assert.Equal(t, target.Up(), shadowlord.Pos(), "dynamic occupancy must still block Shadowlord movement")
}

func TestReviewShardRitualRequiresOwnedMatchingShardButDefersPhysicalFailure(t *testing.T) {
	fixture := shardRitualFixtures[0]
	state, enemy := newShardRitualTestState(t, fixture)

	state.PartyState.Inventory.QuestItems.Set(fixture.shard, 0)
	assert.Equal(t, ShardUsageNotAttempted, state.ReviewShardRitual(fixture.shard).Result)

	state.PartyState.Inventory.QuestItems.Set(references.ShardOfHatred, 1)
	wrongShard := state.ReviewShardRitual(references.ShardOfHatred)
	assert.Equal(t, ShardUsageNoEffect, wrongShard.Result)
	assert.False(t, wrongShard.AtMatchingFlame)

	state.PartyState.Inventory.QuestItems.Set(fixture.shard, 1)
	enemy.SetPos(enemy.Pos().Up())
	missingPhysicalObject := state.ReviewShardRitual(fixture.shard)
	assert.Equal(t, ShardUsageNoEffect, missingPhysicalObject.Result)
	assert.True(t, missingPhysicalObject.AtMatchingFlame,
		"the presentation must reach the earthquakes before the physical-object failure")
}

func newShardRitualTestState(t *testing.T, fixture shardRitualFixture) (*GameState, *map_units.NPCEnemy) {
	t.Helper()
	state, _ := NewIntegrationTestBuilder(t).
		WithLocation(fixture.location).
		WithFloor(fixture.floor).
		WithPlayerAt(int(fixture.position.X), int(fixture.position.Y)).
		WithSystemCallbacks().
		Build()
	require.NotNil(t, state.CurrentNPCAIController)
	require.NotNil(t, state.CurrentNPCAIController.GetNpcs())
	state.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
	state.PartyState.RestoreNPCFlags(nil, nil)
	state.ShadowlordState.Reset()
	state.PartyState.Inventory.QuestItems.Set(fixture.shard, 1)
	require.True(t, state.ShadowlordState.Summon(fixture.lord))
	require.True(t, state.SpawnShadowlordNPCAtPosition(fixture.lord, fixture.position.Up()))
	enemy := state.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(
		fixture.position.Up(), fixture.floor,
	)
	require.NotNil(t, enemy)

	return state, enemy
}
