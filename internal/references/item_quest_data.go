package references

// Credits shown beside quest item descriptions. Every detail in a description
// comes from one of these sources or from behaviour the game implements; none
// is invented. The wording is ours, not a quotation.
const (
	creditBookOfLore     = "The Book of Lore (Ultima V)"
	creditPathsOfDestiny = "Paths of Destiny (Ultima V)"
	creditUltimaCodex    = "UltimaCodex.com"
	creditBookOfBrad     = "The Book of Brad"
)

// shardDescription names the force a shard of Mondain's gem carries.
func shardDescription(force string) string {
	return "A shard of the black gem with which Mondain sought to make himself immortal. " +
		"A hero shattered the gem long ago, but its evil did not die with it. " +
		"This shard carries the force of " + force + "."
}

// newQuestItemReferences owns player-facing quest metadata. A description says
// what the thing is and leaves how, where and why to use it for the player to
// learn from the people of Britannia. Z-stats and Use show the same text.
func newQuestItemReferences() map[QuestItem]InventoryItem {
	shardCredit := creditBookOfLore + " and " + creditUltimaCodex

	return map[QuestItem]InventoryItem{
		Amulet: {
			ItemName: "Amulet", ItemDisplayName: "Amulet of Lord British", ItemPickupMessage: "The Amulet of Lord British!", ItemSprite: 439,
			ItemDescription: "A silver serpent on a gold chain, one of the Crown Jewels, said to have come from another world. " +
				"It gives off a blue glow and is warm to the touch.",
			ItemDescriptionAttribution: creditUltimaCodex + " and " + creditBookOfLore,
		},
		Crown: {
			ItemName: "Crown", ItemDisplayName: "Crown of Lord British", ItemPickupMessage: "The Crown of Lord British!", ItemSprite: 437,
			ItemDescription: "The jewel-encrusted gold crown of Lord British, one of the three Crown Jewels, " +
				"believed to hold a perfect ray of sunlight.",
			ItemDescriptionAttribution: creditBookOfLore,
		},
		Sceptre: {
			ItemName: "Sceptre", ItemDisplayName: "Sceptre of Lord British", ItemPickupMessage: "The Sceptre of Lord British!", ItemSprite: 438,
			ItemDescription:            "The pure gold sceptre of Lord British, topped by a crystal orb, one of the three Crown Jewels.",
			ItemDescriptionAttribution: creditBookOfLore,
		},
		ShardOfFalsehood: {
			ItemName: "ShardOfFalsehood", ItemDisplayName: "Shard of Falsehood", ItemPickupMessage: "The Shard of Falsehood!", ItemSprite: 436,
			ItemDescription: shardDescription("Falsehood"), ItemDescriptionAttribution: shardCredit,
		},
		ShardOfHatred: {
			ItemName: "ShardOfHatred", ItemDisplayName: "Shard of Hatred", ItemPickupMessage: "The Shard of Hatred!", ItemSprite: 436,
			ItemDescription: shardDescription("Hatred"), ItemDescriptionAttribution: shardCredit,
		},
		ShardOfCowardice: {
			ItemName: "ShardOfCowardice", ItemDisplayName: "Shard of Cowardice", ItemPickupMessage: "The Shard of Cowardice!", ItemSprite: 436,
			ItemDescription: shardDescription("Cowardice"), ItemDescriptionAttribution: shardCredit,
		},
		Carpet: {
			ItemName: "Carpet", ItemDisplayName: "Magic Carpet", ItemPickupMessage: "A Magic Carpet!", ItemSprite: 283,
			ItemDescription: "Said to come from a faraway desert land, this carpet flies a handspan above the ground. " +
				"It is quick over swamp and calm water, but cannot pass mountains, walls or anything tall. " +
				"Rough water will soak it and endanger its riders.",
			ItemDescriptionAttribution: creditPathsOfDestiny,
		},
		Grapple: {
			ItemName: "Grapple", ItemDisplayName: "Grapple", ItemPickupMessage: "A Grappling Hook!", ItemSprite: 12,
			ItemDescription: "A grappling hook for climbing mountains. It makes the climb possible, not safe: " +
				"climbers can fall, and the highest peaks stay out of reach.",
			ItemDescriptionAttribution: creditPathsOfDestiny + " and " + creditUltimaCodex,
		},
		Spyglass: {
			ItemName: "Spyglass", ItemDisplayName: "Spyglass", ItemPickupMessage: "A Spyglass!", ItemSprite: 89,
			ItemDescription: "A small spyglass of the kind sailors navigate by. " +
				"At night it shows the planets and the brighter stars.",
			ItemDescriptionAttribution: creditPathsOfDestiny,
		},
		HMSCape: {
			ItemName: "HMSCape", ItemDisplayName: "H.M.S. Cape Plans", ItemPickupMessage: "Plans for the H.M.S. Cape!", ItemSprite: 260,
			ItemDescription: "Old plans for the H.M.S. Cape, the fastest vessel of her age. " +
				"They describe how a ship may be rigged to sail faster than any other of her class.",
			ItemDescriptionAttribution: creditUltimaCodex + " and " + creditPathsOfDestiny,
		},
		PocketWatch: {
			ItemName: "PocketWatch", ItemDisplayName: "Pocket Watch", ItemPickupMessage: "A Pocket Watch!", ItemSprite: 256,
			ItemDescription: "A pocket watch, one of the few things the Avatar brought to Britannia from Earth. " +
				"It shows the time of day.",
			ItemDescriptionAttribution: creditUltimaCodex,
		},
		BlackBadge: {
			ItemName: "BlackBadge", ItemDisplayName: "Black Badge", ItemPickupMessage: "A Black Badge!", ItemSprite: 256,
			ItemDescription: "A black badge, the mark of the Oppression.", ItemDescriptionAttribution: "",
		},
		WoodenBox: {
			ItemName: "WoodenBox", ItemDisplayName: "Sandalwood Box", ItemPickupMessage: "A sandalwood box!", ItemSprite: 270,
			ItemDescription:            "What a curious little box... You feel compelled to bring it along on your journey.",
			ItemDescriptionAttribution: creditBookOfBrad,
		},
		Sextant: {
			ItemName: "Sextant", ItemDisplayName: "Sextant", ItemPickupMessage: "A Sextant!", ItemSprite: 256,
			ItemDescription: "An instrument for navigation. By measuring the heavens it gives your latitude and longitude, " +
				"so you can find places that maps and guides name by their coordinates.",
			ItemDescriptionAttribution: creditUltimaCodex,
		},
	}
}
