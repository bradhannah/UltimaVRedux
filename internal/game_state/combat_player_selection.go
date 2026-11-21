package game_state

// SelectCombatPlayer handles numeric selection at a player-input boundary.
// Combat selection must use encounter entries, not just the persistent roster:
// a living party member can already have fled or entered the final mirror.
func (g *GameState) SelectCombatPlayer(index int) bool {
	combat := g.CombatScene
	if combat == nil {
		panic("SelectCombatPlayer called outside combat")
	}
	if combat.HasScheduledCombatStep() || g.EventQueueManager.HasInputBlockingWorkflows() {
		return false
	}
	if index == -1 {
		g.clearCombatSoloSelection()

		return true
	}
	if !g.canSelectCombatPlayer(index) || !g.SetActivePlayer(index) {
		return false
	}

	actor := combat.GetCurrentTurnIdentity()
	combat.SetSoloMoveCharacter(index)
	g.SystemCallbacks.Message.AddRowStr("Set active player: " + g.PartyState.GetCharacter(index).GetNameAsString())
	if !actor.IsPartyMember || actor.EntityIndex != index {
		g.FinishCombatSelection()
	}

	return true
}

func (g *GameState) clearCombatSoloSelection() {
	if !g.CombatScene.IsSoloMoveActive() {
		return
	}
	g.ClearActivePlayer()
	g.CombatScene.ClearSoloMoveMode()
	g.SystemCallbacks.Message.AddRowStr("Set active player: None!")
}

func (g *GameState) canSelectCombatPlayer(index int) bool {
	// Active actor selection scans combat queue in source order for a matching
	// party-loyal PC, rejecting sleep, underground and object/departed entries.
	// See ref:COMBAT-0002.
	for _, entry := range g.CombatScene.TurnQueue.entries {
		identity := turnQueueIdentity(entry)
		if !g.CombatScene.isLivingCombatPCEntry(entry, index) ||
			g.combatIdentityLoyalty(identity) != CombatMonsterLoyaltyParty {
			continue
		}

		return !g.CombatScene.isCombatPCSleeping(identity) &&
			!g.CombatScene.isCombatPCUnderground(identity)
	}

	return false
}

func (combat *CombatState) isLivingCombatPCEntry(entry *TurnQueueEntry, index int) bool {
	return entry.status == turnQueueActive && entry.Combatant.IsPC() &&
		entry.EntityIndex == index && entry.Combatant.IsAlive() &&
		combat.isCombatPCActive(turnQueueIdentity(entry))
}

func (g *GameState) clearUnreachableCombatSoloSelection() {
	combat := g.CombatScene
	if !combat.IsSoloMoveActive() {
		return
	}
	for _, entry := range combat.TurnQueue.entries {
		if combat.isLivingCombatPCEntry(entry, combat.GetSoloMoveCharacter()) {
			// Temporary sleep, captivity, or possession still reaches a scheduled
			// boundary. Do not erase those conditions or their normal turn effects.
			return
		}
	}

	// An unreachable target would auto-pass every remaining friendly actor
	// forever after victory. Revalidate at every boundary, including advances
	// from workflow finalizers and after actor-local effects change the roster.
	combat.ClearSoloMoveMode()
	g.ClearActivePlayer()
}
