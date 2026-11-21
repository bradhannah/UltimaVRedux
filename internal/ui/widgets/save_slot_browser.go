package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	saveSlotBrowserVisibleRows           = 8
	saveSlotBrowserDefaultStartPercent   = 0.10
	saveSlotBrowserDefaultEndPercent     = 0.90
	saveSlotBrowserBodyLineSpacing       = 1.2
	saveSlotBrowserMinimumTextChars      = 10
	saveSlotBrowserMinimumPaddingPixels  = 4
	saveSlotBrowserPaddingDivisor        = 50
	saveSlotBrowserListWidthRatio        = 0.43
	saveSlotBrowserFooterLineCount       = 2
	saveSlotBrowserCenterDivisor         = 2
	saveSlotBrowserRowHorizontalPadding  = 4
	saveSlotBrowserMinimumScrollCueInset = 3
	saveSlotBrowserMinimumScrollCueSize  = 2
	saveSlotBrowserScrollCueXDivisor     = 30
	saveSlotBrowserScrollCueSizeDivisor  = 80
	saveSlotBrowserTruncationSuffix      = "..."
	saveSlotBrowserCornerRadiusUnits     = 2
	saveSlotBrowserReviewLineCount       = 3
	saveSlotBrowserRowLines              = 2
	defaultCancelActionLabel             = "Cancel"

	// SaveSlotRowDescriptionMaxRunes is the longest description that fits whole
	// on its row in the narrowest (in-game) Save/Load layout at every supported size.
	SaveSlotRowDescriptionMaxRunes = 22
)

var _ Widget = (*SaveSlotBrowser)(nil)

// SaveSlotDetail is one label/value row in a save slot's detail pane.
type SaveSlotDetail struct {
	Label string
	Value string
}

// SaveSlotView is a detached, presentation-only description of one player save slot.
type SaveSlotView struct {
	ID                 string
	Label              string
	Status             string
	RowSummary         string
	ReviewValue        string
	Description        string
	Newest             bool
	HasSave            bool
	NavigationDisabled bool
	Details            []SaveSlotDetail
	Problem            string
	PrimaryEnabled     bool
}

// SaveSlotBrowserConfig configures a SaveSlotBrowser.
type SaveSlotBrowserConfig struct {
	Title                    string
	PrimaryActionLabel       string
	CancelActionLabel        string
	InitialSelectionID       string
	NewestLegend             string
	ReviewLabel              string
	RequireExplicitSelection bool
	Notice                   string
	Keyboard                 *input.Keyboard
	Placement                sprites.PercentBasedPlacement
	OnPrimary                func(SaveSlotView)
	OnCancel                 func()
	IsKeyPressed             func(ebiten.Key) bool
}

// SaveSlotBrowser presents save-slot view models without owning save persistence.
type SaveSlotBrowser struct {
	navigation
	slots              []SaveSlotView
	title              string
	primaryActionLabel string
	cancelActionLabel  string
	keyboard           *input.Keyboard
	placement          sprites.PercentBasedPlacement
	onPrimary          func(SaveSlotView)
	onCancel           func()
	isKeyPressed       func(ebiten.Key) bool
	notice             string
	newestLegend       string
	reviewLabel        string

	listState    *ScrollableListState
	hasSelection bool

	outerBorder         *Border
	titleOutput         *text.Output
	rowOutput           *text.Output
	detailOutput        *text.Output
	footerOutput        *text.Output
	bodyFont            *text.UltimaFont
	detailHeadingOutput *text.Output
	reviewHeadingOutput *text.Output
	reviewOutput        *text.Output
	titleY              int
	detailHeadingRect   image.Rectangle
	detailHeadingY      int
	reviewHeadingRect   image.Rectangle
	reviewTextRect      image.Rectangle
	reviewHeadingY      int
	reviewTextY         int

	titleRect       image.Rectangle
	listRect        image.Rectangle
	detailRect      image.Rectangle
	detailTextRect  image.Rectangle
	footerRect      image.Rectangle
	footerTextRect  image.Rectangle
	reviewFrameRect image.Rectangle
	rowRects        [saveSlotBrowserVisibleRows]image.Rectangle
	frameRects      []image.Rectangle

	showDescriptions      bool
	detailLineCapacity    int
	detailContentCapacity int
	detailPage            int
	lastScreenWidth       int
	lastScreenHeight      int
}

