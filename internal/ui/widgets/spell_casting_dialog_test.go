package widgets

import (
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

func newCastBrowserTest(t *testing.T) (*game_state.GameState, *SpellCastingDialog, func(...ebiten.Key)) {
	t.Helper()
	setResponsiveDialogTestResolution(t, 1280, 720)
	state, _ := game_state.NewIntegrationTestBuilder(t).WithLocation(references.Britain).WithSystemCallbacks().Build()
	state.PartyState.GetAvatar().Class = party_state.Wizard
	state.PartyState.GetAvatar().Level = 8
	state.PartyState.GetAvatar().SetMp(30)
	held := map[ebiten.Key]bool{}
	keyboard := input.NewKeyboardWithKeyStateSource(165, func(key ebiten.Key) bool { return held[key] })
	dialog := NewSpellCastingDialog(keyboard, &state.PartyState, references.ContextTown,
		state.CurrentSpellCatalog(), state.ValidateSpellTargetingPrerequisites)
	press := func(keys ...ebiten.Key) {
		for _, key := range keys {
			held[key] = true
		}
		dialog.Update()
		clear(held)
		keyboard.ClearFrameConsumption()
		dialog.Update()
	}
	return state, dialog, press
}

func TestSpellCastingDialogUsesAuthoritativeReadiness(t *testing.T) {
	state, dialog, _ := newCastBrowserTest(t)
	dialog.browser.selectID(references.InLor.ID())
	state.PartyState.Inventory.Spells.Set(references.InLor, 1)
	caster := state.PartyState.GetAvatar()
	caster.Class = party_state.Fighter
	caster.SetMp(0)
	valid, message := dialog.getSpellValidationStatus()
	require.False(t, valid)
	require.Equal(t, game_state.SpellNoMagicAbilityMessage, message)
	caster.Class = party_state.Wizard
	valid, message = dialog.getSpellValidationStatus()
	require.False(t, valid)
	require.Equal(t, game_state.SpellNoMPMessage, message)
	dialog.attemptCast()
	require.False(t, dialog.IsDismissed())
	require.Equal(t, game_state.SpellNoMPMessage, dialog.notice)
	require.False(t, dialog.HasPendingCastRequest())
	caster.SetMp(10)
	dialog.attemptCast()
	require.True(t, dialog.HasPendingCastRequest())
	require.Equal(t, references.InLor, dialog.ConsumePendingCastRequest().Spell)
	require.Equal(t, uint16(1), state.PartyState.Inventory.Spells.Get(references.InLor))
	require.Equal(t, byte(10), caster.GetCurrentMp())
}

func TestSpellCastingDialogLockedCasterAndNonCasterBrowsing(t *testing.T) {
	state, dialog, press := newCastBrowserTest(t)
	dialog.LockCasterSelection(2)
	press(ebiten.KeyTab)
	dialog.selectCaster(0)
	require.Equal(t, 2, dialog.selectedCasterIdx)
	state.PartyState.GetCharacter(2).Class = party_state.Fighter
	dialog.ensureLayout()
	dialog.prepareStatus(dialog.view.detailRect.Inset(dialog.view.padding))
	require.NotEmpty(t, dialog.view.body.GetOutputStr(false), "non-casters may still browse and prepare")
	require.Contains(t, dialog.view.status.GetOutputStr(false), "No magical ability")
}

func TestSpellCastingDialogMixIntentPreservesSelectionAndRefreshesStock(t *testing.T) {
	state, dialog, press := newCastBrowserTest(t)
	press(ebiten.KeyA)
	press(ebiten.KeyN)
	require.Equal(t, references.AnNox, dialog.GetSelectedSpell())
	press(ebiten.KeyControl, ebiten.KeyM)
	require.True(t, dialog.HasPendingMixRequest())
	require.Equal(t, "AN", dialog.browser.syllables, "Ctrl+M cannot type Mani")
	require.Equal(t, references.AnNox, dialog.ConsumePendingMixSpell())
	state.PartyState.Inventory.Spells.Set(references.AnNox, 5)
	dialog.RefreshCatalog(state.CurrentSpellCatalog())
	require.Equal(t, references.AnNox, dialog.GetSelectedSpell())
	require.Equal(t, "AN", dialog.browser.syllables)
	require.Equal(t, uint16(5), dialog.browser.current().Prepared)
	dialog.currentContext = references.ContextCombat
	press(ebiten.KeyControl, ebiten.KeyM)
	require.False(t, dialog.HasPendingMixRequest())
	require.Equal(t, "Cannot mix during combat.", dialog.notice)
}

func TestSpellCastingDialogDebugOverridesUseCoreValidation(t *testing.T) {
	state, dialog, _ := newCastBrowserTest(t)
	state.PartyState.GetAvatar().SetMp(0)
	state.PartyState.GetAvatar().Level = 1
	state.PartyState.Inventory.Spells.Set(references.AnTym, 0)
	dialog.browser.selectID(references.AnTym.ID())
	state.DebugOptions.UnlimitedCastingResources = true
	state.DebugOptions.BypassSpellLevelRestrictions = true
	valid, _ := dialog.getSpellValidationStatus()
	require.True(t, valid)
	state.DebugOptions.BypassSpellLevelRestrictions = false
	valid, message := dialog.getSpellValidationStatus()
	require.False(t, valid)
	require.Equal(t, "Level too low!", message)
}

// In battle the list opens on what the acting caster can cast right now, so
// spells that are mixed but beyond them are out of the way. Ctrl+O steps on
// through the other filters, and Ready follows a change of caster.
func TestCastListOpensOnReadySpellsInBattle(t *testing.T) {
	state, _, _ := newCastBrowserTest(t)
	catalog := state.CurrentSpellCatalog()
	ready := map[references.Spell]bool{references.GravPor: true, references.Mani: true}
	validate := func(request game_state.CastRequest) game_state.SpellValidationResult {
		return game_state.SpellValidationResult{
			Valid: request.CasterIdx == 0 && ready[request.Spell], ErrorMsg: "", SpellInfo: nil, IsAbsorb: false,
		}
	}
	open := func(context references.SpellContext) *SpellCastingDialog {
		return NewSpellCastingDialog(newWidgetTestKeyboard(0), &state.PartyState, context, catalog, validate)
	}

	battle := open(references.ContextCombat)
	battle.LockCasterSelection(0)
	require.Equal(t, spellFilterReady, battle.browser.filterMode)
	require.Len(t, battle.browser.filtered, len(ready))
	require.Equal(t, references.GravPor, battle.GetSelectedSpell())
	var cycle []spellBrowserFilter
	for range 4 {
		battle.browser.filterMode = battle.browser.nextFilter()
		cycle = append(cycle, battle.browser.filterMode)
	}
	require.Equal(t, []spellBrowserFilter{spellFilterPrepared, spellFilterAll, spellFilterMixable, spellFilterReady}, cycle)

	// Words that match no ready spell widen the list, so a known spell is always
	// found by name and can say why it cannot be cast.
	battle.browser.syllables = spellInitials("Vas Mani")
	battle.browser.filter()
	require.Equal(t, spellFilterAll, battle.browser.filterMode)
	require.Equal(t, references.VasMani, battle.GetSelectedSpell())

	// Outside battle the list opens as before; Ready is there and follows the caster.
	towne := open(references.ContextTown)
	require.Equal(t, spellFilterAll, towne.browser.filterMode)
	require.Len(t, towne.browser.filtered, len(catalog))
	towne.browser.filterMode = spellFilterReady
	towne.browser.refilter()
	require.Len(t, towne.browser.filtered, len(ready))
	towne.cycleCaster(1)
	require.NotZero(t, towne.selectedCasterIdx, "fixture: a second caster")
	require.Empty(t, towne.browser.filtered, "the new caster can cast none of them")

	// With nothing ready, battle shows every known spell rather than an empty list.
	clear(ready)
	nothing := open(references.ContextCombat)
	nothing.LockCasterSelection(0)
	require.Equal(t, spellFilterAll, nothing.browser.filterMode)
	require.Len(t, nothing.browser.filtered, len(catalog))

	// Where no caster is chosen (Z-stats, Mix) the filters are as they were.
	plain := newSpellBrowser(catalog)
	cycle = cycle[:0]
	for range 3 {
		plain.filterMode = plain.nextFilter()
		cycle = append(cycle, plain.filterMode)
	}
	require.Equal(t, []spellBrowserFilter{spellFilterPrepared, spellFilterMixable, spellFilterAll}, cycle)
}
