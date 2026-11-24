package main

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/stretchr/testify/require"
)

// Focused tests update between commands, with no blocker released inside a frame.
func (g *GameScene) updateJournalNotice() { g.updateJournalNoticeAfterFrame(false) }

func newMapNoticeScene(t *testing.T) (*GameScene, *time.Time) {
	t.Helper()
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).WithSystemCallbacks().Build()
	now := time.Unix(100, 0)
	scene := &GameScene{gameState: state, gameConfig: state.GameConfig, keyboard: input.NewKeyboardWithKeyStateSource(0, noSceneTestKeyHeld),
		output: text.NewOutput(nil, 1, 64, 80), now: func() time.Time { return now }}
	scene.output.EnableJournalHistory()
	return scene, &now
}

func TestMapNoticeCoalescesSameSiteWaitsForArrivalAndSequencesJournal(t *testing.T) {
	scene, _ := newMapNoticeScene(t)
	arrival := &countingDialog{}
	scene.dialogStack.PushModalDialog(arrival)
	scene.output.AddText("Enter Fogsbane.")
	scene.journalChanged(journal.ChronicleTab, 1)
	scene.updateJournalNotice() // Journal was queued in an earlier blocked frame.
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "site-one", Name: "Unnamed lighthouse", Kind: "lighthouse", Discovered: true})
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "site-one", Name: "Fogsbane", Kind: "lighthouse", Named: true, Surveyed: true, Measured: true})
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "site-two", Name: "Unnamed hut", Kind: "hut", Discovered: true})
	scene.journalChanged(journal.VirtuesTab, 0)
	scene.updateJournalNotice()
	require.Len(t, scene.journalNotice.queue.pending, 3)
	require.Equal(t, "Enter Fogsbane.", scene.output.JournalHistory())
	scene.dialogStack.MustRemoveWidgetInstance(arrival)
	scene.updateJournalNoticeAfterFrame(true)
	require.Equal(t, "Enter Fogsbane.", scene.output.JournalHistory(), "arrival dismissal retains its frame")
	scene.updateJournalNotice()
	expected := "Enter Fogsbane.\nMap updated: Fogsbane and its coordinates recorded.\n[Shift+M] View map\n" +
		"Map updated: Unnamed hut recorded. Coordinates not measured.\n[Shift+M] View map\nJournal updated.\n[Shift+J] View journal"
	require.Equal(t, expected, scene.output.JournalHistory())
	require.Empty(t, scene.journalNotice.queue.pending)
	scene.updateJournalNotice()
	require.Equal(t, expected, scene.output.JournalHistory(), "ordinary updates cannot replay feedback")
	require.NoError(t, scene.ValidateSaveSnapshotEligibility())
	require.False(t, scene.inputBlocked)
	require.Zero(t, scene.dialogStack.Count())
	require.True(t, scene.canOpenAtlas())
}

func TestMapNoticeDefersForEveryBlockingOwner(t *testing.T) {
	for _, blocker := range []string{"modal", "workflow", "fade", "input", "atlas", "journal", "direction"} {
		t.Run(blocker, func(t *testing.T) {
			scene, now := newMapNoticeScene(t)
			modal := &countingDialog{}
			switch blocker {
			case "modal":
				scene.dialogStack.PushModalDialog(modal)
			case "workflow":
				scene.gameState.EventQueueManager.Workflow("notice-test").Priority(event_queue.PriorityNormal).Sequential().First(func() {}).Wait(10000).Start()
			case "fade":
				scene.mapFadeLevel = 0.5
			case "input":
				scene.inputBlocked = true
			case "atlas":
				scene.atlasSession = &atlasSession{}
			case "journal":
				scene.journalSession = &journalSession{}
			case "direction":
				scene.secondaryKeyState = OpenDirectionInput
			}
			scene.atlasUpdated(journal.AtlasChange{EntryKey: "place", Name: "Fogsbane", Surveyed: true, Measured: true})
			scene.updateJournalNotice()
			*now = now.Add(time.Hour)
			scene.updateJournalNotice()
			require.Empty(t, scene.output.JournalHistory())
			scene.dialogStack.RemoveWidgetInstance(modal)
			if blocker == "workflow" {
				scene.gameState.EventQueueManager.Shutdown()
			}
			scene.mapFadeLevel, scene.inputBlocked = 0, false
			scene.atlasSession, scene.journalSession = nil, nil
			scene.secondaryKeyState = PrimaryInput
			scene.updateJournalNotice()
			scene.updateJournalNotice()
			require.Equal(t, 1, strings.Count(scene.output.JournalHistory(), "Map updated:"))
		})
	}
}

