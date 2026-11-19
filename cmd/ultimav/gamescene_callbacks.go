package main

import (
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
)

// SystemCallbacks wiring and dependency injection setup
// Handles creation and injection of all callback interfaces into GameState

// wireSystemCallbacks creates and wires up all SystemCallbacks for dependency injection
func (g *GameScene) wireSystemCallbacks() error {
	// Wire up system callbacks dependency injection using constructors
	messageCallbacks := game_state.NewMessageCallbacks(game_state.MessageCallbacks{
		AddRowStr:             g.addRowStr,
		AppendToCurrentRowStr: g.appendToCurrentRowStr,
		ShowCommandPrompt:     g.showCommandPrompt,
		AppendStyledMessage:   g.appendStyledMessage,
	})

	visualCallbacks := game_state.NewVisualCallbacks(game_state.VisualCallbacks{
		SetWandererDissolveLevel:                  g.setWandererDissolveLevel,
		InitializeWandererDissolveEffect:          g.initializeWandererDissolveEffect,
		CleanupWandererDissolveEffect:             g.cleanupWandererDissolveEffect,
		SetAvatarDissolveLevel:                    g.setAvatarDissolveLevel,
		InitializeAvatarDissolveEffect:            g.initializeAvatarDissolveEffect,
		CleanupAvatarDissolveEffect:               g.cleanupAvatarDissolveEffect,
		SetMapFadeLevel:                           g.setMapFadeLevel,
		SetScreenFlashColor:                       g.setScreenFlashColor,
		SetScreenFlashIntensity:                   g.setScreenFlashIntensity,
		SetScreenShakeOffset:                      g.setScreenShakeOffset,
		SetScreenInvert:                           g.setScreenInvert,
		BlockAllInput:                             g.blockAllInput,
		IsAnyKeyPressed:                           g.IsAnyKeyPressed,
		GetPressedKey:                             g.GetPressedKey,
		ResetTeleportationSourceRenderer:          g.resetTeleportationSourceRenderer,
		StabilizeTeleportationDestinationRenderer: g.stabilizeTeleportationDestinationRenderer,
		StartProjectile:                           g.startProjectile,
		UpdateProjectileProgress:                  g.updateProjectileProgress,
		ClearProjectile:                           g.clearProjectile,
		PlayKapowAtPosition:                       g.playKapowAtPosition,
		ShowCombatImpact:                          g.showCombatImpact,
		ShowDungeonInjury:                         g.showDungeonInjury,
	})

	// Note: Death scene dissolve callbacks removed - now uses generic tile dissolve system
	// See gamescene_dissolve.go: StartTileDissolveWithOptions()

	audioCallbacks := game_state.NewAudioCallbacks(
		g.playSound,
		g.stopSound,
		g.updateMusicForCurrentLocation, // Wire to location-based music system
		g.pauseBackgroundMusic,          // Wire to background music pause functionality
		g.resumeBackgroundMusic,         // Wire to background music resume functionality
		g.playSoundVoice,
	)

	screenCallbacks := game_state.NewScreenCallbacks(game_state.ScreenCallbacks{
		UpdateDisplay:                    g.updateDisplay,
		HideAllOverlays:                  g.HideAllOverlays,
		ShowAllOverlays:                  g.ShowAllOverlays,
		CreateAndPushMessageDialog:       g.createAndPushMessageDialog,
		CreateAndPushCampingResultDialog: g.createAndPushCampingResultDialog,
		PopMessageDialog:                 g.popMessageDialog,
		HideMessageDialog:                g.hideMessageDialog,
		ClearMessageDialog:               g.clearMessageDialog,
		SetMessagePrompt:                 g.setMessagePrompt,
		SetMessagePromptColor:            g.setMessagePromptWithColor,
		ClearMessagePrompt:               g.clearMessagePrompt,
		SetMessageDialogFadeDurations:    g.setMessageDialogFadeDurations,
		GetMessageDialogFadeInDuration:   g.getMessageDialogFadeInDuration,
		GetMessageDialogFadeOutDuration:  g.getMessageDialogFadeOutDuration,
		ShowPressAnyKeyPrompt:            g.showPressAnyKeyPrompt,
	})

	flowCallbacks := game_state.NewFlowCallbacks(game_state.FlowCallbacks{
		Consequences: game_state.FlowConsequenceCallbacks{
			TriggerDeathSequence: g.triggerDeathSequence,
			TriggerStonegateTrap: g.StartStonegateTrapWorkflow,
		},
		Rest: game_state.FlowRestCallbacks{
			RequestSleepDuration: g.RequestSleepDuration,
			RequestCampDuration:  g.RequestCampDuration,
		},
		WorldInteraction: game_state.FlowWorldInteractionCallbacks{
			ShowWellInteraction:     g.ShowWellInteraction,
			ShowFountainInteraction: g.ShowFountainInteraction,
			ShowHorseRefusalDialog:  g.ShowHorseRefusalDialog,
		},
		Observation: game_state.FlowObservationCallbacks{
			ShowTelescopeInteraction:   g.ShowTelescopeInteraction,
			ShowCrystalBallInteraction: g.ShowCrystalBallInteraction,
			ShowGemViewInteraction:     g.ShowGemViewInteraction,
			ShowSpyglassInteraction:    g.ShowSpyglassInteraction,
			ShowPocketWatchTime:        g.ShowPocketWatchTime,
		},
		Shadowlord: game_state.FlowShadowlordCallbacks{
			TriggerShadowlordSummoned:    g.TriggerShadowlordSummoned,
			ThenShadowlordPresenceEffect: g.ThenShadowlordPresenceEffect,
			ShowShardUsageWorkflow:       g.startShardWorkflow,
		},
		Items: game_state.FlowItemCallbacks{
			StartPotionWorkflow:             g.startPotionWorkflow,
			StartWindChangeScrollWorkflow:   g.startWindChangeScrollWorkflow,
			StartLightScrollWorkflow:        g.startLightScrollWorkflow,
			StartTimeStopScrollWorkflow:     g.startTimeStopScrollWorkflow,
			StartResurrectionScrollWorkflow: g.startResurrectionScrollWorkflow,
			TriggerToggleItemFlash:          g.TriggerToggleItemFlash,
			StartMoonstoneBurialWorkflow:    g.startMoonstoneBurialWorkflow,
			StartCarpetBoardingWorkflow:     g.startCarpetBoardingWorkflow,
			StartSkullKeyWorkflow:           g.startSkullKeyWorkflow,
			StartHMSCapeWorkflow:            g.startHMSCapeWorkflow,
			StartSceptreWorkflow:            g.startSceptreWorkflow,
			ShowReadyDialog:                 g.startReadyEquipmentWorkflow,
		},
		SacredSites: game_state.FlowSacredSiteCallbacks{
			StartShrineWorkflow: g.StartShrineWorkflow,
			StartCodexWorkflow:  g.StartCodexWorkflow,
		},
	})

	talkCallbacks := game_state.NewTalkCallbacks(
		g.CreateTalkDialog,
		g.PushDialog,
	)

	uiCallbacks := game_state.UICallbacks{
		ShowZStatsDossier:       g.ShowZStatsDossier,
		ShowQuestJournalDossier: g.ShowQuestJournalDossier,
		JournalChanged:          g.journalChanged,
		AtlasUpdated:            g.atlasUpdated,
		ShowYellDialogLargeMap:  g.ShowYellDialogLargeMap,
		ShowTimedOverlayMessage: g.ShowTimedOverlayMessage,
		ShowActionFeedback:      g.ShowActionFeedback,
		ShowSearchFeedback:      g.ShowSearchFeedback,
		ShowSpellExpiredPopup:   g.showSpellExpiredPopup,
		ShowTransientIcon: func(kind game_state.TransientIconKind) {
			g.activeEffectsOverlay.RegisterTransient(kind, g.gameState.ElapsedMs)
		},
		ShowSignDialog: g.ShowSignDialog,
		ShowExperienceAward: func(award game_state.DeedExperienceAward) {
			g.showExperienceAward(award, true)
		},
	}
	combatCallbacks := game_state.NewCombatCallbacks(g.recordCombatEvent, g.recordCombatConclusion)

	systemCallbacks, err := game_state.NewSystemCallbacks(
		messageCallbacks,
		visualCallbacks,
		audioCallbacks,
		screenCallbacks,
		flowCallbacks,
		talkCallbacks,
		uiCallbacks,
		combatCallbacks,
	)
	if err != nil {
		return fmt.Errorf("creating system callbacks: %w", err)
	}

	g.gameState.SystemCallbacks = systemCallbacks

	// Update environmental systems with proper SystemCallbacks
	g.gameState.SetupEnvironmentalSystemCallbacks()

	// Note: Teleportation visual sequence is now managed by event queue system
	// in internal/game_state/teleportation_sequence.go - no initialization needed

	return nil
}

func (g *GameScene) recordCombatConclusion(conclusion game_state.CombatConclusion) {
	g.clearCombatSignatureEffects()
	g.combatAllegiance = combatAllegianceState{}
	g.clearCombatInvocationEffects()
	g.clearCombatTeleportEffect()
	if conclusion.Special == game_state.CombatSpecialConclusionNone {
		g.combatConclusion = nil
		conclusion.Collected = append([]game_state.CombatCollectedItem(nil), conclusion.Collected...)
		g.pendingBattleSummaries = append(g.pendingBattleSummaries, conclusion)

		return
	}
	g.combatConclusion = &conclusion
}
