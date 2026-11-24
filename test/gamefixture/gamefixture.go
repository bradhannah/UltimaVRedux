// Package gamefixture builds game-state integration fixtures for tests outside
// the game_state package.
package gamefixture

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
)

// NewIntegrationTestBuilder creates a builder over the original game data. The
// test is skipped when that data is not installed.
func NewIntegrationTestBuilder(t *testing.T) *game_state.IntegrationTestBuilder {
	t.Helper()

	return game_state.NewIntegrationTestBuilderFrom(t, helpers.IntegrationFixtureSource(t))
}
