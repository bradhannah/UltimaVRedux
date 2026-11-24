package widgets

import (
	"image"
	"math"
	"unicode/utf8"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	buttonListDefaultRowWidthPercent = 0.2
	buttonListCenterDivisor          = 2
	buttonListPaddingUnits           = 3
	buttonListSectionGapUnits        = 2
	buttonListRowGapUnits            = 1
	buttonListRowVerticalPadUnits    = 1
	buttonListDescriptionRows        = 4
	buttonListLineSpacing            = 1.2
	buttonListFooterLines            = 2
	buttonListFooterTopText          = "[Arrows] Select   [Enter] Choose"
	buttonListReadHint               = "[Shift+Up/Down] Read   "
)

var _ Widget = (*ButtonListModal)(nil)

type buttonListItem struct {
	text     string
	callback func()
}

type buttonListLayout struct {
	cardRect        image.Rectangle
	titleRect       image.Rectangle
	descriptionRect image.Rectangle
	rowRects        []image.Rectangle
	footerRect      image.Rectangle
	pixelUnit       int
}

type buttonListMetrics struct {
	pixelUnit         int
	padding           int
	sectionGap        int
	rowGap            int
	rowWidth          int
	cardWidth         int
	descriptionHeight int
	titleHeight       int
	rowHeight         int
	footerHeight      int
	cardHeight        int
}

// ButtonListModal presents a compact keyboard-driven menu in a classic dialog card.
type ButtonListModal struct {
	navigation
	items                 []buttonListItem
	keyboard              *input.Keyboard
	closeNoActionCallback func()
	gameScreenPercents    sprites.PercentBasedPlacement
	buttonWidthPercent    float64
	borderWidthPercent    float64
	currentSelectionIndex int
	titleText             string
	description           string
	descriptionRows       int
	descriptionPage       int
	escapeLabel           string
	titleOutput           *text.Output
	descriptionOutput     *text.Output
	rowOutput             *text.Output
	footerOutput          *text.Output
	layout                buttonListLayout
	lastWidth             int
	lastHeight            int
}

// NewButtonListModal creates an empty menu titled titleText; add choices with
// AddButton. Escape invokes closeNoActionCallback. All arguments are required.
func NewButtonListModal(
	titleText string,
	closeNoActionCallback func(),
	keyboard *input.Keyboard,
	gameScreenPercents *sprites.PercentBasedPlacement,
) *ButtonListModal {
	if closeNoActionCallback == nil || keyboard == nil || gameScreenPercents == nil {
		panic("NewButtonListModal: close callback, keyboard, and game area are required")
	}

	modal := new(ButtonListModal)
	modal.navigation = newNavigation(keyboard.IsKeyHeld)
	modal.keyboard = keyboard
	modal.closeNoActionCallback = closeNoActionCallback
	modal.gameScreenPercents = *gameScreenPercents
	modal.buttonWidthPercent = buttonListDefaultRowWidthPercent
	modal.currentSelectionIndex = -1
	modal.titleText = titleText
	modal.descriptionRows = buttonListDescriptionRows
	modal.escapeLabel = "Back"
	keyboard.PrepareForTransition(0)
	width, height := display.GetManager().GetCurrentSize()
	modal.ensureLayout(width, height)

	return modal
}

// Draw renders the card, title, description page, buttons and control footer.
func (b *ButtonListModal) Draw(screen *ebiten.Image) {
	if screen == nil {
		return
	}
	b.ensureLayout(screen.Bounds().Dx(), screen.Bounds().Dy())
	FillClassicPixelRoundedRect(
		screen,
		b.layout.cardRect,
		b.layout.pixelUnit,
		ucolor.DialogBackground.RGBA(),
	)

	b.titleOutput.DrawPanelContent(
		screen,
		image.Pt(b.layout.titleRect.Min.X, verticallyCenteredOutputY(b.layout.titleRect, b.titleOutput)),
		b.layout.titleRect.Dx(),
		etext.AlignCenter,
	)
	if !b.layout.descriptionRect.Empty() {
		b.descriptionOutput.DrawPanelContentRange(
			screen,
			b.layout.descriptionRect.Min,
			b.layout.descriptionRect.Dx(),
			etext.AlignCenter,
			b.descriptionPage*b.descriptionRows,
			b.descriptionRows,
		)
	}
	for index, item := range b.items {
		b.drawRow(screen, b.layout.rowRects[index], item.text, index == b.currentSelectionIndex)
	}
	b.footerOutput.DrawPanelContent(
		screen,
		image.Pt(b.layout.footerRect.Min.X, verticallyCenteredOutputY(b.layout.footerRect, b.footerOutput)),
		b.layout.footerRect.Dx(),
		etext.AlignCenter,
	)
	DrawClassicPixelRoundedFrame(screen, b.layout.cardRect, b.layout.pixelUnit, DialogRuleColor)
}

