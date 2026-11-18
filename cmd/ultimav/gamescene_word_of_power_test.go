package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/conversation/semantic"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestGoethSerializedWordOfPowerDefersUntilPresented(t *testing.T) {
	// Synthetic control boundaries around the exact spoken/runic clue exercise
	// the production scalar loader, engine, and mounted dialog without original files.
	const document = `schema: ultima5redux.conversation/v1
protocol: ultima5
id: redux.goeth_word_boundary_test
origin: {kind: redux}
name: Goeth
description: A speaker waits.
greeting: Welcome.
job: I remember words.
farewell: Farewell.
topics:
  - keywords: [drow]
    response:
      - ask: question_01
questions:
  question_01:
    prompt: Which dungeon?
    answers:
      - keywords: [drat]
        response:
          - say: 'Drow of Rewop thou seeketh; remember '
          - pause
          - say:
              - runic: AIPONI
              - text: ' htod I.'
          - discover_knowledge: dungeon.destard.word_of_power
          - return_to_topics
    default: Remember ton I.
`
	definition, err := semantic.Load("synthetic-goeth.yaml", strings.NewReader(document))
	require.NoError(t, err)
	for _, cancel := range []bool{false, true} {
		name := "present"
		if cancel {
			name = "cancel unseen tail"
		}
		t.Run(name, func(t *testing.T) {
			dialog := newBoundaryTalkDialog(t, definition)
			scene := dialog.gameScene
			scene.PushDialog(dialog)
			for _, value := range []string{"drow", "drat"} {
				dialog.currentResponse = dialog.engine.ProcessInput(value)
				dialog.processCurrentResponse()
			}
			require.NoError(t, dialog.currentResponse.Error)
			require.True(t, dialog.isPausing)
			require.Contains(t, dialog.currentResponse.Output, "AIPONI")
			require.NotContains(t, dialog.Output.GetOutputStr(true), "AIPONI")
			before, err := scene.gameState.CaptureSaveSnapshot()
			require.NoError(t, err)
			require.Empty(t, before.Redux.PlayerKnowledge)
			require.Empty(t, scene.gameState.CurrentQuestJournalDossier().Chronicle)
			require.Empty(t, scene.gameState.CurrentQuestJournalDossier().Dungeons)
			require.False(t, scene.journalNotice.pending)
			dialog.Refresh()
			if cancel {
				dialog.Close()
				dialog.finishCurrentPause()
				after, err := scene.gameState.CaptureSaveSnapshot()
				require.NoError(t, err)
				require.Equal(t, before, after)
				require.False(t, scene.journalNotice.pending)
				return
			}
			dialog.callbacks.SetNPCName("Changed after execution")
			dialog.finishCurrentPause()
			require.Contains(t, dialog.Output.GetOutputStr(true), "AIPONI")
			entry, learned := scene.gameState.WordOfPowerKnowledge("destard")
			require.True(t, learned)
			require.Equal(t, game_state.DialogueKnowledgeSource("Goeth"), entry.Source)
			notes := scene.gameState.CurrentQuestJournalDossier().Chronicle
			require.Len(t, notes, 1)
			require.Equal(t, "Goeth spoke the runes AIPONI for Destard.", notes[0].Summary)
			require.Contains(t, notes[0].Title+notes[0].Summary, "Destard")
			require.NotContains(t, notes[0].Title+notes[0].Summary, "INOPIA")
			dungeons := scene.gameState.CurrentQuestJournalDossier().Dungeons
			require.Len(t, dungeons, 1)
			require.Equal(t, "Destard", dungeons[0].Name)
			require.Equal(t, "AIPONI", dungeons[0].Word)
			require.True(t, dungeons[0].WordIsClue)
			require.True(t, scene.journalNotice.pending)
			after, err := scene.gameState.CaptureSaveSnapshot()
			require.NoError(t, err)
			require.Len(t, after.Redux.PlayerKnowledge, 1)
			require.Equal(t, "destard.word_of_power", after.Redux.PlayerKnowledge[0].Key)
			revision := scene.journalVisits.Revision
			scene.updateJournalNotice()
			for _, value := range []string{"drow", "drat"} {
				dialog.currentResponse = dialog.engine.ProcessInput(value)
				dialog.processCurrentResponse()
			}
			dialog.finishCurrentPause()
			require.Equal(t, notes, scene.gameState.CurrentQuestJournalDossier().Chronicle)
			require.Equal(t, revision, scene.journalVisits.Revision)
			require.False(t, scene.journalNotice.pending)
		})
	}
}

