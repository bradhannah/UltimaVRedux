package widgets

import "image"

type dungeonViewLayout struct {
	header        image.Rectangle
	mapArea       image.Rectangle
	mapGrid       image.Rectangle
	north         image.Rectangle // The "N" above the map: north is up.
	legend        image.Rectangle
	footer        image.Rectangle
	mapScale      int
	legendScale   int
	legendColumns int
	legendRows    int
	rowHeight     int
	textHeight    int
	edgeWidth     int
	edgeHeight    int
	gap           int
}

type dungeonViewMeasurements struct {
	headerHeight       int
	footerHeight       int
	textHeight         int
	compactLabelHeight int
	labelWidth         int
	legendScale        int
	edgeWidth          int
	edgeHeight         int
	entryCount         int
	horizontalWrap     bool
	verticalWrap       bool
}

//nolint:mnd // A compact logical-pixel gap scales with the shared classic frame.
func dungeonViewLayoutGap(screenWidth, screenHeight int) int {
	return max(2, getClassicUIPixelUnit(screenWidth, screenHeight)*2)
}

// The actual key, font metrics and available interior determine row count and
// columns. Try the largest crisp map first; do not reserve space for absent features.
//
//nolint:mnd // Halves center the composition and keep the key subordinate to the map.
func calculateDungeonViewLayout(
	bounds image.Rectangle,
	screenWidth, screenHeight int,
	measure dungeonViewMeasurements,
) dungeonViewLayout {
	gap := dungeonViewLayoutGap(screenWidth, screenHeight)
	header := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Max.X, bounds.Min.Y+measure.headerHeight)
	footer := image.Rect(bounds.Min.X, bounds.Max.Y-measure.footerHeight, bounds.Max.X, bounds.Max.Y)
	body := image.Rect(bounds.Min.X, header.Max.Y+gap, bounds.Max.X, footer.Min.Y-gap)
	var gutter image.Point
	if measure.horizontalWrap {
		gutter.X = measure.edgeWidth + gap
	}
	if measure.verticalWrap {
		gutter.Y = measure.edgeHeight + gap
	}
	const nativeGrid = dungeonViewCellSize * dungeonViewFloorCells
	// The north marker has its own strip above the map and any edge labels.
	north := measure.edgeHeight + gap
	maximum := max(1, min((body.Dy()-north-gutter.Y*2)/nativeGrid, (body.Dx()-gutter.X*2)/nativeGrid))
	// Key icons follow the key's typography, not the size of the primary map.
	// Compact row gaps allow a dense revealed key to stay in one readable column.
	keyScale := measure.legendScale
	rowHeight := max(dungeonViewCellSize*keyScale, measure.textHeight) + max(2, gap/2)
	maxRows := max(1, body.Dy()/rowHeight)
	columns := (measure.entryCount + maxRows - 1) / maxRows
	rows := (measure.entryCount + columns - 1) / columns
	keyWidth := columns*(dungeonViewCellSize*keyScale+measure.labelWidth+gap*2) - gap
	keyHeight := rows * rowHeight
	for scale := maximum; scale >= 1; scale-- {
		mapSize := image.Pt(nativeGrid*scale+gutter.X*2, nativeGrid*scale+gutter.Y*2)
		if columns > 2 || mapSize.X+gap+keyWidth > body.Dx() {
			continue
		}
		if measure.verticalWrap && measure.edgeWidth+gap > dungeonViewCellSize*scale {
			continue
		}
		origin := image.Pt(body.Min.X+(body.Dx()-mapSize.X-gap-keyWidth)/2,
			body.Min.Y+north+(body.Dy()-north-mapSize.Y)/2)
		mapArea := image.Rectangle{Min: origin, Max: origin.Add(mapSize)}
		keyOrigin := image.Pt(mapArea.Max.X+gap, body.Min.Y+(body.Dy()-keyHeight)/2)

		return dungeonViewLayout{
			header: header, mapArea: mapArea, mapGrid: image.Rect(0, 0, 0, 0), north: image.Rect(0, 0, 0, 0),
			legend: image.Rectangle{Min: keyOrigin, Max: keyOrigin.Add(image.Pt(keyWidth, keyHeight))},
			footer: footer, mapScale: scale, legendScale: keyScale,
			legendColumns: columns, legendRows: rows, rowHeight: rowHeight, textHeight: measure.textHeight,
			edgeWidth: measure.edgeWidth, edgeHeight: measure.edgeHeight, gap: gap,
		}.withGrid(gutter)
	}

	return dungeonViewLayoutBelow(header, footer, body, gutter, gap, measure)
}

