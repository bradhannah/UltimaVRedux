package widgets

import (
	"image"
	stdcolor "image/color"
	"strings"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

// A search result is its title line and the one line of it that matched.
const journalSearchResultRows = 2

func journalMatchMask(value, query string) []bool {
	runes := []rune(value)
	mask := make([]bool, len(runes))
	lower := strings.ToLower(value)
	for _, word := range strings.Fields(strings.ToLower(query)) {
		for offset := 0; offset < len(lower); {
			index := strings.Index(lower[offset:], word)
			if index < 0 {
				break
			}
			start := utf8.RuneCountInString(lower[:offset+index])
			end := min(len(mask), start+utf8.RuneCountInString(word))
			for position := start; position < end; position++ {
				mask[position] = true
			}
			offset += index + len(word)
		}
	}

	return mask
}

func journalMatchColour(selected bool) stdcolor.Color {
	if selected {
		return ucolor.EGABlue
	}

	return ucolor.EGALightCyan
}

func appendJournalMatches(line *text.LineBuilder, value, query string, foreground stdcolor.Color, selected bool) {
	for _, segment := range journalMatchSegments(value, query, foreground, selected) {
		line.Text(segment.Text, segment.Color)
	}
}

func journalMatchSegments(value, query string, foreground stdcolor.Color, selected bool) []text.ColorSegment {
	if query == "" || value == "" {
		return []text.ColorSegment{{Text: value, Color: foreground,
			FontType: text.FontTypeNormal, FontScale: text.FontScaleNormal}}
	}
	runes, mask := []rune(value), journalMatchMask(value, query)
	var segments []text.ColorSegment
	for start := 0; start < len(runes); {
		end := start + 1
		for end < len(runes) && mask[start] == mask[end] {
			end++
		}
		colour := foreground
		if mask[start] {
			colour = journalMatchColour(selected)
		}
		segments = append(segments, text.ColorSegment{
			Text: string(runes[start:end]), Color: colour, FontType: text.FontTypeNormal, FontScale: text.FontScaleNormal,
		})
		start = end
	}

	return segments
}

// journalSearchSnippet fits value to capacity letters. When the value is longer
// the window starts near the first match so the match stays in view; the cut is
// plain, with no dots.
func journalSearchSnippet(value, query string, capacity int) string {
	value = strings.Join(strings.Fields(value), " ")
	runes := []rune(value)
	if len(runes) <= capacity {
		return value
	}
	const contextDivisor = 4
	capacity = max(1, capacity)
	start := 0
	for index, matched := range journalMatchMask(value, query) {
		if matched {
			start = max(0, min(index-capacity/contextDivisor, len(runes)-capacity))
			for start > 0 && start < index && runes[start-1] != ' ' {
				start++ // Begin on a whole word.
			}

			break
		}
	}

	return string(runes[start:min(len(runes), start+capacity)])
}

// journalSearchLine picks the first labelled line that matches the query, else
// the first line, and returns its label with its value fitted to capacity.
func journalSearchLine(lines []questJournalSearchLine, query string, capacity int) (string, string) {
	var chosen questJournalSearchLine
	for _, line := range lines {
		if strings.TrimSpace(line.value) == "" {
			continue
		}
		if chosen.value == "" {
			chosen = line
		}
		if journalHasMatch(line.label+" "+line.value, query) {
			chosen = line

			break
		}
	}
	if chosen.label != "" {
		capacity -= utf8.RuneCountInString(chosen.label) + len(journalLabelSeparator)
	}

	return chosen.label, journalSearchSnippet(chosen.value, query, capacity)
}

const (
	journalSnippetIndentText = "  "
	journalLabelSeparator    = ": "
	journalLabelPlace        = "Place"
	journalLabelSection      = "Section"
)

// snippetText is the matched line as shown, with its label.
func (e questJournalSearchEntry) snippetText() string {
	if e.label == "" {
		return e.snippet
	}

	return e.label + journalLabelSeparator + e.snippet
}

// exchangeSearchLines lists what a conversation entry can match on: the words
// spoken, then where they were heard.
func exchangeSearchLines(exchange journal.Exchange) []questJournalSearchLine {
	return []questJournalSearchLine{
		{label: "", value: journalSnippetSource(exchange)}, {label: journalLabelPlace, value: exchange.Location},
	}
}

func journalSnippetSource(exchange journal.Exchange) string {
	var body strings.Builder
	for _, passage := range exchange.Passages {
		if passage.Runic {
			body.WriteString(" ... ")
		} else {
			body.WriteString(passage.Text)
		}
	}

	return body.String()
}

func (d *QuestJournalDialog) drawJournalMatchLine(screen *ebiten.Image, rect image.Rectangle,
	value, query string, foreground stdcolor.Color, selected bool) {
	value = CutDialogLine(d.font, value, "", rect.Dx())
	d.rowOutput.Reset()
	d.rowOutput.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(value)))
	line := d.rowOutput.AppendLine()
	appendJournalMatches(line, value, query, foreground, selected)
	line.Done()
	d.rowOutput.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignStart)
}

func (d *QuestJournalDialog) revealSearchMatch() {
	if d.isConversationTab() {
		d.prepareExchangeReader()
		start, end := d.selectedExchangeSpan()
		if row, found := d.exchangeOutput.FirstPanelLineWithColor(start, end,
			journalMatchColour(d.activeTab == QuestJournalConversationTab)); found {
			capacity := max(1, d.exchangeOutput.GetLineCapacity(d.exchangeContentRect().Dy()))
			d.detailPage = (row - start) / capacity
			d.prepareExchangeReader()
		}

		return
	}
	for page, lines := range d.detailPages() {
		for _, line := range lines {
			value := line.value
			if d.activeTab == QuestJournalMoonstonesTab {
				if moonstonePlaceholder(value) {
					continue
				}
				value = line.label + ": " + value
			}
			if line.value != questJournalNotLearned && journalHasMatch(value, d.readingQuery) {
				d.detailPage = page

				return
			}
		}
	}
}

func journalHasMatch(value, query string) bool {
	for _, word := range strings.Fields(strings.ToLower(query)) {
		if strings.Contains(strings.ToLower(value), word) {
			return true
		}
	}

	return false
}
