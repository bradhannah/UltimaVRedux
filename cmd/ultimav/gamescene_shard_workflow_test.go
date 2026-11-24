package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestSmallMapUseSelectorDispatchesEachSelectedShard(t *testing.T) {
	for _, test := range []struct {
		name  string
		shard references.QuestItem
	}{
		{name: "Falsehood", shard: references.ShardOfFalsehood},
		{name: "Hatred", shard: references.ShardOfHatred},
		{name: "Cowardice", shard: references.ShardOfCowardice},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, _ := newShardWorkflowTestScene(t)
			clearShardUseTestInventory(scene.gameState)
			scene.gameState.PartyState.Inventory.QuestItems.Set(test.shard, 1)
			var selected []references.QuestItem
			scene.gameState.SystemCallbacks.Flow.Shadowlord.ShowShardUsageWorkflow = func(item references.QuestItem, _ func()) {
				selected = append(selected, item)
			}

			scene.smallMapInputHandler(ebiten.KeyU)
			dialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
			dialog.HandleEnter()
			drainSceneWorkflow(t, scene, "use_item_workflow", DialogPollingRateMs)

			assert.Equal(t, []references.QuestItem{test.shard}, selected)
			assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.QuestItems.Get(test.shard))
		})
	}
}

func TestSmallMapUseSelectorCancellationDoesNotQueueOrConsumeShard(t *testing.T) {
	scene, _ := newShardWorkflowTestScene(t)
	clearShardUseTestInventory(scene.gameState)
	scene.gameState.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)
	var selected []references.QuestItem
	scene.gameState.SystemCallbacks.Flow.Shadowlord.ShowShardUsageWorkflow = func(item references.QuestItem, _ func()) {
		selected = append(selected, item)
	}

	scene.smallMapInputHandler(ebiten.KeyU)
	dialog := waitForTopModal[*widgets.ItemSelectionDialog](t, scene)
	dialog.Dismiss()
	drainSceneWorkflow(t, scene, "use_item_workflow", DialogPollingRateMs)

	assert.Empty(t, selected)
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow("shard_workflow"))
}

func TestSmallMapYellResolvesTurnAfterSubmittedResult(t *testing.T) {
	for _, test := range []struct {
		name       string
		word       string
		suppresses bool
	}{
		{name: "successful summoning", word: "FAULINEI", suppresses: true},
		{name: "failed yell", word: "RANDOM"},
	} {
		t.Run(test.name, func(t *testing.T) {
			scene, _ := newShardWorkflowTestScene(t)
			scene.gameState.ShadowlordState.Reset()
			scene.gameState.CurrentNPCAIController.GetNpcs().ReplaceWith(nil)
			before := scene.gameState.DateTime

			scene.smallMapInputHandler(ebiten.KeyY)

			assert.Equal(t, before, scene.gameState.DateTime, "Yell must wait for submitted input before resolving the turn")
			dialog := waitForTopModal[*widgets.InputBox](t, scene)
			submitSmallMapYell(t, scene, dialog, test.word)
			drainSceneWorkflow(t, scene, "yell_small_map", KeyboardDebounceMs)

			if test.suppresses {
				assert.Equal(t, before, scene.gameState.DateTime)
				enemy := scene.gameState.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(
					references.Position{X: 15, Y: 7},
					2,
				)
				require.NotNil(t, enemy, "the new Shadowlord must not move or attack immediately")

				return
			}

			expected := before
			expected.Advance(1)
			assert.Equal(t, expected, scene.gameState.DateTime, "failed Yell must complete one normal small-map turn")
		})
	}
}

