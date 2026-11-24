// Overlay System - Generalized debug overlay rendering for game visualization
//
// This system provides a unified way to render debug overlays on the game map,
// supporting various overlay types like lighting, search locations, pathfinding, etc.
package main

import (
	"image/color"
	"strconv"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/ebitenutil"
	"github.com/hajimehoshi/ebiten/v2/vector"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
	ucolor "github.com/bradhannah/UltimaVRedux/pkg/color"
)

// OverlayRenderer handles rendering of debug overlays on the game map
type OverlayRenderer struct {
	// Pre-allocated overlay image to avoid per-frame allocations
	overlayImage *ebiten.Image

	// Colors for different overlay types
	moonstoneOverlayColor color.RGBA

	// Overlay enable flags
	moonstoneOverlayEnabled bool
}

// NewOverlayRenderer creates a new overlay renderer with default colors
func NewOverlayRenderer() *OverlayRenderer {
	return &OverlayRenderer{
		moonstoneOverlayColor:   ucolor.DebugMoonstoneOverlay.RGBA(), // Purple for moonstones
		moonstoneOverlayEnabled: false,
	}
}

// initializeOverlayImage creates or resizes the overlay image buffer
func (r *OverlayRenderer) initializeOverlayImage(width, height int) {
	if r.overlayImage == nil || r.overlayImage.Bounds().Dx() != width || r.overlayImage.Bounds().Dy() != height {
		r.overlayImage = ebiten.NewImage(width, height)
	}
}

// RenderOverlays draws all active debug overlays onto the screen
func (r *OverlayRenderer) RenderOverlays(screen *ebiten.Image, g *GameScene) {
	// Initialize overlay image if needed
	mapWidth := sprites.TileSize * xTilesVisibleOnGameScreen
	mapHeight := sprites.TileSize * yTilesVisibleOnGameScreen
	r.initializeOverlayImage(mapWidth, mapHeight)

	// Clear the overlay image
	r.overlayImage.Clear()

	// Check which overlays are enabled and render them
	if g.gameState.DebugOptions.LightOverlay {
		r.renderLightOverlay(g)
	}

	if g.gameState.DebugOptions.SearchOverlay {
		r.renderSearchOverlay(g)
	}

	if r.moonstoneOverlayEnabled {
		r.renderMoonstoneOverlay(g)
	}

	// Draw the combined overlay onto the screen
	if g.gameState.DebugOptions.LightOverlay || g.gameState.DebugOptions.SearchOverlay || r.moonstoneOverlayEnabled {
		op := sprites.GetDrawOptionsFromPercentsForWholeScreen(r.overlayImage, g.mapScreenPlacement())
		screen.DrawImage(r.overlayImage, op)
	}
}

// renderLightOverlay draws the lighting debug overlay
func (r *OverlayRenderer) renderLightOverlay(g *GameScene) {
	// Get the current layered map
	layer := g.gameState.GetLayeredMapByCurrentLocation()
	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()

	// Get the lighting distance maps directly from the layered map
	// These are already calculated during the normal rendering process
	primaryDistanceMap := layer.GetPrimaryDistanceMap()
	lightSourcesDistanceMap := layer.GetLightSourcesDistanceMap()

	// Render each visible tile with appropriate lighting overlay
	for tileX := range references.Coordinate(xTilesVisibleOnGameScreen) {
		for tileY := range references.Coordinate(yTilesVisibleOnGameScreen) {
			// Calculate world position
			worldPos := references.Position{
				X: tileX + g.gameState.MapState.PlayerLocation.Position.X - xCenter,
				Y: tileY + g.gameState.MapState.PlayerLocation.Position.Y - yCenter,
			}

			// Handle wrapping for large maps
			if mapType == references.LargeMapType {
				worldPos = *worldPos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			}

			// Check visibility using the lighting system
			isVisible := g.isPositionVisible(layer, &worldPos)

			// Calculate intensity based on distance from light sources
			intensity := uint8(0)
			distance := 999 // Default to far away

			if isVisible {
				// Check primary light (avatar position)
				if dist, ok := primaryDistanceMap[worldPos]; ok && dist < distance {
					distance = dist
				}

				// Check secondary light sources (torches, braziers, etc)
				if dist, ok := lightSourcesDistanceMap[worldPos]; ok && dist < distance {
					distance = dist
				}

				// Calculate intensity based on distance
				// Distance 0 = full brightness, distance increases = dimmer
				if distance < 10 {
					// Convert distance to intensity (0-255 scale)
					// Distance 0 = 255, Distance 1 = 200, Distance 2 = 150, etc.
					intensity = uint8(255 - (distance * 25))
					if intensity < 30 {
						intensity = 30 // Minimum visibility
					}
				}
			}

			// Draw the light overlay tile with distance-based coloring
			if intensity > 0 {
				var overlayColor color.RGBA

				// Use EGA colors with intensity and alpha adjustment via ColorBuilder
				if distance > 3 {
					// Distant tiles = dim orange (EGA brown with intensity and alpha adjustment)
					overlayColor = ucolor.From(ucolor.EGABrown).Intensity(float32(intensity) / 255.0).Alpha(intensity / 3).RGBA()
				} else {
					// Close tiles = bright yellow with intensity and alpha adjustment
					overlayColor = ucolor.From(ucolor.EGAYellow).Intensity(float32(intensity) / 255.0).Alpha(intensity / 3).RGBA()
				}

				vector.FillRect(
					r.overlayImage,
					float32(tileX*sprites.TileSize),
					float32(tileY*sprites.TileSize),
					float32(sprites.TileSize),
					float32(sprites.TileSize),
					overlayColor,
					false,
				)
			}
		}
	}
}

