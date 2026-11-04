package widgets

import (
	"fmt"
	"image"
	"strconv"
	"strings"

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

// ReagentMixingDialog owns a finder and a preparation view. It never spends
// inventory; only Enter in preparation emits an authoritative Mix request.
type ReagentMixingDialog struct {
	navigation
	keyboard             *input.Keyboard
	browser              *spellBrowser
	view                 spellPresentation
	reagentInventory     []ReagentInfo
	reagentListState     *ScrollableListState
	preparing            bool
	hasPreparation       bool
	returnAfterMix       bool
	spell                game_state.ZStatsSpell
	selection            uint16
	quantityBuffer       string
	maxQuantity          int
	notice               string
	pendingMixRequest    *game_state.MixRequest
	awaitingConfirmation bool
	isDismissed          bool
}

// NewReagentMixingDialog opens the shared finder with detached knowledge and inventory.
func NewReagentMixingDialog(keyboard *input.Keyboard,
	catalog []game_state.ZStatsSpell, reagents []ReagentInfo,
) *ReagentMixingDialog {
	if keyboard == nil {
		panic("ReagentMixingDialog: keyboard required")
	}
	//nolint:exhaustruct_v5 // Preparation and intents start empty; ensureLayout initializes presentation.
	d := &ReagentMixingDialog{keyboard: keyboard, browser: newSpellBrowser(catalog),
		navigation:       newNavigation(keyboard.IsKeyHeld),
		reagentInventory: append([]ReagentInfo(nil), reagents...), reagentListState: NewScrollableListState(len(reagents), 1)}
	d.ensureLayout()

	return d
}

func (d *ReagentMixingDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	d.view.layout(width, height)
	if d.preparing {
		d.view.configureControls(d.controls(), false)
	} else {
		d.view.configureDetails(d.browser, d.controls(), 0, nil)
	}
	selected := d.reagentListState.GetCurrentIndex()
	listHeight := d.view.listRect.Inset(d.view.padding).Dy() - d.view.lineHeight
	visible := max(1, listHeight/spellRowHeight(d.view.row))
	if visible != d.reagentListState.visibleSlots {
		d.reagentListState = NewScrollableListState(len(d.reagentInventory), visible)
		d.reagentListState.SetIndex(selected)
	}
}

// ChooseSpell skips the finder when another spell screen supplied an identity.
func (d *ReagentMixingDialog) ChooseSpell(spell references.Spell, returnAfterMix bool) {
	d.browser.selectID(spell.ID())
	selected := d.browser.current()
	if selected == nil || selected.ID != spell.ID() {
		panic("Mix: requested spell is not in visible catalog")
	}
	d.returnAfterMix = returnAfterMix
	d.prepare(*selected)
}

func (d *ReagentMixingDialog) prepare(spell game_state.ZStatsSpell) {
	d.Reset()
	d.browser.navigation.Reset()
	d.browser.page, d.browser.pages = 0, 1
	if !d.hasPreparation || d.spell.ID != spell.ID || d.spell.Name != spell.Name {
		d.spell = spell
		d.selection = spell.RecipeMask
		d.quantityBuffer = ""
		d.reagentListState.SetIndex(0)
	}
	d.preparing, d.hasPreparation = true, true
	d.notice = ""
	d.maxQuantity = d.maximumForSelection()
}

// Update routes finder or preparation input and emits requests only on confirmation.
func (d *ReagentMixingDialog) Update() {
	if d.isDismissed || d.awaitingConfirmation {
		return
	}
	d.ensureLayout()
	if !d.preparing {
		d.updateFinder()

		return
	}
	if d.handlePreparationControls() {
		return
	}
	for digit := range 10 {
		if d.keyboard.IsTextKeyPressedAndRegister(ebiten.Key0+ebiten.Key(digit)) ||
			d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyNumpad0+ebiten.Key(digit)) {
			d.handleQuantityInput(digit)

			return
		}
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		if d.selection == 0 || d.maxQuantity == 0 || d.getEffectiveQuantity() > d.maxQuantity {
			d.notice = "Select ingredients and a quantity within the maximum."

			return
		}
		d.pendingMixRequest = &game_state.MixRequest{
			Spell: references.Spell(d.spell.ID), ReagentMask: d.selection, Quantity: d.getEffectiveQuantity(),
		}
		d.awaitingConfirmation = true

		return
	}
	d.navigateIngredients()
}