// NewSaveSlotBrowser creates a modal browser for detached player save-slot views.
func NewSaveSlotBrowser(views []SaveSlotView, config SaveSlotBrowserConfig) *SaveSlotBrowser {
	placement := config.Placement
	if saveSlotBrowserPlacementIsZero(placement) {
		placement = saveSlotBrowserDefaultPlacement()
	}
	if placement.EndPercentX <= placement.StartPercentX || placement.EndPercentY <= placement.StartPercentY {
		panic("SaveSlotBrowser: placement must have positive width and height")
	}

	browser := new(SaveSlotBrowser)
	browser.slots = cloneSaveSlotViews(views)
	browser.title = saveSlotBrowserLabel(config.Title, "Save Slots")
	browser.primaryActionLabel = saveSlotBrowserLabel(config.PrimaryActionLabel, "Select")
	browser.cancelActionLabel = saveSlotBrowserLabel(strings.TrimSpace(config.CancelActionLabel), defaultCancelActionLabel)
	browser.keyboard = config.Keyboard
	browser.placement = placement
	browser.onPrimary = config.OnPrimary
	browser.onCancel = config.OnCancel
	browser.isKeyPressed = config.IsKeyPressed
	if browser.isKeyPressed == nil {
		browser.isKeyPressed = browser.keyboard.IsKeyHeld
	}
	browser.navigation = newNavigation(browser.isKeyPressed)
	browser.notice = strings.TrimSpace(config.Notice)
	browser.newestLegend = strings.TrimSpace(config.NewestLegend)
	browser.reviewLabel = strings.TrimSpace(config.ReviewLabel)
	browser.listState = NewScrollableListState(len(views), saveSlotBrowserVisibleRows)
	initialIndex := initialNavigableSaveSlotIndex(views, config.InitialSelectionID)
	if config.RequireExplicitSelection && strings.TrimSpace(config.InitialSelectionID) == "" {
		initialIndex = -1
	}
	if initialIndex >= 0 {
		browser.listState.SetIndex(initialIndex)
		browser.hasSelection = true
	}
	browser.ensureLayout()

	return browser
}

// saveSlotBrowserLabel returns label, or fallback when label is empty.
func saveSlotBrowserLabel(label, fallback string) string {
	if label == "" {
		return fallback
	}

	return label
}

// Draw renders the opaque modal panel, slot rows, selected details, and keyboard guidance.
func (b *SaveSlotBrowser) Draw(screen *ebiten.Image) {
	if screen == nil {
		return
	}
	b.ensureLayout()
	fillClassicPaneBackgrounds(screen, b.frameRects, b.pixelUnit(), pkgcolor.DialogBackground.RGBA())
	b.drawTitle(screen)
	b.drawSlotRows(screen)
	b.drawDetails(screen)
	b.drawReview(screen)
	b.drawFooter(screen)
	drawClassicPaneFrames(screen, b.frameRects, b.pixelUnit(), DialogRuleColor)
}

// Update processes at most one registered keyboard action.
func (b *SaveSlotBrowser) Update() {
	b.ensureLayout()
	if b.keyboard == nil {
		return
	}

	if b.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		b.cancel()

		return
	}
	if b.handleDirectionalNavigation() {
		return
	}

	switch {
	case b.keyboard.IsKeyPressedAndRegister(ebiten.KeyLeft):
		b.moveDetailPage(-1)
	case b.keyboard.IsKeyPressedAndRegister(ebiten.KeyRight):
		b.moveDetailPage(1)
	case b.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace):
		b.detailPage = (b.detailPage + 1) % b.detailPageCount()
		b.populateFooterOutput()
	case b.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter),
		b.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter):
		b.activatePrimary()
	}
}

func (b *SaveSlotBrowser) moveDetailPage(direction int) {
	b.detailPage = max(0, min(b.detailPage+direction, b.detailPageCount()-1))
	b.populateFooterOutput()
}

func (b *SaveSlotBrowser) detailPageCount() int {
	return max(1, (b.detailOutput.GetPanelContentLineCount()+b.detailLineCapacity-1)/b.detailLineCapacity)
}

func (b *SaveSlotBrowser) handleDirectionalNavigation() bool {
	key, owned := b.PollWithKeyState(b.keyboard, b.isKeyPressed)
	if key != nil {
		if delta := listNavigationDelta(*key, 0); delta != 0 {
			b.moveSelection(delta)
		}
	}

	return owned
}

// GetSelectedIndex returns the selected model index, or -1 when there are no models.
func (b *SaveSlotBrowser) GetSelectedIndex() int {
	if len(b.slots) == 0 || !b.hasSelection {
		return -1
	}

	return b.listState.GetCurrentIndex()
}

func (b *SaveSlotBrowser) ensureLayout() {
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	if screenWidth <= 0 || screenHeight <= 0 {
		return
	}
	if b.outerBorder != nil && screenWidth == b.lastScreenWidth && screenHeight == b.lastScreenHeight {
		return
	}

	b.lastScreenWidth = screenWidth
	b.lastScreenHeight = screenHeight
	b.outerBorder = NewOpaqueBorder(b.placement, BorderWidthStandard, pkgcolor.DialogBackground.RGBA())

	_, b.titleOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeTitle,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              modalDialogTitleMaxChars,
	})
	b.bodyFont, b.rowOutput = GetScaledFontWithOutput(FontConfig{
		Size:                  FontSizeBody,
		LineSpacingMultiplier: saveSlotBrowserBodyLineSpacing,
		MaxLines:              saveSlotBrowserRowLines,
		MaxChars:              saveSlotBrowserMinimumTextChars,
	})
	b.detailHeadingOutput = text.NewOutput(b.bodyFont, b.rowOutput.GetLineSpacing(), 1, modalDialogTitleMaxChars)
	b.detailHeadingOutput.AppendLine().Text("DETAILS", DialogHeaderColor).Done()
	if b.reviewLabel != "" {
		b.reviewHeadingOutput = text.NewOutput(b.bodyFont, b.rowOutput.GetLineSpacing(), 1, modalDialogTitleMaxChars)
		b.reviewHeadingOutput.AppendLine().Text(b.reviewLabel, DialogHeaderColor).Done()
	}

	b.calculateLayout()
	b.populateDetailOutput()
	b.populateFooterOutput()
}

