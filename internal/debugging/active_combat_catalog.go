package debugging

import "encoding/json"

const (
	activeCombatCategory           = "Active Combat"
	activeCombatEncounterName      = "Encounter"
	activeCombatMonsterName        = "Monster"
	activeCombatExpectedSnapshot   = "Expected Snapshot"
	activeCombatExpectedSnapshotID = "expectedSnapshot"
	activeCombatEncounterID        = "encounterId"
	activeCombatSeedID             = "seed"
	activeCombatIdentifierPattern  = `^[1-9][0-9]*$`
	activeCombatTextControlHint    = "text"
	// CommandCombatStatus reports a detached snapshot of the active tactical encounter.
	CommandCombatStatus = "combat.status"
	// CommandCombatMonsterSleepSet changes one live monster's sleep condition.
	CommandCombatMonsterSleepSet = "combat.monster.sleep.set"
	// CommandCombatMonsterSleepAll puts every live monster in one reviewed encounter to sleep.
	CommandCombatMonsterSleepAll = "combat.monster.sleep-all"
	// CommandCombatExitDebug discards one reviewed tactical encounter without resolving an outcome.
	CommandCombatExitDebug = "combat.exit-debug"
	// CommandCombatMonsterSpawn atomically adds one reviewed reinforcement batch.
	CommandCombatMonsterSpawn = "combat.monster.spawn"
	// CommandRNGSeedSet resets the global gameplay RNG from reviewed active combat.
	CommandRNGSeedSet = "rng.seed.set"
	// CommandCombatMonsterDamageRaw applies bounded damage under the debug defeat policy.
	CommandCombatMonsterDamageRaw = "combat.monster.damage-raw"
	// CommandCombatMonsterKill defeats one reviewed combatant under the debug defeat policy.
	CommandCombatMonsterKill = "combat.monster.kill"
	// CommandCombatMonsterKillAll defeats every monster in one reviewed encounter snapshot.
	CommandCombatMonsterKillAll = "combat.monster.kill-all"
	// CommandCombatDungeonRoomClear defeats hostile enemies in an authoritative fixed dungeon room.
	CommandCombatDungeonRoomClear = "combat.dungeon-room.clear"
	// CommandCombatMonsterClear removes every monster without resolving a combat outcome.
	CommandCombatMonsterClear = "combat.monster.clear"
)

