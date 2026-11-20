package widgets

import (
	"fmt"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*ReagentMixingDialog)(nil)

// ReagentInfo is detached inventory for the preparation preview.
type ReagentInfo struct {
	Reagent     references.Reagent
	Name        string
	Quantity    int
	SpriteIndex int
}

// mixTab is one page of the quantity window.
type mixTab int

const (
	// mixTabRecipe mixes the recorded recipe: one amount and the rows it changes.
	mixTabRecipe mixTab = iota
	// mixTabExperiment lists every ingredient with a tick box, for trying a mixture.
	mixTabExperiment
)

// ReagentMixingDialog owns the spell list and the quantity window. It never
// spends inventory; only Enter in the quantity window emits a Mix request.
type ReagentMixingDialog struct {
	navigation
	keyboard         *input.Keyboard
	browser          *spellBrowser
	view             spellPresentation
	window           mixWindow
	reagentInventory []ReagentInfo
	preparing        bool
	returnAfterMix   bool
	spell            game_state.ZStatsSpell
	tab              mixTab
	selection        uint16 // The Experiment tab's ticked ingredients.
	cursor           int    // The Experiment tab's highlighted ingredient.
	quantity         int    // At least one, and no more than the ingredients allow.
	typedDigits      int    // Digits typed in a row, so a second digit joins the first.
	notice           string
	// result reports the last mix on the list's header; resultLoss marks one
	// that threw mixtures away.
	result               string
	resultLoss           bool
	pendingMixRequest    *game_state.MixRequest
	awaitingConfirmation bool
	isDismissed          bool
}

// NewReagentMixingDialog opens the spell list with detached knowledge and inventory.
func NewReagentMixingDialog(keyboard *input.Keyboard,
	catalog []game_state.ZStatsSpell, reagents []ReagentInfo,
) *ReagentMixingDialog {
	if keyboard == nil {
		panic("ReagentMixingDialog: keyboard required")
	}
	//nolint:exhaustruct_v5 // The quantity window and intents start empty; ensureLayout initializes presentation.
	dialog := &ReagentMixingDialog{keyboard: keyboard, browser: newSpellBrowser(catalog),
		navigation:       newNavigation(keyboard.IsKeyHeld),
		reagentInventory: append([]ReagentInfo(nil), reagents...), quantity: 1}
	// Mix writes every refusal and shortage in the red that reads on black.
	dialog.view.warning = DialogWarningColor
	browser := dialog.browser
	browser.widenMixable = true
	// Open on what can be mixed now. With nothing mixable, an empty list would
	// say less than every known spell and the reason beside each.
	if slices.ContainsFunc(browser.catalog, spellMixable) {
		browser.filterMode = spellFilterMixable
		browser.filter()
	}
	dialog.ensureLayout()

	return dialog
}

func (d *ReagentMixingDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	d.view.layout(width, height)
	if d.preparing {
		d.layoutWindow()
	} else {
		d.view.configureDetails(d.browser, packedSpellFooter(d.controls()))
	}
}

// ChooseSpell skips the list when another spell screen supplied an identity.
func (d *ReagentMixingDialog) ChooseSpell(spell references.Spell, returnAfterMix bool) {
	d.browser.filterMode = spellFilterAll
	d.browser.filter()
	d.browser.selectID(spell.ID())
	selected := d.browser.current()
	if selected == nil || selected.ID != spell.ID() {
		panic("Mix: requested spell is not in visible catalog")
	}
	d.returnAfterMix = returnAfterMix
	d.prepare(*selected)
}

// prepare opens the quantity window: on the recorded recipe when there is one,
// otherwise on the experiment, which is then the only tab.
func (d *ReagentMixingDialog) prepare(spell game_state.ZStatsSpell) {
	d.Reset()
	d.browser.navigation.Reset()
	d.spell = spell
	d.selection = spell.RecipeMask
	d.tab = mixTabRecipe
	if spell.RecipeMask == 0 {
		d.tab = mixTabExperiment
	}
	d.quantity, d.typedDigits, d.cursor = 1, 0, 0
	d.preparing = true
	d.notice = ""
}