func (b *ButtonListModal) drawRow(screen *ebiten.Image, rect image.Rectangle, label string, selected bool) {
	textColor := DialogItemNameColor
	if selected {
		FillClassicPixelRoundedRect(screen, rect, b.layout.pixelUnit, DialogSelectedBackground)
		textColor = DialogSelectedText
	}
	b.rowOutput.Reset()
	b.rowOutput.AppendLine().Text(label, textColor).Done()
	b.rowOutput.DrawPanelContent(
		screen,
		image.Pt(rect.Min.X, verticallyCenteredOutputY(rect, b.rowOutput)),
		rect.Dx(),
		etext.AlignCenter,
	)
}

// SetButtonWidthPercent customizes the width of every selectable row.
func (b *ButtonListModal) SetButtonWidthPercent(width float64) {
	if width <= 0 || width >= 1 {
		panic("ButtonListModal.SetButtonWidthPercent: width must be between zero and one")
	}
	if len(b.items) != 0 {
		panic("ButtonListModal.SetButtonWidthPercent: width must be set before adding buttons")
	}
	b.buttonWidthPercent = width
	b.invalidateLayout()
}

// SetBorderWidthPercent customizes the card width without resizing its rows.
func (b *ButtonListModal) SetBorderWidthPercent(width float64) {
	if width <= b.buttonWidthPercent || width >= 1 {
		panic("ButtonListModal.SetBorderWidthPercent: width must be wider than buttons and less than one")
	}
	if len(b.items) != 0 {
		panic("ButtonListModal.SetBorderWidthPercent: width must be set before adding buttons")
	}
	b.borderWidthPercent = width
	b.invalidateLayout()
}

// SetDescription adds wrapped explanatory text above the selectable rows.
func (b *ButtonListModal) SetDescription(description string) {
	if len(b.items) != 0 {
		panic("ButtonListModal.SetDescription: description must be set before adding buttons")
	}
	b.description = description
	b.descriptionPage = 0
	b.invalidateLayout()
}

// SetDescriptionRowCapacity customizes the maximum wrapped description rows.
func (b *ButtonListModal) SetDescriptionRowCapacity(rows int) {
	if rows <= 0 {
		panic("ButtonListModal.SetDescriptionRowCapacity: rows must be positive")
	}
	if len(b.items) != 0 {
		panic("ButtonListModal.SetDescriptionRowCapacity: capacity must be set before adding buttons")
	}
	b.descriptionRows = rows
	b.invalidateLayout()
}

// SetEscapeLabel customizes the action described by the footer's Escape hint.
func (b *ButtonListModal) SetEscapeLabel(label string) {
	if label == "" {
		panic("ButtonListModal.SetEscapeLabel: label is required")
	}
	if len(b.items) != 0 {
		panic("ButtonListModal.SetEscapeLabel: label must be set before adding buttons")
	}
	b.escapeLabel = label
	b.invalidateLayout()
}

