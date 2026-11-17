package text

import (
	"slices"
	"strings"
	"unicode/utf8"
)

// ============================================================================
// Text Wrapping and Line Utilities
// ============================================================================

// WrappedCopyForWidth uses the widest retained glyph (at least a normal M) to
// choose a conservative character capacity, then delegates to WrappedCopy.
// A single glyph wider than pixelWidth is retained on its own row.
func (o *Output) WrappedCopyForWidth(pixelWidth int) *Output {
	if pixelWidth <= 0 {
		panic("Output pixelWidth must be positive")
	}
	if o.Font == nil || o.Font.TextFace == nil || o.Font.TextFace.Source == nil {
		panic("Output WrappedCopyForWidth requires an initialized font")
	}
	maxWidth, _ := o.Font.MeasureString("M")
	if maxWidth <= 0 {
		panic("Output WrappedCopyForWidth requires a positive glyph width")
	}
	for _, line := range o.lines {
		segments := line.Segments
		if len(segments) == 0 {
			segments = []ColorSegment{{
				Text: line.Text, Color: line.Color, FontType: FontTypeNormal, FontScale: FontScaleNormal,
			}}
		}
		for _, segment := range segments {
			for _, glyph := range segment.Text {
				glyphSegment := segment
				glyphSegment.Text = string(glyph)
				maxWidth = max(maxWidth, o.getSegmentWidth(glyphSegment))
			}
		}
	}

	return o.WrappedCopy(max(1, int(float64(pixelWidth)/maxWidth)))
}

// WrappedCopy rewraps retained rows in chronological order without changing o.
// Empty edge rows are omitted; interior blank rows and original row boundaries
// are preserved. Re-layout should always copy the retained source, not a prior
// wrapped copy, because original rows are never joined when widening.
func (o *Output) WrappedCopy(maxCharsPerLine int) *Output {
	wrapped := *o
	wrapped.journalHistory = nil
	wrapped.history = nil // A presentation copy must not append to its source history.
	wrapped.SetMaxCharsPerLine(maxCharsPerLine)
	wrapped.lines = nil
	wrapped.lineLayouts = nil
	wrapped.nextLineToIndex = 0

	first, last := len(o.lines), -1
	for index := range o.lines {
		line := o.lines[(index+o.nextLineToIndex)%o.maxLines]
		if !styledLineIsEmpty(line) {
			first = min(first, index)
			last = index
		}
	}
	for index := first; index <= last; index++ {
		line := o.lines[(index+o.nextLineToIndex)%o.maxLines]
		segments := line.Segments
		if len(segments) == 0 {
			segments = []ColorSegment{{
				Text: line.Text, Color: line.Color, FontType: FontTypeNormal, FontScale: FontScaleNormal,
			}}
		}
		for _, row := range wrapped.wrapStyledLine(segments) {
			row.Alignment = line.Alignment
			row.Color = line.Color
			if len(line.Segments) == 0 {
				for _, segment := range row.Segments {
					row.Text += segment.Text
				}
				row.Segments = nil
			}
			wrapped.lines = append(wrapped.lines, row)
		}
	}
	wrapped.maxLines = max(len(wrapped.lines), 1)
	if len(wrapped.lines) == 0 {
		wrapped.lines = make([]StyledLine, wrapped.maxLines)
	}

	return &wrapped
}

// wrapStyledLine wraps colored segments across multiple lines while preserving colors and exact spacing.
// Handles explicit newlines (\n) by creating separate output lines.
// Segments are kept intact with their original spacing - no automatic space insertion.
func (o *Output) wrapStyledLine(segments []ColorSegment) []StyledLine {
	return o.wrapStyledLineWithContinuationIndent(segments, 0)
}

func (o *Output) wrapStyledLineWithContinuationIndent(
	segments []ColorSegment,
	continuationIndent int,
) []StyledLine {
	if len(segments) > 0 {
		length, fits := 0, true
		for _, segment := range segments {
			length += utf8.RuneCountInString(segment.Text)
			if strings.Contains(segment.Text, "\n") || length > o.maxCharsPerLine {
				fits = false

				break
			}
		}
		if fits {
			return []StyledLine{{Segments: slices.Clone(segments), Text: "", Color: nil, Alignment: 0}}
		}
	}
	// First pass: split segments at newline boundaries into logical lines
	logicalLines := splitSegmentsAtNewlines(segments)

	// Second pass: word-wrap each logical line
	var wrappedLines []StyledLine
	for _, lineSegments := range logicalLines {
		wrapped := o.wrapSingleLogicalLineWithContinuationIndent(lineSegments, continuationIndent)
		wrappedLines = append(wrappedLines, wrapped...)
	}

	// Post-process: merge punctuation-only lines back to previous line if they fit
	wrappedLines = mergePunctuationLines(wrappedLines, o.maxCharsPerLine)

	return wrappedLines
}

