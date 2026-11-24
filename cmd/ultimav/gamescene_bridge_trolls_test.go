package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

type bridgeCombatSignature struct {
	monsterType references.MonsterType
	position    references.Position
	dexterity   int
}

func TestBridgeTrollAttackPathsEnterSameSeededCombat(t *testing.T) {
	refusalSignatures, refusalGold := runBridgeTrollAttackPath(t, ebiten.KeyN, 100, 1)
	insufficientSignatures, insufficientGold := runBridgeTrollAttackPath(t, ebiten.KeyY, 0, 1)

	assert.Equal(t, refusalSignatures, insufficientSignatures)
	assert.Equal(t, uint16(100), refusalGold)
	assert.Equal(t, uint16(0), insufficientGold)
}

func TestBridgeTrollSneakingStopsAfterFirstCaughtMember(t *testing.T) {
	oneMember, _ := runBridgeTrollAttackPath(t, ebiten.KeyN, 100, 1)
	twoMembers, _ := runBridgeTrollAttackPath(t, ebiten.KeyN, 100, 2)

	assert.Equal(t, oneMember, twoMembers,
		"a later party member must not consume another sneak roll after the first failure")
}

func TestBridgeTrollSuccessfulPaymentRetainsTollTranscript(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	state.SetRandomSeed(95)
	state.PartyState.Inventory.Gold.Set(100)
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapAtInn(references.Britain)
		character.SetStatus(party_state.Good)
	}
	character := state.PartyState.GetCharacter(0)
	character.PartyMap = party_state.PartyMapActive
	character.Dexterity = 0
	character.Strength = 20
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}

	scene.StartBridgeTrollEncounterWorkflow()
	initialDialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	require.Contains(t, initialDialog.TranscriptText(), "Thou spieth trolls under the bridge!")
	require.Empty(t, initialDialog.ActivePrompt())
	tollDialog := waitForBridgeTollDialog(t, scene)
	require.Same(t, initialDialog, tollDialog)
	require.Contains(t, tollDialog.TranscriptText(), character.GetNameAsString()+" sneaks across...")
	require.Contains(t, tollDialog.TranscriptText(), "Caught!")
	require.Contains(t, tollDialog.TranscriptText(), "The trolls demand a 39 gp toll!")
	scene.keyboard.SimulateKeyPress(ebiten.KeyY)
	tollDialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	tollDialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !tollDialog.InputComplete() {
			break
		}
	}

	require.Equal(t, uint16(61), state.PartyState.Inventory.Gold.Get())
	require.Contains(t, tollDialog.TranscriptText(), "Dost thou pay?  > Yes")
	require.Contains(t, tollDialog.TranscriptText(), "Paid 39 gold.")
	require.NotContains(t, tollDialog.TranscriptText(), "Not enough")
	require.Contains(t, callbacks.SoundEffectsPlayed, game_state.SoundCoinSplash)
	require.False(t, state.HasPendingOverworldContact())
	scene.keyboard.ClearFrameConsumption()
	scene.keyboard.SetForceWaitAnyKey(-1000)
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	tollDialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("bridge_troll_encounter") {
			break
		}
	}
	require.False(t, state.EventQueueManager.HasActiveWorkflows())
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func TestBridgeTrollEvasionUsesPassiveTranscriptUntilAcknowledgement(t *testing.T) {
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapAtInn(references.Britain)
		character.SetStatus(party_state.Good)
	}
	character := state.PartyState.GetCharacter(0)
	character.PartyMap = party_state.PartyMapActive
	character.Dexterity = 30
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}

	scene.StartBridgeTrollEncounterWorkflow()
	dialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	require.Empty(t, dialog.ActivePrompt())
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if strings.Contains(dialog.TranscriptText(), "Trolls evaded!") && !dialog.InputComplete() {
			break
		}
	}

	require.Contains(t, dialog.TranscriptText(), "Thou spieth trolls under the bridge!")
	require.Contains(t, dialog.TranscriptText(), character.GetNameAsString()+" sneaks across...")
	require.Contains(t, dialog.TranscriptText(), "Trolls evaded!")
	require.NotContains(t, dialog.TranscriptText(), "Caught!")
	require.Empty(t, dialog.ActivePrompt())
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	dialog.Update()
	for range 20 {
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("bridge_troll_encounter") {
			break
		}
	}

	require.False(t, state.EventQueueManager.HasActiveWorkflows())
	require.False(t, scene.dialogStack.HasOpenDialog())
	require.False(t, state.HasPendingOverworldContact())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
}

