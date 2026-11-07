package main

import (
	"errors"
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/save_snapshot"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/ui/mainscreen"
	"github.com/bradhannah/UltimaVRedux/internal/ui/widgets"
	"github.com/bradhannah/UltimaVRedux/pkg/input"
)

var (
	errDisposedScene      = errors.New("disposed scene cannot be activated")
	errSceneConfiguration = errors.New("scene configuration and game state are required")
)

// GameScene initialization and constructor
// Handles creation and setup of all GameScene components

// NewGameSceneFromSaveSnapshot constructs a dormant gameplay scene from a native snapshot.
// The scene does not start audio or alter process-wide display settings until Game activates it.
func NewGameSceneFromSaveSnapshot(
	gameConfig *config.UltimaVConfiguration,
	snapshot save_snapshot.SaveSnapshot,
) (*GameScene, error) {
	if issues := save_snapshot.Validate(snapshot); len(issues) != 0 {
		return nil, &save_snapshot.ValidationError{Issues: issues}
	}

	scene, err := newGameScene(gameConfig, func(gameReferences *references.GameReferences) (*game_state.GameState, error) {
		return game_state.NewGameStateFromSaveSnapshot(
			snapshot,
			gameConfig,
			gameReferences,
			xTilesVisibleOnGameScreen,
			yTilesVisibleOnGameScreen,
		)
	})
	if err != nil {
		return nil, err
	}
	if err := scene.output.RestoreJournalHistory(snapshot.Presentation.History); err != nil {
		scene.Dispose()

		return nil, fmt.Errorf("restoring journal history: %w", err)
	}
	scene.journalVisits = snapshot.Presentation.Visits.Clone()
	scene.lastConversation = snapshot.LastConversation.Clone()
	scene.excerpts = snapshot.Excerpts.Clone()

	return scene, nil
}

func newGameScene(
	gameConfig *config.UltimaVConfiguration,
	newGameState func(*references.GameReferences) (*game_state.GameState, error),
) (*GameScene, error) {
	if gameConfig == nil || gameConfig.SavedConfigData == nil {
		return nil, errors.New("game configuration and original data path are required")
	}
	gameScene := GameScene{gameConfig: gameConfig}

	var err error
	gameScene.dungeonSprites, err = sprites.LoadDungeonSprites(gameConfig.SavedConfigData.DataFilePath)
	if err != nil {
		return nil, fmt.Errorf("loading dungeon presentation assets: %w", err)
	}
	gameScene.dungeonCanvas = ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)
	gameScene.dungeonClipLayer = ebiten.NewImage(dungeonCanvasWidth, dungeonCanvasHeight)

	gameScene.gameReferences, err = references.NewGameReferences(gameConfig)
	if err != nil {
		return nil, fmt.Errorf("loading game references: %w", err)
	}

	gameScene.gameState, err = newGameState(gameScene.gameReferences)
	if err != nil {
		return nil, fmt.Errorf("constructing game state: %w", err)
	}
	gameScene.combatHUD = newCombatHUD()
	gameScene.combatHUD.detail = gameConfig.DebugConfiguration.CombatDetailShown
	gameScene.spriteSheet = sprites.NewSpriteSheet()
	gameScene.activeEffectsOverlay = NewActiveEffectsOverlay(gameScene.spriteSheet)

	// Wire up system callbacks (dependency injection)
	err = gameScene.wireSystemCallbacks()
	if err != nil {
		return nil, fmt.Errorf("wiring system callbacks: %w", err)
	}

	gameScene.keyboard = input.NewKeyboard(keyPressDelay)
	gameScene.initializeTransientCommandInput()
	gameScene.initializeResizeableVisualElements()

	gameScene.autoOpenDoorState = &AutoOpenDoorState{}

	// Initialize the dialog stack for modal dialogs
	// (zero value is valid - entries slice is nil initially and append works on nil slices)
	gameScene.dialogStack = widgets.DialogStack{}

	// Initialize the overlay renderer for debug visualizations
	gameScene.overlayRenderer = NewOverlayRenderer()

	// Initialize moongate visual system
	gameScene.moongateVisualSystem = NewMoongateVisualSystem(gameScene.gameState)

	// Initialize the wind display overlay
	gameScene.windDisplay = NewWindDisplay()

	gameScene.activeEffectsPanel = NewActiveEffectsPanel(
		gameScene.gameState,
		gameScene.spriteSheet,
		gameScene.rightRail.activeBorder,
	)

	// Pre-allocate draw options to reduce GC pressure during frame rendering
	gameScene.mapDrawOptions = &ebiten.DrawImageOptions{}
	gameScene.dungeonDrawOptions = &ebiten.DrawImageOptions{}
	gameScene.avatarDrawOptions = &ebiten.DrawImageOptions{}
	gameScene.npcDrawOptions = &ebiten.DrawImageOptions{}
	gameScene.itemDrawOptions = &ebiten.DrawImageOptions{}
	gameScene.moongateDrawOptions = &ebiten.DrawImageOptions{}

	// Audio resources may be allocated while dormant, but playback begins only on activation.
	if !gameConfig.SoundDisabled {
		gameScene.audioManager, err = audio.NewGameAudioManagerWithConfig(gameConfig.AudioConfiguration)
		if err != nil {
			logging.Warnf(logging.CategoryAudio, "Failed to initialize audio system: %v", err)
			gameScene.audioManager = nil
		}
	}

	return &gameScene, nil
}

