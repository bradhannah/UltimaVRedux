package main

import (
	"fmt"
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/display"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/profiling"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// gameScreenPercents defines the game map area placement (left 76% of screen, full height)
//
//nolint:gochecknoglobals // Read-only layout template shared by scene viewport and overlay calculations.
var gameScreenPercents = sprites.PercentBasedPlacement{
	StartPercentX: 0.0,
	EndPercentX:   0.76,
	StartPercentY: 0.0,
	EndPercentY:   1.0,
}

type cinematicMapLayout struct {
	placement sprites.PercentBasedPlacement
	viewport  image.Rectangle
}

func cinematicTranscriptPlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: 0.02,
		EndPercentX:   0.30,
		StartPercentY: 0.02,
		EndPercentY:   0.98,
	}
}

func cinematicMapPanePlacement() sprites.PercentBasedPlacement {
	return sprites.PercentBasedPlacement{
		StartPercentX: 0.32,
		EndPercentX:   0.99,
		StartPercentY: 0.02,
		EndPercentY:   0.98,
	}
}

// centeredGameDialogPercents defines a square dialog centered within the game area
// Used for overlay dialogs like GemViewDialog and AstronomyDialog that need to
// display visual content (maps, star charts) without covering UI panels
//
//nolint:gochecknoglobals // Read-only placement template shared by centered map dialogs.
var centeredGameDialogPercents = sprites.PercentBasedPlacement{
	StartPercentX: 0.14,
	EndPercentX:   0.62,
	StartPercentY: 0.07,
	EndPercentY:   0.93,
}

