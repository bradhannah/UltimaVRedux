package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFlexibleSextantUseDefaultsAndPersistence(t *testing.T) {
	for _, contents := range []string{"{}\n", "Gameplay:\n  Navigation:\n    FlexibleSextantUse: false\n"} {
		configuration, file := configurationFixture(t, contents)
		original, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.False(t, configuration.GameplayConfiguration.FlexibleSextantUse)
		unchanged, err := os.ReadFile(file)
		require.NoError(t, err)
		assert.Equal(t, original, unchanged, "loading a legacy preference must not rewrite it")
		gameplay := configuration.GameplayConfiguration
		for _, enabled := range []bool{true, false} {
			require.NoError(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.FlexibleSextantUse = enabled }))
			assert.Same(t, gameplay, configuration.GameplayConfiguration)
			loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
			require.NoError(t, err)
			assert.Equal(t, enabled, loaded.GameplayConfiguration.FlexibleSextantUse)
		}
	}
	assert.ErrorIs(t, (&UltimaVConfiguration{}).saveGameplayPreference(func(settings *GameplayConfiguration) { settings.FlexibleSextantUse = true }), ErrGameplayConfigNil)
}

func TestFlexibleSextantUseSurvivesNormalSettingsSave(t *testing.T) {
	configuration, file := configurationFixture(t, "unknown:\n  option: preserved\nGameplay:\n  Navigation:\n    FlexibleSextantUse: true\n")
	settings := ConfigurationSettings{
		Audio:    *configuration.AudioConfiguration,
		Display:  *configuration.DisplayConfiguration,
		Gameplay: *configuration.GameplayConfiguration,
	}
	settings.Audio.MusicVolume = 7
	require.NoError(t, configuration.SaveConfigurationSettings(settings))
	loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
	require.NoError(t, err)
	assert.True(t, loaded.GameplayConfiguration.FlexibleSextantUse)
	assert.Equal(t, 7, loaded.AudioConfiguration.MusicVolume)
	assert.Equal(t, "preserved", readConfigurationFixture(t, file).GetString("unknown.option"))
}