func (b *SaveSlotBrowser) calculateLayout() {
	bounds := b.outerBorder.GetInteriorBounds()
	padding := max(
		saveSlotBrowserMinimumPaddingPixels,
		min(bounds.Dx(), bounds.Dy())/saveSlotBrowserPaddingDivisor,
	)
	footerLines := saveSlotBrowserFooterLineCount
	if b.newestLegend != "" {
		footerLines++
	}
	b.layoutTitleAndFooter(bounds, padding, footerLines)
	contentTop := min(b.titleRect.Max.Y+padding, b.footerRect.Min.Y)
	contentBottom := max(contentTop, b.footerRect.Min.Y-padding)
	contentRect := image.Rect(bounds.Min.X+padding, contentTop, bounds.Max.X-padding, contentBottom)
	listWidth := int(float64(contentRect.Dx()) * saveSlotBrowserListWidthRatio)
	listWidth = max(1, min(listWidth, contentRect.Dx()))
	b.listRect = image.Rect(contentRect.Min.X, contentRect.Min.Y, contentRect.Min.X+listWidth, contentRect.Max.Y)
	detailStartX := min(contentRect.Max.X, b.listRect.Max.X+padding)
	b.detailRect = image.Rect(detailStartX, contentRect.Min.Y, contentRect.Max.X, contentRect.Max.Y)
	b.frameRects = connectClassicPaneGrid([][]image.Rectangle{
		{b.titleRect},
		{b.listRect, b.detailRect},
		{b.footerRect},
	}, b.pixelUnit())
	// Connected frames, not the pre-connection grid, own every content inset.
	inset := max(padding, b.pixelUnit()*saveSlotBrowserCornerRadiusUnits)
	b.titleRect = b.frameRects[0].Inset(inset)
	b.listRect = b.frameRects[1].Inset(inset)
	b.detailRect = b.frameRects[2].Inset(inset)
	b.footerRect = b.frameRects[3]
	b.footerTextRect = b.footerRect.Inset(inset)
	b.detailHeadingRect = b.detailRect
	if b.reviewLabel != "" {
		b.layoutReviewFrame(inset, padding)
	}
	b.detailHeadingRect.Max.Y = b.detailHeadingRect.Min.Y + int(math.Ceil(b.titleOutput.GetFontHeight()))
	b.detailTextRect = b.detailRect
	b.detailTextRect.Min.Y = min(b.detailRect.Max.Y, b.detailHeadingRect.Max.Y+padding)
	b.titleY = centeredGlyphOutputY(b.titleRect, b.titleOutput, b.title)
	b.detailHeadingY = centeredGlyphOutputY(b.detailHeadingRect, b.detailHeadingOutput, "DETAILS")
	if b.reviewLabel != "" {
		b.reviewHeadingY = centeredGlyphOutputY(b.reviewHeadingRect, b.reviewHeadingOutput, b.reviewLabel)
		b.reviewOutput = text.NewOutput(
			b.bodyFont,
			b.rowOutput.GetLineSpacing(),
			saveSlotBrowserReviewLineCount,
			max(1, b.bodyFont.CharsForWidth(max(1, b.reviewTextRect.Dx()))),
		)
	}

	b.calculateRowRectangles()
	b.showDescriptions = float64(b.rowRects[0].Dy()) >= b.rowOutput.GetTotalHeightForLines(saveSlotBrowserRowLines)+
		float64(b.pixelUnit()*saveSlotBrowserCenterDivisor)

	b.sizeOutputsToLayout(footerLines)
}

// layoutTitleAndFooter places the title pane at the top of bounds and a footer
// pane tall enough for footerLines of guidance at the bottom.
func (b *SaveSlotBrowser) layoutTitleAndFooter(bounds image.Rectangle, padding, footerLines int) {
	titleHeight := int(math.Ceil(b.titleOutput.GetFontHeight())) + padding
	footerHeight := int(math.Ceil(b.rowOutput.GetTotalHeightForLines(footerLines))) +
		padding*saveSlotBrowserCenterDivisor

	b.titleRect = image.Rect(
		bounds.Min.X+padding,
		bounds.Min.Y+padding,
		bounds.Max.X-padding,
		bounds.Min.Y+padding+titleHeight,
	)
	b.footerRect = image.Rect(
		bounds.Min.X+padding,
		bounds.Max.Y-padding-footerHeight,
		bounds.Max.X-padding,
		bounds.Max.Y-padding,
	)
}

