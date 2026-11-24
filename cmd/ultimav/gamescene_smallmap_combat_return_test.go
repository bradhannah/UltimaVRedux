package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestProductionSmallMapAttackInputCompletesOneFullTurnAfterEveryConclusion(t *testing.T) {
	for _, conclusion := range []game_state.CombatEndReason{
		game_state.CombatVictory,
		game_state.CombatDefeat,
		game_state.CombatEvacuated,
	} {
		t.Run(smallMapInputCombatConclusionName(conclusion), func(t *testing.T) {
			scene, state, callbacks, _, direction := newSmallMapCombatCommandScene(t)

			scene.smallMapInputHandler(ebiten.KeyA)
			require.Equal(t, AttackDirectionInput, scene.secondaryKeyState)
			scene.keyboard.SimulateKeyPress(smallMapMovementKey(direction))
			scene.smallMapHandleSecondaryInput()

			require.Equal(t, PrimaryInput, scene.secondaryKeyState)
			require.NotNil(t, state.CombatScene)
			beforeTurn := state.DateTime
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 2,
			}
			callbacks.Reset()

			state.EndCombat(conclusion)

			expectedDate := beforeTurn
			expectedDate.Advance(game_state.DefaultSmallMapMinutesPerTurn)
			assert.Equal(t, expectedDate, state.DateTime)
			assert.Equal(t, 1, state.ActiveDurationEffect.TurnsRemaining)
			require.Len(t, callbacks.CombatConclusions, 1)
			assert.Equal(t, conclusion, callbacks.CombatConclusions[0].Reason)
		})
	}
}

