package main

import (
	"slices"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/text"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
)

// Each entry is an Output row with natural wrapping; empty entries are real
// paragraph-spacing rows. Do not embed newlines or unsupported punctuation.
func combatSpeedHelpLines(spec config.NumericSettingSpec) []string {
	choices := make([]string, 0, len(spec.Choices()))
	for _, value := range spec.Choices() {
		choices = append(choices, config.FormatCombatSpeed(value))
	}

	lines := []string{
		"Change how quickly combat actions and animations play.", "",
		config.FormatCombatSpeed(spec.Default) + " is normal speed.",
		"Higher values make combat faster.",
	}
	if slices.Contains(spec.Choices(), config.CombatSpeedLudicrous) {
		lines = append(lines, "Ludicrous also skips movement and missile animation.")
	}

	return append(lines, "",
		"Choose "+joinGameplayChoices(choices)+".", "",
		"Default: "+config.FormatCombatSpeed(spec.Default),
	)
}

// formatCombatKeyRepeat names a Combat Key Repeat choice: Off, Measured, or a
// Fast rate in actions per second.
func formatCombatKeyRepeat(value float64) string {
	switch rate := int(value); rate {
	case 0:
		return gameplaySettingOffLabel
	case config.CombatKeyRepeatMeasured:
		return "Measured"
	default:
		return "Fast " + strconv.Itoa(rate)
	}
}

func combatKeyRepeatHelpLines() []string {
	return []string{
		"Hold a key in combat to repeat it, for every party member in turn.", "",
		"Measured repeats arrows and Tab about 6 times a second.",
		"Fast also repeats A, up to 10, 20 or 30 times a second.", "",
		"Repeats begin after a short hold. Blocked, a prompt or the end of combat stops them.", "",
		"Default: " + formatCombatKeyRepeat(config.CombatKeyRepeatSpec().Default),
	}
}

func actionPopupsHelpLines(spec config.NumericSettingSpec) []string {
	choices := make([]string, 0, len(spec.Choices()))
	for _, value := range spec.Choices() {
		choice := strconv.Itoa(int(value))
		if value == 0 {
			choice = gameplaySettingOffLabel
		}
		choices = append(choices, choice)
	}

	return []string{
		"Choose how many action messages can appear over the map at once.", "",
		"Off hides these popups.",
		"Messages still appear in the lower-right text area.", "",
		"Choose " + joinGameplayChoices(choices) + ".", "",
		"Default: " + strconv.Itoa(int(spec.Default)),
	}
}

func joinGameplayChoices(choices []string) string {
	last := len(choices) - 1

	return strings.Join(choices[:last], ", ") + " or " + choices[last]
}

func (setting gameplaySettingDefinition) detailLines() []string {
	if setting.helpLines != nil {
		return setting.helpLines
	}

	return []string{setting.detail}
}

// Measurement and drawing populate the same Output, including blank rows and
// wrapping. This avoids estimating paragraph height from concatenated strings.
func populateGameplayDetail(output *text.Output, setting gameplaySettingDefinition, value string) {
	output.Reset()
	output.AppendLine().Text(setting.label, widgets.DialogHeaderColor).WordWrap().Done()
	for _, line := range setting.detailLines() {
		output.AppendLine().Text(line, widgets.DialogValueColor).WordWrap().Done()
	}
	output.AppendLine().Text("Value: ", widgets.DialogLabelColor).Text(value, widgets.DialogValueColor).Done()
}
