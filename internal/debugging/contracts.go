// Package debugging defines transport-neutral contracts for the external debug workbench.
package debugging

import "encoding/json"

// ParameterKind identifies the value and control type for a command parameter.
type ParameterKind string

// Supported parameter kinds for generated workbench controls.
const (
	ParameterString      ParameterKind = "string"
	ParameterInteger     ParameterKind = "integer"
	ParameterIntegerList ParameterKind = "integer_list"
	ParameterBoolean     ParameterKind = "boolean"
	ParameterEnum        ParameterKind = "enum"
	ParameterCharacter   ParameterKind = "character"
	ParameterLocation    ParameterKind = "location"
	ParameterMonster     ParameterKind = "monster"
	ParameterEquipment   ParameterKind = "equipment"
)

// ExecutionMode describes how a command interacts with live game state.
type ExecutionMode string

// Supported command execution modes.
const (
	ExecutionQuery        ExecutionMode = "query"
	ExecutionLiveSafe     ExecutionMode = "live_safe"
	ExecutionSynchronized ExecutionMode = "synchronized_live"
	ExecutionWorkflow     ExecutionMode = "workflow"
	ExecutionAlteration   ExecutionMode = "coordinated_alteration"
	ExecutionSceneRestart ExecutionMode = "scene_restart"
	ExecutionPresentation ExecutionMode = "presentation"
)

