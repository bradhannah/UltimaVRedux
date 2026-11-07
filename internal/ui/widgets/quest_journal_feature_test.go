package widgets

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestQuestJournalSearchRedactionStableIdentityAndVisits(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	dossier := testQuestJournalDossier()
	dossier.Chronicle = []game_state.QuestJournalChronicleEntry{
		{Sequence: 1, Title: "Flame of Truth", Summary: "Discovered the Flame", Source: "Lycaeum"},
		{Sequence: 2, Title: "Shadowlord of Hatred", Summary: "An independent domain note"},
	}
	dossier.Virtues[1].Mantra = "SECRET-MANTRA"
	dossier.Virtues[1].CodexAnswer = "SECRET-ANSWER"
	dossier.Shadowlords[0].Opposes = "Truth"
	keyboard := newWidgetTestKeyboard(0)
	configuration := testQuestJournalConfig(keyboard)
	configuration.NewEntries = [3][]bool{{true, true}, {true}, {true}}
	dialog := NewQuestJournalDialog(dossier, configuration)
	configuration.NewEntries[0][0] = false
	require.True(t, dialog.entryIsNew(QuestJournalChronicleTab, 0), "opening markers are detached")
	dialog.beginSearch()
	require.Equal(t, "Type words to search the Journal.", dialog.searchStatus())
	for _, hidden := range []string{"secret-mantra", "secret-answer", "nosfentor", "astaroth hatred", "justice not learned"} {
		dialog.setSearchQuery(hidden)
		require.Empty(t, dialog.searchResults, hidden)
		require.Equal(t, "No matching entries.", dialog.searchStatus())
	}
	for _, query := range []string{"not started", "justice not started"} {
		dialog.setSearchQuery(query)
		var titles []string
		for _, result := range dialog.searchResults {
			titles = append(titles, dialog.searchEntries[result].title)
		}
		require.Contains(t, titles, "Justice", "visible state text is searchable regardless of its colour")
	}
	dialog.setSearchQuery("TRUTH flame lycaeum")
	require.Len(t, dialog.searchResults, 1)
	dialog.setSearchQuery("truth")
	require.Len(t, dialog.searchResults, 3)
	dialog.navigate(ebiten.KeyEnd)
	selected := dialog.searchResults[dialog.searchSelection]
	dialog.setSearchQuery("trut")
	require.Equal(t, selected, dialog.searchResults[dialog.searchSelection])
	setResponsiveDialogTestResolution(t, 1280, 720)
	require.NotPanics(t, dialog.ensureLayout, "search row indices must never index the underlying active-tab detail pane")
	require.Equal(t, selected, dialog.searchResults[dialog.searchSelection])
	require.Equal(t, [3]bool{true, false, false}, dialog.VisitedTabs())
	dialog.openSearchResult()
	require.Equal(t, QuestJournalShadowlordsTab, dialog.activeTab)
	require.Equal(t, [3]bool{true, false, true}, dialog.VisitedTabs())
	require.True(t, dialog.tabIsNew(QuestJournalChronicleTab))
	require.True(t, dialog.tabIsNew(QuestJournalShadowlordsTab))
	dialog.beginSearch()
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyEscape)
	require.False(t, dialog.searching)
	require.False(t, dialog.dismissed)
}

func TestQuestJournalSearchTypingDoesNotRunTabOrDetailCommands(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), testQuestJournalConfig(keyboard))
	pressQuestJournalKey(dialog, keyboard, ebiten.KeySlash)
	require.True(t, dialog.searching)
	for _, key := range []ebiten.Key{ebiten.KeyT, ebiten.KeyR, ebiten.KeySpace, ebiten.KeyU, ebiten.KeyTab} {
		pressQuestJournalKey(dialog, keyboard, key)
	}
	require.Equal(t, "tr u", strings.ToLower(dialog.searchQuery))
	require.Equal(t, QuestJournalChronicleTab, dialog.activeTab)
	require.Zero(t, dialog.detailPage)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyBackspace)
	require.Equal(t, "tr ", strings.ToLower(dialog.searchQuery))
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyEscape)
	require.False(t, dialog.dismissed)
	pressQuestJournalKey(dialog, keyboard, ebiten.KeyEscape)
	require.True(t, dialog.dismissed)
}

