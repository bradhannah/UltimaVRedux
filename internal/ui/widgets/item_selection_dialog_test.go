package widgets

import (
	"image"
	"image/color"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestItemSelectionDialogMandatoryConfigPanics(t *testing.T) {
	valid := testItemSelectionConfig(newWidgetTestKeyboard(0), testUseItems())
	tests := []struct {
		name   string
		mutate func(*ItemSelectionDialogConfig)
		panic  string
	}{
		{
			name:   "keyboard",
			mutate: func(config *ItemSelectionDialogConfig) { config.Keyboard = nil },
			panic:  "ItemSelectionDialog: keyboard is required",
		},
		{
			name:   "select callback",
			mutate: func(config *ItemSelectionDialogConfig) { config.OnSelect = nil },
			panic:  "ItemSelectionDialog: OnSelect is required",
		},
		{
			name:   "cancel callback",
			mutate: func(config *ItemSelectionDialogConfig) { config.OnCancel = nil },
			panic:  "ItemSelectionDialog: OnCancel is required",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := valid
			test.mutate(&config)
			require.PanicsWithValue(t, test.panic, func() {
				NewItemSelectionDialog(config)
			})
		})
	}
}

func TestItemSelectionDialogOnlyBuildsNonEmptyTabsIncludingKeys(t *testing.T) {
	setItemDialogTestResolution(t, 320, 200)
	items := []game_state.UseableItemInfo{
		testUseItem("Crown", game_state.UseItemCategoryQuest, references.Crown),
		testUseItem("Yellow", game_state.UseItemCategoryPotion, references.Yellow),
		testUseItem("Skull Keys", game_state.UseItemCategoryKeys, references.SkullKeys),
	}
	dialog := NewItemSelectionDialog(testItemSelectionConfig(newWidgetTestKeyboard(0), items))

	require.Equal(t, []ItemCategory{
		ItemCategoryAll,
		ItemCategoryQuest,
		ItemCategoryPotion,
		ItemCategoryKeys,
	}, dialog.categories)
	require.Len(t, dialog.layout.tabRects, len(dialog.categories))
	for _, rect := range dialog.layout.tabRects {
		require.False(t, rect.Empty())
		require.True(t, rect.In(dialog.layout.headerRect))
	}
	assert.NotContains(t, dialog.categories, ItemCategoryScroll)
	assert.NotContains(t, dialog.categories, ItemCategoryMoonstone)
}

func TestItemSelectionDialogNavigationPreservesPerTabSelectionAndScroll(t *testing.T) {
	setItemDialogTestResolution(t, 320, 200)
	items := make([]game_state.UseableItemInfo, 0, 42)
	for index := range 40 {
		item := testUseItem("Quest "+string(rune('A'+index)), game_state.UseItemCategoryQuest, references.Crown)
		items = append(items, item)
	}
	items = append(items,
		testUseItem("Yellow", game_state.UseItemCategoryPotion, references.Yellow),
		testUseItem("Red", game_state.UseItemCategoryPotion, references.Red),
	)
	dialog := NewItemSelectionDialog(testItemSelectionConfig(newWidgetTestKeyboard(0), items))

	for range 35 {
		dialog.moveSelection(1)
	}
	allSelection := dialog.GetSelectedItemIndex()
	allScroll := dialog.scrollStarts[ItemCategoryAll]
	require.Equal(t, 35, allSelection)
	require.Positive(t, allScroll)

	dialog.switchCategoryBy(1)
	require.Equal(t, ItemCategoryQuest, dialog.currentCategory())
	dialog.moveSelection(2)
	require.Equal(t, 2, dialog.GetSelectedItemIndex())
	dialog.switchCategoryBy(-1)
	require.Equal(t, ItemCategoryAll, dialog.currentCategory())
	assert.Equal(t, allSelection, dialog.GetSelectedItemIndex())
	assert.Equal(t, allScroll, dialog.scrollStarts[ItemCategoryAll])
	assert.Equal(t, -1, itemDialogCategoryDelta(true))
	assert.Equal(t, 1, itemDialogCategoryDelta(false))
}

