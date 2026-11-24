package game_state

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// UseItemCategory represents the category groupings for usable items
type UseItemCategory int

// Use inventory categories group owned items independently of their availability.
const (
	UseItemCategoryQuest UseItemCategory = iota
	UseItemCategoryPotion
	UseItemCategoryScroll
	UseItemCategoryTools
	UseItemCategoryMoonstone
	UseItemCategoryKeys
)

// UsageContext indicates when an item can be used
type UsageContext int

// Usage contexts identify the supported item interaction routes.
const (
	UsageAnytime UsageContext = iota // Usable in combat and field
	UsageField                       // Field/exploration use only
	UsageCombat                      // Combat use only
	UsageDungeon                     // Dungeon Use workflow only
)

// String returns the display text for the usage context
func (u UsageContext) String() string {
	switch u {
	case UsageAnytime:
		return "Anytime"
	case UsageField:
		return "Field"
	case UsageCombat:
		return "Combat"
	case UsageDungeon:
		return "Dungeon"
	default:
		return "Anytime"
	}
}

// UseableItemInfo contains information about a usable item for the UI
type UseableItemInfo struct {
	Name          string
	FullName      string
	Description   string
	EffectSummary string
	Requirement   string
	Consumption   string
	Warning       string
	// UnavailableReason blocks activation; AvailableNow describes effect applicability only.
	UnavailableReason string
	AvailableNow      bool
	Category          UseItemCategory
	UsageContext      UsageContext
	Quantity          int
	SpriteIndex       indexes.SpriteIndex
	ItemRef           references.InventoryItem
	ItemType          references.Item
}

// GetUsableItems returns all usable items in the player's inventory, organized for the UI
func (g *GameState) GetUsableItems() []UseableItemInfo {
	var items []UseableItemInfo

	// Tool items (Pocket Watch, Sextant, Carpet, etc.)
	// These are now QuestItem indices 6-13
	for questItem := references.Carpet; questItem <= references.Sextant; questItem++ {
		qty := g.PartyState.Inventory.QuestItems.Get(questItem)
		if qty > 0 {
			itemRef := g.GameReferences.InventoryItemReferences.QuestItem[questItem]
			usageContext := UsageField
			if questItem == references.HMSCape || questItem == references.BlackBadge || questItem == references.PocketWatch {
				usageContext = UsageAnytime
			}
			items = append(items, g.newUseableItemInfo(
				itemRef,
				getCategoryForToolQuestItem(questItem),
				usageContext,
				int(qty),
				questItem,
			))
		}
	}

	// Potions (Blue through White)
	for potion := references.Potion(0); potion <= references.White; potion++ {
		qty := g.PartyState.Inventory.Potions.Get(potion)
		if qty > 0 {
			itemRef := g.GameReferences.InventoryItemReferences.Potion[potion]
			items = append(items, g.newUseableItemInfo(
				itemRef,
				UseItemCategoryPotion,
				UsageAnytime,
				int(qty),
				potion,
			))
		}
	}

	// Scrolls (VasLor through AnTym)
	for scroll := references.Scroll(0); scroll <= references.ScrollAnTym; scroll++ {
		qty := g.PartyState.Inventory.Scrolls.Get(scroll)
		if qty > 0 {
			itemRef := g.GameReferences.InventoryItemReferences.Scroll[scroll]
			items = append(items, g.newUseableItemInfo(
				itemRef,
				UseItemCategoryScroll,
				scrollUsageContext(scroll),
				int(qty),
				scroll,
			))
		}
	}

	// Quest items (Amulet, Crown, Sceptre, Shards)
	for questItem := references.QuestItem(0); questItem <= references.ShardOfCowardice; questItem++ {
		qty := g.PartyState.Inventory.QuestItems.Get(questItem)
		if qty > 0 {
			itemRef := g.GameReferences.InventoryItemReferences.QuestItem[questItem]
			usageContext := UsageField
			if questItem == references.Sceptre || questItem == references.Crown || questItem == references.Amulet {
				usageContext = UsageAnytime
			}

			items = append(items, g.newUseableItemInfo(
				itemRef,
				UseItemCategoryQuest,
				usageContext,
				int(qty),
				questItem,
			))
		}
	}

	// Moonstones (NewMoon through CrescentWaning)
	for moonstone := references.Moonstone(0); moonstone <= references.CrescentWaning; moonstone++ {
		qty := g.PartyState.Inventory.Moonstones.Get(moonstone)
		if qty > 0 {
			itemRef := g.GameReferences.InventoryItemReferences.Moonstone[moonstone]
			items = append(items, g.newUseableItemInfo(
				itemRef,
				UseItemCategoryMoonstone,
				UsageField,
				int(qty),
				moonstone,
			))
		}
	}

	// Skull Keys (from Provisions - unlocks magically locked doors)
	if g.PartyState.Inventory.Provisions.SkullKeys.Get() > 0 {
		itemRef := g.GameReferences.InventoryItemReferences.Provision[references.SkullKeys]
		itemRef.ItemSprite = indexes.ItemKey
		items = append(items, g.newUseableItemInfo(
			itemRef,
			UseItemCategoryKeys,
			UsageAnytime,
			int(g.PartyState.Inventory.Provisions.SkullKeys.Get()),
			references.SkullKeys,
		))
	}

	return items
}

