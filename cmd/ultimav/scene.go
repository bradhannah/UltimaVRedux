package main

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"sync/atomic"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/debugging"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/player_save"
	"github.com/bradhannah/UltimaVRedux/internal/profiling"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	rng "github.com/bradhannah/berad-sdk/modules/berad-rng"
)

const developmentGameVersion = "development"

// Scene interface that requires Update and Draw methods
type Scene interface {
	Update(game *Game) error
	Draw(screen *ebiten.Image)
	GetUltimaConfiguration() *config.UltimaVConfiguration
	InvalidateResolution()
}

// Game struct that holds the current scene
type Game struct {
	atlasTexture                      atlasTextureOwner
	currentScene                      Scene
	musicHistory                      audio.MusicHistory
	debugBridge                       *debugging.Bridge
	debugCatalog                      *debugging.Catalog
	debugCommandRegistry              *debugCommandRegistry
	debugDiscovery                    atomic.Pointer[debugging.DiscoverySnapshot]
	debugDefinitions                  []debugging.CommandDefinition
	debugDiscoveryPolling             atomic.Bool
	debugDiscoveryRetired             atomic.Bool
	sceneGeneration                   atomic.Uint64
	snapshotRepository                snapshotRepository
	snapshotSceneBuilder              func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error)
	snapshotNow                       func() time.Time
	snapshotGameVersion               string
	playerSaveRepository              playerSaveRepository
	notebookRoot                      string
	playerSaveSceneBuilder            func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error)
	playerSaveNow                     func() time.Time
	playerSaveGameVersion             string
	doomRecoveryRepository            doomRecoveryRepository
	doomRecoverySceneBuilder          func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error)
	doomRecoveryNow                   func() time.Time
	doomRunID                         func() (string, error)
	playthroughIDProvider             func() (string, error)
	characterCreationSeedProvider     characterCreationSeedProvider
	characterCreationSceneBuilder     characterCreationSceneBuilder
	characterCreationSnapshotPreparer characterCreationSnapshotPreparer
	mainMenuSceneBuilder              func(*config.UltimaVConfiguration) (Scene, error)
	exitRequested                     bool
	shutdown                          bool
	windowCloseRequested              func() bool
}

// windowCloseHandler lets a scene keep the window open to offer saving first.
type windowCloseHandler interface {
	HandleWindowCloseRequest(game *Game) bool
}

// ConfigureSnapshotRepository injects native snapshot persistence and dormant scene construction.
func (g *Game) ConfigureSnapshotRepository(
	repository snapshotRepository,
	builder func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error),
) {
	g.snapshotRepository = repository
	g.snapshotSceneBuilder = builder
}

// ConfigurePlayerSaveRepository injects player-slot persistence and dormant
// native scene construction without coupling Game to filesystem policy.
func (g *Game) ConfigurePlayerSaveRepository(
	repository playerSaveRepository,
	builder func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error),
) {
	g.playerSaveRepository = repository
	g.playerSaveSceneBuilder = builder
	g.publishPlayerSaveCatalog()
}

// ConfigureDoomRecoveryRepository injects the dedicated pre-Doom checkpoint
// persistence and dormant scene construction services.
func (g *Game) ConfigureDoomRecoveryRepository(
	repository doomRecoveryRepository,
	builder func(*config.UltimaVConfiguration, save_snapshot.SaveSnapshot) (Scene, error),
) {
	g.doomRecoveryRepository = repository
	g.doomRecoverySceneBuilder = builder
}

type playerSaveCatalogObserver interface {
	applyPlayerSaveCatalog(summaries []player_save.SlotSummary)
}

func (g *Game) publishPlayerSaveCatalog() {
	observer, ok := g.currentScene.(playerSaveCatalogObserver)
	if !ok {
		return
	}
	if g.playerSaveRepository == nil {
		observer.applyPlayerSaveCatalog(nil)

		return
	}
	observer.applyPlayerSaveCatalog(g.playerSaveRepository.List())
}

const maxDebugRequestsPerUpdate = 8

