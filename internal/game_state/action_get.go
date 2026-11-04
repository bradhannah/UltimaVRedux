package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/journal"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ActionGetSmallMap collects an adjacent town item through its domain pickup handler.
func (g *GameState) ActionGetSmallMap(direction references.Direction) bool {
	getThingPos := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if getThingPos == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}
	getThingTile := g.MapState.LayeredMaps.GetTileTopMapOnlyTileByPosition(references.SmallMapType, getThingPos, g.MapState.PlayerLocation.Floor)
	if getThingTile == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	mapLayers := g.MapState.LayeredMaps.GetLayeredMap(references.SmallMapType, g.MapState.PlayerLocation.Floor)

	// Check for item stacks first
	if g.ItemStacksMap.HasItemStackAtPosition(getThingPos) {
		return g.getFromItemStack(getThingPos)
	}

	if g.isSandalwoodBoxAt(getThingPos) {
		return g.getSandalwoodBox()
	}
	if carpet := g.magicCarpetAt(getThingPos); carpet != nil {
		return g.getMagicCarpet(carpet)
	}

	// Check for quest items (Crown, Sceptre) at position before checking tiles
	// These are now rendered as overlays instead of NPCs
	if hasQuestItem, questItem := g.IsQuestItemAtPosition(*getThingPos); hasQuestItem {
		// Only Crown and Sceptre can be in small maps (dungeons)
		if questItem == references.Crown || questItem == references.Sceptre {
			return g.getQuestItemFromOverlay(questItem)
		}
	}

	//nolint:exhaustive // Only collectible terrain has a Get handler; all other sprites report nothing to get.
	switch getThingTile.Index {
	case indexes.WheatInField:
		g.SystemCallbacks.UI.ShowActionFeedback("Crops picked!", true)
		// REDUX DEVIATION: original Search/Get/Open/Jimmy rules silently lowers karma. Explain the theft separately for clarity.
		// See ref:KARMA-0006.
		g.SystemCallbacks.Message.AddRowStr("Those crops are not yours, Avatar.")
		mapLayers.SetTileByLayer(map_state.MapLayer, getThingPos, indexes.PlowedField)
		g.PartyState.Inventory.Provisions.Food.IncrementByOne()
		// REDUX DEVIATION: object pickup has no generic pickup sound.
		// Redux exposes committed acquisition through an empty authored slot.
		// See ref:KARMA-0006.
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundItemPickup)
		g.AdjustPartyKarma(-1)
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true

	case indexes.RightSconce, indexes.LeftSconce:
		mapLayers.SetTileByLayer(map_state.MapLayer, getThingPos, indexes.BrickFloor)
		g.lightBorrowedWallTorch()
		g.SystemCallbacks.UI.ShowActionFeedback(wallTorchBorrowedMessage, true)
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true

	case indexes.TableFoodBoth, indexes.TableFoodBottom, indexes.TableFoodTop:
		remaining, reachable := tableFoodAfterPickup(direction, getThingTile.Index)
		if reachable {
			mapLayers.SetTileByLayer(map_state.MapLayer, getThingPos, remaining)
			g.SystemCallbacks.UI.ShowActionFeedback(tableFoodTakenMessage, true)
			g.takeUnownedTableFood()
			g.AdvanceOrdinarySurfaceCommandMinutes(1)

			return true
		}

		g.SystemCallbacks.UI.ShowActionFeedback(tableFoodUnreachableMessage, true)

		return false

	case indexes.Moon:
		// Getting a found moonstone (discovered via search)
		return g.getMoonstoneFromPosition(getThingPos)

	case indexes.Crown, indexes.Sceptre, indexes.Amulet:
		// Getting quest items (Crown, Sceptre, Amulet of Lord British)
		return g.getQuestItem(getThingPos, getThingTile, mapLayers)

	case indexes.Shard:
		// Shards should not appear on small maps - they're only in the underworld (large map)
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Nothing to get
	g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

	return false
}

func (g *GameState) magicCarpetAt(position *references.Position) *map_units.NPCFriendly {
	vehicle := g.CurrentNPCAIController.GetNpcs().GetVehicleAtPositionAndFloorOrNil(
		*position,
		g.MapState.PlayerLocation.Floor,
	)
	if vehicle == nil || vehicle.GetVehicleDetails().VehicleType != references.CarpetVehicle ||
		vehicle.GetVehicleDetails().IsOwnedByNPC {
		return nil
	}

	return vehicle
}

