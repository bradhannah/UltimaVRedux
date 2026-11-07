// Package game_state provides the central game state management for Ultima 5 Redux.
//
// The GameState struct serves as the main coordinator for all game systems including:
//   - Player party and character management
//   - World map state and player location
//   - NPC AI controllers for different map types
//   - Deterministic random number generation for consistent gameplay
//   - Environmental systems (weather, wind, hazards)
//   - SystemCallbacks for UI interaction without tight coupling
//
// Key architectural patterns:
//   - Centrally supplied elapsed time for animations and AI
//   - SystemCallbacks dependency injection for UI decoupling
//   - Deterministic RNG methods for testing and reproducible behavior
//
// The package follows the Action* pattern for player commands, where each
// command has ActionCommandMapType methods that use SystemCallbacks for
// user feedback while maintaining clean separation of concerns.
package game_state

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/ai"
	"github.com/bradhannah/UltimaVRedux/internal/camping"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/datetime"
	"github.com/bradhannah/UltimaVRedux/internal/environment"
	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/shrine"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
	"github.com/bradhannah/UltimaVRedux/pkg/semantic_input"
	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

// Ordinary surface command durations in game minutes.
const (
	DefaultSmallMapMinutesPerTurn = 1
	DefaultLargeMapMinutesPerTurn = 2
)

const (
	nightTowneCloseTime = 8 + 12
	nightTowneOpenTime  = 5

	// Britain drawbridge coordinates from original game data
	leftXDrawBridge   = 14 // Western edge of drawbridge area
	rightXDrawBridge  = 16 // Eastern edge of drawbridge area
	topYDrawBridge    = 28 // Northern edge of drawbridge area
	bottomYDrawBridge = 29 // Southern edge of drawbridge area

	// Mirror reflection position offset
	mirrorReflectionYOffset = 1 // Avatar must be 1 tile south of mirror to see reflection

)

// GameState owns authoritative gameplay state, clocks, RNG and action mutations.
type GameState struct {
	PartyState   party_state.PartyState
	MapState     map_state.MapState
	AiController ai.NPCAIController

	surfaceCycleOwned          bool
	completingSurfaceCycle     bool
	transientLargeMapPushTiles map[transientLargeMapPushTileKey]transientLargeMapPushTile

	MoonstoneStatus MoonstoneStatus

	// Moonstone system for moongate transportation
	moonstoneSystem *MoonstoneSystem
	// REDUX DEVIATION: Original SAVED.GAM stores the gate-stage byte at 0x02E1.
	// Redux reconstructs stable loads at stage 0 or 16 and rejects transition saves
	// instead of resuming a partially materialized gate, keeping restoration stable. ref:MOON-0003
	moongateStage MoongateStage

	DebugOptions references.DebugOptions

	GameReferences *references.GameReferences
	GameConfig     *config.UltimaVConfiguration

	PartyVehicle map_units.NPCFriendly

	// Automatic sailing state (frigate with sails raised)
	isSailing                     bool
	automaticSailingStopEpoch     uint64
	pendingSkiffDeepWaterHits     int
	hmsCapeAutomaticWaitActorSkip bool

	// hmsCapeActivated is the persisted global HMS Cape player preference.
	hmsCapeActivated bool

	CurrentNPCAIController  ai.NPCAIController
	LargeMapNPCAIController map[references.World]*ai.NPCAIControllerLargeMap

	LastLargeMapPosition references.Position
	LastLargeMapFloor    references.FloorNumber

	TheOdds references.TheOdds

	DateTime  datetime.UltimaDate
	ElapsedMs int64

	openingCampaignEncounterReduction bool
	combatActionTimeCounter           int

	ItemStacksMap  references.ItemStacksMap
	SurfaceCorpses []SurfaceCorpse

	// Indexed by the three verified Castle basement source identities, not runtime slots.
	BasementChestHistory [3]BasementChestDailyRecord

	// Harpsichord puzzle state. Partial melody progress is transient; the opened
	// Lord British passage is durable native-save progression.
	harpsichordMelodyProgress int
	lordBritishPassageOpen    bool

	// Search system manager
	SearchManager *SearchSystemManager

	// Dependency injection callbacks for external systems
	SystemCallbacks *SystemCallbacks

	// Centralized deterministic random number generator
	rng *rng.Source

	// Environmental systems
	environmentalHazards *environment.EnvironmentalHazards
	windSystem           *environment.WindSystem
	earthquakeSystem     *EarthquakeSystem
	pirateAttackQueue    []pirateAttackRequest
	// Lighthouse rotation system for small maps (rotating beacon light cone)
	LighthouseState *map_state.LighthouseState
	// Light source tinting state for colored light effects at night
	LightSourceTintingState *map_state.LightSourceTintingState
	daylightTinting         *map_state.DaylightField
	daylightLayer           *map_state.LayeredMap

	// Duration effect tracking (follows original U5 single-effect model)
	ActiveDurationEffect DurationEffect

	// MagicLightMinutesRemaining is independent of the active duration-effect
	// slot, preserving the separate light-fuel authority. ref:LIGHT-0004
	MagicLightMinutesRemaining int

	// Outdoor cadence counters persist across map visits. ref:TIME-0002
	quicknessWorldActorSkip bool
	outdoorMountedActorSkip bool
	// Town cadence belongs to one townloop visit, not a vehicle or calendar turn.
	townMountedActorSkip   bool
	townQuicknessActorSkip bool
	// drunk counts successful drinks; smashed drives original forced-command intoxication.
	drunk   int
	smashed int

	// Toggle item states (Amulet, Crown, Badge) - can be active concurrently
	AmuletState ToggleItemState
	CrownState  ToggleItemState
	BadgeState  ToggleItemState

	// Event queue system for orchestrated sequences
	EventQueueManager *event_queue.EventQueueManager

	// Testing overrides
	jimmySuccessForTesting         func(*party_state.PlayerCharacter) bool
	jimmyPrisonerSuccessForTesting func(*party_state.PlayerCharacter) bool

	// Active player selection (-1 for none/all, 0-5 for specific character)
	activePlayerIndex             int
	merchantSessionSequence       uint64
	tavernCustomerSessionSequence uint64

	// Shadowlord state tracking for summoning at Keeps and defeat at Flames
	ShadowlordState *references.ShadowlordState

	// Dungeon entrance states for Words of Power system
	// Index corresponds to dungeon order: [0]=Deceit, [1]=Despise, [2]=Destard, [3]=Wrong, [4]=Covetous, [5]=Shame, [6]=Hythloth, [7]=Doom
	DungeonStates [8]bool // Track open/closed status: true=open, false=sealed

	// deferredDungeonRoomProgress preserves cleared-room bits for future dungeon reconstruction.
	deferredDungeonRoomProgress [14]byte
	// dungeonDiscoveries preserves optional Redux hidden-door and hidden-pit discoveries between visits.
	dungeonDiscoveries map[dungeonDiscoveryKey]dungeonDiscoveryKind

	// activeDungeon owns the mutable semantic working map for an active visit.
	// MapState.PlayerLocation remains authoritative for location, floor, and X/Y.
	activeDungeon *DungeonState

	dungeonCommandSequence uint64
	castUseCommitSequence  uint64
	activeDungeonCommand   *dungeonCommandCycleState

	// Last dungeon toggle result for Word of Power earthquake sequence
	// true = dungeon entrance was opened, false = dungeon entrance was sealed
	// Used by UI layer to determine earthquake message ("entrance opens!" vs "entrance seals!")
	lastDungeonToggleOpened bool

	// Note: TRAP fall sequences now managed by event queue system
	// in action_fall_through.go - no state fields needed

	// Camping system
	CampingScene       *camping.CampingScene // Active camping scene state
	PreCampingLocation references.Location   // Location before entering camping (for restoration)
	LastWandererMonth  int                   // Track last month wanderer appeared (prevents multiple per month)

	// Combat system
	CombatScene                  *CombatState // Active combat state (nil when not in combat)
	combatEncounterSequence      uint64
	combatRecoveryReviewDeferred bool
	pendingOverworldEncounter    *map_units.NPCEnemy
	outdoorActorContinuation     *outdoorActorContinuation
	pendingCombatEncounter       *CombatEncounterRequest
	pendingSmallMapEncounter     *SmallMapEncounterOrigin
	pendingCampingCombat         *pendingCampingCombat
	pendingDirectMonsterContact  OverworldContactType
	activeShipRepair             *shipRepairState
	activeSlowMovement           *slowMovementState
	activeWaterfall              *waterfallCascade
	outdoorWindow                outdoorWindow
	// LastCamped tracks hours until ordinary camping can restore the party again.
	// It decrements at hour crossings and is set to 14 after successful camping.
	// See ref:MOON-0003.
	LastCamped int

	// lastProcessedHour is the hour observation replaced by positive time requests
	// and completed player updates. Native snapshots retain its legacy field name.
	// pendingProvisionHours holds at most one deferred observation, never a backlog.
	// See ref:PROV-0002.
	lastProcessedHour     int
	pendingProvisionHours []int
	beggarKarmaCounter    int
	// Sprite override system for character/NPC visual state changes
	SpriteOverrides *SpriteOverrideManager // Manages context-based sprite overrides (camping, combat, etc.)

	// Magic system state tracking spell recipe discoveries
	MagicState *MagicState
	// playerKnowledge is the mandatory generic journal of durable discoveries.
	playerKnowledge *playerKnowledge
	// questJournal owns ordered semantic Chronicle events; progression remains authoritative elsewhere.
	questJournal *questJournal
	// CodexRevelationComplete records acknowledgment of the final revelation page.
	CodexRevelationComplete bool
	dungeonJournal          [8]journal.DungeonObservation
	atlasLandmarks          []journal.AtlasLandmarkObservation
	atlasNotes              journal.AtlasNotesState
	atlasFleet              journal.AtlasFleetState
	atlasNoteSession        *atlasNoteSession
	playerNotes             journal.NotesState
	moonstoneJournal        [8]journal.MoonstoneObservation

	// Criminal status tracking - guards arrest criminals instead of extortion
	// Does NOT persist across map transitions (guards forget when you leave)
	// Set by CallGuards() when crimes committed, cleared after arrest/map change
	isCriminal bool

	// Shrine system state - using clear booleans, not bitmasks
	// These fields are private - access ONLY through getter/setter methods
	shrineQuestsCompleted [TotalShrines]bool // true = quest completed (visited Codex for answer)
	shrineQuestsActive    [TotalShrines]bool // true = quest assigned, not yet completed at shrine
	closedShrines         [TotalShrines]bool // true = shrine is ruined (closed by Blackthorn)
	blackthornSession     *blackthornInterrogationSession
	finaleHandoffPending  bool
	lordBritishFinale     *LordBritishFinaleState

	// Shrine visual scene system (for 11x11 shrine map with walk animation)
	ShrineScene       *shrine.Scene       // Active shrine scene state (nil when not in shrine)
	PreShrineLocation references.Location // Location before entering shrine (for restoration)
}

