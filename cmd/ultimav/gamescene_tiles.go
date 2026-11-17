// GameScene Tiles System - Handles all visual tile rendering for the Ultima V game world
//
// This system manages a multi-layer rendering pipeline that displays:
// - Base terrain tiles (MapLayer)
// - Special tile states like open doors (MapOverrideLayer)
// - Items on ground from searches/drops (EquipmentAndProvisionsLayer) - now rendered as overlay
// - NPCs and creatures (MapUnitLayer)
//
// Player avatar is rendered separately via overlay system (see gamescene_draw.go)
// to ensure proper layering on top of moongates and other post-process effects.
//
// The system uses centralized GameState-managed layer refresh system.
package main

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/bradhannah/UltimaVRedux/internal/map_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Rendering constants for 11x11 tile viewport centered on player
const (
	xCenter = references.Coordinate(xTilesVisibleOnGameScreen / 2) // Center X offset (5 for 11-wide view)
	yCenter = references.Coordinate(yTilesVisibleOnGameScreen / 2) // Center Y offset (5 for 11-high view)

	// Buffer border pixels for 18x18 sprite rendering with proper edge handling
	borderPixels = 2
)

// Fallback edge darkness for combat and scripted layouts. Normal small-map
// exploration uses a terrain-only dither mask instead of these tile-row bands.
const (
	// EdgeGradientMaxDistance is tiles past edge to reach full black
	EdgeGradientMaxDistance = 6
	// EdgeMinBrightness is the brightness at the farthest visible edge (0.0 = full black)
	EdgeMinBrightness = float32(0.0)
	// EdgeStartBrightness is the brightness at the first out-of-bounds tile
	EdgeStartBrightness = float32(0.8)
)

