package main

import (
	"image"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

const (
	audioSettingsPreferredWidthPercent   = 0.58
	audioSettingsPreferredHeightScale    = 1.30
	audioSettingsCenterDivisor           = 2
	audioSettingsSliderCount             = 3
	audioSettingsGameplayCount           = 14
	audioSettingsCardSectionGapCount     = 3
	audioSettingsPaddingUnits            = 2
	audioSettingsSectionGapUnits         = 2
	audioSettingsTabVerticalUnits        = 1
	audioSettingsSliderHeightUnits       = 6
	audioSettingsSliderRowGapUnits       = 2
	audioSettingsGameplayRowPaddingUnits = 0
	audioSettingsDetailLines             = 32
	audioSettingsFooterLines             = 4
	audioSettingsDisplayPresetLines      = 3
	audioSettingsGameplayListRatio       = 0.50
	audioSettingsDisplayLabelRatio       = 0.36
)

type audioSettingsLayout struct {
	cardRect          image.Rectangle
	titleRect         image.Rectangle
	tabRects          []image.Rectangle
	bodyRect          image.Rectangle
	sliderLabelRects  []image.Rectangle
	sliderValueRects  []image.Rectangle
	sliderRects       []image.Rectangle
	gameplayListRect  image.Rectangle
	gameplayRows      []image.Rectangle
	gameplayDetail    image.Rectangle
	displayRows       []image.Rectangle
	displaySelections []image.Rectangle
	displayLabels     []image.Rectangle
	displayValues     []image.Rectangle
	displayHeading    image.Rectangle
	displayHelp       image.Rectangle
	footerRect        image.Rectangle
	pixelUnit         int
}

type audioSettingsMetrics struct {
	padding         int
	sectionGap      int
	titleHeight     int
	tabHeight       int
	bodyHeight      int
	footerHeight    int
	labelHeight     int
	sliderLabelGap  int
	sliderRowHeight int
	sliderRowGap    int
	cardHeight      int
}

func (d *ConfigurationDialog) ensureLayout(width, height int) {
	if d.layoutIsCurrent(width, height) {
		return
	}
	d.lastWidth = width
	d.lastHeight = height
	pixelUnit := widgets.ClassicUIPixelUnit(width, height)
	d.initializeLayoutOutputs(width)
	placementRect := audioSettingsPercentRect(*d.placement, width, height)
	metrics := d.measureLayout(pixelUnit, placementRect.Dx())
	cardWidth := d.measureCardWidth(width, placementRect, metrics.padding)
	contentWidth := max(1, cardWidth-metrics.padding*audioSettingsCenterDivisor)
	d.configureLayoutOutputs(contentWidth)
	metrics = d.measureLayout(pixelUnit, contentWidth)
	metrics.fitCardHeight(placementRect.Dy())
	cardRect := centeredAudioSettingsCard(placementRect, cardWidth, metrics.cardHeight)
	titleRect, tabRects, bodyRect, footerRect := d.buildSections(cardRect, metrics, pixelUnit)
	labelRects, valueRects, sliderRects := d.buildSliderRects(bodyRect, metrics)
	gameplayListRect, gameplayRows, gameplayDetail := d.buildGameplayRects(bodyRect, pixelUnit)
	displayRows := buildDisplayRects(
		bodyRect,
		metrics.labelHeight,
		int(math.Ceil(d.presetOutput.GetTotalHeightForLines(audioSettingsDisplayPresetLines))),
		pixelUnit,
	)
	textInset := max(1, pixelUnit*audioSettingsCenterDivisor)
	headingTop := bodyRect.Min.Y + metrics.labelHeight*audioSettingsSectionGapUnits
	headingRect := image.Rect(bodyRect.Min.X, headingTop, bodyRect.Max.X, headingTop+metrics.labelHeight)
	helpTop := displayRows[1].Max.Y + metrics.labelHeight
	helpRect := image.Rect(bodyRect.Min.X, helpTop, bodyRect.Max.X, bodyRect.Max.Y)
	displayLabels, displayValues := buildDisplayTextRects(displayRows, textInset)
	displaySelections := buildDisplaySelectionRects(displayRows, metrics.labelHeight, pixelUnit)
	d.layout = audioSettingsLayout{
		cardRect:          cardRect,
		titleRect:         titleRect,
		tabRects:          tabRects,
		bodyRect:          bodyRect,
		sliderLabelRects:  labelRects,
		sliderValueRects:  valueRects,
		sliderRects:       sliderRects,
		gameplayListRect:  gameplayListRect,
		gameplayRows:      gameplayRows,
		gameplayDetail:    gameplayDetail,
		displayRows:       displayRows,
		displaySelections: displaySelections,
		displayLabels:     displayLabels,
		displayValues:     displayValues,
		displayHeading:    headingRect,
		displayHelp:       helpRect,
		footerRect:        footerRect,
		pixelUnit:         pixelUnit,
	}
	d.musicSlider.SetBounds(sliderRects[0])
	d.sfxSlider.SetBounds(sliderRects[1])
	d.ambientSlider.SetBounds(sliderRects[2])
	d.configurePaneTextWidths(textInset)
	d.ensureGameplaySelectionVisible()
}

func (metrics *audioSettingsMetrics) fitCardHeight(availableHeight int) {
	preferredCardHeight := min(availableHeight,
		int(math.Ceil(float64(metrics.cardHeight)*audioSettingsPreferredHeightScale)))
	if preferredCardHeight > metrics.cardHeight {
		metrics.bodyHeight += preferredCardHeight - metrics.cardHeight
		metrics.cardHeight = preferredCardHeight
	}
	if metrics.cardHeight > availableHeight {
		metrics.bodyHeight -= metrics.cardHeight - availableHeight
		metrics.cardHeight = availableHeight
	}
}

func (d *ConfigurationDialog) configurePaneTextWidths(textInset int) {
	d.gameplayOutput.SetMaxCharsPerLine(max(
		1,
		d.gameplayOutput.Font.CharsForWidth(d.gameplayTextWidth(d.layout.gameplayListRect.Dx(), d.layout.pixelUnit)),
	))
	d.detailOutput.SetMaxCharsPerLine(max(
		1,
		d.detailOutput.Font.CharsForWidth(d.layout.gameplayDetail.Dx()-textInset*audioSettingsCenterDivisor),
	))
	d.labelOutput.SetMaxCharsPerLine(max(1, d.labelOutput.Font.CharsForWidth(d.layout.displayLabels[0].Dx())))
	d.valueOutput.SetMaxCharsPerLine(max(1, d.valueOutput.Font.CharsForWidth(d.layout.displayValues[0].Dx())))
}

func buildDisplayRects(bodyRect image.Rectangle, textHeight, _, pixelUnit int) []image.Rectangle {
	const displayVisualRowCount = 2
	selectionHeight := textHeight + pixelUnit*audioSettingsPaddingUnits*audioSettingsCenterDivisor
	rowGap := max(textHeight/audioSettingsCenterDivisor, pixelUnit)
	rows := make([]image.Rectangle, displayVisualRowCount)
	rowTop := bodyRect.Min.Y + textHeight*(audioSettingsSectionGapUnits+1) + rowGap
	for index := range rows {
		rowHeight := selectionHeight
		rows[index] = image.Rect(bodyRect.Min.X, rowTop, bodyRect.Max.X, rowTop+rowHeight)
		rowTop += rowHeight + rowGap
	}

	return rows
}

func buildDisplayTextRects(rows []image.Rectangle, inset int) ([]image.Rectangle, []image.Rectangle) {
	labelRects := make([]image.Rectangle, len(rows))
	valueRects := make([]image.Rectangle, len(rows))
	for index, row := range rows {
		splitX := row.Min.X + int(math.Round(float64(row.Dx())*audioSettingsDisplayLabelRatio))
		labelRects[index] = image.Rect(row.Min.X+inset, row.Min.Y, splitX-inset, row.Max.Y)
		valueRects[index] = image.Rect(splitX+inset, row.Min.Y, row.Max.X-inset, row.Max.Y)
	}

	return labelRects, valueRects
}

func buildDisplaySelectionRects(rows []image.Rectangle, textHeight, pixelUnit int) []image.Rectangle {
	selectionHeight := textHeight + pixelUnit*audioSettingsCenterDivisor
	selections := make([]image.Rectangle, len(rows))
	for index, row := range rows {
		top := row.Min.Y + (row.Dy()-selectionHeight)/audioSettingsCenterDivisor
		selections[index] = image.Rect(row.Min.X, top, row.Max.X, top+selectionHeight)
	}

	return selections
}

func (d *ConfigurationDialog) layoutIsCurrent(width, height int) bool {
	if width <= 0 || height <= 0 {
		return true
	}

	return d.lastWidth == width && d.lastHeight == height
}

func (d *ConfigurationDialog) initializeLayoutOutputs(width int) {
	_, d.titleOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeTitle,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	}, width)
	_, d.tabOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	}, width)
	// Even a one-character-wide label must retain its leading lines. Capacity
	// follows the complete source labels, independently of their measured wrapping.
	gameplayLines := 1
	for _, setting := range gameplaySettings() {
		gameplayLines = max(gameplayLines, len([]rune(setting.label)))
	}
	_, d.gameplayOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              gameplayLines,
		MaxChars:              1,
	}, width)
	_, d.detailOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              audioSettingsDetailLines,
		MaxChars:              1,
	}, width)
	_, d.labelOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	}, width)
	_, d.valueOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              1,
		MaxChars:              1,
	}, width)
	_, d.presetOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              audioSettingsDisplayPresetLines,
		MaxChars:              1,
	}, width)
	_, d.footerOutput = widgets.GetScaledFontWithOutputForWidth(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 0,
		MaxLines:              audioSettingsFooterLines,
		MaxChars:              1,
	}, width)
}

