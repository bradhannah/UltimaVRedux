package widgets

import (
	"fmt"
	"image"
	"math"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/internal/text"
)

func TestZStatsCanonicalSpellDetailsFitOnePage(t *testing.T) {
	dossier := testZStatsDossier()
	dossier.Magic.Spells = nil
	items := references.NewInventoryItemsReferences()
	for spell := references.InLor; spell <= references.AnTym; spell++ {
		info := references.GetSpellInfo(spell)
		var reagents []string
		for reagent := references.SulfurAsh; reagent <= references.MandrakeRoot; reagent++ {
			if references.GetSpellReagents(spell)&(1<<reagent) != 0 {
				reagents = append(reagents, items.GetReferenceByItem(reagent).ItemName)
			}
		}
		dossier.Magic.Spells = append(dossier.Magic.Spells, game_state.ZStatsSpell{
			ID: spell.ID(), Name: info.Name, Syllables: info.Name, Circle: info.Circle,
			MPCost: info.Circle, Prepared: 99, FormulaState: "confirmed",
			Reagents: reagents, Targeting: info.Targeting.String(), Contexts: info.Context.String(),
			Description: info.Description,
		})
	}
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			dialog.switchTab(ZStatsDossierSpellsTab)
			for index, spell := range dossier.Magic.Spells {
				dialog.tabSelections[ZStatsDossierSpellsTab] = index
				dialog.resetDetailPage()
				dialog.prepareSpellDetail(dialog.layout.globalDetailRect)
				require.Equal(t, 1, dialog.detailPageCount(), spell.Name)
			}
		})
	}
}

func TestZStatsSpellSummaryStaysFixedWhileOnlyOverflowBodyPages(t *testing.T) {
	for _, size := range []image.Point{{1280, 720}, {1920, 1080}} {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			keyboard := newWidgetTestKeyboard(0)
			dialog := newRenderedTestZStatsDossierDialog(testZStatsDossier(), keyboard)
			dialog.switchTab(ZStatsDossierSpellsTab)
			screen := ebiten.NewImageWithOptions(dialog.layout.globalDetailRect, nil)
			t.Cleanup(screen.Dispose)
			dialog.drawSpellDetail(screen, dialog.layout.globalDetailRect)
			dialog.prepareFooter()
			require.Equal(t, 1, dialog.detailPageCount())
			require.NotContains(t, dialog.footerOutput.GetOutputStr(false), "Space")
			summary := dialog.summaryOutput.GetOutputStr(false)
			require.Contains(t, summary, "Circle: 1")
			require.Contains(t, summary, "MP cost: 1")
			require.Contains(t, summary, "Prepared: 2")
			require.Contains(t, dialog.detailOutput.GetOutputStr(false), "Formula: candidate")
			_, top, _ := dialog.catalogDetailRects(dialog.layout.globalDetailRect)
			before := make([]byte, top.Dx()*top.Dy()*4)
			screen.SubImage(top).(*ebiten.Image).ReadPixels(before)

			dialog.dossier.Magic.Spells[0].Description = strings.Repeat("A lengthy spell description. ", 100)
			screen.Clear()
			dialog.drawSpellDetail(screen, dialog.layout.globalDetailRect)
			require.Greater(t, dialog.detailPageCount(), 1)
			pressZStatsDossierKey(dialog, keyboard, ebiten.KeySpace)
			screen.Clear()
			dialog.drawSpellDetail(screen, dialog.layout.globalDetailRect)
			dialog.prepareFooter()
			require.Equal(t, 1, dialog.detailPage)
			after := make([]byte, len(before))
			screen.SubImage(top).(*ebiten.Image).ReadPixels(after)
			require.Equal(t, before, after, "paging the body must not replace the summary")
			require.Equal(t, summary, dialog.summaryOutput.GetOutputStr(false))
			require.Contains(t, dialog.footerOutput.GetOutputStr(false), "Space")
		})
	}
}

