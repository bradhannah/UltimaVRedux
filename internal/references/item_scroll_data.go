package references

import "strings"

// newScrollItemReferences owns player-facing scroll metadata. A description says
// what the scroll is for and where it is useless, without the arithmetic.
func newScrollItemReferences() map[Scroll]InventoryItem {
	items := map[Scroll]InventoryItem{
		ScrollVasLor:     {ItemName: "Vas_Lor", ItemSprite: 260, ItemDescription: "Fills the dark with strong magical light for several hours.", EffectDuration: 240},
		ScrollRelHur:     {ItemName: "Rel_Hur", ItemSprite: 260, ItemDescription: "Turns the wind the way you choose, or stills it. No use in dungeons or in battle."},
		ScrollInSanct:    {ItemName: "In_Sanct", ItemSprite: 260, ItemDescription: "Shields the whole party for a short while. The time starts at once, so read it close to the fight.", EffectDuration: 100},
		ScrollInAn:       {ItemName: "In_An", ItemSprite: 260, ItemDescription: "Silences all magic in battle for a short while: the enemy's, and yours.", EffectDuration: 20},
		ScrollInQuasWis:  {ItemName: "In_Quas_Wis", ItemSprite: 260, ItemDescription: "Shows the land around you from above, or a map of the dungeon level. No use in battle."},
		ScrollKalXenCorp: {ItemName: "Kal_Xen_Corp", ItemSprite: 260, ItemDescription: "In battle, calls a daemon to fight for you, if there is room for it. No use elsewhere."},
		ScrollInManiCorp: {ItemName: "In_Mani_Corp", ItemSprite: 260, ItemDescription: "Raises one dead companion, barely alive. Not in battle. Unless your karma is very high, they return with less experience."},
		ScrollAnTym:      {ItemName: "An_Tym", ItemSprite: 260, ItemDescription: "Stops time for a short while: everything freezes but your party. It does not work everywhere.", EffectDuration: 20},
	}
	for scroll, item := range items {
		item.ItemDisplayName = strings.ReplaceAll(item.ItemName, "_", " ") + " Scroll"
		items[scroll] = item
	}

	return items
}