// Choice defines one serialized value and display label for an enum parameter.
type Choice struct {
	Value    string          `json:"value"`
	Label    string          `json:"label"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}

// OptionSet contains detached choices resolved for one dynamic parameter provider.
type OptionSet struct {
	Provider          string   `json:"provider"`
	Choices           []Choice `json:"choices"`
	UnavailableReason string   `json:"unavailableReason,omitempty"`
}

// DiscoverySnapshot atomically describes commands and their update-thread-resolved options.
type DiscoverySnapshot struct {
	SceneGeneration uint64              `json:"sceneGeneration"`
	OptionsRevision uint64              `json:"optionsRevision"`
	Commands        []CommandDefinition `json:"commands"`
	OptionSets      []OptionSet         `json:"optionSets,omitempty"`
}

// ParameterDefinition describes one typed command argument.
type ParameterDefinition struct {
	ID              string          `json:"id"`
	Name            string          `json:"name"`
	Description     string          `json:"description,omitempty"`
	Kind            ParameterKind   `json:"kind"`
	Required        bool            `json:"required"`
	Default         json.RawMessage `json:"default,omitempty"`
	Minimum         *int            `json:"minimum,omitempty"`
	Maximum         *int            `json:"maximum,omitempty"`
	Step            *int            `json:"step,omitempty"`
	Pattern         string          `json:"pattern,omitempty"`
	Choices         []Choice        `json:"choices,omitempty"`
	OptionsProvider string          `json:"optionsProvider,omitempty"`
	ControlHint     string          `json:"controlHint,omitempty"`
	Availability    []string        `json:"availability,omitempty"`
}

// CommandDefinition is the authoritative discovery schema for a logical command.
type CommandDefinition struct {
	ID             string                `json:"id"`
	Category       string                `json:"category"`
	Name           string                `json:"name"`
	Description    string                `json:"description"`
	Aliases        []string              `json:"aliases,omitempty"`
	SearchTerms    []string              `json:"searchTerms,omitempty"`
	Parameters     []ParameterDefinition `json:"parameters,omitempty"`
	ExecutionModes []ExecutionMode       `json:"executionModes"`
	Destructive    bool                  `json:"destructive"`
}

// Request is an immutable command invocation submitted to the update thread.
type Request struct {
	SessionID               string                     `json:"sessionId"`
	ID                      string                     `json:"id"`
	CommandID               string                     `json:"commandId"`
	ExpectedSceneGeneration uint64                     `json:"expectedSceneGeneration,omitempty"`
	ExpectedOptionsRevision uint64                     `json:"expectedOptionsRevision,omitempty"`
	Arguments               map[string]json.RawMessage `json:"arguments,omitempty"`
}

// CloneDiscoverySnapshot returns a detached copy safe for publication across goroutines.
func CloneDiscoverySnapshot(snapshot DiscoverySnapshot) DiscoverySnapshot {
	clone := DiscoverySnapshot{
		SceneGeneration: snapshot.SceneGeneration,
		OptionsRevision: snapshot.OptionsRevision,
		Commands:        make([]CommandDefinition, len(snapshot.Commands)),
		OptionSets:      CloneOptionSets(snapshot.OptionSets),
	}
	for index, definition := range snapshot.Commands {
		clone.Commands[index] = cloneDefinition(definition)
	}

	return clone
}

// CloneOptionSets detaches dynamic choices without copying immutable commands.
func CloneOptionSets(optionSets []OptionSet) []OptionSet {
	clone := make([]OptionSet, len(optionSets))
	for index, optionSet := range optionSets {
		clone[index] = OptionSet{
			Provider:          optionSet.Provider,
			Choices:           cloneChoices(optionSet.Choices),
			UnavailableReason: optionSet.UnavailableReason,
		}
	}

	return clone
}

// StateChange describes one semantic before/after value produced by a command.
type StateChange struct {
	Path   string          `json:"path"`
	Before json.RawMessage `json:"before,omitempty"`
	After  json.RawMessage `json:"after,omitempty"`
}

// CommandInvocation identifies the normalized command and typed arguments that were executed.
type CommandInvocation struct {
	CommandID string                     `json:"commandId"`
	Arguments map[string]json.RawMessage `json:"arguments"`
}

// Result is the structured outcome of a command invocation.
type Result struct {
	SessionID              string             `json:"sessionId"`
	RequestID              string             `json:"requestId"`
	CommandID              string             `json:"commandId"`
	SceneGeneration        uint64             `json:"sceneGeneration"`
	Invocation             *CommandInvocation `json:"invocation,omitempty"`
	EffectiveExecutionMode ExecutionMode      `json:"effectiveExecutionMode,omitempty"`
	Success                bool               `json:"success"`
	Summary                string             `json:"summary,omitempty"`
	ErrorCode              string             `json:"errorCode,omitempty"`
	ErrorMessage           string             `json:"errorMessage,omitempty"`
	Changes                []StateChange      `json:"changes,omitempty"`
	Data                   json.RawMessage    `json:"data,omitempty"`
}

// WorldPositionBoundsData describes one destination's valid coordinate space.
type WorldPositionBoundsData struct {
	MinimumX int `json:"minimumX"`
	MaximumX int `json:"maximumX"`
	MinimumY int `json:"minimumY"`
	MaximumY int `json:"maximumY"`
}

// WorldPositionSetupData is the destination's useful initial Workbench draft.
type WorldPositionSetupData struct {
	Floor int `json:"floor"`
	X     int `json:"x"`
	Y     int `json:"y"`
}

// WorldPositionPlacementTargetData describes one server-resolved dungeon route target.
type WorldPositionPlacementTargetData struct {
	Placement          string                  `json:"placement"`
	PlacementLabel     string                  `json:"placementLabel"`
	Route              string                  `json:"route"`
	RouteLabel         string                  `json:"routeLabel"`
	Available          bool                    `json:"available"`
	UnavailableReason  string                  `json:"unavailableReason,omitempty"`
	LocationID         int                     `json:"locationId"`
	Setup              *WorldPositionSetupData `json:"setup,omitempty"`
	FacingLabel        string                  `json:"facingLabel,omitempty"`
	LastDirectionLabel string                  `json:"lastDirectionLabel,omitempty"`
}

// WorldPositionDungeonRoomData describes one fixed dungeon room and the cell the party stands on to enter it.
type WorldPositionDungeonRoomData struct {
	Room              int                     `json:"room"`
	Label             string                  `json:"label"`
	Floor             int                     `json:"floor"`
	RoomX             int                     `json:"roomX"`
	RoomY             int                     `json:"roomY"`
	Available         bool                    `json:"available"`
	UnavailableReason string                  `json:"unavailableReason,omitempty"`
	Setup             *WorldPositionSetupData `json:"setup,omitempty"`
	FacingLabel       string                  `json:"facingLabel,omitempty"`
	Summary           string                  `json:"summary,omitempty"`
}

// WorldPositionDestinationData describes one complete World Navigator destination.
type WorldPositionDestinationData struct {
	LocationID       int                                `json:"locationId"`
	LocationName     string                             `json:"locationName"`
	MapType          string                             `json:"mapType"`
	Floors           []int                              `json:"floors"`
	Bounds           WorldPositionBoundsData            `json:"bounds"`
	DefaultSetup     WorldPositionSetupData             `json:"defaultSetup"`
	WorldEntrance    *WorldPositionSetupData            `json:"worldEntrance,omitempty"`
	PlacementTargets []WorldPositionPlacementTargetData `json:"placementTargets,omitempty"`
	Rooms            []WorldPositionDungeonRoomData     `json:"rooms,omitempty"`
}

// WorldPositionStatusData is the typed World Navigator status payload.
type WorldPositionStatusData struct {
	SourceFingerprint string                         `json:"sourceFingerprint"`
	LocationID        int                            `json:"locationId"`
	LocationName      string                         `json:"locationName"`
	MapType           string                         `json:"mapType"`
	Floor             int                            `json:"floor"`
	X                 int                            `json:"x"`
	Y                 int                            `json:"y"`
	Bounds            WorldPositionBoundsData        `json:"bounds"`
	Destinations      []WorldPositionDestinationData `json:"destinations"`
}

// MerchantTesterContextData is the reviewed launch context for Merchant Tester operations.
type MerchantTesterContextData struct {
	LocationID   int    `json:"locationId"`
	LocationName string `json:"locationName"`
	Floor        int    `json:"floor"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
}

