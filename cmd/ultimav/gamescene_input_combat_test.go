package main

import (
	"slices"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

type fastCombatant struct{}

func (fastCombatant) GetDexterity() int { return 35 }
func (fastCombatant) IsAlive() bool     { return true }
func (fastCombatant) CanTakeTurn() bool { return true }
func (fastCombatant) IsPC() bool        { return true }

func nextNonActionablePCPresentationStep(
	t *testing.T,
	gs *game_state.GameState,
) game_state.CombatPresentationStep {
	t.Helper()
	for index := range gs.PartyState.GetCharacterCount() {
		if gs.CombatScene.IsPartyMemberActive(index) {
			gs.CombatScene.PartySleeping[index] = true
			gs.PartyState.GetCharacter(index).SetStatus(party_state.Sleep)
		}
	}
	gs.BeginCombatTurns(game_state.CombatTurnsPresented)
	for range 128 {
		if step, ok := gs.ReviewScheduledCombatStep(); ok {
			if step.Kind == game_state.CombatPresentationAutoPass {
				return step
			}
			if step.Kind == game_state.CombatPresentationMonsterTurn {
				result, err := gs.ResolveScheduledCombatStep(step.Token)
				require.NoError(t, err)
				if result.ProjectileLaunch != nil {
					gs.ResolvePendingMonsterMissile()
				}
			}
			require.NoError(t, gs.CompleteScheduledCombatStep(step.Token))

			continue
		}
		if !gs.CombatScene.HasScheduledCombatStep() {
			gs.FinishCombatAction()
		}
	}
	t.Fatal("combat did not schedule a non-actionable PC presentation step")

	return game_state.CombatPresentationStep{}
}

func TestCombatAutoAction_TabIsBound(t *testing.T) {
	assert.True(t, slices.Contains(boundKeysGame, ebiten.KeyTab))
}

func setupCombatAutoAttackInputTest(
	t *testing.T,
) (*GameScene, game_state.CombatTurnIdentity) {
	t.Helper()
	retirePresentationFixtureCaches(t)
	scene, _ := newCombatUseTestScene(t)
	state := scene.gameState
	require.False(t, state.CombatScene.CombatTurnsStarted())
	state.CombatScene.MonsterManager.Clear()
	state.CombatScene.TurnQueue.RemoveAllMonsters()
	state.CombatScene.ClearSoloMoveMode()
	state.ActiveDurationEffect = game_state.DurationEffect{}
	for x := range references.XCombatMapTiles {
		for y := range references.YCombatMapTiles {
			state.CombatScene.CombatMapRef.SetTileNumber(references.Position{
				X: references.Coordinate(x), Y: references.Coordinate(y),
			}, indexes.BrickFloor)
		}
	}
	start := references.Position{X: 5, Y: 5}
	target := references.Position{X: 4, Y: 4}
	monsterStart := references.Position{X: 10, Y: 10}
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &start, indexes.BrickFloor)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &target, indexes.BrickFloor)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &monsterStart, indexes.BrickFloor)
	activeCount := 0
	for index, active := range state.CombatScene.PartyActive {
		if !active {
			continue
		}
		activeCount++
		character := state.PartyState.GetCharacter(index)
		character.Dexterity = 35
		character.SetStatus(party_state.Good)
		character.SetEquippedItem(references.SlotHelm, references.NoEquipment)
		character.SetEquippedItem(references.SlotWeapon, references.ShortSword)
		character.SetEquippedItem(references.SlotShield, references.MainGauche)
		state.CombatScene.PartySleeping[index] = false
		state.CombatScene.PartyPossessed[index] = false
		state.CombatScene.PartyUnderground[index] = false
		// Park every PC before selecting the actor, avoiding inherited CBT party
		// occupancy at the victim, firing line, or newly selected actor's origin.
		state.CombatScene.SetPartyMemberPosition(index, references.Position{X: references.Coordinate(index + 1), Y: 10})
		identity := game_state.CombatTurnIdentity{IsPartyMember: true, EntityIndex: index}
		entry := state.CombatScene.TurnQueue.GetEntryByIdentity(identity)
		require.NotNil(t, entry)
		// DEX is live, but a countdown was already initialized at combat entry.
		// Reset it through the queue API while retaining its wired PC provider.
		state.CombatScene.TurnQueue.ReplaceCombatant(identity, identity, entry.Combatant)
		snapshot, ok := state.CombatScene.TurnQueue.GetEntrySnapshot(identity)
		require.True(t, ok)
		require.Equal(t, 35, snapshot.Dexterity)
		require.Equal(t, 1, snapshot.Countdown)
	}
	require.GreaterOrEqual(t, activeCount, 2, "fixture requires an observable next PC turn")
	enemyReferences := *state.GameReferences.EnemyReferences
	ratReference := enemyReferences[references.MonsterRat]
	ratReference.HitPoints = 100 // SetCurrentHP alone clamps to the licensed rat's much smaller maximum.
	state.SetRandomSeed(1)
	rat := game_state.NewCombatMonster(
		references.MonsterRat,
		&ratReference,
		monsterStart,
		state.AsRandomProvider(),
	)
	rat.SetCurrentHP(100) // Keep both weapon attacks observable regardless of their rolls.
	require.Equal(t, 100, rat.MaxHP())
	require.Equal(t, 100, rat.CurrentHP())
	state.CombatScene.AddMonsterToQueue(rat)
	var opening [1]game_state.CombatTurnForecastEntry
	require.Equal(t, 1, state.CombatScene.TurnQueue.FillForecast(opening[:]))
	require.True(t, opening[0].IsPartyMember)
	require.True(t, opening[0].CanTakeTurn)
	state.BeginCombatTurns(game_state.CombatTurnsPresented)
	actor := state.CombatScene.GetCurrentTurnIdentity()
	require.Equal(t, opening[0].CombatTurnIdentity, actor)
	require.True(t, actor.IsPartyMember)
	state.CombatScene.SetPartyMemberPosition(actor.EntityIndex, start)
	rat.SetPosition(target)
	character := state.PartyState.GetCharacter(actor.EntityIndex)
	require.Equal(t, references.ShortSword, character.GetEquippedItem(references.SlotWeapon))
	require.Equal(t, references.MainGauche, character.GetEquippedItem(references.SlotShield))
	require.Equal(t, start, state.CombatScene.GetCombatantPosition(actor))
	require.Equal(t, target, rat.Position())
	require.True(t, rat.IsHostile())
	require.True(t, rat.IsActive())
	require.False(t, rat.IsCorpserHidden())
	require.True(t, state.CombatScene.IsCombatPCActive(actor))
	require.False(t, state.CombatScene.IsCombatPCUnderground(actor))
	require.False(t, state.CombatScene.HasScheduledCombatStep())
	require.Nil(t, state.CombatScene.AttackTargeting)
	require.Equal(t, game_state.CombatAimEnemy, state.GetCombatAimDisposition(target), "victim must not share a party square")
	require.Equal(t, game_state.CombatAutoActionAttack, state.PlanCombatAutoAction().Kind)
	state.SystemCallbacks.Visual.ShowCombatImpact = scene.showCombatImpact
	state.SystemCallbacks.Visual.BlockAllInput = scene.blockAllInput

	return scene, actor
}

func TestCombatAutoAttackPresentsDualWeaponImpactsSequentially(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	state := scene.gameState

	scene.combatMapInputHandler(ebiten.KeyTab)

	assert.True(t, state.EventQueueManager.HasWorkflow(combatAutoAttackWorkflowName))
	assert.True(t, scene.inputBlocked)
	assert.Empty(t, scene.combatImpactEffects)
	assert.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())
	assert.False(t, state.CombatScene.HasScheduledCombatStep(), "action completion must wait for all sources")

	state.EventQueueManager.Update()
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Equal(t, references.ShortSword, scene.combatImpactEffects[0].Feedback.Presentation.Source)
	assert.Zero(t, combatImpactLane(scene.combatImpactEffects, 0))

	state.ElapsedMs += combatImpactDurationMs - 1
	state.EventQueueManager.Update()
	require.Len(t, scene.combatImpactEffects, 1, "later sources must wait for the entire impact hold")
	assert.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())
	require.NotNil(t, state.CombatScene.AttackTargeting)
	assert.Equal(t, 1, state.CombatScene.AttackTargeting.SourceIndex)

	scene.combatImpactEffects[0].Elapsed = scene.combatImpactEffects[0].timing.impactLabelSeconds()
	state.ElapsedMs++
	state.EventQueueManager.Update()
	scene.updateCombatImpactEffects(0)
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Equal(t, references.MainGauche, scene.combatImpactEffects[0].Feedback.Presentation.Source)
	assert.Zero(t, combatImpactLane(scene.combatImpactEffects, 0))
	assert.Equal(t, actor, state.CombatScene.GetCurrentTurnIdentity())

	scene.combatImpactEffects[0].Elapsed = scene.combatImpactEffects[0].timing.impactLabelSeconds()
	state.ElapsedMs += combatImpactDurationMs
	for range 4 {
		state.EventQueueManager.Update()
	}
	scene.updateCombatImpactEffects(0)
	assert.False(t, state.EventQueueManager.HasWorkflow(combatAutoAttackWorkflowName))
	assert.False(t, scene.inputBlocked)
	assert.Empty(t, scene.combatImpactEffects)
	assert.NotEqual(t, actor, state.CombatScene.GetCurrentTurnIdentity())
}

