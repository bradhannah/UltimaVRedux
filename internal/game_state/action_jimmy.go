package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/map_units"
	"github.com/bradhannah/UltimaVRedux/internal/party_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// JimmyDoorState describes the result of a physical unlock or prisoner attempt.
type JimmyDoorState int

// Jimmy results distinguish resource failure, target refusal and successful mutations.
const (
	JimmyDoorNoLockPicks JimmyDoorState = iota
	JimmyUnlocked
	JimmyNotADoor
	JimmyBrokenPick
	JimmyPrisonerFreed
	JimmyPrisonerAlreadyFreed
	JimmyPrisonerNoOne
	JimmyNoCharacter
	JimmyChestUnlocked
	JimmyChestAlreadySafe
)

// Prisoner freeing constants from original Search/Get/Open/Jimmy rules
// See ref:PROV-0004.
const (
	// prisonerFreeKarmaBonus awards two karma for a newly freed prisoner,
	// with the party karma cap applied by its mutation authority. ref:KARMA-0008
	prisonerFreeKarmaBonus = 2

	// prisonerDexCheckMax bounds the physical-unlock draw at 0..29.
	// Success requires Dexterity strictly greater than the draw. ref:PROV-0004
	prisonerDexCheckMax               = 29
	combatChestJimmyDifficultyDivisor = 2
)

// ActionJimmySmallMapWithCharacter resolves Jimmy with the character selected by the UI.
func (g *GameState) ActionJimmySmallMapWithCharacter(
	direction references.Direction,
	selectedCharacter *party_state.PlayerCharacter,
) bool {
	return g.actionJimmySurface(direction, selectedCharacter)
}

func (g *GameState) actionJimmySurface(
	direction references.Direction,
	selectedCharacter *party_state.PlayerCharacter,
) bool {
	jimmyResult := g.JimmyDoor(direction, selectedCharacter)
	if !g.surfaceCycleOwned {
		g.CompleteSurfaceCommandCycle(g.MapState.PlayerLocation.Location.GetMapType())
	}

	switch jimmyResult {
	case JimmyUnlocked:
		// REDUX DEVIATION: physical unlocking opens a door without a dedicated success sound.
		// Redux exposes the successful physical unlock as an authorable (currently empty) slot.
		// See ref:PROV-0004.
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
		g.SystemCallbacks.UI.ShowActionFeedback("Unlocked!", true)

		return true
	case JimmyChestUnlocked:
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
		g.SystemCallbacks.UI.ShowActionFeedback(chestDisarmedMessage, true)

		return true
	case JimmyChestAlreadySafe:
		g.SystemCallbacks.UI.ShowActionFeedback(chestAlreadySafeMessage, true)

		return false
	case JimmyPrisonerFreed:
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
		g.SystemCallbacks.UI.ShowActionFeedback("\"I thank thee!\"", true)

		return true
	case JimmyPrisonerAlreadyFreed:
		return true
	case JimmyPrisonerNoOne:
		g.SystemCallbacks.Message.AddRowStr("No one is there!")

		return false
	case JimmyNotADoor:
		g.SystemCallbacks.Message.AddRowStr("No lock!")

		return false
	case JimmyBrokenPick:
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundKeyBreak)
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)

		return false
	case JimmyDoorNoLockPicks:
		g.SystemCallbacks.UI.ShowActionFeedback("No Keys!", true)

		return false
	case JimmyNoCharacter:
		g.SystemCallbacks.Message.AddRowStr("No characters available!")

		return false
	default:
		g.SystemCallbacks.Message.AddRowStr("Failed!")

		return false
	}
}

// ActionJimmyLargeMapWithCharacter resolves Jimmy with the character selected by the UI.
func (g *GameState) ActionJimmyLargeMapWithCharacter(
	direction references.Direction,
	selectedCharacter *party_state.PlayerCharacter,
) bool {
	return g.actionJimmySurface(direction, selectedCharacter)
}

