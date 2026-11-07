package save_snapshot

import "github.com/bradhannah/UltimaVRedux/internal/journal"

// saveSnapshotV25 freezes the last native format without player notes.
type saveSnapshotV25 struct {
	Version                 int                                 `json:"version"`
	Party                   PartySnapshot                       `json:"party"`
	Inventory               InventorySnapshot                   `json:"inventory"`
	World                   WorldSnapshot                       `json:"world"`
	Progression             ProgressionSnapshot                 `json:"progression"`
	Effects                 EffectsSnapshot                     `json:"effects"`
	Wind                    WindSnapshot                        `json:"wind"`
	Random                  RandomSnapshot                      `json:"random"`
	Continuation            ContinuationSnapshot                `json:"continuation"`
	Redux                   ReduxSnapshot                       `json:"redux"`
	Presentation            journal.Presentation                `json:"presentation"`
	LastConversation        journal.Conversation                `json:"lastConversation"`
	Excerpts                journal.Excerpts                    `json:"excerpts"`
	DungeonJournal          [8]journal.DungeonObservation       `json:"dungeonJournal"`
	MoonstoneJournal        [8]journal.MoonstoneObservation     `json:"moonstoneJournal"`
	AtlasLandmarks          []journal.AtlasLandmarkObservation  `json:"atlasLandmarks"`
	Dungeon                 *DungeonSnapshot                    `json:"dungeon"`
	PartyVehicle            *VehicleSnapshot                    `json:"partyVehicle"`
	MapUnits                []MapUnitSnapshot                   `json:"mapUnits"`
	ItemStacks              []ItemStackSnapshot                 `json:"itemStacks"`
	MapMutations            []MapMutationSnapshot               `json:"mapMutations"`
	OpenDoor                *OpenDoorSnapshot                   `json:"openDoor"`
	BasementChestHistory    [3]BasementChestDailyRecordSnapshot `json:"basementChestHistory"`
	AtlasNotes              journal.AtlasNotesState             `json:"atlasNotes"`
	AtlasFleet              journal.AtlasFleetState             `json:"atlasFleet"`
	CodexRevelationComplete bool                                `json:"codexRevelationComplete"`
}

func (previous saveSnapshotV25) validationView() SaveSnapshot {
	return SaveSnapshot{Version: previous.Version, Party: previous.Party, Inventory: previous.Inventory,
		World: previous.World, Progression: previous.Progression, Effects: previous.Effects, Wind: previous.Wind,
		Random: previous.Random, Continuation: previous.Continuation, Redux: previous.Redux,
		Presentation: previous.Presentation, LastConversation: previous.LastConversation,
		Excerpts:       previous.Excerpts,
		DungeonJournal: previous.DungeonJournal, MoonstoneJournal: previous.MoonstoneJournal, AtlasLandmarks: previous.AtlasLandmarks,
		Dungeon: previous.Dungeon, PartyVehicle: previous.PartyVehicle, MapUnits: previous.MapUnits,
		ItemStacks: previous.ItemStacks, MapMutations: previous.MapMutations, OpenDoor: previous.OpenDoor,
		BasementChestHistory: previous.BasementChestHistory, AtlasNotes: previous.AtlasNotes,
		AtlasFleet: previous.AtlasFleet, CodexRevelationComplete: previous.CodexRevelationComplete}
}
