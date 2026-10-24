package audio

import (
	"errors"
	"fmt"
	"sync"
	"time"

	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
	"github.com/bradhannah/berad-sdk/modules/berad-audio/backend/ebitengine"
	ea "github.com/hajimehoshi/ebiten/v2/audio"

	"github.com/bradhannah/UltimaVRedux/internal/config"
	"github.com/bradhannah/UltimaVRedux/internal/game_state"
	"github.com/bradhannah/UltimaVRedux/internal/logging"
)

// GameAudioManager is Redux's live adapter over Berad Audio. It owns JSON/world
// interpretation and music policy, never a second playback/fade/admission engine.
// Required gameplay callbacks remain game-owned and presentation-only.
type GameAudioManager struct {
	mutex                              sync.RWMutex
	library                            *Library
	playback                           *berad.Library
	config                             *AudioConfiguration
	clock                              *presentationClock
	sfx                                *berad.Runtime
	legacyOwner                        *berad.Owner
	music                              *berad.MusicPlayer
	ambience                           *berad.AmbientController
	preview                            *berad.Runtime
	previewOwner                       *berad.Owner
	planner                            *AmbientPlanner
	policy                             *MusicManager
	effects                            map[string]game_state.SoundEffect
	roles                              map[berad.SourceID]string
	sourceMix                          map[berad.SourceID]AmbientSourcePlan
	steps                              uint64
	enabled, closed, ambientSuppressed bool
}

// ErrAudioClosed reports an operation on a terminally closed game adapter.
var ErrAudioClosed = errors.New("audio manager is closed")

// NewGameAudioManagerWithConfig translates persisted sliders on the game side.
func NewGameAudioManagerWithConfig(preferences *config.AudioConfiguration) (*GameAudioManager, error) {
	configuration, err := NewAudioConfiguration()
	if err != nil {
		return nil, err
	}
	if preferences != nil {
		configuration.SoundEffectVolume = ConvertSliderToVolume(preferences.SoundEffectVolume)
		configuration.MusicVolume = ConvertSliderToVolume(preferences.MusicVolume)
		configuration.AmbientVolume = ConvertSliderToVolume(preferences.AmbientVolume)
	}

	return NewGameAudioManagerWithRuntime(configuration, getSharedAudioContext(), time.Now)
}

// NewGameAudioManagerWithRuntime borrows an explicit Ebitengine context/clock.
func NewGameAudioManagerWithRuntime(configuration *AudioConfiguration, context *ea.Context,
	now func() time.Time,
) (*GameAudioManager, error) {
	return newGameAudioManager(defaultRuntimeLibrary(), configuration,
		ebitengine.Backend{Context: context}, ebitengine.Decode, now)
}

func newGameAudioManager(library *Library, configuration *AudioConfiguration, backend berad.Backend,
	decode berad.Decoder, now func() time.Time,
) (*GameAudioManager, error) {
	if library == nil || configuration == nil || now == nil {
		return nil, fmt.Errorf("%w: library, configuration and clock required", ErrAudioLibrary)
	}
	playback, err := library.PlaybackLibrary()
	if err != nil {
		return nil, err
	}
	planner, err := NewAmbientPlanner(library)
	if err != nil {
		return nil, err
	}
	manager := new(GameAudioManager)
	manager.library, manager.playback, manager.planner = library, playback, planner
	manager.config, manager.clock = configuration.Clone(), newPresentationClock(now)
	manager.effects, manager.roles = make(map[string]game_state.SoundEffect), make(map[berad.SourceID]string)
	manager.sourceMix = make(map[berad.SourceID]AmbientSourcePlan)
	for effect, symbol := range catalogEffectNames() {
		manager.effects["sfx/"+symbol] = effect
	}
	if err := manager.initializePlayback(backend, decode); err != nil {
		return nil, err
	}
	manager.enabled = true
	manager.policy = newMusicManager(manager, gameMusicTransport{music: manager.music}, manager.clock.Now)
	manager.policy.SetSuppressed(musicMuted, manager.config.MusicVolume == 0)

	return manager, nil
}

