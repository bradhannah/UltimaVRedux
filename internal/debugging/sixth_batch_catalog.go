package debugging

const (
	categoryDeath = "Death"
	// CommandDeathKillParty atomically wipes the reviewed active party without starting resurrection.
	CommandDeathKillParty = "death.kill-party"
	// CommandDeathTestSequence atomically wipes the reviewed active party and queues resurrection.
	CommandDeathTestSequence = "death.test-sequence"
)

func sixthBatchCommands() []CommandDefinition {
	fingerprint := func() []ParameterDefinition {
		return []ParameterDefinition{{
			ID: expectedFingerprintID, Name: "Reviewed Wipe Fingerprint", Kind: ParameterString, Required: true,
			Availability: []string{contextGameplay},
		}}
	}

	return []CommandDefinition{
		{
			ID: CommandDeathKillParty, Category: categoryDeath, Name: "Kill Active Party",
			Description: "Atomically set every member in the reviewed active party to zero HP and Dead " +
				"without starting resurrection.",
			Aliases:        []string{"death kill"},
			Parameters:     fingerprint(),
			ExecutionModes: []ExecutionMode{ExecutionSynchronized}, Destructive: true,
		},
		{
			ID: CommandDeathTestSequence, Category: categoryDeath, Name: "Test Death Sequence",
			Description: "Atomically wipe the reviewed active party and queue the production death and " +
				"resurrection workflow.",
			Aliases:        []string{"death test"},
			Parameters:     fingerprint(),
			ExecutionModes: []ExecutionMode{ExecutionWorkflow}, Destructive: true,
		},
	}
}
