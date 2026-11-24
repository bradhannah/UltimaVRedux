package map_state

import (
	"slices"

	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// lightSourceCacheGridSize is the side, in tiles, of the grid cell that keeps
// cached light sources valid while the avatar stays inside it.
const lightSourceCacheGridSize = 5

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// RecalculateVisibleTilesWithMoongates recalculates visible tiles including moongate light sources
func (l *LayeredMap) RecalculateVisibleTilesWithMoongates(
	avatarPos references.Position,
	lighting *Lighting,
	moongatePositions []references.Position,
	playerLightType PlayerLightType,
) {
	l.TopLeft = references.Position{
		X: avatarPos.X - overflowTiles,
		Y: avatarPos.Y - overflowTiles,
	}
	l.BottomRight = references.Position{
		X: avatarPos.X + overflowTiles,
		Y: avatarPos.Y + overflowTiles,
	}

	// Resetting and then marking the rectangle could be done in one pass.
	l.testForVisibilityMap.ResetVisibilityCoords(false)
	l.testForVisibilityMap.setVisibilityCoordsRectangle(
		&l.TopLeft,
		&l.BottomRight,
		l.XMaxTilesPerMap,
		l.YMaxTilesPerMap,
		l.bWrappingMap,
	)
	l.visibleFlags.ResetVisibilityCoords(false)
	l.visiblePositions = l.visiblePositions[:0]

	l.SetVisible(true, &avatarPos)

	l.setTilesAroundPositionVisible(&avatarPos)

	l.floodFillIfInside(&avatarPos, true)
	l.floodFillIfInside(avatarPos.GetPositionToLeft(), true)
	l.floodFillIfInside(avatarPos.GetPositionToRight(), true)
	l.floodFillIfInside(avatarPos.GetPositionDown(), true)
	l.floodFillIfInside(avatarPos.GetPositionUp(), true)

	// get a full game screen lighting map
	l.primaryDistanceMaskMap = lighting.BuildGameScreenDistanceMap(avatarPos)

	// get lighting sources including moongates and overlay them of a light map
	lightSources := l.getCachedOrCalculateLightSources(avatarPos, moongatePositions)
	l.lightSourcesDistanceMap = lighting.BuildLightSourceDistanceMap(lightSources,
		l.visibleFlags,
		playerLightType,
		avatarPos,
	)
}

// getAllLightSourcesInRangeWithMoongates gets light sources including active moongates
func (l *LayeredMap) getAllLightSourcesInRangeWithMoongates(
	centerPosition references.Position,
	moongatePositions []references.Position,
) LightSources {
	lightSources := l.getAllLightSourcesInRange(centerPosition)
	l.addMoongateLightSources(&lightSources, moongatePositions)

	return lightSources
}

// getCachedOrCalculateLightSources returns cached light sources if valid, otherwise calculates and caches them
func (l *LayeredMap) getCachedOrCalculateLightSources(
	avatarPos references.Position,
	moongatePositions []references.Position,
) LightSources {
	// Optimization: Cache light sources based on a 5x5 grid position
	// This means we only recalculate when the avatar moves to a different grid cell
	// This reduces calculations by ~25x (5x5 grid) while maintaining accuracy
	gridSize := references.Coordinate(lightSourceCacheGridSize)
	gridX := avatarPos.X / gridSize
	gridY := avatarPos.Y / gridSize
	cachedGridX := l.cachedAvatarPos.X / gridSize
	cachedGridY := l.cachedAvatarPos.Y / gridSize

	// Check if cache is valid
	if l.lightSourceCacheValid &&
		gridX == cachedGridX && gridY == cachedGridY &&
		slices.Equal(l.cachedMoongatePositions, moongatePositions) {
		// Avatar is still in the same grid cell and moongates haven't changed
		return l.cachedLightSources
	}

	// Recalculate and cache
	l.cachedLightSources = l.getAllLightSourcesInRangeWithMoongates(avatarPos, moongatePositions)
	l.cachedAvatarPos = avatarPos
	l.cachedMoongatePositions = slices.Clone(moongatePositions) // Deep copy to avoid external modification
	l.lightSourceCacheValid = true

	return l.cachedLightSources
}

// IsTileLit returns true if the tile at the given position is lit by any light source.
func (ls *LightSourceTintingState) IsTileLit(pos references.Position) bool {
	contributions, ok := ls.litTilesMap[pos]

	return ok && len(contributions) > 0
}

// GetLitTileCount returns the number of tiles currently lit.
func (ls *LightSourceTintingState) GetLitTileCount() int {
	return len(ls.litTilesMap)
}
