package widgets

import (
	"image"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

const (
	saveDescriptionSectionGaps   = 5
	saveDescriptionCenterPercent = 0.5
)

func (m *SaveDescriptionEntryModal) configureDescriptionRendering() {
	_, m.titleOutput = GetScaledFontWithOutput(FontConfig{
		Size: FontSizeTitle, LineSpacingMultiplier: 0, MaxLines: 1, MaxChars: modalDialogTitleMaxChars,
	})
	m.titleOutput.AppendLine().Text(m.title, DialogHeaderColor).Done()
	padding := m.scaledDimension(saveDescriptionContentPadding, saveDescriptionMinimumPadding)
	width := m.border.GetInteriorBounds().Dx() - padding*saveDescriptionCenterDivisor
	maxChars := m.font.CharsForWidth(max(1, width))
	noticeLines := max(1, text.MeasureText(m.notice, m.font, m.fieldOutput.GetLineSpacing(), maxChars).Lines)
	m.noticeOutput = text.NewOutput(m.font, m.fieldOutput.GetLineSpacing(), noticeLines, maxChars)
	if m.notice != "" {
		m.noticeOutput.AppendLine().Text(m.notice, DialogStatusWarningColor).WordWrap().Done()
	}
	spacing := m.scaledDimension(saveDescriptionVerticalSpacing, saveDescriptionMinimumSpacing)
	fieldPadding := m.scaledDimension(saveDescriptionFieldPaddingY, saveDescriptionMinimumPaddingY) + m.pixelUnit()
	// Reserve the complete measured warning and validation space, not a fixed
	// percentage that silently clips long cross-playthrough warnings at 720p.
	contentHeight := m.titleOutput.GetFontHeight() + m.promptOutput.GetFontHeight() +
		m.fieldOutput.GetFontHeight() + float64(fieldPadding*saveDescriptionCenterDivisor) +
		m.noticeOutput.GetTotalHeightForLines(noticeLines) +
		m.messageOutput.GetTotalHeightForLines(saveDescriptionValidationLines) +
		m.hintOutput.GetTotalHeightForLines(saveDescriptionHintMaximumLines) + float64(spacing*saveDescriptionSectionGaps)
	height := int(math.Ceil((contentHeight + float64(padding*saveDescriptionCenterDivisor)) /
		(1 - float64(LayoutSpacingMedium)*saveDescriptionCenterDivisor)))
	halfHeight := float64(height) / float64(m.lastHeight) / saveDescriptionCenterDivisor
	m.border = newDialogBorder(sprites.PercentBasedPlacement{
		StartPercentX: saveDescriptionModalStartX, EndPercentX: saveDescriptionModalEndX,
		StartPercentY: saveDescriptionCenterPercent - halfHeight, EndPercentY: saveDescriptionCenterPercent + halfHeight,
	})
}

func (m *SaveDescriptionEntryModal) calculateRects() saveDescriptionEntryRects {
	padding := m.scaledDimension(saveDescriptionContentPadding, saveDescriptionMinimumPadding)
	content := m.border.GetInteriorBounds().Inset(padding)
	spacing := m.scaledDimension(saveDescriptionVerticalSpacing, saveDescriptionMinimumSpacing)
	y := content.Min.Y
	next := func(height float64) image.Rectangle {
		rect := image.Rect(content.Min.X, y, content.Max.X, y+int(math.Ceil(height)))
		y = rect.Max.Y + spacing

		return rect
	}
	var rects saveDescriptionEntryRects
	rects.title = next(m.titleOutput.GetFontHeight())
	rects.prompt = next(m.promptOutput.GetFontHeight())
	rects.field = next(m.fieldOutput.GetFontHeight() + float64(saveDescriptionCenterDivisor*
		(m.pixelUnit()+m.scaledDimension(saveDescriptionFieldPaddingY, saveDescriptionMinimumPaddingY))))
	rects.notice = next(m.noticeOutput.GetTotalHeightForLines(max(1, m.noticeOutput.GetPanelContentLineCount())))
	rects.message = next(m.messageOutput.GetTotalHeightForLines(saveDescriptionValidationLines))
	rects.hint = next(m.hintOutput.GetTotalHeightForLines(saveDescriptionHintMaximumLines))

	return rects
}