func initBlankGameState(gameReferences *references.GameReferences, gameConfig *config.UltimaVConfiguration, xTilesVisibleOnGameScreen,
	yTilesVisibleOnGameScreen int) *GameState {
	gameState := &GameState{}

	gameState.GameReferences = gameReferences
	gameState.GameConfig = gameConfig

	// Initialize deterministic RNG with fixed seed for consistent behavior
	gameState.rng = rng.New(1)
	gameState.MapState = *map_state.NewMapState(
		map_state.NewMapStateInput{
			GameDimensions:            gameState,
			XTilesVisibleOnGameScreen: xTilesVisibleOnGameScreen,
			YTilesVisibleOnGameScreen: yTilesVisibleOnGameScreen,
		},
	)

	// Initialize search system manager
	gameState.SearchManager = NewSearchSystemManager(gameState)

	// Initialize moonstone system with default buried locations
	gameState.moonstoneSystem = NewMoonstoneSystem()

	// Initialize active player to none/all (-1)
	gameState.activePlayerIndex = -1

	// Initialize Shadowlord state with all Shadowlords alive
	gameState.ShadowlordState = references.NewShadowlordState()

	// Initialize event queue system with time source and SystemCallbacks
	gameState.EventQueueManager = event_queue.NewEventQueueManager(
		func() int64 { return gameState.ElapsedMs },
		&eventQueueSystemCallbacks{gameState},
	)
	gameState.EventQueueManager.SetShutdownAction(func() {
		gameState.cancelWaterfallCascade(gameState.activeWaterfall)
		gameState.cancelOutdoorActorPhase()
		gameState.pendingOverworldEncounter = nil
	})

	// Initialize sprite override system for character visual state management
	gameState.SpriteOverrides = NewSpriteOverrideManager()

	// Initialize magic state with starting spell recipes (In Lor, An Nox)
	gameState.MagicState = NewMagicState()
	gameState.playerKnowledge = newPlayerKnowledge()
	gameState.questJournal = newQuestJournal()

	// Initialize environmental systems (must be done after SystemCallbacks setup in actual usage)
	// These are created with nil callbacks initially, will be properly set when SystemCallbacks are available
	gameState.initializeEnvironmentalSystems()

	return gameState
}

// initializeEnvironmentalSystems initializes all environmental systems with nil callbacks initially.
// The systems will be properly configured with SystemCallbacks when they become available.
func (g *GameState) initializeEnvironmentalSystems() {
	// Initialize environmental hazards system (swamp poison, lava damage, etc.)
	g.environmentalHazards = environment.NewEnvironmentalHazards(g.rng, nil)

	// Initialize wind system (wind changes, pirate ship speed modifiers)
	g.windSystem = environment.NewWindSystem(g.rng, nil)

	// Initialize earthquake system (underworld earthquakes, dungeon entrance effects)
	g.earthquakeSystem = newEarthquakeSystem(g.rng, nil)
}

// SetupEnvironmentalSystemCallbacks configures SystemCallbacks for all environmental systems.
// This should be called after SystemCallbacks are available to enable proper system interactions.
func (g *GameState) SetupEnvironmentalSystemCallbacks() {
	if g.SystemCallbacks == nil {
		panic("SetupEnvironmentalSystemCallbacks: SystemCallbacks is nil")
	}

	g.environmentalHazards.SetCallbacks(&messageCallbacksAdapter{g.SystemCallbacks})
	g.windSystem.SetCallbacks(&windSystemCallbacksAdapter{g.SystemCallbacks})
	g.earthquakeSystem.SetCallbacks(&earthquakeCallbacksAdapter{g})
}

