package main

import (
	"bytes"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

func TestProvisionFeedbackDungeonViewHeadingRemainsUncovered(t *testing.T) {
	scene := newFourProvisionCompositionScene(t)
	dialog := widgets.NewDungeonViewDialog(widgets.DungeonViewDialogConfig{
		Placement: dungeonViewDialogPercents,
		Model:     game_state.DungeonViewModel{Location: references.Deceit, Floor: 0},
	})
	scene.dialogStack.PushModalDialog(dialog)
	before := scene.provisionChanges.changes.Rows()
	baseline, actual := newPresentationRaster(1280, 720), newPresentationRaster(1280, 720)
	t.Cleanup(baseline.Deallocate)
	t.Cleanup(actual.Deallocate)
	dialog.Draw(baseline)
	scene.drawDialogs(actual)

	// Four ordinary entries make two rows, which reach down over the top of the
	// fitted Dungeon View window at minimum resolution.
	overlap := scene.provisionChanges.panel.Bounds().Intersect(dialog.GetBounds())
	// The window's rounded corner is cut away, so leave it out of the comparison.
	overlap.Min.X += overlap.Dy()
	require.False(t, overlap.Empty(), "fixture must reproduce the reported overlap")
	require.False(t, scene.provisionChangesAboveDialogs())
	want := provisionRegionPixels(baseline, overlap)
	got := provisionRegionPixels(actual, overlap)
	require.True(t, bytes.Equal(want, got), "provision feedback must not overwrite the dungeon view window")

	// Verify the old always-on-top order would actually fail this pixel check.
	scene.provisionChanges.panel.Draw(baseline)
	require.False(t, bytes.Equal(want, provisionRegionPixels(baseline, overlap)))
	assert.Equal(t, before, scene.provisionChanges.changes.Rows(), "composition cannot change deltas or expiry")
	assert.Same(t, dialog, scene.dialogStack.PeekTopModalDialog())
}

func TestProvisionFeedbackTightAndUnknownModalsCoverFeedbackAboveMerchant(t *testing.T) {
	for _, measured := range []bool{true, false} {
		name := "unknown bounds"
		if measured {
			name = "measured overlapping bounds"
		}
		t.Run(name, func(t *testing.T) {
			scene := newFourProvisionCompositionScene(t)
			merchant := newProvisionMerchantModal()
			scene.dialogStack.PushModalDialog(merchant)
			scene.layoutProvisionChanges(image.Rect(0, 0, 1280, 720))
			require.True(t, scene.provisionChangesAboveDialogs(), "ordinary merchant has free space")
			modal := &provisionOpaqueModal{bounds: image.Rect(0, 0, 1280, 80)}
			var top widgets.Widget = modal
			if !measured {
				// Expose only the Widget contract, as tight legacy modals do.
				top = &provisionUnmeasuredModal{Widget: modal}
			}
			scene.dialogStack.PushModalDialog(top)
			baseline, actual := newPresentationRaster(1280, 720), newPresentationRaster(1280, 720)
			t.Cleanup(baseline.Deallocate)
			t.Cleanup(actual.Deallocate)
			top.Draw(baseline)
			scene.drawDialogs(actual)
			require.False(t, scene.provisionChangesAboveDialogs(), "the active modal, not a merchant beneath it, owns this space")
			bounds := scene.provisionChanges.panel.Bounds()
			require.True(t, bounds.In(modal.bounds))
			assert.True(t, bytes.Equal(provisionRegionPixels(baseline, bounds), provisionRegionPixels(actual, bounds)))
			assert.Same(t, top, scene.dialogStack.PeekTopModalDialog())
			scene.dialogStack.MustRemoveWidgetInstance(top)
			require.True(t, scene.provisionChangesAboveDialogs(), "merchant readability returns without refreshing feedback")
		})
	}
}

func TestProvisionFeedbackMerchantKeepsFourOrdinaryEntriesVisible(t *testing.T) {
	scene := newFourProvisionCompositionScene(t)
	merchant := newProvisionMerchantModal()
	scene.dialogStack.PushModalDialog(merchant)
	actual := newPresentationRaster(1280, 720)
	t.Cleanup(actual.Deallocate)
	scene.drawDialogs(actual)
	require.True(t, scene.provisionChangesAboveDialogs())
	bounds := scene.provisionChanges.panel.Bounds()
	require.False(t, bounds.Overlaps(merchant.GetShell().GetBounds()))
	expected := ebiten.NewImageWithOptions(bounds.Inset(-2), &ebiten.NewImageOptions{Unmanaged: true})
	t.Cleanup(expected.Deallocate)
	scene.provisionChanges.panel.Draw(expected)
	assert.True(t, bytes.Equal(provisionRegionPixels(expected, bounds), provisionRegionPixels(actual, bounds)),
		"all signed amounts and current totals remain visible during merchant interaction")
}

func newFourProvisionCompositionScene(t *testing.T) *GameScene {
	t.Helper()
	scene, state := newTransientFeedbackTestScene(t, references.Deceit)
	inventory := &state.PartyState.Inventory
	inventory.Provisions.Food.Set(100)
	inventory.Gold.Set(100)
	inventory.Provisions.Keys.Set(1)
	inventory.Provisions.Gems.Set(2)
	scene.observeProvisionChanges()
	inventory.Provisions.Food.IncrementBy(10)
	inventory.Gold.IncrementBy(20)
	inventory.Provisions.Keys.IncrementBy(2)
	inventory.Provisions.Gems.DecrementByOne()
	scene.observeProvisionChanges()
	count := 0
	for _, row := range scene.provisionChanges.changes.Rows() {
		if row.Name != "" {
			count++
		}
	}
	require.Equal(t, 4, count)

	return scene
}

func provisionRegionPixels(screen *ebiten.Image, bounds image.Rectangle) []byte {
	pixels := make([]byte, bounds.Dx()*bounds.Dy()*4)
	screen.SubImage(bounds).(*ebiten.Image).ReadPixels(pixels)

	return pixels
}

type provisionOpaqueModal struct{ bounds image.Rectangle }

func (m *provisionOpaqueModal) GetBounds() image.Rectangle { return m.bounds }
func (*provisionOpaqueModal) Update()                      {}
func (m *provisionOpaqueModal) Draw(screen *ebiten.Image) {
	screen.SubImage(m.bounds).(*ebiten.Image).Fill(ucolor.EGABlue)
}

type provisionUnmeasuredModal struct{ widgets.Widget }

type provisionMerchantModal struct{ shell *widgets.MerchantDialogShell }

func newProvisionMerchantModal() *provisionMerchantModal {
	return &provisionMerchantModal{shell: widgets.NewMerchantDialogShell(widgets.MerchantDialogShellConfig{
		Header: widgets.MerchantDialogHeaderView{ShopIdentity: "Provisions", Owner: "Barkeep", PartyGold: "120"},
	})}
}

func (m *provisionMerchantModal) GetShell() *widgets.MerchantDialogShell { return m.shell }
func (*provisionMerchantModal) Update()                                  {}
func (m *provisionMerchantModal) Draw(screen *ebiten.Image) {
	m.shell.DrawBackground(screen)
	m.shell.DrawCommonRegions(screen)
}
