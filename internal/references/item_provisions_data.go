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
		Key:       {ItemName: "Keys", ItemDescription: "Use Jimmy with a regular key on ordinary locked doors, chests, stocks, or manacles. A successful physical unlock keeps the key; failure breaks one. Magical locks always break it."},
		SkullKeys: {ItemName: "SkullKeys", ItemDisplayName: "Skull Keys", ItemDescription: "Opens a magically locked door, as In Ex Por does. Works outdoors, in towns and in battle. The key is spent even if there is nothing to open."},
		Food:      {ItemName: "Food", ItemDescription: "The party's shared rations. Everyone awake eats three times a day. When the food runs out, the party starves and takes harm."},
		Gold:      {ItemName: "Gold", ItemDescription: "Shared coin used for goods, lodging, healing, ships, offerings, tolls, and other payments. Gold is spent when a transaction or payment is accepted."},
	}
	for provision, item := range items {
		item.ItemSprite = provision.GetSpriteIndex()
		items[provision] = item
	}

	return items
}
