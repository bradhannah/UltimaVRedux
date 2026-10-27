package text

import (
	"image"
	"math"

	beradtext "github.com/bradhannah/berad-sdk/modules/berad-text"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/pkg/color"
)

// IntroChoice is one title-menu row. A choice with Children opens a sub-menu
// instead of acting on its own; a Muted choice rests in light grey so the
// play actions above it read first.
type IntroChoice struct {
	Label    string
	Children []string
	Muted    bool
}

// IntroChoices defines the adapted Redux title-menu tree.
//
// REDUX DEVIATION: The original menu exposes six choices (ref:INTRO-0001). Redux
// shows seven top-level rows and keeps its fresh-game and setup additions one
// level down in the New Game and Configure sub-menus.
//
//nolint:gochecknoglobals // Immutable menu configuration shared by rendering and dispatch.
var IntroChoices = []IntroChoice{
	{Label: "Journey Onward", Children: nil, Muted: false},
	{Label: "Select Save Game", Children: nil, Muted: false},
	{Label: "New Game", Children: []string{"Create New Character", "Import from Legacy"}, Muted: false},
	{Label: "Configure", Children: []string{"Display", "Audio", "Set Data Directory"}, Muted: false},
	{Label: "Ultima V Introduction", Children: nil, Muted: false},
	{Label: "Acknowledgments", Children: nil, Muted: true},
	{Label: "Return to the View", Children: nil, Muted: true},
}

// IntroMenuState describes what the title menu currently shows.
type IntroMenuState struct {
	// Selection is the top-level row under the cursor, or the open parent.
	Selection int
	// Enabled mirrors IntroChoices; a missing entry counts as enabled.
	Enabled []bool
	// SubmenuOpen moves the cursor into Selection's children.
	SubmenuOpen bool
	// SubmenuSelection is the child row under the cursor while SubmenuOpen.
	SubmenuSelection int
}

// Intro menu layout constants
const (
	introMenuTextStartY      = 0.63
	introMenuTextEndY        = 0.96
	introMenuFrameStartX     = 0.02
	introMenuFrameEndX       = 0.98
	introMenuFrameStartY     = 0.595
	introMenuFrameEndY       = 0.995
	introMenuFrameSafeInsetY = 0.032
	introMenuInteriorStartX  = 0.06
	introBaseFontHeight      = 0.035
	introTextCoverage        = 0.75
	introFontReferenceHeight = 0.30
	introFontReferenceRows   = 8
	introMenuMaxChoices      = 9
	introFontMetricScale     = 0.85
	introLogicalWidth        = 320
	introLogicalHeight       = 200
	// introMarkerWidthUnits and introMarkerHeightUnits size the cursor
	// triangles in classic pixels; the parent chevron is narrower.
	introMarkerWidthUnits  = 6
	introMarkerHeightUnits = 7
	introChevronWidthUnits = 4
	introMarkerGapUnits    = 3
	// IntroSubmenuBorderIdealWidth scales the sub-menu panel border to half the
	// title frame's band so the panel reads as a child of the frame.
	IntroSubmenuBorderIdealWidth    = 800
	introSubmenuBorderReferenceBand = 6
	introSubmenuPanelEndX           = 0.955
	introSubmenuPanelGapUnits       = 4
	introSubmenuInsetUnits          = 2
	introSubmenuFontScale           = 0.9
	introSubmenuHintScale           = 0.65
	introSubmenuRowSpacing          = 1.6
	introSubmenuHintGapUnits        = 2
	introSubmenuHint                = "Esc Back"
)

// introMarkerRows lists each classic-pixel row width of the cursor triangle.
//
//nolint:gochecknoglobals // Immutable marker shape.
var introMarkerRows = [introMarkerHeightUnits]int{2, 4, 5, 6, 5, 4, 2}

// introChevronRows lists each row width of the parent chevron.
//
//nolint:gochecknoglobals // Immutable marker shape.
var introChevronRows = [introMarkerHeightUnits]int{1, 2, 3, 4, 3, 2, 1}

// IntroMenuFramePlacement returns the title-menu frame bounds.
func IntroMenuFramePlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: introMenuFrameStartX,
		EndPercentX:   introMenuFrameEndX,
		StartPercentY: introMenuFrameStartY,
		EndPercentY:   introMenuFrameEndY,
	}
}