func TestZStatsCatalogQuantitiesAndUncountedEquipment(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	for _, quantity := range []uint16{0, 12} {
		dossier := testZStatsDossier()
		dossier.Provisions[0].Quantity = quantity
		dossier.Armory[0].Available = quantity
		dossier.Magic.Spells[0].Prepared = quantity
		dossier.Magic.Reagents[0].Quantity = quantity
		dossier.Magic.Potions[0].Quantity = quantity
		dossier.Magic.Scrolls[0].Quantity = quantity
		dossier.Items[0].Quantity = max(1, quantity)
		dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
		assertZStatsCatalogRows(t, dialog, quantity)
	}
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), newWidgetTestKeyboard(0))
	dialog.switchTab(ZStatsDossierCharacterTab)
	dialog.prepareListRow(image.Rect(100, 100, 1000, 140), 0, false)
	require.NotContains(t, dialog.rowOutput.GetOutputStr(false), "(")
	dialog.switchTab(ZStatsDossierItemsTab)
	require.Equal(t, 2, dialog.currentItemCount(), "unowned possessions remain filtered")
	dialog.prepareListRow(image.Rect(100, 100, 1000, 140), 1, false)
	require.Contains(t, dialog.rowOutput.GetOutputStr(false), "Blue Moonstone (1)")
}

func assertZStatsCatalogRows(t *testing.T, dialog *ZStatsDossierDialog, quantity uint16) {
	t.Helper()
	for _, test := range []struct {
		tab  ZStatsDossierTab
		want string
	}{
		{ZStatsDossierProvisionsTab, fmt.Sprintf("Food (%d)", quantity)},
		{ZStatsDossierArmoryTab, "Long Sword"},
		{ZStatsDossierSpellsTab, "In Lor"},
		{ZStatsDossierReagentsTab, fmt.Sprintf("Garlic (%d)", quantity)},
		{ZStatsDossierPotionsTab, fmt.Sprintf("Red Potion (%d)", quantity)},
		{ZStatsDossierScrollsTab, fmt.Sprintf("In Lor Scroll (%d)", quantity)},
		{ZStatsDossierItemsTab, fmt.Sprintf("Crown (%d)", max(1, quantity))},
	} {
		dialog.switchTab(test.tab)
		for _, selected := range []bool{false, true} {
			dialog.prepareListRow(image.Rect(100, 100, 1000, 140), 0, selected)
			require.Contains(t, dialog.rowOutput.GetOutputStr(false), test.want)
			if test.tab == ZStatsDossierSpellsTab {
				require.Equal(t, fmt.Sprint(quantity), dialog.spellView.count.GetOutputStr(false))
				require.Equal(t, 2, dialog.rowOutput.GetPanelContentLineCount())
			} else {
				require.Equal(t, 1, dialog.rowOutput.GetPanelContentLineCount())
			}
		}
	}
}

// A catalog name too long for the list wraps whole: nothing is cut, the quantity
// stays on the first line, and only that row grows.
func TestZStatsLongCatalogNameWrapsWholeAndKeepsItsQuantity(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	dossier := testZStatsDossier()
	dossier.Provisions[0].Name = "Provision with an exceedingly lengthy name"
	dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
	dialog.switchTab(ZStatsDossierProvisionsTab)
	list := dialog.layout.globalListRect
	require.Greater(t, dialog.listRowHeight(list, 0), dialog.listRowHeight(list, 1), "only the long row is tall")
	dialog.prepareListRow(dialog.panelBodyRect(list), 0, false)
	row := dialog.rowOutput.GetOutputStr(false)
	first, _, _ := strings.Cut(row, "\n")
	require.True(t, strings.HasSuffix(strings.TrimSpace(first), "(123)"), "quantity stays on the first line: %q", row)
	require.Equal(t, strings.Fields(dossier.Provisions[0].Name),
		strings.Fields(strings.Replace(row, "(123)", "", 1)), "the whole name is shown")
}

func TestZStatsArmoryNamesUseNormalFontAndFitActualCatalogWidth(t *testing.T) {
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			dossier := testZStatsDossier()
			dossier.Armory[0].Available = 65535
			dossier.Armory[0].Readied = 6
			dialog := newRenderedTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			dialog.switchTab(ZStatsDossierArmoryTab)
			row := dialog.panelBodyRect(dialog.layout.globalListRect)
			row.Max.Y = row.Min.Y + int(math.Ceil(dialog.rowOutput.GetLineSpacing()))
			screen := ebiten.NewImageWithOptions(row, nil)
			t.Cleanup(screen.Dispose)
			dialog.drawListRow(screen, row, 0, false)
			require.Equal(t, "Long Sword", dialog.rowOutput.GetOutputStr(false))
			require.NotEmpty(t, dialog.rowIconOutput.GetOutputStr(false))
			require.NotContains(t, dialog.rowOutput.GetOutputStr(false), "(")
			require.Same(t, dialog.bodyFont, dialog.rowOutput.Font)
			require.Equal(t, 1, dialog.rowOutput.GetPanelContentLineCount())
			rendered := renderedContentBounds(t, screen)
			require.LessOrEqual(t, rendered.Max.X, row.Max.X-dialog.scrollIndicatorWidth())

			// The same equipment in Character uses the standard list font.
			screen.Clear()
			dialog.switchTab(ZStatsDossierCharacterTab)
			dialog.drawListRow(screen, row, 2, false)
			require.Equal(t, renderedContentBounds(t, screen).Size(), rendered.Size())

			dialog.switchTab(ZStatsDossierArmoryTab)
			dialog.prepareArmoryDetail(dialog.layout.globalDetailRect)
			require.Contains(t, dialog.summaryOutput.GetOutputStr(false), "65535 / 6")
		})
	}
}