func (d *ConfigurationDialog) configureLayoutOutputs(contentWidth int) {
	d.titleOutput.SetMaxCharsPerLine(max(1, d.titleOutput.Font.CharsForWidth(contentWidth)))
	d.titleOutput.Reset()
	d.titleOutput.AppendLine().Text("Configuration", widgets.DialogHeaderColor).Done()
	d.tabOutput.SetMaxCharsPerLine(max(1, d.tabOutput.Font.CharsForWidth(contentWidth)))
	d.gameplayOutput.SetMaxCharsPerLine(max(1, d.gameplayOutput.Font.CharsForWidth(contentWidth)))
	d.detailOutput.SetMaxCharsPerLine(max(1, d.detailOutput.Font.CharsForWidth(contentWidth)))
	d.labelOutput.SetMaxCharsPerLine(max(1, d.labelOutput.Font.CharsForWidth(contentWidth)))
	d.valueOutput.SetMaxCharsPerLine(max(1, d.valueOutput.Font.CharsForWidth(contentWidth)))
	d.presetOutput.SetMaxCharsPerLine(max(1, d.presetOutput.Font.CharsForWidth(contentWidth)))
	d.footerOutput.SetMaxCharsPerLine(max(1, d.footerOutput.Font.CharsForWidth(contentWidth)))
}

