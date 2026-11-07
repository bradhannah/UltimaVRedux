package widgets

import (
	"fmt"
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func moonstoneJournalDossier() game_state.QuestJournalDossier {
	dossier := testQuestJournalDossier()
	for phase, origin := range references.ChartedMoonstoneOrigins() {
		dossier.Moonstones[phase] = journal.MoonstoneEntry{Phase: datetime.MoonPhase(phase), ChartedOrigin: origin.Label}
	}
	return dossier
}

func moonstoneTestTime(hour, minute int) journal.OptionalGameTime {
	return journal.OptionalGameTime{Known: true, Time: journal.GameTime{Year: 139, Month: 4, Day: 12, Hour: hour, Minute: minute}}
}

func moonstoneDetailText(entry journal.MoonstoneEntry) string {
	var result strings.Builder
	for _, line := range questJournalMoonstoneDetail(entry) {
		result.WriteString(line.label + ": " + line.value + "\n")
	}
	return result.String()
}

func TestQuestJournalMoonstoneDetailDisclosureAndSurveyStates(t *testing.T) {
	entry := moonstoneJournalDossier().Moonstones[0]
	assert.Equal(t, "Charted origin: Near Moonglow\nCurrent destination: Not personally confirmed\n", moonstoneDetailText(entry))
	entry.Known, entry.Placed = true, true
	entry.FirstSource, entry.DiscoveredAt = journal.MoonstoneSourceGate, moonstoneTestTime(12, 0)
	entry.ObservedAt = moonstoneTestTime(19, 2)
	unmeasured := moonstoneDetailText(entry)
	assert.Contains(t, unmeasured, "Status: Placed (last observed)")
	assert.Contains(t, unmeasured, "Coordinates: Not measured")
	assert.Contains(t, unmeasured, moonstoneSurveyHelp)
	assert.Contains(t, unmeasured, "First discovered: 4-12-139 12:00")
	assert.Contains(t, unmeasured, "Last observed: 4-12-139 19:02")
	entry.LastObservedSurvey = journal.SurveyedLocation{Measured: true,
		Location: journal.SiteLocation{Map: references.BritanniaUnderworld, Floor: 0, X: 151, Y: 173}, RecordedAt: moonstoneTestTime(19, 1)}
	entry.SurveySource = journal.MoonstoneSourceLook
	measured := moonstoneDetailText(entry)
	coordinates := references.FormatBritannianCoordinates(151, 173)
	assert.Contains(t, measured, "Last-observed placement: Britannia surface (floor 0) · "+coordinates)
	assert.Contains(t, measured, "Last-observed placement measured: 4-12-139 19:01")
	assert.Contains(t, measured, "Last-observed placement method: Recognized a moongate")
	assert.NotContains(t, measured, moonstoneUnmeasured)
	entry.HistoricalSurvey, entry.LastObservedSurvey = entry.LastObservedSurvey, journal.SurveyedLocation{}
	entry.OriginalSiteRetired = true
	relocated := moonstoneDetailText(entry)
	assert.Contains(t, relocated, "Coordinates: Not measured")
	assert.NotContains(t, relocated, coordinates)
	assert.NotContains(t, relocated, "origin")
	assert.NotContains(t, relocated, "Last-observed placement: Britannia")
	entry.Carried, entry.Placed = true, false
	carried := moonstoneDetailText(entry)
	assert.Contains(t, carried, "Status: Carried by the party")
	assert.Contains(t, carried, "Gate destination: None while carried")
	assert.NotContains(t, carried, coordinates)
	entry.HistoricalSurvey, entry.DiscoveredAt, entry.ObservedAt = journal.SurveyedLocation{}, journal.OptionalGameTime{}, journal.OptionalGameTime{}
	entry.FirstSource = journal.MoonstoneSourcePossession
	imported := moonstoneDetailText(entry)
	assert.Contains(t, imported, "Discovery source: Known from possession")
	assert.Contains(t, imported, "First discovered: Not recorded")
	assert.NotContains(t, imported, coordinates)
}

func TestQuestJournalMoonstoneGlyphRowsHeadingPagingAndEightTabs(t *testing.T) {
	for _, size := range append(config.SupportedWindowResolutions(), config.ScreenResolution{X: 320, Y: 200}) {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setQuestJournalTestResolution(t, size.X, size.Y)
			dossier := moonstoneJournalDossier()
			for phase := range dossier.Moonstones {
				entry := &dossier.Moonstones[phase]
				entry.Known, entry.Placed = true, true
				entry.FirstSource, entry.DiscoveredAt = journal.MoonstoneSourceGate, moonstoneTestTime(12, 0)
				entry.ObservedAt = moonstoneTestTime(19, 2)
				entry.HistoricalSurvey = journal.SurveyedLocation{Measured: true,
					Location: journal.SiteLocation{Map: references.BritanniaUnderworld, X: 151, Y: 173}, RecordedAt: moonstoneTestTime(19, 1)}
				entry.SurveySource = journal.MoonstoneSourcePickup
			}
			keyboard := newWidgetTestKeyboard(0)
			d := NewQuestJournalDialog(dossier, testQuestJournalConfig(keyboard))
			t.Cleanup(d.outerBorder.Dispose)
			dossier.Moonstones[0].ChartedOrigin = "Changed"
			assert.Equal(t, "Near Moonglow", d.dossier.Moonstones[0].ChartedOrigin)
			d.switchTabBy(-2)
			require.Equal(t, QuestJournalMoonstonesTab, d.GetActiveTab())
			require.Equal(t, 8, d.currentItemCount())
			for tab, rect := range d.layout.tabRects {
				require.False(t, rect.Empty(), "tab %d", tab)
				require.True(t, rect.In(d.layout.tabsRect), "tab %d", tab)
			}
			rect := d.listContentRect()
			rect.Max.Y = rect.Min.Y + d.rowHeight()
			for phase := range 8 {
				d.selections[QuestJournalMoonstonesTab] = phase
				d.cachedDetailPages = nil
				for _, selected := range []bool{false, true} {
					foreground := DialogValueColor
					if selected {
						foreground = DialogSelectedText
					}
					line := moonstoneTitleLine(d.moonstoneRowOutput, datetime.MoonPhase(phase), "> ", foreground)
					segments := line.GetSegments()
					require.Len(t, segments, 3)
					assert.Equal(t, "> ", segments[0].Text)
					assert.Equal(t, string(rune(0x30+phase)), segments[1].Text)
					assert.Equal(t, text.FontTypeRunic, segments[1].FontType)
					assert.Equal(t, " "+datetime.MoonPhase(phase).String(), segments[2].Text)
					for _, segment := range segments {
						assert.Equal(t, foreground, segment.Color)
					}
					assert.LessOrEqual(t, d.moonstoneRowOutput.MeasureLineSegments(segments), float64(rect.Dx()))
					line.Done()
					d.prepareMoonstoneRow(phase, selected, foreground)
					assert.Equal(t, 1, d.moonstoneRowOutput.GetPanelContentLineCount())
					assert.Contains(t, d.moonstoneRowOutput.GetOutputStr(false), datetime.MoonPhase(phase).String())
				}
				capacity := d.detailOutput.GetLineCapacity(d.detailContentRect().Dy())
				require.Positive(t, capacity)
				for page, lines := range d.detailPages() {
					d.detailPage = page
					assert.LessOrEqual(t, d.detailLineCount(lines), capacity)
					d.prepareMoonstoneHeading()
					assert.Equal(t, string(rune(0x30+phase))+" "+datetime.MoonPhase(phase).String(), d.moonstoneHeadingOutput.GetOutputStr(false))
				}
			}
			pressQuestJournalKey(d, keyboard, ebiten.KeyHome)
			assert.Zero(t, d.GetSelectedIndex())
			pressQuestJournalKey(d, keyboard, ebiten.KeyEnd)
			assert.Equal(t, 7, d.GetSelectedIndex())
			pressQuestJournalKey(d, keyboard, ebiten.KeyTab)
			assert.Equal(t, QuestJournalNotesTab, d.GetActiveTab())
			d.switchTabBy(-1)
			assert.Equal(t, 7, d.GetSelectedIndex())
			pressQuestJournalKey(d, keyboard, ebiten.KeyEnter)
			assert.Equal(t, QuestJournalMoonstonesTab, d.GetActiveTab())
			assert.False(t, d.IsDismissed())
		})
	}
}