func activeCombatCommands() []CommandDefinition {
	minimumCount, maximumCount := 1, 16
	minimumMonsterType, maximumMonsterType := 0, 47
	minimumSeed, maximumSeed := 0, 2147483647
	minimumLegacySeed, maximumLegacySeed := 1, 999999
	minimumDamage, maximumDamage := 1, 999
	encounterParameter := ParameterDefinition{ID: activeCombatEncounterID, Name: activeCombatEncounterName, Kind: ParameterString, Required: true, Pattern: activeCombatIdentifierPattern, ControlHint: activeCombatTextControlHint}
	snapshotParameter := ParameterDefinition{ID: activeCombatExpectedSnapshotID, Name: "Reviewed Status Fingerprint", Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint}
	monsterParameter := ParameterDefinition{ID: "monsterId", Name: activeCombatMonsterName, Kind: ParameterString, Required: true, Pattern: activeCombatIdentifierPattern, ControlHint: activeCombatTextControlHint}
	expectedHPParameter := ParameterDefinition{ID: "expectedCurrentHp", Name: "Expected Current HP", Kind: ParameterInteger, Required: true, Minimum: &minimumCount, Maximum: &maximumDamage, ControlHint: controlHintNumber}

	return []CommandDefinition{
		{
			ID:             CommandCombatStatus,
			Category:       activeCombatCategory,
			Name:           "Active Combat",
			Description:    "Inspect the tactical map, combatants, conditions, and DEX queue without advancing combat.",
			Aliases:        []string{"combat status"},
			Parameters:     nil,
			ExecutionModes: []ExecutionMode{ExecutionQuery},
			Destructive:    false,
		},
		{
			ID:          CommandCombatMonsterSleepSet,
			Category:    activeCombatCategory,
			Name:        "Set Monster Sleep",
			Description: "Set one live monster's sleep condition at a quiescent player-turn boundary.",
			Aliases:     []string{"combat sleep monster", "combat wake monster"},
			Parameters: []ParameterDefinition{
				{ID: activeCombatEncounterID, Name: activeCombatEncounterName, Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint},
				{ID: "monsterId", Name: activeCombatMonsterName, Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint},
				{ID: "asleep", Name: "Asleep", Kind: ParameterBoolean, Required: true, ControlHint: controlHintSelect},
				{ID: "expectedAsleep", Name: "Expected Current State", Kind: ParameterBoolean, Required: true, ControlHint: controlHintSelect},
				{ID: activeCombatExpectedSnapshotID, Name: activeCombatExpectedSnapshot, Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    false,
		},
		{
			ID:       CommandCombatMonsterSleepAll,
			Category: activeCombatCategory,
			Name:     "Put All Monsters to Sleep",
			Description: "Put every live monster in the exact reviewed snapshot to sleep at a quiescent " +
				"player-turn boundary; monsters that already sleep are unchanged.",
			Aliases:        []string{"combat sleep all"},
			Parameters:     []ParameterDefinition{encounterParameter, snapshotParameter},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    false,
		},
		{
			ID:       CommandCombatExitDebug,
			Category: activeCombatCategory,
			Name:     "Discard Active Encounter",
			Description: "Discard one reviewed tactical encounter and return to its pre-combat location " +
				"without resolving victory, defeat, evacuation, or overworld contact.",
			Aliases: []string{"combat exit"},
			Parameters: []ParameterDefinition{
				{ID: activeCombatEncounterID, Name: activeCombatEncounterName, Kind: ParameterString, Required: true, Pattern: activeCombatIdentifierPattern, ControlHint: activeCombatTextControlHint},
				{ID: activeCombatExpectedSnapshotID, Name: activeCombatExpectedSnapshot, Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    true,
		},
		{
			ID:          CommandCombatMonsterSpawn,
			Category:    activeCombatCategory,
			Name:        "Spawn Reinforcements",
			Description: "Atomically add a reviewed monster batch at unoccupied authoritative combat-map positions.",
			Aliases:     []string{"combat spawn"},
			Parameters: []ParameterDefinition{
				{ID: activeCombatEncounterID, Name: activeCombatEncounterName, Kind: ParameterString, Required: true, Pattern: activeCombatIdentifierPattern, ControlHint: activeCombatTextControlHint},
				{ID: activeCombatExpectedSnapshotID, Name: activeCombatExpectedSnapshot, Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint},
				{ID: "monsterType", Name: "Monster Type ID", Kind: ParameterInteger, Required: true, Minimum: &minimumMonsterType, Maximum: &maximumMonsterType, ControlHint: controlHintNumber},
				{ID: "countMode", Name: "Count Mode", Kind: ParameterEnum, Required: true, Choices: []Choice{{Value: "automatic", Label: "Automatic"}, {Value: "exact", Label: "Exact"}, {Value: "maximum", Label: "Maximum"}}, ControlHint: controlHintSelect},
				{ID: "exactCount", Name: "Exact Count", Kind: ParameterInteger, Minimum: &minimumCount, Maximum: &maximumCount, ControlHint: controlHintNumber},
				{ID: "friends", Name: "Include Friends", Kind: ParameterBoolean, Default: json.RawMessage(`true`), ControlHint: controlHintSelect},
				{ID: activeCombatSeedID, Name: "Seed", Kind: ParameterInteger, Minimum: &minimumSeed, Maximum: &maximumSeed, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    false,
		},
		{
			ID:       CommandRNGSeedSet,
			Category: activeCombatCategory,
			Name:     "Reset Gameplay RNG Seed",
			Description: "Reset the global gameplay RNG from a reviewed active encounter; all subsequent " +
				"gameplay randomness, including post-combat randomness, uses the new seed.",
			Aliases: []string{"combat seed"},
			Parameters: []ParameterDefinition{
				encounterParameter,
				snapshotParameter,
				{ID: activeCombatSeedID, Name: "Global Gameplay Seed", Kind: ParameterInteger, Required: true, Minimum: &minimumLegacySeed, Maximum: &maximumLegacySeed, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    true,
		},
		{
			ID:       CommandCombatMonsterDamageRaw,
			Category: activeCombatCategory,
			Name:     "Damage Monster",
			Description: "Apply bounded raw damage to one reviewed stable combatant; lethal damage " +
				"synchronizes the queue and victory but awards no XP and creates no corpse or treasure.",
			Aliases: []string{"combat damage"},
			Parameters: []ParameterDefinition{
				encounterParameter, monsterParameter, expectedHPParameter, snapshotParameter,
				{ID: "damage", Name: "Damage", Kind: ParameterInteger, Required: true, Minimum: &minimumDamage, Maximum: &maximumDamage, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    true,
		},
		{
			ID:       CommandCombatMonsterKill,
			Category: activeCombatCategory,
			Name:     "Kill Monster",
			Description: "Defeat one reviewed stable combatant, synchronize the queue, and trigger victory " +
				"when none remain; awards no XP and creates no corpse or treasure.",
			Aliases:        []string{"combat kill"},
			Parameters:     []ParameterDefinition{encounterParameter, monsterParameter, expectedHPParameter, snapshotParameter},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    true,
		},
		{
			ID:       CommandCombatMonsterKillAll,
			Category: activeCombatCategory,
			Name:     "Kill All Monsters",
			Description: "Defeat every monster in the exact reviewed snapshot, synchronize the queue, and " +
				"trigger victory; awards no XP and creates no corpses or treasure.",
			Aliases:        []string{"combat kill all"},
			Parameters:     []ParameterDefinition{encounterParameter, snapshotParameter},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    true,
		},
		{
			ID:             CommandCombatMonsterClear,
			Category:       activeCombatCategory,
			Name:           "Clear All Monsters",
			Description:    "Remove manager and queue monsters from the exact reviewed snapshot without XP, objects, or victory.",
			Aliases:        []string{"combat clear"},
			Parameters:     []ParameterDefinition{encounterParameter, snapshotParameter},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
			Destructive:    true,
		},
	}
}

func activeCombatDungeonRoomCommands() []CommandDefinition {
	return []CommandDefinition{{
		ID:       CommandCombatDungeonRoomClear,
		Category: activeCombatCategory,
		Name:     "Clear Fixed Dungeon Room",
		Description: "Defeat every hostile enemy in an authoritative fixed dungeon room and declare victory; " +
			"awards no XP and creates no corpses or treasure.",
		Aliases: []string{"combat clear room"},
		Parameters: []ParameterDefinition{
			{ID: activeCombatEncounterID, Name: activeCombatEncounterName, Kind: ParameterString, Required: true, Pattern: activeCombatIdentifierPattern, ControlHint: activeCombatTextControlHint},
			{ID: activeCombatExpectedSnapshotID, Name: "Reviewed Status Fingerprint", Kind: ParameterString, Required: true, ControlHint: activeCombatTextControlHint},
		},
		ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		Destructive:    true,
	}}
}