func TestCombatAutoAttackPresentsSingleWeaponImpactInOwningWorkflow(t *testing.T) {
	scene, actor := setupCombatAutoAttackInputTest(t)
	character := scene.gameState.PartyState.GetCharacter(actor.EntityIndex)
	character.SetEquippedItem(references.SlotShield, references.NoEquipment)

	scene.combatMapInputHandler(ebiten.KeyTab)

	assert.True(t, scene.inputBlocked)
	scene.gameState.EventQueueManager.Update()
	require.Len(t, scene.combatImpactEffects, 1)
	assert.Equal(t, references.ShortSword, scene.combatImpactEffects[0].Feedback.Presentation.Source)
	assert.True(t, scene.gameState.EventQueueManager.HasWorkflow(combatAutoAttackWorkflowName))
	scene.gameState.ElapsedMs += combatImpactDurationMs
	for range 4 {
		scene.gameState.EventQueueManager.Update()
	}
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow(combatAutoAttackWorkflowName))
	assert.False(t, scene.inputBlocked)
}

func configureDoomMirrorInputActor(t *testing.T, gs *game_state.GameState) game_state.CombatTurnIdentity {
	t.Helper()
	actor := gs.CombatScene.GetCurrentTurnIdentity()
	gs.CombatScene.DungeonRoomOrigin = &game_state.DungeonRoomEncounterOrigin{
		Location:     references.Doom,
		Floor:        7,
		Position:     references.Position{X: 5, Y: 7},
		DungeonIndex: references.DoomDungeon,
		Room:         15,
		Live:         true,
	}
	gs.CombatScene.CombatMapRef.MapIndex = references.CalculateDungeonCombatMapIndex(references.DoomDungeon, 15)
	gs.CombatScene.CombatMapRef.Territory = references.DungeonCombatTerritory
	gs.CombatScene.CombatMapRef.SetTileNumber(references.Position{X: 5, Y: 1}, indexes.Mirror)
	gs.CombatScene.SetPartyMemberPosition(actor.EntityIndex, references.Position{X: 5, Y: 2})

	return actor
}

func TestCombatNumberSelectionDoesNotTriggerDoomMirrorAbsorption(t *testing.T) {
	gs, callbacks := newBritainCombatTestState(t, 1)
	actor := configureDoomMirrorInputActor(t, gs)
	selected := -1
	for index, active := range gs.CombatScene.PartyActive {
		if active && index != actor.EntityIndex {
			selected = index

			break
		}
	}
	if selected < 0 {
		t.Skip("selection regression requires two active party members")
	}

	gs.BeginCombatTurns(game_state.CombatTurnsPresented)
	callbacks.Reset()
	scene := &GameScene{gameState: gs}
	key := ebiten.Key(int(ebiten.Key1) + selected)

	require.True(t, scene.handleActivePlayerSelection(key))

	assert.True(t, gs.CombatScene.IsPartyMemberActive(actor.EntityIndex))
	entry, ok := gs.CombatScene.TurnQueue.GetEntrySnapshot(actor)
	require.True(t, ok)
	assert.Equal(t, game_state.CombatTurnActive, entry.Status)
	assert.Empty(t, callbacks.CombatConclusions)
}

func TestRejectedAndModalCombatCommandsDoNotTriggerDoomMirrorImmediately(t *testing.T) {
	tests := []struct {
		name string
		key  ebiten.Key
	}{
		{name: "mix rejected", key: ebiten.KeyM},
		{name: "camp rejected", key: ebiten.KeyH},
		{name: "ignite rejected", key: ebiten.KeyI},
		{name: "view rejected", key: ebiten.KeyV},
		{name: "ready waits for dialog", key: ebiten.KeyR},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gs, callbacks := newBritainCombatTestState(t, 1)
			gs.BeginCombatTurns(game_state.CombatTurnsPresented)
			actor := configureDoomMirrorInputActor(t, gs)
			callbacks.Reset()
			scene := &GameScene{
				gameState: gs,
				keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
				output:    text.NewOutput(nil, 1, 10, 80),
			}

			scene.combatMapInputHandler(test.key)

			assert.True(t, gs.CombatScene.IsPartyMemberActive(actor.EntityIndex))
			entry, ok := gs.CombatScene.TurnQueue.GetEntrySnapshot(actor)
			require.True(t, ok)
			assert.Equal(t, game_state.CombatTurnActive, entry.Status)
			assert.Empty(t, callbacks.CombatConclusions)
		})
	}
}

type blockedCombatMonsterSnapshot struct {
	Position references.Position
	HP       int
	Active   bool
	Dead     bool
}

type blockedCombatStateSnapshot struct {
	Identity       game_state.CombatTurnIdentity
	ActorQueue     game_state.CombatTurnForecastEntry
	DateTime       datetime.UltimaDate
	ElapsedMs      int64
	RNGCalls       uint64
	Inventory      party_state.InventoryLockerSnapshot
	Spells         party_state.SpellInventorySnapshot
	Tiles          [3][references.YCombatMapTiles][references.XCombatMapTiles]indexes.SpriteIndex
	PartyPositions [party_state.MaxActiveParty]references.Position
	PartyActive    [party_state.MaxActiveParty]bool
	PartyInvisible [party_state.MaxActiveParty]bool
	PartySleeping  [party_state.MaxActiveParty]bool
	PartyPossessed [party_state.MaxActiveParty]bool
	Monsters       []blockedCombatMonsterSnapshot
	Fields         []game_state.CombatField
	ObjectCount    int
	ExitRoute      game_state.CombatExitRoute
	Victory        bool
}

func snapshotBlockedCombatState(t *testing.T, state *game_state.GameState) blockedCombatStateSnapshot {
	t.Helper()
	combat := state.CombatScene
	require.NotNil(t, combat)
	identity := combat.GetCurrentTurnIdentity()
	queue, found := combat.TurnQueue.GetEntrySnapshot(identity)
	require.True(t, found)
	snapshot := blockedCombatStateSnapshot{
		Identity:       identity,
		ActorQueue:     queue,
		DateTime:       state.DateTime,
		ElapsedMs:      state.ElapsedMs,
		RNGCalls:       state.GetRNG().Calls(),
		Inventory:      state.PartyState.Inventory.LockerSnapshot(),
		Spells:         state.PartyState.Inventory.SpellSnapshot(),
		PartyActive:    combat.PartyActive,
		PartyInvisible: combat.PartyInvisible,
		PartySleeping:  combat.PartySleeping,
		PartyPossessed: combat.PartyPossessed,
		Fields:         append([]game_state.CombatField(nil), combat.Fields()...),
		ObjectCount:    combat.ObjectCount(),
		ExitRoute:      combat.ExitRoute,
		Victory:        combat.Victory,
	}
	for index := range party_state.MaxActiveParty {
		snapshot.PartyPositions[index] = combat.GetPartyMemberPosition(index)
	}
	layeredMap := state.GetCurrentLayeredMap()
	for layer := map_state.MapLayer; layer <= map_state.MapUnitLayer; layer++ {
		for y := range references.YCombatMapTiles {
			for x := range references.XCombatMapTiles {
				position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
				snapshot.Tiles[int(layer)][y][x] = layeredMap.SpriteIndexByLayer(layer, position)
			}
		}
	}
	combat.MonsterManager.ForEach(func(_ int, monster *game_state.CombatMonster) {
		snapshot.Monsters = append(snapshot.Monsters, blockedCombatMonsterSnapshot{
			Position: monster.Position(),
			HP:       monster.CurrentHP(),
			Active:   monster.IsActive(),
			Dead:     monster.IsDead(),
		})
	})

	return snapshot
}

