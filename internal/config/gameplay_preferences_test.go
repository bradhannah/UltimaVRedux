package config

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQoLDefaultsDistinguishAbsentAndExplicitValues(t *testing.T) {
	configuration, _ := configurationFixture(t, "{}\n")
	assert.Equal(t, DefaultGameplayConfiguration(), configuration.GameplayConfiguration)
	for _, config := range []*UltimaVConfiguration{nil, {}} {
		assert.Equal(t, 1.0, config.GetCombatSpeedFactor())
		assert.True(t, config.GetConfirmQuickSave())
		assert.True(t, config.GetShowCombatHPBars())
		assert.Equal(t, 3, config.GetMaxVisibleActionPopups())
	}
	configuration, file := configurationFixture(t, "Gameplay:\n  Combat:\n    SpeedFactor: 2\n    ShowHealthBars: false\n  Saves:\n    ConfirmQuickSave: false\n  Presentation:\n    MaxVisibleActionPopups: 0\n")
	require.NoError(t, configuration.UpdateSaveFile())
	loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
	require.NoError(t, err)
	assert.Equal(t, 2.0, loaded.GetCombatSpeedFactor())
	assert.False(t, loaded.GetConfirmQuickSave())
	assert.False(t, loaded.GetShowCombatHPBars())
	assert.Zero(t, loaded.GetMaxVisibleActionPopups())
}

func TestQoLEveryNumericValueRoundTrips(t *testing.T) {
	configuration, file := configurationFixture(t, "unknown: retained\n")
	pointer := configuration.GameplayConfiguration
	for _, speed := range CombatSpeedSpec().Choices() {
		for count := 0; count <= 3; count++ {
			require.NoError(t, configuration.SetCombatSpeedFactor(speed))
			require.NoError(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.MaxVisibleActionPopups = count }))
			loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
			require.NoError(t, err)
			assert.Equal(t, speed, loaded.GetCombatSpeedFactor())
			assert.Equal(t, count, loaded.GetMaxVisibleActionPopups())
			assert.Same(t, pointer, configuration.GameplayConfiguration)
		}
	}
}

func TestQoLStrictSourceValidation(t *testing.T) {
	for _, contents := range []string{"Gameplay:\n  Combat:\n    SpeedFactor: null\n", "Gameplay:\n  Presentation:\n    MaxVisibleActionPopups: null\n"} {
		source := viper.New()
		source.SetConfigType("yaml")
		require.NoError(t, source.ReadConfig(bytes.NewBufferString(contents)))
		_, err := newUltimaVConfiguration(source, "", false)
		require.Error(t, err)
	}
	for _, test := range []struct {
		key   string
		value any
	}{
		{gameplayCombatSpeedKey, 0}, {gameplayCombatSpeedKey, 0.75}, {gameplayCombatSpeedKey, 20.5},
		{gameplayCombatSpeedKey, "2"}, {gameplayCombatSpeedKey, true},
		{gameplayCombatSpeedKey, math.NaN()}, {gameplayCombatSpeedKey, math.Inf(1)},
		{gameplayMaxActionPopupsKey, 1.5}, {gameplayMaxActionPopupsKey, 1.0},
		{gameplayMaxActionPopupsKey, -1}, {gameplayMaxActionPopupsKey, 4},
		{gameplayConfirmQuickSaveKey, "false"}, {gameplayShowCombatHPBarsKey, 0},
	} {
		t.Run(fmt.Sprintf("%s=%v", test.key, test.value), func(t *testing.T) {
			source := viper.New()
			source.Set(test.key, test.value)
			_, err := newUltimaVConfiguration(source, "", false)
			require.Error(t, err)
		})
	}
}

func TestLegacySpeedMigrationAndCanonicalPrecedence(t *testing.T) {
	for _, test := range []struct{ legacy, want float64 }{
		{0.25, 0.5}, {0.75, 1}, {1.25, 1.5}, {2.25, 2}, {2.5, 3}, {3.5, 4}, {3.9, 4}, {2, 2},
	} {
		configuration, file := configurationFixture(t, fmt.Sprintf("Debug:\n  CombatMovementSpeedFactor: %g\n", test.legacy))
		assert.Equal(t, test.want, configuration.GetCombatSpeedFactor())
		if test.legacy != test.want {
			require.Len(t, configuration.configurationDiagnostics, 1)
			assert.Contains(t, configuration.configurationDiagnostics[0], fmt.Sprintf("%g to %g", test.legacy, test.want))
		} else {
			assert.Empty(t, configuration.configurationDiagnostics)
		}
		require.NoError(t, configuration.SaveDebugConfiguration())
		assert.Equal(t, test.want, readConfigurationFixture(t, file).GetFloat64(gameplayCombatSpeedKey))
		require.NoError(t, configuration.SetCombatSpeedFactor(3))
		configuration.DebugConfiguration.SetCombatMovementSpeedFactor(0.25)
		require.NoError(t, configuration.SaveDebugConfiguration())
		require.NoError(t, configuration.UpdateSaveFile())
		loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
		require.NoError(t, err)
		assert.Equal(t, 3.0, loaded.GetCombatSpeedFactor(), "debug/F2/full saves cannot restore legacy authority")
	}
	configuration, _ := configurationFixture(t, "Gameplay:\n  Combat:\n    SpeedFactor: 16\nDebug:\n  CombatMovementSpeedFactor: 1.25\n")
	assert.Equal(t, 16.0, configuration.GetCombatSpeedFactor())
	assert.Empty(t, configuration.configurationDiagnostics)
}