func TestQuestJournalTabCycleContainsOnlyRetainedReaders(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	keyboard := newWidgetTestKeyboard(0)
	configuration := testQuestJournalConfig(keyboard)
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	var titles []string
	for range questJournalTabCount {
		titles = append(titles, questJournalTabTitle(dialog.activeTab))
		pressQuestJournalKey(dialog, keyboard, ebiten.KeyTab)
	}
	require.Equal(t, []string{"Chronicle", "Virtues", "Shadowlords", "Dungeons", "Last Conversation", "Excerpts",
		"Moonstones", "My Notes"}, titles)
	require.Equal(t, QuestJournalChronicleTab, dialog.activeTab)
	for range questJournalTabCount {
		dialog.switchTabBy(-1)
		require.NotEqual(t, "History", questJournalTabTitle(dialog.activeTab))
	}
	require.Equal(t, QuestJournalChronicleTab, dialog.activeTab)
}

func TestQuestJournalLayoutAndCompleteFooterAtAllSizes(t *testing.T) {
	sizes := append(config.SupportedWindowResolutions(), config.ScreenResolution{X: 320, Y: 200})
	for _, size := range sizes {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setQuestJournalTestResolution(t, size.X, size.Y)
			configuration := testQuestJournalConfig(newWidgetTestKeyboard(0))
			configuration.NewEntries = [3][]bool{{true}, {true}, {true}}
			dossier := testQuestJournalDossier()
			dossier.Shadowlords[2].DisplayName = "Shadowlord III"
			dialog := NewQuestJournalDialog(dossier, configuration)
			require.Equal(t, GetScaledFont(FontSizeTitle).TextFace.Size, dialog.titleOutput.Font.TextFace.Size)
			for tab, rect := range dialog.layout.tabRects {
				width, _ := dialog.font.MeasureString(dialog.tabTitle(QuestJournalTab(tab)))
				require.LessOrEqual(t, width, float64(rect.Dx()))
				require.True(t, rect.In(dialog.layout.tabsRect))
			}
			for tab := range questJournalTabCount {
				dialog.activeTab = QuestJournalTab(tab)
				dialog.enterTab()
				rows := packedFooterRows(dialog.font, dialog.footerGuidance(), dialog.footerContentRect().Dx())
				require.LessOrEqual(t, len(rows), dialog.footerLineCount())
				guidance := strings.Join(rows, "\n")
				for _, line := range rows {
					width, _ := dialog.font.MeasureString(line)
					require.LessOrEqual(t, width, float64(dialog.footerContentRect().Dx()), "%s", line)
				}
				require.Contains(t, guidance, "Close")
				require.NotContains(t, guidance, "S-J")
				if tab == int(QuestJournalShadowlordsTab) {
					require.True(t, dialog.shadowlordSpriteRect().In(dialog.layout.detailRect))
					require.Equal(t, "Shadowlord III", fitJournalLine("Shadowlord III", dialog.font, dialog.listContentRect().Dx()-dialog.layout.padding))
				}
			}
		})
	}
}

func TestQuestJournalCuratedPagesUseFullCapacityAndCenter(t *testing.T) {
	setQuestJournalTestResolution(t, 320, 200)
	dossier := testQuestJournalDossier()
	dossier.Chronicle = make([]game_state.QuestJournalChronicleEntry, 32)
	for index := range dossier.Chronicle {
		dossier.Chronicle[index].Title = fmt.Sprintf("Note %d", index)
	}
	dialog := NewQuestJournalDialog(dossier, testQuestJournalConfig(newWidgetTestKeyboard(0)))
	capacity := dialog.visibleRows(dialog.listContentRect())
	dialog.navigate(ebiten.KeyPageDown)
	require.Equal(t, capacity, dialog.GetSelectedIndex())
	start, _ := zStatsVisibleRange(dialog.GetSelectedIndex(), 32, capacity)
	require.Equal(t, capacity-capacity/2, start)
	dialog.navigate(ebiten.KeyEnd)
	require.Equal(t, 31, dialog.GetSelectedIndex())
	dialog.navigate(ebiten.KeyDown)
	require.Equal(t, 31, dialog.GetSelectedIndex())
	dialog.navigate(ebiten.KeyHome)
	require.Zero(t, dialog.GetSelectedIndex())
}

