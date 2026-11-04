package references

import (
	"github.com/bradhannah/UltimaVRedux/internal/sprites/indexes"
)

// Tiles is the tile table by sprite index.
type Tiles map[indexes.SpriteIndex]*Tile

// GetTile returns the tile for the sprite index, or nil when it is unknown.
func (t *Tiles) GetTile(tileNum indexes.SpriteIndex) *Tile {
	tile, exists := (*t)[tileNum]
	if !exists {
		return nil // Return nil if the tileNum doesn't exist
	}

	return tile
}

// NewTileReferences returns the tile table.
func NewTileReferences() *Tiles {
	tiles := newTileDefinitions()
	for index, tile := range tiles {
		tile.Index = index
	}

	return &tiles
}
