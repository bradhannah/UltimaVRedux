package widgets

import (
	"image"
	"image/color"
	"math"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"
)

func (d *AtlasDialog) drawGeography(screen *ebiten.Image) {
	screen = atlasSubImage(screen, d.mapRect)
	if d.texture != nil {
		flicker := d.lightFlicker()
		for _, part := range d.view.textureRects(d.mapRect) {
			op := &ebiten.DrawImageOptions{}
			op.GeoM.Scale(part.scale, part.scale)
			op.GeoM.Translate(part.x, part.y)
			op.Filter = ebiten.FilterLinear
			d.applyTerrainLight(op, flicker)
			screen.DrawImage(atlasSubImage(d.texture, part.source), op)
		}
		d.drawLightPool(screen, flicker)
	} else {
		d.label.Reset()
		d.label.SetMaxCharsPerLine(len("Geography unavailable"))
		d.label.AppendLine().Text("Geography unavailable", DialogControlHintColor).Done()
		d.label.DrawPanelContent(screen, d.mapRect.Min, d.mapRect.Dx(), etext.AlignStart)
	}
	vector.StrokeRect(screen, float32(d.mapRect.Min.X), float32(d.mapRect.Min.Y),
		float32(d.mapRect.Dx()), float32(d.mapRect.Dy()), 1, DialogRuleLightColor, false)
	if d.view.Grid {
		d.drawGrid(screen)
	}
	var callouts []atlasMapCallout
	if d.view.Overview {
		callouts = d.mapCallouts()
	} else if callout := d.selectedMapCallout(); !callout.plate.Empty() {
		callouts = append(callouts, callout)
	}
	plates := make([]image.Rectangle, 0, len(callouts))
	for _, callout := range callouts {
		plates = append(plates, callout.plate)
	}
	d.drawRegionLabels(screen, plates...)
	for _, callout := range callouts {
		d.drawMapCallout(screen, callout)
	}
	d.drawSuppliedSymbols(screen)
	// Unselected symbols first so the selected outline remains visible at overlaps.
	for _, e := range d.entries {
		if d.view.Overview || e.Key != d.view.SelectedKey {
			d.drawPoint(screen, e, false)
		}
	}
	// The selected symbol sits above its leader, at the very same pixel centre.
	for _, e := range d.entries {
		if !d.view.Overview && e.Key == d.view.SelectedKey {
			d.drawPoint(screen, e, true)
		}
	}
	// The current fix must remain visible above symbols, selection and labels.
	d.drawAvatarFix(screen)
}

//nolint:mnd // Half-cell projection identifies the Avatar's actual map position.
func (d *AtlasDialog) drawAvatarFix(screen *ebiten.Image) {
	if !d.dossier.CurrentFix.Measured {
		return
	}
	p := d.dossier.CurrentFix.Location
	screenX, screenY, ok := d.view.project(float64(p.X)+0.5, float64(p.Y)+0.5, d.mapRect)
	if !ok {
		return
	}
	screen = atlasSubImage(screen, d.mapRect)
	arms := d.avatarCrossRects(image.Pt(int(screenX), int(screenY)))
	outline := atlasMarkerOutlineWidth(ClassicUIPixelUnit(d.width, d.height))
	for _, arm := range arms {
		fillAtlasRect(screen, arm.Inset(-outline), ucolor.EGABlack)
	}
	for _, arm := range arms {
		fillAtlasRect(screen, arm, ucolor.EGARed)
	}
}

// Scale the cross body to 100%, 150%, then 200%; the outline remains unchanged.
//
//nolint:mnd // Integer half-steps implement the approved 1x/1.5x/2x marker-size progression.
func (d *AtlasDialog) avatarMarkerBodySize() int {
	unit := ClassicUIPixelUnit(d.width, d.height)
	base := atlasMarkerBodySize(unit)

	return (base*(2+d.view.Zoom/2) + 1) / 2
}

