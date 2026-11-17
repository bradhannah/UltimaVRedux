package main

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/vector"

	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

// drawVisualEffects renders all active visual effects on screen
func (g *GameScene) drawVisualEffects(screen *ebiten.Image) {
	g.drawCombatImpactEffects(screen)

	for _, effect := range g.visualEffects {
		g.renderEffect(screen, effect)
	}
}

// renderEffect renders a specific visual effect based on its type
func (g *GameScene) renderEffect(screen *ebiten.Image, effect VisualEffect) {
	switch effect.Type {
	case "kapow":
		g.renderKapowEffect(screen, effect)
	default:
		// Note: Old missile_* effects removed - replaced by new projectile system
		// in gamescene_projectile.go that uses Animate() for proper timing
	}
}

// renderKapowEffect renders an explosion sprite with scale animation
func (g *GameScene) renderKapowEffect(screen *ebiten.Image, effect VisualEffect) {
	// Must have world position for sprite-based rendering
	if effect.WorldPos == nil {
		return
	}
	if !g.ditherAllowsWorldEffect(*effect.WorldPos) {
		return
	}

	sprite := g.spriteSheet.GetSprite(effect.SpriteIndex)
	if sprite == nil {
		return
	}

	// Calculate scale (triangle wave: min → max → min)
	progress := effect.Elapsed / effect.Duration
	var scale float64
	if progress < 0.5 {
		// First half: ScaleMin → ScaleMax
		scale = effect.ScaleMin + (effect.ScaleMax-effect.ScaleMin)*(progress*2)
	} else {
		// Second half: ScaleMax → ScaleMin
		scale = effect.ScaleMax - (effect.ScaleMax-effect.ScaleMin)*((progress-0.5)*2)
	}

	// Convert world position to unscaled source coordinates.
	playerPos := g.gameState.MapState.PlayerLocation.Position
	screenX, screenY := visualEffectPositionToSourcePixels(*effect.WorldPos, playerPos)
	if g.gameState.IsInCombat() {
		screenX, screenY = g.combatLayout().positionToSourcePixels(*effect.WorldPos)
	}
	if g.terrainDither.enabled {
		relative := g.ditherRelativePosition(*effect.WorldPos)
		screenX, screenY = sprites.ConvertPosition16to18(int(relative.X+xCenter), int(relative.Y+yCenter))
	}

	// Center of the tile (use ExpandedTileSize since ConvertPosition16to18 uses 18px grid)
	halfTile := float64(sprites.ExpandedTileSize) / 2.0
	centerX := screenX + halfTile
	centerY := screenY + halfTile

	// Get sprite dimensions
	spriteW := float64(sprite.Bounds().Dx())
	spriteH := float64(sprite.Bounds().Dy())

	// Calculate position to keep sprite centered after scaling
	drawX := centerX - (spriteW*scale)/2.0
	drawY := centerY - (spriteH*scale)/2.0

	// Setup draw options
	op := &ebiten.DrawImageOptions{}
	op.GeoM.Scale(scale, scale)
	op.GeoM.Translate(drawX, drawY)

	// Apply game screen scaling transform
	mapOp := sprites.GetDrawOptionsFromPercentsForWholeScreen(g.mapImage, g.mapScreenPlacement())

	// Apply screen shake if active
	if g.terrainDither.enabled {
		mapOp = g.worldDrawOptions()
	} else if g.screenShakeOffsetX != 0.0 || g.screenShakeOffsetY != 0.0 {
		mapOp.GeoM.Translate(g.screenShakeOffsetX, g.screenShakeOffsetY)
	}

	// Concatenate transforms
	op.GeoM.Concat(mapOp.GeoM)

	screen.DrawImage(sprite, op)
}

func visualEffectPositionToSourcePixels(worldPos, playerPos references.Position) (float64, float64) {
	tileX := int(xCenter) + int(worldPos.X-playerPos.X)
	tileY := int(yCenter) + int(worldPos.Y-playerPos.Y)

	return sprites.ConvertPosition16to18(tileX, tileY)
}

