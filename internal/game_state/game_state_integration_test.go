package game_state

import (
	"errors"
	"sync"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"

	_ "embed"
)

const (
	xTilesVisibleOnGameScreen = 19
	yTilesVisibleOnGameScreen = 13
)

//go:embed testdata/britain2_SAVED.GAM
var saveFile []byte

var (
	baseGameReferences   *references.GameReferences
	cachedIntegrationCfg *config.UltimaVConfiguration
	integrationDataMu    sync.Mutex
)

func getBaseGameReferences() *references.GameReferences {
	integrationDataMu.Lock()
	defer integrationDataMu.Unlock()
	cfg, refs, err := helpers.LoadGameData()
	if err != nil {
		if errors.Is(err, originaldata.ErrUnavailable) && !originaldata.Required() {
			if baseGameReferences != nil {
				cachedIntegrationCfg, baseGameReferences = nil, nil
			}

			return nil
		}
		// Legacy snapshot callers have no testing.T argument. Keep their existing
		// nil-on-optional-absence contract, but never hide selected-data failures.
		panic(err)
	}
	// Existing legacy callers read these globals after initialization. Do not
	// rewrite them on same-source cache hits, preserving concurrent read safety.
	if baseGameReferences != refs {
		cachedIntegrationCfg, baseGameReferences = cfg, refs
	}

	return baseGameReferences
}

func getBritain2GameState() *GameState {
	getBaseGameReferences()

	if cachedIntegrationCfg == nil {
		return nil // No configuration available
	}

	baseState := mustNewGameStateFromOriginalSaveBytes(
		saveFile, cachedIntegrationCfg, baseGameReferences,
		xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen,
	)

	return baseState
}
