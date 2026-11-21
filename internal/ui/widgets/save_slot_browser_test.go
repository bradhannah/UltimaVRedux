package widgets

import (
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestSaveSlotBrowserKeyboardNavigation(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(10), SaveSlotBrowserConfig{})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	require.Equal(t, 2, browser.GetSelectedIndex())

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyUp)
	require.Equal(t, 1, browser.GetSelectedIndex())

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyPageDown)
	require.Equal(t, 1, browser.GetSelectedIndex())

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyPageUp)
	require.Equal(t, 1, browser.GetSelectedIndex())
}

func TestSaveSlotBrowserDisabledPrimaryRemainsSelectable(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	primaryCalls := 0
	views := []SaveSlotView{
		{ID: "slot-01", Label: "Slot 1", PrimaryEnabled: true},
		{ID: "slot-02", Label: "Slot 2", Problem: "Corrupt data", PrimaryEnabled: false},
	}
	browser := newTestSaveSlotBrowser(keyboard, views, SaveSlotBrowserConfig{
		OnPrimary: func(SaveSlotView) { primaryCalls++ },
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	require.Equal(t, 1, browser.GetSelectedIndex())
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyEnter)
	require.Zero(t, primaryCalls)
	require.Contains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Cannot select this")
}

func TestSaveSlotBrowserDeleteHasNoBrowserAction(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	primaryCalls := 0
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(2), SaveSlotBrowserConfig{
		OnPrimary: func(SaveSlotView) { primaryCalls++ },
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDelete)
	require.Zero(t, primaryCalls)
	require.Equal(t, 0, browser.GetSelectedIndex())
}

func TestSaveSlotBrowserCancelCallbackRunsOncePerRegisteredKeypress(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	cancelCalls := 0
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(1), SaveSlotBrowserConfig{
		OnCancel: func() { cancelCalls++ },
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyEscape)
	browser.Update()
	require.Equal(t, 1, cancelCalls)
}

func TestSaveSlotBrowserPrimaryCallbackReceivesDetachedViewOnce(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	views := []SaveSlotView{{
		ID:             "slot-01",
		Label:          "Slot 1",
		Details:        []SaveSlotDetail{{Label: "Day", Value: "12"}},
		PrimaryEnabled: true,
	}}
	var received []SaveSlotView
	browser := newTestSaveSlotBrowser(keyboard, views, SaveSlotBrowserConfig{
		OnPrimary: func(view SaveSlotView) {
			received = append(received, view)
			view.Details[0].Value = "changed by callback"
		},
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyEnter)
	browser.Update()
	require.Len(t, received, 1)
	require.Equal(t, "slot-01", received[0].ID)
	selected, ok := browser.GetSelectedView()
	require.True(t, ok)
	require.Equal(t, []SaveSlotDetail{{Label: "Day", Value: "12"}}, selected.Details)
}

func TestSaveSlotBrowserKeyboardDispatchesOnlyOneActionPerUpdate(t *testing.T) {
	pressed := map[ebiten.Key]bool{
		ebiten.KeyEscape: true,
		ebiten.KeyEnter:  true,
	}
	keyPressed := func(key ebiten.Key) bool { return pressed[key] }
	keyboard := input.NewKeyboardWithKeyStateSource(0, keyPressed)
	primaryCalls := 0
	cancelCalls := 0
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(1), SaveSlotBrowserConfig{
		OnPrimary:    func(SaveSlotView) { primaryCalls++ },
		OnCancel:     func() { cancelCalls++ },
		IsKeyPressed: keyPressed,
	})

	browser.Update()

	require.Zero(t, primaryCalls)
	require.Equal(t, 1, cancelCalls)
}

func TestSaveSlotBrowserEmptyModels(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	primaryCalls := 0
	browser := newTestSaveSlotBrowser(keyboard, nil, SaveSlotBrowserConfig{
		OnPrimary: func(SaveSlotView) { primaryCalls++ },
	})

	require.Equal(t, -1, browser.GetSelectedIndex())
	_, ok := browser.GetSelectedView()
	require.False(t, ok)
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyEnter)
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDelete)
	require.Zero(t, primaryCalls)
	require.NotContains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Cannot select this")
	browser.ensureLayout()
	notice := "This save changed since you opened the screen. Review the slot and try again."
	browser.SetNotice(notice)
	require.Contains(t, strings.Join(strings.Fields(browser.detailOutput.GetOutputStr(false)), " "), notice)
}

