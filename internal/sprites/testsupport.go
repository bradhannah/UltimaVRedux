package sprites

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// ClearBorderCache retires shared frame references between independent native
// scene fixtures. Production scenes share the application-lifetime cache.
// Live borrowers remain valid; no shared image is deallocated by this operation.
func ClearBorderCache() {
	globalBorderCache.mu.Lock()
	defer globalBorderCache.mu.Unlock()
	clear(globalBorderCache.entries)
	globalBorderCache.recent.Init()
	globalBorderCache.bytes = 0
}