func TestProductionSmallMapAttackInputAcceptsShadowlordOriginsAndConclusions(t *testing.T) {
	sourceKinds := []game_state.SmallMapEncounterSourceKind{
		game_state.SmallMapEncounterAutomaticShadowlord,
		game_state.SmallMapEncounterYellShadowlord,
	}
	conclusions := []game_state.CombatEndReason{
		game_state.CombatVictory,
		game_state.CombatDefeat,
		game_state.CombatEvacuated,
	}

	for _, sourceKind := range sourceKinds {
		for _, ownsSceptre := range []bool{false, true} {
			for _, conclusion := range conclusions {
				name := productionShadowlordSourceName(sourceKind) + "/without_Sceptre/" +
					smallMapInputCombatConclusionName(conclusion)
				if ownsSceptre {
					name = productionShadowlordSourceName(sourceKind) + "/with_Sceptre/" +
						smallMapInputCombatConclusionName(conclusion)
				}
				t.Run(name, func(t *testing.T) {
					scene, state, callbacks, source, direction := newProductionShadowlordAttackScene(
						t,
						sourceKind,
						ownsSceptre,
					)
					beforeTurn := state.DateTime

					runProductionSmallMapAttackInput(t, scene, direction)

					require.NotNil(t, state.CombatScene)
					require.NotNil(t, state.CombatScene.SmallMapOrigin)
					assert.Same(t, source, state.CombatScene.SmallMapOrigin.SourceEnemy)
					assert.Equal(t, sourceKind, state.CombatScene.SmallMapOrigin.SourceKind)
					assert.Equal(t, game_state.SmallMapEncounterAttack, state.CombatScene.SmallMapOrigin.Cause)
					assert.Equal(t, references.ShadowlordCombatMap, state.CombatScene.SmallMapOrigin.CombatMap)
					assert.Equal(t, 1,
						state.CombatScene.MonsterManager.GetMonsterTypeCount(references.MonsterShadowlord))
					assert.Equal(t, uint16(0), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
					assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
					assert.True(t, state.ShadowlordState.IsAlive(references.Faulinei))
					expectedReclaims := 0
					if ownsSceptre {
						expectedReclaims = 1
					}
					assert.Equal(t, expectedReclaims,
						countEqual(callbacks.Messages, "The Sceptre is reclaimed!"))
					if ownsSceptre {
						assert.Equal(t, []game_state.SoundEffect{game_state.SoundShardPulse}, callbacks.SoundEffectsPlayed)
					} else {
						assert.Empty(t, callbacks.SoundEffectsPlayed)
					}

					state.EndCombat(conclusion)

					assert.Nil(t, state.CombatScene)
					expectedDate := beforeTurn
					expectedDate.Advance(game_state.DefaultSmallMapMinutesPerTurn)
					assert.Equal(t, expectedDate, state.DateTime)
					assert.Equal(t, uint16(0), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
					assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
					assert.True(t, state.ShadowlordState.IsAlive(references.Faulinei))
					require.Len(t, callbacks.CombatConclusions, 1)
					assert.Equal(t, conclusion, callbacks.CombatConclusions[0].Reason)
				})
			}
		}
	}
}

func TestProductionSmallMapAttackInputRejectsInvalidShadowlordTargets(t *testing.T) {
	tests := []struct {
		name         string
		mutate       func(*testing.T, *game_state.GameState, *map_units.NPCEnemy)
		wantFeedback string
	}{
		{
			name: "out of range",
			mutate: func(_ *testing.T, _ *game_state.GameState, source *map_units.NPCEnemy) {
				source.SetPos(source.PosPtr().Up())
			},
			wantFeedback: "Nothing to attack!",
		},
		{
			name: "different floor",
			mutate: func(_ *testing.T, _ *game_state.GameState, source *map_units.NPCEnemy) {
				source.SetFloor(1)
			},
			wantFeedback: "Nothing to attack!",
		},
		{
			name: "stale entity",
			mutate: func(t *testing.T, state *game_state.GameState, source *map_units.NPCEnemy) {
				require.True(t, state.CurrentNPCAIController.GetNpcs().RemoveEnemy(source))
			},
			wantFeedback: "Nothing to attack!",
		},
		{
			name: "stale provenance",
			mutate: func(_ *testing.T, state *game_state.GameState, _ *map_units.NPCEnemy) {
				state.ShadowlordState.ClearAutomaticPresence()
			},
			wantFeedback: "Combat could not begin.",
		},
		{
			name:         "non Shadowlord enemy",
			mutate:       replaceProductionShadowlordWithOrc,
			wantFeedback: "Nothing to attack!",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks, source, direction := newProductionShadowlordAttackScene(
				t,
				game_state.SmallMapEncounterAutomaticShadowlord,
				true,
			)
			beforeTurn := state.DateTime
			scene.smallMapInputHandler(ebiten.KeyA)
			require.Equal(t, AttackDirectionInput, scene.secondaryKeyState)
			test.mutate(t, state, source)

			scene.keyboard.SimulateKeyPress(smallMapMovementKey(direction))
			scene.smallMapHandleSecondaryInput()

			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Nil(t, state.CombatScene)
			expectedTurn := beforeTurn
			expectedTurn.Advance(game_state.DefaultSmallMapMinutesPerTurn)
			assert.Equal(t, expectedTurn, state.DateTime)
			assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
			assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
			assert.True(t, state.ShadowlordState.IsAlive(references.Faulinei))
			assert.Equal(t, 1, countEqual(callbacks.Messages, test.wantFeedback))
			assert.NotContains(t, callbacks.Messages, "The Sceptre is reclaimed!")
			assert.Empty(t, callbacks.SoundEffectsPlayed)
			assert.Empty(t, callbacks.CombatConclusions)
		})
	}

	t.Run("Escape cancels direction prompt without a cycle", func(t *testing.T) {
		scene, state, callbacks, source, _ := newProductionShadowlordAttackScene(
			t,
			game_state.SmallMapEncounterAutomaticShadowlord,
			true,
		)
		beforeTurn := state.DateTime
		beforeRNG := state.GetRNG().Calls()

		scene.smallMapInputHandler(ebiten.KeyA)
		require.Equal(t, AttackDirectionInput, scene.secondaryKeyState)
		source.SetPos(references.Position{X: 20, Y: 20})
		scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
		scene.smallMapHandleSecondaryInput()

		assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
		assert.Nil(t, state.CombatScene)
		assert.Equal(t, beforeTurn, state.DateTime)
		assert.Equal(t, beforeRNG, state.GetRNG().Calls())
		assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.Sceptre))
		assert.Equal(t, uint16(1), state.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
		assert.False(t, state.HasPendingOverworldContact())
		assert.True(t, source.IsVisible())
		assert.Empty(t, callbacks.Messages)
		assert.Empty(t, callbacks.SoundEffectsPlayed)
	})
}