func TestSaveSlotBrowserProblemTextRemainsCompleteAndPaged(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	primaryCalls := 0
	problem := strings.Repeat("corrupt save data cannot be loaded safely ", 200)
	browser := newTestSaveSlotBrowser(keyboard, []SaveSlotView{{
		ID:      "slot-01",
		Label:   "Slot 1",
		Status:  "Unavailable",
		Details: []SaveSlotDetail{{Label: "Day", Value: "12"}},
		Problem: problem,
	}}, SaveSlotBrowserConfig{OnPrimary: func(SaveSlotView) { primaryCalls++ }})

	browser.populateDetailOutput()
	require.Greater(t, browser.detailOutput.GetPanelContentLineCount(), browser.detailLineCapacity)
	require.Contains(t, browser.detailOutput.GetOutputStr(false), "Day: 12")
	require.NotContains(t, browser.detailOutput.GetOutputStr(false), "...")
	require.Contains(t, strings.Join(strings.Fields(browser.detailOutput.GetOutputStr(false)), " "), strings.TrimSpace(problem))
	pages := browser.detailPageCount()
	for page := 1; page < pages; page++ {
		pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyRight)
		require.Equal(t, page, browser.detailPage)
		require.Zero(t, browser.GetSelectedIndex())
		pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyEnter)
	}
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyRight)
	require.Equal(t, pages-1, browser.detailPage)
	for range pages {
		pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyLeft)
	}
	require.Zero(t, browser.detailPage)
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeySpace)
	require.Equal(t, 1, browser.detailPage, "Space pages details as well as Left/Right")
	require.Zero(t, primaryCalls, "unavailable entries must remain disabled on every detail page")
}

func TestSaveSlotBrowserRecoveryAndNoticePagesFit(t *testing.T) {
	for _, size := range []image.Point{{X: 320, Y: 200}, {X: 1920, Y: 1080}} {
		setResponsiveDialogTestResolution(t, size.X, size.Y)
		keyboard := newWidgetTestKeyboard(0)
		problem := "This save is damaged and cannot be loaded or overwritten. Choose another slot. " +
			"To reclaim this slot, close the game, back up slot-04.json from the configured save directory, " +
			"then move that file out of the directory. Reopen the game to use the empty slot."
		notice := "This save changed since you opened the screen. Review the slot and try again."
		browser := newTestSaveSlotBrowser(keyboard, []SaveSlotView{
			{ID: "slot-04", Label: "4", Status: "Corrupt", Problem: problem},
			{ID: "slot-05", Label: "5"},
		}, SaveSlotBrowserConfig{Notice: notice})
		fullText := strings.Join(strings.Fields(browser.detailOutput.GetOutputStr(false)), " ")
		require.Contains(t, fullText, problem)
		require.Contains(t, fullText, notice)
		for page := range browser.detailPageCount() {
			require.Equal(t, page, browser.detailPage)
			visibleLines := min(browser.detailLineCapacity,
				browser.detailOutput.GetPanelContentLineCount()-page*browser.detailLineCapacity)
			require.LessOrEqual(t, browser.detailOutput.GetTotalHeightForLines(visibleLines), float64(browser.detailTextRect.Dy()))
			if browser.detailPageCount() > 1 {
				require.Contains(t, browser.footerOutput.GetOutputStr(false), "Left/Right: Details")
			}
			for _, line := range strings.Split(browser.footerOutput.GetOutputStr(false), "\n") {
				width, _ := browser.bodyFont.MeasureString(strings.TrimSpace(line))
				require.LessOrEqual(t, width, float64(browser.footerTextRect.Dx()))
			}
			pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyRight)
		}
		browser.SetNotice(notice)
		require.Zero(t, browser.detailPage)
		pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyRight)
		pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
		require.Zero(t, browser.detailPage)
		require.Equal(t, 1, browser.GetSelectedIndex())
	}
}