func (d *ConfigurationDialog) measureCardWidth(
	width int,
	placementRect image.Rectangle,
	padding int,
) int {
	preferredWidth := int(math.Round(audioSettingsPreferredWidthPercent * float64(width)))
	titleWidth, _ := d.titleOutput.Font.MeasureString("Configuration")
	minimumWidth := int(math.Ceil(titleWidth)) + padding*audioSettingsCenterDivisor

	return min(placementRect.Dx(), max(preferredWidth, minimumWidth))
}

func (d *ConfigurationDialog) measureLayout(pixelUnit, contentWidth int) audioSettingsMetrics {
	padding := max(1, pixelUnit*audioSettingsPaddingUnits)
	sectionGap := max(1, pixelUnit*audioSettingsSectionGapUnits)
	titleHeight := int(math.Ceil(d.titleOutput.GetFontHeight()))
	tabHeight := int(math.Ceil(d.tabOutput.GetFontHeight())) +
		pixelUnit*audioSettingsTabVerticalUnits*audioSettingsCenterDivisor
	labelHeight := int(math.Ceil(d.labelOutput.GetFontHeight()))
	sliderHeight := max(pixelUnit*audioSettingsSliderHeightUnits, labelHeight)
	sliderLabelGap := max(1, pixelUnit)
	sliderRowHeight := labelHeight + sliderLabelGap + sliderHeight
	sliderRowGap := max(1, pixelUnit*audioSettingsSliderRowGapUnits)
	soundHeight := sliderRowHeight*audioSettingsSliderCount +
		sliderRowGap*(audioSettingsSliderCount-1)
	bodyHeight := max(soundHeight, d.measureGameplayHeight(pixelUnit, contentWidth, padding))
	const displayBodyLines = 14
	displayHeight := int(math.Ceil(d.detailOutput.GetTotalHeightForLines(displayBodyLines))) +
		padding*audioSettingsCenterDivisor
	bodyHeight = max(bodyHeight, displayHeight)
	footerHeight := int(math.Ceil(d.footerOutput.GetTotalHeightForLines(audioSettingsFooterLines)))
	cardHeight := padding*audioSettingsCenterDivisor + titleHeight + tabHeight + bodyHeight + footerHeight +
		sectionGap*audioSettingsCardSectionGapCount

	return audioSettingsMetrics{
		padding:         padding,
		sectionGap:      sectionGap,
		titleHeight:     titleHeight,
		tabHeight:       tabHeight,
		bodyHeight:      bodyHeight,
		footerHeight:    footerHeight,
		labelHeight:     labelHeight,
		sliderLabelGap:  sliderLabelGap,
		sliderRowHeight: sliderRowHeight,
		sliderRowGap:    sliderRowGap,
		cardHeight:      cardHeight,
	}
}

