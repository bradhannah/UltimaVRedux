package audio

import (
	"encoding/json"
	"errors"
	"io"
	"math"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	"github.com/stretchr/testify/require"

	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/references"
)

type gamePlaybackBackend struct {
	mutex   sync.Mutex
	players []*conformanceDevice
	fail    error
}

func (*gamePlaybackBackend) SampleRate() int { return EditorSampleRate }
func (backend *gamePlaybackBackend) NewPlayer(source io.ReadSeeker) (berad.Transport, error) {
	backend.mutex.Lock()
	defer backend.mutex.Unlock()
	if backend.fail != nil {
		return nil, backend.fail
	}
	device := &conformanceDevice{source: source, barrierEntered: make(chan struct{})}
	backend.players = append(backend.players, device)
	return device, nil
}

func gameAudioFixture(t *testing.T) (*GameAudioManager, *gamePlaybackBackend, *time.Time) {
	t.Helper()
	return gameAudioDocumentFixture(t, func(*LibraryDocument) {})
}

func gameAudioDocumentFixture(t *testing.T, edit func(*LibraryDocument)) (*GameAudioManager, *gamePlaybackBackend, *time.Time) {
	t.Helper()
	document := *cloneAudioValue(productionLibrary())
	edit(&document)
	file := "sounds/synthetic.wav"
	for identity, recording := range document.Recordings {
		if recording.File != nil || identity == "tavern_lute_01" {
			recording.File = &file
		}
		recording.CalibrationGainDB, recording.PlaybackWindow, recording.CalibrationTargetDB = nil, nil, nil
		recording.CalibrationPolicy = ""
		document.Recordings[identity] = recording
	}
	encoded, err := json.Marshal(document)
	require.NoError(t, err)
	library, err := NewLibrary(encoded, fstest.MapFS{file: {Data: editorTestWAV()}}, LibraryOptions{Validate: ReduxLibraryValidator})
	require.NoError(t, err)
	return gameAudioLibraryFixture(t, library)
}

func gameAudioLibraryFixture(t *testing.T, library *Library) (*GameAudioManager, *gamePlaybackBackend, *time.Time) {
	t.Helper()
	configuration, err := NewAudioConfiguration()
	require.NoError(t, err)
	configuration.SoundEffectVolume, configuration.MusicVolume, configuration.AmbientVolume = 1, 1, 1
	now := time.Unix(100, 0)
	backend := new(gamePlaybackBackend)
	manager, err := newGameAudioManager(library, configuration, backend, ebitengine.Decode, func() time.Time { return now })
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, manager.Close()) })
	return manager, backend, &now
}

func TestGameAudioUsesModuleOwnershipAndLegacyEffectStops(t *testing.T) {
	manager, _, _ := gameAudioFixture(t)
	request := game_state.SoundPlaybackRequest{Effect: game_state.SoundEarthquake, Mode: game_state.SoundPlaybackLoop, Volume: 1}
	first, err := manager.PlayVoice(request)
	require.NoError(t, err)
	second, err := manager.PlayVoice(request)
	require.NoError(t, err)
	require.Len(t, manager.sfx.Snapshot(), 2)
	first.Stop(0)
	require.Len(t, manager.sfx.Snapshot(), 1)
	require.NoError(t, manager.PlaySound(request))
	manager.StopSound(game_state.SoundStopRequest{Effect: request.Effect})
	require.Empty(t, manager.sfx.Snapshot())
	second.Stop(0)
	request.Effect = game_state.SoundFireBurn
	request.Mode = game_state.SoundPlaybackOneShot
	require.NoError(t, manager.PlaySound(request))
	require.NoError(t, manager.PlaySound(request))
	require.Len(t, manager.sfx.Snapshot(), 1, "legacy burn replacement remains cue-wide within its lifetime")
	_, err = manager.PlayVoice(request)
	require.NoError(t, err)
	require.Len(t, manager.sfx.Snapshot(), 2, "owned burn must not replace the unrelated legacy voice")
}

func TestGameAudioLivePreferencesPreviewDisableAndClose(t *testing.T) {
	manager, _, now := gameAudioFixture(t)
	require.NoError(t, manager.PlaySoundEffect(game_state.SoundHeal))
	require.NoError(t, manager.PlayAmbientSoundPreview())
	require.NoError(t, manager.PlayMusicForContext("title"))
	*now = now.Add(musicFadeDuration)
	manager.Update()
	require.True(t, manager.music.IsPlaying())
	require.NoError(t, manager.SetAmbientVolume(0.5))
	require.InDelta(t, manager.library.document.Cues[string(ambientPreviewCue)].VolumeModifier*0.5,
		manager.preview.Snapshot()[0].Gain, 1e-9)
	configuration := manager.GetConfiguration()
	configuration.MusicVolume = 0
	require.Equal(t, 1.0, manager.GetConfiguration().MusicVolume)
	manager.SetEnabled(false)
	require.Empty(t, manager.sfx.Snapshot())
	require.Empty(t, manager.preview.Snapshot())
	require.Equal(t, berad.VoicePaused, manager.music.Snapshot().Voice.State)
	require.NoError(t, manager.PlayAmbientSoundPreview())
	require.Empty(t, manager.preview.Snapshot())
	manager.SetEnabled(true)
	manager.Update()
	require.True(t, manager.music.IsPlaying())
	require.NoError(t, manager.Close())
	manager.SetEnabled(true)
	require.False(t, manager.PlaybackSnapshot().Enabled)
	require.ErrorIs(t, manager.PlaySoundEffect(game_state.SoundHeal), ErrAudioClosed)
	require.Empty(t, manager.PlaybackSnapshot().Voices)
}