func TestSaveSlotBrowserUsesConfiguredInitialSelection(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(4), SaveSlotBrowserConfig{
		InitialSelectionID: "c",
	})

	require.Equal(t, 2, browser.GetSelectedIndex())
}

func TestSaveSlotBrowserNumpadEnterInvokesPrimary(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	primaryCalls := 0
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(1), SaveSlotBrowserConfig{
		OnPrimary: func(SaveSlotView) { primaryCalls++ },
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyNumpadEnter)
	require.Equal(t, 1, primaryCalls)
}

func TestSaveSlotBrowserHeldDownRepeatsDeterministically(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	held := false
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(8), SaveSlotBrowserConfig{
		IsKeyPressed: func(key ebiten.Key) bool {
			return held && key == ebiten.KeyDown
		},
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	require.Equal(t, 1, browser.GetSelectedIndex())

	held = true
	for range 17 { // 300 ms initial delay at 60 updates/second.
		browser.Update()
	}
	require.Equal(t, 1, browser.GetSelectedIndex())
	browser.Update()
	require.Equal(t, 2, browser.GetSelectedIndex())

	for range 7 { // The next 120 ms deadline is reached on the eighth tick.
		browser.Update()
	}
	require.Equal(t, 2, browser.GetSelectedIndex())
	browser.Update()
	require.Equal(t, 3, browser.GetSelectedIndex())
}

func TestSaveSlotBrowserHeldRepeatResetsAfterRelease(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	held := false
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(8), SaveSlotBrowserConfig{
		IsKeyPressed: func(key ebiten.Key) bool {
			return held && key == ebiten.KeyDown
		},
	})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	held = true
	for range 5 {
		browser.Update()
	}
	held = false
	browser.Update()
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	require.Equal(t, 2, browser.GetSelectedIndex())
}

func TestSaveSlotBrowserRapidPhysicalTapsIgnoreGlobalKeyboardDelay(t *testing.T) {
	keyboard := newWidgetTestKeyboard(1000)
	held := false
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(8), SaveSlotBrowserConfig{
		IsKeyPressed: func(key ebiten.Key) bool {
			return held && key == ebiten.KeyDown
		},
	})

	held = true
	browser.Update()
	held = false
	browser.Update()
	held = true
	browser.Update()

	require.Equal(t, 2, browser.GetSelectedIndex())
}

func TestClassicUIPixelUnitMatchesIntroCanvasPixels(t *testing.T) {
	require.Equal(t, 1, getClassicUIPixelUnit(320, 200))
	require.Equal(t, 5, getClassicUIPixelUnit(1920, 1080))
}

func TestClassicPixelRoundedFrameClipsSquareCorners(t *testing.T) {
	frame := image.Rect(10, 10, 90, 90)
	segments := classicPixelRoundedFrameSegments(frame, 5)
	contains := func(point image.Point) bool {
		for _, segment := range segments {
			if point.In(segment) {
				return true
			}
		}

		return false
	}

	require.False(t, contains(frame.Min))
	require.True(t, contains(image.Pt(frame.Min.X+10, frame.Min.Y)))
	require.True(t, contains(image.Pt(frame.Min.X+5, frame.Min.Y+5)))
}

func TestIntroSaveDialogsUseConsistentFontSize(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(1), SaveSlotBrowserConfig{})
	pathModal := newTestSaveDescriptionEntryModal(new(fakeSaveDescriptionTextField), func(string, bool) {})
	pathModal.ensureRendering(browser.lastScreenWidth, browser.lastScreenHeight)
	expectedSize := GetScaledFont(FontSizeBody).TextFace.Size

	require.Equal(t, GetScaledFont(FontSizeTitle).TextFace.Size, browser.titleOutput.Font.TextFace.Size)
	require.Equal(t, expectedSize, browser.bodyFont.TextFace.Size)
	require.Equal(t, expectedSize, browser.footerOutput.Font.TextFace.Size)
	require.Equal(t, expectedSize, pathModal.promptOutput.Font.TextFace.Size)
	require.Equal(t, expectedSize, pathModal.fieldOutput.Font.TextFace.Size)
	require.Equal(t, expectedSize, pathModal.hintOutput.Font.TextFace.Size)
}

