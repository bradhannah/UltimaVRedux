package widgets

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/atlasterrain"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

// This is bounded production-renderer acceptance, not an input/playthrough test.
// Assertions always run; only PNG writing requires ULTIMA5_BATCH_REVIEW_DIR.
func TestBatchReviewAtlasAndEditors(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.BritanniaUnderworld).
		WithSystemCallbacks().WithRandomSeed(1).Build()
	state.PartyState.Inventory.QuestItems.Set(references.Sextant, 0)
	dossier := state.CurrentAtlasDossier()
	require.False(t, dossier.CurrentFix.Measured)
	require.NotEmpty(t, dossier.Symbols)
	terrain, err := atlasterrain.Capture(func(x, y int) int {
		return int(state.GameReferences.OverworldLargeMapReference.GetSpriteIndex(references.Coordinate(x), references.Coordinate(y)))
	})
	require.NoError(t, err)
	normalized, _, err := atlasterrain.Normalize(terrain)
	require.NoError(t, err)
	geography, err := atlasterrain.Compose(normalized, sprites.TileAtlasPNG())
	require.NoError(t, err)
	texture := ebiten.NewImageFromImage(geography)
	t.Cleanup(texture.Deallocate)
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}} {
		t.Run(fmt.Sprintf("%dp", size.Y), func(t *testing.T) {
			setGuidanceTestResolution(t, size.X, size.Y)
			view := NewAtlasViewState()
			dialog := NewAtlasDialog(dossier, texture, newWidgetTestKeyboard(0), &view, func() {})
			dialog.SetNoteIntentHandler(func(AtlasNoteIntent) {})
			screen := ebiten.NewImage(size.X, size.Y)
			t.Cleanup(screen.Deallocate)
			dialog.Draw(screen)
			require.Zero(t, dialog.mappedCount())
			require.Empty(t, dialog.mapCallouts())
			require.Contains(t, dialog.position.GetOutputStr(false), "sextant")
			// Supplied icons must contribute actual map pixels without a sextant.
			withSymbols := atlasMarkerPixels(atlasSubImage(screen, dialog.mapRect))
			dialog.dossier.Symbols = nil
			dialog.Draw(screen)
			require.NotEqual(t, withSymbols.Pix, atlasMarkerPixels(atlasSubImage(screen, dialog.mapRect)).Pix)
			dialog.Refresh(dossier)
			dialog.Draw(screen)
			batchReviewAtlasLayout(t, dialog, screen.Bounds())
			writeBatchWidgetReview(t, screen, "atlas-no-sextant")

			// Detached measurements exercise the reader, without simulating acquisition.
			measured := dossier
			measured.Entries = append([]journal.AtlasEntry(nil), dossier.Entries...)
			for i := range measured.Entries {
				entry := &measured.Entries[i]
				for _, symbol := range dossier.Symbols {
					if entry.Key == symbol.Key && entry.Kind == "shrine" {
						entry.Survey = journal.SurveyedLocation{Measured: true, Location: symbol.Position}
						entry.Discovered = true
					}
				}
			}
			view.Filters.Places, view.Overview = journal.AtlasShrinePlaces, true
			dialog.Refresh(measured)
			dialog.Draw(screen)
			require.Equal(t, 7, dialog.mappedCount())
			require.Len(t, dialog.mapCallouts(), 7)
			for _, callout := range dialog.mapCallouts() {
				require.True(t, callout.plate.In(dialog.mapRect), callout.name)
			}
			batchReviewAtlasLayout(t, dialog, screen.Bounds())
			writeBatchWidgetReview(t, screen, "atlas-shrine-overview")
			batchReviewPersonalAtlas(t, dialog, screen)
			batchReviewTranslatedSign(t, screen)
		})
	}
}

