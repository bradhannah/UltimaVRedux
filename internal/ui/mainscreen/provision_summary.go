package mainscreen

import (
	"image"
	"image/color"
	"math"
	"strconv"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// ProvisionSummary displays resources, ammunition, and a dedicated calendar row.
type ProvisionSummary struct {
	ultimaFont          *text.UltimaFont
	output              *text.Output
	fields              [provisionFieldCount]provisionField
	bounds              image.Rectangle
	rowHeight           int
	ammoLabelWidth      int
	ammoValueWidth      int
	statusWidth         int
	statusGap           int
	labelGap            int
	frameInset          int
	resourceLabelWidths [provisionColumnCount]int
	resourceValueWidths [provisionColumnCount]int
}

const (
	provisionRowCount               = 4
	provisionColumnCount            = 3
	provisionResourceRows           = 2
	provisionAmmoColumns            = 2
	provisionAmmoRow                = 2
	provisionCalendarRow            = 3
	provisionCenterDivisor          = 2
	provisionOutputMaxLines         = 1
	provisionLabelGapAtReference    = 2
	provisionFrameGutterAtReference = 2
	provisionFrameGutterCount       = 2
	provisionAmmoCellBase           = provisionResourceRows * provisionColumnCount
	provisionCalendarCellBase       = provisionAmmoCellBase + provisionAmmoColumns
	provisionCalendarCells          = 2
	provisionCellFields             = 3
	provisionFieldCount             = (provisionCalendarCellBase + provisionCalendarCells) * provisionCellFields
)

type provisionResource struct {
	label       string
	value       string
	status      string
	labelColor  color.Color
	valueColor  color.Color
	statusColor color.Color
}

// Fixed resource blocks are centered in thirds, sharing label and quantity slots
// across both rows. Ammunition blocks are centered in halves with reserved status
// slots. Calendar text occupies its own bottom row.
// Showing LOW or OUT cannot move a quantity or the next column.
type provisionCell struct {
	label  image.Rectangle
	value  image.Rectangle
	status image.Rectangle
}

// NewProvisionSummaryForWidth creates a summary scaled to the logical render width.
func NewProvisionSummaryForWidth(screenWidth int) *ProvisionSummary {
	var summary ProvisionSummary
	summary.rowHeight = ProvisionAmmoRowHeight(screenWidth)
	summary.ultimaFont, summary.output = newMainScreenOutput(screenWidth, widgets.FontSizeHUD, provisionOutputMaxLines)
	// Each fixed field keeps its own output and refills it only when its content
	// changes, so an unchanged field reuses its cached layout every frame.
	for slot := range summary.fields {
		_, summary.fields[slot].output = newMainScreenOutput(screenWidth, widgets.FontSizeHUD, provisionOutputMaxLines)
	}
	summary.ammoLabelWidth = max(measuredProvisionWidth(summary.ultimaFont, "Ar"),
		measuredProvisionWidth(summary.ultimaFont, "Qr"))
	summary.ammoValueWidth = measuredProvisionWidth(summary.ultimaFont, "99")
	summary.statusWidth = max(measuredProvisionWidth(summary.ultimaFont, "LOW"),
		measuredProvisionWidth(summary.ultimaFont, "OUT"))
	summary.statusGap = measuredProvisionWidth(summary.ultimaFont, " ")
	labels := [provisionColumnCount][provisionResourceRows]string{{"F", "$"}, {"K", "T"}, {"G", "Ka"}}
	maximumValues := [provisionColumnCount]string{"9999", "99", "99"}
	for column, pair := range labels {
		summary.resourceLabelWidths[column] = max(measuredProvisionWidth(summary.ultimaFont, pair[0]),
			measuredProvisionWidth(summary.ultimaFont, pair[1]))
		summary.resourceValueWidths[column] = measuredProvisionWidth(summary.ultimaFont, maximumValues[column])
	}
	scale := float64(screenWidth) / mainScreenReferenceWidth
	summary.labelGap = max(1, int(math.Round(provisionLabelGapAtReference*scale)))
	// Retain the established raster-frame clearance independently of text gaps.
	frameGutter := max(1, int(math.Round(provisionFrameGutterAtReference*scale)))
	summary.frameInset = measuredProvisionWidth(summary.ultimaFont, "0") + provisionFrameGutterCount*frameGutter

	return &summary
}

// ProvisionAmmoRowHeight reserves one normal-font summary line, including the
// established scaled baseline spacing. No compact typography is used for ammo.
func ProvisionAmmoRowHeight(screenWidth int) int {
	return int(math.Ceil(float64(widgets.FontSizeHUD) * widgets.MainScreenLineSpacingMultiplier *
		float64(screenWidth) / mainScreenReferenceWidth))
}

// ProvisionSummaryHeight reserves the same four normal-font rows at every resolution.
func ProvisionSummaryHeight(screenWidth int) int {
	return provisionRowCount * ProvisionAmmoRowHeight(screenWidth)
}

// SetBounds assigns the four-line content area reserved by the main-screen layout.
func (p *ProvisionSummary) SetBounds(bounds image.Rectangle) {
	p.bounds = bounds
}

// Draw renders the four-row summary without changing gameplay state.
func (p *ProvisionSummary) Draw(gameState *game_state.GameState, screen *ebiten.Image) {
	if gameState == nil {
		logging.Warnf(logging.CategoryUI, "ProvisionSummary.Draw called with nil gameState, skipping draw")

		return
	}
	if screen == nil {
		logging.Warnf(logging.CategoryUI, "ProvisionSummary.Draw called with nil screen, skipping draw")

		return
	}
	if p.bounds.Empty() {
		return
	}

	resources := provisionResources(gameState)
	rows := p.rows()
	for rowIndex, row := range resources {
		cells := p.resourceCells(rows[rowIndex])
		for column, resource := range row {
			p.drawResource(screen, rowIndex*provisionColumnCount+column, cells[column], resource)
		}
	}
	ammunition := p.ammunitionCells(rows[provisionAmmoRow])
	for column, resource := range provisionAmmunitionResources(gameState) {
		p.drawResource(screen, provisionAmmoCellBase+column, ammunition[column], resource)
	}
	date, clock := p.calendarCells(rows[provisionCalendarRow])
	p.fields[provisionCalendarCellBase*provisionCellFields].draw(screen, date,
		gameState.DateTime.GetDateAsString(), ucolor.UITextPrimary.RGBA(), etext.AlignCenter)
	p.fields[(provisionCalendarCellBase+1)*provisionCellFields].draw(screen, clock,
		gameState.DateTime.GetTimeAsString(), ucolor.UITextPrimary.RGBA(), etext.AlignCenter)
}

func provisionResources(state *game_state.GameState) [provisionResourceRows][provisionColumnCount]provisionResource {
	white := ucolor.UITextPrimary.RGBA()

	return [provisionResourceRows][provisionColumnCount]provisionResource{
		{
			{label: "F", value: formatProvisionQuantity(state.PartyState.Inventory.Provisions.Food.Get()), status: "",
				labelColor: ucolor.EGALightGreen, valueColor: white, statusColor: white},
			{label: "K", value: formatProvisionQuantity(state.PartyState.Inventory.Provisions.Keys.Get()), status: "",
				labelColor: ucolor.EGAYellow, valueColor: white, statusColor: white},
			{label: "G", value: formatProvisionQuantity(state.PartyState.Inventory.Provisions.Gems.Get()), status: "",
				labelColor: ucolor.EGALightCyan, valueColor: white, statusColor: white},
		},
		{
			{label: "$", value: formatProvisionQuantity(state.PartyState.Inventory.Gold.Get()), status: "",
				labelColor: ucolor.EGAYellow, valueColor: white, statusColor: white},
			{label: "T", value: formatProvisionQuantity(state.PartyState.Inventory.Provisions.Torches.Get()), status: "",
				labelColor: ucolor.EGALightRed, valueColor: white, statusColor: white},
			{label: "Ka", value: strconv.Itoa(state.PartyState.Karma.Value), status: "",
				labelColor: ucolor.EGALightMagenta, valueColor: white, statusColor: white},
		},
	}
}

func provisionAmmunitionResources(state *game_state.GameState) [provisionAmmoColumns]provisionResource {
	var resources [provisionAmmoColumns]provisionResource
	for index, count := range SnapshotAmmunition(&state.PartyState.Inventory) {
		value, statusColor := ammunitionValue(count)
		quantity, status, _ := strings.Cut(value, " ")
		resources[index] = provisionResource{
			label: [2]string{"Ar", "Qr"}[index], value: quantity, status: status,
			labelColor: ucolor.EGALightCyan, valueColor: ucolor.UITextPrimary.RGBA(), statusColor: statusColor,
		}
	}

	return resources
}

func formatProvisionQuantity(value uint16) string {
	return strconv.FormatUint(uint64(value), 10)
}

func (p *ProvisionSummary) rows() [provisionRowCount]image.Rectangle {
	// The scene's left content edge already clears the rasterized frame. Its
	// asymmetric right edge needs a normal glyph plus gutters of frame clearance;
	// reserve that on the right instead of shifting the entire grid into the left frame.
	bounds := p.bounds
	bounds.Max.X -= p.frameInset
	var rows [provisionRowCount]image.Rectangle
	for index := range rows {
		bottom := bounds.Max.Y - (provisionRowCount-1-index)*p.rowHeight
		rows[index] = image.Rect(bounds.Min.X, bottom-p.rowHeight, bounds.Max.X, bottom)
	}

	return rows
}

func (p *ProvisionSummary) resourceCells(bounds image.Rectangle) [provisionColumnCount]provisionCell {
	var cells [provisionColumnCount]provisionCell
	for index := range cells {
		cells[index] = p.centeredResourceCell(provisionColumnBounds(bounds, index, len(cells)),
			p.resourceLabelWidths[index], p.resourceValueWidths[index], 0)
	}

	return cells
}

func (p *ProvisionSummary) ammunitionCells(bounds image.Rectangle) [provisionAmmoColumns]provisionCell {
	var cells [provisionAmmoColumns]provisionCell
	left, right := p.calendarCells(bounds)
	for index, half := range [provisionAmmoColumns]image.Rectangle{left, right} {
		cells[index] = p.centeredResourceCell(half, p.ammoLabelWidth, p.ammoValueWidth, p.statusWidth)
	}

	return cells
}

func provisionColumnBounds(bounds image.Rectangle, index, columns int) image.Rectangle {
	left := bounds.Min.X + index*bounds.Dx()/columns
	right := bounds.Min.X + (index+1)*bounds.Dx()/columns

	return image.Rect(left, bounds.Min.Y, right, bounds.Max.Y)
}

func (p *ProvisionSummary) centeredResourceCell(
	bounds image.Rectangle, labelWidth, valueWidth, statusWidth int,
) provisionCell {
	width := labelWidth + p.labelGap + valueWidth
	if statusWidth > 0 {
		width += p.statusGap + statusWidth
	}
	left := bounds.Min.X + (bounds.Dx()-width)/provisionCenterDivisor
	valueLeft := left + labelWidth + p.labelGap
	valueRight := valueLeft + valueWidth
	var status image.Rectangle
	if statusWidth > 0 {
		statusLeft := valueRight + p.statusGap
		status = image.Rect(statusLeft, bounds.Min.Y, statusLeft+statusWidth, bounds.Max.Y)
	}

	return provisionCell{
		label:  image.Rect(left, bounds.Min.Y, left+labelWidth, bounds.Max.Y),
		value:  image.Rect(valueLeft, bounds.Min.Y, valueRight, bounds.Max.Y),
		status: status,
	}
}

func (p *ProvisionSummary) calendarCells(bounds image.Rectangle) (image.Rectangle, image.Rectangle) {
	// Give an odd pixel to the longer date rather than the shorter clock.
	middle := bounds.Min.X + (bounds.Dx()+1)/provisionCenterDivisor
	date := image.Rect(bounds.Min.X, bounds.Min.Y, middle, bounds.Max.Y)
	clock := image.Rect(middle, bounds.Min.Y, bounds.Max.X, bounds.Max.Y)

	return date, clock
}

func (p *ProvisionSummary) drawResource(
	screen *ebiten.Image, cellIndex int, cell provisionCell, resource provisionResource,
) {
	fields := p.fields[cellIndex*provisionCellFields : (cellIndex+1)*provisionCellFields]
	fields[0].draw(screen, cell.label, resource.label, resource.labelColor, etext.AlignEnd)
	fields[1].draw(screen, cell.value, resource.value, resource.valueColor, etext.AlignStart)
	if resource.status != "" {
		fields[2].draw(screen, cell.status, resource.status, resource.statusColor, etext.AlignStart)
	}
}

// provisionField is one fixed text field with its own output. It refills the output
// only when the value, colour or alignment changes: Reset discards the layout cache.
type provisionField struct {
	output    *text.Output
	value     string
	rgba      [4]uint32
	alignment etext.Align
	filled    bool
}

func (f *provisionField) draw(
	screen *ebiten.Image,
	bounds image.Rectangle,
	value string,
	valueColor color.Color,
	alignment etext.Align,
) {
	var rgba [4]uint32
	rgba[0], rgba[1], rgba[2], rgba[3] = valueColor.RGBA()
	if f.filled && f.value == value && f.rgba == rgba && f.alignment == alignment {
		drawFilledProvisionText(screen, f.output, bounds, alignment)

		return
	}
	drawProvisionText(screen, f.output, bounds, value, valueColor, alignment)
	f.value, f.rgba, f.alignment, f.filled = value, rgba, alignment, true
}

func drawProvisionText(
	screen *ebiten.Image,
	output *text.Output,
	bounds image.Rectangle,
	value string,
	valueColor color.Color,
	alignment etext.Align,
) {
	fillProvisionText(output, value, valueColor, alignment)
	drawFilledProvisionText(screen, output, bounds, alignment)
}

func fillProvisionText(output *text.Output, value string, valueColor color.Color, alignment etext.Align) {
	output.Reset()
	// Layout measures the complete field. An M-based character estimate can
	// otherwise wrap a numeric date that fits at a fractional display scale.
	output.SetMaxCharsPerLine(max(1, len(value)))
	output.AppendLine().Text(value, valueColor).WithAlignment(alignment).Done()
}

func drawFilledProvisionText(
	screen *ebiten.Image, output *text.Output, bounds image.Rectangle, alignment etext.Align,
) {
	y := bounds.Min.Y + (bounds.Dy()-int(math.Ceil(output.GetFontHeight())))/provisionCenterDivisor
	output.DrawPanelContent(screen, image.Pt(bounds.Min.X, y), bounds.Dx(), alignment)
}

func measuredProvisionWidth(font *text.UltimaFont, value string) int {
	width, _ := font.MeasureString(value)

	return int(math.Ceil(width))
}
