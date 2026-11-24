package shrine

import (
	"github.com/bradhannah/UltimaVRedux/internal/logging"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// NewShrineMap creates a new shrine map with default positions
func NewShrineMap() *Map {
	return &Map{
		CenterPosition:   references.Position{X: CenterPositionX, Y: CenterPositionY},
		EntrancePosition: references.Position{X: EntrancePositionX, Y: EntrancePositionY},
		MapData:          nil, // No loaded data, use fallback rendering
	}
}

// NewShrineScene creates a new shrine scene for meditation
func NewShrineScene(shrineIndex int, originalPlayerLocation references.PlayerLocation) *Scene {
	scene := &Scene{
		ShrineIndex:            shrineIndex,
		IsActive:               true,
		OriginalPlayerLocation: originalPlayerLocation,
		ShrineMap:              NewShrineMap(),
	}

	logging.Debugf(logging.CategoryGameState, "Created shrine scene for shrine %d", shrineIndex)

	return scene
}