func (d *ConfigurationDialog) measureGameplayHeight(pixelUnit, contentWidth, padding int) int {
	gameplayListWidth := int(math.Round(float64(contentWidth) * audioSettingsGameplayListRatio))
	gameplayInset := max(1, pixelUnit*audioSettingsCenterDivisor)
	gameplayHeight := gameplayInset*audioSettingsCenterDivisor +
		max(1, pixelUnit/audioSettingsCenterDivisor)*(audioSettingsGameplayCount-1)
	for _, setting := range gameplaySettings() {
		measurement := text.MeasureText(
			setting.label,
			d.gameplayOutput.Font,
			d.gameplayOutput.GetLineSpacing(),
			max(1, d.gameplayOutput.Font.CharsForWidth(d.gameplayLabelWidth(gameplayListWidth, pixelUnit, setting))),
		)
		gameplayHeight += int(math.Ceil(measurement.TotalHeight)) +
			pixelUnit*audioSettingsGameplayRowPaddingUnits
	}
	detailWidth := contentWidth - gameplayListWidth - gameplayInset*audioSettingsCenterDivisor
	detailChars := max(1, d.detailOutput.Font.CharsForWidth(detailWidth))
	d.detailOutput.SetMaxCharsPerLine(detailChars)
	detailLines := 1
	for _, setting := range gameplaySettings() {
		value := gameplaySettingOffLabel
		if setting.numeric != nil {
			value = setting.numeric.format(setting.numeric.spec.Max)
		}
		populateGameplayDetail(d.detailOutput, setting, value)
		detailLines = max(detailLines, d.detailOutput.GetPanelContentLineCount())
	}

	return max(gameplayHeight, int(math.Ceil(d.detailOutput.GetTotalHeightForLines(detailLines)))+
		padding*audioSettingsCenterDivisor)
}

func (d *ConfigurationDialog) buildGameplayRects(
	bodyRect image.Rectangle,
	pixelUnit int,
) (image.Rectangle, []image.Rectangle, image.Rectangle) {
	listWidth := int(math.Round(float64(bodyRect.Dx()) * audioSettingsGameplayListRatio))
	listRect := image.Rect(bodyRect.Min.X, bodyRect.Min.Y, bodyRect.Min.X+listWidth, bodyRect.Max.Y)
	detailRect := image.Rect(listRect.Max.X, bodyRect.Min.Y, bodyRect.Max.X, bodyRect.Max.Y)
	rows := make([]image.Rectangle, audioSettingsGameplayCount)
	inset := max(1, pixelUnit*audioSettingsCenterDivisor)
	rowGap := max(1, pixelUnit/audioSettingsCenterDivisor)
	rowHeights := make([]int, len(rows))
	for index, setting := range gameplaySettings() {
		measurement := text.MeasureText(
			setting.label,
			d.gameplayOutput.Font,
			d.gameplayOutput.GetLineSpacing(),
			max(1, d.gameplayOutput.Font.CharsForWidth(d.gameplayLabelWidth(listRect.Dx(), pixelUnit, setting))),
		)
		rowHeights[index] = int(math.Ceil(measurement.TotalHeight)) + pixelUnit*audioSettingsGameplayRowPaddingUnits
	}
	rowTop := listRect.Min.Y + inset
	for index, rowHeight := range rowHeights {
		rows[index] = image.Rect(listRect.Min.X, rowTop, listRect.Max.X-inset, rowTop+rowHeight)
		rowTop += rowHeight + rowGap
	}

	return listRect, rows, detailRect
}