// Update routes list or quantity-window input and emits requests only on confirmation.
func (d *ReagentMixingDialog) Update() {
	if d.isDismissed || d.awaitingConfirmation {
		return
	}
	d.ensureLayout()
	if !d.preparing {
		d.updateFinder()

		return
	}
	if d.handleWindowControls() || d.handleDigits() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		if d.amount() == 0 {
			return // The top line already says why nothing can be mixed.
		}
		d.pendingMixRequest = &game_state.MixRequest{
			Spell: references.Spell(d.spell.ID), ReagentMask: d.mask(), Quantity: d.amount(),
		}
		d.awaitingConfirmation = true

		return
	}
	d.navigateWindow()
}

func (d *ReagentMixingDialog) onDialogFocusChanged(focused bool) {
	d.navigation.onDialogFocusChanged(focused)
	d.browser.navigation.BlockUntilRelease(d.keyboard.IsKeyHeld)
}

// navigateWindow steps the amount with Left and Right on either tab, and moves
// the Experiment tab's highlight with Up and Down.
func (d *ReagentMixingDialog) navigateWindow() {
	key, _ := d.Poll(d.keyboard, ebiten.KeyUp, ebiten.KeyDown, ebiten.KeyPageUp, ebiten.KeyPageDown,
		ebiten.KeyLeft, ebiten.KeyRight)
	if key == nil {
		return
	}
	switch *key { //nolint:exhaustive // Poll returns only the keys named above.
	case ebiten.KeyLeft:
		d.setQuantity(d.quantity - 1)
	case ebiten.KeyRight:
		d.setQuantity(d.quantity + 1)
	default:
		if d.tab == mixTabExperiment {
			const halfList = 2
			last := max(0, len(d.reagentInventory)-1)
			d.cursor = max(0, min(last, d.cursor+listNavigationDelta(*key, len(d.reagentInventory)/halfList)))
		}
	}
}

func (d *ReagentMixingDialog) handleWindowControls() bool {
	switch {
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape):
		if d.returnAfterMix {
			d.isDismissed = true
		} else {
			d.preparing = false
		}
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab):
		if d.spell.RecipeMask != 0 {
			d.tab = d.otherTab()
			d.maskChanged()
		}
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeyBackspace), d.keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete):
		d.setQuantity(1)
	case d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace):
		d.toggleReagent()
	case !d.keyboard.IsModifierPressed(input.ModifierCtrl) && d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyM):
		d.setMost()
	default:
		return false
	}

	return true
}

func (d *ReagentMixingDialog) handleDigits() bool {
	for digit := range 10 {
		if d.keyboard.IsTextKeyPressedAndRegister(ebiten.Key0+ebiten.Key(digit)) ||
			d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyNumpad0+ebiten.Key(digit)) {
			d.handleQuantityInput(digit)

			return true
		}
	}

	return false
}

func (d *ReagentMixingDialog) updateFinder() {
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		if spell := d.browser.current(); spell != nil {
			d.prepare(*spell)
		} else if !d.browser.searching && d.browser.filterMode == spellFilterAll && d.browser.syllables != "" {
			// Preserve manual experimentation without revealing an undiscovered formula.
			spell := references.InvalidSpell
			for candidate := range references.Spell(references.TotalSpellCount) {
				if spellInitials(references.GetSpellName(candidate)) == d.browser.syllables {
					spell = candidate

					break
				}
			}
			//nolint:exhaustruct_v5 // An unrecorded spell deliberately exposes no formula, effect or inventory preview.
			d.prepare(game_state.ZStatsSpell{ID: int(spell), Name: d.browser.syllables,
				EffectLabel: "Unrecorded spell", FormulaState: "unknown"})
		}

		return
	}
	asked := d.listRequest()
	if d.browser.handleInput(d.keyboard, d.view.font) {
		// The last mix keeps its line and its row only until the list is asked
		// for something else: new letters, a search or another filter.
		if d.result != "" && d.listRequest() != asked {
			d.result, d.browser.keepID = "", -1
			d.browser.refilter()
		}

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.isDismissed = true
	}
}