// layoutReviewFrame splits a review frame with its heading from the top of the
// details pane; the details heading moves below it.
func (b *SaveSlotBrowser) layoutReviewFrame(inset, padding int) {
	reviewHeight := int(math.Ceil(
		b.titleOutput.GetFontHeight()+b.rowOutput.GetTotalHeightForLines(saveSlotBrowserReviewLineCount),
	)) +
		inset*saveSlotBrowserCenterDivisor + b.pixelUnit()*saveSlotBrowserCenterDivisor
	reviewHeight = min(reviewHeight, b.detailRect.Dy()/saveSlotBrowserCenterDivisor)
	b.reviewFrameRect = image.Rect(
		b.detailRect.Min.X, b.detailRect.Min.Y, b.detailRect.Max.X, b.detailRect.Min.Y+reviewHeight,
	)
	b.reviewTextRect = b.reviewFrameRect.Inset(inset)
	b.reviewHeadingRect = b.reviewTextRect
	b.reviewHeadingRect.Max.Y = b.reviewHeadingRect.Min.Y + int(math.Ceil(b.titleOutput.GetFontHeight()))
	b.reviewTextY = b.reviewHeadingRect.Max.Y + b.pixelUnit()
	b.detailHeadingRect.Min.Y = min(b.detailRect.Max.Y, b.reviewFrameRect.Max.Y+padding)
	b.frameRects = append(b.frameRects, b.reviewFrameRect)
}

// sizeOutputsToLayout fits row, title, detail and footer outputs to the laid-out panes.
func (b *SaveSlotBrowser) sizeOutputsToLayout(footerLines int) {
	rowTextWidth := max(1, b.listRect.Dx()-saveSlotBrowserRowHorizontalPadding*saveSlotBrowserCenterDivisor)
	b.rowOutput.SetMaxCharsPerLine(b.bodyFont.CharsForWidth(rowTextWidth))
	b.titleOutput.SetMaxCharsPerLine(b.titleOutput.Font.CharsForWidth(max(1, b.titleRect.Dx())))

	detailTextWidth := max(1, b.detailTextRect.Dx())
	detailMaxChars := b.bodyFont.CharsForWidth(detailTextWidth)
	b.detailLineCapacity = max(1, b.rowOutput.GetLineCapacity(max(1, b.detailTextRect.Dy())))
	b.detailOutput = text.NewOutput(
		b.bodyFont,
		b.rowOutput.GetLineSpacing(),
		b.detailLineCapacity,
		detailMaxChars,
	)
	b.footerOutput = text.NewOutput(
		b.bodyFont,
		b.rowOutput.GetLineSpacing(),
		footerLines,
		max(1, b.bodyFont.CharsForWidth(max(1, b.footerTextRect.Dx()))),
	)
}

func (b *SaveSlotBrowser) calculateRowRectangles() {
	for row := range saveSlotBrowserVisibleRows {
		rowTop := b.listRect.Min.Y + row*b.listRect.Dy()/saveSlotBrowserVisibleRows
		rowBottom := b.listRect.Min.Y + (row+1)*b.listRect.Dy()/saveSlotBrowserVisibleRows
		b.rowRects[row] = image.Rect(b.listRect.Min.X, rowTop, b.listRect.Max.X, rowBottom)
	}
}

func (b *SaveSlotBrowser) drawTitle(screen *ebiten.Image) {
	b.titleOutput.Reset()
	b.titleOutput.AppendLine().Text(b.title, DialogHeaderColor).Done()
	b.titleOutput.DrawPanelContent(screen, image.Pt(b.titleRect.Min.X, b.titleY), b.titleRect.Dx(), etext.AlignCenter)
}

func (b *SaveSlotBrowser) drawReview(screen *ebiten.Image) {
	if b.reviewOutput == nil || b.reviewHeadingOutput == nil {
		return
	}
	b.reviewHeadingOutput.DrawPanelContent(
		screen,
		image.Pt(b.reviewHeadingRect.Min.X, b.reviewHeadingY),
		b.reviewHeadingRect.Dx(),
		etext.AlignStart,
	)
	b.prepareReview()
	b.reviewOutput.DrawPanelContent(
		screen,
		image.Pt(b.reviewTextRect.Min.X, b.reviewTextY),
		b.reviewTextRect.Dx(),
		etext.AlignStart,
	)
}

func (b *SaveSlotBrowser) prepareReview() {
	if b.reviewOutput == nil {
		return
	}
	b.reviewOutput.Reset()
	value := "No folder selected."
	if view := b.selectedView(); view != nil && strings.TrimSpace(view.ReviewValue) != "" {
		value = view.ReviewValue
	}
	b.reviewOutput.AppendLine().Text(value, DialogValueColor).WordWrap().Done()
}

func (b *SaveSlotBrowser) drawSlotRows(screen *ebiten.Image) {
	visibleStart, visibleEnd := b.listState.GetVisibleRange()
	selectedScreenRow := -1
	if b.hasSelection {
		selectedScreenRow = b.listState.GetCursorScreenPosition()
	}
	for row, rowRect := range b.rowRects {
		modelIndex := visibleStart + row
		if modelIndex >= visibleEnd {
			continue
		}

		b.drawSlotRow(
			screen,
			rowRect,
			b.slots[modelIndex],
			row == selectedScreenRow,
		)
	}

	b.drawScrollCues(screen)
}