func TestArrestRefusalTalkTurnOwnershipDistinguishesManualAndNPCAutoTalk(t *testing.T) {
	for _, test := range []struct {
		name           string
		npcInitiated   bool
		wantTalkMinute bool
	}{
		{name: "manual Talk", wantTalkMinute: true},
		{name: "NPC initiated arrest", npcInitiated: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, state, callbacks, source, direction := newSmallMapCombatCommandScene(t)
			state.SetCriminalStatus(true)
			beforeTalk := state.DateTime
			if test.npcInitiated {
				scene.startNPCTalkWorkflow(source)
			} else {
				scene.StartTalkWorkflow(direction)
			}
			refuseArrestWorkflow(t, scene)
			require.True(t, state.HasPendingOverworldContact())
			expectedAfterTalk := beforeTalk
			if test.wantTalkMinute {
				expectedAfterTalk.Advance(game_state.DefaultSmallMapMinutesPerTurn)
				assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
			} else {
				assert.Empty(t, callbacks.TimeAdvanced())
			}
			assert.Equal(t, expectedAfterTalk, state.DateTime)
			require.NoError(t, state.ResolvePendingOverworldContact())
			require.NotNil(t, state.CombatScene)
			beforeTurn := state.DateTime
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 2,
			}

			state.EndCombat(game_state.CombatDefeat)

			assert.Equal(t, beforeTurn, state.DateTime)
			assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
		})
	}
}

func TestProductionWindemereDevilHostileContactQueuesOrdinaryCombat(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Windemere).
		WithFloor(references.MainFloor).
		WithSystemCallbacks().
		WithRandomSeed(472).
		Build()
	state.ShadowlordState.Reset()
	controller, ok := state.CurrentNPCAIController.(*ai.NPCAIControllerSmallMap)
	require.True(t, ok)
	var source *map_units.NPCFriendly
	for _, unit := range *controller.GetNpcs() {
		friendly, friendlyOK := unit.(*map_units.NPCFriendly)
		if !friendlyOK || friendly.MapUnitDetails().NPCNum != 10 || friendly.NPCReference.Location != references.Windemere {
			continue
		}
		if monster, classified := friendly.NPCReference.SmallMapCombatMonsterType(); classified && monster == references.MonsterDevil {
			source = friendly

			break
		}
	}
	require.NotNil(t, source)
	state.MapState.PlayerLocation.Position = source.Pos().Down()
	scene := &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 64, 128),
	}
	assert.Equal(t, TalkTypeNormal, scene.determineTalkType(source, false))
	assert.Equal(t, "No response!", scene.getSpecialNPCResponseMessage(source))
	callbacks.Reset()
	originalPosition := source.Pos()
	controller.AdvanceNextTurnCalcAndMoveNPCs()
	assert.Equal(t, originalPosition, source.Pos())
	assert.Equal(t, map_units.PendingNPCInteractionNone, source.MapUnitDetails().PendingInteraction)

	activation, err := state.ActivateGuards()
	require.NoError(t, err)
	require.Positive(t, activation.HostileAfter)
	controller.AdvanceNextTurnCalcAndMoveNPCs()
	require.Equal(t,
		map_units.PendingNPCInteractionHostileCombat,
		source.MapUnitDetails().PendingInteraction,
	)

	require.True(t, scene.processNPCAutoTalk())
	assert.Equal(t, map_units.PendingNPCInteractionNone, source.MapUnitDetails().PendingInteraction)
	assert.True(t, state.HasPendingOverworldContact())
	assert.Nil(t, state.CombatScene)
	require.NoError(t, state.ResolvePendingOverworldContact())
	require.NotNil(t, state.CombatScene)
	origin := state.CombatScene.SmallMapOrigin
	require.NotNil(t, origin)
	assert.Same(t, source, origin.SourceNPC)
	assert.Equal(t, game_state.SmallMapEncounterHostileNPCAdjacency, origin.Cause)
	assert.Equal(t, references.MonsterDevil, origin.MonsterType)
	assert.Zero(t, origin.Consequences.GuardActivationCount)
	assert.False(t, origin.Consequences.CompleteSourceTurn)
}

