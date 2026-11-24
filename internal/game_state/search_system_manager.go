package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// reagentCacheLocations defines three midnight caches: Mandrake at
// (182,54) and (97,165), Nightshade at (44,137). ref:MIX-0001
func reagentCacheLocations() [3]ReagentCacheLocation {
	return [3]ReagentCacheLocation{
		// Index 0: Mandrake root at (0xb6, 0x36) = (182, 54)
		{
			X: 182, Y: 54,
			ReagentType: references.MandrakeRoot,
			MinQuantity: 2, MaxQuantity: 15,
			Message:       "mandrake root!",
			ReagentOffset: 7, // offsetr[0] = 0x07
		},
		// Index 1: Mandrake root at (0x61, 0xa5) = (97, 165)
		{
			X: 97, Y: 165,
			ReagentType: references.MandrakeRoot,
			MinQuantity: 2, MaxQuantity: 15,
			Message:       "mandrake root!",
			ReagentOffset: 7, // offsetr[1] = 0x07
		},
		// Index 2: Nightshade at (0x2c, 0x89) = (44, 137)
		{
			X: 44, Y: 137,
			ReagentType: references.NightShade,
			MinQuantity: 2, MaxQuantity: 15,
			Message:       "nightshade!",
			ReagentOffset: 6, // offsetr[2] = 0x06
		},
	}
}

// ReagentCacheLocation represents a reagent cache location from original Search/Get/Open/Jimmy rules reagent-cache search
// See ref:INV-0008.
type ReagentCacheLocation struct {
	X, Y          references.Coordinate // Coordinates from xreag[], yreag[]
	ReagentType   references.Reagent    // Type of reagent (Mandrake or Nightshade)
	MinQuantity   int                   // Minimum quantity to find
	MaxQuantity   int                   // Maximum quantity to find
	Message       string                // Message to display when found
	ReagentOffset int                   // Offset in reagent array (from offsetr[])
}

// SearchSystemManager manages all search operations with a clear priority-based system
type SearchSystemManager struct {
	gameState                *GameState
	authenticLocationManager *AuthenticSearchLocationManager
	reagentCachesFoundDay    [3]byte // Tracks the day each reagent cache was last found for daily regeneration
}

func (ssm *SearchSystemManager) snapshotState() ([24]byte, byte, [3]byte) {
	found, skullDay := ssm.authenticLocationManager.snapshotState()

	return found, skullDay, ssm.reagentCachesFoundDay
}

func (ssm *SearchSystemManager) restoreState(found [24]byte, skullDay byte, reagentDays [3]byte) {
	ssm.authenticLocationManager.restoreState(found, skullDay)
	ssm.reagentCachesFoundDay = reagentDays
}

func (ssm *SearchSystemManager) resetDailyMarkersAtMonthBoundary() {
	ssm.authenticLocationManager.skullKeysFoundDay = 0
	ssm.reagentCachesFoundDay = [3]byte{}
}

// NewSearchSystemManager creates a new search system manager
func NewSearchSystemManager(gameState *GameState) *SearchSystemManager {
	ssm := &SearchSystemManager{
		gameState: gameState,
	}
	ssm.authenticLocationManager = NewAuthenticSearchLocationManager(gameState)

	return ssm
}

// GetAuthenticLocationManager returns the authentic search location manager
func (ssm *SearchSystemManager) GetAuthenticLocationManager() *AuthenticSearchLocationManager {
	return ssm.authenticLocationManager
}

// ExecuteSearchWithCharacter performs a source-ordered surface search for the selected actor.
func (ssm *SearchSystemManager) ExecuteSearchWithCharacter(direction references.Direction, characterIndex int) SearchResponse {
	if !ssm.gameState.isSurfaceSearchCharacterEligible(characterIndex) {
		return SearchResponse{Result: SearchResultNotFound, Message: "No one able!", TimeSpent: 0}
	}
	// Validate search position and get basic information
	searchPos, topTile, err := ssm.validateSearchPosition(direction)
	if err != nil {
		return *err
	}

	// Check each search type in priority order
	// The ORDER here defines the priority - no need for numeric constants

	// Priority 1: Always check chests first (highest priority items)
	if resp := ssm.checkChests(searchPos, characterIndex); resp != nil {
		return *resp
	}

	// Priority 2: Then corpses
	if resp := ssm.checkCorpses(searchPos, characterIndex); resp != nil {
		return *resp
	}

	// Original terrain prelude is selected before these checks, but the first
	// successful handler owns construction of the complete response.
	if resp := ssm.checkSecretDoors(searchPos, topTile); resp != nil {
		return *resp
	}

	if resp := ssm.checkMoonstones(searchPos); resp != nil {
		return *resp
	}

	if resp := ssm.checkReagentCaches(searchPos); resp != nil {
		return *resp
	}

	if resp := ssm.checkSpecialAreas(searchPos); resp != nil {
		return *resp
	}

	if resp := ssm.checkNormalSearchableAreas(searchPos); resp != nil {
		return *resp
	}

	// Handle edge cases: existing items on map or search locations that prevent finding
	if resp := ssm.handleSearchEdgeCases(searchPos); resp != nil {
		return *resp
	}

	// Default: nothing found
	return ssm.buildNothingFoundResponse()
}