func TestShardWorkflowFollowsSourceDestructionSequence(t *testing.T) {
	scene, callbacks := newShardWorkflowTestScene(t)
	setupShardWorkflowRitual(t, scene.gameState)
	stopped := trackStoppedSounds(scene)
	looped := trackLoopedSounds(scene)
	before := scene.gameState.DateTime

	scene.StartShardWorkflow(references.ShardOfFalsehood)
	dialogCount := drainShardWorkflow(t, scene)

	assert.Equal(t, 3, dialogCount, "raise, Flame, and doom dialogs must be serialized")
	assert.Equal(t, []game_state.SoundEffect{
		game_state.SoundShardPulse,
		game_state.SoundShadowlordVictory,
	}, callbacks.SoundEffectsPlayed)
	assert.Equal(t, []soundFadeCall{{effect: game_state.SoundShardRitualEarthquake, durationMs: earthquakeFadeInMs}}, *looped)
	require.Len(t, scene.visualEffects, shardImpactCount)
	for _, effect := range scene.visualEffects {
		require.NotNil(t, effect.WorldPos)
		assert.Equal(t, (references.Position{X: 15, Y: 8}), *effect.WorldPos)
		assert.Equal(t, "kapow", effect.Type)
	}
	assert.Empty(t, scene.activeTileDissolves, "source ritual must not teleport or dissolve the Shadowlord")
	assert.Zero(t, scene.gameState.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.False(t, scene.gameState.ShadowlordState.IsAlive(references.Faulinei))
	assert.False(t, scene.gameState.ShadowlordState.Summoned)
	assert.True(t, scene.gameState.PartyState.IsNPCDead(references.Stonegate, 1))
	assert.Nil(t, scene.gameState.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(
		references.Position{X: 15, Y: 8}, 2,
	))
	assertShadowlordWorkflowPresentationClean(t, scene, callbacks)
	assert.Contains(t, *stopped, game_state.SoundStopRequest{
		Effect: game_state.SoundShardRitualEarthquake, FadeOutMs: earthquakeFadeOutMs,
	})
	assert.False(t, soundStopRequested(*stopped, game_state.SoundEarthquake))
	assertShardWorkflowFinishedTurn(t, before, scene.gameState.DateTime)
}

func TestShardWorkflowRevealsNameOnlyAtMountedDoomFeedback(t *testing.T) {
	scene, _ := newShardWorkflowTestScene(t)
	setupShardWorkflowRitual(t, scene.gameState)
	scene.gameState.SystemCallbacks.UI.JournalChanged = scene.journalChanged
	scene.StartShardWorkflow(references.ShardOfFalsehood)
	committedBeforeFeedback := false
	namedFeedbackPresented := false
	for range 1000 {
		scene.gameState.ElapsedMs += 2500
		scene.gameState.EventQueueManager.Update()
		overlay, _ := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog)
		if overlay != nil && strings.Contains(overlay.GetMessage(), "The doom of the Shadowlord") {
			require.True(t, overlay.IsVisible())
			namedFeedbackPresented = true
		}
		_, learned := scene.gameState.ShadowlordKnowledge(references.Faulinei, game_state.ShadowlordKnowledgeName)
		require.Equal(t, namedFeedbackPresented, learned)
		_, associated := scene.gameState.ShadowlordKnowledge(references.Faulinei, game_state.ShadowlordKnowledgeAssociation)
		require.False(t, associated, "named doom feedback does not explicitly teach a domain association")
		if !scene.gameState.ShadowlordState.IsAlive(references.Faulinei) && !namedFeedbackPresented {
			committedBeforeFeedback = true
		}
		if overlay != nil {
			overlay.Dismiss()
		}
		if !scene.gameState.EventQueueManager.HasWorkflow("shard_workflow") {
			break
		}
	}
	require.True(t, committedBeforeFeedback, "authoritative defeat precedes the seven impacts and named feedback")
	require.True(t, namedFeedbackPresented)
	require.False(t, scene.gameState.EventQueueManager.HasWorkflow("shard_workflow"))
	entry, learned := scene.gameState.ShadowlordKnowledge(references.Faulinei, game_state.ShadowlordKnowledgeName)
	require.True(t, learned)
	require.Equal(t, game_state.RevelationKnowledgeSource("Banishment"), entry.Source)
}