func (manager *GameAudioManager) initializePlayback(backend berad.Backend, decode berad.Decoder) error {
	var err error
	options := berad.DefaultOptions(backend, decode)
	options.Now, options.Volume = manager.clock.Now, manager.config.SoundEffectVolume
	manager.sfx, err = berad.NewRuntime(manager.playback, options)
	if err != nil {
		return fmt.Errorf("construct game SFX: %w", err)
	}
	manager.legacyOwner = manager.sfx.NewOwner()
	options.Volume = manager.config.MusicVolume
	manager.music, err = berad.NewMusicPlayer(manager.playback, options)
	if err != nil {
		return errors.Join(err, manager.sfx.Close())
	}
	ambientOptions := berad.DefaultAmbientOptions(backend, decode)
	ambientOptions.Playback.VoiceLimit = AmbientVoiceLimit
	ambientOptions.Playback.Now, ambientOptions.Playback.Volume = manager.clock.Now, manager.config.AmbientVolume
	manager.ambience, err = berad.NewAmbientController(manager.playback, ambientOptions)
	if err != nil {
		return errors.Join(err, manager.sfx.Close(), manager.music.Close())
	}
	options.Volume, options.VoiceLimit = manager.config.AmbientVolume, 1
	manager.preview, err = berad.NewRuntime(manager.playback, options)
	if err != nil {
		return errors.Join(err, manager.sfx.Close(), manager.music.Close(), manager.ambience.Close())
	}
	manager.previewOwner = manager.preview.NewOwner()

	return nil
}

func (manager *GameAudioManager) musicLibrary() *Library { return manager.library }
func (manager *GameAudioManager) musicReadLock()         { manager.mutex.RLock() }
func (manager *GameAudioManager) musicReadUnlock()       { manager.mutex.RUnlock() }
func (manager *GameAudioManager) ensureAssetsLoaded() error {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	if manager.closed {
		return ErrAudioClosed
	}

	return nil // Encoded library already frozen; each module lane prepares lazily.
}

// SetEnabled clears effects/ambience/previews and pauses score when disabled.
func (manager *GameAudioManager) SetEnabled(enabled bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed {
		return
	}
	manager.enabled = enabled
	manager.policy.SetSuppressed(musicSystem, !enabled)
	if !enabled {
		manager.music.Pause()
		for _, voice := range manager.sfx.Snapshot() {
			manager.report(manager.sfx.StopCue(voice.Cue, 0))
		}
		manager.report(manager.preview.StopCue(ambientPreviewCue, 0))
	}
	manager.report(manager.ambience.SetEnabled(enabled && !manager.ambientSuppressed))
}

// SetAmbientSuppressed clears proximity playback until a fresh world snapshot.
func (manager *GameAudioManager) SetAmbientSuppressed(suppressed bool) {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed {
		return
	}
	manager.ambientSuppressed = suppressed
	manager.report(manager.ambience.SetEnabled(manager.enabled && !suppressed))
}

// Update services all lanes, then advances Redux's score policy outside the facade lock.
func (manager *GameAudioManager) Update() {
	manager.mutex.Lock()
	if manager.closed {
		manager.mutex.Unlock()

		return
	}
	manager.report(manager.sfx.Update())
	manager.report(manager.preview.Update())
	manager.report(manager.ambience.Update())
	if manager.hasLocalMusic() {
		manager.policy.LocalMusic()
	}
	manager.mutex.Unlock()
	manager.policy.Update()
}

// Close retires every owned lane, preserving all errors and the borrowed device.
func (manager *GameAudioManager) Close() error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed {
		return nil
	}
	manager.policy.SetSuppressed(musicSystem, true)
	manager.closed, manager.enabled = true, false
	clear(manager.roles)
	clear(manager.sourceMix)

	return errors.Join(manager.sfx.Close(), manager.music.Close(), manager.ambience.Close(), manager.preview.Close())
}

func (*GameAudioManager) report(err error) {
	if err != nil {
		logging.Warnf(logging.CategoryAudio, "Game audio: %v", err)
	}
}

type gameMusicTransport struct{ music *berad.MusicPlayer }

func (transport gameMusicTransport) Update() error {
	return gameAudioError("service music", transport.music.Update())
}
func (transport gameMusicTransport) Play(track string) error {
	return gameAudioError("play music", transport.music.Play(berad.CueID("music/track/"+track)))
}
func (transport gameMusicTransport) Stop() error {
	return gameAudioError("stop music", transport.music.Stop())
}
func (transport gameMusicTransport) IsPlaying() bool      { return transport.music.IsPlaying() }
func (transport gameMusicTransport) Pause() time.Duration { return transport.music.Pause() }
func (transport gameMusicTransport) Resume(track string, position time.Duration) error {
	return gameAudioError("resume music", transport.music.Resume(berad.CueID("music/track/"+track), position))
}
func (transport gameMusicTransport) SetVolume(gain float64) error {
	return gameAudioError("set music gain", transport.music.SetGain(gain))
}

func gameAudioError(operation string, err error) error {
	if err != nil {
		return fmt.Errorf("game audio %s: %w", operation, err)
	}

	return nil
}