func TestSaveSlotBrowserFooterUsesClassicTwoLineLayout(t *testing.T) {
	browser := newTestSaveSlotBrowser(newWidgetTestKeyboard(0), makeSaveSlotViews(1), SaveSlotBrowserConfig{
		PrimaryActionLabel: "Save",
	})
	interior := browser.outerBorder.GetInteriorBounds()
	footerText := browser.footerOutput.GetOutputStr(false)

	require.True(t, browser.footerRect.In(interior))
	require.True(t, browser.footerTextRect.In(browser.footerRect))
	require.Less(t, browser.listRect.Max.Y, browser.footerRect.Min.Y)
	require.Equal(t, saveSlotBrowserFooterLineCount, browser.footerOutput.GetPanelContentLineCount())
	require.Contains(t, footerText, "[Up/Down] Slot")
	require.NotContains(t, footerText, "Page Up/Page Down")
	require.Contains(t, footerText, "[Enter] Save")
	require.Contains(t, footerText, "[Esc] Cancel")

	screen := ebiten.NewImage(browser.lastScreenWidth, browser.lastScreenHeight)
	t.Cleanup(screen.Dispose)
	browser.Draw(screen)
	requireImageRegionContainsColor(t, screen, browser.footerRect, DialogRuleColor)
}

func TestSaveSlotBrowserFooterUsesConfiguredPrimaryActionLabel(t *testing.T) {
	browser := newTestSaveSlotBrowser(newWidgetTestKeyboard(0), makeSaveSlotViews(1), SaveSlotBrowserConfig{
		PrimaryActionLabel: "Import",
	})

	require.Contains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Import")
	require.NotContains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Save")
}

func TestSaveSlotBrowserFooterUsesConfiguredCancelActionLabel(t *testing.T) {
	browser := newTestSaveSlotBrowser(newWidgetTestKeyboard(0), makeSaveSlotViews(1), SaveSlotBrowserConfig{
		CancelActionLabel: "Back",
	})

	footer := browser.footerOutput.GetOutputStr(false)
	require.Contains(t, footer, "[Esc] Back")
	require.NotContains(t, footer, "[Esc] Cancel")
}

func TestSaveSlotBrowserDrawsSelectedFolderReview(t *testing.T) {
	path := "/Users/avatar/Documents/InstalledGames/UltimaV"
	browser := newTestSaveSlotBrowser(newWidgetTestKeyboard(0), []SaveSlotView{{
		ID: "gog", Label: "GOG installation", ReviewValue: path, HasSave: true, PrimaryEnabled: true,
		Details: []SaveSlotDetail{{Label: "Folder", Value: path}},
	}}, SaveSlotBrowserConfig{
		ReviewLabel: "Selected Folder",
	})

	browser.ensureLayout()
	browser.populateDetailOutput()
	browser.prepareReview()
	require.NotEmpty(t, browser.reviewFrameRect)
	require.NotNil(t, browser.reviewOutput)
	require.Contains(t, browser.reviewHeadingOutput.GetOutputStr(false), "Selected Folder")
	require.Contains(t, strings.Join(strings.Fields(browser.detailOutput.GetOutputStr(false)), ""), path)
	require.Contains(t, strings.Join(strings.Fields(browser.reviewOutput.GetOutputStr(false)), ""), path)
	require.Greater(t, browser.reviewOutput.GetPanelContentLineCount(), 1)
	require.NotContains(t, browser.reviewOutput.GetOutputStr(false), "No folder selected")
}