func TestShardConfirmationDiscoveryWaitsForVisibleOwnedDialog(t *testing.T) {
	scene := &GameScene{gameState: newGameActionCallbacksKnowledgeState(t), keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
	called := 0
	scene.gameState.EventQueueManager.Workflow("journal_shard_confirmation").Priority(event_queue.PriorityNormal).
		Sequential().First(func() {}).ThenSequence(scene.thenShowShardConfirmationDialog("Named feedback", func() {
		called++
		require.Same(t, scene.shadowlordShardDialog, scene.dialogStack.PeekTopModalDialog())
		require.True(t, scene.shadowlordShardDialog.IsVisible())
	})).Start()
	require.Zero(t, called, "building the sequence must not publish knowledge")
	for range 10 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if scene.shadowlordShardDialog != nil {
			break
		}
	}
	require.Equal(t, 1, called)
	scene.Dispose()
	require.Equal(t, 1, called, "shutdown does not replay presentation discovery")
}

func TestShardWorkflowSceneDisposalCleansOwnedPresentationWithoutMutation(t *testing.T) {
	scene, callbacks := newShardWorkflowTestScene(t)
	setupShardWorkflowRitual(t, scene.gameState)
	stopped := trackStoppedSounds(scene)
	looped := trackLoopedSounds(scene)
	before := scene.gameState.DateTime

	scene.StartShardWorkflow(references.ShardOfFalsehood)
	for range 50 {
		if overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay.Dismiss()
		}
		scene.gameState.ElapsedMs += 2500
		scene.gameState.EventQueueManager.Update()
		if len(*looped) != 0 {
			break
		}
	}
	require.Equal(t, []soundFadeCall{{effect: game_state.SoundShardRitualEarthquake, durationMs: earthquakeFadeInMs}}, *looped)
	require.True(t, callbacks.InputBlocked)
	require.NotNil(t, scene.gameState.CurrentNPCAIController.GetNpcs().GetVisibleEnemyAtPositionAndFloorOrNil(
		references.Position{X: 15, Y: 8}, 2,
	))

	scene.Dispose()

	assertShadowlordWorkflowPresentationClean(t, scene, callbacks)
	assert.False(t, scene.gameState.EventQueueManager.HasWorkflow("shard_workflow"))
	assert.False(t, callbacks.InputBlocked)
	assert.Contains(t, *stopped, game_state.SoundStopRequest{
		Effect: game_state.SoundShardRitualEarthquake, FadeOutMs: earthquakeFadeOutMs,
	})
	assert.False(t, soundStopRequested(*stopped, game_state.SoundEarthquake))
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.True(t, scene.gameState.ShadowlordState.IsAlive(references.Faulinei))
	assert.True(t, scene.gameState.ShadowlordState.Summoned)
	assert.Equal(t, before, scene.gameState.DateTime, "shard Finally must not finish a turn after scene disposal")
}

func TestShardWorkflowPriorityInterruptionResumesAndCleansPresentation(t *testing.T) {
	scene, callbacks := newShardWorkflowTestScene(t)
	setupShardWorkflowRitual(t, scene.gameState)
	scene.gameState.MapState.PlayerLocation.Position = references.Position{X: 14, Y: 9}

	scene.StartShardWorkflow(references.ShardOfFalsehood)
	for range 20 {
		scene.gameState.ElapsedMs += DialogPollingRateMs
		scene.gameState.EventQueueManager.Update()
		if scene.shadowlordShardDialog != nil {
			break
		}
	}
	require.NotNil(t, scene.shadowlordShardDialog)

	scene.gameState.EventQueueManager.Workflow("shadowlord_presentation_interrupt").
		Priority(event_queue.PriorityHigh).
		Sequential().
		First(func() {}).
		Start()
	scene.gameState.EventQueueManager.Update()
	scene.gameState.EventQueueManager.Update()
	drainShardWorkflow(t, scene)

	assertShadowlordWorkflowPresentationClean(t, scene, callbacks)
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.True(t, scene.gameState.ShadowlordState.IsAlive(references.Faulinei))
}