// NewGame constructs the application with mandatory debug control-plane services.
func NewGame(initialScene Scene) *Game {
	if initialScene == nil {
		panic("NewGame: initial scene is required")
	}

	game := &Game{
		currentScene:                  initialScene,
		debugBridge:                   debugging.NewDefaultBridge(),
		debugCatalog:                  debugging.NewInitialCatalog(),
		debugCommandRegistry:          initialDebugCommandRegistry,
		debugDiscovery:                atomic.Pointer[debugging.DiscoverySnapshot]{},
		sceneGeneration:               atomic.Uint64{},
		snapshotNow:                   time.Now,
		snapshotGameVersion:           developmentGameVersion,
		playerSaveNow:                 time.Now,
		playerSaveGameVersion:         runtimeGameVersion(),
		doomRecoveryRepository:        nil,
		doomRecoverySceneBuilder:      nil,
		doomRecoveryNow:               time.Now,
		doomRunID:                     newDoomRunID,
		playthroughIDProvider:         newPlaythroughID,
		characterCreationSeedProvider: newCharacterCreationSeed,
		characterCreationSceneBuilder: func(
			configuration *config.UltimaVConfiguration,
			source *rng.Source,
			callbacks CharacterCreationCallbacks,
			identityPresentation *characterCreationIdentityPresentation,
		) (Scene, error) {
			return CreateCharacterCreationScene(configuration, source, callbacks, identityPresentation)
		},
		characterCreationSnapshotPreparer: prepareCharacterCreationSnapshot,
		mainMenuSceneBuilder: func(configuration *config.UltimaVConfiguration) (Scene, error) {
			return CreateMainMenuScene(configuration)
		},
		exitRequested:        false,
		windowCloseRequested: ebiten.IsWindowBeingClosed,
	}
	game.sceneGeneration.Store(1)
	game.debugDefinitions = game.debugCatalog.Definitions()
	displayManager := display.GetManager()
	previous := displayManager.SetMapPopupViewportProvider(game.mapPopupViewport)
	defer displayManager.SetMapPopupViewportProvider(previous)
	activateScene(initialScene)
	game.publishDebugDiscovery(nil, 1)

	return game
}

// SubmitDebugRequest queues a command for execution on the update thread.
func (g *Game) SubmitDebugRequest(request debugging.Request) error {
	if err := g.debugBridge.Submit(request); err != nil {
		return fmt.Errorf("submitting debug request: %w", err)
	}

	return nil
}

// NextDebugResult returns the next completed command result without blocking.
func (g *Game) NextDebugResult() (debugging.Result, bool) {
	return g.debugBridge.NextResult()
}

// DebugDiscoverySnapshot returns detached command and dynamic-option discovery data.
func (g *Game) DebugDiscoverySnapshot() debugging.DiscoverySnapshot {
	snapshot := g.debugDiscovery.Load()
	if snapshot == nil {
		panic("Game.DebugDiscoverySnapshot: discovery snapshot is required")
	}

	return debugging.CloneDiscoverySnapshot(*snapshot)
}

// DebugSceneGeneration identifies the active scene for stale-request protection.
func (g *Game) DebugSceneGeneration() uint64 {
	return g.sceneGeneration.Load()
}

type gameDebugExecutor struct {
	game *Game
}

func (e gameDebugExecutor) ExecuteDebugCommand(request debugging.Request) debugging.Result {
	return e.game.ExecuteLocalDebugCommand(request)
}

// ExecuteLocalDebugCommand runs a request synchronously on the update thread.
// It shares the browser path so catalog normalization, stale checks, and result
// stamping remain identical for all Workbench callers.
func (g *Game) ExecuteLocalDebugCommand(request debugging.Request) debugging.Result {
	result := g.executeDebugCommand(request)
	g.refreshDebugDiscoverySnapshot()

	return result
}

func (g *Game) executeDebugCommand(request debugging.Request) debugging.Result {
	normalized, err := g.debugCatalog.NormalizeRequest(request)
	if err != nil {
		return g.withSceneGeneration(debugging.FailureResult(request, "invalid_request", err.Error()))
	}
	request = normalized
	invocation := debugging.NewNormalizedInvocation(request)
	registration, ok := g.debugCommandRegistry.lookup(request.CommandID)
	if !ok {
		panic("normalized debug command missing from registry: " + request.CommandID)
	}
	if registration.scope == debugCommandApplication {
		result := registration.handler(debugCommandContext{game: g, scene: nil}, request)

		return g.withCommandCompletion(result, invocation)
	}

	sceneGeneration := g.sceneGeneration.Load()
	if request.ExpectedSceneGeneration != 0 && request.ExpectedSceneGeneration != sceneGeneration {
		return g.withCommandCompletion(debugging.FailureResult(
			request,
			"stale_scene",
			"the active scene changed after this request was prepared",
		), invocation)
	}
	definition, definitionFound := g.debugCatalog.Get(request.CommandID)
	if !definitionFound {
		panic("normalized debug command missing from catalog: " + request.CommandID)
	}
	if commandUsesDynamicOptions(definition) {
		// Requests can arrive while unsolicited discovery is dormant. Review
		// current choices on the update thread before checking the caller's revision.
		g.refreshDebugDiscoverySnapshot()
		discovery := g.debugDiscovery.Load()
		staleOptions := discovery == nil || request.ExpectedOptionsRevision == 0 ||
			request.ExpectedOptionsRevision != discovery.OptionsRevision
		if staleOptions {
			return g.withCommandCompletion(debugging.FailureResult(
				request,
				"stale_options",
				"the available command options changed after this request was prepared",
			), invocation)
		}
	}
	if registration.scope == debugCommandApplicationValidated {
		result := registration.handler(debugCommandContext{game: g, scene: nil}, request)

		return g.withCommandCompletion(result, invocation)
	}

	executor, ok := g.currentScene.(debugging.Executor)
	if !ok {
		return g.withCommandCompletion(debugging.FailureResult(
			request,
			"gameplay_unavailable",
			"this debug command requires an active gameplay scene",
		), invocation)
	}

	if scene, concrete := g.currentScene.(*GameScene); concrete {
		return g.withCommandCompletion(g.debugCommandRegistry.executeScene(scene, request), invocation)
	}

	return g.withCommandCompletion(executor.ExecuteDebugCommand(request), invocation)
}

