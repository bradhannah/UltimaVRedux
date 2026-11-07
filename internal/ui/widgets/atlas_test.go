package widgets

import (
	"fmt"
	"image"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/atlasterrain"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

func atlasTestDossier() journal.AtlasDossier {
	return journal.AtlasDossier{FixUnavailable: "No sextant", Entries: []journal.AtlasEntry{
		{Key: "chart", Name: "Unnamed castle", Kind: "castle", Category: journal.AtlasCastles,
			Charted: true, Survey: atlasMeasuredSite(255, 0)},
		{Key: "known", Name: "Observed hut", Kind: "hut", Category: journal.AtlasSettlements},
	}}
}

func atlasMeasuredSite(x, y int) journal.SurveyedLocation {
	return journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{X: x, Y: y}}
}

func TestAtlasViewWrapUsesOneTransformAndFourTextureSlices(t *testing.T) {
	rect := image.Rect(20, 30, 532, 542)
	for _, zoom := range []int{1, 2, 4} {
		for _, center := range []float64{-1025, 0, 1, 128, 255, 1025} {
			view := NewAtlasViewState()
			view.Zoom, view.CenterX, view.CenterY = zoom, atlasModulo(center), atlasModulo(center)
			parts := view.textureRects(rect)
			require.LessOrEqual(t, len(parts), 4)
			area := 0
			for _, part := range parts {
				require.True(t, part.source.In(image.Rect(0, 0, atlasterrain.ImageSize, atlasterrain.ImageSize)))
				area += part.source.Dx() * part.source.Dy()
				x, y, visible := view.project(float64(part.source.Min.X)/16, float64(part.source.Min.Y)/16, rect)
				require.True(t, visible)
				require.InDelta(t, part.x, x, 0.001)
				require.InDelta(t, part.y, y, 0.001)
			}
			require.Equal(t, (4096/zoom)*(4096/zoom), area)
			if zoom == 1 {
				require.Len(t, parts, 1)
				require.Equal(t, image.Pt(0, 0), parts[0].source.Min)
			}
		}
	}
}

func TestAtlasReaderFocusFiltersAndNoStaleFix(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	keyboard := newWidgetTestKeyboard(0)
	view := AtlasViewState{}
	closed := 0
	dossier := atlasTestDossier()
	dialog := NewAtlasDialog(dossier, nil, keyboard, &view, func() { closed++ })
	require.Equal(t, 1, view.Zoom)
	require.Equal(t, AtlasLabelsSelected, view.Labels)
	require.Len(t, dialog.entries, 2)
	before := view
	dialog.handleKey(ebiten.KeyA)
	require.Equal(t, before, view)
	require.Equal(t, "No sextant", dialog.status)
	dialog.handleKey(ebiten.KeyEnter)
	require.Equal(t, 2, view.Zoom)
	require.Equal(t, float64(255), view.CenterX)
	dialog.navigate(ebiten.KeyDown)
	before = view
	dialog.handleKey(ebiten.KeyEnter)
	require.Equal(t, before, view, "unmeasured focus cannot consult private coordinates")
	require.Contains(t, dialog.status, "Measure this place")
	dossier.CurrentFix = journal.SurveyedLocation{Measured: true, Location: journal.SiteLocation{X: 7, Y: 9}}
	dialog.Refresh(dossier)
	dialog.handleKey(ebiten.KeyA)
	require.Equal(t, float64(7), view.CenterX)
	dossier.CurrentFix = journal.SurveyedLocation{}
	dialog.Refresh(dossier)
	before = view
	dialog.handleKey(ebiten.KeyA)
	require.Equal(t, before, view, "refresh must discard the former fix")
	keyboard.SimulateKeyPress(ebiten.KeyEscape)
	dialog.Update()
	require.Equal(t, 1, closed)
	dialog.close()
	require.Equal(t, 1, closed)
	NewAtlasDialog(dossier, nil, keyboard, &view, func() {})
	require.Equal(t, before, view, "reopening keeps scene preferences")
}

func TestAtlasExactChordBothShiftKeysAndExtraModifiers(t *testing.T) {
	for _, shift := range []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyShiftRight} {
		keys := map[ebiten.Key]bool{shift: true, ebiten.KeyM: true}
		keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return keys[key] })
		require.True(t, IsAtlasChord(keyboard))
		for _, extra := range []ebiten.Key{ebiten.KeyControlLeft, ebiten.KeyAltRight, ebiten.KeyMetaLeft} {
			keys[extra] = true
			require.False(t, IsAtlasChord(keyboard))
			delete(keys, extra)
		}
		delete(keys, shift)
		require.False(t, IsAtlasChord(keyboard))
	}
}