// gameplayCheckboxWidth fits the checkbox cell that starts every row.
func (d *ConfigurationDialog) gameplayCheckboxWidth() int {
	width, _ := d.gameplayOutput.Font.MeasureString("[X] ")

	return int(math.Ceil(width))
}

// gameplayValueWidth fits a numeric row's widest choice, drawn at the row's
// right edge, so a long choice such as Ludicrous narrows only its own label.
func (d *ConfigurationDialog) gameplayValueWidth(setting gameplaySettingDefinition) int {
	if setting.numeric == nil {
		return 0
	}
	width := 0.0
	for _, choice := range setting.numeric.spec.Choices() {
		choiceWidth, _ := d.gameplayOutput.Font.MeasureString(" " + setting.numeric.format(choice))
		width = max(width, choiceWidth)
	}

	return int(math.Ceil(width))
}

// gameplayTextWidth is a checkbox row's label width.
func (d *ConfigurationDialog) gameplayTextWidth(listWidth, pixelUnit int) int {
	inset := max(1, pixelUnit*audioSettingsCenterDivisor)

	return max(1, listWidth-inset*(audioSettingsCenterDivisor+1)-d.gameplayCheckboxWidth())
}

func (d *ConfigurationDialog) gameplayLabelWidth(listWidth, pixelUnit int, setting gameplaySettingDefinition) int {
	return max(1, d.gameplayTextWidth(listWidth, pixelUnit)-d.gameplayValueWidth(setting))
}

func (d *ConfigurationDialog) gameplayViewport() image.Rectangle {
	inset := max(1, d.layout.pixelUnit*audioSettingsCenterDivisor)

	return image.Rect(d.layout.gameplayListRect.Min.X, d.layout.gameplayListRect.Min.Y+inset,
		d.layout.gameplayListRect.Max.X-inset, d.layout.gameplayListRect.Max.Y-inset)
}

func (d *ConfigurationDialog) gameplayVisibleRow(index int) image.Rectangle {
	offset := d.layout.gameplayRows[d.firstGameplayIndex].Min.Y - d.gameplayViewport().Min.Y

	return d.layout.gameplayRows[index].Sub(image.Pt(0, offset))
}

func (d *ConfigurationDialog) ensureGameplaySelectionVisible() {
	if len(d.layout.gameplayRows) == 0 {
		return
	}
	d.firstGameplayIndex = min(d.firstGameplayIndex, d.focusedGameplayIndex)
	for d.firstGameplayIndex < d.focusedGameplayIndex &&
		d.gameplayVisibleRow(d.focusedGameplayIndex).Max.Y > d.gameplayViewport().Max.Y {
		d.firstGameplayIndex++
	}
}

func centeredAudioSettingsCard(placement image.Rectangle, width, height int) image.Rectangle {
	centerX := placement.Min.X + placement.Dx()/audioSettingsCenterDivisor
	centerY := placement.Min.Y + placement.Dy()/audioSettingsCenterDivisor
	minimumX := min(max(placement.Min.X, centerX-width/audioSettingsCenterDivisor), placement.Max.X-width)
	minimumY := min(max(placement.Min.Y, centerY-height/audioSettingsCenterDivisor), placement.Max.Y-height)

	return image.Rect(minimumX, minimumY, minimumX+width, minimumY+height)
}

