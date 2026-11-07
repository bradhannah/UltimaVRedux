package game_state

import (
	"math"

	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// generateCombatChestContents admits the shared chest-loot roll in generation
// order and returns one reveal line per admitted object. The roll finishes
// before admission, so a full object list still consumes every draw.
// See ref:CMD-0009.
func (g *GameState) generateCombatChestContents(position references.Position, maximumValue int) []string {
	var revealLines []string
	for _, entry := range references.RollChestLoot(maximumValue, g.AsRandomProvider()) {
		if g.admitCombatChestLoot(position, entry) {
			revealLines = append(revealLines, combatChestRevealLine(entry))
		}
	}

	return revealLines
}

func (g *GameState) admitCombatChestLoot(position references.Position, entry references.ChestLoot) bool {
	if entry.ChestValue != 0 {
		return g.CombatScene.addLootObject(NewCombatChest(position, int(entry.ChestValue), false))
	}

	return g.addCombatChestLooseItem(position, entry.Item.Item, entry.Item.Quantity)
}

func combatChestRevealLine(entry references.ChestLoot) string {
	if entry.ChestValue != 0 {
		return "a chest!"
	}
	switch item := entry.Item.Item.(type) {
	case references.Equipment:
		return combatArmamentRevealLine(item)
	case references.Potion:
		return "a potion!"
	case references.Scroll:
		return "a scroll!"
	case references.Provision:
		return combatProvisionRevealLine(item)
	default:
		panic("combat chest generated an unknown item type")
	}
}

func combatProvisionRevealLine(provision references.Provision) string {
	switch provision {
	case references.Gold:
		return "a sack of gold!"
	case references.Key:
		return "a ring of keys!"
	case references.Gem:
		return "a gem!"
	case references.Torches:
		return "some torches!"
	case references.Food:
		return "some food!"
	case references.NoProvision, references.SkullKeys:
		panic("combat chest generated a provision outside the loot table")
	default:
		panic("combat chest generated an unknown provision")
	}
}

func combatArmamentRevealLine(equipment references.Equipment) string {
	switch equipment.GetCategory() {
	case references.CategoryHelm:
		return "a helm!"
	case references.CategoryShield:
		return "a shield!"
	case references.CategoryArmour:
		return "some armour!"
	case references.CategoryWeapon, references.CategoryAmmo:
		return "a weapon!"
	case references.CategoryRing:
		return "a ring!"
	case references.CategoryAmulet:
		return "an amulet!"
	case references.CategoryNone:
		panic("combat chest generated uncategorized equipment")
	default:
		panic("combat chest generated equipment with unknown category")
	}
}

func combatQuantity(value int) uint16 {
	if value < 1 || value > math.MaxUint16 {
		panic("combat item quantity is outside uint16 range")
	}

	return uint16(value)
}

func (g *GameState) addCombatChestLooseItem(position references.Position, item references.Item, quantity uint16) bool {
	itemReference := g.GameReferences.InventoryItemReferences.GetReferenceByItem(item)

	return g.CombatScene.addLootObject(NewCombatLooseItem(position, item, quantity, itemReference.ItemSprite))
}

func (g *GameState) triggerCombatChestTrap(pcIndex int) CombatChestTrapResult {
	trapType := TrapType(g.RandomIntInRange(int(TrapAcid), int(TrapPoison)))
	character := g.PartyState.GetCharacter(pcIndex)
	beforeHP := character.GetCurrentHp()

	g.SystemCallbacks.Message.AddRowStr("Trapped!")
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundTrapTrigger)
	switch trapType {
	case TrapAcid:
		g.SystemCallbacks.Message.AddRowStr("ACID!")
		damage := g.RollOriginalD30() // Failed-experiment trap selection calls transformed d30 for acid damage.
		// See ref:RNG-0001.
		character.SubtractHp(uint16(damage))
		if character.IsDead() {
			g.markCombatPCDead(pcIndex)
		}
	case TrapPoison:
		g.SystemCallbacks.Message.AddRowStr("POISON!")
		statusBefore := character.GetStatus()
		character.SetStatus(party_state.Poisoned)
		changes := transientStatusChanges{}
		changes.observePoison(statusBefore, character)
		g.publishTransientStatusChanges(changes)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundPoison)
	case TrapNone, TrapBomb, TrapGas:
		panic("combat chest selected an invalid trap type")
	default:
		panic("combat chest selected an unknown trap type")
	}
	result := CombatChestTrapResult{
		Type: trapType, ActorName: character.GetNameAsString(),
		HPLost: beforeHP - character.GetCurrentHp(), Fatal: character.IsDead(),
	}
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{Kind: CombatEventChestTrap, ChestTrap: result})

	return result
}
