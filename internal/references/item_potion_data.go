package references

// potionColourCaveat follows every potion description: a draught may do something
// other than its colour suggests.
const potionColourCaveat = "A potion does not always do what its colour promises."

// newPotionItemReferences owns player-facing potion metadata. A description says
// what the draught is for and where it is useless, without the arithmetic.
func newPotionItemReferences() map[Potion]InventoryItem {
	items := map[Potion]InventoryItem{
		Blue:   {ItemName: "Blue", ItemSprite: 259, ItemDescription: "Wakes a sleeping companion."},
		Yellow: {ItemName: "Yellow", ItemSprite: 259, ItemDescription: "Heals some of a companion's wounds."},
		Red:    {ItemName: "Red", ItemSprite: 259, ItemDescription: "Cures a poisoned companion. It does not heal wounds."},
		Green:  {ItemName: "Green", ItemSprite: 259, ItemDescription: "Poisons whoever drinks it."},
		Orange: {ItemName: "Orange", ItemSprite: 259, ItemDescription: "Puts whoever drinks it to sleep."},
		Purple: {ItemName: "Purple", ItemSprite: 259, ItemDescription: "In battle, the drinker takes the shape of a rat until the fight ends. Does nothing elsewhere."},
		Black:  {ItemName: "Black", ItemSprite: 259, ItemDescription: "In battle, the drinker turns invisible until the fight ends, and most enemies cannot single them out. Does nothing elsewhere."},
		White:  {ItemName: "White", ItemSprite: 259, ItemDescription: "Shows everything nearby, even through walls. Does nothing in dungeons or in battle."},
	}
	for potion, item := range items {
		item.ItemDisplayName = item.ItemName + " Potion"
		item.ItemDescription += "\n\n" + potionColourCaveat
		items[potion] = item
	}

	return items
}
