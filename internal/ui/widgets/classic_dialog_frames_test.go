package widgets

import (
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestCommandDialogsUseConnectedClassicFrames(t *testing.T) {
	setResponsiveDialogTestResolution(t, 640, 360)
	screen := ebiten.NewImage(640, 360)
	t.Cleanup(screen.Dispose)
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		Build()

	t.Run("cast", func(t *testing.T) {
		dialog := NewSpellCastingDialog(
			newWidgetTestKeyboard(0), &state.PartyState, references.ContextTown, state.CurrentSpellCatalog(), state.ValidateSpellTargetingPrerequisites,
		)
		screen.Clear()
		dialog.Draw(screen)
		requireConnectedClassicFrameRows(t, dialog.view.frames, []int{1, 1, 2, 1}, dialogPixelUnit(640, 360))
		requireClassicFrameSilhouette(t, screen, dialog.view.frames[0])
	})

	t.Run("mix", func(t *testing.T) {
		dialog := NewReagentMixingDialog(
			newWidgetTestKeyboard(0),
			state.CurrentSpellCatalog(),
			[]ReagentInfo{{Reagent: references.SulfurAsh, Name: "Sulfur Ash", Quantity: 5}},
		)
		screen.Clear()
		dialog.Draw(screen)
		requireConnectedClassicFrameRows(t, dialog.view.frames, []int{1, 1, 2, 1}, dialogPixelUnit(640, 360))
		requireClassicFrameSilhouette(t, screen, dialog.view.frames[0])
	})

	t.Run("ready", func(t *testing.T) {
		dialog := NewReadyEquipmentDialog(
			newWidgetTestKeyboard(0), &sprites.SpriteSheet{}, state, game_state.ReadyEquipmentRequest{},
		)
		screen.Clear()
		dialog.Draw(screen)
		requireConnectedClassicFrameRows(t, dialog.frameRects, []int{1, 3, 1}, dialogPixelUnit(640, 360))
		requireClassicFrameSilhouette(t, screen, dialog.frameRects[0])
	})
}

func TestQuestJournalAndSaveBrowserUseConnectedClassicFrames(t *testing.T) {
	setResponsiveDialogTestResolution(t, 640, 360)
	screen := ebiten.NewImage(640, 360)
	t.Cleanup(screen.Dispose)

	t.Run("quest journal", func(t *testing.T) {
		dialog := NewQuestJournalDialog(
			testQuestJournalDossier(),
			testQuestJournalConfig(newWidgetTestKeyboard(0)),
		)
		screen.Clear()
		dialog.Draw(screen)
		requireConnectedClassicFrameRows(t, dialog.frameRects, []int{1, 2, 1}, dialog.pixelUnit())
		requireClassicFrameSilhouette(t, screen, dialog.frameRects[0])
		legacyOuterPoint := image.Pt(
			dialog.outerBorder.GetBounds().Min.X+dialog.outerBorder.GetBounds().Dx()/2,
			dialog.outerBorder.GetBounds().Min.Y,
		)
		requireColorNotRGBA(t, DialogRuleColor, screen.At(legacyOuterPoint.X, legacyOuterPoint.Y))
	})

	t.Run("save slots", func(t *testing.T) {
		dialog := newTestSaveSlotBrowser(newWidgetTestKeyboard(0), makeSaveSlotViews(2), SaveSlotBrowserConfig{})
		screen.Clear()
		dialog.Draw(screen)
		requireConnectedClassicFrameRows(t, dialog.frameRects, []int{1, 2, 1}, dialog.pixelUnit())
		requireClassicFrameSilhouette(t, screen, dialog.frameRects[0])
		legacyOuterPoint := image.Pt(
			dialog.outerBorder.GetBounds().Min.X+dialog.outerBorder.GetBounds().Dx()/2,
			dialog.outerBorder.GetBounds().Min.Y,
		)
		requireColorNotRGBA(t, DialogRuleColor, screen.At(legacyOuterPoint.X, legacyOuterPoint.Y))
	})
}

func requireConnectedClassicFrameRows(t *testing.T, frames []image.Rectangle, rowLengths []int, pixelUnit int) {
	t.Helper()
	require.Equal(t, sumInts(rowLengths), len(frames))

	rowStart := 0
	for rowIndex, rowLength := range rowLengths {
		row := frames[rowStart : rowStart+rowLength]
		for column := 0; column+1 < len(row); column++ {
			require.Equal(t, row[column].Max.X-pixelUnit, row[column+1].Min.X)
		}
		if rowIndex+1 < len(rowLengths) {
			nextStart := rowStart + rowLength
			require.Equal(t, row[0].Max.Y-pixelUnit, frames[nextStart].Min.Y)
		}
		rowStart += rowLength
	}
}

func requireClassicFrameSilhouette(t *testing.T, screen *ebiten.Image, frame image.Rectangle) {
	t.Helper()
	topCenter := image.Pt(frame.Min.X+frame.Dx()/2, frame.Min.Y)
	requireColorRGBA(t, DialogRuleColor, screen.At(topCenter.X, topCenter.Y))
	requireColorNotRGBA(t, MerchantDialogBackgroundColor, screen.At(frame.Min.X, frame.Min.Y))
}

func sumInts(values []int) int {
	total := 0
	for _, value := range values {
		total += value
	}

	return total
}

func dialogPixelUnit(width, height int) int {
	return getClassicUIPixelUnit(width, height)
}
