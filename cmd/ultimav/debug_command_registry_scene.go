package main

import (
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

func registerCoreSceneDebugCommands(registry *debugCommandRegistry) {
	coreHandler := func(context debugCommandContext, request debugging.Request) debugging.Result {
		if request.CommandID == debugging.CommandMoongateStatus {
			return executeMoongateStatusWithOverlay(
				request,
				context.scene.gameState,
				context.scene.overlayRenderer.IsMoonstoneOverlayEnabled(),
			)
		}

		return executeDebugCommand(context.scene.gameState, request)
	}
	registry.registerGroup("core gameplay", debugCommandScene, coreHandler,
		debugging.CommandEnvironmentWindStatus,
		debugging.CommandEnvironmentWindSet,
		debugging.CommandShrineStatus,
		debugging.CommandShrineProgressUpdate,
		debugging.CommandPartyCharacterInspect,
		debugging.CommandPartyCharacterUpdate,
		debugging.CommandPartyKarmaSet,
		debugging.CommandCalendarStatus,
		debugging.CommandCalendarSetTime,
		debugging.CommandCalendarSetDay,
		debugging.CommandSearchStatus,
		debugging.CommandSearchItemStackAdd,
		debugging.CommandSearchProgressReset,
		debugging.CommandQuestItemStatus,
		debugging.CommandWorldPositionStatus,
		debugging.CommandMoongateStatus,
	)
	for _, commandID := range []string{debugging.CommandQuestItemEnsure, debugging.CommandQuestItemRemove} {
		registry.registerScene(commandID, "core gameplay", coreHandler, ritualStateMutationGuard)
	}
	for _, commandID := range []string{
		debugging.CommandPartyMaximizeLevels, debugging.CommandPartyRestore,
		debugging.CommandPartyFill, debugging.CommandPartyMemberAdd,
	} {
		registry.registerScene(commandID, "party roster", coreHandler, nil)
	}
	registry.registerScene(
		debugging.CommandPartyEquipmentLoadoutUpdate,
		"party equipment",
		coreHandler,
		equipmentMutationGuard,
	)
}

func registerSecondBatchDebugCommands(registry *debugCommandRegistry) {
	stateHandler := func(context debugCommandContext, request debugging.Request) debugging.Result {
		return executeSecondBatchStateDebugCommand(request, context.scene.gameState)
	}
	registry.registerGroup("game state tools", debugCommandScene, stateHandler,
		debugging.CommandCombatEncounterOptions,
		debugging.CommandCombatEncounterStart,
		debugging.CommandMonsterEcosystemStatus,
		debugging.CommandMonsterGenerationSet,
		debugging.CommandMonsterClearMapEnemies,
		debugging.CommandMonsterSpawnWhirlpool,
		debugging.CommandMonsterSpawnPirate,
		debugging.CommandVehicleStatus,
		debugging.CommandVehicleSpawn,
		debugging.CommandVehicleForceBoard,
		debugging.CommandShipDamageRaw,
		debugging.CommandShipRepairFull,
		debugging.CommandShipSkiffSet,
	)
}

func registerThirdBatchDebugCommands(registry *debugCommandRegistry) {
	handler := sceneHandledDebugCommand(func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
		return scene.executeThirdBatchDebugCommand(request)
	})
	registry.registerScene(debugging.CommandTestHoleUp, "hole-up workflow", handler, nil)
	registry.registerGroup("command workflows", debugCommandScene, handler,
		debugging.CommandTestSearch,
		debugging.CommandMoongateTravel,
	)
}

func registerFourthBatchDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("quest workflows", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeFourthBatchDebugCommand(request)
		}),
		debugging.CommandQuestShardUse,
		debugging.CommandQuestInventoryPresetUsable,
	)
}

func registerFifthBatchDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("observation workflows", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeFifthBatchDebugCommand(request)
		}),
		debugging.CommandTestTelescope,
		debugging.CommandTestSpyglass,
		debugging.CommandTestCrystalBall,
	)
}

func registerSixthBatchDebugCommands(registry *debugCommandRegistry) {
	handler := sceneHandledDebugCommand(func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
		return scene.executeSixthBatchDebugCommand(request)
	})
	registry.registerScene(debugging.CommandDeathKillParty, "death testing", handler, nil)
	registry.registerScene(debugging.CommandDeathTestSequence, "death testing", handler, nil)
}

func registerSeventhBatchDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("quest state", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeSeventhBatchDebugCommand(request)
		}),
		debugging.CommandCodexQuestStateSetAll,
	)
}

func registerTestSetupDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("test setups", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeTestSetupDebugCommand(request)
		}),
		debugging.CommandTestSetupStatus,
	)
}

func registerWorldSetupDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("world setup", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeWorldSetupDebugCommand(request)
		}),
		debugging.CommandMovementLandmarkTravel,
	)
}

func registerWorldNavigatorDebugCommands(registry *debugCommandRegistry) {
	handler := sceneHandledDebugCommand(func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
		return scene.executeWorldNavigatorDebugCommand(request)
	})
	registry.registerScene(debugging.CommandWorldPositionSet, "world navigator", handler, nil)
}

func registerOperationalDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("operations", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeOperationalDebugCommand(request)
		}),
		debugging.CommandGuardsStatus,
		debugging.CommandShadowlordStatus,
		debugging.CommandDeathStatus,
		debugging.CommandDebugOptionsStatus,
		debugging.CommandMovementFreeMoveSet,
		debugging.CommandTextPlaceholdersSet,
		debugging.CommandDialogBoundsSet,
		debugging.CommandUnlimitedCastingResourcesSet,
		debugging.CommandSpellLevelRestrictionsBypassSet,
	)
}

func registerPresentationDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("presentation", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executePresentationDebugCommand(request)
		}),
		debugging.CommandLightingOverlaySet,
		debugging.CommandSearchOverlaySet,
		debugging.CommandMoongateOverlaySet,
	)
}

func registerInventoryLockerDebugCommands(registry *debugCommandRegistry) {
	handler := func(context debugCommandContext, request debugging.Request) debugging.Result {
		result, handled := executeInventoryLockerDebugCommand(request, context.scene.gameState)
		if !handled {
			panic("registered inventory locker command was not handled: " + request.CommandID)
		}

		return result
	}
	registry.registerGroup("inventory locker", debugCommandScene, handler,
		debugging.CommandInventoryLockerStatus,
		debugging.CommandInventoryProvisionSet,
		debugging.CommandInventoryProvisionPreset,
		debugging.CommandInventoryReagentSet,
		debugging.CommandInventoryReagentPreset,
		debugging.CommandInventoryPotionAdd,
		debugging.CommandInventoryPotionPreset,
		debugging.CommandInventoryScrollAdd,
		debugging.CommandInventoryScrollPreset,
		debugging.CommandInventoryEquipmentSet,
		debugging.CommandInventoryEquipmentPreset,
		debugging.CommandInventoryEquipmentGrantPreset,
	)
	registry.registerScene(
		debugging.CommandInventoryEquipmentClear,
		"inventory locker",
		handler,
		equipmentMutationGuard,
	)
}

func registerSpellDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("prepared spells", debugCommandScene, func(
		context debugCommandContext,
		request debugging.Request,
	) debugging.Result {
		result, handled := executeSpellDebugCommand(request, context.scene.gameState)
		if !handled {
			panic("registered spell command was not handled: " + request.CommandID)
		}

		return result
	},
		debugging.CommandInventorySpellStatus,
		debugging.CommandInventorySpellSet,
		debugging.CommandInventorySpellPresetMax,
		debugging.CommandSpellEffectSetRemaining,
		debugging.CommandSpellEffectActivate,
	)
}

func registerActiveCombatDebugCommands(registry *debugCommandRegistry) {
	handler := func(context debugCommandContext, request debugging.Request) debugging.Result {
		if request.CommandID == debugging.CommandCombatStatus {
			data := newActiveCombatResult(context.scene.gameState)
			data.ConfigurationEditing = context.scene.gameConfig.ConfigurationEditing()
			data.PresentationSpeedPercent = int(
				context.scene.gameConfig.GetCombatSpeedFactor() * debugPercentScale,
			)

			return debugSuccessResult(request, "Active combat status loaded.", data)
		}
		result, handled := executeActiveCombatDebugCommand(request, context.scene.gameState)
		if !handled {
			panic("registered active combat command was not handled: " + request.CommandID)
		}

		return result
	}
	registry.registerGroup("active combat", debugCommandScene, handler,
		debugging.CommandCombatStatus,
		debugging.CommandRNGSeedSet,
		debugging.CommandCombatMonsterDamageRaw,
		debugging.CommandCombatMonsterKill,
		debugging.CommandCombatMonsterKillAll,
		debugging.CommandCombatMonsterClear,
	)
	for _, commandID := range []string{
		debugging.CommandCombatMonsterSleepSet,
		debugging.CommandCombatMonsterSleepAll,
		debugging.CommandCombatExitDebug,
		debugging.CommandCombatMonsterSpawn,
		debugging.CommandCombatDungeonRoomClear,
	} {
		registry.registerScene(commandID, "active combat", handler, activeCombatMutationGuard)
	}
}

func registerMerchantTesterDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("merchant tester", debugCommandScene, sceneHandledDebugCommand(
		func(scene *GameScene, request debugging.Request) (debugging.Result, bool) {
			return scene.executeMerchantTesterDebugCommand(request)
		}),
		debugging.CommandMerchantTesterCatalog,
		debugging.CommandMerchantTesterPreviewOpen,
		debugging.CommandMerchantTesterLiveOpen,
	)
}

func ritualStateMutationGuard(scene *GameScene, request debugging.Request) *debugging.Result {
	return scene.thirdBatchWorkflowBlocked(request)
}

func equipmentMutationGuard(scene *GameScene, request debugging.Request) *debugging.Result {
	if scene.dialogStack.HasOpenDialog() {
		result := debugging.FailureResult(
			request, "dialog_active", "equipment mutation is unavailable while a dialog is open",
		)

		return &result
	}
	if scene.gameState.EventQueueManager.HasActiveWorkflows() {
		result := debugging.FailureResult(
			request, "gameplay_workflow_active", "equipment mutation is unavailable during a gameplay workflow",
		)

		return &result
	}

	return nil
}

func activeCombatMutationGuard(scene *GameScene, request debugging.Request) *debugging.Result {
	if scene.dialogStack.HasOpenDialog() {
		result := debugging.FailureResult(
			request, "dialog_active", "active combat mutations are unavailable while a dialog is open",
		)

		return &result
	}
	if scene.combatMovement.Active {
		result := debugging.FailureResult(
			request, "combat_busy", "active combat mutations are unavailable during movement",
		)

		return &result
	}

	return nil
}