func TestQuestJournalHeldNavigationUsesRepeatRegistration(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	configuration := testQuestJournalConfig(keyboard)
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	dialog.switchTabBy(1)
	held[ebiten.KeyDown] = true
	dialog.Update()
	require.Equal(t, 1, dialog.GetSelectedIndex())
	for range 18 {
		dialog.Update()
	}
	require.Equal(t, 2, dialog.GetSelectedIndex(), "held Down does not require physical release")
	delete(held, ebiten.KeyDown)
	dialog.navigate(ebiten.KeyDown)
	held[ebiten.KeyUp] = true
	dialog.Update()
	require.Equal(t, 2, dialog.GetSelectedIndex())
	for range 18 {
		dialog.Update()
	}
	require.Equal(t, 1, dialog.GetSelectedIndex(), "held Up retains ordinary list repeat")
}

func TestQuestJournalSearchHeldPageKeysRespectRepeatDelay(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	for _, key := range []ebiten.Key{ebiten.KeyPageDown, ebiten.KeyPageUp} {
		t.Run(key.String(), func(t *testing.T) {
			held := map[ebiten.Key]bool{}
			keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
			dossier := testQuestJournalDossier()
			dossier.Chronicle = make([]game_state.QuestJournalChronicleEntry, 32)
			for index := range dossier.Chronicle {
				dossier.Chronicle[index].Title = fmt.Sprintf("Note %d", index)
			}
			dialog := NewQuestJournalDialog(dossier, testQuestJournalConfig(keyboard))
			dialog.beginSearch()
			dialog.searchInput.SetText("note")
			dialog.setSearchQuery("note")
			capacity := max(1, dialog.visibleRows(dialog.searchContentRect())/journalSearchResultRows)
			require.Less(t, capacity, len(dialog.searchResults)-1)
			expected := capacity
			if key == ebiten.KeyPageUp {
				dialog.navigate(ebiten.KeyEnd)
				expected = len(dialog.searchResults) - 1 - capacity
			}
			held[key] = true
			dialog.Update()
			require.Equal(t, expected, dialog.GetSelectedIndex())
			for range 17 {
				dialog.Update()
				require.Equal(t, expected, dialog.GetSelectedIndex(), "throttled page keys must not reach TextInput")
			}
			dialog.Update()
			if key == ebiten.KeyPageDown {
				expected = min(expected+capacity, len(dialog.searchResults)-1)
			} else {
				expected = max(0, expected-capacity)
			}
			require.Equal(t, expected, dialog.GetSelectedIndex(), "repeat resumes after the delay")
			require.Equal(t, "note", dialog.searchQuery)
		})
	}
}

func TestQuestJournalEmptyChronicleAndFocusOwnership(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	dialog := NewQuestJournalDialog(game_state.QuestJournalDossier{}, testQuestJournalConfig(keyboard))
	require.Equal(t, -1, dialog.GetSelectedIndex())
	for _, key := range []ebiten.Key{ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyPageDown, ebiten.KeyHome, ebiten.KeyEnd, ebiten.KeyEnter} {
		pressQuestJournalKey(dialog, keyboard, key)
	}
	stack := DialogStack{}
	stack.PushModalDialog(dialog)
	other := NewQuestJournalDialog(testQuestJournalDossier(), testQuestJournalConfig(keyboard))
	stack.PushModalDialog(other)
	require.False(t, dialog.focused)
	held[ebiten.KeyEscape] = true
	dialog.Update()
	require.False(t, dialog.dismissed)
	stack.MustRemoveWidgetInstance(other)
	require.True(t, dialog.focused)
	dialog.Update()
	require.False(t, dialog.dismissed, "the focusing key must first be released")
	delete(held, ebiten.KeyEscape)
	keyboard.ClearFrameConsumption()
	held[ebiten.KeyShift] = true
	held[ebiten.KeyJ] = true
	dialog.beginSearch()
	dialog.Update()
	require.True(t, dialog.dismissed, "Shift-J closes even while searching")
}