func (d *ConfigurationDialog) buildSections(
	cardRect image.Rectangle,
	metrics audioSettingsMetrics,
	pixelUnit int,
) (image.Rectangle, []image.Rectangle, image.Rectangle, image.Rectangle) {
	contentMinX := cardRect.Min.X + metrics.padding
	contentMaxX := cardRect.Max.X - metrics.padding
	contentY := cardRect.Min.Y + metrics.padding
	titleRect := image.Rect(
		contentMinX,
		contentY,
		contentMaxX,
		contentY+metrics.titleHeight,
	)
	contentY = titleRect.Max.Y + metrics.sectionGap
	tabRects := d.buildTabRects(contentMinX, contentMaxX, contentY, metrics.tabHeight, pixelUnit)
	contentY += metrics.tabHeight + metrics.sectionGap
	bodyRect := image.Rect(contentMinX, contentY, contentMaxX, contentY+metrics.bodyHeight)
	contentY = bodyRect.Max.Y + metrics.sectionGap
	footerRect := image.Rect(contentMinX, contentY, contentMaxX, contentY+metrics.footerHeight)

	return titleRect, tabRects, bodyRect, footerRect
}

func (d *ConfigurationDialog) buildTabRects(
	contentMinX, contentMaxX int,
	top, height, pixelUnit int,
) []image.Rectangle {
	if len(d.tabNames) == 0 {
		return nil
	}
	tabGap := max(1, pixelUnit)
	gapWidth := tabGap * (len(d.tabNames) - 1)
	availableWidth := max(len(d.tabNames), contentMaxX-contentMinX-gapWidth)
	tabWidth := availableWidth / len(d.tabNames)
	tabGroupWidth := tabWidth*len(d.tabNames) + gapWidth
	tabX := contentMinX + (contentMaxX-contentMinX-tabGroupWidth)/audioSettingsCenterDivisor
	tabRects := make([]image.Rectangle, len(d.tabNames))
	for index := range d.tabNames {
		tabRects[index] = image.Rect(tabX, top, tabX+tabWidth, top+height)
		tabX = tabRects[index].Max.X + tabGap
	}

	return tabRects
}

func (d *ConfigurationDialog) buildSliderRects(
	bodyRect image.Rectangle,
	metrics audioSettingsMetrics,
) ([]image.Rectangle, []image.Rectangle, []image.Rectangle) {
	sliderContentHeight := metrics.sliderRowHeight*audioSettingsSliderCount +
		metrics.sliderRowGap*(audioSettingsSliderCount-1)
	sliderY := bodyRect.Min.Y +
		max(0, (bodyRect.Dy()-sliderContentHeight)/audioSettingsCenterDivisor)
	labelRects := make([]image.Rectangle, audioSettingsSliderCount)
	valueRects := make([]image.Rectangle, audioSettingsSliderCount)
	sliderRects := make([]image.Rectangle, audioSettingsSliderCount)
	valueWidth, _ := d.valueOutput.Font.MeasureString("10")
	for index := range audioSettingsSliderCount {
		rowTop := sliderY + index*(metrics.sliderRowHeight+metrics.sliderRowGap)
		labelRects[index] = image.Rect(
			bodyRect.Min.X,
			rowTop,
			bodyRect.Max.X,
			rowTop+metrics.labelHeight,
		)
		valueRects[index] = image.Rect(
			bodyRect.Max.X-int(math.Ceil(valueWidth)),
			rowTop,
			bodyRect.Max.X,
			rowTop+metrics.labelHeight,
		)
		sliderRects[index] = image.Rect(
			bodyRect.Min.X,
			rowTop+metrics.labelHeight+metrics.sliderLabelGap,
			bodyRect.Max.X,
			rowTop+metrics.sliderRowHeight,
		)
	}

	return labelRects, valueRects, sliderRects
}

func audioSettingsPercentRect(placement sprites.PercentBasedPlacement, width, height int) image.Rectangle {
	return image.Rect(
		int(math.Round(placement.StartPercentX*float64(width))),
		int(math.Round(placement.StartPercentY*float64(height))),
		int(math.Round(placement.EndPercentX*float64(width))),
		int(math.Round(placement.EndPercentY*float64(height))),
	)
}
