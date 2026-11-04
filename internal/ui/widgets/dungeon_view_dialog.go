package widgets

import (
	"image"
	"math"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	utext "github.com/bradhannah/UltimaVRedux/internal/text"
	pkgcolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

const (
	dungeonViewMinimumTextSize  = 10
	dungeonViewMinimumTextSpace = 11
	// Measured exception: four-character axis tags must fit eight adjacent columns.
	dungeonViewEdgeFontSize StandardFontSize = 16
)

// DungeonViewDialog presents a detached current-floor observation with a matching key.
type DungeonViewDialog struct {
	model         game_state.DungeonViewModel
	plan          dungeonViewRenderPlan
	legend        []dungeonViewLegendEntry
	continuations []dungeonViewContinuation
	keyboard      *input.Keyboard
	placement     sprites.PercentBasedPlacement
	border        *Border
	buffer        *ebiten.Image
	drawOptions   *ebiten.DrawImageOptions
	bounds        image.Rectangle
	layout        dungeonViewLayout
	screenWidth   int
	screenHeight  int

	titleOutput  *utext.Output
	legendOutput *utext.Output
	edgeOutput   *utext.Output
	footerOutput *utext.Output

	dismissed         bool
	waitingForRelease bool
}

// DungeonViewDialogConfig contains immutable state for one symbolic dungeon view.
type DungeonViewDialogConfig struct {
	Placement sprites.PercentBasedPlacement
	Keyboard  *input.Keyboard
	Model     game_state.DungeonViewModel
}

// NewDungeonViewDialog creates a static north-up map with a revealed-feature key.
//
// REDUX DEVIATION: Original dungeon View draws IBM/runic glyphs across display-space
// copies (ref:VIEW-0003). Redux collapses the same revealed occurrences into an 8x8
// floor with solid walls, shared geometric artwork and plain paired wrap labels,
// prioritizing navigation clarity without changing the 22x22 flood or disclosure.
//
//nolint:exhaustruct_v5 // Rendering resources are initialized together by rebuildLayout.
func NewDungeonViewDialog(config DungeonViewDialogConfig) *DungeonViewDialog {
	plan := buildDungeonViewRenderPlan(config.Model)
	dialog := &DungeonViewDialog{
		model: config.Model, plan: plan,
		legend: dungeonViewLegendFor(plan, config.Model.Facing), continuations: dungeonViewContinuations(plan),
		keyboard: config.Keyboard, placement: config.Placement, waitingForRelease: true,
	}
	if dialog.keyboard != nil {
		dialog.keyboard.PrepareForTransition(0)
	}
	screenWidth, screenHeight := display.GetManager().GetCurrentSize()
	dialog.rebuildLayout(screenWidth, screenHeight)

	return dialog
}

func (d *DungeonViewDialog) rebuildLayout(screenWidth, screenHeight int) {
	if d.border != nil && screenWidth == d.screenWidth && screenHeight == d.screenHeight {
		return
	}
	d.screenWidth, d.screenHeight = screenWidth, screenHeight
	d.border = newDialogBorder(d.placement)
	d.bounds = d.border.GetBounds()
	bounds := localDungeonViewBounds(d.border)
	d.prepareOutputs(screenWidth, bounds.Dx())
	gap := dungeonViewLayoutGap(screenWidth, screenHeight)
	d.layout = calculateDungeonViewLayout(bounds, screenWidth, screenHeight, d.measurements(bounds.Dx(), gap))
	d.buffer = ebiten.NewImage(d.bounds.Dx(), d.bounds.Dy())
	d.drawOptions = &ebiten.DrawImageOptions{}
	d.drawOptions.GeoM.Translate(float64(d.bounds.Min.X), float64(d.bounds.Min.Y))
	d.renderToBuffer()
}

func newDungeonViewOutput(screenWidth int) *utext.Output {
	return newDungeonViewOutputSized(screenWidth, FontSizeCompact)
}

//nolint:exhaustruct_v5,mnd // A bounded text panel has room for wrapped headings and hints, even at compact widths.
func newDungeonViewOutputSized(screenWidth int, size StandardFontSize) *utext.Output {
	_, output := GetScaledFontWithOutputForWidth(FontConfig{
		Size: size, LineSpacingMultiplier: 1.1, MaxLines: 16, MaxChars: 120,
	}, screenWidth)
	if output.Font.TextFace.Size < dungeonViewMinimumTextSize {
		output.SetFont(utext.NewUltimaFont(dungeonViewMinimumTextSize), dungeonViewMinimumTextSpace)
	}

	return output
}

func localDungeonViewBounds(border *Border) image.Rectangle {
	return border.GetInteriorBounds().Sub(border.GetBounds().Min)
}

func (d *DungeonViewDialog) prepareOutputs(screenWidth, width int) {
	d.titleOutput = newDungeonViewOutputSized(screenWidth, FontSizeTitle)
	d.legendOutput = newDungeonViewOutput(screenWidth)
	// Ordinary edge labels have their own measured typography. Four-character
	// axis tags must fit eight adjacent columns without borrowing neighboring cells.
	d.edgeOutput = newDungeonViewOutputSized(screenWidth, dungeonViewEdgeFontSize)
	d.footerOutput = newDungeonViewOutput(screenWidth)
	d.titleOutput.AppendLine().Yellow(strings.ToUpper(d.model.Location.String())).
		LightGray("   LEVEL ").Whitef("%d OF %d", int(d.model.Floor)+1, references.DungeonFloorCount).
		LightCyan("   NORTH UP").Done()
	d.titleOutput = d.titleOutput.WrappedCopyForWidth(width)
	if len(d.continuations) > 0 {
		d.footerOutput.AppendLine().LightCyan("Matching W-E / N-S labels join opposite edges on this level.").Done()
	}
	d.footerOutput.AppendLine().LightGray("Current view only. Visible does not mean safe or reachable.").Done()
	AppendControlHints(d.footerOutput.AppendLine(), "[Any key] Close").Done()
	d.footerOutput = d.footerOutput.WrappedCopyForWidth(width)
}

func (d *DungeonViewDialog) measurements(width, gap int) dungeonViewMeasurements {
	labelWidth := 0.0
	compactHeight := d.legendOutput.GetFontHeight()
	compactWidth := max(1, (width+gap)/2-dungeonViewCellSize-gap*2)
	for _, entry := range d.legend {
		width, _ := d.legendOutput.Font.MeasureString(entry.label)
		labelWidth = max(labelWidth, width)
		d.legendOutput.Reset()
		d.legendOutput.AppendLine().LightGray(entry.label).Done()
		wrapped := d.legendOutput.WrappedCopyForWidth(compactWidth)
		compactHeight = max(compactHeight, wrapped.GetTotalHeightForLines(wrapped.GetPanelContentLineCount()))
	}
	edgeWidth := 0.0
	horizontalWrap, verticalWrap := false, false
	for _, mark := range d.continuations {
		width, _ := d.edgeOutput.Font.MeasureString(mark.label)
		edgeWidth = max(edgeWidth, width)
		horizontalWrap = horizontalWrap || mark.axis == dungeonViewAxisHorizontal
		verticalWrap = verticalWrap || mark.axis == dungeonViewAxisVertical
	}

	return dungeonViewMeasurements{
		headerHeight: int(math.Ceil(d.titleOutput.GetTotalHeightForLines(d.titleOutput.GetPanelContentLineCount()))),
		footerHeight: int(math.Ceil(d.footerOutput.GetTotalHeightForLines(d.footerOutput.GetPanelContentLineCount()))),
		textHeight:   int(math.Ceil(d.legendOutput.GetFontHeight())), compactLabelHeight: int(math.Ceil(compactHeight)),
		labelWidth: int(math.Ceil(labelWidth)), edgeWidth: int(math.Ceil(edgeWidth)), entryCount: len(d.legend),
		legendScale:    max(1, int(math.Ceil(d.legendOutput.Font.TextFace.Size/dungeonViewCellSize))),
		edgeHeight:     int(math.Ceil(d.edgeOutput.GetFontHeight())),
		horizontalWrap: horizontalWrap, verticalWrap: verticalWrap,
	}
}

func (d *DungeonViewDialog) renderToBuffer() {
	FillClassicPixelRoundedRect(d.buffer, d.buffer.Bounds(),
		getClassicUIPixelUnit(d.screenWidth, d.screenHeight), pkgcolor.UIBackground.RGBA())
	d.titleOutput.DrawPanelContent(d.buffer, d.layout.header.Min, d.layout.header.Dx(), text.AlignCenter)
	drawDungeonViewRaster(d.buffer, dungeonViewMapImage(d.plan, d.model.Facing),
		d.layout.mapGrid.Min, d.layout.mapScale)
	d.renderContinuations()
	d.renderLegend()
	d.footerOutput.DrawPanelContent(d.buffer, d.layout.footer.Min, d.layout.footer.Dx(), text.AlignCenter)
}

func drawDungeonViewRaster(dst *ebiten.Image, raster *image.RGBA, at image.Point, scale int) {
	texture := ebiten.NewImageFromImage(raster)
	op := &ebiten.DrawImageOptions{Filter: ebiten.FilterNearest}
	op.GeoM.Scale(float64(scale), float64(scale))
	op.GeoM.Translate(float64(at.X), float64(at.Y))
	dst.DrawImage(texture, op)
}

func (d *DungeonViewDialog) renderContinuations() {
	for _, mark := range d.continuations {
		d.edgeOutput.Reset()
		d.edgeOutput.AppendLine().LightCyan(mark.label).Done()
		for _, bounds := range d.layout.continuationBounds(mark) {
			d.edgeOutput.DrawPanelContent(d.buffer, bounds.Min, bounds.Dx(), text.AlignCenter)
		}
	}
}

func (d *DungeonViewDialog) renderLegend() {
	d.legendOutput.Reset()
	d.legendOutput.AppendLine().LightCyan("KEY").Done()
	d.legendOutput.DrawPanelContent(d.buffer, d.layout.legend.Min, d.layout.legend.Dx(), text.AlignStart)
	iconSize := dungeonViewCellSize * d.layout.legendScale
	for index, entry := range d.legend {
		row := d.layout.legendRow(index)
		origin := image.Pt(row.Min.X, row.Min.Y+(row.Dy()-iconSize)/2)
		drawDungeonViewRaster(d.buffer, dungeonViewSymbolImage(entry.plan, d.model.Facing), origin, d.layout.legendScale)
		d.legendOutput.Reset()
		if entry.plan.player {
			d.legendOutput.AppendLine().LightGray("You: facing ").
				LightGreen(strings.ToLower(d.model.Facing.Direction().GetDirectionCompassName())).Done()
		} else {
			d.legendOutput.AppendLine().LightGray(entry.label).Done()
		}
		textX := row.Min.X + iconSize + d.layout.gap
		label := d.legendOutput.WrappedCopyForWidth(max(1, row.Max.X-textX))
		height := int(math.Ceil(label.GetTotalHeightForLines(label.GetPanelContentLineCount())))
		label.DrawPanelContent(d.buffer, image.Pt(textX, row.Min.Y+(row.Dy()-height)/2),
			row.Max.X-textX, text.AlignStart)
	}
}

// Draw implements Widget.
func (d *DungeonViewDialog) Draw(screen *ebiten.Image) {
	d.rebuildLayout(screen.Bounds().Dx(), screen.Bounds().Dy())
	screen.DrawImage(d.buffer, d.drawOptions)
	DrawClassicPixelRoundedFrame(screen, d.bounds,
		getClassicUIPixelUnit(d.screenWidth, d.screenHeight), DialogRuleColor)
}

// Update dismisses on one fresh Enter press after the invoking key has been released.
func (d *DungeonViewDialog) Update() {
	if d.keyboard == nil {
		return
	}
	if d.waitingForRelease {
		if d.keyboard.IsWaitingForRelease() {
			return
		}
		d.waitingForRelease = false
	}
	// Any key closes it, as the surface Gem view does.
	if d.keyboard.IsAnyKeyPressed() {
		d.keyboard.ResetReleaseDebouncing()
		d.dismissed = true
	}
}

// IsDismissed reports whether the modal may be removed.
func (d *DungeonViewDialog) IsDismissed() bool {
	return d.dismissed
}

// Dismiss allows workflow cleanup to close the dialog safely.
func (d *DungeonViewDialog) Dismiss() {
	d.dismissed = true
}
