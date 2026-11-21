package widgets

import (
	"fmt"
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
	// The window keeps this many layout gaps clear around what it shows.
	dungeonViewWindowMargin = 2
	dungeonViewBothSides    = 2
	dungeonViewPixelCentre  = 0.5
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
	textWidth     int // The widest heading or footer line, which the window must hold.

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

// NewDungeonViewDialog creates a static north-up map with a revealed-feature key,
// in a window only as large as that needs. Placement is the most room it may take.
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
	d.layOut(screenWidth, screenHeight)
	d.buffer = ebiten.NewImage(d.bounds.Dx(), d.bounds.Dy())
	d.drawOptions = &ebiten.DrawImageOptions{}
	d.drawOptions.GeoM.Translate(float64(d.bounds.Min.X), float64(d.bounds.Min.Y))
	d.renderToBuffer()
}

// layOut sizes the window to what it shows. The view is first laid out in all
// the room the placement allows, which settles the map's scale. The window is
// then only as large as that needs, so the game stays in view around it.
func (d *DungeonViewDialog) layOut(screenWidth, screenHeight int) {
	d.screenWidth, d.screenHeight = screenWidth, screenHeight
	gap := dungeonViewLayoutGap(screenWidth, screenHeight)
	room := newDialogBorder(d.placement)
	d.border = newDialogBorder(d.fittedPlacement(room, d.layoutWithin(room, gap), gap))
	d.bounds = d.border.GetBounds()
	d.layout = d.layoutWithin(d.border, gap)
}

func (d *DungeonViewDialog) layoutWithin(border *Border, gap int) dungeonViewLayout {
	bounds := localDungeonViewBounds(border)
	d.prepareOutputs(d.screenWidth, bounds.Dx())

	return calculateDungeonViewLayout(bounds, d.screenWidth, d.screenHeight, d.measurements(bounds.Dx(), gap))
}

// fittedPlacement shrinks the room to the laid-out view and a margin, centred
// where the room was. A border's interior is a fixed share of its size, so a
// window fitted this way always has at least the room the view was laid out in.
func (d *DungeonViewDialog) fittedPlacement(
	room *Border, layout dungeonViewLayout, gap int,
) sprites.PercentBasedPlacement {
	outer, inner := room.GetBounds(), localDungeonViewBounds(room)
	margin := gap * dungeonViewWindowMargin
	used := layout.used()
	width := max(used.Dx(), d.textWidth) + margin*dungeonViewBothSides
	height := layout.header.Dy() + used.Dy() + layout.footer.Dy() + (gap+margin)*dungeonViewBothSides
	spare := image.Pt(max(0, inner.Dx()-width), max(0, inner.Dy()-height)).Div(dungeonViewBothSides)
	// Aiming at a pixel's centre keeps each edge on that pixel when the border scales it back.
	share := func(pixel, extent int) float64 { return (float64(pixel) + dungeonViewPixelCentre) / float64(extent) }

	return sprites.PercentBasedPlacement{
		StartPercentX: share(outer.Min.X+spare.X, d.screenWidth), EndPercentX: share(outer.Max.X-spare.X, d.screenWidth),
		StartPercentY: share(outer.Min.Y+spare.Y, d.screenHeight), EndPercentY: share(outer.Max.Y-spare.Y, d.screenHeight),
	}
}

func newDungeonViewOutput(screenWidth int) *utext.Output {
	return newDungeonViewOutputSized(screenWidth, FontSizeCompact)
}

//nolint:mnd // A bounded text panel has room for wrapped headings and hints, even at compact widths.
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
	const wrapNote, closeHint = "Matching W-E / N-S labels join opposite edges on this level.", "[Any key] Close"
	name := strings.ToUpper(d.model.Location.String())
	level := fmt.Sprintf("%d OF %d", int(d.model.Floor)+1, references.DungeonFloorCount)
	d.titleOutput.AppendLine().Yellow(name).LightGray("   LEVEL ").White(level).Done()
	titleWidth, _ := d.titleOutput.Font.MeasureString(name + "   LEVEL " + level)
	footerWidth, _ := d.footerOutput.Font.MeasureString(closeHint)
	d.titleOutput = d.titleOutput.WrappedCopyForWidth(width)
	if len(d.continuations) > 0 {
		d.footerOutput.AppendLine().LightCyan(wrapNote).Done()
		footerWidth, _ = d.footerOutput.Font.MeasureString(wrapNote)
	}
	AppendControlHints(d.footerOutput.AppendLine(), closeHint).Done()
	d.footerOutput = d.footerOutput.WrappedCopyForWidth(width)
	d.textWidth = int(math.Ceil(max(titleWidth, footerWidth)))
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
	d.edgeOutput.Reset()
	d.edgeOutput.AppendLine().LightCyan("N").Done()
	d.edgeOutput.DrawPanelContent(d.buffer, d.layout.north.Min, d.layout.north.Dx(), text.AlignCenter)
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
	iconSize := dungeonViewCellSize * d.layout.legendScale
	for index, entry := range d.legend {
		row := d.layout.legendRow(index)
		textX := row.Min.X
		d.legendOutput.Reset()
		line := d.legendOutput.AppendLine()
		switch {
		case entry.heading:
			line.LightCyan(entry.label).Done()
		case entry.plan.player:
			line.LightGray("You: facing ").
				LightGreen(strings.ToLower(d.model.Facing.Direction().GetDirectionCompassName())).Done()
		default:
			line.LightGray(entry.label).Done()
		}
		if !entry.heading {
			origin := image.Pt(row.Min.X, row.Min.Y+(row.Dy()-iconSize)/2)
			drawDungeonViewRaster(d.buffer, dungeonViewSymbolImage(entry.plan, d.model.Facing), origin, d.layout.legendScale)
			textX += iconSize + d.layout.gap
		}
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

// GetBounds returns the window's rectangle on screen, so the scene can tell what
// the fitted window covers and what stays clear around it.
func (d *DungeonViewDialog) GetBounds() image.Rectangle {
	return d.bounds
}

// IsDismissed reports whether the modal may be removed.
func (d *DungeonViewDialog) IsDismissed() bool {
	return d.dismissed
}

// Dismiss allows workflow cleanup to close the dialog safely.
func (d *DungeonViewDialog) Dismiss() {
	d.dismissed = true
}