// validateSearchPosition performs common validation for search operations
func (ssm *SearchSystemManager) validateSearchPosition(direction references.Direction) (*references.Position, *references.Tile, *SearchResponse) {
	// Calculate search position
	searchPos := direction.GetNewPositionInDirection(&ssm.gameState.MapState.PlayerLocation.Position)
	if searchPos == nil {
		err := &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   MessageCantSearchThere,
			TimeSpent: 0,
		}

		return nil, nil, err
	}

	// Check bounds before attempting to get tile
	if ssm.gameState.IsOutOfBounds(*searchPos) {
		err := &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   MessageNothingThere,
			TimeSpent: 1,
		}

		return nil, nil, err
	}

	// Get tile at search position
	layeredMap := ssm.gameState.GetLayeredMapByCurrentLocation()
	if layeredMap == nil {
		err := &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   MessageMapNotAvailable,
			TimeSpent: 0,
		}

		return nil, nil, err
	}

	topTile := layeredMap.GetTopTile(searchPos)
	if topTile == nil {
		err := &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   MessageNothingThere,
			TimeSpent: 1,
		}

		return nil, nil, err
	}

	return searchPos, topTile, nil
}

// Priority 1: Assess authoritative surface chests without changing physical state.
func (ssm *SearchSystemManager) checkChests(searchPos *references.Position, characterIndex int) *SearchResponse {
	chest := ssm.gameState.surfaceChestAt(searchPos)
	if chest == nil {
		return nil
	}
	character := ssm.gameState.PartyState.GetCharacter(characterIndex)
	value := chest.SurfaceChestTreasureValue()
	assessment := ssm.gameState.inspectObjectChest(chest.SurfaceChestAssessment(), chest.SurfaceChestLocked(), value, int(character.Intelligence))
	chest.RestoreSurfaceChestAssessment(assessment)

	return &SearchResponse{
		Result:    SearchResultFoundItem,
		Message:   objectChestAssessmentMessage(assessment, value),
		TimeSpent: 1,
	}
}

func (ssm *SearchSystemManager) checkCorpses(searchPos *references.Position, characterIndex int) *SearchResponse {
	index, corpse := ssm.gameState.surfaceCorpseAt(*searchPos, references.SearchObjectDeadMonster)
	if corpse == nil {
		return nil
	}

	outcome := rollCombatCorpseSearch(ssm.gameState.RandomIntInRange)
	ssm.gameState.removeSurfaceCorpse(index)
	if outcome.poisoned {
		character := ssm.gameState.PartyState.GetCharacter(characterIndex)
		statusBefore := character.GetStatus()
		character.SetStatus(party_state.Poisoned)
		changes := transientStatusChanges{}
		changes.observePoison(statusBefore, character)
		ssm.gameState.publishTransientStatusChanges(changes)
		ssm.gameState.SystemCallbacks.Audio.PlaySoundEffect(SoundPoison)
	}
	if outcome.hasItem {
		item := references.ItemAndQuantity{Item: outcome.item, Quantity: outcome.quantity}
		if err := ssm.gameState.ItemStacksMap.PushUnder(searchPos, &references.ItemStack{Items: references.ItemStacks{item}}); err != nil {
			panic("transforming searched surface corpse: " + err.Error())
		}
	}
	ssm.gameState.SystemCallbacks.Screen.UpdateDisplay()

	return &SearchResponse{
		Result:    SearchResultFoundItem,
		Message:   MessageFoundPrefix + outcome.message,
		TimeSpent: 1,
	}
}