func (b *SaveSlotBrowser) drawSlotRow(
	screen *ebiten.Image,
	rowRect image.Rectangle,
	view SaveSlotView,
	selected bool,
) {
	if selected {
		vector.FillRect(
			screen,
			float32(rowRect.Min.X),
			float32(rowRect.Min.Y),
			float32(rowRect.Dx()),
			float32(rowRect.Dy()),
			pkgcolor.DialogTabActive.RGBA(),
			false,
		)
	}

	textWidth := max(1, rowRect.Dx()-saveSlotBrowserRowHorizontalPadding*saveSlotBrowserCenterDivisor)
	rowText := saveSlotRowText(view, b.bodyFont.CharsForWidth(textWidth))
	rowColor := DialogItemNameColor
	if !view.HasSave {
		rowColor = DialogItemQuantityColor
	}
	if view.Problem != "" {
		rowColor = DialogStatusWarningColor
	}
	if selected {
		rowColor = DialogSelectedText
	}

	b.rowOutput.Reset()
	b.rowOutput.AppendLine().Text(rowText, rowColor).Done()
	textHeight := b.rowOutput.GetFontHeight()
	if b.showDescriptions {
		// Reserve both lines on every row so slot names align whether or not a row has a description.
		textHeight = b.rowOutput.GetTotalHeightForLines(saveSlotBrowserRowLines)
		if view.Description != "" {
			descriptionColor := DialogItemQuantityColor
			if selected {
				descriptionColor = DialogSelectedText
			}
			b.rowOutput.AppendLine().Text(truncateRunes(view.Description, b.bodyFont.CharsForWidth(textWidth)),
				descriptionColor).Done()
		}
	}
	rowY := rowRect.Min.Y + max(0, (rowRect.Dy()-int(math.Ceil(textHeight)))/saveSlotBrowserCenterDivisor)
	b.rowOutput.DrawPanelContent(
		screen,
		image.Pt(rowRect.Min.X+saveSlotBrowserRowHorizontalPadding, rowY),
		textWidth,
		etext.AlignStart,
	)
}

func (b *SaveSlotBrowser) drawScrollCues(screen *ebiten.Image) {
	centerX := float32(b.listRect.Max.X - max(
		saveSlotBrowserMinimumScrollCueInset,
		b.listRect.Dx()/saveSlotBrowserScrollCueXDivisor,
	))
	size := float32(max(
		saveSlotBrowserMinimumScrollCueSize,
		b.listRect.Dx()/saveSlotBrowserScrollCueSizeDivisor,
	))
	if b.listState.HasItemsAbove() {
		y := float32(b.listRect.Min.Y + max(
			saveSlotBrowserMinimumScrollCueInset,
			b.listRect.Dy()/saveSlotBrowserScrollCueSizeDivisor,
		))
		vector.StrokeLine(screen, centerX-size, y+size, centerX, y, float32(b.pixelUnit()), DialogScrollIndicator, false)
		vector.StrokeLine(screen, centerX, y, centerX+size, y+size, float32(b.pixelUnit()), DialogScrollIndicator, false)
	}
	if b.listState.HasItemsBelow() {
		y := float32(b.listRect.Max.Y - max(
			saveSlotBrowserMinimumScrollCueInset,
			b.listRect.Dy()/saveSlotBrowserScrollCueSizeDivisor,
		))
		vector.StrokeLine(screen, centerX-size, y-size, centerX, y, float32(b.pixelUnit()), DialogScrollIndicator, false)
		vector.StrokeLine(screen, centerX, y, centerX+size, y-size, float32(b.pixelUnit()), DialogScrollIndicator, false)
	}
}

func (b *SaveSlotBrowser) drawDetails(screen *ebiten.Image) {
	b.detailHeadingOutput.DrawPanelContent(screen, image.Pt(b.detailHeadingRect.Min.X, b.detailHeadingY),
		b.detailHeadingRect.Dx(), etext.AlignCenter)
	b.detailOutput.DrawPanelContentRange(
		screen,
		b.detailTextRect.Min,
		max(1, b.detailTextRect.Dx()),
		etext.AlignStart,
		b.detailPage*b.detailLineCapacity,
		b.detailLineCapacity,
	)
}

func (b *SaveSlotBrowser) drawFooter(screen *ebiten.Image) {
	b.footerOutput.DrawPanelContent(
		screen,
		image.Pt(b.footerTextRect.Min.X, verticallyCenteredOutputY(b.footerTextRect, b.footerOutput)),
		max(1, b.footerTextRect.Dx()),
		etext.AlignCenter,
	)
}

// DrawClassicPixelRoundedFrame draws a fixed-pixel frame with stepped rounded corners.
func DrawClassicPixelRoundedFrame(
	screen *ebiten.Image,
	rect image.Rectangle,
	pixelUnit int,
	frameColor stdcolor.Color,
) {
	pixelUnit = max(1, pixelUnit)
	segments := classicPixelRoundedFrameSegments(rect, pixelUnit)
	if len(segments) == 0 {
		vector.StrokeRect(
			screen,
			float32(rect.Min.X),
			float32(rect.Min.Y),
			float32(rect.Dx()),
			float32(rect.Dy()),
			float32(pixelUnit),
			frameColor,
			false,
		)

		return
	}
	for _, segment := range segments {
		vector.FillRect(
			screen,
			float32(segment.Min.X),
			float32(segment.Min.Y),
			float32(segment.Dx()),
			float32(segment.Dy()),
			frameColor,
			false,
		)
	}
}