func TestQuestJournalMoonstoneSearchOnlyCurrentMeasuredPublicFacts(t *testing.T) {
	setQuestJournalTestResolution(t, 320, 200)
	dossier := moonstoneJournalDossier()
	entry := &dossier.Moonstones[1]
	entry.Known, entry.Placed = true, true
	entry.FirstSource, entry.DiscoveredAt, entry.ObservedAt = journal.MoonstoneSourceBurial, moonstoneTestTime(5, 0), moonstoneTestTime(12, 0)
	entry.LastObservedSurvey = journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{Map: references.BritanniaUnderworld, X: 151, Y: 173}, RecordedAt: moonstoneTestTime(5, 0)}
	entry.SurveySource = journal.MoonstoneSourceBurial
	d := NewQuestJournalDialog(dossier, testQuestJournalConfig(newWidgetTestKeyboard(0)))
	moonstoneRows := 0
	for _, row := range d.searchEntries {
		if row.tab == QuestJournalMoonstonesTab {
			moonstoneRows++
		}
	}
	require.Equal(t, 8, moonstoneRows)
	for _, hidden := range []string{"Not personally confirmed", "Not measured", "Not recorded", "Requires a usable sextant", "224", "133", "Observer", "HasSextant"} {
		d.setSearchQuery(hidden)
		assert.Empty(t, d.searchResults, hidden)
	}
	for phase, origin := range references.ChartedMoonstoneOrigins() {
		d.setSearchQuery(origin.Label)
		require.Len(t, d.searchResults, 1)
		result := d.searchEntries[d.searchResults[0]]
		assert.Equal(t, QuestJournalMoonstonesTab, result.tab)
		assert.Equal(t, phase, result.row)
	}
	d.setSearchQuery("Crescent Waxing")
	require.Len(t, d.searchResults, 1)
	d.setSearchQuery(references.FormatBritannianCoordinates(151, 173))
	require.Len(t, d.searchResults, 1)
	result := d.searchEntries[d.searchResults[0]]
	assert.Contains(t, result.body, "Last-observed placement")
	// A compact detail viewport forces the matching survey onto a later page.
	d.layout.detailRect.Max.Y = d.layout.detailRect.Min.Y + 7*d.rowHeight()
	d.openSearchResult()
	assert.Equal(t, QuestJournalMoonstonesTab, d.GetActiveTab())
	assert.Equal(t, 1, d.GetSelectedIndex())
	assert.Positive(t, d.GetDetailPage(), "opens the survey's detail page")
	var visible strings.Builder
	for _, line := range d.detailPages()[d.detailPage] {
		visible.WriteString(line.label + " " + line.value)
	}
	assert.Contains(t, visible.String(), references.FormatBritannianCoordinates(151, 173))
	d.beginSearch()
	d.setSearchQuery("Last-observed placement")
	d.openSearchResult()
	assert.Positive(t, d.GetDetailPage(), "matching field labels also reveal their page")
	assert.Equal(t, dossier.Moonstones, d.dossier.Moonstones, "reading/searching never changes the snapshot")
}

