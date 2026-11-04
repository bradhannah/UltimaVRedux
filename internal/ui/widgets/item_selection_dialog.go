package widgets

import (
	"fmt"
	"image"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	itemCategoryCount                = 7
	itemDialogHeaderMinimumLines     = 2
	itemDialogFooterLines            = 2
	itemDialogMinimumTextChars       = 8
	itemDialogDetailMaxLines         = 128
	itemDialogCenterDivisor          = 2
	itemDialogPaneInsetMultiplier    = 2
	itemDialogLineSpacingMultiplier  = 1.15
	itemDialogMinimumTabsPerRow      = 1
	itemDialogSpriteMaximumScale     = 2
	itemDialogQuantityMinimumColumns = 4
	itemDialogHalfPageDivisor        = 2
	itemDialogTitleMaxLines          = 2
	itemDialogDetailHeaderLines      = 2
)

var _ Widget = (*ItemSelectionDialog)(nil)

// ItemCategory identifies a non-empty tab in the Use item picker.
type ItemCategory int

const (
	// ItemCategoryAll shows every usable identity.
	ItemCategoryAll ItemCategory = iota
	// ItemCategoryQuest shows quest artifacts and plot items.
	ItemCategoryQuest
	// ItemCategoryPotion shows coloured potions.
	ItemCategoryPotion
	// ItemCategoryScroll shows magical scrolls.
	ItemCategoryScroll
	// ItemCategoryTools shows general-purpose tools.
	ItemCategoryTools
	// ItemCategoryMoonstone shows the eight moonstones.
	ItemCategoryMoonstone
	// ItemCategoryKeys shows Skull Keys.
	ItemCategoryKeys
)

// String returns the display name for the category.
func (c ItemCategory) String() string {
	switch c {
	case ItemCategoryAll:
		return "All"
	case ItemCategoryQuest:
		return "Artifacts"
	case ItemCategoryPotion:
		return "Potions"
	case ItemCategoryScroll:
		return "Scrolls"
	case ItemCategoryTools:
		return "Tools"
	case ItemCategoryMoonstone:
		return "Moonstones"
	case ItemCategoryKeys:
		return "Keys"
	default:
		panic("unknown Use item category")
	}
}

// ItemSelectionDialogConfig configures the presentation-only Use item picker.
type ItemSelectionDialogConfig struct {
	Items       []game_state.UseableItemInfo
	Keyboard    *input.Keyboard
	SpriteSheet *sprites.SpriteSheet
	OnSelect    func(game_state.UseableItemInfo)
	OnCancel    func()
}

type itemSelectionLayout struct {
	panelRect        image.Rectangle
	headerRect       image.Rectangle
	titleRect        image.Rectangle
	tabsRect         image.Rectangle
	bodyRect         image.Rectangle
	listRect         image.Rectangle
	detailRect       image.Rectangle
	detailHeaderRect image.Rectangle
	detailTextRect   image.Rectangle
	detailPageRect   image.Rectangle
	footerRect       image.Rectangle
	tabRects         []image.Rectangle
	padding          int
}

// ItemSelectionDialog presents a responsive keyboard-only master/detail Use picker.
type ItemSelectionDialog struct {
	navigation
	itemsByCategory map[ItemCategory][]game_state.UseableItemInfo
	allItems        []game_state.UseableItemInfo
	categories      []ItemCategory
	selections      [itemCategoryCount]int
	scrollStarts    [itemCategoryCount]int
	activeCategory  int
	detailPage      int

	keyboard    *input.Keyboard
	spriteSheet *sprites.SpriteSheet
	onSelect    func(game_state.UseableItemInfo)
	onCancel    func()
	dismissed   bool

	outerBorder  *Border
	font         *text.UltimaFont
	titleOutput  *text.Output
	itemOutput   *text.Output
	tabOutput    *text.Output
	rowOutput    *text.Output
	detailOutput *text.Output
	pageOutput   *text.Output
	footerOutput *text.Output
	layout       itemSelectionLayout

	lastScreenWidth  int
	lastScreenHeight int
}

