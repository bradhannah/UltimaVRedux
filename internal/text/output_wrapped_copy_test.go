package text

import (
	"slices"
	"strings"
	"testing"
	"unicode/utf8"

	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestOutputWrappedCopySimpleRowsAndConfiguration(t *testing.T) {
	source := NewOutput(NewUltimaFont(12), 18, 2, 80)
	source.SetColor(ucolor.EGAYellow)
	source.SetDebugOptions(&references.DebugOptions{})
	source.AddText("alpha beta gamma")
	source.AddTextWithColor("delta epsilon", ucolor.EGALightBlue)
	source.lines[0].Alignment = etext.AlignCenter

	wrapped := source.WrappedCopy(7)

	assert.Equal(t, []string{"alpha", "beta", "gamma", "delta", "epsilon"}, outputPhysicalRows(wrapped))
	assert.Equal(t, 5, wrapped.maxLines, "expanded rows must not overwrite one another")
	assert.Equal(t, "alpha \nbeta \ngamma \ndelta \nepsilon", wrapped.GetOutputStr(false))
	assert.Same(t, source.Font, wrapped.Font)
	assert.Same(t, source.debugOptions, wrapped.debugOptions)
	assert.Equal(t, source.lineSpacing, wrapped.lineSpacing)
	assert.Equal(t, source.color, wrapped.color)
	assert.Equal(t, 7, wrapped.maxCharsPerLine)
	for _, line := range wrapped.lines[:3] {
		assert.Equal(t, ucolor.EGAYellow, line.Color)
		assert.Equal(t, etext.AlignCenter, line.Alignment)
	}
	for _, line := range wrapped.lines[3:] {
		assert.Equal(t, ucolor.EGALightBlue, line.Color)
		assert.Equal(t, etext.AlignStart, line.Alignment)
	}
}

func TestOutputWrappedCopyPreservesStyledSegments(t *testing.T) {
	source := NewOutput(nil, 18, 4, 80)
	segments := []ColorSegment{
		{Text: "Title: ", Color: ucolor.EGAYellow, FontType: FontTypeNormal, FontScale: FontScaleHeader},
		{Text: "abcdefghijk lmno pqrs\nRunes", Color: ucolor.EGALightBlue, FontType: FontTypeRunic, FontScale: 1.5},
	}
	source.AddStyledText(segments, etext.AlignEnd)
	assert.Equal(t, FontScaleHeader, source.lines[0].Segments[0].FontScale)
	assert.Equal(t, 1.5, source.lines[1].Segments[0].FontScale, "newline splitting retains scale")

	wrapped := source.WrappedCopy(8)

	assert.Equal(t, []string{"Title: ", "abcdefgh", "ijk lmno", "pqrs", "Runes"}, outputPhysicalRows(wrapped))
	for rowIndex, line := range wrapped.lines {
		assert.Equal(t, etext.AlignEnd, line.Alignment)
		require.NotEmpty(t, line.Segments)
		for _, segment := range line.Segments {
			expected := segments[1]
			if rowIndex == 0 {
				expected = segments[0]
			}
			expected.Text = segment.Text
			assert.Equal(t, expected, segment)
		}
	}
}

func TestOutputWrappedCopyCircularOrder(t *testing.T) {
	source := NewOutput(nil, 18, 3, 80)
	for _, row := range []string{"discarded", "oldest row", "middle row", "newest row"} {
		source.AddText(row)
	}
	require.Equal(t, 1, source.nextLineToIndex)

	wrapped := source.WrappedCopy(6)

	assert.Equal(t, []string{"oldest", "row", "middle", "row", "newest", "row"}, outputPhysicalRows(wrapped))
	assert.Equal(t, "oldest \nrow \nmiddle \nrow \nnewest \nrow", wrapped.GetOutputStr(false))
	assert.Equal(t, 0, wrapped.nextLineToIndex)
}

func TestOutputWrappedCopyTrimsEmptyEdgesPreservesInteriorBlankRows(t *testing.T) {
	source := NewOutput(nil, 18, 8, 80)
	source.AppendLine().NewLine().Done()
	source.AddText("first")
	source.AppendLine().NewLine().Done()
	source.AddText("\nlast")
	source.AppendLine().NewLine().Done()

	wrapped := source.WrappedCopy(80)

	assert.Equal(t, []string{"first", "", "", "last"}, outputPhysicalRows(wrapped))
	assert.Equal(t, source.GetOutputStr(false), wrapped.GetOutputStr(false)+" \n")
}

func TestOutputWrappedCopyEmpty(t *testing.T) {
	for _, capacity := range []int{0, 1, 4} {
		source := NewOutput(nil, 18, capacity, 80)
		source.AppendLine().NewLine().Done()

		wrapped := source.WrappedCopy(5)

		assert.Equal(t, 1, wrapped.maxLines)
		assert.Equal(t, []string{""}, outputPhysicalRows(wrapped))
		assert.Empty(t, wrapped.GetOutputStr(false))
		assert.Zero(t, wrapped.GetPanelContentLineCount())
		wrapped.AddText("ready")
		assert.Equal(t, "ready", wrapped.GetOutputStr(false))
	}
}

func TestOutputWrappedCopyRepeatedWidthsLeaveSourceUnchanged(t *testing.T) {
	source := NewOutput(nil, 18, 3, 80)
	source.AddText("alpha beta gamma")
	source.AddStyledText([]ColorSegment{
		{Text: "delta epsilon", Color: ucolor.EGAYellow, FontType: FontTypeRunic, FontScale: FontScaleHeader},
	}, etext.AlignCenter)
	before := *source
	before.lines = slices.Clone(source.lines)
	for index := range before.lines {
		before.lines[index].Segments = slices.Clone(source.lines[index].Segments)
	}

	for _, width := range []int{5, 80, 1, 10, 80} {
		wrapped := source.WrappedCopy(width)
		if width == 80 {
			assert.Equal(t, []string{"alpha beta gamma", "delta epsilon"}, outputPhysicalRows(wrapped))
		}
		for _, line := range wrapped.lines {
			for _, segment := range line.Segments {
				assert.Equal(t, FontScaleHeader, segment.FontScale)
				assert.Equal(t, FontTypeRunic, segment.FontType)
				assert.Equal(t, ucolor.EGAYellow, segment.Color)
			}
		}
		assert.Equal(t, before, *source)
		wrapped.RecolorCurrentLine(ucolor.EGARed)
		wrapped.RemoveRightSideCharacter()
		wrapped.Clear()
		assert.Equal(t, before, *source, "copy mutations must not alias source rows or segments")
	}
}

func TestOutputWrappedCopyRejectsInvalidWidth(t *testing.T) {
	source := NewOutput(nil, 18, 1, 80)
	assert.Panics(t, func() { source.WrappedCopy(0) })
	assert.Panics(t, func() { source.WrappedCopy(-1) })
}

func TestOutputWrappedCopyForWidthFitsMixedFonts(t *testing.T) {
	for _, test := range []struct {
		name  string
		scale float64
	}{
		{name: "normal", scale: FontScaleNormal},
		{name: "header", scale: FontScaleHeader},
		{name: "oversized", scale: 4},
	} {
		t.Run(test.name, func(t *testing.T) {
			source := NewOutput(NewUltimaFont(12), 18, 3, 1000)
			source.AddText(strings.Repeat("Simple text ", 8))
			source.AddStyledText([]ColorSegment{
				{Text: "Normal text ", Color: ucolor.EGAWhite, FontType: FontTypeNormal},
				{Text: strings.Repeat("RUNICATTACK", 8), Color: ucolor.EGAYellow, FontType: FontTypeRunic},
				{Text: strings.Repeat("IBMATTACK", 8), Color: ucolor.EGALightBlue, FontType: FontTypeIBM},
				{Text: strings.Repeat("Scaled text ", 8), Color: ucolor.EGARed, FontType: FontTypeNormal, FontScale: test.scale},
			}, etext.AlignCenter)
			before := *source
			before.lines = slices.Clone(source.lines)
			for index := range before.lines {
				before.lines[index].Segments = slices.Clone(source.lines[index].Segments)
			}
			expectedText := strings.ReplaceAll(strings.Join(outputPhysicalRows(source), ""), " ", "")

			for _, pixelWidth := range []int{1, 17, 36, 83, 300, 2000, 36} {
				wrapped := source.WrappedCopyForWidth(pixelWidth)
				for _, line := range wrapped.lines {
					segments := line.Segments
					if len(segments) == 0 {
						segments = []ColorSegment{{Text: line.Text, FontType: FontTypeNormal}}
					}
					var rowWidth float64
					var glyphCount int
					for _, segment := range segments {
						rowWidth += wrapped.getSegmentWidth(segment)
						glyphCount += utf8.RuneCountInString(segment.Text)
					}
					if rowWidth > float64(pixelWidth) {
						assert.Equal(t, 1, glyphCount,
							"only a single oversized glyph may exceed viewport %d (row width %f)", pixelWidth, rowWidth)
					}
				}
				assert.Equal(t, expectedText, strings.ReplaceAll(strings.Join(outputPhysicalRows(wrapped), ""), " ", ""))
				assert.Equal(t, before, *source)
				wrapped.RecolorCurrentLine(ucolor.EGAGreen)
				wrapped.RemoveRightSideCharacter()
				assert.Equal(t, before, *source, "pixel-width copies must not share mutable content")
			}
		})
	}
}

func TestOutputWrappedCopyForWidthMeasuresOnlyRetainedContent(t *testing.T) {
	source := NewOutput(NewUltimaFont(12), 18, 1, 1000)
	source.AddStyledText([]ColorSegment{
		{Text: "discarded", FontType: FontTypeNormal, FontScale: 100},
	}, etext.AlignStart)
	source.AddStyledText([]ColorSegment{
		{Text: strings.Repeat("R", 40), FontType: FontTypeRunic},
	}, etext.AlignStart)

	wrapped := source.WrappedCopyForWidth(95)

	assert.Equal(t, 5, wrapped.maxCharsPerLine, "runic glyph width is 18px, not the normal M width")
	assert.Len(t, wrapped.lines, 8)
	assert.Equal(t, strings.Repeat("R", 40), strings.Join(outputPhysicalRows(wrapped), ""))
}

func TestOutputWrappedCopyForWidthEmptyUsesNormalM(t *testing.T) {
	source := NewOutput(NewUltimaFont(12), 18, 0, 80)
	width, _ := source.Font.MeasureString("M")

	wrapped := source.WrappedCopyForWidth(100)

	assert.Equal(t, int(100/width), wrapped.maxCharsPerLine)
	assert.Equal(t, 1, wrapped.maxLines)
	assert.Empty(t, wrapped.GetOutputStr(false))
}

func TestOutputWrappedCopyForWidthRejectsInvalidConfiguration(t *testing.T) {
	source := NewOutput(NewUltimaFont(12), 18, 1, 80)
	assert.Panics(t, func() { source.WrappedCopyForWidth(0) })
	assert.Panics(t, func() { source.WrappedCopyForWidth(-1) })
	source.Font = nil
	assert.Panics(t, func() { source.WrappedCopyForWidth(100) })
	source.Font = &UltimaFont{}
	assert.Panics(t, func() { source.WrappedCopyForWidth(100) })
	source.Font.TextFace = &etext.GoTextFace{}
	assert.Panics(t, func() { source.WrappedCopyForWidth(100) })
	source.Font = NewUltimaFont(0)
	assert.Panics(t, func() { source.WrappedCopyForWidth(100) })
}