func TestQuestJournalSelectedStarIsMonochrome(t *testing.T) {
	setQuestJournalTestResolution(t, 640, 400)
	configuration := testQuestJournalConfig(newWidgetTestKeyboard(0))
	configuration.NewEntries = [3][]bool{{true}, nil, nil}
	dialog := NewQuestJournalDialog(testQuestJournalDossier(), configuration)
	screen := ebiten.NewImage(640, 400)
	t.Cleanup(screen.Deallocate)
	dialog.Draw(screen)
	rect := dialog.listContentRect()
	rect.Max.Y = rect.Min.Y + dialog.rowHeight()
	dialog.drawListRow(screen, rect, 0, true)
	starWidth, _ := dialog.font.MeasureString("*")
	black := 0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Min.X+int(starWidth); x++ {
			red, green, blue, _ := screen.At(x, y).RGBA()
			require.Equal(t, red, green)
			require.Equal(t, red, blue, "selected star must not remain yellow")
			if red == 0 {
				black++
			}
		}
	}
	require.Positive(t, black)
}

// packedFooterRows exposes every packed row, including any beyond a footer's capacity.
func packedFooterRows(font *text.UltimaFont, guidance string, width int) []string {
	output := text.NewOutput(font, 1, 8, 1)
	PrepareKeyboardFooter(image.Rect(0, 0, width, 1), guidance, output)

	rows := strings.Split(output.GetOutputStr(false), "\n")
	for index := range rows {
		rows[index] = strings.TrimSpace(rows[index])
	}

	return rows
}

func TestQuestJournalSharedFooterPolicy(t *testing.T) {
	for _, size := range append(config.SupportedWindowResolutions(), config.ScreenResolution{X: 320, Y: 200}) {
		setQuestJournalTestResolution(t, size.X, size.Y)
		item := NewItemSelectionDialog(ItemSelectionDialogConfig{
			Keyboard: newWidgetTestKeyboard(0), OnSelect: func(game_state.UseableItemInfo) {}, OnCancel: func() {},
		})
		rows := packedFooterRows(item.font, "[Tab/Shift+Tab] Category   [Space] Details\n"+
			"[Up/Down] Item   [Shift+Up/Down] Page   [Enter] Use   [Esc] Cancel", item.footerContentRect().Dx())
		require.LessOrEqual(t, len(rows), itemDialogFooterLines, "%v", size)
		guidance := strings.Join(rows, "\n")
		require.Contains(t, guidance, "[Shift+Up/Down] Page")
		require.Contains(t, guidance, "Category")
		require.Contains(t, guidance, "Cancel")
		for _, line := range strings.Split(guidance, "\n") {
			width, _ := item.font.MeasureString(line)
			require.LessOrEqual(t, width, float64(item.footerContentRect().Dx()))
		}
		dialog := NewQuestJournalDialog(testQuestJournalDossier(), testQuestJournalConfig(newWidgetTestKeyboard(0)))
		dialog.beginSearch()
		rows = packedFooterRows(dialog.font, dialog.footerGuidance(), dialog.footerContentRect().Dx())
		require.LessOrEqual(t, len(rows), dialog.footerLineCount(), "%v", size)
		for _, line := range rows {
			width, _ := dialog.font.MeasureString(line)
			require.LessOrEqual(t, width, float64(dialog.footerContentRect().Dx()))
		}
	}
}
