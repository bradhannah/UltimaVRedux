package game_state

import (
	"strings"

	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// ActionYellSmallMap attempts Keep summoning and reports other words' local effects.
func (g *GameState) ActionYellSmallMap(word string) bool {
	// Small map yell handles Shadowlord summoning at Keeps
	// Sailing commands are handled on large map only (frigates don't exist on small maps)

	word = strings.ToUpper(strings.TrimSpace(word))

	// Check for Shadowlord summoning at Keeps
	if references.IsKeepLocation(g.MapState.PlayerLocation.Location) {
		if references.ShadowlordFromName(word) != references.NoShadowlord {
			return g.TrySummonShadowlord(word)
		}
	}

	// Check for sailing commands (HOIST, FURL) - not valid on small maps
	if word == "HOIST" || word == "FURL" {
		g.SystemCallbacks.Message.AddRowStr("Not on a ship!")

		return false
	}

	// Check for Words of Power that should work on large map only
	if _, exists := GetDungeonForWord(word); exists {
		g.SystemCallbacks.UI.ShowActionFeedback("The words echo emptily here.", true)

		return false
	}

	// Unknown word or context.
	g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

	return false
}

// TrySummonShadowlord attempts to summon a Shadowlord by name at a Keep.
// Returns true only when summoning succeeds.
// See ref:WORD-0003.
func (g *GameState) TrySummonShadowlord(word string) bool {
	// Check if word matches a Shadowlord name (case insensitive substring match)
	lord := references.ShadowlordFromName(word)
	if lord == -1 {
		return false // Not a Shadowlord name, let other handlers try
	}

	state := g.ShadowlordState
	playerY := g.MapState.PlayerLocation.Position.Y
	floor := g.MapState.PlayerLocation.Floor
	if state == nil {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	logging.Debugf(logging.CategoryGameState,
		"TrySummonShadowlord: lord=%s, playerY=%d, floor=%d, summoned=%v, alive=%v",
		lord.String(), playerY, floor, state.Summoned, state.IsAlive(lord))

	// Original command rules - Check if player Y position is >= 2
	// This prevents summoning too close to the top of the map (Shadowlord spawns at Y-2)
	// See ref:WORD-0003.
	if playerY < 2 {
		logging.Debugf(logging.CategoryGameState,
			"TrySummonShadowlord: FAILED - playerY=%d < 2 (Shadowlord would spawn off map at Y=%d)",
			playerY, playerY-2)
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	// Check if this Shadowlord is alive.
	if !state.IsAlive(lord) {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	// Check if another Shadowlord is already summoned
	if state.Summoned {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	// Insert the physical Shadowlord before committing progression state.
	enemy := g.insertShadowlordNPC(lord)
	if enemy == nil {
		logging.Errorf(logging.CategoryGameState,
			"TrySummonShadowlord: failed to spawn Shadowlord NPC")
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}
	if !state.Summon(lord) {
		if !g.CurrentNPCAIController.GetNpcs().RemoveEnemy(enemy) {
			panic("validated Shadowlord summon rollback failed")
		}
		panic("validated Shadowlord summon was rejected")
	}

	// Trigger the summoning visual/audio sequence via SystemCallbacks
	g.SystemCallbacks.Flow.Shadowlord.TriggerShadowlordSummoned(lord)

	return true
}

// ActionYellLargeMap toggles frigate sails or prompts for a Word of Power.
func (g *GameState) ActionYellLargeMap() bool {
	vehicleType := g.PartyVehicle.GetVehicleDetails().VehicleType

	// Not on frigate - show dialog for Word of Power
	if vehicleType != references.FrigateVehicle {
		g.SystemCallbacks.UI.ShowYellDialogLargeMap()

		return false
	}

	// On frigate - auto-toggle sails (original Ultima V behavior: original command rules)
	// Simple 2-state toggle: SailRaised ↔ SailFurled
	// See ref:WORD-0003.
	vehicleDetails := g.PartyVehicle.GetVehicleDetails()

	if vehicleDetails.SailState == map_units.SailRaised {
		// Furl sails (sailing → anchored) - immediate state change
		g.StopAutomaticSailing()
		vehicleDetails.SetSailState(map_units.SailFurled)
		g.SystemCallbacks.UI.ShowActionFeedback("FURL!", true)
	} else {
		// Hoist sails (anchored → sailing) - immediate state change
		vehicleDetails.SetSailState(map_units.SailRaised)
		g.SystemCallbacks.UI.ShowActionFeedback("HOIST!", true)
	}

	return true
}

// ProcessWordOfPower processes a Word of Power for dungeon entrance unsealing
// Returns true if earthquake should trigger (valid word + correct location)
// Stores dungeon toggle state in GameState.lastDungeonToggleOpened for UI layer
func (g *GameState) ProcessWordOfPower(word string) bool {
	dungeon, exists := GetDungeonForResponse(word)
	if !exists {
		// REDUX DEVIATION: outdoor Yell has a silent coordinate-mismatch
		// edge case. Redux resolves every submitted failure explicitly and safely.
		// See ref:WORD-0003.
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	// Non-Doom entrances share one sealed state across Britannia and the
	// Underworld, so the word works beside either side. See ref:WORD-0001.
	if !DungeonEntranceExistsOnFloor(dungeon, g.MapState.PlayerLocation.Floor) {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	openSprite := dungeonOpenEntranceSprite(dungeon)
	playerPos := g.MapState.PlayerLocation.Position
	candidates := [...]references.Position{
		playerPos.Left(),
		playerPos.Down(),
		playerPos.Right(),
		playerPos.Up(),
	}
	var matchedPosition *references.Position
	for i := range candidates {
		tile := g.GetEffectiveTile(&candidates[i])
		if tile == openSprite || tile == indexes.BlockEntrance {
			matchedPosition = &candidates[i]

			break
		}
	}
	entrance := g.GetDungeonEntranceCoordinates()[dungeon]
	if matchedPosition == nil || !matchedPosition.Equals(&entrance) {
		g.SystemCallbacks.UI.ShowActionFeedback("Nothing happens.", true)

		return false
	}

	// Player is near correct dungeon with correct word - toggle the entrance
	g.lastDungeonToggleOpened = g.ToggleDungeon(dungeon)
	// A successful toggle establishes a word association only for an identified
	// entrance; its internal identity alone must not disclose an unknown name.
	if g.journalDungeonIdentityKnown(dungeon) {
		g.observeDungeonEntrance(dungeon, entrance, g.sampleDungeonObservation(), true)
	}
	g.lastDungeonOpenExperience = g.settleDeedExperience(false)

	// Return true to trigger earthquake visual sequence
	return true
}

// WasLastDungeonToggleOpening returns whether the last dungeon toggle opened (true) or sealed (false) the entrance
// Used by UI layer to determine earthquake message ("The entrance opens!" vs "The entrance seals!")
func (g *GameState) WasLastDungeonToggleOpening() bool {
	return g.lastDungeonToggleOpened
}

// LastDungeonOpenExperience returns the deed experience the last entrance
// toggle paid, for the earthquake dialog. ref:PROGRESS-0004
func (g *GameState) LastDungeonOpenExperience() DeedExperienceAward {
	return g.lastDungeonOpenExperience
}

// GetAdjacentRuinedShrine finds an adjacent ruined shrine corresponding
// to the submitted Word of Power and returns its ShrineData. ref:WORD-0003
func (g *GameState) GetAdjacentRuinedShrine(word string) *ShrineData {
	// REDUX DEVIATION: outdoor Yell can dispatch restoration for any
	// adjacent ruined shrine. Redux requires the word's corresponding shrine.
	// See ref:WORD-0003.
	shrineIndex := GetShrineForWordResponse(word)
	if shrineIndex == -1 {
		return nil
	}
	if g.MapState.PlayerLocation.Floor != references.FloorNumber(references.OVERWORLD) {
		return nil
	}

	if !g.IsShrineRuined(shrineIndex) {
		return nil
	}

	shrineData := GetShrineData(shrineIndex)
	if shrineData == nil {
		return nil
	}

	playerPos := g.MapState.PlayerLocation.Position
	shrinePos := references.Position{
		X: references.Coordinate(shrineData.WorldX),
		Y: references.Coordinate(shrineData.WorldY),
	}

	adjacentPositions := []references.Position{
		playerPos.Left(),
		playerPos.Right(),
		playerPos.Up(),
		playerPos.Down(),
	}

	for _, pos := range adjacentPositions {
		if pos.Equals(&shrinePos) && g.GetEffectiveTile(&pos) == indexes.BrokenShrine {
			return shrineData
		}
	}

	return nil
}

// ActionYellWithoutEffect reports a combat or dungeon yell; neither applies a gameplay effect.
func (g *GameState) ActionYellWithoutEffect(word string) bool {
	if word == "" {
		g.SystemCallbacks.Message.AddRowStr("Nothing")

		return true
	}
	g.SystemCallbacks.Message.AddRowStr("No effect!")

	return true
}