func (d *ReagentMixingDialog) onDialogFocusChanged(focused bool) {
	d.navigation.onDialogFocusChanged(focused)
	d.browser.navigation.BlockUntilRelease(d.keyboard.IsKeyHeld)
}

func (d *ReagentMixingDialog) navigateIngredients() {
	if key, _ := d.Poll(d.keyboard); key != nil {
		navigateScrollableList(d.reagentListState, *key)
	}
}

func (d *ReagentMixingDialog) handlePreparationControls() bool {
	if d.keyboard.IsModifierPressed(input.ModifierCtrl) && d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyF) {
		d.fillUsefulQuantity()

		return true
	}
	if d.keyboard.IsModifierPressed(input.ModifierCtrl) && d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyB) {
		d.preparing = false

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		if d.returnAfterMix {
			d.isDismissed = true
		} else {
			d.preparing = false
			d.hasPreparation = false
		}

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyBackspace) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyDelete) {
		d.quantityBuffer, d.notice = "", ""

		return true
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeySpace) {
		d.toggleReagent()

		return true
	}

	return false
}

func (d *ReagentMixingDialog) fillUsefulQuantity() {
	if d.spell.FormulaState != string(game_state.SpellFormulaConfirmed) || d.spell.RecipeMask == 0 ||
		d.selection != d.spell.RecipeMask {
		d.notice = "Select the confirmed recipe before using maximum that fits."

		return
	}
	room := max(0, spellInventoryLimit-int(d.spell.Prepared))
	if room == 0 {
		d.notice = "No room in prepared stock."

		return
	}
	quantity := min(d.maximumForSelection(), room)
	if quantity == 0 {
		d.notice = "Missing ingredients for this recipe."

		return
	}
	d.quantityBuffer = strconv.Itoa(quantity)
	d.notice = "Quantity set to maximum that fits; Enter mixes."
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
	if d.browser.handleInput(d.keyboard, d.view.font) {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		if d.hasPreparation {
			d.preparing = true
		} else {
			d.isDismissed = true
		}
	}
}

func (d *ReagentMixingDialog) toggleReagent() {
	index := d.reagentListState.GetCurrentIndex()
	if index >= len(d.reagentInventory) {
		return
	}
	reagent := d.reagentInventory[index]
	bit := uint16(1) << uint(reagent.Reagent)
	if reagent.Quantity == 0 && d.selection&bit == 0 {
		return
	}
	d.selection ^= bit
	d.maxQuantity = d.maximumForSelection()
	d.notice = ""
}

func (d *ReagentMixingDialog) maximumForSelection() int {
	if d.selection == 0 {
		return 0
	}
	maximum, found := 99, uint16(0)
	for _, reagent := range d.reagentInventory {
		bit := uint16(1) << uint(reagent.Reagent)
		if d.selection&bit != 0 {
			maximum = min(maximum, reagent.Quantity)
			found |= bit
		}
	}
	if found != d.selection {
		return 0
	}

	return max(0, maximum)
}

func (d *ReagentMixingDialog) handleQuantityInput(digit int) {
	if d.maxQuantity == 0 {
		return
	}
	buffer := d.quantityBuffer
	if d.maxQuantity <= 9 || len(buffer) >= 2 {
		buffer = ""
	}
	if buffer == "" && digit == 0 {
		return
	}
	candidate := buffer + strconv.Itoa(digit)
	quantity, err := strconv.Atoi(candidate)
	if err != nil {
		panic("Mix: nonnumeric quantity")
	}
	if quantity > d.maxQuantity {
		d.notice = fmt.Sprintf("Maximum is %d; digit not accepted.", d.maxQuantity)

		return
	}
	// REDUX DEVIATION: Original mixing reads two digits and zero cancels; ref:MIX-0002.
	// Redux defaults to one and replaces single-digit quantities when max < 10,
	// making editing explicit; Escape remains the cancellation key.
	d.quantityBuffer, d.notice = candidate, ""
}

func (d *ReagentMixingDialog) getEffectiveQuantity() int {
	if d.maxQuantity == 0 {
		return 0
	}
	if d.quantityBuffer == "" {
		return 1
	}
	quantity, err := strconv.Atoi(d.quantityBuffer)
	if err != nil {
		panic("Mix: invalid quantity buffer")
	}

	return quantity
}