// IsAvatarAtPosition compares a position with the party's current coordinates.
func (g *GameState) IsAvatarAtPosition(pos *references.Position) bool {
	return g.MapState.PlayerLocation.Position.Equals(pos)
}

// SetPlayerPosition sets the player's current position on the map.
// Use for programmatic position changes (teleportation, push-back, etc.)
func (g *GameState) SetPlayerPosition(pos references.Position) {
	g.MapState.PlayerLocation.Position = pos
}

// GetCurrentSmallLocationReference returns the current location's static reference.
func (g *GameState) GetCurrentSmallLocationReference() *references.SmallLocationReference {
	return g.GameReferences.LocationReferences.GetLocationReference(g.MapState.PlayerLocation.Location)
}

// GetLayeredMapByCurrentLocation resolves the active map type and floor.
func (g *GameState) GetLayeredMapByCurrentLocation() *map_state.LayeredMap {
	return g.MapState.LayeredMaps.GetLayeredMap(g.MapState.PlayerLocation.Location.GetMapType(), g.MapState.PlayerLocation.Floor)
}

// GetOuterTileForCurrentLocation returns the appropriate tile to render for out-of-bounds positions.
// For small maps (towns, dungeons), this returns the location-specific outer tile.
// For large maps, this returns the world border tile.
// For combat maps (camping, encounters), this returns a void tile since combat maps don't have borders.
func (g *GameState) GetOuterTileForCurrentLocation() indexes.SpriteIndex {
	mapType := g.MapState.PlayerLocation.Location.GetMapType()

	//nolint:exhaustive // All local and unknown contexts share the location-reference fallback.
	switch mapType {
	case references.CombatMapType:
		// Combat/camping maps don't have outer tiles - use black void
		return indexes.NoSprites

	case references.LargeMapType:
		// Large maps (overworld) use water as border
		return indexes.Water1

	default:
		// Small maps and dungeons use location-specific outer tiles
		smallLocRef := g.GetCurrentSmallLocationReference()
		if smallLocRef == nil {
			// Fallback to black void if location reference not found
			return indexes.NoSprites
		}

		return smallLocRef.GetOuterTile()
	}
}

// IsOutOfBounds checks if a position is outside the valid map boundaries.
// For large maps, this checks against the world map boundaries.
// For small maps (towns, dungeons), it checks against the specific location's boundaries.
// For combat/camping maps, checks against 11x11 combat map boundaries.
func (g *GameState) IsOutOfBounds(position references.Position) bool {
	mapType := g.MapState.PlayerLocation.Location.GetMapType()

	if mapType == references.LargeMapType {
		// Check if position is outside the valid coordinate range (0 to 255 for both axes)
		if position.X < 0 || position.Y < 0 || position.X >= references.XLargeMapTiles || position.Y >= references.YLargeMapTiles {
			return true
		}

		return false
	}

	if mapType == references.CombatMapType {
		// Combat/camping maps are 11x11
		if position.X < 0 || position.Y < 0 || position.X >= references.XCombatMapTiles || position.Y >= references.YCombatMapTiles {
			return true
		}

		return false
	}

	if position.X < 0 || position.Y < 0 {
		return true
	}

	// Small maps and dungeons
	if position.X > g.GetCurrentSmallLocationReference().GetMaxX() ||
		position.Y > g.GetCurrentSmallLocationReference().GetMaxY() {
		return true
	}

	return false
}

// GetCurrentLayeredMap returns the authoritative current-floor map.
func (g *GameState) GetCurrentLayeredMap() *map_state.LayeredMap {
	return g.MapState.LayeredMaps.GetLayeredMap(
		g.MapState.PlayerLocation.Location.GetMapType(),
		g.MapState.PlayerLocation.Floor)
}

// GetCurrentLayeredMapAvatarTopTile returns the party's topmost tile, if loaded.
func (g *GameState) GetCurrentLayeredMapAvatarTopTile() *references.Tile {
	theMap := g.GetCurrentLayeredMap()
	topTile := theMap.GetTopTile(&g.MapState.PlayerLocation.Position)
	if topTile == nil {
		return nil
	}

	return topTile
}

// IsPassable checks terrain, occupancy and boarding rules for the party.
func (g *GameState) IsPassable(pos *references.Position) bool {
	theMap := g.GetCurrentLayeredMap()
	topTile := theMap.GetTopTile(pos)

	if topTile == nil {
		return false
	}

	// Check boardable vehicle tiles before ordinary terrain passability.
	// Carpets, horses, skiffs and anchored frigates have their own admission
	// path; object occupancy must still be considered. ref:VEH-0002

	// Check for quest item collision - quest items are rendered as overlays but should block movement
	hasQuestItemHere, questItem := g.IsQuestItemAtPosition(*pos)
	if hasQuestItemHere && g.IsQuestItemVisible(*pos, questItem) {
		return false // Block movement through visible quest items (shards, Crown, Sceptre, Amulet)
	}

	// Check if there's a vehicle object at this position
	if g.ObjectPresentAt(pos) {
		vehicle := g.CurrentNPCAIController.GetNpcs().GetVehicleAtPositionAndFloorOrNil(
			*pos,
			g.MapState.PlayerLocation.Floor,
		)
		if vehicle != nil {
			// Allow boarding vehicles - they should be passable
			return true
		}

		// Check if it's a whirlpool - whirlpools should be passable for frigates
		if g.GetWhirlpoolAt(*pos) != nil {
			// Allow movement onto whirlpools (they trigger transportation after landing)
			return true
		}

		// Block movement for non-vehicle objects (NPCs, obstacles, etc.)
		return false
	}
	if _, corpse := g.surfaceCorpseAt(*pos, references.SearchObjectDeadPC); corpse != nil {
		return true
	}
	if _, corpse := g.surfaceCorpseAt(*pos, references.SearchObjectDeadMonster); corpse != nil {
		return true
	}

	// Check terrain passability using effective tile (includes time-based overrides like portcullis)
	effectiveTileIndex := g.GetEffectiveTile(pos)
	effectiveTile := &references.Tile{Index: effectiveTileIndex}
	vehicle := g.PartyVehicle.GetVehicleDetails()
	if vehicle.VehicleType == references.SkiffVehicle {
		return effectiveTile.IsSkiffPassableInDirection(vehicle.CurrentDirection())
	}

	return effectiveTile.IsPassable(vehicle.VehicleType)
}

// GetTileInDirectionWrapped returns the tile in the specified direction from the given position,
// automatically handling map wrapping based on the current map type.
//
// For LargeMapType (overworld/underworld): positions wrap around using XLargeMapTiles/YLargeMapTiles
// For SmallMapType (towns/dungeons): out-of-bounds positions return nil (no wrapping)
//
// Special cases:
// - NoneDirection: returns the tile at the current position
// - Invalid direction: returns nil
//
// This is a general-purpose utility for directional tile queries used throughout the game.
func (g *GameState) GetTileInDirectionWrapped(position *references.Position, direction references.Direction) *references.Tile {
	layeredMap := g.GetCurrentLayeredMap()

	// Handle NoneDirection: return tile at current position
	if direction == references.NoneDirection {
		return layeredMap.GetTopTile(position)
	}

	// Get position in specified direction
	newPos := direction.GetNewPositionInDirection(position)
	if newPos == nil {
		return nil // Invalid direction
	}

	// Auto-detect map type and apply appropriate handling
	if g.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType {
		// Large maps wrap around
		wrappedPos := newPos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)

		return layeredMap.GetTopTile(wrappedPos)
	}
	// Small maps do not wrap.
	if g.IsOutOfBounds(*newPos) {
		return nil
	}

	return layeredMap.GetTopTile(newPos)
}

