// Package save_snapshot defines the renderer-independent native save snapshot
// format and its filesystem persistence primitives.
package save_snapshot

import "github.com/bradhannah/UltimaVRedux/internal/journal"

const (
	// CurrentVersion is the native snapshot schema version.
	CurrentVersion = 26
	// MinimumVersion is the oldest native snapshot schema this build promotes.
	// It shipped in v0.1.0-preview.1; older native saves are unsupported.
	MinimumVersion = 24
	// MaxElapsedMs bounds runtime-compatible saves to 100 years of play. This
	// conservative finite domain leaves ample int64 headroom for activation
	// epochs and gameplay deadlines without rebasing clock phase or RNG state.
	// It is a compatibility restriction, not a wire-shape change.
	MaxElapsedMs int64 = 100 * 365 * 24 * 60 * 60 * 1000
	// DungeonCellCount is the complete raw-cell count for one active dungeon.
	DungeonCellCount = 512
	// RosterSlotCount is the fixed number of legacy roster slots.
	RosterSlotCount = 16
	// EquipmentCount is the number of equipment slots in a character snapshot.
	EquipmentCount = 48
	// SpellCount is the number of spell slots in a character snapshot.
	SpellCount = 48
	// ConsumableCount is the number of potion and reagent slots in a character snapshot.
	ConsumableCount = 8
	// QuestItemCount is the number of quest item slots in a character snapshot.
	QuestItemCount = 14
	// MoonstoneCount is the number of moonstone records in a character snapshot.
	MoonstoneCount = 8
	// ShrineCount is the number of shrine records in a character snapshot.
	ShrineCount = 8
	// DungeonCount is the number of dungeon records in a character snapshot.
	DungeonCount = 8
	// ShadowlordCount is the number of shadowlord records in a character snapshot.
	ShadowlordCount = 3
	// SearchBitmaskBytes is the fixed byte length of the search-state bitmask.
	SearchBitmaskBytes = 24
	// DungeonRoomProgressBytes is the fixed byte length of cleared dungeon-room state.
	DungeonRoomProgressBytes = 14
)

// SaveSnapshot is the complete versioned native gameplay state DTO.
type SaveSnapshot struct {
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
	Notes                   journal.NotesState                  `json:"notes"`
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

	reconstructMapUnits bool
}

// BasementChestDailyRecordSnapshot is indexed by LordBritishBasementChests,
// independently of active objects. A null date means no known prior opening.
type BasementChestDailyRecordSnapshot struct {
	LastOpened *BritannianDateSnapshot `json:"lastOpened"`
}

// OpenDoorSnapshot preserves the single timed opening independently of persistent
// terrain. Occupancy-held openings are derived from final actors, never serialized.
type OpenDoorSnapshot struct {
	Position       PositionSnapshot `json:"position"`
	RemainingTurns int              `json:"remainingTurns"`
	ClosedTile     int              `json:"closedTile"`
}

// MarkMapUnitsForReconstruction records that runtime map units must be rebuilt
// after hydration. The hint is transient and is never encoded in native JSON.
func (snapshot *SaveSnapshot) MarkMapUnitsForReconstruction() {
	snapshot.reconstructMapUnits = true
}

// NeedsMapUnitReconstruction reports whether runtime map units must be rebuilt.
func (snapshot SaveSnapshot) NeedsMapUnitReconstruction() bool {
	return snapshot.reconstructMapUnits
}

// DungeonSnapshot preserves one active dungeon visit's semantic authority.
// Cells are ordered by floor, then Y, then X.
type DungeonSnapshot struct {
	Location          int                      `json:"location"`
	Cells             []int                    `json:"cells"`
	ChestAssessments  [DungeonCellCount]int    `json:"chestAssessments"`
	Facing            int                      `json:"facing"`
	LastDirection     int                      `json:"lastDirection"`
	Wanderer          *DungeonWandererSnapshot `json:"wanderer"`
	QuicknessSkipNext bool                     `json:"quicknessSkipNext"`
}

