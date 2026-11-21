package widgets

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"unicode/utf8"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

// A measured single line must not be rewrapped by an unrelated M-width budget.
func atlasSingleLine(output *text.Output, value string, colour color.Color) {
	output.Reset()
	output.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(value)))
	output.AppendLine().Text(value, colour).Done()
}

func (d *AtlasDialog) drawHeading(screen *ebiten.Image) {
	d.prepareHeading()
	point := image.Pt(d.mapRect.Min.X, d.mapRect.Min.X)
	width := d.width - point.X - point.X
	d.header.DrawPanelContent(screen, point, width, etext.AlignStart)
	d.zoom.DrawPanelContent(screen, point, width, etext.AlignEnd)
}

func (d *AtlasDialog) prepareHeading() {
	const title = "MAP OF BRITANNIA"
	atlasSingleLine(d.header, title, DialogHeaderColor)
	d.zoom.Reset()
	line := d.zoom.AppendLine().Text("Zoom: ", DialogLabelColor).Textf("%dx", DialogValueColor, d.view.Zoom)
	if d.panning {
		line.Text("   PAN MODE", DialogLabelColor)
	}
	line.Done()
}

func (d *AtlasDialog) drawPlacesHeading(screen *ebiten.Image) {
	const legendGap = 4
	d.preparePlacesHeading()
	d.panelHeader.DrawPanelContent(screen, d.placesHeaderRect.Min, d.placesHeaderRect.Dx(), etext.AlignStart)
	cell := d.atlasSymbolCell()
	center := image.Pt(d.placesHeaderRect.Min.X+cell/2, d.placesHeaderRect.Min.Y+d.rowHeight+d.rowHeight/2)
	drawAtlasStamp(screen, atlasCoordinateStamp(), center, ClassicUIPixelUnit(d.width, d.height), DialogValueColor, false)
	atlasSingleLine(d.row, "Coordinates recorded", DialogControlHintColor)
	d.row.DrawPanelContent(screen, image.Pt(center.X+cell/2+legendGap, d.placesHeaderRect.Min.Y+d.rowHeight),
		d.placesHeaderRect.Dx()-cell-legendGap, etext.AlignStart)
}

func (d *AtlasDialog) preparePlacesHeading() {
	prefix := "PLACES  "
	prefixWidth, _ := d.font.MeasureString(prefix)
	value := FitDialogLine(fmt.Sprintf("%d total, %d mapped", len(d.entries), d.mappedCount()), d.font,
		d.placesHeaderRect.Dx()-int(math.Ceil(prefixWidth)))
	d.panelHeader.Reset()
	d.panelHeader.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(prefix+value)))
	d.panelHeader.AppendLine().Text(prefix, DialogHeaderColor).Text(value, DialogValueColor).Done()
}

func (d *AtlasDialog) drawOverviewRow(screen *ebiten.Image, rect image.Rectangle, selected bool) {
	const separatorTextInset = 2

	colour, prefix := DialogValueColor, "  "
	if selected {
		colour, prefix = DialogSelectedText, "> "
	}
	fillAtlasRect(screen, image.Rect(rect.Min.X, rect.Min.Y, rect.Max.X, rect.Min.Y+1), DialogRuleLightColor)
	atlasSingleLine(d.row, fmt.Sprintf("%sAll mapped (%d)", prefix, d.mappedCount()), colour)
	d.row.DrawPanelContent(atlasSubImage(screen, rect), rect.Min.Add(image.Pt(0, separatorTextInset)),
		rect.Dx(), etext.AlignStart)
}

// All categories are always visible. Labels are measured before placing cells;
// compact custom layouts can wrap, while supported landscape sizes use one row.
//
//nolint:mnd // Half a standard margin separates measured category cells.
func (d *AtlasDialog) layoutCategoryTabs(margin int) int {
	padding := margin / 2
	left, top := margin, margin+int(math.Ceil(d.header.GetTotalHeightForLines(1)))+2
	for _, choice := range atlasTabOrder() {
		width, _ := d.font.MeasureString(d.atlasTabLabel(choice))
		cellWidth := int(math.Ceil(width)) + margin
		if left > margin && left+cellWidth > d.width-margin {
			left = margin
			top += d.rowHeight + padding
		}
		d.tabs[choice] = image.Rect(left, top, left+cellWidth, top+d.rowHeight)
		left += cellWidth + padding
	}

	return top + d.rowHeight
}