// Activate makes a fully constructed candidate scene live.
func (g *GameScene) Activate() {
	if g.disposed {
		panic("GameScene.Activate: disposed scene cannot be activated")
	}
	if g.active {
		return
	}

	g.gameState.MapState.Lighting.SetAmbientLightRadius(g.gameConfig.GetAmbientLightRadius())
	const desiredTPS = 60
	ebiten.SetTPS(desiredTPS)
	ebiten.SetScreenClearedEveryFrame(false)
	now := g.sceneNow()
	g.startTime = now
	g.elapsedBaseMs = g.gameState.ElapsedMs
	g.lastEffectsUpdate = now
	g.active = true
	g.clearActorAnimationClocks()
	g.updateActorAnimationClocks()
	g.resetProvisionChanges()

	if g.audioManager != nil {
		currentLocation := g.gameState.MapState.PlayerLocation.Location
		if err := g.audioManager.PlayMusicForLocation(currentLocation); err != nil {
			logging.Warnf(logging.CategoryAudio, "Failed to start background music for location %v: %v", currentLocation, err)
		}
	}
}

// PrepareInputForSceneActivation prevents held keys from the prior scene from
// being interpreted by this scene after replacement.
func (g *GameScene) PrepareInputForSceneActivation() {
	g.keyboard.PrepareForTransition(0)
}

// PreflightActivation rejects invalid candidates before the active scene is prepared.
func (g *GameScene) PreflightActivation() error {
	if g.disposed {
		return errDisposedScene
	}
	if g.gameConfig == nil || g.gameState == nil {
		return errSceneConfiguration
	}

	return nil
}

// initializeResizeableVisualElements initializes all visual elements that scale with screen resolution
func (g *GameScene) initializeResizeableVisualElements() {
	width, height := display.GetManager().GetCurrentSize()
	g.lastCheckedResolution = config.ScreenResolution{X: width, Y: height}
	logicalBounds := image.Rect(0, 0, width, height)
	g.rightRail = calculateRightRailLayout(logicalBounds)

	g.initializeBorders()

	font, scaledOutput := newMainOutput(logicalBounds.Dx())
	if g.output == nil {
		g.output = scaledOutput
	} else {
		g.output.SetFont(font, scaledOutput.GetLineSpacing())
	}
	g.ultimaFont = font
	g.initializeDungeonHUDOutputs()
	g.dungeonGraffitiOutput = newDungeonGraffitiOutput(g.ultimaFont)

	_, g.combatDamageOutput = widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  widgets.FontSizeBody,
		LineSpacing:           0,
		LineSpacingMultiplier: 1,
		MaxLines:              1,
		MaxChars:              combatDamageMaxChars,
	})
	_, combatTurnOutput := widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacing:           0,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	_, combatEventOutput := widgets.GetScaledFontWithOutput(widgets.FontConfig{
		Size:                  combatHUDFontSize,
		LineSpacing:           0,
		LineSpacingMultiplier: 1,
		MaxLines:              combatHUDOutputLines,
		MaxChars:              combatHUDOutputChars,
	})
	if g.combatHUD == nil {
		g.combatHUD = newCombatHUD()
	}
	g.combatHUD.setOutputs(combatTurnOutput, combatEventOutput)

	// MessageDialog initialization REMOVED - now created on-demand via CreateAndPushMessageDialog
	// Each usage creates a NEW instance and pushes it onto the dialog stack

	if g.characterSummary != nil {
		g.characterSummary.Dispose()
	}
	g.characterSummary = mainscreen.NewCharacterSummaryForWidth(g.spriteSheet, logicalBounds.Dx())
	g.characterSummary.SetBounds(g.rightRail.characterContent)
	g.provisionSummary = mainscreen.NewProvisionSummaryForWidth(logicalBounds.Dx())
	g.provisionSummary.SetBounds(g.rightRail.provisionContent)
	if g.activeEffectsPanel != nil {
		g.activeEffectsPanel.SetBounds(g.rightRail.activeBorder)
	}
	g.syncRightRailLayout(logicalBounds)
}

// InvalidateResolution triggers re-initialization of resolution-dependent visual elements
func (g *GameScene) InvalidateResolution() {
	g.initializeResizeableVisualElements()
	g.transientActionFeedbackLayoutSet = false
}

// GetUltimaConfiguration returns the game configuration
func (g *GameScene) GetUltimaConfiguration() *config.UltimaVConfiguration {
	return g.gameConfig
}

// getElapsedMs returns the preserved gameplay elapsed time plus this activation epoch.
func (g *GameScene) getElapsedMs() int64 {
	if g.atlasSession != nil {
		return g.atlasSession.frozenMs
	}
	if g.journalSession != nil {
		return g.journalSession.frozenMs
	}
	if g.startTime.IsZero() {
		if g.gameState == nil {
			return g.elapsedBaseMs
		}

		return g.gameState.ElapsedMs
	}

	epochMs := g.sceneNow().Sub(g.startTime).Milliseconds()
	if g.elapsedBaseMs < 0 || epochMs < 0 || g.elapsedBaseMs > math.MaxInt64-epochMs {
		panic("scene elapsed clock is outside the safe addition domain")
	}

	return g.elapsedBaseMs + epochMs
}