// DungeonWandererSnapshot preserves the optional active dungeon occupant.
type DungeonWandererSnapshot struct {
	Monster   int  `json:"monster"`
	Floor     int  `json:"floor"`
	X         int  `json:"x"`
	Y         int  `json:"y"`
	PreviousX int  `json:"previousX"`
	PreviousY int  `json:"previousY"`
	OnCeiling bool `json:"onCeiling"`
}

// PartySnapshot contains roster and NPC relationship state.
type PartySnapshot struct {
	Roster   [RosterSlotCount]CharacterSnapshot `json:"roster"`
	Karma    int                                `json:"karma"`
	MetNPCs  []NPCFlagSnapshot                  `json:"metNpcs"`
	DeadNPCs []NPCFlagSnapshot                  `json:"deadNpcs"`
}

// NPCFlagSnapshot identifies one NPC relationship flag.
type NPCFlagSnapshot struct {
	Location int `json:"location"`
	NPC      int `json:"npc"`
}

// CharacterSnapshot preserves every gameplay-relevant field in one roster slot.
type CharacterSnapshot struct {
	Occupied     bool   `json:"occupied"`
	Name         string `json:"name"`
	Gender       int    `json:"gender"`
	Class        int    `json:"class"`
	Status       int    `json:"status"`
	Strength     int    `json:"strength"`
	Dexterity    int    `json:"dexterity"`
	Intelligence int    `json:"intelligence"`
	CurrentMP    int    `json:"currentMp"`
	CurrentHP    int    `json:"currentHp"`
	MaximumHP    int    `json:"maximumHp"`
	Experience   int    `json:"experience"`
	Level        int    `json:"level"`
	MonthsAtInn  int    `json:"monthsAtInn"`
	ArmorClass   int    `json:"armorClass"`
	Helmet       int    `json:"helmet"`
	Armor        int    `json:"armor"`
	Weapon       int    `json:"weapon"`
	Shield       int    `json:"shield"`
	Ring         int    `json:"ring"`
	Amulet       int    `json:"amulet"`
	PartyMap     int    `json:"partyMap"`
}

// InventorySnapshot uses authoritative catalog order for every inventory category.
type InventorySnapshot struct {
	Food       uint16                  `json:"food"`
	Gold       uint16                  `json:"gold"`
	Gems       uint16                  `json:"gems"`
	Torches    uint16                  `json:"torches"`
	Keys       uint16                  `json:"keys"`
	SkullKeys  uint16                  `json:"skullKeys"`
	Equipment  [EquipmentCount]uint16  `json:"equipment"`
	Spells     [SpellCount]uint16      `json:"spells"`
	Scrolls    [ConsumableCount]uint16 `json:"scrolls"`
	Potions    [ConsumableCount]uint16 `json:"potions"`
	Reagents   [ConsumableCount]uint16 `json:"reagents"`
	QuestItems [QuestItemCount]uint16  `json:"questItems"`
	Moonstones [MoonstoneCount]uint16  `json:"moonstones"`
}

// WorldSnapshot contains position, calendar, and world-clock state.
type WorldSnapshot struct {
	Position             PositionSnapshot `json:"position"`
	LastLargeMapPosition PositionSnapshot `json:"lastLargeMapPosition"`
	Calendar             CalendarSnapshot `json:"calendar"`
	ElapsedMs            int64            `json:"elapsedMs"`
	LastProcessedHour    int              `json:"lastProcessedHour"`
	LastCamped           int              `json:"lastCamped"`
	LastWandererMonth    int              `json:"lastWandererMonth"`
	CombatSequence       uint64           `json:"combatSequence"`
	IsSailing            bool             `json:"isSailing"`
}

// PositionSnapshot stores a map location and coordinates.
type PositionSnapshot struct {
	Location int `json:"location"`
	X        int `json:"x"`
	Y        int `json:"y"`
	Floor    int `json:"floor"`
}

// CalendarSnapshot stores the in-game calendar and turn counter.
type CalendarSnapshot struct {
	Year   int    `json:"year"`
	Month  int    `json:"month"`
	Day    int    `json:"day"`
	Hour   int    `json:"hour"`
	Minute int    `json:"minute"`
	Turns  uint32 `json:"turns"`
}

