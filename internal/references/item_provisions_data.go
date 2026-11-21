package references

// newProvisionItemReferences owns player-facing provisions metadata.
// Torch and gem commitment: ref:PROV-0003 and ref:PROV-0005.
// Keys, food, and gold: ref:PROV-0004, ref:PROV-0001, and ref:INV-0004.
// Skull Keys: ref:PROV-0006. A description says what the supply is for, without
// the arithmetic.
func newProvisionItemReferences() map[Provision]InventoryItem {
	items := map[Provision]InventoryItem{
		Torches:   {ItemName: "Torches", ItemDescription: "Lights the dark. One torch burns for a few hours, and for less time in a dungeon."},
		Gem:       {ItemName: "Gems", ItemDescription: "Peer into a gem to see the land around you from above, or a map of the dungeon level you can reach. Each look uses one gem. No use in battle."},
		Key:       {ItemName: "Keys", ItemDescription: "Jimmy an ordinary lock with one: a door, a chest, stocks or manacles. A key that fails breaks, and a magical lock always breaks it."},
		SkullKeys: {ItemName: "SkullKeys", ItemDisplayName: "Skull Keys", ItemDescription: "Opens a magically locked door, as In Ex Por does. Works outdoors, in towns and in battle. The key is spent even if there is nothing to open."},
		Food:      {ItemName: "Food", ItemDescription: "The party's shared rations. Everyone awake eats through the day; when the food runs out, the party starves."},
		Gold:      {ItemName: "Gold", ItemDescription: "The party's shared coin, for goods, lodging, healing, ships, offerings and tolls."},
	}
	for provision, item := range items {
		item.ItemSprite = provision.GetSpriteIndex()
		items[provision] = item
	}

	return items
}