// NewItemSelectionDialog creates a modal over a detached authoritative item projection.
func NewItemSelectionDialog(config ItemSelectionDialogConfig) *ItemSelectionDialog {
	if config.Keyboard == nil {
		panic("ItemSelectionDialog: keyboard is required")
	}
	if config.OnSelect == nil {
		panic("ItemSelectionDialog: OnSelect is required")
	}
	if config.OnCancel == nil {
		panic("ItemSelectionDialog: OnCancel is required")
	}

	dialog := &ItemSelectionDialog{ //nolint:exhaustruct_v5 // Responsive outputs and layout are initialized below.
		navigation:      newNavigation(config.Keyboard.IsKeyHeld),
		itemsByCategory: make(map[ItemCategory][]game_state.UseableItemInfo),
		allItems:        append([]game_state.UseableItemInfo(nil), config.Items...),
		keyboard:        config.Keyboard,
		spriteSheet:     config.SpriteSheet,
		onSelect:        config.OnSelect,
		onCancel:        config.OnCancel,
	}
	dialog.organizeItems()
	dialog.ensureLayout()

	return dialog
}

func (d *ItemSelectionDialog) organizeItems() {
	for _, item := range d.allItems {
		category := widgetCategoryForUseItem(item.Category)
		d.itemsByCategory[category] = append(d.itemsByCategory[category], item)
	}
	if len(d.allItems) == 0 {
		return
	}
	d.categories = append(d.categories, ItemCategoryAll)
	for _, category := range []ItemCategory{
		ItemCategoryQuest,
		ItemCategoryPotion,
		ItemCategoryScroll,
		ItemCategoryTools,
		ItemCategoryMoonstone,
		ItemCategoryKeys,
	} {
		if len(d.itemsByCategory[category]) > 0 {
			d.categories = append(d.categories, category)
		}
	}
}

func widgetCategoryForUseItem(category game_state.UseItemCategory) ItemCategory {
	switch category {
	case game_state.UseItemCategoryQuest:
		return ItemCategoryQuest
	case game_state.UseItemCategoryPotion:
		return ItemCategoryPotion
	case game_state.UseItemCategoryScroll:
		return ItemCategoryScroll
	case game_state.UseItemCategoryTools:
		return ItemCategoryTools
	case game_state.UseItemCategoryMoonstone:
		return ItemCategoryMoonstone
	case game_state.UseItemCategoryKeys:
		return ItemCategoryKeys
	default:
		panic("unknown game-state Use item category")
	}
}

// Update processes one keyboard action while the picker owns modal focus.
func (d *ItemSelectionDialog) Update() {
	if d.dismissed {
		return
	}
	d.ensureLayout()

	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.dismissed = true
		d.onCancel()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) ||
		d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		d.HandleEnter()

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		d.switchCategoryBy(itemDialogCategoryDelta(d.keyboard.IsModifierPressed(input.ModifierShift)))

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft) {
		d.switchCategoryBy(-1)

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight) {
		d.switchCategoryBy(1)

		return
	}
	if key, owned := d.Poll(d.keyboard); owned {
		if key != nil {
			d.moveSelection(listNavigationDelta(*key, d.pageJump()))
		}

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		pageCount := d.GetDetailPageCount()
		if pageCount > 1 {
			d.detailPage = (d.detailPage + 1) % pageCount
		}
	}
}

func itemDialogCategoryDelta(shiftPressed bool) int {
	if shiftPressed {
		return -1
	}

	return 1
}

func (d *ItemSelectionDialog) switchCategoryBy(delta int) {
	d.Reset()
	if len(d.categories) == 0 {
		return
	}
	d.activeCategory = (d.activeCategory + delta + len(d.categories)) % len(d.categories)
	d.clampCurrentSelection()
	d.ensureCurrentSelectionVisible()
	d.detailPage = 0
}

func (d *ItemSelectionDialog) moveSelection(delta int) {
	items := d.currentItems()
	if len(items) == 0 {
		return
	}
	category := d.currentCategory()
	selection := max(0, min(d.selections[category]+delta, len(items)-1))
	if selection == d.selections[category] {
		return
	}
	d.selections[category] = selection
	d.ensureCurrentSelectionVisible()
	d.detailPage = 0
}

func (d *ItemSelectionDialog) pageJump() int {
	return max(1, d.visibleRowCount()/itemDialogHalfPageDivisor)
}