//nolint:mnd // Quarter-body stroke width and integer centering keep the cross crisp at every zoom.
func (d *AtlasDialog) avatarCrossRects(center image.Point) [2]image.Rectangle {
	bodySize := d.avatarMarkerBodySize()
	stroke := max(1, (bodySize+2)/4)

	return [2]image.Rectangle{
		image.Rect(center.X-bodySize/2, center.Y-stroke/2, center.X+(bodySize+1)/2, center.Y+(stroke+1)/2),
		image.Rect(center.X-stroke/2, center.Y-bodySize/2, center.X+(stroke+1)/2, center.Y+(bodySize+1)/2),
	}
}

//nolint:mnd // Integer body bounds match the stamp rasterization and include its outline.
func (d *AtlasDialog) avatarMarkerBounds(center image.Point) image.Rectangle {
	bodySize := d.avatarMarkerBodySize()
	origin := center.Sub(image.Pt(bodySize/2, bodySize/2))

	return image.Rectangle{Min: origin, Max: origin.Add(image.Pt(bodySize, bodySize))}.
		Inset(-atlasMarkerOutlineWidth(ClassicUIPixelUnit(d.width, d.height)))
}

//nolint:varnamelen // e is one detached marker entry.
func (d *AtlasDialog) drawPoint(screen *ebiten.Image, e journal.AtlasEntry, selected bool) {
	center, ok := d.atlasMarkerCenter(e)
	if !ok {
		return
	}
	// Clip geometry to the map pane; labels are measured separately within it.
	mapScreen := atlasSubImage(screen, d.mapRect)
	if selected {
		d.drawSelectionBrackets(mapScreen, center)
	}
	colour := atlasPlaceColor(e)
	if e.Note.ID != 0 {
		// Personal-note map ink is independent of discovery and Places-row text.
		colour = DialogAtlasDiscoveredColor
	}
	d.drawAtlasMarker(mapScreen, e, center, colour, true)
}

// Selection stays centred on the actual marker, with integer classic-pixel corners.
// The symbol's discovery colour is never replaced by the selection highlight.
//
//nolint:mnd // Half-unit backing clearance keeps brackets outside the full marker outline.
func (d *AtlasDialog) selectionBounds(center image.Point) image.Rectangle {
	unit := ClassicUIPixelUnit(d.width, d.height)
	radius := (d.atlasSymbolCell()+1)/2 + unit + max(1, unit/2)

	return image.Rect(center.X-radius, center.Y-radius, center.X+radius+1, center.Y+radius+1)
}

//nolint:mnd // Two-cell corner arms with a half-unit contrast backing, separate from the icon outline.
func (d *AtlasDialog) drawSelectionBrackets(screen *ebiten.Image, center image.Point) {
	unit := ClassicUIPixelUnit(d.width, d.height)
	bounds := d.selectionBounds(center)
	arm := 2 * unit
	corners := make([]image.Rectangle, 0, 8)
	for _, corner := range []image.Point{bounds.Min, {X: bounds.Max.X - arm, Y: bounds.Min.Y},
		{X: bounds.Min.X, Y: bounds.Max.Y - unit}, {X: bounds.Max.X - arm, Y: bounds.Max.Y - unit}} {
		rect := image.Rect(corner.X, corner.Y, corner.X+arm, corner.Y+unit)
		corners = append(corners, rect)
	}
	for _, corner := range []image.Point{bounds.Min, {X: bounds.Max.X - unit, Y: bounds.Min.Y},
		{X: bounds.Min.X, Y: bounds.Max.Y - arm}, {X: bounds.Max.X - unit, Y: bounds.Max.Y - arm}} {
		rect := image.Rect(corner.X, corner.Y, corner.X+unit, corner.Y+arm)
		corners = append(corners, rect)
	}
	for _, rect := range corners {
		fillAtlasRect(screen, rect.Inset(-max(1, unit/2)), ucolor.EGABlack)
	}
	for _, rect := range corners {
		fillAtlasRect(screen, rect, DialogLabelColor)
	}
}

func fillAtlasRect(screen *ebiten.Image, rect image.Rectangle, colour color.Color) {
	vector.FillRect(screen, float32(rect.Min.X), float32(rect.Min.Y),
		float32(rect.Dx()), float32(rect.Dy()), colour, false)
}