// refreshStaticMapTiles renders terrain with layer priority: Equipment > Override > Base
// Excludes MapUnitLayer (handled by separate NPC rendering)
// OPTIMIZED: Uses static tile cache to avoid recalculating unchanged tiles
func (g *GameScene) refreshStaticMapTiles(pos *references.Position, mapLayer *map_state.LayeredMap, drawOptions *ebiten.DrawImageOptions) {
	var spriteIndex indexes.SpriteIndex

	// Performance optimization: skip invisible positions entirely
	// Uses both lighting system and line-of-sight calculations for visibility
	// Basement flag (floor < 0) affects visibility range and lighting calculations
	visible, bonus := g.isPositionVisible(mapLayer, pos), false
	if g.terrainDither.enabled {
		if cell, found := g.terrainDither.cellAt(*pos); found {
			visible, bonus = cell.visible, cell.bonus
		}
	}
	if !visible && !bonus {
		return
	}
	if !bonus && !g.gameState.IsOutOfBounds(*pos) {
		mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
		if g.suppressTerrainUnderlayAtPosition(mapLayer, mapType, *pos) {
			return
		}
	}

	// Track if any color scaling was applied (for proper reset at end)
	colorScaleApplied := false

	// Handle out-of-bounds positions first to avoid layer access issues
	// Out-of-bounds positions don't have valid layer data and would return index 0 (explosions)
	if g.gameState.IsOutOfBounds(*pos) {
		// Use location-specific outer tile (different for each map/town)
		// This creates appropriate "void" or "boundary" visual around maps
		// GetOuterTileForCurrentLocation handles all map types including CombatMapType (camping)
		spriteIndex = g.gameState.GetOuterTileForCurrentLocation()

		// Surface exploration masks the completed terrain buffer instead, keeping
		// retained pixels at their normally lit colour rather than darkening twice.
		distPastEdge := g.getDistancePastMapEdge(*pos)
		if distPastEdge > 0 && !g.usesOuterTerrainDither() {
			darkenFactor := calculateEdgeDarkness(distPastEdge)
			drawOptions.ColorScale.Scale(darkenFactor, darkenFactor, darkenFactor, 1.0)
			colorScaleApplied = true
		}
	} else {
		// Use unified tile override system for all tile rendering
		// This handles equipment, special tiles (portcullis, mirrors, drawbridges),
		// time-based overrides, and base terrain in proper priority order
		if bonus {
			spriteIndex = g.gameState.GetBonusTerrainTile(pos)
		} else {
			spriteIndex = g.gameState.GetEffectiveTile(pos)
			spriteIndex = g.combatLootTile(*pos, spriteIndex)
		}
	}

	// Apply the original terrain display illusion after object/visibility selection,
	// without feeding a presentation substitution into gameplay terrain queries.
	spriteIndex = references.MirageTerrainSprite(spriteIndex, *pos,
		g.gameState.MapState.PlayerLocation.Position, g.gameState.MapState.PlayerLocation.Location.GetMapType())
	if spriteIndex == indexes.NoSprites {
		if colorScaleApplied {
			drawOptions.ColorScale.Reset()
		}

		return // Absent terrain, notably combat margins, contributes no pixels.
	}

	// Prepare deterministic animation timing
	// Uses real-time clock for consistent behavior across systems
	animationTime := g.getElapsedMs()

	// Apply light source tinting at night (torches, braziers, lighthouse beams, etc.)
	// Uses unified helper that handles both lit tiles and darkening for contrast
	// This compounds multiplicatively with edge darkness if both are applied
	if g.applyLightTinting(*pos, drawOptions, animationTime) {
		colorScaleApplied = true
	}

	// Create position-based hash for animation variety
	// Different positions get different animation phases to avoid synchronized effects
	// Formula: X*256 + Y creates unique hash per position without conflicts
	posHash := pos.AnimationHash()

	// Render the final animated sprite to the unscaled map image buffer
	// GetAnimatedSprite handles frame calculation, timing, and sprite selection
	oof := g.spriteSheet.GetAnimatedSprite(spriteIndex, posHash, animationTime)
	location := g.gameState.MapState.PlayerLocation
	if background, overridden := references.DecorativeTerrainBase(location.Location, location.Floor, *pos, spriteIndex); overridden {
		oof = g.spriteSheet.GetAnimatedSpriteWithBackground(spriteIndex, background, posHash, animationTime)
	}
	if bonus && oof.Bounds().Dx() == sprites.ExpandedTileSize && oof.Bounds().Dy() == sprites.ExpandedTileSize {
		// Keep the bonus source in its logical cell; bordered tile effects must not
		// spill extra hidden artwork into another cell's permitted coverage.
		bounds := oof.Bounds().Inset(1)
		cropped, ok := oof.SubImage(bounds).(*ebiten.Image)
		if !ok {
			panic("bonus terrain: unexpected subimage type")
		}
		oof = cropped
		drawOptions.GeoM.Translate(1, 1)
	}
	target := g.unscaledMapImage
	if g.surfaceCamera.active {
		target = g.surfaceTerrainImage
	}
	if spriteIndex == indexes.Splat && g.gameState.IsInCombat() && !bonus &&
		g.gameState.CombatTerrainAllowsObjectDisplay(*pos) {
		// The selected corpse is transparent artwork, not replacement terrain.
		// Selection above already excludes losing corpses and chest previews.
		terrain := g.gameState.GetBonusTerrainTile(pos)
		underlay := g.spriteSheet.GetAnimatedSprite(terrain, posHash, animationTime)
		g.drawTilePaletteSprite(target, underlay, drawOptions, terrain)
	}
	g.drawTilePaletteSprite(target, oof, drawOptions, spriteIndex)

	// Reset color scale if any tint was applied (for next tile)
	if colorScaleApplied {
		drawOptions.ColorScale.Reset()
	}
}

func (g *GameScene) suppressTerrainUnderlayAtPosition(
	layer *map_state.LayeredMap,
	mapType references.GeneralMapType,
	position references.Position,
) bool {
	_, presentation, ok := g.mapUnitPresentationAtPosition(layer, mapType, position)

	return ok && presentation.SuppressTerrainUnderlay
}