func TestQuestJournalMoonstoneNewAndFullGlyphContrast(t *testing.T) {
	for _, selected := range []bool{false, true} {
		foreground, background := DialogValueColor, MerchantDialogBackgroundColor
		if selected {
			foreground, background = DialogSelectedText, DialogSelectedBackground
		}
		counts := make([]int, 2)
		for i, phase := range []datetime.MoonPhase{datetime.MoonPhaseNew, datetime.MoonPhaseFull} {
			screen := ebiten.NewImage(48, 48)
			screen.Fill(background)
			output := text.NewOutput(text.NewUltimaFont(20), 24, 1, 1)
			appendMoonPhaseGlyph(output.AppendLine(), phase, foreground).Done()
			output.DrawPanelContent(screen, image.Pt(0, 0), 48, etext.AlignStart)
			wantR, wantG, wantB, _ := foreground.RGBA()
			for y := range 48 {
				for x := range 48 {
					r, g, b, _ := screen.At(x, y).RGBA()
					if r == wantR && g == wantG && b == wantB {
						counts[i]++
					}
				}
			}
			screen.Dispose()
		}
		assert.Positive(t, counts[0], "New Moon's outline remains visible, selected=%v", selected)
		assert.Greater(t, counts[1], counts[0], "Full Moon must be visibly filled, selected=%v", selected)
	}
}

func TestSharedMoonPhaseGlyphUsesTypedIdentityAndActualBitmapAdvance(t *testing.T) {
	for _, spacing := range []float64{8, 16, 24, 40} {
		output := text.NewOutput(text.NewUltimaFont(20), spacing, 1, 1)
		for phase := datetime.MoonPhaseNew; phase <= datetime.MoonPhaseCrescentWaning; phase++ {
			for _, foreground := range []color.Color{DialogItemNameColor, DialogSelectedText, DialogHeaderColor} {
				output.Reset()
				line := appendMoonPhaseGlyph(output.AppendLine(), phase, foreground)
				segments := line.GetSegments()
				require.Len(t, segments, 1)
				require.Equal(t, text.FontTypeRunic, segments[0].FontType)
				require.Equal(t, string(rune(0x30+phase)), segments[0].Text)
				require.Equal(t, foreground, segments[0].Color)
				require.Equal(t, spacing, output.MeasureLineSegments(segments))
				line.Done()
				require.Equal(t, 1, output.GetPanelContentLineCount())
			}
		}
	}
	for _, invalid := range []datetime.MoonPhase{-1, 8} {
		output := text.NewOutput(text.NewUltimaFont(20), 24, 1, 1)
		require.PanicsWithValue(t, "invalid moonstone phase", func() {
			appendMoonPhaseGlyph(output.AppendLine(), invalid, DialogValueColor)
		})
	}
}
