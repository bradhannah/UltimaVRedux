package text

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// FontScaleHeader is a fixture for scaled wrapping and copy tests.
const FontScaleHeader float64 = 1.25

// =============================================================================
// Runic Font Segment Tests
// =============================================================================

// TestRunic_DefaultColor validates that Runic() with no previous segment
// defaults to EGABrown color (matching the original Ultima V runic color scheme)
func TestRunic_DefaultColor(t *testing.T) {
	font := NewUltimaFont(12.0)
	output := NewOutput(font, 18.0, 10, 30)

	lb := output.AppendLine()
	lb.Runic("PIGDOG")

	segments := lb.GetSegments()
	require.Len(t, segments, 1, "Should have 1 runic segment")
	assert.Equal(t, "PIGDOG", segments[0].Text)
	assert.Equal(t, ucolor.EGABrown, segments[0].Color, "Runic() without prior segment should default to EGABrown")
	assert.Equal(t, FontTypeRunic, segments[0].FontType, "FontType should be FontTypeRunic")
}

// TestRunic_InheritsColor validates that Runic() inherits color from the
// previous segment when one exists
func TestRunic_InheritsColor(t *testing.T) {
	font := NewUltimaFont(12.0)
	output := NewOutput(font, 18.0, 10, 30)

	lb := output.AppendLine()
	lb.White("He says: ").Runic("PIGDOG")

	segments := lb.GetSegments()
	require.Len(t, segments, 2, "Should have 2 segments")

	// First segment: white TTF text
	assert.Equal(t, "He says: ", segments[0].Text)
	assert.Equal(t, ucolor.EGAWhite, segments[0].Color)
	assert.Equal(t, FontTypeNormal, segments[0].FontType)

	// Second segment: runic text inheriting white
	assert.Equal(t, "PIGDOG", segments[1].Text)
	assert.Equal(t, ucolor.EGAWhite, segments[1].Color, "Runic() should inherit White color from previous segment")
	assert.Equal(t, FontTypeRunic, segments[1].FontType)
}

// TestRunicWithColor validates that RunicWithColor() uses the specified color
// regardless of any previous segments
func TestRunicWithColor(t *testing.T) {
	font := NewUltimaFont(12.0)
	output := NewOutput(font, 18.0, 10, 30)

	lb := output.AppendLine()
	lb.White("He says: ").RunicWithColor("PIGDOG", ucolor.EGAYellow)

	segments := lb.GetSegments()
	require.Len(t, segments, 2, "Should have 2 segments")

	// Second segment should use specified color, not inherited
	assert.Equal(t, "PIGDOG", segments[1].Text)
	assert.Equal(t, ucolor.EGAYellow, segments[1].Color, "RunicWithColor() should use specified color")
	assert.Equal(t, FontTypeRunic, segments[1].FontType)
}

// TestRunic_FontType validates that Runic segments have FontTypeRunic set
func TestRunic_FontType(t *testing.T) {
	font := NewUltimaFont(12.0)
	output := NewOutput(font, 18.0, 10, 30)

	lb := output.AppendLine()
	lb.Runic("TEST")

	segments := lb.GetSegments()
	require.Len(t, segments, 1)
	assert.Equal(t, FontTypeRunic, segments[0].FontType, "Runic segment must have FontTypeRunic")
}

// TestMixedSegments_FontTypes validates that a line with both normal and runic
// segments has the correct FontType for each segment
func TestMixedSegments_FontTypes(t *testing.T) {
	font := NewUltimaFont(12.0)
	output := NewOutput(font, 18.0, 10, 30)

	lb := output.AppendLine()
	lb.White("Normal ").Runic("RUNIC").Yellow(" more normal")

	segments := lb.GetSegments()
	require.Len(t, segments, 3, "Should have 3 segments")

	// First: normal TTF
	assert.Equal(t, "Normal ", segments[0].Text)
	assert.Equal(t, FontTypeNormal, segments[0].FontType, "First segment should be FontTypeNormal")

	// Second: runic
	assert.Equal(t, "RUNIC", segments[1].Text)
	assert.Equal(t, FontTypeRunic, segments[1].FontType, "Middle segment should be FontTypeRunic")

	// Third: normal TTF again
	assert.Equal(t, " more normal", segments[2].Text)
	assert.Equal(t, FontTypeNormal, segments[2].FontType, "Last segment should be FontTypeNormal")
}

// TestRunic_InheritsFromVariousColors validates color inheritance from
// different colored segments
func TestRunic_InheritsFromVariousColors(t *testing.T) {
	tests := []struct {
		name          string
		colorMethod   func(*LineBuilder) *LineBuilder
		expectedColor interface{}
	}{
		{"Yellow", func(lb *LineBuilder) *LineBuilder { return lb.Yellow("text") }, ucolor.EGAYellow},
		{"LightGreen", func(lb *LineBuilder) *LineBuilder { return lb.LightGreen("text") }, ucolor.EGALightGreen},
		{"Cyan", func(lb *LineBuilder) *LineBuilder { return lb.Cyan("text") }, ucolor.EGACyan},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			font := NewUltimaFont(12.0)
			output := NewOutput(font, 18.0, 10, 30)

			lb := output.AppendLine()
			tt.colorMethod(lb).Runic("RUNIC")

			segments := lb.GetSegments()
			require.Len(t, segments, 2)
			assert.Equal(t, tt.expectedColor, segments[1].Color, "Runic should inherit %s color", tt.name)
			assert.Equal(t, FontTypeRunic, segments[1].FontType)
		})
	}
}

// TestRunic_MultipleRunicSegments validates multiple runic segments in one line
func TestRunic_MultipleRunicSegments(t *testing.T) {
	font := NewUltimaFont(12.0)
	output := NewOutput(font, 18.0, 10, 30)

	lb := output.AppendLine()
	lb.RunicWithColor("FIRST", ucolor.EGAWhite).
		White(" and ").
		RunicWithColor("SECOND", ucolor.EGAYellow)

	segments := lb.GetSegments()
	require.Len(t, segments, 3, "Should have 3 segments")

	assert.Equal(t, FontTypeRunic, segments[0].FontType, "First runic segment")
	assert.Equal(t, FontTypeNormal, segments[1].FontType, "Middle TTF segment")
	assert.Equal(t, FontTypeRunic, segments[2].FontType, "Second runic segment")
}

func TestIBMWithColorUsesFixedCells(t *testing.T) {
	output := NewOutput(NewUltimaFont(8), BitmapCharHeight, 1, 40)
	line := output.AppendLine().IBMWithColor("AVATAR", ucolor.EGAYellow)

	segments := line.GetSegments()
	require.Len(t, segments, 1)
	assert.Equal(t, FontTypeIBM, segments[0].FontType)
	assert.Equal(t, ucolor.EGAYellow, segments[0].Color)
	assert.Equal(t, float64(len("AVATAR")*BitmapCharWidth), output.getSegmentWidth(segments[0]))
}
