package main

import (
	"fmt"
	"image"
	"runtime"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/conversation"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func TestTalkResizeKeepsDisplayedExchangeAndExcerptIdentity(t *testing.T) {
	setTalkHistoryTestResolution(t, 1920, 1080)
	scene := &GameScene{gameState: &game_state.GameState{}, keyboard: newPresentationTestKeyboard(),
		activePlaythrough: playthroughMetadata{id: "0123456789abcdef0123456789abcdef"},
		lastConversation: journal.Conversation{Exchanges: []journal.Exchange{
			{ID: "first", Speaker: "Iolo", Location: "Britain", Passages: []journal.Passage{{Text: "First clue"}}},
			{ID: "second", Speaker: "Iolo", Location: "Britain", Passages: []journal.Passage{{Text: "Second clue"}}},
		}}}
	dialog := &LinearTalkDialog{gameScene: scene, historyExchange: 0}
	dialog.initializeResizeableVisualElements()
	dialog.Output.AddText("First clue")
	dialog.Output.AddText("Second clue")
	dialog.showJournalHistoryExchange()
	display.GetManager().PublishLogicalSize(1280, 720)
	dialog.ensureTalkLayout()
	require.Contains(t, dialog.historyPanel.GetOutputStr(true), "First clue")
	require.NotContains(t, dialog.historyPanel.GetOutputStr(true), "Second clue")
	dialog.keepPresentedExchange()
	excerpts, err := scene.loadExcerpts()
	require.NoError(t, err)
	require.Len(t, excerpts, 1)
	require.Equal(t, "first", excerpts[0].ID)
}

func TestTalkHistoryExchangeSizesPanelFromContent(t *testing.T) {
	setTalkHistoryTestResolution(t, 1920, 1080)
	passage := strings.Repeat("a\n", 200) + strings.Repeat("x", 300) + " last words"
	scene := &GameScene{gameState: &game_state.GameState{}, keyboard: newPresentationTestKeyboard(),
		lastConversation: journal.Conversation{Exchanges: []journal.Exchange{
			{ID: "long", Speaker: "Iolo", Location: "Britain", Passages: []journal.Passage{{Text: passage}}},
		}}}
	dialog := &LinearTalkDialog{gameScene: scene}
	dialog.initializeResizeableVisualElements()
	var before, after runtime.MemStats
	runtime.ReadMemStats(&before)
	dialog.showJournalHistoryExchange()
	runtime.ReadMemStats(&after)
	// A panel sized by the 256 KiB transcript budget allocated about 17 MB per page turn.
	require.Less(t, after.TotalAlloc-before.TotalAlloc, uint64(1<<20))
	shown := dialog.historyPanel.GetOutputStr(true)
	require.Contains(t, shown, "Iolo")
	require.Contains(t, shown, "last words")
}

func TestTalkHistoryResizePreservesLiveInputPauseAndKeyWait(t *testing.T) {
	for _, mode := range []string{"input", "pause", "keywait"} {
		t.Run(mode, func(t *testing.T) {
			setTalkHistoryTestResolution(t, 3840, 2160)
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithRandomSeed(42).WithSystemCallbacks().Build()
			held := make(map[ebiten.Key]bool)
			keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
			scene := &GameScene{gameState: state, keyboard: keyboard}
			callbacks := NewGameActionCallbacks(scene, references.NPCReference{Location: references.Britain}, nil)
			// No engine is installed: history must never invoke a continuation.
			dialog := &LinearTalkDialog{gameScene: scene, callbacks: callbacks, waitingForInput: true}
			dialog.initializeResizeableVisualElements()
			scene.PushDialog(dialog)
			state.PartyState.Inventory.Provisions.SkullKeys.Set(0)
			dialog.outputSegmentsToDialog([]conversation.TextSegment{{Text: "EARLY RUNIC CLUE", IsRunic: true}}, false)
			callbacks.GiveItem(75)
			for index := range 35 {
				dialog.outputTextToDialog(fmt.Sprintf("Already shown speech %02d", index), false)
			}
			dialog.TextInput.SetText("YOUR NA")
			state.ElapsedMs = 1000
			dialog.isPausing = mode == "pause"
			dialog.pauseStartMs = 500
			dialog.waitingForKeypressContinue = mode == "keywait"
			dialog.pendingOutput = "NOT DISPLAYED YET"
			dialog.pendingSegments = []conversation.TextSegment{{Text: "PENDING RUNIC", IsRunic: true}}
			callbacks.ShowOutput("UNDISPLAYED CALLBACK")
			before, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			press := func(key ebiten.Key) {
				held[key] = true
				dialog.Update()
				delete(held, key)
				keyboard.ClearFrameConsumption()
				dialog.Update()
			}
			press(ebiten.KeyHome)
			require.NotNil(t, dialog.historyPanel)
			content := dialog.historyPanel.GetOutputStr(true)
			require.Contains(t, content, "EARLY RUNIC CLUE")
			require.Contains(t, content, "Received: Skull Keys")
			require.NotContains(t, content, "NOT DISPLAYED")
			require.NotContains(t, content, "PENDING RUNIC")
			require.NotContains(t, content, "UNDISPLAYED CALLBACK")
			press(ebiten.KeyPageDown)
			require.Greater(t, dialog.historyRow, 0)
			press(ebiten.KeyPageUp)
			require.Zero(t, dialog.historyRow)
			press(ebiten.KeyEnter)
			state.ElapsedMs += 5000
			for _, size := range []image.Point{{1280, 720}, {3840, 2160}} {
				display.GetManager().PublishLogicalSize(size.X, size.Y)
				dialog.Update()
				require.Contains(t, dialog.historyPanel.GetOutputStr(true), "EARLY RUNIC CLUE")
				require.InDelta(t, float64(size.X)/1920*float64(talkFontSize), dialog.font.TextFace.Size, .01)
				require.Equal(t, "YOUR NA", dialog.TextInput.GetText())
			}
			held[ebiten.KeyEscape] = true
			dialog.Update()
			require.Nil(t, dialog.historyPanel)
			require.True(t, keyboard.IsWaitingForRelease())
			require.Equal(t, "YOUR NA", dialog.TextInput.GetText())
			require.Equal(t, mode == "pause", dialog.isPausing)
			require.Equal(t, mode == "keywait", dialog.waitingForKeypressContinue)
			require.Equal(t, "NOT DISPLAYED YET", dialog.pendingOutput)
			require.Equal(t, "UNDISPLAYED CALLBACK", callbacks.GetAccumulatedOutput())
			if mode == "pause" {
				require.EqualValues(t, 500, state.ElapsedMs-dialog.pauseStartMs)
			}
			after, err := state.CaptureSaveSnapshot()
			require.NoError(t, err)
			require.Equal(t, before.Random, after.Random)
			require.Equal(t, before.Party, after.Party)
			require.EqualValues(t, 1, state.PartyState.Inventory.Provisions.SkullKeys.Get())
		})
	}
}