// A name too long for the list wraps whole onto further lines. It used to show
// only its last word, so every Crown Jewel read "British".
func TestItemSelectionLongNamesWrapWholeAndStayOnScreen(t *testing.T) {
	setItemDialogTestResolution(t, 1280, 720)
	items := make([]game_state.UseableItemInfo, 0, 40)
	for range 20 {
		items = append(items,
			testUseItem("Amulet of Lord British", game_state.UseItemCategoryQuest, references.Amulet),
			testUseItem("Crown", game_state.UseItemCategoryQuest, references.Crown))
	}
	dialog := NewItemSelectionDialog(testItemSelectionConfig(newWidgetTestKeyboard(0), items))
	line := dialog.rowLineHeight()
	require.Equal(t, line, dialog.rowHeight(items[1]), "a short name keeps one line")
	require.Greater(t, dialog.rowHeight(items[0]), line, "the long name wraps")
	require.Equal(t, strings.Fields(items[0].Name), strings.Fields(dialog.rowOutput.GetOutputStr(false)),
		"the whole name is shown")

	room := dialog.listContentRect().Dy()
	for range len(items) - 1 {
		dialog.moveSelection(1)
		start, selection := dialog.scrollStarts[ItemCategoryAll], dialog.GetSelectedItemIndex()
		require.LessOrEqual(t, start, selection)
		height := 0
		for index := start; index <= selection; index++ {
			height += dialog.rowHeight(items[index])
		}
		require.LessOrEqual(t, height, room, "the selected row is wholly on screen")
	}
	require.Positive(t, dialog.scrollStarts[ItemCategoryAll], "tall rows scroll the list")
}

func TestItemSelectionDialogLongLoreSharesNormalDetailsAndPagesWithoutLoss(t *testing.T) {
	setItemDialogTestResolution(t, 320, 200)
	item := testUseItem("Shard", game_state.UseItemCategoryQuest, references.ShardOfFalsehood)
	item.FullName = "Shard of Falsehood"
	item.EffectSummary, item.Requirement, item.Consumption = "", "", ""
	item.Description = strings.Repeat("Long lore remains readable at the standard font size. ", 60)
	dialog := NewItemSelectionDialog(testItemSelectionConfig(newWidgetTestKeyboard(0), []game_state.UseableItemInfo{item}))

	pages := dialog.detailPages()
	require.Greater(t, len(pages), 2)
	require.Contains(t, pages[0][0].value, "Long lore")
	var recovered strings.Builder
	for _, page := range pages {
		for _, line := range page {
			recovered.WriteString(line.value)
			recovered.WriteByte(' ')
		}
	}
	require.Equal(t, strings.Fields(item.Description), strings.Fields(recovered.String()))
	capacity := dialog.detailOutput.GetLineCapacity(dialog.detailTextRect().Dy())
	for _, page := range pages {
		require.LessOrEqual(t, dialog.detailLineCount(page), capacity)
	}

	keyboard := dialog.keyboard
	pressItemDialogKey(dialog, keyboard, ebiten.KeySpace)
	require.Equal(t, 1, dialog.GetDetailPage())
	dialog.moveSelection(0)
	require.Equal(t, 1, dialog.GetDetailPage(), "a bounded no-op does not discard the page")
}

