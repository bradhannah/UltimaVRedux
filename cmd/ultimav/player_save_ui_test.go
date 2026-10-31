package main

import (
	"errors"
	"fmt"
	"image"
	"io/fs"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

func TestPlayerSaveBrowserModelLoadEnablesOnlyReadySlots(t *testing.T) {
	summaries := playerSaveBrowserTestSummaries()

	model := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserLoad, "")

	require.Len(t, model.views, len(summaries))
	for index, view := range model.views {
		assert.Equal(t, summaries[index].Status == player_save.SlotReady, view.PrimaryEnabled)
	}
}

func TestPlayerSaveBrowserModelWriteEnablesSafeDestinations(t *testing.T) {
	summaries := playerSaveBrowserTestSummaries()

	model := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserWrite, testPlaythroughID)

	for index, view := range model.views {
		expected := summaries[index].Status == player_save.SlotEmpty ||
			summaries[index].Status == player_save.SlotReady ||
			summaries[index].Status == player_save.SlotIncompatible
		assert.Equal(t, expected, view.PrimaryEnabled)
	}
}

func TestPlayerSaveBrowserModelLoadSelectsNewestReadySlot(t *testing.T) {
	older := time.Date(2026, time.August, 10, 12, 0, 0, 0, time.UTC)
	newer := older.Add(time.Hour)
	summaries := []player_save.SlotSummary{
		{Slot: player_save.Slot01, Status: player_save.SlotReady, Metadata: player_save.MetadataSummary{CreatedAt: older, PlaythroughID: testPlaythroughID}},
		{Slot: player_save.Slot02, Status: player_save.SlotReady, Metadata: player_save.MetadataSummary{CreatedAt: newer, PlaythroughID: testPlaythroughID}},
		{Slot: player_save.Slot03, Status: player_save.SlotReady, Metadata: player_save.MetadataSummary{CreatedAt: newer, PlaythroughID: testPlaythroughID}},
	}

	model := buildPlayerSaveBrowserModel(summaries, playerSaveBrowserLoad, "")

	assert.Equal(t, string(player_save.Slot02), model.initialSelectionID)
}

func TestPlayerSaveBrowserModelPreservesReviewedRevision(t *testing.T) {
	summary := player_save.SlotSummary{
		Slot:     player_save.Slot03,
		Status:   player_save.SlotReady,
		Revision: "reviewed-revision",
	}
	model := buildPlayerSaveBrowserModel([]player_save.SlotSummary{summary}, playerSaveBrowserLoad, "")

	got, ok := model.summary(model.views[0])

	require.True(t, ok)
	assert.Equal(t, summary, got)
}

func TestPlayerSaveBrowserModelShowsProblems(t *testing.T) {
	summary := player_save.SlotSummary{
		Slot:    player_save.Slot04,
		Status:  player_save.SlotCorrupt,
		Problem: "invalid snapshot",
	}

	model := buildPlayerSaveBrowserModel([]player_save.SlotSummary{summary}, playerSaveBrowserLoad, "")

	require.Len(t, model.views, 1)
	assert.Equal(t, "Corrupt", model.views[0].Status)
	assert.NotContains(t, model.views[0].Problem, summary.Problem)
	assert.Contains(t, model.views[0].Problem, "back up slot-04.json")
	assert.Contains(t, model.views[0].Problem, "Reopen the game to use the empty slot.")
	assert.Contains(t, model.views[0].Problem, "slot-04.json")
}

func TestPlayerSaveFailureMessagesUseTypedErrors(t *testing.T) {
	for _, action := range []string{"Save", "Load", "Quick save", "Import"} {
		t.Run(action, func(t *testing.T) {
			stale := &player_save.StaleRevisionError{Slot: player_save.Slot04, Expected: "old", Actual: "new"}
			err := fmt.Errorf("private diagnostic: %w", stale)
			assert.Equal(t, "This save changed since you opened the screen. Review the slot and try again.", playerSaveFailureMessage(action, err))
			for _, test := range []struct {
				err  error
				want string
			}{
				{fs.ErrPermission, "Check file permissions"},
				{player_save.ErrSlotNotFound, "could not be found"},
				{player_save.ErrIncompatible, "compatible game version"},
				{ErrGameSceneSnapshotNotRepresentable, "Finish the current action"},
				{ErrNoActivePlayerSaveSlot, "Save or load a player slot first"},
				{errors.New("secret path: low-level failure"), "check the game log"},
			} {
				message := playerSaveFailureMessage(action, fmt.Errorf("private diagnostic: %w", test.err))
				assert.Contains(t, message, test.want)
				assert.NotContains(t, message, "private diagnostic")
				assert.NotContains(t, message, "secret path")
			}
		})
	}
}

func TestQuickLoadErrorsDescribeLoadRecoveryWithoutBrowserOrSaveClaims(t *testing.T) {
	for _, test := range []struct {
		err  error
		want string
	}{
		{ErrNoActivePlayerSaveSlot, "Save or load a player slot first"},
		{player_save.ErrStaleRevision, "Use Load to review"},
		{player_save.ErrSlotNotFound, "Use Load to review"},
		{save_snapshot.ErrInvalidEnvelope, "could not be validated"},
		{save_snapshot.ErrInvalidSnapshot, "could not be validated"},
		{player_save.ErrIncompatible, "compatible game"},
		{ErrPlayerSaveUnavailable, "Load services are unavailable"},
		{fs.ErrPermission, "Check file permissions"},
	} {
		message := playerQuickLoadErrorDetail(fmt.Errorf("private-path: %w", test.err))
		assert.Contains(t, message, test.want)
		assert.NotContains(t, message, "since you opened")
		assert.NotContains(t, message, "Saving is not available")
		assert.NotContains(t, message, "private-path")
	}
}

