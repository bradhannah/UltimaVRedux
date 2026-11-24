package main

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestGameSceneShowZStatsDossierOwnsExactModalLifecycle(t *testing.T) {
	state := newGameActionCallbacksKnowledgeState(t)
	require.True(t, state.DiscoverSpellKnowledge(
		references.AnYlem,
		game_state.DialogueKnowledgeSource("Mariah"),
	))
	dossier := game_state.ZStatsDossier{
		Characters: []game_state.ZStatsCharacter{
			{RosterIndex: 2, Name: "Avatar"},
			{RosterIndex: 7, Name: "Mariah"},
		},
		Magic: game_state.ZStatsMagicDossier{
			Spells: []game_state.ZStatsSpell{{
				ID: references.AnYlem.ID(), Name: "An Ylem", Hidden: true, Unread: true,
			}},
		},
		UnreadHiddenSpellCount: 1,
		InitialCharacterIndex:  7,
	}
	keyboard := input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld)
	scene := &GameScene{
		gameState:   state,
		keyboard:    keyboard,
		spriteSheet: sharedPresentationTestSpriteSheet(),
	}
	scene.secondaryKeyState = LookDirectionInput
	locationBefore := state.MapState.PlayerLocation
	activePlayerBefore := state.GetActivePlayerIndex()
	inputBefore := scene.secondaryKeyState
	underlay := &countingDialog{}
	scene.dialogStack.PushModalDialog(underlay)

	scene.ShowZStatsDossier(dossier)

	dialog, ok := scene.dialogStack.PeekTopModalDialog().(*widgets.ZStatsDossierDialog)
	require.True(t, ok)
	assert.Equal(t, 1, dialog.GetSelectedCharacterPosition())
	assert.Equal(t, 2, scene.dialogStack.Count())

	knowledgeBefore := state.KnowledgeSnapshot()
	for range 3 {
		keyboard.SimulateKeyPress(ebiten.KeyTab)
		require.True(t, scene.updateDialogs())
	}
	knowledge, found := state.SpellKnowledge(references.AnYlem)
	require.True(t, found)
	assert.True(t, knowledge.Unread)
	assert.Equal(t, knowledgeBefore, state.KnowledgeSnapshot())
	assert.True(t, dossier.Magic.Spells[0].Unread, "the modal must not mutate its detached input")

	keyboard.SimulateKeyPress(ebiten.KeyEscape)
	require.True(t, scene.updateDialogs())

	assert.True(t, dialog.IsDismissed())
	assert.Equal(t, 1, scene.dialogStack.Count())
	assert.Same(t, underlay, scene.dialogStack.PeekTopModalDialog())
	assert.Equal(t, locationBefore, state.MapState.PlayerLocation)
	assert.Equal(t, activePlayerBefore, state.GetActivePlayerIndex())
	assert.Equal(t, inputBefore, scene.secondaryKeyState)
}

func TestExplorationZStatsRoutingDoesNotFinishTurn(t *testing.T) {
	tests := []struct {
		name     string
		location references.Location
		handle   func(*GameScene)
	}{
		{
			name:     "small map",
			location: references.Britain,
			handle:   func(scene *GameScene) { scene.smallMapInputHandler(ebiten.KeyZ) },
		},
		{
			name:     "large map",
			location: references.BritanniaUnderworld,
			handle:   func(scene *GameScene) { scene.largeMapInputHandler(ebiten.KeyZ) },
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			state, callbacks := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(test.location).
				WithSystemCallbacks().
				Build()
			state.ActiveDurationEffect = game_state.DurationEffect{
				EffectType: game_state.DurationEffectProtection, TurnsRemaining: 7,
			}
			callbacks.Reset()
			scene := &GameScene{
				gameState:         state,
				keyboard:          input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
				spriteSheet:       sharedPresentationTestSpriteSheet(),
				autoOpenDoorState: &AutoOpenDoorState{},
			}
			var shown []game_state.ZStatsDossier
			state.SystemCallbacks.UI.ShowZStatsDossier = func(dossier game_state.ZStatsDossier) {
				shown = append(shown, dossier)
				scene.ShowZStatsDossier(dossier)
			}
			beforeDate := state.DateTime
			beforeEffect := state.ActiveDurationEffect
			beforeLocation := state.MapState.PlayerLocation
			beforeActivePlayer := state.GetActivePlayerIndex()
			beforeInput := scene.secondaryKeyState
			beforeInventory := state.PartyState.Inventory.LockerSnapshot()
			beforeSpells := state.PartyState.Inventory.SpellSnapshot()
			beforeKnowledge := state.KnowledgeSnapshot()

			test.handle(scene)

			require.Len(t, shown, 1)
			assert.NotEmpty(t, shown[0].Characters)
			require.IsType(t, &widgets.ZStatsDossierDialog{}, scene.dialogStack.PeekTopModalDialog())
			for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeyDown, ebiten.KeyPageDown, ebiten.KeyEscape} {
				scene.keyboard.SimulateKeyPress(key)
				require.True(t, scene.updateDialogs())
			}
			assert.False(t, scene.dialogStack.HasModalDialog())
			assert.Equal(t, []string{"Ztats..."}, callbacks.CommandPrompts)
			assert.Equal(t, beforeDate, state.DateTime)
			assert.Equal(t, beforeEffect, state.ActiveDurationEffect)
			assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
			assert.Equal(t, beforeActivePlayer, state.GetActivePlayerIndex())
			assert.Equal(t, beforeInput, scene.secondaryKeyState)
			assert.Equal(t, beforeInventory, state.PartyState.Inventory.LockerSnapshot())
			assert.Equal(t, beforeSpells, state.PartyState.Inventory.SpellSnapshot())
			assert.Equal(t, beforeKnowledge, state.KnowledgeSnapshot())
			assert.Empty(t, callbacks.TimeAdvanced())
		})
	}
}