// MerchantTesterMerchantData describes one canonical active merchant and its current launch availability.
type MerchantTesterMerchantData struct {
	MerchantID               string        `json:"merchantId"`
	TownID                   int           `json:"townId"`
	TownName                 string        `json:"townName"`
	ShopName                 string        `json:"shopName"`
	OwnerName                string        `json:"ownerName"`
	FamilyID                 string        `json:"familyId"`
	FamilyLabel              string        `json:"familyLabel"`
	DialogCode               int           `json:"dialogCode"`
	Implemented              bool          `json:"implemented"`
	UnavailableReason        string        `json:"unavailableReason,omitempty"`
	PreviewAvailable         bool          `json:"previewAvailable"`
	PreviewUnavailableReason string        `json:"previewUnavailableReason,omitempty"`
	LiveAvailable            bool          `json:"liveAvailable"`
	LiveUnavailableReason    string        `json:"liveUnavailableReason,omitempty"`
	LiveExecutionMode        ExecutionMode `json:"liveExecutionMode"`
}

// MerchantTesterCatalogData is the authoritative Merchant Tester catalog and reviewed source state.
type MerchantTesterCatalogData struct {
	SourceFingerprint string                       `json:"sourceFingerprint"`
	CurrentContext    MerchantTesterContextData    `json:"currentContext"`
	Merchants         []MerchantTesterMerchantData `json:"merchants"`
}

// MerchantTesterOpenData reports presentation/setup queued by an open command.
type MerchantTesterOpenData struct {
	MerchantID     string `json:"merchantId"`
	TownID         int    `json:"townId"`
	TownName       string `json:"townName"`
	Preview        bool   `json:"preview"`
	Transitioned   bool   `json:"transitioned"`
	WorkflowQueued bool   `json:"workflowQueued"`
	LocationID     int    `json:"locationId"`
	Floor          int    `json:"floor"`
	X              int    `json:"x"`
	Y              int    `json:"y"`
}

// SaveSnapshotCompatibility reports whether this process can hydrate a capture.
type SaveSnapshotCompatibility struct {
	Compatible bool   `json:"compatible"`
	Reason     string `json:"reason,omitempty"`
}

// SaveSnapshotEligibility reports operations allowed for a detached repository record.
type SaveSnapshotEligibility struct {
	CanCapture bool   `json:"canCapture"`
	CanLoad    bool   `json:"canLoad"`
	CanDelete  bool   `json:"canDelete"`
	Reason     string `json:"reason,omitempty"`
}

// SaveSnapshotSummary is detached repository metadata and never contains gameplay state.
type SaveSnapshotSummary struct {
	Reference     string                    `json:"reference"`
	Source        string                    `json:"source"`
	ID            string                    `json:"id"`
	Name          string                    `json:"name"`
	Description   string                    `json:"description,omitempty"`
	CreatedAt     string                    `json:"createdAt"`
	GameVersion   string                    `json:"gameVersion"`
	Revision      string                    `json:"revision"`
	Compatibility SaveSnapshotCompatibility `json:"compatibility"`
	Eligibility   SaveSnapshotEligibility   `json:"eligibility"`
}

// SaveSnapshotDetail adds a compact review projection without exposing the full snapshot.
type SaveSnapshotDetail struct {
	Summary         SaveSnapshotSummary `json:"summary"`
	SnapshotVersion int                 `json:"snapshotVersion"`
	Location        int                 `json:"location"`
	LocationName    string              `json:"locationName"`
	X               int                 `json:"x"`
	Y               int                 `json:"y"`
	Floor           int                 `json:"floor"`
	Year            int                 `json:"year"`
	Month           int                 `json:"month"`
	Day             int                 `json:"day"`
	Hour            int                 `json:"hour"`
	Minute          int                 `json:"minute"`
	PartyMembers    int                 `json:"partyMembers"`
	MoonPhase       string              `json:"moonPhase"`
	MoonPhaseIndex  int                 `json:"moonPhaseIndex"`
}

