package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func summaryItems(count int) []game_state.CombatCollectedItem {
	items := make([]game_state.CombatCollectedItem, count)
	for index := range items {
		items[index] = game_state.CombatCollectedItem{Name: "Gold", Sprite: indexes.ItemMoney, Quantity: uint64(index + 100)}
	}
	return items
}

func setSummaryViewport(t *testing.T, viewport image.Rectangle) {
	t.Helper()
	previous := display.GetManager().SetMapPopupViewportProvider(func(image.Rectangle) image.Rectangle { return viewport })
	t.Cleanup(func() { display.GetManager().SetMapPopupViewportProvider(previous) })
}

func assertSummaryInterior(t *testing.T, dialog *CombatLootDialog, viewport image.Rectangle) {
	t.Helper()
	s := dialog.summary
	assert.True(t, dialog.bounds.In(viewport), "bounds %v, viewport %v", dialog.bounds, viewport)
	assert.LessOrEqual(t, dialog.bounds.Dy(), viewport.Dy()/2)
	require.Positive(t, s.body.Dy())
	assert.True(t, s.body.In(dialog.bounds))
	assert.True(t, s.footer.In(dialog.bounds))
	assert.GreaterOrEqual(t, s.body.Min.Y, dialog.bounds.Min.Y+s.padding+s.headingHeight)
	assert.LessOrEqual(t, s.body.Max.Y, s.footer.Min.Y, "the clipped scroll region cannot paint over the footer")
	assert.GreaterOrEqual(t, s.footer.Dy(), combatSummaryTextHeight(dialog.output))
	assert.Equal(t, text.GetScaledNumberToWidth(s.screenWidth, float64(FontSizeBody)), dialog.output.Font.TextFace.Size)
	assert.Equal(t, text.GetScaledNumberToWidth(s.screenWidth, float64(FontSizeTitle)), dialog.heading.Font.TextFace.Size)
}

func TestCombatSummaryBottomAnchorContentHeightAndResize(t *testing.T) {
	sheet := &sprites.SpriteSheet{}
	for _, supplied := range []bool{false, true} {
		t.Run(fmt.Sprintf("viewport=%t", supplied), func(t *testing.T) {
			// Keep the same dialogs through resize, including a return to the small size.
			dialogs := make([]*CombatLootDialog, 0, 4)
			for _, count := range []int{0, 1, 3, 45} {
				dialogs = append(dialogs, NewCombatSummaryDialog(game_state.CombatConclusion{
					Reason: game_state.CombatVictory, Collected: summaryItems(count),
				}, newWidgetTestKeyboard(0), sheet, func() {}))
			}
			for _, size := range []image.Point{{960, 540}, {1280, 720}, {1920, 1080}, {2560, 1440}, {960, 540}} {
				t.Run(fmt.Sprint(size), func(t *testing.T) {
					viewport := image.Rect(0, 0, size.X*3/4, size.Y)
					if supplied {
						// Both horizontal and vertical insets; the lower edge clamps the screen anchor.
						viewport = image.Rect(size.X/10, size.Y/12, size.X*7/10, size.Y*5/6)
						setSummaryViewport(t, viewport)
					} else {
						setSummaryViewport(t, image.Rectangle{})
					}
					previousHeight := 0
					for index, dialog := range dialogs {
						dialog.prepareSummary(image.Rectangle{Max: size}, viewport)
						assertSummaryInterior(t, dialog, viewport)
						assert.InDelta(t, float64(size.X)*540/1280, dialog.bounds.Dx(), 1)
						assert.InDelta(t, float64(viewport.Min.X+viewport.Max.X)/2, float64(dialog.bounds.Min.X+dialog.bounds.Max.X)/2, 0.5)
						bottom := min(int(float64(size.Y)*0.88), viewport.Max.Y-dialog.summary.padding)
						assert.Equal(t, bottom, dialog.bounds.Max.Y, "entry count must not move the bottom")
						assert.GreaterOrEqual(t, dialog.bounds.Dy(), previousHeight)
						previousHeight = dialog.bounds.Dy()
						assert.Contains(t, dialog.output.GetOutputStr(false), "Continue")
						if index < 3 {
							assert.NotContains(t, dialog.output.GetOutputStr(false), "Scroll")
							assert.Equal(t, dialog.summary.contentHeight, dialog.summary.body.Dy())
						} else {
							assert.Contains(t, dialog.output.GetOutputStr(false), "Scroll")
						}
						if index == 0 {
							assert.Nil(t, dialog.summary.notice, "nothing collected needs no notice")
							assert.Contains(t, dialog.rows[0].GetOutputStr(false), "No items collected")
							assert.Nil(t, dialog.icons, "measuring text does not initialize raster icons")
						} else {
							require.NotNil(t, dialog.summary.notice)
							assert.Contains(t, dialog.summary.notice.GetOutputStr(false), "Collected into party inventory")
							assert.GreaterOrEqual(t, dialog.summary.footer.Dy(),
								combatSummaryTextHeight(dialog.summary.notice)+dialog.summary.padding+
									combatSummaryTextHeight(dialog.output), "the notice and the key hints keep a gap")
						}
					}
				})
			}
		})
	}
}