// Update moves the selection, pages the description, and runs the selected
// button's callback on Enter or the close callback on Escape.
func (b *ButtonListModal) Update() {
	key, owned := b.Poll(b.keyboard,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyLeft, ebiten.KeyRight, ebiten.KeyPageUp, ebiten.KeyPageDown)
	if owned {
		if key != nil {
			b.navigate(*key)
		}

		return
	}
	boundKey := b.keyboard.GetBoundKeyPressed(&nonAlphaNumericBoundKeys)
	if boundKey == nil {
		b.keyboard.SetAllowKeyPressImmediately()

		return
	}
	if !b.keyboard.TryToRegisterKeyPress(*boundKey) {
		return
	}

	if *boundKey == ebiten.KeyEscape {
		b.closeNoActionCallback()

		return
	}
	if *boundKey != ebiten.KeyEnter && *boundKey != ebiten.KeyNumpadEnter {
		return
	}
	if b.currentSelectionIndex >= 0 && b.currentSelectionIndex < len(b.items) {
		callback := b.items[b.currentSelectionIndex].callback
		if callback != nil {
			callback()
		}
	}
}

func (b *ButtonListModal) navigate(key ebiten.Key) {
	switch key { //nolint:exhaustive // Only navigation keys reach this switch.
	case ebiten.KeyDown, ebiten.KeyRight:
		b.GoDown()
	case ebiten.KeyUp, ebiten.KeyLeft:
		b.GoUp()
	case ebiten.KeyPageUp, ebiten.KeyPageDown:
		b.descriptionPage = max(0, min(b.descriptionPage+listNavigationDelta(key, 1), b.descriptionPageCount()-1))
		b.populateFooterOutput(b.layout.descriptionRect.Dx())
	}
}

// GoDown selects the next button, wrapping from the last to the first.
func (b *ButtonListModal) GoDown() {
	if len(b.items) == 0 {
		return
	}
	b.currentSelectionIndex = (b.currentSelectionIndex + 1) % len(b.items)
}

// GoUp selects the previous button, wrapping from the first to the last.
func (b *ButtonListModal) GoUp() {
	if len(b.items) == 0 {
		return
	}
	if b.currentSelectionIndex == 0 {
		b.currentSelectionIndex = len(b.items) - 1
	} else {
		b.currentSelectionIndex = max(b.currentSelectionIndex-1, 0)
	}
}

// AddButton appends a choice; the first button added becomes the selection.
func (b *ButtonListModal) AddButton(buttonText string, onClickCallback func()) {
	b.items = append(b.items, buttonListItem{text: buttonText, callback: onClickCallback})
	if b.currentSelectionIndex == -1 {
		b.currentSelectionIndex = 0
	}
	b.invalidateLayout()
}

func (b *ButtonListModal) invalidateLayout() {
	b.lastWidth = 0
	b.lastHeight = 0
}

func (b *ButtonListModal) ensureLayout(width, height int) {
	if b.layoutIsCurrent(width, height) {
		return
	}
	b.lastWidth = width
	b.lastHeight = height
	pixelUnit := ClassicUIPixelUnit(width, height)
	b.initializeLayoutOutputs(width)
	gameRect, metrics := b.measureLayout(width, height, pixelUnit)
	b.layout = b.buildLayout(gameRect, metrics)
}

func (b *ButtonListModal) initializeLayoutOutputs(width int) {
	_, b.titleOutput = GetScaledFontWithOutputForWidth(FontConfig{
		Size:                  FontSizeTitle,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	}, width)
	_, b.descriptionOutput = GetScaledFontWithOutputForWidth(FontConfig{
		Size:                  FontSizeBody,
		LineSpacingMultiplier: buttonListLineSpacing,
		MaxLines:              max(1, utf8.RuneCountInString(b.description)+1),
		MaxChars:              1,
	}, width)
	b.descriptionOutput.SetHyphenateLongWords(false)
	_, b.rowOutput = GetScaledFontWithOutputForWidth(FontConfig{
		Size:                  FontSizeBody,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	}, width)
	// The two hint rows get the description's leading: at one-to-one they touched.
	_, b.footerOutput = GetScaledFontWithOutputForWidth(FontConfig{
		Size:                  FontSizeBody,
		LineSpacingMultiplier: buttonListLineSpacing,
		MaxLines:              buttonListFooterLines,
		MaxChars:              1,
	}, width)
}