// Draw uses the same readable typography and catalog as Cast and Z-stats.
func (d *ReagentMixingDialog) Draw(screen *ebiten.Image) {
	if d.isDismissed {
		return
	}
	d.ensureLayout()
	view := &d.view
	view.drawFrame(screen)
	header := view.headerRect.Inset(view.padding)
	prepareSpellOutput(view.title, header)
	view.title.AppendLine().Text("MIX REAGENTS", DialogHeaderColor).Done()
	if d.preparing {
		label := FitDialogLine(d.spell.Name+" - "+d.spell.EffectLabel, view.title.Font, header.Dx())
		view.title.AppendLine().Text(label, DialogValueColor).Done()
	}
	drawSpellOutput(screen, view.title, header, 0, spellBrowserRowLines)
	if !d.preparing {
		view.drawSearch(screen, d.browser, view.searchRect.Inset(view.padding))
		view.drawList(screen, d.browser, view.listRect.Inset(view.padding), spellMixable)
		view.drawDetails(screen, d.browser)
		view.drawFooter(screen)
	} else {
		d.drawPreparation(screen)
	}
	view.finishFrame(screen)
}

func (d *ReagentMixingDialog) drawPreparation(screen *ebiten.Image) {
	view := &d.view
	status := view.searchRect.Inset(view.padding)
	prepareSpellOutput(view.search, status)
	view.search.AppendLine().Text(d.preparationStatus(), DialogLabelColor).WordWrap().Done()
	line := AppendControlHints(view.search.AppendLine(), "[0-9] Type quantity")
	line.Text("; then ", DialogControlHintColor)
	AppendControlHints(line, fmt.Sprintf("[Enter] Mix %d", d.getEffectiveQuantity())).Done()
	drawSpellOutput(screen, view.search, status, 0, view.search.GetPanelContentLineCount())
	d.drawIngredients(screen)
	rect := view.detailRect.Inset(view.padding)
	prepareSpellOutput(view.heading, rect)
	quantity := d.getEffectiveQuantity()
	appendSpellField(view.heading.AppendLine(), "Quantity", fmt.Sprintf("[%d_]", quantity)).Done()
	appendSpellField(view.heading.AppendLine(), "Prepared",
		fmt.Sprintf("%d / %d", d.spell.Prepared, spellInventoryLimit)).Done()
	drawSpellOutput(screen, view.heading, rect, 0, spellBrowserRowLines)
	rect.Min.Y += view.lineHeight * spellBrowserRowLines
	prepareSpellOutput(view.body, rect)
	d.populatePreparation()
	capacity := max(1, view.body.GetLineCapacity(rect.Dy()))
	d.browser.pages = max(1, (view.body.GetPanelContentLineCount()+capacity-1)/capacity)
	d.browser.page = min(d.browser.page, d.browser.pages-1)
	drawSpellOutput(screen, view.body, rect, d.browser.page*capacity, capacity)
	view.drawFooter(screen)
}

func (d *ReagentMixingDialog) preparationStatus() string {
	if d.notice != "" {
		return d.notice
	}
	if d.selection == 0 {
		return "Choose ingredients."
	}
	if d.maxQuantity == 0 {
		return "Missing ingredients; cannot mix this selection."
	}
	if d.getEffectiveQuantity() > d.maxQuantity {
		return "Quantity exceeds available ingredients."
	}
	if d.spell.RecipeMask != 0 && d.selection == d.spell.RecipeMask {
		return "Ready to mix."
	}

	return "Untested mixture"
}

func (d *ReagentMixingDialog) drawIngredients(screen *ebiten.Image) {
	view := &d.view
	list := view.listRect.Inset(view.padding)
	prepareSpellOutput(view.heading, list)
	view.heading.AppendLine().Text("INGREDIENTS", DialogHeaderColor).Done()
	drawSpellOutput(screen, view.heading, list, 0, 1)
	list.Min.Y += view.lineHeight
	start, end := d.reagentListState.GetVisibleRange()
	rowHeight := spellRowHeight(view.row)
	for index := start; index < end; index++ {
		reagent := d.reagentInventory[index]
		top := list.Min.Y + (index-start)*rowHeight
		row := image.Rect(list.Min.X, top, list.Max.X-view.scrollGutter(), min(list.Max.Y, top+rowHeight))
		d.drawIngredientRow(screen, row, reagent, index == d.reagentListState.GetCurrentIndex())
	}
	drawMasterDetailScrollIndicators(screen, list, start, end, len(d.reagentInventory), rowHeight,
		view.scrollGutter(), view.font, view.indicator)
}