func TestBlockedCombatCommandMatrixRepromptsWithoutMutation(t *testing.T) {
	tests := []struct {
		key      ebiten.Key
		feedback string
	}{
		{key: ebiten.KeyB, feedback: "Board what?"},
		{key: ebiten.KeyD, feedback: "D-What?"},
		{key: ebiten.KeyE, feedback: "Enter-Not here"},
		{key: ebiten.KeyF, feedback: "Fire-Not here"},
		{key: ebiten.KeyH, feedback: "Hole up-Not here"},
		{key: ebiten.KeyI, feedback: "Ignite torch-Not here"},
		{key: ebiten.KeyL, feedback: "Look-Not here"},
		{key: ebiten.KeyM, feedback: "Mix-Not here"},
		{key: ebiten.KeyN, feedback: "New order-Not here"},
		{key: ebiten.KeyQ, feedback: "Quit-Not here"},
		{key: ebiten.KeyT, feedback: "Talk-Funny, no response!"},
		{key: ebiten.KeyV, feedback: "View-Not here"},
		{key: ebiten.KeyW, feedback: "W-What?"},
		{key: ebiten.KeyX, feedback: "X-it what?"},
	}

	for _, test := range tests {
		t.Run(test.feedback, func(t *testing.T) {
			scene, callbacks := newCombatUseTestScene(t)
			scene.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
			callbacks.Reset()
			before := snapshotBlockedCombatState(t, scene.gameState)

			scene.combatMapInputHandler(test.key)

			assert.Equal(t, test.feedback, callbacks.CurrentRowContent)
			assert.Equal(t, []string{""}, callbacks.CommandPrompts)
			assert.Empty(t, callbacks.Messages)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
			assert.Empty(t, callbacks.TimeAdvanced())
			assert.Empty(t, callbacks.SoundEffectsPlayed)
			assert.Empty(t, callbacks.CombatEvents)
			assert.Empty(t, callbacks.CombatConclusions)
			assert.Zero(t, callbacks.FlowCallCount)
			assert.Zero(t, callbacks.VisualCallCount)
			assert.False(t, callbacks.ShowGemViewInteractionCalled)
			assert.False(t, callbacks.ReadyDialogShown)
		})
	}
}

func TestUnknownCombatKeyUsesGenericInnerReprompt(t *testing.T) {
	scene, callbacks := newCombatUseTestScene(t)
	scene.gameState.BeginCombatTurns(game_state.CombatTurnsPresented)
	callbacks.Reset()
	before := snapshotBlockedCombatState(t, scene.gameState)

	scene.combatMapInputHandler(ebiten.KeyBackquote)

	assert.Equal(t, "What?", callbacks.CurrentRowContent)
	assert.Equal(t, []string{""}, callbacks.CommandPrompts)
	assert.Equal(t, before, snapshotBlockedCombatState(t, scene.gameState))
}

func setupControlledMonsterInputTest(
	t *testing.T,
) (*game_state.GameState, *game_state.MockSystemCallbacks, int) {
	t.Helper()

	gs, mock := newBritainCombatTestState(t, 1)
	for index, active := range gs.CombatScene.PartyActive {
		if active {
			gs.CombatScene.TurnQueue.MarkFled(index, true)
		}
	}
	enemyReferences := *gs.GameReferences.EnemyReferences
	daemon := game_state.NewCombatMonster(
		references.MonsterDevil,
		&enemyReferences[references.MonsterDevil],
		references.Position{X: 5, Y: 5},
		gs.AsRandomProvider(),
	)
	daemon.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
	daemonIndex := gs.CombatScene.AddMonsterToQueue(daemon)
	gs.BeginCombatTurns(game_state.CombatTurnsImmediate)
	require.Equal(t, daemonIndex, gs.CombatScene.ActiveMonsterIndex)
	mock.Reset()

	return gs, mock, daemonIndex
}

func TestControlledMonsterRejectsPCOnlyCommandWithoutConsumingTurn(t *testing.T) {
	gs, mock, daemonIndex := setupControlledMonsterInputTest(t)
	scene := &GameScene{gameState: gs}
	identityBefore := gs.CombatScene.GetCurrentTurnIdentity()
	timeBefore := gs.DateTime

	assert.True(t, scene.handleControlledMonsterCombatInput(ebiten.KeyC))

	assert.Contains(t, mock.Messages, "Can't!")
	assert.Equal(t, identityBefore, gs.CombatScene.GetCurrentTurnIdentity())
	assert.Equal(t, daemonIndex, gs.CombatScene.ActiveMonsterIndex)
	assert.Equal(t, timeBefore, gs.DateTime)
}

func TestControlledMonsterUsesBlockedCombatCommandMatrix(t *testing.T) {
	gs, callbacks, _ := setupControlledMonsterInputTest(t)
	scene := &GameScene{gameState: gs}
	before := snapshotBlockedCombatState(t, gs)

	scene.combatMapInputHandler(ebiten.KeyV)

	assert.Equal(t, "View-Not here", callbacks.CurrentRowContent)
	assert.Equal(t, []string{""}, callbacks.CommandPrompts)
	assert.Empty(t, callbacks.Messages)
	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, before, snapshotBlockedCombatState(t, gs))
	assert.Empty(t, callbacks.TimeAdvanced())
	assert.Empty(t, callbacks.CombatEvents)
	assert.Empty(t, callbacks.CombatConclusions)
}

func TestControlledMonsterCombatPushUsesProductionDirectionAndCompletesOnce(t *testing.T) {
	actualState, actualCallbacks, _ := setupControlledMonsterInputTest(t)
	expectedState, expectedCallbacks, _ := setupControlledMonsterInputTest(t)
	if actualState == nil || expectedState == nil {
		return
	}
	actualScene := &GameScene{
		gameState: actualState,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 10, 80),
	}
	configureProductionCombatPush(t, actualState, indexes.Barrel, indexes.BrickFloor)
	configureProductionCombatPush(t, expectedState, indexes.Barrel, indexes.BrickFloor)
	actualCallbacks.Reset()
	expectedCallbacks.Reset()

	actualScene.combatMapInputHandler(ebiten.KeyP)
	require.Equal(t, PushDirectionInput, actualScene.secondaryKeyState)
	actualScene.keyboard.SimulateKeyPress(ebiten.KeyRight)
	actualScene.combatMapHandleSecondaryInput()

	require.True(t, expectedState.ActionPushCombatMap(references.Right))
	expectedState.FinishCombatAction()
	assertProductionCombatPushCompletion(t, actualScene, expectedState, actualCallbacks, expectedCallbacks)
}

func TestCombatPushProductionDirectionOutcomesCompleteExactlyOnce(t *testing.T) {
	tests := []struct {
		name         string
		object       indexes.SpriteIndex
		forwardFloor indexes.SpriteIndex
	}{
		{name: "push", object: indexes.Barrel, forwardFloor: indexes.BrickFloor},
		{name: "pull", object: indexes.Barrel, forwardFloor: indexes.StoneBrickWall},
		{name: "failure", object: indexes.TableMiddle, forwardFloor: indexes.BrickFloor},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actualScene, actualCallbacks := newStartedCombatPushScene(t)
			expectedScene, expectedCallbacks := newStartedCombatPushScene(t)
			configureProductionCombatPush(t, actualScene.gameState, test.object, test.forwardFloor)
			configureProductionCombatPush(t, expectedScene.gameState, test.object, test.forwardFloor)
			actualCallbacks.Reset()
			expectedCallbacks.Reset()

			actualScene.secondaryKeyState = PushDirectionInput
			actualScene.keyboard.SimulateKeyPress(ebiten.KeyRight)
			actualScene.combatMapHandleSecondaryInput()

			expectedScene.gameState.ActionPushCombatMap(references.Right)
			expectedScene.gameState.FinishCombatAction()
			assertProductionCombatPushCompletion(
				t, actualScene, expectedScene.gameState, actualCallbacks, expectedCallbacks,
			)
		})
	}
}

func TestCombatPushProductionPassCompletesExactlyOnce(t *testing.T) {
	actualScene, actualCallbacks := newStartedCombatPushScene(t)
	expectedScene, expectedCallbacks := newStartedCombatPushScene(t)
	actualCallbacks.Reset()
	expectedCallbacks.Reset()

	actualScene.secondaryKeyState = PushDirectionInput
	actualScene.keyboard.SimulateKeyPress(ebiten.KeySpace)
	actualScene.combatMapHandleSecondaryInput()
	expectedScene.gameState.FinishCombatAction()

	assertProductionCombatPushCompletion(
		t, actualScene, expectedScene.gameState, actualCallbacks, expectedCallbacks,
	)
	assert.Contains(t, actualScene.output.GetOutputStr(true), "Pass")
}

func TestCombatPushProductionTrapCompletesExactlyOnceWithoutPushFeedback(t *testing.T) {
	actualScene, actualCallbacks, actualDirection := newStartedDungeonCombatPushTrapScene(t)
	expectedScene, expectedCallbacks, expectedDirection := newStartedDungeonCombatPushTrapScene(t)
	require.Equal(t, expectedDirection, actualDirection)
	actualCallbacks.Reset()
	expectedCallbacks.Reset()

	actualScene.secondaryKeyState = PushDirectionInput
	simulateCombatDirectionKey(t, actualScene, actualDirection)
	actualScene.combatMapHandleSecondaryInput()

	require.True(t, expectedScene.gameState.ActionPushCombatMap(expectedDirection))
	expectedScene.gameState.FinishCombatAction()
	assertProductionCombatPushCompletion(
		t, actualScene, expectedScene.gameState, actualCallbacks, expectedCallbacks,
	)
	assert.Equal(t, expectedCallbacks.DisplayUpdates, actualCallbacks.DisplayUpdates)
	assert.Equal(t, 1, actualCallbacks.DisplayUpdates)
	for _, message := range actualCallbacks.Messages {
		assert.NotContains(t, []string{"Pushed!", "Pulled!", "Won't budge!", "Won't budge"}, message)
	}
}