func (d *ItemSelectionDialog) clampAllSelections() {
	active := d.activeCategory
	for index := range d.categories {
		d.activeCategory = index
		d.clampCurrentSelection()
		d.ensureCurrentSelectionVisible()
	}
	d.activeCategory = active
}

func (d *ItemSelectionDialog) clampCurrentSelection() {
	if len(d.categories) == 0 {
		return
	}
	category := d.currentCategory()
	count := len(d.currentItems())
	if count == 0 {
		d.selections[category] = 0
		d.scrollStarts[category] = 0

		return
	}
	d.selections[category] = max(0, min(d.selections[category], count-1))
}

func (d *ItemSelectionDialog) ensureCurrentSelectionVisible() {
	if len(d.categories) == 0 {
		return
	}
	category := d.currentCategory()
	count := len(d.currentItems())
	visibleRows := d.visibleRowCount()
	maxStart := max(0, count-visibleRows)
	start := max(0, min(d.scrollStarts[category], maxStart))
	selection := d.selections[category]
	if selection < start {
		start = selection
	} else if selection >= start+visibleRows {
		start = selection - visibleRows + 1
	}
	d.scrollStarts[category] = max(0, min(start, maxStart))
}

func (d *ItemSelectionDialog) currentCategory() ItemCategory {
	if len(d.categories) == 0 {
		return ItemCategoryAll
	}

	return d.categories[d.activeCategory]
}

func (d *ItemSelectionDialog) currentItems() []game_state.UseableItemInfo {
	if len(d.categories) == 0 {
		return nil
	}
	category := d.currentCategory()
	if category == ItemCategoryAll {
		return d.allItems
	}

	return d.itemsByCategory[category]
}

func (d *ItemSelectionDialog) selectedItem() *game_state.UseableItemInfo {
	items := d.currentItems()
	if len(items) == 0 {
		return nil
	}
	index := d.selections[d.currentCategory()]
	if index < 0 || index >= len(items) {
		return nil
	}

	return &items[index]
}

// HandleEnter submits permitted attempts; unavailable entries remain inspectable.
func (d *ItemSelectionDialog) HandleEnter() {
	if d.dismissed {
		return
	}
	item := d.selectedItem()
	if item == nil {
		return
	}
	if item.UnavailableReason != "" {
		d.detailPage = 0 // Show the concrete restriction rather than dismissing the picker.

		return
	}
	d.dismissed = true
	d.onSelect(*item)
}

// Draw renders the dimmed surround and responsive Use panels.
func (d *ItemSelectionDialog) Draw(screen *ebiten.Image) {
	if screen == nil || d.dismissed {
		return
	}
	d.ensureLayout()

	bounds := screen.Bounds()
	vector.FillRect(
		screen,
		float32(bounds.Min.X),
		float32(bounds.Min.Y),
		float32(bounds.Dx()),
		float32(bounds.Dy()),
		pkgcolor.UIBackgroundSemi.RGBA(),
		false,
	)
	for _, rect := range []image.Rectangle{
		d.layout.headerRect,
		d.layout.listRect,
		d.layout.detailRect,
		d.layout.footerRect,
	} {
		FillClassicPixelRoundedRect(screen, rect, d.pixelUnit(), MerchantDialogBackgroundColor)
	}
	d.drawHeader(screen)
	d.drawList(screen)
	d.drawDetail(screen)
	d.drawFooter(screen)
	for _, rect := range []image.Rectangle{
		d.layout.headerRect,
		d.layout.listRect,
		d.layout.detailRect,
		d.layout.footerRect,
	} {
		DrawClassicPixelRoundedFrame(screen, rect, d.pixelUnit(), DialogRuleColor)
	}
}

func (d *ItemSelectionDialog) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if d.outerBorder != nil && screenWidth == d.lastScreenWidth && screenHeight == d.lastScreenHeight {
		return
	}

	previousPage := d.detailPage
	d.lastScreenWidth = screenWidth
	d.lastScreenHeight = screenHeight
	d.outerBorder = NewOpaqueBorder(itemDialogPlacement(), BorderWidthStandard, MerchantDialogBackgroundColor)
	d.rebuildOutputs()
	d.calculateLayout()
	d.clampAllSelections()
	pageCount := len(d.detailPages())
	d.detailPage = min(previousPage, max(0, pageCount-1))
}

