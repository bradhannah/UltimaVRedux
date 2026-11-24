package input

import "github.com/hajimehoshi/ebiten/v2"

// Test support: fixtures and seams that tests in other packages use. The
// dead-code check treats this file as test support.

// NewKeyboardWithKeyStateSource constructs a keyboard backed by a caller-owned
// key-state source. It supports deterministic input adapters and tests without
// changing the keyboard's release-debouncing behavior.
func NewKeyboardWithKeyStateSource(
	millisecondDelayBetweenKeyPresses int,
	keyPressed func(ebiten.Key) bool,
) *Keyboard {
	if keyPressed == nil {
		panic("NewKeyboardWithKeyStateSource: key-state source is required")
	}
	keyboard := NewKeyboard(millisecondDelayBetweenKeyPresses)
	keyboard.keyPressed = keyPressed

	return keyboard
}

// SetElapsedMillisecondsSource injects the adapter's elapsed clock. Configure it
// before accepting input; switching clocks resets timing, but not release gates.
func (k *Keyboard) SetElapsedMillisecondsSource(source func() int64) {
	if source == nil {
		panic("Keyboard.SetElapsedMillisecondsSource: clock is required")
	}
	k.elapsedMilliseconds = source
	k.cadence.ResetClock()
	k.SetAllowKeyPressImmediately()
}

// SimulateKeyPress simulates a key press for testing purposes
// This bypasses the normal key press timing restrictions
func (k *Keyboard) SimulateKeyPress(key ebiten.Key) {
	if k.keys.Suspended() {
		return
	}
	k.simulatedKeyPressed = key
	k.hasSimulatedKeyPress = true
}