func TestProductionVisitLocalRatAndBatBlockedContactOwnsOnlyMovementTurn(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		npcSlot  int
		monster  references.MonsterType
		position references.Position
	}{
		{name: "Iolo hut rat", location: references.IolosHut, npcSlot: 1, monster: references.MonsterRat, position: references.Position{X: 23, Y: 8}},
		{name: "Skara Brae bat", location: references.SkaraBrae, npcSlot: 8, monster: references.MonsterBat, position: references.Position{X: 15, Y: 15}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithFloor(references.MainFloor).
				WithSystemCallbacks().
				WithRandomSeed(484).
				Build()
			state.ShadowlordState.Reset()
			source := productionVisitLocalRatOrBatBySlot(t, state, test.location, test.npcSlot, test.monster)
			player, direction, found := emptyProductionVisitLocalAttackPosition(state, source)
			require.True(t, found)
			state.MapState.PlayerLocation.Position = player
			state.CurrentNPCAIController.GetNpcs().ReplaceWith([]map_units.MapUnit{source})
			state.CurrentNPCAIController.FreshenExistingNPCsOnMap()
			state.PartyState.Karma.Value = 60
			state.SetCriminalStatus(false)
			callbacks.Reset()
			scene := &GameScene{
				gameState:            state,
				keyboard:             input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
				output:               text.NewOutput(nil, 1, 64, 128),
				autoOpenDoorState:    &AutoOpenDoorState{},
				moongateVisualSystem: NewMoongateVisualSystem(state),
			}
			beforeMovement := state.DateTime

			scene.smallMapInputHandler(smallMapMovementKey(direction))

			assert.Equal(t, player, state.MapState.PlayerLocation.Position)
			expectedAfterMovement := beforeMovement
			expectedAfterMovement.Advance(game_state.DefaultSmallMapMinutesPerTurn)
			assert.Equal(t, expectedAfterMovement, state.DateTime)
			assert.Equal(t, []int{game_state.DefaultSmallMapMinutesPerTurn}, callbacks.TimeAdvanced())
			assert.Equal(t,
				map_units.PendingNPCInteractionHostileCombat,
				source.MapUnitDetails().PendingInteraction,
			)
			assert.False(t, state.HasPendingOverworldContact())

			require.True(t, scene.processNPCAutoTalk())
			assert.Equal(t, map_units.PendingNPCInteractionNone, source.MapUnitDetails().PendingInteraction)
			assert.True(t, state.HasPendingOverworldContact())
			assert.Equal(t, expectedAfterMovement, state.DateTime)
			require.NoError(t, state.ResolvePendingOverworldContact())
			require.NotNil(t, state.CombatScene)
			origin := state.CombatScene.SmallMapOrigin
			require.NotNil(t, origin)
			assert.Same(t, source, origin.SourceNPC)
			assert.Equal(t, game_state.SmallMapEncounterHostileNPCAdjacency, origin.Cause)
			assert.Equal(t, test.monster, origin.MonsterType)
			assert.Equal(t, 1, state.CombatScene.MonsterManager.Count())
			assert.Equal(t, test.monster, state.CombatScene.MonsterManager.GetMonster(0).MonsterType())
			assert.Equal(t, 60, state.PartyState.Karma.Value)
			assert.False(t, state.IsCriminal())
			assert.False(t, state.PartyState.IsNPCDead(test.location, test.npcSlot))
			assert.Zero(t, origin.Consequences.KarmaPenalty)
			assert.False(t, origin.Consequences.ActivateGuards)
			assert.False(t, origin.Consequences.PersistentSourceDead)
			assert.False(t, origin.Consequences.CompleteSourceTurn)
			assert.Zero(t, origin.Consequences.GuardActivationCount)
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType:     game_state.DurationEffectProtection,
				TurnsRemaining: 2,
			}

			state.EndCombat(game_state.CombatDefeat)

			assert.Equal(t, expectedAfterMovement, state.DateTime)
			assert.Equal(t, 2, state.ActiveDurationEffect.TurnsRemaining)
			assert.Nil(t, state.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
				test.position,
				references.MainFloor,
			))
			assert.False(t, state.PartyState.IsNPCDead(test.location, test.npcSlot))

			state.UpdateSmallMap(state.GameReferences.TileReferences, state.GameReferences.LocationReferences)
			reloaded := productionVisitLocalRatOrBatBySlot(t, state, test.location, test.npcSlot, test.monster)
			assert.Equal(t, test.position, reloaded.Pos())
			assert.True(t, reloaded.IsVisible())
		})
	}
}