func (d *ItemSelectionDialog) rebuildOutputs() {
	config := FontConfig{
		Size:                  FontSizeTitle,
		LineSpacing:           0,
		LineSpacingMultiplier: itemDialogLineSpacingMultiplier,
		MaxLines:              1,
		MaxChars:              itemDialogMinimumTextChars,
	}
	_, d.titleOutput = GetScaledFontWithOutput(config)
	config.Size = FontSizeBody
	d.font, d.tabOutput = GetScaledFontWithOutput(config)
	_, d.rowOutput = GetScaledFontWithOutput(config)
	config.MaxLines = itemDialogTitleMaxLines
	_, d.itemOutput = GetScaledFontWithOutput(config)
	config.MaxLines = itemDialogDetailMaxLines
	_, d.detailOutput = GetScaledFontWithOutput(config)
	config.MaxLines = 1
	_, d.pageOutput = GetScaledFontWithOutput(config)
	config.MaxLines = itemDialogFooterLines
	_, d.footerOutput = GetScaledFontWithOutput(config)
}

func (d *ItemSelectionDialog) calculateLayout() {
	interior := d.outerBorder.GetInteriorBounds()
	padding := max(itemDialogMinimumPadding, min(interior.Dx(), interior.Dy())/itemDialogPaddingDivisor)
	content := insetRectangle(interior, padding)
	lineHeight := max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))
	titleHeight := max(1, int(math.Ceil(d.titleOutput.GetLineSpacing())))
	tabRows := d.tabRowCount(content.Dx(), padding)
	headerLines := max(itemDialogHeaderMinimumLines, tabRows+1)
	headerHeight := min(
		max(1, titleHeight+lineHeight*(headerLines-1)+padding*3),
		max(1, content.Dy()/itemDialogVerticalRegionDivisor),
	)
	footerHeight := min(
		max(1, lineHeight*itemDialogFooterLines+padding*2),
		max(1, content.Dy()/itemDialogVerticalRegionDivisor),
	)
	overlap := max(1, d.pixelUnit())

	d.layout = itemSelectionLayout{ //nolint:exhaustruct_v5 // Remaining rectangles are calculated below.
		panelRect:  interior,
		headerRect: image.Rect(content.Min.X, content.Min.Y, content.Max.X, content.Min.Y+headerHeight),
		footerRect: image.Rect(content.Min.X, content.Max.Y-footerHeight, content.Max.X, content.Max.Y),
		padding:    padding,
	}
	d.layout.bodyRect = image.Rect(
		content.Min.X,
		d.layout.headerRect.Max.Y-overlap,
		content.Max.X,
		d.layout.footerRect.Min.Y+overlap,
	)
	d.layout.titleRect = image.Rect(
		d.layout.headerRect.Min.X+padding,
		d.layout.headerRect.Min.Y+padding,
		d.layout.headerRect.Max.X-padding,
		min(d.layout.headerRect.Max.Y, d.layout.headerRect.Min.Y+padding+titleHeight),
	)
	d.layout.tabsRect = image.Rect(
		d.layout.headerRect.Min.X+padding,
		d.layout.titleRect.Max.Y,
		d.layout.headerRect.Max.X-padding,
		d.layout.headerRect.Max.Y-padding,
	)

	d.calculateBodyPanes(padding)
	d.calculateTabRects(tabRows)
	d.calculateDetailRects(lineHeight)

	d.titleOutput.SetMaxCharsPerLine(max(1, d.titleOutput.Font.CharsForWidth(d.layout.titleRect.Dx())))
	if len(d.layout.tabRects) > 0 {
		d.tabOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.layout.tabRects[0].Dx())))
	}
	d.footerOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.footerContentRect().Dx())))
}

func (d *ItemSelectionDialog) calculateBodyPanes(_ int) {
	overlap := max(1, d.pixelUnit())
	usableWidth := max(1, d.layout.bodyRect.Dx())
	listWidth := max(1, int(math.Round(float64(usableWidth)*itemDialogListRatio)))
	d.layout.listRect = image.Rect(
		d.layout.bodyRect.Min.X,
		d.layout.bodyRect.Min.Y,
		d.layout.bodyRect.Min.X+listWidth,
		d.layout.bodyRect.Max.Y,
	)
	d.layout.detailRect = image.Rect(
		d.layout.listRect.Max.X-overlap,
		d.layout.bodyRect.Min.Y,
		d.layout.bodyRect.Max.X,
		d.layout.bodyRect.Max.Y,
	)
}