// ProgressionSnapshot contains persistent quest and discovery state.
type ProgressionSnapshot struct {
	SearchFound           [SearchBitmaskBytes]byte          `json:"searchFound"`
	SkullKeysFoundDay     byte                              `json:"skullKeysFoundDay"`
	ReagentCachesFoundDay [3]byte                           `json:"reagentCachesFoundDay"`
	Moonstones            [MoonstoneCount]MoonstoneSnapshot `json:"moonstones"`
	Shrines               [ShrineCount]ShrineSnapshot       `json:"shrines"`
	Dungeons              [DungeonCount]bool                `json:"dungeons"`
	DungeonRooms          [DungeonRoomProgressBytes]byte    `json:"dungeonRooms"`
	Shadowlords           ShadowlordsSnapshot               `json:"shadowlords"`
	Recipes               [SpellCount]RecipeSnapshot        `json:"recipes"`
	HMSCapeActivated      bool                              `json:"hmsCapeActivated"`
}

// RecipeSnapshot records one spell recipe's discovery state.
type RecipeSnapshot struct {
	Known bool   `json:"known"`
	Mask  uint16 `json:"mask"`
}

// MoonstoneSnapshot records one moonstone's location and state.
type MoonstoneSnapshot struct {
	X       byte `json:"x"`
	Y       byte `json:"y"`
	Map     byte `json:"map"`
	Level   byte `json:"level"`
	State   byte `json:"state"`
	IsFound bool `json:"isFound"`
}

// ShrineSnapshot records one shrine's quest state.
type ShrineSnapshot struct {
	Active    bool `json:"active"`
	Completed bool `json:"completed"`
	Ruined    bool `json:"ruined"`
}

// ShadowlordsSnapshot records shadowlord progression.
type ShadowlordsSnapshot struct {
	Alive             [ShadowlordCount]bool `json:"alive"`
	TownAssignments   [ShadowlordCount]int  `json:"townAssignments"`
	LastCalled        int                   `json:"lastCalled"`
	Summoned          bool                  `json:"summoned"`
	AutomaticPresence int                   `json:"automaticPresence"`
}

// EffectsSnapshot records active duration and toggle effects.
type EffectsSnapshot struct {
	DurationType                 byte `json:"durationType"`
	DurationTurns                int  `json:"durationTurns"`
	DurationToggleOnly           bool `json:"durationToggleOnly"`
	MagicLightMinutesRemaining   int  `json:"magicLightMinutesRemaining"`
	CarriedTorchMinutesRemaining int  `json:"carriedTorchMinutesRemaining"`
	// REDUX DEVIATION: Original actor cadence is not stored in SAVED.GAM; ref:TIME-0003.
	// Native v15 records the independent phases for deterministic continuation.
	QuicknessWorldActorSkip bool           `json:"quicknessWorldActorSkip"`
	TownMountedActorSkip    bool           `json:"townMountedActorSkip"`
	TownQuicknessActorSkip  bool           `json:"townQuicknessActorSkip"`
	OutdoorMountedActorSkip bool           `json:"outdoorMountedActorSkip"`
	Amulet                  ToggleSnapshot `json:"amulet"`
	Crown                   ToggleSnapshot `json:"crown"`
	Badge                   ToggleSnapshot `json:"badge"`
	// REDUX DEVIATION: Original intoxication persists only for the running
	// process (ref:MERCH-0013). Native v6 persists it to preserve deterministic Redux continuation.
	Drunk   int `json:"drunk"`
	Smashed int `json:"smashed"`
}

// ToggleSnapshot records one toggle effect.
type ToggleSnapshot struct {
	Active bool `json:"active"`
	Turns  int  `json:"turns"`
}

// WindSnapshot records persistent wind state.
type WindSnapshot struct {
	Direction     int   `json:"direction"`
	UpdateCounter int   `json:"updateCounter"`
	NextTickMs    int64 `json:"nextTickMs"`
}

// RandomSnapshot records deterministic random-source state.
type RandomSnapshot struct {
	Seed        uint64 `json:"seed"`
	SourceCalls uint64 `json:"sourceCalls"`
}