// Priority 3: Check for moonstones (buried at current map/level)
// Based on original Search/Get/Open/Jimmy rules buried-moonstone search which checks: x, y, level, AND map
// Note: Original game does NOT require active moongate to find buried moonstones
// See ref:INV-0008.
func (ssm *SearchSystemManager) checkMoonstones(searchPos *references.Position) *SearchResponse {
	// Original Search/Get/Open/Jimmy rules skips buried-moonstone search while the moongate tile covers the stone.
	// See ref:INV-0008.
	if ssm.gameState.IsMoongateActiveAt(*searchPos) {
		return nil
	}

	// Get current map and level from player location
	currentMap := uint8(ssm.gameState.MapState.PlayerLocation.Location)
	currentLevel := uint8(ssm.gameState.MapState.PlayerLocation.Floor)

	// Search for moonstone at position with proper map/level matching
	moonstoneIndex := ssm.gameState.moonstoneSystem.GetMoonstoneAtPositionOnMap(
		uint8(searchPos.X), uint8(searchPos.Y), currentLevel, currentMap)

	if moonstoneIndex >= 0 {
		if err := ssm.addMoonSpriteToWorld(searchPos, moonstoneIndex); err != nil {
			return &SearchResponse{
				Result: SearchResultNotFound, Message: MessageCantPlaceItems + err.Error(), TimeSpent: 1,
			}
		}
		ssm.gameState.moonstoneSystem.MarkMoonstoneFoundAtPosition(moonstoneIndex)
		ssm.gameState.observeMoonstone(moonstoneIndex, ssm.gameState.localMoonstoneSite(*searchPos), journal.MoonstoneSourceSearch)

		// Trigger screen update for visual feedback
		ssm.gameState.SystemCallbacks.Screen.UpdateDisplay()

		return &SearchResponse{
			Result:    SearchResultFoundMoonstone,
			Items:     nil,
			Message:   ssm.buildDiscoveryMessage(ssm.getTileLocationName(topTileAt(ssm.gameState, searchPos).Index), "a strange rock!"),
			TimeSpent: 1,
		}
	}

	return nil
}

// Priority 5: Check for secret doors
func (ssm *SearchSystemManager) checkSecretDoors(searchPos *references.Position, topTile *references.Tile) *SearchResponse {
	// Check for secret doors on walls
	if topTile.Is(indexes.StoneBrickWallSecret) {
		return ssm.handleSecretDoorSearch(searchPos, topTile)
	}

	return nil
}

// Priority 6: Check for normal one-time searchable areas
func (ssm *SearchSystemManager) checkNormalSearchableAreas(searchPos *references.Position) *SearchResponse {
	// Check for searchable tile types - tile-specific messages only
	layeredMap := ssm.gameState.GetLayeredMapByCurrentLocation()
	topTile := layeredMap.GetTopTile(searchPos)

	location := ssm.getTileLocationName(topTile.Index)
	if location != "" {
		// Tile is searchable but no items found at this location
		message := ssm.buildNothingFoundMessage(location)

		return &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   message,
			TimeSpent: 1,
		}
	}

	return nil
}

// handleSearchEdgeCases handles special cases for search locations and existing items
func (ssm *SearchSystemManager) handleSearchEdgeCases(searchPos *references.Position) *SearchResponse {
	onmap := ssm.gameState.MapState.PlayerLocation.Location
	level := ssm.gameState.MapState.PlayerLocation.Floor

	// Check for existing items first (applies to all cases)
	if ssm.gameState.ItemStacksMap.HasItemStackAtPosition(searchPos) {
		return &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   MessageAlreadyFoundItems,
			TimeSpent: 0,
		}
	}

	// Check if any search location exists at this position (regardless of whether items can be found)
	// This handles the case where a search location exists but daily logic prevents finding items
	hasSearchLocationAtPosition := ssm.hasSearchLocationAtPosition(searchPos.X, searchPos.Y, level, onmap)

	// If a search location exists at this position but daily logic prevented finding items,
	// return "nothing" instead of "already found items there"
	if hasSearchLocationAtPosition {
		return &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   MessageNothingOfNote,
			TimeSpent: 1,
		}
	}

	return nil
}

// buildNothingFoundResponse creates the default "nothing found" response
func (ssm *SearchSystemManager) buildNothingFoundResponse() SearchResponse {
	return SearchResponse{
		Result:    SearchResultNotFound,
		Message:   MessageNothingOfNote,
		TimeSpent: 1,
	}
}

// Message generation functions

// buildFoundMessage creates a "found items" message with location-specific prefix
func (ssm *SearchSystemManager) buildFoundMessage(location string, items []references.ItemAndQuantity) string {
	prefix := ssm.GetLocationPrefix(location)
	itemsText := ssm.FormatFoundItems(items)

	return fmt.Sprintf("%s%s", prefix, itemsText)
}