func (g *GameState) getMagicCarpet(carpet *map_units.NPCFriendly) bool {
	if err := g.PartyState.Inventory.PutItemInInventory(&references.ItemAndQuantity{
		Item: references.Carpet, Quantity: 1,
	}); err != nil {
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Can't take: %v", err))

		return false
	}

	// Original Search/Get/Open/Jimmy rules remove the runtime Castle object only; it does not set persistent NPC absence.
	// See ref:KARMA-0006.
	if !g.CurrentNPCAIController.GetNpcs().RemoveNPCByNumber(carpet.MapUnitDetails().NPCNum) {
		panic("collected magic carpet could not be removed")
	}
	g.CurrentNPCAIController.FreshenExistingNPCsOnMap()
	g.SystemCallbacks.Message.AddRowStr("A magic carpet!")
	g.recordQuestItemAcquired(references.Carpet)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

func (g *GameState) getFromItemStack(position *references.Position) bool {
	item := g.ItemStacksMap.Peek(position)
	if moonstone, isMoonstone := item.Item.(references.Moonstone); isMoonstone {
		if !g.getMoonstoneFromItemStack(position, moonstone, item) {
			return false
		}
		g.ItemStacksMap.Pop(position)

		return true
	}

	quantity := g.surfaceLootCollectibleQuantity(*item)
	if quantity == 0 {
		g.SystemCallbacks.Message.AddRowStr("Can't take: inventory full!")

		return false
	}
	collected := references.ItemAndQuantity{Item: item.Item, Quantity: quantity}
	if err := g.PartyState.Inventory.PutItemInInventory(&collected); err != nil {
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Can't take: %v", err))

		return false
	}
	if quantity == item.Quantity {
		g.ItemStacksMap.Pop(position)
	} else {
		item.Quantity -= quantity
	}
	itemRef := g.GameReferences.InventoryItemReferences.GetReferenceByItem(item.Item)
	pickupMessage := itemRef.FullDisplayName() + "!"
	if questItem, quest := item.Item.(references.QuestItem); quest {
		pickupMessage = g.GameReferences.InventoryItemReferences.GetQuestItemPickupMessage(questItem)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	} else {
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundItemPickup)
	}
	g.SystemCallbacks.UI.ShowActionFeedback(pickupMessage, true)
	if questItem, ok := item.Item.(references.QuestItem); ok {
		g.recordQuestItemAcquired(questItem)
	}
	if item.Item == references.SkullKeys && g.MapState.PlayerLocation.Location == references.Minoc &&
		g.MapState.PlayerLocation.Floor == 0 && position.X == 2 && position.Y == 2 {
		g.recordItemAcquired("minoc-odd-keys")
	}
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

func (g *GameState) isSandalwoodBoxAt(position *references.Position) bool {
	if g.MapState.PlayerLocation.Location != references.LordBritishsCastle ||
		g.MapState.PlayerLocation.Floor != lordBritishHarpsichordFloor {
		return false
	}

	mapUnit := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		*position,
		g.MapState.PlayerLocation.Floor,
	)
	if mapUnit == nil {
		return false
	}
	friendly, ok := (*mapUnit).(*map_units.NPCFriendly)

	return ok && friendly.MapUnitDetails().NPCNum == lordBritishSandalwoodBoxNPC &&
		friendly.NPCReference.GetSpriteIndex() == indexes.ItemSandalwoodBox
}

