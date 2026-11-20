package widgets

import (
	"fmt"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var _ Widget = (*SpellCastingDialog)(nil)

// SpellCastingDialog browses detached spell knowledge and emits Cast/Mix intents.
// The owner validates the selected caster and commits resources after targeting.
type SpellCastingDialog struct {
	browser               *spellBrowser
	view                  spellPresentation
	keyboard              *input.Keyboard
	partyState            *party_state.PartyState
	selectedCasterIdx     int
	casterSelectionLocked bool
	currentContext        references.SpellContext
	validate              func(game_state.CastRequest) game_state.SpellValidationResult
	pendingCastRequest    *game_state.CastRequest
	pendingMixSpell       *references.Spell
	notice                string
	isDismissed           bool
	mixInProgress         bool
}

// NewSpellCastingDialog requires authoritative read-only validation, keeping
// readiness and final commitment on the same gameplay rules.
func NewSpellCastingDialog(keyboard *input.Keyboard, party *party_state.PartyState,
	context references.SpellContext, catalog []game_state.ZStatsSpell,
	validate func(game_state.CastRequest) game_state.SpellValidationResult,
) *SpellCastingDialog {
	if keyboard == nil || party == nil || validate == nil {
		panic("SpellCastingDialog: keyboard, party and validation are required")
	}
	//nolint:exhaustruct_v5 // Optional intents and selection state start empty; layout initializes presentation.
	dialog := &SpellCastingDialog{keyboard: keyboard, partyState: party, currentContext: context,
		browser: newSpellBrowser(catalog), validate: validate}
	dialog.browser.ready, dialog.browser.readyFirst = dialog.spellAvailable, true
	dialog.browser.refilter()
	dialog.view.entry = dialog.appendEntry
	dialog.ensureLayout()

	return dialog
}

func (d *SpellCastingDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	d.view.layout(width, height)
	d.view.configureDetails(d.browser, d.writeFooter)
}

// RefreshCatalog retains the filter, caster and selected identity after mixing.
func (d *SpellCastingDialog) RefreshCatalog(catalog []game_state.ZStatsSpell) {
	d.browser.refresh(catalog)
	d.mixInProgress = false
	d.notice = ""
}

func (d *SpellCastingDialog) onDialogFocusChanged(_ bool) {
	d.browser.navigation.BlockUntilRelease(d.keyboard.IsKeyHeld)
}

// Update routes modal keyboard input without committing gameplay.
func (d *SpellCastingDialog) Update() {
	if d.isDismissed || d.mixInProgress || d.pendingCastRequest != nil || d.pendingMixSpell != nil {
		return
	}
	d.ensureLayout()
	if d.handleMixShortcut() {
		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEnter) || d.keyboard.IsKeyPressedAndRegister(ebiten.KeyNumpadEnter) {
		d.attemptCast()

		return
	}
	if d.handleCasterKeys() {
		return
	}
	previousSpell, previousQuery := d.GetSelectedSpell(), d.browser.query
	if d.browser.handleInput(d.keyboard, d.view.font) {
		if previousSpell != d.GetSelectedSpell() || previousQuery != d.browser.query {
			d.notice = ""
		}

		return
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyEscape) {
		d.Dismiss()
	}
}

// handleMixShortcut requests mixing of the selected spell on Ctrl+M, which is
// refused during combat. It reports whether the shortcut was pressed.
func (d *SpellCastingDialog) handleMixShortcut() bool {
	if !d.keyboard.IsModifierPressed(input.ModifierCtrl) || !d.keyboard.IsTextKeyPressedAndRegister(ebiten.KeyM) {
		return false
	}
	if d.currentContext == references.ContextCombat {
		d.notice = "Cannot mix during battle."
	} else if spell := d.GetSelectedSpell(); spell != references.InvalidSpell {
		d.pendingMixSpell = &spell
	}

	return true
}

// handleCasterKeys changes an unlocked caster with Tab/Shift+Tab, or with a
// party digit while not searching. It reports whether the caster key was used.
func (d *SpellCastingDialog) handleCasterKeys() bool {
	if d.casterSelectionLocked {
		return false
	}
	if d.keyboard.IsKeyPressedAndRegister(ebiten.KeyTab) {
		direction := 1
		if d.keyboard.IsModifierPressed(input.ModifierShift) {
			direction = -1
		}
		d.cycleCaster(direction)

		return true
	}
	if d.browser.searching {
		return false
	}
	if digit := d.keyboard.IsNumberKeyPressedAndRegister(); digit >= 1 && digit <= 6 {
		d.selectCaster(digit - 1)

		return true
	}

	return false
}

// Draw renders the shared catalog with a caster strip. Whether Enter will work is
// the first line of the selected spell's details.
func (d *SpellCastingDialog) Draw(screen *ebiten.Image) {
	if d.isDismissed {
		return
	}
	d.ensureLayout()
	view := &d.view
	view.drawFrame(screen)
	header := view.headerRect.Inset(view.padding)
	prepareSpellOutput(view.title, header)
	view.title.AppendLine().Text("CAST", DialogHeaderColor).Done()
	if caster := d.getSelectedCaster(); caster != nil {
		line := appendSpellField(view.title.AppendLine(), spellCasterLabel, caster.GetNameAsString())
		line.Text("   ", DialogValueColor)
		appendSpellField(line, "Level", strconv.Itoa(int(caster.Level))).Text("   ", DialogValueColor)
		appendSpellField(line, "MP", fmt.Sprintf("%d / %d", caster.GetCurrentMp(), caster.GetMaxMp())).Done()
	}
	drawSpellOutput(screen, view.title, header, 0, spellBrowserRowLines)
	view.drawSearch(screen, d.browser, view.searchRect.Inset(view.padding))
	view.drawList(screen, d.browser, view.listRect.Inset(view.padding), d.spellAvailable)
	view.drawDetails(screen, d.browser)
	view.drawFooter(screen)
	view.finishFrame(screen)
}

// appendEntry writes what a caster needs to know: whether Enter will work and
// why not, then what the spell does, what it costs and what it is aimed at.
// Recipes and stock belong to Mix and Z-Stats.
func (d *SpellCastingDialog) appendEntry(output *text.Output, spell game_state.ZStatsSpell) {
	d.appendStatus(output.AppendLine(), spell).WordWrap().Done()
	output.AppendLine().Text(spell.Description, DialogValueColor).WordWrap().Done()
	appendSpellField(output.AppendLine(), "Prepared", strconv.Itoa(int(spell.Prepared))).Done()
	line := appendSpellField(output.AppendLine(), "Circle", strconv.Itoa(spell.Circle)).Text("   ", DialogValueColor)
	appendSpellField(line, "Costs", strconv.Itoa(spell.MPCost)+" MP").Done()
	appendSpellAim(output, spell)
}

// appendStatus is the line under the spell's name. A refusal is a sentence with
// its reason in the red that reads on black.
func (d *SpellCastingDialog) appendStatus(line *text.LineBuilder, spell game_state.ZStatsSpell) *text.LineBuilder {
	if d.notice != "" {
		return line.Text(d.notice, DialogWarningColor)
	}
	result := d.check(references.Spell(spell.ID))
	if result.Valid {
		return line.Text("Ready to cast.", DialogLabelColor)
	}
	line.Text(d.refusal(spell, result), DialogWarningColor)
	mixable := usefulSpellMixCount(spell)
	if result.Refusal == game_state.SpellRefusalNoneMixed && mixable > 0 && d.currentContext != references.ContextCombat {
		AppendControlHints(line.Text(" ", DialogControlHintColor), fmt.Sprintf("[Ctrl+M] Mix up to %d", mixable))
	}

	return line
}

// refusal says why the selected caster cannot cast the spell here, with the
// figure that matters. A refusal with no sentence of its own keeps the game's words.
func (d *SpellCastingDialog) refusal(spell game_state.ZStatsSpell, result game_state.SpellValidationResult) string {
	if result.IsAbsorb {
		return "Magic is absorbed here."
	}
	caster := d.getSelectedCaster()
	switch result.Refusal {
	case game_state.SpellRefusalNoMagic:
		return caster.GetNameAsString() + " has no magical ability."
	case game_state.SpellRefusalNotHere:
		where := references.GetSpellInfo(references.Spell(spell.ID)).Context
		if where.HasContext(d.currentContext) {
			return "Cannot be cast here."
		}

		return "Cannot be cast here: " + where.OnlyLabel() + "."
	case game_state.SpellRefusalNotUntilVictory:
		return "Not until the battle is won."
	case game_state.SpellRefusalNoneMixed:
		return "None prepared."
	case game_state.SpellRefusalNoMP:
		return fmt.Sprintf("Not enough MP: costs %d, %s has %d.",
			spell.MPCost, caster.GetNameAsString(), caster.GetCurrentMp())
	case game_state.SpellRefusalLevelTooLow:
		return fmt.Sprintf("%s cannot cast the %s Circle yet.", caster.GetNameAsString(), circleOrdinal(spell.Circle))
	case game_state.SpellRefusalOther:
		return result.ErrorMsg
	default:
		panic("SpellCastingDialog: unknown spell refusal")
	}
}

func circleOrdinal(circle int) string {
	irregular := [...]string{"1st", "2nd", "3rd"}
	if circle >= 1 && circle <= len(irregular) {
		return irregular[circle-1]
	}

	return strconv.Itoa(circle) + "th"
}

// check asks the rules whether the selected caster may begin casting the spell.
// Targeting follows selection, so no target is part of the question.
func (d *SpellCastingDialog) check(spell references.Spell) game_state.SpellValidationResult {
	var request game_state.CastRequest
	request.Spell, request.CasterIdx = spell, d.selectedCasterIdx

	return d.validate(request)
}

// entersWorkflow reports whether Enter hands the spell on. Absorption feedback
// is workflow-owned, so an absorbed cast still goes through.
func (d *SpellCastingDialog) entersWorkflow(spell references.Spell) bool {
	result := d.check(spell)

	return result.Valid || result.IsAbsorb
}

func (d *SpellCastingDialog) spellAvailable(spell game_state.ZStatsSpell) bool {
	return d.check(references.Spell(spell.ID)).Valid
}

// enterAction names what Enter leads to for the spell here, so the key itself
// says whether this press spends the mixture or opens a step that can be left.
func (d *SpellCastingDialog) enterAction(spell references.Spell) string {
	inBattle := d.currentContext == references.ContextCombat
	switch references.GetSpellTargetingProfile(spell, d.currentContext).Selection {
	case references.TargetAutomatic:
		return "Cast now"
	case references.TargetPartyMember, references.TargetCombatPartyMember:
		return "Choose who"
	case references.TargetMoonPhase:
		return "Name the phase"
	case references.TargetDirection, references.TargetAdjacentTile:
		if !inBattle {
			return "Choose direction"
		}

		return spellAimAction
	case references.TargetCombatantCrosshair, references.TargetTileCrosshair:
		return spellAimAction
	default:
		panic("SpellCastingDialog: unknown spell targeting")
	}
}

// Dismiss closes the selector; its workflow removes the exact modal instance.
func (d *SpellCastingDialog) Dismiss() { d.isDismissed = true }

// IsDismissed implements Widget.
func (d *SpellCastingDialog) IsDismissed() bool { return d.isDismissed }

// GetSelectedSpell returns InvalidSpell when no visible result is selected.
func (d *SpellCastingDialog) GetSelectedSpell() references.Spell {
	if spell := d.browser.current(); spell != nil {
		return references.Spell(spell.ID)
	}

	return references.InvalidSpell
}

// HasPendingMixRequest reports a Ctrl+M intent.
func (d *SpellCastingDialog) HasPendingMixRequest() bool { return d.pendingMixSpell != nil }

// ConsumePendingMixSpell suspends input until RefreshCatalog resumes the selector.
func (d *SpellCastingDialog) ConsumePendingMixSpell() references.Spell {
	spell := *d.pendingMixSpell
	d.pendingMixSpell = nil
	d.mixInProgress = true

	return spell
}

// HasPendingCastRequest reports an Enter intent.
func (d *SpellCastingDialog) HasPendingCastRequest() bool { return d.pendingCastRequest != nil }

// ConsumePendingCastRequest transfers targeting/commit ownership to the workflow.
func (d *SpellCastingDialog) ConsumePendingCastRequest() game_state.CastRequest {
	request := *d.pendingCastRequest
	d.pendingCastRequest = nil

	return request
}

// attemptCast hands a spell that can be cast to the workflow. A refused spell
// stays put: the line under its name already says why.
func (d *SpellCastingDialog) attemptCast() {
	spell := d.GetSelectedSpell()
	if spell == references.InvalidSpell {
		return
	}
	if d.entersWorkflow(spell) {
		var request game_state.CastRequest
		request.Spell, request.CasterIdx = spell, d.selectedCasterIdx
		d.pendingCastRequest = &request
	}
}

func (d *SpellCastingDialog) selectCaster(index int) {
	if d.casterSelectionLocked || index < 0 || index >= 6 || !d.partyState.HasCharacter(index) {
		return
	}
	caster := d.partyState.GetCharacter(index)
	if caster.IsInParty() && caster.IsAlive() {
		d.selectedCasterIdx = index
		d.notice = ""
		d.browser.refilter() // Ready depends on who is casting.
	}
}

// SetInitialCaster restores the workflow's previously selected caster and opens
// the list on the first spell that caster can cast.
func (d *SpellCastingDialog) SetInitialCaster(index int) {
	d.selectCaster(index)
	d.browser.selected = 0
}

// LockCasterSelection restricts combat casting to the active actor, whose
// castable spells then head the list.
func (d *SpellCastingDialog) LockCasterSelection(index int) {
	if index < 0 || index >= 6 {
		panic("SpellCastingDialog: invalid caster index")
	}
	d.selectedCasterIdx, d.casterSelectionLocked = index, true
	d.browser.refilter()
	d.browser.selected = 0
}
func (d *SpellCastingDialog) cycleCaster(direction int) {
	if d.casterSelectionLocked {
		return
	}
	for step := 1; step <= zStatsDossierCharacterSelectors; step++ {
		index := (d.selectedCasterIdx + direction*step + zStatsDossierCharacterSelectors) % zStatsDossierCharacterSelectors
		if d.partyState.HasCharacter(index) {
			caster := d.partyState.GetCharacter(index)
			if caster.IsInParty() && caster.IsAlive() {
				d.selectedCasterIdx = index
				d.notice = ""
				d.browser.refilter() // Ready depends on who is casting.

				return
			}
		}
	}
}
func (d *SpellCastingDialog) getSelectedCaster() *party_state.PlayerCharacter {
	if !d.partyState.HasCharacter(d.selectedCasterIdx) {
		return nil
	}

	return d.partyState.GetCharacter(d.selectedCasterIdx)
}