func TestMapNoticeDoesNotStealSecondaryPromptAppendTarget(t *testing.T) {
	scene, _ := newMapNoticeScene(t)
	scene.output.AddCommandPrompt("Open: ")
	scene.secondaryKeyState = OpenDirectionInput
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "place", Name: "Fogsbane", Named: true})
	scene.journalChanged(journal.ChronicleTab, 1)
	scene.updateJournalNotice()
	scene.output.AppendText("North")
	scene.secondaryKeyState = PrimaryInput
	scene.updateJournalNoticeAfterFrame(true)
	require.Contains(t, scene.output.JournalHistory(), "Open: North")
	require.NotContains(t, scene.output.JournalHistory(), "Map updated")
	scene.updateJournalNotice()
	require.Contains(t, scene.output.JournalHistory(), "Open: North\nMap updated:")
	require.Equal(t, 1, strings.Count(scene.output.JournalHistory(), "Journal updated."))
}

func TestMapNoticeAtlasEarlyReturnFramesAndTeardown(t *testing.T) {
	scene, _ := newMapNoticeScene(t)
	scene.openAtlas(nil)
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "place", Name: "Fogsbane", Surveyed: true, Measured: true})
	require.NoError(t, scene.Update(nil))
	require.Empty(t, scene.output.JournalHistory())
	scene.closeAtlasSession(true)
	scene.updateJournalNotice()
	require.Contains(t, scene.output.JournalHistory(), "Coordinates added for Fogsbane.")
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "next", Name: "Other", Discovered: true})
	scene.Dispose()
	require.Equal(t, journalNotice{}, scene.journalNotice)
	scene.atlasUpdated(journal.AtlasChange{EntryKey: "late", Discovered: true})
	require.Equal(t, journalNotice{}, scene.journalNotice)
}

func TestMapNoticeNormalOutputLongNamesAndSemanticHints(t *testing.T) {
	for _, size := range []image.Point{{1280, 720}, {1366, 768}, {1920, 1080}, {2560, 1440}, {3840, 2160}} {
		t.Run(size.String(), func(t *testing.T) {
			scene, _ := newMapNoticeScene(t)
			bounds := logicalGameBounds(size.X, size.Y)
			font, output := newMainOutput(bounds.Dx())
			scene.output = output
			name := "A very long legitimately learned lighthouse name"
			scene.atlasUpdated(journal.AtlasChange{EntryKey: "place", Name: name, Named: true})
			scene.updateJournalNotice()
			require.Equal(t, "Map updated: "+name+" identified. Coordinates not measured.\n[Shift+M] View map", output.JournalHistory())
			for _, row := range strings.Split(output.GetOutputStr(false), "\n") {
				width, _ := font.MeasureString(row)
				require.LessOrEqual(t, width, float64(calculateRightRailLayout(bounds).outputContent.Dx()))
			}
			_, key := output.FirstPanelLineWithColor(0, maxLinesForOutput, widgets.DialogControlKeyColor)
			_, hint := output.FirstPanelLineWithColor(0, maxLinesForOutput, widgets.DialogControlHintColor)
			_, label := output.FirstPanelLineWithColor(0, maxLinesForOutput, widgets.DialogLabelColor)
			require.True(t, key && hint && label)
		})
	}
}
