package color

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// Matches returns true if the given RGB values exactly match this EGA color.
func (c EGAColor) Matches(r, g, b uint8) bool {
	return c.R == r && c.G == g && c.B == b
}