// GetDrawBridgeWaterByTime selects the nighttime drawbridge water tile.
func (g *GameState) GetDrawBridgeWaterByTime(origIndex indexes.SpriteIndex) indexes.SpriteIndex {
	if g.DateTime.Hour >= nightTowneCloseTime || g.DateTime.Hour < nightTowneOpenTime {
		return indexes.WaterCoast
	}

	return origIndex
}

// GetEffectiveTile returns the effective tile at a position considering all override systems.
// This function implements the unified tile override system that replaces multiple competing approaches.
//
// Priority order (highest to lowest):
// 1. Encounter-local combat objects - Chests and loose battlefield treasure
// 2. Equipment/Items - Items dropped by players or found via search (ItemStacksMap)
// 3. Time-based terrain - Scheduled gate squares (below stored overrides), drawbridges
// 4. Dungeon entrance transformations - Show blocked/open entrances based on Words of Power state
// 5. Quest item conditional visibility - Hide quest items if already owned
// 6. Mirror reflections - Avatar reflection when positioned correctly (calculated)
// 7. State-based overrides - Player actions stored in MapOverrideLayer (doors, broken mirrors, pushed objects)
// 8. Base map tile - Original map data (MapLayer)
//
// The function gracefully handles edge cases like nil positions and missing maps.
func (g *GameState) GetEffectiveTile(pos *references.Position) indexes.SpriteIndex {
	// Handle nil position gracefully
	if pos == nil {
		return indexes.NoSprites
	}

	// Get the LayeredMap for current location (works for all map types including CombatMapType)
	currentMap := g.GetLayeredMapByCurrentLocation()
	if currentMap == nil {
		return indexes.NoSprites
	}

	if g.CombatScene != nil && g.MapState.PlayerLocation.Location.GetMapType() == references.CombatMapType &&
		!g.CombatTerrainAllowsObjectDisplay(*pos) {
		return g.getEffectiveMapTerrainTile(currentMap, pos)
	}

	// PRIORITY 1: Encounter-local combat objects.
	if g.CombatScene != nil && g.MapState.PlayerLocation.Location.GetMapType() == references.CombatMapType {
		if object, found := g.CombatScene.ObjectAt(*pos); found {
			return object.Sprite()
		}
	}

	// PRIORITY 2: Equipment/Items (visual indication of searchable/obtainable items)
	if g.ItemStacksMap.HasItemStackAtPosition(pos) {
		topItem := g.ItemStacksMap.Peek(pos)
		if topItem != nil {
			itemRef := g.GameReferences.InventoryItemReferences.GetReferenceByItem(topItem.Item)

			return itemRef.ItemSprite
		}
	}
	if corpseSprite, found := g.surfaceCorpseSpriteAt(*pos); found {
		return corpseSprite
	}

	effectiveTile := g.getEffectiveMapTerrainTile(currentMap, pos)

	// Quest item conditional visibility - hide if already owned
	if effectiveTile == indexes.Crown {
		if g.PartyState.Inventory.QuestItems.HasSome(references.Crown) {
			// Crown already in inventory - replace with floor tile
			return indexes.BrickFloor
		}
	}
	if effectiveTile == indexes.Sceptre {
		if g.PartyState.Inventory.QuestItems.HasSome(references.Sceptre) {
			// Sceptre already in inventory - replace with floor tile
			return indexes.BrickFloor
		}
	}
	if effectiveTile == indexes.Amulet {
		if g.PartyState.Inventory.QuestItems.HasSome(references.Amulet) {
			// Amulet already in inventory - replace with floor tile
			return indexes.BrickFloor
		}
	}

	// Mirror reflections (calculated based on avatar position)
	if effectiveTile == indexes.Mirror {
		// Check if avatar is positioned to see reflection (south of mirror)
		avatarPos := g.MapState.PlayerLocation.Position
		if avatarPos.X == pos.X && avatarPos.Y == pos.Y+mirrorReflectionYOffset {
			return indexes.MirrorAvatar // Show reflection
		}
	}

	return effectiveTile
}

// GetBonusTerrainTile returns map terrain for bonus-tile sampling without combat objects,
// item stacks, surface corpses, or avatar reflections. Legacy artifact tiles use their
// known brick-floor underlay; modern quest placements leave the actual terrain intact.
// Like GetEffectiveTile, nil positions and missing maps or tiles return NoSprites.
// Callers own map bounds and wrapping.
func (g *GameState) GetBonusTerrainTile(pos *references.Position) indexes.SpriteIndex {
	if pos == nil {
		return indexes.NoSprites
	}
	currentMap := g.GetLayeredMapByCurrentLocation()
	if currentMap == nil {
		return indexes.NoSprites
	}

	effectiveTile := g.getEffectiveMapTerrainTile(currentMap, pos)
	if effectiveTile == indexes.Crown || effectiveTile == indexes.Sceptre || effectiveTile == indexes.Amulet {
		return indexes.BrickFloor
	}

	return effectiveTile
}

// getEffectiveMapTerrainTile resolves real map state shared by gameplay and terrain sampling.
func (g *GameState) getEffectiveMapTerrainTile(currentMap *map_state.LayeredMap, pos *references.Position) indexes.SpriteIndex {
	// Get the effective tile from map layers (base + overrides)
	// This handles MapOverrideLayer vs MapLayer priority automatically
	var effectiveTile *references.Tile
	if g.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType {
		effectiveTile = currentMap.GetSmallMapTerrainTile(pos, g.DateTime.Hour)
	} else {
		effectiveTile = currentMap.GetTileTopMapOnlyTile(pos)
	}
	if effectiveTile == nil || effectiveTile.Index <= 0 {
		return indexes.NoSprites
	}

	// Time-based overrides (calculated, not stored)

	// Drawbridge time-based behavior (only in specific area in Britain)
	if g.MapState.PlayerLocation.Location == references.Britain &&
		pos.X >= leftXDrawBridge && pos.X <= rightXDrawBridge && pos.Y >= topYDrawBridge && pos.Y <= bottomYDrawBridge {
		// Check if this is a drawbridge tile
		if effectiveTile.Is(indexes.LeftWoodFloor2) || effectiveTile.Is(indexes.RightWoodFloor2) {
			// At night, drawbridge is down (shows water)
			if g.DateTime.Hour >= nightTowneCloseTime || g.DateTime.Hour < nightTowneOpenTime {
				return g.GetDrawBridgeWaterByTime(effectiveTile.Index)
			}
			// During day, drawbridge is up (shows wood floor)
			return effectiveTile.Index
		}
	}

	// Dungeon entrance state-based transformation
	// Check if this position is a dungeon entrance and transform tile based on dungeon state
	if g.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType {
		dungeonCoords := g.GetDungeonEntranceCoordinates()
		for dungeon, entrancePos := range dungeonCoords {
			if !DungeonEntranceExistsOnFloor(dungeon, g.MapState.PlayerLocation.Floor) {
				continue
			}
			if pos.X == entrancePos.X && pos.Y == entrancePos.Y {
				openSprite := dungeonOpenEntranceSprite(dungeon)
				if effectiveTile.Index != openSprite && !effectiveTile.Is(indexes.BlockEntrance) {
					continue
				}
				if g.IsDungeonOpen(dungeon) {
					return openSprite
				}
				// Dungeon is sealed - show blocked entrance.
				return indexes.BlockEntrance
			}
		}
	}

	// Shrine ruined state (on large map only)
	// Check if this position is a shrine and show BrokenShrine if ruined
	if g.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType &&
		effectiveTile.Is(indexes.Shrine) {
		shrineIndex := GetShrineByPosition(byte(pos.X), byte(pos.Y))
		if shrineIndex != -1 && g.IsShrineRuined(shrineIndex) {
			return indexes.BrokenShrine
		}
	}

	return effectiveTile.Index
}

