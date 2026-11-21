package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/hajimehoshi/ebiten/v2"
	etext "github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/stretchr/testify/require"
)

func atlasStatusWords(output *text.Output) string {
	return strings.Join(strings.Fields(output.GetOutputStr(false)), " ")
}

func TestAtlasStatusCompleteAtEveryPreset(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setQuestJournalTestResolution(t, size.X, size.Y)
			view := NewAtlasViewState()
			dialog := NewAtlasDialog(journal.AtlasDossier{}, nil, newWidgetTestKeyboard(0), &view, func() {})
			mapRect, footerRect := dialog.mapRect, dialog.footerRect
			var screen *ebiten.Image
			if size.X == 1280 || size.X == 1366 {
				screen = ebiten.NewImageWithOptions(dialog.positionRect.Union(dialog.statusRect), nil)
				t.Cleanup(screen.Dispose)
			}
			for _, reason := range []string{"Only at night!", "Use on the world map.",
				"Unavailable underground.", "No sextant carried."} {
				dialog.Refresh(journal.AtlasDossier{FixUnavailable: reason})
				dialog.status = reason
				dialog.ensureLayout()
				dialog.buildPositionStatus()
				if screen != nil {
					screen.Clear()
					dialog.drawPositionStatus(screen)
				}
				require.Equal(t, "POSITION UNAVAILABLE "+reason, atlasStatusWords(dialog.position))
				require.Equal(t, reason, atlasStatusWords(dialog.row))
				assertAtlasStatusFits(t, dialog, screen)
				require.Equal(t, mapRect, dialog.mapRect)
				require.Equal(t, footerRect, dialog.footerRect)
			}
			for _, context := range []string{"", "Within Fogsbane", "Within this settlement",
				"Within Buccaneer's Den", "Within Empath Abbey", "Within Serpent's Hold",
				"Within Lord British's Castle", "Within Blackthorn's Castle"} {
				fix := journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{X: 255, Y: 255}}
				dialog.Refresh(journal.AtlasDossier{CurrentFix: fix, FixContext: context})
				dialog.status = "Take a sextant reading there to place it on the Map."
				dialog.buildPositionStatus()
				if screen != nil {
					screen.Clear()
					dialog.drawPositionStatus(screen)
				}
				want := strings.TrimSpace("YOUR POSITION " + atlasCoordinates(fix.Location) + " " + context)
				require.Equal(t, want, atlasStatusWords(dialog.position))
				require.Equal(t, dialog.status, atlasStatusWords(dialog.row))
				assertAtlasStatusFits(t, dialog, screen)
				require.Equal(t, mapRect, dialog.mapRect)
				dialog.buildFooter()
				require.Equal(t, 2, dialog.footer.GetPanelContentLineCount())
				require.Contains(t, dialog.footer.GetOutputStr(false), "[P] Pan")
			}
		})
	}
}

func assertAtlasStatusFits(t *testing.T, dialog *AtlasDialog, screen *ebiten.Image) {
	t.Helper()
	for _, block := range []struct {
		output *text.Output
		rect   image.Rectangle
	}{{dialog.position, dialog.positionRect}, {dialog.row, dialog.statusRect}} {
		count := block.output.GetPanelContentLineCount()
		require.LessOrEqual(t, int(math.Ceil(block.output.GetTotalHeightForLines(count))), block.rect.Dy())
		require.True(t, block.rect.In(image.Rect(0, 0, dialog.width, dialog.height)))
		require.NotContains(t, block.output.GetOutputStr(false), "...")
		// Render the retained complete physical rows independently without wrapping.
		// Equality proves every glyph is drawn inside the reserved block, including
		// continuation rows, rather than merely retained in an invisible buffer.
		expected := text.NewOutput(dialog.font, block.output.GetLineSpacing(), 128, 128)
		for index, line := range strings.Split(block.output.GetOutputStr(false), "\n") {
			width, _ := dialog.font.MeasureString(strings.TrimSpace(line))
			require.LessOrEqual(t, width, float64(block.rect.Dx()), "complete physical row must fit")
			colour := DialogControlHintColor
			if block.output == dialog.position {
				colour = DialogValueColor
				if index == 0 {
					colour = DialogHeaderColor
				}
			}
			expected.AppendLine().Text(strings.TrimSpace(line), colour).Done()
		}
		if screen == nil {
			continue
		}
		reference := ebiten.NewImage(block.rect.Dx(), block.rect.Dy())
		expected.DrawPanelContent(reference, image.Point{}, block.rect.Dx(), etext.AlignStart)
		require.Equal(t, atlasMarkerPixels(reference).Pix,
			atlasMarkerPixels(atlasSubImage(screen, block.rect)).Pix, "complete rendered font block")
		reference.Deallocate()
	}
	require.LessOrEqual(t, dialog.detailRect.Max.Y, dialog.positionRect.Min.Y)
	require.LessOrEqual(t, dialog.positionRect.Max.Y, dialog.statusRect.Min.Y)
	require.LessOrEqual(t, dialog.statusRect.Max.Y, dialog.footerRect.Min.Y)
}

func TestAtlasStatusRelayoutAfterResize(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	dialog := NewAtlasDialog(journal.AtlasDossier{FixUnavailable: "Only at night!"},
		nil, newWidgetTestKeyboard(0), &view, func() {})
	before := dialog.positionRect
	setQuestJournalTestResolution(t, 1920, 1080)
	dialog.ensureLayout()
	require.NotEqual(t, before, dialog.positionRect)
	dialog.buildPositionStatus()
	require.Equal(t, "POSITION UNAVAILABLE Only at night!", atlasStatusWords(dialog.position))
	dialog.Refresh(journal.AtlasDossier{CurrentFix: journal.SurveyedLocation{Measured: true}, FixContext: "Within this settlement"})
	dialog.buildPositionStatus()
	require.Contains(t, atlasStatusWords(dialog.position), "Within this settlement")
}