// ContinuationSnapshot records non-derivable gameplay continuation state.
type ContinuationSnapshot struct {
	ActivePlayerIndex                 int  `json:"activePlayerIndex"`
	IsCriminal                        bool `json:"isCriminal"`
	HMSCapeAutomaticWaitActorSkip     bool `json:"hmsCapeAutomaticWaitActorSkip"`
	BeggarKarmaCounter                int  `json:"beggarKarmaCounter"`
	OpeningCampaignEncounterReduction bool `json:"openingCampaignEncounterReduction"`
	CombatActionTimeCounter           int  `json:"combatActionTimeCounter"`
}

// ReduxSnapshot contains versioned Redux-only native extensions.
type ReduxSnapshot struct {
	DungeonDiscoveries     []DungeonDiscoverySnapshot  `json:"dungeonDiscoveries"`
	PlayerKnowledge        []PlayerKnowledgeSnapshot   `json:"playerKnowledge"`
	QuestJournal           []QuestJournalEventSnapshot `json:"questJournal"`
	LordBritishPassageOpen bool                        `json:"lordBritishPassageOpen"`
}

const (
	// QuestJournalEventShadowlordDefeated identifies one permanent Shadowlord defeat.
	QuestJournalEventShadowlordDefeated = "shadowlord-defeated"
	// QuestJournalEventKnowledgeLearned identifies a first non-spell lore discovery.
	QuestJournalEventKnowledgeLearned = "knowledge-learned"
	// QuestJournalEventCodexAnswerLearned identifies a first Codex answer.
	QuestJournalEventCodexAnswerLearned = "codex-answer-learned"

	// QuestJournalSubjectFaulinei identifies Faulinei without using display prose.
	QuestJournalSubjectFaulinei = "faulinei"
	// QuestJournalSubjectAstaroth identifies Astaroth without using display prose.
	QuestJournalSubjectAstaroth = "astaroth"
	// QuestJournalSubjectNosfentor identifies Nosfentor without using display prose.
	QuestJournalSubjectNosfentor = "nosfentor"

	// QuestJournalSourceGameplay identifies an event recorded by live gameplay.
	QuestJournalSourceGameplay = "gameplay"
	// QuestJournalSourceMigration identifies an event inferred from older durable state.
	QuestJournalSourceMigration = "migration"
	// QuestJournalSourceDialogue identifies a named speaker's discovery.
	QuestJournalSourceDialogue = "dialogue"
	// QuestJournalSourceObservation identifies a directly observed discovery.
	QuestJournalSourceObservation = "observation"
	// QuestJournalSourceRevelation identifies a named revelation.
	QuestJournalSourceRevelation = "revelation"
)

// QuestJournalEventSnapshot records one ordered semantic Chronicle event.
// Kind and Subject identify the fact; rendered prose remains a projection concern.
type QuestJournalEventSnapshot struct {
	Sequence     uint64                  `json:"sequence"`
	Kind         string                  `json:"kind"`
	Subject      string                  `json:"subject"`
	OccurredOn   *BritannianDateSnapshot `json:"occurredOn"`
	Source       string                  `json:"source"`
	SourceName   string                  `json:"sourceName"`
	Acknowledged bool                    `json:"acknowledged"`
}

const (
	// KnowledgeCategorySpell identifies hidden-spell knowledge.
	KnowledgeCategorySpell = "spell"
	// KnowledgeCategoryShadowlord identifies learned Shadowlord lore.
	KnowledgeCategoryShadowlord = "shadowlord"
	// KnowledgeCategoryShrine identifies learned shrine mantra lore.
	KnowledgeCategoryShrine = "shrine"
	// KnowledgeCategoryJournal identifies independent domain, Flame, and method clues.
	KnowledgeCategoryJournal = "journal"
	// KnowledgeCategoryDungeon identifies the eight learned Words of Power.
	KnowledgeCategoryDungeon = "dungeon"

	// KnowledgeSourceDialogue identifies knowledge learned from a named speaker.
	KnowledgeSourceDialogue = "dialogue"
	// KnowledgeSourceObservation identifies directly observed lore.
	KnowledgeSourceObservation = "observation"
	// KnowledgeSourceRevelation identifies revealed lore.
	KnowledgeSourceRevelation = "revelation"
	// KnowledgeSourceExperimentation identifies knowledge learned by successful mixing.
	KnowledgeSourceExperimentation = "experimentation"
	// KnowledgeSourceMigration identifies knowledge inferred while migrating an older save.
	KnowledgeSourceMigration = "migration"

	// SpellFormulaUncertain identifies a formula represented by multiple candidates.
	SpellFormulaUncertain = "uncertain"
	// SpellFormulaConfirmed identifies a canonical formula.
	SpellFormulaConfirmed = "confirmed"
)

