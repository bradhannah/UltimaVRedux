package game_state

import (
	"testing"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/references"
	"github.com/bradhannah/UltimaVRedux/test/helpers"
)

// Helper function for creating test game state with real data (as per TESTING.md)
func createTestGameStateWithRealData(t *testing.T, location references.Location) (*GameState, *config.UltimaVConfiguration, *references.GameReferences) {
	t.Helper()

	gameConfig := loadTestConfiguration(t)
	if gameConfig == nil {
		return nil, nil, nil
	}

	gameRefs := loadTestGameReferences(t, gameConfig)
	if gameRefs == nil {
		return nil, nil, nil
	}

	// Create game state with real data
	gameState, err := newGameStateFromOriginalSaveFile(
		gameConfig.SavedConfigData.DataFilePath+"/SAVED.GAM",
		gameConfig,
		gameRefs,
		16, 16) // Standard screen dimensions for testing
	if err != nil {
		t.Fatalf("loading legacy test save: %v", err)
	}

	// Initialize to specified location if needed
	if location != references.BritanniaUnderworld {
		gameState.MapState.PlayerLocation.Location = location
	}

	return gameState, gameConfig, gameRefs
}

// Helper for loading test configuration (tries multiple common locations)
func loadTestConfiguration(t *testing.T) *config.UltimaVConfiguration {
	t.Helper()

	return helpers.LoadTestConfiguration(t)
}

func loadTestGameReferences(t *testing.T, gameConfig *config.UltimaVConfiguration) *references.GameReferences {
	t.Helper()

	return helpers.LoadTestGameReferences(t, gameConfig)
}
