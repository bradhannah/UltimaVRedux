package widgets

import (
	"fmt"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func TestZStatsPhysicalShiftPagingAndRepeat(t *testing.T) {
	setZStatsTestResolution(t, 640, 400)
	for _, tab := range []ZStatsDossierTab{ZStatsDossierArmoryTab, ZStatsDossierSpellsTab} {
		for _, shift := range []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyShiftRight} {
			t.Run(fmt.Sprintf("tab%d/%s", tab, shift), func(t *testing.T) {
				held := map[ebiten.Key]bool{}
				keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
				dossier := testZStatsDossier()
				dossier.Armory = make([]game_state.ZStatsArmoryItem, 40)
				dossier.Magic.Spells = make([]game_state.ZStatsSpell, 40)
				for index := range dossier.Magic.Spells {
					dossier.Magic.Spells[index] = game_state.ZStatsSpell{ID: index, Name: "In Lor", Circle: 1}
				}
				dialog := newTestZStatsDossierDialog(dossier, keyboard)
				dialog.switchTab(tab)
				release := func() {
					clear(held)
					keyboard.ClearFrameConsumption()
					dialog.Update()
				}
				if tab == ZStatsDossierSpellsTab {
					held[ebiten.KeySlash] = true
					dialog.Update()
					release()
					require.True(t, dialog.spellBrowser.searching)
				}
				held[ebiten.KeyPageDown] = true
				dialog.Update()
				page := dialog.GetSelectedListIndex()
				require.Greater(t, page, 1)
				release()
				held[shift], held[ebiten.KeyUp] = true, true
				dialog.Update()
				require.Zero(t, dialog.GetSelectedListIndex(), "Shift+Up matches physical PageUp")
				release()
				held[shift], held[ebiten.KeyDown] = true, true
				dialog.Update()
				require.Equal(t, page, dialog.GetSelectedListIndex(), "one page, with no extra row movement")
				for range 17 {
					dialog.Update()
					require.Equal(t, page, dialog.GetSelectedListIndex())
				}
				dialog.Update()
				require.Equal(t, min(39, page*2), dialog.GetSelectedListIndex())
				release()
				held[ebiten.KeyDown] = true
				for range 480 {
					dialog.Update()
				}
				require.Equal(t, 39, dialog.GetSelectedListIndex())
				release()
				held[ebiten.KeyUp] = true
				for range 480 {
					dialog.Update()
				}
				require.Zero(t, dialog.GetSelectedListIndex())
				if tab == ZStatsDossierSpellsTab {
					require.Empty(t, dialog.spellBrowser.searchInput.GetText(), "navigation must not edit the query")
				}
			})
		}
	}
}

func TestListNavigationModalFocusRequiresRelease(t *testing.T) {
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	dialog := newTestZStatsDossierDialog(testZStatsDossier(), keyboard)
	dialog.switchTab(ZStatsDossierArmoryTab)
	var stack DialogStack
	stack.PushModalDialog(dialog)
	dialog.Update()
	held[ebiten.KeyDown] = true
	dialog.Update()
	require.Equal(t, 1, dialog.GetSelectedListIndex())
	child := newTestButtonListModal("Nested")
	stack.PushModalDialog(child)
	stack.MustRemoveWidgetInstance(child)
	for range 60 {
		dialog.Update()
	}
	require.Equal(t, 1, dialog.GetSelectedListIndex(), "returning focus cannot resume an old hold")
	clear(held)
	keyboard.ClearFrameConsumption()
	dialog.Update()
	held[ebiten.KeyDown] = true
	dialog.Update()
	require.Equal(t, 2, dialog.GetSelectedListIndex())
	clear(held)
	keyboard.ClearFrameConsumption()
	stack.PushModalDialog(child)
	stack.MustRemoveWidgetInstance(child)
	held[ebiten.KeyDown] = true
	dialog.Update()
	require.Equal(t, 3, dialog.GetSelectedListIndex(), "fresh input after focus returns needs no extra release")
}

func TestSaveSlotUnsupportedShiftPagingDoesNotSelect(t *testing.T) {
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(0, func(key ebiten.Key) bool { return held[key] })
	browser := newTestSaveSlotBrowser(keyboard, makeSaveSlotViews(8), SaveSlotBrowserConfig{})
	held[ebiten.KeyShiftRight], held[ebiten.KeyDown] = true, true
	for range 60 {
		browser.Update()
	}
	require.Zero(t, browser.GetSelectedIndex())
	clear(held)
	browser.Update()
	held[ebiten.KeyDown] = true
	browser.Update()
	require.Equal(t, 1, browser.GetSelectedIndex())
}

func TestZStatsPagingFooterFitsMinimumAndLargerSizes(t *testing.T) {
	for _, size := range [][2]int{{320, 200}, {640, 400}, {1280, 720}} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setZStatsTestResolution(t, size[0], size[1])
			dialog := newTestZStatsDossierDialog(testZStatsDossier(), newWidgetTestKeyboard(0))
			for tab := ZStatsDossierCharacterTab; tab < zStatsDossierTabCount; tab++ {
				dialog.switchTab(tab)
				dialog.prepareFooter()
				footer := dialog.footerOutput.GetOutputStr(false)
				require.Contains(t, footer, "[Tab/Shift+Tab]")
				require.Contains(t, footer, "[Shift+Up/Down]")
				require.Contains(t, footer, "[Esc")
				require.LessOrEqual(t, dialog.footerOutput.GetTotalHeightForLines(
					dialog.footerOutput.GetPanelContentLineCount()),
					float64(dialog.layout.footerRect.Dy()-2*dialog.layout.padding))
			}
		})
	}
}
