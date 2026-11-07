package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSurfaceCameraDebugConfiguration(t *testing.T) {
	c, _ := configurationFixture(t, "{}\n")
	c.source.Set("Debug.SurfaceMovementDurationMs", 150)
	c.source.Set("Debug.SurfaceMovementCurve", "smoothstep")
	c.loadDebugConfiguration()
	assert.Equal(t, 150, c.DebugConfiguration.SurfaceMovementDurationMs)
	assert.Equal(t, "smoothstep", c.DebugConfiguration.SurfaceMovementCurve)
}

func TestSmoothSurfaceMovementLoadsOptionalGameplayPreference(t *testing.T) {
	directory := t.TempDir()
	configPath := filepath.Join(directory, "config.yaml")
	require.NoError(t, os.WriteFile(filepath.Join(directory, "DATA.OVL"), nil, 0o600))
	require.NoError(t, os.WriteFile(configPath, []byte("{}\n"), 0o600))
	configuration, err := LoadUltimaVConfiguration(configPath, directory)
	require.NoError(t, err)
	assert.False(t, configuration.GameplayConfiguration.SmoothSurfaceMovement)
	gameplay := configuration.GameplayConfiguration
	for _, enabled := range []bool{true, false} {
		require.NoError(t, configuration.saveGameplayPreference(func(settings *GameplayConfiguration) { settings.SmoothSurfaceMovement = enabled }))
		assert.Same(t, gameplay, configuration.GameplayConfiguration)
		assert.Equal(t, enabled, gameplay.SmoothSurfaceMovement)
		loaded, loadErr := LoadUltimaVConfiguration(configPath, directory)
		require.NoError(t, loadErr)
		assert.Equal(t, enabled, loaded.GameplayConfiguration.SmoothSurfaceMovement)
	}
}
