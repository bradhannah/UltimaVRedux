package widgets

import (
	"strings"
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

// castStatusLine is the line under the selected spell's name.
func castStatusLine(dialog *SpellCastingDialog) string {
	dialog.ensureLayout()
	status, _, _ := strings.Cut(dialog.view.body.GetOutputStr(false), "\n")

	return strings.TrimSpace(status)
}

func TestSpellCastingDialogUsesAuthoritativeReadiness(t *testing.T) {
	state, dialog, _ := newCastBrowserTest(t)
	dialog.browser.selectID(references.InLor.ID())
	state.PartyState.Inventory.Spells.Set(references.InLor, 1)
	caster := state.PartyState.GetAvatar()
	caster.Class = party_state.Fighter
	caster.SetMp(0)
	require.Equal(t, caster.GetNameAsString()+" has no magical ability.", castStatusLine(dialog))
	caster.Class = party_state.Wizard
	require.Equal(t, "Not enough MP: costs 1, "+caster.GetNameAsString()+" has 0.", castStatusLine(dialog))
	require.Contains(t, dialog.view.footer.GetOutputStr(false), "[Enter] Cast now")
	dialog.attemptCast()
	require.False(t, dialog.IsDismissed())
	require.False(t, dialog.HasPendingCastRequest(), "a refused spell stays on the list with its reason")
	caster.SetMp(10)
	require.Equal(t, "Ready to cast.", castStatusLine(dialog))
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
	require.Contains(t, castStatusLine(dialog), "has no magical ability.", "non-casters may still browse and prepare")
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
	require.Equal(t, "Cannot mix during battle.", castStatusLine(dialog))
}

func TestSpellCastingDialogDebugOverridesUseCoreValidation(t *testing.T) {
	state, dialog, _ := newCastBrowserTest(t)
	state.PartyState.GetAvatar().SetMp(0)
	state.PartyState.GetAvatar().Level = 1
	state.PartyState.Inventory.Spells.Set(references.AnTym, 0)
	state.DebugOptions.UnlimitedCastingResources = true
	state.DebugOptions.BypassSpellLevelRestrictions = true
	require.True(t, dialog.check(references.AnTym).Valid)
	state.DebugOptions.BypassSpellLevelRestrictions = false
	result := dialog.check(references.AnTym)
	require.False(t, result.Valid)
	require.Equal(t, "Level too low!", result.ErrorMsg)
	require.Equal(t, game_state.SpellRefusalLevelTooLow, result.Refusal)
}

// In and out of battle the list holds every known spell with what the caster
// can cast now first, so the opening rows all answer Enter and the rest stay in
// view for reference. Ctrl+O narrows it, and the order follows a change of caster.
func TestCastListsWhatCanBeCastFirst(t *testing.T) {
	state, _, _ := newCastBrowserTest(t)
	catalog := state.CurrentSpellCatalog()
	ready := map[references.Spell]bool{references.GravPor: true, references.Mani: true}
	validate := func(request game_state.CastRequest) game_state.SpellValidationResult {
		var result game_state.SpellValidationResult
		result.Valid = request.CasterIdx == 0 && ready[request.Spell]

		return result
	}
	listed := func(dialog *SpellCastingDialog, position int) references.Spell {
		return references.Spell(dialog.browser.catalog[dialog.browser.filtered[position]].ID)
	}
	for _, context := range []references.SpellContext{references.ContextCombat, references.ContextTown} {
		dialog := NewSpellCastingDialog(newWidgetTestKeyboard(0), &state.PartyState, context, catalog, validate)
		dialog.SetInitialCaster(0)
		require.Equal(t, spellFilterAll, dialog.browser.filterMode)
		require.Len(t, dialog.browser.filtered, len(catalog))
		require.Equal(t, references.GravPor, dialog.GetSelectedSpell(), "the list opens on a spell that can be cast")
		require.Equal(t, references.Mani, listed(dialog, 1))
		require.Equal(t, references.InLor, listed(dialog, 2), "the rest follow in the book's order")
		var cycle []spellBrowserFilter
		for range 3 {
			dialog.browser.filterMode = dialog.browser.nextFilter()
			cycle = append(cycle, dialog.browser.filterMode)
		}
		require.Equal(t, []spellBrowserFilter{spellFilterReady, spellFilterPrepared, spellFilterAll}, cycle)
	}

	towne := NewSpellCastingDialog(newWidgetTestKeyboard(0), &state.PartyState, references.ContextTown, catalog, validate)
	towne.browser.filterMode = spellFilterReady
	towne.browser.refilter()
	require.Len(t, towne.browser.filtered, len(ready))
	// Words that match no ready spell widen the list, so a known spell is always
	// found by name and can say why it cannot be cast.
	towne.browser.syllables = spellInitials("Vas Mani")
	towne.browser.filter()
	require.Equal(t, spellFilterAll, towne.browser.filterMode)
	require.Equal(t, references.VasMani, towne.GetSelectedSpell())
	towne.browser.clearTyped()
	towne.browser.filter()
	towne.cycleCaster(1)
	require.NotZero(t, towne.selectedCasterIdx, "fixture: a second caster")
	require.Equal(t, references.InLor, listed(towne, 0), "the new caster can cast none of them")

	// Where no caster is chosen (Z-stats, Mix) the filters are as they were.
	plain := newSpellBrowser(catalog)
	var cycle []spellBrowserFilter
	for range 3 {
		plain.filterMode = plain.nextFilter()
		cycle = append(cycle, plain.filterMode)
	}
	require.Equal(t, []spellBrowserFilter{spellFilterPrepared, spellFilterMixable, spellFilterAll}, cycle)
}

// A refusal is a sentence with its reason and the figure that matters; one the
// dialog has no sentence for keeps the game's own words.
func TestCastStatusLineGivesTheReason(t *testing.T) {
	state, _, _ := newCastBrowserTest(t)
	caster := state.PartyState.GetAvatar()
	caster.SetMp(2)
	name := caster.GetNameAsString()
	state.PartyState.Inventory.Spells.Set(references.InLor, 0)
	state.PartyState.Inventory.Reagent.Set(references.SulfurAsh, 12)
	var result game_state.SpellValidationResult
	validate := func(game_state.CastRequest) game_state.SpellValidationResult { return result }
	for _, test := range []struct {
		spell   references.Spell
		context references.SpellContext
		refusal game_state.SpellRefusal
		message string
		want    string
	}{
		{references.GravPor, references.ContextTown, game_state.SpellRefusalNotHere, "", "Cannot be cast here: battle only."},
		{references.InLor, references.ContextCombat, game_state.SpellRefusalNotHere, "", "Cannot be cast here: not in battle."},
		{references.InPor, references.ContextCombat, game_state.SpellRefusalNotHere, "", "Cannot be cast here."},
		{references.VasMani, references.ContextCombat, game_state.SpellRefusalNotUntilVictory, "", "Not until the battle is won."},
		{references.VasMani, references.ContextTown, game_state.SpellRefusalNoMP, "", "Not enough MP: costs 5, " + name + " has 2."},
		{references.VasMani, references.ContextTown, game_state.SpellRefusalLevelTooLow, "", name + " cannot cast the 5th Circle yet."},
		{references.InLor, references.ContextTown, game_state.SpellRefusalNoneMixed, "", "None prepared. [Ctrl+M] Mix up to 12"},
		{references.InLor, references.ContextCombat, game_state.SpellRefusalNoneMixed, "", "None prepared."},
		{references.InLor, references.ContextTown, game_state.SpellRefusalOther, "Not thy turn!", "Not thy turn!"},
	} {
		result.ErrorMsg, result.Refusal = test.message, test.refusal
		dialog := NewSpellCastingDialog(newWidgetTestKeyboard(0), &state.PartyState, test.context,
			state.CurrentSpellCatalog(), validate)
		dialog.browser.selectID(test.spell.ID())
		require.Equal(t, test.want, castStatusLine(dialog))
		dialog.attemptCast()
		require.False(t, dialog.HasPendingCastRequest())
	}
}