func newSmallMapCombatCommandScene(
	t *testing.T,
) (*GameScene, *game_state.GameState, *game_state.MockSystemCallbacks, *map_units.NPCFriendly, references.Direction) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(319).
		Build()
	state.ShadowlordState.Reset()
	units := state.CurrentNPCAIController.GetNpcs()
	var source *map_units.NPCFriendly
	var direction references.Direction
	for _, unit := range *units {
		guard, ok := unit.(*map_units.NPCFriendly)
		if !ok || !guard.IsVisible() || guard.Floor() != state.MapState.PlayerLocation.Floor ||
			guard.NPCReference.GetNPCType() != references.Guard {
			continue
		}
		baseTile := state.GetLayeredMapByCurrentLocation().GetTileTopMapOnlyTile(guard.PosPtr())
		if baseTile == nil || smallMapInputDefenselessTile(baseTile.Index) {
			continue
		}
		player, attackDirection, found := emptySmallMapInputAttackPosition(units, guard)
		if !found {
			continue
		}
		state.MapState.PlayerLocation.Position = player
		guard.NPCReference.DialogNumber = byte(references.DialogGuard)
		source = guard
		direction = attackDirection

		break
	}
	require.NotNil(t, source)

	keyboard := input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)
	scene := &GameScene{
		gameState:         state,
		keyboard:          keyboard,
		output:            text.NewOutput(nil, 1, 64, 128),
		secondaryKeyState: PrimaryInput,
	}

	return scene, state, callbacks, source, direction
}

func newProductionShadowlordAttackScene(
	t *testing.T,
	sourceKind game_state.SmallMapEncounterSourceKind,
	ownsSceptre bool,
) (*GameScene, *game_state.GameState, *game_state.MockSystemCallbacks, *map_units.NPCEnemy, references.Direction) {
	t.Helper()
	location := references.Britain
	if sourceKind == game_state.SmallMapEncounterYellShadowlord {
		location = references.Lycaeum
	}
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(location).
		WithFloor(references.MainFloor).
		WithPlayerAt(10, 10).
		WithRandomSeed(317).
		WithSystemCallbacks().
		Build()
	state.ShadowlordState.Reset()
	units := state.CurrentNPCAIController.GetNpcs()
	units.ReplaceWith(nil)
	direction := references.Up
	var source *map_units.NPCEnemy
	switch sourceKind {
	case game_state.SmallMapEncounterAutomaticShadowlord:
		target := references.Position{X: 10, Y: 9}
		state.GetLayeredMapByCurrentLocation().SetTileByLayer(map_state.MapOverrideLayer, &target, indexes.Grass)
		state.ShadowlordState.TownAssignments[references.Faulinei] = location
		state.ShadowlordState.AutomaticPresence = references.Faulinei
		require.True(t, state.SpawnShadowlordNPCAtPosition(references.Faulinei, target))
		source = units.GetVisibleEnemyAtPositionAndFloorOrNil(target, references.MainFloor)
	case game_state.SmallMapEncounterYellShadowlord:
		require.True(t, state.ActionYellSmallMap("FAULINEI"))
		target := references.Position{X: 10, Y: 8}
		source = units.GetVisibleEnemyAtPositionAndFloorOrNil(target, references.MainFloor)
		state.MapState.PlayerLocation.Position = references.Position{X: 10, Y: 9}
	case game_state.SmallMapEncounterOrdinaryNPC:
		t.Fatal("Shadowlord production-input fixture requires Shadowlord provenance")
	}
	require.NotNil(t, source)
	if ownsSceptre {
		state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 1)
	} else {
		state.PartyState.Inventory.QuestItems.Set(references.Sceptre, 0)
	}
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)
	callbacks.Reset()
	scene := &GameScene{
		gameState:         state,
		keyboard:          input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:            text.NewOutput(nil, 1, 64, 128),
		autoOpenDoorState: &AutoOpenDoorState{},
	}

	return scene, state, callbacks, source, direction
}

func runProductionSmallMapAttackInput(t *testing.T, scene *GameScene, direction references.Direction) {
	t.Helper()
	scene.smallMapInputHandler(ebiten.KeyA)
	require.Equal(t, AttackDirectionInput, scene.secondaryKeyState)
	scene.keyboard.SimulateKeyPress(smallMapMovementKey(direction))
	scene.smallMapHandleSecondaryInput()
	require.Equal(t, PrimaryInput, scene.secondaryKeyState)
}

