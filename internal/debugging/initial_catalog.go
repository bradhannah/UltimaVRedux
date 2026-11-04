package debugging

import (
	"encoding/json"
	"strconv"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	controlHintSelect             = "select"
	controlHintNumber             = "number"
	categoryShrines               = "Shrines"
	categoryCalendar              = "Calendar"
	categoryCombat                = "Combat"
	categoryMonsters              = "Monsters"
	categoryVehicles              = "Vehicles"
	parameterIndex                = "index"
	parameterEnabled              = "enabled"
	categorySearch                = "Search"
	categoryQuestItems            = "Quest Items"
	contextGameplay               = "gameplay"
	contextOutsideCombat          = "outside_combat"
	initialCatalogSize            = 133
	reviewedRosterFingerprintName = "Reviewed Roster Fingerprint"
)

// OptionProviderActivePartyCharacters resolves current active-party character choices.
const OptionProviderActivePartyCharacters = "party.active-characters"

// OptionProviderActivePartyPositions resolves current occupied party positions.
const OptionProviderActivePartyPositions = "party.active-positions"

// OptionProviderEquipmentLoadoutTargets resolves all-party and current character loadout targets.
const OptionProviderEquipmentLoadoutTargets = "party.equipment-loadout-targets"

const (
	// CommandEnvironmentWindStatus reports the current wind direction.
	CommandEnvironmentWindStatus = "environment.wind.status"
	// CommandEnvironmentWindSet changes the current wind direction without consuming gameplay resources.
	CommandEnvironmentWindSet = "environment.wind.set"
	// CommandLoggingStatus reports current categorized logging configuration.
	CommandLoggingStatus = "logging.status"
	// CommandShrineStatus reports reference and progression state for every shrine.
	CommandShrineStatus = "shrine.status"
	// CommandShrineProgressUpdate atomically changes one shrine's progression flags.
	CommandShrineProgressUpdate = "shrine.progress.update"
	// CommandPartyCharacterInspect reports editable state for the active party.
	CommandPartyCharacterInspect = "party.character.inspect"
	// CommandPartyCharacterUpdate atomically alters one active character outside combat.
	CommandPartyCharacterUpdate = "party.character.update"
	// CommandPartyMaximizeLevels raises every active character to level eight.
	CommandPartyMaximizeLevels = "party.maximize-levels"
	// CommandPartyStatusSet applies one explicit status to reviewed active roster indices.
	CommandPartyStatusSet = "party.status.set"
	// CommandPartyFill fills open active-party positions in roster order.
	CommandPartyFill = "party.fill"
	// CommandPartyMemberAdd adds one reviewed inactive roster member.
	CommandPartyMemberAdd = "party.member.add"
	// CommandPartyKarmaSet sets the party-wide karma value exactly.
	CommandPartyKarmaSet = "party.karma.set"
	// CommandCalendarStatus reports the current Britannian calendar.
	CommandCalendarStatus = "calendar.status"
	// CommandCalendarSetTime sets the current hour and minute without advancing time.
	CommandCalendarSetTime = "calendar.set-time"
	// CommandCalendarSetDay sets the day within the current month.
	CommandCalendarSetDay = "calendar.set-day"
	// CommandSearchStatus reports search state around the Avatar.
	CommandSearchStatus = "search.status"
	// CommandSearchItemStackAdd adds a supported provision to a current-map item stack.
	CommandSearchItemStackAdd = "search.item-stack.add"
	// CommandSearchProgressReset clears one-time authentic search progress.
	CommandSearchProgressReset = "search.progress.reset"
	// CommandQuestItemStatus reports all quest-item quantities and known placements.
	CommandQuestItemStatus = "quest.item.status"
	// CommandQuestItemEnsure ensures one quest item is in inventory.
	CommandQuestItemEnsure = "quest.item.ensure"
	// CommandQuestItemRemove removes one quest item from inventory.
	CommandQuestItemRemove = "quest.item.remove"
	// CommandWorldPositionStatus reports the current same-map coordinate space.
	CommandWorldPositionStatus = "world.position.status"
	// CommandMovementLandmarkTravel transitions to a curated physical landmark.
	CommandMovementLandmarkTravel = "movement.landmark.travel"
	// CommandMoongateStatus reports moon phase, moonstones, and computed destination.
	CommandMoongateStatus = "moongate.status"
	// CommandCombatEncounterOptions reports valid encounter composer options.
	CommandCombatEncounterOptions = "combat.encounter.options"
	// CommandCombatEncounterStart starts a composed tactical encounter.
	CommandCombatEncounterStart = "combat.encounter.start"
	// CommandMonsterEcosystemStatus reports current-map monster generation and units.
	CommandMonsterEcosystemStatus = "monster.ecosystem.status"
	// CommandMonsterGenerationSet changes natural monster generation settings.
	CommandMonsterGenerationSet = "monster.generation.set"
	// CommandMonsterClearMapEnemies removes current-map hostile units.
	CommandMonsterClearMapEnemies = "monster.clear-map-enemies"
	// CommandMonsterSpawnWhirlpool attempts an authoritative whirlpool spawn.
	CommandMonsterSpawnWhirlpool = "monster.spawn-whirlpool"
	// CommandMonsterSpawnPirate attempts an authoritative pirate spawn.
	CommandMonsterSpawnPirate = "monster.spawn-pirate"
	// CommandVehicleStatus reports the currently boarded party vehicle.
	CommandVehicleStatus = "vehicle.status"
	// CommandVehicleSpawn inserts a frigate or skiff at the Avatar or a stable overworld dock.
	CommandVehicleSpawn = "vehicle.spawn"
	// CommandVehicleForceBoard replaces the party vehicle while intentionally bypassing terrain checks.
	CommandVehicleForceBoard = "vehicle.force-board"
	// CommandShipDamageRaw applies non-sinking raw hull damage.
	CommandShipDamageRaw = "ship.damage-raw"
	// CommandShipRepairFull restores frigate hull strength.
	CommandShipRepairFull = "ship.repair-full"
	// CommandShipSkiffSet sets the exact number of skiffs attached to the current frigate.
	CommandShipSkiffSet = "ship.skiff.set"

	// WindDirectionCalm is the stable serialized calm-wind identity.
	WindDirectionCalm = "calm"
	// WindDirectionNorth is the stable serialized north-wind identity.
	WindDirectionNorth = "north"
	// WindDirectionSouth is the stable serialized south-wind identity.
	WindDirectionSouth = "south"
	// WindDirectionEast is the stable serialized east-wind identity.
	WindDirectionEast = "east"
	// WindDirectionWest is the stable serialized west-wind identity.
	WindDirectionWest = "west"

	// ShrineQuestNotAssigned normalizes cleared completed and active flags.
	ShrineQuestNotAssigned = "not_assigned"
	// ShrineQuestAssigned normalizes a cleared completed flag and set active flag.
	ShrineQuestAssigned = "assigned"
	// ShrineQuestReadyToComplete normalizes set completed and active flags.
	ShrineQuestReadyToComplete = "ready_to_complete"
	// ShrineQuestOffering normalizes a set completed flag and cleared active flag.
	ShrineQuestOffering = "offering"
)