func (d *ItemSelectionDialog) tabRowCount(width int, padding int) int {
	if len(d.categories) == 0 {
		return 1
	}
	maximumLabelWidth := 1
	for _, category := range d.categories {
		labelWidth, _ := d.font.MeasureString(category.String())
		maximumLabelWidth = max(maximumLabelWidth, int(math.Ceil(labelWidth))+padding*2)
	}
	tabsPerRow := max(
		itemDialogMinimumTabsPerRow,
		min(len(d.categories), max(1, width/maximumLabelWidth)),
	)

	return (len(d.categories) + tabsPerRow - 1) / tabsPerRow
}

func (d *ItemSelectionDialog) calculateTabRects(rows int) {
	d.layout.tabRects = make([]image.Rectangle, len(d.categories))
	if len(d.categories) == 0 {
		return
	}
	rows = max(1, rows)
	perRow := (len(d.categories) + rows - 1) / rows
	for row := range rows {
		start := row * perRow
		end := min(len(d.categories), start+perRow)
		if start >= end {
			break
		}
		rowTop := d.layout.tabsRect.Min.Y + row*d.layout.tabsRect.Dy()/rows
		rowBottom := d.layout.tabsRect.Min.Y + (row+1)*d.layout.tabsRect.Dy()/rows
		for index := start; index < end; index++ {
			column := index - start
			count := end - start
			left := d.layout.tabsRect.Min.X + column*d.layout.tabsRect.Dx()/count
			right := d.layout.tabsRect.Min.X + (column+1)*d.layout.tabsRect.Dx()/count
			d.layout.tabRects[index] = image.Rect(left, rowTop, right, rowBottom)
		}
	}
}

func (d *ItemSelectionDialog) calculateDetailRects(lineHeight int) {
	content := insetRectangle(d.layout.detailRect, d.layout.padding*itemDialogPaneInsetMultiplier)
	spriteScale := max(1, min(itemDialogSpriteMaximumScale, d.pixelUnit()))
	headerHeight := max(
		lineHeight*itemDialogDetailHeaderLines,
		sprites.OriginalTileSize*spriteScale+d.layout.padding,
	)
	headerHeight = min(headerHeight, max(1, content.Dy()/itemDialogVerticalRegionDivisor))
	pageHeight := min(lineHeight, max(1, content.Dy()/itemDialogVerticalRegionDivisor))
	d.layout.detailHeaderRect = image.Rect(
		content.Min.X,
		content.Min.Y,
		content.Max.X,
		content.Min.Y+headerHeight,
	)
	d.layout.detailPageRect = image.Rect(
		content.Min.X,
		content.Max.Y-pageHeight,
		content.Max.X,
		content.Max.Y,
	)
	d.layout.detailTextRect = image.Rect(
		content.Min.X,
		min(content.Max.Y, d.layout.detailHeaderRect.Max.Y+d.layout.padding),
		content.Max.X,
		max(content.Min.Y, d.layout.detailPageRect.Min.Y-d.layout.padding),
	)
}

func (d *ItemSelectionDialog) drawHeader(screen *ebiten.Image) {
	title := "Use Item"
	for _, item := range d.allItems {
		if item.UnavailableReason != "" {
			title = "Use Item  [No] Unavailable"

			break
		}
	}
	d.titleOutput.SetMaxCharsPerLine(max(1, d.titleOutput.Font.CharsForWidth(d.layout.titleRect.Dx())))
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().
		WithAlignment(etext.AlignCenter).
		Text(title, DialogHeaderColor).
		Done()
	d.titleOutput.DrawPanelContent(screen, d.layout.titleRect.Min, d.layout.titleRect.Dx(), etext.AlignCenter)

	for index, rect := range d.layout.tabRects {
		textColor := DialogItemNameColor
		if index == d.activeCategory {
			textColor = pkgcolor.EGAYellow
		}
		d.tabOutput.Reset()
		d.tabOutput.AppendLine().
			WithAlignment(etext.AlignCenter).
			Text(d.categories[index].String(), textColor).
			Done()
		top := rect.Min.Y + max(0, (rect.Dy()-int(math.Ceil(d.tabOutput.GetFontHeight())))/itemDialogCenterDivisor)
		d.tabOutput.DrawPanelContent(screen, image.Pt(rect.Min.X, top), rect.Dx(), etext.AlignCenter)
	}
}