func replaceProductionShadowlordWithOrc(
	t *testing.T,
	state *game_state.GameState,
	source *map_units.NPCEnemy,
) {
	t.Helper()
	units := state.CurrentNPCAIController.GetNpcs()
	require.True(t, units.RemoveEnemy(source))
	enemyReferences := *state.GameReferences.EnemyReferences
	orc := map_units.NewEnemyNPC(enemyReferences[references.MonsterOrc], references.DynamicSpawnIndex)
	orc.SetPos(source.Pos())
	orc.SetFloor(source.Floor())
	orc.SetVisible(true)
	require.True(t, units.AddEnemy(orc))
}

func productionShadowlordSourceName(sourceKind game_state.SmallMapEncounterSourceKind) string {
	if sourceKind == game_state.SmallMapEncounterAutomaticShadowlord {
		return "automatic"
	}

	return "yell"
}

func productionVisitLocalRatOrBatBySlot(
	t *testing.T,
	state *game_state.GameState,
	location references.Location,
	slot int,
	monster references.MonsterType,
) *map_units.NPCFriendly {
	t.Helper()
	for _, unit := range *state.CurrentNPCAIController.GetNpcs() {
		friendly, ok := unit.(*map_units.NPCFriendly)
		if !ok || friendly.NPCReference.Location != location || friendly.MapUnitDetails().NPCNum != slot {
			continue
		}
		classifiedMonster, classified := friendly.NPCReference.SmallMapCombatMonsterType()
		if classified && classifiedMonster == monster {
			return friendly
		}
	}

	t.Fatalf("%s slot %d is not an active %s", location, slot, monster)

	return nil
}

func emptyProductionVisitLocalAttackPosition(
	state *game_state.GameState,
	source *map_units.NPCFriendly,
) (references.Position, references.Direction, bool) {
	for _, candidate := range []struct {
		fromSource references.Direction
		attack     references.Direction
	}{
		{fromSource: references.Down, attack: references.Up},
		{fromSource: references.Up, attack: references.Down},
		{fromSource: references.Left, attack: references.Right},
		{fromSource: references.Right, attack: references.Left},
	} {
		player := candidate.fromSource.GetNewPositionInDirection(source.PosPtr())
		if player == nil || state.IsOutOfBounds(*player) || !state.IsPassable(player) {
			continue
		}

		return *player, candidate.attack, true
	}

	return references.Position{}, references.NoneDirection, false
}

func emptySmallMapInputAttackPosition(
	units *map_units.MapUnits,
	source *map_units.NPCFriendly,
) (references.Position, references.Direction, bool) {
	for _, candidate := range []struct {
		fromSource references.Direction
		attack     references.Direction
	}{
		{fromSource: references.Down, attack: references.Up},
		{fromSource: references.Up, attack: references.Down},
		{fromSource: references.Left, attack: references.Right},
		{fromSource: references.Right, attack: references.Left},
	} {
		player := candidate.fromSource.GetNewPositionInDirection(source.PosPtr())
		if player == nil || player.X < 0 || player.X >= references.XSmallMapTiles ||
			player.Y < 0 || player.Y >= references.YSmallMapTiles {
			continue
		}
		if units.GetMapUnitAtPositionAndFloorOrNil(*player, source.Floor()) == nil {
			return *player, candidate.attack, true
		}
	}

	return references.Position{}, references.NoneDirection, false
}

func smallMapInputDefenselessTile(index indexes.SpriteIndex) bool {
	return index == indexes.Stocks || index == indexes.Manacles ||
		index == indexes.LeftBed || index == indexes.MirrorBroken
}

func refuseArrestWorkflow(t *testing.T, scene *GameScene) {
	t.Helper()
	var dialog *widgets.InteractionTranscript
	for range 30 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if current, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			current.ActivePrompt() == "Wilt thou come quietly?" && !current.InputComplete() {
			dialog = current

			break
		}
	}
	require.NotNil(t, dialog)
	scene.keyboard.SimulateKeyPress(ebiten.KeyN)
	dialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	for range 30 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasActiveWorkflows() {
			break
		}
	}
	require.False(t, scene.gameState.EventQueueManager.HasActiveWorkflows())
}

func smallMapInputCombatConclusionName(reason game_state.CombatEndReason) string {
	switch reason {
	case game_state.CombatVictory:
		return "victory"
	case game_state.CombatDefeat:
		return "defeat"
	case game_state.CombatEvacuated:
		return "evacuated"
	default:
		return "unknown"
	}
}
