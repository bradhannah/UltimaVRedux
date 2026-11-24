package config

import (
	"fmt"

	"github.com/spf13/viper"
)

// SetCombatMovementSpeedFactor updates the inert legacy snapshot without persistence.
//
// Deprecated: use UltimaVConfiguration.SetCombatSpeedFactor for player preferences.
func (d *DebugConfiguration) SetCombatMovementSpeedFactor(factor float64) bool {
	if d == nil || !validCombatMovementSpeedFactor(factor) {
		return false
	}

	d.CombatMovementSpeedFactor = factor

	return true
}

// Clone creates a deep copy of the AudioConfiguration
func (a *AudioConfiguration) Clone() *AudioConfiguration {
	return &AudioConfiguration{
		MusicVolume:       a.MusicVolume,
		SoundEffectVolume: a.SoundEffectVolume,
		AmbientVolume:     a.AmbientVolume,
	}
}

// SaveAudioConfiguration immediately persists current audio settings to config file
func (uc *UltimaVConfiguration) SaveAudioConfiguration() error {
	if uc.AudioConfiguration == nil {
		return ErrAudioConfigNil
	}

	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		setAudioConfiguration(candidate, *uc.AudioConfiguration)
	}); err != nil {
		return fmt.Errorf("saving audio configuration: %w", err)
	}

	return nil
}

// SaveGameplayConfiguration immediately persists current gameplay settings.
func (uc *UltimaVConfiguration) SaveGameplayConfiguration() error {
	if uc.ConfigurationEditing() {
		return ErrConfigurationEditing
	}
	if uc.GameplayConfiguration == nil {
		return ErrGameplayConfigNil
	}
	if err := uc.saveConfiguration(func(candidate *viper.Viper) {
		setGameplayConfiguration(candidate, *uc.GameplayConfiguration)
	}); err != nil {
		return fmt.Errorf("saving gameplay configuration: %w", err)
	}

	return nil
}