func TestGameAudioMusicPolicyRestoresPreviewPositionAndKeepsQuietHistory(t *testing.T) {
	manager, backend, now := gameAudioFixture(t)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	*now = now.Add(musicFadeDuration)
	manager.Update()
	require.Equal(t, "town", manager.policy.GetCurrentTrack())
	backend.players[0].position = 20 * EditorSampleRate * decodedPCMBytesPerFrame / 1000
	manager.PauseBackgroundMusic()
	manager.policy.SetSuppressed(MusicMenu, true)
	require.NoError(t, manager.PlayMusicPreview("town"))
	manager.ResumeBackgroundMusic()
	manager.Update()
	require.Len(t, backend.players, 2, "menu still owns interruption")
	manager.policy.SetSuppressed(MusicMenu, false)
	manager.Update()
	require.Len(t, backend.players, 3)
	require.Equal(t, 20*time.Millisecond, manager.music.Snapshot().Position)
	*now = now.Add(musicFadeDuration)
	manager.Update()
	backend.players[2].playing.Store(false)
	manager.Update()
	require.Empty(t, manager.policy.GetCurrentTrack())
	*now = now.Add(musicQuietInterval - time.Millisecond)
	manager.Update()
	require.Len(t, backend.players, 3)
	*now = now.Add(time.Millisecond)
	manager.Update()
	require.Len(t, backend.players, 4)
	require.Equal(t, "town", manager.policy.GetCurrentTrack())
	require.InDelta(t, musicRepeatDB, 20*math.Log10(manager.policy.policy.gain), 1e-9)
}

func TestGameAudioSailingCombatVictoryPreserveGamePolicy(t *testing.T) {
	manager, backend, now := gameAudioFixture(t)
	location := references.PlayerLocation{Location: references.BritanniaUnderworld, Floor: 0}
	manager.policy.SetSceneSoundscape(location, 0, true, false)
	manager.Update()
	require.Equal(t, "hornpipe", manager.policy.GetCurrentTrack())
	*now = now.Add(musicFadeDuration)
	manager.Update()
	manager.policy.SetSceneSoundscape(location, 1, true, false)
	*now = now.Add(musicFadeDuration)
	manager.Update()
	manager.Update()
	require.Equal(t, "combat", manager.policy.GetCurrentTrack())
	count := len(backend.players)
	manager.policy.SetSceneSoundscape(location, 1, true, true)
	manager.Update()
	require.Empty(t, manager.policy.GetCurrentTrack())
	manager.Update()
	require.Len(t, backend.players, count)
	manager.policy.SetSceneSoundscape(location, 0, true, false)
	manager.Update()
	require.Empty(t, manager.policy.GetCurrentTrack(), "same hoist must not replay after combat")
	manager.policy.SetSceneSoundscape(location, 0, false, false)
	manager.policy.SetSceneSoundscape(location, 0, true, false)
	manager.Update()
	require.Equal(t, "hornpipe", manager.policy.GetCurrentTrack())
}

func TestGameAudioBardPrecedenceIncludesInitialEnvelopeAndTail(t *testing.T) {
	manager, _, now := gameAudioFixture(t)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	source := manager.EmitterAmbientSource("bard", references.Position{}, 1)
	source.ID = "stable-bard"
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, []AmbientSoundSource{source})
	manager.Update()
	require.Equal(t, berad.VoicePaused, manager.music.Snapshot().Voice.State)
	var bard PlaybackVoice
	for _, entry := range manager.PlaybackSnapshot().Voices {
		if entry.Lane == "ambient" {
			bard = entry
		}
	}
	require.Equal(t, 1.0, bard.Gain.Modifier)
	require.Equal(t, 1.0, bard.Gain.Distance)
	require.Zero(t, bard.Gain.Envelope, "the initial envelope is distinct from an audible target mix")
	*now = now.Add(time.Second)
	manager.Update()
	manager.UpdateAmbientSoundsWithDynamicSources(references.Position{}, nil, nil)
	*now = now.Add(500 * time.Millisecond)
	manager.Update()
	require.Equal(t, berad.VoicePaused, manager.music.Snapshot().Voice.State)
	*now = now.Add(500 * time.Millisecond)
	manager.Update()
	require.Empty(t, manager.roles)
	*now = now.Add(musicLocalRelease)
	manager.Update()
	manager.Update()
	require.True(t, manager.music.IsPlaying())
}

func TestGameAudioFailedMusicPreviewPreservesCurrentTransport(t *testing.T) {
	manager, backend, _ := gameAudioFixture(t)
	require.NoError(t, manager.PlayMusicForLocation(references.Britain))
	broken := errors.New("device rejected player")
	backend.fail = broken
	require.ErrorIs(t, manager.PlayMusicPreview("combat"), broken)
	require.Equal(t, berad.CueID("music/track/town"), manager.music.Snapshot().Voice.Cue)
	require.True(t, manager.music.IsPlaying())
}