func TestCombatPushFailureHasOneProductionFeedbackOwner(t *testing.T) {
	scene, callbacks := newStartedCombatPushScene(t)
	configureProductionCombatPush(t, scene.gameState, indexes.TableMiddle, indexes.BrickFloor)
	originalAddRow := scene.gameState.SystemCallbacks.Message.AddRowStr
	scene.gameState.SystemCallbacks.Message.AddRowStr = func(message string) {
		originalAddRow(message)
		scene.addRowStr(message)
	}
	callbacks.Reset()

	scene.combatMapPushSecondary(references.Right)

	assert.Equal(t, []string{"Won't budge!"}, callbacks.Messages)
	assert.Equal(t, 1, strings.Count(scene.output.GetOutputStr(true), "Won't budge!"))
}

func newStartedCombatPushScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	scene, callbacks := newCombatUseTestScene(t)
	scene.gameState.BeginCombatTurns(game_state.CombatTurnsImmediate)
	callbacks.Reset()

	return scene, callbacks
}

func newStartedDungeonCombatPushTrapScene(
	t *testing.T,
) (*GameScene, *game_state.MockSystemCallbacks, references.Direction) {
	t.Helper()
	state, callbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(295).
		Build()
	require.NoError(t, state.TransitionToCombatMap(
		0, references.DungeonCombatTerritory, game_state.CombatEntryNorth,
	))
	state.BeginCombatTurns(game_state.CombatTurnsImmediate)
	identity := state.CombatScene.GetCurrentTurnIdentity()
	var direction references.Direction
	var trigger references.Position
	slots, present := state.CombatScene.CombatMapRef.DungeonTriggerSlots()
	require.True(t, present)
	for _, slot := range slots {
		if slot.Trigger.X < 0 || slot.Trigger.X >= references.XCombatMapTiles ||
			slot.Trigger.Y < 0 || slot.Trigger.Y >= references.YCombatMapTiles {
			continue
		}
		for _, candidate := range []references.Direction{references.Right, references.Down, references.Left, references.Up} {
			actor := candidate.GetOppositeDirection().GetNewPositionInDirection(&slot.Trigger)
			if actor != nil && actor.X >= 0 && actor.X < references.XCombatMapTiles &&
				actor.Y >= 0 && actor.Y < references.YCombatMapTiles {
				direction = candidate
				trigger = slot.Trigger
				state.CombatScene.SetPartyMemberPosition(identity.EntityIndex, *actor)

				break
			}
		}
		if direction != references.NoneDirection {
			break
		}
	}
	require.NotEqual(t, references.NoneDirection, direction, "DUNGEON.CBT map 0 must expose an aimable trigger")
	setProductionCombatPushTerrain(state, trigger, indexes.Barrel)
	callbacks.Reset()

	return &GameScene{
		gameState: state,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:    text.NewOutput(nil, 1, 10, 80),
	}, callbacks, direction
}

func simulateCombatDirectionKey(t *testing.T, scene *GameScene, direction references.Direction) {
	t.Helper()
	switch direction {
	case references.Up:
		scene.keyboard.SimulateKeyPress(ebiten.KeyUp)
	case references.Right:
		scene.keyboard.SimulateKeyPress(ebiten.KeyRight)
	case references.Down:
		scene.keyboard.SimulateKeyPress(ebiten.KeyDown)
	case references.Left:
		scene.keyboard.SimulateKeyPress(ebiten.KeyLeft)
	case references.NoneDirection:
		t.Fatal("cannot simulate no direction")
	default:
		t.Fatal("unsupported combat direction")
	}
}

func configureProductionCombatPush(
	t *testing.T,
	state *game_state.GameState,
	object indexes.SpriteIndex,
	forwardFloor indexes.SpriteIndex,
) {
	t.Helper()
	identity := state.CombatScene.GetCurrentTurnIdentity()
	actor := state.CombatScene.GetCombatantPosition(identity)
	if !identity.IsPartyMember {
		monster, _ := state.CombatScene.GetControlledMonster()
		require.NotNil(t, monster)
		actor = monster.Position()
	}
	target := references.Right.GetNewPositionInDirection(&actor)
	require.NotNil(t, target)
	forward := references.Right.GetNewPositionInDirection(target)
	require.NotNil(t, forward)
	require.Less(t, forward.X, references.XCombatMapTiles)
	setProductionCombatPushTerrain(state, actor, indexes.BrickFloor)
	setProductionCombatPushTerrain(state, *target, object)
	setProductionCombatPushTerrain(state, *forward, forwardFloor)
}

func setProductionCombatPushTerrain(
	state *game_state.GameState,
	position references.Position,
	tile indexes.SpriteIndex,
) {
	state.CombatScene.CombatMapRef.SetTileNumber(position, tile)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, &position, tile)
}

func assertProductionCombatPushCompletion(
	t *testing.T,
	actualScene *GameScene,
	expectedState *game_state.GameState,
	actualCallbacks *game_state.MockSystemCallbacks,
	expectedCallbacks *game_state.MockSystemCallbacks,
) {
	t.Helper()
	actualState := actualScene.gameState
	assert.Equal(t, PrimaryInput, actualScene.secondaryKeyState)
	assert.Equal(t, expectedState.CombatScene.GetCurrentTurnIdentity(), actualState.CombatScene.GetCurrentTurnIdentity())
	assert.Equal(t, expectedState.DateTime, actualState.DateTime)
	assert.Equal(t, expectedState.GetRNG().Calls(), actualState.GetRNG().Calls())
	assert.Equal(t, expectedCallbacks.Messages, actualCallbacks.Messages)
	assert.Equal(t, expectedCallbacks.CombatEvents, actualCallbacks.CombatEvents)
	for y := range references.YCombatMapTiles {
		for x := range references.XCombatMapTiles {
			position := references.Position{X: references.Coordinate(x), Y: references.Coordinate(y)}
			assert.Equal(t,
				expectedState.CombatScene.CombatMapRef.GetTileNumber(position),
				actualState.CombatScene.CombatMapRef.GetTileNumber(position),
			)
		}
	}
}

func TestControlledMonsterProjectileUsesExistingVisualWorkflow(t *testing.T) {
	gs, mock, daemonIndex := setupControlledMonsterInputTest(t)
	enemyReferences := *gs.GameReferences.EnemyReferences
	targetPosition := references.Position{X: 7, Y: 5}
	target := game_state.NewCombatMonster(
		references.MonsterOrc,
		&enemyReferences[references.MonsterOrc],
		targetPosition,
		gs.AsRandomProvider(),
	)
	targetIndex := gs.CombatScene.AddMonsterToQueue(target)
	require.True(t, gs.BeginCombatAttack())
	gs.CombatScene.AttackTargeting.Cursor = targetPosition
	scene := &GameScene{
		gameState:         gs,
		secondaryKeyState: CombatAttackTargetInput,
	}

	result := gs.ConfirmCombatAttackTarget()
	require.Equal(t, game_state.CombatAttackSourcePending, result.Outcome)
	require.NotNil(t, result.Launch)
	scene.startCombatProjectileAttack(result)
	gs.EventQueueManager.Update()
	require.Len(t, mock.ProjectilesStarted, 1)
	projectile := mock.ProjectilesStarted[0]
	assert.Equal(t, references.MissileRed, projectile.ProjectileType)
	assert.Equal(t, references.Position{X: 5, Y: 5}, projectile.FromPos)
	assert.Equal(t, targetPosition, projectile.ToPos)
	assert.Empty(t, mock.CombatEvents, "damage resolves only after projectile travel")

	gs.ElapsedMs += int64(projectile.DurationMs)
	gs.EventQueueManager.Update()
	gs.EventQueueManager.Update()
	require.Len(t, mock.CombatEvents, 1)
	assert.Equal(t, daemonIndex, mock.CombatEvents[0].Attacker.Index)
	assert.Equal(t, targetIndex, mock.CombatEvents[0].Target.Index)
	assert.GreaterOrEqual(t, mock.ProjectilesClearedCount, 1)
}