func TestShardWorkflowDefersMissingPhysicalShadowlordFailureUntilAfterEarthquakes(t *testing.T) {
	scene, callbacks := newShardWorkflowTestScene(t)
	setupShardWorkflowRitual(t, scene.gameState)
	before := scene.gameState.DateTime
	looped := trackLoopedSounds(scene)
	units := scene.gameState.CurrentNPCAIController.GetNpcs()
	enemy := units.GetVisibleEnemyAtPositionAndFloorOrNil(references.Position{X: 15, Y: 8}, 2)
	require.NotNil(t, enemy)
	require.True(t, units.RemoveEnemy(enemy))

	scene.StartShardWorkflow(references.ShardOfFalsehood)
	dialogCount := drainShardWorkflow(t, scene)

	assert.Equal(t, 2, dialogCount, "late physical failure has no extra no-effect or doom message (ref:SHADOW-0005)")
	assert.Equal(t, []game_state.SoundEffect{
		game_state.SoundShardPulse,
	}, callbacks.SoundEffectsPlayed)
	assert.Equal(t, []soundFadeCall{{effect: game_state.SoundShardRitualEarthquake, durationMs: earthquakeFadeInMs}}, *looped)
	assert.Empty(t, scene.visualEffects)
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.True(t, scene.gameState.ShadowlordState.IsAlive(references.Faulinei))
	assert.True(t, scene.gameState.ShadowlordState.Summoned)
	assert.Equal(t, before, scene.gameState.DateTime, "missing physical target is a free prerequisite rejection")
}

func TestShardWorkflowRejectsWrongFlameBeforeEarthquakes(t *testing.T) {
	scene, callbacks := newShardWorkflowTestScene(t)
	setupShardWorkflowRitual(t, scene.gameState)
	scene.gameState.MapState.PlayerLocation.Position = references.Position{X: 14, Y: 9}
	before := scene.gameState.DateTime

	scene.StartShardWorkflow(references.ShardOfFalsehood)
	dialogCount := drainShardWorkflow(t, scene)

	assert.Equal(t, 2, dialogCount, "raise and no-effect dialogs must be serialized")
	assert.Equal(t, []game_state.SoundEffect{game_state.SoundShardPulse}, callbacks.SoundEffectsPlayed)
	assert.Empty(t, scene.visualEffects)
	assert.Equal(t, uint16(1), scene.gameState.PartyState.Inventory.QuestItems.Get(references.ShardOfFalsehood))
	assert.True(t, scene.gameState.ShadowlordState.IsAlive(references.Faulinei))
	assert.Equal(t, before, scene.gameState.DateTime, "wrong flame is a free prerequisite rejection")
}

func newShardWorkflowTestScene(t *testing.T) (*GameScene, *game_state.MockSystemCallbacks) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Lycaeum).
		WithFloor(2).
		WithPlayerAt(15, 9).
		WithSystemCallbacks().
		WithRandomSeed(1).
		Build()

	_, output := newMainOutput(1280)

	return &GameScene{
		gameState:         state,
		output:            output,
		gameReferences:    state.GameReferences,
		keyboard:          input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		spriteSheet:       &sprites.SpriteSheet{},
		autoOpenDoorState: &AutoOpenDoorState{},
	}, callbacks
}

func setupShardWorkflowRitual(t *testing.T, state *game_state.GameState) {
	t.Helper()
	units := state.CurrentNPCAIController.GetNpcs()
	require.NotNil(t, units)
	units.ReplaceWith(nil)
	state.PartyState.RestoreNPCFlags(nil, nil)
	state.ShadowlordState.Reset()
	state.PartyState.Inventory.QuestItems.Set(references.ShardOfFalsehood, 1)
	require.True(t, state.ShadowlordState.Summon(references.Faulinei))
	require.True(t, state.SpawnShadowlordNPCAtPosition(references.Faulinei, references.Position{X: 15, Y: 8}))
}