// FillClassicPixelRoundedRect fills the same stepped-corner silhouette used by
// DrawClassicPixelRoundedFrame without painting outside its visible perimeter.
func FillClassicPixelRoundedRect(
	screen *ebiten.Image,
	rect image.Rectangle,
	pixelUnit int,
	fillColor stdcolor.Color,
) {
	pixelUnit = max(1, pixelUnit)
	radius := pixelUnit * saveSlotBrowserCornerRadiusUnits
	if rect.Dx() <= radius*saveSlotBrowserCenterDivisor ||
		rect.Dy() <= radius*saveSlotBrowserCenterDivisor {
		vector.FillRect(
			screen, float32(rect.Min.X), float32(rect.Min.Y), float32(rect.Dx()), float32(rect.Dy()), fillColor, false,
		)

		return
	}
	for _, band := range [...]image.Rectangle{
		image.Rect(rect.Min.X+radius, rect.Min.Y, rect.Max.X-radius, rect.Min.Y+pixelUnit),
		image.Rect(rect.Min.X+pixelUnit, rect.Min.Y+pixelUnit, rect.Max.X-pixelUnit, rect.Max.Y-pixelUnit),
		image.Rect(rect.Min.X+radius, rect.Max.Y-pixelUnit, rect.Max.X-radius, rect.Max.Y),
		image.Rect(rect.Min.X, rect.Min.Y+radius, rect.Max.X, rect.Max.Y-radius),
	} {
		vector.FillRect(
			screen,
			float32(band.Min.X),
			float32(band.Min.Y),
			float32(band.Dx()),
			float32(band.Dy()),
			fillColor,
			false,
		)
	}
}

func classicPixelRoundedFrameSegments(rect image.Rectangle, pixelUnit int) []image.Rectangle {
	pixelUnit = max(1, pixelUnit)
	radius := pixelUnit * saveSlotBrowserCornerRadiusUnits
	if rect.Dx() <= radius*saveSlotBrowserCenterDivisor ||
		rect.Dy() <= radius*saveSlotBrowserCenterDivisor {
		return nil
	}

	return []image.Rectangle{
		image.Rect(rect.Min.X+radius, rect.Min.Y, rect.Max.X-radius, rect.Min.Y+pixelUnit),
		image.Rect(rect.Min.X+radius, rect.Max.Y-pixelUnit, rect.Max.X-radius, rect.Max.Y),
		image.Rect(rect.Min.X, rect.Min.Y+radius, rect.Min.X+pixelUnit, rect.Max.Y-radius),
		image.Rect(rect.Max.X-pixelUnit, rect.Min.Y+radius, rect.Max.X, rect.Max.Y-radius),
		image.Rect(rect.Min.X+pixelUnit, rect.Min.Y+pixelUnit, rect.Min.X+radius, rect.Min.Y+radius),
		image.Rect(rect.Max.X-radius, rect.Min.Y+pixelUnit, rect.Max.X-pixelUnit, rect.Min.Y+radius),
		image.Rect(rect.Min.X+pixelUnit, rect.Max.Y-radius, rect.Min.X+radius, rect.Max.Y-pixelUnit),
		image.Rect(rect.Max.X-radius, rect.Max.Y-radius, rect.Max.X-pixelUnit, rect.Max.Y-pixelUnit),
	}
}

func (b *SaveSlotBrowser) pixelUnit() int {
	return getClassicUIPixelUnit(b.lastScreenWidth, b.lastScreenHeight)
}

func (b *SaveSlotBrowser) populateDetailOutput() {
	if b.detailOutput == nil {
		return
	}
	b.detailOutput.Reset()
	b.detailPage = 0
	view := b.selectedView()
	// Retain every wrapped row; only the rendered range is limited to the pane.
	emptyMessage := "No save slots available."
	if len(b.slots) > 0 {
		emptyMessage = "Select a destination with Up/Down."
		if firstNavigableSaveSlotIndex(b.slots) < 0 {
			emptyMessage = "No saved games to load. Create a new character or import an original save from the main menu."
		}
	}
	values := []string{b.notice, emptyMessage}
	if view != nil {
		values = append(values, saveSlotDisplayLabel(*view), "Status: "+view.Status,
			"Description: "+view.Description, "Problem: "+view.Problem)
		for _, detail := range view.Details {
			values = append(values, detail.Label+": "+detail.Value)
		}
	}
	maxChars := max(1, b.bodyFont.CharsForWidth(max(1, b.detailTextRect.Dx())))
	b.detailContentCapacity = 0
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		b.detailContentCapacity += max(1, text.MeasureText(value, b.bodyFont, b.rowOutput.GetLineSpacing(), maxChars).Lines)
	}
	b.detailOutput = text.NewOutput(b.bodyFont, b.rowOutput.GetLineSpacing(), b.detailContentCapacity, maxChars)
	if view == nil {
		remaining := b.detailContentCapacity
		remaining -= b.appendDetailText(emptyMessage, DialogEmptyColor, remaining)
		b.appendDetailText(b.notice, DialogStatusWarningColor, remaining)

		return
	}
	b.populateSelectedDetailOutput(*view)
}

