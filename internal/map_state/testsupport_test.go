package map_state

// GetCurrentAngle returns the current rotation angle (0-15) or UninitializedAngle.
// Useful for debugging and testing.
func (ls *LighthouseState) GetCurrentAngle() int {
	return ls.currentAngle
}

// AdvanceTurn advances the lighting by one elapsed minute.
func (l *Lighting) AdvanceTurn() {
	l.AdvanceMinutes(1)
}