// Draw method for the GameScene
func (g *GameScene) Draw(screen *ebiten.Image) {
	g.updateSurfaceCamera()
	g.terrainDither.enabled = false
	// FPS limiting based on DisplayConfiguration - pure time-based
	if g.gameConfig.DisplayConfiguration.EnableFpsLimiting {
		currentTime := g.sceneNow().UnixMilli()
		frameInterval := g.gameConfig.DisplayConfiguration.GetFrameInterval()

		// Skip frame if we're within the target frame interval
		if frameInterval > 0 && currentTime-g.lastDrawTime < frameInterval {
			// Debug logging for low FPS testing (only for very slow frame rates)
			if frameInterval > 100 { // Only for very low FPS (like 2 FPS = 500ms interval)
				logging.Debugf(logging.CategorySystem, "FPS limiting: SKIPPING frame - interval=%dms, timeSince=%dms",
					frameInterval, currentTime-g.lastDrawTime)
			}

			return // Skip this frame to maintain target FPS
		}

		// Debug logging for frames that are drawn (only for very slow frame rates)
		if frameInterval > 100 {
			logging.Debugf(logging.CategorySystem, "FPS limiting: DRAWING frame - interval=%dms, timeSince=%dms",
				frameInterval, currentTime-g.lastDrawTime)
		}

		g.lastDrawTime = currentTime
	}

	started := profiling.Begin()
	defer profiling.EndDraw(started)
	g.beginCombatRenderFrame()
	defer g.endCombatRenderFrame()
	// Clear the screen every frame for proper rendering
	screen.Clear()

	width, height := display.GetManager().GetCurrentSize()
	currentResolution := config.ScreenResolution{X: width, Y: height}
	if g.lastCheckedResolution != currentResolution {
		g.initializeResizeableVisualElements()
	}
	g.syncRightRailLayout(screen.Bounds())
	if g.drawLordBritishFinaleFullScreen(screen) {
		return
	}
	usesCinematicSplit := g.usesCinematicSplitLayout()
	mapRenderTarget := g.worldRenderTarget(screen)
	surfaceFrame := g.usesSurfaceMapFrame()
	mapEffectsTarget := screen
	if surfaceFrame {
		mapEffectsTarget = mapRenderTarget
	}

	mapWidth := sprites.TileSize * xTilesVisibleOnGameScreen
	mapHeight := sprites.TileSize * yTilesVisibleOnGameScreen

	if g.mapImage == nil {
		g.mapImage = ebiten.NewImage(mapWidth, mapHeight)
	}

	isDungeon := g.gameState.MapState.PlayerLocation.Location.GetMapType() == references.DungeonMapType
	var dungeonStatus dungeonHUDStatus
	if isDungeon {
		dungeonStatus = g.drawDungeonViewport(mapRenderTarget)
	} else {
		// Refresh and compose the surface, combat, or scripted tile viewport.
		g.refreshAllMapLayerTiles()
		if g.surfaceCamera.active {
			op := &ebiten.DrawImageOptions{}
			margin := float64(g.surfaceOverscanTiles()) * sprites.TileSize
			op.GeoM.Translate(-margin, -margin)
			op.GeoM.Concat(g.worldDrawOptions().GeoM)
			mapRenderTarget.DrawImage(g.surfaceTerrainImage, op)
		} else {
			g.drawMap(g.mapImage)
			g.mapDrawOptions.GeoM = g.worldDrawOptions().GeoM
			mapRenderTarget.DrawImage(g.mapImage, g.mapDrawOptions)
		}
		g.drawTavernMealTableOverlay(mapRenderTarget)

		if !g.hideMoongates {
			g.drawMoongates(mapRenderTarget)
		}
		if !g.hideItemStacks && !g.gameState.IsInCombat() {
			g.drawItemStacksOverlay(mapRenderTarget)
		}
		if !g.hideQuestItems {
			g.drawQuestItemsOverlay(mapRenderTarget)
		}
		if g.shouldDrawMapUnitsOverlay() {
			g.drawMapUnitsOverlay(mapRenderTarget)
		}
		g.drawAvatarOverlay(mapRenderTarget)
		g.drawSurfaceArchForeground(mapRenderTarget)
		g.drawActiveTileDissolves(mapRenderTarget, RenderLayerDefault)
	}

	// Render teleportation map fade effect (after map, before UI)
	g.drawMapFadeEffect(mapEffectsTarget)

	// Render active tile dissolve effects - OVER FADE layer (after fade)
	// Used for: death scene apparitions that appear on the black background
	if !isDungeon {
		g.drawActiveTileDissolves(mapRenderTarget, RenderLayerOverFade)
	}

	// Render screen flash effect (for fire damage, etc.)
	viewportLit := !isDungeon || g.gameState.MapState.Lighting.HasAnyLight()
	g.drawViewportFlashEffects(mapEffectsTarget, viewportLit)

	// Render debug overlays on top of the map but below UI elements
	if !isDungeon && !surfaceFrame {
		if g.overlayRenderer == nil {
			panic("overlayRenderer is nil")
		}
		g.overlayRenderer.RenderOverlays(mapRenderTarget, g)
	}
	if surfaceFrame {
		// Finish all surface drawing before static chrome and dialogs. The frame's
		// opaque corner art completes the rectangular aperture's rounded shape.
		g.drawVisualEffects(mapRenderTarget)
		g.drawActiveProjectile(mapRenderTarget)
		g.drawTerrainDither(mapRenderTarget)
		g.overlayRenderer.RenderOverlays(mapRenderTarget, g)
		g.activeEffectsOverlay.DrawWithScaling(
			mapRenderTarget, g.mapImage, g.mapScreenPlacement(), xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
		)
	}
	if !isDungeon && !surfaceFrame {
		// Tactical/scripted world effects also precede static border chrome.
		g.drawVisualEffects(mapRenderTarget)
		g.drawActiveProjectile(mapRenderTarget)
	}
	if viewportLit && !surfaceFrame && !usesCinematicSplit {
		g.activeEffectsOverlay.DrawWithScaling(
			mapEffectsTarget, g.mapImage, g.mapScreenPlacement(), xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
		)
	}
	if !usesCinematicSplit {
		g.drawCombatHUD(screen)

		// Render UI elements that should NOT shake (borders, text outputs, etc.)
		g.drawBorders(screen)
		g.drawSkyStrip(screen)
		g.activeEffectsPanel.Draw(screen)

		// Static frame chrome follows the exploration context, never the camera.
		if label := g.mapBorderLabel(); label != "" {
			g.windDisplay.DrawLabel(screen, label)
		}

		if isDungeon {
			g.drawDungeonHUD(screen, dungeonStatus)
		}
		// Draw right-side message output panel
		// Position at percentage-based coordinates matching the midRightTextOutputBorder placement
		g.drawMainOutput(screen)

		g.drawCharacterSummary(screen)
		g.provisionSummary.Draw(g.gameState, screen)
	}

	g.drawRunicReference(screen)

	// draw the dialogs - but stacked on top of each other
	// All MessageDialog instances are now managed via dialog stack (no persistent messageDialog)
	g.drawDialogs(screen)

	if usesCinematicSplit {
		g.drawCinematicMapBorder(screen)
	}

	// Render the game scene debug info
	if !isDungeon && !usesCinematicSplit && g.showRendererDiagnostics() {
		ebitenutil.DebugPrint(screen, g.debugMessage)
	}
}

// drawViewportFlashEffects follows the composed viewport and precedes chrome.
// Ordinary flash/invert effects retain the dungeon light guard. Injury colours
// cover the already-black/subdued image without drawing any hidden terrain.
func (g *GameScene) drawViewportFlashEffects(screen *ebiten.Image, viewportLit bool) {
	if viewportLit {
		g.drawScreenFlashEffect(screen)
		g.drawScreenInvertEffect(screen)
	}
	g.drawDungeonInjury(screen)
}

