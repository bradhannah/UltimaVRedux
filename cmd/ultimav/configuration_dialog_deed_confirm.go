package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Experience already paid for a deed is never topped up or taken back, so a
// changed Deed Experience level is confirmed before it is applied.
// ref:PROGRESS-0004

const deedExperienceConfirmHint = "[Enter] Apply   [Esc] Back"

// deedExperienceChangeNotes says what applying a level does to deed pay. Only
// a level that pays carries a warning.
func deedExperienceChangeNotes(level int) (string, []string) {
	if level == config.DeedExperienceNone {
		return "", []string{"Nothing is taken back.", "Deeds done from now on wait unpaid until this is turned on."}
	}

	return "Permanent once paid.", []string{
		"Deeds not yet paid, including any already done, are paid at " +
			config.FormatDeedExperience(float64(level)) + ".",
		"Deeds already paid do not change, now or later.",
	}
}

func (d *ConfigurationDialog) deedExperienceChanged() bool {
	return d.stagedGameplay.DeedExperience != d.originalGameplay.DeedExperience
}

// beginDeedExperienceConfirmation asks on the setting's own row, whichever
// section Apply was pressed in.
func (d *ConfigurationDialog) beginDeedExperienceConfirmation() {
	if d.currentTab == audioSettingsAudioTab {
		d.leaveAudioTab()
	}
	d.currentTab = audioSettingsGameplayTab
	d.focusedGameplayIndex = gameplayDeedExperienceIndex
	d.ensureGameplaySelectionVisible()
	d.confirmingDeedExperience = true
	d.navigation.Reset()
	d.keyboard.PrepareForTransition(0)
}

// updateDeedExperienceConfirmation applies on Enter and returns to the
// settings on Escape, keeping what is staged.
func (d *ConfigurationDialog) updateDeedExperienceConfirmation() {
	boundKeys := []ebiten.Key{ebiten.KeyEscape, ebiten.KeyEnter, ebiten.KeyNumpadEnter}
	boundKey := d.keyboard.GetBoundKeyPressed(&boundKeys)
	if boundKey == nil || !d.keyboard.TryToRegisterKeyPress(*boundKey) {
		return
	}
	d.confirmingDeedExperience = false
	d.keyboard.PrepareForTransition(0)
	if *boundKey != ebiten.KeyEscape {
		d.applyConfiguration()
	}
}

func (d *ConfigurationDialog) populateDeedExperienceConfirmation(output *text.Output) {
	level := func(value int) string { return config.FormatDeedExperience(float64(value)) }
	paragraph := func(line string, colour color.Color) {
		output.AppendLine().Text("", colour).Done()
		output.AppendLine().Text(line, colour).WordWrap().Done()
	}
	warning, notes := deedExperienceChangeNotes(d.stagedGameplay.DeedExperience)
	output.Reset()
	output.AppendLine().Text(gameplaySettings()[gameplayDeedExperienceIndex].label, widgets.DialogHeaderColor).Done()
	output.AppendLine().Text("Now: ", widgets.DialogLabelColor).
		Text(level(d.originalGameplay.DeedExperience), widgets.DialogValueColor).Done()
	output.AppendLine().Text("Change to: ", widgets.DialogLabelColor).
		Text(level(d.stagedGameplay.DeedExperience), widgets.DialogValueColor).Done()
	if warning != "" {
		paragraph(warning, widgets.DialogStatusWarningColor)
	}
	for _, note := range notes {
		paragraph(note, widgets.DialogValueColor)
	}
	paragraph("Apply this change?", widgets.DialogValueColor)
}
