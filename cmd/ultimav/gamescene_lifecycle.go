package main

import "github.com/bradhannah/UltimaVRedux/internal/logging"

// PrepareForSceneReplacement stops live playback before the candidate is activated.
func (g *GameScene) PrepareForSceneReplacement() {
	g.clearCombatMovement()
	clear(g.combatImpactEffects)
	g.combatImpactEffects = g.combatImpactEffects[:0]
	g.ResetTransientCommandInput()
	g.clearDungeonInjury()
	g.clearDungeonFieldDispels()
	g.clearActorAnimationClocks()
	g.clearKnowledgeNotices()
	g.clearBattleSummaries()
	g.resetProvisionChanges()
	g.surfaceCamera = surfaceCameraProjection{}
	g.terrainDither.enabled = false
	g.clearTimedOverlay()
	g.clearTransientActionFeedback()
	g.clearTransientIcons()
	if g.audioManager == nil {
		return
	}
	if err := g.audioManager.StopBackgroundMusic(); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to stop music before scene replacement: %v", err)
	}
}

// Dispose releases scene-owned resources. Repeated calls are harmless.
func (g *GameScene) Dispose() {
	g.clearSaveQuestion()
	if g.disposed {
		return
	}
	g.disposed = true
	g.ResetTransientCommandInput()
	g.releaseDungeonHUD()
	g.closeRunicReference()
	g.sky = skyPresentation{}
	if g.borders.skyBacking != nil {
		g.borders.skyBacking.Deallocate()
		g.borders.skyBacking = nil
	}
	g.resetProvisionChanges()
	g.closeJournalSession(g.journalSession, false)
	g.closeAtlasSession(true)
	g.clearKnowledgeNotices()
	for _, widget := range g.dialogStack.Dialogs() {
		if dialog, ok := widget.(*LinearTalkDialog); ok {
			dialog.Close()
		}
	}
	g.terrainDither.image.Dispose()
	g.outerTerrainDither.Dispose()
	g.terrainDither = terrainDitherFrame{}
	g.surfaceCamera = surfaceCameraProjection{}
	if g.surfaceTerrainImage != nil {
		g.surfaceTerrainImage.Deallocate()
		g.surfaceTerrainImage = nil
	}
	g.active = false
	if g.gameState != nil && g.gameState.EventQueueManager != nil {
		if g.gameState.IsCombatPartySpellTargeting() {
			g.gameState.CancelCombatSpellTargeting()
			g.secondaryKeyState = PrimaryInput
		}
		g.gameState.EventQueueManager.Shutdown()
	}
	g.clearCombatMovement()
	clear(g.combatImpactEffects)
	g.combatImpactEffects = g.combatImpactEffects[:0]
	g.clearDungeonFieldDispels()
	g.clearActorAnimationClocks()
	g.clearTimedOverlay()
	g.clearDungeonInjury()
	g.clearTransientActionFeedback()
	g.clearTransientIcons()
	g.clearShadowlordPresentationForSceneTransition()
	g.clearBattleSummaries()
	g.clearLordBritishFinalePresentation(false)
	g.avatarDissolve.Cleanup()
	g.wandererDissolve.Cleanup()
	g.clearCombatVanishEffects()
	g.clearCombatSignatureEffects()
	g.combatAllegiance = combatAllegianceState{}
	g.clearCombatInvocationEffects()
	g.clearCombatTeleportEffect()
	for index := range g.dungeonSkeletonCrumbles {
		g.dungeonSkeletonCrumbles[index].cleanup()
	}
	g.dungeonSkeletonCrumbles = nil
	for index := range g.dungeonPitReveals {
		g.dungeonPitReveals[index].cleanup()
	}
	g.dungeonPitReveals = nil
	g.StopAllTileDissolves()
	g.visualEffects = g.visualEffects[:0]
	g.screenFlashIntensity = 0
	g.screenShakeOffsetX = 0
	g.screenShakeOffsetY = 0
	g.screenInvertIntensity = 0
	g.inputBlocked = false
	if g.characterSummary != nil {
		g.characterSummary.Dispose()
	}
	if g.audioManager == nil {
		return
	}
	if err := g.audioManager.Close(); err != nil {
		logging.Warnf(logging.CategoryAudio, "Failed to close audio during scene disposal: %v", err)
	}
	g.audioManager = nil
}

func (g *GameScene) clearTransientIcons() {
	if g.activeEffectsOverlay != nil {
		g.activeEffectsOverlay.Clear()
	}
}