// TestCombatMapAttackSecondary_ConsumesTurn prevents attacks from leaving the
// same PC active indefinitely. Original combat input completes the action (ref:COMBAT-0003)
// after Attack, then the turn manager selects the next combatant.
// TestFinishCombatAttackTargetAction_FinalizesTurn prevents completed targeted
// attacks from advancing the queue without processing combat time and effects.
func TestFinishCombatAttackTargetAction_FinalizesTurn(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	queue := game_state.NewCombatTurnQueue()
	queue.AddCombatant(fastCombatant{}, 0)
	for range 10 {
		require.NotNil(t, queue.AdvanceToNextActor())
	}
	gs.CombatScene.TurnQueue = queue
	gs.DateTime.Minute = 0

	gameScene := &GameScene{
		gameState:         gs,
		secondaryKeyState: CombatAttackTargetInput,
	}
	gameScene.finishCombatAttackTargetAction(game_state.CombatAttackTargetResult{
		Outcome: game_state.CombatAttackSequenceComplete,
	})

	assert.Equal(t, PrimaryInput, gameScene.secondaryKeyState)
	assert.Equal(t, byte(1), gs.DateTime.Minute,
		"the tenth completed combat action must advance game time")
}

// TestCombatChestRevealWorkflowPacesRightPanelMessages prevents chest contents
// from appearing in one burst before the player can read the original-style list.
func TestCombatChestRevealWorkflowPacesRightPanelMessages(t *testing.T) {
	gs, mockCallbacks := newBritainCombatTestState(t, 42)
	activeBefore := gs.CombatScene.GetActiveCombatantIndex()
	mockCallbacks.Reset()
	scene := &GameScene{gameState: gs, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), spriteSheet: sharedPresentationTestSpriteSheet()}

	token := gs.FinishCombatTurnDeferred()
	scene.startCombatChestRevealWorkflow(game_state.CombatOpenResult{RevealItems: []game_state.CombatLootReveal{
		{Sprite: indexes.ItemGem, Description: "a gem!"}, {Sprite: indexes.ItemWeapon, Description: "a weapon!"},
	}}, token)
	require.True(t, mockCallbacks.InputBlocked)

	gs.EventQueueManager.Update()
	assert.Equal(t, []string{"Found:"}, mockCallbacks.Messages)
	assert.Equal(t, activeBefore, gs.CombatScene.GetActiveCombatantIndex())

	gs.ElapsedMs += combatChestRevealDelayMs - 1
	gs.EventQueueManager.Update()
	assert.Equal(t, []string{"Found:"}, mockCallbacks.Messages)

	gs.ElapsedMs++
	gs.EventQueueManager.Update()
	assert.Equal(t, []string{"Found:", "a gem!"}, mockCallbacks.Messages)
	assert.True(t, gs.CombatScene.HasScheduledCombatStep())

	gs.ElapsedMs += combatChestRevealDelayMs
	gs.EventQueueManager.Update()
	require.GreaterOrEqual(t, len(mockCallbacks.Messages), 3)
	assert.Equal(t, []string{"Found:", "a gem!", "a weapon!"}, mockCallbacks.Messages[:3])
	assert.True(t, gs.CombatScene.HasScheduledCombatStep(), "acknowledgement retains the action")
	dialog := requireEscapeMenuDialog[*widgets.CombatLootDialog](t, scene)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	gs.ElapsedMs += DialogPollingRateMs
	gs.EventQueueManager.Update()
	assert.False(t, gs.CombatScene.HasScheduledCombatStep())
	assert.True(t, mockCallbacks.InputBlocked)
	gs.EventQueueManager.Update()
	assert.False(t, mockCallbacks.InputBlocked)
}

func TestCombatAutoPassSchedulerBlocksInputUntilNonActionablePCTurnIsShown(t *testing.T) {
	gs, mockCallbacks := newBritainCombatTestState(t, 42)
	// The unblock gate queries held keys; never inherit live desktop input.
	scene := &GameScene{gameState: gs, keyboard: input.NewKeyboardWithKeyStateSource(-1, func(ebiten.Key) bool { return false })}
	gs.ElapsedMs = 0
	step := nextNonActionablePCPresentationStep(t, gs)

	scene.startScheduledCombatAutoPass(step)
	require.True(t, mockCallbacks.InputBlocked)
	gs.EventQueueManager.Update()
	assert.True(t, gs.CombatScene.HasScheduledCombatStep())

	duration := scene.captureCombatTiming().duration(combatAutoPassPCDurationMs)
	require.Positive(t, duration)
	gs.ElapsedMs += int64(duration - 1)
	gs.EventQueueManager.Update()
	assert.True(t, gs.CombatScene.HasScheduledCombatStep())

	gs.ElapsedMs++
	gs.EventQueueManager.Update()
	gs.EventQueueManager.Update()
	assert.ErrorIs(t, gs.CompleteScheduledCombatStep(step.Token), game_state.ErrCombatPresentationDuplicate)
	assert.False(t, mockCallbacks.InputBlocked)
}

func TestCombatProjectileSequencePreservesPhysicalProjectileIdentity(t *testing.T) {
	for _, missile := range []references.MissileType{
		references.MissileArrow,
		references.MissileAxe,
		references.MissileRock,
	} {
		t.Run(missile.GetStringName(), func(t *testing.T) {
			gs, mock := newBritainCombatTestState(t, 42)
			scene := &GameScene{gameState: gs}
			launch := game_state.CombatProjectileLaunch{
				From: references.Position{X: 2, Y: 5}, Aim: references.Position{X: 6, Y: 5},
				Impact: references.Position{X: 6, Y: 5}, Missile: missile, Animate: true,
			}
			impactCalled := false
			mock.Reset()
			gs.ElapsedMs = 0

			gs.EventQueueManager.Workflow("test_physical_projectile_identity").
				Priority(event_queue.PriorityHigh).
				Sequential().
				First(func() {}).
				ThenSequence(scene.combatProjectileAnimationSequenceWithTiming(scene.captureCombatTiming(), launch, func() {
					impactCalled = true
				})).
				Finally(func() {}).
				Start()
			gs.EventQueueManager.Update()
			for range 5 {
				gs.EventQueueManager.Update()
				if len(mock.ProjectilesStarted) > 0 {
					break
				}
			}

			require.Len(t, mock.ProjectilesStarted, 1)
			assert.Equal(t, missile, mock.ProjectilesStarted[0].ProjectileType)
			assert.Equal(t, launch.From, mock.ProjectilesStarted[0].FromPos)
			assert.Equal(t, launch.Impact, mock.ProjectilesStarted[0].ToPos)

			gs.ElapsedMs += int64(mock.ProjectilesStarted[0].DurationMs)
			for range 6 {
				gs.EventQueueManager.Update()
				if impactCalled {
					break
				}
			}
			assert.True(t, impactCalled)
		})
	}
}

func TestPlayerCombatMovementCompletesExactScheduledTurn(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	scene := &GameScene{gameState: gs}
	actor := gs.CombatScene.GetCurrentTurnIdentity()
	from := gs.CombatScene.GetCombatantPosition(actor)
	to := references.Position{X: from.X + 1, Y: from.Y}
	gs.ElapsedMs = 0
	token := gs.FinishCombatTurnDeferred()

	scene.startPlayerCombatMovementWorkflow(actor, from, to, token)
	gs.EventQueueManager.Update()
	require.True(t, scene.combatMovement.Active)
	assert.True(t, gs.CombatScene.HasScheduledCombatStep())

	gs.ElapsedMs += int64(scene.captureCombatTiming().duration(combatPlayerMovementDurationMs))
	for range 5 {
		gs.EventQueueManager.Update()
		if !gs.CombatScene.HasScheduledCombatStep() {
			break
		}
	}
	assert.False(t, gs.CombatScene.HasScheduledCombatStep())
	assert.False(t, scene.combatMovement.Active)
}

func TestScheduledMonsterTurnDoesNotAnimateTeleportAsWalking(t *testing.T) {
	from := references.Position{X: 2, Y: 2}
	to := references.Position{X: from.X + 2, Y: from.Y + 2}

	assert.False(t, shouldAnimateScheduledMonsterMovement(
		game_state.MonsterTurnExecutionResult{Moved: true, Teleported: true}, from, to,
	))
}

// TestCancelInitialCombatAttack_PreservesTurn prevents Escape from treating an
// untouched Attack command as the active PC's completed action.
func TestCancelInitialCombatAttack_PreservesTurn(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	require.True(t, gs.BeginCombatAttack())
	activeBefore := gs.CombatScene.GetActiveCombatantIndex()
	scene := &GameScene{
		gameState:         gs,
		secondaryKeyState: CombatAttackTargetInput,
	}

	result := gs.CancelInitialCombatAttack()
	scene.finishCombatAttackTargetAction(result)

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Nil(t, gs.CombatScene.AttackTargeting)
	assert.Equal(t, activeBefore, gs.CombatScene.GetActiveCombatantIndex())
}