func TestCodexWordOfPowerAfterWordPageAndOnCompletedRevisit(t *testing.T) {
	for _, completed := range []bool{false, true} {
		name := "final quest completion"
		if completed {
			name = "already complete checkpoint"
		}
		t.Run(name, func(t *testing.T) {
			scene := newCodexWordOfPowerTestScene(t, completed)
			var firstEntry game_state.KnowledgeEntry
			for visit := range 2 {
				scene.StartCodexWorkflow()
				_, known := scene.gameState.WordOfPowerKnowledge("doom")
				require.Equal(t, visit > 0, known, "building the visit must not reveal a word")
				dismissNextShrineOverlay(t, scene) // The book opens.
				if visit == 0 && !completed {
					answer := dismissNextShrineOverlay(t, scene)
					require.NotContains(t, answer, "VERAMOCOR")
					require.True(t, scene.gameState.AreAllQuestsComplete())
					_, known = scene.gameState.WordOfPowerKnowledge("doom")
					require.False(t, known, "authoritative quest completion is not word revelation")
				}
				scene.updateJournalNotice()
				openingRevision := scene.journalVisits.Revision
				for index, message := range game_state.CodexRevelationMessages {
					dialog := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
					require.True(t, dialog.IsVisible())
					require.Equal(t, message, dialog.GetMessage(), "retain the verified Codex text verbatim")
					if index == 3 {
						require.Contains(t, dialog.GetContentText(), "VERAMOCOR")
					}
					_, known = scene.gameState.WordOfPowerKnowledge("doom")
					require.Equal(t, visit > 0 || index == 4, known, "discovery follows the acknowledged word-bearing page")
					gates := scene.gameState.CurrentQuestJournalDossier().Dungeons
					if known {
						require.Len(t, gates, 1, "the Final Gate row follows word discovery")
						require.Equal(t, visit > 0, strings.Contains(gates[0].Description, "awaits thy coming"))
					} else {
						require.Empty(t, gates, "no Final Gate row before the word page")
					}
					require.Equal(t, visit == 0 && index == 4, scene.journalNotice.pending)
					require.Equal(t, visit > 0, scene.gameState.CodexRevelationComplete,
						"the final paragraph is not recorded until it is acknowledged")
					dialog.Dismiss()
				}
				drainSceneWorkflow(t, scene, "codex_reading", 1000)
				entry, known := scene.gameState.WordOfPowerKnowledge("doom")
				require.True(t, known)
				require.True(t, scene.gameState.CodexRevelationComplete)
				require.Equal(t, game_state.RevelationKnowledgeSource("Codex of Ultimate Wisdom"), entry.Source)
				if visit == 0 {
					firstEntry = entry
					require.Equal(t, openingRevision+2, scene.journalVisits.Revision)
				} else {
					require.Equal(t, firstEntry, entry)
					require.Equal(t, openingRevision, scene.journalVisits.Revision)
				}
			}
			wordNotes := 0
			for _, event := range scene.gameState.QuestJournalEvents() {
				if event.Subject == "dungeon.doom.word_of_power" {
					wordNotes++
				}
			}
			require.Equal(t, 1, wordNotes)
			gates := scene.gameState.CurrentQuestJournalDossier().Dungeons
			require.Len(t, gates, 1, "repeated visits keep one Final Gate row")
			require.Equal(t, "Final Gate", gates[0].Name)
		})
	}
}

func TestCodexWordOfPowerCancelledBeforeWordPageRemainsUnknown(t *testing.T) {
	scene := newCodexWordOfPowerTestScene(t, true)
	scene.StartCodexWorkflow()
	dismissNextShrineOverlay(t, scene) // The book opens.
	for _, message := range game_state.CodexRevelationMessages[:2] {
		dialog := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
		require.Equal(t, message, dialog.GetMessage())
		dialog.Dismiss()
	}
	dialog := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
	require.Equal(t, game_state.CodexRevelationMessages[2], dialog.GetMessage())
	scene.Dispose()
	dialog.Dismiss()
	scene.gameState.EventQueueManager.Update()
	_, known := scene.gameState.WordOfPowerKnowledge("doom")
	require.False(t, known)
	require.Empty(t, scene.gameState.QuestJournalEvents())
	require.False(t, scene.journalNotice.pending)
	require.False(t, scene.gameState.CodexRevelationComplete)
}

