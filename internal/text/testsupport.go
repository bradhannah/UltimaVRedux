package text

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// ClearTextCache drops shared raster reuse without invalidating retained geometry.
// Cross-package native fixtures use this seam to retire independent application
// lifetimes and verify widget-owned compositions survive shared-cache eviction.
// Production scenes share the application renderer until CloseRenderer.
func ClearTextCache() {
	textRenderer.ClearCaches()
	publishTextAccounting()
}