func TestAtlasAvatarShortcutKeyboardZoomAvailabilityAndFocus(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	for _, zoom := range []int{1, 2, 4} {
		for _, panning := range []bool{false, true} {
			for _, available := range []bool{false, true} {
				for _, focused := range []bool{false, true} {
					t.Run(fmt.Sprintf("zoom-%d-pan-%t-fix-%t-focus-%t", zoom, panning, available, focused), func(t *testing.T) {
						keyboard := newWidgetTestKeyboard(0)
						view := NewAtlasViewState()
						view.Zoom, view.CenterX, view.CenterY = zoom, 200, 210
						dossier := atlasTestDossier()
						dossier.CurrentFix = journal.SurveyedLocation{Measured: available, Location: journal.SiteLocation{X: 7, Y: 9}}
						dialog := NewAtlasDialog(dossier, nil, keyboard, &view, func() {})
						dialog.panning = panning
						dialog.onDialogFocusChanged(focused)
						dialog.buildFooter()
						if available {
							require.Contains(t, dialog.footer.GetOutputStr(false), "[A] Avatar")
						} else {
							require.NotContains(t, dialog.footer.GetOutputStr(false), "[A] Avatar")
						}
						before := view
						keyboard.SimulateKeyPress(ebiten.KeyA)
						dialog.Update()
						if available && focused {
							require.Equal(t, float64(7), view.CenterX)
							require.Equal(t, float64(9), view.CenterY)
							require.Equal(t, max(2, zoom), view.Zoom, "same minimum 2x focus as selecting a place")
							require.Equal(t, before.SelectedKey, view.SelectedKey, "avatar focus keeps the selected place")
						} else {
							require.Equal(t, before, view, "an unavailable/stale fix or unfocused modal cannot move the camera")
							if focused {
								require.Equal(t, dossier.FixUnavailable, dialog.status)
							}
						}
						require.Equal(t, panning, dialog.panning)
						require.Equal(t, dossier, dialog.dossier, "shortcut is read-only presentation")
					})
				}
			}
		}
	}
}

func TestAtlasLayoutsKeepRowsAndControlsInsideLogicalScreen(t *testing.T) {
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}, {3840, 2160}, {800, 600}} {
		setQuestJournalTestResolution(t, size.X, size.Y)
		view := NewAtlasViewState()
		dossier := atlasTestDossier()
		dossier.Entries[0].Name = strings.Repeat("Long displayed entry name ", 8)
		dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})
		bounds := image.Rectangle{Max: size}
		for _, rect := range []image.Rectangle{dialog.mapRect, dialog.listRect, dialog.detailRect, dialog.footerRect} {
			require.True(t, rect.In(bounds), "%v: %v", size, rect)
			require.Positive(t, rect.Dx())
			require.Positive(t, rect.Dy())
		}
		dialog.buildPositionStatus()
		dialog.buildFooter()
		require.LessOrEqual(t, dialog.row.GetPanelContentLineCount(), 1)
		require.LessOrEqual(t, dialog.footer.GetTotalHeightForLines(dialog.footer.GetPanelContentLineCount()), float64(dialog.footerRect.Dy()))
	}
}

func TestAtlasFocusSelectsAnEntryFromOutsideTheMap(t *testing.T) {
	setQuestJournalTestResolution(t, 1280, 720)
	view := NewAtlasViewState()
	view.Zoom = 2
	dossier := atlasTestDossier()
	dossier.Entries = append(dossier.Entries, journal.AtlasEntry{Key: "journal-note-3", Name: "Hermit hut",
		Kind: AtlasJournalNoteKind, Category: journal.AtlasNotes, Phase: -1, NameKnown: true,
		Note: journal.AtlasNoteRecord{ID: 3, Position: journal.SiteLocation{X: 40, Y: 30}, Label: "Hermit hut",
			Description: "North of Yew."}})
	dialog := NewAtlasDialog(dossier, nil, newWidgetTestKeyboard(0), &view, func() {})

	require.True(t, dialog.Focus("journal-note-3", journal.AtlasNotePlaces))
	require.Equal(t, journal.AtlasNotePlaces, view.Filters.Places)
	require.Equal(t, "journal-note-3", view.SelectedKey)
	require.Equal(t, []float64{40, 30, 2}, []float64{view.CenterX, view.CenterY, float64(view.Zoom)},
		"centred on the note; zoom stays the player's")
	require.Equal(t, [2]string{"Journal note", "Hermit hut"}, dialog.detailFields()[0])

	require.True(t, dialog.Focus("known", journal.AtlasPlaceFilter(journal.AtlasSettlements)+1))
	require.Equal(t, "Measure this place to locate it from the list.", dialog.status,
		"an unmeasured place is selected but never located")
	require.False(t, dialog.Focus("missing", journal.AtlasAllPlaces))
}