// atlasPlaceColor is shared by map symbols and Places rows. A supplied name or
// chart coordinate does not establish personal discovery, even when selected.
func atlasPlaceColor(entry journal.AtlasEntry) color.Color {
	if entry.Note.ID != 0 || entry.Vessel.ID != 0 {
		return DialogValueColor
	}
	if entry.Discovered {
		return DialogAtlasDiscoveredColor
	}

	return DialogAtlasUndiscoveredColor
}

// drawAtlasSymbol is the bare directory silhouette; map markers add dark contrast.
// Unknown kinds draw nothing rather than inventing a house or a location type.
func (d *AtlasDialog) drawAtlasSymbol(screen *ebiten.Image, e journal.AtlasEntry,
	center image.Point, colour color.Color) {
	d.drawAtlasMarker(screen, e, center, colour, false)
}

func (d *AtlasDialog) drawAtlasMarker(screen *ebiten.Image, e journal.AtlasEntry,
	center image.Point, colour color.Color, outlined bool) {
	stamp := atlasMarkerStamp(e.Kind)
	if e.Kind == atlasMoonstoneKind {
		stamp = atlasMoonStamp(e.Phase)
	}
	drawAtlasStamp(screen, stamp, center, ClassicUIPixelUnit(d.width, d.height), colour, outlined)
}

// Includes both artwork-cell-width outline edges in the compact size budget.
func (d *AtlasDialog) atlasSymbolCell() int {
	unit := ClassicUIPixelUnit(d.width, d.height)

	return atlasMarkerBodySize(unit) + 2*atlasMarkerOutlineWidth(unit)
}

func (d *AtlasDialog) directoryRowHeight() int {
	return max(d.rowHeight, d.atlasSymbolCell())
}

//nolint:mnd,varnamelen // A-P major grid spacing and tiny inset are presentation geometry.
func (d *AtlasDialog) drawGrid(screen *ebiten.Image) {
	ox, oy := d.view.origin()
	for coordinate := 0; coordinate < 256; coordinate += 16 {
		x, _, vx := d.view.project(float64(coordinate), oy, d.mapRect)
		if vx {
			vector.StrokeLine(screen, float32(x), float32(d.mapRect.Min.Y), float32(x), float32(d.mapRect.Max.Y),
				1, ucolor.From(ucolor.EGAWhite).Intensity(.25).RGBA(), false)
		}
		_, y, vy := d.view.project(ox, float64(coordinate), d.mapRect)
		if vy {
			vector.StrokeLine(screen, float32(d.mapRect.Min.X), float32(y), float32(d.mapRect.Max.X), float32(y),
				1, ucolor.From(ucolor.EGAWhite).Intensity(.25).RGBA(), false)
		}
		// A-P major grid lettering allows printed-chart coordinate reading.
		letter := string(rune('A' + coordinate/16))
		if vx {
			d.gridLetter(screen, letter, image.Pt(int(x)+2, d.mapRect.Min.Y+2))
		}
		if vy {
			d.gridLetter(screen, letter, image.Pt(d.mapRect.Min.X+2, int(y)+2))
		}
	}
}

func (d *AtlasDialog) gridLetter(screen *ebiten.Image, value string, point image.Point) {
	d.label.Reset()
	d.label.AppendLine().Text(value, DialogControlHintColor).Done()
	d.label.DrawPanelContent(atlasSubImage(screen, d.mapRect), point, d.rowHeight, etext.AlignStart)
}

// atlasMapCallout is a single authorised place name and its screen geometry.
// A zero plate means no callout; offscreen places never produce clamped points.
type atlasMapCallout struct {
	name         string
	plate        image.Rectangle
	marker, edge image.Point
}

func (d *AtlasDialog) selectedMapCallout() atlasMapCallout {
	callouts := d.mapCallouts()
	if len(callouts) == 0 {
		var absent atlasMapCallout

		return absent
	}

	return callouts[0]
}