func (ssm *SearchSystemManager) buildDiscoveryMessage(location, discovery string) string {
	return ssm.GetLocationPrefix(location) + discovery
}

// buildNothingFoundMessage creates a "nothing found" message with location-specific prefix
func (ssm *SearchSystemManager) buildNothingFoundMessage(location string) string {
	prelude := searchLocationPrelude(location)
	if prelude == "" {
		return MessageNothingOfNote
	}

	// Original object-specific results contain authored line breaks;
	// Redux supplies prose and lets each output surface wrap to its measured width.
	return prelude + ", thou dost find nothing of note."
}

// GetLocationPrefix returns the appropriate prefix for different search locations
func (ssm *SearchSystemManager) GetLocationPrefix(location string) string {
	prelude := searchLocationPrelude(location)
	if prelude == "" {
		return MessageFoundPrefix
	}

	return prelude + "\nthou dost find\n"
}

func searchLocationPrelude(location string) string {
	switch location {
	case "well":
		return "Near the well"
	case "stump":
		return "In the stump"
	case "shelf":
		return "On the shelf"
	case "bookshelf":
		return "In the bookshelf"
	case "desk":
		return "In the desk"
	case "barrel":
		return "In the barrel"
	case "vanity":
		return "In the vanity"
	case "bed":
		return "Under the bed"
	case "dresser":
		return "In the dresser"
	case "trunk":
		return "In the trunk"
	case "fireplace":
		return "In the fireplace"
	case "brazier":
		return "In the brazier"
	case "wall":
		return "In the wall"
	default:
		return ""
	}
}

// FormatFoundItems creates the item description part of found messages
func (ssm *SearchSystemManager) FormatFoundItems(items []references.ItemAndQuantity) string {
	if len(items) == 0 {
		return "nothing of note."
	}

	if len(items) == 1 {
		item := items[0]
		switch item.Item {
		case references.Gold:
			if item.Quantity == 1 {
				return "1 gold coin!"
			}

			return fmt.Sprintf("%d gold coins!", item.Quantity)
		case references.Key:
			return "a ring of keys!"
		case references.Torches:
			if item.Quantity == 1 {
				return "a torch!"
			}

			return fmt.Sprintf("%d torches!", item.Quantity)
		case references.SkullKeys:
			if item.Quantity == 1 {
				return "a skull key!"
			}

			return fmt.Sprintf("%d skull keys!", item.Quantity)
		case references.GlassSword:
			if item.Quantity == 1 {
				return "a glass sword!"
			}

			return fmt.Sprintf("%d glass swords!", item.Quantity)
		case references.NightShade:
			if item.Quantity == 1 {
				return "1 sprig of nightshade!"
			}

			return fmt.Sprintf("%d sprigs of nightshade!", item.Quantity)
		case references.MandrakeRoot:
			if item.Quantity == 1 {
				return "mandrake root!"
			}

			return fmt.Sprintf("%d mandrake root!", item.Quantity)
		default:
			return fmt.Sprintf("an item! (type: %d, qty: %d)", item.Item.ID(), item.Quantity)
		}
	}

	return fmt.Sprintf("%d items!", len(items))
}

// getTileLocationName returns the location name for a given tile index, or
// an empty string when the tile has no searchable furniture name.
func (ssm *SearchSystemManager) getTileLocationName(tileIndex indexes.SpriteIndex) string {
	return map[indexes.SpriteIndex]string{
		indexes.DeadTree:       "stump",
		indexes.Bookcase:       "shelf",
		indexes.BookcaseLeft:   "bookshelf",
		indexes.BookcaseRight:  "bookshelf",
		indexes.Well:           "well",
		indexes.EndTable:       "desk",
		indexes.Barrel:         "barrel",
		indexes.Vanity:         "vanity",
		indexes.LeftBed:        "bed",
		indexes.RightBed:       "bed",
		indexes.Dresser:        "dresser",
		indexes.Box:            "trunk",
		indexes.Fireplace:      "fireplace",
		indexes.Brazier:        "brazier",
		indexes.StoneBrickWall: "wall",
	}[tileIndex]
}

func topTileAt(gameState *GameState, position *references.Position) *references.Tile {
	tile := gameState.GetLayeredMapByCurrentLocation().GetTopTile(position)
	if tile == nil {
		panic("validated search position has no tile")
	}

	return tile
}

// Helper functions for existing functionality

