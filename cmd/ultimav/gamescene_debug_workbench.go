package main

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

const (
	activeCharacterTokenPartCount = 2
	allActivePartyValue           = "all"
	characterChangeCapacity       = 11
	shrineChangeCapacity          = 4
	calendarChangeCapacity        = 3
	debugStatusSleep              = "sleep"
	debugMapLarge                 = "large"
	debugMapSmall                 = "small"
	debugMapCombat                = "combat"
	debugMapDungeon               = "dungeon"
	debugMapUnknown               = "unknown"
	debugProvisionGold            = "gold"
	debugProvisionTorches         = "torches"
	debugProvisionKeys            = "keys"
	debugProvisionFood            = "food"
	debugQuestAmulet              = "amulet"
	debugQuestCrown               = "crown"
	debugQuestSceptre             = "sceptre"
	debugQuestCarpet              = "carpet"
	debugQuestSextant             = "sextant"
	debugQuestSpyglass            = "spyglass"
	debugQuestCategoryShard       = "shard"
	debugQuestCategoryArtifact    = "artifact"
	debugQuestCategoryTool        = "tool"
	debugQuestGrapple             = "grapple"
	debugDirectionDown            = "down"
	debugDirectionLeft            = "left"
	debugDirectionRight           = "right"
	moongateVisibleHour           = 20
	moongateHiddenHour            = 5
	debugPercentScale             = 100
	debugStatusDead               = "dead"
)

var (
	errInvalidDebugAlterationArgument = errors.New("invalid debug alteration argument")
	errInvalidDebugCharacterSelection = errors.New("invalid debug character selection")
)

//nolint:interfacebloat // This adapter seam groups the existing Workbench state operations and their shared test doubles.
type debugWorkbenchState interface {
	GetCurrentWind() environment.WindDirection
	SetWindDirection(direction environment.WindDirection, immediate bool)
	ShrineProgress(index int) game_state.ShrineProgressSnapshot
	AlterShrineProgress(input game_state.ShrineProgressAlteration) (game_state.ShrineProgressAlterationResult, error)
	ReviewAllShrineQuestState() game_state.AllShrineQuestStateReview
	ActivePartyCharacters() []game_state.ActiveCharacterSnapshot
	AlterActiveCharacter(input game_state.ActiveCharacterAlteration) (game_state.ActiveCharacterAlterationResult, error)
	CurrentPartyRosterSnapshot() game_state.PartyRosterSnapshot
	MaximizeActiveParty(expectedFingerprint string) (game_state.PartyRosterMutationResult, error)
	RestoreActiveParty(expectedFingerprint string) (game_state.PartyRosterMutationResult, error)
	SetPartyRosterStatuses(request game_state.PartyStatusMutationRequest) (game_state.PartyRosterMutationResult, error)
	FillActiveParty(expectedFingerprint string) (game_state.PartyRosterMutationResult, error)
	AddInactiveRosterMember(
		request game_state.AddInactiveRosterMemberRequest,
	) (game_state.PartyRosterMutationResult, error)
	CurrentPartyKarma() int
	SetPartyKarma(value int) (game_state.PartyKarmaAlteration, error)
}

type windStatusResult struct {
	Direction string `json:"direction"`
}

type loggingStatusResult struct {
	Enabled        bool            `json:"enabled"`
	StdoutEnabled  bool            `json:"stdoutEnabled"`
	MinimumLevel   string          `json:"minimumLevel"`
	CategoryStates map[string]bool `json:"categoryStates"`
}

func newLoggingStatusResult() loggingStatusResult {
	logger := logging.GetGlobalLogger()
	configured := logger.GetCategoryStates()
	categories := make(map[string]bool, len(logging.GetAllCategories()))
	for _, category := range logging.GetAllCategories() {
		categories[category] = configured[category]
	}

	return loggingStatusResult{
		Enabled: logger.IsEnabled(), StdoutEnabled: logger.IsStdoutEnabled(),
		MinimumLevel: logger.GetMinLevel().String(), CategoryStates: categories,
	}
}

type shrineReferenceResult struct {
	Index       int    `json:"index"`
	VirtueName  string `json:"virtueName"`
	VirtueKey   string `json:"virtueKey"`
	Mantra      string `json:"mantra"`
	WordOfPower string `json:"wordOfPower"`
	WorldX      byte   `json:"worldX"`
	WorldY      byte   `json:"worldY"`
	StrBonus    byte   `json:"strBonus"`
	DexBonus    byte   `json:"dexBonus"`
	IntBonus    byte   `json:"intBonus"`
	KarmaBonus  int    `json:"karmaBonus"`
}

type shrineStatusResult struct {
	Fingerprint      string              `json:"fingerprint"`
	Shrines          []shrineStatusEntry `json:"shrines"`
	FirstActiveIndex int                 `json:"firstActiveIndex"`
	AllCompleted     bool                `json:"allCompleted"`
}

type shrineStatusEntry struct {
	shrineReferenceResult
	QuestState string `json:"questState"`
	Completed  bool   `json:"completed"`
	Active     bool   `json:"active"`
	Ruined     bool   `json:"ruined"`
}

type activePartyResult struct {
	Fingerprint string                       `json:"fingerprint"`
	Karma       int                          `json:"karma"`
	Characters  []activeCharacterResult      `json:"characters"`
	Roster      []partyRosterCharacterResult `json:"roster"`
}

type partyRosterCharacterResult struct {
	RosterIndex  int    `json:"rosterIndex"`
	Name         string `json:"name"`
	Gender       byte   `json:"gender"`
	Class        string `json:"class"`
	Status       string `json:"status"`
	Active       bool   `json:"active"`
	PartyMap     byte   `json:"partyMap"`
	Strength     byte   `json:"strength"`
	Dexterity    byte   `json:"dexterity"`
	Intelligence byte   `json:"intelligence"`
	CurrentMP    byte   `json:"currentMp"`
	CurrentHP    uint16 `json:"currentHp"`
	MaximumHP    uint16 `json:"maximumHp"`
	Experience   uint16 `json:"experience"`
	Level        byte   `json:"level"`
	MonthsAtInn  byte   `json:"monthsAtInn"`
	ArmorClass   byte   `json:"armorClass"`
	Helmet       byte   `json:"helmet"`
	Armor        byte   `json:"armor"`
	Weapon       byte   `json:"weapon"`
	Shield       byte   `json:"shield"`
	Ring         byte   `json:"ring"`
	Amulet       byte   `json:"amulet"`
}

type partyRosterResult struct {
	Fingerprint string                       `json:"fingerprint"`
	Characters  []partyRosterCharacterResult `json:"characters"`
}

type partyRosterMutationResult struct {
	Before partyRosterResult `json:"before"`
	After  partyRosterResult `json:"after"`
}

type activeCharacterResult struct {
	RosterIndex   int    `json:"rosterIndex"`
	PartyPosition int    `json:"partyPosition"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Status        string `json:"status"`
	Strength      byte   `json:"strength"`
	Dexterity     byte   `json:"dexterity"`
	Intelligence  byte   `json:"intelligence"`
	CurrentMP     byte   `json:"currentMp"`
	CurrentHP     uint16 `json:"currentHp"`
	MaximumHP     uint16 `json:"maximumHp"`
	Experience    uint16 `json:"experience"`
	Level         byte   `json:"level"`
}

type activeCharacterOptionMetadata struct {
	PartyPosition int    `json:"partyPosition"`
	Name          string `json:"name"`
	Class         string `json:"class"`
	Status        string `json:"status"`
	Strength      byte   `json:"strength"`
	Dexterity     byte   `json:"dexterity"`
	Intelligence  byte   `json:"intelligence"`
	CurrentMP     byte   `json:"currentMp"`
	CurrentHP     uint16 `json:"currentHp"`
	MaximumHP     uint16 `json:"maximumHp"`
	Experience    uint16 `json:"experience"`
	Level         byte   `json:"level"`
	Karma         int    `json:"karma"`
}

type characterAlterationResult struct {
	Character activeCharacterResult  `json:"character"`
	Order     []partyOrderSlotResult `json:"order"`
}

type partyKarmaResult struct {
	Value int `json:"value"`
}

type partyOrderSlotResult struct {
	PartyPosition int    `json:"partyPosition"`
	Name          string `json:"name"`
}

type calendarStatusResult struct {
	Year           uint16 `json:"year"`
	Month          byte   `json:"month"`
	Day            byte   `json:"day"`
	Hour           byte   `json:"hour"`
	Minute         byte   `json:"minute"`
	Turn           uint32 `json:"turn"`
	Date           string `json:"date"`
	Time           string `json:"time"`
	MoonPhase      string `json:"moonPhase"`
	MoonPhaseIndex int    `json:"moonPhaseIndex"`
}

type searchStatusResult struct {
	LocationID     int                  `json:"locationId"`
	LocationName   string               `json:"locationName"`
	MapType        string               `json:"mapType"`
	Floor          int                  `json:"floor"`
	Cells          []searchCellResult   `json:"cells"`
	Progress       searchProgressResult `json:"progress"`
	OverlayEnabled bool                 `json:"overlayEnabled"`
}

type searchProgressResult struct {
	TotalLocations        int `json:"totalLocations"`
	FoundLocations        int `json:"foundLocations"`
	OneTimeLocations      int `json:"oneTimeLocations"`
	FoundOneTimeLocations int `json:"foundOneTimeLocations"`
}

type searchCellResult struct {
	Direction          string                          `json:"direction"`
	X                  int                             `json:"x"`
	Y                  int                             `json:"y"`
	LocationID         int                             `json:"locationId"`
	LocationName       string                          `json:"locationName"`
	MapType            string                          `json:"mapType"`
	Floor              int                             `json:"floor"`
	InBounds           bool                            `json:"inBounds"`
	TileIndex          *int                            `json:"tileIndex,omitempty"`
	TileName           string                          `json:"tileName,omitempty"`
	AuthenticLocations []authenticSearchLocationResult `json:"authenticLocations"`
	ItemStack          []itemStackEntryResult          `json:"itemStack"`
}

type authenticSearchLocationResult struct {
	Index          int    `json:"index"`
	Found          bool   `json:"found"`
	Repeatable     bool   `json:"repeatable"`
	RawObjectType  int    `json:"rawObjectType"`
	RawPayload     int    `json:"rawPayload"`
	Classification string `json:"classification"`
	Category       string `json:"category"`
	ItemType       int    `json:"itemType"`
	ItemID         int    `json:"itemId"`
	ItemName       string `json:"itemName"`
	Quantity       uint16 `json:"quantity"`
	DiscoveryText  string `json:"discoveryText"`
}

type itemStackEntryResult struct {
	ItemType int    `json:"itemType"`
	ItemID   int    `json:"itemId"`
	ItemName string `json:"itemName"`
	Quantity uint16 `json:"quantity"`
}

type questItemStatusResult struct {
	Items []questItemResult `json:"items"`
}

type questItemResult struct {
	Key                string                    `json:"key"`
	Name               string                    `json:"name"`
	Category           string                    `json:"category"`
	Quantity           uint16                    `json:"quantity"`
	CanonicalPlacement *canonicalPlacementResult `json:"canonicalPlacement,omitempty"`
	ShardUseReview     *shardUseReviewResult     `json:"shardUseReview,omitempty"`
}

type canonicalPlacementResult struct {
	LocationID   int    `json:"locationId"`
	LocationName string `json:"locationName"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Floor        int    `json:"floor"`
}