// ActionJimmyCombatMapWithDisarm retains handled-command semantics and returns
// a detached identity for the successfully disarmed chest's presentation ownership.
func (g *GameState) ActionJimmyCombatMapWithDisarm(direction references.Direction) (bool, *CombatChestIdentity) {
	combatantPos := g.getActiveCombatantPosition()
	targetPos := direction.GetNewPositionInDirection(&combatantPos)
	if targetPos == nil || !isCombatMapPosition(*targetPos) {
		return false, nil
	}

	combatantIndex := g.CombatScene.GetActiveCombatantIndex()
	combatant := g.PartyState.GetCharacter(combatantIndex)
	if chest, found := g.CombatScene.objectAtMatching(*targetPos, CombatObject.IsChest); found {
		if g.jimmyCombatChest(chest, combatant) {
			return true, &CombatChestIdentity{Position: *targetPos, ObjectID: chest.LifetimeID()}
		}

		return true, nil
	}
	if tile := g.CombatScene.CombatMapRef.GetTileNumber(*targetPos); tile == indexes.Stocks || tile == indexes.Manacles {
		return g.jimmyCombatRestraint(*targetPos, combatant), nil
	}
	result := g.jimmyWorldDoorAt(g.GetCurrentLayeredMap(), targetPos, combatant)
	if result == JimmyUnlocked {
		g.setCombatTerrain(*targetPos, g.GetCurrentLayeredMap().GetTileTopMapOnlyTile(targetPos).Index)
	}

	return g.reportCombatJimmyResult(result), nil
}

// combatRestraintUnlockedMessage says what changed: the restraint is gone and
// the square is open floor. No prisoner is involved on a tactical map.
const combatRestraintUnlockedMessage = "Unlocked! The empty restraint falls open."

// jimmyCombatRestraint picks the lock of stocks or manacles on a tactical map.
// Tactical maps carry no prisoner, so success simply clears the restraint to
// floor; the key and Dexterity rules match a locked door. See ref:PROV-0004.
func (g *GameState) jimmyCombatRestraint(target references.Position, player *party_state.PlayerCharacter) bool {
	keys := &g.PartyState.Inventory.Provisions.Keys
	switch {
	case !keys.HasSome():
		g.SystemCallbacks.UI.ShowActionFeedback("No keys!", true)
	case player == nil:
		return false
	case !g.isJimmySuccessful(player):
		keys.DecrementByOne()
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundKeyBreak)
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)
	default:
		g.setCombatTerrain(target, indexes.BrickFloor)
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
		g.SystemCallbacks.UI.ShowActionFeedback(combatRestraintUnlockedMessage, true)
	}

	return true
}

func (g *GameState) jimmyCombatChest(
	chest CombatObject,
	character *party_state.PlayerCharacter,
) bool {
	// REDUX DEVIATION: chest disarming breaks a key on every safe
	// chest attempt. Known safety instead permits a harmless refusal; blind
	// attempts retain the original cost without revealing hidden trap state.
	// See ref:CMD-0008.
	if chest.ChestAssessment() == references.ChestSafe {
		g.SystemCallbacks.UI.ShowActionFeedback(chestAlreadySafeMessage, true)

		return false
	}
	if g.PartyState.Inventory.Provisions.Keys.Get() == 0 {
		g.SystemCallbacks.UI.ShowActionFeedback("No keys!", true)

		return false
	}

	if !chest.IsLockedAndTrapped() {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundKeyBreak)
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)

		return false
	}

	difficulty := (combatTreasureDiceSides + chest.TreasureValue() - int(character.Dexterity)) /
		combatChestJimmyDifficultyDivisor
	if g.RandomIntInRange(1, combatTreasureDiceSides) <= difficulty {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundKeyBreak)
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)

		return false
	}

	g.CombatScene.liveChest(chest).UnlockChest()
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
	g.SystemCallbacks.UI.ShowActionFeedback(chestDisarmedMessage, true)

	return true
}

// ActionJimmyDungeonMap attempts to disarm the current dungeon chest.
func (g *GameState) ActionJimmyDungeonMap(characterIndex int) bool {
	return g.actionJimmyDungeonMap(characterIndex)
}

// SelectCharacterForJimmy selects the active or first eligible character.
func (g *GameState) SelectCharacterForJimmy() *party_state.PlayerCharacter {
	// REDUX DEVIATION: acting-character selection trusts the active designation; direct
	// Open/Jimmy retain the surface selector's Good/Poisoned eligibility policy.
	// See ref:PROV-0004.
	if activePlayer := g.GetActivePlayer(); activePlayer != nil && (activePlayer.IsGood() || activePlayer.IsPoisoned()) {
		return activePlayer
	}

	// The direct API has no selector; preserve its first-eligible fallback order.
	for i := range g.PartyState.GetCharacterCount() {
		character := g.PartyState.GetCharacter(i)
		if character.IsGood() || character.IsPoisoned() {
			return character
		}
	}

	return nil // No available characters
}

