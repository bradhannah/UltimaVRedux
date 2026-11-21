package widgets

import (
	"image"
	"math"
	"strings"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

// ControlHint is one keyboard control shown as "[Key] Action". Keys use the
// shared spellings (Esc, Enter, Space, Tab, Shift+X, Ctrl+X, Up/Down,
// Left/Right, PgUp/PgDn, Home/End). An empty Key is plain guidance prose.
type ControlHint struct {
	Key    string
	Action string
}

// Shared key spellings and actions repeated across typed hints.
const (
	hintKeyEnter     = "Enter"
	hintKeyEsc       = "Esc"
	hintKeySpace     = "Space"
	hintKeyUpDown    = "Up/Down"
	hintKeyTabCycle  = "Tab/Shift+Tab"
	hintKeyShiftPage = "Shift+Up/Down"
	hintActionClose  = "Close"
	hintActionItem   = "Item"
	// Words for keys that mean the same thing everywhere: Space is more about
	// the selected row, Shift+Up/Down a page of the list, Tab the next group.
	hintActionDetails  = "Details"
	hintActionListPage = "List page"
	hintActionSection  = "Section"
	hintActionSpell    = "Spell"
	hintActionSearch   = "Search"
	hintActionSelect   = "Select"
)

// controlHintSeparator separates complete hints within one physical row. A
// packed footer tightens it only when that saves a row.
const controlHintSeparator = "   "

func (h ControlHint) String() string {
	switch {
	case h.Key == "":
		return h.Action
	case h.Action == "":
		return "[" + h.Key + "]"
	default:
		return "[" + h.Key + "] " + h.Action
	}
}

func joinControlHints(hints []ControlHint, separator string) string {
	parts := make([]string, len(hints))
	for index, hint := range hints {
		parts[index] = hint.String()
	}

	return strings.Join(parts, separator)
}

// ParseControlHints reads one row of bracketed "[Key] Action" guidance. Text
// outside a complete bracket pair stays prose, so malformed input is preserved.
func ParseControlHints(guidance string) []ControlHint {
	var hints []ControlHint
	for guidance = strings.TrimSpace(guidance); guidance != ""; {
		opening := strings.IndexByte(guidance, '[')
		closing := -1
		if opening >= 0 {
			closing = strings.IndexByte(guidance[opening:], ']')
		}
		if opening != 0 || closing < 0 {
			end := len(guidance)
			if opening > 0 && closing >= 0 {
				end = opening
			}
			hints = append(hints, ControlHint{Key: "", Action: strings.TrimSpace(guidance[:end])})
			guidance = strings.TrimSpace(guidance[end:])

			continue
		}
		rest := guidance[closing+1:]
		next := strings.IndexByte(rest, '[')
		if next < 0 {
			next = len(rest)
		}
		hints = append(hints, ControlHint{Key: guidance[1:closing], Action: strings.TrimSpace(rest[:next])})
		guidance = strings.TrimSpace(rest[next:])
	}

	return hints
}

// AppendControlHints styles one row of bracketed guidance on line.
func AppendControlHints(line *text.LineBuilder, guidance string) *text.LineBuilder {
	return appendControlHintRow(line, ParseControlHints(guidance), controlHintSeparator, nil)
}

func appendControlHintRow(
	line *text.LineBuilder, hints []ControlHint, separator string, dimmed func(ControlHint) bool,
) *text.LineBuilder {
	if len(hints) == 0 {
		return line.Text("", DialogControlHintColor) // Keep a blank guidance row.
	}
	for index, hint := range hints {
		if index > 0 {
			line.Text(separator, DialogControlHintColor)
		}
		if hint.Key == "" {
			line.Text(hint.Action, DialogControlHintColor)

			continue
		}
		if dimmed != nil && dimmed(hint) {
			line.Text(hint.String(), DialogUnavailableItemColor)

			continue
		}
		line.Text("["+hint.Key+"]", DialogControlKeyColor)
		if hint.Action != "" {
			line.Text(" "+hint.Action, DialogControlHintColor)
		}
	}

	return line
}

// PackControlHints appends hints to output as measured physical rows, wrapping
// only between complete hints so a key never separates from its action.
func PackControlHints(output *text.Output, hints []ControlHint, width int, align etext.Align) {
	PackControlHintsDimming(output, hints, width, align, nil)
}

// PackControlHintsDimming is PackControlHints with the hints for which dimmed
// returns true drawn whole in the unavailable colour: a key that cannot work
// now. The caller's text says why. A nil dimmed dims nothing.
func PackControlHintsDimming(
	output *text.Output, hints []ControlHint, width int, align etext.Align, dimmed func(ControlHint) bool,
) {
	separator, rows := controlHintSeparator, packControlHintRows(output.Font, hints, width, controlHintSeparator)
	for _, tighter := range [...]string{"  ", " "} {
		if candidate := packControlHintRows(output.Font, hints, width, tighter); len(candidate) < len(rows) {
			separator, rows = tighter, candidate
		}
	}
	for _, row := range rows {
		runes := utf8.RuneCountInString(joinControlHints(row, separator))
		output.SetMaxCharsPerLine(max(runes, output.Font.CharsForWidth(max(1, width))))
		appendControlHintRow(output.AppendLine().WithAlignment(align), row, separator, dimmed).Done()
	}
}

func packControlHintRows(font *text.UltimaFont, hints []ControlHint, width int, separator string) [][]ControlHint {
	var rows [][]ControlHint
	start := 0
	for end := 1; end <= len(hints); end++ {
		measured, _ := font.MeasureString(joinControlHints(hints[start:end], separator))
		if measured > float64(width) && end-start > 1 {
			rows = append(rows, hints[start:end-1])
			start = end - 1
		}
	}
	if start < len(hints) {
		rows = append(rows, hints[start:])
	}

	return rows
}

// staticViewerHint keeps return guidance outside the original artwork.
type staticViewerHint struct {
	output *text.Output
	width  int
}

const (
	staticViewerHintLines       = 4
	staticViewerHintChars       = 80
	staticViewerHintGapDivisor  = 2
	staticViewerHintLineSpacing = 1.2
)

func (h *staticViewerHint) draw(screen *ebiten.Image, artwork image.Rectangle, hints ...ControlHint) {
	width, _ := display.GetManager().GetCurrentSize()
	if h.output == nil || h.width != width {
		h.width = width
		_, h.output = GetScaledFontWithOutputForWidth(FontConfig{Size: FontSizeBody,
			MaxLines: staticViewerHintLines, MaxChars: staticViewerHintChars,
			LineSpacingMultiplier: staticViewerHintLineSpacing}, width)
	}
	h.output.Reset()
	PackControlHints(h.output, hints, artwork.Dx(), etext.AlignCenter)
	height := int(math.Ceil(h.output.GetTotalHeightForLines(h.output.GetPanelContentLineCount())))
	gap := max(1, height/staticViewerHintGapDivisor)
	top := artwork.Max.Y + gap
	if top+height > screen.Bounds().Max.Y {
		top = artwork.Min.Y - gap - height
	}
	if top < screen.Bounds().Min.Y {
		return // Never cover source art in unsupported full-screen placements.
	}
	vector.FillRect(screen, float32(artwork.Min.X), float32(top),
		float32(artwork.Dx()), float32(height), ucolor.EGABlack, false)
	h.output.DrawPanelContent(screen, image.Pt(artwork.Min.X, top), artwork.Dx(), etext.AlignCenter)
}