// GetUsableItemsForContext projects ownership separately from activation permission.
// REDUX DEVIATION: CONSUME-0001: original Use checks context after expenditure;
// Redux keeps owned items inspectable and rejects unavailable activation for free.
func (g *GameState) GetUsableItemsForContext(context UsageContext) []UseableItemInfo {
	items := g.GetUsableItems()
	for index := range items {
		items[index] = g.withUseItemGuidance(items[index], context)
	}

	return items
}

func useItemSupportsContext(item references.Item, usage UsageContext, context UsageContext) bool {
	if context == UsageDungeon {
		switch typedItem := item.(type) {
		case references.Potion:
			return true
		case references.Scroll:
			switch typedItem {
			case references.ScrollVasLor,
				references.ScrollInSanct,
				references.ScrollInAn,
				references.ScrollInQuasWis,
				references.ScrollInManiCorp,
				references.ScrollAnTym:
				return true
			case references.ScrollRelHur, references.ScrollKalXenCorp:
				return false
			}
		case references.QuestItem:
			switch typedItem {
			case references.Amulet,
				references.Crown,
				references.Sceptre,
				references.ShardOfFalsehood,
				references.ShardOfHatred,
				references.ShardOfCowardice,
				references.Carpet,
				references.Spyglass,
				references.HMSCape,
				references.PocketWatch,
				references.BlackBadge,
				references.WoodenBox,
				references.Sextant:
				return true
			case references.Grapple:
				return false
			}
		case references.Moonstone:
			return true
		case references.Provision:
			return typedItem == references.SkullKeys
		default:
			return false
		}
	}

	return usage == UsageAnytime || usage == context
}

// RevalidateUseItemSelection resolves a detached selection against current inventory and context.
func (g *GameState) RevalidateUseItemSelection(
	selection UseableItemInfo,
	context UsageContext,
) (UseableItemInfo, bool) {
	currentContext := g.GetCurrentSpellContext()
	if (context == UsageCombat && currentContext != references.ContextCombat) ||
		(context == UsageDungeon && currentContext != references.ContextDungeon) ||
		(context == UsageField && currentContext != references.ContextTown && currentContext != references.ContextOverworld) {
		return UseableItemInfo{}, false
	}
	for _, current := range g.GetUsableItemsForContext(context) {
		if current.ItemType.Type() == selection.ItemType.Type() &&
			current.ItemType.ID() == selection.ItemType.ID() &&
			current.Quantity == selection.Quantity && current.UnavailableReason == "" {
			return current, true
		}
	}

	return UseableItemInfo{}, false
}