// refreshAllMapLayerTiles renders all map layers to the screen in the complete tile rendering pipeline
//
// This is the main rendering method that orchestrates the entire visual display process.
// It manages the complete multi-layer tile system and ensures proper rendering order,
// positioning, and visual effects for the 11x11 tile viewport centered on the player.
//
// Rendering Pipeline:
// 1. Visibility: Recalculates lighting and line-of-sight for current position
// 2. Buffer Management: Creates/clears the unscaled map image buffer
// 3. Terrain Rendering: All tiles with unified GetEffectiveTile() system via refreshStaticMapTiles
// 4. NPCs/Units: Rendered separately as overlays (see gamescene_draw.go)
// 5. Avatar: Player sprite rendered via overlay system (see gamescene_draw.go)
//
// Key Architectural Features:
// - 11x11 tile viewport (xTilesVisibleOnGameScreen × yTilesVisibleOnGameScreen)
// - Player-centered rendering with xCenter, yCenter offsets (5,5 for 11×11 grid)
// - Large map wrapping support (overworld map coordinates wrap at boundaries)
// - Single unscaled image buffer for performance (scaled during final draw)
// - Proper z-order: terrain → overrides → items → NPCs → avatar
// - Deterministic animation timing via central game clock
//
// Tile System Integration:
//   - Uses unified GetEffectiveTile() system for all tile display logic
//   - Handles time-based tiles (portcullis, drawbridge), equipment, mirrors, and state overrides
//   - Single source of truth for what sprite appears at any position
//
// Search System Visual Feedback:
// The complete visual feedback chain for found items is:
// SearchSystem → ItemStacksMap → GetEffectiveTile() → Visual Display
//
// Performance Considerations:
// - 121 tile positions rendered per frame (11×11 grid)
// - Single image buffer reduces draw calls compared to individual tile draws
// - Early visibility checks in individual refresh methods improve performance
// - Position-based animation prevents unnecessary recalculations
//
// Technical Implementation:
// - World coordinates calculated relative to player position with centering offsets
// - Large map coordinate wrapping prevents out-of-bounds access
// - Avatar position tracked during tile iteration for final rendering
// - Draw options reset after each tile to prevent transform accumulation
// - Avatar uses fixed animation hash (0) for smooth continuous movement animation
//
// Args: None - operates on current GameState and renders to internal unscaledMapImage buffer
//
// Side Effects:
// - Modifies g.unscaledMapImage with complete rendered frame
// - Recalculates visibility based on current lighting and position
func (g *GameScene) refreshAllMapLayerTiles() {
	// PHASE 1: Visibility and Preparation
	// GetEffectiveTile() handles all tile display logic dynamically

	// Get the current layered map for the player's location
	// Contains all tile data and layer information needed for rendering
	// This now includes camping maps via CampingLocation → CombatMapType → LayeredMap
	layer := g.gameState.GetLayeredMapByCurrentLocation()

	// Recalculate tile visibility based on current lighting conditions
	// Factors in: time of day, torch/light sources, basement lighting, line-of-sight, active moongates, and quest items
	// This updates internal visibility flags used by all refresh methods
	// Use visual system for moongate positions to include shrinking moongates
	moongatePositions := g.moongateVisualSystem.GetVisibleMoongatePositions()

	// Get quest item light information for lighting system integration
	questItemLights := g.gameState.GetVisibleQuestItemLights()

	// Get magical NPC light information (wisps, seahorses, daemons, ghosts)
	npcLights := g.gameState.GetVisibleNPCLights()

	// Get player light type for visibility calculation (torch, magic light, or ambient)
	playerLightType := g.gameState.GetPlayerLightType()

	// Use the enhanced method that includes moongates, quest items, and NPCs as light sources
	layer.RecalculateVisibleTilesWithCustomLights(g.gameState.MapState.PlayerLocation.Position,
		&g.gameState.MapState.Lighting, moongatePositions, questItemLights, npcLights, playerLightType)

	// Update light source tinting state based on computed light sources
	// This enables colored light effects (warm orange from torches, etc.) at night
	g.gameState.UpdateLightSourceTinting(layer)
	g.prepareTerrainDither(layer)

	// PHASE 2: Rendering Buffer Setup
	// Create or validate the unscaled map image buffer
	// Size: 11×11 tiles at 16-pixel intervals plus border pixels for 18x18 sprite rendering
	// This buffer is later scaled to screen resolution during final composition
	if g.unscaledMapImage == nil {
		// We need 11 tiles × 16 pixels + border pixels for proper 18x18 sprite rendering
		// This accounts for the 18x18 sprites with 1-pixel overlap
		bufferWidth := sprites.TileSize*xTilesVisibleOnGameScreen + borderPixels
		bufferHeight := sprites.TileSize*yTilesVisibleOnGameScreen + borderPixels
		g.unscaledMapImage = ebiten.NewImage(bufferWidth, bufferHeight)
	}

	// Clear the buffer with black background
	// Essential for proper alpha blending and handling transparent/missing tiles
	g.unscaledMapImage.Fill(image.Black)
	margin := g.surfaceOverscanTiles()
	if margin != 0 {
		if g.surfaceTerrainImage == nil {
			g.surfaceTerrainImage = ebiten.NewImage(
				sprites.TileSize*(xTilesVisibleOnGameScreen+int(margin)*2)+borderPixels,
				sprites.TileSize*(yTilesVisibleOnGameScreen+int(margin)*2)+borderPixels,
			)
		}
		g.surfaceTerrainImage.Fill(image.Black)
	}

	// Determine map type for coordinate handling (small maps vs large overworld map)
	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()
	renderCenterX := xCenter
	if mapType == references.CombatMapType && g.gameState.IsInCombat() {
		renderCenterX = combatMapCenterCoordinate
	}

	// PHASE 3: Rendering Variables Setup
	// Reusable draw options to avoid allocation overhead in tight loops
	var drawImageOptions ebiten.DrawImageOptions

	// Avatar rendering is now handled by overlay system in gamescene_draw.go
	// This ensures avatar appears on top of moongates and other post-process effects

	// Position iterator - reused for all tile calculations to avoid allocations
	pos := &references.Position{}

	// Pre-allocate draw options to avoid repeated allocation overhead
	drawImageOptions.GeoM.Reset() // Ensure clean state

	// PHASE 4: Main Tile Rendering Loop
	// Iterate through the entire 11×11 visible tile grid
	// X: 0 to 10 (11 tiles wide), Y: 0 to 10 (11 tiles high)
	var x, y float64
	for tileX := -margin; tileX < references.Coordinate(xTilesVisibleOnGameScreen)+margin; tileX++ {
		for tileY := -margin; tileY < references.Coordinate(yTilesVisibleOnGameScreen)+margin; tileY++ {
			// Calculate world coordinates for this screen position
			// Formula: screen_pos + player_pos - center_offset
			// xCenter, yCenter are 5,5 for 11×11 grid (player at center)
			pos.X = tileX + g.gameState.MapState.PlayerLocation.Position.X - renderCenterX
			pos.Y = tileY + g.gameState.MapState.PlayerLocation.Position.Y - yCenter

			// Handle coordinate wrapping for large maps (overworld)
			// Large maps wrap at boundaries to create seamless world
			// Small maps (towns, dungeons) do not wrap
			if mapType == references.LargeMapType {
				pos = pos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			}

			// Set up screen positioning for this tile
			// Translates world position to screen pixel coordinates
			// Offset by -1 pixel to account for 18x18 sprites with 1-pixel border
			x, y = sprites.ConvertPosition16to18(int(tileX+margin), int(tileY+margin))
			if mapType == references.CombatMapType && g.gameState.IsInCombat() {
				x += float64(g.combatLayout().battlefieldLeft())
			}
			drawImageOptions.GeoM.Translate(x, y)

			// Render terrain using unified GetEffectiveTile() system
			g.refreshStaticMapTiles(pos, layer, &drawImageOptions)

			// NPCs/Units and Avatar rendered separately as overlays in gamescene_draw.go

			// Reset transform matrix to prevent accumulation between tiles
			// Critical for correct positioning - each tile starts with clean transform
			drawImageOptions.GeoM.Reset()
		}
	}
	g.drawOuterTerrainDither()
}

