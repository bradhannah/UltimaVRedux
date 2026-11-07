package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func configurationFixture(t *testing.T, contents string) (*UltimaVConfiguration, string) {
	t.Helper()
	directory := t.TempDir()
	configFile := filepath.Join(directory, "config.yaml")
	require.NoError(t, os.WriteFile(configFile, []byte(contents), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(directory, "DATA.OVL"), nil, 0o600))
	configuration, err := LoadUltimaVConfiguration(configFile, directory)
	require.NoError(t, err)

	return configuration, configFile
}

func readConfigurationFixture(t *testing.T, file string) *viper.Viper {
	t.Helper()
	source := viper.New()
	source.SetConfigType("yaml")
	source.SetConfigFile(file)
	require.NoError(t, source.ReadInConfig())

	return source
}

func TestConfigurationSettingsPersistAtomicallyAndPreservePointers(t *testing.T) {
	configuration, file := configurationFixture(t, "unknown:\n  option: preserved\nDataFilePath: shipped/path\nDebug:\n  SurfaceMovementCurve: smoothstep\nLighting:\n  AmbientLightRadius: 3\n")
	audio, display, gameplay := configuration.AudioConfiguration, configuration.DisplayConfiguration, configuration.GameplayConfiguration
	flags, debug, logging := configuration.SavedConfigData, configuration.DebugConfiguration, configuration.LoggingConfiguration
	settings := ConfigurationSettings{
		Audio: AudioConfiguration{MusicVolume: 7, SoundEffectVolume: 8, AmbientVolume: 9},
		Display: DisplayConfiguration{
			Mode: ModeMaximized, WindowWidth: 1700, WindowHeight: 950, TargetFPS: 60, EnableFpsLimiting: true,
		},
		Gameplay: GameplayConfiguration{
			AllowRepeatedMonsterDivision: true, PersistHiddenDungeonDiscoveries: true,
			ShowSubduedUnlitDungeons: true, SmoothSurfaceMovement: true,
			ShowConcealedTurnOrderSlots: true, FlexibleSextantUse: true,
			CombatSpeedFactor: 1, ConfirmQuickSave: true, ShowCombatHPBars: true, MaxVisibleActionPopups: 3,
		},
	}
	require.NoError(t, configuration.SaveConfigurationSettings(settings))
	assert.Same(t, audio, configuration.AudioConfiguration)
	assert.Same(t, display, configuration.DisplayConfiguration)
	assert.Same(t, gameplay, configuration.GameplayConfiguration)
	assert.Same(t, flags, configuration.SavedConfigData)
	assert.Same(t, debug, configuration.DebugConfiguration)
	assert.Same(t, logging, configuration.LoggingConfiguration)
	assert.Equal(t, settings.Audio, *audio)
	assert.Equal(t, settings.Display, *display)
	assert.Equal(t, settings.Gameplay, *gameplay)
	loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
	require.NoError(t, err)
	assert.Equal(t, settings.Audio, *loaded.AudioConfiguration)
	assert.Equal(t, settings.Display, *loaded.DisplayConfiguration)
	assert.Equal(t, settings.Gameplay, *loaded.GameplayConfiguration)
	require.NoError(t, configuration.UpdateSaveFile())
	source := readConfigurationFixture(t, file)
	assert.Equal(t, "preserved", source.GetString("unknown.option"))
	assert.Equal(t, "shipped/path", source.GetString("DataFilePath"))
	assert.Equal(t, "smoothstep", source.GetString("Debug.SurfaceMovementCurve"))
	assert.Equal(t, 3, source.GetInt("Lighting.AmbientLightRadius"))
	info, err := os.Stat(file)
	require.NoError(t, err)
	if runtime.GOOS == "windows" {
		// Go reports writable Windows files as 0666; Unix owner/group modes do not apply.
		assert.Equal(t, os.FileMode(0o666), info.Mode().Perm())
	} else {
		assert.Equal(t, os.FileMode(0o600), info.Mode().Perm())
	}
}

