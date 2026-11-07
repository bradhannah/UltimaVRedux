package game_state

import (
	"context"
	"fmt"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/legacy_save"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

// newGameStateFromOriginalSaveFile loads an installed SAVED.GAM and its
// SAVED.OOL sibling exactly as a player's save loads.
func newGameStateFromOriginalSaveFile(savedGamFilePath string,
	gameConfig *config.UltimaVConfiguration,
	gameReferences *references.GameReferences,
	xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen int,
) (*GameState, error) {
	bundle, err := legacy_save.ReadBundleContext(context.Background(), savedGamFilePath)
	if err != nil {
		return nil, fmt.Errorf("reading original save %s: %w", savedGamFilePath, err)
	}

	return newGameStateFromOriginalBundle(bundle,
		gameConfig, gameReferences, xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen)
}

// Focused scroll fixtures leave command completion to their test harness.
func (g *GameState) useScroll(scroll references.Scroll) bool {
	return g.useScrollWithCompletion(scroll, nil)
}