// isPositionVisible checks if a position should be visible, with special handling for camping mode
// In camping mode, the entire map should be visible regardless of time of day
func (g *GameScene) isPositionVisible(layer *map_state.LayeredMap, pos *references.Position) bool {
	// X-Ray vision reveals everything (Black potion effect)
	if g.xRayVisionActive {
		return true
	}

	// ref:COMBAT-0001: original tactical display bypasses LOS and illumination,
	// copying the complete combat map directly into the display buffer.
	if g.gameState.IsInCombat() {
		return true
	}

	// In camping mode, everything should be visible (campfire provides light)
	if g.gameState.IsInCamping() {
		return true
	}

	// In mini-map mode (shrine, codex, etc.), everything should be visible (always lit)
	if g.gameState.IsInMiniMap() {
		return true
	}

	// Normal visibility check using lighting system
	if layer.IsPositionVisible(pos, g.gameState.DateTime, g.gameState.MapState.PlayerLocation.Floor < 0) {
		return true
	}
	if g.isDaylightSurfaceFringeVisible(layer, *pos) {
		return true
	}

	// Check lighthouse visibility (rotating beacon light cone)
	// Lighthouse can make tiles visible that wouldn't otherwise be visible at night
	if g.gameState.LighthouseState != nil && g.gameState.LighthouseState.IsTileLit(*pos) {
		// Still require line-of-sight for lighthouse to illuminate
		visFlags := layer.GetVisibilityFlags()

		return visFlags[pos.X][pos.Y]
	}

	return false
}

