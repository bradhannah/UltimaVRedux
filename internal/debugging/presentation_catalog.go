package debugging

import (
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/config"
)

const (
	parameterEnabledName = "Enabled"

	// CommandLightingOverlaySet changes the game-window lighting diagnostic overlay.
	CommandLightingOverlaySet = "overlay.lighting.set"
	// CommandSearchOverlaySet changes the game-window search-location overlay.
	CommandSearchOverlaySet = "overlay.search.set"
	// CommandMoongateOverlaySet changes the game-window moonstone overlay.
	CommandMoongateOverlaySet = "overlay.moongate.set"
	// CommandCombatPresentationSpeedSet is the compatibility ID for master combat speed.
	CommandCombatPresentationSpeedSet = "combat.presentation.speed.set"
)

func presentationCommands() []CommandDefinition {
	const percentScale = 100
	spec := config.CombatSpeedSpec()
	minimumSpeed, maximumSpeed, step := int(spec.Min*percentScale), int(spec.Max*percentScale), int(spec.Step*percentScale)
	speedChoices := make([]Choice, 0, len(spec.Choices()))
	for _, speed := range spec.Choices() {
		speedChoices = append(speedChoices, Choice{
			Value: strconv.Itoa(int(speed * percentScale)), Label: strconv.FormatFloat(speed, 'f', -1, 64) + "x",
		})
	}
	setBoolean := func(id, name, description string, aliases []string) CommandDefinition {
		return CommandDefinition{
			ID: id, Category: "Presentation", Name: name, Description: description, Aliases: aliases,
			Parameters:     []ParameterDefinition{{ID: parameterEnabled, Name: parameterEnabledName, Kind: ParameterBoolean, Required: true, ControlHint: controlHintSelect, Availability: []string{contextGameplay}}},
			ExecutionModes: []ExecutionMode{ExecutionPresentation}, Destructive: false,
		}
	}

	return []CommandDefinition{
		setBoolean(CommandLightingOverlaySet, "Set Lighting Overlay", "Show or hide game-window light-distance diagnostics.", []string{"light overlay"}),
		setBoolean(CommandSearchOverlaySet, "Set Search Overlay", "Show or hide authentic search locations in the game window.", []string{"command search overlay"}),
		setBoolean(CommandMoongateOverlaySet, "Set Moongate Overlay", "Show or hide moonstone and moongate diagnostics in the game window.", []string{"moongate overlay"}),
		{
			ID: CommandCombatPresentationSpeedSet, Category: categoryCombat, Name: "Set Combat Speed",
			Description: "Persist master combat animation and action-pause speed. Higher values shorten presentation without changing combat rules.", Aliases: []string{"combat speed"},
			Parameters:     []ParameterDefinition{{ID: "percent", Name: "Speed Percent", Kind: ParameterInteger, Required: true, Minimum: &minimumSpeed, Maximum: &maximumSpeed, Step: &step, Choices: speedChoices, ControlHint: controlHintNumber}},
			ExecutionModes: []ExecutionMode{ExecutionPresentation}, Destructive: false,
		},
	}
}
