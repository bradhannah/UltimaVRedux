package main

import (
	"encoding/json"
	"errors"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

// combatSpeedResult is the typed payload of the combat presentation speed command.
type combatSpeedResult struct {
	Percent int `json:"percent"`
}

// executeCombatSpeedDebugCommand persists the master combat presentation speed.
// The in-game Configure dialog owns the player-facing control; this command
// serves the Workbench Active Combat console.
func executeCombatSpeedDebugCommand(
	request debugging.Request,
	configuration *config.UltimaVConfiguration,
) debugging.Result {
	if configuration.ConfigurationEditing() {
		return debugging.FailureResult(request, "configuration_editing", config.ErrConfigurationEditing.Error())
	}
	var percent int
	if err := json.Unmarshal(request.Arguments["percent"], &percent); err != nil {
		return debugging.FailureResult(request, "invalid_argument", "Combat speed requires an integer percent.")
	}
	before := int(configuration.GetCombatSpeedFactor() * debugPercentScale)
	if err := configuration.SetCombatSpeedFactor(float64(percent) / debugPercentScale); err != nil {
		code := "gameplay_configuration_save_failed"
		if errors.Is(err, config.ErrGameplayPreference) {
			code = "invalid_argument"
		}

		return debugging.FailureResult(request, code, err.Error())
	}
	data := combatSpeedResult{Percent: int(configuration.GetCombatSpeedFactor() * debugPercentScale)}
	result := debugSuccessResultWithMode(request, debugging.ExecutionPresentation, "Gameplay preference updated.", data)
	appendSemanticChange(&result.Changes, "gameplay.combat.speedPercent", before, percent)

	return result
}
