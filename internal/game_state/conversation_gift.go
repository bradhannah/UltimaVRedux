package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

// Original conversation grant IDs (ref:TALK-0003): IDs below
// ConversationEquipmentLimit are equipment, and IDs from ConversationProvisionFirst
// are provisions and tools at the offsets below.
const (
	ConversationEquipmentLimit = 0x40
	ConversationProvisionFirst = 0x41
)

// Offsets within the conversation provision/tool grant block.
const (
	ConversationGiftFood = iota
	ConversationGiftGold
	ConversationGiftKeys
	ConversationGiftGems
	ConversationGiftTorches
	ConversationGiftGrapple
	ConversationGiftCarpet
	ConversationGiftSextant
	ConversationGiftSpyglass
	ConversationGiftBadge
	ConversationGiftSkullKeys
)

// GiftOutcome says what a conversation grant did.
type GiftOutcome int

const (
	// GiftUnknown means the grant ID names nothing; the inventory is untouched.
	GiftUnknown GiftOutcome = iota
	// GiftGranted means one item was added.
	GiftGranted
	// GiftAtLimit means the party already carries the most it can; nothing was added.
	GiftAtLimit
)

// GiveConversationItem adds one item named by an original conversation grant
// ID and returns its display name and what happened. A gift at its carrying
// limit grants nothing.
func (g *GameState) GiveConversationItem(itemID int) (string, GiftOutcome) {
	name, quantity, grant := g.conversationGift(itemID)
	if quantity == nil {
		return "", GiftUnknown
	}
	before := quantity()
	grant()
	if quantity() == before {
		return name, GiftAtLimit
	}

	return name, GiftGranted
}

// conversationGift resolves a grant ID to its name, a quantity reader and the
// grant itself. The reader is nil for an unknown ID.
func (g *GameState) conversationGift(itemID int) (string, func() uint16, func()) {
	inventory := &g.PartyState.Inventory
	if itemID < ConversationEquipmentLimit {
		equipment := references.Equipment(itemID)

		return equipment.String(), func() uint16 { return inventory.Equipment.Get(equipment) },
			func() { inventory.Equipment.IncrementByOne(equipment) }
	}
	provision := func(name string, quantity interface {
		Get() uint16
		IncrementByOne()
	},
	) (string, func() uint16, func()) {
		return name, quantity.Get, quantity.IncrementByOne
	}
	questItem := func(item references.QuestItem, name string) (string, func() uint16, func()) {
		if name == "" {
			name = g.GameReferences.InventoryItemReferences.GetQuestItemDisplayName(item)
		}

		return name, func() uint16 { return inventory.QuestItems.Get(item) },
			func() { inventory.QuestItems.IncrementByOne(item) }
	}
	switch itemID - ConversationProvisionFirst {
	case ConversationGiftFood:
		return provision("Food", &inventory.Provisions.Food)
	case ConversationGiftGold:
		return provision("Gold", &inventory.Gold)
	case ConversationGiftKeys:
		return provision("Keys", &inventory.Provisions.Keys)
	case ConversationGiftGems:
		return provision("Gems", &inventory.Provisions.Gems)
	case ConversationGiftTorches:
		return provision("Torches", &inventory.Provisions.Torches)
	case ConversationGiftSkullKeys:
		return provision("Skull Keys", &inventory.Provisions.SkullKeys)
	case ConversationGiftGrapple:
		return questItem(references.Grapple, "")
	case ConversationGiftCarpet:
		return questItem(references.Carpet, "")
	case ConversationGiftSpyglass:
		return questItem(references.Spyglass, "Spyglass")
	case ConversationGiftBadge:
		return questItem(references.BlackBadge, "Black Badge")
	case ConversationGiftSextant:
		name, quantity, _ := questItem(references.Sextant, "Sextant")

		return name, quantity, g.GrantSextant
	default:
		return "", nil, nil
	}
}
