package main

import (
	"image"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/event_queue"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

//nolint:gochecknoglobals // Immutable sprite resources are shared by every moongate renderer.
var (
	// moongateSprite holds the actual Ultima V moongate sprite (tile 220) for rendering.
	moongateSprite   *ebiten.Image
	moongateGlowMask *ebiten.Image
)

// initMoongateSprite initializes the moongate sprite if not already created
func (g *GameScene) initMoongateSprite() {
	if moongateSprite == nil {
		// Moongates are clipped to one map tile, so they do not use the bordered actor sprite.
		moongateSprite = g.spriteSheet.GetRawSprite(indexes.Moongate)
		moongateGlowMask = ebiten.NewImage(sprites.TileSize, sprites.TileSize)
		moongateGlowMask.Fill(color.White)
	}
}

// drawMoongates renders active moongates as a post-process effect
// This is called after the map is drawn but before UI elements
func (g *GameScene) drawMoongates(screen *ebiten.Image) {
	// Moongates can exist on both large maps (overworld) and small maps (towns)

	// Initialize moongate sprite if needed
	g.initMoongateSprite()

	// Get elapsed time for animations; the presentation clock never runs backwards.
	elapsed := g.getElapsedMs()
	if elapsed < 0 {
		elapsed = 0
	}
	elapsedMs := uint64(elapsed)

	// Get active moongate renderers from the visual system
	activeRenderers := g.moongateVisualSystem.GetActiveRenderers()
	stage := g.gameState.MoongateStage()

	// Draw all active moongates with animation
	for _, renderer := range activeRenderers {
		// Set sprite in renderer if not already set
		if renderer.moongateSprite == nil {
			renderer.SetSprite(moongateSprite)
		}

		// Get drawing information from renderer
		opacity, rows, shouldDraw := renderer.GetDrawInfo(stage, elapsedMs)

		if shouldDraw {
			g.drawAnimatedMoongate(screen, renderer.GetPosition(), opacity, rows)
		}
	}
}

// drawAnimatedMoongate draws a moongate with animation support (growing/shrinking)
func (g *GameScene) drawAnimatedMoongate(screen *ebiten.Image, moongatePos references.Position, opacity float32,
	rows int) {
	if !g.ditherAllowsWorldEffect(moongatePos) {
		return
	}
	// Calculate relative position from player
	relative := g.surfaceWorldRelativePosition(moongatePos)
	if g.terrainDither.enabled {
		relative = g.ditherRelativePosition(moongatePos)
	}
	relX, relY := int(relative.X), int(relative.Y)

	// Check if moongate is visible on screen
	halfScreenWidth := xTilesVisibleOnGameScreen/2 + int(g.surfaceOverscanTiles())
	halfScreenHeight := yTilesVisibleOnGameScreen/2 + int(g.surfaceOverscanTiles())

	if relX < -halfScreenWidth || relX > halfScreenWidth ||
		relY < -halfScreenHeight || relY > halfScreenHeight {
		// Moongate is off-screen
		return
	}

	// Calculate tile coordinates within the visible map area
	// Center tiles around player position
	tileX := (xTilesVisibleOnGameScreen / 2) + relX
	tileY := (yTilesVisibleOnGameScreen / 2) + relY

	// Calculate exact pixel position on the map image (same as regular tiles)
	screenX := float64(tileX * sprites.TileSize)
	screenY := float64(tileY * sprites.TileSize)

	// Create draw options for the moongate
	opts := &ebiten.DrawImageOptions{}

	// For growth animation, we need to crop the sprite to show only the bottom rows
	// Calculate how many pixel rows to show (16 rows per tile)
	pixelRowsToShow := rows
	if pixelRowsToShow > sprites.TileSize {
		pixelRowsToShow = sprites.TileSize
	}

	moongateToDraw, offsetY := verticallyRevealMoongate(moongateSprite, pixelRowsToShow)
	if moongateToDraw == nil {
		return
	}

	// Position at the exact tile location with animation offset
	opts.GeoM.Translate(screenX, screenY+offsetY)

	// Apply game screen scaling and positioning (same as map)
	mapOp := sprites.GetDrawOptionsFromPercentsForWholeScreen(g.mapImage, g.mapScreenPlacement())
	if g.surfaceCamera.active || g.terrainDither.enabled {
		mapOp = g.worldDrawOptions()
	}
	opts.GeoM.Concat(mapOp.GeoM)

	// Apply opacity from animation system
	opts.ColorScale.ScaleAlpha(float32(opacity))

	// Draw the base moongate sprite first
	screen.DrawImage(moongateToDraw, opts)

	// Crop the glow to the same rows so it cannot extend into the tile below while the gate rises.
	glowToDraw, _ := verticallyRevealMoongate(moongateGlowMask, pixelRowsToShow)
	glowOpts := &ebiten.DrawImageOptions{}
	glowOpts.GeoM.Concat(opts.GeoM)
	glowOpts.ColorScale = opts.ColorScale

	elapsedMs := g.getElapsedMs()
	sprites.MoongateGlowOverlay.ApplyOverlayWithMask(screen, glowOpts, elapsedMs, opacity, glowToDraw)
}

func verticallyRevealMoongate(sprite *ebiten.Image, rows int) (*ebiten.Image, float64) {
	rows = min(max(rows, 0), sprites.TileSize)
	if rows == 0 {
		return nil, sprites.TileSize
	}
	if rows == sprites.TileSize {
		return sprite, 0
	}
	bounds := sprite.Bounds()
	cropRect := image.Rect(bounds.Min.X, bounds.Min.Y, bounds.Min.X+sprites.TileSize, bounds.Min.Y+rows)
	cropped, ok := sprite.SubImage(cropRect).(*ebiten.Image)
	if !ok {
		panic("verticallyRevealMoongate: Ebitengine image returned an incompatible subimage")
	}

	return cropped, float64(sprites.TileSize - rows)
}

// checkMoongateEntry checks if player has stepped on any materialized moongate.
func (g *GameScene) checkMoongateEntry() {
	// Moongates can exist on both large maps (overworld) and small maps (towns)

	// First check if player is on an active moongate (game state check)
	if !g.gameState.IsPlayerOnActiveMoongate() {
		return
	}

	// Check if teleportation is already in progress
	if g.gameState.IsTeleportationActive() {
		return
	}

	g.triggerMoongateTravel()
}

// triggerMoongateTravel starts the teleportation event queue
// Safe to call - not inside another event queue
func (g *GameScene) triggerMoongateTravel() {
	// Capture the destination before reserving the workflow; effects begin only after reservation.
	travelResult := g.gameState.ReviewMoongateTravel()
	if !travelResult.Success {
		if travelResult.FailedButActivated {
			g.gameState.EventQueueManager.Workflow("moongate_activation").
				Priority(event_queue.PriorityHigh).
				Sequential().BlockInput().
				First(func() { g.gameState.ApplyMoongateTravelEffects(false) }).
				Finally(func() { g.gameState.ProcessPendingProvisionHours() }).
				Start()
		}

		return // Travel failed (no valid destination, etc.)
	}
	g.queueReviewedMoongateTravel(travelResult)
}

// queueReviewedMoongateTravel starts the route captured before workflow-owned time advances.
func (g *GameScene) queueReviewedMoongateTravel(travelResult game_state.MoongateTravelResult) {
	q := g.gameState.EventQueueManager.Workflow(game_state.MoongateTeleportWorkflow).
		Priority(event_queue.PriorityHigh).
		Sequential().BlockInput()

	switch travelResult.Route.Kind {
	case game_state.MoongateTravelToMoonstone:
		q.First(g.gameState.ApplyMoongateDepartureEffects).
			ThenSequence(g.gameState.BuildMoongateTeleportationSequence(travelResult.Route.Destination))
	case game_state.MoongateTravelToSpirituality:
		q.First(func() {
			g.gameState.ApplyMoongateTravelEffects(true)
		}).Then(func() {
			g.gameState.SystemCallbacks.Visual.ResetTeleportationSourceRenderer()
		}).HandoffTo(shrineMeditationWorkflowName, event_queue.PriorityNormal, func(shrineQ *event_queue.SequentialEventBuilder) {
			g.buildShrineWorkflow(shrineQ, travelResult.Route.ShrineIndex)
		})
	default:
		panic("queueReviewedMoongateTravel: unknown route kind")
	}

	q.Finally(func() {
		g.gameState.SystemCallbacks.Visual.SetAvatarDissolveLevel(0)
		g.gameState.SystemCallbacks.Visual.SetMapFadeLevel(0)
		g.gameState.SystemCallbacks.Visual.CleanupAvatarDissolveEffect()
		g.gameState.ProcessPendingProvisionHours()
	}).Start()
}