func (g *Game) withCommandCompletion(result debugging.Result, invocation *debugging.CommandInvocation) debugging.Result {
	result.Invocation = invocation

	return g.withSceneGeneration(result)
}

func (g *Game) withSceneGeneration(result debugging.Result) debugging.Result {
	result.SceneGeneration = g.sceneGeneration.Load()

	return result
}

// ReplaceScene constructs a candidate before preparing or otherwise touching the
// active scene, then commits the replacement on the update thread.
func (g *Game) ReplaceScene(newScene func() (Scene, error)) error {
	if newScene == nil {
		panic("Game.ReplaceScene: scene factory is required")
	}
	scene, err := newScene()
	if err != nil {
		return err
	}
	if scene == nil {
		return errors.New("constructing replacement scene: scene is required")
	}

	return g.commitSceneReplacement(scene)
}

func (g *Game) returnToMainMenu(configuration *config.UltimaVConfiguration) error {
	if g.mainMenuSceneBuilder == nil {
		panic("Game.returnToMainMenu: main menu scene builder is required")
	}

	return g.ReplaceScene(func() (Scene, error) {
		return g.mainMenuSceneBuilder(configuration)
	})
}

func (g *Game) handleWindowCloseRequest() {
	if handler, ok := g.currentScene.(windowCloseHandler); ok && handler.HandleWindowCloseRequest(g) {
		return
	}
	g.requestExit()
}

func (g *Game) requestExit() {
	g.exitRequested = true
}

func (g *Game) commitSceneReplacement(scene Scene) error {
	oldScene := g.currentScene
	if preflight, ok := scene.(sceneActivationPreflighter); ok {
		if err := preflight.PreflightActivation(); err != nil {
			if disposer, disposable := scene.(sceneDisposer); disposable {
				disposer.Dispose()
			}

			return fmt.Errorf("preflight replacement scene activation: %w", err)
		}
	}
	if preparer, ok := scene.(sceneActivationInputPreparer); ok {
		preparer.PrepareInputForSceneActivation()
	}
	history := g.musicHistoryForReplacement()
	if candidate, ok := scene.(*GameScene); ok && candidate.audioManager != nil {
		candidate.audioManager.GetMusicManager().RestoreHistory(history)
	}
	if err := activateSceneSafely(scene); err != nil {
		if disposer, ok := scene.(sceneDisposer); ok {
			disposer.Dispose()
		}

		return err
	}
	if preparer, ok := oldScene.(sceneReplacementPreparer); ok {
		preparer.PrepareForSceneReplacement()
	}
	// Retire old queues while application callbacks still resolve their owner.
	// Publishing the candidate first would let a shutdown finalizer address the
	// new scene. Candidate failure above never reaches this destructive boundary.
	if disposer, ok := oldScene.(sceneDisposer); ok {
		disposer.Dispose()
	}
	g.currentScene = scene
	g.musicHistory = history
	g.sceneGeneration.Add(1)
	g.publishPlayerSaveCatalog()
	previous := g.debugDiscovery.Load()
	optionsRevision := uint64(1)
	if previous != nil {
		optionsRevision = previous.OptionsRevision + 1
	}
	g.publishDebugDiscovery(nil, optionsRevision)

	return nil
}

type sceneActivationPreflighter interface {
	PreflightActivation() error
}

type sceneActivationInputPreparer interface {
	PrepareInputForSceneActivation()
}

type sceneReplacementPreparer interface {
	PrepareForSceneReplacement()
}

type sceneActivator interface {
	Activate()
}

type sceneDisposer interface {
	Dispose()
}

