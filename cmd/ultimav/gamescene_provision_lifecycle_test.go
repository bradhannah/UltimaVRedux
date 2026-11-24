package main

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func seedProvisionLifecycleFeedback(t *testing.T, scene *GameScene) {
	t.Helper()
	inventory := &scene.gameState.PartyState.Inventory
	inventory.Provisions.Food.Set(10)
	inventory.Provisions.Torches.Set(2)
	scene.resetProvisionChanges()
	inventory.Provisions.Food.DecrementByOne()
	inventory.Provisions.Torches.DecrementByOne()
	scene.observeProvisionChanges()
	require.Equal(t, uint64(1), scene.provisionChanges.changes.Rows()[0].Lost)
	require.Equal(t, uint64(1), scene.provisionChanges.changes.Rows()[4].Lost)
}

func TestProvisionFeedbackContextTransitions(t *testing.T) {
	for _, scenario := range []struct {
		name       string
		transition func(*testing.T, *GameScene)
	}{
		{name: "combat entry", transition: func(t *testing.T, scene *GameScene) {
			t.Helper()
			require.NoError(t, scene.gameState.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
		}},
		{name: "town entry", transition: func(_ *testing.T, scene *GameScene) {
			scene.gameState.TransitionToLocation(references.Britain, 0, references.Position{X: 15, Y: 15}, game_state.TransitionFullLocation, true)
		}},
		{name: "camp setup", transition: func(t *testing.T, scene *GameScene) {
			t.Helper()
			require.NoError(t, scene.gameState.TransitionToCampingMap())
		}},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			state, _ := gamefixture.NewIntegrationTestBuilder(t).
				WithLocation(references.BritanniaUnderworld).WithFloor(0).WithPlayerAt(50, 50).
				WithSystemCallbacks().WithRandomSeed(1).Build()
			scene := &GameScene{gameState: state}
			seedProvisionLifecycleFeedback(t, scene)
			scenario.transition(t, scene)
			scene.observeProvisionChanges()
			assertNoSceneProvisionChanges(t, scene)
			assert.True(t, scene.provisionChanges.panel.Bounds().Empty())
			state.PartyState.Inventory.Gold.IncrementByOne()
			scene.observeProvisionChanges()
			assert.Equal(t, uint64(1), scene.provisionChanges.changes.Rows()[1].Gained, "fresh context feedback remains available")
		})
	}
}

func TestProvisionFeedbackModalBoundaryPreservesFreshActivity(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 0, 1)
	seedProvisionLifecycleFeedback(t, scene)
	modal := &blockingDebugWorkflowDialog{}
	scene.dialogStack.PushModalDialog(modal)
	state.PartyState.Inventory.Gold.IncrementBy(7)
	scene.observeProvisionChanges()
	rows := scene.provisionChanges.changes.Rows()
	assert.Empty(t, rows[0].Name)
	assert.Empty(t, rows[4].Name)
	assert.Equal(t, uint64(7), rows[1].Gained)
	// Nested dialogs are part of the same interaction, not a new context.
	child := &struct {
		blockingDebugWorkflowDialog
		distinct bool
	}{distinct: true}
	scene.dialogStack.PushModalDialog(child)
	scene.observeProvisionChanges()
	assert.Equal(t, rows, scene.provisionChanges.changes.Rows())
	state.ElapsedMs += mainscreen.ProvisionChangeHoldMs
	scene.observeProvisionChanges()
	scene.dialogStack.MustRemoveWidgetInstance(child)
	scene.dialogStack.MustRemoveWidgetInstance(modal)
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
}

func TestProvisionFeedbackReadersDoNotRestoreFrozenReceipts(t *testing.T) {
	for _, reader := range []string{"atlas", "journal"} {
		t.Run(reader, func(t *testing.T) {
			scene, state, _ := newSmashedInputTestScene(t, 0, 1)
			scene.spriteSheet = sharedPresentationTestSpriteSheet()
			now := time.Unix(100, 0)
			scene.now = func() time.Time { return now }
			seedProvisionLifecycleFeedback(t, scene)
			beforeTime, beforeRNG := state.ElapsedMs, state.GetRNG().Calls()
			if reader == "atlas" {
				scene.openAtlas(nil)
			} else {
				scene.ShowQuestJournalDossier(state.CurrentQuestJournalDossier())
			}
			assertNoSceneProvisionChanges(t, scene)
			now = now.Add(time.Hour)
			for range 10 {
				require.NoError(t, scene.Update(nil))
			}
			if reader == "atlas" {
				scene.closeAtlasSession(true)
			} else {
				scene.closeJournalSession(scene.journalSession, false)
			}
			scene.observeProvisionChanges()
			assertNoSceneProvisionChanges(t, scene)
			assert.Equal(t, beforeTime, state.ElapsedMs)
			assert.Equal(t, beforeRNG, state.GetRNG().Calls())
		})
	}
}

func TestProvisionFeedbackCombatExitFloorChangeAndOrdinaryMovement(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.BritanniaUnderworld).WithFloor(0).WithPlayerAt(50, 50).
		WithSystemCallbacks().WithRandomSeed(1).Build()
	scene := &GameScene{gameState: state}
	require.NoError(t, state.TransitionToCombatMap(0, references.BritanniaCombatTerritory, game_state.CombatEntryNorth))
	seedProvisionLifecycleFeedback(t, scene)
	state.TransitionFromCombatMap()
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	state.TransitionToLocation(references.Jhelom, 0, references.Position{X: 15, Y: 15}, game_state.TransitionFullLocation, true)
	seedProvisionLifecycleFeedback(t, scene)
	state.TransitionToLocation(references.Jhelom, 1, references.Position{X: 15, Y: 15}, game_state.TransitionFloorOnly, false)
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	seedProvisionLifecycleFeedback(t, scene)
	before := scene.provisionChanges.changes.Rows()
	state.TransitionToLocation(references.Jhelom, 1, references.Position{X: 16, Y: 15}, game_state.TransitionPositionOnly, false)
	scene.observeProvisionChanges()
	assert.Equal(t, before, scene.provisionChanges.changes.Rows(), "ordinary movement preserves the receipt")
}

func TestProvisionFeedbackScriptedSuppressionAndTeleport(t *testing.T) {
	scene, state, _ := newSmashedInputTestScene(t, 0, 1)
	seedProvisionLifecycleFeedback(t, scene)
	scene.HideAllOverlays()
	assertNoSceneProvisionChanges(t, scene)
	state.PartyState.Inventory.Gold.IncrementByOne()
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	scene.ShowAllOverlays()
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	state.PartyState.Inventory.Gold.IncrementByOne()
	scene.observeProvisionChanges()
	require.Equal(t, uint64(1), scene.provisionChanges.changes.Rows()[1].Gained)
	scene.resetTeleportationSourceRenderer()
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	seedProvisionLifecycleFeedback(t, scene)
	scene.setMapFadeLevel(0.5)
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
	state.PartyState.Inventory.Gold.IncrementByOne()
	scene.observeProvisionChanges()
	scene.setMapFadeLevel(0)
	scene.observeProvisionChanges()
	assertNoSceneProvisionChanges(t, scene)
}