func TestConfigurationInstancesAreIsolated(t *testing.T) {
	first, firstFile := configurationFixture(t, "Audio:\n  MusicVolume: 1\nunknown: first\n")
	second, secondFile := configurationFixture(t, "Audio:\n  MusicVolume: 9\nunknown: second\n")
	secondBytes, err := os.ReadFile(secondFile)
	require.NoError(t, err)
	first.AudioConfiguration.MusicVolume = 3
	require.NoError(t, first.SaveAudioConfiguration())
	unchanged, err := os.ReadFile(secondFile)
	require.NoError(t, err)
	assert.Equal(t, secondBytes, unchanged)
	assert.Equal(t, 9, second.AudioConfiguration.MusicVolume)
	second.DebugConfiguration.CombatDetailSet = true
	second.DebugConfiguration.CombatDetailShown = true
	require.NoError(t, second.SaveDebugConfiguration())
	assert.False(t, readConfigurationFixture(t, firstFile).IsSet("Debug.CombatDetailShown"))
	assert.Equal(t, "first", readConfigurationFixture(t, firstFile).GetString("unknown"))
	assert.Equal(t, "second", readConfigurationFixture(t, secondFile).GetString("unknown"))
}

func TestConfigurationSettingsFailurePreservesModelsSourceAndFiles(t *testing.T) {
	configuration, file := configurationFixture(t, "Audio:\n  MusicVolume: 1\n")
	audio, display, gameplay := configuration.AudioConfiguration, configuration.DisplayConfiguration, configuration.GameplayConfiguration
	beforeAudio, beforeDisplay, beforeGameplay := *audio, *display, *gameplay
	beforeSource := configuration.source.AllSettings()
	require.NoError(t, os.Remove(file))
	require.NoError(t, os.Mkdir(file, 0o700))
	sentinel := filepath.Join(file, "sentinel")
	require.NoError(t, os.WriteFile(sentinel, []byte("unchanged"), 0o600))
	err := configuration.SaveConfigurationSettings(ConfigurationSettings{
		Audio:    AudioConfiguration{MusicVolume: 7},
		Display:  DisplayConfiguration{Mode: ModeFullscreen, WindowWidth: 1920, WindowHeight: 1080},
		Gameplay: GameplayConfiguration{SmoothSurfaceMovement: true, CombatSpeedFactor: 1, MaxVisibleActionPopups: 3},
	})
	require.ErrorContains(t, err, "saving configuration settings")
	assert.Same(t, audio, configuration.AudioConfiguration)
	assert.Same(t, display, configuration.DisplayConfiguration)
	assert.Same(t, gameplay, configuration.GameplayConfiguration)
	assert.Equal(t, beforeAudio, *audio)
	assert.Equal(t, beforeDisplay, *display)
	assert.Equal(t, beforeGameplay, *gameplay)
	assert.Equal(t, beforeSource, configuration.source.AllSettings())
	bytes, err := os.ReadFile(sentinel)
	require.NoError(t, err)
	assert.Equal(t, "unchanged", string(bytes))
	entries, err := os.ReadDir(filepath.Dir(file))
	require.NoError(t, err)
	assert.Len(t, entries, 2, "failed atomic write must clean up temporary files")
}

