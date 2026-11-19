package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ToggleItemDuration is the permanent sentinel for normally donned artifacts.
const ToggleItemDuration = 255

// ToggleItemType identifies which toggle item is being tracked.
type ToggleItemType byte

// Toggle item kinds; the byte is the original item letter (Amulet, Crown, Badge).
const (
	ToggleItemAmulet ToggleItemType = 'A'
	ToggleItemCrown  ToggleItemType = 'C'
	ToggleItemBadge  ToggleItemType = 'B'
)

// ToggleItemState tracks a single active toggle item.
type ToggleItemState struct {
	IsActive       bool
	TurnsRemaining int
}

// FriendlyName returns display name for messages.
func (t ToggleItemType) FriendlyName() string {
	switch t {
	case ToggleItemAmulet:
		return "Amulet"
	case ToggleItemCrown:
		return "Crown"
	case ToggleItemBadge:
		return "Badge"
	default:
		return "Item"
	}
}

// ToggleAmulet toggles the Amulet of Lord British.
func (gs *GameState) ToggleAmulet() bool {
	return gs.toggleItem(&gs.AmuletState, ToggleItemAmulet)
}

// spellEndedByCrownMessage tells the player which duration spell the Crown ended.
func spellEndedByCrownMessage(effect DurationEffectType) string {
	return effect.DisplayName() + " ended!"
}

// ToggleCrown toggles the Crown of Lord British.
func (gs *GameState) ToggleCrown() bool {
	// REDUX DEVIATION: original item-use rules and original spell-effect rules
	// put Crown, Amulet, Badge and duration spells in one active effect/duration counter
	// slot. Redux keeps Badge and Amulet independent for concurrent artifact use;
	// only Crown competes with duration spells, without resuming replaced effects.
	// See ref:QUEST-0003.
	if !gs.CrownState.IsActive {
		// The original ends the spell silently; Redux names it, as it names the
		// Crown when a spell displaces it. See CrownDisplacedMessage.
		if gs.HasAnyDurationEffect() {
			message := spellEndedByCrownMessage(gs.ActiveDurationEffect.EffectType)
			gs.SystemCallbacks.UI.ShowActionFeedback(message, true)
		}
		gs.ClearDurationEffect()
	}

	return gs.toggleItem(&gs.CrownState, ToggleItemCrown)
}

// ToggleBadge toggles the Black Badge.
func (gs *GameState) ToggleBadge() bool {
	return gs.toggleItem(&gs.BadgeState, ToggleItemBadge)
}

// toggleItem is internal - activates or deactivates a toggle item.
func (gs *GameState) toggleItem(state *ToggleItemState, itemType ToggleItemType) bool {
	if state.IsActive {
		state.IsActive = false
		state.TurnsRemaining = 0
		gs.SystemCallbacks.UI.ShowActionFeedback("Removed!", true)

		return true // Item was used (deactivated)
	}

	state.IsActive = true
	state.TurnsRemaining = ToggleItemDuration
	gs.SystemCallbacks.UI.ShowActionFeedback(itemType.FriendlyName()+" donned!", true)

	// Trigger visual flash effect based on item type
	gs.triggerToggleItemFlash(itemType)

	return true // Item was used (activated)
}

// triggerToggleItemFlash triggers a screen flash when equipping a toggle item.
// Crown/Amulet get white/gold flash (royal items), Badge gets dark flash (sinister).
// Delegates to UI layer via FlowCallback which uses PartyStateBuilder for animation.
func (gs *GameState) triggerToggleItemFlash(itemType ToggleItemType) {
	gs.SystemCallbacks.Flow.Items.TriggerToggleItemFlash(itemType)
}

// IsAmuletActive checks if the Amulet effect is active.
func (gs *GameState) IsAmuletActive() bool {
	return gs.AmuletState.IsActive
}

// LostInDarknessMessage tells the player why a move went nowhere.
// REDUX DEVIATION: QUEST-0004: the original prints only the direction and gives
// no reason; Redux adds this line so a blank view and a party that will not move
// do not read as a fault.
const LostInDarknessMessage = "Lost in darkness!"

// IsLostInDarkness reports whether the party stands on a darkness square of the
// large map without the Amulet of Lord British worn. While lost the party sees
// nothing and cannot walk: a move is accepted and takes its turn, but the party
// stays where it is. Wearing the Amulet lifts both. See ref:QUEST-0004.
func (gs *GameState) IsLostInDarkness() bool {
	if gs.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType || gs.IsAmuletActive() {
		return false
	}
	ground := gs.GetCurrentAvatarGroundTile()

	return ground != nil && ground.Index == indexes.BlackSquare
}

// IsCrownActive checks if the Crown effect is active.
func (gs *GameState) IsCrownActive() bool {
	return gs.CrownState.IsActive
}

// IsBadgeActive checks if the Badge effect is active.
func (gs *GameState) IsBadgeActive() bool {
	return gs.BadgeState.IsActive
}

// processToggleItems decrements and expires toggle items.
// Called from ProcessTurnEffects each turn.
func (gs *GameState) processToggleItems() {
	gs.processOneToggleItem(&gs.AmuletState, ToggleItemAmulet)
	gs.processOneToggleItem(&gs.CrownState, ToggleItemCrown)
	gs.processOneToggleItem(&gs.BadgeState, ToggleItemBadge)
}

func (gs *GameState) processOneToggleItem(state *ToggleItemState, itemType ToggleItemType) {
	if !state.IsActive {
		return
	}

	// Permanent effects (255) never decrement - Crown/Amulet/Badge never wear off
	if state.TurnsRemaining == ToggleItemDuration {
		return
	}

	state.TurnsRemaining--
	if state.TurnsRemaining <= 0 {
		state.IsActive = false
		gs.SystemCallbacks.UI.ShowSpellExpiredPopup(itemType.FriendlyName())
	}
}

// WornArtifactStatus is a detached view of a party-level Use toggle, not character equipment.
type WornArtifactStatus struct {
	ID    ToggleItemType
	Owned bool
	Worn  bool
}

// GetWornArtifactStatuses preserves fixed Crown, royal Amulet, Badge positions.
// Ownership never overrides an authoritative active toggle during presentation.
func (gs *GameState) GetWornArtifactStatuses() [3]WornArtifactStatus {
	items := &gs.PartyState.Inventory.QuestItems

	return [3]WornArtifactStatus{
		{ID: ToggleItemCrown, Owned: items.HasSome(references.Crown), Worn: gs.IsCrownActive()},
		{ID: ToggleItemAmulet, Owned: items.HasSome(references.Amulet), Worn: gs.IsAmuletActive()},
		{ID: ToggleItemBadge, Owned: items.HasSome(references.BlackBadge), Worn: gs.IsBadgeActive()},
	}
}
