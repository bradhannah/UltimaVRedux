package widgets

import (
	"fmt"
	"image"
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
	d := &SpellCastingDialog{keyboard: keyboard, partyState: party, currentContext: context,
		browser: newSpellBrowser(catalog), validate: validate}
	d.ensureLayout()

	return d
}

func (d *SpellCastingDialog) ensureLayout() {
	width, height := display.GetManager().GetCurrentSize()
	d.view.layout(width, height)
	statusHeight := d.view.lineHeight*spellBrowserRowLines + d.view.padding
	d.view.configureDetails(d.browser, d.controls(), statusHeight, d.appendCastDetails)
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
		if !d.browser.searching || d.browser.current() != nil {
			d.attemptCast()
		}

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
		d.notice = "Cannot mix during combat."
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

// Draw renders the shared catalog with a caster strip and fixed action status.
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
	detail := view.detailRect.Inset(view.padding)
	statusHeight := view.lineHeight * spellBrowserRowLines
	status := image.Rect(detail.Min.X, detail.Max.Y-statusHeight, detail.Max.X, detail.Max.Y)
	view.drawDetails(screen, d.browser)
	d.drawStatus(screen, status)
	view.drawFooter(screen)
	view.finishFrame(screen)
}

func (d *SpellCastingDialog) appendCastDetails(output *text.Output) {
	output.AppendLine().Text("Final confirmation spends MP and one mixture; cancelling beforehand is free.",
		DialogControlHintColor).WordWrap().Done()
	if d.currentContext == references.ContextCombat {
		output.AppendLine().Text("Cancelling before final confirmation preserves the combat action.",
			DialogControlHintColor).WordWrap().Done()
	}
}

func (d *SpellCastingDialog) spellAvailable(spell game_state.ZStatsSpell) bool {
	//nolint:exhaustruct_v5 // List readiness validates prerequisites without selecting a target or spending resources.
	result := d.validate(game_state.CastRequest{Spell: references.Spell(spell.ID), CasterIdx: d.selectedCasterIdx})

	return result.Valid
}

func (d *SpellCastingDialog) drawStatus(screen *ebiten.Image, rect image.Rectangle) {
	d.prepareStatus(rect)
	drawSpellOutput(screen, d.view.status, rect, 0, d.view.status.GetPanelContentLineCount())
}

func (d *SpellCastingDialog) prepareStatus(rect image.Rectangle) {
	output := d.view.status
	prepareSpellOutput(output, rect)
	valid, message := d.getSpellValidationStatus()
	statusColor := DialogStatusWarningColor
	if valid {
		message, statusColor = "Ready to cast", DialogStatusOKColor
	} else if message == "None mixed!" {
		message = "Prepare a mixture first."
	}
	if d.notice != "" {
		message = d.notice
		statusColor = DialogStatusWarningColor
	}
	line := output.AppendLine().Text(message, statusColor)
	if !valid && d.notice == "" && message == "Prepare a mixture first." && d.currentContext != references.ContextCombat {
		AppendControlHints(line.Text(controlHintSeparator, DialogControlHintColor), "[Ctrl+M] Mix")
	}
	line.WordWrap().Done()
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

func (d *SpellCastingDialog) getSpellValidationStatus() (bool, string) {
	if d.GetSelectedSpell() == references.InvalidSpell {
		return false, "No spell selected"
	}
	//nolint:exhaustruct_v5 // Targeting is collected by the workflow after this prerequisite check.
	result := d.validate(game_state.CastRequest{Spell: d.GetSelectedSpell(), CasterIdx: d.selectedCasterIdx})

	return result.Valid, result.ErrorMsg
}
func (d *SpellCastingDialog) attemptCast() {
	d.notice = ""
	if d.GetSelectedSpell() == references.InvalidSpell {
		d.notice = "No spell selected."

		return
	}
	//nolint:exhaustruct_v5 // Targeting follows selection; absorption feedback remains workflow-owned.
	request := game_state.CastRequest{Spell: d.GetSelectedSpell(), CasterIdx: d.selectedCasterIdx}
	result := d.validate(request)
	if !result.Valid && !result.IsAbsorb {
		d.notice = result.ErrorMsg

		return
	}
	d.pendingCastRequest = &request
}
func (d *SpellCastingDialog) selectCaster(index int) {
	if d.casterSelectionLocked || index < 0 || index >= 6 || !d.partyState.HasCharacter(index) {
		return
	}
	caster := d.partyState.GetCharacter(index)
	if caster.IsInParty() && caster.IsAlive() {
		d.selectedCasterIdx = index
		d.notice = ""
	}
}

// SetInitialCaster restores the workflow's previously selected caster.
func (d *SpellCastingDialog) SetInitialCaster(index int) { d.selectCaster(index) }

// LockCasterSelection restricts combat casting to the active actor.
func (d *SpellCastingDialog) LockCasterSelection(index int) {
	if index < 0 || index >= 6 {
		panic("SpellCastingDialog: invalid caster index")
	}
	d.selectedCasterIdx, d.casterSelectionLocked = index, true
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