const (
	initialExperienceParameter = "experience"
	initialMovementCategory    = "Movement"
)

// NewInitialCatalog creates the first transport-neutral command slice.
//
//nolint:maintidx // Declarative catalog keeps parameter constraints beside stable commands in fixture order.
func NewInitialCatalog() *Catalog {
	minimumStat := 1
	maximumStat := 30
	minimumHP := 1
	maximumHP := 240
	minimumResource := 0
	maximumExperience := 9999
	minimumLevel := 1
	maximumLevel := 8
	minimumRosterIndex := 0
	maximumRosterIndex := 15
	definitions := make([]CommandDefinition, 0, initialCatalogSize)
	definitions = append(definitions, []CommandDefinition{
		{
			ID:             CommandEnvironmentWindStatus,
			Category:       "Environment",
			Name:           "Wind Status",
			Description:    "Show the current wind direction.",
			Aliases:        []string{"env wind", "wind"},
			Parameters:     nil,
			ExecutionModes: []ExecutionMode{ExecutionQuery},
			Destructive:    false,
		},
		{
			ID:          CommandEnvironmentWindSet,
			Category:    "Environment",
			Name:        "Set Wind",
			Description: "Set the current wind direction without consuming a scroll or advancing time.",
			Aliases:     []string{"env wind set", "set wind"},
			Parameters: []ParameterDefinition{
				{
					ID:          "direction",
					Name:        "Direction",
					Description: "Choose the new live wind direction.",
					Kind:        ParameterEnum,
					Required:    true,
					Default:     nil,
					Minimum:     nil,
					Maximum:     nil,
					Pattern:     "",
					Choices: []Choice{
						{Value: WindDirectionCalm, Label: "Calm", Metadata: nil},
						{Value: WindDirectionNorth, Label: "North", Metadata: nil},
						{Value: WindDirectionEast, Label: "East", Metadata: nil},
						{Value: WindDirectionSouth, Label: "South", Metadata: nil},
						{Value: WindDirectionWest, Label: "West", Metadata: nil},
					},
					OptionsProvider: "",
					ControlHint:     controlHintSelect,
					Availability:    []string{contextGameplay},
				},
			},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
			Destructive:    false,
		},
		{
			ID:             CommandLoggingStatus,
			Category:       commandCategoryDiagnostics,
			Name:           "Logging Status",
			Description:    "Show global logging, level, output, and category state.",
			Aliases:        []string{"debug status", "logging status"},
			Parameters:     nil,
			ExecutionModes: []ExecutionMode{ExecutionQuery},
			Destructive:    false,
		},
		{
			ID:             CommandShrineStatus,
			Category:       categoryShrines,
			Name:           "Shrine Atlas",
			Description:    "Inspect immutable shrine reference data alongside current progression state.",
			Aliases:        []string{"shrine status", "shrine atlas"},
			Parameters:     nil,
			ExecutionModes: []ExecutionMode{ExecutionQuery},
			Destructive:    false,
		},
		{
			ID:          CommandShrineProgressUpdate,
			Category:    categoryShrines,
			Name:        "Shrine Progress Alteration",
			Description: "Atomically update actual shrine quest and ruined-state variables without awarding rewards.",
			Aliases:     []string{"shrine quest", "shrine ruin", "shrine restore"},
			Parameters: []ParameterDefinition{
				{
					ID:              parameterIndex,
					Name:            "Shrine",
					Description:     "Choose the shrine whose live progression state will change.",
					Kind:            ParameterEnum,
					Required:        true,
					Default:         nil,
					Minimum:         nil,
					Maximum:         nil,
					Pattern:         "",
					Choices:         shrineChoices(),
					OptionsProvider: "",
					ControlHint:     controlHintSelect,
					Availability:    []string{contextGameplay},
				},
				{
					ID:          "questState",
					Name:        "Quest State",
					Description: "Set the normalized completed/active flag combination without replaying rewards.",
					Kind:        ParameterEnum,
					Required:    false,
					Default:     nil,
					Minimum:     nil,
					Maximum:     nil,
					Pattern:     "",
					Choices: []Choice{
						{Value: ShrineQuestNotAssigned, Label: "Not Assigned", Metadata: nil},
						{Value: ShrineQuestAssigned, Label: "Assigned / Seeking Codex", Metadata: nil},
						{Value: ShrineQuestReadyToComplete, Label: "Ready to Complete", Metadata: nil},
						{Value: ShrineQuestOffering, Label: "Completed / Offering", Metadata: nil},
					},
					OptionsProvider: "",
					ControlHint:     controlHintSelect,
					Availability:    nil,
				},
				{
					ID:              "ruined",
					Name:            "Shrine Condition",
					Description:     "Set whether Blackthorn has ruined the shrine.",
					Kind:            ParameterBoolean,
					Required:        false,
					Default:         nil,
					Minimum:         nil,
					Maximum:         nil,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintSelect,
					Availability:    nil,
				},
			},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
			Destructive:    false,
		},
		{
			ID:             CommandPartyCharacterInspect,
			Category:       partyEquipmentCategory,
			Name:           "Party Character Roster",
			Description:    "Inspect the detached full roster, stable numeric indices, and review fingerprint.",
			Aliases:        []string{"pc inspect", "party status"},
			Parameters:     nil,
			ExecutionModes: []ExecutionMode{ExecutionQuery},
			Destructive:    false,
		},
		{
			ID:          CommandPartyCharacterUpdate,
			Category:    partyEquipmentCategory,
			Name:        "Character Alteration",
			Description: "Atomically update safe character attributes, resources, status, or party position. Combat permits current HP and MP only.",
			Aliases:     []string{"pc stats", "pc status", "new order"},
			Parameters: []ParameterDefinition{
				{
					ID:              "character",
					Name:            "Character",
					Description:     "Select a current active-party member.",
					Kind:            ParameterCharacter,
					Required:        true,
					Default:         nil,
					Minimum:         nil,
					Maximum:         nil,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: OptionProviderActivePartyCharacters,
					ControlHint:     controlHintSelect,
					Availability:    []string{contextGameplay},
				},
				{
					ID:              "strength",
					Name:            "Strength",
					Description:     "Leave unset to preserve the current value.",
					Kind:            ParameterInteger,
					Required:        false,
					Default:         nil,
					Minimum:         &minimumStat,
					Maximum:         &maximumStat,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintNumber,
					Availability:    nil,
				},
				{
					ID:              "dexterity",
					Name:            "Dexterity",
					Description:     "Leave unset to preserve the current value.",
					Kind:            ParameterInteger,
					Required:        false,
					Default:         nil,
					Minimum:         &minimumStat,
					Maximum:         &maximumStat,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintNumber,
					Availability:    nil,
				},
				{
					ID:              "intelligence",
					Name:            "Intelligence",
					Description:     "Lower values clamp current MP to the new maximum.",
					Kind:            ParameterInteger,
					Required:        false,
					Default:         nil,
					Minimum:         &minimumStat,
					Maximum:         &maximumStat,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintNumber,
					Availability:    nil,
				},
				{
					ID:              "currentHp",
					Name:            "Current HP",
					Description:     "Exact living HP; zero and death transitions are intentionally excluded.",
					Kind:            ParameterInteger,
					Required:        false,
					Default:         nil,
					Minimum:         &minimumHP,
					Maximum:         &maximumHP,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintNumber,
					Availability:    nil,
				},
				{
					ID:              "currentMp",
					Name:            "Current MP",
					Description:     "Exact MP, validated against the resulting Intelligence value.",
					Kind:            ParameterInteger,
					Required:        false,
					Default:         nil,
					Minimum:         &minimumResource,
					Maximum:         &maximumStat,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintNumber,
					Availability:    nil,
				},
				{
					ID:              initialExperienceParameter,
					Name:            "Experience",
					Description:     "Exact XP without forcing level or maximum-HP progression.",
					Kind:            ParameterInteger,
					Required:        false,
					Default:         nil,
					Minimum:         &minimumResource,
					Maximum:         &maximumExperience,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: "",
					ControlHint:     controlHintNumber,
					Availability:    nil,
				},
				{
					ID:          "level",
					Name:        "Level",
					Description: "Set level, minimum XP, and level-derived maximum HP without healing or stat boosts.",
					Kind:        ParameterInteger,
					Required:    false,
					Minimum:     &minimumLevel,
					Maximum:     &maximumLevel,
					ControlHint: controlHintSelect,
				},
				{
					ID:          "status",
					Name:        "Status",
					Description: "Safe non-dead status; leave unset to preserve the current status.",
					Kind:        ParameterEnum,
					Required:    false,
					Default:     nil,
					Minimum:     nil,
					Maximum:     nil,
					Pattern:     "",
					Choices: []Choice{
						{Value: "good", Label: "Good", Metadata: nil},
						{Value: "poisoned", Label: "Poisoned", Metadata: nil},
						{Value: "sleep", Label: "Sleep", Metadata: nil},
						{Value: "charmed", Label: "Charmed", Metadata: nil},
					},
					OptionsProvider: "",
					ControlHint:     controlHintSelect,
					Availability:    nil,
				},
				{
					ID:              "partyPosition",
					Name:            "Party Position",
					Description:     "Swap with an occupied position; the Avatar must remain first.",
					Kind:            ParameterEnum,
					Required:        false,
					Default:         nil,
					Minimum:         nil,
					Maximum:         nil,
					Pattern:         "",
					Choices:         nil,
					OptionsProvider: OptionProviderActivePartyPositions,
					ControlHint:     controlHintSelect,
					Availability:    nil,
				},
			},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
			Destructive:    false,
		},
		{
			ID: CommandPartyMaximizeLevels, Category: partyEquipmentCategory, Name: "Maximize Active Levels",
			Description:    "Raise every reviewed active roster member to level eight and restore progression-derived resources.",
			Aliases:        []string{"pc stats maxlevels"},
			Parameters:     []ParameterDefinition{{ID: expectedFingerprintID, Name: reviewedRosterFingerprintName, Kind: ParameterString, Required: true, Availability: []string{contextGameplay, contextOutsideCombat}}},
			ExecutionModes: []ExecutionMode{ExecutionAlteration}, Destructive: true,
		},
		{
			ID: CommandPartyStatusSet, Category: partyEquipmentCategory, Name: "Set Party Status",
			Description: "Atomically set an explicit status for one or a reviewed list of stable active roster indices.",
			Aliases:     []string{"pc status"},
			Parameters: []ParameterDefinition{
				{ID: expectedFingerprintID, Name: reviewedRosterFingerprintName, Kind: ParameterString, Required: true, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: "rosterIndices", Name: "Active Roster Indices", Kind: ParameterIntegerList, Required: true, Minimum: &minimumRosterIndex, Maximum: &maximumRosterIndex},
				{ID: "status", Name: "Status", Kind: ParameterEnum, Required: true, Choices: []Choice{{Value: "good", Label: "Good"}, {Value: "poisoned", Label: "Poisoned"}, {Value: "sleep", Label: "Sleep"}, {Value: "charmed", Label: "Charmed"}, {Value: "dead", Label: "Dead"}}, ControlHint: controlHintSelect},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized}, Destructive: true,
		},
		{
			ID: CommandPartyFill, Category: partyEquipmentCategory, Name: "Fill Active Party",
			Description:    "Fill every open active-party position from the reviewed roster using authoritative composition rules.",
			Aliases:        []string{"pc party fill"},
			Parameters:     []ParameterDefinition{{ID: expectedFingerprintID, Name: reviewedRosterFingerprintName, Kind: ParameterString, Required: true, Availability: []string{contextGameplay, contextOutsideCombat}}},
			ExecutionModes: []ExecutionMode{ExecutionAlteration}, Destructive: true,
		},
		{
			ID: CommandPartyMemberAdd, Category: partyEquipmentCategory, Name: "Add Roster Member",
			Description: "Add one reviewed inactive member by stable numeric roster index.",
			Aliases:     []string{"pc party add"},
			Parameters: []ParameterDefinition{
				{ID: expectedFingerprintID, Name: reviewedRosterFingerprintName, Kind: ParameterString, Required: true, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: "rosterIndex", Name: "Roster Index", Kind: ParameterInteger, Required: true, Minimum: &minimumRosterIndex, Maximum: &maximumRosterIndex, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionAlteration}, Destructive: true,
		},
	}...)
	definitions = append(definitions, approvedToolCommands()...)
	definitions = append(definitions, secondBatchToolCommands()...)
	definitions = append(definitions, inventoryLockerCommands()...)
	definitions = append(definitions, spellDebugCommands()...)
	definitions = append(definitions, activeCombatCommands()...)
	definitions = append(definitions, loggingCommands()...)
	definitions = append(definitions, presentationCommands()...)
	definitions = append(definitions, operationalCommands()...)
	definitions = append(definitions, partyKarmaCommands()...)
	definitions = append(definitions, testSetupCommands()...)
	definitions = append(definitions, thirdBatchCommands()...)
	definitions = append(definitions, fourthBatchCommands()...)
	definitions = append(definitions, fifthBatchCommands()...)
	definitions = append(definitions, sixthBatchCommands()...)
	definitions = append(definitions, seventhBatchCommands()...)
	definitions = append(definitions, saveSnapshotCommands()...)
	definitions = append(definitions, partyEquipmentCommands()...)
	definitions = append(definitions, worldNavigatorCommands()...)
	definitions = append(definitions, activeCombatDungeonRoomCommands()...)
	definitions = append(definitions, merchantTesterCommands()...)
	applyCommandSearchTerms(definitions)
	catalog, err := NewCatalog(definitions)
	if err != nil {
		panic("invalid initial debug command catalog: " + err.Error())
	}

	return catalog
}

//nolint:goconst // Search vocabulary intentionally repeats player-facing terms across instruments.
func applyCommandSearchTerms(definitions []CommandDefinition) {
	terms := map[string][]string{
		CommandWorldPositionStatus:       {"teleport", "travel", "map", "location", "coordinates", "floor", "navigator"},
		CommandWorldPositionSet:          {"teleport", "travel", "map", "location", "coordinates", "floor", "navigator"},
		CommandInventoryLockerStatus:     {"items", "supplies", "food", "gold", "reagents", "potions", "scrolls", "equipment", "armory"},
		CommandInventorySpellStatus:      {"spells", "magic", "prepared", "light", "time stop", "duration", "incantations"},
		CommandGuardsStatus:              {"guards", "criminal", "wanted", "shadowlords", "death", "resurrection", "world diagnostics"},
		CommandDebugOptionsStatus:        {"free move", "collision", "placeholders", "dialog bounds", "lighting overlay", "debug overlays", "unlimited casting", "spell level bypass"},
		CommandLoggingStatus:             {"logs", "console", "categories", "stdout", "verbosity", "logging"},
		CommandMonsterEcosystemStatus:    {"spawn", "enemies", "generation", "pirate", "whirlpool", "monsters"},
		CommandCombatStatus:              {"battle", "tactical", "monsters", "turn", "dex", "encounter", "combat"},
		CommandSearchStatus:              {"find", "nearby", "hidden", "secret", "item stacks", "search overlay"},
		CommandPartyCharacterInspect:     {"party", "characters", "stats", "hp", "mp", initialExperienceParameter, "roster", "karma"},
		CommandPartyCharacterUpdate:      {"party", "characters", "stats", "hp", "mp", initialExperienceParameter, "roster", "karma"},
		CommandQuestItemStatus:           {"artifacts", "shards", "quest inventory", "special items"},
		CommandVehicleStatus:             {"ships", "frigate", "skiff", "hull", "boat", "vehicle"},
		CommandTestSetupStatus:           {"test setup", "reproduce", "camping", "death"},
		CommandMerchantTesterCatalog:     {"merchant", "shop", "guild", "preview", "live", "tester"},
		CommandMerchantTesterPreviewOpen: {"merchant", "shop", "guild", "preview", "tester"},
		CommandMerchantTesterLiveOpen:    {"merchant", "shop", "guild", "live", "tester"},
	}
	for index := range definitions {
		definitions[index].SearchTerms = append([]string(nil), terms[definitions[index].ID]...)
	}
}

func partyKarmaCommands() []CommandDefinition {
	minimum, maximum := 0, 99

	return []CommandDefinition{{
		ID: CommandPartyKarmaSet, Category: partyEquipmentCategory, Name: "Set Party Karma",
		Description: "Set party-wide karma exactly without applying a moral event or advancing time.", Aliases: []string{"pc stats karma"},
		Parameters: []ParameterDefinition{{
			ID: "value", Name: "Karma", Kind: ParameterInteger, Required: true,
			Minimum: &minimum, Maximum: &maximum, ControlHint: controlHintNumber, Availability: []string{contextGameplay},
		}},
		ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
	}}
}

func secondBatchToolCommands() []CommandDefinition {
	minimumCoordinate, maximumCoordinate := 0, 255
	minimumMap, maximumMap := 0, references.TotalCombatMaps-1
	minimumDungeon, maximumDungeon := 0, references.MaxDungeonCombatIndex
	minimumRoom, maximumRoom := 0, references.MaxDungeonRooms-1
	minimumCount, maximumCount := 1, references.MaxMonstersInCombat
	minimumSeed := 0
	minimumOdds, maximumOdds := 1, 1000
	minimumDamage, maximumDamage := 1, 98
	minimumSkiffs, maximumSkiffs := 0, references.MaxFrigateSkiffs
	minimumFloor, maximumFloor := -1, 5
	minimumEnemyCount, maximumEnemyCount := 0, 32

	return []CommandDefinition{
		{
			ID: CommandCombatEncounterOptions, Category: categoryCombat, Name: "Encounter Options",
			Description:    "List maps, entry directions, count modes, and spawnable monsters.",
			ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
		{
			ID: CommandCombatEncounterStart, Category: categoryCombat, Name: "Start Encounter",
			Description: "Validate, compose, and synchronously start a tactical encounter, or load a fixed dungeon room as authored.",
			Parameters: []ParameterDefinition{
				{ID: "territory", Name: "Territory", Kind: ParameterEnum, Required: true, Choices: []Choice{{Value: "britannia", Label: "Britannia"}, {Value: "dungeon", Label: "Dungeon"}}, ControlHint: controlHintSelect, Availability: []string{contextGameplay, contextOutsideCombat}},
				{ID: "britanniaMap", Name: "Britannia Map", Kind: ParameterInteger, Minimum: &minimumMap, Maximum: &maximumMap, ControlHint: controlHintNumber},
				{ID: "dungeon", Name: "Dungeon", Kind: ParameterInteger, Minimum: &minimumDungeon, Maximum: &maximumDungeon, ControlHint: controlHintNumber},
				{ID: "dungeonRoom", Name: "Dungeon Room", Kind: ParameterInteger, Minimum: &minimumRoom, Maximum: &maximumRoom, ControlHint: controlHintNumber},
				{ID: "entryDirection", Name: "Entry Direction", Kind: ParameterEnum, Required: true, Choices: directionChoices(), ControlHint: controlHintSelect},
				{ID: "contents", Name: "Contents", Kind: ParameterEnum, Default: json.RawMessage(`"custom"`), Choices: []Choice{{Value: "custom", Label: "Custom monsters"}, {Value: "authored", Label: "Authored room"}}, ControlHint: controlHintSelect},
				{ID: "monsterType", Name: "Monster", Kind: ParameterEnum, Choices: monsterChoices(), ControlHint: controlHintSelect},
				{ID: "countMode", Name: "Count Mode", Kind: ParameterEnum, Choices: []Choice{{Value: "automatic", Label: "Automatic"}, {Value: "exact", Label: "Exact"}, {Value: "maximum", Label: "Maximum"}}, ControlHint: controlHintSelect},
				{ID: "exactCount", Name: "Exact Count", Kind: ParameterInteger, Minimum: &minimumCount, Maximum: &maximumCount, ControlHint: controlHintNumber},
				{ID: "friends", Name: "Include Friends", Kind: ParameterBoolean, Default: json.RawMessage(`true`), ControlHint: controlHintSelect},
				{ID: "seed", Name: "Seed", Kind: ParameterInteger, Minimum: &minimumSeed, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
		{ID: CommandMonsterEcosystemStatus, Category: categoryMonsters, Name: "Monster Ecosystem", Description: "Show current-map generation and enemy state.", ExecutionModes: []ExecutionMode{ExecutionQuery}},
		{
			ID: CommandMonsterGenerationSet, Category: categoryMonsters, Name: "Set Monster Generation", Description: "Set natural generation enabled state and inclusive roll maximum; 30 matches the original.",
			Parameters:     []ParameterDefinition{{ID: parameterEnabled, Name: "Generation Enabled", Kind: ParameterBoolean, Required: true, Availability: []string{contextGameplay}}, {ID: "odds", Name: "Roll Maximum", Kind: ParameterInteger, Required: true, Minimum: &minimumOdds, Maximum: &maximumOdds, ControlHint: controlHintNumber}},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
		},
		{
			ID: CommandMonsterClearMapEnemies, Category: categoryMonsters, Name: "Clear Map Enemies", Description: "Remove hostile current-map units while preserving friendlies and vehicles.",
			Parameters: []ParameterDefinition{
				{ID: "expectedLocationId", Name: "Expected Location", Kind: ParameterInteger, Required: true, Minimum: &minimumCoordinate, Maximum: &maximumCoordinate, ControlHint: controlHintNumber},
				{ID: "expectedFloor", Name: "Expected Floor", Kind: ParameterInteger, Required: true, Minimum: &minimumFloor, Maximum: &maximumFloor, ControlHint: controlHintNumber},
				{ID: "expectedEnemyCount", Name: "Expected Enemy Count", Kind: ParameterInteger, Required: true, Minimum: &minimumEnemyCount, Maximum: &maximumEnemyCount, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionAlteration}, Destructive: true,
		},
		{ID: CommandMonsterSpawnWhirlpool, Category: categoryMonsters, Name: "Spawn Whirlpool", Description: "Attempt a forced valid whirlpool spawn.", ExecutionModes: []ExecutionMode{ExecutionLiveSafe}},
		{ID: CommandMonsterSpawnPirate, Category: categoryMonsters, Name: "Spawn Pirate", Description: "Attempt a forced valid pirate spawn.", ExecutionModes: []ExecutionMode{ExecutionLiveSafe}},
		{ID: CommandVehicleStatus, Category: categoryVehicles, Name: "Ships", Description: "Inspect the boarded party vehicle and exceptional frigate test state.", ExecutionModes: []ExecutionMode{ExecutionQuery}},
		{
			ID: CommandVehicleSpawn, Category: categoryVehicles, Name: "Spawn Vehicle", Description: "Spawn a frigate or skiff at the Avatar or an authoritative overworld dock without advancing time.",
			Parameters: []ParameterDefinition{
				{ID: "type", Name: "Vehicle Type", Kind: ParameterEnum, Required: true, Choices: spawnVehicleChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay}},
				{ID: "placement", Name: "Placement", Kind: ParameterEnum, Required: true, Default: json.RawMessage(`"avatar"`), Choices: vehiclePlacementChoices(), ControlHint: controlHintSelect},
				{ID: "dock", Name: "Dock", Description: "Required only when placement is Dock.", Kind: ParameterEnum, Required: false, Choices: vehicleDockChoices(), ControlHint: controlHintSelect},
			},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
		{
			ID: CommandVehicleForceBoard, Category: categoryVehicles, Name: "Force Board Vehicle", Description: "Replace the party vehicle in a supported world context. Intentionally bypasses terrain restrictions.",
			Parameters:     []ParameterDefinition{{ID: "type", Name: "Vehicle Type", Kind: ParameterEnum, Required: true, Choices: boardableVehicleChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay}}},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
		{
			ID: CommandShipDamageRaw, Category: categoryVehicles, Name: "Damage Ship Hull", Description: "Apply non-sinking raw hull damage.",
			Parameters:     []ParameterDefinition{{ID: inventoryAmountParameterID, Name: "Damage", Kind: ParameterInteger, Required: true, Minimum: &minimumDamage, Maximum: &maximumDamage, ControlHint: controlHintNumber, Availability: []string{contextGameplay}}},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
		{ID: CommandShipRepairFull, Category: categoryVehicles, Name: "Repair Ship Hull", Description: "Restore the current frigate to full hull strength.", ExecutionModes: []ExecutionMode{ExecutionSynchronized}},
		{
			ID: CommandShipSkiffSet, Category: categoryVehicles, Name: "Set Frigate Skiffs", Description: "Set the exact Redux attached-skiff quantity.", Aliases: []string{"ship add skiff"},
			Parameters:     []ParameterDefinition{{ID: "quantity", Name: "Skiffs", Kind: ParameterInteger, Required: true, Minimum: &minimumSkiffs, Maximum: &maximumSkiffs, ControlHint: controlHintNumber, Availability: []string{contextGameplay}}},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
	}
}

func directionChoices() []Choice {
	return []Choice{
		{Value: "west", Label: "West", Metadata: nil}, {Value: "east", Label: "East", Metadata: nil},
		{Value: "north", Label: "North", Metadata: nil}, {Value: "south", Label: "South", Metadata: nil},
	}
}

func spawnVehicleChoices() []Choice {
	return []Choice{
		{Value: "frigate", Label: "Frigate", Metadata: nil},
		{Value: "skiff", Label: "Skiff", Metadata: nil},
	}
}

func boardableVehicleChoices() []Choice {
	return []Choice{
		{Value: "horse", Label: "Horse", Metadata: nil},
		{Value: "carpet", Label: "Magic Carpet", Metadata: nil},
		{Value: "skiff", Label: "Skiff", Metadata: nil},
		{Value: "frigate", Label: "Frigate", Metadata: nil},
	}
}

func vehiclePlacementChoices() []Choice {
	return []Choice{
		{Value: "avatar", Label: "At Avatar", Metadata: nil},
		{Value: "dock", Label: "At Dock", Metadata: nil},
	}
}

func vehicleDockChoices() []Choice {
	return []Choice{
		{Value: "jhelom", Label: "Jhelom", Metadata: nil},
		{Value: "minoc", Label: "Minoc", Metadata: nil},
		{Value: "east_britanny", Label: "East Britanny", Metadata: nil},
		{Value: "buccaneers_den", Label: "Buccaneer's Den", Metadata: nil},
	}
}

func monsterChoices() []Choice {
	choices := make([]Choice, 0, references.TotalMonsterTypes)
	maximumMonsterType := references.MonsterType(references.TotalMonsterTypes)
	for monsterType := references.MonsterType(0); monsterType < maximumMonsterType; monsterType++ {
		choices = append(choices, Choice{Value: strconv.Itoa(monsterType.Index()), Label: monsterType.Name(), Metadata: nil})
	}

	return choices
}

func approvedToolCommands() []CommandDefinition {
	minimumCoordinate := 0
	maximumCoordinate := 255
	minimumHour := 0
	maximumHour := 23
	minimumMinute := 0
	maximumMinute := 59
	minimumDay := 1
	maximumDay := 28
	minimumQuantity := 1
	maximumQuantity := 999

	coordinateParameter := func(id, name string) ParameterDefinition {
		return ParameterDefinition{
			ID: id, Name: name, Description: "Coordinate on the active map.", Kind: ParameterInteger,
			Required: true, Minimum: &minimumCoordinate, Maximum: &maximumCoordinate, ControlHint: controlHintNumber,
		}
	}
	questItemParameter := func() ParameterDefinition {
		return ParameterDefinition{
			ID: "item", Name: "Quest Item", Description: "Choose one stable quest-item identity.", Kind: ParameterEnum,
			Required: true, Choices: questItemChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay},
		}
	}

	return []CommandDefinition{
		{
			ID: CommandCalendarStatus, Category: categoryCalendar, Name: "Calendar Status",
			Description:    "Show the current Britannian date, time, turn, and moon phase.",
			ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
		{
			ID: CommandCalendarSetTime, Category: categoryCalendar, Name: "Set Calendar Time",
			Description: "Set the exact hour and minute without advancing game time.",
			Parameters: []ParameterDefinition{
				{ID: "hour", Name: "Hour", Kind: ParameterInteger, Required: true, Minimum: &minimumHour, Maximum: &maximumHour, ControlHint: controlHintNumber, Availability: []string{contextGameplay}},
				{ID: "minute", Name: "Minute", Kind: ParameterInteger, Required: true, Minimum: &minimumMinute, Maximum: &maximumMinute, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
		},
		{
			ID: CommandCalendarSetDay, Category: categoryCalendar, Name: "Set Calendar Day",
			Description:    "Set the day of the current month without advancing game time.",
			Parameters:     []ParameterDefinition{{ID: "day", Name: "Day", Kind: ParameterInteger, Required: true, Minimum: &minimumDay, Maximum: &maximumDay, ControlHint: controlHintNumber, Availability: []string{contextGameplay}}},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
		},
		{
			ID: CommandSearchStatus, Category: categorySearch, Name: "Nearby Search Status",
			Description:    "Inspect the Avatar's current and four adjacent cells, authentic search progress, and detached item stacks.",
			ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
		{
			ID: CommandSearchItemStackAdd, Category: categorySearch, Name: "Add Search Item Stack",
			Description: "Add or merge a supported provision at a logical coordinate on the current map.",
			Parameters: []ParameterDefinition{
				coordinateParameter("x", "X"), coordinateParameter("y", "Y"),
				{ID: "item", Name: "Provision", Kind: ParameterEnum, Required: true, Choices: provisionChoices(), ControlHint: controlHintSelect},
				{ID: "quantity", Name: "Quantity", Kind: ParameterInteger, Required: true, Minimum: &minimumQuantity, Maximum: &maximumQuantity, ControlHint: controlHintNumber},
			},
			ExecutionModes: []ExecutionMode{ExecutionLiveSafe},
		},
		{
			ID: CommandSearchProgressReset, Category: categorySearch, Name: "Reset One-Time Search Progress",
			Description: "Destructively clear only authentic one-time search-location found flags.",
			Parameters: []ParameterDefinition{{
				ID: "scope", Name: "Scope", Kind: ParameterEnum, Required: true,
				Default: json.RawMessage(`"one_time_locations"`), Choices: []Choice{{Value: "one_time_locations", Label: "One-Time Locations"}}, ControlHint: controlHintSelect,
			}},
			ExecutionModes: []ExecutionMode{ExecutionAlteration}, Destructive: true,
		},
		{
			ID: CommandQuestItemStatus, Category: categoryQuestItems, Name: "Quest Item Status",
			Description:    "Show all authentic quest items, quantities, categories, and known canonical placements.",
			ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
		{
			ID: CommandQuestItemEnsure, Category: categoryQuestItems, Name: "Grant Quest Item",
			Description: "Add one quest item only when it is absent.", Parameters: []ParameterDefinition{questItemParameter()},
			ExecutionModes: []ExecutionMode{ExecutionAlteration},
		},
		{
			ID: CommandQuestItemRemove, Category: categoryQuestItems, Name: "Remove Quest Item",
			Description: "Destructively remove the complete quantity of one quest item from inventory.", Parameters: []ParameterDefinition{questItemParameter()},
			ExecutionModes: []ExecutionMode{ExecutionAlteration}, Destructive: true,
		},
		{
			ID: CommandWorldPositionStatus, Category: "World", Name: "Position Status",
			Description:    "Show current location, map type, coordinates, logical maxima, and available floor range.",
			ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
		{
			ID: CommandMovementLandmarkTravel, Category: initialMovementCategory, Name: "Travel to Landmark",
			Description: "Transition to a curated shrine, quest, or Flame landmark without simulating travel time.", Aliases: []string{"mv special shrine"},
			Parameters: []ParameterDefinition{{
				ID: "landmark", Name: "Landmark", Kind: ParameterEnum, Required: true,
				Choices: worldLandmarkChoices(), ControlHint: controlHintSelect, Availability: []string{contextGameplay},
			}},
			ExecutionModes: []ExecutionMode{ExecutionSynchronized},
		},
		{
			ID: CommandMoongateStatus, Category: "Moongates", Name: "Moongate Status",
			Description:    "Show current phase, visibility, activation state, all moonstones, and the computed destination without travelling.",
			ExecutionModes: []ExecutionMode{ExecutionQuery},
		},
	}
}

func worldLandmarkChoices() []Choice {
	return []Choice{
		{Value: "honesty", Label: "Shrine of Honesty", Metadata: nil},
		{Value: "compassion", Label: "Shrine of Compassion", Metadata: nil},
		{Value: "valour", Label: "Shrine of Valour", Metadata: nil},
		{Value: "justice", Label: "Shrine of Justice", Metadata: nil},
		{Value: "sacrifice", Label: "Shrine of Sacrifice", Metadata: nil},
		{Value: "honor", Label: "Shrine of Honor", Metadata: nil},
		{Value: "humility", Label: "Shrine of Humility", Metadata: nil},
		{Value: "codex", Label: "Shrine of the Codex", Metadata: nil},
		{Value: "lord_british_secret_room", Label: "Lord British's Secret Room", Metadata: nil},
		{Value: "flame_truth", Label: "Flame of Truth (Lycaeum)", Metadata: nil},
		{Value: "flame_love", Label: "Flame of Love (Empath Abbey)", Metadata: nil},
		{Value: "flame_courage", Label: "Flame of Courage (Serpent's Hold)", Metadata: nil},
	}
}

func provisionChoices() []Choice {
	return []Choice{
		{Value: "gold", Label: "Gold", Metadata: nil},
		{Value: "torches", Label: "Torches", Metadata: nil},
		{Value: "keys", Label: "Keys", Metadata: nil},
		{Value: "food", Label: "Food", Metadata: nil},
	}
}

func questItemChoices() []Choice {
	return []Choice{
		{Value: "amulet", Label: "Amulet of Lord British", Metadata: nil},
		{Value: "crown", Label: "Crown of Lord British", Metadata: nil},
		{Value: "sceptre", Label: "Sceptre of Lord British", Metadata: nil},
		{Value: "shard_falsehood", Label: "Shard of Falsehood", Metadata: nil},
		{Value: "shard_hatred", Label: "Shard of Hatred", Metadata: nil},
		{Value: "shard_cowardice", Label: "Shard of Cowardice", Metadata: nil},
		{Value: "carpet", Label: "Magic Carpet", Metadata: nil},
		{Value: "grapple", Label: "Grapple", Metadata: nil},
		{Value: "spyglass", Label: "Spyglass", Metadata: nil},
		{Value: "hms_cape", Label: "H.M.S. Cape Plans", Metadata: nil},
		{Value: "pocket_watch", Label: "Pocket Watch", Metadata: nil},
		{Value: "black_badge", Label: "Black Badge", Metadata: nil},
		{Value: "wooden_box", Label: "Sandalwood Box", Metadata: nil},
		{Value: "sextant", Label: "Sextant", Metadata: nil},
	}
}

func shrineChoices() []Choice {
	return []Choice{
		{Value: "0", Label: "Honesty", Metadata: nil},
		{Value: "1", Label: "Compassion", Metadata: nil},
		{Value: "2", Label: "Valour", Metadata: nil},
		{Value: "3", Label: "Justice", Metadata: nil},
		{Value: "4", Label: "Sacrifice", Metadata: nil},
		{Value: "5", Label: "Honor", Metadata: nil},
		{Value: "6", Label: "Spirituality", Metadata: nil},
		{Value: "7", Label: "Humility", Metadata: nil},
	}
}
