package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// CombatCollectedItem is an encounter total of actual inventory additions, not
// treasure revealed or a before/after inventory balance. Order is first acquired.
type CombatCollectedItem struct {
	ItemType references.ItemType
	ItemID   int
	Name     string
	Sprite   indexes.SpriteIndex
	Quantity uint64
}

// CombatPickupEvent keeps identity and wording stable after the acting turn ends.
type CombatPickupEvent struct {
	ActorName string
	Position  references.Position
	Result    CombatPickupResult
}

// CombatChestTrapResult is detached from the victim, including fatal acid hits.
type CombatChestTrapResult struct {
	Type      TrapType
	ActorName string
	HPLost    uint16
	Fatal     bool
}

// combatGetAllLimit bounds one whole-pile Get. No square can hold this many
// objects, even when chests keep turning up inside chests.
const combatGetAllLimit = 4 * sourceCombatQueueSlots

// GetCombatItem publishes exactly one result and records only committed gains.
func (g *GameState) GetCombatItem(direction references.Direction) CombatPickupResult {
	return g.getAndReportCombatItem(direction, true)
}

// GetAllCombatItems clears the adjacent square in one action. It takes every
// loose item in the ordinary pickup order and reports each as its own pickup.
// A chest that came out of another chest is opened on the way, since such a
// chest is never trapped. Any other chest still answers "Open it first!", so
// its trap stays the player's own risk to take or to disarm.
//
// REDUX DEVIATION: COMBAT-0010: the original tactical Get takes one object per
// action and opens nothing. Redux adds this whole-pile form behind Ctrl+G so
// that looting what a chest spilled costs one action instead of one per item.
func (g *GameState) GetAllCombatItems(direction references.Direction) CombatPickupResult {
	if !g.IsInCombat() || g.CombatScene == nil {
		return g.GetCombatItem(direction)
	}
	position := g.getActiveCombatantPosition()
	target := direction.GetNewPositionInDirection(&position)
	if target == nil {
		return g.GetCombatItem(direction)
	}
	var result CombatPickupResult
	audible := true
	for range combatGetAllLimit {
		if g.openCombatChestFromChest(*target) {
			// An opened chest is an outcome even when it held nothing.
			result.Handled = true
		} else {
			before := len(g.CombatScene.objects)
			result = g.getAndReportCombatItem(direction, audible)
			audible = false
			if len(g.CombatScene.objects) >= before {
				break // Refused, or nothing was there.
			}
		}
		// Never ask an emptied square for more.
		if _, more := g.CombatScene.firstGettableObjectAt(*target); !more {
			break
		}
	}

	return result
}

// openCombatChestFromChest opens the chest next in line on the square when it
// came out of another chest, spilling its loot onto that square.
func (g *GameState) openCombatChestFromChest(target references.Position) bool {
	next, found := g.CombatScene.firstGettableObjectAt(target)
	if !found || !next.fromChest {
		return false
	}
	g.CombatScene.takeFirstObjectAt(target, func(candidate CombatObject) bool {
		return candidate.lifetimeID == next.lifetimeID
	})
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundOpenDoor)
	if len(g.generateCombatChestContents(target, next.TreasureValue())) == 0 {
		g.SystemCallbacks.Message.AddRowStr("Chest empty!")
	}

	return true
}

func (g *GameState) getAndReportCombatItem(direction references.Direction, audible bool) CombatPickupResult {
	result := g.getCombatItem(direction, audible)
	if !g.IsInCombat() || g.CombatScene == nil {
		return result
	}
	combat := g.CombatScene
	position := g.getActiveCombatantPosition()
	if target := direction.GetNewPositionInDirection(&position); target != nil {
		position = *target
	}
	combat.recordCollection(result)
	g.SystemCallbacks.Combat.PublishEvent(CombatEvent{
		Kind: CombatEventPickup,
		Pickup: CombatPickupEvent{
			ActorName: g.combatIdentityName(combat.GetCurrentTurnIdentity()),
			Position:  position, Result: result,
		},
	})

	return result
}

func (s *CombatState) recordCollection(result CombatPickupResult) {
	if result.Acquired == 0 {
		return
	}
	for index := range s.collectedItems {
		item := &s.collectedItems[index]
		if item.ItemType == result.ItemType && item.ItemID == result.ItemID {
			item.Quantity += uint64(result.Acquired)

			return
		}
	}
	s.collectedItems = append(s.collectedItems, CombatCollectedItem{
		ItemType: result.ItemType, ItemID: result.ItemID, Name: result.ItemName,
		Sprite: result.Sprite, Quantity: uint64(result.Acquired),
	})
}