// Note: Old renderMissileEffect() removed - replaced by drawActiveProjectile() in gamescene_projectile.go
// The new system uses actual game sprites and Animate() callbacks for smooth animation.

// drawMapFadeEffect renders the teleportation map fade overlay
func (g *GameScene) drawMapFadeEffect(screen *ebiten.Image) {
	if g.mapFadeLevel <= 0.0 {
		return // No fade effect needed
	}

	// Calculate the fade alpha (0-255)
	alpha := uint8(255.0 * g.mapFadeLevel)

	gameArea := g.mapViewportRectangle()

	// Draw black overlay only over the game area (map region)
	vector.FillRect(screen,
		float32(gameArea.Min.X),
		float32(gameArea.Min.Y),
		float32(gameArea.Dx()),
		float32(gameArea.Dy()),
		ucolor.From(ucolor.MapFadeBlack).Alpha(alpha).RGBA(), false) // Black fade with calculated transparency
}

// drawScreenFlashEffect renders a colored screen flash overlay (for fire damage, etc.)
// Supports both legacy duration-based pattern and new event queue pattern
// Only affects the map area, leaving status bars unaffected.
func (g *GameScene) drawScreenFlashEffect(screen *ebiten.Image) {
	// Event queue pattern: intensity set directly via SetScreenFlashIntensity()
	// Animation timing managed by event queue Animate() callback
	currentIntensity := g.screenFlashIntensity

	// Early exit if no flash to render
	if currentIntensity <= 0.0 {
		return
	}

	// Calculate the flash alpha (0-255)
	alpha := uint8(255.0 * currentIntensity)

	if alpha == 0 {
		return // Nothing to draw
	}

	gameArea := g.mapViewportRectangle()

	// Create flash color from RGB components
	flashColor := color.RGBA{
		R: uint8(g.screenFlashR * 255),
		G: uint8(g.screenFlashG * 255),
		B: uint8(g.screenFlashB * 255),
		A: alpha,
	}

	// Draw colored overlay only over the game area (map region)
	vector.FillRect(screen,
		float32(gameArea.Min.X),
		float32(gameArea.Min.Y),
		float32(gameArea.Dx()),
		float32(gameArea.Dy()),
		flashColor, false)
}

func (g *GameScene) drawScreenInvertEffect(screen *ebiten.Image) {
	if g.screenInvertIntensity <= 0.0 {
		return // No invert effect needed
	}

	// Duration-based auto-expiration (only if duration > 0)
	// Duration=0 means intensity is being controlled externally (e.g., PartyStateBuilder animation)
	if g.screenInvertDuration > 0 {
		elapsedMs := g.getElapsedMs() - g.screenInvertStartTime
		if elapsedMs >= g.screenInvertDuration {
			g.screenInvertIntensity = 0.0

			return
		}
	}

	// Binary X-ray effect: stay at full intensity for entire duration
	// Matches original Ultima V behavior - instant on, stay inverted, instant off
	currentIntensity := g.screenInvertIntensity

	if currentIntensity <= 0.0 {
		return // Nothing to apply
	}

	// Apply color inversion effect using Kage shader
	// This creates an "X-ray" style effect for level-up visual feedback
	// Shader inverts RGB: output = vec3(1.0) - input.rgb
	bounds := screen.Bounds()
	shader := getInvertShader()
	screenCopy := ebiten.NewImageFromImage(screen)
	defer screenCopy.Deallocate()

	screen.Clear()
	op := &ebiten.DrawRectShaderOptions{
		Images: [4]*ebiten.Image{screenCopy, nil, nil, nil},
		Uniforms: map[string]interface{}{
			"Intensity": float32(currentIntensity),
		},
	}
	op.GeoM.Translate(float64(bounds.Min.X), float64(bounds.Min.Y))
	screen.DrawRectShader(bounds.Dx(), bounds.Dy(), shader, op)
}
