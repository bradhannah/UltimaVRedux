package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isolatedConfigurationHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	selected, err := os.UserHomeDir()
	require.NoError(t, err)
	require.Equal(t, home, selected)

	return home
}

func TestExplicitConfigurationIgnoresOppositeHomePreferences(t *testing.T) {
	for _, homePreference := range []bool{true, false} {
		t.Run(fmt.Sprint(homePreference), func(t *testing.T) {
			home := isolatedConfigurationHome(t)
			directory := filepath.Join(home, ".ultima_v_redux")
			require.NoError(t, os.Mkdir(directory, 0o700))
			homeFile := filepath.Join(directory, "config.yaml")
			sentinel := []byte(fmt.Sprintf("# user preferences\nGameplay:\n  Presentation:\n    SmoothSurfaceMovement: %t\n", homePreference))
			require.NoError(t, os.WriteFile(homeFile, sentinel, 0o600))
			configuration, file := configurationFixture(t, fmt.Sprintf("Gameplay:\n  Presentation:\n    SmoothSurfaceMovement: %t\n", !homePreference))
			require.Equal(t, !homePreference, configuration.GameplayConfiguration.SmoothSurfaceMovement)
			for _, enabled := range []bool{homePreference, !homePreference} {
				require.NoError(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.SmoothSurfaceMovement = enabled }))
				loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
				require.NoError(t, err)
				assert.Equal(t, enabled, loaded.GameplayConfiguration.SmoothSurfaceMovement)
			}
			defaults, err := NewUltimaVConfigurationWithPath(filepath.Dir(file))
			require.NoError(t, err)
			assert.False(t, defaults.GameplayConfiguration.SmoothSurfaceMovement)
			require.ErrorIs(t, defaults.UpdateSaveFile(), ErrConfigFileUnknown)
			bytes, err := os.ReadFile(homeFile)
			require.NoError(t, err)
			assert.Equal(t, sentinel, bytes)
			entries, err := os.ReadDir(directory)
			require.NoError(t, err)
			assert.Len(t, entries, 1)
		})
	}
}

func TestExistingReadOnlyStartupConfigurationIsNotRewritten(t *testing.T) {
	home := isolatedConfigurationHome(t)
	directory := filepath.Join(home, ".ultima_v_redux")
	require.NoError(t, os.Mkdir(directory, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "DATA.OVL"), nil, 0o600))
	file := filepath.Join(directory, "config.yaml")
	contents := []byte(fmt.Sprintf("# preserve formatting and unknown settings\nDataFilePath: %q\nUnknown: KEEP\nAudio: {MusicVolume: 2}\n", home))
	require.NoError(t, os.WriteFile(file, contents, 0o400))
	t.Cleanup(func() { require.NoError(t, os.Chmod(file, 0o600)) })
	if runtime.GOOS != "windows" {
		// Windows Chmod controls the file read-only attribute, not directory write access.
		require.NoError(t, os.Chmod(directory, 0o500))
		t.Cleanup(func() { require.NoError(t, os.Chmod(directory, 0o700)) })
	}
	configuration, err := NewUltimaVConfiguration()
	require.NoError(t, err)
	assert.Equal(t, 2, configuration.AudioConfiguration.MusicVolume)
	bytes, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, contents, bytes)
	if runtime.GOOS != "windows" && os.Geteuid() != 0 {
		err = configuration.UpdateSaveFile()
		require.ErrorIs(t, err, os.ErrPermission)
		assert.Contains(t, err.Error(), "saving configuration")
	}
	bytes, err = os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, contents, bytes)
}

func TestSelectedConfigurationErrorsAreRecoverable(t *testing.T) {
	directory := t.TempDir()
	file := filepath.Join(directory, "selected.yaml")
	_, err := LoadUltimaVConfiguration(file, directory)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, file)
	require.NoError(t, os.WriteFile(file, []byte("invalid: ["), 0o600))
	_, err = LoadUltimaVConfiguration(file, directory)
	require.ErrorContains(t, err, "reading configuration")
	require.NoError(t, os.WriteFile(file, []byte("Resolution: [1, 2]\n"), 0o600))
	_, err = LoadUltimaVConfiguration(file, directory)
	require.ErrorContains(t, err, "decoding configuration")
	require.NoError(t, os.WriteFile(file, []byte("{}\n"), 0o600))
	_, err = LoadUltimaVConfiguration(file, directory)
	require.ErrorIs(t, err, os.ErrNotExist)
	require.ErrorContains(t, err, "DATA.OVL")
}