// JimmyDoor dispatches the target by map context after resource admission.
func (g *GameState) JimmyDoor(direction references.Direction, player *party_state.PlayerCharacter) JimmyDoorState {
	if !g.PartyState.Inventory.Provisions.Keys.HasSome() {
		position := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
		if chest := g.surfaceChestAt(position); chest != nil && chest.SurfaceChestAssessment() == references.ChestSafe {
			return JimmyChestAlreadySafe
		}

		return JimmyDoorNoLockPicks
	}

	switch g.MapState.PlayerLocation.Location.GetMapType() {
	case references.SmallMapType:
		return g.jimmySmallMap(direction, player)
	case references.LargeMapType:
		return g.jimmyLargeMap(direction, player)
	case references.DungeonMapType:
		return g.jimmyDungeon(direction, player)
	case references.CombatMapType, references.CampingMapType:
		panic("surface Jimmy called outside a surface map")
	default:
		panic("surface Jimmy called with unknown map type")
	}
}

// CanJimmySurfaceTarget reports whether the current authoritative surface map
// has a lock, chest, or small-map prisoner fixture in the requested direction.
func (g *GameState) CanJimmySurfaceTarget(direction references.Direction) bool {
	position := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if position == nil {
		return false
	}
	if g.surfaceChestAt(position) != nil {
		return true
	}
	tile := g.GetCurrentLayeredMap().GetTileTopMapOnlyTile(position)
	if tile == nil {
		return false
	}
	switch tile.Index { //nolint:exhaustive // This is a target-family query.
	case indexes.LockedDoor, indexes.LockedDoorView,
		indexes.MagicLockDoor, indexes.MagicLockDoorWithView:
		return true
	case indexes.Stocks, indexes.Manacles:
		return g.MapState.PlayerLocation.Location.GetMapType() == references.SmallMapType
	default:
		return false
	}
}

// HasAdjacentKnownSafeSurfaceChest allows a no-key direction prompt only when
// Jimmy can repeat safety already established by the party, without inspecting.
func (g *GameState) HasAdjacentKnownSafeSurfaceChest() bool {
	for _, direction := range []references.Direction{references.Up, references.Right, references.Down, references.Left} {
		position := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
		if chest := g.surfaceChestAt(position); chest != nil && chest.SurfaceChestAssessment() == references.ChestSafe {
			return true
		}
	}

	return false
}

func (g *GameState) jimmySmallMap(direction references.Direction, player *party_state.PlayerCharacter) JimmyDoorState {
	newPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if newPosition == nil {
		return JimmyNotADoor
	}
	theMap := g.GetCurrentLayeredMap()
	if result := g.jimmyWorldDoorAt(theMap, newPosition, player); result != JimmyNotADoor {
		return result
	}
	if chest := g.surfaceChestAt(newPosition); chest != nil {
		return g.jimmySurfaceChest(chest, player)
	}
	targetTile := theMap.GetTileTopMapOnlyTile(newPosition)
	if targetTile == nil {
		return JimmyNotADoor
	}

	if targetTile.Index == indexes.Stocks || targetTile.Index == indexes.Manacles {
		// Prisoner freeing - original Search/Get/Open/Jimmy rules
		// See ref:PROV-0004.
		return g.jimmyPrisoner(newPosition, player)
	}

	return JimmyNotADoor
}

func (g *GameState) jimmyLargeMap(direction references.Direction, player *party_state.PlayerCharacter) JimmyDoorState {
	newPosition := direction.GetNewPositionInDirection(&g.MapState.PlayerLocation.Position)
	if newPosition == nil {
		return JimmyNotADoor
	}
	if result := g.jimmyWorldDoorAt(g.GetCurrentLayeredMap(), newPosition, player); result != JimmyNotADoor {
		return result
	}
	if chest := g.surfaceChestAt(newPosition); chest != nil {
		return g.jimmySurfaceChest(chest, player)
	}

	return JimmyNotADoor
}