func TestCombatSummaryNarrowViewportHeadingWrapAndTypography(t *testing.T) {
	viewport := image.Rect(170, 30, 530, 700)
	setSummaryViewport(t, viewport)
	items := []game_state.CombatCollectedItem{{
		Name: "A deliberately long equipment description with a final word: END", Sprite: indexes.ItemWeapon, Quantity: math.MaxUint64,
	}}
	dialog := NewCombatSummaryDialog(game_state.CombatConclusion{Reason: game_state.CombatDefeat, Collected: items}, newWidgetTestKeyboard(0), &sprites.SpriteSheet{}, func() {})
	dialog.prepareSummary(image.Rect(0, 0, 1280, 720), viewport)
	s := dialog.summary
	assertSummaryInterior(t, dialog, viewport)
	assert.Equal(t, viewport.Dx()-s.padding*2, dialog.bounds.Dx(), "width is capped to the supplied pane")
	assert.Positive(t, s.outcomeTop, "heading falls back to separate lines without shrinking")
	assert.Contains(t, dialog.heading.GetOutputStr(false), "Battle complete")
	assert.Contains(t, s.outcomeOutput.GetOutputStr(false), "Battle lost")
	assert.GreaterOrEqual(t, s.outcomeTop, combatSummaryTextHeight(dialog.heading))
	assert.Greater(t, dialog.rows[0].GetPanelContentLineCount(), 1)
	assert.Contains(t, dialog.rows[0].GetOutputStr(false), "END")
	assert.Equal(t, "18446744073709551615", strings.Join(strings.Fields(s.values[0].GetOutputStr(false)), ""))
	assert.GreaterOrEqual(t, s.valueTops[0], combatSummaryTextHeight(dialog.rows[0]), "large quantities start below wrapped names")
	assert.GreaterOrEqual(t, s.rowHeights[0]-s.padding, s.valueTops[0]+combatSummaryTextHeight(s.values[0]))
	for _, output := range []*text.Output{dialog.rows[0], s.values[0]} {
		assert.Equal(t, dialog.output.Font, output.Font, "details retain the readable body font")
		available := s.body.Dx() - int(math.Ceil(dialog.output.GetTotalHeightForLines(2))) - s.padding
		for _, line := range strings.Split(output.GetOutputStr(false), "\n") {
			width, _ := output.Font.MeasureString(strings.TrimSpace(line)) // GetOutputStr adds a separator space before newlines.
			assert.LessOrEqual(t, width, float64(available), "wrapped content %q must fit its measured column", line)
		}
	}
	_, label := dialog.rows[0].FirstPanelLineWithColor(0, dialog.rows[0].GetPanelContentLineCount(), DialogLabelColor)
	_, value := s.values[0].FirstPanelLineWithColor(0, s.values[0].GetPanelContentLineCount(), DialogValueColor)
	_, header := dialog.heading.FirstPanelLineWithColor(0, dialog.heading.GetPanelContentLineCount(), DialogHeaderColor)
	_, keys := dialog.output.FirstPanelLineWithColor(0, dialog.output.GetPanelContentLineCount(), DialogControlKeyColor)
	_, hints := dialog.output.FirstPanelLineWithColor(0, dialog.output.GetPanelContentLineCount(), DialogControlHintColor)
	assert.True(t, label && value && header && keys && hints)
	assert.NotEqual(t, DialogLabelColor, DialogValueColor)
	// The ordinary width fits the structured left/right heading on one row.
	setSummaryViewport(t, image.Rectangle{})
	dialog.prepareSummary(image.Rect(0, 0, 1280, 720), image.Rect(0, 0, 960, 720))
	assert.Zero(t, s.outcomeTop)
}