func scrollUsageContext(scroll references.Scroll) UsageContext {
	//nolint:exhaustive // The default intentionally groups all currently combat-and-field scrolls.
	switch scroll {
	case references.ScrollRelHur, references.ScrollInQuasWis, references.ScrollInManiCorp:
		return UsageField
	case references.ScrollKalXenCorp:
		return UsageCombat
	default:
		return UsageAnytime
	}
}

// getCategoryForToolQuestItem maps tool quest items to their UI category
func getCategoryForToolQuestItem(item references.QuestItem) UseItemCategory {
	switch item {
	case references.PocketWatch, references.Sextant, references.Spyglass:
		return UseItemCategoryTools
	case references.Amulet, references.Crown, references.Sceptre,
		references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice,
		references.Carpet, references.Grapple, references.HMSCape, references.BlackBadge, references.WoodenBox:
		return UseItemCategoryQuest
	default:
		panic("unknown Use quest item category")
	}
}

// usePocketWatchWithCompletion reads the time without advancing it.
// REDUX DEVIATION: TIME-0005: original dungeon input is charged before the command.
// Redux charges the deferred token when the watch is selected, then adds no second
// minute or surface completion. Cancelling the menu first stays free.
// See ref:TIME-0005.
func (g *GameState) usePocketWatchWithCompletion(complete func()) bool {
	g.CommitCastUseAttempt()
	timeStr := g.DateTime.GetTimeAsString()
	g.SystemCallbacks.Flow.Observation.ShowPocketWatchTime(timeStr, complete)

	return true
}

// UseItem executes the use action for a specific item selected from the UI
func (g *GameState) UseItem(item UseableItemInfo) bool {
	return g.useItem(item, nil)
}

// UseItemForSurface transfers accepted asynchronous completion back to the
// surface Use workflow that owns the command cycle.
func (g *GameState) UseItemForSurface(item UseableItemInfo, complete func()) bool {
	if complete == nil {
		panic("surface Use requires a completion callback")
	}

	return g.useItem(item, complete)
}

// UseItemWithCompletion executes an item whose owning context supplies the
// command completion boundary.
func (g *GameState) UseItemWithCompletion(item UseableItemInfo, complete func()) bool {
	if complete == nil {
		panic("Use item requires a completion callback")
	}

	return g.useItem(item, complete)
}

func (g *GameState) useItem(item UseableItemInfo, complete func()) bool {
	switch itemType := item.ItemType.(type) {
	case references.Potion:
		return g.usePotionWithCompletion(itemType, complete)
	case references.Scroll:
		return g.useScrollWithCompletion(itemType, complete)
	case references.QuestItem:
		return g.useQuestItem(itemType, complete)
	case references.Moonstone:
		return g.useMoonstone(itemType, complete)
	case references.Provision:
		if itemType == references.SkullKeys {
			return g.useSkullKey(complete)
		}

		g.SystemCallbacks.Message.AddRowStr("Can't use that!")

		return false
	default:
		g.SystemCallbacks.Message.AddRowStr("Can't use that!")

		return false
	}
}

// usePotionWithCompletion reviews one field potion use before delegating presentation to GameScene.
// GameState commit operations own consumption, randomness, and effects.
// Based on original item-use rules potion mechanics.
// See ref:CONSUME-0001.
func (g *GameState) usePotionWithCompletion(potion references.Potion, complete func()) bool {
	commit, review := g.BeginPotionUse(potion, UsageField)
	if commit == nil {
		if review.Quantity == 0 {
			g.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)
		} else {
			g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)
		}

		return false
	}
	g.SystemCallbacks.Flow.Items.StartPotionWorkflow(commit, complete)

	return true // Workflow handles success/failure messaging
}