// GetIntroFontSize calculates the font point size needed to fill the target height
// at 1:1 scale (no geometric stretching). This ensures crisp text rendering.
func GetIntroFontSize() float64 {
	dm := display.GetManager()
	_, screenHeight := dm.GetCurrentSize()

	targetPixelHeight := float64(screenHeight) * introMenuFontHeightPercent(len(IntroChoices))

	// Font point size roughly equals pixel height for this font
	// Slight adjustment factor based on font metrics (ascent + descent vs point size)
	return targetPixelHeight * introFontMetricScale
}

// GetIntroNoticeFontSize sizes the notice line between the flames and the
// frame. It keeps the nine-row face so two lines still fit above the frame at
// 320x200 even though the seven-row menu itself draws larger.
func GetIntroNoticeFontSize() float64 {
	_, screenHeight := display.GetManager().GetCurrentSize()

	return float64(screenHeight) * introMenuFontHeightPercent(introMenuMaxChoices) * introFontMetricScale
}

func introMenuFontHeightPercent(choiceCount int) float64 {
	if choiceCount <= 0 {
		return introBaseFontHeight
	}
	if choiceCount > introMenuMaxChoices {
		panic("intro menu exceeds its nine-choice layout capacity")
	}
	fontRows := min(choiceCount, introFontReferenceRows)

	return min(introBaseFontHeight, introFontReferenceHeight*introTextCoverage/float64(fontRows))
}

func introMenuChoicePositions(screenHeight int, fontHeight float64, choiceCount int) []float64 {
	if choiceCount <= 0 {
		return nil
	}
	menuTop := float64(screenHeight) * introMenuTextStartY
	menuBottom := float64(screenHeight) * introMenuTextEndY
	choiceStep := 0.0
	if choiceCount > 1 {
		choiceStep = (menuBottom - menuTop - fontHeight) / float64(choiceCount-1)
	}
	positions := make([]float64, choiceCount)
	for index := range positions {
		positions[index] = menuTop + choiceStep*float64(index)
	}

	return positions
}

// IntroMenuRenderer draws the title menu: a centred choice column whose
// selected row sits between two cursor triangles, and the open choice's
// sub-menu panel on the frame's right flank.
type IntroMenuRenderer struct {
	choiceFont  *UltimaFont
	submenuFont *UltimaFont
	hintFont    *UltimaFont
}

// NewIntroMenuRenderer derives the sub-menu and hint faces from the current
// intro font size; rebuild it after a resolution change.
func NewIntroMenuRenderer(choiceFont *UltimaFont) *IntroMenuRenderer {
	size := GetIntroFontSize()

	return &IntroMenuRenderer{
		choiceFont:  choiceFont,
		submenuFont: NewUltimaFont(size * introSubmenuFontScale),
		hintFont:    NewUltimaFont(size * introSubmenuHintScale),
	}
}

func fontLineHeight(font *UltimaFont) float64 {
	return font.TextFace.Metrics().HAscent + font.TextFace.Metrics().HDescent
}

// DrawChoices renders the top-level rows. While a sub-menu is open the column
// moves left of the panel and dims, leaving only the open parent lit.
func (r *IntroMenuRenderer) DrawChoices(screen *ebiten.Image, state IntroMenuState) {
	bounds := screen.Bounds()
	unit := introClassicPixelUnit(bounds.Dx(), bounds.Dy())
	fontHeight := fontLineHeight(r.choiceFont)
	positions := introMenuChoicePositions(bounds.Dy(), fontHeight, len(IntroChoices))
	columnLeft := float64(bounds.Dx()) * introMenuInteriorStartX
	columnRight := float64(bounds.Dx()) * (1 - introMenuInteriorStartX)
	if state.SubmenuOpen {
		panel := r.SubmenuPanelRect(bounds, state)
		columnRight = float64(panel.Min.X) - float64(introSubmenuPanelGapUnits*unit)
	}
	centerX := (columnLeft + columnRight) / centerDivisor

	for index, choice := range IntroChoices {
		enabled := index >= len(state.Enabled) || state.Enabled[index]
		selected := enabled && index == state.Selection
		ink := introChoiceInk(choice, enabled, selected, state.SubmenuOpen)
		row := r.layoutChoiceRow(choice, centerX, positions[index], unit)
		r.drawLabel(screen, r.choiceFont, choice.Label, row.labelX, positions[index], ink)
		if len(choice.Children) > 0 {
			fillMarker(screen, introChevronRows[:], image.Pt(row.chevronX, row.markerY), unit, false, ink)
		}
		if selected && !state.SubmenuOpen {
			fillMarker(screen, introMarkerRows[:], image.Pt(row.leftMarkerX, row.markerY), unit, false, color.EGAWhite)
			fillMarker(screen, introMarkerRows[:], image.Pt(row.rightMarkerX, row.markerY), unit, true, color.EGAWhite)
		}
	}
}