func (d *ItemSelectionDialog) drawList(screen *ebiten.Image) {
	content := d.listContentRect()
	items := d.currentItems()
	if len(items) == 0 {
		drawMasterDetailCenteredLine(
			screen, content, "No usable items.", DialogEmptyColor, d.font, d.rowOutput,
		)

		return
	}

	rowHeight := max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))
	category := d.currentCategory()
	start := d.scrollStarts[category]
	end := min(len(items), start+d.visibleRowCount())
	selection := d.selections[category]
	for index := start; index < end; index++ {
		rowTop := content.Min.Y + (index-start)*rowHeight
		rowRect := image.Rect(content.Min.X, rowTop, content.Max.X, min(content.Max.Y, rowTop+rowHeight))
		d.drawListRow(screen, rowRect, items[index], index == selection)
	}
	d.drawScrollIndicators(screen, content, start, end, len(items), rowHeight)
}

func (d *ItemSelectionDialog) drawListRow(
	screen *ebiten.Image,
	rect image.Rectangle,
	item game_state.UseableItemInfo,
	selected bool,
) {
	textColor := DialogItemNameColor
	quantityColor := DialogItemQuantityColor
	if item.UnavailableReason != "" {
		textColor = DialogUnavailableItemColor
		quantityColor = DialogUnavailableItemColor
	}
	if selected {
		fillSelectedRow(screen, rect)
		textColor = DialogSelectedText
		quantityColor = DialogSelectedText
	}

	padding := d.layout.padding
	contentWidth := max(1, rect.Dx()-padding*2-d.listIndicatorWidth())
	quantity := strconv.Itoa(item.Quantity)
	quantityWidth, _ := d.font.MeasureString(quantity)
	nameWidth := max(1, contentWidth-max(itemDialogQuantityMinimumColumns, int(math.Ceil(quantityWidth))+padding))

	d.rowOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(nameWidth)))
	d.rowOutput.Reset()
	d.rowOutput.AppendLine().Text(unavailableRowLabel(item.Name, item.UnavailableReason == ""), textColor).Done()
	d.rowOutput.DrawPanelContent(
		screen,
		image.Pt(rect.Min.X+padding, rect.Min.Y),
		nameWidth,
		etext.AlignStart,
	)
	d.rowOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(contentWidth)))
	d.rowOutput.Reset()
	d.rowOutput.AppendLine().WithAlignment(etext.AlignEnd).Text(quantity, quantityColor).Done()
	d.rowOutput.DrawPanelContent(
		screen,
		image.Pt(rect.Min.X+padding, rect.Min.Y),
		contentWidth,
		etext.AlignEnd,
	)
}

func (d *ItemSelectionDialog) drawScrollIndicators(
	screen *ebiten.Image,
	content image.Rectangle,
	start int,
	end int,
	count int,
	rowHeight int,
) {
	drawMasterDetailScrollIndicators(
		screen, content, start, end, count, rowHeight, d.listIndicatorWidth(), d.font, d.rowOutput,
	)
}

func (d *ItemSelectionDialog) listIndicatorWidth() int {
	return max(d.layout.padding*itemDialogPaneInsetMultiplier, int(math.Ceil(d.font.TextFace.Size)))
}

func (d *ItemSelectionDialog) drawDetail(screen *ebiten.Image) {
	item := d.selectedItem()
	if item == nil {
		drawMasterDetailCenteredLine(
			screen, d.detailTextRect(), "No item selected.", DialogEmptyColor, d.font, d.rowOutput,
		)

		return
	}
	d.drawDetailHeader(screen, *item)

	pages := d.detailPages()
	if d.detailPage >= len(pages) {
		d.detailPage = max(0, len(pages)-1)
	}
	content := d.detailTextRect()
	d.detailOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(content.Dx())))
	d.detailOutput.Reset()
	for _, line := range pages[d.detailPage] {
		d.appendDetailLine(line)
	}
	d.detailOutput.DrawPanelContent(screen, content.Min, content.Dx(), etext.AlignStart)

	d.pageOutput.Reset()
	if len(pages) <= 1 {
		return
	}
	d.pageOutput.AppendLine().
		WithAlignment(etext.AlignCenter).
		Text(fmt.Sprintf("Page %d/%d", d.detailPage+1, len(pages)), DialogNavHintColor).
		Done()
	d.pageOutput.DrawPanelContent(
		screen,
		d.layout.detailPageRect.Min,
		d.layout.detailPageRect.Dx(),
		etext.AlignCenter,
	)
}