func TestZStatsCompactSpellDetailsRetainUsableOverflowArea(t *testing.T) {
	for _, size := range []image.Point{{320, 200}, {640, 360}} {
		t.Run(size.String(), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			dossier := testZStatsDossier()
			dossier.Magic.Spells[0].Name = "In Vas Grav Corp"
			dossier.Magic.Spells[0].Prepared = 65535
			dossier.Magic.Spells[0].Description = strings.Repeat("The spell description remains accessible. ", 40)
			dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			dialog.switchTab(ZStatsDossierSpellsTab)
			dialog.prepareSpellDetail(dialog.layout.globalDetailRect)
			portrait, summary, body := dialog.catalogDetailRects(dialog.layout.globalDetailRect)
			summary.Max.Y = summary.Min.Y + int(math.Ceil(dialog.summaryOutput.GetTotalHeightForLines(dialog.summaryOutput.GetPanelContentLineCount())))
			body.Min.Y = max(portrait.Max.Y, summary.Max.Y) + dialog.layout.padding
			require.GreaterOrEqual(t, body.Dy(), int(math.Ceil(dialog.detailOutput.GetFontHeight())))
			require.Greater(t, dialog.detailPageCount(), 1)
		})
	}
}

func TestZStatsAllMoonstoneGlyphsFollowRenamedReorderedFilteredRows(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	dossier := testZStatsDossier()
	dossier.Items = []game_state.ZStatsOwnedItem{
		{ID: 0, Category: "Quest Item", Name: "New Moon Moonstone", Quantity: 1, SpriteIndex: indexes.Moon},
		{ID: 7, Category: "Moonstone", Name: "Unowned", MoonPhase: game_state.ZStatsMoonPhase{Present: true, Phase: datetime.MoonPhaseCrescentWaning}},
	}
	for _, phase := range []datetime.MoonPhase{6, 0, 4, 1, 7, 3, 5, 2} {
		dossier.Items = append(dossier.Items, game_state.ZStatsOwnedItem{
			ID: int(phase), Category: "Moonstone", Name: "Renamed possession", Quantity: 1, SpriteIndex: indexes.Moon,
			MoonPhase: game_state.ZStatsMoonPhase{Present: true, Phase: phase},
		})
	}
	dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
	t.Cleanup(dialog.outerBorder.Dispose)
	dialog.switchTab(ZStatsDossierItemsTab)
	require.Equal(t, 9, dialog.currentItemCount())
	row := dialog.panelBodyRect(dialog.layout.globalListRect)
	row.Max.Y = row.Min.Y + dialog.listRowHeight(dialog.layout.globalListRect, 0)
	dialog.prepareListRow(row, 0, false)
	require.Empty(t, dialog.rowIconOutput.GetOutputStr(false), "phase zero is not inferred from name, sprite or ID")
	for position := 1; position < dialog.currentItemCount(); position++ {
		item := dossier.Items[position+1]
		dialog.setCurrentSelection(position)
		row.Max.Y = row.Min.Y + dialog.listRowHeight(dialog.layout.globalListRect, position)
		for _, selected := range []bool{false, true} {
			dialog.prepareListRow(row, position, selected)
			requireZStatsCountedPhaseRow(t, dialog, row, item)
		}
		prepared := dialog.prepareItemDetail(dialog.layout.globalDetailRect)
		require.True(t, prepared.portrait.Empty())
		require.False(t, prepared.phaseIcon.Empty())
		require.Equal(t, string(rune(0x30+item.MoonPhase.Phase)), dialog.phaseOutput.GetOutputStr(false))
		require.Contains(t, dialog.summaryOutput.GetOutputStr(false), item.Name)
	}
	// The optional phase is detached along with the rest of the item record.
	dossier.Items[2].MoonPhase.Phase = datetime.MoonPhaseNew
	require.Equal(t, datetime.MoonPhaseLastQuarter, dialog.dossier.Items[2].MoonPhase.Phase)
	dialog.setCurrentSelection(1)
	refreshed := cloneZStatsDossier(dialog.dossier)
	refreshed.Items[2], refreshed.Items[9] = refreshed.Items[9], refreshed.Items[2]
	refreshed.Items[9].Name = "Renamed again"
	dialog.RefreshDossier(refreshed)
	require.Equal(t, 8, dialog.GetSelectedListIndex(), "refresh follows the typed identity, not the old row or label")
	dialog.prepareItemDetail(dialog.layout.globalDetailRect)
	require.Equal(t, "6", dialog.phaseOutput.GetOutputStr(false))
	require.Contains(t, dialog.summaryOutput.GetOutputStr(false), "Renamed again")
}

