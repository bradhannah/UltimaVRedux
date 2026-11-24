package main

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"
)

// Reuse the native renderer fixture and production rail composition. This is
// deliberately a HUD-only frame, with no claim of real-game keyboard acceptance.
func TestBatchReviewHUDAmmunition(t *testing.T) {
	manager := display.GetManager()
	saved := *manager
	t.Cleanup(func() { *manager = saved })
	// Keep the smallest supported and fractional-width composition anchors;
	// supported-size capacity/geometry is covered by the cheap rail tests.
	for _, size := range []image.Point{{1280, 720}, {1366, 768}} {
		t.Run(fmt.Sprintf("%dp", size.Y), func(t *testing.T) {
			bounds := logicalGameBounds(size.X, size.Y)
			manager.PublishLogicalSize(bounds.Dx(), bounds.Dy())
			state, _ := gamefixture.NewIntegrationTestBuilder(t).WithLocation(references.Britain).
				WithSystemCallbacks().WithRandomSeed(42).Build()
			scene := &GameScene{gameState: state, gameConfig: state.GameConfig}
			state.DateTime = datetime.UltimaDate{Year: 65535, Month: 13, Day: 28, Hour: 12, Minute: 59}
			state.PartyState.Inventory.Provisions.Food.Set(9999)
			state.PartyState.Inventory.Provisions.Torches.Set(99)
			state.PartyState.Inventory.Provisions.Keys.Set(99)
			state.PartyState.Inventory.Provisions.Gems.Set(99)
			state.PartyState.Inventory.Gold.Set(9999)
			state.PartyState.Karma.Value = 99
			scene.gameConfig = state.GameConfig
			scene.spriteSheet = sharedPresentationTestSpriteSheet()
			scene.mapImage = ebiten.NewImage(304, 208)
			t.Cleanup(scene.mapImage.Deallocate)
			scene.output = nil
			scene.initializeResizeableVisualElements()
			scene.activeEffectsPanel = NewActiveEffectsPanel(state, scene.spriteSheet, scene.rightRail.activeBorder)
			scene.syncRightRailLayout(bounds)
			t.Cleanup(scene.outputClipImage.Deallocate)
			scene.output.AddText("Ammunition supplies.")
			layout := scene.rightRail
			// Native screen coordinates on a rail-sized target preserve clipping
			// and transcript protection without allocating the unused map viewport.
			rail := layout.activeBorder.Union(layout.characterContent).Union(layout.provisionBorder).Union(layout.outputContent)
			screen := ebiten.NewImageWithOptions(rail, nil)
			t.Cleanup(screen.Deallocate)
			t.Cleanup(scene.characterSummary.Dispose)
			require.True(t, layout.provisionContent.In(layout.provisionBorder))
			require.False(t, layout.outputContent.Overlaps(layout.provisionContent))
			require.Equal(t, 11, scene.output.GetLineCapacity(layout.outputContent.Dy()))
			for _, counts := range []mainscreen.AmmunitionCounts{{20, 0}, {0, 20}} {
				state.PartyState.Inventory.Equipment.Set(references.Arrows, counts[0])
				state.PartyState.Inventory.Equipment.Set(references.Quarrels, counts[1])
				screen.Fill(ucolor.EGABlack)
				scene.drawBorders(screen)
				scene.activeEffectsPanel.Draw(screen)
				scene.drawMainOutput(screen)
				scene.characterSummary.Draw(&state.PartyState, screen, state.GetActivePlayerIndex(), false, -1)
				protected := provisionRegionPixels(screen, layout.outputContent)
				scene.provisionSummary.Draw(state, screen)
				require.Equal(t, protected, provisionRegionPixels(screen, layout.outputContent), "ammo cannot overwrite the transcript")
				// The third row must contain both semantic warning inks. Existing
				// mainscreen tests verify the LOW/OUT strings; here they must rasterize.
				ammo := layout.provisionContent
				ammo.Max.Y -= mainscreen.ProvisionAmmoRowHeight(bounds.Dx()) // Calendar occupies the bottom row.
				ammo.Min.Y = ammo.Max.Y - mainscreen.ProvisionAmmoRowHeight(bounds.Dx())
				region := screen.SubImage(ammo).(*ebiten.Image)
				require.Positive(t, countExactColor(region, ucolor.SupplyLow))
				require.Positive(t, countExactColor(region, ucolor.SupplyEmpty))
				require.Equal(t, counts, mainscreen.SnapshotAmmunition(&state.PartyState.Inventory), "drawing is read-only")
				writeBatchHUDReview(t, screen, fmt.Sprintf("hud-ammo-ar%d-qr%d-%dp.png", counts[0], counts[1], size.Y))
			}
		})
	}
}

func writeBatchHUDReview(t *testing.T, screen *ebiten.Image, name string) {
	t.Helper()
	directory := os.Getenv("ULTIMA5_BATCH_REVIEW_DIR")
	if directory == "" {
		return // Assertions above remain mandatory without artifact output.
	}
	require.True(t, filepath.IsAbs(directory), "capture directory must be absolute and outside the repository")
	directory, err := filepath.EvalSymlinks(directory)
	require.NoError(t, err, "create the capture directory before running")
	root, err := filepath.Abs("../..")
	require.NoError(t, err)
	root, err = filepath.EvalSymlinks(root)
	require.NoError(t, err)
	relative, err := filepath.Rel(root, directory)
	require.NoError(t, err)
	require.True(t, relative == ".." || strings.HasPrefix(relative, ".."+string(filepath.Separator)), "captures must stay outside the repository")
	t.Setenv("ULTIMA5_REVIEW_IMAGES", directory)
	exportPresentationReview(t, screen, name)
	t.Log(filepath.Join(directory, name))
}
