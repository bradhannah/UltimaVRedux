package debugging

const (
	commandCategoryDiagnostics = "Diagnostics"

	// CommandGuardsStatus reports current town-guard availability and hostility.
	CommandGuardsStatus = "guards.status"
	// CommandShadowlordStatus reports current Shadowlord life, presence, and assignment state.
	CommandShadowlordStatus = "shadowlord.status"
	// CommandDeathStatus reports current party death state and resurrection consequences.
	CommandDeathStatus = "death.status"
	// CommandDebugOptionsStatus reports frequently used gameplay and renderer diagnostics.
	CommandDebugOptionsStatus = "debug.options.status"
	// CommandMovementFreeMoveSet explicitly enables or disables collision-free movement.
	CommandMovementFreeMoveSet = "movement.free-move.set"
	// CommandTextPlaceholdersSet explicitly enables or disables empty-line placeholders.
	CommandTextPlaceholdersSet = "overlay.text-placeholders.set"
	// CommandDialogBoundsSet explicitly enables or disables dialog text-area bounds.
	CommandDialogBoundsSet = "overlay.dialog-bounds.set"
	// CommandUnlimitedCastingResourcesSet explicitly enables or disables casting resource enforcement.
	CommandUnlimitedCastingResourcesSet = "magic.unlimited-casting-resources.set"
	// CommandSpellLevelRestrictionsBypassSet explicitly enables or disables spell-circle level enforcement.
	CommandSpellLevelRestrictionsBypassSet = "magic.spell-level-restrictions-bypass.set"
)

func operationalCommands() []CommandDefinition {
	setBoolean := func(id, category, name, description string, aliases []string, mode ExecutionMode) CommandDefinition {
		return CommandDefinition{
			ID: id, Category: category, Name: name, Description: description, Aliases: aliases,
			Parameters: []ParameterDefinition{{
				ID: parameterEnabled, Name: "Enabled", Kind: ParameterBoolean, Required: true,
				ControlHint: controlHintSelect, Availability: []string{contextGameplay},
			}},
			ExecutionModes: []ExecutionMode{mode}, Destructive: false,
		}
	}

	return []CommandDefinition{
		{ID: CommandGuardsStatus, Category: "Guards", Name: "Guard Status", Description: "Inspect criminal state, potential guards, and current hostility.", Aliases: []string{"guards status"}, ExecutionModes: []ExecutionMode{ExecutionQuery}},
		{ID: CommandShadowlordStatus, Category: "Shadowlords", Name: "Shadowlord Status", Description: "Inspect life, summoned presence, and deterministic town assignments.", Aliases: []string{"shadowlord status"}, ExecutionModes: []ExecutionMode{ExecutionQuery}},
		{ID: CommandDeathStatus, Category: "Death", Name: "Death Status", Description: "Inspect party mortality and authoritative resurrection consequences.", Aliases: []string{"death status"}, ExecutionModes: []ExecutionMode{ExecutionQuery}},
		{ID: CommandDebugOptionsStatus, Category: commandCategoryDiagnostics, Name: "Debug Options", Description: "Inspect gameplay overrides and game-window diagnostic rendering options.", ExecutionModes: []ExecutionMode{ExecutionQuery}},
		setBoolean(CommandMovementFreeMoveSet, "Movement", "Set Free Move", "Explicitly enable or disable collision-free Avatar movement.", []string{"mv freemove"}, ExecutionLiveSafe),
		setBoolean(CommandTextPlaceholdersSet, commandCategoryDiagnostics, "Set Text Placeholders", "Show or hide empty-line placeholders in game-window text output.", []string{"debug conversation textplaceholders"}, ExecutionPresentation),
		setBoolean(CommandDialogBoundsSet, commandCategoryDiagnostics, "Set Dialog Bounds", "Show or hide dialog text-area bounds in the game window.", []string{"debug dialog boxes"}, ExecutionPresentation),
		setBoolean(CommandUnlimitedCastingResourcesSet, "Magic", "Set Unlimited Casting Resources", "Cast without prepared spells or current MP consumption while preserving magical-ability requirements.", []string{"unlimited magic", "infinite spells", "infinite mp"}, ExecutionLiveSafe),
		setBoolean(CommandSpellLevelRestrictionsBypassSet, "Magic", "Set Spell Level Bypass", "Allow magical characters to cast spells above their current level.", []string{"all spell circles", "ignore spell level"}, ExecutionLiveSafe),
	}
}