func (g *GameState) useScrollWithCompletion(scroll references.Scroll, complete func()) bool {
	if !g.PartyState.Inventory.Scrolls.HasSome(scroll) {
		g.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)

		return false
	}
	// Wind Change scroll requires direction input - delegate to UI workflow
	// Based on original item-use rules - prompts "Direction-" before wind change
	// See ref:CONSUME-0001.
	if scroll == references.ScrollRelHur {
		// Pre-validate location before starting workflow
		if !g.isScrollLocationValid(scroll) {
			g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

			return false
		}

		// Start workflow - handles direction prompt, scroll consumption, and effects
		g.SystemCallbacks.Flow.Items.StartWindChangeScrollWorkflow(complete)

		return true // Workflow handles success/failure
	}

	// Resurrection scroll requires character selection before commitment.
	if scroll == references.ScrollInManiCorp {
		// Pre-validate location (non-combat only)
		if !g.isScrollLocationValid(scroll) {
			g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

			return false
		}

		// Start workflow - handles character selection and consumption
		g.SystemCallbacks.Flow.Items.StartResurrectionScrollWorkflow(complete)

		return true // Workflow handles success/failure
	}

	scrollName := g.GameReferences.InventoryItemReferences.Scroll[scroll].FullDisplayName()

	// Check location restrictions before attempting to use
	if !g.isScrollLocationValid(scroll) {
		g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

		return false
	}

	if !g.CommitScrollUse(scroll) {
		return false
	}
	// Effect failure does not undo a confirmed attempt.
	success := g.executeScrollEffect(scroll, complete)
	if success {
		g.SystemCallbacks.UI.ShowActionFeedback(scrollName+"!", true)
	} else {
		g.SystemCallbacks.UI.ShowActionFeedback("No effect!", true)
	}
	if complete == nil {
		g.AdvanceOrdinarySurfaceCommandMinutes(1)
	}
	if complete != nil && (!success || !scrollEffectOwnsWorkflow(scroll)) {
		complete()
	}

	return true
}

// CommitScrollUse validates inventory/context and pays for a final confirmation.
// REDUX DEVIATION: original item-use rules consume before direction/target selection;
// Redux cancellation is free, but a committed no-effect attempt still costs one.
// See ref:CONSUME-0001.
func (g *GameState) CommitScrollUse(scroll references.Scroll) bool {
	if scroll < references.ScrollVasLor || scroll > references.ScrollAnTym {
		return false
	}
	if g.GetCurrentSpellContext() == references.ContextDungeon {
		if !useItemSupportsContext(scroll, scrollUsageContext(scroll), UsageDungeon) {
			return false
		}
	} else if !g.isScrollLocationValid(scroll) {
		return false
	}
	if !g.PartyState.Inventory.Scrolls.DecrementByOne(scroll) {
		return false
	}
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundScrollInvoke)
	g.CommitCastUseAttempt()

	return true
}

// ResurrectionScrollUseResult reports whether a confirmed target received the effect.
type ResurrectionScrollUseResult int

const (
	// ResurrectionScrollUnavailable means no scroll could be committed.
	ResurrectionScrollUnavailable ResurrectionScrollUseResult = iota
	// ResurrectionScrollNoEffect means a scroll was consumed for a living target.
	ResurrectionScrollNoEffect
	// ResurrectionScrollSuccess means a scroll was consumed and the target was resurrected.
	ResurrectionScrollSuccess
)

// CommitResurrectionScrollUse consumes one scroll for a confirmed target, then
// applies the authoritative resurrection effect when that target is dead.
// REDUX DEVIATION: original item-use rules consume before recipient selection; Redux cancellation
// is free, but a confirmed living target consumes the scroll with no effect.
// See ref:CONSUME-0001.
func (g *GameState) CommitResurrectionScrollUse(targetIndex int) ResurrectionScrollUseResult {
	if !g.PartyState.HasCharacter(targetIndex) || !g.PartyState.GetCharacter(targetIndex).IsInParty() {
		return ResurrectionScrollUnavailable
	}
	if !g.ConsumeResurrectionScroll() {
		return ResurrectionScrollUnavailable
	}

	return g.ResolveConsumedResurrectionScroll(targetIndex)
}

