package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// chestTheftKarmaPenalty is the two-point karma loss on committed town
// chest Open, floored at zero. No ownership exemption is applied. ref:KARMA-0005
const chestTheftKarmaPenalty = 2

// ActionOpenSmallMap opens the target using the surface acting-character selector.
func (g *GameState) ActionOpenSmallMap(direction references.Direction) bool {
	return g.actionOpenSmallMap(direction, g.SelectCharacterForJimmy())
}

// ActionOpenSmallMapWithCharacter resolves a chest trap against the UI-selected actor.
func (g *GameState) ActionOpenSmallMapWithCharacter(
	direction references.Direction,
	character *party_state.PlayerCharacter,
) bool {
	return g.actionOpenSmallMap(direction, character)
}

func (g *GameState) actionOpenSmallMap(
	direction references.Direction,
	character *party_state.PlayerCharacter,
) bool {
	openThingPos := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if openThingPos == nil {
		g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

		return false
	}
	openThingTile := g.GetLayeredMapByCurrentLocation().GetSmallMapPersistentTerrainTile(openThingPos, g.DateTime.Hour)

	// Handle doors first
	if openThingTile != nil && openThingTile.Index.IsDoor() {
		switch g.MapState.OpenDoor(direction) {
		case map_state.OpenDoorNotADoor:
			g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

			return false
		case map_state.OpenDoorLocked:
			g.SystemCallbacks.UI.ShowActionFeedback("Locked!", true)

			return false
		case map_state.OpenDoorLockedMagical:
			g.SystemCallbacks.UI.ShowActionFeedback("Locked!", true)

			return false
		case map_state.OpenDoorOpened:
			g.SystemCallbacks.Message.AddRowStr("Opened!")
			g.SystemCallbacks.Audio.Play(SoundOpenDoor).Start()
			g.AdvanceOrdinarySurfaceCommandMinutes(1)

			return true
		}

		return false
	}

	if chest := g.surfaceChestAt(openThingPos); chest != nil {
		return g.openSurfaceChest(openThingPos, chest, character)
	}

	if openThingTile != nil {
		switch openThingTile.Index { //nolint:exhaustive // Only source-defined Open object responses are relevant.
		case indexes.Box: // Original FOOTLOCKER, not original BOX.
			g.SystemCallbacks.Message.AddRowStr("It's open!")
			g.AdvanceOrdinarySurfaceCommandMinutes(1)

			return true
		case indexes.Portcullis:
			g.SystemCallbacks.Message.AddRowStr("Too heavy!")

			return false
		default:
		}
	}

	// Nothing openable
	g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

	return false
}

// ActionOpenLargeMap opens the outdoor target using the surface acting-character selector.
func (g *GameState) ActionOpenLargeMap(direction references.Direction) bool {
	return g.actionOpenLargeMap(direction, g.SelectCharacterForJimmy())
}

// ActionOpenLargeMapWithCharacter resolves a chest trap against the UI-selected actor.
func (g *GameState) ActionOpenLargeMapWithCharacter(
	direction references.Direction,
	character *party_state.PlayerCharacter,
) bool {
	return g.actionOpenLargeMap(direction, character)
}

func (g *GameState) actionOpenLargeMap(
	direction references.Direction,
	character *party_state.PlayerCharacter,
) bool {
	targetPos := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if targetPos == nil {
		g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

		return false
	}

	layeredMap := g.GetCurrentLayeredMap()
	topTile := layeredMap.GetTopTile(targetPos)

	if g.ItemStacksMap.HasItemStackAtPosition(targetPos) {
		g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

		return false
	}
	if chest := g.surfaceChestAt(targetPos); chest != nil {
		return g.openSurfaceChest(targetPos, chest, character)
	}
	if topTile == nil {
		g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

		return false
	}

	switch topTile.Index { //nolint:exhaustive // Only source-defined Open object responses are relevant.
	case indexes.Box:
		g.SystemCallbacks.Message.AddRowStr("It's open!")
		g.AdvanceOrdinarySurfaceCommandMinutes(1)

		return true
	case indexes.Portcullis:
		g.SystemCallbacks.Message.AddRowStr("Too heavy!")

		return false
	default:
	}

	if topTile.Index.IsDoor() {
		switch map_state.OpenDoorAt(layeredMap, targetPos) {
		case map_state.OpenDoorOpened:
			g.SystemCallbacks.Message.AddRowStr("Opened!")
			g.SystemCallbacks.Audio.Play(SoundOpenDoor).Start()
			g.AdvanceOrdinarySurfaceCommandMinutes(1)

			return true
		case map_state.OpenDoorLocked:
			g.SystemCallbacks.UI.ShowActionFeedback("Locked!", true)

			return false
		case map_state.OpenDoorLockedMagical:
			g.SystemCallbacks.UI.ShowActionFeedback("Locked!", true)

			return false
		case map_state.OpenDoorNotADoor:
			// Not a door, continue to check other openable things
			break
		}
	}

	g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

	return false
}