// BoardVehicleResult distinguishes boarding success, warnings and capacity refusal.
type BoardVehicleResult int

// Boarding outcomes preserve both vehicles when admission fails.
const (
	BoardVehicleResultSuccess BoardVehicleResult = iota
	BoardVehicleResultNoVehicle
	BoardVehicleResultNoSkiffs
	BoardVehicleResultSkiffCapacity
)

// BoardVehicle commits boarding and transfers carried skiffs or a carpet.
func (g *GameState) BoardVehicle(vehicle map_units.NPCFriendly) BoardVehicleResult {
	result := BoardVehicleResultSuccess
	partyVehicleType := g.PartyVehicle.GetVehicleDetails().VehicleType

	if vehicle.GetVehicleDetails().VehicleType == references.FrigateVehicle {
		if partyVehicleType == references.SkiffVehicle {
			// REDUX DEVIATION: original command rules increment the frigate's skiff count without
			// a capacity check. Redux refuses at the six-bit limit so both vehicles
			// remain intact and native v5 state stays representable.
			// See ref:MOON-0003.
			if vehicle.GetVehicleDetails().GetSkiffQuantity() >= MaxFrigateSkiffs {
				return BoardVehicleResultSkiffCapacity
			}
			vehicle.GetVehicleDetails().IncrementSkiffQuantity()
		}
		if vehicle.GetVehicleDetails().GetSkiffQuantity() == 0 {
			result = BoardVehicleResultNoSkiffs
		}
	}

	if !g.CurrentNPCAIController.GetNpcs().RemoveNPCAtPositionAndFloor(
		g.MapState.PlayerLocation.Position,
		g.MapState.PlayerLocation.Floor,
	) {
		// Programming bug: NPC should exist at player position during vehicle boarding
		panic(fmt.Sprintf(
			"BoardVehicle: vehicle removal failed at X=%d,Y=%d,floor=%d",
			g.MapState.PlayerLocation.Position.X,
			g.MapState.PlayerLocation.Position.Y,
			g.MapState.PlayerLocation.Floor,
		))
	}
	if partyVehicleType == references.CarpetVehicle &&
		vehicle.GetVehicleDetails().VehicleType == references.FrigateVehicle {
		g.PartyState.Inventory.QuestItems.IncrementByOne(references.Carpet)
	}
	g.PartyVehicle = vehicle
	g.registerAtlasVessel(g.PartyVehicle.GetVehicleDetails())

	return result
}

// GetTilesVisibleOnScreen returns the viewport's tile dimensions.
func (g *GameState) GetTilesVisibleOnScreen() (int, int) {
	return g.GetCurrentLayeredMap().GetTilesVisibleOnScreen()
}

// GetTopLeftExtent returns the viewport's top-left world coordinate.
func (g *GameState) GetTopLeftExtent() references.Position {
	return g.GetCurrentLayeredMap().TopLeft
}

// GetBottomRightExtent returns the viewport's bottom-right coordinate.
func (g *GameState) GetBottomRightExtent() references.Position {
	return g.GetCurrentLayeredMap().BottomRight
}

// IsWrappedMap reports whether coordinates wrap on the current map.
func (g *GameState) IsWrappedMap() bool {
	return g.GetCurrentLayeredMap().IsWrappedMap()
}

// GetBottomRightWithoutOverflow returns the map-bounded viewport extent.
func (g *GameState) GetBottomRightWithoutOverflow() references.Position {
	return g.GetCurrentLayeredMap().GetBottomRightWithoutOverflow()
}

// GetCurrentLargeMapNPCAIController selects the surface or Underworld controller.
func (g *GameState) GetCurrentLargeMapNPCAIController() *ai.NPCAIControllerLargeMap {
	if g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		panic(fmt.Sprintf("expected large map type, got %d", g.MapState.PlayerLocation.Location.GetMapType()))
	}
	var npcAiCon *ai.NPCAIControllerLargeMap

	if g.MapState.IsOverworld() {
		npcAiCon = g.LargeMapNPCAIController[references.OVERWORLD]
	} else {
		npcAiCon = g.LargeMapNPCAIController[references.UNDERWORLD]
	}

	return npcAiCon
}

// UpdateLighthouseForLargeMap scans the visible viewport for lighthouse buildings.
// Called after player movement on large maps to detect lighthouses entering/leaving view.
// Original: original outdoor effect rules scan 32x32 map buffer for LIGHTHOUSE tile.
// See ref:MOON-0003.
func (g *GameState) UpdateLighthouseForLargeMap() {
	if g.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return
	}

	if g.LighthouseState == nil {
		g.LighthouseState = map_state.NewLighthouseState()
	}

	layeredMap := g.GetCurrentLayeredMap()
	if layeredMap == nil {
		return
	}

	// Scan 16 tiles in each direction (32x32 area matching original)
	// Original: memchr(&map[0][0][0], LIGHTHOUSE, 1024) scans 4 quadrants of 16x16 each
	const viewRadius = 16
	positions := layeredMap.FindLighthousePositionsInViewport(
		g.MapState.PlayerLocation.Position,
		viewRadius,
		indexes.Lighthouse, // Overworld lighthouse building tile (0x1b = 27)
	)
	g.LighthouseState.SetPositions(positions)
	g.LighthouseState.SetLargeMapMode(true) // Enable coordinate wrapping for 256x256 overworld
}

// UpdateLighthouseForSmallMap scans the current floor for lighthouse beacon tiles.
// Called after player movement on small maps to detect lighthouse beacons.
// Only scans the current floor - naturally handles floor changes via klimb.
func (g *GameState) UpdateLighthouseForSmallMap() {
	if g.MapState.PlayerLocation.Location.GetMapType() != references.SmallMapType {
		return
	}

	if g.LighthouseState == nil {
		g.LighthouseState = map_state.NewLighthouseState()
	}

	layeredMap := g.GetCurrentLayeredMap()
	if layeredMap == nil {
		return
	}

	// Scan current floor only for LighthouseLight beacon tiles (sprite 0x2a)
	// This naturally handles floor changes - if player klimbs to a floor without
	// a beacon, no positions are found and lighthouse tinting is disabled
	positions := layeredMap.FindLighthousePositions()
	g.LighthouseState.SetPositions(positions)
	g.LighthouseState.SetLargeMapMode(false) // Small maps don't wrap coordinates
}

