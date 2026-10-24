package audio

import (
	"errors"

	"github.com/bradhannah/UltimaVRedux/internal/references"
	berad "github.com/bradhannah/berad-sdk/modules/berad-audio"
)

// ambientPreviewCue is the loop the settings screen plays to audition ambience volume.
const ambientPreviewCue berad.CueID = "ambient/AmbientForest"

// SetSoundEffectVolume updates live SFX and game preference state together.
func (manager *GameAudioManager) SetSoundEffectVolume(volume float64) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed {
		return ErrAudioClosed
	}
	if err := manager.sfx.SetVolume(volume); err != nil {
		return gameAudioError("set SFX volume", err)
	}

	return manager.config.SetSoundEffectVolume(volume)
}

// SetMusicVolume retains Redux's muted-policy hold independently of transport gain.
func (manager *GameAudioManager) SetMusicVolume(volume float64) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed {
		return ErrAudioClosed
	}
	if err := manager.music.SetVolume(volume); err != nil {
		return gameAudioError("set music volume", err)
	}
	manager.policy.SetSuppressed(musicMuted, volume == 0)

	return manager.config.SetMusicVolume(volume)
}

// SetAmbientVolume updates retained ambient and preview voices without restarting.
func (manager *GameAudioManager) SetAmbientVolume(volume float64) error {
	manager.mutex.Lock()
	defer manager.mutex.Unlock()
	if manager.closed {
		return ErrAudioClosed
	}
	if err := manager.config.SetAmbientVolume(volume); err != nil {
		return err
	}

	return errors.Join(manager.ambience.SetVolume(volume), manager.preview.SetVolume(volume))
}

// StopBackgroundMusic clears transport without discarding Redux policy/history.
func (manager *GameAudioManager) StopBackgroundMusic() error {
	return gameAudioError("stop music", manager.music.Stop())
}

// PlayMusicPreview borrows the music lane while the enclosing menu owns its hold.
func (manager *GameAudioManager) PlayMusicPreview(track string) error {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	if manager.closed {
		return ErrAudioClosed
	}
	if !manager.enabled {
		return nil
	}
	if err := manager.music.SetGain(1); err != nil {
		return gameAudioError("set preview gain", err)
	}

	return gameAudioError("play music preview", manager.music.Play(berad.CueID("music/track/"+track)))
}

// PlayAmbientSoundPreview uses the game-selected forest cue and preview-only lifetime.
func (manager *GameAudioManager) PlayAmbientSoundPreview() error {
	manager.mutex.RLock()
	defer manager.mutex.RUnlock()
	if manager.closed {
		return ErrAudioClosed
	}
	if !manager.enabled {
		return nil
	}
	options := berad.DefaultPlayOptions()
	options.Loop, options.Slot = true, "settings"
	result, err := manager.previewOwner.Play(ambientPreviewCue, options)
	if err != nil {
		return gameAudioError("play ambient preview", err)
	}
	if result.Outcome == berad.Empty {
		return gameAudioError("stop empty preview", manager.preview.StopCue(ambientPreviewCue, 0))
	}

	return nil
}

// StopAmbientSoundPreview only stops the preview lane.
func (manager *GameAudioManager) StopAmbientSoundPreview() error {
	return gameAudioError("stop ambient preview", manager.preview.StopCue(ambientPreviewCue, 0))
}

// GetMusicManager exposes Redux's existing game-specific policy controller.
func (manager *GameAudioManager) GetMusicManager() *MusicManager { return manager.policy }

// PlayMusicForLocation resolves game location policy.
func (manager *GameAudioManager) PlayMusicForLocation(location references.Location) error {
	return manager.policy.PlayMusicForLocation(location)
}

// PlayMusicForContext resolves the game-owned context mapping.
func (manager *GameAudioManager) PlayMusicForContext(context string) error {
	return manager.policy.PlayMusicForContext(context)
}

// PauseBackgroundMusic acquires a nested workflow hold.
func (manager *GameAudioManager) PauseBackgroundMusic() { manager.policy.PauseBackgroundMusic() }

// ResumeBackgroundMusic releases only the matching workflow nesting depth.
func (manager *GameAudioManager) ResumeBackgroundMusic() { manager.policy.ResumeBackgroundMusic() }
