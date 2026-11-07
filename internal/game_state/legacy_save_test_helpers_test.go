package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
)

func mustNewGameStateFromOriginalSaveBytes(
	rawSaveData []byte,
	gameConfig *config.UltimaVConfiguration,
	gameReferences *references.GameReferences,
	xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen int,
) *GameState {
	gameState, err := newGameStateFromOriginalSaveBytes(
		rawSaveData,
		gameConfig,
		gameReferences,
		xTilesVisibleOnGameScreen,
		yTilesVisibleOnGameScreen,
	)
	if err != nil {
		panic(err)
	}

	return gameState
}

// newBootstrapSaveTestState builds a state from the bootstrap save with mock
// callbacks attached.
func newBootstrapSaveTestState(t *testing.T) (*GameState, *MockSystemCallbacks) {
	t.Helper()
	gameConfig := helpers.LoadTestConfiguration(t)
	xTiles, yTiles := helpers.GetTestGameConstants()
	gs := mustNewGameStateFromOriginalSaveBytes(helpers.LoadBootstrapSaveData(t), gameConfig,
		helpers.LoadTestGameReferences(t, gameConfig), xTiles, yTiles)
	mockCallbacks := NewMockSystemCallbacks(t)
	gs.SystemCallbacks = mockCallbacks.ToSystemCallbacks()

	return gs, mockCallbacks
}