func (g *GameState) getSandalwoodBox() bool {
	if !g.acquireSandalwoodBox() {
		g.removeSandalwoodBoxObject()
		g.SystemCallbacks.UI.ShowActionFeedback("Already have that!", true)

		return false
	}

	g.removeSandalwoodBoxObject()
	g.SystemCallbacks.Message.AddRowStr("A sandalwood box!")
	g.recordQuestItemAcquired(references.WoodenBox)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

func (g *GameState) acquireSandalwoodBox() bool {
	g.lordBritishPassageOpen = true
	g.PartyState.SetDeadNpc(references.LordBritishsCastle, lordBritishSandalwoodBoxNPC)
	if g.PartyState.Inventory.QuestItems.HasSome(references.WoodenBox) {
		g.applyLordBritishPassageState()

		return false
	}
	g.PartyState.Inventory.QuestItems.SetHasOne(references.WoodenBox)
	g.applyLordBritishPassageState()

	return true
}

func (g *GameState) removeSandalwoodBoxObject() {
	g.CurrentNPCAIController.GetNpcs().RemoveNPCByNumber(lordBritishSandalwoodBoxNPC)
	g.CurrentNPCAIController.FreshenExistingNPCsOnMap()
}

const (
	// wallTorchBorrowedMessage matches the original acknowledgement; the party is lit, nothing is pocketed.
	wallTorchBorrowedMessage    = "Borrowed!"
	tableFoodTakenMessage       = "Mmmmm...! But that food isn't yours!"
	tableFoodUnreachableMessage = "Can't reach plate!"
)

// tableFoodAfterPickup resolves the plate a diner can reach from direction and
// the table tile left behind. Plates are reachable only across the table's
// north or south edge. See ref:KARMA-0006.
func tableFoodAfterPickup(direction references.Direction, table indexes.SpriteIndex) (indexes.SpriteIndex, bool) {
	//nolint:exhaustive // Table food can be taken only from its north or south edge.
	switch direction {
	case references.Down:
		switch table { //nolint:exhaustive // Only the plate tiles reach this helper.
		case indexes.TableFoodBoth:
			return indexes.TableFoodBottom, true
		case indexes.TableFoodTop:
			return indexes.TableMiddle, true
		}
	case references.Up:
		switch table { //nolint:exhaustive // Only the plate tiles reach this helper.
		case indexes.TableFoodBoth:
			return indexes.TableFoodTop, true
		case indexes.TableFoodBottom:
			return indexes.TableMiddle, true
		}
	}

	return table, false
}

// takeUnownedTableFood commits the shared food gain and karma loss of eating
// from a plate that is not the party's. See ref:KARMA-0006.
func (g *GameState) takeUnownedTableFood() {
	g.PartyState.Inventory.Provisions.Food.IncrementByOne()
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundItemPickup)
	g.AdjustPartyKarma(-1)
}

// lightBorrowedWallTorch applies the shared effect of taking a wall torch: the
// party carries a lit torch for a fixed burn and no inventory torch is gained.
// Callers replace the sconce tile on their own map. See ref:LIGHT-0004.
func (g *GameState) lightBorrowedWallTorch() {
	g.MapState.Lighting.LightBorrowedWallTorch()
	g.GetCurrentLayeredMap().InvalidateLightSourceCache()
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundItemPickup)
}