func (d *ReagentMixingDialog) drawIngredientRow(screen *ebiten.Image, rect image.Rectangle,
	reagent ReagentInfo, selected bool,
) {
	output := d.view.row
	nameColor, _ := spellRowColours(selected, reagent.Quantity > 0)
	if selected {
		fillSelectedRow(screen, rect)
	}
	rect = spellRowContentRect(output, rect)
	prepareSpellOutput(output, rect)
	checked := d.selection&(1<<uint(reagent.Reagent)) != 0
	markWidth, _ := output.Font.MeasureString("[X] ")
	line := appendReagentMark(output.AppendLine(), checked, selected)
	line.Text(FitDialogLine(reagent.Name, output.Font, rect.Dx()-int(markWidth)), nameColor).Done()
	labelColor, valueColor := DialogLabelColor, DialogValueColor
	if selected {
		labelColor, valueColor = DialogSelectedLabelColor, DialogSelectedText
	}
	output.AppendLine().Text("Have: ", labelColor).Text(strconv.Itoa(reagent.Quantity), valueColor).Done()
	drawSpellOutput(screen, output, rect, 0, spellBrowserRowLines)
}

func (d *ReagentMixingDialog) populatePreparation() {
	output := d.view.body
	appendSpellField(output.AppendLine(), "Ingredients allow", strconv.Itoa(d.maxQuantity)).Done()
	if d.spell.FormulaState == string(game_state.SpellFormulaConfirmed) {
		room := max(0, spellInventoryLimit-int(d.spell.Prepared))
		appendSpellField(output.AppendLine(), "Room in stock", strconv.Itoa(room)).Done()
	}
	quantity := d.getEffectiveQuantity()
	if d.spell.RecipeMask != 0 && d.selection == d.spell.RecipeMask {
		stock := min(spellInventoryLimit, int(d.spell.Prepared)+quantity)
		appendSpellField(output.AppendLine(), "After mixing", strconv.Itoa(stock)).Done()
		if excess := int(d.spell.Prepared) + quantity - stock; excess > 0 {
			output.AppendLine().Textf("%d excess discarded; full reagent cost.",
				DialogStatusWarningColor, excess).WordWrap().Done()
		}
	} else {
		output.AppendLine().Text("Untested: ingredients may be wasted and cause injury.",
			DialogStatusWarningColor).WordWrap().Done()
	}
	output.AppendLine().Text("INGREDIENT COST", DialogHeaderColor).Done()
	for _, reagent := range d.reagentInventory {
		if d.selection&(1<<uint(reagent.Reagent)) != 0 {
			line := appendSpellField(output.AppendLine(), reagent.Name, strconv.Itoa(quantity))
			line.Text(" / ", DialogValueColor)
			appendSpellField(line, "Remaining", strconv.Itoa(max(0, reagent.Quantity-quantity))).WordWrap().Done()
		}
	}
	if len(d.spell.ReagentCandidates) > 0 {
		for _, candidate := range d.spell.ReagentCandidates {
			appendSpellWarningField(output.AppendLine(), "Untested formula", strings.Join(candidate, " + ")).WordWrap().Done()
		}
	}
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

// Acknowledge refreshes detached inventory once, after the authoritative result.
func (d *ReagentMixingDialog) Acknowledge(outcome game_state.MixOutcome,
	catalog []game_state.ZStatsSpell, reagents []ReagentInfo,
) {
	d.awaitingConfirmation = false
	d.browser.refresh(catalog)
	d.reagentInventory = append(d.reagentInventory[:0], reagents...)
	d.maxQuantity = d.maximumForSelection()
	if !outcome.Success {
		d.isDismissed = outcome.TrapResult != nil
		d.notice = outcome.Message

		return
	}
	if d.returnAfterMix {
		d.isDismissed = true
	} else {
		d.preparing, d.hasPreparation = false, false
		d.quantityBuffer = ""
		d.browser.selectID(d.spell.ID)
	}
}
