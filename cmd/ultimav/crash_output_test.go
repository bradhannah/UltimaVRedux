package main

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigureCrashOutputAtCreatesRestrictedCrashLog(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows does not expose Unix permission bits")
	}
	path := filepath.Join(t.TempDir(), "diagnostics", crashLogFilename)
	require.NoError(t, configureCrashOutputAt(path))
	t.Cleanup(func() {
		require.NoError(t, debug.SetCrashOutput(nil, debug.CrashOptions{}))
	})

	contents, err := os.ReadFile(path)
	require.NoError(t, err)
	assert.Contains(t, string(contents), "Ultima V Redux launch")
	assert.Contains(t, string(contents), "os="+runtime.GOOS)

	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, crashFilePermissions, info.Mode().Perm())
}

func TestRotateCrashLogPreservesMostRecentFullLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), crashLogFilename)
	require.NoError(t, os.WriteFile(path, []byte("previous crash"), crashFilePermissions))

	require.NoError(t, rotateCrashLog(path, int64(len("previous crash"))))

	_, err := os.Stat(path)
	assert.True(t, os.IsNotExist(err))
	oldContents, err := os.ReadFile(path + ".old")
	require.NoError(t, err)
	assert.Equal(t, "previous crash", strings.TrimSpace(string(oldContents)))
}

func TestSoundFailureRestartsSilentOnlyOnce(t *testing.T) {
	sound := errors.New("audio: audio error: oto: no device")
	assert.True(t, restartsWithoutSound(nil, sound))
	assert.False(t, restartsWithoutSound([]string{"--sound-unavailable"}, sound), "a silent start is not restarted")
	assert.False(t, restartsWithoutSound([]string{"--no-sound"}, sound))
	assert.False(t, restartsWithoutSound(nil, errors.New("display is unavailable")))
}

func TestLaunchFailureMessagePointsToNoSoundWhenSoundFails(t *testing.T) {
	sound := launchFailureMessage(errors.New("oto: ALSA device initialization failed: no device"))
	assert.Contains(t, sound, "--no-sound")

	other := launchFailureMessage(errors.New("display is unavailable"))
	assert.Equal(t, "display is unavailable", other)
}