func TestCodexFinalPageInterruptionRetainsOnlyWordUntilRevisit(t *testing.T) {
	scene := newCodexWordOfPowerTestScene(t, true)
	scene.StartCodexWorkflow()
	dismissNextShrineOverlay(t, scene)
	for range game_state.CodexRevelationMessages[:4] {
		dismissNextShrineOverlay(t, scene)
	}
	dialog := waitForTopModalWithin[*widgets.OverlayDialog](t, scene, 1000, 200)
	require.Equal(t, game_state.CodexRevelationMessages[4], dialog.GetMessage())
	word, known := scene.gameState.WordOfPowerKnowledge("doom")
	require.True(t, known)
	require.False(t, scene.gameState.CodexRevelationComplete)
	before := scene.gameState.QuestJournalEvents()
	require.Len(t, before, 1)
	require.NotContains(t, scene.gameState.CurrentQuestJournalDossier().Chronicle[0].Summary, "awaits")
	require.NotContains(t, scene.gameState.CurrentQuestJournalDossier().Dungeons[0].Description, "awaits")
	scene.Dispose()
	dialog.Dismiss()
	scene.gameState.EventQueueManager.Update()
	require.False(t, scene.gameState.CodexRevelationComplete)

	// Continue through the real checkpoint hydration path; a disposed event-queue
	// manager is intentionally terminal and cannot be reused by a new scene.
	snapshot, err := scene.gameState.CaptureSaveSnapshot()
	require.NoError(t, err)
	// The workflow fixture advances queue time without running environmental
	// frames. Supply the current wind phase before exercising native hydration.
	snapshot.Wind.NextTickMs = snapshot.World.ElapsedMs + environment.WindTickIntervalMs
	restored, err := game_state.NewGameStateFromSaveSnapshot(snapshot, scene.gameState.GameConfig,
		scene.gameState.GameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
	require.NoError(t, err)
	restored.SystemCallbacks = game_state.NewMockSystemCallbacks(t).ToSystemCallbacks()
	revisit := &GameScene{gameState: restored, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), output: text.NewOutput(nil, 1, 64, 80)}
	revisit.gameState.SystemCallbacks.UI.JournalChanged = revisit.journalChanged
	t.Cleanup(revisit.Dispose)
	revisit.journalVisits.Record(journal.ChronicleTab, 1)
	revisit.journalVisits.Acknowledge(revisit.journalVisits.Revision, [3]bool{true, false, false})
	require.False(t, revisit.journalVisits.IsNew(journal.ChronicleTab, 1))
	revisit.StartCodexWorkflow()
	dismissNextShrineOverlay(t, revisit)
	for range game_state.CodexRevelationMessages {
		dismissNextShrineOverlay(t, revisit)
	}
	drainSceneWorkflow(t, revisit, "codex_reading", 1000)
	require.True(t, revisit.gameState.CodexRevelationComplete)
	require.Equal(t, before, revisit.gameState.QuestJournalEvents())
	after, known := revisit.gameState.WordOfPowerKnowledge("doom")
	require.True(t, known)
	require.Equal(t, word, after)
	require.True(t, revisit.journalVisits.IsNew(journal.ChronicleTab, 1))
	entry := revisit.gameState.CurrentQuestJournalDossier().Chronicle[0]
	require.Equal(t, "Codex revelation", entry.Title)
	require.Equal(t, "Codex of Ultimate Wisdom", entry.Source)
	require.Equal(t, before[0].OccurredOn, entry.OccurredOn)
	for _, clue := range []string{"Shame's egress", "Underworld", "darkness", "gate", "VERAMOCOR", "ethereal wards", "stealers of souls", "awaits thy coming"} {
		require.Contains(t, entry.Summary, clue)
	}
	require.NotContains(t, entry.Title+entry.Summary, "Doom")
	gate := revisit.gameState.CurrentQuestJournalDossier().Dungeons[0]
	require.Equal(t, entry.Summary, gate.Description, "both views share the retained revelation")
	require.Equal(t, entry.Source, gate.Source)
}

func newCodexWordOfPowerTestScene(t *testing.T, completed bool) *GameScene {
	t.Helper()
	state := newGameActionCallbacksKnowledgeState(t)
	// Four padded 11x16 maps are sufficient for the real Codex workflow. These
	// synthetic floor tiles contain no original assets and need no external data.
	directory := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(directory, "MISCMAPS.DAT"), bytes.Repeat([]byte{byte(indexes.BrickFloor)}, 4*11*16), 0o600))
	state.GameConfig.SavedConfigData = &config.UltimaVConfigurationFlags{DataFilePath: directory}
	state.GameReferences.TileReferences = references.NewTileReferences()
	for index := range game_state.TotalShrines {
		state.SetShrineQuestCompleted(index, completed || index < game_state.TotalShrines-1)
		state.SetShrineQuestActive(index, !completed && index == game_state.TotalShrines-1)
	}
	scene := &GameScene{gameState: state, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld), output: text.NewOutput(nil, 1, 64, 80)}
	state.SystemCallbacks.UI.JournalChanged = scene.journalChanged
	t.Cleanup(scene.Dispose)
	return scene
}