func runBridgeTrollAttackPath(
	t *testing.T,
	choice ebiten.Key,
	gold uint16,
	activeMembers int,
) ([]bridgeCombatSignature, uint16) {
	t.Helper()
	state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().
		Build()
	state.SetRandomSeed(95)
	state.PartyState.Inventory.Gold.Set(gold)
	for index := range party_state.MaxActiveParty {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapAtInn(references.Britain)
		character.SetStatus(party_state.Good)
	}
	for index := range activeMembers {
		character := state.PartyState.GetCharacter(index)
		character.PartyMap = party_state.PartyMapActive
		character.SetStatus(party_state.Good)
		character.Dexterity = 0
		character.Strength = 20
	}

	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)}
	before := state.DateTime
	scene.StartBridgeTrollEncounterWorkflow()

	initialDialog := waitForTopModalWithin[*widgets.InteractionTranscript](t, scene, 1000, 20)
	tollDialog := waitForBridgeTollDialog(t, scene)
	require.Same(t, initialDialog, tollDialog)
	require.Equal(t, "Dost thou pay?", tollDialog.ActivePrompt())
	require.Contains(t, tollDialog.TranscriptText(), "Caught!")
	scene.keyboard.SimulateKeyPress(choice)
	tollDialog.Update()
	scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
	tollDialog.Update()
	require.Same(t, tollDialog, scene.dialogStack.PeekTopModalDialog())

	if choice == ebiten.KeyY {
		for range 30 {
			state.ElapsedMs += 1000
			state.EventQueueManager.Update()
		}
		require.Same(t, tollDialog, scene.dialogStack.PeekTopModalDialog())
		require.Contains(t, tollDialog.TranscriptText(), "Thou hast not enough gold. The toll is unpaid.")
		require.Contains(t, tollDialog.TranscriptText(), "Dost thou pay?  > Yes")
		require.Empty(t, tollDialog.ActivePrompt())
		require.False(t, tollDialog.InputComplete())
		require.True(t, state.EventQueueManager.HasWorkflow("bridge_troll_encounter"))
		require.False(t, state.HasPendingOverworldContact())
		require.Nil(t, state.CombatScene)
		require.Equal(t, gold, state.PartyState.Inventory.Gold.Get())
		require.Equal(t, before, state.DateTime)
		require.Zero(t, callbacks.UnblockInputCalls)
	}

	for range 30 {
		if overlay, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.OverlayDialog); ok {
			overlay.Dismiss()
		}
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			dialog.ActivePrompt() == "" && !dialog.InputComplete() {
			scene.keyboard.ClearFrameConsumption()
			scene.keyboard.SetForceWaitAnyKey(-1000)
			scene.keyboard.SimulateKeyPress(ebiten.KeyEnter)
			dialog.Update()
		}
		state.ElapsedMs += 1000
		state.EventQueueManager.Update()
		if !state.EventQueueManager.HasWorkflow("bridge_troll_encounter") {
			break
		}
	}
	require.False(t, state.EventQueueManager.HasActiveWorkflows())
	require.Equal(t, 1, callbacks.BlockInputCalls)
	require.Equal(t, 1, callbacks.UnblockInputCalls)
	require.Empty(t, callbacks.TimeAdvanced())
	require.Equal(t, before, state.DateTime)
	require.True(t, state.HasPendingOverworldContact())
	require.Nil(t, state.CombatScene)
	require.NoError(t, state.ResolvePendingOverworldContact())
	require.NotNil(t, state.CombatScene)
	require.False(t, state.HasPendingOverworldContact())
	assert.Equal(t, int(references.BigBridgeCombatMap), state.CombatScene.CombatMapRef.MapIndex)
	assert.False(t, scene.dialogStack.HasOpenDialog())

	signatures := make([]bridgeCombatSignature, state.CombatScene.MonsterManager.Count())
	state.CombatScene.MonsterManager.ForEach(func(index int, monster *game_state.CombatMonster) {
		signatures[index] = bridgeCombatSignature{
			monsterType: monster.MonsterType(),
			position:    monster.Position(),
			dexterity:   monster.CurrentDex(),
		}
	})

	return signatures, state.PartyState.Inventory.Gold.Get()
}

func waitForBridgeTollDialog(t *testing.T, scene *GameScene) *widgets.InteractionTranscript {
	t.Helper()
	for range 20 {
		scene.gameState.ElapsedMs += 1000
		scene.gameState.EventQueueManager.Update()
		if dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.InteractionTranscript); ok &&
			dialog.ActivePrompt() != "" {
			return dialog
		}
	}
	t.Fatal("bridge toll yes/no dialog was not shown")

	return nil
}
