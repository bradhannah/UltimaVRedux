package main

import (
	"errors"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/debugging"
)

type debugCommandScope uint8

const (
	debugCommandApplication debugCommandScope = iota
	debugCommandApplicationValidated
	debugCommandScene
)

type debugCommandContext struct {
	game  *Game
	scene *GameScene
}

type debugCommandHandler func(debugCommandContext, debugging.Request) debugging.Result
type debugCommandGuard func(*GameScene, debugging.Request) *debugging.Result

type debugCommandRegistration struct {
	commandID string
	owner     string
	scope     debugCommandScope
	handler   debugCommandHandler
	guard     debugCommandGuard
}

type debugCommandRegistry struct {
	registrations map[string]debugCommandRegistration
}

var (
	errDebugRegistryCatalogRequired = errors.New("debug command registry requires a catalog")
	errDebugRegistrationInvalid     = errors.New("debug command registration requires an ID, owner, and handler")
	errDebugRegistrationMissing     = errors.New("catalog command has no application registration")
	errDebugRegistrationUnknown     = errors.New("registered debug command is absent from the catalog")
	errDebugRegistrationDuplicate   = errors.New("debug command already has an execution owner")
)

//nolint:gochecknoglobals // Immutable startup registry; command handlers never mutate it after construction.
var initialDebugCommandRegistry = mustNewDebugCommandRegistry(debugging.NewInitialCatalog())

func newEmptyDebugCommandRegistry() *debugCommandRegistry {
	return &debugCommandRegistry{registrations: make(map[string]debugCommandRegistration)}
}

func newDebugCommandRegistry(catalog *debugging.Catalog) (*debugCommandRegistry, error) {
	if catalog == nil {
		return nil, errDebugRegistryCatalogRequired
	}
	registry := newEmptyDebugCommandRegistry()
	registerCoreSceneDebugCommands(registry)
	registerApplicationDebugCommands(registry)
	registerSecondBatchDebugCommands(registry)
	registerThirdBatchDebugCommands(registry)
	registerFourthBatchDebugCommands(registry)
	registerFifthBatchDebugCommands(registry)
	registerSixthBatchDebugCommands(registry)
	registerSeventhBatchDebugCommands(registry)
	registerWorldSetupDebugCommands(registry)
	registerWorldNavigatorDebugCommands(registry)
	registerOperationalDebugCommands(registry)
	registerPresentationDebugCommands(registry)
	registerInventoryLockerDebugCommands(registry)
	registerSpellDebugCommands(registry)
	registerActiveCombatDebugCommands(registry)
	registerMerchantTesterDebugCommands(registry)
	registerCombatSpeedDebugCommand(registry)
	registerTestSetupDebugCommands(registry)
	registerSnapshotDebugCommands(registry)

	if err := validateDebugCommandRegistry(catalog, registry); err != nil {
		return nil, err
	}

	return registry, nil
}

func validateDebugCommandRegistry(catalog *debugging.Catalog, registry *debugCommandRegistry) error {
	definitions := catalog.Definitions()
	catalogIDs := make(map[string]struct{}, len(definitions))
	for _, definition := range definitions {
		catalogIDs[definition.ID] = struct{}{}
		_, ok := registry.registrations[definition.ID]
		if !ok {
			return fmt.Errorf("%w: %q", errDebugRegistrationMissing, definition.ID)
		}
	}
	for commandID := range registry.registrations {
		if _, ok := catalogIDs[commandID]; !ok {
			return fmt.Errorf("%w: %q", errDebugRegistrationUnknown, commandID)
		}
	}

	return nil
}

func mustNewDebugCommandRegistry(catalog *debugging.Catalog) *debugCommandRegistry {
	registry, err := newDebugCommandRegistry(catalog)
	if err != nil {
		panic("constructing debug command registry: " + err.Error())
	}

	return registry
}

func (r *debugCommandRegistry) register(registration debugCommandRegistration) error {
	if registration.commandID == "" || registration.owner == "" || registration.handler == nil {
		return errDebugRegistrationInvalid
	}
	if _, exists := r.registrations[registration.commandID]; exists {
		return fmt.Errorf("%w: %q", errDebugRegistrationDuplicate, registration.commandID)
	}
	r.registrations[registration.commandID] = registration

	return nil
}

func (r *debugCommandRegistry) registerGroup(
	owner string,
	scope debugCommandScope,
	handler debugCommandHandler,
	commandIDs ...string,
) {
	for _, commandID := range commandIDs {
		if err := r.register(debugCommandRegistration{
			commandID: commandID,
			owner:     owner,
			scope:     scope,
			handler:   handler,
			guard:     nil,
		}); err != nil {
			panic(err)
		}
	}
}

func (r *debugCommandRegistry) registerScene(
	commandID string,
	owner string,
	handler debugCommandHandler,
	guard debugCommandGuard,
) {
	if err := r.register(debugCommandRegistration{
		commandID: commandID,
		owner:     owner,
		scope:     debugCommandScene,
		handler:   handler,
		guard:     guard,
	}); err != nil {
		panic(err)
	}
}

func (r *debugCommandRegistry) lookup(commandID string) (debugCommandRegistration, bool) {
	registration, ok := r.registrations[commandID]

	return registration, ok
}

func (r *debugCommandRegistry) executeScene(scene *GameScene, request debugging.Request) debugging.Result {
	registration, ok := r.lookup(request.CommandID)
	if !ok || registration.scope != debugCommandScene {
		return debugging.FailureResult(
			request,
			"command_not_implemented",
			"the active gameplay scene does not implement this command",
		)
	}
	if registration.guard != nil {
		if blocked := registration.guard(scene, request); blocked != nil {
			return *blocked
		}
	}

	return registration.handler(debugCommandContext{game: nil, scene: scene}, request)
}

func sceneHandledDebugCommand(
	handler func(*GameScene, debugging.Request) (debugging.Result, bool),
) debugCommandHandler {
	return func(context debugCommandContext, request debugging.Request) debugging.Result {
		result, handled := handler(context.scene, request)
		if !handled {
			panic(fmt.Sprintf("registered scene owner did not handle debug command %q", request.CommandID))
		}

		return result
	}
}

func applicationHandledDebugCommand(
	handler func(*Game, debugging.Request) (debugging.Result, bool),
) debugCommandHandler {
	return func(context debugCommandContext, request debugging.Request) debugging.Result {
		result, handled := handler(context.game, request)
		if !handled {
			panic(fmt.Sprintf("registered application owner did not handle debug command %q", request.CommandID))
		}

		return result
	}
}