// mixListRequest is what the player has asked the list to show.
type mixListRequest struct {
	syllables, query string
	searching        bool
	filter           spellBrowserFilter
}

func (d *ReagentMixingDialog) listRequest() mixListRequest {
	return mixListRequest{syllables: d.browser.syllables, query: d.browser.query,
		searching: d.browser.searching, filter: d.browser.filterMode}
}

// toggleReagent ticks or clears the highlighted ingredient on the Experiment
// tab. An ingredient the party carries none of cannot be added.
func (d *ReagentMixingDialog) toggleReagent() {
	if d.tab != mixTabExperiment || d.cursor >= len(d.reagentInventory) {
		return
	}
	reagent := d.reagentInventory[d.cursor]
	bit := uint16(1) << uint(reagent.Reagent)
	if reagent.Quantity == 0 && d.selection&bit == 0 {
		return
	}
	d.selection ^= bit
	d.maskChanged()
}

// mask is the mixture Enter would request: the recorded recipe on the Recipe
// tab, the ticked ingredients on the Experiment tab.
func (d *ReagentMixingDialog) mask() uint16 {
	if d.tab == mixTabRecipe {
		return d.spell.RecipeMask
	}

	return d.selection
}

// maskChanged keeps the amount within what the new mixture allows.
func (d *ReagentMixingDialog) maskChanged() {
	d.notice, d.typedDigits = "", 0
	if maximum := d.maximum(); maximum > 0 {
		d.quantity = min(d.quantity, maximum)
	}
}

// maximum is how many mixtures the carried ingredients allow, whether or not
// the prepared stock has room for them.
func (d *ReagentMixingDialog) maximum() int {
	mask := d.mask()
	if mask == 0 {
		return 0
	}
	maximum, found := spellInventoryLimit, uint16(0)
	for _, reagent := range d.reagentInventory {
		bit := uint16(1) << uint(reagent.Reagent)
		if mask&bit != 0 {
			maximum = min(maximum, reagent.Quantity)
			found |= bit
		}
	}
	if found != mask {
		return 0
	}

	return max(0, maximum)
}

// room is how many more of the spell the prepared stock can hold.
func (d *ReagentMixingDialog) room() int {
	return max(0, spellInventoryLimit-int(d.spell.Prepared))
}

// amount is how many mixtures Enter would request; zero when none can be mixed.
func (d *ReagentMixingDialog) amount() int {
	if d.maximum() == 0 {
		return 0
	}

	return d.quantity
}

func (d *ReagentMixingDialog) setQuantity(quantity int) {
	if maximum := d.maximum(); maximum > 0 {
		d.quantity = max(1, min(maximum, quantity))
		d.notice, d.typedDigits = "", 0
	}
}

// setMost sets the largest amount that the ingredients allow and the prepared
// stock can hold. With no room at all the amount stays; the top line says why.
func (d *ReagentMixingDialog) setMost() {
	if most := min(d.maximum(), d.room()); most > 0 {
		d.setQuantity(most)
	}
}

func (d *ReagentMixingDialog) handleQuantityInput(digit int) {
	maximum := d.maximum()
	if maximum == 0 {
		return
	}
	const (
		singleDigitMaximum = 9
		decimal            = 10
	)
	candidate, typed := digit, 1
	if maximum > singleDigitMaximum && d.typedDigits == 1 {
		candidate, typed = d.quantity*decimal+digit, typed+1
	}
	if candidate == 0 {
		return
	}
	if candidate > maximum {
		d.notice = fmt.Sprintf("Maximum is %d; digit not accepted.", maximum)

		return
	}
	// REDUX DEVIATION: Original mixing reads two digits and zero cancels; ref:MIX-0002.
	// Redux defaults to one and replaces single-digit quantities when max < 10,
	// making editing explicit; Escape remains the cancellation key.
	d.quantity, d.typedDigits, d.notice = candidate, typed, ""
}

