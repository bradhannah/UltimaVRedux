package game_state

import (
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

const (
	doomFinalRoomID    = 15
	doomFinalRoomFloor = references.FloorNumber(7)
	doomFinalRoomX     = references.Coordinate(5)
	doomFinalRoomY     = references.Coordinate(7)
	doomMirrorActorY   = references.Coordinate(2)
	doomMirrorTerrainY = references.Coordinate(1)
)

// resolveDoomMirrorAbsorption applies mirror absorption to Redux's verified
// Doom room only. The current queue identity remains authoritative for clones.
// See ref:DNG-0015.
func (g *GameState) resolveDoomMirrorAbsorption() bool {
	combat := g.CombatScene
	// REDUX DEVIATION: mirror absorption checks only actor state, Y=2,
	// and the IN_MIRROR family directly north. Redux also requires the verified
	// live Doom room-15 origin so custom or unrelated combat maps cannot start the finale.
	// See ref:DNG-0015.
	if combat == nil || !combat.isVerifiedDoomFinalRoom() {
		return false
	}
	identity := combat.GetCurrentTurnIdentity()
	if !identity.IsPartyMember || combat.isCombatPCPossessed(identity) {
		return false
	}
	entry := combat.TurnQueue.GetEntryByIdentity(identity)
	if entry == nil || entry.status != turnQueueActive || !entry.Combatant.IsAlive() {
		return false
	}
	position := combat.GetCombatantPosition(identity)
	if position.Y != doomMirrorActorY {
		return false
	}
	mirrorPosition := references.Position{X: position.X, Y: doomMirrorTerrainY}
	if combat.CombatMapRef.GetTileNumber(mirrorPosition) != indexes.Mirror {
		return false
	}

	combat.ExitRoute = CombatExitDoomMirror
	combat.TurnQueue.MarkAbsorbed(identity)
	if identity.Slot == 0 {
		combat.PartyActive[identity.EntityIndex] = false
	} else {
		combat.removePCClone(identity)
	}
	g.clearInactiveCombatPCSelection(identity.EntityIndex)
	g.SystemCallbacks.Message.AddRowStr(
		g.PartyState.GetCharacter(identity.EntityIndex).GetNameAsString() + " is absorbed!",
	)
	// The original follows the message with its rising vanishing tone as the
	// mirror takes the member; Redux plays its absorb cue. See ref:DNG-0015.
	g.SystemCallbacks.Audio.PlaySoundEffect(SoundAbsorb)

	return true
}

func (combat *CombatState) isVerifiedDoomFinalRoom() bool {
	origin := combat.DungeonRoomOrigin
	if origin == nil || !origin.Live || combat.CombatMapRef == nil {
		return false
	}

	return origin.Location == references.Doom &&
		origin.DungeonIndex == references.DoomDungeon &&
		origin.Room == doomFinalRoomID &&
		origin.Floor == doomFinalRoomFloor &&
		origin.Position == (references.Position{X: doomFinalRoomX, Y: doomFinalRoomY}) &&
		combat.CombatMapRef.Territory == references.DungeonCombatTerritory &&
		combat.CombatMapRef.MapIndex == references.CalculateDungeonCombatMapIndex(
			references.DoomDungeon,
			doomFinalRoomID,
		)
}