func introChoiceInk(choice IntroChoice, enabled, selected, submenuOpen bool) color.EGAColor {
	switch {
	case !enabled:
		return color.EGADarkGray
	case selected:
		return color.EGAYellow
	case submenuOpen:
		return color.EGADarkGray
	case choice.Muted:
		return color.EGALightGray
	default:
		return color.EGAWhite
	}
}

type introChoiceRowLayout struct {
	labelX       float64
	chevronX     int
	leftMarkerX  int
	rightMarkerX int
	markerY      int
}

// layoutChoiceRow centres the label plus its chevron and places the cursor
// triangles one gap outside that group.
func (r *IntroMenuRenderer) layoutChoiceRow(choice IntroChoice, centerX, top float64, unit int) introChoiceRowLayout {
	labelWidth, _ := r.choiceFont.MeasureString(choice.Label)
	gap := float64(introMarkerGapUnits * unit)
	groupWidth := labelWidth
	if len(choice.Children) > 0 {
		groupWidth += gap + float64(introChevronWidthUnits*unit)
	}
	groupLeft := centerX - groupWidth/centerDivisor
	markerWidth := float64(introMarkerWidthUnits * unit)
	markerHeight := float64(introMarkerHeightUnits * unit)

	return introChoiceRowLayout{
		labelX:       math.Round(groupLeft),
		chevronX:     int(math.Round(groupLeft + labelWidth + gap)),
		leftMarkerX:  int(math.Round(groupLeft - gap - markerWidth)),
		rightMarkerX: int(math.Round(groupLeft + groupWidth + gap)),
		markerY:      int(math.Round(top + (fontLineHeight(r.choiceFont)-markerHeight)/centerDivisor)),
	}
}

// SubmenuPanelRect returns the open sub-menu panel, border band included,
// sized to its longest child and centred on the title frame's height.
func (r *IntroMenuRenderer) SubmenuPanelRect(bounds image.Rectangle, state IntroMenuState) image.Rectangle {
	children := introSubmenuChildren(state)
	unit := introClassicPixelUnit(bounds.Dx(), bounds.Dy())
	inset := r.submenuInset(bounds.Dx(), unit)
	widest := 0.0
	for _, child := range children {
		width, _ := r.submenuFont.MeasureString(child)
		widest = max(widest, width)
	}
	markerSpan := float64((introMarkerWidthUnits + introMarkerGapUnits) * unit)
	width := inset*centerDivisor + markerSpan + widest
	height := inset*centerDivisor + r.submenuRowPitch()*float64(len(children)) +
		float64(introSubmenuHintGapUnits*unit) + fontLineHeight(r.hintFont)
	right := float64(bounds.Dx()) * introSubmenuPanelEndX
	centerY := float64(bounds.Dy()) * (introMenuFrameStartY + introMenuFrameEndY) / centerDivisor

	return image.Rect(
		int(math.Round(right-width)),
		int(math.Round(centerY-height/centerDivisor)),
		int(math.Round(right)),
		int(math.Round(centerY+height/centerDivisor)),
	)
}

// SubmenuPanelPlacement expresses the panel as frame-style percentages for the
// shared border sprite.
func (r *IntroMenuRenderer) SubmenuPanelPlacement(
	bounds image.Rectangle, state IntroMenuState,
) sprites.PercentBasedPlacement {
	rect := r.SubmenuPanelRect(bounds, state)

	return sprites.PercentBasedPlacement{
		StartPercentX: float64(rect.Min.X) / float64(bounds.Dx()),
		EndPercentX:   float64(rect.Max.X) / float64(bounds.Dx()),
		StartPercentY: float64(rect.Min.Y) / float64(bounds.Dy()),
		EndPercentY:   float64(rect.Max.Y) / float64(bounds.Dy()),
	}
}