func activateScene(scene Scene) {
	if activator, ok := scene.(sceneActivator); ok {
		activator.Activate()
	}
}

func activateSceneSafely(scene Scene) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("activate replacement scene: %v", recovered)
		}
	}()
	activateScene(scene)

	return nil
}

// Update calls the current scene's Update method
func (g *Game) Update() error {
	started := profiling.Begin()
	defer profiling.EndUpdate(started)
	// Scope global presentation ownership so dormant/test games cannot retain it.
	displayManager := display.GetManager()
	previous := displayManager.SetMapPopupViewportProvider(g.mapPopupViewport)
	defer displayManager.SetMapPopupViewportProvider(previous)

	// Use DisplayManager to handle resolution changes
	if g.currentScene != nil && g.currentScene.GetUltimaConfiguration() != nil {
		if displayManager.Update() {
			g.currentScene.InvalidateResolution()
		}
	}

	if g.currentScene != nil {
		if g.debugDiscoveryPolling.Load() {
			g.refreshDebugDiscoverySnapshot()
		}
		g.debugBridge.Process(gameDebugExecutor{game: g}, maxDebugRequestsPerUpdate)
		if g.windowCloseRequested() {
			g.handleWindowCloseRequest()
		}

		if err := g.currentScene.Update(g); err != nil {
			return err
		}
	}
	if g.exitRequested {
		return ebiten.Termination
	}

	return nil
}

type debugOptionSetProvider interface {
	DebugOptionSets() []debugging.OptionSet
}

func (g *Game) refreshDebugDiscoverySnapshot() {
	var optionSets []debugging.OptionSet
	if provider, ok := g.currentScene.(debugOptionSetProvider); ok {
		optionSets = provider.DebugOptionSets()
	}
	previous := g.debugDiscovery.Load()
	retired := g.debugDiscoveryRetired.Swap(false)
	optionsRevision := uint64(1)
	if previous != nil {
		optionsRevision = previous.OptionsRevision
		if !retired && previous.SceneGeneration == g.sceneGeneration.Load() &&
			equalDebugOptionSets(previous.OptionSets, optionSets) {
			return
		}
		optionsRevision++
	}
	g.publishDebugDiscovery(optionSets, optionsRevision)
}

func (g *Game) publishDebugDiscovery(optionSets []debugging.OptionSet, optionsRevision uint64) {
	// Static definitions are Game-owned and immutable. Clone mutable provider
	// output once here; external readers still receive a complete detached copy.
	snapshot := debugging.DiscoverySnapshot{
		SceneGeneration: g.sceneGeneration.Load(),
		OptionsRevision: optionsRevision,
		Commands:        g.debugDefinitions,
		OptionSets:      debugging.CloneOptionSets(optionSets),
	}
	g.debugDiscovery.Store(&snapshot)
}

func (g *Game) stopDebugDiscoveryPolling() {
	if g.debugDiscoveryPolling.Swap(false) {
		// Retire reviews across a dormant interval, even if state changes A->B->A.
		g.debugDiscoveryRetired.Store(true)
	}
}

func equalDebugOptionSets(left, right []debugging.OptionSet) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].Provider != right[index].Provider ||
			left[index].UnavailableReason != right[index].UnavailableReason ||
			!equalDebugChoices(left[index].Choices, right[index].Choices) {
			return false
		}
	}

	return true
}

func equalDebugChoices(left, right []debugging.Choice) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		valuesDiffer := left[index].Value != right[index].Value || left[index].Label != right[index].Label
		if valuesDiffer || !bytes.Equal(left[index].Metadata, right[index].Metadata) {
			return false
		}
	}

	return true
}

func commandUsesDynamicOptions(definition debugging.CommandDefinition) bool {
	for _, parameter := range definition.Parameters {
		if parameter.OptionsProvider != "" {
			return true
		}
	}

	return false
}

// Draw calls the current scene's Draw method
func (g *Game) Draw(screen *ebiten.Image) {
	displayManager := display.GetManager()
	previous := displayManager.SetMapPopupViewportProvider(g.mapPopupViewport)
	defer displayManager.SetMapPopupViewportProvider(previous)

	if g.currentScene != nil {
		g.currentScene.Draw(screen)
	}
}

type mapPopupViewportProvider interface {
	mapPopupViewport(screenBounds image.Rectangle) image.Rectangle
}

func (g *Game) mapPopupViewport(screenBounds image.Rectangle) image.Rectangle {
	// Resolve the current scene on every query, including after scene replacement.
	if provider, ok := g.currentScene.(mapPopupViewportProvider); ok {
		return provider.mapPopupViewport(screenBounds)
	}

	return image.Rectangle{}
}