// splitSegmentsAtNewlines splits segments at \n boundaries, creating logical lines.
// Each logical line is a slice of segments that should appear on the same output line.
// Empty segments are preserved to maintain blank lines from consecutive \n characters.
func splitSegmentsAtNewlines(segments []ColorSegment) [][]ColorSegment {
	if len(segments) == 0 {
		return nil
	}

	var result [][]ColorSegment
	currentLine := make([]ColorSegment, 0, len(segments))

	for _, seg := range segments {
		// Check if segment contains newlines
		if !strings.Contains(seg.Text, "\n") {
			// No newlines - add segment to current line as-is
			currentLine = append(currentLine, seg)

			continue
		}

		// Segment contains newlines - split it
		parts := strings.Split(seg.Text, "\n")
		for i, part := range parts {
			if i > 0 {
				// Flush current line before starting new one
				result = append(result, currentLine)
				currentLine = make([]ColorSegment, 0, len(segments))
			}

			// Add non-empty part as segment (preserves color and font type)
			if part != "" {
				partSegment := seg
				partSegment.Text = part
				currentLine = append(currentLine, partSegment)
			}
		}
	}

	// Flush final line (even if empty, to preserve trailing blank lines)
	result = append(result, currentLine)

	return result
}

// wrapSingleLogicalLine wraps a single logical line (no embedded newlines) using word wrapping.
// This is the original wrapping logic extracted for reuse.
func (o *Output) wrapSingleLogicalLine(segments []ColorSegment) []StyledLine {
	return o.wrapSingleLogicalLineWithContinuationIndent(segments, 0)
}

//nolint:funlen // Word wrapping keeps segment colors, boundaries, indentation, and over-width words coherent.
func (o *Output) wrapSingleLogicalLineWithContinuationIndent(
	segments []ColorSegment,
	continuationIndent int,
) []StyledLine {
	// Handle empty line (blank line from \n\n)
	if len(segments) == 0 {
		return []StyledLine{{Segments: []ColorSegment{}, Text: "", Color: nil, Alignment: 0}}
	}

	var wrappedLines []StyledLine
	currentLine := StyledLine{Segments: []ColorSegment{}, Text: "", Color: nil, Alignment: 0}
	currentLength := 0
	newContinuationLine := func() (StyledLine, int) {
		line := StyledLine{
			Segments:  []ColorSegment{},
			Text:      "",
			Color:     nil,
			Alignment: 0,
		}
		if continuationIndent <= 0 {
			return line, 0
		}
		line.Segments = append(line.Segments, ColorSegment{
			Text:      strings.Repeat(" ", continuationIndent),
			Color:     segments[0].Color,
			FontType:  FontTypeNormal,
			FontScale: FontScaleNormal,
		})

		return line, continuationIndent
	}

	for _, seg := range segments {
		segText := seg.Text

		// If segment fits on current line, add it as-is
		if currentLength+utf8.RuneCountInString(segText) <= o.maxCharsPerLine {
			currentLine.Segments = append(currentLine.Segments, seg)
			currentLength += utf8.RuneCountInString(segText)

			continue
		}

		// Segment doesn't fit - need to wrap
		// Try to find last space in the part that would fit
		remainingSpace := o.maxCharsPerLine - currentLength

		wrapPoint := 0
		if remainingSpace > 0 {
			// Find last word boundary before max length
			wrapPoint = findLastSpace(segText, remainingSpace)
		}
		if wrapPoint > 0 {
			// Split at word boundary
			partSegment := seg
			partSegment.Text = segText[:wrapPoint]
			currentLine.Segments = append(currentLine.Segments, partSegment)
			segText = strings.TrimLeft(segText[wrapPoint:], " ")
		} else if currentLength > 0 {
			// Move the whole incoming word to the continuation row instead of
			// splitting it merely to fill the remaining columns. A full row
			// moves it too, so no continuation row starts with its space.
			segText = strings.TrimLeft(segText, " ")
		}

		// Flush current line and continue with remainder
		if len(currentLine.Segments) > 0 {
			wrappedLines = append(wrappedLines, currentLine)
			currentLine, currentLength = newContinuationLine()
		}

		// Add remaining text (might need further splitting if very long)
		for len(segText) > 0 {
			availableWidth := max(o.maxCharsPerLine-currentLength, 1)
			if utf8.RuneCountInString(segText) <= availableWidth {
				partSegment := seg
				partSegment.Text = segText
				currentLine.Segments = append(currentLine.Segments, partSegment)
				currentLength += utf8.RuneCountInString(segText)

				break
			}

			// Text still too long, wrap again
			wrapPoint := findLastSpace(segText, availableWidth)
			if wrapPoint == 0 {
				wrapPoint = byteIndexAfterRunes(segText, availableWidth)
			}

			partSegment := seg
			partSegment.Text = segText[:wrapPoint]
			currentLine.Segments = append(currentLine.Segments, partSegment)
			wrappedLines = append(wrappedLines, currentLine)
			currentLine, currentLength = newContinuationLine()

			segText = strings.TrimLeft(segText[wrapPoint:], " ")
		}
	}

	// Flush final line if it has content
	if len(currentLine.Segments) > 0 {
		wrappedLines = append(wrappedLines, currentLine)
	}

	// If no lines were generated, return empty line
	if len(wrappedLines) == 0 {
		return []StyledLine{{Segments: []ColorSegment{}, Text: "", Color: nil, Alignment: 0}}
	}

	return wrappedLines
}