func TestDungeonZStatsRoutingStaysAtCommandBoundary(t *testing.T) {
	scene, state, callbacks := newDungeonInputTestScene(t)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 7,
	}
	scene.spriteSheet = sharedPresentationTestSpriteSheet()
	var shown []game_state.ZStatsDossier
	state.SystemCallbacks.UI.ShowZStatsDossier = func(dossier game_state.ZStatsDossier) {
		shown = append(shown, dossier)
		scene.ShowZStatsDossier(dossier)
	}
	beforeDate := state.DateTime
	beforeEffect := state.ActiveDurationEffect
	beforeLocation := state.MapState.PlayerLocation
	beforeActivePlayer := state.GetActivePlayerIndex()
	beforeInput := scene.secondaryKeyState
	beforeInventory := state.PartyState.Inventory.LockerSnapshot()
	beforeKnowledge := state.KnowledgeSnapshot()
	require.True(t, state.IsAtDungeonCommandBoundary())

	scene.dungeonMapInputHandler(ebiten.KeyZ)

	require.Len(t, shown, 1)
	for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeyDown, ebiten.KeyPageDown, ebiten.KeyEscape} {
		scene.keyboard.SimulateKeyPress(key)
		require.True(t, scene.updateDialogs())
	}
	assert.False(t, scene.dialogStack.HasModalDialog())
	assert.Equal(t, []string{"Ztats..."}, callbacks.CommandPrompts)
	assert.True(t, state.IsAtDungeonCommandBoundary())
	assert.Equal(t, beforeDate, state.DateTime)
	assert.Equal(t, beforeEffect, state.ActiveDurationEffect)
	assert.Equal(t, beforeLocation, state.MapState.PlayerLocation)
	assert.Equal(t, beforeActivePlayer, state.GetActivePlayerIndex())
	assert.Equal(t, beforeInput, scene.secondaryKeyState)
	assert.Equal(t, beforeInventory, state.PartyState.Inventory.LockerSnapshot())
	assert.Equal(t, beforeKnowledge, state.KnowledgeSnapshot())
	assert.Empty(t, callbacks.TimeAdvanced())
}

func TestCombatZStatsRoutingDoesNotFinishAction(t *testing.T) {
	state, callbacks := newBritainCombatTestState(t, 42)
	state.BeginCombatTurns(game_state.CombatTurnsPresented)
	state.ActiveDurationEffect = game_state.DurationEffect{
		EffectType: game_state.DurationEffectProtection, TurnsRemaining: 7,
	}
	callbacks.Reset()
	scene := &GameScene{
		gameState:   state,
		keyboard:    input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output:      text.NewOutput(nil, 1, 10, 80),
		spriteSheet: sharedPresentationTestSpriteSheet(),
	}
	var shown []game_state.ZStatsDossier
	state.SystemCallbacks.UI.ShowZStatsDossier = func(dossier game_state.ZStatsDossier) {
		shown = append(shown, dossier)
		scene.ShowZStatsDossier(dossier)
	}
	identityBefore := state.CombatScene.GetCurrentTurnIdentity()
	queueBefore := zStatsCombatQueueForecast(state.CombatScene.TurnQueue)
	pendingBefore := state.CombatScene.HasScheduledCombatStep()
	dateBefore := state.DateTime
	effectBefore := state.ActiveDurationEffect
	inputBefore := scene.secondaryKeyState
	locationBefore := state.MapState.PlayerLocation
	inventoryBefore := state.PartyState.Inventory.LockerSnapshot()
	spellsBefore := state.PartyState.Inventory.SpellSnapshot()
	knowledgeBefore := state.KnowledgeSnapshot()
	rosterBefore := state.CurrentPartyRosterSnapshot()

	scene.combatMapInputHandler(ebiten.KeyZ)

	require.Len(t, shown, 1)
	for _, key := range []ebiten.Key{ebiten.KeyTab, ebiten.KeyDown, ebiten.KeyPageDown, ebiten.KeyEscape} {
		scene.keyboard.SimulateKeyPress(key)
		require.True(t, scene.updateDialogs())
	}
	assert.False(t, scene.dialogStack.HasModalDialog())
	assert.Equal(t, identityBefore, state.CombatScene.GetCurrentTurnIdentity())
	assert.Equal(t, queueBefore, zStatsCombatQueueForecast(state.CombatScene.TurnQueue))
	assert.Equal(t, pendingBefore, state.CombatScene.HasScheduledCombatStep())
	assert.Equal(t, dateBefore, state.DateTime)
	assert.Equal(t, effectBefore, state.ActiveDurationEffect)
	assert.Equal(t, inputBefore, scene.secondaryKeyState)
	assert.Equal(t, locationBefore, state.MapState.PlayerLocation)
	assert.Equal(t, inventoryBefore, state.PartyState.Inventory.LockerSnapshot())
	assert.Equal(t, spellsBefore, state.PartyState.Inventory.SpellSnapshot())
	assert.Equal(t, knowledgeBefore, state.KnowledgeSnapshot())
	assert.Equal(t, rosterBefore, state.CurrentPartyRosterSnapshot())
	assert.Empty(t, callbacks.TimeAdvanced())
}

func zStatsCombatQueueForecast(queue *game_state.CombatTurnQueue) []game_state.CombatTurnForecastEntry {
	forecast := make([]game_state.CombatTurnForecastEntry, queue.TotalEntryCount()*2)
	count := queue.FillForecast(forecast)

	return forecast[:count]
}