func dungeonViewLayoutBelow(
	header, footer, body image.Rectangle,
	gutter image.Point,
	gap int,
	measure dungeonViewMeasurements,
) dungeonViewLayout {
	const columns = 2
	const nativeGrid = dungeonViewCellSize * dungeonViewFloorCells
	rows := (measure.entryCount + columns - 1) / columns
	rowHeight := max(dungeonViewCellSize, measure.compactLabelHeight)
	keyHeight := rows * rowHeight
	north := measure.edgeHeight + gap
	scale := max(1, min((body.Dy()-north-keyHeight-gap-gutter.Y*2)/nativeGrid, (body.Dx()-gutter.X*2)/nativeGrid))
	mapSize := image.Pt(nativeGrid*scale+gutter.X*2, nativeGrid*scale+gutter.Y*2)
	origin := image.Pt(body.Min.X+(body.Dx()-mapSize.X)/2, body.Min.Y+north)
	mapArea := image.Rectangle{Min: origin, Max: origin.Add(mapSize)}

	return dungeonViewLayout{
		header: header, mapArea: mapArea, mapGrid: image.Rect(0, 0, 0, 0), north: image.Rect(0, 0, 0, 0),
		legend: image.Rect(body.Min.X, mapArea.Max.Y+gap, body.Max.X, mapArea.Max.Y+gap+keyHeight),
		footer: footer, mapScale: scale, legendScale: 1,
		legendColumns: columns, legendRows: rows, rowHeight: rowHeight, textHeight: measure.textHeight,
		edgeWidth: measure.edgeWidth, edgeHeight: measure.edgeHeight, gap: gap,
	}.withGrid(gutter)
}

func (layout dungeonViewLayout) withGrid(gutter image.Point) dungeonViewLayout {
	layout.mapGrid = image.Rectangle{Min: layout.mapArea.Min.Add(gutter), Max: layout.mapArea.Max.Sub(gutter)}
	layout.north = image.Rect(layout.mapGrid.Min.X, layout.mapArea.Min.Y-layout.edgeHeight-layout.gap,
		layout.mapGrid.Max.X, layout.mapArea.Min.Y-layout.gap)

	return layout
}

// used is the area the map, its north marker and the key occupy.
func (layout dungeonViewLayout) used() image.Rectangle {
	return layout.mapArea.Union(layout.north).Union(layout.legend)
}

func (layout dungeonViewLayout) legendRow(index int) image.Rectangle {
	column, row := index/layout.legendRows, index%layout.legendRows
	columnWidth := (layout.legend.Dx() + layout.gap) / layout.legendColumns
	origin := image.Pt(layout.legend.Min.X+column*columnWidth, layout.legend.Min.Y+row*layout.rowHeight)

	return image.Rectangle{Min: origin, Max: origin.Add(image.Pt(columnWidth-layout.gap, layout.rowHeight))}
}

//nolint:mnd // Two plain labels are centered on the same row or column outside opposite edges.
func (layout dungeonViewLayout) continuationBounds(mark dungeonViewContinuation) [2]image.Rectangle {
	cellSize := dungeonViewCellSize * layout.mapScale
	if mark.axis == dungeonViewAxisHorizontal {
		top := layout.mapGrid.Min.Y + mark.index*cellSize + (cellSize-layout.edgeHeight)/2

		return [2]image.Rectangle{
			image.Rect(layout.mapArea.Min.X, top, layout.mapGrid.Min.X-layout.gap, top+layout.edgeHeight),
			image.Rect(layout.mapGrid.Max.X+layout.gap, top, layout.mapArea.Max.X, top+layout.edgeHeight),
		}
	}
	left := layout.mapGrid.Min.X + mark.index*cellSize + (cellSize-layout.edgeWidth)/2

	return [2]image.Rectangle{
		image.Rect(left, layout.mapArea.Min.Y, left+layout.edgeWidth, layout.mapGrid.Min.Y-layout.gap),
		image.Rect(left, layout.mapGrid.Max.Y+layout.gap, left+layout.edgeWidth, layout.mapArea.Max.Y),
	}
}
