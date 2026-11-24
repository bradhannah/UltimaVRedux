package audio

import "github.com/bradhannah/UltimaVRedux/internal/references"

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// GetCurrentTrack returns the name of the currently playing track
func (mm *MusicManager) GetCurrentTrack() string {
	mm.mutex.RLock()
	defer mm.mutex.RUnlock()

	return mm.policy.track
}

// GetCurrentLocation returns the location for which music is currently playing
func (mm *MusicManager) GetCurrentLocation() references.Location {
	mm.mutex.RLock()
	defer mm.mutex.RUnlock()

	return mm.currentLocation
}