func TestItemSelectionDialogAllowsWarningSelectionAndCancelsExactlyOnce(t *testing.T) {
	item := testUseItem("Purple", game_state.UseItemCategoryPotion, references.Purple)
	item.AvailableNow = false
	item.Warning = "No noticeable effect outside combat; it is still consumed."
	keyboard := newWidgetTestKeyboard(0)
	selected := make([]game_state.UseableItemInfo, 0, 1)
	cancelCalls := 0
	config := testItemSelectionConfig(keyboard, []game_state.UseableItemInfo{item})
	config.OnSelect = func(result game_state.UseableItemInfo) { selected = append(selected, result) }
	config.OnCancel = func() { cancelCalls++ }
	dialog := NewItemSelectionDialog(config)

	pressItemDialogKey(dialog, keyboard, ebiten.KeyNumpadEnter)
	require.True(t, dialog.IsDismissed())
	require.Len(t, selected, 1)
	assert.Equal(t, references.Item(references.Purple), selected[0].ItemType)
	assert.Zero(t, cancelCalls)
	dialog.HandleEnter()
	assert.Len(t, selected, 1)

	cancelDialog := NewItemSelectionDialog(config)
	pressItemDialogKey(cancelDialog, keyboard, ebiten.KeyEscape)
	cancelDialog.Dismiss()
	assert.Equal(t, 1, cancelCalls)
}

func TestItemSelectionDialogUnavailableEnterStaysOpenAndShowsReason(t *testing.T) {
	for _, key := range []ebiten.Key{ebiten.KeyEnter, ebiten.KeyNumpadEnter} {
		item := testUseItem("Resurrection", game_state.UseItemCategoryScroll, references.ScrollInManiCorp)
		item.UnavailableReason = "Cannot resurrect during combat."
		item.AvailableNow = false
		keyboard := newWidgetTestKeyboard(0)
		config := testItemSelectionConfig(keyboard, []game_state.UseableItemInfo{item})
		config.OnSelect = func(game_state.UseableItemInfo) { t.Fatal("unavailable activation dispatched") }
		dialog := NewItemSelectionDialog(config)
		dialog.detailPage = 1
		pressItemDialogKey(dialog, keyboard, key)
		require.False(t, dialog.IsDismissed())
		require.Zero(t, dialog.detailPage)
		require.Equal(t, item, *dialog.selectedItem())
		require.Empty(t, dialog.actionDetailLines(item)[0].label)
		require.Equal(t, item.UnavailableReason, dialog.actionDetailLines(item)[0].value)
		require.Equal(t, "[No] Resurrection", unavailableRowLabel(item.Name, item.UnavailableReason == ""))
		pressItemDialogKey(dialog, keyboard, ebiten.KeyEscape)
		require.True(t, dialog.IsDismissed())
	}
}

func TestItemSelectionListsUsableFirstAndOpensOnIt(t *testing.T) {
	sextant := testUseItem("Sextant", game_state.UseItemCategoryQuest, references.Sextant)
	sextant.UnavailableReason = "Use on the world map."
	yellow := testUseItem("Yellow", game_state.UseItemCategoryPotion, references.Yellow)
	watch := testUseItem("Watch", game_state.UseItemCategoryTools, references.PocketWatch)
	locked := testUseItem("Spare Scroll", game_state.UseItemCategoryScroll, references.ScrollVasLor)
	locked.UnavailableReason = "Not during battle."
	dialog := NewItemSelectionDialog(testItemSelectionConfig(
		newWidgetTestKeyboard(0), []game_state.UseableItemInfo{sextant, yellow, locked, watch}))
	var names []string
	for _, item := range dialog.currentItems() {
		names = append(names, item.Name)
	}
	require.Equal(t, []string{"Yellow", "Watch", "Sextant", "Spare Scroll"}, names)
	require.Equal(t, "Yellow", dialog.selectedItem().Name)
	require.Equal(t, "Sextant", dialog.itemsByCategory[ItemCategoryQuest][0].Name)
}