//nolint:mnd // Half-cell map anchors and half-symbol obstacle radii match marker rasterization.
func (d *AtlasDialog) mapCallouts() []atlasMapCallout {
	if d.view.Labels != AtlasLabelsSelected {
		return nil
	}
	var obstacles []image.Rectangle
	radius := (d.atlasSymbolCell() + 1) / 2
	addMarker := func(point image.Point) {
		obstacles = append(obstacles, image.Rect(point.X-radius, point.Y-radius, point.X+radius+1, point.Y+radius+1))
	}
	for _, entry := range d.entries {
		if point, ok := d.atlasMarkerCenter(entry); ok {
			addMarker(point)
		}
	}
	for _, symbol := range d.dossier.Symbols {
		if point, ok := d.suppliedSymbolCenter(symbol); ok {
			addMarker(point)
		}
	}
	if d.dossier.CurrentFix.Measured {
		position := d.dossier.CurrentFix.Location
		x, y, visible := d.view.project(float64(position.X)+0.5, float64(position.Y)+0.5, d.mapRect)
		if visible {
			obstacles = append(obstacles, d.avatarMarkerBounds(image.Pt(int(x), int(y))))
		}
	}
	var callouts []atlasMapCallout
	for _, entry := range d.entries {
		if !d.view.Overview && entry.Key != d.view.SelectedKey {
			continue
		}
		marker, ok := d.atlasMarkerCenter(entry)
		if !ok {
			continue
		}
		callout := d.placeMapCallout(entry, marker, obstacles)
		if callout.plate.Empty() {
			continue
		}
		callouts = append(callouts, callout)
		obstacles = append(obstacles, callout.plate)
	}

	return callouts
}

//nolint:mnd // Classic pixel padding and edge clearance match the existing single-nameplate style.
func (d *AtlasDialog) placeMapCallout(selected journal.AtlasEntry, marker image.Point,
	obstacles []image.Rectangle) atlasMapCallout {
	// Name is already public/generic as appropriate; never look up an identity here.
	unit := ClassicUIPixelUnit(d.width, d.height)
	padding := 3 * unit
	name := FitDialogLine(selected.Name, d.font, d.mapRect.Dx()-2*padding-2*unit)
	width, height := d.font.MeasureString(name)
	plate := atlasCalloutPlate(marker, image.Pt(int(math.Ceil(width))+2*padding,
		int(math.Ceil(height))+2*padding), d.mapRect, obstacles, d.selectionBounds(marker), unit)
	if plate.Empty() {
		// A bounded crowded fallback keeps every authorized name. Only plates,
		// never marker anchors, may be repositioned inside the pane.
		size := image.Pt(min(d.mapRect.Dx()-2*unit, int(math.Ceil(width))+2*padding), int(math.Ceil(height))+2*padding)
		pane := d.mapRect.Inset(unit)
		left := min(max(pane.Min.X, marker.X), pane.Max.X-size.X)
		top := min(max(pane.Min.Y, marker.Y), pane.Max.Y-size.Y)
		plate = image.Rectangle{Min: image.Pt(left, top), Max: image.Pt(left+size.X, top+size.Y)}
	}
	// Meet a straight plate edge, leaving the stepped corners clear of the elbow.
	edge := image.Pt(min(max(marker.X, plate.Min.X+padding), plate.Max.X-padding),
		min(max(marker.Y, plate.Min.Y+padding), plate.Max.Y-padding))
	switch {
	case marker.X < plate.Min.X:
		edge.X = plate.Min.X
	case marker.X >= plate.Max.X:
		edge.X = plate.Max.X - 1
	case marker.Y < plate.Min.Y:
		edge.Y = plate.Min.Y
	default:
		edge.Y = plate.Max.Y - 1
	}

	return atlasMapCallout{name: name, plate: plate, marker: marker, edge: edge}
}

//nolint:mnd // Half a world cell identifies the displayed marker's centre.
func (d *AtlasDialog) atlasMarkerCenter(entry journal.AtlasEntry) (image.Point, bool) {
	position, ok := entry.PlotPosition()
	if !ok {
		var absent image.Point

		return absent, false
	}
	x, y, visible := d.view.project(float64(position.X)+0.5, float64(position.Y)+0.5, d.mapRect)

	return image.Pt(int(x), int(y)), visible
}