func clearShardUseTestInventory(state *game_state.GameState) {
	for _, item := range references.QuestItemCatalog() {
		state.PartyState.Inventory.QuestItems.Set(item, 0)
	}
	for potion := references.Blue; potion <= references.White; potion++ {
		state.PartyState.Inventory.Potions.Set(potion, 0)
	}
	for scroll := references.ScrollVasLor; scroll <= references.ScrollAnTym; scroll++ {
		state.PartyState.Inventory.Scrolls.Set(scroll, 0)
	}
	for moonstone := references.NewMoon; moonstone <= references.CrescentWaning; moonstone++ {
		state.PartyState.Inventory.Moonstones.Set(moonstone, 0)
	}
	state.PartyState.Inventory.Provisions.SkullKeys.Set(0)
}

func submitSmallMapYell(t *testing.T, scene *GameScene, dialog *widgets.InputBox, word string) {
	t.Helper()
	for _, letter := range word {
		key := ebiten.Key(int(ebiten.KeyA) + int(letter-'A'))
		scene.keyboard.SetAllowKeyPressImmediately()
		scene.keyboard.SimulateKeyPress(key)
		dialog.Update()
		scene.keyboard.ClearFrameConsumption()
	}
	require.Equal(t, word, dialog.GetText())
	scene.keyboard.SetAllowKeyPressImmediately()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
}

func assertShardWorkflowFinishedTurn(t *testing.T, before, actual datetime.UltimaDate) {
	t.Helper()
	expected := before
	expected.Advance(1)
	assert.Equal(t, expected, actual, "selected Shard Use must complete one normal small-map turn")
}

func drainShardWorkflow(t *testing.T, scene *GameScene) int {
	t.Helper()
	dialogCount := 0
	for range 100 {
		if overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			dialogCount++
			overlay.Dismiss()
		}
		scene.gameState.ElapsedMs += 2500
		scene.gameState.EventQueueManager.Update()
		if !scene.gameState.EventQueueManager.HasWorkflow("shard_workflow") {
			return dialogCount
		}
	}
	t.Fatal("Shard workflow did not complete")

	return 0
}

func assertShadowlordWorkflowPresentationClean(
	t *testing.T,
	scene *GameScene,
	callbacks *game_state.MockSystemCallbacks,
) {
	t.Helper()
	assert.Nil(t, scene.shadowlordShardDialog)
	assert.False(t, scene.shadowlordShardActive)
	assert.False(t, scene.dialogStack.HasModalDialog())
	assert.False(t, scene.inputBlocked)
	assert.False(t, callbacks.InputBlocked)
	require.NotEmpty(t, callbacks.ScreenShakeOffsets)
	lastShake := callbacks.ScreenShakeOffsets[len(callbacks.ScreenShakeOffsets)-1]
	assert.Zero(t, lastShake.X)
	assert.Zero(t, lastShake.Y)
}

type soundFadeCall struct {
	effect     game_state.SoundEffect
	durationMs int64
}

func trackLoopedSounds(scene *GameScene) *[]soundFadeCall {
	looped := make([]soundFadeCall, 0)
	audio := scene.gameState.SystemCallbacks.Audio
	scene.gameState.SystemCallbacks.Audio = game_state.NewAudioCallbacks(
		func(request game_state.SoundPlaybackRequest) {
			if request.Mode == game_state.SoundPlaybackLoop {
				looped = append(looped, soundFadeCall{effect: request.Effect, durationMs: request.FadeInMs})

				return
			}
			audio.RequestPlayback(request)
		},
		audio.RequestStop,
		audio.UpdateLocationMusic,
		audio.PauseBackgroundMusic,
		audio.ResumeBackgroundMusic,
		audio.RequestVoice,
	)

	return &looped
}

func soundStopRequested(requests []game_state.SoundStopRequest, effect game_state.SoundEffect) bool {
	for _, request := range requests {
		if request.Effect == effect {
			return true
		}
	}

	return false
}