func TestTalkSameInstanceLiveReflowRetainsTranscriptAndInput(t *testing.T) {
	setTalkHistoryTestResolution(t, 3840, 2160)
	scene := &GameScene{gameState: &game_state.GameState{}, keyboard: newPresentationTestKeyboard()}
	dialog := &LinearTalkDialog{gameScene: scene}
	dialog.initializeResizeableVisualElements()
	dialog.outputSegmentsToDialog([]conversation.TextSegment{{Text: "Runic", IsRunic: true}}, false)
	dialog.outputTextToDialog(" continuation", true)
	dialog.TextInput.SetText("PARTIAL")
	output, inputWidget := dialog.Output, dialog.TextInput
	for _, size := range []image.Point{{1280, 720}, {3840, 2160}} {
		display.GetManager().PublishLogicalSize(size.X, size.Y)
		dialog.ensureTalkLayout()
		require.Same(t, output, dialog.Output)
		require.Same(t, inputWidget, dialog.TextInput)
		require.Contains(t, dialog.Output.GetOutputStr(true), "Runic continuation")
		require.Equal(t, "PARTIAL", dialog.TextInput.GetText())
	}
}

func setTalkHistoryTestResolution(t *testing.T, width, height int) {
	t.Helper()
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	manager.PublishLogicalSize(width, height)
}

func TestTalkTruncatedHistoryReturnAndResizeKeepVisibleSpeech(t *testing.T) {
	for _, pausing := range []bool{false, true} {
		t.Run(fmt.Sprint(pausing), func(t *testing.T) {
			setTalkHistoryTestResolution(t, 1280, 720)
			held := make(map[ebiten.Key]bool)
			keyboard := input.NewKeyboardWithKeyStateSource(-1, func(key ebiten.Key) bool { return held[key] })
			state := &game_state.GameState{ElapsedMs: 1000}
			dialog := &LinearTalkDialog{gameScene: &GameScene{gameState: state, keyboard: keyboard}}
			dialog.initializeResizeableVisualElements()
			dialog.Output.AddTextRaw("Visible prefix ")
			for range 300 {
				dialog.Output.AppendText("x")
			}
			visible := dialog.Output.GetOutputStr(true)
			require.Contains(t, visible, "Visible prefix")
			dialog.TextInput.SetText("PARTIAL INPUT")
			dialog.isPausing, dialog.pauseStartMs = pausing, 500
			dialog.pendingOutput = "FUTURE SPEECH"
			held[ebiten.KeyHome] = true
			dialog.Update()
			require.Contains(t, dialog.historyPanel.GetOutputStr(true), "Earlier conversation text truncated")
			require.NotContains(t, dialog.historyPanel.GetOutputStr(true), "FUTURE SPEECH")
			delete(held, ebiten.KeyHome)
			keyboard.ClearFrameConsumption()
			state.ElapsedMs += 5000
			held[ebiten.KeyEscape] = true
			dialog.Update()
			require.Nil(t, dialog.historyPanel)
			for _, size := range []image.Point{{3840, 2160}, {1280, 720}} {
				display.GetManager().PublishLogicalSize(size.X, size.Y)
				dialog.ensureTalkLayout()
				require.Equal(t, visible, dialog.Output.GetOutputStr(true))
				require.Equal(t, "PARTIAL INPUT", dialog.TextInput.GetText())
				require.Equal(t, "FUTURE SPEECH", dialog.pendingOutput)
				require.Equal(t, pausing, dialog.isPausing)
			}
			if pausing {
				require.EqualValues(t, 500, state.ElapsedMs-dialog.pauseStartMs)
			}
		})
	}
}