// renderSearchOverlay draws the search location debug overlay
func (r *OverlayRenderer) renderSearchOverlay(g *GameScene) {
	// Get the search manager to find searchable positions
	searchManager := g.gameState.SearchManager
	if searchManager == nil {
		// Programming error: search manager should be initialized during game state creation
		panic("OverlayRenderer.renderSearchOverlay: searchManager is nil - should never happen")
	}

	mapType := g.gameState.MapState.PlayerLocation.Location.GetMapType()

	// Render searchable positions within the visible area
	for tileX := range references.Coordinate(xTilesVisibleOnGameScreen) {
		for tileY := range references.Coordinate(yTilesVisibleOnGameScreen) {
			// Calculate world position
			worldPos := references.Position{
				X: tileX + g.gameState.MapState.PlayerLocation.Position.X - xCenter,
				Y: tileY + g.gameState.MapState.PlayerLocation.Position.Y - yCenter,
			}

			// Handle wrapping for large maps
			if mapType == references.LargeMapType {
				worldPos = *worldPos.GetWrapped(references.XLargeMapTiles, references.YLargeMapTiles)
			}

			// Get the search status for this position
			searchStatus := searchManager.GetSearchLocationStatus(&worldPos)

			if searchStatus != game_state.SearchLocationNotSearchable {
				// Draw search overlay with pulsing effect
				elapsedMs := g.getElapsedMs()

				// Create pulsing alpha effect
				pulsePhase := float64(elapsedMs%1000) / 1000.0
				alpha := uint8(30 + 30*pulsePhase) // Pulse between 30 and 60 alpha

				// Choose color based on search status using EGA constants with ColorBuilder
				var overlayColor color.RGBA
				if searchStatus == game_state.SearchLocationAlreadyFound {
					// Red for already searched locations using EGA constants
					overlayColor = ucolor.DebugSearchAlreadyFound.Alpha(alpha).RGBA()
				} else {
					// Green for unsearched locations (SearchLocationUnsearched) using EGA constants
					overlayColor = ucolor.DebugSearchUnsearched.Alpha(alpha).RGBA()
				}

				vector.FillRect(
					r.overlayImage,
					float32(tileX*sprites.TileSize),
					float32(tileY*sprites.TileSize),
					float32(sprites.TileSize),
					float32(sprites.TileSize),
					overlayColor,
					false,
				)

				// Add a border for extra visibility using EGA constants with ColorBuilder
				var borderColor color.RGBA
				if searchStatus == game_state.SearchLocationAlreadyFound {
					// Red border for searched locations using EGA constants
					borderColor = ucolor.DebugSearchBorderFound.Alpha(alpha * 2).RGBA()
				} else {
					// Green border for unsearched locations using EGA constants
					borderColor = ucolor.DebugSearchBorderUnsearched.Alpha(alpha * 2).RGBA()
				}

				// Top border
				vector.FillRect(r.overlayImage,
					float32(tileX*sprites.TileSize), float32(tileY*sprites.TileSize),
					float32(sprites.TileSize), 2, borderColor, false)
				// Bottom border
				vector.FillRect(r.overlayImage,
					float32(tileX*sprites.TileSize), float32((tileY+1)*sprites.TileSize-2),
					float32(sprites.TileSize), 2, borderColor, false)
				// Left border
				vector.FillRect(r.overlayImage,
					float32(tileX*sprites.TileSize), float32(tileY*sprites.TileSize),
					2, float32(sprites.TileSize), borderColor, false)
				// Right border
				vector.FillRect(r.overlayImage,
					float32((tileX+1)*sprites.TileSize-2), float32(tileY*sprites.TileSize),
					2, float32(sprites.TileSize), borderColor, false)
			}
		}
	}
}

