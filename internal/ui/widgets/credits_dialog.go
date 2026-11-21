package widgets

import (
	"image"
	"math"
	"slices"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	creditsMargin                    = 0.06
	creditsPadding                   = 12
	creditsLineSpacing               = 1.3
	creditsOpeningPauseMs      int64 = 2000
	creditsMillisecondsPerLine       = 2000
	creditsFooterRows                = 12
	creditsParagraphRows             = 3
	creditsEdgeRows                  = 2
	creditsSides                     = 2
)

type creditParagraphLayout struct {
	output      *text.Output
	start, rows int
}

// CreditsDialog is a title-only reader. Its injected presentation clock is
// advanced by the intro scene, never by Draw and never by a gameplay session.
type CreditsDialog struct {
	navigation
	document                              []text.CreditParagraph
	keyboard                              *input.Keyboard
	now                                   func() int64
	onClose                               func()
	border                                *Border
	header, footer, metrics               *text.Output
	paragraphs                            []creditParagraphLayout
	canvas                                *ebiten.Image
	width, height, totalRows, cachedFirst int
	offset, lastOffset                    float64
	lastMs, openingPauseMs                int64
	paused, closed                        bool
	footerState                           string
}

// NewCreditsDialog snapshots the complete document once. Reflow only changes
// its layout; it never re-enumerates contributors or audio provenance.
func NewCreditsDialog(
	document []text.CreditParagraph, keyboard *input.Keyboard, now func() int64, onClose func(),
) *CreditsDialog {
	if keyboard == nil || now == nil || onClose == nil {
		panic("credits require a keyboard, presentation clock and close callback")
	}
	dialog := &CreditsDialog{
		navigation: newNavigation(keyboard.IsKeyHeld), document: slices.Clone(document),
		keyboard: keyboard, now: now, onClose: onClose,
		lastMs: now(), openingPauseMs: creditsOpeningPauseMs,
		border: nil, header: nil, footer: nil, metrics: nil, paragraphs: nil, canvas: nil,
		width: 0, height: 0, totalRows: 0, cachedFirst: -1, offset: 0, lastOffset: 0,
		paused: false, closed: false, footerState: "",
	}
	dialog.ensureLayout()
	keyboard.PrepareForTransition(KeyboardTransitionDelayMs)

	return dialog
}

func (d *CreditsDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	if d.metrics != nil && width == d.width && height == d.height {
		return
	}
	anchor, fraction := d.readingAnchor()
	wasComplete := d.metrics != nil && d.complete()
	d.width, d.height = width, height
	d.border = newDialogBorder(sprites.PercentBasedPlacement{
		StartPercentX: creditsMargin, EndPercentX: 1 - creditsMargin,
		StartPercentY: creditsMargin, EndPercentY: 1 - creditsMargin,
	})
	font, metrics := GetScaledFontWithOutput(FontConfig{
		Size: FontSizeBody, MaxLines: 1, MaxChars: 1, LineSpacingMultiplier: creditsLineSpacing,
	})
	d.metrics = metrics
	spacing := metrics.GetLineSpacing()
	contentWidth := d.border.GetInteriorBounds().Dx() - creditsSides*creditsPadding
	d.header = text.NewOutput(GetScaledFont(FontSizeTitle), GetScaledLineHeight(FontSizeTitle)*creditsLineSpacing,
		1, len("ACKNOWLEDGMENTS"))
	d.header.AppendLine().Text("ACKNOWLEDGMENTS", DialogHeaderColor).Done()
	d.header = d.header.WrappedCopyForWidth(contentWidth)
	d.footer = text.NewOutput(font, spacing, creditsFooterRows, max(1, font.CharsForWidth(contentWidth)))
	d.footerState = ""
	d.updateFooter()
	d.paragraphs = make([]creditParagraphLayout, 0, len(d.document))
	d.totalRows = 0
	for _, paragraph := range d.document {
		rows := creditsParagraphRows + strings.Count(paragraph.Heading+paragraph.Label+paragraph.Value, "\n")
		output := text.NewOutput(font, spacing, rows,
			1+len(paragraph.Heading)+len(paragraph.Label)+len(paragraph.Value))
		if paragraph.Heading != "" {
			output.AppendLine().Text("- - -", DialogRuleColor).Done()
			output.AppendLine().Text(paragraph.Heading, DialogHeaderColor).Done()
		} else {
			if paragraph.Label != "" {
				output.AppendLine().Text(paragraph.Label, DialogLabelColor).Done()
			}
			output.AppendLine().Text(paragraph.Value, DialogValueColor).Done()
		}
		output = output.WrappedCopyForWidth(contentWidth)
		rows = max(1, output.GetPanelContentLineCount())
		d.paragraphs = append(d.paragraphs, creditParagraphLayout{output: output, start: d.totalRows, rows: rows})
		d.totalRows += rows
	}
	d.lastOffset = max(0, (metrics.GetTotalHeightForLines(d.totalRows)-float64(d.contentBounds().Dy()))/spacing)
	if anchor < len(d.paragraphs) {
		paragraph := d.paragraphs[anchor]
		d.offset = min(d.lastOffset, float64(paragraph.start)+fraction*float64(paragraph.rows))
	}
	if wasComplete {
		d.offset = d.lastOffset
	}
	if d.canvas != nil {
		d.canvas.Deallocate()
	}
	d.canvas = ebiten.NewImage(contentWidth, d.contentBounds().Dy()+int(math.Ceil(creditsEdgeRows*spacing)))
	d.cachedFirst = -1
	d.updateFooter()
}