func TestFormerCanonicalSpeedGridValuesMoveToNearestChoice(t *testing.T) {
	for _, test := range []struct{ saved, want float64 }{{2.5, 3}, {3.5, 4}, {5, 6}, {13.5, 12}} {
		configuration, file := configurationFixture(t, fmt.Sprintf("Gameplay:\n  Combat:\n    SpeedFactor: %g\n", test.saved))
		assert.Equal(t, test.want, configuration.GetCombatSpeedFactor())
		require.Len(t, configuration.configurationDiagnostics, 1)
		assert.Contains(t, configuration.configurationDiagnostics[0], fmt.Sprintf("combat speed %g to %g", test.saved, test.want))
		require.NoError(t, configuration.SaveGameplayConfiguration())
		assert.Equal(t, test.want, readConfigurationFixture(t, file).GetFloat64(gameplayCombatSpeedKey))
	}
}

func TestQoLInvalidPersistenceDoesNotMutateSourceOrFile(t *testing.T) {
	configuration, file := configurationFixture(t, "unknown: retained\n")
	beforeFile, err := os.ReadFile(file)
	require.NoError(t, err)
	beforeSource := configuration.source.AllSettings()
	before := *configuration.GameplayConfiguration
	for _, speed := range []float64{0, 0.75, 2.5, 4.1, 5, 21, math.NaN(), math.Inf(1)} {
		require.ErrorIs(t, configuration.SetCombatSpeedFactor(speed), ErrGameplayPreference)
		settings := ConfigurationSettings{Audio: *configuration.AudioConfiguration, Display: *configuration.DisplayConfiguration, Gameplay: before}
		settings.Gameplay.CombatSpeedFactor = speed
		require.ErrorIs(t, configuration.SaveConfigurationSettings(settings), ErrGameplayPreference)
	}
	require.ErrorIs(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.MaxVisibleActionPopups = 4 }), ErrGameplayPreference)
	assert.Equal(t, before, *configuration.GameplayConfiguration)
	configuration.GameplayConfiguration.CombatSpeedFactor = 0
	for _, save := range []func() error{configuration.SaveGameplayConfiguration, configuration.SaveDebugConfiguration,
		configuration.SaveAudioConfiguration, configuration.SaveLoggingConfiguration, configuration.UpdateSaveFile} {
		require.ErrorIs(t, save(), ErrGameplayPreference)
	}
	assert.Equal(t, beforeSource, configuration.source.AllSettings())
	afterFile, err := os.ReadFile(file)
	require.NoError(t, err)
	assert.Equal(t, beforeFile, afterFile)
}

func TestQoLStagedConflictAndFailedWritePreserveLiveSettings(t *testing.T) {
	configuration, file := configurationFixture(t, "unknown: retained\n")
	before := *configuration.GameplayConfiguration
	release := configuration.BeginConfigurationEditing()
	assert.True(t, configuration.ConfigurationEditing())
	for _, mutate := range []func() error{
		func() error { return configuration.SetCombatSpeedFactor(2) },
		func() error {
			return configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.ConfirmQuickSave = false })
		},
		func() error {
			return configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.ShowCombatHPBars = false })
		},
		func() error {
			return configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.MaxVisibleActionPopups = 0 })
		},
		func() error {
			return configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.SmoothSurfaceMovement = true })
		},
	} {
		require.ErrorIs(t, mutate(), ErrConfigurationEditing)
	}
	assert.Equal(t, before, *configuration.GameplayConfiguration)
	release()
	assert.False(t, configuration.ConfigurationEditing())
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o700))
	require.Error(t, configuration.SetCombatSpeedFactor(2))
	assert.Equal(t, before, *configuration.GameplayConfiguration)
}

func TestNumericSettingSpecValidationAndIndexedSteps(t *testing.T) {
	for _, spec := range []NumericSettingSpec{
		{Min: 0, Max: 3, Step: 0, Default: 1}, {Min: 2, Max: 1, Step: 1, Default: 1},
		{Min: 0, Max: 3, Step: 2, Default: 0}, {Min: 0, Max: 3, Step: 1, Default: 4},
	} {
		require.Error(t, spec.Validate(1))
	}
	spec := NumericSettingSpec{Min: 0.1, Max: 0.4, Step: 0.1, Default: 0.2}
	require.NoError(t, spec.Validate(0.3))
	assert.InDelta(t, 0.3, spec.Advance(0.2, 1), 1e-12)
	assert.Equal(t, spec.Min, spec.Advance(spec.Default, -100))
	assert.InDelta(t, spec.Max, spec.Advance(spec.Default, 100), 1e-12)
}

func TestNumericSettingSpecValueListValidationAndSteps(t *testing.T) {
	for _, values := range [][]float64{{1, 2, 3}, {0, 2, 3}, {0, 3, 2}, {0, 1, 1, 3}, {0, 1.5, 3}, {0, 2, 3, 4}} {
		require.Error(t, NumericSettingSpec{Min: 0, Max: 3, Step: 1, Default: 1, Values: values}.Validate(0))
	}
	spec := CombatSpeedSpec()
	require.NoError(t, spec.Validate(12))
	require.Error(t, spec.Validate(10))
	require.NoError(t, spec.ValidateGrid(10))
	assert.Equal(t, []float64{0.5, 1, 1.5, 2, 3, 4, 6, 8, 12, 16, 20}, spec.Choices())
	assert.Equal(t, 6.0, spec.Advance(4, 1))
	assert.Equal(t, 3.0, spec.Advance(4, -1))
	assert.Equal(t, 20.0, spec.Advance(16, 5))
	assert.Equal(t, 0.5, spec.Advance(1, -5))
	for _, test := range []struct{ value, want float64 }{{0, 0.5}, {1.75, 2}, {10, 12}, {14, 16}, {99, 20}} {
		assert.Equal(t, test.want, spec.Nearest(test.value), "%g", test.value)
	}
}