func (b *ButtonListModal) measureLayout(width, height, pixelUnit int) (image.Rectangle, buttonListMetrics) {
	padding := max(1, pixelUnit*buttonListPaddingUnits)
	sectionGap := max(1, pixelUnit*buttonListSectionGapUnits)
	rowGap := max(1, pixelUnit*buttonListRowGapUnits)

	gameRect := image.Rect(
		int(math.Round(b.gameScreenPercents.StartPercentX*float64(width))),
		int(math.Round(b.gameScreenPercents.StartPercentY*float64(height))),
		int(math.Round(b.gameScreenPercents.EndPercentX*float64(width))),
		int(math.Round(b.gameScreenPercents.EndPercentY*float64(height))),
	)
	rowWidth := max(1, int(math.Round(b.buttonWidthPercent*float64(width))))
	cardWidth := rowWidth + padding*buttonListCenterDivisor
	if b.borderWidthPercent > 0 {
		cardWidth = int(math.Round(b.borderWidthPercent * float64(width)))
	}
	titleWidth, _ := b.titleOutput.Font.MeasureString(b.titleText)
	footerTopWidth, _ := b.footerOutput.Font.MeasureString(buttonListFooterTopText)
	footerBottomWidth, _ := b.footerOutput.Font.MeasureString(b.footerBottomTextForMeasurement())
	footerWidth := max(footerTopWidth, footerBottomWidth)
	cardWidth = max(cardWidth, int(math.Ceil(titleWidth))+padding*buttonListCenterDivisor)
	cardWidth = max(cardWidth, int(math.Ceil(footerWidth))+padding*buttonListCenterDivisor)
	cardWidth = min(cardWidth, max(1, gameRect.Dx()-padding*buttonListCenterDivisor))
	rowWidth = min(rowWidth, max(1, cardWidth-padding*buttonListCenterDivisor))
	contentWidth := max(1, cardWidth-padding*buttonListCenterDivisor)
	descriptionHeight := b.configureLayoutOutputs(contentWidth, rowWidth)
	titleHeight := int(math.Ceil(b.titleOutput.GetFontHeight()))
	rowHeight := int(math.Ceil(b.rowOutput.GetFontHeight())) +
		pixelUnit*buttonListRowVerticalPadUnits*buttonListCenterDivisor
	footerHeight := int(math.Ceil(b.footerOutput.GetTotalHeightForLines(buttonListFooterLines)))
	cardHeight := padding*buttonListCenterDivisor + titleHeight + sectionGap + footerHeight
	if descriptionHeight > 0 {
		cardHeight += descriptionHeight + sectionGap
	}
	if len(b.items) > 0 {
		cardHeight += rowHeight*len(b.items) + rowGap*(len(b.items)-1) + sectionGap
	}

	return gameRect, buttonListMetrics{
		pixelUnit:         pixelUnit,
		padding:           padding,
		sectionGap:        sectionGap,
		rowGap:            rowGap,
		rowWidth:          rowWidth,
		cardWidth:         cardWidth,
		descriptionHeight: descriptionHeight,
		titleHeight:       titleHeight,
		rowHeight:         rowHeight,
		footerHeight:      footerHeight,
		cardHeight:        cardHeight,
	}
}

func (b *ButtonListModal) configureLayoutOutputs(contentWidth, rowWidth int) int {
	b.titleOutput.SetMaxCharsPerLine(max(1, b.titleOutput.Font.CharsForWidth(contentWidth)))
	b.titleOutput.Reset()
	b.titleOutput.AppendLine().Text(b.titleText, DialogHeaderColor).Done()
	b.rowOutput.SetMaxCharsPerLine(max(1, b.rowOutput.Font.CharsForWidth(rowWidth)))

	if b.description == "" {
		b.populateFooterOutput(contentWidth)

		return 0
	}
	b.descriptionOutput.SetMaxCharsPerLine(max(1, b.descriptionOutput.Font.CharsForWidth(contentWidth)))
	b.descriptionOutput.Reset()
	b.descriptionOutput.AddTextWithColor(b.description, DialogValueColor)
	b.descriptionPage = min(b.descriptionPage, b.descriptionPageCount()-1)
	b.populateFooterOutput(contentWidth)

	return int(math.Ceil(b.descriptionOutput.GetTotalHeightForLines(
		min(b.descriptionRows, b.descriptionOutput.GetPanelContentLineCount()),
	)))
}