func (d *CreditsDialog) readingAnchor() (int, float64) {
	for index, paragraph := range d.paragraphs {
		if d.offset < float64(paragraph.start+paragraph.rows) {
			return index, (d.offset - float64(paragraph.start)) / float64(paragraph.rows)
		}
	}

	return 0, 0
}

func (d *CreditsDialog) contentBounds() image.Rectangle {
	bounds := d.border.GetInteriorBounds().Inset(creditsPadding)
	bounds.Min.Y += int(math.Ceil(d.header.GetTotalHeightForLines(d.header.GetPanelContentLineCount()))) + creditsPadding
	bounds.Max.Y -= int(math.Ceil(d.footer.GetTotalHeightForLines(d.footer.GetPanelContentLineCount()))) + creditsPadding

	return bounds
}

func (d *CreditsDialog) complete() bool { return d.offset >= d.lastOffset }

func (d *CreditsDialog) stateLabel() string {
	if d.complete() {
		return "Complete"
	}
	if d.paused {
		return "Paused"
	}

	return "Scrolling"
}

func (d *CreditsDialog) updateFooter() {
	state := d.stateLabel()
	if state == d.footerState {
		return
	}
	d.footerState = state
	d.footer.Reset()
	d.footer.AppendLine().Text("State: ", DialogLabelColor).Text(state, DialogValueColor).Done()
	AppendControlHints(d.footer.AppendLine(), "[Space] Pause/Resume   [Up/Down] Scroll   "+
		"[Shift+Up/Down/PgUp/PgDn] Page   [Home/End] First/Last   [Esc] Close").WordWrap().Done()
}

// Update advances only presentation and owns all credits input while modal.
func (d *CreditsDialog) Update() {
	if d.closed {
		return
	}
	d.ensureLayout()
	now := d.now()
	delta := max(0, now-d.lastMs)
	d.lastMs = now
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.closed = true
		d.canvas.Deallocate()
		d.keyboard.PrepareForTransition(KeyboardTransitionDelayMs)
		d.onClose()

		return
	}
	key, owned := d.Poll(d.keyboard,
		ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyPageUp, ebiten.KeyPageDown, ebiten.KeyHome, ebiten.KeyEnd)
	switch {
	case key != nil:
		d.paused = true
		switch *key { //nolint:exhaustive // Only navigation keys reach this switch.
		case ebiten.KeyHome:
			d.offset = 0
		case ebiten.KeyEnd:
			d.offset = d.lastOffset
		default:
			page := max(1, d.metrics.GetLineCapacity(d.contentBounds().Dy()))
			d.offset = max(0, min(d.lastOffset, d.offset+float64(listNavigationDelta(*key, page))))
		}
	case !owned && d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace):
		if !d.complete() {
			d.paused = !d.paused
		}
	case !d.paused && !d.complete():
		pause := min(delta, d.openingPauseMs)
		d.openingPauseMs -= pause
		d.offset = min(d.lastOffset, d.offset+float64(delta-pause)/creditsMillisecondsPerLine)
	}
	d.updateFooter()
}

// Draw reuses a body-sized canvas, rendering only visible rows when its first
// row changes. Fractional translation is clipped separately from the fixed chrome.
func (d *CreditsDialog) Draw(screen *ebiten.Image) {
	if d.closed {
		return
	}
	d.ensureLayout()
	pixelUnit := getClassicUIPixelUnit(d.width, d.height)
	FillClassicPixelRoundedRect(screen, d.border.GetBounds(), pixelUnit, ucolor.DialogBackground.RGBA())
	DrawClassicPixelRoundedFrame(screen, d.border.GetBounds(), pixelUnit, DialogRuleColor)
	bounds := d.border.GetInteriorBounds().Inset(creditsPadding)
	d.header.DrawPanelContent(screen, bounds.Min, bounds.Dx(), etext.AlignCenter)
	body := d.contentBounds()
	first := int(math.Floor(d.offset))
	if first != d.cachedFirst {
		d.canvas.Clear()
		end := first + d.metrics.GetLineCapacity(d.canvas.Bounds().Dy())
		for _, paragraph := range d.paragraphs {
			start, last := max(first, paragraph.start), min(end, paragraph.start+paragraph.rows)
			if start >= last {
				continue
			}
			paragraph.output.DrawPanelContentRange(d.canvas,
				image.Pt(0, int(float64(start-first)*d.metrics.GetLineSpacing())), body.Dx(), etext.AlignStart,
				start-paragraph.start, last-start)
		}
		d.cachedFirst = first
	}
	if clip := body.Intersect(screen.Bounds()); !clip.Empty() {
		clipped, ok := screen.SubImage(clip).(*ebiten.Image)
		if !ok {
			panic("credits body clip must be an Ebitengine image")
		}
		options := &ebiten.DrawImageOptions{Filter: ebiten.FilterLinear}
		options.GeoM.Translate(float64(body.Min.X), float64(body.Min.Y)-(d.offset-float64(first))*d.metrics.GetLineSpacing())
		clipped.DrawImage(d.canvas, options)
	}
	footerY := bounds.Max.Y - int(math.Ceil(d.footer.GetTotalHeightForLines(d.footer.GetPanelContentLineCount())))
	d.footer.DrawPanelContent(screen, image.Pt(bounds.Min.X, footerY), bounds.Dx(), etext.AlignStart)
}