func TestItemSelectionUnavailableMarkerQuantityAndFocusRemainReadable(t *testing.T) {
	for _, size := range []image.Point{{X: 320, Y: 200}, {X: 640, Y: 480}, {X: 1280, Y: 800}} {
		setItemDialogTestResolution(t, size.X, size.Y)
		item := testUseItem("Resurrection scroll with a long name", game_state.UseItemCategoryScroll, references.ScrollInManiCorp)
		item.Quantity = 99
		item.UnavailableReason = "Cannot resurrect during combat."
		item.AvailableNow = false
		dialog := NewItemSelectionDialog(testItemSelectionConfig(newWidgetTestKeyboard(0), []game_state.UseableItemInfo{item}))
		content := dialog.listContentRect()
		height := int(math.Ceil(dialog.rowOutput.GetLineSpacing()))
		rect := image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+height)
		markerWidth, _ := dialog.font.MeasureString("[No]")
		quantityWidth, _ := dialog.font.MeasureString("99")
		marker := image.Rect(rect.Min.X+dialog.layout.padding, rect.Min.Y,
			rect.Min.X+dialog.layout.padding+int(math.Ceil(markerWidth)), rect.Max.Y)
		right := rect.Max.X - dialog.layout.padding - dialog.listIndicatorWidth()
		quantity := image.Rect(right-int(math.Ceil(quantityWidth)), rect.Min.Y, right, rect.Max.Y)
		require.Less(t, marker.Max.X+dialog.layout.padding, quantity.Min.X, "marker must fit beside the quantity")
		for _, selected := range []bool{false, true} {
			screen := ebiten.NewImage(size.X, size.Y)
			screen.Fill(MerchantDialogBackgroundColor)
			dialog.drawDetailHeader(screen, item)
			dialog.drawHeader(screen)
			require.NotContains(t, dialog.titleOutput.GetOutputStr(false), "Unavailable", "the title carries no legend")
			dialog.drawListRow(screen, rect, item, selected)
			// Keep exactly the same glyphs and geometry, but render the control as an available row.
			// Its brighter text exposes glyph coverage even when no small glyph pixel is fully opaque.
			control := ebiten.NewImage(size.X, size.Y)
			control.Fill(MerchantDialogBackgroundColor)
			available := item
			available.Name = unavailableRowLabel(item.Name, item.UnavailableReason == "")
			available.UnavailableReason = ""
			available.AvailableNow = true
			dialog.drawListRow(control, rect, available, false)
			var want color.Color = pkgcolor.UITextDisabled.RGBA()
			background := MerchantDialogBackgroundColor
			if selected {
				want = DialogSelectedText
				background = DialogSelectedBackground
				require.NotEqual(t, DialogSelectedBackground, DialogSelectedText)
				requireColorRGBA(t, DialogSelectedBackground, screen.At(rect.Min.X, rect.Min.Y))
			}
			requireUseRowInk(t, screen, control, marker,
				background, want, MerchantDialogBackgroundColor, DialogItemNameColor)
			requireUseRowInk(t, screen, control, quantity,
				background, want, MerchantDialogBackgroundColor, DialogItemQuantityColor)
		}
	}
}

func requireUseRowInk(
	t *testing.T,
	screen, control *ebiten.Image,
	rect image.Rectangle,
	background, want, controlBackground, controlColor color.Color,
) {
	t.Helper()
	rgb := func(value color.Color) [3]float64 {
		r, g, b, _ := value.RGBA()

		return [3]float64{float64(r >> 8), float64(g >> 8), float64(b >> 8)}
	}
	base, foreground, controlForeground := rgb(background), rgb(want), rgb(controlColor)
	controlBase := rgb(controlBackground)
	channel := 0
	for candidate := 1; candidate < len(base); candidate++ {
		if math.Abs(controlForeground[candidate]-controlBase[candidate]) >
			math.Abs(controlForeground[channel]-controlBase[channel]) {
			channel = candidate
		}
	}
	require.NotEqual(t, controlBase[channel], controlForeground[channel], "control text must contrast with its background")
	maxInk, maxBlendError := 0.0, 0.0
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			actual, reference := rgb(screen.At(x, y)), rgb(control.At(x, y))
			coverage := (reference[channel] - controlBase[channel]) /
				(controlForeground[channel] - controlBase[channel])
			for component := range base {
				expected := base[component] + coverage*(foreground[component]-base[component])
				maxBlendError = math.Max(maxBlendError, math.Abs(actual[component]-expected))
				maxInk = math.Max(maxInk, math.Abs(actual[component]-base[component]))
			}
		}
	}
	// Allow 8-bit raster/compositing rounding, not arbitrary colours or an empty region.
	require.LessOrEqual(t, maxBlendError, 3.0, "wrong semantic text colour or glyph placement in %v", rect)
	require.Greater(t, maxInk, 3.0, "no visible marker/quantity contrast in %v", rect)
}

