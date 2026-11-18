package game_state

import "github.com/bradhannah/UltimaVRedux/internal/references"

func (g *GameState) newUseableItemInfo(
	itemRef references.InventoryItem,
	category UseItemCategory,
	usage UsageContext,
	quantity int,
	item references.Item,
) UseableItemInfo {
	name := itemRef.FullDisplayName()
	info := UseableItemInfo{
		Name: name, FullName: name, Description: itemRef.ItemDescription,
		EffectSummary: "", Requirement: "", Consumption: "", Warning: "", UnavailableReason: "",
		AvailableNow: true, Category: category, UsageContext: usage, Quantity: quantity,
		SpriteIndex: itemRef.ItemSprite, ItemRef: itemRef, ItemType: item,
	}

	return g.withUseItemGuidance(info, UsageField)
}

func (g *GameState) withUseItemGuidance(info UseableItemInfo, context UsageContext) UseableItemInfo {
	info.AvailableNow = true
	info.EffectSummary, info.Requirement, info.Consumption = "", "", ""
	info.Warning, info.UnavailableReason = "", ""
	switch item := info.ItemType.(type) {
	case references.Potion:
		g.addPotionUseGuidance(&info, item, context)
	case references.Scroll:
		g.addScrollUseGuidance(&info, item)
	case references.QuestItem:
		g.addQuestItemUseGuidance(&info, item, context)
	case references.Moonstone:
		g.addMoonstoneUseGuidance(&info, context)
	case references.Provision:
		g.addProvisionUseGuidance(&info, item, context)
	default:
		panic("use item projection received an unsupported item type")
	}
	if !useItemSupportsContext(info.ItemType, info.UsageContext, context) {
		markUseItemUnavailable(&info, useItemContextRestriction(info.ItemType, context))
	}

	return info
}

// Potions and scrolls describe themselves in the shared catalog, which Z-stats
// shows too. Use adds only what is true right now: the cost, and any reason the
// attempt would be refused or wasted here.
func (g *GameState) addPotionUseGuidance(info *UseableItemInfo, potion references.Potion, context UsageContext) {
	info.Consumption = "Single use."
	if !g.ReviewPotionUse(potion, context).EffectAvailable {
		warnUseItemIneffective(info, "It may have no effect here, but will still be consumed.")
	}
}

func (g *GameState) addScrollUseGuidance(info *UseableItemInfo, scroll references.Scroll) {
	info.Consumption = "Single use."
	switch scroll {
	case references.ScrollRelHur:
		if !g.isScrollLocationValid(scroll) {
			markUseItemUnavailable(info, "You cannot change the wind here.")
		}
	case references.ScrollInManiCorp:
		if !g.PartyState.HasDeadMembers() {
			warnUseItemIneffective(info, "No companion is dead. An attempt still consumes the scroll.")
		}
	case references.ScrollAnTym:
		location := g.MapState.PlayerLocation.Location
		if location == references.Stonegate || location == references.Doom {
			warnUseItemIneffective(info, "It has no effect here, but will still be consumed.")
		}
	case references.ScrollVasLor, references.ScrollInSanct, references.ScrollInAn,
		references.ScrollInQuasWis, references.ScrollKalXenCorp:
		// Nothing situational to add.
	default:
		panic("use item projection received an unknown scroll")
	}
}