// mergePunctuationLines merges lines that start with punctuation-only segments back to previous line
// This prevents closing quotes, periods, etc. from appearing alone on wrapped lines
// Example: "Awake Avatar and feel\nrestored!" becomes "Awake Avatar and feel\nrestored!"
func mergePunctuationLines(lines []StyledLine, maxChars int) []StyledLine {
	if len(lines) <= 1 {
		return lines // Nothing to merge
	}

	result := []StyledLine{}

	for i, line := range lines {
		if i == 0 {
			result = append(result, line)

			continue
		}

		// Check if this line starts with punctuation-only segments
		if isPunctuationOnlyLine(line) {
			prevLine := &result[len(result)-1]
			combinedLength := getStyledLineLength(prevLine) + getStyledLineLength(&line)

			// If merged line fits within max chars, merge it
			if combinedLength <= maxChars {
				prevLine.Segments = append(prevLine.Segments, line.Segments...)

				continue // Skip adding current line (it's merged)
			}
		}

		result = append(result, line)
	}

	return result
}

// isPunctuationOnlyLine returns true if all segments in the line contain only punctuation and whitespace
// Returns false for empty lines (blank lines should not be merged)
func isPunctuationOnlyLine(line StyledLine) bool {
	if len(line.Segments) == 0 {
		return false // Blank lines should not be treated as punctuation-only
	}

	for _, seg := range line.Segments {
		if !isOnlyPunctuation(seg.Text) {
			return false
		}
	}

	return true
}

// isOnlyPunctuation returns true if text contains only punctuation characters and whitespace
// Punctuation includes: " . ! ? , ; : ' and whitespace
func isOnlyPunctuation(text string) bool {
	if len(text) == 0 {
		return false
	}

	for _, ch := range text {
		if ch != '"' && ch != '.' && ch != '!' && ch != '?' && ch != ',' &&
			ch != ';' && ch != ':' && ch != '\'' && ch != ' ' && ch != '\t' {
			return false
		}
	}

	return true
}

// getStyledLineLength returns the total character count of all segments in a StyledLine
func getStyledLineLength(line *StyledLine) int {
	totalLength := 0
	for _, seg := range line.Segments {
		totalLength += utf8.RuneCountInString(seg.Text)
	}

	return totalLength
}

// findLastSpace finds the last space in text before maxLen
func findLastSpace(text string, maxLen int) int {
	if maxLen >= utf8.RuneCountInString(text) {
		return len(text)
	}

	lastSpace := 0
	runeCount := 0
	for byteIndex, char := range text {
		if runeCount >= maxLen {
			// A word that ends exactly at the limit fills the row.
			if char == ' ' {
				return byteIndex
			}

			break
		}
		if char == ' ' {
			lastSpace = byteIndex
		}
		runeCount++
	}

	return lastSpace
}

func byteIndexAfterRunes(text string, count int) int {
	if count <= 0 {
		return 0
	}
	runeCount := 0
	for byteIndex := range text {
		if runeCount == count {
			return byteIndex
		}
		runeCount++
	}

	return len(text)
}