type worldPositionResult struct {
	LocationID   int    `json:"locationId"`
	LocationName string `json:"locationName"`
	MapType      string `json:"mapType"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	Floor        int    `json:"floor"`
	MaximumX     int    `json:"maximumX"`
	MaximumY     int    `json:"maximumY"`
	MinimumFloor *int   `json:"minimumFloor,omitempty"`
	MaximumFloor *int   `json:"maximumFloor,omitempty"`
}

type moongateStatusResult struct {
	SourceFingerprint   string                 `json:"sourceFingerprint"`
	Phase               string                 `json:"phase"`
	PhaseIndex          int                    `json:"phaseIndex"`
	Visible             bool                   `json:"visible"`
	PlayerOnActiveGate  bool                   `json:"playerOnActiveGate"`
	TeleportationActive bool                   `json:"teleportationActive"`
	Moonstones          []moonstoneStateResult `json:"moonstones"`
	Route               *moongateRouteResult   `json:"route,omitempty"`
	UnavailableReason   string                 `json:"unavailableReason,omitempty"`
	OverlayEnabled      bool                   `json:"overlayEnabled"`
}

type moongateRouteResult struct {
	Fingerprint string                     `json:"fingerprint"`
	Kind        string                     `json:"kind"`
	Destination *moongateDestinationResult `json:"destination,omitempty"`
	ShrineIndex *int                       `json:"shrineIndex,omitempty"`
	ShrineName  string                     `json:"shrineName,omitempty"`
}

type moonstoneStateResult struct {
	Index int    `json:"index"`
	Phase string `json:"phase"`
	X     uint8  `json:"x"`
	Y     uint8  `json:"y"`
	Map   uint8  `json:"map"`
	Level uint8  `json:"level"`
	State string `json:"state"`
	Found bool   `json:"found"`
}

type moongateDestinationResult struct {
	X            uint8  `json:"x"`
	Y            uint8  `json:"y"`
	Map          uint8  `json:"map"`
	Level        uint8  `json:"level"`
	LocationID   int    `json:"locationId"`
	LocationName string `json:"locationName"`
}

// DebugOptionSets returns detached dynamic choices while running on Game.Update.
func (g *GameScene) DebugOptionSets() []debugging.OptionSet {
	characters := g.gameState.ActivePartyCharacters()

	return []debugging.OptionSet{
		activeCharacterOptionSet(characters, g.gameState.PartyState.Karma.Value, ""),
		activePartyPositionOptionSet(characters, g.gameState.PartyState.Karma.Value, ""),
		equipmentLoadoutTargetOptionSet(characters, g.gameState.PartyState.Karma.Value, ""),
		g.merchantTesterOptionSet(),
	}
}

func activeCharacterOptionSet(
	characters []game_state.ActiveCharacterSnapshot,
	karma int,
	unavailableReason string,
) debugging.OptionSet {
	choices := make([]debugging.Choice, 0, len(characters))
	for _, character := range characters {
		choices = append(choices, debugging.Choice{
			Value: activeCharacterToken(character.Slot, character.Name),
			Label: fmt.Sprintf("Position %d · %s · %s", character.Slot+1, character.Name, character.Class.String()),
			Metadata: mustDebugJSON(activeCharacterOptionMetadata{
				PartyPosition: character.Slot + 1,
				Name:          character.Name,
				Class:         character.Class.String(),
				Status:        characterStatusName(character.Status),
				Strength:      character.Strength,
				Dexterity:     character.Dexterity,
				Intelligence:  character.Intelligence,
				CurrentMP:     character.CurrentMP,
				CurrentHP:     character.CurrentHP,
				MaximumHP:     character.MaximumHP,
				Experience:    character.Experience,
				Level:         character.Level,
				Karma:         karma,
			}),
		})
	}

	return debugging.OptionSet{
		Provider:          debugging.OptionProviderActivePartyCharacters,
		Choices:           choices,
		UnavailableReason: unavailableReason,
	}
}

func activePartyPositionOptionSet(
	characters []game_state.ActiveCharacterSnapshot,
	karma int,
	unavailableReason string,
) debugging.OptionSet {
	choices := make([]debugging.Choice, 0, len(characters))
	for _, character := range characters {
		choices = append(choices, debugging.Choice{
			Value: strconv.Itoa(character.Slot + 1),
			Label: fmt.Sprintf("Position %d · %s", character.Slot+1, character.Name),
			Metadata: mustDebugJSON(activeCharacterOptionMetadata{
				PartyPosition: character.Slot + 1,
				Name:          character.Name,
				Class:         character.Class.String(),
				Status:        characterStatusName(character.Status),
				Strength:      character.Strength,
				Dexterity:     character.Dexterity,
				Intelligence:  character.Intelligence,
				CurrentMP:     character.CurrentMP,
				CurrentHP:     character.CurrentHP,
				MaximumHP:     character.MaximumHP,
				Experience:    character.Experience,
				Level:         character.Level,
				Karma:         karma,
			}),
		})
	}

	return debugging.OptionSet{
		Provider:          debugging.OptionProviderActivePartyPositions,
		Choices:           choices,
		UnavailableReason: unavailableReason,
	}
}

func equipmentLoadoutTargetOptionSet(
	characters []game_state.ActiveCharacterSnapshot,
	karma int,
	unavailableReason string,
) debugging.OptionSet {
	characterOptions := activeCharacterOptionSet(characters, karma, unavailableReason)
	choices := make([]debugging.Choice, 0, len(characterOptions.Choices)+1)
	choices = append(choices, debugging.Choice{Value: allActivePartyValue, Label: "All active party members"})
	choices = append(choices, characterOptions.Choices...)

	return debugging.OptionSet{
		Provider:          debugging.OptionProviderEquipmentLoadoutTargets,
		Choices:           choices,
		UnavailableReason: unavailableReason,
	}
}

// ExecuteDebugCommand runs a validated workbench command on the update thread.
func (g *GameScene) ExecuteDebugCommand(request debugging.Request) debugging.Result {
	return initialDebugCommandRegistry.executeScene(g, request)
}

//nolint:maintidx // Explicit command dispatch keeps each stable wire ID adjacent to its authority call and result mapping.
func executeDebugCommand(gameState debugWorkbenchState, request debugging.Request) debugging.Result {
	switch request.CommandID {
	case debugging.CommandEnvironmentWindStatus:
		wind := gameState.GetCurrentWind()

		return debugSuccessResult(request, "Current wind: "+wind.String(), windStatusResult{
			Direction: wind.String(),
		})
	case debugging.CommandEnvironmentWindSet:
		direction, ok := debugWindDirection(mustDebugStringArgument(request, "direction"))
		if !ok {
			return debugging.FailureResult(request, "invalid_wind", "unknown wind direction")
		}
		before := gameState.GetCurrentWind()
		gameState.SetWindDirection(direction, true)
		after := gameState.GetCurrentWind()
		result := debugSuccessResultWithMode(
			request,
			debugging.ExecutionLiveSafe,
			"Wind changed to "+after.String()+".",
			windStatusResult{Direction: after.String()},
		)
		if before != after {
			result.Changes = []debugging.StateChange{{
				Path:   "environment.wind.direction",
				Before: mustDebugJSON(before.String()),
				After:  mustDebugJSON(after.String()),
			}}
		}

		return result
	case debugging.CommandShrineStatus:
		return debugSuccessResult(request, "Shrine atlas loaded.", newShrineStatusResult(gameState))
	case debugging.CommandShrineProgressUpdate:
		input, err := shrineProgressAlterationFromRequest(request)
		if err != nil {
			return debugging.FailureResult(request, "invalid_shrine_progress", err.Error())
		}
		alteration, err := gameState.AlterShrineProgress(input)
		if err != nil {
			return debugging.FailureResult(request, "invalid_shrine_progress", err.Error())
		}
		shrine := newShrineStatusEntry(input.Index, alteration.After)
		result := debugSuccessResultWithMode(
			request,
			debugging.ExecutionLiveSafe,
			shrine.VirtueName+" shrine progression updated.",
			shrine,
		)
		result.Changes = shrineProgressChanges(shrine.VirtueKey, alteration)

		return result
	case debugging.CommandPartyCharacterInspect:
		characters := gameState.ActivePartyCharacters()
		roster := gameState.CurrentPartyRosterSnapshot()
		result := activePartyResult{
			Fingerprint: roster.Fingerprint,
			Karma:       gameState.CurrentPartyKarma(),
			Characters:  make([]activeCharacterResult, 0, len(characters)),
			Roster:      newPartyRosterResult(roster).Characters,
		}
		for _, character := range characters {
			result.Characters = append(result.Characters, newActiveCharacterResult(character))
		}

		return debugSuccessResult(
			request, fmt.Sprintf("Full party roster loaded: %d active characters.", len(characters)), result,
		)
	case debugging.CommandPartyCharacterUpdate:
		slot, expectedOccupant, err := parseActiveCharacterToken(mustDebugStringArgument(request, "character"))
		if err != nil {
			return debugging.FailureResult(request, "invalid_character", err.Error())
		}
		input, err := characterAlterationFromRequest(request, slot, expectedOccupant)
		if err != nil {
			return debugging.FailureResult(request, "invalid_alteration", err.Error())
		}
		alteration, err := gameState.AlterActiveCharacter(input)
		if err != nil {
			return debugging.FailureResult(request, "invalid_alteration", err.Error())
		}
		result := characterAlterationResult{
			Character: newActiveCharacterResult(alteration.CharacterAfter),
			Order:     newPartyOrderResult(alteration.OrderAfter),
		}
		debugResult := debugSuccessResultWithMode(
			request,
			debugging.ExecutionLiveSafe,
			alteration.CharacterAfter.Name+" updated.",
			result,
		)
		debugResult.Changes = characterAlterationChanges(alteration)

		return debugResult
	case debugging.CommandPartyEquipmentLoadoutUpdate:
		return executePartyEquipmentLoadoutUpdate(request, concreteDebugGameState(gameState))
	case debugging.CommandPartyMaximizeLevels:
		mutation, err := gameState.MaximizeActiveParty(mustDebugStringArgument(request, "expectedFingerprint"))

		return partyRosterMutationDebugResult(request, "Active party maximized to level eight.", mutation, err)
	case debugging.CommandPartyRestore:
		mutation, err := gameState.RestoreActiveParty(mustDebugStringArgument(request, "expectedFingerprint"))

		return partyRosterMutationDebugResult(request, "Active party healed and cured.", mutation, err)
	case debugging.CommandPartyStatusSet:
		status, ok := debugRosterStatus(mustDebugStringArgument(request, "status"))
		if !ok {
			return debugging.FailureResult(request, "invalid_party_mutation", "unknown party status")
		}
		mutation, err := gameState.SetPartyRosterStatuses(game_state.PartyStatusMutationRequest{
			ExpectedFingerprint: mustDebugStringArgument(request, "expectedFingerprint"),
			RosterIndices:       mustDebugIntegerListArgument(request, "rosterIndices"),
			Status:              status,
		})

		return partyRosterMutationDebugResult(request, "Party status updated.", mutation, err)
	case debugging.CommandPartyFill:
		mutation, err := gameState.FillActiveParty(mustDebugStringArgument(request, "expectedFingerprint"))

		return partyRosterMutationDebugResult(request, "Active party filled.", mutation, err)
	case debugging.CommandPartyMemberAdd:
		mutation, err := gameState.AddInactiveRosterMember(game_state.AddInactiveRosterMemberRequest{
			ExpectedFingerprint: mustDebugStringArgument(request, "expectedFingerprint"),
			RosterIndex:         mustDebugIntegerArgument(request, "rosterIndex"),
		})

		return partyRosterMutationDebugResult(request, "Roster member added to the active party.", mutation, err)
	case debugging.CommandPartyKarmaSet:
		alteration, err := gameState.SetPartyKarma(mustDebugIntegerArgument(request, "value"))
		if err != nil {
			return debugging.FailureResult(request, "invalid_party_karma", err.Error())
		}
		result := debugSuccessResultWithMode(
			request,
			debugging.ExecutionLiveSafe,
			"Party karma updated.",
			partyKarmaResult{Value: alteration.After},
		)
		appendSemanticChange(&result.Changes, "party.karma", alteration.Before, alteration.After)

		return result
	case debugging.CommandCalendarStatus:
		return executeCalendarStatus(request, concreteDebugGameState(gameState))
	case debugging.CommandCalendarSetTime:
		return executeCalendarSetTime(request, concreteDebugGameState(gameState))
	case debugging.CommandCalendarSetDay:
		return executeCalendarSetDay(request, concreteDebugGameState(gameState))
	case debugging.CommandSearchStatus:
		return executeSearchStatus(request, concreteDebugGameState(gameState))
	case debugging.CommandSearchItemStackAdd:
		return executeSearchItemStackAdd(request, concreteDebugGameState(gameState))
	case debugging.CommandSearchProgressReset:
		return executeSearchProgressReset(request, concreteDebugGameState(gameState))
	case debugging.CommandQuestItemStatus:
		return executeQuestItemStatus(request, concreteDebugGameState(gameState))
	case debugging.CommandQuestItemEnsure:
		return executeQuestItemMutation(request, concreteDebugGameState(gameState), true)
	case debugging.CommandQuestItemRemove:
		return executeQuestItemMutation(request, concreteDebugGameState(gameState), false)
	case debugging.CommandWorldPositionStatus:
		return executeWorldPositionStatus(request, concreteDebugGameState(gameState))
	case debugging.CommandMoongateStatus:
		return executeMoongateStatus(request, concreteDebugGameState(gameState))
	case debugging.CommandInventoryLockerStatus,
		debugging.CommandInventoryProvisionSet,
		debugging.CommandInventoryProvisionPreset,
		debugging.CommandInventoryReagentSet,
		debugging.CommandInventoryReagentPreset,
		debugging.CommandInventoryPotionAdd,
		debugging.CommandInventoryPotionPreset,
		debugging.CommandInventoryScrollAdd,
		debugging.CommandInventoryScrollPreset,
		debugging.CommandInventoryEquipmentSet,
		debugging.CommandInventoryEquipmentPreset,
		debugging.CommandInventoryEquipmentGrantPreset,
		debugging.CommandInventoryEquipmentClear:
		result, _ := executeInventoryLockerDebugCommand(request, concreteDebugGameState(gameState))

		return result
	case debugging.CommandInventorySpellStatus,
		debugging.CommandInventorySpellSet,
		debugging.CommandInventorySpellPresetMax,
		debugging.CommandSpellEffectSetRemaining,
		debugging.CommandSpellEffectActivate:
		result, _ := executeSpellDebugCommand(request, concreteDebugGameState(gameState))

		return result
	case debugging.CommandCombatStatus,
		debugging.CommandCombatMonsterSleepSet,
		debugging.CommandCombatMonsterSleepAll:
		result, _ := executeActiveCombatDebugCommand(request, concreteDebugGameState(gameState))

		return result
	case debugging.CommandCombatEncounterOptions,
		debugging.CommandCombatEncounterStart,
		debugging.CommandMonsterEcosystemStatus,
		debugging.CommandMonsterGenerationSet,
		debugging.CommandMonsterClearMapEnemies,
		debugging.CommandMonsterSpawnWhirlpool,
		debugging.CommandMonsterSpawnPirate,
		debugging.CommandVehicleStatus,
		debugging.CommandVehicleSpawn,
		debugging.CommandVehicleForceBoard,
		debugging.CommandShipDamageRaw,
		debugging.CommandShipRepairFull,
		debugging.CommandShipSkiffSet:
		return executeSecondBatchStateDebugCommand(request, concreteDebugGameState(gameState))
	default:
		if result, ok := executeApplicationDebugCommand(request, nil); ok {
			return result
		}

		return debugging.FailureResult(
			request,
			"command_not_implemented",
			"the active gameplay scene does not implement this command",
		)
	}
}

func concreteDebugGameState(state debugWorkbenchState) *game_state.GameState {
	gameState, ok := state.(*game_state.GameState)
	if !ok {
		panic("approved debug tool requires the active concrete GameState")
	}

	return gameState
}

func executeCalendarStatus(request debugging.Request, state *game_state.GameState) debugging.Result {
	return debugSuccessResult(request, "Calendar status loaded.", newCalendarStatusResult(state))
}

func executeCalendarSetTime(request debugging.Request, state *game_state.GameState) debugging.Result {
	hour := mustDebugIntegerArgument(request, "hour")
	minute := mustDebugIntegerArgument(request, "minute")
	change, err := state.SetCalendarTime(hour, minute)
	if err != nil {
		return debugging.FailureResult(request, "invalid_calendar_time", err.Error())
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionLiveSafe,
		"Calendar time updated.",
		newCalendarStatusResult(state),
	)
	result.Changes = dateTimeChanges(change)

	return result
}

func executeCalendarSetDay(request debugging.Request, state *game_state.GameState) debugging.Result {
	day := mustDebugIntegerArgument(request, "day")
	change, err := state.SetCalendarDay(day)
	if err != nil {
		return debugging.FailureResult(request, "invalid_calendar_day", err.Error())
	}
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionLiveSafe,
		"Calendar day updated.",
		newCalendarStatusResult(state),
	)
	result.Changes = dateTimeChanges(change)

	return result
}

func newCalendarStatusResult(state *game_state.GameState) calendarStatusResult {
	date := state.DateTime
	moonPhase := date.GetCurrentMoonPhase()

	return calendarStatusResult{
		Year: date.Year, Month: date.Month, Day: date.Day, Hour: date.Hour, Minute: date.Minute, Turn: date.Turn,
		Date: date.GetDateAsString(), Time: date.GetTimeAsString(), MoonPhase: moonPhase.String(), MoonPhaseIndex: moonPhase.ToMoonstoneIndex(),
	}
}

func dateTimeChanges(change game_state.CalendarChange) []debugging.StateChange {
	changes := make([]debugging.StateChange, 0, calendarChangeCapacity)
	appendSemanticChange(&changes, "calendar.day", change.Before.Day, change.After.Day)
	appendSemanticChange(&changes, "calendar.time.hour", change.Before.Hour, change.After.Hour)
	appendSemanticChange(&changes, "calendar.time.minute", change.Before.Minute, change.After.Minute)

	return changes
}

func executeSearchStatus(request debugging.Request, state *game_state.GameState) debugging.Result {
	status := newSearchStatusResult(state)

	return debugSuccessResult(request, fmt.Sprintf("Search status loaded for %d nearby cells.", len(status.Cells)), status)
}

func newSearchStatusResult(state *game_state.GameState) searchStatusResult {
	location := state.MapState.PlayerLocation
	progress := searchProgressResult{
		TotalLocations:        0,
		FoundLocations:        0,
		OneTimeLocations:      0,
		FoundOneTimeLocations: 0,
	}
	manager := state.SearchManager.GetAuthenticLocationManager()
	allLocations := references.GetAllSearchLocations()
	progress.TotalLocations = len(allLocations)
	progress.FoundLocations = manager.GetFoundCount()
	for _, searchLocation := range allLocations {
		if searchLocation.Requirements.IsSpecialItem || searchLocation.Requirements.IsRepeatable {
			continue
		}
		progress.OneTimeLocations++
		if manager.IsLocationFound(searchLocation.Index) {
			progress.FoundOneTimeLocations++
		}
	}

	offsets := []struct {
		name string
		x    int
		y    int
	}{
		{"current", 0, 0}, {"up", 0, -1}, {debugDirectionDown, 0, 1},
		{debugDirectionLeft, -1, 0}, {debugDirectionRight, 1, 0},
	}
	cells := make([]searchCellResult, 0, len(offsets))
	maximumX, maximumY := debugMapMaxima(location.Location.GetMapType())
	for _, offset := range offsets {
		coordinateX := int(location.Position.X) + offset.x
		coordinateY := int(location.Position.Y) + offset.y
		cell := searchCellResult{
			Direction: offset.name, X: coordinateX, Y: coordinateY,
			LocationID:         int(location.Location),
			LocationName:       location.Location.String(),
			MapType:            debugMapTypeName(location.Location.GetMapType()),
			Floor:              int(location.Floor),
			InBounds:           coordinateX >= 0 && coordinateX <= maximumX && coordinateY >= 0 && coordinateY <= maximumY,
			TileIndex:          nil,
			TileName:           "",
			AuthenticLocations: make([]authenticSearchLocationResult, 0),
			ItemStack:          make([]itemStackEntryResult, 0),
		}
		if cell.InBounds {
			position := references.Position{
				X: references.Coordinate(coordinateX), //nolint:gosec // Catalog and logical bounds constrain coordinates.
				Y: references.Coordinate(coordinateY), //nolint:gosec // Catalog and logical bounds constrain coordinates.
			}
			if layeredMap := state.GetLayeredMapByCurrentLocation(); layeredMap != nil {
				if tile := layeredMap.GetTopTile(&position); tile != nil {
					index := int(tile.Index)
					cell.TileIndex = &index
					cell.TileName = tile.Name
				}
			}
			for _, searchLocation := range allLocations {
				if searchLocation.MapNumber != int(location.Location) ||
					searchLocation.Level != int(location.Floor) ||
					searchLocation.X != coordinateX || searchLocation.Y != coordinateY {
					continue
				}
				decoded := references.DecodeSearchLocationItem(searchLocation)
				itemType := -1
				if decoded.Item != nil {
					itemType = int(decoded.Item.Type())
				}
				cell.AuthenticLocations = append(cell.AuthenticLocations, authenticSearchLocationResult{
					Index: searchLocation.Index, Found: manager.IsLocationFound(searchLocation.Index),
					Repeatable: searchLocation.Requirements.IsRepeatable, RawObjectType: int(decoded.RawObjectType),
					RawPayload: int(decoded.RawPayload), Classification: decoded.Classification, Category: decoded.Category,
					ItemType: itemType, ItemID: decoded.ItemID, ItemName: decoded.ItemName,
					Quantity: uint16(decoded.PickupQuantity), DiscoveryText: decoded.DiscoveryText, //nolint:gosec // Original payloads are byte-sized.
				})
			}
			if stack, exists := state.ItemStacksMap.Snapshot(&position); exists {
				for _, item := range stack.Items {
					if item.Item == nil {
						continue
					}
					cell.ItemStack = append(cell.ItemStack, itemStackEntryResult{
						ItemType: int(item.Item.Type()),
						ItemID:   item.Item.ID(),
						ItemName: debugItemName(state, item.Item),
						Quantity: item.Quantity,
					})
				}
			}
		}
		cells = append(cells, cell)
	}

	return searchStatusResult{
		LocationID:     int(location.Location),
		LocationName:   location.Location.String(),
		MapType:        debugMapTypeName(location.Location.GetMapType()),
		Floor:          int(location.Floor),
		Cells:          cells,
		Progress:       progress,
		OverlayEnabled: state.DebugOptions.SearchOverlay,
	}
}

func executeSearchItemStackAdd(request debugging.Request, state *game_state.GameState) debugging.Result {
	coordinateX := mustDebugIntegerArgument(request, "x")
	coordinateY := mustDebugIntegerArgument(request, "y")
	quantity := mustDebugIntegerArgument(request, "quantity")
	provision, ok := debugProvision(mustDebugStringArgument(request, "item"))
	if !ok {
		return debugging.FailureResult(request, "invalid_search_item", "unsupported search provision")
	}
	position := references.Position{
		X: references.Coordinate(coordinateX), //nolint:gosec // Catalog bounds constrain coordinates to 0..255.
		Y: references.Coordinate(coordinateY), //nolint:gosec // Catalog bounds constrain coordinates to 0..255.
	}
	before := debugProvisionQuantity(state, position, provision)
	if err := state.AddDebugSearchProvision(
		position,
		provision,
		uint16(quantity), //nolint:gosec // Catalog bounds constrain quantity to 1..999.
	); err != nil {
		return debugging.FailureResult(request, "invalid_search_position", err.Error())
	}
	after := debugProvisionQuantity(state, position, provision)
	result := debugSuccessResultWithMode(
		request, debugging.ExecutionLiveSafe, "Search item stack updated.", newSearchStatusResult(state),
	)
	path := fmt.Sprintf(
		"search.itemStacks.%d.%d.%s", coordinateX, coordinateY, mustDebugStringArgument(request, "item"),
	)
	appendSemanticChange(&result.Changes, path, before, after)

	return result
}

func executeSearchProgressReset(request debugging.Request, state *game_state.GameState) debugging.Result {
	before, after := state.ResetAuthenticSearchFoundFlags()
	result := debugSuccessResultWithMode(
		request,
		debugging.ExecutionAlteration,
		"One-time authentic search progress reset.",
		newSearchStatusResult(state).Progress,
	)
	appendSemanticChange(&result.Changes, "search.progress.one_time_locations.found", before, after)

	return result
}

func executeQuestItemStatus(request debugging.Request, state *game_state.GameState) debugging.Result {
	status := newQuestItemStatusResult(state)

	return debugSuccessResult(request, fmt.Sprintf("Quest item status loaded: %d items.", len(status.Items)), status)
}

func executeQuestItemMutation(request debugging.Request, state *game_state.GameState, present bool) debugging.Result {
	key := mustDebugStringArgument(request, "item")
	item, ok := debugQuestItem(key)
	if !ok {
		return debugging.FailureResult(request, "invalid_quest_item", "unsupported quest item")
	}
	before, after, err := state.SetQuestItemPresent(item, present)
	if err != nil {
		return debugging.FailureResult(request, "invalid_quest_item", err.Error())
	}
	status := newQuestItemResult(state, item, debugQuestPlacements())
	verb := "ensured"
	if !present {
		verb = "removed"
	}
	result := debugSuccessResultWithMode(request, debugging.ExecutionAlteration, status.Name+" "+verb+".", status)
	appendSemanticChange(&result.Changes, "quest.items."+key+".quantity", before, after)

	return result
}

func newQuestItemStatusResult(state *game_state.GameState) questItemStatusResult {
	placements := debugQuestPlacements()
	items := make([]questItemResult, 0, references.NumberOfQuestItems)
	for item := references.QuestItem(0); item < references.NumberOfQuestItems; item++ {
		items = append(items, newQuestItemResult(state, item, placements))
	}

	return questItemStatusResult{Items: items}
}

func newQuestItemResult(
	state *game_state.GameState,
	item references.QuestItem,
	placements map[references.QuestItem]canonicalPlacementResult,
) questItemResult {
	key, category := debugQuestItemIdentity(item)
	result := questItemResult{
		Key: key, Name: debugQuestItemName(state, item), Category: category,
		Quantity:           state.PartyState.Inventory.QuestItems.Get(item),
		CanonicalPlacement: nil,
		ShardUseReview:     nil,
	}
	if isShard(item) {
		review := newShardUseReviewResult(state, item)
		result.ShardUseReview = &review
	}
	if placement, ok := placements[item]; ok {
		placementCopy := placement
		result.CanonicalPlacement = &placementCopy
	}

	return result
}

func debugQuestPlacements() map[references.QuestItem]canonicalPlacementResult {
	placements := make(map[references.QuestItem]canonicalPlacementResult)
	for _, placement := range game_state.GetAllQuestItemPlacements() {
		placements[placement.QuestItemType] = canonicalPlacementResult{
			LocationID:   int(placement.Location),
			LocationName: placement.Location.String(),
			X:            int(placement.Position.X),
			Y:            int(placement.Position.Y),
			Floor:        int(placement.Floor),
		}
	}

	return placements
}

func executeWorldPositionStatus(request debugging.Request, state *game_state.GameState) debugging.Result {
	return debugSuccessResult(request, "World position loaded.", newWorldPositionStatusData(state))
}

func newWorldPositionResult(state *game_state.GameState) worldPositionResult {
	location := state.MapState.PlayerLocation
	maximumX, maximumY := debugMapMaxima(location.Location.GetMapType())
	result := worldPositionResult{
		LocationID:   int(location.Location),
		LocationName: location.Location.String(),
		MapType:      debugMapTypeName(location.Location.GetMapType()),
		X:            int(location.Position.X),
		Y:            int(location.Position.Y),
		Floor:        int(location.Floor),
		MaximumX:     maximumX,
		MaximumY:     maximumY,
		MinimumFloor: nil,
		MaximumFloor: nil,
	}
	if location.Location.GetMapType() == references.LargeMapType {
		minimumValue, maximumValue := -1, 0
		result.MinimumFloor, result.MaximumFloor = &minimumValue, &maximumValue
	}
	if state.GameReferences != nil && state.GameReferences.LocationReferences != nil &&
		location.Location.GetMapType() != references.LargeMapType {
		locationReference := state.GameReferences.LocationReferences.GetLocationReference(location.Location)
		if locationReference != nil {
			minimum, maximum := locationReference.GetFloorMinMax()
			minimumValue, maximumValue := int(minimum), int(maximum)
			result.MinimumFloor, result.MaximumFloor = &minimumValue, &maximumValue
		}
	}

	return result
}

func executeMoongateStatus(request debugging.Request, state *game_state.GameState) debugging.Result {
	return executeMoongateStatusWithOverlay(request, state, false)
}

func executeMoongateStatusWithOverlay(
	request debugging.Request,
	state *game_state.GameState,
	overlayEnabled bool,
) debugging.Result {
	date := state.DateTime
	phase := date.GetCurrentMoonPhase()
	moonstones := state.GetMoonstones()
	status := moongateStatusResult{
		SourceFingerprint: moongateSourceFingerprint(state),
		Phase:             phase.String(), PhaseIndex: int(phase), PlayerOnActiveGate: state.IsPlayerOnActiveMoongate(),
		TeleportationActive: state.IsTeleportationActive(),
		Visible:             false,
		Moonstones:          make([]moonstoneStateResult, 0, len(moonstones)),
		Route:               nil,
		UnavailableReason:   "",
		OverlayEnabled:      overlayEnabled,
	}
	night := date.Hour >= moongateVisibleHour || date.Hour < moongateHiddenHour
	for index, moonstone := range moonstones {
		status.Moonstones = append(status.Moonstones, moonstoneStateResult{
			Index: index, Phase: debugMoonPhaseName(index),
			X: moonstone.X, Y: moonstone.Y, Map: moonstone.Map, Level: moonstone.Level,
			State: debugMoonstoneState(moonstone.State), Found: moonstone.IsFound,
		})
		status.Visible = status.Visible || (night &&
			state.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType &&
			moonstone.CanCreateMoongate())
	}
	route, err := state.GetMoongateTravelRoute()
	if err != nil {
		status.UnavailableReason = "destination_moonstone_unavailable"
	} else {
		result := debugMoongateRoute(route)
		status.Route = &result
	}

	return debugSuccessResult(request, "Moongate status loaded.", status)
}

func moongateSourceFingerprint(state *game_state.GameState) string {
	location := state.MapState.PlayerLocation

	return fmt.Sprintf("%d:%d:%d:%d:%d:%t", location.Location, location.Position.X, location.Position.Y,
		location.Floor, state.DateTime.GetCurrentMoonPhase(), state.IsPlayerOnActiveMoongate())
}

func debugMoongateRoute(route game_state.MoongateTravelRoute) moongateRouteResult {
	result := moongateRouteResult{Fingerprint: moongateRouteFingerprint(route)}
	switch route.Kind {
	case game_state.MoongateTravelToMoonstone:
		result.Kind = "moonstone"
		result.Destination = &moongateDestinationResult{
			X: route.Destination.X, Y: route.Destination.Y,
			Map: route.Destination.Map, Level: route.Destination.Level,
			LocationID: int(route.Destination.Location), LocationName: route.Destination.Location.String(),
		}
	case game_state.MoongateTravelToSpirituality:
		result.Kind = "spirituality"
		shrineIndex := route.ShrineIndex
		result.ShrineIndex = &shrineIndex
		if shrineData := game_state.GetShrineData(shrineIndex); shrineData != nil {
			result.ShrineName = shrineData.VirtueName
		}
	default:
		panic("debugMoongateRoute: unknown route kind")
	}

	return result
}

func moongateRouteFingerprint(route game_state.MoongateTravelRoute) string {
	switch route.Kind {
	case game_state.MoongateTravelToMoonstone:
		destination := route.Destination

		return fmt.Sprintf("moonstone:%d:%d:%d:%d:%d", destination.Location, destination.X, destination.Y,
			destination.Map, destination.Level)
	case game_state.MoongateTravelToSpirituality:
		return fmt.Sprintf("spirituality:%d", route.ShrineIndex)
	default:
		panic("moongateRouteFingerprint: unknown route kind")
	}
}

func mustDebugIntegerArgument(request debugging.Request, argumentID string) int {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		panic(fmt.Sprintf("validated debug command %q missing argument %q", request.CommandID, argumentID))
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		panic(fmt.Sprintf("decoding validated debug command %q argument %q: %v", request.CommandID, argumentID, err))
	}

	return value
}

func mustDebugIntegerListArgument(request debugging.Request, argumentID string) []int {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		panic(fmt.Sprintf("validated debug command %q missing argument %q", request.CommandID, argumentID))
	}
	var values []int
	if err := json.Unmarshal(raw, &values); err != nil {
		panic(fmt.Sprintf("decoding validated debug command %q argument %q: %v", request.CommandID, argumentID, err))
	}

	return values
}

func mustDebugBooleanArgument(request debugging.Request, argumentID string) bool {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		panic(fmt.Sprintf("validated debug command %q missing argument %q", request.CommandID, argumentID))
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		panic(fmt.Sprintf("decoding validated debug command %q argument %q: %v", request.CommandID, argumentID, err))
	}

	return value
}

func loggingMutationResult(request debugging.Request, summary, path string, before, after any) debugging.Result {
	result := debugSuccessResultWithMode(request, debugging.ExecutionLiveSafe, summary, newLoggingStatusResult())
	appendSemanticChange(&result.Changes, path, before, after)

	return result
}

func appendSemanticChange(changes *[]debugging.StateChange, path string, before, after any) {
	if reflect.DeepEqual(before, after) {
		return
	}
	*changes = append(*changes, debugging.StateChange{
		Path: path, Before: mustDebugJSON(before), After: mustDebugJSON(after),
	})
}

func debugMapMaxima(mapType references.GeneralMapType) (int, int) {
	switch mapType {
	case references.LargeMapType:
		return int(references.XLargeMapTiles - 1), int(references.YLargeMapTiles - 1)
	case references.SmallMapType:
		return int(references.XSmallMapTiles - 1), int(references.YSmallMapTiles - 1)
	case references.DungeonMapType:
		return int(references.XDungeonMapTiles - 1), int(references.YDungeonMapTiles - 1)
	case references.CombatMapType, references.CampingMapType:
		return int(references.XCombatMapTiles - 1), int(references.YCombatMapTiles - 1)
	default:
		return -1, -1
	}
}

func debugMapTypeName(mapType references.GeneralMapType) string {
	switch mapType {
	case references.LargeMapType:
		return debugMapLarge
	case references.SmallMapType:
		return debugMapSmall
	case references.CombatMapType:
		return debugMapCombat
	case references.DungeonMapType:
		return debugMapDungeon
	case references.CampingMapType:
		return "camping"
	default:
		return debugMapUnknown
	}
}

func debugItemName(state *game_state.GameState, item references.Item) string {
	if state.GameReferences != nil && state.GameReferences.InventoryItemReferences != nil {
		return state.GameReferences.InventoryItemReferences.GetReferenceByItem(item).ItemName
	}
	if provision, ok := item.(references.Provision); ok {
		switch provision {
		case references.Gold:
			return "Gold"
		case references.Torches:
			return "Torches"
		case references.Key:
			return "Keys"
		case references.Food:
			return "Food"
		case references.NoProvision, references.Gem, references.SkullKeys:
			return fmt.Sprintf("Provision %d", provision)
		}
	}

	return fmt.Sprintf("Item %d:%d", item.Type(), item.ID())
}

func debugProvision(value string) (references.Provision, bool) {
	switch value {
	case debugProvisionGold:
		return references.Gold, true
	case debugProvisionTorches:
		return references.Torches, true
	case debugProvisionKeys:
		return references.Key, true
	case debugProvisionFood:
		return references.Food, true
	default:
		return references.NoProvision, false
	}
}

func debugProvisionQuantity(
	state *game_state.GameState,
	position references.Position,
	provision references.Provision,
) uint16 {
	stack, exists := state.ItemStacksMap.Snapshot(&position)
	if !exists {
		return 0
	}
	for _, entry := range stack.Items {
		if entry.Item != nil && entry.Item.Type() == provision.Type() && entry.Item.ID() == provision.ID() {
			return entry.Quantity
		}
	}

	return 0
}

func debugQuestItem(value string) (references.QuestItem, bool) {
	items := []references.QuestItem{
		references.Amulet, references.Crown, references.Sceptre, references.ShardOfFalsehood,
		references.ShardOfHatred, references.ShardOfCowardice, references.Carpet, references.Grapple,
		references.Spyglass, references.HMSCape, references.PocketWatch, references.BlackBadge,
		references.WoodenBox, references.Sextant,
	}
	for _, item := range items {
		key, _ := debugQuestItemIdentity(item)
		if key == value {
			return item, true
		}
	}

	return 0, false
}

func debugQuestItemIdentity(item references.QuestItem) (string, string) {
	identities := []struct {
		key      string
		category string
	}{
		{debugQuestAmulet, debugQuestCategoryArtifact}, {debugQuestCrown, debugQuestCategoryArtifact},
		{debugQuestSceptre, debugQuestCategoryArtifact},
		{"shard_falsehood", debugQuestCategoryShard}, {"shard_hatred", debugQuestCategoryShard},
		{"shard_cowardice", debugQuestCategoryShard},
		{debugQuestCarpet, debugQuestCategoryTool}, {debugQuestGrapple, debugQuestCategoryTool},
		{debugQuestSpyglass, debugQuestCategoryTool}, {"hms_cape", debugQuestCategoryTool},
		{"pocket_watch", debugQuestCategoryTool}, {"black_badge", debugQuestCategoryTool},
		{"wooden_box", debugQuestCategoryTool}, {debugQuestSextant, debugQuestCategoryTool},
	}
	if !item.IsValid() {
		return debugMapUnknown, debugMapUnknown
	}

	return identities[item].key, identities[item].category
}

func debugQuestItemName(state *game_state.GameState, item references.QuestItem) string {
	if state.GameReferences != nil && state.GameReferences.InventoryItemReferences != nil {
		return state.GameReferences.InventoryItemReferences.GetQuestItemDisplayName(item)
	}
	names := []string{
		"Amulet of Lord British", "Crown of Lord British", "Sceptre of Lord British", "Shard of Falsehood",
		"Shard of Hatred", "Shard of Cowardice", "Magic Carpet", "Grapple", "Spyglass", "H.M.S. Cape Plans",
		"Pocket Watch", "Black Badge", "Sandalwood Box", "Sextant",
	}

	return names[item]
}

func debugMoonPhaseName(index int) string {
	names := []string{
		"New Moon", "Crescent Waxing", "First Quarter", "Gibbous Waxing",
		"Full Moon", "Gibbous Waning", "Last Quarter", "Crescent Waning",
	}
	if index < 0 || index >= len(names) {
		return "Unknown Phase"
	}

	return names[index]
}

func debugMoonstoneState(state game_state.MoonstoneState) string {
	switch state {
	case game_state.MoonstoneStateBuried:
		return "buried"
	case game_state.MoonstoneStateFoundNotCollected:
		return "found_not_collected"
	case game_state.MoonstoneStateInInventory:
		return "inventory"
	default:
		return fmt.Sprintf("unknown_%d", state)
	}
}

func executeApplicationDebugCommand(
	request debugging.Request,
	configuration *config.UltimaVConfiguration,
) (debugging.Result, bool) {
	switch request.CommandID {
	case debugging.CommandLoggingStatus:
		return debugSuccessResult(request, "Logging status loaded.", newLoggingStatusResult()), true
	case debugging.CommandLoggingEnabledSet:
		logger := logging.GetGlobalLogger()
		before := logger.IsEnabled()
		after := mustDebugBooleanArgument(request, "enabled")
		if err := mutateAndPersistLoggingConfiguration(configuration, func() { logger.SetEnabled(after) }); err != nil {
			return debugging.FailureResult(request, "logging_persistence_failed", err.Error()), true
		}

		return loggingMutationResult(request, "Logging master state updated.", "logging.enabled", before, after), true
	case debugging.CommandLoggingStdoutSet:
		logger := logging.GetGlobalLogger()
		before := logger.IsStdoutEnabled()
		after := mustDebugBooleanArgument(request, "enabled")
		if err := mutateAndPersistLoggingConfiguration(configuration, func() { logger.SetStdoutEnabled(after) }); err != nil {
			return debugging.FailureResult(request, "logging_persistence_failed", err.Error()), true
		}

		return loggingMutationResult(request, "Logging stdout state updated.", "logging.stdoutEnabled", before, after), true
	case debugging.CommandLoggingLevelSet:
		logger := logging.GetGlobalLogger()
		before := logger.GetMinLevel().String()
		after := mustDebugStringArgument(request, "level")
		if err := mutateAndPersistLoggingConfiguration(configuration, func() {
			logger.SetMinLevel(logging.ParseLogLevel(after))
		}); err != nil {
			return debugging.FailureResult(request, "logging_persistence_failed", err.Error()), true
		}

		return loggingMutationResult(request, "Logging minimum level updated.", "logging.minimumLevel", before, after), true
	case debugging.CommandLoggingCategorySet:
		logger := logging.GetGlobalLogger()
		category := mustDebugStringArgument(request, "category")
		before := logger.GetCategoryStates()[category]
		after := mustDebugBooleanArgument(request, "enabled")
		if err := mutateAndPersistLoggingConfiguration(configuration, func() {
			logger.SetCategoryEnabled(category, after)
		}); err != nil {
			return debugging.FailureResult(request, "logging_persistence_failed", err.Error()), true
		}

		return loggingMutationResult(
			request, "Logging category updated: "+category+".", "logging.categories."+category, before, after,
		), true
	case debugging.CommandLoggingCategoriesSetAll:
		logger := logging.GetGlobalLogger()
		before := newLoggingStatusResult().CategoryStates
		after := mustDebugBooleanArgument(request, "enabled")
		if err := mutateAndPersistLoggingConfiguration(configuration, func() {
			states := logger.GetCategoryStates()
			for _, category := range logging.GetAllCategories() {
				states[category] = after
			}
			logger.SetCategoryStates(states)
		}); err != nil {
			return debugging.FailureResult(request, "logging_persistence_failed", err.Error()), true
		}
		result := debugSuccessResultWithMode(
			request, debugging.ExecutionLiveSafe, "All logging categories updated.", newLoggingStatusResult(),
		)
		for _, category := range logging.GetAllCategories() {
			appendSemanticChange(&result.Changes, "logging.categories."+category, before[category], after)
		}

		return result, true
	default:
		var empty debugging.Result

		return empty, false
	}
}

func debugWindDirection(value string) (environment.WindDirection, bool) {
	switch value {
	case debugging.WindDirectionCalm:
		return environment.Calm, true
	case debugging.WindDirectionNorth:
		return environment.North, true
	case debugging.WindDirectionSouth:
		return environment.South, true
	case debugging.WindDirectionEast:
		return environment.East, true
	case debugging.WindDirectionWest:
		return environment.West, true
	default:
		return environment.Calm, false
	}
}

func newShrineStatusResult(gameState debugWorkbenchState) shrineStatusResult {
	review := gameState.ReviewAllShrineQuestState()
	result := shrineStatusResult{
		Fingerprint:      review.Fingerprint,
		Shrines:          make([]shrineStatusEntry, 0, game_state.TotalShrines),
		FirstActiveIndex: -1,
		AllCompleted:     true,
	}
	for index, progress := range review.Shrines {
		result.Shrines = append(result.Shrines, newShrineStatusEntry(index, progress))
		if progress.Active && result.FirstActiveIndex < 0 {
			result.FirstActiveIndex = index
		}
		if !progress.Completed {
			result.AllCompleted = false
		}
	}

	return result
}

func newShrineStatusEntry(index int, progress game_state.ShrineProgressSnapshot) shrineStatusEntry {
	shrine := game_state.GetShrineData(index)
	if shrine == nil {
		panic(fmt.Sprintf("validated shrine index %d has no reference data", index))
	}

	return shrineStatusEntry{
		shrineReferenceResult: shrineReferenceResult{
			Index:       shrine.Index,
			VirtueName:  shrine.VirtueName,
			VirtueKey:   shrine.VirtueKey,
			Mantra:      shrine.Mantra,
			WordOfPower: shrine.WordOfPower,
			WorldX:      shrine.WorldX,
			WorldY:      shrine.WorldY,
			StrBonus:    shrine.StrBonus,
			DexBonus:    shrine.DexBonus,
			IntBonus:    shrine.IntBonus,
			KarmaBonus:  shrine.KarmaBonus(),
		},
		QuestState: string(progress.QuestState),
		Completed:  progress.Completed,
		Active:     progress.Active,
		Ruined:     progress.Ruined,
	}
}

func shrineProgressAlterationFromRequest(request debugging.Request) (game_state.ShrineProgressAlteration, error) {
	indexValue := mustDebugStringArgument(request, "index")
	index, err := strconv.Atoi(indexValue)
	if err != nil {
		return game_state.ShrineProgressAlteration{}, fmt.Errorf("invalid shrine index %q: %w", indexValue, err)
	}
	questValue, questSet, err := optionalDebugStringArgument(request, "questState")
	if err != nil {
		return game_state.ShrineProgressAlteration{}, err
	}
	ruinedValue, ruinedSet, err := optionalDebugBooleanArgument(request, "ruined")
	if err != nil {
		return game_state.ShrineProgressAlteration{}, err
	}
	var questState *game_state.ShrineProgressState
	if questSet {
		state := game_state.ShrineProgressState(questValue)
		questState = &state
	}
	var ruined *bool
	if ruinedSet {
		ruined = &ruinedValue
	}

	return game_state.ShrineProgressAlteration{Index: index, QuestState: questState, Ruined: ruined}, nil
}

func optionalDebugBooleanArgument(request debugging.Request, argumentID string) (bool, bool, error) {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		return false, false, nil
	}
	var value bool
	if err := json.Unmarshal(raw, &value); err != nil {
		return false, false, fmt.Errorf("decoding argument %q: %w", argumentID, err)
	}

	return value, true, nil
}

func shrineProgressChanges(
	virtueKey string,
	result game_state.ShrineProgressAlterationResult,
) []debugging.StateChange {
	changes := make([]debugging.StateChange, 0, shrineChangeCapacity)
	appendChange := func(field string, before, after any) {
		if reflect.DeepEqual(before, after) {
			return
		}
		changes = append(changes, debugging.StateChange{
			Path:   "shrines." + virtueKey + "." + field,
			Before: mustDebugJSON(before),
			After:  mustDebugJSON(after),
		})
	}
	appendChange("questState", result.Before.QuestState, result.After.QuestState)
	appendChange("completed", result.Before.Completed, result.After.Completed)
	appendChange("active", result.Before.Active, result.After.Active)
	appendChange("ruined", result.Before.Ruined, result.After.Ruined)

	return changes
}

func mustDebugStringArgument(request debugging.Request, argumentID string) string {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		panic(fmt.Sprintf("validated debug command %q missing argument %q", request.CommandID, argumentID))
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		panic(fmt.Sprintf("decoding validated debug command %q argument %q: %v", request.CommandID, argumentID, err))
	}

	return value
}

func optionalDebugIntegerArgument(request debugging.Request, argumentID string) (int, bool, error) {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		return 0, false, nil
	}
	var value int
	if err := json.Unmarshal(raw, &value); err != nil {
		return 0, false, fmt.Errorf("decoding argument %q: %w", argumentID, err)
	}

	return value, true, nil
}

func optionalDebugStringArgument(request debugging.Request, argumentID string) (string, bool, error) {
	raw, ok := request.Arguments[argumentID]
	if !ok {
		return "", false, nil
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", false, fmt.Errorf("decoding argument %q: %w", argumentID, err)
	}

	return value, true, nil
}

func characterAlterationFromRequest(
	request debugging.Request,
	slot int,
	occupant string,
) (game_state.ActiveCharacterAlteration, error) {
	strengthValue, strengthSet, err := optionalDebugIntegerArgument(request, "strength")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	dexterityValue, dexteritySet, err := optionalDebugIntegerArgument(request, "dexterity")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	intelligenceValue, intelligenceSet, err := optionalDebugIntegerArgument(request, "intelligence")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	currentHPValue, currentHPSet, err := optionalDebugIntegerArgument(request, "currentHp")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	currentMPValue, currentMPSet, err := optionalDebugIntegerArgument(request, "currentMp")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	experienceValue, experienceSet, err := optionalDebugIntegerArgument(request, "experience")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	levelValue, levelSet, err := optionalDebugIntegerArgument(request, "level")
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	statusValue, statusSet, err := optionalDebugCharacterStatus(request)
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}
	destinationValue, destinationSet, err := optionalDebugPartyPosition(request)
	if err != nil {
		return game_state.ActiveCharacterAlteration{}, err
	}

	strength := optionalIntPointer(strengthValue, strengthSet)
	dexterity := optionalIntPointer(dexterityValue, dexteritySet)
	intelligence := optionalIntPointer(intelligenceValue, intelligenceSet)
	currentHP := optionalIntPointer(currentHPValue, currentHPSet)
	currentMP := optionalIntPointer(currentMPValue, currentMPSet)
	experience := optionalIntPointer(experienceValue, experienceSet)
	level := optionalIntPointer(levelValue, levelSet)
	status := optionalStatusPointer(statusValue, statusSet)
	destination := optionalIntPointer(destinationValue, destinationSet)

	return game_state.ActiveCharacterAlteration{
		Slot:               slot,
		ExpectedOccupant:   occupant,
		Strength:           strength,
		Dexterity:          dexterity,
		Intelligence:       intelligence,
		CurrentHP:          currentHP,
		CurrentMP:          currentMP,
		Experience:         experience,
		Level:              level,
		Status:             status,
		ReorderDestination: destination,
	}, nil
}

func optionalDebugCharacterStatus(request debugging.Request) (party_state.CharacterStatus, bool, error) {
	value, supplied, err := optionalDebugStringArgument(request, "status")
	if err != nil || !supplied {
		return 0, false, err
	}
	status, found := debugCharacterStatus(value)
	if !found {
		return 0, false, fmt.Errorf(
			"%w: unknown safe character status %q",
			errInvalidDebugAlterationArgument,
			value,
		)
	}

	return status, true, nil
}

func optionalDebugPartyPosition(request debugging.Request) (int, bool, error) {
	value, supplied, err := optionalDebugStringArgument(request, "partyPosition")
	if err != nil || !supplied {
		return 0, false, err
	}
	position, err := strconv.Atoi(value)
	if err != nil {
		return 0, false, fmt.Errorf(
			"%w: invalid party position %q",
			errInvalidDebugAlterationArgument,
			value,
		)
	}
	position--

	return position, true, nil
}

func optionalIntPointer(value int, supplied bool) *int {
	if !supplied {
		return nil
	}

	return &value
}

func optionalStatusPointer(value party_state.CharacterStatus, supplied bool) *party_state.CharacterStatus {
	if !supplied {
		return nil
	}

	return &value
}

func debugCharacterStatus(value string) (party_state.CharacterStatus, bool) {
	switch value {
	case "good":
		return party_state.Good, true
	case "poisoned":
		return party_state.Poisoned, true
	case debugStatusSleep:
		return party_state.Sleep, true
	case "charmed":
		return party_state.Charmed, true
	default:
		return 0, false
	}
}

func debugRosterStatus(value string) (party_state.CharacterStatus, bool) {
	if value == debugStatusDead {
		return party_state.Dead, true
	}

	return debugCharacterStatus(value)
}

func activeCharacterToken(slot int, name string) string {
	return strconv.Itoa(slot) + "." + base64.RawURLEncoding.EncodeToString([]byte(name))
}

func parseActiveCharacterToken(token string) (int, string, error) {
	parts := strings.SplitN(token, ".", activeCharacterTokenPartCount)
	if len(parts) != activeCharacterTokenPartCount {
		return 0, "", errInvalidDebugCharacterSelection
	}
	slot, err := strconv.Atoi(parts[0])
	if err != nil || slot < 0 || slot >= party_state.MaxActiveParty {
		return 0, "", fmt.Errorf("%w: invalid active-character slot", errInvalidDebugCharacterSelection)
	}
	name, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || len(name) == 0 {
		return 0, "", fmt.Errorf("%w: invalid active-character identity", errInvalidDebugCharacterSelection)
	}

	return slot, string(name), nil
}

func newActiveCharacterResult(character game_state.ActiveCharacterSnapshot) activeCharacterResult {
	return activeCharacterResult{
		RosterIndex:   character.Slot,
		PartyPosition: character.Slot + 1,
		Name:          character.Name,
		Class:         character.Class.String(),
		Status:        characterStatusName(character.Status),
		Strength:      character.Strength,
		Dexterity:     character.Dexterity,
		Intelligence:  character.Intelligence,
		CurrentMP:     character.CurrentMP,
		CurrentHP:     character.CurrentHP,
		MaximumHP:     character.MaximumHP,
		Experience:    character.Experience,
		Level:         character.Level,
	}
}

func characterStatusName(status party_state.CharacterStatus) string {
	name, ok := status.FriendlyName()
	if !ok {
		return "Unrecognized status"
	}

	return name
}

func characterAlterationChanges(result game_state.ActiveCharacterAlterationResult) []debugging.StateChange {
	before := result.CharacterBefore
	after := result.CharacterAfter
	changes := make([]debugging.StateChange, 0, characterChangeCapacity)
	appendChange := func(path string, previous, current any) {
		if reflect.DeepEqual(previous, current) {
			return
		}
		changes = append(changes, debugging.StateChange{
			Path:   path,
			Before: mustDebugJSON(previous),
			After:  mustDebugJSON(current),
		})
	}
	prefix := "party.character." + before.Name
	appendChange(prefix+".strength", before.Strength, after.Strength)
	appendChange(prefix+".dexterity", before.Dexterity, after.Dexterity)
	appendChange(prefix+".intelligence", before.Intelligence, after.Intelligence)
	appendChange(prefix+".currentMp", before.CurrentMP, after.CurrentMP)
	appendChange(prefix+".currentHp", before.CurrentHP, after.CurrentHP)
	appendChange(prefix+".maximumHp", before.MaximumHP, after.MaximumHP)
	appendChange(prefix+".experience", before.Experience, after.Experience)
	appendChange(prefix+".level", before.Level, after.Level)
	appendChange(prefix+".status", characterStatusName(before.Status), characterStatusName(after.Status))
	appendChange(prefix+".partyPosition", before.Slot+1, after.Slot+1)
	if before.Slot != after.Slot {
		appendChange("party.order", newPartyOrderResult(result.OrderBefore), newPartyOrderResult(result.OrderAfter))
	}

	return changes
}

func newPartyOrderResult(order []game_state.ActivePartySlotSnapshot) []partyOrderSlotResult {
	result := make([]partyOrderSlotResult, 0, len(order))
	for _, slot := range order {
		result = append(result, partyOrderSlotResult{PartyPosition: slot.Slot + 1, Name: slot.Name})
	}

	return result
}

func newPartyRosterResult(snapshot game_state.PartyRosterSnapshot) partyRosterResult {
	result := partyRosterResult{
		Fingerprint: snapshot.Fingerprint,
		Characters:  make([]partyRosterCharacterResult, 0, len(snapshot.Characters)),
	}
	for _, character := range snapshot.Characters {
		result.Characters = append(result.Characters, partyRosterCharacterResult{
			RosterIndex: character.RosterIndex,
			Name:        character.Name, Gender: byte(character.Gender), Class: character.Class.String(),
			Status: rosterStatusName(character.Status),
			Active: character.PartyMap == party_state.PartyMapActive, PartyMap: byte(character.PartyMap),
			Strength: character.Strength, Dexterity: character.Dexterity, Intelligence: character.Intelligence,
			CurrentMP: character.CurrentMP, CurrentHP: character.CurrentHP, MaximumHP: character.MaximumHP,
			Experience: character.Experience, Level: character.Level, MonthsAtInn: character.MonthsAtInn,
			ArmorClass: character.ArmorClass,
			Helmet:     character.Helmet, Armor: character.Armor, Weapon: character.Weapon,
			Shield: character.Shield, Ring: character.Ring, Amulet: character.Amulet,
		})
	}

	return result
}

func rosterStatusName(status party_state.CharacterStatus) string {
	switch status {
	case party_state.Good:
		return "good"
	case party_state.Poisoned:
		return "poisoned"
	case party_state.Sleep:
		return "sleep"
	case party_state.Charmed:
		return "charmed"
	case party_state.Dead:
		return debugStatusDead
	default:
		return "unknown"
	}
}

func partyRosterMutationDebugResult(
	request debugging.Request,
	summary string,
	mutation game_state.PartyRosterMutationResult,
	err error,
) debugging.Result {
	if err != nil {
		code := "invalid_party_mutation"
		switch {
		case errors.Is(err, game_state.ErrStalePartyRoster):
			code = "stale_party_roster"
		case errors.Is(err, game_state.ErrPartyMutationInCombat):
			code = "party_mutation_in_combat"
		case errors.Is(err, game_state.ErrPartyCapacity):
			code = "party_capacity"
		}

		return debugging.FailureResult(request, code, err.Error())
	}
	mode := debugging.ExecutionAlteration
	if request.CommandID == debugging.CommandPartyStatusSet {
		mode = debugging.ExecutionSynchronized
	}
	before := newPartyRosterResult(mutation.Before)
	after := newPartyRosterResult(mutation.After)
	result := debugSuccessResultWithMode(request, mode, summary, partyRosterMutationResult{Before: before, After: after})
	result.Changes = partyRosterChanges(before, after)

	return result
}

func partyRosterChanges(before, after partyRosterResult) []debugging.StateChange {
	changes := make([]debugging.StateChange, 0)
	for index := range min(len(before.Characters), len(after.Characters)) {
		previous, current := before.Characters[index], after.Characters[index]
		prefix := fmt.Sprintf("party.roster.%d", previous.RosterIndex)
		appendSemanticChange(&changes, prefix+".name", previous.Name, current.Name)
		appendSemanticChange(&changes, prefix+".gender", previous.Gender, current.Gender)
		appendSemanticChange(&changes, prefix+".class", previous.Class, current.Class)
		appendSemanticChange(&changes, prefix+".active", previous.Active, current.Active)
		appendSemanticChange(&changes, prefix+".partyMap", previous.PartyMap, current.PartyMap)
		appendSemanticChange(&changes, prefix+".status", previous.Status, current.Status)
		appendSemanticChange(&changes, prefix+".strength", previous.Strength, current.Strength)
		appendSemanticChange(&changes, prefix+".dexterity", previous.Dexterity, current.Dexterity)
		appendSemanticChange(&changes, prefix+".intelligence", previous.Intelligence, current.Intelligence)
		appendSemanticChange(&changes, prefix+".currentMp", previous.CurrentMP, current.CurrentMP)
		appendSemanticChange(&changes, prefix+".currentHp", previous.CurrentHP, current.CurrentHP)
		appendSemanticChange(&changes, prefix+".maximumHp", previous.MaximumHP, current.MaximumHP)
		appendSemanticChange(&changes, prefix+".experience", previous.Experience, current.Experience)
		appendSemanticChange(&changes, prefix+".level", previous.Level, current.Level)
		appendSemanticChange(&changes, prefix+".monthsAtInn", previous.MonthsAtInn, current.MonthsAtInn)
		appendSemanticChange(&changes, prefix+".armorClass", previous.ArmorClass, current.ArmorClass)
		appendSemanticChange(&changes, prefix+".helmet", previous.Helmet, current.Helmet)
		appendSemanticChange(&changes, prefix+".armor", previous.Armor, current.Armor)
		appendSemanticChange(&changes, prefix+".weapon", previous.Weapon, current.Weapon)
		appendSemanticChange(&changes, prefix+".shield", previous.Shield, current.Shield)
		appendSemanticChange(&changes, prefix+".ring", previous.Ring, current.Ring)
		appendSemanticChange(&changes, prefix+".amulet", previous.Amulet, current.Amulet)
	}

	return changes
}

func mustDebugJSON(value any) json.RawMessage {
	raw, err := json.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("marshalling debug state change: %v", err))
	}

	return raw
}

func debugSuccessResult(request debugging.Request, summary string, data any) debugging.Result {
	return debugSuccessResultWithMode(request, debugging.ExecutionQuery, summary, data)
}

func debugSuccessResultWithMode(
	request debugging.Request,
	mode debugging.ExecutionMode,
	summary string,
	data any,
) debugging.Result {
	raw, err := json.Marshal(data)
	if err != nil {
		panic(fmt.Sprintf("marshalling debug command %q result: %v", request.CommandID, err))
	}

	return debugging.SuccessResult(request, mode, summary, raw)
}
