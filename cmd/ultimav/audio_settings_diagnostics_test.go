package main

import (
	"strings"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/audio"
	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/sprites"
)

type diagnosticsAudioStub struct {
	audioSettingsManagerStub
	captures int
	name     string
}

func (stub *diagnosticsAudioStub) PlaybackSnapshot() audio.PlaybackSnapshot {
	stub.captures++
	voices := make([]audio.PlaybackVoice, 26)
	for index := range voices {
		voices[index] = audio.PlaybackVoice{CueID: stub.name, RecordingID: strings.Repeat("long-source-", 10), State: "playing"}
	}
	return audio.PlaybackSnapshot{Enabled: true, Voices: voices}
}

func TestSoundDiagnosticsOwnsInputAndCapturesOnlyOnRequest(t *testing.T) {
	provider := &diagnosticsAudioStub{name: "first cue"}
	dialog := newConfigurationDialogForTab(&sprites.PercentBasedPlacement{StartPercentX: 0.1, EndPercentX: 0.9, StartPercentY: 0.1, EndPercentY: 0.9},
		newPresentationTestKeyboard(), func() {}, dialogConfiguration(&config.AudioConfiguration{}), provider, audioSettingsAudioTab)
	dialog.ultimaConfig.DeveloperTools = true
	staged := dialog.stagedAudio
	pressAudioSettingsKey(dialog, ebiten.KeyD)
	require.NotNil(t, dialog.audioReport)
	require.Equal(t, 1, provider.captures)
	for _, size := range [][2]int{{640, 360}, {1920, 1080}} {
		screen := newPresentationRaster(size[0], size[1])
		dialog.Draw(screen)
		dialog.Draw(screen)
		require.Equal(t, 1, provider.captures, "rendering cannot query or service audio")
		pressAudioSettingsKey(dialog, ebiten.KeyEnd)
		require.Positive(t, dialog.audioReport.offset)
		screen.Deallocate()
	}
	pressAudioSettingsKey(dialog, ebiten.KeyRight)
	pressAudioSettingsKey(dialog, ebiten.KeyEnter)
	require.Equal(t, staged, dialog.stagedAudio)
	require.False(t, dialog.dismissed)
	provider.name = "second cue"
	pressAudioSettingsKey(dialog, ebiten.KeyR)
	require.Equal(t, 2, provider.captures)
	require.Contains(t, strings.Join(dialog.audioReport.lines, "\n"), "second cue")
	pressAudioSettingsKey(dialog, ebiten.KeyEscape)
	require.Nil(t, dialog.audioReport)
	require.False(t, dialog.dismissed, "Escape leaves diagnostics before it can cancel Configuration")
}