// atlasCalloutPlate tries nearby sides/corners, preferring no marker overlap.
// Even a crowded fallback may never cover the selected marker or its outline.
//
//nolint:mnd // Short leaders, selection clearance and pane inset are screen-pixel geometry.
func atlasCalloutPlate(marker, size image.Point, pane image.Rectangle, obstacles []image.Rectangle,
	selected image.Rectangle, unit int) image.Rectangle {
	pane = pane.Inset(unit)
	if size.X > pane.Dx() || size.Y > pane.Dy() {
		var absent image.Rectangle

		return absent
	}
	selected = selected.Inset(-max(1, unit/2))
	right, left := selected.Max.X-marker.X+2*unit, selected.Min.X-marker.X-size.X-2*unit
	below, above := selected.Max.Y-marker.Y+2*unit, selected.Min.Y-marker.Y-size.Y-2*unit
	var best image.Rectangle
	bestOverlap := math.MaxInt
	for _, offset := range []image.Point{
		{X: right, Y: -size.Y / 2}, {X: left, Y: -size.Y / 2},
		{X: -size.X / 2, Y: below}, {X: -size.X / 2, Y: above},
		{X: right, Y: above}, {X: left, Y: above},
		{X: right, Y: below}, {X: left, Y: below},
	} {
		left := min(max(pane.Min.X, marker.X+offset.X), pane.Max.X-size.X)
		top := min(max(pane.Min.Y, marker.Y+offset.Y), pane.Max.Y-size.Y)
		plate := image.Rect(left, top, left+size.X, top+size.Y)
		if plate.Overlaps(selected) {
			continue
		}
		overlap := atlasCalloutOverlap(plate, obstacles, unit)
		if overlap < bestOverlap {
			best, bestOverlap = plate, overlap
		}
		if overlap == 0 {
			break
		}
	}
	// Nearby candidates normally suffice. A fixed grid broadens crowded layouts
	// without culling labels, random choices, or an unbounded search.
	if bestOverlap > 0 {
		return atlasCrowdedCalloutPlate(size, pane, obstacles, selected, unit, best, bestOverlap)
	}

	return best
}

func atlasCalloutOverlap(plate image.Rectangle, obstacles []image.Rectangle, unit int) int {
	overlap := 0
	for _, obstacle := range obstacles {
		intersection := plate.Inset(-unit).Intersect(obstacle)
		overlap += intersection.Dx() * intersection.Dy()
	}

	return overlap
}

func atlasCrowdedCalloutPlate(size image.Point, pane image.Rectangle, obstacles []image.Rectangle,
	selected image.Rectangle, unit int, best image.Rectangle, bestOverlap int) image.Rectangle {
	const gridSteps = 12
	for row := range gridSteps + 1 {
		for column := range gridSteps + 1 {
			left := pane.Min.X + (pane.Dx()-size.X)*column/gridSteps
			top := pane.Min.Y + (pane.Dy()-size.Y)*row/gridSteps
			plate := image.Rect(left, top, left+size.X, top+size.Y)
			if plate.Overlaps(selected) {
				continue
			}
			overlap := atlasCalloutOverlap(plate, obstacles, unit)
			if overlap < bestOverlap {
				best, bestOverlap = plate, overlap
			}
			if overlap == 0 {
				return best
			}
		}
	}

	return best
}

//nolint:mnd // Classic-cell frame, interior padding and integer elbow thickness.
func (d *AtlasDialog) drawMapCallout(screen *ebiten.Image, callout atlasMapCallout) {
	if callout.plate.Empty() {
		return
	}
	screen = atlasSubImage(screen, d.mapRect)
	unit := ClassicUIPixelUnit(d.width, d.height)
	elbow := image.Pt(callout.marker.X, callout.edge.Y)
	if callout.edge.Y == callout.plate.Min.Y || callout.edge.Y == callout.plate.Max.Y-1 {
		elbow = image.Pt(callout.edge.X, callout.marker.Y)
	}
	for _, thickness := range []int{unit + 2*max(1, unit/2), unit} {
		colour := color.Color(ucolor.EGABlack)
		if thickness == unit {
			colour = DialogLabelColor
		}
		for _, segment := range [][2]image.Point{{callout.marker, elbow}, {elbow, callout.edge}} {
			a, b := segment[0], segment[1]
			rect := image.Rect(min(a.X, b.X)-thickness/2, min(a.Y, b.Y)-thickness/2,
				max(a.X, b.X)+(thickness+1)/2, max(a.Y, b.Y)+(thickness+1)/2)
			fillAtlasRect(screen, rect, colour)
		}
	}
	plate := callout.plate
	FillClassicPixelRoundedRect(screen, plate, unit, ucolor.DialogBackground.RGBA())
	DrawClassicPixelRoundedFrame(screen, plate, unit, DialogLabelColor)
	d.label.Reset()
	d.label.SetMaxCharsPerLine(max(1, len([]rune(callout.name))))
	d.label.AppendLine().Text(callout.name, DialogLabelColor).Done()
	d.label.DrawPanelContent(screen, plate.Min.Add(image.Pt(3*unit, 3*unit)), plate.Dx()-6*unit, etext.AlignStart)
}