func TestMalformedLoggingCategorySourceRejectedBeforeSave(t *testing.T) {
	for _, categories := range []string{`true`, `[combat]`, `null`, `{combat: "true"}`, `{combat: 1}`, `{combat: null}`} {
		t.Run(categories, func(t *testing.T) {
			configuration, file := configurationFixture(t, "unknown: retained\n")
			configuration.LoggingConfiguration.CategoryStates["combat"] = true
			beforeSource := configuration.source.AllSettings()
			contents := []byte("Logging:\n  CategoryStates: " + categories + "\n")
			require.NoError(t, os.WriteFile(file, contents, 0o600))

			_, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
			require.ErrorIs(t, err, errConfigurationType)
			require.ErrorContains(t, err, "Logging.CategoryStates")
			require.ErrorIs(t, configuration.SaveLoggingConfiguration(), errConfigurationType,
				"normalizing the candidate must not hide malformed source being overwritten")
			assert.Equal(t, beforeSource, configuration.source.AllSettings())
			unchanged, err := os.ReadFile(file)
			require.NoError(t, err)
			assert.Equal(t, contents, unchanged)
		})
	}
}

func TestSelectedConfigurationPathRemainsBoundAfterDirectoryChange(t *testing.T) {
	directory := t.TempDir()
	t.Chdir(directory)
	require.NoError(t, os.WriteFile("preferences", []byte("unknown: retained\n"), 0o600))
	require.NoError(t, os.WriteFile("DATA.OVL", nil, 0o600))
	configuration, err := LoadUltimaVConfiguration("preferences", directory)
	require.NoError(t, err)
	t.Chdir(t.TempDir())
	require.NoError(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.SmoothSurfaceMovement = true }))
	persisted := readConfigurationFixture(t, filepath.Join(directory, "preferences"))
	assert.True(t, persisted.GetBool(gameplaySmoothSurfaceMovementKey))
	assert.Equal(t, "retained", persisted.GetString("unknown"))
	_, err = os.Stat("preferences")
	require.ErrorIs(t, err, os.ErrNotExist)
}

func TestStartupCreatesOnlyMissingDefaultConfiguration(t *testing.T) {
	home := isolatedConfigurationHome(t)
	// Missing game data remains a recoverable startup error after default creation.
	t.Chdir(home)
	_, err := NewUltimaVConfiguration()
	require.ErrorIs(t, err, ErrDataFilePathUnset)
	file := filepath.Join(home, ".ultima_v_redux", "config.yaml")
	assert.Empty(t, readConfigurationFixture(t, file).GetString("DataFilePath"))
	require.NoError(t, os.Mkdir(filepath.Join(home, "OLD"), 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(home, "OLD", "DATA.OVL"), nil, 0o600))
	require.NoError(t, os.WriteFile(file, []byte("DataFilePath: ./OLD\n"), 0o600))
	configuration, err := NewUltimaVConfiguration()
	require.NoError(t, err)
	require.NoError(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.SmoothSurfaceMovement = true }))
	assert.True(t, readConfigurationFixture(t, file).GetBool(gameplaySmoothSurfaceMovementKey))
}

func TestQoLMigrationDoesNotWriteOnLoadAndKeepsExternalDataPath(t *testing.T) {
	contents := "# keep until save\nDebug:\n  CombatMovementSpeedFactor: 0.75\nDataFilePath: old\nUnknown: retained\n"
	configuration, file := configurationFixture(t, contents)
	loadedBytes, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, contents, string(loadedBytes))
	assert.Equal(t, 1.0, configuration.GetCombatSpeedFactor())
	external := "Debug:\n  CombatMovementSpeedFactor: 0.75\nDataFilePath: new-selection\nUnknown: external\n"
	require.NoError(t, os.WriteFile(file, []byte(external), 0o600))
	require.NoError(t, configuration.SetCombatSpeedFactor(12))
	source := readConfigurationFixture(t, file)
	assert.Equal(t, "new-selection", source.GetString("DataFilePath"))
	assert.Equal(t, "external", source.GetString("Unknown"))
	assert.Equal(t, 12.0, source.GetFloat64(gameplayCombatSpeedKey))
}