func TestZStatsFullEquipmentRowsWrapWithMeasuredIconsAndNoClipping(t *testing.T) {
	for _, size := range append(config.SupportedWindowResolutions(), config.ScreenResolution{X: 320, Y: 200}) {
		t.Run(fmt.Sprintf("%dx%d", size.X, size.Y), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			dossier := testZStatsDossier()
			dossier.Armory = nil
			dossier.Characters[0].Equipped = nil
			for _, equipment := range []references.Equipment{references.TwoHSword, references.RingRegeneration, references.AmuletOfTurning} {
				dossier.Armory = append(dossier.Armory, game_state.ZStatsArmoryItem{
					ID: equipment.ID(), Name: equipment.String(), Available: 65535, Readied: 6,
				})
				dossier.Characters[0].Equipped = append(dossier.Characters[0].Equipped, game_state.ZStatsEquippedItem{
					ItemID: equipment.ID(), Name: equipment.String(), Slot: "Weapon",
				})
			}
			dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			t.Cleanup(dialog.outerBorder.Dispose)
			for _, tab := range []ZStatsDossierTab{ZStatsDossierCharacterTab, ZStatsDossierArmoryTab} {
				dialog.switchTab(tab)
				list := dialog.activeListRect()
				row := dialog.panelBodyRect(list)
				for index, item := range dossier.Armory {
					pitch := dialog.listRowHeight(list, index)
					row.Max.Y = row.Min.Y + pitch
					for _, selected := range []bool{false, true} {
						dialog.prepareListRow(row, index, selected)
						label := dialog.rowOutput.GetOutputStr(false)
						require.Equal(t, item.Name, strings.ReplaceAll(label, " \n", ""))
						require.NotContains(t, label, "...")
						require.NotEmpty(t, dialog.rowIconOutput.GetOutputStr(false))
						require.Same(t, dialog.bodyFont, dialog.rowOutput.Font)
						require.LessOrEqual(t, dialog.rowOutput.GetTotalHeightForLines(dialog.rowOutput.GetPanelContentLineCount()), float64(pitch))
						for _, physical := range strings.Split(label, " \n") {
							width, _ := dialog.bodyFont.MeasureString(physical)
							require.LessOrEqual(t, width, float64(dialog.rowWidth))
						}
					}
				}
				// Force a genuine wrap independently of font/catalog changes.
				narrow := row
				narrow.Max.X = narrow.Min.X + row.Dx()/2
				dialog.prepareListRow(narrow, 1, true)
				require.Greater(t, dialog.rowOutput.GetPanelContentLineCount(), 1)
				require.Equal(t, references.RingRegeneration.String(), strings.ReplaceAll(dialog.rowOutput.GetOutputStr(false), " \n", ""))
			}
		})
	}
}

func TestZStatsMoonstoneLongRowsReserveBitmapQuantityAndGutter(t *testing.T) {
	for _, size := range []image.Point{{320, 200}, {1280, 720}, {2560, 1440}} {
		t.Run(size.String(), func(t *testing.T) {
			setZStatsTestResolution(t, size.X, size.Y)
			dossier := testZStatsDossier()
			dossier.Items[1].Name = "An unusually long renamed Moonstone possession"
			dossier.Items[1].Quantity = 65535
			dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
			t.Cleanup(dialog.outerBorder.Dispose)
			dialog.switchTab(ZStatsDossierItemsTab)
			row := dialog.panelBodyRect(dialog.layout.globalListRect)
			row.Max.Y = row.Min.Y + dialog.listRowHeight(dialog.layout.globalListRect, 1)
			for _, selected := range []bool{false, true} {
				dialog.prepareListRow(row, 1, selected)
				requireZStatsCountedPhaseRow(t, dialog, row, dossier.Items[1])
				require.Greater(t, dialog.rowOutput.GetPanelContentLineCount(), 1, "the long-name fixture exercises continuation rows")
			}
		})
	}
}