// CanOpenSurfaceChest reports whether Open targets an authoritative surface chest.
func (g *GameState) CanOpenSurfaceChest(direction references.Direction) bool {
	position := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)

	return g.surfaceChestAt(position) != nil
}

func (g *GameState) surfaceChestAt(position *references.Position) *map_units.NPCFriendly {
	if position == nil {
		return nil
	}
	unit := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		*position,
		g.MapState.PlayerLocation.Floor,
	)
	if unit == nil {
		return nil
	}
	friendly, ok := (*unit).(*map_units.NPCFriendly)
	if !ok || !friendly.IsVisible() || !friendly.IsSurfaceChest() {
		return nil
	}

	return friendly
}

func (g *GameState) openSurfaceChest(
	position *references.Position,
	chest *map_units.NPCFriendly,
	character *party_state.PlayerCharacter,
) bool {
	characterIndex := g.partyCharacterIndex(character)
	if characterIndex < 0 {
		g.SystemCallbacks.Message.AddRowStr("No characters available!")

		return false
	}
	if g.surfaceChestAt(position) != chest || g.surfaceChestPlacementBlocked(position, chest) {
		g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

		return false
	}
	isTown := g.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType
	isLordBritishTreasure := isTown &&
		g.MapState.PlayerLocation.Location == references.LordBritishsCastle &&
		g.MapState.PlayerLocation.Floor == references.Basement
	dailyIndex, daily := dailyBasementChestIndex(chest)
	if daily && !g.BasementChestHistory[dailyIndex].eligible(g.DateTime.DateOnly()) {
		g.SystemCallbacks.Message.AddRowStr("Nothing to open!")

		return false
	}
	trapped := chest.SurfaceChestLocked()

	npcNumber := chest.MapUnitDetails().NPCNum
	if !g.CurrentNPCAIController.GetNpcs().RemoveNPCByNumber(npcNumber) {
		panic("surface chest disappeared during Open resolution")
	}
	if daily {
		date := g.DateTime.DateOnly()
		g.BasementChestHistory[dailyIndex].LastOpened = &date
	}
	if isTown {
		if !daily && !chest.IsTransientSurfaceChest() {
			g.PartyState.SetDeadNpc(g.MapState.PlayerLocation.Location, npcNumber)
		}
		g.AdjustPartyKarma(-chestTheftKarmaPenalty)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundKarmaDown)
		g.SystemCallbacks.UI.ShowActionFeedback("Opened! That chest isn't yours Avatar!", true)
	} else {
		g.SystemCallbacks.Message.AddRowStr("Opened!")
	}
	if trapped {
		g.applyChestTrap(characterIndex)
	}
	if isLordBritishTreasure || chest.IsTransientSurfaceChest() {
		g.revealSurfaceChestLoot(*position, chest.SurfaceChestTreasureValue())
	} else {
		// Generic surface loot remains intentionally bounded until its source-backed
		// object-list representation is implemented.
		g.SystemCallbacks.Message.AddRowStr("Chest empty!")
	}
	g.CurrentNPCAIController.FreshenExistingNPCsOnMap()
	g.AdvanceOrdinarySurfaceCommandMinutes(1)

	return true
}