func (d *ItemSelectionDialog) drawDetailHeader(screen *ebiten.Image, item game_state.UseableItemInfo) {
	header := d.layout.detailHeaderRect
	textStart := header.Min.X
	if d.spriteSheet != nil && item.SpriteIndex > 0 {
		spriteImage := d.spriteSheet.GetSprite(item.SpriteIndex)
		if spriteImage != nil {
			scale := max(1, min(itemDialogSpriteMaximumScale, d.pixelUnit()))
			options := new(ebiten.DrawImageOptions)
			options.Filter = ebiten.FilterNearest
			options.GeoM.Scale(float64(scale), float64(scale))
			options.GeoM.Translate(float64(header.Min.X), float64(header.Min.Y))
			screen.DrawImage(spriteImage, options)
			textStart += spriteImage.Bounds().Dx()*scale + d.layout.padding
		}
	}

	textRect := image.Rect(textStart, header.Min.Y, header.Max.X, header.Max.Y)
	d.itemOutput.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(max(1, textRect.Dx()))))
	d.itemOutput.Reset()
	d.itemOutput.AppendLine().Text(item.FullName, DialogHeaderColor).WordWrap().Done()
	d.itemOutput.AppendLine().
		Text("Qty: ", DialogLabelColor).
		Textf("%d", DialogValueColor, item.Quantity).
		Done()
	d.itemOutput.DrawPanelContent(screen, textRect.Min, max(1, textRect.Dx()), etext.AlignStart)
}

func (d *ItemSelectionDialog) drawFooter(screen *ebiten.Image) {
	content := d.footerContentRect()
	enter := "Use"
	if item := d.selectedItem(); item != nil && item.UnavailableReason != "" {
		enter = "Unavailable"
	}
	details := ""
	if len(d.detailPages()) > 1 {
		details = "  [Space] Details"
	}
	drawModifierFooter(
		screen,
		content,
		"[Tab/Shift+Tab] Category"+details+"\n"+
			"[Up/Down] Item   [Shift+Up/Down] Page   [Enter] "+enter+"   [Esc] Cancel",
		d.footerOutput,
	)
}

func (d *ItemSelectionDialog) detailPages() [][]detailLine {
	item := d.selectedItem()
	if item == nil {
		return [][]detailLine{{{
			label: "", value: "No item selected.", color: DialogEmptyColor, header: false,
		}}}
	}

	lines := append(d.actionDetailLines(*item), d.loreDetailLines(*item)...)
	pages := d.paginateDetailLines(lines)
	if len(pages) == 0 {
		return [][]detailLine{{}}
	}

	return pages
}

func (d *ItemSelectionDialog) actionDetailLines(item game_state.UseableItemInfo) []detailLine {
	var lines []detailLine
	if item.UnavailableReason != "" {
		lines = append(lines, detailLine{
			label: "", value: item.UnavailableReason, color: DialogStatusWarningColor, header: false,
		})
	} else {
		for _, hint := range []string{item.EffectSummary, item.Requirement, item.Consumption} {
			if hint != "" {
				lines = append(lines, detailLine{label: "", value: hint, color: DialogControlHintColor, header: false})
			}
		}
		if warning := strings.TrimSpace(item.Warning); warning != "" {
			lines = append(lines, detailLine{label: "", value: warning, color: DialogWarningColor, header: false})
		}
	}

	return lines
}

// unavailableRowMarker marks a row that cannot be chosen in text as well as colour.
const unavailableRowMarker = "[No]"

func unavailableRowLabel(name string, available bool) string {
	if !available {
		return unavailableRowMarker + " " + name
	}

	return name
}