func (g *GameScene) worldRenderTarget(screen *ebiten.Image) *ebiten.Image {
	var viewport image.Rectangle
	switch {
	case g.usesSurfaceMapFrame():
		viewport = g.borders.surfaceAperture
	case !g.usesCinematicSplitLayout() && !g.surfaceCamera.active:
		return screen
	default:
		viewport = g.mapViewportRectangle()
	}
	clippedTarget, ok := screen.SubImage(viewport).(*ebiten.Image)
	if !ok {
		panic("Draw: Ebitengine subimage has unexpected type")
	}

	return clippedTarget
}

func (g *GameScene) usesSurfaceMapFrame() bool {
	if g.gameState == nil || g.usesCinematicSplitLayout() || g.miniMapScene != nil || g.finalePresentation != nil ||
		g.gameState.IsInCombat() || g.gameState.IsInCamping() || g.gameState.IsInMiniMap() {
		return false
	}
	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()

	return mapType == references.SmallMapType || mapType == references.LargeMapType
}

func (g *GameScene) usesCinematicSplitLayout() bool {
	if g.finalePresentation != nil {
		return g.finalePresentation.mode == finaleRescueMap || g.finalePresentation.mode == finaleFailureScreen
	}

	return g.miniMapScene != nil && g.gameState != nil &&
		g.gameState.EventQueueManager.HasWorkflow("blackthorn_interrogation")
}

func (g *GameScene) mapScreenPlacement() sprites.PercentBasedPlacement {
	if !g.usesCinematicSplitLayout() {
		return gameScreenPercents
	}
	width, height := display.GetManager().GetCurrentSize()

	return cinematicMapLayoutForScreen(width, height).placement
}

func (g *GameScene) mapViewportRectangle() image.Rectangle {
	width, height := display.GetManager().GetCurrentSize()
	if g.usesCinematicSplitLayout() {
		return cinematicMapLayoutForScreen(width, height).viewport
	}
	if g.usesSurfaceMapFrame() {
		return image.Rect(0, 0, int(float64(width)*gameScreenPercents.EndPercentX), height)
	}

	return image.Rect(
		int(math.Round(float64(width)*gameScreenPercents.StartPercentX)),
		int(math.Round(float64(height)*gameScreenPercents.StartPercentY)),
		int(math.Round(float64(width)*gameScreenPercents.EndPercentX)),
		int(math.Round(float64(height)*gameScreenPercents.EndPercentY)),
	)
}

func cinematicMapLayoutForScreen(width, height int) cinematicMapLayout {
	panePlacement := cinematicMapPanePlacement()
	pane := image.Rect(
		int(math.Round(float64(width)*panePlacement.StartPercentX)),
		int(math.Round(float64(height)*panePlacement.StartPercentY)),
		int(math.Round(float64(width)*panePlacement.EndPercentX)),
		int(math.Round(float64(height)*panePlacement.EndPercentY)),
	)
	roomSize := min(pane.Dx(), pane.Dy())
	roomLeft := pane.Min.X
	roomTop := pane.Min.Y + (pane.Dy()-roomSize)/2
	viewport := image.Rect(roomLeft, roomTop, roomLeft+roomSize, roomTop+roomSize)
	const cinematicTiles = 11
	scale := float64(roomSize) / float64(cinematicTiles*sprites.TileSize)
	fullLeft := float64(roomLeft) - float64((xTilesVisibleOnGameScreen-cinematicTiles)/2*sprites.TileSize)*scale
	fullTop := float64(roomTop) - float64((yTilesVisibleOnGameScreen-cinematicTiles)/2*sprites.TileSize)*scale
	fullWidth := float64(xTilesVisibleOnGameScreen*sprites.TileSize) * scale
	fullHeight := float64(yTilesVisibleOnGameScreen*sprites.TileSize) * scale

	return cinematicMapLayout{
		placement: sprites.PercentBasedPlacement{
			StartPercentX: fullLeft / float64(width),
			EndPercentX:   (fullLeft + fullWidth) / float64(width),
			StartPercentY: fullTop / float64(height),
			EndPercentY:   (fullTop + fullHeight) / float64(height),
		},
		viewport: viewport,
	}
}

func (g *GameScene) drawCinematicMapBorder(screen *ebiten.Image) {
	const baseScreenHeight = 200
	viewport := g.mapViewportRectangle()
	strokeWidth := float32(max(1, int(math.Round(float64(screen.Bounds().Dy())/baseScreenHeight))))
	vector.StrokeRect(
		screen,
		float32(viewport.Min.X), float32(viewport.Min.Y),
		float32(viewport.Dx()), float32(viewport.Dy()),
		strokeWidth, ucolor.EGAWhite.ToRGBA(), false,
	)
}