func TestItemSelectionCategoriesIncludeUnavailableOwnershipAndWarningRowsStayActionable(t *testing.T) {
	items := testUseItems()
	for index := range items {
		items[index].UnavailableReason = "This interaction is not supported here."
		items[index].AvailableNow = false
		items[index].Quantity = index + 1
	}
	dialog := NewItemSelectionDialog(testItemSelectionConfig(newWidgetTestKeyboard(0), items))
	require.Len(t, dialog.categories, itemCategoryCount)
	require.Len(t, dialog.currentItems(), len(items))
	for _, category := range dialog.categories[1:] {
		require.NotEmpty(t, dialog.itemsByCategory[category])
	}
	dialog.moveSelection(3)
	selected := *dialog.selectedItem()
	dialog.HandleEnter()
	require.False(t, dialog.IsDismissed())
	require.Equal(t, selected, *dialog.selectedItem())

	warning := testUseItem("White", game_state.UseItemCategoryPotion, references.White)
	warning.AvailableNow = false
	warning.Warning = "No noticeable effect in combat; it is still consumed."
	warning.Consumption = "One potion is consumed before its effect is known."
	selectedCount := 0
	config := testItemSelectionConfig(newWidgetTestKeyboard(0), []game_state.UseableItemInfo{warning})
	config.OnSelect = func(game_state.UseableItemInfo) { selectedCount++ }
	warningDialog := NewItemSelectionDialog(config)
	require.Equal(t, "White", unavailableRowLabel(warning.Name, warning.UnavailableReason == ""))
	require.Contains(t, warningDialog.actionDetailLines(warning), detailLine{
		label: "", value: warning.Warning, color: DialogWarningColor, header: false,
	})
	warningDialog.HandleEnter()
	require.True(t, warningDialog.IsDismissed())
	require.Equal(t, 1, selectedCount)
}

func TestItemSelectionDialogIgnoresNumberShortcutsAndUsesResponsiveMainFont(t *testing.T) {
	setItemDialogTestResolution(t, 320, 200)
	keyboard := newWidgetTestKeyboard(0)
	dialog := NewItemSelectionDialog(testItemSelectionConfig(keyboard, testUseItems()))
	dialog.ensureLayout()
	smallSize := dialog.font.TextFace.Size

	pressItemDialogKey(dialog, keyboard, ebiten.Key1)
	assert.Zero(t, dialog.GetSelectedCategory(), "number keys do not select categories")
	require.Greater(t, dialog.titleOutput.Font.TextFace.Size, smallSize, "the window title uses the title size")
	for _, size := range []float64{
		dialog.itemOutput.Font.TextFace.Size,
		dialog.tabOutput.Font.TextFace.Size,
		dialog.rowOutput.Font.TextFace.Size,
		dialog.detailOutput.Font.TextFace.Size,
		dialog.pageOutput.Font.TextFace.Size,
		dialog.footerOutput.Font.TextFace.Size,
	} {
		require.Equal(t, smallSize, size)
	}

	dialog.moveSelection(2)
	dialog.detailPage = min(1, dialog.GetDetailPageCount()-1)
	setResponsiveDialogTestResolution(t, 1280, 720)
	dialog.ensureLayout()
	require.Equal(t, 2, dialog.GetSelectedItemIndex())
	require.Less(t, dialog.GetDetailPage(), dialog.GetDetailPageCount())
	require.Greater(t, dialog.font.TextFace.Size, smallSize)
	largeBounds := dialog.outerBorder.GetBounds()
	require.InDelta(t, 1280*itemDialogMarginX, largeBounds.Min.X, 1)
	require.InDelta(t, 720*itemDialogMarginY, largeBounds.Min.Y, 1)
	require.InDelta(t, 1280*(1-itemDialogMarginX), largeBounds.Max.X, 1)
	require.InDelta(t, 720*(1-itemDialogMarginY), largeBounds.Max.Y, 1)
	drawn := dialogImage(1280, 720, dialog)
	t.Cleanup(drawn.Dispose)
	assert.NotEqual(t, uint32(0), alphaAt(drawn, dialog.layout.panelRect.Min.Add(image.Pt(4, 4))))
	overlap := dialog.pixelUnit()
	require.Equal(t, dialog.layout.headerRect.Max.Y-overlap, dialog.layout.bodyRect.Min.Y)
	require.Equal(t, dialog.layout.bodyRect.Max.Y-overlap, dialog.layout.footerRect.Min.Y)
	require.Equal(t, dialog.layout.listRect.Max.X-overlap, dialog.layout.detailRect.Min.X)
	panePerimeterPoint := image.Pt(
		dialog.layout.headerRect.Min.X+dialog.layout.headerRect.Dx()/2,
		dialog.layout.headerRect.Min.Y,
	)
	requireColorRGBA(t, DialogRuleColor, drawn.At(panePerimeterPoint.X, panePerimeterPoint.Y))
	paneCorner := dialog.layout.headerRect.Min
	requireColorNotRGBA(t, MerchantDialogBackgroundColor, drawn.At(paneCorner.X, paneCorner.Y))
	legacyOuterPoint := image.Pt(largeBounds.Min.X+largeBounds.Dx()/2, largeBounds.Min.Y)
	requireColorNotRGBA(t, DialogRuleColor, drawn.At(legacyOuterPoint.X, legacyOuterPoint.Y))
}