func (d *ItemSelectionDialog) loreDetailLines(item game_state.UseableItemInfo) []detailLine {
	description := strings.TrimSpace(item.Description)
	if description == "" {
		return nil
	}
	lines := []detailLine{
		{label: "", value: description, color: DialogValueColor, header: false},
	}
	if attribution := strings.TrimSpace(item.ItemRef.ItemDescriptionAttribution); attribution != "" {
		lines = append(lines, detailLine{
			label: "Source", value: attribution, color: DialogNavHintColor, header: false,
		})
	}

	return lines
}

func (d *ItemSelectionDialog) paginateDetailLines(lines []detailLine) [][]detailLine {
	content := d.detailTextRect()
	maxChars := max(1, d.font.CharsForWidth(max(1, content.Dx())))
	lineCapacity := max(1, d.detailOutput.GetLineCapacity(max(1, content.Dy())))
	d.detailOutput.SetMaxCharsPerLine(maxChars)
	if len(lines) == 0 {
		return nil
	}

	remaining := append([]detailLine(nil), lines...)
	pages := make([][]detailLine, 0, 1)
	page := make([]detailLine, 0, lineCapacity)
	for len(remaining) > 0 {
		candidate := append(append([]detailLine(nil), page...), remaining[0])
		if d.detailLineCount(candidate) <= lineCapacity {
			page = candidate
			remaining = remaining[1:]

			continue
		}
		if len(page) > 0 {
			if remainingRows := lineCapacity - d.detailLineCount(page); remainingRows > 0 {
				first, remainder := splitDetailLine(remaining[0], remainingRows, d.detailLineCount)
				page = append(page, first)
				if remainder == nil {
					remaining = remaining[1:]
				} else {
					remaining[0] = *remainder
				}
			}
			pages = append(pages, page)
			page = nil

			continue
		}

		first, remainder := splitDetailLine(remaining[0], lineCapacity, d.detailLineCount)
		pages = append(pages, []detailLine{first})
		if remainder == nil {
			remaining = remaining[1:]
		} else {
			remaining[0] = *remainder
		}
	}
	if len(page) > 0 {
		pages = append(pages, page)
	}

	return pages
}

func (d *ItemSelectionDialog) detailLineCount(lines []detailLine) int {
	d.detailOutput.Reset()
	for _, line := range lines {
		d.appendDetailLine(line)
	}

	return d.detailOutput.GetPanelContentLineCount()
}

func (d *ItemSelectionDialog) appendDetailLine(line detailLine) {
	builder := d.detailOutput.AppendLine()
	if line.header {
		builder.Text(line.value, DialogHeaderColor).WordWrap().Done()

		return
	}
	if line.label != "" {
		builder.Text(strings.TrimSpace(line.label)+": ", DialogLabelColor)
	}
	builder.Text(line.value, line.color).WordWrap().Done()
}

func (d *ItemSelectionDialog) listContentRect() image.Rectangle {
	return insetRectangle(d.layout.listRect, d.layout.padding*itemDialogPaneInsetMultiplier)
}

func (d *ItemSelectionDialog) detailTextRect() image.Rectangle {
	return d.layout.detailTextRect
}

func (d *ItemSelectionDialog) footerContentRect() image.Rectangle {
	return insetRectangle(d.layout.footerRect, d.layout.padding)
}

func (d *ItemSelectionDialog) visibleRowCount() int {
	if d.rowOutput == nil {
		return 1
	}
	rowHeight := max(1, int(math.Ceil(d.rowOutput.GetLineSpacing())))

	return max(1, d.listContentRect().Dy()/rowHeight)
}

func (d *ItemSelectionDialog) pixelUnit() int {
	return getClassicUIPixelUnit(d.lastScreenWidth, d.lastScreenHeight)
}

func itemDialogPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: itemDialogMarginX,
		EndPercentX:   1 - itemDialogMarginX,
		StartPercentY: itemDialogMarginY,
		EndPercentY:   1 - itemDialogMarginY,
	}
}

// IsDismissed reports whether the picker has emitted a result or been cancelled.
func (d *ItemSelectionDialog) IsDismissed() bool { return d.dismissed }

// Dismiss closes the picker without invoking either result callback.
func (d *ItemSelectionDialog) Dismiss() { d.dismissed = true }

// GetDetailPageCount reports measured action and lore pages for the selected item.
func (d *ItemSelectionDialog) GetDetailPageCount() int {
	d.ensureLayout()

	return len(d.detailPages())
}