const (
	// SpellKnowledgeAnYlem is the stable key for An Ylem knowledge.
	SpellKnowledgeAnYlem = "an-ylem"
	// SpellKnowledgeInXenMani is the stable key for In Xen Mani knowledge.
	SpellKnowledgeInXenMani = "in-xen-mani"
	// SpellKnowledgeRelXenBet is the stable key for Rel Xen Bet knowledge.
	SpellKnowledgeRelXenBet = "rel-xen-bet"
)

const (
	// ShadowlordKnowledgeFaulineiName identifies knowledge of Faulinei's name.
	ShadowlordKnowledgeFaulineiName = "faulinei.name"
	// ShadowlordKnowledgeFaulineiAssociation identifies Faulinei with Falsehood.
	ShadowlordKnowledgeFaulineiAssociation = "faulinei.association"
	// ShadowlordKnowledgeAstarothName identifies knowledge of Astaroth's name.
	ShadowlordKnowledgeAstarothName = "astaroth.name"
	// ShadowlordKnowledgeAstarothAssociation identifies Astaroth with Hatred.
	ShadowlordKnowledgeAstarothAssociation = "astaroth.association"
	// ShadowlordKnowledgeNosfentorName identifies knowledge of Nosfentor's name.
	ShadowlordKnowledgeNosfentorName = "nosfentor.name"
	// ShadowlordKnowledgeNosfentorAssociation identifies Nosfentor with Cowardice.
	ShadowlordKnowledgeNosfentorAssociation = "nosfentor.association"
)

const (
	// JournalKnowledgeDomainFalsehood identifies a standalone Falsehood clue.
	JournalKnowledgeDomainFalsehood = "domain.falsehood"
	// JournalKnowledgeDomainHatred identifies a standalone Hatred clue.
	JournalKnowledgeDomainHatred = "domain.hatred"
	// JournalKnowledgeDomainCowardice identifies a standalone Cowardice clue.
	JournalKnowledgeDomainCowardice = "domain.cowardice"
	// JournalKnowledgeFlameTruth identifies the Flame of Truth and its location.
	JournalKnowledgeFlameTruth = "flame.truth"
	// JournalKnowledgeFlameLove identifies the Flame of Love and its location.
	JournalKnowledgeFlameLove = "flame.love"
	// JournalKnowledgeFlameCourage identifies the Flame of Courage and its location.
	JournalKnowledgeFlameCourage = "flame.courage"
	// JournalKnowledgeBanishmentMethod identifies Sutek's complete method.
	JournalKnowledgeBanishmentMethod = "banishment.method"
	// JournalKnowledgeResistancePassword identifies the disclosed Resistance password.
	JournalKnowledgeResistancePassword = "resistance.password"
	// JournalKnowledgeStonesMelody identifies Kenneth's harpsichord instructions.
	JournalKnowledgeStonesMelody = "stones.melody"
	// JournalKnowledgeOppressionPassword identifies the disclosed Oppression password.
	JournalKnowledgeOppressionPassword = "oppression.password"
)