func TestCombatSummaryScrollReachesLongLastEntryAndResizes(t *testing.T) {
	setSummaryViewport(t, image.Rectangle{})
	items := summaryItems(45)
	items[len(items)-1].Name = strings.Repeat("very long equipment description ", 20) + "LAST"
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewCombatSummaryDialog(game_state.CombatConclusion{Reason: game_state.CombatVictory, Collected: items}, keyboard, &sprites.SpriteSheet{}, func() {})
	items[0].Name, items[0].Quantity = "mutated", 0
	for _, size := range []image.Point{{960, 540}, {1280, 720}, {1920, 1080}, {2560, 1440}, {1280, 720}} {
		bounds := image.Rectangle{Max: size}
		viewport := image.Rect(0, 0, size.X*3/4, size.Y)
		dialog.prepareSummary(bounds, viewport)
		s := dialog.summary
		assert.NotContains(t, dialog.rows[0].GetOutputStr(false), "mutated")
		assert.Contains(t, s.values[0].GetOutputStr(false), "100")
		assert.Contains(t, dialog.rows[len(items)-1].GetOutputStr(false), "LAST", "long text must survive buffer wrapping")
		assert.Greater(t, s.rowHeights[len(items)-1], s.body.Dy(), "even a row larger than the page is scrollable")
		for range s.contentHeight/s.scrollStep + 2 {
			keyboard.SimulateKeyPress(ebiten.KeyDown)
			dialog.Update()
		}
		dialog.prepareSummary(bounds, viewport)
		assert.Equal(t, s.contentHeight-s.body.Dy(), s.scroll, "the bottom of the final entry is reachable")
		assert.Equal(t, s.body.Max.Y, s.body.Min.Y-s.scroll+s.contentHeight)
		keyboard.SimulateKeyPress(ebiten.KeyUp)
		dialog.Update()
		assert.Equal(t, s.contentHeight-s.body.Dy()-s.scrollStep, s.scroll)
		for range s.contentHeight/s.scrollStep + 2 {
			keyboard.SimulateKeyPress(ebiten.KeyUp)
			dialog.Update()
		}
		assert.Zero(t, s.scroll)
		assert.Contains(t, dialog.output.GetOutputStr(false), "Scroll")
		assert.Contains(t, dialog.output.GetOutputStr(false), "Continue", "footer survives repeated rebuilding")
	}
}

func TestCombatSummaryAcknowledgementDebounceExactOnceAndF4Ownership(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyNumpadEnter, ebiten.KeyEscape} {
		t.Run(key.String(), func(t *testing.T) {
			pressed := map[ebiten.Key]bool{key: true}
			keyboard := input.NewKeyboardWithKeyStateSource(0, func(candidate ebiten.Key) bool { return pressed[candidate] })
			dismissals := 0
			dialog := NewCombatSummaryDialog(game_state.CombatConclusion{Reason: game_state.CombatEvacuated}, keyboard, &sprites.SpriteSheet{}, func() { dismissals++ })
			dialog.Update()
			assert.False(t, dialog.IsDismissed(), "opening key must be released")
			pressed[key] = false
			keyboard.ClearFrameConsumption()
			dialog.Update()
			pressed[ebiten.KeyF4] = true
			dialog.Update()
			assert.False(t, dialog.IsDismissed())
			assert.True(t, keyboard.IsKeyPressedAndRegister(ebiten.KeyF4), "F4 remains available to the scene")
			pressed[ebiten.KeyF4], pressed[key] = false, true
			keyboard.ClearFrameConsumption()
			dialog.Update()
			require.True(t, dialog.IsDismissed())
			dialog.Update()
			pressed[key] = false
			keyboard.ClearFrameConsumption()
			dialog.Update()
			pressed[key] = true
			dialog.Update()
			assert.Equal(t, 1, dismissals)
		})
	}
}

func TestCombatSummaryAndDiscoveryGeometryRemainDistinct(t *testing.T) {
	setSummaryViewport(t, image.Rectangle{})
	sheet := &sprites.SpriteSheet{}
	bounds := image.Rect(0, 0, 1280, 720)
	viewport := image.Rect(0, 0, 960, 720)
	dialog := NewCombatSummaryDialog(game_state.CombatConclusion{Reason: game_state.CombatDefeat}, newWidgetTestKeyboard(0), sheet, func() {})
	dialog.prepareSummary(bounds, viewport)
	chest := NewCombatLootDialog([]game_state.CombatLootReveal{{Sprite: indexes.ItemMoney, Description: "a sack of gold!"}}, newWidgetTestKeyboard(0), sheet)
	chest.SetTrapNotice("Shamino took 7 acid damage!")
	chest.Reveal(1)
	chest.prepareDiscovery(bounds, viewport)
	assert.Nil(t, chest.summary)
	assert.Contains(t, chest.heading.GetOutputStr(false), "Chest contents")
	assert.Contains(t, chest.heading.GetOutputStr(false), "TRAP:")
	assert.Contains(t, chest.heading.GetOutputStr(false), "7 acid")
	assert.Contains(t, chest.output.GetOutputStr(false), "ground")
	assert.Contains(t, chest.output.GetOutputStr(false), "Revealing")
	assert.Equal(t, 900, chest.bounds.Dx(), "discovery retains the wide pane geometry")
	assert.InDelta(t, 360, float64(chest.bounds.Min.Y+chest.bounds.Max.Y)/2, 0.5)
	assert.Less(t, dialog.bounds.Dx(), chest.bounds.Dx())
}