func TestSaveSlotBrowserResizeRebuildsFooterAndPreservesSelection(t *testing.T) {
	setResponsiveDialogTestResolution(t, 320, 200)

	keyboard := newWidgetTestKeyboard(0)
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(10), SaveSlotBrowserConfig{
		PrimaryActionLabel: "Load",
	})
	require.Equal(t, saveSlotBrowserFooterLineCount, browser.footerOutput.GetPanelContentLineCount())
	require.Contains(t, browser.footerOutput.GetOutputStr(false), "[Up/Down] Slot")
	require.NotContains(t, browser.footerOutput.GetOutputStr(false), "Page Up/Page Down")
	require.Contains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Load")
	require.Contains(t, browser.footerOutput.GetOutputStr(false), "[Esc] Cancel")
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	originalFooter := browser.footerOutput
	originalRect := browser.footerRect

	setResponsiveDialogTestResolution(t, 1280, 720)
	browser.ensureLayout()

	require.Equal(t, 1, browser.GetSelectedIndex())
	require.NotSame(t, originalFooter, browser.footerOutput)
	require.NotEqual(t, originalRect, browser.footerRect)
	require.True(t, browser.footerRect.In(browser.outerBorder.GetInteriorBounds()))
	require.Equal(t, saveSlotBrowserFooterLineCount, browser.footerOutput.GetPanelContentLineCount())
	require.Contains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Load")
}

func TestSaveSlotBrowserRowsReserveNewestColumn(t *testing.T) {
	occupied := SaveSlotView{Label: "Slot 1", RowSummary: "Avatar", HasSave: true, Newest: true}
	empty := SaveSlotView{Label: "Slot 2", Status: "Empty"}
	require.Equal(t, "Slot 1 * Avatar", saveSlotRowText(occupied, 100))
	require.Equal(t, "Slot 2   Empty", saveSlotRowText(empty, 100))
	occupied.Newest = false
	require.Equal(t, "Slot 1   Avatar", saveSlotRowText(occupied, 100))
	longLabel := SaveSlotView{Label: "Recommended personal folder", HasSave: true, Newest: true}
	require.Contains(t, saveSlotRowText(longLabel, 12), "*")
	require.Equal(t, "Look Somewhere Else", saveSlotRowText(SaveSlotView{
		Label: "Look Somewhere Else", HasSave: true,
	}, 20))
}

func TestSaveSlotBrowserNavigationSkipsDisabledRows(t *testing.T) {
	keyboard := newWidgetTestKeyboard(0)
	browser := newTestSaveSlotBrowser(keyboard, []SaveSlotView{
		{ID: "a", PrimaryEnabled: true},
		{ID: "b", NavigationDisabled: true},
		{ID: "c", NavigationDisabled: true},
		{ID: "d", PrimaryEnabled: true},
	}, SaveSlotBrowserConfig{InitialSelectionID: "a"})

	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyDown)
	require.Equal(t, 3, browser.GetSelectedIndex())
	pressSaveSlotBrowserKey(browser, keyboard, ebiten.KeyUp)
	require.Equal(t, 0, browser.GetSelectedIndex())
}

func TestSaveSlotBrowserWithNoNavigableRowsHasNoSelection(t *testing.T) {
	browser := newTestSaveSlotBrowser(newWidgetTestKeyboard(0), []SaveSlotView{
		{ID: "a", NavigationDisabled: true},
		{ID: "b", NavigationDisabled: true},
	}, SaveSlotBrowserConfig{InitialSelectionID: "a"})

	require.Equal(t, -1, browser.GetSelectedIndex())
	_, selected := browser.GetSelectedView()
	require.False(t, selected)
	require.NotContains(t, browser.footerOutput.GetOutputStr(false), "[Enter] Cannot select this")
	require.Contains(t, strings.Join(strings.Fields(browser.detailOutput.GetOutputStr(false)), " "), "No saved games to load.")
}

func newTestSaveSlotBrowser(
	keyboard *input.Keyboard,
	views []SaveSlotView,
	overrides SaveSlotBrowserConfig,
) *SaveSlotBrowser {
	overrides.Keyboard = keyboard
	return NewSaveSlotBrowser(views, overrides)
}

func makeSaveSlotViews(count int) []SaveSlotView {
	views := make([]SaveSlotView, count)
	for index := range views {
		views[index] = SaveSlotView{
			ID:             string(rune('a' + index)),
			Label:          "Save slot",
			Status:         "Available",
			PrimaryEnabled: true,
		}
	}
	return views
}

func pressSaveSlotBrowserKey(browser *SaveSlotBrowser, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	browser.Update()
}