// UpdateLightSourceTinting updates the light source tinting state based on current map light sources.
// Should be called after RecalculateVisibleTilesWithCustomLights in the rendering pipeline.
// This is separate from visibility (handled by Lighting) and lighthouse (LighthouseState).
func (g *GameState) UpdateLightSourceTinting(layer *map_state.LayeredMap) {
	if g.LightSourceTintingState == nil {
		g.LightSourceTintingState = map_state.NewLightSourceTintingState()
	}

	// Get the light sources that were computed during visibility calculation
	lightSources := layer.GetLastComputedLightSources()

	// Build the lit tiles map from light sources, filtered by visibility
	// This ensures light from behind walls doesn't bleed through
	g.LightSourceTintingState.BuildLitTilesMap(lightSources, layer.GetVisibleFlags())

	// Set large map mode based on current map type
	isLargeMap := g.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType
	g.LightSourceTintingState.SetLargeMapMode(isLargeMap)

	// Inject lighthouse tiles if active (lighthouse uses unified tinting system)
	// Only inject at night when lighthouse beam is visible
	if g.LighthouseState != nil && g.LighthouseState.HasLighthouses() && !g.DateTime.IsFullDaylight() {
		g.LightSourceTintingState.InjectLighthouseTiles(g.LighthouseState)
	}

	// Add player-carried light (torch or magic light spell)
	playerLightType := g.GetPlayerLightType()
	if playerLightType == map_state.PlayerLightNone && g.AmbientTintDarkness() > 0 {
		// Surface dusk starts cosmetic ambient light before night visibility.
		playerLightType = map_state.PlayerLightAmbient
	}
	g.LightSourceTintingState.SetAmbientLightRadius(g.MapState.Lighting.AmbientLightRadius())
	g.LightSourceTintingState.UpdatePlayerLight(
		g.MapState.PlayerLocation.Position,
		playerLightType,
	)
	g.updateDaylightTinting(layer)
}

// GetPlayerLightType returns the current player light type based on torch and magic light state.
// Exported for use by the visibility system to apply consistent light radius for both
// visibility calculation and color tinting.
func (g *GameState) GetPlayerLightType() map_state.PlayerLightType {
	// Magic light takes precedence (brighter, white light)
	if g.MapState.Lighting.HasMagicLight() {
		return map_state.PlayerLightInLor
	}

	// Torch gives warm orange light
	if g.MapState.Lighting.HasTorchLit() {
		return map_state.PlayerLightTorch
	}

	// At night or in dark areas without a torch, avatar has minimal ambient vision
	// This prevents tiles from being full brightness when they should be dimmed
	// REDUX DEVIATION: LIGHT-0008: ordinary Underworld exploration keeps the
	// configured ambient pool at every hour, independent of surface daylight.
	if (g.MapState.IsUnderworld() && !g.IsInCombat()) || !g.DateTime.IsVisibilityDaylight() {
		return map_state.PlayerLightAmbient
	}

	return map_state.PlayerLightNone
}

// ClearLightSourceTinting clears the light source tinting state.
// Called when transitioning between maps or to locations without light sources.
func (g *GameState) ClearLightSourceTinting() {
	if g.LightSourceTintingState != nil {
		g.LightSourceTintingState.Clear()
	}
	if g.daylightTinting != nil {
		g.daylightTinting.Clear()
	}
	g.daylightLayer = nil
}

// IsHMSCapeActivated returns whether HMS Cape Plans have been used.
func (g *GameState) IsHMSCapeActivated() bool {
	return g.hmsCapeActivated
}

// SetHMSCapeActivated sets the HMS Cape Plans activation state.
func (g *GameState) SetHMSCapeActivated(activated bool) {
	g.hmsCapeActivated = activated
}

// Centralized RNG methods preserve shared gameplay draw order and bounds.

// SetRandomSeed sets the seed for the centralized RNG (for testing and deterministic behavior)
func (g *GameState) SetRandomSeed(seed uint64) {
	if g.rng == nil {
		g.rng = rng.New(seed)

		return
	}
	g.rng.Reset(seed)
}

// RandomSeed returns the seed most recently configured for the centralized RNG.
// It reports reproducibility metadata, not the RNG's current internal position.
func (g *GameState) RandomSeed() uint64 {
	if g.rng == nil {
		return 0
	}

	return g.rng.Seed()
}

// GetRNG returns the centralized RNG for advanced operations (e.g., Fisher-Yates shuffling)
// Use this when you need direct RNG access that can't be achieved with helper methods
func (g *GameState) GetRNG() *rng.Source {
	return g.rng
}

// OneInXOdds returns true with probability 1/odds.
func (g *GameState) OneInXOdds(odds int) bool {
	if odds <= 0 {
		return false
	}

	return g.rng.Intn(odds) == 0
}

// RollDice rolls a dice with the specified number of sides (1 to sides)
func (g *GameState) RollDice(sides int) int {
	if sides <= 0 {
		return 1
	}

	return g.rng.Intn(sides) + 1
}

// RandomIntInRange returns a random integer in [min, max] inclusive.
func (g *GameState) RandomIntInRange(minimum, maximum int) int {
	if minimum > maximum {
		panic(fmt.Sprintf("RandomIntInRange: min (%d) cannot be greater than max (%d)", minimum, maximum))
	}

	return g.rng.Intn(maximum-minimum+1) + minimum
}

// ShuffleDirections shuffles a slice of positions for deterministic randomness in AI movement
func (g *GameState) ShuffleDirections(directions []references.Position) {
	g.rng.Shuffle(len(directions), func(i, j int) {
		directions[i], directions[j] = directions[j], directions[i]
	})
}

// IsNPCDead implements ai.NPCDeadChecker interface.
// Checks if an NPC at a specific location is marked as dead.
// This enables filtering out dead NPCs (freed prisoners, killed NPCs) when loading a location.
func (g *GameState) IsNPCDead(location references.Location, npcID int) bool {
	return g.PartyState.IsNPCDead(location, npcID)
}

// Active Player Management Methods

// SetActivePlayer sets the active player (0-5) or -1 for all/none
// Returns false if the index is invalid or character is not in party
// In combat, also enables solo move mode for the selected character
func (g *GameState) SetActivePlayer(index int) bool {
	if !g.isValidActivePlayerIndex(index) {
		return false
	}

	g.activePlayerIndex = index

	return true
}

func (g *GameState) isValidActivePlayerIndex(index int) bool {
	if index == -1 {
		return true
	}
	if index < 0 || index >= party_state.MaxActiveParty || !g.PartyState.HasCharacter(index) {
		return false
	}
	character := g.PartyState.GetCharacter(index)

	return character.PartyMap == party_state.PartyMapActive &&
		character.GetCurrentHp() > 0 && !character.IsDead() &&
		!character.IsAsleep() && character.GetNameAsString() != ""
}

func (g *GameState) clearUnavailableActivePlayer() {
	if g.activePlayerIndex < 0 || g.activePlayerIndex >= party_state.MaxActiveParty ||
		g.isValidActivePlayerIndex(g.activePlayerIndex) {
		return
	}

	g.activePlayerIndex = -1
}

// GetActivePlayer returns the currently active player or nil if none/all selected
func (g *GameState) GetActivePlayer() *party_state.PlayerCharacter {
	g.clearUnavailableActivePlayer()
	if g.activePlayerIndex < 0 || g.activePlayerIndex >= party_state.MaxActiveParty {
		return nil
	}

	return g.PartyState.GetCharacter(g.activePlayerIndex)
}

