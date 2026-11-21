package widgets

import (
	"fmt"
	"image"
	stdcolor "image/color"
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
)

const (
	moonstoneUnconfirmed = "Not personally confirmed"
	moonstoneUnmeasured  = "Not measured"
	moonstoneUnrecorded  = "Not recorded"
	moonstoneSurveyHelp  = "Needs a sextant reading there, at night, on the surface."
)

func moonstonePlaceholder(value string) bool {
	return value == moonstoneUnconfirmed || value == moonstoneUnmeasured ||
		value == moonstoneUnrecorded || value == moonstoneSurveyHelp
}

func questJournalMoonstoneDetail(entry journal.MoonstoneEntry) []detailLine {
	var lines []detailLine
	if !entry.OriginalSiteRetired && entry.ChartedOrigin != "" {
		lines = append(lines, questJournalLearnedField("Charted origin", entry.ChartedOrigin))
	}
	if !entry.Known {
		return append(lines, questJournalLearnedField("Current destination", moonstoneUnconfirmed))
	}
	switch {
	case entry.Carried:
		lines = append(lines, questJournalLearnedField("Status", "Carried by the party"),
			questJournalLearnedField("Gate destination", "None while carried"))
	case entry.Placed:
		lines = append(lines, questJournalLearnedField("Status", "Placed (last observed)"))
	default:
		lines = append(lines, questJournalLearnedField("Current destination", moonstoneUnconfirmed))
	}
	if entry.Placed {
		if entry.LastObservedSurvey.Measured {
			lines = append(lines,
				moonstoneSurveyLines("Last-observed placement", entry.LastObservedSurvey, entry.SurveySource)...)
		} else {
			lines = append(lines, questJournalLearnedField("Coordinates", moonstoneUnmeasured),
				detailLine{label: "", value: moonstoneSurveyHelp, color: DialogNavHintColor, header: false})
		}
	}
	lines = append(lines, questJournalLearnedField("Discovery source", entry.FirstSource.String()),
		questJournalLearnedField("First discovered", moonstoneDate(entry.DiscoveredAt)),
		questJournalLearnedField("Last observed", moonstoneDate(entry.ObservedAt)))

	return lines
}

func moonstoneSurveyLines(
	label string, survey journal.SurveyedLocation, source journal.MoonstoneSource,
) []detailLine {
	site := survey.Location
	world := site.Map.String()
	if site.Map == references.BritanniaUnderworld {
		world = "Britannia surface"
		if site.Floor == references.FloorNumber(references.UNDERWORLD) {
			world = "Underworld"
		}
	}
	//nolint:gosec // Measured locations are validated map coordinates (0..255).
	coordinates := references.FormatBritannianCoordinates(references.Coordinate(site.X), references.Coordinate(site.Y))

	return []detailLine{
		questJournalLearnedField(label, fmt.Sprintf("%s (floor %d) · %s", world, site.Floor, coordinates)),
		questJournalLearnedField(label+" measured", moonstoneDate(survey.RecordedAt)),
		questJournalLearnedField(label+" method", source.String()),
	}
}

func moonstoneDate(at journal.OptionalGameTime) string {
	if !at.Known {
		return moonstoneUnrecorded
	}

	return fmt.Sprintf("%d-%d-%d %02d:%02d", at.Time.Month, at.Time.Day, at.Time.Year, at.Time.Hour, at.Time.Minute)
}

func moonstoneTitleLine(
	output *text.Output, phase datetime.MoonPhase, marker string, foreground stdcolor.Color,
) *text.LineBuilder {
	output.Reset()
	// Budget the entire mixed row before insertion: a one-row buffer must never
	// wrap away its marker/glyph prefix. Width is measured separately below.
	output.SetMaxCharsPerLine(len(phase.String()) + len(marker) + len("0 "))

	return appendMoonPhaseGlyph(output.AppendLine().Text(marker, foreground), phase, foreground).
		Text(" "+phase.String(), foreground)
}

func fittedMoonstoneOutput(size float64, width int, marker string) *text.Output {
	output := text.NewOutput(text.NewUltimaFont(size), size*questJournalLineSpacingMultiplier,
		1, questJournalMinimumTextChars)
	for {
		fits := true
		for phase := datetime.MoonPhaseNew; phase <= datetime.MoonPhaseCrescentWaning; phase++ {
			line := moonstoneTitleLine(output, phase, marker, DialogValueColor)
			fits = fits && output.MeasureLineSegments(line.GetSegments()) <= float64(width)
			line.Done()
		}
		if fits || size <= 1 {
			return output
		}
		size--
		output.SetFont(text.NewUltimaFont(size), size*questJournalLineSpacingMultiplier)
	}
}

func (d *QuestJournalDialog) rebuildMoonstoneOutputs() {
	d.moonstoneRowOutput = fittedMoonstoneOutput(d.font.TextFace.Size, d.listContentRect().Dx(), "> ")
	content := insetRectangle(d.layout.detailRect, d.layout.padding*questJournalPaneInsetMultiplier)
	d.moonstoneHeadingOutput = fittedMoonstoneOutput(d.titleOutput.Font.TextFace.Size, content.Dx(), "")
}

func (d *QuestJournalDialog) drawMoonstoneRow(
	screen *ebiten.Image, rect image.Rectangle, index int, selected bool, foreground stdcolor.Color,
) {
	d.prepareMoonstoneRow(index, selected, foreground)
	d.moonstoneRowOutput.DrawPanelContent(screen, rect.Min, rect.Dx(), etext.AlignStart)
}

func (d *QuestJournalDialog) prepareMoonstoneRow(index int, selected bool, foreground stdcolor.Color) {
	marker := "  "
	if selected {
		marker = "> "
	}
	moonstoneTitleLine(d.moonstoneRowOutput, d.dossier.Moonstones[index].Phase, marker, foreground).Done()
}

func (d *QuestJournalDialog) drawMoonstoneHeading(screen *ebiten.Image) {
	content := d.prepareMoonstoneHeading()
	d.moonstoneHeadingOutput.DrawPanelContent(screen, content.Min, content.Dx(), etext.AlignStart)
}

func (d *QuestJournalDialog) prepareMoonstoneHeading() image.Rectangle {
	content := insetRectangle(d.layout.detailRect, d.layout.padding*questJournalPaneInsetMultiplier)
	content.Max.Y = content.Min.Y + int(math.Ceil(d.moonstoneHeadingOutput.GetLineSpacing()))
	phase := d.dossier.Moonstones[d.selections[QuestJournalMoonstonesTab]].Phase
	moonstoneTitleLine(d.moonstoneHeadingOutput, phase, "", DialogHeaderColor).Done()

	return content
}