func (g *GameState) addQuestItemUseGuidance(info *UseableItemInfo, item references.QuestItem, context UsageContext) {
	switch item {
	case references.Amulet:
		info.EffectSummary, info.Requirement = wearableUseGuidance("Amulet", g.IsAmuletActive())
	case references.Crown:
		info.EffectSummary, info.Requirement = wearableUseGuidance("Crown", g.IsCrownActive())
	case references.BlackBadge:
		info.EffectSummary, info.Requirement = wearableUseGuidance("Badge", g.IsBadgeActive())
	case references.Sceptre:
		// Ownership is not evidence that the Avatar has learned its powers.
	case references.ShardOfFalsehood, references.ShardOfHatred, references.ShardOfCowardice:
		if context == UsageDungeon {
			markUseItemUnavailable(info, "You cannot use it here.")
		}
	case references.Carpet:
		info.EffectSummary = "Unfurl and board the carpet."
		//nolint:exhaustive // Only pre-use refusals require a browsing restriction.
		switch g.ReviewCarpetBoarding().Result {
		case CarpetBoardingNotHere:
			markUseItemUnavailable(info, "You cannot unfurl the carpet here.")
		case CarpetBoardingExitShipFirst:
			markUseItemUnavailable(info, "Leave your boat first.")
		case CarpetBoardingOnlyOnFoot:
			markUseItemUnavailable(info, "Dismount first.")
		case CarpetBoardingOccupied:
			markUseItemUnavailable(info, "This spot is occupied.")
		case CarpetBoardingNoMapCapacity:
			markUseItemUnavailable(info, "There is no room to unfurl it here.")
		}
	case references.Grapple:
		markUseItemUnavailable(info, "Use [K]limb.")
	case references.Spyglass:
		info.EffectSummary = "Study the night sky."
		if !g.IsSpyglassLocationValid() {
			markUseItemUnavailable(info, "You cannot see the stars from here.")
		} else if !g.IsUseAstronomyNight() {
			markUseItemUnavailable(info, "Wait until nightfall.")
		}
	case references.HMSCape:
		info.EffectSummary = "Consult the plans."
		if g.IsHMSCapeActivated() {
			info.EffectSummary = "Put the plans aside."
		}
	case references.PocketWatch:
		info.EffectSummary = "Read the time."
		if context == UsageCombat {
			info.Consumption = "Uses your turn."
		}
	case references.WoodenBox:
		markUseItemUnavailable(info, "You cannot use it here.")
	case references.Sextant:
		info.EffectSummary = "Take your bearings."
		switch g.CurrentSextantEligibility() {
		case SextantMissing, SextantNotOverworld, SextantDaylight:
			markUseItemUnavailable(info, g.sextantUnavailableReason())
		case SextantReady:
		}
	default:
		panic("use item projection received an unknown quest item")
	}
}

func (g *GameState) addMoonstoneUseGuidance(info *UseableItemInfo, context UsageContext) {
	info.EffectSummary = "Bury the moonstone."
	if context != UsageField {
		markUseItemUnavailable(info, "You cannot bury it here.")

		return
	}
	mapType := g.MapState.PlayerLocation.Location.GetMapType()
	if mapType != references.LargeMapType && mapType != references.SmallMapType {
		markUseItemUnavailable(info, "You cannot bury it here.")

		return
	}
	position := g.MapState.PlayerLocation.Position
	tile := g.GetLayeredMapByCurrentLocation().GetTopTile(&position)
	if tile == nil || !tile.IsValidForMoonstoneBurial() {
		markUseItemUnavailable(info, "This ground cannot hold a buried moonstone.")
	}
}

func (g *GameState) addProvisionUseGuidance(info *UseableItemInfo, item references.Provision, context UsageContext) {
	if item != references.SkullKeys {
		panic("use item projection received an unsupported provision")
	}
	info.EffectSummary = "Try the key on an adjacent door."
	info.Consumption = "Each attempt uses one key."
	if context == UsageCombat {
		info.Consumption = "Each attempt uses one key and your turn."
	}
	if (context != UsageField && context != UsageCombat) || g.ReviewSkullKeyUse().Result != SkullKeyUseReady {
		markUseItemUnavailable(info, "You cannot use a Skull Key here.")
	}
}

func wearableUseGuidance(name string, worn bool) (string, string) {
	if worn {
		return "Remove the " + name + ".", "Worn."
	}

	return "Wear the " + name + ".", "Not worn."
}

func markUseItemUnavailable(info *UseableItemInfo, reason string) {
	info.UnavailableReason = reason
	info.AvailableNow = false
	info.Warning = ""
}

func warnUseItemIneffective(info *UseableItemInfo, warning string) {
	info.AvailableNow = false
	info.Warning = warning
}

func useItemContextRestriction(_ references.Item, context UsageContext) string {
	if context == UsageCombat {
		return "Not during battle."
	}

	return "You cannot use it here."
}