// getDistancePastMapEdge returns how many tiles past the map boundary a position is.
// Returns 0 for in-bounds positions. Only applies to small and combat maps.
// Large maps wrap and don't have visible edges, so they always return 0.
func (g *GameScene) getDistancePastMapEdge(pos references.Position) int {
	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()

	// Large maps wrap, no edge darkness needed
	if mapType == references.LargeMapType {
		return 0
	}

	var maxX, maxY references.Coordinate
	if mapType == references.CombatMapType {
		maxX, maxY = references.XCombatMapTiles-1, references.YCombatMapTiles-1
	} else {
		// Small maps (towns, dungeons)
		maxX = g.gameState.GetCurrentSmallLocationReference().GetMaxX()
		maxY = g.gameState.GetCurrentSmallLocationReference().GetMaxY()
	}

	// Calculate distance past each edge (negative values mean in-bounds)
	distPastLeft := -int(pos.X)
	distPastRight := int(pos.X) - int(maxX)
	distPastTop := -int(pos.Y)
	distPastBottom := int(pos.Y) - int(maxY)

	// Return maximum distance past any edge (0 if in-bounds on all sides)
	return max(0, distPastLeft, distPastRight, distPastTop, distPastBottom)
}

// calculateEdgeDarkness returns a brightness factor for out-of-bounds tiles.
// Uses linear interpolation from EdgeStartBrightness (at edge) to EdgeMinBrightness (far out).
// Returns 1.0 for in-bounds positions (no darkening).
func calculateEdgeDarkness(distPastEdge int) float32 {
	if distPastEdge <= 0 {
		return 1.0 // Full brightness for in-bounds
	}
	if distPastEdge >= EdgeGradientMaxDistance {
		return EdgeMinBrightness
	}

	// Linear interpolation: start → end based on distance ratio
	t := float32(distPastEdge) / float32(EdgeGradientMaxDistance)

	return EdgeStartBrightness + (EdgeMinBrightness-EdgeStartBrightness)*t
}