func (b *ButtonListModal) escapeFooterText() string {
	if b.descriptionPageCount() > 1 {
		return buttonListReadHint + "[Esc] " + b.escapeLabel
	}

	return "[Esc] " + b.escapeLabel
}

func (b *ButtonListModal) footerBottomTextForMeasurement() string {
	if b.description != "" {
		return buttonListReadHint + "[Esc] " + b.escapeLabel
	}

	return b.escapeFooterText()
}

func (b *ButtonListModal) populateFooterOutput(contentWidth int) {
	b.footerOutput.Reset()
	for _, row := range []string{buttonListFooterTopText, b.escapeFooterText()} {
		PackControlHints(b.footerOutput, ParseControlHints(row), contentWidth, etext.AlignStart)
	}
}

func (b *ButtonListModal) descriptionPageCount() int {
	if b.descriptionOutput == nil || b.descriptionRows <= 0 {
		return 1
	}

	return max(1, (b.descriptionOutput.GetPanelContentLineCount()+b.descriptionRows-1)/b.descriptionRows)
}

func (b *ButtonListModal) buildLayout(
	gameRect image.Rectangle,
	metrics buttonListMetrics,
) buttonListLayout {
	centerX := gameRect.Min.X + gameRect.Dx()/buttonListCenterDivisor
	centerY := gameRect.Min.Y + gameRect.Dy()/buttonListCenterDivisor
	cardMinX := min(
		max(gameRect.Min.X+metrics.padding, centerX-metrics.cardWidth/buttonListCenterDivisor),
		gameRect.Max.X-metrics.padding-metrics.cardWidth,
	)
	cardMinY := min(
		max(gameRect.Min.Y+metrics.padding, centerY-metrics.cardHeight/buttonListCenterDivisor),
		gameRect.Max.Y-metrics.padding-metrics.cardHeight,
	)
	cardRect := image.Rect(
		cardMinX,
		cardMinY,
		cardMinX+metrics.cardWidth,
		cardMinY+metrics.cardHeight,
	)
	contentMinX := cardRect.Min.X + metrics.padding
	contentMaxX := cardRect.Max.X - metrics.padding
	contentY := cardRect.Min.Y + metrics.padding
	titleRect := image.Rect(contentMinX, contentY, contentMaxX, contentY+metrics.titleHeight)
	contentY = titleRect.Max.Y + metrics.sectionGap

	var descriptionRect image.Rectangle
	if metrics.descriptionHeight > 0 {
		descriptionRect = image.Rect(
			contentMinX,
			contentY,
			contentMaxX,
			contentY+metrics.descriptionHeight,
		)
		contentY = descriptionRect.Max.Y + metrics.sectionGap
	}
	rowMinX := cardRect.Min.X + (cardRect.Dx()-metrics.rowWidth)/buttonListCenterDivisor
	rowRects := make([]image.Rectangle, len(b.items))
	for index := range b.items {
		rowRects[index] = image.Rect(
			rowMinX,
			contentY,
			rowMinX+metrics.rowWidth,
			contentY+metrics.rowHeight,
		)
		contentY = rowRects[index].Max.Y + metrics.rowGap
	}
	if len(rowRects) > 0 {
		contentY = rowRects[len(rowRects)-1].Max.Y + metrics.sectionGap
	}
	footerRect := image.Rect(contentMinX, contentY, contentMaxX, contentY+metrics.footerHeight)

	return buttonListLayout{
		cardRect:        cardRect,
		titleRect:       titleRect,
		descriptionRect: descriptionRect,
		rowRects:        rowRects,
		footerRect:      footerRect,
		pixelUnit:       metrics.pixelUnit,
	}
}

func (b *ButtonListModal) layoutIsCurrent(width, height int) bool {
	if width <= 0 || height <= 0 {
		return true
	}

	return b.lastWidth == width && b.lastHeight == height
}
