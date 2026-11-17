package main

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

func TestVisualEffectPositionPreservesPlayerCenteredWorldGeometry(t *testing.T) {
	world := references.Position{X: 12, Y: 8}
	player := references.Position{X: 10, Y: 9}
	wantX, wantY := sprites.ConvertPosition16to18(int(xCenter)+2, int(yCenter)-1)

	gotX, gotY := visualEffectPositionToSourcePixels(world, player)

	assert.Equal(t, wantX, gotX)
	assert.Equal(t, wantY, gotY)
}