// GetActivePlayerIndex returns the active player index (-1 for none/all, 0-5 for specific character)
func (g *GameState) GetActivePlayerIndex() int {
	g.clearUnavailableActivePlayer()

	return g.activePlayerIndex
}

// ClearActivePlayer sets the active player to none/all (-1)
func (g *GameState) ClearActivePlayer() {
	g.SetActivePlayer(-1)
}

// HasActivePlayer returns true if a specific player is currently active (not all/none)
func (g *GameState) HasActivePlayer() bool {
	return g.GetActivePlayer() != nil
}

// IsSoloMoveMode returns true if in combat with solo move mode active.
// In solo mode, only one character moves per round.
func (g *GameState) IsSoloMoveMode() bool {
	if !g.IsInCombat() || g.CombatScene == nil {
		return false
	}

	return g.CombatScene.IsSoloMoveActive()
}

// GetSoloMoveCharacterIndex returns the solo character index in combat.
// Returns -1 if not in solo mode or not in combat.
func (g *GameState) GetSoloMoveCharacterIndex() int {
	if !g.IsSoloMoveMode() {
		return -1
	}

	return g.CombatScene.GetSoloMoveCharacter()
}

// IsInCamping returns true if the player is currently in camping mode
// This is determined by checking if the player location is CampingLocation
func (g *GameState) IsInCamping() bool {
	return g.MapState.PlayerLocation.Location == references.CampingLocation
}

// TransitionToCampingMap loads the camping map and sets up camping mode
func (g *GameState) TransitionToCampingMap() error {
	campingMap, err := references.LoadCampingMap(g.GameConfig)
	if err != nil {
		return err
	}
	g.resetHarpsichordMelody()

	return g.transitionToCampingMap(campingMap)
}

func (g *GameState) transitionToCampingMap(campingMap *references.CombatMapReference) error {
	if campingMap == nil {
		return errors.New("camping map is required")
	}
	// Store pre-camping location for restoration
	g.PreCampingLocation = g.MapState.PlayerLocation.Location

	// Create LayeredMap for combat map type
	g.MapState.LayeredMaps.ResetAndCreateCombatMap(
		campingMap,
		g.GameReferences.TileReferences,
		g.MapState.XTilesVisibleOnGameScreen,
		g.MapState.YTilesVisibleOnGameScreen,
	)

	// Set location to CampingLocation (virtual location for camping)
	g.MapState.PlayerLocation.Location = references.CampingLocation
	g.MapState.PlayerLocation.Floor = 0 // Combat maps are always floor 0

	// Set player position to center of camping map (5, 5 in 11x11 grid)
	// This is critical! Visibility system expects coordinates within the camping map bounds
	g.MapState.PlayerLocation.Position = references.Position{X: 5, Y: 5}

	logging.Debugf(logging.CategoryGameState, "Transitioned to camping map: %s", campingMap.MapName)

	return nil
}

// TransitionFromCampingMap clears camping mode and returns to normal map view
func (g *GameState) TransitionFromCampingMap() {
	// Restore pre-camping location
	g.MapState.PlayerLocation.Location = g.PreCampingLocation

	logging.Debugf(logging.CategoryGameState, "Transitioned from camping map back to normal view")
}

// IsInShrine returns true if the player is currently in shrine meditation mode
// This is determined by checking if the player location is ShrineLocation
func (g *GameState) IsInShrine() bool {
	return g.MapState.PlayerLocation.Location == references.ShrineLocation
}

// IsInCodex returns true if the player is currently in Codex reading mode
// This is determined by checking if the player location is CodexLocation
func (g *GameState) IsInCodex() bool {
	return g.MapState.PlayerLocation.Location == references.CodexLocation
}

// IsInMiniMap returns true if the player is in any tile-based mini-map (shrine, codex, etc.)
func (g *GameState) IsInMiniMap() bool {
	return g.IsInShrine() || g.IsInCodex() || g.MapState.PlayerLocation.Location == references.FinaleLocation
}

// TransitionToShrineMap loads the shrine map from MISCMAPS.DAT and sets up shrine meditation mode.
func (g *GameState) TransitionToShrineMap(shrineIndex int) error {
	return g.transitionToMiniMap(shrine.MiniMapShrine, shrineIndex, references.ShrineLocation)
}

// TransitionToCodexMap loads the Codex map from MISCMAPS.DAT and sets up Codex reading mode.
func (g *GameState) TransitionToCodexMap() error {
	return g.transitionToMiniMap(shrine.MiniMapCodex, -1, references.CodexLocation)
}

// transitionToMiniMap is a generic helper for transitioning to tile-based mini-maps.
// Used by both shrine and codex (and future mini-map types).
func (g *GameState) transitionToMiniMap(mapType shrine.MiniMapType, sceneIndex int, location references.Location) error {
	// Load mini-map from MISCMAPS.DAT
	mapData, err := shrine.LoadMiniMapFromMiscmaps(g.GameConfig, mapType)
	if err != nil {
		return fmt.Errorf("loading mini-map type %d: %w", mapType, err)
	}

	// Store pre-transition location only after the transition is known to be viable.
	g.PreShrineLocation = g.MapState.PlayerLocation.Location
	g.resetHarpsichordMelody()

	// Create the scene with loaded map data
	originalPlayerLocation := g.MapState.PlayerLocation
	g.ShrineScene = shrine.NewShrineSceneWithMap(sceneIndex, originalPlayerLocation, mapData)

	// Create LayeredMap using raw tile data
	var tileData [references.XCombatMapTiles][references.YCombatMapTiles]byte
	for x := range shrine.MiniMapSize {
		for y := range shrine.MiniMapSize {
			tileData[x][y] = mapData.Tiles[x][y]
		}
	}
	g.MapState.LayeredMaps.ResetAndCreateMiniMapFromData(
		&tileData,
		g.GameReferences.TileReferences,
		g.MapState.XTilesVisibleOnGameScreen,
		g.MapState.YTilesVisibleOnGameScreen,
	)

	// Set location to virtual location for this mini-map type
	g.MapState.PlayerLocation.Location = location
	g.MapState.PlayerLocation.Floor = 0 // Mini-maps are always floor 0

	// The visual actor starts at the entrance; authoritative map rendering remains centered.
	g.MapState.PlayerLocation.Position = references.Position{X: shrine.CenterX, Y: shrine.AltarY}

	logging.Debugf(logging.CategoryGameState, "Transitioned to mini-map type %d", mapType)

	return nil
}

// TransitionFromShrineMap clears shrine mode and returns to normal map view
func (g *GameState) TransitionFromShrineMap() {
	if g.ShrineScene == nil {
		logging.Warnf(logging.CategoryGameState, "TransitionFromShrineMap called with nil ShrineScene")

		return
	}

	// Restore pre-shrine location
	g.MapState.PlayerLocation = g.ShrineScene.GetOriginalPlayerLocation()

	// Clear the shrine scene
	g.ShrineScene = nil

	logging.Debugf(logging.CategoryGameState, "Transitioned from shrine map back to normal view")
}