func TestConfigurationSectionSavesAreAtomic(t *testing.T) {
	for name, save := range map[string]func(*UltimaVConfiguration) error{
		"audio":    (*UltimaVConfiguration).SaveAudioConfiguration,
		"debug":    (*UltimaVConfiguration).SaveDebugConfiguration,
		"logging":  (*UltimaVConfiguration).SaveLoggingConfiguration,
		"gameplay": (*UltimaVConfiguration).SaveGameplayConfiguration,
		"all":      (*UltimaVConfiguration).UpdateSaveFile,
	} {
		t.Run(name, func(t *testing.T) {
			configuration, file := configurationFixture(t, "existing: true\n")
			originalAudio := *configuration.AudioConfiguration
			originalDebug := *configuration.DebugConfiguration
			originalGameplay := *configuration.GameplayConfiguration
			configuration.AudioConfiguration.MusicVolume = 3
			configuration.DebugConfiguration.CombatDetailSet = true
			configuration.DebugConfiguration.CombatDetailShown = false
			configuration.LoggingConfiguration.CategoryStates["combat"] = true
			configuration.LoggingConfiguration.CategoryStates["audio"] = false
			configuration.GameplayConfiguration.SmoothSurfaceMovement = true
			require.NoError(t, save(configuration))
			loaded, err := LoadUltimaVConfiguration(file, filepath.Dir(file))
			require.NoError(t, err)
			if name == "audio" || name == "all" {
				assert.Equal(t, configuration.AudioConfiguration, loaded.AudioConfiguration)
			} else {
				assert.Equal(t, originalAudio, *loaded.AudioConfiguration)
			}
			if name == "debug" || name == "all" {
				assert.Equal(t, configuration.DebugConfiguration, loaded.DebugConfiguration)
			} else {
				assert.Equal(t, originalDebug, *loaded.DebugConfiguration)
			}
			if name == "logging" || name == "all" {
				assert.Equal(t, map[string]bool{"combat": true, "audio": false}, loaded.LoggingConfiguration.CategoryStates)
			} else {
				assert.Empty(t, loaded.LoggingConfiguration.CategoryStates)
			}
			if name == "gameplay" || name == "all" {
				assert.Equal(t, configuration.GameplayConfiguration, loaded.GameplayConfiguration)
			} else {
				assert.Equal(t, originalGameplay, *loaded.GameplayConfiguration)
			}
			before := configuration.source.AllSettings()
			require.NoError(t, os.Remove(file))
			require.NoError(t, os.Mkdir(file, 0o700))
			configuration.AudioConfiguration.MusicVolume = 8
			require.Error(t, save(configuration))
			assert.Equal(t, before, configuration.source.AllSettings())
		})
	}
}

func TestGameplayPreferencesDefaultTrueOnlyWhenAbsent(t *testing.T) {
	configuration, _ := configurationFixture(t, "{}\n")
	assert.True(t, configuration.GameplayConfiguration.PersistHiddenDungeonDiscoveries)
	assert.True(t, configuration.GameplayConfiguration.ShowSubduedUnlitDungeons)
	assert.True(t, configuration.GameplayConfiguration.ShowConcealedTurnOrderSlots)
	assert.False(t, configuration.GameplayConfiguration.AllowRepeatedMonsterDivision)
	for _, key := range []string{
		gameplayPersistHiddenDungeonDiscoveriesKey,
		gameplayShowSubduedUnlitDungeonsKey,
		gameplayShowConcealedTurnOrderSlotsKey,
	} {
		configuration.source.Set(key, false)
		value := true
		configuration.configuredBoolean(&value, key)
		assert.False(t, value)
	}
}

func TestEmptyConfigurationLoadsTheGameplayDefaults(t *testing.T) {
	configuration, _ := configurationFixture(t, "{}\n")
	assert.Equal(t, DefaultGameplayConfiguration(), configuration.GameplayConfiguration)
}

func TestAudioConfigurationMethods(t *testing.T) {
	audio := &AudioConfiguration{}
	audio.SetMusicVolume(6)
	audio.SetSoundEffectVolume(20)
	audio.SetAmbientVolume(-1)
	assert.Equal(t, AudioConfiguration{MusicVolume: 6, SoundEffectVolume: 10}, *audio)
	clone := audio.Clone()
	assert.Equal(t, audio, clone)
	assert.NotSame(t, audio, clone)
	clone.SetMusicVolume(1)
	assert.Equal(t, 6, audio.MusicVolume)
}
