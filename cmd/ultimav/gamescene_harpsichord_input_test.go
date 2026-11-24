package main

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/assert"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/gamefixture"
)

func TestHarpsichordNumberKeysAreBoundAndNormalized(t *testing.T) {
	for digit := range 10 {
		topRow := ebiten.Key(int(ebiten.Key0) + digit)
		numpad := ebiten.Key(int(ebiten.KeyNumpad0) + digit)

		if !slices.Contains(boundKeysGame, topRow) {
			t.Fatalf("top-row digit %d is not bound", digit)
		}
		if !slices.Contains(boundKeysGame, numpad) {
			t.Fatalf("numpad digit %d is not bound", digit)
		}
		if got, ok := gameplayNumberKeyDigit(topRow); !ok || got != digit {
			t.Fatalf("top-row digit %d normalized to (%d, %t)", digit, got, ok)
		}
		if got, ok := gameplayNumberKeyDigit(numpad); !ok || got != digit {
			t.Fatalf("numpad digit %d normalized to (%d, %t)", digit, got, ok)
		}
	}

	if _, ok := gameplayNumberKeyDigit(ebiten.KeyA); ok {
		t.Fatal("letter key normalized as a gameplay number")
	}
}

func TestNumberKeyRoutingPreservesNumpadMovementAwayFromHarpsichord(t *testing.T) {
	state, _ := gamefixture.NewIntegrationTestBuilder(t).
		WithLocation(references.Britain).
		WithSystemCallbacks().
		WithRandomSeed(79).
		Build()
	scene := &GameScene{gameState: state}

	assert.False(t, scene.handleNumberKey(ebiten.KeyNumpad8))
	assert.True(t, scene.handleNumberKey(ebiten.Key8))
}