func (g *GameScene) drawMainOutput(screen *ebiten.Image) {
	outputBounds := g.rightRail.outputContent
	g.outputClipImage.Clear()
	g.output.DrawScrollingBufferWithHeight(
		g.outputClipImage,
		image.Point{},
		outputBounds.Dx(),
		outputBounds.Dy(),
		true,
		text.AlignStart,
		text.AlignEnd,
	)
	outputOp := new(ebiten.DrawImageOptions)
	outputOp.GeoM.Translate(float64(outputBounds.Min.X), float64(outputBounds.Min.Y))
	screen.DrawImage(g.outputClipImage, outputOp)
}

func (g *GameScene) shouldDrawMapUnitsOverlay() bool {
	return !g.hideMapUnits && !g.gameState.IsInCombat() &&
		g.gameState.MapState.PlayerLocation.Location.GetMapType() != references.DungeonMapType
}

func (g *GameScene) drawDialogs(screen *ebiten.Image) {
	// Reflow before deciding z-order: a resize or additional resource can make
	// the strip overlap a modal that previously had enough free space above it.
	g.layoutProvisionChanges(screen.Bounds())
	aboveDialogs := g.provisionChangesAboveDialogs()
	if !aboveDialogs {
		g.provisionChanges.panel.Draw(screen)
	}
	g.dialogStack.Draw(screen)
	if aboveDialogs {
		g.provisionChanges.panel.Draw(screen)
	}
}

// drawBorders renders the frame borders around the game screen
func (g *GameScene) drawBorders(screen *ebiten.Image) {
	screen.DrawImage(g.borders.outsideBorder, g.borders.outsideBorderOp)
	screen.DrawImage(g.borders.rightSideBorder, g.borders.rightSideBorderOp)
	screen.DrawImage(g.borders.activeEffectsBorder, g.borders.activeEffectsBorderOp)
	screen.DrawImage(g.borders.midRightTextOutputBorder, g.borders.midRightTextOutputBorderOp)
	screen.DrawImage(g.borders.bottomRightProvisionsBorder, g.borders.bottomRightProvisionsBorderOp)
	screen.DrawImage(g.borders.topRightCharacterBorder, g.borders.topRightCharacterBorderOp)
}

// drawMap renders the base map tiles
func (g *GameScene) drawMap(screen *ebiten.Image) {
	// Draw map tiles without screen shake offset (handled at top level)
	opts := &ebiten.DrawImageOptions{}
	screen.DrawImage(g.unscaledMapImage, opts)
	g.debugMessage = fmt.Sprintf("%d, %d", g.gameState.MapState.PlayerLocation.Position.X, g.gameState.MapState.PlayerLocation.Position.Y)
}

// applyLightTinting applies light source tinting to draw options for a given position.
// Tiles within light source radii get colored tinting (warm orange for torches,
// cool blue for magic, etc.). During daytime, only TintAlways and TintHalfDuringDay
// sources apply tinting. Ambient darkness fades during dawn and dusk; tiles
// outside light sources get progressively darker without changing visibility.
// Returns true if any color scale was applied to the draw options.
func (g *GameScene) applyLightTinting(pos references.Position, opts *ebiten.DrawImageOptions, animationTime int64) bool {
	// Original combat bypasses exploration lighting entirely; dungeon combat
	// displays the full battlefield regardless of torches or magic light.
	if g.gameState.IsInCombat() {
		return false
	}

	// No tinting if system not initialized
	if g.gameState.LightSourceTintingState == nil {
		return false
	}

	ambientDarkness := g.gameState.AmbientTintDarkness()
	daylightTarget, hasDaylight := g.gameState.DaylightTintTarget(pos)
	// Scripted mini-maps/finale layouts retain their existing presentation even
	// if their source GameState still points at an above-ground exploration map.
	hasDaylight = hasDaylight && g.usesSurfaceMapFrame()

	// Check if tile is lit by a source and blend its day/night tint for the current time.
	var tint *map_state.LightTintColor
	if hasDaylight {
		tint = g.gameState.LightSourceTintingState.GetTintScaleFactorsWithDaylight(pos, animationTime, ambientDarkness, daylightTarget)
	} else {
		tint = g.gameState.LightSourceTintingState.GetTintScaleFactors(pos, animationTime, ambientDarkness)
	}
	if tint != nil {
		opts.ColorScale.Scale(tint.R, tint.G, tint.B, 1.0)

		return true
	}

	if ambientDarkness <= 0 && (!hasDaylight || daylightTarget == 1) {
		return false
	}

	// Fade the ambient tint independently of visibility so dusk never creates
	// additional black tiles.
	darkeningFactor := map_state.UnlitAmbientBrightness(ambientDarkness)
	if hasDaylight {
		darkeningFactor = daylightTarget + (map_state.UnlitDarkeningFactor-daylightTarget)*ambientDarkness
	}
	opts.ColorScale.Scale(darkeningFactor, darkeningFactor, darkeningFactor, 1.0)

	return true
}