func testItemSelectionConfig(
	keyboard *input.Keyboard,
	items []game_state.UseableItemInfo,
) ItemSelectionDialogConfig {
	return ItemSelectionDialogConfig{
		Items: items, Keyboard: keyboard, OnSelect: func(game_state.UseableItemInfo) {}, OnCancel: func() {},
	}
}

func testUseItems() []game_state.UseableItemInfo {
	return []game_state.UseableItemInfo{
		testUseItem("Crown", game_state.UseItemCategoryQuest, references.Crown),
		testUseItem("Sceptre", game_state.UseItemCategoryQuest, references.Sceptre),
		testUseItem("Shard", game_state.UseItemCategoryQuest, references.ShardOfFalsehood),
		testUseItem("Yellow", game_state.UseItemCategoryPotion, references.Yellow),
		testUseItem("Vas Lor", game_state.UseItemCategoryScroll, references.ScrollVasLor),
		testUseItem("Watch", game_state.UseItemCategoryTools, references.PocketWatch),
		testUseItem("New Moon", game_state.UseItemCategoryMoonstone, references.NewMoon),
		testUseItem("Skull Keys", game_state.UseItemCategoryKeys, references.SkullKeys),
	}
}

func testUseItem(
	name string,
	category game_state.UseItemCategory,
	item references.Item,
) game_state.UseableItemInfo {
	return game_state.UseableItemInfo{
		Name:          name,
		FullName:      name,
		Description:   "Lore for " + name + ".",
		EffectSummary: "Produces the expected effect.",
		Requirement:   "Ready to use.",
		Consumption:   "Not consumed.",
		Warning:       "",
		AvailableNow:  true,
		Category:      category,
		UsageContext:  game_state.UsageField,
		Quantity:      1,
		SpriteIndex:   indexes.ItemKey,
		ItemRef:       references.InventoryItem{},
		ItemType:      item,
	}
}

func pressItemDialogKey(dialog *ItemSelectionDialog, keyboard *input.Keyboard, key ebiten.Key) {
	keyboard.SimulateKeyPress(key)
	dialog.Update()
}

func setItemDialogTestResolution(t *testing.T, width, height int) {
	t.Helper()
	setResponsiveDialogTestResolution(t, width, height)
}

func dialogImage(width, height int, dialog *ItemSelectionDialog) *ebiten.Image {
	screen := ebiten.NewImage(width, height)
	dialog.Draw(screen)

	return screen
}