// getMoonstoneFromItemStack handles getting a found moonstone from an item stack
// Legacy exposed items may still have a buried placement record. The exact item
// and full local source must agree before inventory and physical state commit.
func (g *GameState) getMoonstoneFromItemStack(pos *references.Position, moonstone references.Moonstone, item *references.ItemAndQuantity) bool {
	moonstoneIndex := int(moonstone)

	site := g.localMoonstoneSite(*pos)
	moonstoneLocation := g.moonstoneSystem.GetMoonstoneByIndex(moonstoneIndex)
	if moonstoneLocation == nil || !moonstoneLocation.CanCreateMoongate() ||
		moonstoneLocation.Map != uint8(site.Map) || moonstoneLocation.Level != uint8(site.Floor) ||
		int(moonstoneLocation.X) != site.X || int(moonstoneLocation.Y) != site.Y ||
		item.Quantity != 1 || g.PartyState.Inventory.Moonstones.HasSome(moonstone) {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Add moonstone to inventory
	if err := g.PartyState.Inventory.PutItemInInventory(item); err != nil {
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Can't take: %v", err))

		return false
	}

	// Transition moonstone to inventory state
	g.moonstoneSystem.MarkMoonstoneFoundAtPosition(moonstoneIndex)
	g.moonstoneSystem.TakeMoonstoneToInventory(moonstoneIndex)
	g.observeMoonstone(moonstoneIndex, site, journal.MoonstoneSourcePickup)

	// Display original game message for moonstone pickup
	g.SystemCallbacks.UI.ShowActionFeedback("A moonstone!", true)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

// getMoonstoneFromPosition handles getting a found moonstone (Moon sprite) from a position
// Transitions moonstone from FoundNotCollected to InInventory state
func (g *GameState) getMoonstoneFromPosition(pos *references.Position) bool {
	// Find moonstone at this position in FoundNotCollected state
	moonstoneIndex := -1
	location := g.MapState.PlayerLocation
	for index, stone := range g.moonstoneSystem.Moonstones {
		if stone.IsFoundNotCollected() && stone.Map == uint8(location.Location) &&
			stone.Level == uint8(location.Floor) && int(stone.X) == int(pos.X) && int(stone.Y) == int(pos.Y) {
			moonstoneIndex = index

			break
		}
	}
	if moonstoneIndex < 0 {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Get the moonstone reference for inventory
	moonstone := g.moonstoneSystem.GetMoonstoneByIndex(moonstoneIndex)
	if moonstone == nil || !moonstone.IsFoundNotCollected() {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Add moonstone to inventory
	moonstoneItem := references.Moonstone(moonstoneIndex) // Use moonstone index as item type
	itemAndQuantity := &references.ItemAndQuantity{
		Item:     moonstoneItem,
		Quantity: 1,
	}

	return g.getMoonstoneFromItemStack(pos, moonstoneItem, itemAndQuantity)
}

// getQuestItem handles getting quest items (Crown, Sceptre, Amulet of Lord British)
// Removes item from world, adds to inventory, shows special message
func (g *GameState) getQuestItem(getThingPos *references.Position, getThingTile *references.Tile, mapLayers *map_state.LayeredMap) bool {
	// Determine which quest item this is
	var questItem references.QuestItem
	//nolint:exhaustive // Only these three world sprites represent tile-backed royal artifacts.
	switch getThingTile.Index {
	case indexes.Crown:
		questItem = references.Crown
	case indexes.Sceptre:
		questItem = references.Sceptre
	case indexes.Amulet:
		questItem = references.Amulet
	default:
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Check if player already has this quest item
	if g.PartyState.Inventory.QuestItems.HasSome(questItem) {
		g.SystemCallbacks.UI.ShowActionFeedback("Already have that!", true)

		return false
	}

	// Add quest item to inventory
	itemAndQuantity := &references.ItemAndQuantity{
		Item:     questItem,
		Quantity: 1,
	}

	if err := g.PartyState.Inventory.PutItemInInventory(itemAndQuantity); err != nil {
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Can't take: %v", err))

		return false
	}

	// Replace quest item sprite with floor tile
	mapLayers.SetTileByLayer(map_state.MapLayer, getThingPos, indexes.BrickFloor)

	// Display special pickup message
	g.SystemCallbacks.Message.AddRowStr(g.GameReferences.InventoryItemReferences.GetQuestItemPickupMessage(questItem))
	g.recordQuestItemAcquired(questItem)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

// getShardOfEvil handles getting Shards of Evil (all use CannonUp sprite 0xb4)
// Uses placement logic to determine which shard type and handles collection tracking
func (g *GameState) getShardOfEvil(getThingPos *references.Position, _ *map_state.LayeredMap) bool {
	// Check if there should be a shard at this position
	hasShardHere, questItem := g.IsShardAtPosition(*getThingPos)
	if !hasShardHere || !g.IsShardVisible(*getThingPos, questItem) {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Check if player already has this shard
	if g.PartyState.Inventory.QuestItems.HasSome(questItem) {
		g.SystemCallbacks.UI.ShowActionFeedback("Already have that!", true)

		return false
	}

	// Add shard to inventory
	itemAndQuantity := &references.ItemAndQuantity{
		Item:     questItem,
		Quantity: 1,
	}

	if err := g.PartyState.Inventory.PutItemInInventory(itemAndQuantity); err != nil {
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Can't take: %v", err))

		return false
	}

	// Shard collection tracking is handled automatically via inventory

	// Note: No need to replace tile since shards are now rendered as overlays
	// The underlying grass tile is already there

	// Display special pickup message using quest item method
	g.SystemCallbacks.Message.AddRowStr(g.GameReferences.InventoryItemReferences.GetQuestItemPickupMessage(questItem))
	g.recordQuestItemAcquired(questItem)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

// getQuestItemFromOverlay handles getting quest items from overlay positions (Crown, Sceptre, Amulet)
// Similar to getQuestItem but works with overlay system instead of tile-based
func (g *GameState) getQuestItemFromOverlay(questItem references.QuestItem) bool {
	// Check if player already has this quest item
	if g.PartyState.Inventory.QuestItems.HasSome(questItem) {
		g.SystemCallbacks.UI.ShowActionFeedback("Already have that!", true)

		return false
	}

	// Add quest item to inventory
	itemAndQuantity := &references.ItemAndQuantity{
		Item:     questItem,
		Quantity: 1,
	}

	if err := g.PartyState.Inventory.PutItemInInventory(itemAndQuantity); err != nil {
		g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("Can't take: %v", err))

		return false
	}

	// Note: No need to replace tile since quest items are overlays
	// The underlying floor tile is already there

	// Display special pickup message
	g.SystemCallbacks.Message.AddRowStr(g.GameReferences.InventoryItemReferences.GetQuestItemPickupMessage(questItem))
	g.recordQuestItemAcquired(questItem)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

// ActionGetLargeMap collects an adjacent loose item, artifact or discovered moonstone.
func (g *GameState) ActionGetLargeMap(direction references.Direction) bool {
	getThingPos := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if getThingPos == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}

	// Check for item stacks first (from search results)
	if g.ItemStacksMap.HasItemStackAtPosition(getThingPos) {
		return g.getFromItemStack(getThingPos)
	}
	if carpet := g.magicCarpetAt(getThingPos); carpet != nil {
		return g.getMagicCarpet(carpet)
	}

	// Check tile at position for large map specific items
	getThingTile := g.MapState.LayeredMaps.GetTileTopMapOnlyTileByPosition(references.LargeMapType, getThingPos, g.MapState.PlayerLocation.Floor)
	if getThingTile == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

		return false
	}
	mapLayers := g.MapState.LayeredMaps.GetLayeredMap(references.LargeMapType, g.MapState.PlayerLocation.Floor)

	// Check for quest items (shards + Amulet) first - they are now rendered as overlays, not tiles
	// So we need to check by position rather than tile index
	if hasQuestItem, questItem := g.IsQuestItemAtPosition(*getThingPos); hasQuestItem {
		// Handle shards and Amulet (the only quest items that can be in large maps)
		//nolint:exhaustive // Only shards and the Amulet have large-map overlay pickups.
		switch questItem {
		case references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice:
			return g.getShardOfEvil(getThingPos, mapLayers)
		case references.Amulet:
			return g.getQuestItemFromOverlay(questItem)
		}
	}

	if getThingTile.Index == indexes.Moon {
		// Getting a found moonstone (discovered via search)
		return g.getMoonstoneFromPosition(getThingPos)
	}

	// Nothing to get
	g.SystemCallbacks.UI.ShowActionFeedback("Nothing to get!", true)

	return false
}

// CombatPickupResult reports committed inventory movement for a compact receipt.
// Succeeded distinguishes a pickup that changed the world without adding stock,
// such as a borrowed wall torch, from a refusal.
type CombatPickupResult struct {
	Handled   bool
	Succeeded bool
	Message   string
	Sprite    indexes.SpriteIndex
	Acquired  uint16
	ItemType  references.ItemType
	ItemID    int
	ItemName  string
}

// getCombatItem retains original object ordering while returning truthful feedback.
func (g *GameState) getCombatItem(direction references.Direction) CombatPickupResult {
	result := CombatPickupResult{Message: "Nothing to get!", Sprite: indexes.NoSprites}
	if !g.IsInCombat() || g.CombatScene == nil {
		return result
	}

	combatantPosition := g.getActiveCombatantPosition()
	targetPosition := direction.GetNewPositionInDirection(&combatantPosition)
	if targetPosition == nil {
		return result
	}
	object, found := g.CombatScene.firstGettableObjectAt(*targetPosition)
	if !found {
		return g.getCombatTerrain(*targetPosition, direction, result)
	}
	result.Sprite = object.Sprite()
	if object.IsChest() {
		g.SystemCallbacks.Message.AddRowStr("Open it first!")
		result.Handled, result.Message = true, "Open it first!"

		return result
	}

	item, looseItem := object.LooseItem()
	if !looseItem {
		return result
	}
	result.ItemType, result.ItemID = item.Item.Type(), item.Item.ID()
	result.ItemName = g.GameReferences.InventoryItemReferences.GetReferenceByItem(item.Item).FullDisplayName()
	before := g.combatPickupQuantity(item.Item)
	message, err := g.collectCombatItem(item)
	if err != nil {
		result.Handled, result.Message = true, fmt.Sprintf("Could not collect item: %v", err)
		g.SystemCallbacks.Message.AddRowStr(result.Message)

		return result
	}
	result.Acquired = g.combatPickupQuantity(item.Item) - before
	result.Succeeded = result.Acquired > 0
	if _, unique := item.Item.(references.QuestItem); !unique {
		name := g.GameReferences.InventoryItemReferences.GetReferenceByItem(item.Item).FullDisplayName()
		message = fmt.Sprintf("Picked up %s +%d", name, result.Acquired)
		if result.Acquired == 0 {
			message = "No room for " + name + "."
		}
	}

	g.CombatScene.takeFirstObjectAt(*targetPosition, func(candidate CombatObject) bool {
		candidateItem, looseItem := candidate.LooseItem()

		return looseItem && candidateItem.Item.Type() == item.Item.Type() &&
			candidateItem.Item.ID() == item.Item.ID() && candidateItem.Quantity == item.Quantity
	})
	g.SystemCallbacks.Message.AddRowStr(message)

	result.Handled, result.Message = true, message

	return result
}

// getCombatTerrain applies the shared terrain pickups to the tactical map once
// no object claims the square: wall torches and reachable table food. Crops do
// not occur on combat maps. See ref:COMBAT-0010, ref:LIGHT-0004, ref:KARMA-0006.
func (g *GameState) getCombatTerrain(
	target references.Position,
	direction references.Direction,
	result CombatPickupResult,
) CombatPickupResult {
	if !combatAdjacentPositionInBounds(target) {
		return result
	}
	tile := g.CombatScene.CombatMapRef.GetTileNumber(target)
	switch tile { //nolint:exhaustive // Only collectible terrain has a tactical Get handler.
	case indexes.RightSconce, indexes.LeftSconce:
		g.setCombatTerrain(target, indexes.BrickFloor)
		g.lightBorrowedWallTorch()
		result.Handled, result.Succeeded = true, true
		result.Message, result.Sprite, result.ItemName = wallTorchBorrowedMessage, indexes.ItemTorch, "Wall torch"
	case indexes.TableFoodBoth, indexes.TableFoodBottom, indexes.TableFoodTop:
		remaining, reachable := tableFoodAfterPickup(direction, tile)
		result.Handled = true
		if !reachable {
			result.Message = tableFoodUnreachableMessage

			break
		}
		before := g.combatPickupQuantity(references.Food)
		g.setCombatTerrain(target, remaining)
		g.takeUnownedTableFood()
		result.Succeeded, result.Message = true, tableFoodTakenMessage
		result.Acquired = g.combatPickupQuantity(references.Food) - before
		result.ItemType, result.ItemID = references.Food.Type(), references.Food.ID()
		result.ItemName = g.GameReferences.InventoryItemReferences.GetReferenceByItem(references.Food).FullDisplayName()
		result.Sprite = references.Food.GetSpriteIndex()
	default:
		return result
	}
	g.SystemCallbacks.Message.AddRowStr(result.Message)

	return result
}

func (g *GameState) combatPickupQuantity(item references.Item) uint16 {
	inv := &g.PartyState.Inventory
	switch value := item.(type) {
	case references.Equipment:
		return inv.Equipment.Get(value)
	case references.Potion:
		return inv.Potions.Get(value)
	case references.Scroll:
		return inv.Scrolls.Get(value)
	case references.Reagent:
		return inv.Reagent.Get(value)
	case references.QuestItem:
		return inv.QuestItems.Get(value)
	case references.Provision:
		return inv.LockerSnapshot().Provisions[value]
	default:
		return 0
	}
}

func (g *GameState) collectCombatItem(item references.ItemAndQuantity) (string, error) {
	questItem, unique := item.Item.(references.QuestItem)
	if !unique {
		if err := g.PartyState.Inventory.PutItemInInventory(&item); err != nil {
			return "", err
		}
		itemReference := g.GameReferences.InventoryItemReferences.GetReferenceByItem(item.Item)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundItemPickup)

		return fmt.Sprintf("Got %d %s!", item.Quantity, itemReference.FullDisplayName()), nil
	}

	acquired := false
	if questItem == references.WoodenBox {
		acquired = g.acquireSandalwoodBox()
	} else if !g.PartyState.Inventory.QuestItems.HasSome(questItem) {
		g.PartyState.Inventory.QuestItems.SetHasOne(questItem)
		acquired = true
	}
	if !acquired {
		return "Already have that!", nil
	}
	g.recordQuestItemAcquired(questItem)
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundQuestPickup)

	return g.GameReferences.InventoryItemReferences.GetQuestItemPickupMessage(questItem), nil
}

// ActionGetDungeonMap takes an open chest from the party's current dungeon cell.
func (g *GameState) ActionGetDungeonMap() bool {
	return g.actionGetDungeonMap()
}