func (d *AtlasDialog) drawCategoryTabs(screen *ebiten.Image) {
	for _, choice := range atlasTabOrder() {
		rect := d.tabs[choice]
		colour := DialogTabColor
		if choice == journal.AtlasNotePlaces {
			colour = atlasNoteColor()
		}
		if choice == d.view.Filters.Places {
			colour = DialogActiveTabText
			width, _ := d.font.MeasureString(d.atlasTabLabel(choice))
			DrawActiveTabPlate(screen, rect, width, rect.Min.Y, int(math.Ceil(d.tabText.GetFontHeight())),
				getClassicUIPixelUnit(d.width, d.height))
		}
		atlasSingleLine(d.tabText, d.atlasTabLabel(choice), colour)
		d.tabText.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignCenter)
	}
}

// atlasTabOrder offers everything, then the player's own notes, then the place
// families. It is display order only; filter values keep their meaning.
func atlasTabOrder() [journal.AtlasPlaceFilterCount]journal.AtlasPlaceFilter {
	return [journal.AtlasPlaceFilterCount]journal.AtlasPlaceFilter{
		journal.AtlasAllPlaces, journal.AtlasNotePlaces, journal.AtlasTownes, journal.AtlasKeepPlaces,
		journal.AtlasCastlePlaces, journal.AtlasShrinePlaces, journal.AtlasLighthousePlaces,
		journal.AtlasDungeonPlaces, journal.AtlasMoonstoneSites, journal.AtlasVesselPlaces,
	}
}

// The notes tab carries its count, so a player sees at a glance that notes exist.
func (d *AtlasDialog) atlasTabLabel(choice journal.AtlasPlaceFilter) string {
	if choice != journal.AtlasNotePlaces {
		return choice.String()
	}

	return fmt.Sprintf("%s (%d)", choice, d.noteCount())
}

func (d *AtlasDialog) noteCount() int {
	count := 0
	for _, entry := range d.dossier.Entries {
		if entry.Note.ID != 0 {
			count++
		}
	}

	return count
}

//nolint:mnd // Small label/symbol gaps are measured separately from the row's font.
func (d *AtlasDialog) drawPlaceRow(screen *ebiten.Image, rect image.Rectangle, entry journal.AtlasEntry,
	selected bool) {
	target := atlasSubImage(screen, rect)
	mark := "  "
	colour, symbolColour := atlasPlaceColor(entry), atlasPlaceColor(entry)
	if entry.Note.ID != 0 {
		symbolColour = atlasNoteColor()
	}
	if selected {
		mark, colour = "> ", DialogSelectedText
		symbolColour = DialogSelectedLabelColor
		if entry.Discovered {
			symbolColour = DialogSelectedCheckedMarkColor
		}
	}
	markWidth, _ := d.font.MeasureString(mark)
	cell := d.atlasSymbolCell()
	left := rect.Min.X + int(math.Ceil(markWidth)) + 4
	d.drawAtlasSymbol(target, entry, image.Pt(left+cell/2, rect.Min.Y+rect.Dy()/2), symbolColour)
	textTop := rect.Min.Y + max(0, (rect.Dy()-int(math.Ceil(d.row.GetTotalHeightForLines(1))))/2)
	atlasSingleLine(d.row, mark, colour)
	d.row.DrawPanelContent(target, image.Pt(rect.Min.X, textTop), rect.Dx(), etext.AlignStart)
	left += cell + 8
	name := FitDialogLine(entry.Name, d.font, rect.Max.X-left-cell-8)
	atlasSingleLine(d.row, name, colour)
	d.row.DrawPanelContent(target, image.Pt(left, textTop), rect.Max.X-left, etext.AlignStart)
	if _, ok := entry.PlotPosition(); ok {
		drawAtlasStamp(target, atlasCoordinateStamp(), image.Pt(rect.Max.X-cell/2-4, rect.Min.Y+rect.Dy()/2),
			ClassicUIPixelUnit(d.width, d.height), symbolColour, false)
	}
}