// GetCharacterSpriteForCamping returns the appropriate sprite for a character during camping
func (g *GameState) GetCharacterSpriteForCamping() indexes.SpriteIndex {
	// For now, use basic player sprite - can be enhanced with character-specific sprites later
	// The avatar sprite system handles different character types
	return indexes.SpriteIndex(indexes.DeadBody)
}

// GetGuardSpriteForCamping returns the sprite for a guard during camping (standing alert)
func (g *GameState) GetGuardSpriteForCamping(guard *party_state.PlayerCharacter, _ int) indexes.SpriteIndex {
	// Guard uses the default character sprite - special animations handled by SpriteOverrideManager
	return guard.GetKeySpriteIndex()
}

// eventQueueSystemCallbacks adapts GameState SystemCallbacks to event_queue.SystemCallbacks interface
type eventQueueSystemCallbacks struct {
	gameState *GameState
}

// BlockAllInput implements the event_queue.SystemCallbacks interface
func (eq *eventQueueSystemCallbacks) BlockAllInput(block bool) {
	if eq.gameState.SystemCallbacks != nil && eq.gameState.SystemCallbacks.Visual.BlockAllInput != nil {
		eq.gameState.SystemCallbacks.Visual.BlockAllInput(block)
	}
}

// IsAnyKeyPressed implements the event_queue.SystemCallbacks interface
func (eq *eventQueueSystemCallbacks) IsAnyKeyPressed() bool {
	if eq.gameState.SystemCallbacks != nil && eq.gameState.SystemCallbacks.Visual.IsAnyKeyPressed != nil {
		return eq.gameState.SystemCallbacks.Visual.IsAnyKeyPressed()
	}

	return false
}

// GetPressedKey implements the event_queue.SystemCallbacks interface
func (eq *eventQueueSystemCallbacks) GetPressedKey() *semantic_input.Key {
	if eq.gameState.SystemCallbacks != nil && eq.gameState.SystemCallbacks.Visual.GetPressedKey != nil {
		return eq.gameState.SystemCallbacks.Visual.GetPressedKey()
	}

	return nil
}

// SetCriminalStatus marks the player as wanted by guards in the current location.
// When true, guards will attempt arrest instead of extortion.
// Criminal status does NOT persist across map transitions - guards forget when you leave.
func (g *GameState) SetCriminalStatus(wanted bool) {
	g.isCriminal = wanted
	if wanted {
		logging.Infof(logging.CategoryGameState, "Player marked as criminal in current location")
	}
}

// IsCriminal returns true if the player is currently wanted by guards.
// Used by guard AI to determine whether to arrest or extort.
func (g *GameState) IsCriminal() bool {
	return g.isCriminal
}

// ClearCriminalStatus clears the wanted status.
// Called after arrest (jail) or when leaving the map (guards forget).
// Exposed as public for use in inlined event queue patterns (guard extortion flow).
func (g *GameState) ClearCriminalStatus() {
	g.isCriminal = false
	logging.Debugf(logging.CategoryGameState, "Criminal status cleared")
}

// OnMapTransition should be called when the player changes maps.
// Clears criminal status as guards don't remember crimes across locations.
func (g *GameState) OnMapTransition() {
	if g.isCriminal {
		logging.Debugf(logging.CategoryGameState,
			"Map transition - guards no longer remember crimes")
		g.ClearCriminalStatus()
	}
}

// Shrine System Getter/Setter Methods
// These methods provide controlled access to shrine state.
// Direct field access is only allowed during initial deserialization from save file.

// IsShrineQuestCompleted returns true if the quest for the specified shrine has been completed.
// A quest is "completed" when the player has visited the Codex and received the answer.
// This is separate from "active" - a quest can be completed but still active (ready for shrine completion).
func (g *GameState) IsShrineQuestCompleted(shrineIndex int) bool {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return false
	}

	return g.shrineQuestsCompleted[shrineIndex]
}

// SetShrineQuestCompleted sets the completion status for a shrine quest.
// Called by the Codex when revealing the answer to the player.
func (g *GameState) SetShrineQuestCompleted(shrineIndex int, completed bool) {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return
	}

	if g.shrineQuestsCompleted[shrineIndex] != completed {
		g.shrineQuestsCompleted[shrineIndex] = completed
		g.SystemCallbacks.UI.JournalChanged(journal.VirtuesTab, shrineIndex)
	}
}

// IsShrineQuestActive returns true if the quest for the specified shrine is currently active.
// A quest becomes active when assigned at the shrine, and is cleared when completed at the shrine.
func (g *GameState) IsShrineQuestActive(shrineIndex int) bool {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return false
	}

	return g.shrineQuestsActive[shrineIndex]
}

// SetShrineQuestActive sets the active status for a shrine quest.
// Called when assigning a quest (true) or completing a quest at the shrine (false).
func (g *GameState) SetShrineQuestActive(shrineIndex int, active bool) {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return
	}

	if g.shrineQuestsActive[shrineIndex] != active {
		g.shrineQuestsActive[shrineIndex] = active
		g.SystemCallbacks.UI.JournalChanged(journal.VirtuesTab, shrineIndex)
	}
}

// IsShrineRuined returns true if the specified shrine is ruined (closed by Blackthorn).
// Ruined shrines cannot be entered and must be restored by yelling the Word of Power.
func (g *GameState) IsShrineRuined(shrineIndex int) bool {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return false
	}

	return g.closedShrines[shrineIndex]
}

// SetShrineRuined sets the ruined status for a shrine.
// Shrines are ruined by failing Blackthorn's interrogation, restored by yelling Word of Power.
func (g *GameState) SetShrineRuined(shrineIndex int, ruined bool) {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return
	}

	if g.closedShrines[shrineIndex] != ruined {
		g.closedShrines[shrineIndex] = ruined
		g.SystemCallbacks.UI.JournalChanged(journal.VirtuesTab, shrineIndex)
	}
}

// GetShrineQuestState returns the current quest state for a shrine.
// Returns one of: QuestStateNotAssigned, QuestStateReadyToComplete, QuestStateOffering
//
// State logic :
// - If completed=false: Quest not assigned (assign quest)
// - If completed=true and active=true: Ready to complete (give bonuses)
// - If completed=true and active=false: Offering mode (accept gold)
// See ref:MOON-0003.
func (g *GameState) GetShrineQuestState(shrineIndex int) int {
	if shrineIndex < 0 || shrineIndex >= TotalShrines {
		return QuestStateNotAssigned
	}

	completed := g.IsShrineQuestCompleted(shrineIndex)
	active := g.IsShrineQuestActive(shrineIndex)

	if !completed {
		return QuestStateNotAssigned // Assign quest (active doesn't matter for this check)
	}

	if !active {
		return QuestStateOffering // completed=true, active=false → gold offerings
	}

	return QuestStateReadyToComplete // completed=true, active=true → complete quest
}

// GetFirstActiveQuest returns the index of the first shrine with an active quest, or -1 if none.
// Used by the Codex to determine which quest answer to display.
func (g *GameState) GetFirstActiveQuest() int {
	for i := range TotalShrines {
		if g.IsShrineQuestActive(i) {
			return i
		}
	}

	return -1
}

// AreAllQuestsComplete returns true if all 8 shrine quests have been completed.
// Used to trigger the endgame sequence at the Codex.
func (g *GameState) AreAllQuestsComplete() bool {
	for i := range TotalShrines {
		if !g.IsShrineQuestCompleted(i) {
			return false
		}
	}

	return true
}
