package debugging

// CommandTestSetupStatus reports the context and state used by the camping and death test setups.
const CommandTestSetupStatus = "test-setup.status"

func testSetupCommands() []CommandDefinition {
	return []CommandDefinition{{
		ID: CommandTestSetupStatus, Category: categoryTestSetups, Name: "Test Setups",
		Description:    "Inspect the active party and run the camping and death test setups.",
		ExecutionModes: []ExecutionMode{ExecutionQuery},
	}}
}