// Note: isPositionSearchable has been replaced by GetSearchLocationStatus enum method

// SetMoonstoneOverlayEnabled explicitly controls moonstone overlay rendering.
func (r *OverlayRenderer) SetMoonstoneOverlayEnabled(enabled bool) {
	r.moonstoneOverlayEnabled = enabled
}

// IsMoonstoneOverlayEnabled returns whether the moonstone overlay is enabled
func (r *OverlayRenderer) IsMoonstoneOverlayEnabled() bool {
	return r.moonstoneOverlayEnabled
}

// renderMoonstoneOverlay draws the moonstone location debug overlay
func (r *OverlayRenderer) renderMoonstoneOverlay(g *GameScene) {
	// Only render on large map
	if g.gameState.MapState.PlayerLocation.Location.GetMapType() != references.LargeMapType {
		return
	}

	// Get all moonstones
	moonstones := g.gameState.GetMoonstones()

	// Get player position for viewport calculation
	playerPos := g.gameState.MapState.PlayerLocation.Position

	// Get current moon phase to highlight active moonstone
	currentPhase := int(g.gameState.DateTime.GetCurrentMoonPhase())

	// Draw each moonstone location
	for i, moonstone := range moonstones {
		// Skip moonstones in inventory
		if moonstone.Map == 0xFF {
			continue
		}

		// Calculate relative position from player
		relX := int(moonstone.X) - int(playerPos.X)
		relY := int(moonstone.Y) - int(playerPos.Y)

		// Check if moonstone is visible on screen
		halfScreenWidth := xTilesVisibleOnGameScreen / 2
		halfScreenHeight := yTilesVisibleOnGameScreen / 2

		if relX < -halfScreenWidth || relX > halfScreenWidth ||
			relY < -halfScreenHeight || relY > halfScreenHeight {
			// Moonstone is off-screen
			continue
		}

		// Calculate screen tile position
		tileX := halfScreenWidth + relX
		tileY := halfScreenHeight + relY

		// Choose color based on moonstone state
		var overlayColor color.RGBA
		switch {
		case i == currentPhase:
			// Active moonstone - bright purple
			overlayColor = ucolor.MoonstoneActiveBright.RGBA()
		case moonstone.IsFound:
			// Found moonstone - green
			overlayColor = ucolor.MoonstoneFoundGreen.RGBA()
		default:
			// Buried moonstone - dim purple
			overlayColor = r.moonstoneOverlayColor
		}

		// Draw moonstone marker
		vector.FillRect(r.overlayImage,
			float32(tileX*sprites.TileSize), float32(tileY*sprites.TileSize),
			float32(sprites.TileSize), float32(sprites.TileSize),
			overlayColor, false)

		// Draw moonstone index number in center
		indexStr := strconv.Itoa(i)
		ebitenutil.DebugPrintAt(r.overlayImage, indexStr,
			tileX*sprites.TileSize+sprites.TileSize/2-4,
			tileY*sprites.TileSize+sprites.TileSize/2-4)

		// Draw border for active moonstone
		if i == currentPhase {
			borderColor := ucolor.MoonstoneActiveBorder.RGBA()
			// Top border
			vector.FillRect(r.overlayImage,
				float32(tileX*sprites.TileSize), float32(tileY*sprites.TileSize),
				float32(sprites.TileSize), 2, borderColor, false)
			// Bottom border
			vector.FillRect(r.overlayImage,
				float32(tileX*sprites.TileSize), float32((tileY+1)*sprites.TileSize-2),
				float32(sprites.TileSize), 2, borderColor, false)
			// Left border
			vector.FillRect(r.overlayImage,
				float32(tileX*sprites.TileSize), float32(tileY*sprites.TileSize),
				2, float32(sprites.TileSize), borderColor, false)
			// Right border
			vector.FillRect(r.overlayImage,
				float32((tileX+1)*sprites.TileSize-2), float32(tileY*sprites.TileSize),
				2, float32(sprites.TileSize), borderColor, false)
		}
	}
}