func batchReviewAtlasLayout(t *testing.T, dialog *AtlasDialog, bounds image.Rectangle) {
	t.Helper()
	for _, rect := range []image.Rectangle{dialog.mapRect, dialog.listRect, dialog.detailRect, dialog.positionRect, dialog.footerRect} {
		require.False(t, rect.Empty())
		require.True(t, rect.In(bounds), "%v outside %v", rect, bounds)
	}
	require.LessOrEqual(t, dialog.detailRect.Max.Y, dialog.positionRect.Min.Y)
	require.Equal(t, 2, dialog.footer.GetPanelContentLineCount())
	for _, line := range strings.Split(dialog.footer.GetOutputStr(false), "\n") {
		width, _ := dialog.font.MeasureString(line)
		require.LessOrEqual(t, width, float64(dialog.footerRect.Dx()))
	}
}

func batchReviewPersonalAtlas(t *testing.T, dialog *AtlasDialog, screen *ebiten.Image) {
	t.Helper()
	position := journal.SiteLocation{Map: references.BritanniaUnderworld, X: 128, Y: 92}
	note := journal.AtlasNoteRecord{ID: 1, Position: position, Label: "North shore landing near shrines",
		Description: strings.Repeat("Watch the inlet; land beside the shrine.\n\n", 13)[:500]}
	require.Len(t, []rune(note.Label), 32)
	require.Len(t, []rune(note.Description), 500)
	require.NoError(t, journal.ValidateAtlasNoteText(note.Label, note.Description))
	dossier := dialog.dossier
	dossier.VesselsUnlocked = true
	dossier.CurrentFix = journal.SurveyedLocation{Measured: true, Location: position}
	dossier.FixUnavailable = ""
	dossier.Entries = append(dossier.Entries,
		journal.AtlasEntry{Key: "vessel-1", Name: "Frigate 1", Kind: "frigate", Category: journal.AtlasVessels,
			Vessel: journal.AtlasVesselPosition{ID: 1, Position: position}},
		journal.AtlasEntry{Key: journal.AtlasNoteKey(note.ID), Name: note.Label, Kind: "note", Category: journal.AtlasNotes, Note: note})
	dialog.view.Filters.Places, dialog.view.Overview = journal.AtlasAllPlaces, false
	for _, key := range []string{"vessel-1", journal.AtlasNoteKey(note.ID)} {
		dialog.view.SelectedKey = key
		dialog.Refresh(dossier)
		dialog.Draw(screen)
		center, visible := dialog.atlasMarkerCenter(dialog.entries[dialog.selection])
		require.True(t, visible)
		pixels := atlasMarkerPixels(screen)
		require.Equal(t, ucolor.EGARed.ToRGBA(), pixels.RGBAAt(center.X, center.Y), "coincident current fix stays above symbols")
		brackets := dialog.selectionBounds(center)
		require.Equal(t, ucolor.EGAYellow.ToRGBA(), pixels.RGBAAt(brackets.Min.X, brackets.Min.Y), "selected brackets survive coincidence")
		batchReviewAtlasLayout(t, dialog, screen.Bounds())
		writeBatchWidgetReview(t, screen, "atlas-coincident-"+key)
	}
	editor := NewAtlasNoteEditor(AtlasNoteEditorConfig{Record: note, Context: "Near the Shrine of Compassion",
		Keyboard: newWidgetTestKeyboard(0), Submit: func(string, string) error { return nil }, Cancel: func() {}})
	editor.focus = 1
	for _, cursor := range []int{0, 500} {
		editor.cursors[1] = cursor
		dialog.Draw(screen)
		editor.Draw(screen)
		require.True(t, editor.pane.In(screen.Bounds()))
		for _, rect := range editor.fieldRects {
			require.True(t, rect.In(editor.pane))
		}
		lines := editor.lines(1)
		row := editor.cursorLine(1, lines)
		layout := editor.fieldLayout(1)
		capacity := layout.capacity
		require.GreaterOrEqual(t, row, editor.scroll[1])
		require.Less(t, row, editor.scroll[1]+capacity, "caret remains in the visible page")
		labelWidth, _ := editor.font.MeasureString(note.Label)
		require.LessOrEqual(t, labelWidth, float64(editor.fieldLayout(0).textWidth))
		caretWidth, _ := editor.font.MeasureString(string(editor.fields[1][lines[row].start:cursor]))
		caret := layout.caretRect(row-editor.scroll[1], caretWidth)
		require.True(t, caret.In(layout.content))
		require.Equal(t, ucolor.EGAYellow.ToRGBA(), atlasMarkerPixels(screen).RGBAAt(caret.Min.X, caret.Min.Y), "caret must actually rasterize inside the field")
		require.Contains(t, editor.output.GetOutputStr(false), "Ctrl+Enter")
		if cursor == 500 {
			require.Positive(t, editor.scroll[1])
		}
		writeBatchWidgetReview(t, screen, fmt.Sprintf("atlas-note-editor-%d", cursor))
	}
}