func (b *SaveSlotBrowser) populateSelectedDetailOutput(view SaveSlotView) {
	remaining := b.detailContentCapacity
	remaining -= b.appendDetailText(
		saveSlotDisplayLabel(view),
		DialogHeaderColor,
		remaining,
	)
	if remaining > 0 && view.Description != "" {
		remaining -= b.appendDetailText(view.Description, DialogValueColor, remaining)
	}
	remaining = b.appendDetailStatus(view, remaining)
	if remaining > 0 && b.notice != "" {
		remaining -= b.appendDetailText(
			b.notice,
			DialogStatusWarningColor,
			remaining,
		)
	}
	remaining = b.appendDetailRows(view, remaining)
	if remaining > 0 && view.Problem != "" {
		b.appendDetailText(
			"Problem: "+view.Problem,
			DialogStatusWarningColor,
			remaining,
		)
	}
}

func (b *SaveSlotBrowser) appendDetailStatus(view SaveSlotView, remaining int) int {
	if remaining > 0 && view.Status != "" {
		statusColor := DialogValueColor
		if view.Problem != "" {
			statusColor = DialogStatusWarningColor
		}
		remaining -= b.appendDetailPair(
			"Status",
			view.Status,
			statusColor,
			remaining,
		)
	}

	return remaining
}

func (b *SaveSlotBrowser) appendDetailRows(view SaveSlotView, remaining int) int {
	for _, detail := range view.Details {
		available := remaining
		if available <= 0 {
			break
		}
		remaining -= b.appendDetailPair(detail.Label, detail.Value, DialogValueColor, available)
	}

	return remaining
}

func (b *SaveSlotBrowser) appendDetailPair(
	label string,
	value string,
	valueColor stdcolor.Color,
	maxLines int,
) int {
	label = strings.TrimSpace(label)
	value = strings.TrimSpace(value)
	if label == "" {
		textColor := valueColor
		if value == "No save data." {
			textColor = DialogEmptyColor
		}

		return b.appendDetailText(value, textColor, maxLines)
	}

	prefix := label + ": "
	fitted, lineCount := fitWrappedText(
		prefix+value,
		b.bodyFont,
		b.detailOutput.GetLineSpacing(),
		b.bodyFont.CharsForWidth(max(1, b.detailTextRect.Dx())),
		maxLines,
	)
	if fitted == "" || lineCount == 0 {
		return 0
	}
	if !strings.HasPrefix(fitted, prefix) {
		b.detailOutput.AppendLine().Text(fitted, valueColor).WordWrap().Done()

		return lineCount
	}

	b.detailOutput.AppendLine().
		Text(prefix, DialogLabelColor).
		Text(strings.TrimPrefix(fitted, prefix), valueColor).
		WordWrap().
		Done()

	return lineCount
}

func (b *SaveSlotBrowser) appendDetailText(value string, textColor stdcolor.Color, maxLines int) int {
	fitted, lineCount := fitWrappedText(
		value,
		b.bodyFont,
		b.detailOutput.GetLineSpacing(),
		b.bodyFont.CharsForWidth(max(1, b.detailTextRect.Dx())),
		maxLines,
	)
	if fitted == "" || lineCount == 0 {
		return 0
	}

	b.detailOutput.AppendLine().Text(fitted, textColor).WordWrap().Done()

	return lineCount
}

func (b *SaveSlotBrowser) moveSelection(direction int) {
	if direction != -1 && direction != 1 {
		panic("SaveSlotBrowser.moveSelection: direction must be -1 or 1")
	}
	target := b.navigationTarget(direction)
	if target < 0 || b.hasSelection && target == b.listState.GetCurrentIndex() {
		return
	}
	b.listState.SetIndex(target)
	b.hasSelection = true
	b.populateDetailOutput()
	b.populateFooterOutput()
}

func (b *SaveSlotBrowser) navigationTarget(direction int) int {
	index := b.listState.GetCurrentIndex()
	if !b.hasSelection {
		if direction > 0 {
			index = -1
		} else {
			index = len(b.slots)
		}
	}
	for index += direction; index >= 0 && index < len(b.slots); index += direction {
		if b.slots[index].NavigationDisabled {
			continue
		}

		return index
	}

	return -1
}

func (b *SaveSlotBrowser) activatePrimary() {
	view := b.selectedView()
	if view == nil || !view.PrimaryEnabled || b.onPrimary == nil {
		return
	}
	b.onPrimary(cloneSaveSlotView(*view))
}

func (b *SaveSlotBrowser) cancel() {
	if b.onCancel != nil {
		b.onCancel()
	}
}

func (b *SaveSlotBrowser) selectedView() *SaveSlotView {
	index := b.GetSelectedIndex()
	if index < 0 || index >= len(b.slots) {
		return nil
	}

	return &b.slots[index]
}