func requireZStatsCountedPhaseRow(
	t *testing.T, dialog *ZStatsDossierDialog, row image.Rectangle, item game_state.ZStatsOwnedItem,
) {
	t.Helper()
	label := dialog.rowOutput.GetOutputStr(false)
	// GetOutputStr inserts " \n" between physical rows. Remove only those
	// separators and the single first-row quantity suffix: authored name bytes,
	// including spaces at the wrap boundary, must survive exactly and in order.
	physicalRows := strings.Split(label, " \n")
	suffix := fmt.Sprintf(" (%d)", item.Quantity)
	require.True(t, strings.HasSuffix(physicalRows[0], suffix), "quantity belongs at the end of the first physical row: %q", label)
	require.Equal(t, 1, strings.Count(label, suffix), "quantity must appear exactly once")
	name := strings.TrimSuffix(physicalRows[0], suffix) + strings.Join(physicalRows[1:], "")
	require.Equal(t, item.Name, name, "wrapping must retain the complete name without truncation or reordered words")
	require.Equal(t, len(physicalRows), dialog.rowOutput.GetPanelContentLineCount())
	for _, physical := range physicalRows {
		width, _ := dialog.bodyFont.MeasureString(physical)
		require.LessOrEqual(t, width, float64(dialog.rowWidth), "name and first-row quantity must fit together")
	}
	require.True(t, item.MoonPhase.Present)
	require.Equal(t, string(rune(0x30+item.MoonPhase.Phase)), dialog.rowIconOutput.GetOutputStr(false))
	require.Equal(t, 1, dialog.rowIconOutput.GetPanelContentLineCount())
	require.Positive(t, dialog.rowIconWidth)
	require.Zero(t, dialog.rowIconWidth%text.BitmapCharWidth)
	require.Equal(t, float64(dialog.rowIconWidth), dialog.rowIconOutput.GetLineSpacing(), "phase cell uses its actual bitmap advance")

	viewport := image.Rect(row.Min.X+dialog.layout.padding, row.Min.Y,
		row.Max.X-dialog.layout.padding-dialog.scrollIndicatorWidth(), row.Max.Y)
	glyph := image.Rect(viewport.Min.X, viewport.Min.Y,
		viewport.Min.X+dialog.rowIconWidth, viewport.Min.Y+dialog.rowIconWidth)
	nameLeft := glyph.Max.X + dialog.pixelUnit()
	nameHeight := int(math.Ceil(dialog.rowOutput.GetTotalHeightForLines(len(physicalRows))))
	nameRect := image.Rect(nameLeft, row.Min.Y, nameLeft+dialog.rowWidth, row.Min.Y+nameHeight)
	require.True(t, glyph.In(viewport), "phase glyph %v must fit the actual row viewport %v", glyph, viewport)
	require.True(t, nameRect.In(viewport), "complete name/count block %v must fit the actual row viewport %v", nameRect, viewport)
	require.False(t, glyph.Overlaps(nameRect), "glyph and name/count block must remain separate")
}

// One name too long for the list makes only its own row tall.
func TestZStatsOnlyTheWrappedRowIsTall(t *testing.T) {
	setZStatsTestResolution(t, 1280, 720)
	dossier := testZStatsDossier()
	dossier.Characters[0].Equipped[1].Name = "Ring of Exceedingly Lengthy Protection"
	dialog := newTestZStatsDossierDialog(dossier, newWidgetTestKeyboard(0))
	t.Cleanup(dialog.outerBorder.Dispose)
	list := dialog.layout.characterListRect
	line := dialog.listRowHeight(list, 0)
	require.Greater(t, dialog.listRowHeight(list, 1), line, "the long name wraps")
	for index := 2; index < dialog.currentItemCount(); index++ {
		require.Equal(t, line, dialog.listRowHeight(list, index), "its neighbours keep one line")
	}
	// Every row is shown, each at its own height, within the list.
	start, end := dialog.visibleListRange(list)
	require.Equal(t, []int{0, dialog.currentItemCount()}, []int{start, end})
	height := 0
	for index := start; index < end; index++ {
		height += dialog.listRowHeight(list, index)
	}
	require.LessOrEqual(t, height, dialog.panelBodyRect(list).Dy())
	require.Greater(t, dialog.visibleListRows(list), dialog.currentItemCount(), "room is left under the short list")
}