func (d *AtlasDialog) drawPositionStatus(screen *ebiten.Image) {
	d.buildPositionStatus()
	d.position.DrawPanelContent(screen, d.positionRect.Min, d.positionRect.Dx(), etext.AlignStart)
	d.row.DrawPanelContent(screen, d.statusRect.Min, d.statusRect.Dx(), etext.AlignStart)
}

// Reserve the same measured worst-case block across fix/policy changes.
// Only list/details yield space; geography and the two help rows stay fixed.
//
//nolint:mnd // Two content rows plus the header/context rows and half-margin gap.
func (d *AtlasDialog) layoutPositionStatus(right, margin int) {
	chars := max(1, d.font.CharsForWidth(d.width-margin-right))
	d.position.SetMaxCharsPerLine(chars)
	d.row.SetMaxCharsPerLine(chars)
	positionLines, statusLines := 3, 4
	for _, context := range []string{"Within this settlement", "Within Empath Abbey", "Within Buccaneer's Den",
		"Within Lord British's Castle", "Within Blackthorn's Castle"} {
		d.position.Reset()
		d.position.AppendLine().Text(context, DialogValueColor).Done()
		positionLines = max(positionLines, 2+d.position.GetPanelContentLineCount())
	}
	for _, reason := range []string{"Night only: 19:00-05:59.", "Use on the world map.",
		"Unavailable underground.", "No sextant carried.", "Measure this place to locate it from the list.",
		"Selected place label shown.", "Selected place label hidden.", "Zoom in to pan."} {
		d.row.Reset()
		d.row.AppendLine().Text(reason, DialogControlHintColor).Done()
		statusLines = max(statusLines, d.row.GetPanelContentLineCount())
		positionLines = max(positionLines, 1+d.row.GetPanelContentLineCount())
	}
	statusHeight := int(math.Ceil(d.row.GetTotalHeightForLines(statusLines)))
	positionHeight := int(math.Ceil(d.position.GetTotalHeightForLines(positionLines)))
	notesHeight := int(math.Ceil(d.row.GetTotalHeightForLines(atlasNotesBarRows))) + margin
	d.statusRect = image.Rect(right, d.mapRect.Max.Y-statusHeight, d.width-margin, d.mapRect.Max.Y)
	d.notesRect = image.Rect(right, d.statusRect.Min.Y-notesHeight-margin/2,
		d.width-margin, d.statusRect.Min.Y-margin/2)
	d.positionRect = image.Rect(right, d.notesRect.Min.Y-positionHeight-margin/2,
		d.width-margin, d.notesRect.Min.Y-margin/2)
}

// Both buffers wrap complete values using measured normal body typography.
func (d *AtlasDialog) buildPositionStatus() {
	d.position.Reset()
	d.position.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.positionRect.Dx())))
	if d.dossier.CurrentFix.Measured {
		d.position.AppendLine().Text("YOUR POSITION", DialogHeaderColor).Done()
		d.position.AppendLine().Text(atlasCoordinates(d.dossier.CurrentFix.Location), DialogValueColor).Done()
		if d.dossier.FixContext != "" {
			d.position.AppendLine().Text(d.dossier.FixContext, DialogValueColor).Done()
		}
	} else {
		d.position.AppendLine().Text("POSITION UNAVAILABLE", DialogHeaderColor).Done()
		d.position.AppendLine().Text(d.dossier.FixUnavailable, DialogValueColor).Done()
	}
	d.row.Reset()
	d.row.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(d.statusRect.Dx())))
	d.row.AppendLine().Text(d.status, DialogControlHintColor).Done()
}

