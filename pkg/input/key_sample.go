package input

import (
	beradinput "github.com/bradhannah/berad-sdk/modules/berad-input"
	"github.com/hajimehoshi/ebiten/v2"
)

// KeySample is one detached physical-key snapshot. Pressed is an edge, not a
// repeat or an ordering claim about keys pressed between application updates.
// The arrays are projections for game policy; admission uses the SDK capture's
// identity and cannot consume a stale, foreign, or manually fabricated sample.
type KeySample struct {
	Held     [ebiten.KeyMax + 1]bool
	Pressed  [ebiten.KeyMax + 1]bool
	captured beradinput.Sample[ebiten.Key]
}

// CaptureKeySample samples once before advancing input owners. Readers still
// receive their ordinary input; sampling alone consumes nothing.
func (k *Keyboard) CaptureKeySample() KeySample {
	var sample KeySample
	sample.captured = k.keys.Capture()
	for key := ebiten.KeyA; key <= ebiten.KeyMax; key++ {
		sample.Held[key] = sample.captured.Held(key)
		sample.Pressed[key] = sample.captured.Pressed(key)
	}

	return sample
}

// SuppressUntilRelease rejects this physical press even in repeatable movement.
// It deliberately survives ResetReleaseDebouncing and command cancellation.
func (k *Keyboard) SuppressUntilRelease(key ebiten.Key) {
	k.keys.SuppressUntilRelease(key)
}

// IsSuppressedUntilRelease is separate from normal one-shot consumption, which
// repeatable movement is allowed to bypass.
func (k *Keyboard) IsSuppressedUntilRelease(key ebiten.Key) bool {
	return k.keys.Suppressed(key)
}

// HasUnprotectedHeldKey is a workflow-release query, not an acknowledgement
// query. Rejected presses remain physically held for raw any-key waits.
func (k *Keyboard) HasUnprotectedHeldKey() bool {
	return k.hasUnprotectedHeldKey(nil)
}

// HasUnprotectedHeldKeyExcept also treats one key held by a scene-owned repeat
// as protected, so the repeat can keep it down through workflow hand-offs.
func (k *Keyboard) HasUnprotectedHeldKeyExcept(owned ebiten.Key) bool {
	return k.hasUnprotectedHeldKey(&owned)
}

func (k *Keyboard) hasUnprotectedHeldKey(owned *ebiten.Key) bool {
	for key := ebiten.KeyA; key <= ebiten.KeyMax; key++ {
		if (owned == nil || key != *owned) && k.isKeyPressed(key) && !k.IsSuppressedUntilRelease(key) {
			return true
		}
	}

	return false
}

// ConsumePressEdge accepts an already captured fresh press without inheriting
// movement cadence. Ownership/transition gates still apply.
func (k *Keyboard) ConsumePressEdge(sample KeySample, key ebiten.Key) bool {
	return k.keys.ConsumeEdge(sample.captured, key)
}

// SuspendInputPolling masks input while an owner advances non-input work, for
// example while its application window lacks focus. Synthetic input is discarded,
// not replayed later. Restore once; physical holds remain blocked until release.
func (k *Keyboard) SuspendInputPolling() func() {
	k.hasSimulatedKeyPress = false

	return k.keys.Suspend()
}

// HasModifiers distinguishes plain command input from reserved shortcuts.
func (s KeySample) HasModifiers() bool {
	for _, key := range [...]ebiten.Key{
		ebiten.KeyShift, ebiten.KeyShiftLeft, ebiten.KeyShiftRight,
		ebiten.KeyControl, ebiten.KeyControlLeft, ebiten.KeyControlRight,
		ebiten.KeyAlt, ebiten.KeyAltLeft, ebiten.KeyAltRight,
		ebiten.KeyMeta, ebiten.KeyMetaLeft, ebiten.KeyMetaRight,
	} {
		if s.Held[key] {
			return true
		}
	}

	return false
}