// ConsumeResurrectionScroll commits inventory after the caller confirms a valid target.
func (g *GameState) ConsumeResurrectionScroll() bool {
	return g.CommitScrollUse(references.ScrollInManiCorp)
}

// ResolveConsumedResurrectionScroll applies a previously consumed scroll.
func (g *GameState) ResolveConsumedResurrectionScroll(targetIndex int) ResurrectionScrollUseResult {
	if !g.ResurrectCharacter(targetIndex) {
		return ResurrectionScrollNoEffect
	}

	return ResurrectionScrollSuccess
}

func scrollEffectOwnsWorkflow(scroll references.Scroll) bool {
	//nolint:exhaustive // Only scrolls whose presentation owns completion are relevant.
	switch scroll {
	case references.ScrollVasLor, references.ScrollInQuasWis, references.ScrollAnTym:
		return true
	default:
		return false
	}
}

// isScrollLocationValid checks if a scroll can be used at the current location.
// Each scroll type has specific location restrictions per original Ultima V.
func (g *GameState) isScrollLocationValid(scroll references.Scroll) bool {
	mapType := g.MapState.PlayerLocation.Location.GetMapType()

	switch scroll {
	case references.ScrollVasLor: // Light - Any location
		return true

	case references.ScrollRelHur: // Original item-use rules permit surface towns as well as outdoors.
		// See ref:CONSUME-0001.
		return mapType == references.LargeMapType || mapType == references.SmallMapType

	case references.ScrollInSanct: // Protection - Any (but only useful in combat)
		return true

	case references.ScrollInAn: // Negate Magic - Any location
		return true

	case references.ScrollInQuasWis: // View - dungeon presentation owns a separate command workflow
		// REDUX DEVIATION: VIEW-0001: the original scroll is consumed before its combat
		// rejection. Redux rejects the illegal combat View before consuming it, under
		// its validated-attempt policy. See ref:VIEW-0001.
		return mapType == references.LargeMapType || mapType == references.SmallMapType

	case references.ScrollKalXenCorp: // Summon Daemon - Combat only
		return mapType == references.CombatMapType

	case references.ScrollInManiCorp: // Resurrection - Non-combat only
		return mapType != references.CombatMapType

	case references.ScrollAnTym: // Time Stop - Always useable, location check in workflow
		// Original item-use rules block direct use in Stonegate (0x1d) and Doom (0x28).
		// Location check moved to workflow so scroll is consumed and "No effect!" popup shown
		// See ref:CONSUME-0001.
		return true

	default:
		return false
	}
}