// Regions are area typography, not synthetic AtlasEntry points. They remain
// subdued in every Places filter/label mode and yield to the selected nameplate.
func (d *AtlasDialog) drawRegionLabels(screen *ebiten.Image, plates ...image.Rectangle) {
	var absent image.Rectangle
	screen = atlasSubImage(screen, d.mapRect)
	for _, region := range references.AtlasRegionLabels() {
		x, y, visible := d.view.project(float64(region.X), float64(region.Y), d.mapRect)
		if !visible {
			continue
		}
		lines, rect := d.regionLabelLayout(region.Name, image.Pt(int(x), int(y)), absent)
		for _, plate := range plates {
			if rect.Overlaps(plate.Inset(-2)) {
				rect = absent

				break
			}
		}
		if rect.Empty() {
			continue
		}
		d.drawRegionLabel(screen, lines, rect)
	}
}

// regionLabelLayout balances multiword proper names over two complete lines.
// Only decoration is clamped: the projected anchor and map geometry are unchanged.
// Regions yield to a selected plate rather than inventing leaders or backing boxes.
//
//nolint:mnd // Two-pixel pane/plate clearance and centering are screen layout geometry.
func (d *AtlasDialog) regionLabelLayout(name string, anchor image.Point,
	selectedPlate image.Rectangle) ([]string, image.Rectangle) {
	var absent image.Rectangle
	words := strings.Fields(name)
	if len(words) == 0 {
		return nil, absent
	}
	lines := []string{strings.Join(words, " ")}
	width, _ := d.font.MeasureString(lines[0])
	if len(words) > 1 {
		width = math.Inf(1)
		for split := 1; split < len(words); split++ {
			first, second := strings.Join(words[:split], " "), strings.Join(words[split:], " ")
			firstWidth, _ := d.font.MeasureString(first)
			secondWidth, _ := d.font.MeasureString(second)
			if widest := max(firstWidth, secondWidth); widest < width {
				lines, width = []string{first, second}, widest
			}
		}
	}
	size := image.Pt(int(math.Ceil(width)), int(math.Ceil(d.label.GetTotalHeightForLines(len(lines)))))
	pane := d.mapRect.Inset(2)
	if size.X > pane.Dx() || size.Y > pane.Dy() {
		return nil, absent
	}
	left := min(max(pane.Min.X, anchor.X-size.X/2), pane.Max.X-size.X)
	top := min(max(pane.Min.Y, anchor.Y-size.Y/2), pane.Max.Y-size.Y)
	rect := image.Rectangle{Min: image.Pt(left, top), Max: image.Pt(left+size.X, top+size.Y)}
	if !selectedPlate.Empty() && rect.Overlaps(selectedPlate.Inset(-2)) {
		return nil, absent
	}

	return lines, rect
}

func (d *AtlasDialog) drawRegionLabel(screen *ebiten.Image, lines []string, rect image.Rectangle) {
	maxChars := 1
	for _, line := range lines {
		maxChars = max(maxChars, len([]rune(line)))
	}
	d.label.Reset()
	d.label.SetMaxCharsPerLine(maxChars)
	for _, line := range lines {
		d.label.AppendLine().Text(line, DialogControlHintColor).Done()
	}
	d.label.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignCenter)
}

func atlasSubImage(source *ebiten.Image, rect image.Rectangle) *ebiten.Image {
	result, ok := source.SubImage(rect).(*ebiten.Image)
	if !ok {
		panic("Ebitengine SubImage violated its image contract")
	}

	return result
}