func (g *GameState) partyCharacterIndex(character *party_state.PlayerCharacter) int {
	if character == nil {
		return -1
	}
	for index := range g.PartyState.GetCharacterCount() {
		if g.PartyState.GetCharacter(index) == character && character.PartyMap == party_state.PartyMapActive && !character.IsDead() {
			return index
		}
	}

	return -1
}

// CombatOpenResult describes whether Open consumed the target and any original-style
// treasure descriptions that the presentation layer should reveal in order.
type CombatOpenResult struct {
	Handled     bool
	RevealLines []string
	RevealItems []CombatLootReveal
	Position    references.Position
	OpenedChest bool
	// ChestID survives removal so presentation can identify the opened lifetime.
	ChestID CombatObjectID
	Trap    CombatChestTrapResult
}

// ActionOpenCombatMap opens an adjacent combat chest or door and returns any
// chest descriptions that should be presented at the original game's cadence.
func (g *GameState) ActionOpenCombatMap(direction references.Direction) CombatOpenResult {
	combatantPos := g.getActiveCombatantPosition()
	targetPos := direction.GetNewPositionInDirection(&combatantPos)
	if targetPos == nil || !isCombatMapPosition(*targetPos) {
		return CombatOpenResult{Handled: false, RevealLines: nil}
	}

	if chest, found := g.CombatScene.takeFirstObjectAt(*targetPos, CombatObject.IsChest); found {
		activeIndex := g.CombatScene.GetActiveCombatantIndex()
		trap := CombatChestTrapResult{}
		if chest.IsLockedAndTrapped() {
			trap = g.triggerCombatChestTrap(activeIndex)
		}

		before := g.CombatScene.lootSlotMask()
		revealLines := g.generateCombatChestContents(*targetPos, chest.TreasureValue())
		reveals := g.CombatScene.newLootReveals(*targetPos, before, revealLines)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundOpenDoor)
		if len(revealLines) == 0 {
			g.SystemCallbacks.Message.AddRowStr("Chest empty!")
		}

		return CombatOpenResult{
			Handled: true, RevealLines: revealLines, RevealItems: reveals, Position: *targetPos,
			OpenedChest: true, ChestID: chest.LifetimeID(), Trap: trap,
		}
	}

	// Tactical Open answers the same fixed terrain as the town path before
	// trying doors: a footlocker is already open and a portcullis cannot be
	// lifted. Both still spend the attempt. See ref:COMBAT-0010.
	switch g.CombatScene.CombatMapRef.GetTileNumber(*targetPos) { //nolint:exhaustive // Only fixed Open responses.
	case indexes.Box:
		g.SystemCallbacks.Message.AddRowStr("It's open!")

		return CombatOpenResult{Handled: true, RevealLines: nil}
	case indexes.Portcullis:
		g.SystemCallbacks.UI.ShowActionFeedback("Too heavy!", true)

		return CombatOpenResult{Handled: true, RevealLines: nil}
	}

	switch map_state.OpenDoorAt(g.GetCurrentLayeredMap(), targetPos) {
	case map_state.OpenDoorOpened:
		g.setCombatTerrain(*targetPos, indexes.BrickFloor)
		g.SystemCallbacks.Message.AddRowStr("Opened!")
		g.SystemCallbacks.Audio.Play(SoundOpenDoor).Start()
	case map_state.OpenDoorLocked:
		g.SystemCallbacks.UI.ShowActionFeedback("Locked!", true)
	case map_state.OpenDoorLockedMagical:
		g.SystemCallbacks.UI.ShowActionFeedback("Locked!", true)
	case map_state.OpenDoorNotADoor:
		return CombatOpenResult{Handled: false, RevealLines: nil}
	default:
		return CombatOpenResult{Handled: false, RevealLines: nil}
	}

	return CombatOpenResult{Handled: true, RevealLines: nil}
}

// ActionOpenDungeonMap opens the chest in the party's current dungeon cell.
func (g *GameState) ActionOpenDungeonMap(characterIndex int) bool {
	return g.actionOpenDungeonMap(characterIndex)
}