// addFoundItemsToMap adds found items to the ItemStacksMap for retrieval via Get command
func (ssm *SearchSystemManager) addFoundItemsToMap(pos *references.Position, items []references.ItemAndQuantity) error {
	// Create item stack from found items
	itemStack := &references.ItemStack{Items: items}

	// Add to ItemStacksMap at the search position
	// REDUX DEVIATION: Original outdoor object admission may evict another dynamic object
	// when all 23 slots are occupied. Redux retains existing world objects.
	// See ref:INV-0008.
	return ssm.gameState.ItemStacksMap.PushUnder(pos, itemStack)
}

// hasSearchLocationAtPosition checks if any search location exists at the given position
// This is used to distinguish between "no search location exists" vs "search location exists but daily logic prevents finding"
func (ssm *SearchSystemManager) hasSearchLocationAtPosition(x, y references.Coordinate, level references.FloorNumber, onmap references.Location) bool {
	allLocations := references.GetAllSearchLocations()

	for _, location := range allLocations {
		if references.Location(location.MapNumber) == onmap &&
			references.FloorNumber(location.Level) == level &&
			references.Coordinate(location.X) == x &&
			references.Coordinate(location.Y) == y {
			return true
		}
	}

	return false
}

// addMoonSpriteToWorld adds a moon sprite to represent a found moonstone
func (ssm *SearchSystemManager) addMoonSpriteToWorld(pos *references.Position, moonstoneIndex int) error {
	// Create a moonstone item with the correct index
	moonstoneItem := references.Moonstone(moonstoneIndex)
	itemAndQuantity := references.ItemAndQuantity{
		Item:     moonstoneItem,
		Quantity: 1,
	}

	itemStack := &references.ItemStack{
		Items: []references.ItemAndQuantity{itemAndQuantity},
	}

	return ssm.gameState.ItemStacksMap.PushUnder(pos, itemStack)
}

// handleSecretDoorSearch handles secret door discovery logic
func (ssm *SearchSystemManager) handleSecretDoorSearch(pos *references.Position, tile *references.Tile) *SearchResponse {
	// Check if this is actually a secret door wall
	if !tile.Is(indexes.StoneBrickWallSecret) {
		// This is a regular wall, not a secret door
		return &SearchResponse{
			Result:    SearchResultNotFound,
			Message:   ssm.buildNothingFoundMessage("wall"),
			TimeSpent: 1,
		}
	}

	// Found a secret door! Transform the tile
	currentMapType := ssm.gameState.MapState.PlayerLocation.Location.GetMapType()
	currentMap := ssm.gameState.MapState.LayeredMaps.GetLayeredMap(currentMapType, ssm.gameState.MapState.PlayerLocation.Floor)
	if currentMap != nil {
		// Determine the correct door type based on map type
		newTileIndex := indexes.LockedDoor
		if ssm.gameState.MapState.PlayerLocation.Location.GetMapType() == references.LargeMapType {
			newTileIndex = indexes.RegularDoor
		}

		// Add permanent override to MapOverrideLayer
		currentMap.SetTileByLayer(map_state.MapOverrideLayer, pos, indexes.SpriteIndex(newTileIndex))

		// Play unlock sound effect (discovering a secret door is like unlocking it)
		ssm.gameState.SystemCallbacks.Audio.Play(SoundUnlock).Start()

		// Trigger screen update
		ssm.gameState.SystemCallbacks.Screen.UpdateDisplay()

		return &SearchResponse{
			Result:    SearchResultFoundSecretDoor,
			Message:   ssm.buildDiscoveryMessage(ssm.getTileLocationName(tile.Index), "a hidden door!"),
			TimeSpent: 1,
		}
	}

	return &SearchResponse{
		Result:    SearchResultNotFound,
		Message:   ssm.buildNothingFoundMessage("wall"),
		TimeSpent: 1,
	}
}

// GetSearchLocationStatus returns the status of a search location for overlay rendering
func (ssm *SearchSystemManager) GetSearchLocationStatus(pos *references.Position) SearchLocationStatus {
	onmap := ssm.gameState.MapState.PlayerLocation.Location
	level := ssm.gameState.MapState.PlayerLocation.Floor

	// Check if there's a search location at this position
	allLocations := references.GetAllSearchLocations()
	for _, location := range allLocations {
		if references.Location(location.MapNumber) == onmap &&
			references.FloorNumber(location.Level) == level &&
			references.Coordinate(location.X) == pos.X &&
			references.Coordinate(location.Y) == pos.Y {
			// Check if this location has been found
			if ssm.authenticLocationManager.IsLocationFound(location.Index) {
				return SearchLocationAlreadyFound
			}

			return SearchLocationUnsearched
		}
	}

	return SearchLocationNotSearchable
}