// SaveSnapshotListData is the typed list-command payload.
type SaveSnapshotListData struct {
	Snapshots          []SaveSnapshotSummary   `json:"snapshots"`
	CaptureEligibility SaveSnapshotEligibility `json:"captureEligibility"`
}

// SaveSnapshotGetData is the typed detail-command payload.
type SaveSnapshotGetData struct {
	Snapshot SaveSnapshotDetail `json:"snapshot"`
}

// SaveSnapshotCaptureData identifies the immutable capture that was created.
type SaveSnapshotCaptureData struct {
	Snapshot SaveSnapshotSummary `json:"snapshot"`
}

// SaveSnapshotLoadData identifies the loaded capture, rollback, and replacement generation.
type SaveSnapshotLoadData struct {
	Reference         string `json:"reference"`
	Revision          string `json:"revision"`
	RollbackReference string `json:"rollbackReference"`
	RollbackRevision  string `json:"rollbackRevision"`
	SceneGeneration   uint64 `json:"sceneGeneration"`
}

// SaveSnapshotDeleteData identifies the exact revision removed from the repository.
type SaveSnapshotDeleteData struct {
	Reference string `json:"reference"`
	Revision  string `json:"revision"`
}

// SpellInventoryStatusData is the typed prepared-spell instrument payload.
type SpellInventoryStatusData struct {
	SnapshotFingerprint string                    `json:"snapshotFingerprint"`
	Spells              []SpellInventoryEntryData `json:"spells"`
	MagicLight          MagicLightEffectData      `json:"magicLight"`
	ActiveSpell         ActiveSpellEffectData     `json:"activeSpell"`
}

// SpellInventoryEntryData describes one prepared spell quantity.
type SpellInventoryEntryData struct {
	ID          int    `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Circle      int    `json:"circle"`
	Targeting   string `json:"targeting"`
	Context     string `json:"context"`
	Quantity    uint16 `json:"quantity"`
	Maximum     uint16 `json:"maximum"`
}

// MagicLightEffectData reports independent magical illumination.
type MagicLightEffectData struct {
	Active           bool `json:"active"`
	MinutesRemaining int  `json:"minutesRemaining"`
}

// ActiveSpellEffectData reports the single active duration-spell slot.
type ActiveSpellEffectData struct {
	Active          bool     `json:"active"`
	EffectType      string   `json:"effectType"`
	FriendlyName    string   `json:"friendlyName"`
	Incantations    []string `json:"incantations"`
	SourceAmbiguous bool     `json:"sourceAmbiguous"`
	Description     string   `json:"description"`
	TurnsRemaining  int      `json:"turnsRemaining"`
}

// SpellEffectsData reports both independent persistent magical effects.
type SpellEffectsData struct {
	MagicLight  MagicLightEffectData  `json:"magicLight"`
	ActiveSpell ActiveSpellEffectData `json:"activeSpell"`
}

// NewNormalizedInvocation snapshots a catalog-normalized request without transport ownership fields.
func NewNormalizedInvocation(request Request) *CommandInvocation {
	arguments := cloneArguments(request.Arguments)
	if arguments == nil {
		arguments = make(map[string]json.RawMessage)
	}

	return &CommandInvocation{
		CommandID: request.CommandID,
		Arguments: arguments,
	}
}

// Executor executes a validated command on the Ebitengine update thread.
type Executor interface {
	ExecuteDebugCommand(request Request) Result
}

// SuccessResult creates a successful command result.
func SuccessResult(request Request, mode ExecutionMode, summary string, data json.RawMessage) Result {
	return Result{
		SessionID:              request.SessionID,
		RequestID:              request.ID,
		CommandID:              request.CommandID,
		SceneGeneration:        0,
		Invocation:             nil,
		EffectiveExecutionMode: mode,
		Success:                true,
		Summary:                summary,
		ErrorCode:              "",
		ErrorMessage:           "",
		Changes:                nil,
		Data:                   append(json.RawMessage(nil), data...),
	}
}

// FailureResult creates a failed command result.
func FailureResult(request Request, code, message string) Result {
	return Result{
		SessionID:              request.SessionID,
		RequestID:              request.ID,
		CommandID:              request.CommandID,
		SceneGeneration:        0,
		Invocation:             nil,
		EffectiveExecutionMode: "",
		Success:                false,
		Summary:                "",
		ErrorCode:              code,
		ErrorMessage:           message,
		Changes:                nil,
		Data:                   nil,
	}
}