func TestCombatSummaryUsesStandardDialogFrameRatherThanContentPadding(t *testing.T) {
	setSummaryViewport(t, image.Rectangle{})
	for _, size := range []image.Point{{960, 540}, {1280, 720}, {1920, 1080}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			screen := ebiten.NewImage(size.X, size.Y)
			t.Cleanup(screen.Deallocate)
			dialog := NewCombatSummaryDialog(game_state.CombatConclusion{Reason: game_state.CombatVictory},
				newWidgetTestKeyboard(0), &sprites.SpriteSheet{}, func() {})
			dialog.Draw(screen)
			require.Len(t, dialog.icons, 1)
			require.Nil(t, dialog.icons[0], "the empty summary needs no sprite bitmap")
			bounds := dialog.GetBounds()
			x := bounds.Min.X + bounds.Dx()/2
			stroke := 0
			for y := bounds.Min.Y; y < bounds.Max.Y && rgba(screen.At(x, y)) == rgba(DialogRuleColor); y++ {
				stroke++
			}
			assert.Equal(t, getClassicUIPixelUnit(size.X, size.Y), stroke,
				"frame should match standard dialogs rather than grow with content padding")
			assert.Greater(t, dialog.summary.padding, stroke,
				"text padding remains independent from the frame stroke")
		})
	}
}

// The experience section sits between the heading and the list: every member
// in two columns, or one line when an even split paid everyone the same.
func TestCombatSummaryExperienceSectionGridSharedAndAbsent(t *testing.T) {
	setGuidanceTestResolution(t, 1280, 720)
	viewport := image.Rect(16, 16, 720, 704)
	setSummaryViewport(t, viewport)
	members := []game_state.CombatExperienceShare{
		{Name: "Berad", Amount: 25}, {Name: "Shamino", Amount: 11}, {Name: "Mariah", Amount: 0},
		{Name: "Jaana", Amount: 3}, {Name: "Gwenno", Amount: 0},
	}
	build := func(experience game_state.CombatExperienceSummary) *CombatLootDialog {
		dialog := NewCombatSummaryDialog(game_state.CombatConclusion{
			Reason: game_state.CombatVictory, Collected: summaryItems(1), Experience: experience,
		}, newWidgetTestKeyboard(0), &sprites.SpriteSheet{}, func() {})
		dialog.prepareSummary(image.Rect(0, 0, 1280, 720), viewport)

		return dialog
	}

	plain := build(game_state.CombatExperienceSummary{})
	assert.Zero(t, plain.summary.experience.height, "a battle that earned nothing shows no experience")
	assert.Nil(t, plain.summary.experience.heading)

	grid := build(game_state.CombatExperienceSummary{
		Members: members, Ready: []game_state.ExperienceLevelReady{{Name: "Jaana", Level: 5}},
	})
	section := grid.summary.experience
	require.NotNil(t, section.heading)
	assert.Contains(t, section.heading.GetOutputStr(false), "Experience", "the purple figures are named in words")
	require.Len(t, section.names, len(members))
	assert.Equal(t, 3, section.gridRows(), "five members fill three rows of two")
	assert.Contains(t, section.amounts[0].GetOutputStr(false), "+25")
	assert.Contains(t, section.amounts[2].GetOutputStr(false), "-", "a member who earned nothing is still listed")
	require.Len(t, section.ready, 1)
	assert.Contains(t, section.ready[0].GetOutputStr(false), "Jaana is ready for level 5.")
	assert.Nil(t, section.each)
	assert.Equal(t, plain.bounds.Dy()+section.height, grid.bounds.Dy(), "the section adds exactly its own height")
	assert.Equal(t, plain.summary.body.Dy(), grid.summary.body.Dy(), "and the collected list keeps its room")
	assert.True(t, grid.bounds.In(viewport))

	shared := build(game_state.CombatExperienceSummary{Members: members[:2], Shared: true, Each: 4})
	require.NotNil(t, shared.summary.experience.each)
	assert.Contains(t, shared.summary.experience.each.GetOutputStr(false), "+4 each")
	assert.Empty(t, shared.summary.experience.names, "an even split needs one line, not a grid")
	assert.Less(t, shared.summary.experience.height, section.height)
}