const (
	// DungeonKnowledgeDeceitWordOfPower identifies Deceit's Word of Power.
	DungeonKnowledgeDeceitWordOfPower = "deceit.word_of_power"
	// DungeonKnowledgeDespiseWordOfPower identifies Despise's Word of Power.
	DungeonKnowledgeDespiseWordOfPower = "despise.word_of_power"
	// DungeonKnowledgeDestardWordOfPower identifies Destard's Word of Power.
	DungeonKnowledgeDestardWordOfPower = "destard.word_of_power"
	// DungeonKnowledgeWrongWordOfPower identifies Wrong's Word of Power.
	DungeonKnowledgeWrongWordOfPower = "wrong.word_of_power"
	// DungeonKnowledgeCovetousWordOfPower identifies Covetous's Word of Power.
	DungeonKnowledgeCovetousWordOfPower = "covetous.word_of_power"
	// DungeonKnowledgeShameWordOfPower identifies Shame's Word of Power.
	DungeonKnowledgeShameWordOfPower = "shame.word_of_power"
	// DungeonKnowledgeHythlothWordOfPower identifies Hythloth's Word of Power.
	DungeonKnowledgeHythlothWordOfPower = "hythloth.word_of_power"
	// DungeonKnowledgeDoomWordOfPower identifies Doom's Word of Power.
	DungeonKnowledgeDoomWordOfPower = "doom.word_of_power"
)

const (
	// ShrineKnowledgeHonestyMantra identifies knowledge of the Honesty mantra.
	ShrineKnowledgeHonestyMantra = "honesty.mantra"
	// ShrineKnowledgeCompassionMantra identifies knowledge of the Compassion mantra.
	ShrineKnowledgeCompassionMantra = "compassion.mantra"
	// ShrineKnowledgeValourMantra identifies knowledge of the Valour mantra.
	ShrineKnowledgeValourMantra = "valour.mantra"
	// ShrineKnowledgeJusticeMantra identifies knowledge of the Justice mantra.
	ShrineKnowledgeJusticeMantra = "justice.mantra"
	// ShrineKnowledgeSacrificeMantra identifies knowledge of the Sacrifice mantra.
	ShrineKnowledgeSacrificeMantra = "sacrifice.mantra"
	// ShrineKnowledgeHonorMantra identifies knowledge of the Honor mantra.
	ShrineKnowledgeHonorMantra = "honor.mantra"
	// ShrineKnowledgeSpiritualityMantra identifies knowledge of the Spirituality mantra.
	ShrineKnowledgeSpiritualityMantra = "spirituality.mantra"
	// ShrineKnowledgeHumilityMantra identifies knowledge of the Humility mantra.
	ShrineKnowledgeHumilityMantra = "humility.mantra"
)

// PlayerKnowledgeSnapshot records one durable generic player-knowledge entry.
type PlayerKnowledgeSnapshot struct {
	Category     string                         `json:"category"`
	Key          string                         `json:"key"`
	Source       KnowledgeSourceSnapshot        `json:"source"`
	DiscoveredOn *BritannianDateSnapshot        `json:"discoveredOn"`
	Unread       bool                           `json:"unread"`
	SpellFormula *SpellFormulaKnowledgeSnapshot `json:"spellFormula"`
}

// KnowledgeSourceSnapshot records how the player learned or confirmed knowledge.
type KnowledgeSourceSnapshot struct {
	Kind        string `json:"kind"`
	DisplayName string `json:"displayName"`
}

// BritannianDateSnapshot records a calendar date without time or turn state.
type BritannianDateSnapshot struct {
	Year  int `json:"year"`
	Month int `json:"month"`
	Day   int `json:"day"`
}

// SpellFormulaKnowledgeSnapshot records optional formula detail for spell knowledge.
type SpellFormulaKnowledgeSnapshot struct {
	State          string   `json:"state"`
	ConfirmedMask  uint16   `json:"confirmedMask"`
	CandidateMasks []uint16 `json:"candidateMasks"`
}

const (
	// DungeonDiscoveryHiddenDoor identifies a revealed original 0xD* cell.
	DungeonDiscoveryHiddenDoor = "hiddenDoor"
	// DungeonDiscoveryHiddenPit identifies a revealed original 0x61/0x69 cell.
	DungeonDiscoveryHiddenPit = "hiddenPit"
)

