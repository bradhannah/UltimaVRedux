package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// QuestItemPlacementData contains the location information for placing quest items (shards and artifacts)
type QuestItemPlacementData struct {
	QuestItemType references.QuestItem
	Position      references.Position
	Location      references.Location
	Floor         references.FloorNumber
}

// GetAllQuestItemPlacements returns placement information for all quest items (shards + artifacts)
//
//goland:noinspection GoLinter
func GetAllQuestItemPlacements() []QuestItemPlacementData {
	//goland:noinspection ALL
	return []QuestItemPlacementData{
		// Shards - Based on original Ultima V placement data
		// See ref:QUEST-0002.
		{
			QuestItemType: references.ShardOfFalsehood,
			Position:      references.Position{X: 192, Y: 80}, // 0xc0, 0x50 from original outdoor effect rules
			// See ref:QUEST-0002.
			Location: references.BritanniaUnderworld,
			Floor:    -1, // Underworld floor
		},
		{
			QuestItemType: references.ShardOfHatred,
			Position:      references.Position{X: 130, Y: 65}, // Northwest underworld area
			Location:      references.BritanniaUnderworld,     // Large map
			Floor:         -1,                                 // Underworld floor
		},
		{
			QuestItemType: references.ShardOfCowardice,
			Position:      references.Position{X: 176, Y: 184}, // Southeast underworld area
			Location:      references.BritanniaUnderworld,      // Large map
			Floor:         -1,                                  // Underworld floor
		},

		// === LORD BRITISH'S ARTIFACTS ===
		{
			QuestItemType: references.Sceptre,
			Position:      references.Position{X: 15, Y: 15}, // Center of Stonegate dungeon
			Location:      references.Stonegate,              // Small map dungeon
			Floor:         0,                                 // Ground floor
		},
		{
			QuestItemType: references.Crown,
			Position:      references.Position{X: 15, Y: 13}, // Throne room area
			Location:      references.PalaceOfBlackthorn,     // Small map palace
			Floor:         3,                                 // Third floor
		},
		{
			QuestItemType: references.Amulet,
			Position:      references.Position{X: 105, Y: 225}, // Central underworld area
			Location:      references.BritanniaUnderworld,      // Large map
			Floor:         -1,                                  // Underworld floor
		},
	}
}

// IsQuestItemVisible determines if a quest item should be visible at a given position
func (gs *GameState) IsQuestItemVisible(position references.Position, questItem references.QuestItem) bool {
	if gs.PartyState.Inventory.QuestItems.HasSome(questItem) {
		return false
	}
	if !IsShard(questItem) {
		return true
	}
	hasPlacement, placedItem := gs.IsQuestItemAtPosition(position)
	if !hasPlacement || placedItem != questItem {
		return false
	}
	if gs.ShadowlordState == nil {
		return false
	}

	lord := references.Shadowlord(questItem - references.ShardOfFalsehood)

	return gs.ShadowlordState.IsAlive(lord)
}

// IsShardVisible determines if a shard should be visible at a given position (backwards compatibility)
func (gs *GameState) IsShardVisible(position references.Position, questItem references.QuestItem) bool {
	return gs.IsQuestItemVisible(position, questItem)
}

// IsQuestItemAtPosition checks if there is a quest item placement at the given position
// Returns true if position matches a quest item location, regardless of visibility/inventory status
func (gs *GameState) IsQuestItemAtPosition(position references.Position) (bool, references.QuestItem) {
	placementData := GetAllQuestItemPlacements()

	// Check each quest item placement location
	for _, itemData := range placementData {
		if itemData.Position.X == position.X && itemData.Position.Y == position.Y {
			// Only check if we're in the correct location/floor
			if gs.MapState.PlayerLocation.Location == itemData.Location &&
				gs.MapState.PlayerLocation.Floor == itemData.Floor {
				return true, itemData.QuestItemType
			}
		}
	}

	return false, references.QuestItem(0)
}

// IsShardAtPosition checks if there is a shard placement at the given position (backwards compatibility)
func (gs *GameState) IsShardAtPosition(position references.Position) (bool, references.QuestItem) {
	hasItem, questItem := gs.IsQuestItemAtPosition(position)
	if !hasItem {
		return false, references.QuestItem(0)
	}
	// Only return true if it's actually a shard
	if questItem == references.ShardOfFalsehood ||
		questItem == references.ShardOfHatred ||
		questItem == references.ShardOfCowardice {
		return true, questItem
	}

	return false, references.QuestItem(0)
}

// Note: GetQuestItemSpriteIndex() has been moved to InventoryItemReferences.GetQuestItemSpriteIndex()
// It uses the shared quest-item definitions for consistency with inventory presentation.

// IsLordBritishArtifact checks if a quest item is one of Lord British's three artifacts
func IsLordBritishArtifact(questItem references.QuestItem) bool {
	return questItem == references.Crown || questItem == references.Sceptre || questItem == references.Amulet
}

// IsShard checks if a quest item is one of the three shards
func IsShard(questItem references.QuestItem) bool {
	return questItem == references.ShardOfFalsehood ||
		questItem == references.ShardOfHatred ||
		questItem == references.ShardOfCowardice
}

// GetVisibleQuestItemLights returns quest item light information for the current location
// Returns data needed for the lighting system to add quest items as light sources
func (gs *GameState) GetVisibleQuestItemLights() []map_state.QuestItemLightInfo {
	placements := GetAllQuestItemPlacements()
	var questItemLights []map_state.QuestItemLightInfo

	for _, placement := range placements {
		// Check if we're in the correct location and floor
		if gs.MapState.PlayerLocation.Location == placement.Location &&
			gs.MapState.PlayerLocation.Floor == placement.Floor {
			// Check if quest item is visible (not in inventory)
			if gs.IsQuestItemVisible(placement.Position, placement.QuestItemType) {
				// Determine light radius and marker index based on item type
				var lightRadius int
				var markerIndex = map_state.ShardMarkerIndex // Default to shard

				if IsLordBritishArtifact(placement.QuestItemType) {
					lightRadius = map_state.ArtifactLightDistance // Artifacts emit stronger light
					markerIndex = map_state.ArtifactMarkerIndex
				} else if IsShard(placement.QuestItemType) {
					lightRadius = map_state.ShardLightDistance // Shards emit moderate light
					markerIndex = map_state.ShardMarkerIndex
				}

				if lightRadius > 0 {
					questItemLights = append(questItemLights, map_state.QuestItemLightInfo{
						Position:    placement.Position,
						MarkerIndex: markerIndex,
						LightRadius: lightRadius,
					})
				}
			}
		}
	}

	return questItemLights
}