// executeScrollEffect performs the actual scroll effect.
// Returns true if the effect was successfully applied.
// Note: ScrollRelHur is handled via StartWindChangeScrollWorkflow, not here.
func (g *GameState) executeScrollEffect(scroll references.Scroll, complete func()) bool {
	switch scroll {
	case references.ScrollRelHur: // Wind Change - handled by workflow, shouldn't reach here
		g.SystemCallbacks.UI.ShowActionFeedback("Use workflow for wind change!", true)

		return false

	case references.ScrollInQuasWis: // View (overhead map)
		return g.executeScrollView(complete)

	case references.ScrollVasLor: // Light (240 gameplay minutes)
		// Trigger workflow with visual effects (invert + sound), then activate light
		g.SystemCallbacks.Flow.Items.StartLightScrollWorkflow(complete)

		return true // Scroll consumed, workflow handles the rest

	case references.ScrollInSanct: // Protection
		duration := g.GameReferences.InventoryItemReferences.GetScrollEffectDuration(references.ScrollInSanct)

		return g.ActivateProtectionEffect(duration)

	case references.ScrollInAn: // Negate Magic
		duration := g.GameReferences.InventoryItemReferences.GetScrollEffectDuration(references.ScrollInAn)

		return g.ActivateNegateMagicEffect(duration)

	case references.ScrollKalXenCorp: // Summon Daemon (combat only)
		// The combat Use workflow owns summoning; this exploration dispatcher cannot perform it.
		g.SystemCallbacks.UI.ShowActionFeedback("Use the combat scroll workflow!", true)

		return false

	case references.ScrollInManiCorp: // Resurrection (non-combat only)
		// Handled in useScroll() special case - shouldn't reach here
		// Workflow handles dead member check, character selection, and consumption
		return false

	case references.ScrollAnTym: // Time Stop (20 turns)
		// Trigger workflow with visual effects (invert + popup), then activate time stop
		g.SystemCallbacks.Flow.Items.StartTimeStopScrollWorkflow(complete)

		return true // Scroll consumed, workflow handles the rest

	default:
		g.SystemCallbacks.UI.ShowActionFeedback("Unknown scroll!", true)

		return false
	}
}

// Scroll durations are owned by the reference scroll definitions.
// Access via: g.GameReferences.InventoryItemReferences.GetScrollEffectDuration(scroll)
// Original values from original party-effect rules duration installation: Light=240, Protection=100, NegateMagic=20, TimeStop=20
// See ref:CONSUME-0001.

// WindChangePopupDurationMs is how long to display the wind change popup (2 seconds).
const WindChangePopupDurationMs = 2000

// CompleteWindChangeScroll finishes the Wind Change scroll workflow after direction input.
// Called from GameScene after player selects a direction. Handles scroll consumption,
// wind change, and user feedback. Based on original item-use rules.
// See ref:CONSUME-0001.
func (g *GameState) CompleteWindChangeScroll(direction references.Direction) bool {
	return g.completeWindChangeScroll(direction, true)
}

// CompleteWindChangeScrollForSurfaceUse leaves ordinary cycle completion to the owning Use workflow.
func (g *GameState) CompleteWindChangeScrollForSurfaceUse(direction references.Direction) bool {
	return g.completeWindChangeScroll(direction, false)
}

func (g *GameState) completeWindChangeScroll(direction references.Direction, advanceOrdinaryMinutes bool) bool {
	if direction != references.Up && direction != references.Down &&
		direction != references.Left && direction != references.Right {
		return false
	}
	if !g.CommitScrollUse(references.ScrollRelHur) {
		return false
	}
	// Set wind to the chosen direction
	success := g.SetWindByDirection(direction, true)

	if success {
		// Show combined spell invocation and new wind direction popup
		windMessage := "Rel Hur!\nWind: " + direction.GetDirectionCompassName()
		g.SystemCallbacks.UI.ShowTimedOverlayMessage(windMessage, WindChangePopupDurationMs, true, false)

		if advanceOrdinaryMinutes {
			g.AdvanceOrdinarySurfaceCommandMinutes(1)
		}
	}

	return success
}

// executeScrollView implements the In Quas Wis (View) scroll effect.
// Shows overhead map view - same as using a gem but scroll is consumed instead.
func (g *GameState) executeScrollView(complete func()) bool {
	// Trigger gem view workflow (same visual result, different consumption)
	g.SystemCallbacks.Flow.Observation.ShowGemViewInteraction(complete)

	return true
}