func (g *GameState) jimmyWorldDoorAt(
	theMap *map_state.LayeredMap,
	targetPos *references.Position,
	player *party_state.PlayerCharacter,
) JimmyDoorState {
	if targetPos == nil {
		return JimmyNotADoor
	}
	targetTile := theMap.GetTileTopMapOnlyTile(targetPos)
	if targetTile == nil {
		return JimmyNotADoor
	}
	keys := &g.PartyState.Inventory.Provisions.Keys
	switch targetTile.Index { //nolint:exhaustive // Only surface lock families are relevant.
	case indexes.LockedDoor, indexes.LockedDoorView:
		if !keys.HasSome() {
			return JimmyDoorNoLockPicks
		}
		if player == nil {
			return JimmyNoCharacter
		}
		if !g.isJimmySuccessful(player) {
			keys.DecrementByOne()

			return JimmyBrokenPick
		}
		unlockedDoor := indexes.SpriteIndex(indexes.RegularDoor)
		if targetTile.Is(indexes.LockedDoorView) {
			unlockedDoor = indexes.RegularDoorView
		}
		theMap.SetTileByLayer(map_state.MapOverrideLayer, targetPos, unlockedDoor)

		return JimmyUnlocked
	case indexes.MagicLockDoor, indexes.MagicLockDoorWithView:
		if !keys.HasSome() {
			return JimmyDoorNoLockPicks
		}
		// Original Search/Get/Open/Jimmy rules: a magical lock always breaks one
		// key, in towns and on combat maps alike. See ref:PROV-0004.
		keys.DecrementByOne()

		return JimmyBrokenPick
	default:
		return JimmyNotADoor
	}
}

func (g *GameState) jimmySurfaceChest(
	chest *map_units.NPCFriendly,
	player *party_state.PlayerCharacter,
) JimmyDoorState {
	// REDUX DEVIATION: chest disarming always breaks a key on a safe
	// chest. Refuse known-safe attempts freely for usability, but preserve
	// the indistinguishable broken-key cost for uncertain/uninspected chests.
	// See ref:CMD-0008.
	if chest.SurfaceChestAssessment() == references.ChestSafe {
		return JimmyChestAlreadySafe
	}
	if !g.PartyState.Inventory.Provisions.Keys.HasSome() {
		return JimmyDoorNoLockPicks
	}
	if player == nil {
		return JimmyNoCharacter
	}
	if !chest.SurfaceChestLocked() {
		// Original Search/Get/Open/Jimmy rules: attempting an unlocked chest breaks one key.
		// See ref:PROV-0004.
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()

		return JimmyBrokenPick
	}

	difficulty := (combatTreasureDiceSides + chest.SurfaceChestTreasureValue() - int(player.Dexterity)) /
		combatChestJimmyDifficultyDivisor
	if g.jimmySuccessForTesting == nil && g.RandomIntInRange(1, combatTreasureDiceSides) <= difficulty {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()

		return JimmyBrokenPick
	}
	if g.jimmySuccessForTesting != nil && !g.jimmySuccessForTesting(player) {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()

		return JimmyBrokenPick
	}
	chest.UnlockSurfaceChest()

	return JimmyChestUnlocked
}

func (g *GameState) reportCombatJimmyResult(result JimmyDoorState) bool {
	switch result {
	case JimmyUnlocked:
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundUnlock)
		g.SystemCallbacks.UI.ShowActionFeedback("Unlocked!", true)
	case JimmyBrokenPick:
		g.SystemCallbacks.Audio.PlaySoundEffect(SoundKeyBreak)
		g.SystemCallbacks.UI.ShowActionFeedback("Key broke!", true)
	case JimmyDoorNoLockPicks:
		g.SystemCallbacks.UI.ShowActionFeedback("No keys!", true)
	case JimmyNotADoor:
		return false
	case JimmyPrisonerFreed, JimmyPrisonerAlreadyFreed, JimmyPrisonerNoOne,
		JimmyNoCharacter, JimmyChestUnlocked, JimmyChestAlreadySafe:
		return false
	default:
		return false
	}

	return true
}

func (g *GameState) jimmyDungeon(_ references.Direction, _ *party_state.PlayerCharacter) JimmyDoorState {
	panic("dungeon Jimmy requires the command-cycle ActionJimmyDungeonMap path")
}