func TestPlayerSaveIntroNoticesFit(t *testing.T) {
	for _, size := range []image.Point{{X: 320, Y: 200}, {X: 1920, Y: 1080}} {
		setTalkHistoryTestResolution(t, size.X, size.Y)
		font := text.NewUltimaFont(text.GetIntroNoticeFontSize())
		scene := &IntroMenuScene{
			ultimaFont:   font,
			noticeFont:   font,
			noticeOutput: text.NewOutput(font, font.TextFace.Metrics().HAscent*1.2, introNoticeLineCount, 80),
		}
		for _, err := range []error{player_save.ErrStaleRevision, fs.ErrPermission, player_save.ErrSlotNotFound,
			player_save.ErrIncompatible, ErrPlayerSaveUnavailable, errors.New("private failure")} {
			scene.reportPlayerSaveError("List saves", err)
			scene.prepareNotice(image.Rectangle{Max: size})
			require.Equal(t, scene.notice, strings.Join(strings.Fields(scene.noticeOutput.GetOutputStr(false)), " "))
			require.LessOrEqual(t, scene.noticeOutput.GetPanelContentLineCount(), introNoticeLineCount)
			for _, line := range strings.Split(scene.noticeOutput.GetOutputStr(false), "\n") {
				width, _ := font.MeasureString(strings.TrimSpace(line))
				require.LessOrEqual(t, width, float64(size.X*8/10))
			}
		}
	}
}

func TestPlayerSaveBrowserModelBuildsFriendlyDetails(t *testing.T) {
	summary := player_save.SlotSummary{
		Slot:   player_save.Slot02,
		Status: player_save.SlotReady,
		Metadata: player_save.MetadataSummary{
			CreatedAt:        time.Date(2026, time.August, 14, 21, 5, 0, 0, time.UTC),
			AvatarName:       "Shamino",
			Location:         2,
			Floor:            0,
			ActivePartyCount: 3,
		},
	}
	summary.Metadata.Calendar.Year = 142
	summary.Metadata.Calendar.Month = 8
	summary.Metadata.Calendar.Day = 17
	summary.Metadata.Calendar.Hour = 21
	summary.Metadata.Calendar.Minute = 5

	model := buildPlayerSaveBrowserModel([]player_save.SlotSummary{summary}, playerSaveBrowserLoad, "")

	assert.Equal(t, []string{"Avatar", "Location", "Party", "Britannian", "Saved"}, []string{
		model.views[0].Details[0].Label,
		model.views[0].Details[1].Label,
		model.views[0].Details[2].Label,
		model.views[0].Details[3].Label,
		model.views[0].Details[4].Label,
	})
	assert.Equal(t, "Britain, main", model.views[0].Details[1].Value)
	assert.Equal(t, "3 members", model.views[0].Details[2].Value)
	assert.Equal(t, "8-17-142, 9:05PM", model.views[0].Details[3].Value)
	assert.Equal(t, "Slot 2", model.views[0].Label)
	assert.Equal(t, "Shamino", model.views[0].RowSummary)
	assert.True(t, model.views[0].HasSave)
}

func TestPlayerSaveBrowserModelDistinguishesEmptyAndUnreadableRows(t *testing.T) {
	model := buildPlayerSaveBrowserModel([]player_save.SlotSummary{
		{Slot: player_save.Slot01, Status: player_save.SlotEmpty},
		{Slot: player_save.Slot02, Status: player_save.SlotCorrupt, Problem: "invalid snapshot"},
	}, playerSaveBrowserLoad, "")

	assert.Equal(t, "Slot 1", model.views[0].Label)
	assert.Empty(t, model.views[0].RowSummary)
	assert.False(t, model.views[0].HasSave)
	assert.True(t, model.views[0].NavigationDisabled)
	assert.Equal(t, "Slot 2", model.views[1].Label)
	assert.Equal(t, "Corrupt", model.views[1].RowSummary)
	assert.True(t, model.views[1].HasSave)
	assert.False(t, model.views[1].NavigationDisabled)

	writeModel := buildPlayerSaveBrowserModel([]player_save.SlotSummary{
		{Slot: player_save.Slot01, Status: player_save.SlotEmpty},
	}, playerSaveBrowserWrite, testPlaythroughID)
	assert.False(t, writeModel.views[0].NavigationDisabled)
}

func playerSaveBrowserTestSummaries() []player_save.SlotSummary {
	return []player_save.SlotSummary{
		{Slot: player_save.Slot01, Status: player_save.SlotEmpty},
		{Slot: player_save.Slot02, Status: player_save.SlotReady, Revision: "ready"},
		{Slot: player_save.Slot03, Status: player_save.SlotIncompatible, Revision: "incompatible"},
		{Slot: player_save.Slot04, Status: player_save.SlotCorrupt, Problem: "corrupt"},
		{Slot: player_save.Slot05, Status: player_save.SlotUnavailable, Problem: "unavailable"},
	}
}
