package main

import "github.com/bradhannah/UltimaVRedux/internal/debugging"

func registerApplicationDebugCommands(registry *debugCommandRegistry) {
	registry.registerGroup("application diagnostics", debugCommandApplication, func(
		context debugCommandContext,
		request debugging.Request,
	) debugging.Result {
		result, handled := executeApplicationDebugCommand(request, context.game.currentScene.GetUltimaConfiguration())
		if !handled {
			panic("registered application command was not handled: " + request.CommandID)
		}

		return result
	},
		debugging.CommandLoggingStatus,
		debugging.CommandLoggingEnabledSet,
		debugging.CommandLoggingStdoutSet,
		debugging.CommandLoggingLevelSet,
		debugging.CommandLoggingCategorySet,
		debugging.CommandLoggingCategoriesSetAll,
	)
}

func registerCombatSpeedDebugCommand(registry *debugCommandRegistry) {
	registry.registerGroup("combat presentation speed", debugCommandApplication, func(
		context debugCommandContext,
		request debugging.Request,
	) debugging.Result {
		return executeCombatSpeedDebugCommand(request, context.game.currentScene.GetUltimaConfiguration())
	},
		debugging.CommandCombatPresentationSpeedSet,
	)
}

func registerSnapshotDebugCommands(registry *debugCommandRegistry) {
	queryHandler := applicationHandledDebugCommand(func(game *Game, request debugging.Request) (debugging.Result, bool) {
		return game.executeSnapshotQueryCommand(request)
	})
	registry.registerGroup("snapshot repository", debugCommandApplication, queryHandler,
		debugging.CommandSaveSnapshotList,
		debugging.CommandSaveSnapshotGet,
	)
	mutationHandler := applicationHandledDebugCommand(
		func(game *Game, request debugging.Request) (debugging.Result, bool) {
			return game.executeSnapshotMutationCommand(request)
		},
	)
	for _, commandID := range []string{
		debugging.CommandSaveSnapshotCapture,
		debugging.CommandSaveSnapshotLoad,
		debugging.CommandSaveSnapshotDelete,
	} {
		if err := registry.register(debugCommandRegistration{
			commandID: commandID,
			owner:     "snapshot repository",
			scope:     debugCommandApplicationValidated,
			handler:   mutationHandler,
			guard:     nil,
		}); err != nil {
			panic(err)
		}
	}
}