func TestFinishCombatSpellTargetAction_CancelPreservesAction(t *testing.T) {
	gs, _ := newBritainCombatTestState(t, 42)
	activeBefore := gs.CombatScene.GetActiveCombatantIndex()
	scene := &GameScene{
		gameState:         gs,
		secondaryKeyState: CombatSpellTargetInput,
	}

	scene.finishCombatSpellTargetAction(game_state.CombatSpellTargetResult{
		Outcome: game_state.CombatSpellTargetCancelled,
	})

	assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
	assert.Equal(t, activeBefore, gs.CombatScene.GetActiveCombatantIndex())
}

func TestFinishCombatSpellTargetAction_InvalidKeepsTargetingInput(t *testing.T) {
	gs, _ := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	scene := &GameScene{
		gameState:         gs,
		secondaryKeyState: CombatSpellTargetInput,
	}

	scene.finishCombatSpellTargetAction(game_state.CombatSpellTargetResult{
		Outcome: game_state.CombatSpellTargetInvalid,
	})

	assert.Equal(t, CombatSpellTargetInput, scene.secondaryKeyState)
}

func TestCombatAdjacentSpellConcealedTargetUsesNeutralConfirmationPrompt(t *testing.T) {
	state, mock := newProductionCombatCastState(t)
	casterIdx := state.CombatScene.GetActiveCombatantIndex()
	state.PartyState.Inventory.Spells.Set(references.AnGrav, 1)
	origin := state.CombatScene.GetPartyMemberPosition(casterIdx)
	target := references.Down.GetNewPositionInDirection(&origin)
	require.NotNil(t, target)
	require.True(t, state.CombatScene.AddField(game_state.NewCombatField(game_state.CombatFieldSleep, *target)))
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapLayer, target, indexes.BlackSquare)
	state.GetCurrentLayeredMap().SetTileByLayer(map_state.MapOverrideLayer, target, indexes.NoSprites)
	require.True(t, state.BeginCombatAdjacentSpellTargeting(game_state.CastRequest{
		Spell: references.AnGrav, CasterIdx: casterIdx,
	}).Valid)
	mock.Reset()
	scene := &GameScene{
		gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), secondaryKeyState: CombatAdjacentSpellTargetInput,
	}
	scene.keyboard.SimulateKeyPress(ebiten.KeyDown)

	scene.handleCombatAdjacentSpellTargetInput()

	require.NotEmpty(t, mock.CommandPrompts)
	prompt := mock.CommandPrompts[len(mock.CommandPrompts)-1]
	assert.Contains(t, prompt, "Enter: cast")
	assert.NotContains(t, prompt, "invalid target")
}

func TestCombatSpellTargetCancellationRoutesPreserveActionAndResources(t *testing.T) {
	tests := []struct {
		name      string
		spell     references.Spell
		begin     func(*game_state.GameState, game_state.CastRequest) game_state.SpellValidationResult
		input     InputState
		handleEsc func(*GameScene)
	}{
		{
			name: "combatant crosshair", spell: references.VasFlam,
			begin: func(state *game_state.GameState, request game_state.CastRequest) game_state.SpellValidationResult {
				return state.BeginCombatSpellTargeting(request)
			},
			input: CombatSpellTargetInput, handleEsc: (*GameScene).handleCombatSpellTargetInput,
		},
		{
			name: "field tile crosshair", spell: references.InFlamGrav,
			begin: func(state *game_state.GameState, request game_state.CastRequest) game_state.SpellValidationResult {
				return state.BeginCombatSpellTargeting(request)
			},
			input: CombatSpellTargetInput, handleEsc: (*GameScene).handleCombatSpellTargetInput,
		},
		{
			name: "adjacent", spell: references.AnYlem,
			begin: func(state *game_state.GameState, request game_state.CastRequest) game_state.SpellValidationResult {
				return state.BeginCombatAdjacentSpellTargeting(request)
			},
			input: CombatAdjacentSpellTargetInput, handleEsc: (*GameScene).handleCombatAdjacentSpellTargetInput,
		},
		{
			name: "cone", spell: references.InZu,
			begin: func(state *game_state.GameState, request game_state.CastRequest) game_state.SpellValidationResult {
				return state.BeginCombatConeTargeting(request)
			},
			input: CombatConeTargetInput, handleEsc: (*GameScene).handleCombatConeTargetInput,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actual, actualMock := newProductionCombatCastState(t)
			expected, _ := newProductionCombatCastState(t)
			casterIdx := actual.CombatScene.GetActiveCombatantIndex()
			actual.PartyState.Inventory.Spells.Set(test.spell, 1)
			expected.PartyState.Inventory.Spells.Set(test.spell, 1)
			request := game_state.CastRequest{Spell: test.spell, CasterIdx: casterIdx}
			require.True(t, test.begin(actual, request).Valid)
			caster := actual.PartyState.GetCharacter(casterIdx)
			mpBefore := caster.GetCurrentMp()
			actualMock.Reset()
			scene := &GameScene{
				gameState: actual, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), secondaryKeyState: test.input,
			}

			scene.keyboard.SimulateKeyPress(ebiten.KeyEscape)
			test.handleEsc(scene)

			assertCombatMatchesSingleFinish(t, actual, expected)
			assert.Equal(t, PrimaryInput, scene.secondaryKeyState)
			assert.Equal(t, mpBefore, caster.GetCurrentMp())
			assert.Equal(t, uint16(1), actual.PartyState.Inventory.Spells.Get(test.spell))
			assert.Empty(t, actualMock.CombatEvents)
		})
	}
}

func TestCombatProjectileAnimationSequenceOwnsOnlyProjectileLifecycle(t *testing.T) {
	gs, mock := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(
		0,
		references.BritanniaCombatTerritory,
		game_state.CombatEntryNorth,
	))
	mock.Reset()
	scene := &GameScene{gameState: gs}
	impactCalled := false
	clearedAtImpact := false
	launch := game_state.CombatProjectileLaunch{
		From:    references.Position{X: 2, Y: 3},
		Aim:     references.Position{X: 5, Y: 3},
		Impact:  references.Position{X: 4, Y: 3},
		Missile: references.MissileGreen,
		Blocked: true,
		Animate: true,
	}

	gs.EventQueueManager.Workflow("test_combat_projectile_sequence").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.combatProjectileAnimationSequenceWithTiming(scene.captureCombatTiming(), launch, func() {
			impactCalled = true
			clearedAtImpact = mock.ProjectilesClearedCount > 0
		})).
		Finally(func() {}).
		Start()
	for range 5 {
		gs.EventQueueManager.Update()
		if len(mock.ProjectilesStarted) > 0 {
			break
		}
	}
	require.Len(t, mock.ProjectilesStarted, 1)
	projectile := mock.ProjectilesStarted[0]
	assert.Equal(t, launch.From, projectile.FromPos)
	assert.Equal(t, launch.Impact, projectile.ToPos)
	assert.Equal(t, launch.Missile, projectile.ProjectileType)
	assert.False(t, impactCalled)
	assert.Empty(t, mock.SoundEffectsPlayed)
	assert.Empty(t, mock.TimeAdvanced())
	assert.False(t, gs.CombatScene.HasScheduledCombatStep())

	gs.ElapsedMs += int64(projectile.DurationMs)
	for range 5 {
		gs.EventQueueManager.Update()
		if impactCalled {
			break
		}
	}
	assert.True(t, impactCalled)
	assert.True(t, clearedAtImpact)
	assert.NotEmpty(t, mock.ProjectileProgress)
	assert.Empty(t, mock.SoundEffectsPlayed)
	assert.Empty(t, mock.TimeAdvanced())
	assert.False(t, gs.CombatScene.HasScheduledCombatStep())
}

func TestCombatUtilitySpellTargetingUsesOriginalContextBehavior(t *testing.T) {
	assert.Equal(t,
		references.TargetAutomatic,
		references.GetSpellTargetingProfile(references.InPor, references.ContextCombat).Selection,
		"combat In Por chooses a random destination without a direction prompt",
	)
	assert.Equal(t,
		references.TargetDirection,
		references.GetSpellTargetingProfile(references.InPor, references.ContextOverworld).Selection,
		"overworld In Por retains directional targeting",
	)
	for _, spell := range []references.Spell{references.AnSanct, references.AnExPor, references.InExPor} {
		assert.Equal(t, references.TargetAdjacentTile,
			references.GetSpellTargetingProfile(spell, references.ContextCombat).Selection,
			references.GetSpellName(spell))
	}
}