func batchReviewTranslatedSign(t *testing.T, screen *ebiten.Image) {
	t.Helper()
	dialog := NewSignDialog(SignDialogConfig{Placement: sprites.PercentBasedPlacement{
		StartPercentX: .1, EndPercentX: .65, StartPercentY: .2, EndPercentY: .8}, RawText: framedTranslationSignFixture()})
	t.Cleanup(func() { dialog.buffer.Deallocate() })
	screen.Fill(ucolor.EGADarkGray)
	dialog.Draw(screen)
	before, bounds := atlasMarkerPixels(dialog.buffer), dialog.signBounds
	require.Positive(t, atlasExactInkCount(before, ucolor.EGABrown), "realistic original source frame must rasterize")
	writeBatchWidgetReview(t, screen, "sign-original-mixed")
	dialog.SetTranslated(true)
	screen.Fill(ucolor.EGADarkGray)
	dialog.Draw(screen)
	for _, phrase := range []string{"LIVE BY THE", "EIGHT LAWS", "[George]"} {
		require.Contains(t, signTranslationRowsText(dialog.translationRows), phrase)
	}
	require.True(t, dialog.bounds.In(screen.Bounds()))
	require.Equal(t, bounds, dialog.signBounds)
	require.True(t, bounds.In(dialog.border.GetInteriorBounds().Sub(dialog.bounds.Min)))
	requireSignFixedInk(t, before, atlasMarkerPixels(dialog.buffer))
	require.Positive(t, atlasExactInkCount(atlasMarkerPixels(dialog.buffer), ucolor.EGAYellow), "translated English must rasterize in source yellow")
	require.Positive(t, atlasExactInkCount(atlasMarkerPixels(dialog.buffer), ucolor.EGAWhite), "existing IBM lettering remains white")
	require.Positive(t, atlasExactInkCount(atlasMarkerPixels(atlasSubImage(screen, dialog.bounds)), DialogRuleColor), "outer frame remains visible")
	writeBatchWidgetReview(t, screen, "sign-translated-mixed")
}

func writeBatchWidgetReview(t *testing.T, screen *ebiten.Image, name string) {
	t.Helper()
	directory := os.Getenv("ULTIMA5_BATCH_REVIEW_DIR")
	if directory == "" {
		return // Only the optional artifact write is disabled.
	}
	require.True(t, filepath.IsAbs(directory), "capture directory must be absolute and outside the repository")
	directory, err := filepath.EvalSymlinks(directory)
	require.NoError(t, err, "create the capture directory before running")
	root, err := filepath.Abs("../../..")
	require.NoError(t, err)
	root, err = filepath.EvalSymlinks(root)
	require.NoError(t, err)
	relative, err := filepath.Rel(root, directory)
	require.NoError(t, err)
	require.True(t, relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)), "captures must stay outside the repository")
	var encoded bytes.Buffer
	require.NoError(t, png.Encode(&encoded, atlasMarkerPixels(screen)))
	path := filepath.Join(directory, fmt.Sprintf("%s-%dp.png", name, screen.Bounds().Dy()))
	require.NoError(t, os.WriteFile(path, encoded.Bytes(), 0o600))
	t.Log(path)
}