// useQuestItem handles quest items (Amulet, Crown, Sceptre) and tool items
func (g *GameState) useQuestItem(item references.QuestItem, complete func()) bool {
	automaticToggle := item == references.Amulet || item == references.Crown || item == references.BlackBadge
	if automaticToggle {
		// Capture the dungeon actor phase before Crown can replace Time Stop or Quickness.
		g.CommitCastUseAttempt()
	}
	var success bool
	switch item {
	case references.Amulet:
		success = g.ToggleAmulet()
	case references.Crown:
		success = g.ToggleCrown()
	// Former SpecialItems - now unified under QuestItem
	case references.PocketWatch:
		return g.usePocketWatchWithCompletion(complete)
	case references.Sextant:
		success = g.useSextant()
	case references.Spyglass:
		if !g.IsSpyglassLocationValid() || !g.IsUseAstronomyNight() {
			g.SystemCallbacks.UI.ShowActionFeedback("Not here or not at night!", true)

			return false
		}
		g.CommitCastUseAttempt()
		// Trigger spyglass workflow in GameScene (handles day/night, location checks)
		g.SystemCallbacks.Flow.Observation.ShowSpyglassInteraction(complete)

		return true
	case references.Grapple:
		// Grapples are used via the Klimb (K) command on mountains
		g.SystemCallbacks.UI.ShowActionFeedback("Use 'K'limb on mountains!", true)

		return false
	case references.WoodenBox:
		// Quest item for endgame only - can't be used elsewhere
		g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

		return false
	case references.Carpet:
		return g.useCarpet(complete)
	case references.BlackBadge:
		success = g.ToggleBadge()
	case references.HMSCape:
		return g.useHMSCape(complete)
	case references.Sceptre:
		return g.useSceptre(complete)
	case references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice:
		g.SystemCallbacks.Flow.Shadowlord.ShowShardUsageWorkflow(item, complete)

		return true
	default:
		g.SystemCallbacks.UI.ShowActionFeedback("Cannot use that!", true)

		return false
	}
	if success {
		if !automaticToggle {
			g.CommitCastUseAttempt()
		}
		if complete != nil {
			complete()
		}
	}

	return success
}

// useMoonstone handles moonstone usage (burial at current location)
// Based on original item-use rules moonstone burial logic
// See ref:CONSUME-0001.
func (g *GameState) useMoonstone(moonstone references.Moonstone, complete func()) bool {
	// Check map type restriction: overworld and towns only (mapNum < 0x21)
	// Dungeons and underworld are not valid burial locations
	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	if mapType != references.LargeMapType && mapType != references.SmallMapType {
		g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

		return false
	}

	// Check terrain restriction at player position
	playerPos := g.MapState.PlayerLocation.Position
	tile := g.GetLayeredMapByCurrentLocation().GetTopTile(&playerPos)
	if tile == nil || !tile.IsValidForMoonstoneBurial() {
		g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)

		return false
	}

	// Trigger the burial workflow in GameScene (handles event queue, effects, time advancement)
	// Burial is automatic: choosing an available stone on valid terrain is its
	// final confirmation, not dismissal of the subsequent feedback dialog.
	if !g.PartyState.Inventory.Moonstones.HasSome(moonstone) {
		return false
	}
	g.CommitCastUseAttempt()
	g.SystemCallbacks.Flow.Items.StartMoonstoneBurialWorkflow(moonstone, complete)

	return true
}

// useCarpet handles using the magic carpet from inventory to board it.
// Based on original item-use rules carpet mechanics.
// See ref:CONSUME-0001.
func (g *GameState) useCarpet(complete func()) bool {
	commit, review := g.BeginCarpetBoarding()
	if commit == nil {
		//nolint:exhaustive // Ready and commit-only results cannot accompany a nil commit.
		switch review.Result {
		case CarpetBoardingNoInventory:
			g.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)
		case CarpetBoardingExitShipFirst:
			g.SystemCallbacks.UI.ShowActionFeedback("X-it ship first!", true)
		case CarpetBoardingOnlyOnFoot:
			g.SystemCallbacks.UI.ShowActionFeedback("Only on foot!", true)
		case CarpetBoardingNotHere, CarpetBoardingOccupied, CarpetBoardingNoMapCapacity:
			g.SystemCallbacks.UI.ShowActionFeedback("Not here!", true)
		default:
			panic("unsupported carpet boarding review")
		}

		return false
	}
	g.SystemCallbacks.Flow.Items.StartCarpetBoardingWorkflow(commit, complete)

	return true
}