// DrawSubmenu renders the open sub-menu's rows inside its panel; draw the
// panel border first.
func (r *IntroMenuRenderer) DrawSubmenu(screen *ebiten.Image, state IntroMenuState) {
	if !state.SubmenuOpen {
		return
	}
	bounds := screen.Bounds()
	unit := introClassicPixelUnit(bounds.Dx(), bounds.Dy())
	panel := r.SubmenuPanelRect(bounds, state)
	inset := r.submenuInset(bounds.Dx(), unit)
	markerX := int(math.Round(float64(panel.Min.X) + inset))
	labelX := float64(markerX) + float64((introMarkerWidthUnits+introMarkerGapUnits)*unit)
	pitch := r.submenuRowPitch()
	rowHeight := fontLineHeight(r.submenuFont)
	markerHeight := float64(introMarkerHeightUnits * unit)
	top := float64(panel.Min.Y) + inset
	for index, child := range introSubmenuChildren(state) {
		rowTop := top + pitch*float64(index) + (pitch-rowHeight)/centerDivisor
		ink := color.EGAWhite
		if index == state.SubmenuSelection {
			ink = color.EGAYellow
			markerY := int(math.Round(rowTop + (rowHeight-markerHeight)/centerDivisor))
			fillMarker(screen, introMarkerRows[:], image.Pt(markerX, markerY), unit, false, color.EGAWhite)
		}
		r.drawLabel(screen, r.submenuFont, child, labelX, rowTop, ink)
	}
	hintTop := top + pitch*float64(len(introSubmenuChildren(state))) + float64(introSubmenuHintGapUnits*unit)
	r.drawLabel(screen, r.hintFont, introSubmenuHint, labelX, hintTop, color.EGADarkGray)
}

func introSubmenuChildren(state IntroMenuState) []string {
	if !state.SubmenuOpen || state.Selection < 0 || state.Selection >= len(IntroChoices) {
		return nil
	}

	return IntroChoices[state.Selection].Children
}

func (r *IntroMenuRenderer) submenuRowPitch() float64 {
	return fontLineHeight(r.submenuFont) * introSubmenuRowSpacing
}

// submenuInset keeps content clear of the scaled border band plus a breath.
func (*IntroMenuRenderer) submenuInset(screenWidth, unit int) float64 {
	band := introSubmenuBorderReferenceBand * float64(screenWidth) / IntroSubmenuBorderIdealWidth

	return band + float64(introSubmenuInsetUnits*unit)
}

func (*IntroMenuRenderer) drawLabel(
	screen *ebiten.Image, font *UltimaFont, label string, x, y float64, ink color.EGAColor,
) {
	layout := font.Layout(label)
	options := &ebiten.DrawImageOptions{}
	options.GeoM.Translate(x, y)
	// The title formerly drew directly, so preserve full glyph ink here.
	drawLayoutWithClip(screen, layout, options, beradtext.Palette{"": ink}, nil)
}

func introClassicPixelUnit(width, height int) int {
	scaleX := float64(max(1, width)) / introLogicalWidth
	scaleY := float64(max(1, height)) / introLogicalHeight

	return max(1, int(math.Round(min(scaleX, scaleY))))
}

// introMarkerBands converts a row-width profile into classic-pixel rectangles.
// A right-pointing shape grows from origin; mirrored shapes hug the right
// edge of the same width so a facing pair stays symmetric.
func introMarkerBands(rows []int, origin image.Point, unit int, mirrored bool) []image.Rectangle {
	unit = max(1, unit)
	width := 0
	for _, row := range rows {
		width = max(width, row)
	}
	bands := make([]image.Rectangle, 0, len(rows))
	for index, row := range rows {
		top := origin.Y + index*unit
		left := origin.X
		if mirrored {
			left = origin.X + (width-row)*unit
		}
		bands = append(bands, image.Rect(left, top, left+row*unit, top+unit))
	}

	return bands
}

func fillMarker(screen *ebiten.Image, rows []int, origin image.Point, unit int, mirrored bool, ink color.EGAColor) {
	for _, band := range introMarkerBands(rows, origin, unit, mirrored) {
		vector.FillRect(
			screen,
			float32(band.Min.X),
			float32(band.Min.Y),
			float32(band.Dx()),
			float32(band.Dy()),
			ink,
			false,
		)
	}
}