// Draw uses the same readable typography and catalog as Cast and Z-stats.
func (d *ReagentMixingDialog) Draw(screen *ebiten.Image) {
	if d.isDismissed {
		return
	}
	d.ensureLayout()
	if d.preparing {
		d.drawWindow(screen)

		return
	}
	view := &d.view
	view.drawFrame(screen)
	header := view.headerRect.Inset(view.padding)
	prepareSpellOutput(view.title, header)
	view.title.AppendLine().Text("MIX REAGENTS", DialogHeaderColor).Done()
	switch {
	case d.result == "":
		view.title.AppendLine().Text("Choose a spell to mix", DialogControlHintColor).Done()
	case d.resultLoss:
		view.title.AppendLine().Text(d.result, DialogWarningColor).Done()
	default:
		view.title.AppendLine().Text(d.result, DialogStatusOKColor).Done()
	}
	drawSpellOutput(screen, view.title, header, 0, spellBrowserRowLines)
	view.drawSearch(screen, d.browser, view.searchRect.Inset(view.padding))
	view.drawList(screen, d.browser, view.listRect.Inset(view.padding), spellMixable)
	view.drawDetails(screen, d.browser)
	view.drawFooter(screen)
	view.finishFrame(screen)
}

// IsDismissed implements Widget.
func (d *ReagentMixingDialog) IsDismissed() bool { return d.isDismissed }

// HasPendingMixRequest reports a confirmed, unconsumed preparation intent.
func (d *ReagentMixingDialog) HasPendingMixRequest() bool { return d.pendingMixRequest != nil }

// ConsumePendingMixRequest transfers the request to the authoritative owner.
func (d *ReagentMixingDialog) ConsumePendingMixRequest() game_state.MixRequest {
	request := *d.pendingMixRequest
	d.pendingMixRequest = nil

	return request
}

// NeedsResultBox reports whether the owner must show a successful mix in its
// own box. A plain success needs none: the dialog returns to its list and
// reports the mix on the header. Two cases keep the box: a mix opened for one
// spell from Cast or Z-stats, which closes with no Mix list to return to, and a
// mixture that had no recorded recipe, so the player has just learned one.
func (d *ReagentMixingDialog) NeedsResultBox() bool {
	return d.returnAfterMix || d.spell.RecipeMask == 0
}

// Acknowledge refreshes detached inventory once, after the authoritative result.
// A success returns to the list with the result on its header, the typed
// letters cleared and the mixed spell kept in view even when it can no longer
// be mixed.
func (d *ReagentMixingDialog) Acknowledge(outcome game_state.MixOutcome,
	catalog []game_state.ZStatsSpell, reagents []ReagentInfo,
) {
	d.awaitingConfirmation = false
	// Both before the refresh, which lists by what is typed and what is kept.
	switch {
	case !outcome.Success:
		// An earlier success no longer describes the stock.
		d.result, d.browser.keepID = "", -1
	case !d.returnAfterMix:
		d.preparing = false
		d.result, d.resultLoss = mixResultLine(outcome)
		d.browser.clearTyped()
		d.browser.keepID = d.spell.ID
	}
	d.browser.refresh(catalog)
	d.browser.selectID(d.spell.ID)
	d.reagentInventory = append(d.reagentInventory[:0], reagents...)
	switch {
	case !outcome.Success:
		d.isDismissed = outcome.TrapResult != nil
		d.maskChanged()
		d.notice = outcome.Message
	case d.returnAfterMix:
		d.isDismissed = true
	}
}

// mixResultLine says what a successful mix added, and whether any of it was
// lost to the prepared-stock limit.
// REDUX DEVIATION: MIX-0004: original success feedback is brief. Redux states
// the charges added, the stock and any loss to the 99 limit, so a capped mix is
// never silent.
func mixResultLine(outcome game_state.MixOutcome) (string, bool) {
	switch {
	case outcome.Added >= outcome.Quantity:
		return fmt.Sprintf("Mixed %d %s. You now have %d.", outcome.Added, outcome.SpellName, outcome.Stock), false
	case outcome.Added == 0:
		return fmt.Sprintf("No room for %d %s. You still have %d.",
			outcome.Quantity, outcome.SpellName, outcome.Stock), true
	default:
		return fmt.Sprintf("Only %d of %d %s fit. You now have %d.",
			outcome.Added, outcome.Quantity, outcome.SpellName, outcome.Stock), true
	}
}
