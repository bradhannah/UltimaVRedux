package references

import "github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"

// newMoonstoneItemReferences describes each stone by the moon it is marked with.
// The details come from UltimaCodex.com and the Ultima VI Compendium, and the
// night-time gate is what the game does; the wording is ours. Z-stats and Use
// show the same text.
func newMoonstoneItemReferences() map[Moonstone]InventoryItem {
	names := map[Moonstone]struct{ internal, display, mark string }{
		NewMoon:        {"NewMoon", "New Moon", "new moon"},
		CrescentWaxing: {"CrescentWaxing", "Crescent Waxing", "waxing crescent moon"},
		FirstQuarter:   {"FirstQuarter", "First Quarter", "first quarter moon"},
		GibbousWaxing:  {"GibbousWaxing", "Gibbous Waxing", "waxing gibbous moon"},
		FullMoon:       {"FullMoon", "Full Moon", "full moon"},
		GibbousWaning:  {"GibbousWaning", "Gibbous Waning", "waning gibbous moon"},
		LastQuarter:    {"LastQuarter", "Last Quarter", "last quarter moon"},
		CrescentWaning: {"CrescentWaning", "Crescent Waning", "waning crescent moon"},
	}
	items := make(map[Moonstone]InventoryItem, len(names))
	for phase, name := range names {
		items[phase] = InventoryItem{
			ItemName: name.internal, ItemDisplayName: name.display + " Moonstone", ItemSprite: indexes.Moon,
			ItemDescription: "A stone marked with the " + name.mark + ". Wherever it is buried, a moongate appears at night. " +
				"A traveller who enters any gate while this moon is overhead comes out where this stone is buried.",
			ItemDescriptionAttribution: creditUltimaCodex + " and Compendium (Ultima VI)",
		}
	}

	return items
}
