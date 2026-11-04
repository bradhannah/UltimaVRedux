package debugging

import "encoding/json"

const (
	categoryTestSetups = "Test Setups"
	// CommandTestHoleUp queues the production camping workflow with explicit test branches.
	CommandTestHoleUp = "command-test.hole-up"
	// CommandTestSearch queues the production map-specific Search action.
	CommandTestSearch = "command-test.search"
	// CommandMoongateTravel queues production moongate travel from reviewed live state.
	CommandMoongateTravel = "moongate.travel"
)

func thirdBatchCommands() []CommandDefinition {
	minimumHours, maximumHours := 1, 24
	minimumRosterIndex, maximumRosterIndex := 0, 15

	return []CommandDefinition{
		{
			ID: CommandTestHoleUp, Category: categoryTestSetups, Name: "Test Hole Up",
			Description: "Queue production camping with explicit wanderer and ambush branches against a reviewed party roster.",
			Aliases:     []string{"command holeup"},
			Parameters: []ParameterDefinition{
				{ID: "hours", Name: "Hours", Kind: ParameterInteger, Required: true, Minimum: &minimumHours, Maximum: &maximumHours, ControlHint: controlHintNumber, Availability: []string{contextGameplay}},
				{ID: "forceWanderer", Name: "Force Wanderer", Kind: ParameterBoolean, Required: false, Default: json.RawMessage(`false`), Availability: []string{contextGameplay}},
				{ID: "forceAmbush", Name: "Force Ambush", Kind: ParameterBoolean, Required: false, Default: json.RawMessage(`false`), Availability: []string{contextGameplay}},
				{ID: "expectedPartyFingerprint", Name: "Reviewed Party", Kind: ParameterString, Required: true, Availability: []string{contextGameplay}},
				{ID: "guardRosterIndex", Name: "Guard Roster Index", Description: "Optional stable active-party roster index.", Kind: ParameterInteger, Required: false, Minimum: &minimumRosterIndex, Maximum: &maximumRosterIndex, ControlHint: controlHintNumber, Availability: []string{contextGameplay}},
			},
			ExecutionModes: []ExecutionMode{ExecutionWorkflow}, Destructive: false,
		},
		{
			ID: CommandTestSearch, Category: categorySearch, Name: "Test Search",
			Description: "Queue the production Search action using dispatch for the current map type.",
			Aliases:     []string{"command search test"},
			Parameters: []ParameterDefinition{{
				ID: "direction", Name: "Direction", Kind: ParameterEnum, Required: true,
				Choices: []Choice{
					{Value: "up", Label: "Up", Metadata: nil}, {Value: "down", Label: "Down", Metadata: nil},
					{Value: "left", Label: "Left", Metadata: nil}, {Value: "right", Label: "Right", Metadata: nil},
				},
				ControlHint: controlHintSelect, Availability: []string{contextGameplay},
			}},
			ExecutionModes: []ExecutionMode{ExecutionWorkflow}, Destructive: false,
		},
		{
			ID: CommandMoongateTravel, Category: "Moongates", Name: "Travel Through Moongate",
			Description: "Queue production moongate travel only while source and route still match reviewed status.",
			Aliases:     []string{"moongate teleport"},
			Parameters: []ParameterDefinition{
				{ID: "expectedSourceFingerprint", Name: "Reviewed Source", Kind: ParameterString, Required: true, Availability: []string{contextGameplay}},
				{ID: "expectedRouteFingerprint", Name: "Reviewed Route", Kind: ParameterString, Required: true, Availability: []string{contextGameplay}},
			},
			ExecutionModes: []ExecutionMode{ExecutionWorkflow}, Destructive: false,
		},
	}
}