// DungeonDiscoverySnapshot identifies one Redux-persistent hidden dungeon feature.
type DungeonDiscoverySnapshot struct {
	Location int    `json:"location"`
	Floor    int    `json:"floor"`
	X        int    `json:"x"`
	Y        int    `json:"y"`
	Kind     string `json:"kind"`
}

// VehicleSnapshot records vehicle state.
type VehicleSnapshot struct {
	VesselID          uint64 `json:"vesselId"`
	Type              int    `json:"type"`
	CurrentDirection  int    `json:"currentDirection"`
	PreviousDirection int    `json:"previousDirection"`
	Skiffs            int    `json:"skiffs"`
	OwnedByNPC        bool   `json:"ownedByNpc"`
	Hull              int    `json:"hull"`
	SailState         int    `json:"sailState"`
	// HasHMSCape is a nonauthoritative compatibility field; progression owns the global preference.
	HasHMSCape      bool `json:"hasHmsCape"`
	PendingDelivery bool `json:"pendingDelivery"`
}

// Stable map-unit kind values used by the native wire contract.
const (
	MapUnitKindEnemy   = "enemy"
	MapUnitKindNPC     = "npc"
	MapUnitKindVehicle = "vehicle"
	MapUnitKindChest   = "chest"
)

// MapUnitSnapshot records a unit in one persistent controller. Kind is enemy,
// npc, vehicle, or chest; ReferenceID is monster type for enemies and NPC number otherwise.
type MapUnitSnapshot struct {
	World           int              `json:"world"`
	ControllerOrder int              `json:"controllerOrder"`
	Kind            string           `json:"kind"`
	ReferenceID     int              `json:"referenceId"`
	NPCNum          int              `json:"npcNum"`
	Position        PositionSnapshot `json:"position"`
	Visible         bool             `json:"visible"`
	SkipNextTurn    bool             `json:"skipNextTurn"`
	// LegacyWantsToAutoTalk retains the version 11 wire member. Runtime hydration
	// reconstructs the typed pending command from this bit and authoritative AI state.
	LegacyWantsToAutoTalk     bool               `json:"wantsToAutoTalk"`
	OverriddenAI              int                `json:"overriddenAi"`
	AIOverrideProvenance      string             `json:"aiOverrideProvenance"`
	ArrivedScheduleSlot       *int               `json:"arrivedScheduleSlot,omitempty"`
	ConsumedTalk3ScheduleSlot *int               `json:"consumedTalk3ScheduleSlot,omitempty"`
	CurrentPath               []PositionSnapshot `json:"currentPath"`
	Vehicle                   *VehicleSnapshot   `json:"vehicle"`
	EnemyVariant              string             `json:"enemyVariant"`
	EnemyCurrentHP            int                `json:"enemyCurrentHp"`
	EnemyFacing               int                `json:"enemyFacing"`
	Shadowlord                *int               `json:"shadowlord"`
	SurfaceChestNumber        *int               `json:"surfaceChestNumber,omitempty"`
	SurfaceChestAssessment    int                `json:"surfaceChestAssessment"`
	SurfaceChestAuthored      bool               `json:"surfaceChestAuthored"`
	SurfaceChestTransient     bool               `json:"surfaceChestTransient"`
}

// ItemStackSnapshot records items at one map position.
type ItemStackSnapshot struct {
	Position PositionSnapshot `json:"position"`
	Items    []ItemSnapshot   `json:"items"`
}

// ItemSnapshot records one item and its quantity.
type ItemSnapshot struct {
	Type     int    `json:"type"`
	ID       int    `json:"id"`
	Quantity uint16 `json:"quantity"`
}

// MapMutationKind identifies the map layer changed by a mutation.
type MapMutationKind string

const (
	// MapMutationSetTile changes a map tile.
	MapMutationSetTile MapMutationKind = "setTile"
	// MapMutationSetObject changes a map object layer.
	MapMutationSetObject MapMutationKind = "setObject"
)

// MapMutationSnapshot records one persistent map mutation.
type MapMutationSnapshot struct {
	Position PositionSnapshot `json:"position"`
	Kind     MapMutationKind  `json:"kind"`
	Value    int              `json:"value"`
}
