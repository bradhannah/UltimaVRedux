// Package helpers provides test fixtures that load the original game data and its references.
package helpers

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/originaldata"
)

// LoadTestConfiguration loads the configuration for the original game data; the test is
// skipped or failed when that data is unavailable.
func LoadTestConfiguration(t *testing.T) *config.UltimaVConfiguration {
	t.Helper()

	cfg, err := LoadConfiguration()
	originaldata.Check(t, err)

	return cfg
}

// LoadConfiguration creates fresh in-memory preferences for the selected data.
// Parsing DATA.OVL here distinguishes corrupt data from optional absence before
// any reference cache lookup, even for tests that only request configuration.
func LoadConfiguration() (*config.UltimaVConfiguration, error) {
	directory, err := originaldata.ResolveDirectory()
	if err != nil {
		return nil, fmt.Errorf("selecting test data: %w", err)
	}
	cfg, err := config.LoadUltimaVConfiguration("", directory)
	if err != nil {
		return nil, fmt.Errorf("loading test configuration from %s: %w", directory, err)
	}
	if _, err := references.NewDataOvl(cfg); err != nil {
		return nil, fmt.Errorf("validating DATA.OVL from %s: %w", directory, err)
	}

	return cfg, nil
}

// Cache only references, not mutable preferences or failures. Tests treat the
// selected installation as immutable for the lifetime of the process.
//
//nolint:gochecknoglobals // Shared integration references are expensive and keyed by canonical data source.
var (
	gameReferencesMu       sync.Mutex
	gameReferencesBySource = make(map[string]*references.GameReferences)
)

// LoadGameData reuses references by canonical selected source, with fresh config.
func LoadGameData() (*config.UltimaVConfiguration, *references.GameReferences, error) {
	cfg, err := LoadConfiguration()
	if err != nil {
		return nil, nil, err
	}
	gameReferencesMu.Lock()
	defer gameReferencesMu.Unlock()
	directory := cfg.SavedConfigData.DataFilePath
	refs := gameReferencesBySource[directory]
	if refs == nil {
		refs, err = references.NewGameReferences(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("loading test references from %s: %w", directory, err)
		}
		gameReferencesBySource[directory] = refs
	}

	return cfg, refs, nil
}

// IntegrationFixtureSource returns the loader a game-state integration fixture
// reads its data through: fresh configuration, cached references and the
// bootstrap save. The test is skipped or failed when the original game data is
// unavailable. Its result is a game_state.IntegrationFixtureSource; this
// package cannot name that type because game_state's own tests import it.
func IntegrationFixtureSource(
	t *testing.T,
) func() (*config.UltimaVConfiguration, *references.GameReferences, []byte) {
	t.Helper()

	return func() (*config.UltimaVConfiguration, *references.GameReferences, []byte) {
		t.Helper()
		cfg, refs, err := LoadGameData()
		originaldata.Check(t, err)

		return cfg, refs, LoadBootstrapSaveData(t)
	}
}

// LoadTestGameReferences loads the game references for config, failing the test on error.
func LoadTestGameReferences(t *testing.T, config *config.UltimaVConfiguration) *references.GameReferences {
	t.Helper()

	gameRefs, err := references.NewGameReferences(config)
	if err != nil {
		t.Fatalf("Failed to load game references: %v", err)
	}

	return gameRefs
}

// GetTestGameConstants returns the constants needed for GameState creation
func GetTestGameConstants() (int, int) {
	const (
		xTilesVisibleOnGameScreen = 19
		yTilesVisibleOnGameScreen = 13
	)

	return xTilesVisibleOnGameScreen, yTilesVisibleOnGameScreen
}

// LoadBootstrapSaveData loads the bootstrap save file for testing
func LoadBootstrapSaveData(t *testing.T) []byte {
	t.Helper()

	_, source, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate checked-in bootstrap save fixture")
	}
	saveFilePath := filepath.Join(filepath.Dir(source), "../../internal/game_state/testdata/britain2_SAVED.GAM")
	//nolint:gosec // Fixed repository fixture resolved from this helper's source file.
	data, err := os.ReadFile(saveFilePath)
	if err != nil {
		t.Fatalf("Failed to load bootstrap save fixture: %v", err)
	}

	return data
}

// Note: CreateMockSystemCallbacks moved to individual test files to avoid import cycle