func (g *GameState) isJimmySuccessful(player *party_state.PlayerCharacter) bool {
	// Testing override
	if g.jimmySuccessForTesting != nil {
		return g.jimmySuccessForTesting(player)
	}

	// Physical unlocking succeeds when Dexterity exceeds a draw in 0..29.
	// Dexterity 30 always succeeds; Dexterity 1 succeeds only on zero.
	// The prisoner and door paths use the same bound. ref:PROV-0004
	roll := g.RandomIntInRange(0, prisonerDexCheckMax)

	return int(player.Dexterity) > roll
}

// jimmyPrisoner handles freeing a prisoner from stocks or manacles.
// Based on original Search/Get/Open/Jimmy rules implementation.
//
// Logic:
// 1. Check if NPC exists at position → "No one is there!" if empty
// 2. Check for keys
// 3. DEX check: dex <= random(0,29) → "Key broke!" on fail
// 4. On success:
//   - Clear NPC's dialog number (NPC says "No response!" when talked to)
//   - +2 karma
//   - NPC resumes normal AI behavior (walks away)
//
// See ref:PROV-0004.
func (g *GameState) jimmyPrisoner(targetPos *references.Position, player *party_state.PlayerCharacter) JimmyDoorState {
	// REDUX DEVIATION: KARMA-0008: original reward/absence eligibility depends
	// on shape-specific flags. Redux limits rewards to persistent friendly
	// town NPCs, preventing arbitrary occupants and repeated rescues earning karma.
	npcUnit := g.CurrentNPCAIController.GetNpcs().GetMapUnitAtPositionAndFloorOrNil(
		*targetPos,
		g.MapState.PlayerLocation.Floor,
	)
	if npcUnit == nil {
		return JimmyPrisonerNoOne
	}
	npcFriendly, ok := (*npcUnit).(*map_units.NPCFriendly)
	if !ok || npcFriendly.NPCReference.Location != g.MapState.PlayerLocation.Location ||
		npcFriendly.MapUnitDetails().NPCNum < 0 ||
		npcFriendly.MapUnitDetails().NPCNum >= party_state.NPcsPerLocation {
		return JimmyPrisonerNoOne
	}

	// Check if we have keys before attempting
	if !g.PartyState.Inventory.Provisions.Keys.HasSome() {
		return JimmyDoorNoLockPicks
	}

	// Fail when Dexterity is no greater than the 0..29 draw. ref:PROV-0004
	if !g.isJimmyPrisonerSuccessful(player) {
		g.PartyState.Inventory.Provisions.Keys.DecrementByOne()

		return JimmyBrokenPick
	}

	// Original Search/Get/Open/Jimmy rules clears dialogue before checking whether this prisoner was already
	// rewarded, then marks the persistent absence on every successful attempt.
	// See ref:PROV-0004.
	npcFriendly.NPCReference.DialogNumber = byte(references.NoResponse)
	location := g.MapState.PlayerLocation.Location
	npcSlot := npcFriendly.MapUnitDetails().NPCNum
	alreadyFreed := g.PartyState.IsNPCDead(location, npcSlot)
	if !alreadyFreed {
		npcFriendly.MapUnitDetails().SetOverriddenAiType(references.FollowAroundAndBeAnnoyingThenNeverSeeAgain)
		npcFriendly.MapUnitDetails().SetCurrentPath(nil)
		npcFriendly.MapUnitDetails().PendingInteraction = map_units.PendingNPCInteractionNone
		g.AdjustPartyKarma(prisonerFreeKarmaBonus)
	}
	g.PartyState.SetDeadNpc(location, npcSlot)
	if alreadyFreed {
		return JimmyPrisonerAlreadyFreed
	}

	return JimmyPrisonerFreed
}

// isJimmyPrisonerSuccessful requires Dexterity greater than a draw in
// 0..29. Dexterity 30 always succeeds; Dexterity 1 succeeds only on zero. ref:CRIME-0005
func (g *GameState) isJimmyPrisonerSuccessful(player *party_state.PlayerCharacter) bool {
	// Testing override
	if g.jimmyPrisonerSuccessForTesting != nil {
		return g.jimmyPrisonerSuccessForTesting(player)
	}

	roll := g.RandomIntInRange(0, prisonerDexCheckMax) // Matches random(0,29) from original

	return int(player.Dexterity) > roll
}
