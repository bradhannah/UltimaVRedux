package profiling

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// Disable stops collection without discarding a caller's completed snapshot.
func Disable() { active.Store(nil) }