func TestCombatWisQuasEffectProvidesAudioAndVisualFeedback(t *testing.T) {
	gs, mock := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	mock.Reset()
	scene := &GameScene{gameState: gs}

	gs.EventQueueManager.Workflow("test_wis_quas_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenCastCombatWisQuas()).
		Finally(func() {}).
		Start()
	for range 3 {
		gs.EventQueueManager.Update()
	}
	gs.ElapsedMs += combatRevealDurationMs / 2
	gs.EventQueueManager.Update()

	assert.True(t, mock.SoundEffectWasPlayed(game_state.SoundXRay))
	assert.NotEmpty(t, mock.ScreenFlashColors)
	require.NotEmpty(t, mock.ScreenFlashIntensities)
	assert.Greater(t, mock.ScreenFlashIntensities[len(mock.ScreenFlashIntensities)-1], 0.0)
}

func TestCombatEarthquakeEffectShakesScreenAndPlaysSound(t *testing.T) {
	gs, mock := newBritainCombatTestState(t, 42)
	mock.Reset()
	scene := &GameScene{gameState: gs}
	casterIdx := gs.CombatScene.GetActiveCombatantIndex()

	gs.EventQueueManager.Workflow("test_combat_earthquake_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenCastCombatInVasPorYlem(casterIdx)).
		Finally(func() {}).
		Start()
	for range 3 {
		gs.EventQueueManager.Update()
	}
	gs.ElapsedMs += combatEarthquakeDurationMs * 2 / 5
	gs.EventQueueManager.Update()

	assert.True(t, mock.SoundEffectWasPlayed(game_state.SoundEarthquake))
	require.NotEmpty(t, mock.ScreenShakeOffsets)
	shake := mock.ScreenShakeOffsets[len(mock.ScreenShakeOffsets)-1]
	assert.NotZero(t, shake.X)
	assert.NotZero(t, shake.Y)
}

func TestCombatSanctLorEffectUsesDoubleLightBluePulseAndLeavesShimmerState(t *testing.T) {
	gs, mock := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	mock.Reset()
	scene := &GameScene{gameState: gs}
	casterIdx := gs.CombatScene.GetActiveCombatantIndex()

	gs.EventQueueManager.Workflow("test_combat_sanct_lor_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenCastCombatSanctLor(casterIdx)).
		Finally(func() {}).
		Start()
	for range 3 {
		gs.EventQueueManager.Update()
	}
	for range 4 {
		gs.ElapsedMs += combatRelTymPulseMs
		gs.EventQueueManager.Update()
		gs.ElapsedMs += combatRelTymPulseGapMs
		gs.EventQueueManager.Update()
	}

	assert.True(t, gs.CombatScene.IsPartyMemberInvisible(casterIdx))
	assert.True(t, mock.SoundEffectWasPlayed(game_state.SoundMagic))
	require.Len(t, mock.ScreenFlashColors, 2)
	for _, flashColor := range mock.ScreenFlashColors {
		assert.Equal(t, game_state.ScreenFlashColorCall{
			R: combatLightBlueFlashRed, G: combatLightBlueFlashGreen, B: combatLightBlueFlashBlue,
		}, flashColor)
	}
	require.NotEmpty(t, mock.ScreenFlashIntensities)
	assert.Zero(t, mock.ScreenFlashIntensities[len(mock.ScreenFlashIntensities)-1])
}

func TestCombatRelTymEffectUsesTwoLightBluePulsesAndCleansUp(t *testing.T) {
	gs, mock := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	mock.Reset()
	scene := &GameScene{gameState: gs}

	gs.EventQueueManager.Workflow("test_combat_rel_tym_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenCastCombatRelTym()).
		Finally(func() {}).
		Start()
	for range 3 {
		gs.EventQueueManager.Update()
	}
	for range 4 {
		gs.ElapsedMs += combatRelTymPulseMs
		gs.EventQueueManager.Update()
		gs.ElapsedMs += combatRelTymPulseGapMs
		gs.EventQueueManager.Update()
	}

	assert.True(t, gs.HasActiveDurationEffect(game_state.DurationEffectQuickness))
	assert.True(t, mock.SoundEffectWasPlayed(game_state.SoundMagic))
	require.Len(t, mock.ScreenFlashColors, 2)
	for _, flashColor := range mock.ScreenFlashColors {
		assert.Equal(t, game_state.ScreenFlashColorCall{
			R: combatLightBlueFlashRed, G: combatLightBlueFlashGreen, B: combatLightBlueFlashBlue,
		}, flashColor)
	}
	require.NotEmpty(t, mock.ScreenFlashIntensities)
	assert.Zero(t, mock.ScreenFlashIntensities[len(mock.ScreenFlashIntensities)-1])
}

func TestCombatMagicLockEffectMutatesAtSealClimaxAndCleansUp(t *testing.T) {
	scene, mock := newCombatDoorSpellScene(t, references.AnExPor)
	baseline, _ := newCombatDoorSpellScene(t, references.AnExPor)
	gs, expected := scene.gameState, baseline.gameState
	casterIdx := gs.CombatScene.GetActiveCombatantIndex()
	caster := gs.PartyState.GetCharacter(casterIdx)
	target := references.Position{X: 6, Y: 5}
	for _, state := range []*game_state.GameState{gs, expected} {
		setProductionCombatPushTerrain(state, target, indexes.RegularDoor)
	}
	mp, stock := caster.GetCurrentMp(), gs.PartyState.Inventory.Spells.Get(references.AnExPor)
	commits, rng := gs.CastUseCommitSequence(), gs.GetRNG().Calls()
	before := snapshotBlockedCombatState(t, gs)
	require.True(t, gs.BeginCombatAdjacentSpellTargeting(game_state.CastRequest{
		Spell: references.AnExPor, CasterIdx: casterIdx,
	}).Valid)
	require.Equal(t, game_state.CombatAdjacentSpellTargetAwaitingDirection,
		gs.SetCombatAdjacentSpellDirection(references.Right))
	assert.Equal(t, before, snapshotBlockedCombatState(t, gs), "selection is free")
	assert.Equal(t, mp, caster.GetCurrentMp())
	assert.Equal(t, commits, gs.CastUseCommitSequence())
	result := gs.ConfirmCombatAdjacentSpellTarget()
	require.Equal(t, game_state.CombatAdjacentSpellTargetResolved, result.Outcome)
	require.NotNil(t, result.Utility)
	require.Equal(t, target, result.Target)
	wantMP := mp - byte(references.GetSpellCircle(references.AnExPor))
	assert.Equal(t, wantMP, caster.GetCurrentMp())
	assert.Equal(t, stock-1, gs.PartyState.Inventory.Spells.Get(references.AnExPor))
	assert.Equal(t, commits+1, gs.CastUseCommitSequence())
	assert.Equal(t, game_state.CombatAdjacentSpellTargetIgnored, gs.ConfirmCombatAdjacentSpellTarget().Outcome)
	assertSceneCombatDoorTerrain(t, gs, target, indexes.RegularDoor)

	scene.startCombatUtilitySpellEffect(result)
	scene.startCombatUtilitySpellEffect(result) // A repeated delivery cannot start another workflow.
	gs.EventQueueManager.Update()
	require.NotNil(t, scene.combatUtilityEffect)
	assert.True(t, mock.SoundEffectWasPlayed(game_state.SoundMagicLock))
	assert.Equal(t, combatUtilityApproach, scene.combatUtilityEffect.Phase)
	assertSceneCombatDoorTerrain(t, gs, target, indexes.RegularDoor)
	assert.True(t, mock.InputBlocked)

	gs.ElapsedMs += combatUtilityApproachDurationMs - 1
	gs.EventQueueManager.Update()
	assertSceneCombatDoorTerrain(t, gs, target, indexes.RegularDoor)
	assert.Empty(t, combatDoorSpellEvents(mock))
	assertCombatMatchesSingleFinish(t, gs, expected)
	assert.Equal(t, rng, gs.GetRNG().Calls())
	gs.ElapsedMs++
	gs.EventQueueManager.Update()
	gs.EventQueueManager.Update()
	assertSceneCombatDoorTerrain(t, gs, target, indexes.MagicLockDoor)
	require.NotNil(t, scene.combatUtilityEffect)
	assert.Equal(t, combatUtilityResult, scene.combatUtilityEffect.Phase)
	require.Len(t, combatDoorSpellEvents(mock), 1)
	assert.Equal(t, game_state.CombatUtilityDoorMagicLocked, combatDoorSpellEvents(mock)[0].UtilityResult)
	assertCombatMatchesSingleFinish(t, gs, expected)

	gs.ElapsedMs += combatUtilityResultDurationMs - 1
	gs.EventQueueManager.Update()
	require.NotNil(t, scene.combatUtilityEffect)
	assert.True(t, mock.InputBlocked)
	assertCombatMatchesSingleFinish(t, gs, expected)
	gs.ElapsedMs++
	gs.EventQueueManager.Update()
	gs.EventQueueManager.Update()
	assert.Nil(t, scene.combatUtilityEffect)
	assert.Nil(t, gs.CombatScene.AdjacentSpellTargeting)
	assert.False(t, mock.InputBlocked)
	assert.False(t, gs.EventQueueManager.HasWorkflow("combat_utility_spell_effect"))
	expected.FinishCombatAction()
	assertCombatMatchesSingleFinish(t, gs, expected)
	assert.Equal(t, expected.GetRNG().Calls(), gs.GetRNG().Calls())

	scene.startCombatUtilitySpellEffect(result)
	assert.False(t, gs.ResolveCombatUtilitySpell(result.Utility))
	gs.FinishCombatUtilitySpell(result.Utility)
	gs.EventQueueManager.Update()
	assertCombatMatchesSingleFinish(t, gs, expected)
	assertSceneCombatDoorTerrain(t, gs, target, indexes.MagicLockDoor)
	assert.Equal(t, wantMP, caster.GetCurrentMp())
	assert.Equal(t, stock-1, gs.PartyState.Inventory.Spells.Get(references.AnExPor))
	assert.Equal(t, commits+1, gs.CastUseCommitSequence())
	require.Len(t, combatDoorSpellEvents(mock), 1)
	assert.Equal(t, []game_state.SoundEffect{game_state.SoundMagicLock}, mock.SoundEffectsPlayed)
	assert.Equal(t, 1, mock.BlockInputCalls)
	assert.Equal(t, 1, mock.UnblockInputCalls)
	assert.False(t, gs.EventQueueManager.HasWorkflow("combat_utility_spell_effect"))
}

func TestCombatInPorEffectConcealsMovesAndRevealsCaster(t *testing.T) {
	gs, mock := newBritainCombatTestState(t, 1)
	casterIdx := gs.CombatScene.GetActiveCombatantIndex()
	from := gs.CombatScene.GetPartyMemberPosition(casterIdx)
	mock.Reset()
	scene := &GameScene{gameState: gs}

	gs.EventQueueManager.Workflow("test_combat_in_por_effect").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		ThenSequence(scene.thenCastCombatInPor(casterIdx)).
		Finally(func() {}).
		Start()
	for range 3 {
		gs.EventQueueManager.Update()
	}
	require.NotNil(t, scene.combatTeleportEffect)
	assert.Equal(t, combatTeleportDeparture, scene.combatTeleportEffect.Phase)
	assert.Equal(t, from, gs.CombatScene.GetPartyMemberPosition(casterIdx))

	gs.ElapsedMs += combatTeleportDepartureMs
	gs.EventQueueManager.Update()
	gs.EventQueueManager.Update()
	assert.Equal(t, combatTeleportHidden, scene.combatTeleportEffect.Phase)
	assert.NotEqual(t, from, gs.CombatScene.GetPartyMemberPosition(casterIdx))

	gs.ElapsedMs += combatTeleportHiddenMs
	gs.EventQueueManager.Update()
	assert.Equal(t, combatTeleportArrival, scene.combatTeleportEffect.Phase)
	gs.ElapsedMs += combatTeleportArrivalMs
	gs.EventQueueManager.Update()
	gs.EventQueueManager.Update()
	assert.Nil(t, scene.combatTeleportEffect)
	require.NotEmpty(t, mock.ScreenFlashIntensities)
	assert.Zero(t, mock.ScreenFlashIntensities[len(mock.ScreenFlashIntensities)-1])
}

func TestFinishSpellCastingWorkflow_AdvancesCommittedCombatQueue(t *testing.T) {
	gs, mock := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()
	require.NoError(t, gs.TransitionToCombatMap(
		0,
		references.BritanniaCombatTerritory,
		game_state.CombatEntryNorth,
	))
	queue := game_state.NewCombatTurnQueue()
	queue.AddCombatant(fastCombatant{}, 0)
	for range 10 {
		require.NotNil(t, queue.AdvanceToNextActor())
	}
	gs.CombatScene.TurnQueue = queue
	gs.DateTime.Minute = 0
	mock.Reset()
	scene := newProductionCastTestScene(gs)
	castState := &spellCastingWorkflowState{
		ownsCombatAction: true,
		commitSequence:   gs.CastUseCommitSequence(),
	}
	gs.CommitCastUseAttempt()
	scene.finishSpellCastingWorkflow(castState)

	assert.Equal(t, byte(1), gs.DateTime.Minute)
}

func TestCombatEscapeAfterVictoryLeavesInsteadOfOpeningMenu(t *testing.T) {
	gs, mockCallbacks := newBritainCombatTestState(t, 42)
	gs.CombatScene.Victory = true
	mockCallbacks.Reset()
	scene := &GameScene{gameState: gs}
	activeBefore := gs.ActiveCombatPartyMembers()

	scene.handleCombatEscape()
	assert.NotNil(t, gs.CombatScene, "victory exit should animate before leaving combat")
	assert.Equal(t, "Escape...", mockCallbacks.CurrentRowContent)
	assert.True(t, mockCallbacks.InputBlocked)
	for sequenceIndex, expectedPC := range activeBefore {
		if sequenceIndex > 0 {
			gs.ElapsedMs += combatVictoryExitDelayMs
		}
		gs.EventQueueManager.Update()
		require.Len(t, mockCallbacks.CombatEvents, sequenceIndex+1)
		assert.Equal(t, expectedPC, mockCallbacks.CombatEvents[sequenceIndex].Attacker.Index)
		assert.Equal(t, game_state.CombatEventFled, mockCallbacks.CombatEvents[sequenceIndex].Kind)
		assert.Len(t, gs.ActiveCombatPartyMembers(), len(activeBefore)-sequenceIndex-1)
		assert.NotNil(t, gs.CombatScene, "map transition must wait until every PC has disappeared")
	}
	gs.ElapsedMs += combatVictoryExitDelayMs
	gs.EventQueueManager.Update()
	assert.Nil(t, gs.CombatScene)
	gs.EventQueueManager.Update()
	assert.False(t, mockCallbacks.InputBlocked)
	assert.Equal(t, 1, mockCallbacks.BlockInputCalls)
	assert.Equal(t, 1, mockCallbacks.UnblockInputCalls)
}

func TestCombatEscapeWithNoPCsAndSurvivingAllyPreservesDefeat(t *testing.T) {
	gs, mockCallbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(42).
		Build()
	require.NoError(t, gs.TransitionToCombatMap(
		int(references.ShipSeaCombatMap),
		references.BritanniaCombatTerritory,
		game_state.CombatEntryNorth,
	))
	for index := range gs.CombatScene.PartyActive {
		gs.CombatScene.PartyActive[index] = false
	}
	allyRef := (*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	ally := game_state.NewCombatMonster(
		references.MonsterOrc, &allyRef, references.Position{X: 4, Y: 5}, gs.AsRandomProvider(),
	)
	ally.SetLoyalty(game_state.CombatMonsterLoyaltyParty)
	gs.CombatScene.AddMonsterToQueue(ally)
	hostileRef := (*gs.GameReferences.EnemyReferences)[references.MonsterTroll]
	gs.CombatScene.AddMonsterToQueue(game_state.NewCombatMonster(
		references.MonsterTroll, &hostileRef, references.Position{X: 6, Y: 5}, gs.AsRandomProvider(),
	))
	mockCallbacks.Reset()
	scene := &GameScene{gameState: gs}

	scene.handleCombatEscape()

	assert.Nil(t, gs.CombatScene)
	require.Len(t, mockCallbacks.CombatConclusions, 1)
	assert.Equal(t, game_state.CombatDefeat, mockCallbacks.CombatConclusions[0].Reason)
}

func TestCombatEscapeBeforeVictoryReportsRefusalInsteadOfOpeningMenu(t *testing.T) {
	gs, mockCallbacks := game_state.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(42).
		Build()
	require.NoError(t, gs.TransitionToCombatMap(
		int(references.ShipSeaCombatMap),
		references.BritanniaCombatTerritory,
		game_state.CombatEntryNorth,
	))
	monsterRef := &(*gs.GameReferences.EnemyReferences)[references.MonsterOrc]
	gs.CombatScene.AddMonsterToQueue(game_state.NewCombatMonster(
		references.MonsterOrc,
		monsterRef,
		references.Position{X: 5, Y: 5},
		gs.AsRandomProvider(),
	))
	gs.BeginCombatTurns(game_state.CombatTurnsImmediate)
	mockCallbacks.Reset()
	scene := &GameScene{
		gameState: gs,
		keyboard:  input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
	}

	scene.combatMapInputHandler(ebiten.KeyEscape)
	gs.EventQueueManager.Update()

	assert.NotNil(t, gs.CombatScene)
	assert.Equal(t, "Escape...", mockCallbacks.CurrentRowContent)
	assert.Equal(t, []string{"Not yet!"}, mockCallbacks.Messages)
	assert.False(t, scene.dialogStack.HasModalDialog(),
		"combat Escape must not fall through to the global menu")
}