// Help stays in two physical rows: describe only the active pane's actions.
// Alternate close shortcuts and conditional detail paging live outside this footer.
func (d *AtlasDialog) helpRows() [2][]ControlHint {
	category, selection := ControlHint{hintKeyTabCycle, labelCategory}, ControlHint{hintKeyUpDown, hintActionSelect}
	zoom, world := ControlHint{"+/-", "Zoom"}, ControlHint{"0", "World"}
	first := []ControlHint{category, selection, {hintKeyEnter, "Show"}, zoom, world}
	if d.view.Overview {
		first = []ControlHint{category, selection, zoom, world}
	}
	pan := "Pan"
	if d.panning {
		first = []ControlHint{category, {"Arrows", "Pan"}, zoom, world}
		pan = "Places"
	}
	second := []ControlHint{{"P", pan}, {"L", "Label"}, {"G", "Grid"}}
	if d.view.Overview {
		second[1].Action = "Labels"
	}
	if d.dossier.CurrentFix.Measured {
		second = append(second, ControlHint{"A", "Avatar"})
	}
	second = append(second, ControlHint{hintKeyEsc, hintActionClose})

	return [2][]ControlHint{first, second}
}

func (d *AtlasDialog) buildFooter() {
	d.footer.Reset()
	for _, hints := range d.helpRows() {
		PackControlHints(d.footer, hints, d.footerRect.Dx(), etext.AlignStart)
	}
}

const (
	atlasNotesBarRows       = 2
	atlasNotesBarHalf       = 2
	atlasNotesBarInsetUnits = 2
	atlasNotesBarSeparator  = "  "
)

// drawNotesBar keeps the note keys in one framed place. A key that cannot be
// used is dimmed and the second row says why in words.
func (d *AtlasDialog) drawNotesBar(screen *ebiten.Image) {
	if d.onNoteIntent == nil {
		return // A read-only Map has no notes to author.
	}
	unit := ClassicUIPixelUnit(d.width, d.height)
	DrawClassicPixelRoundedFrame(screen, d.notesRect, unit, atlasNoteColor())
	canMark, canEdit := d.dossier.CurrentFix.Measured, d.selectedNoteID() != 0
	hints := []struct {
		hint      ControlHint
		available bool
	}{{ControlHint{"N", "New here"}, canMark}, {ControlHint{"E", "Edit"}, canEdit}, {ControlHint{"D", "Delete"}, canEdit}}
	keys := make([]ControlHint, 0, len(hints))
	for _, entry := range hints {
		keys = append(keys, entry.hint)
	}
	plain := joinControlHints(keys, atlasNotesBarSeparator)
	heading := "NOTES" + atlasNotesBarSeparator
	inset := atlasNotesBarInsetUnits * unit
	width := d.notesRect.Dx() - inset - inset
	if measured, _ := d.font.MeasureString(heading + plain); measured > float64(width) {
		heading = "" // The magenta frame still says whose keys these are.
	}
	d.row.Reset()
	d.row.SetMaxCharsPerLine(max(1, utf8.RuneCountInString(heading+plain)))
	line := d.row.AppendLine().Text(heading, atlasNoteColor())
	for index, entry := range hints {
		keyColour, actionColour := DialogControlKeyColor, DialogControlHintColor
		if !entry.available {
			keyColour, actionColour = DialogEmptyColor, DialogEmptyColor
		}
		if index > 0 {
			line.Text(atlasNotesBarSeparator, actionColour)
		}
		line.Text("["+entry.hint.Key+"]", keyColour).Text(" "+entry.hint.Action, actionColour)
	}
	line.Done()
	d.row.SetMaxCharsPerLine(max(1, d.font.CharsForWidth(width)))
	switch {
	case !canMark:
		d.row.AppendLine().Text("N: "+d.dossier.FixUnavailable, DialogControlHintColor).Done()
	case !canEdit:
		d.row.AppendLine().Text("E, D: select one of your notes.", DialogControlHintColor).Done()
	}
	// Centre what is written: the keys alone, or the keys and their reason.
	written := int(math.Ceil(d.row.GetTotalHeightForLines(d.row.GetPanelContentLineCount())))
	top := max(unit, (d.notesRect.Dy()-written)/atlasNotesBarHalf)
	d.row.DrawPanelContent(screen, d.notesRect.Min.Add(image.Pt(inset, top)), width, etext.AlignStart)
}