func (b *SaveSlotBrowser) populateFooterOutput() {
	if b.footerOutput == nil {
		return
	}
	b.footerOutput.Reset()
	width := b.footerTextRect.Dx()
	cancel := ControlHint{hintKeyEsc, b.cancelActionLabel}
	if firstNavigableSaveSlotIndex(b.slots) < 0 {
		PackControlHints(b.footerOutput, []ControlHint{cancel}, width, etext.AlignCenter)

		return
	}
	navigation := []ControlHint{{hintKeyUpDown, "Slot"}}
	if pages := b.detailPageCount(); pages > 1 {
		navigation = append(navigation, ControlHint{hintKeySpace, fmt.Sprintf("Details %d/%d", b.detailPage+1, pages)})
	}
	PackControlHints(b.footerOutput, navigation, width, etext.AlignCenter)
	primary := b.primaryActionLabel
	if view := b.selectedView(); view == nil || !view.PrimaryEnabled {
		primary = "Cannot " + strings.ToLower(strings.Fields(primary)[0]) + " this"
	}
	PackControlHints(b.footerOutput, []ControlHint{{hintKeyEnter, primary}, cancel}, width, etext.AlignCenter)
	if b.newestLegend != "" {
		b.footerOutput.AppendLine().WithAlignment(etext.AlignCenter).Text(b.newestLegend, DialogControlHintColor).Done()
	}
}

func saveSlotRowText(view SaveSlotView, maxChars int) string {
	marker := " "
	if view.Newest {
		marker = "*"
	}
	summary := strings.TrimSpace(view.RowSummary)
	if !view.HasSave || view.Problem != "" {
		summary = view.Status
	}
	label := saveSlotDisplayLabel(view)
	if summary == "" && !view.Newest {
		return truncateRunes(label, maxChars)
	}
	suffix := " " + marker + " " + summary
	if utf8.RuneCountInString(label)+utf8.RuneCountInString(suffix) > maxChars {
		// Reserve the marker column so a long label cannot hide the active row indicator.
		maximumSummary := max(0, maxChars-saveSlotBrowserMinimumTextChars/2)
		summary = truncateRunes(summary, maximumSummary)
		suffix = " " + marker + " " + summary
		label = truncateRunes(label, max(1, maxChars-utf8.RuneCountInString(suffix)))
	}

	return truncateRunes(label+suffix, maxChars)
}

func saveSlotDisplayLabel(view SaveSlotView) string {
	if label := strings.TrimSpace(view.Label); label != "" {
		return label
	}
	if id := strings.TrimSpace(view.ID); id != "" {
		return id
	}

	return "(unnamed slot)"
}

func fitWrappedText(value string, font *text.UltimaFont, lineSpacing float64, maxChars, maxLines int) (string, int) {
	value = strings.Join(strings.Fields(value), " ")
	if value == "" || maxLines <= 0 {
		return "", 0
	}
	maxChars = max(1, maxChars)
	measurement := text.MeasureText(value, font, lineSpacing, maxChars)
	if measurement.Lines <= maxLines {
		return value, measurement.Lines
	}

	runes := []rune(value)
	low, high := 0, len(runes)
	best := saveSlotBrowserTruncationSuffix
	for low <= high {
		middle := low + (high-low)/saveSlotBrowserCenterDivisor
		candidate := strings.TrimSpace(string(runes[:middle]))
		if candidate != "" {
			candidate += saveSlotBrowserTruncationSuffix
		} else {
			candidate = saveSlotBrowserTruncationSuffix
		}
		if text.MeasureText(candidate, font, lineSpacing, maxChars).Lines <= maxLines {
			best = candidate
			low = middle + 1
		} else {
			high = middle - 1
		}
	}

	return best, text.MeasureText(best, font, lineSpacing, maxChars).Lines
}

func truncateRunes(value string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	suffixLength := utf8.RuneCountInString(saveSlotBrowserTruncationSuffix)
	if maxRunes <= suffixLength {
		return string([]rune(value)[:maxRunes])
	}

	return strings.TrimSpace(string([]rune(value)[:maxRunes-suffixLength])) + saveSlotBrowserTruncationSuffix
}

func cloneSaveSlotViews(views []SaveSlotView) []SaveSlotView {
	cloned := make([]SaveSlotView, len(views))
	for index, view := range views {
		cloned[index] = cloneSaveSlotView(view)
	}

	return cloned
}

func cloneSaveSlotView(view SaveSlotView) SaveSlotView {
	view.Details = append([]SaveSlotDetail(nil), view.Details...)

	return view
}

func saveSlotViewIndexByID(views []SaveSlotView, slotID string) int {
	slotID = strings.TrimSpace(slotID)
	if slotID == "" {
		return -1
	}
	for index, view := range views {
		if view.ID == slotID {
			return index
		}
	}

	return -1
}

func initialNavigableSaveSlotIndex(views []SaveSlotView, slotID string) int {
	initialIndex := saveSlotViewIndexByID(views, slotID)
	if initialIndex >= 0 && !views[initialIndex].NavigationDisabled {
		return initialIndex
	}

	return firstNavigableSaveSlotIndex(views)
}

func firstNavigableSaveSlotIndex(views []SaveSlotView) int {
	for index, view := range views {
		if !view.NavigationDisabled {
			return index
		}
	}

	return -1
}

func saveSlotBrowserPlacementIsZero(placement sprites.PercentBasedPlacement) bool {
	return placement.StartPercentX == 0 &&
		placement.EndPercentX == 0 &&
		placement.StartPercentY == 0 &&
		placement.EndPercentY == 0
}

func saveSlotBrowserDefaultPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: saveSlotBrowserDefaultStartPercent,
		EndPercentX:   saveSlotBrowserDefaultEndPercent,
		StartPercentY: saveSlotBrowserDefaultStartPercent,
		EndPercentY:   saveSlotBrowserDefaultEndPercent,
	}
}