// useSkullKey handles skull key usage for unlocking magically locked doors.
// Based on original item-use rules and magical unlocking.
// Note: Key consumption happens in the event queue workflow.
// See ref:PROV-0006.
func (g *GameState) useSkullKey(complete func()) bool {
	commit, _ := g.BeginSkullKeyUse()
	if commit == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("No skull keys!", true)

		return false
	}
	g.SystemCallbacks.Flow.Items.StartSkullKeyWorkflow(commit, complete)

	return true
}

// useHMSCape begins the global HMS Cape preference confirmation.
// REDUX DEVIATION: SHIP-0002: Original item-use rules permit one-way rigging only while
// shipboard. Redux makes the persisted setting reversible and usable anywhere
// once the plans are owned, as an explicit player preference.
// See ref:SHIP-0002, ref:CONSUME-0001.
func (g *GameState) useHMSCape(complete func()) bool {
	commit, review := g.BeginHMSCapeUse()
	if commit == nil {
		if review.Result != HMSCapeUseNoPlans {
			panic("unsupported HMS Cape use review")
		}
		g.SystemCallbacks.UI.ShowActionFeedback("You have none!", true)

		return false
	}
	g.SystemCallbacks.Flow.Items.StartHMSCapeWorkflow(commit, complete)

	return true
}

// useSceptre handles Sceptre of Lord British usage - dispels energy fields.
// Triggers workflow in GameScene for direction prompt and field dispelling.
// See ref:CONSUME-0001.
func (g *GameState) useSceptre(complete func()) bool {
	g.SystemCallbacks.Flow.Items.StartSceptreWorkflow(complete)

	return true
}

// useSextant displays coordinates to the player (outdoors + night only in original game)
func (g *GameState) useSextant() bool {
	switch g.CurrentSextantEligibility() {
	case SextantMissing:
		g.SystemCallbacks.Message.AddRowStr("You have none!")

		return false
	case SextantNotOverworld:
		g.SystemCallbacks.Message.AddRowStr("Only outdoors!")

		return false
	case SextantDaylight:
		g.SystemCallbacks.Message.AddRowStr("Only at night!")

		return false
	case SextantReady:
		// The Use effect reports the Avatar, not a journal observation target.
	}

	fix, ok := g.CurrentSextantFix()
	if !ok {
		panic("ready sextant has no fix")
	}
	g.surveyCurrentAtlasSettlement(fix)
	pos := fix.Position
	g.SystemCallbacks.Message.AddRowStr("Position:")
	g.SystemCallbacks.Message.AddRowStr(fmt.Sprintf("%s, %s", encodeSextantCoordinate(pos.Y), encodeSextantCoordinate(pos.X)))

	return true
}

func encodeSextantCoordinate(coordinate references.Coordinate) string {
	value := byte(coordinate)

	return fmt.Sprintf("%c'%c\"", 'A'+(value>>4), 'A'+(value&0x0f))
}

// IsUseAstronomyNight matches original item-use rules's Spyglass and Sextant hour gate.
// See ref:CONSUME-0001.
func (g *GameState) IsUseAstronomyNight() bool {
	return isUseAstronomyNight(g.DateTime.Hour)
}

// IsSpyglassLocationValid permits surface world and small-map contexts
// with a nonnegative floor in Redux’s signed representation. ref:ASTRO-0003
func (g *GameState) IsSpyglassLocationValid() bool {
	mapType := g.MapState.PlayerLocation.Location.GetMapType()

	return (mapType == references.LargeMapType || mapType == references.SmallMapType) &&
		g.MapState.PlayerLocation.Floor >= 0
}
