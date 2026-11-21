package widgets

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

func TestSpellBrowserSearchAndSyllables(t *testing.T) {
	state, dialog, press := newCastBrowserTest(t)
	press(ebiten.KeyV)
	press(ebiten.KeyF)
	require.Equal(t, references.VasFlam, dialog.GetSelectedSpell())
	press(ebiten.KeySlash)
	require.True(t, dialog.browser.searching)
	for _, test := range []struct {
		query string
		count int
	}{
		{"poison", 3}, {"EnErGy", 2}, {" FIELD pois ", 1}, {"heal", 2}, {"vas flam", 1}, {"poison energy", 0},
	} {
		dialog.browser.query = test.query
		dialog.browser.filter()
		require.Len(t, dialog.browser.filtered, test.count, test.query)
	}
	press(ebiten.KeyEscape)
	require.False(t, dialog.IsDismissed())
	require.Equal(t, "VF", dialog.browser.syllables)
	press(ebiten.KeySlash)
	for _, key := range []ebiten.Key{ebiten.KeyP, ebiten.KeyO, ebiten.KeyI, ebiten.KeyS, ebiten.KeyO, ebiten.KeyN} {
		press(key)
	}
	require.Equal(t, "poison", strings.ToLower(dialog.browser.query))
	require.Equal(t, spellFilterAll, dialog.browser.filterMode)
	for spell := range references.Spell(references.TotalSpellCount) {
		state.PartyState.Inventory.Spells.Set(spell, 0)
	}
	state.PartyState.Inventory.Spells.Set(references.AnNox, 2)
	dialog.RefreshCatalog(state.CurrentSpellCatalog())
	// From All known, Ctrl+O steps through Ready to Prepared.
	for _, filter := range []spellBrowserFilter{spellFilterReady, spellFilterPrepared} {
		press(ebiten.KeyControl, ebiten.KeyO)
		require.Equal(t, filter, dialog.browser.filterMode)
	}
	require.Len(t, dialog.browser.filtered, 1)
	require.Equal(t, references.AnNox, dialog.GetSelectedSpell())
	require.Equal(t, "poison", strings.ToLower(dialog.browser.query))
	press(ebiten.KeySpace)
	press(ebiten.Key2)
	require.Equal(t, "poison 2", strings.ToLower(dialog.browser.query))
	require.Zero(t, dialog.selectedCasterIdx)
	press(ebiten.KeyEnter)
	require.False(t, dialog.HasPendingCastRequest())
	require.Empty(t, dialog.notice)
	press(ebiten.KeyControl, ebiten.KeyU)
	require.Empty(t, dialog.browser.query)
}

func TestSpellBrowserLayoutAndResize(t *testing.T) {
	_, dialog, press := newCastBrowserTest(t)
	press(ebiten.KeyV)
	press(ebiten.KeyF)
	for _, size := range config.SupportedWindowResolutions() {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			setResponsiveDialogTestResolution(t, size.X, size.Y)
			dialog.ensureLayout()
			spell := dialog.browser.current()
			require.NotNil(t, spell)
			prepareSpellCatalogRow(dialog.view.row, dialog.view.count, dialog.view.listRect.Inset(dialog.view.padding), *spell, true, dialog.spellAvailable(*spell))
			require.Equal(t, references.VasFlam, dialog.GetSelectedSpell())
			require.Equal(t, GetScaledFont(FontSizeBody).TextFace.Size, dialog.view.font.TextFace.Size)
			require.Equal(t, 2, dialog.view.row.GetPanelContentLineCount())
			require.NotContains(t, dialog.view.row.GetOutputStr(false), "(Low MP)")
			require.Contains(t, dialog.view.row.GetOutputStr(false), "Fireball")
			require.GreaterOrEqual(t, dialog.browser.visible, 1)
			require.GreaterOrEqual(t, dialog.browser.pages, 1)
			footer := dialog.view.footerRect.Inset(dialog.view.padding)
			require.LessOrEqual(t, dialog.view.footer.GetTotalHeightForLines(dialog.view.footer.GetPanelContentLineCount()), float64(footer.Dy()))
		})
	}
}

func TestSpellBrowserKnowledgeIsConsistent(t *testing.T) {
	state, cast, _ := newCastBrowserTest(t)
	mix := NewReagentMixingDialog(cast.keyboard, state.CurrentSpellCatalog(), nil)
	stats := newTestZStatsDossierDialog(state.CurrentZStatsDossier(), cast.keyboard)
	require.Equal(t, cast.browser.catalog, mix.browser.catalog)
	require.Equal(t, cast.browser.catalog, stats.dossier.Magic.Spells)
	for _, spell := range cast.browser.catalog {
		require.NotEqual(t, references.AnYlem.ID(), spell.ID)
	}
}

func TestLeavingSpellSearchRetainsTheResultInsteadOfItsOrdinal(t *testing.T) {
	_, dialog, press := newCastBrowserTest(t)
	press(ebiten.KeyV)
	press(ebiten.KeyF)
	press(ebiten.KeySlash)
	for _, key := range []ebiten.Key{ebiten.KeyH, ebiten.KeyE, ebiten.KeyA, ebiten.KeyL} {
		press(key)
	}
	selected := dialog.GetSelectedSpell()
	require.NotEqual(t, references.InvalidSpell, selected)
	require.NotEqual(t, references.VasFlam, selected)
	press(ebiten.KeyEscape)
	require.False(t, dialog.browser.searching)
	require.Equal(t, selected, dialog.GetSelectedSpell())
	press(ebiten.KeySpace)
	require.Equal(t, selected, dialog.GetSelectedSpell())
}

func TestCasterTabTraversalReversesWithEitherShiftKey(t *testing.T) {
	for _, shift := range []ebiten.Key{ebiten.KeyShiftLeft, ebiten.KeyShiftRight} {
		_, dialog, press := newCastBrowserTest(t)
		press(ebiten.KeyTab)
		require.NotZero(t, dialog.selectedCasterIdx)
		dialog.keyboard.SetAllowKeyPressImmediately()
		press(shift, ebiten.KeyTab)
		require.Zero(t, dialog.selectedCasterIdx)
	}
}
